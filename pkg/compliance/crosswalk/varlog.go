// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import "strings"

// The names CIS 6.1.4.1 lets be looser than 0640 under /var/log. The
// var-log-perms probe judges by them, the var-log-modes fix tightens to
// them, and the Ansible rendering repeats them: one list each.
var (
	// VarLogLoginRecords may be group-writable and world-readable (0664),
	// with their rotated copies (name.N, name-DATE).
	VarLogLoginRecords = []string{"lastlog", "wtmp", "btmp"}
	// VarLogWorldReadable are agents' logs that may be world-readable
	// (0644); the names are prefixes, so rotated copies are covered.
	VarLogWorldReadable = []string{"cloud-init.log", "localmessages", "waagent.log"}
	// VarLogGroupWritableDirs hold logs that may be group-writable (0660).
	VarLogGroupWritableDirs = []string{"sssd", "SSSD", "gdm", "gdm3"}
)

// VarLogMaxMode is CIS 6.1.4.1's ceiling for one file under /var/log, by
// its name: 0664 for login records and README, 0644 for a few agents' logs,
// 0660 in the sssd and gdm directories, 0640 for everything else.
func VarLogMaxMode(path string) uint32 {
	base := path[strings.LastIndex(path, "/")+1:]
	dir := path[:strings.LastIndex(path, "/")+1]
	for _, p := range VarLogLoginRecords {
		if base == p || strings.HasPrefix(base, p+".") || strings.HasPrefix(base, p+"-") {
			return 0o664
		}
	}
	if base == "README" {
		return 0o664
	}
	for _, p := range VarLogWorldReadable {
		if strings.HasPrefix(base, p) {
			return 0o644
		}
	}
	for _, d := range VarLogGroupWritableDirs {
		if strings.Contains(dir, "/"+d+"/") {
			return 0o660
		}
	}
	return 0o640
}
