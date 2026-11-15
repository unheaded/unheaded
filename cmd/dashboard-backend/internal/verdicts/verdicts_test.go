// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package verdicts

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite inventory.json from the dated inventory under docs/policy")

const sample = `# Third-party inventory (2026-10-29)

Prose with a | pipe is not a table row.

| Section | Items |
|---|---:|
| Go modules (direct) | 2 |

## Go modules (direct)

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| ` + "`github.com/rs/zerolog`" + ` | v1.35.1 | go.mod | ` + "`7d90a293`" + ` 2026-01-26 foundation |  | keep |
| ` + "`modernc.org/sqlite`" + ` | v1.61.0 | go.mod | ` + "`a68d6e57`" + ` 2026-02-02 |  assistant | Replace with our own (ADR-004) |
| ` + "`example.com/x`" + ` | v0 | go.mod | c |  | |

## Outside services the code calls

| Service | Called from | Verdict |
|---|---|---|
| CISA KEV feed | ` + "`cmd/kev-poller`" + ` | cut |
| Anthropic API | **pkg/claudebridge** | later, with Fable |

## Corrections

**Missed rows.** Text.

| Item | Version / tag | Where | Notes | Verdict |
|---|---|---|---|---|
| FRR | source build | docker/firewall/frr/ | builds | |

Verdicts are collected on a review page.
`

func TestParse(t *testing.T) {
	inv, err := Parse("x.md", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Title != "Third-party inventory (2026-10-29)" || inv.Source != "x.md" {
		t.Errorf("title %q source %q", inv.Title, inv.Source)
	}
	// The totals table has no Verdict column and is not a section.
	if len(inv.Sections) != 3 {
		t.Fatalf("%d sections, want 3: %+v", len(inv.Sections), inv.Sections)
	}
	g := inv.Sections[0]
	if g.Name != "Go modules (direct)" || len(g.Items) != 3 {
		t.Fatalf("section 0: %+v", g)
	}
	if got := strings.Join(g.Columns, ","); got != "Version / tag,Where,First added,By" {
		t.Errorf("columns %q", got)
	}
	z := g.Items[0]
	if z.Name != "github.com/rs/zerolog" || z.Verdict != "keep" || z.Class != Keep {
		t.Errorf("item 0: %+v", z)
	}
	if z.Fields[2].Column != "First added" || z.Fields[2].Value != "7d90a293 2026-01-26 foundation" {
		t.Errorf("field: %+v", z.Fields[2])
	}
	if s := g.Items[1]; s.Class != Replace || s.Verdict != "Replace with our own (ADR-004)" || s.Fields[3].Value != "assistant" {
		t.Errorf("item 1: %+v", s)
	}
	if u := g.Items[2]; u.Class != Undecided || u.Verdict != "" {
		t.Errorf("item 2: %+v", u)
	}
	svc := inv.Sections[1]
	if svc.Items[0].Class != Cut || svc.Items[1].Class != Other || svc.Items[1].Fields[0].Value != "pkg/claudebridge" {
		t.Errorf("services: %+v", svc.Items)
	}
	if inv.Sections[2].Name != "Corrections" || inv.Sections[2].Items[0].Name != "FRR" {
		t.Errorf("corrections: %+v", inv.Sections[2])
	}
	want := map[string]int{"total": 6, Keep: 1, Replace: 1, Cut: 1, Other: 1, Undecided: 2}
	for k, v := range want {
		if inv.Counts[k] != v {
			t.Errorf("count %s = %d, want %d", k, inv.Counts[k], v)
		}
	}
}

// A damaged record is refused, not shown shorter.
func TestParseRefuses(t *testing.T) {
	for name, md := range map[string]string{
		"no verdict table": "# T\n\n## A\n\n| Item | Where |\n|---|---|\n| x | y |\n",
		"short row":        "# T\n\n## A\n\n| Item | Where | Verdict |\n|---|---|---|\n| x | y |\n",
		"long row":         "# T\n\n## A\n\n| Item | Where | Verdict |\n|---|---|---|\n| x | y | keep | extra |\n",
		"nameless row":     "# T\n\n## A\n\n| Item | Where | Verdict |\n|---|---|---|\n|  | y | keep |\n",
		"empty":            "",
	} {
		if inv, err := Parse("x.md", []byte(md)); err == nil {
			t.Errorf("%s: parsed %+v", name, inv)
		}
	}
}

func TestClassify(t *testing.T) {
	for in, want := range map[string]string{
		"": Undecided, "  ": Undecided, "keep": Keep, "Keep (pinned)": Keep, "replace with our own": Replace,
		"REPLACE": Replace, "cut": Cut, "Cut 2026-11-04": Cut, "ask Barrister": Other, "keeper?": Keep,
	} {
		if got := Classify(in); got != want {
			t.Errorf("Classify(%q) = %q, want %q", in, got, want)
		}
	}
}

// inventory.json is what Parse makes of the dated record today. Run with
// -update after the record gains verdicts or a newer record replaces it.
func TestEmbeddedInventoryIsCurrent(t *testing.T) {
	src := filepath.Join("..", "..", "..", "..", filepath.FromSlash(SourceFile))
	md, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("the dated inventory: %v", err)
	}
	inv, err := Parse(SourceFile, md)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Encode(inv)
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile("inventory.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("inventory.json rewritten: %d items in %d sections", inv.Counts["total"], len(inv.Sections))
		return
	}
	if !bytes.Equal(JSON(), want) {
		t.Fatalf("inventory.json is stale against %s: go test ./cmd/dashboard-backend/internal/verdicts -run TestEmbeddedInventoryIsCurrent -update", SourceFile)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// The record's own totals: 216 direct items in its first seven
	// sections, then 5 pieces of foreign code, 9 outside services and the
	// 5 rows its corrections added.
	n := 0
	for _, s := range got.Sections[:7] {
		n += len(s.Items)
	}
	if n != 216 || len(got.Sections) != 10 || got.Counts["total"] != 235 {
		t.Errorf("%d direct items, %d sections, %d rows; the record says 216, 10 and 235", n, len(got.Sections), got.Counts["total"])
	}
}
