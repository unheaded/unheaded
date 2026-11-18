// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/discovery"
)

// Discovery is off unless the config or --discover-subnets names subnets
// (ADR-147 Phase 9); the flag wins; a bad prefix is an error.
func TestDiscoverSubnets(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	plain := write("plain.yaml", "targets:\n  - name: wotan\n    host: localhost\n    port: 18000\n")
	on := write("on.yaml", "targets: []\ndiscover:\n  subnets: [\"172.28.1.0/24\"]\n")
	bad := write("bad.yaml", "discover:\n  subnets: [\"not-a-prefix\"]\n")

	if got, err := discoverSubnets("", ""); err != nil || got != nil {
		t.Fatalf("no config, no flag: %v %v, want off", got, err)
	}
	if got, err := discoverSubnets(plain, ""); err != nil || got != nil {
		t.Fatalf("config without discover: %v %v, want off", got, err)
	}
	if got, err := discoverSubnets(on, ""); err != nil || len(got) != 1 || got[0].String() != "172.28.1.0/24" {
		t.Fatalf("config: %v %v", got, err)
	}
	if got, err := discoverSubnets(on, "10.10.20.0/24,192.168.13.0/30"); err != nil || len(got) != 2 || got[0].String() != "10.10.20.0/24" {
		t.Fatalf("flag wins: %v %v", got, err)
	}
	if _, err := discoverSubnets(bad, ""); err == nil {
		t.Fatal("a bad prefix was accepted")
	}
	if _, err := discoverSubnets("", "0.0.0.0/0"); err == nil {
		t.Fatal("a /0 was accepted")
	}
}

// REV-8: a discovered service that stops announcing is reaped from the
// registry after its TTL; Akira keeps checking it at its last address for
// discoverKeep, so a dead service goes red instead of vanishing, then drops
// it with one log line.
func TestReapedServiceStaysChecked(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	var lines []string
	m := newDiscoverMemory(func() time.Time { return now }, func(f string, a ...any) { lines = append(lines, fmt.Sprintf(f, a...)) })
	live := []discovery.ServiceEntry{{Name: "kanban-app", Address: "172.28.1.30", Port: 20001}}
	got := m.targets(live)
	if len(got) != 1 || got[0].Host != "172.28.1.30" || got[0].ServerName != "kanban-app" {
		t.Fatalf("live: %+v", got)
	}
	// Reaped: the registry no longer lists it.
	now = now.Add(discovery.DefaultTTL * 2)
	got = m.targets(nil)
	if len(got) != 1 || got[0].Host != "172.28.1.30" || got[0].Port != 20001 {
		t.Fatalf("reaped %v ago: %+v, want it still checked at its last address", discovery.DefaultTTL*2, got)
	}
	if len(lines) != 0 {
		t.Errorf("logged while still kept: %v", lines)
	}
	now = now.Add(discoverKeep)
	if got = m.targets(nil); len(got) != 0 {
		t.Fatalf("after discoverKeep: %+v, want dropped", got)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "kanban-app") {
		t.Fatalf("drop logged %v, want one line naming kanban-app", lines)
	}
	if got = m.targets(nil); len(got) != 0 || len(lines) != 1 {
		t.Fatalf("a dropped name came back or logged again: %+v %v", got, lines)
	}
	// Announcing again brings it back.
	if got = m.targets(live); len(got) != 1 {
		t.Fatalf("announced again: %+v", got)
	}
}

func TestDiscoverMemoryBounded(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	m := newDiscoverMemory(func() time.Time { return now }, func(string, ...any) {})
	for i := 0; i < discovery.MaxDiscovered+10; i++ {
		m.targets([]discovery.ServiceEntry{{Name: "svc-" + strconv.Itoa(i), Address: "172.28.1.1", Port: 1000 + i}})
		now = now.Add(time.Second)
	}
	if n := len(m.targets(nil)); n > discovery.MaxDiscovered {
		t.Fatalf("remembers %d, want at most %d", n, discovery.MaxDiscovered)
	}
}
