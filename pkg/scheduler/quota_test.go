// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package scheduler

import (
	"sync"
	"testing"
)

// =============================================================================
// ResourceQuota Tests
// =============================================================================

func TestResourceQuota_NewResourceQuota(t *testing.T) {
	hard := ResourceLimits{
		CPU:       4000,
		Memory:    8 * 1024 * 1024 * 1024,
		Workloads: 10,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	if quota == nil {
		t.Fatal("expected non-nil quota")
	}
	if quota.Name != "test-quota" {
		t.Errorf("expected name test-quota, got %s", quota.Name)
	}
	if quota.Namespace != "default" {
		t.Errorf("expected namespace default, got %s", quota.Namespace)
	}
	if quota.Hard.CPU != 4000 {
		t.Errorf("expected hard CPU 4000, got %d", quota.Hard.CPU)
	}
}

func TestResourceQuota_HasCapacity(t *testing.T) {
	hard := ResourceLimits{
		CPU:       4000,
		Memory:    8 * 1024 * 1024 * 1024,
		Workloads: 10,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Should have capacity initially
	requested := Resources{CPU: 1000, Memory: 1 * 1024 * 1024 * 1024}
	if !quota.HasCapacity(requested, 1) {
		t.Error("expected to have capacity")
	}
}

func TestResourceQuota_HasCapacity_Exceeded(t *testing.T) {
	hard := ResourceLimits{
		CPU:       1000,
		Memory:    1 * 1024 * 1024 * 1024,
		Workloads: 2,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Use up some capacity
	quota.Used.CPU = 800
	quota.Used.Memory = 800 * 1024 * 1024
	quota.Used.Workloads = 1

	// Request that exceeds remaining capacity
	requested := Resources{CPU: 500, Memory: 500 * 1024 * 1024}
	if quota.HasCapacity(requested, 1) {
		t.Error("expected no capacity (CPU exceeded)")
	}
}

func TestResourceQuota_HasCapacity_WorkloadLimit(t *testing.T) {
	hard := ResourceLimits{
		CPU:       10000,
		Memory:    10 * 1024 * 1024 * 1024,
		Workloads: 2,
	}

	quota := NewResourceQuota("test-quota", "default", hard)
	quota.Used.Workloads = 2

	// Even small request should fail due to workload limit
	requested := Resources{CPU: 100, Memory: 100 * 1024 * 1024}
	if quota.HasCapacity(requested, 1) {
		t.Error("expected no capacity (workload limit reached)")
	}
}

func TestResourceQuota_Reserve(t *testing.T) {
	hard := ResourceLimits{
		CPU:       4000,
		Memory:    8 * 1024 * 1024 * 1024,
		Workloads: 10,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	requested := Resources{CPU: 1000, Memory: 2 * 1024 * 1024 * 1024}
	err := quota.Reserve(requested)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if quota.Used.CPU != 1000 {
		t.Errorf("expected used CPU 1000, got %d", quota.Used.CPU)
	}
	if quota.Used.Workloads != 1 {
		t.Errorf("expected used workloads 1, got %d", quota.Used.Workloads)
	}
}

func TestResourceQuota_Reserve_Exceeded(t *testing.T) {
	hard := ResourceLimits{
		CPU:       1000,
		Memory:    1 * 1024 * 1024 * 1024,
		Workloads: 10,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Request exceeding capacity
	requested := Resources{CPU: 2000, Memory: 2 * 1024 * 1024 * 1024}
	err := quota.Reserve(requested)

	if err != ErrQuotaExceeded {
		t.Errorf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestResourceQuota_Release(t *testing.T) {
	hard := ResourceLimits{
		CPU:       4000,
		Memory:    8 * 1024 * 1024 * 1024,
		Workloads: 10,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Reserve some resources
	requested := Resources{CPU: 1000, Memory: 2 * 1024 * 1024 * 1024}
	quota.Reserve(requested)

	// Release them
	quota.Release(requested)

	if quota.Used.CPU != 0 {
		t.Errorf("expected used CPU 0, got %d", quota.Used.CPU)
	}
	if quota.Used.Workloads != 0 {
		t.Errorf("expected used workloads 0, got %d", quota.Used.Workloads)
	}
}

func TestResourceQuota_Release_NoNegative(t *testing.T) {
	hard := ResourceLimits{
		CPU:       4000,
		Memory:    8 * 1024 * 1024 * 1024,
		Workloads: 10,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Release more than used (should clamp to 0)
	quota.Release(Resources{CPU: 1000, Memory: 1 * 1024 * 1024 * 1024})

	if quota.Used.CPU < 0 {
		t.Error("used CPU should not be negative")
	}
	if quota.Used.Memory < 0 {
		t.Error("used memory should not be negative")
	}
	if quota.Used.Workloads < 0 {
		t.Error("used workloads should not be negative")
	}
}

func TestResourceQuota_UtilizationRatio(t *testing.T) {
	hard := ResourceLimits{
		CPU:    1000,
		Memory: 1000 * 1024 * 1024,
	}

	quota := NewResourceQuota("test-quota", "default", hard)
	quota.Used.CPU = 500
	quota.Used.Memory = 500 * 1024 * 1024

	ratio := quota.UtilizationRatio()
	if ratio != 0.5 {
		t.Errorf("expected ratio 0.5, got %f", ratio)
	}
}

func TestResourceQuota_UtilizationRatio_ZeroHard(t *testing.T) {
	hard := ResourceLimits{
		CPU:    0,
		Memory: 0,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	ratio := quota.UtilizationRatio()
	if ratio != 0 {
		t.Errorf("expected ratio 0 for zero hard limits, got %f", ratio)
	}
}

// =============================================================================
// QuotaManager Tests
// =============================================================================

func TestQuotaManager_NewQuotaManager(t *testing.T) {
	qm := NewQuotaManager()
	if qm == nil {
		t.Fatal("expected non-nil quota manager")
	}
}

func TestQuotaManager_SetQuota(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU: 4000,
	})

	qm.SetQuota("default", quota)

	retrieved := qm.GetQuota("default")
	if retrieved == nil {
		t.Error("expected to get quota")
	}
	if retrieved.Hard.CPU != 4000 {
		t.Errorf("expected CPU 4000, got %d", retrieved.Hard.CPU)
	}
}

func TestQuotaManager_GetQuota_NotFound(t *testing.T) {
	qm := NewQuotaManager()

	quota := qm.GetQuota("nonexistent")
	if quota != nil {
		t.Error("expected nil for nonexistent namespace")
	}
}

func TestQuotaManager_DeleteQuota(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU: 4000,
	})
	qm.SetQuota("default", quota)

	qm.DeleteQuota("default")

	if qm.GetQuota("default") != nil {
		t.Error("expected quota to be deleted")
	}
}

func TestQuotaManager_ListQuotas(t *testing.T) {
	qm := NewQuotaManager()

	qm.SetQuota("ns1", NewResourceQuota("q1", "ns1", ResourceLimits{CPU: 1000}))
	qm.SetQuota("ns2", NewResourceQuota("q2", "ns2", ResourceLimits{CPU: 2000}))

	quotas := qm.ListQuotas()
	if len(quotas) != 2 {
		t.Errorf("expected 2 quotas, got %d", len(quotas))
	}
}

func TestQuotaManager_CheckQuota(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:       1000,
		Memory:    1 * 1024 * 1024 * 1024,
		Workloads: 5,
	})
	qm.SetQuota("default", quota)

	// Should pass
	err := qm.CheckQuota("default", Resources{CPU: 500, Memory: 500 * 1024 * 1024})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Should fail
	err = qm.CheckQuota("default", Resources{CPU: 2000, Memory: 2 * 1024 * 1024 * 1024})
	if err != ErrQuotaExceeded {
		t.Errorf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestQuotaManager_CheckQuota_NoQuota(t *testing.T) {
	qm := NewQuotaManager()

	// No quota means unlimited
	err := qm.CheckQuota("no-quota-ns", Resources{CPU: 999999, Memory: 999999 * 1024 * 1024})
	if err != nil {
		t.Errorf("expected no error when no quota exists, got %v", err)
	}
}

func TestQuotaManager_ReserveQuota(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:       1000,
		Memory:    1 * 1024 * 1024 * 1024,
		Workloads: 5,
	})
	qm.SetQuota("default", quota)

	err := qm.ReserveQuota("default", Resources{CPU: 500, Memory: 500 * 1024 * 1024})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved := qm.GetQuota("default")
	if retrieved.Used.CPU != 500 {
		t.Errorf("expected used CPU 500, got %d", retrieved.Used.CPU)
	}
}

func TestQuotaManager_ReserveQuota_NoQuota(t *testing.T) {
	qm := NewQuotaManager()

	// Should succeed when no quota exists
	err := qm.ReserveQuota("no-quota-ns", Resources{CPU: 500})
	if err != nil {
		t.Errorf("expected no error when no quota exists, got %v", err)
	}
}

func TestQuotaManager_ReleaseQuota(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:       1000,
		Memory:    1 * 1024 * 1024 * 1024,
		Workloads: 5,
	})
	qm.SetQuota("default", quota)

	qm.ReserveQuota("default", Resources{CPU: 500})
	qm.ReleaseQuota("default", Resources{CPU: 500})

	retrieved := qm.GetQuota("default")
	if retrieved.Used.CPU != 0 {
		t.Errorf("expected used CPU 0, got %d", retrieved.Used.CPU)
	}
}

func TestQuotaManager_ReleaseQuota_NoQuota(t *testing.T) {
	qm := NewQuotaManager()

	// Should not panic when no quota exists
	qm.ReleaseQuota("no-quota-ns", Resources{CPU: 500})
}

func TestQuotaManager_UpdateQuota(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU: 1000,
	})
	qm.SetQuota("default", quota)

	err := qm.UpdateQuota("default", ResourceLimits{CPU: 2000})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved := qm.GetQuota("default")
	if retrieved.Hard.CPU != 2000 {
		t.Errorf("expected hard CPU 2000, got %d", retrieved.Hard.CPU)
	}
}

func TestQuotaManager_UpdateQuota_NotFound(t *testing.T) {
	qm := NewQuotaManager()

	err := qm.UpdateQuota("nonexistent", ResourceLimits{CPU: 2000})
	if err == nil {
		t.Error("expected error for nonexistent namespace")
	}
}

func TestQuotaManager_GetUsage(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU: 1000,
	})
	qm.SetQuota("default", quota)
	qm.ReserveQuota("default", Resources{CPU: 500})

	usage, err := qm.GetUsage("default")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usage.CPU != 500 {
		t.Errorf("expected usage CPU 500, got %d", usage.CPU)
	}
}

func TestQuotaManager_GetUsage_NotFound(t *testing.T) {
	qm := NewQuotaManager()

	_, err := qm.GetUsage("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent namespace")
	}
}

func TestQuotaManager_SetScopes(t *testing.T) {
	qm := NewQuotaManager()

	scopes := []QuotaScope{QuotaScopeTerminating, QuotaScopeBestEffort}
	qm.SetScopes("default", scopes)

	retrieved := qm.GetScopes("default")
	if len(retrieved) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(retrieved))
	}
}

func TestQuotaManager_GetQuotaStatus(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:    1000,
		Memory: 1000 * 1024 * 1024,
	})
	qm.SetQuota("default", quota)

	// Use 50%
	qm.ReserveQuota("default", Resources{CPU: 500, Memory: 500 * 1024 * 1024})

	status := qm.GetQuotaStatus("default")
	if status == nil {
		t.Fatal("expected status")
	}
	if status.Status != QuotaStatusOK {
		t.Errorf("expected OK status, got %s", status.Status)
	}
}

func TestQuotaManager_GetQuotaStatus_Warning(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:    1000,
		Memory: 1000 * 1024 * 1024,
	})
	qm.SetQuota("default", quota)

	// Use 80%
	qm.ReserveQuota("default", Resources{CPU: 800, Memory: 800 * 1024 * 1024})

	status := qm.GetQuotaStatus("default")
	if status.Status != QuotaStatusWarning {
		t.Errorf("expected Warning status at 80%%, got %s", status.Status)
	}
}

func TestQuotaManager_GetQuotaStatus_Critical(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:    1000,
		Memory: 1000 * 1024 * 1024,
	})
	qm.SetQuota("default", quota)

	// Use 95%
	qm.ReserveQuota("default", Resources{CPU: 950, Memory: 950 * 1024 * 1024})

	status := qm.GetQuotaStatus("default")
	if status.Status != QuotaStatusCritical {
		t.Errorf("expected Critical status at 95%%, got %s", status.Status)
	}
}

func TestQuotaManager_ListQuotaStatuses(t *testing.T) {
	qm := NewQuotaManager()

	qm.SetQuota("ns1", NewResourceQuota("q1", "ns1", ResourceLimits{CPU: 1000}))
	qm.SetQuota("ns2", NewResourceQuota("q2", "ns2", ResourceLimits{CPU: 1000}))

	statuses := qm.ListQuotaStatuses()
	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}
}

func TestQuotaManager_ConcurrentAccess(t *testing.T) {
	qm := NewQuotaManager()

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:       100000,
		Memory:    100 * 1024 * 1024 * 1024,
		Workloads: 10000,
	})
	qm.SetQuota("default", quota)

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			switch i % 4 {
			case 0:
				qm.CheckQuota("default", Resources{CPU: 100})
			case 1:
				qm.ReserveQuota("default", Resources{CPU: 10})
			case 2:
				qm.ReleaseQuota("default", Resources{CPU: 10})
			case 3:
				qm.GetQuotaStatus("default")
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// LimitRange Tests
// =============================================================================

func TestLimitRangeManager_NewLimitRangeManager(t *testing.T) {
	lrm := NewLimitRangeManager()
	if lrm == nil {
		t.Fatal("expected non-nil limit range manager")
	}
}

func TestLimitRangeManager_SetLimitRange(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{
		Name:      "test-lr",
		Namespace: "default",
		Items: []LimitRangeItem{
			{
				Type:           LimitTypeWorkload,
				Min:            Resources{CPU: 100, Memory: 128 * 1024 * 1024},
				Max:            Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
				Default:        Resources{CPU: 500, Memory: 512 * 1024 * 1024},
				DefaultRequest: Resources{CPU: 250, Memory: 256 * 1024 * 1024},
			},
		},
	}

	lrm.SetLimitRange("default", lr)

	retrieved := lrm.GetLimitRange("default")
	if retrieved == nil {
		t.Error("expected to get limit range")
	}
}

func TestLimitRangeManager_DeleteLimitRange(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{Name: "test-lr", Namespace: "default"}
	lrm.SetLimitRange("default", lr)

	lrm.DeleteLimitRange("default")

	if lrm.GetLimitRange("default") != nil {
		t.Error("expected limit range to be deleted")
	}
}

func TestLimitRangeManager_ValidateWorkload_NoLimitRange(t *testing.T) {
	lrm := NewLimitRangeManager()

	w := createTestWorkload("w1", 100, 100*1024*1024)

	err := lrm.ValidateWorkload(w)
	if err != nil {
		t.Errorf("expected no error when no limit range, got %v", err)
	}
}

func TestLimitRangeManager_ValidateWorkload_BelowMin(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{
		Name:      "test-lr",
		Namespace: "default",
		Items: []LimitRangeItem{
			{
				Type: LimitTypeWorkload,
				Min:  Resources{CPU: 500, Memory: 512 * 1024 * 1024},
			},
		},
	}
	lrm.SetLimitRange("default", lr)

	w := createTestWorkload("w1", 100, 100*1024*1024) // Below min
	w.Namespace = "default"

	err := lrm.ValidateWorkload(w)
	if err == nil {
		t.Error("expected error for below minimum")
	}
}

func TestLimitRangeManager_ValidateWorkload_AboveMax(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{
		Name:      "test-lr",
		Namespace: "default",
		Items: []LimitRangeItem{
			{
				Type: LimitTypeWorkload,
				Max:  Resources{CPU: 1000, Memory: 1 * 1024 * 1024 * 1024},
			},
		},
	}
	lrm.SetLimitRange("default", lr)

	w := createTestWorkload("w1", 2000, 2*1024*1024*1024) // Above max
	w.Namespace = "default"

	err := lrm.ValidateWorkload(w)
	if err == nil {
		t.Error("expected error for above maximum")
	}
}

func TestLimitRangeManager_ValidateWorkload_Valid(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{
		Name:      "test-lr",
		Namespace: "default",
		Items: []LimitRangeItem{
			{
				Type: LimitTypeWorkload,
				Min:  Resources{CPU: 100, Memory: 128 * 1024 * 1024},
				Max:  Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
			},
		},
	}
	lrm.SetLimitRange("default", lr)

	w := createTestWorkload("w1", 500, 512*1024*1024) // Within range
	w.Namespace = "default"

	err := lrm.ValidateWorkload(w)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestLimitRangeManager_ApplyDefaults(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{
		Name:      "test-lr",
		Namespace: "default",
		Items: []LimitRangeItem{
			{
				Type:           LimitTypeWorkload,
				Default:        Resources{CPU: 500, Memory: 512 * 1024 * 1024},
				DefaultRequest: Resources{CPU: 250, Memory: 256 * 1024 * 1024},
			},
		},
	}
	lrm.SetLimitRange("default", lr)

	w := createTestWorkload("w1", 0, 0) // No resources specified
	w.Namespace = "default"
	w.Resources.Requests = Resources{}
	w.Resources.Limits = Resources{}

	lrm.ApplyDefaults(w)

	if w.Resources.Requests.CPU != 250 {
		t.Errorf("expected default request CPU 250, got %d", w.Resources.Requests.CPU)
	}
	if w.Resources.Limits.CPU != 500 {
		t.Errorf("expected default limit CPU 500, got %d", w.Resources.Limits.CPU)
	}
}

func TestLimitRangeManager_ApplyDefaults_ExistingValues(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{
		Name:      "test-lr",
		Namespace: "default",
		Items: []LimitRangeItem{
			{
				Type:           LimitTypeWorkload,
				Default:        Resources{CPU: 500, Memory: 512 * 1024 * 1024},
				DefaultRequest: Resources{CPU: 250, Memory: 256 * 1024 * 1024},
			},
		},
	}
	lrm.SetLimitRange("default", lr)

	w := createTestWorkload("w1", 1000, 1*1024*1024*1024) // Has existing values
	w.Namespace = "default"

	lrm.ApplyDefaults(w)

	// Should not override existing values
	if w.Resources.Requests.CPU != 1000 {
		t.Errorf("expected existing request CPU 1000, got %d", w.Resources.Requests.CPU)
	}
}

// =============================================================================
// PriorityClassManager Tests
// =============================================================================

func TestPriorityClassManager_NewPriorityClassManager(t *testing.T) {
	pcm := NewPriorityClassManager()
	if pcm == nil {
		t.Fatal("expected non-nil priority class manager")
	}

	// Should have system classes
	classes := pcm.ListClasses()
	if len(classes) < 2 {
		t.Error("expected at least 2 system priority classes")
	}
}

func TestPriorityClassManager_AddClass(t *testing.T) {
	pcm := NewPriorityClassManager()

	class := &PriorityClass{
		Name:             "high-priority",
		Value:            1000,
		Description:      "High priority workloads",
		PreemptionPolicy: PreemptionPolicyPreemptLowerPriority,
	}

	err := pcm.AddClass(class)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved, exists := pcm.GetClass("high-priority")
	if !exists {
		t.Error("expected class to exist")
	}
	if retrieved.Value != 1000 {
		t.Errorf("expected value 1000, got %d", retrieved.Value)
	}
}

func TestPriorityClassManager_AddClass_Duplicate(t *testing.T) {
	pcm := NewPriorityClassManager()

	class := &PriorityClass{Name: "test", Value: 100}
	pcm.AddClass(class)

	err := pcm.AddClass(class)
	if err == nil {
		t.Error("expected error for duplicate class")
	}
}

func TestPriorityClassManager_AddClass_GlobalDefault(t *testing.T) {
	pcm := NewPriorityClassManager()

	class := &PriorityClass{
		Name:          "default-class",
		Value:         0,
		GlobalDefault: true,
	}

	pcm.AddClass(class)

	defaultClass := pcm.GetDefaultClass()
	if defaultClass == nil {
		t.Error("expected default class to be set")
	}
	if defaultClass.Name != "default-class" {
		t.Errorf("expected default-class, got %s", defaultClass.Name)
	}
}

func TestPriorityClassManager_DeleteClass(t *testing.T) {
	pcm := NewPriorityClassManager()

	class := &PriorityClass{Name: "deleteme", Value: 100}
	pcm.AddClass(class)

	err := pcm.DeleteClass("deleteme")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, exists := pcm.GetClass("deleteme")
	if exists {
		t.Error("expected class to be deleted")
	}
}

func TestPriorityClassManager_DeleteClass_SystemClass(t *testing.T) {
	pcm := NewPriorityClassManager()

	err := pcm.DeleteClass("system-critical")
	if err == nil {
		t.Error("expected error when deleting system class")
	}
}

func TestPriorityClassManager_GetPriority(t *testing.T) {
	pcm := NewPriorityClassManager()

	class := &PriorityClass{Name: "test", Value: 500}
	pcm.AddClass(class)

	priority := pcm.GetPriority("test")
	if priority != 500 {
		t.Errorf("expected 500, got %d", priority)
	}
}

func TestPriorityClassManager_GetPriority_NotFound(t *testing.T) {
	pcm := NewPriorityClassManager()

	priority := pcm.GetPriority("nonexistent")
	if priority != 0 {
		t.Errorf("expected 0 for nonexistent, got %d", priority)
	}
}

func TestPriorityClassManager_GetPriority_UsesDefault(t *testing.T) {
	pcm := NewPriorityClassManager()

	class := &PriorityClass{
		Name:          "default-class",
		Value:         100,
		GlobalDefault: true,
	}
	pcm.AddClass(class)

	// Should use default for unknown class
	priority := pcm.GetPriority("unknown")
	if priority != 100 {
		t.Errorf("expected default value 100, got %d", priority)
	}
}

func TestPriorityClassManager_ConcurrentAccess(t *testing.T) {
	pcm := NewPriorityClassManager()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			className := "class-" + string(rune('a'+i%26))

			switch i % 3 {
			case 0:
				pcm.AddClass(&PriorityClass{Name: className, Value: int32(i)})
			case 1:
				pcm.GetClass(className)
			case 2:
				pcm.GetPriority(className)
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// QuotaEnforcer Tests
// =============================================================================

func TestQuotaEnforcer_NewQuotaEnforcer(t *testing.T) {
	qm := NewQuotaManager()
	qe := NewQuotaEnforcer(qm)

	if qe == nil {
		t.Fatal("expected non-nil quota enforcer")
	}
	if !qe.IsEnabled() {
		t.Error("expected enforcer to be enabled by default")
	}
}

func TestQuotaEnforcer_SetEnabled(t *testing.T) {
	qm := NewQuotaManager()
	qe := NewQuotaEnforcer(qm)

	qe.SetEnabled(false)
	if qe.IsEnabled() {
		t.Error("expected enforcer to be disabled")
	}

	qe.SetEnabled(true)
	if !qe.IsEnabled() {
		t.Error("expected enforcer to be enabled")
	}
}

func TestQuotaEnforcer_Enforce(t *testing.T) {
	qm := NewQuotaManager()
	qe := NewQuotaEnforcer(qm)

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU: 1000,
	})
	qm.SetQuota("default", quota)

	w := createTestWorkload("w1", 500, 500*1024*1024)
	w.Namespace = "default"

	err := qe.Enforce(w)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestQuotaEnforcer_Enforce_Exceeded(t *testing.T) {
	qm := NewQuotaManager()
	qe := NewQuotaEnforcer(qm)

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU: 1000,
	})
	qm.SetQuota("default", quota)

	w := createTestWorkload("w1", 2000, 500*1024*1024) // Exceeds quota
	w.Namespace = "default"

	err := qe.Enforce(w)
	if err != ErrQuotaExceeded {
		t.Errorf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestQuotaEnforcer_Enforce_Disabled(t *testing.T) {
	qm := NewQuotaManager()
	qe := NewQuotaEnforcer(qm)

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU: 1000,
	})
	qm.SetQuota("default", quota)

	qe.SetEnabled(false)

	w := createTestWorkload("w1", 2000, 500*1024*1024) // Exceeds quota
	w.Namespace = "default"

	err := qe.Enforce(w)
	if err != nil {
		t.Errorf("expected no error when disabled, got %v", err)
	}
}

// =============================================================================
// QuotaReconciler Tests
// =============================================================================

func TestQuotaReconciler_NewQuotaReconciler(t *testing.T) {
	qm := NewQuotaManager()
	qr := NewQuotaReconciler(qm)

	if qr == nil {
		t.Fatal("expected non-nil quota reconciler")
	}
}

func TestQuotaReconciler_Reconcile_NoDiscrepancies(t *testing.T) {
	qm := NewQuotaManager()
	qr := NewQuotaReconciler(qm)

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:       4000,
		Workloads: 10,
	})
	qm.SetQuota("default", quota)

	// Reserve resources for workloads
	qm.ReserveQuota("default", Resources{CPU: 500})
	qm.ReserveQuota("default", Resources{CPU: 300})

	// Create matching workloads
	workloads := []*Workload{
		{ID: "w1", Namespace: "default", State: WorkloadStateScheduled, Resources: ResourceRequirements{Requests: Resources{CPU: 500}}},
		{ID: "w2", Namespace: "default", State: WorkloadStateScheduled, Resources: ResourceRequirements{Requests: Resources{CPU: 300}}},
	}

	result := qr.Reconcile(workloads)

	if len(result.Discrepancies) != 0 {
		t.Errorf("expected no discrepancies, got %v", result.Discrepancies)
	}
}

func TestQuotaReconciler_Reconcile_WithDiscrepancies(t *testing.T) {
	qm := NewQuotaManager()
	qr := NewQuotaReconciler(qm)

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:       4000,
		Workloads: 10,
	})
	// Set incorrect usage
	quota.Used.CPU = 1000
	quota.Used.Workloads = 5
	qm.SetQuota("default", quota)

	// Actual workloads have different usage
	workloads := []*Workload{
		{ID: "w1", Namespace: "default", State: WorkloadStateScheduled, Resources: ResourceRequirements{Requests: Resources{CPU: 500}}},
	}

	result := qr.Reconcile(workloads)

	if len(result.Discrepancies) == 0 {
		t.Error("expected discrepancies")
	}

	// Check that quota was fixed
	updated := qm.GetQuota("default")
	if updated.Used.CPU != 500 {
		t.Errorf("expected reconciled CPU 500, got %d", updated.Used.CPU)
	}
	if updated.Used.Workloads != 1 {
		t.Errorf("expected reconciled workloads 1, got %d", updated.Used.Workloads)
	}
}

func TestQuotaReconciler_Reconcile_IgnoresNonScheduledWorkloads(t *testing.T) {
	qm := NewQuotaManager()
	qr := NewQuotaReconciler(qm)

	quota := NewResourceQuota("test-quota", "default", ResourceLimits{
		CPU:       4000,
		Workloads: 10,
	})
	qm.SetQuota("default", quota)

	workloads := []*Workload{
		{ID: "w1", Namespace: "default", State: WorkloadStateScheduled, Resources: ResourceRequirements{Requests: Resources{CPU: 500}}},
		{ID: "w2", Namespace: "default", State: WorkloadStatePending, Resources: ResourceRequirements{Requests: Resources{CPU: 300}}},
		{ID: "w3", Namespace: "default", State: WorkloadStateFailed, Resources: ResourceRequirements{Requests: Resources{CPU: 200}}},
	}

	result := qr.Reconcile(workloads)

	// Only w1 should count
	updated := qm.GetQuota("default")
	if updated.Used.CPU != 500 {
		t.Errorf("expected only scheduled workload counted, got CPU %d", updated.Used.CPU)
	}

	_ = result
}

// =============================================================================
// GPUs and Ephemeral Storage Tests
// =============================================================================

func TestResourceQuota_GPUs(t *testing.T) {
	hard := ResourceLimits{
		GPUs: 4,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Reserve GPUs
	err := quota.Reserve(Resources{GPUs: 2})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if quota.Used.GPUs != 2 {
		t.Errorf("expected 2 GPUs used, got %d", quota.Used.GPUs)
	}

	// Try to exceed
	if quota.HasCapacity(Resources{GPUs: 3}, 1) {
		t.Error("expected no capacity for 3 more GPUs")
	}
}

func TestResourceQuota_EphemeralStorage(t *testing.T) {
	hard := ResourceLimits{
		Ephemeral: 100 * 1024 * 1024 * 1024, // 100GB
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Reserve ephemeral storage
	err := quota.Reserve(Resources{Ephemeral: 50 * 1024 * 1024 * 1024})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if quota.Used.Ephemeral != 50*1024*1024*1024 {
		t.Errorf("expected 50GB ephemeral used, got %d", quota.Used.Ephemeral)
	}

	// Try to exceed
	if quota.HasCapacity(Resources{Ephemeral: 60 * 1024 * 1024 * 1024}, 1) {
		t.Error("expected no capacity for 60GB more")
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestResourceQuota_AllResourceTypes(t *testing.T) {
	hard := ResourceLimits{
		CPU:       4000,
		Memory:    8 * 1024 * 1024 * 1024,
		Disk:      100 * 1024 * 1024 * 1024,
		GPUs:      4,
		Ephemeral: 50 * 1024 * 1024 * 1024,
		Workloads: 10,
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Reserve all resource types
	err := quota.Reserve(Resources{
		CPU:       1000,
		Memory:    2 * 1024 * 1024 * 1024,
		Disk:      25 * 1024 * 1024 * 1024,
		GPUs:      1,
		Ephemeral: 10 * 1024 * 1024 * 1024,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify all were updated
	if quota.Used.CPU != 1000 {
		t.Errorf("expected CPU 1000, got %d", quota.Used.CPU)
	}
	if quota.Used.GPUs != 1 {
		t.Errorf("expected GPUs 1, got %d", quota.Used.GPUs)
	}
}

func TestResourceQuota_ZeroLimits(t *testing.T) {
	// Zero limit means unlimited
	hard := ResourceLimits{
		CPU:       0, // Unlimited
		Memory:    0, // Unlimited
		Workloads: 0, // Unlimited
	}

	quota := NewResourceQuota("test-quota", "default", hard)

	// Should have capacity for anything
	if !quota.HasCapacity(Resources{CPU: 999999, Memory: 999999 * 1024 * 1024}, 9999) {
		t.Error("expected unlimited capacity when limits are 0")
	}
}

func TestLimitRange_ContainerType(t *testing.T) {
	lrm := NewLimitRangeManager()

	lr := &LimitRange{
		Name:      "test-lr",
		Namespace: "default",
		Items: []LimitRangeItem{
			{
				Type: LimitTypeContainer, // Not workload
				Min:  Resources{CPU: 1000},
			},
		},
	}
	lrm.SetLimitRange("default", lr)

	// Workload validation should skip container type items
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "default"

	err := lrm.ValidateWorkload(w)
	if err != nil {
		t.Errorf("expected no error for container type limit, got %v", err)
	}
}
