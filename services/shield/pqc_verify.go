// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// PQCVerificationStep identifies the 7-point verification check.
type PQCVerificationStep uint8

const (
	StepFlagCheck     PQCVerificationStep = iota + 1 // 1. S+CUSTOM flag check
	StepSigRefLookup                                  // 2. SigRef lookup in Sophia map
	StepKeyRefLookup                                  // 3. KeyRef lookup + trust tier check
	StepAlgoCompliance                                // 4. Algorithm compliance check
	StepCryptoVerify                                  // 5. Cryptographic signature verify
	StepHashPfxMatch                                  // 6. HashPfx match (SHA-256[0:4])
	StepSeqNumReplay                                  // 7. SeqNum replay protection check
)

// String returns the step name.
func (s PQCVerificationStep) String() string {
	names := [...]string{
		"", "flag_check", "sigref_lookup", "keyref_lookup",
		"algo_compliance", "crypto_verify", "hashpfx_match", "seqnum_replay",
	}
	if int(s) < len(names) {
		return names[s]
	}
	return "unknown"
}

// PQCVerifyResult holds the result of PQC verification.
type PQCVerifyResult struct {
	Passed    bool
	FailStep  PQCVerificationStep
	FailMsg   string
	AlgoID    uint8
	KeyRef    uint16
	SigRef    uint32
	Duration  time.Duration
	Tier      uint8
	SeqNum    uint16
}

// PQCPacketInfo contains the PQC-relevant fields extracted from a packet.
type PQCPacketInfo struct {
	Flags    byte     // Monad flags byte
	SigRef   uint32   // SigRef from PQC value
	KeyRef   uint16   // KeyRef from PQC value
	HashPfx  [4]byte  // HashPfx from PQC value
	SeqNum   uint16   // SeqNum from PQC value
	SrcSvcID uint16   // Source service ID
	DstSvcID uint16   // Destination service ID
	PseudoHeader []byte // Signed data (52 bytes)
}

// SigEntry is a locally-cached signature entry.
type SigEntry struct {
	AlgoID    uint8
	ParamSet  uint8
	Tier      uint8
	Signature []byte
	CreatedAt time.Time
}

// KeyEntry is a locally-cached key entry.
type KeyEntry struct {
	AlgoID    uint8
	ParamSet  uint8
	TrustTier uint8
	Status    uint8
	PublicKey []byte
}

// PQCVerifier implements the 7-point PQC verification pipeline.
type PQCVerifier struct {
	mu sync.RWMutex

	// Cached map data (mirrors of Sophia BPF maps)
	signatures map[uint32]*SigEntry
	keys       map[uint16]*KeyEntry

	// Verification policy
	requiredTier uint8
	allowedAlgos map[uint8]bool

	// Replay protection: sliding window per flow
	replayWindows sync.Map // flowKey → *replayWindow

	// Stats
	verifyPass int64
	verifyFail int64

	// Verification function (pluggable for testing)
	cryptoVerify func(algoID uint8, message, signature, pubKey []byte) (bool, error)
}

// NewPQCVerifier creates a new PQC verifier.
func NewPQCVerifier() *PQCVerifier {
	return &PQCVerifier{
		signatures: make(map[uint32]*SigEntry),
		keys:       make(map[uint16]*KeyEntry),
		allowedAlgos: map[uint8]bool{
			0x01: true, // SLH-DSA
			0x02: true, // ML-DSA
			0x03: true, // FN-DSA
		},
	}
}

// Verify runs the 7-point PQC verification pipeline.
func (v *PQCVerifier) Verify(pkt *PQCPacketInfo) *PQCVerifyResult {
	start := time.Now()
	result := &PQCVerifyResult{
		SigRef: pkt.SigRef,
		KeyRef: pkt.KeyRef,
		SeqNum: pkt.SeqNum,
	}

	// Step 1: S+CUSTOM flag check
	if pkt.Flags&0x0A != 0x0A {
		result.FailStep = StepFlagCheck
		result.FailMsg = "PQC flags not set (S+CUSTOM required)"
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}

	// Step 2: SigRef lookup
	v.mu.RLock()
	sigEntry, sigOK := v.signatures[pkt.SigRef]
	v.mu.RUnlock()

	if !sigOK {
		result.FailStep = StepSigRefLookup
		result.FailMsg = fmt.Sprintf("SigRef %d not found in map", pkt.SigRef)
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}
	result.AlgoID = sigEntry.AlgoID
	result.Tier = sigEntry.Tier

	// Step 3: KeyRef lookup + trust tier
	v.mu.RLock()
	keyEntry, keyOK := v.keys[pkt.KeyRef]
	v.mu.RUnlock()

	if !keyOK {
		result.FailStep = StepKeyRefLookup
		result.FailMsg = fmt.Sprintf("KeyRef %d not found in map", pkt.KeyRef)
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}

	if keyEntry.Status != 0x01 && keyEntry.Status != 0x02 { // active or rotating
		result.FailStep = StepKeyRefLookup
		result.FailMsg = fmt.Sprintf("key status %d not valid", keyEntry.Status)
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}

	if keyEntry.TrustTier < v.requiredTier {
		result.FailStep = StepKeyRefLookup
		result.FailMsg = fmt.Sprintf("key trust tier %d below required %d", keyEntry.TrustTier, v.requiredTier)
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}

	// Step 4: Algorithm compliance
	if !v.allowedAlgos[sigEntry.AlgoID] {
		result.FailStep = StepAlgoCompliance
		result.FailMsg = fmt.Sprintf("algorithm 0x%02x not allowed", sigEntry.AlgoID)
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}

	// Step 5: Cryptographic signature verification
	if v.cryptoVerify != nil {
		valid, err := v.cryptoVerify(sigEntry.AlgoID, pkt.PseudoHeader, sigEntry.Signature, keyEntry.PublicKey)
		if err != nil || !valid {
			result.FailStep = StepCryptoVerify
			if err != nil {
				result.FailMsg = fmt.Sprintf("crypto verify error: %v", err)
			} else {
				result.FailMsg = "signature verification failed"
			}
			result.Duration = time.Since(start)
			atomic.AddInt64(&v.verifyFail, 1)
			return result
		}
	}

	// Step 6: HashPfx match (constant-time)
	hash := sha256.Sum256(sigEntry.Signature)
	var expectedPfx [4]byte
	copy(expectedPfx[:], hash[:4])
	if subtle.ConstantTimeCompare(pkt.HashPfx[:], expectedPfx[:]) != 1 {
		result.FailStep = StepHashPfxMatch
		result.FailMsg = "HashPfx mismatch"
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}

	// Step 7: SeqNum replay check
	flowKey := fmt.Sprintf("%d-%d", pkt.SrcSvcID, pkt.DstSvcID)
	if !v.checkReplay(flowKey, pkt.SeqNum) {
		result.FailStep = StepSeqNumReplay
		result.FailMsg = fmt.Sprintf("replay detected: SeqNum %d", pkt.SeqNum)
		result.Duration = time.Since(start)
		atomic.AddInt64(&v.verifyFail, 1)
		return result
	}

	// All 7 steps passed
	result.Passed = true
	result.Duration = time.Since(start)
	atomic.AddInt64(&v.verifyPass, 1)
	return result
}

// LoadSignature adds a signature entry to the verifier's cache.
func (v *PQCVerifier) LoadSignature(sigRef uint32, entry *SigEntry) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.signatures[sigRef] = entry
}

// LoadKey adds a key entry to the verifier's cache.
func (v *PQCVerifier) LoadKey(keyRef uint16, entry *KeyEntry) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys[keyRef] = entry
}

// SetCryptoVerify sets the cryptographic verification function.
func (v *PQCVerifier) SetCryptoVerify(fn func(algoID uint8, message, signature, pubKey []byte) (bool, error)) {
	v.cryptoVerify = fn
}

// SetRequiredTier sets the minimum trust tier.
func (v *PQCVerifier) SetRequiredTier(tier uint8) {
	v.requiredTier = tier
}

// VerifyPassCount returns successful verification count.
func (v *PQCVerifier) VerifyPassCount() int64 {
	return atomic.LoadInt64(&v.verifyPass)
}

// VerifyFailCount returns failed verification count.
func (v *PQCVerifier) VerifyFailCount() int64 {
	return atomic.LoadInt64(&v.verifyFail)
}

// replayWindow implements a sliding window SeqNum tracker.
type replayWindow struct {
	mu       sync.Mutex
	maxSeq   uint16
	bitmap   [2048]byte // 16384-bit bitmap (2048 bytes)
	windowSz uint16
}

// checkReplay validates a SeqNum against the per-flow sliding window.
func (v *PQCVerifier) checkReplay(flowKey string, seqNum uint16) bool {
	raw, _ := v.replayWindows.LoadOrStore(flowKey, &replayWindow{windowSz: 16384})
	w := raw.(*replayWindow)

	w.mu.Lock()
	defer w.mu.Unlock()

	// If seqNum is ahead of the window, advance
	if seqNum > w.maxSeq {
		// Clear bits between old max and new max
		diff := seqNum - w.maxSeq
		if diff >= w.windowSz {
			// Clear entire bitmap
			for i := range w.bitmap {
				w.bitmap[i] = 0
			}
		} else {
			for i := w.maxSeq + 1; i <= seqNum; i++ {
				idx := i % w.windowSz
				w.bitmap[idx/8] &^= 1 << (idx % 8)
			}
		}
		w.maxSeq = seqNum
	} else if w.maxSeq-seqNum >= w.windowSz {
		// Too old, outside window
		return false
	}

	// Check if already seen
	idx := seqNum % w.windowSz
	if w.bitmap[idx/8]&(1<<(idx%8)) != 0 {
		return false // replay!
	}

	// Mark as seen
	w.bitmap[idx/8] |= 1 << (idx % 8)
	return true
}
