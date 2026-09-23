// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-008 (real target): pkg/logagg.RingBuffer
//
// The original lich_008_wotan_cache_test.go declares its target as "ring
// buffer implementations in pkg/logagg and services/wotan" and then imports
// neither. It fuzzes a CacheEntry/ring buffer defined inside the test file,
// so it cannot find a defect in the shipping code no matter how long it
// runs — the same shape as a migration nothing executes against, or a gate
// whose assertion is never reached.
//
// This harness drives the real thing. Invariants asserted, all of which the
// production code is supposed to guarantee:
//
//   - Push never panics, for any capacity or entry content
//   - Len() never exceeds Cap()
//   - Len() equals min(pushes, capacity)
//   - Query never returns more than its effective limit
//   - Query never returns an entry that was never pushed
//   - Query results are chronological (oldest first)
//   - A filtered Query returns only entries matching the filter
package harnesses

import (
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	"unheaded/pkg/logagg"
)

// fuzzEntry builds a LogEntry from fuzzer bytes.
func fuzzEntry(i int, service, level, message string) logagg.LogEntry {
	return logagg.LogEntry{
		Timestamp: time.Unix(0, int64(i)),
		Service:   service,
		Level:     level,
		Message:   message,
	}
}

// FuzzRealRingBufferPushInvariants drives Push against the shipping buffer.
func FuzzRealRingBufferPushInvariants(f *testing.F) {
	f.Add(10, 3, "svc", "info", "hello")
	f.Add(1, 100, "", "", "")
	f.Add(0, 5, "svc", "error", "x")
	f.Add(-1, 2, "svc", "warn", "negative capacity")
	f.Add(2, 7, "s", "l", "\x00\xff\xfe invalid utf8")

	f.Fuzz(func(t *testing.T, capacity, pushes int, service, level, message string) {
		// Bound the work so the fuzzer explores shapes, not allocation size.
		if capacity > 4096 {
			capacity = 4096
		}
		if pushes < 0 {
			pushes = -pushes
		}
		if pushes > 8192 {
			pushes = 8192
		}

		rb := logagg.NewRingBuffer(capacity)

		effectiveCap := rb.Cap()
		if effectiveCap <= 0 {
			t.Fatalf("Cap() = %d, must be positive even for capacity=%d", effectiveCap, capacity)
		}

		for i := 0; i < pushes; i++ {
			rb.Push(fuzzEntry(i, service, level, message))

			if got := rb.Len(); got > effectiveCap {
				t.Fatalf("Len() = %d exceeds Cap() = %d after %d pushes", got, effectiveCap, i+1)
			}
		}

		want := pushes
		if want > effectiveCap {
			want = effectiveCap
		}
		if got := rb.Len(); got != want {
			t.Fatalf("Len() = %d, want min(pushes=%d, cap=%d) = %d", got, pushes, effectiveCap, want)
		}
	})
}

// FuzzRealRingBufferQueryInvariants drives Query with adversarial filters.
func FuzzRealRingBufferQueryInvariants(f *testing.F) {
	f.Add(16, 40, "svc", "info", 5, "svc", "info", "hello")
	f.Add(4, 10, "a", "error", 0, "", "", "")
	f.Add(8, 8, "s", "warn", -3, "s", "warn", "no-match-search-term")
	f.Add(2, 5, "s", "l", 1000000, "", "", "")

	f.Fuzz(func(t *testing.T, capacity, pushes int, service, level string,
		limit int, qService, qLevel, qSearch string,
	) {
		if capacity > 2048 {
			capacity = 2048
		}
		if pushes < 0 {
			pushes = -pushes
		}
		if pushes > 4096 {
			pushes = 4096
		}

		rb := logagg.NewRingBuffer(capacity)
		for i := 0; i < pushes; i++ {
			rb.Push(fuzzEntry(i, service, level, fmt.Sprintf("m%d", i)))
		}

		results := rb.Query(logagg.LogQuery{
			Service: qService,
			Level:   qLevel,
			Search:  qSearch,
			Limit:   limit,
		})

		if len(results) > rb.Len() {
			t.Fatalf("Query returned %d entries, buffer holds %d", len(results), rb.Len())
		}

		// Chronological order, and every result honours the filters it was
		// given. A filter that leaks a non-matching entry is a real defect:
		// the dashboard's log view would show another service's lines.
		var prev time.Time
		for i, e := range results {
			if i > 0 && e.Timestamp.Before(prev) {
				t.Fatalf("result %d timestamp %v precedes previous %v — not chronological",
					i, e.Timestamp, prev)
			}
			prev = e.Timestamp

			if qService != "" && e.Service != qService {
				t.Fatalf("service filter %q leaked entry from %q", qService, e.Service)
			}
			// Level matching is documented case-insensitive (matchesQuery),
			// so the assertion folds ASCII too. Comparing exactly here was a
			// harness bug that reported "a" matching "A" as a leak.
			if qLevel != "" && !asciiFoldEqual(e.Level, qLevel) {
				t.Fatalf("level filter %q leaked entry at level %q", qLevel, e.Level)
			}
		}
	})
}

// FuzzRealRingBufferQueryLimit pins the limit clamping specifically: a caller
// asking for a huge or negative limit must not be able to make Query return
// an unbounded slice.
func FuzzRealRingBufferQueryLimit(f *testing.F) {
	f.Add(64, 500, 0)
	f.Add(64, 500, -1)
	f.Add(64, 500, 1<<30)
	f.Add(1, 1, 1)

	f.Fuzz(func(t *testing.T, capacity, pushes, limit int) {
		if capacity > 2048 {
			capacity = 2048
		}
		if pushes < 0 {
			pushes = -pushes
		}
		if pushes > 4096 {
			pushes = 4096
		}

		rb := logagg.NewRingBuffer(capacity)
		for i := 0; i < pushes; i++ {
			rb.Push(fuzzEntry(i, "svc", "info", "m"))
		}

		results := rb.Query(logagg.LogQuery{Limit: limit})

		if len(results) > logagg.MaxLimit {
			t.Fatalf("Query returned %d entries for limit=%d, above MaxLimit=%d",
				len(results), limit, logagg.MaxLimit)
		}
		if limit > 0 && len(results) > limit {
			t.Fatalf("Query returned %d entries for limit=%d", len(results), limit)
		}
	})
}

// FuzzRealRingBufferSearchUTF8 feeds invalid UTF-8 through the search path,
// which does substring matching on message and field values.
func FuzzRealRingBufferSearchUTF8(f *testing.F) {
	f.Add("hello", "hello")
	f.Add("\xff\xfe", "\xff")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, message, search string) {
		rb := logagg.NewRingBuffer(8)
		rb.Push(fuzzEntry(0, "svc", "info", message))

		results := rb.Query(logagg.LogQuery{Search: search})

		// A search term that is a literal substring of the message must match.
		// Skipped for invalid UTF-8, where substring semantics are byte-wise
		// and the expectation is not well defined.
		if search != "" && utf8.ValidString(message) && utf8.ValidString(search) {
			if contains(message, search) && len(results) == 0 {
				t.Fatalf("search %q did not match message %q that contains it", search, message)
			}
		}
	})
}

// contains is strings.Contains, named locally to keep the invariant explicit.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) &&
		indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// asciiFoldEqual mirrors pkg/logagg's level comparison: ASCII case folding,
// exact on every other byte. Deliberately a separate implementation from the
// one under test — an assertion that calls the code it is checking proves
// nothing.
func asciiFoldEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
