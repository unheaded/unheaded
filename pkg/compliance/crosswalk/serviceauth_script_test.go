// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// A service that checks a bearer token itself (logstore-writer) is not
// "open" because it lacks AUTH_ENABLED. The gate knows such services by
// name, each with the variable that carries the token, and nothing else
// gets the benefit of a token-looking variable.
func TestServiceAuthCheckTokenAuth(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	script, _ := filepath.Abs("../../../scripts/compliance/check-service-auth.sh")
	dir := t.TempDir()
	run := func(body string) (string, error) {
		p := filepath.Join(dir, "compose.yml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("bash", script, p).CombinedOutput()
		return string(out), err
	}
	const api = "services:\n  api:\n    build: .\n    environment:\n      AUTH_ENABLED: \"true\"\n"
	const writer = "  logstore-writer:\n    build: .\n    environment:\n"

	// (a) Named in the table, token variable set, no AUTH_ENABLED: passes,
	// and the output says why.
	for _, env := range []string{
		"      LOGSTORE_INGEST_TOKEN: \"${LOGSTORE_INGEST_TOKEN:?set it}\"\n",
		"      - LOGSTORE_INGEST_TOKEN=${LOGSTORE_INGEST_TOKEN}\n",
	} {
		out, err := run(api + writer + env)
		if err != nil {
			t.Errorf("token-auth service with its token variable rejected: %v\n%s", err, out)
		}
		if !strings.Contains(out, "[TOKEN] logstore-writer: ") {
			t.Errorf("no [TOKEN] line for logstore-writer:\n%s", out)
		}
	}

	// (b) The same service without its token variable fails, and so does
	// an empty value: the writer would refuse to start, and a gate that
	// passed it would have read nothing.
	for _, env := range []string{
		"      WELL_HOST: postgres\n",
		"      LOGSTORE_INGEST_TOKEN: \"\"\n",
		"      - LOGSTORE_INGEST_TOKEN\n",
	} {
		out, err := run(api + writer + env)
		if err == nil {
			t.Errorf("logstore-writer without its token variable accepted (%q):\n%s", env, out)
		}
		if !strings.Contains(out, "logstore-writer") {
			t.Errorf("the failure does not name logstore-writer:\n%s", out)
		}
	}

	// (c) A service the table does not name gains nothing from a
	// token-looking variable.
	for _, env := range []string{
		"      LOGSTORE_INGEST_TOKEN: \"x\"\n",
		"      API_TOKEN: \"x\"\n",
		"      INGEST_TOKEN: \"x\"\n",
	} {
		out, err := run(api + "  sidecar:\n    build: .\n    environment:\n" + env)
		if err == nil {
			t.Errorf("an unnamed service with a token-looking variable was accepted (%q):\n%s", env, out)
		}
	}

	// (d) Neither form.
	if out, err := run(api + "  sidecar:\n    build: .\n"); err == nil {
		t.Errorf("a service with neither AUTH_ENABLED nor a token was accepted:\n%s", out)
	}
	if out, err := run(api + "  logstore-writer:\n    build: .\n"); err == nil {
		t.Errorf("logstore-writer with no environment was accepted:\n%s", out)
	}
}
