// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cuirassdriver

import (
	"context"
	"testing"

	"unheaded/pkg/deploy/cuirassdriver/cuirasstest"
)

func TestDriver_Ping(t *testing.T) {
	f := cuirasstest.New(t)
	d := newTestDriver(t, f.URL)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("ping a live Cuirass: %v", err)
	}
	f.Mu.Lock()
	f.Reachable = false
	f.Mu.Unlock()
	if err := d.Ping(context.Background()); err == nil {
		t.Fatal("ping succeeded against a Cuirass answering 503")
	}
}
