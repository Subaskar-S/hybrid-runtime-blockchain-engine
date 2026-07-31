# Contributing to Hybrid Runtime Blockchain Engine

## Before You Start

Read **CLAUDE.md** first — it defines the mandatory coding rules, naming conventions, and the known incomplete areas of this project. This document covers the workflow and process side.

---

## Development Workflow

### 1. Understand the Component Boundary

This project has two runtimes that must stay separated:

- **Go** (`internal/`, `cmd/`) — I/O, orchestration, observability, HTTP
- **Rust** (`rust-core/`) — deterministic state transitions, state root, rollback history

Before adding code, decide which side it belongs on. **When in doubt: if it touches state transitions, it goes in Rust. Everything else stays in Go.**

### 2. Build First, Then Change

Always verify the project builds cleanly before making changes:

```bash
make build
make test
```

If either fails on an unmodified checkout, resolve that first and report the issue.

### 3. Understand the Affected Component

Read the relevant source files before writing anything:

```bash
# Example: working on the reorg engine
# Read: internal/reorg/engine.go, internal/reorg/engine_test.go
# Also read: internal/ffi/ffi.go (for the FFI calls it makes)
```

### 4. Write Tests Before Implementation (for bug fixes)

When fixing a bug, write a failing test that reproduces it first. Then fix the bug. Confirm the test passes. This ensures the bug cannot regress.

### 5. Run the Full Test Suite

```bash
# Run everything — Go + race detector, Rust tests
make test

# Or manually:
CGO_ENABLED=1 go test ./... -v -race -timeout=120s
cd rust-core && cargo test
```

Never submit a PR with failing tests.

### 6. Check for Races

The `-race` flag is mandatory. Any data race is a bug, not a warning. The race detector is included in `make test`.

---

## Adding a New Go Package

1. Create `internal/<packagename>/` directory
2. Name the package to match the directory
3. Define interfaces before implementations — the consumer defines what it needs
4. Add tests in `internal/<packagename>/<file>_test.go`
5. Wire the new package into `cmd/server/main.go` — don't leave it disconnected

### Package dependency rules

Allowed dependency directions (no cycles):

```
cmd/server
    └── internal/config
    └── internal/logging
    └── internal/shutdown
    └── internal/ffi
    └── internal/reorg     (depends on ffi)
    └── internal/worker    (depends on ffi)
    └── internal/streamer
    └── internal/metrics   (depends on ffi, reorg, worker via adapters)
    └── internal/mcp       (depends on reorg, loadtest)
    └── internal/loadtest  (depends on worker, ffi)
```

**Prohibited**: `ffi` importing `metrics`, `metrics` importing `worker` directly, `reorg` importing `mcp`.

---

## Adding a New Rust Module

1. Create `rust-core/src/<module>.rs`
2. Declare it in `rust-core/src/lib.rs`: `mod <module>;`
3. If it needs FFI exports, add `#[no_mangle] pub extern "C"` functions to `rust-core/src/ffi.rs`
4. Add corresponding Go wrapper functions in `internal/ffi/ffi.go`
5. Add tests in the module file under `#[cfg(test)]`

---

## Adding a New MCP Tool

The MCP server exposes 11 JSON-RPC tools. To add a 12th:

1. Add the tool handler function in `internal/mcp/tools.go`:
   ```go
   func handleMyNewTool(params map[string]interface{}) (interface{}, error) {
       // implementation
   }
   ```

2. Register it in `RegisterRuntimeTools()`:
   ```go
   server.RegisterTool("my_new_tool", "Description of what it does", handleMyNewTool)
   ```

3. Add it to `RUNTIME_TOOLS.md`

4. Write a test in `internal/mcp/tools_test.go`

5. Rate limiting is automatic — the existing middleware applies to all registered tools.

**Do not implement stub tools.** If the tool cannot be fully implemented yet, do not register it. A placeholder that returns a hardcoded error is acceptable only when explicitly documented with a `// TODO(#N):` comment.

---

## Adding a New Prometheus Metric

Defining a metric is not enough — it must be wired to the event that produces it.

1. Define the metric in `internal/metrics/collector.go` (follow existing patterns)
2. Add the recording method: `func (c *Collector) RecordMyEvent(value float64)`
3. **Call the recording method** at the event site in the appropriate package
4. Since `metrics` cannot import `worker`/`reorg`/`ffi` (would create cycles), use the adapter pattern:
   - Add a method to the appropriate adapter interface in `internal/metrics/adapters.go`
   - Or pass a recording callback to the component via constructor

---

## Commit Message Format

```
<component>: <short imperative description>

<optional body explaining why, not what>

Fixes #<issue>  (if applicable)
```

Examples:
```
worker: restart goroutine after panic recovery

Previously a panicking worker would exit its for-range loop permanently,
silently reducing pool capacity. Now the worker goroutine relaunches itself
after calling handlePanic.

Fixes #12
```

```
config: allow 32-char hex segments in ETH_RPC_URL

The secret detector rejected valid Infura/Alchemy keys because they appear
as 32-char hex path segments. Now only rejects keys that look hardcoded
(present in source files), not values supplied via environment variable.
```

---

## PR Size Limits

| Constraint | Limit |
|-----------|-------|
| Lines changed per PR | ≤ 300 |
| Lines per function | ≤ 100 |
| Nesting depth | ≤ 3 levels |
| Files per PR | ≤ 10 (prefer focused changes) |

Break large features into sequential, reviewable PRs. Each PR should leave the system in a working state.

---

## Pre-Commit Checklist

Run these before every commit:

```bash
# 1. Build
make build

# 2. All tests + race detector
make test

# 3. Go vet
CGO_ENABLED=1 go vet ./...

# 4. Rust lints
cd rust-core && cargo clippy -- -D warnings
```

No test failures, no vet errors, no clippy warnings before pushing.

---

## Sensitive Areas

These areas require extra care — changes here affect system-wide correctness:

| Area | Risk | Extra steps |
|------|------|-------------|
| `internal/ffi/` | Memory safety, crashes | Always validate pointers + lengths; test with -race |
| `rust-core/src/state.rs` | Determinism guarantee | Never add time/rand calls; test round-trip property |
| `internal/reorg/engine.go` | State consistency | Test all ring buffer edge cases; verify rollback restores exact state |
| `internal/config/config.go` | Startup failure | Test all env var combinations; never break existing behavior |
| `cmd/server/main.go` | Component wiring | Changes here affect startup order and shutdown order |
| FFI serialization format | Breaking change | Version byte exists for this reason — bump it and handle both versions |

---

## Getting Help

- **Architecture questions**: Read `docs/design.md` first
- **Requirements questions**: Read `docs/requirements.md`
- **API reference**: `docs/DEVELOPER_GUIDE.md` has full MCP tool and endpoint docs
- **Known gaps**: See the technical debt table in `CLAUDE.md`
- **Build problems**: Check the Troubleshooting section in `CLAUDE.md`
