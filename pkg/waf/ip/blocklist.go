// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package ip provides IP blocklist functionality
package ip

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// BlockEntry represents a blocked IP entry
type BlockEntry struct {
	IP        string
	Network   *net.IPNet
	Reason    string
	ExpiresAt time.Time
	Permanent bool
	AddedAt   time.Time
}

// IsExpired checks if the entry has expired
func (e *BlockEntry) IsExpired() bool {
	if e.Permanent {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

// Blocklist manages blocked IPs
type Blocklist struct {
	entries  map[string]*BlockEntry
	networks []*BlockEntry
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewBlocklist creates a new blocklist
func NewBlocklist() *Blocklist {
	bl := &Blocklist{
		entries:  make(map[string]*BlockEntry),
		networks: make([]*BlockEntry, 0),
		stopCh:   make(chan struct{}),
	}

	go bl.cleanup()
	return bl
}

// Add adds an IP to the blocklist permanently
func (bl *Blocklist) Add(ip, reason string) {
	bl.AddWithDuration(ip, reason, 0)
}

// AddWithDuration adds an IP with an expiration
func (bl *Blocklist) AddWithDuration(ip, reason string, duration time.Duration) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	entry := &BlockEntry{
		IP:        ip,
		Reason:    reason,
		AddedAt:   time.Now(),
		Permanent: duration == 0,
	}

	if duration > 0 {
		entry.ExpiresAt = time.Now().Add(duration)
	}

	// Check if it's a CIDR
	if strings.Contains(ip, "/") {
		_, network, err := net.ParseCIDR(ip)
		if err == nil {
			entry.Network = network
			bl.networks = append(bl.networks, entry)
			return
		}
	}

	bl.entries[ip] = entry
}

// AddNetwork adds a network range to the blocklist
func (bl *Blocklist) AddNetwork(cidr, reason string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	entry := &BlockEntry{
		IP:        cidr,
		Network:   network,
		Reason:    reason,
		AddedAt:   time.Now(),
		Permanent: true,
	}

	bl.networks = append(bl.networks, entry)
	return nil
}

// Remove removes an IP from the blocklist
func (bl *Blocklist) Remove(ip string) bool {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	if _, ok := bl.entries[ip]; ok {
		delete(bl.entries, ip)
		return true
	}

	// Check networks
	for i, entry := range bl.networks {
		if entry.IP == ip {
			bl.networks = append(bl.networks[:i], bl.networks[i+1:]...)
			return true
		}
	}

	return false
}

// Contains checks if an IP is in the blocklist
func (bl *Blocklist) Contains(ip string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	// Check exact match
	if entry, ok := bl.entries[ip]; ok {
		if !entry.IsExpired() {
			return true
		}
	}

	// Check network ranges
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, entry := range bl.networks {
		if entry.Network != nil && !entry.IsExpired() {
			if entry.Network.Contains(parsedIP) {
				return true
			}
		}
	}

	return false
}

// GetEntry returns the block entry for an IP
func (bl *Blocklist) GetEntry(ip string) *BlockEntry {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	if entry, ok := bl.entries[ip]; ok {
		if !entry.IsExpired() {
			return entry
		}
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil
	}

	for _, entry := range bl.networks {
		if entry.Network != nil && !entry.IsExpired() {
			if entry.Network.Contains(parsedIP) {
				return entry
			}
		}
	}

	return nil
}

// List returns all blocked IPs
func (bl *Blocklist) List() []*BlockEntry {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	entries := make([]*BlockEntry, 0, len(bl.entries)+len(bl.networks))

	for _, entry := range bl.entries {
		if !entry.IsExpired() {
			entries = append(entries, entry)
		}
	}

	for _, entry := range bl.networks {
		if !entry.IsExpired() {
			entries = append(entries, entry)
		}
	}

	return entries
}

// Count returns the number of blocked entries
func (bl *Blocklist) Count() int {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	count := 0
	for _, entry := range bl.entries {
		if !entry.IsExpired() {
			count++
		}
	}
	for _, entry := range bl.networks {
		if !entry.IsExpired() {
			count++
		}
	}
	return count
}

// Clear removes all entries
func (bl *Blocklist) Clear() {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	bl.entries = make(map[string]*BlockEntry)
	bl.networks = make([]*BlockEntry, 0)
}

// LoadFromReader loads IPs from a reader (one per line)
func (bl *Blocklist) LoadFromReader(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line: IP [reason]
		parts := strings.SplitN(line, " ", 2)
		ip := parts[0]
		reason := "loaded from file"
		if len(parts) > 1 {
			reason = parts[1]
		}

		bl.Add(ip, reason)
	}

	return scanner.Err()
}

// Close stops the cleanup goroutine
func (bl *Blocklist) Close() error {
	close(bl.stopCh)
	return nil
}

// cleanup periodically removes expired entries
func (bl *Blocklist) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bl.doCleanup()
		case <-bl.stopCh:
			return
		}
	}
}

// doCleanup removes expired entries
func (bl *Blocklist) doCleanup() {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	// Clean entries
	for ip, entry := range bl.entries {
		if entry.IsExpired() {
			delete(bl.entries, ip)
		}
	}

	// Clean networks
	activeNetworks := make([]*BlockEntry, 0, len(bl.networks))
	for _, entry := range bl.networks {
		if !entry.IsExpired() {
			activeNetworks = append(activeNetworks, entry)
		}
	}
	bl.networks = activeNetworks
}
