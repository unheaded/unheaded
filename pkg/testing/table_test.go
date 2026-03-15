// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package testing

import (
	"errors"
	"testing"
)

func TestTableTest(t *testing.T) {
	type testCase struct {
		Name     string
		Input    int
		Expected int
	}

	cases := []testCase{
		{Name: "double_one", Input: 1, Expected: 2},
		{Name: "double_two", Input: 2, Expected: 4},
		{Name: "double_zero", Input: 0, Expected: 0},
	}

	NewTableTest[testCase](t).
		Cases(cases).
		Run(func(t *testing.T, tc testCase) {
			result := tc.Input * 2
			if result != tc.Expected {
				t.Errorf("Expected %d, got %d", tc.Expected, result)
			}
		})
}

func TestTableTestWithCustomName(t *testing.T) {
	type testCase struct {
		A        int
		B        int
		Expected int
	}

	cases := []testCase{
		{A: 1, B: 2, Expected: 3},
		{A: 5, B: 3, Expected: 8},
	}

	NewTableTest[testCase](t).
		Cases(cases).
		Name(func(tc testCase) string {
			return "add_test"
		}).
		Run(func(t *testing.T, tc testCase) {
			result := tc.A + tc.B
			if result != tc.Expected {
				t.Errorf("Expected %d, got %d", tc.Expected, result)
			}
		})
}

func TestRunTestCases(t *testing.T) {
	cases := []TestCase[int, int]{
		{Name: "square_2", Input: 2, Expected: 4},
		{Name: "square_3", Input: 3, Expected: 9},
		{Name: "square_0", Input: 0, Expected: 0},
	}

	RunTestCases(t, cases, func(n int) int {
		return n * n
	})
}

func TestRunErrorTestCases(t *testing.T) {
	divide := func(pair [2]int) (int, error) {
		if pair[1] == 0 {
			return 0, errors.New("division by zero")
		}
		return pair[0] / pair[1], nil
	}

	cases := []ErrorTestCase[[2]int, int]{
		{Name: "valid_division", Input: [2]int{10, 2}, Expected: 5, ExpectError: false},
		{Name: "division_by_zero", Input: [2]int{10, 0}, ExpectError: true, ErrorMsg: "division by zero"},
	}

	RunErrorTestCases(t, cases, divide)
}

func TestSubtests(t *testing.T) {
	ranTests := map[string]bool{}

	Subtests(t).
		Run("test_one", func(t *testing.T) {
			ranTests["test_one"] = true
		}).
		Run("test_two", func(t *testing.T) {
			ranTests["test_two"] = true
		})

	if !ranTests["test_one"] {
		t.Error("test_one should have run")
	}
	if !ranTests["test_two"] {
		t.Error("test_two should have run")
	}
}

func TestMatrixTest(t *testing.T) {
	combinations := []map[string]interface{}{}

	NewMatrixTest(t).
		Dimension("size", "small", "large").
		Dimension("color", "red", "blue").
		Run(func(t *testing.T, params map[string]interface{}) {
			combinations = append(combinations, params)
		})

	// Should have 4 combinations: small-red, small-blue, large-red, large-blue
	if len(combinations) != 4 {
		t.Errorf("Expected 4 combinations, got %d", len(combinations))
	}
}

func TestBoundaryTest(t *testing.T) {
	testedValues := []int{}

	bt := NewBoundaryTest[int](t)
	bt.AddMin(-100).
		AddMax(100).
		AddZero(0).
		Add("custom", 50).
		Run(func(t *testing.T, value int) {
			testedValues = append(testedValues, value)
		})

	if len(testedValues) != 4 {
		t.Errorf("Expected 4 boundary values, got %d", len(testedValues))
	}
}

func TestIntBoundaries(t *testing.T) {
	bt := IntBoundaries()
	count := 0

	bt.t = t
	bt.Run(func(t *testing.T, value int) {
		count++
	})

	if count != 5 {
		t.Errorf("Expected 5 int boundaries, got %d", count)
	}
}

func TestStringBoundaries(t *testing.T) {
	bt := StringBoundaries()
	count := 0

	bt.t = t
	bt.Run(func(t *testing.T, value string) {
		count++
	})

	if count != 6 {
		t.Errorf("Expected 6 string boundaries, got %d", count)
	}
}

func TestParameterizedTest(t *testing.T) {
	type Config struct {
		Timeout int
		Retries int
	}

	testedConfigs := []Config{}

	NewParameterizedTest[Config](t).
		Param("default", Config{Timeout: 30, Retries: 3}).
		Param("fast", Config{Timeout: 10, Retries: 1}).
		Param("resilient", Config{Timeout: 60, Retries: 5}).
		Run(func(t *testing.T, cfg Config) {
			testedConfigs = append(testedConfigs, cfg)
		})

	if len(testedConfigs) != 3 {
		t.Errorf("Expected 3 configs, got %d", len(testedConfigs))
	}
}

func TestPropertyTest(t *testing.T) {
	// Test that doubling always produces even numbers
	NewPropertyTest(t).
		Count(50).
		ForAll(
			func(i int) interface{} { return i },
			func(v interface{}) bool {
				n := v.(int)
				doubled := n * 2
				return doubled%2 == 0
			},
		)
}

func TestTableTestWithSkip(t *testing.T) {
	type testCase struct {
		Name  string
		Value int
		Skip  bool
	}

	ranTests := []string{}

	cases := []testCase{
		{Name: "run_this", Value: 1, Skip: false},
		{Name: "skip_this", Value: 2, Skip: true},
		{Name: "run_this_too", Value: 3, Skip: false},
	}

	NewTableTest[testCase](t).
		Cases(cases).
		Skip(func(tc testCase) bool { return tc.Skip }).
		Run(func(t *testing.T, tc testCase) {
			ranTests = append(ranTests, tc.Name)
		})

	// Only 2 tests should have run
	if len(ranTests) != 2 {
		t.Errorf("Expected 2 tests to run, got %d", len(ranTests))
	}
}
