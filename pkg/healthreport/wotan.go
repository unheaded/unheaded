// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package healthreport

import (
	"context"
	"log"

	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

// joiner is the part of the Wotan client StartService needs.
type joiner interface {
	Publisher
	SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*wotanClient.Message)) error
}

// StartService makes the service a reporter, beside its discovery.Start:
// it joins Topic on cfg's Wotan as displayName (which must be on
// topics.auto_approve, as for discovery), then reports BallotName(
// displayName) on HostName() every DefaultInterval, probing healthURL
// (see HealthURL). Joining is retried in the background until ctx ends, so a
// service may start before Wotan. Best-effort like discovery: when cfg's
// Wotan address cannot be parsed it logs and reports nothing.
func StartService(ctx context.Context, cfg transport.Config, displayName, healthURL string) {
	c, err := wotanClient.NewClient(cfg.WotanHTTPAddr)
	if err != nil {
		log.Printf("[healthreport] %s: no Wotan client (%v); not reporting", displayName, err)
		return
	}
	startWith(ctx, c, displayName, Config{
		Service: BallotName(displayName),
		Host:    HostName(),
		Probe:   HTTPProbe(healthURL),
	})
}

func startWith(ctx context.Context, c joiner, displayName string, cfg Config) {
	go func() { // #nosec G118 -- runs until ctx ends
		// With onMsg nil this only joins, returning nil once Wotan approves.
		if err := c.SubscribeWithRetry(ctx, Topic, displayName, nil); err != nil {
			return // ctx ended before Wotan approved
		}
		<-Start(ctx, c, cfg)
	}()
}
