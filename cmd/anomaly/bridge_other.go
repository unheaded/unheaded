// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package main

import (
	"context"
	"errors"
)

func runBridge(context.Context, string, sink, logf) error {
	return errors.New("the cartridge bridge needs Linux (BPF ring buffer); use --no-cartridge")
}
