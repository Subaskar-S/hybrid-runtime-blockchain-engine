package ffi

/*
#cgo darwin  LDFLAGS: -L../../rust-core/target/release -lrust_core -framework CoreFoundation -framework Security
#cgo linux   LDFLAGS: -L../../rust-core/target/release -lrust_core -ldl -lpthread -lm
#cgo windows LDFLAGS: -L../../rust-core/target/release -lrust_core -lws2_32 -luserenv -lbcrypt -lntdll -lgcc_eh -lstdc++
#include <stdlib.h>

extern int init_engine();
extern int apply_block(const unsigned char* data, size_t len, unsigned char** result, size_t* result_len);
extern int rollback_to(unsigned long long block_number);
extern int get_state_root(unsigned char** root, size_t* root_len);
extern int get_stats(unsigned char** stats, size_t* stats_len);
extern void free_buffer(unsigned char* ptr, size_t len);
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"go.uber.org/zap"
)

// FFI provides the interface to the Rust core
type FFI struct {
	mu              sync.Mutex
	lastBlockNumber uint64
	validator       *Validator

	// onApplyBlock is called after every successful apply_block FFI call
	// with the measured execution duration. Injected from main.go to avoid
	// import cycles. nil means no-op.
	onApplyBlock func(duration time.Duration)
}

// SetOnApplyBlock sets the callback invoked after each successful apply_block
// call. Safe to call before any blocks are processed. Pass nil to disable.
func (f *FFI) SetOnApplyBlock(fn func(time.Duration)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onApplyBlock = fn
}

// Stats represents statistics from the Rust core
type Stats struct {
	BlockNumber      uint64 `json:"block_number"`
	StateSize        int    `json:"state_size"`
	HistoryLength    int    `json:"history_length"`
	MemoryUsageBytes int    `json:"memory_usage_bytes"`
}

// NewFFI creates a new FFI instance
func NewFFI() *FFI {
	logger, _ := zap.NewProduction()
	return &FFI{
		lastBlockNumber: 0,
		validator:       NewValidator(logger),
	}
}

// NewFFIWithLogger creates a new FFI instance with a custom logger
func NewFFIWithLogger(logger *zap.Logger) *FFI {
	return &FFI{
		lastBlockNumber: 0,
		validator:       NewValidator(logger),
	}
}

// InitEngine initializes the Rust state engine
func (f *FFI) InitEngine() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	ret := C.init_engine()
	if ret != 0 {
		return fmt.Errorf("init_engine failed with code %d", ret)
	}

	return nil
}

// ApplyBlock applies a block to the state and returns the new state root
func (f *FFI) ApplyBlock(block *Block) (Hash, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Validate block
	if err := f.validator.ValidateBlock(block, f.lastBlockNumber); err != nil {
		return Hash{}, fmt.Errorf("block validation failed: %w", err)
	}

	// Serialize block
	data, err := SerializeBlock(block)
	if err != nil {
		return Hash{}, fmt.Errorf("serialization failed: %w", err)
	}

	// Validate serialized data
	if err := f.validator.ValidateSerializedData(data); err != nil {
		return Hash{}, fmt.Errorf("serialized data validation failed: %w", err)
	}

	// Call Rust FFI — time the actual cgo call only
	var resultPtr *C.uchar
	var resultLen C.size_t

	ffiStart := time.Now()
	ret := C.apply_block(
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.size_t(len(data)),
		&resultPtr,
		&resultLen,
	)
	ffiDuration := time.Since(ffiStart)

	if ret != 0 {
		return Hash{}, fmt.Errorf("apply_block failed with code %d", ret)
	}

	// Copy result and free Rust memory
	defer C.free_buffer(resultPtr, resultLen)

	if resultLen != 32 {
		return Hash{}, fmt.Errorf("unexpected state root length: %d", resultLen)
	}

	var stateRoot Hash
	copy(stateRoot[:], C.GoBytes(unsafe.Pointer(resultPtr), C.int(resultLen)))

	// Update last block number
	f.lastBlockNumber = block.Number

	// Fire latency callback after releasing nothing — we still hold the mutex,
	// which is fine since the callback is a non-blocking counter increment.
	if f.onApplyBlock != nil {
		f.onApplyBlock(ffiDuration)
	}

	return stateRoot, nil
}

// RollbackTo rolls back the state to a specific block number
func (f *FFI) RollbackTo(blockNumber uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	ret := C.rollback_to(C.ulonglong(blockNumber))
	if ret != 0 {
		return fmt.Errorf("rollback_to failed with code %d", ret)
	}

	// Update last block number
	f.lastBlockNumber = blockNumber

	return nil
}

// GetStateRoot returns the current state root
func (f *FFI) GetStateRoot() (Hash, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var rootPtr *C.uchar
	var rootLen C.size_t

	ret := C.get_state_root(&rootPtr, &rootLen)
	if ret != 0 {
		return Hash{}, fmt.Errorf("get_state_root failed with code %d", ret)
	}

	// Copy result and free Rust memory
	defer C.free_buffer(rootPtr, rootLen)

	if rootLen != 32 {
		return Hash{}, fmt.Errorf("unexpected state root length: %d", rootLen)
	}

	var stateRoot Hash
	copy(stateRoot[:], C.GoBytes(unsafe.Pointer(rootPtr), C.int(rootLen)))

	return stateRoot, nil
}

// GetStats returns statistics from the Rust core
func (f *FFI) GetStats() (*Stats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var statsPtr *C.uchar
	var statsLen C.size_t

	ret := C.get_stats(&statsPtr, &statsLen)
	if ret != 0 {
		return nil, fmt.Errorf("get_stats failed with code %d", ret)
	}

	// Copy result and free Rust memory
	defer C.free_buffer(statsPtr, statsLen)

	statsJSON := C.GoBytes(unsafe.Pointer(statsPtr), C.int(statsLen))

	var stats Stats
	if err := json.Unmarshal(statsJSON, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse stats JSON: %w", err)
	}

	return &stats, nil
}
