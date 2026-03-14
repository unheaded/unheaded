// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper creates an Expander with builtin functions registered.
func newTestExpander() *Expander {
	e := NewExpander()
	RegisterBuiltinFunctions(e)
	return e
}

// ---------------------------------------------------------------------
// String functions
// ---------------------------------------------------------------------

func TestFuncConcat(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, ""},
		{"single", []string{"hello"}, "hello"},
		{"two", []string{"hello", " world"}, "hello world"},
		{"three", []string{"a", "b", "c"}, "abc"},
		{"empty strings", []string{"", "", ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcConcat(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncContains(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"found", []string{"hello world", "world"}, "true", false},
		{"not found", []string{"hello world", "xyz"}, "false", false},
		{"empty substring", []string{"hello", ""}, "true", false},
		{"too few args", []string{"hello"}, "", true},
		{"no args", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcContains(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncHasPrefix(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"has prefix", []string{"hello", "hel"}, "true", false},
		{"no prefix", []string{"hello", "xyz"}, "false", false},
		{"too few args", []string{"hello"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcHasPrefix(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncHasSuffix(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"has suffix", []string{"hello", "llo"}, "true", false},
		{"no suffix", []string{"hello", "xyz"}, "false", false},
		{"too few args", []string{"hello"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcHasSuffix(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncRepeat(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"repeat 3", []string{"ab", "3"}, "ababab", false},
		{"repeat 0", []string{"ab", "0"}, "", false},
		{"repeat 1", []string{"ab", "1"}, "ab", false},
		{"invalid count", []string{"ab", "xyz"}, "", true},
		{"too few args", []string{"ab"}, "", true},
		{"no args", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcRepeat(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncReverse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"normal", []string{"hello"}, "olleh"},
		{"empty", []string{""}, ""},
		{"single char", []string{"a"}, "a"},
		{"unicode", []string{"ab"}, "ba"},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcReverse(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncTitle(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"lowercase", []string{"hello world"}, "Hello World"},
		{"already titled", []string{"Hello"}, "Hello"},
		{"empty", []string{""}, ""},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcTitle(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncTrimPrefix(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"has prefix", []string{"hello", "hel"}, "lo", false},
		{"no prefix", []string{"hello", "xyz"}, "hello", false},
		{"empty prefix", []string{"hello", ""}, "hello", false},
		{"too few args", []string{"hello"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcTrimPrefix(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncTrimSuffix(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"has suffix", []string{"hello", "llo"}, "he", false},
		{"no suffix", []string{"hello", "xyz"}, "hello", false},
		{"too few args", []string{"hello"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcTrimSuffix(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncPadLeft(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"pad with spaces", []string{"hi", "5"}, "   hi", false},
		{"custom pad char", []string{"hi", "5", "0"}, "000hi", false},
		{"already wide enough", []string{"hello", "3"}, "hello", false},
		{"invalid width", []string{"hi", "abc"}, "", true},
		{"too few args", []string{"hi"}, "", true},
		{"no args", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcPadLeft(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncPadRight(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"pad with spaces", []string{"hi", "5"}, "hi   ", false},
		{"custom pad char", []string{"hi", "5", "."}, "hi...", false},
		{"already wide enough", []string{"hello", "3"}, "hello", false},
		{"invalid width", []string{"hi", "abc"}, "", true},
		{"too few args", []string{"hi"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcPadRight(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Math functions
// ---------------------------------------------------------------------

func TestFuncAdd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"integers", []string{"3", "4"}, "7", false},
		{"floats", []string{"1.5", "2.5"}, "4", false},
		{"negative", []string{"-1", "1"}, "0", false},
		{"float result", []string{"1.1", "2.2"}, "3.3000000000000003", false},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few args", []string{"1"}, "", true},
		{"no args", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcAdd(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncSub(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"integers", []string{"10", "3"}, "7", false},
		{"negative result", []string{"3", "10"}, "-7", false},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few args", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcSub(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncMul(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"integers", []string{"3", "4"}, "12", false},
		{"by zero", []string{"5", "0"}, "0", false},
		{"floats", []string{"2.5", "4"}, "10", false},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few args", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcMul(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncDiv(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"even", []string{"10", "2"}, "5", false},
		{"fractional", []string{"10", "3"}, "3.3333333333333335", false},
		{"divide by zero", []string{"10", "0"}, "", true},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few args", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcDiv(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncMod(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"10 mod 3", []string{"10", "3"}, "1", false},
		{"even mod", []string{"10", "5"}, "0", false},
		{"mod by zero", []string{"10", "0"}, "", true},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few args", []string{"1"}, "", true},
		{"float arg", []string{"1.5", "2"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcMod(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncMax(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"two ints", []string{"3", "7"}, "7", false},
		{"three values", []string{"1", "9", "5"}, "9", false},
		{"negative", []string{"-5", "-2"}, "-2", false},
		{"equal", []string{"3", "3"}, "3", false},
		{"invalid arg", []string{"1", "abc"}, "", true},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"too few args", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcMax(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncMin(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"two ints", []string{"3", "7"}, "3", false},
		{"three values", []string{"9", "1", "5"}, "1", false},
		{"negative", []string{"-5", "-2"}, "-5", false},
		{"equal", []string{"3", "3"}, "3", false},
		{"invalid arg", []string{"1", "abc"}, "", true},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"too few args", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcMin(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Date/time functions
// ---------------------------------------------------------------------

func TestFuncNow(t *testing.T) {
	// Default format (RFC3339)
	got, err := funcNow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty result")
	}

	// Custom format
	got, err = funcNow("2006-01-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 10 { // YYYY-MM-DD
		t.Errorf("expected date format, got %q", got)
	}
}

func TestFuncDate(t *testing.T) {
	got, err := funcDate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 10 {
		t.Errorf("expected YYYY-MM-DD format, got %q", got)
	}
}

func TestFuncFormatDate(t *testing.T) {
	// RFC3339 input
	got, err := funcFormatDate("2024-01-15T10:30:00Z", "2006-01-02")
	if err != nil {
		t.Fatalf("rfc3339: unexpected error: %v", err)
	}
	if got != "2024-01-15" {
		t.Errorf("rfc3339: got %q, want %q", got, "2024-01-15")
	}

	// Unix timestamp input — compare against local time.Unix result
	got, err = funcFormatDate("0", "2006")
	if err != nil {
		t.Fatalf("unix: unexpected error: %v", err)
	}
	if got == "" {
		t.Error("unix: expected non-empty result")
	}

	// Invalid timestamp
	_, err = funcFormatDate("not-a-date", "2006")
	if err == nil {
		t.Error("invalid: expected error")
	}

	// Too few args
	_, err = funcFormatDate("2024-01-15T10:30:00Z")
	if err == nil {
		t.Error("too few args: expected error")
	}
}

// ---------------------------------------------------------------------
// Path functions
// ---------------------------------------------------------------------

func TestFuncBasename(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"normal", []string{"/foo/bar/baz.txt"}, "baz.txt"},
		{"no dir", []string{"baz.txt"}, "baz.txt"},
		{"trailing slash", []string{"/foo/bar/"}, "bar"},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcBasename(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncDirname(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"normal", []string{"/foo/bar/baz.txt"}, "/foo/bar"},
		{"root", []string{"/baz.txt"}, "/"},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcDirname(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncExt(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"has ext", []string{"file.txt"}, ".txt"},
		{"no ext", []string{"file"}, ""},
		{"double ext", []string{"file.tar.gz"}, ".gz"},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcExt(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncClean(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"double slash", []string{"/foo//bar"}, "/foo/bar"},
		{"dot segments", []string{"/foo/./bar/../baz"}, "/foo/baz"},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcClean(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncAbs(t *testing.T) {
	// No args
	got, err := funcAbs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty for no args, got %q", got)
	}

	// Absolute path stays the same
	got, err = funcAbs("/foo/bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/foo/bar" {
		t.Errorf("expected /foo/bar, got %q", got)
	}

	// Relative path gets cwd prepended
	got, err = funcAbs("relative")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cwd, _ := os.Getwd()
	want := filepath.Join(cwd, "relative")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------
// Encoding functions
// ---------------------------------------------------------------------

func TestFuncSHA256(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"hello", []string{"hello"}, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{"empty", []string{""}, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcSHA256(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncHex(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"ascii", []string{"AB"}, "4142"},
		{"empty", []string{""}, ""},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcHex(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Conditional functions
// ---------------------------------------------------------------------

func TestFuncIf(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"true literal", []string{"true", "yes", "no"}, "yes", false},
		{"false literal", []string{"false", "yes", "no"}, "no", false},
		{"1 is truthy", []string{"1", "yes", "no"}, "yes", false},
		{"0 is falsy", []string{"0", "yes", "no"}, "no", false},
		{"empty is falsy", []string{"", "yes", "no"}, "no", false},
		{"non-empty truthy", []string{"anything", "yes", "no"}, "yes", false},
		{"too few args", []string{"true", "yes"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcIf(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncEq(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"equal", []string{"a", "a"}, "true", false},
		{"not equal", []string{"a", "b"}, "false", false},
		{"too few", []string{"a"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcEq(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncNe(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"not equal", []string{"a", "b"}, "true", false},
		{"equal", []string{"a", "a"}, "false", false},
		{"too few", []string{"a"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcNe(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncLt(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"less", []string{"1", "2"}, "true", false},
		{"equal", []string{"2", "2"}, "false", false},
		{"greater", []string{"3", "2"}, "false", false},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcLt(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncGt(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"greater", []string{"3", "2"}, "true", false},
		{"equal", []string{"2", "2"}, "false", false},
		{"less", []string{"1", "2"}, "false", false},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcGt(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncLe(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"less", []string{"1", "2"}, "true", false},
		{"equal", []string{"2", "2"}, "true", false},
		{"greater", []string{"3", "2"}, "false", false},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcLe(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncGe(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"greater", []string{"3", "2"}, "true", false},
		{"equal", []string{"2", "2"}, "true", false},
		{"less", []string{"1", "2"}, "false", false},
		{"invalid first", []string{"abc", "1"}, "", true},
		{"invalid second", []string{"1", "abc"}, "", true},
		{"too few", []string{"1"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcGe(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncAnd(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"all truthy", []string{"true", "yes", "1"}, "true"},
		{"one false", []string{"true", "false", "yes"}, "false"},
		{"one empty", []string{"true", "", "yes"}, "false"},
		{"one zero", []string{"true", "0", "yes"}, "false"},
		{"no args", nil, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcAnd(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncOr(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"one truthy", []string{"false", "yes", "0"}, "true"},
		{"all falsy", []string{"false", "", "0"}, "false"},
		{"first truthy", []string{"true", "false"}, "true"},
		{"no args", nil, "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcOr(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncNot(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"true becomes false", []string{"true"}, "false", false},
		{"false becomes true", []string{"false"}, "true", false},
		{"empty becomes true", []string{""}, "true", false},
		{"0 becomes true", []string{"0"}, "true", false},
		{"non-empty becomes false", []string{"anything"}, "false", false},
		{"no args", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcNot(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// System functions
// ---------------------------------------------------------------------

func TestFuncHostname(t *testing.T) {
	got, err := funcHostname()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected, _ := os.Hostname()
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFuncPwd(t *testing.T) {
	got, err := funcPwd()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected, _ := os.Getwd()
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFuncUser(t *testing.T) {
	got, err := funcUser()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := os.Getenv("USER")
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFuncHome(t *testing.T) {
	got, err := funcHome()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected, _ := os.UserHomeDir()
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// ---------------------------------------------------------------------
// Utility functions
// ---------------------------------------------------------------------

func TestFuncCoalesce(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"first non-empty", []string{"", "", "hello", "world"}, "hello"},
		{"first arg", []string{"first", "second"}, "first"},
		{"all empty", []string{"", "", ""}, ""},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcCoalesce(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncTernary(t *testing.T) {
	// ternary delegates to funcIf, so just verify it works
	got, err := funcTernary("true", "yes", "no")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "yes" {
		t.Errorf("got %q, want %q", got, "yes")
	}

	got, err = funcTernary("false", "yes", "no")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "no" {
		t.Errorf("got %q, want %q", got, "no")
	}

	// Error case
	_, err = funcTernary("true")
	if err == nil {
		t.Error("expected error for too few args")
	}
}

func TestFuncEmpty(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty string", []string{""}, "true"},
		{"non-empty", []string{"hello"}, "false"},
		{"no args", nil, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcEmpty(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFuncLen(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"normal", []string{"hello"}, "5"},
		{"empty", []string{""}, "0"},
		{"no args", nil, "0"},
		{"unicode", []string{"abc"}, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := funcLen(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// RegisterBuiltinFunctions integration test
// ---------------------------------------------------------------------

func TestRegisterBuiltinFunctions(t *testing.T) {
	e := newTestExpander()

	// Verify a sampling of registered functions work through the expander
	expectedFuncs := []string{
		"concat", "contains", "hasPrefix", "hasSuffix", "repeat",
		"reverse", "title", "trimPrefix", "trimSuffix", "padLeft", "padRight",
		"add", "sub", "mul", "div", "mod", "max", "min",
		"now", "date", "formatDate",
		"basename", "dirname", "ext", "clean", "abs",
		"sha256", "hex",
		"if", "eq", "ne", "lt", "gt", "le", "ge", "and", "or", "not",
		"hostname", "pwd", "user", "home",
		"coalesce", "ternary", "empty", "len",
	}

	for _, name := range expectedFuncs {
		if _, ok := e.funcs[name]; !ok {
			t.Errorf("expected function %q to be registered", name)
		}
	}
}

// ---------------------------------------------------------------------
// Integration: calling functions through the expander
// ---------------------------------------------------------------------

func TestExpanderCallsBuiltinFunctions(t *testing.T) {
	e := newTestExpander()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"concat via expand", "${concat(hello, world)}", "helloworld", false},
		{"add via expand", "${add(2, 3)}", "5", false},
		{"upper via expand", "${upper(hello)}", "HELLO", false},
		{"title via expand", "${title(hello world)}", "Hello World", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.expandString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Math formatting edge cases
// ---------------------------------------------------------------------

func TestMathIntegerFormatting(t *testing.T) {
	// Verify that integer results are formatted without decimal point
	got, err := funcAdd("1.0", "2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, ".") {
		t.Errorf("expected integer format, got %q", got)
	}
	if got != "3" {
		t.Errorf("got %q, want %q", got, "3")
	}
}

func TestFuncMaxFloatResult(t *testing.T) {
	got, err := funcMax("1.5", "2.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2.5" {
		t.Errorf("got %q, want %q", got, "2.5")
	}
}

func TestFuncMinFloatResult(t *testing.T) {
	got, err := funcMin("1.5", "2.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.5" {
		t.Errorf("got %q, want %q", got, "1.5")
	}
}

// ---------------------------------------------------------------------
// Template.go: Resolvers
// ---------------------------------------------------------------------

func TestEnvResolver(t *testing.T) {
	r := &EnvResolver{}

	t.Setenv("TEST_RESOLVE_VAR", "hello")
	v, ok := r.Resolve("TEST_RESOLVE_VAR")
	if !ok || v != "hello" {
		t.Errorf("got %q, ok=%v, want hello, ok=true", v, ok)
	}

	_, ok = r.Resolve("UNLIKELY_UNSET_VAR_XYZ123")
	if ok {
		t.Error("expected ok=false for unset var")
	}
}

func TestMapResolver(t *testing.T) {
	r := NewMapResolver(map[string]string{"key": "val"})
	v, ok := r.Resolve("key")
	if !ok || v != "val" {
		t.Errorf("got %q, ok=%v", v, ok)
	}
	_, ok = r.Resolve("missing")
	if ok {
		t.Error("expected ok=false")
	}
}

func TestChainResolver(t *testing.T) {
	m1 := NewMapResolver(map[string]string{"a": "from-m1"})
	m2 := NewMapResolver(map[string]string{"a": "from-m2", "b": "from-m2"})
	cr := NewChainResolver(m1, m2)

	// First resolver wins
	v, ok := cr.Resolve("a")
	if !ok || v != "from-m1" {
		t.Errorf("got %q, want from-m1", v)
	}

	// Falls through to second
	v, ok = cr.Resolve("b")
	if !ok || v != "from-m2" {
		t.Errorf("got %q, want from-m2", v)
	}

	// Not found in any
	_, ok = cr.Resolve("c")
	if ok {
		t.Error("expected ok=false")
	}
}

// ---------------------------------------------------------------------
// Template.go: Expander
// ---------------------------------------------------------------------

func TestNewExpanderWithResolver(t *testing.T) {
	m := NewMapResolver(map[string]string{"myvar": "myval"})
	e := NewExpanderWithResolver(m)
	got, err := e.expandString("${myvar}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "myval" {
		t.Errorf("got %q, want %q", got, "myval")
	}
}

func TestExpanderSetVar(t *testing.T) {
	e := NewExpander()
	e.SetVar("foo", "bar")
	got, err := e.expandString("${foo}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}

func TestExpanderSetVars(t *testing.T) {
	e := NewExpander()
	e.SetVars(map[string]string{"x": "1", "y": "2"})
	got, err := e.expandString("${x}-${y}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1-2" {
		t.Errorf("got %q, want %q", got, "1-2")
	}
}

func TestExpand(t *testing.T) {
	e := NewExpander()
	e.SetVar("name", "world")
	result, err := e.Expand(map[string]any{
		"greeting": "hello ${name}",
		"nested": map[string]any{
			"key": "${name}",
		},
		"list": []any{"${name}", "literal"},
		"number": 42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["greeting"] != "hello world" {
		t.Errorf("greeting: got %q", result["greeting"])
	}
	nested := result["nested"].(map[string]any)
	if nested["key"] != "world" {
		t.Errorf("nested.key: got %q", nested["key"])
	}
	list := result["list"].([]any)
	if list[0] != "world" {
		t.Errorf("list[0]: got %q", list[0])
	}
	if result["number"] != 42 {
		t.Errorf("number: got %v", result["number"])
	}
}

func TestExpandStringFunc(t *testing.T) {
	t.Setenv("TEST_ES_VAR", "hello")
	got, err := ExpandString("${TEST_ES_VAR}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestExpandWithVars(t *testing.T) {
	result, err := ExpandWithVars(
		map[string]any{"val": "${mykey}"},
		map[string]string{"mykey": "myvalue"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["val"] != "myvalue" {
		t.Errorf("got %q, want %q", result["val"], "myvalue")
	}
}

func TestPackageLevelExpand(t *testing.T) {
	t.Setenv("TEST_PKG_VAR", "pkgval")
	result, err := Expand(map[string]any{"k": "${TEST_PKG_VAR}"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["k"] != "pkgval" {
		t.Errorf("got %q", result["k"])
	}
}

// ---------------------------------------------------------------------
// Template.go: StringExpander
// ---------------------------------------------------------------------

func TestStringExpander(t *testing.T) {
	se := NewStringExpander()
	se.SetVar("color", "blue")

	got, err := se.Expand("the sky is ${color}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "the sky is blue" {
		t.Errorf("got %q", got)
	}
}

func TestStringExpanderMustExpand(t *testing.T) {
	se := NewStringExpander()
	se.SetVar("x", "1")

	got := se.MustExpand("${x}")
	if got != "1" {
		t.Errorf("got %q, want %q", got, "1")
	}
}

func TestStringExpanderMustExpandPanics(t *testing.T) {
	se := NewStringExpander()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	// required() with unset variable should cause an error
	se.MustExpand("${required(UNSET_VAR_ABC)}")
}

// ---------------------------------------------------------------------
// Template.go: Expression evaluation features
// ---------------------------------------------------------------------

func TestExpandDefaultValue(t *testing.T) {
	e := NewExpander()
	got, err := e.expandString("${UNSET_VAR_XYZ:-fallback}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}

	// Set the var — should use the set value
	e.SetVar("MYVAR", "actual")
	got, err = e.expandString("${MYVAR:-fallback}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "actual" {
		t.Errorf("got %q, want actual", got)
	}
}

func TestExpandAlternateValue(t *testing.T) {
	e := NewExpander()
	// Unset var — returns empty
	got, err := e.expandString("${UNSET_VAR_ABC:+alternate}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}

	// Set var — returns alternate
	e.SetVar("SETVAR", "something")
	got, err = e.expandString("${SETVAR:+alternate}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alternate" {
		t.Errorf("got %q, want alternate", got)
	}
}

func TestExpandErrorOnUnset(t *testing.T) {
	e := NewExpander()
	_, err := e.expandString("${UNSET_VAR_ABC:?is required}")
	if err == nil {
		t.Error("expected error")
	}

	// Set var — no error
	e.SetVar("OKVAR", "present")
	got, err := e.expandString("${OKVAR:?is required}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "present" {
		t.Errorf("got %q, want present", got)
	}
}

func TestExpandSimpleDollarVar(t *testing.T) {
	e := NewExpander()
	e.SetVar("SIMPLE", "value")
	got, err := e.expandString("hello $SIMPLE world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello value world" {
		t.Errorf("got %q", got)
	}
}

func TestExpandUnknownFunction(t *testing.T) {
	e := NewExpander()
	_, err := e.expandString("${nosuchfunc(arg)}")
	if err == nil {
		t.Error("expected error for unknown function")
	}
}

// ---------------------------------------------------------------------
// Template.go: Default functions (on Expander methods)
// ---------------------------------------------------------------------

func TestExpanderFuncEnv(t *testing.T) {
	e := NewExpander()
	t.Setenv("TEST_TMPL_ENV", "envval")

	got, err := e.expandString("${env(TEST_TMPL_ENV)}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "envval" {
		t.Errorf("got %q, want envval", got)
	}

	// With default
	got, err = e.expandString("${env(UNSET_VAR_999, fallback)}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}

	// No args
	_, err = e.funcEnv()
	if err == nil {
		t.Error("expected error for no args")
	}
}

func TestExpanderFuncDefault(t *testing.T) {
	e := NewExpander()

	got, err := e.funcDefault("", "fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}

	got, err = e.funcDefault("value", "fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "value" {
		t.Errorf("got %q, want value", got)
	}

	_, err = e.funcDefault("only-one")
	if err == nil {
		t.Error("expected error for too few args")
	}
}

func TestExpanderFuncRequired(t *testing.T) {
	e := NewExpander()
	e.SetVar("exists", "val")

	got, err := e.funcRequired("exists")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "val" {
		t.Errorf("got %q", got)
	}

	// Unset
	_, err = e.funcRequired("unset_var_xyz")
	if err == nil {
		t.Error("expected error for unset required var")
	}

	// Custom message
	_, err = e.funcRequired("unset_var_xyz", "must be set")
	if err == nil {
		t.Error("expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "must be set") {
		t.Errorf("expected custom message in error, got: %v", err)
	}

	// No args
	_, err = e.funcRequired()
	if err == nil {
		t.Error("expected error for no args")
	}
}

func TestExpanderFuncLower(t *testing.T) {
	e := NewExpander()
	got, err := e.funcLower("HELLO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q", got)
	}

	got, err = e.funcLower()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExpanderFuncTrim(t *testing.T) {
	e := NewExpander()

	got, err := e.funcTrim("  hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q", got)
	}

	// Custom cutset
	got, err = e.funcTrim("xxhelloxx", "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q", got)
	}

	got, err = e.funcTrim()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExpanderFuncReplace(t *testing.T) {
	e := NewExpander()

	got, err := e.funcReplace("hello world", "world", "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello go" {
		t.Errorf("got %q", got)
	}

	_, err = e.funcReplace("a", "b")
	if err == nil {
		t.Error("expected error for too few args")
	}
}

func TestExpanderFuncSplit(t *testing.T) {
	e := NewExpander()

	got, err := e.funcSplit("a:b:c", ":", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "b" {
		t.Errorf("got %q", got)
	}

	// Out of range index
	got, err = e.funcSplit("a:b", ":", "5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}

	// Invalid index
	_, err = e.funcSplit("a:b", ":", "abc")
	if err == nil {
		t.Error("expected error")
	}

	// Too few args
	_, err = e.funcSplit("a", ":")
	if err == nil {
		t.Error("expected error")
	}
}

func TestExpanderFuncJoin(t *testing.T) {
	e := NewExpander()

	got, err := e.funcJoin(",", "a", "b", "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a,b,c" {
		t.Errorf("got %q", got)
	}

	_, err = e.funcJoin(",")
	if err == nil {
		t.Error("expected error for too few args")
	}
}

func TestExpanderFuncSubstr(t *testing.T) {
	e := NewExpander()

	// start only
	got, err := e.funcSubstr("hello", "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "llo" {
		t.Errorf("got %q", got)
	}

	// start + length
	got, err = e.funcSubstr("hello", "1", "3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ell" {
		t.Errorf("got %q", got)
	}

	// start beyond end
	got, err = e.funcSubstr("hi", "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}

	// Negative start clamped to 0
	got, err = e.funcSubstr("hello", "-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q", got)
	}

	// Length beyond string end
	got, err = e.funcSubstr("hi", "0", "100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hi" {
		t.Errorf("got %q", got)
	}

	// Invalid start
	_, err = e.funcSubstr("hello", "abc")
	if err == nil {
		t.Error("expected error")
	}

	// Invalid length
	_, err = e.funcSubstr("hello", "0", "abc")
	if err == nil {
		t.Error("expected error")
	}

	// Too few args
	_, err = e.funcSubstr("hello")
	if err == nil {
		t.Error("expected error")
	}
}

func TestExpanderFuncBase64(t *testing.T) {
	e := NewExpander()

	got, err := e.funcBase64("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "aGVsbG8=" {
		t.Errorf("got %q, want aGVsbG8=", got)
	}

	// Empty
	got, err = e.funcBase64("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}

	// No args
	got, err = e.funcBase64()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExpanderFuncQuote(t *testing.T) {
	e := NewExpander()

	got, err := e.funcQuote("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `"hello"` {
		t.Errorf("got %q", got)
	}

	got, err = e.funcQuote()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ---------------------------------------------------------------------
// Template.go: base64Encode edge cases
// ---------------------------------------------------------------------

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := base64Encode([]byte(tt.input))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Template.go: parseArgs
// ---------------------------------------------------------------------

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "hello", []string{"hello"}},
		{"two", "a, b", []string{"a", "b"}},
		{"quoted", `"hello, world"`, []string{"hello, world"}},
		{"single-quoted", `'hello, world'`, []string{"hello, world"}},
		{"nested parens", "func(a, b), c", []string{"func(a, b)", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArgs(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v (len %d), want %v (len %d)", got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------
// Sub/Mul float result formatting
// ---------------------------------------------------------------------

func TestFuncSubFloatResult(t *testing.T) {
	got, err := funcSub("1.5", "0.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty result")
	}
}

func TestFuncMulFloatResult(t *testing.T) {
	got, err := funcMul("1.5", "1.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty result")
	}
}
