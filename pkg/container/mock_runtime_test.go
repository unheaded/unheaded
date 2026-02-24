package container

import (
	"context"
	"testing"
	"time"
)

func testSpec(name string) *ContainerSpec {
	return &ContainerSpec{
		Name:  name,
		Image: "unheaded/" + name + ":latest",
		Labels: map[string]string{
			"app": name,
		},
	}
}

func TestMockRuntime_CreateAndGet(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, err := rt.Create(ctx, testSpec("timeguru"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.Name != "timeguru" {
		t.Errorf("expected timeguru, got %s", c.Name)
	}
	if c.State != StateCreated {
		t.Errorf("expected created state, got %s", c.State)
	}
	if c.Backend != BackendDocker {
		t.Errorf("expected docker backend, got %s", c.Backend)
	}

	got, err := rt.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != c.ID {
		t.Errorf("IDs don't match")
	}
}

func TestMockRuntime_CreateDuplicate(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	rt.Create(ctx, testSpec("test"))
	_, err := rt.Create(ctx, testSpec("test"))
	if err != ErrContainerExists {
		t.Errorf("expected ErrContainerExists, got %v", err)
	}
}

func TestMockRuntime_CreateInvalidSpec(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	_, err := rt.Create(ctx, &ContainerSpec{})
	if err == nil {
		t.Error("expected error for empty spec")
	}
}

func TestMockRuntime_StartStop(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("wotan"))

	if err := rt.Start(ctx, c.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got, _ := rt.Get(ctx, c.ID)
	if got.State != StateRunning {
		t.Errorf("expected running, got %s", got.State)
	}
	if got.PID == 0 {
		t.Error("expected non-zero PID")
	}

	if err := rt.Stop(ctx, c.ID, 10*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got, _ = rt.Get(ctx, c.ID)
	if got.State != StateStopped {
		t.Errorf("expected stopped, got %s", got.State)
	}
}

func TestMockRuntime_StartAlreadyRunning(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	rt.Start(ctx, c.ID)

	err := rt.Start(ctx, c.ID)
	if err != ErrContainerRunning {
		t.Errorf("expected ErrContainerRunning, got %v", err)
	}
}

func TestMockRuntime_StopNotRunning(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	err := rt.Stop(ctx, c.ID, 10*time.Second)
	if err != ErrContainerNotRunning {
		t.Errorf("expected ErrContainerNotRunning, got %v", err)
	}
}

func TestMockRuntime_Restart(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	rt.Start(ctx, c.ID)

	if err := rt.Restart(ctx, c.ID, 10*time.Second); err != nil {
		t.Fatalf("Restart: %v", err)
	}

	got, _ := rt.Get(ctx, c.ID)
	if got.State != StateRunning {
		t.Errorf("expected running after restart, got %s", got.State)
	}
}

func TestMockRuntime_PauseUnpause(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	rt.Start(ctx, c.ID)

	if err := rt.Pause(ctx, c.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	got, _ := rt.Get(ctx, c.ID)
	if got.State != StatePaused {
		t.Errorf("expected paused, got %s", got.State)
	}

	if err := rt.Unpause(ctx, c.ID); err != nil {
		t.Fatalf("Unpause: %v", err)
	}

	got, _ = rt.Get(ctx, c.ID)
	if got.State != StateRunning {
		t.Errorf("expected running after unpause, got %s", got.State)
	}
}

func TestMockRuntime_Delete(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))

	if err := rt.Delete(ctx, c.ID, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, _ := rt.Exists(ctx, c.ID)
	if exists {
		t.Error("container should not exist after delete")
	}
}

func TestMockRuntime_DeleteRunning(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	rt.Start(ctx, c.ID)

	err := rt.Delete(ctx, c.ID, false)
	if err != ErrContainerRunning {
		t.Errorf("expected ErrContainerRunning, got %v", err)
	}

	// Force delete should work
	if err := rt.Delete(ctx, c.ID, true); err != nil {
		t.Fatalf("Force delete: %v", err)
	}
}

func TestMockRuntime_List(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	rt.Create(ctx, testSpec("a"))
	rt.Create(ctx, testSpec("b"))
	rt.Create(ctx, testSpec("c"))

	all, _ := rt.List(ctx, nil)
	if len(all) != 3 {
		t.Errorf("expected 3 containers, got %d", len(all))
	}

	// Filter by label
	filtered, _ := rt.List(ctx, &Filter{
		Labels: map[string]string{"app": "a"},
	})
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered container, got %d", len(filtered))
	}
}

func TestMockRuntime_ListByState(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c1, _ := rt.Create(ctx, testSpec("running"))
	rt.Start(ctx, c1.ID)
	rt.Create(ctx, testSpec("stopped"))

	running, _ := rt.List(ctx, &Filter{State: []ContainerState{StateRunning}})
	if len(running) != 1 {
		t.Errorf("expected 1 running, got %d", len(running))
	}
}

func TestMockRuntime_Exec(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	rt.Start(ctx, c.ID)

	result, err := rt.Exec(ctx, c.ID, &ExecConfig{
		Command: []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestMockRuntime_ExecNotRunning(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	_, err := rt.Exec(ctx, c.ID, &ExecConfig{Command: []string{"ls"}})
	if err != ErrContainerNotRunning {
		t.Errorf("expected ErrContainerNotRunning, got %v", err)
	}
}

func TestMockRuntime_Logs(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	reader, err := rt.Logs(ctx, c.ID, nil)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 1024)
	n, _ := reader.Read(buf)
	if n == 0 {
		t.Error("expected some log output")
	}
}

func TestMockRuntime_Stats(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	rt.Start(ctx, c.ID)

	stats, err := rt.Stats(ctx, c.ID)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.ContainerID != c.ID {
		t.Error("stats should have container ID")
	}
	if stats.CPU.OnlineCPUs == 0 {
		t.Error("stats should have CPU info")
	}
	if stats.Memory.Usage == 0 {
		t.Error("stats should have memory info")
	}
}

func TestMockRuntime_HealthCheck(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	c, _ := rt.Create(ctx, testSpec("test"))
	rt.Start(ctx, c.ID)

	health, err := rt.HealthCheck(ctx, c.ID)
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if health.Status != HealthStateHealthy {
		t.Errorf("expected healthy, got %s", health.Status)
	}
}

func TestMockRuntime_Info(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	rt.Create(ctx, testSpec("a"))
	c, _ := rt.Create(ctx, testSpec("b"))
	rt.Start(ctx, c.ID)

	info, err := rt.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Backend != BackendDocker {
		t.Errorf("expected docker, got %s", info.Backend)
	}
	if info.Containers.Total != 2 {
		t.Errorf("expected 2 total, got %d", info.Containers.Total)
	}
	if info.Containers.Running != 1 {
		t.Errorf("expected 1 running, got %d", info.Containers.Running)
	}
}

func TestMockRuntime_Version(t *testing.T) {
	rt := NewTestableRuntime(BackendPodman)
	ctx := context.Background()

	ver, err := rt.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if ver.Version == "" {
		t.Error("expected non-empty version")
	}
}

func TestMockRuntime_NotFound(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	_, err := rt.Get(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}

	err = rt.Start(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound for Start, got %v", err)
	}

	err = rt.Delete(ctx, "nonexistent", false)
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound for Delete, got %v", err)
	}
}

func TestMockRuntime_Close(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	if err := rt.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := rt.Create(ctx, testSpec("test"))
	if err != ErrRuntimeNotInitialized {
		t.Errorf("expected ErrRuntimeNotInitialized after close, got %v", err)
	}
}

func TestMockRuntime_ContainerCount(t *testing.T) {
	rt := NewTestableRuntime(BackendDocker)
	ctx := context.Background()

	if rt.ContainerCount() != 0 {
		t.Error("expected 0 containers initially")
	}

	rt.Create(ctx, testSpec("a"))
	rt.Create(ctx, testSpec("b"))

	if rt.ContainerCount() != 2 {
		t.Errorf("expected 2 containers, got %d", rt.ContainerCount())
	}
}
