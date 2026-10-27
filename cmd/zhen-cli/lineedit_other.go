// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build !linux

package main

import "errors"

// makeRaw is Linux-only; elsewhere zhen-cli reads plain lines.
func makeRaw(int) (func(), error) {
	return nil, errors.New("raw terminal mode is implemented for Linux only")
}
