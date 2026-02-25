// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// container represents an internal container instance.
type container struct {
	mu sync.RWMutex

	info       *ContainerInfo
	config     *ContainerConfig
	process    *os.Process
	rootfs     string
	bundlePath string
	logPath    string
	pidFile    string
	exitCh     chan struct{}
	exitCode   int
	exitErr    error

	// IO streams
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// Cgroup resources
	cgroupPath string

	// Namespace file descriptors
	nsFDs map[NamespaceType]int
}

// CreateContainer creates a new container.
func (r *DefaultRuntime) CreateContainer(ctx context.Context, config *ContainerConfig) (*ContainerInfo, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	if config.ID == "" {
		config.ID = generateID()
	}

	r.mu.Lock()
	if _, exists := r.containers[config.ID]; exists {
		r.mu.Unlock()
		return nil, ErrContainerExists
	}
	r.mu.Unlock()

	// Validate image exists
	img, err := r.images.GetImage(config.Image)
	if err != nil {
		return nil, fmt.Errorf("image not found: %w", err)
	}

	// Create container directories
	containerRoot := filepath.Join(r.config.Root, "containers", config.ID)
	bundlePath := filepath.Join(containerRoot, "bundle")
	rootfs := filepath.Join(bundlePath, "rootfs")

	if err := os.MkdirAll(rootfs, 0755); err != nil {
		return nil, fmt.Errorf("failed to create container rootfs: %w", err)
	}

	// Extract image layers to rootfs
	if err := r.images.ExtractImage(img.ID, rootfs); err != nil {
		os.RemoveAll(containerRoot)
		return nil, fmt.Errorf("failed to extract image: %w", err)
	}

	// Generate OCI spec
	spec, err := r.generateOCISpec(config, rootfs)
	if err != nil {
		os.RemoveAll(containerRoot)
		return nil, fmt.Errorf("failed to generate OCI spec: %w", err)
	}

	specPath := filepath.Join(bundlePath, "config.json")
	specData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		os.RemoveAll(containerRoot)
		return nil, fmt.Errorf("failed to marshal OCI spec: %w", err)
	}

	if err := os.WriteFile(specPath, specData, 0644); err != nil {
		os.RemoveAll(containerRoot)
		return nil, fmt.Errorf("failed to write OCI spec: %w", err)
	}

	// Setup cgroup
	cgroupPath := ""
	if config.Resources != nil {
		cgroupPath, err = r.cgroups.CreateCgroup(config.ID, config.Resources)
		if err != nil {
			os.RemoveAll(containerRoot)
			return nil, fmt.Errorf("failed to create cgroup: %w", err)
		}
	}

	// Setup mounts
	if err := r.setupMounts(config, rootfs); err != nil {
		if cgroupPath != "" {
			r.cgroups.RemoveCgroup(cgroupPath)
		}
		os.RemoveAll(containerRoot)
		return nil, fmt.Errorf("failed to setup mounts: %w", err)
	}

	now := time.Now()
	info := &ContainerInfo{
		ID:          config.ID,
		Name:        config.Name,
		Image:       config.Image,
		State:       ContainerStateCreated,
		CreatedAt:   now,
		SandboxID:   config.SandboxID,
		Labels:      config.Labels,
		Annotations: config.Annotations,
	}

	c := &container{
		info:       info,
		config:     config,
		rootfs:     rootfs,
		bundlePath: bundlePath,
		logPath:    filepath.Join(containerRoot, "container.log"),
		pidFile:    filepath.Join(containerRoot, "container.pid"),
		cgroupPath: cgroupPath,
		exitCh:     make(chan struct{}),
		nsFDs:      make(map[NamespaceType]int),
	}

	r.mu.Lock()
	r.containers[config.ID] = c
	r.mu.Unlock()

	return info, nil
}

// StartContainer starts a created container.
func (r *DefaultRuntime) StartContainer(ctx context.Context, containerID string) error {
	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return ErrContainerNotFound
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.info.State == ContainerStateRunning {
		return nil // Already running
	}

	if c.info.State != ContainerStateCreated && c.info.State != ContainerStateStopped {
		return fmt.Errorf("cannot start container in state %s", c.info.State)
	}

	// Build the command
	cmd := c.config.Command
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}
	if len(c.config.Args) > 0 {
		cmd = append(cmd, c.config.Args...)
	}

	// Create process
	proc := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	proc.Dir = c.rootfs
	proc.Env = c.config.Env

	// Setup process attributes for namespaces
	proc.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC,
		Unshareflags: syscall.CLONE_NEWNS,
	}

	// If joining a sandbox, use its namespaces
	if c.config.SandboxID != "" {
		r.mu.RLock()
		sandbox, sandboxExists := r.sandboxes[c.config.SandboxID]
		r.mu.RUnlock()

		if sandboxExists {
			// Share network namespace with sandbox
			if sandbox.NetNS != "" {
				proc.SysProcAttr.Cloneflags &^= syscall.CLONE_NEWNET
			}
		}
	}

	// Setup user if specified
	if c.config.User != "" {
		// Parse user:group format
		uid, gid := parseUserGroup(c.config.User)
		proc.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
	}

	// Setup stdio
	logFile, err := os.OpenFile(c.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	if c.config.Stdin {
		stdin, err := proc.StdinPipe()
		if err != nil {
			logFile.Close()
			return fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		c.stdin = stdin
	}

	stdout, err := proc.StdoutPipe()
	if err != nil {
		logFile.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	c.stdout = stdout

	stderr, err := proc.StderrPipe()
	if err != nil {
		logFile.Close()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	c.stderr = stderr

	// Start copying output to log file
	go func() {
		io.Copy(logFile, stdout)
	}()
	go func() {
		io.Copy(logFile, stderr)
		logFile.Close()
	}()

	// Start the process
	if err := proc.Start(); err != nil {
		return fmt.Errorf("failed to start container process: %w", err)
	}

	c.process = proc.Process
	c.info.PID = proc.Process.Pid
	c.info.State = ContainerStateRunning
	c.info.StartedAt = time.Now()

	// Write PID file
	if err := os.WriteFile(c.pidFile, []byte(fmt.Sprintf("%d", proc.Process.Pid)), 0644); err != nil {
		// Non-fatal error
	}

	// Add process to cgroup
	if c.cgroupPath != "" {
		if err := r.cgroups.AddProcess(c.cgroupPath, proc.Process.Pid); err != nil {
			// Log but don't fail
		}
	}

	// Wait for process in background
	go func() {
		err := proc.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()

		c.info.State = ContainerStateExited
		c.info.FinishedAt = time.Now()

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				c.info.ExitCode = exitErr.ExitCode()
			} else {
				c.info.ExitCode = -1
			}
			c.exitErr = err
		} else {
			c.info.ExitCode = 0
		}
		c.exitCode = c.info.ExitCode

		close(c.exitCh)
	}()

	return nil
}

// StopContainer stops a running container.
func (r *DefaultRuntime) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return ErrContainerNotFound
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.info.State != ContainerStateRunning {
		return ErrContainerNotRunning
	}

	if c.process == nil {
		c.info.State = ContainerStateStopped
		return nil
	}

	// Send SIGTERM first
	if err := c.process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		if err == os.ErrProcessDone {
			c.info.State = ContainerStateStopped
			return nil
		}
	}

	// Wait for graceful shutdown or timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-c.exitCh:
		c.info.State = ContainerStateStopped
		return nil
	case <-timer.C:
		// Forcefully kill
		if err := c.process.Kill(); err != nil {
			if err != os.ErrProcessDone {
				return fmt.Errorf("failed to kill container: %w", err)
			}
		}
		c.info.State = ContainerStateStopped
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RemoveContainer removes a container.
func (r *DefaultRuntime) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	r.mu.Lock()
	c, exists := r.containers[containerID]
	if !exists {
		r.mu.Unlock()
		return ErrContainerNotFound
	}
	r.mu.Unlock()

	c.mu.Lock()
	state := c.info.State
	c.mu.Unlock()

	if state == ContainerStateRunning {
		if !force {
			return ErrContainerRunning
		}
		// Force stop
		if err := r.StopContainer(ctx, containerID, 0); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
	}

	// Remove cgroup
	if c.cgroupPath != "" {
		r.cgroups.RemoveCgroup(c.cgroupPath)
	}

	// Cleanup mounts
	r.cleanupMounts(c.config, c.rootfs)

	// Remove container directory
	containerRoot := filepath.Join(r.config.Root, "containers", containerID)
	os.RemoveAll(containerRoot)

	// Remove from map
	r.mu.Lock()
	delete(r.containers, containerID)
	r.mu.Unlock()

	return nil
}

// GetContainer returns information about a container.
func (r *DefaultRuntime) GetContainer(ctx context.Context, containerID string) (*ContainerInfo, error) {
	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrContainerNotFound
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	info := *c.info
	return &info, nil
}

// ListContainers lists containers matching the filter.
func (r *DefaultRuntime) ListContainers(ctx context.Context, filter *ContainerFilter) ([]*ContainerInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ContainerInfo

	for _, c := range r.containers {
		c.mu.RLock()
		info := c.info
		c.mu.RUnlock()

		if filter != nil {
			if filter.ID != "" && info.ID != filter.ID {
				continue
			}
			if filter.Name != "" && info.Name != filter.Name {
				continue
			}
			if filter.State != ContainerStateUnknown && info.State != filter.State {
				continue
			}
			if filter.SandboxID != "" && info.SandboxID != filter.SandboxID {
				continue
			}
			if len(filter.Labels) > 0 {
				match := true
				for k, v := range filter.Labels {
					if info.Labels[k] != v {
						match = false
						break
					}
				}
				if !match {
					continue
				}
			}
		}

		// Return a copy
		infoCopy := *info
		result = append(result, &infoCopy)
	}

	return result, nil
}

// PauseContainer pauses a running container.
func (r *DefaultRuntime) PauseContainer(ctx context.Context, containerID string) error {
	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return ErrContainerNotFound
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.info.State != ContainerStateRunning {
		return ErrContainerNotRunning
	}

	if c.cgroupPath != "" {
		if err := r.cgroups.Freeze(c.cgroupPath); err != nil {
			return fmt.Errorf("failed to freeze cgroup: %w", err)
		}
	}

	c.info.State = ContainerStatePaused
	return nil
}

// ResumeContainer resumes a paused container.
func (r *DefaultRuntime) ResumeContainer(ctx context.Context, containerID string) error {
	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return ErrContainerNotFound
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.info.State != ContainerStatePaused {
		return fmt.Errorf("container is not paused")
	}

	if c.cgroupPath != "" {
		if err := r.cgroups.Thaw(c.cgroupPath); err != nil {
			return fmt.Errorf("failed to thaw cgroup: %w", err)
		}
	}

	c.info.State = ContainerStateRunning
	return nil
}

// setupMounts sets up container mounts.
func (r *DefaultRuntime) setupMounts(config *ContainerConfig, rootfs string) error {
	for _, mount := range config.Mounts {
		if err := r.volumes.Mount(&mount, rootfs); err != nil {
			return fmt.Errorf("failed to mount %s: %w", mount.Destination, err)
		}
	}
	return nil
}

// cleanupMounts cleans up container mounts.
func (r *DefaultRuntime) cleanupMounts(config *ContainerConfig, rootfs string) {
	for i := len(config.Mounts) - 1; i >= 0; i-- {
		r.volumes.Unmount(&config.Mounts[i], rootfs)
	}
}

// parseUserGroup parses a user:group string.
func parseUserGroup(user string) (int, int) {
	// Default to root
	uid, gid := 0, 0

	// Simple parsing - in production would look up /etc/passwd
	if user == "root" || user == "0" {
		return 0, 0
	}

	// Try to parse as uid:gid
	_, err := fmt.Sscanf(user, "%d:%d", &uid, &gid)
	if err != nil {
		// Try just uid
		fmt.Sscanf(user, "%d", &uid)
		gid = uid
	}

	return uid, gid
}
