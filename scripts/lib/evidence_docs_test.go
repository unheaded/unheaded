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

// M-evidence: Doom left this repository, so the service map no longer draws it.
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
