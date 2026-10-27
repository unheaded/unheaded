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
	Vector          = 16706
	// ClickHouse ran as the image's 101, which is _chrony on west and east:
	// the database and the time daemon were one identity on the host. The
	// image's writable dirs are 0777, so any uid works; existing volumes are
	// chowned once by compose's volume-init (2026-10-18).
	ClickHouse = 16707
	// ClickHouseSchema is compose's one-shot that applies
	// config/clickhouse/schema/ to the running ClickHouse (a client only;
	// owns no files).
	ClickHouseSchema = 16708
	// Alertmanager is monitoring/docker-compose.yml's (FND-001, FND-009).
	Alertmanager = 16709
	// AlertMailer is monitoring/docker-compose.yml's alert-mailer (ADR-100):
	// the only thing that sends alert mail, so the only holder of the SMTP
	// login.
	AlertMailer = 16710
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
	Akira            = 16731
	KEVPoller        = 16732
	// SophiaGateway is deploy/sophia-eye's gateway (its own compose stack).
	SophiaGateway = 16733
	// Anomaly is cmd/anomaly (services/anomaly), a host systemd unit only
	// (deploy/systemd/unheaded-anomaly.service, account unheaded-anomaly).
	Anomaly = 16734
	// Auth is cmd/unheaded-auth, the built-in OIDC issuer (ADR-103): host
	// unit unheaded-auth.service (account unheaded-auth) and compose service
	// "auth".
	Auth = 16735
	// AllInOneDev is the Dockerfile's default stage: every service under one
	// supervisord in one container (development only), so one account.
	AllInOneDev = 16759
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
	// docker/docker-compose.loadbalancers.yml (kingdom.armor: pauldrons).
	HAProxyEdge     = 16747
	HAProxyInternal = 16748
	NginxWotan      = 16749
	NginxMonad      = 16750
	NginxSophia     = 16751
	NginxDashboard  = 16752
	NginxKanban     = 16753
	NginxGateway    = 16754
)

// Gnostic tier (16760-16779). State and history services.
const (
	Anamnesis  = 16760
	Kenoma     = 16761
	Pleroma    = 16762
	Yaldabaoth = 16763 // chaos-controller (host account unheaded-chaos)
)

// Infrastructure tier (16780-16799). Workloads that hold host-level access —
// eBPF, packet capture, VPN, GPU. These need capabilities rather than root:
// verified on WEST (kernel 6.17, unprivileged_bpf_disabled=0) that CAP_BPF and
// CAP_PERFMON work for a non-root UID, and that ROCm device access needs
// membership of render (GID 992) and video (GID 44) rather than UID 0.
const (
	VoidCollector = 16780
	VLLM          = 16781
	// 16782 retired: Suricata, removed 2026-10-22. Not reused.
	WireGuard = 16783
	Traefik   = 16784
	CoreDNS   = 16785
	// deploy/sophia-eye: the second vLLM server, and the RAG stack around it.
	QwenCoder  = 16786
	Qdrant     = 16787
	Embeddings = 16788 // text-embeddings-inference
	OpenWebUI  = 16789
)

// Third-party image UIDs. These are NOT ours to choose: each image ships data
// directories owned by a specific UID, so imposing a Kingdom-range ID would
// leave the container unable to write its own state. The registry records what
// the image actually uses, so these two are checked against something rather
// than being the only unverified workloads.
//
// TestManifestsMatchRegistry caught exactly this: an earlier draft assigned
// ClickHouse 16785 and the test failed against the real 101. (ClickHouse has
// since moved to 16707 on purpose: 101 is a host account, _chrony.)
const (
	HAProxyIngress = 1000 // haproxytech/kubernetes-ingress image default
	Postgres       = 70   // postgres:*-alpine image: owns PGDATA on existing volumes
)

// Host group IDs required for device access, read from WEST rather than
// assumed. /dev/kfd is root:render mode 660, so ROCm needs the render group;
// video covers /dev/dri.
const (
	HostGroupRender = 992
	HostGroupVideo  = 44
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
	"vector":          Vector,

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
	"akira":             Akira,
	"kev-poller":        KEVPoller,
	"sophia-gateway":    SophiaGateway,
	"anomaly":           Anomaly,
	"unheaded-auth":     Auth,
	"all-in-one-dev":    AllInOneDev,

	"cuirass":      Cuirass,
	"gauntlets":    Gauntlets,
	"gorget":       Gorget,
	"helm-runtime": HelmRuntime,
	"pauldrons":    Pauldrons,
	"shield":       Shield,
	"sword":        Sword,

	"haproxy-edge":     HAProxyEdge,
	"haproxy-internal": HAProxyInternal,
	"nginx-wotan":      NginxWotan,
	"nginx-monad":      NginxMonad,
	"nginx-sophia":     NginxSophia,
	"nginx-dashboard":  NginxDashboard,
	"nginx-kanban":     NginxKanban,
	"nginx-gateway":    NginxGateway,

	"anamnesis":        Anamnesis,
	"kenoma":           Kenoma,
	"pleroma":          Pleroma,
	"chaos-controller": Yaldabaoth,

	"void-collector":    VoidCollector,
	"vllm":              VLLM,
	"wireguard":         WireGuard,
	"traefik":           Traefik,
	"coredns":           CoreDNS,
	"qwen-coder":        QwenCoder,
	"qdrant":            Qdrant,
	"embeddings":        Embeddings,
	"open-webui":        OpenWebUI,
	"haproxy-ingress":   HAProxyIngress,
	"clickhouse":        ClickHouse,
	"clickhouse-schema": ClickHouseSchema,
	"alertmanager":      Alertmanager,
	"alert-mailer":      AlertMailer,
	"postgres":          Postgres,
}

// Lookup returns the assigned UID for a service.
func Lookup(service string) (int, error) {
	uid, ok := Registry[service]
	if !ok {
		return 0, fmt.Errorf("no UID assigned for service %q — add one to pkg/uids", service)
	}
	return uid, nil
}

// thirdPartyUIDs are services whose UID is dictated by an upstream image and
// therefore legitimately sits outside the Kingdom reserved range. Listing them
// explicitly keeps the range check meaningful for everything else.
var thirdPartyUIDs = map[string]struct{}{
	"haproxy-ingress": {},
	"postgres":        {},
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
		if _, thirdParty := thirdPartyUIDs[service]; !thirdParty {
			if uid < RangeStart || uid > RangeEnd {
				return fmt.Errorf("service %q UID %d outside reserved range [%d, %d]",
					service, uid, RangeStart, RangeEnd)
			}
		}
		if other, dup := seen[uid]; dup {
			return fmt.Errorf("UID %d shared by %q and %q — each service needs its own identity",
				uid, other, service)
		}
		seen[uid] = service
	}
	return nil
}
