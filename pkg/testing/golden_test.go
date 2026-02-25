// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package testing

import (
	"os"
	"path/filepath"
	"testing"
)

// testHelperGolden is used to capture assertion failures without actually failing the test
type testHelperGolden struct {
	testing.TB
	failed bool
}

func newGoldenTestHelper(t *testing.T) *testHelperGolden {
	return &testHelperGolden{TB: t}
}

func (h *testHelperGolden) Helper() {}

func (h *testHelperGolden) Error(args ...interface{}) {
	h.failed = true
}

func (h *testHelperGolden) Errorf(format string, args ...interface{}) {
	h.failed = true
}

func (h *testHelperGolden) Fatal(args ...interface{}) {
	h.failed = true
}

func (h *testHelperGolden) Fatalf(format string, args ...interface{}) {
	h.failed = true
}

func (h *testHelperGolden) Logf(format string, args ...interface{}) {
	// No-op for tests
}

func TestGoldenBasic(t *testing.T) {
	// Create a temp golden file
	tempDir := t.TempDir()
	goldenPath := filepath.Join(tempDir, "test.golden")

	content := []byte("Hello, World!")

	// First, create the golden file by simulating update mode
	if err := os.WriteFile(goldenPath, content, 0644); err != nil {
		t.Fatalf("Failed to create golden file: %v", err)
	}

	// Now test comparison
	if !compareGoldenFile(t, goldenPath, content) {
		t.Error("Expected golden comparison to pass")
	}
}

func TestGoldenMismatch(t *testing.T) {
	tempDir := t.TempDir()
	goldenPath := filepath.Join(tempDir, "test.golden")

	// Create golden file with expected content
	if err := os.WriteFile(goldenPath, []byte("expected"), 0644); err != nil {
		t.Fatalf("Failed to create golden file: %v", err)
	}

	// Create a test helper to capture the failure
	th := newGoldenTestHelper(t)

	// This should fail because content doesn't match
	result := compareGoldenFile(th, goldenPath, []byte("actual"))
	if result {
		t.Error("Expected golden comparison to fail for mismatched content")
	}
}

func TestGoldenUpdate(t *testing.T) {
	tempDir := t.TempDir()
	goldenPath := filepath.Join(tempDir, "test.golden")

	content := []byte("New content")

	// Update should create the file
	if !updateGoldenFile(t, goldenPath, content) {
		t.Error("Expected update to succeed")
	}

	// Verify content was written
	read, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Failed to read golden file: %v", err)
	}

	if string(read) != string(content) {
		t.Errorf("Expected content %q, got %q", content, read)
	}
}

func TestGoldenDir(t *testing.T) {
	tempDir := t.TempDir()

	// Create golden files
	if err := os.WriteFile(filepath.Join(tempDir, "test1.golden"), []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to create golden file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "test2.golden"), []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to create golden file: %v", err)
	}

	golden := NewGoldenDir(t, tempDir)

	if !golden.Assert("test1", []byte("content1")) {
		t.Error("Expected test1 assertion to pass")
	}

	if !golden.AssertString("test2", "content2") {
		t.Error("Expected test2 assertion to pass")
	}
}

func TestGoldenSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	goldenPath := filepath.Join(tempDir, "snapshot.golden")

	// Create expected snapshot content (sections are sorted alphabetically)
	expectedContent := "=== body ===\nBody content\n\n=== header ===\nHeader content\n\n"
	if err := os.WriteFile(goldenPath, []byte(expectedContent), 0644); err != nil {
		t.Fatalf("Failed to create golden file: %v", err)
	}

	snapshot := NewGoldenSnapshot(t, "test")
	snapshot.AddString("header", "Header content")
	snapshot.AddString("body", "Body content")

	if !snapshot.Assert(goldenPath) {
		t.Error("Expected snapshot assertion to pass")
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	input := []byte("hello   world\n\ttab")
	expected := []byte("hello world tab")

	result := NormalizeWhitespace(input)
	if string(result) != string(expected) {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	input := []byte("line1\r\nline2\r\nline3")
	expected := []byte("line1\nline2\nline3")

	result := NormalizeLineEndings(input)
	if string(result) != string(expected) {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestNormalizeJSON(t *testing.T) {
	input := []byte(`{"b":2,"a":1}`)

	result := normalizeJSON(input)

	// Should be formatted with indentation
	expected := `{
  "a": 1,
  "b": 2
}`
	if string(result) != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result)
	}
}

func TestGenerateDiff(t *testing.T) {
	expected := []byte("line1\nline2\nline3")
	actual := []byte("line1\nmodified\nline3")

	diff := generateDiff(expected, actual)

	if diff == "" {
		t.Error("Expected diff to be non-empty")
	}

	// Should contain markers for changed lines
	if !containsString(diff, "- line2") && !containsString(diff, "-line2") {
		t.Error("Expected diff to show removed line")
	}
	if !containsString(diff, "+ modified") && !containsString(diff, "+modified") {
		t.Error("Expected diff to show added line")
	}
}

func TestWithNormalizer(t *testing.T) {
	cfg := &goldenConfig{}
	opt := WithNormalizer(func(data []byte) []byte {
		return []byte("normalized")
	})
	opt(cfg)

	if cfg.normalize == nil {
		t.Error("Expected normalizer to be set")
	}

	result := cfg.normalize([]byte("input"))
	if string(result) != "normalized" {
		t.Errorf("Expected 'normalized', got %q", result)
	}
}

func TestWithSuffix(t *testing.T) {
	cfg := &goldenConfig{}
	opt := WithSuffix(".expected")
	opt(cfg)

	if cfg.suffix != ".expected" {
		t.Errorf("Expected suffix '.expected', got %q", cfg.suffix)
	}
}

func TestWithSubdir(t *testing.T) {
	cfg := &goldenConfig{}
	opt := WithSubdir("snapshots")
	opt(cfg)

	if cfg.subdir != "snapshots" {
		t.Errorf("Expected subdir 'snapshots', got %q", cfg.subdir)
	}
}

func TestLoadGoldenTestCases(t *testing.T) {
	// Create temp directory structure
	tempDir := t.TempDir()

	// Create test case directories
	case1Dir := filepath.Join(tempDir, "case1")
	case2Dir := filepath.Join(tempDir, "case2")

	if err := os.MkdirAll(case1Dir, 0755); err != nil {
		t.Fatalf("Failed to create case1 dir: %v", err)
	}
	if err := os.MkdirAll(case2Dir, 0755); err != nil {
		t.Fatalf("Failed to create case2 dir: %v", err)
	}

	// Create input and expected files
	if err := os.WriteFile(filepath.Join(case1Dir, "input"), []byte("input1"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(case1Dir, "expected"), []byte("expected1"), 0644); err != nil {
		t.Fatalf("Failed to create expected file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(case2Dir, "input"), []byte("input2"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(case2Dir, "expected"), []byte("expected2"), 0644); err != nil {
		t.Fatalf("Failed to create expected file: %v", err)
	}

	cases := LoadGoldenTestCases(t, tempDir)

	if len(cases) != 2 {
		t.Errorf("Expected 2 test cases, got %d", len(cases))
	}

	// Find case1
	var found bool
	for _, tc := range cases {
		if tc.Name == "case1" {
			found = true
			if string(tc.Input) != "input1" {
				t.Errorf("Expected input1, got %s", tc.Input)
			}
			if string(tc.Expected) != "expected1" {
				t.Errorf("Expected expected1, got %s", tc.Expected)
			}
		}
	}
	if !found {
		t.Error("Expected to find case1")
	}
}
