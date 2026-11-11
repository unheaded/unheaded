// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
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
