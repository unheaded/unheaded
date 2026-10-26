// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import "testing"

// FuzzParseAllowlist: the blast-radius allowlist is operator-written YAML
// read at start (D-17). Any input must parse or be refused; never panic.
func FuzzParseAllowlist(f *testing.F) {
	f.Add([]byte("lxd:\n  project: unheaded\n  label: chaos=allowed\nnetns: '^steel[0-9]+$'\npacket_interfaces: '^steel[0-9]+-h$'\nsystemd:\n  host: east\n  units:\n    - unheaded-timeguru.service\nlimits:\n  max_concurrent: 1\n  max_duration: 10m\n"))
	f.Add([]byte("netns: '('\n"))
	f.Add([]byte("systemd:\n  units: [ssh.service, '*.target']\n"))
	f.Add([]byte(""))
	f.Add([]byte("limits:\n  max_duration: -1h\n  max_loss_percent: 1e308\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		a, err := ParseAllowlist(b)
		if err == nil && a == nil {
			t.Fatal("nil allowlist without error")
		}
	})
}
