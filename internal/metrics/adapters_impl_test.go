package metrics

import (
	"testing"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/hybrid-runtime-blockchain-engine/internal/reorg"
	"github.com/hybrid-runtime-blockchain-engine/internal/worker"
	"go.uber.org/zap"
)

// TestAdapters_WithRealFFI exercises all adapter code paths using a live
// Rust core. This test requires CGO_ENABLED=1 and the compiled Rust library.
func TestAdapters_WithRealFFI(t *testing.T) {
	logger := zap.NewNop()

	ffiLayer := ffi.NewFFI()
	if err := ffiLayer.InitEngine(); err != nil {
		t.Fatalf("InitEngine: %v", err)
	}

	// WorkerPoolAdapter
	reorgEngine := reorg.NewReorgEngine(logger, ffiLayer)
	pool := worker.NewPool(logger, reorgEngine)
	wpa := NewWorkerPoolAdapter(pool)
	s := wpa.GetStats()
	if s.NumWorkers != 0 {
		t.Errorf("expected 0 workers before start, got %d", s.NumWorkers)
	}

	// ReorgEngineAdapter
	rea := NewReorgEngineAdapter(reorgEngine)
	rs := rea.GetStats()
	if rs.ReorgCount != 0 {
		t.Errorf("expected 0 reorgs, got %d", rs.ReorgCount)
	}

	// RustCoreAdapter
	rca := NewRustCoreAdapter(ffiLayer)
	rustStats, err := rca.GetStats()
	if err != nil {
		t.Fatalf("RustCoreAdapter.GetStats: %v", err)
	}
	if rustStats.BlockNumber != 0 {
		t.Errorf("expected block 0, got %d", rustStats.BlockNumber)
	}
	root, err := rca.GetStateRoot()
	if err != nil {
		t.Fatalf("RustCoreAdapter.GetStateRoot: %v", err)
	}
	var zero [32]byte
	if root != zero {
		t.Errorf("expected zero state root initially")
	}

	// MCPFFIAdapter
	mcpAdapter := NewMCPFFIAdapter(ffiLayer)
	mcpStats, err := mcpAdapter.GetStats()
	if err != nil {
		t.Fatalf("MCPFFIAdapter.GetStats: %v", err)
	}
	if mcpStats.BlockNumber != 0 {
		t.Errorf("expected block 0, got %d", mcpStats.BlockNumber)
	}
	mcpRoot, err := mcpAdapter.GetStateRoot()
	if err != nil {
		t.Fatalf("MCPFFIAdapter.GetStateRoot: %v", err)
	}
	if mcpRoot != zero {
		t.Errorf("expected zero state root")
	}
}
