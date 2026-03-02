package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

// MockProcessor is a mock block processor for testing
type MockProcessor struct {
	processCount int32
	shouldFail   bool
	shouldPanic  bool
	delay        time.Duration
}

func (m *MockProcessor) ProcessBlock(block *ffi.Block) error {
	atomic.AddInt32(&m.processCount, 1)
	
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	
	if m.shouldPanic {
		panic("mock panic")
	}
	
	if m.shouldFail {
		return errors.New("mock error")
	}
	
	return nil
}

func TestNewPool(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{}
	pool := NewPool(logger, processor)

	if pool == nil {
		t.Fatal("Expected non-nil pool")
	}
}

func TestPool_Start_InvalidWorkerCount(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{}
	pool := NewPool(logger, processor)

	tests := []int{0, -1, 257, 1000}
	for _, count := range tests {
		err := pool.Start(context.Background(), count)
		if err == nil {
			t.Errorf("Expected error for worker count %d", count)
		}
	}
}

func TestPool_Start_ValidWorkerCount(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{}
	pool := NewPool(logger, processor)

	tests := []int{1, 4, 8, 256}
	for _, count := range tests {
		pool = NewPool(logger, processor)
		err := pool.Start(context.Background(), count)
		if err != nil {
			t.Errorf("Unexpected error for worker count %d: %v", count, err)
		}
		pool.Stop(context.Background())
	}
}

func TestPool_Submit_ProcessBlock(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{}
	pool := NewPool(logger, processor)

	ctx := context.Background()
	if err := pool.Start(ctx, 2); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pool.Stop(ctx)

	// Submit a block
	block := &ffi.Block{
		Number:       1,
		ParentHash:   ffi.Hash{},
		Timestamp:    1234567890,
		Transactions: []ffi.Transaction{},
	}

	if err := pool.Submit(block); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&processor.processCount) != 1 {
		t.Errorf("Expected 1 processed block, got %d", processor.processCount)
	}
}

func TestPool_Backpressure(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{delay: 100 * time.Millisecond}
	pool := NewPool(logger, processor)

	ctx := context.Background()
	if err := pool.Start(ctx, 1); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pool.Stop(ctx)

	// Fill the channel (size = 2 for 1 worker)
	for i := 0; i < 2; i++ {
		block := &ffi.Block{Number: uint64(i + 1)}
		if err := pool.Submit(block); err != nil {
			t.Fatalf("Submit %d failed: %v", i, err)
		}
	}

	// Next submit should block briefly
	done := make(chan bool)
	go func() {
		block := &ffi.Block{Number: 3}
		pool.Submit(block)
		done <- true
	}()

	select {
	case <-done:
		// Success - submit completed
	case <-time.After(500 * time.Millisecond):
		t.Error("Submit blocked for too long (backpressure not working)")
	}
}

func TestPool_PanicRecovery(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{shouldPanic: true}
	pool := NewPool(logger, processor)

	ctx := context.Background()
	if err := pool.Start(ctx, 2); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pool.Stop(ctx)

	// Submit a block that will cause panic
	block := &ffi.Block{Number: 1}
	if err := pool.Submit(block); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for panic handling
	time.Sleep(200 * time.Millisecond)

	stats := pool.GetStats()
	if stats.PanicCount == 0 {
		t.Error("Expected panic count > 0")
	}

	// Worker should still be active after panic
	if stats.ActiveWorkers == 0 {
		t.Error("Expected active workers after panic recovery")
	}
}

func TestPool_GracefulShutdown(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{delay: 50 * time.Millisecond}
	pool := NewPool(logger, processor)

	ctx := context.Background()
	if err := pool.Start(ctx, 2); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Submit some blocks
	for i := 0; i < 5; i++ {
		block := &ffi.Block{Number: uint64(i + 1)}
		pool.Submit(block)
	}

	// Stop with timeout
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pool.Stop(stopCtx); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify blocks were processed
	if atomic.LoadInt32(&processor.processCount) == 0 {
		t.Error("Expected some blocks to be processed before shutdown")
	}
}

func TestPool_StopTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{delay: 5 * time.Second} // Long delay
	pool := NewPool(logger, processor)

	ctx := context.Background()
	if err := pool.Start(ctx, 1); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Submit a block with long processing time
	block := &ffi.Block{Number: 1}
	pool.Submit(block)

	// Stop with short timeout
	stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := pool.Stop(stopCtx)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestPool_ActiveWorkers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{}
	pool := NewPool(logger, processor)

	if pool.ActiveWorkers() != 0 {
		t.Error("Expected 0 active workers before start")
	}

	ctx := context.Background()
	if err := pool.Start(ctx, 4); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pool.Stop(ctx)

	// Wait for workers to start
	time.Sleep(50 * time.Millisecond)

	active := pool.ActiveWorkers()
	if active != 4 {
		t.Errorf("Expected 4 active workers, got %d", active)
	}
}

func TestPool_GetStats(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockProcessor{}
	pool := NewPool(logger, processor)

	ctx := context.Background()
	if err := pool.Start(ctx, 3); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pool.Stop(ctx)

	// Submit blocks
	for i := 0; i < 10; i++ {
		block := &ffi.Block{Number: uint64(i + 1)}
		pool.Submit(block)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	stats := pool.GetStats()
	if stats.NumWorkers != 3 {
		t.Errorf("Expected 3 workers, got %d", stats.NumWorkers)
	}
	if stats.ProcessedBlocks == 0 {
		t.Error("Expected some processed blocks")
	}
}
