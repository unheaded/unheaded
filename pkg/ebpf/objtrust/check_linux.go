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

// Check applies chainError to path with the real owners and modes.
func Check(path string) error {
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("%w: resolve %s: %v", ErrUntrusted, path, err)
		}
		path = wd + "/" + path
	}
	return chainError(path, lstatOwner, os.Readlink)
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
