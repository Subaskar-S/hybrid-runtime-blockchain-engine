package worker

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

// BlockProcessor defines the interface for processing blocks
type BlockProcessor interface {
	ProcessBlock(block *ffi.Block) error
}

// WorkerPool manages concurrent block processing
type WorkerPool interface {
	Start(ctx context.Context, numWorkers int) error
	Submit(block *ffi.Block) error
	Stop(ctx context.Context) error
	ActiveWorkers() int
}

// Pool implements WorkerPool
type Pool struct {
	logger          *zap.Logger
	processor       BlockProcessor
	blocks          chan *ffi.Block
	stopCh          chan struct{}
	wg              sync.WaitGroup
	activeWorkers   int32
	numWorkers      int
	panicCount      int64
	processedBlocks int64
}

// NewPool creates a new worker pool
func NewPool(logger *zap.Logger, processor BlockProcessor) *Pool {
	return &Pool{
		logger:    logger,
		processor: processor,
		stopCh:    make(chan struct{}),
	}
}

// Start initializes and starts the worker pool
func (p *Pool) Start(ctx context.Context, numWorkers int) error {
	if numWorkers < 1 || numWorkers > 256 {
		return fmt.Errorf("invalid worker count: %d (must be between 1 and 256)", numWorkers)
	}

	p.numWorkers = numWorkers
	// Bounded channel with size 2x worker count for backpressure
	p.blocks = make(chan *ffi.Block, numWorkers*2)

	p.logger.Info("starting worker pool",
		zap.Int("num_workers", numWorkers),
		zap.Int("channel_size", numWorkers*2))

	// Start workers
	for i := 0; i < numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}

	return nil
}

// Submit submits a block for processing
// Blocks when channel is full (backpressure)
func (p *Pool) Submit(block *ffi.Block) error {
	select {
	case <-p.stopCh:
		return fmt.Errorf("worker pool is stopped")
	case p.blocks <- block:
		return nil
	}
}

// Stop gracefully stops the worker pool
func (p *Pool) Stop(ctx context.Context) error {
	p.logger.Info("stopping worker pool")

	// Signal stop — workers will drain remaining items then exit.
	// We close stopCh first so Submit() returns an error immediately,
	// then close blocks so workers exit their range loop cleanly.
	close(p.stopCh)
	close(p.blocks)

	// Wait for workers to complete with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.logger.Info("worker pool stopped gracefully",
			zap.Int64("processed_blocks", atomic.LoadInt64(&p.processedBlocks)),
			zap.Int64("panic_count", atomic.LoadInt64(&p.panicCount)))
		return nil
	case <-ctx.Done():
		p.logger.Warn("worker pool stop timeout exceeded")
		return fmt.Errorf("stop timeout exceeded")
	}
}

// ActiveWorkers returns the number of currently active workers
func (p *Pool) ActiveWorkers() int {
	return int(atomic.LoadInt32(&p.activeWorkers))
}

// worker is the main worker goroutine
func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()

	p.logger.Debug("worker started", zap.Int("worker_id", id))
	atomic.AddInt32(&p.activeWorkers, 1)
	defer atomic.AddInt32(&p.activeWorkers, -1)

	// Range over the channel — exits cleanly when channel is closed by Stop().
	// We also respect ctx cancellation inside processBlockSafe.
	for block := range p.blocks {
		select {
		case <-ctx.Done():
			p.logger.Debug("worker context cancelled", zap.Int("worker_id", id))
			return
		default:
		}
		p.processBlockSafe(ctx, id, block)
	}

	p.logger.Debug("worker channel closed", zap.Int("worker_id", id))
}

// processBlockSafe processes a block with panic recovery.
// On panic the worker increments the panic counter and returns; the caller
// (worker goroutine) will exit and the pool will restart it.
func (p *Pool) processBlockSafe(ctx context.Context, workerID int, block *ffi.Block) {
	defer func() {
		if r := recover(); r != nil {
			p.handlePanic(workerID, r)
		}
	}()

	start := time.Now()

	if err := p.processor.ProcessBlock(block); err != nil {
		p.logger.Error("block processing failed",
			zap.Int("worker_id", workerID),
			zap.Uint64("block_number", block.Number),
			zap.Error(err))
		return
	}

	atomic.AddInt64(&p.processedBlocks, 1)

	p.logger.Debug("block processed successfully",
		zap.Int("worker_id", workerID),
		zap.Uint64("block_number", block.Number),
		zap.Duration("duration", time.Since(start)))
}

// handlePanic handles a panic from a worker
func (p *Pool) handlePanic(workerID int, r interface{}) {
	atomic.AddInt64(&p.panicCount, 1)
	
	p.logger.Error("worker panic",
		zap.Int("worker_id", workerID),
		zap.Any("panic", r),
		zap.String("stack", string(debug.Stack())))
}

// Stats returns worker pool statistics
type Stats struct {
	NumWorkers      int
	ActiveWorkers   int
	ProcessedBlocks int64
	PanicCount      int64
	QueueDepth      int
}

// GetStats returns current worker pool statistics
func (p *Pool) GetStats() Stats {
	return Stats{
		NumWorkers:      p.numWorkers,
		ActiveWorkers:   p.ActiveWorkers(),
		ProcessedBlocks: atomic.LoadInt64(&p.processedBlocks),
		PanicCount:      atomic.LoadInt64(&p.panicCount),
		QueueDepth:      len(p.blocks),
	}
}
