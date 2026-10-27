// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

//go:build !unix

package auth

import "os"

// lockFile is a no-op where flock does not exist; writers are not
// serialised across processes there.
func lockFile(string) (func(), error) { return func() {}, nil }

func inodeOf(os.FileInfo) uint64 { return 0 }
