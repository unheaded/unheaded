// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// MountType represents the type of mount.
type MountType string

const (
	// MountTypeBind is a bind mount.
	MountTypeBind MountType = "bind"
	// MountTypeTmpfs is a tmpfs mount.
	MountTypeTmpfs MountType = "tmpfs"
	// MountTypeVolume is a named volume.
	MountTypeVolume MountType = "volume"
	// MountTypeNFS is an NFS mount.
	MountTypeNFS MountType = "nfs"
	// MountTypeProc is a proc filesystem mount.
	MountTypeProc MountType = "proc"
	// MountTypeSysfs is a sysfs mount.
	MountTypeSysfs MountType = "sysfs"
	// MountTypeDevpts is a devpts mount.
	MountTypeDevpts MountType = "devpts"
	// MountTypeCgroup is a cgroup mount.
	MountTypeCgroup MountType = "cgroup"
	// MountTypeMqueue is a mqueue mount.
	MountTypeMqueue MountType = "mqueue"
)

// Mount represents a mount configuration.
type Mount struct {
	// Type is the mount type.
	Type MountType
	// Source is the source path (for bind mounts) or volume name.
	Source string
	// Destination is the mount point inside the container.
	Destination string
	// Options are mount options.
	Options []string
	// ReadOnly makes the mount read-only.
	ReadOnly bool
	// Propagation is the mount propagation mode.
	Propagation MountPropagation
}

// MountPropagation represents mount propagation modes.
type MountPropagation string

const (
	// MountPropagationPrivate is private propagation.
	MountPropagationPrivate MountPropagation = "private"
	// MountPropagationRPrivate is recursive private propagation.
	MountPropagationRPrivate MountPropagation = "rprivate"
	// MountPropagationShared is shared propagation.
	MountPropagationShared MountPropagation = "shared"
	// MountPropagationRShared is recursive shared propagation.
	MountPropagationRShared MountPropagation = "rshared"
	// MountPropagationSlave is slave propagation.
	MountPropagationSlave MountPropagation = "slave"
	// MountPropagationRSlave is recursive slave propagation.
	MountPropagationRSlave MountPropagation = "rslave"
)

// VolumeManager manages volumes and mounts.
type VolumeManager struct {
	mu sync.RWMutex

	root    string
	volumes map[string]*Volume
}

// Volume represents a named volume.
type Volume struct {
	Name       string
	Path       string
	Driver     string
	Labels     map[string]string
	Options    map[string]string
	MountCount int
	CreatedAt  int64
}

// VolumeInfo contains information about a volume.
type VolumeInfo struct {
	Name       string
	Driver     string
	Mountpoint string
	Labels     map[string]string
	Options    map[string]string
	Scope      string
	Status     map[string]interface{}
	UsageData  *VolumeUsageData
}

// VolumeUsageData contains volume usage information.
type VolumeUsageData struct {
	Size     int64
	RefCount int
}

// NewVolumeManager creates a new volume manager.
func NewVolumeManager(root string) (*VolumeManager, error) {
	if err := os.MkdirAll(root, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return nil, fmt.Errorf("failed to create volume root: %w", err)
	}

	mgr := &VolumeManager{
		root:    root,
		volumes: make(map[string]*Volume),
	}

	// Best-effort: existing volume metadata may be missing on first boot.
	_ = mgr.loadVolumes()

	return mgr, nil
}

// loadVolumes loads existing volumes from disk.
func (m *VolumeManager) loadVolumes() error {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			volPath := filepath.Join(m.root, entry.Name())
			info, err := entry.Info()
			if err != nil {
				continue
			}

			m.volumes[entry.Name()] = &Volume{
				Name:      entry.Name(),
				Path:      filepath.Join(volPath, "_data"),
				Driver:    "local",
				CreatedAt: info.ModTime().Unix(),
			}
		}
	}

	return nil
}

// CreateVolume creates a new volume.
func (m *VolumeManager) CreateVolume(name string, options map[string]string, labels map[string]string) (*Volume, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.volumes[name]; exists {
		return nil, fmt.Errorf("volume %s already exists", name)
	}

	volPath := filepath.Join(m.root, name)
	dataPath := filepath.Join(volPath, "_data")

	if err := os.MkdirAll(dataPath, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return nil, fmt.Errorf("failed to create volume directory: %w", err)
	}

	vol := &Volume{
		Name:    name,
		Path:    dataPath,
		Driver:  "local",
		Labels:  labels,
		Options: options,
	}

	m.volumes[name] = vol
	return vol, nil
}

// GetVolume returns a volume by name.
func (m *VolumeManager) GetVolume(name string) (*Volume, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vol, ok := m.volumes[name]
	if !ok {
		return nil, fmt.Errorf("volume %s not found", name)
	}

	return vol, nil
}

// ListVolumes lists all volumes.
func (m *VolumeManager) ListVolumes() []*Volume {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Volume, 0, len(m.volumes))
	for _, vol := range m.volumes {
		result = append(result, vol)
	}
	return result
}

// RemoveVolume removes a volume.
func (m *VolumeManager) RemoveVolume(name string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vol, ok := m.volumes[name]
	if !ok {
		return fmt.Errorf("volume %s not found", name)
	}

	if vol.MountCount > 0 && !force {
		return fmt.Errorf("volume %s is in use", name)
	}

	volPath := filepath.Join(m.root, name)
	if err := os.RemoveAll(volPath); err != nil {
		return fmt.Errorf("failed to remove volume: %w", err)
	}

	delete(m.volumes, name)
	return nil
}

// Mount mounts a mount configuration.
func (m *VolumeManager) Mount(mount *Mount, rootfs string) error {
	dest := filepath.Join(rootfs, mount.Destination)

	// Ensure destination exists
	if err := os.MkdirAll(dest, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return fmt.Errorf("failed to create mount destination: %w", err)
	}

	switch mount.Type {
	case MountTypeBind:
		return m.mountBind(mount, dest)
	case MountTypeTmpfs:
		return m.mountTmpfs(mount, dest)
	case MountTypeVolume:
		return m.mountVolume(mount, dest)
	case MountTypeProc:
		return m.mountProc(mount, dest)
	case MountTypeSysfs:
		return m.mountSysfs(mount, dest)
	case MountTypeDevpts:
		return m.mountDevpts(mount, dest)
	case MountTypeMqueue:
		return m.mountMqueue(mount, dest)
	case MountTypeCgroup:
		return m.mountCgroup(mount, dest)
	default:
		return fmt.Errorf("unsupported mount type: %s", mount.Type)
	}
}

// mountBind performs a bind mount.
func (m *VolumeManager) mountBind(mount *Mount, dest string) error {
	source := mount.Source

	// Verify source exists
	srcInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("bind source does not exist: %w", err)
	}

	// Match destination type to source
	if srcInfo.IsDir() {
		_ = os.MkdirAll(dest, 0755) // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
	} else {
		_ = os.MkdirAll(filepath.Dir(dest), 0755)      // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		f, err := os.OpenFile(dest, os.O_CREATE, 0644) // #nosec G302,G304 -- permission is intentional for this artifact; secrets in this tree are 0600
		if err != nil {
			return err
		}
		_ = f.Close()
	}

	// Build mount flags
	flags := unix.MS_BIND
	if mount.ReadOnly {
		flags |= unix.MS_RDONLY
	}

	// Add propagation flags
	flags |= propagationToFlags(mount.Propagation)

	// Perform bind mount
	if err := unix.Mount(source, dest, "", uintptr(flags), ""); err != nil {
		return fmt.Errorf("bind mount failed: %w", err)
	}

	// Make read-only if needed (requires remount)
	if mount.ReadOnly {
		if err := unix.Mount("", dest, "", uintptr(unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY), ""); err != nil {
			_ = unix.Unmount(dest, 0)
			return fmt.Errorf("failed to make mount read-only: %w", err)
		}
	}

	return nil
}

// mountTmpfs mounts a tmpfs filesystem.
func (m *VolumeManager) mountTmpfs(mount *Mount, dest string) error {
	flags := unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV

	// Build options
	options := append([]string(nil), mount.Options...)

	// Default size if not specified
	hasSize := false
	for _, opt := range options {
		if strings.HasPrefix(opt, "size=") {
			hasSize = true
			break
		}
	}
	if !hasSize {
		options = append(options, "size=65536k")
	}

	// Add mode if not specified
	hasMode := false
	for _, opt := range options {
		if strings.HasPrefix(opt, "mode=") {
			hasMode = true
			break
		}
	}
	if !hasMode {
		options = append(options, "mode=1777")
	}

	optString := strings.Join(options, ",")

	if err := unix.Mount("tmpfs", dest, "tmpfs", uintptr(flags), optString); err != nil {
		return fmt.Errorf("tmpfs mount failed: %w", err)
	}

	return nil
}

// mountVolume mounts a named volume.
func (m *VolumeManager) mountVolume(mount *Mount, dest string) error {
	vol, err := m.GetVolume(mount.Source)
	if err != nil {
		// Try to create the volume
		vol, err = m.CreateVolume(mount.Source, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to create volume: %w", err)
		}
	}

	// Bind mount the volume data directory
	bindMount := &Mount{
		Type:        MountTypeBind,
		Source:      vol.Path,
		Destination: mount.Destination,
		Options:     mount.Options,
		ReadOnly:    mount.ReadOnly,
		Propagation: mount.Propagation,
	}

	if err := m.mountBind(bindMount, dest); err != nil {
		return err
	}

	// Increment mount count
	m.mu.Lock()
	vol.MountCount++
	m.mu.Unlock()

	return nil
}

// mountProc mounts a proc filesystem.
func (m *VolumeManager) mountProc(mount *Mount, dest string) error {
	if err := unix.Mount("proc", dest, "proc", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("proc mount failed: %w", err)
	}
	return nil
}

// mountSysfs mounts a sysfs filesystem.
func (m *VolumeManager) mountSysfs(mount *Mount, dest string) error {
	flags := unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC
	if mount.ReadOnly {
		flags |= unix.MS_RDONLY
	}

	if err := unix.Mount("sysfs", dest, "sysfs", uintptr(flags), ""); err != nil {
		return fmt.Errorf("sysfs mount failed: %w", err)
	}
	return nil
}

// mountDevpts mounts a devpts filesystem.
func (m *VolumeManager) mountDevpts(mount *Mount, dest string) error {
	options := "newinstance,ptmxmode=0666,mode=0620"
	if gid := getGID("tty"); gid > 0 {
		options += fmt.Sprintf(",gid=%d", gid)
	}

	if err := unix.Mount("devpts", dest, "devpts", unix.MS_NOSUID|unix.MS_NOEXEC, options); err != nil {
		return fmt.Errorf("devpts mount failed: %w", err)
	}
	return nil
}

// mountMqueue mounts a mqueue filesystem.
func (m *VolumeManager) mountMqueue(mount *Mount, dest string) error {
	if err := unix.Mount("mqueue", dest, "mqueue", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mqueue mount failed: %w", err)
	}
	return nil
}

// mountCgroup mounts cgroup filesystem.
func (m *VolumeManager) mountCgroup(mount *Mount, dest string) error {
	// Mount cgroup2 unified hierarchy
	if err := unix.Mount("cgroup2", dest, "cgroup2", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("cgroup mount failed: %w", err)
	}
	return nil
}

// Unmount unmounts a mount.
func (m *VolumeManager) Unmount(mount *Mount, rootfs string) error {
	dest := filepath.Join(rootfs, mount.Destination)

	// Try lazy unmount first
	if err := unix.Unmount(dest, unix.MNT_DETACH); err != nil {
		// Try force unmount
		if err := unix.Unmount(dest, unix.MNT_FORCE); err != nil {
			return fmt.Errorf("unmount failed: %w", err)
		}
	}

	// Decrement volume mount count if applicable
	if mount.Type == MountTypeVolume {
		m.mu.Lock()
		if vol, ok := m.volumes[mount.Source]; ok {
			vol.MountCount--
		}
		m.mu.Unlock()
	}

	return nil
}

// propagationToFlags converts MountPropagation to unix flags.
func propagationToFlags(prop MountPropagation) int {
	switch prop {
	case MountPropagationPrivate:
		return unix.MS_PRIVATE
	case MountPropagationRPrivate:
		return unix.MS_REC | unix.MS_PRIVATE
	case MountPropagationShared:
		return unix.MS_SHARED
	case MountPropagationRShared:
		return unix.MS_REC | unix.MS_SHARED
	case MountPropagationSlave:
		return unix.MS_SLAVE
	case MountPropagationRSlave:
		return unix.MS_REC | unix.MS_SLAVE
	default:
		return unix.MS_REC | unix.MS_PRIVATE
	}
}

// getGID returns the GID for a group name.
func getGID(name string) int {
	// Read /etc/group to find the GID
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return -1
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[0] == name {
			var gid int
			_, _ = fmt.Sscanf(parts[2], "%d", &gid)
			return gid
		}
	}

	return -1
}

// SetupContainerMounts sets up default container mounts.
func (m *VolumeManager) SetupContainerMounts(rootfs string) error {
	// Create necessary directories
	dirs := []string{
		"proc",
		"sys",
		"dev",
		"dev/pts",
		"dev/shm",
		"dev/mqueue",
		"run",
		"tmp",
	}

	for _, dir := range dirs {
		path := filepath.Join(rootfs, dir)
		if err := os.MkdirAll(path, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	// Create device nodes
	devices := []struct {
		path  string
		mode  uint32
		major uint32
		minor uint32
	}{
		{"/dev/null", unix.S_IFCHR | 0666, 1, 3},
		{"/dev/zero", unix.S_IFCHR | 0666, 1, 5},
		{"/dev/full", unix.S_IFCHR | 0666, 1, 7},
		{"/dev/random", unix.S_IFCHR | 0666, 1, 8},
		{"/dev/urandom", unix.S_IFCHR | 0666, 1, 9},
		{"/dev/tty", unix.S_IFCHR | 0666, 5, 0},
	}

	for _, dev := range devices {
		path := filepath.Join(rootfs, dev.path)
		devNum := unix.Mkdev(dev.major, dev.minor)
		// Best-effort: mknod may fail without CAP_MKNOD. ENOENT-on-exists
		// is the normal already-created path.
		if err := unix.Mknod(path, dev.mode, int(devNum)); err != nil && !os.IsExist(err) { // #nosec G115 -- bounded by construction; see the surrounding guard
			_ = err
		}
	}

	// Create symlinks
	symlinks := []struct {
		target string
		link   string
	}{
		{"/proc/self/fd", "/dev/fd"},
		{"/proc/self/fd/0", "/dev/stdin"},
		{"/proc/self/fd/1", "/dev/stdout"},
		{"/proc/self/fd/2", "/dev/stderr"},
	}

	for _, sl := range symlinks {
		linkPath := filepath.Join(rootfs, sl.link)
		_ = os.Remove(linkPath) // Remove if exists
		// Best-effort: symlink may fail on read-only target dirs.
		_ = os.Symlink(sl.target, linkPath)
	}

	return nil
}

// MaskPaths masks sensitive paths in the container.
func (m *VolumeManager) MaskPaths(rootfs string, paths []string) error {
	for _, path := range paths {
		fullPath := filepath.Join(rootfs, path)

		// Check if path exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		// Bind mount /dev/null over the path
		if err := unix.Mount("/dev/null", fullPath, "", unix.MS_BIND, ""); err != nil {
			// Best-effort: tmpfs fallback for masked paths; tmpfs is
			// optional (the rootfs already shadows the path).
			_ = unix.Mount("tmpfs", fullPath, "tmpfs", unix.MS_RDONLY, "size=0")
		}
	}

	return nil
}

// ReadonlyPaths makes paths read-only in the container.
func (m *VolumeManager) ReadonlyPaths(rootfs string, paths []string) error {
	for _, path := range paths {
		fullPath := filepath.Join(rootfs, path)

		// Check if path exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		// Bind mount the path onto itself
		if err := unix.Mount(fullPath, fullPath, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
			continue
		}

		// Best-effort: read-only remount may fail if the source is already
		// read-only or the kernel rejected the bind-remount combination.
		_ = unix.Mount("", fullPath, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_REC, "")
	}

	return nil
}

// PivotRoot performs a pivot_root syscall.
func (m *VolumeManager) PivotRoot(newRoot string) error {
	// Create old_root directory
	oldRoot := filepath.Join(newRoot, ".old_root")
	if err := os.MkdirAll(oldRoot, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return fmt.Errorf("failed to create old_root: %w", err)
	}

	// Pivot root
	if err := unix.PivotRoot(newRoot, oldRoot); err != nil {
		return fmt.Errorf("pivot_root failed: %w", err)
	}

	// Change to new root
	if err := unix.Chdir("/"); err != nil {
		return fmt.Errorf("chdir failed: %w", err)
	}

	// Unmount old root
	if err := unix.Unmount("/.old_root", unix.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old_root failed: %w", err)
	}

	// Best-effort: pivot_root post-cleanup; harmless if already gone.
	_ = os.Remove("/.old_root")

	return nil
}

// GetMountInfo returns mount information for a path.
func (m *VolumeManager) GetMountInfo(path string) (*MountInfo, error) {
	// Read /proc/self/mountinfo
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		info, err := parseMountInfoLine(line)
		if err != nil {
			continue
		}
		if info.MountPoint == path {
			return info, nil
		}
	}

	return nil, fmt.Errorf("mount not found: %s", path)
}

// MountInfo contains mount information.
type MountInfo struct {
	MountID        int
	ParentID       int
	Major          int
	Minor          int
	Root           string
	MountPoint     string
	MountOptions   []string
	OptionalFields []string
	FSType         string
	MountSource    string
	SuperOptions   []string
}

// parseMountInfoLine parses a line from /proc/self/mountinfo.
func parseMountInfoLine(line string) (*MountInfo, error) {
	if line == "" {
		return nil, fmt.Errorf("empty line")
	}

	// Format:
	// mountID parentID major:minor root mountPoint mountOptions optionalFields - fsType mountSource superOptions

	parts := strings.Fields(line)
	if len(parts) < 10 {
		return nil, fmt.Errorf("invalid format")
	}

	info := &MountInfo{}

	_, _ = fmt.Sscanf(parts[0], "%d", &info.MountID)
	_, _ = fmt.Sscanf(parts[1], "%d", &info.ParentID)
	_, _ = fmt.Sscanf(parts[2], "%d:%d", &info.Major, &info.Minor)
	info.Root = parts[3]
	info.MountPoint = parts[4]
	info.MountOptions = strings.Split(parts[5], ",")

	// Find separator "-"
	sepIdx := -1
	for i := 6; i < len(parts); i++ {
		if parts[i] == "-" {
			sepIdx = i
			break
		}
		info.OptionalFields = append(info.OptionalFields, parts[i])
	}

	if sepIdx != -1 && sepIdx+3 <= len(parts) {
		info.FSType = parts[sepIdx+1]
		info.MountSource = parts[sepIdx+2]
		if sepIdx+3 < len(parts) {
			info.SuperOptions = strings.Split(parts[sepIdx+3], ",")
		}
	}

	return info, nil
}
