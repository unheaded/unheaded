// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"io"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"
)

const (
	serviceUnit = "../../deploy/systemd/unheaded-syslog-relay.service"
	socketUnit  = "../../deploy/systemd/unheaded-syslog-relay.socket"
)

// directives reads KEY=value lines, every value per key, in order.
func directives(t *testing.T, path string) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- a fixed repo path
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = append(out[k], v)
		}
	}
	return out
}

// The relay refuses to start when systemd passes a socket its --listen does
// not name, or names one systemd did not pass. Catch that here, not on west.
func TestSocketUnitMatchesExecStart(t *testing.T) {
	svc, sock := directives(t, serviceUnit), directives(t, socketUnit)
	if len(svc["ExecStart"]) != 1 {
		t.Fatalf("ExecStart %q", svc["ExecStart"])
	}
	argv := strings.Fields(svc["ExecStart"][0])
	cfg, err := parseFlags(argv[1:], io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("the unit's ExecStart does not parse: %v", err)
	}
	var udp, tcp []string
	for _, l := range cfg.listens {
		if l.network == "udp" {
			udp = append(udp, l.addr.String())
		} else {
			tcp = append(tcp, l.addr.String())
		}
	}
	for _, c := range []struct {
		key  string
		want []string
	}{{"ListenDatagram", udp}, {"ListenStream", tcp}} {
		got := slices.Clone(sock[c.key])
		for _, a := range got {
			if _, err := netip.ParseAddrPort(a); err != nil {
				t.Errorf("%s=%s: an address and port, never a bare port (a wildcard): %v", c.key, a, err)
			}
		}
		slices.Sort(got)
		slices.Sort(c.want)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s %v, but ExecStart --listen says %v", c.key, got, c.want)
		}
	}
	if got := sock["Service"]; len(got) != 1 || got[0] != "unheaded-syslog-relay.service" {
		t.Errorf("socket Service= %v", got)
	}
}

// The repo's sources file, as the relay reads it: Pi-hole's tagged files reach
// their own rows, everything else from its address the journal row, and each
// carries its row's retention, not the facility rsyslog sets.
func TestRepoSourcesStampPihole(t *testing.T) {
	r := &relay{cfg: config{sources: "../../config/logging/sources.yaml", rate: defaultRate, burst: defaultBurst}}
	list, err := r.loadAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	s := list.senders[netip.MustParseAddr("10.10.20.252")]
	if s == nil {
		t.Fatal("10.10.20.252 is not a sender")
	}
	for tag, want := range map[string]row{
		"pihole-dns":  {"pihole/dns", "pihole", "operational"},
		"pihole-ftl":  {"pihole/ftl", "pihole", "operational"},
		"pihole-web":  {"pihole/web", "pihole", "audit"},
		"pihole-FTL":  {"pihole/journal", "pihole", "operational"},
		"cron":        {"pihole/journal", "pihole", "operational"},
		"":            {"pihole/journal", "pihole", "operational"},
		"pihole-dns ": {"pihole/journal", "pihole", "operational"},
	} {
		if got := s.source(tag); got != want {
			t.Errorf("tag %q: %+v, want %+v", tag, got, want)
		}
	}
}

// Answer #18 of 2026-11-09: no capability, its own user namespace, and it
// binds only the metrics port itself.
func TestServiceUnitHoldsNoCapability(t *testing.T) {
	svc := directives(t, serviceUnit)
	if v, ok := svc["AmbientCapabilities"]; ok {
		t.Errorf("AmbientCapabilities=%v", v)
	}
	if got := svc["CapabilityBoundingSet"]; len(got) != 1 || got[0] != "" {
		t.Errorf("CapabilityBoundingSet=%v, want it empty", got)
	}
	if got := svc["PrivateUsers"]; len(got) != 1 || got[0] != "yes" {
		t.Errorf("PrivateUsers=%v", got)
	}
	for _, a := range svc["SocketBindAllow"] {
		if strings.HasSuffix(a, ":514") {
			t.Errorf("SocketBindAllow=%s: 514 comes bound from the socket unit", a)
		}
	}
	if !slices.Contains(svc["Requires"], "unheaded-syslog-relay.socket") {
		t.Errorf("Requires=%v does not name the socket", svc["Requires"])
	}
}
