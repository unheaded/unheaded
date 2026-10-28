// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build !linux

package champion

import (
	"errors"
	"os"
)

// Sandbox writes need openat2 (safeopen_linux.go). Elsewhere they fail
// closed so non-Linux builds compile.

func openRoot(string) (int, error) { return -1, errors.ErrUnsupported }

func closeFD(int) {}

func openBeneath(int, string, int, uint32) (*os.File, error) { return nil, errors.ErrUnsupported }

func mkdirAllBeneath(int, string, uint32) error { return errors.ErrUnsupported }
