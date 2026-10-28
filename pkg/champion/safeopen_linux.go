// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package champion

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Race-free sandbox writes (M-124, ADR-019 T-05; S-CHAMPION-HARDENING
// Phase 3). validatePath checks a path string; between that check and
// the open, a directory in the path can be swapped for a symlink. Every
// write therefore resolves its path again inside the kernel, relative to
// a directory fd of the sandbox root, with openat2 and
// RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS|RESOLVE_NO_MAGICLINKS: no component
// may be a symlink and none may leave the root, at the moment of the open.

const resolveBeneath = unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS

// openRoot opens dir as an O_PATH directory fd. The root itself is the
// operator's configured sandbox (validatePath resolved it); symlinks in
// its own path are followed, as validatePath does.
func openRoot(dir string) (int, error) {
	fd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open sandbox root %s: %w", dir, err)
	}
	return fd, nil
}

func closeFD(fd int) { _ = unix.Close(fd) }

// openBeneath opens rel (relative, no "..") under rootFD; see above.
func openBeneath(rootFD int, rel string, flags int, mode uint32) (*os.File, error) {
	if flags&unix.O_CREAT == 0 {
		mode = 0 // openat2 rejects a mode without O_CREAT (EINVAL)
	}
	how := unix.OpenHow{
		Flags:   uint64(flags | unix.O_CLOEXEC | unix.O_NOFOLLOW), // #nosec G115 -- open flags are small non-negative constants
		Mode:    uint64(mode),
		Resolve: resolveBeneath,
	}
	fd, err := unix.Openat2(rootFD, rel, &how)
	if err != nil {
		return nil, beneathError(rel, err)
	}
	return os.NewFile(uintptr(fd), rel), nil
}

// mkdirAllBeneath creates every missing directory of relDir under rootFD
// one component at a time: each existing component is opened with the
// same resolve rules (a symlink refuses), each missing one is created
// with mkdirat in the directory fd just opened.
func mkdirAllBeneath(rootFD int, relDir string, mode uint32) error {
	relDir = filepath.Clean(relDir)
	if relDir == "." || relDir == "" {
		return nil
	}
	dirFD, err := unix.Dup(rootFD)
	if err != nil {
		return err
	}
	defer func() { closeFD(dirFD) }()
	for _, comp := range strings.Split(relDir, string(filepath.Separator)) {
		if comp == "" || comp == "." || comp == ".." {
			return fmt.Errorf("refusing path component %q", comp)
		}
		if err := unix.Mkdirat(dirFD, comp, mode); err != nil && !errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("create directory %s: %w", comp, err)
		}
		how := unix.OpenHow{Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW, Resolve: resolveBeneath}
		next, err := unix.Openat2(dirFD, comp, &how)
		if err != nil {
			return beneathError(comp, err)
		}
		closeFD(dirFD)
		dirFD = next
	}
	return nil
}

// beneathError names why openat2 refused, so a refusal reads as one.
func beneathError(rel string, err error) error {
	switch {
	case errors.Is(err, unix.ELOOP):
		return fmt.Errorf("%s: refusing to follow a symlink inside the sandbox: %w", rel, err)
	case errors.Is(err, unix.EXDEV):
		return fmt.Errorf("%s: resolves outside the sandbox root: %w", rel, err)
	case errors.Is(err, unix.ENOTDIR):
		// O_NOFOLLOW|O_DIRECTORY on a symlink, or a file in the path.
		return fmt.Errorf("%s: a path component is not a directory (or is a symlink): %w", rel, err)
	}
	return fmt.Errorf("%s: %w", rel, err)
}
