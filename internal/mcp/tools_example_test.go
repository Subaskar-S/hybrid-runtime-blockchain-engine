package mcp_test

import (
	"fmt"

	"github.com/hybrid-runtime-blockchain-engine/internal/mcp"
	"github.com/hybrid-runtime-blockchain-engine/internal/reorg"
	"go.uber.org/zap"
)

// Example demonstrates how to register and use runtime introspection tools
func Example_runtimeTools() {
	// Create logger
	logger := zap.NewNop()

	// Create MCP server
	server := mcp.NewServer(logger, 8080)

	// Create reorg engine (nil FFI for example)
	reorgEngine := reorg.NewReorgEngine(logger, nil)

	// Register runtime tools (nil FFI for example)
	tools := mcp.RegisterRuntimeTools(server, reorgEngine, nil)

	// Record some latencies
	latencyTracker := tools.GetLatencyTracker()
	latencyTracker.Record(1.5)
	latencyTracker.Record(2.3)
	latencyTracker.Record(3.1)

	// Get latency distribution
	dist := latencyTracker.GetDistribution()
	fmt.Printf("P50: %.2f ms\n", dist["p50_ms"])
	fmt.Printf("P95: %.2f ms\n", dist["p95_ms"])
	fmt.Printf("P99: %.2f ms\n", dist["p99_ms"])
	fmt.Printf("Max: %.2f ms\n", dist["max_ms"])

	// Output:
	// P50: 2.30 ms
	// P95: 3.10 ms
	// P99: 3.10 ms
	// Max: 3.10 ms
}

// Example demonstrates the latency tracker circular buffer behavior
func Example_latencyTrackerCircularBuffer() {
	tracker := mcp.NewLatencyTracker(3)

	// Fill buffer
	tracker.Record(1.0)
	tracker.Record(2.0)
	tracker.Record(3.0)

	// Overflow - should wrap around
	tracker.Record(4.0)
	tracker.Record(5.0)

	// Get distribution (should only include last 3 values: 3.0, 4.0, 5.0)
	dist := tracker.GetDistribution()
	fmt.Printf("Max: %.1f ms\n", dist["max_ms"])

	// Output:
	// Max: 5.0 ms
}
