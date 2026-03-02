package shutdown

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewManager(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	require.NotNil(t, manager)
	assert.NotNil(t, manager.logger)
	assert.NotNil(t, manager.shutdownFns)
	assert.NotNil(t, manager.sigChan)
	assert.Len(t, manager.shutdownFns, 0)
}

func TestRegisterShutdownFunc(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Register shutdown functions
	fn1 := func(ctx context.Context) error { return nil }
	fn2 := func(ctx context.Context) error { return nil }
	
	manager.RegisterShutdownFunc(fn1)
	manager.RegisterShutdownFunc(fn2)
	
	assert.Len(t, manager.shutdownFns, 2)
}

func TestShutdown_Success(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Track execution order
	var executionOrder []int
	
	// Register shutdown functions
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		executionOrder = append(executionOrder, 1)
		return nil
	})
	
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		executionOrder = append(executionOrder, 2)
		return nil
	})
	
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		executionOrder = append(executionOrder, 3)
		return nil
	})
	
	// Execute shutdown
	err := manager.Shutdown(5 * time.Second)
	require.NoError(t, err)
	
	// Verify LIFO order (3, 2, 1)
	assert.Equal(t, []int{3, 2, 1}, executionOrder)
}

func TestShutdown_WithError(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	testErr := errors.New("shutdown error")
	
	// Register shutdown functions
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		return nil
	})
	
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		return testErr
	})
	
	// Execute shutdown
	err := manager.Shutdown(5 * time.Second)
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
}

func TestShutdown_Timeout(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Register a slow shutdown function
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		select {
		case <-time.After(2 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	
	// Execute shutdown with short timeout
	err := manager.Shutdown(100 * time.Millisecond)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestShutdown_ContextPropagation(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	var receivedCtx context.Context
	
	// Register shutdown function that captures context
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		receivedCtx = ctx
		return nil
	})
	
	// Execute shutdown
	err := manager.Shutdown(5 * time.Second)
	require.NoError(t, err)
	
	// Verify context was passed
	assert.NotNil(t, receivedCtx)
	
	// Verify context has deadline
	_, hasDeadline := receivedCtx.Deadline()
	assert.True(t, hasDeadline)
}

func TestWaitForShutdown_SIGTERM(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping signal test in short mode")
	}
	
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Send SIGTERM in a goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		manager.sigChan <- syscall.SIGTERM
	}()
	
	// Wait for shutdown
	sig := manager.WaitForShutdown()
	assert.Equal(t, syscall.SIGTERM, sig)
}

func TestWaitForShutdown_SIGINT(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping signal test in short mode")
	}
	
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Send SIGINT in a goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		manager.sigChan <- syscall.SIGINT
	}()
	
	// Wait for shutdown
	sig := manager.WaitForShutdown()
	assert.Equal(t, syscall.SIGINT, sig)
}

func TestStop(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Stop should not panic
	manager.Stop()
	
	// Verify channel is closed
	_, ok := <-manager.sigChan
	assert.False(t, ok)
}

func TestShutdown_EmptyFunctions(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Execute shutdown with no registered functions
	err := manager.Shutdown(5 * time.Second)
	assert.NoError(t, err)
}

func TestShutdown_MultipleErrors(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	
	// Register shutdown functions that error
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		return err1
	})
	
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		return err2
	})
	
	// Execute shutdown - should return first error encountered (LIFO order)
	err := manager.Shutdown(5 * time.Second)
	assert.Error(t, err)
	assert.Equal(t, err2, err) // err2 is executed first (LIFO)
}

func TestShutdown_PartialExecution(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	var executed []int
	testErr := errors.New("shutdown error")
	
	// Register shutdown functions
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		executed = append(executed, 1)
		return nil
	})
	
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		executed = append(executed, 2)
		return testErr // This will stop execution
	})
	
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		executed = append(executed, 3)
		return nil
	})
	
	// Execute shutdown
	err := manager.Shutdown(5 * time.Second)
	assert.Error(t, err)
	
	// Only function 3 and 2 should execute (LIFO order, stops at error)
	assert.Equal(t, []int{3, 2}, executed)
}

func TestShutdown_LongRunning(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	// Register a function that takes some time but completes
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})
	
	// Execute shutdown with sufficient timeout
	start := time.Now()
	err := manager.Shutdown(1 * time.Second)
	duration := time.Since(start)
	
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, duration, 50*time.Millisecond)
	assert.Less(t, duration, 1*time.Second)
}

func TestShutdown_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	var ctxCancelled bool
	
	// Register a function that checks if context is cancelled
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			ctxCancelled = true
			return ctx.Err()
		case <-time.After(2 * time.Second):
			return nil
		}
	})
	
	// Execute shutdown with short timeout
	err := manager.Shutdown(100 * time.Millisecond)
	
	assert.Error(t, err)
	assert.True(t, ctxCancelled)
}

func TestWaitForShutdown_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	logger := zap.NewNop()
	manager := NewManager(logger)
	
	var shutdownExecuted bool
	
	// Register shutdown function
	manager.RegisterShutdownFunc(func(ctx context.Context) error {
		shutdownExecuted = true
		return nil
	})
	
	// Start goroutine to wait for shutdown
	done := make(chan bool)
	go func() {
		sig := manager.WaitForShutdown()
		assert.Equal(t, syscall.SIGTERM, sig)
		
		err := manager.Shutdown(5 * time.Second)
		assert.NoError(t, err)
		
		done <- true
	}()
	
	// Send signal
	time.Sleep(100 * time.Millisecond)
	manager.sigChan <- syscall.SIGTERM
	
	// Wait for completion
	select {
	case <-done:
		assert.True(t, shutdownExecuted)
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown did not complete in time")
	}
}
