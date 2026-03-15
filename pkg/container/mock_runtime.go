// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// TestableRuntime is an in-memory container runtime for testing.
// It implements the full Runtime interface and tracks container state.
type TestableRuntime struct {
	mu         sync.RWMutex
	containers map[string]*Container
	nextID     int
	backend    BackendType
	closed     bool
}

// NewTestableRuntime creates a mock runtime for testing.
func NewTestableRuntime(backend BackendType) *TestableRuntime {
	return &TestableRuntime{
		containers: make(map[string]*Container),
		backend:    backend,
	}
}

func (r *TestableRuntime) Create(_ context.Context, spec *ContainerSpec) (*Container, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrRuntimeNotInitialized
	}

	for _, c := range r.containers {
		if c.Name == spec.Name {
			return nil, ErrContainerExists
		}
	}

	r.nextID++
	id := fmt.Sprintf("%s-%d", r.backend, r.nextID)
	now := time.Now()

	c := &Container{
		ID:        id,
		Name:      spec.Name,
		Image:     spec.Image,
		State:     StateCreated,
		Status:    "created",
		CreatedAt: now,
		UpdatedAt: now,
		Spec:      spec,
		Labels:    spec.Labels,
		Backend:   r.backend,
		Network:   make(map[string]NetworkInfo),
	}

	r.containers[id] = c
	return c, nil
}

func (r *TestableRuntime) Start(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.containers[id]
	if !ok {
		return ErrContainerNotFound
	}
	if c.State == StateRunning {
		return ErrContainerRunning
	}

	c.State = StateRunning
	c.Status = "running"
	c.StartedAt = time.Now()
	c.UpdatedAt = time.Now()
	c.PID = int64(r.nextID * 1000)
	return nil
}

func (r *TestableRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.containers[id]
	if !ok {
		return ErrContainerNotFound
	}
	if c.State != StateRunning && c.State != StatePaused {
		return ErrContainerNotRunning
	}

	c.State = StateStopped
	c.Status = "stopped"
	c.FinishedAt = time.Now()
	c.UpdatedAt = time.Now()
	c.PID = 0
	return nil
}

func (r *TestableRuntime) Restart(ctx context.Context, id string, timeout time.Duration) error {
	if err := r.Stop(ctx, id, timeout); err != nil && err != ErrContainerNotRunning {
		return err
	}
	return r.Start(ctx, id)
}

func (r *TestableRuntime) Pause(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.containers[id]
	if !ok {
		return ErrContainerNotFound
	}
	if c.State != StateRunning {
		return ErrContainerNotRunning
	}

	c.State = StatePaused
	c.Status = "paused"
	c.UpdatedAt = time.Now()
	return nil
}

func (r *TestableRuntime) Unpause(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.containers[id]
	if !ok {
		return ErrContainerNotFound
	}
	if c.State != StatePaused {
		return fmt.Errorf("container is not paused")
	}

	c.State = StateRunning
	c.Status = "running"
	c.UpdatedAt = time.Now()
	return nil
}

func (r *TestableRuntime) Delete(_ context.Context, id string, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.containers[id]
	if !ok {
		return ErrContainerNotFound
	}
	if c.State == StateRunning && !force {
		return ErrContainerRunning
	}

	delete(r.containers, id)
	return nil
}

func (r *TestableRuntime) Get(_ context.Context, id string) (*Container, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.containers[id]
	if !ok {
		return nil, ErrContainerNotFound
	}
	return c, nil
}

func (r *TestableRuntime) List(_ context.Context, filter *Filter) ([]*Container, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Container
	for _, c := range r.containers {
		if c.Match(filter) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (r *TestableRuntime) Exists(_ context.Context, id string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.containers[id]
	return ok, nil
}

func (r *TestableRuntime) Exec(_ context.Context, id string, cmd *ExecConfig) (*ExecResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.containers[id]
	if !ok {
		return nil, ErrContainerNotFound
	}
	if c.State != StateRunning {
		return nil, ErrContainerNotRunning
	}

	return &ExecResult{
		ExitCode: 0,
		Stdout:   []byte(fmt.Sprintf("mock exec: %s\n", strings.Join(cmd.Command, " "))),
	}, nil
}

func (r *TestableRuntime) Logs(_ context.Context, id string, _ *LogOptions) (io.ReadCloser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.containers[id]
	if !ok {
		return nil, ErrContainerNotFound
	}

	logContent := fmt.Sprintf("[%s] mock log output for %s\n", time.Now().Format(time.RFC3339), c.Name)
	return io.NopCloser(bytes.NewReader([]byte(logContent))), nil
}

func (r *TestableRuntime) Attach(_ context.Context, id string, _ *AttachOptions) (*AttachResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.containers[id]; !ok {
		return nil, ErrContainerNotFound
	}

	exitChan := make(chan int, 1)
	exitChan <- 0

	return &AttachResult{
		Stdout:   io.NopCloser(bytes.NewReader(nil)),
		Stderr:   io.NopCloser(bytes.NewReader(nil)),
		ExitChan: exitChan,
	}, nil
}

func (r *TestableRuntime) Stats(_ context.Context, id string) (*Stats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.containers[id]
	if !ok {
		return nil, ErrContainerNotFound
	}
	if c.State != StateRunning {
		return nil, ErrContainerNotRunning
	}

	return &Stats{
		ContainerID: id,
		Timestamp:   time.Now(),
		CPU: CPUStats{
			PercentUsage: 5.0,
			OnlineCPUs:   4,
		},
		Memory: MemoryStats{
			Usage: 64 * 1024 * 1024, // 64MB
			Limit: 256 * 1024 * 1024,
		},
		PIDs: 12,
	}, nil
}

func (r *TestableRuntime) StatsStream(_ context.Context, id string) (<-chan *Stats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.containers[id]; !ok {
		return nil, ErrContainerNotFound
	}

	ch := make(chan *Stats)
	close(ch)
	return ch, nil
}

func (r *TestableRuntime) HealthCheck(_ context.Context, id string) (*HealthStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.containers[id]
	if !ok {
		return nil, ErrContainerNotFound
	}

	state := HealthStateNone
	if c.State == StateRunning {
		state = HealthStateHealthy
	}

	return &HealthStatus{
		Status: state,
	}, nil
}

func (r *TestableRuntime) Info(_ context.Context) (*RuntimeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	running, paused, stopped := 0, 0, 0
	for _, c := range r.containers {
		switch c.State {
		case StateRunning:
			running++
		case StatePaused:
			paused++
		case StateStopped, StateExited:
			stopped++
		}
	}

	return &RuntimeInfo{
		Backend:    r.backend,
		Version:    "mock-1.0",
		APIVersion: "1.0",
		OS:         "linux",
		Arch:       "amd64",
		Containers: RuntimeCounts{
			Total:   len(r.containers),
			Running: running,
			Paused:  paused,
			Stopped: stopped,
		},
		CPUs:     4,
		MemTotal: 8 * 1024 * 1024 * 1024,
	}, nil
}

func (r *TestableRuntime) Version(_ context.Context) (*RuntimeVersion, error) {
	return &RuntimeVersion{
		Version:    "mock-1.0",
		APIVersion: "1.0",
		OS:         "linux",
		Arch:       "amd64",
	}, nil
}

func (r *TestableRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// ContainerCount returns the number of containers for testing.
func (r *TestableRuntime) ContainerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.containers)
}
