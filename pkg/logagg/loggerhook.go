// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"encoding/json"
	"strings"

	"unheaded/pkg/logger"
)

// loggerHook adapts a Publisher to pkg/logger's Hook interface so services
// built on pkg/logger (dashboard-backend, unheaded-daemon, sophia, monad,
// kanban-app) forward logs the same way zerolog services do.
type loggerHook struct {
	p *Publisher
}

// LoggerHook returns a pkg/logger Hook backed by this publisher.
func (p *Publisher) LoggerHook() logger.Hook {
	return loggerHook{p: p}
}

// Run implements logger.Hook. The structured fields come from entry.Raw,
// the JSON line pkg/logger wrote; Entry.Fields is never filled in. Raw is
// the logger's pooled buffer, so it is parsed here, before Run returns.
func (h loggerHook) Run(entry *logger.Entry) error {
	if entry == nil || !h.p.Enabled() {
		return nil
	}
	h.p.enqueueFields(strings.ToLower(entry.Level.String()), entry.Message, rawFields(entry.Raw, h.p.serviceName))
	return nil
}

// rawFields returns the fields of a JSON log line other than those LogEntry
// already carries (level, message, time, and service when it names this
// publisher's own service). Nil when the line is not a JSON object or has
// nothing else. Default pkg/logger field names; a logger configured with
// other names forwards those keys as fields too, duplicated but not lost.
func rawFields(raw []byte, service string) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	for _, k := range []string{"level", "message", "time"} {
		delete(fields, k)
	}
	if s, ok := fields["service"].(string); ok && s == service {
		delete(fields, "service")
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}
