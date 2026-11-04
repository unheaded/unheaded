// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package hoprules

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// Boottime reads CLOCK_BOOTTIME, the clock hop-ebpf compares expiry against
// with bpf_ktime_get_boot_ns.
func Boottime() (time.Duration, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		return 0, fmt.Errorf("hoprules: CLOCK_BOOTTIME: %w", err)
	}
	return time.Duration(ts.Nano()), nil
}
