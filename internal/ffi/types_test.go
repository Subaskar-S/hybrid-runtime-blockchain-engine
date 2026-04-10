package ffi

import (
	"math/big"
	"testing"
)

func TestNewU256FromBigInt(t *testing.T) {
	// zero
	z := NewU256FromBigInt(big.NewInt(0))
	if z.BigInt().Sign() != 0 {
		t.Errorf("expected zero, got %v", z.BigInt())
	}

	// small value
	v := NewU256FromBigInt(big.NewInt(12345))
	if v.BigInt().Int64() != 12345 {
		t.Errorf("expected 12345, got %v", v.BigInt())
	}

	// round-trip with large value
	large := new(big.Int).Lsh(big.NewInt(1), 200) // 2^200
	u := NewU256FromBigInt(large)
	if u.BigInt().Cmp(large) != 0 {
		t.Errorf("round-trip failed: got %v", u.BigInt())
	}
}

func TestBlockHash_NonZero(t *testing.T) {
	b := &Block{Number: 1, ParentHash: Hash{}, Timestamp: 1700000001}
	h := b.Hash()
	var zero Hash
	if h == zero {
		t.Error("block hash should not be zero")
	}
}

func TestBlockHash_Deterministic(t *testing.T) {
	b := &Block{Number: 42, ParentHash: Hash{0xab}, Timestamp: 1700000042}
	if b.Hash() != b.Hash() {
		t.Error("block hash must be deterministic")
	}
}

func TestBlockHash_DifferentBlocks(t *testing.T) {
	b1 := &Block{Number: 1, ParentHash: Hash{}, Timestamp: 1700000001}
	b2 := &Block{Number: 2, ParentHash: Hash{}, Timestamp: 1700000002}
	if b1.Hash() == b2.Hash() {
		t.Error("different blocks should have different hashes")
	}
}
