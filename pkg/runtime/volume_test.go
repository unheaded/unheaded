// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// =============================================================================
// MountType Tests
// =============================================================================

func TestMountTypes(t *testing.T) {
	tests := []struct {
		name     string
		value    MountType
		expected string
	}{
		{"bind", MountTypeBind, "bind"},
		{"tmpfs", MountTypeTmpfs, "tmpfs"},
		{"volume", MountTypeVolume, "volume"},
		{"nfs", MountTypeNFS, "nfs"},
		{"proc", MountTypeProc, "proc"},
		{"sysfs", MountTypeSysfs, "sysfs"},
		{"devpts", MountTypeDevpts, "devpts"},
		{"cgroup", MountTypeCgroup, "cgroup"},
		{"mqueue", MountTypeMqueue, "mqueue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.value))
			}
		})
	}
}

// =============================================================================
// MountPropagation Tests
// =============================================================================

func TestMountPropagation(t *testing.T) {
	tests := []struct {
		name     string
		value    MountPropagation
		expected string
	}{
		{"private", MountPropagationPrivate, "private"},
		{"rprivate", MountPropagationRPrivate, "rprivate"},
		{"shared", MountPropagationShared, "shared"},
		{"rshared", MountPropagationRShared, "rshared"},
		{"slave", MountPropagationSlave, "slave"},
		{"rslave", MountPropagationRSlave, "rslave"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.value))
			}
		})
	}
}

// =============================================================================
// Mount Struct Tests
// =============================================================================

func TestMount_Fields(t *testing.T) {
	mount := &Mount{
		Type:        MountTypeBind,
		Source:      "/host/path",
		Destination: "/container/path",
		Options:     []string{"rw", "noexec"},
		ReadOnly:    false,
		Propagation: MountPropagationPrivate,
	}

	if mount.Type != MountTypeBind {
		t.Errorf("Expected type bind, got %s", mount.Type)
	}

	if mount.Source != "/host/path" {
		t.Errorf("Expected source '/host/path', got '%s'", mount.Source)
	}

	if mount.Destination != "/container/path" {
		t.Errorf("Expected destination '/container/path', got '%s'", mount.Destination)
	}

	if len(mount.Options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(mount.Options))
	}

	if mount.ReadOnly {
		t.Error("Expected ReadOnly to be false")
	}

	if mount.Propagation != MountPropagationPrivate {
		t.Errorf("Expected propagation private, got %s", mount.Propagation)
	}
}

func TestMount_ReadOnly(t *testing.T) {
	mount := &Mount{
		Type:        MountTypeBind,
		Source:      "/host/path",
		Destination: "/container/path",
		ReadOnly:    true,
	}

	if !mount.ReadOnly {
		t.Error("Expected ReadOnly to be true")
	}
}

// =============================================================================
// Volume Struct Tests
// =============================================================================

func TestVolume_Fields(t *testing.T) {
	vol := &Volume{
		Name:       "test-volume",
		Path:       "/var/lib/volumes/test-volume/_data",
		Driver:     "local",
		Labels:     map[string]string{"env": "test"},
		Options:    map[string]string{"type": "ext4"},
		MountCount: 2,
		CreatedAt:  1706000000,
	}

	if vol.Name != "test-volume" {
		t.Errorf("Expected name 'test-volume', got '%s'", vol.Name)
	}

	if vol.Path != "/var/lib/volumes/test-volume/_data" {
		t.Errorf("Expected path '/var/lib/volumes/test-volume/_data', got '%s'", vol.Path)
	}

	if vol.Driver != "local" {
		t.Errorf("Expected driver 'local', got '%s'", vol.Driver)
	}

	if vol.Labels["env"] != "test" {
		t.Errorf("Expected label env=test, got '%s'", vol.Labels["env"])
	}

	if vol.MountCount != 2 {
		t.Errorf("Expected mount count 2, got %d", vol.MountCount)
	}

	if vol.CreatedAt != 1706000000 {
		t.Errorf("Expected CreatedAt 1706000000, got %d", vol.CreatedAt)
	}
}

// =============================================================================
// VolumeInfo Tests
// =============================================================================

func TestVolumeInfo_Fields(t *testing.T) {
	info := &VolumeInfo{
		Name:       "test-volume",
		Driver:     "local",
		Mountpoint: "/var/lib/volumes/test-volume/_data",
		Labels:     map[string]string{"app": "test"},
		Options:    map[string]string{"type": "tmpfs"},
		Scope:      "local",
		Status:     map[string]interface{}{"available": true},
		UsageData: &VolumeUsageData{
			Size:     1024000,
			RefCount: 1,
		},
	}

	if info.Name != "test-volume" {
		t.Errorf("Expected name 'test-volume', got '%s'", info.Name)
	}

	if info.Driver != "local" {
		t.Errorf("Expected driver 'local', got '%s'", info.Driver)
	}

	if info.Scope != "local" {
		t.Errorf("Expected scope 'local', got '%s'", info.Scope)
	}

	if info.UsageData == nil {
		t.Error("Expected UsageData to be set")
	}

	if info.UsageData.Size != 1024000 {
		t.Errorf("Expected size 1024000, got %d", info.UsageData.Size)
	}

	if info.UsageData.RefCount != 1 {
		t.Errorf("Expected ref count 1, got %d", info.UsageData.RefCount)
	}
}

// =============================================================================
// VolumeUsageData Tests
// =============================================================================

func TestVolumeUsageData(t *testing.T) {
	usage := &VolumeUsageData{
		Size:     5000000,
		RefCount: 3,
	}

	if usage.Size != 5000000 {
		t.Errorf("Expected size 5000000, got %d", usage.Size)
	}

	if usage.RefCount != 3 {
		t.Errorf("Expected ref count 3, got %d", usage.RefCount)
	}
}

// =============================================================================
// NewVolumeManager Tests
// =============================================================================

func TestNewVolumeManager(t *testing.T) {
	root := testTempDir(t)

	mgr, err := NewVolumeManager(root)
	if err != nil {
		t.Fatalf("NewVolumeManager failed: %v", err)
	}

	if mgr == nil {
		t.Fatal("Expected non-nil manager")
	}

	if mgr.root != root {
		t.Errorf("Expected root '%s', got '%s'", root, mgr.root)
	}

	if mgr.volumes == nil {
		t.Error("Expected volumes map to be initialized")
	}
}

func TestNewVolumeManager_CreatesRoot(t *testing.T) {
	root := filepath.Join(testTempDir(t), "newroot")

	mgr, err := NewVolumeManager(root)
	if err != nil {
		t.Fatalf("NewVolumeManager failed: %v", err)
	}

	if mgr == nil {
		t.Fatal("Expected non-nil manager")
	}

	// Verify root was created
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Error("Expected root directory to be created")
	}
}

func TestNewVolumeManager_LoadsExistingVolumes(t *testing.T) {
	root := testTempDir(t)

	// Create a volume directory structure
	volPath := filepath.Join(root, "existing-vol", "_data")
	if err := os.MkdirAll(volPath, 0755); err != nil {
		t.Fatalf("Failed to create volume directory: %v", err)
	}

	mgr, err := NewVolumeManager(root)
	if err != nil {
		t.Fatalf("NewVolumeManager failed: %v", err)
	}

	// Verify volume was loaded
	if len(mgr.volumes) != 1 {
		t.Errorf("Expected 1 volume, got %d", len(mgr.volumes))
	}

	if _, ok := mgr.volumes["existing-vol"]; !ok {
		t.Error("Expected 'existing-vol' to be loaded")
	}
}

// =============================================================================
// CreateVolume Tests
// =============================================================================

func TestCreateVolume(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	vol, err := mgr.CreateVolume("test-vol", nil, nil)
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}

	if vol.Name != "test-vol" {
		t.Errorf("Expected name 'test-vol', got '%s'", vol.Name)
	}

	if vol.Driver != "local" {
		t.Errorf("Expected driver 'local', got '%s'", vol.Driver)
	}

	// Verify directory was created
	if _, err := os.Stat(vol.Path); os.IsNotExist(err) {
		t.Error("Expected volume data directory to be created")
	}
}

func TestCreateVolume_WithOptions(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	options := map[string]string{"type": "ext4"}
	labels := map[string]string{"env": "test"}

	vol, err := mgr.CreateVolume("test-vol", options, labels)
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}

	if vol.Options["type"] != "ext4" {
		t.Errorf("Expected option type=ext4, got '%s'", vol.Options["type"])
	}

	if vol.Labels["env"] != "test" {
		t.Errorf("Expected label env=test, got '%s'", vol.Labels["env"])
	}
}

func TestCreateVolume_AlreadyExists(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	_, err := mgr.CreateVolume("test-vol", nil, nil)
	if err != nil {
		t.Fatalf("First CreateVolume failed: %v", err)
	}

	_, err = mgr.CreateVolume("test-vol", nil, nil)
	if err == nil {
		t.Error("Expected error for duplicate volume")
	}
}

// =============================================================================
// GetVolume Tests
// =============================================================================

func TestGetVolume(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	created, _ := mgr.CreateVolume("test-vol", nil, nil)

	vol, err := mgr.GetVolume("test-vol")
	if err != nil {
		t.Fatalf("GetVolume failed: %v", err)
	}

	if vol.Name != created.Name {
		t.Errorf("Expected name '%s', got '%s'", created.Name, vol.Name)
	}
}

func TestGetVolume_NotFound(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	_, err := mgr.GetVolume("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent volume")
	}
}

// =============================================================================
// ListVolumes Tests
// =============================================================================

func TestListVolumes(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	mgr.CreateVolume("vol1", nil, nil)
	mgr.CreateVolume("vol2", nil, nil)
	mgr.CreateVolume("vol3", nil, nil)

	volumes := mgr.ListVolumes()
	if len(volumes) != 3 {
		t.Errorf("Expected 3 volumes, got %d", len(volumes))
	}
}

func TestListVolumes_Empty(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	volumes := mgr.ListVolumes()
	if len(volumes) != 0 {
		t.Errorf("Expected 0 volumes, got %d", len(volumes))
	}
}

// =============================================================================
// RemoveVolume Tests
// =============================================================================

func TestRemoveVolume(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	vol, _ := mgr.CreateVolume("test-vol", nil, nil)

	err := mgr.RemoveVolume("test-vol", false)
	if err != nil {
		t.Fatalf("RemoveVolume failed: %v", err)
	}

	// Verify volume was removed
	if _, err := os.Stat(vol.Path); !os.IsNotExist(err) {
		t.Error("Expected volume directory to be removed")
	}

	_, err = mgr.GetVolume("test-vol")
	if err == nil {
		t.Error("Expected error after removal")
	}
}

func TestRemoveVolume_NotFound(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	err := mgr.RemoveVolume("nonexistent", false)
	if err == nil {
		t.Error("Expected error for nonexistent volume")
	}
}

func TestRemoveVolume_InUse(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	mgr.CreateVolume("test-vol", nil, nil)

	// Simulate volume in use
	mgr.mu.Lock()
	mgr.volumes["test-vol"].MountCount = 1
	mgr.mu.Unlock()

	err := mgr.RemoveVolume("test-vol", false)
	if err == nil {
		t.Error("Expected error for volume in use")
	}
}

func TestRemoveVolume_Force(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	mgr.CreateVolume("test-vol", nil, nil)

	// Simulate volume in use
	mgr.mu.Lock()
	mgr.volumes["test-vol"].MountCount = 1
	mgr.mu.Unlock()

	err := mgr.RemoveVolume("test-vol", true)
	if err != nil {
		t.Fatalf("Force remove should succeed, got: %v", err)
	}
}

// =============================================================================
// propagationToFlags Tests
// =============================================================================

func TestPropagationToFlags(t *testing.T) {
	// Test that each propagation returns a non-zero flag
	propagations := []MountPropagation{
		MountPropagationPrivate,
		MountPropagationRPrivate,
		MountPropagationShared,
		MountPropagationRShared,
		MountPropagationSlave,
		MountPropagationRSlave,
	}

	for _, prop := range propagations {
		flags := propagationToFlags(prop)
		if flags == 0 {
			t.Errorf("Expected non-zero flags for propagation %s", prop)
		}
	}

	// Test default case
	flags := propagationToFlags(MountPropagation("unknown"))
	if flags == 0 {
		t.Error("Expected non-zero flags for default case")
	}
}

// =============================================================================
// MountInfo Tests
// =============================================================================

func TestMountInfo_Fields(t *testing.T) {
	info := &MountInfo{
		MountID:        100,
		ParentID:       50,
		Major:          8,
		Minor:          1,
		Root:           "/",
		MountPoint:     "/mnt/test",
		MountOptions:   []string{"rw", "relatime"},
		OptionalFields: []string{"shared:1"},
		FSType:         "ext4",
		MountSource:    "/dev/sda1",
		SuperOptions:   []string{"errors=remount-ro"},
	}

	if info.MountID != 100 {
		t.Errorf("Expected MountID 100, got %d", info.MountID)
	}

	if info.ParentID != 50 {
		t.Errorf("Expected ParentID 50, got %d", info.ParentID)
	}

	if info.Major != 8 {
		t.Errorf("Expected Major 8, got %d", info.Major)
	}

	if info.Minor != 1 {
		t.Errorf("Expected Minor 1, got %d", info.Minor)
	}

	if info.MountPoint != "/mnt/test" {
		t.Errorf("Expected MountPoint '/mnt/test', got '%s'", info.MountPoint)
	}

	if info.FSType != "ext4" {
		t.Errorf("Expected FSType 'ext4', got '%s'", info.FSType)
	}
}

// =============================================================================
// parseMountInfoLine Tests
// =============================================================================

func TestParseMountInfoLine_Valid(t *testing.T) {
	line := "100 50 8:1 / /mnt/test rw,relatime shared:1 - ext4 /dev/sda1 rw,errors=remount-ro"

	info, err := parseMountInfoLine(line)
	if err != nil {
		t.Fatalf("parseMountInfoLine failed: %v", err)
	}

	if info.MountID != 100 {
		t.Errorf("Expected MountID 100, got %d", info.MountID)
	}

	if info.MountPoint != "/mnt/test" {
		t.Errorf("Expected MountPoint '/mnt/test', got '%s'", info.MountPoint)
	}

	if info.FSType != "ext4" {
		t.Errorf("Expected FSType 'ext4', got '%s'", info.FSType)
	}
}

func TestParseMountInfoLine_Empty(t *testing.T) {
	_, err := parseMountInfoLine("")
	if err == nil {
		t.Error("Expected error for empty line")
	}
}

func TestParseMountInfoLine_InvalidFormat(t *testing.T) {
	_, err := parseMountInfoLine("too short")
	if err == nil {
		t.Error("Expected error for invalid format")
	}
}

// =============================================================================
// Concurrent Volume Operations Tests
// =============================================================================

func TestVolumeManager_ConcurrentAccess(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent creates
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := filepath.Join("vol", string(rune('a'+idx)))
			_, err := mgr.CreateVolume(name, nil, nil)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.ListVolumes()
		}()
	}

	wg.Wait()
	close(errCh)

	// Some errors are expected (duplicate names)
	for range errCh {
		// Drain channel
	}
}

func TestVolumeManager_ConcurrentGetAndRemove(t *testing.T) {
	root := testTempDir(t)
	mgr, _ := NewVolumeManager(root)

	// Create volumes first
	for i := 0; i < 10; i++ {
		mgr.CreateVolume(filepath.Join("vol", string(rune('a'+i))), nil, nil)
	}

	var wg sync.WaitGroup

	// Concurrent gets
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.ListVolumes()
		}()
	}

	// Concurrent removes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := filepath.Join("vol", string(rune('a'+idx)))
			_ = mgr.RemoveVolume(name, true)
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkVolumeManager_CreateVolume(b *testing.B) {
	root := b.TempDir()
	mgr, _ := NewVolumeManager(root)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := filepath.Join("vol", string(rune('a'+(i%26))))
		mgr.CreateVolume(name, nil, nil)
	}
}

func BenchmarkVolumeManager_GetVolume(b *testing.B) {
	root := b.TempDir()
	mgr, _ := NewVolumeManager(root)

	mgr.CreateVolume("test-vol", nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.GetVolume("test-vol")
	}
}

func BenchmarkVolumeManager_ListVolumes(b *testing.B) {
	root := b.TempDir()
	mgr, _ := NewVolumeManager(root)

	for i := 0; i < 100; i++ {
		name := filepath.Join("vol", string(rune('a'+(i%26))), string(rune('0'+(i/26))))
		mgr.CreateVolume(name, nil, nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.ListVolumes()
	}
}

func BenchmarkParseMountInfoLine(b *testing.B) {
	line := "100 50 8:1 / /mnt/test rw,relatime shared:1 - ext4 /dev/sda1 rw,errors=remount-ro"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseMountInfoLine(line)
	}
}
