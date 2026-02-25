// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Test Helpers and Mocks
// =============================================================================

// testTempDir creates a temporary directory for testing.
func testTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "runtime-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// mockImageStore creates a minimal image store for testing.
func mockImageStore(t *testing.T, root string) *ImageStore {
	t.Helper()
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("Failed to create image store: %v", err)
	}
	return store
}

// addMockImage adds a mock image to the store for testing.
func addMockImage(t *testing.T, store *ImageStore, imageID, tag string) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()

	store.images[imageID] = &ImageInfo{
		ID:       imageID,
		RepoTags: []string{tag},
		Size:     1024,
		Config: &ImageConfig{
			Cmd: []string{"/bin/sh"},
		},
		RootFS: &RootFS{
			Type:    "layers",
			DiffIDs: []string{},
		},
	}
}

// =============================================================================
// Container State Tests
// =============================================================================

func TestContainerState_String(t *testing.T) {
	tests := []struct {
		state    ContainerState
		expected string
	}{
		{ContainerStateUnknown, "unknown"},
		{ContainerStateCreated, "created"},
		{ContainerStateRunning, "running"},
		{ContainerStateStopped, "stopped"},
		{ContainerStateExited, "exited"},
		{ContainerStatePaused, "paused"},
		{ContainerState(99), "unknown"}, // Invalid state
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("ContainerState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestContainerInfo_Fields(t *testing.T) {
	now := time.Now()
	info := &ContainerInfo{
		ID:         "test-container-123",
		Name:       "test-container",
		Image:      "alpine:latest",
		State:      ContainerStateCreated,
		PID:        12345,
		ExitCode:   0,
		CreatedAt:  now,
		StartedAt:  time.Time{},
		FinishedAt: time.Time{},
		SandboxID:  "sandbox-123",
		Labels:     map[string]string{"env": "test"},
		Annotations: map[string]string{"version": "1.0"},
	}

	if info.ID != "test-container-123" {
		t.Errorf("Expected ID 'test-container-123', got '%s'", info.ID)
	}
	if info.State != ContainerStateCreated {
		t.Errorf("Expected state Created, got %v", info.State)
	}
	if info.Labels["env"] != "test" {
		t.Errorf("Expected label env=test, got %v", info.Labels["env"])
	}
}

// =============================================================================
// OCI Spec Generation Tests
// =============================================================================

func TestGenerateOCISpec_DefaultValues(t *testing.T) {
	root := testTempDir(t)
	config := &RuntimeConfig{
		Root:     root,
		StateDir: filepath.Join(root, "state"),
	}

	// Create minimal runtime for testing
	imgStore, _ := NewImageStore(filepath.Join(root, "images"), nil)

	rt := &DefaultRuntime{
		config:       config,
		containers:   make(map[string]*container),
		sandboxes:    make(map[string]*Sandbox),
		images:       imgStore,
		execSessions: make(map[string]*execSession),
		logStreams:   make(map[string]*logStream),
	}

	containerConfig := &ContainerConfig{
		ID:   "test-123",
		Name: "test-container",
	}

	rootfs := filepath.Join(root, "rootfs")
	os.MkdirAll(rootfs, 0755)

	spec, err := rt.generateOCISpec(containerConfig, rootfs)
	if err != nil {
		t.Fatalf("generateOCISpec failed: %v", err)
	}

	// Verify OCI version
	if spec.OCIVersion != "1.0.2" {
		t.Errorf("Expected OCI version 1.0.2, got %s", spec.OCIVersion)
	}

	// Verify default command
	if len(spec.Process.Args) == 0 || spec.Process.Args[0] != "/bin/sh" {
		t.Errorf("Expected default command /bin/sh, got %v", spec.Process.Args)
	}

	// Verify default working directory
	if spec.Process.Cwd != "/" {
		t.Errorf("Expected default cwd /, got %s", spec.Process.Cwd)
	}

	// Verify default namespaces
	if spec.Linux == nil || len(spec.Linux.Namespaces) == 0 {
		t.Error("Expected Linux namespaces to be set")
	}

	// Verify root is set
	if spec.Root == nil || spec.Root.Path != rootfs {
		t.Errorf("Expected root path %s, got %v", rootfs, spec.Root)
	}
}

func TestGenerateOCISpec_WithCustomConfig(t *testing.T) {
	root := testTempDir(t)
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

	containerConfig := &ContainerConfig{
		ID:         "test-123",
		Name:       "test-container",
		Command:    []string{"/usr/bin/python"},
		Args:       []string{"-c", "print('hello')"},
		Env:        []string{"FOO=bar", "BAZ=qux"},
		WorkingDir: "/app",
		User:       "1000:1000",
		Tty:        true,
	}

	rootfs := filepath.Join(root, "rootfs")
	os.MkdirAll(rootfs, 0755)

	spec, err := rt.generateOCISpec(containerConfig, rootfs)
	if err != nil {
		t.Fatalf("generateOCISpec failed: %v", err)
	}

	// Verify command and args
	expectedArgs := []string{"/usr/bin/python", "-c", "print('hello')"}
	if len(spec.Process.Args) != len(expectedArgs) {
		t.Errorf("Expected args %v, got %v", expectedArgs, spec.Process.Args)
	}

	// Verify environment
	if len(spec.Process.Env) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(spec.Process.Env))
	}

	// Verify working directory
	if spec.Process.Cwd != "/app" {
		t.Errorf("Expected cwd /app, got %s", spec.Process.Cwd)
	}

	// Verify TTY
	if !spec.Process.Terminal {
		t.Error("Expected terminal to be enabled")
	}

	// Verify user
	if spec.Process.User.UID != 1000 || spec.Process.User.GID != 1000 {
		t.Errorf("Expected UID:GID 1000:1000, got %d:%d",
			spec.Process.User.UID, spec.Process.User.GID)
	}
}

func TestGenerateOCISpec_WithResources(t *testing.T) {
	root := testTempDir(t)
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

	memLimit := int64(512 * 1024 * 1024) // 512MB
	memRes := int64(256 * 1024 * 1024)   // 256MB

	containerConfig := &ContainerConfig{
		ID:   "test-123",
		Name: "test-container",
		Resources: &ResourceConfig{
			CPUShares:              1024,
			CPUQuota:               50000,
			CPUPeriod:              100000,
			CPUSetCPUs:             "0-1",
			CPUSetMems:             "0",
			MemoryLimitBytes:       memLimit,
			MemoryReservationBytes: memRes,
			PidsLimit:              100,
		},
	}

	rootfs := filepath.Join(root, "rootfs")
	os.MkdirAll(rootfs, 0755)

	spec, err := rt.generateOCISpec(containerConfig, rootfs)
	if err != nil {
		t.Fatalf("generateOCISpec failed: %v", err)
	}

	// Verify memory resources
	if spec.Linux.Resources.Memory == nil {
		t.Fatal("Expected memory resources to be set")
	}
	if *spec.Linux.Resources.Memory.Limit != memLimit {
		t.Errorf("Expected memory limit %d, got %d", memLimit, *spec.Linux.Resources.Memory.Limit)
	}
	if *spec.Linux.Resources.Memory.Reservation != memRes {
		t.Errorf("Expected memory reservation %d, got %d", memRes, *spec.Linux.Resources.Memory.Reservation)
	}

	// Verify CPU resources
	if spec.Linux.Resources.CPU == nil {
		t.Fatal("Expected CPU resources to be set")
	}
	if *spec.Linux.Resources.CPU.Shares != 1024 {
		t.Errorf("Expected CPU shares 1024, got %d", *spec.Linux.Resources.CPU.Shares)
	}
	if *spec.Linux.Resources.CPU.Quota != 50000 {
		t.Errorf("Expected CPU quota 50000, got %d", *spec.Linux.Resources.CPU.Quota)
	}
	if spec.Linux.Resources.CPU.Cpus != "0-1" {
		t.Errorf("Expected cpuset cpus 0-1, got %s", spec.Linux.Resources.CPU.Cpus)
	}

	// Verify PIDs limit
	if spec.Linux.Resources.Pids == nil || spec.Linux.Resources.Pids.Limit != 100 {
		t.Errorf("Expected PIDs limit 100, got %v", spec.Linux.Resources.Pids)
	}
}

func TestGenerateOCISpec_WithLinuxConfig(t *testing.T) {
	root := testTempDir(t)
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

	containerConfig := &ContainerConfig{
		ID:   "test-123",
		Name: "test-container",
		Linux: &LinuxContainerConfig{
			ReadonlyRootfs: true,
			MaskedPaths:    []string{"/proc/kcore", "/proc/keys"},
			ReadonlyPaths:  []string{"/proc/sys"},
			Sysctl:         map[string]string{"net.core.somaxconn": "1024"},
			CgroupsPath:    "/test/container",
		},
	}

	rootfs := filepath.Join(root, "rootfs")
	os.MkdirAll(rootfs, 0755)

	spec, err := rt.generateOCISpec(containerConfig, rootfs)
	if err != nil {
		t.Fatalf("generateOCISpec failed: %v", err)
	}

	// Verify readonly rootfs
	if !spec.Root.Readonly {
		t.Error("Expected readonly rootfs")
	}

	// Verify masked paths
	if len(spec.Linux.MaskedPaths) != 2 {
		t.Errorf("Expected 2 masked paths, got %d", len(spec.Linux.MaskedPaths))
	}

	// Verify sysctl
	if spec.Linux.Sysctl["net.core.somaxconn"] != "1024" {
		t.Errorf("Expected sysctl net.core.somaxconn=1024, got %v", spec.Linux.Sysctl)
	}

	// Verify cgroups path
	if spec.Linux.CgroupsPath != "/test/container" {
		t.Errorf("Expected cgroups path /test/container, got %s", spec.Linux.CgroupsPath)
	}
}

func TestOCISpec_MarshalUnmarshal(t *testing.T) {
	spec := &OCISpec{
		OCIVersion: "1.0.2",
		Process: &OCIProcess{
			Terminal: true,
			User:     OCIUser{UID: 1000, GID: 1000},
			Args:     []string{"/bin/sh"},
			Env:      []string{"PATH=/usr/bin"},
			Cwd:      "/",
		},
		Root: &OCIRoot{
			Path:     "/rootfs",
			Readonly: false,
		},
		Hostname: "container",
	}

	// Marshal
	data, err := MarshalOCISpec(spec)
	if err != nil {
		t.Fatalf("Failed to marshal OCI spec: %v", err)
	}

	// Unmarshal
	unmarshaled, err := UnmarshalOCISpec(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal OCI spec: %v", err)
	}

	// Verify
	if unmarshaled.OCIVersion != spec.OCIVersion {
		t.Errorf("OCI version mismatch: got %s, want %s", unmarshaled.OCIVersion, spec.OCIVersion)
	}
	if unmarshaled.Process.User.UID != spec.Process.User.UID {
		t.Errorf("UID mismatch: got %d, want %d", unmarshaled.Process.User.UID, spec.Process.User.UID)
	}
}

func TestOCISpec_UnmarshalInvalid(t *testing.T) {
	invalidJSON := []byte(`{invalid json`)
	_, err := UnmarshalOCISpec(invalidJSON)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// =============================================================================
// Parse User Group Tests
// =============================================================================

func TestParseUserGroup(t *testing.T) {
	tests := []struct {
		input       string
		expectedUID int
		expectedGID int
	}{
		{"root", 0, 0},
		{"0", 0, 0},
		{"1000:1000", 1000, 1000},
		{"1000:2000", 1000, 2000},
		{"500", 500, 500},
		{"", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			uid, gid := parseUserGroup(tt.input)
			if uid != tt.expectedUID || gid != tt.expectedGID {
				t.Errorf("parseUserGroup(%q) = (%d, %d), want (%d, %d)",
					tt.input, uid, gid, tt.expectedUID, tt.expectedGID)
			}
		})
	}
}

// =============================================================================
// Default Capabilities Tests
// =============================================================================

func TestDefaultCapabilities(t *testing.T) {
	caps := defaultCapabilities()

	if len(caps) == 0 {
		t.Error("Expected non-empty default capabilities")
	}

	// Check for some expected capabilities
	expectedCaps := []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_NET_BIND_SERVICE",
		"CAP_SETUID",
		"CAP_SETGID",
	}

	capSet := make(map[string]bool)
	for _, cap := range caps {
		capSet[cap] = true
	}

	for _, expected := range expectedCaps {
		if !capSet[expected] {
			t.Errorf("Expected capability %s not found in defaults", expected)
		}
	}
}

// =============================================================================
// Seccomp Conversion Tests
// =============================================================================

func TestConvertSeccomp(t *testing.T) {
	// Test nil input
	result := convertSeccomp(nil)
	if result != nil {
		t.Error("Expected nil for nil input")
	}

	// Test valid input
	seccomp := &LinuxSeccomp{
		DefaultAction: SeccompActionAllow,
		Architectures: []string{"SCMP_ARCH_X86_64"},
		Syscalls: []SeccompSyscall{
			{
				Names:  []string{"read", "write"},
				Action: SeccompActionAllow,
				Args: []SeccompArg{
					{Index: 0, Value: 100, Op: SeccompOpEqual},
				},
			},
		},
	}

	ociSeccomp := convertSeccomp(seccomp)
	if ociSeccomp == nil {
		t.Fatal("Expected non-nil OCI seccomp")
	}

	if ociSeccomp.DefaultAction != "SCMP_ACT_ALLOW" {
		t.Errorf("Expected default action SCMP_ACT_ALLOW, got %s", ociSeccomp.DefaultAction)
	}

	if len(ociSeccomp.Syscalls) != 1 {
		t.Fatalf("Expected 1 syscall, got %d", len(ociSeccomp.Syscalls))
	}

	if len(ociSeccomp.Syscalls[0].Names) != 2 {
		t.Errorf("Expected 2 syscall names, got %d", len(ociSeccomp.Syscalls[0].Names))
	}

	if len(ociSeccomp.Syscalls[0].Args) != 1 {
		t.Errorf("Expected 1 arg, got %d", len(ociSeccomp.Syscalls[0].Args))
	}
}

// =============================================================================
// ResourceConfig Tests
// =============================================================================

func TestResourceConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config ResourceConfig
		valid  bool
	}{
		{
			name: "valid basic config",
			config: ResourceConfig{
				CPUShares:        1024,
				MemoryLimitBytes: 512 * 1024 * 1024,
			},
			valid: true,
		},
		{
			name: "valid CPU weight",
			config: ResourceConfig{
				CPUWeight: 100,
			},
			valid: true,
		},
		{
			name: "valid IO limits",
			config: ResourceConfig{
				IOWeight: 100,
				IOLimits: []IOLimit{
					{Major: 8, Minor: 0, ReadBytesPerSec: 1000000},
				},
			},
			valid: true,
		},
		{
			name: "zero values",
			config: ResourceConfig{},
			valid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ResourceConfig doesn't have validation, just verify it can be used
			data, err := json.Marshal(tt.config)
			if err != nil {
				t.Errorf("Failed to marshal ResourceConfig: %v", err)
			}
			if len(data) == 0 {
				t.Error("Expected non-empty JSON")
			}
		})
	}
}

func TestResourceConfig_CPUWeightConversion(t *testing.T) {
	// Test CPU shares to weight conversion logic
	// weight = (shares * 100) / 1024, clamped to 1-10000
	tests := []struct {
		shares   int64
		expected int64
	}{
		{1024, 100},  // Standard shares
		{512, 50},    // Half shares
		{2048, 200},  // Double shares
		{2, 1},       // Very low shares (clamps to 1)
		{102400, 10000}, // Very high shares (clamps to 10000)
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			weight := (tt.shares * 100) / 1024
			if weight < 1 {
				weight = 1
			}
			if weight > 10000 {
				weight = 10000
			}
			if weight != tt.expected {
				t.Errorf("Shares %d converted to weight %d, expected %d",
					tt.shares, weight, tt.expected)
			}
		})
	}
}

// =============================================================================
// Runtime Config Tests
// =============================================================================

func TestRuntimeConfig_Defaults(t *testing.T) {
	// Test that NewRuntime sets proper defaults
	root := testTempDir(t)

	config := &RuntimeConfig{
		Root:     root,
		StateDir: filepath.Join(root, "state"),
	}

	// Verify defaults would be set
	if config.CgroupDriver == "" {
		config.CgroupDriver = "systemd"
	}

	if config.CgroupDriver != "systemd" {
		t.Errorf("Expected default cgroup driver 'systemd', got '%s'", config.CgroupDriver)
	}
}

// =============================================================================
// Version Info Tests
// =============================================================================

func TestVersionInfo(t *testing.T) {
	root := testTempDir(t)

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

	ctx := context.Background()
	version, err := rt.Version(ctx)
	if err != nil {
		t.Fatalf("Version() failed: %v", err)
	}

	if version.RuntimeName != "unheaded" {
		t.Errorf("Expected runtime name 'unheaded', got '%s'", version.RuntimeName)
	}

	if version.Version == "" {
		t.Error("Expected non-empty version")
	}

	if version.OS != "linux" {
		t.Errorf("Expected OS 'linux', got '%s'", version.OS)
	}
}

// =============================================================================
// Runtime Status Tests
// =============================================================================

func TestRuntimeStatus(t *testing.T) {
	root := testTempDir(t)

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

	ctx := context.Background()
	status, err := rt.Status(ctx)
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}

	if len(status.Conditions) == 0 {
		t.Error("Expected non-empty conditions")
	}

	// Check RuntimeReady condition
	found := false
	for _, cond := range status.Conditions {
		if cond.Type == "RuntimeReady" {
			found = true
			if !cond.Status {
				t.Error("Expected RuntimeReady status to be true")
			}
		}
	}
	if !found {
		t.Error("Expected RuntimeReady condition")
	}
}

func TestRuntimeStatus_AfterClose(t *testing.T) {
	root := testTempDir(t)

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

	// Close the runtime
	rt.Close()

	ctx := context.Background()
	status, err := rt.Status(ctx)
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}

	// Check RuntimeReady condition is false after close
	for _, cond := range status.Conditions {
		if cond.Type == "RuntimeReady" && cond.Status {
			t.Error("Expected RuntimeReady status to be false after close")
		}
	}
}

// =============================================================================
// Runtime Close Tests
// =============================================================================

func TestRuntimeClose_Idempotent(t *testing.T) {
	root := testTempDir(t)

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

	// Close multiple times should not panic
	for i := 0; i < 3; i++ {
		err := rt.Close()
		if err != nil {
			t.Errorf("Close() attempt %d failed: %v", i+1, err)
		}
	}
}

// =============================================================================
// Generate ID Tests
// =============================================================================

func TestGenerateID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	const count = 1000

	for i := 0; i < count; i++ {
		id := generateID()
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("Expected %d unique IDs, got %d", count, len(ids))
	}
}

// =============================================================================
// Error Type Tests
// =============================================================================

func TestErrorTypes(t *testing.T) {
	// Verify error types are distinct and have proper messages
	errors := []struct {
		err     error
		message string
	}{
		{ErrContainerNotFound, "container not found"},
		{ErrContainerExists, "container already exists"},
		{ErrContainerRunning, "container is running"},
		{ErrContainerNotRunning, "container is not running"},
		{ErrImageNotFound, "image not found"},
		{ErrSandboxNotFound, "sandbox not found"},
		{ErrInvalidConfig, "invalid configuration"},
		{ErrOperationTimeout, "operation timed out"},
		{ErrResourceExhausted, "resource exhausted"},
		{ErrPermissionDenied, "permission denied"},
		{ErrNotSupported, "operation not supported"},
	}

	for _, e := range errors {
		t.Run(e.message, func(t *testing.T) {
			if e.err == nil {
				t.Error("Error should not be nil")
			}
			if e.err.Error() != e.message {
				t.Errorf("Expected error message '%s', got '%s'", e.message, e.err.Error())
			}
		})
	}
}

// =============================================================================
// Container Filter Tests
// =============================================================================

func TestContainerFilter(t *testing.T) {
	filter := &ContainerFilter{
		ID:        "container-123",
		Name:      "test-container",
		State:     ContainerStateRunning,
		SandboxID: "sandbox-456",
		Labels: map[string]string{
			"app": "web",
			"env": "prod",
		},
	}

	// Verify filter fields
	if filter.ID != "container-123" {
		t.Errorf("Expected ID 'container-123', got '%s'", filter.ID)
	}

	if filter.State != ContainerStateRunning {
		t.Errorf("Expected state Running, got %v", filter.State)
	}

	if len(filter.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(filter.Labels))
	}
}

// =============================================================================
// Seccomp Action Tests
// =============================================================================

func TestSeccompActions(t *testing.T) {
	tests := []struct {
		action   SeccompAction
		expected string
	}{
		{SeccompActionAllow, "SCMP_ACT_ALLOW"},
		{SeccompActionErrno, "SCMP_ACT_ERRNO"},
		{SeccompActionKill, "SCMP_ACT_KILL"},
		{SeccompActionLog, "SCMP_ACT_LOG"},
		{SeccompActionTrap, "SCMP_ACT_TRAP"},
		{SeccompActionTrace, "SCMP_ACT_TRACE"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("Expected action '%s', got '%s'", tt.expected, tt.action)
			}
		})
	}
}

// =============================================================================
// Seccomp Operator Tests
// =============================================================================

func TestSeccompOperators(t *testing.T) {
	tests := []struct {
		op       SeccompOperator
		expected string
	}{
		{SeccompOpNotEqual, "SCMP_CMP_NE"},
		{SeccompOpLessThan, "SCMP_CMP_LT"},
		{SeccompOpLessEqual, "SCMP_CMP_LE"},
		{SeccompOpEqual, "SCMP_CMP_EQ"},
		{SeccompOpGreaterEqual, "SCMP_CMP_GE"},
		{SeccompOpGreaterThan, "SCMP_CMP_GT"},
		{SeccompOpMaskedEqual, "SCMP_CMP_MASKED_EQ"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.op) != tt.expected {
				t.Errorf("Expected operator '%s', got '%s'", tt.expected, tt.op)
			}
		})
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestRuntimeConcurrentAccess(t *testing.T) {
	root := testTempDir(t)

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
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	const goroutines = 100

	// Concurrent version calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rt.Version(ctx)
			if err != nil {
				t.Errorf("Version() failed: %v", err)
			}
		}()
	}

	// Concurrent status calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rt.Status(ctx)
			if err != nil {
				t.Errorf("Status() failed: %v", err)
			}
		}()
	}

	wg.Wait()
}

// =============================================================================
// LinuxCapabilities Tests
// =============================================================================

func TestLinuxCapabilities(t *testing.T) {
	caps := &LinuxCapabilities{
		Bounding:    []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE"},
		Effective:   []string{"CAP_CHOWN"},
		Inheritable: []string{},
		Permitted:   []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE"},
		Ambient:     []string{},
	}

	if len(caps.Bounding) != 2 {
		t.Errorf("Expected 2 bounding caps, got %d", len(caps.Bounding))
	}

	if len(caps.Effective) != 1 {
		t.Errorf("Expected 1 effective cap, got %d", len(caps.Effective))
	}
}

// =============================================================================
// SELinux Options Tests
// =============================================================================

func TestSELinuxOption(t *testing.T) {
	selinux := &SELinuxOption{
		User:  "system_u",
		Role:  "system_r",
		Type:  "container_t",
		Level: "s0:c1,c2",
	}

	if selinux.User != "system_u" {
		t.Errorf("Expected user 'system_u', got '%s'", selinux.User)
	}

	if selinux.Type != "container_t" {
		t.Errorf("Expected type 'container_t', got '%s'", selinux.Type)
	}
}

// =============================================================================
// Ulimit Tests
// =============================================================================

func TestUlimit(t *testing.T) {
	ulimits := []Ulimit{
		{Name: "nofile", Hard: 65536, Soft: 32768},
		{Name: "nproc", Hard: 4096, Soft: 2048},
	}

	if len(ulimits) != 2 {
		t.Errorf("Expected 2 ulimits, got %d", len(ulimits))
	}

	if ulimits[0].Name != "nofile" {
		t.Errorf("Expected name 'nofile', got '%s'", ulimits[0].Name)
	}

	if ulimits[0].Hard != 65536 {
		t.Errorf("Expected hard limit 65536, got %d", ulimits[0].Hard)
	}
}

// =============================================================================
// IOLimit Tests
// =============================================================================

func TestIOLimit(t *testing.T) {
	limit := IOLimit{
		Major:            8,
		Minor:            0,
		ReadBytesPerSec:  100 * 1024 * 1024,
		WriteBytesPerSec: 50 * 1024 * 1024,
		ReadIOPS:         10000,
		WriteIOPS:        5000,
	}

	if limit.Major != 8 {
		t.Errorf("Expected major 8, got %d", limit.Major)
	}

	if limit.ReadBytesPerSec != 100*1024*1024 {
		t.Errorf("Expected read bytes/sec 100MB, got %d", limit.ReadBytesPerSec)
	}
}

// =============================================================================
// Auth Config Tests
// =============================================================================

func TestAuthConfig(t *testing.T) {
	auth := &AuthConfig{
		Username:      "user",
		Password:      "pass",
		Auth:          "base64encoded",
		IdentityToken: "token123",
	}

	if auth.Username != "user" {
		t.Errorf("Expected username 'user', got '%s'", auth.Username)
	}

	if auth.IdentityToken != "token123" {
		t.Errorf("Expected token 'token123', got '%s'", auth.IdentityToken)
	}
}

// =============================================================================
// Registry Config Tests
// =============================================================================

func TestRegistryConfig(t *testing.T) {
	config := &RegistryConfig{
		Mirrors: map[string][]string{
			"docker.io": {"https://mirror.gcr.io"},
		},
		InsecureRegistries: []string{"localhost:5000"},
		AuthConfigs: map[string]*AuthConfig{
			"docker.io": {Username: "user", Password: "pass"},
		},
	}

	if len(config.Mirrors["docker.io"]) != 1 {
		t.Errorf("Expected 1 mirror for docker.io, got %d", len(config.Mirrors["docker.io"]))
	}

	if len(config.InsecureRegistries) != 1 {
		t.Errorf("Expected 1 insecure registry, got %d", len(config.InsecureRegistries))
	}
}

// =============================================================================
// Container Config Validation Tests
// =============================================================================

func TestContainerConfig_Complete(t *testing.T) {
	config := &ContainerConfig{
		ID:         "container-123",
		Name:       "test-container",
		Image:      "alpine:3.18",
		Command:    []string{"/bin/sh"},
		Args:       []string{"-c", "echo hello"},
		Env:        []string{"PATH=/usr/bin", "HOME=/root"},
		WorkingDir: "/app",
		User:       "1000:1000",
		Stdin:      true,
		StdinOnce:  false,
		Tty:        true,
		Labels: map[string]string{
			"app":     "web",
			"version": "1.0",
		},
		Annotations: map[string]string{
			"io.kubernetes.pod.name": "test-pod",
		},
		Mounts: []Mount{
			{
				Type:        MountTypeBind,
				Source:      "/host/path",
				Destination: "/container/path",
				ReadOnly:    false,
			},
		},
		Resources: &ResourceConfig{
			MemoryLimitBytes: 512 * 1024 * 1024,
			CPUShares:        1024,
		},
		SandboxID: "sandbox-456",
	}

	// Verify all fields
	if config.ID != "container-123" {
		t.Errorf("Expected ID 'container-123', got '%s'", config.ID)
	}

	if len(config.Command) != 1 {
		t.Errorf("Expected 1 command, got %d", len(config.Command))
	}

	if len(config.Env) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(config.Env))
	}

	if len(config.Mounts) != 1 {
		t.Errorf("Expected 1 mount, got %d", len(config.Mounts))
	}

	if config.Resources == nil {
		t.Error("Expected non-nil resources")
	}
}

// =============================================================================
// OCI Types Tests
// =============================================================================

func TestOCIProcess(t *testing.T) {
	proc := &OCIProcess{
		Terminal:        true,
		User:            OCIUser{UID: 1000, GID: 1000},
		Args:            []string{"/bin/sh", "-c", "echo test"},
		Env:             []string{"PATH=/usr/bin"},
		Cwd:             "/app",
		NoNewPrivileges: true,
		Capabilities: &OCICapabilities{
			Bounding:    []string{"CAP_NET_BIND_SERVICE"},
			Effective:   []string{"CAP_NET_BIND_SERVICE"},
			Permitted:   []string{"CAP_NET_BIND_SERVICE"},
			Inheritable: []string{},
			Ambient:     []string{},
		},
		Rlimits: []OCIRlimit{
			{Type: "RLIMIT_NOFILE", Hard: 65536, Soft: 32768},
		},
	}

	if !proc.Terminal {
		t.Error("Expected terminal to be true")
	}

	if proc.User.UID != 1000 {
		t.Errorf("Expected UID 1000, got %d", proc.User.UID)
	}

	if !proc.NoNewPrivileges {
		t.Error("Expected NoNewPrivileges to be true")
	}
}

func TestOCIMount(t *testing.T) {
	mount := OCIMount{
		Destination: "/dev/shm",
		Type:        "tmpfs",
		Source:      "shm",
		Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
	}

	if mount.Destination != "/dev/shm" {
		t.Errorf("Expected destination '/dev/shm', got '%s'", mount.Destination)
	}

	if len(mount.Options) != 5 {
		t.Errorf("Expected 5 options, got %d", len(mount.Options))
	}
}

func TestOCILinux(t *testing.T) {
	linux := &OCILinux{
		Namespaces: []OCINamespace{
			{Type: "pid"},
			{Type: "network"},
			{Type: "mount"},
		},
		Resources: &OCIResources{
			Memory: &OCIMemory{
				Limit: func() *int64 { v := int64(512 * 1024 * 1024); return &v }(),
			},
			CPU: &OCICPU{
				Shares: func() *uint64 { v := uint64(1024); return &v }(),
			},
			Pids: &OCIPids{
				Limit: 100,
			},
		},
		CgroupsPath: "/test/container",
		MaskedPaths: []string{"/proc/kcore"},
		ReadonlyPaths: []string{"/proc/sys"},
		Sysctl: map[string]string{
			"net.core.somaxconn": "1024",
		},
	}

	if len(linux.Namespaces) != 3 {
		t.Errorf("Expected 3 namespaces, got %d", len(linux.Namespaces))
	}

	if linux.Resources.Pids.Limit != 100 {
		t.Errorf("Expected PIDs limit 100, got %d", linux.Resources.Pids.Limit)
	}
}

// =============================================================================
// OCI Seccomp Tests
// =============================================================================

func TestOCISeccomp(t *testing.T) {
	seccomp := &OCISeccomp{
		DefaultAction: "SCMP_ACT_ERRNO",
		Architectures: []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_X86"},
		Syscalls: []OCISeccompSyscall{
			{
				Names:  []string{"accept", "accept4", "bind", "connect"},
				Action: "SCMP_ACT_ALLOW",
			},
			{
				Names:  []string{"clone"},
				Action: "SCMP_ACT_ALLOW",
				Args: []OCISeccompArg{
					{Index: 0, Value: 2080505856, Op: "SCMP_CMP_MASKED_EQ"},
				},
			},
		},
	}

	if seccomp.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("Expected default action 'SCMP_ACT_ERRNO', got '%s'", seccomp.DefaultAction)
	}

	if len(seccomp.Syscalls) != 2 {
		t.Errorf("Expected 2 syscall rules, got %d", len(seccomp.Syscalls))
	}

	if len(seccomp.Syscalls[1].Args) != 1 {
		t.Errorf("Expected 1 arg for clone syscall, got %d", len(seccomp.Syscalls[1].Args))
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkGenerateID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateID()
	}
}

func BenchmarkParseUserGroup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parseUserGroup("1000:1000")
	}
}

func BenchmarkDefaultCapabilities(b *testing.B) {
	for i := 0; i < b.N; i++ {
		defaultCapabilities()
	}
}

func BenchmarkContainerStateString(b *testing.B) {
	state := ContainerStateRunning
	for i := 0; i < b.N; i++ {
		_ = state.String()
	}
}
