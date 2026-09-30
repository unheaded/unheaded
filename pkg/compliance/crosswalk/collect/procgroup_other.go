// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !unix

package collect

import (
	"os/exec"
	"time"
)

func ownGroup(cmd *exec.Cmd) { cmd.WaitDelay = 2 * time.Second }
