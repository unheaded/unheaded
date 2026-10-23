// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build usersystemd

package champion

// run_check through the real sandbox. Needs a user systemd session:
//   go test -race -tags usersystemd ./pkg/champion/ -run RunCheck

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const probeChecks = `version: 1
checks:
  probe:
    argv: [bash, "{path}"]
    timeout: 3s
`

// realAgentChampion: a worktree under an agent root, the real sandbox.
func realAgentChampion(t *testing.T, script string) (*Champion, string) {
	t.Helper()
	agent := t.TempDir()
	wt := filepath.Join(agent, "zhen-probe")
	if err := os.MkdirAll(wt, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "probe.sh"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ProjectRoot: wt}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	checks, err := ParseChecks([]byte(probeChecks))
	if err != nil {
		t.Fatal(err)
	}
	c.WithRunCheck(RunCheckConfig{Checks: checks, AgentWorktreeRoot: agent, Env: []string{"PATH=/usr/bin:/bin"}})
	return c, wt
}

func probe(t *testing.T, script string) RunCheckResult {
	t.Helper()
	c, _ := realAgentChampion(t, script)
	r, err := c.RunCheck(context.Background(), "probe", "probe.sh", "")
	if err != nil {
		t.Fatalf("run_check: %v", err)
	}
	return r
}

func TestRunCheckSystemd_NoNetwork(t *testing.T) {
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
	r := probe(t, fmt.Sprintf("(exec 3<>/dev/tcp/127.0.0.1/%d) 2>/dev/null && echo NET-LEAK || echo net-blocked\n", port))
	if strings.Contains(r.Output, "NET-LEAK") || !strings.Contains(r.Output, "net-blocked") {
		t.Errorf("network: %q", r.Output)
	}
}

func TestRunCheckSystemd_HomeIsNotTheRealHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	marker := filepath.Join(home, "zhen-runcheck-probe")
	r := probe(t, "echo x > "+marker+" 2>/dev/null; ls -A /home | wc -l\n")
	if _, err := os.Stat(marker); err == nil {
		_ = os.Remove(marker)
		t.Fatal("a check wrote into the real home")
	}
	_ = r
}

func TestRunCheckSystemd_MemoryKilled(t *testing.T) {
	r := probe(t, "python3 -c 'b = bytearray(3 << 30); print(\"ALLOCATED\")'\n")
	if r.Pass || strings.Contains(r.Output, "ALLOCATED") {
		t.Errorf("3 GiB allocation survived: %+v", r)
	}
}

func TestRunCheckSystemd_LongSleepKilled(t *testing.T) {
	r := probe(t, "sleep 700; echo WOKE\n") // the probe check's timeout is 3 s
	if r.Pass || strings.Contains(r.Output, "WOKE") {
		t.Errorf("sleep survived: %+v", r)
	}
}

func TestRunCheckSystemd_OutputTruncated(t *testing.T) {
	r := probe(t, "head -c 100000 /dev/zero | tr '\\0' x\n")
	if !r.Truncated || len(r.Output) != 16<<10 {
		t.Errorf("output %d bytes, truncated %v", len(r.Output), r.Truncated)
	}
}

func TestRunCheckSystemd_TasksCapped(t *testing.T) {
	r := probe(t, "for i in $(seq 1 300); do sleep 5 & done 2>/dev/null; jobs -p | wc -l\n")
	if strings.TrimSpace(r.Output) == "300" {
		t.Errorf("300 tasks under TasksMax=256: %q", r.Output)
	}
}

func TestRunCheckSystemd_GoTestInWorktree(t *testing.T) {
	agent := t.TempDir()
	wt := filepath.Join(agent, "zhen-go")
	_ = os.MkdirAll(wt, 0o700)
	for name, body := range map[string]string{
		"go.mod":    "module probe\n\ngo 1.22\n",
		"a.go":      "package probe\n\nfunc Two() int { return 2 }\n",
		"a_test.go": "package probe\n\nimport \"testing\"\n\nfunc TestTwo(t *testing.T) {\n\tif Two() != 2 {\n\t\tt.Fatal(\"no\")\n\t}\n}\n",
	} {
		_ = os.WriteFile(filepath.Join(wt, name), []byte(body), 0o600)
	}
	env, ro, rw, err := GoSandbox(context.Background(), filepath.Join(agent, ".gocache"))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := New(Config{ProjectRoot: wt}, &mockStore{})
	checks, _ := LoadChecks("../../configs/zhen-checks.yaml")
	c.WithRunCheck(RunCheckConfig{Checks: checks, AgentWorktreeRoot: agent, Env: env, ReadOnly: ro, Writable: rw})
	r, err := c.RunCheck(context.Background(), "go-test", "./...", "")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Pass {
		t.Errorf("go test in the sandbox: %+v", r)
	}
}
