// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// Audit event types.
const (
	AuditJWTSuccess    = "jwt_validation_success"
	AuditJWTFailure    = "jwt_validation_failure"
	AuditAPIKeySuccess = "apikey_validation_success" // #nosec G101 -- audit event name, not a credential
	AuditAPIKeyFailure = "apikey_validation_failure" // #nosec G101 -- audit event name, not a credential
	AuditServiceToken  = "service_token_validation"
	AuditRBACAllowed   = "rbac_allowed"
	AuditRBACDenied    = "rbac_denied"
	AuditRateLimited   = "rate_limited"
	AuditUnauthorized  = "unauthorized_access"
)

// AuditEvent represents a single auth audit log entry.
type AuditEvent struct {
	Timestamp string `json:"timestamp"`
	EventType string `json:"event_type"`
	Result    string `json:"result"`
	Subject   string `json:"subject,omitempty"`
	Service   string `json:"service,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Method    string `json:"method,omitempty"`
	IP        string `json:"ip,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// AuditLogger writes structured audit events to an io.Writer.
type AuditLogger struct {
	mu      sync.Mutex
	w       io.Writer
	service string
}

// NewAuditLogger creates a new audit logger writing to the given writer.
func NewAuditLogger(w io.Writer, serviceName string) *AuditLogger {
	return &AuditLogger{w: w, service: serviceName}
}

// NewFileAuditLogger creates an audit logger that appends to a file.
// Creates the file if it doesn't exist.
func NewFileAuditLogger(path, serviceName string) (*AuditLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- auth store path from configuration, not from a request
	if err != nil {
		return nil, err
	}
	return NewAuditLogger(f, serviceName), nil
}

// Log writes an audit event with automatic timestamp and service name.
func (a *AuditLogger) Log(event AuditEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if event.Service == "" {
		event.Service = a.service
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')

	a.mu.Lock()
	_, _ = a.w.Write(data)
	a.mu.Unlock()
}

// LogFunc returns a function suitable for RBACConfig.AuditFunc.
func (a *AuditLogger) LogFunc() func(AuditEvent) {
	return a.Log
}

// Close closes the underlying writer if it implements io.Closer.
func (a *AuditLogger) Close() error {
	if c, ok := a.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// auditStderr is the default sink: a service's stderr is its journal.
var auditStderr io.Writer = os.Stderr

// newAuditFunc is where a service's auth decisions go. AUTH_AUDIT_LOG keeps
// every decision in that file. Without it, refusals go to stderr; allowed
// requests do not, as one line per Wotan publish would drown the journal. An
// audit log that cannot be opened is an error: the service does not run with
// no trail.
func newAuditFunc(cfg ServiceAuthConfig) (func(AuditEvent), error) {
	if cfg.AuditLog != "" {
		l, err := NewFileAuditLogger(cfg.AuditLog, cfg.ServiceName)
		if err != nil {
			return nil, fmt.Errorf("auth: AUTH_AUDIT_LOG unusable: %w", err)
		}
		return l.LogFunc(), nil
	}
	l := NewAuditLogger(auditStderr, cfg.ServiceName)
	return func(e AuditEvent) {
		if e.EventType != AuditRBACAllowed {
			l.Log(e)
		}
	}, nil
}

// auditedAuthenticator records every refusal of the authenticator it wraps.
type auditedAuthenticator struct {
	Authenticator
	audit func(AuditEvent)
}

// Authenticate implements Authenticator. The event carries a fixed reason,
// not err's text, and never a credential.
func (a auditedAuthenticator) Authenticate(ctx context.Context, r *http.Request) (*Identity, error) {
	id, err := a.Authenticator.Authenticate(ctx, r)
	if err == nil && id != nil {
		return id, nil
	}
	e := AuditEvent{EventType: AuditUnauthorized, Result: "denied", Method: r.Method, IP: peerIP(r), Reason: "unauthenticated"}
	if r.URL != nil {
		e.Endpoint = r.URL.Path
	}
	switch {
	case errors.Is(err, ErrRateLimited):
		e.EventType, e.Reason = AuditRateLimited, "rate limited"
	case errors.Is(err, ErrForbidden):
		e.Reason = "forbidden"
	}
	a.audit(e)
	return id, err
}

// peerIP is the address the request came from, without its port.
func peerIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// NopAuditLogger returns an audit function that discards events.
func NopAuditLogger() func(AuditEvent) {
	return func(AuditEvent) {}
}
