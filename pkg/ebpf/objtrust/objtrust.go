// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package objtrust decides whether an eBPF object on disk may be loaded. Every
// Go load path calls it before the object is read, whatever user the process
// runs as: a unit with CAP_BPF and no root can put a program in the kernel too,
// so write access to its object is worth as much as write access to a root
// loader's.
//
// What it guarantees: only root could have written the file, replaced any
// directory above it or repointed any symlink on the way to it, at the moment
// of the check. So the path a caller opens afterwards still leads there.
//
// What it cannot cover:
//   - Integrity. It reads owners and modes, not bytes. A bad object that root
//     installed passes; there is no hash or signature here.
//   - Root itself. Anyone already root can change the object after the check.
//   - Loaders outside Go. The Rust loaders ask crates/objtrust, this package's
//     twin, whose opt-out is --allow-untrusted-ebpf-obj-dir. The shell tools
//     (bpftool, ip, tc) ask neither.
//   - Pinned objects. Opening a pin in bpffs takes what is already in the
//     kernel; it has no file to check.
//   - Explicit opt-outs, for development trees: --allow-untrusted-ebpf-obj-dir,
//     -allow-untrusted-obj, --allow-untrusted-bpf-dir, YALD_ALLOW_UNTRUSTED_OBJ
//     and LoaderConfig.AllowUntrustedObjectPaths.
package objtrust

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// ErrUntrusted is returned for an object that a non-root user could have written.
var ErrUntrusted = errors.New("eBPF object path is writable by a non-root user")

// statFunc reports a path's owner uid and mode without following a final
// symlink.
type statFunc func(path string) (uid uint32, mode fs.FileMode, err error)

// readlinkFunc reports a symlink's target as written.
type readlinkFunc func(path string) (string, error)

// maxLinks is how many symlinks one path may pass through, as in the kernel.
const maxLinks = 40

// chainError applies ssh StrictModes semantics to an object about to be
// loaded: the file, every directory above it and every symlink met on the way
// must be owned by root, and the file and directories must not be group- or
// world-writable. Write access to any of them is the ability to run arbitrary
// BPF in the kernel, so a user-owned build directory (ebpf/target on west) is
// not a trust anchor. It walks path one component at a time, as the kernel
// does, following links as it meets them, so a link's own directory is checked
// and not only where the link ends up. path must be absolute.
func chainError(path string, stat statFunc, readlink readlinkFunc) error {
	if _, err := trustedEntry("/", stat); err != nil {
		return err
	}
	cur, links := "/", 0
	pending := strings.Split(path, "/")
	for len(pending) > 0 {
		name := pending[0]
		pending = pending[1:]
		switch name {
		case "", ".":
			continue
		case "..":
			cur = filepath.Dir(cur)
			continue
		}
		next := filepath.Join(cur, name)
		mode, err := trustedEntry(next, stat)
		if err != nil {
			return err
		}
		if mode&fs.ModeSymlink == 0 {
			cur = next
			continue
		}
		if links++; links > maxLinks {
			return fmt.Errorf("%w: %s passes through more than %d symlinks", ErrUntrusted, path, maxLinks)
		}
		target, err := readlink(next)
		if err != nil {
			return fmt.Errorf("%w: readlink %s: %v", ErrUntrusted, next, err)
		}
		if filepath.IsAbs(target) {
			cur = "/"
		}
		pending = append(strings.Split(target, "/"), pending...)
	}
	return nil
}

// trustedEntry checks one file, directory or symlink and returns its mode. A
// symlink's mode means nothing: only its owner and its directory decide who can
// repoint it.
func trustedEntry(p string, stat statFunc) (fs.FileMode, error) {
	uid, mode, err := stat(p)
	if err != nil {
		return 0, fmt.Errorf("%w: stat %s: %v", ErrUntrusted, p, err)
	}
	if uid != 0 {
		return 0, fmt.Errorf("%w: %s is owned by uid %d, not root", ErrUntrusted, p, uid)
	}
	if mode&fs.ModeSymlink == 0 && mode.Perm()&0o022 != 0 {
		return 0, fmt.Errorf("%w: %s is group- or world-writable (%v)", ErrUntrusted, p, mode.Perm())
	}
	return mode, nil
}
