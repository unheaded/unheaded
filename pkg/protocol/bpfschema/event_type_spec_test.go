// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpfschema

import (
	"regexp"
	"strings"
	"testing"
)

// goEventTypes names every Event* constant by its foundation-06 registry name.
var goEventTypes = map[string]uint8{
	"BORN":     EventBorn,
	"COMPUTED": EventComputed,
	"WOTAN_RD": EventWotanRd,
	"WOTAN_WR": EventWotanWr,
	"CHAOS":    EventChaos,
	"ROLLBACK": EventRollback,
	"DIED":     EventDied,
	"KEY_OP":   EventKeyOp,
	"ANOMALY":  EventAnomaly,
}

// specEventTypes parses "EVENT_NAME (0xNN)" rows of the foundation-06
// Anamnesis Event Type Registry.
func specEventTypes(t *testing.T) map[string]uint8 {
	t.Helper()
	row := regexp.MustCompile(`^\s*EVENT_([A-Z_]+) \((0x[0-9A-Fa-f]{2})\)`)
	out := map[string]uint8{}
	for _, line := range specSection(t, "Anamnesis Event Type Registry") {
		m := row.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out[m[1]] = parseHexByte(t, m[2])
	}
	if len(out) != 9 {
		t.Fatalf("parsed %d event-type rows from foundation-06, want 9: %v", len(out), out)
	}
	return out
}

// TestEventTypeValuesMatchSpec: every Event* constant equals its
// foundation-06 registry value and every row has a constant (S-TRUE-WIRE
// Step 89, M-141).
func TestEventTypeValuesMatchSpec(t *testing.T) {
	spec := specEventTypes(t)
	for name, want := range spec {
		got, ok := goEventTypes[name]
		if !ok {
			t.Errorf("foundation-06 EVENT_%s (0x%02x) has no Go constant", name, want)
			continue
		}
		if got != want {
			t.Errorf("EVENT_%s = 0x%02x in Go, foundation-06 says 0x%02x", name, got, want)
		}
	}
	// The deprecated names follow the spec names they map to.
	if EventBirth != EventBorn || EventHop != EventComputed || EventDeath != EventDied {
		t.Errorf("deprecated aliases: Birth 0x%02x Hop 0x%02x Death 0x%02x, want Born/Computed/Died",
			EventBirth, EventHop, EventDeath)
	}
}

// TestEventTypeRustMatchesSpec: the Rust EventType enum carries the spec
// values under the spec names (CamelCase).
func TestEventTypeRustMatchesSpec(t *testing.T) {
	spec := specEventTypes(t)
	body := rustBlock(t, monadCommonSource(t), regexp.MustCompile(`pub enum EventType\s*\{`))
	variant := regexp.MustCompile(`(?m)^\s*([A-Z][A-Za-z]+) = (0x[0-9A-Fa-f]{2}),`)
	got := map[string]uint8{}
	for _, m := range variant.FindAllStringSubmatch(body, -1) {
		got[strings.ToUpper(camelToSnake(m[1]))] = parseHexByte(t, m[2])
	}
	for name, want := range spec {
		v, ok := got[name]
		if !ok {
			t.Errorf("Rust EventType has no variant for EVENT_%s", name)
			continue
		}
		if v != want {
			t.Errorf("Rust EventType for EVENT_%s = 0x%02x, foundation-06 says 0x%02x", name, v, want)
		}
	}
	if len(got) != len(spec) {
		t.Errorf("Rust EventType has %d variants, the registry %d: %v", len(got), len(spec), got)
	}
}

// camelToSnake turns WotanRd into Wotan_Rd.
func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestEventTypeLegacy: objects built before S-TRUE-WIRE emitted Birth 0x01,
// Hop 0x02, Death 0x03; Chaos 0x04 and Anomaly 0x08 already matched (E4).
func TestEventTypeLegacy(t *testing.T) {
	cases := []struct {
		legacy uint8
		want   uint8
		ok     bool
	}{
		{0x01, EventBorn, true},
		{0x02, EventComputed, true},
		{0x03, EventDied, true},
		{0x04, EventChaos, true},
		{0x08, EventAnomaly, true},
		{0x00, 0, false},
		{0x05, 0, false},
		{0x06, 0, false},
		{0x07, 0, false},
		{0xFF, 0, false},
	}
	for _, c := range cases {
		got, ok := EventTypeLegacy(c.legacy)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("EventTypeLegacy(0x%02x) = (0x%02x, %v), want (0x%02x, %v)", c.legacy, got, ok, c.want, c.ok)
		}
	}
}
