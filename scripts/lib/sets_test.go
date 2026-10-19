// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// setComm runs set_comm from sets.sh on two sets.
func setComm(t *testing.T, flags string, a, b []string) []string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	cmd := exec.Command("bash", "-c", `source ./sets.sh && set_comm "$1" "$2" "$3"`, "x",
		flags, strings.Join(a, "\n"), strings.Join(b, "\n"))
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("set_comm %s: %v\n%s", flags, err, out)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// TestSetCommLargeSets uses sets well past the ~8 KB at which comm with two
// pipe operands goes wrong on uutils coreutils 0.2.2.
func TestSetCommLargeSets(t *testing.T) {
	var a, b []string
	for i := 0; i < 5000; i++ {
		a = append(a, fmt.Sprintf("pkg-%06d", i))
		if i != 4321 {
			b = append(b, fmt.Sprintf("pkg-%06d", i))
		}
	}
	b = append(b, "zzz-only-in-b")

	if got := setComm(t, "-23", a, b); len(got) != 1 || got[0] != "pkg-004321" {
		t.Fatalf("-23 = %v, want [pkg-004321]", got)
	}
	if got := setComm(t, "-13", a, b); len(got) != 1 || got[0] != "zzz-only-in-b" {
		t.Fatalf("-13 = %v, want [zzz-only-in-b]", got)
	}
	if got := setComm(t, "-12", a, b); len(got) != len(a)-1 {
		t.Fatalf("-12 has %d lines, want %d", len(got), len(a)-1)
	}
}

func TestSetCommEmptySides(t *testing.T) {
	if got := setComm(t, "-23", []string{"a", "b"}, nil); len(got) != 2 {
		t.Fatalf("A minus empty = %v", got)
	}
	if got := setComm(t, "-23", nil, []string{"a"}); got != nil {
		t.Fatalf("empty minus B = %v", got)
	}
}
