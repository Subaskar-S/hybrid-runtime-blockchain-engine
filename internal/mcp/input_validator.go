package mcp

import (
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// InputValidator validates tool input parameters
type InputValidator struct {
	logger *zap.Logger
	// Patterns that indicate shell command execution attempts
	shellPatterns []*regexp.Regexp
}

// NewInputValidator creates a new input validator
func NewInputValidator(logger *zap.Logger) *InputValidator {
	return &InputValidator{
		logger: logger,
		shellPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\bsh\b`),
			regexp.MustCompile(`(?i)\bbash\b`),
			regexp.MustCompile(`(?i)\bcmd\b`),
			regexp.MustCompile(`(?i)\bexec\b`),
			regexp.MustCompile(`(?i)\bsystem\b`),
			regexp.MustCompile(`(?i)\bpowershell\b`),
			regexp.MustCompile(`(?i)\b/bin/`),
			regexp.MustCompile(`(?i)\beval\b`),
			regexp.MustCompile(`[;&|]`), // Shell operators
			regexp.MustCompile("`"),     // Backticks
		},
	}
}

// Validate validates tool parameters
func (v *InputValidator) Validate(tool string, params map[string]interface{}) error {
	// Check for shell command execution attempts in all string parameters
	for key, value := range params {
		if err := v.validateValue(key, value); err != nil {
			return err
		}
	}

	// Tool-specific validation
	switch tool {
	case "run_load_test":
		return v.validateLoadTest(params)
	case "get_reorg_history":
		return v.validateReorgHistory(params)
	case "get_apply_block_latency":
		return v.validateApplyBlockLatency(params)
	case "validate_determinism":
		return v.validateDeterminism(params)
	}

	return nil
}

// validateValue recursively validates a parameter value
func (v *InputValidator) validateValue(key string, value interface{}) error {
	switch val := value.(type) {
	case string:
		// Check for shell command patterns
		if v.containsShellPattern(val) {
			v.logger.Warn("shell command pattern detected",
				zap.String("key", key),
				zap.String("value", val))
			return fmt.Errorf("shell command execution not allowed")
		}
	case map[string]interface{}:
		// Recursively validate nested maps
		for k, nestedVal := range val {
			if err := v.validateValue(k, nestedVal); err != nil {
				return err
			}
		}
	case []interface{}:
		// Recursively validate arrays
		for i, item := range val {
			if err := v.validateValue(fmt.Sprintf("%s[%d]", key, i), item); err != nil {
				return err
			}
		}
	}

	return nil
}

// containsShellPattern checks if a string contains shell command patterns
func (v *InputValidator) containsShellPattern(s string) bool {
	for _, pattern := range v.shellPatterns {
		if pattern.MatchString(s) {
			return true
		}
	}
	return false
}

// validateLoadTest validates run_load_test parameters
func (v *InputValidator) validateLoadTest(params map[string]interface{}) error {
	tps, ok := params["tps"]
	if !ok {
		return fmt.Errorf("missing required parameter: tps")
	}

	tpsFloat, ok := tps.(float64)
	if !ok {
		return fmt.Errorf("tps must be a number")
	}

	if tpsFloat < 1 || tpsFloat > 10000 {
		return fmt.Errorf("tps must be between 1 and 10000")
	}

	duration, ok := params["duration_seconds"]
	if !ok {
		return fmt.Errorf("missing required parameter: duration_seconds")
	}

	durationFloat, ok := duration.(float64)
	if !ok {
		return fmt.Errorf("duration_seconds must be a number")
	}

	if durationFloat < 1 || durationFloat > 300 {
		return fmt.Errorf("duration_seconds must be between 1 and 300")
	}

	return nil
}

// validateReorgHistory validates get_reorg_history parameters
func (v *InputValidator) validateReorgHistory(params map[string]interface{}) error {
	if limit, ok := params["limit"]; ok {
		limitFloat, ok := limit.(float64)
		if !ok {
			return fmt.Errorf("limit must be a number")
		}
		if limitFloat < 1 || limitFloat > 1000 {
			return fmt.Errorf("limit must be between 1 and 1000")
		}
	}
	return nil
}

// validateApplyBlockLatency validates get_apply_block_latency parameters
func (v *InputValidator) validateApplyBlockLatency(params map[string]interface{}) error {
	if limit, ok := params["limit"]; ok {
		limitFloat, ok := limit.(float64)
		if !ok {
			return fmt.Errorf("limit must be a number")
		}
		if limitFloat < 1 || limitFloat > 10000 {
			return fmt.Errorf("limit must be between 1 and 10000")
		}
	}
	return nil
}

// validateDeterminism validates validate_determinism parameters
func (v *InputValidator) validateDeterminism(params map[string]interface{}) error {
	if blockCount, ok := params["block_count"]; ok {
		blockCountFloat, ok := blockCount.(float64)
		if !ok {
			return fmt.Errorf("block_count must be a number")
		}
		if blockCountFloat < 1 || blockCountFloat > 100 {
			return fmt.Errorf("block_count must be between 1 and 100")
		}
	}
	return nil
}

// ValidateType validates that a parameter has the expected type
func (v *InputValidator) ValidateType(value interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("expected number, got %T", value)
		}
	case "integer":
		if f, ok := value.(float64); !ok || f != float64(int(f)) {
			return fmt.Errorf("expected integer, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	default:
		return fmt.Errorf("unknown type: %s", expectedType)
	}
	return nil
}

// ValidateRange validates that a numeric value is within a range
func (v *InputValidator) ValidateRange(value interface{}, min, max float64) error {
	f, ok := value.(float64)
	if !ok {
		return fmt.Errorf("expected number, got %T", value)
	}
	if f < min || f > max {
		return fmt.Errorf("value %f out of range [%f, %f]", f, min, max)
	}
	return nil
}

// SanitizeString removes potentially dangerous characters from strings
func (v *InputValidator) SanitizeString(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")
	// Remove control characters except newline and tab
	var result strings.Builder
	for _, r := range s {
		if r >= 32 || r == '\n' || r == '\t' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
