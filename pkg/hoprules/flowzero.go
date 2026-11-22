// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"unheaded/pkg/protocol/bpfschema"
)

// ErrNoFlowZero is CheckFlowZero's refusal: a validator asks for a key or a
// retry entry and flow 0 has none that is live (FND-035).
var ErrNoFlowZero = errors.New("hoprules: no live flow-0 entry")

// MapReader reads the maps as they are before an apply.
type MapReader interface {
	// Lookup returns the value for key, or (nil, nil) when it is absent.
	Lookup(mapName string, key []byte) ([]byte, error)
	// Iterate calls fn for every entry; an error from fn stops the walk.
	Iterate(mapName string, fn func(key, value []byte) error) error
}

// CheckFlowZero refuses a rules file that would fail every Shield-born packet
// (FND-035). Shield leaves both service ids at 0, so all its packets are
// flow 0: a require_hmac validator needs a live HMAC_KEYS entry for flow 0 on
// its namespace, and a require_retry validator a live RETRY_TOKENS entry for
// flow 0 (that map has no namespace), or an enforcing hop drops them all.
//
// An entry counts if f holds it, or if r, the maps as they are now, holds one
// that has not expired on the boottime clock. r is read only when f leaves
// the question open, and a map that cannot be read refuses. A nil r makes f
// stand alone, which is how a pruning apply must check: what f does not hold
// is about to be deleted.
//
// The refusal names every validator at fault. The notes say where the flow-0
// entry that satisfied the check has a ttl: when it lapses, with nothing to
// renew it, the same packets fail.
func CheckFlowZero(f *File, r MapReader, boottime func() time.Duration) (notes []string, err error) {
	if boottime == nil {
		return nil, errors.New("hoprules: no boottime clock")
	}
	now, err := bootSeconds(boottime())
	if err != nil {
		return nil, err
	}

	fileKeys := map[uint8]int{} // namespace -> index of its flow-0 hmac_keys entry
	for i, h := range f.HMACKeys {
		if h.Flow == 0 {
			fileKeys[h.Namespace] = i
		}
	}
	fileRetry := -1
	for i, e := range f.Retry {
		if e.Flow == 0 {
			fileRetry = i
			break
		}
	}

	var gaps []string
	noted := map[string]bool{}
	note := func(s string) {
		if !noted[s] {
			noted[s] = true
			notes = append(notes, s)
		}
	}
	mapRetry := 0 // 0 unread, 1 live entry, -1 none
	for i, v := range f.Validators {
		if v.RequireHMAC {
			if j, ok := fileKeys[v.Namespace]; ok {
				if ttl := f.HMACKeys[j].TTL; ttl != 0 {
					note(fmt.Sprintf("hmac_keys[%d] (flow 0, namespace %d) expires in %s; nothing renews it, and once it lapses every flow-0 packet on that namespace fails require_hmac", j, v.Namespace, ttl))
				}
			} else {
				live, err := liveHMACInMap(r, v.Namespace, now)
				if err != nil {
					return nil, err
				}
				if !live {
					gaps = append(gaps, fmt.Sprintf("validators[%d] (hop %d, namespace %d) has require_hmac, and HMAC_KEYS has no live entry for flow 0, namespace %d",
						i, v.Hop, v.Namespace, v.Namespace))
				}
			}
		}
		if v.RequireRetry {
			if fileRetry >= 0 {
				if ttl := f.Retry[fileRetry].TTL; ttl != 0 {
					note(fmt.Sprintf("retry[%d] (flow 0) expires in %s; nothing renews it, and once it lapses its source fails require_retry", fileRetry, ttl))
				}
				continue
			}
			if mapRetry == 0 {
				live, err := liveRetryInMap(r, now)
				if err != nil {
					return nil, err
				}
				mapRetry = -1
				if live {
					mapRetry = 1
				}
			}
			if mapRetry < 0 {
				gaps = append(gaps, fmt.Sprintf("validators[%d] (hop %d, namespace %d) has require_retry, and RETRY_TOKENS has no live entry for flow 0",
					i, v.Hop, v.Namespace))
			}
		}
	}
	if len(gaps) == 0 {
		return notes, nil
	}
	where := "in this file or in the maps"
	if r == nil {
		where = "in this file"
	}
	return nil, fmt.Errorf("%w %s: %s. Shield-born packets are all flow 0, so an enforcing hop would drop every one of them (FND-035); add the flow 0 entry to the rules file",
		ErrNoFlowZero, where, strings.Join(gaps, "; "))
}

// isLive is hop-ebpf's is_live: 0 never expires, otherwise live until the
// second it names.
func isLive(expiresAt, now uint32) bool {
	return expiresAt == 0 || now < expiresAt
}

func liveHMACInMap(r MapReader, namespace uint8, now uint32) (bool, error) {
	if r == nil {
		return false, nil
	}
	key, err := binary.Append(nil, binary.LittleEndian, bpfschema.HMACKeyKey{FlowID: 0, Namespace: uint16(namespace)})
	if err != nil {
		return false, fmt.Errorf("hoprules: %s key: %w", bpfschema.MapHMACKeys, err)
	}
	raw, err := r.Lookup(bpfschema.MapHMACKeys, key)
	if err != nil {
		return false, fmt.Errorf("hoprules: %s: %w", bpfschema.MapHMACKeys, err)
	}
	if raw == nil {
		return false, nil
	}
	var value bpfschema.HMACKeyValue
	if err := decodeExact(raw, &value); err != nil {
		return false, fmt.Errorf("hoprules: %s value: %w", bpfschema.MapHMACKeys, err)
	}
	return isLive(value.ExpiresAt, now), nil
}

func liveRetryInMap(r MapReader, now uint32) (bool, error) {
	if r == nil {
		return false, nil
	}
	live := false
	err := r.Iterate(bpfschema.MapRetryTokens, func(k, v []byte) error {
		var key bpfschema.RetryTokenKey
		if err := decodeExact(k, &key); err != nil {
			return fmt.Errorf("key: %w", err)
		}
		if key.FlowID != 0 {
			return nil
		}
		var value bpfschema.RetryTokenValue
		if err := decodeExact(v, &value); err != nil {
			return fmt.Errorf("value: %w", err)
		}
		if isLive(value.ExpiresAt, now) {
			live = true
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("hoprules: %s: %w", bpfschema.MapRetryTokens, err)
	}
	return live, nil
}

// decodeExact reads raw into out and refuses a length that is not out's: a
// map of another layout is not one this package can judge.
func decodeExact(raw []byte, out any) error {
	if want := binary.Size(out); len(raw) != want {
		return fmt.Errorf("%d bytes, want %d", len(raw), want)
	}
	return binary.Read(bytes.NewReader(raw), binary.LittleEndian, out)
}
