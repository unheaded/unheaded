// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package merge provides configuration merging and validation.
package merge

import (
	"fmt"
)

// Merge merges two maps, with the second map taking precedence.
func Merge(base, override map[string]any) map[string]any {
	result := DeepCopy(base)
	return mergeInto(result, override)
}

// mergeInto merges override into base in place.
func mergeInto(base, override map[string]any) map[string]any {
	for key, overrideVal := range override {
		baseVal, exists := base[key]

		if !exists {
			base[key] = DeepCopyValue(overrideVal)
			continue
		}

		// Both values are maps - merge recursively
		baseMap, baseIsMap := baseVal.(map[string]any)
		overrideMap, overrideIsMap := overrideVal.(map[string]any)

		if baseIsMap && overrideIsMap {
			base[key] = mergeInto(baseMap, overrideMap)
			continue
		}

		// Override takes precedence
		base[key] = DeepCopyValue(overrideVal)
	}

	return base
}

// DeepCopy creates a deep copy of a map.
func DeepCopy(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = DeepCopyValue(v)
	}
	return result
}

// DeepCopyValue creates a deep copy of a value.
func DeepCopyValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		return DeepCopy(val)
	case []any:
		return DeepCopySlice(val)
	case []string:
		result := make([]string, len(val))
		copy(result, val)
		return result
	case []int:
		result := make([]int, len(val))
		copy(result, val)
		return result
	case []float64:
		result := make([]float64, len(val))
		copy(result, val)
		return result
	default:
		// Primitive types are immutable, return as-is
		return v
	}
}

// DeepCopySlice creates a deep copy of a slice.
func DeepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}

	result := make([]any, len(s))
	for i, v := range s {
		result[i] = DeepCopyValue(v)
	}
	return result
}

// MergeWithStrategy merges maps using a custom strategy.
type MergeStrategy int

const (
	// StrategyOverride replaces values.
	StrategyOverride MergeStrategy = iota
	// StrategyDeep performs deep merging of nested maps.
	StrategyDeep
	// StrategyAppend appends arrays instead of replacing.
	StrategyAppend
	// StrategyUnion unions arrays (unique values only).
	StrategyUnion
)

// MergeOptions configures merge behavior.
type MergeOptions struct {
	Strategy      MergeStrategy
	ArrayStrategy MergeStrategy
	KeyTransform  func(string) string
	ValueTransform func(any) any
	SkipEmpty     bool
	SkipNil       bool
}

// DefaultMergeOptions returns default merge options.
func DefaultMergeOptions() MergeOptions {
	return MergeOptions{
		Strategy:      StrategyDeep,
		ArrayStrategy: StrategyOverride,
		SkipEmpty:     false,
		SkipNil:       false,
	}
}

// MergeWithOptions merges maps with custom options.
func MergeWithOptions(base, override map[string]any, opts MergeOptions) map[string]any {
	result := DeepCopy(base)
	return mergeWithOptions(result, override, opts)
}

// mergeWithOptions performs the merge with options.
func mergeWithOptions(base, override map[string]any, opts MergeOptions) map[string]any {
	for key, overrideVal := range override {
		// Apply key transform
		if opts.KeyTransform != nil {
			key = opts.KeyTransform(key)
		}

		// Skip nil values if configured
		if opts.SkipNil && overrideVal == nil {
			continue
		}

		// Skip empty values if configured
		if opts.SkipEmpty && isEmpty(overrideVal) {
			continue
		}

		// Apply value transform
		if opts.ValueTransform != nil {
			overrideVal = opts.ValueTransform(overrideVal)
		}

		baseVal, exists := base[key]

		if !exists {
			base[key] = DeepCopyValue(overrideVal)
			continue
		}

		// Handle map merging
		baseMap, baseIsMap := baseVal.(map[string]any)
		overrideMap, overrideIsMap := overrideVal.(map[string]any)

		if baseIsMap && overrideIsMap && opts.Strategy == StrategyDeep {
			base[key] = mergeWithOptions(baseMap, overrideMap, opts)
			continue
		}

		// Handle array merging
		baseArr, baseIsArr := baseVal.([]any)
		overrideArr, overrideIsArr := overrideVal.([]any)

		if baseIsArr && overrideIsArr {
			switch opts.ArrayStrategy {
			case StrategyAppend:
				base[key] = append(DeepCopySlice(baseArr), DeepCopySlice(overrideArr)...)
			case StrategyUnion:
				base[key] = unionArrays(baseArr, overrideArr)
			default:
				base[key] = DeepCopySlice(overrideArr)
			}
			continue
		}

		// Default: override
		base[key] = DeepCopyValue(overrideVal)
	}

	return base
}

// isEmpty checks if a value is considered empty.
func isEmpty(v any) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	default:
		return false
	}
}

// unionArrays creates a union of two arrays.
func unionArrays(a, b []any) []any {
	seen := make(map[string]bool)
	var result []any

	addUnique := func(arr []any) {
		for _, v := range arr {
			key := fmt.Sprintf("%v", v)
			if !seen[key] {
				seen[key] = true
				result = append(result, DeepCopyValue(v))
			}
		}
	}

	addUnique(a)
	addUnique(b)

	return result
}

// Flatten flattens a nested map to a single-level map with dot-notation keys.
func Flatten(m map[string]any) map[string]any {
	result := make(map[string]any)
	flattenRecursive("", m, result)
	return result
}

// flattenRecursive flattens a map recursively.
func flattenRecursive(prefix string, m map[string]any, result map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if nested, ok := v.(map[string]any); ok {
			flattenRecursive(key, nested, result)
		} else {
			result[key] = v
		}
	}
}

// Unflatten converts a flat map with dot-notation keys to a nested map.
func Unflatten(m map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range m {
		setNestedValue(result, key, value)
	}

	return result
}

// setNestedValue sets a value in a nested map using dot notation.
func setNestedValue(m map[string]any, key string, value any) {
	parts := splitKey(key)
	current := m

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

// splitKey splits a dot-notation key into parts.
func splitKey(key string) []string {
	var parts []string
	var current string
	escaped := false

	for _, ch := range key {
		if escaped {
			current += string(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// Filter creates a new map containing only keys that match the filter.
func Filter(m map[string]any, filter func(key string, value any) bool) map[string]any {
	result := make(map[string]any)
	filterRecursive("", m, result, filter)
	return result
}

// filterRecursive filters a map recursively.
func filterRecursive(prefix string, m map[string]any, result map[string]any, filter func(string, any) bool) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if !filter(key, v) {
			continue
		}

		if nested, ok := v.(map[string]any); ok {
			nestedResult := make(map[string]any)
			filterRecursive(key, nested, nestedResult, filter)
			if len(nestedResult) > 0 {
				result[k] = nestedResult
			}
		} else {
			result[k] = DeepCopyValue(v)
		}
	}
}

// Transform applies a transformation function to all values.
func Transform(m map[string]any, transform func(key string, value any) any) map[string]any {
	result := make(map[string]any)
	transformRecursive("", m, result, transform)
	return result
}

// transformRecursive transforms a map recursively.
func transformRecursive(prefix string, m map[string]any, result map[string]any, transform func(string, any) any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if nested, ok := v.(map[string]any); ok {
			nestedResult := make(map[string]any)
			transformRecursive(key, nested, nestedResult, transform)
			result[k] = nestedResult
		} else {
			result[k] = transform(key, v)
		}
	}
}

// Diff returns the differences between two maps.
func Diff(a, b map[string]any) map[string]DiffResult {
	result := make(map[string]DiffResult)
	diffRecursive("", a, b, result)
	return result
}

// DiffResult represents a difference between two values.
type DiffResult struct {
	Type     string
	OldValue any
	NewValue any
}

// diffRecursive computes differences recursively.
func diffRecursive(prefix string, a, b map[string]any, result map[string]DiffResult) {
	// Check for added and modified keys in b
	for k, bVal := range b {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		aVal, exists := a[k]
		if !exists {
			result[key] = DiffResult{Type: "added", NewValue: bVal}
			continue
		}

		// Both are maps - recurse
		aMap, aIsMap := aVal.(map[string]any)
		bMap, bIsMap := bVal.(map[string]any)

		if aIsMap && bIsMap {
			diffRecursive(key, aMap, bMap, result)
			continue
		}

		// Compare values
		if !equal(aVal, bVal) {
			result[key] = DiffResult{Type: "modified", OldValue: aVal, NewValue: bVal}
		}
	}

	// Check for removed keys
	for k, aVal := range a {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if _, exists := b[k]; !exists {
			result[key] = DiffResult{Type: "removed", OldValue: aVal}
		}
	}
}

// equal compares two values for equality.
func equal(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !equal(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i, v := range av {
			if !equal(v, bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// MergeMultiple merges multiple maps in order.
func MergeMultiple(maps ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range maps {
		result = Merge(result, m)
	}
	return result
}

// MergeMultipleWithOptions merges multiple maps with options.
func MergeMultipleWithOptions(opts MergeOptions, maps ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range maps {
		result = MergeWithOptions(result, m, opts)
	}
	return result
}

// Get retrieves a value from a nested map using dot notation.
func Get(m map[string]any, key string) (any, bool) {
	parts := splitKey(key)
	current := any(m)

	for _, part := range parts {
		if current == nil {
			return nil, false
		}
		switch v := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil, false
			}
		default:
			return nil, false
		}
	}

	return current, true
}

// Set sets a value in a nested map using dot notation.
func Set(m map[string]any, key string, value any) {
	setNestedValue(m, key, value)
}

// Delete removes a key from a nested map.
func Delete(m map[string]any, key string) bool {
	parts := splitKey(key)
	if len(parts) == 0 {
		return false
	}

	current := m
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			return false
		}
		current = next
	}

	lastKey := parts[len(parts)-1]
	if _, exists := current[lastKey]; exists {
		delete(current, lastKey)
		return true
	}

	return false
}

// Keys returns all keys in a nested map using dot notation.
func Keys(m map[string]any) []string {
	var result []string
	keysRecursive("", m, &result)
	return result
}

// keysRecursive collects keys recursively.
func keysRecursive(prefix string, m map[string]any, result *[]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		*result = append(*result, key)

		if nested, ok := v.(map[string]any); ok {
			keysRecursive(key, nested, result)
		}
	}
}
