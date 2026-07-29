// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package uids

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// helmValueKeys maps a Helm values.yaml top-level key to its registry name.
// The chart uses camelCase keys while the registry (and the rendered workload
// names) use kebab-case.
var helmValueKeys = map[string]string{
	"wotan":            "wotan",
	"unheadedDaemon":   "unheaded-daemon",
	"timeguru":         "timeguru",
	"architect":        "architect",
	"captain":          "captain",
	"micromanager":     "micromanager",
	"monad":            "monad",
	"sophia":           "sophia",
	"dashboardBackend": "dashboard-backend",
	"kanbanApp":        "kanban-app",
	"gateway":          "gateway",
}

// TestHelmValuesMatchRegistry is the enforcement half for the Helm chart, the
// counterpart to TestManifestsMatchRegistry for the raw K8s manifests.
//
// The chart previously set global.security.runAsUser = 1000 for EVERY service:
// the same shared-account problem as the telemetry tier, one layer up. 1000 is
// also the first normal-user UID on most Linux systems, so it collides with a
// real human account wherever userns mapping or a hostPath is involved.
//
// Without this test, a new service can be added to values.yaml with no uid and
// silently inherit the shared fallback.
func TestHelmValuesMatchRegistry(t *testing.T) {
	path := filepath.Join("..", "..", "helm", "unheaded", "values.yaml")
	data, err := os.ReadFile(path) //nolint:gosec // fixed repo-relative path
	if err != nil {
		t.Skipf("helm values not readable: %v", err)
	}
	src := string(data)

	for key, service := range helmValueKeys {
		want, ok := Registry[service]
		if !ok {
			t.Errorf("%s: no UID assigned in pkg/uids", service)
			continue
		}

		// Find the service block and its uid: line.
		re := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(key) + `:\n(.*?)(?:\n[a-zA-Z]|\z)`)
		m := re.FindStringSubmatch(src)
		if m == nil {
			t.Errorf("%s: block not found in values.yaml", key)
			continue
		}
		uidRe := regexp.MustCompile(`(?m)^\s*uid:\s*(\d+)`)
		um := uidRe.FindStringSubmatch(m[1])
		if um == nil {
			t.Errorf("%s: no uid set — it would inherit the shared global fallback", key)
			continue
		}
		got, err := strconv.Atoi(um[1])
		if err != nil {
			t.Errorf("%s: unparseable uid %q", key, um[1])
			continue
		}
		if got != want {
			t.Errorf("%s: values.yaml uid=%d, pkg/uids says %d", key, got, want)
		}
	}
}

// TestHelmGlobalFallbackIsNotAHumanUID guards the fallback itself. If a service
// ever does slip through without a uid, it must not land on a UID that a real
// person could own on the host.
func TestHelmGlobalFallbackIsNotAHumanUID(t *testing.T) {
	path := filepath.Join("..", "..", "helm", "unheaded", "values.yaml")
	data, err := os.ReadFile(path) //nolint:gosec // fixed repo-relative path
	if err != nil {
		t.Skipf("helm values not readable: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\s*runAsUser:\s*(\d+)`)
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		uid, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if uid == 0 {
			t.Error("global fallback runAsUser is 0 (root)")
		}
		if uid < RangeStart || uid > RangeEnd {
			t.Errorf("runAsUser %d is outside the reserved range [%d, %d]; "+
				"UIDs below 1000 collide with system accounts and 1000+ with real users",
				uid, RangeStart, RangeEnd)
		}
	}
	if !strings.Contains(string(data), "FALLBACK ONLY") {
		t.Error("the global runAsUser should be documented as a fallback, not a default")
	}
}
