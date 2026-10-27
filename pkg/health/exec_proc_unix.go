// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

//go:build unix

package health

import (
	"os/exec"
	"syscall"
)

// killProcessGroup puts the check in its own process group and makes the
// timeout kill the whole group, so a script's children (openssl under a
// runbook step) die with it instead of holding the output pipe open.
func killProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
