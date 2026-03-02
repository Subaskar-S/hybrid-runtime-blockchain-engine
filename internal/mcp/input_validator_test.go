package mcp

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestInputValidator_ShellCommandDetection(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name      string
		params    map[string]interface{}
		shouldErr bool
	}{
		{
			name: "safe string",
			params: map[string]interface{}{
				"param": "safe value",
			},
			shouldErr: false,
		},
		{
			name: "shell command - sh",
			params: map[string]interface{}{
				"param": "sh -c 'echo hello'",
			},
			shouldErr: true,
		},
		{
			name: "shell command - bash",
			params: map[string]interface{}{
				"param": "bash script.sh",
			},
			shouldErr: true,
		},
		{
			name: "shell command - exec",
			params: map[string]interface{}{
				"param": "exec('command')",
			},
			shouldErr: true,
		},
		{
			name: "shell command - system",
			params: map[string]interface{}{
				"param": "system('ls')",
			},
			shouldErr: true,
		},
		{
			name: "shell operator - semicolon",
			params: map[string]interface{}{
				"param": "cmd1; cmd2",
			},
			shouldErr: true,
		},
		{
			name: "shell operator - pipe",
			params: map[string]interface{}{
				"param": "cmd1 | cmd2",
			},
			shouldErr: true,
		},
		{
			name: "shell operator - backtick",
			params: map[string]interface{}{
				"param": "`command`",
			},
			shouldErr: true,
		},
		{
			name: "nested object with shell command",
			params: map[string]interface{}{
				"nested": map[string]interface{}{
					"param": "bash script.sh",
				},
			},
			shouldErr: true,
		},
		{
			name: "array with shell command",
			params: map[string]interface{}{
				"array": []interface{}{"safe", "sh -c 'cmd'"},
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate("test_tool", tc.params)
			if tc.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInputValidator_LoadTestValidation(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name      string
		params    map[string]interface{}
		shouldErr bool
	}{
		{
			name: "valid parameters",
			params: map[string]interface{}{
				"tps":              float64(1000),
				"duration_seconds": float64(60),
			},
			shouldErr: false,
		},
		{
			name: "missing tps",
			params: map[string]interface{}{
				"duration_seconds": float64(60),
			},
			shouldErr: true,
		},
		{
			name: "missing duration",
			params: map[string]interface{}{
				"tps": float64(1000),
			},
			shouldErr: true,
		},
		{
			name: "tps too low",
			params: map[string]interface{}{
				"tps":              float64(0),
				"duration_seconds": float64(60),
			},
			shouldErr: true,
		},
		{
			name: "tps too high",
			params: map[string]interface{}{
				"tps":              float64(20000),
				"duration_seconds": float64(60),
			},
			shouldErr: true,
		},
		{
			name: "duration too low",
			params: map[string]interface{}{
				"tps":              float64(1000),
				"duration_seconds": float64(0),
			},
			shouldErr: true,
		},
		{
			name: "duration too high",
			params: map[string]interface{}{
				"tps":              float64(1000),
				"duration_seconds": float64(500),
			},
			shouldErr: true,
		},
		{
			name: "tps wrong type",
			params: map[string]interface{}{
				"tps":              "1000",
				"duration_seconds": float64(60),
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate("run_load_test", tc.params)
			if tc.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInputValidator_ReorgHistoryValidation(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name      string
		params    map[string]interface{}
		shouldErr bool
	}{
		{
			name:      "no parameters",
			params:    map[string]interface{}{},
			shouldErr: false,
		},
		{
			name: "valid limit",
			params: map[string]interface{}{
				"limit": float64(10),
			},
			shouldErr: false,
		},
		{
			name: "limit too low",
			params: map[string]interface{}{
				"limit": float64(0),
			},
			shouldErr: true,
		},
		{
			name: "limit too high",
			params: map[string]interface{}{
				"limit": float64(2000),
			},
			shouldErr: true,
		},
		{
			name: "limit wrong type",
			params: map[string]interface{}{
				"limit": "10",
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate("get_reorg_history", tc.params)
			if tc.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInputValidator_ApplyBlockLatencyValidation(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name      string
		params    map[string]interface{}
		shouldErr bool
	}{
		{
			name:      "no parameters",
			params:    map[string]interface{}{},
			shouldErr: false,
		},
		{
			name: "valid limit",
			params: map[string]interface{}{
				"limit": float64(100),
			},
			shouldErr: false,
		},
		{
			name: "limit too low",
			params: map[string]interface{}{
				"limit": float64(0),
			},
			shouldErr: true,
		},
		{
			name: "limit too high",
			params: map[string]interface{}{
				"limit": float64(20000),
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate("get_apply_block_latency", tc.params)
			if tc.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInputValidator_DeterminismValidation(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name      string
		params    map[string]interface{}
		shouldErr bool
	}{
		{
			name:      "no parameters",
			params:    map[string]interface{}{},
			shouldErr: false,
		},
		{
			name: "valid block_count",
			params: map[string]interface{}{
				"block_count": float64(10),
			},
			shouldErr: false,
		},
		{
			name: "block_count too low",
			params: map[string]interface{}{
				"block_count": float64(0),
			},
			shouldErr: true,
		},
		{
			name: "block_count too high",
			params: map[string]interface{}{
				"block_count": float64(200),
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate("validate_determinism", tc.params)
			if tc.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInputValidator_ValidateType(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name         string
		value        interface{}
		expectedType string
		shouldErr    bool
	}{
		{"string valid", "test", "string", false},
		{"string invalid", 123, "string", true},
		{"number valid", float64(123), "number", false},
		{"number invalid", "123", "number", true},
		{"integer valid", float64(123), "integer", false},
		{"integer invalid", float64(123.5), "integer", true},
		{"boolean valid", true, "boolean", false},
		{"boolean invalid", "true", "boolean", true},
		{"object valid", map[string]interface{}{}, "object", false},
		{"object invalid", []interface{}{}, "object", true},
		{"array valid", []interface{}{}, "array", false},
		{"array invalid", map[string]interface{}{}, "array", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateType(tc.value, tc.expectedType)
			if tc.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInputValidator_ValidateRange(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name      string
		value     interface{}
		min       float64
		max       float64
		shouldErr bool
	}{
		{"in range", float64(50), 0, 100, false},
		{"at min", float64(0), 0, 100, false},
		{"at max", float64(100), 0, 100, false},
		{"below min", float64(-1), 0, 100, true},
		{"above max", float64(101), 0, 100, true},
		{"wrong type", "50", 0, 100, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateRange(tc.value, tc.min, tc.max)
			if tc.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestInputValidator_SanitizeString(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "null bytes",
			input:    "hello\x00world",
			expected: "helloworld",
		},
		{
			name:     "control characters",
			input:    "hello\x01\x02world",
			expected: "helloworld",
		},
		{
			name:     "preserve newline",
			input:    "hello\nworld",
			expected: "hello\nworld",
		},
		{
			name:     "preserve tab",
			input:    "hello\tworld",
			expected: "hello\tworld",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.SanitizeString(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestInputValidator_ContainsShellPattern(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	testCases := []struct {
		input    string
		expected bool
	}{
		{"safe string", false},
		{"sh command", true},
		{"bash script", true},
		{"cmd.exe", true},
		{"exec function", true},
		{"system call", true},
		{"powershell", true},
		{"/bin/bash", true},
		{"eval code", true},
		{"cmd1; cmd2", true},
		{"cmd1 | cmd2", true},
		{"`command`", true},
		{"BASH", true}, // Case insensitive
		{"SH", true},   // Case insensitive
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := validator.containsShellPattern(tc.input)
			if result != tc.expected {
				t.Errorf("input %q: expected %v, got %v", tc.input, tc.expected, result)
			}
		})
	}
}

func TestInputValidator_RecursiveValidation(t *testing.T) {
	logger := zap.NewNop()
	validator := NewInputValidator(logger)

	// Deeply nested structure with shell command
	params := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": []interface{}{
					"safe",
					map[string]interface{}{
						"dangerous": "bash script.sh",
					},
				},
			},
		},
	}

	err := validator.Validate("test_tool", params)
	if err == nil {
		t.Error("expected error for deeply nested shell command, got nil")
	}

	if !strings.Contains(err.Error(), "shell command") {
		t.Errorf("expected shell command error, got %v", err)
	}
}
