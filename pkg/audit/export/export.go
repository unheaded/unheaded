// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package export provides audit event export functionality.
package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"unheaded/pkg/audit"
)

// JSONExporter exports events as JSON.
type JSONExporter struct{}

// NewJSONExporter creates a new JSON exporter.
func NewJSONExporter() *JSONExporter {
	return &JSONExporter{}
}

// Export exports events as JSON.
func (e *JSONExporter) Export(ctx context.Context, events []*audit.AuditEvent, filter *audit.AuditFilter) (io.Reader, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(events); err != nil {
		return nil, fmt.Errorf("failed to encode events: %w", err)
	}

	return &buf, nil
}

// Format returns the export format name.
func (e *JSONExporter) Format() string {
	return "json"
}

// JSONLExporter exports events as JSON Lines (one JSON object per line).
type JSONLExporter struct{}

// NewJSONLExporter creates a new JSONL exporter.
func NewJSONLExporter() *JSONLExporter {
	return &JSONLExporter{}
}

// Export exports events as JSONL.
func (e *JSONLExporter) Export(ctx context.Context, events []*audit.AuditEvent, filter *audit.AuditFilter) (io.Reader, error) {
	var buf bytes.Buffer

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal event %s: %w", event.ID, err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}

	return &buf, nil
}

// Format returns the export format name.
func (e *JSONLExporter) Format() string {
	return "jsonl"
}

// CSVExporter exports events as CSV.
type CSVExporter struct {
	headers []string
}

// NewCSVExporter creates a new CSV exporter.
func NewCSVExporter() *CSVExporter {
	return &CSVExporter{
		headers: []string{
			"id", "timestamp", "type", "severity", "action", "outcome",
			"actor_id", "actor_type", "actor_name",
			"resource_id", "resource_type", "resource_path",
			"source_ip", "source_service", "request_id",
			"hash",
		},
	}
}

// Export exports events as CSV.
func (e *CSVExporter) Export(ctx context.Context, events []*audit.AuditEvent, filter *audit.AuditFilter) (io.Reader, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write headers
	if err := writer.Write(e.headers); err != nil {
		return nil, fmt.Errorf("failed to write headers: %w", err)
	}

	// Write events
	for _, event := range events {
		record := e.eventToRecord(event)
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write event %s: %w", event.ID, err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("csv writer error: %w", err)
	}

	return &buf, nil
}

// Format returns the export format name.
func (e *CSVExporter) Format() string {
	return "csv"
}

func (e *CSVExporter) eventToRecord(event *audit.AuditEvent) []string {
	record := make([]string, len(e.headers))

	record[0] = event.ID
	record[1] = event.Timestamp.Format(time.RFC3339)
	record[2] = string(event.Type)
	record[3] = string(event.Severity)
	record[4] = event.Action
	record[5] = string(event.Outcome)

	if event.Actor != nil {
		record[6] = event.Actor.ID
		record[7] = event.Actor.Type
		record[8] = event.Actor.Name
	}

	if event.Resource != nil {
		record[9] = event.Resource.ID
		record[10] = event.Resource.Type
		record[11] = event.Resource.Path
	}

	if event.Source != nil {
		record[12] = event.Source.IP
		record[13] = event.Source.Service
		record[14] = event.Source.RequestID
	}

	record[15] = event.Hash

	return record
}

// CEFExporter exports events in Common Event Format (CEF).
type CEFExporter struct {
	vendor  string
	product string
	version string
}

// NewCEFExporter creates a new CEF exporter.
func NewCEFExporter(vendor, product, version string) *CEFExporter {
	return &CEFExporter{
		vendor:  vendor,
		product: product,
		version: version,
	}
}

// Export exports events in CEF format.
func (e *CEFExporter) Export(ctx context.Context, events []*audit.AuditEvent, filter *audit.AuditFilter) (io.Reader, error) {
	var buf bytes.Buffer

	for _, event := range events {
		cef := e.formatCEF(event)
		buf.WriteString(cef)
		buf.WriteByte('\n')
	}

	return &buf, nil
}

// Format returns the export format name.
func (e *CEFExporter) Format() string {
	return "cef"
}

func (e *CEFExporter) formatCEF(event *audit.AuditEvent) string {
	// CEF format: CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension
	severity := e.mapSeverity(event.Severity)

	extension := fmt.Sprintf("rt=%d", event.Timestamp.UnixMilli())

	if event.Actor != nil {
		extension += fmt.Sprintf(" suser=%s", event.Actor.ID)
		if event.Actor.Name != "" {
			extension += fmt.Sprintf(" sname=%s", event.Actor.Name)
		}
	}

	if event.Source != nil {
		if event.Source.IP != "" {
			extension += fmt.Sprintf(" src=%s", event.Source.IP)
		}
		if event.Source.RequestID != "" {
			extension += fmt.Sprintf(" cn1=%s cn1Label=RequestID", event.Source.RequestID)
		}
	}

	if event.Resource != nil {
		extension += fmt.Sprintf(" fname=%s", event.Resource.ID)
		if event.Resource.Path != "" {
			extension += fmt.Sprintf(" filePath=%s", event.Resource.Path)
		}
	}

	extension += fmt.Sprintf(" outcome=%s", event.Outcome)
	extension += fmt.Sprintf(" cs1=%s cs1Label=EventID", event.ID)
	extension += fmt.Sprintf(" cs2=%s cs2Label=Hash", event.Hash)

	return fmt.Sprintf("CEF:0|%s|%s|%s|%s|%s|%d|%s",
		e.vendor,
		e.product,
		e.version,
		string(event.Type),
		event.Action,
		severity,
		extension,
	)
}

func (e *CEFExporter) mapSeverity(s audit.Severity) int {
	switch s {
	case audit.SeverityInfo:
		return 3
	case audit.SeverityWarning:
		return 5
	case audit.SeverityError:
		return 7
	case audit.SeverityCritical:
		return 10
	default:
		return 0
	}
}

// StreamExporter provides streaming export capabilities.
type StreamExporter struct {
	exporter audit.Exporter
	storage  audit.Storage
}

// NewStreamExporter creates a new streaming exporter.
func NewStreamExporter(exporter audit.Exporter, storage audit.Storage) *StreamExporter {
	return &StreamExporter{
		exporter: exporter,
		storage:  storage,
	}
}

// ExportStream exports events as a stream.
func (e *StreamExporter) ExportStream(ctx context.Context, filter *audit.AuditFilter, writer io.Writer) error {
	var query *audit.AuditQuery
	if filter != nil && filter.Query != nil {
		query = filter.Query
	} else {
		query = &audit.AuditQuery{Limit: 1000}
	}

	offset := 0
	batchSize := 1000

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		query.Offset = offset
		query.Limit = batchSize

		events, err := e.storage.Query(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to query events: %w", err)
		}

		if len(events) == 0 {
			break
		}

		reader, err := e.exporter.Export(ctx, events, filter)
		if err != nil {
			return fmt.Errorf("failed to export batch: %w", err)
		}

		if _, err := io.Copy(writer, reader); err != nil {
			return fmt.Errorf("failed to write batch: %w", err)
		}

		offset += len(events)

		if len(events) < batchSize {
			break
		}
	}

	return nil
}

// RegisterDefaultExporters registers all default exporters.
func RegisterDefaultExporters(registry *audit.Registry) {
	registry.RegisterExporter(NewJSONExporter())
	registry.RegisterExporter(NewJSONLExporter())
	registry.RegisterExporter(NewCSVExporter())
	registry.RegisterExporter(NewCEFExporter("Unheaded", "AuditSystem", "1.0"))
}

func init() {
	RegisterDefaultExporters(audit.DefaultRegistry)
}
