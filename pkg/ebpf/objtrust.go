// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import "unheaded/pkg/ebpf/objtrust"

// ErrUntrustedObject is returned when a loader is asked to load an object that
// a non-root user could have written.
var ErrUntrustedObject = objtrust.ErrUntrusted
