// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package inventory

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewInventory(t *testing.T) {
	inv := NewInventory()
	if inv == nil {
		t.Fatal("NewInventory returned nil")
	}
	if inv.hardware == nil {
		t.Error("hardware map not initialized")
	}
	if inv.byMAC == nil {
		t.Error("byMAC map not initialized")
	}
	if inv.byUUID == nil {
		t.Error("byUUID map not initialized")
	}
}

func TestAdd_Success(t *testing.T) {
	inv := NewInventory()
	hw := &Hardware{
		SerialNumber: "SN001",
		MACAddress:   "00:11:22:33:44:55",
		UUID:         "uuid-001",
		Manufacturer: "TestCo",
	}

	if err := inv.Add(hw); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if hw.DiscoveredAt.IsZero() {
		t.Error("DiscoveredAt should be set")
	}
	if hw.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set")
	}
}

func TestAdd_NilHardware(t *testing.T) {
	inv := NewInventory()
	err := inv.Add(nil)
	if err == nil {
		t.Fatal("expected error for nil hardware")
	}
	if !strings.Contains(err.Error(), "cannot be nil") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAdd_EmptySerialNumber(t *testing.T) {
	inv := NewInventory()
	err := inv.Add(&Hardware{})
	if err == nil {
		t.Fatal("expected error for empty serial number")
	}
	if !strings.Contains(err.Error(), "serial number is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAdd_UpdateExisting(t *testing.T) {
	inv := NewInventory()

	hw := &Hardware{
		SerialNumber: "SN001",
		Manufacturer: "OldCo",
	}
	if err := inv.Add(hw); err != nil {
		t.Fatal(err)
	}
	originalDiscoveredAt := hw.DiscoveredAt

	// Small delay to ensure timestamps differ
	time.Sleep(time.Millisecond)

	hw2 := &Hardware{
		SerialNumber: "SN001",
		Manufacturer: "NewCo",
	}
	if err := inv.Add(hw2); err != nil {
		t.Fatal(err)
	}

	// DiscoveredAt should be preserved from first add
	if hw2.DiscoveredAt != originalDiscoveredAt {
		t.Error("DiscoveredAt should be preserved on update")
	}
	if hw2.LastUpdated.Equal(originalDiscoveredAt) {
		t.Error("LastUpdated should be newer")
	}
}

func TestGet(t *testing.T) {
	inv := NewInventory()
	inv.Add(&Hardware{SerialNumber: "SN001", Manufacturer: "TestCo"})

	hw, err := inv.Get("SN001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hw.Manufacturer != "TestCo" {
		t.Errorf("Manufacturer = %s", hw.Manufacturer)
	}
}

func TestGet_NotFound(t *testing.T) {
	inv := NewInventory()
	_, err := inv.Get("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetByMAC(t *testing.T) {
	inv := NewInventory()
	inv.Add(&Hardware{SerialNumber: "SN001", MACAddress: "aa:bb:cc:dd:ee:ff"})

	hw, err := inv.GetByMAC("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("GetByMAC: %v", err)
	}
	if hw.SerialNumber != "SN001" {
		t.Errorf("SerialNumber = %s", hw.SerialNumber)
	}
}

func TestGetByMAC_NotFound(t *testing.T) {
	inv := NewInventory()
	_, err := inv.GetByMAC("00:00:00:00:00:00")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetByUUID(t *testing.T) {
	inv := NewInventory()
	inv.Add(&Hardware{SerialNumber: "SN001", UUID: "test-uuid"})

	hw, err := inv.GetByUUID("test-uuid")
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if hw.SerialNumber != "SN001" {
		t.Errorf("SerialNumber = %s", hw.SerialNumber)
	}
}

func TestGetByUUID_NotFound(t *testing.T) {
	inv := NewInventory()
	_, err := inv.GetByUUID("missing-uuid")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRemove(t *testing.T) {
	inv := NewInventory()
	inv.Add(&Hardware{
		SerialNumber: "SN001",
		MACAddress:   "aa:bb:cc:dd:ee:ff",
		UUID:         "uuid-001",
	})

	if err := inv.Remove("SN001"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Should be gone from all indexes
	_, err := inv.Get("SN001")
	if err == nil {
		t.Error("expected error after remove")
	}
	_, err = inv.GetByMAC("aa:bb:cc:dd:ee:ff")
	if err == nil {
		t.Error("expected MAC index to be cleaned up")
	}
	_, err = inv.GetByUUID("uuid-001")
	if err == nil {
		t.Error("expected UUID index to be cleaned up")
	}
}

func TestRemove_NotFound(t *testing.T) {
	inv := NewInventory()
	err := inv.Remove("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList(t *testing.T) {
	inv := NewInventory()
	inv.Add(&Hardware{SerialNumber: "SN001"})
	inv.Add(&Hardware{SerialNumber: "SN002"})
	inv.Add(&Hardware{SerialNumber: "SN003"})

	list := inv.List()
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
}

func TestList_Empty(t *testing.T) {
	inv := NewInventory()
	list := inv.List()
	if len(list) != 0 {
		t.Errorf("expected 0, got %d", len(list))
	}
}

// --- Filter tests ---

func makeTestHardware() []*Hardware {
	gpu := true
	_ = gpu
	return []*Hardware{
		{
			SerialNumber: "SN-BIG",
			Manufacturer: "Dell",
			CPUs:         []CPU{{Cores: 16}, {Cores: 16}},
			Memory:       Memory{TotalBytes: 128 * 1024 * 1024 * 1024},
			Storage:      []Disk{{SizeBytes: 2e12, Type: "nvme"}},
			GPUs:         []GPU{{Model: "A100"}},
		},
		{
			SerialNumber: "SN-SMALL",
			Manufacturer: "HP",
			CPUs:         []CPU{{Cores: 4}},
			Memory:       Memory{TotalBytes: 16 * 1024 * 1024 * 1024},
			Storage:      []Disk{{SizeBytes: 500e9, Type: "ssd"}},
		},
		{
			SerialNumber: "SN-MED",
			Manufacturer: "Dell",
			CPUs:         []CPU{{Cores: 8}, {Cores: 8}},
			Memory:       Memory{TotalBytes: 64 * 1024 * 1024 * 1024},
			Storage:      []Disk{{SizeBytes: 1e12, Type: "hdd"}, {SizeBytes: 500e9, Type: "ssd"}},
		},
	}
}

func populatedInventory(t *testing.T) *Inventory {
	t.Helper()
	inv := NewInventory()
	for _, hw := range makeTestHardware() {
		if err := inv.Add(hw); err != nil {
			t.Fatal(err)
		}
	}
	return inv
}

func TestFilter_MinCPUs(t *testing.T) {
	inv := populatedInventory(t)
	result := inv.Filter(&HardwareFilter{MinCPUs: 2})
	if len(result) != 2 {
		t.Errorf("expected 2 machines with >=2 CPUs, got %d", len(result))
	}
}

func TestFilter_MinCores(t *testing.T) {
	inv := populatedInventory(t)
	result := inv.Filter(&HardwareFilter{MinCores: 10})
	if len(result) != 2 {
		t.Errorf("expected 2 machines with >=10 cores, got %d", len(result))
	}
}

func TestFilter_MinMemory(t *testing.T) {
	inv := populatedInventory(t)
	result := inv.Filter(&HardwareFilter{MinMemoryBytes: 32 * 1024 * 1024 * 1024})
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestFilter_MinStorage(t *testing.T) {
	inv := populatedInventory(t)
	result := inv.Filter(&HardwareFilter{MinStorageBytes: 1e12})
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestFilter_DiskType(t *testing.T) {
	inv := populatedInventory(t)
	result := inv.Filter(&HardwareFilter{DiskType: "nvme"})
	if len(result) != 1 {
		t.Errorf("expected 1 nvme machine, got %d", len(result))
	}
}

func TestFilter_HasGPU(t *testing.T) {
	inv := populatedInventory(t)
	hasGPU := true
	result := inv.Filter(&HardwareFilter{HasGPU: &hasGPU})
	if len(result) != 1 {
		t.Errorf("expected 1 GPU machine, got %d", len(result))
	}

	noGPU := false
	result = inv.Filter(&HardwareFilter{HasGPU: &noGPU})
	if len(result) != 2 {
		t.Errorf("expected 2 non-GPU machines, got %d", len(result))
	}
}

func TestFilter_Manufacturer(t *testing.T) {
	inv := populatedInventory(t)
	result := inv.Filter(&HardwareFilter{Manufacturer: "Dell"})
	if len(result) != 2 {
		t.Errorf("expected 2 Dell machines, got %d", len(result))
	}
}

func TestFilter_Combined(t *testing.T) {
	inv := populatedInventory(t)
	hasGPU := true
	result := inv.Filter(&HardwareFilter{
		Manufacturer: "Dell",
		MinCores:     16,
		HasGPU:       &hasGPU,
	})
	if len(result) != 1 {
		t.Errorf("expected 1 match, got %d", len(result))
	}
	if len(result) > 0 && result[0].SerialNumber != "SN-BIG" {
		t.Errorf("expected SN-BIG, got %s", result[0].SerialNumber)
	}
}

func TestFilter_NoMatch(t *testing.T) {
	inv := populatedInventory(t)
	result := inv.Filter(&HardwareFilter{Manufacturer: "Lenovo"})
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

// --- Summary tests ---

func TestSummary(t *testing.T) {
	inv := populatedInventory(t)
	summary := inv.Summary()

	if summary.TotalMachines != 3 {
		t.Errorf("TotalMachines = %d", summary.TotalMachines)
	}
	// 16+16 + 4 + 8+8 = 52
	if summary.TotalCores != 52 {
		t.Errorf("TotalCores = %d, want 52", summary.TotalCores)
	}
	if summary.MachinesWithGPU != 1 {
		t.Errorf("MachinesWithGPU = %d", summary.MachinesWithGPU)
	}
	if summary.Manufacturers["Dell"] != 2 {
		t.Errorf("Dell count = %d", summary.Manufacturers["Dell"])
	}
	if summary.Manufacturers["HP"] != 1 {
		t.Errorf("HP count = %d", summary.Manufacturers["HP"])
	}
}

func TestSummary_Empty(t *testing.T) {
	inv := NewInventory()
	summary := inv.Summary()
	if summary.TotalMachines != 0 {
		t.Errorf("TotalMachines = %d", summary.TotalMachines)
	}
}

// --- Export/Import tests ---

func TestExportImport(t *testing.T) {
	inv := NewInventory()
	inv.Add(&Hardware{
		SerialNumber: "SN001",
		MACAddress:   "aa:bb:cc:dd:ee:ff",
		UUID:         "uuid-001",
		Manufacturer: "TestCo",
		CPUs:         []CPU{{Model: "Xeon", Cores: 8}},
	})

	data, err := inv.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}

	// Import into a new inventory
	inv2 := NewInventory()
	if err := inv2.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}

	hw, err := inv2.Get("SN001")
	if err != nil {
		t.Fatalf("Get after import: %v", err)
	}
	if hw.Manufacturer != "TestCo" {
		t.Errorf("Manufacturer = %s", hw.Manufacturer)
	}

	// MAC index should work
	hw2, err := inv2.GetByMAC("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("GetByMAC after import: %v", err)
	}
	if hw2.SerialNumber != "SN001" {
		t.Error("MAC index not rebuilt")
	}

	// UUID index should work
	hw3, err := inv2.GetByUUID("uuid-001")
	if err != nil {
		t.Fatalf("GetByUUID after import: %v", err)
	}
	if hw3.SerialNumber != "SN001" {
		t.Error("UUID index not rebuilt")
	}
}

func TestImport_BadJSON(t *testing.T) {
	inv := NewInventory()
	err := inv.Import([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

// --- Discovery tests ---

func TestDefaultDiscoveryConfig(t *testing.T) {
	cfg := DefaultDiscoveryConfig()
	if cfg == nil {
		t.Fatal("nil config")
	}
	if len(cfg.Networks) != 1 || cfg.Networks[0] != "10.0.0.0/24" {
		t.Errorf("unexpected networks: %v", cfg.Networks)
	}
	if cfg.Concurrency != 50 {
		t.Errorf("Concurrency = %d", cfg.Concurrency)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
}

func TestNewDiscovery(t *testing.T) {
	d := NewDiscovery(nil)
	if d == nil {
		t.Fatal("nil discovery")
	}
	if d.config.Concurrency != 50 {
		t.Error("should use default config when nil")
	}
}

func TestNewDiscovery_CustomConfig(t *testing.T) {
	cfg := &DiscoveryConfig{
		Networks:    []string{"192.168.1.0/24"},
		Concurrency: 10,
		Timeout:     time.Second,
	}
	d := NewDiscovery(cfg)
	if d.config.Concurrency != 10 {
		t.Errorf("Concurrency = %d", d.config.Concurrency)
	}
}

func TestDiscovery_Callbacks(t *testing.T) {
	d := NewDiscovery(nil)

	var discovered []*Hardware
	d.OnDiscovered(func(hw *Hardware) {
		discovered = append(discovered, hw)
	})

	var lost []string
	d.OnLost(func(serial string) {
		lost = append(lost, serial)
	})

	// handleDiscovered with a machine that has a serial
	hw := &Hardware{
		SerialNumber: "SN-TEST",
		MACAddress:   "00:11:22:33:44:55",
	}
	d.handleDiscovered(hw)

	if len(discovered) != 1 {
		t.Errorf("expected 1 discovered, got %d", len(discovered))
	}

	// Calling again with same serial should NOT trigger callback again
	d.handleDiscovered(hw)
	if len(discovered) != 1 {
		t.Errorf("expected still 1 discovered (update, not new), got %d", len(discovered))
	}
}

func TestDiscovery_HandleDiscovered_NoIdentifier(t *testing.T) {
	d := NewDiscovery(nil)
	callCount := 0
	d.OnDiscovered(func(hw *Hardware) {
		callCount++
	})

	// Hardware with no serial or MAC should be ignored
	d.handleDiscovered(&Hardware{})
	if callCount != 0 {
		t.Error("callback should not fire for hardware without identifiers")
	}
}

func TestDiscovery_HandleDiscovered_MACOnly(t *testing.T) {
	d := NewDiscovery(nil)
	callCount := 0
	d.OnDiscovered(func(hw *Hardware) {
		callCount++
	})

	hw := &Hardware{MACAddress: "aa:bb:cc:dd:ee:ff"}
	d.handleDiscovered(hw)
	if callCount != 1 {
		t.Errorf("expected 1 callback, got %d", callCount)
	}
}

func TestDiscovery_GetKnown(t *testing.T) {
	d := NewDiscovery(nil)
	d.handleDiscovered(&Hardware{SerialNumber: "SN1"})
	d.handleDiscovered(&Hardware{SerialNumber: "SN2"})

	known := d.GetKnown()
	if len(known) != 2 {
		t.Errorf("expected 2 known, got %d", len(known))
	}
}

func TestDiscovery_StartStop(t *testing.T) {
	cfg := &DiscoveryConfig{
		Networks:     []string{},
		ScanInterval: time.Hour, // long interval so scan doesn't run multiple times
		Concurrency:  1,
		Timeout:      time.Millisecond,
	}
	d := NewDiscovery(cfg)

	// Start should succeed
	ctx := t.Context()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double start should fail
	if err := d.Start(ctx); err == nil {
		t.Error("expected error for double start")
	}

	// Stop should succeed
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double stop should be idempotent
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop again: %v", err)
	}
}

func TestExpandNetwork(t *testing.T) {
	d := NewDiscovery(nil)

	ips, err := d.expandNetwork("192.168.1.0/30")
	if err != nil {
		t.Fatalf("expandNetwork: %v", err)
	}
	// /30 = 4 addresses, minus network and broadcast = 2
	if len(ips) != 2 {
		t.Errorf("expected 2 IPs, got %d: %v", len(ips), ips)
	}
}

func TestExpandNetwork_Invalid(t *testing.T) {
	d := NewDiscovery(nil)
	_, err := d.expandNetwork("invalid")
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestNewDHCPLeaseDiscovery(t *testing.T) {
	d := NewDHCPLeaseDiscovery("/var/lib/dhcp/leases")
	if d == nil {
		t.Fatal("nil")
	}
	if d.leasesFile != "/var/lib/dhcp/leases" {
		t.Errorf("leasesFile = %s", d.leasesFile)
	}
}

func TestNewLLDPDiscovery(t *testing.T) {
	d := NewLLDPDiscovery([]string{"eth0", "eth1"})
	if d == nil {
		t.Fatal("nil")
	}
	if len(d.interfaces) != 2 {
		t.Errorf("interfaces len = %d", len(d.interfaces))
	}
}

func TestConcurrentInventoryAccess(t *testing.T) {
	inv := NewInventory()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sn := string(rune('A' + i))
			inv.Add(&Hardware{SerialNumber: sn, Manufacturer: "Test"})
			inv.List()
			inv.Summary()
			inv.Get(sn)
		}(i)
	}
	wg.Wait()
}
