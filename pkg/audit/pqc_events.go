// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package audit

import "time"

// PQCEventType classifies PQC audit events for the Anamnesis system.
type PQCEventType string

const (
	PQCVerifySuccess  PQCEventType = "PQC_VERIFY_SUCCESS"
	PQCVerifyFail     PQCEventType = "PQC_VERIFY_FAIL"
	PQCKeyRotate      PQCEventType = "PQC_KEY_ROTATE"
	PQCKeyRevoke      PQCEventType = "PQC_KEY_REVOKE"
	PQCTierChange     PQCEventType = "PQC_TIER_CHANGE"
	PQCVerifierHealth PQCEventType = "PQC_VERIFIER_HEALTH"
	PQCMapUtilization PQCEventType = "PQC_MAP_UTILIZATION"
	PQCEntropyLow     PQCEventType = "PQC_ENTROPY_LOW"
)

// PQCEvent is an Anamnesis audit event for PQC operations.
type PQCEvent struct {
	ID        string                 `json:"id"`
	Type      PQCEventType           `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	ServiceID uint16                 `json:"service_id,omitempty"`
	AlgoID    uint8                  `json:"algo_id,omitempty"`
	KeyRef    uint16                 `json:"key_ref,omitempty"`
	SigRef    uint32                 `json:"sig_ref,omitempty"`
	Tier      uint8                  `json:"tier,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
}

// WotanTopics defines the 10 Wotan topics for PQC events.
var WotanTopics = struct {
	SigCreated    string
	SigVerified   string
	SigExpired    string
	KeyGenerated  string
	KeyRotated    string
	KeyRevoked    string
	PolicyChanged string
	KEMTunnel     string
	Sovereign     string
	Anomaly       string
}{
	SigCreated:    "pqc.sig.created",
	SigVerified:   "pqc.sig.verified",
	SigExpired:    "pqc.sig.expired",
	KeyGenerated:  "pqc.key.generated",
	KeyRotated:    "pqc.key.rotated",
	KeyRevoked:    "pqc.key.revoked",
	PolicyChanged: "pqc.policy.changed",
	KEMTunnel:     "pqc.kem.tunnel",
	Sovereign:     "pqc.sovereign.validated",
	Anomaly:       "pqc.anomaly",
}

// NewPQCEvent creates a new PQC audit event with a timestamp.
func NewPQCEvent(eventType PQCEventType) *PQCEvent {
	return &PQCEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}
}

// WithService sets the service ID.
func (e *PQCEvent) WithService(id uint16) *PQCEvent {
	e.ServiceID = id
	return e
}

// WithAlgo sets the algorithm ID.
func (e *PQCEvent) WithAlgo(id uint8) *PQCEvent {
	e.AlgoID = id
	return e
}

// WithKeyRef sets the key reference.
func (e *PQCEvent) WithKeyRef(ref uint16) *PQCEvent {
	e.KeyRef = ref
	return e
}

// WithSigRef sets the signature reference.
func (e *PQCEvent) WithSigRef(ref uint32) *PQCEvent {
	e.SigRef = ref
	return e
}

// WithTier sets the compliance tier.
func (e *PQCEvent) WithTier(tier uint8) *PQCEvent {
	e.Tier = tier
	return e
}

// WithDetail adds a detail key-value pair.
func (e *PQCEvent) WithDetail(key string, value interface{}) *PQCEvent {
	e.Details[key] = value
	return e
}

// WithTrace sets the trace ID.
func (e *PQCEvent) WithTrace(traceID string) *PQCEvent {
	e.TraceID = traceID
	return e
}
