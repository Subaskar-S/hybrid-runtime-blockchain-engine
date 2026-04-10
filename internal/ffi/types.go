package ffi

import (
	"crypto/sha256"
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

// Hash calculates the block hash using the same approach as the Rust implementation.
// It serializes the block fields deterministically and hashes with SHA-256.
// In production this would use Blake3 to match Rust exactly, but SHA-256 is
// available without cgo and produces a stable, non-zero hash for reorg detection.
func (b *Block) Hash() Hash {
	var buf [8 + 32 + 8]byte
	// block number big-endian
	buf[0] = byte(b.Number >> 56)
	buf[1] = byte(b.Number >> 48)
	buf[2] = byte(b.Number >> 40)
	buf[3] = byte(b.Number >> 32)
	buf[4] = byte(b.Number >> 24)
	buf[5] = byte(b.Number >> 16)
	buf[6] = byte(b.Number >> 8)
	buf[7] = byte(b.Number)
	// parent hash
	copy(buf[8:40], b.ParentHash[:])
	// timestamp big-endian
	buf[40] = byte(b.Timestamp >> 56)
	buf[41] = byte(b.Timestamp >> 48)
	buf[42] = byte(b.Timestamp >> 40)
	buf[43] = byte(b.Timestamp >> 32)
	buf[44] = byte(b.Timestamp >> 24)
	buf[45] = byte(b.Timestamp >> 16)
	buf[46] = byte(b.Timestamp >> 8)
	buf[47] = byte(b.Timestamp)

	return Hash(sha256.Sum256(buf[:]))
}
