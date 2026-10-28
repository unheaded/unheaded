// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophiasync

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
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
//
// Signature is an ML-DSA-65 signature (signing.go) by PublicKey over the
// delta's canonical bytes, Algorithm and SignedAtUnixMs; all four are empty
// on an unsigned delta. Until M-149 it held SHA-256 of the delta, which
// authenticated nothing: that digest is Digest now.
type DictionaryDelta struct {
	Version        TableVersion
	Additions      []DictEntry
	Removals       []uint32
	Signature      []byte
	PublicKey      []byte
	Algorithm      string
	SignedAtUnixMs int64
}

// CompressedDelta is a varint-encoded dictionary update.
type CompressedDelta struct {
	Version     TableVersion
	EncodedData []byte
	Signature   []byte
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
	HopID          string
	TableVersion   TableVersion
	AcknowledgedAt time.Time
}

// EncoderStream distributes delta updates to hops via Wotan control topic.
type EncoderStream struct {
	mu           sync.RWMutex
	wotanClient  *wotanClient.Client
	controlTopic string
	logger       *logger.Logger
	pendingAcks  map[string]TableVersion
	ackMutex     sync.Mutex
}

// DecoderStream handles acknowledgments of table state from hops.
type DecoderStream struct {
	mu      sync.RWMutex
	hopID   string
	version TableVersion
	logger  *logger.Logger
}

// SyncManager tracks table version per endpoint and validates state changes.
//
// (decoder *DecoderStream was previously held here but never used — incoming
// QPACK frames are decoded ad-hoc per receive callback. Re-add when there's
// a long-lived decoder state to attach.)
type SyncManager struct {
	mu               sync.RWMutex
	endpointVersions map[string]TableVersion
	encoder          *EncoderStream
	gracePeriod      time.Duration
	expiredVersions  map[TableVersion]time.Time
	currentVersion   TableVersion
	logger           *logger.Logger
	wotanClient      *wotanClient.Client

	// Delta signatures (M-149): the keys trusted to sign deltas, and
	// whether an unsigned delta is refused (true) or applied, counted and
	// logged (false, observe mode: the default).
	trustedKeys      [][]byte
	requireSignature bool
	now              func() time.Time
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
		now:              time.Now,
	}, nil
}

// SetSignaturePolicy sets the public keys trusted to sign deltas and whether
// unsigned deltas are refused (require) or applied in observe mode. With no
// trusted keys every signed delta is refused (fail closed).
func (sm *SyncManager) SetSignaturePolicy(trusted [][]byte, require bool) {
	keys := make([][]byte, 0, len(trusted))
	for _, k := range trusted {
		keys = append(keys, bytes.Clone(k))
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.trustedKeys = keys
	sm.requireSignature = require
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

	if err := sm.checkSignature(delta); err != nil {
		return err
	}

	// Update current version
	sm.currentVersion = delta.Version

	// Record old version as expired
	sm.expiredVersions[delta.Version-1] = time.Now()

	sm.logger.Info().Msgf("Applied delta version %d (additions: %d, removals: %d)",
		delta.Version, len(delta.Additions), len(delta.Removals))

	return nil
}

// checkSignature applies the delta signature policy. Callers hold sm.mu,
// and ApplyDelta has already required delta.Version == currentVersion+1,
// which is what stops a replay: a delta is applied once, as the next
// version, however old its signature is.
func (sm *SyncManager) checkSignature(delta *DictionaryDelta) error {
	if len(delta.Signature) == 0 {
		deltaUnsigned.Inc()
		if sm.requireSignature {
			sm.logger.Warn().
				Uint64("version", uint64(delta.Version)).
				Str("reason", "unsigned").
				Msg("sophia_delta_refused")
			return fmt.Errorf("delta version %d: %w", delta.Version, ErrUnsignedDelta)
		}
		sm.logger.Warn().
			Uint64("version", uint64(delta.Version)).
			Str("mode", "observe").
			Str("require_signature_env", RequireSignatureEnv).
			Msg("sophia_delta_unsigned_applied")
		return nil
	}
	if err := Verify(delta, sm.trustedKeys); err != nil {
		deltaSignatureFailures.Inc()
		sm.logger.Warn().
			Err(err).
			Uint64("version", uint64(delta.Version)).
			Str("reason", "signature").
			Msg("sophia_delta_refused")
		return fmt.Errorf("delta version %d signature: %w", delta.Version, err)
	}
	now := sm.now().UnixMilli()
	if skew := MaxSkew.Milliseconds(); delta.SignedAtUnixMs < now-skew || delta.SignedAtUnixMs > now+skew {
		deltaStale.Inc()
		sm.logger.Warn().
			Uint64("version", uint64(delta.Version)).
			Int64("signed_at_unix_ms", delta.SignedAtUnixMs).
			Int64("now_unix_ms", now).
			Dur("max_skew", MaxSkew).
			Msg("sophia_delta_stale_applied")
	}
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

// Wire encoding (all integers big-endian):
//
//	version u64 | n_add u32 | n_add x (index u32 | len u32 | value) |
//	n_rem u32 | n_rem x index u32 |
//	sig_len u32 | signature | key_len u32 | public_key |
//	alg_len u8 | algorithm | signed_at_unix_ms i64
//
// The part before sig_len is Canonical. An unsigned delta has zero-length
// signature, key and algorithm and signed_at 0.

// Decoder limits: the input is hostile. A signature or key larger than any
// ML-DSA parameter set's is refused before it is copied.
const (
	maxDeltaBytes   = 16 << 20
	maxSignatureLen = 8192
	maxPublicKeyLen = 4096
)

func encodeDelta(delta *DictionaryDelta) ([]byte, error) {
	if delta == nil {
		return nil, fmt.Errorf("delta cannot be nil")
	}
	if len(delta.Algorithm) > 255 {
		return nil, fmt.Errorf("algorithm name longer than 255 bytes")
	}
	buf := appendBody(make([]byte, 0, 1024), delta)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(delta.Signature))) // #nosec G115 -- bounded by the length of an already-allocated slice
	buf = append(buf, delta.Signature...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(delta.PublicKey))) // #nosec G115 -- bounded by the length of an already-allocated slice
	buf = append(buf, delta.PublicKey...)
	buf = append(buf, byte(len(delta.Algorithm)))
	buf = append(buf, delta.Algorithm...)
	buf = binary.BigEndian.AppendUint64(buf, uint64(delta.SignedAtUnixMs)) // #nosec G115 -- two's-complement round trip, decoded as int64
	return buf, nil
}

// appendBody appends the canonical part of the encoding: version,
// additions, removals.
func appendBody(buf []byte, delta *DictionaryDelta) []byte {
	buf = binary.BigEndian.AppendUint64(buf, uint64(delta.Version))
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(delta.Additions))) // #nosec G115 -- bounded by the length of an already-allocated slice
	for _, entry := range delta.Additions {
		buf = binary.BigEndian.AppendUint32(buf, entry.Index)
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(entry.Value))) // #nosec G115 -- bounded by the length of an already-allocated slice
		buf = append(buf, entry.Value...)
	}
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(delta.Removals))) // #nosec G115 -- bounded by the length of an already-allocated slice
	for _, idx := range delta.Removals {
		buf = binary.BigEndian.AppendUint32(buf, idx)
	}
	return buf
}

var errTruncated = errors.New("sophiasync: delta truncated")

// reader is a bounds-checked cursor over hostile bytes.
type reader struct {
	b   []byte
	off int
}

func (r *reader) remaining() int { return len(r.b) - r.off }

func (r *reader) take(n int) ([]byte, error) {
	if n < 0 || n > r.remaining() {
		return nil, errTruncated
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out, nil
}

func (r *reader) u8() (uint8, error) {
	b, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *reader) u32() (uint32, error) {
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (r *reader) u64() (uint64, error) {
	b, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}

// lenPrefixed reads a u32 length (at most limit) and that many bytes.
func (r *reader) lenPrefixed(limit int) ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if uint64(n) > uint64(limit) {
		return nil, fmt.Errorf("sophiasync: length %d over the limit %d", n, limit)
	}
	return r.take(int(n))
}

// DecodeDelta parses a delta encoded by DistributeDelta. Every count and
// length is checked against the bytes that remain before anything is
// allocated, and trailing bytes are refused. It does not verify the
// signature: that is Verify (or SyncManager.ApplyDelta).
func DecodeDelta(b []byte) (*DictionaryDelta, error) {
	if len(b) > maxDeltaBytes {
		return nil, fmt.Errorf("sophiasync: delta of %d bytes over the %d limit", len(b), maxDeltaBytes)
	}
	r := &reader{b: b}
	version, err := r.u64()
	if err != nil {
		return nil, err
	}
	d := &DictionaryDelta{Version: TableVersion(version)}

	nAdd, err := r.u32()
	if err != nil {
		return nil, err
	}
	if uint64(nAdd)*8 > uint64(r.remaining()) { // each addition is at least 8 bytes
		return nil, errTruncated
	}
	if nAdd > 0 {
		d.Additions = make([]DictEntry, 0, nAdd)
	}
	for i := uint32(0); i < nAdd; i++ {
		idx, err := r.u32()
		if err != nil {
			return nil, err
		}
		val, err := r.lenPrefixed(r.remaining())
		if err != nil {
			return nil, err
		}
		d.Additions = append(d.Additions, DictEntry{Index: idx, Value: string(val)})
	}

	nRem, err := r.u32()
	if err != nil {
		return nil, err
	}
	if uint64(nRem)*4 > uint64(r.remaining()) {
		return nil, errTruncated
	}
	if nRem > 0 {
		d.Removals = make([]uint32, 0, nRem)
	}
	for i := uint32(0); i < nRem; i++ {
		idx, err := r.u32()
		if err != nil {
			return nil, err
		}
		d.Removals = append(d.Removals, idx)
	}

	sig, err := r.lenPrefixed(maxSignatureLen)
	if err != nil {
		return nil, err
	}
	key, err := r.lenPrefixed(maxPublicKeyLen)
	if err != nil {
		return nil, err
	}
	algLen, err := r.u8()
	if err != nil {
		return nil, err
	}
	alg, err := r.take(int(algLen))
	if err != nil {
		return nil, err
	}
	at, err := r.u64()
	if err != nil {
		return nil, err
	}
	if r.remaining() != 0 {
		return nil, fmt.Errorf("sophiasync: %d trailing bytes after the delta", r.remaining())
	}
	if len(sig) > 0 {
		d.Signature = bytes.Clone(sig)
	}
	if len(key) > 0 {
		d.PublicKey = bytes.Clone(key)
	}
	d.Algorithm = string(alg)
	d.SignedAtUnixMs = int64(at) // #nosec G115 -- two's-complement round trip of the encoder's uint64(int64)
	return d, nil
}
