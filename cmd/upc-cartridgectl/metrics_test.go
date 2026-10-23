// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"

	"unheaded/pkg/cartridge"
)

func TestRenderMetrics(t *testing.T) {
	t.Parallel()
	var s [cartridge.StatCount]uint64
	s[0*8+cartridge.StatPass] = 5
	s[0*8+cartridge.StatDrop] = 1
	s[0*8+cartridge.StatFaults] = 2
	s[0*8+cartridge.StatInsns] = 40
	s[1*8+cartridge.StatQueueOK] = 7
	s[cartridge.StatTickJobsDone] = 3
	s[cartridge.StatTickStale] = 1
	s[cartridge.StatXDPPackets] = 6
	slots := map[int]SlotState{0: {Name: "pass", Lab: true}, 1: {Name: "sampler"}}
	out := RenderMetrics(s, slots)
	for _, want := range []string{
		`unheaded_cart_packets_total{slot="0",verdict="pass",lab="1"} 5`,
		`unheaded_cart_packets_total{slot="0",verdict="drop",lab="1"} 1`,
		`unheaded_cart_faults_total{slot="0",lab="1"} 2`,
		`unheaded_cart_insns_total{slot="0",lab="1"} 40`,
		`unheaded_cart_packets_total{slot="1",verdict="pass",lab="0"} 0`,
		`unheaded_cart_queue_depth 3`, // 7 queued - 3 done - 1 stale
		`unheaded_cart_scores_total 3`,
		`unheaded_cart_xdp_packets_total 6`,
		"# TYPE unheaded_cart_packets_total counter",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Underflow (more done than queued after a restart) clamps at 0.
	s[cartridge.StatTickJobsDone] = 100
	if !strings.Contains(RenderMetrics(s, slots), "unheaded_cart_queue_depth 0") {
		t.Error("queue depth underflowed")
	}
}

func TestLoopbackOnly(t *testing.T) {
	t.Parallel()
	for addr, ok := range map[string]bool{
		"127.0.0.1:26610": true, "[::1]:26610": true, "localhost:26610": true,
		"0.0.0.0:26610": false, ":26610": false, "10.0.0.1:26610": false,
	} {
		if got := isLoopbackAddr(addr); got != ok {
			t.Errorf("%s: %v, want %v", addr, got, ok)
		}
	}
}
