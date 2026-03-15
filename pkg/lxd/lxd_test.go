// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package lxd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// HELPERS
// ============================================================================

// connectedMock returns a MockClient that is already connected with an
// optional set of pre-seeded containers created from the supplied configs.
func connectedMock(t *testing.T, cfgs ...ContainerConfig) *MockClient {
	t.Helper()
	m := NewMockClient()
	ctx := context.Background()
	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	for _, cfg := range cfgs {
		if _, err := m.CreateContainer(ctx, cfg); err != nil {
			t.Fatalf("seed CreateContainer(%q): %v", cfg.Name, err)
		}
	}
	return m
}

// baseCfg returns a minimal valid ContainerConfig.
func baseCfg(name string) ContainerConfig {
	return ContainerConfig{
		Name:         name,
		Image:        "nixos:latest",
		Profiles:     []string{"default"},
		Architecture: "x86_64",
	}
}

// ============================================================================
// 1. CLIENT INTERFACE COMPLIANCE
// ============================================================================

func TestInterfaceCompliance(t *testing.T) {
	// Compile-time checks already exist in client.go via var _ Client = ...
	// but we verify at runtime that both constructors return a Client.

	t.Run("MockClient implements Client", func(t *testing.T) {
		var c Client = NewMockClient()
		if c == nil {
			t.Fatal("NewMockClient returned nil")
		}
	})

	t.Run("NewClient with useMock=true", func(t *testing.T) {
		c, err := NewClient(ClientConfig{}, true)
		if err != nil {
			t.Fatalf("NewClient mock: %v", err)
		}
		if c == nil {
			t.Fatal("NewClient returned nil client")
		}
	})

	t.Run("NewClient with useMock=false", func(t *testing.T) {
		c, err := NewClient(ClientConfig{}, false)
		if err != nil {
			t.Fatalf("NewClient real: %v", err)
		}
		if c == nil {
			t.Fatal("NewClient returned nil client")
		}
	})
}

// ============================================================================
// 2. CONNECTION MANAGEMENT
// ============================================================================

func TestConnectionLifecycle(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	if m.IsConnected() {
		t.Fatal("expected not connected initially")
	}

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !m.IsConnected() {
		t.Fatal("expected connected after Connect")
	}

	if err := m.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if m.IsConnected() {
		t.Fatal("expected disconnected after Disconnect")
	}
}

func TestDisconnectedOperationsReturnError(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"ServerInfo", func() error { _, err := m.ServerInfo(ctx); return err }},
		{"CreateContainer", func() error { _, err := m.CreateContainer(ctx, baseCfg("c")); return err }},
		{"DeleteContainer", func() error { _, err := m.DeleteContainer(ctx, "c"); return err }},
		{"StartContainer", func() error { _, err := m.StartContainer(ctx, "c"); return err }},
		{"StopContainer", func() error { _, err := m.StopContainer(ctx, "c", false, 30); return err }},
		{"RestartContainer", func() error { _, err := m.RestartContainer(ctx, "c", false, 30); return err }},
		{"FreezeContainer", func() error { _, err := m.FreezeContainer(ctx, "c"); return err }},
		{"UnfreezeContainer", func() error { _, err := m.UnfreezeContainer(ctx, "c"); return err }},
		{"GetContainer", func() error { _, err := m.GetContainer(ctx, "c"); return err }},
		{"GetContainerState", func() error { _, err := m.GetContainerState(ctx, "c"); return err }},
		{"ListContainers", func() error { _, err := m.ListContainers(ctx); return err }},
		{"ContainerExists", func() error { _, err := m.ContainerExists(ctx, "c"); return err }},
		{"UpdateContainer", func() error { _, err := m.UpdateContainer(ctx, "c", baseCfg("c")); return err }},
		{"RenameContainer", func() error { _, err := m.RenameContainer(ctx, "c", "d"); return err }},
		{"CreateSnapshot", func() error { _, err := m.CreateSnapshot(ctx, "c", "snap1", false); return err }},
		{"DeleteSnapshot", func() error { _, err := m.DeleteSnapshot(ctx, "c", "snap1"); return err }},
		{"RestoreSnapshot", func() error { _, err := m.RestoreSnapshot(ctx, "c", "snap1"); return err }},
		{"ListSnapshots", func() error { _, err := m.ListSnapshots(ctx, "c"); return err }},
		{"ExecContainer", func() error { _, err := m.ExecContainer(ctx, "c", []string{"ls"}, nil); return err }},
		{"PushFile", func() error { return m.PushFile(ctx, "c", "/tmp/f", []byte("x"), 0644) }},
		{"PullFile", func() error { _, err := m.PullFile(ctx, "c", "/tmp/f"); return err }},
		{"CancelOperation", func() error {
			return m.CancelOperation(ctx, &Operation{ID: "op-1"})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if !errors.Is(err, ErrClientNotConnected) {
				t.Errorf("expected ErrClientNotConnected, got %v", err)
			}
		})
	}
}

// ============================================================================
// 3. SERVER INFO
// ============================================================================

func TestServerInfo(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	info, err := m.ServerInfo(ctx)
	if err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}
	if info.APIVersion != "1.0" {
		t.Errorf("expected api_version 1.0, got %s", info.APIVersion)
	}
	if info.Auth != "trusted" {
		t.Errorf("expected auth trusted, got %s", info.Auth)
	}
	if info.Public {
		t.Error("expected Public=false")
	}
	if len(info.AuthMethods) == 0 {
		t.Error("expected non-empty AuthMethods")
	}
	if v, ok := info.Environment["server_version"]; !ok || v != "5.21" {
		t.Errorf("expected server_version 5.21, got %s", v)
	}
}

// ============================================================================
// 4. CONTAINER LIFECYCLE (CREATE / START / STOP / DELETE) - TABLE DRIVEN
// ============================================================================

func TestCreateContainer(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ContainerConfig
		wantErr error
	}{
		{
			name:    "valid container",
			cfg:     baseCfg("test-1"),
			wantErr: nil,
		},
		{
			name:    "empty name",
			cfg:     ContainerConfig{Name: "", Image: "nixos:latest"},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "with devices and config",
			cfg: ContainerConfig{
				Name:  "test-dev",
				Image: "nixos:latest",
				Devices: map[string]Device{
					"eth0": {Type: "nic", Properties: map[string]string{"network": "lxdbr0"}},
				},
				Config: map[string]string{
					"limits.cpu":    "2",
					"limits.memory": "1GB",
				},
				Profiles:     []string{"default", "custom"},
				Architecture: "arm64",
			},
			wantErr: nil,
		},
		{
			name:    "ephemeral container",
			cfg:     ContainerConfig{Name: "eph-1", Image: "nixos:latest", Ephemeral: true},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t)
			ctx := context.Background()

			op, err := m.CreateContainer(ctx, tt.cfg)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateContainer: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}

			if op == nil {
				t.Fatal("expected non-nil operation")
			}
			if op.Status != "Success" {
				t.Errorf("expected op status Success, got %s", op.Status)
			}
			if op.StatusCode != 200 {
				t.Errorf("expected op status code 200, got %d", op.StatusCode)
			}
			if op.ID == "" {
				t.Error("expected non-empty operation ID")
			}
		})
	}
}

func TestCreateContainerDuplicate(t *testing.T) {
	m := connectedMock(t, baseCfg("dup"))
	ctx := context.Background()

	_, err := m.CreateContainer(ctx, baseCfg("dup"))
	if !errors.Is(err, ErrContainerExists) {
		t.Fatalf("expected ErrContainerExists, got %v", err)
	}
}

func TestContainerCreatedFieldsMatch(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()
	cfg := ContainerConfig{
		Name:         "verify-fields",
		Image:        "nixos:latest",
		Profiles:     []string{"default", "hardened"},
		Architecture: "x86_64",
		Ephemeral:    true,
		Config:       map[string]string{"limits.cpu": "4"},
		Devices: map[string]Device{
			"root": {Type: "disk", Properties: map[string]string{"path": "/", "pool": "default"}},
		},
	}

	_, err := m.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}

	info, err := m.GetContainer(ctx, "verify-fields")
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}

	if info.Name != cfg.Name {
		t.Errorf("name: want %s, got %s", cfg.Name, info.Name)
	}
	if info.Architecture != cfg.Architecture {
		t.Errorf("architecture: want %s, got %s", cfg.Architecture, info.Architecture)
	}
	if info.Ephemeral != cfg.Ephemeral {
		t.Errorf("ephemeral: want %v, got %v", cfg.Ephemeral, info.Ephemeral)
	}
	if info.Status != "Stopped" {
		t.Errorf("initial status: want Stopped, got %s", info.Status)
	}
	if info.StatusCode != 102 {
		t.Errorf("initial status code: want 102, got %d", info.StatusCode)
	}
	if len(info.Profiles) != 2 {
		t.Errorf("profiles length: want 2, got %d", len(info.Profiles))
	}
	if info.Config["limits.cpu"] != "4" {
		t.Errorf("config limits.cpu: want 4, got %s", info.Config["limits.cpu"])
	}
	if info.Devices["root"].Type != "disk" {
		t.Errorf("device root type: want disk, got %s", info.Devices["root"].Type)
	}
	if info.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestStartContainer(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr error
	}{
		{"existing container", "c1", nil},
		{"non-existent container", "ghost", ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()

			op, err := m.StartContainer(ctx, tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("StartContainer: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}
			if op == nil || op.Status != "Success" {
				t.Fatal("expected successful operation")
			}

			info, _ := m.GetContainer(ctx, tt.target)
			if info.Status != "Running" {
				t.Errorf("expected Running, got %s", info.Status)
			}
			if info.StatusCode != 103 {
				t.Errorf("expected status code 103, got %d", info.StatusCode)
			}

			state, _ := m.GetContainerState(ctx, tt.target)
			if state.Status != "Running" {
				t.Errorf("state: expected Running, got %s", state.Status)
			}
			if state.Pid == 0 {
				t.Error("expected non-zero PID after start")
			}
			if state.Processes == 0 {
				t.Error("expected non-zero Processes after start")
			}
		})
	}
}

func TestStopContainer(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		force   bool
		timeout int
		wantErr error
	}{
		{"graceful stop", "c1", false, 30, nil},
		{"force stop", "c1", true, 0, nil},
		{"non-existent container", "ghost", false, 30, ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()
			m.StartContainer(ctx, "c1")

			op, err := m.StopContainer(ctx, tt.target, tt.force, tt.timeout)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("StopContainer: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}
			if op == nil || op.Status != "Success" {
				t.Fatal("expected successful operation")
			}

			info, _ := m.GetContainer(ctx, tt.target)
			if info.Status != "Stopped" {
				t.Errorf("expected Stopped, got %s", info.Status)
			}

			state, _ := m.GetContainerState(ctx, tt.target)
			if state.Pid != 0 {
				t.Errorf("expected pid=0 after stop, got %d", state.Pid)
			}
			if state.Processes != 0 {
				t.Errorf("expected processes=0 after stop, got %d", state.Processes)
			}
		})
	}
}

func TestDeleteContainer(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr error
	}{
		{"existing container", "c1", nil},
		{"non-existent container", "ghost", ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()

			op, err := m.DeleteContainer(ctx, tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DeleteContainer: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}
			if op == nil || op.Status != "Success" {
				t.Fatal("expected successful operation")
			}

			exists, _ := m.ContainerExists(ctx, "c1")
			if exists {
				t.Error("container should not exist after delete")
			}
		})
	}
}

func TestDeleteContainerCleansUpStateAndSnapshots(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	m.CreateSnapshot(ctx, "c1", "snap1", false)
	m.StartContainer(ctx, "c1")

	_, err := m.DeleteContainer(ctx, "c1")
	if err != nil {
		t.Fatalf("DeleteContainer: %v", err)
	}

	_, err = m.GetContainerState(ctx, "c1")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Error("expected container state to be cleaned up")
	}

	_, err = m.ListSnapshots(ctx, "c1")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Error("expected snapshots to be cleaned up")
	}
}

// ============================================================================
// 5. FULL LIFECYCLE TEST
// ============================================================================

func TestFullContainerLifecycle(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()
	name := "lifecycle-test"

	// Create
	op, err := m.CreateContainer(ctx, baseCfg(name))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if op.Type != "container_create" {
		t.Errorf("op type: want container_create, got %s", op.Type)
	}

	// Verify exists
	exists, _ := m.ContainerExists(ctx, name)
	if !exists {
		t.Fatal("container should exist after create")
	}

	// Start
	op, err = m.StartContainer(ctx, name)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if op.Type != "container_start" {
		t.Errorf("op type: want container_start, got %s", op.Type)
	}

	info, _ := m.GetContainer(ctx, name)
	if info.Status != "Running" {
		t.Fatalf("expected Running, got %s", info.Status)
	}

	// Freeze
	op, err = m.FreezeContainer(ctx, name)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if op.Type != "container_freeze" {
		t.Errorf("op type: want container_freeze, got %s", op.Type)
	}

	info, _ = m.GetContainer(ctx, name)
	if info.Status != "Frozen" {
		t.Fatalf("expected Frozen, got %s", info.Status)
	}
	if info.StatusCode != 110 {
		t.Errorf("frozen status code: want 110, got %d", info.StatusCode)
	}

	// Unfreeze
	op, err = m.UnfreezeContainer(ctx, name)
	if err != nil {
		t.Fatalf("Unfreeze: %v", err)
	}
	if op.Type != "container_unfreeze" {
		t.Errorf("op type: want container_unfreeze, got %s", op.Type)
	}

	info, _ = m.GetContainer(ctx, name)
	if info.Status != "Running" {
		t.Fatalf("expected Running after unfreeze, got %s", info.Status)
	}

	// Restart
	op, err = m.RestartContainer(ctx, name, false, 30)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	info, _ = m.GetContainer(ctx, name)
	if info.Status != "Running" {
		t.Fatalf("expected Running after restart, got %s", info.Status)
	}

	// Stop
	op, err = m.StopContainer(ctx, name, false, 30)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if op.Type != "container_stop" {
		t.Errorf("op type: want container_stop, got %s", op.Type)
	}

	info, _ = m.GetContainer(ctx, name)
	if info.Status != "Stopped" {
		t.Fatalf("expected Stopped, got %s", info.Status)
	}

	// Delete
	op, err = m.DeleteContainer(ctx, name)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if op.Type != "container_delete" {
		t.Errorf("op type: want container_delete, got %s", op.Type)
	}

	exists, _ = m.ContainerExists(ctx, name)
	if exists {
		t.Fatal("container should not exist after delete")
	}
}

// ============================================================================
// 6. CONTAINER INFORMATION & STATE
// ============================================================================

func TestGetContainer_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.GetContainer(ctx, "nonexistent")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestGetContainerState_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.GetContainerState(ctx, "nonexistent")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestListContainers(t *testing.T) {
	tests := []struct {
		name  string
		seed  []ContainerConfig
		count int
	}{
		{"empty", nil, 0},
		{"single", []ContainerConfig{baseCfg("c1")}, 1},
		{"multiple", []ContainerConfig{baseCfg("c1"), baseCfg("c2"), baseCfg("c3")}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, tt.seed...)
			ctx := context.Background()

			list, err := m.ListContainers(ctx)
			if err != nil {
				t.Fatalf("ListContainers: %v", err)
			}
			if len(list) != tt.count {
				t.Errorf("expected %d containers, got %d", tt.count, len(list))
			}
		})
	}
}

func TestContainerExists(t *testing.T) {
	m := connectedMock(t, baseCfg("present"))
	ctx := context.Background()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"existing", "present", true},
		{"missing", "absent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.ContainerExists(ctx, tt.target)
			if err != nil {
				t.Fatalf("ContainerExists: %v", err)
			}
			if got != tt.want {
				t.Errorf("want %v, got %v", tt.want, got)
			}
		})
	}
}

// ============================================================================
// 7. UPDATE & RENAME
// ============================================================================

func TestUpdateContainer(t *testing.T) {
	m := connectedMock(t, baseCfg("upd"))
	ctx := context.Background()

	newCfg := ContainerConfig{
		Config:   map[string]string{"limits.cpu": "8", "limits.memory": "4GB"},
		Profiles: []string{"default", "gpu"},
		Devices: map[string]Device{
			"gpu0": {Type: "gpu", Properties: map[string]string{"id": "0"}},
		},
	}

	op, err := m.UpdateContainer(ctx, "upd", newCfg)
	if err != nil {
		t.Fatalf("UpdateContainer: %v", err)
	}
	if op.Type != "container_update" {
		t.Errorf("op type: want container_update, got %s", op.Type)
	}

	info, _ := m.GetContainer(ctx, "upd")
	if info.Config["limits.cpu"] != "8" {
		t.Errorf("config not updated: %v", info.Config)
	}
	if len(info.Profiles) != 2 || info.Profiles[1] != "gpu" {
		t.Errorf("profiles not updated: %v", info.Profiles)
	}
	if info.Devices["gpu0"].Type != "gpu" {
		t.Error("devices not updated")
	}
}

func TestUpdateContainer_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.UpdateContainer(ctx, "ghost", baseCfg("ghost"))
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestRenameContainer(t *testing.T) {
	m := connectedMock(t, baseCfg("old"))
	ctx := context.Background()

	op, err := m.RenameContainer(ctx, "old", "new")
	if err != nil {
		t.Fatalf("RenameContainer: %v", err)
	}
	if op.Type != "container_rename" {
		t.Errorf("op type: want container_rename, got %s", op.Type)
	}

	// Old name should be gone
	exists, _ := m.ContainerExists(ctx, "old")
	if exists {
		t.Error("old name should not exist")
	}

	// New name should exist
	info, err := m.GetContainer(ctx, "new")
	if err != nil {
		t.Fatalf("GetContainer(new): %v", err)
	}
	if info.Name != "new" {
		t.Errorf("name: want new, got %s", info.Name)
	}

	// State should transfer
	state, err := m.GetContainerState(ctx, "new")
	if err != nil {
		t.Fatalf("GetContainerState(new): %v", err)
	}
	if state == nil {
		t.Fatal("state should transfer to new name")
	}

	// Old state should be gone
	_, err = m.GetContainerState(ctx, "old")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Error("old state should not exist")
	}
}

func TestRenameContainer_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.RenameContainer(ctx, "ghost", "new")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestRenameContainer_TargetExists(t *testing.T) {
	m := connectedMock(t, baseCfg("a"), baseCfg("b"))
	ctx := context.Background()

	_, err := m.RenameContainer(ctx, "a", "b")
	if !errors.Is(err, ErrContainerExists) {
		t.Fatalf("expected ErrContainerExists, got %v", err)
	}
}

// ============================================================================
// 8. FREEZE / UNFREEZE EDGE CASES
// ============================================================================

func TestFreezeContainer_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.FreezeContainer(ctx, "ghost")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestUnfreezeContainer_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.UnfreezeContainer(ctx, "ghost")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestFreezeUnfreezeStateTransitions(t *testing.T) {
	m := connectedMock(t, baseCfg("c"))
	ctx := context.Background()
	m.StartContainer(ctx, "c")

	// Freeze
	_, err := m.FreezeContainer(ctx, "c")
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	state, _ := m.GetContainerState(ctx, "c")
	if state.Status != "Frozen" {
		t.Errorf("expected Frozen, got %s", state.Status)
	}
	if state.StatusCode != 110 {
		t.Errorf("expected status code 110, got %d", state.StatusCode)
	}

	// Unfreeze
	_, err = m.UnfreezeContainer(ctx, "c")
	if err != nil {
		t.Fatalf("Unfreeze: %v", err)
	}
	state, _ = m.GetContainerState(ctx, "c")
	if state.Status != "Running" {
		t.Errorf("expected Running after unfreeze, got %s", state.Status)
	}
	if state.StatusCode != 103 {
		t.Errorf("expected status code 103, got %d", state.StatusCode)
	}
}

// ============================================================================
// 9. RESTART EDGE CASES
// ============================================================================

func TestRestartContainer_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.RestartContainer(ctx, "ghost", false, 30)
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestRestartContainer_StatusTransitions(t *testing.T) {
	m := connectedMock(t, baseCfg("c"))
	ctx := context.Background()
	m.StartContainer(ctx, "c")

	_, err := m.RestartContainer(ctx, "c", true, 10)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}

	info, _ := m.GetContainer(ctx, "c")
	if info.Status != "Running" {
		t.Errorf("expected Running after restart, got %s", info.Status)
	}
}

// ============================================================================
// 10. SNAPSHOT MANAGEMENT - TABLE DRIVEN
// ============================================================================

func TestCreateSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		container string
		snapshot  string
		stateful  bool
		wantErr   error
	}{
		{"valid snapshot", "c1", "snap1", false, nil},
		{"stateful snapshot", "c1", "snap-stateful", true, nil},
		{"non-existent container", "ghost", "snap1", false, ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()

			op, err := m.CreateSnapshot(ctx, tt.container, tt.snapshot, tt.stateful)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateSnapshot: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}

			if op.Type != "snapshot_create" {
				t.Errorf("op type: want snapshot_create, got %s", op.Type)
			}

			snaps, _ := m.ListSnapshots(ctx, tt.container)
			if len(snaps) != 1 {
				t.Fatalf("expected 1 snapshot, got %d", len(snaps))
			}
			if snaps[0].Name != tt.snapshot {
				t.Errorf("snapshot name: want %s, got %s", tt.snapshot, snaps[0].Name)
			}
			if snaps[0].Stateful != tt.stateful {
				t.Errorf("stateful: want %v, got %v", tt.stateful, snaps[0].Stateful)
			}
			if snaps[0].CreatedAt.IsZero() {
				t.Error("expected non-zero CreatedAt on snapshot")
			}
		})
	}
}

func TestCreateMultipleSnapshots(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	names := []string{"snap-a", "snap-b", "snap-c"}
	for _, n := range names {
		if _, err := m.CreateSnapshot(ctx, "c1", n, false); err != nil {
			t.Fatalf("CreateSnapshot(%s): %v", n, err)
		}
	}

	snaps, err := m.ListSnapshots(ctx, "c1")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(snaps))
	}
}

func TestDeleteSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		container string
		snapshot  string
		wantErr   error
	}{
		{"existing snapshot", "c1", "snap1", nil},
		{"non-existent container", "ghost", "snap1", ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()
			if tt.wantErr == nil {
				m.CreateSnapshot(ctx, "c1", "snap1", false)
			}

			op, err := m.DeleteSnapshot(ctx, tt.container, tt.snapshot)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DeleteSnapshot: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}
			if op.Type != "snapshot_delete" {
				t.Errorf("op type: want snapshot_delete, got %s", op.Type)
			}

			snaps, _ := m.ListSnapshots(ctx, "c1")
			if len(snaps) != 0 {
				t.Errorf("expected 0 snapshots after delete, got %d", len(snaps))
			}
		})
	}
}

func TestDeleteSnapshot_NotFound(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()
	// Create one snapshot so the container's snapshot list exists but is searched
	m.CreateSnapshot(ctx, "c1", "other", false)

	_, err := m.DeleteSnapshot(ctx, "c1", "nonexistent")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestDeleteSnapshotLeavesOthers(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()
	m.CreateSnapshot(ctx, "c1", "keep", false)
	m.CreateSnapshot(ctx, "c1", "remove", false)

	_, err := m.DeleteSnapshot(ctx, "c1", "remove")
	if err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	snaps, _ := m.ListSnapshots(ctx, "c1")
	if len(snaps) != 1 {
		t.Fatalf("expected 1 remaining snapshot, got %d", len(snaps))
	}
	if snaps[0].Name != "keep" {
		t.Errorf("wrong snapshot remaining: %s", snaps[0].Name)
	}
}

func TestRestoreSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		container string
		snapshot  string
		wantErr   error
	}{
		{"valid restore", "c1", "snap1", nil},
		{"non-existent container", "ghost", "snap1", ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()
			if tt.wantErr == nil {
				m.CreateSnapshot(ctx, "c1", "snap1", false)
			}

			op, err := m.RestoreSnapshot(ctx, tt.container, tt.snapshot)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RestoreSnapshot: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}
			if op.Type != "snapshot_restore" {
				t.Errorf("op type: want snapshot_restore, got %s", op.Type)
			}
		})
	}
}

func TestRestoreSnapshot_SnapshotNotFound(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()
	// Create a snapshot so the container has a snapshot list, then try to restore a different one
	m.CreateSnapshot(ctx, "c1", "other", false)

	_, err := m.RestoreSnapshot(ctx, "c1", "missing-snap")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestListSnapshots_Empty(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	snaps, err := m.ListSnapshots(ctx, "c1")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if snaps == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snaps))
	}
}

func TestListSnapshots_NotFound(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	_, err := m.ListSnapshots(ctx, "ghost")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

// ============================================================================
// 11. EXEC / FILE OPS
// ============================================================================

func TestExecContainer(t *testing.T) {
	tests := []struct {
		name       string
		container  string
		status     string // container status to set
		wantExit   int
		wantErr    error
		wantStdout string
	}{
		{
			name:       "running container",
			container:  "c1",
			status:     "start",
			wantExit:   0,
			wantStdout: "command executed successfully",
		},
		{
			name:      "stopped container",
			container: "c1",
			status:    "stopped",
			wantExit:  1,
		},
		{
			name:      "non-existent container",
			container: "ghost",
			wantErr:   ErrContainerNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()

			if tt.status == "start" {
				m.StartContainer(ctx, "c1")
			}

			result, err := m.ExecContainer(ctx, tt.container, []string{"echo", "hi"}, map[string]string{"FOO": "bar"})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ExecContainer: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil {
				return
			}

			if result.ExitCode != tt.wantExit {
				t.Errorf("exit code: want %d, got %d", tt.wantExit, result.ExitCode)
			}
			if tt.wantStdout != "" && string(result.Stdout) != tt.wantStdout {
				t.Errorf("stdout: want %q, got %q", tt.wantStdout, string(result.Stdout))
			}
		})
	}
}

func TestExecContainer_StoppedReturnsStderr(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()
	// Container is created in Stopped state

	result, err := m.ExecContainer(ctx, "c1", []string{"ls"}, nil)
	if err != nil {
		t.Fatalf("ExecContainer: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code: want 1, got %d", result.ExitCode)
	}
	if len(result.Stderr) == 0 {
		t.Error("expected non-empty stderr for stopped container")
	}
}

func TestPushFile(t *testing.T) {
	tests := []struct {
		name      string
		container string
		wantErr   error
	}{
		{"existing container", "c1", nil},
		{"non-existent container", "ghost", ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()

			err := m.PushFile(ctx, tt.container, "/tmp/test.txt", []byte("hello"), 0644)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("PushFile: want err=%v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestPullFile(t *testing.T) {
	tests := []struct {
		name      string
		container string
		wantErr   error
	}{
		{"existing container", "c1", nil},
		{"non-existent container", "ghost", ErrContainerNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := connectedMock(t, baseCfg("c1"))
			ctx := context.Background()

			data, err := m.PullFile(ctx, tt.container, "/tmp/test.txt")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("PullFile: want err=%v, got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil {
				if string(data) != "mock file content" {
					t.Errorf("data: want %q, got %q", "mock file content", string(data))
				}
			}
		})
	}
}

// ============================================================================
// 12. OPERATIONS
// ============================================================================

func TestWaitOperation(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	op, _ := m.StartContainer(ctx, "c1")
	err := m.WaitOperation(ctx, op)
	if err != nil {
		t.Fatalf("WaitOperation: %v", err)
	}
}

func TestCancelOperation(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	op, _ := m.StartContainer(ctx, "c1")
	err := m.CancelOperation(ctx, op)
	if err != nil {
		t.Fatalf("CancelOperation: %v", err)
	}
}

func TestCancelOperation_UnknownOp(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	// Cancelling an unknown operation should not error - the mock just ignores it
	err := m.CancelOperation(ctx, &Operation{ID: "unknown-op"})
	if err != nil {
		t.Fatalf("CancelOperation unknown: %v", err)
	}
}

func TestOperationIDsAreUnique(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	ids := make(map[string]bool)
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("c-%d", i)
		op, err := m.CreateContainer(ctx, baseCfg(name))
		if err != nil {
			t.Fatalf("CreateContainer(%s): %v", name, err)
		}
		if ids[op.ID] {
			t.Fatalf("duplicate operation ID: %s", op.ID)
		}
		ids[op.ID] = true
	}
}

// ============================================================================
// 13. REAL CLIENT TESTS
// ============================================================================

func TestRealClientDefaults(t *testing.T) {
	c, err := NewRealClient(ClientConfig{})
	if err != nil {
		t.Fatalf("NewRealClient: %v", err)
	}

	rc, ok := c.(*RealClient)
	if !ok {
		t.Fatal("expected *RealClient")
	}

	if rc.config.Timeout != 30*time.Second {
		t.Errorf("default timeout: want 30s, got %v", rc.config.Timeout)
	}
	if rc.config.Socket != "/var/lib/lxd/unix.socket" {
		t.Errorf("default socket: want /var/lib/lxd/unix.socket, got %s", rc.config.Socket)
	}
}

func TestRealClientCustomConfig(t *testing.T) {
	cfg := ClientConfig{
		HTTPS:      "https://lxd.example.com:8443",
		TLSCert:    "/path/to/cert.pem",
		TLSKey:     "/path/to/key.pem",
		TLSCA:      "/path/to/ca.pem",
		Timeout:    60 * time.Second,
		SkipVerify: true,
	}

	c, err := NewRealClient(cfg)
	if err != nil {
		t.Fatalf("NewRealClient: %v", err)
	}

	rc := c.(*RealClient)
	if rc.config.HTTPS != cfg.HTTPS {
		t.Errorf("HTTPS: want %s, got %s", cfg.HTTPS, rc.config.HTTPS)
	}
	if rc.config.Timeout != 60*time.Second {
		t.Errorf("timeout: want 60s, got %v", rc.config.Timeout)
	}
	// Socket should remain empty because HTTPS was provided
	if rc.config.Socket != "" {
		t.Errorf("socket should be empty when HTTPS is set, got %s", rc.config.Socket)
	}
}

func TestRealClientSocketDefault_NotSetWhenHTTPS(t *testing.T) {
	cfg := ClientConfig{
		HTTPS: "https://remote:8443",
	}
	c, _ := NewRealClient(cfg)
	rc := c.(*RealClient)
	if rc.config.Socket != "" {
		t.Errorf("socket should be empty when HTTPS endpoint is specified, got %q", rc.config.Socket)
	}
}

func TestRealClientConnectionLifecycle(t *testing.T) {
	c, _ := NewRealClient(ClientConfig{})
	ctx := context.Background()

	if c.IsConnected() {
		t.Fatal("should not be connected initially")
	}

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !c.IsConnected() {
		t.Fatal("should be connected after Connect")
	}

	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if c.IsConnected() {
		t.Fatal("should not be connected after Disconnect")
	}
}

func TestRealClientDisconnectedErrors(t *testing.T) {
	c, _ := NewRealClient(ClientConfig{})
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"ServerInfo", func() error { _, err := c.ServerInfo(ctx); return err }},
		{"CreateContainer", func() error { _, err := c.CreateContainer(ctx, baseCfg("c")); return err }},
		{"DeleteContainer", func() error { _, err := c.DeleteContainer(ctx, "c"); return err }},
		{"StartContainer", func() error { _, err := c.StartContainer(ctx, "c"); return err }},
		{"StopContainer", func() error { _, err := c.StopContainer(ctx, "c", false, 30); return err }},
		{"RestartContainer", func() error { _, err := c.RestartContainer(ctx, "c", false, 30); return err }},
		{"FreezeContainer", func() error { _, err := c.FreezeContainer(ctx, "c"); return err }},
		{"UnfreezeContainer", func() error { _, err := c.UnfreezeContainer(ctx, "c"); return err }},
		{"GetContainer", func() error { _, err := c.GetContainer(ctx, "c"); return err }},
		{"GetContainerState", func() error { _, err := c.GetContainerState(ctx, "c"); return err }},
		{"ListContainers", func() error { _, err := c.ListContainers(ctx); return err }},
		{"ContainerExists", func() error { _, err := c.ContainerExists(ctx, "c"); return err }},
		{"UpdateContainer", func() error { _, err := c.UpdateContainer(ctx, "c", baseCfg("c")); return err }},
		{"RenameContainer", func() error { _, err := c.RenameContainer(ctx, "c", "d"); return err }},
		{"CreateSnapshot", func() error { _, err := c.CreateSnapshot(ctx, "c", "s", false); return err }},
		{"DeleteSnapshot", func() error { _, err := c.DeleteSnapshot(ctx, "c", "s"); return err }},
		{"RestoreSnapshot", func() error { _, err := c.RestoreSnapshot(ctx, "c", "s"); return err }},
		{"ListSnapshots", func() error { _, err := c.ListSnapshots(ctx, "c"); return err }},
		{"ExecContainer", func() error { _, err := c.ExecContainer(ctx, "c", []string{"ls"}, nil); return err }},
		{"PushFile", func() error { return c.PushFile(ctx, "c", "/f", []byte("x"), 0644) }},
		{"PullFile", func() error { _, err := c.PullFile(ctx, "c", "/f"); return err }},
		{"WaitOperation", func() error { return c.WaitOperation(ctx, &Operation{ID: "x"}) }},
		{"CancelOperation", func() error { return c.CancelOperation(ctx, &Operation{ID: "x"}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if !errors.Is(err, ErrClientNotConnected) {
				t.Errorf("expected ErrClientNotConnected, got %v", err)
			}
		})
	}
}

func TestRealClientServerInfo(t *testing.T) {
	c, _ := NewRealClient(ClientConfig{})
	ctx := context.Background()
	c.Connect(ctx)

	info, err := c.ServerInfo(ctx)
	if err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}
	if info.APIVersion != "1.0" {
		t.Errorf("api version: want 1.0, got %s", info.APIVersion)
	}
}

func TestRealClientConnectedStubMethods(t *testing.T) {
	c, _ := NewRealClient(ClientConfig{})
	ctx := context.Background()
	c.Connect(ctx)

	// Test that connected stub methods don't panic and return expected values
	t.Run("CreateContainer", func(t *testing.T) {
		op, err := c.CreateContainer(ctx, baseCfg("c"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if op == nil || op.Status != "Running" {
			t.Error("expected non-nil op with Running status")
		}
	})

	t.Run("ListContainers", func(t *testing.T) {
		list, err := c.ListContainers(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if list == nil {
			t.Error("expected non-nil list")
		}
	})

	t.Run("ContainerExists", func(t *testing.T) {
		exists, err := c.ContainerExists(ctx, "c")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Error("stub should return false")
		}
	})

	t.Run("GetContainer returns NotFound", func(t *testing.T) {
		_, err := c.GetContainer(ctx, "c")
		if !errors.Is(err, ErrContainerNotFound) {
			t.Errorf("expected ErrContainerNotFound, got %v", err)
		}
	})

	t.Run("GetContainerState returns NotFound", func(t *testing.T) {
		_, err := c.GetContainerState(ctx, "c")
		if !errors.Is(err, ErrContainerNotFound) {
			t.Errorf("expected ErrContainerNotFound, got %v", err)
		}
	})

	t.Run("ExecContainer", func(t *testing.T) {
		result, err := c.ExecContainer(ctx, "c", []string{"echo"}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.ExitCode)
		}
	})

	t.Run("PushFile", func(t *testing.T) {
		err := c.PushFile(ctx, "c", "/f", []byte("data"), 0644)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("PullFile", func(t *testing.T) {
		data, err := c.PullFile(ctx, "c", "/f")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if data != nil {
			t.Errorf("stub should return nil data, got %v", data)
		}
	})

	t.Run("WaitOperation", func(t *testing.T) {
		err := c.WaitOperation(ctx, &Operation{ID: "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("CancelOperation", func(t *testing.T) {
		err := c.CancelOperation(ctx, &Operation{ID: "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ListSnapshots", func(t *testing.T) {
		snaps, err := c.ListSnapshots(ctx, "c")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(snaps) != 0 {
			t.Errorf("stub should return empty slice, got %d", len(snaps))
		}
	})
}

// ============================================================================
// 14. CLIENT FACTORY
// ============================================================================

func TestNewClient_Factory(t *testing.T) {
	t.Run("mock mode", func(t *testing.T) {
		c, err := NewClient(ClientConfig{}, true)
		if err != nil {
			t.Fatalf("NewClient mock: %v", err)
		}
		if _, ok := c.(*MockClient); !ok {
			t.Errorf("expected *MockClient, got %T", c)
		}
	})

	t.Run("real mode", func(t *testing.T) {
		c, err := NewClient(ClientConfig{}, false)
		if err != nil {
			t.Fatalf("NewClient real: %v", err)
		}
		if _, ok := c.(*RealClient); !ok {
			t.Errorf("expected *RealClient, got %T", c)
		}
	})
}

// ============================================================================
// 15. CONCURRENT ACCESS (RACE-SAFE TESTS)
// ============================================================================

func TestConcurrentCreateAndList(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	const n = 100
	var wg sync.WaitGroup

	// Concurrently create n containers
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("concurrent-%d", idx)
			_, err := m.CreateContainer(ctx, baseCfg(name))
			if err != nil {
				t.Errorf("CreateContainer(%s): %v", name, err)
			}
		}(i)
	}
	wg.Wait()

	list, err := m.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(list) != n {
		t.Errorf("expected %d containers, got %d", n, len(list))
	}
}

func TestConcurrentConnectDisconnect(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	const n = 100
	var wg sync.WaitGroup

	// Half connect, half disconnect
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = m.Connect(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = m.Disconnect()
		}()
	}
	wg.Wait()
	// No races or panics == pass
}

func TestConcurrentReadOperations(t *testing.T) {
	m := connectedMock(t, baseCfg("shared"))
	ctx := context.Background()
	m.StartContainer(ctx, "shared")

	const readers = 50
	var wg sync.WaitGroup
	wg.Add(readers * 5)

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			m.GetContainer(ctx, "shared")
		}()
		go func() {
			defer wg.Done()
			m.GetContainerState(ctx, "shared")
		}()
		go func() {
			defer wg.Done()
			m.ListContainers(ctx)
		}()
		go func() {
			defer wg.Done()
			m.ContainerExists(ctx, "shared")
		}()
		go func() {
			defer wg.Done()
			m.IsConnected()
		}()
	}
	wg.Wait()
}

func TestConcurrentMixedReadWrite(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 3)

	// Writers: create containers
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			m.CreateContainer(ctx, baseCfg(fmt.Sprintf("rw-%d", idx)))
		}(i)
	}

	// Readers: list containers
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			m.ListContainers(ctx)
		}()
	}

	// Readers: check existence
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			m.ContainerExists(ctx, fmt.Sprintf("rw-%d", idx))
		}(i)
	}

	wg.Wait()
}

func TestConcurrentSnapshotOperations(t *testing.T) {
	m := connectedMock(t, baseCfg("snap-race"))
	ctx := context.Background()

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n * 2)

	// Create snapshots concurrently
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			m.CreateSnapshot(ctx, "snap-race", fmt.Sprintf("snap-%d", idx), false)
		}(i)
	}

	// List snapshots concurrently
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			m.ListSnapshots(ctx, "snap-race")
		}()
	}

	wg.Wait()

	snaps, err := m.ListSnapshots(ctx, "snap-race")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != n {
		t.Errorf("expected %d snapshots, got %d", n, len(snaps))
	}
}

func TestConcurrentLifecycle(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)

	// Each goroutine does a full lifecycle on its own container
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("lifecycle-%d", idx)

			m.CreateContainer(ctx, baseCfg(name))
			m.StartContainer(ctx, name)
			m.CreateSnapshot(ctx, name, "snap", false)
			m.StopContainer(ctx, name, false, 30)
			m.DeleteContainer(ctx, name)
		}(i)
	}

	wg.Wait()

	list, _ := m.ListContainers(ctx)
	if len(list) != 0 {
		t.Errorf("expected 0 containers after cleanup, got %d", len(list))
	}
}

// ============================================================================
// 16. RETRY LOGIC SIMULATION
// ============================================================================

// retryableClient wraps a Client to simulate transient failures
// and test retry logic patterns.
type retryableClient struct {
	Client
	mu           sync.Mutex
	failCount    int
	maxFails     int
	failErr      error
}

func (r *retryableClient) CreateContainer(ctx context.Context, cfg ContainerConfig) (*Operation, error) {
	r.mu.Lock()
	if r.failCount < r.maxFails {
		r.failCount++
		r.mu.Unlock()
		return nil, r.failErr
	}
	r.mu.Unlock()
	return r.Client.CreateContainer(ctx, cfg)
}

// retry executes fn up to maxRetries times with backoff.
func retry(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if i < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(baseDelay):
			}
		}
	}
	return lastErr
}

func TestRetryLogic_EventualSuccess(t *testing.T) {
	base := connectedMock(t)
	rc := &retryableClient{
		Client:   base,
		maxFails: 3,
		failErr:  ErrNetworkError,
	}
	ctx := context.Background()

	var op *Operation
	err := retry(ctx, 5, time.Millisecond, func() error {
		var e error
		op, e = rc.CreateContainer(ctx, baseCfg("retry-test"))
		return e
	})

	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if op == nil {
		t.Fatal("expected non-nil operation")
	}

	rc.mu.Lock()
	if rc.failCount != 3 {
		t.Errorf("expected 3 failures before success, got %d", rc.failCount)
	}
	rc.mu.Unlock()
}

func TestRetryLogic_MaxRetriesExhausted(t *testing.T) {
	base := connectedMock(t)
	rc := &retryableClient{
		Client:   base,
		maxFails: 100, // always fail
		failErr:  ErrNetworkError,
	}
	ctx := context.Background()

	err := retry(ctx, 3, time.Millisecond, func() error {
		_, e := rc.CreateContainer(ctx, baseCfg("retry-fail"))
		return e
	})

	if !errors.Is(err, ErrNetworkError) {
		t.Fatalf("expected ErrNetworkError after exhausting retries, got %v", err)
	}
}

func TestRetryLogic_ContextCancellation(t *testing.T) {
	base := connectedMock(t)
	rc := &retryableClient{
		Client:   base,
		maxFails: 100,
		failErr:  ErrNetworkError,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so retry exits fast
	cancel()

	err := retry(ctx, 10, time.Second, func() error {
		_, e := rc.CreateContainer(ctx, baseCfg("cancel-test"))
		return e
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetryLogic_ContextTimeout(t *testing.T) {
	base := connectedMock(t)
	rc := &retryableClient{
		Client:   base,
		maxFails: 100,
		failErr:  ErrNetworkError,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	err := retry(ctx, 100, 10*time.Millisecond, func() error {
		_, e := rc.CreateContainer(ctx, baseCfg("timeout-test"))
		return e
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestRetryLogic_ImmediateSuccess(t *testing.T) {
	base := connectedMock(t)
	rc := &retryableClient{
		Client:   base,
		maxFails: 0, // no failures
		failErr:  ErrNetworkError,
	}
	ctx := context.Background()

	err := retry(ctx, 3, time.Millisecond, func() error {
		_, e := rc.CreateContainer(ctx, baseCfg("no-retry"))
		return e
	})

	if err != nil {
		t.Fatalf("expected immediate success, got %v", err)
	}
}

// ============================================================================
// 17. ERROR SENTINEL VALUES
// ============================================================================

func TestErrorSentinels(t *testing.T) {
	errs := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrClientNotConnected", ErrClientNotConnected, "LXD client not connected"},
		{"ErrContainerNotFound", ErrContainerNotFound, "container not found"},
		{"ErrContainerExists", ErrContainerExists, "container already exists"},
		{"ErrInvalidConfig", ErrInvalidConfig, "invalid container configuration"},
		{"ErrOperationTimeout", ErrOperationTimeout, "operation timed out"},
		{"ErrImageNotFound", ErrImageNotFound, "image not found"},
		{"ErrNetworkError", ErrNetworkError, "network configuration error"},
		{"ErrStorageError", ErrStorageError, "storage configuration error"},
		{"ErrSnapshotNotFound", ErrSnapshotNotFound, "snapshot not found"},
	}

	for _, tt := range errs {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("error message: want %q, got %q", tt.msg, tt.err.Error())
			}
			// Sentinel errors should be unwrappable via errors.Is
			wrapped := fmt.Errorf("operation failed: %w", tt.err)
			if !errors.Is(wrapped, tt.err) {
				t.Errorf("wrapped error should match via errors.Is")
			}
		})
	}
}

// ============================================================================
// 18. EDGE CASES
// ============================================================================

func TestCreateContainerWithNilMaps(t *testing.T) {
	m := connectedMock(t)
	ctx := context.Background()

	cfg := ContainerConfig{
		Name:  "nil-maps",
		Image: "nixos:latest",
		// Devices, Config, Profiles are all nil
	}

	_, err := m.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer with nil maps: %v", err)
	}

	info, err := m.GetContainer(ctx, "nil-maps")
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	// Should store nil maps without panicking
	if info.Name != "nil-maps" {
		t.Errorf("name: want nil-maps, got %s", info.Name)
	}
}

func TestLastUsedAtUpdatesOnStart(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	info, _ := m.GetContainer(ctx, "c1")
	beforeStart := info.LastUsedAt

	// Small sleep to ensure time difference
	time.Sleep(time.Millisecond)

	m.StartContainer(ctx, "c1")
	info, _ = m.GetContainer(ctx, "c1")
	if !info.LastUsedAt.After(beforeStart) {
		t.Error("LastUsedAt should be updated after start")
	}
}

func TestOperationTimestamps(t *testing.T) {
	m := connectedMock(t, baseCfg("ts"))
	ctx := context.Background()

	before := time.Now()
	op, _ := m.StartContainer(ctx, "ts")
	after := time.Now()

	if op.CreatedAt.Before(before) || op.CreatedAt.After(after) {
		t.Error("operation CreatedAt should be between test bounds")
	}
	if op.UpdatedAt.Before(before) || op.UpdatedAt.After(after) {
		t.Error("operation UpdatedAt should be between test bounds")
	}
}

func TestMultipleDisconnectIdempotent(t *testing.T) {
	m := connectedMock(t)

	for i := 0; i < 5; i++ {
		if err := m.Disconnect(); err != nil {
			t.Fatalf("Disconnect #%d: %v", i, err)
		}
	}
	if m.IsConnected() {
		t.Error("should remain disconnected")
	}
}

func TestMultipleConnectIdempotent(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := m.Connect(ctx); err != nil {
			t.Fatalf("Connect #%d: %v", i, err)
		}
	}
	if !m.IsConnected() {
		t.Error("should remain connected")
	}
}

func TestStartAlreadyRunningContainer(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	m.StartContainer(ctx, "c1")

	// Start again - mock allows it without error
	op, err := m.StartContainer(ctx, "c1")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if op.Status != "Success" {
		t.Errorf("expected Success, got %s", op.Status)
	}

	info, _ := m.GetContainer(ctx, "c1")
	if info.Status != "Running" {
		t.Errorf("expected Running, got %s", info.Status)
	}
}

func TestStopAlreadyStoppedContainer(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()
	// Container is created in Stopped state

	op, err := m.StopContainer(ctx, "c1", false, 30)
	if err != nil {
		t.Fatalf("stop already stopped: %v", err)
	}
	if op.Status != "Success" {
		t.Errorf("expected Success, got %s", op.Status)
	}
}

func TestExecContainerWithNilEnv(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()
	m.StartContainer(ctx, "c1")

	result, err := m.ExecContainer(ctx, "c1", []string{"whoami"}, nil)
	if err != nil {
		t.Fatalf("ExecContainer nil env: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestPushFileEmptyContent(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	err := m.PushFile(ctx, "c1", "/empty", []byte{}, 0644)
	if err != nil {
		t.Fatalf("PushFile empty: %v", err)
	}
}

func TestPushFileNilContent(t *testing.T) {
	m := connectedMock(t, baseCfg("c1"))
	ctx := context.Background()

	err := m.PushFile(ctx, "c1", "/nil", nil, 0644)
	if err != nil {
		t.Fatalf("PushFile nil content: %v", err)
	}
}

// ============================================================================
// 19. REST API CLIENT MOCKING PATTERN
// ============================================================================
//
// Demonstrates how to use the Client interface to mock LXD REST API
// interactions in higher-level service tests.

// ContainerManager represents a higher-level orchestrator that uses the
// Client interface. This pattern tests that the interface is usable as
// a mock injection point for REST API-driven code.
type ContainerManager struct {
	client Client
}

func NewContainerManager(c Client) *ContainerManager {
	return &ContainerManager{client: c}
}

// ProvisionContainer creates, starts, and verifies a container is running.
func (cm *ContainerManager) ProvisionContainer(ctx context.Context, cfg ContainerConfig) error {
	op, err := cm.client.CreateContainer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if err := cm.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait create: %w", err)
	}

	op, err = cm.client.StartContainer(ctx, cfg.Name)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := cm.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait start: %w", err)
	}

	info, err := cm.client.GetContainer(ctx, cfg.Name)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if info.Status != "Running" {
		return fmt.Errorf("container not running, status: %s", info.Status)
	}

	return nil
}

// DecommissionContainer stops and deletes a container.
func (cm *ContainerManager) DecommissionContainer(ctx context.Context, name string) error {
	_, err := cm.client.StopContainer(ctx, name, true, 30)
	if err != nil {
		return fmt.Errorf("stop: %w", err)
	}

	_, err = cm.client.DeleteContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}

// SnapshotAndRestore creates a snapshot and then restores it.
func (cm *ContainerManager) SnapshotAndRestore(ctx context.Context, container, snapName string) error {
	_, err := cm.client.CreateSnapshot(ctx, container, snapName, false)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	_, err = cm.client.RestoreSnapshot(ctx, container, snapName)
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	return nil
}

func TestContainerManagerProvision(t *testing.T) {
	mock := connectedMock(t)
	mgr := NewContainerManager(mock)
	ctx := context.Background()

	err := mgr.ProvisionContainer(ctx, baseCfg("app-server"))
	if err != nil {
		t.Fatalf("ProvisionContainer: %v", err)
	}

	info, _ := mock.GetContainer(ctx, "app-server")
	if info.Status != "Running" {
		t.Errorf("expected Running, got %s", info.Status)
	}
}

func TestContainerManagerProvision_DuplicateName(t *testing.T) {
	mock := connectedMock(t, baseCfg("exists"))
	mgr := NewContainerManager(mock)
	ctx := context.Background()

	err := mgr.ProvisionContainer(ctx, baseCfg("exists"))
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !errors.Is(err, ErrContainerExists) {
		t.Errorf("expected ErrContainerExists in chain, got %v", err)
	}
}

func TestContainerManagerDecommission(t *testing.T) {
	mock := connectedMock(t, baseCfg("doomed"))
	ctx := context.Background()
	mock.StartContainer(ctx, "doomed")

	mgr := NewContainerManager(mock)
	err := mgr.DecommissionContainer(ctx, "doomed")
	if err != nil {
		t.Fatalf("DecommissionContainer: %v", err)
	}

	exists, _ := mock.ContainerExists(ctx, "doomed")
	if exists {
		t.Error("container should not exist after decommission")
	}
}

func TestContainerManagerDecommission_NotFound(t *testing.T) {
	mock := connectedMock(t)
	mgr := NewContainerManager(mock)
	ctx := context.Background()

	err := mgr.DecommissionContainer(ctx, "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent container")
	}
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound in chain, got %v", err)
	}
}

func TestContainerManagerSnapshotAndRestore(t *testing.T) {
	mock := connectedMock(t, baseCfg("snap-target"))
	mgr := NewContainerManager(mock)
	ctx := context.Background()

	err := mgr.SnapshotAndRestore(ctx, "snap-target", "backup-1")
	if err != nil {
		t.Fatalf("SnapshotAndRestore: %v", err)
	}

	snaps, _ := mock.ListSnapshots(ctx, "snap-target")
	if len(snaps) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(snaps))
	}
}

func TestContainerManagerSnapshotAndRestore_NoContainer(t *testing.T) {
	mock := connectedMock(t)
	mgr := NewContainerManager(mock)
	ctx := context.Background()

	err := mgr.SnapshotAndRestore(ctx, "ghost", "snap")
	if err == nil {
		t.Fatal("expected error for non-existent container")
	}
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound in chain, got %v", err)
	}
}

// ============================================================================
// 20. CONCURRENT MANAGER OPERATIONS (full integration)
// ============================================================================

func TestConcurrentManagerProvisioning(t *testing.T) {
	mock := connectedMock(t)
	mgr := NewContainerManager(mock)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = mgr.ProvisionContainer(ctx, baseCfg(fmt.Sprintf("svc-%d", idx)))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	list, _ := mock.ListContainers(ctx)
	if len(list) != n {
		t.Errorf("expected %d containers, got %d", n, len(list))
	}
}

// ============================================================================
// 21. TYPE ASSERTIONS AND STRUCT FIELD VALIDATION
// ============================================================================

func TestContainerConfigFields(t *testing.T) {
	cfg := ContainerConfig{
		Name:         "test",
		Image:        "nixos:latest",
		Profiles:     []string{"default", "hardened"},
		Architecture: "x86_64",
		Ephemeral:    true,
		Config: map[string]string{
			"limits.cpu":    "2",
			"limits.memory": "2GB",
		},
		Devices: map[string]Device{
			"eth0": {
				Type:       "nic",
				Properties: map[string]string{"network": "lxdbr0", "name": "eth0"},
			},
		},
	}

	if cfg.Name != "test" {
		t.Errorf("Name: %s", cfg.Name)
	}
	if cfg.Image != "nixos:latest" {
		t.Errorf("Image: %s", cfg.Image)
	}
	if len(cfg.Profiles) != 2 {
		t.Errorf("Profiles: %v", cfg.Profiles)
	}
	if cfg.Architecture != "x86_64" {
		t.Errorf("Architecture: %s", cfg.Architecture)
	}
	if !cfg.Ephemeral {
		t.Error("Ephemeral should be true")
	}
	if cfg.Config["limits.cpu"] != "2" {
		t.Errorf("Config: %v", cfg.Config)
	}
	if cfg.Devices["eth0"].Type != "nic" {
		t.Errorf("Device type: %s", cfg.Devices["eth0"].Type)
	}
}

func TestContainerStateFields(t *testing.T) {
	state := ContainerState{
		Status:     "Running",
		StatusCode: 103,
		CPU:        CPUState{Usage: 12345678},
		Memory: MemoryState{
			Usage:     1024 * 1024 * 256,
			UsagePeak: 1024 * 1024 * 512,
			SwapUsage: 0,
		},
		Network: map[string]NetState{
			"eth0": {
				Addresses: []Address{
					{Address: "10.10.10.20", Family: "inet", Netmask: "24", Scope: "global"},
				},
				Counters: NetCounters{
					BytesReceived:   1000,
					BytesSent:       2000,
					PacketsReceived: 100,
					PacketsSent:     200,
				},
				Hwaddr: "00:16:3e:xx:xx:xx",
				Mtu:    1500,
				State:  "up",
				Type:   "broadcast",
			},
		},
		Disk: map[string]DiskState{
			"root": {Usage: 1024 * 1024 * 1024},
		},
		Pid:       42,
		Processes: 15,
	}

	if state.CPU.Usage != 12345678 {
		t.Errorf("CPU usage: %d", state.CPU.Usage)
	}
	if state.Memory.UsagePeak != 1024*1024*512 {
		t.Errorf("Memory peak: %d", state.Memory.UsagePeak)
	}
	if state.Network["eth0"].Addresses[0].Address != "10.10.10.20" {
		t.Error("Network address mismatch")
	}
	if state.Network["eth0"].Counters.BytesSent != 2000 {
		t.Error("Net counter mismatch")
	}
	if state.Disk["root"].Usage != 1024*1024*1024 {
		t.Error("Disk usage mismatch")
	}
	if state.Pid != 42 {
		t.Errorf("Pid: %d", state.Pid)
	}
}

func TestOperationFields(t *testing.T) {
	now := time.Now()
	op := Operation{
		ID:          "op-123",
		Type:        "container_create",
		Status:      "Running",
		StatusCode:  200,
		Resources:   map[string][]string{"containers": {"/1.0/containers/test"}},
		Metadata:    map[string]interface{}{"key": "value"},
		MayCancel:   true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Description: "Creating container",
		Err:         "",
	}

	if op.ID != "op-123" {
		t.Errorf("ID: %s", op.ID)
	}
	if !op.MayCancel {
		t.Error("MayCancel should be true")
	}
	if op.Description != "Creating container" {
		t.Errorf("Description: %s", op.Description)
	}
	if op.Resources["containers"][0] != "/1.0/containers/test" {
		t.Errorf("Resources: %v", op.Resources)
	}
}

func TestSnapshotFields(t *testing.T) {
	now := time.Now()
	expires := now.Add(24 * time.Hour)
	snap := Snapshot{
		Name:      "daily-backup",
		Stateful:  true,
		CreatedAt: now,
		ExpiresAt: expires,
		Config:    map[string]string{"key": "value"},
	}

	if snap.Name != "daily-backup" {
		t.Errorf("Name: %s", snap.Name)
	}
	if !snap.Stateful {
		t.Error("Stateful should be true")
	}
	if snap.ExpiresAt.Before(snap.CreatedAt) {
		t.Error("ExpiresAt should be after CreatedAt")
	}
	if snap.Config["key"] != "value" {
		t.Errorf("Config: %v", snap.Config)
	}
}

func TestServerInfoFields(t *testing.T) {
	info := ServerInfo{
		APIVersion:  "1.0",
		Auth:        "trusted",
		Public:      false,
		AuthMethods: []string{"tls", "candid"},
		Environment: map[string]string{
			"kernel":  "linux",
			"storage": "zfs",
		},
		Config: map[string]interface{}{
			"core.https_address": ":8443",
		},
	}

	if info.APIVersion != "1.0" {
		t.Errorf("APIVersion: %s", info.APIVersion)
	}
	if len(info.AuthMethods) != 2 {
		t.Errorf("AuthMethods: %v", info.AuthMethods)
	}
	if info.Config["core.https_address"] != ":8443" {
		t.Errorf("Config: %v", info.Config)
	}
}

func TestExecResultFields(t *testing.T) {
	r := ExecResult{
		Stdout:   []byte("hello world"),
		Stderr:   []byte("warning: something"),
		ExitCode: 0,
	}

	if string(r.Stdout) != "hello world" {
		t.Errorf("Stdout: %s", r.Stdout)
	}
	if string(r.Stderr) != "warning: something" {
		t.Errorf("Stderr: %s", r.Stderr)
	}
	if r.ExitCode != 0 {
		t.Errorf("ExitCode: %d", r.ExitCode)
	}
}

func TestClientConfigFields(t *testing.T) {
	cfg := ClientConfig{
		Socket:     "/var/snap/lxd/common/lxd/unix.socket",
		HTTPS:      "https://lxd.example.com:8443",
		TLSCert:    "/path/cert.pem",
		TLSKey:     "/path/key.pem",
		TLSCA:      "/path/ca.pem",
		Timeout:    45 * time.Second,
		SkipVerify: true,
	}

	if cfg.Socket != "/var/snap/lxd/common/lxd/unix.socket" {
		t.Errorf("Socket: %s", cfg.Socket)
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("Timeout: %v", cfg.Timeout)
	}
	if !cfg.SkipVerify {
		t.Error("SkipVerify should be true")
	}
}

func TestDeviceFields(t *testing.T) {
	d := Device{
		Type: "gpu",
		Properties: map[string]string{
			"vendorid":  "10de",
			"productid": "1b06",
		},
	}

	if d.Type != "gpu" {
		t.Errorf("Type: %s", d.Type)
	}
	if d.Properties["vendorid"] != "10de" {
		t.Errorf("Properties: %v", d.Properties)
	}
}

func TestAddressFields(t *testing.T) {
	a := Address{
		Address: "10.10.10.20",
		Family:  "inet",
		Netmask: "24",
		Scope:   "global",
	}

	if a.Address != "10.10.10.20" {
		t.Errorf("Address: %s", a.Address)
	}
	if a.Family != "inet" {
		t.Errorf("Family: %s", a.Family)
	}
}
