// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build !linux

package runtime

import (
	"fmt"
	"sync"
)

// NamespaceManager manages Linux namespaces (stub for non-Linux).
type NamespaceManager struct {
	mu         sync.RWMutex
	nsBasePath string
	namespaces map[string]*NamespaceSet
}

// NamespaceSet represents a set of namespaces.
type NamespaceSet struct {
	ID    string
	PIDs  map[NamespaceType]int
	Paths map[NamespaceType]string
}

// NamespaceInfo contains information about a namespace.
type NamespaceInfo struct {
	Type  NamespaceType
	Path  string
	Inode uint64
}

// IDMapping represents a user/group ID mapping.
type IDMapping struct {
	ContainerID int
	HostID      int
	Size        int
}

// NewNamespaceManager creates a new namespace manager (stub for non-Linux).
func NewNamespaceManager() (*NamespaceManager, error) {
	return &NamespaceManager{
		namespaces: make(map[string]*NamespaceSet),
	}, nil
}

// CreateNamespaces creates namespaces (stub for non-Linux).
func (m *NamespaceManager) CreateNamespaces(id string, types []NamespaceType) (*NamespaceSet, error) {
	return nil, fmt.Errorf("namespaces require Linux")
}

// BindNamespaces binds namespaces (stub for non-Linux).
func (m *NamespaceManager) BindNamespaces(nsSet *NamespaceSet, pid int) error {
	return fmt.Errorf("namespaces require Linux")
}

// GetNamespace returns namespace path (stub for non-Linux).
func (m *NamespaceManager) GetNamespace(id string, nsType NamespaceType) (string, error) {
	return "", fmt.Errorf("namespaces require Linux")
}

// RemoveNamespaces removes namespaces (stub for non-Linux).
func (m *NamespaceManager) RemoveNamespaces(id string) error {
	return fmt.Errorf("namespaces require Linux")
}

// GetNamespaceInfo returns namespace info (stub for non-Linux).
func (m *NamespaceManager) GetNamespaceInfo(path string) (*NamespaceInfo, error) {
	return nil, fmt.Errorf("namespaces require Linux")
}

// EnterNamespace enters a namespace (stub for non-Linux).
func (m *NamespaceManager) EnterNamespace(path string, nsType NamespaceType) error {
	return fmt.Errorf("namespaces require Linux")
}

// CreateNetworkNamespace creates network namespace (stub for non-Linux).
func (m *NamespaceManager) CreateNetworkNamespace(id string) (string, error) {
	return "", fmt.Errorf("namespaces require Linux")
}

// ConfigureNetworkNamespace configures network namespace (stub for non-Linux).
func (m *NamespaceManager) ConfigureNetworkNamespace(nsPath string) error {
	return fmt.Errorf("namespaces require Linux")
}

// JoinNamespace joins a namespace (stub for non-Linux).
func (m *NamespaceManager) JoinNamespace(pid int, nsType NamespaceType, nsPath string) error {
	return fmt.Errorf("namespaces require Linux")
}

// GetProcessNamespaces returns process namespaces (stub for non-Linux).
func (m *NamespaceManager) GetProcessNamespaces(pid int) (map[NamespaceType]*NamespaceInfo, error) {
	return nil, fmt.Errorf("namespaces require Linux")
}

// IsNamespaceSame checks if namespaces are same (stub for non-Linux).
func (m *NamespaceManager) IsNamespaceSame(path1, path2 string) (bool, error) {
	return false, fmt.Errorf("namespaces require Linux")
}

// CreateUserNamespace creates user namespace (stub for non-Linux).
func (m *NamespaceManager) CreateUserNamespace(id string, uidMappings, gidMappings []IDMapping) (string, error) {
	return "", fmt.Errorf("namespaces require Linux")
}

// SetHostname sets hostname in UTS namespace (stub for non-Linux).
func (m *NamespaceManager) SetHostname(nsPath, hostname string) error {
	return fmt.Errorf("namespaces require Linux")
}
