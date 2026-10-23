// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/cartridge"
)

// SlotState is what upc-cartridgectl loaded into a slot (slots.json in the
// registry dir), so metrics can carry the cartridge name and a lab label.
type SlotState struct {
	Name     string    `json:"name"`
	SHA256   string    `json:"sha256"`
	KeyID    string    `json:"seal_key_id"`
	Gen      uint32    `json:"gen"`
	Lab      bool      `json:"lab"`
	LoadedAt time.Time `json:"loaded_at"`
}

func slotsPath(dir string) string { return filepath.Join(dir, "slots.json") }

func readSlots(dir string) (map[int]SlotState, error) {
	b, err := os.ReadFile(slotsPath(dir)) // #nosec G304 -- inside the operator's registry dir
	if errors.Is(err, os.ErrNotExist) {
		return map[int]SlotState{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[int]SlotState{}
	return m, json.Unmarshal(b, &m)
}

func writeSlots(dir string, m map[int]SlotState) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(slotsPath(dir), append(b, '\n'), 0o600)
}

// RenderMetrics renders the Prometheus text exposition of the counters.
func RenderMetrics(s [cartridge.StatCount]uint64, slots map[int]SlotState) string {
	var b strings.Builder
	lab := func(slot int) string {
		if slots[slot].Lab {
			return "1"
		}
		return "0"
	}
	family := func(name, help string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	}
	family("unheaded_cart_packets_total", "Cartridge runs by slot and verdict.")
	for slot := 0; slot < cartridge.Slots; slot++ {
		fmt.Fprintf(&b, "unheaded_cart_packets_total{slot=\"%d\",verdict=\"pass\",lab=\"%s\"} %d\n", slot, lab(slot), cartridge.SlotStat(s, slot, cartridge.StatPass))
		fmt.Fprintf(&b, "unheaded_cart_packets_total{slot=\"%d\",verdict=\"drop\",lab=\"%s\"} %d\n", slot, lab(slot), cartridge.SlotStat(s, slot, cartridge.StatDrop))
	}
	per := []struct {
		name, help string
		off        int
	}{
		{"unheaded_cart_faults_total", "Cartridge faults by slot.", cartridge.StatFaults},
		{"unheaded_cart_insns_total", "MBC instructions executed by slot.", cartridge.StatInsns},
		{"unheaded_cart_runs_total", "Cartridge runs (packets or tick calls) by slot.", cartridge.StatRuns},
		{"unheaded_cart_emits_total", "EMIT records by slot.", cartridge.StatEmits},
		{"unheaded_cart_queue_drops_total", "QUEUE pushes dropped (queue full) by slot.", cartridge.StatQueueDrops},
	}
	for _, p := range per {
		family(p.name, p.help)
		for slot := 0; slot < cartridge.Slots; slot++ {
			fmt.Fprintf(&b, "%s{slot=\"%d\",lab=\"%s\"} %d\n", p.name, slot, lab(slot), cartridge.SlotStat(s, slot, p.off))
		}
	}
	var queued uint64
	for slot := 0; slot < cartridge.Slots; slot++ {
		queued += cartridge.SlotStat(s, slot, cartridge.StatQueueOK)
	}
	taken := s[cartridge.StatTickJobsDone] + s[cartridge.StatTickStale]
	depth := uint64(0)
	if queued > taken {
		depth = queued - taken
	}
	fmt.Fprintf(&b, "# HELP unheaded_cart_queue_depth ScoreJobs queued and not yet finished (queued - done - stale; a BPF queue has no length).\n# TYPE unheaded_cart_queue_depth gauge\nunheaded_cart_queue_depth %d\n", depth)
	for _, g := range []struct {
		name, help string
		idx        int
	}{
		{"unheaded_cart_scores_total", "Tick-stage jobs finished.", cartridge.StatTickJobsDone},
		{"unheaded_cart_xdp_packets_total", "Packets through cart_xdp.", cartridge.StatXDPPackets},
		{"unheaded_cart_breaker_trips_total", "Slots disabled by the fault breaker.", cartridge.StatBreakerTrips},
		{"unheaded_cart_events_dropped_total", "EMIT records dropped (ringbuf full).", cartridge.StatEventsDrop},
	} {
		family(g.name, g.help)
		fmt.Fprintf(&b, "%s %d\n", g.name, s[g.idx])
	}
	// Loaded cartridge names, as info.
	keys := make([]int, 0, len(slots))
	for k := range slots {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	b.WriteString("# HELP unheaded_cart_slot_info Cartridge loaded per slot.\n# TYPE unheaded_cart_slot_info gauge\n")
	for _, k := range keys {
		st := slots[k]
		fmt.Fprintf(&b, "unheaded_cart_slot_info{slot=\"%d\",name=%q,sha256=%q,gen=\"%d\",lab=\"%s\"} 1\n", k, st.Name, st.SHA256, st.Gen, lab(k))
	}
	return b.String()
}

// isLoopbackAddr reports whether a listen address binds loopback only.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
