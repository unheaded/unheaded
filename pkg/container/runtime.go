// Package container provides a unified container runtime interface for Unheaded
// Supports multiple backends: LXD (primary), Docker, Podman
package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// ============================================================================
// ERRORS - CONTAINER RUNTIME FAILURES
// ============================================================================

var (
	ErrRuntimeNotInitialized = errors.New("runtime not initialized")
	ErrContainerNotFound     = errors.New("container not found")
	ErrContainerExists       = errors.New("container already exists")
	ErrContainerNotRunning   = errors.New("container not running")
	ErrContainerRunning      = errors.New("container is running")
	ErrInvalidSpec           = errors.New("invalid container specification")
	ErrImageNotFound         = errors.New("image not found")
	ErrNetworkNotFound       = errors.New("network not found")
	ErrVolumeNotFound        = errors.New("volume not found")
	ErrOperationTimeout      = errors.New("operation timed out")
	ErrExecFailed            = errors.New("command execution failed")
	ErrHealthCheckFailed     = errors.New("health check failed")
	ErrUnsupportedBackend    = errors.New("unsupported backend")
)

// ============================================================================
// RUNTIME INTERFACE - THE UNIFIED ABSTRACTION
// ============================================================================

// Runtime defines the unified interface for container operations
// All backends (LXD, Docker, Podman) implement this interface
type Runtime interface {
	// Lifecycle operations
	Create(ctx context.Context, spec *ContainerSpec) (*Container, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string, timeout time.Duration) error
	Restart(ctx context.Context, id string, timeout time.Duration) error
	Pause(ctx context.Context, id string) error
	Unpause(ctx context.Context, id string) error
	Delete(ctx context.Context, id string, force bool) error

	// Container information
	Get(ctx context.Context, id string) (*Container, error)
	List(ctx context.Context, filter *Filter) ([]*Container, error)
	Exists(ctx context.Context, id string) (bool, error)

	// Container interaction
	Exec(ctx context.Context, id string, cmd *ExecConfig) (*ExecResult, error)
	Logs(ctx context.Context, id string, opts *LogOptions) (io.ReadCloser, error)
	Attach(ctx context.Context, id string, opts *AttachOptions) (*AttachResult, error)

	// Resource monitoring
	Stats(ctx context.Context, id string) (*Stats, error)
	StatsStream(ctx context.Context, id string) (<-chan *Stats, error)

	// Health checking
	HealthCheck(ctx context.Context, id string) (*HealthStatus, error)

	// Runtime information
	Info(ctx context.Context) (*RuntimeInfo, error)
	Version(ctx context.Context) (*RuntimeVersion, error)

	// Connection management
	Close() error
}

// ============================================================================
// CONTAINER SPECIFICATION - DECLARATIVE CONTAINER DEFINITION
// ============================================================================

// ContainerSpec defines the desired state for a container
type ContainerSpec struct {
	// Basic identification
	Name        string            `json:"name" yaml:"name"`
	Image       string            `json:"image" yaml:"image"`
	Hostname    string            `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Command and entrypoint
	Command    []string `json:"command,omitempty" yaml:"command,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`

	// Environment
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	EnvFrom []EnvSource       `json:"env_from,omitempty" yaml:"env_from,omitempty"`

	// Resource limits
	Resources ResourceSpec `json:"resources,omitempty" yaml:"resources,omitempty"`

	// Networking
	Network     NetworkSpec `json:"network,omitempty" yaml:"network,omitempty"`
	ExposedPorts []PortSpec `json:"exposed_ports,omitempty" yaml:"exposed_ports,omitempty"`

	// Storage
	Mounts  []MountSpec  `json:"mounts,omitempty" yaml:"mounts,omitempty"`
	Volumes []VolumeSpec `json:"volumes,omitempty" yaml:"volumes,omitempty"`

	// Security
	Security SecuritySpec `json:"security,omitempty" yaml:"security,omitempty"`

	// Health check
	HealthCheck *HealthCheckSpec `json:"health_check,omitempty" yaml:"health_check,omitempty"`

	// Restart policy
	RestartPolicy RestartPolicy `json:"restart_policy,omitempty" yaml:"restart_policy,omitempty"`

	// Backend-specific configuration
	BackendConfig map[string]interface{} `json:"backend_config,omitempty" yaml:"backend_config,omitempty"`

	// LXD-specific fields
	Profiles  []string `json:"profiles,omitempty" yaml:"profiles,omitempty"`
	Ephemeral bool     `json:"ephemeral,omitempty" yaml:"ephemeral,omitempty"`
}

// EnvSource defines a source for environment variables
type EnvSource struct {
	ConfigMap string `json:"config_map,omitempty" yaml:"config_map,omitempty"`
	Secret    string `json:"secret,omitempty" yaml:"secret,omitempty"`
	File      string `json:"file,omitempty" yaml:"file,omitempty"`
}

// ResourceSpec defines resource limits and requests
type ResourceSpec struct {
	Limits   ResourceLimits `json:"limits,omitempty" yaml:"limits,omitempty"`
	Requests ResourceLimits `json:"requests,omitempty" yaml:"requests,omitempty"`
}

// ResourceLimits defines specific resource constraints
type ResourceLimits struct {
	CPU       string `json:"cpu,omitempty" yaml:"cpu,omitempty"`             // e.g., "2" or "500m"
	Memory    string `json:"memory,omitempty" yaml:"memory,omitempty"`       // e.g., "1Gi" or "512Mi"
	DiskRead  string `json:"disk_read,omitempty" yaml:"disk_read,omitempty"` // IOPS or bandwidth
	DiskWrite string `json:"disk_write,omitempty" yaml:"disk_write,omitempty"`
	PidsLimit int64  `json:"pids_limit,omitempty" yaml:"pids_limit,omitempty"`
}

// NetworkSpec defines network configuration
type NetworkSpec struct {
	Mode       NetworkMode       `json:"mode,omitempty" yaml:"mode,omitempty"`
	Networks   []string          `json:"networks,omitempty" yaml:"networks,omitempty"`
	IPAddress  string            `json:"ip_address,omitempty" yaml:"ip_address,omitempty"`
	MacAddress string            `json:"mac_address,omitempty" yaml:"mac_address,omitempty"`
	DNS        DNSSpec           `json:"dns,omitempty" yaml:"dns,omitempty"`
	ExtraHosts map[string]string `json:"extra_hosts,omitempty" yaml:"extra_hosts,omitempty"`
}

// NetworkMode defines how a container connects to networks
type NetworkMode string

const (
	NetworkModeDefault NetworkMode = ""
	NetworkModeBridge  NetworkMode = "bridge"
	NetworkModeHost    NetworkMode = "host"
	NetworkModeNone    NetworkMode = "none"
	NetworkModeManaged NetworkMode = "managed"
)

// DNSSpec defines DNS configuration
type DNSSpec struct {
	Servers  []string `json:"servers,omitempty" yaml:"servers,omitempty"`
	Search   []string `json:"search,omitempty" yaml:"search,omitempty"`
	Options  []string `json:"options,omitempty" yaml:"options,omitempty"`
}

// PortSpec defines an exposed port
type PortSpec struct {
	ContainerPort int    `json:"container_port" yaml:"container_port"`
	HostPort      int    `json:"host_port,omitempty" yaml:"host_port,omitempty"`
	Protocol      string `json:"protocol,omitempty" yaml:"protocol,omitempty"` // tcp, udp
	HostIP        string `json:"host_ip,omitempty" yaml:"host_ip,omitempty"`
}

// MountSpec defines a filesystem mount
type MountSpec struct {
	Type        MountType `json:"type" yaml:"type"`
	Source      string    `json:"source" yaml:"source"`
	Target      string    `json:"target" yaml:"target"`
	ReadOnly    bool      `json:"read_only,omitempty" yaml:"read_only,omitempty"`
	Propagation string    `json:"propagation,omitempty" yaml:"propagation,omitempty"`
}

// MountType defines the type of mount
type MountType string

const (
	MountTypeBind   MountType = "bind"
	MountTypeVolume MountType = "volume"
	MountTypeTmpfs  MountType = "tmpfs"
	MountTypeDisk   MountType = "disk"
)

// VolumeSpec defines a volume to be created
type VolumeSpec struct {
	Name   string            `json:"name" yaml:"name"`
	Driver string            `json:"driver,omitempty" yaml:"driver,omitempty"`
	Size   string            `json:"size,omitempty" yaml:"size,omitempty"`
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// SecuritySpec defines security configuration
type SecuritySpec struct {
	Privileged       bool     `json:"privileged,omitempty" yaml:"privileged,omitempty"`
	ReadOnlyRootfs   bool     `json:"read_only_rootfs,omitempty" yaml:"read_only_rootfs,omitempty"`
	NoNewPrivileges  bool     `json:"no_new_privileges,omitempty" yaml:"no_new_privileges,omitempty"`
	User             string   `json:"user,omitempty" yaml:"user,omitempty"`
	Group            string   `json:"group,omitempty" yaml:"group,omitempty"`
	Capabilities     CapSpec  `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	SeccompProfile   string   `json:"seccomp_profile,omitempty" yaml:"seccomp_profile,omitempty"`
	AppArmorProfile  string   `json:"apparmor_profile,omitempty" yaml:"apparmor_profile,omitempty"`
	SELinuxOptions   *SELinux `json:"selinux_options,omitempty" yaml:"selinux_options,omitempty"`
}

// CapSpec defines Linux capabilities
type CapSpec struct {
	Add  []string `json:"add,omitempty" yaml:"add,omitempty"`
	Drop []string `json:"drop,omitempty" yaml:"drop,omitempty"`
}

// SELinux defines SELinux options
type SELinux struct {
	User  string `json:"user,omitempty" yaml:"user,omitempty"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
	Type  string `json:"type,omitempty" yaml:"type,omitempty"`
	Level string `json:"level,omitempty" yaml:"level,omitempty"`
}

// HealthCheckSpec defines health check configuration
type HealthCheckSpec struct {
	Command     []string      `json:"command" yaml:"command"`
	Interval    time.Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StartPeriod time.Duration `json:"start_period,omitempty" yaml:"start_period,omitempty"`
	Retries     int           `json:"retries,omitempty" yaml:"retries,omitempty"`
}

// RestartPolicy defines container restart behavior
type RestartPolicy struct {
	Policy     RestartPolicyType `json:"policy" yaml:"policy"`
	MaxRetries int               `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
}

// RestartPolicyType defines the type of restart policy
type RestartPolicyType string

const (
	RestartPolicyNone      RestartPolicyType = "none"
	RestartPolicyAlways    RestartPolicyType = "always"
	RestartPolicyOnFailure RestartPolicyType = "on-failure"
	RestartPolicyUnlessStopped RestartPolicyType = "unless-stopped"
)

// ============================================================================
// CONTAINER STATE - ACTUAL RUNTIME STATE
// ============================================================================

// Container represents a container's current state
type Container struct {
	// Identification
	ID          string `json:"id"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	ImageDigest string `json:"image_digest,omitempty"`

	// State
	State      ContainerState `json:"state"`
	Status     string         `json:"status"`
	ExitCode   int            `json:"exit_code,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	// Configuration
	Spec *ContainerSpec `json:"spec,omitempty"`

	// Runtime information
	PID       int64                    `json:"pid,omitempty"`
	Platform  string                   `json:"platform,omitempty"`
	Network   map[string]NetworkInfo   `json:"network,omitempty"`
	Mounts    []MountInfo              `json:"mounts,omitempty"`

	// Labels and annotations
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`

	// Backend-specific metadata
	Backend     BackendType            `json:"backend"`
	BackendData map[string]interface{} `json:"backend_data,omitempty"`
}

// ContainerState represents the lifecycle state of a container
type ContainerState string

const (
	StateCreated    ContainerState = "created"
	StateRunning    ContainerState = "running"
	StatePaused     ContainerState = "paused"
	StateStopped    ContainerState = "stopped"
	StateExited     ContainerState = "exited"
	StateRemoving   ContainerState = "removing"
	StateRestarting ContainerState = "restarting"
	StateDead       ContainerState = "dead"
	StateUnknown    ContainerState = "unknown"
)

// NetworkInfo contains network information for a container
type NetworkInfo struct {
	NetworkID  string    `json:"network_id"`
	EndpointID string    `json:"endpoint_id,omitempty"`
	Gateway    string    `json:"gateway,omitempty"`
	IPAddress  string    `json:"ip_address,omitempty"`
	MacAddress string    `json:"mac_address,omitempty"`
	Ports      []PortMapping `json:"ports,omitempty"`
}

// PortMapping represents a port binding
type PortMapping struct {
	HostIP        string `json:"host_ip,omitempty"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// MountInfo contains mount information for a container
type MountInfo struct {
	Type        MountType `json:"type"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Mode        string    `json:"mode,omitempty"`
	RW          bool      `json:"rw"`
	Propagation string    `json:"propagation,omitempty"`
}

// BackendType identifies the container backend
type BackendType string

const (
	BackendLXD    BackendType = "lxd"
	BackendDocker BackendType = "docker"
	BackendPodman BackendType = "podman"
)

// ============================================================================
// EXECUTION AND LOGGING
// ============================================================================

// ExecConfig defines command execution options
type ExecConfig struct {
	Command      []string          `json:"command"`
	Env          map[string]string `json:"env,omitempty"`
	WorkingDir   string            `json:"working_dir,omitempty"`
	User         string            `json:"user,omitempty"`
	Privileged   bool              `json:"privileged,omitempty"`
	Tty          bool              `json:"tty,omitempty"`
	AttachStdin  bool              `json:"attach_stdin,omitempty"`
	AttachStdout bool              `json:"attach_stdout,omitempty"`
	AttachStderr bool              `json:"attach_stderr,omitempty"`
	Detach       bool              `json:"detach,omitempty"`
}

// ExecResult contains the result of command execution
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   []byte `json:"stdout,omitempty"`
	Stderr   []byte `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

// LogOptions defines log retrieval options
type LogOptions struct {
	Follow     bool      `json:"follow,omitempty"`
	Timestamps bool      `json:"timestamps,omitempty"`
	Tail       int       `json:"tail,omitempty"`
	Since      time.Time `json:"since,omitempty"`
	Until      time.Time `json:"until,omitempty"`
	Stdout     bool      `json:"stdout,omitempty"`
	Stderr     bool      `json:"stderr,omitempty"`
}

// AttachOptions defines container attach options
type AttachOptions struct {
	Stdin  io.Reader `json:"-"`
	Stdout io.Writer `json:"-"`
	Stderr io.Writer `json:"-"`
	Tty    bool      `json:"tty,omitempty"`
	Width  int       `json:"width,omitempty"`
	Height int       `json:"height,omitempty"`
}

// AttachResult contains the result of container attach
type AttachResult struct {
	Stdin    io.WriteCloser `json:"-"`
	Stdout   io.ReadCloser  `json:"-"`
	Stderr   io.ReadCloser  `json:"-"`
	ExitChan <-chan int     `json:"-"`
}

// ============================================================================
// RESOURCE STATISTICS
// ============================================================================

// Stats contains container resource usage statistics
type Stats struct {
	ContainerID string    `json:"container_id"`
	Timestamp   time.Time `json:"timestamp"`

	// CPU
	CPU CPUStats `json:"cpu"`

	// Memory
	Memory MemoryStats `json:"memory"`

	// Network
	Network map[string]NetworkStats `json:"network"`

	// Block I/O
	BlockIO BlockIOStats `json:"block_io"`

	// Process count
	PIDs int64 `json:"pids"`
}

// CPUStats contains CPU usage statistics
type CPUStats struct {
	UsageNanos      uint64  `json:"usage_nanos"`
	SystemNanos     uint64  `json:"system_nanos"`
	UserNanos       uint64  `json:"user_nanos"`
	PercentUsage    float64 `json:"percent_usage"`
	OnlineCPUs      int     `json:"online_cpus"`
	ThrottledPeriods uint64  `json:"throttled_periods,omitempty"`
	ThrottledTime   uint64  `json:"throttled_time,omitempty"`
}

// MemoryStats contains memory usage statistics
type MemoryStats struct {
	Usage       uint64  `json:"usage"`
	MaxUsage    uint64  `json:"max_usage,omitempty"`
	Limit       uint64  `json:"limit,omitempty"`
	Cache       uint64  `json:"cache,omitempty"`
	RSS         uint64  `json:"rss,omitempty"`
	Swap        uint64  `json:"swap,omitempty"`
	PercentUsage float64 `json:"percent_usage,omitempty"`
}

// NetworkStats contains network interface statistics
type NetworkStats struct {
	RxBytes   uint64 `json:"rx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	RxErrors  uint64 `json:"rx_errors"`
	RxDropped uint64 `json:"rx_dropped"`
	TxBytes   uint64 `json:"tx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
	TxErrors  uint64 `json:"tx_errors"`
	TxDropped uint64 `json:"tx_dropped"`
}

// BlockIOStats contains block I/O statistics
type BlockIOStats struct {
	ReadBytes   uint64 `json:"read_bytes"`
	WriteBytes  uint64 `json:"write_bytes"`
	ReadOps     uint64 `json:"read_ops"`
	WriteOps    uint64 `json:"write_ops"`
}

// ============================================================================
// HEALTH AND FILTERING
// ============================================================================

// HealthStatus represents container health check result
type HealthStatus struct {
	Status      HealthState `json:"status"`
	FailingStreak int        `json:"failing_streak,omitempty"`
	Log         []HealthLog `json:"log,omitempty"`
}

// HealthState represents health check state
type HealthState string

const (
	HealthStateHealthy   HealthState = "healthy"
	HealthStateUnhealthy HealthState = "unhealthy"
	HealthStateStarting  HealthState = "starting"
	HealthStateNone      HealthState = "none"
)

// HealthLog contains a single health check result
type HealthLog struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	ExitCode int       `json:"exit_code"`
	Output   string    `json:"output"`
}

// Filter defines container listing filter
type Filter struct {
	ID        string                `json:"id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Image     string                `json:"image,omitempty"`
	State     []ContainerState      `json:"state,omitempty"`
	Labels    map[string]string     `json:"labels,omitempty"`
	Before    string                `json:"before,omitempty"`
	Since     string                `json:"since,omitempty"`
	Health    HealthState           `json:"health,omitempty"`
	Network   string                `json:"network,omitempty"`
	Ancestors []string              `json:"ancestors,omitempty"`
}

// ============================================================================
// RUNTIME INFORMATION
// ============================================================================

// RuntimeInfo contains information about the container runtime
type RuntimeInfo struct {
	Backend       BackendType   `json:"backend"`
	Version       string        `json:"version"`
	APIVersion    string        `json:"api_version"`
	OS            string        `json:"os"`
	Arch          string        `json:"arch"`
	KernelVersion string        `json:"kernel_version,omitempty"`
	Driver        string        `json:"driver,omitempty"`
	Containers    RuntimeCounts `json:"containers"`
	Images        int           `json:"images"`
	MemTotal      uint64        `json:"mem_total,omitempty"`
	CPUs          int           `json:"cpus,omitempty"`
}

// RuntimeCounts contains container counts by state
type RuntimeCounts struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Paused  int `json:"paused"`
	Stopped int `json:"stopped"`
}

// RuntimeVersion contains detailed version information
type RuntimeVersion struct {
	Version       string            `json:"version"`
	APIVersion    string            `json:"api_version"`
	MinAPIVersion string            `json:"min_api_version,omitempty"`
	GitCommit     string            `json:"git_commit,omitempty"`
	GoVersion     string            `json:"go_version,omitempty"`
	OS            string            `json:"os"`
	Arch          string            `json:"arch"`
	KernelVersion string            `json:"kernel_version,omitempty"`
	BuildTime     string            `json:"build_time,omitempty"`
	Components    map[string]string `json:"components,omitempty"`
}

// ============================================================================
// VALIDATION
// ============================================================================

// Validate validates a container specification
func (s *ContainerSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidSpec)
	}

	if s.Image == "" {
		return fmt.Errorf("%w: image is required", ErrInvalidSpec)
	}

	// Validate name format (alphanumeric, dashes, underscores)
	for _, c := range s.Name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return fmt.Errorf("%w: name contains invalid character: %c", ErrInvalidSpec, c)
		}
	}

	// Validate resource limits
	if err := s.Resources.Validate(); err != nil {
		return err
	}

	// Validate health check
	if s.HealthCheck != nil {
		if err := s.HealthCheck.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate validates resource specifications
func (r *ResourceSpec) Validate() error {
	// Add validation for resource formats if needed
	return nil
}

// Validate validates health check specification
func (h *HealthCheckSpec) Validate() error {
	if len(h.Command) == 0 {
		return fmt.Errorf("%w: health check command is required", ErrInvalidSpec)
	}

	if h.Interval < 0 {
		return fmt.Errorf("%w: health check interval cannot be negative", ErrInvalidSpec)
	}

	if h.Timeout < 0 {
		return fmt.Errorf("%w: health check timeout cannot be negative", ErrInvalidSpec)
	}

	if h.Retries < 0 {
		return fmt.Errorf("%w: health check retries cannot be negative", ErrInvalidSpec)
	}

	return nil
}

// IsRunning returns true if the container is in a running state
func (c *Container) IsRunning() bool {
	return c.State == StateRunning
}

// IsStopped returns true if the container is stopped
func (c *Container) IsStopped() bool {
	return c.State == StateStopped || c.State == StateExited
}

// Match checks if a container matches the given filter
func (c *Container) Match(f *Filter) bool {
	if f == nil {
		return true
	}

	if f.ID != "" && c.ID != f.ID {
		return false
	}

	if f.Name != "" && c.Name != f.Name {
		return false
	}

	if f.Image != "" && c.Image != f.Image {
		return false
	}

	if len(f.State) > 0 {
		found := false
		for _, s := range f.State {
			if c.State == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(f.Labels) > 0 {
		for k, v := range f.Labels {
			if cv, ok := c.Labels[k]; !ok || cv != v {
				return false
			}
		}
	}

	return true
}
