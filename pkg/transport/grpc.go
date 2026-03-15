// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package transport

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	wotanClient "unheaded/pkg/wotan-client"
)

// grpcConnection implements Connection using the Wotan gRPC client.
type grpcConnection struct {
	client  *wotanClient.TopicStreamClient
	addr    string
	healthy atomic.Bool
	mu      sync.RWMutex
}

// newGRPCConnection creates a gRPC connection to Wotan.
func newGRPCConnection(_ context.Context, addr string) (*grpcConnection, error) {
	client, err := wotanClient.NewTopicStreamClient(addr)
	if err != nil {
		return nil, err
	}

	gc := &grpcConnection{
		client: client,
		addr:   addr,
	}
	gc.healthy.Store(true)
	return gc, nil
}

func (g *grpcConnection) Type() Type { return GRPC }

func (g *grpcConnection) Publish(ctx context.Context, topic string, data []byte) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.client == nil {
		return ErrNotConnected
	}
	err := g.client.Publish(ctx, topic, data)
	if err != nil {
		g.healthy.Store(false)
		return err
	}
	g.healthy.Store(true)
	return nil
}

func (g *grpcConnection) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.client == nil {
		return nil, ErrNotConnected
	}
	msgCh, err := g.client.StreamMessages(ctx, topic)
	if err != nil {
		g.healthy.Store(false)
		return nil, err
	}
	// Convert *Message channel to []byte channel (JSON-encoded payload)
	out := make(chan []byte, 100)
	go func() {
		defer close(out)
		for msg := range msgCh {
			payload, _ := json.Marshal(msg)
			select {
			case out <- payload:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (g *grpcConnection) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.client == nil {
		return nil
	}
	g.healthy.Store(false)
	err := g.client.Close()
	g.client = nil
	return err
}

func (g *grpcConnection) Healthy() bool {
	return g.healthy.Load()
}
