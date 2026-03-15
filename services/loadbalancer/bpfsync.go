// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"io"

	"github.com/cilium/ebpf"
)

// bpfMap is a minimal interface covering the ebpf.Map methods used by SyncToBPF.
type bpfMap interface {
	io.Closer
	Update(key, value interface{}, flags ebpf.MapUpdateFlags) error
}

// syncOpenPinnedMap opens a pinned BPF map at the given path.
// Returns (nil, error) when the map does not exist or cannot be opened.
func syncOpenPinnedMap(path string) (bpfMap, error) {
	return ebpf.LoadPinnedMap(path, nil)
}
