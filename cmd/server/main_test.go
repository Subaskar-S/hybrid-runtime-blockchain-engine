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

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "ws://localhost:8545", cfg.ETHRPCURL)
	assert.Equal(t, 2, cfg.WorkerCount)

	logger, err := logging.NewLogger()
	require.NoError(t, err)
	defer logger.Sync()

	ffiLayer := ffi.NewFFI()
	require.NotNil(t, ffiLayer)
	require.NoError(t, ffiLayer.InitEngine())

	shutdownMgr := shutdown.NewManager(logger)
	require.NotNil(t, shutdownMgr)

	reorgEngine := reorg.NewReorgEngine(logger.Named("reorg"), ffiLayer)
	require.NotNil(t, reorgEngine)

	workerPool := worker.NewPool(logger.Named("worker"), reorgEngine)
	require.NotNil(t, workerPool)
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		return workerPool.Stop(ctx)
	})

	metricsCollector := metrics.NewCollector(logger.Named("metrics"))
	require.NotNil(t, metricsCollector)
	metricsCollector.RegisterWorkerPool(metrics.NewWorkerPoolAdapter(workerPool))
	metricsCollector.RegisterRustCore(metrics.NewRustCoreAdapter(ffiLayer))
	metricsCollector.RegisterReorgEngine(metrics.NewReorgEngineAdapter(reorgEngine))

	require.NoError(t, metricsCollector.Start(context.Background(), cfg.MetricsPort))
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		return metricsCollector.Stop(ctx)
	})

	mcpServer := mcp.NewServer(logger.Named("mcp"), cfg.MCPPort)
	require.NotNil(t, mcpServer)
	runtimeTools := mcp.RegisterRuntimeTools(mcpServer, reorgEngine, metrics.NewMCPFFIAdapter(ffiLayer))
	require.NotNil(t, runtimeTools)
	require.NoError(t, mcpServer.Start())
	shutdownMgr.RegisterShutdownFunc(func(ctx context.Context) error {
		return mcpServer.Stop()
	})

	require.NoError(t, workerPool.Start(context.Background(), cfg.WorkerCount))
	time.Sleep(50 * time.Millisecond)
	assert.Greater(t, workerPool.ActiveWorkers(), 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, shutdownMgr.Shutdown(5*time.Second))

	select {
	case <-ctx.Done():
		t.Fatal("Shutdown did not complete in time")
	default:
	}
}

func TestComponentCommunication(t *testing.T) {
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

	cfg, err := config.Load()
	require.NoError(t, err)

	logger, err := logging.NewLogger()
	require.NoError(t, err)
	defer logger.Sync()

	ffiLayer := ffi.NewFFI()
	require.NoError(t, ffiLayer.InitEngine())

	reorgEngine := reorg.NewReorgEngine(logger.Named("reorg"), ffiLayer)
	workerPool := worker.NewPool(logger.Named("worker"), reorgEngine)
	mcpServer := mcp.NewServer(logger.Named("mcp"), cfg.MCPPort)
	runtimeTools := mcp.RegisterRuntimeTools(mcpServer, reorgEngine, metrics.NewMCPFFIAdapter(ffiLayer))

	require.NoError(t, workerPool.Start(context.Background(), cfg.WorkerCount))
	require.NoError(t, mcpServer.Start())

	// Submit a block with no transactions (avoids insufficient-balance error)
	block := &ffi.Block{
		Number:       1,
		ParentHash:   ffi.Hash{},
		Timestamp:    uint64(time.Now().Unix()),
		Transactions: []ffi.Transaction{},
	}
	require.NoError(t, workerPool.Submit(block))

	// Give worker time to process
	time.Sleep(200 * time.Millisecond)

	// Verify stats are available via FFI
	stats, err := ffiLayer.GetStats()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), stats.BlockNumber)

	// Verify MCP tools can access runtime data
	result, err := runtimeTools.GetStateRoot(nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workerPool.Stop(ctx)
	mcpServer.Stop()
}

func TestShutdownSequence(t *testing.T) {
	os.Setenv("ETH_RPC_URL", "ws://localhost:8545")
	os.Setenv("WORKER_COUNT", "2")
	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("WORKER_COUNT")
	}()

	logger, err := logging.NewLogger()
	require.NoError(t, err)
	defer logger.Sync()

	shutdownMgr := shutdown.NewManager(logger)

	var shutdownOrder []string
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

	require.NoError(t, shutdownMgr.Shutdown(5*time.Second))
	assert.Equal(t, []string{"component3", "component2", "component1"}, shutdownOrder)
}
