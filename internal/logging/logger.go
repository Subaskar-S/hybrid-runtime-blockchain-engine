package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a new production-ready zap logger
func NewLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	
	// Configure log levels
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	
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
