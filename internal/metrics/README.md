# Metrics Collector

This package implements Prometheus metrics collection and exposition for the Hybrid Runtime Blockchain Engine.

## Features

### Metrics Defined

#### Go Runtime Metrics
- `gc_pause_seconds` (histogram) - Go garbage collection pause duration
- `memory_alloc_bytes` (gauge) - Current memory allocation
- `goroutine_count` (gauge) - Number of active goroutines

#### Worker Pool Metrics
- `worker_active_count` (gauge) - Number of currently active workers
- `worker_queue_depth` (gauge) - Number of blocks waiting in worker queue
- `worker_utilization_percent` (gauge) - Worker pool utilization percentage
- `worker_panic_total` (counter) - Total number of worker panics

#### Rust Core Metrics
- `rust_apply_block_duration_seconds` (histogram) - Rust core apply_block execution duration
- `rust_state_size_entries` (gauge) - Number of entries in Rust state
- `rust_memory_usage_bytes` (gauge) - Rust core memory usage

#### Reorg Metrics
- `reorg_total` (counter) - Total number of blockchain reorganizations
- `reorg_depth_blocks` (histogram) - Blockchain reorganization depth
- `reorg_rollback_duration_seconds` (histogram) - Blockchain reorganization rollback duration

#### Block Processing Metrics
- `blocks_processed_total` (counter) - Total number of blocks processed
- `block_processing_duration_seconds` (histogram) - Block processing duration

## Usage

### Creating a Collector

```go
import (
    "github.com/hybrid-runtime-blockchain-engine/internal/metrics"
    "go.uber.org/zap"
)

logger, _ := zap.NewProduction()
collector := metrics.NewCollector(logger)
```

### Starting the Collector

```go
ctx := context.Background()
port := 9090

if err := collector.Start(ctx, port); err != nil {
    log.Fatal(err)
}
```

This starts:
1. HTTP server on the specified port with `/metrics` endpoint
2. Periodic collection of Go runtime metrics (every 5 seconds)
3. Periodic collection of component metrics (every 5 seconds)

### Registering Components

```go
// Register worker pool for automatic stats collection
workerPool := worker.NewPool(logger, processor)
collector.RegisterWorkerPool(metrics.NewWorkerPoolAdapter(workerPool))

// Register reorg engine
reorgEngine := reorg.NewReorgEngine(logger, ffi)
collector.RegisterReorgEngine(metrics.NewReorgEngineAdapter(reorgEngine))

// Register Rust core
ffiInstance := ffi.NewFFI()
collector.RegisterRustCore(metrics.NewRustCoreAdapter(ffiInstance))

// Register block streamer for health checks
blockStreamer := streamer.NewEthBlockStreamer(logger)
collector.RegisterBlockStreamer(blockStreamer)

// Register worker pool for health checks
collector.RegisterWorkerPoolHealth(workerPool)
```

### Recording Metrics Manually

```go
// Record a processed block
collector.RecordBlockProcessed(10 * time.Millisecond)

// Record Rust apply_block call
collector.RecordRustApplyBlock(5 * time.Millisecond)

// Record a reorganization
collector.RecordReorg(depth, rollbackDuration)

// Record a worker panic
collector.RecordWorkerPanic()

// Update worker stats manually (if not using auto-collection)
collector.UpdateWorkerStats(activeWorkers, queueDepth, totalWorkers)

// Update Rust stats manually (if not using auto-collection)
collector.UpdateRustStats(stateSize, memoryUsage)
```

### Stopping the Collector

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := collector.Stop(ctx); err != nil {
    log.Error("failed to stop collector", zap.Error(err))
}
```

## HTTP Endpoints

### /metrics

Exposes Prometheus metrics in the standard exposition format.

**Example:**
```bash
curl http://localhost:9090/metrics
```

**Response:**
```
# HELP gc_pause_seconds Go garbage collection pause duration in seconds
# TYPE gc_pause_seconds histogram
gc_pause_seconds_bucket{le="0.0001"} 0
gc_pause_seconds_bucket{le="0.0002"} 5
...

# HELP memory_alloc_bytes Current memory allocation in bytes
# TYPE memory_alloc_bytes gauge
memory_alloc_bytes 1.234567e+07

# HELP blocks_processed_total Total number of blocks processed
# TYPE blocks_processed_total counter
blocks_processed_total 1000
...
```

### /health

Health check endpoint that returns HTTP 200 when the system is healthy, or HTTP 503 when unhealthy.

**Health Criteria:**
- Block streamer must be connected
- At least one worker must be active

**Example:**
```bash
curl http://localhost:9090/health
```

**Response (Healthy):**
```
HTTP/1.1 200 OK
OK
```

**Response (Unhealthy):**
```
HTTP/1.1 503 Service Unavailable
Block streamer not connected
```

### /debug/gc

Returns Go garbage collection statistics as JSON.

**Example:**
```bash
curl http://localhost:9090/debug/gc
```

**Response:**
```json
{
  "num_gc": 42,
  "pause_total_ns": 1234567890,
  "pause_ns": [12345, 23456, 34567, ...],
  "last_gc": "2024-01-15T10:30:00Z"
}
```

**Fields:**
- `num_gc`: Total number of GC cycles completed
- `pause_total_ns`: Cumulative GC pause time in nanoseconds
- `pause_ns`: Array of recent GC pause times (up to last 256)
- `last_gc`: Timestamp of last GC cycle (RFC3339 format)

### /debug/state

Returns current Rust core state information as JSON.

**Example:**
```bash
curl http://localhost:9090/debug/state
```

**Response:**
```json
{
  "state_root": "0123456789abcdef...",
  "state_size": 1000,
  "block_number": 12345
}
```

**Fields:**
- `state_root`: Current state root hash (64 hex characters)
- `state_size`: Number of entries in the state
- `block_number`: Current block number


## Architecture

### Periodic Collection

The collector automatically collects metrics every 5 seconds:

1. **Go Runtime Metrics**: Collected via `runtime.ReadMemStats()` and `runtime.NumGoroutine()`
2. **Component Metrics**: Collected by calling `GetStats()` on registered components
3. **Event Metrics**: Recorded when events occur (block processed, reorg detected, etc.)

### Adapters

Adapter types bridge the gap between component interfaces and metrics interfaces:

- `WorkerPoolAdapter` - Adapts `worker.Pool` to `WorkerPoolStats`
- `ReorgEngineAdapter` - Adapts `reorg.ReorgEngine` to `ReorgEngineStats`
- `RustCoreAdapter` - Adapts `ffi.FFI` to `RustCoreStats`

### Thread Safety

All metric updates are thread-safe. Prometheus client library handles synchronization internally.

## Configuration

The metrics collector is configured via:

- **Port**: Specified when calling `Start(ctx, port)`
- **Collection Interval**: Fixed at 5 seconds (can be modified in `collectPeriodically`)

## Integration with Prometheus

Add the following to your Prometheus configuration:

```yaml
scrape_configs:
  - job_name: 'hybrid-runtime-blockchain-engine'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

## Testing

Note: Tests require CGO and Rust library linking. On Windows, there may be linking issues with MinGW.

To run tests:
```bash
go test ./internal/metrics/... -v
```

## Requirements Satisfied

This implementation satisfies the following requirements from the design document:

- **Requirement 10.1**: /metrics HTTP endpoint on configurable port
- **Requirement 10.2**: GC pause duration tracking
- **Requirement 10.3**: Memory allocation rate tracking
- **Requirement 10.4**: Rust core apply_block latency tracking
- **Requirement 10.5**: Reorg count tracking
- **Requirement 10.6**: Worker queue depth tracking
- **Requirement 10.7**: Worker utilization percentage tracking
- **Requirement 11.1**: /health HTTP endpoint (200 = healthy, 503 = unhealthy)
- **Requirement 11.2**: /debug/gc endpoint with GC statistics as JSON
- **Requirement 11.3**: /debug/state endpoint with state root and size as JSON
- **Requirement 11.4**: /health returns 503 when Block_Streamer is disconnected
