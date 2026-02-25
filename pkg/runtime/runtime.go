// Package runtime provides a Container Runtime Interface for Unheaded Kingdom.
// It implements container lifecycle management, image handling, process execution,
// and resource management using only Go standard library and golang.org/x/sys.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Common errors returned by the runtime.
var (
	ErrContainerNotFound   = errors.New("container not found")
	ErrContainerExists     = errors.New("container already exists")
	ErrContainerRunning    = errors.New("container is running")
	ErrContainerNotRunning = errors.New("container is not running")
	ErrImageNotFound       = errors.New("image not found")
	ErrSandboxNotFound     = errors.New("sandbox not found")
	ErrInvalidConfig       = errors.New("invalid configuration")
	ErrOperationTimeout    = errors.New("operation timed out")
	ErrResourceExhausted   = errors.New("resource exhausted")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrNotSupported        = errors.New("operation not supported")
)

// ContainerState represents the state of a container.
type ContainerState int

const (
	// ContainerStateUnknown indicates the container state is unknown.
	ContainerStateUnknown ContainerState = iota
	// ContainerStateCreated indicates the container has been created but not started.
	ContainerStateCreated
	// ContainerStateRunning indicates the container is currently running.
	ContainerStateRunning
	// ContainerStateStopped indicates the container has stopped.
	ContainerStateStopped
	// ContainerStateExited indicates the container has exited.
	ContainerStateExited
	// ContainerStatePaused indicates the container is paused.
	ContainerStatePaused
)

// String returns the string representation of ContainerState.
func (s ContainerState) String() string {
	switch s {
	case ContainerStateCreated:
		return "created"
	case ContainerStateRunning:
		return "running"
	case ContainerStateStopped:
		return "stopped"
	case ContainerStateExited:
		return "exited"
	case ContainerStatePaused:
		return "paused"
	default:
		return "unknown"
	}
}

// ContainerInfo contains information about a container.
type ContainerInfo struct {
	// ID is the unique identifier of the container.
	ID string
	// Name is the human-readable name of the container.
	Name string
	// Image is the image used to create the container.
	Image string
	// State is the current state of the container.
	State ContainerState
	// PID is the process ID of the container's init process.
	PID int
	// ExitCode is the exit code of the container (if exited).
	ExitCode int
	// CreatedAt is when the container was created.
	CreatedAt time.Time
	// StartedAt is when the container was started.
	StartedAt time.Time
	// FinishedAt is when the container finished.
	FinishedAt time.Time
	// SandboxID is the ID of the sandbox this container belongs to.
	SandboxID string
	// Labels are user-defined metadata.
	Labels map[string]string
	// Annotations are runtime-specific metadata.
	Annotations map[string]string
}

// ContainerConfig defines the configuration for creating a container.
type ContainerConfig struct {
	// ID is the unique identifier for the container.
	ID string
	// Name is the human-readable name.
	Name string
	// Image is the image reference.
	Image string
	// Command is the command to run.
	Command []string
	// Args are arguments to the command.
	Args []string
	// Env are environment variables.
	Env []string
	// WorkingDir is the working directory.
	WorkingDir string
	// User is the user to run as.
	User string
	// Stdin enables stdin.
	Stdin bool
	// StdinOnce closes stdin after first attach.
	StdinOnce bool
	// Tty allocates a pseudo-TTY.
	Tty bool
	// Labels are user-defined metadata.
	Labels map[string]string
	// Annotations are runtime-specific metadata.
	Annotations map[string]string
	// Mounts are volume mounts.
	Mounts []Mount
	// Resources are resource limits.
	Resources *ResourceConfig
	// Linux contains Linux-specific configuration.
	Linux *LinuxContainerConfig
	// SandboxID is the ID of the sandbox to join.
	SandboxID string
}

// ResourceConfig defines resource limits for a container.
type ResourceConfig struct {
	// CPUShares is the CPU shares (relative weight).
	// In cgroup v2, this is converted to cpu.weight (1-10000).
	CPUShares int64
	// CPUQuota is the CPU CFS quota in microseconds.
	// In cgroup v2, this becomes the first value in cpu.max.
	CPUQuota int64
	// CPUPeriod is the CPU CFS period in microseconds.
	// In cgroup v2, this becomes the second value in cpu.max.
	CPUPeriod int64
	// CPUSetCPUs is the CPUs in which to allow execution.
	// Maps to cpuset.cpus in cgroup v2.
	CPUSetCPUs string
	// CPUSetMems is the memory nodes in which to allow execution.
	// Maps to cpuset.mems in cgroup v2.
	CPUSetMems string
	// CPUWeight is the cgroup v2 CPU weight (1-10000, default 100).
	// Takes precedence over CPUShares if set.
	CPUWeight uint64

	// MemoryLimitBytes is the memory limit in bytes.
	// Maps to memory.max in cgroup v2.
	MemoryLimitBytes int64
	// MemorySwapLimitBytes is the swap limit in bytes.
	// In cgroup v2, memory.swap.max is for swap only (not memory+swap).
	MemorySwapLimitBytes int64
	// MemoryReservationBytes is the soft memory limit.
	// Maps to memory.low in cgroup v2 (best-effort protection).
	MemoryReservationBytes int64
	// MemoryHighBytes is the memory throttling threshold.
	// Maps to memory.high in cgroup v2 - processes are throttled when exceeded.
	MemoryHighBytes int64
	// MemoryMinBytes is the guaranteed minimum memory.
	// Maps to memory.min in cgroup v2 (hard protection).
	MemoryMinBytes int64
	// OOMKillDisable disables OOM killer.
	OOMKillDisable bool
	// OOMScoreAdj is the OOM score adjustment.
	OOMScoreAdj int
	// OOMGroup enables memory.oom.group in cgroup v2.
	// When true, all processes in the cgroup are killed together on OOM.
	OOMGroup bool

	// PidsLimit is the maximum number of processes.
	// Maps to pids.max in cgroup v2.
	PidsLimit int64

	// IOWeight is the IO weight (1-10000, default 100).
	// Maps to io.weight in cgroup v2.
	IOWeight uint16
	// IOLimits are per-device IO limits.
	// Maps to io.max in cgroup v2.
	IOLimits []IOLimit

	// Ulimits are resource limits.
	Ulimits []Ulimit

	// HugepageLimits are hugepage limits per page size.
	HugepageLimits map[string]int64
}

// IOLimit defines IO limits for a block device.
type IOLimit struct {
	// Major is the device major number.
	Major uint64
	// Minor is the device minor number.
	Minor uint64
	// ReadBytesPerSec is the read byte rate limit (-1 for unlimited).
	ReadBytesPerSec int64
	// WriteBytesPerSec is the write byte rate limit (-1 for unlimited).
	WriteBytesPerSec int64
	// ReadIOPS is the read IOPS limit (-1 for unlimited).
	ReadIOPS int64
	// WriteIOPS is the write IOPS limit (-1 for unlimited).
	WriteIOPS int64
}

// Ulimit represents a resource limit.
type Ulimit struct {
	Name string
	Hard uint64
	Soft uint64
}

// LinuxContainerConfig contains Linux-specific container configuration.
type LinuxContainerConfig struct {
	// Namespaces are the namespaces to create/join.
	Namespaces []LinuxNamespace
	// Capabilities are Linux capabilities.
	Capabilities *LinuxCapabilities
	// Seccomp is the seccomp profile.
	Seccomp *LinuxSeccomp
	// AppArmor is the AppArmor profile.
	AppArmor string
	// SELinux are SELinux options.
	SELinux *SELinuxOption
	// ReadonlyRootfs makes the root filesystem read-only.
	ReadonlyRootfs bool
	// MaskedPaths are paths to mask.
	MaskedPaths []string
	// ReadonlyPaths are paths to make read-only.
	ReadonlyPaths []string
	// Sysctl are sysctl settings.
	Sysctl map[string]string
	// CgroupsPath is the path to the cgroup.
	CgroupsPath string
}

// LinuxNamespace represents a Linux namespace.
type LinuxNamespace struct {
	Type NamespaceType
	Path string
}

// NamespaceType is the type of Linux namespace.
type NamespaceType string

const (
	PIDNamespace     NamespaceType = "pid"
	NetworkNamespace NamespaceType = "net"
	MountNamespace   NamespaceType = "mnt"
	IPCNamespace     NamespaceType = "ipc"
	UTSNamespace     NamespaceType = "uts"
	UserNamespace    NamespaceType = "user"
	CgroupNamespace  NamespaceType = "cgroup"
)

// LinuxCapabilities represents Linux capabilities.
type LinuxCapabilities struct {
	Bounding    []string
	Effective   []string
	Inheritable []string
	Permitted   []string
	Ambient     []string
}

// LinuxSeccomp represents a seccomp profile.
type LinuxSeccomp struct {
	DefaultAction SeccompAction
	Architectures []string
	Syscalls      []SeccompSyscall
}

// SeccompAction is the action to take for a syscall.
type SeccompAction string

const (
	SeccompActionAllow SeccompAction = "SCMP_ACT_ALLOW"
	SeccompActionErrno SeccompAction = "SCMP_ACT_ERRNO"
	SeccompActionKill  SeccompAction = "SCMP_ACT_KILL"
	SeccompActionLog   SeccompAction = "SCMP_ACT_LOG"
	SeccompActionTrap  SeccompAction = "SCMP_ACT_TRAP"
	SeccompActionTrace SeccompAction = "SCMP_ACT_TRACE"
)

// SeccompSyscall represents a syscall rule.
type SeccompSyscall struct {
	Names  []string
	Action SeccompAction
	Args   []SeccompArg
}

// SeccompArg represents a syscall argument rule.
type SeccompArg struct {
	Index    uint
	Value    uint64
	ValueTwo uint64
	Op       SeccompOperator
}

// SeccompOperator is the comparison operator for seccomp args.
type SeccompOperator string

const (
	SeccompOpNotEqual     SeccompOperator = "SCMP_CMP_NE"
	SeccompOpLessThan     SeccompOperator = "SCMP_CMP_LT"
	SeccompOpLessEqual    SeccompOperator = "SCMP_CMP_LE"
	SeccompOpEqual        SeccompOperator = "SCMP_CMP_EQ"
	SeccompOpGreaterEqual SeccompOperator = "SCMP_CMP_GE"
	SeccompOpGreaterThan  SeccompOperator = "SCMP_CMP_GT"
	SeccompOpMaskedEqual  SeccompOperator = "SCMP_CMP_MASKED_EQ"
)

// SELinuxOption represents SELinux options.
type SELinuxOption struct {
	User  string
	Role  string
	Type  string
	Level string
}

// Runtime is the main container runtime interface.
type Runtime interface {
	// Container operations
	CreateContainer(ctx context.Context, config *ContainerConfig) (*ContainerInfo, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, containerID string, force bool) error
	GetContainer(ctx context.Context, containerID string) (*ContainerInfo, error)
	ListContainers(ctx context.Context, filter *ContainerFilter) ([]*ContainerInfo, error)
	PauseContainer(ctx context.Context, containerID string) error
	ResumeContainer(ctx context.Context, containerID string) error

	// Image operations
	PullImage(ctx context.Context, ref string, options *ImagePullOptions) (*ImageInfo, error)
	GetImage(ctx context.Context, ref string) (*ImageInfo, error)
	ListImages(ctx context.Context, filter *ImageFilter) ([]*ImageInfo, error)
	RemoveImage(ctx context.Context, ref string, force bool) error

	// Exec operations
	ExecInContainer(ctx context.Context, containerID string, config *ExecConfig) (*ExecResult, error)
	ExecAttach(ctx context.Context, execID string, streams *IOStreams) error

	// Log operations
	GetContainerLogs(ctx context.Context, containerID string, options *LogOptions) (io.ReadCloser, error)

	// Sandbox operations
	CreateSandbox(ctx context.Context, config *SandboxConfig) (*SandboxInfo, error)
	StopSandbox(ctx context.Context, sandboxID string) error
	RemoveSandbox(ctx context.Context, sandboxID string) error
	GetSandbox(ctx context.Context, sandboxID string) (*SandboxInfo, error)
	ListSandboxes(ctx context.Context, filter *SandboxFilter) ([]*SandboxInfo, error)

	// Runtime info
	Version(ctx context.Context) (*VersionInfo, error)
	Status(ctx context.Context) (*RuntimeStatus, error)

	// Cleanup
	Close() error
}

// ContainerFilter defines filters for listing containers.
type ContainerFilter struct {
	ID       string
	Name     string
	State    ContainerState
	Labels   map[string]string
	SandboxID string
}

// VersionInfo contains runtime version information.
type VersionInfo struct {
	Version      string
	RuntimeName  string
	RuntimeVersion string
	APIVersion   string
	GitCommit    string
	GoVersion    string
	OS           string
	Arch         string
}

// RuntimeStatus contains runtime status information.
type RuntimeStatus struct {
	Conditions []RuntimeCondition
}

// RuntimeCondition represents a runtime condition.
type RuntimeCondition struct {
	Type    string
	Status  bool
	Reason  string
	Message string
}

// RuntimeConfig contains configuration for the runtime.
type RuntimeConfig struct {
	// Root is the root directory for the runtime.
	Root string
	// StateDir is the directory for runtime state.
	StateDir string
	// RuntimePath is the path to the low-level runtime binary.
	RuntimePath string
	// RegistryConfig is the registry configuration.
	RegistryConfig *RegistryConfig
	// CgroupDriver is the cgroup driver to use.
	CgroupDriver string
	// DefaultCgroupParent is the default parent cgroup.
	DefaultCgroupParent string
	// NetworkPluginDir is the directory containing network plugins.
	NetworkPluginDir string
	// CNIConfigDir is the directory containing CNI configuration.
	CNIConfigDir string
	// CNIBinDir is the directory containing CNI binaries.
	CNIBinDir string
}

// RegistryConfig contains registry configuration.
type RegistryConfig struct {
	// Mirrors maps registry hosts to mirror URLs.
	Mirrors map[string][]string
	// InsecureRegistries are registries that don't require TLS.
	InsecureRegistries []string
	// AuthConfigs are authentication configurations.
	AuthConfigs map[string]*AuthConfig
}

// AuthConfig contains authentication configuration for a registry.
type AuthConfig struct {
	Username      string
	Password      string
	Auth          string
	IdentityToken string
}

// DefaultRuntime implements the Runtime interface.
type DefaultRuntime struct {
	mu sync.RWMutex

	config     *RuntimeConfig
	containers map[string]*container
	sandboxes  map[string]*Sandbox
	images     *ImageStore
	cgroups    *CgroupManager
	namespaces *NamespaceManager
	volumes    *VolumeManager

	execSessions map[string]*execSession
	logStreams   map[string]*logStream

	closed bool
}

// NewRuntime creates a new runtime instance.
func NewRuntime(config *RuntimeConfig) (*DefaultRuntime, error) {
	if config == nil {
		config = &RuntimeConfig{}
	}

	if config.Root == "" {
		config.Root = "/var/lib/unheaded"
	}
	if config.StateDir == "" {
		config.StateDir = "/run/unheaded"
	}
	if config.CgroupDriver == "" {
		config.CgroupDriver = "systemd"
	}

	cgroupMgr, err := NewCgroupManager(config.CgroupDriver, config.DefaultCgroupParent)
	if err != nil {
		return nil, fmt.Errorf("failed to create cgroup manager: %w", err)
	}

	nsMgr, err := NewNamespaceManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace manager: %w", err)
	}

	volMgr, err := NewVolumeManager(config.Root + "/volumes")
	if err != nil {
		return nil, fmt.Errorf("failed to create volume manager: %w", err)
	}

	imgStore, err := NewImageStore(config.Root+"/images", config.RegistryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create image store: %w", err)
	}

	return &DefaultRuntime{
		config:       config,
		containers:   make(map[string]*container),
		sandboxes:    make(map[string]*Sandbox),
		images:       imgStore,
		cgroups:      cgroupMgr,
		namespaces:   nsMgr,
		volumes:      volMgr,
		execSessions: make(map[string]*execSession),
		logStreams:   make(map[string]*logStream),
	}, nil
}

// Version returns runtime version information.
func (r *DefaultRuntime) Version(ctx context.Context) (*VersionInfo, error) {
	return &VersionInfo{
		Version:        "1.0.0",
		RuntimeName:    "unheaded",
		RuntimeVersion: "1.0.0",
		APIVersion:     "v1",
		GitCommit:      "dev",
		GoVersion:      "go1.21",
		OS:             "linux",
		Arch:           "amd64",
	}, nil
}

// Status returns runtime status.
func (r *DefaultRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conditions := []RuntimeCondition{
		{
			Type:    "RuntimeReady",
			Status:  !r.closed,
			Reason:  "RuntimeReady",
			Message: "runtime is ready",
		},
		{
			Type:    "NetworkReady",
			Status:  true,
			Reason:  "NetworkReady",
			Message: "network is ready",
		},
	}

	return &RuntimeStatus{
		Conditions: conditions,
	}, nil
}

// Close releases runtime resources.
func (r *DefaultRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	// Close all log streams
	for _, ls := range r.logStreams {
		ls.Close()
	}

	// Close image store
	if r.images != nil {
		r.images.Close()
	}

	return nil
}

// generateID generates a unique ID.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
