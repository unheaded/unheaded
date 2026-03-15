// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophiasync

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// TableVersion represents the version of the Sophia state table.
type TableVersion uint64

// DictEntry represents a dictionary entry in the Sophia table.
type DictEntry struct {
	Index uint32
	Value string
}

// DictionaryDelta represents changes to the Sophia dictionary state.
type DictionaryDelta struct {
	Version   TableVersion
	Additions []DictEntry
	Removals  []uint32
	Signature []byte
}

// CompressedDelta is a varint-encoded dictionary update.
type CompressedDelta struct {
	Version      TableVersion
	EncodedData  []byte
	Signature    []byte
}

// BackpressureLevel represents the level of backpressure in the system.
type BackpressureLevel int

const (
	BackpressureNone BackpressureLevel = iota
	BackpressureLow
	BackpressureMedium
	BackpressureHigh
	BackpressureCritical
)

// HopAckState tracks acknowledgment state from a hop.
type HopAckState struct {
	HopID        string
	TableVersion TableVersion
	AcknowledgedAt time.Time
}

// EncoderStream distributes delta updates to hops via Wotan control topic.
type EncoderStream struct {
	mu             sync.RWMutex
	wotanClient    *wotanClient.Client
	controlTopic   string
	logger         *logger.Logger
	pendingAcks    map[string]TableVersion
	ackMutex       sync.Mutex
}

// DecoderStream handles acknowledgments of table state from hops.
type DecoderStream struct {
	mu      sync.RWMutex
	hopID   string
	version TableVersion
	logger  *logger.Logger
}

// SyncManager tracks table version per endpoint and validates state changes.
type SyncManager struct {
	mu                sync.RWMutex
	endpointVersions  map[string]TableVersion
	encoder           *EncoderStream
	decoder           *DecoderStream
	gracePeriod       time.Duration
	expiredVersions   map[TableVersion]time.Time
	currentVersion    TableVersion
	logger            *logger.Logger
	wotanClient       *wotanClient.Client
}

// NewEncoderStream creates a new encoder stream.
func NewEncoderStream(client *wotanClient.Client, controlTopic string, log *logger.Logger) (*EncoderStream, error) {
	if client == nil {
		return nil, fmt.Errorf("wotan client cannot be nil")
	}
	if controlTopic == "" {
		return nil, fmt.Errorf("control topic cannot be empty")
	}
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	return &EncoderStream{
		wotanClient:  client,
		controlTopic: controlTopic,
		logger:       log,
		pendingAcks:  make(map[string]TableVersion),
	}, nil
}

// DistributeDelta sends a delta to all hops via Wotan control topic.
func (es *EncoderStream) DistributeDelta(ctx context.Context, delta *DictionaryDelta) error {
	if delta == nil {
		return fmt.Errorf("delta cannot be nil")
	}

	es.mu.Lock()
	defer es.mu.Unlock()

	payload, err := encodeDelta(delta)
	if err != nil {
		return fmt.Errorf("failed to encode delta: %w", err)
	}

	es.logger.Info().Msgf("Distributing delta version %d with %d additions and %d removals",
		delta.Version, len(delta.Additions), len(delta.Removals))

	if err := es.wotanClient.Publish(ctx, es.controlTopic, payload); err != nil {
		return fmt.Errorf("failed to publish delta to control topic: %w", err)
	}

	return nil
}

// RegisterPendingAck registers a hop as awaiting acknowledgment.
func (es *EncoderStream) RegisterPendingAck(hopID string, version TableVersion) {
	es.ackMutex.Lock()
	defer es.ackMutex.Unlock()
	es.pendingAcks[hopID] = version
}

// AcknowledgeAck removes a hop from pending acknowledgments.
func (es *EncoderStream) AcknowledgeAck(hopID string) {
	es.ackMutex.Lock()
	defer es.ackMutex.Unlock()
	delete(es.pendingAcks, hopID)
}

// PendingAcksCount returns the number of pending acknowledgments.
func (es *EncoderStream) PendingAcksCount() int {
	es.ackMutex.Lock()
	defer es.ackMutex.Unlock()
	return len(es.pendingAcks)
}

// NewDecoderStream creates a new decoder stream.
func NewDecoderStream(hopID string, log *logger.Logger) (*DecoderStream, error) {
	if hopID == "" {
		return nil, fmt.Errorf("hop ID cannot be empty")
	}
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	return &DecoderStream{
		hopID:   hopID,
		version: 0,
		logger:  log,
	}, nil
}

// UpdateState updates the acknowledged table state.
func (ds *DecoderStream) UpdateState(version TableVersion) error {
	if version == 0 {
		return fmt.Errorf("version cannot be zero")
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.version = version
	ds.logger.Info().Msgf("Updated hop %s to table version %d", ds.hopID, version)
	return nil
}

// GetState returns the current acknowledged state.
func (ds *DecoderStream) GetState() (hopID string, version TableVersion) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.hopID, ds.version
}

// NewSyncManager creates a new sync manager.
func NewSyncManager(client *wotanClient.Client, controlTopic string, log *logger.Logger) (*SyncManager, error) {
	if client == nil {
		return nil, fmt.Errorf("wotan client cannot be nil")
	}
	if controlTopic == "" {
		return nil, fmt.Errorf("control topic cannot be empty")
	}
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	encoder, err := NewEncoderStream(client, controlTopic, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create encoder stream: %w", err)
	}

	return &SyncManager{
		endpointVersions: make(map[string]TableVersion),
		encoder:          encoder,
		gracePeriod:      60 * time.Second,
		expiredVersions:  make(map[TableVersion]time.Time),
		currentVersion:   0,
		logger:           log,
		wotanClient:      client,
	}, nil
}

// SetGracePeriod sets the grace period for old version retention.
func (sm *SyncManager) SetGracePeriod(period time.Duration) error {
	if period < 0 {
		return fmt.Errorf("grace period cannot be negative")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.gracePeriod = period
	sm.logger.Info().Msgf("Grace period set to %v", period)
	return nil
}

// ApplyDelta atomically applies a delta to the table state.
func (sm *SyncManager) ApplyDelta(delta *DictionaryDelta) error {
	if delta == nil {
		return fmt.Errorf("delta cannot be nil")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Validate delta version
	if delta.Version != sm.currentVersion+1 {
		return fmt.Errorf("invalid delta version: expected %d, got %d", sm.currentVersion+1, delta.Version)
	}

	// Validate signature
	if len(delta.Signature) > 0 {
		expectedSig := computeSignature(delta)
		if !bytesEqual(expectedSig, delta.Signature) {
			return fmt.Errorf("delta signature mismatch")
		}
	}

	// Update current version
	sm.currentVersion = delta.Version

	// Record old version as expired
	sm.expiredVersions[delta.Version-1] = time.Now()

	sm.logger.Info().Msgf("Applied delta version %d (additions: %d, removals: %d)",
		delta.Version, len(delta.Additions), len(delta.Removals))

	return nil
}

// WaitForAcks blocks until all hops acknowledge the current table version or timeout occurs.
func (sm *SyncManager) WaitForAcks(ctx context.Context, timeout time.Duration) error {
	if timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		case <-time.After(timeout):
			pending := sm.encoder.PendingAcksCount()
			return fmt.Errorf("timeout waiting for acknowledgments, %d hops still pending", pending)
		case <-ticker.C:
			if sm.encoder.PendingAcksCount() == 0 {
				sm.logger.Info().Msgf("All hops acknowledged table version %d", sm.currentVersion)
				return nil
			}

			if time.Now().After(deadline) {
				pending := sm.encoder.PendingAcksCount()
				return fmt.Errorf("timeout waiting for acknowledgments, %d hops still pending", pending)
			}
		}
	}
}

// RegisterEndpoint registers an endpoint and initializes its version tracking.
func (sm *SyncManager) RegisterEndpoint(endpointID string) error {
	if endpointID == "" {
		return fmt.Errorf("endpoint ID cannot be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.endpointVersions[endpointID]; exists {
		return fmt.Errorf("endpoint %s already registered", endpointID)
	}

	sm.endpointVersions[endpointID] = 0
	sm.logger.Info().Msgf("Registered endpoint %s", endpointID)
	return nil
}

// UpdateEndpointVersion updates the tracked version for an endpoint.
func (sm *SyncManager) UpdateEndpointVersion(endpointID string, version TableVersion) error {
	if endpointID == "" {
		return fmt.Errorf("endpoint ID cannot be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.endpointVersions[endpointID]; !exists {
		return fmt.Errorf("endpoint %s not registered", endpointID)
	}

	sm.endpointVersions[endpointID] = version
	sm.logger.Debug().Msgf("Updated endpoint %s to version %d", endpointID, version)
	return nil
}

// GetEndpointVersion returns the current version for an endpoint.
func (sm *SyncManager) GetEndpointVersion(endpointID string) (TableVersion, error) {
	if endpointID == "" {
		return 0, fmt.Errorf("endpoint ID cannot be empty")
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	version, exists := sm.endpointVersions[endpointID]
	if !exists {
		return 0, fmt.Errorf("endpoint %s not registered", endpointID)
	}

	return version, nil
}

// CleanupExpiredVersions removes old versions that have exceeded the grace period.
func (sm *SyncManager) CleanupExpiredVersions() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	removed := 0

	for version, expiredAt := range sm.expiredVersions {
		if now.Sub(expiredAt) > sm.gracePeriod {
			delete(sm.expiredVersions, version)
			removed++
		}
	}

	if removed > 0 {
		sm.logger.Debug().Msgf("Cleaned up %d expired versions", removed)
	}

	return nil
}

// CurrentVersion returns the current table version.
func (sm *SyncManager) CurrentVersion() TableVersion {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.currentVersion
}

// Helper functions

func encodeDelta(delta *DictionaryDelta) ([]byte, error) {
	if delta == nil {
		return nil, fmt.Errorf("delta cannot be nil")
	}

	buf := make([]byte, 0, 1024)

	// Encode version
	versionBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(versionBytes, uint64(delta.Version))
	buf = append(buf, versionBytes...)

	// Encode additions count
	addCount := make([]byte, 4)
	binary.BigEndian.PutUint32(addCount, uint32(len(delta.Additions)))
	buf = append(buf, addCount...)

	// Encode additions
	for _, entry := range delta.Additions {
		indexBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(indexBytes, entry.Index)
		buf = append(buf, indexBytes...)

		lenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBytes, uint32(len(entry.Value)))
		buf = append(buf, lenBytes...)

		buf = append(buf, []byte(entry.Value)...)
	}

	// Encode removals count
	remCount := make([]byte, 4)
	binary.BigEndian.PutUint32(remCount, uint32(len(delta.Removals)))
	buf = append(buf, remCount...)

	// Encode removals
	for _, idx := range delta.Removals {
		idxBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(idxBytes, idx)
		buf = append(buf, idxBytes...)
	}

	// Encode signature length and signature
	sigLen := make([]byte, 4)
	binary.BigEndian.PutUint32(sigLen, uint32(len(delta.Signature)))
	buf = append(buf, sigLen...)
	buf = append(buf, delta.Signature...)

	return buf, nil
}

func computeSignature(delta *DictionaryDelta) []byte {
	h := sha256.New()

	// Hash version
	versionBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(versionBytes, uint64(delta.Version))
	h.Write(versionBytes)

	// Hash additions
	for _, entry := range delta.Additions {
		indexBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(indexBytes, entry.Index)
		h.Write(indexBytes)
		h.Write([]byte(entry.Value))
	}

	// Hash removals
	for _, idx := range delta.Removals {
		idxBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(idxBytes, idx)
		h.Write(idxBytes)
	}

	return h.Sum(nil)
}

func bytesEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
