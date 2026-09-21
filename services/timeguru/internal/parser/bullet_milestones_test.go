// SPDX-License-Identifier: GPL-3.0-or-later
package parser

import (
	"strings"
	"testing"
)

// references/timeline.md's real shape: no "####" milestones, bold section
// lines, long bullets. Each bullet must become a milestone whose status comes
// from the section (or the phase), with a stable content-hashed id.
func TestBulletMilestones_RealFileConventions(t *testing.T) {
	content := `### Age 3: The Public Release (🔄 IN PROGRESS)
**Progress:** ~70%
**Status:** Some prose about the thread.

**Completed sub-items:**
- S67 Wire Format FROZEN at v0x01 (12 IANA registries, IPR clear)
- **Doom browser-verified LIVE (2026-07-19) — reference point.** Long tail of detail.

**Remaining for Age 3:**
- Sophia draft-03 ship-or-defer
- Public accessibility (optional auth)

### Age 4: The MVP Era (📋 PLANNED)
*Track-call dependent.*
- WAVE14 BackwardScratch + KV-cache (gated on Track A or C)
- Billing/metering
`
	tl, err := (&MarkdownParser{}).ParseContent(content)
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len(tl.Milestones) != 6 {
		t.Fatalf("want 6 bullet milestones, got %d", len(tl.Milestones))
	}
	want := []struct{ name, status, phase string }{
		{"S67 Wire Format FROZEN at v0x01", "completed", "phase-3"},
		{"Doom browser-verified LIVE", "completed", "phase-3"},
		{"Sophia draft-03 ship-or-defer", "pending", "phase-3"},
		{"Public accessibility", "pending", "phase-3"},
		{"WAVE14 BackwardScratch + KV-cache", "pending", "phase-4"}, // inherits the phase (planned -> pending)
		{"Billing/metering", "pending", "phase-4"},
	}
	for i, w := range want {
		m := tl.Milestones[i]
		if m.Name != w.name || m.Status != w.status {
			t.Errorf("milestone %d: want %q/%s, got %q/%s", i, w.name, w.status, m.Name, m.Status)
		}
		if m.Description == "" || m.Description == m.Name && i == 0 {
			t.Errorf("milestone %d: description must carry the full bullet text", i)
		}
		if len(m.ID) < len(w.phase)+6 || m.ID[:len(w.phase)] != w.phase {
			t.Errorf("milestone %d: id %q must start with %q", i, m.ID, w.phase)
		}
		if m.Status == "completed" && m.Progress != 100 {
			t.Errorf("milestone %d: completed must be progress 100, got %d", i, m.Progress)
		}
	}
	// Phase links are the same ids, in order.
	if got := len(tl.Phases[0].Milestones); got != 4 {
		t.Errorf("phase-3 should link 4 milestones, got %d", got)
	}
	// Prose lines and the Progress/Status declarations are not milestones
	// and do not change phase progress.
	if tl.Phases[0].Progress != 70 {
		t.Errorf("phase progress must stay 70, got %d", tl.Phases[0].Progress)
	}

	// Stable id: parsing again yields the same ids; changing the text changes it.
	tl2, _ := (&MarkdownParser{}).ParseContent(content)
	if tl2.Milestones[0].ID != tl.Milestones[0].ID {
		t.Error("id must be stable across parses")
	}
	// Inserting a bullet ABOVE an existing one must not change the existing
	// one's id (a positional scheme would shift it, and kanban would see a
	// remove+add for a card that did not change).
	inserted := strings.Replace(content, "- Billing/metering\n", "- Metering research\n- Billing/metering\n", 1)
	tl3, _ := (&MarkdownParser{}).ParseContent(inserted)
	if len(tl3.Milestones) != 7 {
		t.Fatalf("want 7 after insert, got %d", len(tl3.Milestones))
	}
	if tl3.Milestones[6].ID != tl.Milestones[5].ID {
		t.Errorf("Billing/metering id changed after an insertion above it: %s -> %s", tl.Milestones[5].ID, tl3.Milestones[6].ID)
	}
	if tl3.Milestones[5].ID == tl.Milestones[5].ID {
		t.Error("different text must not share an id")
	}
}
