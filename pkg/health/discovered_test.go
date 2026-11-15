// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package health

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okServer(t *testing.T) (string, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(srv.Close)
	a := srv.Listener.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port
}

// TestSweepIncludesDiscovered (ADR-147 Phase 9, D3 = A): a service that
// announced itself and was admitted is checked with no line in the config,
// and its verdict says it was discovered.
func TestSweepIncludesDiscovered(t *testing.T) {
	host, port := okServer(t)
	a := NewAkira("west", []ServiceTarget{{Name: "timeguru", Host: host, Port: port, Scheme: "http"}})
	a.SetDiscovered(func() []ServiceTarget {
		return []ServiceTarget{{Name: "sword-east", Host: host, Port: port, Scheme: "http"}}
	})
	reports := a.CheckAll()
	if len(reports) != 2 || reports[1].Service != "sword-east" || !reports[1].Healthy {
		t.Fatalf("reports %+v, want timeguru and the discovered sword-east, healthy", reports)
	}
	a.EvaluateConsensus()
	st := a.GetStates()
	if st["sword-east"].Source != "discovered" || st["timeguru"].Source != "" {
		t.Errorf("sources: sword-east %q, timeguru %q", st["sword-east"].Source, st["timeguru"].Source)
	}
}

// TestConfiguredWinsOverDiscovered: a discovered name the config already
// lists is not checked twice; the configured target is used.
func TestConfiguredWinsOverDiscovered(t *testing.T) {
	host, port := okServer(t)
	a := NewAkira("west", []ServiceTarget{{Name: "timeguru", Host: host, Port: port, Scheme: "http"}})
	a.SetDiscovered(func() []ServiceTarget {
		return []ServiceTarget{{Name: "timeguru", Host: "192.0.2.1", Port: 1, Scheme: "http"}}
	})
	reports := a.CheckAll()
	if len(reports) != 1 || !reports[0].Healthy {
		t.Fatalf("reports %+v, want one healthy timeguru (the configured target)", reports)
	}
}

// TestDiscoveryOffByDefault: without SetDiscovered only the config is swept.
func TestDiscoveryOffByDefault(t *testing.T) {
	host, port := okServer(t)
	a := NewAkira("west", []ServiceTarget{{Name: "timeguru", Host: host, Port: port, Scheme: "http"}})
	if n := len(a.CheckAll()); n != 1 {
		t.Fatalf("%d reports, want 1", n)
	}
}
