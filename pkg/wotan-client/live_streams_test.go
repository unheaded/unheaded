// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"testing"
	"time"
)

// TestLiveStreamsSkipHistory: WithLiveStreams starts a stream at the live
// position, over gRPC and over the HTTP fallback, so a live view (the
// dashboard) does not re-count Wotan's held history as new (ADR-147
// Phase 7: the dashboard's TopicStreamClient had WithLiveStart; its one
// Client needs the same).
func TestLiveStreamsSkipHistory(t *testing.T) {
	for _, grpcUp := range []bool{true, false} {
		name := "grpc"
		if !grpcUp {
			name = "http-poll"
		}
		t.Run(name, func(t *testing.T) {
			w := newSwitchWotan(t)
			for seq := int64(1); seq <= 3; seq++ {
				w.publish("traces.flow", seq)
			}
			if !grpcUp {
				w.refuse()
			}
			c, err := newClientWithGRPC(w.httpAddr, w.grpcAddr, time.Hour, WithLiveStreams())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { c.Close() })
			c.subscribers["traces.flow"] = &Subscriber{SubscriberID: "s", DisplayName: "dashboard-backend", Status: "approved"}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ch, err := c.StreamMessages(ctx, "traces.flow")
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(1200 * time.Millisecond) // the stream (or first poll) is in place
			w.publish("traces.flow", 4)
			select {
			case m := <-ch:
				if m.Seq != 4 {
					t.Fatalf("first message seq %d, want 4 (history 1-3 is not live)", m.Seq)
				}
			case <-ctx.Done():
				t.Fatal("no live message")
			}
		})
	}
}
