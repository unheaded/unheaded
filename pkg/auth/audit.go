package auth

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Audit event types.
const (
	AuditJWTSuccess     = "jwt_validation_success"
	AuditJWTFailure     = "jwt_validation_failure"
	AuditAPIKeySuccess  = "apikey_validation_success"
	AuditAPIKeyFailure  = "apikey_validation_failure"
	AuditServiceToken   = "service_token_validation"
	AuditRBACAllowed    = "rbac_allowed"
	AuditRBACDenied     = "rbac_denied"
	AuditRateLimited    = "rate_limited"
	AuditUnauthorized   = "unauthorized_access"
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
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
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

// NopAuditLogger returns an audit function that discards events.
func NopAuditLogger() func(AuditEvent) {
	return func(AuditEvent) {}
}
