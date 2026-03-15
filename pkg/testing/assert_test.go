// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package testing

import (
	"errors"
	"testing"
	"time"
)

// testHelper is used to capture assertion failures without actually failing the test
type testHelper struct {
	testing.TB
	failed   bool
	messages []string
}

func newTestHelper(t *testing.T) *testHelper {
	return &testHelper{TB: t}
}

func (h *testHelper) Helper() {}

func (h *testHelper) Error(args ...interface{}) {
	h.failed = true
	if len(args) > 0 {
		if s, ok := args[0].(string); ok {
			h.messages = append(h.messages, s)
		}
	}
}

func (h *testHelper) Errorf(format string, args ...interface{}) {
	h.failed = true
}

func (h *testHelper) Fatal(args ...interface{}) {
	h.failed = true
	if len(args) > 0 {
		if s, ok := args[0].(string); ok {
			h.messages = append(h.messages, s)
		}
	}
}

func (h *testHelper) Fatalf(format string, args ...interface{}) {
	h.failed = true
}

func TestAssertEqual(t *testing.T) {
	assert := Assert(t)

	// Test passing cases
	assert.Equal(1, 1)
	assert.Equal("hello", "hello")
	assert.Equal(true, true)

	// Test failure detection using a helper that doesn't fail the actual test
	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Equal(1, 2)
	if !th.failed || result {
		t.Error("Expected Equal(1, 2) to fail")
	}
}

func TestAssertNotEqual(t *testing.T) {
	assert := Assert(t)

	assert.NotEqual(1, 2)
	assert.NotEqual("hello", "world")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.NotEqual(1, 1)
	if !th.failed || result {
		t.Error("Expected NotEqual(1, 1) to fail")
	}
}

func TestAssertTrueFalse(t *testing.T) {
	assert := Assert(t)

	assert.True(true)
	assert.False(false)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.True(false)
	if !th.failed || result {
		t.Error("Expected True(false) to fail")
	}

	th = newTestHelper(t)
	a = &Asserter{t: th, failNow: false}
	result = a.False(true)
	if !th.failed || result {
		t.Error("Expected False(true) to fail")
	}
}

func TestAssertNil(t *testing.T) {
	assert := Assert(t)

	var nilPtr *int
	var nilSlice []int
	var nilMap map[string]int

	assert.Nil(nil)
	assert.Nil(nilPtr)
	assert.Nil(nilSlice)
	assert.Nil(nilMap)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Nil("not nil")
	if !th.failed || result {
		t.Error("Expected Nil(\"not nil\") to fail")
	}
}

func TestAssertNotNil(t *testing.T) {
	assert := Assert(t)

	assert.NotNil("hello")
	assert.NotNil([]int{1, 2, 3})

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.NotNil(nil)
	if !th.failed || result {
		t.Error("Expected NotNil(nil) to fail")
	}
}

func TestAssertError(t *testing.T) {
	assert := Assert(t)

	err := errors.New("test error")
	assert.Error(err)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Error(nil)
	if !th.failed || result {
		t.Error("Expected Error(nil) to fail")
	}
}

func TestAssertNoError(t *testing.T) {
	assert := Assert(t)

	assert.NoError(nil)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.NoError(errors.New("test"))
	if !th.failed || result {
		t.Error("Expected NoError(err) to fail")
	}
}

func TestAssertErrorContains(t *testing.T) {
	assert := Assert(t)

	err := errors.New("connection refused: port 8080")
	assert.ErrorContains(err, "connection refused")
	assert.ErrorContains(err, "8080")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.ErrorContains(err, "not found")
	if !th.failed || result {
		t.Error("Expected ErrorContains to fail")
	}
}

func TestAssertContains(t *testing.T) {
	assert := Assert(t)

	// String
	assert.Contains("hello world", "world")

	// Slice
	assert.Contains([]int{1, 2, 3}, 2)
	assert.Contains([]string{"a", "b", "c"}, "b")

	// Map
	assert.Contains(map[string]int{"a": 1, "b": 2}, "a")

	// Failure cases
	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Contains("hello", "xyz")
	if !th.failed || result {
		t.Error("Expected Contains(\"hello\", \"xyz\") to fail")
	}

	th = newTestHelper(t)
	a = &Asserter{t: th, failNow: false}
	result = a.Contains([]int{1, 2, 3}, 5)
	if !th.failed || result {
		t.Error("Expected Contains(slice, 5) to fail")
	}
}

func TestAssertNotContains(t *testing.T) {
	assert := Assert(t)

	assert.NotContains("hello world", "xyz")
	assert.NotContains([]int{1, 2, 3}, 5)
	assert.NotContains(map[string]int{"a": 1}, "b")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.NotContains("hello", "ell")
	if !th.failed || result {
		t.Error("Expected NotContains to fail")
	}
}

func TestAssertLen(t *testing.T) {
	assert := Assert(t)

	assert.Len("hello", 5)
	assert.Len([]int{1, 2, 3}, 3)
	assert.Len(map[string]int{"a": 1, "b": 2}, 2)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Len([]int{1, 2}, 5)
	if !th.failed || result {
		t.Error("Expected Len to fail")
	}
}

func TestAssertEmptyNotEmpty(t *testing.T) {
	assert := Assert(t)

	assert.Empty("")
	assert.Empty([]int{})
	assert.NotEmpty("hello")
	assert.NotEmpty([]int{1})

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Empty("hello")
	if !th.failed || result {
		t.Error("Expected Empty(\"hello\") to fail")
	}
}

func TestAssertPanics(t *testing.T) {
	assert := Assert(t)

	panicked := assert.Panics(func() {
		panic("test panic")
	})
	if !panicked {
		t.Error("Expected Panics to return true")
	}

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Panics(func() {
		// no panic
	})
	if !th.failed || result {
		t.Error("Expected Panics to fail when no panic occurs")
	}
}

func TestAssertPanicsWithValue(t *testing.T) {
	assert := Assert(t)

	assert.PanicsWithValue("test panic", func() {
		panic("test panic")
	})

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.PanicsWithValue("expected", func() {
		panic("different")
	})
	if !th.failed || result {
		t.Error("Expected PanicsWithValue to fail")
	}
}

func TestAssertNotPanics(t *testing.T) {
	assert := Assert(t)

	assert.NotPanics(func() {
		// no panic
	})

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.NotPanics(func() {
		panic("oops")
	})
	if !th.failed || result {
		t.Error("Expected NotPanics to fail")
	}
}

func TestAssertEventually(t *testing.T) {
	assert := Assert(t)

	counter := 0
	assert.Eventually(func() bool {
		counter++
		return counter >= 3
	}, 100*time.Millisecond, 10*time.Millisecond)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Eventually(func() bool {
		return false
	}, 50*time.Millisecond, 10*time.Millisecond)
	if !th.failed || result {
		t.Error("Expected Eventually to fail when condition never satisfied")
	}
}

func TestAssertNever(t *testing.T) {
	assert := Assert(t)

	assert.Never(func() bool {
		return false
	}, 50*time.Millisecond, 10*time.Millisecond)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	counter := 0
	result := a.Never(func() bool {
		counter++
		return counter >= 2
	}, 100*time.Millisecond, 10*time.Millisecond)
	if !th.failed || result {
		t.Error("Expected Never to fail when condition becomes true")
	}
}

func TestAssertDeepEqual(t *testing.T) {
	assert := Assert(t)

	type person struct {
		Name string
		Age  int
	}

	assert.DeepEqual(
		person{Name: "John", Age: 30},
		person{Name: "John", Age: 30},
	)

	assert.DeepEqual(
		[]int{1, 2, 3},
		[]int{1, 2, 3},
	)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.DeepEqual(
		person{Name: "John", Age: 30},
		person{Name: "Jane", Age: 25},
	)
	if !th.failed || result {
		t.Error("Expected DeepEqual to fail")
	}
}

func TestAssertStringContains(t *testing.T) {
	assert := Assert(t)

	assert.StringContains("hello world", "world")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.StringContains("hello", "xyz")
	if !th.failed || result {
		t.Error("Expected StringContains to fail")
	}
}

func TestAssertStringHasPrefix(t *testing.T) {
	assert := Assert(t)

	assert.StringHasPrefix("hello world", "hello")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.StringHasPrefix("hello", "world")
	if !th.failed || result {
		t.Error("Expected StringHasPrefix to fail")
	}
}

func TestAssertStringHasSuffix(t *testing.T) {
	assert := Assert(t)

	assert.StringHasSuffix("hello world", "world")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.StringHasSuffix("hello", "world")
	if !th.failed || result {
		t.Error("Expected StringHasSuffix to fail")
	}
}

func TestAssertStringMatches(t *testing.T) {
	assert := Assert(t)

	assert.StringMatches("hello123", `hello\d+`)
	assert.StringMatches("test@example.com", `^[a-z]+@[a-z]+\.[a-z]+$`)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.StringMatches("hello", `\d+`)
	if !th.failed || result {
		t.Error("Expected StringMatches to fail")
	}
}

func TestAssertComparisons(t *testing.T) {
	assert := Assert(t)

	assert.Greater(5, 3)
	assert.GreaterOrEqual(5, 5)
	assert.GreaterOrEqual(5, 3)
	assert.Less(3, 5)
	assert.LessOrEqual(3, 3)
	assert.LessOrEqual(3, 5)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Greater(3, 5)
	if !th.failed || result {
		t.Error("Expected Greater(3, 5) to fail")
	}
}

func TestAssertInDelta(t *testing.T) {
	assert := Assert(t)

	assert.InDelta(1.0, 1.001, 0.01)

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.InDelta(1.0, 2.0, 0.1)
	if !th.failed || result {
		t.Error("Expected InDelta to fail")
	}
}

func TestAssertZero(t *testing.T) {
	assert := Assert(t)

	assert.Zero(0)
	assert.Zero("")
	assert.Zero(nil)
	assert.NotZero(1)
	assert.NotZero("hello")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.Zero(5)
	if !th.failed || result {
		t.Error("Expected Zero(5) to fail")
	}
}

func TestAssertSameType(t *testing.T) {
	assert := Assert(t)

	assert.SameType(1, 2)
	assert.SameType("a", "b")

	th := newTestHelper(t)
	a := &Asserter{t: th, failNow: false}
	result := a.SameType(1, "hello")
	if !th.failed || result {
		t.Error("Expected SameType to fail")
	}
}

func TestAssertFatal(t *testing.T) {
	th := newTestHelper(t)
	a := AssertFatal(th)
	a.Equal(1, 2)
	if !th.failed {
		t.Error("Expected AssertFatal to fail")
	}
}
