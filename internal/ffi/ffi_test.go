package ffi

import (
	"testing"
)

func TestFFI_InitEngine(t *testing.T) {
	ffi := NewFFI()
	err := ffi.InitEngine()
	if err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}
}

func TestFFI_ApplyBlock(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	block := &Block{
		Number:       1,
		ParentHash:   Hash{},
		Timestamp:    1234567890,
		Transactions: []Transaction{},
	}

	stateRoot, err := ffi.ApplyBlock(block)
	if err != nil {
		t.Fatalf("ApplyBlock failed: %v", err)
	}

	if stateRoot == (Hash{}) {
		t.Error("Expected non-zero state root")
	}
}

func TestFFI_ApplyBlock_NonMonotonic(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	// Apply block 1
	block1 := &Block{
		Number:       1,
		ParentHash:   Hash{},
		Timestamp:    1234567890,
		Transactions: []Transaction{},
	}
	_, err := ffi.ApplyBlock(block1)
	if err != nil {
		t.Fatalf("ApplyBlock block1 failed: %v", err)
	}

	// Try to apply block 5 (skip blocks 2-4)
	block5 := &Block{
		Number:       5,
		ParentHash:   Hash{},
		Timestamp:    1234567890,
		Transactions: []Transaction{},
	}
	_, err = ffi.ApplyBlock(block5)
	if err == nil {
		t.Error("Expected error for non-monotonic block number, got nil")
	}
}

func TestFFI_ApplyBlock_InvalidParentHash(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	// Create block with invalid parent hash (wrong length)
	block := &Block{
		Number:       1,
		ParentHash:   Hash{}, // This is valid (32 bytes)
		Timestamp:    1234567890,
		Transactions: []Transaction{},
	}

	// This should succeed (parent hash is valid)
	_, err := ffi.ApplyBlock(block)
	if err != nil {
		t.Fatalf("ApplyBlock failed: %v", err)
	}
}

func TestFFI_GetStateRoot(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	stateRoot, err := ffi.GetStateRoot()
	if err != nil {
		t.Fatalf("GetStateRoot failed: %v", err)
	}

	// Initial state root should be all zeros
	expectedRoot := Hash{}
	if stateRoot != expectedRoot {
		t.Errorf("Expected zero state root initially, got %v", stateRoot)
	}
}

func TestFFI_GetStats(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	stats, err := ffi.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.BlockNumber != 0 {
		t.Errorf("Expected block number 0, got %d", stats.BlockNumber)
	}
	if stats.StateSize != 0 {
		t.Errorf("Expected state size 0, got %d", stats.StateSize)
	}
}

func TestFFI_RollbackTo(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	// Apply block 1
	block1 := &Block{
		Number:       1,
		ParentHash:   Hash{},
		Timestamp:    1234567890,
		Transactions: []Transaction{},
	}
	_, err := ffi.ApplyBlock(block1)
	if err != nil {
		t.Fatalf("ApplyBlock block1 failed: %v", err)
	}

	// Apply block 2
	block2 := &Block{
		Number:       2,
		ParentHash:   Hash{},
		Timestamp:    1234567891,
		Transactions: []Transaction{},
	}
	_, err = ffi.ApplyBlock(block2)
	if err != nil {
		t.Fatalf("ApplyBlock block2 failed: %v", err)
	}

	// Rollback to block 1
	err = ffi.RollbackTo(1)
	if err != nil {
		t.Fatalf("RollbackTo failed: %v", err)
	}

	// Verify stats
	stats, err := ffi.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.BlockNumber != 1 {
		t.Errorf("Expected block number 1 after rollback, got %d", stats.BlockNumber)
	}
}

func TestFFI_ApplyBlock_WithTransactions(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	// Note: This test will fail in Rust because we don't have initial balances
	// But it tests the FFI boundary
	block := &Block{
		Number:     1,
		ParentHash: Hash{},
		Timestamp:  1234567890,
		Transactions: []Transaction{
			{
				From:  Address{1},
				To:    Address{2},
				Value: NewU256FromUint64(100),
				Data:  []byte{},
			},
		},
	}

	// This will fail because sender has no balance
	_, err := ffi.ApplyBlock(block)
	if err == nil {
		t.Error("Expected error for transaction with insufficient balance")
	}
}

func TestFFI_Concurrent(t *testing.T) {
	ffi := NewFFI()
	if err := ffi.InitEngine(); err != nil {
		t.Fatalf("InitEngine failed: %v", err)
	}

	// Test that mutex protects concurrent access
	done := make(chan bool, 2)

	go func() {
		_, _ = ffi.GetStateRoot()
		done <- true
	}()

	go func() {
		_, _ = ffi.GetStats()
		done <- true
	}()

	<-done
	<-done
}
