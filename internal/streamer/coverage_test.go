package streamer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// newMockWSServer creates a test WebSocket server that sends one message then closes.
func newMockWSServer(t *testing.T, messages [][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the subscribe message
		_, _, _ = conn.ReadMessage()

		// Send each message
		for _, msg := range messages {
			conn.WriteMessage(websocket.TextMessage, msg)
		}
		// Close cleanly
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
}

func TestParseBlockMessage_ValidBlock(t *testing.T) {
	logger := zap.NewNop()
	s := NewEthBlockStreamer(logger)

	msg := map[string]interface{}{
		"params": map[string]interface{}{
			"result": map[string]interface{}{
				"number":       "0x1",
				"parentHash":   "0x" + strings.Repeat("ab", 32),
				"timestamp":    "0x5f5e100",
				"transactions": []string{},
			},
		},
	}
	data, _ := json.Marshal(msg)

	block, err := s.parseBlockMessage(data)
	if err != nil {
		t.Fatalf("parseBlockMessage: %v", err)
	}
	if block == nil {
		t.Fatal("expected non-nil block")
	}
	if block.Number != 1 {
		t.Errorf("expected block number 1, got %d", block.Number)
	}
}

func TestParseBlockMessage_NotABlock(t *testing.T) {
	logger := zap.NewNop()
	s := NewEthBlockStreamer(logger)

	// A message with no block number (e.g. subscription confirmation)
	msg := map[string]interface{}{
		"id":     1,
		"result": "0xsubscriptionid",
	}
	data, _ := json.Marshal(msg)

	block, err := s.parseBlockMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block != nil {
		t.Error("expected nil block for non-block message")
	}
}

func TestParseBlockMessage_InvalidJSON(t *testing.T) {
	logger := zap.NewNop()
	s := NewEthBlockStreamer(logger)

	_, err := s.parseBlockMessage([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestStreamBlocks_ReceivesBlock(t *testing.T) {
	// Build a valid newHeads notification
	blockMsg := map[string]interface{}{
		"params": map[string]interface{}{
			"result": map[string]interface{}{
				"number":       "0xa",
				"parentHash":   "0x" + strings.Repeat("cd", 32),
				"timestamp":    "0x5f5e200",
				"transactions": []string{},
			},
		},
	}
	data, _ := json.Marshal(blockMsg)

	srv := newMockWSServer(t, [][]byte{data})
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	logger := zap.NewNop()
	s := NewEthBlockStreamer(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := s.Start(ctx, wsURL); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	select {
	case block, ok := <-s.Blocks():
		if !ok {
			t.Fatal("channel closed without receiving block")
		}
		if block.Number != 10 {
			t.Errorf("expected block 10, got %d", block.Number)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for block")
	}
}

func TestSubscribeToNewHeads(t *testing.T) {
	subscribed := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req map[string]interface{}
		if json.Unmarshal(msg, &req) == nil {
			if req["method"] == "eth_subscribe" {
				subscribed <- struct{}{}
			}
		}
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	logger := zap.NewNop()
	s := NewEthBlockStreamer(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	s.Start(ctx, wsURL)
	defer s.Stop()

	select {
	case <-subscribed:
		// success
	case <-ctx.Done():
		t.Fatal("timed out waiting for subscribe message")
	}
}
