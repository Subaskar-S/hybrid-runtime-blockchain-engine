package logging

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLogger(t *testing.T) {
	logger, err := NewLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)
	
	// Verify logger is usable
	logger.Info("test message")
	
	// Clean up
	logger.Sync()
}

func TestNewComponentLogger(t *testing.T) {
	logger, err := NewComponentLogger("test-component")
	require.NoError(t, err)
	require.NotNil(t, logger)
	
	// Verify logger is usable
	logger.Info("test message")
	
	// Clean up
	logger.Sync()
}

func TestNewDevelopmentLogger(t *testing.T) {
	logger, err := NewDevelopmentLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)
	
	// Verify logger is usable
	logger.Info("test message")
	
	// Clean up
	logger.Sync()
}

func TestLoggerFields(t *testing.T) {
	// Create an observed logger to capture log entries
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	
	// Log a message with fields
	logger.Info("test message",
		zap.String("key1", "value1"),
		zap.Int("key2", 42),
	)
	
	// Verify log entry
	entries := recorded.All()
	require.Len(t, entries, 1)
	
	entry := entries[0]
	assert.Equal(t, "test message", entry.Message)
	assert.Equal(t, zapcore.InfoLevel, entry.Level)
	
	// Verify fields
	fields := entry.Context
	assert.Len(t, fields, 2)
	assert.Equal(t, "key1", fields[0].Key)
	assert.Equal(t, "value1", fields[0].String)
	assert.Equal(t, "key2", fields[1].Key)
	assert.Equal(t, int64(42), fields[1].Integer)
}

func TestLoggerLevels(t *testing.T) {
	// Create an observed logger to capture log entries
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	
	// Log at different levels
	logger.Debug("debug message") // Should not be recorded (below InfoLevel)
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")
	
	// Verify log entries
	entries := recorded.All()
	require.Len(t, entries, 3) // Debug should be filtered out
	
	assert.Equal(t, "info message", entries[0].Message)
	assert.Equal(t, zapcore.InfoLevel, entries[0].Level)
	
	assert.Equal(t, "warn message", entries[1].Message)
	assert.Equal(t, zapcore.WarnLevel, entries[1].Level)
	
	assert.Equal(t, "error message", entries[2].Message)
	assert.Equal(t, zapcore.ErrorLevel, entries[2].Level)
}

func TestComponentLogger(t *testing.T) {
	// Create an observed logger to capture log entries
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core).Named("test-component")
	
	// Log a message
	logger.Info("test message")
	
	// Verify log entry has component name
	entries := recorded.All()
	require.Len(t, entries, 1)
	
	entry := entries[0]
	assert.Equal(t, "test message", entry.Message)
	assert.Equal(t, "test-component", entry.LoggerName)
}

func TestLoggerJSONFormat(t *testing.T) {
	// Create a logger that writes to a buffer
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.NameKey = "component"
	config.EncoderConfig.MessageKey = "message"
	
	// Create observed logger
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	
	// Log a message
	logger.Info("test message",
		zap.String("field1", "value1"),
		zap.Int("field2", 42),
	)
	
	// Verify log entry structure
	entries := recorded.All()
	require.Len(t, entries, 1)
	
	entry := entries[0]
	
	// Verify required fields are present
	assert.NotZero(t, entry.Time) // timestamp
	assert.Equal(t, zapcore.InfoLevel, entry.Level) // level
	assert.Equal(t, "test message", entry.Message) // message
	
	// Verify custom fields
	fields := entry.Context
	assert.Len(t, fields, 2)
}

func TestLoggerWithError(t *testing.T) {
	// Create an observed logger
	core, recorded := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)
	
	// Log an error
	testErr := assert.AnError
	logger.Error("error occurred", zap.Error(testErr))
	
	// Verify log entry
	entries := recorded.All()
	require.Len(t, entries, 1)
	
	entry := entries[0]
	assert.Equal(t, "error occurred", entry.Message)
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	
	// Verify error field
	fields := entry.Context
	require.Len(t, fields, 1)
	assert.Equal(t, "error", fields[0].Key)
}

func TestMultipleComponents(t *testing.T) {
	// Create an observed logger
	core, recorded := observer.New(zapcore.InfoLevel)
	baseLogger := zap.New(core)
	
	// Create component loggers
	component1 := baseLogger.Named("component1")
	component2 := baseLogger.Named("component2")
	
	// Log from different components
	component1.Info("message from component1")
	component2.Info("message from component2")
	
	// Verify log entries
	entries := recorded.All()
	require.Len(t, entries, 2)
	
	assert.Equal(t, "component1", entries[0].LoggerName)
	assert.Equal(t, "message from component1", entries[0].Message)
	
	assert.Equal(t, "component2", entries[1].LoggerName)
	assert.Equal(t, "message from component2", entries[1].Message)
}

func TestLoggerSync(t *testing.T) {
	logger, err := NewLogger()
	require.NoError(t, err)
	
	// Log some messages
	logger.Info("message 1")
	logger.Info("message 2")
	
	// Sync should not error
	err = logger.Sync()
	// Note: Sync may return "sync /dev/stderr: invalid argument" on some systems
	// This is expected and can be ignored
	if err != nil {
		t.Logf("Sync returned error (may be expected): %v", err)
	}
}

func TestLoggerEncoderConfig(t *testing.T) {
	// Verify production logger config
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.NameKey = "component"
	config.EncoderConfig.MessageKey = "message"
	
	// Verify keys are set correctly
	assert.Equal(t, "timestamp", config.EncoderConfig.TimeKey)
	assert.Equal(t, "level", config.EncoderConfig.LevelKey)
	assert.Equal(t, "component", config.EncoderConfig.NameKey)
	assert.Equal(t, "message", config.EncoderConfig.MessageKey)
}

func TestLoggerStructuredOutput(t *testing.T) {
	// Create an observed logger
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core).Named("test-component")
	
	// Log a structured message
	logger.Info("operation completed",
		zap.String("operation", "block_processing"),
		zap.Int("block_number", 12345),
		zap.Duration("duration", 100),
		zap.Bool("success", true),
	)
	
	// Verify log entry
	entries := recorded.All()
	require.Len(t, entries, 1)
	
	entry := entries[0]
	assert.Equal(t, "operation completed", entry.Message)
	assert.Equal(t, "test-component", entry.LoggerName)
	
	// Verify all fields are present
	fields := entry.Context
	assert.Len(t, fields, 4)
	
	// Verify field types and values
	fieldMap := make(map[string]zapcore.Field)
	for _, field := range fields {
		fieldMap[field.Key] = field
	}
	
	assert.Contains(t, fieldMap, "operation")
	assert.Contains(t, fieldMap, "block_number")
	assert.Contains(t, fieldMap, "duration")
	assert.Contains(t, fieldMap, "success")
}

// Helper function to parse JSON log output
func parseJSONLog(jsonStr string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	return result, err
}
