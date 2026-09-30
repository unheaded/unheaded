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

// scripts/compliance/check-edge-tls.sh is UH-TLS-01's evidence. It fails on
// today's compose file; this proves it can also pass, so a FAIL means the
// config and not a broken check.
func TestEdgeTLSCheck(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	script, _ := filepath.Abs("../../../scripts/compliance/check-edge-tls.sh")
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("traefik/dynamic.yml", "tls:\n  options:\n    default:\n      minVersion: VersionTLS13\n")
	good := write("good.yml", `services:
  traefik:
    command:
      - "--entrypoints.web.address=:80"
      - "--entrypoints.web.http.redirections.entrypoint.to=websecure"
      - "--providers.file.filename=/etc/traefik/dynamic.yml"
    volumes:
      - ./traefik:/etc/traefik:ro
`)
	run := func(compose string) (string, error) {
		out, err := exec.Command("bash", script, compose).CombinedOutput()
		return string(out), err
	}
	if out, err := run(good); err != nil {
		t.Fatalf("hardened edge rejected: %v\n%s", err, out)
	}
	for name, mutate := range map[string]func(string) string{
		"no redirect": func(s string) string {
			return strings.Replace(s, "redirections.entrypoint.to=websecure", "redirections.entrypoint.to=web", 1)
		},
		"insecure api":      func(s string) string { return s + "" },
		"tls 1.2 minimum":   nil,
		"no traefik at all": func(string) string { return "services:\n  other:\n    image: x\n" },
	} {
		body, _ := os.ReadFile(good)
		s := string(body)
		switch name {
		case "insecure api":
			s = strings.Replace(s, "    command:\n", "    command:\n      - \"--api.insecure=true\"\n", 1)
		case "tls 1.2 minimum":
			write("traefik/dynamic.yml", "tls:\n  options:\n    default:\n      minVersion: VersionTLS12\n")
		default:
			s = mutate(s)
		}
		p := write("bad.yml", s)
		if out, err := run(p); err == nil {
			t.Errorf("%s: accepted\n%s", name, out)
		}
		write("traefik/dynamic.yml", "tls:\n  options:\n    default:\n      minVersion: VersionTLS13\n")
	}
}
