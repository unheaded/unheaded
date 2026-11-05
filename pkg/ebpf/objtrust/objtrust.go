// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package objtrust decides whether an eBPF object on disk may be loaded. Every
// Go load path calls it before the object is read, whatever user the process
// runs as: a unit with CAP_BPF and no root can put a program in the kernel too,
// so write access to its object is worth as much as write access to a root
// loader's.
//
// What it guarantees: only root could have written the file or replaced any
// directory above it, at the moment of the check.
//
// What it cannot cover:
//   - Integrity. It reads owners and modes, not bytes. A bad object that root
//     installed passes; there is no hash or signature here.
//   - Root itself. Anyone already root can change the object after the check.
//   - Loaders outside Go. The Rust loaders under crates/ and cmd/ebpf-load-check
//     and the shell tools (bpftool, ip, tc) do not come through here.
//   - Pinned objects. Opening a pin in bpffs takes what is already in the
//     kernel; it has no file to check.
//   - Explicit opt-outs: --allow-untrusted-ebpf-obj-dir, -allow-untrusted-obj
//     and LoaderConfig.AllowUntrustedObjectPaths, for development trees.
package objtrust

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// ErrUntrusted is returned for an object that a non-root user could have written.
var ErrUntrusted = errors.New("eBPF object path is writable by a non-root user")

// statFunc reports a path's owner uid and mode without following a final
// symlink (the caller resolves symlinks first).
type statFunc func(path string) (uid uint32, mode fs.FileMode, err error)

// chainError applies ssh StrictModes semantics to an object about to be
// loaded: the file and every directory above it must be owned by root and not
// group- or world-writable. Write access to any of them is the ability to run
// arbitrary BPF in the kernel, so a user-owned build directory (ebpf/target on
// west) is not a trust anchor. path must already be absolute and symlink-free.
func chainError(path string, stat statFunc) error {
	for p := path; ; p = filepath.Dir(p) {
		uid, mode, err := stat(p)
		if err != nil {
			return fmt.Errorf("%w: stat %s: %v", ErrUntrusted, p, err)
		}
		if uid != 0 {
			return fmt.Errorf("%w: %s is owned by uid %d, not root", ErrUntrusted, p, uid)
		}
		if mode.Perm()&0o022 != 0 {
			return fmt.Errorf("%w: %s is group- or world-writable (%v)", ErrUntrusted, p, mode.Perm())
		}
		if parent := filepath.Dir(p); parent == p {
			return nil
		}
	}
}
