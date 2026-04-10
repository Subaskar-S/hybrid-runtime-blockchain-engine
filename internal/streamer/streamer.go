package streamer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hybrid-runtime-blockchain-engine/internal/ffi"
	"go.uber.org/zap"
)

// BlockStreamer defines the interface for streaming blocks from Ethereum
type BlockStreamer interface {
	Start(ctx context.Context, rpcURL string) error
	Blocks() <-chan *ffi.Block
	Stop() error
	IsConnected() bool
}

// EthBlockStreamer implements BlockStreamer for Ethereum JSON-RPC
type EthBlockStreamer struct {
	logger      *zap.Logger
	conn        *websocket.Conn
	blocks      chan *ffi.Block
	stopCh      chan struct{}
	isConnected bool
	rpcURL      string
}

// NewEthBlockStreamer creates a new Ethereum block streamer
func NewEthBlockStreamer(logger *zap.Logger) *EthBlockStreamer {
	return &EthBlockStreamer{
		logger: logger,
		blocks: make(chan *ffi.Block, 100), // Buffered channel
		stopCh: make(chan struct{}),
	}
}

// Start begins streaming blocks from the Ethereum RPC endpoint
func (s *EthBlockStreamer) Start(ctx context.Context, rpcURL string) error {
	s.rpcURL = rpcURL

	// Connect with retry
	if err := s.connectWithRetry(ctx); err != nil {
		return fmt.Errorf("failed to connect after retries: %w", err)
	}

	// Start streaming in background
	go s.streamBlocks(ctx)

	return nil
}

// Blocks returns the channel for receiving blocks
func (s *EthBlockStreamer) Blocks() <-chan *ffi.Block {
	return s.blocks
}

// Stop stops the block streamer
func (s *EthBlockStreamer) Stop() error {
	close(s.stopCh)
	
	if s.conn != nil {
		s.isConnected = false
		return s.conn.Close()
	}
	
	return nil
}

// IsConnected returns whether the streamer is connected
func (s *EthBlockStreamer) IsConnected() bool {
	return s.isConnected
}

// connectWithRetry attempts to connect with exponential backoff
func (s *EthBlockStreamer) connectWithRetry(ctx context.Context) error {
	backoff := time.Second
	maxAttempts := 5
	timeout := 30 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		s.logger.Info("attempting to connect to Ethereum RPC",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", maxAttempts),
			zap.String("rpc_url", s.rpcURL))

		// Create connection with timeout
		connCtx, cancel := context.WithTimeout(ctx, timeout)
		conn, _, err := websocket.DefaultDialer.DialContext(connCtx, s.rpcURL, nil)
		cancel()

		if err == nil {
			s.conn = conn
			s.isConnected = true
			s.logger.Info("successfully connected to Ethereum RPC",
				zap.String("rpc_url", s.rpcURL))
			return nil
		}

		s.logger.Error("connection attempt failed",
			zap.Int("attempt", attempt),
			zap.Error(err))

		// Don't sleep after last attempt
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2 // Exponential backoff
			}
		}
	}

	return fmt.Errorf("max retry attempts exceeded")
}

// streamBlocks continuously streams blocks from Ethereum
func (s *EthBlockStreamer) streamBlocks(ctx context.Context) {
	defer close(s.blocks)

	// Subscribe to new block headers
	if err := s.subscribeToNewHeads(); err != nil {
		s.logger.Error("failed to subscribe to new heads", zap.Error(err))
		return
	}

	for {
		// ReadMessage blocks until a message arrives or the connection closes —
		// no busy-wait needed.
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case <-ctx.Done():
				s.logger.Info("context cancelled, stopping block stream")
			case <-s.stopCh:
				s.logger.Info("stop signal received, stopping block stream")
			default:
				s.logger.Error("failed to read message", zap.Error(err))
				s.isConnected = false
			}
			return
		}

		// Check for cancellation after each message
		select {
		case <-ctx.Done():
			s.logger.Info("context cancelled, stopping block stream")
			return
		case <-s.stopCh:
			s.logger.Info("stop signal received, stopping block stream")
			return
		default:
		}

		// Parse and process block
		block, err := s.parseBlockMessage(message)
		if err != nil {
			s.logger.Error("failed to parse block message", zap.Error(err))
			continue
		}

		if block != nil && s.validateBlock(block) {
			select {
			case s.blocks <- block:
				s.logger.Debug("forwarded block", zap.Uint64("block_number", block.Number))
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			}
		}
	}
}

// subscribeToNewHeads subscribes to new block headers via JSON-RPC
func (s *EthBlockStreamer) subscribeToNewHeads() error {
	subscribeMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_subscribe",
		"params":  []string{"newHeads"},
	}

	data, err := json.Marshal(subscribeMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal subscribe message: %w", err)
	}

	if err := s.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send subscribe message: %w", err)
	}

	s.logger.Info("subscribed to newHeads")
	return nil
}

// parseBlockMessage parses a block from a WebSocket message
func (s *EthBlockStreamer) parseBlockMessage(message []byte) (*ffi.Block, error) {
	var response struct {
		Params struct {
			Result struct {
				Number       string   `json:"number"`
				ParentHash   string   `json:"parentHash"`
				Timestamp    string   `json:"timestamp"`
				Transactions []string `json:"transactions"`
			} `json:"result"`
		} `json:"params"`
	}

	if err := json.Unmarshal(message, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Check if this is a block notification
	if response.Params.Result.Number == "" {
		return nil, nil // Not a block message
	}

	// Parse block number
	blockNumber := new(big.Int)
	blockNumber.SetString(response.Params.Result.Number[2:], 16) // Remove "0x" prefix

	// Parse parent hash
	var parentHash ffi.Hash
	if len(response.Params.Result.ParentHash) >= 2 {
		parentHashBytes := hexToBytes(response.Params.Result.ParentHash[2:])
		if len(parentHashBytes) == 32 {
			copy(parentHash[:], parentHashBytes)
		}
	}

	// Parse timestamp
	timestamp := new(big.Int)
	timestamp.SetString(response.Params.Result.Timestamp[2:], 16)

	// For now, we'll create blocks with empty transactions
	// In a full implementation, we'd fetch full block data
	block := &ffi.Block{
		Number:       blockNumber.Uint64(),
		ParentHash:   parentHash,
		Timestamp:    timestamp.Uint64(),
		Transactions: []ffi.Transaction{},
	}

	return block, nil
}

// validateBlock validates a block before forwarding
func (s *EthBlockStreamer) validateBlock(block *ffi.Block) bool {
	// Validate block number is present
	if block.Number == 0 {
		s.logger.Warn("block has zero block number, skipping")
		return false
	}

	// Validate parent hash length
	if len(block.ParentHash) != 32 {
		s.logger.Warn("block has invalid parent hash length",
			zap.Uint64("block_number", block.Number),
			zap.Int("parent_hash_length", len(block.ParentHash)))
		return false
	}

	return true
}

// hexToBytes converts a hex string to bytes using the standard library.
func hexToBytes(s string) []byte {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}
