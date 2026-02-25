// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Container Lifecycle Tests
// =============================================================================

// TestContainerCreate_BasicCreation tests basic container creation.
func TestContainerCreate_BasicCreation(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	// Add mock image
	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-container-1",
		Name:  "test",
		Image: "alpine:latest",
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.ID != "test-container-1" {
		t.Errorf("Expected ID 'test-container-1', got '%s'", info.ID)
	}

	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created, got %v", info.State)
	}

	if info.Image != "alpine:latest" {
		t.Errorf("Expected image 'alpine:latest', got '%s'", info.Image)
	}
}

// TestContainerCreate_WithoutID tests container creation with auto-generated ID.
func TestContainerCreate_WithoutID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		Name:  "test",
		Image: "alpine:latest",
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.ID == "" {
		t.Error("Expected auto-generated ID, got empty string")
	}
}

// TestContainerCreate_DuplicateID tests that duplicate container IDs are rejected.
func TestContainerCreate_DuplicateID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-container-dup",
		Name:  "test",
		Image: "alpine:latest",
	}

	_, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("First CreateContainer failed: %v", err)
	}

	// Try to create with same ID
	_, err = rt.CreateContainer(ctx, config)
	if err != ErrContainerExists {
		t.Errorf("Expected ErrContainerExists, got %v", err)
	}
}

// TestContainerCreate_NilConfig tests that nil config returns error.
func TestContainerCreate_NilConfig(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.CreateContainer(ctx, nil)
	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

// TestContainerCreate_ImageNotFound tests container creation with non-existent image.
func TestContainerCreate_ImageNotFound(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &ContainerConfig{
		ID:    "test-container",
		Name:  "test",
		Image: "nonexistent:latest",
	}

	_, err := rt.CreateContainer(ctx, config)
	if err == nil {
		t.Error("Expected error for non-existent image")
	}
}

// TestContainerCreate_WithMounts tests container creation with mount configuration.
func TestContainerCreate_WithMounts(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	// Create source directory for bind mount
	srcDir := filepath.Join(root, "src")
	os.MkdirAll(srcDir, 0755)

	config := &ContainerConfig{
		ID:    "test-container-mounts",
		Name:  "test",
		Image: "alpine:latest",
		Mounts: []Mount{
			{
				Type:        MountTypeTmpfs,
				Destination: "/tmp",
				Options:     []string{"size=64m"},
			},
		},
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created, got %v", info.State)
	}
}

// TestContainerCreate_WithResources tests container creation with resource limits.
func TestContainerCreate_WithResources(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-container-resources",
		Name:  "test",
		Image: "alpine:latest",
		Resources: &ResourceConfig{
			MemoryLimitBytes: 256 * 1024 * 1024, // 256MB
			CPUShares:        512,
			PidsLimit:        100,
		},
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created, got %v", info.State)
	}
}

// =============================================================================
// Container Get Tests
// =============================================================================

// TestContainerGet_Existing tests getting an existing container.
func TestContainerGet_Existing(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-container-get",
		Name:  "test",
		Image: "alpine:latest",
	}

	_, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	info, err := rt.GetContainer(ctx, "test-container-get")
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}

	if info.ID != "test-container-get" {
		t.Errorf("Expected ID 'test-container-get', got '%s'", info.ID)
	}
}

// TestContainerGet_NotFound tests getting a non-existent container.
func TestContainerGet_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.GetContainer(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// =============================================================================
// Container List Tests
// =============================================================================

// TestContainerList_Empty tests listing containers when none exist.
func TestContainerList_Empty(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	containers, err := rt.ListContainers(ctx, nil)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	if len(containers) != 0 {
		t.Errorf("Expected 0 containers, got %d", len(containers))
	}
}

// TestContainerList_Multiple tests listing multiple containers.
func TestContainerList_Multiple(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	// Create multiple containers
	for i := 0; i < 5; i++ {
		config := &ContainerConfig{
			Name:  "test",
			Image: "alpine:latest",
			Labels: map[string]string{
				"group": "test-list",
			},
		}
		_, err := rt.CreateContainer(ctx, config)
		if err != nil {
			t.Fatalf("CreateContainer %d failed: %v", i, err)
		}
	}

	containers, err := rt.ListContainers(ctx, nil)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	if len(containers) != 5 {
		t.Errorf("Expected 5 containers, got %d", len(containers))
	}
}

// TestContainerList_WithFilter tests listing containers with filters.
func TestContainerList_WithFilter(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	// Create containers with different labels
	for i := 0; i < 3; i++ {
		config := &ContainerConfig{
			Name:  "web",
			Image: "alpine:latest",
			Labels: map[string]string{
				"app": "web",
			},
		}
		rt.CreateContainer(ctx, config)
	}

	for i := 0; i < 2; i++ {
		config := &ContainerConfig{
			Name:  "db",
			Image: "alpine:latest",
			Labels: map[string]string{
				"app": "db",
			},
		}
		rt.CreateContainer(ctx, config)
	}

	// Filter by label
	filter := &ContainerFilter{
		Labels: map[string]string{"app": "web"},
	}

	containers, err := rt.ListContainers(ctx, filter)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	if len(containers) != 3 {
		t.Errorf("Expected 3 containers with app=web, got %d", len(containers))
	}
}

// TestContainerList_FilterByState tests filtering containers by state.
func TestContainerList_FilterByState(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	// Create a container
	config := &ContainerConfig{
		ID:    "test-filter-state",
		Name:  "test",
		Image: "alpine:latest",
	}
	rt.CreateContainer(ctx, config)

	// Filter for Created state
	filter := &ContainerFilter{
		State: ContainerStateCreated,
	}

	containers, err := rt.ListContainers(ctx, filter)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	if len(containers) != 1 {
		t.Errorf("Expected 1 container in Created state, got %d", len(containers))
	}

	// Filter for Running state (should be empty)
	filter.State = ContainerStateRunning
	containers, err = rt.ListContainers(ctx, filter)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	if len(containers) != 0 {
		t.Errorf("Expected 0 containers in Running state, got %d", len(containers))
	}
}

// =============================================================================
// Container Remove Tests
// =============================================================================

// TestContainerRemove_Created tests removing a created (not running) container.
func TestContainerRemove_Created(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-container-remove",
		Name:  "test",
		Image: "alpine:latest",
	}

	_, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	err = rt.RemoveContainer(ctx, "test-container-remove", false)
	if err != nil {
		t.Fatalf("RemoveContainer failed: %v", err)
	}

	// Verify container is removed
	_, err = rt.GetContainer(ctx, "test-container-remove")
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// TestContainerRemove_NotFound tests removing a non-existent container.
func TestContainerRemove_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.RemoveContainer(ctx, "nonexistent", false)
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// =============================================================================
// Container Stop Tests
// =============================================================================

// TestContainerStop_NotRunning tests stopping a non-running container.
func TestContainerStop_NotRunning(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-container-stop",
		Name:  "test",
		Image: "alpine:latest",
	}

	_, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	err = rt.StopContainer(ctx, "test-container-stop", 10*time.Second)
	if err != ErrContainerNotRunning {
		t.Errorf("Expected ErrContainerNotRunning, got %v", err)
	}
}

// TestContainerStop_NotFound tests stopping a non-existent container.
func TestContainerStop_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.StopContainer(ctx, "nonexistent", 10*time.Second)
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// =============================================================================
// Container Start Tests
// =============================================================================

// TestContainerStart_NotFound tests starting a non-existent container.
func TestContainerStart_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.StartContainer(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// =============================================================================
// Container Pause/Resume Tests
// =============================================================================

// TestContainerPause_NotFound tests pausing a non-existent container.
func TestContainerPause_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.PauseContainer(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// TestContainerPause_NotRunning tests pausing a non-running container.
func TestContainerPause_NotRunning(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-container-pause",
		Name:  "test",
		Image: "alpine:latest",
	}

	_, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	err = rt.PauseContainer(ctx, "test-container-pause")
	if err != ErrContainerNotRunning {
		t.Errorf("Expected ErrContainerNotRunning, got %v", err)
	}
}

// TestContainerResume_NotFound tests resuming a non-existent container.
func TestContainerResume_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.ResumeContainer(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// =============================================================================
// Container State Tracking Tests
// =============================================================================

// TestContainerStateTracking tests that container states are properly tracked.
func TestContainerStateTracking(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-state-tracking",
		Name:  "test",
		Image: "alpine:latest",
	}

	// Create container
	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created after create, got %v", info.State)
	}

	// Get and verify state
	info, err = rt.GetContainer(ctx, "test-state-tracking")
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}

	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created, got %v", info.State)
	}

	// Verify timestamps
	if info.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt")
	}

	if !info.StartedAt.IsZero() {
		t.Error("Expected zero StartedAt for created container")
	}
}

// =============================================================================
// Concurrent Container Operations Tests
// =============================================================================

// TestContainerConcurrentCreation tests concurrent container creation.
func TestContainerConcurrentCreation(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	var wg sync.WaitGroup
	errCh := make(chan error, 100)
	const count = 20

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			config := &ContainerConfig{
				Name:  "test",
				Image: "alpine:latest",
			}

			_, err := rt.CreateContainer(ctx, config)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent creation error: %v", err)
	}

	// Verify all containers were created
	containers, err := rt.ListContainers(ctx, nil)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	if len(containers) != count {
		t.Errorf("Expected %d containers, got %d", count, len(containers))
	}
}

// TestContainerConcurrentGetAndList tests concurrent get and list operations.
func TestContainerConcurrentGetAndList(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	// Create some containers first
	containerIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		config := &ContainerConfig{
			Name:  "test",
			Image: "alpine:latest",
		}
		info, err := rt.CreateContainer(ctx, config)
		if err != nil {
			t.Fatalf("CreateContainer failed: %v", err)
		}
		containerIDs[i] = info.ID
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent gets
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			containerID := containerIDs[idx%len(containerIDs)]
			_, err := rt.GetContainer(ctx, containerID)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	// Concurrent lists
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rt.ListContainers(ctx, nil)
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

// TestContainerCreate_EmptyName tests container creation with empty name.
func TestContainerCreate_EmptyName(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-empty-name",
		Name:  "",
		Image: "alpine:latest",
	}

	// Should succeed - name is optional
	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.ID != "test-empty-name" {
		t.Errorf("Expected ID 'test-empty-name', got '%s'", info.ID)
	}
}

// TestContainerCreate_LongID tests container creation with very long ID.
func TestContainerCreate_LongID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	longID := "a"
	for i := 0; i < 256; i++ {
		longID += "a"
	}

	config := &ContainerConfig{
		ID:    longID,
		Name:  "test",
		Image: "alpine:latest",
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.ID != longID {
		t.Errorf("ID mismatch")
	}
}

// TestContainerCreate_SpecialCharactersInLabels tests labels with special characters.
func TestContainerCreate_SpecialCharactersInLabels(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-special-labels",
		Name:  "test",
		Image: "alpine:latest",
		Labels: map[string]string{
			"io.kubernetes.pod.name":      "test-pod",
			"com.example/version":         "1.0.0-beta+build.123",
			"key.with.dots":               "value",
			"key_with_underscores":        "value",
			"key-with-dashes":             "value",
		},
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if len(info.Labels) != 5 {
		t.Errorf("Expected 5 labels, got %d", len(info.Labels))
	}
}

// TestContainerCreate_NilResources tests container creation with nil resources.
func TestContainerCreate_NilResources(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:        "test-nil-resources",
		Name:      "test",
		Image:     "alpine:latest",
		Resources: nil,
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created, got %v", info.State)
	}
}

// TestContainerCreate_ZeroResources tests container creation with zero resource limits.
func TestContainerCreate_ZeroResources(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-zero-resources",
		Name:  "test",
		Image: "alpine:latest",
		Resources: &ResourceConfig{
			MemoryLimitBytes: 0,
			CPUShares:        0,
			PidsLimit:        0,
		},
	}

	info, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created, got %v", info.State)
	}
}

// =============================================================================
// Container Cleanup Tests
// =============================================================================

// TestContainerRemove_CleansUpResources tests that remove properly cleans up resources.
func TestContainerRemove_CleansUpResources(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	addMockImage(t, rt.images, "sha256:abc123", "alpine:latest")

	config := &ContainerConfig{
		ID:    "test-cleanup",
		Name:  "test",
		Image: "alpine:latest",
		Resources: &ResourceConfig{
			MemoryLimitBytes: 256 * 1024 * 1024,
		},
	}

	_, err := rt.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	// Verify container directory exists
	containerDir := filepath.Join(root, "containers", "test-cleanup")
	if _, err := os.Stat(containerDir); os.IsNotExist(err) {
		t.Skip("Container directory not created (expected in mock)")
	}

	err = rt.RemoveContainer(ctx, "test-cleanup", false)
	if err != nil {
		t.Fatalf("RemoveContainer failed: %v", err)
	}

	// Verify container directory is removed
	if _, err := os.Stat(containerDir); !os.IsNotExist(err) {
		t.Errorf("Container directory should be removed after delete")
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// setupTestRuntime creates a test runtime instance.
func setupTestRuntime(t *testing.T, root string) *DefaultRuntime {
	t.Helper()

	config := &RuntimeConfig{
		Root:     root,
		StateDir: filepath.Join(root, "state"),
	}

	imgStore, err := NewImageStore(filepath.Join(root, "images"), nil)
	if err != nil {
		t.Fatalf("Failed to create image store: %v", err)
	}

	// Create minimal mock components
	rt := &DefaultRuntime{
		config:       config,
		containers:   make(map[string]*container),
		sandboxes:    make(map[string]*Sandbox),
		images:       imgStore,
		execSessions: make(map[string]*execSession),
		logStreams:   make(map[string]*logStream),
	}

	// Try to create real cgroup manager (will fail without privileges)
	cgroupMgr, err := NewCgroupManager("systemd", "/unheaded-test")
	if err != nil {
		// Create mock cgroup manager for non-root tests
		rt.cgroups = &CgroupManager{
			driver:      "systemd",
			cgroupPaths: make(map[string]string),
		}
	} else {
		rt.cgroups = cgroupMgr
	}

	// Try to create real namespace manager
	nsMgr, err := NewNamespaceManager()
	if err != nil {
		// Skip namespace manager for non-root tests
	} else {
		rt.namespaces = nsMgr
	}

	// Try to create volume manager
	volMgr, err := NewVolumeManager(filepath.Join(root, "volumes"))
	if err != nil {
		// Skip volume manager for non-root tests
	} else {
		rt.volumes = volMgr
	}

	return rt
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkContainerCreate(b *testing.B) {
	if os.Getuid() != 0 {
		b.Skip("test requires root privileges")
	}

	root, err := os.MkdirTemp("", "runtime-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(root)

	config := &RuntimeConfig{
		Root:     root,
		StateDir: filepath.Join(root, "state"),
	}

	imgStore, _ := NewImageStore(filepath.Join(root, "images"), nil)

	rt := &DefaultRuntime{
		config:       config,
		containers:   make(map[string]*container),
		sandboxes:    make(map[string]*Sandbox),
		images:       imgStore,
		execSessions: make(map[string]*execSession),
		logStreams:   make(map[string]*logStream),
		cgroups: &CgroupManager{
			driver:      "systemd",
			cgroupPaths: make(map[string]string),
		},
	}

	imgStore.mu.Lock()
	imgStore.images["sha256:abc123"] = &ImageInfo{
		ID:       "sha256:abc123",
		RepoTags: []string{"alpine:latest"},
		Config:   &ImageConfig{Cmd: []string{"/bin/sh"}},
		RootFS:   &RootFS{Type: "layers", DiffIDs: []string{}},
	}
	imgStore.mu.Unlock()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		containerConfig := &ContainerConfig{
			Name:  "bench-test",
			Image: "alpine:latest",
		}
		rt.CreateContainer(ctx, containerConfig)
	}
}

func BenchmarkContainerList(b *testing.B) {
	root, err := os.MkdirTemp("", "runtime-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(root)

	config := &RuntimeConfig{
		Root:     root,
		StateDir: filepath.Join(root, "state"),
	}

	imgStore, _ := NewImageStore(filepath.Join(root, "images"), nil)

	rt := &DefaultRuntime{
		config:       config,
		containers:   make(map[string]*container),
		images:       imgStore,
		execSessions: make(map[string]*execSession),
		logStreams:   make(map[string]*logStream),
	}

	// Pre-populate with containers
	for i := 0; i < 100; i++ {
		rt.containers[generateID()] = &container{
			info: &ContainerInfo{
				ID:    generateID(),
				State: ContainerStateCreated,
			},
		}
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.ListContainers(ctx, nil)
	}
}
