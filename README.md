# Hybrid Runtime Blockchain Engine

A blockchain event processing engine that combines **Go** for orchestration with **Rust** for deterministic state execution. Streams Ethereum blocks in real-time, detects chain reorganizations, and maintains verifiable state with predictable latency.

[![CI](https://github.com/Subaskar-S/hybrid-runtime-blockchain-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/Subaskar-S/hybrid-runtime-blockchain-engine/actions/workflows/ci.yml)

## Why Hybrid Runtime?

Blockchain state transitions must be **deterministic** — the same inputs must always produce the same outputs. Go's garbage collector introduces unpredictable pauses that break this guarantee. Our solution:

- **Go** handles I/O, concurrency, networking, and observability
- **Rust** handles state transitions with zero GC, zero non-determinism

The two communicate through a C FFI boundary with binary serialization.

## Architecture

```
Ethereum Node (WebSocket)
       │
       ▼
Block Streamer (Go) ─── auto-reconnect with exponential backoff
       │
       ▼
Worker Pool (Go) ─────── 1-256 goroutines, backpressure, panic recovery
       │
       ▼
Reorg Engine (Go) ────── ring buffer, fork detection, rollback coordination
       │  cgo FFI
       ▼
Rust State Engine ────── apply_block / rollback_to / Blake3 state root
       │
       ▼
Observability ─────────── Prometheus :9090 │ MCP Server :8080 │ Structured Logging
```

## Quick Start

### Prerequisites

- Go ≥ 1.23
- Rust ≥ 1.85
- C compiler (Xcode CLT on macOS, gcc on Linux)

### Build & Run

```bash
make build
ETH_RPC_URL="wss://mainnet.infura.io/ws/v3/YOUR-KEY" make run
```

### Load Test Mode (no Ethereum node needed)

```bash
ETH_RPC_URL=ws://localhost:8545 LOAD_TEST_ENABLED=true make run
```

### Docker

```bash
docker compose up
```

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ETH_RPC_URL` | Yes | — | Ethereum WebSocket endpoint |
| `WORKER_COUNT` | No | `4` | Parallel block processing goroutines (1–256) |
| `METRICS_PORT` | No | `9090` | Prometheus metrics + health endpoints |
| `MCP_PORT` | No | `8080` | MCP JSON-RPC introspection server |
| `LOAD_TEST_ENABLED` | No | `false` | Run without a real Ethereum node |

## Endpoints

| Endpoint | Port | Description |
|----------|------|-------------|
| `GET /metrics` | 9090 | Prometheus metrics |
| `GET /health` | 9090 | Health check (200/503) |
| `GET /livez` | 9090 | Kubernetes liveness probe |
| `GET /readyz` | 9090 | Kubernetes readiness probe |
| `GET /debug/gc` | 9090 | GC statistics (JSON) |
| `GET /debug/state` | 9090 | State root and size (JSON) |
| `POST /` | 8080 | MCP JSON-RPC (11 tools) |

## MCP Tools

The MCP server exposes 11 runtime introspection tools via JSON-RPC:

```bash
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"get_gc_stats","params":{},"id":1}'
```

| Tool | Description |
|------|-------------|
| `get_gc_stats` | Go GC pause times and counts |
| `get_heap_usage` | Heap memory allocation |
| `get_goroutine_count` | Active goroutines |
| `get_latency_distribution` | Block processing p50/p95/p99/max |
| `get_reorg_history` | Recent chain reorganizations |
| `get_state_root` | Current Blake3 state root hash |
| `get_state_size` | State entry count and memory |
| `get_apply_block_latency` | Rust core execution times |
| `validate_determinism` | State root consistency check |
| `compare_gc_vs_core_latency` | GC impact analysis |
| `run_load_test` | Execute synthetic load test |

Rate limited to 10 requests/minute per tool. Binds to localhost only.

## Project Structure

```
├── cmd/server/          Main entry point
├── internal/
│   ├── ffi/             Go ↔ Rust bridge (cgo, serialization, validation)
│   ├── streamer/        Ethereum WebSocket block streamer with auto-reconnect
│   ├── worker/          Concurrent worker pool with panic recovery
│   ├── reorg/           Chain reorganization detection and rollback
│   ├── metrics/         Prometheus metrics + health/debug HTTP endpoints
│   ├── mcp/             MCP server — 11 JSON-RPC introspection tools
│   ├── loadtest/        Synthetic block generator and benchmark reporter
│   ├── config/          Environment variable configuration with validation
│   ├── logging/         Structured zap logger
│   └── shutdown/        Graceful shutdown manager (LIFO ordering)
├── rust-core/src/
│   ├── state.rs         StateEngine: apply_block, rollback_to, Blake3 root
│   ├── ffi.rs           C-exported FFI functions
│   └── types.rs         Block, Transaction, Account, U256
├── docker/
│   ├── Dockerfile       Multi-stage build (Rust → Go → debian-slim)
│   └── prometheus.yml   Prometheus scrape config
├── .github/workflows/
│   ├── ci.yml           CI pipeline (test, build, docker)
│   └── auto-approve.yml Auto-approve PRs after CI passes
└── Makefile
```

## Development

```bash
make build      # Build Rust + Go
make test       # Run all tests with race detector
make bench      # Run benchmarks
make coverage   # Generate HTML coverage report
make clean      # Remove build artifacts
```

### Test Coverage

| Package | Coverage |
|---------|----------|
| shutdown | 100% |
| mcp | 94% |
| config | 93% |
| loadtest | 92% |
| logging | 90% |
| worker | 89% |
| reorg | 86% |
| ffi | 83% |
| streamer | 82% |
| metrics | 80% |

## Key Design Decisions

- **Bounded rollback history** — Rust keeps only the last 10 state snapshots (matching max reorg depth), preventing unbounded memory growth
- **Binary serialization** — Custom binary format with version byte, not JSON, for FFI performance
- **Backpressure** — Worker pool uses bounded channels (2× worker count) to prevent memory exhaustion
- **Panic isolation** — Go worker panics are recovered; Rust errors return codes without crossing the FFI boundary
- **Auto-reconnection** — Block streamer reconnects with exponential backoff on WebSocket disconnects

## Documentation

- [Developer Guide](docs/DEVELOPER_GUIDE.md) — Setup, build, run, test, troubleshoot
- [Design Document](docs/design.md) — Architecture, algorithms, correctness properties
- [Requirements](docs/requirements.md) — Functional and non-functional requirements

## License

MIT
