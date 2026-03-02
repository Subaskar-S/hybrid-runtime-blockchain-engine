package ffi

import (
	"fmt"

	"go.uber.org/zap"
)

// Validator provides input validation for FFI operations
type Validator struct {
	logger *zap.Logger
}

// NewValidator creates a new validator
func NewValidator(logger *zap.Logger) *Validator {
	return &Validator{
		logger: logger,
	}
}

// ValidateBlock validates a block before FFI processing
func (v *Validator) ValidateBlock(block *Block, lastBlockNumber uint64) error {
	if block == nil {
		v.logValidationFailure("block is nil", nil)
		return fmt.Errorf("block is nil")
	}

	// Validate block number is monotonically increasing
	if lastBlockNumber > 0 && block.Number != lastBlockNumber+1 {
		v.logValidationFailure("non-monotonic block number", map[string]interface{}{
			"expected":     lastBlockNumber + 1,
			"got":          block.Number,
			"last_block":   lastBlockNumber,
		})
		return fmt.Errorf("non-monotonic block number: expected %d, got %d", lastBlockNumber+1, block.Number)
	}

	// Validate parent hash is exactly 32 bytes
	if len(block.ParentHash) != 32 {
		v.logValidationFailure("invalid parent hash length", map[string]interface{}{
			"expected": 32,
			"got":      len(block.ParentHash),
			"block":    block.Number,
		})
		return fmt.Errorf("invalid parent hash length: expected 32, got %d", len(block.ParentHash))
	}

	// Validate transactions
	for i, tx := range block.Transactions {
		if err := v.ValidateTransaction(&tx, i); err != nil {
			return fmt.Errorf("transaction %d validation failed: %w", i, err)
		}
	}

	return nil
}

// ValidateTransaction validates a transaction
func (v *Validator) ValidateTransaction(tx *Transaction, index int) error {
	if tx == nil {
		v.logValidationFailure("transaction is nil", map[string]interface{}{
			"index": index,
		})
		return fmt.Errorf("transaction is nil")
	}

	// Validate addresses (always 20 bytes, no additional validation needed for byte arrays)
	// Validate data length is reasonable (max 1MB per transaction)
	if len(tx.Data) > 1024*1024 {
		v.logValidationFailure("transaction data too large", map[string]interface{}{
			"index":    index,
			"size":     len(tx.Data),
			"max_size": 1024 * 1024,
		})
		return fmt.Errorf("transaction data too large: %d bytes", len(tx.Data))
	}

	return nil
}

// ValidateSerializedData validates serialized block data
func (v *Validator) ValidateSerializedData(data []byte) error {
	if data == nil {
		v.logValidationFailure("serialized data is nil", nil)
		return fmt.Errorf("serialized data is nil")
	}

	// Validate length is within bounds (0 to 10MB)
	if len(data) == 0 {
		v.logValidationFailure("serialized data is empty", nil)
		return fmt.Errorf("serialized data is empty")
	}

	if len(data) > MaxBlockSize {
		v.logValidationFailure("serialized data exceeds maximum size", map[string]interface{}{
			"size":     len(data),
			"max_size": MaxBlockSize,
		})
		return fmt.Errorf("serialized data exceeds maximum size: %d > %d", len(data), MaxBlockSize)
	}

	// Validate version byte
	if len(data) > 0 && data[0] != SerializationVersion {
		v.logValidationFailure("unsupported serialization version", map[string]interface{}{
			"expected": SerializationVersion,
			"got":      data[0],
		})
		return fmt.Errorf("unsupported serialization version: %d", data[0])
	}

	return nil
}

// ValidateBlockNumber validates a block number for rollback
func (v *Validator) ValidateBlockNumber(blockNumber uint64, currentBlockNumber uint64) error {
	if blockNumber > currentBlockNumber {
		v.logValidationFailure("rollback target exceeds current block", map[string]interface{}{
			"target":  blockNumber,
			"current": currentBlockNumber,
		})
		return fmt.Errorf("rollback target %d exceeds current block %d", blockNumber, currentBlockNumber)
	}

	return nil
}

// logValidationFailure logs validation failures with details
func (v *Validator) logValidationFailure(message string, details map[string]interface{}) {
	if v.logger == nil {
		return
	}

	fields := []zap.Field{
		zap.String("validation_error", message),
	}

	for key, value := range details {
		fields = append(fields, zap.Any(key, value))
	}

	v.logger.Error("FFI validation failed", fields...)
}
