// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// The fallback queue keeps telemetry and event publishes that failed because
// Wotan was unreachable, and a background flusher delivers them once it
// answers again (S-REAL-STEEL D-10). Until 2026-10-23 nothing called
// FlushFallbackQueue, so the queue filled to 1000 and then dropped the oldest
// message forever.
const (
	// maxFallbackQueue is the most messages held; the oldest is dropped when full.
	maxFallbackQueue = 1000
	// maxFallbackAge is how long a buffered message stays worth sending.
	// Older ones are dropped and counted as expired: minutes-old metrics
	// replayed as current are noise.
	maxFallbackAge = 2 * time.Minute
	// fallbackFlushEvery is how often the flusher retries while the queue is
	// not empty. A successful publish or gRPC probe also wakes it at once.
	fallbackFlushEvery = 15 * time.Second
	// fallbackFlushTimeout bounds one flush pass.
	fallbackFlushTimeout = 30 * time.Second
)

// noBufferPrefixes are control topics that are never buffered: a late
// chaos.abort or deploy.* replayed minutes after the fact is worse than one
// that fails now, so Publish returns the error to the caller instead.
var noBufferPrefixes = []string{"chaos.", "config.", "deploy.", "cuirass."}

// bufferable reports whether a failed publish on topic may be queued.
func bufferable(topic string) bool {
	for _, p := range noBufferPrefixes {
		if strings.HasPrefix(topic, p) {
			return false
		}
	}
	return true
}

// pendingMessage is a message waiting in the fallback queue for retry.
type pendingMessage struct {
	topic      string
	payload    []byte
	enqueuedAt time.Time
}

// FallbackStats counts what happened to publishes that could not reach Wotan.
type FallbackStats struct {
	Queued  int64 // buffered for retry
	Flushed int64 // delivered later by a flush
	Expired int64 // dropped unsent because older than the max age
	Dropped int64 // dropped: queue full, topic left, or refused by Wotan on retry
	Refused int64 // never buffered: control topic (chaos., config., deploy., cuirass.)
	Pending int   // in the queue now
}

// FallbackStats returns the fallback queue's counters.
func (c *Client) FallbackStats() FallbackStats {
	return FallbackStats{
		Queued:  atomic.LoadInt64(&c.fallbackQueued),
		Flushed: atomic.LoadInt64(&c.fallbackFlushed),
		Expired: atomic.LoadInt64(&c.fallbackExpired),
		Dropped: atomic.LoadInt64(&c.fallbackDrops),
		Refused: atomic.LoadInt64(&c.fallbackRefused),
		Pending: c.FallbackQueueLen(),
	}
}

// countFallback records one fallback outcome in the client counter and the
// unheaded_wotan_client_fallback_total metric.
func (c *Client) countFallback(result string, n int64) {
	if n <= 0 {
		return
	}
	switch result {
	case "queued":
		atomic.AddInt64(&c.fallbackQueued, n)
	case "flushed":
		atomic.AddInt64(&c.fallbackFlushed, n)
	case "expired":
		atomic.AddInt64(&c.fallbackExpired, n)
	case "dropped":
		atomic.AddInt64(&c.fallbackDrops, n)
	case "refused":
		atomic.AddInt64(&c.fallbackRefused, n)
	}
	wotanClientFallbackTotal.WithLabelValues(result).Add(float64(n))
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Client) fallbackMaxAgeOr() time.Duration {
	if c.fallbackMaxAge > 0 {
		return c.fallbackMaxAge
	}
	return maxFallbackAge
}

func (c *Client) fallbackEveryOr() time.Duration {
	if c.fallbackEvery > 0 {
		return c.fallbackEvery
	}
	return fallbackFlushEvery
}

// enqueueFallback adds a message to the local fallback queue and makes sure a
// flusher is running. A control topic is refused, not queued. If the queue is
// full the oldest message is dropped.
func (c *Client) enqueueFallback(topic string, payload []byte) {
	if !bufferable(topic) {
		c.countFallback("refused", 1)
		return
	}
	c.fallbackMu.Lock()
	var dropped int64
	if len(c.fallbackQueue) >= maxFallbackQueue {
		c.fallbackQueue = c.fallbackQueue[1:]
		dropped = 1
	}
	// Copy payload to avoid caller mutation.
	buf := make([]byte, len(payload))
	copy(buf, payload)
	c.fallbackQueue = append(c.fallbackQueue, pendingMessage{topic: topic, payload: buf, enqueuedAt: c.clock()})
	c.fallbackMu.Unlock()

	c.countFallback("dropped", dropped)
	c.countFallback("queued", 1)
	c.updateFallbackMetrics()
	c.ensureFlusher()
}

// FlushFallbackQueue re-publishes buffered messages, oldest first. It stops at
// the first network failure so the rest keep their order, drops entries older
// than the max age, and joins again (once per topic) when Wotan has forgotten
// the membership, which it does on restart. Returns the number delivered and
// the last error.
func (c *Client) FlushFallbackQueue(ctx context.Context) (flushed int, err error) {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	c.fallbackMu.Lock()
	queue := c.fallbackQueue
	c.fallbackQueue = nil
	c.fallbackMu.Unlock()

	now := c.clock()
	maxAge := c.fallbackMaxAgeOr()
	var expired, dropped int64
	rejoined := map[string]bool{}
	var remaining []pendingMessage

	for i, msg := range queue {
		if now.Sub(msg.enqueuedAt) > maxAge {
			expired++
			continue
		}
		c.mu.RLock()
		sub, ok := c.subscribers[msg.topic]
		c.mu.RUnlock()
		if !ok {
			dropped++ // topic no longer subscribed
			continue
		}

		netErr, appErr := c.doPublish(ctx, msg.topic, sub.SubscriberID, msg.payload)
		if errors.Is(appErr, ErrSubscriberNotApproved) && !rejoined[msg.topic] {
			rejoined[msg.topic] = true
			fresh, jerr := c.Subscribe(ctx, msg.topic, sub.DisplayName)
			switch {
			case jerr != nil:
				netErr, appErr = fmt.Errorf("rejoin %s: %w", msg.topic, jerr), nil
			case fresh.Status != "approved":
				appErr = ErrSubscriptionPending
			default:
				netErr, appErr = c.doPublish(ctx, msg.topic, fresh.SubscriberID, msg.payload)
			}
		}
		if netErr != nil {
			err = netErr
			remaining = queue[i:]
			break
		}
		if appErr != nil {
			err = appErr
			dropped++ // Wotan refused it; a retry would not change that
			continue
		}
		flushed++
	}

	if len(remaining) > 0 {
		c.fallbackMu.Lock()
		c.fallbackQueue = append(append([]pendingMessage(nil), remaining...), c.fallbackQueue...)
		if over := len(c.fallbackQueue) - maxFallbackQueue; over > 0 {
			c.fallbackQueue = c.fallbackQueue[over:]
			dropped += int64(over)
		}
		c.fallbackMu.Unlock()
	}

	c.countFallback("flushed", int64(flushed))
	c.countFallback("expired", expired)
	c.countFallback("dropped", dropped)
	c.updateFallbackMetrics()
	return flushed, err
}

// FallbackQueueLen returns the current number of messages in the fallback queue.
func (c *Client) FallbackQueueLen() int {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	return len(c.fallbackQueue)
}

// FallbackDrops returns the total number of buffered messages dropped unsent
// for any reason other than age (queue full, topic left, refused on retry).
func (c *Client) FallbackDrops() int64 {
	return atomic.LoadInt64(&c.fallbackDrops)
}

// ensureFlusher starts the background flusher unless one is running or the
// client is closed.
func (c *Client) ensureFlusher() {
	c.flusherMu.Lock()
	defer c.flusherMu.Unlock()
	if c.closed || c.flusherOn {
		return
	}
	if c.flusherCtx == nil {
		c.flusherCtx, c.flusherCancel = context.WithCancel(context.Background())
		c.flusherKick = make(chan struct{}, 1)
	}
	c.flusherOn = true
	c.flusherWG.Add(1)
	go c.runFlusher(c.flusherCtx, c.flusherKick)
}

// kickFlusher wakes the flusher now, if anything is queued: Wotan just
// answered, so the backlog has a chance.
func (c *Client) kickFlusher() {
	if c.FallbackQueueLen() == 0 {
		return
	}
	c.ensureFlusher()
	c.flusherMu.Lock()
	kick := c.flusherKick
	c.flusherMu.Unlock()
	if kick != nil {
		select {
		case kick <- struct{}{}:
		default:
		}
	}
}

// flusherRunning reports whether a flusher goroutine is live (tests).
func (c *Client) flusherRunning() bool {
	c.flusherMu.Lock()
	defer c.flusherMu.Unlock()
	return c.flusherOn
}

// runFlusher flushes every fallbackEvery and on each kick, and exits once the
// queue is empty or the client closes. The next enqueue starts a new one.
func (c *Client) runFlusher(ctx context.Context, kick <-chan struct{}) {
	defer c.flusherWG.Done()
	timer := time.NewTimer(c.fallbackEveryOr())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			c.flusherMu.Lock()
			c.flusherOn = false
			c.flusherMu.Unlock()
			return
		case <-kick:
		case <-timer.C:
		}
		fctx, cancel := context.WithTimeout(ctx, fallbackFlushTimeout)
		_, _ = c.FlushFallbackQueue(fctx)
		cancel()

		c.flusherMu.Lock()
		if c.FallbackQueueLen() == 0 || c.closed {
			c.flusherOn = false
			c.flusherMu.Unlock()
			return
		}
		c.flusherMu.Unlock()
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(c.fallbackEveryOr())
	}
}

// stopFlusher stops the flusher for good and waits for it to exit.
func (c *Client) stopFlusher() {
	c.flusherMu.Lock()
	c.closed = true
	if c.flusherCancel != nil {
		c.flusherCancel()
	}
	c.flusherMu.Unlock()
	c.flusherWG.Wait()
}
