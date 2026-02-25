// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package inventory

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// DiscoveryConfig holds configuration for auto-discovery.
type DiscoveryConfig struct {
	// Network ranges to scan
	Networks []string `json:"networks"`

	// IPMI credentials for discovery
	IPMIUsername string `json:"ipmi_username"`
	IPMIPassword string `json:"ipmi_password"`

	// Scan interval
	ScanInterval time.Duration `json:"scan_interval"`

	// Concurrency for scanning
	Concurrency int `json:"concurrency"`

	// Timeout for connection attempts
	Timeout time.Duration `json:"timeout"`
}

// DefaultDiscoveryConfig returns default discovery configuration.
func DefaultDiscoveryConfig() *DiscoveryConfig {
	return &DiscoveryConfig{
		Networks:     []string{"10.0.0.0/24"},
		ScanInterval: 5 * time.Minute,
		Concurrency:  50,
		Timeout:      5 * time.Second,
	}
}

// Discovery handles auto-discovery of bare metal machines.
type Discovery struct {
	config *DiscoveryConfig

	// Callbacks for discovery events
	onDiscovered func(*Hardware)
	onLost       func(string)

	// Known machines
	known   map[string]*Hardware
	knownMu sync.RWMutex

	// Running state
	running bool
	done    chan struct{}
}

// NewDiscovery creates a new discovery service.
func NewDiscovery(config *DiscoveryConfig) *Discovery {
	if config == nil {
		config = DefaultDiscoveryConfig()
	}

	return &Discovery{
		config: config,
		known:  make(map[string]*Hardware),
		done:   make(chan struct{}),
	}
}

// OnDiscovered sets the callback for when a new machine is discovered.
func (d *Discovery) OnDiscovered(cb func(*Hardware)) {
	d.onDiscovered = cb
}

// OnLost sets the callback for when a machine is no longer reachable.
func (d *Discovery) OnLost(cb func(string)) {
	d.onLost = cb
}

// Start starts the discovery service.
func (d *Discovery) Start(ctx context.Context) error {
	if d.running {
		return fmt.Errorf("discovery already running")
	}

	d.running = true
	d.done = make(chan struct{})

	// Run initial discovery
	go d.runDiscoveryLoop(ctx)

	return nil
}

// Stop stops the discovery service.
func (d *Discovery) Stop() error {
	if !d.running {
		return nil
	}

	close(d.done)
	d.running = false

	return nil
}

// runDiscoveryLoop runs the discovery loop.
func (d *Discovery) runDiscoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(d.config.ScanInterval)
	defer ticker.Stop()

	// Initial scan
	d.scan(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.done:
			return
		case <-ticker.C:
			d.scan(ctx)
		}
	}
}

// Discover performs a one-time discovery scan.
func (d *Discovery) Discover(ctx context.Context) ([]*Hardware, error) {
	return d.scan(ctx), nil
}

// scan scans all configured networks.
func (d *Discovery) scan(ctx context.Context) []*Hardware {
	var discovered []*Hardware
	var mu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, d.config.Concurrency)

	for _, network := range d.config.Networks {
		ips, err := d.expandNetwork(network)
		if err != nil {
			continue
		}

		for _, ip := range ips {
			select {
			case <-ctx.Done():
				return discovered
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				defer func() { <-sem }()

				hw := d.probeHost(ctx, ip)
				if hw != nil {
					mu.Lock()
					discovered = append(discovered, hw)
					mu.Unlock()

					d.handleDiscovered(hw)
				}
			}(ip)
		}
	}

	wg.Wait()

	return discovered
}

// expandNetwork expands a CIDR network to a list of IPs.
func (d *Discovery) expandNetwork(network string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); d.incIP(ip) {
		ips = append(ips, ip.String())
	}

	// Remove network and broadcast addresses
	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}

	return ips, nil
}

// incIP increments an IP address.
func (d *Discovery) incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// probeHost probes a host for IPMI/BMC.
func (d *Discovery) probeHost(ctx context.Context, ip string) *Hardware {
	// Try IPMI port (623)
	if d.checkPort(ctx, ip, 623) {
		return d.probeIPMI(ctx, ip)
	}

	// Try Redfish port (443)
	if d.checkPort(ctx, ip, 443) {
		return d.probeRedfish(ctx, ip)
	}

	return nil
}

// checkPort checks if a port is open.
func (d *Discovery) checkPort(ctx context.Context, ip string, port int) bool {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, d.config.Timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// probeIPMI probes an IPMI endpoint.
func (d *Discovery) probeIPMI(ctx context.Context, ip string) *Hardware {
	// In production, use IPMI to query hardware info
	// This is a simplified implementation

	return &Hardware{
		MACAddress: "", // Would be discovered via IPMI
		BMC: &BMCInfo{
			Address: ip,
			Type:    "ipmi",
		},
		DiscoveredAt: time.Now(),
		LastUpdated:  time.Now(),
	}
}

// probeRedfish probes a Redfish endpoint.
func (d *Discovery) probeRedfish(ctx context.Context, ip string) *Hardware {
	// In production, use Redfish to query hardware info
	// This is a simplified implementation

	return &Hardware{
		MACAddress: "", // Would be discovered via Redfish
		BMC: &BMCInfo{
			Address: ip,
			Type:    "redfish",
		},
		DiscoveredAt: time.Now(),
		LastUpdated:  time.Now(),
	}
}

// handleDiscovered handles a newly discovered machine.
func (d *Discovery) handleDiscovered(hw *Hardware) {
	if hw.SerialNumber == "" && hw.MACAddress == "" {
		return
	}

	key := hw.SerialNumber
	if key == "" {
		key = hw.MACAddress
	}

	d.knownMu.Lock()
	defer d.knownMu.Unlock()

	if _, exists := d.known[key]; !exists {
		d.known[key] = hw
		if d.onDiscovered != nil {
			d.onDiscovered(hw)
		}
	} else {
		d.known[key] = hw
	}
}

// GetKnown returns all known machines.
func (d *Discovery) GetKnown() []*Hardware {
	d.knownMu.RLock()
	defer d.knownMu.RUnlock()

	result := make([]*Hardware, 0, len(d.known))
	for _, hw := range d.known {
		result = append(result, hw)
	}

	return result
}

// DHCPLeaseDiscovery discovers machines from DHCP leases.
type DHCPLeaseDiscovery struct {
	leasesFile string
}

// NewDHCPLeaseDiscovery creates a new DHCP lease discovery.
func NewDHCPLeaseDiscovery(leasesFile string) *DHCPLeaseDiscovery {
	return &DHCPLeaseDiscovery{
		leasesFile: leasesFile,
	}
}

// DiscoverFromLeases discovers machines from DHCP lease file.
func (d *DHCPLeaseDiscovery) DiscoverFromLeases(ctx context.Context) ([]*Hardware, error) {
	// In production, parse the DHCP leases file
	// This is a simplified implementation
	return nil, nil
}

// LLDPDiscovery discovers machines using LLDP.
type LLDPDiscovery struct {
	interfaces []string
}

// NewLLDPDiscovery creates a new LLDP discovery.
func NewLLDPDiscovery(interfaces []string) *LLDPDiscovery {
	return &LLDPDiscovery{
		interfaces: interfaces,
	}
}

// DiscoverLLDP discovers machines using LLDP.
func (d *LLDPDiscovery) DiscoverLLDP(ctx context.Context) ([]*Hardware, error) {
	// In production, listen for LLDP packets
	// This is a simplified implementation
	return nil, nil
}
