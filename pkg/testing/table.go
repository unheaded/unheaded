// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package testing

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TableTest represents a table-driven test.
type TableTest[TC any] struct {
	t        *testing.T
	cases    []TC
	setupFn  func(*testing.T, TC)
	runFn    func(*testing.T, TC)
	nameFn   func(TC) string
	parallel bool
	skipFn   func(TC) bool
}

// NewTableTest creates a new table-driven test.
func NewTableTest[TC any](t *testing.T) *TableTest[TC] {
	return &TableTest[TC]{t: t}
}

// Cases sets the test cases.
func (tt *TableTest[TC]) Cases(cases []TC) *TableTest[TC] {
	tt.cases = cases
	return tt
}

// Name sets the function to generate test names.
func (tt *TableTest[TC]) Name(fn func(TC) string) *TableTest[TC] {
	tt.nameFn = fn
	return tt
}

// Setup sets a function to run before each test case.
func (tt *TableTest[TC]) Setup(fn func(*testing.T, TC)) *TableTest[TC] {
	tt.setupFn = fn
	return tt
}

// Run sets the test function and executes all cases.
func (tt *TableTest[TC]) Run(fn func(*testing.T, TC)) {
	tt.t.Helper()
	tt.runFn = fn

	for i, tc := range tt.cases {
		tc := tc // Capture range variable
		name := tt.getTestName(i, tc)

		tt.t.Run(name, func(t *testing.T) {
			if tt.parallel {
				t.Parallel()
			}

			if tt.skipFn != nil && tt.skipFn(tc) {
				t.Skip("Skipped by skip function")
			}

			if tt.setupFn != nil {
				tt.setupFn(t, tc)
			}

			tt.runFn(t, tc)
		})
	}
}

// Parallel marks the tests as parallelizable.
func (tt *TableTest[TC]) Parallel() *TableTest[TC] {
	tt.parallel = true
	return tt
}

// Skip sets a function to determine if a test case should be skipped.
func (tt *TableTest[TC]) Skip(fn func(TC) bool) *TableTest[TC] {
	tt.skipFn = fn
	return tt
}

// getTestName generates a test name for a test case.
func (tt *TableTest[TC]) getTestName(index int, tc TC) string {
	if tt.nameFn != nil {
		return tt.nameFn(tc)
	}

	// Try to get name from struct field
	v := reflect.ValueOf(tc)
	if v.Kind() == reflect.Struct {
		nameField := v.FieldByName("Name")
		if nameField.IsValid() && nameField.Kind() == reflect.String {
			name := nameField.String()
			if name != "" {
				return name
			}
		}
	}

	return fmt.Sprintf("case_%d", index)
}

// TestCase is a generic test case structure.
type TestCase[I, E any] struct {
	Name     string
	Input    I
	Expected E
	Skip     bool
	Error    bool
}

// RunTestCases runs a slice of generic test cases.
func RunTestCases[I, E any](t *testing.T, cases []TestCase[I, E], fn func(I) E) {
	t.Helper()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Skip {
				t.Skip("Skipped")
			}

			result := fn(tc.Input)

			if !reflect.DeepEqual(result, tc.Expected) {
				t.Errorf("Input: %v\nExpected: %v\nGot: %v", tc.Input, tc.Expected, result)
			}
		})
	}
}

// RunTestCasesParallel runs test cases in parallel.
func RunTestCasesParallel[I, E any](t *testing.T, cases []TestCase[I, E], fn func(I) E) {
	t.Helper()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			if tc.Skip {
				t.Skip("Skipped")
			}

			result := fn(tc.Input)

			if !reflect.DeepEqual(result, tc.Expected) {
				t.Errorf("Input: %v\nExpected: %v\nGot: %v", tc.Input, tc.Expected, result)
			}
		})
	}
}

// ErrorTestCase is a test case for functions that return errors.
type ErrorTestCase[I, E any] struct {
	Name        string
	Input       I
	Expected    E
	ExpectError bool
	ErrorMsg    string
}

// RunErrorTestCases runs test cases for functions that return (result, error).
func RunErrorTestCases[I, E any](t *testing.T, cases []ErrorTestCase[I, E], fn func(I) (E, error)) {
	t.Helper()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			result, err := fn(tc.Input)

			if tc.ExpectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tc.ErrorMsg != "" && !strings.Contains(err.Error(), tc.ErrorMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.ErrorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(result, tc.Expected) {
				t.Errorf("Input: %v\nExpected: %v\nGot: %v", tc.Input, tc.Expected, result)
			}
		})
	}
}

// SubtestRunner provides a fluent API for running subtests.
type SubtestRunner struct {
	t        *testing.T
	parallel bool
}

// Subtests creates a new subtest runner.
func Subtests(t *testing.T) *SubtestRunner {
	return &SubtestRunner{t: t}
}

// Parallel marks subtests as parallelizable.
func (r *SubtestRunner) Parallel() *SubtestRunner {
	r.parallel = true
	return r
}

// Run runs a named subtest.
func (r *SubtestRunner) Run(name string, fn func(*testing.T)) *SubtestRunner {
	r.t.Run(name, func(t *testing.T) {
		if r.parallel {
			t.Parallel()
		}
		fn(t)
	})
	return r
}

// MatrixTest runs tests across a matrix of parameters.
type MatrixTest struct {
	t          *testing.T
	dimensions []matrixDimension
	parallel   bool
}

type matrixDimension struct {
	name   string
	values []interface{}
}

// NewMatrixTest creates a new matrix test.
func NewMatrixTest(t *testing.T) *MatrixTest {
	return &MatrixTest{t: t}
}

// Dimension adds a dimension to the test matrix.
func (m *MatrixTest) Dimension(name string, values ...interface{}) *MatrixTest {
	m.dimensions = append(m.dimensions, matrixDimension{name: name, values: values})
	return m
}

// Parallel marks the tests as parallelizable.
func (m *MatrixTest) Parallel() *MatrixTest {
	m.parallel = true
	return m
}

// Run executes the test function for each combination in the matrix.
func (m *MatrixTest) Run(fn func(*testing.T, map[string]interface{})) {
	m.t.Helper()

	if len(m.dimensions) == 0 {
		return
	}

	// Generate all combinations
	combinations := m.generateCombinations()

	for _, combo := range combinations {
		combo := combo
		name := m.generateName(combo)

		m.t.Run(name, func(t *testing.T) {
			if m.parallel {
				t.Parallel()
			}
			fn(t, combo)
		})
	}
}

// generateCombinations generates all combinations of dimension values.
func (m *MatrixTest) generateCombinations() []map[string]interface{} {
	if len(m.dimensions) == 0 {
		return nil
	}

	// Calculate total combinations
	total := 1
	for _, d := range m.dimensions {
		total *= len(d.values)
	}

	combinations := make([]map[string]interface{}, total)
	for i := range combinations {
		combinations[i] = make(map[string]interface{})
	}

	// Fill in combinations
	repeat := total
	for _, dim := range m.dimensions {
		repeat /= len(dim.values)
		for i := 0; i < total; i++ {
			valueIndex := (i / repeat) % len(dim.values)
			combinations[i][dim.name] = dim.values[valueIndex]
		}
	}

	return combinations
}

// generateName creates a test name from a combination.
func (m *MatrixTest) generateName(combo map[string]interface{}) string {
	var parts []string
	for _, dim := range m.dimensions {
		value := combo[dim.name]
		parts = append(parts, fmt.Sprintf("%s=%v", dim.name, value))
	}
	return strings.Join(parts, ",")
}

// PropertyTest provides property-based testing helpers.
type PropertyTest struct {
	t      *testing.T
	count  int
	seed   int64
	shrink bool
}

// NewPropertyTest creates a new property test.
func NewPropertyTest(t *testing.T) *PropertyTest {
	return &PropertyTest{
		t:      t,
		count:  100,
		shrink: true,
	}
}

// Count sets the number of test iterations.
func (p *PropertyTest) Count(n int) *PropertyTest {
	p.count = n
	return p
}

// Seed sets the random seed for reproducibility.
func (p *PropertyTest) Seed(seed int64) *PropertyTest {
	p.seed = seed
	return p
}

// NoShrink disables shrinking on failure.
func (p *PropertyTest) NoShrink() *PropertyTest {
	p.shrink = false
	return p
}

// ForAll runs the property test with generated values.
// The generator function should generate random values.
func (p *PropertyTest) ForAll(generator func(i int) interface{}, property func(interface{}) bool) {
	p.t.Helper()

	for i := 0; i < p.count; i++ {
		value := generator(i)
		if !property(value) {
			p.t.Errorf("Property failed for value: %v (iteration %d)", value, i)
			return
		}
	}
}

// BoundaryTest provides boundary value testing.
type BoundaryTest[T any] struct {
	t      *testing.T
	values []boundaryValue[T]
}

type boundaryValue[T any] struct {
	name  string
	value T
}

// NewBoundaryTest creates a new boundary test.
func NewBoundaryTest[T any](t *testing.T) *BoundaryTest[T] {
	return &BoundaryTest[T]{t: t}
}

// Add adds a boundary value to test.
func (b *BoundaryTest[T]) Add(name string, value T) *BoundaryTest[T] {
	b.values = append(b.values, boundaryValue[T]{name: name, value: value})
	return b
}

// AddMin adds a minimum boundary value.
func (b *BoundaryTest[T]) AddMin(value T) *BoundaryTest[T] {
	return b.Add("min", value)
}

// AddMax adds a maximum boundary value.
func (b *BoundaryTest[T]) AddMax(value T) *BoundaryTest[T] {
	return b.Add("max", value)
}

// AddZero adds a zero boundary value.
func (b *BoundaryTest[T]) AddZero(value T) *BoundaryTest[T] {
	return b.Add("zero", value)
}

// Run executes the test function for each boundary value.
func (b *BoundaryTest[T]) Run(fn func(*testing.T, T)) {
	b.t.Helper()

	for _, bv := range b.values {
		bv := bv
		b.t.Run(bv.name, func(t *testing.T) {
			fn(t, bv.value)
		})
	}
}

// IntBoundaries returns common integer boundary values.
func IntBoundaries() *BoundaryTest[int] {
	bt := &BoundaryTest[int]{}
	bt.Add("min_int", -1<<31)
	bt.Add("max_int", 1<<31-1)
	bt.Add("zero", 0)
	bt.Add("one", 1)
	bt.Add("negative_one", -1)
	return bt
}

// StringBoundaries returns common string boundary values.
func StringBoundaries() *BoundaryTest[string] {
	bt := &BoundaryTest[string]{}
	bt.Add("empty", "")
	bt.Add("single_char", "a")
	bt.Add("whitespace", " ")
	bt.Add("newline", "\n")
	bt.Add("tab", "\t")
	bt.Add("unicode", "\u0000")
	return bt
}

// ParameterizedTest provides parameterized testing.
type ParameterizedTest[P any] struct {
	t        *testing.T
	params   []namedParam[P]
	parallel bool
}

type namedParam[P any] struct {
	name  string
	param P
}

// NewParameterizedTest creates a new parameterized test.
func NewParameterizedTest[P any](t *testing.T) *ParameterizedTest[P] {
	return &ParameterizedTest[P]{t: t}
}

// Param adds a named parameter set.
func (p *ParameterizedTest[P]) Param(name string, param P) *ParameterizedTest[P] {
	p.params = append(p.params, namedParam[P]{name: name, param: param})
	return p
}

// Parallel marks tests as parallelizable.
func (p *ParameterizedTest[P]) Parallel() *ParameterizedTest[P] {
	p.parallel = true
	return p
}

// Run executes the test for each parameter set.
func (p *ParameterizedTest[P]) Run(fn func(*testing.T, P)) {
	p.t.Helper()

	for _, np := range p.params {
		np := np
		p.t.Run(np.name, func(t *testing.T) {
			if p.parallel {
				t.Parallel()
			}
			fn(t, np.param)
		})
	}
}
