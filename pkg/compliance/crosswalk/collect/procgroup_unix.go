// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build unix

package collect

import (
	"os/exec"
	"syscall"
	"time"
)

// ownGroup runs cmd in its own process group and, on cancel, kills the
// whole group: killing only bash left its children (cargo, sleep) holding
// the output pipes open, so Wait blocked until they finished.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
}
