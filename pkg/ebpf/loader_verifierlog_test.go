// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

// A program the verifier accepts can still come back ENOSPC: the kernel
// reports a log that outgrew its buffer that way. The load must grow the
// buffer and try again, not call it a verifier rejection.
func TestLoadWithLogGrowth_RetriesWhenTheLogOverflows(t *testing.T) {
	var sizes []int
	fd, vlog, err := loadWithLogGrowth(1024, func(log []byte) (int, string, error) {
		sizes = append(sizes, len(log))
		if len(log) < 4096 {
			return -1, "cut short", fmt.Errorf("%w: prog_load: %w", ErrSyscallFailed, unix.ENOSPC)
		}
		return 7, "whole log", nil
	})
	if err != nil || fd != 7 || vlog != "whole log" {
		t.Fatalf("fd %d log %q err %v", fd, vlog, err)
	}
	if len(sizes) != 3 || sizes[0] != 1024 || sizes[1] != 2048 || sizes[2] != 4096 {
		t.Fatalf("buffer sizes %v", sizes)
	}
}

func TestLoadWithLogGrowth_StopsAtTheCap(t *testing.T) {
	var last, calls int
	_, _, err := loadWithLogGrowth(maxVerifierLogSize/2, func(log []byte) (int, string, error) {
		calls++
		last = len(log)
		return -1, "", unix.ENOSPC
	})
	if !errors.Is(err, unix.ENOSPC) {
		t.Fatalf("err %v", err)
	}
	if calls != 2 || last != maxVerifierLogSize {
		t.Fatalf("calls %d, last size %d", calls, last)
	}
}

func TestLoadWithLogGrowth_OtherErrorsAreNotRetried(t *testing.T) {
	calls := 0
	_, vlog, err := loadWithLogGrowth(1024, func([]byte) (int, string, error) {
		calls++
		return -1, "rejected here", unix.EACCES
	})
	if calls != 1 || vlog != "rejected here" || !errors.Is(err, unix.EACCES) {
		t.Fatalf("calls %d log %q err %v", calls, vlog, err)
	}
}

// With no log asked for, the kernel's ENOSPC cannot mean an overflow.
func TestLoadWithLogGrowth_NoLogNoRetry(t *testing.T) {
	calls := 0
	_, _, err := loadWithLogGrowth(0, func(log []byte) (int, string, error) {
		calls++
		if len(log) != 0 {
			t.Fatalf("log of %d bytes", len(log))
		}
		return -1, "", unix.ENOSPC
	})
	if calls != 1 || !errors.Is(err, unix.ENOSPC) {
		t.Fatalf("calls %d err %v", calls, err)
	}
}

// The retry reads the errno through bpfProgLoad's wrapping, so it must survive.
func TestProgLoadError_KeepsTheErrno(t *testing.T) {
	err := progLoadError(unix.ENOSPC)
	if !errors.Is(err, unix.ENOSPC) || !errors.Is(err, ErrSyscallFailed) {
		t.Fatalf("err %v", err)
	}
}
