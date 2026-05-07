// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package runtime

import (
	"fmt"
	"sync"
)

// CgroupManager manages cgroups (stub for non-Linux).
type CgroupManager struct {
	mu          sync.RWMutex
	cgroupPaths map[string]string
	v2Manager   *CgroupV2Manager
}

// CgroupStats contains cgroup statistics.
type CgroupStats struct {
	CPUUsage            uint64
	CPUUserUsage        uint64
	CPUSystemUsage      uint64
	CPUThrottledPeriods uint64
	CPUThrottledTime    uint64
	MemoryUsage         uint64
	MemoryMaxUsage      uint64
	MemoryLimit         uint64
	MemoryCache         uint64
	MemoryRSS           uint64
	MemorySwap          uint64
	MemoryKernelUsage   uint64
	IOReadBytes         uint64
	IOWriteBytes        uint64
	IOReadOps           uint64
	IOWriteOps          uint64
	PIDsCurrent         uint64
	PIDsLimit           uint64
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

// NewCgroupManager creates a new cgroup manager (stub for non-Linux).
func NewCgroupManager(driver, parentPath string) (*CgroupManager, error) {
	return &CgroupManager{
		cgroupPaths: make(map[string]string),
	}, nil
}

// CreateCgroup creates a cgroup (stub for non-Linux).
func (m *CgroupManager) CreateCgroup(containerID string, resources *ResourceConfig) (string, error) {
	return "", fmt.Errorf("cgroups require Linux")
}

// AddProcess adds a process to a cgroup (stub for non-Linux).
func (m *CgroupManager) AddProcess(cgroupPath string, pid int) error {
	return fmt.Errorf("cgroups require Linux")
}

// RemoveCgroup removes a cgroup (stub for non-Linux).
func (m *CgroupManager) RemoveCgroup(cgroupPath string) error {
	return fmt.Errorf("cgroups require Linux")
}

// GetStats returns cgroup statistics (stub for non-Linux).
func (m *CgroupManager) GetStats(cgroupPath string) (*CgroupStats, error) {
	return nil, fmt.Errorf("cgroups require Linux")
}

// Freeze freezes a cgroup (stub for non-Linux).
func (m *CgroupManager) Freeze(cgroupPath string) error {
	return fmt.Errorf("cgroups require Linux")
}

// Thaw thaws a frozen cgroup (stub for non-Linux).
func (m *CgroupManager) Thaw(cgroupPath string) error {
	return fmt.Errorf("cgroups require Linux")
}

// IsFrozen checks if a cgroup is frozen (stub for non-Linux).
func (m *CgroupManager) IsFrozen(cgroupPath string) (bool, error) {
	return false, fmt.Errorf("cgroups require Linux")
}

// UpdateResources updates resource limits (stub for non-Linux).
func (m *CgroupManager) UpdateResources(cgroupPath string, resources *ResourceConfig) error {
	return fmt.Errorf("cgroups require Linux")
}

// GetCgroupPath returns the cgroup path (stub for non-Linux).
func (m *CgroupManager) GetCgroupPath(containerID string) (string, error) {
	return "", fmt.Errorf("cgroups require Linux")
}

// ListProcesses lists processes in a cgroup (stub for non-Linux).
func (m *CgroupManager) ListProcesses(cgroupPath string) ([]int, error) {
	return nil, fmt.Errorf("cgroups require Linux")
}

// IsControllerAvailable checks controller availability (stub for non-Linux).
func (m *CgroupManager) IsControllerAvailable(controller CgroupController) bool {
	return false
}

// GetAvailableControllers returns available controllers (stub for non-Linux).
func (m *CgroupManager) GetAvailableControllers() []CgroupController {
	return nil
}

// SetIOLimit sets IO limits (stub for non-Linux).
func (m *CgroupManager) SetIOLimit(cgroupPath string, deviceMajor, deviceMinor int, rbps, wbps, riops, wiops int64) error {
	return fmt.Errorf("cgroups require Linux")
}

// SetMemoryEvents enables memory events (stub for non-Linux).
func (m *CgroupManager) SetMemoryEvents(cgroupPath string) error {
	return fmt.Errorf("cgroups require Linux")
}

// GetMemoryEvents returns memory events (stub for non-Linux).
func (m *CgroupManager) GetMemoryEvents(cgroupPath string) (map[string]uint64, error) {
	return nil, fmt.Errorf("cgroups require Linux")
}

// GetV2Manager returns the enhanced cgroups v2 manager (stub for non-Linux).
func (m *CgroupManager) GetV2Manager() *CgroupV2Manager {
	return nil
}

// WatchContainerEvents starts watching cgroup events (stub for non-Linux).
func (m *CgroupManager) WatchContainerEvents(containerID string) (<-chan CgroupEvent, error) {
	return nil, fmt.Errorf("cgroups require Linux")
}

// StopContainerEventWatcher stops watching events (stub for non-Linux).
func (m *CgroupManager) StopContainerEventWatcher(containerID string) {}

// StartPressureMonitoring starts monitoring pressure (stub for non-Linux).
func (m *CgroupManager) StartPressureMonitoring(containerID string, thresholds PressureThresholds, callback PressureCallback) error {
	return fmt.Errorf("cgroups require Linux")
}

// StopPressureMonitoring stops pressure monitoring (stub for non-Linux).
func (m *CgroupManager) StopPressureMonitoring(containerID string) {}

// GetPressureStats returns pressure statistics (stub for non-Linux).
func (m *CgroupManager) GetPressureStats(containerID string) (*CgroupPressureStats, error) {
	return nil, fmt.Errorf("cgroups require Linux")
}

// GetDetailedStats returns comprehensive cgroup v2 statistics (stub for non-Linux).
func (m *CgroupManager) GetDetailedStats(containerID string) (*CgroupV2Stats, error) {
	return nil, fmt.Errorf("cgroups require Linux")
}

// KillAllProcesses sends SIGKILL to all processes (stub for non-Linux).
func (m *CgroupManager) KillAllProcesses(containerID string) error {
	return fmt.Errorf("cgroups require Linux")
}

// SetIOWeight sets the default IO weight (stub for non-Linux).
func (m *CgroupManager) SetIOWeight(containerID string, weight uint16) error {
	return fmt.Errorf("cgroups require Linux")
}

// SetIOLatencyTarget sets the IO latency target (stub for non-Linux).
func (m *CgroupManager) SetIOLatencyTarget(containerID string, major, minor uint64, targetUsec uint64) error {
	return fmt.Errorf("cgroups require Linux")
}

// GetIOStats returns IO statistics (stub for non-Linux).
func (m *CgroupManager) GetIOStats(containerID string) (*CgroupIOStats, error) {
	return nil, fmt.Errorf("cgroups require Linux")
}

// Close cleans up resources (stub for non-Linux).
func (m *CgroupManager) Close() error {
	return nil
}
