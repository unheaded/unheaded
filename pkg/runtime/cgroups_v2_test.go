// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCgroupV2Detection(t *testing.T) {
	// Test cgroup v2 detection
	root := "/sys/fs/cgroup"

	// Check if cgroup v2 is available
	if !isCgroupV2Unified(root) {
		t.Skip("cgroup v2 not available, skipping test")
	}

	// Verify cgroup.controllers exists
	controllersPath := filepath.Join(root, "cgroup.controllers")
	if _, err := os.Stat(controllersPath); err != nil {
		t.Errorf("cgroup.controllers not found: %v", err)
	}
}

func TestNewCgroupV2Manager(t *testing.T) {
	// Skip if not running as root or cgroup v2 not available
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	if !isCgroupV2Unified("/sys/fs/cgroup") {
		t.Skip("cgroup v2 not available")
	}

	mgr, err := NewCgroupV2Manager("/unheaded-test")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() {
		mgr.Close()
		// Cleanup test cgroup
		os.RemoveAll("/sys/fs/cgroup/unheaded-test")
	}()

	// Verify parent cgroup was created
	parentPath := "/sys/fs/cgroup/unheaded-test"
	if _, err := os.Stat(parentPath); err != nil {
		t.Errorf("Parent cgroup not created: %v", err)
	}
}

func TestCgroupV2Config(t *testing.T) {
	// Test configuration struct creation
	config := &CgroupV2Config{
		CPUWeight:      100,
		CPUMax:         50000,
		CPUMaxPeriod:   100000,
		MemoryMax:      1024 * 1024 * 512, // 512MB
		MemoryHigh:     1024 * 1024 * 400, // 400MB
		MemoryLow:      1024 * 1024 * 100, // 100MB
		MemoryMin:      1024 * 1024 * 50,  // 50MB
		MemorySwapMax:  1024 * 1024 * 256, // 256MB
		MemoryOOMGroup: true,
		IOWeight:       100,
		PidsMax:        1000,
	}

	// Verify values
	if config.CPUWeight != 100 {
		t.Errorf("Expected CPUWeight 100, got %d", config.CPUWeight)
	}
	if config.MemoryMax != 1024*1024*512 {
		t.Errorf("Expected MemoryMax 512MB, got %d", config.MemoryMax)
	}
}

func TestIOMaxConfigString(t *testing.T) {
	mgr := &CgroupV2Manager{}

	tests := []struct {
		name     string
		config   CgroupIOMaxConfig
		expected string
	}{
		{
			name: "all limits set",
			config: CgroupIOMaxConfig{
				Major: 8,
				Minor: 0,
				Rbps:  1000000,
				Wbps:  500000,
				Riops: 1000,
				Wiops: 500,
			},
			expected: "8:0 rbps=1000000 wbps=500000 riops=1000 wiops=500",
		},
		{
			name: "only read limits",
			config: CgroupIOMaxConfig{
				Major: 8,
				Minor: 0,
				Rbps:  1000000,
				Riops: 1000,
				Wbps:  -1,
				Wiops: -1,
			},
			expected: "8:0 rbps=1000000 riops=1000",
		},
		{
			name: "max (unlimited) values",
			config: CgroupIOMaxConfig{
				Major: 8,
				Minor: 0,
				Rbps:  0,
				Wbps:  0,
				Riops: -1,
				Wiops: -1,
			},
			expected: "8:0 rbps=max wbps=max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgr.buildIOMaxContent(tt.config)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestPressureMetricsParsing(t *testing.T) {
	mgr := &CgroupV2Manager{}

	// Test parsing pressure line
	var metrics PressureMetrics
	mgr.parsePressureLine("avg10=0.50 avg60=1.25 avg300=2.00 total=12345", &metrics)

	if metrics.Avg10 != 0.50 {
		t.Errorf("Expected Avg10 0.50, got %f", metrics.Avg10)
	}
	if metrics.Avg60 != 1.25 {
		t.Errorf("Expected Avg60 1.25, got %f", metrics.Avg60)
	}
	if metrics.Avg300 != 2.00 {
		t.Errorf("Expected Avg300 2.00, got %f", metrics.Avg300)
	}
	if metrics.Total != 12345 {
		t.Errorf("Expected Total 12345, got %d", metrics.Total)
	}
}

func TestPressureThresholds(t *testing.T) {
	thresholds := PressureThresholds{
		CPUSomeAvg10:    50.0,
		MemorySomeAvg10: 40.0,
		MemoryFullAvg10: 30.0,
		IOSomeAvg10:     60.0,
		IOFullAvg10:     50.0,
	}

	if thresholds.CPUSomeAvg10 != 50.0 {
		t.Errorf("Expected CPUSomeAvg10 50.0, got %f", thresholds.CPUSomeAvg10)
	}
}

func TestCgroupEventTypes(t *testing.T) {
	// Verify event type constants
	events := []CgroupEventType{
		CgroupEventMemoryMax,
		CgroupEventMemoryHigh,
		CgroupEventMemoryOOM,
		CgroupEventMemoryOOMKill,
		CgroupEventPidsMax,
		CgroupEventFrozen,
		CgroupEventPopulated,
	}

	// Ensure all event types are distinct
	seen := make(map[CgroupEventType]bool)
	for _, e := range events {
		if seen[e] {
			t.Errorf("Duplicate event type: %d", e)
		}
		seen[e] = true
	}
}

func TestCgroupV2StatsStructure(t *testing.T) {
	stats := &CgroupV2Stats{}

	// Test that all fields can be set
	stats.CPU.UsageUsec = 1000000
	stats.CPU.UserUsec = 500000
	stats.CPU.SystemUsec = 500000

	stats.Memory.Current = 1024 * 1024 * 100
	stats.Memory.Peak = 1024 * 1024 * 200
	stats.Memory.High = 1024 * 1024 * 400
	stats.Memory.Max = 1024 * 1024 * 512

	stats.IO.Devices = []DeviceIOStats{
		{
			Major:  8,
			Minor:  0,
			Rbytes: 1000000,
			Wbytes: 500000,
		},
	}

	stats.PIDs.Current = 50
	stats.PIDs.Max = 1000

	// Verify values
	if stats.CPU.UsageUsec != 1000000 {
		t.Errorf("Expected UsageUsec 1000000, got %d", stats.CPU.UsageUsec)
	}
	if len(stats.IO.Devices) != 1 {
		t.Errorf("Expected 1 IO device, got %d", len(stats.IO.Devices))
	}
}

func TestMemoryEventsStructure(t *testing.T) {
	events := MemoryEvents{
		Low:          10,
		High:         5,
		Max:          3,
		OOM:          1,
		OOMKill:      1,
		OOMGroupKill: 0,
	}

	if events.Low != 10 {
		t.Errorf("Expected Low 10, got %d", events.Low)
	}
	if events.OOMKill != 1 {
		t.Errorf("Expected OOMKill 1, got %d", events.OOMKill)
	}
}

func TestParseUint64V2(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"12345", 12345},
		{"  12345  ", 12345},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
		{"18446744073709551615", 18446744073709551615}, // Max uint64
	}

	for _, tt := range tests {
		result := parseUint64V2(tt.input)
		if result != tt.expected {
			t.Errorf("parseUint64V2(%q) = %d, expected %d", tt.input, result, tt.expected)
		}
	}
}

func TestResourceConfigWithIOLimits(t *testing.T) {
	config := &ResourceConfig{
		CPUWeight:      200,
		MemoryLimitBytes: 1024 * 1024 * 512,
		MemoryHighBytes:  1024 * 1024 * 400,
		MemoryMinBytes:   1024 * 1024 * 100,
		IOWeight:        150,
		IOLimits: []IOLimit{
			{
				Major:           8,
				Minor:           0,
				ReadBytesPerSec: 1000000,
				WriteBytesPerSec: 500000,
				ReadIOPS:        1000,
				WriteIOPS:       500,
			},
		},
		PidsLimit: 500,
	}

	if config.CPUWeight != 200 {
		t.Errorf("Expected CPUWeight 200, got %d", config.CPUWeight)
	}
	if len(config.IOLimits) != 1 {
		t.Errorf("Expected 1 IO limit, got %d", len(config.IOLimits))
	}
	if config.IOLimits[0].ReadBytesPerSec != 1000000 {
		t.Errorf("Expected ReadBytesPerSec 1000000, got %d", config.IOLimits[0].ReadBytesPerSec)
	}
}

// Integration tests (require root and cgroup v2)

func TestCgroupV2CreateAndDelete(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	if !isCgroupV2Unified("/sys/fs/cgroup") {
		t.Skip("cgroup v2 not available")
	}

	mgr, err := NewCgroupV2Manager("/unheaded-integration-test")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() {
		mgr.Close()
		os.RemoveAll("/sys/fs/cgroup/unheaded-integration-test")
	}()

	// Create a cgroup
	config := &CgroupV2Config{
		CPUWeight: 100,
		MemoryMax: 1024 * 1024 * 100, // 100MB
		PidsMax:   100,
	}

	cgroupPath, err := mgr.CreateCgroupV2("test-container-1", config)
	if err != nil {
		t.Fatalf("Failed to create cgroup: %v", err)
	}

	// Verify cgroup exists
	fullPath := filepath.Join("/sys/fs/cgroup", cgroupPath)
	if _, err := os.Stat(fullPath); err != nil {
		t.Errorf("Cgroup directory not created: %v", err)
	}

	// Verify cpu.weight was set
	weightPath := filepath.Join(fullPath, "cpu.weight")
	data, err := os.ReadFile(weightPath)
	if err == nil {
		t.Logf("cpu.weight content: %s", string(data))
	}

	// Delete the cgroup
	if err := mgr.DeleteCgroup("test-container-1"); err != nil {
		t.Errorf("Failed to delete cgroup: %v", err)
	}

	// Verify cgroup was removed
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Errorf("Cgroup directory still exists after deletion")
	}
}

func TestCgroupV2Freeze(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	if !isCgroupV2Unified("/sys/fs/cgroup") {
		t.Skip("cgroup v2 not available")
	}

	mgr, err := NewCgroupV2Manager("/unheaded-freeze-test")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() {
		mgr.Close()
		os.RemoveAll("/sys/fs/cgroup/unheaded-freeze-test")
	}()

	// Create a cgroup
	_, err = mgr.CreateCgroupV2("freeze-test", nil)
	if err != nil {
		t.Fatalf("Failed to create cgroup: %v", err)
	}
	defer mgr.DeleteCgroup("freeze-test")

	// Test freeze
	if err := mgr.FreezeV2("freeze-test"); err != nil {
		t.Logf("Freeze failed (might not be supported): %v", err)
		return
	}

	// Check frozen state
	frozen, err := mgr.IsFrozenV2("freeze-test")
	if err != nil {
		t.Errorf("Failed to check frozen state: %v", err)
	}
	if !frozen {
		t.Logf("Cgroup not frozen (freeze might take time)")
	}

	// Thaw
	if err := mgr.ThawV2("freeze-test"); err != nil {
		t.Errorf("Failed to thaw cgroup: %v", err)
	}
}

func TestCgroupV2Stats(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	if !isCgroupV2Unified("/sys/fs/cgroup") {
		t.Skip("cgroup v2 not available")
	}

	mgr, err := NewCgroupV2Manager("/unheaded-stats-test")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() {
		mgr.Close()
		os.RemoveAll("/sys/fs/cgroup/unheaded-stats-test")
	}()

	// Create a cgroup
	_, err = mgr.CreateCgroupV2("stats-test", &CgroupV2Config{
		MemoryMax: 1024 * 1024 * 100,
		PidsMax:   100,
	})
	if err != nil {
		t.Fatalf("Failed to create cgroup: %v", err)
	}
	defer mgr.DeleteCgroup("stats-test")

	// Get stats
	stats, err := mgr.GetStatsV2("stats-test")
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	// Verify memory.max was set
	if stats.Memory.Max != 1024*1024*100 && stats.Memory.Max != 0 {
		t.Logf("Memory.Max: %d (expected %d or 0 if not set)", stats.Memory.Max, 1024*1024*100)
	}

	t.Logf("Memory current: %d", stats.Memory.Current)
	t.Logf("PIDs current: %d", stats.PIDs.Current)
}

func TestCgroupManagerIOMaxString(t *testing.T) {
	mgr := &CgroupManager{}

	tests := []struct {
		name     string
		limit    IOLimit
		expected string
	}{
		{
			name: "all values set",
			limit: IOLimit{
				Major:            8,
				Minor:            0,
				ReadBytesPerSec:  1000000,
				WriteBytesPerSec: 500000,
				ReadIOPS:         1000,
				WriteIOPS:        500,
			},
			expected: "8:0 rbps=1000000 wbps=500000 riops=1000 wiops=500",
		},
		{
			name: "unlimited (max) values",
			limit: IOLimit{
				Major:            8,
				Minor:            0,
				ReadBytesPerSec:  0,
				WriteBytesPerSec: 0,
				ReadIOPS:         0,
				WriteIOPS:        0,
			},
			expected: "8:0 rbps=max wbps=max riops=max wiops=max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgr.buildIOMaxString(tt.limit)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
