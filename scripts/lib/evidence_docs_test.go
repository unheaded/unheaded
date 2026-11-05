// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"encoding/xml"
	"regexp"
	"strings"
	"testing"
)

// ontologyCell is the part of a draw.io cell the checks below read.
type ontologyCell struct {
	ID     string `xml:"id,attr"`
	Value  string `xml:"value,attr"`
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
}

// ontologyCells parses the diagram source, every page of it.
func ontologyCells(t *testing.T) []ontologyCell {
	t.Helper()
	var file struct {
		Diagrams []struct {
			Cells []ontologyCell `xml:"mxGraphModel>root>mxCell"`
		} `xml:"diagram"`
	}
	src := moveoutRepoFile(t, "docs/architecture/service-ontology.drawio")
	if err := xml.Unmarshal([]byte(src), &file); err != nil {
		t.Fatalf("service-ontology.drawio does not parse: %v", err)
	}
	var cells []ontologyCell
	for _, d := range file.Diagrams {
		cells = append(cells, d.Cells...)
	}
	if len(cells) == 0 {
		t.Fatal("service-ontology.drawio has no cells")
	}
	return cells
}

// Doom left this repository, so the service map no longer draws it.
func TestOntologyDrawsNoDoom(t *testing.T) {
	doom := regexp.MustCompile(`(?i)doom|E1M1`)
	for _, c := range ontologyCells(t) {
		if doom.MatchString(c.ID) || doom.MatchString(c.Value) {
			t.Errorf("cell %s still draws it: %q", c.ID, c.Value)
		}
	}
}

// A removed box takes its arrows with it: draw.io drops an edge whose end is gone.
func TestOntologyEdgesReachCells(t *testing.T) {
	cells := ontologyCells(t)
	ids := make(map[string]bool, len(cells))
	for _, c := range cells {
		ids[c.ID] = true
	}
	for _, c := range cells {
		for _, end := range []string{c.Source, c.Target} {
			if end != "" && !ids[end] {
				t.Errorf("edge %s reaches %s, which does not exist", c.ID, end)
			}
		}
	}
}

// The PDF cannot be redrawn on a host with no draw.io, so the page beside it says so.
func TestOntologyPDFIsMarkedStale(t *testing.T) {
	note := moveoutRepoFile(t, "docs/architecture/diagrams/README.md")
	if !strings.Contains(note, "service-ontology.drawio.pdf") || !strings.Contains(note, "stale") {
		t.Errorf("docs/architecture/diagrams/README.md does not mark the PDF stale:\n%s", note)
	}
}

// hostTidyStep gives one step of the S-HOST-TIDY plan, up to the next step.
func hostTidyStep(t *testing.T, n string) string {
	t.Helper()
	plan := moveoutRepoFile(t, "docs/battle-plans/S-HOST-TIDY-battle-plan.md")
	_, after, found := strings.Cut(plan, "**Step "+n+"**")
	if !found {
		t.Fatalf("no Step %s in the plan", n)
	}
	body, _, _ := strings.Cut(after, "\n- [ ] **Step ")
	return body
}

// Ruling 1: resolved keeps a current server that stays listed, so the plan sets
// Pi-hole alone, looks one name up, and confirms it, before the full list.
func TestHostTidyStep58SetsPiHoleAlone(t *testing.T) {
	step := hostTidyStep(t, "58")
	alone := strings.Index(step, "resolvectl dns enp2s0 10.10.20.252 &&")
	lookup := strings.Index(step, "resolvectl query archive.ubuntu.com")
	confirm := strings.Index(step, `grep "Current DNS"`)
	full := strings.Index(step, "resolvectl dns enp2s0 10.10.20.252 1.1.1.1 8.8.8.8")
	if alone < 0 || lookup < alone || confirm < lookup || full < confirm {
		t.Errorf("Step 58 must set Pi-hole alone, look up, confirm, then set the list; at %d %d %d %d", alone, lookup, confirm, full)
	}
	if !strings.Contains(step, "`Current DNS Server: 10.10.20.252`") {
		t.Error("Step 58's Expect does not name the current server")
	}
	if strings.Contains(step, "sudo systemctl restart") || strings.Contains(step, "sudo netplan apply") {
		t.Error("the ruling runs no resolved restart and no netplan apply")
	}
}

// Ruling 2: resolvectl prints A and AAAA in varying order, so the check asks for A.
func TestHostTidyStep59AsksForA(t *testing.T) {
	step := hostTidyStep(t, "59")
	if !strings.Contains(step, "resolvectl query -t A o401343.ingest.us.sentry.io") {
		t.Error("Step 59's first check does not ask for the A record")
	}
	if !strings.Contains(step, "`o401343.ingest.us.sentry.io IN A 0.0.0.0") {
		t.Error("Step 59's Expect is not the shape -t A prints")
	}
	if strings.Contains(step, "`o401343.ingest.us.sentry.io: 0.0.0.0") {
		t.Error("Step 59 still expects the shape of a plain query")
	}
}

// Phase 18 ran on west. The progress file says so, and says what it did beyond
// the plan: Step 131 installs the whole sysusers file, not three lines of it.
func TestHostTidyProgressRecordsPhase18(t *testing.T) {
	progress := moveoutRepoFile(t, "docs/battle-plans/progress/S-HOST-TIDY-progress.md")
	if strings.Contains(progress, "Phase 18 has not run") {
		t.Error("the progress file still says Phase 18 has not run")
	}
	var record string
	for _, line := range strings.Split(progress, "\n") {
		if strings.HasPrefix(line, "- Phase 18 ran on west") {
			record = line
		}
	}
	if record == "" {
		t.Fatal("the progress file has no line recording that Phase 18 ran on west")
	}
	sysusers := moveoutRepoFile(t, "deploy/sysusers.d/unheaded.conf")
	for _, account := range []string{
		"unheaded-vor", "unheaded-buildkey", "unheaded-heimdall", "unheaded-auth",
		"unheaded-well-retention", "unheaded-ci-status", "unheaded-heartbeat", "unheaded-vector",
	} {
		if !strings.Contains(record, "`"+account+"`") {
			t.Errorf("the Phase 18 line does not name %s", account)
		}
		if !strings.Contains(sysusers, "u     "+account+" ") {
			t.Errorf("%s is not in deploy/sysusers.d/unheaded.conf", account)
		}
	}
	for _, fact := range []string{"eight", "labelled east", "/var/backups/unheaded-sysusers.conf.bak-host-tidy"} {
		if !strings.Contains(record, fact) {
			t.Errorf("the Phase 18 line does not say %q", fact)
		}
	}
}
