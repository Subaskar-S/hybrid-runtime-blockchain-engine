package worker

import (
	"context"
	"testing"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

// BenchmarkWorkerPool_Submit measures block submission throughput.
func BenchmarkWorkerPool_Submit(b *testing.B) {
	logger := zap.NewNop()
	processor := &MockProcessor{}
	pool := NewPool(logger, processor)

	ctx := context.Background()
	if err := pool.Start(ctx, 4); err != nil {
		b.Fatalf("Start: %v", err)
	}
	defer pool.Stop(ctx)

	block := &ffi.Block{
		Number:       1,
		ParentHash:   ffi.Hash{},
		Timestamp:    1700000000,
		Transactions: []ffi.Transaction{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.Number = uint64(i + 1)
		if err := pool.Submit(block); err != nil {
			b.Fatalf("Submit: %v", err)
		}
	}
}
