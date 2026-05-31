# Hybrid Runtime Blockchain Engine — Developer Guide

## What Is This and Why Does It Exist?

### The Problem

Blockchain indexers need to process Ethereum blocks in real time and maintain accurate state. Two hard problems arise:

1. **Determinism** — State transitions (balance updates, nonce increments) must produce *identical results* given the same input, every single time. Go's garbage collector introduces non-deterministic pauses and memory allocation patterns that make this guarantee impossible in pure Go.

2. **Chain Reorganizations (Reorgs)** — Ethereum occasionally reorganizes its chain. A block you processed at height 100 may be replaced by a different block at the same height. Your state must roll back and replay correctly.

### The Solution

This engine uses a **hybrid runtime architecture**:

- **Go** handles everything that touches the outside world: WebSocket connections, HTTP servers, worker pools, metrics, logging.
- **Rust** handles all state transitions: balance updates, state root calculation, rollback history. Rust has no garbage collector, so execution is deterministic and latency is predictable.

The two runtimes communicate through a **C FFI boundary** (cgo). Go serializes blocks into binary, passes them to Rust, and Rust returns a 32-byte state root hash.

```
Ethereum Node
     │  WebSocket
     ▼
Block Streamer (Go)
     │  channel
     ▼
Worker Pool (Go) ──── 4 goroutines
     │  cgo FFI
     ▼
Rust State Engine ──── apply_block / rollback_to
     │
     ▼
State Root (Blake3 hash)

Reorg Engine (Go) ──── detects parent hash mismatch → triggers rollback

Metrics Server  :9090  ──── Prometheus /metrics, /health, /debug/gc, /debug/state
MCP Server      :8080  ──── 11 JSON-RPC tools for AI agent introspection
```

---

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | ≥ 1.23 | https://go.dev/dl |
| Rust + Cargo | ≥ 1.75 | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` |
| C compiler | any | macOS: Xcode CLT (`xcode-select --install`), Linux: `gcc` |

Verify:
```bash
go version        # go version go1.23+
rustc --version   # rustc 1.75+
cargo --version   # cargo 1.75+
```

---

## Project Structure

```
hybrid-runtime-blockchain-engine/
├── cmd/server/          # main.go — application entry point
├── internal/
│   ├── ffi/             # Go ↔ Rust bridge (cgo, serialization, validation)
│   ├── streamer/        # Ethereum WebSocket block streamer
│   ├── worker/          # Concurrent worker pool with panic recovery
│   ├── reorg/           # Chain reorganization detection and rollback
│   ├── metrics/         # Prometheus metrics + health/debug HTTP endpoints
│   ├── mcp/             # MCP server — 11 JSON-RPC introspection tools
│   ├── loadtest/        # Synthetic block generator and benchmark reporter
│   ├── config/          # Environment variable configuration
│   ├── logging/         # Structured zap logger
│   └── shutdown/        # Graceful shutdown manager
├── rust-core/           # Rust deterministic state engine
│   └── src/
│       ├── state.rs     # StateEngine: apply_block, rollback_to, state root
│       ├── ffi.rs       # C-exported FFI functions
│       └── types.rs     # Block, Transaction, Account, U256
├── docker/
│   ├── Dockerfile       # Multi-stage build (Rust → Go → debian-slim)
│   └── prometheus.yml   # Prometheus scrape config
├── Makefile
└── go.mod
```

---

## Step 1 — Build

### Build the Rust library first

```bash
cd rust-core
cargo build --release
cd ..
```

This produces `rust-core/target/release/librust_core.a` — the static library that Go links against.

### Build the Go binary

```bash
CGO_ENABLED=1 go build -o bin/hybrid-runtime-blockchain-engine ./cmd/server
```

Or use the Makefile (does both steps):

```bash
make build
```

---

## Step 2 — Configuration

All configuration is via environment variables. No config files.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ETH_RPC_URL` | **Yes** | — | Ethereum WebSocket endpoint, e.g. `wss://mainnet.infura.io/ws/v3/YOUR-KEY` |
| `WORKER_COUNT` | No | `4` | Number of parallel block processing goroutines (1–256) |
| `METRICS_PORT` | No | `9090` | Port for Prometheus metrics server |
| `MCP_PORT` | No | `8080` | Port for MCP JSON-RPC server |
| `LOAD_TEST_ENABLED` | No | `false` | Enable synthetic load testing mode (no real Ethereum node needed) |

---

## Step 3 — Run

### With a real Ethereum node

```bash
export ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-API-KEY"
export WORKER_COUNT=4
./bin/hybrid-runtime-blockchain-engine
```

### Without a real Ethereum node (load test mode)

```bash
ETH_RPC_URL=ws://localhost:8545 \
LOAD_TEST_ENABLED=true \
./bin/hybrid-runtime-blockchain-engine
```

The streamer will fail to connect (expected), log a warning, and the rest of the system starts normally. You can then trigger load tests via the MCP server.

### Expected startup output

```json
{"level":"info","message":"Starting Hybrid Runtime Blockchain Engine","worker_count":4}
{"level":"info","message":"FFI layer initialized"}
{"level":"info","message":"Rust core initialized"}
{"level":"info","message":"Metrics Collector started","port":9090}
{"level":"info","message":"registered MCP tool","tool":"get_gc_stats"}
... (11 tools registered)
{"level":"info","message":"MCP Server started","port":8080}
{"level":"info","message":"Worker Pool started","worker_count":4}
{"level":"warn","message":"Block Streamer failed to connect (load test mode)"}
{"level":"info","message":"All components started successfully"}
```

---

## Step 4 — Verify the Running System

### Health check

```bash
curl http://localhost:9090/health
# OK                          ← system healthy
# Block streamer not connected ← streamer down (503)
```

### Prometheus metrics

```bash
curl http://localhost:9090/metrics | grep -E "^(worker|rust|reorg|blocks|goroutine)"
```

Key metrics:
- `goroutine_count` — active Go goroutines
- `worker_active_count` — workers currently processing blocks
- `blocks_processed_total` — total blocks processed since start
- `reorg_total` — total chain reorganizations detected
- `rust_apply_block_duration_seconds` — Rust state transition latency histogram

### GC debug endpoint

```bash
curl http://localhost:9090/debug/gc | python3 -m json.tool
```

```json
{
  "num_gc": 3,
  "pause_total_ns": 450000,
  "pause_ns": [150000, 160000, 140000],
  "last_gc": "2026-04-10T12:00:00Z"
}
```

### State debug endpoint

```bash
curl http://localhost:9090/debug/state | python3 -m json.tool
```

```json
{
  "block_number": 19500000,
  "state_root": "a1b2c3d4...",
  "state_size": 1500
}
```

---

## Step 5 — Use the MCP Tools

The MCP server at `http://localhost:8080` exposes 11 JSON-RPC tools. All requests use `POST` with `Content-Type: application/json`.

**Rate limit: 10 requests per minute per tool** (returns HTTP 429 when exceeded).

### Tool reference

#### 1. `get_gc_stats` — Go garbage collection statistics

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_gc_stats","params":{},"id":1}'
```

```json
{
  "result": {
    "num_gc": 42,
    "pause_total_ns": 1500000,
    "pause_ns": [12000, 15000, 13000],
    "last_gc": "2026-04-10T12:00:00Z"
  }
}
```

#### 2. `get_heap_usage` — Current heap memory

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_heap_usage","params":{},"id":1}'
```

```json
{
  "result": {
    "alloc_bytes": 5242880,
    "total_alloc_bytes": 104857600,
    "sys_bytes": 10485760,
    "heap_objects": 12345
  }
}
```

#### 3. `get_goroutine_count` — Active goroutines

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_goroutine_count","params":{},"id":1}'
```

```json
{"result": {"count": 12}}
```

#### 4. `get_latency_distribution` — Block processing latency percentiles

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_latency_distribution","params":{},"id":1}'
```

```json
{
  "result": {
    "p50_ms": 2.1,
    "p95_ms": 4.8,
    "p99_ms": 7.3,
    "max_ms": 12.5
  }
}
```

#### 5. `get_reorg_history` — Recent chain reorganizations

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_reorg_history","params":{"limit":5},"id":1}'
```

```json
{
  "result": {
    "reorgs": [
      {
        "timestamp": "2026-04-10T12:00:00Z",
        "fork_point": 19499998,
        "depth": 2,
        "rollback_duration_ms": 18.5
      }
    ]
  }
}
```

#### 6. `get_state_root` — Current Rust state root

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_state_root","params":{},"id":1}'
```

```json
{
  "result": {
    "state_root": "a1b2c3d4e5f6...",
    "block_number": 19500000
  }
}
```

#### 7. `get_state_size` — State entry count and memory

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_state_size","params":{},"id":1}'
```

```json
{
  "result": {
    "entry_count": 1500,
    "memory_bytes": 524288
  }
}
```

#### 8. `get_apply_block_latency` — Rust apply_block execution times

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_apply_block_latency","params":{"limit":50},"id":1}'
```

```json
{
  "result": {
    "latencies_ms": [1.2, 1.5, 1.3, 1.1],
    "mean_ms": 1.275,
    "stddev_ms": 0.15
  }
}
```

#### 9. `validate_determinism` — Verify state root consistency

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"validate_determinism","params":{"block_count":10},"id":1}'
```

```json
{
  "result": {
    "consistent": false,
    "blocks_checked": 0,
    "error": "validate_determinism not yet implemented: requires block history infrastructure"
  }
}
```

> Note: This is a placeholder. Full implementation requires storing block history for replay.

#### 10. `compare_gc_vs_core_latency` — GC impact analysis

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"compare_gc_vs_core_latency","params":{},"id":1}'
```

```json
{
  "result": {
    "go_latency_variance_ms": 16.0,
    "rust_latency_variance_ms": 0.04,
    "variance_ratio": 400.0
  }
}
```

A high `variance_ratio` (>10) means GC pauses are causing latency spikes in Go compared to the deterministic Rust core.

#### 11. `run_load_test` — Execute a synthetic load test

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"run_load_test","params":{"tps":1000,"duration":10},"id":1}'
```

Parameters:
- `tps` — target transactions per second (1–10000)
- `duration` — test duration in seconds (1–300)

```json
{
  "result": {
    "success": true,
    "message": "Load test completed: 1000 TPS for 10 seconds"
  }
}
```

> Requires `LOAD_TEST_ENABLED=true` at startup.

---

## Step 6 — Run Tests

### All tests (recommended)

```bash
CGO_ENABLED=1 go test ./... -v -race -timeout=120s
```

### Rust tests only

```bash
cd rust-core && cargo test
```

### Go tests only (no Rust library needed for most packages)

```bash
# Packages that don't need cgo
CGO_ENABLED=0 go test ./internal/config/... ./internal/logging/... \
  ./internal/shutdown/... ./internal/loadtest/... \
  ./internal/mcp/... ./internal/worker/... -v

# Packages that need the Rust library
CGO_ENABLED=1 go test ./internal/ffi/... ./internal/reorg/... \
  ./internal/metrics/... ./internal/streamer/... -v
```

### With coverage report

```bash
CGO_ENABLED=1 go test ./... -coverprofile=coverage.out -timeout=120s
go tool cover -func=coverage.out | tail -1   # total coverage
go tool cover -html=coverage.out -o coverage.html  # visual report
open coverage.html
```

Current coverage by package:

| Package | Coverage |
|---------|----------|
| shutdown | 100% |
| mcp | 94.1% |
| config | 93.6% |
| loadtest | 92.6% |
| logging | 90.9% |
| worker | 89.6% |
| ffi | 83.6% |
| streamer | 82.3% |
| reorg | 84.1% |
| metrics | 80.3% |

### Run benchmarks

```bash
# Go benchmarks
CGO_ENABLED=1 go test ./... -bench=. -benchmem -run=^$ -timeout=120s

# Rust benchmarks
cd rust-core && cargo bench
```

---

## Step 7 — Docker

### Build the image

```bash
docker build -t hybrid-runtime-blockchain-engine:latest -f docker/Dockerfile .
```

### Run with Docker

```bash
docker run -d \
  --name blockchain-engine \
  -e ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-KEY" \
  -e WORKER_COUNT=4 \
  -p 9090:9090 \
  -p 8080:8080 \
  hybrid-runtime-blockchain-engine:latest
```

### Run with Docker Compose (includes Prometheus)

```bash
ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-KEY" docker-compose up
```

This starts:
- The blockchain engine on ports 9090 and 8080
- Prometheus on port 9091 (scrapes the engine every 15s)

---

## Graceful Shutdown

Send `SIGTERM` or `SIGINT` (Ctrl+C). The engine will:

1. Stop accepting new blocks from the streamer
2. Finish processing all in-flight blocks (up to 30s timeout)
3. Log the final state root and block number
4. Stop the metrics and MCP HTTP servers

```json
{"level":"info","message":"Shutdown signal received","signal":"interrupt"}
{"level":"info","message":"Final state root","state_root":"a1b2c3..."}
{"level":"info","message":"Final stats","block_number":19500000,"state_size":1500}
{"level":"info","message":"Shutdown complete"}
```

---

## Troubleshooting

### Build fails: `library 'ws2_32' not found`

You're on macOS/Linux but the old Windows-only linker flags are active. The `ffi.go` file now uses platform-specific `#cgo` directives — make sure you have the latest version of `internal/ffi/ffi.go`.

### Build fails: `CGO_ENABLED` not set

Always build with `CGO_ENABLED=1`. The FFI layer requires cgo.

### Tests fail: `undefined: ffi.NewFFI`

You're running with `CGO_ENABLED=0`. The `ffi`, `reorg`, `metrics`, and `streamer` packages require cgo. Run with `CGO_ENABLED=1`.

### Server exits immediately: `Failed to start Block Streamer`

You don't have a local Ethereum node. Either:
- Set `LOAD_TEST_ENABLED=true` to run without a real node
- Point `ETH_RPC_URL` to a real endpoint (Infura, Alchemy, local Geth/Hardhat)

### Rate limit hit: HTTP 429

Each MCP tool allows 10 requests per minute. Wait 60 seconds for the window to reset, or restart the server.

### `cargo build` not found

Rust is installed but not on your PATH. Use the full path:
```bash
~/.cargo/bin/cargo build --release
```
Or add `~/.cargo/bin` to your PATH:
```bash
echo 'export PATH="$HOME/.cargo/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

---

## How the FFI Boundary Works

When a block arrives from Ethereum:

1. **Go serializes** the block into a compact binary format:
   ```
   [1 byte version][8 bytes block number][32 bytes parent hash]
   [8 bytes timestamp][4 bytes tx count][...transactions]
   ```

2. **Go calls Rust** via cgo, passing a pointer and length.

3. **Rust deserializes** the block, validates it, applies state transitions (balance updates with checked arithmetic, nonce increments), calculates a Blake3 state root hash, and saves a snapshot for rollback.

4. **Rust returns** a 32-byte state root pointer. Go copies it and calls `free_buffer` to release Rust-allocated memory.

5. **Go stores** the state root and updates metrics.

Memory ownership rule: **Go owns Go memory, Rust owns Rust memory**. Nothing is shared. All data is copied across the boundary.

---

## How Reorg Detection Works

The reorg engine maintains a **ring buffer** of the last 10 blocks. For each new block:

1. Compute `prevBlock.Hash()` (SHA-256 of block number + parent hash + timestamp).
2. Compare with `newBlock.ParentHash`.
3. If they match → normal sequential block, apply it.
4. If they don't match → scan the ring buffer backwards for the block whose hash equals `newBlock.ParentHash`. That's the **fork point**.
5. Call `ffi.RollbackTo(forkPoint)` — Rust restores state from its snapshot history.
6. Apply the new block on top of the rolled-back state.
7. Record the reorg event (fork point, depth, duration).

---

## Makefile Reference

```bash
make build      # Build Rust library + Go binary
make run        # Build and run with default env vars
make test       # Run Rust tests + Go tests with race detector
make bench      # Run Rust + Go benchmarks
make coverage   # Open HTML coverage report (run make test first)
make docker     # Build Docker image
make clean      # Remove all build artifacts
```
