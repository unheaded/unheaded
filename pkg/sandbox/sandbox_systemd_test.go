// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build usersystemd

package sandbox

// These need a user systemd session (systemd-run --user); CI has none.
// Run them on west: go test -race -tags usersystemd ./pkg/sandbox/

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, cfg Config, script string) Result {
	t.Helper()
	if cfg.Dir == "" {
		cfg.Dir = t.TempDir()
	}
	r, err := Run(context.Background(), cfg, []string{"/bin/sh", "-c", script})
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, r.Output)
	}
	return r
}

func TestSystemd_NoNetwork(t *testing.T) {
	// A listener on the host's loopback: reachable from here, not from
	// the sandbox (its own network namespace has only its own lo).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	probe := fmt.Sprintf(`python3 -c 'import socket; socket.create_connection(("127.0.0.1", %d), 2); print("NET-LEAK")' 2>/dev/null || echo net-blocked`, port)
	if c, err := net.Dial("tcp", ln.Addr().String()); err != nil {
		t.Fatalf("the listener is not reachable from the host either: %v", err)
	} else {
		_ = c.Close()
	}
	r := run(t, Config{}, probe)
	if strings.Contains(r.Output, "NET-LEAK") || !strings.Contains(r.Output, "net-blocked") {
		t.Errorf("host loopback reachable from the sandbox: %q", r.Output)
	}
}

func TestSystemd_HomeHiddenAndUnwritable(t *testing.T) {
	home, _ := os.UserHomeDir()
	r := run(t, Config{}, `ls -A "$HOME" 2>/dev/null | wc -l; echo x > "$HOME/zhen-sbx-probe" 2>/dev/null; ls -A `+home+` 2>/dev/null | wc -l`)
	if _, err := os.Stat(filepath.Join(home, "zhen-sbx-probe")); err == nil {
		_ = os.Remove(filepath.Join(home, "zhen-sbx-probe"))
		t.Fatal("the sandbox wrote into the real home")
	}
	if !strings.HasPrefix(strings.TrimSpace(r.Output), "0") {
		t.Errorf("home visible: %q", r.Output)
	}
}

func TestSystemd_OnlyDirWritable(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	r := run(t, Config{Dir: dir}, `echo ok > inside.txt; echo bad > `+outside+`/outside.txt 2>/dev/null; ls /tmp | wc -l`)
	if _, err := os.Stat(filepath.Join(dir, "inside.txt")); err != nil {
		t.Error("the working dir is not writable")
	}
	if _, err := os.Stat(filepath.Join(outside, "outside.txt")); err == nil {
		t.Error("a path outside the sandbox was writable")
	}
	_ = r
}

func TestSystemd_ExitCodePropagates(t *testing.T) {
	if r := run(t, Config{}, `exit 7`); r.Exit != 7 {
		t.Errorf("exit = %d", r.Exit)
	}
}

func TestSystemd_MemoryLimitKills(t *testing.T) {
	// 3 GiB of allocation against MemoryMax=512M (and no swap).
	r := run(t, Config{MemoryMax: "512M"}, `python3 -c 'b = bytearray(3 << 30); print("ALLOCATED")'`)
	if r.Exit == 0 || strings.Contains(r.Output, "ALLOCATED") {
		t.Errorf("3 GiB allocation survived: exit %d %q", r.Exit, r.Output)
	}
}

func TestSystemd_RuntimeLimitKills(t *testing.T) {
	t0 := time.Now()
	r := run(t, Config{RuntimeMax: 2 * time.Second}, `sleep 700; echo WOKE`)
	if r.Exit == 0 || strings.Contains(r.Output, "WOKE") || time.Since(t0) > 30*time.Second {
		t.Errorf("sleep not killed: exit %d after %s", r.Exit, time.Since(t0))
	}
}

func TestSystemd_TasksLimit(t *testing.T) {
	r := run(t, Config{TasksMax: 16}, `n=0; for i in $(seq 1 64); do sleep 5 & n=$((n+1)); done 2>/dev/null; jobs -p | wc -l`)
	if strings.TrimSpace(r.Output) == "64" {
		t.Errorf("64 tasks started under TasksMax=16: %q", r.Output)
	}
}

func TestSystemd_OutputTruncated(t *testing.T) {
	r := run(t, Config{OutputLimit: 1024}, `head -c 100000 /dev/zero | tr '\0' x`)
	if len(r.Output) != 1024 || !r.Truncated {
		t.Errorf("output %d bytes, truncated %v", len(r.Output), r.Truncated)
	}
}

func TestSystemd_EnvIsOnlyWhatIsPassed(t *testing.T) {
	t.Setenv("ZHEN_SBX_SECRET", "should-not-leak")
	r := run(t, Config{Env: []string{"ONLY=this"}}, `echo "ONLY=$ONLY SECRET=$ZHEN_SBX_SECRET"`)
	if !strings.Contains(r.Output, "ONLY=this") || strings.Contains(r.Output, "should-not-leak") {
		t.Errorf("env: %q", r.Output)
	}
}

func TestSystemd_Available(t *testing.T) {
	if err := Available(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSystemd_KingdomConfigHidden(t *testing.T) {
	if _, err := os.Stat("/etc/unheaded"); err != nil {
		t.Skip("no /etc/unheaded on this host")
	}
	r := run(t, Config{}, `ls /etc/unheaded 2>&1 | head -3; cat /etc/unheaded/*.env 2>&1 | head -c 200; echo END`)
	if !strings.Contains(r.Output, "Permission denied") && !strings.Contains(r.Output, "cannot open") && !strings.Contains(r.Output, "No such file") {
		t.Errorf("/etc/unheaded readable in the sandbox: %q", r.Output)
	}
	if strings.Contains(r.Output, "ANTHROPIC") || strings.Contains(r.Output, "KEY=") {
		t.Fatal("a credential file was readable in the sandbox")
	}
}
