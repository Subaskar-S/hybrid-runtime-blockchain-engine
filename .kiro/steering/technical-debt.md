---
inclusion: manual
---

# Technical Debt Register

This file tracks known incomplete, broken, or placeholder code. Each item has a description of the problem, its impact, and what a correct implementation would look like.

**Do not fix items from this list without explicit user instruction.** Reference this file when asked about known issues.

---

## TD-1: Worker Panic Does Not Restart Worker (High)

**File**: `internal/worker/pool.go` — `processBlockSafe()` and `handlePanic()`

**Problem**: When a worker goroutine panics, `processBlockSafe` recovers the panic and calls `handlePanic` (which logs and increments `panicCount`). The goroutine then exits its `for range` loop and terminates. The pool's `wg` decrements permanently. `ActiveWorkers()` drops by one and never recovers.

The comment in the code says "the pool will restart it" — this is incorrect. There is no restart code.

**Impact**: Every panic permanently reduces processing capacity. After enough panics, the pool may have zero workers and stop processing blocks entirely. The `worker_panic_total` Prometheus metric is also never recorded (see TD-3).

**Correct fix**: In `handlePanic`, re-launch the worker goroutine:
```go
func (p *Pool) handlePanic(workerID int, r interface{}) {
    atomic.AddInt64(&p.panicCount, 1)
    p.logger.Error("worker panic — restarting",
        zap.Int("worker_id", workerID),
        zap.Any("panic", r),
        zap.String("stack", string(debug.Stack())))
    // Re-launch the worker so pool capacity is maintained
    p.wg.Add(1)
    go p.worker(context.Background(), workerID)
}
```

---

## TD-2: validate_determinism MCP Tool Is a Stub (Medium)

**File**: `internal/mcp/tools.go` — `handleValidateDeterminism()`

**Problem**: The function unconditionally returns:
```go
return map[string]interface{}{
    "consistent":     false,
    "blocks_checked": 0,
    "error":          "validate_determinism not yet implemented: requires block history infrastructure",
}, nil
```

**Impact**: The 11th MCP tool is non-functional. Any AI agent or operator calling it gets a misleading "not consistent" result.

**Root cause**: There is no block history store. The system processes blocks through the Rust state engine but never persists the raw block data in Go.

**Correct fix**: 
1. Add a block store (in-memory ring buffer or persistent) that retains the last N raw blocks
2. Implement the tool to: save current state root → rollback N blocks → replay them → compare final state root
3. This also requires wiring the block store into the reorg engine (which already has partial block history in its ring buffer)

---

## TD-3: Prometheus Event Metrics Never Wired (High)

**File**: `internal/metrics/collector.go` defines `RecordBlockProcessed`, `RecordRustApplyBlock`, `RecordReorg`, `RecordWorkerPanic` — but none are called from production code.

**Problem**: All histogram and counter metrics for actual events are zero in production. The collector only polls gauges (worker count, state size) every 5 seconds. Event-driven metrics (`blocks_processed_total`, `block_processing_duration_seconds`, `rust_apply_block_duration_seconds`, reorg metrics, panic counter) are structurally dead.

**Impact**: Prometheus dashboards will show all histogram/counter metrics as zero regardless of actual activity. Alerting based on these metrics is impossible.

**Correct fix**: Wire the recording calls into the event sites. The challenge is import cycles — `worker` cannot import `metrics` and `metrics` cannot import `worker`. Use callback injection or the existing adapter pattern:

```go
// Option A: callback injection
type Pool struct {
    onBlockProcessed func(duration time.Duration) // set by main.go
    onPanic          func()
}

// Option B: the adapter interfaces in metrics/adapters.go already handle polling;
// add an EventRecorder interface that event sites accept
type EventRecorder interface {
    RecordBlockProcessed(d time.Duration)
    RecordPanic()
}
```

---

## TD-4: apply_block Latency Tracker Never Populated (Medium)

**File**: `internal/mcp/tools.go` — `ApplyBlockLatencyTracker`

**Problem**: The `ApplyBlockLatencyTracker` is a circular buffer that `get_apply_block_latency` reads from. However, no production code ever calls `.Record()` on it. The tracker is created, registered, and read — but never written to.

**Impact**: `get_apply_block_latency` always returns empty arrays and zero means/stddev.

**Correct fix**: Wire `ffi.ApplyBlock()` to record timing to the tracker. Since MCP cannot import FFI directly, use a callback or the same EventRecorder pattern from TD-3.

---

## TD-5: Secret Detector Breaks Valid Infura/Alchemy URLs (High)

**File**: `internal/config/config.go` — `containsHardcodedSecret()`

**Problem**: The function rejects any URL path segment that is ≥32 hex characters. Infura, Alchemy, QuickNode, and every other major Ethereum RPC provider uses exactly this format: `wss://mainnet.infura.io/ws/v3/<32-char-hex-key>`. Setting `ETH_RPC_URL` to a valid provider URL via environment variable causes the system to refuse to start.

**Impact**: The primary documented usage pattern is broken. `README.md` shows this exact URL format as the example, but it would be rejected immediately.

**Correct fix**: The check should only reject keys that appear to be **hardcoded in source code** (via static analysis or a separate linting step), not values provided via environment variable at runtime. The simplest runtime fix: remove the check entirely and add a note that secret scanning should happen in CI (e.g., `gitleaks`, `trufflehog`), not at runtime.

---

## TD-6: Reorg Simulation Mode Not Implemented (Low)

**File**: Missing — `internal/reorg/engine.go` and `internal/config/config.go`

**Requirement**: Req 4.5: "WHERE reorg simulation mode is enabled, THE Reorg_Engine SHALL inject synthetic reorgs at configurable intervals for testing."

**Problem**: No `REORG_SIMULATION_ENABLED` env var exists, no simulation logic exists anywhere.

**Impact**: Cannot test reorg handling in a controlled environment without constructing specific block sequences. Load tests cannot measure reorg overhead.

**Correct fix**:
1. Add `ReorgSimulationEnabled bool` and `ReorgSimulationIntervalBlocks int` to `Config`
2. In `ReorgEngine.ProcessBlock`, when simulation is enabled and `blocksSinceLastReorg >= interval`, inject a synthetic fork by creating a competing block with the same number but different hash

---

## TD-7: /health Always Returns 503 in Load-Test Mode (Low)

**File**: `internal/metrics/collector.go` — health handler

**Problem**: `/health` returns 503 when `!blockStreamer.IsConnected()`. In `LOAD_TEST_ENABLED=true` mode, the block streamer intentionally fails to connect. So `/health` always returns 503 in this mode, even when the system is fully operational.

**Impact**: Kubernetes liveness probes using `/health` will kill the pod in load-test mode.

**Correct fix**: Either (a) make health check mode-aware (skip streamer check when `LOAD_TEST_ENABLED=true`), or (b) document that `/livez` should be used for Kubernetes probes in load-test mode (already functional).

---

## TD-8: No Block History / Persistence (Medium)

**Problem**: Blocks are processed and state is updated, but the raw block data is never stored in Go. After a restart, the ring buffer is empty and the first 10 real reorgs are undetectable until the buffer refills.

**Impact**: (a) `validate_determinism` cannot be implemented (TD-2). (b) After restart, any reorg in the first 10 blocks is silently missed. (c) No audit trail of processed blocks.

**Correct fix**: A simple in-memory LRU cache (or the existing reorg ring buffer extended) would cover the restart window gap. Full persistence would require a database (out of scope for initial implementation).

---

## TD-9: No LOG_LEVEL Configuration (Low)

**File**: `internal/logging/logger.go`

**Problem**: The logger is hardcoded to `zapcore.InfoLevel`. There is no way to enable debug logging without changing and recompiling the code.

**Correct fix**: Read `LOG_LEVEL` from environment variable in `config.go` and pass it to `logging.NewLogger()`.

---

## Debt Priority Summary

| ID | Severity | Effort | Description |
|----|----------|--------|-------------|
| TD-5 | 🔴 High | Small | Secret detector breaks Infura/Alchemy URLs |
| TD-1 | 🔴 High | Small | Worker panic doesn't restart worker |
| TD-3 | 🔴 High | Medium | Prometheus event metrics never wired |
| TD-2 | 🟡 Medium | Large | validate_determinism is a stub |
| TD-4 | 🟡 Medium | Small | apply_block latency tracker unpopulated |
| TD-8 | 🟡 Medium | Medium | No block history |
| TD-6 | 🟢 Low | Small | Reorg simulation mode missing |
| TD-7 | 🟢 Low | Small | /health broken in load-test mode |
| TD-9 | 🟢 Low | Small | No LOG_LEVEL config |
