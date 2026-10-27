// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/health"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "akira.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// S-IRON-GATE Step 288: a `type: exec` target is an exec check.
func TestLoadConfig_ExecTarget(t *testing.T) {
	t.Parallel()
	targets, err := loadConfig(writeConfig(t, `targets:
  - {name: wotan, port: 18000}
  - name: cert-expiry
    type: exec
    argv: ["/usr/bin/python3", "-B", "/opt/unheaded/scripts/run-runbook.py", "/opt/unheaded/runbooks/pki/akira-cert-expiry-check.yaml"]
    interval: 6h
    timeout: 30s
    env: {UNHEADED_TLS_DIR: /etc/unheaded/tls}
`))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(targets) != 2 || targets[0].Exec != nil {
		t.Fatalf("targets = %+v", targets)
	}
	c := targets[1].Exec
	if c == nil {
		t.Fatal("cert-expiry is not an exec target")
	}
	if c.Interval != 6*time.Hour || c.Timeout != 30*time.Second {
		t.Errorf("interval %v timeout %v", c.Interval, c.Timeout)
	}
	if len(c.Argv) != 4 || c.Argv[0] != "/usr/bin/python3" {
		t.Errorf("argv = %q", c.Argv)
	}
	if c.Env["UNHEADED_TLS_DIR"] != "/etc/unheaded/tls" {
		t.Errorf("env = %v", c.Env)
	}
}

func TestLoadConfig_ExecDefaults(t *testing.T) {
	t.Parallel()
	targets, err := loadConfig(writeConfig(t, "targets:\n  - {name: c, type: exec, argv: [/bin/true]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c := targets[0].Exec; c == nil || c.Timeout != health.DefaultExecTimeout || c.Interval != 0 {
		t.Fatalf("exec = %+v", c)
	}
}

func TestLoadConfig_RejectsBadExecTargets(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"no argv":          "targets:\n  - {name: c, type: exec}\n",
		"relative argv0":   "targets:\n  - {name: c, type: exec, argv: [python3, x.py]}\n",
		"long timeout":     "targets:\n  - {name: c, type: exec, argv: [/bin/true], timeout: 5m}\n",
		"bad timeout":      "targets:\n  - {name: c, type: exec, argv: [/bin/true], timeout: soon}\n",
		"bad interval":     "targets:\n  - {name: c, type: exec, argv: [/bin/true], interval: -1h}\n",
		"port on exec":     "targets:\n  - {name: c, type: exec, argv: [/bin/true], port: 19000}\n",
		"argv on http":     "targets:\n  - {name: c, port: 19000, argv: [/bin/true]}\n",
		"unknown type":     "targets:\n  - {name: c, type: script, argv: [/bin/true]}\n",
		"bad env name":     "targets:\n  - {name: c, type: exec, argv: [/bin/true], env: {\"A B\": x}}\n",
		"duplicate w/http": "targets:\n  - {name: c, port: 19000}\n  - {name: c, type: exec, argv: [/bin/true]}\n",
	} {
		if targets, err := loadConfig(writeConfig(t, body)); err == nil {
			t.Errorf("%s: loadConfig = %+v, nil; want an error", name, targets)
		}
	}
}

// Every shipped Akira config parses, and the host configs carry the
// cert-expiry exec check (ADR-034 Phase 7).
func TestLoadConfig_ShippedConfigsHaveCertExpiry(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"../../configs/akira-west.yaml", "../../configs/akira-east.yaml", "../../config/akira/akira.yaml"} {
		targets, err := loadConfig(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		var found bool
		for _, tg := range targets {
			if tg.Name == "cert-expiry" && tg.Exec != nil {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no cert-expiry exec target", p)
		}
	}
}

// An exec target has no unit: --remediate must not systemctl-restart it.
func TestExecTargetsHaveNoUnit(t *testing.T) {
	t.Parallel()
	targets := []health.ServiceTarget{
		{Name: "wotan", Port: 18000},
		{Name: "cert-expiry", Exec: &health.ExecCheck{Argv: []string{"/bin/true"}}},
	}
	got := execTargetNames(targets)
	if !got["cert-expiry"] || got["wotan"] {
		t.Fatalf("execTargetNames = %v", got)
	}
}
