// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package metricstest checks Prometheus text exposition in tests.
//
// It exists because each service that hand-wrote /metrics also hand-wrote
// its own checker, and the checkers disagreed: monad's rejected a summary's
// _sum and _count, which the text format requires. One checker, used
// everywhere, means one reading of the format.
package metricstest

import (
	"fmt"
	"strings"
)

var validTypes = map[string]bool{"counter": true, "gauge": true, "histogram": true, "summary": true, "untyped": true}

// Lint returns every problem in an exposition page, or nil if there is none:
// an unknown or malformed TYPE, a HELP or TYPE line given twice for one
// metric, and a sample whose family has no TYPE declared before it.
func Lint(body string) []string {
	var problems []string
	declared := map[string]string{} // family -> type
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, "# HELP "), strings.HasPrefix(line, "# TYPE "):
			f := strings.Fields(line)
			if len(f) < 3 {
				problems = append(problems, fmt.Sprintf("malformed line %q", line))
				continue
			}
			key := f[1] + " " + f[2]
			if seen[key] {
				problems = append(problems, fmt.Sprintf("# %s for %s appears twice", f[1], f[2]))
			}
			seen[key] = true
			if f[1] == "TYPE" {
				if len(f) != 4 || !validTypes[f[3]] {
					problems = append(problems, fmt.Sprintf("bad TYPE line %q", line))
					continue
				}
				declared[f[2]] = f[3]
			}
		case strings.HasPrefix(line, "#"):
		default:
			f := strings.Fields(line)
			if len(f) < 2 {
				problems = append(problems, fmt.Sprintf("malformed sample line %q", line))
				continue
			}
			name := strings.SplitN(f[0], "{", 2)[0]
			if !SampleDeclared(declared, name) {
				problems = append(problems, fmt.Sprintf("sample %s has no preceding # TYPE", name))
			}
		}
	}
	return problems
}

// SampleDeclared reports whether a sample name belongs to a declared family.
// Summaries publish <name>_sum and <name>_count beside their quantiles, and
// histograms publish <name>_bucket, _sum and _count. Those suffixes are valid
// only for those types: a _bucket under a summary is still refused.
func SampleDeclared(declared map[string]string, sample string) bool {
	if _, ok := declared[sample]; ok {
		return true
	}
	for suffix, types := range map[string][]string{
		"_sum":    {"summary", "histogram"},
		"_count":  {"summary", "histogram"},
		"_bucket": {"histogram"},
	} {
		family := strings.TrimSuffix(sample, suffix)
		if family == sample {
			continue
		}
		for _, t := range types {
			if declared[family] == t {
				return true
			}
		}
	}
	return false
}
