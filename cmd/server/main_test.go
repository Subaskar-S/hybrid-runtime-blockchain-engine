package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/config"
	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/hybrid-runtime-blockchain-engine/internal/logging"
	"github.com/hybrid-runtime-blockchain-engine/internal/mcp"
	"github.com/hybrid-runtime-blockchain-engine/internal/metrics"
	"github.com/hybrid-runtime-blockchain-engine/internal/reorg"
	"github.com/hybrid-runtime-blockchain-engine/internal/shutdown"
	"github.com/hybrid-runtime-blockchain-engine/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentWiring(t *testing.T) {
	// Set test environment variables
	os.Setenv("ETH_RPC_URL", "ws://localhost:8545")
	os.Setenv("WORKER_COUNT", "2")
	os.Setenv("METRICS_PORT", "19090")
	os.Setenv("MCP_PORT", "18080")
	os.Setenv("LOAD_TEST_ENABLED", "false")
	
	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("WORKER_COUNT")
		os.Unsetenv("METRICS_PORT")
		os.Unsetenv("MCP_PORT")
		os.Unsetenv("LOAD_TEST_ENABLED")
	}()

	// Load configuration
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "ws://localhost:8545", cfg.ETHRPCURL)
	assert.Equal(t, 2, cfg.WorkerCount)

	// Initialize logger
	logger, err := logging.NewLogger()
	require.NoError(t, err)
	defer logger.Sync()

	// Initialize FFI layer
	ffiLayer := ffi.NewFFI()
	require.NotNil(t, ffiLayer)

	// Initialize Rust core
	err = ffiLayer.InitEngine()
	require.NoError(t, err)

	// Initialize shutdown manager
	shutdownMgr := shutdown.NewManager(logger)
	require.NotNil(t, shutdownMgr)

	// Initialize Worker Pool
	workerPool := worker.NewWorkerPool(logger.Named("worker"), cfg.WorkerCount, ffiLayer)
	require.NotNil(t, workerPool)
	
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		return workerPool.Stop(ctx)
	})

	// Initialize Reorg Engine
	reorgEngine := reorg.NewReorgEngine(logger.Named("reorg"), ffiLayer)
	require.NotNil(t, reorgEngine)

	// Initialize Metrics Collector
	metricsCollector := metrics.NewCollector(logger.Named("metrics"), cfg.MetricsPort)
	require.NotNil(t, metricsCollector)
	
	metricsCollector.RegisterAdapters(
		metrics.NewWorkerPoolAdapter(workerPool),
		metrics.NewFFIAdapter(ffiLayer),
		metrics.NewReorgEngineAdapter(reorgEngine),
	)
	
	err = metricsCollector.Start()
	require.NoError(t, err)
	
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		return metricsCollector.Stop(ctx)
	})

	// Initialize MCP Server
	mcpServer := mcp.NewServer(logger.Named("mcp"), cfg.MCPPort)
	require.NotNil(t, mcpServer)
	
	runtimeTools := mcp.RegisterRuntimeTools(mcpServer, reorgEngine, ffiLayer)
	require.NotNil(t, runtimeTools)
	
	err = mcpServer.Start()
	require.NoError(t, err)
	
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		return mcpServer.Stop(ctx)
	})

	// Start Worker Pool
	workerPool.Start()

	// Verify components are running
	assert.Greater(t, workerPool.ActiveWorkers(), 0)

	// Give components time to initialize
	time.Sleep(100 * time.Millisecond)

	// Execute graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err = shutdownMgr.Shutdown(5 * time.Second)
	assert.NoError(t, err)

	// Verify shutdown completed
	select {
	case <-ctx.Done():
		t.Fatal("Shutdown did not complete in time")
	default:
		// Shutdown completed successfully
	}
}

func TestComponentCommunication(t *testing.T) {
	// Set test environment variables
	os.Setenv("ETH_RPC_URL", "ws://localhost:8545")
	os.Setenv("WORKER_COUNT", "2")
	os.Setenv("METRICS_PORT", "19091")
	os.Setenv("MCP_PORT", "18081")
	
	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("WORKER_COUNT")
		os.Unsetenv("METRICS_PORT")
		os.Unsetenv("MCP_PORT")
	}()

	// Load configuration
	cfg, err := config.Load()
	require.NoError(t, err)

	// Initialize logger
	logger, err := logging.NewLogger()
	require.NoError(t, err)
	defer logger.Sync()

	// Initialize FFI layer
	ffiLayer := ffi.NewFFI()
	err = ffiLayer.InitEngine()
	require.NoError(t, err)

	// Initialize components
	workerPool := worker.NewWorkerPool(logger.Named("worker"), cfg.WorkerCount, ffiLayer)
	reorgEngine := reorg.NewReorgEngine(logger.Named("reorg"), ffiLayer)
	mcpServer := mcp.NewServer(logger.Named("mcp"), cfg.MCPPort)
	runtimeTools := mcp.RegisterRuntimeTools(mcpServer, reorgEngine, ffiLayer)

	// Start components
	workerPool.Start()
	err = mcpServer.Start()
	require.NoError(t, err)

	// Test communication: Submit a block to worker pool
	block := &ffi.Block{
		Number:     1,
		ParentHash: [32]byte{},
		Timestamp:  uint64(time.Now().Unix()),
		Transactions: []ffi.Transaction{
			{
				From:  [20]byte{0x01},
				To:    [20]byte{0x02},
				Value: [32]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64}, // 100
				Nonce: 0,
			},
		},
	}

	workerPool.Submit(block)

	// Give worker time to process
	time.Sleep(100 * time.Millisecond)

	// Verify state was updated via FFI
	stateRoot, err := ffiLayer.GetStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, [32]byte{}, stateRoot)

	// Verify stats are available
	stats, err := ffiLayer.GetStats()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), stats.BlockNumber)
	assert.Greater(t, stats.StateSize, 0)

	// Test MCP tools can access runtime data
	result, err := runtimeTools.GetStateRoot(nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	workerPool.Stop(ctx)
	mcpServer.Stop(ctx)
}

func TestShutdownSequence(t *testing.T) {
	// Set test environment variables
	os.Setenv("ETH_RPC_URL", "ws://localhost:8545")
	os.Setenv("WORKER_COUNT", "2")
	
	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("WORKER_COUNT")
	}()

	// Initialize logger
	logger, err := logging.NewLogger()
	require.NoError(t, err)
	defer logger.Sync()

	// Initialize shutdown manager
	shutdownMgr := shutdown.NewManager(logger)

	// Track shutdown order
	var shutdownOrder []string

	// Register shutdown functions in order
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		shutdownOrder = append(shutdownOrder, "component1")
		return nil
	})

	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		shutdownOrder = append(shutdownOrder, "component2")
		return nil
	})

	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		shutdownOrder = append(shutdownOrder, "component3")
		return nil
	})

	// Execute shutdown
	err = shutdownMgr.Shutdown(5 * time.Second)
	require.NoError(t, err)

	// Verify LIFO order (component3, component2, component1)
	assert.Equal(t, []string{"component3", "component2", "component1"}, shutdownOrder)
}
