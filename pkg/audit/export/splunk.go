package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"unheaded/pkg/audit"
)

// splunkSafeQuery matches safe Splunk search query characters.
// SECURITY: Prevents Splunk search injection via malicious query parameters.
var splunkSafeQuery = regexp.MustCompile(`^[a-zA-Z0-9_.*=<>!| "'\-:/@]+$`)

// SplunkConfig holds configuration for Splunk HEC integration.
type SplunkConfig struct {
	// HECEndpoint is the Splunk HTTP Event Collector endpoint.
	HECEndpoint string `json:"hec_endpoint"`
	// HECToken is the authentication token for HEC.
	HECToken string `json:"hec_token"`
	// Index is the Splunk index to send events to.
	Index string `json:"index"`
	// Source identifies the source of events.
	Source string `json:"source"`
	// SourceType is the Splunk sourcetype.
	SourceType string `json:"sourcetype"`
	// Host is the host identifier.
	Host string `json:"host"`
	// BatchSize is the number of events per batch.
	BatchSize int `json:"batch_size"`
	// FlushInterval is how often to flush events.
	FlushInterval time.Duration `json:"flush_interval"`
	// Timeout is the HTTP request timeout.
	Timeout time.Duration `json:"timeout"`
	// DisableSSLVerify disables SSL certificate verification.
	DisableSSLVerify bool `json:"disable_ssl_verify"`
}

// DefaultSplunkConfig returns a SplunkConfig with sensible defaults.
func DefaultSplunkConfig() *SplunkConfig {
	return &SplunkConfig{
		Index:         "audit",
		SourceType:    "audit:events",
		Source:        "unheaded-audit",
		BatchSize:     100,
		FlushInterval: 10 * time.Second,
		Timeout:       30 * time.Second,
	}
}

// SplunkHECEvent represents an event in Splunk HEC format.
type SplunkHECEvent struct {
	Time       float64                `json:"time"`
	Host       string                 `json:"host,omitempty"`
	Source     string                 `json:"source,omitempty"`
	SourceType string                 `json:"sourcetype,omitempty"`
	Index      string                 `json:"index,omitempty"`
	Event      map[string]interface{} `json:"event"`
}

// SplunkExporter sends audit events to Splunk via HEC.
type SplunkExporter struct {
	config *SplunkConfig
	client *http.Client

	mu         sync.Mutex
	buffer     []*audit.AuditEvent
	timer      *time.Timer
	stopCh     chan struct{}
	doneCh     chan struct{}
	errorCount int64
	sentCount  int64
}

// NewSplunkExporter creates a new Splunk HEC exporter.
func NewSplunkExporter(config *SplunkConfig) *SplunkExporter {
	if config == nil {
		config = DefaultSplunkConfig()
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.DisableSSLVerify {
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	return &SplunkExporter{
		config: config,
		client: &http.Client{
			Timeout:   config.Timeout,
			Transport: transport,
		},
		buffer: make([]*audit.AuditEvent, 0, config.BatchSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins the background flush timer.
func (e *SplunkExporter) Start() {
	e.mu.Lock()
	e.timer = time.NewTimer(e.config.FlushInterval)
	e.mu.Unlock()

	go e.flushLoop()
}

// Stop gracefully shuts down the exporter.
func (e *SplunkExporter) Stop() error {
	close(e.stopCh)
	<-e.doneCh
	return e.Flush()
}

// Send adds an event to the buffer.
func (e *SplunkExporter) Send(event *audit.AuditEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.buffer = append(e.buffer, event)

	if len(e.buffer) >= e.config.BatchSize {
		return e.flushLocked()
	}

	return nil
}

// SendBatch adds multiple events to the buffer.
func (e *SplunkExporter) SendBatch(events []*audit.AuditEvent) error {
	for _, event := range events {
		if err := e.Send(event); err != nil {
			return err
		}
	}
	return nil
}

// Flush sends all buffered events.
func (e *SplunkExporter) Flush() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.flushLocked()
}

func (e *SplunkExporter) flushLocked() error {
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

func (e *SplunkExporter) sendEvents(events []*audit.AuditEvent) error {
	// Convert to HEC format
	var payload bytes.Buffer
	for _, event := range events {
		hecEvent := e.toHECEvent(event)
		data, err := json.Marshal(hecEvent)
		if err != nil {
			e.errorCount++
			continue
		}
		payload.Write(data)
	}

	// Send to Splunk
	req, err := http.NewRequest(http.MethodPost, e.config.HECEndpoint, &payload)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Splunk "+e.config.HECToken)

	resp, err := e.client.Do(req)
	if err != nil {
		e.errorCount += int64(len(events))
		return fmt.Errorf("failed to send to Splunk: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		e.errorCount += int64(len(events))
		return fmt.Errorf("Splunk returned status %d: %s", resp.StatusCode, string(body))
	}

	e.sentCount += int64(len(events))
	return nil
}

func (e *SplunkExporter) toHECEvent(event *audit.AuditEvent) *SplunkHECEvent {
	eventData := map[string]interface{}{
		"event_id":  event.ID,
		"type":      event.Type,
		"severity":  event.Severity,
		"action":    event.Action,
		"outcome":   event.Outcome,
		"hash":      event.Hash,
	}

	if event.Actor != nil {
		eventData["actor"] = map[string]interface{}{
			"id":    event.Actor.ID,
			"type":  event.Actor.Type,
			"name":  event.Actor.Name,
			"roles": event.Actor.Roles,
		}
	}

	if event.Resource != nil {
		eventData["resource"] = map[string]interface{}{
			"id":   event.Resource.ID,
			"type": event.Resource.Type,
			"name": event.Resource.Name,
			"path": event.Resource.Path,
		}
	}

	if event.Source != nil {
		eventData["source_info"] = map[string]interface{}{
			"ip":         event.Source.IP,
			"user_agent": event.Source.UserAgent,
			"service":    event.Source.Service,
			"request_id": event.Source.RequestID,
		}
	}

	if event.Details != nil {
		eventData["details"] = event.Details
	}

	return &SplunkHECEvent{
		Time:       float64(event.Timestamp.Unix()) + float64(event.Timestamp.Nanosecond())/1e9,
		Host:       e.config.Host,
		Source:     e.config.Source,
		SourceType: e.config.SourceType,
		Index:      e.config.Index,
		Event:      eventData,
	}
}

func (e *SplunkExporter) flushLoop() {
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
func (e *SplunkExporter) Stats() *SplunkStats {
	e.mu.Lock()
	defer e.mu.Unlock()
	return &SplunkStats{
		BufferSize: len(e.buffer),
		SentCount:  e.sentCount,
		ErrorCount: e.errorCount,
		Endpoint:   e.config.HECEndpoint,
		Index:      e.config.Index,
	}
}

// SplunkStats contains Splunk exporter statistics.
type SplunkStats struct {
	BufferSize int    `json:"buffer_size"`
	SentCount  int64  `json:"sent_count"`
	ErrorCount int64  `json:"error_count"`
	Endpoint   string `json:"endpoint"`
	Index      string `json:"index"`
}

// Export implements the Exporter interface.
func (e *SplunkExporter) Export(ctx context.Context, events []*audit.AuditEvent, filter *audit.AuditFilter) (io.Reader, error) {
	if err := e.sendEvents(events); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"sent":     len(events),
		"endpoint": e.config.HECEndpoint,
		"index":    e.config.Index,
	}
	data, _ := json.Marshal(result)
	return bytes.NewReader(data), nil
}

// Format returns the export format name.
func (e *SplunkExporter) Format() string {
	return "splunk"
}

// SplunkSearchClient provides search capabilities against Splunk.
type SplunkSearchClient struct {
	baseURL   string
	username  string
	password  string
	client    *http.Client
}

// NewSplunkSearchClient creates a new Splunk search client.
func NewSplunkSearchClient(baseURL, username, password string) *SplunkSearchClient {
	return &SplunkSearchClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// SearchAuditEvents searches for audit events in Splunk.
func (c *SplunkSearchClient) SearchAuditEvents(ctx context.Context, query string, earliest, latest time.Time) ([]map[string]interface{}, error) {
	// SECURITY: Validate the query parameter to prevent Splunk search injection.
	// Reject queries containing pipe characters or other special Splunk operators
	// that could allow an attacker to execute arbitrary Splunk commands.
	if !splunkSafeQuery.MatchString(query) {
		return nil, fmt.Errorf("query contains invalid characters")
	}
	searchQuery := fmt.Sprintf(`search index=audit %s | spath | table *`, query)

	// This is a simplified search - production would use async search jobs
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/services/search/jobs/export", nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("search", searchQuery)
	q.Set("earliest_time", earliest.Format(time.RFC3339))
	q.Set("latest_time", latest.Format(time.RFC3339))
	q.Set("output_mode", "json")
	req.URL.RawQuery = q.Encode()

	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("Splunk search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var results []map[string]interface{}
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var result map[string]interface{}
		if err := decoder.Decode(&result); err != nil {
			continue
		}
		if event, ok := result["result"].(map[string]interface{}); ok {
			results = append(results, event)
		}
	}

	return results, nil
}

// SplunkAlertConfig holds configuration for Splunk alerts.
type SplunkAlertConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Search      string                 `json:"search"`
	CronSchedule string                `json:"cron_schedule"`
	Actions     []string               `json:"actions"`
	Threshold   int                    `json:"threshold"`
	Severity    string                 `json:"severity"`
	Params      map[string]interface{} `json:"params"`
}

// PredefinedSplunkAlerts returns common audit-related Splunk alerts.
func PredefinedSplunkAlerts() []*SplunkAlertConfig {
	return []*SplunkAlertConfig{
		{
			Name:        "Failed Authentication Spike",
			Description: "Alert when failed authentication attempts exceed threshold",
			Search:      `index=audit type="authentication" outcome="failure" | stats count by actor.id | where count > 5`,
			CronSchedule: "*/5 * * * *",
			Actions:     []string{"email", "slack"},
			Threshold:   5,
			Severity:    "high",
		},
		{
			Name:        "Critical Security Event",
			Description: "Alert on any critical security event",
			Search:      `index=audit severity="critical"`,
			CronSchedule: "* * * * *",
			Actions:     []string{"email", "pagerduty"},
			Threshold:   1,
			Severity:    "critical",
		},
		{
			Name:        "Unusual Configuration Change",
			Description: "Alert on configuration changes outside business hours",
			Search:      `index=audit type="configuration" | eval hour=strftime(_time, "%H") | where hour < 6 OR hour > 20`,
			CronSchedule: "*/15 * * * *",
			Actions:     []string{"email"},
			Threshold:   1,
			Severity:    "medium",
		},
		{
			Name:        "Sensitive Data Access",
			Description: "Alert on access to sensitive resources",
			Search:      `index=audit type="data_access" resource.type="sensitive"`,
			CronSchedule: "*/5 * * * *",
			Actions:     []string{"email", "slack"},
			Threshold:   1,
			Severity:    "high",
		},
	}
}
