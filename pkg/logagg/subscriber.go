// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logagg

import (
	"context"
	"encoding/json"
	"sync"

	"unheaded/pkg/transport"
)

// Subscriber listens to Wotan log topics and feeds entries into a ring buffer.
// Used by dashboard-backend to aggregate logs from all services.
type Subscriber struct {
	conn    transport.Connection
	buffer  *RingBuffer
	topic   string // Wotan topic pattern (e.g., "logs.>")
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	running bool
}

// NewSubscriber creates a log subscriber that feeds into the given buffer.
func NewSubscriber(conn transport.Connection, buf *RingBuffer) *Subscriber {
	return &Subscriber{
		conn:   conn,
		buffer: buf,
		topic:  "logs.>", // wildcard: all log topics
		done:   make(chan struct{}),
	}
}

// Start begins listening for log messages on the configured topic.
// It blocks until the context is cancelled or the connection is lost.
func (s *Subscriber) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		close(s.done)
		s.mu.Unlock()
	}()

	ch, err := s.conn.Subscribe(ctx, s.topic)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data, ok := <-ch:
			if !ok {
				return nil
			}
			var entry LogEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				continue // skip malformed entries
			}
			s.buffer.Push(entry)
		}
	}
}

// Stop stops the subscriber.
func (s *Subscriber) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

// Running returns whether the subscriber is actively listening.
func (s *Subscriber) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
