// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build !linux

package runtime

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// container represents an internal container instance (stub for non-Linux).
type container struct {
	mu sync.RWMutex

	info       *ContainerInfo
	config     *ContainerConfig
	rootfs     string
	bundlePath string
	logPath    string
	pidFile    string
	exitCh     chan struct{}
	exitCode   int

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	cgroupPath string
	nsFDs      map[NamespaceType]int
}

// CreateContainer creates a new container (stub for non-Linux).
func (r *DefaultRuntime) CreateContainer(ctx context.Context, config *ContainerConfig) (*ContainerInfo, error) {
	return nil, fmt.Errorf("container operations require Linux")
}

// StartContainer starts a created container (stub for non-Linux).
func (r *DefaultRuntime) StartContainer(ctx context.Context, containerID string) error {
	return fmt.Errorf("container operations require Linux")
}

// StopContainer stops a running container (stub for non-Linux).
func (r *DefaultRuntime) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	return fmt.Errorf("container operations require Linux")
}

// RemoveContainer removes a container (stub for non-Linux).
func (r *DefaultRuntime) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	return fmt.Errorf("container operations require Linux")
}

// GetContainer returns information about a container (stub for non-Linux).
func (r *DefaultRuntime) GetContainer(ctx context.Context, containerID string) (*ContainerInfo, error) {
	return nil, fmt.Errorf("container operations require Linux")
}

// ListContainers lists containers matching the filter (stub for non-Linux).
func (r *DefaultRuntime) ListContainers(ctx context.Context, filter *ContainerFilter) ([]*ContainerInfo, error) {
	return nil, fmt.Errorf("container operations require Linux")
}

// PauseContainer pauses a running container (stub for non-Linux).
func (r *DefaultRuntime) PauseContainer(ctx context.Context, containerID string) error {
	return fmt.Errorf("container operations require Linux")
}

// ResumeContainer resumes a paused container (stub for non-Linux).
func (r *DefaultRuntime) ResumeContainer(ctx context.Context, containerID string) error {
	return fmt.Errorf("container operations require Linux")
}

// setupMounts sets up container mounts (stub for non-Linux).
func (r *DefaultRuntime) setupMounts(config *ContainerConfig, rootfs string) error {
	return fmt.Errorf("container operations require Linux")
}

// cleanupMounts cleans up container mounts (stub for non-Linux).
func (r *DefaultRuntime) cleanupMounts(config *ContainerConfig, rootfs string) {
}

// parseUserGroup parses a user:group string.
func parseUserGroup(user string) (int, int) {
	uid, gid := 0, 0
	if user == "root" || user == "0" {
		return 0, 0
	}
	fmt.Sscanf(user, "%d:%d", &uid, &gid)
	return uid, gid
}
