// SPDX-License-Identifier: BUSL-1.1
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
// NamespaceType Tests
// =============================================================================

func TestNamespaceTypes(t *testing.T) {
	tests := []struct {
		name     string
		value    NamespaceType
		expected string
	}{
		{"pid", PIDNamespace, "pid"},
		{"net", NetworkNamespace, "net"},
		{"mnt", MountNamespace, "mnt"},
		{"ipc", IPCNamespace, "ipc"},
		{"uts", UTSNamespace, "uts"},
		{"user", UserNamespace, "user"},
		{"cgroup", CgroupNamespace, "cgroup"},
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
// NamespaceSet Tests
// =============================================================================

func TestNamespaceSet_Fields(t *testing.T) {
	nsSet := &NamespaceSet{
		ID:   "container-123",
		PIDs: map[NamespaceType]int{PIDNamespace: 12345},
		Paths: map[NamespaceType]string{
			PIDNamespace:     "/run/ns/container-123/pid",
			NetworkNamespace: "/run/ns/container-123/net",
		},
	}

	if nsSet.ID != "container-123" {
		t.Errorf("Expected ID 'container-123', got '%s'", nsSet.ID)
	}

	if nsSet.PIDs[PIDNamespace] != 12345 {
		t.Errorf("Expected PID 12345, got %d", nsSet.PIDs[PIDNamespace])
	}

	if len(nsSet.Paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(nsSet.Paths))
	}

	if nsSet.Paths[PIDNamespace] != "/run/ns/container-123/pid" {
		t.Errorf("Expected path '/run/ns/container-123/pid', got '%s'", nsSet.Paths[PIDNamespace])
	}
}

// =============================================================================
// NamespaceInfo Tests
// =============================================================================

func TestNamespaceInfo_Fields(t *testing.T) {
	info := &NamespaceInfo{
		Type:  NetworkNamespace,
		Path:  "/run/ns/container-123/net",
		Inode: 4026531840,
	}

	if info.Type != NetworkNamespace {
		t.Errorf("Expected type net, got %s", info.Type)
	}

	if info.Path != "/run/ns/container-123/net" {
		t.Errorf("Expected path '/run/ns/container-123/net', got '%s'", info.Path)
	}

	if info.Inode != 4026531840 {
		t.Errorf("Expected inode 4026531840, got %d", info.Inode)
	}
}

// =============================================================================
// IDMapping Tests
// =============================================================================

func TestIDMapping_Fields(t *testing.T) {
	mapping := IDMapping{
		ContainerID: 0,
		HostID:      1000,
		Size:        65536,
	}

	if mapping.ContainerID != 0 {
		t.Errorf("Expected ContainerID 0, got %d", mapping.ContainerID)
	}

	if mapping.HostID != 1000 {
		t.Errorf("Expected HostID 1000, got %d", mapping.HostID)
	}

	if mapping.Size != 65536 {
		t.Errorf("Expected Size 65536, got %d", mapping.Size)
	}
}

func TestIDMapping_Multiple(t *testing.T) {
	mappings := []IDMapping{
		{ContainerID: 0, HostID: 1000, Size: 1},
		{ContainerID: 1, HostID: 100000, Size: 65535},
	}

	if len(mappings) != 2 {
		t.Errorf("Expected 2 mappings, got %d", len(mappings))
	}

	if mappings[0].HostID != 1000 {
		t.Errorf("Expected first mapping HostID 1000, got %d", mappings[0].HostID)
	}

	if mappings[1].Size != 65535 {
		t.Errorf("Expected second mapping Size 65535, got %d", mappings[1].Size)
	}
}

// =============================================================================
// NewNamespaceManager Tests
// =============================================================================

func TestNewNamespaceManager(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	mgr, err := NewNamespaceManager()
	if err != nil {
		t.Fatalf("NewNamespaceManager failed: %v", err)
	}

	if mgr == nil {
		t.Fatal("Expected non-nil manager")
	}

	if mgr.namespaces == nil {
		t.Error("Expected namespaces map to be initialized")
	}

	if mgr.nsBasePath == "" {
		t.Error("Expected nsBasePath to be set")
	}
}

// =============================================================================
// nsTypeToProc Tests
// =============================================================================

func TestNsTypeToProc(t *testing.T) {
	tests := []struct {
		nsType   NamespaceType
		expected string
	}{
		{NetworkNamespace, "net"},
		{MountNamespace, "mnt"},
		{PIDNamespace, "pid"},
		{IPCNamespace, "ipc"},
		{UTSNamespace, "uts"},
		{UserNamespace, "user"},
		{CgroupNamespace, "cgroup"},
	}

	for _, tt := range tests {
		t.Run(string(tt.nsType), func(t *testing.T) {
			result := nsTypeToProc(tt.nsType)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// nsTypeToCloneFlag Tests
// =============================================================================

func TestNsTypeToCloneFlag(t *testing.T) {
	tests := []struct {
		nsType       NamespaceType
		shouldBeZero bool
	}{
		{PIDNamespace, false},
		{NetworkNamespace, false},
		{MountNamespace, false},
		{IPCNamespace, false},
		{UTSNamespace, false},
		{UserNamespace, false},
		{CgroupNamespace, false},
		{NamespaceType("unknown"), true},
	}

	for _, tt := range tests {
		t.Run(string(tt.nsType), func(t *testing.T) {
			result := nsTypeToCloneFlag(tt.nsType)
			if tt.shouldBeZero && result != 0 {
				t.Errorf("Expected 0 for unknown type, got %d", result)
			}
			if !tt.shouldBeZero && result == 0 {
				t.Errorf("Expected non-zero for type %s, got 0", tt.nsType)
			}
		})
	}
}

// =============================================================================
// createNamespaceBindTarget Tests
// =============================================================================

func TestCreateNamespaceBindTarget(t *testing.T) {
	tmpDir := testTempDir(t)
	targetPath := filepath.Join(tmpDir, "ns-target")

	err := createNamespaceBindTarget(targetPath)
	if err != nil {
		t.Fatalf("createNamespaceBindTarget failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Error("Expected target file to exist")
	}
}

func TestCreateNamespaceBindTarget_AlreadyExists(t *testing.T) {
	tmpDir := testTempDir(t)
	targetPath := filepath.Join(tmpDir, "ns-target")

	// Create first time
	if err := createNamespaceBindTarget(targetPath); err != nil {
		t.Fatalf("First createNamespaceBindTarget failed: %v", err)
	}

	// Create second time should succeed (no error)
	err := createNamespaceBindTarget(targetPath)
	if err != nil {
		t.Errorf("Second createNamespaceBindTarget should not error, got: %v", err)
	}
}

// =============================================================================
// NamespaceManager GetNamespace Tests
// =============================================================================

func TestNamespaceManager_GetNamespace_NotFound(t *testing.T) {
	mgr := &NamespaceManager{
		nsBasePath: testTempDir(t),
		namespaces: make(map[string]*NamespaceSet),
	}

	_, err := mgr.GetNamespace("nonexistent", PIDNamespace)
	if err == nil {
		t.Error("Expected error for nonexistent namespace set")
	}
}

func TestNamespaceManager_GetNamespace_TypeNotFound(t *testing.T) {
	mgr := &NamespaceManager{
		nsBasePath: testTempDir(t),
		namespaces: map[string]*NamespaceSet{
			"test": {
				ID:    "test",
				Paths: map[NamespaceType]string{},
			},
		},
	}

	_, err := mgr.GetNamespace("test", PIDNamespace)
	if err == nil {
		t.Error("Expected error for nonexistent namespace type")
	}
}

func TestNamespaceManager_GetNamespace_Success(t *testing.T) {
	mgr := &NamespaceManager{
		nsBasePath: testTempDir(t),
		namespaces: map[string]*NamespaceSet{
			"test": {
				ID: "test",
				Paths: map[NamespaceType]string{
					PIDNamespace: "/run/ns/test/pid",
				},
			},
		},
	}

	path, err := mgr.GetNamespace("test", PIDNamespace)
	if err != nil {
		t.Fatalf("GetNamespace failed: %v", err)
	}

	if path != "/run/ns/test/pid" {
		t.Errorf("Expected path '/run/ns/test/pid', got '%s'", path)
	}
}

// =============================================================================
// NamespaceManager RemoveNamespaces Tests
// =============================================================================

func TestNamespaceManager_RemoveNamespaces_NotFound(t *testing.T) {
	mgr := &NamespaceManager{
		nsBasePath: testTempDir(t),
		namespaces: make(map[string]*NamespaceSet),
	}

	// Should not error for nonexistent
	err := mgr.RemoveNamespaces("nonexistent")
	if err != nil {
		t.Errorf("RemoveNamespaces should not error for nonexistent: %v", err)
	}
}

func TestNamespaceManager_RemoveNamespaces_Success(t *testing.T) {
	tmpDir := testTempDir(t)
	nsDir := filepath.Join(tmpDir, "test")
	os.MkdirAll(nsDir, 0755)

	pidPath := filepath.Join(nsDir, "pid")
	os.WriteFile(pidPath, []byte{}, 0644)

	mgr := &NamespaceManager{
		nsBasePath: tmpDir,
		namespaces: map[string]*NamespaceSet{
			"test": {
				ID: "test",
				Paths: map[NamespaceType]string{
					PIDNamespace: pidPath,
				},
			},
		},
	}

	err := mgr.RemoveNamespaces("test")
	if err != nil {
		t.Fatalf("RemoveNamespaces failed: %v", err)
	}

	// Verify removed from map
	if _, ok := mgr.namespaces["test"]; ok {
		t.Error("Expected namespace set to be removed from map")
	}
}

// =============================================================================
// NamespaceManager GetNamespaceInfo Tests
// =============================================================================

func TestNamespaceManager_GetNamespaceInfo_FileNotFound(t *testing.T) {
	mgr := &NamespaceManager{
		nsBasePath: testTempDir(t),
		namespaces: make(map[string]*NamespaceSet),
	}

	_, err := mgr.GetNamespaceInfo("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestNamespaceManager_GetNamespaceInfo_Success(t *testing.T) {
	tmpDir := testTempDir(t)
	nsPath := filepath.Join(tmpDir, "pid")
	os.WriteFile(nsPath, []byte{}, 0644)

	mgr := &NamespaceManager{
		nsBasePath: tmpDir,
		namespaces: make(map[string]*NamespaceSet),
	}

	info, err := mgr.GetNamespaceInfo(nsPath)
	if err != nil {
		t.Fatalf("GetNamespaceInfo failed: %v", err)
	}

	if info.Path != nsPath {
		t.Errorf("Expected path '%s', got '%s'", nsPath, info.Path)
	}

	if info.Inode == 0 {
		t.Error("Expected non-zero inode")
	}
}

// =============================================================================
// NamespaceManager JoinNamespace Tests
// =============================================================================

func TestNamespaceManager_JoinNamespace(t *testing.T) {
	mgr := &NamespaceManager{
		nsBasePath: testTempDir(t),
		namespaces: make(map[string]*NamespaceSet),
	}

	// JoinNamespace is a no-op that always returns nil
	err := mgr.JoinNamespace(12345, PIDNamespace, "/run/ns/test/pid")
	if err != nil {
		t.Errorf("JoinNamespace should not error: %v", err)
	}
}

// =============================================================================
// ifreqFlags Tests
// =============================================================================

func TestIfreqFlags_Fields(t *testing.T) {
	ifreq := &ifreqFlags{
		name:  [16]byte{'l', 'o'},
		flags: 0x1043,
	}

	if ifreq.name[0] != 'l' || ifreq.name[1] != 'o' {
		t.Error("Expected name 'lo'")
	}

	if ifreq.flags != 0x1043 {
		t.Errorf("Expected flags 0x1043, got 0x%x", ifreq.flags)
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestNamespaceManager_ConcurrentAccess(t *testing.T) {
	mgr := &NamespaceManager{
		nsBasePath: testTempDir(t),
		namespaces: make(map[string]*NamespaceSet),
	}

	// Add some namespaces
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		mgr.namespaces[id] = &NamespaceSet{
			ID: id,
			Paths: map[NamespaceType]string{
				PIDNamespace: "/run/ns/" + id + "/pid",
			},
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := string(rune('a' + (idx % 10)))
			_, err := mgr.GetNamespace(id, PIDNamespace)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	// Concurrent removes (some will fail - expected)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := string(rune('a' + idx))
			_ = mgr.RemoveNamespaces(id)
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Some errors are expected due to concurrent removes
	for range errCh {
		// Drain channel
	}
}

func TestNamespaceManager_ConcurrentGetNamespaceInfo(t *testing.T) {
	tmpDir := testTempDir(t)

	// Create test files
	for i := 0; i < 10; i++ {
		path := filepath.Join(tmpDir, string(rune('a'+i)))
		os.WriteFile(path, []byte{}, 0644)
	}

	mgr := &NamespaceManager{
		nsBasePath: tmpDir,
		namespaces: make(map[string]*NamespaceSet),
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			path := filepath.Join(tmpDir, string(rune('a'+(idx%10))))
			_, err := mgr.GetNamespaceInfo(path)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("GetNamespaceInfo error: %v", err)
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestNamespaceSet_EmptyMaps(t *testing.T) {
	nsSet := &NamespaceSet{
		ID:    "empty",
		PIDs:  make(map[NamespaceType]int),
		Paths: make(map[NamespaceType]string),
	}

	if len(nsSet.PIDs) != 0 {
		t.Errorf("Expected 0 PIDs, got %d", len(nsSet.PIDs))
	}

	if len(nsSet.Paths) != 0 {
		t.Errorf("Expected 0 paths, got %d", len(nsSet.Paths))
	}
}

func TestIDMapping_ZeroValues(t *testing.T) {
	mapping := IDMapping{}

	if mapping.ContainerID != 0 {
		t.Errorf("Expected ContainerID 0, got %d", mapping.ContainerID)
	}

	if mapping.HostID != 0 {
		t.Errorf("Expected HostID 0, got %d", mapping.HostID)
	}

	if mapping.Size != 0 {
		t.Errorf("Expected Size 0, got %d", mapping.Size)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkNsTypeToProc(b *testing.B) {
	types := []NamespaceType{
		PIDNamespace,
		NetworkNamespace,
		MountNamespace,
		IPCNamespace,
		UTSNamespace,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nsTypeToProc(types[i%len(types)])
	}
}

func BenchmarkNsTypeToCloneFlag(b *testing.B) {
	types := []NamespaceType{
		PIDNamespace,
		NetworkNamespace,
		MountNamespace,
		IPCNamespace,
		UTSNamespace,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nsTypeToCloneFlag(types[i%len(types)])
	}
}

func BenchmarkNamespaceManager_GetNamespace(b *testing.B) {
	mgr := &NamespaceManager{
		nsBasePath: "/tmp",
		namespaces: map[string]*NamespaceSet{
			"test": {
				ID: "test",
				Paths: map[NamespaceType]string{
					PIDNamespace:     "/run/ns/test/pid",
					NetworkNamespace: "/run/ns/test/net",
					MountNamespace:   "/run/ns/test/mnt",
				},
			},
		},
	}

	types := []NamespaceType{PIDNamespace, NetworkNamespace, MountNamespace}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.GetNamespace("test", types[i%len(types)])
	}
}

func BenchmarkCreateNamespaceBindTarget(b *testing.B) {
	tmpDir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := filepath.Join(tmpDir, string(rune('a'+(i%26))))
		createNamespaceBindTarget(path)
	}
}

func BenchmarkNamespaceSet_Creation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = &NamespaceSet{
			ID:   "test",
			PIDs: make(map[NamespaceType]int),
			Paths: map[NamespaceType]string{
				PIDNamespace:     "/run/ns/test/pid",
				NetworkNamespace: "/run/ns/test/net",
				MountNamespace:   "/run/ns/test/mnt",
				IPCNamespace:     "/run/ns/test/ipc",
				UTSNamespace:     "/run/ns/test/uts",
			},
		}
	}
}
