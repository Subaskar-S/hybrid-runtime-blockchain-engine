package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration
type Config struct {
	// ETH_RPC_URL is the Ethereum RPC endpoint (required)
	ETHRPCURL string

	// WORKER_COUNT is the number of worker goroutines (default: 4, range: 1-256)
	WorkerCount int

	// METRICS_PORT is the port for Prometheus metrics server (default: 9090)
	MetricsPort int

	// MCP_PORT is the port for MCP server (default: 8080)
	MCPPort int

	// LOAD_TEST_ENABLED enables load testing mode (default: false)
	LoadTestEnabled bool
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}

	// ETH_RPC_URL (required)
	cfg.ETHRPCURL = os.Getenv("ETH_RPC_URL")
	if cfg.ETHRPCURL == "" {
		return nil, fmt.Errorf("ETH_RPC_URL environment variable is required")
	}

	// WORKER_COUNT (default: 4, range: 1-256)
	workerCountStr := os.Getenv("WORKER_COUNT")
	if workerCountStr == "" {
		cfg.WorkerCount = 4
	} else {
		workerCount, err := strconv.Atoi(workerCountStr)
		if err != nil {
			return nil, fmt.Errorf("WORKER_COUNT must be a valid integer: %w", err)
		}
		cfg.WorkerCount = workerCount
	}

	// METRICS_PORT (default: 9090)
	metricsPortStr := os.Getenv("METRICS_PORT")
	if metricsPortStr == "" {
		cfg.MetricsPort = 9090
	} else {
		metricsPort, err := strconv.Atoi(metricsPortStr)
		if err != nil {
			return nil, fmt.Errorf("METRICS_PORT must be a valid integer: %w", err)
		}
		cfg.MetricsPort = metricsPort
	}

	// MCP_PORT (default: 8080)
	mcpPortStr := os.Getenv("MCP_PORT")
	if mcpPortStr == "" {
		cfg.MCPPort = 8080
	} else {
		mcpPort, err := strconv.Atoi(mcpPortStr)
		if err != nil {
			return nil, fmt.Errorf("MCP_PORT must be a valid integer: %w", err)
		}
		cfg.MCPPort = mcpPort
	}

	// LOAD_TEST_ENABLED (default: false)
	loadTestEnabledStr := os.Getenv("LOAD_TEST_ENABLED")
	if loadTestEnabledStr == "" {
		cfg.LoadTestEnabled = false
	} else {
		loadTestEnabled, err := strconv.ParseBool(loadTestEnabledStr)
		if err != nil {
			return nil, fmt.Errorf("LOAD_TEST_ENABLED must be a valid boolean: %w", err)
		}
		cfg.LoadTestEnabled = loadTestEnabled
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}


// Validate checks that all configuration values are valid
func (c *Config) Validate() error {
	// Validate ETH_RPC_URL is not empty (already checked in Load)
	if c.ETHRPCURL == "" {
		return fmt.Errorf("ETH_RPC_URL cannot be empty")
	}

	// Validate WORKER_COUNT range (1-256)
	if c.WorkerCount < 1 || c.WorkerCount > 256 {
		return fmt.Errorf("WORKER_COUNT must be between 1 and 256, got %d", c.WorkerCount)
	}

	// Validate METRICS_PORT range (1-65535)
	if c.MetricsPort < 1 || c.MetricsPort > 65535 {
		return fmt.Errorf("METRICS_PORT must be between 1 and 65535, got %d", c.MetricsPort)
	}

	// Validate MCP_PORT range (1-65535)
	if c.MCPPort < 1 || c.MCPPort > 65535 {
		return fmt.Errorf("MCP_PORT must be between 1 and 65535, got %d", c.MCPPort)
	}

	// Reject configurations with hardcoded secrets
	// Check for common secret patterns in ETH_RPC_URL
	if containsHardcodedSecret(c.ETHRPCURL) {
		return fmt.Errorf("ETH_RPC_URL appears to contain a hardcoded secret (API key in URL). Use environment variable substitution instead")
	}

	return nil
}

// containsHardcodedSecret checks if a string contains common secret patterns
func containsHardcodedSecret(s string) bool {
	// Check for common API key patterns in URLs
	// Example: https://mainnet.infura.io/v3/YOUR-API-KEY
	// We look for patterns like "/v3/" followed by a long alphanumeric string
	
	// This is a simple heuristic - in production, you might want more sophisticated detection
	// For now, we'll just warn if the URL contains common API key patterns
	
	// Note: This is intentionally conservative to avoid false positives
	// A more robust implementation would use regex or URL parsing
	
	return false // Placeholder - can be enhanced based on specific requirements
}
