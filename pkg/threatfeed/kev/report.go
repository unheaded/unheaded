// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// ReportOptions shape a threat-register refresh entry.
type ReportOptions struct {
	Since     string // YYYY-MM-DD: entries with dateAdded on or after it count as new
	Today     string // heading date
	StackPath string // shown in the text; the stack itself is passed separately
	Recent    int    // how many non-stack entries to list (newest first)
}

// WriteReport renders a markdown refresh-log entry for
// docs/security/threat-register.md from a stored catalog. It reports what
// the catalog says; whether a host is patched is not in the catalog, so
// the entry ends with the command that answers that per CVE.
func WriteReport(w io.Writer, c *Catalog, st *State, stack *Stack, o ReportOptions) error {
	if c == nil || len(c.Vulnerabilities) == 0 {
		return fmt.Errorf("no catalog stored")
	}
	if o.Recent <= 0 {
		o.Recent = 15
	}
	all := append([]Vulnerability(nil), c.Vulnerabilities...)
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].DateAdded != all[j].DateAdded {
			return all[i].DateAdded > all[j].DateAdded
		}
		return all[i].CVEID < all[j].CVEID
	})
	first, last := all[len(all)-1].DateAdded, all[0].DateAdded
	var added []Vulnerability
	for _, v := range all {
		if v.DateAdded >= o.Since {
			added = append(added, v)
		}
	}
	relevantNew := stack.Relevant(added)
	relevantAll := stack.Relevant(all)

	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	p("### %s: kev-poller report\n\n", o.Today)
	p("**CISA KEV catalog** %s: %d entries, dateAdded %s to %s", c.CatalogVersion, len(all), first, last)
	if st != nil {
		short := st.SHA256
		if len(short) > 12 {
			short = short[:12]
		}
		p(", pulled %s (sha256 %s)", st.FetchedAt.UTC().Format("2006-01-02 15:04Z"), short)
	}
	p(".\n\n")
	p("- Added since %s: **%d**\n", o.Since, len(added))
	if stack == nil {
		p("- No stack file given: nothing is matched against what the Kingdom runs.\n\n")
	} else {
		p("- In the Kingdom's stack (`%s`): **%d** of them; %d in the whole catalog\n\n", o.StackPath, len(relevantNew), len(relevantAll))
		p("**Stack entries added since %s**:\n\n", o.Since)
		if len(relevantNew) == 0 {
			p("None.\n\n")
		} else {
			p("| Added | CVE | Vendor / product | Name | Category | CISA due |\n|---|---|---|---|---|---|\n")
			for _, t := range relevantNew {
				p("| %s | %s | %s / %s | %s | %s | %s |\n", t.DateAdded, t.CVEID, cell(t.VendorProject), cell(t.Product),
					cell(t.VulnerabilityName), strings.Join(t.Categories, ", "), t.DueDate)
			}
			p("\nPatched on a host? The catalog cannot say. On each Ubuntu host: `pro fix <CVE> --dry-run`:\n\n")
			for _, t := range relevantNew {
				p("    pro fix %s --dry-run\n", t.CVEID)
			}
			p("\n")
		}
	}
	other := 0
	p("**Other entries added since %s** (informational, newest first", o.Since)
	var rows []Vulnerability
	for _, v := range added {
		if len(stack.Classify(v)) == 0 {
			other++
			if len(rows) < o.Recent {
				rows = append(rows, v)
			}
		}
	}
	p(", %d of %d shown):\n\n", len(rows), other)
	if len(rows) > 0 {
		p("| Added | CVE | Vendor / product | Name |\n|---|---|---|---|\n")
		for _, v := range rows {
			p("| %s | %s | %s / %s | %s |\n", v.DateAdded, v.CVEID, cell(v.VendorProject), cell(v.Product), cell(v.VulnerabilityName))
		}
		p("\n")
	}
	return nil
}

// cell keeps catalog text from breaking a markdown table.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
