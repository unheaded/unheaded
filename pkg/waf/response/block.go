// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package response provides block response functionality
package response

import (
	"encoding/json"
	"net/http"
	"time"
)

// BlockReason represents why a request was blocked
type BlockReason struct {
	Type      string    `json:"type"`
	RuleID    string    `json:"rule_id,omitempty"`
	Score     int       `json:"score,omitempty"`
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// BlockWriter writes block responses
type BlockWriter struct {
	handler *Handler
}

// NewBlockWriter creates a new block writer
func NewBlockWriter(handler *Handler) *BlockWriter {
	return &BlockWriter{handler: handler}
}

// WriteBlock writes a standard block response
func (bw *BlockWriter) WriteBlock(w http.ResponseWriter, reason *BlockReason, requestID string) {
	bw.handler.Block(w, reason.Message, reason.RuleID, requestID)
}

// WriteJSONBlock writes a JSON block response
func (bw *BlockWriter) WriteJSONBlock(w http.ResponseWriter, reason *BlockReason, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)

	response := struct {
		Blocked   bool         `json:"blocked"`
		RequestID string       `json:"request_id"`
		Reason    *BlockReason `json:"reason"`
	}{
		Blocked:   true,
		RequestID: requestID,
		Reason:    reason,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// WriteIPBlock writes an IP-based block response
func (bw *BlockWriter) WriteIPBlock(w http.ResponseWriter, ip string, requestID string) {
	reason := &BlockReason{
		Type:      "ip_block",
		Message:   "Your IP address has been blocked",
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}

// WriteRateLimit writes a rate limit block response
func (bw *BlockWriter) WriteRateLimit(w http.ResponseWriter, retryAfter int, requestID string) {
	w.Header().Set("Retry-After", string(rune(retryAfter)))
	w.Header().Set("X-RateLimit-Reset", time.Now().Add(time.Duration(retryAfter)*time.Second).Format(time.RFC3339))

	reason := &BlockReason{
		Type:      "rate_limit",
		Message:   "Rate limit exceeded",
		Details:   "Too many requests. Please slow down.",
		Timestamp: time.Now(),
	}

	w.WriteHeader(http.StatusTooManyRequests)

	response := struct {
		Blocked    bool         `json:"blocked"`
		RequestID  string       `json:"request_id"`
		RetryAfter int          `json:"retry_after"`
		Reason     *BlockReason `json:"reason"`
	}{
		Blocked:    true,
		RequestID:  requestID,
		RetryAfter: retryAfter,
		Reason:     reason,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// WriteGeoBlock writes a geo-based block response
func (bw *BlockWriter) WriteGeoBlock(w http.ResponseWriter, country string, requestID string) {
	reason := &BlockReason{
		Type:      "geo_block",
		Message:   "Access from your region is not permitted",
		Details:   "Country: " + country,
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}

// WriteSQLiBlock writes an SQL injection block response
func (bw *BlockWriter) WriteSQLiBlock(w http.ResponseWriter, ruleID string, requestID string) {
	reason := &BlockReason{
		Type:      "sqli",
		RuleID:    ruleID,
		Message:   "SQL injection attempt detected",
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}

// WriteXSSBlock writes an XSS block response
func (bw *BlockWriter) WriteXSSBlock(w http.ResponseWriter, ruleID string, requestID string) {
	reason := &BlockReason{
		Type:      "xss",
		RuleID:    ruleID,
		Message:   "Cross-site scripting attempt detected",
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}

// WriteTraversalBlock writes a path traversal block response
func (bw *BlockWriter) WriteTraversalBlock(w http.ResponseWriter, ruleID string, requestID string) {
	reason := &BlockReason{
		Type:      "traversal",
		RuleID:    ruleID,
		Message:   "Path traversal attempt detected",
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}

// WriteRCEBlock writes an RCE block response
func (bw *BlockWriter) WriteRCEBlock(w http.ResponseWriter, ruleID string, requestID string) {
	reason := &BlockReason{
		Type:      "rce",
		RuleID:    ruleID,
		Message:   "Remote code execution attempt detected",
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}

// WriteSSRFBlock writes an SSRF block response
func (bw *BlockWriter) WriteSSRFBlock(w http.ResponseWriter, ruleID string, requestID string) {
	reason := &BlockReason{
		Type:      "ssrf",
		RuleID:    ruleID,
		Message:   "Server-side request forgery attempt detected",
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}

// WriteCustomBlock writes a custom block response
func (bw *BlockWriter) WriteCustomBlock(w http.ResponseWriter, blockType, message, ruleID, requestID string) {
	reason := &BlockReason{
		Type:      blockType,
		RuleID:    ruleID,
		Message:   message,
		Timestamp: time.Now(),
	}
	bw.WriteBlock(w, reason, requestID)
}
