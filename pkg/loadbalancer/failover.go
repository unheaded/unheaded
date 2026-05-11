// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// FailoverConfig configures failover behavior.
type FailoverConfig struct {
	// Enabled enables failover functionality.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Mode is the failover mode.
	Mode FailoverMode `json:"mode" yaml:"mode"`

	// PrimaryGroup is the name of the primary backend group.
	PrimaryGroup string `json:"primary_group,omitempty" yaml:"primary_group,omitempty"`

	// BackupGroups are backup groups in priority order.
	BackupGroups []string `json:"backup_groups,omitempty" yaml:"backup_groups,omitempty"`

	// FailoverThreshold is the minimum healthy backends in primary before failover.
	FailoverThreshold int `json:"failover_threshold" yaml:"failover_threshold"`

	// FailbackDelay is the delay before failing back to primary.
	FailbackDelay time.Duration `json:"failback_delay" yaml:"failback_delay"`

	// FailbackThreshold is the minimum healthy backends in primary to trigger failback.
	FailbackThreshold int `json:"failback_threshold" yaml:"failback_threshold"`

	// ZoneAware enables zone-aware failover.
	ZoneAware bool `json:"zone_aware" yaml:"zone_aware"`

	// PreferLocalZone prefers backends in the same zone as the load balancer.
	PreferLocalZone bool `json:"prefer_local_zone" yaml:"prefer_local_zone"`

	// LocalZone is the zone where this load balancer is running.
	LocalZone string `json:"local_zone,omitempty" yaml:"local_zone,omitempty"`

	// ZoneFailoverThreshold is the minimum healthy backends in local zone before cross-zone failover.
	ZoneFailoverThreshold int `json:"zone_failover_threshold" yaml:"zone_failover_threshold"`

	// CrossZoneTrafficCost is the cost factor for cross-zone traffic (for weighted decisions).
	CrossZoneTrafficCost float64 `json:"cross_zone_traffic_cost" yaml:"cross_zone_traffic_cost"`
}

// FailoverMode represents the failover strategy.
type FailoverMode string

const (
	// FailoverModePrimaryBackup uses primary/backup groups.
	FailoverModePrimaryBackup FailoverMode = "primary-backup"

	// FailoverModeWeighted uses weighted distribution across groups.
	FailoverModeWeighted FailoverMode = "weighted"

	// FailoverModePriority uses priority-based selection.
	FailoverModePriority FailoverMode = "priority"

	// FailoverModeZone uses zone-aware selection.
	FailoverModeZone FailoverMode = "zone"
)

// FailoverManager manages backend failover.
type FailoverManager struct {
	config FailoverConfig
	pool   *BackendPool

	// Backend groups
	groups   map[string]*BackendGroup
	groupsMu sync.RWMutex

	// Current failover state
	currentGroup      string
	failoverStartTime time.Time
	failoverMu        sync.RWMutex

	// Zone mapping
	zones   map[string][]string // zone -> backend names
	zonesMu sync.RWMutex

	// State
	running int32
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Metrics
	metrics *FailoverMetrics

	// Callbacks
	onFailover  func(fromGroup, toGroup string, reason string)
	onFailback  func(toGroup string)
	onZoneShift func(fromZone, toZone string)

	// Inner algorithm for selection within a group
	innerAlgorithm Algorithm
}

// BackendGroup represents a group of backends.
type BackendGroup struct {
	Name     string
	Priority int      // Lower = higher priority
	Weight   int      // For weighted distribution
	Backends []string // Backend names in this group
	Zone     string   // Optional zone assignment

	// Runtime state
	mu           sync.RWMutex
	healthyCount int
	isDraining   bool
}

// FailoverMetrics tracks failover statistics.
type FailoverMetrics struct {
	mu sync.RWMutex

	// Failover events
	FailoverCount  int64
	FailbackCount  int64
	ZoneShiftCount int64

	// Time in failover state
	TotalFailoverDuration time.Duration

	// Per-group statistics
	GroupStats map[string]*GroupMetrics

	// Zone statistics
	ZoneStats map[string]*ZoneMetrics

	// Last events
	LastFailoverTime time.Time
	LastFailbackTime time.Time
	LastZoneShift    time.Time
}

// GroupMetrics holds metrics for a backend group.
type GroupMetrics struct {
	RequestsServed      int64
	TimeAsPrimary       time.Duration
	FailoverActivations int64
}

// ZoneMetrics holds metrics for a zone.
type ZoneMetrics struct {
	RequestsServed    int64
	CrossZoneRequests int64
	TimeAsActiveZone  time.Duration
}

// NewFailoverMetrics creates new failover metrics.
func NewFailoverMetrics() *FailoverMetrics {
	return &FailoverMetrics{
		GroupStats: make(map[string]*GroupMetrics),
		ZoneStats:  make(map[string]*ZoneMetrics),
	}
}

// NewFailoverManager creates a new failover manager.
func NewFailoverManager(config FailoverConfig, pool *BackendPool) *FailoverManager {
	fm := &FailoverManager{
		config:         config,
		pool:           pool,
		groups:         make(map[string]*BackendGroup),
		zones:          make(map[string][]string),
		stopCh:         make(chan struct{}),
		metrics:        NewFailoverMetrics(),
		currentGroup:   config.PrimaryGroup,
		innerAlgorithm: AlgorithmRoundRobin,
	}

	return fm
}

// SetInnerAlgorithm sets the algorithm used within a group.
func (fm *FailoverManager) SetInnerAlgorithm(algorithm Algorithm) {
	fm.innerAlgorithm = algorithm
}

// AddGroup adds a backend group.
func (fm *FailoverManager) AddGroup(group *BackendGroup) error {
	fm.groupsMu.Lock()
	defer fm.groupsMu.Unlock()

	if _, exists := fm.groups[group.Name]; exists {
		return fmt.Errorf("group %s already exists", group.Name)
	}

	// Verify all backends exist
	for _, name := range group.Backends {
		if _, ok := fm.pool.Get(name); !ok {
			return fmt.Errorf("backend %s not found in pool", name)
		}
	}

	fm.groups[group.Name] = group

	// Update zone mapping if zone is specified
	if group.Zone != "" {
		fm.zonesMu.Lock()
		fm.zones[group.Zone] = append(fm.zones[group.Zone], group.Backends...)
		fm.zonesMu.Unlock()
	}

	// Initialize metrics
	fm.metrics.mu.Lock()
	fm.metrics.GroupStats[group.Name] = &GroupMetrics{}
	if group.Zone != "" {
		if fm.metrics.ZoneStats[group.Zone] == nil {
			fm.metrics.ZoneStats[group.Zone] = &ZoneMetrics{}
		}
	}
	fm.metrics.mu.Unlock()

	return nil
}

// RemoveGroup removes a backend group.
func (fm *FailoverManager) RemoveGroup(name string) error {
	fm.groupsMu.Lock()
	defer fm.groupsMu.Unlock()

	group, ok := fm.groups[name]
	if !ok {
		return fmt.Errorf("group %s not found", name)
	}

	// Can't remove the current active group
	fm.failoverMu.RLock()
	current := fm.currentGroup
	fm.failoverMu.RUnlock()

	if current == name {
		return errors.New("cannot remove currently active group")
	}

	// Update zone mapping
	if group.Zone != "" {
		fm.zonesMu.Lock()
		newZoneBackends := make([]string, 0)
		for _, b := range fm.zones[group.Zone] {
			found := false
			for _, gb := range group.Backends {
				if b == gb {
					found = true
					break
				}
			}
			if !found {
				newZoneBackends = append(newZoneBackends, b)
			}
		}
		fm.zones[group.Zone] = newZoneBackends
		fm.zonesMu.Unlock()
	}

	delete(fm.groups, name)
	return nil
}

// Start starts the failover manager.
func (fm *FailoverManager) Start() error {
	if !atomic.CompareAndSwapInt32(&fm.running, 0, 1) {
		return errors.New("failover manager already running")
	}

	fm.wg.Add(1)
	go fm.monitorLoop()

	return nil
}

// Stop stops the failover manager.
func (fm *FailoverManager) Stop() error {
	if !atomic.CompareAndSwapInt32(&fm.running, 1, 0) {
		return errors.New("failover manager not running")
	}

	close(fm.stopCh)
	fm.wg.Wait()
	return nil
}

// monitorLoop monitors group health and triggers failover.
func (fm *FailoverManager) monitorLoop() {
	defer fm.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fm.stopCh:
			return
		case <-ticker.C:
			fm.checkAndFailover()
		}
	}
}

// checkAndFailover checks group health and triggers failover if needed.
func (fm *FailoverManager) checkAndFailover() {
	fm.groupsMu.RLock()
	defer fm.groupsMu.RUnlock()

	// Update healthy counts for all groups
	for _, group := range fm.groups {
		healthy := fm.countHealthyBackends(group)
		group.mu.Lock()
		group.healthyCount = healthy
		group.mu.Unlock()
	}

	fm.failoverMu.Lock()
	defer fm.failoverMu.Unlock()

	currentGroup := fm.groups[fm.currentGroup]
	if currentGroup == nil {
		return
	}

	switch fm.config.Mode {
	case FailoverModePrimaryBackup:
		fm.checkPrimaryBackupFailover(currentGroup)
	case FailoverModePriority:
		fm.checkPriorityFailover()
	case FailoverModeZone:
		fm.checkZoneFailover()
	}
}

// countHealthyBackends counts healthy backends in a group.
func (fm *FailoverManager) countHealthyBackends(group *BackendGroup) int {
	count := 0
	for _, name := range group.Backends {
		backend, ok := fm.pool.Get(name)
		if ok && backend.IsHealthy() {
			count++
		}
	}
	return count
}

// checkPrimaryBackupFailover handles primary/backup failover logic.
func (fm *FailoverManager) checkPrimaryBackupFailover(currentGroup *BackendGroup) {
	primaryGroup := fm.groups[fm.config.PrimaryGroup]
	if primaryGroup == nil {
		return
	}

	primaryGroup.mu.RLock()
	primaryHealthy := primaryGroup.healthyCount
	primaryGroup.mu.RUnlock()

	// Check if we should failover
	if fm.currentGroup == fm.config.PrimaryGroup {
		// Currently on primary, check if we need to failover
		if primaryHealthy < fm.config.FailoverThreshold {
			fm.triggerFailover("primary unhealthy")
		}
	} else {
		// Currently on backup, check if we should fail back
		if primaryHealthy >= fm.config.FailbackThreshold {
			// Check failback delay
			if time.Since(fm.failoverStartTime) >= fm.config.FailbackDelay {
				fm.triggerFailback()
			}
		}
	}
}

// triggerFailover triggers failover to the next available group.
func (fm *FailoverManager) triggerFailover(reason string) {
	// Find next healthy backup group
	for _, groupName := range fm.config.BackupGroups {
		group, ok := fm.groups[groupName]
		if !ok {
			continue
		}

		group.mu.RLock()
		healthy := group.healthyCount
		group.mu.RUnlock()

		if healthy > 0 {
			prevGroup := fm.currentGroup
			fm.currentGroup = groupName
			fm.failoverStartTime = time.Now()

			// Update metrics
			fm.metrics.mu.Lock()
			fm.metrics.FailoverCount++
			fm.metrics.LastFailoverTime = time.Now()
			if gs, ok := fm.metrics.GroupStats[groupName]; ok {
				gs.FailoverActivations++
			}
			fm.metrics.mu.Unlock()

			// Callback
			if fm.onFailover != nil {
				fm.onFailover(prevGroup, groupName, reason)
			}

			return
		}
	}
}

// triggerFailback triggers failback to primary.
func (fm *FailoverManager) triggerFailback() {
	fm.currentGroup = fm.config.PrimaryGroup
	fm.failoverStartTime = time.Time{}

	// Update metrics
	fm.metrics.mu.Lock()
	fm.metrics.FailbackCount++
	fm.metrics.LastFailbackTime = time.Now()
	fm.metrics.mu.Unlock()

	// Callback
	if fm.onFailback != nil {
		fm.onFailback(fm.config.PrimaryGroup)
	}
}

// checkPriorityFailover handles priority-based failover.
func (fm *FailoverManager) checkPriorityFailover() {
	// Get all groups sorted by priority
	groups := make([]*BackendGroup, 0, len(fm.groups))
	for _, g := range fm.groups {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Priority < groups[j].Priority
	})

	// Find highest priority group with healthy backends
	for _, group := range groups {
		group.mu.RLock()
		healthy := group.healthyCount
		group.mu.RUnlock()

		if healthy > 0 {
			if fm.currentGroup != group.Name {
				prevGroup := fm.currentGroup
				fm.currentGroup = group.Name
				fm.failoverStartTime = time.Now()

				if fm.onFailover != nil {
					fm.onFailover(prevGroup, group.Name, "priority failover")
				}

				fm.metrics.mu.Lock()
				fm.metrics.FailoverCount++
				fm.metrics.LastFailoverTime = time.Now()
				fm.metrics.mu.Unlock()
			}
			return
		}
	}
}

// checkZoneFailover handles zone-aware failover.
func (fm *FailoverManager) checkZoneFailover() {
	if fm.config.LocalZone == "" {
		return
	}

	fm.zonesMu.RLock()
	localBackends := fm.zones[fm.config.LocalZone]
	fm.zonesMu.RUnlock()

	// Count healthy backends in local zone
	localHealthy := 0
	for _, name := range localBackends {
		backend, ok := fm.pool.Get(name)
		if ok && backend.IsHealthy() {
			localHealthy++
		}
	}

	// Check if we need cross-zone failover
	if localHealthy < fm.config.ZoneFailoverThreshold {
		// Find another zone with healthy backends
		fm.zonesMu.RLock()
		for zone, backends := range fm.zones {
			if zone == fm.config.LocalZone {
				continue
			}

			zoneHealthy := 0
			for _, name := range backends {
				backend, ok := fm.pool.Get(name)
				if ok && backend.IsHealthy() {
					zoneHealthy++
				}
			}

			if zoneHealthy >= fm.config.ZoneFailoverThreshold {
				// Find the group for this zone
				for _, group := range fm.groups {
					if group.Zone == zone {
						if fm.currentGroup != group.Name {
							prevGroup := fm.currentGroup
							fm.currentGroup = group.Name

							if fm.onZoneShift != nil {
								fm.onZoneShift(fm.config.LocalZone, zone)
							}

							if fm.onFailover != nil {
								fm.onFailover(prevGroup, group.Name, "zone failover")
							}

							fm.metrics.mu.Lock()
							fm.metrics.ZoneShiftCount++
							fm.metrics.LastZoneShift = time.Now()
							fm.metrics.mu.Unlock()
						}
						fm.zonesMu.RUnlock()
						return
					}
				}
			}
		}
		fm.zonesMu.RUnlock()
	}
}

// Select selects a backend using failover-aware selection.
func (fm *FailoverManager) Select(ctx context.Context, key string) (*Backend, error) {
	if !fm.config.Enabled {
		return fm.pool.Select(ctx, key)
	}

	fm.failoverMu.RLock()
	currentGroup := fm.currentGroup
	fm.failoverMu.RUnlock()

	fm.groupsMu.RLock()
	group, ok := fm.groups[currentGroup]
	fm.groupsMu.RUnlock()

	if !ok || group == nil {
		return fm.pool.Select(ctx, key)
	}

	// Get healthy backends from the current group
	var healthy []*Backend
	for _, name := range group.Backends {
		backend, ok := fm.pool.Get(name)
		if ok && backend.IsAvailable() {
			healthy = append(healthy, backend)
		}
	}

	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	// Use inner algorithm to select
	algorithm := NewAlgorithmState(fm.innerAlgorithm, healthy)
	backend, err := algorithm.Select(ctx, key, healthy)
	if err != nil {
		return nil, err
	}

	// Record metrics
	fm.metrics.mu.Lock()
	if gs, ok := fm.metrics.GroupStats[currentGroup]; ok {
		gs.RequestsServed++
	}
	if group.Zone != "" {
		if zs, ok := fm.metrics.ZoneStats[group.Zone]; ok {
			zs.RequestsServed++
			if group.Zone != fm.config.LocalZone {
				zs.CrossZoneRequests++
			}
		}
	}
	fm.metrics.mu.Unlock()

	return backend, nil
}

// SelectWithZonePreference selects a backend preferring the local zone.
func (fm *FailoverManager) SelectWithZonePreference(ctx context.Context, key string) (*Backend, error) {
	if !fm.config.ZoneAware || fm.config.LocalZone == "" {
		return fm.Select(ctx, key)
	}

	fm.zonesMu.RLock()
	localBackends := fm.zones[fm.config.LocalZone]
	fm.zonesMu.RUnlock()

	// Try to select from local zone first
	var localHealthy []*Backend
	for _, name := range localBackends {
		backend, ok := fm.pool.Get(name)
		if ok && backend.IsAvailable() {
			localHealthy = append(localHealthy, backend)
		}
	}

	if len(localHealthy) > 0 {
		algorithm := NewAlgorithmState(fm.innerAlgorithm, localHealthy)
		backend, err := algorithm.Select(ctx, key, localHealthy)
		if err == nil {
			return backend, nil
		}
	}

	// Fall back to cross-zone selection
	return fm.Select(ctx, key)
}

// OnFailover sets the failover callback.
func (fm *FailoverManager) OnFailover(fn func(fromGroup, toGroup string, reason string)) {
	fm.onFailover = fn
}

// OnFailback sets the failback callback.
func (fm *FailoverManager) OnFailback(fn func(toGroup string)) {
	fm.onFailback = fn
}

// OnZoneShift sets the zone shift callback.
func (fm *FailoverManager) OnZoneShift(fn func(fromZone, toZone string)) {
	fm.onZoneShift = fn
}

// CurrentGroup returns the current active group.
func (fm *FailoverManager) CurrentGroup() string {
	fm.failoverMu.RLock()
	defer fm.failoverMu.RUnlock()
	return fm.currentGroup
}

// IsInFailover returns true if currently in failover state.
func (fm *FailoverManager) IsInFailover() bool {
	fm.failoverMu.RLock()
	defer fm.failoverMu.RUnlock()
	return fm.currentGroup != fm.config.PrimaryGroup
}

// GetGroupHealth returns health info for a group.
func (fm *FailoverManager) GetGroupHealth(name string) (healthy, total int, draining bool) {
	fm.groupsMu.RLock()
	group, ok := fm.groups[name]
	fm.groupsMu.RUnlock()

	if !ok {
		return 0, 0, false
	}

	group.mu.RLock()
	defer group.mu.RUnlock()

	return group.healthyCount, len(group.Backends), group.isDraining
}

// Stats returns failover statistics.
func (fm *FailoverManager) Stats() FailoverStats {
	fm.failoverMu.RLock()
	currentGroup := fm.currentGroup
	inFailover := currentGroup != fm.config.PrimaryGroup
	failoverDuration := time.Duration(0)
	if inFailover && !fm.failoverStartTime.IsZero() {
		failoverDuration = time.Since(fm.failoverStartTime)
	}
	fm.failoverMu.RUnlock()

	fm.metrics.mu.RLock()
	defer fm.metrics.mu.RUnlock()

	stats := FailoverStats{
		CurrentGroup:     currentGroup,
		InFailover:       inFailover,
		FailoverDuration: failoverDuration,
		FailoverCount:    fm.metrics.FailoverCount,
		FailbackCount:    fm.metrics.FailbackCount,
		ZoneShiftCount:   fm.metrics.ZoneShiftCount,
		LastFailoverTime: fm.metrics.LastFailoverTime,
		LastFailbackTime: fm.metrics.LastFailbackTime,
		GroupStats:       make(map[string]GroupStats),
		ZoneStats:        make(map[string]ZoneStats),
	}

	for name, gs := range fm.metrics.GroupStats {
		fm.groupsMu.RLock()
		group := fm.groups[name]
		fm.groupsMu.RUnlock()

		healthy := 0
		total := 0
		if group != nil {
			group.mu.RLock()
			healthy = group.healthyCount
			total = len(group.Backends)
			group.mu.RUnlock()
		}

		stats.GroupStats[name] = GroupStats{
			HealthyBackends:     healthy,
			TotalBackends:       total,
			RequestsServed:      gs.RequestsServed,
			FailoverActivations: gs.FailoverActivations,
		}
	}

	for zone, zs := range fm.metrics.ZoneStats {
		stats.ZoneStats[zone] = ZoneStats{
			RequestsServed:    zs.RequestsServed,
			CrossZoneRequests: zs.CrossZoneRequests,
		}
	}

	return stats
}

// FailoverStats holds failover statistics.
type FailoverStats struct {
	CurrentGroup     string
	InFailover       bool
	FailoverDuration time.Duration
	FailoverCount    int64
	FailbackCount    int64
	ZoneShiftCount   int64
	LastFailoverTime time.Time
	LastFailbackTime time.Time
	GroupStats       map[string]GroupStats
	ZoneStats        map[string]ZoneStats
}

// GroupStats holds statistics for a group.
type GroupStats struct {
	HealthyBackends     int
	TotalBackends       int
	RequestsServed      int64
	FailoverActivations int64
}

// ZoneStats holds statistics for a zone.
type ZoneStats struct {
	RequestsServed    int64
	CrossZoneRequests int64
}

// WeightedFailoverManager implements weighted failover distribution.
type WeightedFailoverManager struct {
	*FailoverManager

	// Spillover configuration
	spilloverEnabled   bool
	spilloverThreshold float64 // Start spillover when group utilization exceeds this
}

// NewWeightedFailoverManager creates a weighted failover manager.
func NewWeightedFailoverManager(config FailoverConfig, pool *BackendPool) *WeightedFailoverManager {
	return &WeightedFailoverManager{
		FailoverManager:    NewFailoverManager(config, pool),
		spilloverEnabled:   false,
		spilloverThreshold: 0.8,
	}
}

// EnableSpillover enables spillover to backup groups when primary is overloaded.
func (wfm *WeightedFailoverManager) EnableSpillover(threshold float64) {
	wfm.spilloverEnabled = true
	wfm.spilloverThreshold = threshold
}

// SelectWeighted selects a backend using weighted distribution across groups.
func (wfm *WeightedFailoverManager) SelectWeighted(ctx context.Context, key string) (*Backend, error) {
	wfm.groupsMu.RLock()
	defer wfm.groupsMu.RUnlock()

	// Calculate total weight of groups with healthy backends
	var totalWeight int
	var weightedGroups []*BackendGroup

	for _, group := range wfm.groups {
		group.mu.RLock()
		healthy := group.healthyCount
		draining := group.isDraining
		group.mu.RUnlock()

		if healthy > 0 && !draining && group.Weight > 0 {
			totalWeight += group.Weight
			weightedGroups = append(weightedGroups, group)
		}
	}

	if totalWeight == 0 {
		return nil, ErrNoHealthyBackends
	}

	// Simple weighted selection based on hash
	hash := hashString(key)
	target := int(hash % uint64(totalWeight))

	var cumulative int
	for _, group := range weightedGroups {
		cumulative += group.Weight
		if target < cumulative {
			// Select from this group
			var healthy []*Backend
			for _, name := range group.Backends {
				backend, ok := wfm.pool.Get(name)
				if ok && backend.IsAvailable() {
					healthy = append(healthy, backend)
				}
			}

			if len(healthy) > 0 {
				algorithm := NewAlgorithmState(wfm.innerAlgorithm, healthy)
				return algorithm.Select(ctx, key, healthy)
			}
		}
	}

	return nil, ErrNoHealthyBackends
}

// hashString returns a hash of a string.
func hashString(s string) uint64 {
	var h uint64
	for _, c := range s {
		h = h*31 + uint64(c)
	}
	return h
}

// GracefulDrainManager manages graceful draining of backends.
type GracefulDrainManager struct {
	pool *BackendPool

	// Draining backends
	draining   map[string]*DrainState
	drainingMu sync.RWMutex

	// Configuration
	defaultDrainTimeout time.Duration
	checkInterval       time.Duration

	// State
	running int32
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Callbacks
	onDrainStart    func(backend *Backend)
	onDrainComplete func(backend *Backend, success bool)
}

// DrainState tracks the drain state of a backend.
type DrainState struct {
	StartTime     time.Time
	Timeout       time.Duration
	InitialConns  int64
	ForceComplete bool
}

// NewGracefulDrainManager creates a new graceful drain manager.
func NewGracefulDrainManager(pool *BackendPool, defaultTimeout time.Duration) *GracefulDrainManager {
	return &GracefulDrainManager{
		pool:                pool,
		draining:            make(map[string]*DrainState),
		defaultDrainTimeout: defaultTimeout,
		checkInterval:       time.Second,
		stopCh:              make(chan struct{}),
	}
}

// Start starts the drain manager.
func (dm *GracefulDrainManager) Start() error {
	if !atomic.CompareAndSwapInt32(&dm.running, 0, 1) {
		return errors.New("drain manager already running")
	}

	dm.wg.Add(1)
	go dm.monitorLoop()

	return nil
}

// Stop stops the drain manager.
func (dm *GracefulDrainManager) Stop() error {
	if !atomic.CompareAndSwapInt32(&dm.running, 1, 0) {
		return errors.New("drain manager not running")
	}

	close(dm.stopCh)
	dm.wg.Wait()
	return nil
}

// monitorLoop monitors draining backends.
func (dm *GracefulDrainManager) monitorLoop() {
	defer dm.wg.Done()

	ticker := time.NewTicker(dm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dm.stopCh:
			return
		case <-ticker.C:
			dm.checkDrainingBackends()
		}
	}
}

// checkDrainingBackends checks on draining backends.
func (dm *GracefulDrainManager) checkDrainingBackends() {
	dm.drainingMu.Lock()
	defer dm.drainingMu.Unlock()

	for name, state := range dm.draining {
		backend, ok := dm.pool.Get(name)
		if !ok {
			// Backend was removed
			delete(dm.draining, name)
			continue
		}

		// Check if drain is complete
		if backend.ActiveConnections() == 0 || state.ForceComplete {
			delete(dm.draining, name)
			if dm.onDrainComplete != nil {
				dm.onDrainComplete(backend, backend.ActiveConnections() == 0)
			}
			continue
		}

		// Check timeout
		if time.Since(state.StartTime) > state.Timeout {
			delete(dm.draining, name)
			if dm.onDrainComplete != nil {
				dm.onDrainComplete(backend, false)
			}
		}
	}
}

// StartDrain starts draining a backend.
func (dm *GracefulDrainManager) StartDrain(name string, timeout time.Duration) error {
	backend, ok := dm.pool.Get(name)
	if !ok {
		return fmt.Errorf("backend %s not found", name)
	}

	if timeout == 0 {
		timeout = dm.defaultDrainTimeout
	}

	// Mark backend as draining
	backend.Drain()

	// Track drain state
	dm.drainingMu.Lock()
	dm.draining[name] = &DrainState{
		StartTime:    time.Now(),
		Timeout:      timeout,
		InitialConns: backend.ActiveConnections(),
	}
	dm.drainingMu.Unlock()

	if dm.onDrainStart != nil {
		dm.onDrainStart(backend)
	}

	return nil
}

// StopDrain cancels draining and re-enables a backend.
func (dm *GracefulDrainManager) StopDrain(name string) error {
	backend, ok := dm.pool.Get(name)
	if !ok {
		return fmt.Errorf("backend %s not found", name)
	}

	dm.drainingMu.Lock()
	delete(dm.draining, name)
	dm.drainingMu.Unlock()

	// Re-enable the backend
	backend.Enable()

	return nil
}

// ForceDrain immediately completes the drain.
func (dm *GracefulDrainManager) ForceDrain(name string) error {
	dm.drainingMu.Lock()
	defer dm.drainingMu.Unlock()

	state, ok := dm.draining[name]
	if !ok {
		return fmt.Errorf("backend %s is not draining", name)
	}

	state.ForceComplete = true
	return nil
}

// IsDraining returns true if a backend is draining.
func (dm *GracefulDrainManager) IsDraining(name string) bool {
	dm.drainingMu.RLock()
	defer dm.drainingMu.RUnlock()
	_, ok := dm.draining[name]
	return ok
}

// GetDrainState returns the drain state for a backend.
func (dm *GracefulDrainManager) GetDrainState(name string) (*DrainState, bool) {
	dm.drainingMu.RLock()
	defer dm.drainingMu.RUnlock()
	state, ok := dm.draining[name]
	if !ok {
		return nil, false
	}
	// Return a copy
	stateCopy := *state
	return &stateCopy, true
}

// DrainAll starts draining all backends.
func (dm *GracefulDrainManager) DrainAll(timeout time.Duration) {
	for _, backend := range dm.pool.All() {
		_ = dm.StartDrain(backend.Config.Name, timeout)
	}
}

// OnDrainStart sets the callback for when drain starts.
func (dm *GracefulDrainManager) OnDrainStart(fn func(backend *Backend)) {
	dm.onDrainStart = fn
}

// OnDrainComplete sets the callback for when drain completes.
func (dm *GracefulDrainManager) OnDrainComplete(fn func(backend *Backend, success bool)) {
	dm.onDrainComplete = fn
}

// WaitForDrain waits for a backend to complete draining.
func (dm *GracefulDrainManager) WaitForDrain(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if !dm.IsDraining(name) {
			return nil
		}

		select {
		case <-dm.stopCh:
			return errors.New("drain manager stopped")
		case <-time.After(100 * time.Millisecond):
		}
	}

	return errors.New("timeout waiting for drain")
}

// DrainStats returns statistics about draining backends.
func (dm *GracefulDrainManager) DrainStats() DrainManagerStats {
	dm.drainingMu.RLock()
	defer dm.drainingMu.RUnlock()

	stats := DrainManagerStats{
		DrainingBackends: make(map[string]DrainBackendStats),
	}

	for name, state := range dm.draining {
		backend, ok := dm.pool.Get(name)
		if !ok {
			continue
		}

		stats.TotalDraining++
		stats.DrainingBackends[name] = DrainBackendStats{
			StartTime:     state.StartTime,
			Duration:      time.Since(state.StartTime),
			Timeout:       state.Timeout,
			InitialConns:  state.InitialConns,
			CurrentConns:  backend.ActiveConnections(),
			ForceComplete: state.ForceComplete,
		}
	}

	return stats
}

// DrainManagerStats holds drain manager statistics.
type DrainManagerStats struct {
	TotalDraining    int
	DrainingBackends map[string]DrainBackendStats
}

// DrainBackendStats holds stats for a draining backend.
type DrainBackendStats struct {
	StartTime     time.Time
	Duration      time.Duration
	Timeout       time.Duration
	InitialConns  int64
	CurrentConns  int64
	ForceComplete bool
}
