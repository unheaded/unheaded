// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package ports defines the canonical port assignments for all Unheaded services.
// The "Doom Range" (16666-26666) ensures no conflicts with common dev tools.
// This file is the SINGLE SOURCE OF TRUTH for port assignments.
package ports

import (
	"fmt"
	"net"
	"strconv"
)

const (
	// Infrastructure Tier (16666-16999)
	DoomBridge            = 16666
	DoomGoInjector        = 16667
	TraceCollectorHTTP    = 16670
	TraceCollectorMetrics = 16671

	// Control Plane (17000-17999)
	DaemonHTTP      = 17000
	DaemonGRPC      = 17001
	CLIServer       = 17010
	ProtocolAPIREST = 17100
	ProtocolAPIGRPC = 17101

	// Wotan — Message Bus (18000-18099)
	WotanHTTP = 18000
	WotanGRPC = 18001

	// Core Services (19000-19999)
	Timeguru     = 19000
	Architect    = 19001
	Captain      = 19002
	Micromanager = 19003
	Monad        = 19004
	Sophia       = 19005
	Cuirass      = 19006
	KeyMgmt      = 19007
	PQCVerifier  = 19008
	Shield       = 19009 // WAF / Zero-Trust gateway daemon (services/shield)

	// Monad CPU Application Services (19010-19019)
	Firewall     = 19010
	Sword        = 19011 // deploy service (cmd/sword); was Canary, folded into Sword (S-REAL-STEEL D-08)
	Chaos        = 19012
	QoS          = 19013
	BackupMgr    = 19014
	Compliance   = 19015
	NFV          = 19016
	VersionSvc   = 19017
	Anomaly      = 19018
	TelemetryAgg = 19019
	Auth         = 19020 // unheaded-auth issuer (ADR-103): login, JWKS, discovery

	// Kingdom health (19100-19109)
	Akira           = 19100 // consensus health daemon (ADR-029)
	HeimdallMetrics = 19101 // heimdall-daemon /metrics (loopback by default; M-085)

	// Threat-intel feeds (19120-19129; 19110 is loadbalancer xdpsync's)
	KEVPoller = 19120 // CISA KEV catalog poller (ADR-055)

	// Alerting (19130-19139)
	AlertMailer          = 19130 // alert-mailer: Alertmanager webhook to capped email (ADR-100)
	AlertMailerHeartbeat = 19131 // alert-mailer: dead-man's switch heartbeats from the other host only (ADR-100)

	// Log store (19140-19149)
	LogstoreWriter = 19140 // logstore-writer: Vector's second sink, journal rows into the Well (S-E1-LIVE Evening 16)

	// Applications (20000-20999)
	DashboardBackend = 20000
	KanbanApp        = 20001
	WikiServer       = 20002

	// AI Model Stack — Sophia's Eye (20100-20199)
	VLLMDeepSeek = 20100
	VLLMQwen     = 20101
	Qdrant       = 20102
	BGEM3        = 20104
	SophiaEye    = 20105
	AIWebUI      = 20106

	// Zhen (20110-20119). 20107 and 20108 are sophia-eye's published host
	// ports (deploy/sophia-eye/docker-compose.yml); DP-712.
	ZhenVor    = 20110 // vor serve: retrieval corpus
	ZhenAgentd = 20111 // zhen-agentd HTTP API
	// ZhenWebUI is the web UI's TLS listener, zhen-ui-front (ADR-138). The
	// number is older than this block: the Flask app held it until the front
	// took the listener and the app moved to a Unix socket.
	ZhenWebUI = 20103
	// ZhenUIFrontMetrics is zhen-ui-front's loopback /metrics and /health.
	ZhenUIFrontMetrics = 20112

	// Gateway (21000-21443)
	GatewayHTTP  = 21000
	GatewayHTTPS = 21443

	// Utilities (22000-22099)
	CertGen = 22000

	// Customer Apps (26000-26666)
	CustomerStart = 26000
	CustomerEnd   = 26666

	// ASCEND-LINUX (per references/battle-plan-ascend-linux-2026-05-08.md):
	// UPC TTY bridge for Mode A demo surface (browser xterm).
	UPCTtyBridge = 26100

	// MBC cartridges (ADR-108): upc-cartridgectl serve, metrics/health
	// (loopback by default).
	UPCCartridgeMetrics = 26610
)

// DefaultAddr returns ":<port>" string for net.Listen.
func DefaultAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}

// LoopbackAddr returns "127.0.0.1:<port>", the default bind of a listener
// nothing off the host needs.
func LoopbackAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// PortOf returns the port of a listen address such as ":19006" or
// "0.0.0.0:17000", or fallback when addr has no valid port.
func PortOf(addr string, fallback int) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return fallback
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return fallback
	}
	return n
}

// DefaultWotanGRPC returns the default Wotan gRPC dial target.
func DefaultWotanGRPC() string {
	return fmt.Sprintf("localhost:%d", WotanGRPC)
}

// DefaultWotanHTTP returns the default Wotan HTTP base URL.
func DefaultWotanHTTP() string {
	return fmt.Sprintf("http://localhost:%d", WotanHTTP)
}
