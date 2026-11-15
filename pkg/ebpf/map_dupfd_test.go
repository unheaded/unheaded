// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package ebpf

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// DupMapFD hands out a second descriptor for a loaded map. The caller owns it:
// closing it must not close the loader's own.
func TestDupMapFD(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stand-in-for-a-map")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	own := int(f.Fd())

	l := oneMapLoader(MapTypeLruHash)
	l.programs["p"].maps["m"].fd = own

	dup, err := l.DupMapFD("p", "m")
	if err != nil {
		t.Fatal(err)
	}
	if dup == own || dup < 0 {
		t.Fatalf("dup = %d, the loader's is %d: want a different, valid descriptor", dup, own)
	}
	var a, b unix.Stat_t
	if err := unix.Fstat(own, &a); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fstat(dup, &b); err != nil {
		t.Fatal(err)
	}
	if a.Ino != b.Ino || a.Dev != b.Dev {
		t.Fatalf("the duplicate is another file (inode %d, want %d)", b.Ino, a.Ino)
	}
	flags, err := unix.FcntlInt(uintptr(dup), unix.F_GETFD, 0)
	if err != nil {
		t.Fatal(err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("the duplicate is not close-on-exec")
	}
	if err := unix.Close(dup); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fstat(own, &a); err != nil {
		t.Fatalf("closing the duplicate closed the loader's descriptor: %v", err)
	}
}

func TestDupMapFD_Refusals(t *testing.T) {
	l := oneMapLoader(MapTypeLruHash) // its map's fd is -1: never loaded
	if _, err := l.DupMapFD("nope", "m"); !errors.Is(err, ErrProgramNotFound) {
		t.Errorf("unknown program: %v, want ErrProgramNotFound", err)
	}
	if _, err := l.DupMapFD("p", "nope"); !errors.Is(err, ErrMapNotFound) {
		t.Errorf("unknown map: %v, want ErrMapNotFound", err)
	}
	if fd, err := l.DupMapFD("p", "m"); err == nil {
		t.Errorf("a map with no descriptor gave fd %d, want an error", fd)
	}
	l.closed = true
	if _, err := l.DupMapFD("p", "m"); !errors.Is(err, ErrLoaderClosed) {
		t.Errorf("closed loader: %v, want ErrLoaderClosed", err)
	}
}
