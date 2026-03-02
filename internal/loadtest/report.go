package loadtest

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// BenchmarkReport contains comprehensive load test results for export
type BenchmarkReport struct {
	Timestamp string                 `json:"timestamp"`
	Config    BenchmarkConfig        `json:"config"`
	Results   BenchmarkResults       `json:"results"`
	GCMetrics GCMetrics              `json:"gc_metrics"`
	RustMetrics RustMetrics          `json:"rust_metrics"`
	Memory    MemoryMetrics          `json:"memory"`
	ReorgCost ReorgCostMetrics       `json:"reorg_cost"`
}

// BenchmarkConfig contains the test configuration
type BenchmarkConfig struct {
	TPS                  int `json:"tps"`
	DurationSeconds      int `json:"duration_seconds"`
	WorkerCount          int `json:"worker_count"`
	TransactionsPerBlock int `json:"transactions_per_block"`
}

// BenchmarkResults contains the test results
type BenchmarkResults struct {
	P50LatencyMs  float64 `json:"p50_latency_ms"`
	P99LatencyMs  float64 `json:"p99_latency_ms"`
	ThroughputTPS float64 `json:"throughput_tps"`
	ErrorCount    int     `json:"error_count"`
}

// GCMetrics contains garbage collection metrics
type GCMetrics struct {
	TotalPauseMs float64 `json:"total_pause_ms"`
	MaxPauseMs   float64 `json:"max_pause_ms"`
	NumGC        uint32  `json:"num_gc"`
}

// RustMetrics contains Rust core performance metrics
type RustMetrics struct {
	MeanLatencyMs     float64 `json:"mean_latency_ms"`
	LatencyVarianceMs float64 `json:"latency_variance_ms"`
}

// MemoryMetrics contains memory usage metrics
type MemoryMetrics struct {
	StartMB           float64            `json:"start_mb"`
	EndMB             float64            `json:"end_mb"`
	GrowthRateMBPerSec float64           `json:"growth_rate_mb_per_sec"`
	Snapshots         []MemoryDataPoint `json:"snapshots"`
}

// MemoryDataPoint represents a memory measurement at a point in time
type MemoryDataPoint struct {
	TimestampSec float64 `json:"timestamp_sec"`
	AllocMB      float64 `json:"alloc_mb"`
}

// ReorgCostMetrics contains reorg operation costs
type ReorgCostMetrics struct {
	Rollback10BlocksMs float64 `json:"rollback_10_blocks_ms"`
	Replay10BlocksMs   float64 `json:"replay_10_blocks_ms"`
}

// ExportBenchmarkReport exports a benchmark report to a JSON file
func ExportBenchmarkReport(
	result *LoadTestResult,
	metrics *LoadTestMetrics,
	config LoadTestConfig,
	workerCount int,
	rustMeanLatency float64,
	rustVariance float64,
	filename string,
) error {
	// Build report
	report := BenchmarkReport{
		Timestamp: time.Now().Format(time.RFC3339),
		Config: BenchmarkConfig{
			TPS:                  config.TPS,
			DurationSeconds:      config.DurationSeconds,
			WorkerCount:          workerCount,
			TransactionsPerBlock: config.TransactionsPerBlock,
		},
		Results: BenchmarkResults{
			P50LatencyMs:  result.P50LatencyMs,
			P99LatencyMs:  result.P99LatencyMs,
			ThroughputTPS: result.ThroughputTPS,
			ErrorCount:    result.ErrorCount,
		},
		GCMetrics: GCMetrics{
			TotalPauseMs: result.GCPauseTimeMs,
			MaxPauseMs:   result.MaxGCPauseMs,
			NumGC:        result.GCCount,
		},
		RustMetrics: RustMetrics{
			MeanLatencyMs:     rustMeanLatency,
			LatencyVarianceMs: rustVariance,
		},
		Memory: MemoryMetrics{
			StartMB:            result.MemoryStartMB,
			EndMB:              result.MemoryEndMB,
			GrowthRateMBPerSec: result.MemoryGrowthRateMBPS,
			Snapshots:          convertMemorySnapshots(metrics.GetMemorySnapshots(), metrics.startTime),
		},
		ReorgCost: ReorgCostMetrics{},
	}

	// Get reorg costs if available
	if rollback, replay, found := metrics.GetReorgRollbackCost(10); found {
		report.ReorgCost.Rollback10BlocksMs = rollback
		report.ReorgCost.Replay10BlocksMs = replay
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	return nil
}

// convertMemorySnapshots converts memory snapshots to data points
func convertMemorySnapshots(snapshots []MemorySnapshot, startTime time.Time) []MemoryDataPoint {
	dataPoints := make([]MemoryDataPoint, len(snapshots))
	for i, snapshot := range snapshots {
		dataPoints[i] = MemoryDataPoint{
			TimestampSec: snapshot.Timestamp.Sub(startTime).Seconds(),
			AllocMB:      snapshot.AllocMB,
		}
	}
	return dataPoints
}

// LoadBenchmarkReport loads a benchmark report from a JSON file
func LoadBenchmarkReport(filename string) (*BenchmarkReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	var report BenchmarkReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to unmarshal report: %w", err)
	}

	return &report, nil
}
