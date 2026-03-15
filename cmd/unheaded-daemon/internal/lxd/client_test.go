// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package lxd

import (
	"context"
	"testing"
)

func TestNewMockClient(t *testing.T) {
	client := NewMockClient()
	if client == nil {
		t.Fatal("NewMockClient returned nil")
	}
	if client.IsConnected() {
		t.Error("new mock client should not be connected")
	}
}

func TestMockClient_ConnectDisconnect(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if !client.IsConnected() {
		t.Error("expected connected after Connect")
	}

	if err := client.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}
	if client.IsConnected() {
		t.Error("expected disconnected after Disconnect")
	}
}

func TestMockClient_NotConnectedErrors(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()

	// All operations should fail when not connected
	_, err := client.ServerInfo(ctx)
	if err != ErrClientNotConnected {
		t.Errorf("ServerInfo: expected ErrClientNotConnected, got %v", err)
	}

	_, err = client.CreateContainer(ctx, ContainerConfig{Name: "test"})
	if err != ErrClientNotConnected {
		t.Errorf("CreateContainer: expected ErrClientNotConnected, got %v", err)
	}

	_, err = client.ListContainers(ctx)
	if err != ErrClientNotConnected {
		t.Errorf("ListContainers: expected ErrClientNotConnected, got %v", err)
	}

	_, err = client.GetContainer(ctx, "test")
	if err != ErrClientNotConnected {
		t.Errorf("GetContainer: expected ErrClientNotConnected, got %v", err)
	}

	_, err = client.ContainerExists(ctx, "test")
	if err != ErrClientNotConnected {
		t.Errorf("ContainerExists: expected ErrClientNotConnected, got %v", err)
	}
}

func TestMockClient_ContainerLifecycle(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	cfg := ContainerConfig{
		Name:         "test-container",
		Image:        "nixos:latest",
		Profiles:     []string{"default"},
		Architecture: "x86_64",
	}

	// Create
	op, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	if op == nil {
		t.Fatal("expected operation, got nil")
	}
	if op.Status != "Success" {
		t.Errorf("expected operation status 'Success', got %q", op.Status)
	}

	// Verify exists
	exists, err := client.ContainerExists(ctx, "test-container")
	if err != nil {
		t.Fatalf("ContainerExists failed: %v", err)
	}
	if !exists {
		t.Error("expected container to exist")
	}

	// Get info
	info, err := client.GetContainer(ctx, "test-container")
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}
	if info.Name != "test-container" {
		t.Errorf("expected name 'test-container', got %q", info.Name)
	}
	if info.Status != "Stopped" {
		t.Errorf("expected status 'Stopped', got %q", info.Status)
	}

	// Start
	_, err = client.StartContainer(ctx, "test-container")
	if err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	info, _ = client.GetContainer(ctx, "test-container")
	if info.Status != "Running" {
		t.Errorf("expected status 'Running' after start, got %q", info.Status)
	}

	// Get state
	state, err := client.GetContainerState(ctx, "test-container")
	if err != nil {
		t.Fatalf("GetContainerState failed: %v", err)
	}
	if state.Status != "Running" {
		t.Errorf("expected state 'Running', got %q", state.Status)
	}
	if state.Pid == 0 {
		t.Error("expected non-zero PID for running container")
	}

	// Stop
	_, err = client.StopContainer(ctx, "test-container", false, 30)
	if err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}

	info, _ = client.GetContainer(ctx, "test-container")
	if info.Status != "Stopped" {
		t.Errorf("expected status 'Stopped' after stop, got %q", info.Status)
	}

	// Delete
	_, err = client.DeleteContainer(ctx, "test-container")
	if err != nil {
		t.Fatalf("DeleteContainer failed: %v", err)
	}

	exists, _ = client.ContainerExists(ctx, "test-container")
	if exists {
		t.Error("expected container to not exist after deletion")
	}
}

func TestMockClient_CreateContainer_Errors(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	// Empty name
	_, err := client.CreateContainer(ctx, ContainerConfig{Name: ""})
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig for empty name, got %v", err)
	}

	// Create first
	client.CreateContainer(ctx, ContainerConfig{Name: "dup"})

	// Duplicate
	_, err = client.CreateContainer(ctx, ContainerConfig{Name: "dup"})
	if err != ErrContainerExists {
		t.Errorf("expected ErrContainerExists for duplicate, got %v", err)
	}
}

func TestMockClient_ContainerNotFound(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	_, err := client.GetContainer(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}

	_, err = client.StartContainer(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound for start, got %v", err)
	}

	_, err = client.StopContainer(ctx, "nonexistent", false, 0)
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound for stop, got %v", err)
	}

	_, err = client.DeleteContainer(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound for delete, got %v", err)
	}
}

func TestMockClient_FreezeUnfreeze(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	client.CreateContainer(ctx, ContainerConfig{Name: "freeze-test"})
	client.StartContainer(ctx, "freeze-test")

	// Freeze
	_, err := client.FreezeContainer(ctx, "freeze-test")
	if err != nil {
		t.Fatalf("FreezeContainer failed: %v", err)
	}

	info, _ := client.GetContainer(ctx, "freeze-test")
	if info.Status != "Frozen" {
		t.Errorf("expected 'Frozen', got %q", info.Status)
	}

	// Unfreeze
	_, err = client.UnfreezeContainer(ctx, "freeze-test")
	if err != nil {
		t.Fatalf("UnfreezeContainer failed: %v", err)
	}

	info, _ = client.GetContainer(ctx, "freeze-test")
	if info.Status != "Running" {
		t.Errorf("expected 'Running' after unfreeze, got %q", info.Status)
	}
}

func TestMockClient_RestartContainer(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	client.CreateContainer(ctx, ContainerConfig{Name: "restart-test"})
	client.StartContainer(ctx, "restart-test")

	_, err := client.RestartContainer(ctx, "restart-test", false, 30)
	if err != nil {
		t.Fatalf("RestartContainer failed: %v", err)
	}

	info, _ := client.GetContainer(ctx, "restart-test")
	if info.Status != "Running" {
		t.Errorf("expected 'Running' after restart, got %q", info.Status)
	}
}

func TestMockClient_ListContainers(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	// Empty list
	containers, err := client.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("expected 0 containers, got %d", len(containers))
	}

	// Create some containers
	client.CreateContainer(ctx, ContainerConfig{Name: "c1"})
	client.CreateContainer(ctx, ContainerConfig{Name: "c2"})
	client.CreateContainer(ctx, ContainerConfig{Name: "c3"})

	containers, err = client.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	if len(containers) != 3 {
		t.Errorf("expected 3 containers, got %d", len(containers))
	}
}

func TestMockClient_RenameContainer(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	client.CreateContainer(ctx, ContainerConfig{Name: "old-name"})

	_, err := client.RenameContainer(ctx, "old-name", "new-name")
	if err != nil {
		t.Fatalf("RenameContainer failed: %v", err)
	}

	// Old name should not exist
	exists, _ := client.ContainerExists(ctx, "old-name")
	if exists {
		t.Error("expected old name to not exist")
	}

	// New name should exist
	exists, _ = client.ContainerExists(ctx, "new-name")
	if !exists {
		t.Error("expected new name to exist")
	}

	info, _ := client.GetContainer(ctx, "new-name")
	if info.Name != "new-name" {
		t.Errorf("expected name 'new-name', got %q", info.Name)
	}
}

func TestMockClient_RenameContainer_Errors(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	// Source not found
	_, err := client.RenameContainer(ctx, "nonexistent", "new")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}

	// Target exists
	client.CreateContainer(ctx, ContainerConfig{Name: "src"})
	client.CreateContainer(ctx, ContainerConfig{Name: "dst"})

	_, err = client.RenameContainer(ctx, "src", "dst")
	if err != ErrContainerExists {
		t.Errorf("expected ErrContainerExists, got %v", err)
	}
}

func TestMockClient_Snapshots(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	client.CreateContainer(ctx, ContainerConfig{Name: "snap-test"})

	// Create snapshot
	_, err := client.CreateSnapshot(ctx, "snap-test", "snap1", false)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// List snapshots
	snaps, err := client.ListSnapshots(ctx, "snap-test")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].Name != "snap1" {
		t.Errorf("expected snapshot name 'snap1', got %q", snaps[0].Name)
	}

	// Restore snapshot
	_, err = client.RestoreSnapshot(ctx, "snap-test", "snap1")
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	// Delete snapshot
	_, err = client.DeleteSnapshot(ctx, "snap-test", "snap1")
	if err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	snaps, _ = client.ListSnapshots(ctx, "snap-test")
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots after delete, got %d", len(snaps))
	}
}

func TestMockClient_Snapshot_Errors(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	// Nonexistent container
	_, err := client.CreateSnapshot(ctx, "nocontainer", "s1", false)
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}

	// Nonexistent snapshot
	client.CreateContainer(ctx, ContainerConfig{Name: "has-snaps"})
	_, err = client.DeleteSnapshot(ctx, "has-snaps", "no-snap")
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}

	_, err = client.RestoreSnapshot(ctx, "has-snaps", "no-snap")
	if err != ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestMockClient_ExecContainer(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	client.CreateContainer(ctx, ContainerConfig{Name: "exec-test"})

	// Exec on stopped container
	result, err := client.ExecContainer(ctx, "exec-test", []string{"ls"}, nil)
	if err != nil {
		t.Fatalf("ExecContainer failed: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1 for stopped container, got %d", result.ExitCode)
	}

	// Start and exec
	client.StartContainer(ctx, "exec-test")
	result, err = client.ExecContainer(ctx, "exec-test", []string{"ls"}, nil)
	if err != nil {
		t.Fatalf("ExecContainer failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for running container, got %d", result.ExitCode)
	}
}

func TestMockClient_FileOps(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	client.CreateContainer(ctx, ContainerConfig{Name: "file-test"})

	// Push file
	err := client.PushFile(ctx, "file-test", "/tmp/test.txt", []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("PushFile failed: %v", err)
	}

	// Pull file
	content, err := client.PullFile(ctx, "file-test", "/tmp/test.txt")
	if err != nil {
		t.Fatalf("PullFile failed: %v", err)
	}
	if string(content) != "mock file content" {
		t.Errorf("expected mock content, got %q", string(content))
	}

	// File ops on nonexistent container
	err = client.PushFile(ctx, "nonexistent", "/tmp/test.txt", []byte("hello"), 0644)
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestMockClient_ServerInfo(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	info, err := client.ServerInfo(ctx)
	if err != nil {
		t.Fatalf("ServerInfo failed: %v", err)
	}
	if info.APIVersion != "1.0" {
		t.Errorf("expected API version '1.0', got %q", info.APIVersion)
	}
	if info.Auth != "trusted" {
		t.Errorf("expected auth 'trusted', got %q", info.Auth)
	}
}

func TestMockClient_WaitOperation(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()

	// WaitOperation should succeed immediately
	err := client.WaitOperation(ctx, &Operation{ID: "test"})
	if err != nil {
		t.Errorf("WaitOperation failed: %v", err)
	}
}

func TestMockClient_CancelOperation(t *testing.T) {
	client := NewMockClient()
	ctx := context.Background()
	client.Connect(ctx)

	client.CreateContainer(ctx, ContainerConfig{Name: "cancel-test"})
	op, _ := client.StartContainer(ctx, "cancel-test")

	err := client.CancelOperation(ctx, op)
	if err != nil {
		t.Errorf("CancelOperation failed: %v", err)
	}
}

func TestNewClient(t *testing.T) {
	// NewClient currently returns a MockClient
	c, err := NewClient(ClientConfig{Socket: "/tmp/test.socket"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	// Verify it implements Client interface
	var _ Client = c
}
