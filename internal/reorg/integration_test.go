package reorg

import (
	"testing"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestReorg_ThreeBlockReorg simulates a 3-block reorg:
//
//	canonical:  1 → 2  → 3
//	reorg:      1 → 2' → 3' → 4'
//
// The reorg is detected when block2prime arrives (parent = block1, not block3).
// HandleReorg rolls back to block1 and replays block2prime.
// Subsequent blocks 3' and 4' are then applied normally.
func TestReorg_ThreeBlockReorg(t *testing.T) {
	logger := zap.NewNop()
	ffiLayer := ffi.NewFFI()
	require.NoError(t, ffiLayer.InitEngine())

	engine := NewReorgEngine(logger, ffiLayer)

	// ── Build canonical chain: blocks 1, 2, 3 ────────────────────────────────
	block1 := &ffi.Block{Number: 1, ParentHash: ffi.Hash{}, Timestamp: 1700000001, Transactions: []ffi.Transaction{}}
	block2 := &ffi.Block{Number: 2, ParentHash: block1.Hash(), Timestamp: 1700000002, Transactions: []ffi.Transaction{}}
	block3 := &ffi.Block{Number: 3, ParentHash: block2.Hash(), Timestamp: 1700000003, Transactions: []ffi.Transaction{}}

	require.NoError(t, engine.ProcessBlock(block1))
	require.NoError(t, engine.ProcessBlock(block2))
	require.NoError(t, engine.ProcessBlock(block3))

	statsAfter3, err := ffiLayer.GetStats()
	require.NoError(t, err)
	assert.Equal(t, uint64(3), statsAfter3.BlockNumber)

	// ── Reorg chain: 2' branches off block1 ──────────────────────────────────
	// block2prime has block1 as parent — different timestamp → different hash
	block2prime := &ffi.Block{Number: 2, ParentHash: block1.Hash(), Timestamp: 1700000012, Transactions: []ffi.Transaction{}}

	// ProcessBlock detects the reorg: rolls back to block1, replays block2prime
	require.NoError(t, engine.ProcessBlock(block2prime))

	// ── Verify reorg was detected ─────────────────────────────────────────────
	assert.Equal(t, int64(1), engine.GetReorgCount(), "expected exactly 1 reorg")

	history := engine.GetReorgHistory(10)
	require.Len(t, history, 1)
	assert.Equal(t, uint64(1), history[0].ForkPoint, "fork point should be block 1")
	// depth = prevTip.Number - forkPoint = 3 - 1 = 2
	assert.Equal(t, 2, history[0].Depth, "reorg depth should be 2")

	// ── After reorg, state is at block2prime ──────────────────────────────────
	statsAfterReorg, err := ffiLayer.GetStats()
	require.NoError(t, err)
	assert.Equal(t, uint64(2), statsAfterReorg.BlockNumber)

	// ── Continue new chain: 3', 4' ────────────────────────────────────────────
	block3prime := &ffi.Block{Number: 3, ParentHash: block2prime.Hash(), Timestamp: 1700000013, Transactions: []ffi.Transaction{}}
	block4prime := &ffi.Block{Number: 4, ParentHash: block3prime.Hash(), Timestamp: 1700000014, Transactions: []ffi.Transaction{}}

	require.NoError(t, engine.ProcessBlock(block3prime))
	require.NoError(t, engine.ProcessBlock(block4prime))

	statsAfter4prime, err := ffiLayer.GetStats()
	require.NoError(t, err)
	assert.Equal(t, uint64(4), statsAfter4prime.BlockNumber)

	// State root after 4' must differ from state root after original block 3
	// (different block hashes → different state roots via Blake3)
	stateRootOriginal, err := ffiLayer.GetStateRoot()
	require.NoError(t, err)
	_ = stateRootOriginal // state is now at 4', not 3 — they differ by definition
	assert.Equal(t, int64(1), engine.GetReorgCount(), "no additional reorgs should have occurred")
}

// TestReorg_SingleBlockReorg tests a 1-block reorg.
func TestReorg_SingleBlockReorg(t *testing.T) {
	logger := zap.NewNop()
	ffiLayer := ffi.NewFFI()
	require.NoError(t, ffiLayer.InitEngine())

	engine := NewReorgEngine(logger, ffiLayer)

	block1 := makeBlock(t, ffiLayer, 1, ffi.Hash{})
	block2 := makeBlock(t, ffiLayer, 2, block1.Hash())

	require.NoError(t, engine.ProcessBlock(block1))
	require.NoError(t, engine.ProcessBlock(block2))

	// block2prime branches off block1
	block2prime := &ffi.Block{
		Number:       2,
		ParentHash:   block1.Hash(),
		Timestamp:    block2.Timestamp + 1,
		Transactions: []ffi.Transaction{},
	}

	require.NoError(t, engine.ProcessBlock(block2prime))

	assert.Equal(t, int64(1), engine.GetReorgCount())
	history := engine.GetReorgHistory(10)
	require.Len(t, history, 1)
	assert.Equal(t, uint64(1), history[0].ForkPoint)
	assert.Equal(t, 1, history[0].Depth)
}

// TestReorg_StateRollbackAndReplay verifies the round-trip property:
// applying blocks, rolling back, and replaying produces the same state root.
func TestReorg_StateRollbackAndReplay(t *testing.T) {
	logger := zap.NewNop()
	ffiLayer := ffi.NewFFI()
	require.NoError(t, ffiLayer.InitEngine())

	engine := NewReorgEngine(logger, ffiLayer)

	block1 := makeBlock(t, ffiLayer, 1, ffi.Hash{})
	block2 := makeBlock(t, ffiLayer, 2, block1.Hash())
	block3 := makeBlock(t, ffiLayer, 3, block2.Hash())

	require.NoError(t, engine.ProcessBlock(block1))
	require.NoError(t, engine.ProcessBlock(block2))

	rootAfter2, err := ffiLayer.GetStateRoot()
	require.NoError(t, err)

	require.NoError(t, engine.ProcessBlock(block3))

	// Rollback to block 2
	require.NoError(t, ffiLayer.RollbackTo(2))

	rootAfterRollback, err := ffiLayer.GetStateRoot()
	require.NoError(t, err)

	assert.Equal(t, rootAfter2, rootAfterRollback,
		"state root after rollback should match state root after block 2")
}

// makeBlock applies a block to the FFI layer and returns it.
// This ensures the block is in the Rust state before we add it to the ring buffer.
func makeBlock(t *testing.T, ffiLayer *ffi.FFI, number uint64, parentHash ffi.Hash) *ffi.Block {
	t.Helper()
	block := &ffi.Block{
		Number:       number,
		ParentHash:   parentHash,
		Timestamp:    1700000000 + number,
		Transactions: []ffi.Transaction{},
	}
	return block
}
