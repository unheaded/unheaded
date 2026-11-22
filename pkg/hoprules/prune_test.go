// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/protocol/bpfschema"
)

// pruneState is fakeState with deletes, recorded in order.
type pruneState struct {
	fakeState
	deleted   []string // "MAP key-hex"
	failAfter int      // fail the delete numbered failAfter+1; 0: never
}

var errDelete = errors.New("fake delete failure")

func (s *pruneState) Delete(mapName string, key []byte) error {
	if s.failAfter != 0 && len(s.deleted) == s.failAfter {
		return errDelete
	}
	s.deleted = append(s.deleted, fmt.Sprintf("%s %x", mapName, key))
	delete(s.maps[mapName], string(key))
	return nil
}

// stocked holds two validators, two keys and two retry entries; the file
// below keeps the first of each.
func stocked(t *testing.T) *pruneState {
	t.Helper()
	s := &pruneState{}
	s.put(t, bpfschema.MapHopValidators, bpfschema.HopValidatorKey{HopID: 3, Namespace: 1}, bpfschema.HopValidatorValue{RequireHMAC: 1})
	s.put(t, bpfschema.MapHopValidators, bpfschema.HopValidatorKey{HopID: 3, Namespace: 2}, bpfschema.HopValidatorValue{RequireCRC: 1})
	s.put(t, bpfschema.MapHMACKeys, bpfschema.HMACKeyKey{FlowID: 0, Namespace: 1}, bpfschema.HMACKeyValue{CreatedAt: 1})
	s.put(t, bpfschema.MapHMACKeys, bpfschema.HMACKeyKey{FlowID: 0x2211, Namespace: 2}, bpfschema.HMACKeyValue{CreatedAt: 1})
	s.put(t, bpfschema.MapRetryTokens, retryKey(Retry{Flow: 0, Source: src1}), bpfschema.RetryTokenValue{IssuedAt: 1})
	s.put(t, bpfschema.MapRetryTokens, retryKey(Retry{Flow: 9, Source: netip.MustParseAddr("fd00::a:b:c")}), bpfschema.RetryTokenValue{IssuedAt: 1})
	return s
}

func keeper() *File {
	return &File{
		Validators: []Validator{{Hop: 3, Namespace: 1, RequireHMAC: true}},
		HMACKeys:   []HMACKey{{Flow: 0, Namespace: 1, KeyFile: "/k"}},
		Retry:      []Retry{{Flow: 0, Source: src1}},
	}
}

func TestStaleEntries_ListsWhatTheFileDoesNotHold(t *testing.T) {
	s := stocked(t)
	stale, err := StaleEntries(keeper(), s)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range stale {
		got = append(got, e.String())
	}
	want := []string{
		"HOP_VALIDATORS hop 3 namespace 2",
		"HMAC_KEYS flow 0x2211 namespace 2",
		"RETRY_TOKENS flow 0x0009 source low 48 bits 000a:000b:000c",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("stale =\n %q\nwant\n %q", got, want)
	}
	if len(s.deleted) != 0 {
		t.Errorf("listing deleted %v", s.deleted)
	}
}

// A file that holds everything in the maps leaves nothing to prune.
func TestStaleEntries_NothingStale(t *testing.T) {
	s := stocked(t)
	f := keeper()
	f.Validators = append(f.Validators, Validator{Hop: 3, Namespace: 2})
	f.HMACKeys = append(f.HMACKeys, HMACKey{Flow: 0x2211, Namespace: 2, KeyFile: "/k"})
	f.Retry = append(f.Retry, Retry{Flow: 9, Source: netip.MustParseAddr("fd00::a:b:c")})
	stale, err := StaleEntries(f, s)
	if err != nil || len(stale) != 0 {
		t.Fatalf("stale = %v, err = %v", stale, err)
	}
}

// The listing never repeats a value: an entry written before M-198.3 holds
// key bytes.
func TestStaleEntries_NeverPrintsAValue(t *testing.T) {
	s := &pruneState{}
	var key [KeySize]byte
	copy(key[:], testKey())
	s.put(t, bpfschema.MapHMACKeys, bpfschema.HMACKeyKey{FlowID: 5, Namespace: 1}, bpfschema.HMACKeyValue{Key: key})
	stale, err := StaleEntries(&File{Validators: []Validator{{}}}, s)
	if err != nil || len(stale) != 1 {
		t.Fatalf("stale = %v, err = %v", stale, err)
	}
	for _, verb := range []string{"%v", "%+v", "%s", "%x", "%#v"} {
		out := fmt.Sprintf(verb, stale[0])
		if strings.Contains(out, "a0a1a2") || strings.Contains(out, string(testKey())) || strings.Contains(out, "160 161") {
			t.Errorf("%s prints key bytes: %s", verb, out)
		}
	}
}

func TestStaleEntries_Refuses(t *testing.T) {
	boom := errors.New("no such pin")
	if _, err := StaleEntries(keeper(), &pruneState{fakeState: fakeState{fail: boom}}); !errors.Is(err, boom) {
		t.Errorf("unreadable map: err = %v", err)
	}
	if _, err := StaleEntries(keeper(), nil); err == nil {
		t.Error("no reader: accepted")
	}
	// A key of another size is a map of another layout: nothing is listed.
	s := &pruneState{}
	s.maps = map[string]map[string][]byte{bpfschema.MapHopValidators: {"abc": make([]byte, 32)}}
	if stale, err := StaleEntries(keeper(), s); err == nil || !strings.Contains(err.Error(), bpfschema.MapHopValidators) {
		t.Errorf("odd key: stale = %v, err = %v", stale, err)
	}
	// A file that validate refuses prunes nothing.
	bad := keeper()
	bad.Validators = append(bad.Validators, bad.Validators[0])
	if _, err := StaleEntries(bad, stocked(t)); err == nil || !strings.Contains(err.Error(), "same hop") {
		t.Errorf("invalid file: err = %v", err)
	}
}

// Validators go first: deleting a key before the rule that asks for it would
// make an enforcing hop drop that flow in between.
func TestPrune_DeletesValidatorsFirst(t *testing.T) {
	s := stocked(t)
	stale, err := StaleEntries(keeper(), s)
	if err != nil {
		t.Fatal(err)
	}
	// Hand them over in the worst order.
	stale[0], stale[2] = stale[2], stale[0]
	got, err := Prune(s, stale)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Pruned{Validators: 1, HMACKeys: 1, Retry: 1}); got != want {
		t.Errorf("pruned = %+v, want %+v", got, want)
	}
	if len(s.deleted) != 3 || !strings.HasPrefix(s.deleted[0], bpfschema.MapHopValidators) {
		t.Errorf("delete order = %v", s.deleted)
	}
	if n := len(s.maps[bpfschema.MapHopValidators]) + len(s.maps[bpfschema.MapHMACKeys]) + len(s.maps[bpfschema.MapRetryTokens]); n != 3 {
		t.Errorf("%d entries left, want the file's 3", n)
	}
	if after, err := StaleEntries(keeper(), s); err != nil || len(after) != 0 {
		t.Errorf("after the prune: stale = %v, err = %v", after, err)
	}
}

// A failed delete stops the walk and Pruned says what had gone.
func TestPrune_DeleteFailure(t *testing.T) {
	s := stocked(t)
	s.failAfter = 1
	stale, err := StaleEntries(keeper(), s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Prune(s, stale)
	if !errors.Is(err, errDelete) {
		t.Fatalf("err = %v", err)
	}
	if want := (Pruned{Validators: 1}); got != want {
		t.Errorf("pruned = %+v, want %+v", got, want)
	}
	if !strings.Contains(err.Error(), bpfschema.MapHMACKeys) {
		t.Errorf("error names no map: %v", err)
	}
}

// Only the three rule maps are ever pruned: CONFIG holds the mode.
func TestPrune_RefusesAnotherMap(t *testing.T) {
	s := stocked(t)
	_, err := Prune(s, []Stale{{Map: configMap, Key: []byte{4, 0, 0, 0}}})
	if err == nil || len(s.deleted) != 0 {
		t.Fatalf("err = %v, deleted = %v", err, s.deleted)
	}
}

// With --prune the flow-0 check must not count what is about to go.
func TestPruneThenFlowZero_FileStandsAlone(t *testing.T) {
	s := stocked(t)
	f := &File{Validators: []Validator{{Hop: 3, Namespace: 1, RequireHMAC: true}}}
	if _, err := CheckFlowZero(f, s, at(time.Second)); err != nil {
		t.Fatalf("with the map: %v", err)
	}
	if _, err := CheckFlowZero(f, nil, at(time.Second)); !errors.Is(err, ErrNoFlowZero) {
		t.Fatalf("file alone: err = %v, want ErrNoFlowZero", err)
	}
}
