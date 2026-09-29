// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package jstest runs the dashboard's JavaScript checks under node, so
// `go test ./...` covers static/dashboard.js wherever node is installed.
package jstest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDashboardScenarios(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	scenarios, _ := filepath.Glob("*.cjs")
	ran := 0
	for _, sc := range scenarios {
		if sc == "harness.cjs" {
			continue
		}
		ran++
		t.Run(sc, func(t *testing.T) {
			out, err := exec.Command(node, sc, "../static/dashboard.js").CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no scenario files found")
	}
}
