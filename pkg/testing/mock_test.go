// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package testing

import (
	"errors"
	"testing"
)

// Example mock service for testing
type mockService struct {
	Mock
}

func (m *mockService) DoSomething(arg string) error {
	return m.Method("DoSomething").Call(arg).Error(0)
}

func (m *mockService) GetValue(key string) (string, error) {
	ret := m.Method("GetValue").Call(key)
	return ret.String(0), ret.Error(1)
}

func (m *mockService) Process(data []byte) int {
	return m.Method("Process").Call(data).Int(0)
}

func TestMockBasicUsage(t *testing.T) {
	svc := &mockService{}

	// Set up expectation
	svc.On("DoSomething", "test").Return(nil)

	// Call the method
	err := svc.DoSomething("test")
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	// Verify it was called
	svc.AssertCalled(t, "DoSomething", "test")
}

func TestMockReturnError(t *testing.T) {
	svc := &mockService{}

	expectedErr := errors.New("something went wrong")
	svc.On("DoSomething", "fail").Return(expectedErr)

	err := svc.DoSomething("fail")
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestMockMultipleReturns(t *testing.T) {
	svc := &mockService{}

	svc.On("GetValue", "key1").Return("value1", nil)
	svc.On("GetValue", "key2").Return("", errors.New("not found"))

	val, err := svc.GetValue("key1")
	if val != "value1" || err != nil {
		t.Errorf("Expected (value1, nil), got (%s, %v)", val, err)
	}

	val, err = svc.GetValue("key2")
	if val != "" || err == nil {
		t.Errorf("Expected (, error), got (%s, %v)", val, err)
	}
}

func TestMockAnything(t *testing.T) {
	svc := &mockService{}

	svc.On("DoSomething", Anything).Return(nil)

	// Should match any argument
	err := svc.DoSomething("anything")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}

	err = svc.DoSomething("something else")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}

func TestMockAssertNotCalled(t *testing.T) {
	svc := &mockService{}
	svc.On("DoSomething", Anything).Return(nil)

	// Don't call DoSomething

	// This should pass
	svc.AssertNotCalled(t, "DoSomething")
}

func TestMockAssertNumberOfCalls(t *testing.T) {
	svc := &mockService{}
	svc.On("DoSomething", Anything).Return(nil)

	svc.DoSomething("first")
	svc.DoSomething("second")
	svc.DoSomething("third")

	svc.AssertNumberOfCalls(t, "DoSomething", 3)
}

func TestMockTimes(t *testing.T) {
	svc := &mockService{}
	svc.On("DoSomething", "once").Return(nil).Once()

	svc.DoSomething("once")

	// Verify expectations
	svc.AssertExpectations(t)
}

func TestMockRun(t *testing.T) {
	svc := &mockService{}

	var captured string
	svc.On("DoSomething", Anything).Return(nil).Run(func(args []interface{}) {
		captured = args[0].(string)
	})

	svc.DoSomething("captured value")

	if captured != "captured value" {
		t.Errorf("Expected captured value, got %s", captured)
	}
}

func TestMockReset(t *testing.T) {
	svc := &mockService{}
	svc.On("DoSomething", "test").Return(nil)
	svc.DoSomething("test")

	svc.Reset()

	calls := svc.Calls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 calls after reset, got %d", len(calls))
	}
}

func TestArgumentMatcherType(t *testing.T) {
	svc := &mockService{}

	// Match any string
	svc.On("DoSomething", MatchType("")).Return(nil)

	err := svc.DoSomething("any string")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}

func TestArgumentMatcherPredicate(t *testing.T) {
	svc := &mockService{}

	// Match strings longer than 5 characters
	svc.On("DoSomething", MatchPredicate(func(v interface{}) bool {
		s, ok := v.(string)
		return ok && len(s) > 5
	})).Return(nil)

	err := svc.DoSomething("longer string")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}

func TestMockCalls(t *testing.T) {
	svc := &mockService{}
	svc.On("DoSomething", Anything).Return(nil)

	svc.DoSomething("first")
	svc.DoSomething("second")

	calls := svc.Calls()
	if len(calls) != 2 {
		t.Errorf("Expected 2 calls, got %d", len(calls))
	}

	if calls[0].Method != "DoSomething" {
		t.Errorf("Expected method DoSomething, got %s", calls[0].Method)
	}

	if calls[0].Args[0] != "first" {
		t.Errorf("Expected arg 'first', got %v", calls[0].Args[0])
	}
}

func TestInOrder(t *testing.T) {
	svc := &mockService{}
	svc.On("DoSomething", Anything).Return(nil)
	svc.On("GetValue", Anything).Return("val", nil)

	svc.DoSomething("first")
	svc.GetValue("key")
	svc.DoSomething("second")

	// Verify order
	order := NewInOrder()
	order.Add(&svc.Mock, "DoSomething", "first")
	order.Add(&svc.Mock, "GetValue", "key")
	order.Add(&svc.Mock, "DoSomething", "second")
	order.Verify(t)
}

func TestMockFunc(t *testing.T) {
	fn := NewMockFunc[int]()
	fn.Returns(42)

	result := fn.Invoke("arg1", "arg2")
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	if fn.CallCount() != 1 {
		t.Errorf("Expected 1 call, got %d", fn.CallCount())
	}

	calls := fn.GetCalls()
	if len(calls) != 1 || len(calls[0]) != 2 {
		t.Errorf("Expected 1 call with 2 args, got %v", calls)
	}
}

func TestMockFuncCustomHandler(t *testing.T) {
	fn := NewMockFunc[string]()
	fn.Calls(func(args ...interface{}) string {
		return args[0].(string) + "-modified"
	})

	result := fn.Invoke("test")
	if result != "test-modified" {
		t.Errorf("Expected test-modified, got %s", result)
	}
}

func TestReturnValues(t *testing.T) {
	rv := &ReturnValues{values: []interface{}{"hello", 42, true, errors.New("err")}}

	if rv.String(0) != "hello" {
		t.Errorf("Expected 'hello', got %s", rv.String(0))
	}

	if rv.Int(1) != 42 {
		t.Errorf("Expected 42, got %d", rv.Int(1))
	}

	if rv.Bool(2) != true {
		t.Errorf("Expected true, got %v", rv.Bool(2))
	}

	if rv.Error(3) == nil {
		t.Error("Expected error, got nil")
	}

	// Test out of bounds
	if rv.Get(10) != nil {
		t.Error("Expected nil for out of bounds index")
	}
}

func TestMatchStringContaining(t *testing.T) {
	matcher := MatchStringContaining("world")

	if !matcher.Matches("hello world") {
		t.Error("Expected 'hello world' to match")
	}

	if matcher.Matches("hello") {
		t.Error("Expected 'hello' not to match")
	}

	if matcher.Matches(123) {
		t.Error("Expected non-string not to match")
	}
}
