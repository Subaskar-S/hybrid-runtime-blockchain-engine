package reorg

import (
	"testing"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

func TestNewRingBuffer(t *testing.T) {
	rb := NewRingBuffer()
	if rb == nil {
		t.Fatal("Expected non-nil ring buffer")
	}
	if rb.Size() != 0 {
		t.Errorf("Expected size 0, got %d", rb.Size())
	}
}

func TestRingBuffer_Add(t *testing.T) {
	rb := NewRingBuffer()

	block1 := &ffi.Block{Number: 1}
	rb.Add(block1)

	if rb.Size() != 1 {
		t.Errorf("Expected size 1, got %d", rb.Size())
	}

	latest := rb.Latest()
	if latest == nil || latest.Number != 1 {
		t.Error("Latest block mismatch")
	}
}

func TestRingBuffer_MaxSize(t *testing.T) {
	rb := NewRingBuffer()

	// Add more than MaxReorgDepth blocks
	for i := 0; i < MaxReorgDepth+5; i++ {
		block := &ffi.Block{Number: uint64(i + 1)}
		rb.Add(block)
	}

	// Size should be capped at MaxReorgDepth
	if rb.Size() != MaxReorgDepth {
		t.Errorf("Expected size %d, got %d", MaxReorgDepth, rb.Size())
	}

	// Latest should be the most recent block
	latest := rb.Latest()
	if latest == nil || latest.Number != uint64(MaxReorgDepth+5) {
		t.Errorf("Expected latest block %d, got %d", MaxReorgDepth+5, latest.Number)
	}
}

func TestRingBuffer_Get(t *testing.T) {
	rb := NewRingBuffer()

	// Add 5 blocks
	for i := 0; i < 5; i++ {
		block := &ffi.Block{Number: uint64(i + 1)}
		rb.Add(block)
	}

	// Get first block (oldest)
	first := rb.Get(0)
	if first == nil || first.Number != 1 {
		t.Error("First block mismatch")
	}

	// Get last block (newest)
	last := rb.Get(4)
	if last == nil || last.Number != 5 {
		t.Error("Last block mismatch")
	}

	// Get out of bounds
	invalid := rb.Get(10)
	if invalid != nil {
		t.Error("Expected nil for out of bounds index")
	}
}

func TestRingBuffer_Truncate(t *testing.T) {
	rb := NewRingBuffer()

	// Add 5 blocks
	for i := 0; i < 5; i++ {
		block := &ffi.Block{Number: uint64(i + 1)}
		rb.Add(block)
	}

	// Truncate to 3 blocks
	rb.Truncate(3)

	if rb.Size() != 3 {
		t.Errorf("Expected size 3 after truncate, got %d", rb.Size())
	}

	latest := rb.Latest()
	if latest == nil || latest.Number != 3 {
		t.Error("Latest block should be block 3 after truncate")
	}
}

func TestNewReorgEngine(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ffiInstance := ffi.NewFFI()
	engine := NewReorgEngine(logger, ffiInstance)

	if engine == nil {
		t.Fatal("Expected non-nil reorg engine")
	}

	if engine.GetReorgCount() != 0 {
		t.Error("Expected initial reorg count to be 0")
	}
}

func TestReorgEngine_DetectReorg_EmptyBuffer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ffiInstance := ffi.NewFFI()
	engine := NewReorgEngine(logger, ffiInstance)

	block := &ffi.Block{Number: 1}
	isReorg, _, err := engine.DetectReorg(block)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if isReorg {
		t.Error("Expected no reorg with empty buffer")
	}
}

func TestReorgEngine_DetectReorg_Sequential(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ffiInstance := ffi.NewFFI()
	engine := NewReorgEngine(logger, ffiInstance)

	// Add first block
	block1 := &ffi.Block{Number: 1, ParentHash: ffi.Hash{}, Timestamp: 1700000001}
	engine.ringBuffer.Add(block1)

	// Sequential block: parent hash must match block1.Hash()
	block2 := &ffi.Block{Number: 2, ParentHash: block1.Hash(), Timestamp: 1700000002}
	isReorg, _, err := engine.DetectReorg(block2)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if isReorg {
		t.Error("Expected no reorg for sequential blocks")
	}
}

func TestReorgEngine_DetectReorg_NonSequential(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ffiInstance := ffi.NewFFI()
	engine := NewReorgEngine(logger, ffiInstance)

	// Add blocks 1, 2, 3 with proper hash chain
	block1 := &ffi.Block{Number: 1, ParentHash: ffi.Hash{}, Timestamp: 1700000001}
	block2 := &ffi.Block{Number: 2, ParentHash: block1.Hash(), Timestamp: 1700000002}
	block3 := &ffi.Block{Number: 3, ParentHash: block2.Hash(), Timestamp: 1700000003}
	engine.ringBuffer.Add(block1)
	engine.ringBuffer.Add(block2)
	engine.ringBuffer.Add(block3)

	// block5 has block2 as parent — skips block3, triggering a reorg
	block5 := &ffi.Block{Number: 5, ParentHash: block2.Hash(), Timestamp: 1700000005}
	isReorg, forkPoint, err := engine.DetectReorg(block5)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !isReorg {
		t.Error("Expected reorg detection for non-sequential block")
	}
	if forkPoint != block2.Number {
		t.Errorf("Expected fork point %d, got %d", block2.Number, forkPoint)
	}
}

func TestReorgEngine_GetReorgHistory(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ffiInstance := ffi.NewFFI()
	engine := NewReorgEngine(logger, ffiInstance)

	// Add some reorg events
	for i := 0; i < 5; i++ {
		event := ReorgEvent{
			ForkPoint: uint64(i + 1),
			Depth:     2,
		}
		engine.reorgEvents = append(engine.reorgEvents, event)
	}

	// Get last 3 events
	history := engine.GetReorgHistory(3)
	if len(history) != 3 {
		t.Errorf("Expected 3 events, got %d", len(history))
	}

	// Should be most recent events (3, 4, 5)
	if history[0].ForkPoint != 3 {
		t.Errorf("Expected first event fork point 3, got %d", history[0].ForkPoint)
	}
}

func TestReorgEngine_GetStats(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ffiInstance := ffi.NewFFI()
	engine := NewReorgEngine(logger, ffiInstance)

	// Add some blocks
	for i := 0; i < 5; i++ {
		block := &ffi.Block{Number: uint64(i + 1)}
		engine.ringBuffer.Add(block)
	}

	stats := engine.GetStats()
	if stats.BufferSize != 5 {
		t.Errorf("Expected buffer size 5, got %d", stats.BufferSize)
	}
	if stats.ReorgCount != 0 {
		t.Errorf("Expected reorg count 0, got %d", stats.ReorgCount)
	}
}

func TestReorgEngine_MaxDepthExceeded(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ffiInstance := ffi.NewFFI()
	engine := NewReorgEngine(logger, ffiInstance)

	// Fill buffer with MaxReorgDepth blocks using a proper hash chain
	prev := ffi.Hash{}
	for i := 1; i <= MaxReorgDepth; i++ {
		block := &ffi.Block{Number: uint64(i), ParentHash: prev, Timestamp: 1700000000 + uint64(i)}
		engine.ringBuffer.Add(block)
		prev = block.Hash()
	}

	// A block whose parent is not in the ring buffer — fork point not found
	orphan := &ffi.Block{Number: uint64(MaxReorgDepth + 1), ParentHash: ffi.Hash{0xff}, Timestamp: 1700000099}
	_, _, err := engine.DetectReorg(orphan)

	// Should return an error because the fork point is not in the buffer
	if err == nil {
		t.Error("Expected error when fork point not found in ring buffer")
	}
}
