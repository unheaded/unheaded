// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

//go:build !linux

package main

import (
	"errors"
	"io"
)

func readPasswordTTY(io.Writer, string) (string, error) {
	return "", errors.New("terminal prompt is Linux-only here")
}
