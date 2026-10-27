// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

//go:build !unix

package health

import "os/exec"

// killProcessGroup: no process groups here; the timeout kills the command
// itself (exec.CommandContext's default) and WaitDelay bounds the rest.
func killProcessGroup(*exec.Cmd) {}
