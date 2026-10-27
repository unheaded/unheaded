// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package healthreport

import (
	"context"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

type fakeJoiner struct {
	*fakePub
	mu     sync.Mutex
	joined []string
}

func (f *fakeJoiner) SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*wotanClient.Message)) error {
	f.mu.Lock()
	f.joined = append(f.joined, topic+" as "+name)
	f.mu.Unlock()
	if onMsg != nil {
		panic("reporter must only join, not stream")
	}
	return ctx.Err()
}

func TestStartWithJoinsThenReports(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := &fakeJoiner{fakePub: newFakePub()}
	startWith(ctx, j, "kanban-app", Config{
		Service: "kanban", Host: "west", Interval: time.Hour,
		Probe: func(context.Context) error { return nil },
	})
	wait(t, j.fakePub)
	j.mu.Lock()
	joined := append([]string(nil), j.joined...)
	j.mu.Unlock()
	if len(joined) != 1 || joined[0] != "system.health.reports as kanban-app" {
		t.Fatalf("joined %v", joined)
	}
	if _, r := j.last(t); r.Reporter != "kanban@west" {
		t.Fatalf("reporter %q", r.Reporter)
	}
}

func TestStartWithEndedContextReportsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j := &fakeJoiner{fakePub: newFakePub()}
	startWith(ctx, j, "sophia", Config{
		Service: "sophia", Host: "west", Probe: func(context.Context) error { return nil },
	})
	time.Sleep(50 * time.Millisecond)
	if j.count() != 0 {
		t.Fatal("reported after ctx ended before the join")
	}
}

func TestStartServiceBadWotanAddrIsBestEffort(t *testing.T) {
	// Must not panic or block when the address cannot be parsed.
	StartService(context.Background(), transport.Config{WotanHTTPAddr: "http://[::1"}, "timeguru", "http://127.0.0.1:1/health")
}
