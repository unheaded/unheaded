// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package testing

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden files")

// goldenTB is an interface for testing.T and testing.B
type goldenTB interface {
	Helper()
	Errorf(format string, args ...interface{})
	Logf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// GoldenOption configures golden file behavior.
type GoldenOption func(*goldenConfig)

type goldenConfig struct {
	normalize func([]byte) []byte
	suffix    string
	subdir    string
}

// WithNormalizer sets a normalizer function for the golden content.
func WithNormalizer(fn func([]byte) []byte) GoldenOption {
	return func(c *goldenConfig) {
		c.normalize = fn
	}
}

// WithSuffix sets a custom suffix for the golden file.
func WithSuffix(suffix string) GoldenOption {
	return func(c *goldenConfig) {
		c.suffix = suffix
	}
}

// WithSubdir sets a subdirectory for golden files.
func WithSubdir(subdir string) GoldenOption {
	return func(c *goldenConfig) {
		c.subdir = subdir
	}
}

// AssertGolden compares the actual content against a golden file.
// If the -update flag is set, the golden file is updated instead.
func AssertGolden(t *testing.T, goldenPath string, actual []byte, opts ...GoldenOption) bool {
	t.Helper()

	cfg := &goldenConfig{
		suffix: ".golden",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Ensure path has correct suffix
	if !strings.HasSuffix(goldenPath, cfg.suffix) {
		goldenPath += cfg.suffix
	}

	// Apply subdir if specified
	if cfg.subdir != "" {
		dir := filepath.Dir(goldenPath)
		base := filepath.Base(goldenPath)
		goldenPath = filepath.Join(dir, cfg.subdir, base)
	}

	// Normalize actual content if normalizer is set
	if cfg.normalize != nil {
		actual = cfg.normalize(actual)
	}

	// Update mode
	if *updateGolden {
		return updateGoldenFile(t, goldenPath, actual)
	}

	// Compare mode
	return compareGoldenFile(t, goldenPath, actual)
}

// AssertGoldenString is a convenience wrapper for string content.
func AssertGoldenString(t *testing.T, goldenPath string, actual string, opts ...GoldenOption) bool {
	t.Helper()
	return AssertGolden(t, goldenPath, []byte(actual), opts...)
}

// AssertGoldenJSON compares JSON content against a golden file.
// The JSON is normalized (formatted consistently) before comparison.
func AssertGoldenJSON(t *testing.T, goldenPath string, actual interface{}, opts ...GoldenOption) bool {
	t.Helper()

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Errorf("Failed to marshal actual value to JSON: %v", err)
		return false
	}

	// Add JSON normalizer
	opts = append(opts, WithNormalizer(normalizeJSON))

	return AssertGolden(t, goldenPath, data, opts...)
}

// updateGoldenFile creates or updates the golden file.
func updateGoldenFile(t goldenTB, path string, content []byte) bool {
	t.Helper()

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Errorf("Failed to create golden file directory: %v", err)
		return false
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Errorf("Failed to write golden file: %v", err)
		return false
	}

	t.Logf("Updated golden file: %s", path)
	return true
}

// compareGoldenFile compares the actual content against the golden file.
func compareGoldenFile(t goldenTB, path string, actual []byte) bool {
	t.Helper()

	expected, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Errorf("Golden file does not exist: %s\nRun with -update to create it", path)
		} else {
			t.Errorf("Failed to read golden file: %v", err)
		}
		return false
	}

	if !bytes.Equal(expected, actual) {
		diff := generateDiff(expected, actual)
		t.Errorf("Golden file mismatch: %s\n%s\nRun with -update to update the golden file", path, diff)
		return false
	}

	return true
}

// generateDiff generates a simple diff between expected and actual content.
func generateDiff(expected, actual []byte) string {
	expectedLines := bytes.Split(expected, []byte("\n"))
	actualLines := bytes.Split(actual, []byte("\n"))

	var sb bytes.Buffer
	sb.WriteString("--- expected\n+++ actual\n")

	maxLines := len(expectedLines)
	if len(actualLines) > maxLines {
		maxLines = len(actualLines)
	}

	for i := 0; i < maxLines; i++ {
		var expLine, actLine []byte
		if i < len(expectedLines) {
			expLine = expectedLines[i]
		}
		if i < len(actualLines) {
			actLine = actualLines[i]
		}

		if !bytes.Equal(expLine, actLine) {
			if i < len(expectedLines) {
				sb.WriteString("- ")
				sb.Write(expLine)
				sb.WriteString("\n")
			}
			if i < len(actualLines) {
				sb.WriteString("+ ")
				sb.Write(actLine)
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// normalizeJSON normalizes JSON content for consistent comparison.
func normalizeJSON(data []byte) []byte {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return data
	}

	normalized, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return data
	}

	return normalized
}

// NormalizeWhitespace normalizes whitespace in content.
func NormalizeWhitespace(data []byte) []byte {
	// Replace all whitespace sequences with single space
	var result bytes.Buffer
	inWhitespace := false

	for _, b := range data {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			if !inWhitespace {
				result.WriteByte(' ')
				inWhitespace = true
			}
		} else {
			result.WriteByte(b)
			inWhitespace = false
		}
	}

	return bytes.TrimSpace(result.Bytes())
}

// NormalizeLineEndings normalizes line endings to LF.
func NormalizeLineEndings(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// GoldenDir manages golden files in a directory.
type GoldenDir struct {
	t       *testing.T
	dir     string
	opts    []GoldenOption
	cleanup bool
}

// NewGoldenDir creates a new golden directory manager.
func NewGoldenDir(t *testing.T, dir string) *GoldenDir {
	return &GoldenDir{
		t:   t,
		dir: dir,
	}
}

// WithOptions sets default options for all assertions.
func (g *GoldenDir) WithOptions(opts ...GoldenOption) *GoldenDir {
	g.opts = opts
	return g
}

// Assert compares content against a golden file in the directory.
func (g *GoldenDir) Assert(name string, actual []byte, opts ...GoldenOption) bool {
	g.t.Helper()
	path := filepath.Join(g.dir, name)
	allOpts := append(g.opts, opts...)
	return AssertGolden(g.t, path, actual, allOpts...)
}

// AssertString compares string content against a golden file.
func (g *GoldenDir) AssertString(name string, actual string, opts ...GoldenOption) bool {
	g.t.Helper()
	return g.Assert(name, []byte(actual), opts...)
}

// AssertJSON compares JSON content against a golden file.
func (g *GoldenDir) AssertJSON(name string, actual interface{}, opts ...GoldenOption) bool {
	g.t.Helper()
	path := filepath.Join(g.dir, name)
	allOpts := append(g.opts, opts...)
	return AssertGoldenJSON(g.t, path, actual, allOpts...)
}

// GoldenSnapshot provides snapshot testing functionality.
type GoldenSnapshot struct {
	t        *testing.T
	name     string
	sections map[string][]byte
}

// NewGoldenSnapshot creates a new snapshot.
func NewGoldenSnapshot(t *testing.T, name string) *GoldenSnapshot {
	return &GoldenSnapshot{
		t:        t,
		name:     name,
		sections: make(map[string][]byte),
	}
}

// Add adds a section to the snapshot.
func (s *GoldenSnapshot) Add(section string, content []byte) *GoldenSnapshot {
	s.sections[section] = content
	return s
}

// AddString adds a string section to the snapshot.
func (s *GoldenSnapshot) AddString(section, content string) *GoldenSnapshot {
	return s.Add(section, []byte(content))
}

// AddJSON adds a JSON section to the snapshot.
func (s *GoldenSnapshot) AddJSON(section string, v interface{}) *GoldenSnapshot {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		s.t.Errorf("Failed to marshal JSON for section %s: %v", section, err)
		return s
	}
	return s.Add(section, data)
}

// Assert compares the snapshot against the golden file.
func (s *GoldenSnapshot) Assert(goldenPath string, opts ...GoldenOption) bool {
	s.t.Helper()

	// Collect and sort section names for deterministic output
	sectionNames := make([]string, 0, len(s.sections))
	for name := range s.sections {
		sectionNames = append(sectionNames, name)
	}
	sort.Strings(sectionNames)

	// Build snapshot content in sorted order
	var buf bytes.Buffer
	for _, section := range sectionNames {
		content := s.sections[section]
		buf.WriteString("=== " + section + " ===\n")
		buf.Write(content)
		buf.WriteString("\n\n")
	}

	return AssertGolden(s.t, goldenPath, buf.Bytes(), opts...)
}

// GoldenTestCase represents a test case loaded from golden files.
type GoldenTestCase struct {
	Name     string
	Input    []byte
	Expected []byte
}

// LoadGoldenTestCases loads test cases from a directory structure.
// Expected structure: dir/<name>/input, dir/<name>/expected
func LoadGoldenTestCases(t *testing.T, dir string) []GoldenTestCase {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read test cases directory: %v", err)
	}

	var cases []GoldenTestCase
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		caseDir := filepath.Join(dir, name)

		input, err := os.ReadFile(filepath.Join(caseDir, "input"))
		if err != nil {
			t.Errorf("Failed to read input for test case %s: %v", name, err)
			continue
		}

		expected, err := os.ReadFile(filepath.Join(caseDir, "expected"))
		if err != nil {
			t.Errorf("Failed to read expected output for test case %s: %v", name, err)
			continue
		}

		cases = append(cases, GoldenTestCase{
			Name:     name,
			Input:    input,
			Expected: expected,
		})
	}

	return cases
}

// RunGoldenTestCases runs test cases loaded from golden files.
func RunGoldenTestCases(t *testing.T, dir string, fn func(input []byte) []byte) {
	t.Helper()

	cases := LoadGoldenTestCases(t, dir)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			actual := fn(tc.Input)

			if *updateGolden {
				expectedPath := filepath.Join(dir, tc.Name, "expected")
				if err := os.WriteFile(expectedPath, actual, 0644); err != nil {
					t.Errorf("Failed to update expected file: %v", err)
				}
				return
			}

			if !bytes.Equal(tc.Expected, actual) {
				diff := generateDiff(tc.Expected, actual)
				t.Errorf("Output mismatch:\n%s", diff)
			}
		})
	}
}
