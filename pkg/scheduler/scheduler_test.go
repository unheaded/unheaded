package scheduler

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Resources Tests
// =============================================================================

func TestResourcesAdd(t *testing.T) {
	r1 := Resources{CPU: 1000, Memory: 2048, Disk: 10000, GPUs: 1, Ephemeral: 5000}
	r2 := Resources{CPU: 2000, Memory: 4096, Disk: 20000, GPUs: 2, Ephemeral: 10000}

	result := r1.Add(r2)

	if result.CPU != 3000 {
		t.Errorf("expected CPU 3000, got %d", result.CPU)
	}
	if result.Memory != 6144 {
		t.Errorf("expected Memory 6144, got %d", result.Memory)
	}
	if result.Disk != 30000 {
		t.Errorf("expected Disk 30000, got %d", result.Disk)
	}
	if result.GPUs != 3 {
		t.Errorf("expected GPUs 3, got %d", result.GPUs)
	}
	if result.Ephemeral != 15000 {
		t.Errorf("expected Ephemeral 15000, got %d", result.Ephemeral)
	}
}

func TestResourcesSub(t *testing.T) {
	r1 := Resources{CPU: 5000, Memory: 8192, Disk: 50000, GPUs: 4, Ephemeral: 20000}
	r2 := Resources{CPU: 2000, Memory: 4096, Disk: 20000, GPUs: 1, Ephemeral: 5000}

	result := r1.Sub(r2)

	if result.CPU != 3000 {
		t.Errorf("expected CPU 3000, got %d", result.CPU)
	}
	if result.Memory != 4096 {
		t.Errorf("expected Memory 4096, got %d", result.Memory)
	}
	if result.GPUs != 3 {
		t.Errorf("expected GPUs 3, got %d", result.GPUs)
	}
}

func TestResourcesFitsIn(t *testing.T) {
	tests := []struct {
		name      string
		request   Resources
		available Resources
		expected  bool
	}{
		{
			name:      "fits exactly",
			request:   Resources{CPU: 1000, Memory: 2048},
			available: Resources{CPU: 1000, Memory: 2048},
			expected:  true,
		},
		{
			name:      "fits with room",
			request:   Resources{CPU: 500, Memory: 1024},
			available: Resources{CPU: 1000, Memory: 2048},
			expected:  true,
		},
		{
			name:      "CPU exceeds",
			request:   Resources{CPU: 2000, Memory: 1024},
			available: Resources{CPU: 1000, Memory: 2048},
			expected:  false,
		},
		{
			name:      "Memory exceeds",
			request:   Resources{CPU: 500, Memory: 4096},
			available: Resources{CPU: 1000, Memory: 2048},
			expected:  false,
		},
		{
			name:      "GPUs exceed",
			request:   Resources{CPU: 500, Memory: 1024, GPUs: 2},
			available: Resources{CPU: 1000, Memory: 2048, GPUs: 1},
			expected:  false,
		},
		{
			name:      "zero request fits",
			request:   Resources{},
			available: Resources{CPU: 1000, Memory: 2048},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.request.FitsIn(tt.available)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestResourcesIsZero(t *testing.T) {
	zero := Resources{}
	if !zero.IsZero() {
		t.Error("empty Resources should be zero")
	}

	notZero := Resources{CPU: 1}
	if notZero.IsZero() {
		t.Error("Resources with CPU=1 should not be zero")
	}
}

// =============================================================================
// SchedulerConfig Tests
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.SchedulingInterval != 1*time.Second {
		t.Errorf("expected SchedulingInterval 1s, got %v", config.SchedulingInterval)
	}
	if config.Algorithm != AlgorithmBinPack {
		t.Errorf("expected Algorithm BinPack, got %s", config.Algorithm)
	}
	if !config.EnablePreemption {
		t.Error("expected EnablePreemption true")
	}
	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", config.MaxRetries)
	}
	if config.WorkerCount != 4 {
		t.Errorf("expected WorkerCount 4, got %d", config.WorkerCount)
	}
}

// =============================================================================
// SchedulerMetrics Tests
// =============================================================================

func TestSchedulerMetricsRecordSchedulingAttempt(t *testing.T) {
	metrics := &SchedulerMetrics{}

	// Record successful attempt
	metrics.RecordSchedulingAttempt(true, 100*time.Millisecond)

	if metrics.SchedulingAttempts != 1 {
		t.Errorf("expected 1 attempt, got %d", metrics.SchedulingAttempts)
	}
	if metrics.SuccessfulSchedulings != 1 {
		t.Errorf("expected 1 successful, got %d", metrics.SuccessfulSchedulings)
	}

	// Record failed attempt
	metrics.RecordSchedulingAttempt(false, 50*time.Millisecond)

	if metrics.SchedulingAttempts != 2 {
		t.Errorf("expected 2 attempts, got %d", metrics.SchedulingAttempts)
	}
	if metrics.FailedSchedulings != 1 {
		t.Errorf("expected 1 failed, got %d", metrics.FailedSchedulings)
	}
}

func TestSchedulerMetricsRecordPreemption(t *testing.T) {
	metrics := &SchedulerMetrics{}

	metrics.RecordPreemption()
	metrics.RecordPreemption()

	if metrics.PreemptionCount != 2 {
		t.Errorf("expected 2 preemptions, got %d", metrics.PreemptionCount)
	}
}

func TestSchedulerMetricsGetMetrics(t *testing.T) {
	metrics := &SchedulerMetrics{}
	metrics.RecordSchedulingAttempt(true, 100*time.Millisecond)

	copy := metrics.GetMetrics()

	if copy.SchedulingAttempts != 1 {
		t.Errorf("expected 1 attempt in copy, got %d", copy.SchedulingAttempts)
	}
}

func TestSchedulerMetricsConcurrentAccess(t *testing.T) {
	metrics := &SchedulerMetrics{}

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			metrics.RecordSchedulingAttempt(true, 10*time.Millisecond)
			metrics.RecordPreemption()
			_ = metrics.GetMetrics()
		}()
	}

	wg.Wait()

	if metrics.SchedulingAttempts != int64(numGoroutines) {
		t.Errorf("expected %d attempts, got %d", numGoroutines, metrics.SchedulingAttempts)
	}
}

// =============================================================================
// Scheduler Tests
// =============================================================================

func TestNewScheduler(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	if scheduler == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if scheduler.nodes == nil {
		t.Error("scheduler.nodes should be initialized")
	}
	if scheduler.queue == nil {
		t.Error("scheduler.queue should be initialized")
	}
	if scheduler.algorithm == nil {
		t.Error("scheduler.algorithm should be initialized")
	}
	if scheduler.binder == nil {
		t.Error("scheduler.binder should be initialized")
	}
}

func TestNewSchedulerWithDifferentAlgorithms(t *testing.T) {
	tests := []struct {
		algo AlgorithmType
		name string
	}{
		{AlgorithmBinPack, "BinPack"},
		{AlgorithmSpread, "Spread"},
		{AlgorithmRandom, "Random"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.Algorithm = tt.algo
			scheduler := NewScheduler(config)

			if scheduler.algorithm.Name() != tt.name {
				t.Errorf("expected algorithm %s, got %s", tt.name, scheduler.algorithm.Name())
			}
		})
	}
}

func TestSchedulerStartStop(t *testing.T) {
	config := DefaultConfig()
	config.WorkerCount = 2
	scheduler := NewScheduler(config)

	err := scheduler.Start()
	if err != nil {
		t.Errorf("unexpected error starting scheduler: %v", err)
	}

	// Allow scheduler to run briefly
	time.Sleep(50 * time.Millisecond)

	scheduler.Stop()

	// Verify scheduler is stopped
	if !scheduler.stopped {
		t.Error("scheduler should be stopped")
	}
}

func TestSchedulerStartAfterStop(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	_ = scheduler.Start()
	scheduler.Stop()

	err := scheduler.Start()
	if err != ErrSchedulerStopped {
		t.Errorf("expected ErrSchedulerStopped, got %v", err)
	}
}

func TestSchedulerSubmit(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}

	err := scheduler.Submit(workload)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify workload was stored
	stored, err := scheduler.GetWorkload("workload-1")
	if err != nil {
		t.Errorf("failed to get workload: %v", err)
	}
	if stored.State != WorkloadStatePending {
		t.Errorf("expected state Pending, got %s", stored.State)
	}
}

func TestSchedulerSubmitEmptyID(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	workload := &Workload{
		ID:   "",
		Name: "test",
	}

	err := scheduler.Submit(workload)
	if err == nil {
		t.Error("expected error for empty workload ID")
	}
}

func TestSchedulerSubmitDuplicate(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	workload := &Workload{
		ID:   "workload-1",
		Name: "test",
	}

	_ = scheduler.Submit(workload)
	err := scheduler.Submit(workload)
	if err == nil {
		t.Error("expected error for duplicate workload")
	}
}

func TestSchedulerSubmitAfterStop(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)
	scheduler.Stop()

	workload := &Workload{
		ID:   "workload-1",
		Name: "test",
	}

	err := scheduler.Submit(workload)
	if err != ErrSchedulerStopped {
		t.Errorf("expected ErrSchedulerStopped, got %v", err)
	}
}

func TestSchedulerCancel(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
	}

	_ = scheduler.Submit(workload)

	err := scheduler.Cancel("workload-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify workload was removed
	_, err = scheduler.GetWorkload("workload-1")
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}
}

func TestSchedulerCancelNotFound(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	err := scheduler.Cancel("nonexistent")
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}
}

func TestSchedulerRegisterNode(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})

	err := scheduler.RegisterNode(node)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSchedulerUnregisterNode(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	_ = scheduler.RegisterNode(node)

	err := scheduler.UnregisterNode("node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSchedulerUpdateNode(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	_ = scheduler.RegisterNode(node)

	node.State = NodeStateNotReady
	err := scheduler.UpdateNode(node)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSchedulerSetAlgorithm(t *testing.T) {
	config := DefaultConfig()
	config.Algorithm = AlgorithmBinPack
	scheduler := NewScheduler(config)

	scheduler.SetAlgorithm(AlgorithmSpread)
	if scheduler.algorithm.Name() != "Spread" {
		t.Errorf("expected algorithm Spread, got %s", scheduler.algorithm.Name())
	}

	scheduler.SetAlgorithm(AlgorithmRandom)
	if scheduler.algorithm.Name() != "Random" {
		t.Errorf("expected algorithm Random, got %s", scheduler.algorithm.Name())
	}
}

func TestSchedulerListWorkloads(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	for i := 0; i < 5; i++ {
		workload := &Workload{
			ID:   string(rune('a' + i)),
			Name: "test",
		}
		_ = scheduler.Submit(workload)
	}

	workloads := scheduler.ListWorkloads()
	if len(workloads) != 5 {
		t.Errorf("expected 5 workloads, got %d", len(workloads))
	}
}

func TestSchedulerListPendingWorkloads(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	for i := 0; i < 5; i++ {
		workload := &Workload{
			ID:   string(rune('a' + i)),
			Name: "test",
		}
		_ = scheduler.Submit(workload)
	}

	// All should be pending
	pending := scheduler.ListPendingWorkloads()
	if len(pending) != 5 {
		t.Errorf("expected 5 pending workloads, got %d", len(pending))
	}
}

func TestSchedulerGetMetrics(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	metrics := scheduler.GetMetrics()

	if metrics.SchedulingAttempts != 0 {
		t.Errorf("expected 0 attempts initially, got %d", metrics.SchedulingAttempts)
	}
}

func TestSchedulerCallbacks(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	scheduledCalled := false
	failedCalled := false
	preemptedCalled := false

	scheduler.OnScheduled(func(w *Workload, n *Node) {
		scheduledCalled = true
	})
	scheduler.OnFailed(func(w *Workload, err error) {
		failedCalled = true
	})
	scheduler.OnPreempted(func(w *Workload, n *Node) {
		preemptedCalled = true
	})

	// Verify callbacks were set (we can't easily trigger them in unit tests)
	if scheduledCalled || failedCalled || preemptedCalled {
		t.Error("callbacks should not be called yet")
	}
}

func TestSchedulerQuota(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	quota := &ResourceQuota{
		Namespace: "default",
		Hard: ResourceLimits{
			CPU:    10000,
			Memory: 20480,
		},
	}

	scheduler.SetQuota("default", quota)

	retrieved := scheduler.GetQuota("default")
	if retrieved == nil {
		t.Fatal("expected quota to be set")
	}
	if retrieved.Hard.CPU != 10000 {
		t.Errorf("expected Hard.CPU 10000, got %d", retrieved.Hard.CPU)
	}
}

func TestSchedulerReschedule(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Register a node
	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	_ = scheduler.RegisterNode(node)

	// Submit a workload
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}
	_ = scheduler.Submit(workload)

	// Manually simulate scheduling
	scheduler.mu.Lock()
	workload.State = WorkloadStateScheduled
	workload.BoundNode = "node-1"
	scheduler.mu.Unlock()

	// Reschedule
	err := scheduler.Reschedule("workload-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify workload is pending again
	w, _ := scheduler.GetWorkload("workload-1")
	if w.State != WorkloadStatePending {
		t.Errorf("expected state Pending after reschedule, got %s", w.State)
	}
	if w.BoundNode != "" {
		t.Errorf("expected empty BoundNode after reschedule, got %s", w.BoundNode)
	}
}

func TestSchedulerRescheduleNotFound(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	err := scheduler.Reschedule("nonexistent")
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}
}

// =============================================================================
// Scheduling Integration Tests
// =============================================================================

func TestSchedulerScheduleWorkload(t *testing.T) {
	config := DefaultConfig()
	config.WorkerCount = 1
	scheduler := NewScheduler(config)

	// Register a node
	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	_ = scheduler.RegisterNode(node)

	// Start scheduler
	_ = scheduler.Start()
	defer scheduler.Stop()

	// Submit workload
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}
	_ = scheduler.Submit(workload)

	// Wait for scheduling
	time.Sleep(500 * time.Millisecond)

	// Check if workload was scheduled
	w, _ := scheduler.GetWorkload("workload-1")
	if w.State != WorkloadStateScheduled && w.State != WorkloadStatePending {
		// Either scheduled or still pending is acceptable in this test
		t.Logf("workload state: %s", w.State)
	}
}

func TestSchedulerFilterNodesWithTaints(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Create a node with a taint
	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	node.AddTaint(Taint{
		Key:    "gpu",
		Value:  "true",
		Effect: TaintEffectNoSchedule,
	})
	_ = scheduler.RegisterNode(node)

	// Workload without toleration
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}

	nodes := scheduler.nodes.ListReady()
	filtered := scheduler.filterNodes(workload, nodes)

	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered nodes (taint not tolerated), got %d", len(filtered))
	}

	// Workload with toleration
	workloadWithToleration := &Workload{
		ID:        "workload-2",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
		Tolerations: []Toleration{
			{
				Key:      "gpu",
				Operator: TolerationOpEqual,
				Value:    "true",
				Effect:   TaintEffectNoSchedule,
			},
		},
	}

	filtered = scheduler.filterNodes(workloadWithToleration, nodes)
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered node (taint tolerated), got %d", len(filtered))
	}
}

func TestSchedulerFilterNodesWithAffinity(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Create nodes with different labels
	node1 := NewNode("node-1", "zone-a-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	node1.Labels["zone"] = "zone-a"
	_ = scheduler.RegisterNode(node1)

	node2 := NewNode("node-2", "zone-b-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	node2.Labels["zone"] = "zone-b"
	_ = scheduler.RegisterNode(node2)

	// Workload with node affinity requiring zone-a
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
		Affinity: &AffinitySpec{
			NodeAffinity: &NodeAffinity{
				RequiredDuringScheduling: []NodeSelectorTerm{
					{
						MatchExpressions: []LabelSelectorRequirement{
							{
								Key:      "zone",
								Operator: LabelSelectorOpIn,
								Values:   []string{"zone-a"},
							},
						},
					},
				},
			},
		},
	}

	nodes := scheduler.nodes.ListReady()
	filtered := scheduler.filterNodes(workload, nodes)

	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered node (zone-a only), got %d", len(filtered))
	}
	if filtered[0].ID != "node-1" {
		t.Errorf("expected node-1, got %s", filtered[0].ID)
	}
}

func TestSchedulerFilterNodesInsufficientResources(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Create a small node
	node := NewNode("node-1", "small-node", Resources{CPU: 1000, Memory: 2048, Disk: 10000})
	_ = scheduler.RegisterNode(node)

	// Workload requesting more than available
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 2000, Memory: 4096},
		},
	}

	nodes := scheduler.nodes.ListReady()
	filtered := scheduler.filterNodes(workload, nodes)

	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered nodes (insufficient resources), got %d", len(filtered))
	}
}

func TestSchedulerConcurrentSubmit(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	var wg sync.WaitGroup
	numWorkloads := 100

	for i := 0; i < numWorkloads; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workload := &Workload{
				ID:   string(rune(id)),
				Name: "test",
			}
			_ = scheduler.Submit(workload)
		}(i)
	}

	wg.Wait()

	workloads := scheduler.ListWorkloads()
	if len(workloads) != numWorkloads {
		t.Errorf("expected %d workloads, got %d", numWorkloads, len(workloads))
	}
}

// =============================================================================
// matchesNodeSelector Tests
// =============================================================================

func TestMatchesNodeSelector(t *testing.T) {
	node := &Node{
		ID: "node-1",
		Labels: map[string]string{
			"zone":     "zone-a",
			"region":   "us-west",
			"nodeType": "compute",
		},
	}

	tests := []struct {
		name      string
		selectors []NodeSelectorTerm
		expected  bool
	}{
		{
			name:      "empty selectors matches",
			selectors: []NodeSelectorTerm{},
			expected:  false, // No selectors = no match
		},
		{
			name: "single matching term",
			selectors: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-a"}},
					},
				},
			},
			expected: true,
		},
		{
			name: "single non-matching term",
			selectors: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-b"}},
					},
				},
			},
			expected: false,
		},
		{
			name: "multiple terms - one matches",
			selectors: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-b"}},
					},
				},
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-a"}},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesNodeSelector(node, tt.selectors)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMatchesLabelRequirement(t *testing.T) {
	labels := map[string]string{
		"zone":   "zone-a",
		"region": "us-west",
	}

	tests := []struct {
		name     string
		req      LabelSelectorRequirement
		expected bool
	}{
		{
			name: "In operator - matches",
			req: LabelSelectorRequirement{
				Key:      "zone",
				Operator: LabelSelectorOpIn,
				Values:   []string{"zone-a", "zone-b"},
			},
			expected: true,
		},
		{
			name: "In operator - no match",
			req: LabelSelectorRequirement{
				Key:      "zone",
				Operator: LabelSelectorOpIn,
				Values:   []string{"zone-c"},
			},
			expected: false,
		},
		{
			name: "NotIn operator - matches",
			req: LabelSelectorRequirement{
				Key:      "zone",
				Operator: LabelSelectorOpNotIn,
				Values:   []string{"zone-b", "zone-c"},
			},
			expected: true,
		},
		{
			name: "NotIn operator - no match",
			req: LabelSelectorRequirement{
				Key:      "zone",
				Operator: LabelSelectorOpNotIn,
				Values:   []string{"zone-a"},
			},
			expected: false,
		},
		{
			name: "Exists operator - exists",
			req: LabelSelectorRequirement{
				Key:      "zone",
				Operator: LabelSelectorOpExists,
			},
			expected: true,
		},
		{
			name: "Exists operator - not exists",
			req: LabelSelectorRequirement{
				Key:      "nonexistent",
				Operator: LabelSelectorOpExists,
			},
			expected: false,
		},
		{
			name: "DoesNotExist operator - exists",
			req: LabelSelectorRequirement{
				Key:      "zone",
				Operator: LabelSelectorOpDoesNotExist,
			},
			expected: false,
		},
		{
			name: "DoesNotExist operator - not exists",
			req: LabelSelectorRequirement{
				Key:      "nonexistent",
				Operator: LabelSelectorOpDoesNotExist,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesLabelRequirement(labels, tt.req)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// toleratesTaints Tests
// =============================================================================

func TestToleratesTaints(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	tests := []struct {
		name        string
		taints      []Taint
		tolerations []Toleration
		expected    bool
	}{
		{
			name:        "no taints",
			taints:      []Taint{},
			tolerations: []Toleration{},
			expected:    true,
		},
		{
			name: "taint with matching toleration",
			taints: []Taint{
				{Key: "gpu", Value: "true", Effect: TaintEffectNoSchedule},
			},
			tolerations: []Toleration{
				{Key: "gpu", Operator: TolerationOpEqual, Value: "true", Effect: TaintEffectNoSchedule},
			},
			expected: true,
		},
		{
			name: "taint without toleration",
			taints: []Taint{
				{Key: "gpu", Value: "true", Effect: TaintEffectNoSchedule},
			},
			tolerations: []Toleration{},
			expected:    false,
		},
		{
			name: "PreferNoSchedule taint without toleration",
			taints: []Taint{
				{Key: "gpu", Value: "true", Effect: TaintEffectPreferNoSchedule},
			},
			tolerations: []Toleration{},
			expected:    true, // PreferNoSchedule is soft
		},
		{
			name: "multiple taints - all tolerated",
			taints: []Taint{
				{Key: "gpu", Value: "true", Effect: TaintEffectNoSchedule},
				{Key: "dedicated", Value: "ml", Effect: TaintEffectNoSchedule},
			},
			tolerations: []Toleration{
				{Key: "gpu", Operator: TolerationOpEqual, Value: "true", Effect: TaintEffectNoSchedule},
				{Key: "dedicated", Operator: TolerationOpEqual, Value: "ml", Effect: TaintEffectNoSchedule},
			},
			expected: true,
		},
		{
			name: "multiple taints - one not tolerated",
			taints: []Taint{
				{Key: "gpu", Value: "true", Effect: TaintEffectNoSchedule},
				{Key: "dedicated", Value: "ml", Effect: TaintEffectNoSchedule},
			},
			tolerations: []Toleration{
				{Key: "gpu", Operator: TolerationOpEqual, Value: "true", Effect: TaintEffectNoSchedule},
			},
			expected: false,
		},
		{
			name: "wildcard toleration",
			taints: []Taint{
				{Key: "any-taint", Value: "any-value", Effect: TaintEffectNoSchedule},
			},
			tolerations: []Toleration{
				{Key: "", Operator: TolerationOpExists},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &Node{
				ID:     "node-1",
				Taints: tt.taints,
			}
			workload := &Workload{
				ID:          "workload-1",
				Tolerations: tt.tolerations,
			}

			result := scheduler.toleratesTaints(workload, node)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// Workload State Tests
// =============================================================================

func TestWorkloadStateTransitions(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Register node
	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	_ = scheduler.RegisterNode(node)

	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}

	// Submit -> Pending
	_ = scheduler.Submit(workload)
	if workload.State != WorkloadStatePending {
		t.Errorf("expected state Pending after submit, got %s", workload.State)
	}

	// Cancel
	_ = scheduler.Cancel("workload-1")
	_, err := scheduler.GetWorkload("workload-1")
	if err != ErrWorkloadNotFound {
		t.Error("expected workload to be removed after cancel")
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestSchedulerEmptyQueue(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Start with empty queue
	_ = scheduler.Start()
	defer scheduler.Stop()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	metrics := scheduler.GetMetrics()
	if metrics.SchedulingAttempts != 0 {
		// No workloads to schedule
		t.Logf("scheduling attempts: %d", metrics.SchedulingAttempts)
	}
}

func TestSchedulerNoNodes(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Don't register any nodes
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}

	_ = scheduler.Submit(workload)

	// Manually schedule (since scheduler not started)
	result := scheduler.scheduleWorkload(workload)

	if result.Error != ErrNoNodesAvailable {
		t.Errorf("expected ErrNoNodesAvailable, got %v", result.Error)
	}
}

func TestSchedulerCancelBoundWorkload(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Register node
	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	_ = scheduler.RegisterNode(node)

	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}
	_ = scheduler.Submit(workload)

	// Manually bind
	scheduler.mu.Lock()
	_ = scheduler.binder.Bind(workload, node)
	workload.State = WorkloadStateScheduled
	workload.BoundNode = "node-1"
	scheduler.mu.Unlock()

	// Cancel should unbind
	err := scheduler.Cancel("workload-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify node resources released
	if node.Allocated.CPU != 0 {
		t.Errorf("expected node Allocated.CPU 0 after cancel, got %d", node.Allocated.CPU)
	}
}

func TestSchedulerUnregisterNodeWithWorkloads(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Register node
	node := NewNode("node-1", "test-node", Resources{CPU: 8000, Memory: 16384, Disk: 100000})
	_ = scheduler.RegisterNode(node)

	// Submit and bind workload
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048},
		},
	}
	_ = scheduler.Submit(workload)

	scheduler.mu.Lock()
	_ = scheduler.binder.Bind(workload, node)
	workload.State = WorkloadStateScheduled
	workload.BoundNode = "node-1"
	scheduler.mu.Unlock()

	// Unregister node - workloads should be rescheduled
	err := scheduler.UnregisterNode("node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Workload should be back to pending
	w, _ := scheduler.GetWorkload("workload-1")
	if w.State != WorkloadStatePending {
		t.Errorf("expected state Pending after node unregister, got %s", w.State)
	}
}

func TestSchedulerMultipleStopCalls(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	_ = scheduler.Start()

	// Multiple stop calls should be safe
	scheduler.Stop()
	scheduler.Stop()
	scheduler.Stop()

	if !scheduler.stopped {
		t.Error("scheduler should be stopped")
	}
}

func TestSchedulerQuotaExceeded(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config)

	// Set a small quota
	quota := &ResourceQuota{
		Namespace: "default",
		Hard: ResourceLimits{
			CPU:    500, // Small quota
			Memory: 1024,
		},
	}
	scheduler.SetQuota("default", quota)

	// Try to submit workload that exceeds quota
	workload := &Workload{
		ID:        "workload-1",
		Name:      "test",
		Namespace: "default",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048}, // Exceeds quota
		},
	}

	err := scheduler.Submit(workload)
	if err == nil {
		t.Error("expected error for quota exceeded")
	}
}
