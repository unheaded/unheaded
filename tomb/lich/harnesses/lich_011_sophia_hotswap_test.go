// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-011: Sophia Dictionary Hot-Swap Race Fuzzer
//
// Target: Concurrent dictionary access patterns in Sophia's knowledge layer.
// Goal: Find data races, panics, or garbage reads during dictionary hot-swap,
//       epoch rollover, and concurrent lookup operations.
//
// The Sophia dictionary is a critical path component: it maps compact indices
// to string values for wire-format compression. Dictionary hot-swap replaces
// the entire mapping atomically while readers are active. Epoch (uint8) tracks
// dictionary generations and must handle 255->0 rollover correctly.
//
// Attack vectors:
//   - Concurrent reads during atomic pointer swap (hot-swap race)
//   - Epoch rollover at uint8 boundary (255->0 wrap)
//   - Adversarial key selection under concurrent load
//   - Timing-dependent interleaving of readers and writers
//   - Empty/nil dictionary transitions

package harnesses

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

// sophiaDict is a simplified dictionary for fuzzing hot-swap behavior.
// Uses sync.Map internally for concurrent-safe reads.
type sophiaDict struct {
	entries sync.Map
	epoch   uint8
	size    int
}

// newSophiaDict creates a dictionary with the given epoch and entries.
func newSophiaDict(epoch uint8, keys []string, values []string) *sophiaDict {
	d := &sophiaDict{epoch: epoch}
	count := len(keys)
	if len(values) < count {
		count = len(values)
	}
	for i := 0; i < count; i++ {
		d.entries.Store(keys[i], values[i])
		d.size++
	}
	return d
}

// lookup retrieves a value from the dictionary. Returns ("", false) if not found.
func (d *sophiaDict) lookup(key string) (string, bool) {
	val, ok := d.entries.Load(key)
	if !ok {
		return "", false
	}
	return val.(string), true
}

// sophiaDictHolder holds an atomically-swappable dictionary pointer.
type sophiaDictHolder struct {
	ptr unsafe.Pointer // *sophiaDict
}

// newSophiaDictHolder creates a holder with the given initial dictionary.
func newSophiaDictHolder(initial *sophiaDict) *sophiaDictHolder {
	h := &sophiaDictHolder{}
	atomic.StorePointer(&h.ptr, unsafe.Pointer(initial))
	return h
}

// load atomically loads the current dictionary.
func (h *sophiaDictHolder) load() *sophiaDict {
	return (*sophiaDict)(atomic.LoadPointer(&h.ptr))
}

// swap atomically replaces the dictionary.
func (h *sophiaDictHolder) swap(newDict *sophiaDict) {
	atomic.StorePointer(&h.ptr, unsafe.Pointer(newDict))
}

// epochAfter returns true if epoch a is "after" epoch b, handling uint8 wrap.
// Uses the standard half-range comparison: a is after b if (a - b) is in (0, 128).
func epochAfter(a, b uint8) bool {
	diff := a - b // wraps naturally for uint8
	return diff > 0 && diff < 128
}

// FuzzSophiaDictionarySwap creates a dictionary, launches N goroutines doing
// concurrent reads while another goroutine atomically replaces the map pointer.
// Fuzz the timing and key selection. Verify no panics, no data races, reads
// always return valid data (from either old or new map, never garbage).
func FuzzSophiaDictionarySwap(f *testing.F) {
	// Seeds: number of readers, number of swaps, key selection seed
	f.Add(4, 10, []byte{0, 1, 2, 3, 4})
	f.Add(1, 1, []byte{0})
	f.Add(8, 100, []byte{0xFF, 0x00, 0x55, 0xAA})
	f.Add(16, 50, []byte{})
	f.Add(2, 0, []byte{0, 0, 0})

	f.Fuzz(func(t *testing.T, numReaders, numSwaps int, keySeed []byte) {
		// Clamp inputs for fuzzing safety
		if numReaders < 1 {
			numReaders = 1
		}
		if numReaders > 32 {
			numReaders = 32
		}
		if numSwaps < 0 {
			numSwaps = 0
		}
		if numSwaps > 200 {
			numSwaps = 200
		}

		// Build two dictionaries: "old" and "new"
		oldKeys := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
		oldVals := []string{"old-a", "old-b", "old-g", "old-d", "old-e"}
		newKeys := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
		newVals := []string{"new-a", "new-b", "new-g", "new-d", "new-e"}

		// Valid values from either dictionary
		validValues := map[string]bool{
			"old-a": true, "old-b": true, "old-g": true, "old-d": true, "old-e": true,
			"new-a": true, "new-b": true, "new-g": true, "new-d": true, "new-e": true,
		}

		oldDict := newSophiaDict(0, oldKeys, oldVals)
		holder := newSophiaDictHolder(oldDict)

		var wg sync.WaitGroup
		var readErrors int64

		// Launch reader goroutines
		for r := 0; r < numReaders; r++ {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()

				// Each reader continuously reads keys
				for i := 0; i < numSwaps+10; i++ {
					// Select key based on fuzz seed
					keyIdx := i % len(oldKeys)
					if len(keySeed) > 0 {
						keyIdx = int(keySeed[i%len(keySeed)]) % len(oldKeys)
					}

					dict := holder.load()
					if dict == nil {
						atomic.AddInt64(&readErrors, 1)
						continue
					}

					val, ok := dict.lookup(oldKeys[keyIdx])
					if ok {
						// Value must be from either old or new dict — never garbage
						if !validValues[val] {
							t.Errorf("LICH-011 RACE: reader %d got garbage value %q for key %q",
								readerID, val, oldKeys[keyIdx])
						}
					}
				}
			}(r)
		}

		// Writer goroutine: swap dictionaries
		wg.Add(1)
		go func() {
			defer wg.Done()

			for s := 0; s < numSwaps; s++ {
				var dict *sophiaDict
				if s%2 == 0 {
					dict = newSophiaDict(uint8(s+1), newKeys, newVals)
				} else {
					dict = newSophiaDict(uint8(s+1), oldKeys, oldVals)
				}
				holder.swap(dict)
			}
		}()

		wg.Wait()

		if atomic.LoadInt64(&readErrors) > 0 {
			t.Errorf("LICH-011: %d nil dictionary reads during hot-swap", readErrors)
		}
	})
}

// FuzzSophiaEpochRollover fuzzes rapid epoch increments (255->0 rollover).
// Verifies epoch comparison logic handles wrap correctly.
func FuzzSophiaEpochRollover(f *testing.F) {
	// Seed: pairs of epochs to compare
	f.Add(uint8(0), uint8(0))
	f.Add(uint8(255), uint8(0))
	f.Add(uint8(0), uint8(255))
	f.Add(uint8(254), uint8(255))
	f.Add(uint8(255), uint8(254))
	f.Add(uint8(128), uint8(0))
	f.Add(uint8(0), uint8(128))
	f.Add(uint8(127), uint8(0))
	f.Add(uint8(200), uint8(100))
	f.Add(uint8(100), uint8(200))

	f.Fuzz(func(t *testing.T, epochA, epochB uint8) {
		// Invariant 1: epochAfter must be anti-reflexive
		if epochAfter(epochA, epochA) {
			t.Errorf("LICH-011 EPOCH: epochAfter(%d, %d) should be false (anti-reflexive)",
				epochA, epochA)
		}

		// Invariant 2: epochAfter(a, b) and epochAfter(b, a) cannot both be true
		if epochAfter(epochA, epochB) && epochAfter(epochB, epochA) {
			t.Errorf("LICH-011 EPOCH: both epochAfter(%d, %d) and epochAfter(%d, %d) are true",
				epochA, epochB, epochB, epochA)
		}

		// Invariant 3: for equal epochs, neither direction should be "after"
		if epochA == epochB {
			if epochAfter(epochA, epochB) || epochAfter(epochB, epochA) {
				t.Errorf("LICH-011 EPOCH: equal epochs %d should not be ordered", epochA)
			}
		}

		// Invariant 4: sequential increments (simulating rapid rollover)
		// Walk from epochA forward by (epochB % 64) steps, each step should
		// be "after" the previous one.
		steps := int(epochB % 64)
		if steps > 0 {
			prev := epochA
			for i := 0; i < steps; i++ {
				next := prev + 1 // wraps at 255->0
				if !epochAfter(next, prev) && next != prev {
					t.Errorf("LICH-011 EPOCH: sequential step %d->%d should be after (step %d/%d from %d)",
						prev, next, i+1, steps, epochA)
				}
				prev = next
			}
		}

		// Invariant 5: dictionary swap with epoch check
		oldDict := newSophiaDict(epochA, []string{"key"}, []string{"old"})
		newDict := newSophiaDict(epochB, []string{"key"}, []string{"new"})

		holder := newSophiaDictHolder(oldDict)

		// Only swap if new epoch is "after" old (or equal for idempotent update)
		if epochAfter(epochB, epochA) || epochB == epochA {
			holder.swap(newDict)
			current := holder.load()
			if current.epoch != epochB {
				t.Errorf("LICH-011 EPOCH: after swap epoch should be %d, got %d",
					epochB, current.epoch)
			}
		}
	})
}

// FuzzSophiaConcurrentLookup has multiple goroutines reading different
// fuzz-derived keys from the same dictionary concurrently. Verifies no
// panics under concurrent load.
func FuzzSophiaConcurrentLookup(f *testing.F) {
	f.Add([]byte("alpha\x00beta\x00gamma"), 4)
	f.Add([]byte{}, 8)
	f.Add([]byte("x"), 1)
	f.Add([]byte("\x00\x00\x00"), 16)
	f.Add([]byte("alpha\x00alpha\x00alpha"), 32)

	f.Fuzz(func(t *testing.T, keyData []byte, numGoroutines int) {
		if numGoroutines < 1 {
			numGoroutines = 1
		}
		if numGoroutines > 64 {
			numGoroutines = 64
		}

		// Build dictionary with deterministic entries
		baseKeys := []string{"alpha", "beta", "gamma", "delta", "epsilon",
			"zeta", "eta", "theta", "iota", "kappa"}
		baseVals := make([]string, len(baseKeys))
		for i, k := range baseKeys {
			baseVals[i] = fmt.Sprintf("val-%s-%d", k, i)
		}

		dict := newSophiaDict(1, baseKeys, baseVals)

		// Parse fuzz-derived lookup keys (split on null byte)
		var lookupKeys []string
		current := ""
		for _, b := range keyData {
			if b == 0x00 {
				if current != "" {
					lookupKeys = append(lookupKeys, current)
				}
				current = ""
			} else {
				current += string(b)
			}
		}
		if current != "" {
			lookupKeys = append(lookupKeys, current)
		}

		// Add some base keys to ensure some hits
		lookupKeys = append(lookupKeys, "alpha", "gamma", "nonexistent")

		var wg sync.WaitGroup
		var totalLookups int64
		var totalHits int64

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for i, key := range lookupKeys {
					_ = i
					val, ok := dict.lookup(key)
					atomic.AddInt64(&totalLookups, 1)

					if ok {
						atomic.AddInt64(&totalHits, 1)
						// Verify value is non-empty for known keys
						if val == "" {
							t.Errorf("LICH-011 LOOKUP: goroutine %d got empty value for key %q",
								goroutineID, key)
						}
					}
				}
			}(g)
		}

		wg.Wait()

		// Invariant: total lookups must equal goroutines * keys
		expectedLookups := int64(numGoroutines) * int64(len(lookupKeys))
		if atomic.LoadInt64(&totalLookups) != expectedLookups {
			t.Errorf("LICH-011 LOOKUP: expected %d total lookups, got %d",
				expectedLookups, totalLookups)
		}

		// Invariant: at least some hits expected (we added base keys)
		if atomic.LoadInt64(&totalHits) == 0 && len(lookupKeys) > 0 {
			t.Errorf("LICH-011 LOOKUP: zero hits across %d lookups from %d goroutines",
				totalLookups, numGoroutines)
		}
	})
}
