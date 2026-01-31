//go:build linux

package runtime

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// CgroupManager manages cgroups v2 resources.
type CgroupManager struct {
	mu sync.RWMutex

	driver       string
	root         string
	parentPath   string
	cgroupPaths  map[string]string
}

// CgroupStats contains cgroup statistics.
type CgroupStats struct {
	// CPU statistics
	CPUUsage           uint64
	CPUUserUsage       uint64
	CPUSystemUsage     uint64
	CPUThrottledPeriods uint64
	CPUThrottledTime    uint64

	// Memory statistics
	MemoryUsage      uint64
	MemoryMaxUsage   uint64
	MemoryLimit      uint64
	MemoryCache      uint64
	MemoryRSS        uint64
	MemorySwap       uint64
	MemoryKernelUsage uint64

	// IO statistics
	IOReadBytes  uint64
	IOWriteBytes uint64
	IOReadOps    uint64
	IOWriteOps   uint64

	// PIDs statistics
	PIDsCurrent uint64
	PIDsLimit   uint64
}

// cgroupV2Controllers contains the available cgroup v2 controllers.
var cgroupV2Controllers = []string{
	"cpu",
	"cpuset",
	"io",
	"memory",
	"pids",
}

// NewCgroupManager creates a new cgroup manager.
func NewCgroupManager(driver, parentPath string) (*CgroupManager, error) {
	// Detect cgroup v2 mount point
	root := "/sys/fs/cgroup"

	// Verify cgroup v2 is available
	if !isCgroupV2(root) {
		return nil, fmt.Errorf("cgroup v2 not available at %s", root)
	}

	if parentPath == "" {
		parentPath = "/unheaded"
	}

	mgr := &CgroupManager{
		driver:      driver,
		root:        root,
		parentPath:  parentPath,
		cgroupPaths: make(map[string]string),
	}

	// Create parent cgroup
	parentFullPath := filepath.Join(root, parentPath)
	if err := os.MkdirAll(parentFullPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent cgroup: %w", err)
	}

	// Enable controllers in parent
	if err := mgr.enableControllers(parentFullPath); err != nil {
		// Log but don't fail - some controllers may not be available
	}

	return mgr, nil
}

// isCgroupV2 checks if the given path is a cgroup v2 mount.
func isCgroupV2(path string) bool {
	// Check for cgroup.controllers file (only in cgroup v2)
	_, err := os.Stat(filepath.Join(path, "cgroup.controllers"))
	return err == nil
}

// enableControllers enables cgroup controllers.
func (m *CgroupManager) enableControllers(path string) error {
	// Read available controllers
	controllersPath := filepath.Join(path, "cgroup.controllers")
	data, err := os.ReadFile(controllersPath)
	if err != nil {
		return err
	}

	available := strings.Fields(string(data))
	availableSet := make(map[string]bool)
	for _, c := range available {
		availableSet[c] = true
	}

	// Enable available controllers in subtree_control
	var toEnable []string
	for _, c := range cgroupV2Controllers {
		if availableSet[c] {
			toEnable = append(toEnable, "+"+c)
		}
	}

	if len(toEnable) == 0 {
		return nil
	}

	subtreeControl := filepath.Join(path, "cgroup.subtree_control")
	return os.WriteFile(subtreeControl, []byte(strings.Join(toEnable, " ")), 0644)
}

// CreateCgroup creates a cgroup for a container.
func (m *CgroupManager) CreateCgroup(containerID string, resources *ResourceConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build cgroup path
	cgroupPath := filepath.Join(m.parentPath, containerID)
	fullPath := filepath.Join(m.root, cgroupPath)

	// Create cgroup directory
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create cgroup: %w", err)
	}

	// Apply resource limits
	if err := m.applyResources(fullPath, resources); err != nil {
		os.RemoveAll(fullPath)
		return "", fmt.Errorf("failed to apply resources: %w", err)
	}

	m.cgroupPaths[containerID] = cgroupPath

	return cgroupPath, nil
}

// applyResources applies resource limits to a cgroup.
func (m *CgroupManager) applyResources(cgroupPath string, resources *ResourceConfig) error {
	if resources == nil {
		return nil
	}

	// Apply CPU limits
	if err := m.applyCPUResources(cgroupPath, resources); err != nil {
		return err
	}

	// Apply memory limits
	if err := m.applyMemoryResources(cgroupPath, resources); err != nil {
		return err
	}

	// Apply PIDs limits
	if err := m.applyPidsResources(cgroupPath, resources); err != nil {
		return err
	}

	return nil
}

// applyCPUResources applies CPU resource limits.
func (m *CgroupManager) applyCPUResources(cgroupPath string, resources *ResourceConfig) error {
	// CPU weight (shares in cgroup v1)
	if resources.CPUShares > 0 {
		// Convert shares to weight: weight = 1 + ((shares - 2) * 9999) / 262142
		// Simplified: weight ~= shares * 100 / 1024
		weight := (resources.CPUShares * 100) / 1024
		if weight < 1 {
			weight = 1
		}
		if weight > 10000 {
			weight = 10000
		}
		if err := writeFile(cgroupPath, "cpu.weight", fmt.Sprintf("%d", weight)); err != nil {
			return err
		}
	}

	// CPU quota/period (cpu.max)
	if resources.CPUQuota > 0 || resources.CPUPeriod > 0 {
		quota := resources.CPUQuota
		period := resources.CPUPeriod
		if period == 0 {
			period = 100000 // 100ms default
		}
		if quota <= 0 {
			quota = -1 // Max
		}

		var content string
		if quota < 0 {
			content = fmt.Sprintf("max %d", period)
		} else {
			content = fmt.Sprintf("%d %d", quota, period)
		}
		if err := writeFile(cgroupPath, "cpu.max", content); err != nil {
			return err
		}
	}

	// CPUSet
	if resources.CPUSetCPUs != "" {
		if err := writeFile(cgroupPath, "cpuset.cpus", resources.CPUSetCPUs); err != nil {
			return err
		}
	}

	if resources.CPUSetMems != "" {
		if err := writeFile(cgroupPath, "cpuset.mems", resources.CPUSetMems); err != nil {
			return err
		}
	}

	return nil
}

// applyMemoryResources applies memory resource limits.
func (m *CgroupManager) applyMemoryResources(cgroupPath string, resources *ResourceConfig) error {
	// Memory limit (memory.max)
	if resources.MemoryLimitBytes > 0 {
		if err := writeFile(cgroupPath, "memory.max", fmt.Sprintf("%d", resources.MemoryLimitBytes)); err != nil {
			return err
		}
	}

	// Memory reservation (memory.low)
	if resources.MemoryReservationBytes > 0 {
		if err := writeFile(cgroupPath, "memory.low", fmt.Sprintf("%d", resources.MemoryReservationBytes)); err != nil {
			return err
		}
	}

	// Memory swap limit (memory.swap.max)
	if resources.MemorySwapLimitBytes > 0 {
		swapLimit := resources.MemorySwapLimitBytes
		if resources.MemoryLimitBytes > 0 {
			// In cgroup v2, swap.max is for swap only, not memory+swap
			swapLimit = resources.MemorySwapLimitBytes - resources.MemoryLimitBytes
			if swapLimit < 0 {
				swapLimit = 0
			}
		}
		if err := writeFile(cgroupPath, "memory.swap.max", fmt.Sprintf("%d", swapLimit)); err != nil {
			// Swap might not be enabled
		}
	}

	// OOM behavior
	if resources.OOMKillDisable {
		// In cgroup v2, we can use memory.oom.group
		// Note: This requires additional setup
	}

	return nil
}

// applyPidsResources applies PIDs resource limits.
func (m *CgroupManager) applyPidsResources(cgroupPath string, resources *ResourceConfig) error {
	if resources.PidsLimit > 0 {
		if err := writeFile(cgroupPath, "pids.max", fmt.Sprintf("%d", resources.PidsLimit)); err != nil {
			return err
		}
	}
	return nil
}

// AddProcess adds a process to a cgroup.
func (m *CgroupManager) AddProcess(cgroupPath string, pid int) error {
	fullPath := filepath.Join(m.root, cgroupPath)
	return writeFile(fullPath, "cgroup.procs", fmt.Sprintf("%d", pid))
}

// RemoveCgroup removes a cgroup.
func (m *CgroupManager) RemoveCgroup(cgroupPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fullPath := filepath.Join(m.root, cgroupPath)

	// First, kill all processes in the cgroup
	procsPath := filepath.Join(fullPath, "cgroup.procs")
	procs, err := readProcs(procsPath)
	if err == nil {
		for _, pid := range procs {
			// Send SIGKILL
			proc, err := os.FindProcess(pid)
			if err == nil {
				proc.Kill()
			}
		}
	}

	// Wait a bit for processes to die
	// In production, would use cgroup.events to wait

	// Remove cgroup directory
	if err := os.Remove(fullPath); err != nil {
		// Might fail if not empty, try again after killing all
		return err
	}

	// Remove from tracking
	for id, path := range m.cgroupPaths {
		if path == cgroupPath {
			delete(m.cgroupPaths, id)
			break
		}
	}

	return nil
}

// GetStats returns cgroup statistics.
func (m *CgroupManager) GetStats(cgroupPath string) (*CgroupStats, error) {
	fullPath := filepath.Join(m.root, cgroupPath)
	stats := &CgroupStats{}

	// Read CPU stats
	if cpuStat, err := readKeyValueFile(filepath.Join(fullPath, "cpu.stat")); err == nil {
		stats.CPUUsage = parseUint64(cpuStat["usage_usec"]) * 1000 // Convert to nanoseconds
		stats.CPUUserUsage = parseUint64(cpuStat["user_usec"]) * 1000
		stats.CPUSystemUsage = parseUint64(cpuStat["system_usec"]) * 1000
		stats.CPUThrottledPeriods = parseUint64(cpuStat["nr_throttled"])
		stats.CPUThrottledTime = parseUint64(cpuStat["throttled_usec"]) * 1000
	}

	// Read memory stats
	if memoryStat, err := readKeyValueFile(filepath.Join(fullPath, "memory.stat")); err == nil {
		stats.MemoryCache = parseUint64(memoryStat["file"])
		stats.MemoryRSS = parseUint64(memoryStat["anon"])
		stats.MemoryKernelUsage = parseUint64(memoryStat["kernel"])
	}

	if content, err := readFile(fullPath, "memory.current"); err == nil {
		stats.MemoryUsage = parseUint64(content)
	}

	if content, err := readFile(fullPath, "memory.max"); err == nil {
		if content != "max" {
			stats.MemoryLimit = parseUint64(content)
		}
	}

	if content, err := readFile(fullPath, "memory.peak"); err == nil {
		stats.MemoryMaxUsage = parseUint64(content)
	}

	if content, err := readFile(fullPath, "memory.swap.current"); err == nil {
		stats.MemorySwap = parseUint64(content)
	}

	// Read IO stats
	if ioStat, err := os.ReadFile(filepath.Join(fullPath, "io.stat")); err == nil {
		lines := strings.Split(string(ioStat), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			for _, field := range fields[1:] { // Skip device ID
				parts := strings.Split(field, "=")
				if len(parts) != 2 {
					continue
				}
				value := parseUint64(parts[1])
				switch parts[0] {
				case "rbytes":
					stats.IOReadBytes += value
				case "wbytes":
					stats.IOWriteBytes += value
				case "rios":
					stats.IOReadOps += value
				case "wios":
					stats.IOWriteOps += value
				}
			}
		}
	}

	// Read PIDs stats
	if content, err := readFile(fullPath, "pids.current"); err == nil {
		stats.PIDsCurrent = parseUint64(content)
	}

	if content, err := readFile(fullPath, "pids.max"); err == nil {
		if content != "max" {
			stats.PIDsLimit = parseUint64(content)
		}
	}

	return stats, nil
}

// Freeze freezes a cgroup.
func (m *CgroupManager) Freeze(cgroupPath string) error {
	fullPath := filepath.Join(m.root, cgroupPath)
	return writeFile(fullPath, "cgroup.freeze", "1")
}

// Thaw thaws a frozen cgroup.
func (m *CgroupManager) Thaw(cgroupPath string) error {
	fullPath := filepath.Join(m.root, cgroupPath)
	return writeFile(fullPath, "cgroup.freeze", "0")
}

// IsFrozen checks if a cgroup is frozen.
func (m *CgroupManager) IsFrozen(cgroupPath string) (bool, error) {
	fullPath := filepath.Join(m.root, cgroupPath)
	content, err := readFile(fullPath, "cgroup.freeze")
	if err != nil {
		return false, err
	}
	return content == "1", nil
}

// UpdateResources updates resource limits for a cgroup.
func (m *CgroupManager) UpdateResources(cgroupPath string, resources *ResourceConfig) error {
	fullPath := filepath.Join(m.root, cgroupPath)
	return m.applyResources(fullPath, resources)
}

// GetCgroupPath returns the cgroup path for a container.
func (m *CgroupManager) GetCgroupPath(containerID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path, ok := m.cgroupPaths[containerID]
	if !ok {
		return "", fmt.Errorf("cgroup not found for container %s", containerID)
	}
	return path, nil
}

// ListProcesses lists all processes in a cgroup.
func (m *CgroupManager) ListProcesses(cgroupPath string) ([]int, error) {
	fullPath := filepath.Join(m.root, cgroupPath)
	procsPath := filepath.Join(fullPath, "cgroup.procs")
	return readProcs(procsPath)
}

// Helper functions

// writeFile writes content to a cgroup file.
func writeFile(cgroupPath, filename, content string) error {
	path := filepath.Join(cgroupPath, filename)
	return os.WriteFile(path, []byte(content), 0644)
}

// readFile reads content from a cgroup file.
func readFile(cgroupPath, filename string) (string, error) {
	path := filepath.Join(cgroupPath, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readKeyValueFile reads a key-value cgroup file.
func readKeyValueFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

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

// readProcs reads process IDs from a cgroup.procs file.
func readProcs(path string) ([]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

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

// parseUint64 parses a string to uint64.
func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return v
}

// CgroupController represents a cgroup controller type.
type CgroupController string

const (
	CgroupControllerCPU    CgroupController = "cpu"
	CgroupControllerCPUSet CgroupController = "cpuset"
	CgroupControllerIO     CgroupController = "io"
	CgroupControllerMemory CgroupController = "memory"
	CgroupControllerPids   CgroupController = "pids"
)

// IsControllerAvailable checks if a controller is available.
func (m *CgroupManager) IsControllerAvailable(controller CgroupController) bool {
	controllersPath := filepath.Join(m.root, m.parentPath, "cgroup.controllers")
	data, err := os.ReadFile(controllersPath)
	if err != nil {
		return false
	}

	controllers := strings.Fields(string(data))
	for _, c := range controllers {
		if c == string(controller) {
			return true
		}
	}
	return false
}

// GetAvailableControllers returns the list of available controllers.
func (m *CgroupManager) GetAvailableControllers() []CgroupController {
	controllersPath := filepath.Join(m.root, m.parentPath, "cgroup.controllers")
	data, err := os.ReadFile(controllersPath)
	if err != nil {
		return nil
	}

	var result []CgroupController
	for _, c := range strings.Fields(string(data)) {
		result = append(result, CgroupController(c))
	}
	return result
}

// SetIOLimit sets IO limits for a cgroup.
func (m *CgroupManager) SetIOLimit(cgroupPath string, deviceMajor, deviceMinor int, rbps, wbps, riops, wiops int64) error {
	fullPath := filepath.Join(m.root, cgroupPath)

	// Format: "MAJ:MIN rbps=X wbps=Y riops=Z wiops=W"
	var parts []string
	if rbps > 0 {
		parts = append(parts, fmt.Sprintf("rbps=%d", rbps))
	}
	if wbps > 0 {
		parts = append(parts, fmt.Sprintf("wbps=%d", wbps))
	}
	if riops > 0 {
		parts = append(parts, fmt.Sprintf("riops=%d", riops))
	}
	if wiops > 0 {
		parts = append(parts, fmt.Sprintf("wiops=%d", wiops))
	}

	if len(parts) == 0 {
		return nil
	}

	content := fmt.Sprintf("%d:%d %s", deviceMajor, deviceMinor, strings.Join(parts, " "))
	return writeFile(fullPath, "io.max", content)
}

// SetMemoryEvents enables memory events monitoring.
func (m *CgroupManager) SetMemoryEvents(cgroupPath string) error {
	// Memory events are automatically available in cgroup v2
	// We can read them from memory.events
	return nil
}

// GetMemoryEvents returns memory events for a cgroup.
func (m *CgroupManager) GetMemoryEvents(cgroupPath string) (map[string]uint64, error) {
	fullPath := filepath.Join(m.root, cgroupPath)
	eventsPath := filepath.Join(fullPath, "memory.events")

	kv, err := readKeyValueFile(eventsPath)
	if err != nil {
		return nil, err
	}

	result := make(map[string]uint64)
	for k, v := range kv {
		result[k] = parseUint64(v)
	}

	return result, nil
}
