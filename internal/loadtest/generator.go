package loadtest

import (
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"math/rand"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
)

// BlockGenerator generates deterministic synthetic blocks for load testing
type BlockGenerator struct {
	seed int64
}

// NewBlockGenerator creates a new deterministic block generator
func NewBlockGenerator(seed int64) *BlockGenerator {
	return &BlockGenerator{
		seed: seed,
	}
}

// GenerateBlock generates a deterministic block with the specified number of transactions
// The same seed and parameters will always produce the same block
func (bg *BlockGenerator) GenerateBlock(blockNumber uint64, txCount int) ffi.Block {
	// Create seeded RNG for deterministic generation
	blockSeed := bg.seed + int64(blockNumber)
	rng := rand.New(rand.NewSource(blockSeed))

	// Generate parent hash deterministically
	parentHash := bg.generateHash(blockNumber-1, rng)

	// Generate transactions
	txs := make([]ffi.Transaction, txCount)
	for i := 0; i < txCount; i++ {
		txs[i] = bg.generateTransaction(rng)
	}

	return ffi.Block{
		Number:       blockNumber,
		ParentHash:   parentHash,
		Timestamp:    uint64(1700000000 + blockNumber), // Fixed base timestamp + block number
		Transactions: txs,
	}
}

// generateHash generates a deterministic 32-byte hash
func (bg *BlockGenerator) generateHash(blockNumber uint64, rng *rand.Rand) [32]byte {
	var hash [32]byte
	
	// Use block number and random bytes for deterministic hash
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, blockNumber)
	
	// Add random bytes
	randomBytes := make([]byte, 24)
	rng.Read(randomBytes)
	
	// Combine and hash
	combined := append(buf, randomBytes...)
	hash = sha256.Sum256(combined)
	
	return hash
}

// generateTransaction generates a deterministic transaction
func (bg *BlockGenerator) generateTransaction(rng *rand.Rand) ffi.Transaction {
	return ffi.Transaction{
		From:  bg.generateAddress(rng),
		To:    bg.generateAddress(rng),
		Value: bg.generateU256Value(rng),
		Data:  []byte{}, // Empty data for simplicity
	}
}

// generateAddress generates a deterministic 20-byte address
func (bg *BlockGenerator) generateAddress(rng *rand.Rand) ffi.Address {
	var addr ffi.Address
	rng.Read(addr[:])
	return addr
}

// generateU256Value generates a deterministic transaction value as U256
// Values range from 1 to 1000 ETH (in wei)
func (bg *BlockGenerator) generateU256Value(rng *rand.Rand) ffi.U256 {
	// Generate value between 1 and 1000 ETH
	ethAmount := rng.Int63n(1000) + 1
	
	// Convert to wei (1 ETH = 10^18 wei)
	weiPerEth := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	value := new(big.Int).Mul(big.NewInt(ethAmount), weiPerEth)
	
	return ffi.NewU256FromBigInt(value)
}
