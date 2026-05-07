// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package baremetal

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/baremetal/ipmi"
)

func TestNewProvisioner(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		// Use TestConfig to avoid creating real directories
		p, err := NewProvisioner(TestConfig())
		if err != nil {
			t.Fatalf("NewProvisioner() error = %v", err)
		}
		if p == nil {
			t.Fatal("NewProvisioner() returned nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		config := TestConfig()
		config.ProvisionTimeout = 60 * time.Minute

		p, err := NewProvisioner(config)
		if err != nil {
			t.Fatalf("NewProvisioner() error = %v", err)
		}
		if p == nil {
			t.Fatal("NewProvisioner() returned nil")
		}
	})
}

func TestMachineSpec_Validation(t *testing.T) {
	p, err := NewProvisioner(TestConfig())
	if err != nil {
		t.Fatalf("NewProvisioner() error = %v", err)
	}
	ctx := context.Background()

	tests := []struct {
		name    string
		spec    *MachineSpec
		wantErr bool
	}{
		{
			name:    "nil spec",
			spec:    nil,
			wantErr: true,
		},
		{
			name:    "empty ID",
			spec:    &MachineSpec{MACAddress: "00:11:22:33:44:55"},
			wantErr: true,
		},
		{
			name:    "empty MAC",
			spec:    &MachineSpec{ID: "machine-1"},
			wantErr: true,
		},
		{
			name: "valid spec",
			spec: &MachineSpec{
				ID:          "machine-1",
				MACAddress:  "00:11:22:33:44:55",
				IPMIAddress: "10.0.0.100",
				Hostname:    "worker-1",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Provision(ctx, tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Provision() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMachineState(t *testing.T) {
	states := []MachineState{
		MachineStateUnknown,
		MachineStateDiscovered,
		MachineStateProvisioning,
		MachineStateReady,
		MachineStateFailed,
		MachineStateDraining,
		MachineStatePoweredOff,
	}

	for _, state := range states {
		if state == "" {
			t.Errorf("MachineState should not be empty")
		}
	}
}

func TestProvisionPhase(t *testing.T) {
	phases := []ProvisionPhase{
		PhaseInit,
		PhasePowerOff,
		PhaseImagePrepare,
		PhasePXESetup,
		PhasePowerOn,
		PhaseBooting,
		PhaseInstalling,
		PhaseConfiguring,
		PhaseVerifying,
		PhaseComplete,
		PhaseFailed,
	}

	for _, phase := range phases {
		if phase == "" {
			t.Errorf("ProvisionPhase should not be empty")
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if config.ProvisionTimeout <= 0 {
		t.Errorf("ProvisionTimeout should be positive, got %v", config.ProvisionTimeout)
	}

	if config.HeartbeatInterval <= 0 {
		t.Errorf("HeartbeatInterval should be positive, got %v", config.HeartbeatInterval)
	}

	if config.PXE == nil {
		t.Error("PXE config should not be nil")
	}

	if config.Image == nil {
		t.Error("Image config should not be nil")
	}

	if config.Discovery == nil {
		t.Error("Discovery config should not be nil")
	}
}

func TestMachineSpec_Labels(t *testing.T) {
	spec := &MachineSpec{
		ID:         "machine-1",
		MACAddress: "00:11:22:33:44:55",
		Labels: map[string]string{
			"role":       "worker",
			"datacenter": "dc1",
		},
		Annotations: map[string]string{
			"description": "Test machine",
		},
	}

	if spec.Labels["role"] != "worker" {
		t.Errorf("Expected role=worker, got %s", spec.Labels["role"])
	}

	if spec.Annotations["description"] != "Test machine" {
		t.Errorf("Expected description='Test machine', got %s", spec.Annotations["description"])
	}
}

func TestNetworkConfig(t *testing.T) {
	config := &NetworkConfig{
		Gateway: "10.0.0.1",
		DNS:     []string{"8.8.8.8", "8.8.4.4"},
		VLANID:  100,
		MTU:     9000,
		Bonds: []BondConfig{
			{
				Name:       "bond0",
				Mode:       "802.3ad",
				Interfaces: []string{"eth0", "eth1"},
			},
		},
	}

	if config.Gateway != "10.0.0.1" {
		t.Errorf("Expected gateway 10.0.0.1, got %s", config.Gateway)
	}

	if len(config.DNS) != 2 {
		t.Errorf("Expected 2 DNS servers, got %d", len(config.DNS))
	}

	if len(config.Bonds) != 1 {
		t.Errorf("Expected 1 bond, got %d", len(config.Bonds))
	}

	if config.Bonds[0].Mode != "802.3ad" {
		t.Errorf("Expected bond mode 802.3ad, got %s", config.Bonds[0].Mode)
	}
}

func TestNixOSConfig(t *testing.T) {
	config := &NixOSConfig{
		FlakeRef:     "github:unheaded/infrastructure#worker",
		System:       "x86_64-linux",
		ExtraModules: []string{"./hardware-configuration.nix"},
		Secrets: map[string]string{
			"ssh-key": "secret-value",
		},
	}

	if config.FlakeRef == "" {
		t.Error("FlakeRef should not be empty")
	}

	if config.System != "x86_64-linux" {
		t.Errorf("Expected system x86_64-linux, got %s", config.System)
	}
}

func TestProvisioningStatus(t *testing.T) {
	now := time.Now()
	status := &ProvisioningStatus{
		MachineID: "machine-1",
		State:     MachineStateProvisioning,
		Phase:     PhaseInstalling,
		Progress:  60,
		Message:   "Installing operating system",
		StartedAt: now,
		UpdatedAt: now,
		PhaseHistory: []PhaseHistoryEntry{
			{Phase: PhaseInit, Timestamp: now, Message: "Started"},
			{Phase: PhasePowerOff, Timestamp: now.Add(time.Minute), Message: "Powered off"},
		},
	}

	if status.Progress != 60 {
		t.Errorf("Expected progress 60, got %d", status.Progress)
	}

	if len(status.PhaseHistory) != 2 {
		t.Errorf("Expected 2 history entries, got %d", len(status.PhaseHistory))
	}
}

func TestBareMetalProvisioner_ListMachines(t *testing.T) {
	p, err := NewProvisioner(TestConfig())
	if err != nil {
		t.Fatalf("NewProvisioner() error = %v", err)
	}

	ctx := context.Background()

	// Initially empty
	machines, err := p.ListMachines(ctx)
	if err != nil {
		t.Fatalf("ListMachines() error = %v", err)
	}
	if len(machines) != 0 {
		t.Errorf("Expected 0 machines, got %d", len(machines))
	}

	// Add a machine
	spec := &MachineSpec{
		ID:          "machine-1",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
		IPMICredentials: &ipmi.Credentials{
			Username: "admin",
			Password: "password",
		},
	}

	_, err = p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Now should have one machine
	machines, err = p.ListMachines(ctx)
	if err != nil {
		t.Fatalf("ListMachines() error = %v", err)
	}
	if len(machines) != 1 {
		t.Errorf("Expected 1 machine, got %d", len(machines))
	}
}

func TestBareMetalProvisioner_GetMachine(t *testing.T) {
	p, err := NewProvisioner(TestConfig())
	if err != nil {
		t.Fatalf("NewProvisioner() error = %v", err)
	}
	ctx := context.Background()

	// Non-existent machine
	_, err = p.GetMachine(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error for non-existent machine")
	}

	// Add a machine
	spec := &MachineSpec{
		ID:          "machine-1",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
	}

	_, err = p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Now should find it
	machine, err := p.GetMachine(ctx, "machine-1")
	if err != nil {
		t.Fatalf("GetMachine() error = %v", err)
	}
	if machine.Spec.ID != "machine-1" {
		t.Errorf("Expected machine-1, got %s", machine.Spec.ID)
	}
}

func TestBareMetalProvisioner_Deprovision(t *testing.T) {
	p, err := NewProvisioner(TestConfig())
	if err != nil {
		t.Fatalf("NewProvisioner() error = %v", err)
	}
	ctx := context.Background()

	// Non-existent machine
	err = p.Deprovision(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error for non-existent machine")
	}

	// Add a machine
	spec := &MachineSpec{
		ID:          "machine-1",
		MACAddress:  "00:11:22:33:44:55",
		IPMIAddress: "10.0.0.100",
	}

	_, err = p.Provision(ctx, spec)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Deprovision
	err = p.Deprovision(ctx, "machine-1")
	if err != nil {
		t.Fatalf("Deprovision() error = %v", err)
	}

	// Should no longer exist
	_, err = p.GetMachine(ctx, "machine-1")
	if err == nil {
		t.Error("Expected error for deprovisioned machine")
	}
}

func TestMachine_Timestamps(t *testing.T) {
	now := time.Now()
	machine := &Machine{
		Spec: &MachineSpec{
			ID:         "machine-1",
			MACAddress: "00:11:22:33:44:55",
		},
		State:     MachineStateReady,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if machine.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	if machine.UpdatedAt.Before(machine.CreatedAt) {
		t.Error("UpdatedAt should not be before CreatedAt")
	}
}

func TestPhaseHistoryEntry(t *testing.T) {
	entry := PhaseHistoryEntry{
		Phase:     PhaseInstalling,
		Timestamp: time.Now(),
		Message:   "Installing OS",
		Error:     "",
	}

	if entry.Phase != PhaseInstalling {
		t.Errorf("Expected PhaseInstalling, got %s", entry.Phase)
	}

	entryWithError := PhaseHistoryEntry{
		Phase:     PhaseFailed,
		Timestamp: time.Now(),
		Message:   "Installation failed",
		Error:     "disk not found",
	}

	if entryWithError.Error == "" {
		t.Error("Expected error message")
	}
}

func TestBondConfig(t *testing.T) {
	bond := BondConfig{
		Name:       "bond0",
		Mode:       "balance-rr",
		Interfaces: []string{"eth0", "eth1", "eth2", "eth3"},
	}

	if bond.Name != "bond0" {
		t.Errorf("Expected bond0, got %s", bond.Name)
	}

	if len(bond.Interfaces) != 4 {
		t.Errorf("Expected 4 interfaces, got %d", len(bond.Interfaces))
	}
}
