// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

//go:build unix

package health

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// groupReapLimit bounds how long endProcessGroup waits for a killed group to
// be gone.
const groupReapLimit = 2 * time.Second

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

// endProcessGroup runs after the check's own process has been waited for. A
// check leaves nothing behind: whatever is left of its group is killed, and
// the members that were reparented to this process are reaped.
//
// They are reparented here when this process is PID 1 in a container (Akira
// is) or a subreaper. os/exec waits only for its own child, so before this
// every timed-out check left its grandchildren as zombies: 23 under Akira on
// west by 2026-11-16, from 32 cert-expiry timeouts. Waiting on -pgid reaps
// only this check's group, never another check's child, and the loop ends
// once the group is gone, so the id cannot have been reused while it runs.
func endProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid := cmd.Process.Pid
	if groupGone(pgid) {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	deadline := time.Now().Add(groupReapLimit)
	for {
		for {
			var ws syscall.WaitStatus
			pid, err := syscall.Wait4(-pgid, &ws, syscall.WNOHANG, nil)
			if err != nil || pid <= 0 {
				break
			}
		}
		// Zombies still count as members, so the group is gone only once
		// every member has been reaped, here or by the host's init.
		if groupGone(pgid) || time.Now().After(deadline) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func groupGone(pgid int) bool {
	return errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH)
}
