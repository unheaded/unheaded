// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package uids

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestValidate_CurrentRegistry(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("shipped registry is invalid: %v", err)
	}
}

func TestValidate_RejectsRoot(t *testing.T) {
	orig := Registry
	t.Cleanup(func() { Registry = orig })

	Registry = map[string]int{"grafana": 0}
	if err := Validate(); err == nil {
		t.Fatal("expected UID 0 to be rejected")
	}
}

func TestValidate_RejectsSharedUID(t *testing.T) {
	orig := Registry
	t.Cleanup(func() { Registry = orig })

	// This is exactly the state being replaced: several services on one
	// account. If Validate ever accepts it, the regression is silent.
	Registry = map[string]int{
		"grafana":    65534,
		"prometheus": 65534,
	}
	if err := Validate(); err == nil {
		t.Fatal("expected a shared UID across services to be rejected")
	}
}

func TestValidate_RejectsOutOfRange(t *testing.T) {
	orig := Registry
	t.Cleanup(func() { Registry = orig })

	Registry = map[string]int{"grafana": 1000}
	if err := Validate(); err == nil {
		t.Fatal("expected a UID outside the reserved range to be rejected")
	}
}

func TestLookup_UnknownService(t *testing.T) {
	if _, err := Lookup("nonexistent"); err == nil {
		t.Fatal("expected an error for an unassigned service")
	}
}

// hostOnly lists services that run only as host systemd units, under a
// sysusers account whose ID is pinned to the registry's UID
// (TestHostOnlySysusersPinned checks the pin).
var hostOnly = map[string]string{
	// Faults need CAP_NET_ADMIN (and CAP_BPF on west) on the host itself:
	// S-REAL-STEEL Phase 10, deploy/systemd/unheaded-chaos-controller.service.
	"chaos-controller": "unheaded-chaos",
	// The UPC boot controller: CAP_BPF/NET_ADMIN/NET_RAW/PERFMON inside the
	// upc0 namespace, S-ASCEND-2 Phase 5, unheaded-upc-bootctl@.service.
	"upc-bootctl": "unheaded-upc",
	// The anomaly service and its Wotan intake: deploy/systemd/unheaded-anomaly.service.
	"anomaly": "unheaded-anomaly",
	// The dead-man's switch sender on east and the CI status writer on
	// west: deploy/systemd/unheaded-heartbeat.service, unheaded-ci-status.service.
	"heartbeat": "unheaded-heartbeat",
	"ci-status": "unheaded-ci-status",
	// The Well's weekly retention job: deploy/systemd/unheaded-well-retention.service.
	"well-retention": "unheaded-well-retention",
	// The built-in issuer (ADR-103): deploy/systemd/unheaded-auth.service.
	// It also runs as compose service "auth" with the same uid; listing it
	// here keeps the sysusers pin checked.
	"unheaded-auth": "unheaded-auth",
	// The drift monitor reads root-only files with CAP_DAC_READ_SEARCH on the
	// host it watches: deploy/systemd/unheaded-heimdall.service (S-IRON-GATE 276).
	"heimdall": "unheaded-heimdall",
	// No unit: the account owns the evidence build key (0400) on the custody
	// host and runs `yggdrasil-evidence cask sign` (S-IRON-GATE 339, E1;
	// runbooks/security/build-key-ceremony.yaml).
	"buildkey": "unheaded-buildkey",
	// The syslog receiver: deploy/systemd/unheaded-syslog-relay.service (ADR-124).
	"syslog-relay": "unheaded-syslog-relay",
	// The retrieval corpus server: deploy/systemd/unheaded-vor.service (DP-712).
	"vor": "unheaded-vor",
	// The web UI's TLS front: deploy/systemd/unheaded-zhen-ui-front.service (ADR-138).
	"zhen-ui-front": "unheaded-zhen-ui-front",
}

// composeOnly lists services that exist only in compose files (or as the
// Dockerfile's default stage). TestComposeMatchesRegistry and
// TestAltComposeMatchesRegistry check them instead.
var composeOnly = map[string]bool{
	"akira":             true,
	"alertmanager":      true,
	"alert-mailer":      true,
	"all-in-one-dev":    true,
	"clickhouse-schema": true,
	"embeddings":        true,
	"haproxy-edge":      true,
	"haproxy-internal":  true,
	"nginx-dashboard":   true,
	"nginx-gateway":     true,
	"nginx-kanban":      true,
	"nginx-monad":       true,
	"nginx-sophia":      true,
	"nginx-wotan":       true,
	"open-webui":        true,
	"qdrant":            true,
	"sophia-gateway":    true,
	"coredns":           true,
	"postgres":          true,
	"traefik":           true,
}

// TestHostOnlySysusersPinned: a host-only service's sysusers line carries
// the registry UID, so the registry asserts what the host really runs.
func TestHostOnlySysusersPinned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "sysusers.d", "unheaded.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for service, account := range hostOnly {
		want, ok := Registry[service]
		if !ok {
			t.Errorf("%s: host-only but not in the registry", service)
			continue
		}
		found := false
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) >= 3 && f[0] == "u" && f[1] == account {
				found = true
				if f[2] != strconv.Itoa(want) {
					t.Errorf("%s: sysusers %s has ID %s, registry says %d", service, account, f[2], want)
				}
			}
		}
		if !found {
			t.Errorf("%s: no sysusers line for %s", service, account)
		}
	}
}
