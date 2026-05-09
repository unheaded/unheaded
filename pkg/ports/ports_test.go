// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ports

import (
	"testing"
)

// allPorts is the canonical map of every named port constant in ports.go.
// MUST stay in sync with that file — when you add a new const, add it here too.
// CustomerStart / CustomerEnd are RANGE markers, not service ports, so they
// are listed for reference but excluded from collision and Doom-Range checks
// (CustomerEnd 26666 is the inclusive end of the Doom Range itself).
//
// Last sync: 2026-05-09 (added ProtocolAPI, KeyMgmt, PQCVerifier, Shield,
// the 10 Monad CPU Application Services 19010-19019, the 6 AI Model Stack
// entries 20100-20106, and ASCEND-LINUX UPCTtyBridge 26100).
func allPorts(t *testing.T) map[string]int {
	t.Helper()
	return map[string]int{
		// Infrastructure
		"DoomBridge":            DoomBridge,
		"DoomGoInjector":        DoomGoInjector,
		"TraceCollectorHTTP":    TraceCollectorHTTP,
		"TraceCollectorMetrics": TraceCollectorMetrics,
		// Control Plane
		"DaemonHTTP":      DaemonHTTP,
		"DaemonGRPC":      DaemonGRPC,
		"CLIServer":       CLIServer,
		"ProtocolAPIREST": ProtocolAPIREST,
		"ProtocolAPIGRPC": ProtocolAPIGRPC,
		// Wotan
		"WotanHTTP": WotanHTTP,
		"WotanGRPC": WotanGRPC,
		// Core Services
		"Timeguru":     Timeguru,
		"Architect":    Architect,
		"Captain":      Captain,
		"Micromanager": Micromanager,
		"Monad":        Monad,
		"Sophia":       Sophia,
		"Cuirass":      Cuirass,
		"KeyMgmt":      KeyMgmt,
		"PQCVerifier":  PQCVerifier,
		"Shield":       Shield,
		// Monad CPU Application Services
		"Firewall":     Firewall,
		"Canary":       Canary,
		"Chaos":        Chaos,
		"QoS":          QoS,
		"BackupMgr":    BackupMgr,
		"Compliance":   Compliance,
		"NFV":          NFV,
		"VersionSvc":   VersionSvc,
		"Anomaly":      Anomaly,
		"TelemetryAgg": TelemetryAgg,
		// Applications
		"DashboardBackend": DashboardBackend,
		"KanbanApp":        KanbanApp,
		"WikiServer":       WikiServer,
		// AI Model Stack
		"VLLMDeepSeek": VLLMDeepSeek,
		"VLLMQwen":     VLLMQwen,
		"Qdrant":       Qdrant,
		"BGEM3":        BGEM3,
		"SophiaEye":    SophiaEye,
		"AIWebUI":      AIWebUI,
		// Gateway
		"GatewayHTTP":  GatewayHTTP,
		"GatewayHTTPS": GatewayHTTPS,
		// Utilities
		"CertGen": CertGen,
		// ASCEND-LINUX
		"UPCTtyBridge": UPCTtyBridge,
	}
}

func TestNoDuplicatePorts(t *testing.T) {
	seen := make(map[int]string)
	for name, port := range allPorts(t) {
		if existing, ok := seen[port]; ok {
			t.Errorf("DUPLICATE PORT %d: %s and %s", port, existing, name)
		}
		seen[port] = name
	}
}

func TestAllPortsInDoomRange(t *testing.T) {
	const (
		doomLo = 16666
		doomHi = 26666
	)
	for name, port := range allPorts(t) {
		if port < doomLo || port > doomHi {
			t.Errorf("Port %s=%d outside Doom Range (%d-%d)", name, port, doomLo, doomHi)
		}
	}
}

// TestUPCTtyBridgeInUserAppBand asserts the ASCEND-LINUX bridge sits in the
// 26000-26666 user-app band per pkg/ports/ports.go's "Customer Apps" comment.
func TestUPCTtyBridgeInUserAppBand(t *testing.T) {
	if UPCTtyBridge < CustomerStart || UPCTtyBridge > CustomerEnd {
		t.Errorf("UPCTtyBridge=%d not in user-app band [%d, %d]",
			UPCTtyBridge, CustomerStart, CustomerEnd)
	}
}

// TestCustomerRangeBoundsConsistent ensures CustomerStart < CustomerEnd
// and both sit at the high end of the Doom Range.
func TestCustomerRangeBoundsConsistent(t *testing.T) {
	if CustomerStart >= CustomerEnd {
		t.Errorf("CustomerStart=%d should be < CustomerEnd=%d", CustomerStart, CustomerEnd)
	}
	if CustomerEnd != 26666 {
		t.Errorf("CustomerEnd=%d should equal 26666 (top of Doom Range)", CustomerEnd)
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
