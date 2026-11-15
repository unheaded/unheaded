// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// Announcer is the part of the process's one Wotan client Start needs: it
// joins system.discovery and publishes on it. It has no StreamMessages, so
// announcing cannot open a stream (ADR-147 Phase 7, Developer D3, D6).
type Announcer interface {
	Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error)
	Publish(ctx context.Context, topic string, payload []byte) error
}

// streamer is an Announcer that can also stream: what a reader of
// system.discovery needs (the Registry, through wotanTopics).
type streamer interface {
	Announcer
	StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error)
}

// wotanTopics adapts a streamer to the Registry's WotanClient.
type wotanTopics struct {
	ctx context.Context
	c   streamer
}

func (w *wotanTopics) Publish(topic string, data []byte) error {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()
	return w.c.Publish(ctx, topic, data)
}

func (w *wotanTopics) Subscribe(topic string, handler func([]byte)) error {
	ch, err := w.c.StreamMessages(w.ctx, topic)
	if err != nil {
		return err
	}
	go func() {
		for m := range ch {
			if m != nil && !m.Deleted && !replayed(m.CreatedAt, time.Now()) {
				handler([]byte(m.Payload))
			}
		}
	}()
	return nil
}

// replayed reports whether a message Wotan stamped at created is history
// rather than a sighting. A stream starts at seq 0, so it first replays the
// topic's retained buffer — hours of announcements, from services that may
// have stopped long ago — and the registry stamps each one LastSeen=now as
// it arrives. Anything older than the TTL is already stale on arrival and
// would only resurrect a dead peer until the reaper caught up.
//
// created is Wotan's clock and now is ours; a Wotan on another host whose
// clock runs more than DefaultTTL behind would make every announcement look
// stale. A zero time (a message without one) is accepted.
func replayed(created, now time.Time) bool {
	return !created.IsZero() && now.Sub(created) > DefaultTTL
}

// Start announces name on Wotan's system.discovery topic through c, the
// process's one Wotan client (transport.NewWotanClient), and keeps it
// announced every AnnounceInterval. It opens no stream, builds no registry
// and returns nothing: announcing is all a service does here (the reader is
// a separate role). The address announced is $DISCOVERY_ADDR, else the
// hostname (in compose, the container's short ID, which Docker's DNS
// resolves). name is also the Wotan display name, so it must be on
// topics.auto_approve (configs/wotan.yaml).
//
// Best-effort: a service may start before Wotan, so joining is retried in
// the background (1 s doubling to 1 min) until ctx ends, and only the first
// failure and the recovery are logged. A nil c logs once and does nothing.
func Start(ctx context.Context, c Announcer, name string, port int) {
	if wc, ok := c.(*wotanClient.Client); c == nil || ok && wc == nil { // a nil *Client is nil too
		log.Printf("[discovery] %s: no Wotan client; not announced", name)
		return
	}
	addr := os.Getenv("DISCOVERY_ADDR")
	if addr == "" {
		var err error
		if addr, err = os.Hostname(); err != nil || addr == "" {
			addr = "localhost"
		}
	}
	start(ctx, c, ServiceEntry{Name: name, Address: addr, Port: port}, time.Second, time.Minute, AnnounceInterval)
}

func start(ctx context.Context, c Announcer, self ServiceEntry, first, max, every time.Duration) {
	if self.Health == "" {
		self.Health = "healthy"
	}
	go func() { // #nosec G118 -- runs until ctx ends
		backoff, failures := first, 0
		for {
			err := join(ctx, c, self.Name)
			if err == nil {
				if failures > 0 {
					log.Printf("[discovery] %s: registered after %d failed attempts", self.Name, failures)
				}
				announceLoop(ctx, c, self, every)
				return
			}
			if failures == 0 {
				log.Printf("[discovery] %s: not registered yet, retrying in the background: %v", self.Name, err)
			}
			failures++
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff *= 2; backoff > max {
				backoff = max
			}
		}
	}()
}

// announceLoop publishes self now and every interval until ctx ends. A
// failed publish is not fatal: the next tick repeats it, and a peer's TTL
// (DefaultTTL, two intervals) absorbs one miss.
func announceLoop(ctx context.Context, c Announcer, self ServiceEntry, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		_ = announce(ctx, c, self)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func announce(ctx context.Context, c Announcer, self ServiceEntry) error {
	self.LastSeen = time.Now()
	data, err := json.Marshal(self)
	if err != nil {
		return fmt.Errorf("marshal announcement: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.Publish(ctx, DiscoveryTopic, data)
}

// join makes name an approved member of the discovery topic: Wotan only
// lets members publish, and only display names on auto_approve are
// approved without an operator.
func join(ctx context.Context, c Announcer, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sub, err := c.Subscribe(ctx, DiscoveryTopic, name)
	if err != nil {
		return fmt.Errorf("join %s: %w", DiscoveryTopic, err)
	}
	if sub.Status != "approved" {
		return fmt.Errorf("join %s as %q: %s (not on topics.auto_approve?)", DiscoveryTopic, name, sub.Status)
	}
	return nil
}
