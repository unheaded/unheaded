// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package ip provides IP allowlist functionality
package ip

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"
)

// AllowEntry represents an allowed IP entry
type AllowEntry struct {
	IP      string
	Network *net.IPNet
	Comment string
	AddedAt string
}

// Allowlist manages allowed IPs that bypass WAF checks
type Allowlist struct {
	entries  map[string]*AllowEntry
	networks []*AllowEntry
	mu       sync.RWMutex
}

// NewAllowlist creates a new allowlist
func NewAllowlist() *Allowlist {
	return &Allowlist{
		entries:  make(map[string]*AllowEntry),
		networks: make([]*AllowEntry, 0),
	}
}

// Add adds an IP to the allowlist
func (al *Allowlist) Add(ip, comment string) {
	al.mu.Lock()
	defer al.mu.Unlock()

	entry := &AllowEntry{
		IP:      ip,
		Comment: comment,
	}

	// Check if it's a CIDR
	if strings.Contains(ip, "/") {
		_, network, err := net.ParseCIDR(ip)
		if err == nil {
			entry.Network = network
			al.networks = append(al.networks, entry)
			return
		}
	}

	al.entries[ip] = entry
}

// AddNetwork adds a network range to the allowlist
func (al *Allowlist) AddNetwork(cidr, comment string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	entry := &AllowEntry{
		IP:      cidr,
		Network: network,
		Comment: comment,
	}

	al.networks = append(al.networks, entry)
	return nil
}

// Remove removes an IP from the allowlist
func (al *Allowlist) Remove(ip string) bool {
	al.mu.Lock()
	defer al.mu.Unlock()

	if _, ok := al.entries[ip]; ok {
		delete(al.entries, ip)
		return true
	}

	// Check networks
	for i, entry := range al.networks {
		if entry.IP == ip {
			al.networks = append(al.networks[:i], al.networks[i+1:]...)
			return true
		}
	}

	return false
}

// Contains checks if an IP is in the allowlist
func (al *Allowlist) Contains(ip string) bool {
	al.mu.RLock()
	defer al.mu.RUnlock()

	// Check exact match
	if _, ok := al.entries[ip]; ok {
		return true
	}

	// Check network ranges
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, entry := range al.networks {
		if entry.Network != nil && entry.Network.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// GetEntry returns the allow entry for an IP
func (al *Allowlist) GetEntry(ip string) *AllowEntry {
	al.mu.RLock()
	defer al.mu.RUnlock()

	if entry, ok := al.entries[ip]; ok {
		return entry
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil
	}

	for _, entry := range al.networks {
		if entry.Network != nil && entry.Network.Contains(parsedIP) {
			return entry
		}
	}

	return nil
}

// List returns all allowed entries
func (al *Allowlist) List() []*AllowEntry {
	al.mu.RLock()
	defer al.mu.RUnlock()

	entries := make([]*AllowEntry, 0, len(al.entries)+len(al.networks))

	for _, entry := range al.entries {
		entries = append(entries, entry)
	}

	entries = append(entries, al.networks...)

	return entries
}

// Count returns the number of allowed entries
func (al *Allowlist) Count() int {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return len(al.entries) + len(al.networks)
}

// Clear removes all entries
func (al *Allowlist) Clear() {
	al.mu.Lock()
	defer al.mu.Unlock()

	al.entries = make(map[string]*AllowEntry)
	al.networks = make([]*AllowEntry, 0)
}

// LoadFromReader loads IPs from a reader (one per line)
func (al *Allowlist) LoadFromReader(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line: IP [comment]
		parts := strings.SplitN(line, " ", 2)
		ip := parts[0]
		comment := ""
		if len(parts) > 1 {
			comment = parts[1]
		}

		al.Add(ip, comment)
	}

	return scanner.Err()
}

// AddTrustedProxies adds common trusted proxy networks
func (al *Allowlist) AddTrustedProxies() {
	// Cloudflare IPs (example subset)
	cloudflareIPs := []string{
		"173.245.48.0/20",
		"103.21.244.0/22",
		"103.22.200.0/22",
		"103.31.4.0/22",
		"141.101.64.0/18",
		"108.162.192.0/18",
		"190.93.240.0/20",
		"188.114.96.0/20",
		"197.234.240.0/22",
		"198.41.128.0/17",
	}

	for _, cidr := range cloudflareIPs {
		_ = al.AddNetwork(cidr, "Cloudflare")
	}
}

// AddLocalhost adds localhost addresses
func (al *Allowlist) AddLocalhost() {
	al.Add("127.0.0.1", "localhost IPv4")
	al.Add("::1", "localhost IPv6")
	_ = al.AddNetwork("127.0.0.0/8", "localhost range")
}
