// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/policy"
	"unheaded/services/shield"
)

// VerifyRequest is the JSON body for POST /verify.
type VerifyRequest struct {
	Flags        byte     `json:"flags"`
	SigRef       uint32   `json:"sig_ref"`
	KeyRef       uint16   `json:"key_ref"`
	HashPfx      [4]byte  `json:"hash_pfx"`
	SeqNum       uint16   `json:"seq_num"`
	SrcSvcID     uint16   `json:"src_svc_id"`
	DstSvcID     uint16   `json:"dst_svc_id"`
	PseudoHeader []byte   `json:"pseudo_header"`
}

// VerifyResponse is the JSON response for verification endpoints.
type VerifyResponse struct {
	Passed   bool   `json:"passed"`
	FailStep string `json:"fail_step,omitempty"`
	FailMsg  string `json:"fail_msg,omitempty"`
	AlgoID   uint8  `json:"algo_id,omitempty"`
	Tier     string `json:"tier"`
	Duration string `json:"duration"`
}

// SovereignVerifyRequest is the JSON body for POST /verify/sovereign.
type SovereignVerifyRequest struct {
	VerifyRequest
	Slots [3]struct {
		SigRef uint32 `json:"sig_ref"`
		AlgoID uint8  `json:"algo_id"`
	} `json:"slots"`
}

// SovereignVerifyResponse is the JSON response for sovereign verification.
type SovereignVerifyResponse struct {
	Passed           bool   `json:"passed"`
	ValidCount       int    `json:"valid_count"`
	DistinctFamilies int    `json:"distinct_families"`
	FailReason       string `json:"fail_reason,omitempty"`
}

// Handlers holds the HTTP handler state.
type Handlers struct {
	tierVerifier *shield.TierVerifier
	publisher    *EventPublisher
	startTime    time.Time
	ready        atomic.Bool
	reqCount     atomic.Int64
}

// NewHandlers creates a new handler set.
func NewHandlers(tv *shield.TierVerifier, pub *EventPublisher) *Handlers {
	h := &Handlers{
		tierVerifier: tv,
		publisher:    pub,
		startTime:    time.Now(),
	}
	h.ready.Store(true)
	return h
}

// RegisterRoutes sets up the HTTP routes on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /verify", h.handleVerify)
	mux.HandleFunc("POST /verify/sovereign", h.handleVerifySovereign)
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("GET /ready", h.handleReady)
	mux.HandleFunc("GET /metrics", h.handleMetrics)
}

func (h *Handlers) handleVerify(w http.ResponseWriter, r *http.Request) {
	h.reqCount.Add(1)

	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusBadRequest)
		return
	}

	pkt := &shield.PQCPacketInfo{
		Flags:        req.Flags,
		SigRef:       req.SigRef,
		KeyRef:       req.KeyRef,
		HashPfx:      req.HashPfx,
		SeqNum:       req.SeqNum,
		SrcSvcID:     req.SrcSvcID,
		DstSvcID:     req.DstSvcID,
		PseudoHeader: req.PseudoHeader,
	}

	start := time.Now()
	result := h.tierVerifier.VerifyPacket(pkt, nil)
	dur := time.Since(start)

	resp := VerifyResponse{
		Passed:   result.Passed(),
		Tier:     result.Tier.String(),
		Duration: dur.String(),
	}

	if result.PQCResult != nil {
		resp.AlgoID = result.PQCResult.AlgoID
		if !result.PQCResult.Passed {
			resp.FailStep = result.PQCResult.FailStep.String()
			resp.FailMsg = result.PQCResult.FailMsg
		}
	}

	// Publish verification event.
	if h.publisher != nil {
		evtType := audit.PQCVerifySuccess
		if !result.Passed() {
			evtType = audit.PQCVerifyFail
		}
		evt := audit.NewPQCEvent(evtType).
			WithSigRef(req.SigRef).
			WithKeyRef(req.KeyRef).
			WithTier(uint8(result.Tier))
		h.publisher.PublishVerified(evt)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) handleVerifySovereign(w http.ResponseWriter, r *http.Request) {
	h.reqCount.Add(1)

	var req SovereignVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusBadRequest)
		return
	}

	pkt := &shield.PQCPacketInfo{
		Flags:        req.Flags,
		SigRef:       req.SigRef,
		KeyRef:       req.KeyRef,
		HashPfx:      req.HashPfx,
		SeqNum:       req.SeqNum,
		SrcSvcID:     req.SrcSvcID,
		DstSvcID:     req.DstSvcID,
		PseudoHeader: req.PseudoHeader,
	}

	sov := &shield.SovereignSig{}
	for i, slot := range req.Slots {
		sov.Sigs[i] = shield.SovereignSigSlot{
			SigRef: slot.SigRef,
			AlgoID: slot.AlgoID,
		}
	}

	result := h.tierVerifier.VerifyPacket(pkt, sov)

	resp := SovereignVerifyResponse{}
	if result.SovereignResult != nil {
		resp.Passed = result.SovereignResult.Passed
		resp.ValidCount = result.SovereignResult.ValidCount
		resp.DistinctFamilies = result.SovereignResult.DistinctFamilies
		resp.FailReason = result.SovereignResult.FailReason
	}

	if h.publisher != nil {
		evt := audit.NewPQCEvent(audit.PQCVerifySuccess).
			WithTier(uint8(policy.TierSovereign)).
			WithDetail("valid_count", resp.ValidCount).
			WithDetail("distinct_families", resp.DistinctFamilies)
		h.publisher.PublishSovereign(evt)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "pqc-verifier",
		"uptime":  time.Since(h.startTime).String(),
	})
}

func (h *Handlers) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !h.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (h *Handlers) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	stats := h.tierVerifier.Stats()
	stats["requests_total"] = h.reqCount.Load()
	if h.publisher != nil {
		stats["events_published"] = h.publisher.EventCount()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// SetReady sets the readiness state.
func (h *Handlers) SetReady(ready bool) {
	h.ready.Store(ready)
}
