// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build !unix

package champion

import "os/exec"

// killProcessGroupOnCancel: no process groups here; the context kills the
// script only.
func killProcessGroupOnCancel(*exec.Cmd) {}
