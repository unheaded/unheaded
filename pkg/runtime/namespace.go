//go:build linux

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// NamespaceManager manages Linux namespaces.
type NamespaceManager struct {
	mu sync.RWMutex

	// Base path for namespace files
	nsBasePath string

	// Active namespaces
	namespaces map[string]*NamespaceSet
}

// NamespaceSet represents a set of namespaces.
type NamespaceSet struct {
	ID   string
	PIDs map[NamespaceType]int // File descriptors
	Paths map[NamespaceType]string
}

// NamespaceInfo contains information about a namespace.
type NamespaceInfo struct {
	Type  NamespaceType
	Path  string
	Inode uint64
}

// NewNamespaceManager creates a new namespace manager.
func NewNamespaceManager() (*NamespaceManager, error) {
	nsBasePath := "/var/run/unheaded/ns"
	if err := os.MkdirAll(nsBasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create namespace base path: %w", err)
	}

	return &NamespaceManager{
		nsBasePath: nsBasePath,
		namespaces: make(map[string]*NamespaceSet),
	}, nil
}

// CreateNamespaces creates a new set of namespaces.
func (m *NamespaceManager) CreateNamespaces(id string, types []NamespaceType) (*NamespaceSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	nsSet := &NamespaceSet{
		ID:    id,
		PIDs:  make(map[NamespaceType]int),
		Paths: make(map[NamespaceType]string),
	}

	// Create namespace directory
	nsDir := filepath.Join(m.nsBasePath, id)
	if err := os.MkdirAll(nsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create namespace directory: %w", err)
	}

	for _, nsType := range types {
		nsPath := filepath.Join(nsDir, string(nsType))

		// Create bind mount target
		if err := createNamespaceBindTarget(nsPath); err != nil {
			m.cleanupNamespaces(nsSet)
			return nil, fmt.Errorf("failed to create namespace bind target for %s: %w", nsType, err)
		}

		nsSet.Paths[nsType] = nsPath
	}

	m.namespaces[id] = nsSet
	return nsSet, nil
}

// createNamespaceBindTarget creates a file for namespace bind mounting.
func createNamespaceBindTarget(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL, 0444)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	return f.Close()
}

// BindNamespaces binds namespaces from a process to the namespace set.
func (m *NamespaceManager) BindNamespaces(nsSet *NamespaceSet, pid int) error {
	for nsType, nsPath := range nsSet.Paths {
		procNsPath := fmt.Sprintf("/proc/%d/ns/%s", pid, nsTypeToProc(nsType))

		// Bind mount the namespace
		if err := unix.Mount(procNsPath, nsPath, "", unix.MS_BIND, ""); err != nil {
			return fmt.Errorf("failed to bind mount %s namespace: %w", nsType, err)
		}
	}

	return nil
}

// GetNamespace returns the path to a namespace.
func (m *NamespaceManager) GetNamespace(id string, nsType NamespaceType) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nsSet, ok := m.namespaces[id]
	if !ok {
		return "", fmt.Errorf("namespace set not found: %s", id)
	}

	path, ok := nsSet.Paths[nsType]
	if !ok {
		return "", fmt.Errorf("namespace type not found: %s", nsType)
	}

	return path, nil
}

// RemoveNamespaces removes a namespace set.
func (m *NamespaceManager) RemoveNamespaces(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nsSet, ok := m.namespaces[id]
	if !ok {
		return nil
	}

	m.cleanupNamespaces(nsSet)
	delete(m.namespaces, id)

	return nil
}

// cleanupNamespaces cleans up namespace files.
func (m *NamespaceManager) cleanupNamespaces(nsSet *NamespaceSet) {
	for _, path := range nsSet.Paths {
		// Unmount if mounted
		unix.Unmount(path, unix.MNT_DETACH)
		os.Remove(path)
	}

	// Remove directory
	nsDir := filepath.Join(m.nsBasePath, nsSet.ID)
	os.Remove(nsDir)
}

// nsTypeToProc converts NamespaceType to proc filesystem name.
func nsTypeToProc(nsType NamespaceType) string {
	switch nsType {
	case NetworkNamespace:
		return "net"
	case MountNamespace:
		return "mnt"
	default:
		return string(nsType)
	}
}

// GetNamespaceInfo returns information about a namespace.
func (m *NamespaceManager) GetNamespaceInfo(path string) (*NamespaceInfo, error) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to stat namespace: %w", err)
	}

	// Determine namespace type from path
	nsType := NamespaceType(filepath.Base(path))

	return &NamespaceInfo{
		Type:  nsType,
		Path:  path,
		Inode: stat.Ino,
	}, nil
}

// EnterNamespace enters a namespace.
func (m *NamespaceManager) EnterNamespace(path string, nsType NamespaceType) error {
	// Open the namespace file
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("failed to open namespace: %w", err)
	}
	defer unix.Close(fd)

	// Get clone flag for namespace type
	cloneFlag := nsTypeToCloneFlag(nsType)
	if cloneFlag == 0 {
		return fmt.Errorf("unknown namespace type: %s", nsType)
	}

	// Enter the namespace
	if err := unix.Setns(fd, cloneFlag); err != nil {
		return fmt.Errorf("failed to enter namespace: %w", err)
	}

	return nil
}

// nsTypeToCloneFlag converts NamespaceType to clone flag.
func nsTypeToCloneFlag(nsType NamespaceType) int {
	switch nsType {
	case PIDNamespace:
		return unix.CLONE_NEWPID
	case NetworkNamespace:
		return unix.CLONE_NEWNET
	case MountNamespace:
		return unix.CLONE_NEWNS
	case IPCNamespace:
		return unix.CLONE_NEWIPC
	case UTSNamespace:
		return unix.CLONE_NEWUTS
	case UserNamespace:
		return unix.CLONE_NEWUSER
	case CgroupNamespace:
		return unix.CLONE_NEWCGROUP
	default:
		return 0
	}
}

// CreateNetworkNamespace creates a new network namespace.
func (m *NamespaceManager) CreateNetworkNamespace(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	nsPath := filepath.Join(m.nsBasePath, id, "net")
	if err := os.MkdirAll(filepath.Dir(nsPath), 0755); err != nil {
		return "", err
	}

	// Create target file for bind mount
	if err := createNamespaceBindTarget(nsPath); err != nil {
		return "", err
	}

	// Create new network namespace using unshare
	// We need to do this in a separate goroutine and thread
	errCh := make(chan error, 1)
	go func() {
		// Lock this goroutine to a single thread
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Create new network namespace
		if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
			errCh <- fmt.Errorf("failed to create network namespace: %w", err)
			return
		}

		// Bind mount the namespace
		selfNsPath := "/proc/self/ns/net"
		if err := unix.Mount(selfNsPath, nsPath, "", unix.MS_BIND, ""); err != nil {
			errCh <- fmt.Errorf("failed to bind mount network namespace: %w", err)
			return
		}

		errCh <- nil
	}()

	if err := <-errCh; err != nil {
		os.Remove(nsPath)
		return "", err
	}

	// Track the namespace
	if m.namespaces[id] == nil {
		m.namespaces[id] = &NamespaceSet{
			ID:    id,
			PIDs:  make(map[NamespaceType]int),
			Paths: make(map[NamespaceType]string),
		}
	}
	m.namespaces[id].Paths[NetworkNamespace] = nsPath

	return nsPath, nil
}

// ConfigureNetworkNamespace configures a network namespace.
func (m *NamespaceManager) ConfigureNetworkNamespace(nsPath string) error {
	// Open the namespace
	nsFd, err := unix.Open(nsPath, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("failed to open namespace: %w", err)
	}
	defer unix.Close(nsFd)

	// Enter namespace and configure
	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Save current namespace
		origNsFd, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			errCh <- fmt.Errorf("failed to save original namespace: %w", err)
			return
		}
		defer unix.Close(origNsFd)

		// Enter target namespace
		if err := unix.Setns(nsFd, unix.CLONE_NEWNET); err != nil {
			errCh <- fmt.Errorf("failed to enter namespace: %w", err)
			return
		}

		// Bring up loopback interface
		if err := bringUpLoopback(); err != nil {
			// Log but don't fail
		}

		// Return to original namespace
		if err := unix.Setns(origNsFd, unix.CLONE_NEWNET); err != nil {
			errCh <- fmt.Errorf("failed to return to original namespace: %w", err)
			return
		}

		errCh <- nil
	}()

	return <-errCh
}

// bringUpLoopback brings up the loopback interface.
func bringUpLoopback() error {
	// Use netlink to bring up lo interface
	// This is a simplified version - in production would use proper netlink

	sock, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer unix.Close(sock)

	// Get interface index for lo
	ifreq := &ifreqFlags{
		name: [16]byte{'l', 'o'},
	}

	// Get current flags
	if err := ioctlIfreqFlags(sock, unix.SIOCGIFFLAGS, ifreq); err != nil {
		return err
	}

	// Set IFF_UP flag
	ifreq.flags |= unix.IFF_UP | unix.IFF_RUNNING

	// Set new flags
	if err := ioctlIfreqFlags(sock, unix.SIOCSIFFLAGS, ifreq); err != nil {
		return err
	}

	return nil
}

// ifreqFlags is the structure for interface flags.
type ifreqFlags struct {
	name  [16]byte
	flags uint16
	_     [22]byte // padding
}

// ioctlIfreqFlags performs an ioctl with ifreqFlags.
func ioctlIfreqFlags(fd int, req uint, ifreq *ifreqFlags) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(ifreq)))
	if errno != 0 {
		return errno
	}
	return nil
}

// JoinNamespace makes a process join an existing namespace.
func (m *NamespaceManager) JoinNamespace(pid int, nsType NamespaceType, nsPath string) error {
	// This is typically done by setting up the process's namespace references
	// before exec. We return the path for use in process creation.
	return nil
}

// GetProcessNamespaces returns namespace information for a process.
func (m *NamespaceManager) GetProcessNamespaces(pid int) (map[NamespaceType]*NamespaceInfo, error) {
	result := make(map[NamespaceType]*NamespaceInfo)

	nsTypes := []NamespaceType{
		PIDNamespace,
		NetworkNamespace,
		MountNamespace,
		IPCNamespace,
		UTSNamespace,
		UserNamespace,
		CgroupNamespace,
	}

	for _, nsType := range nsTypes {
		procPath := fmt.Sprintf("/proc/%d/ns/%s", pid, nsTypeToProc(nsType))
		info, err := m.GetNamespaceInfo(procPath)
		if err != nil {
			continue // Namespace might not exist
		}
		result[nsType] = info
	}

	return result, nil
}

// IsNamespaceSame checks if two namespace paths refer to the same namespace.
func (m *NamespaceManager) IsNamespaceSame(path1, path2 string) (bool, error) {
	info1, err := m.GetNamespaceInfo(path1)
	if err != nil {
		return false, err
	}

	info2, err := m.GetNamespaceInfo(path2)
	if err != nil {
		return false, err
	}

	return info1.Inode == info2.Inode, nil
}

// CreateUserNamespace creates a user namespace with ID mapping.
func (m *NamespaceManager) CreateUserNamespace(id string, uidMappings, gidMappings []IDMapping) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	nsPath := filepath.Join(m.nsBasePath, id, "user")
	if err := os.MkdirAll(filepath.Dir(nsPath), 0755); err != nil {
		return "", err
	}

	if err := createNamespaceBindTarget(nsPath); err != nil {
		return "", err
	}

	// Create and configure user namespace
	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Create new user namespace
		if err := unix.Unshare(unix.CLONE_NEWUSER); err != nil {
			errCh <- fmt.Errorf("failed to create user namespace: %w", err)
			return
		}

		// Write UID mappings
		if len(uidMappings) > 0 {
			if err := writeIDMappings("/proc/self/uid_map", uidMappings); err != nil {
				errCh <- fmt.Errorf("failed to write uid_map: %w", err)
				return
			}
		}

		// Write "deny" to setgroups before writing gid_map
		if err := os.WriteFile("/proc/self/setgroups", []byte("deny"), 0); err != nil {
			// Might not exist on older kernels
		}

		// Write GID mappings
		if len(gidMappings) > 0 {
			if err := writeIDMappings("/proc/self/gid_map", gidMappings); err != nil {
				errCh <- fmt.Errorf("failed to write gid_map: %w", err)
				return
			}
		}

		// Bind mount the namespace
		selfNsPath := "/proc/self/ns/user"
		if err := unix.Mount(selfNsPath, nsPath, "", unix.MS_BIND, ""); err != nil {
			errCh <- fmt.Errorf("failed to bind mount user namespace: %w", err)
			return
		}

		errCh <- nil
	}()

	if err := <-errCh; err != nil {
		os.Remove(nsPath)
		return "", err
	}

	if m.namespaces[id] == nil {
		m.namespaces[id] = &NamespaceSet{
			ID:    id,
			PIDs:  make(map[NamespaceType]int),
			Paths: make(map[NamespaceType]string),
		}
	}
	m.namespaces[id].Paths[UserNamespace] = nsPath

	return nsPath, nil
}

// IDMapping represents a user/group ID mapping.
type IDMapping struct {
	ContainerID int
	HostID      int
	Size        int
}

// writeIDMappings writes ID mappings to a file.
func writeIDMappings(path string, mappings []IDMapping) error {
	var content string
	for _, m := range mappings {
		content += fmt.Sprintf("%d %d %d\n", m.ContainerID, m.HostID, m.Size)
	}
	return os.WriteFile(path, []byte(content), 0)
}

// SetHostname sets the hostname in a UTS namespace.
func (m *NamespaceManager) SetHostname(nsPath, hostname string) error {
	// Open and enter the namespace
	nsFd, err := unix.Open(nsPath, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(nsFd)

	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		origFd, err := unix.Open("/proc/self/ns/uts", unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			errCh <- err
			return
		}
		defer unix.Close(origFd)

		if err := unix.Setns(nsFd, unix.CLONE_NEWUTS); err != nil {
			errCh <- err
			return
		}

		if err := unix.Sethostname([]byte(hostname)); err != nil {
			unix.Setns(origFd, unix.CLONE_NEWUTS)
			errCh <- err
			return
		}

		if err := unix.Setns(origFd, unix.CLONE_NEWUTS); err != nil {
			errCh <- err
			return
		}

		errCh <- nil
	}()

	return <-errCh
}

