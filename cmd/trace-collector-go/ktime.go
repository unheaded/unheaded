// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import "golang.org/x/sys/unix"

// ktimeNow returns the clock bpf_ktime_get_ns() reads: CLOCK_MONOTONIC,
// nanoseconds since boot. Every BPF timestamp (timestamp_ns, start_ns,
// last_seen_ns) is on this clock, so ages computed from them must be too:
// against time.Now().UnixNano() (~1.8e18) every flow looked decades stale.
// A variable so tests can move the clock.
var ktimeNow = func() uint64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return 0
	}
	return uint64(ts.Nano()) // #nosec G115 -- CLOCK_MONOTONIC is never negative
}

// olderThan reports whether a BPF timestamp ts is more than ttl nanoseconds
// before now. A timestamp at or after now (read just after now was taken)
// is not old; the plain subtraction wrapped and called it ancient.
func olderThan(now, ts, ttl uint64) bool {
	return now > ts && now-ts > ttl
}
