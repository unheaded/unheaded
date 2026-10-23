// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/zhencapture"
)

// Scanner reports, by kind, what in a text must not enter a corpus.
// Kinds only; never the values.
type Scanner interface {
	Scan(ctx context.Context, s string) (map[string]int, error)
}

// quarantineKinds are the privacy-tier kinds that keep a record out of
// the Well: secrets and personal identifiers. Addresses and host names
// are counted by the redactor but do not quarantine: code answers are
// full of 127.0.0.1 and example subnets, and the words "west"/"east".
var quarantineKinds = map[string]bool{
	"secret": true, "pem": true, "cred": true, "aws": true, // secrets (gitleaks + patterns)
	"email": true, "user": true, "mac": true, // personal / device identifiers
}

// Reserved example names (RFC 2606/6761) and generic home directories
// that code answers use; they are not anyone's identifier.
var (
	exampleEmailRE = regexp.MustCompile(`(?i)[A-Za-z0-9._%+-]+@(?:[A-Za-z0-9-]+\.)*(?:example\.(?:com|org|net)|example|test|invalid|localhost)\b`)
	genericHomeRE  = regexp.MustCompile(`/(?:home|Users)/(?:user|username|you|me|name|\$USER|\$\{USER\}|<user>|<username>)\b`)
)

// privacyScanner is the Phase 3 privacy tier (gitleaks stdin plus the
// T1 patterns) and the writer's own patterns, run again: a record
// captured before a pattern existed is caught here.
type privacyScanner struct {
	r *privacy.Redactor
}

func newPrivacyScanner() (*privacyScanner, error) {
	r, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: os.Getenv("ZHEN_SECRET_SCANNER")})
	if err != nil {
		return nil, err
	}
	return &privacyScanner{r: r}, nil
}

func (p *privacyScanner) Scan(ctx context.Context, s string) (map[string]int, error) {
	neutral := exampleEmailRE.ReplaceAllString(s, "example-address")
	neutral = genericHomeRE.ReplaceAllString(neutral, "~generic")
	_, counts, err := p.r.Redact(ctx, neutral, privacy.TierOps)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for k, v := range counts {
		if quarantineKinds[k] && v > 0 {
			out[k] += v
		}
	}
	if _, n := zhencapture.Redact(neutral); n > 0 {
		out["capture_pattern"] += n
	}
	return out, nil
}

// ScanReport is the result of `zhen-flywheel scan`: findings by file,
// line and kind, no values.
type ScanReport struct {
	Files    int      `json:"files"`
	Lines    int      `json:"lines"`
	Findings []string `json:"findings,omitempty"`
}

// scanDir runs the scanner over every line of every regular file under
// dir (an export). Placeholders the writer left ([REDACTED:...]) are not
// findings.
func scanDir(ctx context.Context, sc Scanner, dir string) (ScanReport, error) {
	var rep ScanReport
	var files []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return rep, err
	}
	sort.Strings(files)
	for _, f := range files {
		rep.Files++
		// #nosec G304 -- walking the export directory the operator named.
		fh, err := os.Open(f)
		if err != nil {
			return rep, err
		}
		r := bufio.NewReaderSize(fh, 64<<10)
		for n := 1; ; n++ {
			line, rerr := readLine(r)
			if rerr != nil && rerr != io.EOF {
				_ = fh.Close()
				return rep, fmt.Errorf("%s:%d: %w", f, n, rerr)
			}
			if len(line) > 0 {
				rep.Lines++
				k, err := sc.Scan(ctx, string(line))
				if err != nil {
					_ = fh.Close()
					return rep, fmt.Errorf("%s:%d: %w", f, n, err)
				}
				if k = nonZero(k); len(k) > 0 {
					rel, _ := filepath.Rel(dir, f)
					rep.Findings = append(rep.Findings, fmt.Sprintf("%s:%d: %s", rel, n, kindList(k)))
				}
			}
			if rerr == io.EOF {
				break
			}
		}
		_ = fh.Close()
	}
	return rep, nil
}

func nonZero(k map[string]int) map[string]int {
	out := map[string]int{}
	for kind, v := range k {
		if v > 0 {
			out[kind] = v
		}
	}
	return out
}
