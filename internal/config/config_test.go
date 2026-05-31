package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_AllDefaults(t *testing.T) {
	// Set only required env var
	os.Setenv("ETH_RPC_URL", "http://localhost:8545")
	defer os.Unsetenv("ETH_RPC_URL")

	// Clear optional env vars
	os.Unsetenv("WORKER_COUNT")
	os.Unsetenv("METRICS_PORT")
	os.Unsetenv("MCP_PORT")
	os.Unsetenv("LOAD_TEST_ENABLED")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "http://localhost:8545", cfg.ETHRPCURL)
	assert.Equal(t, 4, cfg.WorkerCount)
	assert.Equal(t, 9090, cfg.MetricsPort)
	assert.Equal(t, 8080, cfg.MCPPort)
	assert.False(t, cfg.LoadTestEnabled)
}

func TestLoad_AllCustom(t *testing.T) {
	// Set all env vars
	os.Setenv("ETH_RPC_URL", "https://mainnet.infura.io/v3/test")
	os.Setenv("WORKER_COUNT", "8")
	os.Setenv("METRICS_PORT", "9091")
	os.Setenv("MCP_PORT", "8081")
	os.Setenv("LOAD_TEST_ENABLED", "true")

	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("WORKER_COUNT")
		os.Unsetenv("METRICS_PORT")
		os.Unsetenv("MCP_PORT")
		os.Unsetenv("LOAD_TEST_ENABLED")
	}()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "https://mainnet.infura.io/v3/test", cfg.ETHRPCURL)
	assert.Equal(t, 8, cfg.WorkerCount)
	assert.Equal(t, 9091, cfg.MetricsPort)
	assert.Equal(t, 8081, cfg.MCPPort)
	assert.True(t, cfg.LoadTestEnabled)
}

func TestLoad_MissingRequired(t *testing.T) {
	// Clear ETH_RPC_URL
	os.Unsetenv("ETH_RPC_URL")

	cfg, err := Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "ETH_RPC_URL environment variable is required")
}

func TestLoad_InvalidWorkerCount(t *testing.T) {
	os.Setenv("ETH_RPC_URL", "http://localhost:8545")
	os.Setenv("WORKER_COUNT", "invalid")

	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("WORKER_COUNT")
	}()

	cfg, err := Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "WORKER_COUNT must be a valid integer")
}

func TestLoad_InvalidMetricsPort(t *testing.T) {
	os.Setenv("ETH_RPC_URL", "http://localhost:8545")
	os.Setenv("METRICS_PORT", "invalid")

	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("METRICS_PORT")
	}()

	cfg, err := Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "METRICS_PORT must be a valid integer")
}

func TestLoad_InvalidMCPPort(t *testing.T) {
	os.Setenv("ETH_RPC_URL", "http://localhost:8545")
	os.Setenv("MCP_PORT", "invalid")

	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("MCP_PORT")
	}()

	cfg, err := Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "MCP_PORT must be a valid integer")
}

func TestLoad_InvalidLoadTestEnabled(t *testing.T) {
	os.Setenv("ETH_RPC_URL", "http://localhost:8545")
	os.Setenv("LOAD_TEST_ENABLED", "invalid")

	defer func() {
		os.Unsetenv("ETH_RPC_URL")
		os.Unsetenv("LOAD_TEST_ENABLED")
	}()

	cfg, err := Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "LOAD_TEST_ENABLED must be a valid boolean")
}

func TestValidate_WorkerCountTooLow(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     0,
		MetricsPort:     9090,
		MCPPort:         8080,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_COUNT must be between 1 and 256")
}

func TestValidate_WorkerCountTooHigh(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     257,
		MetricsPort:     9090,
		MCPPort:         8080,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_COUNT must be between 1 and 256")
}

func TestValidate_MetricsPortTooLow(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     4,
		MetricsPort:     0,
		MCPPort:         8080,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "METRICS_PORT must be between 1 and 65535")
}

func TestValidate_MetricsPortTooHigh(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     4,
		MetricsPort:     65536,
		MCPPort:         8080,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "METRICS_PORT must be between 1 and 65535")
}

func TestValidate_MCPPortTooLow(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     4,
		MetricsPort:     9090,
		MCPPort:         0,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MCP_PORT must be between 1 and 65535")
}

func TestValidate_MCPPortTooHigh(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     4,
		MetricsPort:     9090,
		MCPPort:         65536,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MCP_PORT must be between 1 and 65535")
}

func TestValidate_ValidConfiguration(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     4,
		MetricsPort:     9090,
		MCPPort:         8080,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_EdgeCases(t *testing.T) {
	// Test minimum valid values
	cfg := &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     1,
		MetricsPort:     1,
		MCPPort:         1,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.NoError(t, err)

	// Test maximum valid values
	cfg = &Config{
		ETHRPCURL:       "http://localhost:8545",
		WorkerCount:     256,
		MetricsPort:     65535,
		MCPPort:         65535,
		LoadTestEnabled: true,
	}

	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestLoad_BooleanVariations(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true lowercase", "true", true},
		{"True capitalized", "True", true},
		{"TRUE uppercase", "TRUE", true},
		{"1", "1", true},
		{"false lowercase", "false", false},
		{"False capitalized", "False", false},
		{"FALSE uppercase", "FALSE", false},
		{"0", "0", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("ETH_RPC_URL", "http://localhost:8545")
			os.Setenv("LOAD_TEST_ENABLED", tc.value)

			defer func() {
				os.Unsetenv("ETH_RPC_URL")
				os.Unsetenv("LOAD_TEST_ENABLED")
			}()

			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, tc.expected, cfg.LoadTestEnabled)
		})
	}
}

func TestContainsHardcodedSecret(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "localhost URL - no secret",
			input:    "ws://localhost:8545",
			expected: false,
		},
		{
			name:     "short path segment - no secret",
			input:    "https://mainnet.infura.io/v3/test",
			expected: false,
		},
		{
			name:     "infura with real API key (32 hex chars)",
			input:    "https://mainnet.infura.io/v3/abcdef1234567890abcdef1234567890",
			expected: true,
		},
		{
			name:     "alchemy with real API key",
			input:    "wss://eth-mainnet.g.alchemy.com/v2/aAbBcCdDeEfF1234567890aAbBcCdDeE",
			expected: true,
		},
		{
			name:     "URL with non-hex path segment",
			input:    "https://example.com/some-long-path-that-is-not-hex-chars",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsHardcodedSecret(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidate_HardcodedSecret(t *testing.T) {
	cfg := &Config{
		ETHRPCURL:       "https://mainnet.infura.io/v3/abcdef1234567890abcdef1234567890",
		WorkerCount:     4,
		MetricsPort:     9090,
		MCPPort:         8080,
		LoadTestEnabled: false,
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hardcoded secret")
}
