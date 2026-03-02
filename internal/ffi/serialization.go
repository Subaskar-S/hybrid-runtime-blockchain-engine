package ffi

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	// SerializationVersion is the current serialization format version
	SerializationVersion byte = 1

	// MaxBlockSize is the maximum allowed block size (10MB)
	MaxBlockSize = 10 * 1024 * 1024
)

// SerializeBlock serializes a block to binary format
// Format:
// [Version: 1 byte]
// [Block Number: 8 bytes, big-endian]
// [Parent Hash: 32 bytes]
// [Timestamp: 8 bytes, big-endian]
// [Tx Count: 4 bytes, big-endian]
// [Transactions: variable length]
//
// Transaction format:
// [From: 20 bytes]
// [To: 20 bytes]
// [Value: 32 bytes]
// [Data Length: 4 bytes, big-endian]
// [Data: variable length]
func SerializeBlock(block *Block) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Version byte
	if err := buf.WriteByte(SerializationVersion); err != nil {
		return nil, fmt.Errorf("failed to write version: %w", err)
	}

	// Block number (8 bytes, big-endian)
	if err := binary.Write(buf, binary.BigEndian, block.Number); err != nil {
		return nil, fmt.Errorf("failed to write block number: %w", err)
	}

	// Parent hash (32 bytes)
	if _, err := buf.Write(block.ParentHash[:]); err != nil {
		return nil, fmt.Errorf("failed to write parent hash: %w", err)
	}

	// Timestamp (8 bytes, big-endian)
	if err := binary.Write(buf, binary.BigEndian, block.Timestamp); err != nil {
		return nil, fmt.Errorf("failed to write timestamp: %w", err)
	}

	// Transaction count (4 bytes, big-endian)
	txCount := uint32(len(block.Transactions))
	if err := binary.Write(buf, binary.BigEndian, txCount); err != nil {
		return nil, fmt.Errorf("failed to write tx count: %w", err)
	}

	// Transactions
	for i, tx := range block.Transactions {
		if err := serializeTransaction(buf, &tx); err != nil {
			return nil, fmt.Errorf("failed to serialize transaction %d: %w", i, err)
		}
	}

	data := buf.Bytes()

	// Validate size
	if len(data) > MaxBlockSize {
		return nil, fmt.Errorf("block size %d exceeds maximum %d", len(data), MaxBlockSize)
	}

	return data, nil
}

// serializeTransaction serializes a single transaction
func serializeTransaction(buf *bytes.Buffer, tx *Transaction) error {
	// From address (20 bytes)
	if _, err := buf.Write(tx.From[:]); err != nil {
		return fmt.Errorf("failed to write from address: %w", err)
	}

	// To address (20 bytes)
	if _, err := buf.Write(tx.To[:]); err != nil {
		return fmt.Errorf("failed to write to address: %w", err)
	}

	// Value (32 bytes)
	valueBytes := tx.Value.Bytes()
	if _, err := buf.Write(valueBytes[:]); err != nil {
		return fmt.Errorf("failed to write value: %w", err)
	}

	// Data length (4 bytes, big-endian)
	dataLen := uint32(len(tx.Data))
	if err := binary.Write(buf, binary.BigEndian, dataLen); err != nil {
		return fmt.Errorf("failed to write data length: %w", err)
	}

	// Data (variable length)
	if len(tx.Data) > 0 {
		if _, err := buf.Write(tx.Data); err != nil {
			return fmt.Errorf("failed to write data: %w", err)
		}
	}

	return nil
}

// DeserializeBlock deserializes a block from binary format
func DeserializeBlock(data []byte) (*Block, error) {
	if len(data) < 53 {
		// Minimum: 1 + 8 + 32 + 8 + 4
		return nil, fmt.Errorf("data too short: %d bytes", len(data))
	}

	buf := bytes.NewReader(data)

	// Version byte
	version, err := buf.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if version != SerializationVersion {
		return nil, fmt.Errorf("unsupported version: %d", version)
	}

	// Block number
	var blockNumber uint64
	if err := binary.Read(buf, binary.BigEndian, &blockNumber); err != nil {
		return nil, fmt.Errorf("failed to read block number: %w", err)
	}

	// Parent hash
	var parentHash Hash
	if _, err := buf.Read(parentHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read parent hash: %w", err)
	}

	// Timestamp
	var timestamp uint64
	if err := binary.Read(buf, binary.BigEndian, &timestamp); err != nil {
		return nil, fmt.Errorf("failed to read timestamp: %w", err)
	}

	// Transaction count
	var txCount uint32
	if err := binary.Read(buf, binary.BigEndian, &txCount); err != nil {
		return nil, fmt.Errorf("failed to read tx count: %w", err)
	}

	// Transactions
	transactions := make([]Transaction, txCount)
	for i := uint32(0); i < txCount; i++ {
		tx, err := deserializeTransaction(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize transaction %d: %w", i, err)
		}
		transactions[i] = *tx
	}

	return &Block{
		Number:       blockNumber,
		ParentHash:   parentHash,
		Timestamp:    timestamp,
		Transactions: transactions,
	}, nil
}

// deserializeTransaction deserializes a single transaction
func deserializeTransaction(buf *bytes.Reader) (*Transaction, error) {
	tx := &Transaction{}

	// From address
	if _, err := buf.Read(tx.From[:]); err != nil {
		return nil, fmt.Errorf("failed to read from address: %w", err)
	}

	// To address
	if _, err := buf.Read(tx.To[:]); err != nil {
		return nil, fmt.Errorf("failed to read to address: %w", err)
	}

	// Value
	var valueBytes [32]byte
	if _, err := buf.Read(valueBytes[:]); err != nil {
		return nil, fmt.Errorf("failed to read value: %w", err)
	}
	tx.Value = U256{bytes: valueBytes}

	// Data length
	var dataLen uint32
	if err := binary.Read(buf, binary.BigEndian, &dataLen); err != nil {
		return nil, fmt.Errorf("failed to read data length: %w", err)
	}

	// Data
	if dataLen > 0 {
		tx.Data = make([]byte, dataLen)
		if _, err := buf.Read(tx.Data); err != nil {
			return nil, fmt.Errorf("failed to read data: %w", err)
		}
	}

	return tx, nil
}
