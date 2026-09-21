// SPDX-License-Identifier: GPL-3.0-or-later
package parser

import "testing"

// B9 review finding 2: a declared "Progress: N%" on a milestone must not be
// overwritten by an incidental percentage later in the same section. The
// phase-level guard existed; the milestone-level one did not.
func TestMarkdownParser_MilestoneDeclaredProgressWins(t *testing.T) {
	content := `### Phase 1: Test

#### Epoch 1.1: Guarded
**Progress:** 25%
coverage sits near 91% and climbing

#### Epoch 1.2: Undeclared
coverage sits near 60%
`
	tl, err := (&MarkdownParser{}).ParseContent(content)
	if err != nil {
		t.Fatalf("ParseContent() error: %v", err)
	}
	if len(tl.Milestones) != 2 {
		t.Fatalf("expected 2 milestones, got %d", len(tl.Milestones))
	}
	if got := tl.Milestones[0].Progress; got != 25 {
		t.Errorf("declared milestone progress must win: want 25, got %d", got)
	}
	// The flag must reset per milestone, or an undeclared one inherits the guard.
	if got := tl.Milestones[1].Progress; got != 60 {
		t.Errorf("undeclared milestone must still pick up an incidental %%: want 60, got %d", got)
	}
}

// B9 review finding 4: the status keyword in a header parenthetical is
// matched on word boundaries. "(incomplete)" is prose, not a COMPLETE marker,
// and must neither flip the status nor be stripped from the name.
func TestMarkdownParser_StatusKeywordsAreWordBounded(t *testing.T) {
	cases := []struct {
		header     string
		wantStatus string
		wantName   string
	}{
		{"### Age 3: The Public Release (🔄 IN PROGRESS)", "in_progress", "The Public Release"},
		{"### Age 4: The Scaling Era (📋 PLANNED)", "planned", "The Scaling Era"},
		{"### Age 5: The Foundation (✅ COMPLETE)", "completed", "The Foundation"},
		{"### Age 6: The Draft (incomplete)", "planned", "The Draft (incomplete)"},
		{"### Age 7: The Gate (unblocked)", "planned", "The Gate (unblocked)"},
		{"### Age 8: The Wall (BLOCKED)", "blocked", "The Wall"},
	}
	for _, c := range cases {
		tl, err := (&MarkdownParser{}).ParseContent(c.header + "\n")
		if err != nil {
			t.Fatalf("%q: ParseContent() error: %v", c.header, err)
		}
		if len(tl.Phases) != 1 {
			t.Fatalf("%q: expected 1 phase, got %d", c.header, len(tl.Phases))
		}
		if tl.Phases[0].Status != c.wantStatus {
			t.Errorf("%q: status want %q, got %q", c.header, c.wantStatus, tl.Phases[0].Status)
		}
		if tl.Phases[0].Name != c.wantName {
			t.Errorf("%q: name want %q, got %q", c.header, c.wantName, tl.Phases[0].Name)
		}
	}
}

// parseStatus is also reachable from body-text status lines, so it needs
// the same word-boundary rule independently of the header regex.
func TestParseStatus_WordBounded(t *testing.T) {
	cases := map[string]string{
		"COMPLETE": "completed", "completed": "completed", "INCOMPLETE": "planned",
		"IN PROGRESS": "in_progress", "IN_PROGRESS": "in_progress",
		"BLOCKED": "blocked", "UNBLOCKED": "planned", "BLOCK": "blocked",
	}
	for in, want := range cases {
		if got := parseStatus(in); got != want {
			t.Errorf("parseStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
