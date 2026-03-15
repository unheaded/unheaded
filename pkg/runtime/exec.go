// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ExecConfig defines the configuration for exec.
type ExecConfig struct {
	// Command is the command to execute.
	Command []string
	// Env are environment variables.
	Env []string
	// WorkingDir is the working directory.
	WorkingDir string
	// User is the user to run as.
	User string
	// Privileged runs the exec as privileged.
	Privileged bool
	// Tty allocates a pseudo-TTY.
	Tty bool
	// Stdin enables stdin.
	Stdin bool
	// AttachStdin attaches stdin.
	AttachStdin bool
	// AttachStdout attaches stdout.
	AttachStdout bool
	// AttachStderr attaches stderr.
	AttachStderr bool
	// DetachKeys are the keys to detach.
	DetachKeys string
}

// ExecResult contains the result of an exec.
type ExecResult struct {
	// ID is the exec ID.
	ID string
	// ExitCode is the exit code.
	ExitCode int
	// Running indicates if the exec is still running.
	Running bool
	// Pid is the process ID.
	Pid int
}

// IOStreams contains IO streams for attach.
type IOStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// execSession represents an exec session.
type execSession struct {
	mu sync.RWMutex

	id          string
	containerID string
	config      *ExecConfig
	cmd         *exec.Cmd
	process     *os.Process
	running     bool
	exitCode    int
	exitCh      chan struct{}

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

// ExecInContainer executes a command in a container.
func (r *DefaultRuntime) ExecInContainer(ctx context.Context, containerID string, config *ExecConfig) (*ExecResult, error) {
	if config == nil || len(config.Command) == 0 {
		return nil, ErrInvalidConfig
	}

	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrContainerNotFound
	}

	c.mu.RLock()
	state := c.info.State
	rootfs := c.rootfs
	pid := c.info.PID
	containerEnv := c.config.Env
	containerWorkDir := c.config.WorkingDir
	containerUser := c.config.User
	c.mu.RUnlock()

	if state != ContainerStateRunning {
		return nil, ErrContainerNotRunning
	}

	// Generate exec ID
	execID := fmt.Sprintf("exec-%s-%d", containerID[:8], time.Now().UnixNano())

	// Build environment
	env := append([]string{}, containerEnv...)
	env = append(env, config.Env...)

	// Determine working directory
	workDir := config.WorkingDir
	if workDir == "" {
		workDir = containerWorkDir
		if workDir == "" {
			workDir = "/"
		}
	}

	// Build command
	cmd := exec.CommandContext(ctx, config.Command[0], config.Command[1:]...)
	cmd.Dir = rootfs + workDir
	cmd.Env = env

	// Setup user
	user := config.User
	if user == "" {
		user = containerUser
	}
	if user != "" {
		uid, gid := parseUserGroup(user)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
	}

	// Setup namespace entry
	if pid > 0 {
		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		// Enter the container's namespaces via nsenter
		cmd = r.wrapWithNsenter(ctx, pid, config.Command, env, workDir, user)
		cmd.Dir = ""
	}

	session := &execSession{
		id:          execID,
		containerID: containerID,
		config:      config,
		cmd:         cmd,
		exitCh:      make(chan struct{}),
	}

	// Setup pipes
	if config.AttachStdin || config.Stdin {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		session.stdin = stdin
	}

	if config.AttachStdout {
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
		}
		session.stdout = stdout
	}

	if config.AttachStderr {
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
		}
		session.stderr = stderr
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start exec: %w", err)
	}

	session.process = cmd.Process
	session.running = true

	// Store session
	r.mu.Lock()
	r.execSessions[execID] = session
	r.mu.Unlock()

	// Wait in background
	go func() {
		err := cmd.Wait()

		session.mu.Lock()
		session.running = false
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				session.exitCode = exitErr.ExitCode()
			} else {
				session.exitCode = -1
			}
		}
		close(session.exitCh)
		session.mu.Unlock()

		// Clean up after a delay
		time.AfterFunc(5*time.Minute, func() {
			r.mu.Lock()
			delete(r.execSessions, execID)
			r.mu.Unlock()
		})
	}()

	return &ExecResult{
		ID:       execID,
		Running:  true,
		Pid:      cmd.Process.Pid,
		ExitCode: 0,
	}, nil
}

// wrapWithNsenter wraps a command with nsenter to enter container namespaces.
func (r *DefaultRuntime) wrapWithNsenter(ctx context.Context, pid int, command []string, env []string, workDir, user string) *exec.Cmd {
	args := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount",
		"--uts",
		"--ipc",
		"--net",
		"--pid",
	}

	if user != "" {
		args = append(args, "--setuid", user)
	}

	args = append(args, "--")
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "nsenter", args...)
	cmd.Env = env

	return cmd
}

// ExecAttach attaches to an exec session.
func (r *DefaultRuntime) ExecAttach(ctx context.Context, execID string, streams *IOStreams) error {
	r.mu.RLock()
	session, exists := r.execSessions[execID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("exec session not found: %s", execID)
	}

	session.mu.RLock()
	running := session.running
	stdin := session.stdin
	stdout := session.stdout
	stderr := session.stderr
	exitCh := session.exitCh
	session.mu.RUnlock()

	if !running {
		return fmt.Errorf("exec session is not running")
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	// Copy stdin
	if streams.Stdin != nil && stdin != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := io.Copy(stdin, streams.Stdin)
			if err != nil {
				errCh <- fmt.Errorf("stdin copy error: %w", err)
			}
			stdin.Close()
		}()
	}

	// Copy stdout
	if streams.Stdout != nil && stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := io.Copy(streams.Stdout, stdout)
			if err != nil {
				errCh <- fmt.Errorf("stdout copy error: %w", err)
			}
		}()
	}

	// Copy stderr
	if streams.Stderr != nil && stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := io.Copy(streams.Stderr, stderr)
			if err != nil {
				errCh <- fmt.Errorf("stderr copy error: %w", err)
			}
		}()
	}

	// Wait for exit or context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-exitCh:
		// Wait for IO to complete
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// Timeout waiting for IO
		}
	}

	// Check for errors
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

// GetExecSession returns information about an exec session.
func (r *DefaultRuntime) GetExecSession(execID string) (*ExecResult, error) {
	r.mu.RLock()
	session, exists := r.execSessions[execID]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("exec session not found: %s", execID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	result := &ExecResult{
		ID:       session.id,
		Running:  session.running,
		ExitCode: session.exitCode,
	}

	if session.process != nil {
		result.Pid = session.process.Pid
	}

	return result, nil
}

// ResizeExecTTY resizes the TTY for an exec session.
func (r *DefaultRuntime) ResizeExecTTY(execID string, height, width uint32) error {
	r.mu.RLock()
	session, exists := r.execSessions[execID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("exec session not found: %s", execID)
	}

	session.mu.RLock()
	running := session.running
	tty := session.config.Tty
	session.mu.RUnlock()

	if !running {
		return fmt.Errorf("exec session is not running")
	}

	if !tty {
		return fmt.Errorf("exec session does not have a TTY")
	}

	// In a real implementation, we would resize the PTY here
	// This requires platform-specific code using ioctl TIOCSWINSZ
	return nil
}

// execInspect contains detailed information about an exec session.
type execInspect struct {
	ID          string
	Running     bool
	ExitCode    int
	ProcessConfig *execProcessConfig
	OpenStdin   bool
	OpenStderr  bool
	OpenStdout  bool
	CanRemove   bool
	ContainerID string
	DetachKeys  string
	Pid         int
}

// execProcessConfig contains process configuration for an exec.
type execProcessConfig struct {
	Tty        bool
	Entrypoint string
	Arguments  []string
	Privileged bool
	User       string
}

// InspectExec returns detailed information about an exec session.
func (r *DefaultRuntime) InspectExec(execID string) (*execInspect, error) {
	r.mu.RLock()
	session, exists := r.execSessions[execID]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("exec session not found: %s", execID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	inspect := &execInspect{
		ID:          session.id,
		Running:     session.running,
		ExitCode:    session.exitCode,
		ContainerID: session.containerID,
		DetachKeys:  session.config.DetachKeys,
		OpenStdin:   session.config.AttachStdin,
		OpenStdout:  session.config.AttachStdout,
		OpenStderr:  session.config.AttachStderr,
		CanRemove:   !session.running,
	}

	if session.process != nil {
		inspect.Pid = session.process.Pid
	}

	if len(session.config.Command) > 0 {
		inspect.ProcessConfig = &execProcessConfig{
			Tty:        session.config.Tty,
			Entrypoint: session.config.Command[0],
			Arguments:  session.config.Command[1:],
			Privileged: session.config.Privileged,
			User:       session.config.User,
		}
	}

	return inspect, nil
}

// StartExec starts a created exec instance (for detached exec).
func (r *DefaultRuntime) StartExec(ctx context.Context, execID string, tty bool) error {
	r.mu.RLock()
	session, exists := r.execSessions[execID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("exec session not found: %s", execID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.running {
		return nil // Already running
	}

	if session.cmd == nil {
		return fmt.Errorf("exec command not configured")
	}

	// Start the process
	if err := session.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start exec: %w", err)
	}

	session.process = session.cmd.Process
	session.running = true

	// Wait in background
	go func() {
		err := session.cmd.Wait()

		session.mu.Lock()
		session.running = false
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				session.exitCode = exitErr.ExitCode()
			} else {
				session.exitCode = -1
			}
		}
		close(session.exitCh)
		session.mu.Unlock()
	}()

	return nil
}

// RemoveExec removes an exec session.
func (r *DefaultRuntime) RemoveExec(execID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.execSessions[execID]
	if !exists {
		return fmt.Errorf("exec session not found: %s", execID)
	}

	session.mu.RLock()
	running := session.running
	session.mu.RUnlock()

	if running {
		return fmt.Errorf("cannot remove running exec session")
	}

	delete(r.execSessions, execID)
	return nil
}

// ContainerExecCreate creates a new exec instance without starting it.
func (r *DefaultRuntime) ContainerExecCreate(ctx context.Context, containerID string, config *ExecConfig) (string, error) {
	if config == nil || len(config.Command) == 0 {
		return "", ErrInvalidConfig
	}

	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return "", ErrContainerNotFound
	}

	c.mu.RLock()
	state := c.info.State
	rootfs := c.rootfs
	pid := c.info.PID
	containerEnv := c.config.Env
	containerWorkDir := c.config.WorkingDir
	containerUser := c.config.User
	c.mu.RUnlock()

	if state != ContainerStateRunning {
		return "", ErrContainerNotRunning
	}

	// Generate exec ID
	execID := fmt.Sprintf("exec-%s-%d", containerID[:8], time.Now().UnixNano())

	// Build environment
	env := append([]string{}, containerEnv...)
	env = append(env, config.Env...)

	// Determine working directory
	workDir := config.WorkingDir
	if workDir == "" {
		workDir = containerWorkDir
		if workDir == "" {
			workDir = "/"
		}
	}

	// Build command
	var cmd *exec.Cmd
	if pid > 0 {
		cmd = r.wrapWithNsenter(ctx, pid, config.Command, env, workDir, config.User)
	} else {
		cmd = exec.CommandContext(ctx, config.Command[0], config.Command[1:]...)
		cmd.Dir = rootfs + workDir
		cmd.Env = env

		user := config.User
		if user == "" {
			user = containerUser
		}
		if user != "" {
			uid, gid := parseUserGroup(user)
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Credential: &syscall.Credential{
					Uid: uint32(uid),
					Gid: uint32(gid),
				},
			}
		}
	}

	session := &execSession{
		id:          execID,
		containerID: containerID,
		config:      config,
		cmd:         cmd,
		exitCh:      make(chan struct{}),
	}

	// Setup pipes if needed
	if config.AttachStdin || config.Stdin {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		session.stdin = stdin
	}

	if config.AttachStdout {
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return "", fmt.Errorf("failed to create stdout pipe: %w", err)
		}
		session.stdout = stdout
	}

	if config.AttachStderr {
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return "", fmt.Errorf("failed to create stderr pipe: %w", err)
		}
		session.stderr = stderr
	}

	// Store session
	r.mu.Lock()
	r.execSessions[execID] = session
	r.mu.Unlock()

	return execID, nil
}
