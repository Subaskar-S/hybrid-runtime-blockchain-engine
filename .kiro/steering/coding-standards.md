---
inclusion: auto
---

# Coding Standards

## Go Standards

### Naming

| Element | Convention | Example |
|---------|-----------|---------|
| Packages | lowercase, single word | `ffi`, `reorg`, `worker` |
| Exported types | PascalCase | `ReorgEngine`, `WorkerPool` |
| Exported functions | PascalCase | `NewPool`, `ApplyBlock` |
| Unexported functions | camelCase | `processBlockSafe`, `handlePanic` |
| Interfaces | Noun or Noun+er | `BlockProcessor`, `RustCore` |
| Constants | PascalCase exported / camelCase unexported | `MaxReorgDepth`, `defaultPort` |
| Error vars | `Err` prefix | `ErrEngineNotInitialized` |
| Test functions | `Test` + description | `TestReorgDetectsDepth3Fork` |
| Source files | snake_case | `reorg_engine.go`, `rate_limiter.go` |

### Error Handling

Always return errors. Never swallow them silently.

```go
// Good — error is returned with context
func (e *ReorgEngine) ProcessBlock(block *ffi.Block) error {
    if block == nil {
        return fmt.Errorf("ProcessBlock: nil block")
    }
    if err := e.ffi.ApplyBlock(block); err != nil {
        return fmt.Errorf("ProcessBlock %d: apply block: %w", block.Number, err)
    }
    return nil
}

// Bad — error is silently ignored
func (e *ReorgEngine) ProcessBlock(block *ffi.Block) {
    e.ffi.ApplyBlock(block)
}
```

Never use `panic()` in production code. In `main.go`, use `logger.Fatal()` for unrecoverable startup failures.

### Documentation

Every exported symbol must have a Go doc comment:

```go
// NewPool creates a worker pool that dispatches blocks to processor.
// numWorkers is validated in Start(), not here.
func NewPool(logger *zap.Logger, processor BlockProcessor) *Pool {
```

Comment the *why*, not the *what*. Code shows what it does; comments explain why.

### Interfaces

Define interfaces at the consumer side, not the implementor side. Keep them small (1–3 methods):

```go
// Good — defined where it's consumed, minimal surface
type BlockProcessor interface {
    ProcessBlock(block *ffi.Block) error
}

// Bad — interface defined at implementor, bloated
type FullReorgEngineInterface interface { ... 15 methods ... }
```

### Goroutines and Channels

- Every goroutine needs a clear owner (who starts it, who stops it)
- Channels are closed by the sender, never the receiver
- Always use `context.Context` for cancellation
- Bounded channels are mandatory for producer/consumer pairs that could diverge in speed

### Const over Magic Numbers

```go
// Good
const maxReorgDepth = 10

// Bad
if depth > 10 {
```

---

## Rust Standards

### Naming

| Element | Convention | Example |
|---------|-----------|---------|
| Types/structs | PascalCase | `StateEngine`, `StateSnapshot` |
| Functions/methods | snake_case | `apply_block`, `rollback_to` |
| Constants | SCREAMING_SNAKE_CASE | `MAX_HISTORY_SIZE` |
| FFI exports | snake_case + `#[no_mangle]` | `init_engine`, `free_buffer` |
| Modules | snake_case | `state`, `ffi`, `types` |

### FFI Functions — Mandatory Pattern

Every `extern "C"` function must follow this exact pattern:

```rust
#[no_mangle]
pub extern "C" fn my_function(
    data_ptr: *const u8,
    data_len: usize,
    result_ptr: *mut *mut u8,
    result_len: *mut usize,
) -> i32 {
    // Step 1: Validate all pointer args
    if data_ptr.is_null() || result_ptr.is_null() || result_len.is_null() {
        return -1;
    }
    // Step 2: Validate length bounds
    if data_len == 0 || data_len > 10 * 1024 * 1024 {
        return -1;
    }
    // Step 3: Safe to work with data now
    let data = unsafe { std::slice::from_raw_parts(data_ptr, data_len) };
    // ... implementation ...
    // Step 4: Return allocated buffer; caller must call free_buffer
    std::mem::forget(result);
    0
}
```

### Determinism — Never Violate

The Rust core guarantees reproducible state. These rules are absolute:

| Rule | Why |
|------|-----|
| Never call `SystemTime::now()` | Breaks reproducibility |
| Never use `rand` | Non-deterministic output |
| Always use `checked_add` / `checked_sub` | Overflow is a bug, not a feature |
| Never use floating-point for balances | Precision divergence |
| Blake3 for state root | Deterministic, fast |
| Strict sequential block numbers | Out-of-order = reject |

---

## FFI Boundary Rules

These are non-negotiable for memory safety:

1. **Go owns Go memory, Rust owns Rust memory** — always copy across the boundary
2. **Go never passes a Go pointer into Rust** — only raw byte slices of serialized data
3. **Rust never stores a Go pointer** — copy data if Rust needs it long-term
4. **Rust-allocated output buffers** — Go must call `free_buffer(ptr, len)` after copying
5. **All return codes are checked** — `0 = success`, any negative = error

### Serialization Format (Binary, Not JSON)

```
Block:
  [1 byte ] version = 0x01
  [8 bytes] block_number, big-endian uint64
  [32 bytes] parent_hash
  [8 bytes] timestamp, big-endian uint64
  [4 bytes] tx_count, big-endian uint32
  [tx_count × Transaction]

Transaction:
  [20 bytes] from
  [20 bytes] to
  [32 bytes] value, big-endian U256
  [4 bytes ] data_len, big-endian uint32
  [data_len] data
```

---

## Metrics Wiring

**Defining a metric is insufficient. It must be recorded at the event site.**

| Metric | Must be recorded in |
|--------|---------------------|
| `blocks_processed_total` | `worker/pool.go` after successful process |
| `block_processing_duration_seconds` | `worker/pool.go` timing around process call |
| `rust_apply_block_duration_seconds` | `ffi/ffi.go` wrapping the cgo call |
| `worker_panic_total` | `worker/pool.go` inside `handlePanic()` |
| `reorg_total` | `reorg/engine.go` inside HandleReorg |
| `reorg_depth_blocks` | `reorg/engine.go` inside HandleReorg |
| `reorg_rollback_duration_seconds` | `reorg/engine.go` inside HandleReorg |

Use the adapter interfaces in `internal/metrics/adapters.go` to avoid import cycles.

---

## Testing Standards

### Go Tests

- Test files: `<source>_test.go` in the same package
- Integration tests: `integration_test.go`
- Benchmarks: `bench_test.go`
- Use table-driven tests for input/output variations
- Never use `time.Sleep` in tests — use channels or `sync.WaitGroup` for synchronization
- Always test error paths, not just the happy path

```go
// Good — table-driven, tests both paths
func TestValidateBlock(t *testing.T) {
    tests := []struct {
        name    string
        block   *ffi.Block
        wantErr bool
    }{
        {"valid block", validBlock, false},
        {"nil block", nil, true},
        {"zero number", zeroNumberBlock, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateBlock(tt.block)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateBlock() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Rust Tests

- Unit tests in `#[cfg(test)]` module at bottom of each source file
- Property-based tests via `proptest` crate (already in `Cargo.toml` as dev-dependency)
- Test determinism explicitly: apply the same block twice and compare state roots

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_apply_block_is_deterministic() {
        let block = make_test_block(1);
        
        let mut engine1 = StateEngine::new();
        let root1 = engine1.apply_block(&block).unwrap();
        
        let mut engine2 = StateEngine::new();
        let root2 = engine2.apply_block(&block).unwrap();
        
        assert_eq!(root1, root2, "same block must produce same state root");
    }
}
```

### Coverage Target

All new code: ≥ 80% coverage. Run `make coverage` to see per-package coverage.
