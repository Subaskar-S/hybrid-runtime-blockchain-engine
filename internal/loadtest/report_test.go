package loadtest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportBenchmarkReport(t *testing.T) {
	// Create test data
	result := &LoadTestResult{
		P50LatencyMs:         2.5,
		P99LatencyMs:         8.3,
		ThroughputTPS:        4950.0,
		ErrorCount:           0,
		BlocksProcessed:      100,
		GCPauseTimeMs:        150.0,
		GCCount:              45,
		MaxGCPauseMs:         12.0,
		MemoryStartMB:        50.0,
		MemoryEndMB:          120.0,
		MemoryGrowthRateMBPS: 1.17,
	}

	metrics := NewLoadTestMetrics()
	metrics.Start()
	time.Sleep(10 * time.Millisecond)
	metrics.RecordMemorySnapshot()
	time.Sleep(10 * time.Millisecond)
	metrics.Stop()
	metrics.RecordReorgCost(10, 15.0, 25.0)

	config := LoadTestConfig{
		TPS:                  5000,
		DurationSeconds:      60,
		TransactionsPerBlock: 100,
	}

	// Create temp file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "benchmark_report.json")

	// Export report
	err := ExportBenchmarkReport(result, metrics, config, 8, 1.2, 0.3, filename)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filename)
	require.NoError(t, err)

	// Load and verify report
	report, err := LoadBenchmarkReport(filename)
	require.NoError(t, err)
	require.NotNil(t, report)

	// Verify config
	assert.Equal(t, 5000, report.Config.TPS)
	assert.Equal(t, 60, report.Config.DurationSeconds)
	assert.Equal(t, 8, report.Config.WorkerCount)
	assert.Equal(t, 100, report.Config.TransactionsPerBlock)

	// Verify results
	assert.Equal(t, 2.5, report.Results.P50LatencyMs)
	assert.Equal(t, 8.3, report.Results.P99LatencyMs)
	assert.Equal(t, 4950.0, report.Results.ThroughputTPS)
	assert.Equal(t, 0, report.Results.ErrorCount)

	// Verify GC metrics
	assert.Equal(t, 150.0, report.GCMetrics.TotalPauseMs)
	assert.Equal(t, 12.0, report.GCMetrics.MaxPauseMs)
	assert.Equal(t, uint32(45), report.GCMetrics.NumGC)

	// Verify Rust metrics
	assert.Equal(t, 1.2, report.RustMetrics.MeanLatencyMs)
	assert.Equal(t, 0.3, report.RustMetrics.LatencyVarianceMs)

	// Verify memory metrics
	assert.Equal(t, 50.0, report.Memory.StartMB)
	assert.Equal(t, 120.0, report.Memory.EndMB)
	assert.Equal(t, 1.17, report.Memory.GrowthRateMBPerSec)
	assert.NotEmpty(t, report.Memory.Snapshots)

	// Verify reorg cost
	assert.Equal(t, 15.0, report.ReorgCost.Rollback10BlocksMs)
	assert.Equal(t, 25.0, report.ReorgCost.Replay10BlocksMs)

	// Verify timestamp
	assert.NotEmpty(t, report.Timestamp)
	_, err = time.Parse(time.RFC3339, report.Timestamp)
	assert.NoError(t, err)
}

func TestExportBenchmarkReport_NoReorgCost(t *testing.T) {
	result := &LoadTestResult{
		P50LatencyMs:  2.5,
		P99LatencyMs:  8.3,
		ThroughputTPS: 4950.0,
	}

	metrics := NewLoadTestMetrics()
	metrics.Start()
	metrics.Stop()

	config := LoadTestConfig{
		TPS:                  1000,
		DurationSeconds:      10,
		TransactionsPerBlock: 10,
	}

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "report_no_reorg.json")

	err := ExportBenchmarkReport(result, metrics, config, 4, 0.0, 0.0, filename)
	require.NoError(t, err)

	report, err := LoadBenchmarkReport(filename)
	require.NoError(t, err)

	// Reorg costs should be zero when not recorded
	assert.Equal(t, 0.0, report.ReorgCost.Rollback10BlocksMs)
	assert.Equal(t, 0.0, report.ReorgCost.Replay10BlocksMs)
}

func TestLoadBenchmarkReport_InvalidFile(t *testing.T) {
	report, err := LoadBenchmarkReport("nonexistent_file.json")
	assert.Error(t, err)
	assert.Nil(t, report)
}

func TestLoadBenchmarkReport_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	err := os.WriteFile(filename, []byte("not valid json"), 0644)
	require.NoError(t, err)

	report, err := LoadBenchmarkReport(filename)
	assert.Error(t, err)
	assert.Nil(t, report)
}

func TestConvertMemorySnapshots(t *testing.T) {
	startTime := time.Now()
	snapshots := []MemorySnapshot{
		{Timestamp: startTime, AllocMB: 50.0},
		{Timestamp: startTime.Add(1 * time.Second), AllocMB: 60.0},
		{Timestamp: startTime.Add(2 * time.Second), AllocMB: 70.0},
	}

	dataPoints := convertMemorySnapshots(snapshots, startTime)

	require.Len(t, dataPoints, 3)
	assert.Equal(t, 0.0, dataPoints[0].TimestampSec)
	assert.Equal(t, 50.0, dataPoints[0].AllocMB)
	assert.InDelta(t, 1.0, dataPoints[1].TimestampSec, 0.01)
	assert.Equal(t, 60.0, dataPoints[1].AllocMB)
	assert.InDelta(t, 2.0, dataPoints[2].TimestampSec, 0.01)
	assert.Equal(t, 70.0, dataPoints[2].AllocMB)
}

func TestBenchmarkReport_JSONRoundTrip(t *testing.T) {
	original := BenchmarkReport{
		Timestamp: "2024-01-15T10:30:00Z",
		Config: BenchmarkConfig{
			TPS:                  5000,
			DurationSeconds:      60,
			WorkerCount:          8,
			TransactionsPerBlock: 100,
		},
		Results: BenchmarkResults{
			P50LatencyMs:  2.5,
			P99LatencyMs:  8.3,
			ThroughputTPS: 4950.0,
			ErrorCount:    0,
		},
		GCMetrics: GCMetrics{
			TotalPauseMs: 150.0,
			MaxPauseMs:   12.0,
			NumGC:        45,
		},
		RustMetrics: RustMetrics{
			MeanLatencyMs:     1.2,
			LatencyVarianceMs: 0.3,
		},
		Memory: MemoryMetrics{
			StartMB:            50.0,
			EndMB:              120.0,
			GrowthRateMBPerSec: 1.17,
			Snapshots: []MemoryDataPoint{
				{TimestampSec: 0.0, AllocMB: 50.0},
				{TimestampSec: 30.0, AllocMB: 85.0},
				{TimestampSec: 60.0, AllocMB: 120.0},
			},
		},
		ReorgCost: ReorgCostMetrics{
			Rollback10BlocksMs: 15.0,
			Replay10BlocksMs:   25.0,
		},
	}

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "roundtrip.json")

	// Create metrics and result for export
	result := &LoadTestResult{
		P50LatencyMs:         original.Results.P50LatencyMs,
		P99LatencyMs:         original.Results.P99LatencyMs,
		ThroughputTPS:        original.Results.ThroughputTPS,
		ErrorCount:           original.Results.ErrorCount,
		GCPauseTimeMs:        original.GCMetrics.TotalPauseMs,
		GCCount:              original.GCMetrics.NumGC,
		MaxGCPauseMs:         original.GCMetrics.MaxPauseMs,
		MemoryStartMB:        original.Memory.StartMB,
		MemoryEndMB:          original.Memory.EndMB,
		MemoryGrowthRateMBPS: original.Memory.GrowthRateMBPerSec,
	}

	metrics := NewLoadTestMetrics()
	metrics.Start()
	metrics.Stop()
	metrics.RecordReorgCost(10, 15.0, 25.0)

	config := LoadTestConfig{
		TPS:                  original.Config.TPS,
		DurationSeconds:      original.Config.DurationSeconds,
		TransactionsPerBlock: original.Config.TransactionsPerBlock,
	}

	// Export
	err := ExportBenchmarkReport(
		result,
		metrics,
		config,
		original.Config.WorkerCount,
		original.RustMetrics.MeanLatencyMs,
		original.RustMetrics.LatencyVarianceMs,
		filename,
	)
	require.NoError(t, err)

	// Load
	loaded, err := LoadBenchmarkReport(filename)
	require.NoError(t, err)

	// Verify key fields match
	assert.Equal(t, original.Config.TPS, loaded.Config.TPS)
	assert.Equal(t, original.Results.P50LatencyMs, loaded.Results.P50LatencyMs)
	assert.Equal(t, original.GCMetrics.TotalPauseMs, loaded.GCMetrics.TotalPauseMs)
	assert.Equal(t, original.RustMetrics.MeanLatencyMs, loaded.RustMetrics.MeanLatencyMs)
	assert.Equal(t, original.Memory.StartMB, loaded.Memory.StartMB)
	assert.Equal(t, original.ReorgCost.Rollback10BlocksMs, loaded.ReorgCost.Rollback10BlocksMs)
}
