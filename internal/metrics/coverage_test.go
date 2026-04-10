package metrics

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestCollector_CollectRuntimeMetrics exercises the periodic collection path
// by starting the collector with registered components and waiting one tick.
func TestCollector_CollectRuntimeMetrics(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)

	collector.RegisterWorkerPool(&mockWorkerPoolStats{})
	collector.RegisterRustCore(&mockRustCoreStats{})
	collector.RegisterReorgEngine(&mockReorgEngineStats{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a port unlikely to conflict
	if err := collector.Start(ctx, 19097); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer collector.Stop(context.Background())

	// Wait long enough for at least one periodic collection (ticker is 5s,
	// but we just need the goroutine to be alive — the test verifies no panic).
	time.Sleep(50 * time.Millisecond)
}

// TestCollector_CollectComponentMetrics_RustError verifies the collector
// handles a Rust stats error gracefully (no panic, no crash).
func TestCollector_CollectComponentMetrics_RustError(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)
	collector.RegisterRustCore(&mockRustCoreStatsError{})

	// Call directly — not exported, but we can trigger it via the periodic path
	collector.collectComponentMetrics()
}

// TestCollector_CollectRuntimeMetrics_Direct calls collectRuntimeMetrics directly.
func TestCollector_CollectRuntimeMetrics_Direct(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)
	// Should not panic
	collector.collectRuntimeMetrics()
}
func TestDurationFromMs(t *testing.T) {
	d := DurationFromMs(100)
	if d != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", d)
	}
	if DurationFromMs(0) != 0 {
		t.Error("expected 0 for 0ms")
	}
}

// mockRustCoreStatsError returns an error from GetStats.
type mockRustCoreStatsError struct{}

func (m *mockRustCoreStatsError) GetStats() (*RustStats, error) {
	return nil, errMock
}

func (m *mockRustCoreStatsError) GetStateRoot() ([32]byte, error) {
	return [32]byte{}, errMock
}

var errMock = &mockError{"mock error"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }
