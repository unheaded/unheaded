// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpfschema

// spec_registry_helpers_test.go reads registry rows out of foundation-06 and
// constants out of the Rust eBPF source at test time (S-TRUE-WIRE Steps 82,
// 89). The spec is the source of truth for wire values (the E4 rule): a Go or
// Rust constant that disagrees with its registry row fails the build.

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// foundationDraft is foundation-06 relative to this package.
const foundationDraft = "../../../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md"

// monadCommonSrc holds the Rust flow_action module and EventType enum.
const monadCommonSrc = ebpfRoot + "/monad-common/src/lib.rs"

// specSection returns the lines of foundation-06 from the "## <heading>"
// line up to (not including) the next "## " heading.
func specSection(t *testing.T, heading string) []string {
	t.Helper()
	f, err := os.Open(foundationDraft) // #nosec G304 -- the repo's own draft
	if err != nil {
		t.Fatalf("open %s: %v", foundationDraft, err)
	}
	defer func() { _ = f.Close() }()

	var out []string
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			if in {
				break
			}
			in = strings.TrimSpace(strings.TrimPrefix(line, "## ")) == heading
			continue
		}
		if in {
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", foundationDraft, err)
	}
	if len(out) == 0 {
		t.Fatalf("section %q not found in %s", heading, foundationDraft)
	}
	return out
}

// parseHexByte parses "0x1F" into a byte or fails the test.
func parseHexByte(t *testing.T, s string) uint8 {
	t.Helper()
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 8)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return uint8(v)
}

// monadCommonSource returns the monad-common source.
func monadCommonSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(monadCommonSrc) // #nosec G304 -- the repo's own Rust source
	if err != nil {
		t.Fatalf("read %s: %v", monadCommonSrc, err)
	}
	return string(b)
}

// rustBlock returns the body of the brace-balanced block whose opening brace
// ends the first match of start in src.
func rustBlock(t *testing.T, src string, start *regexp.Regexp) string {
	t.Helper()
	loc := start.FindStringIndex(src)
	if loc == nil {
		t.Fatalf("%s not found in %s", start, monadCommonSrc)
	}
	i := loc[1] - 1
	if src[i] != '{' {
		t.Fatalf("pattern %s must end at an opening brace", start)
	}
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[i+1 : j]
			}
		}
	}
	t.Fatalf("unbalanced block after %s", start)
	return ""
}
