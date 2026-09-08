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
	base := filepath.Join("..", "..", "kubernetes", "manifests", "base")

	// Find a manifest for each service anywhere under manifests/base. Services
	// deployed only by the Helm chart have none here; those are covered by
	// TestHelmValuesMatchRegistry instead. Skipping them is deliberate — the
	// point is that wherever a service IS declared, it matches the registry.
	found := 0
	for service, want := range Registry {
		var path string
		_ = filepath.WalkDir(base, func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() || path != "" {
				return nil //nolint:nilerr // a missing dir is not fatal here
			}
			name := e.Name()
			if name == service+".yaml" || name == service+"-daemonset.yaml" {
				path = p
			}
			return nil
		})
		if path == "" {
			continue // Helm-only service
		}
		found++

		data, err := os.ReadFile(path) //nolint:gosec // path built by walking a fixed repo dir
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

	if found == 0 {
		t.Error("no manifests matched any registry entry — the walk is probably looking in the wrong place")
	}
}
