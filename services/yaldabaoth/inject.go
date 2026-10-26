// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// The real injectors (S-REAL-STEEL Phases 11 and 12) run external commands
// with a fixed binary and an exact argv: never a shell, never a string the
// controller interpolates into sh -c. The command runner is an interface so
// tests drive the injectors without touching the host.

// CommandRunner runs one external command and returns its stdout. The
// default (ExecRunner) uses os/exec with no shell; tests inject a fake.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs the command directly (no shell) with os/exec.
type ExecRunner struct{}

// Run executes name with args and returns combined stdout; on failure the
// error carries the trimmed stderr.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	// #nosec G204 -- name is a fixed binary chosen by the injector (tc,
	// systemctl), never from a request; args are built from allowlist-resolved
	// values and validated parameters; no shell is involved.
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}
