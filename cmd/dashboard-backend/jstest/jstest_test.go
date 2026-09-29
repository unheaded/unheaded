// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package jstest runs the dashboard's JavaScript checks under node, so
// `go test ./...` covers static/dashboard.js wherever node is installed.
package jstest

import (
	"os/exec"
	"testing"
)

func TestOverviewNumbers(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "overview.cjs", "../static/dashboard.js").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
