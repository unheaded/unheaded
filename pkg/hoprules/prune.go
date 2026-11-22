// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"unheaded/pkg/protocol/bpfschema"
)

// ruleMaps are the maps a rules file describes, in the order a prune deletes
// from them: validators first, because a key or retry entry deleted before
// the rule that asks for it would make an enforcing hop drop that flow in
// between. CONFIG is not among them: it holds the mode, not rules.
var ruleMaps = []string{bpfschema.MapHopValidators, bpfschema.MapHMACKeys, bpfschema.MapRetryTokens}

// Stale is one map entry the rules file does not hold. It carries the key
// only: a value may hold key material written before the writer left it out.
type Stale struct {
	Map string
	Key []byte
}

// String names the entry by its map and decoded key.
func (s Stale) String() string {
	switch s.Map {
	case bpfschema.MapHopValidators:
		var k bpfschema.HopValidatorKey
		if decodeExact(s.Key, &k) == nil {
			return fmt.Sprintf("%s hop %d namespace %d", s.Map, k.HopID, k.Namespace)
		}
	case bpfschema.MapHMACKeys:
		var k bpfschema.HMACKeyKey
		if decodeExact(s.Key, &k) == nil {
			return fmt.Sprintf("%s flow 0x%04x namespace %d", s.Map, k.FlowID, k.Namespace)
		}
	case bpfschema.MapRetryTokens:
		var k bpfschema.RetryTokenKey
		if decodeExact(s.Key, &k) == nil {
			b := k.SrcLow48 // least significant byte first
			return fmt.Sprintf("%s flow 0x%04x source low 48 bits %02x%02x:%02x%02x:%02x%02x",
				s.Map, k.FlowID, b[5], b[4], b[3], b[2], b[1], b[0])
		}
	}
	return fmt.Sprintf("%s key %x", s.Map, s.Key)
}

// Pruned counts the entries deleted, including those deleted before a failure.
type Pruned struct {
	Validators int
	HMACKeys   int
	Retry      int
}

// MapPruner deletes one entry from the named map; an absent key is not an
// error.
type MapPruner interface {
	Delete(mapName string, key []byte) error
}

// StaleEntries lists what HOP_VALIDATORS, HMAC_KEYS and RETRY_TOKENS hold
// that f does not: the entries a pruning apply would delete, validators
// first, each map's in key order. It reads keys and never keeps a value, and
// it deletes nothing. A key of a size this package does not write means a map
// of another layout, and is an error, not a deletion.
func StaleEntries(f *File, r MapReader) ([]Stale, error) {
	if r == nil {
		return nil, errors.New("hoprules: no maps to read")
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	keep, err := fileKeys(f)
	if err != nil {
		return nil, err
	}
	var stale []Stale
	for _, mapName := range ruleMaps {
		var found []Stale
		err := r.Iterate(mapName, func(key, _ []byte) error {
			if len(key) != ruleKeySize {
				return fmt.Errorf("a %d-byte key, want %d: not a map this writer knows", len(key), ruleKeySize)
			}
			if !keep[mapName][string(key)] {
				found = append(found, Stale{Map: mapName, Key: bytes.Clone(key)})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("hoprules: %s: %w", mapName, err)
		}
		sort.Slice(found, func(i, j int) bool { return bytes.Compare(found[i].Key, found[j].Key) < 0 })
		stale = append(stale, found...)
	}
	return stale, nil
}

// ruleKeySize is the key size of all three rule maps.
const ruleKeySize = 8

// fileKeys encodes every key f would write, by map.
func fileKeys(f *File) (map[string]map[string]bool, error) {
	keep := map[string]map[string]bool{}
	add := func(mapName string, key any) error {
		k, err := binary.Append(nil, binary.LittleEndian, key)
		if err != nil {
			return fmt.Errorf("hoprules: %s key: %w", mapName, err)
		}
		if keep[mapName] == nil {
			keep[mapName] = map[string]bool{}
		}
		keep[mapName][string(k)] = true
		return nil
	}
	for _, v := range f.Validators {
		if err := add(bpfschema.MapHopValidators, bpfschema.HopValidatorKey{HopID: v.Hop, Namespace: uint32(v.Namespace)}); err != nil {
			return nil, err
		}
	}
	for _, h := range f.HMACKeys {
		if err := add(bpfschema.MapHMACKeys, bpfschema.HMACKeyKey{FlowID: h.Flow, Namespace: uint16(h.Namespace)}); err != nil {
			return nil, err
		}
	}
	for _, e := range f.Retry {
		if err := add(bpfschema.MapRetryTokens, retryKey(e)); err != nil {
			return nil, err
		}
	}
	return keep, nil
}

// Prune deletes stale through w, in ruleMaps order whatever order it is given
// in. Only the three rule maps are touched: anything else in stale is refused
// before the first delete. A failed delete stops the walk, and the returned
// Pruned says what had gone.
func Prune(w MapPruner, stale []Stale) (Pruned, error) {
	var done Pruned
	counts := map[string]*int{
		bpfschema.MapHopValidators: &done.Validators,
		bpfschema.MapHMACKeys:      &done.HMACKeys,
		bpfschema.MapRetryTokens:   &done.Retry,
	}
	for _, s := range stale {
		if counts[s.Map] == nil {
			return done, fmt.Errorf("hoprules: %s is not a rule map; nothing was deleted", s.Map)
		}
	}
	for _, mapName := range ruleMaps {
		for _, s := range stale {
			if s.Map != mapName {
				continue
			}
			if err := w.Delete(s.Map, s.Key); err != nil {
				return done, fmt.Errorf("hoprules: %s: delete: %w", s.Map, err)
			}
			*counts[mapName]++
		}
	}
	return done, nil
}
