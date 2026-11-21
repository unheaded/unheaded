// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package ebpf

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// ErrPinUntrusted is returned for a pin, or a pin directory, that a user other
// than root or the caller owns, that a group or everyone can write, or that is
// a symlink.
var ErrPinUntrusted = errors.New("ebpf: pinned map path is not trusted")

// ErrPinName is returned when the kernel's name for a pinned map is not the
// one the caller expects.
var ErrPinName = errors.New("ebpf: pinned map name mismatch")

// objNameLen is BPF_OBJ_NAME_LEN less its NUL: the kernel keeps this much of
// a map's name.
const objNameLen = 15

// OpenPinnedMapNamed opens the map pinned at path for a caller that is about
// to write to it, and holds the pin to two things OpenPinnedMap does not:
//
//   - The pin and its directory are real (no symlink), owned by root or by
//     the calling user, and not group- or world-writable: nobody else could
//     have put the pin there.
//   - The kernel's own name for the map behind the pin is name. A stale or
//     wrong pin with the right sizes would otherwise take the write, and
//     nothing would read it.
//
// It checks the last two path components only; the directories above them are
// the operator's. The caller must Close the map.
func OpenPinnedMapNamed(path, name string) (*PinnedMap, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: no expected name for %s", ErrPinName, path)
	}
	if err := pinTrustError(path, os.Geteuid()); err != nil {
		return nil, err
	}
	fd, err := bpfObjGet(path)
	if err != nil {
		return nil, err
	}
	info, err := bpfGetMapInfo(fd)
	if err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("map info %s: %w", path, err)
	}
	if !pinNameMatches(info.Name, name) {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("%w: the map pinned at %s is %q in the kernel, want %q: a stale or wrong pin",
			ErrPinName, path, kernelMapName(info.Name), name)
	}
	if !mapTypeToMapType(info.Type).elemFitsOneBuffer() {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("%w: %s has type %d", ErrUnsupportedMapType, path, info.Type)
	}
	return &PinnedMap{fd: fd, keySize: info.KeySize, valueSize: info.ValueSize}, nil
}

// kernelMapName is the name in a bpf_map_info, up to its NUL.
func kernelMapName(raw [16]byte) string {
	name, _, _ := bytes.Cut(raw[:], []byte{0})
	return string(name)
}

// pinNameMatches compares the kernel's name with want, both cut to the
// objNameLen bytes the kernel keeps. A map with no name matches nothing.
func pinNameMatches(raw [16]byte, want string) bool {
	if len(want) > objNameLen {
		want = want[:objNameLen]
	}
	got := kernelMapName(raw)
	return got != "" && got == want
}

// pinTrustError checks the pin at path and the directory holding it for a
// caller running as euid.
func pinTrustError(path string, euid int) error {
	dir := filepath.Dir(path)
	if err := pinEntryTrusted(dir, euid, true); err != nil {
		return err
	}
	return pinEntryTrusted(path, euid, false)
}

func pinEntryTrusted(p string, euid int, wantDir bool) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is a symlink; name the pin by its real path", ErrPinUntrusted, p)
	}
	if fi.IsDir() != wantDir {
		if wantDir {
			return fmt.Errorf("%w: %s is not a directory", ErrPinUntrusted, p)
		}
		return fmt.Errorf("%w: %s is a directory, not a pin", ErrPinUntrusted, p)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: no owner information for %s", ErrPinUntrusted, p)
	}
	if st.Uid != 0 && int64(st.Uid) != int64(euid) {
		return fmt.Errorf("%w: %s is owned by uid %d, want root or the calling user (uid %d)", ErrPinUntrusted, p, st.Uid, euid)
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return fmt.Errorf("%w: %s is group- or world-writable (%04o)", ErrPinUntrusted, p, perm)
	}
	return nil
}
