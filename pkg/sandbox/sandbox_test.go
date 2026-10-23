// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package sandbox

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestArgs_Confinement(t *testing.T) {
	dir := t.TempDir()
	args, err := Args(Config{Dir: dir, Writable: []string{"/var/cache/x"}, Env: []string{"GOFLAGS=-mod=mod", "GOPROXY=off"}}, []string{"bash", "check.sh"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--user", "--wait", "--pipe", "PrivateNetwork=yes", "ProtectHome=tmpfs", "PrivateTmp=yes",
		"NoNewPrivileges=yes", "MemoryMax=2G", "MemorySwapMax=0", "RuntimeMaxSec=600", "TasksMax=256",
		"BindPaths=" + dir, "WorkingDirectory=" + dir, "BindPaths=/var/cache/x",
		"--setenv=GOFLAGS=-mod=mod", "--setenv=GOPROXY=off",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
	i := slices.Index(args, "--")
	if i < 0 || strings.Join(args[i+1:], " ") != "bash check.sh" {
		t.Errorf("command not after --: %v", args)
	}
}

func TestArgs_Refuses(t *testing.T) {
	dir := t.TempDir()
	for name, cfg := range map[string]Config{
		"relative dir":   {Dir: "rel"},
		"missing dir":    {Dir: "/nonexistent/sbx"},
		"bad env key":    {Dir: dir, Env: []string{"lower=x"}},
		"env no equals":  {Dir: dir, Env: []string{"FOO"}},
		"env newline":    {Dir: dir, Env: []string{"FOO=a\nb"}},
		"bad writable":   {Dir: dir, Writable: []string{"rel/path"}},
		"writable colon": {Dir: dir, Writable: []string{"/a:/b"}},
		"bad memory":     {Dir: dir, MemoryMax: "lots"},
	} {
		if _, err := Args(cfg, []string{"true"}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Args(Config{Dir: dir}, nil); err == nil {
		t.Error("empty command accepted")
	}
	if a, _ := Args(Config{Dir: dir, RuntimeMax: 3 * time.Second, TasksMax: 9, MemoryMax: "512M"}, []string{"true"}); !strings.Contains(strings.Join(a, " "), "RuntimeMaxSec=3 -p TasksMax=9") {
		t.Errorf("limits not applied: %v", a)
	}
}

func TestLimitedBuffer(t *testing.T) {
	b := &limitedBuffer{n: 5}
	_, _ = b.Write([]byte("abc"))
	_, _ = b.Write([]byte("defgh"))
	if b.buf.String() != "abcde" || !b.truncated {
		t.Errorf("%q %v", b.buf.String(), b.truncated)
	}
}
