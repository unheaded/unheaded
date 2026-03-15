// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pxe

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// DHCPProxy handles DHCP proxy/options for PXE boot.
// This works alongside an existing DHCP server to provide PXE boot options.
type DHCPProxy struct {
	iface string

	// Client configurations
	clients map[string]*DHCPClientConfig
	mu      sync.RWMutex

	running bool
	done    chan struct{}
}

// DHCPClientConfig holds DHCP configuration for a client.
type DHCPClientConfig struct {
	MACAddress   string `json:"mac_address"`
	IPAddress    string `json:"ip_address"`
	TFTPServer   string `json:"tftp_server"`
	BootFilename string `json:"boot_filename"`
	NextServer   string `json:"next_server"`
}

// DHCPOption represents a DHCP option.
type DHCPOption struct {
	Code  uint8
	Value []byte
}

// Common DHCP option codes for PXE.
const (
	DHCPOptionSubnetMask         uint8 = 1
	DHCPOptionRouter             uint8 = 3
	DHCPOptionDNS                uint8 = 6
	DHCPOptionHostname           uint8 = 12
	DHCPOptionBootFileSize       uint8 = 13
	DHCPOptionDomainName         uint8 = 15
	DHCPOptionBroadcast          uint8 = 28
	DHCPOptionNTPServers         uint8 = 42
	DHCPOptionVendorSpecific     uint8 = 43
	DHCPOptionRequestedIP        uint8 = 50
	DHCPOptionLeaseTime          uint8 = 51
	DHCPOptionMessageType        uint8 = 53
	DHCPOptionServerID           uint8 = 54
	DHCPOptionParameterList      uint8 = 55
	DHCPOptionMaxMessageSize     uint8 = 57
	DHCPOptionRenewalTime        uint8 = 58
	DHCPOptionRebindingTime      uint8 = 59
	DHCPOptionVendorClass        uint8 = 60
	DHCPOptionClientID           uint8 = 61
	DHCPOptionTFTPServerName     uint8 = 66
	DHCPOptionBootFileName       uint8 = 67
	DHCPOptionUserClass          uint8 = 77
	DHCPOptionClientArch         uint8 = 93
	DHCPOptionClientNDI          uint8 = 94
	DHCPOptionUUID               uint8 = 97
	DHCPOptionPXEDiscoveryCtrl   uint8 = 128
	DHCPOptionPXEBootServer      uint8 = 129
	DHCPOptionPXEBootMenu        uint8 = 130
	DHCPOptionPXEMenuPrompt      uint8 = 131
)

// Client architecture types.
const (
	ArchIntelx86PC       uint16 = 0
	ArchNECPC98          uint16 = 1
	ArchEFIIA32          uint16 = 6
	ArchEFIBC            uint16 = 7
	ArchEFIXscale        uint16 = 8
	ArchEFIx8664         uint16 = 9
	ArchEFIARM32         uint16 = 10
	ArchEFIARM64         uint16 = 11
)

// NewDHCPProxy creates a new DHCP proxy.
func NewDHCPProxy(iface string) (*DHCPProxy, error) {
	return NewDHCPProxyWithOptions(iface, false)
}

// NewDHCPProxyWithOptions creates a new DHCP proxy with options.
// The skipInit parameter is provided for consistency with other components but is currently unused.
func NewDHCPProxyWithOptions(iface string, skipInit bool) (*DHCPProxy, error) {
	return &DHCPProxy{
		iface:   iface,
		clients: make(map[string]*DHCPClientConfig),
		done:    make(chan struct{}),
	}, nil
}

// Start starts the DHCP proxy.
func (d *DHCPProxy) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return fmt.Errorf("proxy already running")
	}

	// In production, this would start listening for DHCP discover/request
	// packets and responding with PXE boot options
	d.running = true

	return nil
}

// Stop stops the DHCP proxy.
func (d *DHCPProxy) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	close(d.done)
	d.running = false

	return nil
}

// ConfigureClient configures DHCP options for a client.
func (d *DHCPProxy) ConfigureClient(macAddress, ipAddress, tftpServer string) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}

	if ipAddress != "" {
		if net.ParseIP(ipAddress) == nil {
			return fmt.Errorf("invalid IP address: %s", ipAddress)
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.clients[mac.String()] = &DHCPClientConfig{
		MACAddress:   mac.String(),
		IPAddress:    ipAddress,
		TFTPServer:   tftpServer,
		BootFilename: "pxelinux.0",
		NextServer:   tftpServer,
	}

	return nil
}

// RemoveClient removes DHCP configuration for a client.
func (d *DHCPProxy) RemoveClient(macAddress string) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.clients, mac.String())

	return nil
}

// GetClientConfig returns the DHCP configuration for a client.
func (d *DHCPProxy) GetClientConfig(macAddress string) (*DHCPClientConfig, error) {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid MAC address: %w", err)
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	config, exists := d.clients[mac.String()]
	if !exists {
		return nil, fmt.Errorf("client not found: %s", macAddress)
	}

	return config, nil
}

// BuildPXEOptions builds DHCP options for a PXE client.
func (d *DHCPProxy) BuildPXEOptions(clientArch uint16, config *DHCPClientConfig) []DHCPOption {
	var options []DHCPOption

	// Boot filename based on architecture
	bootFile := d.getBootFilename(clientArch)

	// TFTP server name
	if config.TFTPServer != "" {
		options = append(options, DHCPOption{
			Code:  DHCPOptionTFTPServerName,
			Value: []byte(config.TFTPServer),
		})
	}

	// Boot filename
	options = append(options, DHCPOption{
		Code:  DHCPOptionBootFileName,
		Value: []byte(bootFile),
	})

	// Vendor class identifier (PXEClient)
	options = append(options, DHCPOption{
		Code:  DHCPOptionVendorClass,
		Value: []byte("PXEClient"),
	})

	return options
}

// getBootFilename returns the appropriate boot filename for the client architecture.
func (d *DHCPProxy) getBootFilename(clientArch uint16) string {
	switch clientArch {
	case ArchEFIx8664:
		return "grubx64.efi"
	case ArchEFIBC:
		return "grubx64.efi"
	case ArchEFIIA32:
		return "grubia32.efi"
	case ArchEFIARM64:
		return "grubaa64.efi"
	default:
		return "pxelinux.0"
	}
}

// ListClients returns all configured clients.
func (d *DHCPProxy) ListClients() []*DHCPClientConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()

	clients := make([]*DHCPClientConfig, 0, len(d.clients))
	for _, c := range d.clients {
		clients = append(clients, c)
	}

	return clients
}
