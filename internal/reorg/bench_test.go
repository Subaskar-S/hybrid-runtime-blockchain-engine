package reorg

import (
	"testing"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

// BenchmarkReorgDetection measures reorg detection on sequential blocks.
func BenchmarkReorgDetection(b *testing.B) {
	logger := zap.NewNop()
	ffiLayer := ffi.NewFFI()
	if err := ffiLayer.InitEngine(); err != nil {
		b.Fatalf("InitEngine: %v", err)
	}
	engine := NewReorgEngine(logger, ffiLayer)

	// Pre-fill ring buffer
	prev := ffi.Hash{}
	for i := 1; i <= 5; i++ {
		blk := &ffi.Block{Number: uint64(i), ParentHash: prev, Timestamp: 1700000000 + uint64(i)}
		engine.ringBuffer.Add(blk)
		prev = blk.Hash()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Sequential block — no reorg
		blk := &ffi.Block{Number: uint64(6 + i), ParentHash: prev, Timestamp: 1700000006 + uint64(i)}
		_, _, _ = engine.DetectReorg(blk)
		prev = blk.Hash()
		engine.ringBuffer.Add(blk)
	}
}

// BenchmarkRollback measures rollback performance.
func BenchmarkRollback(b *testing.B) {
	logger := zap.NewNop()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ffiLayer := ffi.NewFFI()
		if err := ffiLayer.InitEngine(); err != nil {
			b.Fatalf("InitEngine: %v", err)
		}
		engine := NewReorgEngine(logger, ffiLayer)

		// Apply 10 blocks
		prev := ffi.Hash{}
		for j := 1; j <= 10; j++ {
			blk := &ffi.Block{Number: uint64(j), ParentHash: prev, Timestamp: 1700000000 + uint64(j)}
			if err := engine.ProcessBlock(blk); err != nil {
				b.Fatalf("ProcessBlock: %v", err)
			}
			prev = blk.Hash()
		}
		b.StartTimer()

		// Rollback to block 1
		if err := engine.Rollback(1); err != nil {
			b.Fatalf("Rollback: %v", err)
		}
	}
}
