// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhencapture

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

// redactPatterns is shared with raft/zhen_mcp_server.py, which reads the
// same file, so both writers redact the same things.
//
//go:embed redact_patterns.json
var redactPatterns []byte

type pattern struct {
	name string
	re   *regexp.Regexp
	repl string
}

var patterns = mustLoadPatterns(redactPatterns)

func mustLoadPatterns(raw []byte) []pattern {
	ps, err := loadPatterns(raw)
	if err != nil {
		panic(err)
	}
	return ps
}

func loadPatterns(raw []byte) ([]pattern, error) {
	var file struct {
		Version  int `json:"version"`
		Patterns []struct {
			Name  string `json:"name"`
			Regex string `json:"regex"`
		} `json:"patterns"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("zhencapture: redact patterns: %w", err)
	}
	if file.Version != 1 || len(file.Patterns) == 0 {
		return nil, fmt.Errorf("zhencapture: redact patterns: version %d with %d patterns", file.Version, len(file.Patterns))
	}
	out := make([]pattern, 0, len(file.Patterns))
	for _, p := range file.Patterns {
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			return nil, fmt.Errorf("zhencapture: redact pattern %s: %w", p.Name, err)
		}
		// Group 1 is kept before the tag, group 2 after it. ${n} of a
		// group the pattern lacks expands to "".
		out = append(out, pattern{name: p.Name, re: re, repl: "${1}[REDACTED:" + p.Name + "]${2}"})
	}
	return out, nil
}

// Redact replaces secrets in s (API keys, tokens, private keys, URL
// passwords, password= style assignments) with [REDACTED:<kind>] and
// reports how many it replaced. It is a best-effort filter for known
// shapes, not a guarantee: captures still belong outside the repo, 0600.
func Redact(s string) (string, int) {
	n := 0
	for _, p := range patterns {
		hits := len(p.re.FindAllStringIndex(s, -1))
		if hits == 0 {
			continue
		}
		n += hits
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s, n
}
