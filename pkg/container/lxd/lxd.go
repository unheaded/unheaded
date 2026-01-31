// Package lxd provides the LXD backend for the container runtime interface
package lxd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/unheaded/unheaded/pkg/container"
	"github.com/unheaded/unheaded/pkg/lxd"
)

// ============================================================================
// LXD RUNTIME - THE PRIMARY BACKEND
// ============================================================================

// Runtime implements the container.Runtime interface using LXD as the backend
type Runtime struct {
	mu     sync.RWMutex
	client lxd.Client
	config Config
	logger zerolog.Logger

	// Health check tracking
	healthMu     sync.RWMutex
	healthStatus map[string]*container.HealthStatus
	healthCtx    context.Context
	healthCancel context.CancelFunc

	// Stats streaming
	statsStreams map[string]chan *container.Stats
	streamsMu    sync.RWMutex
}

// Config holds configuration for the LXD runtime
type Config struct {
	// Connection settings
	Socket     string        `json:"socket,omitempty" yaml:"socket,omitempty"`
	HTTPS      string        `json:"https,omitempty" yaml:"https,omitempty"`
	TLSCert    string        `json:"tls_cert,omitempty" yaml:"tls_cert,omitempty"`
	TLSKey     string        `json:"tls_key,omitempty" yaml:"tls_key,omitempty"`
	TLSCA      string        `json:"tls_ca,omitempty" yaml:"tls_ca,omitempty"`
	SkipVerify bool          `json:"skip_verify,omitempty" yaml:"skip_verify,omitempty"`
	Timeout    time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// Default settings
	DefaultProfiles []string `json:"default_profiles,omitempty" yaml:"default_profiles,omitempty"`
	DefaultImage    string   `json:"default_image,omitempty" yaml:"default_image,omitempty"`

	// Health check settings
	HealthCheckInterval time.Duration `json:"health_check_interval,omitempty" yaml:"health_check_interval,omitempty"`
	HealthCheckRetries  int           `json:"health_check_retries,omitempty" yaml:"health_check_retries,omitempty"`

	// Use mock client for testing
	UseMock bool `json:"-" yaml:"-"`
}

// New creates a new LXD runtime instance
func New(cfg Config) (*Runtime, error) {
	logger := log.With().Str("component", "lxd-runtime").Logger()

	// Set defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if len(cfg.DefaultProfiles) == 0 {
		cfg.DefaultProfiles = []string{"default"}
	}
	if cfg.HealthCheckInterval == 0 {
		cfg.HealthCheckInterval = 30 * time.Second
	}
	if cfg.HealthCheckRetries == 0 {
		cfg.HealthCheckRetries = 3
	}

	// Create LXD client
	client, err := lxd.NewClient(lxd.ClientConfig{
		Socket:     cfg.Socket,
		HTTPS:      cfg.HTTPS,
		TLSCert:    cfg.TLSCert,
		TLSKey:     cfg.TLSKey,
		TLSCA:      cfg.TLSCA,
		SkipVerify: cfg.SkipVerify,
		Timeout:    cfg.Timeout,
	}, cfg.UseMock)
	if err != nil {
		return nil, fmt.Errorf("create LXD client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	runtime := &Runtime{
		client:       client,
		config:       cfg,
		logger:       logger,
		healthStatus: make(map[string]*container.HealthStatus),
		healthCtx:    ctx,
		healthCancel: cancel,
		statsStreams: make(map[string]chan *container.Stats),
	}

	return runtime, nil
}

// Connect establishes connection to LXD
func (r *Runtime) Connect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.client.Connect(ctx); err != nil {
		return fmt.Errorf("connect to LXD: %w", err)
	}

	r.logger.Info().Msg("connected to LXD")
	return nil
}

// Close closes the runtime connection
func (r *Runtime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Cancel health check goroutines
	r.healthCancel()

	// Close stats streams
	r.streamsMu.Lock()
	for id, ch := range r.statsStreams {
		close(ch)
		delete(r.statsStreams, id)
	}
	r.streamsMu.Unlock()

	if err := r.client.Disconnect(); err != nil {
		return fmt.Errorf("disconnect from LXD: %w", err)
	}

	r.logger.Info().Msg("disconnected from LXD")
	return nil
}

// ============================================================================
// LIFECYCLE OPERATIONS
// ============================================================================

// Create creates a new container from the given specification
func (r *Runtime) Create(ctx context.Context, spec *container.ContainerSpec) (*container.Container, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if spec == nil {
		return nil, container.ErrInvalidSpec
	}

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	r.logger.Info().Str("name", spec.Name).Str("image", spec.Image).Msg("creating container")

	// Convert spec to LXD config
	lxdCfg := r.specToLXDConfig(spec)

	// Create container
	op, err := r.client.CreateContainer(ctx, lxdCfg)
	if err != nil {
		if err == lxd.ErrContainerExists {
			return nil, container.ErrContainerExists
		}
		return nil, fmt.Errorf("create container: %w", err)
	}

	// Wait for operation to complete
	if err := r.client.WaitOperation(ctx, op); err != nil {
		return nil, fmt.Errorf("wait for create: %w", err)
	}

	// Get the created container
	return r.Get(ctx, spec.Name)
}

// Start starts a stopped container
func (r *Runtime) Start(ctx context.Context, id string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info().Str("id", id).Msg("starting container")

	op, err := r.client.StartContainer(ctx, id)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return container.ErrContainerNotFound
		}
		return fmt.Errorf("start container: %w", err)
	}

	if err := r.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait for start: %w", err)
	}

	return nil
}

// Stop stops a running container
func (r *Runtime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info().Str("id", id).Dur("timeout", timeout).Msg("stopping container")

	timeoutSec := int(timeout.Seconds())
	if timeoutSec < 0 {
		timeoutSec = 30
	}

	op, err := r.client.StopContainer(ctx, id, false, timeoutSec)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return container.ErrContainerNotFound
		}
		return fmt.Errorf("stop container: %w", err)
	}

	if err := r.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait for stop: %w", err)
	}

	return nil
}

// Restart restarts a container
func (r *Runtime) Restart(ctx context.Context, id string, timeout time.Duration) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info().Str("id", id).Dur("timeout", timeout).Msg("restarting container")

	timeoutSec := int(timeout.Seconds())
	if timeoutSec < 0 {
		timeoutSec = 30
	}

	op, err := r.client.RestartContainer(ctx, id, false, timeoutSec)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return container.ErrContainerNotFound
		}
		return fmt.Errorf("restart container: %w", err)
	}

	if err := r.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait for restart: %w", err)
	}

	return nil
}

// Pause pauses a running container
func (r *Runtime) Pause(ctx context.Context, id string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info().Str("id", id).Msg("pausing container")

	op, err := r.client.FreezeContainer(ctx, id)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return container.ErrContainerNotFound
		}
		return fmt.Errorf("pause container: %w", err)
	}

	if err := r.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait for pause: %w", err)
	}

	return nil
}

// Unpause unpauses a paused container
func (r *Runtime) Unpause(ctx context.Context, id string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info().Str("id", id).Msg("unpausing container")

	op, err := r.client.UnfreezeContainer(ctx, id)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return container.ErrContainerNotFound
		}
		return fmt.Errorf("unpause container: %w", err)
	}

	if err := r.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait for unpause: %w", err)
	}

	return nil
}

// Delete removes a container
func (r *Runtime) Delete(ctx context.Context, id string, force bool) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info().Str("id", id).Bool("force", force).Msg("deleting container")

	// If force, stop the container first
	if force {
		// Try to stop, ignore errors
		stopOp, err := r.client.StopContainer(ctx, id, true, 10)
		if err == nil {
			_ = r.client.WaitOperation(ctx, stopOp)
		}
	}

	op, err := r.client.DeleteContainer(ctx, id)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return container.ErrContainerNotFound
		}
		return fmt.Errorf("delete container: %w", err)
	}

	if err := r.client.WaitOperation(ctx, op); err != nil {
		return fmt.Errorf("wait for delete: %w", err)
	}

	// Clean up health status
	r.healthMu.Lock()
	delete(r.healthStatus, id)
	r.healthMu.Unlock()

	return nil
}

// ============================================================================
// CONTAINER INFORMATION
// ============================================================================

// Get retrieves a container by ID or name
func (r *Runtime) Get(ctx context.Context, id string) (*container.Container, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, err := r.client.GetContainer(ctx, id)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return nil, container.ErrContainerNotFound
		}
		return nil, fmt.Errorf("get container: %w", err)
	}

	state, err := r.client.GetContainerState(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get container state: %w", err)
	}

	return r.convertContainer(info, state), nil
}

// List lists containers matching the filter
func (r *Runtime) List(ctx context.Context, filter *container.Filter) ([]*container.Container, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lxdContainers, err := r.client.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	containers := make([]*container.Container, 0, len(lxdContainers))
	for _, lxdC := range lxdContainers {
		state, err := r.client.GetContainerState(ctx, lxdC.Name)
		if err != nil {
			r.logger.Warn().Err(err).Str("name", lxdC.Name).Msg("failed to get container state")
			continue
		}

		c := r.convertContainer(&lxdC, state)
		if c.Match(filter) {
			containers = append(containers, c)
		}
	}

	return containers, nil
}

// Exists checks if a container exists
func (r *Runtime) Exists(ctx context.Context, id string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.client.ContainerExists(ctx, id)
}

// ============================================================================
// CONTAINER INTERACTION
// ============================================================================

// Exec executes a command in a container
func (r *Runtime) Exec(ctx context.Context, id string, cmd *container.ExecConfig) (*container.ExecResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if cmd == nil || len(cmd.Command) == 0 {
		return nil, fmt.Errorf("%w: command is required", container.ErrInvalidSpec)
	}

	r.logger.Debug().Str("id", id).Strs("cmd", cmd.Command).Msg("executing command")

	result, err := r.client.ExecContainer(ctx, id, cmd.Command, cmd.Env)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return nil, container.ErrContainerNotFound
		}
		return nil, fmt.Errorf("exec command: %w", err)
	}

	return &container.ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

// Logs retrieves container logs
func (r *Runtime) Logs(ctx context.Context, id string, opts *container.LogOptions) (io.ReadCloser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// LXD doesn't have native log streaming like Docker
	// We implement it by reading from /var/log or systemd journal

	if opts == nil {
		opts = &container.LogOptions{
			Stdout: true,
			Stderr: true,
		}
	}

	// Check if container exists
	exists, err := r.client.ContainerExists(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, container.ErrContainerNotFound
	}

	// Build journalctl command for log retrieval
	cmd := []string{"journalctl", "--no-pager"}

	if opts.Tail > 0 {
		cmd = append(cmd, "-n", strconv.Itoa(opts.Tail))
	}

	if opts.Follow {
		cmd = append(cmd, "-f")
	}

	if !opts.Since.IsZero() {
		cmd = append(cmd, "--since", opts.Since.Format("2006-01-02 15:04:05"))
	}

	if !opts.Until.IsZero() {
		cmd = append(cmd, "--until", opts.Until.Format("2006-01-02 15:04:05"))
	}

	result, err := r.client.ExecContainer(ctx, id, cmd, nil)
	if err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}

	// Combine stdout and stderr
	var buf bytes.Buffer
	if opts.Stdout {
		buf.Write(result.Stdout)
	}
	if opts.Stderr {
		buf.Write(result.Stderr)
	}

	return io.NopCloser(&buf), nil
}

// Attach attaches to a running container
func (r *Runtime) Attach(ctx context.Context, id string, opts *container.AttachOptions) (*container.AttachResult, error) {
	// LXD attach requires WebSocket, which is complex to implement
	// For now, return an error indicating this is not fully supported
	return nil, fmt.Errorf("attach not fully supported for LXD backend - use Exec instead")
}

// ============================================================================
// RESOURCE MONITORING
// ============================================================================

// Stats retrieves container resource statistics
func (r *Runtime) Stats(ctx context.Context, id string) (*container.Stats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, err := r.client.GetContainerState(ctx, id)
	if err != nil {
		if err == lxd.ErrContainerNotFound {
			return nil, container.ErrContainerNotFound
		}
		return nil, fmt.Errorf("get container state: %w", err)
	}

	return r.convertStats(id, state), nil
}

// StatsStream returns a channel that receives container stats periodically
func (r *Runtime) StatsStream(ctx context.Context, id string) (<-chan *container.Stats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if container exists
	exists, err := r.client.ContainerExists(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, container.ErrContainerNotFound
	}

	// Create stats channel
	statsChan := make(chan *container.Stats, 10)

	r.streamsMu.Lock()
	r.statsStreams[id] = statsChan
	r.streamsMu.Unlock()

	// Start stats collection goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer func() {
			r.streamsMu.Lock()
			delete(r.statsStreams, id)
			r.streamsMu.Unlock()
			close(statsChan)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats, err := r.Stats(ctx, id)
				if err != nil {
					r.logger.Warn().Err(err).Str("id", id).Msg("failed to collect stats")
					continue
				}

				select {
				case statsChan <- stats:
				default:
					// Channel full, skip this update
				}
			}
		}
	}()

	return statsChan, nil
}

// ============================================================================
// HEALTH CHECKING
// ============================================================================

// HealthCheck performs a health check on a container
func (r *Runtime) HealthCheck(ctx context.Context, id string) (*container.HealthStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get container to check if it has health check configured
	c, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if c.Spec == nil || c.Spec.HealthCheck == nil {
		return &container.HealthStatus{
			Status: container.HealthStateNone,
		}, nil
	}

	// Check if container is running
	if c.State != container.StateRunning {
		return &container.HealthStatus{
			Status: container.HealthStateUnhealthy,
		}, nil
	}

	// Execute health check command
	result, err := r.client.ExecContainer(ctx, id, c.Spec.HealthCheck.Command, nil)
	if err != nil {
		return &container.HealthStatus{
			Status: container.HealthStateUnhealthy,
			Log: []container.HealthLog{
				{
					Start:    time.Now(),
					End:      time.Now(),
					ExitCode: -1,
					Output:   err.Error(),
				},
			},
		}, nil
	}

	// Update health status
	healthLog := container.HealthLog{
		Start:    time.Now(),
		End:      time.Now(),
		ExitCode: result.ExitCode,
		Output:   string(result.Stdout),
	}

	r.healthMu.Lock()
	defer r.healthMu.Unlock()

	status, exists := r.healthStatus[id]
	if !exists {
		status = &container.HealthStatus{
			Status: container.HealthStateStarting,
		}
		r.healthStatus[id] = status
	}

	status.Log = append(status.Log, healthLog)
	if len(status.Log) > 10 {
		status.Log = status.Log[len(status.Log)-10:]
	}

	if result.ExitCode == 0 {
		status.Status = container.HealthStateHealthy
		status.FailingStreak = 0
	} else {
		status.FailingStreak++
		if status.FailingStreak >= r.config.HealthCheckRetries {
			status.Status = container.HealthStateUnhealthy
		}
	}

	return status, nil
}

// ============================================================================
// RUNTIME INFORMATION
// ============================================================================

// Info returns information about the LXD runtime
func (r *Runtime) Info(ctx context.Context) (*container.RuntimeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	serverInfo, err := r.client.ServerInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get server info: %w", err)
	}

	// Get container counts
	containers, err := r.client.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	counts := container.RuntimeCounts{
		Total: len(containers),
	}

	for _, c := range containers {
		switch c.Status {
		case "Running":
			counts.Running++
		case "Frozen":
			counts.Paused++
		case "Stopped":
			counts.Stopped++
		}
	}

	info := &container.RuntimeInfo{
		Backend:    container.BackendLXD,
		Version:    serverInfo.Environment["server_version"],
		APIVersion: serverInfo.APIVersion,
		OS:         serverInfo.Environment["kernel"],
		Arch:       serverInfo.Environment["kernel_architecture"],
		Driver:     serverInfo.Environment["storage"],
		Containers: counts,
	}

	if kernelVer, ok := serverInfo.Environment["kernel_version"]; ok {
		info.KernelVersion = kernelVer
	}

	return info, nil
}

// Version returns version information about the LXD runtime
func (r *Runtime) Version(ctx context.Context) (*container.RuntimeVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	serverInfo, err := r.client.ServerInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get server info: %w", err)
	}

	version := &container.RuntimeVersion{
		Version:    serverInfo.Environment["server_version"],
		APIVersion: serverInfo.APIVersion,
		OS:         serverInfo.Environment["kernel"],
		Arch:       serverInfo.Environment["kernel_architecture"],
		Components: make(map[string]string),
	}

	if kernelVer, ok := serverInfo.Environment["kernel_version"]; ok {
		version.KernelVersion = kernelVer
	}

	// Add components
	if storageVer, ok := serverInfo.Environment["storage_version"]; ok {
		version.Components["storage"] = storageVer
	}

	return version, nil
}

// ============================================================================
// CONVERSION HELPERS
// ============================================================================

// specToLXDConfig converts a container spec to LXD config
func (r *Runtime) specToLXDConfig(spec *container.ContainerSpec) lxd.ContainerConfig {
	cfg := lxd.ContainerConfig{
		Name:      spec.Name,
		Image:     spec.Image,
		Ephemeral: spec.Ephemeral,
		Profiles:  spec.Profiles,
		Config:    make(map[string]string),
		Devices:   make(map[string]lxd.Device),
	}

	// Set default profiles if none specified
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = r.config.DefaultProfiles
	}

	// Environment variables
	for k, v := range spec.Env {
		cfg.Config["environment."+k] = v
	}

	// Resource limits
	if spec.Resources.Limits.CPU != "" {
		cfg.Config["limits.cpu"] = spec.Resources.Limits.CPU
	}
	if spec.Resources.Limits.Memory != "" {
		cfg.Config["limits.memory"] = spec.Resources.Limits.Memory
	}
	if spec.Resources.Limits.PidsLimit > 0 {
		cfg.Config["limits.processes"] = strconv.FormatInt(spec.Resources.Limits.PidsLimit, 10)
	}

	// Security settings
	if spec.Security.Privileged {
		cfg.Config["security.privileged"] = "true"
	}
	if spec.Security.NoNewPrivileges {
		cfg.Config["security.syscalls.deny_default"] = "true"
	}

	// Mounts
	for i, mount := range spec.Mounts {
		devName := fmt.Sprintf("disk%d", i)
		dev := lxd.Device{
			Type:       "disk",
			Properties: make(map[string]string),
		}
		dev.Properties["source"] = mount.Source
		dev.Properties["path"] = mount.Target
		if mount.ReadOnly {
			dev.Properties["readonly"] = "true"
		}
		cfg.Devices[devName] = dev
	}

	// Network configuration
	if spec.Network.Mode == container.NetworkModeManaged {
		dev := lxd.Device{
			Type:       "nic",
			Properties: make(map[string]string),
		}
		dev.Properties["nictype"] = "bridged"
		dev.Properties["parent"] = "lxdbr0"
		if spec.Network.IPAddress != "" {
			dev.Properties["ipv4.address"] = spec.Network.IPAddress
		}
		if spec.Network.MacAddress != "" {
			dev.Properties["hwaddr"] = spec.Network.MacAddress
		}
		cfg.Devices["eth0"] = dev
	}

	return cfg
}

// convertContainer converts LXD container info to unified Container
func (r *Runtime) convertContainer(info *lxd.ContainerInfo, state *lxd.ContainerState) *container.Container {
	c := &container.Container{
		ID:        info.Name,
		Name:      info.Name,
		State:     r.convertState(state.Status),
		Status:    state.Status,
		CreatedAt: info.CreatedAt,
		UpdatedAt: info.LastUsedAt,
		PID:       state.Pid,
		Backend:   container.BackendLXD,
		Labels:    make(map[string]string),
		Network:   make(map[string]container.NetworkInfo),
	}

	// Extract image from config
	if image, ok := info.Config["image.description"]; ok {
		c.Image = image
	} else if image, ok := info.Config["volatile.base_image"]; ok {
		c.Image = image
	}

	// Convert network info
	for iface, netState := range state.Network {
		netInfo := container.NetworkInfo{
			NetworkID:  iface,
			MacAddress: netState.Hwaddr,
		}
		if len(netState.Addresses) > 0 {
			for _, addr := range netState.Addresses {
				if addr.Family == "inet" {
					netInfo.IPAddress = addr.Address
					break
				}
			}
		}
		c.Network[iface] = netInfo
	}

	// Convert labels from config
	labelPrefix := "user."
	for k, v := range info.Config {
		if strings.HasPrefix(k, labelPrefix) {
			c.Labels[strings.TrimPrefix(k, labelPrefix)] = v
		}
	}

	// Store backend-specific data
	c.BackendData = map[string]interface{}{
		"profiles":     info.Profiles,
		"ephemeral":    info.Ephemeral,
		"architecture": info.Architecture,
		"devices":      info.Devices,
	}

	return c
}

// convertState converts LXD status to unified ContainerState
func (r *Runtime) convertState(status string) container.ContainerState {
	switch strings.ToLower(status) {
	case "running":
		return container.StateRunning
	case "stopped":
		return container.StateStopped
	case "frozen":
		return container.StatePaused
	case "error":
		return container.StateDead
	case "starting":
		return container.StateRestarting
	case "stopping":
		return container.StateStopped
	default:
		return container.StateUnknown
	}
}

// convertStats converts LXD state to unified Stats
func (r *Runtime) convertStats(id string, state *lxd.ContainerState) *container.Stats {
	stats := &container.Stats{
		ContainerID: id,
		Timestamp:   time.Now(),
		CPU: container.CPUStats{
			UsageNanos: uint64(state.CPU.Usage),
		},
		Memory: container.MemoryStats{
			Usage:    uint64(state.Memory.Usage),
			MaxUsage: uint64(state.Memory.UsagePeak),
			Swap:     uint64(state.Memory.SwapUsage),
		},
		Network: make(map[string]container.NetworkStats),
		PIDs:    state.Processes,
	}

	// Convert network stats
	for iface, netState := range state.Network {
		stats.Network[iface] = container.NetworkStats{
			RxBytes:   uint64(netState.Counters.BytesReceived),
			RxPackets: uint64(netState.Counters.PacketsReceived),
			RxErrors:  uint64(netState.Counters.ErrorsReceived),
			TxBytes:   uint64(netState.Counters.BytesSent),
			TxPackets: uint64(netState.Counters.PacketsSent),
			TxErrors:  uint64(netState.Counters.ErrorsSent),
		}
	}

	// Convert disk stats
	for _, diskState := range state.Disk {
		stats.BlockIO.ReadBytes += uint64(diskState.Usage)
	}

	return stats
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// parseMemoryString parses a memory string like "1Gi" or "512Mi" to bytes
func parseMemoryString(mem string) (uint64, error) {
	if mem == "" {
		return 0, nil
	}

	re := regexp.MustCompile(`^(\d+)([KMGT]?)i?[Bb]?$`)
	matches := re.FindStringSubmatch(mem)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid memory format: %s", mem)
	}

	value, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return 0, err
	}

	switch strings.ToUpper(matches[2]) {
	case "K":
		return value * 1024, nil
	case "M":
		return value * 1024 * 1024, nil
	case "G":
		return value * 1024 * 1024 * 1024, nil
	case "T":
		return value * 1024 * 1024 * 1024 * 1024, nil
	default:
		return value, nil
	}
}

// Ensure Runtime implements container.Runtime interface
var _ container.Runtime = (*Runtime)(nil)
