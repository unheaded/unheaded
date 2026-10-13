// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o640); err != nil {
		t.Fatal(err)
	}
}

func TestLogDirsSizesEachService(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "wotan", "wotan.log"), 100)
	write(t, filepath.Join(root, "wotan", "wotan.log.1.gz"), 50)
	write(t, filepath.Join(root, "doom", "runs", "deep", "x.log"), 7)
	write(t, filepath.Join(root, "audit.log"), 3)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o750); err != nil {
		t.Fatal(err)
	}
	// A link out of the tree, both as a directory and as a file: never followed.
	outside := t.TempDir()
	write(t, filepath.Join(outside, "big"), 1<<20)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "big"), filepath.Join(root, "wotan", "big.log")); err != nil {
		t.Fatal(err)
	}

	dirs, ok := logDirs(root, logWalkBudget)
	if !ok {
		t.Fatal("root not read")
	}
	got := map[string]uint64{}
	for _, d := range dirs {
		got[d.Dir] = d.Bytes
		if d.Truncated {
			t.Errorf("%s truncated under the budget", d.Dir)
		}
	}
	want := map[string]uint64{".": 3, "doom": 7, "empty": 0, "wotan": 150}
	if len(got) != len(want) {
		t.Fatalf("dirs = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
}

func TestLogDirsBudgetMarksTruncated(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		write(t, filepath.Join(root, "spam", strings.Repeat("a", i+1)+".log"), 10)
	}
	dirs, _ := logDirs(root, 3)
	for _, d := range dirs {
		if d.Dir != "spam" {
			continue
		}
		if !d.Truncated || d.Bytes != 30 {
			t.Fatalf("spam = %+v, want 30 bytes and truncated", d)
		}
		return
	}
	t.Fatal("spam missing")
}

func TestLogDirsMissingRootIsNotZero(t *testing.T) {
	s := collectLogs(t.TempDir(), filepath.Join(t.TempDir(), "absent"), logWalkBudget)
	if s.RootOK || len(s.Dirs) != 0 {
		t.Fatalf("absent root read as %+v", s)
	}
	out := promTextLogs("west", s)
	if !strings.Contains(out, `huginn_log_root_ok{host="west"} 0`) {
		t.Errorf("missing root_ok 0:\n%s", out)
	}
	if strings.Contains(out, "huginn_log_dir_bytes") {
		t.Errorf("absent root reported directory sizes:\n%s", out)
	}
	if !s.FSOK || !strings.Contains(out, "huginn_log_fs_avail_bytes{host=\"west\",path=") {
		t.Errorf("filesystem of an existing path not reported:\n%s", out)
	}
}

func TestPromTextLogs(t *testing.T) {
	out := promTextLogs("east", LogSnapshot{
		FSPath: "/var/log", FSOK: true, FSSize: 1000, FSAvail: 250, RootOK: true,
		Dirs: []LogDir{{Dir: "wotan", Bytes: 42}, {Dir: "doom", Bytes: 9, Truncated: true}},
	})
	for _, line := range []string{
		`huginn_log_fs_size_bytes{host="east",path="/var/log"} 1000`,
		`huginn_log_fs_avail_bytes{host="east",path="/var/log"} 250`,
		`huginn_log_root_ok{host="east"} 1`,
		`huginn_log_dir_bytes{host="east",dir="wotan"} 42`,
		`huginn_log_dir_truncated{host="east",dir="wotan"} 0`,
		`huginn_log_dir_truncated{host="east",dir="doom"} 1`,
	} {
		if !strings.Contains(out, line+"\n") {
			t.Errorf("missing %q in:\n%s", line, out)
		}
	}
}
