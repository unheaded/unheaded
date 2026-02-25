// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package logagg provides centralized log aggregation for Unheaded services.
// Services publish structured logs to Wotan topics (logs.<service>.<level>).
// The dashboard subscribes and stores them in an in-memory ring buffer
// for query, search, and live tail.
package logagg

import "time"

// LogEntry represents a single log entry in the aggregation system.
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Service   string                 `json:"service"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// LogQuery defines filters for querying log entries.
type LogQuery struct {
	// Service filters by service name (empty = all services)
	Service string
	// Level filters by log level (empty = all levels)
	Level string
	// Search performs full-text search in message and field values
	Search string
	// From is the start time (zero = no lower bound)
	From time.Time
	// To is the end time (zero = no upper bound)
	To time.Time
	// Limit is the maximum number of results (0 = default 100)
	Limit int
	// Offset is the pagination offset
	Offset int
}

// DefaultLimit is the default number of log entries returned per query.
const DefaultLimit = 100

// MaxLimit is the maximum number of log entries that can be returned per query.
const MaxLimit = 10000
