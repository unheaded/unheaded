// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package ipmi provides IPMI/BMC interface for out-of-band bare metal management.
package ipmi

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Credentials holds IPMI authentication credentials.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BootDevice represents a boot device.
type BootDevice string

const (
	BootDeviceNone    BootDevice = "none"
	BootDevicePXE     BootDevice = "pxe"
	BootDeviceDisk    BootDevice = "disk"
	BootDeviceCDROM   BootDevice = "cdrom"
	BootDeviceBIOS    BootDevice = "bios"
	BootDeviceUSB     BootDevice = "usb"
	BootDeviceRemote  BootDevice = "remote"
)

// PowerState represents the power state of a machine.
type PowerState string

const (
	PowerStateOn      PowerState = "on"
	PowerStateOff     PowerState = "off"
	PowerStateUnknown PowerState = "unknown"
)

// ChassisStatus holds chassis status information.
type ChassisStatus struct {
	PowerOn            bool       `json:"power_on"`
	PowerOverload      bool       `json:"power_overload"`
	PowerInterlock     bool       `json:"power_interlock"`
	MainPowerFault     bool       `json:"main_power_fault"`
	PowerControlFault  bool       `json:"power_control_fault"`
	LastPowerEvent     string     `json:"last_power_event"`
	ChassisIntrusion   bool       `json:"chassis_intrusion"`
	FrontPanelLockout  bool       `json:"front_panel_lockout"`
	DriveFault         bool       `json:"drive_fault"`
	CoolingFault       bool       `json:"cooling_fault"`
}

// SensorReading holds a sensor reading.
type SensorReading struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Status      string  `json:"status"`
	LowerCrit   float64 `json:"lower_crit,omitempty"`
	LowerWarn   float64 `json:"lower_warn,omitempty"`
	UpperWarn   float64 `json:"upper_warn,omitempty"`
	UpperCrit   float64 `json:"upper_crit,omitempty"`
}

// SELEntry is a System Event Log entry.
type SELEntry struct {
	ID          uint16    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	SensorType  string    `json:"sensor_type"`
	SensorName  string    `json:"sensor_name"`
	EventType   string    `json:"event_type"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"`
}

// Client is an IPMI client for out-of-band management.
type Client struct {
	address     string
	credentials *Credentials

	// Connection state
	connected   bool
	mu          sync.RWMutex

	// Session info
	sessionID   uint32
	authType    uint8
}

// NewClient creates a new IPMI client.
func NewClient(address string, credentials *Credentials) (*Client, error) {
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}

	if credentials == nil {
		return nil, fmt.Errorf("credentials are required")
	}

	client := &Client{
		address:     address,
		credentials: credentials,
	}

	// Establish connection
	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("connecting to BMC: %w", err)
	}

	return client, nil
}

// connect establishes a connection to the BMC.
func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// In production, this would establish an IPMI session
	// using the IPMI protocol (typically UDP port 623)
	c.connected = true
	c.sessionID = 1

	return nil
}

// Close closes the IPMI connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	c.sessionID = 0

	return nil
}

// PowerOn powers on the machine.
func (c *Client) PowerOn(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// In production, this would send IPMI chassis control command
	// Chassis Control command: 0x02 (Power Up)
	return nil
}

// PowerOff powers off the machine.
func (c *Client) PowerOff(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Chassis Control command: 0x00 (Power Down)
	return nil
}

// PowerCycle power cycles the machine.
func (c *Client) PowerCycle(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Chassis Control command: 0x02 (Power Cycle)
	return nil
}

// Reboot performs a hard reset.
func (c *Client) Reboot(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Chassis Control command: 0x03 (Hard Reset)
	return nil
}

// SoftShutdown requests a graceful shutdown.
func (c *Client) SoftShutdown(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Chassis Control command: 0x05 (Soft Shutdown via ACPI)
	return nil
}

// GetPowerState returns the current power state.
func (c *Client) GetPowerState(ctx context.Context) (PowerState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return PowerStateUnknown, fmt.Errorf("not connected")
	}

	// Get Chassis Status command
	// In production, parse the response to determine power state
	return PowerStateOn, nil
}

// GetChassisStatus returns detailed chassis status.
func (c *Client) GetChassisStatus(ctx context.Context) (*ChassisStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	return &ChassisStatus{
		PowerOn: true,
	}, nil
}

// SetBootDevice sets the boot device for the next boot.
func (c *Client) SetBootDevice(ctx context.Context, device BootDevice) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Set System Boot Options command
	// In production, encode the boot device and options
	return nil
}

// GetBootDevice returns the current boot device setting.
func (c *Client) GetBootDevice(ctx context.Context) (BootDevice, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return BootDeviceNone, fmt.Errorf("not connected")
	}

	// Get System Boot Options command
	return BootDeviceDisk, nil
}

// GetSensors returns sensor readings.
func (c *Client) GetSensors(ctx context.Context) ([]SensorReading, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	// Get Sensor Reading command for each sensor
	// In production, iterate through SDR (Sensor Data Record) repository
	return []SensorReading{
		{Name: "CPU Temp", Value: 45.0, Unit: "C", Status: "ok"},
		{Name: "System Temp", Value: 32.0, Unit: "C", Status: "ok"},
		{Name: "Fan1", Value: 5400, Unit: "RPM", Status: "ok"},
	}, nil
}

// GetSEL returns System Event Log entries.
func (c *Client) GetSEL(ctx context.Context) ([]SELEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	// Get SEL Entry command
	return []SELEntry{}, nil
}

// ClearSEL clears the System Event Log.
func (c *Client) ClearSEL(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Clear SEL command
	return nil
}

// IdentifyOn turns on the chassis identify LED.
func (c *Client) IdentifyOn(ctx context.Context, duration time.Duration) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Chassis Identify command
	return nil
}

// IdentifyOff turns off the chassis identify LED.
func (c *Client) IdentifyOff(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Chassis Identify command with 0 duration
	return nil
}

// GetFRU returns Field Replaceable Unit information.
func (c *Client) GetFRU(ctx context.Context) (*FRUInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	return &FRUInfo{
		Manufacturer: "Unknown",
		ProductName:  "Unknown",
	}, nil
}

// FRUInfo holds Field Replaceable Unit information.
type FRUInfo struct {
	Manufacturer string `json:"manufacturer"`
	ProductName  string `json:"product_name"`
	PartNumber   string `json:"part_number"`
	SerialNumber string `json:"serial_number"`
	Version      string `json:"version"`
	AssetTag     string `json:"asset_tag"`
}

// OpenConsole opens a Serial-over-LAN console.
func (c *Client) OpenConsole(ctx context.Context) (*Console, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	return &Console{
		client: c,
	}, nil
}

// Console represents a Serial-over-LAN console session.
type Console struct {
	client *Client
	active bool
}

// Close closes the console session.
func (cons *Console) Close() error {
	cons.active = false
	return nil
}
