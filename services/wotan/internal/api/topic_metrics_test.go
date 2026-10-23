// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"testing"

	"unheaded/services/wotan/internal/metrics"
)

// messages_created_total was only counted on the gRPC publish path. On east
// (2026-10-23) it had series for the 7 gRPC-fed topics (traces.*, ebpf.*)
// and none for the 57 HTTP-fed ones: system.health.reports, cuirass.metrics
// and every logs.* topic took messages that the counter never saw.
func TestPublishTopic_CountsMessagesCreated(t *testing.T) {
	m := metrics.Get()
	if m == nil {
		m = metrics.Initialize("api_test_created")
	}
	srv := setupTestServerWithTopics()
	id := subscribeID(t, srv, "count.me")
	before := m.MessagesCreated.WithLabelValues("count.me").Value()
	for i := 0; i < 3; i++ {
		publish(t, srv, "count.me", id, "x")
	}
	if got := m.MessagesCreated.WithLabelValues("count.me").Value() - before; got != 3 {
		t.Fatalf("messages_created_total{room_id=count.me} rose by %v after 3 publishes, want 3", got)
	}
}
