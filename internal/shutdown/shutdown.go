package shutdown

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Manager handles graceful shutdown of the application
type Manager struct {
	logger      *zap.Logger
	shutdownFns []func(context.Context) error
	sigChan     chan os.Signal
}

// NewManager creates a new shutdown manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger:      logger,
		shutdownFns: make([]func(context.Context) error, 0),
		sigChan:     make(chan os.Signal, 1),
	}
}

// RegisterShutdownFunc registers a function to be called during shutdown
// Functions are called in reverse order of registration (LIFO)
func (m *Manager) RegisterShutdownFunc(fn func(context.Context) error) {
	m.shutdownFns = append(m.shutdownFns, fn)
}

// WaitForShutdown blocks until a shutdown signal is received
// Returns the signal that triggered the shutdown
func (m *Manager) WaitForShutdown() os.Signal {
	// Register for SIGTERM and SIGINT
	signal.Notify(m.sigChan, syscall.SIGTERM, syscall.SIGINT)
	
	// Wait for signal
	sig := <-m.sigChan
	m.logger.Info("Shutdown signal received", zap.String("signal", sig.String()))
	
	return sig
}

// Shutdown executes all registered shutdown functions with a timeout
func (m *Manager) Shutdown(timeout time.Duration) error {
	m.logger.Info("Starting graceful shutdown", zap.Duration("timeout", timeout))
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	// Execute shutdown functions in reverse order (LIFO)
	for i := len(m.shutdownFns) - 1; i >= 0; i-- {
		fn := m.shutdownFns[i]
		
		if err := fn(ctx); err != nil {
			m.logger.Error("Shutdown function failed", zap.Error(err), zap.Int("index", i))
			return err
		}
	}
	
	m.logger.Info("Graceful shutdown completed")
	return nil
}

// Stop stops listening for shutdown signals
func (m *Manager) Stop() {
	signal.Stop(m.sigChan)
	close(m.sigChan)
}
