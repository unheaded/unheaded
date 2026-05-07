// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// MountType represents the type of mount.
type MountType string

const (
	MountTypeBind   MountType = "bind"
	MountTypeTmpfs  MountType = "tmpfs"
	MountTypeVolume MountType = "volume"
	MountTypeNFS    MountType = "nfs"
	MountTypeProc   MountType = "proc"
	MountTypeSysfs  MountType = "sysfs"
	MountTypeDevpts MountType = "devpts"
	MountTypeCgroup MountType = "cgroup"
	MountTypeMqueue MountType = "mqueue"
)

// Mount represents a mount configuration.
type Mount struct {
	Type        MountType
	Source      string
	Destination string
	Options     []string
	ReadOnly    bool
	Propagation MountPropagation
}

// MountPropagation represents mount propagation modes.
type MountPropagation string

const (
	MountPropagationPrivate  MountPropagation = "private"
	MountPropagationRPrivate MountPropagation = "rprivate"
	MountPropagationShared   MountPropagation = "shared"
	MountPropagationRShared  MountPropagation = "rshared"
	MountPropagationSlave    MountPropagation = "slave"
	MountPropagationRSlave   MountPropagation = "rslave"
)

// VolumeManager manages volumes and mounts (stub for non-Linux).
type VolumeManager struct {
	mu      sync.RWMutex
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

// NewVolumeManager creates a new volume manager.
func NewVolumeManager(root string) (*VolumeManager, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("failed to create volume root: %w", err)
	}

	mgr := &VolumeManager{
		root:    root,
		volumes: make(map[string]*Volume),
	}

	return mgr, nil
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

	if err := os.MkdirAll(dataPath, 0755); err != nil {
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

// Mount mounts a mount configuration (stub for non-Linux).
func (m *VolumeManager) Mount(mount *Mount, rootfs string) error {
	return fmt.Errorf("mounting requires Linux")
}

// Unmount unmounts a mount (stub for non-Linux).
func (m *VolumeManager) Unmount(mount *Mount, rootfs string) error {
	return fmt.Errorf("unmounting requires Linux")
}

// SetupContainerMounts sets up default container mounts (stub for non-Linux).
func (m *VolumeManager) SetupContainerMounts(rootfs string) error {
	return fmt.Errorf("container mounts require Linux")
}

// MaskPaths masks sensitive paths (stub for non-Linux).
func (m *VolumeManager) MaskPaths(rootfs string, paths []string) error {
	return fmt.Errorf("masking paths requires Linux")
}

// ReadonlyPaths makes paths read-only (stub for non-Linux).
func (m *VolumeManager) ReadonlyPaths(rootfs string, paths []string) error {
	return fmt.Errorf("readonly paths require Linux")
}

// PivotRoot performs pivot_root (stub for non-Linux).
func (m *VolumeManager) PivotRoot(newRoot string) error {
	return fmt.Errorf("pivot_root requires Linux")
}

// GetMountInfo returns mount information (stub for non-Linux).
func (m *VolumeManager) GetMountInfo(path string) (*MountInfo, error) {
	return nil, fmt.Errorf("mount info requires Linux")
}

// parseMountInfoLine parses a mount info line (stub for non-Linux).
func parseMountInfoLine(line string) (*MountInfo, error) {
	if line == "" {
		return nil, fmt.Errorf("empty line")
	}

	parts := strings.Fields(line)
	if len(parts) < 10 {
		return nil, fmt.Errorf("invalid format")
	}

	info := &MountInfo{}
	fmt.Sscanf(parts[0], "%d", &info.MountID)
	fmt.Sscanf(parts[1], "%d", &info.ParentID)
	fmt.Sscanf(parts[2], "%d:%d", &info.Major, &info.Minor)
	info.Root = parts[3]
	info.MountPoint = parts[4]
	info.MountOptions = strings.Split(parts[5], ",")

	return info, nil
}
