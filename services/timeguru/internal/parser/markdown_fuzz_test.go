package parser

import (
	"testing"
)

// FuzzParseContent feeds random markdown content to the timeline parser.
// The parser must never panic, even on adversarial input.
func FuzzParseContent(f *testing.F) {
	// Seed corpus: representative timeline markdown fragments.
	f.Add(`# The Unheaded Chronicles

**STATUS:** Alpha Development

### Age 1: The Alpha Ascension (IN PROGRESS)

#### Epoch 1.1: The Whispering Void Awakens
**Risk:** Medium
**Owner:** Agent 5
ETA: Feb 3-4, 2026

- [ ] packet_marker.bpf
- [x] Initial research
`)

	f.Add(`### Phase 1: Test (COMPLETE)
25% FORGED
#### Milestone 1.1: Sub Test
**Risk:** High
50% complete
- [x] Done
- [ ] Not done
`)

	f.Add(`### Phase 2: Beta Trials (PLANNED)
#### Epoch 2.1: Production Hardening
**Owner:** The Architect
**Risk:** Low
ETA: March 15, 2026
- [ ] Task A
- [ ] Task B
- [ ] Task C
`)

	// Edge cases.
	f.Add("")
	f.Add("# Just a Title")
	f.Add("### Age 0: Empty Phase")
	f.Add("#### Epoch 0.1: Empty Milestone")
	f.Add("- [x] Single checkbox")
	f.Add("- [ ] Incomplete task")
	f.Add("**STATUS:** Production")
	f.Add("**STATUS:** Beta Candidate")
	f.Add("**Risk:** High\n**Owner:** Nobody\nETA: 2026-01-01")

	// Potentially adversarial patterns.
	f.Add("### Age 99999999999: Overflow Test (COMPLETE ✓)")
	f.Add("#### Epoch 1.2.3.4.5: Deep nesting")
	f.Add("### Age 1: " + string(make([]byte, 1000)))
	f.Add("- [x] " + string(make([]byte, 500)))
	f.Add("100% FORGED\n200% FORGED\n0% FORGED")
	f.Add("**Risk:** unknown\n**Risk:** HIGH\n**Risk:** low")

	f.Fuzz(func(t *testing.T, content string) {
		parser := &MarkdownParser{}
		tl, err := parser.ParseContent(content)
		if err != nil {
			return // parse errors are expected for random input
		}

		// Invariant: timeline should always have initialized slices.
		if tl.Phases == nil {
			t.Error("Phases should never be nil")
		}
		if tl.Milestones == nil {
			t.Error("Milestones should never be nil")
		}
		if tl.Metadata == nil {
			t.Error("Metadata should never be nil")
		}

		// Invariant: each phase should have an ID.
		for i, phase := range tl.Phases {
			if phase.ID == "" {
				t.Errorf("Phase[%d] has empty ID", i)
			}
			// Note: Name can be empty for edge-case inputs like "### Age 0  "
			// where the regex matches but produces an empty name after trimming.
		}

		// Invariant: each milestone should have an ID.
		for i, ms := range tl.Milestones {
			if ms.ID == "" {
				t.Errorf("Milestone[%d] has empty ID", i)
			}
			// Note: Name can be empty for similar edge-case regex matches.
		}
	})
}

// FuzzParseDate feeds random strings to the date parser.
func FuzzParseDate(f *testing.F) {
	f.Add("Feb 3, 2026")
	f.Add("February 8, 2026")
	f.Add("Feb 3-4, 2026")
	f.Add("2026-02-15")
	f.Add("Jan 1 2026")
	f.Add("02 Jan 2006")
	f.Add("")
	f.Add("not a date")
	f.Add("Dec 31-99, 9999")
	f.Add("Feb 0, 0000")
	f.Add("Month 99, 1234")

	f.Fuzz(func(t *testing.T, input string) {
		// parseDate must never panic.
		result := parseDate(input)
		_ = result
	})
}

// FuzzParseStatus feeds random strings to the status parser.
func FuzzParseStatus(f *testing.F) {
	f.Add("COMPLETE")
	f.Add("COMPLETED")
	f.Add("IN PROGRESS")
	f.Add("IN_PROGRESS")
	f.Add("BLOCKED")
	f.Add("PLANNED")
	f.Add("")
	f.Add("unknown status")
	f.Add("COMPLETE ✓")
	f.Add("IN PROGRESS 🚀")

	f.Fuzz(func(t *testing.T, input string) {
		result := parseStatus(input)
		// Invariant: result must be one of the known statuses.
		validStatuses := map[string]bool{
			"completed":   true,
			"in_progress": true,
			"blocked":     true,
			"planned":     true,
		}
		if !validStatuses[result] {
			t.Errorf("parseStatus(%q) = %q, not a valid status", input, result)
		}
	})
}

// FuzzExtractStatus feeds random strings to the status extractor.
func FuzzExtractStatus(f *testing.F) {
	f.Add("(COMPLETE ✓)")
	f.Add("(IN PROGRESS 🚀)")
	f.Add("(BLOCKED)")
	f.Add("(PLANNED)")
	f.Add("no status here")
	f.Add("")
	f.Add("Status: COMPLETE")

	f.Fuzz(func(t *testing.T, input string) {
		result := extractStatus(input)
		// Invariant: result must be empty or a known status.
		validStatuses := map[string]bool{
			"":            true,
			"completed":   true,
			"in_progress": true,
			"blocked":     true,
			"planned":     true,
		}
		if !validStatuses[result] {
			t.Errorf("extractStatus(%q) = %q, not a valid status", input, result)
		}
	})
}
