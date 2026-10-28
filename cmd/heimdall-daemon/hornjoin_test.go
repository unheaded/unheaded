// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"

	"unheaded/pkg/gjallarhorn"
)

// S-ASCEND-2 E3 finding: on a real NIC a multicast group nobody on the host
// joined is filtered before XDP runs, so the observer never saw a bootstrap
// horn on east until a listener joined the group. With --xdp-iface heimdall
// joins it itself. The live join is checked with `ip -6 maddr` in a netns.
func TestJoinHornGroup_UnknownInterface(t *testing.T) {
	if gjallarhorn.MulticastGroup != "ff02::1:abba" || gjallarhorn.Port != 16901 {
		t.Fatalf("group/port = %s/%d", gjallarhorn.MulticastGroup, gjallarhorn.Port)
	}
	c, err := joinHornGroup("no-such-if0")
	if err == nil {
		c.Close()
		t.Fatal("joining on a missing interface must fail")
	}
	if !strings.Contains(err.Error(), "no-such-if0") {
		t.Fatalf("error does not name the interface: %v", err)
	}
}
