// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// UH-AUTHN-01's evidence fails on today's compose file; this proves it can
// pass, and that one unauthenticated first-party service fails it.
func TestServiceAuthCheck(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	script, _ := filepath.Abs("../../../scripts/compliance/check-service-auth.sh")
	dir := t.TempDir()
	run := func(body string) error {
		p := filepath.Join(dir, "compose.yml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return exec.Command("bash", script, p).Run()
	}
	good := "services:\n  api:\n    build: .\n    environment:\n      AUTH_ENABLED: \"true\"\n" +
		"  web:\n    build: .\n    environment: [\"AUTH_ENABLED=true\"]\n  postgres:\n    image: postgres\n"
	if err := run(good); err != nil {
		t.Fatalf("all first-party services authenticated, rejected: %v", err)
	}
	if err := run(good + "  kanban:\n    build: .\n"); err == nil {
		t.Error("an unauthenticated first-party service was accepted")
	}
	if err := run("services:\n  postgres:\n    image: postgres\n"); err == nil {
		t.Error("no first-party services passed vacuously")
	}
}
