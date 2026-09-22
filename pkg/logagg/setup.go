// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
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

// levels is every level Publisher can emit; Connect joins one topic per level.
var levels = []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}

// Connect opens the connection used for log forwarding and joins the
// logs.<service>.<level> topics, with serviceName as the display name.
//
// Wotan's membership model (7b16ba9d) only lets an approved subscriber
// publish to a topic, and the HTTP API enforces it server-side. A log
// publisher is therefore a member of its own log topics. The gRPC path is
// not used: its client applies the same gate but the server does not, and
// the transport-level Connection cannot join a topic without opening a
// stream. serviceName must be on Wotan's topics.auto_approve list or every
// publish fails with ErrSubscriptionPending.
//
// Best-effort: a returned error means "logs stay local".
func Connect(ctx context.Context, cfg transport.Config, serviceName string) (transport.Connection, error) {
	client, err := wotanClient.NewClient(cfg.WotanHTTPAddr)
	if err != nil {
		return nil, fmt.Errorf("logagg: wotan client: %w", err)
	}
	for _, lvl := range levels {
		topic := fmt.Sprintf("logs.%s.%s", serviceName, lvl)
		sub, err := client.Subscribe(ctx, topic, serviceName)
		if err != nil {
			return nil, fmt.Errorf("logagg: join %s: %w", topic, err)
		}
		if sub.Status != "approved" {
			return nil, fmt.Errorf("logagg: join %s as %q: %s (not on topics.auto_approve?)", topic, serviceName, sub.Status)
		}
	}
	return transport.WrapHTTPClient(client, cfg.WotanHTTPAddr), nil
}
