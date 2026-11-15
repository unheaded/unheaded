// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"fmt"
)

// PublishPreferGRPC publishes payload on topic over gRPC while the client's
// gRPC side is healthy (one PublishTopic call; Wotan admits the publisher
// itself, so no HTTP join is made), and over HTTP otherwise, joining topic
// as displayName first. A transient gRPC failure degrades the client (the
// re-probe brings gRPC back) and the message goes over HTTP; any other
// gRPC refusal is returned as is.
//
// It is for a high-rate publisher such as the trace collector, which kept
// a TopicStreamClient beside its Client only to publish over gRPC
// (ADR-147 Phase 7: one Wotan client per process).
func (c *Client) PublishPreferGRPC(ctx context.Context, topic, displayName string, payload []byte) error {
	if c.grpcAddr != "" && c.IsGRPCHealthy() {
		err := c.publishGRPC(ctx, topic, displayName, payload)
		if err == nil {
			return nil
		}
		if !isTransientError(err) {
			return err
		}
		c.degradeToHTTP()
	}
	c.publishJoinMu.Lock()
	_, joined := c.GetSubscriber(topic)
	if !joined {
		if _, err := c.Subscribe(ctx, topic, displayName); err != nil {
			c.publishJoinMu.Unlock()
			return fmt.Errorf("join %s: %w", topic, err)
		}
	}
	c.publishJoinMu.Unlock()
	return c.Publish(ctx, topic, payload)
}

func (c *Client) publishGRPC(ctx context.Context, topic, displayName string, payload []byte) error {
	gc, err := c.getOrCreateGRPCClient()
	if err != nil {
		return err
	}
	if _, ok := gc.GetSubscriber(topic); !ok {
		if _, err := gc.Subscribe(ctx, topic, displayName); err != nil {
			return err
		}
	}
	return gc.Publish(ctx, topic, payload)
}
