package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/unheaded/unheaded/pkg/audit"
)

// SIEMConfig holds configuration for SIEM integration.
type SIEMConfig struct {
	// Endpoint is the SIEM ingestion endpoint URL.
	Endpoint string `json:"endpoint"`
	// AuthToken is the authentication token.
	AuthToken string `json:"auth_token"`
	// BatchSize is the number of events per batch.
	BatchSize int `json:"batch_size"`
	// FlushInterval is how often to flush events.
	FlushInterval time.Duration `json:"flush_interval"`
	// RetryAttempts is the number of retry attempts.
	RetryAttempts int `json:"retry_attempts"`
	// RetryDelay is the delay between retries.
	RetryDelay time.Duration `json:"retry_delay"`
	// Timeout is the HTTP request timeout.
	Timeout time.Duration `json:"timeout"`
	// Headers are additional HTTP headers.
	Headers map[string]string `json:"headers"`
}

// DefaultSIEMConfig returns a SIEMConfig with sensible defaults.
func DefaultSIEMConfig() *SIEMConfig {
	return &SIEMConfig{
		BatchSize:     100,
		FlushInterval: 10 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    5 * time.Second,
		Timeout:       30 * time.Second,
		Headers:       make(map[string]string),
	}
}

// SIEMExporter sends audit events to a SIEM system.
type SIEMExporter struct {
	config    *SIEMConfig
	client    *http.Client
	formatter EventFormatter

	mu         sync.Mutex
	buffer     []*audit.AuditEvent
	timer      *time.Timer
	stopCh     chan struct{}
	doneCh     chan struct{}
	errorCount int64
	sentCount  int64
}

// EventFormatter formats events for SIEM ingestion.
type EventFormatter interface {
	Format(event *audit.AuditEvent) ([]byte, error)
	ContentType() string
}

// JSONFormatter formats events as JSON.
type JSONFormatter struct{}

// Format formats an event as JSON.
func (f *JSONFormatter) Format(event *audit.AuditEvent) ([]byte, error) {
	return json.Marshal(event)
}

// ContentType returns the content type.
func (f *JSONFormatter) ContentType() string {
	return "application/json"
}

// NewSIEMExporter creates a new SIEM exporter.
func NewSIEMExporter(config *SIEMConfig, formatter EventFormatter) *SIEMExporter {
	if config == nil {
		config = DefaultSIEMConfig()
	}
	if formatter == nil {
		formatter = &JSONFormatter{}
	}

	return &SIEMExporter{
		config:    config,
		formatter: formatter,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		buffer: make([]*audit.AuditEvent, 0, config.BatchSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins the background flush timer.
func (e *SIEMExporter) Start() {
	e.mu.Lock()
	e.timer = time.NewTimer(e.config.FlushInterval)
	e.mu.Unlock()

	go e.flushLoop()
}

// Stop gracefully shuts down the exporter.
func (e *SIEMExporter) Stop() error {
	close(e.stopCh)
	<-e.doneCh
	return e.Flush()
}

// Send adds an event to the buffer.
func (e *SIEMExporter) Send(event *audit.AuditEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.buffer = append(e.buffer, event)

	if len(e.buffer) >= e.config.BatchSize {
		return e.flushLocked()
	}

	return nil
}

// SendBatch adds multiple events to the buffer.
func (e *SIEMExporter) SendBatch(events []*audit.AuditEvent) error {
	for _, event := range events {
		if err := e.Send(event); err != nil {
			return err
		}
	}
	return nil
}

// Flush sends all buffered events.
func (e *SIEMExporter) Flush() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.flushLocked()
}

func (e *SIEMExporter) flushLocked() error {
	if len(e.buffer) == 0 {
		return nil
	}

	events := e.buffer
	e.buffer = make([]*audit.AuditEvent, 0, e.config.BatchSize)

	// Reset timer
	if e.timer != nil {
		e.timer.Reset(e.config.FlushInterval)
	}

	return e.sendEvents(events)
}

func (e *SIEMExporter) sendEvents(events []*audit.AuditEvent) error {
	// Format events
	var payload bytes.Buffer
	payload.WriteByte('[')
	for i, event := range events {
		data, err := e.formatter.Format(event)
		if err != nil {
			e.errorCount++
			continue
		}
		if i > 0 {
			payload.WriteByte(',')
		}
		payload.Write(data)
	}
	payload.WriteByte(']')

	// Send with retry
	var lastErr error
	for attempt := 0; attempt <= e.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(e.config.RetryDelay)
		}

		if err := e.doSend(&payload); err != nil {
			lastErr = err
			continue
		}

		e.sentCount += int64(len(events))
		return nil
	}

	e.errorCount += int64(len(events))
	return fmt.Errorf("failed to send events after %d attempts: %w", e.config.RetryAttempts, lastErr)
}

func (e *SIEMExporter) doSend(payload *bytes.Buffer) error {
	req, err := http.NewRequest(http.MethodPost, e.config.Endpoint, payload)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", e.formatter.ContentType())
	if e.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.config.AuthToken)
	}
	for k, v := range e.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("SIEM returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (e *SIEMExporter) flushLoop() {
	defer close(e.doneCh)

	for {
		select {
		case <-e.stopCh:
			e.mu.Lock()
			if e.timer != nil {
				e.timer.Stop()
			}
			e.mu.Unlock()
			return
		case <-e.timer.C:
			e.mu.Lock()
			_ = e.flushLocked()
			e.mu.Unlock()
		}
	}
}

// Stats returns exporter statistics.
func (e *SIEMExporter) Stats() *SIEMStats {
	e.mu.Lock()
	defer e.mu.Unlock()
	return &SIEMStats{
		BufferSize: len(e.buffer),
		SentCount:  e.sentCount,
		ErrorCount: e.errorCount,
	}
}

// SIEMStats contains SIEM exporter statistics.
type SIEMStats struct {
	BufferSize int   `json:"buffer_size"`
	SentCount  int64 `json:"sent_count"`
	ErrorCount int64 `json:"error_count"`
}

// Export implements the Exporter interface.
func (e *SIEMExporter) Export(ctx context.Context, events []*audit.AuditEvent, filter *audit.AuditFilter) (io.Reader, error) {
	// For the Exporter interface, we send immediately
	if err := e.sendEvents(events); err != nil {
		return nil, err
	}

	// Return confirmation
	result := map[string]interface{}{
		"sent":     len(events),
		"endpoint": e.config.Endpoint,
	}
	data, _ := json.Marshal(result)
	return bytes.NewReader(data), nil
}

// Format returns the export format name.
func (e *SIEMExporter) Format() string {
	return "siem"
}

// SIEMForwarder provides real-time event forwarding to SIEM.
type SIEMForwarder struct {
	exporters []*SIEMExporter
	mu        sync.RWMutex
}

// NewSIEMForwarder creates a new SIEM forwarder.
func NewSIEMForwarder() *SIEMForwarder {
	return &SIEMForwarder{
		exporters: make([]*SIEMExporter, 0),
	}
}

// AddExporter adds a SIEM exporter.
func (f *SIEMForwarder) AddExporter(exporter *SIEMExporter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exporters = append(f.exporters, exporter)
}

// Forward sends an event to all configured SIEM exporters.
func (f *SIEMForwarder) Forward(event *audit.AuditEvent) error {
	f.mu.RLock()
	exporters := f.exporters
	f.mu.RUnlock()

	var lastErr error
	for _, exporter := range exporters {
		if err := exporter.Send(event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Start starts all exporters.
func (f *SIEMForwarder) Start() {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, exporter := range f.exporters {
		exporter.Start()
	}
}

// Stop stops all exporters.
func (f *SIEMForwarder) Stop() error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var lastErr error
	for _, exporter := range f.exporters {
		if err := exporter.Stop(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// LEEFFormatter formats events in Log Event Extended Format (LEEF).
type LEEFFormatter struct {
	vendor  string
	product string
	version string
}

// NewLEEFFormatter creates a new LEEF formatter.
func NewLEEFFormatter(vendor, product, version string) *LEEFFormatter {
	return &LEEFFormatter{
		vendor:  vendor,
		product: product,
		version: version,
	}
}

// Format formats an event as LEEF.
func (f *LEEFFormatter) Format(event *audit.AuditEvent) ([]byte, error) {
	// LEEF:Version|Vendor|Product|Version|EventID|Attributes
	var attrs string

	attrs += fmt.Sprintf("devTime=%s\t", event.Timestamp.Format(time.RFC3339))
	attrs += fmt.Sprintf("cat=%s\t", event.Type)
	attrs += fmt.Sprintf("sev=%s\t", event.Severity)

	if event.Actor != nil {
		attrs += fmt.Sprintf("usrName=%s\t", event.Actor.ID)
	}

	if event.Source != nil && event.Source.IP != "" {
		attrs += fmt.Sprintf("src=%s\t", event.Source.IP)
	}

	if event.Resource != nil {
		attrs += fmt.Sprintf("resource=%s\t", event.Resource.ID)
	}

	attrs += fmt.Sprintf("outcome=%s", event.Outcome)

	leef := fmt.Sprintf("LEEF:2.0|%s|%s|%s|%s|%s",
		f.vendor,
		f.product,
		f.version,
		event.Action,
		attrs,
	)

	return []byte(leef), nil
}

// ContentType returns the content type.
func (f *LEEFFormatter) ContentType() string {
	return "text/plain"
}
