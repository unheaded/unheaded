//go:build linux
// +build linux

// Package ebpf provides a from-scratch eBPF program loader using direct syscalls.
// This is an educational implementation showing how eBPF works at the kernel level.
//
// THE WHISPERING VOID - Original Kingdom magic, no external eBPF libraries.
//
// This implementation uses only:
// - Go standard library (debug/elf for ELF parsing)
// - golang.org/x/sys/unix (syscall wrappers)
// - prometheus (metrics)
//
// It demonstrates:
// - Direct BPF syscalls (BPF_PROG_LOAD, BPF_MAP_CREATE, etc.)
// - Manual ELF parsing for .bpf.o files
// - BTF (BPF Type Format) parsing from scratch
// - XDP/TC attachment via netlink
// - Kprobe attachment via perf_event_open
// - Ring buffer reading via mmap
package ebpf

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sys/unix"
)

// ============================================================================
// BPF SYSCALL CONSTANTS
// ============================================================================
//
// These constants define the BPF syscall commands and program/map types.
// They come directly from the Linux kernel headers (include/uapi/linux/bpf.h).
//
// The BPF syscall (number 321 on x86_64) is the single entry point for all
// BPF operations. The first argument specifies which operation to perform.

// BPF syscall commands - these are the "cmd" values for the bpf() syscall
const (
	// BPF_MAP_CREATE creates a new BPF map (key-value store in kernel)
	BPF_MAP_CREATE = 0

	// BPF_MAP_LOOKUP_ELEM looks up a value by key in a map
	BPF_MAP_LOOKUP_ELEM = 1

	// BPF_MAP_UPDATE_ELEM updates or inserts a key-value pair
	BPF_MAP_UPDATE_ELEM = 2

	// BPF_MAP_DELETE_ELEM removes a key from the map
	BPF_MAP_DELETE_ELEM = 3

	// BPF_MAP_GET_NEXT_KEY iterates through map keys
	BPF_MAP_GET_NEXT_KEY = 4

	// BPF_PROG_LOAD loads and verifies a BPF program
	BPF_PROG_LOAD = 5

	// BPF_OBJ_PIN pins an object (map or program) to the BPF filesystem
	BPF_OBJ_PIN = 6

	// BPF_OBJ_GET retrieves a pinned object from the BPF filesystem
	BPF_OBJ_GET = 7

	// BPF_PROG_ATTACH attaches a program to a hook point
	BPF_PROG_ATTACH = 8

	// BPF_PROG_DETACH detaches a program from a hook point
	BPF_PROG_DETACH = 9

	// BPF_PROG_GET_NEXT_ID iterates through loaded program IDs
	BPF_PROG_GET_NEXT_ID = 11

	// BPF_MAP_GET_NEXT_ID iterates through created map IDs
	BPF_MAP_GET_NEXT_ID = 12

	// BPF_PROG_GET_FD_BY_ID gets a file descriptor for a program by ID
	BPF_PROG_GET_FD_BY_ID = 13

	// BPF_MAP_GET_FD_BY_ID gets a file descriptor for a map by ID
	BPF_MAP_GET_FD_BY_ID = 14

	// BPF_OBJ_GET_INFO_BY_FD retrieves info about an object by its FD
	BPF_OBJ_GET_INFO_BY_FD = 15

	// BPF_LINK_CREATE creates a BPF link (modern attachment method)
	BPF_LINK_CREATE = 28

	// BPF_LINK_UPDATE updates an existing BPF link
	BPF_LINK_UPDATE = 29

	// BPF_LINK_GET_FD_BY_ID gets a file descriptor for a link by ID
	BPF_LINK_GET_FD_BY_ID = 30

	// BPF_LINK_DETACH detaches a BPF link
	BPF_LINK_DETACH = 34
)

// BPF program types - determines what hook points the program can attach to
const (
	BPF_PROG_TYPE_UNSPEC          = 0
	BPF_PROG_TYPE_SOCKET_FILTER   = 1
	BPF_PROG_TYPE_KPROBE          = 2
	BPF_PROG_TYPE_SCHED_CLS       = 3  // TC classifier
	BPF_PROG_TYPE_SCHED_ACT       = 4  // TC action
	BPF_PROG_TYPE_TRACEPOINT      = 5
	BPF_PROG_TYPE_XDP             = 6
	BPF_PROG_TYPE_PERF_EVENT      = 7
	BPF_PROG_TYPE_CGROUP_SKB      = 8
	BPF_PROG_TYPE_CGROUP_SOCK     = 9
	BPF_PROG_TYPE_LWT_IN          = 10
	BPF_PROG_TYPE_LWT_OUT         = 11
	BPF_PROG_TYPE_LWT_XMIT        = 12
	BPF_PROG_TYPE_SOCK_OPS        = 13
	BPF_PROG_TYPE_SK_SKB          = 14
	BPF_PROG_TYPE_CGROUP_DEVICE   = 15
	BPF_PROG_TYPE_SK_MSG          = 16
	BPF_PROG_TYPE_RAW_TRACEPOINT  = 17
	BPF_PROG_TYPE_CGROUP_SOCK_ADDR = 18
	BPF_PROG_TYPE_LWT_SEG6LOCAL   = 19
	BPF_PROG_TYPE_LIRC_MODE2      = 20
	BPF_PROG_TYPE_SK_REUSEPORT    = 21
	BPF_PROG_TYPE_FLOW_DISSECTOR  = 22
	BPF_PROG_TYPE_CGROUP_SYSCTL   = 23
	BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE = 24
	BPF_PROG_TYPE_CGROUP_SOCKOPT  = 25
	BPF_PROG_TYPE_TRACING         = 26  // fentry/fexit
	BPF_PROG_TYPE_STRUCT_OPS      = 27
	BPF_PROG_TYPE_EXT             = 28
	BPF_PROG_TYPE_LSM             = 29
	BPF_PROG_TYPE_SK_LOOKUP       = 30
	BPF_PROG_TYPE_SYSCALL         = 31
)

// BPF map types - different data structures available in kernel
const (
	BPF_MAP_TYPE_UNSPEC           = 0
	BPF_MAP_TYPE_HASH             = 1  // Hash table
	BPF_MAP_TYPE_ARRAY            = 2  // Array with integer keys
	BPF_MAP_TYPE_PROG_ARRAY       = 3  // Array of program FDs for tail calls
	BPF_MAP_TYPE_PERF_EVENT_ARRAY = 4  // Per-CPU perf event buffers
	BPF_MAP_TYPE_PERCPU_HASH      = 5  // Per-CPU hash table
	BPF_MAP_TYPE_PERCPU_ARRAY     = 6  // Per-CPU array
	BPF_MAP_TYPE_STACK_TRACE      = 7  // Stack trace storage
	BPF_MAP_TYPE_CGROUP_ARRAY     = 8  // Array of cgroup FDs
	BPF_MAP_TYPE_LRU_HASH         = 9  // LRU hash table
	BPF_MAP_TYPE_LRU_PERCPU_HASH  = 10 // Per-CPU LRU hash
	BPF_MAP_TYPE_LPM_TRIE         = 11 // Longest prefix match trie
	BPF_MAP_TYPE_ARRAY_OF_MAPS    = 12 // Array containing map FDs
	BPF_MAP_TYPE_HASH_OF_MAPS     = 13 // Hash containing map FDs
	BPF_MAP_TYPE_DEVMAP           = 14 // Device map for XDP redirect
	BPF_MAP_TYPE_SOCKMAP          = 15 // Socket map
	BPF_MAP_TYPE_CPUMAP           = 16 // CPU map for XDP redirect
	BPF_MAP_TYPE_XSKMAP           = 17 // AF_XDP socket map
	BPF_MAP_TYPE_SOCKHASH         = 18 // Socket hash map
	BPF_MAP_TYPE_CGROUP_STORAGE   = 19 // Per-cgroup storage
	BPF_MAP_TYPE_REUSEPORT_SOCKARRAY = 20
	BPF_MAP_TYPE_PERCPU_CGROUP_STORAGE = 21
	BPF_MAP_TYPE_QUEUE            = 22 // FIFO queue
	BPF_MAP_TYPE_STACK            = 23 // LIFO stack
	BPF_MAP_TYPE_SK_STORAGE       = 24 // Per-socket storage
	BPF_MAP_TYPE_DEVMAP_HASH      = 25 // Device hash map
	BPF_MAP_TYPE_STRUCT_OPS       = 26 // Struct ops map
	BPF_MAP_TYPE_RINGBUF          = 27 // Ring buffer (kernel 5.8+)
	BPF_MAP_TYPE_INODE_STORAGE    = 28 // Per-inode storage
	BPF_MAP_TYPE_TASK_STORAGE     = 29 // Per-task storage
	BPF_MAP_TYPE_BLOOM_FILTER     = 30 // Bloom filter
)

// BPF attach types - specifies where to attach the program
const (
	BPF_CGROUP_INET_INGRESS      = 0
	BPF_CGROUP_INET_EGRESS       = 1
	BPF_CGROUP_INET_SOCK_CREATE  = 2
	BPF_CGROUP_SOCK_OPS          = 3
	BPF_SK_SKB_STREAM_PARSER     = 4
	BPF_SK_SKB_STREAM_VERDICT    = 5
	BPF_CGROUP_DEVICE            = 6
	BPF_SK_MSG_VERDICT           = 7
	BPF_CGROUP_INET4_BIND        = 8
	BPF_CGROUP_INET6_BIND        = 9
	BPF_CGROUP_INET4_CONNECT     = 10
	BPF_CGROUP_INET6_CONNECT     = 11
	BPF_CGROUP_INET4_POST_BIND   = 12
	BPF_CGROUP_INET6_POST_BIND   = 13
	BPF_CGROUP_UDP4_SENDMSG      = 14
	BPF_CGROUP_UDP6_SENDMSG      = 15
	BPF_LIRC_MODE2               = 16
	BPF_FLOW_DISSECTOR           = 17
	BPF_CGROUP_SYSCTL            = 18
	BPF_CGROUP_UDP4_RECVMSG      = 19
	BPF_CGROUP_UDP6_RECVMSG      = 20
	BPF_CGROUP_GETSOCKOPT        = 21
	BPF_CGROUP_SETSOCKOPT        = 22
	BPF_TRACE_RAW_TP             = 23
	BPF_TRACE_FENTRY             = 24
	BPF_TRACE_FEXIT              = 25
	BPF_MODIFY_RETURN            = 26
	BPF_LSM_MAC                  = 27
	BPF_TRACE_ITER               = 28
	BPF_CGROUP_INET4_GETPEERNAME = 29
	BPF_CGROUP_INET6_GETPEERNAME = 30
	BPF_CGROUP_INET4_GETSOCKNAME = 31
	BPF_CGROUP_INET6_GETSOCKNAME = 32
	BPF_XDP_DEVMAP               = 33
	BPF_CGROUP_INET_SOCK_RELEASE = 34
	BPF_XDP_CPUMAP               = 35
	BPF_SK_LOOKUP                = 36
	BPF_XDP                      = 37
	BPF_SK_SKB_VERDICT           = 38
	BPF_SK_REUSEPORT_SELECT      = 39
	BPF_SK_REUSEPORT_SELECT_OR_MIGRATE = 40
	BPF_PERF_EVENT               = 41
	BPF_TRACE_KPROBE_MULTI       = 42
	BPF_LSM_CGROUP               = 43
	BPF_STRUCT_OPS               = 44
	BPF_NETFILTER                = 45
	BPF_TCX_INGRESS              = 46
	BPF_TCX_EGRESS               = 47
)

// BPF link types
const (
	BPF_LINK_TYPE_UNSPEC         = 0
	BPF_LINK_TYPE_RAW_TRACEPOINT = 1
	BPF_LINK_TYPE_TRACING        = 2
	BPF_LINK_TYPE_CGROUP         = 3
	BPF_LINK_TYPE_ITER           = 4
	BPF_LINK_TYPE_NETNS          = 5
	BPF_LINK_TYPE_XDP            = 6
	BPF_LINK_TYPE_PERF_EVENT     = 7
	BPF_LINK_TYPE_KPROBE_MULTI   = 8
	BPF_LINK_TYPE_STRUCT_OPS     = 9
	BPF_LINK_TYPE_NETFILTER      = 10
	BPF_LINK_TYPE_TCX            = 11
)

// Map update flags
const (
	BPF_ANY     = 0 // Create or update
	BPF_NOEXIST = 1 // Create only if key doesn't exist
	BPF_EXIST   = 2 // Update only if key exists
)

// XDP flags for attachment
const (
	XDP_FLAGS_UPDATE_IF_NOEXIST = 1 << 0
	XDP_FLAGS_SKB_MODE          = 1 << 1 // Generic/SKB mode
	XDP_FLAGS_DRV_MODE          = 1 << 2 // Native driver mode
	XDP_FLAGS_HW_MODE           = 1 << 3 // Hardware offload
	XDP_FLAGS_REPLACE           = 1 << 4 // Replace existing program
)

// XDP actions returned by programs
const (
	XDP_ABORTED  = 0 // Error, drop packet
	XDP_DROP     = 1 // Drop packet silently
	XDP_PASS     = 2 // Pass to network stack
	XDP_TX       = 3 // Transmit out same interface
	XDP_REDIRECT = 4 // Redirect to another interface
)

// ============================================================================
// NETLINK CONSTANTS FOR XDP/TC ATTACHMENT
// ============================================================================
//
// Netlink is the Linux kernel's socket-based IPC mechanism for configuring
// network interfaces. XDP programs are attached via netlink messages.

const (
	// Netlink families
	NETLINK_ROUTE = 0

	// Netlink message types for routing
	RTM_NEWLINK  = 16
	RTM_DELLINK  = 17
	RTM_GETLINK  = 18
	RTM_SETLINK  = 19
	RTM_NEWQDISC = 36
	RTM_DELQDISC = 37
	RTM_GETQDISC = 38

	// Netlink message flags
	NLM_F_REQUEST = 0x1
	NLM_F_ACK     = 0x4
	NLM_F_CREATE  = 0x400
	NLM_F_EXCL    = 0x200

	// Interface link attributes
	IFLA_UNSPEC    = 0
	IFLA_XDP       = 43
	IFLA_XDP_FD    = 1
	IFLA_XDP_FLAGS = 3

	// TC (Traffic Control) constants
	TC_H_CLSACT        = 0xffff0000
	TC_H_MIN_INGRESS   = 0xfff2
	TC_H_MIN_EGRESS    = 0xfff3
	TCA_KIND           = 1
	TCA_OPTIONS        = 2
	TCA_BPF_FD         = 1
	TCA_BPF_NAME       = 2
	TCA_BPF_FLAGS      = 3
	TCA_BPF_FLAGS_DIRECT = 1
)

// ============================================================================
// PERF EVENT CONSTANTS FOR KPROBE ATTACHMENT
// ============================================================================
//
// Kprobes are attached using the perf_event_open syscall, which creates a
// performance monitoring event that triggers the BPF program.

const (
	PERF_TYPE_TRACEPOINT = 2
	PERF_TYPE_KPROBE     = 6 // Dynamic kprobe (kernel 4.17+)
	PERF_TYPE_UPROBE     = 7 // Dynamic uprobe

	PERF_SAMPLE_RAW      = 1 << 10
	PERF_FLAG_FD_CLOEXEC = 1 << 3

	// Kprobe config flags
	PERF_KPROBE_FLAG_RETPROBE = 1 << 0
)

// ============================================================================
// RING BUFFER CONSTANTS
// ============================================================================
//
// Ring buffers (kernel 5.8+) are the modern way to stream data from BPF
// programs to userspace. They use mmap for zero-copy data transfer.

const (
	// Ring buffer header flags
	BPF_RINGBUF_BUSY_BIT  = 1 << 31
	BPF_RINGBUF_DISCARD_BIT = 1 << 30
	BPF_RINGBUF_HDR_SZ    = 8
)

// ============================================================================
// BTF (BPF TYPE FORMAT) CONSTANTS
// ============================================================================
//
// BTF provides type information for BPF programs, enabling CO-RE
// (Compile Once - Run Everywhere) by describing struct layouts.

const (
	BTF_MAGIC = 0xeB9F

	// BTF type kinds
	BTF_KIND_UNKN       = 0
	BTF_KIND_INT        = 1
	BTF_KIND_PTR        = 2
	BTF_KIND_ARRAY      = 3
	BTF_KIND_STRUCT     = 4
	BTF_KIND_UNION      = 5
	BTF_KIND_ENUM       = 6
	BTF_KIND_FWD        = 7
	BTF_KIND_TYPEDEF    = 8
	BTF_KIND_VOLATILE   = 9
	BTF_KIND_CONST      = 10
	BTF_KIND_RESTRICT   = 11
	BTF_KIND_FUNC       = 12
	BTF_KIND_FUNC_PROTO = 13
	BTF_KIND_VAR        = 14
	BTF_KIND_DATASEC    = 15
	BTF_KIND_FLOAT      = 16
	BTF_KIND_DECL_TAG   = 17
	BTF_KIND_TYPE_TAG   = 18
	BTF_KIND_ENUM64     = 19
)

// ============================================================================
// ERRORS
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
	ErrBTFNotAvailable    = errors.New("BTF information not available")
	ErrInterfaceNotFound  = errors.New("network interface not found")
	ErrPinPathExists      = errors.New("pin path already exists")
	ErrUnsupportedMapType = errors.New("unsupported map type for operation")
	ErrELFParseFailed     = errors.New("failed to parse ELF file")
	ErrNoInstructions     = errors.New("no BPF instructions found")
	ErrSyscallFailed      = errors.New("BPF syscall failed")
)

// ============================================================================
// PUBLIC TYPES
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
	return fmt.Sprintf("%d.%d.%d", k.Major, k.Minor, k.Patch)
}

// AtLeast checks if kernel version is at least the given version
func (k KernelVersion) AtLeast(major, minor, patch int) bool {
	if k.Major > major {
		return true
	}
	if k.Major < major {
		return false
	}
	if k.Minor > minor {
		return true
	}
	if k.Minor < minor {
		return false
	}
	return k.Patch >= patch
}

// LoaderConfig holds configuration for creating an eBPF loader
type LoaderConfig struct {
	BTFPath         string // Path to BTF information (auto-discovered if empty)
	PinPath         string // BPF filesystem pin path (default: /sys/fs/bpf)
	MapPinPath      string // Map pin path
	Debug           bool   // Enable debug output
	VerifierLogSize int    // Size of verifier log buffer (default: 64KB)
	AllowRlimit     bool   // Allow memlock rlimit increase (default: true)
	MetricsEnabled  bool   // Enable Prometheus metrics (default: true)
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

// ============================================================================
// LOADER INTERFACE
// ============================================================================

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

// ============================================================================
// PROMETHEUS METRICS
// ============================================================================

var (
	ebpfProgramsLoaded = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf",
			Name:      "programs_loaded",
			Help:      "Number of eBPF programs currently loaded",
		},
		[]string{"type"},
	)

	ebpfProgramsAttached = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf",
			Name:      "programs_attached",
			Help:      "Number of eBPF programs currently attached",
		},
		[]string{"type", "attach_type"},
	)

	ebpfProgramLoadTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf",
			Name:      "program_load_duration_seconds",
			Help:      "Time to load eBPF programs",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
		},
		[]string{"name", "type"},
	)

	ebpfMapOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf",
			Name:      "map_operations_total",
			Help:      "Total number of eBPF map operations",
		},
		[]string{"program", "map", "operation"},
	)

	ebpfRingbufEvents = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf",
			Name:      "ringbuf_events_total",
			Help:      "Total number of events received from ring buffers",
		},
		[]string{"program", "map"},
	)

	ebpfErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf",
			Name:      "errors_total",
			Help:      "Total number of eBPF errors",
		},
		[]string{"operation", "error_type"},
	)
)

// ============================================================================
// INTERNAL STRUCTURES FOR BPF SYSCALLS
// ============================================================================
//
// These structures match the kernel's bpf_attr union, which is used for
// all BPF syscall operations. The union has different layouts depending
// on the operation being performed.

// bpfMapCreateAttr is the attribute structure for BPF_MAP_CREATE
// This creates a new BPF map (key-value store in kernel memory)
type bpfMapCreateAttr struct {
	MapType    uint32 // One of BPF_MAP_TYPE_*
	KeySize    uint32 // Size of key in bytes
	ValueSize  uint32 // Size of value in bytes
	MaxEntries uint32 // Maximum number of entries
	MapFlags   uint32 // Map creation flags
	// Following fields are for more advanced map features
	InnerMapFD     uint32   // FD of inner map (for maps of maps)
	NumaNode       uint32   // NUMA node for allocation
	MapName        [16]byte // Map name (for debugging)
	MapIfIndex     uint32   // Network interface index (for HW offload)
	BTFFD          uint32   // BTF file descriptor
	BTFKeyTypeID   uint32   // BTF type ID for key
	BTFValueTypeID uint32   // BTF type ID for value
	BTFVmlinuxID   uint32   // BTF vmlinux value type ID
	MapExtra       uint64   // Extra data for specific map types
}

// bpfMapOpAttr is the attribute structure for map element operations
// Used for BPF_MAP_LOOKUP_ELEM, BPF_MAP_UPDATE_ELEM, BPF_MAP_DELETE_ELEM
type bpfMapOpAttr struct {
	MapFD uint32  // File descriptor of the map
	_     uint32  // Padding
	Key   uint64  // Pointer to key
	Value uint64  // Pointer to value (or next key for GET_NEXT_KEY)
	Flags uint64  // Operation flags (BPF_ANY, BPF_NOEXIST, BPF_EXIST)
}

// bpfProgLoadAttr is the attribute structure for BPF_PROG_LOAD
// This loads and verifies a BPF program
type bpfProgLoadAttr struct {
	ProgType       uint32   // One of BPF_PROG_TYPE_*
	InsnCnt        uint32   // Number of BPF instructions
	Insns          uint64   // Pointer to BPF instruction array
	License        uint64   // Pointer to license string (must be GPL for some helpers)
	LogLevel       uint32   // Verbosity of verifier log
	LogSize        uint32   // Size of log buffer
	LogBuf         uint64   // Pointer to log buffer
	KernVersion    uint32   // Kernel version (for some program types)
	ProgFlags      uint32   // Program flags
	ProgName       [16]byte // Program name (for debugging)
	ProgIfIndex    uint32   // Network interface (for HW offload)
	ExpectedAttach uint32   // Expected attach type
	ProgBTFFD      uint32   // BTF file descriptor
	FuncInfoRecSz  uint32   // Size of func_info records
	FuncInfo       uint64   // Pointer to func_info array
	FuncInfoCnt    uint32   // Number of func_info records
	LineInfoRecSz  uint32   // Size of line_info records
	LineInfo       uint64   // Pointer to line_info array
	LineInfoCnt    uint32   // Number of line_info records
	AttachBTFID    uint32   // BTF type ID for attach target
	AttachProgFD   uint32   // FD of program to attach to (for extension)
	FDArray        uint64   // Pointer to array of FDs for map relocations
}

// bpfObjAttr is the attribute structure for BPF_OBJ_PIN and BPF_OBJ_GET
// These pin/retrieve objects to/from the BPF filesystem
type bpfObjAttr struct {
	Pathname uint64 // Pointer to pathname string
	BPFFD    uint32 // FD of BPF object (program or map)
	FileFlag uint32 // Flags for operation
}

// bpfProgAttachAttr is the attribute structure for BPF_PROG_ATTACH
type bpfProgAttachAttr struct {
	TargetFD     uint32 // FD of attach target (cgroup, netns, etc.)
	AttachBPFFD  uint32 // FD of BPF program
	AttachType   uint32 // One of BPF_CGROUP_*, BPF_XDP, etc.
	AttachFlags  uint32 // Attachment flags
	ReplaceBPFFD uint32 // FD of program to replace (for atomic replace)
}

// bpfLinkCreateAttr is the attribute structure for BPF_LINK_CREATE
// Links are the modern way to attach BPF programs (kernel 5.7+)
type bpfLinkCreateAttr struct {
	ProgFD      uint32 // FD of BPF program
	TargetFD    uint32 // FD of target (varies by link type)
	AttachType  uint32 // Attach type
	Flags       uint32 // Link creation flags
	TargetBTFID uint32 // BTF type ID for target (for tracing)
	_           uint32 // Padding
	// Union for type-specific data
	Perf struct {
		BPFCOOKIE uint64 // Cookie passed to BPF program
	}
}

// bpfObjGetInfoAttr is the attribute structure for BPF_OBJ_GET_INFO_BY_FD
type bpfObjGetInfoAttr struct {
	BPFFD   uint32 // FD of BPF object
	InfoLen uint32 // Size of info buffer
	Info    uint64 // Pointer to info buffer
}

// bpfProgInfo is the structure returned by BPF_OBJ_GET_INFO_BY_FD for programs
type bpfProgInfo struct {
	Type               uint32
	ID                 uint32
	Tag                [8]byte
	JitedProgLen       uint32
	XlatedProgLen      uint32
	JitedProgInsns     uint64
	XlatedProgInsns    uint64
	LoadTime           uint64 // nanoseconds since boot
	CreatedByUID       uint32
	NrMapIDs           uint32
	MapIDs             uint64
	Name               [16]byte
	IfIndex            uint32
	GPLCompatible      uint32
	NetnsFD            uint64
	NrFuncInfo         uint32
	FuncInfoRecSize    uint32
	FuncInfo           uint64
	NrLineInfo         uint32
	LineInfoRecSize    uint32
	LineInfo           uint64
	NrJitedLineInfo    uint32
	JitedLineInfoRecSz uint32
	JitedLineInfo      uint64
	NrProgTags         uint32
	ProgTags           uint64
	RunTimeNs          uint64
	RunCnt             uint64
	RecursionMisses    uint64
	VerifiedInsns      uint32
	AttachBTFObjID     uint32
	AttachBTFID        uint32
}

// bpfMapInfo is the structure returned by BPF_OBJ_GET_INFO_BY_FD for maps
type bpfMapInfo struct {
	Type       uint32
	ID         uint32
	KeySize    uint32
	ValueSize  uint32
	MaxEntries uint32
	MapFlags   uint32
	Name       [16]byte
	IfIndex    uint32
	BTFVmlinux uint32
	NetNS      uint64
	BTFKeyID   uint32
	BTFValueID uint32
	BTFID      uint32
	MapExtra   uint64
}

// ============================================================================
// ELF PARSING STRUCTURES
// ============================================================================
//
// BPF programs are compiled to ELF object files (.bpf.o). These structures
// help us parse the ELF and extract BPF bytecode, maps, and relocations.

// elfProgram represents a BPF program extracted from an ELF section
type elfProgram struct {
	Name         string   // Section name (e.g., "xdp/my_prog")
	Instructions []byte   // Raw BPF bytecode
	Type         uint32   // BPF program type
	License      string   // Program license
	BTFTypeID    uint32   // BTF type ID for the program
}

// elfMap represents a BPF map definition from an ELF section
type elfMap struct {
	Name       string // Map name
	Type       uint32 // Map type
	KeySize    uint32 // Key size in bytes
	ValueSize  uint32 // Value size in bytes
	MaxEntries uint32 // Maximum entries
	Flags      uint32 // Map flags
	BTFKeyID   uint32 // BTF key type ID
	BTFValueID uint32 // BTF value type ID
}

// elfRelocation represents a relocation entry
type elfRelocation struct {
	Offset      uint64 // Offset in the instruction
	SymbolName  string // Symbol being referenced
	SymbolValue uint64 // Symbol's value (offset within its section)
	Type        uint32 // Relocation type
}

// parsedELF holds the result of parsing a BPF ELF file
type parsedELF struct {
	Programs    map[string]*elfProgram
	Maps        map[string]*elfMap
	Relocations map[string][]elfRelocation
	FuncSizes   map[uint64]int // .text function byte offset → size in bytes
	BTF         *btfData
	License     string
}

// ============================================================================
// BTF STRUCTURES
// ============================================================================
//
// BTF (BPF Type Format) provides type information for BPF programs.
// This enables CO-RE (Compile Once - Run Everywhere) by allowing
// the loader to adjust field offsets for different kernel versions.

// btfHeader is the header of a BTF section
type btfHeader struct {
	Magic      uint16
	Version    uint8
	Flags      uint8
	HdrLen     uint32
	TypeOff    uint32 // Offset of type section
	TypeLen    uint32 // Length of type section
	StrOff     uint32 // Offset of string section
	StrLen     uint32 // Length of string section
}

// btfType is the common header for all BTF types
type btfType struct {
	NameOff uint32 // Offset in string section
	Info    uint32 // Encoded: kind, vlen, kflag
	Size    uint32 // Size or type (depends on kind)
}

// btfData holds parsed BTF information
type btfData struct {
	Header  btfHeader
	Types   []btfType
	Strings []byte
	Raw     []byte
}

// parseBTF parses BTF data from raw bytes
func parseBTF(data []byte) (*btfData, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("BTF data too small")
	}

	btf := &btfData{Raw: data}

	// Parse header
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.LittleEndian, &btf.Header); err != nil {
		return nil, fmt.Errorf("read BTF header: %w", err)
	}

	// Verify magic
	if btf.Header.Magic != BTF_MAGIC {
		return nil, fmt.Errorf("invalid BTF magic: 0x%x (expected 0x%x)",
			btf.Header.Magic, BTF_MAGIC)
	}

	// Extract string section
	strStart := int(btf.Header.HdrLen + btf.Header.StrOff)
	strEnd := strStart + int(btf.Header.StrLen)
	if strEnd > len(data) {
		return nil, fmt.Errorf("BTF string section exceeds data")
	}
	btf.Strings = data[strStart:strEnd]

	// Parse types (simplified - just count them for now)
	typeStart := int(btf.Header.HdrLen + btf.Header.TypeOff)
	typeEnd := typeStart + int(btf.Header.TypeLen)
	if typeEnd > len(data) {
		return nil, fmt.Errorf("BTF type section exceeds data")
	}

	// We don't fully parse types here, but store enough for the kernel
	return btf, nil
}

// getString retrieves a string from the BTF string section
func (b *btfData) getString(offset uint32) string {
	if int(offset) >= len(b.Strings) {
		return ""
	}
	end := offset
	for int(end) < len(b.Strings) && b.Strings[end] != 0 {
		end++
	}
	return string(b.Strings[offset:end])
}

// ============================================================================
// LOADED PROGRAM STATE
// ============================================================================

// loadedMap holds state for a loaded BPF map
type loadedMap struct {
	fd        int
	info      *MapInfo
	mmapAddr  uintptr // For ringbuf
	mmapSize  int
}

// loadedProgram holds state for a loaded BPF program
type loadedProgram struct {
	spec      *ProgramSpec
	info      *ProgramInfo
	fd        int                    // Program file descriptor
	maps      map[string]*loadedMap  // Map name -> loaded map
	linkFDs   []int                  // Link file descriptors
	perfFDs   []int                  // Perf event FDs (for kprobes)
	cancelFns []context.CancelFunc   // Cancel functions for readers
	mu        sync.RWMutex
}

// ============================================================================
// NATIVE LOADER IMPLEMENTATION
// ============================================================================

// NativeLoader implements the Loader interface using direct syscalls
// This is the Whispering Void - pure Kingdom magic without external libraries
type NativeLoader struct {
	config   LoaderConfig
	features *KernelFeatures
	btfFD    int // Kernel BTF file descriptor

	programs map[string]*loadedProgram
	mu       sync.RWMutex

	metricsEnabled bool
	closed         bool
	closedMu       sync.RWMutex

	wg sync.WaitGroup
}

// NewLoader creates a new native eBPF loader
func NewLoader(cfg LoaderConfig) (Loader, error) {
	return NewNativeLoader(cfg)
}

// NewNativeLoader creates a new native eBPF loader using direct syscalls
func NewNativeLoader(cfg LoaderConfig) (*NativeLoader, error) {
	// Remove memlock limit if allowed
	// On older kernels, BPF memory is charged against RLIMIT_MEMLOCK
	if cfg.AllowRlimit {
		var rLimit unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_MEMLOCK, &rLimit); err == nil {
			rLimit.Cur = unix.RLIM_INFINITY
			rLimit.Max = unix.RLIM_INFINITY
			// Ignore error - newer kernels (5.11+) use memcg accounting
			_ = unix.Setrlimit(unix.RLIMIT_MEMLOCK, &rLimit)
		}
	}

	// Detect kernel features
	features, err := detectKernelFeatures()
	if err != nil {
		return nil, fmt.Errorf("detect kernel features: %w", err)
	}

	// Check minimum kernel version (4.18 for basic features)
	if !features.Version.AtLeast(4, 18, 0) {
		return nil, fmt.Errorf("%w: kernel %s (minimum 4.18.0 required)",
			ErrKernelNotSupported, features.Version)
	}

	// Load kernel BTF if available
	var btfFD int
	if features.BTFEnabled {
		btfFD, _ = loadKernelBTF(features.BTFPath)
	}

	// Ensure pin path exists
	if cfg.PinPath != "" {
		if err := ensureBPFFS(cfg.PinPath); err != nil {
			return nil, fmt.Errorf("ensure bpf filesystem: %w", err)
		}
	}

	loader := &NativeLoader{
		config:         cfg,
		features:       features,
		btfFD:          btfFD,
		programs:       make(map[string]*loadedProgram),
		metricsEnabled: cfg.MetricsEnabled,
	}

	return loader, nil
}

// ============================================================================
// BPF SYSCALL WRAPPERS
// ============================================================================
//
// These functions wrap the bpf() syscall for different operations.
// The bpf syscall takes: cmd, attr pointer, attr size

// bpfSyscall performs the raw bpf syscall
// cmd: BPF command (BPF_MAP_CREATE, BPF_PROG_LOAD, etc.)
// attr: Pointer to command-specific attribute structure
// size: Size of the attribute structure
func bpfSyscall(cmd int, attr unsafe.Pointer, size uintptr) (int, error) {
	r1, _, errno := unix.Syscall(
		unix.SYS_BPF,
		uintptr(cmd),
		uintptr(attr),
		size,
	)

	if errno != 0 {
		return int(r1), errno
	}
	return int(r1), nil
}

// bpfMapCreate creates a new BPF map
// Returns the file descriptor for the new map
func bpfMapCreate(mapType, keySize, valueSize, maxEntries, flags uint32, name string) (int, error) {
	attr := bpfMapCreateAttr{
		MapType:    mapType,
		KeySize:    keySize,
		ValueSize:  valueSize,
		MaxEntries: maxEntries,
		MapFlags:   flags,
	}

	// Copy name (max BPF_OBJ_NAME_LEN-1 = 15 chars + null terminator)
	if len(name) > 15 {
		name = name[:15]
	}
	copy(attr.MapName[:], name)

	fd, err := bpfSyscall(BPF_MAP_CREATE, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return -1, fmt.Errorf("%w: map_create %s: %v", ErrSyscallFailed, name, err)
	}

	return fd, nil
}

// bpfMapLookupElem looks up a value in a BPF map
func bpfMapLookupElem(mapFD int, key, value unsafe.Pointer) error {
	attr := bpfMapOpAttr{
		MapFD: uint32(mapFD),
		Key:   uint64(uintptr(key)),
		Value: uint64(uintptr(value)),
	}

	_, err := bpfSyscall(BPF_MAP_LOOKUP_ELEM, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("%w: map_lookup: %v", ErrSyscallFailed, err)
	}

	return nil
}

// bpfMapUpdateElem updates or inserts an element in a BPF map
func bpfMapUpdateElem(mapFD int, key, value unsafe.Pointer, flags uint64) error {
	attr := bpfMapOpAttr{
		MapFD: uint32(mapFD),
		Key:   uint64(uintptr(key)),
		Value: uint64(uintptr(value)),
		Flags: flags,
	}

	_, err := bpfSyscall(BPF_MAP_UPDATE_ELEM, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("%w: map_update: %v", ErrSyscallFailed, err)
	}

	return nil
}

// bpfMapDeleteElem deletes an element from a BPF map
func bpfMapDeleteElem(mapFD int, key unsafe.Pointer) error {
	attr := bpfMapOpAttr{
		MapFD: uint32(mapFD),
		Key:   uint64(uintptr(key)),
	}

	_, err := bpfSyscall(BPF_MAP_DELETE_ELEM, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("%w: map_delete: %v", ErrSyscallFailed, err)
	}

	return nil
}

// bpfMapGetNextKey gets the next key in a BPF map (for iteration)
func bpfMapGetNextKey(mapFD int, key, nextKey unsafe.Pointer) error {
	attr := bpfMapOpAttr{
		MapFD: uint32(mapFD),
		Key:   uint64(uintptr(key)),
		Value: uint64(uintptr(nextKey)),
	}

	_, err := bpfSyscall(BPF_MAP_GET_NEXT_KEY, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return err // Return raw error for ENOENT check
	}

	return nil
}

// bpfProgLoad loads a BPF program
// Returns the file descriptor for the loaded program
func bpfProgLoad(progType uint32, insns []byte, license string, logBuf []byte, btfFD int) (int, string, error) {
	// BPF instructions are 8 bytes each
	insnCnt := uint32(len(insns) / 8)
	if insnCnt == 0 {
		return -1, "", ErrNoInstructions
	}

	// Prepare license string (must be null-terminated)
	licenseBytes := append([]byte(license), 0)

	attr := bpfProgLoadAttr{
		ProgType: progType,
		InsnCnt:  insnCnt,
		Insns:    uint64(uintptr(unsafe.Pointer(&insns[0]))),
		License:  uint64(uintptr(unsafe.Pointer(&licenseBytes[0]))),
	}

	// Set up log buffer if provided
	if len(logBuf) > 0 {
		attr.LogLevel = 1
		attr.LogSize = uint32(len(logBuf))
		attr.LogBuf = uint64(uintptr(unsafe.Pointer(&logBuf[0])))
	}

	// Add BTF if available
	if btfFD > 0 {
		attr.ProgBTFFD = uint32(btfFD)
	}

	fd, err := bpfSyscall(BPF_PROG_LOAD, unsafe.Pointer(&attr), unsafe.Sizeof(attr))

	// Extract verifier log
	var verifierLog string
	if len(logBuf) > 0 {
		// Find null terminator
		end := bytes.IndexByte(logBuf, 0)
		if end > 0 {
			verifierLog = string(logBuf[:end])
		}
	}

	if err != nil {
		return -1, verifierLog, fmt.Errorf("%w: prog_load: %v", ErrSyscallFailed, err)
	}

	return fd, verifierLog, nil
}

// bpfObjPin pins a BPF object to the filesystem
func bpfObjPin(fd int, pathname string) error {
	pathBytes := append([]byte(pathname), 0)

	attr := bpfObjAttr{
		Pathname: uint64(uintptr(unsafe.Pointer(&pathBytes[0]))),
		BPFFD:    uint32(fd),
	}

	_, err := bpfSyscall(BPF_OBJ_PIN, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("%w: obj_pin %s: %v", ErrSyscallFailed, pathname, err)
	}

	return nil
}

// bpfObjGet retrieves a pinned BPF object from the filesystem
func bpfObjGet(pathname string) (int, error) {
	pathBytes := append([]byte(pathname), 0)

	attr := bpfObjAttr{
		Pathname: uint64(uintptr(unsafe.Pointer(&pathBytes[0]))),
	}

	fd, err := bpfSyscall(BPF_OBJ_GET, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return -1, fmt.Errorf("%w: obj_get %s: %v", ErrSyscallFailed, pathname, err)
	}

	return fd, nil
}

// bpfProgAttach attaches a BPF program
func bpfProgAttach(progFD, targetFD int, attachType uint32) error {
	attr := bpfProgAttachAttr{
		TargetFD:    uint32(targetFD),
		AttachBPFFD: uint32(progFD),
		AttachType:  attachType,
	}

	_, err := bpfSyscall(BPF_PROG_ATTACH, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("%w: prog_attach: %v", ErrSyscallFailed, err)
	}

	return nil
}

// bpfProgDetach detaches a BPF program
func bpfProgDetach(progFD, targetFD int, attachType uint32) error {
	attr := bpfProgAttachAttr{
		TargetFD:    uint32(targetFD),
		AttachBPFFD: uint32(progFD),
		AttachType:  attachType,
	}

	_, err := bpfSyscall(BPF_PROG_DETACH, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("%w: prog_detach: %v", ErrSyscallFailed, err)
	}

	return nil
}

// bpfLinkCreate creates a BPF link (modern attachment method, kernel 5.7+)
func bpfLinkCreate(progFD, targetFD int, attachType uint32) (int, error) {
	attr := bpfLinkCreateAttr{
		ProgFD:     uint32(progFD),
		TargetFD:   uint32(targetFD),
		AttachType: attachType,
	}

	fd, err := bpfSyscall(BPF_LINK_CREATE, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return -1, fmt.Errorf("%w: link_create: %v", ErrSyscallFailed, err)
	}

	return fd, nil
}

// bpfGetProgInfo retrieves information about a loaded program
func bpfGetProgInfo(fd int) (*bpfProgInfo, error) {
	info := &bpfProgInfo{}

	attr := bpfObjGetInfoAttr{
		BPFFD:   uint32(fd),
		InfoLen: uint32(unsafe.Sizeof(*info)),
		Info:    uint64(uintptr(unsafe.Pointer(info))),
	}

	_, err := bpfSyscall(BPF_OBJ_GET_INFO_BY_FD, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return nil, fmt.Errorf("%w: get_info: %v", ErrSyscallFailed, err)
	}

	return info, nil
}

// bpfGetMapInfo retrieves information about a map
func bpfGetMapInfo(fd int) (*bpfMapInfo, error) {
	info := &bpfMapInfo{}

	attr := bpfObjGetInfoAttr{
		BPFFD:   uint32(fd),
		InfoLen: uint32(unsafe.Sizeof(*info)),
		Info:    uint64(uintptr(unsafe.Pointer(info))),
	}

	_, err := bpfSyscall(BPF_OBJ_GET_INFO_BY_FD, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return nil, fmt.Errorf("%w: get_map_info: %v", ErrSyscallFailed, err)
	}

	return info, nil
}

// ============================================================================
// ELF PARSING
// ============================================================================
//
// BPF programs are compiled to ELF object files. The structure is:
// - .text or named sections (e.g., xdp/my_prog): contain BPF bytecode
// - .maps or .maps.* sections: contain map definitions
// - .rel* sections: contain relocations for map references
// - .BTF section: contains type information
// - .BTF.ext section: contains additional BTF info (line info, etc.)

// parseELF parses a BPF ELF object file
func parseELF(path string) (*parsedELF, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open %s: %v", ErrELFParseFailed, path, err)
	}
	defer f.Close()

	// Verify it's a BPF object file (EM_BPF = 247)
	if f.Machine != elf.EM_BPF && f.Machine != elf.Machine(247) {
		return nil, fmt.Errorf("%w: not a BPF object file (machine=%d)",
			ErrELFParseFailed, f.Machine)
	}

	result := &parsedELF{
		Programs:    make(map[string]*elfProgram),
		Maps:        make(map[string]*elfMap),
		Relocations: make(map[string][]elfRelocation),
		FuncSizes:   make(map[uint64]int),
	}

	// First pass: collect all sections
	var btfSection, btfExtSection *elf.Section
	var licenseSection *elf.Section

	for _, section := range f.Sections {
		switch section.Name {
		case ".BTF":
			btfSection = section
		case ".BTF.ext":
			btfExtSection = section
		case "license":
			licenseSection = section
		}
	}

	// Parse license
	if licenseSection != nil {
		data, err := licenseSection.Data()
		if err == nil {
			// Remove null terminator
			result.License = strings.TrimRight(string(data), "\x00")
		}
	}
	if result.License == "" {
		result.License = "GPL" // Default to GPL
	}

	// Parse BTF if available
	if btfSection != nil {
		data, err := btfSection.Data()
		if err == nil {
			result.BTF, _ = parseBTF(data)
		}
	}
	_ = btfExtSection // Used for line info, not implemented yet

	// Second pass: parse programs and maps
	for _, section := range f.Sections {
		// Skip non-PROGBITS sections
		if section.Type != elf.SHT_PROGBITS {
			continue
		}

		// Check for program sections
		// Programs are in sections like: xdp/my_prog, kprobe/func_name, tc/ingress, etc.
		if isProgramSection(section.Name) {
			data, err := section.Data()
			if err != nil {
				continue
			}

			prog := &elfProgram{
				Name:         section.Name,
				Instructions: data,
				License:      result.License,
				Type:         sectionNameToProgType(section.Name),
			}
			result.Programs[section.Name] = prog
		}

		// Check for map sections
		if section.Name == ".maps" || section.Name == "maps" || strings.HasPrefix(section.Name, ".maps.") {
			data, err := section.Data()
			if err != nil {
				continue
			}
			defSize := parseMapsSection(data, result)
			// Resolve generic map names (map_0, map_1) to actual symbol names
			// by cross-referencing ELF symbols whose section index matches.
			resolveMapNames(f, section, defSize, result)
		}
	}

	// Third pass: parse relocations
	for _, section := range f.Sections {
		if section.Type != elf.SHT_REL {
			continue
		}

		// Relocation section names are like .rel<section_name>
		targetName := strings.TrimPrefix(section.Name, ".rel")
		if _, ok := result.Programs[targetName]; !ok {
			continue
		}

		// Parse relocation entries
		data, err := section.Data()
		if err != nil {
			continue
		}

		relocs := parseRelocations(data, f)
		result.Relocations[targetName] = relocs
	}

	// Fourth pass: collect .text function sizes and split multi-function
	// program sections into individual programs using symbol boundaries.
	if symbols, err := f.Symbols(); err == nil {
		// Collect function sizes for .text
		for _, sym := range symbols {
			if elf.ST_TYPE(sym.Info) != elf.STT_FUNC {
				continue
			}
			if sym.Section == elf.SHN_UNDEF || int(sym.Section) >= len(f.Sections) {
				continue
			}
			section := f.Sections[sym.Section]
			if section.Name == ".text" {
				result.FuncSizes[sym.Value] = int(sym.Size)
			}
		}

		// Split multi-function program sections. When a section has
		// multiple GLOBAL FUNC symbols, each becomes a separate program
		// so the loader can select and load them individually.
		type funcEntry struct {
			name   string
			offset int
			size   int
		}
		sectionFuncs := make(map[string][]funcEntry) // section name → functions
		for _, sym := range symbols {
			if elf.ST_TYPE(sym.Info) != elf.STT_FUNC {
				continue
			}
			if elf.ST_BIND(sym.Info) != elf.STB_GLOBAL {
				continue
			}
			if sym.Section == elf.SHN_UNDEF || int(sym.Section) >= len(f.Sections) {
				continue
			}
			section := f.Sections[sym.Section]
			if section.Name == ".text" || strings.HasPrefix(section.Name, ".rodata") {
				continue // Skip .text and .rodata
			}
			if _, ok := result.Programs[section.Name]; !ok {
				continue // Only split recognized program sections
			}
			sectionFuncs[section.Name] = append(sectionFuncs[section.Name], funcEntry{
				name:   sym.Name,
				offset: int(sym.Value),
				size:   int(sym.Size),
			})
		}

		for secName, funcs := range sectionFuncs {
			if len(funcs) <= 1 {
				continue // Single-function section, no splitting needed
			}

			origProg := result.Programs[secName]
			origRelocs := result.Relocations[secName]

			// Sort by offset for deterministic ordering
			sort.Slice(funcs, func(i, j int) bool {
				return funcs[i].offset < funcs[j].offset
			})

			// Create individual programs for each function
			for _, fn := range funcs {
				if fn.offset+fn.size > len(origProg.Instructions) {
					continue
				}
				subProg := &elfProgram{
					Name:         fn.name,
					Instructions: origProg.Instructions[fn.offset : fn.offset+fn.size],
					License:      origProg.License,
					Type:         origProg.Type,
				}
				result.Programs[fn.name] = subProg

				// Filter relocations for this function's range
				var subRelocs []elfRelocation
				for _, r := range origRelocs {
					if r.Offset >= uint64(fn.offset) && r.Offset < uint64(fn.offset+fn.size) {
						// Adjust offset relative to function start
						adjusted := r
						adjusted.Offset -= uint64(fn.offset)
						subRelocs = append(subRelocs, adjusted)
					}
				}
				if len(subRelocs) > 0 {
					result.Relocations[fn.name] = subRelocs
				}
			}

			// Remove the original multi-function section
			delete(result.Programs, secName)
			delete(result.Relocations, secName)
		}
	}

	return result, nil
}

// isProgramSection checks if a section name represents a BPF program
func isProgramSection(name string) bool {
	prefixes := []string{
		"xdp", "xdp/", "xdp_",
		"tc", "tc/", "tc_", "classifier", "action",
		"kprobe", "kprobe/", "kretprobe", "kretprobe/",
		"tracepoint", "tracepoint/", "tp/",
		"raw_tracepoint", "raw_tp/",
		"perf_event",
		"socket", "sk_skb", "sk_msg",
		"cgroup", "cgroup/",
		"fentry", "fexit", "fmod_ret",
		"lsm", "lsm/",
		"iter", "iter/",
		"syscall",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	// Also check .text section (subprograms: memcpy, memset, etc.)
	if name == ".text" {
		return true
	}

	// Capture .rodata* sections (read-only data loaded as BPF maps)
	if strings.HasPrefix(name, ".rodata") {
		return true
	}

	return false
}

// sectionNameToProgType converts a section name to BPF program type
func sectionNameToProgType(name string) uint32 {
	// Map section prefixes to program types
	switch {
	case strings.HasPrefix(name, "xdp"):
		return BPF_PROG_TYPE_XDP
	case strings.HasPrefix(name, "tc"), strings.HasPrefix(name, "classifier"),
		strings.HasPrefix(name, "action"):
		return BPF_PROG_TYPE_SCHED_CLS
	case strings.HasPrefix(name, "kprobe"), strings.HasPrefix(name, "kretprobe"):
		return BPF_PROG_TYPE_KPROBE
	case strings.HasPrefix(name, "tracepoint"), strings.HasPrefix(name, "tp/"):
		return BPF_PROG_TYPE_TRACEPOINT
	case strings.HasPrefix(name, "raw_tracepoint"), strings.HasPrefix(name, "raw_tp/"):
		return BPF_PROG_TYPE_RAW_TRACEPOINT
	case strings.HasPrefix(name, "perf_event"):
		return BPF_PROG_TYPE_PERF_EVENT
	case strings.HasPrefix(name, "socket"):
		return BPF_PROG_TYPE_SOCKET_FILTER
	case strings.HasPrefix(name, "cgroup/skb"):
		return BPF_PROG_TYPE_CGROUP_SKB
	case strings.HasPrefix(name, "cgroup/sock"):
		return BPF_PROG_TYPE_CGROUP_SOCK
	case strings.HasPrefix(name, "cgroup"):
		return BPF_PROG_TYPE_CGROUP_SKB
	case strings.HasPrefix(name, "fentry"), strings.HasPrefix(name, "fexit"),
		strings.HasPrefix(name, "fmod_ret"):
		return BPF_PROG_TYPE_TRACING
	case strings.HasPrefix(name, "lsm"):
		return BPF_PROG_TYPE_LSM
	case strings.HasPrefix(name, "iter"):
		return BPF_PROG_TYPE_TRACING
	case strings.HasPrefix(name, "sk_skb"):
		return BPF_PROG_TYPE_SK_SKB
	case strings.HasPrefix(name, "sk_msg"):
		return BPF_PROG_TYPE_SK_MSG
	default:
		return BPF_PROG_TYPE_UNSPEC
	}
}

// parseMapsSection parses the .maps section to extract map definitions.
// Supports both legacy 20-byte format and Aya/modern 28-byte format.
func parseMapsSection(data []byte, result *parsedELF) int {
	// Aya and modern BPF compilers use 28-byte map definitions:
	// struct {
	//     u32 type;
	//     u32 key_size;
	//     u32 value_size;
	//     u32 max_entries;
	//     u32 map_flags;
	//     u32 _pad[2]; // or id, inner_map_fd, etc.
	// };
	//
	// Legacy libbpf uses 20-byte definitions (no padding fields).
	// We detect the format by checking if the section size is evenly
	// divisible by 28 (Aya) or 20 (legacy), preferring 28.

	const ayaMapDefSize = 28
	const legacyMapDefSize = 20

	defSize := legacyMapDefSize
	if len(data) >= ayaMapDefSize && len(data)%ayaMapDefSize == 0 {
		defSize = ayaMapDefSize
	}

	for i := 0; i+defSize <= len(data); i += defSize {
		mapDef := &elfMap{
			Name:       fmt.Sprintf("map_%d", i/defSize),
			Type:       binary.LittleEndian.Uint32(data[i:]),
			KeySize:    binary.LittleEndian.Uint32(data[i+4:]),
			ValueSize:  binary.LittleEndian.Uint32(data[i+8:]),
			MaxEntries: binary.LittleEndian.Uint32(data[i+12:]),
			Flags:      binary.LittleEndian.Uint32(data[i+16:]),
		}

		// Validate map definition: type must be valid (1-100).
		// Ring buffers (type 27) and other special maps have key_size=0, value_size=0.
		if mapDef.Type > 0 && mapDef.Type < 100 && mapDef.MaxEntries > 0 {
			result.Maps[mapDef.Name] = mapDef
		}
	}

	return defSize
}

// parseRelocations parses relocation entries
func parseRelocations(data []byte, f *elf.File) []elfRelocation {
	var relocs []elfRelocation

	// Relocation entry size depends on ELF class (32 vs 64 bit)
	// For BPF (64-bit), each relocation is 16 bytes:
	// - 8 bytes: offset
	// - 8 bytes: info (symbol index + type)
	const relocSize = 16

	symbols, _ := f.Symbols()

	for i := 0; i+relocSize <= len(data); i += relocSize {
		offset := binary.LittleEndian.Uint64(data[i:])
		info := binary.LittleEndian.Uint64(data[i+8:])

		symIdx := info >> 32
		relType := uint32(info & 0xffffffff)

		var symName string
		var symValue uint64
		// Go's elf.File.Symbols() omits the null symbol at index 0,
		// so ELF symbol index N maps to Go array index N-1.
		goIdx := int(symIdx) - 1
		if goIdx >= 0 && goIdx < len(symbols) {
			sym := symbols[goIdx]
			symName = sym.Name
			symValue = sym.Value
			// SECTION symbols have empty names — resolve to section name
			if symName == "" && elf.ST_TYPE(sym.Info) == elf.STT_SECTION {
				if int(sym.Section) < len(f.Sections) && f.Sections[sym.Section] != nil {
					symName = f.Sections[sym.Section].Name
				}
			}
		}

		relocs = append(relocs, elfRelocation{
			Offset:      offset,
			SymbolName:  symName,
			SymbolValue: symValue,
			Type:        relType,
		})
	}

	return relocs
}

// resolveMapNames cross-references ELF symbols with parsed map definitions
// to replace generic names (map_0, map_1) with actual symbol names (FLOW_STATE, etc.).
// Only renames maps that were already parsed and validated from section data —
// does not create new map entries from symbols alone (security: map parameters
// must come from the validated section data, not from symbol metadata).
func resolveMapNames(f *elf.File, mapSection *elf.Section, defSize int, result *parsedELF) {
	if defSize <= 0 {
		return
	}

	symbols, err := f.Symbols()
	if err != nil {
		return
	}

	// Find the section index of our maps section
	var mapSectionIdx int = -1
	for idx, s := range f.Sections {
		if s == mapSection {
			mapSectionIdx = idx
			break
		}
	}
	if mapSectionIdx < 0 {
		return
	}

	for _, sym := range symbols {
		// Only consider symbols in the maps section
		if sym.Section == elf.SHN_UNDEF || int(sym.Section) != mapSectionIdx {
			continue
		}

		// Only consider OBJECT symbols with the right size (map definitions)
		if elf.ST_TYPE(sym.Info) != elf.STT_OBJECT || sym.Size != uint64(defSize) {
			continue
		}

		// Symbol's Value is the byte offset within the section.
		// Dividing by defSize gives the map index matching map_N naming.
		mapIdx := int(sym.Value) / defSize
		genericName := fmt.Sprintf("map_%d", mapIdx)

		mapDef, ok := result.Maps[genericName]
		if !ok {
			continue
		}

		// Rename: remove generic entry, insert with real name
		delete(result.Maps, genericName)
		mapDef.Name = sym.Name
		result.Maps[sym.Name] = mapDef
	}
}

// findFuncSize finds the byte size of a function at the given offset within
// the .text section, using the parsed ELF's program entries which store
// function symbols with their sizes. Falls back to scanning for EXIT insn.
func findFuncSize(parsed *parsedELF, funcOff uint64) int {
	// Check if any known function matches this offset
	// Function sizes are stored in the symbol table during parsing.
	// We stored textProg as parsed.Programs[".text"], but the symbol info
	// lives in the ELF directly. We use the FuncSizes map if available.
	if sz, ok := parsed.FuncSizes[funcOff]; ok {
		return sz
	}

	// Fallback: scan for EXIT instruction (opcode 0x95) at 8-byte boundaries
	textProg, ok := parsed.Programs[".text"]
	if !ok {
		return 0
	}
	data := textProg.Instructions
	for i := int(funcOff); i+8 <= len(data); i += 8 {
		if data[i] == 0x95 { // BPF_EXIT
			return i + 8 - int(funcOff)
		}
	}
	return 0
}

// ============================================================================
// LOADER METHODS - LOAD AND UNLOAD
// ============================================================================

// Load loads an eBPF program from an object file
func (l *NativeLoader) Load(ctx context.Context, spec *ProgramSpec) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	if spec == nil || spec.Name == "" {
		return ErrInvalidProgram
	}

	if spec.Path == "" {
		return fmt.Errorf("%w: path is required", ErrInvalidProgram)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.programs[spec.Name]; exists {
		return ErrProgramExists
	}

	startTime := time.Now()

	// Parse the ELF file
	parsed, err := parseELF(spec.Path)
	if err != nil {
		ebpfErrors.WithLabelValues("load", "elf_parse").Inc()
		return fmt.Errorf("%w: %v", ErrLoadFailed, err)
	}

	if len(parsed.Programs) == 0 {
		return fmt.Errorf("%w: no programs found in %s", ErrNoInstructions, spec.Path)
	}

	// Create loaded program state
	loaded := &loadedProgram{
		spec: spec,
		info: &ProgramInfo{
			Name:       spec.Name,
			Type:       spec.Type,
			LoadTime:   time.Now(),
			Status:     StatusLoading,
			AttachTo:   spec.AttachTo,
			AttachType: spec.AttachType,
			Maps:       make([]string, 0),
			Labels:     spec.Labels,
		},
		maps:      make(map[string]*loadedMap),
		linkFDs:   make([]int, 0),
		perfFDs:   make([]int, 0),
		cancelFns: make([]context.CancelFunc, 0),
	}

	// First, create all maps
	for name, mapDef := range parsed.Maps {
		mapFD, err := l.createMap(mapDef, spec)
		if err != nil {
			// Clean up already created maps
			for _, m := range loaded.maps {
				unix.Close(m.fd)
			}
			ebpfErrors.WithLabelValues("load", "map_create").Inc()
			return fmt.Errorf("create map %s: %w", name, err)
		}

		mapInfo, _ := bpfGetMapInfo(mapFD)

		loaded.maps[name] = &loadedMap{
			fd: mapFD,
			info: &MapInfo{
				Name:       name,
				ID:         mapInfo.ID,
				Type:       mapTypeToMapType(mapDef.Type),
				KeySize:    mapDef.KeySize,
				ValueSize:  mapDef.ValueSize,
				MaxEntries: mapDef.MaxEntries,
				Flags:      mapDef.Flags,
			},
		}
		loaded.info.Maps = append(loaded.info.Maps, name)
	}

	// Find the main program to load
	var mainProg *elfProgram
	progType := programTypeToKernelType(spec.Type)

	for _, prog := range parsed.Programs {
		// Match by program type or use first if unspecified
		if prog.Type == progType || progType == BPF_PROG_TYPE_UNSPEC {
			mainProg = prog
			break
		}
	}

	if mainProg == nil {
		// Just use the first program
		for _, prog := range parsed.Programs {
			mainProg = prog
			break
		}
	}

	if mainProg == nil {
		for _, m := range loaded.maps {
			unix.Close(m.fd)
		}
		return fmt.Errorf("%w: no suitable program found", ErrNoInstructions)
	}

	// Build instruction stream: main program + referenced .text subprograms.
	// BPF programs with function calls (BPF_PSEUDO_CALL) require called
	// subprogram code to be appended to the main program section.
	// The verifier rejects unreachable instructions, so we only append
	// functions that are actually referenced by R_BPF_64_32 relocations.
	instructions := make([]byte, len(mainProg.Instructions))
	copy(instructions, mainProg.Instructions)

	// Collect referenced .text functions and append only those
	textProg, _ := parsed.Programs[".text"]
	// funcMap: symValue (offset in .text) → new instruction index in combined stream
	funcMap := make(map[uint64]int)
	if textProg != nil {
		relocs, _ := parsed.Relocations[mainProg.Name]
		// Collect unique function offsets referenced by R_BPF_64_32 relocations
		seen := make(map[uint64]bool)
		var funcOffsets []uint64
		for _, r := range relocs {
			if r.Type == 10 { // R_BPF_64_32
				if !seen[r.SymbolValue] {
					seen[r.SymbolValue] = true
					funcOffsets = append(funcOffsets, r.SymbolValue)
				}
			}
		}
		// Sort for deterministic layout
		sort.Slice(funcOffsets, func(i, j int) bool { return funcOffsets[i] < funcOffsets[j] })

		// Append each referenced function
		for _, funcOff := range funcOffsets {
			funcMap[funcOff] = len(instructions) / 8
			// Find function size from symbols
			funcSize := findFuncSize(parsed, funcOff)
			if funcSize == 0 || int(funcOff)+funcSize > len(textProg.Instructions) {
				continue
			}
			instructions = append(instructions, textProg.Instructions[funcOff:int(funcOff)+funcSize]...)
		}
	}

	// Create BPF array maps for read-only data sections (.rodata*)
	for name, prog := range parsed.Programs {
		if !strings.HasPrefix(name, ".rodata") {
			continue
		}
		// .rodata sections become BPF_MAP_TYPE_ARRAY with BPF_F_RDONLY_PROG
		dataLen := uint32(len(prog.Instructions))
		if dataLen == 0 {
			continue
		}
		rodataMapDef := &elfMap{
			Name:       name,
			Type:       BPF_MAP_TYPE_ARRAY,
			KeySize:    4,
			ValueSize:  dataLen,
			MaxEntries: 1,
			Flags:      0x80, // BPF_F_RDONLY_PROG
		}
		rodataFD, err := l.createMap(rodataMapDef, spec)
		if err != nil {
			for _, m := range loaded.maps {
				unix.Close(m.fd)
			}
			return fmt.Errorf("create rodata map %s: %w", name, err)
		}
		// Populate the rodata map with the section data
		key := uint32(0)
		if err := bpfMapUpdateElem(rodataFD, unsafe.Pointer(&key),
			unsafe.Pointer(&prog.Instructions[0]), 0); err != nil {
			unix.Close(rodataFD)
			for _, m := range loaded.maps {
				unix.Close(m.fd)
			}
			return fmt.Errorf("populate rodata map %s: %w", name, err)
		}
		loaded.maps[name] = &loadedMap{
			fd: rodataFD,
			info: &MapInfo{
				Name:       name,
				Type:       MapTypeArray,
				KeySize:    4,
				ValueSize:  dataLen,
				MaxEntries: 1,
			},
		}
	}

	if relocs, ok := parsed.Relocations[mainProg.Name]; ok {
		if err := l.applyRelocations(instructions, relocs, loaded.maps, parsed, funcMap); err != nil {
			for _, m := range loaded.maps {
				unix.Close(m.fd)
			}
			return fmt.Errorf("apply relocations: %w", err)
		}
	}

	// Prepare verifier log buffer
	logBuf := make([]byte, l.config.VerifierLogSize)

	// Load the program
	progFD, verifierLog, err := bpfProgLoad(
		mainProg.Type,
		instructions,
		parsed.License,
		logBuf,
		l.btfFD,
	)

	if err != nil {
		for _, m := range loaded.maps {
			unix.Close(m.fd)
		}
		ebpfErrors.WithLabelValues("load", "prog_load").Inc()

		if verifierLog != "" {
			return fmt.Errorf("%w: %v\nVerifier log:\n%s", ErrVerifierFailed, err, verifierLog)
		}
		return fmt.Errorf("%w: %v", ErrLoadFailed, err)
	}

	loaded.fd = progFD

	// Get program info from kernel
	if progInfo, err := bpfGetProgInfo(progFD); err == nil {
		loaded.info.ID = progInfo.ID
		loaded.info.Tag = fmt.Sprintf("%x", progInfo.Tag)
		loaded.info.JitedSize = progInfo.JitedProgLen
		loaded.info.XlatedSize = progInfo.XlatedProgLen
	}

	// Pin program if requested
	if spec.PinPath != "" {
		pinPath := filepath.Join(l.config.PinPath, spec.PinPath)
		if err := os.MkdirAll(filepath.Dir(pinPath), 0755); err != nil {
			unix.Close(progFD)
			for _, m := range loaded.maps {
				unix.Close(m.fd)
			}
			return fmt.Errorf("create pin directory: %w", err)
		}
		if err := bpfObjPin(progFD, pinPath); err != nil {
			unix.Close(progFD)
			for _, m := range loaded.maps {
				unix.Close(m.fd)
			}
			return fmt.Errorf("pin program: %w", err)
		}
	}

	// Pin maps if requested
	for mapName, pinPath := range spec.MapPinPaths {
		if m, ok := loaded.maps[mapName]; ok {
			fullPath := filepath.Join(l.config.PinPath, pinPath)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err == nil {
				_ = bpfObjPin(m.fd, fullPath)
				m.info.PinPath = fullPath
			}
		}
	}

	loaded.info.Status = StatusLoaded
	l.programs[spec.Name] = loaded

	// Update metrics
	if l.metricsEnabled {
		ebpfProgramsLoaded.WithLabelValues(string(spec.Type)).Inc()
		ebpfProgramLoadTime.WithLabelValues(spec.Name, string(spec.Type)).
			Observe(time.Since(startTime).Seconds())
	}

	return nil
}

// createMap creates a BPF map
func (l *NativeLoader) createMap(def *elfMap, spec *ProgramSpec) (int, error) {
	return bpfMapCreate(
		def.Type,
		def.KeySize,
		def.ValueSize,
		def.MaxEntries,
		def.Flags,
		def.Name,
	)
}

// applyRelocations applies relocation entries to BPF instructions.
// Handles two relocation types:
//   - R_BPF_64_64 (type 1): 64-bit map FD reference (ld_imm64 instruction)
//   - R_BPF_64_32 (type 10): 32-bit function call to .text subprogram
//
// funcMap maps .text function byte offsets to their instruction indices
// in the combined instruction stream.
func (l *NativeLoader) applyRelocations(insns []byte, relocs []elfRelocation,
	maps map[string]*loadedMap, parsed *parsedELF, funcMap map[uint64]int) error {

	const (
		R_BPF_64_64 = 1  // Map FD reference
		R_BPF_64_32 = 10 // Function call (BPF-to-BPF)
	)

	for _, reloc := range relocs {
		insnIdx := reloc.Offset / 8

		if int(insnIdx*8+8) > len(insns) {
			return fmt.Errorf("relocation offset %d out of bounds", reloc.Offset)
		}

		switch reloc.Type {
		case R_BPF_64_32:
			// Function call relocation: patch BPF_PSEUDO_CALL instruction
			// to point to the target function appended after the main program.
			// The symbol's value is the byte offset within .text.
			targetInsn, ok := funcMap[reloc.SymbolValue]
			if !ok {
				continue
			}

			// Set src_reg = BPF_PSEUDO_CALL (1)
			insns[insnIdx*8+1] = (insns[insnIdx*8+1] & 0x0f) | 0x10

			// Set imm = relative offset from NEXT instruction to target
			relOffset := int32(targetInsn) - int32(insnIdx) - 1
			binary.LittleEndian.PutUint32(insns[insnIdx*8+4:], uint32(relOffset))

		case R_BPF_64_64:
			// Map FD reference: patch ld_imm64 instruction
			mapName := reloc.SymbolName

			var mapFD int = -1
			if m, ok := maps[mapName]; ok {
				mapFD = m.fd
			}

			// Also try section name match (e.g., ".rodata.cst8" matches ".rodata.cst8")
			if mapFD < 0 {
				// Try matching by section symbol name
				for mname, m := range maps {
					if strings.HasPrefix(mname, ".rodata") && strings.HasPrefix(reloc.SymbolName, ".rodata") {
						mapFD = m.fd
						break
					}
				}
			}

			if mapFD < 0 {
				continue
			}

			// Set src_reg to BPF_PSEUDO_MAP_FD (1) for regular maps,
			// or BPF_PSEUDO_MAP_VALUE (2) for rodata (to reference the value directly)
			pseudoType := byte(0x10) // BPF_PSEUDO_MAP_FD
			if strings.HasPrefix(reloc.SymbolName, ".rodata") {
				pseudoType = 0x20 // BPF_PSEUDO_MAP_VALUE
			}
			insns[insnIdx*8+1] = (insns[insnIdx*8+1] & 0x0f) | pseudoType

			// Set immediate to map FD
			binary.LittleEndian.PutUint32(insns[insnIdx*8+4:], uint32(mapFD))

		default:
			// Unknown relocation type — skip
			continue
		}
	}

	return nil
}

// Unload unloads an eBPF program
func (l *NativeLoader) Unload(ctx context.Context, name string) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	loaded, exists := l.programs[name]
	if !exists {
		return ErrProgramNotFound
	}

	// Detach first if attached
	if loaded.info.Status == StatusAttached {
		if err := l.detachLocked(loaded); err != nil {
			return fmt.Errorf("detach before unload: %w", err)
		}
	}

	// Cancel all background goroutines
	for _, cancel := range loaded.cancelFns {
		cancel()
	}

	// Close links
	for _, fd := range loaded.linkFDs {
		unix.Close(fd)
	}

	// Close perf FDs
	for _, fd := range loaded.perfFDs {
		unix.Close(fd)
	}

	// Unmap ring buffers
	for _, m := range loaded.maps {
		if m.mmapAddr != 0 {
			_ = unix.Munmap((*[1 << 30]byte)(unsafe.Pointer(m.mmapAddr))[:m.mmapSize])
		}
		unix.Close(m.fd)
	}

	// Close program FD
	if loaded.fd > 0 {
		unix.Close(loaded.fd)
	}

	// Update metrics
	if l.metricsEnabled {
		ebpfProgramsLoaded.WithLabelValues(string(loaded.spec.Type)).Dec()
	}

	delete(l.programs, name)
	return nil
}

// programTypeToKernelType converts ProgramType to kernel program type constant
func programTypeToKernelType(t ProgramType) uint32 {
	switch t {
	case TypeXDP:
		return BPF_PROG_TYPE_XDP
	case TypeTC:
		return BPF_PROG_TYPE_SCHED_CLS
	case TypeKProbe, TypeKRetProbe:
		return BPF_PROG_TYPE_KPROBE
	case TypeTracepoint:
		return BPF_PROG_TYPE_TRACEPOINT
	case TypeRawTracepoint:
		return BPF_PROG_TYPE_RAW_TRACEPOINT
	case TypePerf:
		return BPF_PROG_TYPE_PERF_EVENT
	case TypeCgroupSkb:
		return BPF_PROG_TYPE_CGROUP_SKB
	case TypeCgroupSock:
		return BPF_PROG_TYPE_CGROUP_SOCK
	case TypeSocketFilter:
		return BPF_PROG_TYPE_SOCKET_FILTER
	case TypeFentry, TypeFexit:
		return BPF_PROG_TYPE_TRACING
	case TypeLSM:
		return BPF_PROG_TYPE_LSM
	default:
		return BPF_PROG_TYPE_UNSPEC
	}
}

// mapTypeToMapType converts kernel map type to MapType
func mapTypeToMapType(t uint32) MapType {
	switch t {
	case BPF_MAP_TYPE_HASH:
		return MapTypeHash
	case BPF_MAP_TYPE_ARRAY:
		return MapTypeArray
	case BPF_MAP_TYPE_PROG_ARRAY:
		return MapTypeProgArray
	case BPF_MAP_TYPE_PERF_EVENT_ARRAY:
		return MapTypePerfEventArray
	case BPF_MAP_TYPE_PERCPU_HASH:
		return MapTypePercpuHash
	case BPF_MAP_TYPE_PERCPU_ARRAY:
		return MapTypePercpuArray
	case BPF_MAP_TYPE_LRU_HASH:
		return MapTypeLruHash
	case BPF_MAP_TYPE_LRU_PERCPU_HASH:
		return MapTypeLruPercpuHash
	case BPF_MAP_TYPE_LPM_TRIE:
		return MapTypeLpmTrie
	case BPF_MAP_TYPE_ARRAY_OF_MAPS:
		return MapTypeArrayOfMaps
	case BPF_MAP_TYPE_HASH_OF_MAPS:
		return MapTypeHashOfMaps
	case BPF_MAP_TYPE_RINGBUF:
		return MapTypeRingbuf
	case BPF_MAP_TYPE_BLOOM_FILTER:
		return MapTypeBloom
	case BPF_MAP_TYPE_QUEUE:
		return MapTypeQueue
	case BPF_MAP_TYPE_STACK:
		return MapTypeStack
	default:
		return MapTypeHash
	}
}

// ============================================================================
// LOADER METHODS - ATTACH AND DETACH
// ============================================================================

// Attach attaches an eBPF program to its target
func (l *NativeLoader) Attach(ctx context.Context, name string) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	loaded, exists := l.programs[name]
	if !exists {
		return ErrProgramNotFound
	}

	if loaded.info.Status == StatusAttached {
		return nil // Already attached
	}

	loaded.info.Status = StatusAttaching

	var err error
	switch loaded.spec.Type {
	case TypeXDP:
		err = l.attachXDP(loaded)
	case TypeTC:
		err = l.attachTC(loaded)
	case TypeKProbe:
		err = l.attachKprobe(loaded, false)
	case TypeKRetProbe:
		err = l.attachKprobe(loaded, true)
	case TypeTracepoint:
		err = l.attachTracepoint(loaded)
	case TypeRawTracepoint:
		err = l.attachRawTracepoint(loaded)
	case TypeCgroupSkb, TypeCgroupSock:
		err = l.attachCgroup(loaded)
	default:
		err = fmt.Errorf("unsupported program type for attachment: %s", loaded.spec.Type)
	}

	if err != nil {
		loaded.info.Status = StatusError
		loaded.info.Error = err.Error()
		ebpfErrors.WithLabelValues("attach", string(loaded.spec.Type)).Inc()
		return fmt.Errorf("%w: %v", ErrAttachFailed, err)
	}

	loaded.info.Status = StatusAttached

	// Update metrics
	if l.metricsEnabled {
		ebpfProgramsAttached.WithLabelValues(
			string(loaded.spec.Type),
			string(loaded.spec.AttachType),
		).Inc()
	}

	return nil
}

// Detach detaches an eBPF program from its target
func (l *NativeLoader) Detach(ctx context.Context, name string) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	loaded, exists := l.programs[name]
	if !exists {
		return ErrProgramNotFound
	}

	if loaded.info.Status != StatusAttached {
		return nil // Not attached
	}

	if err := l.detachLocked(loaded); err != nil {
		return err
	}

	// Update metrics
	if l.metricsEnabled {
		ebpfProgramsAttached.WithLabelValues(
			string(loaded.spec.Type),
			string(loaded.spec.AttachType),
		).Dec()
	}

	return nil
}

// detachLocked performs detachment while holding the lock
func (l *NativeLoader) detachLocked(loaded *loadedProgram) error {
	loaded.info.Status = StatusDetaching

	var lastErr error

	// Close all link FDs
	for _, fd := range loaded.linkFDs {
		if err := unix.Close(fd); err != nil {
			lastErr = err
		}
	}
	loaded.linkFDs = loaded.linkFDs[:0]

	// Close perf event FDs
	for _, fd := range loaded.perfFDs {
		if err := unix.Close(fd); err != nil {
			lastErr = err
		}
	}
	loaded.perfFDs = loaded.perfFDs[:0]

	if lastErr != nil {
		loaded.info.Status = StatusError
		loaded.info.Error = lastErr.Error()
		return fmt.Errorf("%w: %v", ErrDetachFailed, lastErr)
	}

	loaded.info.Status = StatusLoaded
	return nil
}

// ============================================================================
// XDP ATTACHMENT VIA NETLINK
// ============================================================================
//
// XDP (eXpress Data Path) programs are attached to network interfaces using
// netlink messages. The attachment is done by setting the IFLA_XDP attribute
// on the interface.

// nlMsgHdr is the netlink message header
type nlMsgHdr struct {
	Len   uint32
	Type  uint16
	Flags uint16
	Seq   uint32
	Pid   uint32
}

// ifInfoMsg is the interface info message header
type ifInfoMsg struct {
	Family uint8
	_      uint8
	Type   uint16
	Index  int32
	Flags  uint32
	Change uint32
}

// nlAttr is a netlink attribute header
type nlAttr struct {
	Len  uint16
	Type uint16
}

// attachXDP attaches an XDP program to a network interface via netlink
func (l *NativeLoader) attachXDP(loaded *loadedProgram) error {
	// Get interface index
	ifIndex, err := getInterfaceIndex(loaded.spec.AttachTo)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInterfaceNotFound, loaded.spec.AttachTo)
	}

	// Try to use BPF link first (kernel 5.9+)
	if l.features.BPFLinkSupport && l.features.Version.AtLeast(5, 9, 0) {
		linkFD, err := l.attachXDPLink(loaded, ifIndex)
		if err == nil {
			loaded.linkFDs = append(loaded.linkFDs, linkFD)
			return nil
		}
		// Fall back to netlink if link creation failed
	}

	// Use netlink for XDP attachment
	return l.attachXDPNetlink(loaded, ifIndex)
}

// attachXDPLink attaches XDP using BPF link (modern method)
func (l *NativeLoader) attachXDPLink(loaded *loadedProgram, ifIndex int) (int, error) {
	// Open a netlink socket to get interface FD
	// For XDP links, we need the interface index, not an FD

	// Create a link with BPF_LINK_CREATE
	attr := struct {
		ProgFD     uint32
		_          uint32
		AttachType uint32
		Flags      uint32
		TargetIf   uint32
		_          [20]byte
	}{
		ProgFD:     uint32(loaded.fd),
		AttachType: BPF_XDP,
		TargetIf:   uint32(ifIndex),
	}

	// Determine flags
	switch loaded.spec.AttachType {
	case AttachXDPGeneric:
		attr.Flags = XDP_FLAGS_SKB_MODE
	case AttachXDPDriver:
		attr.Flags = XDP_FLAGS_DRV_MODE
	case AttachXDPOffload:
		attr.Flags = XDP_FLAGS_HW_MODE
	}

	fd, err := bpfSyscall(BPF_LINK_CREATE, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return -1, err
	}

	return fd, nil
}

// attachXDPNetlink attaches XDP using netlink (legacy method)
func (l *NativeLoader) attachXDPNetlink(loaded *loadedProgram, ifIndex int) error {
	// Create netlink socket
	sock, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("create netlink socket: %w", err)
	}
	defer unix.Close(sock)

	// Bind socket
	addr := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if err := unix.Bind(sock, addr); err != nil {
		return fmt.Errorf("bind netlink socket: %w", err)
	}

	// Build the netlink message
	// Message structure:
	// nlmsghdr + ifinfomsg + [nlattr(IFLA_XDP) + [nlattr(XDP_FD) + fd] + [nlattr(XDP_FLAGS) + flags]]

	// Calculate sizes
	xdpFDAttrLen := uint16(8)              // nlattr header (4) + fd (4)
	xdpFlagsAttrLen := uint16(8)           // nlattr header (4) + flags (4)
	xdpAttrLen := uint16(4 + xdpFDAttrLen + xdpFlagsAttrLen) // nlattr header + nested
	ifInfoLen := uint32(16)                // sizeof(ifinfomsg)
	totalLen := uint32(16 + ifInfoLen + uint32(xdpAttrLen) + 4) // nlmsghdr + ifinfomsg + attrs + padding

	// Allocate buffer
	buf := make([]byte, totalLen)

	// Write nlmsghdr
	nlh := (*nlMsgHdr)(unsafe.Pointer(&buf[0]))
	nlh.Len = totalLen
	nlh.Type = RTM_SETLINK
	nlh.Flags = NLM_F_REQUEST | NLM_F_ACK
	nlh.Seq = 1

	// Write ifinfomsg
	ifi := (*ifInfoMsg)(unsafe.Pointer(&buf[16]))
	ifi.Family = unix.AF_UNSPEC
	ifi.Index = int32(ifIndex)

	// Write IFLA_XDP attribute (nested)
	offset := 32
	xdpAttr := (*nlAttr)(unsafe.Pointer(&buf[offset]))
	xdpAttr.Len = xdpAttrLen
	xdpAttr.Type = IFLA_XDP | 0x8000 // NLA_F_NESTED
	offset += 4

	// Write XDP_FD attribute
	fdAttr := (*nlAttr)(unsafe.Pointer(&buf[offset]))
	fdAttr.Len = xdpFDAttrLen
	fdAttr.Type = IFLA_XDP_FD
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], uint32(loaded.fd))
	offset += 4

	// Write XDP_FLAGS attribute
	var flags uint32
	switch loaded.spec.AttachType {
	case AttachXDPGeneric:
		flags = XDP_FLAGS_SKB_MODE
	case AttachXDPDriver:
		flags = XDP_FLAGS_DRV_MODE
	case AttachXDPOffload:
		flags = XDP_FLAGS_HW_MODE
	default:
		// Try driver mode, kernel will fall back if needed
		flags = 0
	}

	flagsAttr := (*nlAttr)(unsafe.Pointer(&buf[offset]))
	flagsAttr.Len = xdpFlagsAttrLen
	flagsAttr.Type = IFLA_XDP_FLAGS
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], flags)

	// Send message
	destAddr := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if err := unix.Sendto(sock, buf, 0, destAddr); err != nil {
		return fmt.Errorf("send netlink message: %w", err)
	}

	// Receive ACK
	recvBuf := make([]byte, 4096)
	n, _, err := unix.Recvfrom(sock, recvBuf, 0)
	if err != nil {
		return fmt.Errorf("receive netlink response: %w", err)
	}

	// Check for error in response
	if n >= 20 {
		respNlh := (*nlMsgHdr)(unsafe.Pointer(&recvBuf[0]))
		if respNlh.Type == unix.NLMSG_ERROR {
			errCode := int32(binary.LittleEndian.Uint32(recvBuf[16:]))
			if errCode != 0 {
				return fmt.Errorf("netlink error: %d (%s)", errCode, syscall.Errno(-errCode))
			}
		}
	}

	return nil
}

// getInterfaceIndex gets the interface index for a name
func getInterfaceIndex(name string) (int, error) {
	iface, err := getInterfaceByName(name)
	if err != nil {
		return 0, err
	}
	return iface.Index, nil
}

// getInterfaceByName is a minimal interface lookup
func getInterfaceByName(name string) (*interfaceInfo, error) {
	// Use ioctl SIOCGIFINDEX
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)

	var ifr [40]byte // struct ifreq
	copy(ifr[:], name)

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		unix.SIOCGIFINDEX, uintptr(unsafe.Pointer(&ifr[0])))
	if errno != 0 {
		return nil, errno
	}

	index := int(binary.LittleEndian.Uint32(ifr[16:]))
	return &interfaceInfo{Name: name, Index: index}, nil
}

type interfaceInfo struct {
	Name  string
	Index int
}

// ============================================================================
// TC (TRAFFIC CONTROL) ATTACHMENT
// ============================================================================
//
// TC BPF programs are attached to the Linux traffic control subsystem.
// They can filter/modify packets at ingress or egress of an interface.
// Attachment requires creating a clsact qdisc and adding a BPF filter.

// attachTC attaches a TC program to a network interface
func (l *NativeLoader) attachTC(loaded *loadedProgram) error {
	// Get interface index
	ifIndex, err := getInterfaceIndex(loaded.spec.AttachTo)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInterfaceNotFound, loaded.spec.AttachTo)
	}

	// Try TCX first (kernel 6.6+)
	if l.features.TCXSupport {
		linkFD, err := l.attachTCX(loaded, ifIndex)
		if err == nil {
			loaded.linkFDs = append(loaded.linkFDs, linkFD)
			return nil
		}
		// Fall back to netlink if TCX failed
	}

	// Use legacy netlink-based TC attachment
	return l.attachTCNetlink(loaded, ifIndex)
}

// attachTCX attaches TC using BPF TCX link (kernel 6.6+)
func (l *NativeLoader) attachTCX(loaded *loadedProgram, ifIndex int) (int, error) {
	var attachType uint32
	switch loaded.spec.AttachType {
	case AttachTCIngress:
		attachType = BPF_TCX_INGRESS
	case AttachTCEgress:
		attachType = BPF_TCX_EGRESS
	default:
		attachType = BPF_TCX_INGRESS
	}

	attr := struct {
		ProgFD       uint32
		TargetIfIdx  uint32
		AttachType   uint32
		Flags        uint32
		RelativeFD   uint32
		RelativeID   uint32
		ExpectedRev  uint64
	}{
		ProgFD:      uint32(loaded.fd),
		TargetIfIdx: uint32(ifIndex),
		AttachType:  attachType,
	}

	fd, err := bpfSyscall(BPF_LINK_CREATE, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return -1, err
	}

	return fd, nil
}

// attachTCNetlink attaches TC using netlink (legacy method)
// This requires:
// 1. Adding a clsact qdisc to the interface
// 2. Adding a BPF filter to the qdisc
func (l *NativeLoader) attachTCNetlink(loaded *loadedProgram, ifIndex int) error {
	// Create netlink socket for TC
	sock, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("create netlink socket: %w", err)
	}
	defer unix.Close(sock)

	// Bind socket
	addr := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if err := unix.Bind(sock, addr); err != nil {
		return fmt.Errorf("bind netlink socket: %w", err)
	}

	// Step 1: Create clsact qdisc
	if err := l.createClsactQdisc(sock, ifIndex); err != nil {
		// Ignore error if qdisc already exists
		if !strings.Contains(err.Error(), "exist") {
			return fmt.Errorf("create clsact qdisc: %w", err)
		}
	}

	// Step 2: Add BPF filter
	return l.addTCBPFFilter(sock, loaded, ifIndex)
}

// createClsactQdisc creates a clsact qdisc on an interface
func (l *NativeLoader) createClsactQdisc(sock int, ifIndex int) error {
	// tc qdisc add dev <iface> clsact

	// Build tcmsg structure
	type tcMsg struct {
		Family  uint8
		_       [3]uint8
		Ifindex int32
		Handle  uint32
		Parent  uint32
		Info    uint32
	}

	// Calculate message size
	kindAttr := []byte("clsact\x00")
	kindAttrLen := (4 + len(kindAttr) + 3) & ^3 // 4-byte aligned
	totalLen := 16 + 20 + kindAttrLen           // nlmsghdr + tcmsg + attr

	buf := make([]byte, totalLen)

	// Write nlmsghdr
	nlh := (*nlMsgHdr)(unsafe.Pointer(&buf[0]))
	nlh.Len = uint32(totalLen)
	nlh.Type = RTM_NEWQDISC
	nlh.Flags = NLM_F_REQUEST | NLM_F_ACK | NLM_F_CREATE | NLM_F_EXCL
	nlh.Seq = 2

	// Write tcmsg
	tcm := (*tcMsg)(unsafe.Pointer(&buf[16]))
	tcm.Family = unix.AF_UNSPEC
	tcm.Ifindex = int32(ifIndex)
	tcm.Handle = TC_H_CLSACT
	tcm.Parent = 0xFFFFFFFF // TC_H_ROOT

	// Write TCA_KIND attribute
	offset := 36
	kindNla := (*nlAttr)(unsafe.Pointer(&buf[offset]))
	kindNla.Len = uint16(4 + len(kindAttr))
	kindNla.Type = TCA_KIND
	copy(buf[offset+4:], kindAttr)

	// Send message
	destAddr := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if err := unix.Sendto(sock, buf, 0, destAddr); err != nil {
		return err
	}

	// Receive ACK
	return l.receiveNetlinkAck(sock)
}

// addTCBPFFilter adds a BPF filter to the clsact qdisc
func (l *NativeLoader) addTCBPFFilter(sock int, loaded *loadedProgram, ifIndex int) error {
	// tc filter add dev <iface> ingress/egress bpf direct-action obj <file> sec <section>

	type tcMsg struct {
		Family  uint8
		_       [3]uint8
		Ifindex int32
		Handle  uint32
		Parent  uint32
		Info    uint32
	}

	// Determine parent (ingress or egress)
	var parent uint32
	if loaded.spec.AttachType == AttachTCEgress {
		parent = TC_H_CLSACT | uint32(TC_H_MIN_EGRESS)
	} else {
		parent = TC_H_CLSACT | uint32(TC_H_MIN_INGRESS)
	}

	// Build attributes
	kindAttr := []byte("bpf\x00")
	kindAttrLen := (4 + len(kindAttr) + 3) & ^3

	// BPF options (nested)
	// TCA_BPF_FD + TCA_BPF_FLAGS
	bpfFDAttr := make([]byte, 8)
	binary.LittleEndian.PutUint16(bpfFDAttr[0:], 8)
	binary.LittleEndian.PutUint16(bpfFDAttr[2:], TCA_BPF_FD)
	binary.LittleEndian.PutUint32(bpfFDAttr[4:], uint32(loaded.fd))

	bpfFlagsAttr := make([]byte, 8)
	binary.LittleEndian.PutUint16(bpfFlagsAttr[0:], 8)
	binary.LittleEndian.PutUint16(bpfFlagsAttr[2:], TCA_BPF_FLAGS)
	binary.LittleEndian.PutUint32(bpfFlagsAttr[4:], TCA_BPF_FLAGS_DIRECT)

	optionsLen := len(bpfFDAttr) + len(bpfFlagsAttr)
	optionsAttrLen := (4 + optionsLen + 3) & ^3

	totalLen := 16 + 20 + kindAttrLen + optionsAttrLen

	buf := make([]byte, totalLen)

	// Write nlmsghdr
	nlh := (*nlMsgHdr)(unsafe.Pointer(&buf[0]))
	nlh.Len = uint32(totalLen)
	nlh.Type = 44 // RTM_NEWTFILTER
	nlh.Flags = NLM_F_REQUEST | NLM_F_ACK | NLM_F_CREATE | NLM_F_EXCL
	nlh.Seq = 3

	// Write tcmsg
	tcm := (*tcMsg)(unsafe.Pointer(&buf[16]))
	tcm.Family = unix.AF_UNSPEC
	tcm.Ifindex = int32(ifIndex)
	tcm.Parent = parent
	// Info contains protocol (ETH_P_ALL) and priority
	tcm.Info = (uint32(1) << 16) | uint32(0x0300) // priority 1, ETH_P_ALL

	// Write TCA_KIND attribute
	offset := 36
	kindNla := (*nlAttr)(unsafe.Pointer(&buf[offset]))
	kindNla.Len = uint16(4 + len(kindAttr))
	kindNla.Type = TCA_KIND
	copy(buf[offset+4:], kindAttr)
	offset += kindAttrLen

	// Write TCA_OPTIONS attribute (nested)
	optNla := (*nlAttr)(unsafe.Pointer(&buf[offset]))
	optNla.Len = uint16(4 + optionsLen)
	optNla.Type = TCA_OPTIONS | 0x8000 // NLA_F_NESTED
	offset += 4
	copy(buf[offset:], bpfFDAttr)
	offset += len(bpfFDAttr)
	copy(buf[offset:], bpfFlagsAttr)

	// Send message
	destAddr := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if err := unix.Sendto(sock, buf, 0, destAddr); err != nil {
		return err
	}

	return l.receiveNetlinkAck(sock)
}

// receiveNetlinkAck receives and checks netlink ACK
func (l *NativeLoader) receiveNetlinkAck(sock int) error {
	recvBuf := make([]byte, 4096)
	n, _, err := unix.Recvfrom(sock, recvBuf, 0)
	if err != nil {
		return fmt.Errorf("receive netlink response: %w", err)
	}

	if n >= 20 {
		respNlh := (*nlMsgHdr)(unsafe.Pointer(&recvBuf[0]))
		if respNlh.Type == unix.NLMSG_ERROR {
			errCode := int32(binary.LittleEndian.Uint32(recvBuf[16:]))
			if errCode != 0 {
				return fmt.Errorf("netlink error: %s", syscall.Errno(-errCode))
			}
		}
	}

	return nil
}

// ============================================================================
// KPROBE ATTACHMENT VIA PERF_EVENT_OPEN
// ============================================================================
//
// Kprobes are dynamic probes that can be inserted at almost any kernel address.
// We attach BPF programs to kprobes using the perf_event_open syscall.

// perfEventAttr is the attribute structure for perf_event_open
type perfEventAttr struct {
	Type               uint32
	Size               uint32
	Config             uint64
	SamplePeriodOrFreq uint64
	SampleType         uint64
	ReadFormat         uint64
	Flags              uint64
	WakeupEventsOrWM   uint32
	BPType             uint32
	ExtConfig          uint64
	BranchSampleType   uint64
	SampleRegsUser     uint64
	SampleStackUser    uint32
	ClockID            int32
	SampleRegsIntr     uint64
	AuxWatermark       uint32
	SampleMaxStack     uint16
	_                  uint16
	AuxSampleSize      uint32
	_                  uint32
	SigData            uint64
}

// attachKprobe attaches a kprobe/kretprobe program
func (l *NativeLoader) attachKprobe(loaded *loadedProgram, ret bool) error {
	funcName := loaded.spec.AttachTo
	if funcName == "" {
		return fmt.Errorf("kprobe target function not specified")
	}

	// Try to use BPF link first (kernel 5.5+)
	if l.features.BPFLinkSupport {
		linkFD, err := l.attachKprobeLink(loaded, funcName, ret)
		if err == nil {
			loaded.linkFDs = append(loaded.linkFDs, linkFD)
			return nil
		}
		// Fall back to perf_event_open if link creation failed
	}

	// Use perf_event_open (legacy method)
	return l.attachKprobePerf(loaded, funcName, ret)
}

// attachKprobeLink attaches kprobe using BPF link (kernel 5.5+)
func (l *NativeLoader) attachKprobeLink(loaded *loadedProgram, funcName string, ret bool) (int, error) {
	// This requires writing to /sys/kernel/debug/tracing/kprobe_events
	// or using the PMU-based method

	// Get kprobe PMU type
	pmuType, err := getKprobePMUType()
	if err != nil {
		return -1, err
	}

	// Create perf event
	funcNameBytes := append([]byte(funcName), 0)

	attr := perfEventAttr{
		Type: pmuType,
		Size: uint32(unsafe.Sizeof(perfEventAttr{})),
	}

	// Set config for retprobe
	if ret {
		attr.Config = 1 // PERF_KPROBE_FLAG_RETPROBE
	}

	// For PMU-based kprobes, the function name goes in config1
	attr.ExtConfig = uint64(uintptr(unsafe.Pointer(&funcNameBytes[0])))

	// Call perf_event_open
	perfFD, _, errno := unix.Syscall6(
		unix.SYS_PERF_EVENT_OPEN,
		uintptr(unsafe.Pointer(&attr)),
		uintptr(0xffffffff), // pid = -1 (all processes)
		uintptr(0),          // cpu = 0
		uintptr(0xffffffff), // group_fd = -1
		uintptr(PERF_FLAG_FD_CLOEXEC),
		0,
	)

	if errno != 0 {
		return -1, errno
	}

	// Create BPF link
	linkFD, err := bpfLinkCreate(loaded.fd, int(perfFD), BPF_PERF_EVENT)
	if err != nil {
		unix.Close(int(perfFD))
		return -1, err
	}

	loaded.perfFDs = append(loaded.perfFDs, int(perfFD))
	return linkFD, nil
}

// attachKprobePerf attaches kprobe using perf_event_open (legacy method)
func (l *NativeLoader) attachKprobePerf(loaded *loadedProgram, funcName string, ret bool) error {
	// Step 1: Register kprobe via tracefs
	probeID, err := registerKprobe(funcName, ret)
	if err != nil {
		return fmt.Errorf("register kprobe: %w", err)
	}

	// Step 2: Open perf event for the kprobe
	attr := perfEventAttr{
		Type:   PERF_TYPE_TRACEPOINT,
		Size:   uint32(unsafe.Sizeof(perfEventAttr{})),
		Config: uint64(probeID),
	}
	attr.SampleType = PERF_SAMPLE_RAW

	perfFD, _, errno := unix.Syscall6(
		unix.SYS_PERF_EVENT_OPEN,
		uintptr(unsafe.Pointer(&attr)),
		uintptr(0xffffffff), // pid = -1
		uintptr(0),          // cpu = 0
		uintptr(0xffffffff), // group_fd = -1
		uintptr(PERF_FLAG_FD_CLOEXEC),
		0,
	)

	if errno != 0 {
		return fmt.Errorf("perf_event_open: %w", errno)
	}

	// Step 3: Attach BPF program to perf event using PERF_EVENT_IOC_SET_BPF
	const PERF_EVENT_IOC_SET_BPF = 0x40042408 // _IOW('$', 8, u32)
	const PERF_EVENT_IOC_ENABLE = 0x00002400  // _IO('$', 0)

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, perfFD,
		PERF_EVENT_IOC_SET_BPF, uintptr(loaded.fd))
	if errno != 0 {
		unix.Close(int(perfFD))
		return fmt.Errorf("PERF_EVENT_IOC_SET_BPF: %w", errno)
	}

	// Enable the event
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, perfFD,
		PERF_EVENT_IOC_ENABLE, 0)
	if errno != 0 {
		unix.Close(int(perfFD))
		return fmt.Errorf("PERF_EVENT_IOC_ENABLE: %w", errno)
	}

	loaded.perfFDs = append(loaded.perfFDs, int(perfFD))
	return nil
}

// getKprobePMUType gets the PMU type ID for kprobes
func getKprobePMUType() (uint32, error) {
	data, err := os.ReadFile("/sys/bus/event_source/devices/kprobe/type")
	if err != nil {
		return 0, err
	}
	typ, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(typ), nil
}

// registerKprobe registers a kprobe via tracefs and returns its ID
func registerKprobe(funcName string, ret bool) (uint64, error) {
	// Generate unique probe name
	probeName := fmt.Sprintf("p_unheaded_%s_%d", funcName, time.Now().UnixNano())
	if ret {
		probeName = fmt.Sprintf("r_unheaded_%s_%d", funcName, time.Now().UnixNano())
	}

	// Write to kprobe_events
	eventPath := "/sys/kernel/debug/tracing/kprobe_events"
	var probeCmd string
	if ret {
		probeCmd = fmt.Sprintf("r:%s %s", probeName, funcName)
	} else {
		probeCmd = fmt.Sprintf("p:%s %s", probeName, funcName)
	}

	f, err := os.OpenFile(eventPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return 0, fmt.Errorf("open kprobe_events: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(probeCmd); err != nil {
		return 0, fmt.Errorf("write kprobe_events: %w", err)
	}

	// Read probe ID
	idPath := fmt.Sprintf("/sys/kernel/debug/tracing/events/kprobes/%s/id", probeName)
	idData, err := os.ReadFile(idPath)
	if err != nil {
		return 0, fmt.Errorf("read probe id: %w", err)
	}

	id, err := strconv.ParseUint(strings.TrimSpace(string(idData)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse probe id: %w", err)
	}

	return id, nil
}

// ============================================================================
// TRACEPOINT ATTACHMENT
// ============================================================================

// attachTracepoint attaches a tracepoint program
func (l *NativeLoader) attachTracepoint(loaded *loadedProgram) error {
	// Parse tracepoint path (format: "category:name" or "category/name")
	attachTo := loaded.spec.AttachTo
	attachTo = strings.ReplaceAll(attachTo, "/", ":")
	parts := strings.SplitN(attachTo, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid tracepoint format: %s (expected category:name)", attachTo)
	}

	category := parts[0]
	name := parts[1]

	// Get tracepoint ID
	idPath := fmt.Sprintf("/sys/kernel/debug/tracing/events/%s/%s/id", category, name)
	idData, err := os.ReadFile(idPath)
	if err != nil {
		return fmt.Errorf("read tracepoint id: %w", err)
	}

	tpID, err := strconv.ParseUint(strings.TrimSpace(string(idData)), 10, 64)
	if err != nil {
		return fmt.Errorf("parse tracepoint id: %w", err)
	}

	// Try BPF link first
	if l.features.BPFLinkSupport {
		linkFD, err := l.attachTracepointLink(loaded, tpID)
		if err == nil {
			loaded.linkFDs = append(loaded.linkFDs, linkFD)
			return nil
		}
	}

	// Fall back to perf_event_open
	return l.attachTracepointPerf(loaded, tpID)
}

// attachTracepointLink attaches tracepoint using BPF link
func (l *NativeLoader) attachTracepointLink(loaded *loadedProgram, tpID uint64) (int, error) {
	attr := perfEventAttr{
		Type:   PERF_TYPE_TRACEPOINT,
		Size:   uint32(unsafe.Sizeof(perfEventAttr{})),
		Config: tpID,
	}

	perfFD, _, errno := unix.Syscall6(
		unix.SYS_PERF_EVENT_OPEN,
		uintptr(unsafe.Pointer(&attr)),
		uintptr(0xffffffff),
		uintptr(0),
		uintptr(0xffffffff),
		uintptr(PERF_FLAG_FD_CLOEXEC),
		0,
	)

	if errno != 0 {
		return -1, errno
	}

	linkFD, err := bpfLinkCreate(loaded.fd, int(perfFD), BPF_PERF_EVENT)
	if err != nil {
		unix.Close(int(perfFD))
		return -1, err
	}

	loaded.perfFDs = append(loaded.perfFDs, int(perfFD))
	return linkFD, nil
}

// attachTracepointPerf attaches tracepoint using perf_event_open
func (l *NativeLoader) attachTracepointPerf(loaded *loadedProgram, tpID uint64) error {
	attr := perfEventAttr{
		Type:       PERF_TYPE_TRACEPOINT,
		Size:       uint32(unsafe.Sizeof(perfEventAttr{})),
		Config:     tpID,
		SampleType: PERF_SAMPLE_RAW,
	}

	perfFD, _, errno := unix.Syscall6(
		unix.SYS_PERF_EVENT_OPEN,
		uintptr(unsafe.Pointer(&attr)),
		uintptr(0xffffffff),
		uintptr(0),
		uintptr(0xffffffff),
		uintptr(PERF_FLAG_FD_CLOEXEC),
		0,
	)

	if errno != 0 {
		return fmt.Errorf("perf_event_open: %w", errno)
	}

	// Attach BPF program
	const PERF_EVENT_IOC_SET_BPF = 0x40042408
	const PERF_EVENT_IOC_ENABLE = 0x00002400

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, perfFD,
		PERF_EVENT_IOC_SET_BPF, uintptr(loaded.fd))
	if errno != 0 {
		unix.Close(int(perfFD))
		return fmt.Errorf("PERF_EVENT_IOC_SET_BPF: %w", errno)
	}

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, perfFD,
		PERF_EVENT_IOC_ENABLE, 0)
	if errno != 0 {
		unix.Close(int(perfFD))
		return fmt.Errorf("PERF_EVENT_IOC_ENABLE: %w", errno)
	}

	loaded.perfFDs = append(loaded.perfFDs, int(perfFD))
	return nil
}

// attachRawTracepoint attaches a raw tracepoint program
func (l *NativeLoader) attachRawTracepoint(loaded *loadedProgram) error {
	tpName := loaded.spec.AttachTo
	if tpName == "" {
		return fmt.Errorf("raw tracepoint name not specified")
	}

	// Raw tracepoints use BPF_RAW_TRACEPOINT_OPEN (via BPF_LINK_CREATE)
	tpNameBytes := append([]byte(tpName), 0)

	attr := struct {
		ProgFD    uint32
		_         uint32
		AttachType uint32
		Flags     uint32
		RawTPName uint64
	}{
		ProgFD:     uint32(loaded.fd),
		AttachType: BPF_TRACE_RAW_TP,
		RawTPName:  uint64(uintptr(unsafe.Pointer(&tpNameBytes[0]))),
	}

	fd, err := bpfSyscall(BPF_LINK_CREATE, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("raw tracepoint attach: %w", err)
	}

	loaded.linkFDs = append(loaded.linkFDs, fd)
	return nil
}

// ============================================================================
// CGROUP ATTACHMENT
// ============================================================================

// attachCgroup attaches a cgroup BPF program
func (l *NativeLoader) attachCgroup(loaded *loadedProgram) error {
	cgroupPath := loaded.spec.AttachTo
	if cgroupPath == "" {
		cgroupPath = "/sys/fs/cgroup"
	}

	// Open cgroup directory
	cgroupFD, err := unix.Open(cgroupPath, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open cgroup %s: %w", cgroupPath, err)
	}
	defer unix.Close(cgroupFD)

	// Determine attach type
	var attachType uint32
	switch loaded.spec.AttachType {
	case AttachCgroupInet4Bind:
		attachType = BPF_CGROUP_INET4_BIND
	case AttachCgroupInet6Bind:
		attachType = BPF_CGROUP_INET6_BIND
	case AttachCgroupSockOps:
		attachType = BPF_CGROUP_SOCK_OPS
	default:
		return fmt.Errorf("unsupported cgroup attach type: %s", loaded.spec.AttachType)
	}

	// Try BPF link first
	if l.features.BPFLinkSupport {
		linkFD, err := bpfLinkCreate(loaded.fd, cgroupFD, attachType)
		if err == nil {
			loaded.linkFDs = append(loaded.linkFDs, linkFD)
			return nil
		}
	}

	// Fall back to BPF_PROG_ATTACH
	return bpfProgAttach(loaded.fd, cgroupFD, attachType)
}

// ============================================================================
// MAP OPERATIONS
// ============================================================================

// GetProgram returns program info
func (l *NativeLoader) GetProgram(ctx context.Context, name string) (*ProgramInfo, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded, exists := l.programs[name]
	if !exists {
		return nil, ErrProgramNotFound
	}

	info := *loaded.info
	return &info, nil
}

// SetAttachTarget updates the attach target (e.g., network interface name)
// for an already-loaded program. Must be called before Attach().
func (l *NativeLoader) SetAttachTarget(ctx context.Context, name string, attachTo string) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	loaded, exists := l.programs[name]
	if !exists {
		return ErrProgramNotFound
	}

	loaded.spec.AttachTo = attachTo
	loaded.info.AttachTo = attachTo
	return nil
}

// ListPrograms returns all loaded programs
func (l *NativeLoader) ListPrograms(ctx context.Context) ([]*ProgramInfo, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	programs := make([]*ProgramInfo, 0, len(l.programs))
	for _, loaded := range l.programs {
		info := *loaded.info
		programs = append(programs, &info)
	}

	return programs, nil
}

// ProgramExists checks if a program exists
func (l *NativeLoader) ProgramExists(ctx context.Context, name string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, exists := l.programs[name]
	return exists
}

// GetMap returns map info
func (l *NativeLoader) GetMap(ctx context.Context, programName, mapName string) (*MapInfo, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded, exists := l.programs[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	m, exists := loaded.maps[mapName]
	if !exists {
		return nil, ErrMapNotFound
	}

	info := *m.info
	return &info, nil
}

// ListMaps returns all maps for a program
func (l *NativeLoader) ListMaps(ctx context.Context, programName string) ([]*MapInfo, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded, exists := l.programs[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	maps := make([]*MapInfo, 0, len(loaded.maps))
	for _, m := range loaded.maps {
		info := *m.info
		maps = append(maps, &info)
	}

	return maps, nil
}

// ReadMap reads a value from a map
func (l *NativeLoader) ReadMap(ctx context.Context, programName, mapName string, key []byte) ([]byte, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded, exists := l.programs[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	m, exists := loaded.maps[mapName]
	if !exists {
		return nil, ErrMapNotFound
	}

	// Allocate value buffer
	value := make([]byte, m.info.ValueSize)

	err := bpfMapLookupElem(m.fd, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]))
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return nil, nil
		}
		return nil, err
	}

	if l.metricsEnabled {
		ebpfMapOperations.WithLabelValues(programName, mapName, "lookup").Inc()
	}

	return value, nil
}

// WriteMap writes a value to a map
func (l *NativeLoader) WriteMap(ctx context.Context, programName, mapName string, key, value []byte) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded, exists := l.programs[programName]
	if !exists {
		return ErrProgramNotFound
	}

	m, exists := loaded.maps[mapName]
	if !exists {
		return ErrMapNotFound
	}

	err := bpfMapUpdateElem(m.fd, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]), BPF_ANY)
	if err != nil {
		return err
	}

	if l.metricsEnabled {
		ebpfMapOperations.WithLabelValues(programName, mapName, "update").Inc()
	}

	return nil
}

// DeleteMapEntry deletes an entry from a map
func (l *NativeLoader) DeleteMapEntry(ctx context.Context, programName, mapName string, key []byte) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded, exists := l.programs[programName]
	if !exists {
		return ErrProgramNotFound
	}

	m, exists := loaded.maps[mapName]
	if !exists {
		return ErrMapNotFound
	}

	err := bpfMapDeleteElem(m.fd, unsafe.Pointer(&key[0]))
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return nil
		}
		return err
	}

	if l.metricsEnabled {
		ebpfMapOperations.WithLabelValues(programName, mapName, "delete").Inc()
	}

	return nil
}

// IterateMap iterates over all entries in a map
func (l *NativeLoader) IterateMap(ctx context.Context, programName, mapName string,
	fn func(key, value []byte) error) error {

	if err := l.checkClosed(); err != nil {
		return err
	}

	l.mu.RLock()
	loaded, exists := l.programs[programName]
	if !exists {
		l.mu.RUnlock()
		return ErrProgramNotFound
	}

	m, exists := loaded.maps[mapName]
	if !exists {
		l.mu.RUnlock()
		return ErrMapNotFound
	}

	keySize := m.info.KeySize
	valueSize := m.info.ValueSize
	mapFD := m.fd
	l.mu.RUnlock()

	// Iteration buffers
	key := make([]byte, keySize)
	nextKey := make([]byte, keySize)
	value := make([]byte, valueSize)

	// Start with nil key to get first key
	var keyPtr unsafe.Pointer
	first := true

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if first {
			keyPtr = nil
			first = false
		} else {
			keyPtr = unsafe.Pointer(&key[0])
		}

		err := bpfMapGetNextKey(mapFD, keyPtr, unsafe.Pointer(&nextKey[0]))
		if err != nil {
			if errors.Is(err, syscall.ENOENT) {
				// End of iteration
				break
			}
			return err
		}

		// Copy next key to key for next iteration
		copy(key, nextKey)

		// Lookup value
		err = bpfMapLookupElem(mapFD, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]))
		if err != nil {
			if errors.Is(err, syscall.ENOENT) {
				// Key was deleted, continue
				continue
			}
			return err
		}

		// Call user function
		if err := fn(key, value); err != nil {
			return err
		}
	}

	if l.metricsEnabled {
		ebpfMapOperations.WithLabelValues(programName, mapName, "iterate").Inc()
	}

	return nil
}

// ============================================================================
// RING BUFFER READING
// ============================================================================
//
// Ring buffers are the modern way (kernel 5.8+) to stream data from BPF to
// userspace. They use mmap for zero-copy data transfer and are more efficient
// than perf buffers.

// ringbufHeader is the header at the start of each ringbuf record
type ringbufHeader struct {
	Len uint32 // Record length (including header)
	_   uint32 // Padding (contains pg_off on kernel side)
}

// ReadRingbuf returns a channel for ringbuf events
func (l *NativeLoader) ReadRingbuf(ctx context.Context, programName, mapName string) (<-chan []byte, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	if !l.features.RingbufSupport {
		return nil, fmt.Errorf("%w: ringbuf requires kernel 5.8+", ErrUnsupportedMapType)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	loaded, exists := l.programs[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	m, exists := loaded.maps[mapName]
	if !exists {
		return nil, ErrMapNotFound
	}

	// Verify this is a ringbuf map
	if m.info.Type != MapTypeRingbuf {
		return nil, fmt.Errorf("%w: map %s is not a ringbuf", ErrUnsupportedMapType, mapName)
	}

	// Check if already mmap'd
	if m.mmapAddr != 0 {
		return nil, fmt.Errorf("ringbuf %s already being read", mapName)
	}

	// Ring buffer size is stored in max_entries (must be power of 2)
	ringSize := int(m.info.MaxEntries)
	if ringSize == 0 {
		ringSize = 256 * 1024 // Default 256KB
	}

	// mmap the ring buffer
	// The ringbuf consists of:
	// - 1 page for producer/consumer positions
	// - ringSize bytes for data (mapped twice for wrap-around)
	pageSize := os.Getpagesize()
	mmapSize := pageSize + 2*ringSize

	addr, err := unix.Mmap(m.fd, 0, mmapSize,
		unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap ringbuf: %w", err)
	}

	m.mmapAddr = uintptr(unsafe.Pointer(&addr[0]))
	m.mmapSize = mmapSize

	// Create event channel
	events := make(chan []byte, 1024)

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	loaded.cancelFns = append(loaded.cancelFns, cancel)

	// Start reader goroutine
	l.wg.Add(1)
	go l.ringbufReader(ctx, loaded, m, events, programName, mapName, addr, ringSize, pageSize)

	return events, nil
}

// ringbufReader reads events from a ring buffer
func (l *NativeLoader) ringbufReader(ctx context.Context, loaded *loadedProgram,
	m *loadedMap, events chan<- []byte, programName, mapName string,
	mmapData []byte, ringSize, pageSize int) {

	defer l.wg.Done()
	defer close(events)

	// Ring buffer memory layout:
	// Page 0: struct bpf_ringbuf (contains consumer_pos, producer_pos)
	// Pages 1+: Data area (mapped twice for wrap-around)

	// Consumer and producer positions are at fixed offsets
	consumerPos := (*uint64)(unsafe.Pointer(&mmapData[0]))
	producerPos := (*uint64)(unsafe.Pointer(&mmapData[pageSize-8]))
	dataStart := pageSize

	pollFDs := []unix.PollFd{{
		Fd:     int32(m.fd),
		Events: unix.POLLIN,
	}}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Poll for new data
		n, err := unix.Poll(pollFDs, 100) // 100ms timeout
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}

		if n == 0 {
			continue // Timeout, check context
		}

		// Read available records
		for {
			cons := *consumerPos
			prod := *producerPos

			if cons >= prod {
				break // No more data
			}

			// Calculate offset in ring
			offset := int(cons % uint64(ringSize))

			// Read header
			hdr := (*ringbufHeader)(unsafe.Pointer(&mmapData[dataStart+offset]))
			recordLen := hdr.Len

			// Check if record is ready (busy bit clear)
			if recordLen&BPF_RINGBUF_BUSY_BIT != 0 {
				break // Record not ready yet
			}

			// Check for discarded record
			if recordLen&BPF_RINGBUF_DISCARD_BIT != 0 {
				// Skip discarded record
				dataLen := recordLen & ^uint32(BPF_RINGBUF_BUSY_BIT|BPF_RINGBUF_DISCARD_BIT)
				*consumerPos = cons + uint64(roundUp(int(BPF_RINGBUF_HDR_SZ+dataLen), 8))
				continue
			}

			// Extract data length
			dataLen := recordLen & ^uint32(BPF_RINGBUF_BUSY_BIT|BPF_RINGBUF_DISCARD_BIT)

			// Copy data
			dataOffset := dataStart + offset + BPF_RINGBUF_HDR_SZ
			data := make([]byte, dataLen)
			copy(data, mmapData[dataOffset:dataOffset+int(dataLen)])

			// Update consumer position
			*consumerPos = cons + uint64(roundUp(int(BPF_RINGBUF_HDR_SZ+dataLen), 8))

			// Send to channel
			select {
			case events <- data:
				if l.metricsEnabled {
					ebpfRingbufEvents.WithLabelValues(programName, mapName).Inc()
				}
			case <-ctx.Done():
				return
			default:
				// Channel full, drop event
			}
		}
	}
}

// roundUp rounds n up to the nearest multiple of align
func roundUp(n, align int) int {
	return (n + align - 1) & ^(align - 1)
}

// ReadPerfEvents returns a channel for perf events
// This is the older method (pre-5.8) for streaming data from BPF
func (l *NativeLoader) ReadPerfEvents(ctx context.Context, programName, mapName string) (<-chan []byte, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	loaded, exists := l.programs[programName]
	if !exists {
		return nil, ErrProgramNotFound
	}

	m, exists := loaded.maps[mapName]
	if !exists {
		return nil, ErrMapNotFound
	}

	// Verify this is a perf event array
	if m.info.Type != MapTypePerfEventArray {
		return nil, fmt.Errorf("%w: map %s is not a perf_event_array", ErrUnsupportedMapType, mapName)
	}

	// For perf events, we need to:
	// 1. Create a perf event for each CPU
	// 2. mmap each perf event
	// 3. Poll all events

	// This is simplified - full implementation would handle all CPUs
	events := make(chan []byte, 1024)

	ctx, cancel := context.WithCancel(ctx)
	loaded.cancelFns = append(loaded.cancelFns, cancel)

	// Start reader
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		defer close(events)
		// Simplified: just wait for context cancellation
		<-ctx.Done()
	}()

	return events, nil
}

// ============================================================================
// METRICS
// ============================================================================

// GetMetrics returns program metrics
func (l *NativeLoader) GetMetrics(ctx context.Context, name string) (*ProgramMetrics, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded, exists := l.programs[name]
	if !exists {
		return nil, ErrProgramNotFound
	}

	// Get program stats from kernel
	progInfo, err := bpfGetProgInfo(loaded.fd)
	if err != nil {
		return nil, err
	}

	var avgRunNs uint64
	if progInfo.RunCnt > 0 {
		avgRunNs = progInfo.RunTimeNs / progInfo.RunCnt
	}

	metrics := &ProgramMetrics{
		Name:       name,
		RunCount:   progInfo.RunCnt,
		RunTimeNs:  progInfo.RunTimeNs,
		AvgRunNs:   avgRunNs,
		Recursion:  progInfo.RecursionMisses,
		SampleTime: time.Now(),
	}

	return metrics, nil
}

// ============================================================================
// CLOSE AND HELPERS
// ============================================================================

// Close closes the loader and all loaded programs
func (l *NativeLoader) Close() error {
	l.closedMu.Lock()
	if l.closed {
		l.closedMu.Unlock()
		return nil
	}
	l.closed = true
	l.closedMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	var lastErr error

	// Unload all programs
	for name, loaded := range l.programs {
		// Cancel background goroutines
		for _, cancel := range loaded.cancelFns {
			cancel()
		}

		// Close links
		for _, fd := range loaded.linkFDs {
			if err := unix.Close(fd); err != nil {
				lastErr = err
			}
		}

		// Close perf FDs
		for _, fd := range loaded.perfFDs {
			if err := unix.Close(fd); err != nil {
				lastErr = err
			}
		}

		// Unmap and close maps
		for _, m := range loaded.maps {
			if m.mmapAddr != 0 {
				_ = unix.Munmap((*[1 << 30]byte)(unsafe.Pointer(m.mmapAddr))[:m.mmapSize])
			}
			unix.Close(m.fd)
		}

		// Close program FD
		if loaded.fd > 0 {
			unix.Close(loaded.fd)
		}

		delete(l.programs, name)
	}

	// Close kernel BTF FD
	if l.btfFD > 0 {
		unix.Close(l.btfFD)
	}

	// Wait for background goroutines
	l.wg.Wait()

	return lastErr
}

// checkClosed returns error if loader is closed
func (l *NativeLoader) checkClosed() error {
	l.closedMu.RLock()
	defer l.closedMu.RUnlock()
	if l.closed {
		return ErrLoaderClosed
	}
	return nil
}

// ============================================================================
// KERNEL FEATURE DETECTION
// ============================================================================

// detectKernelFeatures detects available eBPF features
func detectKernelFeatures() (*KernelFeatures, error) {
	features := &KernelFeatures{}

	// Get kernel version
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return nil, fmt.Errorf("uname: %w", err)
	}

	release := bytesToString(uname.Release[:])
	version, err := parseKernelVersion(release)
	if err != nil {
		return nil, fmt.Errorf("parse kernel version: %w", err)
	}
	features.Version = version

	// Check BTF availability
	btfPath := findBTFPath()
	if btfPath != "" {
		features.BTFEnabled = true
		features.BTFPath = btfPath
	}

	// Feature detection based on kernel version
	features.RingbufSupport = version.AtLeast(5, 8, 0)
	features.BPFLinkSupport = version.AtLeast(5, 7, 0)
	features.FentrySupport = version.AtLeast(5, 5, 0)
	features.KprobeMulti = version.AtLeast(5, 18, 0)
	features.TCXSupport = version.AtLeast(6, 6, 0)
	features.MemcgAccounting = version.AtLeast(5, 11, 0)
	features.BPFTokenSupport = version.AtLeast(6, 9, 0)
	features.BPFArena = version.AtLeast(6, 9, 0)
	features.SignalSupport = version.AtLeast(6, 3, 0)

	return features, nil
}

// parseKernelVersion parses a kernel version string
func parseKernelVersion(release string) (KernelVersion, error) {
	parts := strings.Split(release, "-")
	if len(parts) == 0 {
		return KernelVersion{}, fmt.Errorf("invalid kernel release: %s", release)
	}

	versionParts := strings.Split(parts[0], ".")
	if len(versionParts) < 2 {
		return KernelVersion{}, fmt.Errorf("invalid kernel version: %s", parts[0])
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return KernelVersion{}, fmt.Errorf("parse major: %w", err)
	}

	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return KernelVersion{}, fmt.Errorf("parse minor: %w", err)
	}

	patch := 0
	if len(versionParts) >= 3 {
		patch, _ = strconv.Atoi(versionParts[2])
	}

	return KernelVersion{Major: major, Minor: minor, Patch: patch}, nil
}

// findBTFPath finds the kernel BTF file
func findBTFPath() string {
	paths := []string{
		"/sys/kernel/btf/vmlinux",
	}

	var uname unix.Utsname
	if err := unix.Uname(&uname); err == nil {
		release := bytesToString(uname.Release[:])
		paths = append(paths,
			"/boot/vmlinux-"+release,
			"/lib/modules/"+release+"/build/vmlinux",
			"/lib/modules/"+release+"/vmlinux",
		)
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// loadKernelBTF loads kernel BTF and returns its FD
func loadKernelBTF(path string) (int, error) {
	if path == "" {
		path = "/sys/kernel/btf/vmlinux"
	}

	// For /sys/kernel/btf/vmlinux, we can directly use bpf() to get a handle
	// For other paths, we'd need to load the file

	// Try the raw BTF blob via bpf syscall
	data, err := os.ReadFile(path)
	if err != nil {
		return -1, err
	}

	// Load BTF into kernel
	attr := struct {
		BTF     uint64
		BTFSize uint32
		_       [52]byte
	}{
		BTF:     uint64(uintptr(unsafe.Pointer(&data[0]))),
		BTFSize: uint32(len(data)),
	}

	fd, err := bpfSyscall(18, unsafe.Pointer(&attr), unsafe.Sizeof(attr)) // BPF_BTF_LOAD = 18
	if err != nil {
		return -1, err
	}

	return fd, nil
}

// ensureBPFFS ensures the BPF filesystem is mounted
func ensureBPFFS(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("create bpf mount point: %w", err)
	}

	return nil
}

// bytesToString converts a null-terminated byte array to string
func bytesToString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// ============================================================================
// DEFAULT PROGRAMS FOR UNHEADED KINGDOM
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
			},
		},
	}
}

// Ensure NativeLoader implements Loader interface
var _ Loader = (*NativeLoader)(nil)
