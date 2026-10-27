// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func directUserRefs() []map[string]any {
	return []map[string]any{{
		"topic": "user-clicked", "category": "user-action", "source_kind": "user-action",
		"source_trust": "direct-user", "source_label": "test",
	}}
}

// M-019 (c): a direct caller that sends no justification is refused; the
// daemon no longer asserts direct-user on its behalf.
func TestToolExec_DirectCallerWithoutJustificationIsRefused(t *testing.T) {
	srv, root := toolExecServer(t, nil)
	f := filepath.Join(root, "r.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, emitted := range []string{"", "direct", "zhen-web-ui", "zhen-cli"} {
		for _, call := range []map[string]any{
			{"tool": "write_file", "args": map[string]any{"path": filepath.Join(root, "w"), "content": "x"}},
			{"tool": "read_file", "args": map[string]any{"path": f}},
		} {
			call["emitted_by"] = emitted
			code, resp := postToolExec(t, srv, call)
			if code != http.StatusBadRequest || resp.Status != "denied" || !strings.Contains(resp.Reason, "justification required") {
				t.Errorf("%s by %q without justification: %d %+v", call["tool"], emitted, code, resp)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "w")); !os.IsNotExist(err) {
		t.Error("a write without justification ran")
	}
}

func TestToolExec_DirectCallerWithJustificationRuns(t *testing.T) {
	srv, root := toolExecServer(t, nil)
	code, resp := postToolExec(t, srv, map[string]any{
		"tool": "write_file", "args": map[string]any{"path": filepath.Join(root, "w"), "content": "x"},
		"emitted_by": "zhen-web-ui", "justification": directUserRefs(),
	})
	if code != http.StatusOK || resp.Status != "ok" {
		t.Errorf("explicit direct-user write: %d %+v", code, resp)
	}
}
