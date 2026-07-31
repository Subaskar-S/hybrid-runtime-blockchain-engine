---
inclusion: manual
---

# Architecture Decisions

This file explains the *why* behind the major design choices. Read this before proposing architectural changes.

---

## ADR-1: Hybrid Go + Rust Runtime

**Decision**: Use Go for orchestration and Rust for state transitions, connected via cgo FFI.

**Why not pure Go?**
Go's garbage collector introduces non-deterministic pauses. These pauses can last several milliseconds and occur at unpredictable times. Blockchain state transitions must produce identical results given identical inputs — including identical timing. A GC pause during state computation would make this guarantee impossible.

**Why not pure Rust?**
Rust's async ecosystem (tokio) and concurrency primitives are powerful but more complex than Go's goroutines and channels. Go excels at I/O-bound orchestration (WebSocket, HTTP, fan-out). We use each language where it wins.

**Tradeoff accepted**: cgo adds overhead (roughly 100ns per call) and increases build complexity. This is acceptable because: (a) we call FFI per block, not per transaction; (b) build complexity is managed via a two-step Makefile.

---

## ADR-2: Binary Serialization over JSON for FFI

**Decision**: Block data crosses the FFI boundary as a binary format with a version byte, not JSON.

**Why not JSON?**
JSON encoding/decoding adds ~5–10µs per block. At 1000 blocks/second, that's 5–10ms of pure serialization overhead. The binary format is measured at ~200ns per block.

**Version byte**: The first byte is always `0x01`. If the format ever changes, we bump to `0x02` and support both versions in Rust's deserializer. This allows rolling upgrades without breaking deployed state.

**Tradeoff accepted**: The binary format is harder to debug. Compensated by the `internal/ffi/serialization.go` helpers and unit tests that document the format.

---

## ADR-3: Ring Buffer for Reorg Detection (max depth 10)

**Decision**: The reorg engine keeps exactly the last 10 blocks in a fixed-size ring buffer. Reorgs deeper than 10 blocks are rejected as critical errors.

**Why 10?**
Ethereum's practical maximum reorg depth is 2–3 blocks. The theoretical maximum for a "selfish mining" attack is higher but not seen in production. 10 provides a comfortable safety margin without unbounded memory growth.

**Why a ring buffer, not a slice?**
Fixed memory allocation. The ring buffer allocates exactly `10 × sizeof(Block)` and never grows. This prevents memory exhaustion under adversarial conditions.

**Why match the Rust history size?**
The Go ring buffer and Rust `MAX_HISTORY_SIZE = 10` are deliberately identical. This ensures that for any reorg depth Go can detect, Rust has a snapshot to roll back to.

**Tradeoff accepted**: Reorgs deeper than 10 blocks cause a halt. This is correct behavior — a >10-block reorg indicates an adversarial chain or severely partitioned network. Continuing with incorrect state is worse than stopping.

---

## ADR-4: Bounded Channel for Backpressure (2× worker count)

**Decision**: The worker pool channel capacity is `2 × numWorkers`. `Submit()` blocks (not drops) when full.

**Why block instead of drop?**
Dropping blocks would leave gaps in state history. The state engine requires sequential block numbers — a dropped block at height N means blocks N+1, N+2... all have wrong parent hashes and would be rejected.

**Why 2×?**
Allows workers to stay busy while the streamer fetches the next block, without allowing the queue to grow unboundedly. Empirically, 2× keeps workers at ~95% utilization with no queue depth growth.

**Tradeoff accepted**: If processing falls behind ingestion, the block streamer will also back up. This is intentional — it propagates pressure back to the Ethereum WebSocket connection, which naturally paces at Ethereum's 12-second block time.

---

## ADR-5: LIFO Shutdown Order

**Decision**: Components are stopped in reverse registration order (LIFO): MCP → Metrics → Worker Pool → Block Streamer.

**Why LIFO?**
Components depend on each other: the worker pool depends on the reorg engine which depends on the FFI layer. Stopping in reverse dependency order ensures that a component is not stopped while something still depends on it.

**Why not a dependency graph?**
LIFO is simpler and sufficient for a linear dependency chain. A DAG-based shutdown would add complexity without benefit here.

---

## ADR-6: MCP Server Binds Only to Localhost

**Decision**: The MCP server listens on `127.0.0.1`, never `0.0.0.0`.

**Why?**
The MCP tools expose internal runtime state (GC pauses, memory layout, state root, goroutine count). This is appropriate for AI agent introspection on the same host, not for public exposure.

**If you need remote access**: put the engine behind a reverse proxy (nginx, Envoy) that handles authentication and TLS termination. Never change the MCP bind address to `0.0.0.0` without adding authentication first.

---

## ADR-7: No Config File — Environment Variables Only

**Decision**: All configuration comes from environment variables. No YAML/JSON/TOML config files.

**Why?**
Environment variables are the 12-factor app standard for containerized services. They work identically in Docker, Kubernetes, bare metal, and CI. A config file would require volume mounting in Docker and secret management in Kubernetes — more complexity for no benefit given the small number of config values (5 variables).

**Tradeoff accepted**: Cannot express complex structured config (nested settings, arrays). Currently not needed. If the config grows significantly, this decision should be revisited.

---

## ADR-8: Blake3 for State Root (Rust), SHA-256 for Block Hash (Go)

**Decision**: Rust uses Blake3 to compute the state root. Go uses SHA-256 to compute block hashes for reorg detection.

**Why different hash functions?**
The state root is computed in Rust and must be deterministic and fast — Blake3 is ideal. Block hashes for reorg detection in Go would ideally also use Blake3, but Blake3 is not in the Go standard library and using it would require cgo. Since block hashes are only used for parent-chain verification (not for state root equality), SHA-256 from the standard library is used instead.

**Implication**: A Go-computed block hash will never equal a Rust-computed block hash for the same block. The two hash systems are self-consistent within their respective runtimes but not interchangeable.

**This is a documented divergence, not a bug.** If you ever need to compare Go and Rust hashes for the same block, they will not match.

---

## What Not to Change Without Discussion

These decisions have deliberate tradeoffs. If you believe they need changing, write up the proposed change and discuss it first:

1. The FFI binary format — any change requires a version bump and backward-compatible deserializer
2. The ring buffer size (10) — must stay equal to Rust `MAX_HISTORY_SIZE`
3. The MCP bind address — security-sensitive
4. The LIFO shutdown order — component lifecycle depends on this ordering
5. The use of cgo — removing it would eliminate the Rust integration entirely
