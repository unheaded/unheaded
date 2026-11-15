// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	chatpb "unheaded/services/wotan/proto"
)

// ErrNoStreamTransport is StreamMessages on a client with no gRPC address
// that was not built WithHTTPPollStreams (ADR-147 Phase 6).
var ErrNoStreamTransport = errors.New("wotan client: no gRPC address; streaming needs one (WithHTTPPollStreams for a tool that must poll)")

// streamLoop runs one topic's stream for its whole life (ADR-147 Phase 6,
// Developer D1, D2, D5). It streams over gRPC from the last delivered seq;
// when gRPC fails past its reconnect budget it marks the client degraded
// (which starts the re-probe) and polls over HTTP from the same seq; when
// the re-probe promotes gRPC it stops polling and opens gRPC again from the
// seq the poller last delivered.
//
// One goroutine does both, so the hand-over needs no lock: the poller has
// stopped before the gRPC stream opens, and both move one cursor. Before
// this loop a stream that fell back to HTTP polled for good (68 GET/s on
// west, 2026-11-15), and Health said "gRPC" while it did.
func (c *Client) streamLoop(ctx context.Context, topic, displayName string, sc *safeChannel) {
	defer c.endStream(topic, sc)
	var cursor int64
	if collectOptions(c.opts).liveStreams {
		cursor = -1 // live start: see WithLiveStreams and pollUntil
	}
	var gsub *Subscriber
	for {
		if streamOver(ctx, sc) {
			return
		}
		if c.IsGRPCHealthy() {
			var cont bool
			gsub, cont = c.grpcPhase(ctx, topic, displayName, gsub, sc, &cursor)
			if !cont {
				return
			}
			continue
		}
		if !c.pollUntil(ctx, topic, sc, &cursor, c.IsGRPCHealthy) {
			return
		}
	}
}

// streamOver reports whether the caller ended the stream: ctx done or the
// channel closed (Close).
func streamOver(ctx context.Context, sc *safeChannel) bool {
	if ctx.Err() != nil {
		return true
	}
	select {
	case <-sc.done:
		return true
	default:
		return false
	}
}

// grpcPhase streams topic over gRPC from *cursor, reconnecting on transient
// errors within the gRPC client's budget (maxRetries per outage, the delay
// growing 1.5x to 10 s; a stream that delivered or stayed up healthyAge
// starts a fresh budget). It returns false when the caller ended the
// stream, true after degrading the client to HTTP (the caller polls).
func (c *Client) grpcPhase(ctx context.Context, topic, displayName string, gsub *Subscriber, sc *safeChannel, cursor *int64) (*Subscriber, bool) {
	gc, err := c.getOrCreateGRPCClient()
	if err == nil && gsub == nil {
		gsub, err = gc.Subscribe(ctx, topic, displayName)
	}
	if err != nil {
		c.degradeToHTTP()
		return gsub, true
	}
	c.setStreamTransport(topic, streamGRPC)

	retries, delay := 0, gc.retryDelay
	healthyAge := gc.healthyAge
	if healthyAge == 0 {
		healthyAge = 30 * time.Second
	}
	for {
		start := time.Now()
		delivered, err := gc.streamMessagesSingle(ctx, topic, gsub, sc, cursor)
		if streamOver(ctx, sc) {
			return gsub, false
		}
		if delivered > 0 || time.Since(start) >= healthyAge {
			retries, delay = 0, gc.retryDelay
		}
		if isTransientError(err) && retries < gc.maxRetries {
			retries++
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return gsub, false
			case <-sc.done:
				return gsub, false
			}
			delay = min(time.Duration(float64(delay)*1.5), 10*time.Second)
			continue
		}
		c.degradeToHTTP()
		return gsub, true
	}
}

// openTopicStream is the one call that opens a Wotan topic stream
// (policy_test.go TestOneStreamOpener): the gRPC client, the topic stream
// client, their streams and their one-shot reads all come through here.
func openTopicStream(ctx context.Context, cl chatpb.TopicStreamClient, req *chatpb.TopicStreamRequest) (grpc.ServerStreamingClient[chatpb.TopicEvent], error) {
	return cl.StreamTopics(ctx, req)
}
