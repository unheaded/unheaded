// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"log"
	"math/rand/v2"
	"time"
)

// Retry bounds for SubscribeWithRetry.
var (
	subscribeRetryMin = 1 * time.Second
	subscribeRetryMax = 30 * time.Second
)

// SubscribeWithRetry joins topic as name and, when onMsg is not nil, streams
// it to onMsg, retrying until ctx ends: 1 s, doubling to 30 s, with jitter.
// A Wotan that starts after the service, refuses the join, keeps it pending
// or ends the stream is retried instead of disabling the subscription for
// the life of the process (timeguru and architect used to log "degraded
// mode" once at start and never try again).
//
// With onMsg nil it only joins (to publish on the topic) and returns nil once
// Wotan approves. Otherwise it blocks until ctx ends and returns ctx.Err(), so
// run it in a goroutine. After Close it returns nil at its next attempt. The
// degraded-mode line is logged once, at the first failure, and recovery once.
func (c *Client) SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*Message)) error {
	failing := false
	attempt := 0
	for {
		ch, err := c.joinAndStream(ctx, topic, name, onMsg != nil)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && c.isClosed() {
			return nil // Close ends the retries
		}
		if err != nil {
			if !failing {
				log.Printf("[WARN] wotan_client: %s cannot join %s, retrying in the background (degraded mode): %v", name, topic, err)
				failing = true
			}
			attempt++
			if !sleepCtx(ctx, retryDelay(attempt)) {
				return ctx.Err()
			}
			continue
		}
		if failing {
			log.Printf("[INFO] wotan_client: %s joined %s after %d retries", name, topic, attempt)
			failing, attempt = false, 0
		}
		if onMsg == nil {
			return nil
		}
		for msg := range ch {
			if msg != nil {
				onMsg(msg)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if c.isClosed() {
			return nil // Close ended the stream
		}
		// The stream ended on its own: join and stream again, after a pause
		// so a stream that keeps ending cannot spin.
		if !sleepCtx(ctx, retryDelay(1)) {
			return ctx.Err()
		}
	}
}

// joinAndStream joins topic as name and, when stream is set, opens its
// message stream.
func (c *Client) joinAndStream(ctx context.Context, topic, name string, stream bool) (<-chan *Message, error) {
	sub, err := c.Subscribe(ctx, topic, name)
	if err != nil {
		return nil, err
	}
	if sub.Status != "approved" {
		return nil, ErrSubscriptionPending
	}
	if !stream {
		return nil, nil
	}
	return c.StreamMessages(ctx, topic)
}

// sleepCtx waits d or until ctx ends; false means ctx ended.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// retryDelay is min(30 s, 1 s * 2^(attempt-1)) with +/-20 % jitter, so many
// services restarting with Wotan do not retry in lockstep.
func retryDelay(attempt int) time.Duration {
	d := subscribeRetryMin
	for i := 1; i < attempt && d < subscribeRetryMax; i++ {
		d *= 2
	}
	if d > subscribeRetryMax {
		d = subscribeRetryMax
	}
	jitter := 0.8 + 0.4*rand.Float64() // #nosec G404 -- retry jitter, not a secret
	return time.Duration(float64(d) * jitter)
}
