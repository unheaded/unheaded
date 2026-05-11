// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pxe provides PXE boot server functionality for bare metal provisioning.
package pxe

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// Config holds PXE server configuration.
type Config struct {
	// TFTPAddress is the TFTP server bind address
	TFTPAddress string `json:"tftp_address"`

	// TFTPRoot is the root directory for TFTP files
	TFTPRoot string `json:"tftp_root"`

	// HTTPAddress is the HTTP server bind address for larger files
	HTTPAddress string `json:"http_address"`

	// HTTPRoot is the root directory for HTTP files
	HTTPRoot string `json:"http_root"`

	// DHCPInterface is the network interface for DHCP proxy
	DHCPInterface string `json:"dhcp_interface"`

	// CallbackAddress is the address for provisioning callbacks
	CallbackAddress string `json:"callback_address"`

	// DefaultKernelParams are default kernel parameters
	DefaultKernelParams string `json:"default_kernel_params"`

	// SkipDirectoryInit skips directory creation (for testing)
	SkipDirectoryInit bool `json:"skip_directory_init,omitempty"`
}

// DefaultConfig returns a default PXE configuration.
func DefaultConfig() *Config {
	return &Config{
		TFTPAddress:         ":69",
		TFTPRoot:            "/var/lib/tftpboot",
		HTTPAddress:         ":17000",
		HTTPRoot:            "/var/lib/pxe",
		DHCPInterface:       "eth0",
		CallbackAddress:     "http://provisioner:17000",
		DefaultKernelParams: "console=ttyS0,115200n8",
	}
}

// ClientConfig holds PXE configuration for a specific client.
type ClientConfig struct {
	// MACAddress identifies the client
	MACAddress string `json:"mac_address"`

	// Hostname for the client
	Hostname string `json:"hostname"`

	// IPAddress to assign via DHCP
	IPAddress string `json:"ip_address"`

	// KernelPath is the path to the kernel image
	KernelPath string `json:"kernel_path"`

	// InitrdPath is the path to the initrd
	InitrdPath string `json:"initrd_path"`

	// RootFSPath is the path to the root filesystem (for NFS or HTTP)
	RootFSPath string `json:"rootfs_path"`

	// KernelParams are additional kernel parameters
	KernelParams string `json:"kernel_params"`

	// BootMode is the boot mode (bios or uefi)
	BootMode BootMode `json:"boot_mode"`
}

// BootMode represents the boot mode.
type BootMode string

const (
	BootModeBIOS BootMode = "bios"
	BootModeUEFI BootMode = "uefi"
)

// ClientState represents the state of a PXE client.
type ClientState string

const (
	ClientStatePending    ClientState = "pending"
	ClientStateBooting    ClientState = "booting"
	ClientStateInstalling ClientState = "installing"
	ClientStateComplete   ClientState = "complete"
	ClientStateFailed     ClientState = "failed"
)

// Client represents a PXE boot client.
type Client struct {
	Config    *ClientConfig `json:"config"`
	State     ClientState   `json:"state"`
	FirstSeen time.Time     `json:"first_seen"`
	LastSeen  time.Time     `json:"last_seen"`
	BootCount int           `json:"boot_count"`
	Error     string        `json:"error,omitempty"`
}

// Server is the PXE boot server.
type Server struct {
	config *Config

	// TFTP server
	tftpServer *TFTPServer

	// DHCP proxy
	dhcpProxy *DHCPProxy

	// Registered clients
	clients map[string]*Client
	mu      sync.RWMutex

	// Boot callbacks
	bootCallbacks map[string]chan struct{}
	bootMu        sync.Mutex

	// Installation callbacks
	installCallbacks map[string]chan struct{}
	installMu        sync.Mutex

	// Running state
	running bool
	done    chan struct{}
}

// NewServer creates a new PXE server.
func NewServer(config *Config) (*Server, error) {
	if config == nil {
		config = DefaultConfig()
	}

	tftpServer, err := NewTFTPServerWithOptions(config.TFTPAddress, config.TFTPRoot, config.SkipDirectoryInit)
	if err != nil {
		return nil, fmt.Errorf("creating TFTP server: %w", err)
	}

	dhcpProxy, err := NewDHCPProxyWithOptions(config.DHCPInterface, config.SkipDirectoryInit)
	if err != nil {
		return nil, fmt.Errorf("creating DHCP proxy: %w", err)
	}

	return &Server{
		config:           config,
		tftpServer:       tftpServer,
		dhcpProxy:        dhcpProxy,
		clients:          make(map[string]*Client),
		bootCallbacks:    make(map[string]chan struct{}),
		installCallbacks: make(map[string]chan struct{}),
		done:             make(chan struct{}),
	}, nil
}

// Start starts the PXE server.
func (s *Server) Start(ctx context.Context) error {
	if s.running {
		return fmt.Errorf("server already running")
	}

	// Start TFTP server
	if err := s.tftpServer.Start(ctx); err != nil {
		return fmt.Errorf("starting TFTP server: %w", err)
	}

	// Start DHCP proxy
	if err := s.dhcpProxy.Start(ctx); err != nil {
		_ = s.tftpServer.Stop()
		return fmt.Errorf("starting DHCP proxy: %w", err)
	}

	s.running = true
	return nil
}

// Stop stops the PXE server.
func (s *Server) Stop() error {
	if !s.running {
		return nil
	}

	close(s.done)
	_ = s.tftpServer.Stop()
	_ = s.dhcpProxy.Stop()
	s.running = false

	return nil
}

// ConfigureClient configures PXE for a specific client.
func (s *Server) ConfigureClient(ctx context.Context, config *ClientConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if config.MACAddress == "" {
		return fmt.Errorf("MAC address is required")
	}

	// Normalize MAC address
	mac, err := net.ParseMAC(config.MACAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	config.MACAddress = mac.String()

	// Determine boot mode if not set
	if config.BootMode == "" {
		config.BootMode = BootModeBIOS
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	client, exists := s.clients[config.MACAddress]
	if !exists {
		client = &Client{
			Config:    config,
			State:     ClientStatePending,
			FirstSeen: now,
			LastSeen:  now,
		}
		s.clients[config.MACAddress] = client
	} else {
		client.Config = config
		client.State = ClientStatePending
		client.LastSeen = now
		client.Error = ""
	}

	// Generate PXE configuration file
	if err := s.generatePXEConfig(config); err != nil {
		return fmt.Errorf("generating PXE config: %w", err)
	}

	// Configure DHCP options
	if err := s.dhcpProxy.ConfigureClient(config.MACAddress, config.IPAddress, s.config.TFTPAddress); err != nil {
		return fmt.Errorf("configuring DHCP: %w", err)
	}

	return nil
}

// RemoveClient removes PXE configuration for a client.
func (s *Server) RemoveClient(ctx context.Context, macAddress string) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	macAddress = mac.String()

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clients, macAddress)

	// Remove PXE configuration file
	if err := s.removePXEConfig(macAddress); err != nil {
		return fmt.Errorf("removing PXE config: %w", err)
	}

	// Remove DHCP configuration
	if err := s.dhcpProxy.RemoveClient(macAddress); err != nil {
		return fmt.Errorf("removing DHCP config: %w", err)
	}

	return nil
}

// GetClient returns a client by MAC address.
func (s *Server) GetClient(macAddress string) (*Client, error) {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid MAC address: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	client, exists := s.clients[mac.String()]
	if !exists {
		return nil, fmt.Errorf("client not found: %s", macAddress)
	}

	return client, nil
}

// NotifyBoot notifies that a client has booted.
func (s *Server) NotifyBoot(macAddress string) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	macAddress = mac.String()

	s.mu.Lock()
	client, exists := s.clients[macAddress]
	if exists {
		client.State = ClientStateBooting
		client.LastSeen = time.Now()
		client.BootCount++
	}
	s.mu.Unlock()

	s.bootMu.Lock()
	if ch, ok := s.bootCallbacks[macAddress]; ok {
		close(ch)
		delete(s.bootCallbacks, macAddress)
	}
	s.bootMu.Unlock()

	return nil
}

// NotifyInstallComplete notifies that installation is complete.
func (s *Server) NotifyInstallComplete(macAddress string) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	macAddress = mac.String()

	s.mu.Lock()
	client, exists := s.clients[macAddress]
	if exists {
		client.State = ClientStateComplete
		client.LastSeen = time.Now()
	}
	s.mu.Unlock()

	s.installMu.Lock()
	if ch, ok := s.installCallbacks[macAddress]; ok {
		close(ch)
		delete(s.installCallbacks, macAddress)
	}
	s.installMu.Unlock()

	return nil
}

// WaitForBoot waits for a client to boot via PXE.
func (s *Server) WaitForBoot(ctx context.Context, macAddress string, timeout time.Duration) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	macAddress = mac.String()

	s.bootMu.Lock()
	ch := make(chan struct{})
	s.bootCallbacks[macAddress] = ch
	s.bootMu.Unlock()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-ch:
		return nil
	case <-timeoutCtx.Done():
		s.bootMu.Lock()
		delete(s.bootCallbacks, macAddress)
		s.bootMu.Unlock()
		return fmt.Errorf("timeout waiting for boot: %s", macAddress)
	}
}

// WaitForInstallation waits for installation to complete.
func (s *Server) WaitForInstallation(ctx context.Context, macAddress string, timeout time.Duration) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	macAddress = mac.String()

	s.installMu.Lock()
	ch := make(chan struct{})
	s.installCallbacks[macAddress] = ch
	s.installMu.Unlock()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-ch:
		return nil
	case <-timeoutCtx.Done():
		s.installMu.Lock()
		delete(s.installCallbacks, macAddress)
		s.installMu.Unlock()
		return fmt.Errorf("timeout waiting for installation: %s", macAddress)
	}
}

// generatePXEConfig generates the PXE configuration file for a client.
func (s *Server) generatePXEConfig(config *ClientConfig) error {
	// Convert MAC to PXE filename format (01-xx-xx-xx-xx-xx-xx)
	mac, _ := net.ParseMAC(config.MACAddress)
	pxeFilename := fmt.Sprintf("01-%s", mac.String())

	// Build kernel parameters
	kernelParams := s.config.DefaultKernelParams
	if config.KernelParams != "" {
		kernelParams = fmt.Sprintf("%s %s", kernelParams, config.KernelParams)
	}

	// Add callback URL
	kernelParams = fmt.Sprintf("%s callback=%s/callback/%s", kernelParams, s.config.CallbackAddress, config.MACAddress)

	// Generate pxelinux configuration
	pxeConfig := fmt.Sprintf(`DEFAULT install
PROMPT 0
TIMEOUT 10

LABEL install
    KERNEL %s
    APPEND initrd=%s %s
`, config.KernelPath, config.InitrdPath, kernelParams)

	// Write to TFTP root
	return s.tftpServer.WriteFile(fmt.Sprintf("pxelinux.cfg/%s", pxeFilename), []byte(pxeConfig))
}

// removePXEConfig removes the PXE configuration file for a client.
func (s *Server) removePXEConfig(macAddress string) error {
	mac, _ := net.ParseMAC(macAddress)
	pxeFilename := fmt.Sprintf("01-%s", mac.String())

	return s.tftpServer.RemoveFile(fmt.Sprintf("pxelinux.cfg/%s", pxeFilename))
}
