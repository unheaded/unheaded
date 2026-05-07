// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package shield

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/crypto/pqc"
)

// SecurityHardening integrates all PQC security mitigations into a single
// validation pipeline. It wires existing guards (AlgoConfusionGuard,
// SigRefRateLimiter, FlowSeqTracker) with new protections (PacketBitmapWindow,
// EntropyMonitor, TOCTOUCache).
type SecurityHardening struct {
	mu sync.RWMutex

	algoGuard      *pqc.AlgoConfusionGuard
	sigRefLimiter  *pqc.SigRefRateLimiter
	replayWindow   *PacketBitmapWindow
	entropyMonitor *EntropyMonitor
	toctouCache    *TOCTOUCache
}

// NewSecurityHardening creates a new security hardening module with
// sensible defaults.
func NewSecurityHardening() *SecurityHardening {
	return &SecurityHardening{
		algoGuard:      pqc.NewAlgoConfusionGuard(),
		sigRefLimiter:  pqc.NewSigRefRateLimiter(10000, 1<<24, 0.9),
		replayWindow:   NewPacketBitmapWindow(),
		entropyMonitor: NewEntropyMonitor(256),
		toctouCache:    NewTOCTOUCache(30 * time.Second),
	}
}

// Validate runs all applicable security checks for a packet.
// Returns nil if all checks pass, or an error describing the first failure.
func (sh *SecurityHardening) Validate(pkt *PQCPacketInfo, flowID string) error {
	// 1. Algorithm confusion guard.
	if err := sh.algoGuard.Check(flowID, pkt.Flags); err != nil {
		return fmt.Errorf("shield: security: %w", err)
	}

	// 2. SigRef rate limiting.
	if !sh.sigRefLimiter.Allow() {
		return fmt.Errorf("shield: security: SigRef rate limit exceeded")
	}

	// 3. Compact replay window.
	if !sh.replayWindow.Check(flowID, pkt.SeqNum) {
		return fmt.Errorf("shield: security: replay detected for flow %s seq %d", flowID, pkt.SeqNum)
	}

	// 4. TOCTOU cache check.
	if sh.toctouCache.IsVerified(flowID, pkt.SigRef) {
		// Already verified — skip re-verification (TOCTOU prevention).
		return nil
	}

	return nil
}

// ValidateWithAlgo runs security checks including algo-specific validation.
func (sh *SecurityHardening) ValidateWithAlgo(pkt *PQCPacketInfo, flowID string, algoID uint8) error {
	// Algorithm confusion guard with actual algo ID.
	if err := sh.algoGuard.Check(flowID, algoID); err != nil {
		return fmt.Errorf("shield: security: %w", err)
	}

	// SigRef rate limiting.
	if !sh.sigRefLimiter.Allow() {
		return fmt.Errorf("shield: security: SigRef rate limit exceeded")
	}

	// Compact replay window.
	if !sh.replayWindow.Check(flowID, pkt.SeqNum) {
		return fmt.Errorf("shield: security: replay detected for flow %s seq %d", flowID, pkt.SeqNum)
	}

	return nil
}

// CheckEntropy verifies that the system entropy pool is sufficient.
func (sh *SecurityHardening) CheckEntropy() error {
	return sh.entropyMonitor.Check()
}

// PinVerification records a successful verification result in the TOCTOU
// cache to prevent time-of-check-time-of-use races.
func (sh *SecurityHardening) PinVerification(flowID string, sigRef uint32) {
	sh.toctouCache.Pin(flowID, sigRef)
}

// EntropyBits returns the current system entropy pool size.
func (sh *SecurityHardening) EntropyBits() int {
	return sh.entropyMonitor.AvailableBits()
}

// PacketBitmapWindow implements a compact 64-bit sliding window for
// per-flow replay protection. This is more memory-efficient than the
// full 16384-bit SeqTracker for lightweight use cases.
type PacketBitmapWindow struct {
	mu      sync.Mutex
	windows map[string]*bitmapEntry
}

type bitmapEntry struct {
	maxSeq uint16
	bitmap uint64 // 64-bit sliding window
}

// NewPacketBitmapWindow creates a new compact replay window.
func NewPacketBitmapWindow() *PacketBitmapWindow {
	return &PacketBitmapWindow{
		windows: make(map[string]*bitmapEntry),
	}
}

// Check validates and marks a sequence number in the per-flow bitmap.
// Returns true if the sequence number is fresh (not a replay).
func (w *PacketBitmapWindow) Check(flowID string, seqNum uint16) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry, ok := w.windows[flowID]
	if !ok {
		entry = &bitmapEntry{}
		w.windows[flowID] = entry
	}

	if seqNum > entry.maxSeq {
		// Advance the window.
		diff := seqNum - entry.maxSeq
		if diff >= 64 {
			entry.bitmap = 0
		} else {
			entry.bitmap <<= diff
		}
		entry.maxSeq = seqNum
		entry.bitmap |= 1 // Mark current as seen
		return true
	}

	// Check if within window.
	diff := entry.maxSeq - seqNum
	if diff >= 64 {
		return false // Too old
	}

	bit := uint64(1) << diff
	if entry.bitmap&bit != 0 {
		return false // Already seen
	}

	entry.bitmap |= bit
	return true
}

// FlowCount returns the number of tracked flows.
func (w *PacketBitmapWindow) FlowCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.windows)
}

// EntropyMonitor checks the system entropy pool to ensure sufficient
// randomness is available for cryptographic operations. Alerts if the
// pool drops below a configurable threshold.
type EntropyMonitor struct {
	mu          sync.Mutex
	minBits     int
	lastCheck   time.Time
	lastBits    int
	entropyFile string
}

// NewEntropyMonitor creates a new entropy monitor.
// minBits is the minimum acceptable entropy pool size in bits.
func NewEntropyMonitor(minBits int) *EntropyMonitor {
	return &EntropyMonitor{
		minBits:     minBits,
		entropyFile: "/proc/sys/kernel/random/entropy_avail",
	}
}

// Check verifies that sufficient entropy is available.
func (em *EntropyMonitor) Check() error {
	bits := em.AvailableBits()
	if bits < em.minBits {
		return fmt.Errorf("shield: entropy: available bits %d below minimum %d", bits, em.minBits)
	}
	return nil
}

// AvailableBits reads the current entropy pool size from /proc.
// Returns -1 if the entropy file cannot be read (non-Linux systems).
func (em *EntropyMonitor) AvailableBits() int {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Cache for 1 second to avoid excessive /proc reads.
	if time.Since(em.lastCheck) < time.Second {
		return em.lastBits
	}

	data, err := os.ReadFile(em.entropyFile)
	if err != nil {
		em.lastBits = -1
		em.lastCheck = time.Now()
		return -1
	}

	bits, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		em.lastBits = -1
		em.lastCheck = time.Now()
		return -1
	}

	em.lastBits = bits
	em.lastCheck = time.Now()
	return bits
}

// TOCTOUCache pins verification results with a TTL to prevent
// time-of-check-time-of-use races. Once a packet is verified,
// its result is cached so that subsequent lookups within the TTL
// see a consistent result.
type TOCTOUCache struct {
	mu      sync.RWMutex
	entries map[string]*toctouEntry
	ttl     time.Duration
}

type toctouEntry struct {
	sigRef     uint32
	verifiedAt time.Time
}

// NewTOCTOUCache creates a new TOCTOU prevention cache.
func NewTOCTOUCache(ttl time.Duration) *TOCTOUCache {
	return &TOCTOUCache{
		entries: make(map[string]*toctouEntry),
		ttl:     ttl,
	}
}

// Pin records a successful verification result.
func (c *TOCTOUCache) Pin(flowID string, sigRef uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s:%d", flowID, sigRef)
	c.entries[key] = &toctouEntry{
		sigRef:     sigRef,
		verifiedAt: time.Now(),
	}
}

// IsVerified checks if a flow+sigRef was recently verified.
func (c *TOCTOUCache) IsVerified(flowID string, sigRef uint32) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s:%d", flowID, sigRef)
	entry, ok := c.entries[key]
	if !ok {
		return false
	}
	return time.Since(entry.verifiedAt) < c.ttl
}

// Evict removes expired entries from the cache.
func (c *TOCTOUCache) Evict() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	evicted := 0
	for key, entry := range c.entries {
		if now.Sub(entry.verifiedAt) >= c.ttl {
			delete(c.entries, key)
			evicted++
		}
	}
	return evicted
}

// Size returns the number of cached entries.
func (c *TOCTOUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
