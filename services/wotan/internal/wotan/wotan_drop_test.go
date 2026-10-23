// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotan

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/ringbuffer"
)

// Publish drops an event for a subscriber whose buffer is full, and said
// nothing: stream_messages_sent_total counts what streams did send, so a
// subscriber losing most of a busy topic looked the same as one keeping up.
func TestPublish_CountsEventsDroppedForAFullSubscriber(t *testing.T) {
	m := metrics.Get()
	if m == nil {
		m = metrics.Initialize("wotan_drop_test")
	}
	dropped := m.StreamErrors.WithLabelValues("drops", "subscriber_buffer_full")
	before := dropped.Value()

	b := NewWotan()
	sub := b.Subscribe("drops", uuid.New(), 1)
	defer b.Unsubscribe(sub)
	for i := 0; i < 3; i++ {
		b.PublishMessageCreated(&ringbuffer.Message{ID: uuid.New(), RoomID: "drops", Timestamp: time.Now()})
	}

	if got := dropped.Value() - before; got != 2 {
		t.Fatalf(`stream_errors_total{room_id="drops",error_type="subscriber_buffer_full"} rose by %v, want 2 (buffer 1, 3 published)`, got)
	}
}
