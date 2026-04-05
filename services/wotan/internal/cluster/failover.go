// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// FailoverState represents the node's current state in the failover lifecycle.
type FailoverState int

const (
	StateStandby   FailoverState = iota // Normal standby — receiving replication
	StatePromoting                      // Transitioning to primary
	StatePrimary                        // Active primary — accepting writes
	StateDemoting                       // Transitioning back to standby
)

func (s FailoverState) String() string {
	switch s {
	case StateStandby:
		return "standby"
	case StatePromoting:
		return "promoting"
	case StatePrimary:
		return "primary"
	case StateDemoting:
		return "demoting"
	default:
		return "unknown"
	}
}

// FailoverManager handles promotion and demotion of Wotan nodes.
// State machine: STANDBY → PROMOTING → PRIMARY → DEMOTING → STANDBY
type FailoverManager struct {
	mu    sync.RWMutex
	state FailoverState

	nodeID string
	config Config

	// Callbacks — wired by the main Wotan server
	OnPromote func(ctx context.Context) error // Called when promoting to primary
	OnDemote  func(ctx context.Context) error // Called when demoting to standby

	// Failover tracking
	lastFailover   time.Time
	failoverCount  int
	cooldownPeriod time.Duration // Minimum time between failovers

	// Limits
	maxFailovers int // Maximum auto-failovers before requiring manual intervention
}

// NewFailoverManager creates a failover manager.
func NewFailoverManager(cfg Config) *FailoverManager {
	initialState := StateStandby
	if cfg.IsPrimary() {
		initialState = StatePrimary
	}

	return &FailoverManager{
		state:          initialState,
		nodeID:         cfg.NodeID,
		config:         cfg,
		cooldownPeriod: 30 * time.Second,
		maxFailovers:   3, // Matches Akira MaxAutoRestarts
	}
}

// State returns the current failover state.
func (f *FailoverManager) State() FailoverState {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state
}

// PromoteToWriter transitions a standby node to primary.
// Called by Akira when consensus says the primary is down.
func (f *FailoverManager) PromoteToWriter(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state != StateStandby {
		return fmt.Errorf("failover: cannot promote from state %s (must be standby)", f.state)
	}

	// Cooldown check
	if time.Since(f.lastFailover) < f.cooldownPeriod {
		return fmt.Errorf("failover: cooldown period (%s) not elapsed since last failover", f.cooldownPeriod)
	}

	// Max failover check
	if f.failoverCount >= f.maxFailovers {
		return fmt.Errorf("failover: max auto-failovers (%d) exceeded — requires manual intervention", f.maxFailovers)
	}

	log.Info().
		Str("node", f.nodeID).
		Str("from", f.state.String()).
		Msg("failover: PROMOTING to primary")

	f.state = StatePromoting

	// Execute promotion callback
	if f.OnPromote != nil {
		if err := f.OnPromote(ctx); err != nil {
			f.state = StateStandby // Roll back on failure
			return fmt.Errorf("failover: promotion failed: %w", err)
		}
	}

	f.state = StatePrimary
	f.lastFailover = time.Now()
	f.failoverCount++

	log.Info().
		Str("node", f.nodeID).
		Int("failover_count", f.failoverCount).
		Msg("failover: PROMOTED to primary successfully")

	return nil
}

// DemoteToReader transitions a primary node back to standby.
// Called when the original primary recovers.
func (f *FailoverManager) DemoteToReader(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state != StatePrimary {
		return fmt.Errorf("failover: cannot demote from state %s (must be primary)", f.state)
	}

	log.Info().
		Str("node", f.nodeID).
		Msg("failover: DEMOTING to standby")

	f.state = StateDemoting

	if f.OnDemote != nil {
		if err := f.OnDemote(ctx); err != nil {
			f.state = StatePrimary // Stay primary on failure
			return fmt.Errorf("failover: demotion failed: %w", err)
		}
	}

	f.state = StateStandby

	log.Info().
		Str("node", f.nodeID).
		Msg("failover: DEMOTED to standby successfully")

	return nil
}

// IsWritable returns true if this node can accept writes.
func (f *FailoverManager) IsWritable() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state == StatePrimary
}

// ResetFailoverCount resets the counter (called after manual intervention).
func (f *FailoverManager) ResetFailoverCount() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failoverCount = 0
}

// Stats returns failover statistics.
func (f *FailoverManager) Stats() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return map[string]interface{}{
		"state":           f.state.String(),
		"node_id":         f.nodeID,
		"failover_count":  f.failoverCount,
		"max_failovers":   f.maxFailovers,
		"cooldown_period": f.cooldownPeriod.String(),
		"last_failover":   f.lastFailover.Format(time.RFC3339),
	}
}
