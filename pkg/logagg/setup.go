// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logagg

import (
	"os"

	"github.com/rs/zerolog"

	"unheaded/pkg/transport"
)

// SetupServiceLogger creates a zerolog.Logger with the log aggregation
// publisher hook. This is the one-liner for services to adopt centralized
// logging — just call this instead of building the logger manually.
//
// If conn is nil, the publisher hook is disabled and logs only go to stdout.
func SetupServiceLogger(serviceName string, conn transport.Connection) zerolog.Logger {
	publisher := NewPublisher(serviceName, conn)
	return zerolog.New(os.Stdout).
		With().Timestamp().Str("service", serviceName).Logger().
		Hook(publisher)
}
