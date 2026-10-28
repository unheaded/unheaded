// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package errors

import (
	"bufio"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// registryRow is one single-code row of the registry table.
type registryRow struct {
	code   uint8
	level  string
	name   string
	action string
}

var mdRow = regexp.MustCompile(`^\|\s*0x([0-9A-Fa-f]{2})\s*\|\s*([^|]+?)\s*\|\s*([A-Z0-9_]+)\s*\|\s*[^|]*\|\s*([^|]+?)\s*\|\s*$`)

// readRegistryMD parses the single-code rows of Section 1's table in
// docs/protocol/error-registry.md. Range rows (0x0E-0x1E) do not match.
func readRegistryMD(t *testing.T) []registryRow {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "docs", "protocol", "error-registry.md"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer f.Close()
	var rows []registryRow
	inTable := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "### Error Code Table") {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(line, "### ") {
			break
		}
		if !inTable {
			continue
		}
		m := mdRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		v, err := strconv.ParseUint(m[1], 16, 8)
		if err != nil {
			t.Fatalf("bad code %q: %v", m[1], err)
		}
		rows = append(rows, registryRow{code: uint8(v), level: m[2], name: m[3], action: m[4]})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

var draftRow = regexp.MustCompile(`^\s+([A-Z0-9_]+) \(0x([0-9A-Fa-f]{2})\)`)

// readDraftRegistry parses foundation-06 §Error Code Registry initial entries.
func readDraftRegistry(t *testing.T) map[uint8]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "protocol",
		"draft-bellis-unheaded-protocol-foundation-06.md"))
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	out := map[uint8]string{}
	in := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "## Error Code Registry") {
			in = true
			continue
		}
		if in && strings.HasPrefix(line, "## ") {
			break
		}
		if !in {
			continue
		}
		if m := draftRow.FindStringSubmatch(line); m != nil {
			v, _ := strconv.ParseUint(m[2], 16, 8)
			out[uint8(v)] = m[1]
		}
	}
	return out
}

// TestRegistryParity: every single-code row of error-registry.md has a Go
// constant with the same number, name and level, and the md agrees with
// foundation-06 (which wins on conflict).
func TestRegistryParity(t *testing.T) {
	rows := readRegistryMD(t)
	// 0x00-0x0D plus the two greasing values.
	if len(rows) != 16 {
		t.Fatalf("parsed %d single-code rows from error-registry.md, want 16", len(rows))
	}
	draft := readDraftRegistry(t)
	if len(draft) != 14 {
		t.Fatalf("parsed %d entries from foundation-06, want 14 (0x00-0x0D)", len(draft))
	}
	for _, r := range rows {
		c := Code(r.code)
		if want, ok := draft[r.code]; ok && want != r.name {
			t.Errorf("0x%02x: error-registry.md says %s, foundation-06 says %s", r.code, r.name, want)
		}
		if got := c.String(); got != r.name {
			t.Errorf("Code(0x%02x).String() = %q, registry %q", r.code, got, r.name)
		}
		if got := c.Level().String(); got != r.level {
			t.Errorf("Code(0x%02x).Level() = %q, registry %q", r.code, got, r.level)
		}
		if info, ok := Lookup(c); r.code <= 0x0D && (!ok || info.RecommendedAction != r.action) {
			t.Errorf("Lookup(0x%02x) action = %q (ok=%v), registry %q", r.code, info.RecommendedAction, ok, r.action)
		}
	}
	for code := range draft {
		if _, ok := Lookup(Code(code)); !ok {
			t.Errorf("foundation-06 code 0x%02x has no Lookup entry", code)
		}
	}
}

// TestNamedConstants pins the exported symbols to the registry numbers so a
// renumbering shows up as a compile or test failure, not a silent drift.
func TestNamedConstants(t *testing.T) {
	cases := []struct {
		c    Code
		want uint8
		name string
	}{
		{NoError, 0x00, "NO_ERROR"},
		{CRCValidationFailed, 0x01, "CRC_VALIDATION_FAILED"},
		{VersionNotSupported, 0x02, "VERSION_NOT_SUPPORTED"},
		{FlowLabelInvalid, 0x03, "FLOW_LABEL_INVALID"},
		{ArithmeticOverflow, 0x04, "ARITHMETIC_OVERFLOW"},
		{WotanBoundsCheckFailed, 0x05, "WOTAN_BOUNDS_CHECK_FAILED"},
		{MultipleHbHHeaders, 0x06, "MULTIPLE_HBH_HEADERS"},
		{UnknownCriticalTLV, 0x07, "UNKNOWN_CRITICAL_TLV"},
		{WALSeqnoDiscontinuity, 0x08, "WAL_SEQNO_DISCONTINUITY"},
		{InsufficientBufferSpace, 0x09, "INSUFFICIENT_BUFFER_SPACE"},
		{FlowStateCorruption, 0x0A, "FLOW_STATE_CORRUPTION"},
		{TLSHandshakeFailure, 0x0B, "TLS_HANDSHAKE_FAILURE"},
		{QUICVersionMismatch, 0x0C, "QUIC_VERSION_MISMATCH"},
		{Reserved0D, 0x0D, "RESERVED"},
		{Greasing0, 0x1F, "GREASING_0"},
		{Greasing1, 0xFF, "GREASING_1"},
	}
	for _, tc := range cases {
		if uint8(tc.c) != tc.want {
			t.Errorf("%s = 0x%02x, want 0x%02x", tc.name, uint8(tc.c), tc.want)
		}
		if tc.c.String() != tc.name {
			t.Errorf("0x%02x String() = %q, want %q", tc.want, tc.c.String(), tc.name)
		}
	}
}

func TestRangeClassification(t *testing.T) {
	for v := 0; v <= 0xFF; v++ {
		c := Code(v)
		name := c.String()
		switch {
		case v <= 0x0D, v == 0x1F, v == 0xFF:
			if strings.HasPrefix(name, "UNALLOCATED") || strings.HasPrefix(name, "PRIVATE_USE") {
				t.Errorf("0x%02x classified as %s", v, name)
			}
		case v >= 0x0E && v <= 0x1E:
			if name != fmt.Sprintf("UNALLOCATED(0x%02x)", v) {
				t.Errorf("0x%02x = %s, want UNALLOCATED", v, name)
			}
			if c.IsStandard() {
				t.Errorf("0x%02x IsStandard", v)
			}
		default:
			if name != fmt.Sprintf("PRIVATE_USE(0x%02x)", v) {
				t.Errorf("0x%02x = %s, want PRIVATE_USE", v, name)
			}
		}
	}
	if !Greasing0.IsGrease() || !Greasing1.IsGrease() || CRCValidationFailed.IsGrease() {
		t.Error("IsGrease classification wrong")
	}
	if !QUICVersionMismatch.IsStandard() || Reserved0D.IsStandard() || !NoError.IsStandard() {
		t.Error("IsStandard: 0x00-0x0C are standard, 0x0D is reserved")
	}
}

func TestProtocolError(t *testing.T) {
	cause := stderrors.New("boom")
	err := Wrap(CRCValidationFailed, "monad checksum", cause)
	if got := err.Error(); got != "[Flow] 0x01 CRC_VALIDATION_FAILED: monad checksum: boom" {
		t.Errorf("Error() = %q", got)
	}
	if !stderrors.Is(err, cause) {
		t.Error("Unwrap lost the cause")
	}
	var pe *ProtocolError
	if !stderrors.As(error(err), &pe) || pe.Code() != CRCValidationFailed {
		t.Error("errors.As / Code() failed")
	}
	if got := New(Code(0x42), "vendor").Error(); got != "[N/A] 0x42 PRIVATE_USE(0x42): vendor" {
		t.Errorf("private-use Error() = %q", got)
	}
}
