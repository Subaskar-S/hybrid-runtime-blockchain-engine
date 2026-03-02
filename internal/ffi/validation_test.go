package ffi

import (
	"testing"

	"go.uber.org/zap"
)

func TestValidator_ValidateBlock(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	validator := NewValidator(logger)

	tests := []struct {
		name            string
		block           *Block
		lastBlockNumber uint64
		wantErr         bool
	}{
		{
			name: "valid block",
			block: &Block{
				Number:       1,
				ParentHash:   Hash{},
				Timestamp:    1234567890,
				Transactions: []Transaction{},
			},
			lastBlockNumber: 0,
			wantErr:         false,
		},
		{
			name:            "nil block",
			block:           nil,
			lastBlockNumber: 0,
			wantErr:         true,
		},
		{
			name: "non-monotonic block number",
			block: &Block{
				Number:       5,
				ParentHash:   Hash{},
				Timestamp:    1234567890,
				Transactions: []Transaction{},
			},
			lastBlockNumber: 2,
			wantErr:         true,
		},
		{
			name: "sequential block numbers",
			block: &Block{
				Number:       3,
				ParentHash:   Hash{},
				Timestamp:    1234567890,
				Transactions: []Transaction{},
			},
			lastBlockNumber: 2,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateBlock(tt.block, tt.lastBlockNumber)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBlock() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateTransaction(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	validator := NewValidator(logger)

	tests := []struct {
		name    string
		tx      *Transaction
		wantErr bool
	}{
		{
			name: "valid transaction",
			tx: &Transaction{
				From:  Address{1},
				To:    Address{2},
				Value: NewU256FromUint64(100),
				Data:  []byte{0x01, 0x02},
			},
			wantErr: false,
		},
		{
			name:    "nil transaction",
			tx:      nil,
			wantErr: true,
		},
		{
			name: "transaction with large data",
			tx: &Transaction{
				From:  Address{1},
				To:    Address{2},
				Value: NewU256FromUint64(100),
				Data:  make([]byte, 2*1024*1024), // 2MB - exceeds limit
			},
			wantErr: true,
		},
		{
			name: "transaction with empty data",
			tx: &Transaction{
				From:  Address{1},
				To:    Address{2},
				Value: NewU256FromUint64(100),
				Data:  []byte{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTransaction(tt.tx, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransaction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateSerializedData(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	validator := NewValidator(logger)

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid data",
			data:    []byte{SerializationVersion, 0x01, 0x02, 0x03},
			wantErr: false,
		},
		{
			name:    "nil data",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "oversized data",
			data:    make([]byte, MaxBlockSize+1),
			wantErr: true,
		},
		{
			name:    "invalid version",
			data:    []byte{99, 0x01, 0x02, 0x03},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateSerializedData(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSerializedData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateBlockNumber(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	validator := NewValidator(logger)

	tests := []struct {
		name               string
		blockNumber        uint64
		currentBlockNumber uint64
		wantErr            bool
	}{
		{
			name:               "valid rollback",
			blockNumber:        5,
			currentBlockNumber: 10,
			wantErr:            false,
		},
		{
			name:               "rollback to current",
			blockNumber:        10,
			currentBlockNumber: 10,
			wantErr:            false,
		},
		{
			name:               "rollback to future",
			blockNumber:        15,
			currentBlockNumber: 10,
			wantErr:            true,
		},
		{
			name:               "rollback to genesis",
			blockNumber:        0,
			currentBlockNumber: 10,
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateBlockNumber(tt.blockNumber, tt.currentBlockNumber)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBlockNumber() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_WithoutLogger(t *testing.T) {
	validator := NewValidator(nil)

	// Should not panic without logger
	block := &Block{
		Number:       5,
		ParentHash:   Hash{},
		Timestamp:    1234567890,
		Transactions: []Transaction{},
	}

	err := validator.ValidateBlock(block, 2)
	if err == nil {
		t.Error("Expected validation error for non-monotonic block")
	}
}
