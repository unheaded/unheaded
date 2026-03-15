// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ELKAdapter forwards logs to Elasticsearch (ELK stack).
// It buffers documents for bulk indexing and generates Logstash pipeline
// configs and Kibana index patterns.
type ELKAdapter struct {
	mu        sync.RWMutex
	documents []ElasticDocument
	maxBuffer int
	index     string
	closed    atomic.Bool
}

// ElasticDocument represents an Elasticsearch document.
type ElasticDocument struct {
	Index     string            `json:"_index"`
	Timestamp string            `json:"@timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Service   string            `json:"service"`
	TraceID   string            `json:"trace_id,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// NewELKAdapter creates an ELK logging adapter.
// maxBuffer limits the in-memory document buffer (0 = unlimited).
// index sets the Elasticsearch index name pattern.
func NewELKAdapter(index string, maxBuffer int) *ELKAdapter {
	if index == "" {
		index = "unheaded-logs"
	}
	return &ELKAdapter{
		maxBuffer: maxBuffer,
		index:     index,
	}
}

func (a *ELKAdapter) Backend() Backend { return BackendELK }

func (a *ELKAdapter) Supports() []SignalType {
	return []SignalType{SignalLog}
}

func (a *ELKAdapter) EmitMetric(_ context.Context, _ Metric) error {
	return nil
}

func (a *ELKAdapter) EmitLog(_ context.Context, e LogEntry) error {
	if a.closed.Load() {
		return ErrAdapterClosed
	}

	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	doc := ElasticDocument{
		Index:     fmt.Sprintf("%s-%s", a.index, ts.Format("2006.01.02")),
		Timestamp: ts.Format(time.RFC3339Nano),
		Level:     e.Level,
		Message:   e.Message,
		Service:   e.Service,
		TraceID:   e.TraceID,
		Fields:    e.Fields,
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.maxBuffer > 0 && len(a.documents) >= a.maxBuffer {
		a.documents = a.documents[1:]
	}
	a.documents = append(a.documents, doc)
	return nil
}

func (a *ELKAdapter) EmitTrace(_ context.Context, _ Span) error {
	return nil
}

func (a *ELKAdapter) EmitAlert(_ context.Context, _ Alert) error {
	return nil
}

func (a *ELKAdapter) Close() error {
	a.closed.Store(true)
	return nil
}

// Documents returns all buffered documents.
func (a *ELKAdapter) Documents() []ElasticDocument {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]ElasticDocument, len(a.documents))
	copy(result, a.documents)
	return result
}

// DocumentCount returns the number of buffered documents.
func (a *ELKAdapter) DocumentCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.documents)
}

// BulkPayload returns an Elasticsearch bulk API compatible payload.
func (a *ELKAdapter) BulkPayload() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var payload string
	for _, doc := range a.documents {
		action := fmt.Sprintf(`{"index":{"_index":"%s"}}`, doc.Index)
		body, _ := json.Marshal(doc)
		payload += action + "\n" + string(body) + "\n"
	}
	return payload
}

// LogstashConfig generates a Logstash pipeline configuration.
func LogstashConfig(service string, esHost string) string {
	if esHost == "" {
		esHost = "localhost:9200"
	}
	return fmt.Sprintf(`input {
  tcp {
    port => 5044
    codec => json
    tags => ["%s"]
  }
}

filter {
  if [service] == "%s" {
    mutate {
      add_field => { "[@metadata][target_index]" => "unheaded-%s-%%{+YYYY.MM.dd}" }
    }
    date {
      match => [ "timestamp", "ISO8601" ]
    }
  }
}

output {
  elasticsearch {
    hosts => ["%s"]
    index => "%%{[@metadata][target_index]}"
  }
}
`, service, service, service, esHost)
}

// KibanaIndexPattern generates a Kibana index pattern definition.
func KibanaIndexPattern(indexPrefix string) string {
	if indexPrefix == "" {
		indexPrefix = "unheaded-logs"
	}
	pattern := map[string]any{
		"type": "index-pattern",
		"index-pattern": map[string]any{
			"title":         indexPrefix + "-*",
			"timeFieldName": "@timestamp",
			"fields": []map[string]string{
				{"name": "level", "type": "keyword"},
				{"name": "message", "type": "text"},
				{"name": "service", "type": "keyword"},
				{"name": "trace_id", "type": "keyword"},
			},
		},
	}
	data, _ := json.MarshalIndent(pattern, "", "  ")
	return string(data)
}
