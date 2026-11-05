// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package ebpf

import "context"

// ConsumePinnedRing returns ErrNotSupported off Linux.
func ConsumePinnedRing(context.Context, string, func([]byte)) error { return ErrNotSupported }
