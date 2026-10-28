// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpfschema

import (
	"regexp"
	"strings"
	"testing"
)

// goFlowActions names every Action* constant by its foundation-06 registry name.
var goFlowActions = map[string]uint8{
	"FORWARD":        ActionForward,
	"TRACE":          ActionTrace,
	"SAMPLE":         ActionSample,
	"DROP":           ActionDrop,
	"MIRROR":         ActionMirror,
	"RATE_LIMIT":     ActionRateLimit,
	"REDIRECT":       ActionRedirect,
	"INJECT":         ActionInject,
	"KEY_ROTATE":     ActionKeyRotate,
	"KEY_REVOKE":     ActionKeyRevoke,
	"KEY_DISTRIBUTE": ActionKeyDistribute,
	"KEY_VERIFY":     ActionKeyVerify,
	"KEY_CHALLENGE":  ActionKeyChallenge,
}

// specFlowActions parses the foundation-06 "Monad Flow Action Registry" rows
// of the form "0xNN  NAME  description".
func specFlowActions(t *testing.T) map[string]uint8 {
	t.Helper()
	row := regexp.MustCompile(`^(0x[0-9A-Fa-f]{2})\s+([A-Z][A-Z_]+)\s`)
	out := map[string]uint8{}
	for _, line := range specSection(t, "Monad Flow Action Registry") {
		m := row.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out[m[2]] = parseHexByte(t, m[1])
	}
	if len(out) < 5 {
		t.Fatalf("parsed %d flow-action rows from foundation-06, want >= 5: %v", len(out), out)
	}
	return out
}

// TestFlowActionValuesMatchSpec: every Action* constant equals its
// foundation-06 registry value, and every registry row has a constant
// (S-TRUE-WIRE Step 82, M-140; RFC-ALIGNMENT-FINDINGS row 5).
func TestFlowActionValuesMatchSpec(t *testing.T) {
	spec := specFlowActions(t)
	if spec["FORWARD"] != 0x00 {
		t.Fatalf("foundation-06 FORWARD = 0x%02x; the registry parse is reading the wrong table", spec["FORWARD"])
	}
	for name, want := range spec {
		got, ok := goFlowActions[name]
		if !ok {
			t.Errorf("foundation-06 flow action %s (0x%02x) has no Go constant", name, want)
			continue
		}
		if got != want {
			t.Errorf("flow action %s = 0x%02x, foundation-06 says 0x%02x", name, got, want)
		}
	}
	for name := range goFlowActions {
		if _, ok := spec[name]; !ok {
			t.Errorf("Go flow action %s is not in the foundation-06 registry", name)
		}
	}
}

// TestFlowActionRustMatchesSpec: the Rust flow_action module carries the
// same values (ebpf/monad-common/src/lib.rs).
func TestFlowActionRustMatchesSpec(t *testing.T) {
	spec := specFlowActions(t)
	body := rustBlock(t, monadCommonSource(t), regexp.MustCompile(`pub mod flow_action\s*\{`))
	konst := regexp.MustCompile(`pub const ([A-Z_]+): u8 = (0x[0-9A-Fa-f]{2});`)
	seen := 0
	for _, m := range konst.FindAllStringSubmatch(body, -1) {
		want, ok := spec[m[1]]
		if !ok {
			continue // helper constants are not registry rows
		}
		seen++
		if got := parseHexByte(t, m[2]); got != want {
			t.Errorf("Rust flow_action::%s = 0x%02x, foundation-06 says 0x%02x", m[1], got, want)
		}
	}
	if seen != len(spec) {
		t.Errorf("Rust flow_action has %d of the %d registry rows", seen, len(spec))
	}
}

// TestFlowActionLegacy: the pre-S-TRUE-WIRE numbering (FORWARD 0x01 ..
// DROP 0x05) decodes to the spec values for one release.
func TestFlowActionLegacy(t *testing.T) {
	cases := []struct {
		legacy uint8
		want   uint8
		ok     bool
	}{
		{0x01, ActionForward, true},
		{0x02, ActionTrace, true},
		{0x03, ActionSample, true},
		{0x04, ActionMirror, true},
		{0x05, ActionDrop, true},
		{0x00, 0, false},
		{0x06, 0, false},
		{0xFF, 0, false},
	}
	for _, c := range cases {
		got, ok := FlowActionLegacy(c.legacy)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("FlowActionLegacy(0x%02x) = (0x%02x, %v), want (0x%02x, %v)", c.legacy, got, ok, c.want, c.ok)
		}
	}
}

// TestFlowActionName covers every registry row.
func TestFlowActionName(t *testing.T) {
	for name, v := range goFlowActions {
		if got := FlowActionName(v); got != strings.ToLower(name) {
			t.Errorf("FlowActionName(0x%02x) = %q, want %q", v, got, strings.ToLower(name))
		}
	}
	if got := FlowActionName(0x08); got != "unknown(8)" {
		t.Errorf("FlowActionName(0x08) = %q, want unknown(8)", got)
	}
}
