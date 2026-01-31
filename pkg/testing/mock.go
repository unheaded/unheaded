package testing

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// Anything is a placeholder for any argument value in mock expectations.
var Anything = &anyMatcher{}

type anyMatcher struct{}

func (a *anyMatcher) String() string {
	return "<any>"
}

// Mock provides mock functionality for testing.
type Mock struct {
	mu           sync.RWMutex
	expectations []*Expectation
	calls        []Call
}

// Call represents a recorded method call.
type Call struct {
	Method string
	Args   []interface{}
}

// Expectation represents an expected method call.
type Expectation struct {
	Method        string
	Args          []interface{}
	ReturnArgs    []interface{}
	ExpectedTimes int // Expected number of calls, -1 for any
	actualCalls   int
	runFunc       func(args []interface{})
}

// Return sets the return values for the expectation.
func (e *Expectation) Return(args ...interface{}) *Expectation {
	e.ReturnArgs = args
	return e
}

// Times sets the expected number of calls.
func (e *Expectation) Times(n int) *Expectation {
	e.ExpectedTimes = n
	return e
}

// Once sets the expectation to be called exactly once.
func (e *Expectation) Once() *Expectation {
	return e.Times(1)
}

// Twice sets the expectation to be called exactly twice.
func (e *Expectation) Twice() *Expectation {
	return e.Times(2)
}

// Maybe sets the expectation to be called any number of times (including zero).
func (e *Expectation) Maybe() *Expectation {
	e.ExpectedTimes = -1
	return e
}

// Run sets a function to run when the expectation is matched.
func (e *Expectation) Run(fn func(args []interface{})) *Expectation {
	e.runFunc = fn
	return e
}

// On sets up an expectation for a method call.
func (m *Mock) On(method string, args ...interface{}) *Expectation {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp := &Expectation{
		Method:        method,
		Args:          args,
		ExpectedTimes: -1, // Default: any number of calls
	}
	m.expectations = append(m.expectations, exp)
	return exp
}

// Called records a method call and returns the expected return values.
func (m *Mock) Called(args ...interface{}) *ReturnValues {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get the caller method name
	method := getCallerMethodName()

	// Record the call
	m.calls = append(m.calls, Call{
		Method: method,
		Args:   args,
	})

	// Find matching expectation
	for _, exp := range m.expectations {
		if exp.Method == method && m.argsMatch(exp.Args, args) {
			exp.actualCalls++
			if exp.runFunc != nil {
				exp.runFunc(args)
			}
			return &ReturnValues{values: exp.ReturnArgs}
		}
	}

	// No matching expectation found
	return &ReturnValues{}
}

// argsMatch checks if expected and actual arguments match.
func (m *Mock) argsMatch(expected, actual []interface{}) bool {
	if len(expected) != len(actual) {
		return false
	}

	for i, exp := range expected {
		// Check for Anything matcher
		if _, ok := exp.(*anyMatcher); ok {
			continue
		}

		// Check for custom matcher
		if matcher, ok := exp.(ArgumentMatcher); ok {
			if !matcher.Matches(actual[i]) {
				return false
			}
			continue
		}

		// Direct comparison
		if !reflect.DeepEqual(exp, actual[i]) {
			return false
		}
	}

	return true
}

// AssertCalled asserts that a method was called with the given arguments.
func (m *Mock) AssertCalled(t *testing.T, method string, args ...interface{}) bool {
	t.Helper()
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, call := range m.calls {
		if call.Method == method && m.argsMatch(args, call.Args) {
			return true
		}
	}

	t.Errorf("Expected method %q to be called with %v, but it wasn't", method, args)
	return false
}

// AssertNotCalled asserts that a method was not called.
func (m *Mock) AssertNotCalled(t *testing.T, method string, args ...interface{}) bool {
	t.Helper()
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, call := range m.calls {
		if call.Method == method {
			if len(args) == 0 || m.argsMatch(args, call.Args) {
				t.Errorf("Expected method %q not to be called, but it was called with %v", method, call.Args)
				return false
			}
		}
	}

	return true
}

// AssertNumberOfCalls asserts that a method was called exactly n times.
func (m *Mock) AssertNumberOfCalls(t *testing.T, method string, expectedCalls int) bool {
	t.Helper()
	m.mu.RLock()
	defer m.mu.RUnlock()

	actualCalls := 0
	for _, call := range m.calls {
		if call.Method == method {
			actualCalls++
		}
	}

	if actualCalls != expectedCalls {
		t.Errorf("Expected %q to be called %d time(s), but it was called %d time(s)", method, expectedCalls, actualCalls)
		return false
	}

	return true
}

// AssertExpectations asserts that all expectations were met.
func (m *Mock) AssertExpectations(t *testing.T) bool {
	t.Helper()
	m.mu.RLock()
	defer m.mu.RUnlock()

	success := true
	for _, exp := range m.expectations {
		if exp.ExpectedTimes >= 0 && exp.actualCalls != exp.ExpectedTimes {
			t.Errorf("Expected %q to be called %d time(s), but it was called %d time(s)",
				exp.Method, exp.ExpectedTimes, exp.actualCalls)
			success = false
		}
	}

	return success
}

// Calls returns all recorded calls.
func (m *Mock) Calls() []Call {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Call{}, m.calls...)
}

// Reset clears all expectations and recorded calls.
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expectations = nil
	m.calls = nil
}

// ReturnValues holds return values from a mock call.
type ReturnValues struct {
	values []interface{}
}

// Get returns the value at index i.
func (r *ReturnValues) Get(i int) interface{} {
	if i >= len(r.values) {
		return nil
	}
	return r.values[i]
}

// Error returns the value at index i as an error.
func (r *ReturnValues) Error(i int) error {
	if i >= len(r.values) {
		return nil
	}
	if r.values[i] == nil {
		return nil
	}
	err, ok := r.values[i].(error)
	if !ok {
		return nil
	}
	return err
}

// String returns the value at index i as a string.
func (r *ReturnValues) String(i int) string {
	if i >= len(r.values) {
		return ""
	}
	s, _ := r.values[i].(string)
	return s
}

// Int returns the value at index i as an int.
func (r *ReturnValues) Int(i int) int {
	if i >= len(r.values) {
		return 0
	}
	n, _ := r.values[i].(int)
	return n
}

// Bool returns the value at index i as a bool.
func (r *ReturnValues) Bool(i int) bool {
	if i >= len(r.values) {
		return false
	}
	b, _ := r.values[i].(bool)
	return b
}

// ArgumentMatcher is an interface for custom argument matching.
type ArgumentMatcher interface {
	Matches(actual interface{}) bool
}

// MatchFunc is a function-based argument matcher.
type MatchFunc func(interface{}) bool

// Matches implements ArgumentMatcher.
func (f MatchFunc) Matches(actual interface{}) bool {
	return f(actual)
}

// MatchAny returns a matcher that matches any value.
func MatchAny() ArgumentMatcher {
	return MatchFunc(func(interface{}) bool { return true })
}

// MatchType returns a matcher that matches values of the given type.
func MatchType(expected interface{}) ArgumentMatcher {
	expectedType := reflect.TypeOf(expected)
	return MatchFunc(func(actual interface{}) bool {
		return reflect.TypeOf(actual) == expectedType
	})
}

// MatchPredicate returns a matcher that uses a predicate function.
func MatchPredicate(fn func(interface{}) bool) ArgumentMatcher {
	return MatchFunc(fn)
}

// MatchString returns a matcher that matches the given string.
func MatchString(s string) ArgumentMatcher {
	return MatchFunc(func(actual interface{}) bool {
		str, ok := actual.(string)
		return ok && str == s
	})
}

// MatchStringContaining returns a matcher that matches strings containing the substring.
func MatchStringContaining(substr string) ArgumentMatcher {
	return MatchFunc(func(actual interface{}) bool {
		str, ok := actual.(string)
		if !ok {
			return false
		}
		return len(str) > 0 && len(substr) > 0 && containsString(str, substr)
	})
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getCallerMethodName gets the method name from the call stack.
// This is a simplified implementation that expects the mock method to call Called().
func getCallerMethodName() string {
	// In practice, we would use runtime.Callers to get the actual method name.
	// For now, we rely on the mock implementation to pass the method name explicitly.
	return ""
}

// MethodCall is an alternative way to record calls with explicit method names.
type MethodCall struct {
	mock   *Mock
	method string
}

// Method returns a MethodCall for explicit method tracking.
func (m *Mock) Method(name string) *MethodCall {
	return &MethodCall{mock: m, method: name}
}

// Call records the method call with arguments.
func (mc *MethodCall) Call(args ...interface{}) *ReturnValues {
	mc.mock.mu.Lock()
	defer mc.mock.mu.Unlock()

	// Record the call
	mc.mock.calls = append(mc.mock.calls, Call{
		Method: mc.method,
		Args:   args,
	})

	// Find matching expectation
	for _, exp := range mc.mock.expectations {
		if exp.Method == mc.method && mc.mock.argsMatch(exp.Args, args) {
			exp.actualCalls++
			if exp.runFunc != nil {
				exp.runFunc(args)
			}
			return &ReturnValues{values: exp.ReturnArgs}
		}
	}

	return &ReturnValues{}
}

// Spy wraps a real implementation and records calls.
type Spy struct {
	Mock
	realImpl interface{}
}

// NewSpy creates a new spy that wraps a real implementation.
func NewSpy(realImpl interface{}) *Spy {
	return &Spy{realImpl: realImpl}
}

// CallReal calls the real implementation if available.
func (s *Spy) CallReal(method string, args ...interface{}) []interface{} {
	if s.realImpl == nil {
		return nil
	}

	v := reflect.ValueOf(s.realImpl)
	m := v.MethodByName(method)
	if !m.IsValid() {
		return nil
	}

	// Convert args to reflect.Value
	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		in[i] = reflect.ValueOf(arg)
	}

	// Call the method
	out := m.Call(in)

	// Convert results back to interface{}
	results := make([]interface{}, len(out))
	for i, v := range out {
		results[i] = v.Interface()
	}

	return results
}

// InOrder verifies that mock calls happened in the specified order.
type InOrder struct {
	calls []struct {
		mock   *Mock
		method string
		args   []interface{}
	}
}

// NewInOrder creates a new InOrder verifier.
func NewInOrder() *InOrder {
	return &InOrder{}
}

// Add adds an expected call to the order.
func (o *InOrder) Add(mock *Mock, method string, args ...interface{}) *InOrder {
	o.calls = append(o.calls, struct {
		mock   *Mock
		method string
		args   []interface{}
	}{mock, method, args})
	return o
}

// Verify checks that calls happened in order.
func (o *InOrder) Verify(t *testing.T) bool {
	t.Helper()

	callIndex := make(map[*Mock]int)
	for _, expected := range o.calls {
		mock := expected.mock
		calls := mock.Calls()

		found := false
		startIdx := callIndex[mock]
		for i := startIdx; i < len(calls); i++ {
			if calls[i].Method == expected.method && mock.argsMatch(expected.args, calls[i].Args) {
				callIndex[mock] = i + 1
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected call to %q with args %v was not found in order", expected.method, expected.args)
			return false
		}
	}

	return true
}

// MockFunc is a mock for a standalone function.
type MockFunc[T any] struct {
	mu     sync.RWMutex
	calls  [][]interface{}
	result T
	fn     func(args ...interface{}) T
}

// NewMockFunc creates a new function mock.
func NewMockFunc[T any]() *MockFunc[T] {
	return &MockFunc[T]{}
}

// Returns sets the return value for the mock function.
func (m *MockFunc[T]) Returns(v T) *MockFunc[T] {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.result = v
	m.fn = nil
	return m
}

// Calls sets a custom function to handle calls.
func (m *MockFunc[T]) Calls(fn func(args ...interface{}) T) *MockFunc[T] {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fn = fn
	return m
}

// Invoke calls the mock function.
func (m *MockFunc[T]) Invoke(args ...interface{}) T {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, args)

	if m.fn != nil {
		return m.fn(args...)
	}
	return m.result
}

// CallCount returns the number of times the function was called.
func (m *MockFunc[T]) CallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.calls)
}

// GetCalls returns all recorded calls.
func (m *MockFunc[T]) GetCalls() [][]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([][]interface{}{}, m.calls...)
}

// MockError is a convenience function to create an error for mock returns.
func MockError(msg string) error {
	return fmt.Errorf("%s", msg)
}
