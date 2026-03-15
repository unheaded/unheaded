// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package testing provides test utilities for the Unheaded Kingdom.
// It builds on top of the standard library testing package without external dependencies.
package testing

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// TB is an interface that both *testing.T and *testing.B implement.
// This allows assertions to work with both test and benchmark contexts.
type TB interface {
	Helper()
	Error(args ...interface{})
	Fatal(args ...interface{})
}

// Asserter provides assertion methods for tests.
type Asserter struct {
	t       TB
	failNow bool // If true, use t.Fatal instead of t.Error
}

// Assert creates a new Asserter for the given test.
func Assert(t TB) *Asserter {
	t.Helper()
	return &Asserter{t: t, failNow: false}
}

// AssertFatal creates a new Asserter that fails immediately on assertion failure.
func AssertFatal(t TB) *Asserter {
	t.Helper()
	return &Asserter{t: t, failNow: true}
}

// fail reports a test failure with the given message.
func (a *Asserter) fail(format string, args ...interface{}) {
	a.t.Helper()
	msg := fmt.Sprintf(format, args...)
	if a.failNow {
		a.t.Fatal(msg)
	} else {
		a.t.Error(msg)
	}
}

// Equal asserts that expected and actual are equal using ==.
func (a *Asserter) Equal(expected, actual interface{}) bool {
	a.t.Helper()
	if expected != actual {
		a.fail("Expected %v (type %T), got %v (type %T)", expected, expected, actual, actual)
		return false
	}
	return true
}

// NotEqual asserts that a and b are not equal.
func (a *Asserter) NotEqual(v1, v2 interface{}) bool {
	a.t.Helper()
	if v1 == v2 {
		a.fail("Expected values to be different, but both are: %v", v1)
		return false
	}
	return true
}

// True asserts that condition is true.
func (a *Asserter) True(condition bool, msgAndArgs ...interface{}) bool {
	a.t.Helper()
	if !condition {
		msg := formatMsgAndArgs("Expected condition to be true", msgAndArgs...)
		a.fail("%s", msg)
		return false
	}
	return true
}

// False asserts that condition is false.
func (a *Asserter) False(condition bool, msgAndArgs ...interface{}) bool {
	a.t.Helper()
	if condition {
		msg := formatMsgAndArgs("Expected condition to be false", msgAndArgs...)
		a.fail("%s", msg)
		return false
	}
	return true
}

// Nil asserts that obj is nil.
func (a *Asserter) Nil(obj interface{}) bool {
	a.t.Helper()
	if !isNil(obj) {
		a.fail("Expected nil, got %v (type %T)", obj, obj)
		return false
	}
	return true
}

// NotNil asserts that obj is not nil.
func (a *Asserter) NotNil(obj interface{}) bool {
	a.t.Helper()
	if isNil(obj) {
		a.fail("Expected value to not be nil")
		return false
	}
	return true
}

// Error asserts that err is not nil.
func (a *Asserter) Error(err error) bool {
	a.t.Helper()
	if err == nil {
		a.fail("Expected an error, got nil")
		return false
	}
	return true
}

// NoError asserts that err is nil.
func (a *Asserter) NoError(err error) bool {
	a.t.Helper()
	if err != nil {
		a.fail("Expected no error, got: %v", err)
		return false
	}
	return true
}

// ErrorIs asserts that err matches target using errors.Is semantics.
func (a *Asserter) ErrorIs(err, target error) bool {
	a.t.Helper()
	if err == nil {
		a.fail("Expected error %v, got nil", target)
		return false
	}
	// Manual implementation of errors.Is to avoid import
	for e := err; e != nil; {
		if e == target {
			return true
		}
		// Check if error implements Is method
		if x, ok := e.(interface{ Is(error) bool }); ok {
			if x.Is(target) {
				return true
			}
		}
		// Unwrap
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			break
		}
		e = u.Unwrap()
	}
	a.fail("Expected error to be %v, got %v", target, err)
	return false
}

// ErrorContains asserts that err contains the given substring.
func (a *Asserter) ErrorContains(err error, substr string) bool {
	a.t.Helper()
	if err == nil {
		a.fail("Expected error containing %q, got nil", substr)
		return false
	}
	if !strings.Contains(err.Error(), substr) {
		a.fail("Expected error containing %q, got %q", substr, err.Error())
		return false
	}
	return true
}

// Contains asserts that container contains element.
// Container can be a string, array, slice, or map.
func (a *Asserter) Contains(container, element interface{}) bool {
	a.t.Helper()

	containerValue := reflect.ValueOf(container)
	switch containerValue.Kind() {
	case reflect.String:
		str := containerValue.String()
		elemStr, ok := element.(string)
		if !ok {
			a.fail("Element must be a string when container is a string")
			return false
		}
		if !strings.Contains(str, elemStr) {
			a.fail("Expected string %q to contain %q", str, elemStr)
			return false
		}
		return true

	case reflect.Slice, reflect.Array:
		for i := 0; i < containerValue.Len(); i++ {
			if reflect.DeepEqual(containerValue.Index(i).Interface(), element) {
				return true
			}
		}
		a.fail("Expected %v to contain %v", container, element)
		return false

	case reflect.Map:
		elemValue := reflect.ValueOf(element)
		if containerValue.MapIndex(elemValue).IsValid() {
			return true
		}
		a.fail("Expected map to contain key %v", element)
		return false

	default:
		a.fail("Contains not supported for type %T", container)
		return false
	}
}

// NotContains asserts that container does not contain element.
func (a *Asserter) NotContains(container, element interface{}) bool {
	a.t.Helper()

	containerValue := reflect.ValueOf(container)
	switch containerValue.Kind() {
	case reflect.String:
		str := containerValue.String()
		elemStr, ok := element.(string)
		if !ok {
			a.fail("Element must be a string when container is a string")
			return false
		}
		if strings.Contains(str, elemStr) {
			a.fail("Expected string %q to not contain %q", str, elemStr)
			return false
		}
		return true

	case reflect.Slice, reflect.Array:
		for i := 0; i < containerValue.Len(); i++ {
			if reflect.DeepEqual(containerValue.Index(i).Interface(), element) {
				a.fail("Expected %v to not contain %v", container, element)
				return false
			}
		}
		return true

	case reflect.Map:
		elemValue := reflect.ValueOf(element)
		if containerValue.MapIndex(elemValue).IsValid() {
			a.fail("Expected map to not contain key %v", element)
			return false
		}
		return true

	default:
		a.fail("NotContains not supported for type %T", container)
		return false
	}
}

// Len asserts that obj has the expected length.
func (a *Asserter) Len(obj interface{}, expected int) bool {
	a.t.Helper()

	v := reflect.ValueOf(obj)
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
		actual := v.Len()
		if actual != expected {
			a.fail("Expected length %d, got %d", expected, actual)
			return false
		}
		return true
	default:
		a.fail("Len not supported for type %T", obj)
		return false
	}
}

// Empty asserts that obj is empty.
func (a *Asserter) Empty(obj interface{}) bool {
	a.t.Helper()
	return a.Len(obj, 0)
}

// NotEmpty asserts that obj is not empty.
func (a *Asserter) NotEmpty(obj interface{}) bool {
	a.t.Helper()

	v := reflect.ValueOf(obj)
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
		if v.Len() == 0 {
			a.fail("Expected non-empty value")
			return false
		}
		return true
	default:
		a.fail("NotEmpty not supported for type %T", obj)
		return false
	}
}

// Panics asserts that fn panics.
func (a *Asserter) Panics(fn func()) (panicked bool) {
	a.t.Helper()

	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()

	fn()

	a.fail("Expected function to panic")
	return false
}

// PanicsWithValue asserts that fn panics with the expected value.
func (a *Asserter) PanicsWithValue(expected interface{}, fn func()) bool {
	a.t.Helper()

	var panicValue interface{}
	func() {
		defer func() {
			panicValue = recover()
		}()
		fn()
	}()

	if panicValue == nil {
		a.fail("Expected function to panic with %v, but it didn't panic", expected)
		return false
	}

	if !reflect.DeepEqual(panicValue, expected) {
		a.fail("Expected panic value %v, got %v", expected, panicValue)
		return false
	}
	return true
}

// NotPanics asserts that fn does not panic.
func (a *Asserter) NotPanics(fn func()) bool {
	a.t.Helper()

	var panicValue interface{}
	func() {
		defer func() {
			panicValue = recover()
		}()
		fn()
	}()

	if panicValue != nil {
		a.fail("Expected function not to panic, but it panicked with: %v", panicValue)
		return false
	}
	return true
}

// Eventually asserts that condition returns true within timeout.
func (a *Asserter) Eventually(condition func() bool, timeout, interval time.Duration) bool {
	a.t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(interval)
	}

	a.fail("Condition not satisfied within %v", timeout)
	return false
}

// Never asserts that condition never returns true within duration.
func (a *Asserter) Never(condition func() bool, duration, interval time.Duration) bool {
	a.t.Helper()

	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if condition() {
			a.fail("Condition became true unexpectedly")
			return false
		}
		time.Sleep(interval)
	}
	return true
}

// DeepEqual asserts that expected and actual are deeply equal.
func (a *Asserter) DeepEqual(expected, actual interface{}) bool {
	a.t.Helper()

	if !reflect.DeepEqual(expected, actual) {
		a.fail("Values not deeply equal:\n  expected: %#v\n  actual:   %#v", expected, actual)
		return false
	}
	return true
}

// NotDeepEqual asserts that expected and actual are not deeply equal.
func (a *Asserter) NotDeepEqual(v1, v2 interface{}) bool {
	a.t.Helper()

	if reflect.DeepEqual(v1, v2) {
		a.fail("Expected values to not be deeply equal: %#v", v1)
		return false
	}
	return true
}

// StringContains asserts that str contains substr.
func (a *Asserter) StringContains(str, substr string) bool {
	a.t.Helper()

	if !strings.Contains(str, substr) {
		a.fail("Expected string %q to contain %q", str, substr)
		return false
	}
	return true
}

// StringHasPrefix asserts that str has the given prefix.
func (a *Asserter) StringHasPrefix(str, prefix string) bool {
	a.t.Helper()

	if !strings.HasPrefix(str, prefix) {
		a.fail("Expected string %q to have prefix %q", str, prefix)
		return false
	}
	return true
}

// StringHasSuffix asserts that str has the given suffix.
func (a *Asserter) StringHasSuffix(str, suffix string) bool {
	a.t.Helper()

	if !strings.HasSuffix(str, suffix) {
		a.fail("Expected string %q to have suffix %q", str, suffix)
		return false
	}
	return true
}

// StringMatches asserts that str matches the regex pattern.
func (a *Asserter) StringMatches(str, pattern string) bool {
	a.t.Helper()

	re, err := regexp.Compile(pattern)
	if err != nil {
		a.fail("Invalid regex pattern %q: %v", pattern, err)
		return false
	}

	if !re.MatchString(str) {
		a.fail("Expected string %q to match pattern %q", str, pattern)
		return false
	}
	return true
}

// StringNotMatches asserts that str does not match the regex pattern.
func (a *Asserter) StringNotMatches(str, pattern string) bool {
	a.t.Helper()

	re, err := regexp.Compile(pattern)
	if err != nil {
		a.fail("Invalid regex pattern %q: %v", pattern, err)
		return false
	}

	if re.MatchString(str) {
		a.fail("Expected string %q to not match pattern %q", str, pattern)
		return false
	}
	return true
}

// Greater asserts that a > b.
func (a *Asserter) Greater(v1, v2 interface{}) bool {
	a.t.Helper()
	return a.compareValues(v1, v2, ">", func(c int) bool { return c > 0 })
}

// GreaterOrEqual asserts that a >= b.
func (a *Asserter) GreaterOrEqual(v1, v2 interface{}) bool {
	a.t.Helper()
	return a.compareValues(v1, v2, ">=", func(c int) bool { return c >= 0 })
}

// Less asserts that a < b.
func (a *Asserter) Less(v1, v2 interface{}) bool {
	a.t.Helper()
	return a.compareValues(v1, v2, "<", func(c int) bool { return c < 0 })
}

// LessOrEqual asserts that a <= b.
func (a *Asserter) LessOrEqual(v1, v2 interface{}) bool {
	a.t.Helper()
	return a.compareValues(v1, v2, "<=", func(c int) bool { return c <= 0 })
}

// InDelta asserts that expected and actual are within delta of each other.
func (a *Asserter) InDelta(expected, actual, delta float64) bool {
	a.t.Helper()

	diff := expected - actual
	if diff < 0 {
		diff = -diff
	}

	if diff > delta {
		a.fail("Expected %v and %v to be within %v, but diff is %v", expected, actual, delta, diff)
		return false
	}
	return true
}

// SameType asserts that expected and actual have the same type.
func (a *Asserter) SameType(expected, actual interface{}) bool {
	a.t.Helper()

	expectedType := reflect.TypeOf(expected)
	actualType := reflect.TypeOf(actual)

	if expectedType != actualType {
		a.fail("Expected type %v, got type %v", expectedType, actualType)
		return false
	}
	return true
}

// Implements asserts that obj implements the interface type.
func (a *Asserter) Implements(interfacePtr, obj interface{}) bool {
	a.t.Helper()

	interfaceType := reflect.TypeOf(interfacePtr).Elem()
	objType := reflect.TypeOf(obj)

	if !objType.Implements(interfaceType) {
		a.fail("Expected %T to implement %v", obj, interfaceType)
		return false
	}
	return true
}

// Zero asserts that obj is the zero value for its type.
func (a *Asserter) Zero(obj interface{}) bool {
	a.t.Helper()

	if obj == nil {
		return true
	}

	v := reflect.ValueOf(obj)
	if !v.IsValid() {
		return true
	}

	zero := reflect.Zero(v.Type()).Interface()
	if !reflect.DeepEqual(obj, zero) {
		a.fail("Expected zero value, got %v", obj)
		return false
	}
	return true
}

// NotZero asserts that obj is not the zero value for its type.
func (a *Asserter) NotZero(obj interface{}) bool {
	a.t.Helper()

	if obj == nil {
		a.fail("Expected non-zero value, got nil")
		return false
	}

	v := reflect.ValueOf(obj)
	if !v.IsValid() {
		a.fail("Expected non-zero value")
		return false
	}

	zero := reflect.Zero(v.Type()).Interface()
	if reflect.DeepEqual(obj, zero) {
		a.fail("Expected non-zero value, got %v", obj)
		return false
	}
	return true
}

// compareValues compares two values using the given comparison function.
func (a *Asserter) compareValues(v1, v2 interface{}, op string, cmp func(int) bool) bool {
	a.t.Helper()

	result, err := compare(v1, v2)
	if err != nil {
		a.fail("Cannot compare %T and %T: %v", v1, v2, err)
		return false
	}

	if !cmp(result) {
		a.fail("Expected %v %s %v", v1, op, v2)
		return false
	}
	return true
}

// compare compares two values and returns -1, 0, or 1.
func compare(a, b interface{}) (int, error) {
	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)

	if aVal.Kind() != bVal.Kind() {
		return 0, fmt.Errorf("types mismatch: %T vs %T", a, b)
	}

	switch aVal.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		av, bv := aVal.Int(), bVal.Int()
		if av < bv {
			return -1, nil
		} else if av > bv {
			return 1, nil
		}
		return 0, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		av, bv := aVal.Uint(), bVal.Uint()
		if av < bv {
			return -1, nil
		} else if av > bv {
			return 1, nil
		}
		return 0, nil

	case reflect.Float32, reflect.Float64:
		av, bv := aVal.Float(), bVal.Float()
		if av < bv {
			return -1, nil
		} else if av > bv {
			return 1, nil
		}
		return 0, nil

	case reflect.String:
		av, bv := aVal.String(), bVal.String()
		if av < bv {
			return -1, nil
		} else if av > bv {
			return 1, nil
		}
		return 0, nil

	default:
		return 0, fmt.Errorf("comparison not supported for type %T", a)
	}
}

// isNil checks if a value is nil, handling interface nil issues.
func isNil(obj interface{}) bool {
	if obj == nil {
		return true
	}

	v := reflect.ValueOf(obj)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// formatMsgAndArgs formats optional message and args.
func formatMsgAndArgs(defaultMsg string, msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 {
		return defaultMsg
	}
	if len(msgAndArgs) == 1 {
		if msg, ok := msgAndArgs[0].(string); ok {
			return msg
		}
		return fmt.Sprintf("%v", msgAndArgs[0])
	}
	if format, ok := msgAndArgs[0].(string); ok {
		return fmt.Sprintf(format, msgAndArgs[1:]...)
	}
	return defaultMsg
}
