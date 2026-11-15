// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const hex64 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// cgTree makes dirs (relative to a temp root) and writes files into them
// (path relative to the root → content). It returns the root.
func cgTree(t *testing.T, dirs []string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for p, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func paths(stats []CgroupStat) map[string]CgroupStat {
	out := map[string]CgroupStat{}
	for _, s := range stats {
		out[s.Path] = s
	}
	return out
}

func TestCgroupsSelect(t *testing.T) {
	root := cgTree(t, []string{
		"system.slice/huginn.service",
		"system.slice/docker-" + hex64 + ".scope",
		"system.slice/session-3.scope",
		"user.slice/user-1000.slice/session-9.scope",
		"unheaded.slice/unheaded-env.slice/docker-" + hex64 + ".scope",
		"system.slice/foo.mount",
		"init.scope",
	}, nil)
	stats, truncated := collectCgroups(root, 4, 512, nil)
	if truncated {
		t.Fatal("truncated with 8 groups")
	}
	got := paths(stats)
	for _, want := range []string{
		"system.slice", "system.slice/huginn.service", "system.slice/docker-" + hex64[:12] + ".scope",
		"user.slice", "user.slice/user-1000.slice", "unheaded.slice", "unheaded.slice/unheaded-env.slice",
		"unheaded.slice/unheaded-env.slice/docker-" + hex64[:12] + ".scope",
	} {
		if _, ok := got[want]; !ok {
			t.Errorf("not selected: %s", want)
		}
	}
	for p := range got {
		if strings.Contains(p, "session-") || strings.HasSuffix(p, ".mount") || p == "init.scope" {
			t.Errorf("selected: %s", p)
		}
	}
	if len(got) != 8 {
		t.Errorf("selected %d groups, want 8: %v", len(got), got)
	}
}

func TestCgroupDockerID(t *testing.T) {
	root := cgTree(t, nil, map[string]string{"system.slice/docker-" + hex64 + ".scope/memory.current": "1\n"})
	stats, _ := collectCgroups(root, 4, 512, nil)
	s, ok := paths(stats)["system.slice/docker-"+hex64[:12]+".scope"]
	if !ok || s.ContainerID != hex64[:12] {
		t.Fatalf("docker scope: %+v", stats)
	}
	out := promTextCgroups("west", stats, false)
	if !strings.Contains(out, `cgroup="system.slice/docker-`+hex64[:12]+`.scope",container_id="`+hex64[:12]+`"`) {
		t.Fatalf("no container_id label:\n%s", out)
	}
	if strings.Contains(out, hex64) {
		t.Fatal("the full 64-hex id leaked into a label")
	}
}

func oneGroup(t *testing.T, files map[string]string) string {
	t.Helper()
	full := map[string]string{}
	for k, v := range files {
		full["system.slice/x.service/"+k] = v
	}
	root := cgTree(t, nil, full)
	stats, _ := collectCgroups(root, 4, 512, map[string]string{"259:0": "nvme0n1"})
	return promTextCgroups("west", stats, false)
}

func wantLines(t *testing.T, out string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(out, l+"\n") {
			t.Errorf("missing %q in:\n%s", l, out)
		}
	}
}

const xsvc = `host="west",cgroup="system.slice/x.service"`

func TestCgroupCPU(t *testing.T) {
	out := oneGroup(t, map[string]string{"cpu.stat": "usage_usec 2500000\nuser_usec 1\nnr_periods 10\nnr_throttled 4\nthrottled_usec 500000\n"})
	wantLines(t, out,
		"huginn_cgroup_cpu_usage_seconds_total{"+xsvc+"} 2.5",
		"huginn_cgroup_cpu_periods_total{"+xsvc+"} 10",
		"huginn_cgroup_cpu_throttled_periods_total{"+xsvc+"} 4",
		"huginn_cgroup_cpu_throttled_seconds_total{"+xsvc+"} 0.5")
}

func TestCgroupMemory(t *testing.T) {
	out := oneGroup(t, map[string]string{"memory.current": "1048576\n", "memory.max": "max\n", "memory.swap.current": "4096\n"})
	wantLines(t, out, "huginn_cgroup_memory_bytes{"+xsvc+"} 1048576", "huginn_cgroup_memory_swap_bytes{"+xsvc+"} 4096")
	if strings.Contains(out, "huginn_cgroup_memory_max_bytes") {
		t.Fatalf("memory.max max must give no line:\n%s", out)
	}
	out = oneGroup(t, map[string]string{"memory.current": "1048576\n", "memory.max": "2097152\n"})
	wantLines(t, out, "huginn_cgroup_memory_max_bytes{"+xsvc+"} 2097152")
}

func TestCgroupMemoryEvents(t *testing.T) {
	out := oneGroup(t, map[string]string{"memory.events": "low 0\nhigh 1\nmax 7\noom 2\noom_kill 2\n"})
	wantLines(t, out,
		"huginn_cgroup_memory_events_total{"+xsvc+`,event="low"} 0`,
		"huginn_cgroup_memory_events_total{"+xsvc+`,event="high"} 1`,
		"huginn_cgroup_memory_events_total{"+xsvc+`,event="max"} 7`,
		"huginn_cgroup_memory_events_total{"+xsvc+`,event="oom"} 2`,
		"huginn_cgroup_memory_events_total{"+xsvc+`,event="oom_kill"} 2`)
	if n := strings.Count(out, "huginn_cgroup_memory_events_total"); n != 5 {
		t.Fatalf("%d event lines, want 5:\n%s", n, out)
	}
}

func TestCgroupPressure(t *testing.T) {
	out := oneGroup(t, map[string]string{"io.pressure": "some avg10=0.00 avg60=0.00 avg300=0.00 total=1500000\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=250000\n"})
	wantLines(t, out,
		"huginn_cgroup_pressure_seconds_total{"+xsvc+`,resource="io",kind="some"} 1.5`,
		"huginn_cgroup_pressure_seconds_total{"+xsvc+`,resource="io",kind="full"} 0.25`)
}

func TestCgroupIOStat(t *testing.T) {
	out := oneGroup(t, map[string]string{"io.stat": "259:0 rbytes=100 wbytes=200 rios=1 wios=2 dbytes=0 dios=0\n8:99 rbytes=5 wbytes=6 rios=7 wios=8 dbytes=0 dios=0\n"})
	wantLines(t, out,
		"huginn_cgroup_io_bytes_total{"+xsvc+`,device="nvme0n1",dir="read"} 100`,
		"huginn_cgroup_io_bytes_total{"+xsvc+`,device="nvme0n1",dir="write"} 200`,
		"huginn_cgroup_io_ops_total{"+xsvc+`,device="nvme0n1",dir="read"} 1`,
		"huginn_cgroup_io_ops_total{"+xsvc+`,device="nvme0n1",dir="write"} 2`,
		"huginn_cgroup_io_bytes_total{"+xsvc+`,device="8:99",dir="read"} 5`)
}

func TestCgroupTruncated(t *testing.T) {
	var dirs []string
	for i := 0; i < 600; i++ {
		dirs = append(dirs, fmt.Sprintf("system.slice/s%03d.service", i))
	}
	stats, truncated := collectCgroups(cgTree(t, dirs, nil), 4, 512, nil)
	if !truncated || len(stats) != 512 {
		t.Fatalf("truncated=%v groups=%d, want true and 512", truncated, len(stats))
	}
	out := promTextCgroups("west", stats, truncated)
	wantLines(t, out, `huginn_cgroups_truncated{host="west"} 1`, `huginn_cgroups_seen{host="west"} 512`)
}

func TestCgroupMissingFilesSkipped(t *testing.T) {
	out := oneGroup(t, map[string]string{"cgroup.procs": "1\n"})
	if strings.Contains(out, "x.service") {
		t.Fatalf("a group with no stat files gave lines:\n%s", out)
	}
}

func TestBlockDevices(t *testing.T) {
	root := cgTree(t, nil, map[string]string{
		"259:0/uevent": "MAJOR=259\nMINOR=0\nDEVNAME=nvme0n1\nDEVTYPE=disk\n",
		"8:16/uevent":  "MAJOR=8\nMINOR=16\nDEVNAME=sdb\n",
		"9:9/uevent":   "DEVNAME=bad\"name\n",
	})
	got := blockDevices(root)
	if got["259:0"] != "nvme0n1" || got["8:16"] != "sdb" {
		t.Fatalf("blockDevices = %v", got)
	}
	if _, ok := got["9:9"]; ok {
		t.Fatal("a device name outside [a-z0-9] was accepted")
	}
}

func TestPromTextCgroupsAcceptedByTextfileRegex(t *testing.T) {
	root := cgTree(t, []string{"system.slice/docker-" + hex64 + ".scope"}, map[string]string{
		"system.slice/x.service/cpu.stat":       "usage_usec 1\nnr_periods 1\nnr_throttled 0\nthrottled_usec 0\n",
		"system.slice/x.service/memory.current": "1\n",
		"system.slice/x.service/memory.max":     "2\n",
		"system.slice/x.service/memory.events":  "low 0\nhigh 0\nmax 0\noom 0\noom_kill 0\n",
		"system.slice/x.service/cpu.pressure":   "some avg10=0.00 avg60=0.00 avg300=0.00 total=1\n",
		"system.slice/x.service/io.stat":        "259:0 rbytes=1 wbytes=1 rios=1 wios=1\n",
		"system.slice/x.service/pids.current":   "3\n",
	})
	stats, tr := collectCgroups(root, 4, 512, map[string]string{"259:0": "nvme0n1"})
	out := promTextCgroups("west", stats, tr)
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		n++
		if !textfileSample.MatchString(l) {
			t.Errorf("line not in the text format huginn accepts: %q", l)
		}
	}
	if n < 15 {
		t.Fatalf("only %d lines:\n%s", n, out)
	}
	wantLines(t, out, "huginn_cgroup_pids{"+xsvc+"} 3")
}

// A live read of this host's tree; it writes nothing. HUGINN_CGROUP_LIVE=1.
func TestCgroupsOnThisHost(t *testing.T) {
	if os.Getenv("HUGINN_CGROUP_LIVE") != "1" {
		t.Skip("set HUGINN_CGROUP_LIVE=1 to read /sys/fs/cgroup")
	}
	stats, truncated := collectCgroups("/sys/fs/cgroup", 4, 512, blockDevices("/sys/dev/block"))
	got := paths(stats)
	docker := 0
	for _, s := range stats {
		if s.ContainerID != "" {
			docker++
		}
	}
	t.Logf("%d groups selected, %d docker scopes, truncated=%v", len(stats), docker, truncated)
	if truncated || docker == 0 {
		t.Fatalf("truncated=%v docker scopes=%d", truncated, docker)
	}
	for _, want := range []string{"system.slice", "system.slice/huginn.service"} {
		if _, ok := got[want]; !ok {
			t.Errorf("not selected: %s", want)
		}
	}
}
