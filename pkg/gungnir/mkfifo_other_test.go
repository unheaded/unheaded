// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !unix

package gungnir

import "errors"

func mkfifo(string, uint32) error { return errors.New("no FIFOs here") }
