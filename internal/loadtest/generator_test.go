package loadtest

import (
	"math/big"
	"testing"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/stretchr/testify/assert"
)

func TestNewBlockGenerator(t *testing.T) {
	seed := int64(12345)
	gen := NewBlockGenerator(seed)

	assert.NotNil(t, gen)
	assert.Equal(t, seed, gen.seed)
}

func TestGenerateBlock_Deterministic(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	// Generate same block twice
	block1 := gen.GenerateBlock(1, 10)
	block2 := gen.GenerateBlock(1, 10)

	// Should be identical
	assert.Equal(t, block1.Number, block2.Number)
	assert.Equal(t, block1.ParentHash, block2.ParentHash)
	assert.Equal(t, block1.Timestamp, block2.Timestamp)
	assert.Equal(t, len(block1.Transactions), len(block2.Transactions))

	// Verify all transactions are identical
	for i := 0; i < len(block1.Transactions); i++ {
		assert.Equal(t, block1.Transactions[i].From, block2.Transactions[i].From)
		assert.Equal(t, block1.Transactions[i].To, block2.Transactions[i].To)
		assert.Equal(t, block1.Transactions[i].Value.Bytes(), block2.Transactions[i].Value.Bytes())
		assert.Equal(t, block1.Transactions[i].Data, block2.Transactions[i].Data)
	}
}

func TestGenerateBlock_DifferentSeeds(t *testing.T) {
	gen1 := NewBlockGenerator(100)
	gen2 := NewBlockGenerator(200)

	block1 := gen1.GenerateBlock(1, 10)
	block2 := gen2.GenerateBlock(1, 10)

	// Should be different
	assert.NotEqual(t, block1.ParentHash, block2.ParentHash)
	
	// At least one transaction should be different
	different := false
	for i := 0; i < len(block1.Transactions); i++ {
		if block1.Transactions[i].From != block2.Transactions[i].From {
			different = true
			break
		}
	}
	assert.True(t, different, "Blocks from different seeds should have different transactions")
}

func TestGenerateBlock_DifferentBlockNumbers(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	block1 := gen.GenerateBlock(1, 10)
	block2 := gen.GenerateBlock(2, 10)

	// Block numbers should be different
	assert.Equal(t, uint64(1), block1.Number)
	assert.Equal(t, uint64(2), block2.Number)

	// Parent hashes should be different
	assert.NotEqual(t, block1.ParentHash, block2.ParentHash)

	// Timestamps should be different
	assert.NotEqual(t, block1.Timestamp, block2.Timestamp)
	assert.Equal(t, block1.Timestamp+1, block2.Timestamp)
}

func TestGenerateBlock_TransactionCount(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	testCases := []int{0, 1, 10, 100, 1000}

	for _, txCount := range testCases {
		block := gen.GenerateBlock(1, txCount)
		assert.Equal(t, txCount, len(block.Transactions), "Block should have %d transactions", txCount)
	}
}

func TestGenerateBlock_ValidStructure(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	block := gen.GenerateBlock(100, 50)

	// Verify block structure
	assert.Equal(t, uint64(100), block.Number)
	assert.NotEqual(t, [32]byte{}, block.ParentHash, "Parent hash should not be zero")
	assert.Greater(t, block.Timestamp, uint64(0), "Timestamp should be positive")
	assert.Equal(t, 50, len(block.Transactions))

	// Verify all transactions have valid structure
	for i, tx := range block.Transactions {
		assert.NotEqual(t, ffi.Address{}, tx.From, "Transaction %d: From address should not be zero", i)
		assert.NotEqual(t, ffi.Address{}, tx.To, "Transaction %d: To address should not be zero", i)
		// Verify value is not zero
		valueInt := tx.Value.BigInt()
		assert.Greater(t, valueInt.Sign(), 0, "Transaction %d: Value should be positive", i)
		assert.NotNil(t, tx.Data, "Transaction %d: Data should not be nil", i)
	}
}

func TestGenerateBlock_ValueRange(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	block := gen.GenerateBlock(1, 100)

	// Verify all transaction values are within expected range (1-1000 ETH in wei)
	weiPerEth := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	minValue := new(big.Int).Set(weiPerEth)       // 1 ETH
	maxValue := new(big.Int).Mul(big.NewInt(1000), weiPerEth) // 1000 ETH

	for i, tx := range block.Transactions {
		valueInt := tx.Value.BigInt()
		assert.GreaterOrEqual(t, valueInt.Cmp(minValue), 0, "Transaction %d: Value should be >= 1 ETH", i)
		assert.LessOrEqual(t, valueInt.Cmp(maxValue), 0, "Transaction %d: Value should be <= 1000 ETH", i)
	}
}

func TestGenerateBlock_Reproducibility(t *testing.T) {
	// Test that the same seed produces the same sequence of blocks
	seed := int64(999)
	
	gen1 := NewBlockGenerator(seed)
	blocks1 := make([]ffi.Block, 10)
	for i := 0; i < 10; i++ {
		blocks1[i] = gen1.GenerateBlock(uint64(i), 5)
	}

	gen2 := NewBlockGenerator(seed)
	blocks2 := make([]ffi.Block, 10)
	for i := 0; i < 10; i++ {
		blocks2[i] = gen2.GenerateBlock(uint64(i), 5)
	}

	// All blocks should be identical
	for i := 0; i < 10; i++ {
		block1 := blocks1[i]
		block2 := blocks2[i]
		
		assert.Equal(t, block1.Number, block2.Number)
		assert.Equal(t, block1.ParentHash, block2.ParentHash)
		assert.Equal(t, block1.Timestamp, block2.Timestamp)
		assert.Equal(t, len(block1.Transactions), len(block2.Transactions))
	}
}

func TestGenerateBlock_ZeroTransactions(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	block := gen.GenerateBlock(1, 0)

	assert.Equal(t, uint64(1), block.Number)
	assert.NotEqual(t, [32]byte{}, block.ParentHash)
	assert.Greater(t, block.Timestamp, uint64(0))
	assert.Empty(t, block.Transactions)
}

func TestGenerateBlock_LargeBlockNumber(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	largeBlockNum := uint64(1000000)
	block := gen.GenerateBlock(largeBlockNum, 10)

	assert.Equal(t, largeBlockNum, block.Number)
	assert.Equal(t, uint64(1700000000+largeBlockNum), block.Timestamp)
	assert.Equal(t, 10, len(block.Transactions))
}

func TestGenerateAddress_Deterministic(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	// Generate blocks and verify addresses are deterministic
	block1 := gen.GenerateBlock(1, 5)
	block2 := gen.GenerateBlock(1, 5)

	for i := 0; i < 5; i++ {
		assert.Equal(t, block1.Transactions[i].From, block2.Transactions[i].From)
		assert.Equal(t, block1.Transactions[i].To, block2.Transactions[i].To)
	}
}

func TestGenerateValue_Deterministic(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	// Generate blocks and verify values are deterministic
	block1 := gen.GenerateBlock(1, 5)
	block2 := gen.GenerateBlock(1, 5)

	for i := 0; i < 5; i++ {
		assert.Equal(t, block1.Transactions[i].Value.Bytes(), block2.Transactions[i].Value.Bytes())
	}
}

func TestGenerateHash_Deterministic(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	// Generate same block multiple times
	hashes := make([][32]byte, 10)
	for i := 0; i < 10; i++ {
		block := gen.GenerateBlock(5, 1)
		hashes[i] = block.ParentHash
	}

	// All hashes should be identical
	for i := 1; i < 10; i++ {
		assert.Equal(t, hashes[0], hashes[i])
	}
}

func TestGenerateBlock_ParentHashChain(t *testing.T) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	// Generate a sequence of blocks
	blocks := make([]ffi.Block, 5)
	for i := 0; i < 5; i++ {
		blocks[i] = gen.GenerateBlock(uint64(i+1), 1)
	}

	// Verify each block has a unique parent hash
	// (Note: parent hashes are deterministic but not actually linked in this simple implementation)
	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			// Different blocks should have different parent hashes
			if blocks[i].Number != blocks[j].Number {
				assert.NotEqual(t, blocks[i].ParentHash, blocks[j].ParentHash)
			}
		}
	}
}

func BenchmarkGenerateBlock_10Tx(b *testing.B) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.GenerateBlock(uint64(i), 10)
	}
}

func BenchmarkGenerateBlock_100Tx(b *testing.B) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.GenerateBlock(uint64(i), 100)
	}
}

func BenchmarkGenerateBlock_1000Tx(b *testing.B) {
	seed := int64(42)
	gen := NewBlockGenerator(seed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.GenerateBlock(uint64(i), 1000)
	}
}
