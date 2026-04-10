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
	blockStreamer := streamer.NewEthBlockStreamer(logger.Named("streamer"))
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping Block Streamer")
		return blockStreamer.Stop()
	})

	// Initialize Reorg Engine
	reorgEngine := reorg.NewReorgEngine(logger.Named("reorg"), ffiLayer)

	// Initialize Worker Pool — the reorg engine acts as the block processor
	workerPool := worker.NewPool(logger.Named("worker"), reorgEngine)
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping Worker Pool", zap.Duration("timeout", 30*time.Second))
		return workerPool.Stop(ctx)
	})

	// Initialize Metrics Collector
	metricsCollector := metrics.NewCollector(logger.Named("metrics"))
	metricsCollector.RegisterWorkerPool(metrics.NewWorkerPoolAdapter(workerPool))
	metricsCollector.RegisterRustCore(metrics.NewRustCoreAdapter(ffiLayer))
	metricsCollector.RegisterReorgEngine(metrics.NewReorgEngineAdapter(reorgEngine))
	metricsCollector.RegisterBlockStreamer(blockStreamer)
	metricsCollector.RegisterWorkerPoolHealth(workerPool)

	if err := metricsCollector.Start(context.Background(), cfg.MetricsPort); err != nil {
		logger.Fatal("Failed to start Metrics Collector", zap.Error(err))
	}
	logger.Info("Metrics Collector started", zap.Int("port", cfg.MetricsPort))

	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping Metrics Collector")
		return metricsCollector.Stop(ctx)
	})

	// Initialize MCP Server
	mcpServer := mcp.NewServer(logger.Named("mcp"), cfg.MCPPort)
	runtimeTools := mcp.RegisterRuntimeTools(mcpServer, reorgEngine, metrics.NewMCPFFIAdapter(ffiLayer))

	if err := mcpServer.Start(); err != nil {
		logger.Fatal("Failed to start MCP Server", zap.Error(err))
	}
	logger.Info("MCP Server started", zap.Int("port", cfg.MCPPort))

	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		logger.Info("Stopping MCP Server")
		return mcpServer.Stop()
	})

	// Initialize Load Tester (if enabled)
	if cfg.LoadTestEnabled {
		lt := loadtest.NewLoadTester(logger.Named("loadtest"), workerPool, 42)
		runtimeTools.SetLoadTester(lt)
		logger.Info("Load Tester enabled")
	}

	// Start Worker Pool
	if err := workerPool.Start(context.Background(), cfg.WorkerCount); err != nil {
		logger.Fatal("Failed to start Worker Pool", zap.Error(err))
	}
	logger.Info("Worker Pool started", zap.Int("worker_count", cfg.WorkerCount))

	// Start Block Streamer — in load test mode a real RPC connection is optional
	if err := blockStreamer.Start(context.Background(), cfg.ETHRPCURL); err != nil {
		if cfg.LoadTestEnabled {
			logger.Warn("Block Streamer failed to connect (load test mode — continuing without live blocks)",
				zap.Error(err))
		} else {
			logger.Fatal("Failed to start Block Streamer", zap.Error(err))
		}
	} else {
		logger.Info("Block Streamer started")
	}

	// Wire: forward blocks from streamer → worker pool, recording latency
	go func() {
		for block := range blockStreamer.Blocks() {
			start := time.Now()
			if err := workerPool.Submit(block); err != nil {
				logger.Error("Failed to submit block", zap.Error(err))
				continue
			}
			latencyMs := float64(time.Since(start).Milliseconds())
			runtimeTools.GetLatencyTracker().Record(latencyMs)
		}
	}()

	logger.Info("All components started successfully")

	// Wait for shutdown signal
	sig := shutdownMgr.WaitForShutdown()
	logger.Info("Shutdown signal received", zap.String("signal", sig.String()))

	// Query and log final state
	if stateRoot, err := ffiLayer.GetStateRoot(); err != nil {
		logger.Error("Failed to get final state root", zap.Error(err))
	} else {
		logger.Info("Final state root", zap.String("state_root", fmt.Sprintf("%x", stateRoot)))
	}

	if stats, err := ffiLayer.GetStats(); err != nil {
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
