// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"strings"

	"unheaded/pkg/logger"
)

// loggerHook adapts a Publisher to pkg/logger's Hook interface so services
// built on pkg/logger (dashboard-backend, unheaded-daemon, sophia, monad)
// forward logs the same way zerolog services do.
type loggerHook struct {
	p *Publisher
}

// LoggerHook returns a pkg/logger Hook backed by this publisher.
func (p *Publisher) LoggerHook() logger.Hook {
	return loggerHook{p: p}
}

// Run implements logger.Hook.
func (h loggerHook) Run(entry *logger.Entry) error {
	if entry == nil {
		return nil
	}
	h.p.enqueue(strings.ToLower(entry.Level.String()), entry.Message)
	return nil
}
