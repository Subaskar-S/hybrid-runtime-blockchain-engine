package logging

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a new production-ready zap logger.
// The log level is read from the LOG_LEVEL environment variable
// (trace|debug|info|warn|error). Defaults to info if unset or invalid.
func NewLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()

	// Configure log level from LOG_LEVEL env var
	level := levelFromEnv()
	config.Level = zap.NewAtomicLevelAt(level)
	
	// Configure encoder to include timestamp, level, and component name
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.NameKey = "component"
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.StacktraceKey = "stacktrace"
	config.EncoderConfig.CallerKey = "caller"
	
	// Use ISO8601 time format
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	// Use lowercase level names
	config.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	
	// Use short caller format (file:line)
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	
	// Build logger
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}
	
	return logger, nil
}

// NewComponentLogger creates a logger with a component name
func NewComponentLogger(componentName string) (*zap.Logger, error) {
	logger, err := NewLogger()
	if err != nil {
		return nil, err
	}
	
	return logger.Named(componentName), nil
}

// NewDevelopmentLogger creates a development logger with more verbose output
func NewDevelopmentLogger() (*zap.Logger, error) {
	config := zap.NewDevelopmentConfig()
	
	// Configure encoder
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.NameKey = "component"
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.StacktraceKey = "stacktrace"
	config.EncoderConfig.CallerKey = "caller"
	
	// Use ISO8601 time format
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	// Use colored level names for development
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	
	// Use short caller format
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	
	// Build logger
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}
	
	return logger, nil
}

// levelFromEnv reads LOG_LEVEL from the environment and returns the
// corresponding zapcore.Level. Valid values: trace, debug, info, warn, error.
// "trace" is mapped to debug (zap has no trace level).
// Returns zapcore.InfoLevel for unrecognised or empty values.
func levelFromEnv() zapcore.Level {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	switch raw {
	case "trace", "debug":
		return zapcore.DebugLevel
	case "info", "":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		// Unknown value — log a warning to stderr and fall back to info
		fmt.Fprintf(os.Stderr, "logging: unknown LOG_LEVEL %q, defaulting to info\n", raw)
		return zapcore.InfoLevel
	}
}
