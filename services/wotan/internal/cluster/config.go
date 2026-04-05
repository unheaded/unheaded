// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Package cluster provides active-passive clustering for Wotan.
// WEST is always the configured primary. EAST is standby.
// Failover is Akira-driven (66.67% consensus threshold).
package cluster

import (
	"fmt"
	"os"
)

// Mode defines whether Wotan runs standalone or in a cluster.
type Mode string

const (
	// ModeStandalone is the default — no clustering, no replication overhead.
	ModeStandalone Mode = "standalone"

	// ModeCluster enables active-passive replication.
	ModeCluster Mode = "cluster"
)

// Role defines this node's role in the cluster.
type Role string

const (
	// RolePrimary accepts writes and replicates to standby.
	RolePrimary Role = "primary"

	// RoleStandby receives replicated WAL entries and serves reads.
	// Promotes to primary on Akira-driven failover.
	RoleStandby Role = "standby"
)

// Config holds cluster configuration.
type Config struct {
	// Mode: standalone (default) or cluster.
	Mode Mode `json:"mode" yaml:"mode"`

	// Role: primary or standby (only used in cluster mode).
	Role Role `json:"role" yaml:"role"`

	// NodeID identifies this node (default: hostname).
	NodeID string `json:"node_id" yaml:"node_id"`

	// PeerAddr is the address of the other node (host:port).
	PeerAddr string `json:"peer_addr" yaml:"peer_addr"`

	// ReplicationPort is the dedicated gRPC port for WAL replication.
	// Default: 18002
	ReplicationPort int `json:"replication_port" yaml:"replication_port"`

	// PKIDir is the directory containing mTLS certificates.
	PKIDir string `json:"pki_dir" yaml:"pki_dir"`
}

// DefaultConfig returns standalone defaults.
func DefaultConfig() Config {
	hostname, _ := os.Hostname()
	return Config{
		Mode:            ModeStandalone,
		Role:            RolePrimary,
		NodeID:          hostname,
		ReplicationPort: 18002,
		PKIDir:          "/var/lib/unheaded/pki",
	}
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeStandalone:
		return nil // No further validation needed
	case ModeCluster:
		// Cluster mode requires peer address
		if c.PeerAddr == "" {
			return fmt.Errorf("cluster: peer_addr required in cluster mode")
		}
		switch c.Role {
		case RolePrimary, RoleStandby:
			// Valid
		default:
			return fmt.Errorf("cluster: invalid role %q (must be primary or standby)", c.Role)
		}
		if c.NodeID == "" {
			return fmt.Errorf("cluster: node_id required in cluster mode")
		}
		if c.ReplicationPort <= 0 || c.ReplicationPort > 65535 {
			return fmt.Errorf("cluster: invalid replication_port %d", c.ReplicationPort)
		}
	default:
		return fmt.Errorf("cluster: invalid mode %q (must be standalone or cluster)", c.Mode)
	}
	return nil
}

// IsCluster returns true if running in cluster mode.
func (c *Config) IsCluster() bool {
	return c.Mode == ModeCluster
}

// IsPrimary returns true if this node is the primary.
func (c *Config) IsPrimary() bool {
	return c.Role == RolePrimary
}
