// SPDX-License-Identifier: GPL-3.0-or-later
//
// host-agent — a small bare-metal host metrics agent.
//
// Runs NATIVELY on each Kingdom host (west, east, ...), reads the real host
// /proc + statfs's every mounted filesystem, and:
//   - serves GET /host-summary  → JSON matching the dashboard's per-host shape
//   - serves GET /metrics       → Prometheus host_* series (host/mount/device labels)
//   - streams the same metrics to VictoriaMetrics on an interval (TSDB history)
//
// Because it runs on bare metal it sees the true host — all disks/partitions,
// real process/connection counts, hostname — with no container namespace hacks.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ── output shapes ────────────────────────────────────────────────────────

type DiskInfo struct {
	Mount      string  `json:"mount"`
	Filesystem string  `json:"filesystem"`
	SizeBytes  uint64  `json:"size_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	AvailBytes uint64  `json:"avail_bytes"`
	UsePercent float64 `json:"use_percent"`
}

type NetConnections struct {
	Established int `json:"established"`
	TimeWait    int `json:"time_wait"`
	CloseWait   int `json:"close_wait"`
}

// HostSummary mirrors the dashboard's per-host HostInfo JSON.
type HostSummary struct {
	CPUPercent    float64        `json:"cpu_percent"`
	CPUCount      int            `json:"cpu_count"`
	MemoryTotal   uint64         `json:"memory_total"`
	MemoryUsed    uint64         `json:"memory_used"`
	MemoryPercent float64        `json:"memory_percent"`
	SwapTotal     uint64         `json:"swap_total"`
	SwapUsed      uint64         `json:"swap_used"`
	SwapPercent   float64        `json:"swap_percent"`
	Load1m        float64        `json:"load_1m"`
	Load5m        float64        `json:"load_5m"`
	Load15m       float64        `json:"load_15m"`
	UptimeSeconds float64        `json:"uptime_seconds"`
	Disks         []DiskInfo     `json:"disks"`
	NetConns      NetConnections `json:"net_connections"`
	ProcessTotal  int            `json:"process_total"`
	ProcessZombie int            `json:"process_zombie"`
	Hostname      string         `json:"hostname"`
	Kernel        string         `json:"kernel"`
}

// ── CPU delta state ──────────────────────────────────────────────────────

var (
	cpuMu               sync.Mutex
	prevTotal, prevIdle float64
)

func cpuPercent() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		var total, idle float64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseFloat(fields[i], 64)
			total += v
			if i == 4 || i == 5 { // idle + iowait
				idle += v
			}
		}
		cpuMu.Lock()
		dt, di := total-prevTotal, idle-prevIdle
		prevTotal, prevIdle = total, idle
		cpuMu.Unlock()
		if dt <= 0 {
			return 0
		}
		return (dt - di) / dt * 100
	}
	return 0
}

func memInfo() (memTotal, memUsed uint64, memPct float64, swapTotal, swapUsed uint64, swapPct float64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()
	var mAvail, sTotal, sFree uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		v *= 1024
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			memTotal = v
		case "MemAvailable":
			mAvail = v
		case "SwapTotal":
			sTotal = v
		case "SwapFree":
			sFree = v
		}
	}
	if memTotal > 0 && mAvail <= memTotal {
		memUsed = memTotal - mAvail
		memPct = float64(memUsed) / float64(memTotal) * 100
	}
	swapTotal = sTotal
	if sTotal > 0 && sFree <= sTotal {
		swapUsed = sTotal - sFree
		swapPct = float64(swapUsed) / float64(sTotal) * 100
	}
	return
}

func loadAvg() (l1, l5, l15 float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		l1, _ = strconv.ParseFloat(fields[0], 64)
		l5, _ = strconv.ParseFloat(fields[1], 64)
		l15, _ = strconv.ParseFloat(fields[2], 64)
	}
	return
}

func uptime() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		v, _ := strconv.ParseFloat(fields[0], 64)
		return v
	}
	return 0
}

var virtualFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "sysfs": true, "proc": true, "cgroup": true,
	"cgroup2": true, "overlay": true, "devpts": true, "securityfs": true,
	"debugfs": true, "tracefs": true, "hugetlbfs": true, "mqueue": true,
	"pstore": true, "configfs": true, "fusectl": true, "binfmt_misc": true,
	"autofs": true, "efivarfs": true, "bpf": true, "nsfs": true, "ramfs": true,
	"fuse.portal": true, "fuse.gvfsd-fuse": true, "squashfs": true,
}

func disks() []DiskInfo {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()
	seen := map[string]bool{}
	var out []DiskInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		device, mount, fstype := fields[0], fields[1], fields[2]
		if virtualFS[fstype] || !strings.HasPrefix(device, "/dev/") || strings.HasPrefix(device, "/dev/loop") {
			continue
		}
		if seen[device] {
			continue
		}
		seen[device] = true
		fi, err := os.Stat(mount)
		if err != nil || !fi.IsDir() {
			continue
		}
		var st syscall.Statfs_t
		if syscall.Statfs(mount, &st) != nil {
			continue
		}
		total := st.Blocks * uint64(st.Bsize)
		if total == 0 {
			continue
		}
		free := st.Bavail * uint64(st.Bsize)
		used := total - st.Bfree*uint64(st.Bsize)
		out = append(out, DiskInfo{
			Mount: mount, Filesystem: device,
			SizeBytes: total, UsedBytes: used, AvailBytes: free,
			UsePercent: float64(used) / float64(total) * 100,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

func netConns() NetConnections {
	var nc NetConnections
	for _, p := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Scan()
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 4 {
				continue
			}
			switch fields[3] {
			case "01":
				nc.Established++
			case "06":
				nc.TimeWait++
			case "08":
				nc.CloseWait++
			}
		}
		f.Close()
	}
	return nc
}

func procCounts() (total, zombie int) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		total++
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		s := string(data)
		if i := strings.LastIndexByte(s, ')'); i != -1 && i+2 < len(s) {
			if strings.TrimSpace(s[i+1:i+3]) == "Z" {
				zombie++
			}
		}
	}
	return
}

func kernel() string {
	if data, err := os.ReadFile("/proc/version"); err == nil {
		if f := strings.Fields(string(data)); len(f) >= 3 {
			return f[2]
		}
	}
	return ""
}

func collect(host string) HostSummary {
	mt, mu, mp, st, su, sp := memInfo()
	l1, l5, l15 := loadAvg()
	pt, pz := procCounts()
	hn, _ := os.Hostname()
	if host != "" {
		hn = host
	}
	return HostSummary{
		CPUPercent:  cpuPercent(),
		CPUCount:    runtime.NumCPU(),
		MemoryTotal: mt, MemoryUsed: mu, MemoryPercent: mp,
		SwapTotal: st, SwapUsed: su, SwapPercent: sp,
		Load1m: l1, Load5m: l5, Load15m: l15,
		UptimeSeconds: uptime(),
		Disks:         disks(),
		NetConns:      netConns(),
		ProcessTotal:  pt, ProcessZombie: pz,
		Hostname: hn, Kernel: kernel(),
	}
}

// ── Prometheus exposition ────────────────────────────────────────────────

func promText(host string, s HostSummary) string {
	var b strings.Builder
	g := func(name string, v float64) {
		fmt.Fprintf(&b, "%s{host=%q} %g\n", name, host, v)
	}
	g("host_cpu_percent", s.CPUPercent)
	g("host_memory_total_bytes", float64(s.MemoryTotal))
	g("host_memory_used_bytes", float64(s.MemoryUsed))
	g("host_memory_percent", s.MemoryPercent)
	g("host_swap_total_bytes", float64(s.SwapTotal))
	g("host_swap_used_bytes", float64(s.SwapUsed))
	g("host_load1", s.Load1m)
	g("host_load5", s.Load5m)
	g("host_load15", s.Load15m)
	g("host_uptime_seconds", s.UptimeSeconds)
	g("host_net_tcp_established", float64(s.NetConns.Established))
	g("host_net_tcp_time_wait", float64(s.NetConns.TimeWait))
	g("host_net_tcp_close_wait", float64(s.NetConns.CloseWait))
	g("host_procs_total", float64(s.ProcessTotal))
	g("host_procs_zombie", float64(s.ProcessZombie))
	for _, d := range s.Disks {
		lbl := fmt.Sprintf("{host=%q,mount=%q,device=%q}", host, d.Mount, d.Filesystem)
		fmt.Fprintf(&b, "host_disk_total_bytes%s %g\n", lbl, float64(d.SizeBytes))
		fmt.Fprintf(&b, "host_disk_used_bytes%s %g\n", lbl, float64(d.UsedBytes))
		fmt.Fprintf(&b, "host_disk_avail_bytes%s %g\n", lbl, float64(d.AvailBytes))
		fmt.Fprintf(&b, "host_disk_use_percent%s %g\n", lbl, d.UsePercent)
	}
	fmt.Fprintf(&b, "host_info{host=%q,hostname=%q,kernel=%q} 1\n", host, s.Hostname, s.Kernel)
	return b.String()
}

func main() {
	host := flag.String("host", "", "host label (default: hostname)")
	listen := flag.String("listen", ":9110", "listen address for /host-summary and /metrics")
	vm := flag.String("vm", "http://localhost:8428", "VictoriaMetrics base URL (empty to disable push)")
	interval := flag.Duration("interval", 10*time.Second, "collection/push interval")
	flag.Parse()

	label := *host
	if label == "" {
		label, _ = os.Hostname()
	}
	cpuPercent() // prime the CPU delta

	http.HandleFunc("/host-summary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(collect(label))
	})
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprint(w, promText(label, collect(label)))
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	go func() {
		if *vm == "" {
			return
		}
		url := strings.TrimRight(*vm, "/") + "/api/v1/import/prometheus"
		client := &http.Client{Timeout: 5 * time.Second}
		for {
			body := promText(label, collect(label))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(body)))
			if resp, err := client.Do(req); err == nil {
				resp.Body.Close()
			} else {
				fmt.Fprintf(os.Stderr, "vm push failed: %v\n", err)
			}
			cancel()
			time.Sleep(*interval)
		}
	}()

	fmt.Fprintf(os.Stderr, "host-agent: host=%s listen=%s vm=%s interval=%s\n", label, *listen, *vm, *interval)
	if err := http.ListenAndServe(*listen, nil); err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
}
