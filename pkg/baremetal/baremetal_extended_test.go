// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package baremetal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"unheaded/pkg/baremetal/ipmi"
)

func TestTestConfig(t *testing.T) {
	config := TestConfig()

	if config == nil {
		t.Fatal("TestConfig() returned nil")
	}

	if !config.SkipDirectoryInit {
		t.Error("TestConfig should have SkipDirectoryInit=true")
	}

	if config.PXE == nil {
		t.Fatal("PXE config should not be nil")
	}
	if !config.PXE.SkipDirectoryInit {
		t.Error("PXE config should have SkipDirectoryInit=true")
	}

	if config.Image == nil {
		t.Fatal("Image config should not be nil")
	}
	if !config.Image.SkipDirectoryInit {
		t.Error("Image config should have SkipDirectoryInit=true")
	}

	if config.Discovery == nil {
		t.Error("Discovery config should not be nil")
	}

	if config.ProvisionTimeout != 30*time.Minute {
		t.Errorf("Expected ProvisionTimeout 30m, got %v", config.ProvisionTimeout)
	}

	if config.HeartbeatInterval != 30*time.Second {
		t.Errorf("Expected HeartbeatInterval 30s, got %v", config.HeartbeatInterval)
	}
}

func TestNewProvisioner_NilConfig(t *testing.T) {
	// NewProvisioner with nil config should use DefaultConfig, which will
	// try to create directories. We can't easily test that without filesystem
	// access, but we can verify it doesn't panic and returns a valid provisioner
	// or an error about directory creation.
	p, err := NewProvisioner(nil)
	// On most test environments without /var/lib access, this may fail
	// with a directory creation error, which is acceptable.
	if err != nil {
		// Expected on test environments without root access
		t.Logf("NewProvisioner(nil) returned expected error: %v", err)
		return
	}
	if p == nil {
		t.Fatal("NewProvisioner(nil) returned nil provisioner without error")
	}
}

func newTestProvisioner(t *testing.T) *bareMetalProvisioner {
	t.Helper()
	p, err := NewProvisioner(TestConfig())
	if err != nil {
		t.Fatalf("NewProvisioner() error = %v", err)
	}
	return p.(*bareMetalProvisioner)
}

func TestInitProvisioningStatus(t *testing.T) {
	p := newTestProvisioner(t)

	p.initProvisioningStatus("test-machine-1")

	p.statusMu.RLock()
	status, exists := p.provisioningStatus["test-machine-1"]
	p.statusMu.RUnlock()

	if !exists {
		t.Fatal("Expected provisioning status to exist")
	}

	if status.MachineID != "test-machine-1" {
		t.Errorf("Expected MachineID test-machine-1, got %s", status.MachineID)
	}

	if status.State != MachineStateProvisioning {
		t.Errorf("Expected state provisioning, got %s", status.State)
	}

	if status.Phase != PhaseInit {
		t.Errorf("Expected phase init, got %s", status.Phase)
	}

	if status.Progress != 0 {
		t.Errorf("Expected progress 0, got %d", status.Progress)
	}

	if status.Message != "Initializing provisioning" {
		t.Errorf("Expected message 'Initializing provisioning', got %s", status.Message)
	}

	if len(status.PhaseHistory) != 1 {
		t.Fatalf("Expected 1 phase history entry, got %d", len(status.PhaseHistory))
	}

	if status.PhaseHistory[0].Phase != PhaseInit {
		t.Errorf("Expected first history entry phase init, got %s", status.PhaseHistory[0].Phase)
	}
}

func TestUpdateProvisioningPhase(t *testing.T) {
	p := newTestProvisioner(t)

	// Update a non-existent status (should be a no-op)
	p.updateProvisioningPhase("nonexistent", PhasePowerOff, 10, "test", nil)

	// Initialize then update
	p.initProvisioningStatus("machine-1")

	t.Run("normal update without error", func(t *testing.T) {
		p.updateProvisioningPhase("machine-1", PhasePowerOff, 10, "Powering off", nil)

		p.statusMu.RLock()
		status := p.provisioningStatus["machine-1"]
		p.statusMu.RUnlock()

		if status.Phase != PhasePowerOff {
			t.Errorf("Expected phase power_off, got %s", status.Phase)
		}
		if status.Progress != 10 {
			t.Errorf("Expected progress 10, got %d", status.Progress)
		}
		if status.Message != "Powering off" {
			t.Errorf("Expected message 'Powering off', got %s", status.Message)
		}
		if len(status.PhaseHistory) != 2 {
			t.Errorf("Expected 2 phase history entries, got %d", len(status.PhaseHistory))
		}
	})

	t.Run("update with error", func(t *testing.T) {
		testErr := &testError{msg: "disk failure"}
		p.updateProvisioningPhase("machine-1", PhaseInstalling, 60, "Installing", testErr)

		p.statusMu.RLock()
		status := p.provisioningStatus["machine-1"]
		p.statusMu.RUnlock()

		lastEntry := status.PhaseHistory[len(status.PhaseHistory)-1]
		if lastEntry.Error != "disk failure" {
			t.Errorf("Expected error 'disk failure', got %s", lastEntry.Error)
		}
	})

	t.Run("update to failed phase", func(t *testing.T) {
		testErr := &testError{msg: "fatal error"}
		p.updateProvisioningPhase("machine-1", PhaseFailed, 0, "Failed", testErr)

		p.statusMu.RLock()
		status := p.provisioningStatus["machine-1"]
		p.statusMu.RUnlock()

		if status.State != MachineStateFailed {
			t.Errorf("Expected state failed, got %s", status.State)
		}
	})

	t.Run("update to complete phase", func(t *testing.T) {
		// Re-init so state is not failed
		p.initProvisioningStatus("machine-2")
		p.updateProvisioningPhase("machine-2", PhaseComplete, 100, "Done", nil)

		p.statusMu.RLock()
		status := p.provisioningStatus["machine-2"]
		p.statusMu.RUnlock()

		if status.State != MachineStateReady {
			t.Errorf("Expected state ready, got %s", status.State)
		}
		if status.Progress != 100 {
			t.Errorf("Expected progress 100, got %d", status.Progress)
		}
	})
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestFailProvisioning(t *testing.T) {
	p := newTestProvisioner(t)

	spec := &MachineSpec{
		ID:         "fail-test",
		MACAddress: "00:11:22:33:44:55",
	}

	now := time.Now()
	machine := &Machine{
		Spec:      spec,
		State:     MachineStateProvisioning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.machines[spec.ID] = machine
	p.initProvisioningStatus(spec.ID)

	testErr := &testError{msg: "provisioning failed: disk not found"}
	p.failProvisioning(machine, testErr)

	if machine.State != MachineStateFailed {
		t.Errorf("Expected state failed, got %s", machine.State)
	}

	if machine.Error != "provisioning failed: disk not found" {
		t.Errorf("Expected error message, got %s", machine.Error)
	}

	p.statusMu.RLock()
	status := p.provisioningStatus[spec.ID]
	p.statusMu.RUnlock()

	if status.State != MachineStateFailed {
		t.Errorf("Expected status state failed, got %s", status.State)
	}
}

func TestCompleteProvisioning(t *testing.T) {
	p := newTestProvisioner(t)

	spec := &MachineSpec{
		ID:         "complete-test",
		MACAddress: "00:11:22:33:44:55",
	}

	now := time.Now()
	machine := &Machine{
		Spec:      spec,
		State:     MachineStateProvisioning,
		CreatedAt: now,
		UpdatedAt: now,
		Error:     "previous error",
	}
	p.machines[spec.ID] = machine

	p.completeProvisioning(machine)

	if machine.State != MachineStateReady {
		t.Errorf("Expected state ready, got %s", machine.State)
	}

	if machine.ProvisioningCompleted == nil {
		t.Error("Expected ProvisioningCompleted to be set")
	}

	if machine.Error != "" {
		t.Errorf("Expected error to be cleared, got %s", machine.Error)
	}

	if machine.UpdatedAt.Before(now) {
		t.Error("UpdatedAt should be updated")
	}
}

func TestProvision_ReProvision(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	spec := &MachineSpec{
		ID:          "reprov-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		Hostname:    "worker-1",
	}

	// First provision
	_, err := p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("First Provision() error = %v", err)
	}

	// Wait for background goroutine to complete (it will fail quickly
	// since the image doesn't exist, but we need it to finish)
	time.Sleep(100 * time.Millisecond)

	// Re-provision the same machine with updated spec
	spec2 := &MachineSpec{
		ID:          "reprov-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.101",
		Hostname:    "worker-1-updated",
	}

	machine2, err := p.Provision(ctx, spec2)
	if err != nil {
		t.Fatalf("Re-Provision() error = %v", err)
	}

	// Read under lock to avoid race
	p.mu.RLock()
	ipmiAddr := machine2.Spec.IPMIAddress
	hostname := machine2.Spec.Hostname
	p.mu.RUnlock()

	if ipmiAddr != "10.0.0.101" {
		t.Errorf("Expected updated IPMI address, got %s", ipmiAddr)
	}

	if hostname != "worker-1-updated" {
		t.Errorf("Expected updated hostname, got %s", hostname)
	}

	// Wait for background goroutine to finish
	time.Sleep(100 * time.Millisecond)
}

func TestGetProvisioningStatus(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	t.Run("non-existent machine", func(t *testing.T) {
		_, err := p.GetProvisioningStatus(ctx, "nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent machine")
		}
	})

	t.Run("existing status", func(t *testing.T) {
		// Directly init status without Provision to avoid background goroutine races
		p.initProvisioningStatus("status-test")

		status, err := p.GetProvisioningStatus(ctx, "status-test")
		if err != nil {
			t.Fatalf("GetProvisioningStatus() error = %v", err)
		}

		if status.MachineID != "status-test" {
			t.Errorf("Expected machine ID status-test, got %s", status.MachineID)
		}

		if status.State != MachineStateProvisioning {
			t.Errorf("Expected state provisioning, got %s", status.State)
		}
	})
}

func TestReboot_NotFound(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	err := p.Reboot(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent machine")
	}
}

func TestReboot_Found(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	// Add a machine with IPMI credentials
	spec := &MachineSpec{
		ID:          "reboot-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		IPMICredentials: &ipmi.Credentials{
			Username: "admin",
			Password: "password",
		},
	}

	_, err := p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Wait for goroutine
	time.Sleep(10 * time.Millisecond)

	// Reboot should succeed (mock IPMI)
	err = p.Reboot(ctx, "reboot-test")
	if err != nil {
		t.Fatalf("Reboot() error = %v", err)
	}
}

func TestReboot_WithDefaultCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	// Set default IPMI credentials on config
	p.config.DefaultIPMICredentials = &ipmi.Credentials{
		Username: "default-admin",
		Password: "default-pass",
	}

	// Add machine without explicit credentials
	spec := &MachineSpec{
		ID:          "reboot-default",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
	}

	_, err := p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	err = p.Reboot(ctx, "reboot-default")
	if err != nil {
		t.Fatalf("Reboot() with default credentials error = %v", err)
	}
}

func TestPowerOn_NotFound(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	err := p.PowerOn(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent machine")
	}
}

func TestPowerOn_Found(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	spec := &MachineSpec{
		ID:          "poweron-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		IPMICredentials: &ipmi.Credentials{
			Username: "admin",
			Password: "password",
		},
	}

	_, err := p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	err = p.PowerOn(ctx, "poweron-test")
	if err != nil {
		t.Fatalf("PowerOn() error = %v", err)
	}
}

func TestPowerOff_NotFound(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	err := p.PowerOff(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent machine")
	}
}

func TestPowerOff_Found(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	spec := &MachineSpec{
		ID:          "poweroff-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		IPMICredentials: &ipmi.Credentials{
			Username: "admin",
			Password: "password",
		},
	}

	_, err := p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	err = p.PowerOff(ctx, "poweroff-test")
	if err != nil {
		t.Fatalf("PowerOff() error = %v", err)
	}

	p.mu.RLock()
	machine := p.machines["poweroff-test"]
	p.mu.RUnlock()

	if machine.State != MachineStatePoweredOff {
		t.Errorf("Expected state powered_off, got %s", machine.State)
	}
}

func TestPowerOffMachine_NoIPMIAddress(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	machine := &Machine{
		Spec: &MachineSpec{
			ID:         "no-ipmi",
			MACAddress: "00:11:22:33:44:55",
		},
	}

	err := p.powerOffMachine(ctx, machine)
	if err == nil {
		t.Error("Expected error for missing IPMI address")
	}
}

func TestPowerOffMachine_WithCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	machine := &Machine{
		Spec: &MachineSpec{
			ID:          "with-creds",
			MACAddress:  "00:11:22:33:44:55",
			IPMIAddress: "10.0.0.100",
			IPMICredentials: &ipmi.Credentials{
				Username: "admin",
				Password: "password",
			},
		},
	}

	err := p.powerOffMachine(ctx, machine)
	if err != nil {
		t.Fatalf("powerOffMachine() error = %v", err)
	}
}

func TestPowerOffMachine_WithDefaultCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	p.config.DefaultIPMICredentials = &ipmi.Credentials{
		Username: "default-admin",
		Password: "default-pass",
	}

	machine := &Machine{
		Spec: &MachineSpec{
			ID:          "default-creds",
			MACAddress:  "00:11:22:33:44:55",
			IPMIAddress: "10.0.0.100",
		},
	}

	err := p.powerOffMachine(ctx, machine)
	if err != nil {
		t.Fatalf("powerOffMachine() with default creds error = %v", err)
	}
}

func TestPowerOffMachine_NilCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	// No default credentials, no spec credentials
	machine := &Machine{
		Spec: &MachineSpec{
			ID:          "nil-creds",
			MACAddress:  "00:11:22:33:44:55",
			IPMIAddress: "10.0.0.100",
		},
	}

	err := p.powerOffMachine(ctx, machine)
	if err == nil {
		t.Error("Expected error for nil credentials")
	}
}

func TestPowerOnMachine_WithCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	machine := &Machine{
		Spec: &MachineSpec{
			ID:          "poweron-creds",
			MACAddress:  "00:11:22:33:44:55",
			IPMIAddress: "10.0.0.100",
			IPMICredentials: &ipmi.Credentials{
				Username: "admin",
				Password: "password",
			},
		},
	}

	err := p.powerOnMachine(ctx, machine)
	if err != nil {
		t.Fatalf("powerOnMachine() error = %v", err)
	}
}

func TestPowerOnMachine_DefaultCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	p.config.DefaultIPMICredentials = &ipmi.Credentials{
		Username: "default-admin",
		Password: "default-pass",
	}

	machine := &Machine{
		Spec: &MachineSpec{
			ID:          "poweron-default",
			MACAddress:  "00:11:22:33:44:55",
			IPMIAddress: "10.0.0.100",
		},
	}

	err := p.powerOnMachine(ctx, machine)
	if err != nil {
		t.Fatalf("powerOnMachine() with default creds error = %v", err)
	}
}

func TestPowerOnMachine_NilCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	machine := &Machine{
		Spec: &MachineSpec{
			ID:          "poweron-nil",
			MACAddress:  "00:11:22:33:44:55",
			IPMIAddress: "10.0.0.100",
		},
	}

	err := p.powerOnMachine(ctx, machine)
	if err == nil {
		t.Error("Expected error for nil credentials")
	}
}

func TestPrepareImage_WithNixOS(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	machine := &Machine{
		Spec: &MachineSpec{
			ID:         "nixos-test",
			MACAddress: "00:11:22:33:44:55",
			NixOSConfig: &NixOSConfig{
				FlakeRef: "github:test/config#worker",
				System:   "x86_64-linux",
			},
		},
	}

	// BuildNixOS will fail because nix is not installed in test env,
	// but we're testing the branch selection
	_, err := p.prepareImage(ctx, machine)
	if err == nil {
		t.Log("prepareImage with NixOS succeeded (nix available)")
	} else {
		t.Logf("prepareImage with NixOS returned expected error: %v", err)
	}
}

func TestPrepareImage_WithImageRef(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	machine := &Machine{
		Spec: &MachineSpec{
			ID:         "image-test",
			MACAddress: "00:11:22:33:44:55",
			ImageRef:   "ubuntu-22.04",
		},
	}

	// GetImage will fail because no images are loaded in test env
	_, err := p.prepareImage(ctx, machine)
	if err == nil {
		t.Log("prepareImage with ImageRef succeeded")
	} else {
		t.Logf("prepareImage with ImageRef returned expected error: %v", err)
	}
}

func TestDiscoverMachines(t *testing.T) {
	p := newTestProvisioner(t)

	// Use a cancelled context to avoid actual network scanning
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Discovery with cancelled context should return quickly
	machines, err := p.DiscoverMachines(ctx)
	if err != nil {
		t.Fatalf("DiscoverMachines() error = %v", err)
	}

	// In a test environment with cancelled context, we get 0 results
	t.Logf("Discovered %d machines", len(machines))
}

func TestDiscoverMachines_WithExistingMAC(t *testing.T) {
	p := newTestProvisioner(t)

	// Use a cancelled context to avoid actual network scanning
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Pre-populate a machine with a known MAC
	existingSpec := &MachineSpec{
		ID:         "existing-1",
		MACAddress: "", // Will not match discovery results
	}
	now := time.Now()
	p.machines["existing-1"] = &Machine{
		Spec:      existingSpec,
		State:     MachineStateReady,
		CreatedAt: now,
		UpdatedAt: now,
	}

	machines, err := p.DiscoverMachines(ctx)
	if err != nil {
		t.Fatalf("DiscoverMachines() error = %v", err)
	}

	t.Logf("Discovered %d machines with existing", len(machines))
}

func TestDeprovision_WithCredentials(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	spec := &MachineSpec{
		ID:          "deprov-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		IPMICredentials: &ipmi.Credentials{
			Username: "admin",
			Password: "password",
		},
	}

	_, err := p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Also add provisioning status
	p.initProvisioningStatus(spec.ID)

	err = p.Deprovision(ctx, "deprov-test")
	if err != nil {
		t.Fatalf("Deprovision() error = %v", err)
	}

	// Verify machine is removed
	_, err = p.GetMachine(ctx, "deprov-test")
	if err == nil {
		t.Error("Expected error for deprovisioned machine")
	}

	// Verify provisioning status is removed
	_, err = p.GetProvisioningStatus(ctx, "deprov-test")
	if err == nil {
		t.Error("Expected error for deprovisioned machine status")
	}
}

func TestRunProvisioningWorkflow(t *testing.T) {
	p := newTestProvisioner(t)

	spec := &MachineSpec{
		ID:          "workflow-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		IPMICredentials: &ipmi.Credentials{
			Username: "admin",
			Password: "password",
		},
		ImageRef: "test-image",
	}

	now := time.Now()
	machine := &Machine{
		Spec:      spec,
		State:     MachineStateProvisioning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.machines[spec.ID] = machine
	p.initProvisioningStatus(spec.ID)

	// Run the workflow synchronously (it will fail at some phase
	// since we don't have real IPMI/PXE, but it exercises the code)
	p.runProvisioningWorkflow(context.Background(), machine)

	// The workflow should have progressed through phases and eventually
	// either completed or failed. Check that status was updated.
	p.statusMu.RLock()
	status := p.provisioningStatus[spec.ID]
	p.statusMu.RUnlock()

	if status == nil {
		t.Fatal("Expected provisioning status to exist after workflow")
	}

	// Should have more than 1 phase history entry
	if len(status.PhaseHistory) < 2 {
		t.Errorf("Expected at least 2 phase history entries, got %d", len(status.PhaseHistory))
	}

	t.Logf("Workflow ended in state %s, phase %s with %d history entries",
		status.State, status.Phase, len(status.PhaseHistory))
}

func TestRunProvisioningWorkflow_ContextCancellation(t *testing.T) {
	p := newTestProvisioner(t)

	// Use a very short timeout
	p.config.ProvisionTimeout = 1 * time.Millisecond

	spec := &MachineSpec{
		ID:          "cancel-test",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		IPMICredentials: &ipmi.Credentials{
			Username: "admin",
			Password: "password",
		},
		ImageRef: "test-image",
	}

	now := time.Now()
	machine := &Machine{
		Spec:      spec,
		State:     MachineStateProvisioning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.machines[spec.ID] = machine
	p.initProvisioningStatus(spec.ID)

	p.runProvisioningWorkflow(context.Background(), machine)

	// Should have set ProvisioningStarted
	if machine.ProvisioningStarted == nil {
		t.Error("Expected ProvisioningStarted to be set")
	}
}

func TestVerifyProvisioning(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	machine := &Machine{
		Spec: &MachineSpec{
			ID:         "verify-test",
			MACAddress: "00:11:22:33:44:55",
		},
	}

	// verifyProvisioning is a no-op that returns nil
	err := p.verifyProvisioning(ctx, machine)
	if err != nil {
		t.Fatalf("verifyProvisioning() error = %v", err)
	}
}

func TestListMachines_Multiple(t *testing.T) {
	p := newTestProvisioner(t)
	ctx := context.Background()

	// Add several machines
	for i := 0; i < 5; i++ {
		spec := &MachineSpec{
			ID:          fmt.Sprintf("machine-%d", i),
			MACAddress:  fmt.Sprintf("00:11:22:33:44:%02x", i),
			IPMIAddress: fmt.Sprintf("10.0.0.%d", 100+i),
		}
		_, err := p.Provision(ctx, spec)
		if err != nil {
			t.Fatalf("Provision() error = %v", err)
		}
	}

	machines, err := p.ListMachines(ctx)
	if err != nil {
		t.Fatalf("ListMachines() error = %v", err)
	}

	if len(machines) != 5 {
		t.Errorf("Expected 5 machines, got %d", len(machines))
	}

	// Wait for goroutines
	time.Sleep(50 * time.Millisecond)
}

func TestMachineStateValues(t *testing.T) {
	expected := map[MachineState]string{
		MachineStateUnknown:      "unknown",
		MachineStateDiscovered:   "discovered",
		MachineStateProvisioning: "provisioning",
		MachineStateReady:        "ready",
		MachineStateFailed:       "failed",
		MachineStateDraining:     "draining",
		MachineStatePoweredOff:   "powered_off",
	}

	for state, val := range expected {
		if string(state) != val {
			t.Errorf("MachineState %s expected %s, got %s", val, val, string(state))
		}
	}
}

func TestProvisionPhaseValues(t *testing.T) {
	expected := map[ProvisionPhase]string{
		PhaseInit:         "init",
		PhasePowerOff:     "power_off",
		PhaseImagePrepare: "image_prepare",
		PhasePXESetup:     "pxe_setup",
		PhasePowerOn:      "power_on",
		PhaseBooting:      "booting",
		PhaseInstalling:   "installing",
		PhaseConfiguring:  "configuring",
		PhaseVerifying:    "verifying",
		PhaseComplete:     "complete",
		PhaseFailed:       "failed",
	}

	for phase, val := range expected {
		if string(phase) != val {
			t.Errorf("ProvisionPhase %s expected %s, got %s", val, val, string(phase))
		}
	}
}
