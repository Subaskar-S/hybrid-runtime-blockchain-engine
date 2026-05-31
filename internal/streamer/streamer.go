package streamer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
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
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
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
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Connect with retry
	if err := s.connectWithRetry(s.ctx); err != nil {
		return fmt.Errorf("failed to connect after retries: %w", err)
	}

	// Start streaming in background with auto-reconnection
	go s.streamWithReconnect()

	return nil
}

// Blocks returns the channel for receiving blocks
func (s *EthBlockStreamer) Blocks() <-chan *ffi.Block {
	return s.blocks
}

// Stop stops the block streamer
func (s *EthBlockStreamer) Stop() error {
	close(s.stopCh)
	if s.cancel != nil {
		s.cancel()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		s.isConnected = false
		return s.conn.Close()
	}

	return nil
}

// IsConnected returns whether the streamer is connected
func (s *EthBlockStreamer) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isConnected
}

// setConnected safely sets the connection status
func (s *EthBlockStreamer) setConnected(connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isConnected = connected
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
			s.mu.Lock()
			s.conn = conn
			s.isConnected = true
			s.mu.Unlock()
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
			case <-s.stopCh:
				return fmt.Errorf("stopped during connection retry")
			case <-time.After(backoff):
				backoff *= 2 // Exponential backoff
			}
		}
	}

	return fmt.Errorf("max retry attempts exceeded")
}

// streamWithReconnect runs the streaming loop with automatic reconnection
func (s *EthBlockStreamer) streamWithReconnect() {
	defer close(s.blocks)

	for {
		// Stream blocks until disconnection
		s.streamBlocks()

		// Check if we should stop
		select {
		case <-s.stopCh:
			s.logger.Info("stop signal received, exiting reconnect loop")
			return
		case <-s.ctx.Done():
			s.logger.Info("context cancelled, exiting reconnect loop")
			return
		default:
		}

		// Connection lost — attempt reconnection with backoff
		s.setConnected(false)
		s.logger.Warn("connection lost, attempting reconnection")

		backoff := 2 * time.Second
		maxBackoff := 60 * time.Second

		for {
			select {
			case <-s.stopCh:
				return
			case <-s.ctx.Done():
				return
			case <-time.After(backoff):
			}

			if err := s.connectWithRetry(s.ctx); err != nil {
				s.logger.Error("reconnection failed, will retry",
					zap.Error(err),
					zap.Duration("next_backoff", backoff))
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			s.logger.Info("reconnected successfully")
			break
		}
	}
}

// streamBlocks continuously streams blocks from Ethereum until an error occurs
func (s *EthBlockStreamer) streamBlocks() {
	// Subscribe to new block headers
	if err := s.subscribeToNewHeads(); err != nil {
		s.logger.Error("failed to subscribe to new heads", zap.Error(err))
		return
	}

	for {
		// ReadMessage blocks until a message arrives or the connection closes
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case <-s.ctx.Done():
				s.logger.Info("context cancelled, stopping block stream")
			case <-s.stopCh:
				s.logger.Info("stop signal received, stopping block stream")
			default:
				s.logger.Error("failed to read message, will reconnect", zap.Error(err))
				s.setConnected(false)
			}
			return
		}

		// Check for cancellation after each message
		select {
		case <-s.ctx.Done():
			s.logger.Info("context cancelled, stopping block stream")
			return
		case <-s.stopCh:
			s.logger.Info("stop signal received, stopping block stream")
			return
		default:
		}

		// Parse block header notification
		blockNumber, parentHash, timestamp, err := s.parseBlockHeader(message)
		if err != nil {
			s.logger.Debug("failed to parse block header", zap.Error(err))
			continue
		}

		if blockNumber == 0 {
			continue // Not a block notification
		}

		// Fetch full block with transactions
		block, err := s.fetchFullBlock(blockNumber, parentHash, timestamp)
		if err != nil {
			s.logger.Error("failed to fetch full block",
				zap.Uint64("block_number", blockNumber),
				zap.Error(err))
			// Fall back to header-only block (empty transactions)
			block = &ffi.Block{
				Number:       blockNumber,
				ParentHash:   parentHash,
				Timestamp:    timestamp,
				Transactions: []ffi.Transaction{},
			}
		}

		if block != nil && s.validateBlock(block) {
			select {
			case s.blocks <- block:
				s.logger.Debug("forwarded block",
					zap.Uint64("block_number", block.Number),
					zap.Int("tx_count", len(block.Transactions)))
			case <-s.ctx.Done():
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

// parseBlockHeader parses a block header from a WebSocket notification message.
// Returns blockNumber=0 if the message is not a block notification.
func (s *EthBlockStreamer) parseBlockHeader(message []byte) (uint64, ffi.Hash, uint64, error) {
	var response struct {
		Params struct {
			Result struct {
				Number     string `json:"number"`
				ParentHash string `json:"parentHash"`
				Timestamp  string `json:"timestamp"`
			} `json:"result"`
		} `json:"params"`
	}

	if err := json.Unmarshal(message, &response); err != nil {
		return 0, ffi.Hash{}, 0, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Check if this is a block notification
	if response.Params.Result.Number == "" {
		return 0, ffi.Hash{}, 0, nil
	}

	// Parse block number
	blockNumber := new(big.Int)
	if len(response.Params.Result.Number) > 2 {
		blockNumber.SetString(response.Params.Result.Number[2:], 16)
	}

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
	if len(response.Params.Result.Timestamp) > 2 {
		timestamp.SetString(response.Params.Result.Timestamp[2:], 16)
	}

	return blockNumber.Uint64(), parentHash, timestamp.Uint64(), nil
}

// fetchFullBlock fetches the full block with transactions from the RPC endpoint
func (s *EthBlockStreamer) fetchFullBlock(blockNumber uint64, parentHash ffi.Hash, timestamp uint64) (*ffi.Block, error) {
	// Build eth_getBlockByNumber request
	blockHex := fmt.Sprintf("0x%x", blockNumber)
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "eth_getBlockByNumber",
		"params":  []interface{}{blockHex, true}, // true = include full transactions
	}

	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if err := s.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response (with timeout via SetReadDeadline)
	s.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer s.conn.SetReadDeadline(time.Time{}) // Clear deadline

	for {
		_, respMsg, err := s.conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// Parse the response — check if it's our eth_getBlockByNumber response (id=2)
		var rpcResp struct {
			ID     interface{} `json:"id"`
			Result *struct {
				Number       string        `json:"number"`
				ParentHash   string        `json:"parentHash"`
				Timestamp    string        `json:"timestamp"`
				Transactions []interface{} `json:"transactions"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(respMsg, &rpcResp); err != nil {
			continue // Not a JSON-RPC response, might be a subscription notification
		}

		// Check if this is our response (id=2)
		idFloat, ok := rpcResp.ID.(float64)
		if !ok || idFloat != 2 {
			continue // Not our response, keep reading
		}

		if rpcResp.Error != nil {
			return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
		}

		if rpcResp.Result == nil {
			return nil, fmt.Errorf("null result for block %d", blockNumber)
		}

		// Parse transactions
		transactions := s.parseTransactions(rpcResp.Result.Transactions)

		block := &ffi.Block{
			Number:       blockNumber,
			ParentHash:   parentHash,
			Timestamp:    timestamp,
			Transactions: transactions,
		}

		return block, nil
	}
}

// parseTransactions parses transaction objects from the RPC response
func (s *EthBlockStreamer) parseTransactions(rawTxs []interface{}) []ffi.Transaction {
	transactions := make([]ffi.Transaction, 0, len(rawTxs))

	for _, rawTx := range rawTxs {
		txMap, ok := rawTx.(map[string]interface{})
		if !ok {
			continue
		}

		tx := ffi.Transaction{}

		// Parse "from" address
		if from, ok := txMap["from"].(string); ok && len(from) >= 2 {
			fromBytes := hexToBytes(from[2:])
			if len(fromBytes) == 20 {
				copy(tx.From[:], fromBytes)
			}
		}

		// Parse "to" address (can be nil for contract creation)
		if to, ok := txMap["to"].(string); ok && len(to) >= 2 {
			toBytes := hexToBytes(to[2:])
			if len(toBytes) == 20 {
				copy(tx.To[:], toBytes)
			}
		}

		// Parse "value"
		if value, ok := txMap["value"].(string); ok && len(value) >= 2 {
			valueBig := new(big.Int)
			valueBig.SetString(value[2:], 16)
			tx.Value = ffi.NewU256FromBigInt(valueBig)
		}

		// Parse "input" data
		if input, ok := txMap["input"].(string); ok && len(input) > 2 {
			tx.Data = hexToBytes(input[2:])
		}

		transactions = append(transactions, tx)
	}

	return transactions
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

// parseBlockMessage parses a block from a WebSocket message (used for testing compatibility)
func (s *EthBlockStreamer) parseBlockMessage(message []byte) (*ffi.Block, error) {
	blockNumber, parentHash, timestamp, err := s.parseBlockHeader(message)
	if err != nil {
		return nil, err
	}
	if blockNumber == 0 {
		return nil, nil
	}
	return &ffi.Block{
		Number:       blockNumber,
		ParentHash:   parentHash,
		Timestamp:    timestamp,
		Transactions: []ffi.Transaction{},
	}, nil
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
