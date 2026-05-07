// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package inventory provides hardware inventory tracking for bare metal machines.
package inventory

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Hardware represents discovered hardware information.
type Hardware struct {
	// Identity
	SerialNumber string `json:"serial_number"`
	UUID         string `json:"uuid"`
	MACAddress   string `json:"mac_address"`
	Manufacturer string `json:"manufacturer"`
	ProductName  string `json:"product_name"`

	// Compute
	CPUs    []CPU  `json:"cpus"`
	Memory  Memory `json:"memory"`
	Storage []Disk `json:"storage"`
	NICs    []NIC  `json:"nics"`
	GPUs    []GPU  `json:"gpus,omitempty"`

	// Management
	BMC *BMCInfo `json:"bmc,omitempty"`

	// Discovery metadata
	DiscoveredAt time.Time `json:"discovered_at"`
	LastUpdated  time.Time `json:"last_updated"`
}

// CPU represents a CPU.
type CPU struct {
	Model        string   `json:"model"`
	Vendor       string   `json:"vendor"`
	Cores        int      `json:"cores"`
	Threads      int      `json:"threads"`
	Architecture string   `json:"architecture"`
	ClockMHz     float64  `json:"clock_mhz"`
	Flags        []string `json:"flags,omitempty"`
}

// Memory represents system memory.
type Memory struct {
	TotalBytes int64  `json:"total_bytes"`
	DIMMs      []DIMM `json:"dimms"`
}

// DIMM represents a memory module.
type DIMM struct {
	Slot         string `json:"slot"`
	SizeBytes    int64  `json:"size_bytes"`
	Type         string `json:"type"`
	SpeedMHz     int    `json:"speed_mhz"`
	Manufacturer string `json:"manufacturer"`
	SerialNumber string `json:"serial_number"`
}

// Disk represents a storage device.
type Disk struct {
	Name         string `json:"name"`
	Model        string `json:"model"`
	Vendor       string `json:"vendor"`
	SerialNumber string `json:"serial_number"`
	SizeBytes    int64  `json:"size_bytes"`
	Type         string `json:"type"` // hdd, ssd, nvme
	Rotational   bool   `json:"rotational"`
	Transport    string `json:"transport"` // sata, sas, nvme
	WWN          string `json:"wwn,omitempty"`
	HCTL         string `json:"hctl,omitempty"`
}

// NIC represents a network interface.
type NIC struct {
	Name       string `json:"name"`
	MACAddress string `json:"mac_address"`
	SpeedMbps  int    `json:"speed_mbps"`
	Driver     string `json:"driver"`
	Vendor     string `json:"vendor"`
	PCIAddress string `json:"pci_address,omitempty"`
	SRIOVCaps  int    `json:"sriov_caps,omitempty"`
}

// GPU represents a GPU.
type GPU struct {
	Model      string `json:"model"`
	Vendor     string `json:"vendor"`
	MemoryMB   int    `json:"memory_mb"`
	PCIAddress string `json:"pci_address"`
	Driver     string `json:"driver"`
}

// BMCInfo represents BMC/IPMI information.
type BMCInfo struct {
	Address         string `json:"address"`
	Type            string `json:"type"` // ipmi, redfish
	MACAddress      string `json:"mac_address"`
	FirmwareVersion string `json:"firmware_version"`
}

// Inventory manages hardware inventory.
type Inventory struct {
	hardware map[string]*Hardware // keyed by serial number
	byMAC    map[string]string    // MAC -> serial number
	byUUID   map[string]string    // UUID -> serial number
	mu       sync.RWMutex
}

// NewInventory creates a new inventory.
func NewInventory() *Inventory {
	return &Inventory{
		hardware: make(map[string]*Hardware),
		byMAC:    make(map[string]string),
		byUUID:   make(map[string]string),
	}
}

// Add adds or updates hardware in the inventory.
func (i *Inventory) Add(hw *Hardware) error {
	if hw == nil {
		return fmt.Errorf("hardware cannot be nil")
	}

	if hw.SerialNumber == "" {
		return fmt.Errorf("serial number is required")
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	now := time.Now()

	existing, exists := i.hardware[hw.SerialNumber]
	if exists {
		hw.DiscoveredAt = existing.DiscoveredAt
	} else {
		hw.DiscoveredAt = now
	}
	hw.LastUpdated = now

	i.hardware[hw.SerialNumber] = hw

	// Update indexes
	if hw.MACAddress != "" {
		i.byMAC[hw.MACAddress] = hw.SerialNumber
	}
	if hw.UUID != "" {
		i.byUUID[hw.UUID] = hw.SerialNumber
	}

	return nil
}

// Get returns hardware by serial number.
func (i *Inventory) Get(serialNumber string) (*Hardware, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	hw, exists := i.hardware[serialNumber]
	if !exists {
		return nil, fmt.Errorf("hardware not found: %s", serialNumber)
	}

	return hw, nil
}

// GetByMAC returns hardware by MAC address.
func (i *Inventory) GetByMAC(macAddress string) (*Hardware, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	serial, exists := i.byMAC[macAddress]
	if !exists {
		return nil, fmt.Errorf("hardware not found for MAC: %s", macAddress)
	}

	return i.hardware[serial], nil
}

// GetByUUID returns hardware by UUID.
func (i *Inventory) GetByUUID(uuid string) (*Hardware, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	serial, exists := i.byUUID[uuid]
	if !exists {
		return nil, fmt.Errorf("hardware not found for UUID: %s", uuid)
	}

	return i.hardware[serial], nil
}

// Remove removes hardware from the inventory.
func (i *Inventory) Remove(serialNumber string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	hw, exists := i.hardware[serialNumber]
	if !exists {
		return fmt.Errorf("hardware not found: %s", serialNumber)
	}

	// Remove from indexes
	if hw.MACAddress != "" {
		delete(i.byMAC, hw.MACAddress)
	}
	if hw.UUID != "" {
		delete(i.byUUID, hw.UUID)
	}

	delete(i.hardware, serialNumber)

	return nil
}

// List returns all hardware in the inventory.
func (i *Inventory) List() []*Hardware {
	i.mu.RLock()
	defer i.mu.RUnlock()

	result := make([]*Hardware, 0, len(i.hardware))
	for _, hw := range i.hardware {
		result = append(result, hw)
	}

	return result
}

// Filter returns hardware matching the given filter.
func (i *Inventory) Filter(filter *HardwareFilter) []*Hardware {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var result []*Hardware

	for _, hw := range i.hardware {
		if filter.Matches(hw) {
			result = append(result, hw)
		}
	}

	return result
}

// HardwareFilter defines criteria for filtering hardware.
type HardwareFilter struct {
	MinCPUs         int    `json:"min_cpus,omitempty"`
	MinCores        int    `json:"min_cores,omitempty"`
	MinMemoryBytes  int64  `json:"min_memory_bytes,omitempty"`
	MinStorageBytes int64  `json:"min_storage_bytes,omitempty"`
	DiskType        string `json:"disk_type,omitempty"`
	HasGPU          *bool  `json:"has_gpu,omitempty"`
	Manufacturer    string `json:"manufacturer,omitempty"`
}

// Matches returns true if the hardware matches the filter.
func (f *HardwareFilter) Matches(hw *Hardware) bool {
	if f.MinCPUs > 0 && len(hw.CPUs) < f.MinCPUs {
		return false
	}

	if f.MinCores > 0 {
		totalCores := 0
		for _, cpu := range hw.CPUs {
			totalCores += cpu.Cores
		}
		if totalCores < f.MinCores {
			return false
		}
	}

	if f.MinMemoryBytes > 0 && hw.Memory.TotalBytes < f.MinMemoryBytes {
		return false
	}

	if f.MinStorageBytes > 0 {
		totalStorage := int64(0)
		for _, disk := range hw.Storage {
			totalStorage += disk.SizeBytes
		}
		if totalStorage < f.MinStorageBytes {
			return false
		}
	}

	if f.DiskType != "" {
		found := false
		for _, disk := range hw.Storage {
			if disk.Type == f.DiskType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if f.HasGPU != nil {
		hasGPU := len(hw.GPUs) > 0
		if *f.HasGPU != hasGPU {
			return false
		}
	}

	if f.Manufacturer != "" && hw.Manufacturer != f.Manufacturer {
		return false
	}

	return true
}

// Summary returns a summary of the inventory.
func (i *Inventory) Summary() *InventorySummary {
	i.mu.RLock()
	defer i.mu.RUnlock()

	summary := &InventorySummary{
		TotalMachines: len(i.hardware),
		Manufacturers: make(map[string]int),
	}

	for _, hw := range i.hardware {
		summary.TotalCores += i.countCores(hw)
		summary.TotalMemoryBytes += hw.Memory.TotalBytes
		summary.TotalStorageBytes += i.countStorage(hw)

		if hw.Manufacturer != "" {
			summary.Manufacturers[hw.Manufacturer]++
		}

		if len(hw.GPUs) > 0 {
			summary.MachinesWithGPU++
		}
	}

	return summary
}

func (i *Inventory) countCores(hw *Hardware) int {
	total := 0
	for _, cpu := range hw.CPUs {
		total += cpu.Cores
	}
	return total
}

func (i *Inventory) countStorage(hw *Hardware) int64 {
	var total int64
	for _, disk := range hw.Storage {
		total += disk.SizeBytes
	}
	return total
}

// InventorySummary provides aggregate inventory statistics.
type InventorySummary struct {
	TotalMachines     int            `json:"total_machines"`
	TotalCores        int            `json:"total_cores"`
	TotalMemoryBytes  int64          `json:"total_memory_bytes"`
	TotalStorageBytes int64          `json:"total_storage_bytes"`
	MachinesWithGPU   int            `json:"machines_with_gpu"`
	Manufacturers     map[string]int `json:"manufacturers"`
}

// Export exports the inventory to JSON.
func (i *Inventory) Export() ([]byte, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return json.MarshalIndent(i.hardware, "", "  ")
}

// Import imports inventory from JSON.
func (i *Inventory) Import(data []byte) error {
	var hardware map[string]*Hardware
	if err := json.Unmarshal(data, &hardware); err != nil {
		return fmt.Errorf("unmarshaling inventory: %w", err)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	for serial, hw := range hardware {
		hw.SerialNumber = serial
		i.hardware[serial] = hw

		if hw.MACAddress != "" {
			i.byMAC[hw.MACAddress] = serial
		}
		if hw.UUID != "" {
			i.byUUID[hw.UUID] = serial
		}
	}

	return nil
}
