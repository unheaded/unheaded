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

// pendingHardening are services whose registry UID is deliberately NOT yet
// applied to their manifest, with the reason. They are privileged network
// workloads where dropping to a non-root UID is a functional change that
// cannot be validated without a live cluster — so applying it blind would be
// worse than recording it.
//
// The registry entry is kept because the UID is reserved for them. This map is
// the honest form of that: the test still asserts the manifest EXISTS and still
// fails if one is deleted, it just does not yet demand the UID. Emptying this
// map is the definition of done for the follow-up.
var pendingHardening = map[string]string{
	"suricata": "runAsNonRoot:false + NET_ADMIN/NET_RAW/SYS_NICE for AF_PACKET " +
		"capture on -i any; needs a live cluster to confirm capture still works " +
		"as UID 16782",
	"wireguard": "SYS_MODULE to insert the kernel module, and the lscr.io image's " +
		"s6 init drops to PUID/PGID itself; a kubelet-set runAsUser pre-empts that " +
		"and needs live validation",
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
			if !helmOnly[service] {
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
