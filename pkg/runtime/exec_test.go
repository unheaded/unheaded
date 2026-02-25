// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// ExecConfig Tests
// =============================================================================

func TestExecConfig_Fields(t *testing.T) {
	config := &ExecConfig{
		Command:      []string{"/bin/sh", "-c", "echo hello"},
		Env:          []string{"FOO=bar", "BAZ=qux"},
		WorkingDir:   "/app",
		User:         "1000:1000",
		Privileged:   false,
		Tty:          true,
		Stdin:        true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		DetachKeys:   "ctrl-p,ctrl-q",
	}

	if len(config.Command) != 3 {
		t.Errorf("Expected 3 command parts, got %d", len(config.Command))
	}

	if config.Command[0] != "/bin/sh" {
		t.Errorf("Expected command '/bin/sh', got '%s'", config.Command[0])
	}

	if len(config.Env) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(config.Env))
	}

	if config.WorkingDir != "/app" {
		t.Errorf("Expected working dir '/app', got '%s'", config.WorkingDir)
	}

	if !config.Tty {
		t.Error("Expected TTY to be enabled")
	}

	if config.DetachKeys != "ctrl-p,ctrl-q" {
		t.Errorf("Expected detach keys 'ctrl-p,ctrl-q', got '%s'", config.DetachKeys)
	}
}

// =============================================================================
// ExecResult Tests
// =============================================================================

func TestExecResult_Fields(t *testing.T) {
	result := &ExecResult{
		ID:       "exec-123",
		ExitCode: 0,
		Running:  true,
		Pid:      12345,
	}

	if result.ID != "exec-123" {
		t.Errorf("Expected ID 'exec-123', got '%s'", result.ID)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	if !result.Running {
		t.Error("Expected running to be true")
	}

	if result.Pid != 12345 {
		t.Errorf("Expected PID 12345, got %d", result.Pid)
	}
}

// =============================================================================
// IOStreams Tests
// =============================================================================

func TestIOStreams(t *testing.T) {
	stdin := bytes.NewBufferString("input data")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	streams := &IOStreams{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	if streams.Stdin == nil {
		t.Error("Expected stdin to be set")
	}

	if streams.Stdout == nil {
		t.Error("Expected stdout to be set")
	}

	if streams.Stderr == nil {
		t.Error("Expected stderr to be set")
	}

	// Test reading from stdin
	data := make([]byte, 10)
	n, _ := streams.Stdin.Read(data)
	if n == 0 {
		t.Error("Expected to read from stdin")
	}

	// Test writing to stdout
	streams.Stdout.Write([]byte("output"))
	if stdout.String() != "output" {
		t.Errorf("Expected stdout 'output', got '%s'", stdout.String())
	}
}

// =============================================================================
// ExecInContainer Tests
// =============================================================================

func TestExecInContainer_NilConfig(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.ExecInContainer(ctx, "any-container", nil)
	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

func TestExecInContainer_EmptyCommand(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &ExecConfig{
		Command: []string{},
	}

	_, err := rt.ExecInContainer(ctx, "any-container", config)
	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

func TestExecInContainer_ContainerNotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &ExecConfig{
		Command: []string{"/bin/echo", "hello"},
	}

	_, err := rt.ExecInContainer(ctx, "nonexistent", config)
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

func TestExecInContainer_ContainerNotRunning(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges")
	}

	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	// Create mock image and container
	addMockImage(t, rt.images, "sha256:test", "test:latest")

	containerConfig := &ContainerConfig{
		ID:    "test-exec",
		Name:  "test",
		Image: "test:latest",
	}

	_, err := rt.CreateContainer(ctx, containerConfig)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	// Try to exec in non-running container
	execConfig := &ExecConfig{
		Command: []string{"/bin/echo", "hello"},
	}

	_, err = rt.ExecInContainer(ctx, "test-exec", execConfig)
	if err != ErrContainerNotRunning {
		t.Errorf("Expected ErrContainerNotRunning, got %v", err)
	}
}

// =============================================================================
// ExecAttach Tests
// =============================================================================

func TestExecAttach_SessionNotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()
	streams := &IOStreams{}

	err := rt.ExecAttach(ctx, "nonexistent-exec", streams)
	if err == nil {
		t.Error("Expected error for nonexistent exec session")
	}
}

// =============================================================================
// GetExecSession Tests
// =============================================================================

func TestGetExecSession_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	_, err := rt.GetExecSession("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent exec session")
	}
}

// =============================================================================
// ResizeExecTTY Tests
// =============================================================================

func TestResizeExecTTY_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	err := rt.ResizeExecTTY("nonexistent", 24, 80)
	if err == nil {
		t.Error("Expected error for nonexistent exec session")
	}
}

// =============================================================================
// InspectExec Tests
// =============================================================================

func TestInspectExec_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	_, err := rt.InspectExec("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent exec session")
	}
}

func TestExecInspect_Fields(t *testing.T) {
	inspect := &execInspect{
		ID:          "exec-123",
		Running:     true,
		ExitCode:    0,
		OpenStdin:   true,
		OpenStdout:  true,
		OpenStderr:  true,
		CanRemove:   false,
		ContainerID: "container-456",
		DetachKeys:  "ctrl-p,ctrl-q",
		Pid:         12345,
		ProcessConfig: &execProcessConfig{
			Tty:        true,
			Entrypoint: "/bin/sh",
			Arguments:  []string{"-c", "echo hello"},
			Privileged: false,
			User:       "root",
		},
	}

	if inspect.ID != "exec-123" {
		t.Errorf("Expected ID 'exec-123', got '%s'", inspect.ID)
	}

	if !inspect.Running {
		t.Error("Expected running to be true")
	}

	if inspect.ProcessConfig.Entrypoint != "/bin/sh" {
		t.Errorf("Expected entrypoint '/bin/sh', got '%s'", inspect.ProcessConfig.Entrypoint)
	}
}

// =============================================================================
// StartExec Tests
// =============================================================================

func TestStartExec_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	err := rt.StartExec(ctx, "nonexistent", false)
	if err == nil {
		t.Error("Expected error for nonexistent exec session")
	}
}

// =============================================================================
// RemoveExec Tests
// =============================================================================

func TestRemoveExec_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	err := rt.RemoveExec("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent exec session")
	}
}

// =============================================================================
// ContainerExecCreate Tests
// =============================================================================

func TestContainerExecCreate_NilConfig(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.ContainerExecCreate(ctx, "any-container", nil)
	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

func TestContainerExecCreate_EmptyCommand(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &ExecConfig{
		Command: []string{},
	}

	_, err := rt.ContainerExecCreate(ctx, "any-container", config)
	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

func TestContainerExecCreate_ContainerNotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	config := &ExecConfig{
		Command: []string{"/bin/echo", "hello"},
	}

	_, err := rt.ContainerExecCreate(ctx, "nonexistent", config)
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// =============================================================================
// execSession Tests
// =============================================================================

func TestExecSession_Fields(t *testing.T) {
	session := &execSession{
		id:          "exec-123",
		containerID: "container-456",
		config: &ExecConfig{
			Command: []string{"/bin/sh"},
			Tty:     true,
		},
		running:  true,
		exitCode: 0,
		exitCh:   make(chan struct{}),
	}

	if session.id != "exec-123" {
		t.Errorf("Expected ID 'exec-123', got '%s'", session.id)
	}

	if session.containerID != "container-456" {
		t.Errorf("Expected container ID 'container-456', got '%s'", session.containerID)
	}

	if !session.running {
		t.Error("Expected running to be true")
	}

	if session.exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", session.exitCode)
	}
}

// =============================================================================
// execProcessConfig Tests
// =============================================================================

func TestExecProcessConfig(t *testing.T) {
	config := &execProcessConfig{
		Tty:        true,
		Entrypoint: "/usr/bin/python",
		Arguments:  []string{"-c", "print('hello')"},
		Privileged: false,
		User:       "1000",
	}

	if !config.Tty {
		t.Error("Expected TTY to be enabled")
	}

	if config.Entrypoint != "/usr/bin/python" {
		t.Errorf("Expected entrypoint '/usr/bin/python', got '%s'", config.Entrypoint)
	}

	if len(config.Arguments) != 2 {
		t.Errorf("Expected 2 arguments, got %d", len(config.Arguments))
	}

	if config.User != "1000" {
		t.Errorf("Expected user '1000', got '%s'", config.User)
	}
}

// =============================================================================
// Concurrent Exec Operations Tests
// =============================================================================

func TestExecSession_ConcurrentAccess(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent exec session lookups (all should fail with not found)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rt.GetExecSession("nonexistent")
			// Error is expected
			if err == nil {
				errCh <- err
			}
		}()
	}

	// Concurrent exec creates (all should fail - container not found)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			config := &ExecConfig{
				Command: []string{"/bin/echo", "test"},
			}
			_, err := rt.ExecInContainer(ctx, "nonexistent", config)
			// Error is expected
			if err == nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Unexpected success: %v", err)
	}
}

// =============================================================================
// IO Copy Tests
// =============================================================================

func TestIOCopy(t *testing.T) {
	// Test basic IO copy functionality
	input := bytes.NewBufferString("test data to copy")
	output := &bytes.Buffer{}

	n, err := io.Copy(output, input)
	if err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}

	if n != 17 {
		t.Errorf("Expected 17 bytes copied, got %d", n)
	}

	if output.String() != "test data to copy" {
		t.Errorf("Expected 'test data to copy', got '%s'", output.String())
	}
}

// =============================================================================
// Context Cancellation Tests
// =============================================================================

func TestExec_ContextCancellation(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	config := &ExecConfig{
		Command: []string{"/bin/echo", "hello"},
	}

	_, err := rt.ExecInContainer(ctx, "nonexistent", config)
	// Should still return container not found since that check happens first
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

func TestExecAttach_ContextCancellation(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(5 * time.Millisecond) // Let context expire

	streams := &IOStreams{}
	err := rt.ExecAttach(ctx, "nonexistent", streams)
	// Should fail - session not found
	if err == nil {
		t.Error("Expected error")
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkExecConfig_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &ExecConfig{
			Command:      []string{"/bin/sh", "-c", "echo hello"},
			Env:          []string{"FOO=bar"},
			WorkingDir:   "/app",
			User:         "1000",
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
		}
	}
}

func BenchmarkIOStreams_Write(b *testing.B) {
	stdout := &bytes.Buffer{}
	data := []byte("benchmark data to write")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stdout.Write(data)
		stdout.Reset()
	}
}
