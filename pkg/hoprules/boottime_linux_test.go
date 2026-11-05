// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package hoprules

import "testing"

func TestBoottime_RunsForward(t *testing.T) {
	a, err := Boottime()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Boottime()
	if err != nil {
		t.Fatal(err)
	}
	if a <= 0 || b < a {
		t.Fatalf("boottime went %v then %v", a, b)
	}
}
