// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

// topicClient is the part of Wotan's HTTP client Start needs.
type topicClient interface {
	Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error)
	Publish(ctx context.Context, topic string, payload []byte) error
	StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error)
}

// wotanTopics adapts topicClient to the Registry's WotanClient.
type wotanTopics struct {
	ctx context.Context
	c   topicClient
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

// Start registers name on Wotan's system.discovery topic and keeps it
// announced (AnnounceInterval, DefaultTTL); peers' announcements fill the
// returned Registry. The address announced is $DISCOVERY_ADDR, else the
// hostname (in compose, the container's short ID, which Docker's DNS
// resolves). name is also the Wotan display name, so it must be on
// topics.auto_approve (configs/wotan.yaml).
//
// Best-effort: a service may start before Wotan, so joining is retried in
// the background (1 s doubling to 1 min) until ctx ends, and only the first
// failure and the recovery are logged. Returns nil only when cfg's Wotan
// address cannot be parsed.
func Start(ctx context.Context, cfg transport.Config, name string, port int) *Registry {
	c, err := wotanClient.NewClient(cfg.WotanHTTPAddr)
	if err != nil {
		log.Printf("[discovery] %s: no Wotan client (%v); not registered", name, err)
		return nil
	}
	addr := os.Getenv("DISCOVERY_ADDR")
	if addr == "" {
		if addr, err = os.Hostname(); err != nil || addr == "" {
			addr = "localhost"
		}
	}
	return start(ctx, c, ServiceEntry{Name: name, Address: addr, Port: port}, time.Second, time.Minute)
}

func start(ctx context.Context, c topicClient, self ServiceEntry, first, max time.Duration) *Registry {
	reg := New(&wotanTopics{ctx: ctx, c: c})
	go func() { // #nosec G118 -- runs until ctx ends; the registry's own loops stop with it
		backoff, failures := first, 0
		for {
			err := join(ctx, c, self.Name)
			if err == nil {
				err = reg.Start(ctx)
			}
			if err == nil {
				if failures > 0 {
					log.Printf("[discovery] %s: registered after %d failed attempts", self.Name, failures)
				}
				_ = reg.Register(self) // the announce loop repeats it every AnnounceInterval
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
	return reg
}

// join makes name an approved member of the discovery topic: Wotan only
// lets members publish, and only display names on auto_approve are
// approved without an operator.
func join(ctx context.Context, c topicClient, name string) error {
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
