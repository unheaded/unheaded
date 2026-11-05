// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package objtrust

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Check resolves path and applies chainError with the real owners and modes.
func Check(path string) error {
	abs, err := filepath.Abs(path)
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return fmt.Errorf("%w: resolve %s: %v", ErrUntrusted, path, err)
	}
	return chainError(abs, lstatOwner)
}

func lstatOwner(p string) (uint32, fs.FileMode, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return 0, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("no owner information for %s", p)
	}
	return st.Uid, fi.Mode(), nil
}
