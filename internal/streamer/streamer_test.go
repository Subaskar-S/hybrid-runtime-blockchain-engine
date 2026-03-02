package streamer

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewEthBlockStreamer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	streamer := NewEthBlockStreamer(logger)

	if streamer == nil {
		t.Fatal("Expected non-nil streamer")
	}

	if streamer.Blocks() == nil {
		t.Error("Expected non-nil blocks channel")
	}
}

func TestEthBlockStreamer_IsConnected(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	streamer := NewEthBlockStreamer(logger)

	if streamer.IsConnected() {
		t.Error("Expected streamer to not be connected initially")
	}
}

func TestEthBlockStreamer_Stop(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	streamer := NewEthBlockStreamer(logger)

	err := streamer.Stop()
	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

func TestEthBlockStreamer_ConnectWithRetry_InvalidURL(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	streamer := NewEthBlockStreamer(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := streamer.Start(ctx, "ws://invalid-url:9999")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestValidateBlock(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	streamer := NewEthBlockStreamer(logger)

	tests := []struct {
		name      string
		block     *ffi.Block
		wantValid bool
	}{
		{
			name: "valid block",
			block: &ffi.Block{
				Number:       1,
				ParentHash:   ffi.Hash{},
				Timestamp:    1234567890,
				Transactions: []ffi.Transaction{},
			},
			wantValid: true,
		},
		{
			name: "zero block number",
			block: &ffi.Block{
				Number:       0,
				ParentHash:   ffi.Hash{},
				Timestamp:    1234567890,
				Transactions: []ffi.Transaction{},
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := streamer.validateBlock(tt.block)
			if valid != tt.wantValid {
				t.Errorf("validateBlock() = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestHexToBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected length
	}{
		{
			name:  "empty string",
			input: "",
			want:  0,
		},
		{
			name:  "single byte",
			input: "ff",
			want:  1,
		},
		{
			name:  "multiple bytes",
			input: "deadbeef",
			want:  4,
		},
		{
			name:  "odd length",
			input: "abc",
			want:  2, // Padded to "0abc"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hexToBytes(tt.input)
			if len(result) != tt.want {
				t.Errorf("hexToBytes() length = %d, want %d", len(result), tt.want)
			}
		})
	}
}
