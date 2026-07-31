---
inclusion: fileMatch
fileMatchPattern: "internal/mcp/**"
---

# MCP Server Guide

This steering file loads automatically when you are working in `internal/mcp/`.

## What the MCP Server Does

Exposes 11 JSON-RPC 2.0 tools on `127.0.0.1:<MCP_PORT>` (default 8080) for AI agent and operator introspection of the running system. Every tool call goes through rate limiting and input validation before reaching the handler.

## File Responsibilities

| File | Purpose |
|------|---------|
| `server.go` | HTTP server, JSON-RPC dispatcher, tool registration, `RegisterTool()` |
| `tools.go` | All 11 tool handler functions + `RegisterRuntimeTools()` + `LatencyTracker` + `ApplyBlockLatencyTracker` |
| `rate_limiter.go` | Sliding-window rate limiter: 10 req/min per tool |
| `input_validator.go` | Rejects shell patterns, validates param types/ranges |
| `RUNTIME_TOOLS.md` | Human-readable reference for all 11 tools |

## The 11 Tools

| Tool name | Handler function | Status |
|-----------|-----------------|--------|
| `get_gc_stats` | `handleGetGCStats` | ✅ Working |
| `get_heap_usage` | `handleGetHeapUsage` | ✅ Working |
| `get_goroutine_count` | `handleGetGoroutineCount` | ✅ Working |
| `get_latency_distribution` | `handleGetLatencyDistribution` | ✅ Working (reads `LatencyTracker`) |
| `get_reorg_history` | `handleGetReorgHistory` | ✅ Working |
| `get_state_root` | `handleGetStateRoot` | ✅ Working |
| `get_state_size` | `handleGetStateSize` | ✅ Working |
| `get_apply_block_latency` | `handleGetApplyBlockLatency` | ⚠️ Tracker never populated (TD-4) |
| `validate_determinism` | `handleValidateDeterminism` | ❌ Stub (TD-2) |
| `compare_gc_vs_core_latency` | `handleCompareGCVsCoreLacy` | ✅ Working |
| `run_load_test` | `handleRunLoadTest` | ⚠️ Requires `LOAD_TEST_ENABLED=true` |

## Adding a New Tool

```go
// 1. Write the handler in tools.go
func handleMyNewTool(params map[string]interface{}) (interface{}, error) {
    // parse params
    // call into reorgEngine, ffiAdapter, or runtime
    return map[string]interface{}{
        "result": value,
    }, nil
}

// 2. Register it in RegisterRuntimeTools()
server.RegisterTool("my_new_tool", "Description shown in tool listings", handleMyNewTool)

// 3. Add to RUNTIME_TOOLS.md

// 4. Write a test in tools_test.go
```

Rate limiting is **automatic** — applied to every registered tool by the middleware layer. You do not need to add it manually.

## Rate Limiter Behaviour

- 10 requests per minute per tool name (sliding window)
- Returns HTTP 429 when exceeded
- Window resets 60 seconds after the first request in the window
- Each tool has its own independent counter — hitting the limit on `get_gc_stats` does not affect `get_state_root`

## Input Validator

Rejects params containing shell metacharacters (`;`, `|`, `` ` ``, `$`, `&&`, `||`). Validates numeric params are within declared ranges. Used to prevent prompt injection attacks via the JSON-RPC interface.

If adding a new tool with numeric params, register the allowed range in `input_validator.go`.

## LatencyTracker vs ApplyBlockLatencyTracker

Two distinct circular buffers:

| Tracker | What it measures | Who populates it |
|---------|-----------------|-----------------|
| `LatencyTracker` | Time from block arriving at `workerPool.Submit()` to submission completing | `cmd/server/main.go` goroutine |
| `ApplyBlockLatencyTracker` | Time Rust's `apply_block` takes inside the FFI call | **Currently nobody (TD-4)** |

`get_latency_distribution` reads `LatencyTracker` — this is populated but only measures channel write latency, not full processing latency.

`get_apply_block_latency` reads `ApplyBlockLatencyTracker` — this always returns empty because nothing writes to it.

## Security Rules — Do Not Relax These

1. **Bind address is `127.0.0.1` only** — never change to `0.0.0.0` without adding auth
2. **No shell execution** — tool handlers must never call `exec.Command` or similar
3. **JSON-only responses** — no HTML, no binary, no executable content in responses
4. **Input validation is mandatory** — all tool params must go through `input_validator.go`
5. **Rate limiting is mandatory** — use `RegisterTool`, never bypass the middleware

## Testing Pattern

```go
func TestHandleGetGCStats(t *testing.T) {
    result, err := handleGetGCStats(map[string]interface{}{})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    m, ok := result.(map[string]interface{})
    if !ok {
        t.Fatal("result is not a map")
    }
    if _, ok := m["num_gc"]; !ok {
        t.Error("result missing num_gc field")
    }
}
```

For rate limiter tests, use `time.Now()` mocking or call the limiter 11 times and verify the 11th returns an error.
