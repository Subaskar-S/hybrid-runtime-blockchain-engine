---
inclusion: fileMatch
fileMatchPattern: "internal/ffi/**"
---

# FFI Layer Guide

This steering file loads automatically when you are working in `internal/ffi/`.

## What the FFI Layer Does

The FFI (Foreign Function Interface) layer is the bridge between Go and Rust. It:
1. Serializes Go `Block` structs into a binary format
2. Calls Rust functions via cgo
3. Copies the result out of Rust-allocated memory
4. Frees Rust-allocated memory via `free_buffer`

## File Responsibilities

| File | Purpose |
|------|---------|
| `ffi.go` | cgo declarations, platform LDFLAGS, and the main `FFI` struct with `ApplyBlock`, `RollbackTo`, `GetStateRoot`, `GetStats`, `InitEngine` |
| `serialization.go` | `SerializeBlock` and `DeserializeBlock` — binary format implementation |
| `types.go` | Go types: `Block`, `Transaction`, `Hash`, `Stats`, and `Block.Hash()` |
| `validation.go` | `Validator` struct — validates block numbers, parent hash length, data size, version byte |

## The Five Rust Functions (and Their Go Wrappers)

| Rust (C-exported) | Go wrapper | Returns |
|-------------------|------------|---------|
| `init_engine() -> i32` | `FFI.InitEngine() error` | error on non-zero |
| `apply_block(ptr, len, out_ptr, out_len) -> i32` | `FFI.ApplyBlock(*Block) ([32]byte, error)` | state root bytes |
| `rollback_to(block_num) -> i32` | `FFI.RollbackTo(uint64) error` | error on non-zero |
| `get_state_root(out_ptr, out_len) -> i32` | `FFI.GetStateRoot() ([32]byte, error)` | state root bytes |
| `get_stats(out_ptr, out_len) -> i32` | `FFI.GetStats() (*Stats, error)` | JSON-decoded stats |
| `free_buffer(ptr, len)` | called internally | void |

## Memory Ownership — The Most Important Rule

```
Go memory:   Go allocates, Go frees
Rust memory: Rust allocates, Go must call free_buffer() to free

Never:
  - Store a Go pointer in Rust
  - Let Rust-allocated memory leak (always call free_buffer)
  - Access Rust memory after calling free_buffer
```

Pattern for receiving Rust-allocated output:
```go
var resultPtr *C.uchar
var resultLen C.size_t

ret := C.apply_block(/* input */, &resultPtr, &resultLen)
if ret != 0 {
    return [32]byte{}, fmt.Errorf("apply_block failed: code %d", ret)
}
// ALWAYS defer free_buffer before using the pointer
defer C.free_buffer(resultPtr, resultLen)

// Copy from Rust memory into Go memory
var stateRoot [32]byte
copy(stateRoot[:], C.GoBytes(unsafe.Pointer(resultPtr), C.int(resultLen)))
return stateRoot, nil
```

## Binary Format (Authoritative)

This is the format `SerializeBlock` produces and `rust-core/src/ffi.rs::deserialize_block` consumes:

```
Offset  Size   Field
------  ----   -----
0       1      Version byte (must be 0x01)
1       8      Block number, big-endian uint64
9       32     Parent hash (32 bytes)
41      8      Timestamp, big-endian uint64
49      4      Transaction count, big-endian uint32
53      ...    Transactions (variable)

Each Transaction:
  0     20     From address
  20    20     To address
  40    32     Value (U256, big-endian 32 bytes)
  72    4      Data length, big-endian uint32
  76    ...    Data bytes (data_length bytes)
```

Minimum block size (no transactions): 53 bytes.

## Error Codes from Rust

| Code | Meaning |
|------|---------|
| 0 | Success |
| -1 | Invalid input (null pointer, bad length, bad version, wrong block number) |
| -2 | Serialization/deserialization error |
| -3 | State engine error (engine not initialized, apply failed, rollback failed) |

## Platform LDFLAGS

The `ffi.go` file uses platform-specific build constraints to set the correct linker flags:

```go
// +build darwin
// #cgo LDFLAGS: -L${SRCDIR}/../../rust-core/target/release -lrust_core -framework Security -framework Foundation

// +build linux
// #cgo LDFLAGS: -L${SRCDIR}/../../rust-core/target/release -lrust_core -ldl -lpthread

// +build windows
// #cgo LDFLAGS: -L${SRCDIR}/../../rust-core/target/release -lrust_core -lws2_32 -luserenv
```

If you're seeing linker errors, first confirm:
1. `rust-core/target/release/librust_core.a` exists (run `cargo build --release`)
2. You're using the correct platform build tag
3. `CGO_ENABLED=1` is set

## Thread Safety

The `FFI` struct contains a mutex. All cgo calls go through this mutex. This is because the Rust state engine uses a global `lazy_static` mutex internally — concurrent calls from multiple goroutines would deadlock without Go-side serialization.

Do not remove the Go-side mutex unless you understand the Rust-side locking model.

## Validator

The `Validator` in `validation.go` checks inputs before they reach the FFI call:

- Block number must be monotonically increasing (rejects duplicates and out-of-order blocks)
- Data size must be ≤ 10MB
- Parent hash must be exactly 32 bytes
- Version byte (in serialized data) must be 0x01

Input validation happens in Go so that invalid input never reaches cgo. This avoids potential undefined behavior in Rust for truly malformed input.
