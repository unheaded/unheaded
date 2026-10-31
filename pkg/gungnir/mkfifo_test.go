// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build unix

package gungnir

import "syscall"

func mkfifo(path string, mode uint32) error { return syscall.Mkfifo(path, mode) }
