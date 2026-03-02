package loadtest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockWorkerPool implements WorkerPoolInterface for testing
type MockWorkerPool struct {
	mu            sync.Mutex
	blocks        []*ffi.Block
	submitLatency time.Duration
	shouldError   bool
}

func NewMockWorkerPool() *MockWorkerPool {
	return &MockWorkerPool{
		blocks:        make([]*ffi.Block, 0),
		submitLatency: 1 * time.Millisecond,
	}
}

func (m *MockWorkerPool) Submit(block *ffi.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return assert.AnError
	}

	// Simulate processing latency
	time.Sleep(m.submitLatency)

	m.blocks = append(m.blocks, block)
	return nil
}

func (m *MockWorkerPool) GetBlockCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.blocks)
}

func TestNewLoadTester(t *testing.T) {
	logger := zap.NewNop()
	mockPool := NewMockWorkerPool()
	seed := int64(42)

	tester := NewLoadTester(logger, mockPool, seed)

	assert.NotNil(t, tester)
	assert.NotNil(t, tester.generator)
	assert.NotNil(t, tester.workerPool)
	assert.NotNil(t, tester.latencies)
}

func TestRunLoadTest_ShortDuration(t *testing.T) {
	logger := zap.NewNop()
	mockPool := NewMockWorkerPool()
	tester := NewLoadTester(logger, mockPool, 42)

	config := LoadTestConfig{
		TPS:                  100,  // 100 TPS
		DurationSeconds:      1,    // 1 second
		TransactionsPerBlock: 10,   // 10 tx per block
	}

	ctx := context.Background()
	result, err := tester.RunLoadTest(ctx, config)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have processed approximately 10 blocks (100 TPS / 10 tx per block = 10 blocks/sec)
	assert.Greater(t, result.BlocksProcessed, 5, "Should process at least 5 blocks")
	assert.Less(t, result.BlocksProcessed, 15, "Should process at most 15 blocks")

	// Should have no errors
	assert.Equal(t, 0, result.ErrorCount)

	// Latencies should be recorded
	assert.Greater(t, result.P50LatencyMs, 0.0)
	assert.Greater(t, result.P99LatencyMs, 0.0)

	// Throughput should be close to target
	assert.Greater(t, result.ThroughputTPS, 50.0)
	assert.Less(t, result.ThroughputTPS, 150.0)
}

func TestRunLoadTest_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	mockPool := NewMockWorkerPool()
	tester := NewLoadTester(logger, mockPool, 42)

	config := LoadTestConfig{
		TPS:                  100,
		DurationSeconds:      10, // Long duration
		TransactionsPerBlock: 10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	// Cancel after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := tester.RunLoadTest(ctx, config)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, context.Canceled, err)
}

func TestRunLoadTest_WithErrors(t *testing.T) {
	logger := zap.NewNop()
	mockPool := NewMockWorkerPool()
	mockPool.shouldError = true
	tester := NewLoadTester(logger, mockPool, 42)

	config := LoadTestConfig{
		TPS:                  100,
		DurationSeconds:      1,
		TransactionsPerBlock: 10,
	}

	ctx := context.Background()
	result, err := tester.RunLoadTest(ctx, config)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have errors
	assert.Greater(t, result.ErrorCount, 0)

	// Should have processed 0 blocks successfully
	assert.Equal(t, 0, result.BlocksProcessed)
}

func TestRunLoadTest_DifferentTPS(t *testing.T) {
	testCases := []struct {
		name string
		tps  int
	}{
		{"1000 TPS", 1000},
		{"5000 TPS", 5000},
		{"10000 TPS", 10000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := zap.NewNop()
			mockPool := NewMockWorkerPool()
			mockPool.submitLatency = 0 // No artificial latency for high TPS
			tester := NewLoadTester(logger, mockPool, 42)

			config := LoadTestConfig{
				TPS:                  tc.tps,
				DurationSeconds:      1,
				TransactionsPerBlock: 100, // 100 tx per block
			}

			ctx := context.Background()
			result, err := tester.RunLoadTest(ctx, config)

			require.NoError(t, err)
			require.NotNil(t, result)

			// Should have processed blocks
			assert.Greater(t, result.BlocksProcessed, 0)

			// Should have no errors
			assert.Equal(t, 0, result.ErrorCount)
		})
	}
}

func TestRunLoadTest_ZeroLatencies(t *testing.T) {
	logger := zap.NewNop()
	mockPool := NewMockWorkerPool()
	mockPool.shouldError = true // All submissions fail
	tester := NewLoadTester(logger, mockPool, 42)

	config := LoadTestConfig{
		TPS:                  100,
		DurationSeconds:      1,
		TransactionsPerBlock: 10,
	}

	ctx := context.Background()
	result, err := tester.RunLoadTest(ctx, config)

	require.NoError(t, err)
	require.NotNil(t, result)

	// With no successful submissions, latencies should be zero
	assert.Equal(t, 0.0, result.P50LatencyMs)
	assert.Equal(t, 0.0, result.P99LatencyMs)
	assert.Equal(t, 0.0, result.ThroughputTPS)
}

func TestGetLatencies(t *testing.T) {
	logger := zap.NewNop()
	mockPool := NewMockWorkerPool()
	tester := NewLoadTester(logger, mockPool, 42)

	config := LoadTestConfig{
		TPS:                  100,
		DurationSeconds:      1,
		TransactionsPerBlock: 10,
	}

	ctx := context.Background()
	_, err := tester.RunLoadTest(ctx, config)
	require.NoError(t, err)

	latencies := tester.GetLatencies()

	// Should have recorded latencies
	assert.Greater(t, len(latencies), 0)

	// All latencies should be non-negative
	for _, lat := range latencies {
		assert.GreaterOrEqual(t, lat, 0.0)
	}
}

func TestLoadTestConfig_Validation(t *testing.T) {
	// Test that various configurations work
	configs := []LoadTestConfig{
		{TPS: 1000, DurationSeconds: 1, TransactionsPerBlock: 10},
		{TPS: 5000, DurationSeconds: 5, TransactionsPerBlock: 50},
		{TPS: 10000, DurationSeconds: 10, TransactionsPerBlock: 100},
	}

	for _, config := range configs {
		logger := zap.NewNop()
		mockPool := NewMockWorkerPool()
		mockPool.submitLatency = 0
		tester := NewLoadTester(logger, mockPool, 42)

		ctx := context.Background()
		result, err := tester.RunLoadTest(ctx, config)

		assert.NoError(t, err)
		assert.NotNil(t, result)
	}
}

func BenchmarkRunLoadTest_1000TPS(b *testing.B) {
	logger := zap.NewNop()
	mockPool := NewMockWorkerPool()
	mockPool.submitLatency = 0
	tester := NewLoadTester(logger, mockPool, 42)

	config := LoadTestConfig{
		TPS:                  1000,
		DurationSeconds:      1,
		TransactionsPerBlock: 10,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tester.RunLoadTest(ctx, config)
	}
}
