// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package uids defines the canonical run-as identity for every third-party
// service the Kingdom executes. This file is the SINGLE SOURCE OF TRUTH for
// UID/GID assignments, in the same way pkg/ports is for port assignments.
//
// # Why a registry
//
// The kernel enforces numbers, not names. A file on a bind mount, hostPath, or
// PersistentVolume records its owner as an integer; the container's
// /etc/passwd is a lookup table the host never consults. That makes the
// numeric UID part of a deployment's ABI the moment any state is persisted.
//
// Two failure modes this registry exists to prevent:
//
//  1. Running as root (UID 0), or leaving the identity unspecified so the
//     image's default applies. Third-party code is untrusted code; a dedicated
//     unprivileged account is the cheapest blast-radius bound available.
//
//  2. Sharing one account across services — the state this replaced, where
//     grafana, prometheus, loki, victoriametrics and node-exporter all ran as
//     65534 (nobody). A shared UID means a compromise of any one of them can
//     read and rewrite the on-disk state of all the others, and it makes
//     file ownership useless for attributing writes during an audit.
//
// Every entry below is a dedicated, non-login, least-privilege identity: one
// service, one account, no shell, only the access that service needs.
package uids

import "fmt"

// Reserved range. Sits far above the 1000+ block that host useradd allocates
// to real users, so a Kingdom UID can never collide with a human account on
// WEST, EAST, or any adopter's host.
const (
	RangeStart = 16600
	RangeEnd   = 16799
)

// Telemetry tier (16700-16719). These carry persistent state on disk, so
// changing an assignment here orphans existing volumes — treat these numbers
// as append-only. To retire one, leave the constant in place and mark it
// reserved rather than reusing the number for a different service.
const (
	Prometheus      = 16700
	Grafana         = 16701
	Loki            = 16702
	VictoriaMetrics = 16703
	Promtail        = 16704
	NodeExporter    = 16705
)

// Kingdom services tier (16720-16759). These are Unheaded's own Go binaries
// deployed by the Helm chart. They previously all shared global.security.runAsUser
// = 1000 — the same shared-account problem as the telemetry tier, one layer up,
// and 1000 is the first normal-user UID on most Linux systems, so it collides
// with a real human account wherever userns mapping or a hostPath is in play.
const (
	Wotan            = 16720
	UnheadedDaemon   = 16721
	Timeguru         = 16722
	Architect        = 16723
	Captain          = 16724
	Micromanager     = 16725
	Monad            = 16726
	Sophia           = 16727
	DashboardBackend = 16728
	KanbanApp        = 16729
	Gateway          = 16730
)

// Armory tier (16740-16759). Infrastructure components under deploy/k8s/armory.
// These manifests set runAsNonRoot but no runAsUser, which stops root without
// giving each service an identity of its own — the UID then comes from whatever
// the image happens to declare, and two images can easily share one.
const (
	Cuirass     = 16740
	Gauntlets   = 16741
	Gorget      = 16742
	HelmRuntime = 16743
	Pauldrons   = 16744
	Shield      = 16745
	Sword       = 16746
)

// Gnostic tier (16760-16779). State and history services.
const (
	Anamnesis = 16760
	Kenoma    = 16761
	Pleroma   = 16762
)

// Registry maps each service name to its assigned UID. Deployment manifests
// are validated against this map by TestManifestsMatchRegistry.
var Registry = map[string]int{
	"prometheus":      Prometheus,
	"grafana":         Grafana,
	"loki":            Loki,
	"victoriametrics": VictoriaMetrics,
	"promtail":        Promtail,
	"node-exporter":   NodeExporter,

	"wotan":             Wotan,
	"unheaded-daemon":   UnheadedDaemon,
	"timeguru":          Timeguru,
	"architect":         Architect,
	"captain":           Captain,
	"micromanager":      Micromanager,
	"monad":             Monad,
	"sophia":            Sophia,
	"dashboard-backend": DashboardBackend,
	"kanban-app":        KanbanApp,
	"gateway":           Gateway,

	"cuirass":      Cuirass,
	"gauntlets":    Gauntlets,
	"gorget":       Gorget,
	"helm-runtime": HelmRuntime,
	"pauldrons":    Pauldrons,
	"shield":       Shield,
	"sword":        Sword,

	"anamnesis": Anamnesis,
	"kenoma":    Kenoma,
	"pleroma":   Pleroma,
}

// Lookup returns the assigned UID for a service.
func Lookup(service string) (int, error) {
	uid, ok := Registry[service]
	if !ok {
		return 0, fmt.Errorf("no UID assigned for service %q — add one to pkg/uids", service)
	}
	return uid, nil
}

// Validate enforces the registry's invariants: every UID is unique, inside the
// reserved range, and never root. A duplicate here is the shared-account bug
// this package exists to prevent, so it is an error rather than a warning.
func Validate() error {
	seen := make(map[int]string, len(Registry))
	for service, uid := range Registry {
		if uid == 0 {
			return fmt.Errorf("service %q assigned UID 0 (root)", service)
		}
		if uid < RangeStart || uid > RangeEnd {
			return fmt.Errorf("service %q UID %d outside reserved range [%d, %d]",
				service, uid, RangeStart, RangeEnd)
		}
		if other, dup := seen[uid]; dup {
			return fmt.Errorf("UID %d shared by %q and %q — each service needs its own identity",
				uid, other, service)
		}
		seen[uid] = service
	}
	return nil
}
