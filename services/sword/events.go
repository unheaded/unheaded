// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sword

import (
	"context"
	"sync"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// WotanName is the subscriber name Sword joins Wotan's deploy.* topics as
// (configs/wotan.yaml auto_approve lists it).
const WotanName = "sword"

const joinTimeout = 10 * time.Second

// wotanAPI is what WotanPublisher needs from the Wotan client.
type wotanAPI interface {
	SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*wotanClient.Message)) error
	Publish(ctx context.Context, topic string, payload []byte) error
}

// WotanPublisher publishes deploy events on Wotan, joining each topic as
// "sword" before its first publish (publishing without joining is refused,
// the S17 "published without joining" bug).
type WotanPublisher struct {
	c      wotanAPI
	mu     sync.Mutex
	joined map[string]bool
}

// NewWotanPublisher wraps a Wotan client.
func NewWotanPublisher(c wotanAPI) *WotanPublisher {
	return &WotanPublisher{c: c, joined: map[string]bool{}}
}

// Publish joins topic once, then publishes. deploy.* are control topics:
// when Wotan is down the client returns the error instead of buffering.
func (p *WotanPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	p.mu.Lock()
	joined := p.joined[topic]
	p.mu.Unlock()
	if !joined {
		jctx, cancel := context.WithTimeout(ctx, joinTimeout)
		err := p.c.SubscribeWithRetry(jctx, topic, WotanName, nil)
		cancel()
		if err != nil {
			return err
		}
		p.mu.Lock()
		p.joined[topic] = true
		p.mu.Unlock()
	}
	return p.c.Publish(ctx, topic, payload)
}
