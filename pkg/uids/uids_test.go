// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package uids

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

var (
	runAsUserRe = regexp.MustCompile(`(?m)^\s*runAsUser:\s*(\d+)`)
	fsGroupRe   = regexp.MustCompile(`(?m)^\s*fsGroup:\s*(\d+)`)
)

// TestManifestsMatchRegistry is the enforcement half of this package. Without
// it the registry is documentation, and documentation drifts away from the
// YAML that actually gets applied. Any manifest that runs as root, omits an
// identity, or disagrees with pkg/uids fails the build here.
func TestManifestsMatchRegistry(t *testing.T) {
	dir := filepath.Join("..", "..", "kubernetes", "manifests", "base", "telemetry")

	for service, want := range Registry {
		var path string
		for _, candidate := range []string{
			service + ".yaml",
			service + "-daemonset.yaml",
		} {
			p := filepath.Join(dir, candidate)
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
		if path == "" {
			t.Errorf("%s: no manifest found in %s", service, dir)
			continue
		}

		data, err := os.ReadFile(path) //nolint:gosec // fixed repo-relative path
		if err != nil {
			t.Errorf("%s: %v", service, err)
			continue
		}

		m := runAsUserRe.FindSubmatch(data)
		if m == nil {
			t.Errorf("%s (%s): no runAsUser — the image default would apply, "+
				"which is how a service ends up running as root", service, filepath.Base(path))
			continue
		}
		got, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Errorf("%s: unparseable runAsUser %q", service, m[1])
			continue
		}
		if got != want {
			t.Errorf("%s (%s): runAsUser=%d, registry says %d",
				service, filepath.Base(path), got, want)
		}

		// Where a manifest sets fsGroup it must agree with runAsUser, or the
		// service cannot write the volume it was just given ownership of.
		if fm := fsGroupRe.FindSubmatch(data); fm != nil {
			fg, err := strconv.Atoi(string(fm[1]))
			if err == nil && fg != want {
				t.Errorf("%s (%s): fsGroup=%d disagrees with runAsUser=%d",
					service, filepath.Base(path), fg, want)
			}
		}
	}
}
