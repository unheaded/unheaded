// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.
//
// cgroups.go — per-cgroup series (ADR-147, Scientist F10 and A1; BlackMage
// F2). Before this, CPU and memory were known only for Go processes that
// export process_*; nothing saw a container's throttling, I/O, pressure or a
// memory kill.
//
// What is read, all read only from cgroupfs (v2; ProtectControlGroups=yes
// leaves it readable): cpu.stat, memory.current, memory.max,
// memory.swap.current, memory.events, cpu/memory/io.pressure, io.stat and
// pids.current.
//
// Which groups: every top-level *.slice, and below a slice every *.slice,
// *.service and docker-<64 hex>.scope, to depth 4, at most maxGroups. Session
// scopes, mounts and anything else are skipped: their names are not bounded.
// A name outside [A-Za-z0-9@:._-] is skipped, so every label is safe text.
//
// A docker scope's 64-hex id is shortened to the 12 that `docker ps` shows,
// in both the cgroup label and container_id: the long form adds nothing a
// person can type and makes every label 52 bytes longer.
//
// memory.events is hierarchical: a slice's count includes its children's,
// and it stays when a child goes. A container killed as its PID 1 takes its
// scope with it, so its own counter is never read again; the slice keeps it.
// MemoryKill (rules/containers.yml) reads the slices for that reason.
//
// Emitted metrics (labels host, cgroup, and container_id on docker scopes):
//   huginn_cgroup_cpu_usage_seconds_total, _cpu_periods_total,
//   _cpu_throttled_periods_total, _cpu_throttled_seconds_total
//   huginn_cgroup_memory_bytes, _memory_max_bytes (only when a limit is
//   set), _memory_swap_bytes, _memory_events_total{event}
//   huginn_cgroup_io_bytes_total{device,dir}, _io_ops_total{device,dir}
//   huginn_cgroup_pressure_seconds_total{resource,kind}
//   huginn_cgroup_pids
//   huginn_cgroups_seen, huginn_cgroups_truncated (host only)

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cgroupMaxDepth  = 4
	cgroupMaxGroups = 512
	// cgroupStatMax bounds what is read from one stat file.
	cgroupStatMax = 64 << 10
)

var (
	cgroupName   = regexp.MustCompile(`^[A-Za-z0-9@:._-]{1,128}$`)
	cgroupDocker = regexp.MustCompile(`^docker-([0-9a-f]{64})\.scope$`)
	blockDevName = regexp.MustCompile(`^[a-z0-9]{1,32}$`)
	majMin       = regexp.MustCompile(`^[0-9]+:[0-9]+$`)
	// The events and pressure resources read, in output order.
	cgroupEvents    = []string{"low", "high", "max", "oom", "oom_kill"}
	cgroupPressures = []string{"cpu", "memory", "io"}
)

// CgroupStat is one group's readings. A map holds only what was read.
type CgroupStat struct {
	Path, ContainerID string
	CPU               map[string]float64    // usage, periods, throttled_periods, throttled_seconds
	Mem               map[string]float64    // bytes, max_bytes, swap_bytes
	Events            map[string]float64    // memory.events
	Pressure          map[[2]string]float64 // {resource, some|full} → seconds
	IO                map[[2]string]float64 // {device, rbytes|wbytes|rios|wios}
	Pids              float64
	HasPids           bool
}

// collectCgroups walks root and reads every selected group. truncated is
// true when maxGroups stopped the walk.
func collectCgroups(root string, maxDepth, maxGroups int, devs map[string]string) (stats []CgroupStat, truncated bool) {
	var walk func(dir, rel string, depth int)
	walk = func(dir, rel string, depth int) {
		if depth > maxDepth || truncated {
			return
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() || !cgroupName.MatchString(e.Name()) {
				continue
			}
			name := e.Name()
			slice := strings.HasSuffix(name, ".slice")
			m := cgroupDocker.FindStringSubmatch(name)
			if !slice && (depth == 1 || (!strings.HasSuffix(name, ".service") && m == nil)) {
				continue
			}
			if len(stats) >= maxGroups {
				truncated = true
				return
			}
			label, id := name, ""
			if m != nil {
				id = m[1][:12]
				label = "docker-" + id + ".scope"
			}
			if rel != "" {
				label = rel + "/" + label
			}
			s := readCgroup(filepath.Join(dir, name), devs)
			s.Path, s.ContainerID = label, id
			stats = append(stats, s)
			if slice {
				walk(filepath.Join(dir, name), label, depth+1)
			}
		}
	}
	walk(root, "", 1)
	return stats, truncated
}

// readStat returns a small file's content, or "" when it cannot be read.
func readStat(path string) string {
	f, err := os.Open(path) // #nosec G304 -- a fixed file name inside a cgroupfs directory huginn listed
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, cgroupStatMax))
	return string(b)
}

// fields parses "key value" lines.
func fields(s string) map[string]float64 {
	out := map[string]float64{}
	for _, l := range strings.Split(s, "\n") {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		if v, err := strconv.ParseFloat(f[1], 64); err == nil {
			out[f[0]] = v
		}
	}
	return out
}

func readNumber(path string) (float64, bool) {
	s := strings.TrimSpace(readStat(path))
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func readCgroup(dir string, devs map[string]string) CgroupStat {
	s := CgroupStat{CPU: map[string]float64{}, Mem: map[string]float64{}, Events: map[string]float64{},
		Pressure: map[[2]string]float64{}, IO: map[[2]string]float64{}}
	if c := fields(readStat(filepath.Join(dir, "cpu.stat"))); len(c) > 0 {
		for k, out := range map[string]string{"usage_usec": "usage", "throttled_usec": "throttled_seconds"} {
			if v, ok := c[k]; ok {
				s.CPU[out] = v / 1e6
			}
		}
		for k, out := range map[string]string{"nr_periods": "periods", "nr_throttled": "throttled_periods"} {
			if v, ok := c[k]; ok {
				s.CPU[out] = v
			}
		}
	}
	if v, ok := readNumber(filepath.Join(dir, "memory.current")); ok {
		s.Mem["bytes"] = v
	}
	if v, ok := readNumber(filepath.Join(dir, "memory.max")); ok { // "max" (no limit) does not parse: no line
		s.Mem["max_bytes"] = v
	}
	if v, ok := readNumber(filepath.Join(dir, "memory.swap.current")); ok {
		s.Mem["swap_bytes"] = v
	}
	ev := fields(readStat(filepath.Join(dir, "memory.events")))
	for _, k := range cgroupEvents {
		if v, ok := ev[k]; ok {
			s.Events[k] = v
		}
	}
	for _, res := range cgroupPressures {
		for _, l := range strings.Split(readStat(filepath.Join(dir, res+".pressure")), "\n") {
			f := strings.Fields(l)
			if len(f) == 0 || (f[0] != "some" && f[0] != "full") {
				continue
			}
			for _, kv := range f[1:] {
				if t, ok := strings.CutPrefix(kv, "total="); ok {
					if v, err := strconv.ParseFloat(t, 64); err == nil {
						s.Pressure[[2]string{res, f[0]}] = v / 1e6
					}
				}
			}
		}
	}
	for _, l := range strings.Split(readStat(filepath.Join(dir, "io.stat")), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 || !majMin.MatchString(f[0]) {
			continue
		}
		dev := f[0]
		if n, ok := devs[dev]; ok {
			dev = n
		}
		for _, kv := range f[1:] {
			k, val, ok := strings.Cut(kv, "=")
			if !ok || (k != "rbytes" && k != "wbytes" && k != "rios" && k != "wios") {
				continue
			}
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				s.IO[[2]string{dev, k}] = v
			}
		}
	}
	if v, ok := readNumber(filepath.Join(dir, "pids.current")); ok {
		s.Pids, s.HasPids = v, true
	}
	return s
}

// blockDevices maps "major:minor" to the kernel's device name from
// <sysDevBlock>/<maj:min>/uevent. A name outside [a-z0-9] is left out.
func blockDevices(sysDevBlock string) map[string]string {
	out := map[string]string{}
	ents, err := os.ReadDir(sysDevBlock)
	if err != nil {
		return out
	}
	for _, e := range ents {
		sc := bufio.NewScanner(strings.NewReader(readStat(filepath.Join(sysDevBlock, e.Name(), "uevent"))))
		for sc.Scan() {
			if n, ok := strings.CutPrefix(sc.Text(), "DEVNAME="); ok && blockDevName.MatchString(n) {
				out[e.Name()] = n
			}
		}
	}
	return out
}

// devCache refreshes the device map every 10 minutes: devices rarely change
// and /sys/dev/block has an entry per partition.
type devCache struct {
	mu   sync.Mutex
	dir  string
	at   time.Time
	devs map[string]string
}

func (c *devCache) get() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.devs == nil || time.Since(c.at) > 10*time.Minute {
		c.devs, c.at = blockDevices(c.dir), time.Now()
	}
	return c.devs
}

func promTextCgroups(host string, stats []CgroupStat, truncated bool) string {
	var b strings.Builder
	for _, s := range stats {
		lbl := fmt.Sprintf("host=%q,cgroup=%q", host, s.Path)
		if s.ContainerID != "" {
			lbl += fmt.Sprintf(",container_id=%q", s.ContainerID)
		}
		line := func(name, extra string, v float64) {
			fmt.Fprintf(&b, "%s{%s%s} %s\n", name, lbl, extra, strconv.FormatFloat(v, 'f', -1, 64))
		}
		for _, k := range []struct{ key, name string }{
			{"usage", "huginn_cgroup_cpu_usage_seconds_total"},
			{"periods", "huginn_cgroup_cpu_periods_total"},
			{"throttled_periods", "huginn_cgroup_cpu_throttled_periods_total"},
			{"throttled_seconds", "huginn_cgroup_cpu_throttled_seconds_total"},
		} {
			if v, ok := s.CPU[k.key]; ok {
				line(k.name, "", v)
			}
		}
		for _, k := range []struct{ key, name string }{
			{"bytes", "huginn_cgroup_memory_bytes"},
			{"max_bytes", "huginn_cgroup_memory_max_bytes"},
			{"swap_bytes", "huginn_cgroup_memory_swap_bytes"},
		} {
			if v, ok := s.Mem[k.key]; ok {
				line(k.name, "", v)
			}
		}
		for _, e := range cgroupEvents {
			if v, ok := s.Events[e]; ok {
				line("huginn_cgroup_memory_events_total", fmt.Sprintf(",event=%q", e), v)
			}
		}
		for _, res := range cgroupPressures {
			for _, kind := range []string{"some", "full"} {
				if v, ok := s.Pressure[[2]string{res, kind}]; ok {
					line("huginn_cgroup_pressure_seconds_total", fmt.Sprintf(",resource=%q,kind=%q", res, kind), v)
				}
			}
		}
		var devs []string
		seen := map[string]bool{}
		for k := range s.IO {
			if !seen[k[0]] {
				seen[k[0]] = true
				devs = append(devs, k[0])
			}
		}
		sort.Strings(devs)
		for _, d := range devs {
			for _, k := range []struct{ key, name, dir string }{
				{"rbytes", "huginn_cgroup_io_bytes_total", "read"}, {"wbytes", "huginn_cgroup_io_bytes_total", "write"},
				{"rios", "huginn_cgroup_io_ops_total", "read"}, {"wios", "huginn_cgroup_io_ops_total", "write"},
			} {
				if v, ok := s.IO[[2]string{d, k.key}]; ok {
					line(k.name, fmt.Sprintf(",device=%q,dir=%q", d, k.dir), v)
				}
			}
		}
		if s.HasPids {
			line("huginn_cgroup_pids", "", s.Pids)
		}
	}
	tr := 0
	if truncated {
		tr = 1
	}
	fmt.Fprintf(&b, "huginn_cgroups_seen{host=%q} %d\n", host, len(stats))
	fmt.Fprintf(&b, "huginn_cgroups_truncated{host=%q} %d\n", host, tr)
	return b.String()
}
