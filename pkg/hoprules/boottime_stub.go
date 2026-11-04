// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package hoprules

import (
	"errors"
	"time"
)

// Boottime fails off Linux: there is no CLOCK_BOOTTIME and no hop-ebpf.
func Boottime() (time.Duration, error) {
	return 0, errors.New("hoprules: CLOCK_BOOTTIME needs Linux")
}
