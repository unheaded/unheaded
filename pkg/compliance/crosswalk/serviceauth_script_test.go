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
	// pkg/auth enables authentication for the exact string "true" only.
	// These once passed: the check lowercased, and YAML 1.1 read yes/on/True
	// as a boolean that printed as "True".
	if err := run("services:\n  api:\n    build: .\n    environment:\n      AUTH_ENABLED: true\n"); err != nil {
		t.Errorf("unquoted true (compose passes \"true\") rejected: %v", err)
	}
	for _, v := range []string{`"TRUE"`, `"True"`, `yes`, `on`, `"1"`} {
		if err := run("services:\n  api:\n    build: .\n    environment:\n      AUTH_ENABLED: " + v + "\n"); err == nil {
			t.Errorf("AUTH_ENABLED: %s accepted; the service would run unauthenticated", v)
		}
	}
	if err := run("services:\n  api:\n    build: .\n    environment: [\"AUTH_ENABLED=True\"]\n"); err == nil {
		t.Error("AUTH_ENABLED=True accepted; the service would run unauthenticated")
	}
}
