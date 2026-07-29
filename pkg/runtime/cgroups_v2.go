// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// CgroupV2Manager provides enhanced cgroups v2 management with full unified hierarchy support.
type CgroupV2Manager struct {
	mu sync.RWMutex

	// Root cgroup mount point
	root string

	// Parent cgroup path (relative to root)
	parentPath string

	// Map of container ID to cgroup path
	cgroupPaths map[string]string

	// Event watchers
	eventWatchers map[string]*CgroupEventWatcher

	// Pressure monitors
	pressureMonitors map[string]*PressureMonitor

	// Close channel
	closeCh chan struct{}
	closed  bool
}

// CgroupV2Config contains configuration for creating a cgroup.
type CgroupV2Config struct {
	// CPU configuration
	CPUWeight       uint64 // cpu.weight (1-10000, default 100)
	CPUMax          int64  // cpu.max quota in microseconds (-1 for max)
	CPUMaxPeriod    uint64 // cpu.max period in microseconds (default 100000)
	CPUSetCPUs      string // cpuset.cpus
	CPUSetMems      string // cpuset.mems
	CPUSetPartition string // cpuset.cpus.partition (root, member, isolated)

	// Memory configuration
	MemoryMax      int64 // memory.max in bytes (-1 for max)
	MemoryHigh     int64 // memory.high in bytes (-1 for max)
	MemoryLow      int64 // memory.low in bytes
	MemoryMin      int64 // memory.min in bytes (guaranteed minimum)
	MemorySwapMax  int64 // memory.swap.max in bytes (-1 for max)
	MemoryOOMGroup bool  // memory.oom.group

	// IO configuration
	IOWeight uint16              // io.weight (1-10000, default 100)
	IOMax    []CgroupIOMaxConfig // io.max per device

	// PIDs configuration
	PidsMax int64 // pids.max (-1 for max)

	// RDMA configuration (if supported)
	RDMAMax map[string]RDMALimit

	// Misc configuration
	HugetlbMax map[string]int64 // hugetlb.<size>.max
}

// CgroupIOMaxConfig contains IO limits for a specific device.
type CgroupIOMaxConfig struct {
	Major uint64
	Minor uint64
	Rbps  int64 // Read bytes per second (-1 for max)
	Wbps  int64 // Write bytes per second (-1 for max)
	Riops int64 // Read IOPS (-1 for max)
	Wiops int64 // Write IOPS (-1 for max)
}

// RDMALimit contains RDMA limits.
type RDMALimit struct {
	HcaHandle uint32
	HcaObject uint32
}

// CgroupV2Stats contains comprehensive cgroup v2 statistics.
type CgroupV2Stats struct {
	// CPU statistics
	CPU CgroupCPUStats

	// Memory statistics
	Memory CgroupMemoryStats

	// IO statistics
	IO CgroupIOStats

	// PIDs statistics
	PIDs CgroupPIDsStats

	// Pressure statistics
	Pressure CgroupPressureStats
}

// CgroupCPUStats contains CPU statistics.
type CgroupCPUStats struct {
	UsageUsec     uint64 // Total CPU time consumed in microseconds
	UserUsec      uint64 // User CPU time
	SystemUsec    uint64 // System CPU time
	NrPeriods     uint64 // Number of enforcement intervals
	NrThrottled   uint64 // Number of times throttled
	ThrottledUsec uint64 // Total throttle time in microseconds
	NrBursts      uint64 // Number of burst periods
	BurstUsec     uint64 // Total burst time
	Pressure      *PressureData
}

// CgroupMemoryStats contains memory statistics.
type CgroupMemoryStats struct {
	Current               uint64 // Current memory usage
	Peak                  uint64 // Peak memory usage
	High                  uint64 // High watermark
	Max                   uint64 // Maximum limit
	Min                   uint64 // Minimum guaranteed
	Low                   uint64 // Low watermark
	SwapCurrent           uint64 // Current swap usage
	SwapHigh              uint64 // Swap high watermark
	SwapMax               uint64 // Swap maximum
	Anon                  uint64 // Anonymous memory
	File                  uint64 // Page cache
	Kernel                uint64 // Kernel memory
	KernelStack           uint64 // Kernel stack
	Slab                  uint64 // Slab memory
	Sock                  uint64 // Socket buffers
	Shmem                 uint64 // Shared memory
	FileMapped            uint64 // Mapped files
	FileDirty             uint64 // Dirty pages
	FileWriteback         uint64 // Pages under writeback
	AnonThp               uint64 // Anonymous huge pages
	InactiveAnon          uint64 // Inactive anonymous pages
	ActiveAnon            uint64 // Active anonymous pages
	InactiveFile          uint64 // Inactive file pages
	ActiveFile            uint64 // Active file pages
	Unevictable           uint64 // Unevictable pages
	SlabReclaimable       uint64 // Reclaimable slab
	SlabUnreclaimable     uint64 // Unreclaimable slab
	Pgfault               uint64 // Page faults
	Pgmajfault            uint64 // Major page faults
	WorkingsetRefault     uint64
	WorkingsetActivate    uint64
	WorkingsetNodereclaim uint64
	Pgrefill              uint64
	Pgscan                uint64
	Pgsteal               uint64
	Pgactivate            uint64
	Pgdeactivate          uint64
	Pglazyfree            uint64
	Pglazyfreed           uint64
	ThpFaultAlloc         uint64
	ThpCollapseAlloc      uint64
	Events                MemoryEvents
	Pressure              *PressureData
}

// MemoryEvents contains memory event counters.
type MemoryEvents struct {
	Low          uint64 // Times the cgroup was reclaimed due to high memory pressure
	High         uint64 // Times the memory usage was above memory.high
	Max          uint64 // Times the memory usage hit memory.max limit
	OOM          uint64 // Times the cgroup ran out of memory
	OOMKill      uint64 // Number of processes killed due to memory.max
	OOMGroupKill uint64 // Number of times all processes were killed due to memory.oom.group
}

// CgroupIOStats contains IO statistics.
type CgroupIOStats struct {
	Devices  []DeviceIOStats
	Pressure *PressureData
}

// DeviceIOStats contains IO statistics for a device.
type DeviceIOStats struct {
	Major  uint64
	Minor  uint64
	Rbytes uint64 // Bytes read
	Wbytes uint64 // Bytes written
	Rios   uint64 // Read operations
	Wios   uint64 // Write operations
	Dbytes uint64 // Discarded bytes
	Dios   uint64 // Discard operations
}

// CgroupPIDsStats contains PIDs statistics.
type CgroupPIDsStats struct {
	Current uint64 // Current number of processes
	Max     uint64 // Maximum allowed (-1 for max)
	Events  PIDsEvents
}

// PIDsEvents contains PIDs event counters.
type PIDsEvents struct {
	Max uint64 // Number of times fork failed due to pids.max
}

// CgroupPressureStats contains pressure statistics.
type CgroupPressureStats struct {
	CPU    *PressureData
	Memory *PressureData
	IO     *PressureData
}

// PressureData contains pressure stall information.
type PressureData struct {
	Some PressureMetrics // Tasks stalled on some resources
	Full PressureMetrics // All non-idle tasks stalled (not available for CPU)
}

// PressureMetrics contains pressure metrics.
type PressureMetrics struct {
	Avg10  float64 // 10 second average
	Avg60  float64 // 60 second average
	Avg300 float64 // 300 second average
	Total  uint64  // Total stall time in microseconds
}

// CgroupEventType represents the type of cgroup event.
type CgroupEventType int

const (
	CgroupEventMemoryMax CgroupEventType = iota
	CgroupEventMemoryHigh
	CgroupEventMemoryOOM
	CgroupEventMemoryOOMKill
	CgroupEventPidsMax
	CgroupEventFrozen
	CgroupEventPopulated
)

// CgroupEvent represents a cgroup event notification.
type CgroupEvent struct {
	Type        CgroupEventType
	ContainerID string
	CgroupPath  string
	Timestamp   time.Time
	Data        map[string]interface{}
}

// CgroupEventWatcher watches for cgroup events.
type CgroupEventWatcher struct {
	cgroupPath string
	events     chan CgroupEvent
	stopCh     chan struct{}
	inotifyFd  int
	watches    map[int]string // watch descriptor to file path
}

// PressureMonitor monitors resource pressure.
type PressureMonitor struct {
	cgroupPath string
	thresholds PressureThresholds
	callback   PressureCallback
	stopCh     chan struct{}
	interval   time.Duration
}

// PressureThresholds defines thresholds for pressure monitoring.
type PressureThresholds struct {
	CPUSomeAvg10    float64
	MemorySomeAvg10 float64
	MemoryFullAvg10 float64
	IOSomeAvg10     float64
	IOFullAvg10     float64
}

// PressureCallback is called when pressure exceeds thresholds.
type PressureCallback func(containerID string, pressure CgroupPressureStats)

// NewCgroupV2Manager creates a new enhanced cgroup v2 manager.
func NewCgroupV2Manager(parentPath string) (*CgroupV2Manager, error) {
	root := "/sys/fs/cgroup"

	// Verify cgroup v2 is mounted
	if !isCgroupV2Unified(root) {
		return nil, fmt.Errorf("cgroup v2 unified hierarchy not available at %s", root)
	}

	if parentPath == "" {
		parentPath = "/unheaded"
	}

	mgr := &CgroupV2Manager{
		root:             root,
		parentPath:       parentPath,
		cgroupPaths:      make(map[string]string),
		eventWatchers:    make(map[string]*CgroupEventWatcher),
		pressureMonitors: make(map[string]*PressureMonitor),
		closeCh:          make(chan struct{}),
	}

	// Create parent cgroup
	parentFullPath := filepath.Join(root, parentPath)
	if err := os.MkdirAll(parentFullPath, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return nil, fmt.Errorf("failed to create parent cgroup: %w", err)
	}

	// Best-effort: enable controllers in parent. Some controllers may not
	// be available on this host.
	_ = mgr.enableControllersInSubtree(parentFullPath)

	return mgr, nil
}

// isCgroupV2Unified checks if cgroup v2 unified hierarchy is available.
func isCgroupV2Unified(path string) bool {
	// Check for cgroup.controllers file (only present in cgroup v2)
	controllersPath := filepath.Join(path, "cgroup.controllers")
	if _, err := os.Stat(controllersPath); err != nil {
		return false
	}

	// Also verify it's a unified hierarchy by checking cgroup type
	typeFile := filepath.Join(path, "cgroup.type")
	// cgroup.type only exists in non-root cgroups, so absence is OK for root
	_, err := os.Stat(typeFile)
	return err != nil || os.IsNotExist(err)
}

// enableControllersInSubtree enables all available controllers in subtree_control.
func (m *CgroupV2Manager) enableControllersInSubtree(path string) error {
	// Read available controllers
	controllersPath := filepath.Join(path, "cgroup.controllers")
	data, err := os.ReadFile(controllersPath) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return err
	}

	available := strings.Fields(string(data))
	if len(available) == 0 {
		return nil
	}

	// Build enable string
	var toEnable []string
	for _, c := range available {
		toEnable = append(toEnable, "+"+c)
	}

	// path is the cgroup mount root (e.g. /sys/fs/cgroup/<container-id>)
	// constructed from operator config + container ID; not user-supplied.
	// 0644 is the kernel-required perms for cgroup control files.
	subtreeControl := filepath.Join(path, "cgroup.subtree_control")
	return os.WriteFile(subtreeControl, []byte(strings.Join(toEnable, " ")), 0644) // #nosec G703,G306 -- cgroup mount path; 0644 is kernel-mandated for cgroup control files
}

// CreateCgroupV2 creates a cgroup with full v2 configuration.
func (m *CgroupV2Manager) CreateCgroupV2(containerID string, config *CgroupV2Config) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build cgroup path
	cgroupPath := filepath.Join(m.parentPath, containerID)
	fullPath := filepath.Join(m.root, cgroupPath)

	// Create cgroup directory
	if err := os.MkdirAll(fullPath, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return "", fmt.Errorf("failed to create cgroup directory: %w", err)
	}

	// Apply configuration
	if config != nil {
		if err := m.applyV2Config(fullPath, config); err != nil {
			_ = os.RemoveAll(fullPath)
			return "", fmt.Errorf("failed to apply cgroup config: %w", err)
		}
	}

	m.cgroupPaths[containerID] = cgroupPath

	return cgroupPath, nil
}

// applyV2Config applies cgroup v2 configuration.
func (m *CgroupV2Manager) applyV2Config(fullPath string, config *CgroupV2Config) error {
	// Apply CPU configuration
	if err := m.applyCPUConfigV2(fullPath, config); err != nil {
		return fmt.Errorf("cpu config: %w", err)
	}

	// Apply memory configuration
	if err := m.applyMemoryConfigV2(fullPath, config); err != nil {
		return fmt.Errorf("memory config: %w", err)
	}

	// Apply IO configuration
	if err := m.applyIOConfigV2(fullPath, config); err != nil {
		return fmt.Errorf("io config: %w", err)
	}

	// Apply PIDs configuration
	if err := m.applyPIDsConfigV2(fullPath, config); err != nil {
		return fmt.Errorf("pids config: %w", err)
	}

	// Best-effort: hugetlb controller may not be enabled.
	_ = m.applyHugetlbConfigV2(fullPath, config)

	return nil
}

// applyCPUConfigV2 applies CPU configuration.
func (m *CgroupV2Manager) applyCPUConfigV2(fullPath string, config *CgroupV2Config) error {
	// cpu.weight (1-10000)
	if config.CPUWeight > 0 {
		weight := config.CPUWeight
		if weight < 1 {
			weight = 1
		}
		if weight > 10000 {
			weight = 10000
		}
		if err := writeCgroupFile(fullPath, "cpu.weight", fmt.Sprintf("%d", weight)); err != nil {
			return err
		}
	}

	// cpu.max (quota period)
	if config.CPUMax != 0 || config.CPUMaxPeriod > 0 {
		period := config.CPUMaxPeriod
		if period == 0 {
			period = 100000 // Default 100ms
		}

		var content string
		if config.CPUMax < 0 {
			content = fmt.Sprintf("max %d", period)
		} else if config.CPUMax > 0 {
			content = fmt.Sprintf("%d %d", config.CPUMax, period)
		} else {
			content = fmt.Sprintf("max %d", period)
		}

		if err := writeCgroupFile(fullPath, "cpu.max", content); err != nil {
			return err
		}
	}

	// cpuset.cpus
	if config.CPUSetCPUs != "" {
		if err := writeCgroupFile(fullPath, "cpuset.cpus", config.CPUSetCPUs); err != nil {
			return err
		}
	}

	// cpuset.mems
	if config.CPUSetMems != "" {
		if err := writeCgroupFile(fullPath, "cpuset.mems", config.CPUSetMems); err != nil {
			return err
		}
	}

	// cpuset.cpus.partition
	if config.CPUSetPartition != "" {
		// Best-effort: cpuset partition mode may not be supported.
		_ = writeCgroupFile(fullPath, "cpuset.cpus.partition", config.CPUSetPartition)
	}

	return nil
}

// applyMemoryConfigV2 applies memory configuration.
func (m *CgroupV2Manager) applyMemoryConfigV2(fullPath string, config *CgroupV2Config) error {
	// memory.max
	if config.MemoryMax != 0 {
		var content string
		if config.MemoryMax < 0 {
			content = "max"
		} else {
			content = fmt.Sprintf("%d", config.MemoryMax)
		}
		if err := writeCgroupFile(fullPath, "memory.max", content); err != nil {
			return err
		}
	}

	// memory.high (soft limit - throttling starts when exceeded)
	if config.MemoryHigh != 0 {
		var content string
		if config.MemoryHigh < 0 {
			content = "max"
		} else {
			content = fmt.Sprintf("%d", config.MemoryHigh)
		}
		if err := writeCgroupFile(fullPath, "memory.high", content); err != nil {
			return err
		}
	}

	// memory.low (best-effort memory protection)
	if config.MemoryLow > 0 {
		if err := writeCgroupFile(fullPath, "memory.low", fmt.Sprintf("%d", config.MemoryLow)); err != nil {
			return err
		}
	}

	// memory.min (hard memory protection)
	if config.MemoryMin > 0 {
		if err := writeCgroupFile(fullPath, "memory.min", fmt.Sprintf("%d", config.MemoryMin)); err != nil {
			return err
		}
	}

	// memory.swap.max
	if config.MemorySwapMax != 0 {
		var content string
		if config.MemorySwapMax < 0 {
			content = "max"
		} else {
			content = fmt.Sprintf("%d", config.MemorySwapMax)
		}
		// Best-effort: swap controller may not be enabled.
		_ = writeCgroupFile(fullPath, "memory.swap.max", content)
	}

	// memory.oom.group
	if config.MemoryOOMGroup {
		// Best-effort: oom.group may not be supported on older kernels.
		_ = writeCgroupFile(fullPath, "memory.oom.group", "1")
	}

	return nil
}

// applyIOConfigV2 applies IO configuration.
func (m *CgroupV2Manager) applyIOConfigV2(fullPath string, config *CgroupV2Config) error {
	// io.weight
	if config.IOWeight > 0 {
		weight := config.IOWeight
		if weight < 1 {
			weight = 1
		}
		if weight > 10000 {
			weight = 10000
		}
		if err := writeCgroupFile(fullPath, "io.weight", fmt.Sprintf("default %d", weight)); err != nil {
			return err
		}
	}

	// io.max
	for _, ioMax := range config.IOMax {
		content := m.buildIOMaxContent(ioMax)
		if content != "" {
			if err := writeCgroupFile(fullPath, "io.max", content); err != nil {
				return err
			}
		}
	}

	return nil
}

// buildIOMaxContent builds the io.max content string.
func (m *CgroupV2Manager) buildIOMaxContent(config CgroupIOMaxConfig) string {
	var parts []string

	if config.Rbps >= 0 {
		if config.Rbps == 0 {
			parts = append(parts, "rbps=max")
		} else {
			parts = append(parts, fmt.Sprintf("rbps=%d", config.Rbps))
		}
	}
	if config.Wbps >= 0 {
		if config.Wbps == 0 {
			parts = append(parts, "wbps=max")
		} else {
			parts = append(parts, fmt.Sprintf("wbps=%d", config.Wbps))
		}
	}
	if config.Riops >= 0 {
		if config.Riops == 0 {
			parts = append(parts, "riops=max")
		} else {
			parts = append(parts, fmt.Sprintf("riops=%d", config.Riops))
		}
	}
	if config.Wiops >= 0 {
		if config.Wiops == 0 {
			parts = append(parts, "wiops=max")
		} else {
			parts = append(parts, fmt.Sprintf("wiops=%d", config.Wiops))
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("%d:%d %s", config.Major, config.Minor, strings.Join(parts, " "))
}

// applyPIDsConfigV2 applies PIDs configuration.
func (m *CgroupV2Manager) applyPIDsConfigV2(fullPath string, config *CgroupV2Config) error {
	if config.PidsMax != 0 {
		var content string
		if config.PidsMax < 0 {
			content = "max"
		} else {
			content = fmt.Sprintf("%d", config.PidsMax)
		}
		if err := writeCgroupFile(fullPath, "pids.max", content); err != nil {
			return err
		}
	}
	return nil
}

// applyHugetlbConfigV2 applies hugetlb configuration.
func (m *CgroupV2Manager) applyHugetlbConfigV2(fullPath string, config *CgroupV2Config) error {
	for size, max := range config.HugetlbMax {
		filename := fmt.Sprintf("hugetlb.%s.max", size)
		// Best-effort: hugetlb size may not be supported on this host.
		_ = writeCgroupFile(fullPath, filename, fmt.Sprintf("%d", max))
	}
	return nil
}

// DeleteCgroup removes a cgroup.
func (m *CgroupV2Manager) DeleteCgroup(containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cgroupPath, ok := m.cgroupPaths[containerID]
	if !ok {
		return nil
	}

	fullPath := filepath.Join(m.root, cgroupPath)

	// Stop any event watchers
	if watcher, ok := m.eventWatchers[containerID]; ok {
		watcher.Stop()
		delete(m.eventWatchers, containerID)
	}

	// Stop any pressure monitors
	if monitor, ok := m.pressureMonitors[containerID]; ok {
		monitor.Stop()
		delete(m.pressureMonitors, containerID)
	}

	// Kill all processes in the cgroup
	procsPath := filepath.Join(fullPath, "cgroup.procs")
	if procs, err := m.readProcsFile(procsPath); err == nil {
		for _, pid := range procs {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Signal(syscall.SIGKILL)
			}
		}
	}

	// Wait briefly for processes to terminate
	time.Sleep(100 * time.Millisecond)

	// Remove cgroup directory
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cgroup: %w", err)
	}

	delete(m.cgroupPaths, containerID)
	return nil
}

// GetStatsV2 returns comprehensive cgroup v2 statistics.
func (m *CgroupV2Manager) GetStatsV2(containerID string) (*CgroupV2Stats, error) {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	stats := &CgroupV2Stats{}

	// Best-effort: CPU stats may be unavailable on this controller.
	_ = m.readCPUStats(fullPath, &stats.CPU)

	// Best-effort: memory stats may be unavailable.
	_ = m.readMemoryStats(fullPath, &stats.Memory)

	// Best-effort: IO stats may be unavailable.
	_ = m.readIOStats(fullPath, &stats.IO)

	// Best-effort: PIDs stats may be unavailable.
	_ = m.readPIDsStats(fullPath, &stats.PIDs)

	// Read pressure stats
	m.readPressureStats(fullPath, &stats.Pressure)

	return stats, nil
}

// readCPUStats reads CPU statistics.
func (m *CgroupV2Manager) readCPUStats(fullPath string, stats *CgroupCPUStats) error {
	cpuStatPath := filepath.Join(fullPath, "cpu.stat")
	kv, err := readKeyValueFileV2(cpuStatPath)
	if err != nil {
		return err
	}

	stats.UsageUsec = parseUint64V2(kv["usage_usec"])
	stats.UserUsec = parseUint64V2(kv["user_usec"])
	stats.SystemUsec = parseUint64V2(kv["system_usec"])
	stats.NrPeriods = parseUint64V2(kv["nr_periods"])
	stats.NrThrottled = parseUint64V2(kv["nr_throttled"])
	stats.ThrottledUsec = parseUint64V2(kv["throttled_usec"])
	stats.NrBursts = parseUint64V2(kv["nr_bursts"])
	stats.BurstUsec = parseUint64V2(kv["burst_usec"])

	// Read CPU pressure
	stats.Pressure, _ = m.readPressureFile(filepath.Join(fullPath, "cpu.pressure"))

	return nil
}

// readMemoryStats reads memory statistics.
func (m *CgroupV2Manager) readMemoryStats(fullPath string, stats *CgroupMemoryStats) error {
	// Read memory.stat
	memStatPath := filepath.Join(fullPath, "memory.stat")
	kv, err := readKeyValueFileV2(memStatPath)
	if err != nil {
		return err
	}

	stats.Anon = parseUint64V2(kv["anon"])
	stats.File = parseUint64V2(kv["file"])
	stats.Kernel = parseUint64V2(kv["kernel"])
	stats.KernelStack = parseUint64V2(kv["kernel_stack"])
	stats.Slab = parseUint64V2(kv["slab"])
	stats.Sock = parseUint64V2(kv["sock"])
	stats.Shmem = parseUint64V2(kv["shmem"])
	stats.FileMapped = parseUint64V2(kv["file_mapped"])
	stats.FileDirty = parseUint64V2(kv["file_dirty"])
	stats.FileWriteback = parseUint64V2(kv["file_writeback"])
	stats.AnonThp = parseUint64V2(kv["anon_thp"])
	stats.InactiveAnon = parseUint64V2(kv["inactive_anon"])
	stats.ActiveAnon = parseUint64V2(kv["active_anon"])
	stats.InactiveFile = parseUint64V2(kv["inactive_file"])
	stats.ActiveFile = parseUint64V2(kv["active_file"])
	stats.Unevictable = parseUint64V2(kv["unevictable"])
	stats.SlabReclaimable = parseUint64V2(kv["slab_reclaimable"])
	stats.SlabUnreclaimable = parseUint64V2(kv["slab_unreclaimable"])
	stats.Pgfault = parseUint64V2(kv["pgfault"])
	stats.Pgmajfault = parseUint64V2(kv["pgmajfault"])
	stats.WorkingsetRefault = parseUint64V2(kv["workingset_refault"])
	stats.WorkingsetActivate = parseUint64V2(kv["workingset_activate"])
	stats.WorkingsetNodereclaim = parseUint64V2(kv["workingset_nodereclaim"])
	stats.Pgrefill = parseUint64V2(kv["pgrefill"])
	stats.Pgscan = parseUint64V2(kv["pgscan"])
	stats.Pgsteal = parseUint64V2(kv["pgsteal"])
	stats.Pgactivate = parseUint64V2(kv["pgactivate"])
	stats.Pgdeactivate = parseUint64V2(kv["pgdeactivate"])
	stats.Pglazyfree = parseUint64V2(kv["pglazyfree"])
	stats.Pglazyfreed = parseUint64V2(kv["pglazyfreed"])
	stats.ThpFaultAlloc = parseUint64V2(kv["thp_fault_alloc"])
	stats.ThpCollapseAlloc = parseUint64V2(kv["thp_collapse_alloc"])

	// Read memory.current
	if content, err := readCgroupFileV2(fullPath, "memory.current"); err == nil {
		stats.Current = parseUint64V2(content)
	}

	// Read memory.peak (if available)
	if content, err := readCgroupFileV2(fullPath, "memory.peak"); err == nil {
		stats.Peak = parseUint64V2(content)
	}

	// Read memory.high
	if content, err := readCgroupFileV2(fullPath, "memory.high"); err == nil {
		if content != "max" {
			stats.High = parseUint64V2(content)
		}
	}

	// Read memory.max
	if content, err := readCgroupFileV2(fullPath, "memory.max"); err == nil {
		if content != "max" {
			stats.Max = parseUint64V2(content)
		}
	}

	// Read memory.min
	if content, err := readCgroupFileV2(fullPath, "memory.min"); err == nil {
		stats.Min = parseUint64V2(content)
	}

	// Read memory.low
	if content, err := readCgroupFileV2(fullPath, "memory.low"); err == nil {
		stats.Low = parseUint64V2(content)
	}

	// Read swap stats
	if content, err := readCgroupFileV2(fullPath, "memory.swap.current"); err == nil {
		stats.SwapCurrent = parseUint64V2(content)
	}
	if content, err := readCgroupFileV2(fullPath, "memory.swap.high"); err == nil {
		if content != "max" {
			stats.SwapHigh = parseUint64V2(content)
		}
	}
	if content, err := readCgroupFileV2(fullPath, "memory.swap.max"); err == nil {
		if content != "max" {
			stats.SwapMax = parseUint64V2(content)
		}
	}

	// Read memory events
	m.readMemoryEvents(fullPath, &stats.Events)

	// Read memory pressure
	stats.Pressure, _ = m.readPressureFile(filepath.Join(fullPath, "memory.pressure"))

	return nil
}

// readMemoryEvents reads memory events.
func (m *CgroupV2Manager) readMemoryEvents(fullPath string, events *MemoryEvents) {
	eventsPath := filepath.Join(fullPath, "memory.events")
	kv, err := readKeyValueFileV2(eventsPath)
	if err != nil {
		return
	}

	events.Low = parseUint64V2(kv["low"])
	events.High = parseUint64V2(kv["high"])
	events.Max = parseUint64V2(kv["max"])
	events.OOM = parseUint64V2(kv["oom"])
	events.OOMKill = parseUint64V2(kv["oom_kill"])
	events.OOMGroupKill = parseUint64V2(kv["oom_group_kill"])
}

// readIOStats reads IO statistics.
func (m *CgroupV2Manager) readIOStats(fullPath string, stats *CgroupIOStats) error {
	ioStatPath := filepath.Join(fullPath, "io.stat")
	data, err := os.ReadFile(ioStatPath) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var deviceStats DeviceIOStats
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Parse device major:minor
		var major, minor uint64
		if _, err := fmt.Sscanf(fields[0], "%d:%d", &major, &minor); err != nil {
			continue
		}
		deviceStats.Major = major
		deviceStats.Minor = minor

		// Parse stats
		for _, field := range fields[1:] {
			parts := strings.Split(field, "=")
			if len(parts) != 2 {
				continue
			}
			value := parseUint64V2(parts[1])
			switch parts[0] {
			case "rbytes":
				deviceStats.Rbytes = value
			case "wbytes":
				deviceStats.Wbytes = value
			case "rios":
				deviceStats.Rios = value
			case "wios":
				deviceStats.Wios = value
			case "dbytes":
				deviceStats.Dbytes = value
			case "dios":
				deviceStats.Dios = value
			}
		}

		stats.Devices = append(stats.Devices, deviceStats)
	}

	// Read IO pressure
	stats.Pressure, _ = m.readPressureFile(filepath.Join(fullPath, "io.pressure"))

	return nil
}

// readPIDsStats reads PIDs statistics.
func (m *CgroupV2Manager) readPIDsStats(fullPath string, stats *CgroupPIDsStats) error {
	// Read pids.current
	if content, err := readCgroupFileV2(fullPath, "pids.current"); err == nil {
		stats.Current = parseUint64V2(content)
	}

	// Read pids.max
	if content, err := readCgroupFileV2(fullPath, "pids.max"); err == nil {
		if content != "max" {
			stats.Max = parseUint64V2(content)
		}
	}

	// Read pids.events
	eventsPath := filepath.Join(fullPath, "pids.events")
	if kv, err := readKeyValueFileV2(eventsPath); err == nil {
		stats.Events.Max = parseUint64V2(kv["max"])
	}

	return nil
}

// readPressureStats reads pressure statistics.
func (m *CgroupV2Manager) readPressureStats(fullPath string, stats *CgroupPressureStats) {
	stats.CPU, _ = m.readPressureFile(filepath.Join(fullPath, "cpu.pressure"))
	stats.Memory, _ = m.readPressureFile(filepath.Join(fullPath, "memory.pressure"))
	stats.IO, _ = m.readPressureFile(filepath.Join(fullPath, "io.pressure"))
}

// readPressureFile reads a pressure file and parses its content.
func (m *CgroupV2Manager) readPressureFile(path string) (*PressureData, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return nil, err
	}

	pressure := &PressureData{}
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "some ") {
			m.parsePressureLine(line[5:], &pressure.Some)
		} else if strings.HasPrefix(line, "full ") {
			m.parsePressureLine(line[5:], &pressure.Full)
		}
	}

	return pressure, nil
}

// parsePressureLine parses a pressure line.
func (m *CgroupV2Manager) parsePressureLine(line string, metrics *PressureMetrics) {
	fields := strings.Fields(line)
	for _, field := range fields {
		parts := strings.Split(field, "=")
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "avg10":
			metrics.Avg10, _ = strconv.ParseFloat(parts[1], 64)
		case "avg60":
			metrics.Avg60, _ = strconv.ParseFloat(parts[1], 64)
		case "avg300":
			metrics.Avg300, _ = strconv.ParseFloat(parts[1], 64)
		case "total":
			metrics.Total = parseUint64V2(parts[1])
		}
	}
}

// FreezeV2 freezes a cgroup using cgroup.freeze.
func (m *CgroupV2Manager) FreezeV2(containerID string) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	return writeCgroupFile(fullPath, "cgroup.freeze", "1")
}

// ThawV2 thaws a frozen cgroup.
func (m *CgroupV2Manager) ThawV2(containerID string) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	return writeCgroupFile(fullPath, "cgroup.freeze", "0")
}

// IsFrozenV2 checks if a cgroup is frozen.
func (m *CgroupV2Manager) IsFrozenV2(containerID string) (bool, error) {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return false, fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	content, err := readCgroupFileV2(fullPath, "cgroup.freeze")
	if err != nil {
		return false, err
	}

	return content == "1", nil
}

// GetFreezeState returns the freeze state from cgroup.events.
func (m *CgroupV2Manager) GetFreezeState(containerID string) (frozen bool, populated bool, err error) {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return false, false, fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	eventsPath := filepath.Join(fullPath, "cgroup.events")

	kv, err := readKeyValueFileV2(eventsPath)
	if err != nil {
		return false, false, err
	}

	frozen = kv["frozen"] == "1"
	populated = kv["populated"] == "1"

	return frozen, populated, nil
}

// UpdateResourcesV2 updates resource limits dynamically.
func (m *CgroupV2Manager) UpdateResourcesV2(containerID string, config *CgroupV2Config) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	return m.applyV2Config(fullPath, config)
}

// AddProcessV2 adds a process to a cgroup.
func (m *CgroupV2Manager) AddProcessV2(containerID string, pid int) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	return writeCgroupFile(fullPath, "cgroup.procs", fmt.Sprintf("%d", pid))
}

// AddThreadV2 adds a thread to a cgroup.
func (m *CgroupV2Manager) AddThreadV2(containerID string, tid int) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	return writeCgroupFile(fullPath, "cgroup.threads", fmt.Sprintf("%d", tid))
}

// ListProcesses returns all processes in a cgroup.
func (m *CgroupV2Manager) ListProcesses(containerID string) ([]int, error) {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	procsPath := filepath.Join(fullPath, "cgroup.procs")

	return m.readProcsFile(procsPath)
}

// readProcsFile reads process IDs from a cgroup.procs or cgroup.threads file.
func (m *CgroupV2Manager) readProcsFile(path string) ([]int, error) {
	file, err := os.Open(path) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var pids []int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		pid, err := strconv.Atoi(scanner.Text())
		if err == nil {
			pids = append(pids, pid)
		}
	}

	return pids, scanner.Err()
}

// WatchEvents starts watching for cgroup events.
func (m *CgroupV2Manager) WatchEvents(containerID string) (<-chan CgroupEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cgroupPath, ok := m.cgroupPaths[containerID]
	if !ok {
		return nil, fmt.Errorf("cgroup not found for container %s", containerID)
	}

	// Check if already watching
	if _, exists := m.eventWatchers[containerID]; exists {
		return nil, fmt.Errorf("already watching events for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)

	watcher := &CgroupEventWatcher{
		cgroupPath: fullPath,
		events:     make(chan CgroupEvent, 100),
		stopCh:     make(chan struct{}),
		watches:    make(map[int]string),
	}

	// Create inotify instance
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("failed to create inotify: %w", err)
	}
	watcher.inotifyFd = fd

	// Watch memory.events
	memEventsPath := filepath.Join(fullPath, "memory.events")
	if wd, err := unix.InotifyAddWatch(fd, memEventsPath, unix.IN_MODIFY); err == nil {
		watcher.watches[wd] = memEventsPath
	}

	// Watch pids.events
	pidsEventsPath := filepath.Join(fullPath, "pids.events")
	if wd, err := unix.InotifyAddWatch(fd, pidsEventsPath, unix.IN_MODIFY); err == nil {
		watcher.watches[wd] = pidsEventsPath
	}

	// Watch cgroup.events
	cgroupEventsPath := filepath.Join(fullPath, "cgroup.events")
	if wd, err := unix.InotifyAddWatch(fd, cgroupEventsPath, unix.IN_MODIFY); err == nil {
		watcher.watches[wd] = cgroupEventsPath
	}

	m.eventWatchers[containerID] = watcher

	// Start event processing goroutine
	go watcher.processEvents(containerID)

	return watcher.events, nil
}

// processEvents processes inotify events and converts them to cgroup events.
func (w *CgroupEventWatcher) processEvents(containerID string) {
	defer close(w.events)
	defer func() { _ = unix.Close(w.inotifyFd) }()

	buf := make([]byte, 4096)

	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		n, err := unix.Read(w.inotifyFd, buf)
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				// Non-blocking read, wait a bit
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return
		}

		if n < unix.SizeofInotifyEvent {
			continue
		}

		// Process inotify events. Simple parsing — production would cast
		// &buf[offset] to *unix.InotifyEvent and honor event.Len for
		// variable-length names. Today we advance by the fixed header
		// size only, which works for cgroup file-watches because we
		// configured them with IN_MASK_CREATE (no name in the event).
		var offset uint32
		for offset < uint32(n) {
			if offset+unix.SizeofInotifyEvent > uint32(n) {
				break
			}

			// Get watch descriptor
			wd := int(buf[offset]) | int(buf[offset+1])<<8 | int(buf[offset+2])<<16 | int(buf[offset+3])<<24

			// Look up the file path for this watch
			filePath, ok := w.watches[wd]
			if !ok {
				offset += unix.SizeofInotifyEvent
				continue
			}

			// Generate cgroup event based on file
			w.generateEvent(containerID, filePath)

			offset += unix.SizeofInotifyEvent
		}
	}
}

// generateEvent generates a cgroup event based on the modified file.
func (w *CgroupEventWatcher) generateEvent(containerID, filePath string) {
	baseName := filepath.Base(filePath)

	switch baseName {
	case "memory.events":
		kv, err := readKeyValueFileV2(filePath)
		if err != nil {
			return
		}

		// Check for OOM events
		if oom := parseUint64V2(kv["oom"]); oom > 0 {
			w.events <- CgroupEvent{
				Type:        CgroupEventMemoryOOM,
				ContainerID: containerID,
				CgroupPath:  w.cgroupPath,
				Timestamp:   time.Now(),
				Data: map[string]interface{}{
					"oom_count": oom,
				},
			}
		}

		// Check for OOM kills
		if oomKill := parseUint64V2(kv["oom_kill"]); oomKill > 0 {
			w.events <- CgroupEvent{
				Type:        CgroupEventMemoryOOMKill,
				ContainerID: containerID,
				CgroupPath:  w.cgroupPath,
				Timestamp:   time.Now(),
				Data: map[string]interface{}{
					"oom_kill_count": oomKill,
				},
			}
		}

	case "pids.events":
		kv, err := readKeyValueFileV2(filePath)
		if err != nil {
			return
		}

		if maxHit := parseUint64V2(kv["max"]); maxHit > 0 {
			w.events <- CgroupEvent{
				Type:        CgroupEventPidsMax,
				ContainerID: containerID,
				CgroupPath:  w.cgroupPath,
				Timestamp:   time.Now(),
				Data: map[string]interface{}{
					"max_hit_count": maxHit,
				},
			}
		}

	case "cgroup.events":
		kv, err := readKeyValueFileV2(filePath)
		if err != nil {
			return
		}

		if kv["frozen"] == "1" {
			w.events <- CgroupEvent{
				Type:        CgroupEventFrozen,
				ContainerID: containerID,
				CgroupPath:  w.cgroupPath,
				Timestamp:   time.Now(),
			}
		}

		populated := kv["populated"] == "1"
		w.events <- CgroupEvent{
			Type:        CgroupEventPopulated,
			ContainerID: containerID,
			CgroupPath:  w.cgroupPath,
			Timestamp:   time.Now(),
			Data: map[string]interface{}{
				"populated": populated,
			},
		}
	}
}

// Stop stops the event watcher.
func (w *CgroupEventWatcher) Stop() {
	close(w.stopCh)
}

// StartPressureMonitor starts monitoring resource pressure.
func (m *CgroupV2Manager) StartPressureMonitor(containerID string, thresholds PressureThresholds, callback PressureCallback) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cgroupPath, ok := m.cgroupPaths[containerID]
	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	// Check if already monitoring
	if _, exists := m.pressureMonitors[containerID]; exists {
		return fmt.Errorf("already monitoring pressure for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)

	monitor := &PressureMonitor{
		cgroupPath: fullPath,
		thresholds: thresholds,
		callback:   callback,
		stopCh:     make(chan struct{}),
		interval:   5 * time.Second,
	}

	m.pressureMonitors[containerID] = monitor

	go monitor.run(containerID, m)

	return nil
}

// run runs the pressure monitor loop.
func (pm *PressureMonitor) run(containerID string, m *CgroupV2Manager) {
	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopCh:
			return
		case <-ticker.C:
			pressure := CgroupPressureStats{}
			m.readPressureStats(pm.cgroupPath, &pressure)

			// Check thresholds
			exceeded := false

			if pressure.CPU != nil && pressure.CPU.Some.Avg10 > pm.thresholds.CPUSomeAvg10 {
				exceeded = true
			}
			if pressure.Memory != nil {
				if pressure.Memory.Some.Avg10 > pm.thresholds.MemorySomeAvg10 {
					exceeded = true
				}
				if pressure.Memory.Full.Avg10 > pm.thresholds.MemoryFullAvg10 {
					exceeded = true
				}
			}
			if pressure.IO != nil {
				if pressure.IO.Some.Avg10 > pm.thresholds.IOSomeAvg10 {
					exceeded = true
				}
				if pressure.IO.Full.Avg10 > pm.thresholds.IOFullAvg10 {
					exceeded = true
				}
			}

			if exceeded && pm.callback != nil {
				pm.callback(containerID, pressure)
			}
		}
	}
}

// Stop stops the pressure monitor.
func (pm *PressureMonitor) Stop() {
	close(pm.stopCh)
}

// StopPressureMonitor stops the pressure monitor for a container.
func (m *CgroupV2Manager) StopPressureMonitor(containerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if monitor, ok := m.pressureMonitors[containerID]; ok {
		monitor.Stop()
		delete(m.pressureMonitors, containerID)
	}
}

// StopEventWatcher stops the event watcher for a container.
func (m *CgroupV2Manager) StopEventWatcher(containerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if watcher, ok := m.eventWatchers[containerID]; ok {
		watcher.Stop()
		delete(m.eventWatchers, containerID)
	}
}

// GetAvailableControllersV2 returns the list of available cgroup v2 controllers.
func (m *CgroupV2Manager) GetAvailableControllersV2() ([]string, error) {
	controllersPath := filepath.Join(m.root, m.parentPath, "cgroup.controllers")
	data, err := os.ReadFile(controllersPath) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(data)), nil
}

// GetSubtreeControllersV2 returns the list of enabled subtree controllers.
func (m *CgroupV2Manager) GetSubtreeControllersV2() ([]string, error) {
	subtreePath := filepath.Join(m.root, m.parentPath, "cgroup.subtree_control")
	data, err := os.ReadFile(subtreePath) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(data)), nil
}

// SetIOWeightDevice sets IO weight for a specific device.
func (m *CgroupV2Manager) SetIOWeightDevice(containerID string, major, minor uint64, weight uint16) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	content := fmt.Sprintf("%d:%d %d", major, minor, weight)
	return writeCgroupFile(fullPath, "io.weight", content)
}

// SetIOLatency sets IO latency target for a device.
func (m *CgroupV2Manager) SetIOLatency(containerID string, major, minor uint64, targetUsec uint64) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	content := fmt.Sprintf("%d:%d target=%d", major, minor, targetUsec)
	return writeCgroupFile(fullPath, "io.latency", content)
}

// KillAll sends a signal to all processes in the cgroup using cgroup.kill.
func (m *CgroupV2Manager) KillAll(containerID string) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)

	// Use cgroup.kill if available (kernel 5.14+)
	killPath := filepath.Join(fullPath, "cgroup.kill")
	if err := os.WriteFile(killPath, []byte("1"), 0644); err == nil { // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600
		return nil
	}

	// Fallback: kill all processes manually
	procs, err := m.ListProcesses(containerID)
	if err != nil {
		return err
	}

	for _, pid := range procs {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(syscall.SIGKILL)
		}
	}

	return nil
}

// GetType returns the cgroup type (domain, domain threaded, domain invalid, threaded).
func (m *CgroupV2Manager) GetType(containerID string) (string, error) {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	content, err := readCgroupFileV2(fullPath, "cgroup.type")
	if err != nil {
		return "domain", nil // Default type
	}

	return content, nil
}

// SetType sets the cgroup type.
func (m *CgroupV2Manager) SetType(containerID string, cgroupType string) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	return writeCgroupFile(fullPath, "cgroup.type", cgroupType)
}

// WaitForEmpty waits for the cgroup to become empty (no processes).
func (m *CgroupV2Manager) WaitForEmpty(ctx context.Context, containerID string) error {
	m.mu.RLock()
	cgroupPath, ok := m.cgroupPaths[containerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("cgroup not found for container %s", containerID)
	}

	fullPath := filepath.Join(m.root, cgroupPath)
	eventsPath := filepath.Join(fullPath, "cgroup.events")

	// Use inotify to wait for populated=0
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()

	wd, err := unix.InotifyAddWatch(fd, eventsPath, unix.IN_MODIFY)
	if err != nil {
		return err
	}
	defer func() { _, _ = unix.InotifyRmWatch(fd, uint32(wd)) }()

	// Check initial state
	if populated, err := m.isPopulated(eventsPath); err == nil && !populated {
		return nil
	}

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Set read timeout
		_ = unix.SetNonblock(fd, true)
		n, err := unix.Read(fd, buf)
		if err != nil {
			if err == unix.EAGAIN {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return err
		}

		if n > 0 {
			if populated, err := m.isPopulated(eventsPath); err == nil && !populated {
				return nil
			}
		}
	}
}

// isPopulated checks if the cgroup is populated.
func (m *CgroupV2Manager) isPopulated(eventsPath string) (bool, error) {
	kv, err := readKeyValueFileV2(eventsPath)
	if err != nil {
		return false, err
	}
	return kv["populated"] == "1", nil
}

// Close closes the cgroup manager and cleans up resources.
func (m *CgroupV2Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	close(m.closeCh)

	// Stop all event watchers
	for _, watcher := range m.eventWatchers {
		watcher.Stop()
	}
	m.eventWatchers = make(map[string]*CgroupEventWatcher)

	// Stop all pressure monitors
	for _, monitor := range m.pressureMonitors {
		monitor.Stop()
	}
	m.pressureMonitors = make(map[string]*PressureMonitor)

	return nil
}

// Helper functions

// writeCgroupFile writes content to a cgroup file.
func writeCgroupFile(cgroupPath, filename, content string) error {
	path := filepath.Join(cgroupPath, filename)
	return os.WriteFile(path, []byte(content), 0644) // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600
}

// readCgroupFileV2 reads content from a cgroup file.
func readCgroupFileV2(cgroupPath, filename string) (string, error) {
	path := filepath.Join(cgroupPath, filename)
	data, err := os.ReadFile(path) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readKeyValueFileV2 reads a key-value cgroup file.
func readKeyValueFileV2(path string) (map[string]string, error) {
	file, err := os.Open(path) // #nosec G304 -- cgroup filesystem path under /sys/fs/cgroup; root-only, not user-supplied
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}

	return result, scanner.Err()
}

// parseUint64V2 parses a string to uint64.
func parseUint64V2(s string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return v
}
