// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package cluster

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mode != ModeStandalone {
		t.Errorf("default mode should be standalone, got %s", cfg.Mode)
	}
	if cfg.ReplicationPort != 18002 {
		t.Errorf("default replication port should be 18002, got %d", cfg.ReplicationPort)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestClusterConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "standalone valid",
			cfg:     Config{Mode: ModeStandalone},
			wantErr: false,
		},
		{
			name:    "cluster without peer",
			cfg:     Config{Mode: ModeCluster, Role: RolePrimary, NodeID: "west"},
			wantErr: true,
		},
		{
			name: "cluster primary valid",
			cfg: Config{
				Mode: ModeCluster, Role: RolePrimary,
				NodeID: "west", PeerAddr: "east:18002", ReplicationPort: 18002,
			},
			wantErr: false,
		},
		{
			name: "cluster standby valid",
			cfg: Config{
				Mode: ModeCluster, Role: RoleStandby,
				NodeID: "east", PeerAddr: "west:18002", ReplicationPort: 18002,
			},
			wantErr: false,
		},
		{
			name: "cluster invalid role",
			cfg: Config{
				Mode: ModeCluster, Role: "observer",
				NodeID: "x", PeerAddr: "y:18002", ReplicationPort: 18002,
			},
			wantErr: true,
		},
		{
			name:    "invalid mode",
			cfg:     Config{Mode: "distributed"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsCluster(t *testing.T) {
	cfg := Config{Mode: ModeStandalone}
	if cfg.IsCluster() {
		t.Error("standalone should not be cluster")
	}
	cfg.Mode = ModeCluster
	if !cfg.IsCluster() {
		t.Error("cluster mode should be cluster")
	}
}
