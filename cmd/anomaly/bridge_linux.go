// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"context"
	"fmt"
	"time"

	"unheaded/pkg/cartridge"
)

// runBridge drains the pinned interpreter's CART_EVENTS every 200 ms and
// forwards score events until ctx ends.
func runBridge(ctx context.Context, pinDir string, out sink, w logf) error {
	c, err := cartridge.OpenEvents(pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	rd, err := c.Events()
	if err != nil {
		return err
	}
	defer rd.Close()
	fmt.Fprintf(w, "anomaly: bridging cartridge scores from %s\n", pinDir)
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	total := 0
	for {
		evs, err := c.DrainEvents(rd)
		if err != nil {
			return err
		}
		n, err := bridge(ctx, evs, out)
		total += n
		if err != nil {
			fmt.Fprintf(w, "anomaly: forward score: %v\n", err)
		} else if n > 0 {
			fmt.Fprintf(w, "anomaly: %d scores forwarded (%d total)\n", n, total)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
