// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package anomaly

import (
	"context"
	"os"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/wotantest"
)

// An anomaly service that starts before Wotan joins its topics and streams
// alerts.critical and anomaly.scores once Wotan comes up. Start used to
// subscribe once per topic and give up.
func TestLateWotan_Anomaly(t *testing.T) {
	fake := wotantest.Reserve(t)
	client, err := wotanClient.NewClient(fake.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	svc := NewService(logger.New(os.Stderr), client)
	svc.ListenAddr = "127.0.0.1:0" // ephemeral; the default is the real port
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = svc.Stop()
	})

	start := time.Now()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start with Wotan down: %v", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("Start blocked %v with Wotan down", took)
	}
	time.Sleep(100 * time.Millisecond)

	fake.Start(t)

	joined := []string{"anomaly.scores", "anomaly.alerts", "anomaly.baseline", "alerts.critical"}
	streamed := []string{"alerts.critical", ScoreTopic}
	ok := wotantest.WaitFor(10*time.Second, func() bool {
		for _, topic := range joined {
			if len(fake.Joins(topic)) == 0 {
				return false
			}
		}
		for _, topic := range streamed {
			if fake.Polls(topic) == 0 {
				return false
			}
		}
		return true
	})
	if !ok {
		for _, topic := range joined {
			t.Logf("%s: joins=%v polls=%d", topic, fake.Joins(topic), fake.Polls(topic))
		}
		t.Fatal("anomaly did not join after Wotan came up")
	}
	for _, topic := range joined {
		for _, name := range fake.Joins(topic) {
			if name != "anomaly-service" {
				t.Errorf("%s joined as %q", topic, name)
			}
		}
	}
}
