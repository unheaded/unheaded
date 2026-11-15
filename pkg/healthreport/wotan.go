// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package healthreport

import (
	"context"
	"log"

	wotanClient "unheaded/pkg/wotan-client"
)

// Joiner is the part of the Wotan client StartService needs.
type Joiner interface {
	Publisher
	SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*wotanClient.Message)) error
}

// StartService makes the service a reporter, beside its discovery.Start:
// through c, the process's one Wotan client (transport.NewWotanClient), it
// joins Topic as displayName (which must be on
// topics.auto_approve, as for discovery), then reports BallotName(
// displayName) on HostName() every DefaultInterval, probing healthURL
// (see HealthURL). Joining is retried in the background until ctx ends, so a
// service may start before Wotan. Best-effort like discovery: a nil c (no
// Wotan configured) logs and reports nothing.
func StartService(ctx context.Context, c Joiner, displayName, healthURL string) {
	if wc, ok := c.(*wotanClient.Client); c == nil || ok && wc == nil { // a nil *Client is nil too
		log.Printf("[healthreport] %s: no Wotan client; not reporting", displayName)
		return
	}
	startWith(ctx, c, displayName, Config{
		Service: BallotName(displayName),
		Host:    HostName(),
		Probe:   HTTPProbe(healthURL),
	})
}

func startWith(ctx context.Context, c Joiner, displayName string, cfg Config) {
	go func() { // #nosec G118 -- runs until ctx ends
		// With onMsg nil this only joins, returning nil once Wotan approves.
		if err := c.SubscribeWithRetry(ctx, Topic, displayName, nil); err != nil {
			return // ctx ended before Wotan approved
		}
		<-Start(ctx, c, cfg)
	}()
}
