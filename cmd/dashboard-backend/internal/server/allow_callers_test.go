// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const allowListRepoRoot = "../../../.."

// dashboardMintsServiceTokens reports whether non-test dashboard-backend
// source calls another service under its own identity.
func dashboardMintsServiceTokens(t *testing.T) bool {
	t.Helper()
	minting := regexp.MustCompile(`AuthorizeOutgoing\(|ServiceTokenMinter|AuthorizationHeader\(`)
	found := false
	err := filepath.WalkDir(filepath.Join(allowListRepoRoot, "cmd", "dashboard-backend"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path) // #nosec G304 -- repo source under test
		if err != nil {
			return err
		}
		found = found || minting.Match(src)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// M13: the one browser-facing service is allowlisted only where it really
// calls. Wotan is the exception: it reaches Wotan as a client, under its
// own identity, and services/wotan/cmd/wotan/allowlist_test.go checks that.
func TestDashboardBackendIsAllowlistedOnlyWhereItCalls(t *testing.T) {
	if dashboardMintsServiceTokens(t) {
		t.Fatal("dashboard-backend mints service tokens now: list its real targets here")
	}
	const caller = "dashboard-backend"
	checked := 0

	units, err := filepath.Glob(filepath.Join(allowListRepoRoot, "deploy", "systemd", "*.service"))
	if err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`(?m)^Environment=AUTH_ALLOW_CALLERS=(.*)$`)
	for _, unit := range units {
		raw, err := os.ReadFile(unit) // #nosec G304 -- repo file under test
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(unit) == "unheaded-wotan.service" {
			continue
		}
		for _, m := range line.FindAllSubmatch(raw, -1) {
			checked++
			if slices.Contains(strings.Split(string(m[1]), ","), caller) {
				t.Errorf("%s allows %s, which never calls it", filepath.Base(unit), caller)
			}
		}
	}

	raw, err := os.ReadFile(filepath.Join(allowListRepoRoot, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Environment yaml.Node `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for name, svc := range doc.Services {
		if name == "wotan" {
			continue
		}
		var env map[string]string
		if svc.Environment.Decode(&env) != nil {
			continue // a list-form environment carries no allowlist here
		}
		list, ok := env["AUTH_ALLOW_CALLERS"]
		if !ok {
			continue
		}
		checked++
		if slices.Contains(strings.Split(list, ","), caller) {
			t.Errorf("docker-compose.yml %s allows %s, which never calls it", name, caller)
		}
	}
	if checked < 10 {
		t.Fatalf("checked %d allowlists: the patterns no longer fit the files, so this test checks nothing", checked)
	}
}
