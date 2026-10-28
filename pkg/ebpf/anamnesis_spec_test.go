// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
)

// S-TRUE-WIRE Steps 89-91: Anamnesis event types follow the foundation-06
// registry (the E4 rule: the spec wins). pkg/protocol/bpfschema checks the
// same values against the draft text; this file checks the decoder.

func TestEventType_FoundationValues(t *testing.T) {
	want := map[EventType]uint8{
		EventBorn: 0x00, EventComputed: 0x01, EventWotanRd: 0x02, EventWotanWr: 0x03,
		EventChaos: 0x04, EventRollback: 0x05, EventDied: 0x06, EventKeyOp: 0x07, EventAnomaly: 0x08,
	}
	for et, v := range want {
		if uint8(et) != v {
			t.Errorf("%s = 0x%02x, want 0x%02x", et, uint8(et), v)
		}
		if !et.Valid() {
			t.Errorf("%s.Valid() = false", et)
		}
	}
	// The pre-sprint names stay as aliases for one release.
	if EventBirth != EventBorn || EventHop != EventComputed || EventDeath != EventDied {
		t.Error("deprecated aliases do not point at the spec values")
	}
}

func spec32(ts uint64, typ uint8) []byte {
	b := make([]byte, AnamnesisEventSize)
	binary.LittleEndian.PutUint64(b[0:8], ts)
	b[8] = typ
	b[12] = MonadVersion
	return b
}

func TestDecodeAnamnesisEvent_SpecNumbering(t *testing.T) {
	for typ, want := range map[uint8]EventType{0x00: EventBorn, 0x01: EventComputed, 0x06: EventDied, 0x04: EventChaos, 0x08: EventAnomaly} {
		ev, err := DecodeAnamnesisEvent(spec32(1000, typ))
		if err != nil {
			t.Fatalf("type 0x%02x: %v", typ, err)
		}
		if ev.EventType != want {
			t.Errorf("type 0x%02x decoded as %s, want %s", typ, ev.EventType, want)
		}
	}
	if _, err := DecodeAnamnesisEvent(spec32(1000, 0x09)); !errors.Is(err, ErrEventUnknownType) {
		t.Errorf("type 0x09: %v, want ErrEventUnknownType", err)
	}
}

// BORN is 0x00, so an all-zero record would read as a birth: a record with
// no timestamp is refused instead (bpf_ktime_get_ns is never 0 on a live
// kernel).
func TestDecodeAnamnesisEvent_ZeroTimestampRefused(t *testing.T) {
	if _, err := DecodeAnamnesisEvent(make([]byte, AnamnesisEventSize)); !errors.Is(err, ErrEventZeroTimestamp) {
		t.Fatalf("all-zero record: %v, want ErrEventZeroTimestamp", err)
	}
	if _, err := DecodeAnamnesisEventLegacy(make([]byte, AnamnesisEventSize)); !errors.Is(err, ErrEventZeroTimestamp) {
		t.Fatalf("all-zero legacy record: %v, want ErrEventZeroTimestamp", err)
	}
}

func TestDecodeAnamnesisEventLegacy(t *testing.T) {
	for typ, want := range map[uint8]EventType{0x01: EventBorn, 0x02: EventComputed, 0x03: EventDied, 0x04: EventChaos, 0x08: EventAnomaly} {
		ev, err := DecodeAnamnesisEventLegacy(spec32(1000, typ))
		if err != nil {
			t.Fatalf("legacy type 0x%02x: %v", typ, err)
		}
		if ev.EventType != want {
			t.Errorf("legacy type 0x%02x decoded as %s, want %s", typ, ev.EventType, want)
		}
	}
	// The old numbering never defined these.
	for _, typ := range []uint8{0x00, 0x05, 0x06, 0x07, 0x09} {
		if _, err := DecodeAnamnesisEventLegacy(spec32(1000, typ)); !errors.Is(err, ErrEventUnknownType) {
			t.Errorf("legacy type 0x%02x: %v, want ErrEventUnknownType", typ, err)
		}
	}
}

// The JSON names are an API (dashboard/js/packet-flow.js, Wotan topics):
// the three renumbered types keep their names; the new ones get spec names.
func TestEventType_JSONNamesStable(t *testing.T) {
	for et, name := range map[EventType]string{
		EventBorn: "birth", EventComputed: "hop", EventDied: "death", EventChaos: "chaos", EventAnomaly: "anomaly",
		EventWotanRd: "wotan_rd", EventWotanWr: "wotan_wr", EventRollback: "rollback", EventKeyOp: "key_op",
	} {
		b, err := json.Marshal(et)
		if err != nil || string(b) != `"`+name+`"` {
			t.Errorf("%d marshals to %s (%v), want %q", uint8(et), b, err, name)
		}
		var back EventType
		if err := json.Unmarshal(b, &back); err != nil || back != et {
			t.Errorf("%q unmarshals to %d (%v), want %d", name, back, err, et)
		}
	}
	// The spec spellings are accepted on input too.
	for name, et := range map[string]EventType{"born": EventBorn, "computed": EventComputed, "died": EventDied} {
		var got EventType
		if err := json.Unmarshal([]byte(`"`+name+`"`), &got); err != nil || got != et {
			t.Errorf("%q unmarshals to %d (%v), want %d", name, got, err, et)
		}
	}
}

// The loader knows which object version it loaded, so the legacy decode is
// chosen per ring source, never guessed from the byte (the numberings
// overlap: legacy BIRTH 0x01 is spec COMPUTED).
func TestAnamnesisReader_LegacySource(t *testing.T) {
	raw := make(chan []byte, 2)
	raw <- spec32(1000, 0x01) // legacy BIRTH
	raw <- spec32(2000, 0x03) // legacy DEATH
	close(raw)
	r := NewAnamnesisReader(raw, 4).WithLegacyEventTypes()
	var got []EventType
	for ev := range r.Poll(context.Background()) {
		got = append(got, ev.EventType)
	}
	if len(got) != 2 || got[0] != EventBorn || got[1] != EventDied {
		t.Fatalf("legacy source decoded %v, want [birth death]", got)
	}
	if s := r.Stats(); s.LegacyDecodes != 2 {
		t.Fatalf("LegacyDecodes = %d, want 2", s.LegacyDecodes)
	}
}
