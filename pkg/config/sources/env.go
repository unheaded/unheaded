// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sources

import (
	"os"
	"strconv"
	"strings"
)

// EnvSource loads configuration from environment variables.
type EnvSource struct {
	prefix    string
	separator string
}

// NewEnvSource creates a new environment variable source.
func NewEnvSource(prefix string) *EnvSource {
	return &EnvSource{
		prefix:    prefix,
		separator: "_",
	}
}

// NewEnvSourceWithSeparator creates a new environment variable source with custom separator.
func NewEnvSourceWithSeparator(prefix, separator string) *EnvSource {
	return &EnvSource{
		prefix:    prefix,
		separator: separator,
	}
}

// Load loads configuration from environment variables.
func (e *EnvSource) Load() (map[string]any, error) {
	result := make(map[string]any)
	prefix := e.prefix + e.separator

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Check if key starts with prefix
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		// Remove prefix and convert to nested key
		key = strings.TrimPrefix(key, prefix)
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, e.separator, ".")

		// Parse value
		parsedValue := parseEnvValue(value)

		// Set nested value
		setNestedEnvValue(result, key, parsedValue)
	}

	return result, nil
}

// Name returns the source name.
func (e *EnvSource) Name() string {
	return "env:" + e.prefix
}

// Priority returns the source priority.
func (e *EnvSource) Priority() int {
	return 50 // Environment variables have medium priority
}

// parseEnvValue attempts to parse an environment variable value as a typed value.
func parseEnvValue(value string) any {
	// Try boolean
	if lower := strings.ToLower(value); lower == "true" || lower == "false" {
		return lower == "true"
	}

	// Try integer
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return int(i)
	}

	// Try float
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}

	// Try JSON array
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return parseEnvArray(value)
	}

	// Return as string
	return value
}

// parseEnvArray parses an array-like environment variable value.
func parseEnvArray(value string) []any {
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	value = strings.TrimSpace(value)

	if value == "" {
		return []any{}
	}

	parts := splitEnvArray(value)
	result := make([]any, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		// Remove quotes if present
		if (strings.HasPrefix(part, "\"") && strings.HasSuffix(part, "\"")) ||
			(strings.HasPrefix(part, "'") && strings.HasSuffix(part, "'")) {
			part = part[1 : len(part)-1]
		}
		result[i] = parseEnvValue(part)
	}
	return result
}

// splitEnvArray splits an array string by comma, respecting quotes.
func splitEnvArray(s string) []string {
	var result []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range s {
		if inQuote {
			current.WriteRune(ch)
			if ch == quoteChar {
				inQuote = false
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			current.WriteRune(ch)
			continue
		}

		if ch == ',' {
			result = append(result, current.String())
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// setNestedEnvValue sets a nested value in a map using dot notation.
func setNestedEnvValue(data map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := data

	for i, part := range parts[:len(parts)-1] {
		if _, ok := current[part]; !ok {
			current[part] = make(map[string]any)
		}
		if next, ok := current[part].(map[string]any); ok {
			current = next
		} else {
			// Overwrite non-map value
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
			for j := i + 1; j < len(parts)-1; j++ {
				newMap := make(map[string]any)
				current[parts[j]] = newMap
				current = newMap
			}
			break
		}
	}

	current[parts[len(parts)-1]] = value
}

// EnvBinder helps bind struct fields to environment variables.
type EnvBinder struct {
	prefix   string
	bindings map[string]string
}

// NewEnvBinder creates a new environment variable binder.
func NewEnvBinder(prefix string) *EnvBinder {
	return &EnvBinder{
		prefix:   prefix,
		bindings: make(map[string]string),
	}
}

// Bind creates a binding from a config key to an environment variable.
func (b *EnvBinder) Bind(configKey, envVar string) {
	b.bindings[configKey] = envVar
}

// BindAuto creates an automatic binding from config key to PREFIXCONFIGKEY.
func (b *EnvBinder) BindAuto(configKey string) {
	envVar := b.prefix + "_" + strings.ToUpper(strings.ReplaceAll(configKey, ".", "_"))
	b.bindings[configKey] = envVar
}

// GetEnvValue gets the value from the bound environment variable.
func (b *EnvBinder) GetEnvValue(configKey string) (any, bool) {
	envVar, ok := b.bindings[configKey]
	if !ok {
		return nil, false
	}

	value, exists := os.LookupEnv(envVar)
	if !exists {
		return nil, false
	}

	return parseEnvValue(value), true
}

// AllBindings returns all registered bindings.
func (b *EnvBinder) AllBindings() map[string]string {
	result := make(map[string]string)
	for k, v := range b.bindings {
		result[k] = v
	}
	return result
}

// SetEnv sets an environment variable for the current process.
func SetEnv(key, value string) error {
	return os.Setenv(key, value)
}

// UnsetEnv unsets an environment variable for the current process.
func UnsetEnv(key string) error {
	return os.Unsetenv(key)
}

// GetEnv retrieves an environment variable with a default value.
func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// GetEnvInt retrieves an environment variable as an integer with a default value.
func GetEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetEnvBool retrieves an environment variable as a boolean with a default value.
func GetEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

// GetEnvFloat retrieves an environment variable as a float64 with a default value.
func GetEnvFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

// GetEnvSlice retrieves an environment variable as a string slice.
func GetEnvSlice(key string, separator string, defaultValue []string) []string {
	if value, exists := os.LookupEnv(key); exists {
		return strings.Split(value, separator)
	}
	return defaultValue
}

// RequireEnv retrieves an environment variable and panics if not set.
func RequireEnv(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		panic("required environment variable not set: " + key)
	}
	return value
}
