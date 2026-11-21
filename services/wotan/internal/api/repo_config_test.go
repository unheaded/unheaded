// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The checked-in configs/wotan.yaml is what west runs (compose mounts it).
// It must load, and every identity it approves must have a permissions
// entry (FND-043): an approved identity the section does not name is
// refused everything under unlisted: deny, which is a broken service, not a
// bounded one. Library-only names with no binary are the exceptions.
func TestRepoWotanConfig_LoadsAndNamesEveryApprovedIdentity(t *testing.T) {
	cfg, err := LoadTopicConfig(filepath.Join("..", "..", "..", "..", "configs", "wotan.yaml"))
	if err != nil {
		t.Fatalf("configs/wotan.yaml: %v", err)
	}
	if !cfg.PermissionsConfigured() {
		t.Fatal("configs/wotan.yaml has no topics.permissions section")
	}
	noBinary := map[string]bool{"loadbalancer": true}
	for _, name := range cfg.Topics.AutoApprove {
		id := strings.ToLower(strings.TrimSpace(name))
		if id == "*" || noBinary[id] {
			continue
		}
		if _, ok := cfg.Topics.Permissions.Services[id]; !ok {
			t.Errorf("auto_approve names %q but topics.permissions.services does not", id)
		}
	}
	for _, name := range []string{"timeguru", "kanban-app", "dashboard-backend", "akira"} {
		if ok, reason := cfg.Permit(serviceIdentity(name), PermitSubscribe, "system.health.reports"); !ok {
			t.Errorf("%s cannot read system.health.reports: %s", name, reason)
		}
	}
}

// A host's copy of the topic config (scripts/wotan-host-config.py: the
// repository's file plus that host's enrolled public keys) is checked with
// Wotan's own parser before Wotan is restarted on it: a bad key is fatal at
// start. Skipped unless WOTAN_HOST_CONFIG names the file:
//
//	WOTAN_HOST_CONFIG=/etc/unheaded/wotan/wotan.yaml go test ./services/wotan/internal/api -run TestHostWotanConfig -v
//
// It must load, and outside the keys it must say what the repository's file
// says: the same approved identities, permissions and signed prefixes.
func TestHostWotanConfig(t *testing.T) {
	path := os.Getenv("WOTAN_HOST_CONFIG")
	if path == "" {
		t.Skip("WOTAN_HOST_CONFIG is not set")
	}
	host, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	repo, err := LoadTopicConfig(filepath.Join("..", "..", "..", "..", "configs", "wotan.yaml"))
	if err != nil {
		t.Fatalf("configs/wotan.yaml: %v", err)
	}
	if got, want := strings.Join(host.Topics.AutoApprove, ","), strings.Join(repo.Topics.AutoApprove, ","); got != want {
		t.Errorf("auto_approve differs from the repository's file: write the copy again (scripts/wotan-host-config.py)")
	}
	if host.PermissionsConfigured() != repo.PermissionsConfigured() {
		t.Errorf("topics.permissions is configured in one file and not the other")
	}
	if len(host.Topics.SignedPrefixes) != len(repo.Topics.SignedPrefixes) {
		t.Fatalf("%d signed prefixes, the repository's file has %d", len(host.Topics.SignedPrefixes), len(repo.Topics.SignedPrefixes))
	}
	enrolled := 0
	for i, sp := range host.Topics.SignedPrefixes {
		if sp.Prefix != repo.Topics.SignedPrefixes[i].Prefix {
			t.Errorf("signed prefix %d is %q, the repository's file has %q", i, sp.Prefix, repo.Topics.SignedPrefixes[i].Prefix)
		}
		enrolled += len(sp.Keys)
		t.Logf("%-26s %d key(s)", sp.Prefix, len(sp.Keys))
	}
	if enrolled == 0 {
		t.Errorf("%s enrols no key: it is the repository's file, not a host's copy", path)
	}
}
