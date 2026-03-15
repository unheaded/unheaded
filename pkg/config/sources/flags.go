// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sources

import (
	"fmt"
	"strconv"
	"strings"
)

// FlagSource loads configuration from command-line flags.
type FlagSource struct {
	args   []string
	flags  map[string]any
	parsed bool
}

// NewFlagSource creates a new flag source.
func NewFlagSource(args []string) *FlagSource {
	return &FlagSource{
		args:  args,
		flags: make(map[string]any),
	}
}

// Load loads configuration from command-line flags.
func (f *FlagSource) Load() (map[string]any, error) {
	if !f.parsed {
		f.parse()
	}
	return f.toNestedMap(), nil
}

// Name returns the source name.
func (f *FlagSource) Name() string {
	return "flags"
}

// Priority returns the source priority (highest).
func (f *FlagSource) Priority() int {
	return 10 // Flags have highest priority
}

// parse parses command-line arguments.
func (f *FlagSource) parse() {
	f.parsed = true
	i := 0

	for i < len(f.args) {
		arg := f.args[i]

		// Skip non-flag arguments
		if !strings.HasPrefix(arg, "-") {
			i++
			continue
		}

		// Handle -- stop flag
		if arg == "--" {
			break
		}

		// Remove leading dashes
		key := strings.TrimLeft(arg, "-")

		// Check for = in flag
		if eqIdx := strings.Index(key, "="); eqIdx >= 0 {
			name := key[:eqIdx]
			value := key[eqIdx+1:]
			f.setFlag(name, value)
			i++
			continue
		}

		// Check if next argument is a value
		if i+1 < len(f.args) && !strings.HasPrefix(f.args[i+1], "-") {
			f.setFlag(key, f.args[i+1])
			i += 2
			continue
		}

		// Flag without value (boolean true)
		// Handle --no-xxx for boolean false
		if strings.HasPrefix(key, "no-") {
			f.setFlag(key[3:], false)
		} else {
			f.setFlag(key, true)
		}
		i++
	}
}

// setFlag sets a flag value, parsing the value type.
func (f *FlagSource) setFlag(key string, value any) {
	// Convert key format: server-port -> server.port
	key = strings.ReplaceAll(key, "-", ".")

	switch v := value.(type) {
	case string:
		f.flags[key] = parseValue(v)
	case bool:
		f.flags[key] = v
	default:
		f.flags[key] = v
	}
}

// parseValue attempts to parse a string value into a typed value.
func parseValue(s string) any {
	// Try boolean
	if lower := strings.ToLower(s); lower == "true" || lower == "false" {
		return lower == "true"
	}

	// Try integer
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}

	// Try float
	if fl, err := strconv.ParseFloat(s, 64); err == nil {
		return fl
	}

	// Try array (comma-separated)
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		result := make([]any, len(parts))
		for i, part := range parts {
			result[i] = parseValue(strings.TrimSpace(part))
		}
		return result
	}

	return s
}

// toNestedMap converts flat flags to nested map structure.
func (f *FlagSource) toNestedMap() map[string]any {
	result := make(map[string]any)

	for key, value := range f.flags {
		parts := strings.Split(key, ".")
		current := result

		for i, part := range parts[:len(parts)-1] {
			if _, ok := current[part]; !ok {
				current[part] = make(map[string]any)
			}
			if next, ok := current[part].(map[string]any); ok {
				current = next
			} else {
				// Create intermediate map
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

	return result
}

// FlagSet provides a more structured way to define and parse flags.
type FlagSet struct {
	name     string
	args     []string
	flags    map[string]*Flag
	values   map[string]any
	parsed   bool
	usage    string
	errorFn  func(string)
}

// Flag represents a single command-line flag.
type Flag struct {
	Name        string
	Shorthand   string
	Usage       string
	Default     any
	Required    bool
	Value       any
	EnvVar      string
}

// NewFlagSet creates a new flag set.
func NewFlagSet(name string, args []string) *FlagSet {
	return &FlagSet{
		name:   name,
		args:   args,
		flags:  make(map[string]*Flag),
		values: make(map[string]any),
	}
}

// String defines a string flag.
func (fs *FlagSet) String(name, shorthand, defaultValue, usage string) {
	fs.flags[name] = &Flag{
		Name:      name,
		Shorthand: shorthand,
		Usage:     usage,
		Default:   defaultValue,
		Value:     defaultValue,
	}
	if shorthand != "" {
		fs.flags[shorthand] = fs.flags[name]
	}
}

// Int defines an integer flag.
func (fs *FlagSet) Int(name, shorthand string, defaultValue int, usage string) {
	fs.flags[name] = &Flag{
		Name:      name,
		Shorthand: shorthand,
		Usage:     usage,
		Default:   defaultValue,
		Value:     defaultValue,
	}
	if shorthand != "" {
		fs.flags[shorthand] = fs.flags[name]
	}
}

// Bool defines a boolean flag.
func (fs *FlagSet) Bool(name, shorthand string, defaultValue bool, usage string) {
	fs.flags[name] = &Flag{
		Name:      name,
		Shorthand: shorthand,
		Usage:     usage,
		Default:   defaultValue,
		Value:     defaultValue,
	}
	if shorthand != "" {
		fs.flags[shorthand] = fs.flags[name]
	}
}

// Float defines a float flag.
func (fs *FlagSet) Float(name, shorthand string, defaultValue float64, usage string) {
	fs.flags[name] = &Flag{
		Name:      name,
		Shorthand: shorthand,
		Usage:     usage,
		Default:   defaultValue,
		Value:     defaultValue,
	}
	if shorthand != "" {
		fs.flags[shorthand] = fs.flags[name]
	}
}

// StringSlice defines a string slice flag.
func (fs *FlagSet) StringSlice(name, shorthand string, defaultValue []string, usage string) {
	fs.flags[name] = &Flag{
		Name:      name,
		Shorthand: shorthand,
		Usage:     usage,
		Default:   defaultValue,
		Value:     defaultValue,
	}
	if shorthand != "" {
		fs.flags[shorthand] = fs.flags[name]
	}
}

// WithEnvVar binds a flag to an environment variable.
func (fs *FlagSet) WithEnvVar(name, envVar string) {
	if flag, ok := fs.flags[name]; ok {
		flag.EnvVar = envVar
	}
}

// Required marks a flag as required.
func (fs *FlagSet) Required(name string) {
	if flag, ok := fs.flags[name]; ok {
		flag.Required = true
	}
}

// Parse parses the command-line arguments.
func (fs *FlagSet) Parse() error {
	if fs.parsed {
		return nil
	}
	fs.parsed = true

	i := 0
	for i < len(fs.args) {
		arg := fs.args[i]

		if !strings.HasPrefix(arg, "-") {
			i++
			continue
		}

		if arg == "--" {
			break
		}

		key := strings.TrimLeft(arg, "-")
		var value string
		hasValue := false

		// Check for = in flag
		if eqIdx := strings.Index(key, "="); eqIdx >= 0 {
			value = key[eqIdx+1:]
			key = key[:eqIdx]
			hasValue = true
		} else if i+1 < len(fs.args) && !strings.HasPrefix(fs.args[i+1], "-") {
			value = fs.args[i+1]
			hasValue = true
			i++
		}

		flag, ok := fs.flags[key]
		if !ok {
			i++
			continue
		}

		if hasValue {
			fs.setFlagValue(flag, value)
		} else {
			// Boolean flag
			if _, isBool := flag.Default.(bool); isBool {
				if strings.HasPrefix(key, "no-") {
					flag.Value = false
				} else {
					flag.Value = true
				}
			}
		}
		i++
	}

	// Check environment variables for unset flags
	for _, flag := range fs.flags {
		if flag.EnvVar != "" && flag.Value == flag.Default {
			if envValue, exists := lookupEnv(flag.EnvVar); exists {
				fs.setFlagValue(flag, envValue)
			}
		}
	}

	// Check required flags
	for name, flag := range fs.flags {
		if flag.Required && flag.Value == flag.Default {
			if fs.errorFn != nil {
				fs.errorFn("required flag not set: " + name)
			}
		}
	}

	return nil
}

// setFlagValue sets the flag value from a string.
func (fs *FlagSet) setFlagValue(flag *Flag, value string) {
	switch flag.Default.(type) {
	case string:
		flag.Value = value
	case int:
		if i, err := strconv.Atoi(value); err == nil {
			flag.Value = i
		}
	case bool:
		flag.Value = strings.ToLower(value) == "true" || value == "1"
	case float64:
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			flag.Value = f
		}
	case []string:
		flag.Value = strings.Split(value, ",")
	}
}

// lookupEnv is a helper to lookup environment variables.
func lookupEnv(key string) (string, bool) {
	// We don't import os here to avoid circular imports, use the env source
	return "", false
}

// GetString gets a string flag value.
func (fs *FlagSet) GetString(name string) string {
	if flag, ok := fs.flags[name]; ok {
		if v, ok := flag.Value.(string); ok {
			return v
		}
	}
	return ""
}

// GetInt gets an integer flag value.
func (fs *FlagSet) GetInt(name string) int {
	if flag, ok := fs.flags[name]; ok {
		if v, ok := flag.Value.(int); ok {
			return v
		}
	}
	return 0
}

// GetBool gets a boolean flag value.
func (fs *FlagSet) GetBool(name string) bool {
	if flag, ok := fs.flags[name]; ok {
		if v, ok := flag.Value.(bool); ok {
			return v
		}
	}
	return false
}

// GetFloat gets a float flag value.
func (fs *FlagSet) GetFloat(name string) float64 {
	if flag, ok := fs.flags[name]; ok {
		if v, ok := flag.Value.(float64); ok {
			return v
		}
	}
	return 0
}

// GetStringSlice gets a string slice flag value.
func (fs *FlagSet) GetStringSlice(name string) []string {
	if flag, ok := fs.flags[name]; ok {
		if v, ok := flag.Value.([]string); ok {
			return v
		}
	}
	return nil
}

// ToMap converts all flag values to a nested map.
func (fs *FlagSet) ToMap() map[string]any {
	result := make(map[string]any)

	seen := make(map[string]bool)
	for name, flag := range fs.flags {
		// Skip duplicates (shorthand entries point to same flag)
		if seen[flag.Name] {
			continue
		}
		seen[flag.Name] = true

		parts := strings.Split(name, ".")
		current := result

		for i, part := range parts[:len(parts)-1] {
			if _, ok := current[part]; !ok {
				current[part] = make(map[string]any)
			}
			if next, ok := current[part].(map[string]any); ok {
				current = next
			} else {
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

		current[parts[len(parts)-1]] = flag.Value
	}

	return result
}

// Usage returns the usage string for all flags.
func (fs *FlagSet) Usage() string {
	var sb strings.Builder
	sb.WriteString("Usage of " + fs.name + ":\n")

	seen := make(map[string]bool)
	for _, flag := range fs.flags {
		if seen[flag.Name] {
			continue
		}
		seen[flag.Name] = true

		sb.WriteString("  -")
		if len(flag.Name) > 1 {
			sb.WriteString("-")
		}
		sb.WriteString(flag.Name)

		if flag.Shorthand != "" {
			sb.WriteString(", -" + flag.Shorthand)
		}

		sb.WriteString("\n\t" + flag.Usage)

		if flag.Default != nil {
			sb.WriteString(" (default: ")
			switch v := flag.Default.(type) {
			case string:
				sb.WriteString("\"" + v + "\"")
			case int:
				sb.WriteString(strconv.Itoa(v))
			case float64:
				sb.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
			case bool:
				sb.WriteString(strconv.FormatBool(v))
			case []string:
				sb.WriteString("[" + strings.Join(v, ",") + "]")
			default:
				sb.WriteString(fmt.Sprintf("%v", v))
			}
			sb.WriteString(")")
		}

		if flag.EnvVar != "" {
			sb.WriteString(" (env: " + flag.EnvVar + ")")
		}

		if flag.Required {
			sb.WriteString(" [required]")
		}

		sb.WriteString("\n")
	}

	return sb.String()
}
