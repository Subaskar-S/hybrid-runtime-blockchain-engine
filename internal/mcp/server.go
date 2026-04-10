package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
	ID      interface{}            `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// RPCError represents a JSON-RPC 2.0 error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// ToolHandler is a function that handles a tool request
type ToolHandler func(params map[string]interface{}) (interface{}, error)

// Server implements the MCP server
type Server struct {
	logger       *zap.Logger
	server       *http.Server
	handlers     map[string]ToolHandler
	rateLimiter  *RateLimiter
	validator    *InputValidator
	mu           sync.RWMutex
}

// NewServer creates a new MCP server
func NewServer(logger *zap.Logger, port int) *Server {
	s := &Server{
		logger:      logger,
		handlers:    make(map[string]ToolHandler),
		rateLimiter: NewRateLimiter(10, time.Minute), // 10 requests per minute
		validator:   NewInputValidator(logger),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return s
}

// RegisterTool registers a tool handler
func (s *Server) RegisterTool(name string, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[name] = handler
	s.logger.Info("registered MCP tool", zap.String("tool", name))
}

// Start starts the MCP server
func (s *Server) Start() error {
	s.logger.Info("starting MCP server", zap.String("addr", s.server.Addr))
	
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("MCP server error", zap.Error(err))
		}
	}()
	
	return nil
}

// Stop stops the MCP server
func (s *Server) Stop() error {
	s.logger.Info("stopping MCP server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// handleRequest handles incoming JSON-RPC requests
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		s.writeError(w, nil, InvalidRequest, "Method must be POST")
		return
	}

	// Parse JSON-RPC request
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("failed to parse request", zap.Error(err))
		s.writeError(w, nil, ParseError, "Parse error")
		return
	}

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		s.writeError(w, req.ID, InvalidRequest, "Invalid JSON-RPC version")
		return
	}

	// Check rate limit
	if !s.rateLimiter.Allow(req.Method) {
		s.logger.Warn("rate limit exceeded", zap.String("method", req.Method))
		w.WriteHeader(http.StatusTooManyRequests)
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: -32000, Message: "Rate limit exceeded"},
			ID:      req.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Validate input parameters
	if err := s.validator.Validate(req.Method, req.Params); err != nil {
		s.logger.Warn("input validation failed",
			zap.String("method", req.Method),
			zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: InvalidParams, Message: err.Error()},
			ID:      req.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Get handler
	s.mu.RLock()
	handler, exists := s.handlers[req.Method]
	s.mu.RUnlock()

	if !exists {
		s.writeError(w, req.ID, MethodNotFound, fmt.Sprintf("Method not found: %s", req.Method))
		return
	}

	// Execute handler
	result, err := handler(req.Params)
	if err != nil {
		s.logger.Error("handler error",
			zap.String("method", req.Method),
			zap.Error(err))
		s.writeError(w, req.ID, InternalError, err.Error())
		return
	}

	// Write success response
	s.writeSuccess(w, req.ID, result)
}

// writeSuccess writes a successful JSON-RPC response
func (s *Server) writeSuccess(w http.ResponseWriter, id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
	}
}

// writeError writes an error JSON-RPC response with HTTP 200 (JSON-RPC spec).
// Callers that need a non-200 HTTP status (429, 400) write the status and
// body themselves before returning.
func (s *Server) writeError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to write error response", zap.Error(err))
	}
}
