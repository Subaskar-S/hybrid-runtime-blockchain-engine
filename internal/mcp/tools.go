package mcp

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/hybrid-runtime-blockchain-engine/internal/loadtest"
	"github.com/hybrid-runtime-blockchain-engine/internal/reorg"
)

// LatencyTracker tracks block processing latencies in a circular buffer
type LatencyTracker struct {
	mu        sync.RWMutex
	latencies []float64
	index     int
	size      int
	capacity  int
}

// NewLatencyTracker creates a new latency tracker with specified capacity
func NewLatencyTracker(capacity int) *LatencyTracker {
	return &LatencyTracker{
		latencies: make([]float64, capacity),
		capacity:  capacity,
	}
}

// Record records a latency value in milliseconds
func (lt *LatencyTracker) Record(latencyMs float64) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.latencies[lt.index] = latencyMs
	lt.index = (lt.index + 1) % lt.capacity
	if lt.size < lt.capacity {
		lt.size++
	}
}

// GetDistribution calculates latency percentiles
func (lt *LatencyTracker) GetDistribution() map[string]float64 {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if lt.size == 0 {
		return map[string]float64{
			"p50_ms":  0,
			"p95_ms":  0,
			"p99_ms":  0,
			"max_ms":  0,
		}
	}

	// Copy latencies for sorting
	sorted := make([]float64, lt.size)
	for i := 0; i < lt.size; i++ {
		sorted[i] = lt.latencies[i]
	}
	sort.Float64s(sorted)

	// Calculate percentiles
	p50 := sorted[int(float64(lt.size)*0.50)]
	p95 := sorted[int(float64(lt.size)*0.95)]
	p99 := sorted[int(float64(lt.size)*0.99)]
	max := sorted[lt.size-1]

	return map[string]float64{
		"p50_ms": p50,
		"p95_ms": p95,
		"p99_ms": p99,
		"max_ms": max,
	}
}

// ApplyBlockLatencyTracker tracks apply_block execution times in a circular buffer
type ApplyBlockLatencyTracker struct {
	mu        sync.RWMutex
	latencies []float64
	index     int
	size      int
	capacity  int
}

// NewApplyBlockLatencyTracker creates a new apply_block latency tracker with specified capacity
func NewApplyBlockLatencyTracker(capacity int) *ApplyBlockLatencyTracker {
	return &ApplyBlockLatencyTracker{
		latencies: make([]float64, capacity),
		capacity:  capacity,
	}
}

// Record records a latency value in milliseconds
func (ablt *ApplyBlockLatencyTracker) Record(latencyMs float64) {
	ablt.mu.Lock()
	defer ablt.mu.Unlock()

	ablt.latencies[ablt.index] = latencyMs
	ablt.index = (ablt.index + 1) % ablt.capacity
	if ablt.size < ablt.capacity {
		ablt.size++
	}
}

// GetLatencies returns recent latencies up to the specified limit
func (ablt *ApplyBlockLatencyTracker) GetLatencies(limit int) []float64 {
	ablt.mu.RLock()
	defer ablt.mu.RUnlock()

	if ablt.size == 0 {
		return []float64{}
	}

	// Determine how many latencies to return
	count := ablt.size
	if limit > 0 && limit < count {
		count = limit
	}

	// Copy most recent latencies
	result := make([]float64, count)
	for i := 0; i < count; i++ {
		// Get most recent entries (working backwards from current index)
		idx := (ablt.index - 1 - i + ablt.capacity) % ablt.capacity
		if idx < 0 {
			idx += ablt.capacity
		}
		result[i] = ablt.latencies[idx]
	}

	return result
}

// GetStatistics calculates mean and standard deviation
func (ablt *ApplyBlockLatencyTracker) GetStatistics(limit int) (mean float64, stddev float64) {
	ablt.mu.RLock()
	defer ablt.mu.RUnlock()

	if ablt.size == 0 {
		return 0, 0
	}

	// Determine how many latencies to use
	count := ablt.size
	if limit > 0 && limit < count {
		count = limit
	}

	// Calculate mean
	sum := 0.0
	for i := 0; i < count; i++ {
		idx := (ablt.index - 1 - i + ablt.capacity) % ablt.capacity
		if idx < 0 {
			idx += ablt.capacity
		}
		sum += ablt.latencies[idx]
	}
	mean = sum / float64(count)

	// Calculate standard deviation
	varianceSum := 0.0
	for i := 0; i < count; i++ {
		idx := (ablt.index - 1 - i + ablt.capacity) % ablt.capacity
		if idx < 0 {
			idx += ablt.capacity
		}
		diff := ablt.latencies[idx] - mean
		varianceSum += diff * diff
	}
	variance := varianceSum / float64(count)
	stddev = math.Sqrt(variance)

	return mean, stddev
}

// FFIInterface defines the interface for FFI operations needed by MCP tools
type FFIInterface interface {
	GetStateRoot() ([32]byte, error)
	GetStats() (*ffi.Stats, error)
}

// LoadTesterInterface defines the interface for load testing
type LoadTesterInterface interface {
	RunLoadTest(ctx context.Context, config loadtest.LoadTestConfig) (*loadtest.LoadTestResult, error)
}

// RuntimeTools provides runtime introspection tool handlers
type RuntimeTools struct {
	latencyTracker      *LatencyTracker
	reorgEngine         *reorg.ReorgEngine
	ffi                 FFIInterface
	applyBlockLatencies *ApplyBlockLatencyTracker
	loadTester          LoadTesterInterface
}

// NewRuntimeTools creates a new runtime tools instance
func NewRuntimeTools(reorgEngine *reorg.ReorgEngine, ffiInstance FFIInterface) *RuntimeTools {
	return &RuntimeTools{
		latencyTracker:      NewLatencyTracker(1000),           // Track last 1000 latencies
		reorgEngine:         reorgEngine,
		ffi:                 ffiInstance,
		applyBlockLatencies: NewApplyBlockLatencyTracker(1000), // Track last 1000 apply_block calls
		loadTester:          nil,                               // Optional, set via SetLoadTester
	}
}

// SetLoadTester sets the load tester instance
func (rt *RuntimeTools) SetLoadTester(loadTester LoadTesterInterface) {
	rt.loadTester = loadTester
}

// GetApplyBlockLatencyTracker returns the apply_block latency tracker for recording latencies
func (rt *RuntimeTools) GetApplyBlockLatencyTracker() *ApplyBlockLatencyTracker {
	return rt.applyBlockLatencies
}

// GetLatencyTracker returns the latency tracker for recording latencies
func (rt *RuntimeTools) GetLatencyTracker() *LatencyTracker {
	return rt.latencyTracker
}

// GetGCStats returns Go garbage collection statistics
func (rt *RuntimeTools) GetGCStats(params map[string]interface{}) (interface{}, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Get pause history (last 256 GC pauses)
	pauseNs := make([]uint64, 0, 256)
	for i := 0; i < 256 && i < int(memStats.NumGC); i++ {
		pauseNs = append(pauseNs, memStats.PauseNs[(memStats.NumGC-uint32(i)+255)%256])
	}

	// Format last GC time
	lastGC := time.Unix(0, int64(memStats.LastGC))

	return map[string]interface{}{
		"num_gc":         memStats.NumGC,
		"pause_total_ns": memStats.PauseTotalNs,
		"pause_ns":       pauseNs,
		"last_gc":        lastGC.Format(time.RFC3339),
	}, nil
}

// GetHeapUsage returns current heap memory usage
func (rt *RuntimeTools) GetHeapUsage(params map[string]interface{}) (interface{}, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"alloc_bytes":       memStats.Alloc,
		"total_alloc_bytes": memStats.TotalAlloc,
		"sys_bytes":         memStats.Sys,
		"heap_objects":      memStats.HeapObjects,
	}, nil
}

// GetGoroutineCount returns the number of active goroutines
func (rt *RuntimeTools) GetGoroutineCount(params map[string]interface{}) (interface{}, error) {
	count := runtime.NumGoroutine()

	return map[string]interface{}{
		"count": count,
	}, nil
}

// GetLatencyDistribution returns block processing latency percentiles
func (rt *RuntimeTools) GetLatencyDistribution(params map[string]interface{}) (interface{}, error) {
	distribution := rt.latencyTracker.GetDistribution()
	return distribution, nil
}

// GetReorgHistory returns recent reorg events
func (rt *RuntimeTools) GetReorgHistory(params map[string]interface{}) (interface{}, error) {
	// Get limit parameter (default 10)
	limit := 10
	if limitVal, ok := params["limit"]; ok {
		if limitFloat, ok := limitVal.(float64); ok {
			limit = int(limitFloat)
		}
	}

	// Validate limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Get reorg history from engine
	events := rt.reorgEngine.GetReorgHistory(limit)

	// Format events for JSON response
	reorgs := make([]map[string]interface{}, 0, len(events))
	for _, event := range events {
		reorgs = append(reorgs, map[string]interface{}{
			"timestamp":            event.Timestamp.Format(time.RFC3339),
			"fork_point":           event.ForkPoint,
			"depth":                event.Depth,
			"rollback_duration_ms": event.RollbackDurationMs,
		})
	}

	return map[string]interface{}{
		"reorgs": reorgs,
	}, nil
}

// GetStateRoot returns the current state root hash
func (rt *RuntimeTools) GetStateRoot(params map[string]interface{}) (interface{}, error) {
	// Call FFI GetStateRoot
	stateRoot, err := rt.ffi.GetStateRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get state root: %w", err)
	}

	// Get stats to retrieve block number
	stats, err := rt.ffi.GetStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return map[string]interface{}{
		"state_root":   hex.EncodeToString(stateRoot[:]),
		"block_number": stats.BlockNumber,
	}, nil
}

// GetStateSize returns the number of state entries and memory usage
func (rt *RuntimeTools) GetStateSize(params map[string]interface{}) (interface{}, error) {
	// Call FFI GetStats to get state entry count and memory usage
	stats, err := rt.ffi.GetStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return map[string]interface{}{
		"entry_count":  stats.StateSize,
		"memory_bytes": stats.MemoryUsageBytes,
	}, nil
}

// GetApplyBlockLatency returns recent apply_block execution times
func (rt *RuntimeTools) GetApplyBlockLatency(params map[string]interface{}) (interface{}, error) {
	// Get limit parameter (default 100)
	limit := 100
	if limitVal, ok := params["limit"]; ok {
		if limitFloat, ok := limitVal.(float64); ok {
			limit = int(limitFloat)
		}
	}

	// Validate limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Get latencies
	latencies := rt.applyBlockLatencies.GetLatencies(limit)

	// Calculate statistics
	mean, stddev := rt.applyBlockLatencies.GetStatistics(limit)

	return map[string]interface{}{
		"latencies_ms": latencies,
		"mean_ms":      mean,
		"stddev_ms":    stddev,
	}, nil
}

// ValidateDeterminism reapplies recent blocks and verifies state root consistency
func (rt *RuntimeTools) ValidateDeterminism(params map[string]interface{}) (interface{}, error) {
	// Get block_count parameter (default 10)
	blockCount := 10
	if blockCountVal, ok := params["block_count"]; ok {
		if blockCountFloat, ok := blockCountVal.(float64); ok {
			blockCount = int(blockCountFloat)
		}
	}

	// Validate block_count
	if blockCount <= 0 {
		blockCount = 10
	}
	if blockCount > 100 {
		blockCount = 100
	}

	// This is a placeholder implementation since we need access to block history
	// which is not yet available in the current architecture.
	// When block history is implemented, this should:
	// 1. Save current state root
	// 2. Rollback N blocks
	// 3. Replay N blocks
	// 4. Compare final state root with saved state root

	return map[string]interface{}{
		"consistent":     false,
		"blocks_checked": 0,
		"error":          "validate_determinism not yet implemented: requires block history infrastructure",
	}, nil
}

// CompareGCVsCoreLatency returns latency variance comparison between GC and core
func (rt *RuntimeTools) CompareGCVsCoreLatency(params map[string]interface{}) (interface{}, error) {
	// Calculate Go latency variance from block processing metrics
	goLatencies := rt.latencyTracker.GetDistribution()
	
	// Calculate variance from percentiles (approximation)
	// Variance ≈ ((p99 - p50) / 2)^2 for a rough estimate
	goP50 := goLatencies["p50_ms"]
	goP99 := goLatencies["p99_ms"]
	goVariance := math.Pow((goP99-goP50)/2, 2)
	
	// Calculate Rust latency variance from apply_block metrics
	_, rustStddev := rt.applyBlockLatencies.GetStatistics(0) // Use all available data
	rustVariance := rustStddev * rustStddev
	
	// Compute variance ratio (Go variance / Rust variance)
	varianceRatio := 0.0
	if rustVariance > 0 {
		varianceRatio = goVariance / rustVariance
	}
	
	return map[string]interface{}{
		"go_latency_variance_ms":   goVariance,
		"rust_latency_variance_ms": rustVariance,
		"variance_ratio":           varianceRatio,
	}, nil
}

// RunLoadTest executes a load test with specified TPS and duration
func (rt *RuntimeTools) RunLoadTest(params map[string]interface{}) (interface{}, error) {
	// Check if load tester is available
	if rt.loadTester == nil {
		return nil, fmt.Errorf("load tester not available")
	}

	// Get TPS parameter (required)
	tpsFloat, ok := params["tps"].(float64)
	if !ok {
		return nil, fmt.Errorf("tps parameter is required and must be a number")
	}
	tps := int(tpsFloat)

	// Validate TPS range (1-10000)
	if tps < 1 || tps > 10000 {
		return nil, fmt.Errorf("tps must be between 1 and 10000")
	}

	// Get duration_seconds parameter (required)
	durationFloat, ok := params["duration_seconds"].(float64)
	if !ok {
		return nil, fmt.Errorf("duration_seconds parameter is required and must be a number")
	}
	durationSeconds := int(durationFloat)

	// Validate duration range (1-300)
	if durationSeconds < 1 || durationSeconds > 300 {
		return nil, fmt.Errorf("duration_seconds must be between 1 and 300")
	}

	// Create load test config
	config := loadtest.LoadTestConfig{
		TPS:                  tps,
		DurationSeconds:      durationSeconds,
		TransactionsPerBlock: 10, // Fixed at 10 transactions per block
	}

	// Run load test with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(durationSeconds+10)*time.Second)
	defer cancel()

	result, err := rt.loadTester.RunLoadTest(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("load test failed: %w", err)
	}

	// Return results
	return map[string]interface{}{
		"p50_latency_ms": result.P50LatencyMs,
		"p99_latency_ms": result.P99LatencyMs,
		"throughput_tps": result.ThroughputTPS,
		"error_count":    result.ErrorCount,
	}, nil
}

// RegisterRuntimeTools registers all runtime introspection tools with the MCP server
func RegisterRuntimeTools(server *Server, reorgEngine *reorg.ReorgEngine, ffiInstance FFIInterface) *RuntimeTools {
	tools := NewRuntimeTools(reorgEngine, ffiInstance)

	server.RegisterTool("get_gc_stats", tools.GetGCStats)
	server.RegisterTool("get_heap_usage", tools.GetHeapUsage)
	server.RegisterTool("get_goroutine_count", tools.GetGoroutineCount)
	server.RegisterTool("get_latency_distribution", tools.GetLatencyDistribution)
	server.RegisterTool("get_reorg_history", tools.GetReorgHistory)
	server.RegisterTool("get_state_root", tools.GetStateRoot)
	server.RegisterTool("get_state_size", tools.GetStateSize)
	server.RegisterTool("get_apply_block_latency", tools.GetApplyBlockLatency)
	server.RegisterTool("validate_determinism", tools.ValidateDeterminism)
	server.RegisterTool("compare_gc_vs_core_latency", tools.CompareGCVsCoreLatency)
	server.RegisterTool("run_load_test", tools.RunLoadTest)

	return tools
}
