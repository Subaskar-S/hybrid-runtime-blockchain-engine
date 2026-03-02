// +build !test

package metrics

import (
	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/hybrid-runtime-blockchain-engine/internal/reorg"
	"github.com/hybrid-runtime-blockchain-engine/internal/worker"
)

// WorkerPoolAdapter adapts worker.Pool to WorkerPoolStats interface
type WorkerPoolAdapter struct {
	pool *worker.Pool
}

// NewWorkerPoolAdapter creates a new worker pool adapter
func NewWorkerPoolAdapter(pool *worker.Pool) *WorkerPoolAdapter {
	return &WorkerPoolAdapter{pool: pool}
}

// GetStats returns worker pool statistics
func (a *WorkerPoolAdapter) GetStats() WorkerStats {
	stats := a.pool.GetStats()
	return WorkerStats{
		NumWorkers:      stats.NumWorkers,
		ActiveWorkers:   stats.ActiveWorkers,
		ProcessedBlocks: stats.ProcessedBlocks,
		PanicCount:      stats.PanicCount,
		QueueDepth:      stats.QueueDepth,
	}
}

// ReorgEngineAdapter adapts reorg.ReorgEngine to ReorgEngineStats interface
type ReorgEngineAdapter struct {
	engine *reorg.ReorgEngine
}

// NewReorgEngineAdapter creates a new reorg engine adapter
func NewReorgEngineAdapter(engine *reorg.ReorgEngine) *ReorgEngineAdapter {
	return &ReorgEngineAdapter{engine: engine}
}

// GetStats returns reorg engine statistics
func (a *ReorgEngineAdapter) GetStats() ReorgStats {
	stats := a.engine.GetStats()
	
	// Convert reorg events
	events := make([]ReorgEvent, len(stats.RecentReorgs))
	for i, e := range stats.RecentReorgs {
		events[i] = ReorgEvent{
			ForkPoint:          e.ForkPoint,
			Depth:              e.Depth,
			RollbackDurationMs: e.RollbackDurationMs,
		}
	}
	
	return ReorgStats{
		ReorgCount:   stats.ReorgCount,
		BufferSize:   stats.BufferSize,
		RecentReorgs: events,
	}
}

// RustCoreAdapter adapts ffi.FFI to RustCoreStats interface
type RustCoreAdapter struct {
	ffi *ffi.FFI
}

// NewRustCoreAdapter creates a new Rust core adapter
func NewRustCoreAdapter(f *ffi.FFI) *RustCoreAdapter {
	return &RustCoreAdapter{ffi: f}
}

// GetStats returns Rust core statistics
func (a *RustCoreAdapter) GetStats() (*RustStats, error) {
	stats, err := a.ffi.GetStats()
	if err != nil {
		return nil, err
	}
	
	return &RustStats{
		BlockNumber:      stats.BlockNumber,
		StateSize:        stats.StateSize,
		HistoryLength:    stats.HistoryLength,
		MemoryUsageBytes: stats.MemoryUsageBytes,
	}, nil
}

// GetStateRoot returns the current state root
func (a *RustCoreAdapter) GetStateRoot() ([32]byte, error) {
	return a.ffi.GetStateRoot()
}

// ReorgEventAdapter converts reorg.ReorgEvent to metrics.ReorgEvent
func ReorgEventAdapter(e reorg.ReorgEvent) ReorgEvent {
	return ReorgEvent{
		ForkPoint:          e.ForkPoint,
		Depth:              e.Depth,
		RollbackDurationMs: e.RollbackDurationMs,
	}
}
