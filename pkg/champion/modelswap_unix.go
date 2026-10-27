// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build unix

package champion

import (
	"os/exec"
	"syscall"
)

// killProcessGroupOnCancel starts cmd in a new process group and, when its
// context ends, kills the group: the script, the GPU lease wrapper and any
// llama-server they started.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
