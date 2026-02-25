// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ports

import (
	"testing"
)

func TestNoDuplicatePorts(t *testing.T) {
	seen := make(map[int]string)
	portMap := map[string]int{
		"DoomBridge":            DoomBridge,
		"DoomGoInjector":        DoomGoInjector,
		"TraceCollectorHTTP":    TraceCollectorHTTP,
		"TraceCollectorMetrics": TraceCollectorMetrics,
		"DaemonHTTP":            DaemonHTTP,
		"DaemonGRPC":            DaemonGRPC,
		"CLIServer":             CLIServer,
		"WotanHTTP":             WotanHTTP,
		"WotanGRPC":             WotanGRPC,
		"Timeguru":              Timeguru,
		"Architect":             Architect,
		"Captain":               Captain,
		"Micromanager":          Micromanager,
		"Monad":                 Monad,
		"Sophia":                Sophia,
		"Cuirass":               Cuirass,
		"DashboardBackend":      DashboardBackend,
		"KanbanApp":             KanbanApp,
		"WikiServer":            WikiServer,
		"GatewayHTTP":           GatewayHTTP,
		"GatewayHTTPS":          GatewayHTTPS,
		"CertGen":               CertGen,
	}
	for name, port := range portMap {
		if existing, ok := seen[port]; ok {
			t.Errorf("DUPLICATE PORT %d: %s and %s", port, existing, name)
		}
		seen[port] = name
	}
}

func TestAllPortsInDoomRange(t *testing.T) {
	ports := []int{
		DoomBridge, DoomGoInjector, TraceCollectorHTTP, TraceCollectorMetrics,
		DaemonHTTP, DaemonGRPC, CLIServer,
		WotanHTTP, WotanGRPC,
		Timeguru, Architect, Captain, Micromanager, Monad, Sophia, Cuirass,
		DashboardBackend, KanbanApp, WikiServer,
		GatewayHTTP, GatewayHTTPS,
		CertGen,
	}
	for _, p := range ports {
		if p < 16666 || p > 26666 {
			t.Errorf("Port %d outside Doom Range (16666-26666)", p)
		}
	}
}

func TestDefaultAddr(t *testing.T) {
	got := DefaultAddr(19000)
	if got != ":19000" {
		t.Errorf("DefaultAddr(19000) = %q, want %q", got, ":19000")
	}
}

func TestDefaultWotanGRPC(t *testing.T) {
	got := DefaultWotanGRPC()
	if got != "localhost:18001" {
		t.Errorf("DefaultWotanGRPC() = %q, want %q", got, "localhost:18001")
	}
}

func TestDefaultWotanHTTP(t *testing.T) {
	got := DefaultWotanHTTP()
	if got != "http://localhost:18000" {
		t.Errorf("DefaultWotanHTTP() = %q, want %q", got, "http://localhost:18000")
	}
}
