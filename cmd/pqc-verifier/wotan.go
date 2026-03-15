// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"io"
	"sync"

	"unheaded/pkg/audit"
)

// WotanPublisher publishes PQC events to Wotan topics.
// In production this would use gRPC streaming; here we provide
// a pluggable interface for testability.
type WotanPublisher interface {
	Publish(topic string, data []byte) error
}

// WotanSubscriber subscribes to Wotan topics.
type WotanSubscriber interface {
	Subscribe(topic string, handler func([]byte)) error
}

// EventPublisher wraps a WotanPublisher with typed event publishing.
type EventPublisher struct {
	mu     sync.RWMutex
	pub    WotanPublisher
	events []*audit.PQCEvent // In-memory buffer for when Wotan is unavailable
}

// NewEventPublisher creates a new event publisher.
func NewEventPublisher(pub WotanPublisher) *EventPublisher {
	return &EventPublisher{
		pub: pub,
	}
}

// PublishVerified publishes a signature verified event.
func (ep *EventPublisher) PublishVerified(evt *audit.PQCEvent) error {
	return ep.publish(audit.WotanTopics.SigVerified, evt)
}

// PublishSovereign publishes a sovereign validation event.
func (ep *EventPublisher) PublishSovereign(evt *audit.PQCEvent) error {
	return ep.publish(audit.WotanTopics.Sovereign, evt)
}

// PublishAnomaly publishes an anomaly detection event.
func (ep *EventPublisher) PublishAnomaly(evt *audit.PQCEvent) error {
	return ep.publish(audit.WotanTopics.Anomaly, evt)
}

// publish serializes and publishes an event to a topic.
func (ep *EventPublisher) publish(topic string, evt *audit.PQCEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	ep.mu.Lock()
	ep.events = append(ep.events, evt)
	ep.mu.Unlock()

	if ep.pub != nil {
		return ep.pub.Publish(topic, data)
	}
	return nil
}

// EventCount returns the number of events published.
func (ep *EventPublisher) EventCount() int {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return len(ep.events)
}

// WotanEventHandler handles incoming Wotan events relevant to PQC verification.
type WotanEventHandler struct {
	mu      sync.RWMutex
	log     io.Writer
	onSigCreated    func(data []byte)
	onKeyEvent      func(data []byte)
	onPolicyChanged func(data []byte)
}

// NewWotanEventHandler creates a new event handler.
func NewWotanEventHandler(log io.Writer) *WotanEventHandler {
	return &WotanEventHandler{log: log}
}

// SetSigCreatedHandler sets the handler for pqc.sig.created events.
func (h *WotanEventHandler) SetSigCreatedHandler(fn func(data []byte)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onSigCreated = fn
}

// SetKeyEventHandler sets the handler for pqc.key.* events.
func (h *WotanEventHandler) SetKeyEventHandler(fn func(data []byte)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onKeyEvent = fn
}

// SetPolicyChangedHandler sets the handler for pqc.policy.changed events.
func (h *WotanEventHandler) SetPolicyChangedHandler(fn func(data []byte)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onPolicyChanged = fn
}

// SubscribeAll subscribes to all relevant Wotan topics.
func (h *WotanEventHandler) SubscribeAll(sub WotanSubscriber) error {
	topics := []struct {
		topic   string
		handler func(data []byte)
	}{
		{audit.WotanTopics.SigCreated, h.handleSigCreated},
		{audit.WotanTopics.KeyGenerated, h.handleKeyEvent},
		{audit.WotanTopics.KeyRotated, h.handleKeyEvent},
		{audit.WotanTopics.KeyRevoked, h.handleKeyEvent},
		{audit.WotanTopics.PolicyChanged, h.handlePolicyChanged},
	}

	for _, t := range topics {
		if err := sub.Subscribe(t.topic, t.handler); err != nil {
			return err
		}
	}
	return nil
}

func (h *WotanEventHandler) handleSigCreated(data []byte) {
	h.mu.RLock()
	fn := h.onSigCreated
	h.mu.RUnlock()
	if fn != nil {
		fn(data)
	}
}

func (h *WotanEventHandler) handleKeyEvent(data []byte) {
	h.mu.RLock()
	fn := h.onKeyEvent
	h.mu.RUnlock()
	if fn != nil {
		fn(data)
	}
}

func (h *WotanEventHandler) handlePolicyChanged(data []byte) {
	h.mu.RLock()
	fn := h.onPolicyChanged
	h.mu.RUnlock()
	if fn != nil {
		fn(data)
	}
}
