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
	// Both block style ("  runAsUser: 16720") and flow style
	// ("securityContext: { runAsUser: 16760, ... }") are in use in this repo.
	// A line-anchored pattern sees only the first, which made every flow-style
	// manifest under deploy/k8s/gnostic read as "no runAsUser" — a false alarm
	// that is just as damaging to a gate's credibility as a missed one.
	runAsUserRe = regexp.MustCompile(`(?m)(?:^|[{,])\s*runAsUser:\s*(\d+)`)
	fsGroupRe   = regexp.MustCompile(`(?m)(?:^|[{,])\s*fsGroup:\s*(\d+)`)
)

// TestManifestsMatchRegistry is the enforcement half of this package. Without
// it the registry is documentation, and documentation drifts away from the
// YAML that actually gets applied. Any manifest that runs as root, omits an
// identity, or disagrees with pkg/uids fails the build here.
// manifestRoots are every directory tree that can hold an applied manifest.
//
// It was `kubernetes/manifests/base` alone until 2026-09-08, which silently
// excluded deploy/k8s (void-collector, armory, gnostic, presentation) and the
// overlays (vllm, wireguard) — i.e. most of the UIDs this package had just
// introduced.
var manifestRoots = []string{
	filepath.Join("..", "..", "kubernetes", "manifests", "base"),
	filepath.Join("..", "..", "kubernetes", "manifests", "overlays"),
	filepath.Join("..", "..", "deploy", "k8s"),
}

// workloadFiles are the conventional per-directory manifest names. A service
// declared at <root>/<service>/deployment.yaml is matched by its DIRECTORY,
// which is how the 11 Kingdom services are laid out.
var workloadFiles = map[string]bool{
	"deployment.yaml":  true,
	"statefulset.yaml": true,
	"daemonset.yaml":   true,
	"pod.yaml":         true,
}

// helmOnly lists services with no raw manifest anywhere. They are covered by
// TestHelmValuesMatchRegistry instead.
//
// This is an explicit list rather than a `found == 0` check because that check
// could not fail: 8 pre-existing telemetry files always matched, so it stayed
// satisfied while most of the registry went unverified. Naming the exemptions
// means a service that vanishes from the walk fails the build instead of
// quietly joining the unchecked majority.
var helmOnly = map[string]bool{
	"haproxy-ingress": true,
	"unheaded-daemon": true,
}

// hostOnly lists services that run only as host systemd units, under a
// sysusers account whose ID is pinned to the registry's UID
// (TestHostOnlySysusersPinned checks the pin).
var hostOnly = map[string]string{
	// Faults need CAP_NET_ADMIN (and CAP_BPF on west) on the host itself:
	// S-REAL-STEEL Phase 10, deploy/systemd/unheaded-chaos-controller.service.
	"chaos-controller": "unheaded-chaos",
	// The anomaly service and its Wotan intake: deploy/systemd/unheaded-anomaly.service.
	"anomaly": "unheaded-anomaly",
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
	"qwen-coder":        true,
	"sophia-gateway":    true,
	"coredns":           true,
	"postgres":          true,
	"traefik":           true,
}

// pendingHardening are services whose registry UID is deliberately NOT yet
// applied to their manifest, with the reason. They are privileged network
// workloads where dropping to a non-root UID is a functional change that
// cannot be validated without a live cluster — so applying it blind would be
// worse than recording it.
//
// The registry entry is kept because the UID is reserved for them. This map is
// the honest form of that: the test still asserts the manifest EXISTS and still
// fails if one is deleted, it just does not yet demand the UID. A pending
// manifest may declare runAsUser: 0 explicitly; any other mismatch fails.
//
// The entry was measured on a live (kind) cluster on 2026-09-25. wireguard
// cannot close while it runs in a container: its entry records a proven limit,
// not unfinished work.
var pendingHardening = map[string]string{
	// Measured 2026-09-25: as uid 16783 with NET_ADMIN added, CapEff is 0 and
	// creating the link fails with EPERM. A kernel tunnel needs root here, so
	// this entry is permanent while WireGuard runs in a container. The manifest
	// is root with NET_ADMIN only (SYS_MODULE removed).
	"wireguard": "root required: non-root gets CapEff 0 and EPERM on link " +
		"creation (measured on kind); root is limited to NET_ADMIN",
}

// findManifests returns every manifest declaring the given service.
func findManifests(service string) []string {
	var paths []string
	for _, root := range manifestRoots {
		_ = filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil //nolint:nilerr // a missing dir is not fatal here
			}
			name := e.Name()
			if name == service+".yaml" ||
				name == service+"-daemonset.yaml" ||
				name == service+"-deployment.yaml" ||
				name == service+"-statefulset.yaml" ||
				(workloadFiles[name] && filepath.Base(filepath.Dir(p)) == service) {
				paths = append(paths, p)
			}
			return nil
		})
	}
	return paths
}

// TestManifestsMatchRegistry is the enforcement half of this package. Without
// it the registry is documentation, and documentation drifts away from the
// YAML that actually gets applied. Any manifest that runs as root, omits an
// identity, or disagrees with pkg/uids fails the build here.
func TestManifestsMatchRegistry(t *testing.T) {
	for service, want := range Registry {
		paths := findManifests(service)

		if len(paths) == 0 {
			if _, host := hostOnly[service]; !helmOnly[service] && !composeOnly[service] && !host {
				t.Errorf("%s: registry assigns UID %d but no manifest declares it, "+
					"and it is not listed as Helm-only — the registry is asserting "+
					"hardening that may never have been applied", service, want)
			}
			continue
		}
		if helmOnly[service] {
			t.Errorf("%s: listed as Helm-only but %d manifest(s) exist (%s) — "+
				"remove it from helmOnly so those are checked",
				service, len(paths), filepath.Base(paths[0]))
		}

		for _, path := range paths {
			rel := filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path))
			data, err := os.ReadFile(path) //nolint:gosec // path built by walking a fixed repo dir
			if err != nil {
				t.Errorf("%s: %v", service, err)
				continue
			}

			m := runAsUserRe.FindSubmatch(data)
			if m == nil {
				if why, ok := pendingHardening[service]; ok {
					t.Logf("%s (%s): runAsUser deliberately not applied yet — %s", service, rel, why)
					continue
				}
				t.Errorf("%s (%s): no runAsUser — the image default would apply, "+
					"which is how a service ends up running as root", service, rel)
				continue
			}
			got, err := strconv.Atoi(string(m[1]))
			if err != nil {
				t.Errorf("%s: unparseable runAsUser %q", service, m[1])
				continue
			}
			// A pending service may declare root EXPLICITLY: that states the
			// exception in the manifest instead of inheriting it from the image.
			// Any other mismatch, and root for a service not pending, still fails.
			if got == 0 && got != want {
				if why, ok := pendingHardening[service]; ok {
					t.Logf("%s (%s): runAsUser=0 declared, pending — %s", service, rel, why)
					continue
				}
			}
			if got != want {
				t.Errorf("%s (%s): runAsUser=%d, registry says %d", service, rel, got, want)
			}

			// Where a manifest sets fsGroup it must agree with runAsUser, or the
			// service cannot write the volume it was just given ownership of.
			if fm := fsGroupRe.FindSubmatch(data); fm != nil {
				fg, err := strconv.Atoi(string(fm[1]))
				if err == nil && fg != want {
					t.Errorf("%s (%s): fsGroup=%d disagrees with runAsUser=%d",
						service, rel, fg, want)
				}
			}
		}
	}
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
