// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PQ key-exchange lint (ADR-105, plan Phase 4): a repo configuration that
// pins or forces classical-only key exchange is a violation. Today's
// violations live in docs/policy/pq-kex-baseline.txt, which may only shrink
// (ADR-098). Keys are "<path> <rule>": no line numbers, so an unrelated
// edit above a site does not churn the baseline.
const (
	ruleCurvePin    = "curve-pin-classical"   // CurvePreferences with no hybrid group
	ruleFloor       = "tls-floor-below-1.3"   // first-party TLS floor below 1.3 (1.2 sessions are classical)
	ruleCap         = "tls-max-below-1.3"     // MaxVersion below 1.3 forces a classical session
	ruleGodebug     = "godebug-pq-off"        // tlsmlkem=0 / tlssecpmlkem=0
	ruleGoDirective = "go-directive-pre-1.24" // go < 1.24 turns the hybrid default off
	ruleRustRing    = "rust-ring-provider"    // rustls on ring (no ML-KEM)
	rulePGPre18     = "postgres-pre18"        // PG <= 17 serving TLS: one classical curve
	rulePGGroups    = "postgres-groups-classical"
	ruleSSHD        = "sshd-kex-classical" // a KexAlgorithms list with no hybrid at all
)

// Violation is one lint hit.
type Violation struct {
	Path, Rule string
	Line       int
	Detail     string
}

// Key is the baseline entry.
func (v Violation) Key() string { return v.Path + " " + v.Rule }

func lintIsExempt(path string) bool {
	for _, e := range lintExempt {
		if strings.HasPrefix(path, e.prefix) {
			return true
		}
	}
	return false
}

// rulesFor returns the rules an asset violates.
func rulesFor(a Asset) []string {
	if lintIsExempt(a.Path) || a.Scanner == "host" {
		return nil // red-team probes; host facts are not repo config
	}
	var rules []string
	switch a.Kind {
	case kindTLSConfig, kindTLSGroups:
		if a.PQStatus == pqClassical && len(a.Groups) > 0 {
			rules = append(rules, ruleCurvePin)
		}
	}
	switch a.Kind {
	case kindTLSConfig, kindTLSFloor, kindGRPCCreds:
		if a.Owner == ownerFirstParty && below13(a.Floor) {
			rules = append(rules, ruleFloor)
		}
	}
	switch a.Kind {
	case kindTLSConfig, kindTLSCap:
		if below13(a.Cap) {
			rules = append(rules, ruleCap)
		}
	case kindGodebug:
		rules = append(rules, ruleGodebug)
	case kindGoDirective:
		if a.PQStatus == pqClassical {
			rules = append(rules, ruleGoDirective)
		}
	case kindRustProvider, kindRustBuilder, kindRustTonicTLS:
		if a.PQStatus == pqClassical && a.Kind != kindRustBuilder {
			rules = append(rules, ruleRustRing)
		}
	case kindRustDep:
		if a.PQStatus == pqClassical && a.LinkState == linkEnabled {
			rules = append(rules, ruleRustRing)
		}
	case kindPostgresImage:
		if a.LinkState == linkEnabled && a.PGMajor > 0 && a.PGMajor < 18 {
			rules = append(rules, rulePGPre18)
		} else if a.LinkState == linkEnabled && a.PGMajor >= 18 && a.PQStatus == pqClassical {
			rules = append(rules, rulePGGroups)
		}
	case kindPostgresConf:
		if len(a.Groups) > 0 && a.PQStatus == pqClassical {
			rules = append(rules, rulePGGroups)
		}
	case kindSSHDKex:
		if a.PQStatus == pqClassical {
			rules = append(rules, ruleSSHD)
		}
	}
	return rules
}

// withRules fills each asset's Rules (shown in the CBOM as unheaded:finding).
func withRules(assets []Asset) []Asset {
	out := make([]Asset, len(assets))
	for i, a := range assets {
		a.Rules = rulesFor(a)
		out[i] = a
	}
	return out
}

// lintAssets returns every violation, sorted by key then line.
func lintAssets(assets []Asset) []Violation {
	var out []Violation
	for _, a := range assets {
		for _, r := range rulesFor(a) {
			out = append(out, Violation{Path: a.Path, Rule: r, Line: a.Line, Detail: a.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key() != out[j].Key() {
			return out[i].Key() < out[j].Key()
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// uniqueKeys returns the sorted set of violation keys.
func uniqueKeys(vs []Violation) []string {
	seen := map[string]bool{}
	var keys []string
	for _, v := range vs {
		if !seen[v.Key()] {
			seen[v.Key()] = true
			keys = append(keys, v.Key())
		}
	}
	sort.Strings(keys)
	return keys
}

// ratchet returns keys in current but not baseline (added) and in baseline
// but not current (resolved). Both inputs must be sorted and unique.
func ratchet(current, baseline []string) (added, resolved []string) {
	cur := map[string]bool{}
	for _, k := range current {
		cur[k] = true
	}
	base := map[string]bool{}
	for _, k := range baseline {
		base[k] = true
		if !cur[k] {
			resolved = append(resolved, k)
		}
	}
	for _, k := range current {
		if !base[k] {
			added = append(added, k)
		}
	}
	return added, resolved
}

const baselineHeader = "# shrink-only (ADR-098 ratchet, ADR-105). Remove lines as Phases 5/7/8/9 fix them.\n" +
	"# <path> <rule>. Regenerate after a fix: go run ./cmd/crypto-inventory lint --update (refuses additions)."

// readBaseline returns the sorted unique entries and the header comments.
// A missing or malformed baseline is an error, never an empty set.
func readBaseline(top, path string) (entries, header []string, err error) {
	b, err := readRepoFile(top, path)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			header = append(header, line)
			continue
		}
		if len(strings.Fields(line)) != 2 {
			return nil, nil, fmt.Errorf("%s:%d: want \"<path> <rule>\", got %q", path, n, line)
		}
		key := strings.Join(strings.Fields(line), " ")
		if !seen[key] {
			seen[key] = true
			entries = append(entries, key)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	sort.Strings(entries)
	return entries, header, nil
}

func writeBaseline(top, path string, header, entries []string) error {
	var b strings.Builder
	if len(header) == 0 {
		header = strings.Split(baselineHeader, "\n")
	}
	for _, h := range header {
		b.WriteString(h + "\n")
	}
	for _, e := range entries {
		b.WriteString(e + "\n")
	}
	return writeRepoFile(top, path, []byte(b.String()))
}

// cmdLint: --print lists violations; --explain P shows what the scanners saw
// in one file; otherwise compare with the baseline (--update rewrites it,
// refusing additions).
func cmdLint(root, baselinePath string, print, update bool, explain string, stdout, stderr io.Writer) int {
	tr, err := gitTree(root)
	if err != nil {
		fmt.Fprintln(stderr, "lint:", err)
		return 2
	}
	assets, err := scanAll(tr)
	if err != nil {
		fmt.Fprintln(stderr, "lint:", err)
		return 2
	}
	if explain != "" {
		return explainPath(tr, assets, explain, stdout)
	}
	vs := lintAssets(assets)
	keys := uniqueKeys(vs)
	if print {
		for _, k := range keys {
			fmt.Fprintln(stdout, k)
		}
		return 0
	}
	p := filepath.FromSlash(baselinePath)
	base, header, err := readBaseline(tr.Root, p)
	if err != nil && (!update || !errors.Is(err, os.ErrNotExist)) {
		fmt.Fprintf(stderr, "[FAIL] baseline %s: %v\n", baselinePath, err)
		return 1
	}
	added, resolved := ratchet(keys, base)
	if update {
		if err == nil && len(added) > 0 {
			fmt.Fprintln(stderr, "[REFUSED] --update only shrinks. New classical key-exchange configuration:")
			for _, k := range added {
				fmt.Fprintln(stderr, "         "+k)
			}
			return 1
		}
		if err := writeBaseline(tr.Root, p, header, keys); err != nil {
			fmt.Fprintln(stderr, "lint:", err)
			return 2
		}
		fmt.Fprintf(stdout, "[UPDATED] %s (%d entries)\n", baselinePath, len(keys))
		return 0
	}
	rc := 0
	if len(added) > 0 {
		fmt.Fprintln(stderr, "[FAIL] classical-only key exchange configured (ADR-105):")
		for _, k := range added {
			for _, v := range vs {
				if v.Key() == k {
					fmt.Fprintf(stderr, "         %s  (%s:%d %s)\n", k, v.Path, v.Line, v.Detail)
					break
				}
			}
		}
		fmt.Fprintln(stderr, "       Use a hybrid group (X25519MLKEM768), a TLS 1.3 floor, the aws-lc-rs provider, PG 18 ssl_groups or an mlkem/sntrup KEX; see ADR-105.")
		rc = 1
	}
	if len(resolved) > 0 {
		fmt.Fprintf(stderr, "[FAIL] resolved: remove from baseline (go run ./cmd/crypto-inventory lint --update):\n")
		for _, k := range resolved {
			fmt.Fprintln(stderr, "         "+k)
		}
		rc = 1
	}
	if rc == 0 {
		fmt.Fprintf(stdout, "[PASS] %d classical key-exchange entries, all in the baseline\n", len(keys))
	}
	return rc
}

func explainPath(tr *Tree, assets []Asset, path string, stdout io.Writer) int {
	fmt.Fprintf(stdout, "%s: in tree: %v\n", path, tr.Has(path))
	found := 0
	for _, a := range assets {
		if a.Path != path {
			continue
		}
		found++
		fmt.Fprintf(stdout, "  line %d  scanner=%s kind=%s pq-status=%s owner=%s link=%s rules=%s\n    %s\n",
			a.Line, a.Scanner, a.Kind, a.PQStatus, a.Owner, a.LinkState, strings.Join(rulesFor(a), ","), a.Detail)
	}
	if found == 0 {
		fmt.Fprintln(stdout, "  no scanner recorded anything in this file")
	}
	if lintIsExempt(path) {
		fmt.Fprintln(stdout, "  lint-exempt (cmd/lich-security probes weak TLS on purpose)")
	}
	return 0
}
