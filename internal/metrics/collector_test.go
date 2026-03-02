package metrics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCollector_Start(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start collector on a test port
	port := 19090
	if err := collector.Start(ctx, port); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test /metrics endpoint
	resp, err := http.Get("http://localhost:19090/metrics")
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	// Check that some metrics are present
	bodyStr := string(body)
	expectedMetrics := []string{
		"gc_pause_seconds",
		"memory_alloc_bytes",
		"goroutine_count",
		"worker_active_count",
		"rust_apply_block_duration_seconds",
		"reorg_total",
		"blocks_processed_total",
	}

	for _, metric := range expectedMetrics {
		if !contains(bodyStr, metric) {
			t.Errorf("expected metric %s not found in response", metric)
		}
	}

	// Stop collector
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := collector.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop collector: %v", err)
	}
}

func TestCollector_RecordMetrics(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	// Test recording various metrics
	collector.RecordBlockProcessed(10 * time.Millisecond)
	collector.RecordRustApplyBlock(5 * time.Millisecond)
	collector.RecordReorg(3, 15*time.Millisecond)
	collector.RecordWorkerPanic()
	collector.UpdateWorkerStats(4, 10, 8)
	collector.UpdateRustStats(1000, 1024*1024)

	// No errors expected - just verify methods don't panic
}

func TestCollector_RegisterComponents(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	// Create mock components
	mockWorkerPool := &mockWorkerPoolStats{}
	mockReorgEngine := &mockReorgEngineStats{}
	mockRustCore := &mockRustCoreStats{}

	// Register components
	collector.RegisterWorkerPool(mockWorkerPool)
	collector.RegisterReorgEngine(mockReorgEngine)
	collector.RegisterRustCore(mockRustCore)

	// Verify components are registered
	if collector.workerPool == nil {
		t.Error("worker pool not registered")
	}
	if collector.reorgEngine == nil {
		t.Error("reorg engine not registered")
	}
	if collector.rustCore == nil {
		t.Error("rust core not registered")
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Mock implementations for testing
type mockWorkerPoolStats struct{}

func (m *mockWorkerPoolStats) GetStats() WorkerStats {
	return WorkerStats{
		NumWorkers:      8,
		ActiveWorkers:   4,
		ProcessedBlocks: 100,
		PanicCount:      0,
		QueueDepth:      5,
	}
}

type mockReorgEngineStats struct{}

func (m *mockReorgEngineStats) GetStats() ReorgStats {
	return ReorgStats{
		ReorgCount:   2,
		BufferSize:   10,
		RecentReorgs: []ReorgEvent{},
	}
}

type mockRustCoreStats struct{}

func (m *mockRustCoreStats) GetStats() (*RustStats, error) {
	return &RustStats{
		BlockNumber:      100,
		StateSize:        1000,
		HistoryLength:    50,
		MemoryUsageBytes: 1024 * 1024,
	}, nil
}

func (m *mockRustCoreStats) GetStateRoot() ([32]byte, error) {
	var root [32]byte
	for i := 0; i < 32; i++ {
		root[i] = byte(i)
	}
	return root, nil
}

type mockBlockStreamerHealth struct {
	connected bool
}

func (m *mockBlockStreamerHealth) IsConnected() bool {
	return m.connected
}

type mockWorkerPoolHealth struct {
	activeWorkers int
}

func (m *mockWorkerPoolHealth) ActiveWorkers() int {
	return m.activeWorkers
}

func TestCollector_HealthEndpoint_Healthy(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	// Register healthy components
	collector.RegisterBlockStreamer(&mockBlockStreamerHealth{connected: true})
	collector.RegisterWorkerPoolHealth(&mockWorkerPoolHealth{activeWorkers: 4})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 19091
	if err := collector.Start(ctx, port); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Test /health endpoint - should return 200
	resp, err := http.Get("http://localhost:19091/health")
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !contains(string(body), "OK") {
		t.Errorf("expected OK in response, got: %s", string(body))
	}
}

func TestCollector_HealthEndpoint_StreamerDisconnected(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	// Register disconnected streamer
	collector.RegisterBlockStreamer(&mockBlockStreamerHealth{connected: false})
	collector.RegisterWorkerPoolHealth(&mockWorkerPoolHealth{activeWorkers: 4})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 19092
	if err := collector.Start(ctx, port); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Test /health endpoint - should return 503
	resp, err := http.Get("http://localhost:19092/health")
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
}

func TestCollector_HealthEndpoint_NoActiveWorkers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	// Register with no active workers
	collector.RegisterBlockStreamer(&mockBlockStreamerHealth{connected: true})
	collector.RegisterWorkerPoolHealth(&mockWorkerPoolHealth{activeWorkers: 0})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 19093
	if err := collector.Start(ctx, port); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Test /health endpoint - should return 503
	resp, err := http.Get("http://localhost:19093/health")
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
}

func TestCollector_DebugGCEndpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 19094
	if err := collector.Start(ctx, port); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Test /debug/gc endpoint
	resp, err := http.Get("http://localhost:19094/debug/gc")
	if err != nil {
		t.Fatalf("failed to get debug/gc: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !contains(contentType, "application/json") {
		t.Errorf("expected JSON content type, got: %s", contentType)
	}

	// Parse JSON response
	var gcStats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&gcStats); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	// Verify expected fields
	expectedFields := []string{"num_gc", "pause_total_ns", "pause_ns", "last_gc"}
	for _, field := range expectedFields {
		if _, ok := gcStats[field]; !ok {
			t.Errorf("expected field %s not found in response", field)
		}
	}
}

func TestCollector_DebugStateEndpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	// Register mock Rust core
	collector.RegisterRustCore(&mockRustCoreStats{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 19095
	if err := collector.Start(ctx, port); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Test /debug/state endpoint
	resp, err := http.Get("http://localhost:19095/debug/state")
	if err != nil {
		t.Fatalf("failed to get debug/state: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !contains(contentType, "application/json") {
		t.Errorf("expected JSON content type, got: %s", contentType)
	}

	// Parse JSON response
	var stateStats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stateStats); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	// Verify expected fields
	expectedFields := []string{"state_root", "state_size", "block_number"}
	for _, field := range expectedFields {
		if _, ok := stateStats[field]; !ok {
			t.Errorf("expected field %s not found in response", field)
		}
	}

	// Verify state_root is a hex string
	stateRoot, ok := stateStats["state_root"].(string)
	if !ok {
		t.Error("state_root should be a string")
	} else if len(stateRoot) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected state_root to be 64 hex chars, got %d", len(stateRoot))
	}
}

func TestCollector_DebugStateEndpoint_NoRustCore(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	collector := NewCollector(logger)

	// Don't register Rust core

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 19096
	if err := collector.Start(ctx, port); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Test /debug/state endpoint - should return 503
	resp, err := http.Get("http://localhost:19096/debug/state")
	if err != nil {
		t.Fatalf("failed to get debug/state: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
}
