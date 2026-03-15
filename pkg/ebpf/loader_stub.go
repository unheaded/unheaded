// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build !linux
// +build !linux

// Package ebpf provides eBPF program loading and management.
// This stub file provides the interface for non-Linux platforms.
//
// THE WHISPERING VOID - eBPF is Linux-only magic
package ebpf

import (
	"context"
	"errors"
	"time"
)

// ErrNotSupported is returned on non-Linux platforms
var ErrNotSupported = errors.New("eBPF is only supported on Linux")

// Errors
var (
	ErrProgramNotFound    = errors.New("eBPF program not found")
	ErrProgramExists      = errors.New("eBPF program already loaded")
	ErrLoadFailed         = errors.New("failed to load eBPF program")
	ErrAttachFailed       = errors.New("failed to attach eBPF program")
	ErrDetachFailed       = errors.New("failed to detach eBPF program")
	ErrMapNotFound        = errors.New("eBPF map not found")
	ErrMapAccessDenied    = errors.New("eBPF map access denied")
	ErrKernelNotSupported = errors.New("kernel version not supported")
	ErrVerifierFailed     = errors.New("eBPF verifier rejected program")
	ErrInvalidProgram     = errors.New("invalid eBPF program")
	ErrLoaderClosed       = errors.New("eBPF loader is closed")
	ErrBTFNotAvailable    = errors.New("BTF information not available")
	ErrInterfaceNotFound  = errors.New("network interface not found")
	ErrPinPathExists      = errors.New("pin path already exists")
	ErrUnsupportedMapType = errors.New("unsupported map type for operation")
	ErrELFParseFailed     = errors.New("failed to parse ELF file")
	ErrNoInstructions     = errors.New("no BPF instructions found")
	ErrSyscallFailed      = errors.New("BPF syscall failed")
)

// ProgramType represents the type of eBPF program
type ProgramType string

const (
	TypeKProbe        ProgramType = "kprobe"
	TypeKRetProbe     ProgramType = "kretprobe"
	TypeTracepoint    ProgramType = "tracepoint"
	TypeXDP           ProgramType = "xdp"
	TypeTC            ProgramType = "tc"
	TypeCgroupSkb     ProgramType = "cgroup_skb"
	TypeCgroupSock    ProgramType = "cgroup_sock"
	TypeSocketFilter  ProgramType = "socket_filter"
	TypeSched         ProgramType = "sched"
	TypePerf          ProgramType = "perf"
	TypeRawTracepoint ProgramType = "raw_tracepoint"
	TypeFentry        ProgramType = "fentry"
	TypeFexit         ProgramType = "fexit"
	TypeLSM           ProgramType = "lsm"
	TypeIterator      ProgramType = "iterator"
)

// AttachType represents how the program is attached
type AttachType string

const (
	AttachNone            AttachType = "none"
	AttachCgroupInet4Bind AttachType = "cgroup_inet4_bind"
	AttachCgroupInet6Bind AttachType = "cgroup_inet6_bind"
	AttachCgroupSockOps   AttachType = "cgroup_sock_ops"
	AttachXDPDriver       AttachType = "xdp_driver"
	AttachXDPGeneric      AttachType = "xdp_generic"
	AttachXDPOffload      AttachType = "xdp_offload"
	AttachTCIngress       AttachType = "tc_ingress"
	AttachTCEgress        AttachType = "tc_egress"
)

// ProgramStatus represents the current status of a program
type ProgramStatus string

const (
	StatusUnloaded  ProgramStatus = "unloaded"
	StatusLoading   ProgramStatus = "loading"
	StatusLoaded    ProgramStatus = "loaded"
	StatusAttaching ProgramStatus = "attaching"
	StatusAttached  ProgramStatus = "attached"
	StatusDetaching ProgramStatus = "detaching"
	StatusError     ProgramStatus = "error"
)

// MapType represents the type of eBPF map
type MapType string

const (
	MapTypeHash           MapType = "hash"
	MapTypeArray          MapType = "array"
	MapTypeProgArray      MapType = "prog_array"
	MapTypePerfEventArray MapType = "perf_event_array"
	MapTypePercpuHash     MapType = "percpu_hash"
	MapTypePercpuArray    MapType = "percpu_array"
	MapTypeLruHash        MapType = "lru_hash"
	MapTypeLruPercpuHash  MapType = "lru_percpu_hash"
	MapTypeLpmTrie        MapType = "lpm_trie"
	MapTypeArrayOfMaps    MapType = "array_of_maps"
	MapTypeHashOfMaps     MapType = "hash_of_maps"
	MapTypeRingbuf        MapType = "ringbuf"
	MapTypeBloom          MapType = "bloom_filter"
	MapTypeStack          MapType = "stack"
	MapTypeQueue          MapType = "queue"
)

// ProgramSpec defines an eBPF program to load
type ProgramSpec struct {
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Type        ProgramType       `json:"type"`
	AttachTo    string            `json:"attach_to"`
	AttachType  AttachType        `json:"attach_type"`
	PinPath     string            `json:"pin_path"`
	MapPinPaths map[string]string `json:"map_pins"`
	Constants   map[string]uint64 `json:"constants"`
	Labels      map[string]string `json:"labels"`
}

// ProgramInfo represents a loaded eBPF program
type ProgramInfo struct {
	Name       string            `json:"name"`
	ID         uint32            `json:"id"`
	Type       ProgramType       `json:"type"`
	Tag        string            `json:"tag"`
	JitedSize  uint32            `json:"jited_size"`
	XlatedSize uint32            `json:"xlated_size"`
	LoadTime   time.Time         `json:"load_time"`
	Status     ProgramStatus     `json:"status"`
	AttachTo   string            `json:"attach_to"`
	AttachType AttachType        `json:"attach_type"`
	Maps       []string          `json:"maps"`
	Labels     map[string]string `json:"labels"`
	Error      string            `json:"error,omitempty"`
}

// MapInfo represents an eBPF map
type MapInfo struct {
	Name       string  `json:"name"`
	ID         uint32  `json:"id"`
	Type       MapType `json:"type"`
	KeySize    uint32  `json:"key_size"`
	ValueSize  uint32  `json:"value_size"`
	MaxEntries uint32  `json:"max_entries"`
	Flags      uint32  `json:"flags"`
	PinPath    string  `json:"pin_path,omitempty"`
}

// ProgramMetrics represents runtime metrics for an eBPF program
type ProgramMetrics struct {
	Name       string    `json:"name"`
	RunCount   uint64    `json:"run_count"`
	RunTimeNs  uint64    `json:"run_time_ns"`
	AvgRunNs   uint64    `json:"avg_run_ns"`
	Recursion  uint64    `json:"recursion_misses"`
	SampleTime time.Time `json:"sample_time"`
}

// KernelFeatures describes available kernel eBPF features
type KernelFeatures struct {
	Version         KernelVersion
	BTFEnabled      bool
	BTFPath         string
	RingbufSupport  bool
	BPFLinkSupport  bool
	KprobeMulti     bool
	FentrySupport   bool
	TCXSupport      bool
	BPFTokenSupport bool
	MemcgAccounting bool
	BPFArena        bool
	SignalSupport   bool
}

// KernelVersion represents a kernel version
type KernelVersion struct {
	Major int
	Minor int
	Patch int
}

// String returns the kernel version as a string
func (k KernelVersion) String() string {
	return "0.0.0"
}

// AtLeast checks if kernel version is at least the given version
func (k KernelVersion) AtLeast(major, minor, patch int) bool {
	return false
}

// LoaderConfig holds configuration for creating an eBPF loader
type LoaderConfig struct {
	BTFPath         string
	PinPath         string
	MapPinPath      string
	Debug           bool
	VerifierLogSize int
	AllowRlimit     bool
	MetricsEnabled  bool
}

// DefaultLoaderConfig returns a default configuration
func DefaultLoaderConfig() LoaderConfig {
	return LoaderConfig{
		PinPath:         "/sys/fs/bpf",
		VerifierLogSize: 64 * 1024,
		AllowRlimit:     true,
		MetricsEnabled:  true,
	}
}

// Loader defines the interface for eBPF program management
type Loader interface {
	Load(ctx context.Context, spec *ProgramSpec) error
	Unload(ctx context.Context, name string) error
	Attach(ctx context.Context, name string) error
	Detach(ctx context.Context, name string) error

	GetProgram(ctx context.Context, name string) (*ProgramInfo, error)
	ListPrograms(ctx context.Context) ([]*ProgramInfo, error)
	ProgramExists(ctx context.Context, name string) bool

	GetMap(ctx context.Context, programName, mapName string) (*MapInfo, error)
	ListMaps(ctx context.Context, programName string) ([]*MapInfo, error)
	ReadMap(ctx context.Context, programName, mapName string, key []byte) ([]byte, error)
	WriteMap(ctx context.Context, programName, mapName string, key, value []byte) error
	DeleteMapEntry(ctx context.Context, programName, mapName string, key []byte) error
	IterateMap(ctx context.Context, programName, mapName string, fn func(key, value []byte) error) error

	ReadRingbuf(ctx context.Context, programName, mapName string) (<-chan []byte, error)
	ReadPerfEvents(ctx context.Context, programName, mapName string) (<-chan []byte, error)

	GetMetrics(ctx context.Context, name string) (*ProgramMetrics, error)

	Close() error
}

// StubLoader is a non-functional loader for non-Linux platforms
type StubLoader struct{}

// NewLoader creates a new eBPF loader (returns error on non-Linux)
func NewLoader(cfg LoaderConfig) (Loader, error) {
	return nil, ErrNotSupported
}

// NewNativeLoader creates a new native eBPF loader (returns error on non-Linux)
func NewNativeLoader(cfg LoaderConfig) (*StubLoader, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) Load(ctx context.Context, spec *ProgramSpec) error {
	return ErrNotSupported
}

func (l *StubLoader) Unload(ctx context.Context, name string) error {
	return ErrNotSupported
}

func (l *StubLoader) Attach(ctx context.Context, name string) error {
	return ErrNotSupported
}

func (l *StubLoader) Detach(ctx context.Context, name string) error {
	return ErrNotSupported
}

func (l *StubLoader) SetAttachTarget(ctx context.Context, name string, attachTo string) error {
	return ErrNotSupported
}

func (l *StubLoader) GetProgram(ctx context.Context, name string) (*ProgramInfo, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) ListPrograms(ctx context.Context) ([]*ProgramInfo, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) ProgramExists(ctx context.Context, name string) bool {
	return false
}

func (l *StubLoader) GetMap(ctx context.Context, programName, mapName string) (*MapInfo, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) ListMaps(ctx context.Context, programName string) ([]*MapInfo, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) ReadMap(ctx context.Context, programName, mapName string, key []byte) ([]byte, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) WriteMap(ctx context.Context, programName, mapName string, key, value []byte) error {
	return ErrNotSupported
}

func (l *StubLoader) DeleteMapEntry(ctx context.Context, programName, mapName string, key []byte) error {
	return ErrNotSupported
}

func (l *StubLoader) IterateMap(ctx context.Context, programName, mapName string, fn func(key, value []byte) error) error {
	return ErrNotSupported
}

func (l *StubLoader) ReadRingbuf(ctx context.Context, programName, mapName string) (<-chan []byte, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) ReadPerfEvents(ctx context.Context, programName, mapName string) (<-chan []byte, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) GetMetrics(ctx context.Context, name string) (*ProgramMetrics, error) {
	return nil, ErrNotSupported
}

func (l *StubLoader) Close() error {
	return nil
}

// DefaultPrograms returns the standard Kingdom eBPF programs
func DefaultPrograms() []*ProgramSpec {
	return []*ProgramSpec{
		{
			Name:       "packet_marker",
			Path:       "/opt/unheaded/ebpf/packet_marker.bpf.o",
			Type:       TypeXDP,
			AttachTo:   "eth0",
			AttachType: AttachXDPDriver,
			Labels: map[string]string{
				"hollow": "whispering_void",
				"role":   "packet_marking",
			},
		},
	}
}

// Ensure StubLoader implements Loader interface
var _ Loader = (*StubLoader)(nil)
