// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build !linux

package runtime

import (
	"context"
	"fmt"
	"time"
)

// CgroupV2Manager provides enhanced cgroups v2 management (stub for non-Linux).
type CgroupV2Manager struct {
	cgroupPaths map[string]string
}

// CgroupV2Config contains configuration for creating a cgroup.
type CgroupV2Config struct {
	CPUWeight       uint64
	CPUMax          int64
	CPUMaxPeriod    uint64
	CPUSetCPUs      string
	CPUSetMems      string
	CPUSetPartition string
	MemoryMax       int64
	MemoryHigh      int64
	MemoryLow       int64
	MemoryMin       int64
	MemorySwapMax   int64
	MemoryOOMGroup  bool
	IOWeight        uint16
	IOMax           []CgroupIOMaxConfig
	PidsMax         int64
	RDMAMax         map[string]RDMALimit
	HugetlbMax      map[string]int64
}

// CgroupIOMaxConfig contains IO limits for a specific device.
type CgroupIOMaxConfig struct {
	Major uint64
	Minor uint64
	Rbps  int64
	Wbps  int64
	Riops int64
	Wiops int64
}

// RDMALimit contains RDMA limits.
type RDMALimit struct {
	HcaHandle uint32
	HcaObject uint32
}

// CgroupV2Stats contains comprehensive cgroup v2 statistics.
type CgroupV2Stats struct {
	CPU      CgroupCPUStats
	Memory   CgroupMemoryStats
	IO       CgroupIOStats
	PIDs     CgroupPIDsStats
	Pressure CgroupPressureStats
}

// CgroupCPUStats contains CPU statistics.
type CgroupCPUStats struct {
	UsageUsec     uint64
	UserUsec      uint64
	SystemUsec    uint64
	NrPeriods     uint64
	NrThrottled   uint64
	ThrottledUsec uint64
	NrBursts      uint64
	BurstUsec     uint64
	Pressure      *PressureData
}

// CgroupMemoryStats contains memory statistics.
type CgroupMemoryStats struct {
	Current               uint64
	Peak                  uint64
	High                  uint64
	Max                   uint64
	Min                   uint64
	Low                   uint64
	SwapCurrent           uint64
	SwapHigh              uint64
	SwapMax               uint64
	Anon                  uint64
	File                  uint64
	Kernel                uint64
	KernelStack           uint64
	Slab                  uint64
	Sock                  uint64
	Shmem                 uint64
	FileMapped            uint64
	FileDirty             uint64
	FileWriteback         uint64
	AnonThp               uint64
	InactiveAnon          uint64
	ActiveAnon            uint64
	InactiveFile          uint64
	ActiveFile            uint64
	Unevictable           uint64
	SlabReclaimable       uint64
	SlabUnreclaimable     uint64
	Pgfault               uint64
	Pgmajfault            uint64
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
	Low          uint64
	High         uint64
	Max          uint64
	OOM          uint64
	OOMKill      uint64
	OOMGroupKill uint64
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
	Rbytes uint64
	Wbytes uint64
	Rios   uint64
	Wios   uint64
	Dbytes uint64
	Dios   uint64
}

// CgroupPIDsStats contains PIDs statistics.
type CgroupPIDsStats struct {
	Current uint64
	Max     uint64
	Events  PIDsEvents
}

// PIDsEvents contains PIDs event counters.
type PIDsEvents struct {
	Max uint64
}

// CgroupPressureStats contains pressure statistics.
type CgroupPressureStats struct {
	CPU    *PressureData
	Memory *PressureData
	IO     *PressureData
}

// PressureData contains pressure stall information.
type PressureData struct {
	Some PressureMetrics
	Full PressureMetrics
}

// PressureMetrics contains pressure metrics.
type PressureMetrics struct {
	Avg10  float64
	Avg60  float64
	Avg300 float64
	Total  uint64
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

// CgroupEventWatcher watches for cgroup events (stub).
type CgroupEventWatcher struct{}

// PressureMonitor monitors resource pressure (stub).
type PressureMonitor struct{}

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

var errNotLinux = fmt.Errorf("cgroups v2 require Linux")

// NewCgroupV2Manager creates a new enhanced cgroup v2 manager (stub for non-Linux).
func NewCgroupV2Manager(parentPath string) (*CgroupV2Manager, error) {
	return &CgroupV2Manager{
		cgroupPaths: make(map[string]string),
	}, nil
}

// CreateCgroupV2 creates a cgroup with full v2 configuration (stub for non-Linux).
func (m *CgroupV2Manager) CreateCgroupV2(containerID string, config *CgroupV2Config) (string, error) {
	return "", errNotLinux
}

// DeleteCgroup removes a cgroup (stub for non-Linux).
func (m *CgroupV2Manager) DeleteCgroup(containerID string) error {
	return errNotLinux
}

// GetStatsV2 returns comprehensive cgroup v2 statistics (stub for non-Linux).
func (m *CgroupV2Manager) GetStatsV2(containerID string) (*CgroupV2Stats, error) {
	return nil, errNotLinux
}

// FreezeV2 freezes a cgroup (stub for non-Linux).
func (m *CgroupV2Manager) FreezeV2(containerID string) error {
	return errNotLinux
}

// ThawV2 thaws a frozen cgroup (stub for non-Linux).
func (m *CgroupV2Manager) ThawV2(containerID string) error {
	return errNotLinux
}

// IsFrozenV2 checks if a cgroup is frozen (stub for non-Linux).
func (m *CgroupV2Manager) IsFrozenV2(containerID string) (bool, error) {
	return false, errNotLinux
}

// GetFreezeState returns the freeze state (stub for non-Linux).
func (m *CgroupV2Manager) GetFreezeState(containerID string) (frozen bool, populated bool, err error) {
	return false, false, errNotLinux
}

// UpdateResourcesV2 updates resource limits dynamically (stub for non-Linux).
func (m *CgroupV2Manager) UpdateResourcesV2(containerID string, config *CgroupV2Config) error {
	return errNotLinux
}

// AddProcessV2 adds a process to a cgroup (stub for non-Linux).
func (m *CgroupV2Manager) AddProcessV2(containerID string, pid int) error {
	return errNotLinux
}

// AddThreadV2 adds a thread to a cgroup (stub for non-Linux).
func (m *CgroupV2Manager) AddThreadV2(containerID string, tid int) error {
	return errNotLinux
}

// ListProcesses returns all processes in a cgroup (stub for non-Linux).
func (m *CgroupV2Manager) ListProcesses(containerID string) ([]int, error) {
	return nil, errNotLinux
}

// WatchEvents starts watching for cgroup events (stub for non-Linux).
func (m *CgroupV2Manager) WatchEvents(containerID string) (<-chan CgroupEvent, error) {
	return nil, errNotLinux
}

// Stop stops the event watcher (stub for non-Linux).
func (w *CgroupEventWatcher) Stop() {}

// StartPressureMonitor starts monitoring resource pressure (stub for non-Linux).
func (m *CgroupV2Manager) StartPressureMonitor(containerID string, thresholds PressureThresholds, callback PressureCallback) error {
	return errNotLinux
}

// Stop stops the pressure monitor (stub for non-Linux).
func (pm *PressureMonitor) Stop() {}

// StopPressureMonitor stops the pressure monitor (stub for non-Linux).
func (m *CgroupV2Manager) StopPressureMonitor(containerID string) {}

// StopEventWatcher stops the event watcher (stub for non-Linux).
func (m *CgroupV2Manager) StopEventWatcher(containerID string) {}

// GetAvailableControllersV2 returns the list of available controllers (stub for non-Linux).
func (m *CgroupV2Manager) GetAvailableControllersV2() ([]string, error) {
	return nil, errNotLinux
}

// GetSubtreeControllersV2 returns the list of enabled subtree controllers (stub for non-Linux).
func (m *CgroupV2Manager) GetSubtreeControllersV2() ([]string, error) {
	return nil, errNotLinux
}

// SetIOWeightDevice sets IO weight for a specific device (stub for non-Linux).
func (m *CgroupV2Manager) SetIOWeightDevice(containerID string, major, minor uint64, weight uint16) error {
	return errNotLinux
}

// SetIOLatency sets IO latency target for a device (stub for non-Linux).
func (m *CgroupV2Manager) SetIOLatency(containerID string, major, minor uint64, targetUsec uint64) error {
	return errNotLinux
}

// KillAll sends a signal to all processes in the cgroup (stub for non-Linux).
func (m *CgroupV2Manager) KillAll(containerID string) error {
	return errNotLinux
}

// GetType returns the cgroup type (stub for non-Linux).
func (m *CgroupV2Manager) GetType(containerID string) (string, error) {
	return "", errNotLinux
}

// SetType sets the cgroup type (stub for non-Linux).
func (m *CgroupV2Manager) SetType(containerID string, cgroupType string) error {
	return errNotLinux
}

// WaitForEmpty waits for the cgroup to become empty (stub for non-Linux).
func (m *CgroupV2Manager) WaitForEmpty(ctx context.Context, containerID string) error {
	return errNotLinux
}

// Close closes the cgroup manager (stub for non-Linux).
func (m *CgroupV2Manager) Close() error {
	return nil
}
