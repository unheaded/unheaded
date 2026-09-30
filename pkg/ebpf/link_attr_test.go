// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"testing"
	"unsafe"
)

// The kernel reads BPF_LINK_CREATE's fields at fixed offsets (union bpf_attr,
// link_create). XDP's attribute once had target_ifindex at 16 instead of 4,
// so every XDP link failed and the loader fell back to a netlink attachment
// that stays on the interface after the process exits.
func TestLinkCreateAttrLayout(t *testing.T) {
	var a linkCreateAttr
	for _, f := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"prog_fd", unsafe.Offsetof(a.ProgFD), 0},
		{"target_ifindex", unsafe.Offsetof(a.TargetIfIdx), 4},
		{"attach_type", unsafe.Offsetof(a.AttachType), 8},
		{"flags", unsafe.Offsetof(a.Flags), 12},
		{"relative_fd", unsafe.Offsetof(a.RelativeFD), 16},
		{"relative_id", unsafe.Offsetof(a.RelativeID), 20},
		{"expected_revision", unsafe.Offsetof(a.ExpectedRev), 24},
	} {
		if f.got != f.want {
			t.Errorf("%s at offset %d, kernel reads it at %d", f.name, f.got, f.want)
		}
	}
}
