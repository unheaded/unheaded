// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The enablement runbook is what an operator reads before setting the flag,
// so it must say what TestRunbookExecuteLiveIsHeldEvenWhenEnabled proves:
// ZHEN_MCP_ALLOW_EXEC reaches a runbook's dry run and no further.
func TestEnablementRunbookSaysExecReachesDryRunsOnly(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "runbooks", "security", "mcp-write-exec-enablement.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, "running a runbook") {
		t.Error("runbook still says the exec flag runs a runbook")
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "export ZHEN_MCP_ALLOW_EXEC=1") && !strings.Contains(line, "dry run") {
			t.Errorf("the line that sets the flag does not say dry run: %s", strings.TrimSpace(line))
		}
	}
}
