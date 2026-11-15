// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import "testing"

// The ceiling the var-log-perms probe judges by and the var-log-modes fix
// tightens to: one function, so the check and the fix cannot disagree.
func TestVarLogMaxMode(t *testing.T) {
	for path, want := range map[string]uint32{
		"/var/log/syslog":                     0o640,
		"/var/log/auth.log.1":                 0o640,
		"/var/log/apt/history.log":            0o640,
		"/var/log/journal/abc/system.journal": 0o640,
		"/var/log/wtmp":                       0o664,
		"/var/log/wtmp.1":                     0o664,
		"/var/log/btmp-20261101":              0o664,
		"/var/log/lastlog":                    0o664,
		"/var/log/wtmpx":                      0o640, // a name that only starts like one
		"/var/log/README":                     0o664,
		"/var/log/cloud-init.log":             0o644,
		"/var/log/cloud-init.log.1":           0o644,
		"/var/log/localmessages":              0o644,
		"/var/log/waagent.log":                0o644,
		"/var/log/sssd/sssd.log":              0o660,
		"/var/log/SSSD/x":                     0o660,
		"/var/log/gdm3/greeter.log":           0o660,
		"/var/log/gdm/x.log":                  0o660,
		"/var/log/sssdx/x.log":                0o640,
	} {
		if got := VarLogMaxMode(path); got != want {
			t.Errorf("%s: ceiling %04o, want %04o", path, got, want)
		}
	}
	// The names the Ansible rendering repeats come from these lists.
	if len(VarLogLoginRecords) != 3 || len(VarLogWorldReadable) != 3 || len(VarLogGroupWritableDirs) != 4 {
		t.Errorf("the ceiling's name lists changed: %v %v %v", VarLogLoginRecords, VarLogWorldReadable, VarLogGroupWritableDirs)
	}
}
