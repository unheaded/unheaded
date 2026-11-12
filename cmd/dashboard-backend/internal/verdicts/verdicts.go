// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package verdicts is the third-party inventory as the dashboard's
// /verdicts page shows it (ADR-113, read-only first step): every outside
// thing the repository pulls in, calls, bundles or carries, with Stevie's
// verdict on it (keep, replace with our own, cut) where he has given one.
//
// The record is docs/policy/third-party-inventory-<date>.md, a dated file
// nobody edits afterwards. The dashboard binary cannot read docs/ where it
// runs, so the tables of that file are kept here as inventory.json, parsed
// by Parse and embedded; a test fails when the two differ, and
//
//	go test ./cmd/dashboard-backend/internal/verdicts -run TestEmbeddedInventoryIsCurrent -update
//
// rewrites it. Nothing here is written to at run time: verdicts are given
// in the record, in git.
package verdicts

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SourceFile is the dated inventory the page shows, relative to the
// repository root.
const SourceFile = "docs/policy/third-party-inventory-2026-10-29.md"

// Verdict values the page counts. Anything else Stevie wrote in the column
// is shown as written and counted as "other".
const (
	Keep      = "keep"
	Replace   = "replace"
	Cut       = "cut"
	Other     = "other"
	Undecided = "undecided"
)

//go:embed inventory.json
var embedded []byte

// Field is one cell of a row besides its name and verdict.
type Field struct {
	Column string `json:"column"`
	Value  string `json:"value"`
}

// Item is one third-party thing.
type Item struct {
	Name string `json:"name"`
	// Verdict is the cell as written, trimmed: empty until Stevie rules.
	Verdict string `json:"verdict"`
	// Class is Verdict reduced to keep, replace, cut, other or undecided.
	Class  string  `json:"class"`
	Fields []Field `json:"fields"`
}

// Section is one table of the inventory.
type Section struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"` // the cells of Fields, in order
	Items   []Item   `json:"items"`
}

// Inventory is the whole record.
type Inventory struct {
	Source   string         `json:"source"`
	Title    string         `json:"title"`
	Sections []Section      `json:"sections"`
	Counts   map[string]int `json:"counts"` // by class, plus "total"
}

// Classify reduces a verdict cell to a class. The column's own vocabulary
// is "keep / replace with our own / cut".
func Classify(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch {
	case s == "":
		return Undecided
	case strings.HasPrefix(s, Keep):
		return Keep
	case strings.HasPrefix(s, Replace):
		return Replace
	case strings.HasPrefix(s, Cut):
		return Cut
	}
	return Other
}

const maxLine = 64 << 10

// Parse reads an inventory file: every "## " section whose table has a
// Verdict column becomes a Section. The first column names the item; the
// Verdict column is its verdict; the rest are fields. Markdown code and
// bold marks are dropped, so every value is plain text. A table row with
// the wrong number of cells is an error: a damaged record must not be
// shown as a shorter one.
func Parse(source string, md []byte) (*Inventory, error) {
	inv := &Inventory{Source: source, Counts: map[string]int{}}
	sc := bufio.NewScanner(bytes.NewReader(md))
	sc.Buffer(make([]byte, 0, maxLine), maxLine)
	var (
		section string
		cur     *Section
		header  []string
		verdict = -1
		line    int
	)
	flush := func() {
		if cur != nil && len(cur.Items) > 0 {
			inv.Sections = append(inv.Sections, *cur)
		}
		cur, header, verdict = nil, nil, -1
	}
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(text, "# ") && inv.Title == "":
			inv.Title = plain(strings.TrimPrefix(text, "# "))
		case strings.HasPrefix(text, "## "):
			flush()
			section = plain(strings.TrimPrefix(text, "## "))
		case strings.HasPrefix(text, "|"):
			cells := splitRow(text)
			if header == nil {
				header = cells
				for i, c := range cells {
					if strings.EqualFold(c, "Verdict") {
						verdict = i
					}
				}
				if verdict > 0 {
					cur = &Section{Name: section}
					for i, c := range cells {
						if i != 0 && i != verdict {
							cur.Columns = append(cur.Columns, c)
						}
					}
				}
				continue
			}
			if isRule(cells) || cur == nil {
				continue
			}
			if len(cells) != len(header) {
				return nil, fmt.Errorf("%s:%d: %d cells in a %d-column table", source, line, len(cells), len(header))
			}
			if cells[0] == "" {
				return nil, fmt.Errorf("%s:%d: a row with no name", source, line)
			}
			it := Item{Name: cells[0], Verdict: cells[verdict], Class: Classify(cells[verdict]), Fields: []Field{}}
			for i, c := range cells {
				if i != 0 && i != verdict {
					it.Fields = append(it.Fields, Field{Column: header[i], Value: c})
				}
			}
			cur.Items = append(cur.Items, it)
			inv.Counts[it.Class]++
			inv.Counts["total"]++
		default:
			// Prose ends a table; a second table under the same heading
			// starts fresh.
			if header != nil {
				flush()
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	flush()
	if len(inv.Sections) == 0 {
		return nil, errors.New(source + ": no table with a Verdict column")
	}
	for _, c := range []string{Keep, Replace, Cut, Other, Undecided} {
		inv.Counts[c] += 0 // every class is present, zero or not
	}
	return inv, nil
}

// splitRow is the cells of a markdown table row, trimmed and plain.
func splitRow(row string) []string {
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	parts := strings.Split(row, "|")
	for i, p := range parts {
		parts[i] = plain(p)
	}
	return parts
}

// isRule reports whether cells are the |---|---:| line under a header.
func isRule(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return len(cells) > 0
}

// plain drops the markdown marks the inventory uses (code spans, bold) and
// trims. What is left is shown as text, never as markup.
func plain(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	return strings.TrimSpace(s)
}

// Encode is the JSON kept in inventory.json: indented, one trailing newline.
func Encode(inv *Inventory) ([]byte, error) {
	b, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// JSON is the embedded inventory, as served.
func JSON() []byte { return embedded }

// Load decodes the embedded inventory.
func Load() (*Inventory, error) {
	var inv Inventory
	if err := json.Unmarshal(embedded, &inv); err != nil {
		return nil, fmt.Errorf("embedded inventory: %w", err)
	}
	return &inv, nil
}
