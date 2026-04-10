package loadtest

import (
	"context"
	"sort"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

// WorkerPoolInterface defines the interface for submitting blocks to the worker pool
type WorkerPoolInterface interface {
	Submit(block *ffi.Block) error
}

// LoadTester executes load tests with synthetic block generation
type LoadTester struct {
	logger     *zap.Logger
	generator  *BlockGenerator
	workerPool WorkerPoolInterface
	metrics    *LoadTestMetrics
	latencies  []float64 // exposed for testing
}

// NewLoadTester creates a new load tester
func NewLoadTester(logger *zap.Logger, workerPool WorkerPoolInterface, seed int64) *LoadTester {
	return &LoadTester{
		logger:     logger,
		generator:  NewBlockGenerator(seed),
		workerPool: workerPool,
		metrics:    NewLoadTestMetrics(),
		latencies:  []float64{},
	}
}

// LoadTestConfig configures a load test
type LoadTestConfig struct {
	TPS              int           // Target transactions per second
	DurationSeconds  int           // Test duration in seconds
	TransactionsPerBlock int       // Number of transactions per block
}

// LoadTestResult contains the results of a load test
type LoadTestResult struct {
	P50LatencyMs         float64
	P99LatencyMs         float64
	ThroughputTPS        float64
	ErrorCount           int
	BlocksProcessed      int
	GCPauseTimeMs        float64
	GCCount              uint32
	MaxGCPauseMs         float64
	MemoryStartMB        float64
	MemoryEndMB          float64
	MemoryGrowthRateMBPS float64
}

// RunLoadTest executes a load test with the specified configuration
func (lt *LoadTester) RunLoadTest(ctx context.Context, config LoadTestConfig) (*LoadTestResult, error) {
	lt.logger.Info("starting load test",
		zap.Int("tps", config.TPS),
		zap.Int("duration_seconds", config.DurationSeconds),
		zap.Int("tx_per_block", config.TransactionsPerBlock))

	// Start metrics collection
	lt.metrics = NewLoadTestMetrics()
	lt.metrics.Start()

	// Start memory snapshot goroutine
	snapshotDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-snapshotDone:
				return
			case <-ticker.C:
				lt.metrics.RecordMemorySnapshot()
			}
		}
	}()

	// Calculate blocks per second based on TPS and transactions per block
	blocksPerSecond := float64(config.TPS) / float64(config.TransactionsPerBlock)
	intervalNs := int64(float64(time.Second) / blocksPerSecond)

	// Track errors and blocks
	errorCount := 0
	blocksProcessed := 0

	// Start time
	startTime := time.Now()
	endTime := startTime.Add(time.Duration(config.DurationSeconds) * time.Second)

	blockNumber := uint64(1)
	ticker := time.NewTicker(time.Duration(intervalNs))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(snapshotDone)
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(endTime) {
				goto done
			}

			// Generate and submit block
			block := lt.generator.GenerateBlock(blockNumber, config.TransactionsPerBlock)
			blockNumber++

			// Measure latency
			submitStart := time.Now()
			err := lt.workerPool.Submit(&block)
			submitLatency := time.Since(submitStart)

			if err != nil {
				errorCount++
				lt.logger.Warn("failed to submit block", zap.Error(err))
			} else {
				blocksProcessed++
				// Record latency
				lt.metrics.RecordLatency(float64(submitLatency.Milliseconds()))
			}
		}
	}

done:
	// Stop memory snapshots
	close(snapshotDone)

	// Stop metrics collection
	lt.metrics.Stop()

	// Calculate results
	duration := time.Since(startTime)
	
	latencies := lt.metrics.GetLatencies()

	if len(latencies) == 0 {
		return &LoadTestResult{
			P50LatencyMs:         0,
			P99LatencyMs:         0,
			ThroughputTPS:        0,
			ErrorCount:           errorCount,
			BlocksProcessed:      blocksProcessed,
			GCPauseTimeMs:        lt.metrics.GetGCPauseTimeMs(),
			GCCount:              lt.metrics.GetGCCount(),
			MaxGCPauseMs:         lt.metrics.GetMaxGCPauseMs(),
			MemoryStartMB:        lt.metrics.memoryStartMB,
			MemoryEndMB:          lt.metrics.memoryEndMB,
			MemoryGrowthRateMBPS: lt.metrics.GetMemoryGrowthRateMBPerSec(),
		}, nil
	}

	// Sort latencies for percentile calculation
	sorted := make([]float64, len(latencies))
	copy(sorted, latencies)
	sort.Float64s(sorted)

	p50 := sorted[int(float64(len(sorted))*0.50)]
	p99 := sorted[int(float64(len(sorted))*0.99)]

	// Calculate actual throughput
	actualTPS := float64(blocksProcessed*config.TransactionsPerBlock) / duration.Seconds()

	result := &LoadTestResult{
		P50LatencyMs:         p50,
		P99LatencyMs:         p99,
		ThroughputTPS:        actualTPS,
		ErrorCount:           errorCount,
		BlocksProcessed:      blocksProcessed,
		GCPauseTimeMs:        lt.metrics.GetGCPauseTimeMs(),
		GCCount:              lt.metrics.GetGCCount(),
		MaxGCPauseMs:         lt.metrics.GetMaxGCPauseMs(),
		MemoryStartMB:        lt.metrics.memoryStartMB,
		MemoryEndMB:          lt.metrics.memoryEndMB,
		MemoryGrowthRateMBPS: lt.metrics.GetMemoryGrowthRateMBPerSec(),
	}

	lt.logger.Info("load test complete",
		zap.Float64("p50_latency_ms", result.P50LatencyMs),
		zap.Float64("p99_latency_ms", result.P99LatencyMs),
		zap.Float64("throughput_tps", result.ThroughputTPS),
		zap.Int("error_count", result.ErrorCount),
		zap.Int("blocks_processed", result.BlocksProcessed),
		zap.Float64("gc_pause_ms", result.GCPauseTimeMs),
		zap.Uint32("gc_count", result.GCCount),
		zap.Float64("memory_growth_mb_per_sec", result.MemoryGrowthRateMBPS))

	// Cache latencies for external access
	lt.latencies = lt.metrics.GetLatencies()

	return result, nil
}

// Run implements mcp.LoadTesterInterface — runs a load test at the given TPS
// for the given duration and returns any error.
func (lt *LoadTester) Run(tps int, duration time.Duration) error {
	durationSeconds := int(duration.Seconds())
	if durationSeconds < 1 {
		durationSeconds = 1
	}
	config := LoadTestConfig{
		TPS:                  tps,
		DurationSeconds:      durationSeconds,
		TransactionsPerBlock: 10,
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration+10*time.Second)
	defer cancel()
	_, err := lt.RunLoadTest(ctx, config)
	return err
}

// GetLatencies returns all recorded latencies from the most recent run
func (lt *LoadTester) GetLatencies() []float64 {
	return lt.metrics.GetLatencies()
}

// GetMetrics returns the metrics collector
func (lt *LoadTester) GetMetrics() *LoadTestMetrics {
	return lt.metrics
}
