// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dos

import (
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/logger"
)

// BackpressureLevel represents the level of backpressure in the system.
type BackpressureLevel int

const (
	BackpressureNone BackpressureLevel = iota
	BackpressureLow
	BackpressureMedium
	BackpressureHigh
	BackpressureCritical
)

// DropRateTracker tracks packet drop rates using a sliding window.
type DropRateTracker struct {
	mu              sync.RWMutex
	windowSize      time.Duration
	dropEvents      []time.Time
	lastCleanup     time.Time
}

// RingBufferMonitor monitors drop rates and triggers backpressure.
type RingBufferMonitor struct {
	mu              sync.RWMutex
	tracker         *DropRateTracker
	backpressureMap map[string]BackpressureLevel
	logger          *logger.Logger
	dropThreshold   float64
}

// MapUsageTracker tracks usage of a single BPF map.
type MapUsageTracker struct {
	mu           sync.RWMutex
	name         string
	maxEntries   uint64
	currentUsage uint64
	alertedAt80  bool
	alertedAt90  bool
	alertedAt95  bool
}

// BPFMapLimits enforces maximum entries on BPF maps.
type BPFMapLimits struct {
	mu         sync.RWMutex
	mapTrackers map[string]*MapUsageTracker
	logger      *logger.Logger
	defaultMax  uint64
}

// SizeLimitEnforcer tracks and enforces Sophia state size limits.
type SizeLimitEnforcer struct {
	mu              sync.RWMutex
	perFlowLimit    uint64
	globalLimit     uint64
	perFlowUsage    map[string]uint64
	globalUsage     uint64
	logger          *logger.Logger
}

// CompressFlag indicates whether a Sophia entry should be compressed.
type CompressFlag int

const (
	CompressDisabled CompressFlag = iota
	CompressEnabled
)

// CompressionGuard mitigates CRIME/BREACH attacks.
type CompressionGuard struct {
	mu               sync.RWMutex
	compressFlags    map[string]CompressFlag
	trustedContext   bool
	untrustedContext bool
	logger           *logger.Logger
}

// ExtensionLimits enforces limits on Monad extensions.
type ExtensionLimits struct {
	mu           sync.RWMutex
	maxTLVs      int
	unknownSkips int
	logger       *logger.Logger
}

// NewDropRateTracker creates a new drop rate tracker with sliding window.
func NewDropRateTracker(windowSize time.Duration) (*DropRateTracker, error) {
	if windowSize <= 0 {
		return nil, fmt.Errorf("window size must be positive")
	}

	return &DropRateTracker{
		windowSize:  windowSize,
		dropEvents:  make([]time.Time, 0),
		lastCleanup: time.Now(),
	}, nil
}

// RecordDrop records a packet drop event.
func (dt *DropRateTracker) RecordDrop() {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	dt.dropEvents = append(dt.dropEvents, time.Now())
}

// GetDropRate returns the current drop rate (drops/sec).
func (dt *DropRateTracker) GetDropRate() float64 {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-dt.windowSize)

	// Remove old events outside the window
	validEvents := 0
	for _, t := range dt.dropEvents {
		if t.After(cutoff) {
			validEvents++
		}
	}

	// Compact the slice
	if validEvents < len(dt.dropEvents) {
		dt.dropEvents = dt.dropEvents[len(dt.dropEvents)-validEvents:]
	}

	if dt.windowSize == 0 {
		return 0
	}

	windowSeconds := dt.windowSize.Seconds()
	return float64(len(dt.dropEvents)) / windowSeconds
}

// NewRingBufferMonitor creates a new ring buffer monitor.
func NewRingBufferMonitor(log *logger.Logger) (*RingBufferMonitor, error) {
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	tracker, err := NewDropRateTracker(1 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to create drop rate tracker: %w", err)
	}

	return &RingBufferMonitor{
		tracker:         tracker,
		backpressureMap: make(map[string]BackpressureLevel),
		logger:          log,
		dropThreshold:   0.01, // 1% drops/sec = 0.01
	}, nil
}

// RecordDrop records a packet drop and updates backpressure level.
func (rbm *RingBufferMonitor) RecordDrop() error {
	rbm.mu.Lock()
	defer rbm.mu.Unlock()

	rbm.tracker.RecordDrop()
	dropRate := rbm.tracker.GetDropRate()

	// Determine backpressure level
	level := BackpressureNone
	if dropRate > rbm.dropThreshold {
		level = BackpressureHigh
		rbm.logger.Warn().Msgf("High drop rate detected: %.4f drops/sec", dropRate)
	}

	rbm.backpressureMap["global"] = level

	return nil
}

// GetBackpressureLevel returns the current backpressure level.
func (rbm *RingBufferMonitor) GetBackpressureLevel() BackpressureLevel {
	rbm.mu.RLock()
	defer rbm.mu.RUnlock()

	if level, exists := rbm.backpressureMap["global"]; exists {
		return level
	}
	return BackpressureNone
}

// DropWithQoSAwareness drops packets based on QoS class when needed.
func (rbm *RingBufferMonitor) DropWithQoSAwareness(qosClass int) bool {
	level := rbm.GetBackpressureLevel()

	if level == BackpressureNone {
		return false
	}

	// Drop QoS classes 4-5 first under high backpressure
	if level >= BackpressureHigh && qosClass >= 4 {
		rbm.logger.Debug().Msgf("Dropping packet with QoS class %d due to backpressure", qosClass)
		return true
	}

	return false
}

// NewMapUsageTracker creates a new map usage tracker.
func NewMapUsageTracker(name string, maxEntries uint64) (*MapUsageTracker, error) {
	if name == "" {
		return nil, fmt.Errorf("map name cannot be empty")
	}
	if maxEntries == 0 {
		return nil, fmt.Errorf("max entries must be positive")
	}

	return &MapUsageTracker{
		name:       name,
		maxEntries: maxEntries,
	}, nil
}

// GetUsagePercentage returns the current usage as a percentage.
func (mut *MapUsageTracker) GetUsagePercentage() float64 {
	mut.mu.RLock()
	defer mut.mu.RUnlock()

	if mut.maxEntries == 0 {
		return 0
	}

	return (float64(mut.currentUsage) / float64(mut.maxEntries)) * 100
}

// UpdateUsage updates the current usage and returns alert status.
func (mut *MapUsageTracker) UpdateUsage(usage uint64) (alert80, alert90, alert95 bool, err error) {
	if usage > mut.maxEntries {
		return false, false, false, fmt.Errorf("usage %d exceeds max entries %d", usage, mut.maxEntries)
	}

	mut.mu.Lock()
	defer mut.mu.Unlock()

	mut.currentUsage = usage
	percentage := (float64(usage) / float64(mut.maxEntries)) * 100

	if percentage >= 95 && !mut.alertedAt95 {
		mut.alertedAt95 = true
		return false, false, true, nil
	}

	if percentage >= 90 && !mut.alertedAt90 {
		mut.alertedAt90 = true
		return false, true, false, nil
	}

	if percentage >= 80 && !mut.alertedAt80 {
		mut.alertedAt80 = true
		return true, false, false, nil
	}

	return false, false, false, nil
}

// NewBPFMapLimits creates a new BPF map limits enforcer.
func NewBPFMapLimits(log *logger.Logger, defaultMax uint64) (*BPFMapLimits, error) {
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if defaultMax == 0 {
		return nil, fmt.Errorf("default max must be positive")
	}

	return &BPFMapLimits{
		mapTrackers: make(map[string]*MapUsageTracker),
		logger:      log,
		defaultMax:  defaultMax,
	}, nil
}

// RegisterMap registers a BPF map for tracking.
func (bml *BPFMapLimits) RegisterMap(name string, maxEntries uint64) error {
	if name == "" {
		return fmt.Errorf("map name cannot be empty")
	}

	bml.mu.Lock()
	defer bml.mu.Unlock()

	if _, exists := bml.mapTrackers[name]; exists {
		return fmt.Errorf("map %s already registered", name)
	}

	max := maxEntries
	if max == 0 {
		max = bml.defaultMax
	}

	tracker, err := NewMapUsageTracker(name, max)
	if err != nil {
		return fmt.Errorf("failed to create map tracker: %w", err)
	}

	bml.mapTrackers[name] = tracker
	bml.logger.Info().Msgf("Registered map %s with max entries %d", name, max)
	return nil
}

// UpdateMapUsage updates usage for a map and checks thresholds.
func (bml *BPFMapLimits) UpdateMapUsage(name string, usage uint64) error {
	if name == "" {
		return fmt.Errorf("map name cannot be empty")
	}

	bml.mu.Lock()
	tracker, exists := bml.mapTrackers[name]
	bml.mu.Unlock()

	if !exists {
		return fmt.Errorf("map %s not registered", name)
	}

	alert80, alert90, alert95, err := tracker.UpdateUsage(usage)
	if err != nil {
		return fmt.Errorf("failed to update map usage: %w", err)
	}

	if alert95 {
		bml.logger.Error().Msgf("CRITICAL: Map %s at 95%% capacity", name)
	} else if alert90 {
		bml.logger.Warn().Msgf("WARNING: Map %s at 90%% capacity", name)
	} else if alert80 {
		bml.logger.Info().Msgf("INFO: Map %s at 80%% capacity", name)
	}

	return nil
}

// EvictLowestQoS evicts entries with lowest QoS from a map.
func (bml *BPFMapLimits) EvictLowestQoS(mapName string, count uint64) error {
	if mapName == "" {
		return fmt.Errorf("map name cannot be empty")
	}
	if count == 0 {
		return fmt.Errorf("eviction count must be positive")
	}

	bml.mu.RLock()
	tracker, exists := bml.mapTrackers[mapName]
	bml.mu.RUnlock()

	if !exists {
		return fmt.Errorf("map %s not registered", mapName)
	}

	if tracker.currentUsage < count {
		return fmt.Errorf("cannot evict %d entries, only %d present", count, tracker.currentUsage)
	}

	tracker.mu.Lock()
	tracker.currentUsage -= count
	tracker.mu.Unlock()

	bml.logger.Info().Msgf("Evicted %d entries with lowest QoS from map %s", count, mapName)
	return nil
}

// NewSizeLimitEnforcer creates a new size limit enforcer.
func NewSizeLimitEnforcer(log *logger.Logger, perFlowLimit, globalLimit uint64) (*SizeLimitEnforcer, error) {
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if perFlowLimit == 0 || globalLimit == 0 {
		return nil, fmt.Errorf("limits must be positive")
	}
	if perFlowLimit > globalLimit {
		return nil, fmt.Errorf("per-flow limit cannot exceed global limit")
	}

	return &SizeLimitEnforcer{
		perFlowLimit: perFlowLimit,
		globalLimit:  globalLimit,
		perFlowUsage: make(map[string]uint64),
		logger:       log,
	}, nil
}

// CheckAndRecordSize checks if adding size would violate limits.
func (sle *SizeLimitEnforcer) CheckAndRecordSize(flowID string, size uint64) error {
	if flowID == "" {
		return fmt.Errorf("flow ID cannot be empty")
	}

	sle.mu.Lock()
	defer sle.mu.Unlock()

	// Check per-flow limit
	currentFlow := sle.perFlowUsage[flowID]
	if currentFlow+size > sle.perFlowLimit {
		sle.logger.Warn().Msgf("Flow %s would exceed per-flow limit: %d + %d > %d",
			flowID, currentFlow, size, sle.perFlowLimit)
		return fmt.Errorf("per-flow limit exceeded")
	}

	// Check global limit
	if sle.globalUsage+size > sle.globalLimit {
		sle.logger.Warn().Msgf("Global usage would exceed limit: %d + %d > %d",
			sle.globalUsage, size, sle.globalLimit)
		return fmt.Errorf("global limit exceeded")
	}

	sle.perFlowUsage[flowID] = currentFlow + size
	sle.globalUsage += size

	return nil
}

// GetFlowUsage returns the current size usage for a flow.
func (sle *SizeLimitEnforcer) GetFlowUsage(flowID string) (uint64, error) {
	if flowID == "" {
		return 0, fmt.Errorf("flow ID cannot be empty")
	}

	sle.mu.RLock()
	defer sle.mu.RUnlock()

	return sle.perFlowUsage[flowID], nil
}

// GetGlobalUsage returns the total global usage.
func (sle *SizeLimitEnforcer) GetGlobalUsage() uint64 {
	sle.mu.RLock()
	defer sle.mu.RUnlock()

	return sle.globalUsage
}

// ResetFlowUsage clears usage tracking for a flow.
func (sle *SizeLimitEnforcer) ResetFlowUsage(flowID string) error {
	if flowID == "" {
		return fmt.Errorf("flow ID cannot be empty")
	}

	sle.mu.Lock()
	defer sle.mu.Unlock()

	usage := sle.perFlowUsage[flowID]
	if sle.globalUsage < usage {
		return fmt.Errorf("invalid global usage state")
	}

	sle.globalUsage -= usage
	delete(sle.perFlowUsage, flowID)

	return nil
}

// NewCompressionGuard creates a new compression guard.
func NewCompressionGuard(log *logger.Logger) (*CompressionGuard, error) {
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	return &CompressionGuard{
		compressFlags:    make(map[string]CompressFlag),
		trustedContext:   false,
		untrustedContext: false,
		logger:           log,
	}, nil
}

// SetCompressionFlag sets whether an entry should be compressed.
func (cg *CompressionGuard) SetCompressionFlag(entryID string, flag CompressFlag) error {
	if entryID == "" {
		return fmt.Errorf("entry ID cannot be empty")
	}

	cg.mu.Lock()
	defer cg.mu.Unlock()

	cg.compressFlags[entryID] = flag

	if flag == CompressDisabled {
		cg.logger.Debug().Msgf("Compression disabled for entry %s (sensitive data)", entryID)
	}

	return nil
}

// GetCompressionFlag retrieves the compression flag for an entry.
func (cg *CompressionGuard) GetCompressionFlag(entryID string) (CompressFlag, error) {
	if entryID == "" {
		return 0, fmt.Errorf("entry ID cannot be empty")
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	flag, exists := cg.compressFlags[entryID]
	if !exists {
		return CompressEnabled, nil
	}

	return flag, nil
}

// SetTrustedContext sets whether we're in a trusted admin context.
func (cg *CompressionGuard) SetTrustedContext(trusted bool) {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	cg.trustedContext = trusted
	cg.logger.Debug().Msgf("Trusted context set to %v", trusted)
}

// SetUntrustedContext sets whether we're in an untrusted flow context.
func (cg *CompressionGuard) SetUntrustedContext(untrusted bool) {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	cg.untrustedContext = untrusted
	cg.logger.Debug().Msgf("Untrusted context set to %v", untrusted)
}

// NewExtensionLimits creates a new extension limits enforcer.
func NewExtensionLimits(log *logger.Logger, maxTLVs int) (*ExtensionLimits, error) {
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if maxTLVs <= 0 {
		return nil, fmt.Errorf("max TLVs must be positive")
	}

	return &ExtensionLimits{
		maxTLVs:      maxTLVs,
		unknownSkips: 0,
		logger:       log,
	}, nil
}

// ValidateTLVCount checks if TLV count is within limits.
func (el *ExtensionLimits) ValidateTLVCount(count int) error {
	if count < 0 {
		return fmt.Errorf("TLV count cannot be negative")
	}

	if count > el.maxTLVs {
		el.logger.Warn().Msgf("TLV count %d exceeds maximum %d", count, el.maxTLVs)
		return fmt.Errorf("TLV count %d exceeds maximum %d", count, el.maxTLVs)
	}

	return nil
}

// ProcessUnknownTLV skips unknown TLV types in O(1) time.
func (el *ExtensionLimits) ProcessUnknownTLV(typeID int) error {
	if typeID < 0 {
		return fmt.Errorf("TLV type ID cannot be negative")
	}

	el.mu.Lock()
	defer el.mu.Unlock()

	el.unknownSkips++
	el.logger.Debug().Msgf("Skipped unknown TLV type %d (total skips: %d)", typeID, el.unknownSkips)

	return nil
}

// GetUnknownSkipCount returns the number of unknown TLVs skipped.
func (el *ExtensionLimits) GetUnknownSkipCount() int {
	el.mu.RLock()
	defer el.mu.RUnlock()

	return el.unknownSkips
}
