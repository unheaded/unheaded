// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SandboxState represents the state of a sandbox.
type SandboxState int

const (
	// SandboxStateUnknown indicates unknown state.
	SandboxStateUnknown SandboxState = iota
	// SandboxStateReady indicates the sandbox is ready.
	SandboxStateReady
	// SandboxStateNotReady indicates the sandbox is not ready.
	SandboxStateNotReady
)

// String returns the string representation of SandboxState.
func (s SandboxState) String() string {
	switch s {
	case SandboxStateReady:
		return "ready"
	case SandboxStateNotReady:
		return "not_ready"
	default:
		return "unknown"
	}
}

// SandboxInfo contains information about a sandbox.
type SandboxInfo struct {
	// ID is the unique identifier of the sandbox.
	ID string
	// Name is the human-readable name.
	Name string
	// State is the current state.
	State SandboxState
	// CreatedAt is when the sandbox was created.
	CreatedAt time.Time
	// Labels are user-defined metadata.
	Labels map[string]string
	// Annotations are runtime-specific metadata.
	Annotations map[string]string
	// NetworkConfig is the network configuration.
	NetworkConfig *SandboxNetworkConfig
	// Linux contains Linux-specific information.
	Linux *SandboxLinuxInfo
}

// SandboxLinuxInfo contains Linux-specific sandbox information.
type SandboxLinuxInfo struct {
	// CgroupParent is the parent cgroup.
	CgroupParent string
	// Namespaces are the namespace paths.
	Namespaces map[NamespaceType]string
}

// SandboxConfig defines the configuration for creating a sandbox.
type SandboxConfig struct {
	// ID is the unique identifier.
	ID string
	// Name is the human-readable name.
	Name string
	// Hostname is the sandbox hostname.
	Hostname string
	// LogDirectory is the directory for logs.
	LogDirectory string
	// DNSConfig is the DNS configuration.
	DNSConfig *DNSConfig
	// PortMappings are port mappings.
	PortMappings []*PortMapping
	// Labels are user-defined metadata.
	Labels map[string]string
	// Annotations are runtime-specific metadata.
	Annotations map[string]string
	// Linux contains Linux-specific configuration.
	Linux *LinuxSandboxConfig
}

// DNSConfig contains DNS configuration.
type DNSConfig struct {
	// Servers are DNS server addresses.
	Servers []string
	// Searches are DNS search domains.
	Searches []string
	// Options are DNS resolver options.
	Options []string
}

// PortMapping represents a port mapping.
type PortMapping struct {
	// Protocol is the protocol (tcp, udp, sctp).
	Protocol string
	// ContainerPort is the container port.
	ContainerPort int32
	// HostPort is the host port.
	HostPort int32
	// HostIP is the host IP to bind to.
	HostIP string
}

// LinuxSandboxConfig contains Linux-specific sandbox configuration.
type LinuxSandboxConfig struct {
	// CgroupParent is the parent cgroup.
	CgroupParent string
	// Sysctls are sysctl settings.
	Sysctls map[string]string
	// SecurityContext is the security context.
	SecurityContext *LinuxSandboxSecurityContext
	// Resources are resource limits.
	Resources *ResourceConfig
}

// LinuxSandboxSecurityContext contains sandbox security context.
type LinuxSandboxSecurityContext struct {
	// NamespaceOptions are namespace options.
	NamespaceOptions *NamespaceOptions
	// SELinuxOptions are SELinux options.
	SELinuxOptions *SELinuxOption
	// RunAsUser is the UID to run as.
	RunAsUser *int64
	// RunAsGroup is the GID to run as.
	RunAsGroup *int64
	// ReadonlyRootfs makes the root filesystem read-only.
	ReadonlyRootfs bool
	// SupplementalGroups are supplemental groups.
	SupplementalGroups []int64
	// Privileged runs in privileged mode.
	Privileged bool
	// SeccompProfilePath is the path to the seccomp profile.
	SeccompProfilePath string
}

// NamespaceOptions contains namespace options.
type NamespaceOptions struct {
	// Network is the network namespace mode.
	Network NamespaceMode
	// Pid is the PID namespace mode.
	Pid NamespaceMode
	// Ipc is the IPC namespace mode.
	Ipc NamespaceMode
	// TargetID is the target sandbox/container ID for namespace sharing.
	TargetID string
}

// NamespaceMode represents a namespace mode.
type NamespaceMode int

const (
	// NamespaceModePOD uses the pod namespace.
	NamespaceModePOD NamespaceMode = iota
	// NamespaceModeContainer uses the container namespace.
	NamespaceModeContainer
	// NamespaceModeNode uses the node namespace.
	NamespaceModeNode
	// NamespaceModeTarget uses a target's namespace.
	NamespaceModeTarget
)

// SandboxNetworkConfig contains sandbox network configuration.
type SandboxNetworkConfig struct {
	// NetworkMode is the network mode.
	NetworkMode string
	// IP is the IP address.
	IP string
	// Gateway is the gateway address.
	Gateway string
	// DNS is the DNS configuration.
	DNS *DNSConfig
}

// SandboxFilter defines filters for listing sandboxes.
type SandboxFilter struct {
	// ID filters by sandbox ID.
	ID string
	// Name filters by sandbox name.
	Name string
	// State filters by state.
	State SandboxState
	// Labels filters by labels.
	Labels map[string]string
}

// Sandbox represents an internal sandbox instance.
type Sandbox struct {
	mu sync.RWMutex

	info   *SandboxInfo
	config *SandboxConfig

	// Namespace paths
	NetNS  string
	IPCNS  string
	UTSNS  string
	UserNS string

	// Cgroup path
	cgroupPath string

	// Container IDs in this sandbox
	containers []string

	// Pause container info (infra container)
	pausePID int

	// Network info
	networkInfo *SandboxNetworkInfo
}

// SandboxNetworkInfo contains runtime network information.
type SandboxNetworkInfo struct {
	IP         string
	Gateway    string
	Interfaces []*NetworkInterface
}

// NetworkInterface represents a network interface.
type NetworkInterface struct {
	Name       string
	MacAddress string
	IPAddress  string
	Mask       string
}

// CreateSandbox creates a new pod sandbox.
func (r *DefaultRuntime) CreateSandbox(ctx context.Context, config *SandboxConfig) (*SandboxInfo, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	if config.ID == "" {
		config.ID = generateID()
	}

	r.mu.Lock()
	if _, exists := r.sandboxes[config.ID]; exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("sandbox already exists: %s", config.ID)
	}
	r.mu.Unlock()

	// Create sandbox directory
	sandboxRoot := filepath.Join(r.config.Root, "sandboxes", config.ID)
	if err := os.MkdirAll(sandboxRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	sandbox := &Sandbox{
		config:     config,
		containers: make([]string, 0),
	}

	// Create namespaces
	if err := r.setupSandboxNamespaces(sandbox, config); err != nil {
		_ = os.RemoveAll(sandboxRoot)
		return nil, fmt.Errorf("failed to setup namespaces: %w", err)
	}

	// Create cgroup if resources are specified
	if config.Linux != nil && config.Linux.Resources != nil {
		cgroupPath, err := r.cgroups.CreateCgroup(config.ID, config.Linux.Resources)
		if err != nil {
			r.cleanupSandboxNamespaces(sandbox)
			_ = os.RemoveAll(sandboxRoot)
			return nil, fmt.Errorf("failed to create cgroup: %w", err)
		}
		sandbox.cgroupPath = cgroupPath
	}

	// Best-effort sandbox setup. Each step is optional — DNS may be
	// read-only, the host's UTS namespace may already be set, the netns
	// may not exist if CNI isn't installed.
	if config.DNSConfig != nil {
		_ = r.setupSandboxDNS(sandboxRoot, config.DNSConfig)
	}
	if config.Hostname != "" && sandbox.UTSNS != "" {
		_ = r.namespaces.SetHostname(sandbox.UTSNS, config.Hostname)
	}
	if sandbox.NetNS != "" {
		_ = r.namespaces.ConfigureNetworkNamespace(sandbox.NetNS)
		_ = r.setupSandboxNetwork(sandbox, config)
	}

	now := time.Now()
	sandbox.info = &SandboxInfo{
		ID:          config.ID,
		Name:        config.Name,
		State:       SandboxStateReady,
		CreatedAt:   now,
		Labels:      config.Labels,
		Annotations: config.Annotations,
		NetworkConfig: &SandboxNetworkConfig{
			DNS: config.DNSConfig,
		},
		Linux: &SandboxLinuxInfo{
			Namespaces: map[NamespaceType]string{
				NetworkNamespace: sandbox.NetNS,
				IPCNamespace:     sandbox.IPCNS,
				UTSNamespace:     sandbox.UTSNS,
			},
		},
	}

	if config.Linux != nil {
		sandbox.info.Linux.CgroupParent = config.Linux.CgroupParent
	}

	r.mu.Lock()
	r.sandboxes[config.ID] = sandbox
	r.mu.Unlock()

	return sandbox.info, nil
}

// setupSandboxNamespaces sets up namespaces for a sandbox.
func (r *DefaultRuntime) setupSandboxNamespaces(sandbox *Sandbox, config *SandboxConfig) error {
	var namespaceTypes []NamespaceType

	// Determine which namespaces to create
	if config.Linux != nil && config.Linux.SecurityContext != nil && config.Linux.SecurityContext.NamespaceOptions != nil {
		opts := config.Linux.SecurityContext.NamespaceOptions

		if opts.Network != NamespaceModeNode {
			namespaceTypes = append(namespaceTypes, NetworkNamespace)
		}
		if opts.Ipc != NamespaceModeNode {
			namespaceTypes = append(namespaceTypes, IPCNamespace)
		}
	} else {
		// Default: create all pod-level namespaces
		namespaceTypes = []NamespaceType{NetworkNamespace, IPCNamespace, UTSNamespace}
	}

	// Create network namespace
	for _, nsType := range namespaceTypes {
		switch nsType {
		case NetworkNamespace:
			nsPath, err := r.namespaces.CreateNetworkNamespace(config.ID)
			if err != nil {
				return fmt.Errorf("failed to create network namespace: %w", err)
			}
			sandbox.NetNS = nsPath
		case IPCNamespace:
			// IPC namespaces are created as part of namespace set
			nsSet, err := r.namespaces.CreateNamespaces(config.ID+"-ipc", []NamespaceType{IPCNamespace})
			if err != nil {
				return fmt.Errorf("failed to create IPC namespace: %w", err)
			}
			sandbox.IPCNS = nsSet.Paths[IPCNamespace]
		case UTSNamespace:
			// UTS namespaces are created as part of namespace set
			nsSet, err := r.namespaces.CreateNamespaces(config.ID+"-uts", []NamespaceType{UTSNamespace})
			if err != nil {
				return fmt.Errorf("failed to create UTS namespace: %w", err)
			}
			sandbox.UTSNS = nsSet.Paths[UTSNamespace]
		}
	}

	return nil
}

// cleanupSandboxNamespaces cleans up sandbox namespaces.
func (r *DefaultRuntime) cleanupSandboxNamespaces(sandbox *Sandbox) {
	if sandbox.NetNS != "" {
		_ = r.namespaces.RemoveNamespaces(sandbox.config.ID)
	}
	if sandbox.IPCNS != "" {
		_ = r.namespaces.RemoveNamespaces(sandbox.config.ID + "-ipc")
	}
	if sandbox.UTSNS != "" {
		_ = r.namespaces.RemoveNamespaces(sandbox.config.ID + "-uts")
	}
}

// setupSandboxDNS sets up DNS configuration for a sandbox.
func (r *DefaultRuntime) setupSandboxDNS(sandboxRoot string, dnsConfig *DNSConfig) error {
	resolvConfPath := filepath.Join(sandboxRoot, "resolv.conf")

	var content string

	for _, search := range dnsConfig.Searches {
		content += fmt.Sprintf("search %s\n", search)
	}

	for _, server := range dnsConfig.Servers {
		content += fmt.Sprintf("nameserver %s\n", server)
	}

	for _, opt := range dnsConfig.Options {
		content += fmt.Sprintf("options %s\n", opt)
	}

	return os.WriteFile(resolvConfPath, []byte(content), 0644)
}

// setupSandboxNetwork sets up networking for a sandbox.
func (r *DefaultRuntime) setupSandboxNetwork(sandbox *Sandbox, config *SandboxConfig) error {
	// This would integrate with CNI or configure basic networking
	// For now, we just configure loopback

	sandbox.networkInfo = &SandboxNetworkInfo{
		IP:      "10.244.0.1", // Placeholder
		Gateway: "10.244.0.0",
	}

	return nil
}

// StopSandbox stops a running sandbox.
func (r *DefaultRuntime) StopSandbox(ctx context.Context, sandboxID string) error {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return ErrSandboxNotFound
	}

	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()

	// Best-effort: stop every container in the sandbox; failures don't
	// block teardown of the rest.
	for _, containerID := range sandbox.containers {
		_ = r.StopContainer(ctx, containerID, 30*time.Second)
	}

	// Stop pause container if exists
	if sandbox.pausePID > 0 {
		proc, err := os.FindProcess(sandbox.pausePID)
		if err == nil {
			_ = proc.Kill()
		}
		sandbox.pausePID = 0
	}

	sandbox.info.State = SandboxStateNotReady

	return nil
}

// RemoveSandbox removes a sandbox.
func (r *DefaultRuntime) RemoveSandbox(ctx context.Context, sandboxID string) error {
	r.mu.Lock()
	sandbox, exists := r.sandboxes[sandboxID]
	if !exists {
		r.mu.Unlock()
		return ErrSandboxNotFound
	}
	r.mu.Unlock()

	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()

	// Check if sandbox has containers
	if len(sandbox.containers) > 0 {
		return fmt.Errorf("sandbox has %d containers", len(sandbox.containers))
	}

	// Cleanup namespaces
	r.cleanupSandboxNamespaces(sandbox)

	// Cleanup cgroup
	if sandbox.cgroupPath != "" {
		_ = r.cgroups.RemoveCgroup(sandbox.cgroupPath)
	}

	// Remove sandbox directory
	sandboxRoot := filepath.Join(r.config.Root, "sandboxes", sandboxID)
	_ = os.RemoveAll(sandboxRoot)

	r.mu.Lock()
	delete(r.sandboxes, sandboxID)
	r.mu.Unlock()

	return nil
}

// GetSandbox returns information about a sandbox.
func (r *DefaultRuntime) GetSandbox(ctx context.Context, sandboxID string) (*SandboxInfo, error) {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrSandboxNotFound
	}

	sandbox.mu.RLock()
	defer sandbox.mu.RUnlock()

	// Return a copy
	info := *sandbox.info
	return &info, nil
}

// ListSandboxes lists sandboxes matching the filter.
func (r *DefaultRuntime) ListSandboxes(ctx context.Context, filter *SandboxFilter) ([]*SandboxInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*SandboxInfo

	for _, sandbox := range r.sandboxes {
		sandbox.mu.RLock()
		info := sandbox.info
		sandbox.mu.RUnlock()

		if filter != nil {
			if filter.ID != "" && info.ID != filter.ID {
				continue
			}
			if filter.Name != "" && info.Name != filter.Name {
				continue
			}
			if filter.State != SandboxStateUnknown && info.State != filter.State {
				continue
			}
			if len(filter.Labels) > 0 {
				match := true
				for k, v := range filter.Labels {
					if info.Labels[k] != v {
						match = false
						break
					}
				}
				if !match {
					continue
				}
			}
		}

		// Return a copy
		infoCopy := *info
		result = append(result, &infoCopy)
	}

	return result, nil
}

// AddContainerToSandbox adds a container to a sandbox.
func (r *DefaultRuntime) AddContainerToSandbox(sandboxID, containerID string) error {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return ErrSandboxNotFound
	}

	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()

	// Check if container already in sandbox
	for _, id := range sandbox.containers {
		if id == containerID {
			return nil
		}
	}

	sandbox.containers = append(sandbox.containers, containerID)
	return nil
}

// RemoveContainerFromSandbox removes a container from a sandbox.
func (r *DefaultRuntime) RemoveContainerFromSandbox(sandboxID, containerID string) error {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return ErrSandboxNotFound
	}

	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()

	for i, id := range sandbox.containers {
		if id == containerID {
			sandbox.containers = append(sandbox.containers[:i], sandbox.containers[i+1:]...)
			return nil
		}
	}

	return nil
}

// GetSandboxContainers returns the containers in a sandbox.
func (r *DefaultRuntime) GetSandboxContainers(sandboxID string) ([]string, error) {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrSandboxNotFound
	}

	sandbox.mu.RLock()
	defer sandbox.mu.RUnlock()

	result := make([]string, len(sandbox.containers))
	copy(result, sandbox.containers)
	return result, nil
}

// GetSandboxNetwork returns network information for a sandbox.
func (r *DefaultRuntime) GetSandboxNetwork(sandboxID string) (*SandboxNetworkInfo, error) {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrSandboxNotFound
	}

	sandbox.mu.RLock()
	defer sandbox.mu.RUnlock()

	if sandbox.networkInfo == nil {
		return nil, fmt.Errorf("no network info for sandbox")
	}

	// Return a copy
	info := *sandbox.networkInfo
	return &info, nil
}

// PortForward forwards a port from a sandbox.
func (r *DefaultRuntime) PortForward(ctx context.Context, sandboxID string, port int32, stream IOStreams) error {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return ErrSandboxNotFound
	}

	sandbox.mu.RLock()
	netNS := sandbox.NetNS
	networkInfo := sandbox.networkInfo
	sandbox.mu.RUnlock()

	if netNS == "" {
		return fmt.Errorf("sandbox has no network namespace")
	}

	if networkInfo == nil {
		return fmt.Errorf("sandbox has no network info")
	}

	// Port forwarding is deferred to v0.2 (roadmap). Container networking
	// handles connectivity directly via the network namespace. For direct
	// port access, use the sandbox's network info to connect to the
	// container IP on the desired port.
	return fmt.Errorf("port forwarding deferred (use container network namespace for direct access)")
}

// SandboxStatus returns detailed status of a sandbox.
type SandboxStatus struct {
	ID        string
	Metadata  *SandboxMetadata
	State     SandboxState
	CreatedAt time.Time
	Network   *SandboxNetworkStatus
	Linux     *LinuxSandboxStatus
}

// SandboxMetadata contains sandbox metadata.
type SandboxMetadata struct {
	Name      string
	UID       string
	Namespace string
}

// SandboxNetworkStatus contains network status.
type SandboxNetworkStatus struct {
	IP            string
	AdditionalIPs []string
}

// LinuxSandboxStatus contains Linux-specific status.
type LinuxSandboxStatus struct {
	Namespaces *NamespaceStatus
}

// NamespaceStatus contains namespace status.
type NamespaceStatus struct {
	Network string
	IPC     string
	UTS     string
	User    string
}

// GetSandboxStatus returns detailed sandbox status.
func (r *DefaultRuntime) GetSandboxStatus(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	r.mu.RLock()
	sandbox, exists := r.sandboxes[sandboxID]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrSandboxNotFound
	}

	sandbox.mu.RLock()
	defer sandbox.mu.RUnlock()

	status := &SandboxStatus{
		ID:        sandboxID,
		State:     sandbox.info.State,
		CreatedAt: sandbox.info.CreatedAt,
		Metadata: &SandboxMetadata{
			Name: sandbox.config.Name,
		},
		Linux: &LinuxSandboxStatus{
			Namespaces: &NamespaceStatus{
				Network: sandbox.NetNS,
				IPC:     sandbox.IPCNS,
				UTS:     sandbox.UTSNS,
				User:    sandbox.UserNS,
			},
		},
	}

	if sandbox.networkInfo != nil {
		status.Network = &SandboxNetworkStatus{
			IP: sandbox.networkInfo.IP,
		}
	}

	return status, nil
}
