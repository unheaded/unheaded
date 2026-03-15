// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package lxd provides LXD orchestration for the Unheaded control plane
// THE CITADEL FORGES - Where NixOS containers are born and managed
package lxd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// ERRORS - WHEN THE FORGE FAILS
// ============================================================================

var (
	ErrClientNotConnected = errors.New("LXD client not connected")
	ErrContainerNotFound  = errors.New("container not found")
	ErrContainerExists    = errors.New("container already exists")
	ErrInvalidConfig      = errors.New("invalid container configuration")
	ErrOperationTimeout   = errors.New("operation timed out")
	ErrImageNotFound      = errors.New("image not found")
	ErrNetworkError       = errors.New("network configuration error")
	ErrStorageError       = errors.New("storage configuration error")
	ErrSnapshotNotFound   = errors.New("snapshot not found")
)

// opIDCounter provides globally unique operation IDs across all client types.
var opIDCounter atomic.Int64

// ============================================================================
// CONTAINER TYPES - THE CITADEL BLUEPRINTS
// ============================================================================

// ContainerConfig represents the configuration for creating a container
type ContainerConfig struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`        // e.g., "nixos:latest"
	Profiles     []string          `json:"profiles"`     // LXD profiles
	Devices      map[string]Device `json:"devices"`      // Attached devices
	Config       map[string]string `json:"config"`       // Container config
	Ephemeral    bool              `json:"ephemeral"`    // Ephemeral container
	Architecture string            `json:"architecture"` // x86_64, arm64
}

// Device represents an LXD device attachment
type Device struct {
	Type       string            `json:"type"` // disk, nic, gpu, etc.
	Properties map[string]string `json:"properties"`
}

// ContainerInfo represents container information from LXD
type ContainerInfo struct {
	Name         string            `json:"name"`
	Status       string            `json:"status"` // Running, Stopped, Frozen
	StatusCode   int               `json:"status_code"`
	Architecture string            `json:"architecture"`
	CreatedAt    time.Time         `json:"created_at"`
	LastUsedAt   time.Time         `json:"last_used_at"`
	Profiles     []string          `json:"profiles"`
	Config       map[string]string `json:"config"`
	Devices      map[string]Device `json:"devices"`
	Ephemeral    bool              `json:"ephemeral"`
	Stateful     bool              `json:"stateful"`
}

// ContainerState represents the runtime state of a container
type ContainerState struct {
	Status     string               `json:"status"`
	StatusCode int                  `json:"status_code"`
	CPU        CPUState             `json:"cpu"`
	Memory     MemoryState          `json:"memory"`
	Network    map[string]NetState  `json:"network"`
	Disk       map[string]DiskState `json:"disk"`
	Pid        int64                `json:"pid"`
	Processes  int64                `json:"processes"`
}

// CPUState represents CPU usage statistics
type CPUState struct {
	Usage int64 `json:"usage"` // CPU time in nanoseconds
}

// MemoryState represents memory usage statistics
type MemoryState struct {
	Usage     int64 `json:"usage"`      // Current memory usage in bytes
	UsagePeak int64 `json:"usage_peak"` // Peak memory usage in bytes
	SwapUsage int64 `json:"swap_usage"` // Current swap usage in bytes
}

// NetState represents network interface statistics
type NetState struct {
	Addresses []Address   `json:"addresses"`
	Counters  NetCounters `json:"counters"`
	Hwaddr    string      `json:"hwaddr"`
	Mtu       int         `json:"mtu"`
	State     string      `json:"state"`
	Type      string      `json:"type"`
}

// Address represents an IP address on an interface
type Address struct {
	Address string `json:"address"`
	Family  string `json:"family"` // inet, inet6
	Netmask string `json:"netmask"`
	Scope   string `json:"scope"`
}

// NetCounters represents network interface counters
type NetCounters struct {
	BytesReceived   int64 `json:"bytes_received"`
	BytesSent       int64 `json:"bytes_sent"`
	PacketsReceived int64 `json:"packets_received"`
	PacketsSent     int64 `json:"packets_sent"`
	ErrorsReceived  int64 `json:"errors_received"`
	ErrorsSent      int64 `json:"errors_sent"`
}

// DiskState represents disk usage statistics
type DiskState struct {
	Usage int64 `json:"usage"` // Disk usage in bytes
}

// Operation represents an LXD async operation
type Operation struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	StatusCode  int                    `json:"status_code"`
	Resources   map[string][]string    `json:"resources"`
	Metadata    map[string]interface{} `json:"metadata"`
	MayCancel   bool                   `json:"may_cancel"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Description string                 `json:"description"`
	Err         string                 `json:"err"`
}

// Snapshot represents a container snapshot
type Snapshot struct {
	Name      string            `json:"name"`
	Stateful  bool              `json:"stateful"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Config    map[string]string `json:"config"`
}

// ============================================================================
// CLIENT INTERFACE - THE FORGE MASTER
// ============================================================================

// Client defines the interface for LXD operations
type Client interface {
	// Connection management
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool
	ServerInfo(ctx context.Context) (*ServerInfo, error)

	// Container lifecycle
	CreateContainer(ctx context.Context, cfg ContainerConfig) (*Operation, error)
	DeleteContainer(ctx context.Context, name string) (*Operation, error)
	StartContainer(ctx context.Context, name string) (*Operation, error)
	StopContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error)
	RestartContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error)
	FreezeContainer(ctx context.Context, name string) (*Operation, error)
	UnfreezeContainer(ctx context.Context, name string) (*Operation, error)

	// Container information
	GetContainer(ctx context.Context, name string) (*ContainerInfo, error)
	GetContainerState(ctx context.Context, name string) (*ContainerState, error)
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
	ContainerExists(ctx context.Context, name string) (bool, error)

	// Container configuration
	UpdateContainer(ctx context.Context, name string, cfg ContainerConfig) (*Operation, error)
	RenameContainer(ctx context.Context, name, newName string) (*Operation, error)

	// Snapshots
	CreateSnapshot(ctx context.Context, container, snapshot string, stateful bool) (*Operation, error)
	DeleteSnapshot(ctx context.Context, container, snapshot string) (*Operation, error)
	RestoreSnapshot(ctx context.Context, container, snapshot string) (*Operation, error)
	ListSnapshots(ctx context.Context, container string) ([]Snapshot, error)

	// Execution
	ExecContainer(ctx context.Context, name string, cmd []string, env map[string]string) (*ExecResult, error)

	// Files
	PushFile(ctx context.Context, container, destPath string, content []byte, mode int) error
	PullFile(ctx context.Context, container, srcPath string) ([]byte, error)

	// Operations
	WaitOperation(ctx context.Context, op *Operation) error
	CancelOperation(ctx context.Context, op *Operation) error
}

// ServerInfo represents LXD server information
type ServerInfo struct {
	APIVersion  string                 `json:"api_version"`
	Auth        string                 `json:"auth"`
	Public      bool                   `json:"public"`
	AuthMethods []string               `json:"auth_methods"`
	Environment map[string]string      `json:"environment"`
	Config      map[string]interface{} `json:"config"`
}

// ExecResult represents the result of container command execution
type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// ============================================================================
// CLIENT CONFIGURATION
// ============================================================================

// ClientConfig holds configuration for creating an LXD client
type ClientConfig struct {
	Socket     string        // Unix socket path (default: /var/lib/lxd/unix.socket)
	HTTPS      string        // HTTPS endpoint for remote LXD
	TLSCert    string        // Path to client certificate
	TLSKey     string        // Path to client key
	TLSCA      string        // Path to CA certificate
	Timeout    time.Duration // Connection timeout
	SkipVerify bool          // Skip TLS verification (not recommended)
}

// ============================================================================
// MOCK CLIENT - FOR TESTING THE FORGE
// ============================================================================

// MockClient implements Client interface for testing
type MockClient struct {
	mu             sync.RWMutex
	connected      bool
	containers     map[string]*ContainerInfo
	containerState map[string]*ContainerState
	snapshots      map[string][]Snapshot
	operations     map[string]*Operation
}

// NewMockClient creates a new mock LXD client
func NewMockClient() *MockClient {
	return &MockClient{
		containers:     make(map[string]*ContainerInfo),
		containerState: make(map[string]*ContainerState),
		snapshots:      make(map[string][]Snapshot),
		operations:     make(map[string]*Operation),
	}
}

// Connect simulates connecting to LXD
func (m *MockClient) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

// Disconnect simulates disconnecting from LXD
func (m *MockClient) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

// IsConnected returns connection status
func (m *MockClient) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// ServerInfo returns mock server information
func (m *MockClient) ServerInfo(ctx context.Context) (*ServerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	return &ServerInfo{
		APIVersion:  "1.0",
		Auth:        "trusted",
		Public:      false,
		AuthMethods: []string{"tls"},
		Environment: map[string]string{
			"kernel":          "linux",
			"kernel_version":  "6.1.0",
			"server":          "lxd",
			"server_version":  "5.21",
			"storage":         "zfs",
			"storage_version": "2.2.0",
		},
	}, nil
}

// CreateContainer creates a mock container
func (m *MockClient) CreateContainer(ctx context.Context, cfg ContainerConfig) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	if cfg.Name == "" {
		return nil, ErrInvalidConfig
	}

	if _, exists := m.containers[cfg.Name]; exists {
		return nil, ErrContainerExists
	}

	m.containers[cfg.Name] = &ContainerInfo{
		Name:         cfg.Name,
		Status:       "Stopped",
		StatusCode:   102,
		Architecture: cfg.Architecture,
		CreatedAt:    time.Now(),
		Profiles:     cfg.Profiles,
		Config:       cfg.Config,
		Devices:      cfg.Devices,
		Ephemeral:    cfg.Ephemeral,
	}

	m.containerState[cfg.Name] = &ContainerState{
		Status:     "Stopped",
		StatusCode: 102,
	}

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_create",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// DeleteContainer deletes a mock container
func (m *MockClient) DeleteContainer(ctx context.Context, name string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	if _, exists := m.containers[name]; !exists {
		return nil, ErrContainerNotFound
	}

	delete(m.containers, name)
	delete(m.containerState, name)
	delete(m.snapshots, name)

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_delete",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// StartContainer starts a mock container
func (m *MockClient) StartContainer(ctx context.Context, name string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	info.Status = "Running"
	info.StatusCode = 103
	info.LastUsedAt = time.Now()

	state := m.containerState[name]
	state.Status = "Running"
	state.StatusCode = 103
	state.Pid = 1000 + int64(len(m.containers))
	state.Processes = 10

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_start",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// StopContainer stops a mock container
func (m *MockClient) StopContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	info.Status = "Stopped"
	info.StatusCode = 102

	state := m.containerState[name]
	state.Status = "Stopped"
	state.StatusCode = 102
	state.Pid = 0
	state.Processes = 0

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_stop",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// RestartContainer restarts a mock container
func (m *MockClient) RestartContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error) {
	_, err := m.StopContainer(ctx, name, force, timeout)
	if err != nil {
		return nil, err
	}
	return m.StartContainer(ctx, name)
}

// FreezeContainer freezes a mock container
func (m *MockClient) FreezeContainer(ctx context.Context, name string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	info.Status = "Frozen"
	info.StatusCode = 110

	state := m.containerState[name]
	state.Status = "Frozen"
	state.StatusCode = 110

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_freeze",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// UnfreezeContainer unfreezes a mock container
func (m *MockClient) UnfreezeContainer(ctx context.Context, name string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	info.Status = "Running"
	info.StatusCode = 103

	state := m.containerState[name]
	state.Status = "Running"
	state.StatusCode = 103

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_unfreeze",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// GetContainer returns container info
func (m *MockClient) GetContainer(ctx context.Context, name string) (*ContainerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	return info, nil
}

// GetContainerState returns container runtime state
func (m *MockClient) GetContainerState(ctx context.Context, name string) (*ContainerState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	state, exists := m.containerState[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	return state, nil
}

// ListContainers returns all containers
func (m *MockClient) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	containers := make([]ContainerInfo, 0, len(m.containers))
	for _, c := range m.containers {
		containers = append(containers, *c)
	}

	return containers, nil
}

// ContainerExists checks if container exists
func (m *MockClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return false, ErrClientNotConnected
	}

	_, exists := m.containers[name]
	return exists, nil
}

// UpdateContainer updates container configuration
func (m *MockClient) UpdateContainer(ctx context.Context, name string, cfg ContainerConfig) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	info.Config = cfg.Config
	info.Devices = cfg.Devices
	info.Profiles = cfg.Profiles

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_update",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// RenameContainer renames a container
func (m *MockClient) RenameContainer(ctx context.Context, name, newName string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	if _, exists := m.containers[newName]; exists {
		return nil, ErrContainerExists
	}

	info.Name = newName
	m.containers[newName] = info
	delete(m.containers, name)

	state := m.containerState[name]
	m.containerState[newName] = state
	delete(m.containerState, name)

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "container_rename",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// CreateSnapshot creates a container snapshot
func (m *MockClient) CreateSnapshot(ctx context.Context, container, snapshot string, stateful bool) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	if _, exists := m.containers[container]; !exists {
		return nil, ErrContainerNotFound
	}

	snap := Snapshot{
		Name:      snapshot,
		Stateful:  stateful,
		CreatedAt: time.Now(),
	}

	m.snapshots[container] = append(m.snapshots[container], snap)

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "snapshot_create",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// DeleteSnapshot deletes a snapshot
func (m *MockClient) DeleteSnapshot(ctx context.Context, container, snapshot string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	snaps, exists := m.snapshots[container]
	if !exists {
		return nil, ErrContainerNotFound
	}

	found := false
	newSnaps := make([]Snapshot, 0, len(snaps))
	for _, s := range snaps {
		if s.Name != snapshot {
			newSnaps = append(newSnaps, s)
		} else {
			found = true
		}
	}

	if !found {
		return nil, ErrSnapshotNotFound
	}

	m.snapshots[container] = newSnaps

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "snapshot_delete",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// RestoreSnapshot restores from a snapshot
func (m *MockClient) RestoreSnapshot(ctx context.Context, container, snapshot string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	snaps, exists := m.snapshots[container]
	if !exists {
		return nil, ErrContainerNotFound
	}

	found := false
	for _, s := range snaps {
		if s.Name == snapshot {
			found = true
			break
		}
	}

	if !found {
		return nil, ErrSnapshotNotFound
	}

	op := &Operation{
		ID:         fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Type:       "snapshot_restore",
		Status:     "Success",
		StatusCode: 200,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.operations[op.ID] = op

	return op, nil
}

// ListSnapshots lists container snapshots
func (m *MockClient) ListSnapshots(ctx context.Context, container string) ([]Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	if _, exists := m.containers[container]; !exists {
		return nil, ErrContainerNotFound
	}

	snaps := m.snapshots[container]
	if snaps == nil {
		snaps = []Snapshot{}
	}

	return snaps, nil
}

// ExecContainer executes a command in a container
func (m *MockClient) ExecContainer(ctx context.Context, name string, cmd []string, env map[string]string) (*ExecResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	info, exists := m.containers[name]
	if !exists {
		return nil, ErrContainerNotFound
	}

	if info.Status != "Running" {
		return &ExecResult{
			Stderr:   []byte("container not running"),
			ExitCode: 1,
		}, nil
	}

	// Mock successful execution
	return &ExecResult{
		Stdout:   []byte("command executed successfully"),
		ExitCode: 0,
	}, nil
}

// PushFile pushes a file to a container
func (m *MockClient) PushFile(ctx context.Context, container, destPath string, content []byte, mode int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return ErrClientNotConnected
	}

	if _, exists := m.containers[container]; !exists {
		return ErrContainerNotFound
	}

	// Mock successful push
	return nil
}

// PullFile pulls a file from a container
func (m *MockClient) PullFile(ctx context.Context, container, srcPath string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, ErrClientNotConnected
	}

	if _, exists := m.containers[container]; !exists {
		return nil, ErrContainerNotFound
	}

	// Mock successful pull
	return []byte("mock file content"), nil
}

// WaitOperation waits for an operation to complete
func (m *MockClient) WaitOperation(ctx context.Context, op *Operation) error {
	// Mock operations complete immediately
	return nil
}

// CancelOperation cancels an operation
func (m *MockClient) CancelOperation(ctx context.Context, op *Operation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return ErrClientNotConnected
	}

	if o, exists := m.operations[op.ID]; exists {
		o.Status = "Cancelled"
		o.StatusCode = 400
	}

	return nil
}

// ============================================================================
// CLIENT FACTORY - FORGE YOUR CONNECTION
// ============================================================================

// NewClient creates a new LXD client based on configuration
// If useMock is true, returns a MockClient for testing
func NewClient(cfg ClientConfig, useMock bool) (Client, error) {
	if useMock {
		return NewMockClient(), nil
	}
	return NewRealClient(cfg)
}

// NewRealClient creates a new real LXD client for production use
// This is a minimal implementation - see real_client.go.dev for full implementation
func NewRealClient(cfg ClientConfig) (Client, error) {
	// Set defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Socket == "" && cfg.HTTPS == "" {
		cfg.Socket = "/var/lib/lxd/unix.socket"
	}

	client := &RealClient{
		config: cfg,
	}

	return client, nil
}

// RealClient implements the Client interface for actual LXD connections
type RealClient struct {
	mu        sync.RWMutex
	connected bool
	config    ClientConfig
}

// Connect establishes connection to LXD
func (c *RealClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// TODO: Implement real connection logic
	// See real_client.go.dev for full implementation
	c.connected = true
	return nil
}

// Disconnect closes the LXD connection
func (c *RealClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	return nil
}

// IsConnected returns connection status
func (c *RealClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// ServerInfo returns LXD server information
func (c *RealClient) ServerInfo(ctx context.Context) (*ServerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &ServerInfo{
		APIVersion: "1.0",
		Auth:       "trusted",
	}, nil
}

// CreateContainer creates a new container
func (c *RealClient) CreateContainer(ctx context.Context, cfg ContainerConfig) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// DeleteContainer deletes a container
func (c *RealClient) DeleteContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// StartContainer starts a container
func (c *RealClient) StartContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// StopContainer stops a container
func (c *RealClient) StopContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// RestartContainer restarts a container
func (c *RealClient) RestartContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// FreezeContainer freezes a container
func (c *RealClient) FreezeContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// UnfreezeContainer unfreezes a container
func (c *RealClient) UnfreezeContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// GetContainer returns container information
func (c *RealClient) GetContainer(ctx context.Context, name string) (*ContainerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return nil, ErrContainerNotFound
}

// GetContainerState returns container runtime state
func (c *RealClient) GetContainerState(ctx context.Context, name string) (*ContainerState, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return nil, ErrContainerNotFound
}

// ListContainers returns all containers
func (c *RealClient) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return []ContainerInfo{}, nil
}

// ContainerExists checks if a container exists
func (c *RealClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	if !c.IsConnected() {
		return false, ErrClientNotConnected
	}
	return false, nil
}

// UpdateContainer updates container configuration
func (c *RealClient) UpdateContainer(ctx context.Context, name string, cfg ContainerConfig) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// RenameContainer renames a container
func (c *RealClient) RenameContainer(ctx context.Context, name, newName string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// CreateSnapshot creates a container snapshot
func (c *RealClient) CreateSnapshot(ctx context.Context, container, snapshot string, stateful bool) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// DeleteSnapshot deletes a snapshot
func (c *RealClient) DeleteSnapshot(ctx context.Context, container, snapshot string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// RestoreSnapshot restores from a snapshot
func (c *RealClient) RestoreSnapshot(ctx context.Context, container, snapshot string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &Operation{
		ID:     fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), opIDCounter.Add(1)),
		Status: "Running",
	}, nil
}

// ListSnapshots lists container snapshots
func (c *RealClient) ListSnapshots(ctx context.Context, container string) ([]Snapshot, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return []Snapshot{}, nil
}

// ExecContainer executes a command in a container
func (c *RealClient) ExecContainer(ctx context.Context, name string, cmd []string, env map[string]string) (*ExecResult, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return &ExecResult{
		ExitCode: 0,
	}, nil
}

// PushFile pushes a file to a container
func (c *RealClient) PushFile(ctx context.Context, container, destPath string, content []byte, mode int) error {
	if !c.IsConnected() {
		return ErrClientNotConnected
	}
	return nil
}

// PullFile pulls a file from a container
func (c *RealClient) PullFile(ctx context.Context, container, srcPath string) ([]byte, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}
	return nil, nil
}

// WaitOperation waits for an async operation to complete
func (c *RealClient) WaitOperation(ctx context.Context, op *Operation) error {
	if !c.IsConnected() {
		return ErrClientNotConnected
	}
	return nil
}

// CancelOperation cancels an async operation
func (c *RealClient) CancelOperation(ctx context.Context, op *Operation) error {
	if !c.IsConnected() {
		return ErrClientNotConnected
	}
	return nil
}

// Ensure RealClient implements Client interface
var _ Client = (*RealClient)(nil)

// Ensure MockClient implements Client interface
var _ Client = (*MockClient)(nil)
