// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build !linux

package hoprules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// openKeyFile off Linux, where hop-ebpf does not exist and nothing can be
// applied (Boottime refuses): Lstat refuses a symlink at the file, and the
// open file must be the one Lstat saw. The checks on directories above the
// file are Linux's (openat2).
func openKeyFile(path string) (*os.File, error) {
	link, err := os.Lstat(path)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		return nil, fmt.Errorf("key_file: %w", err)
	}
	if link.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s is a symlink; name the key file by its real path", ErrKeyFileSymlink, path)
	}
	if !link.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := fh.Stat()
	if err != nil || !os.SameFile(link, st) {
		_ = fh.Close()
		return nil, fmt.Errorf("%s changed while being opened", path)
	}
	return fh, nil
}

// keyFileTrust has no owner or link count to read off Linux.
func keyFileTrust(os.FileInfo, string) error { return nil }
