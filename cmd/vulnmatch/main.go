// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Command vulnmatch matches the components a tree pins against OSV records and
// flags the ones whose CVE is in the CISA KEV catalog (DP-721 stage 1a: lock
// data only, the first step of the native trivy replacement; it replaces no
// trivy use yet, and trivy stays pinned until each stage does). It reads local
// files only and has no network code: OSV is an outside data source that waits
// on the third-party sitting.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/threatfeed/inventory"
	"unheaded/pkg/threatfeed/kev"
	"unheaded/pkg/threatfeed/osv"
)

// config is everything run needs; main fills it from the flags.
type config struct {
	root           string
	osvDir         string
	kevFile        string
	sbomFile       string
	jsonOut        bool
	failOnFindings bool
}

// Finding is one component inside one record's affected range.
type Finding struct {
	KEV       bool     `json:"kev"`
	ID        string   `json:"id"`
	Aliases   []string `json:"aliases,omitempty"`
	Ecosystem string   `json:"ecosystem"`
	Package   string   `json:"package"`
	Version   string   `json:"version"`
	FixedIn   string   `json:"fixed_in,omitempty"`
	Source    string   `json:"source"`
	Summary   string   `json:"summary,omitempty"`
}

// match returns every affected component, KEV entries first, then by id,
// package and version. A withdrawn record never matches.
func match(comps []inventory.Component, vulns []osv.Vuln, kevIDs map[string]bool) ([]Finding, error) {
	var out []Finding
	for _, v := range vulns {
		if v.Withdrawn != "" {
			continue
		}
		inKEV := kevIDs[v.ID]
		for _, a := range v.Aliases {
			inKEV = inKEV || kevIDs[a]
		}
		for _, c := range comps {
			for _, a := range v.Affected {
				hit, err := a.Matches(c.Ecosystem, c.Name, c.Version)
				if err != nil {
					return nil, fmt.Errorf("%s against %s %s: %w", v.ID, c.Name, c.Version, err)
				}
				if hit {
					out = append(out, Finding{inKEV, v.ID, v.Aliases, c.Ecosystem, c.Name, c.Version,
						a.FirstFixed(c.Ecosystem), c.Source, v.Summary})
					break
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.KEV != b.KEV {
			return a.KEV
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		return a.Version < b.Version
	})
	return out, nil
}

// loadKEV returns the catalog's CVE ids; an empty path means no catalog.
func loadKEV(path string) (map[string]bool, error) {
	ids := map[string]bool{}
	if path == "" {
		return ids, nil
	}
	b, err := os.ReadFile(path) // #nosec G304 -- operator-supplied catalog path
	if err != nil {
		return nil, err
	}
	c, err := kev.Parse(b)
	if err != nil {
		return nil, err
	}
	for _, v := range c.Vulnerabilities {
		ids[v.CVEID] = true
	}
	return ids, nil
}

func report(w io.Writer, fs []Finding) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEV\tID\tECOSYSTEM\tPACKAGE\tVERSION\tFIXED\tSOURCE")
	for _, f := range fs {
		k, fixed := "-", f.FixedIn
		if f.KEV {
			k = "KEV"
		}
		if fixed == "" {
			fixed = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", k, f.ID, f.Ecosystem, f.Package, f.Version, fixed, f.Source)
	}
	return tw.Flush()
}

// run scans and reports. It returns the exit code; an error means the scan
// itself failed.
func run(cfg config, stdout, stderr io.Writer) (int, error) {
	if cfg.osvDir == "" {
		return 0, errors.New("--osv-dir is required")
	}
	comps, unpinned, err := inventory.ScanTree(cfg.root)
	if err != nil {
		return 0, err
	}
	if cfg.sbomFile != "" {
		f, err := os.Open(cfg.sbomFile) // #nosec G304 -- operator-supplied SBOM path
		if err != nil {
			return 0, err
		}
		defer func() { _ = f.Close() }()
		more, err := inventory.ReadSBOMJSON(f, cfg.sbomFile)
		if err != nil {
			return 0, err
		}
		comps = append(comps, more...)
	}
	vulns, err := osv.LoadDir(cfg.osvDir)
	if err != nil {
		return 0, err
	}
	kevIDs, err := loadKEV(cfg.kevFile)
	if err != nil {
		return 0, err
	}
	found, err := match(comps, vulns, kevIDs)
	if err != nil {
		return 0, err
	}
	for _, u := range unpinned {
		fmt.Fprintf(stderr, "vulnmatch: not checked, not an exact pin: %s\n", u)
	}
	if cfg.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(found); err != nil {
			return 0, err
		}
	} else if err := report(stdout, found); err != nil {
		return 0, err
	}
	inKEV := 0
	for _, f := range found {
		if f.KEV {
			inKEV++
		}
	}
	fmt.Fprintf(stderr, "vulnmatch: %d components, %d records, %d findings (%d in KEV), %d not an exact pin\n",
		len(comps), len(vulns), len(found), inKEV, len(unpinned))
	if cfg.failOnFindings && len(found) > 0 {
		return 1, nil
	}
	return 0, nil
}

func main() {
	var cfg config
	flag.StringVar(&cfg.root, "root", ".", "tree to read lock data from")
	flag.StringVar(&cfg.osvDir, "osv-dir", "", "directory of OSV records, one *.json each")
	flag.StringVar(&cfg.kevFile, "kev-file", "", "CISA KEV catalog, kev-poller's latest.json")
	flag.StringVar(&cfg.sbomFile, "sbom", "", "also read sbom/go-dependencies.json or sbom/rust-dependencies.json")
	flag.BoolVar(&cfg.jsonOut, "json", false, "print findings as JSON")
	flag.BoolVar(&cfg.failOnFindings, "fail-on-findings", false, "exit 1 when anything matched")
	spec := cmdline.Spec{
		Name:    "vulnmatch",
		Purpose: "Match the components a tree pins against OSV records and the CISA KEV catalog.",
		Examples: []string{
			"vulnmatch --root . --osv-dir pkg/threatfeed/osv/testdata/records --kev-file pkg/threatfeed/osv/testdata/kev.json",
			"vulnmatch --osv-dir /var/tmp/osv --kev-file /var/lib/unheaded/threat-feeds/cisa-kev/latest.json --fail-on-findings",
		},
		Details: "Exit codes: 0 scan complete, 1 findings (with --fail-on-findings), 2 usage or scan error.\nReads local files only.",
	}
	cmdline.Parse(spec)

	code, err := run(cfg, os.Stdout, os.Stderr)
	if err != nil {
		cmdline.Fail(spec, "%s", strings.TrimSpace(err.Error()))
	}
	os.Exit(code)
}
