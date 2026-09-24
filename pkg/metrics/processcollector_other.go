// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package metrics

import "errors"

const processSupported = false

var errNoProc = errors.New("process metrics: no /proc on this platform")

func readProcess() (procSnapshot, error) { return procSnapshot{}, errNoProc }

func readProcessStartTime() (float64, error) { return 0, errNoProc }
