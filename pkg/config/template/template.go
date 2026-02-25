// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package template provides configuration template expansion.
package template

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Expander expands template variables in configuration.
type Expander struct {
	funcs    map[string]TemplateFunc
	vars     map[string]string
	resolver VariableResolver
}

// TemplateFunc is a function that can be called in templates.
type TemplateFunc func(args ...string) (string, error)

// VariableResolver resolves variable values.
type VariableResolver interface {
	Resolve(name string) (string, bool)
}

// EnvResolver resolves variables from environment.
type EnvResolver struct{}

// Resolve resolves a variable from environment.
func (r *EnvResolver) Resolve(name string) (string, bool) {
	return os.LookupEnv(name)
}

// MapResolver resolves variables from a map.
type MapResolver struct {
	vars map[string]string
}

// NewMapResolver creates a new map resolver.
func NewMapResolver(vars map[string]string) *MapResolver {
	return &MapResolver{vars: vars}
}

// Resolve resolves a variable from the map.
func (r *MapResolver) Resolve(name string) (string, bool) {
	v, ok := r.vars[name]
	return v, ok
}

// ChainResolver chains multiple resolvers.
type ChainResolver struct {
	resolvers []VariableResolver
}

// NewChainResolver creates a new chain resolver.
func NewChainResolver(resolvers ...VariableResolver) *ChainResolver {
	return &ChainResolver{resolvers: resolvers}
}

// Resolve resolves a variable by trying each resolver in order.
func (r *ChainResolver) Resolve(name string) (string, bool) {
	for _, resolver := range r.resolvers {
		if v, ok := resolver.Resolve(name); ok {
			return v, true
		}
	}
	return "", false
}

// NewExpander creates a new template expander.
func NewExpander() *Expander {
	e := &Expander{
		funcs:    make(map[string]TemplateFunc),
		vars:     make(map[string]string),
		resolver: &EnvResolver{},
	}

	// Register default functions
	e.registerDefaultFuncs()

	return e
}

// NewExpanderWithResolver creates a new expander with a custom resolver.
func NewExpanderWithResolver(resolver VariableResolver) *Expander {
	e := &Expander{
		funcs:    make(map[string]TemplateFunc),
		vars:     make(map[string]string),
		resolver: resolver,
	}
	e.registerDefaultFuncs()
	return e
}

// registerDefaultFuncs registers default template functions.
func (e *Expander) registerDefaultFuncs() {
	e.funcs["env"] = e.funcEnv
	e.funcs["default"] = e.funcDefault
	e.funcs["required"] = e.funcRequired
	e.funcs["upper"] = e.funcUpper
	e.funcs["lower"] = e.funcLower
	e.funcs["trim"] = e.funcTrim
	e.funcs["replace"] = e.funcReplace
	e.funcs["split"] = e.funcSplit
	e.funcs["join"] = e.funcJoin
	e.funcs["substr"] = e.funcSubstr
	e.funcs["base64"] = e.funcBase64
	e.funcs["quote"] = e.funcQuote
}

// RegisterFunc registers a custom template function.
func (e *Expander) RegisterFunc(name string, fn TemplateFunc) {
	e.funcs[name] = fn
}

// SetVar sets a template variable.
func (e *Expander) SetVar(name, value string) {
	e.vars[name] = value
}

// SetVars sets multiple template variables.
func (e *Expander) SetVars(vars map[string]string) {
	for k, v := range vars {
		e.vars[k] = v
	}
}

// Expand expands template variables in a configuration map.
func (e *Expander) Expand(data map[string]any) (map[string]any, error) {
	return e.expandMap(data)
}

// expandMap expands a map recursively.
func (e *Expander) expandMap(data map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for k, v := range data {
		expandedKey, err := e.expandString(k)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", k, err)
		}

		expandedValue, err := e.expandValue(v)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", k, err)
		}

		result[expandedKey] = expandedValue
	}

	return result, nil
}

// expandValue expands a value recursively.
func (e *Expander) expandValue(v any) (any, error) {
	switch val := v.(type) {
	case string:
		return e.expandString(val)
	case map[string]any:
		return e.expandMap(val)
	case []any:
		return e.expandSlice(val)
	default:
		return v, nil
	}
}

// expandSlice expands a slice recursively.
func (e *Expander) expandSlice(data []any) ([]any, error) {
	result := make([]any, len(data))

	for i, v := range data {
		expanded, err := e.expandValue(v)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		result[i] = expanded
	}

	return result, nil
}

// expandString expands template variables in a string.
func (e *Expander) expandString(s string) (string, error) {
	// Pattern for ${VAR}, ${VAR:-default}, ${VAR:+alternate}
	pattern := regexp.MustCompile(`\$\{([^}]+)\}`)

	var lastErr error
	result := pattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable expression
		expr := match[2 : len(match)-1]

		expanded, err := e.evaluateExpression(expr)
		if err != nil {
			lastErr = err
			return match
		}
		return expanded
	})

	// Also handle $VAR syntax
	simplePattern := regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	result = simplePattern.ReplaceAllStringFunc(result, func(match string) string {
		name := match[1:]
		if value, ok := e.resolveVariable(name); ok {
			return value
		}
		return match
	})

	return result, lastErr
}

// evaluateExpression evaluates a template expression.
func (e *Expander) evaluateExpression(expr string) (string, error) {
	// Check for function call: func(args)
	if funcMatch := regexp.MustCompile(`^(\w+)\((.*)\)$`).FindStringSubmatch(expr); len(funcMatch) == 3 {
		funcName := funcMatch[1]
		args := parseArgs(funcMatch[2])
		return e.callFunc(funcName, args...)
	}

	// Check for default value: VAR:-default
	if idx := strings.Index(expr, ":-"); idx >= 0 {
		name := expr[:idx]
		defaultVal := expr[idx+2:]
		if value, ok := e.resolveVariable(name); ok && value != "" {
			return value, nil
		}
		return e.expandString(defaultVal)
	}

	// Check for alternate value: VAR:+alternate
	if idx := strings.Index(expr, ":+"); idx >= 0 {
		name := expr[:idx]
		alternateVal := expr[idx+2:]
		if value, ok := e.resolveVariable(name); ok && value != "" {
			return e.expandString(alternateVal)
		}
		return "", nil
	}

	// Check for error on unset: VAR:?message
	if idx := strings.Index(expr, ":?"); idx >= 0 {
		name := expr[:idx]
		message := expr[idx+2:]
		if value, ok := e.resolveVariable(name); ok && value != "" {
			return value, nil
		}
		return "", fmt.Errorf("variable %s: %s", name, message)
	}

	// Simple variable reference
	if value, ok := e.resolveVariable(expr); ok {
		return value, nil
	}

	return "", nil
}

// resolveVariable resolves a variable value.
func (e *Expander) resolveVariable(name string) (string, bool) {
	// First check local vars
	if v, ok := e.vars[name]; ok {
		return v, true
	}

	// Then use resolver
	return e.resolver.Resolve(name)
}

// callFunc calls a template function.
func (e *Expander) callFunc(name string, args ...string) (string, error) {
	fn, ok := e.funcs[name]
	if !ok {
		return "", fmt.Errorf("unknown function: %s", name)
	}

	// Expand arguments
	expandedArgs := make([]string, len(args))
	for i, arg := range args {
		expanded, err := e.expandString(arg)
		if err != nil {
			return "", err
		}
		expandedArgs[i] = expanded
	}

	return fn(expandedArgs...)
}

// parseArgs parses function arguments.
func parseArgs(s string) []string {
	if s == "" {
		return nil
	}

	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	depth := 0

	for _, ch := range s {
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			}
			current.WriteRune(ch)
			continue
		}

		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			current.WriteRune(ch)
			continue
		}

		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		}

		if ch == ',' && depth == 0 {
			args = append(args, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}

		current.WriteRune(ch)
	}

	if current.Len() > 0 {
		args = append(args, strings.TrimSpace(current.String()))
	}

	// Remove quotes from arguments
	for i, arg := range args {
		if (strings.HasPrefix(arg, "\"") && strings.HasSuffix(arg, "\"")) ||
			(strings.HasPrefix(arg, "'") && strings.HasSuffix(arg, "'")) {
			args[i] = arg[1 : len(arg)-1]
		}
	}

	return args
}

// Template functions

func (e *Expander) funcEnv(args ...string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("env: requires variable name")
	}
	name := args[0]
	value, _ := os.LookupEnv(name)
	if len(args) > 1 && value == "" {
		return args[1], nil // default value
	}
	return value, nil
}

func (e *Expander) funcDefault(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("default: requires value and default")
	}
	if args[0] != "" {
		return args[0], nil
	}
	return args[1], nil
}

func (e *Expander) funcRequired(args ...string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("required: requires variable name")
	}
	name := args[0]
	value, ok := e.resolveVariable(name)
	if !ok || value == "" {
		message := "is required"
		if len(args) > 1 {
			message = args[1]
		}
		return "", fmt.Errorf("%s %s", name, message)
	}
	return value, nil
}

func (e *Expander) funcUpper(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return strings.ToUpper(args[0]), nil
}

func (e *Expander) funcLower(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return strings.ToLower(args[0]), nil
}

func (e *Expander) funcTrim(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	cutset := " \t\n\r"
	if len(args) > 1 {
		cutset = args[1]
	}
	return strings.Trim(args[0], cutset), nil
}

func (e *Expander) funcReplace(args ...string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("replace: requires string, old, new")
	}
	return strings.ReplaceAll(args[0], args[1], args[2]), nil
}

func (e *Expander) funcSplit(args ...string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("split: requires string, separator, index")
	}
	parts := strings.Split(args[0], args[1])
	idx, err := strconv.Atoi(args[2])
	if err != nil {
		return "", fmt.Errorf("split: invalid index: %s", args[2])
	}
	if idx < 0 || idx >= len(parts) {
		return "", nil
	}
	return parts[idx], nil
}

func (e *Expander) funcJoin(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("join: requires separator and values")
	}
	return strings.Join(args[1:], args[0]), nil
}

func (e *Expander) funcSubstr(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("substr: requires string and start")
	}
	s := args[0]
	start, err := strconv.Atoi(args[1])
	if err != nil {
		return "", fmt.Errorf("substr: invalid start: %s", args[1])
	}
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return "", nil
	}
	if len(args) > 2 {
		length, err := strconv.Atoi(args[2])
		if err != nil {
			return "", fmt.Errorf("substr: invalid length: %s", args[2])
		}
		end := start + length
		if end > len(s) {
			end = len(s)
		}
		return s[start:end], nil
	}
	return s[start:], nil
}

func (e *Expander) funcBase64(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return base64Encode([]byte(args[0])), nil
}

// base64Encode performs base64 encoding.
func base64Encode(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte

	for i := 0; i < len(data); i += 3 {
		var n uint32
		remaining := len(data) - i

		if remaining >= 3 {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result = append(result,
				alphabet[n>>18&0x3F],
				alphabet[n>>12&0x3F],
				alphabet[n>>6&0x3F],
				alphabet[n&0x3F],
			)
		} else if remaining == 2 {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result = append(result,
				alphabet[n>>18&0x3F],
				alphabet[n>>12&0x3F],
				alphabet[n>>6&0x3F],
				'=',
			)
		} else {
			n = uint32(data[i]) << 16
			result = append(result,
				alphabet[n>>18&0x3F],
				alphabet[n>>12&0x3F],
				'=',
				'=',
			)
		}
	}

	return string(result)
}

func (e *Expander) funcQuote(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return strconv.Quote(args[0]), nil
}

// Expand is the default expander for convenience.
func Expand(data map[string]any) (map[string]any, error) {
	expander := NewExpander()
	return expander.Expand(data)
}

// ExpandString expands a single string.
func ExpandString(s string) (string, error) {
	expander := NewExpander()
	return expander.expandString(s)
}

// ExpandWithVars expands with custom variables.
func ExpandWithVars(data map[string]any, vars map[string]string) (map[string]any, error) {
	expander := NewExpander()
	expander.SetVars(vars)
	return expander.Expand(data)
}

// StringExpander provides a simple interface for string expansion.
type StringExpander struct {
	expander *Expander
}

// NewStringExpander creates a new string expander.
func NewStringExpander() *StringExpander {
	return &StringExpander{
		expander: NewExpander(),
	}
}

// SetVar sets a variable.
func (se *StringExpander) SetVar(name, value string) {
	se.expander.SetVar(name, value)
}

// Expand expands a string.
func (se *StringExpander) Expand(s string) (string, error) {
	return se.expander.expandString(s)
}

// MustExpand expands a string and panics on error.
func (se *StringExpander) MustExpand(s string) string {
	result, err := se.Expand(s)
	if err != nil {
		panic(err)
	}
	return result
}
