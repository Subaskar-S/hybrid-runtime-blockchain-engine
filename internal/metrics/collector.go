package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Collector manages Prometheus metrics collection and exposition
type Collector struct {
	logger   *zap.Logger
	registry *prometheus.Registry
	server   *http.Server
	
	// Go Runtime Metrics
	gcPauseSeconds       prometheus.Histogram
	memoryAllocBytes     prometheus.Gauge
	goroutineCount       prometheus.Gauge
	
	// Worker Pool Metrics
	workerActiveCount    prometheus.Gauge
	workerQueueDepth     prometheus.Gauge
	workerUtilization    prometheus.Gauge
	workerPanicTotal     prometheus.Counter
	
	// Rust Core Metrics
	rustApplyBlockDuration prometheus.Histogram
	rustStateSizeEntries   prometheus.Gauge
	rustMemoryUsageBytes   prometheus.Gauge
	
	// Reorg Metrics
	reorgTotal             prometheus.Counter
	reorgDepthBlocks       prometheus.Histogram
	reorgRollbackDuration  prometheus.Histogram
	
	// Block Processing Metrics
	blocksProcessedTotal   prometheus.Counter
	blockProcessingDuration prometheus.Histogram
	
	// Component stats providers
	workerPool  WorkerPoolStats
	reorgEngine ReorgEngineStats
	rustCore    RustCoreStats
	
	// Health check providers
	blockStreamer BlockStreamerHealth
	workerPoolHealth WorkerPoolHealth
	
	// Collection control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewCollector creates a new metrics collector
func NewCollector(logger *zap.Logger) *Collector {
	registry := prometheus.NewRegistry()
	
	c := &Collector{
		logger:   logger,
		registry: registry,
		stopCh:   make(chan struct{}),
	}
	
	c.initMetrics()
	c.registerMetrics()
	
	return c
}

// initMetrics initializes all Prometheus metrics
func (c *Collector) initMetrics() {
	// Go Runtime Metrics
	c.gcPauseSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "gc_pause_seconds",
		Help:    "Go garbage collection pause duration in seconds",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15), // 0.1ms to ~3s
	})
	
	c.memoryAllocBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "memory_alloc_bytes",
		Help: "Current memory allocation in bytes",
	})
	
	c.goroutineCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goroutine_count",
		Help: "Number of active goroutines",
	})
	
	// Worker Pool Metrics
	c.workerActiveCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "worker_active_count",
		Help: "Number of currently active workers",
	})
	
	c.workerQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "worker_queue_depth",
		Help: "Number of blocks waiting in worker queue",
	})
	
	c.workerUtilization = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "worker_utilization_percent",
		Help: "Worker pool utilization percentage",
	})
	
	c.workerPanicTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "worker_panic_total",
		Help: "Total number of worker panics",
	})
	
	// Rust Core Metrics
	c.rustApplyBlockDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "rust_apply_block_duration_seconds",
		Help:    "Rust core apply_block execution duration in seconds",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15), // 0.1ms to ~3s
	})
	
	c.rustStateSizeEntries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rust_state_size_entries",
		Help: "Number of entries in Rust state",
	})
	
	c.rustMemoryUsageBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rust_memory_usage_bytes",
		Help: "Rust core memory usage in bytes",
	})
	
	// Reorg Metrics
	c.reorgTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "reorg_total",
		Help: "Total number of blockchain reorganizations",
	})
	
	c.reorgDepthBlocks = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "reorg_depth_blocks",
		Help:    "Blockchain reorganization depth in blocks",
		Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	})
	
	c.reorgRollbackDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "reorg_rollback_duration_seconds",
		Help:    "Blockchain reorganization rollback duration in seconds",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
	})
	
	// Block Processing Metrics
	c.blocksProcessedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "blocks_processed_total",
		Help: "Total number of blocks processed",
	})
	
	c.blockProcessingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "block_processing_duration_seconds",
		Help:    "Block processing duration in seconds",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15), // 0.1ms to ~3s
	})
}

// registerMetrics registers all metrics with the Prometheus registry
func (c *Collector) registerMetrics() {
	// Go Runtime Metrics
	c.registry.MustRegister(c.gcPauseSeconds)
	c.registry.MustRegister(c.memoryAllocBytes)
	c.registry.MustRegister(c.goroutineCount)
	
	// Worker Pool Metrics
	c.registry.MustRegister(c.workerActiveCount)
	c.registry.MustRegister(c.workerQueueDepth)
	c.registry.MustRegister(c.workerUtilization)
	c.registry.MustRegister(c.workerPanicTotal)
	
	// Rust Core Metrics
	c.registry.MustRegister(c.rustApplyBlockDuration)
	c.registry.MustRegister(c.rustStateSizeEntries)
	c.registry.MustRegister(c.rustMemoryUsageBytes)
	
	// Reorg Metrics
	c.registry.MustRegister(c.reorgTotal)
	c.registry.MustRegister(c.reorgDepthBlocks)
	c.registry.MustRegister(c.reorgRollbackDuration)
	
	// Block Processing Metrics
	c.registry.MustRegister(c.blocksProcessedTotal)
	c.registry.MustRegister(c.blockProcessingDuration)
}

// Start starts the metrics HTTP server and periodic collection
func (c *Collector) Start(ctx context.Context, port int) error {
	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", c.handleHealth)
	mux.HandleFunc("/debug/gc", c.handleDebugGC)
	mux.HandleFunc("/debug/state", c.handleDebugState)
	
	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	
	// Start HTTP server in goroutine
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.logger.Info("starting metrics server", zap.Int("port", port))
		
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("metrics server error", zap.Error(err))
		}
	}()
	
	// Start periodic collection
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.collectPeriodically(ctx)
	}()
	
	return nil
}

// Stop stops the metrics server and collection
func (c *Collector) Stop(ctx context.Context) error {
	c.logger.Info("stopping metrics collector")
	
	// Signal stop
	close(c.stopCh)
	
	// Shutdown HTTP server
	if c.server != nil {
		if err := c.server.Shutdown(ctx); err != nil {
			c.logger.Error("metrics server shutdown error", zap.Error(err))
			return err
		}
	}
	
	// Wait for goroutines
	c.wg.Wait()
	
	c.logger.Info("metrics collector stopped")
	return nil
}

// collectPeriodically collects runtime metrics periodically
func (c *Collector) collectPeriodically(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.collectRuntimeMetrics()
			c.collectComponentMetrics()
		}
	}
}

// collectRuntimeMetrics collects Go runtime metrics
func (c *Collector) collectRuntimeMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Update memory metrics
	c.memoryAllocBytes.Set(float64(m.Alloc))
	
	// Update goroutine count
	c.goroutineCount.Set(float64(runtime.NumGoroutine()))
	
	// Update GC pause metrics
	// Get the most recent GC pause
	if m.NumGC > 0 {
		// PauseNs is a circular buffer, get the most recent pause
		idx := (m.NumGC + 255) % 256
		pauseNs := m.PauseNs[idx]
		c.gcPauseSeconds.Observe(float64(pauseNs) / 1e9)
	}
}

// collectComponentMetrics collects metrics from registered components
func (c *Collector) collectComponentMetrics() {
	// Collect worker pool stats
	if c.workerPool != nil {
		stats := c.workerPool.GetStats()
		c.workerActiveCount.Set(float64(stats.ActiveWorkers))
		c.workerQueueDepth.Set(float64(stats.QueueDepth))
		
		// Calculate utilization percentage
		if stats.NumWorkers > 0 {
			utilization := float64(stats.ActiveWorkers) / float64(stats.NumWorkers) * 100
			c.workerUtilization.Set(utilization)
		}
	}
	
	// Collect Rust core stats
	if c.rustCore != nil {
		stats, err := c.rustCore.GetStats()
		if err != nil {
			c.logger.Error("failed to collect Rust stats", zap.Error(err))
		} else {
			c.rustStateSizeEntries.Set(float64(stats.StateSize))
			c.rustMemoryUsageBytes.Set(float64(stats.MemoryUsageBytes))
		}
	}
	
	// Reorg stats are updated via RecordReorg, not polled
}

// RecordBlockProcessed records a processed block
func (c *Collector) RecordBlockProcessed(duration time.Duration) {
	c.blocksProcessedTotal.Inc()
	c.blockProcessingDuration.Observe(duration.Seconds())
}

// RecordRustApplyBlock records a Rust apply_block call
func (c *Collector) RecordRustApplyBlock(duration time.Duration) {
	c.rustApplyBlockDuration.Observe(duration.Seconds())
}

// RecordReorg records a blockchain reorganization
func (c *Collector) RecordReorg(depth int, rollbackDuration time.Duration) {
	c.reorgTotal.Inc()
	c.reorgDepthBlocks.Observe(float64(depth))
	c.reorgRollbackDuration.Observe(rollbackDuration.Seconds())
}

// RecordWorkerPanic records a worker panic
func (c *Collector) RecordWorkerPanic() {
	c.workerPanicTotal.Inc()
}

// UpdateWorkerStats updates worker pool statistics
func (c *Collector) UpdateWorkerStats(activeWorkers, queueDepth, totalWorkers int) {
	c.workerActiveCount.Set(float64(activeWorkers))
	c.workerQueueDepth.Set(float64(queueDepth))
	
	// Calculate utilization percentage
	if totalWorkers > 0 {
		utilization := float64(activeWorkers) / float64(totalWorkers) * 100
		c.workerUtilization.Set(utilization)
	}
}

// UpdateRustStats updates Rust core statistics
func (c *Collector) UpdateRustStats(stateSize, memoryUsage int) {
	c.rustStateSizeEntries.Set(float64(stateSize))
	c.rustMemoryUsageBytes.Set(float64(memoryUsage))
}

// RegisterWorkerPool registers a worker pool for periodic stats collection
func (c *Collector) RegisterWorkerPool(wp WorkerPoolStats) {
	c.workerPool = wp
}

// RegisterReorgEngine registers a reorg engine for stats collection
func (c *Collector) RegisterReorgEngine(re ReorgEngineStats) {
	c.reorgEngine = re
}

// RegisterRustCore registers a Rust core for periodic stats collection
func (c *Collector) RegisterRustCore(rc RustCoreStats) {
	c.rustCore = rc
}

// RegisterBlockStreamer registers a block streamer for health checks
func (c *Collector) RegisterBlockStreamer(bs BlockStreamerHealth) {
	c.blockStreamer = bs
}

// RegisterWorkerPoolHealth registers a worker pool for health checks
func (c *Collector) RegisterWorkerPoolHealth(wp WorkerPoolHealth) {
	c.workerPoolHealth = wp
}

// handleHealth handles the /health endpoint
func (c *Collector) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check if Block_Streamer is connected
	if c.blockStreamer == nil || !c.blockStreamer.IsConnected() {
		c.logger.Debug("health check failed: block streamer not connected")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Block streamer not connected\n"))
		return
	}
	
	// Check if workers are active
	if c.workerPoolHealth == nil || c.workerPoolHealth.ActiveWorkers() == 0 {
		c.logger.Debug("health check failed: no active workers")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("No active workers\n"))
		return
	}
	
	// System is healthy
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

// handleDebugGC handles the /debug/gc endpoint
func (c *Collector) handleDebugGC(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Collect recent GC pause times (last 256 pauses)
	pauseNs := make([]uint64, 0, 256)
	if m.NumGC > 0 {
		// Get up to the last 256 GC pauses
		numPauses := m.NumGC
		if numPauses > 256 {
			numPauses = 256
		}
		
		for i := uint32(0); i < numPauses; i++ {
			idx := (m.NumGC - 1 - i + 256) % 256
			pauseNs = append(pauseNs, m.PauseNs[idx])
		}
	}
	
	// Format last GC time
	lastGC := time.Unix(0, int64(m.LastGC)).Format(time.RFC3339)
	if m.LastGC == 0 {
		lastGC = "never"
	}
	
	// Build JSON response
	response := map[string]interface{}{
		"num_gc":        m.NumGC,
		"pause_total_ns": m.PauseTotalNs,
		"pause_ns":      pauseNs,
		"last_gc":       lastGC,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.logger.Error("failed to encode GC stats", zap.Error(err))
	}
}

// handleDebugState handles the /debug/state endpoint
func (c *Collector) handleDebugState(w http.ResponseWriter, r *http.Request) {
	if c.rustCore == nil {
		c.logger.Error("rust core not registered")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Rust core not available\n"))
		return
	}
	
	// Get state root
	stateRoot, err := c.rustCore.GetStateRoot()
	if err != nil {
		c.logger.Error("failed to get state root", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Failed to get state root: %v\n", err)))
		return
	}
	
	// Get state stats
	stats, err := c.rustCore.GetStats()
	if err != nil {
		c.logger.Error("failed to get state stats", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Failed to get state stats: %v\n", err)))
		return
	}
	
	// Build JSON response
	response := map[string]interface{}{
		"state_root":  fmt.Sprintf("%x", stateRoot),
		"state_size":  stats.StateSize,
		"block_number": stats.BlockNumber,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.logger.Error("failed to encode state stats", zap.Error(err))
	}
}
