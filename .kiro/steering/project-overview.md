---
inclusion: auto
---

# Project: Hybrid Runtime Blockchain Engine

## What This Project Does

A production-grade blockchain event processor that streams Ethereum blocks in real time, detects chain reorganizations, and maintains deterministic state. It combines two runtimes:

- **Go** (`internal/`, `cmd/`) — I/O, concurrency, networking, observability, HTTP servers
- **Rust** (`rust-core/`) — deterministic state transitions, Blake3 state root, rollback history

They communicate through a **C FFI boundary** (cgo). Go serializes blocks into a binary format, calls Rust via `apply_block`, and Rust returns a 32-byte state root.

## Architecture at a Glance

```
Ethereum Node (WebSocket)
       │
       ▼
Block Streamer (Go)       ← auto-reconnect, exponential backoff
       │  channel (bounded, backpressure)
       ▼
Worker Pool (Go)          ← 1–256 goroutines, panic recovery
       │
       ▼
Reorg Engine (Go)         ← ring buffer, fork detection, rollback coordination
       │  cgo FFI (binary serialization)
       ▼
Rust State Engine         ← apply_block / rollback_to / Blake3 state root
       │
       ▼
Observability             ← Prometheus :9090 | MCP Server :8080 | Structured Logging
```

## Package Map

| Package | Path | Purpose |
|---------|------|---------|
| config | `internal/config/` | Env var loading and validation |
| ffi | `internal/ffi/` | cgo bridge, binary serialization, input validation |
| reorg | `internal/reorg/` | Chain reorg detection and rollback coordination |
| worker | `internal/worker/` | Concurrent block processing pool |
| streamer | `internal/streamer/` | Ethereum WebSocket block streamer |
| metrics | `internal/metrics/` | Prometheus metrics + health/debug HTTP |
| mcp | `internal/mcp/` | MCP JSON-RPC server + 11 introspection tools |
| loadtest | `internal/loadtest/` | Synthetic block generator + benchmark reporter |
| logging | `internal/logging/` | Structured zap logger factory |
| shutdown | `internal/shutdown/` | LIFO graceful shutdown manager |
| rust-core | `rust-core/src/` | Deterministic state engine (Rust) |

## Key Files to Know

| File | Why it matters |
|------|---------------|
| `cmd/server/main.go` | Component wiring and startup order |
| `internal/ffi/ffi.go` | All cgo calls + platform LDFLAGS |
| `internal/ffi/serialization.go` | Binary block serialization format |
| `internal/reorg/engine.go` | Ring buffer + fork detection algorithm |
| `internal/mcp/tools.go` | All 11 MCP tool implementations |
| `internal/metrics/collector.go` | All Prometheus metric definitions |
| `rust-core/src/state.rs` | Core state transitions + Blake3 root |
| `rust-core/src/ffi.rs` | C-exported FFI functions |

## Runtime Split — The Golden Rule

> If it touches **state transitions** (balances, nonces, state root) → it goes in **Rust**.
> Everything else → **Go**.

This separation is the entire value proposition of the project. Never blur this boundary.

## Known Technical Debt (Do Not Fix Without Explicit Instruction)

| Issue | Location |
|-------|----------|
| Worker panic doesn't restart worker | `internal/worker/pool.go` |
| Prometheus event metrics not wired | `internal/metrics/collector.go` + event sites |
| `validate_determinism` MCP tool is a stub | `internal/mcp/tools.go:394` |
| `apply_block` MCP latency tracker unpopulated | `internal/mcp/tools.go` |
| Secret detector rejects valid Infura/Alchemy URLs | `internal/config/config.go` |
| Reorg simulation mode not implemented | Req 4.5 — `REORG_SIMULATION_ENABLED` missing |
| No block history persistence | Needed for `validate_determinism` |
| `/health` returns 503 in load-test mode | `internal/metrics/collector.go` |
