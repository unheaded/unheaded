// SPDX-License-Identifier: GPL-3.0-or-later
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
// Sandbox State Tests
// =============================================================================

func TestSandboxState_String(t *testing.T) {
	tests := []struct {
		state    SandboxState
		expected string
	}{
		{SandboxStateUnknown, "unknown"},
		{SandboxStateReady, "ready"},
		{SandboxStateNotReady, "not_ready"},
		{SandboxState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("SandboxState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// =============================================================================
// Sandbox Info Tests
// =============================================================================

func TestSandboxInfo_Fields(t *testing.T) {
	now := time.Now()
	info := &SandboxInfo{
		ID:          "sandbox-123",
		Name:        "test-sandbox",
		State:       SandboxStateReady,
		CreatedAt:   now,
		Labels:      map[string]string{"app": "test"},
		Annotations: map[string]string{"version": "1.0"},
		NetworkConfig: &SandboxNetworkConfig{
			NetworkMode: "bridge",
			IP:          "10.244.0.5",
			Gateway:     "10.244.0.1",
		},
		Linux: &SandboxLinuxInfo{
			CgroupParent: "/kubepods/pod123",
			Namespaces: map[NamespaceType]string{
				NetworkNamespace: "/var/run/netns/cni-123",
			},
		},
	}

	if info.ID != "sandbox-123" {
		t.Errorf("Expected ID 'sandbox-123', got '%s'", info.ID)
	}

	if info.State != SandboxStateReady {
		t.Errorf("Expected state Ready, got %v", info.State)
	}

	if info.NetworkConfig.IP != "10.244.0.5" {
		t.Errorf("Expected IP '10.244.0.5', got '%s'", info.NetworkConfig.IP)
	}

	if info.Linux.CgroupParent != "/kubepods/pod123" {
		t.Errorf("Expected cgroup parent '/kubepods/pod123', got '%s'", info.Linux.CgroupParent)
	}
}

// =============================================================================
// Sandbox Config Tests
// =============================================================================

func TestSandboxConfig_Complete(t *testing.T) {
	config := &SandboxConfig{
		ID:           "sandbox-456",
		Name:         "test-sandbox",
		Hostname:     "test-host",
		LogDirectory: "/var/log/pods/test",
		DNSConfig: &DNSConfig{
			Servers:  []string{"8.8.8.8", "8.8.4.4"},
			Searches: []string{"example.com", "local"},
			Options:  []string{"ndots:5"},
		},
		PortMappings: []*PortMapping{
			{Protocol: "tcp", ContainerPort: 80, HostPort: 8080, HostIP: "0.0.0.0"},
			{Protocol: "tcp", ContainerPort: 443, HostPort: 8443},
		},
		Labels:      map[string]string{"app": "web"},
		Annotations: map[string]string{"io.kubernetes.pod.uid": "uid-123"},
		Linux: &LinuxSandboxConfig{
			CgroupParent: "/kubepods",
			Sysctls:      map[string]string{"net.core.somaxconn": "1024"},
			SecurityContext: &LinuxSandboxSecurityContext{
				NamespaceOptions: &NamespaceOptions{
					Network: NamespaceModePOD,
					Pid:     NamespaceModePOD,
					Ipc:     NamespaceModePOD,
				},
				Privileged: false,
			},
			Resources: &ResourceConfig{
				MemoryLimitBytes: 1024 * 1024 * 1024,
				CPUShares:        1024,
			},
		},
	}

	if config.ID != "sandbox-456" {
		t.Errorf("Expected ID 'sandbox-456', got '%s'", config.ID)
	}

	if len(config.DNSConfig.Servers) != 2 {
		t.Errorf("Expected 2 DNS servers, got %d", len(config.DNSConfig.Servers))
	}

	if len(config.PortMappings) != 2 {
		t.Errorf("Expected 2 port mappings, got %d", len(config.PortMappings))
	}

	if config.Linux.CgroupParent != "/kubepods" {
		t.Errorf("Expected cgroup parent '/kubepods', got '%s'", config.Linux.CgroupParent)
	}
}

// =============================================================================
// DNS Config Tests
// =============================================================================

func TestDNSConfig(t *testing.T) {
	dns := &DNSConfig{
		Servers:  []string{"10.0.0.10", "10.0.0.11"},
		Searches: []string{"default.svc.cluster.local", "svc.cluster.local", "cluster.local"},
		Options:  []string{"ndots:5", "timeout:2", "attempts:2"},
	}

	if len(dns.Servers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(dns.Servers))
	}

	if len(dns.Searches) != 3 {
		t.Errorf("Expected 3 search domains, got %d", len(dns.Searches))
	}

	if len(dns.Options) != 3 {
		t.Errorf("Expected 3 options, got %d", len(dns.Options))
	}
}

// =============================================================================
// Port Mapping Tests
// =============================================================================

func TestPortMapping(t *testing.T) {
	mapping := &PortMapping{
		Protocol:      "tcp",
		ContainerPort: 8080,
		HostPort:      80,
		HostIP:        "192.168.1.100",
	}

	if mapping.Protocol != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", mapping.Protocol)
	}

	if mapping.ContainerPort != 8080 {
		t.Errorf("Expected container port 8080, got %d", mapping.ContainerPort)
	}

	if mapping.HostPort != 80 {
		t.Errorf("Expected host port 80, got %d", mapping.HostPort)
	}

	if mapping.HostIP != "192.168.1.100" {
		t.Errorf("Expected host IP '192.168.1.100', got '%s'", mapping.HostIP)
	}
}

// =============================================================================
// Namespace Options Tests
// =============================================================================

func TestNamespaceOptions(t *testing.T) {
	opts := &NamespaceOptions{
		Network:  NamespaceModePOD,
		Pid:      NamespaceModeContainer,
		Ipc:      NamespaceModeNode,
		TargetID: "target-container-123",
	}

	if opts.Network != NamespaceModePOD {
		t.Errorf("Expected network mode POD, got %v", opts.Network)
	}

	if opts.Pid != NamespaceModeContainer {
		t.Errorf("Expected PID mode Container, got %v", opts.Pid)
	}

	if opts.TargetID != "target-container-123" {
		t.Errorf("Expected target ID 'target-container-123', got '%s'", opts.TargetID)
	}
}

// =============================================================================
// Namespace Mode Tests
// =============================================================================

func TestNamespaceMode(t *testing.T) {
	tests := []struct {
		mode NamespaceMode
		val  int
	}{
		{NamespaceModePOD, 0},
		{NamespaceModeContainer, 1},
		{NamespaceModeNode, 2},
		{NamespaceModeTarget, 3},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if int(tt.mode) != tt.val {
				t.Errorf("Expected mode %d, got %d", tt.val, int(tt.mode))
			}
		})
	}
}

// =============================================================================
// Sandbox Filter Tests
// =============================================================================

func TestSandboxFilter(t *testing.T) {
	filter := &SandboxFilter{
		ID:     "sandbox-123",
		Name:   "test-sandbox",
		State:  SandboxStateReady,
		Labels: map[string]string{"app": "web", "env": "prod"},
	}

	if filter.ID != "sandbox-123" {
		t.Errorf("Expected ID 'sandbox-123', got '%s'", filter.ID)
	}

	if filter.State != SandboxStateReady {
		t.Errorf("Expected state Ready, got %v", filter.State)
	}

	if len(filter.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(filter.Labels))
	}
}

// =============================================================================
// Sandbox Network Info Tests
// =============================================================================

func TestSandboxNetworkInfo(t *testing.T) {
	info := &SandboxNetworkInfo{
		IP:      "10.244.0.10",
		Gateway: "10.244.0.1",
		Interfaces: []*NetworkInterface{
			{
				Name:       "eth0",
				MacAddress: "02:42:ac:11:00:02",
				IPAddress:  "10.244.0.10",
				Mask:       "255.255.255.0",
			},
		},
	}

	if info.IP != "10.244.0.10" {
		t.Errorf("Expected IP '10.244.0.10', got '%s'", info.IP)
	}

	if len(info.Interfaces) != 1 {
		t.Errorf("Expected 1 interface, got %d", len(info.Interfaces))
	}

	if info.Interfaces[0].Name != "eth0" {
		t.Errorf("Expected interface name 'eth0', got '%s'", info.Interfaces[0].Name)
	}
}

// =============================================================================
// Sandbox Create Tests
// =============================================================================

func TestSandboxCreate_NilConfig(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.CreateSandbox(ctx, nil)
	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

func TestSandboxCreate_WithoutID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &SandboxConfig{
		Name: "test-sandbox",
	}

	info, err := rt.CreateSandbox(ctx, config)
	if err != nil {
		t.Fatalf("CreateSandbox failed: %v", err)
	}

	if info.ID == "" {
		t.Error("Expected auto-generated ID, got empty string")
	}
}

func TestSandboxCreate_DuplicateID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &SandboxConfig{
		ID:   "test-sandbox-dup",
		Name: "test-sandbox",
	}

	_, err := rt.CreateSandbox(ctx, config)
	if err != nil {
		t.Fatalf("First CreateSandbox failed: %v", err)
	}

	_, err = rt.CreateSandbox(ctx, config)
	if err == nil {
		t.Error("Expected error for duplicate sandbox ID")
	}
}

// =============================================================================
// Sandbox Get Tests
// =============================================================================

func TestSandboxGet_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.GetSandbox(ctx, "nonexistent")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

func TestSandboxGet_Existing(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &SandboxConfig{
		ID:   "test-sandbox-get",
		Name: "test-sandbox",
	}

	_, err := rt.CreateSandbox(ctx, config)
	if err != nil {
		t.Fatalf("CreateSandbox failed: %v", err)
	}

	info, err := rt.GetSandbox(ctx, "test-sandbox-get")
	if err != nil {
		t.Fatalf("GetSandbox failed: %v", err)
	}

	if info.ID != "test-sandbox-get" {
		t.Errorf("Expected ID 'test-sandbox-get', got '%s'", info.ID)
	}
}

// =============================================================================
// Sandbox List Tests
// =============================================================================

func TestSandboxList_Empty(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	sandboxes, err := rt.ListSandboxes(ctx, nil)
	if err != nil {
		t.Fatalf("ListSandboxes failed: %v", err)
	}

	if len(sandboxes) != 0 {
		t.Errorf("Expected 0 sandboxes, got %d", len(sandboxes))
	}
}

func TestSandboxList_WithFilter(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	// Create sandboxes
	for i := 0; i < 3; i++ {
		config := &SandboxConfig{
			Name:   "web",
			Labels: map[string]string{"app": "web"},
		}
		rt.CreateSandbox(ctx, config)
	}

	for i := 0; i < 2; i++ {
		config := &SandboxConfig{
			Name:   "db",
			Labels: map[string]string{"app": "db"},
		}
		rt.CreateSandbox(ctx, config)
	}

	// Filter by label
	filter := &SandboxFilter{
		Labels: map[string]string{"app": "web"},
	}

	sandboxes, err := rt.ListSandboxes(ctx, filter)
	if err != nil {
		t.Fatalf("ListSandboxes failed: %v", err)
	}

	if len(sandboxes) != 3 {
		t.Errorf("Expected 3 sandboxes with app=web, got %d", len(sandboxes))
	}
}

// =============================================================================
// Sandbox Stop Tests
// =============================================================================

func TestSandboxStop_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.StopSandbox(ctx, "nonexistent")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

// =============================================================================
// Sandbox Remove Tests
// =============================================================================

func TestSandboxRemove_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.RemoveSandbox(ctx, "nonexistent")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

func TestSandboxRemove_Empty(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &SandboxConfig{
		ID:   "test-sandbox-remove",
		Name: "test-sandbox",
	}

	_, err := rt.CreateSandbox(ctx, config)
	if err != nil {
		t.Fatalf("CreateSandbox failed: %v", err)
	}

	err = rt.RemoveSandbox(ctx, "test-sandbox-remove")
	if err != nil {
		t.Fatalf("RemoveSandbox failed: %v", err)
	}

	// Verify sandbox is removed
	_, err = rt.GetSandbox(ctx, "test-sandbox-remove")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

// =============================================================================
// Sandbox Container Management Tests
// =============================================================================

func TestAddContainerToSandbox_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	err := rt.AddContainerToSandbox("nonexistent", "container-123")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

func TestRemoveContainerFromSandbox_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	err := rt.RemoveContainerFromSandbox("nonexistent", "container-123")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

func TestGetSandboxContainers_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	_, err := rt.GetSandboxContainers("nonexistent")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

func TestGetSandboxNetwork_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	_, err := rt.GetSandboxNetwork("nonexistent")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

// =============================================================================
// Sandbox Status Tests
// =============================================================================

func TestGetSandboxStatus_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.GetSandboxStatus(ctx, "nonexistent")
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}

func TestSandboxStatus_Fields(t *testing.T) {
	now := time.Now()
	status := &SandboxStatus{
		ID:        "sandbox-123",
		State:     SandboxStateReady,
		CreatedAt: now,
		Metadata: &SandboxMetadata{
			Name:      "test-sandbox",
			UID:       "uid-123",
			Namespace: "default",
		},
		Network: &SandboxNetworkStatus{
			IP:            "10.244.0.10",
			AdditionalIPs: []string{"10.244.1.10"},
		},
		Linux: &LinuxSandboxStatus{
			Namespaces: &NamespaceStatus{
				Network: "/var/run/netns/cni-123",
				IPC:     "/proc/123/ns/ipc",
				UTS:     "/proc/123/ns/uts",
			},
		},
	}

	if status.ID != "sandbox-123" {
		t.Errorf("Expected ID 'sandbox-123', got '%s'", status.ID)
	}

	if status.Metadata.Name != "test-sandbox" {
		t.Errorf("Expected name 'test-sandbox', got '%s'", status.Metadata.Name)
	}

	if status.Network.IP != "10.244.0.10" {
		t.Errorf("Expected IP '10.244.0.10', got '%s'", status.Network.IP)
	}
}

// =============================================================================
// Concurrent Sandbox Operations Tests
// =============================================================================

func TestSandboxConcurrentAccess(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	// Create some sandboxes
	sandboxIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		config := &SandboxConfig{
			Name: "test-sandbox",
		}
		info, err := rt.CreateSandbox(ctx, config)
		if err != nil {
			t.Fatalf("CreateSandbox failed: %v", err)
		}
		sandboxIDs[i] = info.ID
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent gets
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := rt.GetSandbox(ctx, sandboxIDs[idx%len(sandboxIDs)])
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	// Concurrent lists
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rt.ListSandboxes(ctx, nil)
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
// Sandbox Struct Tests
// =============================================================================

func TestSandboxStruct(t *testing.T) {
	sandbox := &Sandbox{
		info: &SandboxInfo{
			ID:    "sandbox-123",
			State: SandboxStateReady,
		},
		config: &SandboxConfig{
			ID:   "sandbox-123",
			Name: "test-sandbox",
		},
		NetNS:      "/var/run/netns/cni-123",
		IPCNS:      "/proc/123/ns/ipc",
		UTSNS:      "/proc/123/ns/uts",
		cgroupPath: "/kubepods/pod123",
		containers: []string{"container-1", "container-2"},
		pausePID:   12345,
		networkInfo: &SandboxNetworkInfo{
			IP:      "10.244.0.10",
			Gateway: "10.244.0.1",
		},
	}

	if sandbox.info.ID != "sandbox-123" {
		t.Errorf("Expected ID 'sandbox-123', got '%s'", sandbox.info.ID)
	}

	if len(sandbox.containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(sandbox.containers))
	}

	if sandbox.pausePID != 12345 {
		t.Errorf("Expected pause PID 12345, got %d", sandbox.pausePID)
	}
}

// =============================================================================
// Security Context Tests
// =============================================================================

func TestLinuxSandboxSecurityContext(t *testing.T) {
	uid := int64(1000)
	gid := int64(1000)

	secCtx := &LinuxSandboxSecurityContext{
		NamespaceOptions: &NamespaceOptions{
			Network: NamespaceModePOD,
		},
		SELinuxOptions: &SELinuxOption{
			User:  "system_u",
			Role:  "system_r",
			Type:  "container_t",
			Level: "s0:c1,c2",
		},
		RunAsUser:            &uid,
		RunAsGroup:           &gid,
		ReadonlyRootfs:       true,
		SupplementalGroups:   []int64{100, 101},
		Privileged:           false,
		SeccompProfilePath:   "/etc/seccomp/profiles/default.json",
	}

	if *secCtx.RunAsUser != 1000 {
		t.Errorf("Expected UID 1000, got %d", *secCtx.RunAsUser)
	}

	if !secCtx.ReadonlyRootfs {
		t.Error("Expected readonly rootfs")
	}

	if secCtx.Privileged {
		t.Error("Expected non-privileged")
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkSandboxList(b *testing.B) {
	root, err := os.MkdirTemp("", "sandbox-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(root)

	rt := &DefaultRuntime{
		config:    &RuntimeConfig{Root: root},
		sandboxes: make(map[string]*Sandbox),
	}

	// Pre-populate
	for i := 0; i < 100; i++ {
		id := generateID()
		rt.sandboxes[id] = &Sandbox{
			info: &SandboxInfo{
				ID:    id,
				State: SandboxStateReady,
			},
		}
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.ListSandboxes(ctx, nil)
	}
}

// =============================================================================
// Port Forward Tests
// =============================================================================

func TestPortForward_NotFound(t *testing.T) {
	root := testTempDir(t)

	rt := &DefaultRuntime{
		config:    &RuntimeConfig{Root: root, StateDir: filepath.Join(root, "state")},
		sandboxes: make(map[string]*Sandbox),
	}

	ctx := context.Background()
	err := rt.PortForward(ctx, "nonexistent", 8080, IOStreams{})
	if err != ErrSandboxNotFound {
		t.Errorf("Expected ErrSandboxNotFound, got %v", err)
	}
}
