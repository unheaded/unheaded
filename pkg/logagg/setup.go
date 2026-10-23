// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

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
// Best-effort: a returned error means "logs stay local". That is now only
// for a name Wotan will not approve. A Wotan it cannot reach is not an
// error: the connection joins each topic on a later publish instead. It
// used to fail, and the caller then left log forwarding off for the life of
// the process: on east five services that started alongside Wotan at 18:15
// and 18:17 on 2026-10-20 ("join logs.X.trace: ... connection refused")
// forwarded no logs until they were redeployed 17 hours later.
func Connect(ctx context.Context, cfg transport.Config, serviceName string) (transport.Connection, error) {
	client, err := wotanClient.NewClient(cfg.WotanHTTPAddr)
	if err != nil {
		return nil, fmt.Errorf("logagg: wotan client: %w", err)
	}
	conn := &joiningConn{
		Connection: transport.WrapHTTPClient(client, cfg.WotanHTTPAddr),
		client:     client,
		name:       serviceName,
		nextJoin:   make(map[string]time.Time),
	}
	for _, lvl := range levels {
		topic := fmt.Sprintf("logs.%s.%s", serviceName, lvl)
		sub, err := client.Subscribe(ctx, topic, serviceName)
		if err != nil {
			break // unreachable for now: join on publish
		}
		if sub.Status != "approved" {
			return nil, fmt.Errorf("logagg: join %s as %q: %s (not on topics.auto_approve?)", topic, serviceName, sub.Status)
		}
	}
	return conn, nil
}

// joinRetry is how often a joiningConn tries again to join a topic.
var joinRetry = 30 * time.Second

// joiningConn joins a log topic on the first publish to it that Connect
// could not join, retrying at most once per joinRetry per topic.
type joiningConn struct {
	transport.Connection
	client *wotanClient.Client
	name   string

	mu       sync.Mutex
	nextJoin map[string]time.Time
}

// Publish joins topic first if it has not been joined.
func (j *joiningConn) Publish(ctx context.Context, topic string, data []byte) error {
	if _, ok := j.client.GetSubscriber(topic); !ok {
		if err := j.join(ctx, topic); err != nil {
			return err
		}
	}
	return j.Connection.Publish(ctx, topic, data)
}

func (j *joiningConn) join(ctx context.Context, topic string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.client.GetSubscriber(topic); ok {
		return nil // another publish joined it meanwhile
	}
	now := time.Now()
	if now.Before(j.nextJoin[topic]) {
		return fmt.Errorf("logagg: %s not joined yet", topic)
	}
	j.nextJoin[topic] = now.Add(joinRetry)
	if _, err := j.client.Subscribe(ctx, topic, j.name); err != nil {
		return fmt.Errorf("logagg: join %s: %w", topic, err)
	}
	return nil
}
