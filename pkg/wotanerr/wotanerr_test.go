// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package wotanerr

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The draft and its IETF twin carry the same tables; both are checked so
// neither drifts from the code.
var draftPaths = []string{
	"../../docs/protocol/draft-bellis-unheaded-wotan-memory-03.md",
	"../../ietf-submission/draft-bellis-unheaded-wotan-memory-00.md",
}

func readDraft(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- fixed relative test path
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	return string(b)
}

// section returns the text after the heading line up to the next heading of
// the same or a higher level.
func section(t *testing.T, doc, heading string) string {
	t.Helper()
	i := strings.Index(doc, "\n"+heading+"\n")
	if i < 0 {
		t.Fatalf("heading %q not found", heading)
	}
	rest := doc[i+1+len(heading):]
	level := len(strings.Fields(heading)[0])
	re := regexp.MustCompile(`(?m)^#{1,` + strconv.Itoa(level) + `} `)
	if loc := re.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	return rest
}

// between returns text from the first start marker to the following end
// marker.
func between(t *testing.T, text, start, end string) string {
	t.Helper()
	i := strings.Index(text, start)
	if i < 0 {
		t.Fatalf("marker %q not found", start)
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		t.Fatalf("marker %q not found after %q", end, start)
	}
	return text[i : i+j]
}

// codeRows returns (code, name) for every table row starting with one code
// ("0x0A"); a range row ("0x0D-0x1E") does not match.
func codeRows(text string) map[uint64]string {
	rows := map[uint64]string{}
	for _, m := range hexRow.FindAllStringSubmatch(text, -1) {
		v, err := strconv.ParseUint(strings.TrimPrefix(m[1], "0x"), 16, 8)
		if err != nil {
			continue
		}
		rows[v] = m[2]
	}
	return rows
}

var (
	severityRow = regexp.MustCompile(`(?m)^([A-Z]+)\s+([0-4])\s+(\S.*?\S)\s{2,}(\S.*)$`)
	hexRow      = regexp.MustCompile(`(?m)^(0x[0-9A-F]{2})\s+([A-Z][A-Z0-9_]+)\s`)
)

func TestSeverityTableMatchesDraft(t *testing.T) {
	for _, p := range draftPaths {
		sec := section(t, readDraft(t, p), "## Error Severity Levels")
		got := map[Severity]bool{}
		for _, m := range severityRow.FindAllStringSubmatch(sec, -1) {
			n, _ := strconv.Atoi(m[2])
			s := Severity(n)
			if s.String() != m[1] {
				t.Errorf("%s: severity %d is %q in the draft, %q in code", p, n, m[1], s.String())
			}
			if s.Recovery() != strings.TrimSpace(m[4]) {
				t.Errorf("%s: severity %s action %q in the draft, %q in code", p, m[1], m[4], s.Recovery())
			}
			got[s] = true
		}
		if len(got) != 5 {
			t.Fatalf("%s: parsed %d severity rows, want 5", p, len(got))
		}
	}
}

func TestOriginTableMatchesDraft(t *testing.T) {
	for _, p := range draftPaths {
		sec := section(t, readDraft(t, p), "## Structured Error Code Format")
		rows := codeRows(between(t, sec, "Origin Code", "Category:"))
		if len(rows) != len(Origins()) {
			t.Fatalf("%s: draft has %d origins, code has %d", p, len(rows), len(Origins()))
		}
		for v, name := range rows {
			if o := Origin(v); !o.Defined() || o.String() != name {
				t.Errorf("%s: origin 0x%02X is %s in the draft, %q in code", p, v, name, o.String())
			}
		}
	}
}

func TestCategoryTableMatchesDraft(t *testing.T) {
	for _, p := range draftPaths {
		sec := section(t, readDraft(t, p), "## Structured Error Code Format")
		rows := codeRows(between(t, sec, "Category    Name", "Detail:"))
		if len(rows) != len(Categories()) {
			t.Fatalf("%s: draft has %d categories, code has %d", p, len(rows), len(Categories()))
		}
		for v, name := range rows {
			if c := Category(v); !c.Defined() || c.String() != name {
				t.Errorf("%s: category 0x%02X is %s in the draft, %q in code", p, v, name, c.String())
			}
		}
	}
}

// Every bracketed code the draft writes ([ERROR, GRPC, ACCESS, 0x01]) parses
// with the draft's own abbreviations, and prints back to the same code.
func TestDraftBracketCodesParse(t *testing.T) {
	re := regexp.MustCompile(`\[[A-Z]+, [A-Z0-9_]+, [A-Z_]+, 0x[0-9A-F]{2}\]`)
	for _, p := range draftPaths {
		found := re.FindAllString(readDraft(t, p), -1)
		if len(found) < 30 {
			t.Fatalf("%s: only %d bracketed codes found", p, len(found))
		}
		for _, s := range found {
			c, err := ParseBracket(s)
			if err != nil {
				t.Errorf("%s: %s: %v", p, s, err)
				continue
			}
			back, err := ParseBracket(c.String())
			if err != nil || back != c {
				t.Errorf("%s: %s -> %s -> %v %v", p, s, c, back, err)
			}
		}
	}
	// The draft's gRPC row for an unauthorized publish is the code Wotan sends.
	c, err := ParseBracket("[ERROR, GRPC, ACCESS, 0x01]")
	if err != nil || c != UnauthorizedPublish {
		t.Fatalf("unauthorized publish = %v %v, want %v", c, err, UnauthorizedPublish)
	}
	for _, bad := range []string{"", "[]", "[ERROR, GRPC, ACCESS]", "[BOGUS, GRPC, ACCESS, 0x01]", "[ERROR, NOPE, ACCESS, 0x01]",
		"[ERROR, GRPC, NOPE, 0x01]", "[ERROR, GRPC, ACCESS, 0x100]", "[ERROR, GRPC, ACCESS, 1]", "ERROR, GRPC, ACCESS, 0x01"} {
		if c, err := ParseBracket(bad); err == nil {
			t.Errorf("ParseBracket(%q) = %v, want an error", bad, c)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	n := 0
	for s := SeverityInfo; s <= SeverityFatal; s++ {
		for _, o := range Origins() {
			for _, c := range Categories() {
				for _, d := range []uint8{0x00, 0x01, 0x7F, 0xFF} {
					in := Code{Severity: s, Origin: o, Category: c, Detail: d}
					out, err := Decode(in.Encode())
					if err != nil || out != in {
						t.Fatalf("%v -> 0x%08X -> %v %v", in, in.Encode(), out, err)
					}
					n++
				}
			}
		}
	}
	if n != 5*14*13*4 {
		t.Fatalf("tried %d codes", n)
	}
	// Bit layout from the draft's field text and its example decoder:
	// severity 31-29, origin 28-24, category 23-16, detail 7-0; bits 15-8
	// carry nothing. (The draft's figure draws four byte-wide columns,
	// which disagrees with its own text; reported to the RFC Editor.)
	if got := (Code{SeverityError, OriginGRPC, CategoryAccessControl, 0x01}).Encode(); got != 0x44010001 {
		t.Fatalf("layout: got 0x%08X, want 0x44010001", got)
	}
	if got := (Code{SeverityFatal, OriginVendorSpecific, CategoryVendorSpecific, 0xFF}).Encode(); got != 0x9FFF00FF {
		t.Fatalf("layout: got 0x%08X, want 0x9FFF00FF", got)
	}
	if c, err := Decode(0x44010101); err == nil {
		t.Fatalf("Decode with bits 15-8 set = %v, want an error", c)
	}
}

func TestDecodeRefusesUndefined(t *testing.T) {
	for _, v := range []uint32{
		5 << 29, 7 << 29, // severities 5-7 are undefined
		0x0D << 24, 0x1E << 24, // reserved origins
		0x0C << 16, 0xFE << 16, // reserved categories
	} {
		if c, err := Decode(v); err == nil {
			t.Errorf("Decode(0x%08X) = %v, want an error", v, c)
		}
	}
}

func TestHeaderRoundTripAndHostileInput(t *testing.T) {
	h := http.Header{}
	SetHeader(h, SignatureRequired)
	if v := h.Get(HeaderName); v != fmt.Sprintf("0x%08X", SignatureRequired.Encode()) {
		t.Fatalf("header %q", v)
	}
	c, ok := FromHeader(h)
	if !ok || *c != SignatureRequired {
		t.Fatalf("FromHeader = %v %v", c, ok)
	}
	for _, bad := range []string{"", "0x", "44010001", "0x4401000", "0x440100011", "0xZZ010001", "0xA4010001",
		" 0x44010001", "0x-4010001", "0x+4010001", strings.Repeat("0", 1<<16)} {
		if c, err := ParseHex(bad); err == nil {
			t.Errorf("ParseHex(%q) = %v, want an error", bad, c)
		}
	}
	if _, ok := FromHeader(http.Header{HeaderName: {"nonsense"}}); ok {
		t.Fatal("a malformed header decoded")
	}
	if _, ok := FromHeader(http.Header{}); ok {
		t.Fatal("an absent header decoded")
	}
}

func TestErrorsAs(t *testing.T) {
	base := errors.New("subscriber not approved")
	err := Wrap(base, TopicAccessDenied)
	if !errors.Is(err, base) {
		t.Fatal("Wrap lost the base error")
	}
	var c *Code
	if !errors.As(err, &c) || *c != TopicAccessDenied {
		t.Fatalf("errors.As = %v", c)
	}
	if !strings.Contains(err.Error(), "subscriber not approved") || !strings.Contains(err.Error(), "ACCESS_CONTROL") {
		t.Fatalf("message %q", err.Error())
	}
	if Wrap(nil, TopicAccessDenied) != nil {
		t.Fatal("Wrap(nil) must be nil")
	}
	if c.Recovery() != SeverityError.Recovery() {
		t.Fatal("Code.Recovery must follow its severity")
	}
}

// The codes Wotan emits are all defined and distinct.
func TestEmittedCodesDistinct(t *testing.T) {
	seen := map[uint32]Code{}
	for _, c := range Emitted() {
		if err := c.Validate(); err != nil {
			t.Errorf("%v: %v", c, err)
		}
		if prev, dup := seen[c.Encode()]; dup {
			t.Errorf("%v duplicates %v", c, prev)
		}
		seen[c.Encode()] = c
	}
	if len(seen) < 4 {
		t.Fatalf("only %d emitted codes", len(seen))
	}
}
