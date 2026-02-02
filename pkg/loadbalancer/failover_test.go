package loadbalancer

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestFailoverManager tests basic failover manager functionality.
func TestFailoverManager(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "primary1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "primary2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
		{Name: "backup1", Address: "127.0.0.1:8083", Weight: 1, Enabled: true},
		{Name: "backup2", Address: "127.0.0.1:8084", Weight: 1, Enabled: true},
	}

	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// Set all backends healthy
	for _, b := range pool.All() {
		b.SetState(BackendStateHealthy)
	}

	failoverConfig := FailoverConfig{
		Enabled:           true,
		Mode:              FailoverModePrimaryBackup,
		PrimaryGroup:      "primary",
		BackupGroups:      []string{"backup"},
		FailoverThreshold: 1,
		FailbackDelay:     100 * time.Millisecond,
		FailbackThreshold: 1,
	}

	fm := NewFailoverManager(failoverConfig, pool)

	// Add groups
	primaryGroup := &BackendGroup{
		Name:     "primary",
		Priority: 0,
		Backends: []string{"primary1", "primary2"},
	}
	backupGroup := &BackendGroup{
		Name:     "backup",
		Priority: 1,
		Backends: []string{"backup1", "backup2"},
	}

	if err := fm.AddGroup(primaryGroup); err != nil {
		t.Fatalf("Failed to add primary group: %v", err)
	}
	if err := fm.AddGroup(backupGroup); err != nil {
		t.Fatalf("Failed to add backup group: %v", err)
	}

	// Current group should be primary
	if fm.CurrentGroup() != "primary" {
		t.Errorf("Expected current group 'primary', got '%s'", fm.CurrentGroup())
	}

	// Should not be in failover
	if fm.IsInFailover() {
		t.Error("Should not be in failover initially")
	}
}

// TestFailoverTrigger tests that failover is triggered correctly.
func TestFailoverTrigger(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "primary1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backup1", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	// Set both healthy initially
	primary, _ := pool.Get("primary1")
	backup, _ := pool.Get("backup1")
	primary.SetState(BackendStateHealthy)
	backup.SetState(BackendStateHealthy)

	failoverConfig := FailoverConfig{
		Enabled:           true,
		Mode:              FailoverModePrimaryBackup,
		PrimaryGroup:      "primary",
		BackupGroups:      []string{"backup"},
		FailoverThreshold: 1,
		FailbackDelay:     100 * time.Millisecond,
		FailbackThreshold: 1,
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{Name: "primary", Priority: 0, Backends: []string{"primary1"}})
	fm.AddGroup(&BackendGroup{Name: "backup", Priority: 1, Backends: []string{"backup1"}})

	// Track failover callback
	var failoverCalled int32
	fm.OnFailover(func(fromGroup, toGroup, reason string) {
		atomic.StoreInt32(&failoverCalled, 1)
	})

	// Start failover manager
	if err := fm.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Make primary unhealthy
	primary.SetState(BackendStateUnhealthy)

	// Wait for failover to trigger
	time.Sleep(200 * time.Millisecond)

	// Should be in failover now
	if !fm.IsInFailover() {
		t.Error("Should be in failover after primary became unhealthy")
	}

	if fm.CurrentGroup() != "backup" {
		t.Errorf("Expected current group 'backup', got '%s'", fm.CurrentGroup())
	}

	if atomic.LoadInt32(&failoverCalled) != 1 {
		t.Error("Failover callback should have been called")
	}

	fm.Stop()
}

// TestFailback tests that failback works correctly.
func TestFailback(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "primary1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backup1", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	primary, _ := pool.Get("primary1")
	backup, _ := pool.Get("backup1")
	primary.SetState(BackendStateHealthy)
	backup.SetState(BackendStateHealthy)

	failoverConfig := FailoverConfig{
		Enabled:           true,
		Mode:              FailoverModePrimaryBackup,
		PrimaryGroup:      "primary",
		BackupGroups:      []string{"backup"},
		FailoverThreshold: 1,
		FailbackDelay:     50 * time.Millisecond, // Short delay for testing
		FailbackThreshold: 1,
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{Name: "primary", Priority: 0, Backends: []string{"primary1"}})
	fm.AddGroup(&BackendGroup{Name: "backup", Priority: 1, Backends: []string{"backup1"}})

	var failbackCalled int32
	fm.OnFailback(func(toGroup string) {
		atomic.StoreInt32(&failbackCalled, 1)
	})

	fm.Start()

	// Trigger failover
	primary.SetState(BackendStateUnhealthy)
	time.Sleep(100 * time.Millisecond)

	if !fm.IsInFailover() {
		t.Fatal("Should be in failover")
	}

	// Restore primary
	primary.SetState(BackendStateHealthy)

	// Wait for failback delay + processing
	time.Sleep(200 * time.Millisecond)

	// Should have failed back
	if fm.IsInFailover() {
		t.Error("Should have failed back to primary")
	}

	if fm.CurrentGroup() != "primary" {
		t.Errorf("Expected current group 'primary', got '%s'", fm.CurrentGroup())
	}

	if atomic.LoadInt32(&failbackCalled) != 1 {
		t.Error("Failback callback should have been called")
	}

	fm.Stop()
}

// TestFailoverSelect tests backend selection with failover.
func TestFailoverSelect(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "primary1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backup1", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	primary, _ := pool.Get("primary1")
	backup, _ := pool.Get("backup1")
	primary.SetState(BackendStateHealthy)
	backup.SetState(BackendStateHealthy)

	failoverConfig := FailoverConfig{
		Enabled:           true,
		Mode:              FailoverModePrimaryBackup,
		PrimaryGroup:      "primary",
		BackupGroups:      []string{"backup"},
		FailoverThreshold: 1,
		FailbackDelay:     time.Hour, // Long delay to prevent failback during test
		FailbackThreshold: 1,
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{Name: "primary", Priority: 0, Backends: []string{"primary1"}})
	fm.AddGroup(&BackendGroup{Name: "backup", Priority: 1, Backends: []string{"backup1"}})

	// Select should return primary backend
	ctx := context.Background()
	selected, err := fm.Select(ctx, "test-key")
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected.Config.Name != "primary1" {
		t.Errorf("Expected primary1, got %s", selected.Config.Name)
	}

	// Make primary unavailable
	primary.SetState(BackendStateUnhealthy)

	// Start manager to trigger failover
	fm.Start()
	time.Sleep(100 * time.Millisecond)

	// Select should now return backup
	selected, err = fm.Select(ctx, "test-key")
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected.Config.Name != "backup1" {
		t.Errorf("Expected backup1, got %s", selected.Config.Name)
	}

	fm.Stop()
}

// TestZoneAwareFailover tests zone-aware failover.
func TestZoneAwareFailover(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "zone-a-1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "zone-a-2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
		{Name: "zone-b-1", Address: "127.0.0.1:8083", Weight: 1, Enabled: true},
		{Name: "zone-b-2", Address: "127.0.0.1:8084", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	// Set all healthy
	for _, b := range pool.All() {
		b.SetState(BackendStateHealthy)
	}

	failoverConfig := FailoverConfig{
		Enabled:               true,
		Mode:                  FailoverModeZone,
		ZoneAware:             true,
		PreferLocalZone:       true,
		LocalZone:             "zone-a",
		ZoneFailoverThreshold: 1,
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{
		Name:     "zone-a-group",
		Priority: 0,
		Zone:     "zone-a",
		Backends: []string{"zone-a-1", "zone-a-2"},
	})
	fm.AddGroup(&BackendGroup{
		Name:     "zone-b-group",
		Priority: 1,
		Zone:     "zone-b",
		Backends: []string{"zone-b-1", "zone-b-2"},
	})

	var zoneShiftCalled int32
	fm.OnZoneShift(func(fromZone, toZone string) {
		atomic.StoreInt32(&zoneShiftCalled, 1)
	})

	// Select with zone preference
	ctx := context.Background()
	selected, err := fm.SelectWithZonePreference(ctx, "test-key")
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	// Should be from local zone
	if selected.Config.Name != "zone-a-1" && selected.Config.Name != "zone-a-2" {
		t.Errorf("Expected zone-a backend, got %s", selected.Config.Name)
	}

	// Make local zone unhealthy
	zoneA1, _ := pool.Get("zone-a-1")
	zoneA2, _ := pool.Get("zone-a-2")
	zoneA1.SetState(BackendStateUnhealthy)
	zoneA2.SetState(BackendStateUnhealthy)

	// Start manager
	fm.Start()
	time.Sleep(200 * time.Millisecond)

	// Select should now use zone-b
	selected, err = fm.SelectWithZonePreference(ctx, "test-key")
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	if selected.Config.Name != "zone-b-1" && selected.Config.Name != "zone-b-2" {
		t.Errorf("Expected zone-b backend, got %s", selected.Config.Name)
	}

	fm.Stop()
}

// TestPriorityFailover tests priority-based failover.
func TestPriorityFailover(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "high-pri", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "med-pri", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
		{Name: "low-pri", Address: "127.0.0.1:8083", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	for _, b := range pool.All() {
		b.SetState(BackendStateHealthy)
	}

	failoverConfig := FailoverConfig{
		Enabled:      true,
		Mode:         FailoverModePriority,
		PrimaryGroup: "high",
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{Name: "high", Priority: 0, Backends: []string{"high-pri"}})
	fm.AddGroup(&BackendGroup{Name: "medium", Priority: 1, Backends: []string{"med-pri"}})
	fm.AddGroup(&BackendGroup{Name: "low", Priority: 2, Backends: []string{"low-pri"}})

	fm.Start()
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()

	// Should use highest priority (0)
	selected, _ := fm.Select(ctx, "test")
	if selected.Config.Name != "high-pri" {
		t.Errorf("Expected high-pri, got %s", selected.Config.Name)
	}

	// Make high priority unhealthy
	highPri, _ := pool.Get("high-pri")
	highPri.SetState(BackendStateUnhealthy)
	time.Sleep(200 * time.Millisecond)

	// Should use medium priority
	selected, _ = fm.Select(ctx, "test")
	if selected.Config.Name != "med-pri" {
		t.Errorf("Expected med-pri, got %s", selected.Config.Name)
	}

	fm.Stop()
}

// TestFailoverStats tests failover statistics.
func TestFailoverStats(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "primary1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backup1", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	primary, _ := pool.Get("primary1")
	backup, _ := pool.Get("backup1")
	primary.SetState(BackendStateHealthy)
	backup.SetState(BackendStateHealthy)

	failoverConfig := FailoverConfig{
		Enabled:           true,
		Mode:              FailoverModePrimaryBackup,
		PrimaryGroup:      "primary",
		BackupGroups:      []string{"backup"},
		FailoverThreshold: 1,
		FailbackDelay:     50 * time.Millisecond,
		FailbackThreshold: 1,
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{Name: "primary", Priority: 0, Backends: []string{"primary1"}})
	fm.AddGroup(&BackendGroup{Name: "backup", Priority: 1, Backends: []string{"backup1"}})

	fm.Start()

	// Get initial stats
	stats := fm.Stats()
	if stats.FailoverCount != 0 {
		t.Errorf("Expected 0 failovers, got %d", stats.FailoverCount)
	}

	// Trigger failover
	primary.SetState(BackendStateUnhealthy)
	time.Sleep(200 * time.Millisecond)

	stats = fm.Stats()
	if stats.FailoverCount != 1 {
		t.Errorf("Expected 1 failover, got %d", stats.FailoverCount)
	}
	if !stats.InFailover {
		t.Error("Should be in failover")
	}

	// Trigger failback
	primary.SetState(BackendStateHealthy)
	time.Sleep(200 * time.Millisecond)

	stats = fm.Stats()
	if stats.FailbackCount != 1 {
		t.Errorf("Expected 1 failback, got %d", stats.FailbackCount)
	}
	if stats.InFailover {
		t.Error("Should not be in failover")
	}

	fm.Stop()
}

// TestGracefulDrainManager tests the graceful drain manager.
func TestGracefulDrainManager(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)

	dm := NewGracefulDrainManager(pool, 5*time.Second)

	var drainStarted, drainCompleted int32
	dm.OnDrainStart(func(b *Backend) {
		atomic.StoreInt32(&drainStarted, 1)
	})
	dm.OnDrainComplete(func(b *Backend, success bool) {
		atomic.StoreInt32(&drainCompleted, 1)
	})

	dm.Start()

	// Start draining
	if err := dm.StartDrain("backend1", 100*time.Millisecond); err != nil {
		t.Fatalf("StartDrain failed: %v", err)
	}

	// Verify drain started
	if atomic.LoadInt32(&drainStarted) != 1 {
		t.Error("Drain start callback should have been called")
	}

	// Verify backend is draining
	if !dm.IsDraining("backend1") {
		t.Error("Backend should be draining")
	}

	// Wait for drain to complete (no connections, so immediate)
	time.Sleep(200 * time.Millisecond)

	// Verify drain completed
	if atomic.LoadInt32(&drainCompleted) != 1 {
		t.Error("Drain complete callback should have been called")
	}

	// Should no longer be draining
	if dm.IsDraining("backend1") {
		t.Error("Backend should no longer be draining")
	}

	dm.Stop()
}

// TestGracefulDrainWithConnections tests draining with active connections.
func TestGracefulDrainWithConnections(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)

	// Simulate active connections
	backend.IncrementConnections()
	backend.IncrementConnections()

	dm := NewGracefulDrainManager(pool, 5*time.Second)
	dm.Start()

	// Start draining
	dm.StartDrain("backend1", 1*time.Second)

	// Get drain state
	state, ok := dm.GetDrainState("backend1")
	if !ok {
		t.Fatal("Expected drain state")
	}
	if state.InitialConns != 2 {
		t.Errorf("Expected initial conns 2, got %d", state.InitialConns)
	}

	// Remove one connection
	backend.DecrementConnections()

	// Still draining
	if !dm.IsDraining("backend1") {
		t.Error("Should still be draining")
	}

	// Remove last connection
	backend.DecrementConnections()

	// Wait for drain check
	time.Sleep(200 * time.Millisecond)

	// Should be done
	if dm.IsDraining("backend1") {
		t.Error("Should be done draining")
	}

	dm.Stop()
}

// TestGracefulDrainTimeout tests drain timeout behavior.
func TestGracefulDrainTimeout(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)

	// Keep connection active
	backend.IncrementConnections()

	dm := NewGracefulDrainManager(pool, 100*time.Millisecond) // Short default timeout

	var drainSuccess bool
	dm.OnDrainComplete(func(b *Backend, success bool) {
		drainSuccess = success
	})

	dm.Start()

	// Start draining with short timeout
	dm.StartDrain("backend1", 100*time.Millisecond)

	// Wait for timeout
	time.Sleep(300 * time.Millisecond)

	// Drain should have failed (timed out with active connection)
	if drainSuccess {
		t.Error("Drain should have failed due to timeout")
	}

	dm.Stop()
}

// TestForceDrain tests force completing a drain.
func TestForceDrain(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)
	backend.IncrementConnections()

	dm := NewGracefulDrainManager(pool, 10*time.Second)

	var completeCalled bool
	dm.OnDrainComplete(func(b *Backend, success bool) {
		completeCalled = true
	})

	dm.Start()

	dm.StartDrain("backend1", 10*time.Second)

	// Force complete
	if err := dm.ForceDrain("backend1"); err != nil {
		t.Fatalf("ForceDrain failed: %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	if !completeCalled {
		t.Error("Drain complete callback should have been called")
	}

	if dm.IsDraining("backend1") {
		t.Error("Should no longer be draining after force")
	}

	dm.Stop()
}

// TestDrainStats tests drain statistics.
func TestDrainStats(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backend2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	backend1, _ := pool.Get("backend1")
	backend2, _ := pool.Get("backend2")
	backend1.SetState(BackendStateHealthy)
	backend2.SetState(BackendStateHealthy)
	backend1.IncrementConnections()
	backend2.IncrementConnections()
	backend2.IncrementConnections()

	dm := NewGracefulDrainManager(pool, 10*time.Second)
	dm.Start()

	dm.StartDrain("backend1", 5*time.Second)
	dm.StartDrain("backend2", 5*time.Second)

	stats := dm.DrainStats()

	if stats.TotalDraining != 2 {
		t.Errorf("Expected 2 draining, got %d", stats.TotalDraining)
	}

	if stats.DrainingBackends["backend1"].InitialConns != 1 {
		t.Errorf("Expected backend1 initial conns 1, got %d", stats.DrainingBackends["backend1"].InitialConns)
	}

	if stats.DrainingBackends["backend2"].InitialConns != 2 {
		t.Errorf("Expected backend2 initial conns 2, got %d", stats.DrainingBackends["backend2"].InitialConns)
	}

	dm.Stop()
}

// TestWeightedFailoverManager tests weighted failover distribution.
func TestWeightedFailoverManager(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "group1-1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "group1-2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
		{Name: "group2-1", Address: "127.0.0.1:8083", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	for _, b := range pool.All() {
		b.SetState(BackendStateHealthy)
	}

	failoverConfig := FailoverConfig{
		Enabled: true,
		Mode:    FailoverModeWeighted,
	}

	wfm := NewWeightedFailoverManager(failoverConfig, pool)

	// Add groups with weights
	wfm.AddGroup(&BackendGroup{
		Name:     "group1",
		Weight:   70,
		Backends: []string{"group1-1", "group1-2"},
	})
	wfm.AddGroup(&BackendGroup{
		Name:     "group2",
		Weight:   30,
		Backends: []string{"group2-1"},
	})

	// Make many selections and verify distribution
	group1Count := 0
	group2Count := 0

	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		key := string(rune('a' + i%26))
		selected, err := wfm.SelectWeighted(ctx, key)
		if err != nil {
			continue
		}

		if selected.Config.Name == "group1-1" || selected.Config.Name == "group1-2" {
			group1Count++
		} else {
			group2Count++
		}
	}

	// Distribution should roughly match weights (70/30)
	// Allow 15% tolerance
	group1Ratio := float64(group1Count) / float64(group1Count+group2Count)
	if group1Ratio < 0.55 || group1Ratio > 0.85 {
		t.Errorf("Weighted distribution off: group1 ratio = %f, expected ~0.70", group1Ratio)
	}
}

// TestAddRemoveGroup tests adding and removing groups.
func TestAddRemoveGroup(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backend2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	failoverConfig := FailoverConfig{
		Enabled:      true,
		Mode:         FailoverModePrimaryBackup,
		PrimaryGroup: "group1",
	}

	fm := NewFailoverManager(failoverConfig, pool)

	// Add group
	err := fm.AddGroup(&BackendGroup{
		Name:     "group1",
		Backends: []string{"backend1"},
	})
	if err != nil {
		t.Fatalf("AddGroup failed: %v", err)
	}

	// Try to add duplicate
	err = fm.AddGroup(&BackendGroup{
		Name:     "group1",
		Backends: []string{"backend2"},
	})
	if err == nil {
		t.Error("Expected error adding duplicate group")
	}

	// Add another group
	err = fm.AddGroup(&BackendGroup{
		Name:     "group2",
		Backends: []string{"backend2"},
	})
	if err != nil {
		t.Fatalf("AddGroup failed: %v", err)
	}

	// Can't remove current group
	err = fm.RemoveGroup("group1")
	if err == nil {
		t.Error("Expected error removing current group")
	}

	// Can remove other group
	err = fm.RemoveGroup("group2")
	if err != nil {
		t.Fatalf("RemoveGroup failed: %v", err)
	}

	// Try to remove non-existent
	err = fm.RemoveGroup("nonexistent")
	if err == nil {
		t.Error("Expected error removing non-existent group")
	}
}

// TestGroupHealth tests getting group health information.
func TestGroupHealth(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backend2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	backend1, _ := pool.Get("backend1")
	backend2, _ := pool.Get("backend2")
	backend1.SetState(BackendStateHealthy)
	backend2.SetState(BackendStateUnhealthy)

	failoverConfig := FailoverConfig{
		Enabled:      true,
		PrimaryGroup: "group1",
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{
		Name:     "group1",
		Backends: []string{"backend1", "backend2"},
	})

	fm.Start()
	time.Sleep(100 * time.Millisecond)

	healthy, total, draining := fm.GetGroupHealth("group1")
	if healthy != 1 {
		t.Errorf("Expected 1 healthy, got %d", healthy)
	}
	if total != 2 {
		t.Errorf("Expected 2 total, got %d", total)
	}
	if draining {
		t.Error("Group should not be draining")
	}

	fm.Stop()
}

// TestFailoverManagerDoubleStart tests starting failover manager twice.
func TestFailoverManagerDoubleStart(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	failoverConfig := FailoverConfig{
		Enabled: true,
	}

	fm := NewFailoverManager(failoverConfig, pool)

	fm.AddGroup(&BackendGroup{Name: "group1", Backends: []string{"backend1"}})

	if err := fm.Start(); err != nil {
		t.Fatalf("First start failed: %v", err)
	}

	if err := fm.Start(); err == nil {
		t.Error("Expected error on second start")
	}

	fm.Stop()
}

// TestDrainAllBackends tests draining all backends.
func TestDrainAllBackends(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "backend2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
		{Name: "backend3", Address: "127.0.0.1:8083", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)

	for _, b := range pool.All() {
		b.SetState(BackendStateHealthy)
	}

	dm := NewGracefulDrainManager(pool, 5*time.Second)
	dm.Start()

	// Drain all
	dm.DrainAll(1 * time.Second)

	// All should be draining
	for _, b := range pool.All() {
		if !dm.IsDraining(b.Config.Name) {
			t.Errorf("Backend %s should be draining", b.Config.Name)
		}
	}

	dm.Stop()
}

// TestStopDrain tests cancelling a drain.
func TestStopDrain(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{Name: "backend1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
	}

	pool, _ := NewBackendPool(config)
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)
	backend.Drain() // This sets state to draining

	dm := NewGracefulDrainManager(pool, 10*time.Second)
	dm.Start()

	dm.StartDrain("backend1", 10*time.Second)

	if !dm.IsDraining("backend1") {
		t.Error("Backend should be draining")
	}

	// Stop drain
	if err := dm.StopDrain("backend1"); err != nil {
		t.Fatalf("StopDrain failed: %v", err)
	}

	if dm.IsDraining("backend1") {
		t.Error("Backend should not be draining after stop")
	}

	// Backend should be enabled again (unknown state, will need health check)
	if backend.State() == BackendStateDraining {
		t.Error("Backend state should not be draining after stop")
	}

	dm.Stop()
}

// BenchmarkFailoverSelect benchmarks failover selection.
func BenchmarkFailoverSelect(b *testing.B) {
	config := DefaultConfig()
	for i := 0; i < 10; i++ {
		config.Backends = append(config.Backends, BackendConfig{
			Name:    fmt.Sprintf("backend%d", i),
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		})
	}

	pool, _ := NewBackendPool(config)

	for _, backend := range pool.All() {
		backend.SetState(BackendStateHealthy)
	}

	var backendNames []string
	for i := 0; i < 10; i++ {
		backendNames = append(backendNames, fmt.Sprintf("backend%d", i))
	}

	failoverConfig := FailoverConfig{
		Enabled:      true,
		Mode:         FailoverModePrimaryBackup,
		PrimaryGroup: "primary",
	}

	fm := NewFailoverManager(failoverConfig, pool)
	fm.AddGroup(&BackendGroup{Name: "primary", Priority: 0, Backends: backendNames})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fm.Select(ctx, "test-key")
	}
}

// BenchmarkWeightedFailoverSelect benchmarks weighted failover selection.
func BenchmarkWeightedFailoverSelect(b *testing.B) {
	config := DefaultConfig()
	for i := 0; i < 10; i++ {
		config.Backends = append(config.Backends, BackendConfig{
			Name:    fmt.Sprintf("backend%d", i),
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		})
	}

	pool, _ := NewBackendPool(config)

	for _, backend := range pool.All() {
		backend.SetState(BackendStateHealthy)
	}

	failoverConfig := FailoverConfig{
		Enabled: true,
		Mode:    FailoverModeWeighted,
	}

	wfm := NewWeightedFailoverManager(failoverConfig, pool)

	wfm.AddGroup(&BackendGroup{
		Name:     "group1",
		Weight:   70,
		Backends: []string{"backend0", "backend1", "backend2", "backend3", "backend4"},
	})
	wfm.AddGroup(&BackendGroup{
		Name:     "group2",
		Weight:   30,
		Backends: []string{"backend5", "backend6", "backend7", "backend8", "backend9"},
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wfm.SelectWeighted(ctx, "test-key")
	}
}

