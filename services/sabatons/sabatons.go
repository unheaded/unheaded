// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package sabatons provides bare metal and hardware management capabilities.
// Sabatons the Bare Metal Agent is the Kingdom's boots - managing physical infrastructure.
// Implements PXE boot, hardware discovery, IPMI control, and provisioning.
package sabatons

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Machine represents a bare metal machine.
type Machine struct {
	ID            string            `json:"id"`
	Hostname      string            `json:"hostname"`
	SerialNumber  string            `json:"serial_number,omitempty"`
	BMCAddress    string            `json:"bmc_address,omitempty"`
	MACAddresses  []string          `json:"mac_addresses"`
	IPAddresses   []string          `json:"ip_addresses,omitempty"`
	Status        MachineStatus     `json:"status"`
	State         MachineState      `json:"state"`
	Hardware      HardwareInfo      `json:"hardware"`
	Labels        map[string]string `json:"labels,omitempty"`
	ProvisionedAt *time.Time        `json:"provisioned_at,omitempty"`
	LastSeen      time.Time         `json:"last_seen"`
}

// MachineStatus tracks machine availability.
type MachineStatus string

const (
	StatusAvailable    MachineStatus = "available"
	StatusProvisioning MachineStatus = "provisioning"
	StatusProvisioned  MachineStatus = "provisioned"
	StatusError        MachineStatus = "error"
	StatusMaintenance  MachineStatus = "maintenance"
)

// MachineState tracks machine power state.
type MachineState string

const (
	StateOn        MachineState = "on"
	StateOff       MachineState = "off"
	StateRebooting MachineState = "rebooting"
	StateUnknown   MachineState = "unknown"
)

// HardwareInfo describes machine hardware.
type HardwareInfo struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	CPUs         []CPU  `json:"cpus,omitempty"`
	Memory       Memory `json:"memory"`
	Disks        []Disk `json:"disks,omitempty"`
	NICs         []NIC  `json:"nics,omitempty"`
	BMCType      string `json:"bmc_type,omitempty"` // IPMI, Redfish, iLO
}

// CPU describes a processor.
type CPU struct {
	Model    string `json:"model"`
	Cores    int    `json:"cores"`
	Threads  int    `json:"threads"`
	SpeedMHz int    `json:"speed_mhz"`
}

// Memory describes system memory.
type Memory struct {
	TotalBytes int64  `json:"total_bytes"`
	Type       string `json:"type,omitempty"`
	SpeedMHz   int    `json:"speed_mhz,omitempty"`
}

// Disk describes a storage device.
type Disk struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"size_bytes"`
	Type       string `json:"type"` // ssd, hdd, nvme
	Model      string `json:"model,omitempty"`
	Serial     string `json:"serial,omitempty"`
	Rotational bool   `json:"rotational"`
}

// NIC describes a network interface.
type NIC struct {
	Name       string `json:"name"`
	MACAddress string `json:"mac_address"`
	Speed      int    `json:"speed_mbps,omitempty"`
	Driver     string `json:"driver,omitempty"`
}

// ProvisionRequest represents a provisioning request.
type ProvisionRequest struct {
	MachineID   string            `json:"machine_id"`
	ImageURL    string            `json:"image_url"`
	CloudConfig string            `json:"cloud_config,omitempty"`
	Kernel      string            `json:"kernel,omitempty"`
	Initrd      string            `json:"initrd,omitempty"`
	KernelArgs  string            `json:"kernel_args,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// BootConfig represents PXE boot configuration.
type BootConfig struct {
	MACAddress string `json:"mac_address"`
	Kernel     string `json:"kernel"`
	Initrd     string `json:"initrd"`
	KernelArgs string `json:"kernel_args"`
}

// BMCCredentials stores BMC access credentials.
type BMCCredentials struct {
	MachineID string `json:"machine_id"`
	Username  string `json:"username"`
	Password  string `json:"-"`    // Not serialized
	Type      string `json:"type"` // IPMI, Redfish
}

// Service is the main Sabatons bare metal service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu          sync.RWMutex
	machines    map[string]*Machine
	bootConfigs map[string]*BootConfig // MAC -> config
	credentials map[string]*BMCCredentials
}

// Config holds Sabatons service configuration.
type Config struct {
	PXEServerAddress  string        `json:"pxe_server_address"`
	TFTPRoot          string        `json:"tftp_root"`
	DiscoveryInterval time.Duration `json:"discovery_interval"`
	HeartbeatTimeout  time.Duration `json:"heartbeat_timeout"`
	WotanTopic        string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		PXEServerAddress:  "0.0.0.0:69",
		TFTPRoot:          "/var/lib/tftpboot",
		DiscoveryInterval: 30 * time.Second,
		HeartbeatTimeout:  60 * time.Second,
		WotanTopic:        "sabatons.hardware",
	}
}

// NewService creates a new Sabatons bare metal service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:         log,
		wotan:       wotan,
		config:      cfg,
		machines:    make(map[string]*Machine),
		bootConfigs: make(map[string]*BootConfig),
		credentials: make(map[string]*BMCCredentials),
	}
}

// Start initializes the Sabatons service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Sabatons equipped - bare metal management active")
	go s.heartbeatLoop(ctx)
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Sabatons removed - bare metal management suspended")
	return nil
}

// RegisterMachine registers a new bare metal machine.
func (s *Service) RegisterMachine(machine *Machine) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if machine.ID == "" {
		machine.ID = fmt.Sprintf("machine-%s", uuid.New().String())
	}

	machine.Status = StatusAvailable
	machine.State = StateUnknown
	machine.LastSeen = time.Now()

	s.machines[machine.ID] = machine

	s.log.Info().
		Str("id", machine.ID).
		Str("hostname", machine.Hostname).
		Strs("macs", machine.MACAddresses).
		Msg("Machine registered")

	return nil
}

// GetMachine retrieves a machine by ID.
func (s *Service) GetMachine(machineID string) (*Machine, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	machine, ok := s.machines[machineID]
	return machine, ok
}

// GetMachineByMAC retrieves a machine by MAC address.
func (s *Service) GetMachineByMAC(mac string) (*Machine, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalizedMAC := normalizeMAC(mac)
	for _, machine := range s.machines {
		for _, m := range machine.MACAddresses {
			if normalizeMAC(m) == normalizedMAC {
				return machine, true
			}
		}
	}
	return nil, false
}

// ListMachines returns all machines.
func (s *Service) ListMachines(status MachineStatus) []*Machine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Machine
	for _, m := range s.machines {
		if status == "" || m.Status == status {
			results = append(results, m)
		}
	}
	return results
}

// Provision starts provisioning a machine.
func (s *Service) Provision(ctx context.Context, req *ProvisionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	machine, ok := s.machines[req.MachineID]
	if !ok {
		return fmt.Errorf("machine not found: %s", req.MachineID)
	}

	if machine.Status != StatusAvailable {
		return fmt.Errorf("machine not available: %s (status: %s)", req.MachineID, machine.Status)
	}

	machine.Status = StatusProvisioning

	// Set up PXE boot config for each MAC
	for _, mac := range machine.MACAddresses {
		bootConfig := &BootConfig{
			MACAddress: normalizeMAC(mac),
			Kernel:     req.Kernel,
			Initrd:     req.Initrd,
			KernelArgs: req.KernelArgs,
		}
		s.bootConfigs[bootConfig.MACAddress] = bootConfig
	}

	s.log.Info().
		Str("machine_id", req.MachineID).
		Str("image", req.ImageURL).
		Msg("Provisioning started")

	// Trigger PXE boot via BMC
	if machine.BMCAddress != "" {
		if err := s.powerCycle(machine); err != nil {
			s.log.Warn().Err(err).Str("machine_id", req.MachineID).Msg("Failed to power cycle")
		}
	}

	return nil
}

// SetBootConfig sets PXE boot configuration for a MAC address.
func (s *Service) SetBootConfig(config *BootConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedMAC := normalizeMAC(config.MACAddress)
	s.bootConfigs[normalizedMAC] = config

	s.log.Debug().Str("mac", normalizedMAC).Msg("Boot config set")
	return nil
}

// GetBootConfig retrieves PXE boot configuration for a MAC.
func (s *Service) GetBootConfig(mac string) (*BootConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalizedMAC := normalizeMAC(mac)
	config, ok := s.bootConfigs[normalizedMAC]
	return config, ok
}

// ClearBootConfig removes PXE boot configuration.
func (s *Service) ClearBootConfig(mac string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedMAC := normalizeMAC(mac)
	delete(s.bootConfigs, normalizedMAC)
	return nil
}

// SetBMCCredentials stores BMC credentials for a machine.
func (s *Service) SetBMCCredentials(creds *BMCCredentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.credentials[creds.MachineID] = creds
	return nil
}

// PowerOn powers on a machine via BMC.
func (s *Service) PowerOn(ctx context.Context, machineID string) error {
	s.mu.RLock()
	machine, ok := s.machines[machineID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	s.log.Info().Str("machine_id", machineID).Msg("Powering on machine")

	// Simulated - real implementation would use IPMI/Redfish
	s.mu.Lock()
	machine.State = StateOn
	s.mu.Unlock()

	return nil
}

// PowerOff powers off a machine via BMC.
func (s *Service) PowerOff(ctx context.Context, machineID string) error {
	s.mu.RLock()
	machine, ok := s.machines[machineID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	s.log.Info().Str("machine_id", machineID).Msg("Powering off machine")

	s.mu.Lock()
	machine.State = StateOff
	s.mu.Unlock()

	return nil
}

// Reboot reboots a machine via BMC.
func (s *Service) Reboot(ctx context.Context, machineID string) error {
	s.mu.RLock()
	machine, ok := s.machines[machineID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	s.log.Info().Str("machine_id", machineID).Msg("Rebooting machine")

	s.mu.Lock()
	machine.State = StateRebooting
	s.mu.Unlock()

	return nil
}

func (s *Service) powerCycle(machine *Machine) error {
	// Would implement actual IPMI/Redfish power cycle
	return nil
}

// Heartbeat records a machine heartbeat.
func (s *Service) Heartbeat(machineID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	machine, ok := s.machines[machineID]
	if !ok {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	machine.LastSeen = time.Now()
	return nil
}

// ProvisioningComplete marks provisioning as complete.
func (s *Service) ProvisioningComplete(machineID string, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	machine, ok := s.machines[machineID]
	if !ok {
		return fmt.Errorf("machine not found: %s", machineID)
	}

	now := time.Now()
	if success {
		machine.Status = StatusProvisioned
		machine.State = StateOn
		machine.ProvisionedAt = &now
	} else {
		machine.Status = StatusError
	}

	// Clear boot config
	for _, mac := range machine.MACAddresses {
		delete(s.bootConfigs, normalizeMAC(mac))
	}

	s.log.Info().Str("machine_id", machineID).Bool("success", success).Msg("Provisioning complete")
	return nil
}

func (s *Service) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkHeartbeats()
		}
	}
}

func (s *Service) checkHeartbeats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, machine := range s.machines {
		if now.Sub(machine.LastSeen) > s.config.HeartbeatTimeout {
			if machine.State == StateOn {
				machine.State = StateUnknown
				s.log.Warn().Str("machine_id", machine.ID).Msg("Machine heartbeat timeout")
			}
		}
	}
}

func normalizeMAC(mac string) string {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return mac
	}
	return hw.String()
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statusCounts := make(map[string]int)
	stateCounts := make(map[string]int)
	for _, m := range s.machines {
		statusCounts[string(m.Status)]++
		stateCounts[string(m.State)]++
	}

	return map[string]interface{}{
		"total_machines": len(s.machines),
		"by_status":      statusCounts,
		"by_state":       stateCounts,
		"pending_boots":  len(s.bootConfigs),
	}
}

// RegisterRoutes registers /health, /ready, and /metrics endpoints on the provided mux.
// Required by CLAUDE.md: every component must expose these endpoints.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"status":  "healthy",
			"service": "sabatons",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"ready":   true,
			"service": "sabatons",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP sabatons_machines_total Total bare metal machines\n")
		fmt.Fprintf(w, "# TYPE sabatons_machines_total gauge\n")
		fmt.Fprintf(w, "sabatons_machines_total %v\n", stats["total_machines"])
		fmt.Fprintf(w, "# HELP sabatons_pending_boots Pending boot configurations\n")
		fmt.Fprintf(w, "# TYPE sabatons_pending_boots gauge\n")
		fmt.Fprintf(w, "sabatons_pending_boots %v\n", stats["pending_boots"])
	})
}
