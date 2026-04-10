package reorg

import (
	"fmt"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

const (
	// MaxReorgDepth is the maximum supported reorg depth
	MaxReorgDepth = 10
)

// FFIClient is the subset of ffi.FFI used by the reorg engine.
// Using an interface keeps this package free of direct cgo dependencies
// and makes it easy to mock in tests.
type FFIClient interface {
	ApplyBlock(block *ffi.Block) (ffi.Hash, error)
	RollbackTo(blockNumber uint64) error
}

// ReorgEngine handles blockchain reorganization detection and recovery
type ReorgEngine struct {
	logger      *zap.Logger
	ffi         FFIClient
	ringBuffer  *RingBuffer
	reorgCount  int64
	reorgEvents []ReorgEvent
}

// ReorgEvent represents a detected reorganization
type ReorgEvent struct {
	Timestamp          time.Time
	ForkPoint          uint64
	Depth              int
	RollbackDurationMs float64
}

// RingBuffer maintains a fixed-size circular buffer of recent blocks
type RingBuffer struct {
	blocks [MaxReorgDepth]*ffi.Block
	index  int
	size   int
}

// NewRingBuffer creates a new ring buffer
func NewRingBuffer() *RingBuffer {
	return &RingBuffer{
		blocks: [MaxReorgDepth]*ffi.Block{},
		index:  0,
		size:   0,
	}
}

// Add adds a block to the ring buffer
func (rb *RingBuffer) Add(block *ffi.Block) {
	rb.blocks[rb.index] = block
	rb.index = (rb.index + 1) % MaxReorgDepth
	if rb.size < MaxReorgDepth {
		rb.size++
	}
}

// Get retrieves a block by index (0 = oldest, size-1 = newest)
func (rb *RingBuffer) Get(idx int) *ffi.Block {
	if idx < 0 || idx >= rb.size {
		return nil
	}
	
	// Calculate actual index in circular buffer
	actualIdx := (rb.index - rb.size + idx + MaxReorgDepth) % MaxReorgDepth
	return rb.blocks[actualIdx]
}

// Latest returns the most recently added block
func (rb *RingBuffer) Latest() *ffi.Block {
	if rb.size == 0 {
		return nil
	}
	return rb.Get(rb.size - 1)
}

// Size returns the current number of blocks in the buffer
func (rb *RingBuffer) Size() int {
	return rb.size
}

// Truncate removes blocks after the specified index
func (rb *RingBuffer) Truncate(keepSize int) {
	if keepSize < 0 {
		keepSize = 0
	}
	if keepSize > rb.size {
		keepSize = rb.size
	}
	rb.size = keepSize
	rb.index = keepSize % MaxReorgDepth
}

// NewReorgEngine creates a new reorg engine
func NewReorgEngine(logger *zap.Logger, ffiClient FFIClient) *ReorgEngine {
	return &ReorgEngine{
		logger:      logger,
		ffi:         ffiClient,
		ringBuffer:  NewRingBuffer(),
		reorgEvents: make([]ReorgEvent, 0, 100),
	}
}

// ProcessBlock processes a block and detects reorgs
func (re *ReorgEngine) ProcessBlock(block *ffi.Block) error {
	// Check for reorg
	isReorg, forkPoint, err := re.DetectReorg(block)
	if err != nil {
		return fmt.Errorf("reorg detection failed: %w", err)
	}

	if isReorg {
		re.logger.Warn("blockchain reorganization detected",
			zap.Uint64("fork_point", forkPoint),
			zap.Uint64("new_block", block.Number))

		// Handle reorg
		if err := re.HandleReorg(forkPoint, block); err != nil {
			return fmt.Errorf("reorg handling failed: %w", err)
		}
	} else {
		// Normal block processing
		if _, err := re.ffi.ApplyBlock(block); err != nil {
			return fmt.Errorf("apply block failed: %w", err)
		}

		// Add to ring buffer
		re.ringBuffer.Add(block)
	}

	return nil
}

// DetectReorg detects if a block represents a reorganization.
// A reorg is detected when the new block's ParentHash does not match
// the hash of the most recent block in the ring buffer.
func (re *ReorgEngine) DetectReorg(newBlock *ffi.Block) (bool, uint64, error) {
	// If buffer is empty, no reorg possible
	if re.ringBuffer.Size() == 0 {
		return false, 0, nil
	}

	prevBlock := re.ringBuffer.Latest()
	if prevBlock == nil {
		return false, 0, nil
	}

	// Normal sequential block: parent hash must match previous block's hash
	prevHash := prevBlock.Hash()
	if newBlock.ParentHash == prevHash {
		return false, 0, nil
	}

	// Parent hash mismatch — potential reorg. Find fork point by scanning
	// the ring buffer backwards for the block whose hash matches the new
	// block's parent hash.
	forkPoint, err := re.findForkPoint(newBlock)
	if err != nil {
		return false, 0, err
	}

	// Calculate reorg depth
	depth := int(prevBlock.Number - forkPoint)
	if depth > MaxReorgDepth {
		return false, 0, fmt.Errorf("reorg depth %d exceeds maximum %d", depth, MaxReorgDepth)
	}

	return true, forkPoint, nil
}

// findForkPoint traverses the ring buffer backwards to find the block whose
// hash matches newBlock.ParentHash, returning that block's number.
func (re *ReorgEngine) findForkPoint(newBlock *ffi.Block) (uint64, error) {
	for i := re.ringBuffer.Size() - 1; i >= 0; i-- {
		block := re.ringBuffer.Get(i)
		if block == nil {
			continue
		}
		if block.Hash() == newBlock.ParentHash {
			return block.Number, nil
		}
	}
	return 0, fmt.Errorf("fork point not found in ring buffer (reorg depth may exceed buffer size)")
}

// HandleReorg handles a detected reorganization
func (re *ReorgEngine) HandleReorg(forkPoint uint64, newBlock *ffi.Block) error {
	start := time.Now()

	// Capture the previous tip before we modify the ring buffer
	prevTip := re.ringBuffer.Latest()
	depth := 0
	if prevTip != nil && prevTip.Number > forkPoint {
		depth = int(prevTip.Number - forkPoint)
	}

	// Rollback to fork point
	if err := re.Rollback(forkPoint); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	// Apply the new block
	if _, err := re.ffi.ApplyBlock(newBlock); err != nil {
		return fmt.Errorf("replay failed: %w", err)
	}

	// Update ring buffer: keep only blocks up to and including the fork point
	re.ringBuffer.Truncate(int(forkPoint))
	re.ringBuffer.Add(newBlock)

	// Record reorg event
	duration := time.Since(start)
	event := ReorgEvent{
		Timestamp:          time.Now(),
		ForkPoint:          forkPoint,
		Depth:              depth,
		RollbackDurationMs: float64(duration.Milliseconds()),
	}
	re.reorgEvents = append(re.reorgEvents, event)
	re.reorgCount++

	re.logger.Info("reorganization handled successfully",
		zap.Uint64("fork_point", forkPoint),
		zap.Int("depth", event.Depth),
		zap.Float64("duration_ms", event.RollbackDurationMs))

	return nil
}

// Rollback rolls back the state to the specified block number
func (re *ReorgEngine) Rollback(forkPoint uint64) error {
	re.logger.Info("rolling back to fork point", zap.Uint64("fork_point", forkPoint))

	if err := re.ffi.RollbackTo(forkPoint); err != nil {
		return fmt.Errorf("FFI rollback failed: %w", err)
	}

	return nil
}

// GetReorgHistory returns recent reorg events
func (re *ReorgEngine) GetReorgHistory(limit int) []ReorgEvent {
	if limit <= 0 || limit > len(re.reorgEvents) {
		limit = len(re.reorgEvents)
	}

	// Return most recent events
	start := len(re.reorgEvents) - limit
	if start < 0 {
		start = 0
	}

	return re.reorgEvents[start:]
}

// GetReorgCount returns the total number of reorgs processed
func (re *ReorgEngine) GetReorgCount() int64 {
	return re.reorgCount
}

// Stats returns reorg engine statistics
type Stats struct {
	ReorgCount      int64
	BufferSize      int
	RecentReorgs    []ReorgEvent
}

// GetStats returns current statistics
func (re *ReorgEngine) GetStats() Stats {
	return Stats{
		ReorgCount:   re.reorgCount,
		BufferSize:   re.ringBuffer.Size(),
		RecentReorgs: re.GetReorgHistory(10),
	}
}
