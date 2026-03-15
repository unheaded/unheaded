// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sabatons

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestService creates a Service wired to a silent logger and no wotan
// dependency.  Every test gets an isolated instance.
func newTestService(cfg *Config) *Service {
	log := logger.New(io.Discard)
	return NewService(log, nil, cfg)
}

// newTestServiceDefault is a shorthand that uses DefaultConfig.
func newTestServiceDefault() *Service {
	return newTestService(nil)
}

// testMachine returns a minimal Machine suitable for registration.
func testMachine(id, hostname string, macs ...string) *Machine {
	if len(macs) == 0 {
		macs = []string{"aa:bb:cc:dd:ee:ff"}
	}
	return &Machine{
		ID:           id,
		Hostname:     hostname,
		MACAddresses: macs,
	}
}

// registerTestMachine is a helper that registers and returns the machine,
// failing the test on error.
func registerTestMachine(t *testing.T, svc *Service, m *Machine) *Machine {
	t.Helper()
	if err := svc.RegisterMachine(m); err != nil {
		t.Fatalf("RegisterMachine failed: %v", err)
	}
	return m
}

// ---------------------------------------------------------------------------
// NewService / DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	if cfg.PXEServerAddress != "0.0.0.0:69" {
		t.Errorf("PXEServerAddress = %q, want %q", cfg.PXEServerAddress, "0.0.0.0:69")
	}
	if cfg.TFTPRoot != "/var/lib/tftpboot" {
		t.Errorf("TFTPRoot = %q, want %q", cfg.TFTPRoot, "/var/lib/tftpboot")
	}
	if cfg.DiscoveryInterval != 30*time.Second {
		t.Errorf("DiscoveryInterval = %v, want %v", cfg.DiscoveryInterval, 30*time.Second)
	}
	if cfg.HeartbeatTimeout != 60*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want %v", cfg.HeartbeatTimeout, 60*time.Second)
	}
	if cfg.WotanTopic != "sabatons.hardware" {
		t.Errorf("WotanTopic = %q, want %q", cfg.WotanTopic, "sabatons.hardware")
	}
}

func TestNewService_NilConfig(t *testing.T) {
	t.Parallel()
	svc := newTestService(nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.config.PXEServerAddress != "0.0.0.0:69" {
		t.Error("nil config should default to DefaultConfig()")
	}
}

func TestNewService_CustomConfig(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		PXEServerAddress:  "10.0.0.1:69",
		TFTPRoot:          "/custom/tftp",
		DiscoveryInterval: 10 * time.Second,
		HeartbeatTimeout:  20 * time.Second,
		WotanTopic:       "custom.topic",
	}
	svc := newTestService(cfg)
	if svc.config.PXEServerAddress != "10.0.0.1:69" {
		t.Errorf("expected custom PXEServerAddress, got %q", svc.config.PXEServerAddress)
	}
	if svc.config.WotanTopic != "custom.topic" {
		t.Errorf("expected custom WotanTopic, got %q", svc.config.WotanTopic)
	}
}

func TestNewService_InternalMapsInitialized(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	if svc.machines == nil {
		t.Error("machines map not initialized")
	}
	if svc.bootConfigs == nil {
		t.Error("bootConfigs map not initialized")
	}
	if svc.credentials == nil {
		t.Error("credentials map not initialized")
	}
}

// ---------------------------------------------------------------------------
// Start / Stop lifecycle
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestStart_HeartbeatLoopExitsOnCancel(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		PXEServerAddress:  "0.0.0.0:69",
		TFTPRoot:          "/tmp",
		DiscoveryInterval: 10 * time.Millisecond, // fast for testing
		HeartbeatTimeout:  20 * time.Millisecond,
		WotanTopic:       "test",
	}
	svc := newTestService(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let the heartbeat loop tick at least once.
	time.Sleep(50 * time.Millisecond)
	cancel()
	// Allow goroutine to wind down; if it leaks the race detector will catch it.
	time.Sleep(30 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// RegisterMachine -- table-driven
// ---------------------------------------------------------------------------

func TestRegisterMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		machine    *Machine
		wantID     bool   // expect a non-empty ID after registration
		wantStatus MachineStatus
		wantState  MachineState
	}{
		{
			name:       "with explicit ID",
			machine:    testMachine("m-001", "node1"),
			wantID:     true,
			wantStatus: StatusAvailable,
			wantState:  StateUnknown,
		},
		{
			name: "auto-generated ID",
			machine: &Machine{
				Hostname:     "auto-node",
				MACAddresses: []string{"11:22:33:44:55:66"},
			},
			wantID:     true,
			wantStatus: StatusAvailable,
			wantState:  StateUnknown,
		},
		{
			name: "with BMC address",
			machine: &Machine{
				ID:           "m-bmc",
				Hostname:     "bmc-node",
				BMCAddress:   "192.168.1.100",
				MACAddresses: []string{"aa:bb:cc:dd:ee:01"},
			},
			wantID:     true,
			wantStatus: StatusAvailable,
			wantState:  StateUnknown,
		},
		{
			name: "with labels and serial",
			machine: &Machine{
				ID:           "m-labels",
				Hostname:     "labeled-node",
				SerialNumber: "SN-12345",
				MACAddresses: []string{"aa:bb:cc:dd:ee:02"},
				Labels:       map[string]string{"rack": "R1", "role": "compute"},
			},
			wantID:     true,
			wantStatus: StatusAvailable,
			wantState:  StateUnknown,
		},
		{
			name: "with multiple MACs",
			machine: &Machine{
				ID:           "m-multi-mac",
				Hostname:     "multi-nic",
				MACAddresses: []string{"aa:bb:cc:dd:ee:03", "aa:bb:cc:dd:ee:04"},
			},
			wantID:     true,
			wantStatus: StatusAvailable,
			wantState:  StateUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()
			err := svc.RegisterMachine(tc.machine)
			if err != nil {
				t.Fatalf("RegisterMachine error: %v", err)
			}
			if tc.wantID && tc.machine.ID == "" {
				t.Error("expected non-empty ID")
			}
			if tc.machine.Status != tc.wantStatus {
				t.Errorf("Status = %v, want %v", tc.machine.Status, tc.wantStatus)
			}
			if tc.machine.State != tc.wantState {
				t.Errorf("State = %v, want %v", tc.machine.State, tc.wantState)
			}
			if tc.machine.LastSeen.IsZero() {
				t.Error("LastSeen not set")
			}
		})
	}
}

func TestRegisterMachine_Overwrite(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m1 := testMachine("dup", "first")
	registerTestMachine(t, svc, m1)

	m2 := testMachine("dup", "second")
	registerTestMachine(t, svc, m2)

	got, ok := svc.GetMachine("dup")
	if !ok {
		t.Fatal("machine not found after overwrite")
	}
	if got.Hostname != "second" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "second")
	}
}

// ---------------------------------------------------------------------------
// GetMachine / GetMachineByMAC / ListMachines
// ---------------------------------------------------------------------------

func TestGetMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantOK  bool
	}{
		{"existing machine", "m-1", true},
		{"non-existent machine", "m-999", false},
		{"empty ID", "", false},
	}

	svc := newTestServiceDefault()
	registerTestMachine(t, svc, testMachine("m-1", "node1"))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, ok := svc.GetMachine(tc.id)
			if ok != tc.wantOK {
				t.Errorf("GetMachine(%q) ok = %v, want %v", tc.id, ok, tc.wantOK)
			}
		})
	}
}

func TestGetMachineByMAC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mac     string
		wantOK  bool
	}{
		{"exact match lowercase", "aa:bb:cc:dd:ee:ff", true},
		{"uppercase match", "AA:BB:CC:DD:EE:FF", true},
		{"mixed case", "Aa:Bb:Cc:Dd:Ee:Ff", true},
		{"non-existent MAC", "00:00:00:00:00:00", false},
		{"invalid MAC format", "not-a-mac", false},
		{"empty string", "", false},
		{"second NIC match", "11:22:33:44:55:66", true},
	}

	svc := newTestServiceDefault()
	m := &Machine{
		ID:           "m-mac",
		Hostname:     "mac-node",
		MACAddresses: []string{"aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"},
	}
	registerTestMachine(t, svc, m)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := svc.GetMachineByMAC(tc.mac)
			if ok != tc.wantOK {
				t.Errorf("GetMachineByMAC(%q) ok = %v, want %v", tc.mac, ok, tc.wantOK)
			}
			if ok && got.ID != "m-mac" {
				t.Errorf("returned wrong machine: %s", got.ID)
			}
		})
	}
}

func TestListMachines(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	// Register multiple machines.
	registerTestMachine(t, svc, testMachine("a", "host-a"))
	registerTestMachine(t, svc, testMachine("b", "host-b"))
	registerTestMachine(t, svc, testMachine("c", "host-c"))

	t.Run("no filter returns all", func(t *testing.T) {
		t.Parallel()
		all := svc.ListMachines("")
		if len(all) != 3 {
			t.Errorf("ListMachines('') = %d, want 3", len(all))
		}
	})

	t.Run("filter by available", func(t *testing.T) {
		t.Parallel()
		avail := svc.ListMachines(StatusAvailable)
		if len(avail) != 3 {
			t.Errorf("ListMachines(Available) = %d, want 3", len(avail))
		}
	})

	t.Run("filter by non-existent status", func(t *testing.T) {
		t.Parallel()
		none := svc.ListMachines(StatusError)
		if len(none) != 0 {
			t.Errorf("ListMachines(Error) = %d, want 0", len(none))
		}
	})
}

func TestListMachines_Empty(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	machines := svc.ListMachines("")
	if machines != nil && len(machines) != 0 {
		t.Errorf("expected empty list, got %d", len(machines))
	}
}

// ---------------------------------------------------------------------------
// PXE Boot Configuration Tests
// ---------------------------------------------------------------------------

func TestSetGetBootConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *BootConfig
	}{
		{
			name: "standard config",
			config: &BootConfig{
				MACAddress: "aa:bb:cc:dd:ee:ff",
				Kernel:     "/vmlinuz",
				Initrd:     "/initrd.img",
				KernelArgs: "console=ttyS0 root=/dev/sda1",
			},
		},
		{
			name: "minimal config",
			config: &BootConfig{
				MACAddress: "11:22:33:44:55:66",
				Kernel:     "/boot/kernel",
			},
		},
		{
			name: "full PXE config",
			config: &BootConfig{
				MACAddress: "de:ad:be:ef:00:01",
				Kernel:     "http://pxe.example.com/vmlinuz",
				Initrd:     "http://pxe.example.com/initrd",
				KernelArgs: "ip=dhcp root=nfs:10.0.0.1:/export/root",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()

			if err := svc.SetBootConfig(tc.config); err != nil {
				t.Fatalf("SetBootConfig: %v", err)
			}

			got, ok := svc.GetBootConfig(tc.config.MACAddress)
			if !ok {
				t.Fatal("GetBootConfig returned false")
			}
			if got.Kernel != tc.config.Kernel {
				t.Errorf("Kernel = %q, want %q", got.Kernel, tc.config.Kernel)
			}
			if got.Initrd != tc.config.Initrd {
				t.Errorf("Initrd = %q, want %q", got.Initrd, tc.config.Initrd)
			}
			if got.KernelArgs != tc.config.KernelArgs {
				t.Errorf("KernelArgs = %q, want %q", got.KernelArgs, tc.config.KernelArgs)
			}
		})
	}
}

func TestGetBootConfig_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	_, ok := svc.GetBootConfig("00:00:00:00:00:00")
	if ok {
		t.Error("expected false for non-existent boot config")
	}
}

func TestGetBootConfig_MACNormalization(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	cfg := &BootConfig{
		MACAddress: "AA:BB:CC:DD:EE:FF",
		Kernel:     "/vmlinuz",
	}
	if err := svc.SetBootConfig(cfg); err != nil {
		t.Fatalf("SetBootConfig: %v", err)
	}

	// Retrieve with lowercase -- should work due to normalization.
	got, ok := svc.GetBootConfig("aa:bb:cc:dd:ee:ff")
	if !ok {
		t.Fatal("GetBootConfig returned false for normalized MAC")
	}
	if got.Kernel != "/vmlinuz" {
		t.Errorf("Kernel mismatch after normalization")
	}
}

func TestClearBootConfig(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	cfg := &BootConfig{
		MACAddress: "aa:bb:cc:dd:ee:ff",
		Kernel:     "/vmlinuz",
	}
	if err := svc.SetBootConfig(cfg); err != nil {
		t.Fatalf("SetBootConfig: %v", err)
	}

	if err := svc.ClearBootConfig("aa:bb:cc:dd:ee:ff"); err != nil {
		t.Fatalf("ClearBootConfig: %v", err)
	}

	_, ok := svc.GetBootConfig("aa:bb:cc:dd:ee:ff")
	if ok {
		t.Error("boot config not cleared")
	}
}

func TestClearBootConfig_NonExistent(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	// Should not error even if the key does not exist (delete on missing key is a no-op).
	if err := svc.ClearBootConfig("00:00:00:00:00:00"); err != nil {
		t.Errorf("ClearBootConfig on non-existent returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PXE Boot Sequence Tests (full workflow)
// ---------------------------------------------------------------------------

func TestPXEBootSequence_FullWorkflow(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	// 1. Register machine with known MACs.
	m := &Machine{
		ID:           "pxe-test",
		Hostname:     "pxe-host",
		MACAddresses: []string{"aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02"},
		BMCAddress:   "192.168.1.10",
	}
	registerTestMachine(t, svc, m)

	// 2. No boot config yet.
	_, ok := svc.GetBootConfig("aa:bb:cc:dd:ee:01")
	if ok {
		t.Fatal("boot config exists before provisioning")
	}

	// 3. Provision -- should set boot configs for all MACs.
	req := &ProvisionRequest{
		MachineID:  "pxe-test",
		ImageURL:   "http://images.example.com/ubuntu.img",
		Kernel:     "/vmlinuz-pxe",
		Initrd:     "/initrd-pxe",
		KernelArgs: "auto=true",
	}
	if err := svc.Provision(context.Background(), req); err != nil {
		t.Fatalf("Provision: %v", err)
	}

	// 4. Verify boot configs exist for BOTH MACs.
	for _, mac := range m.MACAddresses {
		bc, ok := svc.GetBootConfig(mac)
		if !ok {
			t.Errorf("no boot config for MAC %s after provisioning", mac)
			continue
		}
		if bc.Kernel != "/vmlinuz-pxe" {
			t.Errorf("wrong kernel for %s: %q", mac, bc.Kernel)
		}
	}

	// 5. Machine status should be provisioning.
	got, _ := svc.GetMachine("pxe-test")
	if got.Status != StatusProvisioning {
		t.Errorf("Status = %v, want %v", got.Status, StatusProvisioning)
	}

	// 6. Complete provisioning successfully.
	if err := svc.ProvisioningComplete("pxe-test", true); err != nil {
		t.Fatalf("ProvisioningComplete: %v", err)
	}

	// 7. Status should be provisioned, state on.
	got, _ = svc.GetMachine("pxe-test")
	if got.Status != StatusProvisioned {
		t.Errorf("Status = %v, want %v", got.Status, StatusProvisioned)
	}
	if got.State != StateOn {
		t.Errorf("State = %v, want %v", got.State, StateOn)
	}
	if got.ProvisionedAt == nil {
		t.Error("ProvisionedAt not set after successful provisioning")
	}

	// 8. Boot configs should be cleared after provisioning completes.
	for _, mac := range m.MACAddresses {
		_, ok := svc.GetBootConfig(mac)
		if ok {
			t.Errorf("boot config for %s still present after provisioning complete", mac)
		}
	}
}

func TestPXEBootSequence_FailedProvisioning(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := testMachine("fail-pxe", "fail-host", "aa:bb:cc:dd:ee:10")
	registerTestMachine(t, svc, m)

	req := &ProvisionRequest{
		MachineID: "fail-pxe",
		ImageURL:  "http://images.example.com/broken.img",
		Kernel:    "/vmlinuz",
		Initrd:    "/initrd",
	}
	if err := svc.Provision(context.Background(), req); err != nil {
		t.Fatalf("Provision: %v", err)
	}

	// Mark as failed.
	if err := svc.ProvisioningComplete("fail-pxe", false); err != nil {
		t.Fatalf("ProvisioningComplete: %v", err)
	}

	got, _ := svc.GetMachine("fail-pxe")
	if got.Status != StatusError {
		t.Errorf("Status = %v, want %v", got.Status, StatusError)
	}
	if got.ProvisionedAt != nil {
		t.Error("ProvisionedAt should be nil on failed provisioning")
	}

	// Boot config should still be cleared even on failure.
	_, ok := svc.GetBootConfig("aa:bb:cc:dd:ee:10")
	if ok {
		t.Error("boot config not cleared after failed provisioning")
	}
}

// ---------------------------------------------------------------------------
// Provisioning State Machine Tests
// ---------------------------------------------------------------------------

func TestProvision_MachineNotFound(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	req := &ProvisionRequest{MachineID: "ghost"}
	err := svc.Provision(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for non-existent machine")
	}
}

func TestProvision_MachineNotAvailable(t *testing.T) {
	t.Parallel()

	statuses := []MachineStatus{
		StatusProvisioning,
		StatusProvisioned,
		StatusError,
		StatusMaintenance,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()

			m := testMachine("busy", "busy-host")
			registerTestMachine(t, svc, m)

			// Manually override status.
			svc.mu.Lock()
			m.Status = status
			svc.mu.Unlock()

			req := &ProvisionRequest{MachineID: "busy", ImageURL: "http://img"}
			err := svc.Provision(context.Background(), req)
			if err == nil {
				t.Errorf("expected error for status %v", status)
			}
		})
	}
}

func TestProvision_WithoutBMCAddress(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := testMachine("no-bmc", "no-bmc-host")
	m.BMCAddress = ""
	registerTestMachine(t, svc, m)

	req := &ProvisionRequest{
		MachineID: "no-bmc",
		ImageURL:  "http://img",
		Kernel:    "/vmlinuz",
		Initrd:    "/initrd",
	}
	// Should succeed even without BMC (just skips power cycle).
	if err := svc.Provision(context.Background(), req); err != nil {
		t.Fatalf("Provision without BMC should succeed: %v", err)
	}
}

func TestProvisioningComplete_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	err := svc.ProvisioningComplete("nonexistent", true)
	if err == nil {
		t.Fatal("expected error for non-existent machine")
	}
}

func TestProvisioningStateMachine_FullCycle(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := testMachine("sm-test", "state-machine-host", "de:ad:be:ef:00:01")
	registerTestMachine(t, svc, m)

	// available -> provisioning
	req := &ProvisionRequest{
		MachineID: "sm-test",
		ImageURL:  "http://img",
		Kernel:    "/k",
		Initrd:    "/i",
	}
	if err := svc.Provision(context.Background(), req); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	got, _ := svc.GetMachine("sm-test")
	if got.Status != StatusProvisioning {
		t.Fatalf("expected provisioning, got %v", got.Status)
	}

	// provisioning -> provisioned
	if err := svc.ProvisioningComplete("sm-test", true); err != nil {
		t.Fatalf("ProvisioningComplete: %v", err)
	}
	got, _ = svc.GetMachine("sm-test")
	if got.Status != StatusProvisioned {
		t.Fatalf("expected provisioned, got %v", got.Status)
	}

	// Cannot re-provision a provisioned machine.
	err := svc.Provision(context.Background(), req)
	if err == nil {
		t.Fatal("expected error re-provisioning a provisioned machine")
	}
}

// ---------------------------------------------------------------------------
// Hardware Inventory Tests
// ---------------------------------------------------------------------------

func TestHardwareInfo_Registration(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := &Machine{
		ID:           "hw-test",
		Hostname:     "hardware-host",
		MACAddresses: []string{"aa:bb:cc:dd:ee:ff"},
		Hardware: HardwareInfo{
			Manufacturer: "Dell",
			Model:        "PowerEdge R740",
			CPUs: []CPU{
				{Model: "Xeon Gold 6248", Cores: 20, Threads: 40, SpeedMHz: 2500},
				{Model: "Xeon Gold 6248", Cores: 20, Threads: 40, SpeedMHz: 2500},
			},
			Memory: Memory{
				TotalBytes: 256 * 1024 * 1024 * 1024, // 256 GB
				Type:       "DDR4",
				SpeedMHz:   2933,
			},
			Disks: []Disk{
				{Name: "sda", SizeBytes: 480 * 1024 * 1024 * 1024, Type: "ssd", Model: "Intel S4610", Rotational: false},
				{Name: "sdb", SizeBytes: 4 * 1024 * 1024 * 1024 * 1024, Type: "hdd", Model: "ST4000NM", Serial: "ABC123", Rotational: true},
				{Name: "nvme0n1", SizeBytes: 1024 * 1024 * 1024 * 1024, Type: "nvme", Model: "Samsung 970 EVO", Rotational: false},
			},
			NICs: []NIC{
				{Name: "eth0", MACAddress: "aa:bb:cc:dd:ee:ff", Speed: 25000, Driver: "mlx5_core"},
				{Name: "eth1", MACAddress: "aa:bb:cc:dd:ee:00", Speed: 10000, Driver: "ixgbe"},
			},
			BMCType: "IPMI",
		},
	}
	registerTestMachine(t, svc, m)

	got, ok := svc.GetMachine("hw-test")
	if !ok {
		t.Fatal("machine not found")
	}

	// Verify hardware inventory fields.
	hw := got.Hardware
	if hw.Manufacturer != "Dell" {
		t.Errorf("Manufacturer = %q, want %q", hw.Manufacturer, "Dell")
	}
	if hw.Model != "PowerEdge R740" {
		t.Errorf("Model = %q, want %q", hw.Model, "PowerEdge R740")
	}
	if len(hw.CPUs) != 2 {
		t.Errorf("CPUs count = %d, want 2", len(hw.CPUs))
	}
	if hw.CPUs[0].Cores != 20 {
		t.Errorf("CPU cores = %d, want 20", hw.CPUs[0].Cores)
	}
	if hw.Memory.TotalBytes != 256*1024*1024*1024 {
		t.Errorf("Memory = %d, want 256GB", hw.Memory.TotalBytes)
	}
	if len(hw.Disks) != 3 {
		t.Errorf("Disks = %d, want 3", len(hw.Disks))
	}
	if hw.Disks[2].Type != "nvme" {
		t.Errorf("Disk 2 type = %q, want nvme", hw.Disks[2].Type)
	}
	if len(hw.NICs) != 2 {
		t.Errorf("NICs = %d, want 2", len(hw.NICs))
	}
	if hw.BMCType != "IPMI" {
		t.Errorf("BMCType = %q, want IPMI", hw.BMCType)
	}
}

func TestHardwareInfo_EmptyDefaults(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := testMachine("hw-empty", "empty-hw")
	registerTestMachine(t, svc, m)

	got, _ := svc.GetMachine("hw-empty")
	if got.Hardware.Manufacturer != "" {
		t.Error("expected empty manufacturer")
	}
	if len(got.Hardware.CPUs) != 0 {
		t.Error("expected no CPUs")
	}
	if len(got.Hardware.Disks) != 0 {
		t.Error("expected no disks")
	}
}

// ---------------------------------------------------------------------------
// IPMI / BMC Credentials Tests
// ---------------------------------------------------------------------------

func TestSetBMCCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		creds *BMCCredentials
	}{
		{
			name: "IPMI credentials",
			creds: &BMCCredentials{
				MachineID: "m-1",
				Username:  "admin",
				Password:  "secret123",
				Type:      "IPMI",
			},
		},
		{
			name: "Redfish credentials",
			creds: &BMCCredentials{
				MachineID: "m-2",
				Username:  "root",
				Password:  "redfish-pass",
				Type:      "Redfish",
			},
		},
		{
			name: "iLO credentials",
			creds: &BMCCredentials{
				MachineID: "m-3",
				Username:  "ilo-admin",
				Password:  "ilo-pass",
				Type:      "iLO",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()
			err := svc.SetBMCCredentials(tc.creds)
			if err != nil {
				t.Fatalf("SetBMCCredentials: %v", err)
			}

			svc.mu.RLock()
			stored, ok := svc.credentials[tc.creds.MachineID]
			svc.mu.RUnlock()

			if !ok {
				t.Fatal("credentials not stored")
			}
			if stored.Username != tc.creds.Username {
				t.Errorf("Username = %q, want %q", stored.Username, tc.creds.Username)
			}
			if stored.Password != tc.creds.Password {
				t.Errorf("Password not stored correctly")
			}
			if stored.Type != tc.creds.Type {
				t.Errorf("Type = %q, want %q", stored.Type, tc.creds.Type)
			}
		})
	}
}

func TestSetBMCCredentials_Overwrite(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	creds1 := &BMCCredentials{MachineID: "m-1", Username: "old", Password: "old-pass", Type: "IPMI"}
	creds2 := &BMCCredentials{MachineID: "m-1", Username: "new", Password: "new-pass", Type: "Redfish"}

	_ = svc.SetBMCCredentials(creds1)
	_ = svc.SetBMCCredentials(creds2)

	svc.mu.RLock()
	stored := svc.credentials["m-1"]
	svc.mu.RUnlock()

	if stored.Username != "new" {
		t.Errorf("expected overwritten username, got %q", stored.Username)
	}
	if stored.Type != "Redfish" {
		t.Errorf("expected overwritten type, got %q", stored.Type)
	}
}

// ---------------------------------------------------------------------------
// Power Management Tests (PowerOn / PowerOff / Reboot)
// ---------------------------------------------------------------------------

func TestPowerOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		register  bool
		wantErr   bool
		wantState MachineState
	}{
		{"existing machine", "pw-on", true, false, StateOn},
		{"non-existent machine", "ghost", false, true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()
			if tc.register {
				registerTestMachine(t, svc, testMachine(tc.machineID, "host"))
			}

			err := svc.PowerOn(context.Background(), tc.machineID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("PowerOn error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				got, _ := svc.GetMachine(tc.machineID)
				if got.State != tc.wantState {
					t.Errorf("State = %v, want %v", got.State, tc.wantState)
				}
			}
		})
	}
}

func TestPowerOff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		register  bool
		wantErr   bool
		wantState MachineState
	}{
		{"existing machine", "pw-off", true, false, StateOff},
		{"non-existent machine", "ghost", false, true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()
			if tc.register {
				registerTestMachine(t, svc, testMachine(tc.machineID, "host"))
			}

			err := svc.PowerOff(context.Background(), tc.machineID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("PowerOff error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				got, _ := svc.GetMachine(tc.machineID)
				if got.State != tc.wantState {
					t.Errorf("State = %v, want %v", got.State, tc.wantState)
				}
			}
		})
	}
}

func TestReboot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		register  bool
		wantErr   bool
		wantState MachineState
	}{
		{"existing machine", "reboot", true, false, StateRebooting},
		{"non-existent machine", "ghost", false, true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()
			if tc.register {
				registerTestMachine(t, svc, testMachine(tc.machineID, "host"))
			}

			err := svc.Reboot(context.Background(), tc.machineID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Reboot error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				got, _ := svc.GetMachine(tc.machineID)
				if got.State != tc.wantState {
					t.Errorf("State = %v, want %v", got.State, tc.wantState)
				}
			}
		})
	}
}

func TestPowerStateTransitions(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	ctx := context.Background()

	m := testMachine("transitions", "transition-host")
	registerTestMachine(t, svc, m)

	// Initial state: unknown.
	got, _ := svc.GetMachine("transitions")
	if got.State != StateUnknown {
		t.Fatalf("initial state = %v, want unknown", got.State)
	}

	// Unknown -> On
	if err := svc.PowerOn(ctx, "transitions"); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}
	got, _ = svc.GetMachine("transitions")
	if got.State != StateOn {
		t.Fatalf("state = %v, want on", got.State)
	}

	// On -> Rebooting
	if err := svc.Reboot(ctx, "transitions"); err != nil {
		t.Fatalf("Reboot: %v", err)
	}
	got, _ = svc.GetMachine("transitions")
	if got.State != StateRebooting {
		t.Fatalf("state = %v, want rebooting", got.State)
	}

	// Rebooting -> Off
	if err := svc.PowerOff(ctx, "transitions"); err != nil {
		t.Fatalf("PowerOff: %v", err)
	}
	got, _ = svc.GetMachine("transitions")
	if got.State != StateOff {
		t.Fatalf("state = %v, want off", got.State)
	}

	// Off -> On
	if err := svc.PowerOn(ctx, "transitions"); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}
	got, _ = svc.GetMachine("transitions")
	if got.State != StateOn {
		t.Fatalf("state = %v, want on", got.State)
	}
}

// ---------------------------------------------------------------------------
// Heartbeat Tests
// ---------------------------------------------------------------------------

func TestHeartbeat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		register  bool
		wantErr   bool
	}{
		{"existing machine", "hb-1", true, false},
		{"non-existent machine", "hb-ghost", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestServiceDefault()
			if tc.register {
				registerTestMachine(t, svc, testMachine(tc.machineID, "host"))
			}
			err := svc.Heartbeat(tc.machineID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Heartbeat error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestHeartbeat_UpdatesLastSeen(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := testMachine("hb-time", "host")
	registerTestMachine(t, svc, m)

	got1, _ := svc.GetMachine("hb-time")
	first := got1.LastSeen

	time.Sleep(2 * time.Millisecond)

	if err := svc.Heartbeat("hb-time"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	got2, _ := svc.GetMachine("hb-time")
	if !got2.LastSeen.After(first) {
		t.Error("LastSeen was not updated by heartbeat")
	}
}

func TestCheckHeartbeats_MarksStale(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PXEServerAddress:  "0.0.0.0:69",
		TFTPRoot:          "/tmp",
		DiscoveryInterval: time.Hour, // we will call checkHeartbeats manually
		HeartbeatTimeout:  10 * time.Millisecond,
		WotanTopic:       "test",
	}
	svc := newTestService(cfg)

	m := testMachine("stale", "stale-host")
	registerTestMachine(t, svc, m)

	// Set machine to "on".
	svc.mu.Lock()
	m.State = StateOn
	m.LastSeen = time.Now().Add(-1 * time.Second) // well past timeout
	svc.mu.Unlock()

	svc.checkHeartbeats()

	got, _ := svc.GetMachine("stale")
	if got.State != StateUnknown {
		t.Errorf("State = %v, want unknown after heartbeat timeout", got.State)
	}
}

func TestCheckHeartbeats_DoesNotAffectOffMachines(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PXEServerAddress:  "0.0.0.0:69",
		TFTPRoot:          "/tmp",
		DiscoveryInterval: time.Hour,
		HeartbeatTimeout:  10 * time.Millisecond,
		WotanTopic:       "test",
	}
	svc := newTestService(cfg)

	m := testMachine("off-stale", "off-host")
	registerTestMachine(t, svc, m)

	// Machine is off and stale.
	svc.mu.Lock()
	m.State = StateOff
	m.LastSeen = time.Now().Add(-1 * time.Second)
	svc.mu.Unlock()

	svc.checkHeartbeats()

	got, _ := svc.GetMachine("off-stale")
	if got.State != StateOff {
		t.Errorf("State = %v, want off (should not change non-on machines)", got.State)
	}
}

func TestCheckHeartbeats_RecentMachineStaysOn(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PXEServerAddress:  "0.0.0.0:69",
		TFTPRoot:          "/tmp",
		DiscoveryInterval: time.Hour,
		HeartbeatTimeout:  5 * time.Second,
		WotanTopic:       "test",
	}
	svc := newTestService(cfg)

	m := testMachine("recent", "recent-host")
	registerTestMachine(t, svc, m)

	svc.mu.Lock()
	m.State = StateOn
	m.LastSeen = time.Now() // just now
	svc.mu.Unlock()

	svc.checkHeartbeats()

	got, _ := svc.GetMachine("recent")
	if got.State != StateOn {
		t.Errorf("State = %v, want on (machine is recent)", got.State)
	}
}

// ---------------------------------------------------------------------------
// Stats Tests
// ---------------------------------------------------------------------------

func TestStats_Empty(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	stats := svc.Stats()

	if stats["total_machines"].(int) != 0 {
		t.Errorf("total_machines = %v, want 0", stats["total_machines"])
	}
	if stats["pending_boots"].(int) != 0 {
		t.Errorf("pending_boots = %v, want 0", stats["pending_boots"])
	}
}

func TestStats_WithMachines(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	registerTestMachine(t, svc, testMachine("s1", "h1", "aa:bb:cc:00:00:01"))
	registerTestMachine(t, svc, testMachine("s2", "h2", "aa:bb:cc:00:00:02"))
	registerTestMachine(t, svc, testMachine("s3", "h3", "aa:bb:cc:00:00:03"))

	// Put one machine in error status.
	svc.mu.Lock()
	svc.machines["s3"].Status = StatusError
	svc.mu.Unlock()

	// Add a boot config.
	_ = svc.SetBootConfig(&BootConfig{MACAddress: "ff:ff:ff:ff:ff:ff", Kernel: "/k"})

	stats := svc.Stats()

	if stats["total_machines"].(int) != 3 {
		t.Errorf("total_machines = %v, want 3", stats["total_machines"])
	}
	if stats["pending_boots"].(int) != 1 {
		t.Errorf("pending_boots = %v, want 1", stats["pending_boots"])
	}

	byStatus := stats["by_status"].(map[string]int)
	if byStatus["available"] != 2 {
		t.Errorf("available count = %d, want 2", byStatus["available"])
	}
	if byStatus["error"] != 1 {
		t.Errorf("error count = %d, want 1", byStatus["error"])
	}

	byState := stats["by_state"].(map[string]int)
	if byState["unknown"] != 3 {
		t.Errorf("unknown state count = %d, want 3", byState["unknown"])
	}
}

// ---------------------------------------------------------------------------
// MAC normalization
// ---------------------------------------------------------------------------

func TestNormalizeMAC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase colon", "aa:bb:cc:dd:ee:ff", "aa:bb:cc:dd:ee:ff"},
		{"uppercase colon", "AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"},
		{"mixed case", "Aa:Bb:Cc:Dd:Ee:Ff", "aa:bb:cc:dd:ee:ff"},
		{"dash separator", "AA-BB-CC-DD-EE-FF", "aa:bb:cc:dd:ee:ff"},
		{"invalid returns input", "not-a-mac-address", "not-a-mac-address"},
		{"empty returns empty", "", ""},
		{"short returns input", "aa:bb", "aa:bb"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeMAC(tc.input)
			if got != tc.want {
				t.Errorf("normalizeMAC(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge Cases and Error Paths
// ---------------------------------------------------------------------------

func TestProvision_EmptyMACAddresses(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := &Machine{
		ID:           "no-mac",
		Hostname:     "no-mac-host",
		MACAddresses: []string{}, // no MACs
	}
	registerTestMachine(t, svc, m)

	req := &ProvisionRequest{
		MachineID: "no-mac",
		ImageURL:  "http://img",
		Kernel:    "/k",
	}
	// Should not error; just no boot configs to set.
	if err := svc.Provision(context.Background(), req); err != nil {
		t.Fatalf("Provision with no MACs should succeed: %v", err)
	}
}

func TestProvisioningComplete_NoMACs(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()

	m := &Machine{
		ID:           "no-mac-complete",
		Hostname:     "host",
		MACAddresses: []string{},
	}
	registerTestMachine(t, svc, m)

	svc.mu.Lock()
	m.Status = StatusProvisioning
	svc.mu.Unlock()

	err := svc.ProvisioningComplete("no-mac-complete", true)
	if err != nil {
		t.Fatalf("ProvisioningComplete with no MACs: %v", err)
	}
}

func TestMultipleMachinesSameOperations(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	ctx := context.Background()

	// Register a fleet.
	for i := 0; i < 50; i++ {
		m := testMachine(
			fmt.Sprintf("fleet-%03d", i),
			fmt.Sprintf("node-%03d", i),
			fmt.Sprintf("aa:bb:cc:dd:%02x:%02x", i/256, i%256),
		)
		registerTestMachine(t, svc, m)
	}

	// Verify all registered.
	all := svc.ListMachines("")
	if len(all) != 50 {
		t.Fatalf("registered %d machines, want 50", len(all))
	}

	// Power on all.
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("fleet-%03d", i)
		if err := svc.PowerOn(ctx, id); err != nil {
			t.Fatalf("PowerOn %s: %v", id, err)
		}
	}

	// Heartbeat all.
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("fleet-%03d", i)
		if err := svc.Heartbeat(id); err != nil {
			t.Fatalf("Heartbeat %s: %v", id, err)
		}
	}

	// Stats.
	stats := svc.Stats()
	if stats["total_machines"].(int) != 50 {
		t.Errorf("total_machines = %v, want 50", stats["total_machines"])
	}
}

// ---------------------------------------------------------------------------
// Concurrent / Race-safety Tests
// ---------------------------------------------------------------------------

func TestConcurrentRegisterAndGet(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n * 2)

	// Concurrent registrations.
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			m := testMachine(
				fmt.Sprintf("conc-%d", i),
				fmt.Sprintf("host-%d", i),
				fmt.Sprintf("aa:bb:cc:00:%02x:%02x", i/256, i%256),
			)
			_ = svc.RegisterMachine(m)
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			svc.GetMachine(fmt.Sprintf("conc-%d", i))
			svc.ListMachines("")
			svc.Stats()
		}(i)
	}

	wg.Wait()

	all := svc.ListMachines("")
	if len(all) != n {
		t.Errorf("expected %d machines, got %d", n, len(all))
	}
}

func TestConcurrentPowerOperations(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	ctx := context.Background()
	const n = 50

	// Pre-register machines.
	for i := 0; i < n; i++ {
		m := testMachine(
			fmt.Sprintf("power-%d", i),
			fmt.Sprintf("host-%d", i),
		)
		registerTestMachine(t, svc, m)
	}

	var wg sync.WaitGroup
	wg.Add(n * 3)

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("power-%d", i)
		go func() {
			defer wg.Done()
			_ = svc.PowerOn(ctx, id)
		}()
		go func() {
			defer wg.Done()
			_ = svc.PowerOff(ctx, id)
		}()
		go func() {
			defer wg.Done()
			_ = svc.Reboot(ctx, id)
		}()
	}

	wg.Wait()
	// No panics or race conditions means pass.
}

func TestConcurrentHeartbeats(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	const n = 100

	m := testMachine("hb-race", "host")
	registerTestMachine(t, svc, m)

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = svc.Heartbeat("hb-race")
		}()
	}

	wg.Wait()
}

func TestConcurrentBootConfigs(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n * 3) // set + get + clear

	for i := 0; i < n; i++ {
		mac := fmt.Sprintf("aa:bb:cc:dd:%02x:%02x", i/256, i%256)

		go func(mac string) {
			defer wg.Done()
			_ = svc.SetBootConfig(&BootConfig{MACAddress: mac, Kernel: "/k"})
		}(mac)

		go func(mac string) {
			defer wg.Done()
			svc.GetBootConfig(mac)
		}(mac)

		go func(mac string) {
			defer wg.Done()
			_ = svc.ClearBootConfig(mac)
		}(mac)
	}

	wg.Wait()
}

func TestConcurrentProvisionAndComplete(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	ctx := context.Background()
	const n = 20

	// Register machines.
	for i := 0; i < n; i++ {
		m := testMachine(
			fmt.Sprintf("prov-%d", i),
			fmt.Sprintf("host-%d", i),
			fmt.Sprintf("dd:ee:ff:00:%02x:%02x", i/256, i%256),
		)
		registerTestMachine(t, svc, m)
	}

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("prov-%d", i)
			req := &ProvisionRequest{
				MachineID: id,
				ImageURL:  "http://img",
				Kernel:    "/k",
				Initrd:    "/i",
			}
			err := svc.Provision(ctx, req)
			if err != nil {
				// Might fail if already provisioning (race between goroutines
				// that got the same machine); that is expected.
				return
			}
			_ = svc.ProvisioningComplete(id, true)
		}(i)
	}

	wg.Wait()
}

func TestConcurrentMixedOperations(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n * 6)

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("mix-%d", i)
		mac := fmt.Sprintf("cc:cc:cc:00:%02x:%02x", i/256, i%256)

		go func() {
			defer wg.Done()
			_ = svc.RegisterMachine(testMachine(id, "host", mac))
		}()
		go func() {
			defer wg.Done()
			svc.GetMachine(id)
		}()
		go func() {
			defer wg.Done()
			svc.GetMachineByMAC(mac)
		}()
		go func() {
			defer wg.Done()
			svc.ListMachines("")
		}()
		go func() {
			defer wg.Done()
			_ = svc.Heartbeat(id) // may fail if not yet registered
		}()
		go func() {
			defer wg.Done()
			svc.Stats()
		}()
	}

	wg.Wait()
}

func TestConcurrentBMCCredentials(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = svc.SetBMCCredentials(&BMCCredentials{
				MachineID: fmt.Sprintf("bmc-%d", i),
				Username:  "admin",
				Password:  "pass",
				Type:      "IPMI",
			})
		}(i)
	}

	wg.Wait()

	svc.mu.RLock()
	count := len(svc.credentials)
	svc.mu.RUnlock()

	if count != n {
		t.Errorf("credentials count = %d, want %d", count, n)
	}
}

// ---------------------------------------------------------------------------
// Integration-style: full provisioning lifecycle
// ---------------------------------------------------------------------------

func TestFullLifecycle_MultiMachine(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	ctx := context.Background()

	// 1. Register three machines.
	machines := []*Machine{
		{ID: "lc-1", Hostname: "compute-1", MACAddresses: []string{"aa:00:00:00:00:01"}, BMCAddress: "10.0.0.1"},
		{ID: "lc-2", Hostname: "compute-2", MACAddresses: []string{"aa:00:00:00:00:02"}, BMCAddress: "10.0.0.2"},
		{ID: "lc-3", Hostname: "storage-1", MACAddresses: []string{"aa:00:00:00:00:03"}},
	}
	for _, m := range machines {
		registerTestMachine(t, svc, m)
	}

	// 2. Set BMC credentials for machines with BMC.
	_ = svc.SetBMCCredentials(&BMCCredentials{MachineID: "lc-1", Username: "admin", Password: "p1", Type: "IPMI"})
	_ = svc.SetBMCCredentials(&BMCCredentials{MachineID: "lc-2", Username: "admin", Password: "p2", Type: "Redfish"})

	// 3. Provision first two.
	for _, id := range []string{"lc-1", "lc-2"} {
		req := &ProvisionRequest{
			MachineID: id,
			ImageURL:  "http://images/ubuntu.img",
			Kernel:    "/vmlinuz",
			Initrd:    "/initrd",
			KernelArgs: "auto",
		}
		if err := svc.Provision(ctx, req); err != nil {
			t.Fatalf("Provision %s: %v", id, err)
		}
	}

	// 4. Third machine should still be available.
	m3, _ := svc.GetMachine("lc-3")
	if m3.Status != StatusAvailable {
		t.Errorf("lc-3 status = %v, want available", m3.Status)
	}

	// 5. Complete provisioning on lc-1 (success) and lc-2 (failure).
	if err := svc.ProvisioningComplete("lc-1", true); err != nil {
		t.Fatalf("ProvisioningComplete lc-1: %v", err)
	}
	if err := svc.ProvisioningComplete("lc-2", false); err != nil {
		t.Fatalf("ProvisioningComplete lc-2: %v", err)
	}

	// 6. Verify final states.
	m1, _ := svc.GetMachine("lc-1")
	if m1.Status != StatusProvisioned || m1.State != StateOn {
		t.Errorf("lc-1: status=%v state=%v, want provisioned/on", m1.Status, m1.State)
	}

	m2, _ := svc.GetMachine("lc-2")
	if m2.Status != StatusError {
		t.Errorf("lc-2: status=%v, want error", m2.Status)
	}

	// 7. Verify stats.
	stats := svc.Stats()
	if stats["total_machines"].(int) != 3 {
		t.Errorf("total_machines = %v, want 3", stats["total_machines"])
	}
}

// ---------------------------------------------------------------------------
// Edge: calling operations on freshly started service
// ---------------------------------------------------------------------------

func TestOperationsOnEmptyService(t *testing.T) {
	t.Parallel()
	svc := newTestServiceDefault()
	ctx := context.Background()

	// All these should not panic and should return appropriate results.
	_, ok := svc.GetMachine("nonexistent")
	if ok {
		t.Error("GetMachine on empty service should return false")
	}

	_, ok = svc.GetMachineByMAC("aa:bb:cc:dd:ee:ff")
	if ok {
		t.Error("GetMachineByMAC on empty service should return false")
	}

	machines := svc.ListMachines("")
	if len(machines) != 0 {
		t.Error("ListMachines on empty service should return empty")
	}

	_, ok = svc.GetBootConfig("aa:bb:cc:dd:ee:ff")
	if ok {
		t.Error("GetBootConfig on empty service should return false")
	}

	if err := svc.ClearBootConfig("aa:bb:cc:dd:ee:ff"); err != nil {
		t.Errorf("ClearBootConfig on empty service: %v", err)
	}

	if err := svc.Heartbeat("nonexistent"); err == nil {
		t.Error("Heartbeat on empty service should error")
	}

	if err := svc.PowerOn(ctx, "nonexistent"); err == nil {
		t.Error("PowerOn on empty service should error")
	}

	if err := svc.PowerOff(ctx, "nonexistent"); err == nil {
		t.Error("PowerOff on empty service should error")
	}

	if err := svc.Reboot(ctx, "nonexistent"); err == nil {
		t.Error("Reboot on empty service should error")
	}

	err := svc.Provision(ctx, &ProvisionRequest{MachineID: "nonexistent"})
	if err == nil {
		t.Error("Provision on empty service should error")
	}

	if err := svc.ProvisioningComplete("nonexistent", true); err == nil {
		t.Error("ProvisioningComplete on empty service should error")
	}

	stats := svc.Stats()
	if stats["total_machines"].(int) != 0 {
		t.Error("Stats on empty service should show zero machines")
	}
}

// ---------------------------------------------------------------------------
// Type / constant verification
// ---------------------------------------------------------------------------

func TestMachineStatusConstants(t *testing.T) {
	t.Parallel()

	statuses := map[MachineStatus]string{
		StatusAvailable:    "available",
		StatusProvisioning: "provisioning",
		StatusProvisioned:  "provisioned",
		StatusError:        "error",
		StatusMaintenance:  "maintenance",
	}

	for status, want := range statuses {
		if string(status) != want {
			t.Errorf("Status %v = %q, want %q", status, string(status), want)
		}
	}
}

func TestMachineStateConstants(t *testing.T) {
	t.Parallel()

	states := map[MachineState]string{
		StateOn:       "on",
		StateOff:      "off",
		StateRebooting: "rebooting",
		StateUnknown:  "unknown",
	}

	for state, want := range states {
		if string(state) != want {
			t.Errorf("State %v = %q, want %q", state, string(state), want)
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkRegisterMachine(b *testing.B) {
	svc := newTestServiceDefault()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := &Machine{
			ID:           fmt.Sprintf("bench-%d", i),
			Hostname:     fmt.Sprintf("host-%d", i),
			MACAddresses: []string{fmt.Sprintf("aa:bb:cc:%02x:%02x:%02x", i/(256*256), (i/256)%256, i%256)},
		}
		_ = svc.RegisterMachine(m)
	}
}

func BenchmarkGetMachine(b *testing.B) {
	svc := newTestServiceDefault()
	for i := 0; i < 1000; i++ {
		_ = svc.RegisterMachine(&Machine{
			ID:           fmt.Sprintf("bench-%d", i),
			Hostname:     fmt.Sprintf("host-%d", i),
			MACAddresses: []string{"aa:bb:cc:dd:ee:ff"},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GetMachine(fmt.Sprintf("bench-%d", i%1000))
	}
}

func BenchmarkStats(b *testing.B) {
	svc := newTestServiceDefault()
	for i := 0; i < 1000; i++ {
		_ = svc.RegisterMachine(&Machine{
			ID:           fmt.Sprintf("bench-%d", i),
			Hostname:     fmt.Sprintf("host-%d", i),
			MACAddresses: []string{"aa:bb:cc:dd:ee:ff"},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Stats()
	}
}

func BenchmarkHeartbeat(b *testing.B) {
	svc := newTestServiceDefault()
	_ = svc.RegisterMachine(&Machine{
		ID:           "bench-hb",
		Hostname:     "host",
		MACAddresses: []string{"aa:bb:cc:dd:ee:ff"},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.Heartbeat("bench-hb")
	}
}

func BenchmarkNormalizeMAC(b *testing.B) {
	for i := 0; i < b.N; i++ {
		normalizeMAC("AA:BB:CC:DD:EE:FF")
	}
}
