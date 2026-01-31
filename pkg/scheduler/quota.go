package scheduler

import (
	"fmt"
	"sync"
	"time"
)

// ResourceQuota defines resource limits for a namespace.
type ResourceQuota struct {
	Name        string
	Namespace   string
	Hard        ResourceLimits
	Used        ResourceLimits
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Annotations map[string]string
}

// ResourceLimits defines resource limits.
type ResourceLimits struct {
	CPU              int64 // millicores
	Memory           int64 // bytes
	Disk             int64 // bytes
	GPUs             int32
	Workloads        int   // max number of workloads
	PriorityClasses  map[string]int // limits per priority class
	Ephemeral        int64 // ephemeral storage
}

// NewResourceQuota creates a new resource quota.
func NewResourceQuota(name, namespace string, hard ResourceLimits) *ResourceQuota {
	return &ResourceQuota{
		Name:        name,
		Namespace:   namespace,
		Hard:        hard,
		Used:        ResourceLimits{PriorityClasses: make(map[string]int)},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Annotations: make(map[string]string),
	}
}

// HasCapacity checks if quota has capacity for resources.
func (q *ResourceQuota) HasCapacity(requested Resources, workloadCount int) bool {
	if q.Hard.CPU > 0 && q.Used.CPU+requested.CPU > q.Hard.CPU {
		return false
	}
	if q.Hard.Memory > 0 && q.Used.Memory+requested.Memory > q.Hard.Memory {
		return false
	}
	if q.Hard.Disk > 0 && q.Used.Disk+requested.Disk > q.Hard.Disk {
		return false
	}
	if q.Hard.GPUs > 0 && q.Used.GPUs+requested.GPUs > q.Hard.GPUs {
		return false
	}
	if q.Hard.Workloads > 0 && q.Used.Workloads+workloadCount > q.Hard.Workloads {
		return false
	}
	if q.Hard.Ephemeral > 0 && q.Used.Ephemeral+requested.Ephemeral > q.Hard.Ephemeral {
		return false
	}
	return true
}

// Reserve reserves resources against the quota.
func (q *ResourceQuota) Reserve(resources Resources) error {
	if !q.HasCapacity(resources, 1) {
		return ErrQuotaExceeded
	}

	q.Used.CPU += resources.CPU
	q.Used.Memory += resources.Memory
	q.Used.Disk += resources.Disk
	q.Used.GPUs += resources.GPUs
	q.Used.Ephemeral += resources.Ephemeral
	q.Used.Workloads++
	q.UpdatedAt = time.Now()

	return nil
}

// Release releases resources from the quota.
func (q *ResourceQuota) Release(resources Resources) {
	q.Used.CPU -= resources.CPU
	q.Used.Memory -= resources.Memory
	q.Used.Disk -= resources.Disk
	q.Used.GPUs -= resources.GPUs
	q.Used.Ephemeral -= resources.Ephemeral
	q.Used.Workloads--

	// Ensure non-negative
	if q.Used.CPU < 0 {
		q.Used.CPU = 0
	}
	if q.Used.Memory < 0 {
		q.Used.Memory = 0
	}
	if q.Used.Disk < 0 {
		q.Used.Disk = 0
	}
	if q.Used.GPUs < 0 {
		q.Used.GPUs = 0
	}
	if q.Used.Ephemeral < 0 {
		q.Used.Ephemeral = 0
	}
	if q.Used.Workloads < 0 {
		q.Used.Workloads = 0
	}

	q.UpdatedAt = time.Now()
}

// UtilizationRatio returns the resource utilization ratio.
func (q *ResourceQuota) UtilizationRatio() float64 {
	var cpuRatio, memRatio float64

	if q.Hard.CPU > 0 {
		cpuRatio = float64(q.Used.CPU) / float64(q.Hard.CPU)
	}
	if q.Hard.Memory > 0 {
		memRatio = float64(q.Used.Memory) / float64(q.Hard.Memory)
	}

	return (cpuRatio + memRatio) / 2
}

// QuotaManager manages resource quotas for namespaces.
type QuotaManager struct {
	mu     sync.RWMutex
	quotas map[string]*ResourceQuota // namespace -> quota
	scopes map[string][]QuotaScope   // namespace -> scopes
}

// QuotaScope defines the scope of a quota.
type QuotaScope string

const (
	QuotaScopeTerminating    QuotaScope = "Terminating"
	QuotaScopeNotTerminating QuotaScope = "NotTerminating"
	QuotaScopeBestEffort     QuotaScope = "BestEffort"
	QuotaScopeNotBestEffort  QuotaScope = "NotBestEffort"
)

// NewQuotaManager creates a new quota manager.
func NewQuotaManager() *QuotaManager {
	return &QuotaManager{
		quotas: make(map[string]*ResourceQuota),
		scopes: make(map[string][]QuotaScope),
	}
}

// SetQuota sets a quota for a namespace.
func (m *QuotaManager) SetQuota(namespace string, quota *ResourceQuota) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotas[namespace] = quota
}

// GetQuota returns a quota for a namespace.
func (m *QuotaManager) GetQuota(namespace string) *ResourceQuota {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.quotas[namespace]
}

// DeleteQuota removes a quota.
func (m *QuotaManager) DeleteQuota(namespace string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.quotas, namespace)
}

// ListQuotas returns all quotas.
func (m *QuotaManager) ListQuotas() []*ResourceQuota {
	m.mu.RLock()
	defer m.mu.RUnlock()

	quotas := make([]*ResourceQuota, 0, len(m.quotas))
	for _, q := range m.quotas {
		quotas = append(quotas, q)
	}
	return quotas
}

// CheckQuota checks if resources can be allocated.
func (m *QuotaManager) CheckQuota(namespace string, resources Resources) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	quota, exists := m.quotas[namespace]
	if !exists {
		return nil // No quota means unlimited
	}

	if !quota.HasCapacity(resources, 1) {
		return ErrQuotaExceeded
	}

	return nil
}

// ReserveQuota reserves resources against a namespace's quota.
func (m *QuotaManager) ReserveQuota(namespace string, resources Resources) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	quota, exists := m.quotas[namespace]
	if !exists {
		return nil
	}

	return quota.Reserve(resources)
}

// ReleaseQuota releases resources from a namespace's quota.
func (m *QuotaManager) ReleaseQuota(namespace string, resources Resources) {
	m.mu.Lock()
	defer m.mu.Unlock()

	quota, exists := m.quotas[namespace]
	if !exists {
		return
	}

	quota.Release(resources)
}

// UpdateQuota updates a quota's hard limits.
func (m *QuotaManager) UpdateQuota(namespace string, hard ResourceLimits) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	quota, exists := m.quotas[namespace]
	if !exists {
		return fmt.Errorf("quota for namespace %s not found", namespace)
	}

	quota.Hard = hard
	quota.UpdatedAt = time.Now()

	return nil
}

// GetUsage returns current resource usage for a namespace.
func (m *QuotaManager) GetUsage(namespace string) (*ResourceLimits, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	quota, exists := m.quotas[namespace]
	if !exists {
		return nil, fmt.Errorf("quota for namespace %s not found", namespace)
	}

	return &quota.Used, nil
}

// SetScopes sets quota scopes for a namespace.
func (m *QuotaManager) SetScopes(namespace string, scopes []QuotaScope) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scopes[namespace] = scopes
}

// GetScopes returns quota scopes for a namespace.
func (m *QuotaManager) GetScopes(namespace string) []QuotaScope {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.scopes[namespace]
}

// LimitRange defines resource limits for a namespace.
type LimitRange struct {
	Name        string
	Namespace   string
	Items       []LimitRangeItem
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// LimitRangeItem defines limits for a resource type.
type LimitRangeItem struct {
	Type           LimitType
	Default        Resources
	DefaultRequest Resources
	Max            Resources
	Min            Resources
	MaxLimitRequestRatio map[string]float64
}

// LimitType is the type of limit.
type LimitType string

const (
	LimitTypeContainer LimitType = "Container"
	LimitTypeWorkload  LimitType = "Workload"
)

// LimitRangeManager manages limit ranges.
type LimitRangeManager struct {
	mu          sync.RWMutex
	limitRanges map[string]*LimitRange // namespace -> limit range
}

// NewLimitRangeManager creates a new limit range manager.
func NewLimitRangeManager() *LimitRangeManager {
	return &LimitRangeManager{
		limitRanges: make(map[string]*LimitRange),
	}
}

// SetLimitRange sets a limit range for a namespace.
func (m *LimitRangeManager) SetLimitRange(namespace string, lr *LimitRange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limitRanges[namespace] = lr
}

// GetLimitRange returns a limit range for a namespace.
func (m *LimitRangeManager) GetLimitRange(namespace string) *LimitRange {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.limitRanges[namespace]
}

// DeleteLimitRange removes a limit range.
func (m *LimitRangeManager) DeleteLimitRange(namespace string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.limitRanges, namespace)
}

// ValidateWorkload validates a workload against limit ranges.
func (m *LimitRangeManager) ValidateWorkload(workload *Workload) error {
	m.mu.RLock()
	lr, exists := m.limitRanges[workload.Namespace]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	for _, item := range lr.Items {
		if item.Type != LimitTypeWorkload {
			continue
		}

		// Check minimum
		if !item.Min.IsZero() {
			if workload.Resources.Requests.CPU < item.Min.CPU {
				return fmt.Errorf("CPU request %d below minimum %d",
					workload.Resources.Requests.CPU, item.Min.CPU)
			}
			if workload.Resources.Requests.Memory < item.Min.Memory {
				return fmt.Errorf("memory request %d below minimum %d",
					workload.Resources.Requests.Memory, item.Min.Memory)
			}
		}

		// Check maximum
		if !item.Max.IsZero() {
			if workload.Resources.Requests.CPU > item.Max.CPU {
				return fmt.Errorf("CPU request %d above maximum %d",
					workload.Resources.Requests.CPU, item.Max.CPU)
			}
			if workload.Resources.Requests.Memory > item.Max.Memory {
				return fmt.Errorf("memory request %d above maximum %d",
					workload.Resources.Requests.Memory, item.Max.Memory)
			}
		}
	}

	return nil
}

// ApplyDefaults applies default resource values.
func (m *LimitRangeManager) ApplyDefaults(workload *Workload) {
	m.mu.RLock()
	lr, exists := m.limitRanges[workload.Namespace]
	m.mu.RUnlock()

	if !exists {
		return
	}

	for _, item := range lr.Items {
		if item.Type != LimitTypeWorkload {
			continue
		}

		// Apply default requests
		if workload.Resources.Requests.CPU == 0 && item.DefaultRequest.CPU > 0 {
			workload.Resources.Requests.CPU = item.DefaultRequest.CPU
		}
		if workload.Resources.Requests.Memory == 0 && item.DefaultRequest.Memory > 0 {
			workload.Resources.Requests.Memory = item.DefaultRequest.Memory
		}

		// Apply default limits
		if workload.Resources.Limits.CPU == 0 && item.Default.CPU > 0 {
			workload.Resources.Limits.CPU = item.Default.CPU
		}
		if workload.Resources.Limits.Memory == 0 && item.Default.Memory > 0 {
			workload.Resources.Limits.Memory = item.Default.Memory
		}
	}
}

// PriorityClass defines a priority class.
type PriorityClass struct {
	Name            string
	Value           int32
	GlobalDefault   bool
	Description     string
	PreemptionPolicy PreemptionPolicyType
}

// PreemptionPolicyType defines preemption policy for a priority class.
type PreemptionPolicyType string

const (
	PreemptionPolicyPreemptLowerPriority PreemptionPolicyType = "PreemptLowerPriority"
	PreemptionPolicyNever                PreemptionPolicyType = "Never"
)

// PriorityClassManager manages priority classes.
type PriorityClassManager struct {
	mu            sync.RWMutex
	classes       map[string]*PriorityClass
	defaultClass  string
}

// NewPriorityClassManager creates a new priority class manager.
func NewPriorityClassManager() *PriorityClassManager {
	mgr := &PriorityClassManager{
		classes: make(map[string]*PriorityClass),
	}

	// Add system priority classes
	mgr.classes["system-critical"] = &PriorityClass{
		Name:             "system-critical",
		Value:            2000000000,
		Description:      "System-critical workloads",
		PreemptionPolicy: PreemptionPolicyPreemptLowerPriority,
	}
	mgr.classes["system-cluster-critical"] = &PriorityClass{
		Name:             "system-cluster-critical",
		Value:            2000001000,
		Description:      "Cluster-critical system workloads",
		PreemptionPolicy: PreemptionPolicyPreemptLowerPriority,
	}

	return mgr
}

// AddClass adds a priority class.
func (m *PriorityClassManager) AddClass(class *PriorityClass) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.classes[class.Name]; exists {
		return fmt.Errorf("priority class %s already exists", class.Name)
	}

	m.classes[class.Name] = class

	if class.GlobalDefault {
		m.defaultClass = class.Name
	}

	return nil
}

// GetClass returns a priority class by name.
func (m *PriorityClassManager) GetClass(name string) (*PriorityClass, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	class, exists := m.classes[name]
	return class, exists
}

// GetDefaultClass returns the default priority class.
func (m *PriorityClassManager) GetDefaultClass() *PriorityClass {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.defaultClass != "" {
		return m.classes[m.defaultClass]
	}
	return nil
}

// DeleteClass removes a priority class.
func (m *PriorityClassManager) DeleteClass(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Don't delete system classes
	if name == "system-critical" || name == "system-cluster-critical" {
		return fmt.Errorf("cannot delete system priority class %s", name)
	}

	delete(m.classes, name)

	if m.defaultClass == name {
		m.defaultClass = ""
	}

	return nil
}

// ListClasses returns all priority classes.
func (m *PriorityClassManager) ListClasses() []*PriorityClass {
	m.mu.RLock()
	defer m.mu.RUnlock()

	classes := make([]*PriorityClass, 0, len(m.classes))
	for _, c := range m.classes {
		classes = append(classes, c)
	}
	return classes
}

// GetPriority returns the priority value for a class name.
func (m *PriorityClassManager) GetPriority(className string) int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if class, exists := m.classes[className]; exists {
		return class.Value
	}

	// Return default if exists
	if m.defaultClass != "" {
		return m.classes[m.defaultClass].Value
	}

	return 0
}

// QuotaStatus represents quota status.
type QuotaStatus struct {
	Namespace     string
	Hard          ResourceLimits
	Used          ResourceLimits
	Percentage    float64
	Status        QuotaStatusType
}

// QuotaStatusType represents quota status type.
type QuotaStatusType string

const (
	QuotaStatusOK       QuotaStatusType = "OK"
	QuotaStatusWarning  QuotaStatusType = "Warning"
	QuotaStatusCritical QuotaStatusType = "Critical"
)

// GetQuotaStatus returns the status of a quota.
func (m *QuotaManager) GetQuotaStatus(namespace string) *QuotaStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	quota, exists := m.quotas[namespace]
	if !exists {
		return nil
	}

	percentage := quota.UtilizationRatio() * 100

	status := QuotaStatusOK
	if percentage >= 90 {
		status = QuotaStatusCritical
	} else if percentage >= 75 {
		status = QuotaStatusWarning
	}

	return &QuotaStatus{
		Namespace:  namespace,
		Hard:       quota.Hard,
		Used:       quota.Used,
		Percentage: percentage,
		Status:     status,
	}
}

// ListQuotaStatuses returns status for all quotas.
func (m *QuotaManager) ListQuotaStatuses() []*QuotaStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]*QuotaStatus, 0, len(m.quotas))
	for namespace := range m.quotas {
		statuses = append(statuses, m.GetQuotaStatus(namespace))
	}
	return statuses
}

// QuotaEnforcer enforces quota limits.
type QuotaEnforcer struct {
	manager *QuotaManager
	mu      sync.RWMutex
	enabled bool
}

// NewQuotaEnforcer creates a new quota enforcer.
func NewQuotaEnforcer(manager *QuotaManager) *QuotaEnforcer {
	return &QuotaEnforcer{
		manager: manager,
		enabled: true,
	}
}

// SetEnabled enables or disables enforcement.
func (e *QuotaEnforcer) SetEnabled(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = enabled
}

// IsEnabled returns whether enforcement is enabled.
func (e *QuotaEnforcer) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

// Enforce enforces quota for a workload.
func (e *QuotaEnforcer) Enforce(workload *Workload) error {
	e.mu.RLock()
	if !e.enabled {
		e.mu.RUnlock()
		return nil
	}
	e.mu.RUnlock()

	return e.manager.CheckQuota(workload.Namespace, workload.Resources.Requests)
}

// QuotaReconciler reconciles quota usage.
type QuotaReconciler struct {
	manager *QuotaManager
	mu      sync.Mutex
}

// NewQuotaReconciler creates a new quota reconciler.
func NewQuotaReconciler(manager *QuotaManager) *QuotaReconciler {
	return &QuotaReconciler{
		manager: manager,
	}
}

// ReconcileResult contains reconciliation results.
type QuotaReconcileResult struct {
	UpdatedNamespaces []string
	Discrepancies     map[string]ResourceLimits
}

// Reconcile reconciles quota usage with actual workloads.
func (r *QuotaReconciler) Reconcile(workloads []*Workload) *QuotaReconcileResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := &QuotaReconcileResult{
		UpdatedNamespaces: []string{},
		Discrepancies:     make(map[string]ResourceLimits),
	}

	// Calculate actual usage per namespace
	actualUsage := make(map[string]ResourceLimits)
	for _, w := range workloads {
		if w.State == WorkloadStateScheduled || w.State == WorkloadStateRunning {
			usage := actualUsage[w.Namespace]
			usage.CPU += w.Resources.Requests.CPU
			usage.Memory += w.Resources.Requests.Memory
			usage.Disk += w.Resources.Requests.Disk
			usage.GPUs += w.Resources.Requests.GPUs
			usage.Ephemeral += w.Resources.Requests.Ephemeral
			usage.Workloads++
			actualUsage[w.Namespace] = usage
		}
	}

	// Compare with recorded usage
	quotas := r.manager.ListQuotas()
	for _, quota := range quotas {
		actual := actualUsage[quota.Namespace]

		if actual.CPU != quota.Used.CPU ||
			actual.Memory != quota.Used.Memory ||
			actual.Workloads != quota.Used.Workloads {

			result.Discrepancies[quota.Namespace] = ResourceLimits{
				CPU:       actual.CPU - quota.Used.CPU,
				Memory:    actual.Memory - quota.Used.Memory,
				Workloads: actual.Workloads - quota.Used.Workloads,
			}

			// Fix the quota
			quota.Used = actual
			quota.UpdatedAt = time.Now()
			result.UpdatedNamespaces = append(result.UpdatedNamespaces, quota.Namespace)
		}
	}

	return result
}
