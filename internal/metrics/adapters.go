package metrics

import (
	"time"
)

// Note: Adapter implementations are in adapters_impl.go to avoid CGO dependencies in tests

// DurationFromMs converts milliseconds to time.Duration
func DurationFromMs(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}
