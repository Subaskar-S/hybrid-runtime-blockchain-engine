package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestServer_HandleRequest_Success(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	// Register a test handler
	server.RegisterTool("test_tool", func(params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"result": "success",
			"param":  params["test_param"],
		}, nil
	})

	// Create request
	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test_tool",
		Params: map[string]interface{}{
			"test_param": "test_value",
		},
		ID: 1,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.Error != nil {
		t.Errorf("expected no error, got %+v", resp.Error)
	}

	if resp.ID != float64(1) {
		t.Errorf("expected ID 1, got %v", resp.ID)
	}
}

func TestServer_HandleRequest_MethodNotFound(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "nonexistent_tool",
		Params:  map[string]interface{}{},
		ID:      1,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	var resp JSONRPCResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error == nil {
		t.Error("expected error, got nil")
	}

	if resp.Error.Code != MethodNotFound {
		t.Errorf("expected error code %d, got %d", MethodNotFound, resp.Error.Code)
	}
}

func TestServer_HandleRequest_InvalidJSONRPCVersion(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	reqBody := JSONRPCRequest{
		JSONRPC: "1.0",
		Method:  "test_tool",
		Params:  map[string]interface{}{},
		ID:      1,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	var resp JSONRPCResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error == nil {
		t.Error("expected error, got nil")
	}

	if resp.Error.Code != InvalidRequest {
		t.Errorf("expected error code %d, got %d", InvalidRequest, resp.Error.Code)
	}
}

func TestServer_HandleRequest_InvalidMethod(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	var resp JSONRPCResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error == nil {
		t.Error("expected error, got nil")
	}

	if resp.Error.Code != InvalidRequest {
		t.Errorf("expected error code %d, got %d", InvalidRequest, resp.Error.Code)
	}
}

func TestServer_HandleRequest_ParseError(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	var resp JSONRPCResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error == nil {
		t.Error("expected error, got nil")
	}

	if resp.Error.Code != ParseError {
		t.Errorf("expected error code %d, got %d", ParseError, resp.Error.Code)
	}
}

func TestServer_HandleRequest_RateLimitExceeded(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)
	server.rateLimiter = NewRateLimiter(2, time.Minute) // Limit to 2 requests

	server.RegisterTool("test_tool", func(params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"result": "success"}, nil
	})

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test_tool",
		Params:  map[string]interface{}{},
		ID:      1,
	}

	body, _ := json.Marshal(reqBody)

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		w := httptest.NewRecorder()
		server.handleRequest(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, w.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleRequest(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", w.Code)
	}
}

func TestServer_HandleRequest_ValidationError(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	server.RegisterTool("run_load_test", func(params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"result": "success"}, nil
	})

	// Invalid parameters (tps out of range)
	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "run_load_test",
		Params: map[string]interface{}{
			"tps":              20000, // Exceeds max of 10000
			"duration_seconds": 60,
		},
		ID: 1,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp JSONRPCResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error == nil {
		t.Error("expected error, got nil")
	}

	if resp.Error.Code != InvalidParams {
		t.Errorf("expected error code %d, got %d", InvalidParams, resp.Error.Code)
	}
}

func TestServer_HandleRequest_HandlerError(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	server.RegisterTool("error_tool", func(params map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("handler error")
	})

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "error_tool",
		Params:  map[string]interface{}{},
		ID:      1,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleRequest(w, req)

	var resp JSONRPCResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error == nil {
		t.Error("expected error, got nil")
	}

	if resp.Error.Code != InternalError {
		t.Errorf("expected error code %d, got %d", InternalError, resp.Error.Code)
	}
}

func TestServer_RegisterTool(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8080)

	handler := func(params map[string]interface{}) (interface{}, error) {
		return nil, nil
	}

	server.RegisterTool("test_tool", handler)

	if _, exists := server.handlers["test_tool"]; !exists {
		t.Error("tool not registered")
	}
}

func TestServer_StartStop(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8081)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	if err := server.Stop(); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}
}

func TestServer_BindsToLocalhost(t *testing.T) {
	logger := zap.NewNop()
	server := NewServer(logger, 8082)

	// Verify server address is localhost
	if server.server.Addr != "127.0.0.1:8082" {
		t.Errorf("expected server to bind to 127.0.0.1:8082, got %s", server.server.Addr)
	}
}
