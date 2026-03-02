package loadtest

import (
	"runtime"
	"sync"
	"time"
)

// LoadTestMetrics collects comprehensive metrics during load testing
type LoadTestMetrics struct {
	mu sync.RWMutex

	// Latency metrics
	latencies []float64

	// GC metrics
	gcPauseStartNs   uint64
	gcPauseEndNs     uint64
	gcCountStart     uint32
	gcCountEnd       uint32

	// Memory metrics
	memoryStartMB float64
	memoryEndMB   float64
	memorySnapshots []MemorySnapshot

	// Reorg metrics
	reorgRollbackCosts []ReorgCost

	// Timing
	startTime time.Time
	endTime   time.Time
}

// MemorySnapshot captures memory usage at a point in time
type MemorySnapshot struct {
	Timestamp time.Time
	AllocMB   float64
}

// ReorgCost captures the cost of a reorg operation
type ReorgCost struct {
	Depth              int
	RollbackDurationMs float64
	ReplayDurationMs   float64
}

// NewLoadTestMetrics creates a new metrics collector
func NewLoadTestMetrics() *LoadTestMetrics {
	return &LoadTestMetrics{
		latencies:          make([]float64, 0, 10000),
		memorySnapshots:    make([]MemorySnapshot, 0, 100),
		reorgRollbackCosts: make([]ReorgCost, 0, 10),
	}
}

// Start begins metrics collection
func (m *LoadTestMetrics) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.startTime = time.Now()

	// Capture initial GC stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.gcPauseStartNs = memStats.PauseTotalNs
	m.gcCountStart = memStats.NumGC
	m.memoryStartMB = float64(memStats.Alloc) / 1024 / 1024

	// Take initial memory snapshot
	m.memorySnapshots = append(m.memorySnapshots, MemorySnapshot{
		Timestamp: m.startTime,
		AllocMB:   m.memoryStartMB,
	})
}

// Stop ends metrics collection
func (m *LoadTestMetrics) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.endTime = time.Now()

	// Capture final GC stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.gcPauseEndNs = memStats.PauseTotalNs
	m.gcCountEnd = memStats.NumGC
	m.memoryEndMB = float64(memStats.Alloc) / 1024 / 1024

	// Take final memory snapshot
	m.memorySnapshots = append(m.memorySnapshots, MemorySnapshot{
		Timestamp: m.endTime,
		AllocMB:   m.memoryEndMB,
	})
}

// RecordLatency records a block processing latency
func (m *LoadTestMetrics) RecordLatency(latencyMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencies = append(m.latencies, latencyMs)
}

// RecordMemorySnapshot records current memory usage
func (m *LoadTestMetrics) RecordMemorySnapshot() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	m.memorySnapshots = append(m.memorySnapshots, MemorySnapshot{
		Timestamp: time.Now(),
		AllocMB:   float64(memStats.Alloc) / 1024 / 1024,
	})
}

// RecordReorgCost records the cost of a reorg operation
func (m *LoadTestMetrics) RecordReorgCost(depth int, rollbackMs, replayMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reorgRollbackCosts = append(m.reorgRollbackCosts, ReorgCost{
		Depth:              depth,
		RollbackDurationMs: rollbackMs,
		ReplayDurationMs:   replayMs,
	})
}

// GetGCPauseTimeMs returns total GC pause time during the test in milliseconds
func (m *LoadTestMetrics) GetGCPauseTimeMs() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pauseNs := m.gcPauseEndNs - m.gcPauseStartNs
	return float64(pauseNs) / 1e6
}

// GetGCCount returns the number of GC cycles during the test
func (m *LoadTestMetrics) GetGCCount() uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.gcCountEnd - m.gcCountStart
}

// GetMaxGCPauseMs returns the maximum GC pause during the test
func (m *LoadTestMetrics) GetMaxGCPauseMs() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Find max pause in the range of GCs during the test
	maxPause := uint64(0)
	gcCount := m.gcCountEnd - m.gcCountStart
	if gcCount > 256 {
		gcCount = 256 // Only last 256 pauses are available
	}

	for i := uint32(0); i < gcCount; i++ {
		idx := (memStats.NumGC - i + 255) % 256
		if memStats.PauseNs[idx] > maxPause {
			maxPause = memStats.PauseNs[idx]
		}
	}

	return float64(maxPause) / 1e6
}

// GetMemoryGrowthRateMBPerSec returns the memory growth rate in MB/sec
func (m *LoadTestMetrics) GetMemoryGrowthRateMBPerSec() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.endTime.IsZero() || m.startTime.IsZero() {
		return 0
	}

	durationSec := m.endTime.Sub(m.startTime).Seconds()
	if durationSec == 0 {
		return 0
	}

	memoryGrowthMB := m.memoryEndMB - m.memoryStartMB
	return memoryGrowthMB / durationSec
}

// GetMemorySnapshots returns all memory snapshots
func (m *LoadTestMetrics) GetMemorySnapshots() []MemorySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]MemorySnapshot, len(m.memorySnapshots))
	copy(result, m.memorySnapshots)
	return result
}

// GetReorgRollbackCost returns the average rollback cost for a given depth
func (m *LoadTestMetrics) GetReorgRollbackCost(depth int) (rollbackMs, replayMs float64, found bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	totalRollback := 0.0
	totalReplay := 0.0

	for _, cost := range m.reorgRollbackCosts {
		if cost.Depth == depth {
			totalRollback += cost.RollbackDurationMs
			totalReplay += cost.ReplayDurationMs
			count++
		}
	}

	if count == 0 {
		return 0, 0, false
	}

	return totalRollback / float64(count), totalReplay / float64(count), true
}

// GetLatencies returns all recorded latencies
func (m *LoadTestMetrics) GetLatencies() []float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]float64, len(m.latencies))
	copy(result, m.latencies)
	return result
}
