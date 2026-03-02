package mcp

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/hybrid-runtime-blockchain-engine/internal/reorg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLatencyTracker_Record(t *testing.T) {
	tracker := NewLatencyTracker(10)

	// Record some latencies
	tracker.Record(1.5)
	tracker.Record(2.3)
	tracker.Record(3.1)

	assert.Equal(t, 3, tracker.size)
	assert.Equal(t, 3, tracker.index)
}

func TestLatencyTracker_CircularBuffer(t *testing.T) {
	tracker := NewLatencyTracker(3)

	// Fill buffer
	tracker.Record(1.0)
	tracker.Record(2.0)
	tracker.Record(3.0)

	assert.Equal(t, 3, tracker.size)

	// Overflow - should wrap around
	tracker.Record(4.0)

	assert.Equal(t, 3, tracker.size)
	assert.Equal(t, 1, tracker.index)
}

func TestLatencyTracker_GetDistribution_Empty(t *testing.T) {
	tracker := NewLatencyTracker(10)

	dist := tracker.GetDistribution()

	assert.Equal(t, 0.0, dist["p50_ms"])
	assert.Equal(t, 0.0, dist["p95_ms"])
	assert.Equal(t, 0.0, dist["p99_ms"])
	assert.Equal(t, 0.0, dist["max_ms"])
}

func TestLatencyTracker_GetDistribution_SingleValue(t *testing.T) {
	tracker := NewLatencyTracker(10)
	tracker.Record(5.0)

	dist := tracker.GetDistribution()

	assert.Equal(t, 5.0, dist["p50_ms"])
	assert.Equal(t, 5.0, dist["p95_ms"])
	assert.Equal(t, 5.0, dist["p99_ms"])
	assert.Equal(t, 5.0, dist["max_ms"])
}

func TestLatencyTracker_GetDistribution_MultipleValues(t *testing.T) {
	tracker := NewLatencyTracker(100)

	// Record 100 values from 1.0 to 100.0
	for i := 1; i <= 100; i++ {
		tracker.Record(float64(i))
	}

	dist := tracker.GetDistribution()

	// Check percentiles are in expected ranges
	assert.InDelta(t, 50.0, dist["p50_ms"], 1.0)
	assert.InDelta(t, 95.0, dist["p95_ms"], 1.0)
	assert.InDelta(t, 99.0, dist["p99_ms"], 1.0)
	assert.Equal(t, 100.0, dist["max_ms"])
}

func TestGetGCStats(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	// Trigger a GC to ensure we have stats
	runtime.GC()

	result, err := tools.GetGCStats(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "num_gc")
	assert.Contains(t, resultMap, "pause_total_ns")
	assert.Contains(t, resultMap, "pause_ns")
	assert.Contains(t, resultMap, "last_gc")

	// Verify types
	numGC, ok := resultMap["num_gc"].(uint32)
	assert.True(t, ok)
	assert.Greater(t, numGC, uint32(0))

	pauseTotalNs, ok := resultMap["pause_total_ns"].(uint64)
	assert.True(t, ok)
	assert.GreaterOrEqual(t, pauseTotalNs, uint64(0))

	pauseNs, ok := resultMap["pause_ns"].([]uint64)
	assert.True(t, ok)
	assert.NotEmpty(t, pauseNs)

	lastGC, ok := resultMap["last_gc"].(string)
	assert.True(t, ok)
	assert.NotEmpty(t, lastGC)

	// Verify last_gc is valid RFC3339 timestamp
	_, err = time.Parse(time.RFC3339, lastGC)
	assert.NoError(t, err)
}

func TestGetHeapUsage(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	result, err := tools.GetHeapUsage(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "alloc_bytes")
	assert.Contains(t, resultMap, "total_alloc_bytes")
	assert.Contains(t, resultMap, "sys_bytes")
	assert.Contains(t, resultMap, "heap_objects")

	// Verify types and values are reasonable
	allocBytes, ok := resultMap["alloc_bytes"].(uint64)
	assert.True(t, ok)
	assert.Greater(t, allocBytes, uint64(0))

	totalAllocBytes, ok := resultMap["total_alloc_bytes"].(uint64)
	assert.True(t, ok)
	assert.Greater(t, totalAllocBytes, uint64(0))

	sysBytes, ok := resultMap["sys_bytes"].(uint64)
	assert.True(t, ok)
	assert.Greater(t, sysBytes, uint64(0))

	heapObjects, ok := resultMap["heap_objects"].(uint64)
	assert.True(t, ok)
	assert.Greater(t, heapObjects, uint64(0))
}

func TestGetGoroutineCount(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	result, err := tools.GetGoroutineCount(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify count field is present
	assert.Contains(t, resultMap, "count")

	count, ok := resultMap["count"].(int)
	assert.True(t, ok)
	assert.Greater(t, count, 0) // At least the test goroutine
}

func TestGetLatencyDistribution(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	// Record some latencies
	tracker := tools.GetLatencyTracker()
	tracker.Record(1.0)
	tracker.Record(2.0)
	tracker.Record(3.0)
	tracker.Record(4.0)
	tracker.Record(5.0)

	result, err := tools.GetLatencyDistribution(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all percentile fields are present
	assert.Contains(t, resultMap, "p50_ms")
	assert.Contains(t, resultMap, "p95_ms")
	assert.Contains(t, resultMap, "p99_ms")
	assert.Contains(t, resultMap, "max_ms")

	// Verify types
	p50, ok := resultMap["p50_ms"].(float64)
	assert.True(t, ok)
	assert.Greater(t, p50, 0.0)

	max, ok := resultMap["max_ms"].(float64)
	assert.True(t, ok)
	assert.Equal(t, 5.0, max)
}

func TestGetLatencyDistribution_Empty(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	result, err := tools.GetLatencyDistribution(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// All values should be zero when no latencies recorded
	assert.Equal(t, 0.0, resultMap["p50_ms"])
	assert.Equal(t, 0.0, resultMap["p95_ms"])
	assert.Equal(t, 0.0, resultMap["p99_ms"])
	assert.Equal(t, 0.0, resultMap["max_ms"])
}

func TestGetReorgHistory_NoReorgs(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	result, err := tools.GetReorgHistory(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	assert.Contains(t, resultMap, "reorgs")

	reorgs, ok := resultMap["reorgs"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Empty(t, reorgs)
}

func TestGetReorgHistory_WithReorgs(t *testing.T) {
	logger := zap.NewNop()
	// Create reorg engine without FFI (nil is acceptable for this test)
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	// Note: We can't actually trigger reorgs without FFI, so we just test
	// that the function works with an empty history
	result, err := tools.GetReorgHistory(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	reorgs, ok := resultMap["reorgs"].([]map[string]interface{})
	assert.True(t, ok)

	// Verify structure even if empty
	assert.NotNil(t, reorgs)
	
	// If there were reorgs, verify structure
	for _, reorg := range reorgs {
		assert.Contains(t, reorg, "timestamp")
		assert.Contains(t, reorg, "fork_point")
		assert.Contains(t, reorg, "depth")
		assert.Contains(t, reorg, "rollback_duration_ms")

		// Verify timestamp is valid RFC3339
		timestamp, ok := reorg["timestamp"].(string)
		assert.True(t, ok)
		_, err := time.Parse(time.RFC3339, timestamp)
		assert.NoError(t, err)
	}
}

func TestGetReorgHistory_WithLimit(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	// Test with custom limit
	params := map[string]interface{}{
		"limit": float64(5),
	}

	result, err := tools.GetReorgHistory(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	reorgs, ok := resultMap["reorgs"].([]map[string]interface{})
	assert.True(t, ok)
	assert.LessOrEqual(t, len(reorgs), 5)
}

func TestGetReorgHistory_InvalidLimit(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	// Test with negative limit (should default to 10)
	params := map[string]interface{}{
		"limit": float64(-5),
	}

	result, err := tools.GetReorgHistory(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should not error, just use default
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, resultMap, "reorgs")
}

func TestGetReorgHistory_LimitTooLarge(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	tools := NewRuntimeTools(mockReorgEngine, nil)

	// Test with limit > 100 (should cap at 100)
	params := map[string]interface{}{
		"limit": float64(200),
	}

	result, err := tools.GetReorgHistory(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	reorgs, ok := resultMap["reorgs"].([]map[string]interface{})
	assert.True(t, ok)
	assert.LessOrEqual(t, len(reorgs), 100)
}

func TestRegisterRuntimeTools(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)

	tools := RegisterRuntimeTools(server, mockReorgEngine, nil)

	assert.NotNil(t, tools)
	assert.NotNil(t, tools.latencyTracker)
	assert.NotNil(t, tools.reorgEngine)

	// Verify tools are registered
	server.mu.RLock()
	defer server.mu.RUnlock()

	assert.Contains(t, server.handlers, "get_gc_stats")
	assert.Contains(t, server.handlers, "get_heap_usage")
	assert.Contains(t, server.handlers, "get_goroutine_count")
	assert.Contains(t, server.handlers, "get_latency_distribution")
	assert.Contains(t, server.handlers, "get_reorg_history")
}

func TestLatencyTracker_Concurrent(t *testing.T) {
	tracker := NewLatencyTracker(1000)

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				tracker.Record(float64(id*100 + j))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have recorded 1000 values (capacity)
	assert.Equal(t, 1000, tracker.size)

	// Should be able to get distribution without panic
	dist := tracker.GetDistribution()
	assert.NotNil(t, dist)
}

// Tests for ApplyBlockLatencyTracker

func TestApplyBlockLatencyTracker_Record(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(10)

	// Record some latencies
	tracker.Record(1.5)
	tracker.Record(2.3)
	tracker.Record(3.1)

	assert.Equal(t, 3, tracker.size)
	assert.Equal(t, 3, tracker.index)
}

func TestApplyBlockLatencyTracker_CircularBuffer(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(3)

	// Fill buffer
	tracker.Record(1.0)
	tracker.Record(2.0)
	tracker.Record(3.0)

	assert.Equal(t, 3, tracker.size)

	// Overflow - should wrap around
	tracker.Record(4.0)

	assert.Equal(t, 3, tracker.size)
	assert.Equal(t, 1, tracker.index)
}

func TestApplyBlockLatencyTracker_GetLatencies_Empty(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(10)

	latencies := tracker.GetLatencies(100)

	assert.Empty(t, latencies)
}

func TestApplyBlockLatencyTracker_GetLatencies_WithLimit(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(100)

	// Record 10 values
	for i := 1; i <= 10; i++ {
		tracker.Record(float64(i))
	}

	// Get only 5 most recent
	latencies := tracker.GetLatencies(5)

	assert.Equal(t, 5, len(latencies))
	// Most recent should be 10, 9, 8, 7, 6
	assert.Equal(t, 10.0, latencies[0])
	assert.Equal(t, 9.0, latencies[1])
	assert.Equal(t, 8.0, latencies[2])
}

func TestApplyBlockLatencyTracker_GetStatistics_Empty(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(10)

	mean, stddev := tracker.GetStatistics(100)

	assert.Equal(t, 0.0, mean)
	assert.Equal(t, 0.0, stddev)
}

func TestApplyBlockLatencyTracker_GetStatistics_SingleValue(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(10)
	tracker.Record(5.0)

	mean, stddev := tracker.GetStatistics(100)

	assert.Equal(t, 5.0, mean)
	assert.Equal(t, 0.0, stddev)
}

func TestApplyBlockLatencyTracker_GetStatistics_MultipleValues(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(100)

	// Record values 1.0 to 10.0
	for i := 1; i <= 10; i++ {
		tracker.Record(float64(i))
	}

	mean, stddev := tracker.GetStatistics(100)

	// Mean should be 5.5
	assert.InDelta(t, 5.5, mean, 0.01)
	// Standard deviation should be approximately 2.87
	assert.InDelta(t, 2.87, stddev, 0.1)
}

func TestApplyBlockLatencyTracker_GetStatistics_WithLimit(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(100)

	// Record values 1.0 to 10.0
	for i := 1; i <= 10; i++ {
		tracker.Record(float64(i))
	}

	// Get statistics for only last 5 values (10, 9, 8, 7, 6)
	mean, stddev := tracker.GetStatistics(5)

	// Mean should be 8.0
	assert.InDelta(t, 8.0, mean, 0.01)
	// Standard deviation should be approximately 1.41
	assert.InDelta(t, 1.41, stddev, 0.1)
}

func TestApplyBlockLatencyTracker_Concurrent(t *testing.T) {
	tracker := NewApplyBlockLatencyTracker(1000)

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				tracker.Record(float64(id*100 + j))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have recorded 1000 values (capacity)
	assert.Equal(t, 1000, tracker.size)

	// Should be able to get statistics without panic
	mean, stddev := tracker.GetStatistics(100)
	assert.Greater(t, mean, 0.0)
	assert.GreaterOrEqual(t, stddev, 0.0)
}

// Tests for Rust Core Introspection Tools

func TestGetStateRoot(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	
	// Create mock FFI
	mockFFI := &MockFFI{
		stateRoot: [32]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
			0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
			0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20},
		stats: &ffi.Stats{
			BlockNumber:      42,
			StateSize:        100,
			HistoryLength:    10,
			MemoryUsageBytes: 1024,
		},
	}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	result, err := tools.GetStateRoot(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "state_root")
	assert.Contains(t, resultMap, "block_number")

	// Verify state_root is hex string
	stateRoot, ok := resultMap["state_root"].(string)
	assert.True(t, ok)
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20", stateRoot)

	// Verify block_number
	blockNumber, ok := resultMap["block_number"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(42), blockNumber)
}

func TestGetStateRoot_FFIError(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	
	// Create mock FFI that returns error
	mockFFI := &MockFFI{
		shouldError: true,
	}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	result, err := tools.GetStateRoot(nil)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get state root")
}

func TestGetStateSize(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	
	// Create mock FFI
	mockFFI := &MockFFI{
		stats: &ffi.Stats{
			BlockNumber:      42,
			StateSize:        100,
			HistoryLength:    10,
			MemoryUsageBytes: 2048,
		},
	}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	result, err := tools.GetStateSize(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "entry_count")
	assert.Contains(t, resultMap, "memory_bytes")

	// Verify entry_count
	entryCount, ok := resultMap["entry_count"].(int)
	assert.True(t, ok)
	assert.Equal(t, 100, entryCount)

	// Verify memory_bytes
	memoryBytes, ok := resultMap["memory_bytes"].(int)
	assert.True(t, ok)
	assert.Equal(t, 2048, memoryBytes)
}

func TestGetStateSize_FFIError(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	
	// Create mock FFI that returns error
	mockFFI := &MockFFI{
		shouldError: true,
	}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	result, err := tools.GetStateSize(nil)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get stats")
}

func TestGetApplyBlockLatency_Empty(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	result, err := tools.GetApplyBlockLatency(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "latencies_ms")
	assert.Contains(t, resultMap, "mean_ms")
	assert.Contains(t, resultMap, "stddev_ms")

	// Verify empty latencies
	latencies, ok := resultMap["latencies_ms"].([]float64)
	assert.True(t, ok)
	assert.Empty(t, latencies)

	// Verify zero statistics
	mean, ok := resultMap["mean_ms"].(float64)
	assert.True(t, ok)
	assert.Equal(t, 0.0, mean)

	stddev, ok := resultMap["stddev_ms"].(float64)
	assert.True(t, ok)
	assert.Equal(t, 0.0, stddev)
}

func TestGetApplyBlockLatency_WithData(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Record some latencies
	tracker := tools.GetApplyBlockLatencyTracker()
	for i := 1; i <= 10; i++ {
		tracker.Record(float64(i))
	}

	result, err := tools.GetApplyBlockLatency(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify latencies
	latencies, ok := resultMap["latencies_ms"].([]float64)
	assert.True(t, ok)
	assert.Equal(t, 10, len(latencies))

	// Verify statistics
	mean, ok := resultMap["mean_ms"].(float64)
	assert.True(t, ok)
	assert.InDelta(t, 5.5, mean, 0.01)

	stddev, ok := resultMap["stddev_ms"].(float64)
	assert.True(t, ok)
	assert.Greater(t, stddev, 0.0)
}

func TestGetApplyBlockLatency_WithLimit(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Record 100 latencies
	tracker := tools.GetApplyBlockLatencyTracker()
	for i := 1; i <= 100; i++ {
		tracker.Record(float64(i))
	}

	// Request only 50
	params := map[string]interface{}{
		"limit": float64(50),
	}

	result, err := tools.GetApplyBlockLatency(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify only 50 latencies returned
	latencies, ok := resultMap["latencies_ms"].([]float64)
	assert.True(t, ok)
	assert.Equal(t, 50, len(latencies))
}

func TestGetApplyBlockLatency_InvalidLimit(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Test with negative limit (should default to 100)
	params := map[string]interface{}{
		"limit": float64(-10),
	}

	result, err := tools.GetApplyBlockLatency(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should not error, just use default
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, resultMap, "latencies_ms")
}

func TestGetApplyBlockLatency_LimitTooLarge(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Test with limit > 1000 (should cap at 1000)
	params := map[string]interface{}{
		"limit": float64(2000),
	}

	result, err := tools.GetApplyBlockLatency(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	latencies, ok := resultMap["latencies_ms"].([]float64)
	assert.True(t, ok)
	// Should be capped at available data (0 in this case)
	assert.LessOrEqual(t, len(latencies), 1000)
}

func TestValidateDeterminism(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	result, err := tools.ValidateDeterminism(nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "consistent")
	assert.Contains(t, resultMap, "blocks_checked")
	assert.Contains(t, resultMap, "error")

	// Verify placeholder implementation returns not implemented error
	consistent, ok := resultMap["consistent"].(bool)
	assert.True(t, ok)
	assert.False(t, consistent)

	blocksChecked, ok := resultMap["blocks_checked"].(int)
	assert.True(t, ok)
	assert.Equal(t, 0, blocksChecked)

	errorMsg, ok := resultMap["error"].(string)
	assert.True(t, ok)
	assert.Contains(t, errorMsg, "not yet implemented")
}

func TestValidateDeterminism_WithBlockCount(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Test with custom block_count
	params := map[string]interface{}{
		"block_count": float64(20),
	}

	result, err := tools.ValidateDeterminism(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should not error, just return placeholder
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, resultMap, "error")
}

func TestRegisterRuntimeTools_WithRustCoreTools(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}

	tools := RegisterRuntimeTools(server, mockReorgEngine, mockFFI)

	assert.NotNil(t, tools)
	assert.NotNil(t, tools.latencyTracker)
	assert.NotNil(t, tools.reorgEngine)
	assert.NotNil(t, tools.ffi)
	assert.NotNil(t, tools.applyBlockLatencies)

	// Verify all tools are registered
	server.mu.RLock()
	defer server.mu.RUnlock()

	assert.Contains(t, server.handlers, "get_gc_stats")
	assert.Contains(t, server.handlers, "get_heap_usage")
	assert.Contains(t, server.handlers, "get_goroutine_count")
	assert.Contains(t, server.handlers, "get_latency_distribution")
	assert.Contains(t, server.handlers, "get_reorg_history")
	assert.Contains(t, server.handlers, "get_state_root")
	assert.Contains(t, server.handlers, "get_state_size")
	assert.Contains(t, server.handlers, "get_apply_block_latency")
	assert.Contains(t, server.handlers, "validate_determinism")
	assert.Contains(t, server.handlers, "compare_gc_vs_core_latency")
	assert.Contains(t, server.handlers, "run_load_test")
}

func TestCompareGCVsCoreLatency(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Record some Go latencies
	tools.latencyTracker.Record(2.0)
	tools.latencyTracker.Record(3.0)
	tools.latencyTracker.Record(5.0)
	tools.latencyTracker.Record(8.0)
	tools.latencyTracker.Record(10.0)

	// Record some Rust latencies
	tools.applyBlockLatencies.Record(1.0)
	tools.applyBlockLatencies.Record(1.2)
	tools.applyBlockLatencies.Record(1.5)
	tools.applyBlockLatencies.Record(1.8)
	tools.applyBlockLatencies.Record(2.0)

	result, err := tools.CompareGCVsCoreLatency(map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "go_latency_variance_ms")
	assert.Contains(t, resultMap, "rust_latency_variance_ms")
	assert.Contains(t, resultMap, "variance_ratio")

	// Verify values are numbers
	goVariance, ok := resultMap["go_latency_variance_ms"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, goVariance, 0.0)

	rustVariance, ok := resultMap["rust_latency_variance_ms"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, rustVariance, 0.0)

	varianceRatio, ok := resultMap["variance_ratio"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, varianceRatio, 0.0)
}

func TestCompareGCVsCoreLatency_NoData(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// No latencies recorded
	result, err := tools.CompareGCVsCoreLatency(map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Should return zero values when no data
	assert.Contains(t, resultMap, "go_latency_variance_ms")
	assert.Contains(t, resultMap, "rust_latency_variance_ms")
	assert.Contains(t, resultMap, "variance_ratio")
}

func TestCompareGCVsCoreLatency_ZeroRustVariance(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Record Go latencies with variance
	tools.latencyTracker.Record(2.0)
	tools.latencyTracker.Record(10.0)

	// Record Rust latencies with no variance (all same value)
	tools.applyBlockLatencies.Record(1.0)
	tools.applyBlockLatencies.Record(1.0)
	tools.applyBlockLatencies.Record(1.0)

	result, err := tools.CompareGCVsCoreLatency(map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Variance ratio should be 0 when Rust variance is 0 (to avoid division by zero)
	varianceRatio, ok := resultMap["variance_ratio"].(float64)
	require.True(t, ok)
	assert.Equal(t, 0.0, varianceRatio)
}

func TestRunLoadTest_ValidParams(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	mockLoadTester := &MockLoadTester{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)
	tools.loadTester = mockLoadTester

	params := map[string]interface{}{
		"tps":      float64(1000),
		"duration": float64(10),
	}

	result, err := tools.RunLoadTest(params)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Verify all required fields are present
	assert.Contains(t, resultMap, "success")
	assert.Contains(t, resultMap, "message")

	success, ok := resultMap["success"].(bool)
	assert.True(t, ok)
	assert.True(t, success)
}

func TestRunLoadTest_InvalidTPS(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Test with TPS < 1
	params := map[string]interface{}{
		"tps":      float64(0),
		"duration": float64(10),
	}

	result, err := tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tps must be between 1 and 10000")

	// Test with TPS > 10000
	params = map[string]interface{}{
		"tps":      float64(20000),
		"duration": float64(10),
	}

	result, err = tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tps must be between 1 and 10000")
}

func TestRunLoadTest_InvalidDuration(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Test with duration < 1
	params := map[string]interface{}{
		"tps":      float64(1000),
		"duration": float64(0),
	}

	result, err := tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "duration must be between 1 and 300")

	// Test with duration > 300
	params = map[string]interface{}{
		"tps":      float64(1000),
		"duration": float64(500),
	}

	result, err = tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "duration must be between 1 and 300")
}

func TestRunLoadTest_MissingParams(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)

	// Test with missing tps
	params := map[string]interface{}{
		"duration": float64(10),
	}

	result, err := tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tps parameter is required")

	// Test with missing duration
	params = map[string]interface{}{
		"tps": float64(1000),
	}

	result, err = tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "duration parameter is required")
}

func TestRunLoadTest_NoLoadTester(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)
	// Don't set loadTester

	params := map[string]interface{}{
		"tps":      float64(1000),
		"duration": float64(10),
	}

	result, err := tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "load tester not configured")
}

func TestRunLoadTest_LoadTesterError(t *testing.T) {
	logger := zap.NewNop()
	mockReorgEngine := reorg.NewReorgEngine(logger, nil)
	mockFFI := &MockFFI{}
	mockLoadTester := &MockLoadTester{shouldError: true}
	
	tools := NewRuntimeTools(mockReorgEngine, mockFFI)
	tools.loadTester = mockLoadTester

	params := map[string]interface{}{
		"tps":      float64(1000),
		"duration": float64(10),
	}

	result, err := tools.RunLoadTest(params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "load test failed")
}

// Mock FFI for testing

type MockFFI struct {
	stateRoot   [32]byte
	stats       *ffi.Stats
	shouldError bool
}

func (m *MockFFI) GetStateRoot() ([32]byte, error) {
	if m.shouldError {
		return [32]byte{}, fmt.Errorf("mock FFI error")
	}
	return m.stateRoot, nil
}

func (m *MockFFI) GetStats() (*ffi.Stats, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock FFI error")
	}
	return m.stats, nil
}

// Mock LoadTester for testing

type MockLoadTester struct {
	shouldError bool
}

func (m *MockLoadTester) Run(tps int, duration time.Duration) error {
	if m.shouldError {
		return fmt.Errorf("mock load tester error")
	}
	return nil
}
