// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/modules"
)

// REV-1 (independent review 2026-11-15): with the switch off, a -worker run
// (the overnight user unit's, or any by hand) claims and moves no card: it
// says so in one log line and exits 0 before any pool opens.
func TestWorkerModuleGate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(modules.EnvDir, dir)

	cases := []struct {
		name     string
		wf       workerFlags
		on       bool
		wantSkip bool
	}{
		{"worker, module off", workerFlags{enabled: true}, false, true},
		{"overnight, module off", workerFlags{enabled: true, overnight: true}, false, true},
		{"once, module off", workerFlags{enabled: true, once: true}, false, true},
		{"worker, module on", workerFlags{enabled: true}, true, false},
		{"no worker, module off", workerFlags{}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			z := filepath.Join(dir, "zhen")
			_ = os.RemoveAll(z)
			if tc.on {
				if err := os.Mkdir(z, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			var buf bytes.Buffer
			got := workerModuleOff(tc.wf, &buf)
			if got != tc.wantSkip {
				t.Fatalf("workerModuleOff = %v, want %v", got, tc.wantSkip)
			}
			if !tc.wantSkip {
				if buf.Len() != 0 {
					t.Fatalf("logged %q while not skipping", buf.String())
				}
				return
			}
			var line map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
				t.Fatalf("log line not one JSON object: %q: %v", buf.String(), err)
			}
			if line["service"] != "zhen-agentd" || line["module"] != "zhen" {
				t.Fatalf("log line fields: %v", line)
			}
			if msg, _ := line["msg"].(string); !strings.Contains(msg, "module is off") {
				t.Fatalf("log msg: %q", msg)
			}
		})
	}
}

// A symlink in place of the module's directory is not "on" (pkg/modules).
func TestWorkerModuleGateSymlinkIsOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(modules.EnvDir, dir)
	real := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "zhen")); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if !workerModuleOff(workerFlags{enabled: true, overnight: true}, &buf) {
		t.Fatal("a symlinked module directory let the worker run")
	}
}
