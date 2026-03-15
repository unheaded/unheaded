// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package parser

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// PARSER TESTS - THE ORACLE'S TRIALS
// ============================================================================

func TestNewMarkdownParser_EmptyPath(t *testing.T) {
	_, err := NewMarkdownParser("")
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

func TestNewMarkdownParser_NonexistentFile(t *testing.T) {
	_, err := NewMarkdownParser("/nonexistent/path/timeline.md")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestMarkdownParser_ParseContent_SimpleTimeline(t *testing.T) {
	content := `# The Unheaded Chronicles

**STATUS:** Alpha Development

### Age 1: The Alpha Ascension (IN PROGRESS)

The Kingdom rises.

#### Epoch 1.1: The Whispering Void Awakens
**eBPF - The Silent Observers**
*ETA: Feb 3-4, 2026 | STATUS: Dormant*

**Risk:** Medium
**Owner:** Agent 5

- [ ] packet_marker.bpf
- [ ] flow_tracker.bpf
- [x] Initial research

#### Epoch 1.2: The Cuirass Takes Form
**ETA:** Feb 5, 2026
**Risk:** Low

- [x] Design complete
- [ ] Implementation pending
`

	parser := &MarkdownParser{}
	tl, err := parser.ParseContent(content)
	if err != nil {
		t.Fatalf("ParseContent() error: %v", err)
	}

	// Verify timeline basics
	if tl.Status != "alpha" {
		t.Errorf("expected status 'alpha', got %q", tl.Status)
	}

	// Verify phases
	if len(tl.Phases) != 1 {
		t.Errorf("expected 1 phase, got %d", len(tl.Phases))
	}

	if tl.Phases[0].Name != "The Alpha Ascension" {
		t.Errorf("expected phase name 'The Alpha Ascension', got %q", tl.Phases[0].Name)
	}

	// Verify milestones
	if len(tl.Milestones) != 2 {
		t.Errorf("expected 2 milestones, got %d", len(tl.Milestones))
	}

	// Check first milestone
	m1 := tl.Milestones[0]
	if m1.Name != "The Whispering Void Awakens" {
		t.Errorf("expected milestone name 'The Whispering Void Awakens', got %q", m1.Name)
	}
	if m1.Risk != "medium" {
		t.Errorf("expected risk 'medium', got %q", m1.Risk)
	}
	if m1.Owner != "Agent 5" {
		t.Errorf("expected owner 'Agent 5', got %q", m1.Owner)
	}
	if len(m1.Tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(m1.Tasks))
	}
}

func TestMarkdownParser_ParseContent_StatusExtraction(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			"complete",
			"### Phase 1: Test (COMPLETE ✓)\n",
			"completed",
		},
		{
			"in_progress",
			"### Phase 1: Test (IN PROGRESS 🚀)\n",
			"in_progress",
		},
		{
			"planned",
			"### Phase 1: Test (PLANNED)\n",
			"planned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &MarkdownParser{}
			tl, err := parser.ParseContent(tt.content)
			if err != nil {
				t.Fatalf("ParseContent() error: %v", err)
			}

			if len(tl.Phases) == 0 {
				t.Fatal("expected at least 1 phase")
			}

			if tl.Phases[0].Status != tt.expected {
				t.Errorf("expected status %q, got %q", tt.expected, tl.Phases[0].Status)
			}
		})
	}
}

func TestMarkdownParser_ParseContent_ProgressExtraction(t *testing.T) {
	content := `### Phase 1: Test

**STATUS:** 75% FORGED

#### Milestone 1.1: Sub Test
25% complete
`

	parser := &MarkdownParser{}
	tl, err := parser.ParseContent(content)
	if err != nil {
		t.Fatalf("ParseContent() error: %v", err)
	}

	if len(tl.Phases) == 0 {
		t.Fatal("expected at least 1 phase")
	}

	// Phase should have 75% progress
	if tl.Phases[0].Progress != 75 {
		t.Errorf("expected phase progress 75, got %d", tl.Phases[0].Progress)
	}

	if len(tl.Milestones) == 0 {
		t.Fatal("expected at least 1 milestone")
	}

	// Milestone should have 25% progress
	if tl.Milestones[0].Progress != 25 {
		t.Errorf("expected milestone progress 25, got %d", tl.Milestones[0].Progress)
	}
}

func TestMarkdownParser_ParseContent_CheckboxParsing(t *testing.T) {
	content := `#### Epoch 1.1: Test Milestone

- [x] Completed task 1
- [x] Completed task 2
- [ ] Pending task 1
- [ ] Pending task 2
- [ ] Pending task 3
`

	parser := &MarkdownParser{}
	tl, err := parser.ParseContent(content)
	if err != nil {
		t.Fatalf("ParseContent() error: %v", err)
	}

	if len(tl.Milestones) == 0 {
		t.Fatal("expected at least 1 milestone")
	}

	m := tl.Milestones[0]
	if len(m.Tasks) != 5 {
		t.Errorf("expected 5 tasks, got %d", len(m.Tasks))
	}

	// Check task names are extracted
	expectedTasks := []string{
		"Completed task 1",
		"Completed task 2",
		"Pending task 1",
		"Pending task 2",
		"Pending task 3",
	}

	for i, expected := range expectedTasks {
		if i >= len(m.Tasks) {
			t.Errorf("missing task %d: %q", i, expected)
			continue
		}
		if m.Tasks[i] != expected {
			t.Errorf("task %d: expected %q, got %q", i, expected, m.Tasks[i])
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
	}{
		{
			"standard format",
			"Feb 3, 2026",
			time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			"full month",
			"February 8, 2026",
			time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC),
		},
		{
			"range format",
			"Feb 3-4, 2026",
			time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			"ISO format",
			"2026-02-15",
			time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDate(tt.input)
			if !result.Equal(tt.expected) {
				t.Errorf("parseDate(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"COMPLETE", "completed"},
		{"COMPLETED", "completed"},
		{"IN PROGRESS", "in_progress"},
		{"IN_PROGRESS", "in_progress"},
		{"BLOCKED", "blocked"},
		{"PLANNED", "planned"},
		{"unknown", "planned"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseStatus(tt.input)
			if result != tt.expected {
				t.Errorf("parseStatus(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"(COMPLETE ✓)", "completed"},
		{"Status: COMPLETE", "completed"},
		{"(IN PROGRESS 🚀)", "in_progress"},
		{"currently in_progress", "in_progress"},
		{"(BLOCKED)", "blocked"},
		{"(PLANNED)", "planned"},
		{"no status here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractStatus(tt.input)
			if result != tt.expected {
				t.Errorf("extractStatus(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// INTEGRATION TEST - Full Timeline Parsing
// ============================================================================

func TestMarkdownParser_ParseContent_FullTimeline(t *testing.T) {
	// This simulates the actual timeline.md structure
	content := `# The Unheaded Chronicles

## A Living Grimoire of the Kingdom's Journey

**STATUS:** The Kingdom Rises
**LAST SCRIBED:** January 28, 2026

---

### Age 0: The Foundation Stone (COMPLETE ✓)
**The Forging of Wotan**
*December 2025 - January 2026*

- [x] Ring buffer implementation
- [x] Pub/sub engine
- [x] gRPC streaming

### Age 1: The Alpha Ascension (IN PROGRESS 🚀)
**The Armor Takes Shape**
*January 26 - February 8, 2026*

**STATUS:** 25% FORGED

#### Epoch 1.1: The Whispering Void Awakens
**eBPF - The Silent Observers**
*ETA: Feb 3-4, 2026 | STATUS: Dormant*

**Risk:** Medium
**Owner:** The Architect

- [ ] packet_marker.bpf
- [ ] flow_tracker.bpf
- [ ] latency_probe.bpf

#### Epoch 1.2: The Cuirass Takes Form
**Control Plane - The Core Heart**
*ETA: Feb 4-5, 2026*

**Risk:** Medium
**Owner:** Agent 7

- [ ] unheaded-daemon skeleton
- [ ] LXD orchestration client
- [x] Design document

### Age 2: The Beta Trials (PLANNED)
**Production Hardening**
*February 16 - March 31, 2026*

- [ ] Comprehensive test coverage
- [ ] Message persistence
`

	parser := &MarkdownParser{}
	tl, err := parser.ParseContent(content)
	if err != nil {
		t.Fatalf("ParseContent() error: %v", err)
	}

	// Verify phases
	if len(tl.Phases) != 3 {
		t.Errorf("expected 3 phases, got %d", len(tl.Phases))
	}

	// Check phase names
	expectedPhases := []string{"The Foundation Stone", "The Alpha Ascension", "The Beta Trials"}
	for i, name := range expectedPhases {
		if i >= len(tl.Phases) {
			t.Errorf("missing phase %d", i)
			continue
		}
		if tl.Phases[i].Name != name {
			t.Errorf("phase %d: expected name %q, got %q", i, name, tl.Phases[i].Name)
		}
	}

	// Check phase statuses
	expectedStatuses := []string{"completed", "in_progress", "planned"}
	for i, status := range expectedStatuses {
		if i >= len(tl.Phases) {
			continue
		}
		if tl.Phases[i].Status != status {
			t.Errorf("phase %d: expected status %q, got %q", i, status, tl.Phases[i].Status)
		}
	}

	// Verify milestones
	if len(tl.Milestones) != 2 {
		t.Errorf("expected 2 milestones, got %d", len(tl.Milestones))
	}

	// Check first milestone details
	if len(tl.Milestones) > 0 {
		m1 := tl.Milestones[0]
		if m1.Risk != "medium" {
			t.Errorf("milestone 1: expected risk 'medium', got %q", m1.Risk)
		}
		if !strings.Contains(m1.Owner, "Architect") {
			t.Errorf("milestone 1: expected owner containing 'Architect', got %q", m1.Owner)
		}
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

func BenchmarkParseContent(b *testing.B) {
	content := `# Timeline

### Phase 1: Alpha (IN PROGRESS)

#### Milestone 1.1: Task A
- [x] Task 1
- [ ] Task 2
- [ ] Task 3

#### Milestone 1.2: Task B
- [x] Task 1
- [x] Task 2
`

	parser := &MarkdownParser{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.ParseContent(content)
	}
}
