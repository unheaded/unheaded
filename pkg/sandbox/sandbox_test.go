// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestArgs_Confinement(t *testing.T) {
	dir := t.TempDir()
	args, err := Args(Config{Dir: dir, Writable: []string{"/var/cache/x"}, ReadOnly: []string{"/opt/go"}, Env: []string{"GOFLAGS=-mod=mod", "GOPROXY=off"}}, []string{"bash", "check.sh"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--user", "--wait", "--pipe", "PrivateNetwork=yes", "ProtectHome=tmpfs", "ProtectSystem=strict", "PrivateTmp=yes",
		"NoNewPrivileges=yes", "MemoryMax=2G", "MemorySwapMax=0", "RuntimeMaxSec=600", "TasksMax=256",
		"BindPaths=" + dir, "WorkingDirectory=" + dir, "BindPaths=/var/cache/x", "BindReadOnlyPaths=/opt/go",
		"--setenv=GOFLAGS=-mod=mod", "--setenv=GOPROXY=off",
		"InaccessiblePaths=-/etc/unheaded",
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
		"bad read-only":  {Dir: dir, ReadOnly: []string{"rel"}},
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

// fakeSystemd writes a systemd-run that stays until killed and a systemctl
// that records its arguments.
func fakeSystemd(t *testing.T, ctlExit int) (run, ctl, runLog, ctlLog string) {
	t.Helper()
	base := t.TempDir()
	run, ctl = filepath.Join(base, "systemd-run"), filepath.Join(base, "systemctl")
	runLog, ctlLog = filepath.Join(base, "run.log"), filepath.Join(base, "ctl.log")
	script := func(p, body string) {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil { // #nosec G306 -- test program
			t.Fatal(err)
		}
	}
	script(run, `echo "$@" > `+runLog+`; exec sleep 30`)
	script(ctl, `echo "$@" > `+ctlLog+`; exit `+strconv.Itoa(ctlExit))
	return run, ctl, runLog, ctlLog
}

func waitForFile(t *testing.T, p string) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", p)
}

func TestRun_CancelStopsTheUnit(t *testing.T) {
	run, ctl, runLog, ctlLog := fakeSystemd(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		waitForFile(t, runLog)
		cancel()
	}()
	if _, err := Run(ctx, Config{Dir: t.TempDir(), SystemdRun: run, Systemctl: ctl}, []string{"true"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	started, err := os.ReadFile(runLog) // #nosec G304 -- test temp dir
	if err != nil {
		t.Fatal(err)
	}
	unit, ok := strings.CutPrefix(strings.Fields(string(started))[0], "--unit=")
	if !ok {
		t.Fatalf("no unit name: %s", started)
	}
	stopped, err := os.ReadFile(ctlLog) // #nosec G304 -- test temp dir
	if err != nil {
		t.Fatalf("the unit was never stopped: %v", err)
	}
	if got := strings.TrimSpace(string(stopped)); got != "--user stop -- "+unit {
		t.Errorf("systemctl %q, unit %q", got, unit)
	}
}

func TestRun_SaysSoWhenTheStopFails(t *testing.T) {
	for code, wantStopErr := range map[int]bool{5: false, 1: true} {
		run, ctl, runLog, _ := fakeSystemd(t, code)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			waitForFile(t, runLog)
			cancel()
		}()
		_, err := Run(ctx, Config{Dir: t.TempDir(), SystemdRun: run, Systemctl: ctl}, []string{"true"})
		cancel()
		if got := err != nil && strings.Contains(err.Error(), "stop sandbox-"); got != wantStopErr {
			t.Errorf("systemctl exit %d: %v", code, err)
		}
	}
}

func TestRun_NoStopWhenTheCheckEnds(t *testing.T) {
	run, ctl, _, ctlLog := fakeSystemd(t, 0)
	if err := os.WriteFile(run, []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil { // #nosec G306 -- test program
		t.Fatal(err)
	}
	r, err := Run(context.Background(), Config{Dir: t.TempDir(), SystemdRun: run, Systemctl: ctl}, []string{"true"})
	if err != nil || r.Exit != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := os.Stat(ctlLog); err == nil {
		t.Error("stopped a unit that had ended")
	}
}
