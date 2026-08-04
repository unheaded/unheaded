// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/services/timeguru/internal/timeline"
)

func testTimeline() *timeline.Timeline {
	return &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
		Status:      "alpha",
		Vision:      "Configuration management automation platform.",
		Phases: []*timeline.Phase{
			{
				ID:         "phase-0",
				Name:       "The Foundation Stone",
				Status:     "completed",
				Progress:   100,
				Milestones: []string{"milestone-0.1"},
			},
			{
				ID:         "phase-1",
				Name:       "Alpha Ascension",
				Status:     "in_progress",
				Progress:   60,
				Milestones: []string{"milestone-1.1", "milestone-1.2"},
			},
		},
		Milestones: []*timeline.Milestone{
			{
				ID:       "milestone-0.1",
				Name:     "Repository Bootstrap",
				Status:   "completed",
				Progress: 100,
			},
			{
				ID:          "milestone-1.1",
				Name:        "eBPF Foundation",
				Status:      "in_progress",
				ETA:         tp(time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)),
				Progress:    25,
				Owner:       "Agent 5",
				Risk:        "medium",
				Description: "eBPF programs for kernel tracing",
				Tasks:       []string{"packet_marker.bpf", "flow_tracker.bpf"},
			},
			{
				ID:       "milestone-1.2",
				Name:     "Docker Compose",
				Status:   "completed",
				Progress: 100,
			},
		},
		Metadata: map[string]string{
			"overall_progress": "75%",
		},
	}
}

// ============================================================================
// ENCODE TESTS
// ============================================================================

func TestEncodeJSON(t *testing.T) {
	tl := testTimeline()
	data, err := Encode(tl, FormatJSON)
	if err != nil {
		t.Fatalf("Encode JSON: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty JSON output")
	}

	// Round-trip
	decoded, err := Decode(data, FormatJSON)
	if err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}
	if decoded.Version != tl.Version {
		t.Errorf("version mismatch: got %q, want %q", decoded.Version, tl.Version)
	}
	if len(decoded.Phases) != len(tl.Phases) {
		t.Errorf("phases count mismatch: got %d, want %d", len(decoded.Phases), len(tl.Phases))
	}
	if len(decoded.Milestones) != len(tl.Milestones) {
		t.Errorf("milestones count mismatch: got %d, want %d", len(decoded.Milestones), len(tl.Milestones))
	}
}

func TestEncodeTOML(t *testing.T) {
	tl := testTimeline()
	data, err := Encode(tl, FormatTOML)
	if err != nil {
		t.Fatalf("Encode TOML: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty TOML output")
	}

	// Round-trip
	decoded, err := Decode(data, FormatTOML)
	if err != nil {
		t.Fatalf("Decode TOML: %v", err)
	}
	if decoded.Version != tl.Version {
		t.Errorf("version mismatch: got %q, want %q", decoded.Version, tl.Version)
	}
	if decoded.Status != tl.Status {
		t.Errorf("status mismatch: got %q, want %q", decoded.Status, tl.Status)
	}
	if len(decoded.Milestones) != len(tl.Milestones) {
		t.Errorf("milestones count mismatch: got %d, want %d", len(decoded.Milestones), len(tl.Milestones))
	}
	if decoded.Milestones[1].Owner != "Agent 5" {
		t.Errorf("owner mismatch: got %q, want %q", decoded.Milestones[1].Owner, "Agent 5")
	}
}

func TestEncodeYAML(t *testing.T) {
	tl := testTimeline()
	data, err := Encode(tl, FormatYAML)
	if err != nil {
		t.Fatalf("Encode YAML: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty YAML output")
	}

	// Round-trip
	decoded, err := Decode(data, FormatYAML)
	if err != nil {
		t.Fatalf("Decode YAML: %v", err)
	}
	if decoded.Vision != tl.Vision {
		t.Errorf("vision mismatch: got %q, want %q", decoded.Vision, tl.Vision)
	}
	if decoded.Milestones[1].Risk != "medium" {
		t.Errorf("risk mismatch: got %q, want %q", decoded.Milestones[1].Risk, "medium")
	}
}

func TestEncodeMarkdown(t *testing.T) {
	tl := testTimeline()
	data, err := Encode(tl, FormatMarkdown)
	if err != nil {
		t.Fatalf("Encode Markdown: %v", err)
	}

	md := string(data)
	checks := []string{
		"# The Unheaded Chronicles",
		"**STATUS:** ALPHA",
		"The Foundation Stone",
		"Alpha Ascension",
		"eBPF Foundation",
		"**Owner:** Agent 5",
		"**Risk:** Medium",
		"packet_marker.bpf",
	}
	for _, check := range checks {
		if !contains(md, check) {
			t.Errorf("markdown missing %q", check)
		}
	}
}

func TestEncodeNil(t *testing.T) {
	_, err := Encode(nil, FormatJSON)
	if err != ErrNilTimeline {
		t.Errorf("expected ErrNilTimeline, got %v", err)
	}
}

func TestEncodeUnsupportedFormat(t *testing.T) {
	tl := testTimeline()
	_, err := Encode(tl, Format("xml"))
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

// ============================================================================
// DECODE TESTS
// ============================================================================

func TestDecodeJSON_Invalid(t *testing.T) {
	_, err := Decode([]byte("{bad json}"), FormatJSON)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecodeTOML_Invalid(t *testing.T) {
	_, err := Decode([]byte("[[[ not toml"), FormatTOML)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestDecodeYAML_Invalid(t *testing.T) {
	_, err := Decode([]byte(":\n  :\n    : [invalid"), FormatYAML)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestDecodeMarkdown_Unsupported(t *testing.T) {
	_, err := Decode([]byte("# hello"), FormatMarkdown)
	if err == nil {
		t.Fatal("expected error for markdown decode")
	}
}

func TestDecodeJSON_ValidationFailure(t *testing.T) {
	// Valid JSON but missing required version field
	data := []byte(`{"status":"alpha","phases":[],"milestones":[]}`)
	_, err := Decode(data, FormatJSON)
	if err == nil {
		t.Fatal("expected validation error for missing version")
	}
}

// ============================================================================
// DETECT FORMAT
// ============================================================================

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		filename string
		want     Format
		wantErr  bool
	}{
		{"timeline.json", FormatJSON, false},
		{"timeline.toml", FormatTOML, false},
		{"timeline.yaml", FormatYAML, false},
		{"timeline.yml", FormatYAML, false},
		{"timeline.md", FormatMarkdown, false},
		{"timeline.markdown", FormatMarkdown, false},
		{"timeline.xml", "", true},
		{"noext", "", true},
	}
	for _, tt := range tests {
		got, err := DetectFormat(tt.filename)
		if tt.wantErr {
			if err == nil {
				t.Errorf("DetectFormat(%q): expected error", tt.filename)
			}
			continue
		}
		if err != nil {
			t.Errorf("DetectFormat(%q): unexpected error: %v", tt.filename, err)
			continue
		}
		if got != tt.want {
			t.Errorf("DetectFormat(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

// ============================================================================
// SYNCER TESTS
// ============================================================================

// TestSyncerDefaultFormats — default Sync writes JSON/TOML/YAML but
// NOT Markdown. Markdown is the source of truth (per CLAUDE.md triple-
// mirror strategy and kanban: debt-timeline-sync 2026-05-04); the
// encoder would otherwise overwrite Stevie's hand-edited timeline.md
// with stubs when the in-memory Timeline is incomplete (e.g. empty
// Milestones).
//
// Callers who genuinely need Markdown output explicitly pass
// FormatMarkdown to NewSyncer — the encoder is still functional, just
// not on the default path.
func TestSyncerDefaultFormats(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSyncer(dir)
	if err != nil {
		t.Fatalf("NewSyncer: %v", err)
	}

	tl := testTimeline()
	result := s.Sync(tl)

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			t.Errorf("sync error: %v", e)
		}
	}

	if len(result.FilesWritten) != 3 {
		t.Fatalf("expected 3 files written (JSON/TOML/YAML; Markdown excluded by default), got %d", len(result.FilesWritten))
	}

	// Regression guard: timeline.md MUST NOT exist after a default Sync.
	mdPath := filepath.Join(dir, "timeline.md")
	if _, err := os.Stat(mdPath); err == nil {
		t.Fatalf("default Sync wrote timeline.md — debt-timeline-sync regression. Markdown is source-of-truth, not a derived output.")
	}

	// Verify files exist and have content
	expected := []string{"timeline.json", "timeline.toml", "timeline.yaml"}
	for _, name := range expected {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("file %s not found: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("file %s is empty", name)
		}
	}

	// Verify JSON round-trip from file
	jsonData, _ := os.ReadFile(filepath.Join(dir, "timeline.json"))
	decoded, err := Decode(jsonData, FormatJSON)
	if err != nil {
		t.Fatalf("decode synced JSON: %v", err)
	}
	if decoded.Version != "1.0.0" {
		t.Errorf("version from synced file: got %q, want %q", decoded.Version, "1.0.0")
	}

	// Verify TOML round-trip from file
	tomlData, _ := os.ReadFile(filepath.Join(dir, "timeline.toml"))
	decoded, err = Decode(tomlData, FormatTOML)
	if err != nil {
		t.Fatalf("decode synced TOML: %v", err)
	}
	if len(decoded.Milestones) != 3 {
		t.Errorf("milestones from synced TOML: got %d, want 3", len(decoded.Milestones))
	}
}

func TestSyncerSelectiveFormats(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSyncer(dir, FormatJSON, FormatTOML)
	if err != nil {
		t.Fatalf("NewSyncer: %v", err)
	}

	result := s.Sync(testTimeline())
	if len(result.FilesWritten) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result.FilesWritten))
	}

	// YAML and MD should NOT exist
	if _, err := os.Stat(filepath.Join(dir, "timeline.yaml")); err == nil {
		t.Error("timeline.yaml should not exist with selective formats")
	}
}

func TestSyncerNilTimeline(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSyncer(dir)
	if err != nil {
		t.Fatalf("NewSyncer: %v", err)
	}

	result := s.Sync(nil)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for nil timeline")
	}
}

func TestSyncerEmptyDir(t *testing.T) {
	_, err := NewSyncer("")
	if err != ErrEmptyDirectory {
		t.Errorf("expected ErrEmptyDirectory, got %v", err)
	}
}

// ============================================================================
// CROSS-FORMAT CONSISTENCY
// ============================================================================

func TestCrossFormatConsistency(t *testing.T) {
	tl := testTimeline()

	// Encode to all structured formats
	jsonData, err := Encode(tl, FormatJSON)
	if err != nil {
		t.Fatalf("JSON encode: %v", err)
	}
	tomlData, err := Encode(tl, FormatTOML)
	if err != nil {
		t.Fatalf("TOML encode: %v", err)
	}
	yamlData, err := Encode(tl, FormatYAML)
	if err != nil {
		t.Fatalf("YAML encode: %v", err)
	}

	// Decode all
	fromJSON, _ := Decode(jsonData, FormatJSON)
	fromTOML, _ := Decode(tomlData, FormatTOML)
	fromYAML, _ := Decode(yamlData, FormatYAML)

	// All should agree on key fields
	sources := map[string]*timeline.Timeline{
		"JSON": fromJSON,
		"TOML": fromTOML,
		"YAML": fromYAML,
	}

	for name, src := range sources {
		if src.Version != "1.0.0" {
			t.Errorf("%s: version = %q, want %q", name, src.Version, "1.0.0")
		}
		if src.Status != "alpha" {
			t.Errorf("%s: status = %q, want %q", name, src.Status, "alpha")
		}
		if len(src.Phases) != 2 {
			t.Errorf("%s: phases = %d, want 2", name, len(src.Phases))
		}
		if len(src.Milestones) != 3 {
			t.Errorf("%s: milestones = %d, want 3", name, len(src.Milestones))
		}
		if src.Milestones[1].Owner != "Agent 5" {
			t.Errorf("%s: milestone[1].owner = %q, want %q", name, src.Milestones[1].Owner, "Agent 5")
		}
		if src.Milestones[1].Progress != 25 {
			t.Errorf("%s: milestone[1].progress = %d, want 25", name, src.Milestones[1].Progress)
		}
		if len(src.Milestones[1].Tasks) != 2 {
			t.Errorf("%s: milestone[1].tasks = %d, want 2", name, len(src.Milestones[1].Tasks))
		}
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// tp returns a pointer to t. The date fields are *time.Time so that a zero
// value can be omitted from JSON/YAML — `omitempty` cannot omit a struct.
func tp(t time.Time) *time.Time { return &t }
