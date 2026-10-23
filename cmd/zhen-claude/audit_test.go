// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func dryRunTools(t *testing.T, out string) []string {
	t.Helper()
	i := strings.Index(out, "{")
	if i < 0 {
		t.Fatalf("no request body in %q", out)
	}
	var m struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out[i:]), &m); err != nil {
		t.Fatalf("body: %v", err)
	}
	var names []string
	for _, tl := range m.Tools {
		names = append(names, tl.Name)
	}
	return names
}

func TestAuditWell_MissingEnvIsUsageError(t *testing.T) {
	noCredentials(t)
	t.Setenv("WELL_OPS_DSN", "")
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-dry-run", "-audit-well", "-p", "x"), strings.NewReader(""), &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "WELL_OPS_DSN") {
		t.Fatalf("exit %d, stderr %s", code, errb.String())
	}
}

func TestAuditWotan_MissingEnvIsUsageError(t *testing.T) {
	noCredentials(t)
	t.Setenv("WOTAN_ADDR", "")
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-dry-run", "-audit-wotan", "-p", "x"), strings.NewReader(""), &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "WOTAN_ADDR") {
		t.Fatalf("exit %d, stderr %s", code, errb.String())
	}
}

func TestAuditWell_WellDownMeansReadOnly(t *testing.T) {
	noCredentials(t)
	// Nothing listens on port 1: the Well is down.
	t.Setenv("WELL_OPS_DSN", "postgres://ops_writer:x@127.0.0.1:1/unheaded_ops?sslmode=disable&connect_timeout=2")
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-dry-run", "-audit-well", "-p", "x"), strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	tools := dryRunTools(t, out.String())
	for _, n := range tools {
		if n == "write_file" || n == "patch_file" || strings.HasPrefix(n, "kanban_create") || n == "kanban_update" {
			t.Errorf("read-only mode still offers %s (tools %v)", n, tools)
		}
	}
	if len(tools) == 0 {
		t.Error("read-only mode dropped the read tools too")
	}
	if !strings.Contains(errb.String(), "read_only") || !strings.Contains(errb.String(), "audit trail") {
		t.Errorf("no read-only warning naming why: %s", errb.String())
	}
	if strings.Contains(errb.String(), "ops_writer:x@") {
		t.Error("the DSN (with its password) was logged")
	}
}

func TestAuditWell_OffKeepsWriteToolsListed(t *testing.T) {
	noCredentials(t)
	var out, errb bytes.Buffer
	if code := run(append(baseArgs(t), "-dry-run", "-p", "x"), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	tools := strings.Join(dryRunTools(t, out.String()), ",")
	if !strings.Contains(tools, "write_file") {
		t.Errorf("tools = %s", tools)
	}
}
