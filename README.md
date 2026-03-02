# Hybrid Runtime Blockchain Engine

A production-grade blockchain event processing system that combines Go orchestration with Rust deterministic execution for optimal performance and reliability.

## Architecture Overview

### Hybrid Runtime Design

This system implements a **hybrid runtime architecture** that separates concerns between two execution environments:

- **Go Orchestration Layer**: Handles I/O, concurrency, networking, and observability
- **Rust Deterministic Core**: Executes state transitions with guaranteed determinism

This design provides the best of both worlds:
- Go's excellent concurrency primitives and ecosystem for orchestration
- Rust's memory safety and deterministic execution for critical state logic

### Why Hybrid Runtime?

**Problem**: Blockchain state transitions must be deterministic and reproducible. Go's garbage collector introduces non-determinism through unpredictable pause times and memory allocation patterns.

**Solution**: Isolate deterministic state logic in Rust (no GC) while leveraging Go's strengths for everything else.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     Go Orchestration Layer                   │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   Block      │───▶│  Worker Pool │───▶│    Reorg     │  │
│  │  Streamer    │    │  (Goroutines)│    │    Engine    │  │
│  └──────────────┘    └──────┬───────┘    └──────────────┘  │
│                              │                                │
│                              │ FFI Boundary                   │
│                              ▼                                │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              FFI Layer (cgo)                        │    │
│  │  • Binary Serialization                             │    │
│  │  • Input Validation                                 │    │
│  │  • Memory Management                                │    │
│  └─────────────────────────────────────────────────────┘    │
│                              │                                │
└──────────────────────────────┼────────────────────────────────┘
                               │
┌──────────────────────────────┼────────────────────────────────┐
│                              ▼                                │
│                   Rust Deterministic Core                     │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │    State     │───▶│   Rollback   │───▶│    Blake3    │  │
│  │    Engine    │    │   Mechanism  │    │  State Root  │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│                                                               │
│  • No GC (deterministic)                                     │
│  • No system time                                            │
│  • No random numbers                                         │
│  • Pure state transitions                                    │
│                                                               │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                    Observability Layer                       │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │  Prometheus  │    │  MCP Server  │    │  Structured  │  │
│  │   Metrics    │    │  (11 tools)  │    │   Logging    │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### GC vs Deterministic Execution Trade-offs

**Go with GC (Orchestration Layer)**:
- ✅ Excellent for I/O and concurrency
- ✅ Rich ecosystem and tooling
- ✅ Fast development iteration
- ❌ Non-deterministic GC pauses
- ❌ Memory allocation patterns vary

**Rust without GC (Deterministic Core)**:
- ✅ Zero-cost abstractions
- ✅ Guaranteed determinism
- ✅ Predictable performance
- ✅ Memory safety without GC
- ❌ Steeper learning curve
- ❌ Slower compilation

**Our Approach**: Use each language where it excels. Go handles the messy real world (network, I/O, concurrency), Rust handles the critical state logic.

### FFI Boundary Design

The FFI (Foreign Function Interface) boundary is carefully designed for safety and performance:

**Memory Ownership Rules**:
1. **Go → Rust**: Go allocates, serializes, and passes ownership to Rust
2. **Rust → Go**: Rust allocates, Go takes ownership and frees
3. **No Shared Memory**: All data is copied across the boundary

**Safety Mechanisms**:
- Input validation on all FFI calls
- Size limits (max 10MB per block)
- Version byte for protocol evolution
- Null pointer checks
- Panic isolation (Go panics don't crash Rust, vice versa)

**Performance Considerations**:
- Binary serialization (not JSON) for speed
- Batch processing to amortize FFI overhead
- Zero-copy where possible within each runtime



## Implementation Guidance

### When to Implement Logic in Go vs Rust

**Implement in Rust (Deterministic Core) when**:
- Logic involves state transitions
- Determinism is critical
- Performance is critical (hot path)
- Memory safety is paramount
- Example: `apply_block`, `rollback_to`, state root calculation

**Implement in Go (Orchestration Layer) when**:
- Logic involves I/O (network, disk, database)
- Logic involves concurrency coordination
- Logic involves observability (metrics, logging)
- Rapid iteration is needed
- Example: WebSocket connection, worker pool, metrics collection

**Rule of Thumb**: If it touches state, use Rust. If it touches the outside world, use Go.

### Component Responsibilities

**Block Streamer** (Go):
- Connect to Ethereum RPC via WebSocket
- Subscribe to new block headers
- Fetch full block data
- Validate blocks before forwarding
- Handle connection failures with exponential backoff

**Worker Pool** (Go):
- Manage goroutine lifecycle
- Provide backpressure via bounded channel
- Recover from panics
- Track active worker count
- Submit blocks to Rust core via FFI

**Reorg Engine** (Go):
- Detect chain reorganizations
- Maintain ring buffer of recent blocks
- Coordinate rollback via FFI
- Replay blocks on new canonical chain
- Track reorg metrics

**State Engine** (Rust):
- Apply block transactions to state
- Calculate state root with Blake3
- Maintain rollback history
- Provide deterministic execution
- No I/O, no system calls

**FFI Layer** (Go + Rust):
- Serialize/deserialize blocks
- Validate inputs
- Manage memory across boundary
- Convert error codes to Go errors
- Provide type-safe wrappers

**Metrics Collector** (Go):
- Expose Prometheus metrics
- Collect Go runtime stats
- Query Rust core stats via FFI
- Provide health endpoints
- Track performance metrics

**MCP Server** (Go):
- Expose runtime introspection tools
- Rate limit requests (10/min per tool)
- Validate inputs
- Bind to localhost only
- Provide JSON-RPC interface

### Data Flow Patterns

**Normal Block Processing**:
```
1. Block Streamer receives new block from RPC
2. Block Streamer validates block structure
3. Block Streamer sends block to Worker Pool channel
4. Worker goroutine picks up block
5. Worker serializes block for FFI
6. Worker calls Rust apply_block via FFI
7. Rust updates state and returns state root
8. Worker records metrics
9. Reorg Engine adds block to ring buffer
```

**Reorg Handling**:
```
1. Reorg Engine detects parent hash mismatch
2. Reorg Engine searches ring buffer for fork point
3. Reorg Engine calls Rust rollback_to(fork_point)
4. Rust restores state from snapshot
5. Reorg Engine replays blocks from new chain
6. Each replayed block goes through normal processing
7. Reorg metrics are updated
```

**Metrics Collection**:
```
1. Metrics Collector runs periodic collection (every 5s)
2. Collector queries Go runtime stats (GC, memory, goroutines)
3. Collector queries Rust stats via FFI (state size, memory)
4. Collector queries component stats (worker pool, reorg engine)
5. Collector updates Prometheus metrics
6. Prometheus scrapes /metrics endpoint
```

**MCP Tool Invocation**:
```
1. AI agent sends JSON-RPC request to MCP server
2. MCP server validates request (rate limit, input validation)
3. MCP server routes to appropriate tool handler
4. Tool handler queries runtime data (Go or Rust via FFI)
5. Tool handler formats response as JSON
6. MCP server returns response to agent
```



## Performance Interpretation Guide

### Expected Latency Ranges

**Block Processing (End-to-End)**:
- p50: 2-3ms (typical)
- p95: 4-5ms (acceptable)
- p99: <5ms (target)
- Max: <10ms (alert threshold)

**Rust Core (apply_block)**:
- p50: 1-1.5ms (typical)
- p95: 1.8-2ms (acceptable)
- p99: <2ms (target)
- Max: <5ms (alert threshold)

**Go GC Pause**:
- p50: 0.5-1ms (typical)
- p95: 5-8ms (acceptable)
- p99: <10ms (target)
- Max: <20ms (alert threshold)

**Reorg Rollback (10 blocks)**:
- Typical: 15-20ms
- Target: <20ms
- Alert: >50ms

### Interpreting GC Pause Metrics

**Metric**: `go_gc_pause_ns` (histogram)

**What it means**: Time the Go runtime spends in stop-the-world GC pauses.

**How to interpret**:
- **Low variance (p99/p50 < 5x)**: GC is well-behaved, heap size is stable
- **High variance (p99/p50 > 10x)**: GC is struggling, possible memory leak or heap pressure
- **Increasing trend**: Memory usage growing, may need more workers or heap tuning

**Troubleshooting**:
- If p99 > 10ms: Check heap size, consider increasing GOGC
- If pause frequency is high: Reduce allocation rate, use sync.Pool
- If pause duration is high: Increase heap size or reduce live objects

**Correlation with block processing**:
- Use `compare_gc_vs_core_latency` MCP tool
- If variance_ratio > 10: GC is causing latency spikes
- If variance_ratio < 2: GC impact is minimal

### Interpreting Reorg Metrics

**Metric**: `reorg_total` (counter)

**What it means**: Total number of chain reorganizations detected.

**How to interpret**:
- **0-5 per day**: Normal for Ethereum mainnet
- **5-20 per day**: Elevated but acceptable
- **>20 per day**: Investigate RPC connection quality or network issues

**Metric**: `reorg_depth` (histogram)

**What it means**: Number of blocks rolled back during reorg.

**How to interpret**:
- **1-2 blocks**: Normal, happens frequently
- **3-5 blocks**: Uncommon but acceptable
- **>5 blocks**: Rare, investigate network conditions
- **>10 blocks**: Critical, may indicate RPC issues or network attack

**Metric**: `reorg_rollback_duration_ms` (histogram)

**What it means**: Time taken to rollback and replay blocks.

**How to interpret**:
- **<20ms**: Excellent, within target
- **20-50ms**: Acceptable, monitor for trends
- **>50ms**: Slow, investigate Rust core performance or disk I/O

**Troubleshooting**:
- If reorg_total is high: Check RPC endpoint quality, consider switching providers
- If reorg_depth is high: Verify RPC is following canonical chain
- If rollback_duration is high: Check Rust core latency, verify no disk I/O bottlenecks

### Performance Under Different Loads

**1000 TPS (4 workers)**:
- Block processing: p99 < 5ms ✓
- Worker utilization: 60-70%
- GC pause: p99 < 10ms ✓
- Throughput: Sustained

**5000 TPS (4 workers)**:
- Block processing: p99 5-8ms
- Worker utilization: 90-95%
- GC pause: p99 10-15ms
- Throughput: Near capacity

**10000 TPS (4 workers)**:
- Block processing: p99 > 10ms ⚠️
- Worker utilization: 100%
- GC pause: p99 > 15ms ⚠️
- Throughput: Saturated, increase workers

**Scaling Guidelines**:
- 1-2 workers: Development/testing
- 4 workers: Production (1000 TPS)
- 8 workers: High load (5000 TPS)
- 16+ workers: Extreme load (10000+ TPS)

### Troubleshooting Guide

**Symptom**: High block processing latency (p99 > 10ms)

**Possible Causes**:
1. GC pressure → Check `go_gc_pause_ns`, increase heap size
2. Worker saturation → Check `worker_pool_active`, increase worker count
3. Rust core slow → Check `rust_apply_block_latency_ms`, investigate state size
4. Network issues → Check Block Streamer connection status

**Symptom**: Frequent reorgs (>20/day)

**Possible Causes**:
1. Poor RPC quality → Switch to better provider
2. Network congestion → Check Ethereum network status
3. Stale blocks → Verify RPC is synced

**Symptom**: Memory growth

**Possible Causes**:
1. Memory leak in Go → Run with `-race` flag, check for goroutine leaks
2. State growth in Rust → Check `rust_state_size`, verify rollback history is bounded
3. Unbounded channels → Check worker pool queue depth

**Symptom**: Panics in worker pool

**Possible Causes**:
1. Invalid block data → Check input validation logs
2. FFI errors → Check Rust core logs
3. Out of memory → Check heap size and state size

**Diagnostic Tools**:
- `/metrics`: Prometheus metrics
- `/health`: Health check endpoint
- `/debug/gc`: GC statistics
- `/debug/state`: State root and size
- MCP tools: Runtime introspection (11 tools available)



## Usage Documentation

### Configuration via Environment Variables

**Required**:
- `ETH_RPC_URL`: Ethereum RPC WebSocket endpoint (e.g., `ws://localhost:8545`)

**Optional**:
- `WORKER_COUNT`: Number of worker goroutines (default: 4, range: 1-256)
- `METRICS_PORT`: Prometheus metrics port (default: 9090, range: 1-65535)
- `MCP_PORT`: MCP server port (default: 8080, range: 1-65535)
- `LOAD_TEST_ENABLED`: Enable load testing mode (default: false)

**Example**:
```bash
export ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-API-KEY"
export WORKER_COUNT=8
export METRICS_PORT=9090
export MCP_PORT=8080
export LOAD_TEST_ENABLED=false
```

**Validation**:
- All required variables must be set
- Numeric values must be within valid ranges
- Configuration is validated at startup
- Invalid configuration causes immediate exit with descriptive error

### Docker Deployment

**Build Image**:
```bash
docker build -t hybrid-runtime-blockchain-engine:latest .
```

**Run Container**:
```bash
docker run -d \
  --name blockchain-engine \
  -e ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-API-KEY" \
  -e WORKER_COUNT=4 \
  -p 9090:9090 \
  -p 8080:8080 \
  hybrid-runtime-blockchain-engine:latest
```

**With Docker Compose**:
```yaml
version: '3.8'
services:
  blockchain-engine:
    image: hybrid-runtime-blockchain-engine:latest
    environment:
      - ETH_RPC_URL=wss://mainnet.infura.io/ws/v3/YOUR-API-KEY
      - WORKER_COUNT=4
      - METRICS_PORT=9090
      - MCP_PORT=8080
    ports:
      - "9090:9090"  # Prometheus metrics
      - "8080:8080"  # MCP server
    restart: unless-stopped
```

**Health Check**:
```bash
curl http://localhost:9090/health
```

**View Logs**:
```bash
docker logs -f blockchain-engine
```

**Graceful Shutdown**:
```bash
docker stop blockchain-engine  # Sends SIGTERM, waits 30s
```

### Prometheus Metrics

**Metrics Endpoint**: `http://localhost:9090/metrics`

**Available Metrics**:

**Go Runtime**:
- `go_gc_pause_ns`: GC pause duration histogram
- `go_gc_count`: Total GC cycles
- `go_memory_alloc_bytes`: Allocated heap memory
- `go_memory_sys_bytes`: Total memory from OS
- `go_goroutines`: Active goroutine count

**Worker Pool**:
- `worker_pool_active`: Active worker count
- `worker_pool_queue_depth`: Pending blocks in queue
- `worker_pool_utilization`: Worker utilization percentage
- `worker_pool_panics_total`: Total panic recoveries

**Rust Core**:
- `rust_apply_block_latency_ms`: apply_block execution time
- `rust_state_size`: Number of accounts in state
- `rust_memory_bytes`: Rust core memory usage

**Reorg**:
- `reorg_total`: Total reorg count
- `reorg_depth`: Reorg depth histogram
- `reorg_rollback_duration_ms`: Rollback time histogram

**Block Processing**:
- `block_processing_total`: Total blocks processed
- `block_processing_duration_ms`: Processing time histogram

**Prometheus Configuration**:
```yaml
scrape_configs:
  - job_name: 'blockchain-engine'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

**Grafana Dashboard**:
- Import dashboard from `grafana/dashboard.json` (if provided)
- Or create custom dashboard using metrics above

### MCP Tools

**MCP Server**: `http://localhost:8080`

**Available Tools** (11 total):

**Go Runtime Introspection**:
1. `get_gc_stats`: GC statistics (num_gc, pause_total_ns, pause_ns, last_gc)
2. `get_heap_usage`: Heap memory usage (alloc_bytes, total_alloc_bytes, sys_bytes, heap_objects)
3. `get_goroutine_count`: Active goroutine count
4. `get_latency_distribution`: Block processing latency percentiles (p50, p95, p99, max)
5. `get_reorg_history`: Recent reorg events (timestamp, fork_point, depth, rollback_duration)

**Rust Core Introspection**:
6. `get_state_root`: Current state root and block number
7. `get_state_size`: State entry count and memory usage
8. `get_apply_block_latency`: Recent apply_block execution times (latencies, mean, stddev)
9. `validate_determinism`: Verify state root consistency (placeholder)

**Load Testing**:
10. `compare_gc_vs_core_latency`: GC vs Rust latency variance comparison
11. `run_load_test`: Execute load test with specified TPS and duration

**Example Usage** (JSON-RPC):
```bash
# Get GC stats
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "get_gc_stats",
    "params": {},
    "id": 1
  }'

# Get latency distribution
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "get_latency_distribution",
    "params": {},
    "id": 2
  }'

# Run load test
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "run_load_test",
    "params": {
      "tps": 1000,
      "duration": 60
    },
    "id": 3
  }'
```

**Rate Limiting**:
- 10 requests per minute per tool
- Returns HTTP 429 when exceeded
- Resets every minute

**Security**:
- Binds to localhost (127.0.0.1) only
- Input validation on all parameters
- No shell command execution
- JSON-only responses

### Load Testing

**Enable Load Testing**:
```bash
export LOAD_TEST_ENABLED=true
```

**Run Load Test via MCP**:
```bash
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "run_load_test",
    "params": {
      "tps": 1000,
      "duration": 60
    },
    "id": 1
  }'
```

**Parameters**:
- `tps`: Target transactions per second (range: 1-10000)
- `duration`: Test duration in seconds (range: 1-300)

**Load Test Modes**:
- 1000 TPS: Light load, baseline performance
- 5000 TPS: Medium load, stress test
- 10000 TPS: Heavy load, capacity test

**Metrics Collected**:
- p50, p99 latency
- Throughput (blocks/second)
- GC pause time during test
- Memory growth rate
- Reorg rollback cost

**Benchmark Report**:
- Exported to JSON file
- Includes all metrics and test configuration
- Use for performance regression testing



## Build and Test Documentation

### Makefile Targets

**Build**:
```bash
make build
```
- Builds Rust library (`cargo build --release`)
- Builds Go binary with cgo
- Output: `./bin/blockchain-engine`

**Run**:
```bash
make run
```
- Sets default environment variables
- Runs compiled binary
- Requires `ETH_RPC_URL` to be set

**Test**:
```bash
make test
```
- Runs Rust tests (`cargo test`)
- Runs Go tests (`go test ./... -v -race`)
- Includes race detector for concurrency issues

**Benchmark**:
```bash
make bench
```
- Runs Rust benchmarks (`cargo bench`)
- Runs Go benchmarks (`go test ./... -bench=. -benchmem`)
- Reports operations per second and memory allocations

**Clean**:
```bash
make clean
```
- Removes Rust target directory
- Removes Go binary
- Removes test artifacts

### Running Tests

**All Tests**:
```bash
go test ./... -v
```

**With Race Detector**:
```bash
go test ./... -v -race
```

**Specific Package**:
```bash
go test ./internal/worker -v
```

**Rust Tests**:
```bash
cd rust-core
cargo test
```

**Integration Tests**:
```bash
go test ./cmd/server -v
```

### Running Benchmarks

**Go Benchmarks**:
```bash
go test ./... -bench=. -benchmem
```

**Rust Benchmarks**:
```bash
cd rust-core
cargo bench
```

**Specific Benchmark**:
```bash
go test ./internal/ffi -bench=BenchmarkApplyBlock -benchmem
```

**Benchmark Output**:
```
BenchmarkApplyBlock-8    1000    1234567 ns/op    12345 B/op    123 allocs/op
```
- `1000`: Number of iterations
- `1234567 ns/op`: Nanoseconds per operation
- `12345 B/op`: Bytes allocated per operation
- `123 allocs/op`: Allocations per operation

### Coverage Analysis

**Generate Coverage Report**:
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

**View Coverage**:
```bash
open coverage.html  # macOS
xdg-open coverage.html  # Linux
start coverage.html  # Windows
```

**Coverage Target**: ≥80%

**Check Coverage**:
```bash
go test ./... -cover
```

**Per-Package Coverage**:
```bash
go test ./internal/worker -coverprofile=worker.out
go tool cover -func=worker.out
```

### Development Workflow

**1. Setup**:
```bash
# Install dependencies
go mod download
cd rust-core && cargo build

# Set environment variables
export ETH_RPC_URL="ws://localhost:8545"
```

**2. Build**:
```bash
make build
```

**3. Test**:
```bash
make test
```

**4. Run**:
```bash
make run
```

**5. Iterate**:
- Make changes
- Run tests
- Check coverage
- Run benchmarks
- Commit

### Debugging

**Enable Debug Logging**:
```go
logger, _ := logging.NewDevelopmentLogger()
```

**Run with Delve**:
```bash
dlv debug ./cmd/server
```

**Attach to Running Process**:
```bash
dlv attach $(pgrep blockchain-engine)
```

**Memory Profiling**:
```bash
go test ./internal/worker -memprofile=mem.prof
go tool pprof mem.prof
```

**CPU Profiling**:
```bash
go test ./internal/worker -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

### Continuous Integration

**GitHub Actions Example**:
```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: 1.21
      - uses: actions-rs/toolchain@v1
        with:
          toolchain: stable
      - name: Build
        run: make build
      - name: Test
        run: make test
      - name: Coverage
        run: |
          go test ./... -coverprofile=coverage.out
          go tool cover -func=coverage.out
```

## License

[Your License Here]

## Contributing

[Your Contributing Guidelines Here]

## Support

For issues and questions:
- GitHub Issues: [Your Repo URL]
- Documentation: [Your Docs URL]
- Email: [Your Email]

