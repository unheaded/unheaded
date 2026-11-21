// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/protocol/bpfschema"
)

// fakeState is the maps as they are before an apply.
type fakeState struct {
	maps    map[string]map[string][]byte
	fail    error
	touched []string
}

func (s *fakeState) put(t *testing.T, mapName string, key, value any) {
	t.Helper()
	if s.maps == nil {
		s.maps = map[string]map[string][]byte{}
	}
	if s.maps[mapName] == nil {
		s.maps[mapName] = map[string][]byte{}
	}
	s.maps[mapName][string(le(t, key))] = le(t, value)
}

func (s *fakeState) Lookup(mapName string, key []byte) ([]byte, error) {
	s.touched = append(s.touched, mapName)
	if s.fail != nil {
		return nil, s.fail
	}
	return s.maps[mapName][string(key)], nil
}

func (s *fakeState) Iterate(mapName string, fn func(key, value []byte) error) error {
	s.touched = append(s.touched, mapName)
	if s.fail != nil {
		return s.fail
	}
	for k, v := range s.maps[mapName] {
		if err := fn([]byte(k), v); err != nil {
			return err
		}
	}
	return nil
}

func hmacValidator(ns uint8) Validator  { return Validator{Namespace: ns, RequireHMAC: true} }
func retryValidator(ns uint8) Validator { return Validator{Namespace: ns, RequireRetry: true} }

var src1 = netip.MustParseAddr("fd00::1")

// FND-035: Shield stamps every packet flow 0, so a require_hmac or
// require_retry validator with no live flow-0 entry fails them all.
func TestCheckFlowZero(t *testing.T) {
	const now = 1000 * time.Second
	hmacAt := func(t *testing.T, s *fakeState, flow uint16, ns uint16, expires uint32) {
		s.put(t, bpfschema.MapHMACKeys, bpfschema.HMACKeyKey{FlowID: flow, Namespace: ns},
			bpfschema.HMACKeyValue{CreatedAt: 1, ExpiresAt: expires})
	}
	retryAt := func(t *testing.T, s *fakeState, flow uint16, expires uint32) {
		s.put(t, bpfschema.MapRetryTokens, bpfschema.RetryTokenKey{FlowID: flow, SrcLow48: [6]byte{1}},
			bpfschema.RetryTokenValue{IssuedAt: 1, ExpiresAt: expires})
	}
	cases := []struct {
		name  string
		file  File
		state func(*testing.T, *fakeState)
		want  []string // substrings of the refusal; nil: accepted
	}{
		{name: "no requirement, nothing needed",
			file: File{Validators: []Validator{{Namespace: 1, RequireCRC: true, MaxHopCount: 8}}}},
		{name: "hmac, flow 0 in the file",
			file: File{Validators: []Validator{hmacValidator(1)}, HMACKeys: []HMACKey{{Flow: 0, Namespace: 1}}}},
		{name: "hmac, flow 0 in the file on another namespace",
			file: File{Validators: []Validator{hmacValidator(1)}, HMACKeys: []HMACKey{{Flow: 0, Namespace: 2}}},
			want: []string{"validators[0]", "namespace 1", "require_hmac", "flow 0", "FND-035"}},
		{name: "hmac, only another flow in the file",
			file: File{Validators: []Validator{hmacValidator(1)}, HMACKeys: []HMACKey{{Flow: 0x2211, Namespace: 1}}},
			want: []string{"validators[0]", "require_hmac"}},
		{name: "hmac, flow 0 live in the map, never expires",
			file:  File{Validators: []Validator{hmacValidator(1)}},
			state: func(t *testing.T, s *fakeState) { hmacAt(t, s, 0, 1, 0) }},
		{name: "hmac, flow 0 live in the map, expires later",
			file:  File{Validators: []Validator{hmacValidator(1)}},
			state: func(t *testing.T, s *fakeState) { hmacAt(t, s, 0, 1, 1001) }},
		{name: "hmac, flow 0 in the map expires this second",
			file:  File{Validators: []Validator{hmacValidator(1)}},
			state: func(t *testing.T, s *fakeState) { hmacAt(t, s, 0, 1, 1000) },
			want:  []string{"validators[0]", "require_hmac"}},
		{name: "hmac, flow 0 in the map on another namespace",
			file:  File{Validators: []Validator{hmacValidator(1)}},
			state: func(t *testing.T, s *fakeState) { hmacAt(t, s, 0, 2, 0) },
			want:  []string{"validators[0]"}},
		{name: "hmac, empty map",
			file: File{Validators: []Validator{hmacValidator(1)}},
			want: []string{"validators[0]", "hop 0", "namespace 1"}},
		{name: "retry, flow 0 in the file",
			file: File{Validators: []Validator{retryValidator(1)}, Retry: []Retry{{Flow: 0, Source: src1}}}},
		{name: "retry, only another flow in the file",
			file: File{Validators: []Validator{retryValidator(1)}, Retry: []Retry{{Flow: 7, Source: src1}}},
			want: []string{"validators[0]", "require_retry", "flow 0"}},
		{name: "retry, flow 0 live in the map",
			file:  File{Validators: []Validator{retryValidator(1)}},
			state: func(t *testing.T, s *fakeState) { retryAt(t, s, 7, 0); retryAt(t, s, 0, 2000) }},
		{name: "retry, flow 0 in the map has expired",
			file:  File{Validators: []Validator{retryValidator(1)}},
			state: func(t *testing.T, s *fakeState) { retryAt(t, s, 0, 999) },
			want:  []string{"validators[0]", "require_retry"}},
		{name: "both asked, only the key is there",
			file: File{Validators: []Validator{{Namespace: 1, RequireHMAC: true, RequireRetry: true}},
				HMACKeys: []HMACKey{{Flow: 0, Namespace: 1}}},
			want: []string{"validators[0]", "require_retry"}},
		{name: "every bad validator is named",
			file: File{Validators: []Validator{hmacValidator(1), {Hop: 4, Namespace: 2, RequireCRC: true}, {Hop: 9, Namespace: 3, RequireHMAC: true}}},
			want: []string{"validators[0]", "validators[2]", "hop 9", "namespace 3"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s fakeState
			if c.state != nil {
				c.state(t, &s)
			}
			_, err := CheckFlowZero(&c.file, &s, at(now))
			if c.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, ErrNoFlowZero) {
				t.Fatalf("err = %v, want ErrNoFlowZero", err)
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("refusal %q does not say %q", err, w)
				}
			}
			if strings.Contains(err.Error(), "validators[1]") {
				t.Errorf("refusal names a validator that asks for nothing: %v", err)
			}
		})
	}
}

// With no reader the file stands alone: that is how --prune checks, because
// what the file does not hold is about to be deleted.
func TestCheckFlowZero_FileAlone(t *testing.T) {
	f := &File{Validators: []Validator{hmacValidator(1)}}
	if _, err := CheckFlowZero(f, nil, at(time.Second)); !errors.Is(err, ErrNoFlowZero) {
		t.Fatalf("err = %v, want ErrNoFlowZero", err)
	}
	f.HMACKeys = []HMACKey{{Flow: 0, Namespace: 1}}
	if _, err := CheckFlowZero(f, nil, at(time.Second)); err != nil {
		t.Fatal(err)
	}
}

// The maps are read only when the file leaves the question open.
func TestCheckFlowZero_ReadsTheMapOnlyWhenItMust(t *testing.T) {
	var s fakeState
	f := &File{Validators: []Validator{hmacValidator(1)}, HMACKeys: []HMACKey{{Flow: 0, Namespace: 1}}}
	if _, err := CheckFlowZero(f, &s, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(s.touched) != 0 {
		t.Errorf("read %v with the answer in the file", s.touched)
	}
}

// A map that cannot be read is not an answer: the check refuses.
func TestCheckFlowZero_MapFailureRefuses(t *testing.T) {
	boom := errors.New("no such pin")
	for name, v := range map[string]Validator{"hmac": hmacValidator(1), "retry": retryValidator(1)} {
		t.Run(name, func(t *testing.T) {
			s := fakeState{fail: boom}
			_, err := CheckFlowZero(&File{Validators: []Validator{v}}, &s, at(time.Second))
			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want the map's error", err)
			}
		})
	}
}

func TestCheckFlowZero_BadClock(t *testing.T) {
	f := &File{Validators: []Validator{hmacValidator(1)}}
	for name, clock := range map[string]func() time.Duration{"none": nil, "before boot": at(-time.Second)} {
		if _, err := CheckFlowZero(f, &fakeState{}, clock); err == nil || !strings.Contains(err.Error(), "boottime") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// A value of the wrong size is a map this writer does not know: refuse.
func TestCheckFlowZero_MalformedValue(t *testing.T) {
	var s fakeState
	key := binary.LittleEndian.AppendUint64(nil, 1<<16) // flow 0, namespace 1
	s.maps = map[string]map[string][]byte{bpfschema.MapHMACKeys: {string(key): make([]byte, 12)}}
	_, err := CheckFlowZero(&File{Validators: []Validator{hmacValidator(1)}}, &s, at(time.Second))
	if err == nil || errors.Is(err, ErrNoFlowZero) || !strings.Contains(err.Error(), bpfschema.MapHMACKeys) {
		t.Fatalf("err = %v, want a decode error naming the map", err)
	}
}

// The flow-0 entry that satisfies the check may itself lapse: a ttl on it is
// the unattended failure FND-035 describes, so the caller gets a note.
func TestCheckFlowZero_NotesATTLOnFlowZero(t *testing.T) {
	f := &File{
		Validators: []Validator{hmacValidator(1), retryValidator(2)},
		HMACKeys:   []HMACKey{{Flow: 0, Namespace: 1, TTL: 24 * time.Hour}},
		Retry:      []Retry{{Flow: 0, Source: src1, TTL: time.Hour}},
	}
	notes, err := CheckFlowZero(f, nil, at(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || !strings.Contains(notes[0], "hmac_keys[0]") || !strings.Contains(notes[0], "24h") || !strings.Contains(notes[1], "retry[0]") {
		t.Errorf("notes = %q", notes)
	}
	f.HMACKeys[0].TTL, f.Retry[0].TTL = 0, 0
	if notes, err := CheckFlowZero(f, nil, at(time.Second)); err != nil || len(notes) != 0 {
		t.Errorf("no ttl: notes = %q, err = %v", notes, err)
	}
}
