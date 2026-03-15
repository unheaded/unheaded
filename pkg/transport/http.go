// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package transport

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	wotanClient "unheaded/pkg/wotan-client"
)

// httpConnection implements Connection using the Wotan HTTP client.
type httpConnection struct {
	client  *wotanClient.Client
	addr    string
	healthy atomic.Bool
	mu      sync.RWMutex
}

// newHTTPConnection creates an HTTP connection to Wotan.
func newHTTPConnection(_ context.Context, addr string) (*httpConnection, error) {
	client, err := wotanClient.NewClient(addr)
	if err != nil {
		return nil, err
	}

	hc := &httpConnection{
		client: client,
		addr:   addr,
	}
	hc.healthy.Store(true)
	return hc, nil
}

func (h *httpConnection) Type() Type { return HTTP }

func (h *httpConnection) Publish(ctx context.Context, topic string, data []byte) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.client == nil {
		return ErrNotConnected
	}
	err := h.client.Publish(ctx, topic, data)
	if err != nil {
		h.healthy.Store(false)
		return err
	}
	h.healthy.Store(true)
	return nil
}

func (h *httpConnection) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.client == nil {
		return nil, ErrNotConnected
	}
	msgCh, err := h.client.StreamMessages(ctx, topic)
	if err != nil {
		h.healthy.Store(false)
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

func (h *httpConnection) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.client == nil {
		return nil
	}
	h.healthy.Store(false)
	h.client.Close()
	h.client = nil
	return nil
}

func (h *httpConnection) Healthy() bool {
	return h.healthy.Load()
}
