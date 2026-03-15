// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/audit"
)

// --- helpers ---

func sampleEvent(id string) *audit.AuditEvent {
	return &audit.AuditEvent{
		ID:        id,
		Timestamp: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
		Type:      audit.EventTypeAuthentication,
		Severity:  audit.SeverityInfo,
		Action:    "user.login",
		Outcome:   audit.OutcomeSuccess,
		Actor:     &audit.Actor{ID: "user-1", Type: "user", Name: "Alice", Roles: []string{"admin"}},
		Resource:  &audit.Resource{ID: "res-1", Type: "session", Name: "login-session", Path: "/auth/login"},
		Source:    &audit.Source{IP: "10.0.0.1", Service: "auth-service", RequestID: "req-abc", UserAgent: "test"},
		Hash:      "hash123",
	}
}

func sampleEvents(n int) []*audit.AuditEvent {
	events := make([]*audit.AuditEvent, n)
	for i := range events {
		events[i] = sampleEvent(string(rune('a' + i)))
	}
	// Set unique IDs
	for i := range events {
		events[i].ID = "evt-" + string(rune('a'+i))
	}
	return events
}

// ===================== JSONExporter Tests =====================

func TestJSONExporter_Format(t *testing.T) {
	e := NewJSONExporter()
	if e.Format() != "json" {
		t.Errorf("expected 'json', got %q", e.Format())
	}
}

func TestJSONExporter_Export(t *testing.T) {
	e := NewJSONExporter()
	events := sampleEvents(3)

	reader, err := e.Export(context.Background(), events, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	var decoded []*audit.AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(decoded) != 3 {
		t.Errorf("expected 3 events, got %d", len(decoded))
	}
}

func TestJSONExporter_Export_Empty(t *testing.T) {
	e := NewJSONExporter()
	reader, err := e.Export(context.Background(), []*audit.AuditEvent{}, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	var decoded []*audit.AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("expected 0 events, got %d", len(decoded))
	}
}

// ===================== JSONLExporter Tests =====================

func TestJSONLExporter_Format(t *testing.T) {
	e := NewJSONLExporter()
	if e.Format() != "jsonl" {
		t.Errorf("expected 'jsonl', got %q", e.Format())
	}
}

func TestJSONLExporter_Export(t *testing.T) {
	e := NewJSONLExporter()
	events := sampleEvents(3)

	reader, err := e.Export(context.Background(), events, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	// Each line should be valid JSON
	for i, line := range lines {
		var ev audit.AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestJSONLExporter_Export_Empty(t *testing.T) {
	e := NewJSONLExporter()
	reader, err := e.Export(context.Background(), []*audit.AuditEvent{}, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	data, _ := io.ReadAll(reader)
	if len(data) != 0 {
		t.Errorf("expected empty output, got %d bytes", len(data))
	}
}

// ===================== CSVExporter Tests =====================

func TestCSVExporter_Format(t *testing.T) {
	e := NewCSVExporter()
	if e.Format() != "csv" {
		t.Errorf("expected 'csv', got %q", e.Format())
	}
}

func TestCSVExporter_Export(t *testing.T) {
	e := NewCSVExporter()
	events := sampleEvents(2)

	reader, err := e.Export(context.Background(), events, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	csvReader := csv.NewReader(bytes.NewReader(data))
	records, err := csvReader.ReadAll()
	if err != nil {
		t.Fatalf("CSV ReadAll failed: %v", err)
	}

	// Header + 2 data rows
	if len(records) != 3 {
		t.Errorf("expected 3 rows (1 header + 2 data), got %d", len(records))
	}

	// Verify headers
	expectedHeaders := []string{
		"id", "timestamp", "type", "severity", "action", "outcome",
		"actor_id", "actor_type", "actor_name",
		"resource_id", "resource_type", "resource_path",
		"source_ip", "source_service", "request_id",
		"hash",
	}
	for i, h := range expectedHeaders {
		if records[0][i] != h {
			t.Errorf("header[%d]: expected %q, got %q", i, h, records[0][i])
		}
	}
}

func TestCSVExporter_Export_NilFields(t *testing.T) {
	e := NewCSVExporter()
	events := []*audit.AuditEvent{
		{
			ID:        "bare",
			Timestamp: time.Now().UTC(),
			Type:      audit.EventTypeSystem,
			Severity:  audit.SeverityInfo,
			Action:    "test",
			Outcome:   audit.OutcomeSuccess,
			Hash:      "h1",
		},
	}

	reader, err := e.Export(context.Background(), events, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	csvReader := csv.NewReader(bytes.NewReader(data))
	records, _ := csvReader.ReadAll()
	if len(records) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(records))
	}
	// Actor, Resource, Source fields should be empty
	row := records[1]
	if row[6] != "" || row[9] != "" || row[12] != "" {
		t.Error("nil actor/resource/source fields should be empty strings")
	}
}

func TestCSVExporter_Export_Empty(t *testing.T) {
	e := NewCSVExporter()
	reader, err := e.Export(context.Background(), []*audit.AuditEvent{}, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	csvReader := csv.NewReader(bytes.NewReader(data))
	records, _ := csvReader.ReadAll()
	// Only header row
	if len(records) != 1 {
		t.Errorf("expected 1 header row, got %d rows", len(records))
	}
}

// ===================== CEFExporter Tests =====================

func TestCEFExporter_Format(t *testing.T) {
	e := NewCEFExporter("TestVendor", "TestProduct", "1.0")
	if e.Format() != "cef" {
		t.Errorf("expected 'cef', got %q", e.Format())
	}
}

func TestCEFExporter_Export(t *testing.T) {
	e := NewCEFExporter("Unheaded", "AuditSystem", "1.0")
	events := sampleEvents(2)

	reader, err := e.Export(context.Background(), events, nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}

	for _, line := range lines {
		if !strings.HasPrefix(line, "CEF:0|Unheaded|AuditSystem|1.0|") {
			t.Errorf("unexpected CEF format: %s", line)
		}
	}
}

func TestCEFExporter_MapSeverity(t *testing.T) {
	e := NewCEFExporter("V", "P", "1")
	tests := []struct {
		sev      audit.Severity
		expected int
	}{
		{audit.SeverityInfo, 3},
		{audit.SeverityWarning, 5},
		{audit.SeverityError, 7},
		{audit.SeverityCritical, 10},
		{audit.Severity("unknown"), 0},
	}
	for _, tt := range tests {
		got := e.mapSeverity(tt.sev)
		if got != tt.expected {
			t.Errorf("mapSeverity(%s) = %d, want %d", tt.sev, got, tt.expected)
		}
	}
}

func TestCEFExporter_FormatCEF_IncludesExtensions(t *testing.T) {
	e := NewCEFExporter("V", "P", "1.0")
	ev := sampleEvent("cef-test")

	cef := e.formatCEF(ev)

	checks := []string{
		"suser=user-1",
		"sname=Alice",
		"src=10.0.0.1",
		"fname=res-1",
		"filePath=/auth/login",
		"outcome=success",
		"cs1=cef-test cs1Label=EventID",
		"cs2=hash123 cs2Label=Hash",
	}

	for _, check := range checks {
		if !strings.Contains(cef, check) {
			t.Errorf("CEF output missing %q in: %s", check, cef)
		}
	}
}

func TestCEFExporter_FormatCEF_NilFields(t *testing.T) {
	e := NewCEFExporter("V", "P", "1.0")
	ev := &audit.AuditEvent{
		ID:        "bare",
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Action:    "test",
		Outcome:   audit.OutcomeSuccess,
		Hash:      "h",
	}

	cef := e.formatCEF(ev)
	if !strings.HasPrefix(cef, "CEF:0|") {
		t.Errorf("unexpected format: %s", cef)
	}
	// Should not contain actor/source/resource fields
	if strings.Contains(cef, "suser=") {
		t.Error("should not contain suser when actor is nil")
	}
}

// ===================== StreamExporter Tests =====================

func TestStreamExporter_ExportStream(t *testing.T) {
	// Create a mock storage
	ms := &mockStorage{
		events: sampleEvents(5),
	}

	jsonExp := NewJSONLExporter()
	se := NewStreamExporter(jsonExp, ms)

	var buf bytes.Buffer
	err := se.ExportStream(context.Background(), nil, &buf)
	if err != nil {
		t.Fatalf("ExportStream failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
}

func TestStreamExporter_ExportStream_WithFilter(t *testing.T) {
	ms := &mockStorage{events: sampleEvents(3)}
	jsonExp := NewJSONLExporter()
	se := NewStreamExporter(jsonExp, ms)

	filter := &audit.AuditFilter{
		Query: &audit.AuditQuery{Limit: 10},
	}

	var buf bytes.Buffer
	err := se.ExportStream(context.Background(), filter, &buf)
	if err != nil {
		t.Fatalf("ExportStream failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestStreamExporter_ExportStream_ContextCancelled(t *testing.T) {
	ms := &mockStorage{events: sampleEvents(3)}
	jsonExp := NewJSONLExporter()
	se := NewStreamExporter(jsonExp, ms)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	err := se.ExportStream(ctx, nil, &buf)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

func TestStreamExporter_ExportStream_Empty(t *testing.T) {
	ms := &mockStorage{events: []*audit.AuditEvent{}}
	jsonExp := NewJSONLExporter()
	se := NewStreamExporter(jsonExp, ms)

	var buf bytes.Buffer
	err := se.ExportStream(context.Background(), nil, &buf)
	if err != nil {
		t.Fatalf("ExportStream failed: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %d bytes", buf.Len())
	}
}

// ===================== SIEM Tests =====================

func TestDefaultSIEMConfig(t *testing.T) {
	cfg := DefaultSIEMConfig()
	if cfg.BatchSize != 100 {
		t.Errorf("expected BatchSize 100, got %d", cfg.BatchSize)
	}
	if cfg.RetryAttempts != 3 {
		t.Errorf("expected RetryAttempts 3, got %d", cfg.RetryAttempts)
	}
	if cfg.Headers == nil {
		t.Error("expected non-nil Headers")
	}
}

func TestJSONFormatter(t *testing.T) {
	f := &JSONFormatter{}
	ev := sampleEvent("jf1")

	data, err := f.Format(ev)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	var decoded audit.AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.ID != "jf1" {
		t.Errorf("expected ID jf1, got %s", decoded.ID)
	}

	if f.ContentType() != "application/json" {
		t.Errorf("expected application/json, got %s", f.ContentType())
	}
}

func TestLEEFFormatter(t *testing.T) {
	f := NewLEEFFormatter("Vendor", "Product", "1.0")
	ev := sampleEvent("leef1")

	data, err := f.Format(ev)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	s := string(data)
	if !strings.HasPrefix(s, "LEEF:2.0|Vendor|Product|1.0|user.login|") {
		t.Errorf("unexpected LEEF format: %s", s)
	}

	checks := []string{"usrName=user-1", "src=10.0.0.1", "resource=res-1", "outcome=success"}
	for _, check := range checks {
		if !strings.Contains(s, check) {
			t.Errorf("LEEF missing %q in: %s", check, s)
		}
	}

	if f.ContentType() != "text/plain" {
		t.Errorf("expected text/plain, got %s", f.ContentType())
	}
}

func TestLEEFFormatter_NilFields(t *testing.T) {
	f := NewLEEFFormatter("V", "P", "1")
	ev := &audit.AuditEvent{
		ID:        "bare",
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Action:    "test",
		Outcome:   audit.OutcomeSuccess,
		Hash:      "h",
	}

	data, err := f.Format(ev)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "usrName=") {
		t.Error("should not contain usrName when actor is nil")
	}
}

func TestNewSIEMExporter(t *testing.T) {
	exp := NewSIEMExporter(nil, nil)
	if exp == nil {
		t.Fatal("NewSIEMExporter returned nil")
	}
	if exp.Format() != "siem" {
		t.Errorf("expected 'siem', got %q", exp.Format())
	}
}

func TestSIEMExporter_SendAndStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.BatchSize = 10
	cfg.RetryAttempts = 0

	exp := NewSIEMExporter(cfg, nil)

	for i := 0; i < 3; i++ {
		if err := exp.Send(sampleEvent("s" + string(rune('0'+i)))); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
	}

	stats := exp.Stats()
	if stats.BufferSize != 3 {
		t.Errorf("expected buffer size 3, got %d", stats.BufferSize)
	}
}

func TestSIEMExporter_Flush(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.BatchSize = 100
	cfg.RetryAttempts = 0

	exp := NewSIEMExporter(cfg, nil)
	_ = exp.Send(sampleEvent("f1"))
	_ = exp.Send(sampleEvent("f2"))

	if err := exp.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	stats := exp.Stats()
	if stats.BufferSize != 0 {
		t.Errorf("expected buffer size 0 after flush, got %d", stats.BufferSize)
	}
	if stats.SentCount != 2 {
		t.Errorf("expected sent count 2, got %d", stats.SentCount)
	}
}

func TestSIEMExporter_FlushEmpty(t *testing.T) {
	cfg := DefaultSIEMConfig()
	exp := NewSIEMExporter(cfg, nil)
	// Flushing empty buffer should not error
	if err := exp.Flush(); err != nil {
		t.Fatalf("Flush of empty buffer failed: %v", err)
	}
}

func TestSIEMExporter_SendBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.BatchSize = 100
	cfg.RetryAttempts = 0

	exp := NewSIEMExporter(cfg, nil)
	events := sampleEvents(5)
	if err := exp.SendBatch(events); err != nil {
		t.Fatalf("SendBatch failed: %v", err)
	}

	stats := exp.Stats()
	if stats.BufferSize != 5 {
		t.Errorf("expected buffer size 5, got %d", stats.BufferSize)
	}
}

func TestSIEMExporter_BatchSizeFlush(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.BatchSize = 3
	cfg.RetryAttempts = 0

	exp := NewSIEMExporter(cfg, nil)

	for i := 0; i < 3; i++ {
		_ = exp.Send(sampleEvent("bf" + string(rune('0'+i))))
	}

	stats := exp.Stats()
	if stats.SentCount != 3 {
		t.Errorf("expected auto-flush after batch size, sent count = %d", stats.SentCount)
	}
	if stats.BufferSize != 0 {
		t.Errorf("expected buffer size 0 after auto-flush, got %d", stats.BufferSize)
	}
}

func TestSIEMExporter_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.BatchSize = 1
	cfg.RetryAttempts = 0
	cfg.RetryDelay = time.Millisecond

	exp := NewSIEMExporter(cfg, nil)
	err := exp.Send(sampleEvent("err1"))
	if err == nil {
		t.Error("expected error for server 500")
	}
}

func TestSIEMExporter_Export(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.RetryAttempts = 0

	exp := NewSIEMExporter(cfg, nil)
	reader, err := exp.Export(context.Background(), sampleEvents(2), nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if int(result["sent"].(float64)) != 2 {
		t.Errorf("expected sent=2, got %v", result["sent"])
	}
}

func TestSIEMExporter_AuthHeaders(t *testing.T) {
	var gotAuth string
	var gotCustom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.AuthToken = "mytoken"
	cfg.BatchSize = 1
	cfg.RetryAttempts = 0
	cfg.Headers = map[string]string{"X-Custom": "custom-value"}

	exp := NewSIEMExporter(cfg, nil)
	_ = exp.Send(sampleEvent("auth1"))

	if gotAuth != "Bearer mytoken" {
		t.Errorf("expected Bearer auth, got %q", gotAuth)
	}
	if gotCustom != "custom-value" {
		t.Errorf("expected custom-value, got %q", gotCustom)
	}
}

func TestSIEMExporter_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.FlushInterval = 50 * time.Millisecond
	cfg.RetryAttempts = 0

	exp := NewSIEMExporter(cfg, nil)
	exp.Start()

	_ = exp.Send(sampleEvent("ss1"))
	time.Sleep(100 * time.Millisecond) // Wait for timer flush

	if err := exp.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// ===================== SIEMForwarder Tests =====================

func TestSIEMForwarder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.BatchSize = 100
	cfg.RetryAttempts = 0

	fwd := NewSIEMForwarder()
	exp := NewSIEMExporter(cfg, nil)
	fwd.AddExporter(exp)

	if err := fwd.Forward(sampleEvent("fwd1")); err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
}

func TestSIEMForwarder_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSIEMConfig()
	cfg.Endpoint = server.URL
	cfg.FlushInterval = 50 * time.Millisecond
	cfg.RetryAttempts = 0

	fwd := NewSIEMForwarder()
	fwd.AddExporter(NewSIEMExporter(cfg, nil))

	fwd.Start()
	_ = fwd.Forward(sampleEvent("fwd-ss"))
	time.Sleep(80 * time.Millisecond)
	if err := fwd.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// ===================== Splunk Tests =====================

func TestDefaultSplunkConfig(t *testing.T) {
	cfg := DefaultSplunkConfig()
	if cfg.Index != "audit" {
		t.Errorf("expected index 'audit', got %q", cfg.Index)
	}
	if cfg.SourceType != "audit:events" {
		t.Errorf("expected sourcetype 'audit:events', got %q", cfg.SourceType)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("expected batch size 100, got %d", cfg.BatchSize)
	}
}

func TestSplunkExporter_Format(t *testing.T) {
	exp := NewSplunkExporter(nil)
	if exp.Format() != "splunk" {
		t.Errorf("expected 'splunk', got %q", exp.Format())
	}
}

func TestSplunkExporter_SendAndFlush(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Splunk auth header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSplunkConfig()
	cfg.HECEndpoint = server.URL
	cfg.HECToken = "test-token"
	cfg.BatchSize = 100

	exp := NewSplunkExporter(cfg)
	_ = exp.Send(sampleEvent("sp1"))
	_ = exp.Send(sampleEvent("sp2"))

	if err := exp.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	stats := exp.Stats()
	if stats.SentCount != 2 {
		t.Errorf("expected sent 2, got %d", stats.SentCount)
	}
	if stats.BufferSize != 0 {
		t.Errorf("expected buffer 0, got %d", stats.BufferSize)
	}
	if stats.Endpoint != server.URL {
		t.Errorf("expected endpoint %s, got %s", server.URL, stats.Endpoint)
	}
	if len(receivedBody) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestSplunkExporter_SendBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSplunkConfig()
	cfg.HECEndpoint = server.URL
	cfg.HECToken = "tok"
	cfg.BatchSize = 100

	exp := NewSplunkExporter(cfg)
	events := sampleEvents(4)
	if err := exp.SendBatch(events); err != nil {
		t.Fatalf("SendBatch failed: %v", err)
	}

	stats := exp.Stats()
	if stats.BufferSize != 4 {
		t.Errorf("expected buffer 4, got %d", stats.BufferSize)
	}
}

func TestSplunkExporter_AutoFlushOnBatchSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSplunkConfig()
	cfg.HECEndpoint = server.URL
	cfg.HECToken = "tok"
	cfg.BatchSize = 2

	exp := NewSplunkExporter(cfg)
	_ = exp.Send(sampleEvent("af1"))
	_ = exp.Send(sampleEvent("af2"))

	stats := exp.Stats()
	if stats.SentCount != 2 {
		t.Errorf("expected auto-flush, sent = %d", stats.SentCount)
	}
}

func TestSplunkExporter_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad"))
	}))
	defer server.Close()

	cfg := DefaultSplunkConfig()
	cfg.HECEndpoint = server.URL
	cfg.HECToken = "tok"
	cfg.BatchSize = 1

	exp := NewSplunkExporter(cfg)
	err := exp.Send(sampleEvent("err"))
	if err == nil {
		t.Error("expected error for 400 response")
	}
}

func TestSplunkExporter_Export(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSplunkConfig()
	cfg.HECEndpoint = server.URL
	cfg.HECToken = "tok"

	exp := NewSplunkExporter(cfg)
	reader, err := exp.Export(context.Background(), sampleEvents(2), nil)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, _ := io.ReadAll(reader)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	if int(result["sent"].(float64)) != 2 {
		t.Errorf("expected sent=2, got %v", result["sent"])
	}
}

func TestSplunkExporter_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultSplunkConfig()
	cfg.HECEndpoint = server.URL
	cfg.HECToken = "tok"
	cfg.FlushInterval = 50 * time.Millisecond

	exp := NewSplunkExporter(cfg)
	exp.Start()
	_ = exp.Send(sampleEvent("ss1"))
	time.Sleep(100 * time.Millisecond)
	if err := exp.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestSplunkExporter_ToHECEvent(t *testing.T) {
	cfg := DefaultSplunkConfig()
	cfg.Host = "myhost"
	exp := NewSplunkExporter(cfg)

	ev := sampleEvent("hec1")
	ev.Details = map[string]interface{}{"key": "value"}
	hecEv := exp.toHECEvent(ev)

	if hecEv.Host != "myhost" {
		t.Errorf("expected host myhost, got %s", hecEv.Host)
	}
	if hecEv.Source != "unheaded-audit" {
		t.Errorf("expected source unheaded-audit, got %s", hecEv.Source)
	}
	if hecEv.Index != "audit" {
		t.Errorf("expected index audit, got %s", hecEv.Index)
	}
	if hecEv.Event["event_id"] != "hec1" {
		t.Errorf("expected event_id hec1, got %v", hecEv.Event["event_id"])
	}
	if hecEv.Event["actor"] == nil {
		t.Error("expected actor in event data")
	}
	if hecEv.Event["resource"] == nil {
		t.Error("expected resource in event data")
	}
	if hecEv.Event["source_info"] == nil {
		t.Error("expected source_info in event data")
	}
	if hecEv.Event["details"] == nil {
		t.Error("expected details in event data")
	}
}

// ===================== PredefinedSplunkAlerts Tests =====================

func TestPredefinedSplunkAlerts(t *testing.T) {
	alerts := PredefinedSplunkAlerts()
	if len(alerts) != 4 {
		t.Errorf("expected 4 alerts, got %d", len(alerts))
	}

	names := make(map[string]bool)
	for _, a := range alerts {
		if a.Name == "" {
			t.Error("alert has empty name")
		}
		if a.Search == "" {
			t.Error("alert has empty search")
		}
		if len(a.Actions) == 0 {
			t.Error("alert has no actions")
		}
		names[a.Name] = true
	}

	expected := []string{
		"Failed Authentication Spike",
		"Critical Security Event",
		"Unusual Configuration Change",
		"Sensitive Data Access",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing alert: %s", name)
		}
	}
}

// ===================== RegisterDefaultExporters Tests =====================

func TestRegisterDefaultExporters(t *testing.T) {
	reg := audit.NewRegistry()
	RegisterDefaultExporters(reg)

	for _, format := range []string{"json", "jsonl", "csv", "cef"} {
		exp, ok := reg.GetExporter(format)
		if !ok {
			t.Errorf("expected exporter for format %q", format)
			continue
		}
		if exp.Format() != format {
			t.Errorf("expected format %q, got %q", format, exp.Format())
		}
	}
}

// ===================== mock storage for StreamExporter =====================

type mockStorage struct {
	events []*audit.AuditEvent
}

func (m *mockStorage) Store(ctx context.Context, event *audit.AuditEvent) error {
	return nil
}

func (m *mockStorage) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	return nil
}

func (m *mockStorage) Get(ctx context.Context, id string) (*audit.AuditEvent, error) {
	return nil, nil
}

func (m *mockStorage) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	offset := query.Offset
	if offset >= len(m.events) {
		return nil, nil
	}
	end := len(m.events)
	if query.Limit > 0 && offset+query.Limit < end {
		end = offset + query.Limit
	}
	return m.events[offset:end], nil
}

func (m *mockStorage) GetLastHash(ctx context.Context) (string, error) {
	return "", nil
}

func (m *mockStorage) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	return int64(len(m.events)), nil
}

func (m *mockStorage) Close() error {
	return nil
}
