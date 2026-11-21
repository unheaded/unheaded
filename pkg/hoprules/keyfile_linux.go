// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package hoprules

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openKeyFile opens a key_file with openat2 and RESOLVE_NO_SYMLINKS, so the
// kernel refuses a symlink at the file or in any directory above it at the
// moment of the open: whoever owns such a link chooses which file is read.
// O_NONBLOCK keeps a FIFO from holding the open; readKey refuses it afterwards
// as not a regular file.
func openKeyFile(path string) (*os.File, error) {
	how := unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK,
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &how)
	switch {
	case errors.Is(err, unix.ELOOP):
		return nil, fmt.Errorf("%w: %s is a symlink or lies under one; name the key file by its real path", ErrKeyFileSymlink, path)
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ENOTDIR):
		// Nothing is at path, so it may be a pasted key and not a path: base64
		// can start with a slash, and validate only refuses a relative one.
		return nil, fmt.Errorf("key_file: %w", err)
	case err != nil:
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return os.NewFile(uintptr(fd), path), nil
}

// keyFileTrust refuses a key file some other user owns, who could then change
// the key, and one with a second hard link, which is a second way to the same
// bytes from a directory this check never saw. st is the open file's.
func keyFileTrust(st os.FileInfo, path string) error {
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: no owner information for %s", ErrKeyFileOwner, path)
	}
	if euid := geteuid(); !keyOwnerOK(sys.Uid, euid) {
		return fmt.Errorf("%w: %s is owned by uid %d, want root or the calling user (uid %d)", ErrKeyFileOwner, path, sys.Uid, euid)
	}
	if sys.Nlink != 1 {
		return fmt.Errorf("%w: %s has %d links, want 1: remove the other names", ErrKeyFileLinks, path, sys.Nlink)
	}
	return nil
}
