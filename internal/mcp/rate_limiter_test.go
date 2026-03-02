package mcp

import (
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Second)

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.Allow("test_tool") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	if rl.Allow("test_tool") {
		t.Error("4th request should be denied")
	}
}

func TestRateLimiter_WindowReset(t *testing.T) {
	rl := NewRateLimiter(2, 100*time.Millisecond)

	// Use up the limit
	rl.Allow("test_tool")
	rl.Allow("test_tool")

	// Should be denied
	if rl.Allow("test_tool") {
		t.Error("request should be denied")
	}

	// Wait for window to reset
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow("test_tool") {
		t.Error("request should be allowed after window reset")
	}
}

func TestRateLimiter_PerToolLimit(t *testing.T) {
	rl := NewRateLimiter(2, time.Second)

	// Use up limit for tool1
	rl.Allow("tool1")
	rl.Allow("tool1")

	// tool1 should be denied
	if rl.Allow("tool1") {
		t.Error("tool1 should be denied")
	}

	// tool2 should still be allowed
	if !rl.Allow("tool2") {
		t.Error("tool2 should be allowed")
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter(2, time.Second)

	// Use up the limit
	rl.Allow("test_tool")
	rl.Allow("test_tool")

	// Should be denied
	if rl.Allow("test_tool") {
		t.Error("request should be denied")
	}

	// Reset
	rl.Reset()

	// Should be allowed again
	if !rl.Allow("test_tool") {
		t.Error("request should be allowed after reset")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(100, time.Second)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				rl.Allow("test_tool")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have used up the limit
	if rl.Allow("test_tool") {
		t.Error("limit should be exhausted")
	}
}

func TestRateLimiter_ZeroLimit(t *testing.T) {
	rl := NewRateLimiter(0, time.Second)

	// All requests should be denied
	if rl.Allow("test_tool") {
		t.Error("request should be denied with zero limit")
	}
}

func TestRateLimiter_MultipleTools(t *testing.T) {
	rl := NewRateLimiter(2, time.Second)

	// Each tool should have independent limits
	tools := []string{"tool1", "tool2", "tool3"}
	
	for _, tool := range tools {
		// First 2 requests allowed
		if !rl.Allow(tool) {
			t.Errorf("%s: first request should be allowed", tool)
		}
		if !rl.Allow(tool) {
			t.Errorf("%s: second request should be allowed", tool)
		}
		
		// Third request denied
		if rl.Allow(tool) {
			t.Errorf("%s: third request should be denied", tool)
		}
	}
}
