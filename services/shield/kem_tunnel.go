// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// TunnelState represents the state of a KEM tunnel.
type TunnelState uint8

const (
	TunnelInit       TunnelState = iota + 1 // Initiator sent encapsulation
	TunnelResponded                          // Responder decapsulated, tunnel ready
	TunnelActive                             // Tunnel in active use
	TunnelExpired                            // Tunnel expired
	TunnelClosed                             // Tunnel closed by either side
)

// String returns a human-readable name.
func (s TunnelState) String() string {
	switch s {
	case TunnelInit:
		return "init"
	case TunnelResponded:
		return "responded"
	case TunnelActive:
		return "active"
	case TunnelExpired:
		return "expired"
	case TunnelClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// KEMTunnel represents a KEM-based encrypted tunnel between services.
type KEMTunnel struct {
	ID             string      `json:"id"`
	LocalService   uint16      `json:"local_service"`
	RemoteService  uint16      `json:"remote_service"`
	AlgoID         uint8       `json:"algo_id"`
	ParamSet       uint8       `json:"param_set"`
	State          TunnelState `json:"state"`
	SharedSecret   [32]byte    `json:"-"` // Never serialized
	Ciphertext     []byte      `json:"ciphertext,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	ExpiresAt      time.Time   `json:"expires_at"`
}

// KEMTunnelManager manages KEM-based encrypted tunnels.
type KEMTunnelManager struct {
	mu      sync.RWMutex
	tunnels map[string]*KEMTunnel // tunnelID → tunnel
	byPair  map[uint32]*KEMTunnel // (localSvc<<16 | remoteSvc) → tunnel
}

// NewKEMTunnelManager creates a new tunnel manager.
func NewKEMTunnelManager() *KEMTunnelManager {
	return &KEMTunnelManager{
		tunnels: make(map[string]*KEMTunnel),
		byPair:  make(map[uint32]*KEMTunnel),
	}
}

// InitiateTunnel starts the KEM handshake as the initiator.
func (m *KEMTunnelManager) InitiateTunnel(localSvc, remoteSvc uint16, algoID, paramSet uint8) (*KEMTunnel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pairKey := uint32(localSvc)<<16 | uint32(remoteSvc)
	if existing, ok := m.byPair[pairKey]; ok && existing.State == TunnelActive {
		return nil, fmt.Errorf("shield: tunnel already active for %d → %d", localSvc, remoteSvc)
	}

	// Generate tunnel ID
	idBytes := make([]byte, 8)
	rand.Read(idBytes)

	now := time.Now()
	tunnel := &KEMTunnel{
		ID:            fmt.Sprintf("tun-%x", idBytes),
		LocalService:  localSvc,
		RemoteService: remoteSvc,
		AlgoID:        algoID,
		ParamSet:      paramSet,
		State:         TunnelInit,
		CreatedAt:     now,
		ExpiresAt:     now.Add(1 * time.Hour),
	}

	m.tunnels[tunnel.ID] = tunnel
	m.byPair[pairKey] = tunnel
	return tunnel, nil
}

// CompleteTunnel marks a tunnel as active after KEM handshake completes.
func (m *KEMTunnelManager) CompleteTunnel(tunnelID string, sharedSecret [32]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tunnel, ok := m.tunnels[tunnelID]
	if !ok {
		return fmt.Errorf("shield: tunnel not found: %s", tunnelID)
	}

	tunnel.SharedSecret = sharedSecret
	tunnel.State = TunnelActive
	return nil
}

// CloseTunnel closes a tunnel.
func (m *KEMTunnelManager) CloseTunnel(tunnelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tunnel, ok := m.tunnels[tunnelID]
	if !ok {
		return fmt.Errorf("shield: tunnel not found: %s", tunnelID)
	}

	tunnel.State = TunnelClosed
	// Zero out shared secret
	for i := range tunnel.SharedSecret {
		tunnel.SharedSecret[i] = 0
	}
	return nil
}

// GetTunnel retrieves a tunnel by ID.
func (m *KEMTunnelManager) GetTunnel(tunnelID string) (*KEMTunnel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tunnels[tunnelID]
	return t, ok
}

// GetTunnelForPair retrieves the active tunnel for a service pair.
func (m *KEMTunnelManager) GetTunnelForPair(localSvc, remoteSvc uint16) (*KEMTunnel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pairKey := uint32(localSvc)<<16 | uint32(remoteSvc)
	t, ok := m.byPair[pairKey]
	return t, ok
}

// ActiveCount returns the number of active tunnels.
func (m *KEMTunnelManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, t := range m.tunnels {
		if t.State == TunnelActive {
			count++
		}
	}
	return count
}

// ExpireStale marks expired tunnels.
func (m *KEMTunnelManager) ExpireStale() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expired := 0
	for _, t := range m.tunnels {
		if t.State == TunnelActive && now.After(t.ExpiresAt) {
			t.State = TunnelExpired
			for i := range t.SharedSecret {
				t.SharedSecret[i] = 0
			}
			expired++
		}
	}
	return expired
}
