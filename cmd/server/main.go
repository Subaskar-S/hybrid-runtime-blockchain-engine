package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/config"
	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/hybrid-runtime-blockchain-engine/internal/loadtest"
	"github.com/hybrid-runtime-blockchain-engine/internal/logging"
	"github.com/hybrid-runtime-blockchain-engine/internal/mcp"
	"github.com/hybrid-runtime-blockchain-engine/internal/metrics"
	"github.com/hybrid-runtime-blockchain-engine/internal/reorg"
	"github.com/hybrid-runtime-blockchain-engine/internal/shutdown"
	"github.com/hybrid-runtime-blockchain-engine/internal/streamer"
	"github.com/hybrid-runtime-blockchain-engine/internal/worker"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize structured logging
	logger, err := logging.NewLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting Hybrid Runtime Blockchain Engine",
		zap.String("eth_rpc_url", cfg.ETHRPCURL),
		zap.Int("worker_count", cfg.WorkerCount),
		zap.Int("metrics_port", cfg.MetricsPort),
		zap.Int("mcp_port", cfg.MCPPort),
		zap.Bool("load_test_enabled", cfg.LoadTestEnabled),
	)

	// Initialize FFI layer
	ffiLayer := ffi.NewFFI()
	logger.Info("FFI layer initialized")

	// Initialize Rust core
	if err := ffiLayer.InitEngine(); err != nil {
		logger.Fatal("Failed to initialize Rust core", zap.Error(err))
	}
	logger.Info("Rust core initialized")

	// Initialize shutdown manager
	shutdownMgr := shutdown.NewManager(logger)

	// Initialize Block Streamer
	blockStreamer := streamer.NewBlockStreamer(logger.Named("streamer"), cfg.ETHRPCURL)
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping Block Streamer")
		blockStreamer.Stop()
		return nil
	})

	// Initialize Worker Pool
	workerPool := worker.NewWorkerPool(logger.Named("worker"), cfg.WorkerCount, ffiLayer)
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping Worker Pool", zap.Duration("timeout", 30*time.Second))
		return workerPool.Stop(ctx)
	})

	// Initialize Reorg Engine
	reorgEngine := reorg.NewReorgEngine(logger.Named("reorg"), ffiLayer)

	// Initialize Metrics Collector
	metricsCollector := metrics.NewCollector(logger.Named("metrics"), cfg.MetricsPort)
	metricsCollector.RegisterAdapters(
		metrics.NewWorkerPoolAdapter(workerPool),
		metrics.NewFFIAdapter(ffiLayer),
		metrics.NewReorgEngineAdapter(reorgEngine),
	)
	
	if err := metricsCollector.Start(); err != nil {
		logger.Fatal("Failed to start Metrics Collector", zap.Error(err))
	}
	logger.Info("Metrics Collector started", zap.Int("port", cfg.MetricsPort))
	
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping Metrics Collector")
		return metricsCollector.Stop(ctx)
	})

	// Initialize MCP Server
	mcpServer := mcp.NewServer(logger.Named("mcp"), cfg.MCPPort)
	runtimeTools := mcp.RegisterRuntimeTools(mcpServer, reorgEngine, ffiLayer)
	
	if err := mcpServer.Start(); err != nil {
		logger.Fatal("Failed to start MCP Server", zap.Error(err))
	}
	logger.Info("MCP Server started", zap.Int("port", cfg.MCPPort))
	
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping MCP Server")
		return mcpServer.Stop(ctx)
	})

	// Initialize Load Tester (if enabled)
	var loadTester *loadtest.LoadTester
	if cfg.LoadTestEnabled {
		loadTester = loadtest.NewLoadTester(logger.Named("loadtest"), ffiLayer, workerPool, reorgEngine)
		runtimeTools.SetLoadTester(loadTester)
		logger.Info("Load Tester enabled")
	}

	// Start Worker Pool
	workerPool.Start()
	logger.Info("Worker Pool started", zap.Int("worker_count", cfg.WorkerCount))

	// Start Block Streamer
	if err := blockStreamer.Start(); err != nil {
		logger.Fatal("Failed to start Block Streamer", zap.Error(err))
	}
	logger.Info("Block Streamer started")

	// Wire components together - process blocks from streamer
	go func() {
		blockChan := blockStreamer.Blocks()
		for block := range blockChan {
			// Record latency
			start := time.Now()
			
			// Check for reorg
			if reorgEngine.DetectReorg(block) {
				logger.Warn("Reorg detected",
					zap.Uint64("block_number", block.Number),
					zap.String("parent_hash", fmt.Sprintf("%x", block.ParentHash)),
				)
				
				// Handle reorg
				if err := reorgEngine.HandleReorg(block); err != nil {
					logger.Error("Failed to handle reorg", zap.Error(err))
					continue
				}
			}
			
			// Submit block to worker pool
			workerPool.Submit(block)
			
			// Record latency
			latencyMs := float64(time.Since(start).Milliseconds())
			runtimeTools.GetLatencyTracker().Record(latencyMs)
		}
	}()

	logger.Info("All components started successfully")

	// Wait for shutdown signal
	sig := shutdownMgr.WaitForShutdown()
	logger.Info("Shutdown signal received", zap.String("signal", sig.String()))

	// Query and log final state
	stateRoot, err := ffiLayer.GetStateRoot()
	if err != nil {
		logger.Error("Failed to get final state root", zap.Error(err))
	} else {
		logger.Info("Final state root", zap.String("state_root", fmt.Sprintf("%x", stateRoot)))
	}

	stats, err := ffiLayer.GetStats()
	if err != nil {
		logger.Error("Failed to get final stats", zap.Error(err))
	} else {
		logger.Info("Final stats",
			zap.Uint64("block_number", stats.BlockNumber),
			zap.Int("state_size", stats.StateSize),
			zap.Int("history_length", stats.HistoryLength),
			zap.Int("memory_usage_bytes", stats.MemoryUsageBytes),
		)
	}

	// Execute graceful shutdown
	if err := shutdownMgr.Shutdown(30 * time.Second); err != nil {
		logger.Error("Graceful shutdown failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("Shutdown complete")
}
