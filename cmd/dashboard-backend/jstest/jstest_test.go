// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package jstest runs the dashboard's JavaScript checks under node, so
// `go test ./...` covers static/dashboard.js wherever node is installed.
package jstest

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scriptFor is the page script a scenario runs: ../static/dashboard.js
// unless the scenario's header names another with a "// script: <path>"
// line (a path under ../static).
func scriptFor(t *testing.T, scenario string) string {
	t.Helper()
	f, err := os.Open(scenario)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; i < 10 && sc.Scan(); i++ {
		if rest, ok := strings.CutPrefix(sc.Text(), "// script: "); ok {
			p := filepath.Clean(strings.TrimSpace(rest))
			if !strings.HasPrefix(p, filepath.Join("..", "static")+string(filepath.Separator)) {
				t.Fatalf("%s: script %q is not under ../static", scenario, p)
			}
			return p
		}
	}
	return "../static/dashboard.js"
}

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
			out, err := exec.Command(node, sc, scriptFor(t, sc)).CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no scenario files found")
	}
}
