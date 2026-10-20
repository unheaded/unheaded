// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"unheaded/services/wotan/internal/api"
)

// Every name a deployed service subscribes under must be on the shipped
// allowlist, or its subscription sits in pending_approval and never sees a
// message: architect ("architect-service") and micromanager
// ("micromanager-service") never received alerts.critical (2026-10-20).
func TestShippedAllowlistCoversDeployedSubscribers(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	cfg, err := api.LoadTopicConfig(filepath.Join(root, "configs", "wotan.yaml"))
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
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path) // #nosec G304 -- repo source under test
			if err != nil {
				return err
			}
			for _, m := range subscribe.FindAllStringSubmatch(string(src), -1) {
				found++
				if !cfg.IsAutoApproved(m[1]) {
					t.Errorf("%s subscribes as %q, which configs/wotan.yaml does not auto-approve", path, m[1])
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
