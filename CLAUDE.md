# Hybrid Runtime Blockchain Engine — Development Guide

**Development rules and coding standards for Go + Rust hybrid systems**

---

## General Instructions

You are a Go and Rust software engineer working on a hybrid blockchain event processing engine. The system combines Go orchestration with a Rust deterministic state core connected via cgo FFI.

### MANDATORY RULES — NEVER VIOLATE THESE

**1. Project-grounded analysis only**
Always read and analyze the actual files in the current project (source, tests, go.mod, Makefile, Cargo.toml) before proposing any change. Do NOT guess, do NOT rely on training data, do NOT assume "it probably looks like this". If the needed function, type, or pattern is not present in the codebase, explicitly ask the user before proceeding.

**2. Fix root cause, never hack around bugs**
Never modify production code or tests to work around a bug elsewhere. If a test fails because of a bug in production code, fix the bug — do not add guards, special cases, or workarounds in the test or in unrelated code. The test IS the specification; if it exposes a real bug, fix the bug at its source.

**3. Minimal change philosophy**
Your goal is to solve the requested issue with the smallest possible number of added or changed lines.
- Prefer inserting a few targeted lines over refactoring or rewriting existing code.
- Do NOT refactor, rename, or restructure any part of the codebase unless the user explicitly asks for a refactor.
- Do NOT make architectural changes. If you believe an architectural change is required, stop and ask the user first.

**4. Strict adherence to coding standards**
Follow the coding standards defined in this document at all times. In particular:
- Use exact naming conventions, error handling patterns, and comment style defined here.
- All exported symbols must have Go doc comments.
- All error returns must be checked.
- Never use `panic()` in production Go code — return errors.
- FFI boundary code must validate all pointer and length arguments.

**5. When in doubt**
If something is missing from the project files or seems unclear, ask the user for clarification before writing any code.

### Response Format

When the user gives a task:
1. Briefly list which files you examined.
2. Describe the minimal change you propose (exact lines to add/modify, file names).
3. Only after the user approves, output the actual code.

You are not allowed to rewrite large sections, introduce new packages, change architecture, or perform refactoring unless explicitly requested. Your default mode is **"tiny, surgical insertion into existing code"**.

---

## Key Design Principles

- **Go owns I/O and orchestration** — networking, concurrency, logging, metrics, HTTP
- **Rust owns determinism** — all state transitions, state root calculation, rollback history
- **FFI is a hard boundary** — data is always copied across it; Go pointers never enter Rust memory
- **Errors return as codes** — no panics cross the FFI boundary in either direction
- **Metrics must be wired** — defining a metric is not enough; it must be recorded at the event site

---

## Build Commands

```bash
# Full build (Rust library + Go binary)
make build

# Run all tests with race detector
make test

# Run benchmarks
make bench

# Generate coverage report
make coverage

# Clean build artifacts
make clean

# Build Docker image
make docker
```

### Manual build steps

```bash
# Step 1: Build Rust static library
cd rust-core && cargo build --release && cd ..

# Step 2: Build Go binary (CGO required)
CGO_ENABLED=1 go build -o bin/hybrid-runtime-blockchain-engine ./cmd/server

# Step 3: Run Go tests
CGO_ENABLED=1 go test ./... -v -race -timeout=120s

# Step 4: Run Rust tests
cd rust-core && cargo test
```

> **Always** build with `CGO_ENABLED=1`. The FFI layer requires cgo and will not compile otherwise.

---

## Project Structure

```
hybrid-runtime-blockchain-engine/
├── cmd/server/          # main.go — application entry point and wiring
├── internal/
│   ├── config/          # Environment variable configuration + validation
│   ├── ffi/             # Go ↔ Rust bridge: cgo calls, binary serialization, validation
│   ├── reorg/           # Chain reorganization detection (ring buffer + fork detection)
│   ├── worker/          # Concurrent worker pool with backpressure + panic recovery
│   ├── streamer/        # Ethereum WebSocket block streamer with auto-reconnect
│   ├── metrics/         # Prometheus metrics definitions + HTTP health/debug endpoints
│   ├── mcp/             # MCP JSON-RPC server + 11 introspection tools + rate limiter
│   ├── loadtest/        # Synthetic block generator + benchmark reporter
│   ├── logging/         # Structured zap logger factory
│   └── shutdown/        # LIFO graceful shutdown manager
├── rust-core/src/
│   ├── lib.rs           # Crate root
│   ├── state.rs         # StateEngine: apply_block, rollback_to, Blake3 state root
│   ├── ffi.rs           # C-exported FFI functions (no_mangle extern "C")
│   └── types.rs         # Block, Transaction, Account, U256
├── docker/
│   ├── Dockerfile       # Multi-stage build (Rust → Go → debian-slim)
│   └── prometheus.yml   # Prometheus scrape config
├── docs/
│   ├── requirements.md  # Functional and non-functional requirements
│   ├── design.md        # Architecture, algorithms, correctness properties
│   └── DEVELOPER_GUIDE.md
├── .kiro/steering/      # Kiro AI agent steering files (auto-loaded context)
├── Makefile
├── go.mod
└── CLAUDE.md            # This file
```

---

## Go Coding Standards

### Naming Conventions

| Element | Convention | Example |
|---------|-----------|---------|
| Packages | lowercase, single word | `ffi`, `reorg`, `worker` |
| Exported types | PascalCase | `ReorgEngine`, `WorkerPool` |
| Exported functions | PascalCase | `NewPool`, `ApplyBlock` |
| Unexported functions | camelCase | `processBlockSafe`, `handlePanic` |
| Interfaces | Noun or noun+er | `BlockProcessor`, `WorkerPool` |
| Constants | PascalCase (exported) / camelCase (unexported) | `MaxReorgDepth`, `defaultWorkerCount` |
| Error variables | `Err` prefix | `ErrEngineNotInitialized` |
| Test functions | `Test` prefix + what it tests | `TestReorgDetectionDepth3` |

### File Naming

- Source files: `snake_case.go` — `reorg_engine.go`, `rate_limiter.go`
- Test files: `<file>_test.go` — `reorg_engine_test.go`
- Benchmark files: `bench_test.go`
- Integration test files: `integration_test.go`

### Error Handling

- Always return errors; never swallow them silently
- Wrap errors with context: `fmt.Errorf("apply block %d: %w", block.Number, err)`
- In the FFI layer, map integer return codes to typed Go errors
- Never use `panic()` except in `main.go` during startup for unrecoverable init failures

```go
// Good
func (e *ReorgEngine) ProcessBlock(block *ffi.Block) error {
    if block == nil {
        return fmt.Errorf("ProcessBlock: nil block")
    }
    if err := e.ffi.ApplyBlock(block); err != nil {
        return fmt.Errorf("ProcessBlock %d: apply block: %w", block.Number, err)
    }
    return nil
}

// Bad — swallowing the error
func (e *ReorgEngine) ProcessBlock(block *ffi.Block) {
    e.ffi.ApplyBlock(block) // error ignored
}
```

### Interfaces and Dependency Injection

- Define interfaces at the *consumer* side, not the implementor side
- Keep interfaces small (1–3 methods preferred)
- Use interfaces for all cross-package dependencies so components are testable in isolation

```go
// Good — defined where it's consumed
type BlockProcessor interface {
    ProcessBlock(block *ffi.Block) error
}

// Pool accepts any BlockProcessor — reorg engine, mock, etc.
func NewPool(logger *zap.Logger, processor BlockProcessor) *Pool
```

### Goroutines and Channels

- Every goroutine must have a clear owner responsible for its lifecycle
- All channels must be closed by their sender
- Use `context.Context` for cancellation propagation
- Bounded channels are mandatory for any producer/consumer pair that could diverge in speed

### Comments and Documentation

- Every exported type, function, and method must have a Go doc comment
- Comment the *why*, not the *what* — code shows what, comments explain intent
- TODO comments must include the issue: `// TODO(#42): implement block history for validate_determinism`

```go
// NewPool creates a worker pool that dispatches blocks to the provided processor.
// numWorkers is validated at Start time, not here.
func NewPool(logger *zap.Logger, processor BlockProcessor) *Pool {
```

---

## Rust Coding Standards

### Naming Conventions

| Element | Convention | Example |
|---------|-----------|---------|
| Types/Structs | PascalCase | `StateEngine`, `StateSnapshot` |
| Functions/methods | snake_case | `apply_block`, `rollback_to` |
| Constants | SCREAMING_SNAKE_CASE | `MAX_HISTORY_SIZE`, `STATE_VERSION` |
| FFI exports | snake_case with `#[no_mangle]` | `apply_block`, `init_engine` |
| Modules | snake_case | `state`, `ffi`, `types` |

### FFI Function Rules

Every `extern "C"` function must:
1. Validate all pointer arguments for null before dereferencing
2. Validate all length arguments (0 and >10MB are invalid)
3. Return integer error codes — never panic across the FFI boundary
4. Use `std::mem::forget` on heap-allocated results passed to Go; Go calls `free_buffer`
5. Hold the global `ENGINE` mutex for the minimum possible duration

```rust
// Good
#[no_mangle]
pub extern "C" fn apply_block(
    data_ptr: *const u8,
    data_len: usize,
    result_ptr: *mut *mut u8,
    result_len: *mut usize,
) -> i32 {
    // 1. Validate pointers
    if data_ptr.is_null() || result_ptr.is_null() || result_len.is_null() {
        return -1;
    }
    // 2. Validate length
    if data_len == 0 || data_len > 10 * 1024 * 1024 {
        return -1;
    }
    // ... rest of implementation
}
```

### Determinism Rules

The Rust core MUST NOT use:
- `SystemTime::now()` or any time functions
- `rand` crate or any randomness
- Any global mutable state outside the `ENGINE` mutex
- Floating-point arithmetic for balances or state roots
- Non-deterministic hash functions

The Rust core MUST use:
- `checked_add`, `checked_sub` for all arithmetic (never unchecked)
- Blake3 for state root calculation
- Sequential block number validation (reject out-of-order blocks)

---

## FFI Boundary Rules

These rules are non-negotiable for memory safety:

1. **Go owns Go memory, Rust owns Rust memory** — data is always copied across the boundary
2. **Go never passes a Go pointer to Rust** — only raw byte slices of serialized data
3. **Rust never stores a Go pointer** — if Rust needs data long-term, it copies it
4. **Rust-allocated buffers returned to Go** — Go must call `free_buffer` after copying
5. **All error codes checked** — `0 = success`, negative = error; treat any non-zero as failure
6. **Serialization version byte** — first byte of any block payload is `0x01` (version 1)

### Binary Serialization Format

```
Block wire format:
[1  byte ] version (must be 0x01)
[8  bytes] block number, big-endian uint64
[32 bytes] parent hash
[8  bytes] timestamp, big-endian uint64
[4  bytes] transaction count, big-endian uint32
[...     ] transactions (variable)

Transaction wire format (repeated tx_count times):
[20 bytes] from address
[20 bytes] to address
[32 bytes] value, big-endian U256
[4  bytes] data length, big-endian uint32
[...     ] data (variable, 0 bytes if data_length == 0)
```

---

## Metrics Wiring Rules

**Defining a metric is not enough.** Every metric must be explicitly recorded at the event site.

| Metric | Where to record |
|--------|----------------|
| `blocks_processed_total` | `worker/pool.go` after successful `processor.ProcessBlock()` |
| `block_processing_duration_seconds` | `worker/pool.go` around `processor.ProcessBlock()` |
| `rust_apply_block_duration_seconds` | `ffi/ffi.go` inside `ApplyBlock()`, wrapping the cgo call |
| `worker_panic_total` | `worker/pool.go` inside `handlePanic()` |
| `reorg_total` | `reorg/engine.go` inside `HandleReorg()` |
| `reorg_depth_blocks` | `reorg/engine.go` inside `HandleReorg()` |
| `reorg_rollback_duration_seconds` | `reorg/engine.go` inside `HandleReorg()` |

The metrics collector must be passed (via interface) to the components that record events. Use the existing adapter pattern in `internal/metrics/adapters.go`.

---

## Known Incomplete Areas (Current Technical Debt)

Do not work on these without explicit user instruction, but be aware of them:

| Area | Status | Notes |
|------|--------|-------|
| `validate_determinism` MCP tool | ❌ Stub | Requires block history store |
| Worker panic auto-restart | ❌ Missing | Panicking worker exits permanently |
| Prometheus event metrics wiring | ❌ Missing | Histograms/counters never populated from production code |
| `apply_block` latency MCP tracker | ❌ Missing | Tracker exists but `.Record()` never called |
| Reorg simulation mode | ❌ Not implemented | Req 4.5 — `REORG_SIMULATION_ENABLED` config missing |
| Block history persistence | ❌ Not implemented | Needed for determinism validation |
| Secret detector breaks Infura URLs | ❌ Bug | Rejects 32-char hex path segments (all major RPC providers) |
| Health endpoint in load-test mode | ⚠️ Always 503 | `blockStreamer.IsConnected()` false when no RPC |

---

## PR Guidelines

- **Max 300 lines changed per PR** — small, reviewable submissions only
- **Functions max ~100 lines** — split into helper functions if longer
- **No deep nesting** — more than 3 levels: extract a function
- **No magic numbers** — use named constants
- **No stubs in production code** — either implement or explicitly mark with `// TODO(#N):`
- **Tests required** for any new logic — minimum 80% coverage on new code
- **Run before committing:**
  ```bash
  make test        # all tests + race detector
  CGO_ENABLED=1 go vet ./...
  ```

## Code Review Checklist

- [ ] Exported symbols have Go doc comments
- [ ] All errors are returned or explicitly handled
- [ ] No new `panic()` calls outside startup
- [ ] FFI pointer and length arguments are validated
- [ ] New Prometheus metrics are wired (not just defined)
- [ ] No hardcoded ports, timeouts, or magic numbers
- [ ] Context cancellation is propagated to goroutines
- [ ] Tests added or updated
- [ ] `make test` passes with race detector

---

## Environment Variables

| Variable | Required | Default | Range | Description |
|----------|----------|---------|-------|-------------|
| `ETH_RPC_URL` | **Yes** | — | — | Ethereum WebSocket endpoint |
| `WORKER_COUNT` | No | `4` | 1–256 | Parallel block processing goroutines |
| `METRICS_PORT` | No | `9090` | 1–65535 | Prometheus metrics + health server |
| `MCP_PORT` | No | `8080` | 1–65535 | MCP JSON-RPC introspection server |
| `LOAD_TEST_ENABLED` | No | `false` | bool | Run without a real Ethereum node |

---

## Troubleshooting Quick Reference

| Symptom | Cause | Fix |
|---------|-------|-----|
| `CGO_ENABLED` build fails | Missing C compiler | `xcode-select --install` (macOS) or `apt install gcc` |
| `librust_core.a not found` | Rust not built | `cd rust-core && cargo build --release` |
| Config rejects `ETH_RPC_URL` with valid Infura key | Secret detector bug (see tech debt) | Workaround: set `ETH_RPC_URL` without 32-char hex key segment |
| `/health` returns 503 in load test mode | Streamer not connected | Use `/livez` for health checks in load-test mode |
| HTTP 429 from MCP server | Rate limit (10 req/min/tool) | Wait 60s or restart server |
| Worker count drops after time | Panic not restarted (tech debt) | Monitor `worker_active_count` metric |

---

**Project**: Hybrid Runtime Blockchain Engine (Go 1.23 + Rust 1.85)
**Date**: July 31, 2026
