// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package topicpattern

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// S-TRUE-WIRE Step 162 (WOT-09): an executable witness for wotan-memory-03
// §"Wotan Topic Namespace Registry": "A registered name ending in ".*"
// reserves every name with exactly one further segment." Every registered
// ".*" entry in the draft and its IETF twin matches one further segment
// and nothing else, as Wotan's pattern matcher (HTTP and gRPC) reads it.
func TestDraftTopicRegistryReservationSemantics(t *testing.T) {
	entry := regexp.MustCompile(`(?m)^  ([a-z][a-z0-9_.*-]*)\s{2,}\S`)
	for _, p := range []string{
		"../../../../docs/protocol/draft-bellis-unheaded-wotan-memory-03.md",
		"../../../../ietf-submission/draft-bellis-unheaded-wotan-memory-00.md",
	} {
		b, err := os.ReadFile(p) // #nosec G304 -- fixed relative test path
		if err != nil {
			t.Fatal(err)
		}
		doc := string(b)
		i := strings.Index(doc, "Registry Name: Wotan Topic Namespace")
		if i < 0 {
			t.Fatalf("%s: topic namespace registry not found", p)
		}
		reg := doc[i:]
		reg = reg[:strings.Index(reg, "~~~~~")]
		var wild, exact int
		for _, m := range entry.FindAllStringSubmatch(reg, -1) {
			name := m[1]
			if name == "topic" {
				continue
			}
			base, isWild := strings.CutSuffix(name, ".*")
			if !isWild {
				exact++
				if !Match(name, name) || Match(name, name+".x") {
					t.Errorf("%s: exact entry %q", p, name)
				}
				continue
			}
			wild++
			if !Match(name, base+".x") || !Match(name, base+".slot-7") {
				t.Errorf("%s: %q does not reserve one further segment", p, name)
			}
			if Match(name, base) || Match(name, base+".a.b") || Match(name, base+"x.y") {
				t.Errorf("%s: %q reserves more than one further segment", p, name)
			}
		}
		if wild < 2 || exact < 1 {
			t.Fatalf("%s: parsed %d wildcard and %d exact entries", p, wild, exact)
		}
	}
}
