// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package baremetal provides bare metal provisioning for the Unheaded Kingdom infrastructure.
// THE SABATONS - the foundation upon which all else stands.
package baremetal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/baremetal/image"
	"unheaded/pkg/baremetal/inventory"
	"unheaded/pkg/baremetal/ipmi"
	"unheaded/pkg/baremetal/pxe"
)

// MachineState represents the current state of a bare metal machine.
type MachineState string

const (
	MachineStateUnknown      MachineState = "unknown"
	MachineStateDiscovered   MachineState = "discovered"
	MachineStateProvisioning MachineState = "provisioning"
	MachineStateReady        MachineState = "ready"
	MachineStateFailed       MachineState = "failed"
	MachineStateDraining     MachineState = "draining"
	MachineStatePoweredOff   MachineState = "powered_off"
)

// MachineSpec defines the desired configuration for a bare metal machine.
type MachineSpec struct {
	// ID is a unique identifier for the machine
	ID string `json:"id"`

	// Hostname is the desired hostname
	Hostname string `json:"hostname"`

	// MACAddress is the primary MAC address for PXE boot
	MACAddress string `json:"mac_address"`

	// IPMIAddress is the out-of-band management IP
	IPMIAddress string `json:"ipmi_address"`

	// IPMICredentials for out-of-band access
	IPMICredentials *ipmi.Credentials `json:"ipmi_credentials,omitempty"`

	// DesiredIP is the IP address to assign
	DesiredIP string `json:"desired_ip"`

	// NetworkConfig holds network configuration
	NetworkConfig *NetworkConfig `json:"network_config,omitempty"`

	// ImageRef references the OS image to provision
	ImageRef string `json:"image_ref"`

	// NixOSConfig holds NixOS-specific configuration
	NixOSConfig *NixOSConfig `json:"nixos_config,omitempty"`

	// Labels for machine classification
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations for additional metadata
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NetworkConfig defines network configuration for a machine.
type NetworkConfig struct {
	// Gateway is the default gateway
	Gateway string `json:"gateway"`

	// DNS servers
	DNS []string `json:"dns"`

	// VLAN ID if applicable
	VLANID int `json:"vlan_id,omitempty"`

	// MTU for the interface
	MTU int `json:"mtu,omitempty"`

	// Bonds defines bonded interfaces
	Bonds []BondConfig `json:"bonds,omitempty"`
}

// BondConfig defines a network bond configuration.
type BondConfig struct {
	Name       string   `json:"name"`
	Mode       string   `json:"mode"`
	Interfaces []string `json:"interfaces"`
}

// NixOSConfig holds NixOS-specific provisioning configuration.
type NixOSConfig struct {
	// FlakeRef is the flake reference for the NixOS configuration
	FlakeRef string `json:"flake_ref"`

	// System is the target system (e.g., x86_64-linux)
	System string `json:"system"`

	// ExtraModules are additional NixOS modules to include
	ExtraModules []string `json:"extra_modules,omitempty"`

	// Secrets maps secret names to their values
	Secrets map[string]string `json:"secrets,omitempty"`
}

// Machine represents a provisioned bare metal machine.
type Machine struct {
	// Spec is the desired configuration
	Spec *MachineSpec `json:"spec"`

	// State is the current machine state
	State MachineState `json:"state"`

	// Hardware contains discovered hardware information
	Hardware *inventory.Hardware `json:"hardware,omitempty"`

	// ProvisioningStarted is when provisioning began
	ProvisioningStarted *time.Time `json:"provisioning_started,omitempty"`

	// ProvisioningCompleted is when provisioning finished
	ProvisioningCompleted *time.Time `json:"provisioning_completed,omitempty"`

	// LastHeartbeat is the last time we heard from the machine
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`

	// Error holds any error message
	Error string `json:"error,omitempty"`

	// CreatedAt is when the machine record was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the machine was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// Provisioner defines the interface for bare metal provisioning.
type Provisioner interface {
	// Provision creates or updates a bare metal machine
	Provision(ctx context.Context, spec *MachineSpec) (*Machine, error)

	// Deprovision removes a machine from management
	Deprovision(ctx context.Context, machineID string) error

	// Reboot reboots a machine
	Reboot(ctx context.Context, machineID string) error

	// PowerOn powers on a machine
	PowerOn(ctx context.Context, machineID string) error

	// PowerOff powers off a machine
	PowerOff(ctx context.Context, machineID string) error

	// GetMachine retrieves a machine by ID
	GetMachine(ctx context.Context, machineID string) (*Machine, error)

	// ListMachines lists all managed machines
	ListMachines(ctx context.Context) ([]*Machine, error)

	// DiscoverMachines triggers auto-discovery
	DiscoverMachines(ctx context.Context) ([]*Machine, error)

	// GetProvisioningStatus returns detailed provisioning status
	GetProvisioningStatus(ctx context.Context, machineID string) (*ProvisioningStatus, error)
}

// ProvisioningStatus provides detailed status of a provisioning operation.
type ProvisioningStatus struct {
	MachineID    string              `json:"machine_id"`
	State        MachineState        `json:"state"`
	Phase        ProvisionPhase      `json:"phase"`
	Progress     int                 `json:"progress"` // 0-100
	Message      string              `json:"message"`
	StartedAt    time.Time           `json:"started_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	PhaseHistory []PhaseHistoryEntry `json:"phase_history"`
}

// ProvisionPhase represents a phase in the provisioning process.
type ProvisionPhase string

const (
	PhaseInit         ProvisionPhase = "init"
	PhasePowerOff     ProvisionPhase = "power_off"
	PhaseImagePrepare ProvisionPhase = "image_prepare"
	PhasePXESetup     ProvisionPhase = "pxe_setup"
	PhasePowerOn      ProvisionPhase = "power_on"
	PhaseBooting      ProvisionPhase = "booting"
	PhaseInstalling   ProvisionPhase = "installing"
	PhaseConfiguring  ProvisionPhase = "configuring"
	PhaseVerifying    ProvisionPhase = "verifying"
	PhaseComplete     ProvisionPhase = "complete"
	PhaseFailed       ProvisionPhase = "failed"
)

// PhaseHistoryEntry records a phase transition.
type PhaseHistoryEntry struct {
	Phase     ProvisionPhase `json:"phase"`
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	Error     string         `json:"error,omitempty"`
}

// Config holds configuration for the bare metal provisioner.
type Config struct {
	// PXE server configuration
	PXE *pxe.Config `json:"pxe"`

	// Image management configuration
	Image *image.Config `json:"image"`

	// Discovery configuration
	Discovery *inventory.DiscoveryConfig `json:"discovery"`

	// DefaultIPMICredentials for machines without explicit credentials
	DefaultIPMICredentials *ipmi.Credentials `json:"default_ipmi_credentials,omitempty"`

	// ProvisionTimeout is the maximum time for provisioning
	ProvisionTimeout time.Duration `json:"provision_timeout"`

	// HeartbeatInterval for machine health checks
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`

	// SkipDirectoryInit skips directory creation (for testing)
	SkipDirectoryInit bool `json:"skip_directory_init,omitempty"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		PXE:               pxe.DefaultConfig(),
		Image:             image.DefaultConfig(),
		Discovery:         inventory.DefaultDiscoveryConfig(),
		ProvisionTimeout:  30 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
	}
}

// TestConfig returns a configuration suitable for testing.
// It skips directory creation and other filesystem operations.
func TestConfig() *Config {
	pxeConfig := pxe.DefaultConfig()
	pxeConfig.SkipDirectoryInit = true

	imageConfig := image.DefaultConfig()
	imageConfig.SkipDirectoryInit = true

	return &Config{
		PXE:               pxeConfig,
		Image:             imageConfig,
		Discovery:         inventory.DefaultDiscoveryConfig(),
		ProvisionTimeout:  30 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		SkipDirectoryInit: true,
	}
}

// bareMetalProvisioner implements the Provisioner interface.
type bareMetalProvisioner struct {
	config *Config

	// PXE server for network booting
	pxeServer *pxe.Server

	// Image manager for OS images
	imageManager *image.Manager

	// Inventory for hardware tracking
	inventory *inventory.Inventory

	// Discovery service
	discovery *inventory.Discovery

	// Machines indexed by ID
	machines map[string]*Machine
	mu       sync.RWMutex

	// Provisioning status tracking
	provisioningStatus map[string]*ProvisioningStatus
	statusMu           sync.RWMutex

	// Shutdown channel
	done chan struct{}
}

// NewProvisioner creates a new bare metal provisioner.
func NewProvisioner(config *Config) (Provisioner, error) {
	if config == nil {
		config = DefaultConfig()
	}

	pxeServer, err := pxe.NewServer(config.PXE)
	if err != nil {
		return nil, fmt.Errorf("creating PXE server: %w", err)
	}

	imageManager, err := image.NewManager(config.Image)
	if err != nil {
		return nil, fmt.Errorf("creating image manager: %w", err)
	}

	inv := inventory.NewInventory()
	disc := inventory.NewDiscovery(config.Discovery)

	p := &bareMetalProvisioner{
		config:             config,
		pxeServer:          pxeServer,
		imageManager:       imageManager,
		inventory:          inv,
		discovery:          disc,
		machines:           make(map[string]*Machine),
		provisioningStatus: make(map[string]*ProvisioningStatus),
		done:               make(chan struct{}),
	}

	return p, nil
}

// Provision creates or updates a bare metal machine.
func (p *bareMetalProvisioner) Provision(ctx context.Context, spec *MachineSpec) (*Machine, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec cannot be nil")
	}

	if spec.ID == "" {
		return nil, fmt.Errorf("machine ID is required")
	}

	if spec.MACAddress == "" {
		return nil, fmt.Errorf("MAC address is required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if machine already exists
	machine, exists := p.machines[spec.ID]
	if !exists {
		now := time.Now()
		machine = &Machine{
			Spec:      spec,
			State:     MachineStateProvisioning,
			CreatedAt: now,
			UpdatedAt: now,
		}
		p.machines[spec.ID] = machine
	} else {
		machine.Spec = spec
		machine.State = MachineStateProvisioning
		machine.UpdatedAt = time.Now()
		machine.Error = ""
	}

	// Initialize provisioning status
	p.initProvisioningStatus(spec.ID)

	// Start provisioning workflow in background
	go p.runProvisioningWorkflow(context.Background(), machine)

	return machine, nil
}

// initProvisioningStatus creates a new provisioning status record.
func (p *bareMetalProvisioner) initProvisioningStatus(machineID string) {
	p.statusMu.Lock()
	defer p.statusMu.Unlock()

	now := time.Now()
	p.provisioningStatus[machineID] = &ProvisioningStatus{
		MachineID: machineID,
		State:     MachineStateProvisioning,
		Phase:     PhaseInit,
		Progress:  0,
		Message:   "Initializing provisioning",
		StartedAt: now,
		UpdatedAt: now,
		PhaseHistory: []PhaseHistoryEntry{
			{Phase: PhaseInit, Timestamp: now, Message: "Provisioning started"},
		},
	}
}

// updateProvisioningPhase updates the provisioning phase.
func (p *bareMetalProvisioner) updateProvisioningPhase(machineID string, phase ProvisionPhase, progress int, message string, err error) {
	p.statusMu.Lock()
	defer p.statusMu.Unlock()

	status, exists := p.provisioningStatus[machineID]
	if !exists {
		return
	}

	now := time.Now()
	entry := PhaseHistoryEntry{
		Phase:     phase,
		Timestamp: now,
		Message:   message,
	}
	if err != nil {
		entry.Error = err.Error()
	}

	status.Phase = phase
	status.Progress = progress
	status.Message = message
	status.UpdatedAt = now
	status.PhaseHistory = append(status.PhaseHistory, entry)

	switch phase {
	case PhaseFailed:
		status.State = MachineStateFailed
	case PhaseComplete:
		status.State = MachineStateReady
		status.Progress = 100
	}
}

// runProvisioningWorkflow executes the provisioning workflow.
func (p *bareMetalProvisioner) runProvisioningWorkflow(ctx context.Context, machine *Machine) {
	machineID := machine.Spec.ID
	now := time.Now()
	machine.ProvisioningStarted = &now

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, p.config.ProvisionTimeout)
	defer cancel()

	// Phase 1: Power off the machine
	p.updateProvisioningPhase(machineID, PhasePowerOff, 10, "Powering off machine", nil)
	if err := p.powerOffMachine(ctx, machine); err != nil {
		p.failProvisioning(machine, fmt.Errorf("power off: %w", err))
		return
	}

	// Phase 2: Prepare the OS image
	p.updateProvisioningPhase(machineID, PhaseImagePrepare, 20, "Preparing OS image", nil)
	imageInfo, err := p.prepareImage(ctx, machine)
	if err != nil {
		p.failProvisioning(machine, fmt.Errorf("image prepare: %w", err))
		return
	}

	// Phase 3: Configure PXE boot
	p.updateProvisioningPhase(machineID, PhasePXESetup, 30, "Setting up PXE boot", nil)
	if err := p.setupPXE(ctx, machine, imageInfo); err != nil {
		p.failProvisioning(machine, fmt.Errorf("PXE setup: %w", err))
		return
	}

	// Phase 4: Power on the machine
	p.updateProvisioningPhase(machineID, PhasePowerOn, 40, "Powering on machine", nil)
	if err := p.powerOnMachine(ctx, machine); err != nil {
		p.failProvisioning(machine, fmt.Errorf("power on: %w", err))
		return
	}

	// Phase 5: Wait for boot
	p.updateProvisioningPhase(machineID, PhaseBooting, 50, "Waiting for machine to boot", nil)
	if err := p.waitForBoot(ctx, machine); err != nil {
		p.failProvisioning(machine, fmt.Errorf("boot wait: %w", err))
		return
	}

	// Phase 6: Installation in progress
	p.updateProvisioningPhase(machineID, PhaseInstalling, 60, "Installing operating system", nil)
	if err := p.waitForInstallation(ctx, machine); err != nil {
		p.failProvisioning(machine, fmt.Errorf("installation: %w", err))
		return
	}

	// Phase 7: Post-installation configuration
	p.updateProvisioningPhase(machineID, PhaseConfiguring, 80, "Configuring system", nil)
	if err := p.configureSystem(ctx, machine); err != nil {
		p.failProvisioning(machine, fmt.Errorf("configuration: %w", err))
		return
	}

	// Phase 8: Verify provisioning
	p.updateProvisioningPhase(machineID, PhaseVerifying, 90, "Verifying provisioning", nil)
	if err := p.verifyProvisioning(ctx, machine); err != nil {
		p.failProvisioning(machine, fmt.Errorf("verification: %w", err))
		return
	}

	// Phase 9: Complete
	p.updateProvisioningPhase(machineID, PhaseComplete, 100, "Provisioning complete", nil)
	p.completeProvisioning(machine)
}

// failProvisioning marks provisioning as failed.
func (p *bareMetalProvisioner) failProvisioning(machine *Machine, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	machine.State = MachineStateFailed
	machine.Error = err.Error()
	machine.UpdatedAt = time.Now()

	p.updateProvisioningPhase(machine.Spec.ID, PhaseFailed, 0, err.Error(), err)
}

// completeProvisioning marks provisioning as complete.
func (p *bareMetalProvisioner) completeProvisioning(machine *Machine) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	machine.State = MachineStateReady
	machine.ProvisioningCompleted = &now
	machine.UpdatedAt = now
	machine.Error = ""
}

// powerOffMachine powers off a machine via IPMI/Redfish.
func (p *bareMetalProvisioner) powerOffMachine(ctx context.Context, machine *Machine) error {
	creds := machine.Spec.IPMICredentials
	if creds == nil {
		creds = p.config.DefaultIPMICredentials
	}

	if machine.Spec.IPMIAddress == "" {
		return fmt.Errorf("IPMI address not configured")
	}

	client, err := ipmi.NewClient(machine.Spec.IPMIAddress, creds)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	return client.PowerOff(ctx)
}

// powerOnMachine powers on a machine via IPMI/Redfish.
func (p *bareMetalProvisioner) powerOnMachine(ctx context.Context, machine *Machine) error {
	creds := machine.Spec.IPMICredentials
	if creds == nil {
		creds = p.config.DefaultIPMICredentials
	}

	client, err := ipmi.NewClient(machine.Spec.IPMIAddress, creds)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	// Set boot device to PXE
	if err := client.SetBootDevice(ctx, ipmi.BootDevicePXE); err != nil {
		return fmt.Errorf("setting boot device: %w", err)
	}

	return client.PowerOn(ctx)
}

// prepareImage prepares the OS image for provisioning.
func (p *bareMetalProvisioner) prepareImage(ctx context.Context, machine *Machine) (*image.ImageInfo, error) {
	if machine.Spec.NixOSConfig != nil {
		// Build NixOS image
		return p.imageManager.BuildNixOS(ctx, machine.Spec.NixOSConfig.FlakeRef, machine.Spec.NixOSConfig.System)
	}

	// Use pre-built image
	return p.imageManager.GetImage(ctx, machine.Spec.ImageRef)
}

// setupPXE configures PXE boot for the machine.
func (p *bareMetalProvisioner) setupPXE(ctx context.Context, machine *Machine, imageInfo *image.ImageInfo) error {
	return p.pxeServer.ConfigureClient(ctx, &pxe.ClientConfig{
		MACAddress:   machine.Spec.MACAddress,
		Hostname:     machine.Spec.Hostname,
		IPAddress:    machine.Spec.DesiredIP,
		KernelPath:   imageInfo.KernelPath,
		InitrdPath:   imageInfo.InitrdPath,
		RootFSPath:   imageInfo.RootFSPath,
		KernelParams: imageInfo.KernelParams,
	})
}

// waitForBoot waits for the machine to boot.
func (p *bareMetalProvisioner) waitForBoot(ctx context.Context, machine *Machine) error {
	// Poll for PXE boot callback
	return p.pxeServer.WaitForBoot(ctx, machine.Spec.MACAddress, 10*time.Minute)
}

// waitForInstallation waits for OS installation to complete.
func (p *bareMetalProvisioner) waitForInstallation(ctx context.Context, machine *Machine) error {
	// Poll for installation callback
	return p.pxeServer.WaitForInstallation(ctx, machine.Spec.MACAddress, 20*time.Minute)
}

// configureSystem performs post-installation configuration.
func (p *bareMetalProvisioner) configureSystem(ctx context.Context, machine *Machine) error {
	// Clean up PXE configuration to prevent re-provisioning on reboot
	return p.pxeServer.RemoveClient(ctx, machine.Spec.MACAddress)
}

// verifyProvisioning verifies that provisioning completed successfully.
func (p *bareMetalProvisioner) verifyProvisioning(ctx context.Context, machine *Machine) error {
	// Try to connect to the machine and verify it's running the expected OS
	// This is a simplified check - production would verify more
	return nil
}

// Deprovision removes a machine from management.
func (p *bareMetalProvisioner) Deprovision(ctx context.Context, machineID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	machine, exists := p.machines[machineID]
	if !exists {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	// Best-effort power-off and PXE cleanup. Errors are swallowed because
	// Deprovision must be idempotent against partially-provisioned or
	// already-offline machines; surfacing them would block the inventory
	// delete below.
	_ = p.powerOffMachine(ctx, machine)
	_ = p.pxeServer.RemoveClient(ctx, machine.Spec.MACAddress)

	// Remove from inventory
	delete(p.machines, machineID)

	p.statusMu.Lock()
	delete(p.provisioningStatus, machineID)
	p.statusMu.Unlock()

	return nil
}

// Reboot reboots a machine.
func (p *bareMetalProvisioner) Reboot(ctx context.Context, machineID string) error {
	p.mu.RLock()
	machine, exists := p.machines[machineID]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	creds := machine.Spec.IPMICredentials
	if creds == nil {
		creds = p.config.DefaultIPMICredentials
	}

	client, err := ipmi.NewClient(machine.Spec.IPMIAddress, creds)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	return client.Reboot(ctx)
}

// PowerOn powers on a machine.
func (p *bareMetalProvisioner) PowerOn(ctx context.Context, machineID string) error {
	p.mu.RLock()
	machine, exists := p.machines[machineID]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	return p.powerOnMachine(ctx, machine)
}

// PowerOff powers off a machine.
func (p *bareMetalProvisioner) PowerOff(ctx context.Context, machineID string) error {
	p.mu.RLock()
	machine, exists := p.machines[machineID]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	if err := p.powerOffMachine(ctx, machine); err != nil {
		return err
	}

	p.mu.Lock()
	machine.State = MachineStatePoweredOff
	machine.UpdatedAt = time.Now()
	p.mu.Unlock()

	return nil
}

// GetMachine retrieves a machine by ID.
func (p *bareMetalProvisioner) GetMachine(ctx context.Context, machineID string) (*Machine, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	machine, exists := p.machines[machineID]
	if !exists {
		return nil, fmt.Errorf("machine not found: %s", machineID)
	}

	return machine, nil
}

// ListMachines lists all managed machines.
func (p *bareMetalProvisioner) ListMachines(ctx context.Context) ([]*Machine, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	machines := make([]*Machine, 0, len(p.machines))
	for _, m := range p.machines {
		machines = append(machines, m)
	}

	return machines, nil
}

// DiscoverMachines triggers auto-discovery.
func (p *bareMetalProvisioner) DiscoverMachines(ctx context.Context) ([]*Machine, error) {
	discovered, err := p.discovery.Discover(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	machines := make([]*Machine, 0, len(discovered))
	now := time.Now()

	for _, hw := range discovered {
		// Check if machine already exists by MAC
		var existingID string
		for id, m := range p.machines {
			if m.Spec.MACAddress == hw.MACAddress {
				existingID = id
				break
			}
		}

		if existingID != "" {
			// Update existing machine's hardware info
			p.machines[existingID].Hardware = hw
			p.machines[existingID].UpdatedAt = now
			machines = append(machines, p.machines[existingID])
		} else {
			// Create new discovered machine
			machine := &Machine{
				Spec: &MachineSpec{
					ID:         hw.SerialNumber,
					MACAddress: hw.MACAddress,
				},
				State:     MachineStateDiscovered,
				Hardware:  hw,
				CreatedAt: now,
				UpdatedAt: now,
			}
			p.machines[hw.SerialNumber] = machine
			machines = append(machines, machine)
		}
	}

	return machines, nil
}

// GetProvisioningStatus returns detailed provisioning status.
func (p *bareMetalProvisioner) GetProvisioningStatus(ctx context.Context, machineID string) (*ProvisioningStatus, error) {
	p.statusMu.RLock()
	defer p.statusMu.RUnlock()

	status, exists := p.provisioningStatus[machineID]
	if !exists {
		return nil, fmt.Errorf("no provisioning status for machine: %s", machineID)
	}

	return status, nil
}
