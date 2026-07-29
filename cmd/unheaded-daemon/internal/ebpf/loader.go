// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package ebpf provides eBPF program loading and management
// THE WHISPERING VOID - Where silent observers attach to the kernel
package ebpf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// ERRORS - WHEN THE VOID REJECTS US
// ============================================================================

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
)

// ============================================================================
// PROGRAM TYPES - THE OBSERVERS' FORMS
// ============================================================================

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

// ============================================================================
// PROGRAM SPEC - THE OBSERVER'S BLUEPRINT
// ============================================================================

// ProgramSpec defines an eBPF program to load
type ProgramSpec struct {
	Name        string            `json:"name"`        // Unique identifier
	Path        string            `json:"path"`        // Path to .bpf.o file
	Type        ProgramType       `json:"type"`        // Program type
	AttachTo    string            `json:"attach_to"`   // Attach target (function, tracepoint, interface)
	AttachType  AttachType        `json:"attach_type"` // How to attach
	PinPath     string            `json:"pin_path"`    // Optional BPF filesystem pin path
	MapPinPaths map[string]string `json:"map_pins"`    // Map pin paths
	Constants   map[string]uint64 `json:"constants"`   // Compile-time constants
	Labels      map[string]string `json:"labels"`      // User labels
}

// ProgramInfo represents a loaded eBPF program
type ProgramInfo struct {
	Name       string            `json:"name"`
	ID         uint32            `json:"id"` // Kernel program ID
	Type       ProgramType       `json:"type"`
	Tag        string            `json:"tag"`         // Program tag (hash)
	JitedSize  uint32            `json:"jited_size"`  // JITed program size
	XlatedSize uint32            `json:"xlated_size"` // Translated bytecode size
	LoadTime   time.Time         `json:"load_time"`
	Status     ProgramStatus     `json:"status"`
	AttachTo   string            `json:"attach_to"`
	AttachType AttachType        `json:"attach_type"`
	Maps       []string          `json:"maps"` // Associated map names
	Labels     map[string]string `json:"labels"`
	Error      string            `json:"error,omitempty"`
}

// ============================================================================
// MAP TYPES - THE VOID'S MEMORY
// ============================================================================

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
)

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

// ============================================================================
// METRICS - THE VOID'S TELEMETRY
// ============================================================================

// ProgramMetrics represents runtime metrics for an eBPF program
type ProgramMetrics struct {
	Name       string    `json:"name"`
	RunCount   uint64    `json:"run_count"`
	RunTimeNs  uint64    `json:"run_time_ns"`
	AvgRunNs   uint64    `json:"avg_run_ns"`
	Recursion  uint64    `json:"recursion_misses"`
	SampleTime time.Time `json:"sample_time"`
}

// ============================================================================
// LOADER INTERFACE - THE VOID MASTER
// ============================================================================

// Loader defines the interface for eBPF program management
type Loader interface {
	// Program lifecycle
	Load(ctx context.Context, spec *ProgramSpec) error
	Unload(ctx context.Context, name string) error
	Attach(ctx context.Context, name string) error
	Detach(ctx context.Context, name string) error

	// Program information
	GetProgram(ctx context.Context, name string) (*ProgramInfo, error)
	ListPrograms(ctx context.Context) ([]*ProgramInfo, error)
	ProgramExists(ctx context.Context, name string) bool

	// Map operations
	GetMap(ctx context.Context, programName, mapName string) (*MapInfo, error)
	ListMaps(ctx context.Context, programName string) ([]*MapInfo, error)
	ReadMap(ctx context.Context, programName, mapName string, key []byte) ([]byte, error)
	WriteMap(ctx context.Context, programName, mapName string, key, value []byte) error
	DeleteMapEntry(ctx context.Context, programName, mapName string, key []byte) error
	IterateMap(ctx context.Context, programName, mapName string, fn func(key, value []byte) error) error

	// Ringbuf/Perf event reading
	ReadRingbuf(ctx context.Context, programName, mapName string) (<-chan []byte, error)
	ReadPerfEvents(ctx context.Context, programName, mapName string) (<-chan []byte, error)

	// Metrics
	GetMetrics(ctx context.Context, name string) (*ProgramMetrics, error)

	// Lifecycle
	Close() error
}

// ============================================================================
// MOCK LOADER - FOR TESTING THE VOID
// ============================================================================

// MockLoader implements Loader interface for testing
type MockLoader struct {
	mu       sync.RWMutex
	programs map[string]*ProgramInfo
	maps     map[string]map[string]*MapInfo          // program -> map name -> info
	mapData  map[string]map[string]map[string][]byte // program -> map -> key -> value
	metrics  map[string]*ProgramMetrics
	ringbufs map[string]chan []byte
	closed   bool
}

// NewMockLoader creates a new mock eBPF loader
func NewMockLoader() *MockLoader {
	return &MockLoader{
		programs: make(map[string]*ProgramInfo),
		maps:     make(map[string]map[string]*MapInfo),
		mapData:  make(map[string]map[string]map[string][]byte),
		metrics:  make(map[string]*ProgramMetrics),
		ringbufs: make(map[string]chan []byte),
	}
}

// Load loads an eBPF program (mock implementation)
func (m *MockLoader) Load(ctx context.Context, spec *ProgramSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrLoaderClosed
	}

	if spec == nil || spec.Name == "" {
		return ErrInvalidProgram
	}

	if _, exists := m.programs[spec.Name]; exists {
		return ErrProgramExists
	}

	info := &ProgramInfo{
		Name:       spec.Name,
		ID:         uint32(len(m.programs) + 1), // #nosec G115 -- bounded by the length of an already-allocated slice
		Type:       spec.Type,
		Tag:        fmt.Sprintf("tag-%s-%d", spec.Name, time.Now().Unix()),
		JitedSize:  4096,
		XlatedSize: 2048,
		LoadTime:   time.Now(),
		Status:     StatusLoaded,
		AttachTo:   spec.AttachTo,
		AttachType: spec.AttachType,
		Labels:     spec.Labels,
		Maps:       []string{},
	}

	m.programs[spec.Name] = info
	m.maps[spec.Name] = make(map[string]*MapInfo)
	m.mapData[spec.Name] = make(map[string]map[string][]byte)
	m.metrics[spec.Name] = &ProgramMetrics{
		Name:       spec.Name,
		SampleTime: time.Now(),
	}

	// Create default maps based on program type
	switch spec.Type {
	case TypeXDP, TypeTC:
		m.createDefaultMaps(spec.Name, "packet_count", MapTypePercpuArray, 8, 8, 256)
		m.createDefaultMaps(spec.Name, "flow_state", MapTypeLruHash, 32, 64, 65536)
	case TypeKProbe, TypeTracepoint:
		m.createDefaultMaps(spec.Name, "events", MapTypeRingbuf, 0, 0, 256*1024)
	}

	return nil
}

func (m *MockLoader) createDefaultMaps(prog, name string, mapType MapType, keySize, valueSize, maxEntries uint32) {
	mapInfo := &MapInfo{
		Name:       name,
		ID:         uint32(len(m.maps[prog]) + 1), // #nosec G115 -- bounded by the length of an already-allocated slice
		Type:       mapType,
		KeySize:    keySize,
		ValueSize:  valueSize,
		MaxEntries: maxEntries,
	}
	m.maps[prog][name] = mapInfo
	m.mapData[prog][name] = make(map[string][]byte)
	m.programs[prog].Maps = append(m.programs[prog].Maps, name)
}

// Unload unloads an eBPF program
func (m *MockLoader) Unload(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrLoaderClosed
	}

	if _, exists := m.programs[name]; !exists {
		return ErrProgramNotFound
	}

	delete(m.programs, name)
	delete(m.maps, name)
	delete(m.mapData, name)
	delete(m.metrics, name)

	if ch, exists := m.ringbufs[name]; exists {
		close(ch)
		delete(m.ringbufs, name)
	}

	return nil
}

// Attach attaches an eBPF program
func (m *MockLoader) Attach(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrLoaderClosed
	}

	info, exists := m.programs[name]
	if !exists {
		return ErrProgramNotFound
	}

	if info.Status == StatusAttached {
		return nil // Already attached
	}

	info.Status = StatusAttached
	return nil
}

// Detach detaches an eBPF program
func (m *MockLoader) Detach(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrLoaderClosed
	}

	info, exists := m.programs[name]
	if !exists {
		return ErrProgramNotFound
	}

	if info.Status != StatusAttached {
		return nil // Not attached
	}

	info.Status = StatusLoaded
	return nil
}

// GetProgram returns program info
func (m *MockLoader) GetProgram(ctx context.Context, name string) (*ProgramInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrLoaderClosed
	}

	info, exists := m.programs[name]
	if !exists {
		return nil, ErrProgramNotFound
	}

	return info, nil
}

// ListPrograms returns all loaded programs
func (m *MockLoader) ListPrograms(ctx context.Context) ([]*ProgramInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrLoaderClosed
	}

	programs := make([]*ProgramInfo, 0, len(m.programs))
	for _, p := range m.programs {
		programs = append(programs, p)
	}

	return programs, nil
}

// ProgramExists checks if program exists
func (m *MockLoader) ProgramExists(ctx context.Context, name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.programs[name]
	return exists
}

// GetMap returns map info
func (m *MockLoader) GetMap(ctx context.Context, programName, mapName string) (*MapInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrLoaderClosed
	}

	progMaps, exists := m.maps[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	mapInfo, exists := progMaps[mapName]
	if !exists {
		return nil, ErrMapNotFound
	}

	return mapInfo, nil
}

// ListMaps returns all maps for a program
func (m *MockLoader) ListMaps(ctx context.Context, programName string) ([]*MapInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrLoaderClosed
	}

	progMaps, exists := m.maps[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	maps := make([]*MapInfo, 0, len(progMaps))
	for _, mi := range progMaps {
		maps = append(maps, mi)
	}

	return maps, nil
}

// ReadMap reads a value from a map
func (m *MockLoader) ReadMap(ctx context.Context, programName, mapName string, key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrLoaderClosed
	}

	progMaps, exists := m.mapData[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	mapData, exists := progMaps[mapName]
	if !exists {
		return nil, ErrMapNotFound
	}

	value, exists := mapData[string(key)]
	if !exists {
		return nil, nil // Key not found, return nil
	}

	return value, nil
}

// WriteMap writes a value to a map
func (m *MockLoader) WriteMap(ctx context.Context, programName, mapName string, key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrLoaderClosed
	}

	progMaps, exists := m.mapData[programName]
	if !exists {
		return ErrProgramNotFound
	}

	mapData, exists := progMaps[mapName]
	if !exists {
		return ErrMapNotFound
	}

	mapData[string(key)] = value
	return nil
}

// DeleteMapEntry deletes an entry from a map
func (m *MockLoader) DeleteMapEntry(ctx context.Context, programName, mapName string, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrLoaderClosed
	}

	progMaps, exists := m.mapData[programName]
	if !exists {
		return ErrProgramNotFound
	}

	mapData, exists := progMaps[mapName]
	if !exists {
		return ErrMapNotFound
	}

	delete(mapData, string(key))
	return nil
}

// IterateMap iterates over all entries in a map
func (m *MockLoader) IterateMap(ctx context.Context, programName, mapName string, fn func(key, value []byte) error) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return ErrLoaderClosed
	}

	progMaps, exists := m.mapData[programName]
	if !exists {
		return ErrProgramNotFound
	}

	mapData, exists := progMaps[mapName]
	if !exists {
		return ErrMapNotFound
	}

	for k, v := range mapData {
		if err := fn([]byte(k), v); err != nil {
			return err
		}
	}

	return nil
}

// ReadRingbuf returns a channel for ringbuf events
func (m *MockLoader) ReadRingbuf(ctx context.Context, programName, mapName string) (<-chan []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrLoaderClosed
	}

	if _, exists := m.programs[programName]; !exists {
		return nil, ErrProgramNotFound
	}

	key := fmt.Sprintf("%s:%s", programName, mapName)
	if ch, exists := m.ringbufs[key]; exists {
		return ch, nil
	}

	ch := make(chan []byte, 1024)
	m.ringbufs[key] = ch

	return ch, nil
}

// ReadPerfEvents returns a channel for perf events
func (m *MockLoader) ReadPerfEvents(ctx context.Context, programName, mapName string) (<-chan []byte, error) {
	// Same implementation as ringbuf for mock
	return m.ReadRingbuf(ctx, programName, mapName)
}

// GetMetrics returns program metrics
func (m *MockLoader) GetMetrics(ctx context.Context, name string) (*ProgramMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrLoaderClosed
	}

	metrics, exists := m.metrics[name]
	if !exists {
		return nil, ErrProgramNotFound
	}

	// Simulate some activity
	metrics.RunCount += 100
	metrics.RunTimeNs += 50000
	metrics.AvgRunNs = metrics.RunTimeNs / metrics.RunCount
	metrics.SampleTime = time.Now()

	return metrics, nil
}

// Close closes the loader
func (m *MockLoader) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	// Close all ringbuf channels
	for _, ch := range m.ringbufs {
		close(ch)
	}

	return nil
}

// ============================================================================
// KINGDOM eBPF PROGRAMS - THE SILENT OBSERVERS
// ============================================================================

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
				"tier":   "network",
			},
		},
		{
			Name:       "flow_tracker",
			Path:       "/opt/unheaded/ebpf/flow_tracker.bpf.o",
			Type:       TypeTC,
			AttachTo:   "eth0",
			AttachType: AttachTCIngress,
			Labels: map[string]string{
				"hollow": "whispering_void",
				"role":   "flow_tracking",
				"tier":   "network",
			},
		},
		{
			Name:       "latency_probe",
			Path:       "/opt/unheaded/ebpf/latency_probe.bpf.o",
			Type:       TypeKProbe,
			AttachTo:   "tcp_sendmsg",
			AttachType: AttachNone,
			Labels: map[string]string{
				"hollow": "mythic_abyss",
				"role":   "latency_measurement",
				"tier":   "kernel",
			},
		},
		{
			Name:       "syscall_tracer",
			Path:       "/opt/unheaded/ebpf/syscall_tracer.bpf.o",
			Type:       TypeRawTracepoint,
			AttachTo:   "sys_enter",
			AttachType: AttachNone,
			Labels: map[string]string{
				"hollow": "mythic_abyss",
				"role":   "security_audit",
				"tier":   "kernel",
			},
		},
		{
			Name:       "container_events",
			Path:       "/opt/unheaded/ebpf/container_events.bpf.o",
			Type:       TypeTracepoint,
			AttachTo:   "sched:sched_process_exec",
			AttachType: AttachNone,
			Labels: map[string]string{
				"hollow": "whispering_void",
				"role":   "container_lifecycle",
				"tier":   "orchestration",
			},
		},
	}
}

// ============================================================================
// LOADER FACTORY - AWAKEN THE VOID
// ============================================================================

// LoaderConfig holds configuration for creating an eBPF loader
type LoaderConfig struct {
	BTFPath    string // Path to BTF information
	PinPath    string // BPF filesystem pin path
	MapPinPath string // Map pin path
	Debug      bool   // Enable debug output
}

// NewLoader creates an eBPF loader.
//
// Currently returns MockLoader (no-op) since real kernel loading via
// cilium/ebpf is not yet integrated. This is safe: the mock tracks
// programs in memory without touching the kernel.
//
// When real eBPF loading is needed:
//  1. Add cilium/ebpf dependency
//  2. Implement RealLoader that calls ebpf.LoadCollection()
//  3. Swap this factory to return RealLoader
//  4. Ensure CAP_BPF + CAP_PERFMON capabilities on the process
func NewLoader(cfg LoaderConfig) (Loader, error) {
	return NewMockLoader(), nil
}
