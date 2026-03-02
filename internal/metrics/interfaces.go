package metrics

// WorkerPoolStats provides worker pool statistics
type WorkerPoolStats interface {
	GetStats() WorkerStats
}

// WorkerStats represents worker pool statistics
type WorkerStats struct {
	NumWorkers      int
	ActiveWorkers   int
	ProcessedBlocks int64
	PanicCount      int64
	QueueDepth      int
}

// ReorgEngineStats provides reorg engine statistics
type ReorgEngineStats interface {
	GetStats() ReorgStats
}

// ReorgStats represents reorg engine statistics
type ReorgStats struct {
	ReorgCount   int64
	BufferSize   int
	RecentReorgs []ReorgEvent
}

// ReorgEvent represents a reorganization event
type ReorgEvent struct {
	ForkPoint          uint64
	Depth              int
	RollbackDurationMs float64
}

// RustCoreStats provides Rust core statistics
type RustCoreStats interface {
	GetStats() (*RustStats, error)
	GetStateRoot() ([32]byte, error)
}

// RustStats represents Rust core statistics
type RustStats struct {
	BlockNumber      uint64
	StateSize        int
	HistoryLength    int
	MemoryUsageBytes int
}

// BlockStreamerHealth provides block streamer health status
type BlockStreamerHealth interface {
	IsConnected() bool
}

// WorkerPoolHealth provides worker pool health status
type WorkerPoolHealth interface {
	ActiveWorkers() int
}
