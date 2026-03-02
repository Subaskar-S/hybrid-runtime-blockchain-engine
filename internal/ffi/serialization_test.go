package ffi

import (
	"bytes"
	"testing"
)

func TestSerializeDeserializeBlock_Empty(t *testing.T) {
	block := &Block{
		Number:       1,
		ParentHash:   Hash{},
		Timestamp:    1234567890,
		Transactions: []Transaction{},
	}

	// Serialize
	data, err := SerializeBlock(block)
	if err != nil {
		t.Fatalf("SerializeBlock failed: %v", err)
	}

	// Deserialize
	decoded, err := DeserializeBlock(data)
	if err != nil {
		t.Fatalf("DeserializeBlock failed: %v", err)
	}

	// Verify
	if decoded.Number != block.Number {
		t.Errorf("Block number mismatch: got %d, want %d", decoded.Number, block.Number)
	}
	if decoded.Timestamp != block.Timestamp {
		t.Errorf("Timestamp mismatch: got %d, want %d", decoded.Timestamp, block.Timestamp)
	}
	if len(decoded.Transactions) != 0 {
		t.Errorf("Expected 0 transactions, got %d", len(decoded.Transactions))
	}
}

func TestSerializeDeserializeBlock_WithTransactions(t *testing.T) {
	block := &Block{
		Number:     1,
		ParentHash: Hash{1, 2, 3},
		Timestamp:  1234567890,
		Transactions: []Transaction{
			{
				From:  Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
				To:    Address{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40},
				Value: NewU256FromUint64(1000),
				Data:  []byte{0xaa, 0xbb, 0xcc},
			},
		},
	}

	// Serialize
	data, err := SerializeBlock(block)
	if err != nil {
		t.Fatalf("SerializeBlock failed: %v", err)
	}

	// Deserialize
	decoded, err := DeserializeBlock(data)
	if err != nil {
		t.Fatalf("DeserializeBlock failed: %v", err)
	}

	// Verify block fields
	if decoded.Number != block.Number {
		t.Errorf("Block number mismatch: got %d, want %d", decoded.Number, block.Number)
	}
	if decoded.ParentHash != block.ParentHash {
		t.Errorf("Parent hash mismatch")
	}

	// Verify transactions
	if len(decoded.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(decoded.Transactions))
	}

	tx := decoded.Transactions[0]
	origTx := block.Transactions[0]

	if tx.From != origTx.From {
		t.Errorf("From address mismatch")
	}
	if tx.To != origTx.To {
		t.Errorf("To address mismatch")
	}
	if tx.Value.Bytes() != origTx.Value.Bytes() {
		t.Errorf("Value mismatch")
	}
	if !bytes.Equal(tx.Data, origTx.Data) {
		t.Errorf("Data mismatch")
	}
}

func TestSerializeBlock_TooLarge(t *testing.T) {
	// Create a block that exceeds MaxBlockSize
	largeData := make([]byte, MaxBlockSize)
	block := &Block{
		Number:     1,
		ParentHash: Hash{},
		Timestamp:  1234567890,
		Transactions: []Transaction{
			{
				From:  Address{},
				To:    Address{},
				Value: NewU256FromUint64(0),
				Data:  largeData,
			},
		},
	}

	_, err := SerializeBlock(block)
	if err == nil {
		t.Error("Expected error for oversized block, got nil")
	}
}

func TestDeserializeBlock_InvalidVersion(t *testing.T) {
	data := []byte{99} // Invalid version
	data = append(data, make([]byte, 52)...)

	_, err := DeserializeBlock(data)
	if err == nil {
		t.Error("Expected error for invalid version, got nil")
	}
}

func TestDeserializeBlock_TooShort(t *testing.T) {
	data := []byte{1, 2, 3} // Too short

	_, err := DeserializeBlock(data)
	if err == nil {
		t.Error("Expected error for short data, got nil")
	}
}

func TestSerializeDeserialize_RoundTrip(t *testing.T) {
	// Test multiple round trips to ensure determinism
	block := &Block{
		Number:     42,
		ParentHash: Hash{0xff, 0xee, 0xdd},
		Timestamp:  9876543210,
		Transactions: []Transaction{
			{
				From:  Address{1},
				To:    Address{2},
				Value: NewU256FromUint64(500),
				Data:  []byte{},
			},
			{
				From:  Address{3},
				To:    Address{4},
				Value: NewU256FromUint64(1500),
				Data:  []byte{0x01, 0x02, 0x03, 0x04},
			},
		},
	}

	// First round trip
	data1, err := SerializeBlock(block)
	if err != nil {
		t.Fatalf("First serialize failed: %v", err)
	}

	decoded1, err := DeserializeBlock(data1)
	if err != nil {
		t.Fatalf("First deserialize failed: %v", err)
	}

	// Second round trip
	data2, err := SerializeBlock(decoded1)
	if err != nil {
		t.Fatalf("Second serialize failed: %v", err)
	}

	// Data should be identical
	if !bytes.Equal(data1, data2) {
		t.Error("Round trip produced different serialization")
	}
}

func TestU256_Conversions(t *testing.T) {
	tests := []uint64{0, 1, 100, 1000, 1<<32 - 1, 1 << 63}

	for _, val := range tests {
		u256 := NewU256FromUint64(val)
		bigInt := u256.BigInt()

		if bigInt.Uint64() != val {
			t.Errorf("U256 conversion failed for %d: got %d", val, bigInt.Uint64())
		}
	}
}
