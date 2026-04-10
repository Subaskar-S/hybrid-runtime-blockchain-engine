package ffi

import (
	"testing"
)

// BenchmarkApplyBlock measures the full FFI round-trip for apply_block.
func BenchmarkApplyBlock(b *testing.B) {
	f := NewFFI()
	if err := f.InitEngine(); err != nil {
		b.Fatalf("InitEngine: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := &Block{
			Number:       uint64(i + 1),
			ParentHash:   Hash{},
			Timestamp:    1700000000 + uint64(i),
			Transactions: []Transaction{},
		}
		if _, err := f.ApplyBlock(block); err != nil {
			b.Fatalf("ApplyBlock: %v", err)
		}
	}
}

// BenchmarkSerializeBlock measures block serialization throughput.
func BenchmarkSerializeBlock(b *testing.B) {
	block := &Block{
		Number:     1,
		ParentHash: Hash{},
		Timestamp:  1700000000,
		Transactions: func() []Transaction {
			txs := make([]Transaction, 100)
			for i := range txs {
				txs[i] = Transaction{
					From:  Address{byte(i)},
					To:    Address{byte(i + 1)},
					Value: NewU256FromUint64(uint64(i) * 1e18),
					Data:  []byte{},
				}
			}
			return txs
		}(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := SerializeBlock(block); err != nil {
			b.Fatalf("SerializeBlock: %v", err)
		}
	}
}

// BenchmarkDeserializeBlock measures block deserialization throughput.
func BenchmarkDeserializeBlock(b *testing.B) {
	block := &Block{
		Number:     1,
		ParentHash: Hash{},
		Timestamp:  1700000000,
		Transactions: func() []Transaction {
			txs := make([]Transaction, 100)
			for i := range txs {
				txs[i] = Transaction{
					From:  Address{byte(i)},
					To:    Address{byte(i + 1)},
					Value: NewU256FromUint64(uint64(i) * 1e18),
					Data:  []byte{},
				}
			}
			return txs
		}(),
	}
	data, err := SerializeBlock(block)
	if err != nil {
		b.Fatalf("SerializeBlock: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DeserializeBlock(data); err != nil {
			b.Fatalf("DeserializeBlock: %v", err)
		}
	}
}

// BenchmarkGetStateRoot measures the FFI get_state_root call.
func BenchmarkGetStateRoot(b *testing.B) {
	f := NewFFI()
	if err := f.InitEngine(); err != nil {
		b.Fatalf("InitEngine: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.GetStateRoot(); err != nil {
			b.Fatalf("GetStateRoot: %v", err)
		}
	}
}
