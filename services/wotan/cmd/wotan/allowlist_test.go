// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/auth"
	"unheaded/services/wotan/internal/api"
)

const repoRoot = "../../../.."

// Every name a deployed service subscribes under must be on the shipped
// allowlist with auth off, or its subscription sits in pending_approval and
// never sees a message: architect ("architect-service") and micromanager
// ("micromanager-service") never received alerts.critical (2026-10-20).
// With auth off the display name is matched against auto_approve and
// legacy_display_names (the Appendix A3 rollback).
func TestShippedAllowlistCoversDeployedSubscribers(t *testing.T) {
	cfg, err := api.LoadTopicConfig(filepath.Join(repoRoot, "configs", "wotan.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	subscribe := regexp.MustCompile(`\.Subscribe(?:Topic)?\(ctx, "[^"]+", "([^"]+)"\)`)
	deployed := []string{
		"services/architect", "services/captain", "services/micromanager", "services/timeguru",
		"services/monad", "services/sophia", "cmd/dashboard-backend", "cmd/kanban-app",
		"cmd/unheaded-daemon", "cmd/trace-collector-go", "cmd/akira", "cmd/kev-poller",
	}
	found := 0
	for _, dir := range deployed {
		err := filepath.WalkDir(filepath.Join(repoRoot, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path) // #nosec G304 -- repo source under test
			if err != nil {
				return err
			}
			for _, m := range subscribe.FindAllStringSubmatch(string(src), -1) {
				found++
				if !cfg.IsAutoApproved(nil, m[1]) {
					t.Errorf("%s subscribes as %q, which configs/wotan.yaml does not auto-approve with auth off", path, m[1])
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
	}
	if found == 0 {
		t.Fatal("matched no Subscribe calls: the pattern no longer fits the code, so this test checks nothing")
	}
}

// wotanClientIdentities are the identities binaries present to Wotan:
// every wotanClient.InitServiceIdentity("<name>") under cmd/ and services/.
func wotanClientIdentities(t *testing.T) []string {
	t.Helper()
	re := regexp.MustCompile(`InitServiceIdentity\("([^"]+)"\)`)
	var names []string
	for _, top := range []string{"cmd", "services"} {
		err := filepath.WalkDir(filepath.Join(repoRoot, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path) // #nosec G304 -- repo source under test
			if err != nil {
				return err
			}
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				if !slices.Contains(names, m[1]) {
					names = append(names, m[1])
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(names) < 15 {
		t.Fatalf("found %d InitServiceIdentity calls (%v): the pattern no longer fits the code", len(names), names)
	}
	return append(names, rustWotanClientIdentities(t)...)
}

// rustWotanClientIdentities are the identities the Rust Wotan clients
// present: `pub const IDENTITY: &str = "<name>";` in a cmd/ source file that
// mints with crates/svc-token (cmd/trace-collector, cmd/ebpf-collector).
func rustWotanClientIdentities(t *testing.T) []string {
	t.Helper()
	re := regexp.MustCompile(`pub const IDENTITY: &str = "([^"]+)";`)
	var names []string
	err := filepath.WalkDir(filepath.Join(repoRoot, "cmd"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "target" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".rs") {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304 -- repo source under test
		if err != nil {
			return err
		}
		if !strings.Contains(string(src), "svc_token") {
			return nil
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if !slices.Contains(names, m[1]) {
				names = append(names, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"trace-collector", "ebpf-collector"} {
		if !slices.Contains(names, want) {
			t.Fatalf("Rust Wotan identities %v lack %q: the pattern no longer fits the code", names, want)
		}
	}
	return names
}

// pending: identities that call Wotan but were never auto-approved (their
// subscriptions waited for an admin before Phase 7, and still do).
var pending = []string{"gateway", "shield"}

// With auth on, a caller gets past Wotan only if its identity is in
// AUTH_ALLOW_CALLERS, and is approved at once only if it is in auto_approve.
// Every binary that presents an identity must be on both lists, wherever
// Wotan is deployed (compose and the systemd unit), or its publishes are
// refused the day auth goes on.
func TestEveryWotanClientIdentityIsAllowedAndApproved(t *testing.T) {
	cfg, err := api.LoadTopicConfig(filepath.Join(repoRoot, "configs", "wotan.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	compose := composeWotanEnv(t)
	if compose["AUTH_ENABLED"] != "true" {
		t.Errorf("docker-compose.yml wotan: AUTH_ENABLED = %q, want \"true\"", compose["AUTH_ENABLED"])
	}
	unit := unitEnv(t, filepath.Join(repoRoot, "deploy", "systemd", "unheaded-wotan.service"))
	if unit["AUTH_ENABLED"] != "true" {
		t.Errorf("unheaded-wotan.service: AUTH_ENABLED = %q, want \"true\"", unit["AUTH_ENABLED"])
	}
	composeAllow := strings.Split(compose["AUTH_ALLOW_CALLERS"], ",")
	unitAllow := strings.Split(unit["AUTH_ALLOW_CALLERS"], ",")
	for _, id := range wotanClientIdentities(t) {
		if !slices.Contains(composeAllow, id) {
			t.Errorf("docker-compose.yml wotan AUTH_ALLOW_CALLERS lacks %q", id)
		}
		if !slices.Contains(unitAllow, id) {
			t.Errorf("unheaded-wotan.service AUTH_ALLOW_CALLERS lacks %q", id)
		}
		verified := &auth.Identity{Subject: id, Roles: []string{auth.RoleService}, Extra: map[string]string{"auth_method": "eddsa"}}
		if !slices.Contains(pending, id) && !cfg.IsAutoApproved(verified, "") {
			t.Errorf("configs/wotan.yaml auto_approve lacks the identity %q", id)
		}
	}
	if slices.Contains(cfg.Topics.AutoApprove, "service") {
		t.Error(`auto_approve still has the catch-all "service": every caller has a name now`)
	}
}

func composeWotanEnv(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Services["wotan"].Environment
}

// unitEnv reads Environment=K=V lines (one assignment each).
func unitEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path) // #nosec G304 -- repo file under test
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		kv, ok := strings.CutPrefix(line, "Environment=")
		if !ok {
			continue
		}
		k, v, _ := strings.Cut(strings.Trim(kv, `"`), "=")
		env[k] = v
	}
	return env
}
