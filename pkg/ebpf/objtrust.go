// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// ErrUntrustedObject is returned when a root loader is asked to load an
// object that a non-root user could have written.
var ErrUntrustedObject = errors.New("eBPF object path is writable by a non-root user")

// objStat reports a path's owner uid and mode without following a final
// symlink (the caller resolves symlinks first).
type objStat func(path string) (uid uint32, mode fs.FileMode, err error)

// objectTrustError applies ssh StrictModes semantics to an object about to
// be loaded as root: the file and every directory above it must be owned by
// root and not group- or world-writable. Loading as root turns write access
// to any of them into the ability to run arbitrary BPF in the kernel, so a
// user-owned build directory (ebpf/target on west) is not a trust anchor.
// path must already be absolute and symlink-free.
func objectTrustError(path string, stat objStat) error {
	for p := path; ; p = filepath.Dir(p) {
		uid, mode, err := stat(p)
		if err != nil {
			return fmt.Errorf("%w: stat %s: %v", ErrUntrustedObject, p, err)
		}
		if uid != 0 {
			return fmt.Errorf("%w: %s is owned by uid %d, not root", ErrUntrustedObject, p, uid)
		}
		if mode.Perm()&0o022 != 0 {
			return fmt.Errorf("%w: %s is group- or world-writable (%v)", ErrUntrustedObject, p, mode.Perm())
		}
		if parent := filepath.Dir(p); parent == p {
			return nil
		}
	}
}
