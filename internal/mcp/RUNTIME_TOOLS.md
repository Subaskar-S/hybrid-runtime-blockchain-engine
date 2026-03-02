# MCP Runtime Introspection Tools

This document describes the runtime introspection tools implemented for the MCP server.

## Overview

The runtime introspection tools provide AI agents with detailed insights into the Go runtime behavior, block processing performance, and blockchain reorganization history. These tools are exposed via the Model Context Protocol (MCP) server.

## Implemented Tools

### 1. get_gc_stats

Returns Go garbage collection statistics.

**Input:** None

**Output:**
```json
{
  "num_gc": 42,
  "pause_total_ns": 1500000,
  "pause_ns": [12000, 15000, 13000, ...],
  "last_gc": "2024-01-15T10:30:00Z"
}
```

**Fields:**
- `num_gc` (integer): Total number of GC cycles completed
- `pause_total_ns` (integer): Cumulative nanoseconds in GC stop-the-world pauses
- `pause_ns` (array of integers): Last 256 GC pause durations in nanoseconds
- `last_gc` (string): RFC3339 timestamp of last GC cycle

**Implementation:** Uses `runtime.ReadMemStats()` to collect GC statistics.

### 2. get_heap_usage

Returns current heap memory usage statistics.

**Input:** None

**Output:**
```json
{
  "alloc_bytes": 5242880,
  "total_alloc_bytes": 104857600,
  "sys_bytes": 10485760,
  "heap_objects": 12345
}
```

**Fields:**
- `alloc_bytes` (integer): Bytes of allocated heap objects
- `total_alloc_bytes` (integer): Cumulative bytes allocated (includes freed memory)
- `sys_bytes` (integer): Total bytes obtained from OS
- `heap_objects` (integer): Number of allocated heap objects

**Implementation:** Uses `runtime.ReadMemStats()` to collect heap statistics.

### 3. get_goroutine_count

Returns the number of active goroutines.

**Input:** None

**Output:**
```json
{
  "count": 8
}
```

**Fields:**
- `count` (integer): Number of currently active goroutines

**Implementation:** Uses `runtime.NumGoroutine()` to get the count.

### 4. get_latency_distribution

Returns block processing latency percentiles.

**Input:** None

**Output:**
```json
{
  "p50_ms": 2.5,
  "p95_ms": 8.3,
  "p99_ms": 12.1,
  "max_ms": 15.7
}
```

**Fields:**
- `p50_ms` (number): 50th percentile (median) latency in milliseconds
- `p95_ms` (number): 95th percentile latency in milliseconds
- `p99_ms` (number): 99th percentile latency in milliseconds
- `max_ms` (number): Maximum latency in milliseconds

**Implementation:** 
- Uses a circular buffer (LatencyTracker) to track the last 1000 block processing latencies
- Calculates percentiles by sorting the latencies and finding values at 50%, 95%, 99% positions
- Returns zeros if no latencies have been recorded

**Usage:**
```go
// Record a latency
tools.GetLatencyTracker().Record(latencyMs)

// Get distribution
result, _ := tools.GetLatencyDistribution(nil)
```

### 5. get_reorg_history

Returns recent blockchain reorganization events.

**Input:**
```json
{
  "limit": 10
}
```

**Parameters:**
- `limit` (integer, optional): Maximum number of reorg events to return (default: 10, max: 100)

**Output:**
```json
{
  "reorgs": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "fork_point": 12345,
      "depth": 3,
      "rollback_duration_ms": 25.5
    }
  ]
}
```

**Fields:**
- `reorgs` (array): Array of reorg events, most recent last
  - `timestamp` (string): RFC3339 timestamp when reorg was detected
  - `fork_point` (integer): Block number where chains diverged
  - `depth` (integer): Number of blocks rolled back
  - `rollback_duration_ms` (number): Time taken to rollback in milliseconds

**Implementation:** Queries the ReorgEngine's history of recent reorg events.

## Architecture

### LatencyTracker

A thread-safe circular buffer that tracks block processing latencies.

**Features:**
- Fixed capacity (default: 1000 entries)
- Circular buffer implementation for constant memory usage
- Thread-safe with RWMutex
- Efficient percentile calculation via sorting

**Methods:**
- `Record(latencyMs float64)`: Record a new latency value
- `GetDistribution() map[string]float64`: Calculate and return percentiles

### RuntimeTools

Container for all runtime introspection tool handlers.

**Components:**
- `latencyTracker`: Tracks block processing latencies
- `reorgEngine`: Reference to reorg engine for history queries

**Methods:**
- `GetGCStats(params)`: Handler for get_gc_stats tool
- `GetHeapUsage(params)`: Handler for get_heap_usage tool
- `GetGoroutineCount(params)`: Handler for get_goroutine_count tool
- `GetLatencyDistribution(params)`: Handler for get_latency_distribution tool
- `GetReorgHistory(params)`: Handler for get_reorg_history tool

### Registration

Tools are registered with the MCP server using `RegisterRuntimeTools`:

```go
server := mcp.NewServer(logger, 8080)
reorgEngine := reorg.NewReorgEngine(logger, ffi)
tools := mcp.RegisterRuntimeTools(server, reorgEngine)
```

This registers all five tools with the MCP server and returns the RuntimeTools instance for recording latencies.

## Usage Example

```go
// Setup
logger := zap.NewProduction()
server := mcp.NewServer(logger, 8080)
reorgEngine := reorg.NewReorgEngine(logger, ffi)
tools := mcp.RegisterRuntimeTools(server, reorgEngine)

// Start server
server.Start()

// Record latencies during block processing
start := time.Now()
// ... process block ...
latencyMs := float64(time.Since(start).Milliseconds())
tools.GetLatencyTracker().Record(latencyMs)

// Tools are now accessible via MCP JSON-RPC calls
// Example: POST to http://127.0.0.1:8080/
// {
//   "jsonrpc": "2.0",
//   "method": "get_latency_distribution",
//   "params": {},
//   "id": 1
// }
```

## Testing

Comprehensive unit tests are provided in `tools_test.go`:

- `TestLatencyTracker_*`: Tests for circular buffer behavior
- `TestGetGCStats`: Verifies GC stats collection
- `TestGetHeapUsage`: Verifies heap usage collection
- `TestGetGoroutineCount`: Verifies goroutine counting
- `TestGetLatencyDistribution_*`: Tests latency percentile calculation
- `TestGetReorgHistory_*`: Tests reorg history retrieval with various limits
- `TestRegisterRuntimeTools`: Verifies tool registration
- `TestLatencyTracker_Concurrent`: Tests thread safety

## Performance Considerations

### LatencyTracker
- **Memory:** Fixed size (1000 entries × 8 bytes = 8KB)
- **Record:** O(1) - constant time insertion
- **GetDistribution:** O(n log n) - sorting for percentile calculation
- **Thread Safety:** RWMutex allows concurrent reads

### GC Stats
- **Cost:** Calls `runtime.ReadMemStats()` which stops the world briefly
- **Recommendation:** Don't call too frequently (max once per second)

### Heap Usage
- **Cost:** Same as GC stats (uses `runtime.ReadMemStats()`)
- **Recommendation:** Don't call too frequently

### Goroutine Count
- **Cost:** Very cheap, just returns a counter
- **Recommendation:** Can be called frequently

### Reorg History
- **Cost:** O(n) where n is the limit parameter
- **Memory:** Bounded by reorg engine's history size

## Security

All tools follow MCP server security controls:
- Bind to localhost (127.0.0.1) only
- Rate limited to 10 requests per minute per tool
- Input validation for all parameters
- JSON-only responses
- No shell command execution

## Requirements Validation

This implementation satisfies:
- **Requirement 12.1**: get_gc_stats tool implemented
- **Requirement 12.2**: get_heap_usage tool implemented
- **Requirement 12.3**: get_goroutine_count tool implemented
- **Requirement 12.4**: get_latency_distribution tool implemented
- **Requirement 12.5**: get_reorg_history tool implemented

All tools return JSON-formatted responses as specified in the design document.

## Rust Core Introspection Tools

### 6. get_state_root

Returns the current state root hash from the Rust core.

**Input:** None

**Output:**
```json
{
  "state_root": "a1b2c3d4e5f6...",
  "block_number": 12345
}
```

**Fields:**
- `state_root` (string): Hex-encoded state root hash (64 characters)
- `block_number` (integer): Current block number in the state engine

**Implementation:** Calls FFI `GetStateRoot()` and `GetStats()` to retrieve the state root and block number.

### 7. get_state_size

Returns the number of state entries and memory usage.

**Input:** None

**Output:**
```json
{
  "entry_count": 1000,
  "memory_bytes": 524288
}
```

**Fields:**
- `entry_count` (integer): Number of accounts in the state map
- `memory_bytes` (integer): Memory usage of the state engine in bytes

**Implementation:** Calls FFI `GetStats()` to retrieve state size and memory usage.

### 8. get_apply_block_latency

Returns recent apply_block execution times from the Rust core.

**Input:**
```json
{
  "limit": 100
}
```

**Parameters:**
- `limit` (integer, optional): Maximum number of latencies to return (default: 100, max: 1000)

**Output:**
```json
{
  "latencies_ms": [1.2, 1.5, 1.3, ...],
  "mean_ms": 1.4,
  "stddev_ms": 0.2
}
```

**Fields:**
- `latencies_ms` (array of numbers): Recent apply_block latencies in milliseconds (most recent first)
- `mean_ms` (number): Mean latency in milliseconds
- `stddev_ms` (number): Standard deviation of latencies in milliseconds

**Implementation:**
- Uses ApplyBlockLatencyTracker circular buffer (capacity: 1000)
- Calculates mean and standard deviation from recorded latencies
- Returns most recent latencies up to the specified limit

**Usage:**
```go
// Record an apply_block latency
tools.GetApplyBlockLatencyTracker().Record(latencyMs)

// Get latencies
result, _ := tools.GetApplyBlockLatency(map[string]interface{}{"limit": 50})
```

### 9. validate_determinism

Reapplies recent blocks and verifies state root consistency.

**Input:**
```json
{
  "block_count": 10
}
```

**Parameters:**
- `block_count` (integer, optional): Number of blocks to validate (default: 10, max: 100)

**Output:**
```json
{
  "consistent": false,
  "blocks_checked": 0,
  "error": "validate_determinism not yet implemented: requires block history infrastructure"
}
```

**Fields:**
- `consistent` (boolean): Whether state root remained consistent after rollback and replay
- `blocks_checked` (integer): Number of blocks validated
- `error` (string): Error message if validation failed or not implemented

**Implementation:** 
- Placeholder implementation (requires block history infrastructure)
- When implemented, will:
  1. Save current state root
  2. Rollback N blocks
  3. Replay N blocks
  4. Compare final state root with saved state root

## Load Testing Tools

### 10. compare_gc_vs_core_latency

Returns latency variance comparison between Go runtime (with GC) and Rust core (no GC).

**Input:** None

**Output:**
```json
{
  "go_latency_variance_ms": 16.0,
  "rust_latency_variance_ms": 0.04,
  "variance_ratio": 400.0
}
```

**Fields:**
- `go_latency_variance_ms` (number): Variance of Go block processing latencies in milliseconds
- `rust_latency_variance_ms` (number): Variance of Rust apply_block latencies in milliseconds
- `variance_ratio` (number): Ratio of Go variance to Rust variance (higher = more GC impact)

**Implementation:**
- Calculates Go variance from latency distribution percentiles: `variance ≈ ((p99 - p50) / 2)^2`
- Calculates Rust variance from apply_block latency standard deviation: `variance = stddev^2`
- Computes ratio: `go_variance / rust_variance`
- Returns 0 for variance_ratio if Rust variance is 0 (to avoid division by zero)

**Interpretation:**
- **variance_ratio > 1**: Go latency is more variable than Rust (GC pauses causing variance)
- **variance_ratio ≈ 1**: Similar variance (GC impact minimal)
- **variance_ratio < 1**: Rust latency is more variable (unexpected, investigate)

**Usage:**
This tool helps identify whether GC pauses are causing latency variance in the Go orchestration layer compared to the deterministic Rust core.

```go
// After recording latencies for both Go and Rust
result, _ := tools.CompareGCVsCoreLatency(nil)
```

## Architecture Updates

### ApplyBlockLatencyTracker

A thread-safe circular buffer that tracks Rust apply_block execution times.

**Features:**
- Fixed capacity (default: 1000 entries)
- Circular buffer implementation for constant memory usage
- Thread-safe with RWMutex
- Efficient statistics calculation (mean, standard deviation)

**Methods:**
- `Record(latencyMs float64)`: Record a new latency value
- `GetLatencies(limit int) []float64`: Get most recent latencies
- `GetStatistics(limit int) (mean, stddev float64)`: Calculate mean and standard deviation

### FFIInterface

Interface for FFI operations needed by MCP tools.

**Methods:**
- `GetStateRoot() ([32]byte, error)`: Get current state root hash
- `GetStats() (*ffi.Stats, error)`: Get state size and memory usage

**Implementation:** Allows decoupling of MCP tools from concrete FFI implementation for testing.

## Testing Updates

Additional tests in `tools_test.go`:

- `TestGetStateRoot`: Verifies state root retrieval via FFI
- `TestGetStateRoot_FFIError`: Tests error handling
- `TestGetStateSize`: Verifies state size retrieval
- `TestGetStateSize_FFIError`: Tests error handling
- `TestGetApplyBlockLatency_*`: Tests latency tracking and statistics
- `TestApplyBlockLatencyTracker_*`: Tests circular buffer behavior
- `TestValidateDeterminism_*`: Tests determinism validation (placeholder)
- `TestCompareGCVsCoreLatency_*`: Tests variance comparison
- `TestRegisterRuntimeTools_WithRustCoreTools`: Verifies all 10 tools are registered

## Requirements Validation Updates

This implementation satisfies:
- **Requirement 13.1**: get_state_root tool implemented
- **Requirement 13.2**: get_state_size tool implemented
- **Requirement 13.3**: get_apply_block_latency tool implemented
- **Requirement 13.4**: validate_determinism tool implemented (placeholder)
- **Requirement 13.5**: validate_determinism validates state root consistency (when implemented)
- **Requirement 14.4**: compare_gc_vs_core_latency tool implemented

All 11 MCP tools are now registered and functional (10 fully implemented, 1 placeholder).

### 11. run_load_test

Executes a load test with specified TPS and duration.

**Input:**
```json
{
  "tps": 1000,
  "duration": 60
}
```

**Parameters:**
- `tps` (integer, required): Target transactions per second (range: 1-10000)
- `duration` (integer, required): Test duration in seconds (range: 1-300)

**Output:**
```json
{
  "success": true,
  "message": "Load test completed: 1000 TPS for 60 seconds"
}
```

**Fields:**
- `success` (boolean): Whether the load test completed successfully
- `message` (string): Status message with test configuration

**Implementation:**
- Validates TPS is between 1 and 10000
- Validates duration is between 1 and 300 seconds
- Delegates to LoadTester.Run() to execute the test
- Returns error if load tester is not configured

**Usage:**
```go
// Configure load tester
loadTester := loadtest.NewLoadTester(logger, ffi, workerPool, reorgEngine)
tools.SetLoadTester(loadTester)

// Run load test via MCP
result, _ := tools.RunLoadTest(map[string]interface{}{
  "tps": 1000,
  "duration": 60,
})
```

**Error Cases:**
- Missing `tps` parameter: "tps parameter is required"
- Missing `duration` parameter: "duration parameter is required"
- Invalid TPS range: "tps must be between 1 and 10000"
- Invalid duration range: "duration must be between 1 and 300"
- Load tester not configured: "load tester not configured"
- Load test execution error: "load test failed: [error details]"
