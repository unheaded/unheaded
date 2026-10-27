// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

//go:build unix

package auth

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive flock on path (created 0600) and returns the
// release.
func lockFile(path string) (func(), error) {
	fh, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- <store>.lock beside the operator-configured store
	if err != nil {
		return nil, err
	}
	fd := int(fh.Fd()) // #nosec G115 -- a file descriptor fits in int
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		_ = fh.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = fh.Close()
	}, nil
}

func inodeOf(st os.FileInfo) uint64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Ino) // #nosec G115 -- inode numbers are non-negative
	}
	return 0
}
