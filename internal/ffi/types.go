package ffi

import (
	"math/big"
)

// Address represents a 20-byte Ethereum address
type Address [20]byte

// Hash represents a 32-byte hash
type Hash [32]byte

// U256 represents a 256-bit unsigned integer
type U256 struct {
	bytes [32]byte
}

// NewU256FromUint64 creates a U256 from a uint64
func NewU256FromUint64(value uint64) U256 {
	var u U256
	// Store as big-endian in last 8 bytes
	for i := 0; i < 8; i++ {
		u.bytes[24+i] = byte(value >> (56 - i*8))
	}
	return u
}

// NewU256FromBigInt creates a U256 from a big.Int
func NewU256FromBigInt(value *big.Int) U256 {
	var u U256
	bytes := value.Bytes()
	// Copy bytes to the end (big-endian)
	if len(bytes) <= 32 {
		copy(u.bytes[32-len(bytes):], bytes)
	}
	return u
}

// Bytes returns the 32-byte representation
func (u *U256) Bytes() [32]byte {
	return u.bytes
}

// BigInt converts U256 to big.Int
func (u *U256) BigInt() *big.Int {
	return new(big.Int).SetBytes(u.bytes[:])
}

// Transaction represents a blockchain transaction
type Transaction struct {
	From  Address
	To    Address
	Value U256
	Data  []byte
}

// Block represents a blockchain block
type Block struct {
	Number       uint64
	ParentHash   Hash
	Timestamp    uint64
	Transactions []Transaction
}

// Hash calculates the block hash (simplified for now)
func (b *Block) Hash() Hash {
	// This should match the Rust implementation
	// For now, return a simple hash
	var h Hash
	// In production, this would use the same Blake3 hashing as Rust
	return h
}
