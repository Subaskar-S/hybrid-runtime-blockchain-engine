---
inclusion: fileMatch
fileMatchPattern: "internal/metrics/**"
---

# Metrics Package Guide

This steering file loads automatically when you are working in `internal/metrics/`.

## What This Package Does

Owns all Prometheus metric definitions and the HTTP server that exposes them. Also owns the health, liveness, readiness, and debug endpoints.

## File Responsibilities

| File | Purpose |
|------|---------|
| `collector.go` | `Collector` struct — all 15 metric definitions, `Start`/`Stop`, background polling goroutine, recording methods |
| `interfaces.go` | Interfaces that external components must satisfy to be registered with the Collector |
| `adapters.go` | Adapter interface declarations (`WorkerPoolAdapter`, `RustCoreAdapter`, etc.) |
| `adapters_impl.go` | Concrete adapter implementations that wrap the real `worker.Pool`, `ffi.FFI`, `reorg.ReorgEngine` |

## The 15 Metrics

### Go Runtime (polled every 5s)
| Metric | Type | Source |
|--------|------|--------|
| `gc_pause_seconds` | Histogram | `runtime.MemStats.PauseNs` |
| `memory_alloc_bytes` | Gauge | `runtime.MemStats.Alloc` |
| `goroutine_count` | Gauge | `runtime.NumGoroutine()` |

### Worker Pool (polled every 5s via adapter)
| Metric | Type | Source |
|--------|------|--------|
| `worker_active_count` | Gauge | `WorkerPoolAdapter.GetStats().ActiveWorkers` |
| `worker_queue_depth` | Gauge | `WorkerPoolAdapter.GetStats().QueueDepth` |
| `worker_utilization_percent` | Gauge | `ActiveWorkers / NumWorkers × 100` |
| `worker_panic_total` | Counter | ❌ **Never recorded — TD-3** |

### Rust Core (polled every 5s via adapter)
| Metric | Type | Source |
|--------|------|--------|
| `rust_apply_block_duration_seconds` | Histogram | ❌ **Never recorded — TD-3** |
| `rust_state_size_entries` | Gauge | `RustCoreAdapter.GetStats().StateSize` |
| `rust_memory_usage_bytes` | Gauge | `RustCoreAdapter.GetStats().MemoryUsageBytes` |

### Reorg (event-driven via adapter)
| Metric | Type | Source |
|--------|------|--------|
| `reorg_total` | Counter | ❌ **Never recorded — TD-3** |
| `reorg_depth_blocks` | Histogram | ❌ **Never recorded — TD-3** |
| `reorg_rollback_duration_seconds` | Histogram | ❌ **Never recorded — TD-3** |

### Block Processing (event-driven)
| Metric | Type | Source |
|--------|------|--------|
| `blocks_processed_total` | Counter | ❌ **Never recorded — TD-3** |
| `block_processing_duration_seconds` | Histogram | ❌ **Never recorded — TD-3** |

## HTTP Endpoints

All served on `METRICS_PORT` (default 9090):

| Endpoint | Method | Returns |
|----------|--------|---------|
| `/metrics` | GET | Prometheus text format |
| `/health` | GET | `"OK"` (200) or error (503) |
| `/livez` | GET | `"OK"` always (200) — for Kubernetes liveness |
| `/readyz` | GET | `"OK"` (200) or error (503) — for Kubernetes readiness |
| `/debug/gc` | GET | JSON: GC stats from `runtime.MemStats` |
| `/debug/state` | GET | JSON: current state root + state size from Rust |

### Health Check Logic (⚠️ has a known issue)

```go
// Current logic — problematic in load-test mode
if !c.blockStreamer.IsConnected() {
    http.Error(w, "Block streamer not connected", http.StatusServiceUnavailable)
    return
}
```

In `LOAD_TEST_ENABLED=true` mode the streamer intentionally fails to connect, so `/health` always returns 503. Use `/livez` for Kubernetes probes in load-test mode (TD-7).

## The Adapter Pattern — Why It Exists

The Collector cannot import `worker`, `reorg`, or `ffi` directly — that would create import cycles. Instead:

```
cmd/server/main.go creates:
  metrics.NewWorkerPoolAdapter(workerPool)   → implements WorkerPoolAdapter
  metrics.NewRustCoreAdapter(ffiLayer)       → implements RustCoreAdapter
  metrics.NewReorgEngineAdapter(reorgEngine) → implements ReorgEngineAdapter

Then passes them to:
  collector.RegisterWorkerPool(adapter)
  collector.RegisterRustCore(adapter)
  collector.RegisterReorgEngine(adapter)
```

The Collector calls adapter methods during its polling loop and recording methods, never touching the concrete types directly.

## Wiring Event Metrics (TD-3)

The 7 event-driven metrics (all marked ❌ above) are defined but never populated. To fix this, recording calls need to reach the Collector from the event sites. Two approaches:

**Option A — Callback injection** (minimal change):
```go
// Add to worker.Pool constructor
type Pool struct {
    onBlockProcessed func(time.Duration)  // injected by main.go
    onPanic          func()               // injected by main.go
}

// In main.go
pool := worker.NewPool(logger, reorgEngine)
pool.SetOnBlockProcessed(metricsCollector.RecordBlockProcessed)
pool.SetOnPanic(metricsCollector.RecordWorkerPanic)
```

**Option B — Event recorder interface** (cleaner):
```go
// Define in metrics/interfaces.go
type EventRecorder interface {
    RecordBlockProcessed(d time.Duration)
    RecordWorkerPanic()
    RecordReorg(depth int, d time.Duration)
    RecordRustApplyBlock(d time.Duration)
}

// Inject into components via constructor
func NewPool(logger *zap.Logger, processor BlockProcessor, recorder EventRecorder) *Pool
```

Do not add a direct import of `metrics` to `worker`, `reorg`, or `ffi` — that breaks the package dependency rules.

## Adding a New Metric

1. Declare the metric field in `collector.go`:
   ```go
   myNewCounter prometheus.Counter
   ```

2. Register it in `newCollector()` (or `NewCollector()`):
   ```go
   c.myNewCounter = prometheus.NewCounter(prometheus.CounterOpts{
       Name: "my_new_total",
       Help: "Description of what this counts",
   })
   c.registry.MustRegister(c.myNewCounter)
   ```

3. Add a recording method:
   ```go
   func (c *Collector) RecordMyNewEvent() {
       c.myNewCounter.Inc()
   }
   ```

4. **Wire the recording call** at the event site (see above). A metric with no recording call is useless.

5. Add a test in `collector_test.go` that calls `RecordMyNewEvent()` and verifies the counter incremented.
