// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package wotanClient — reliability extensions for Wotan message delivery.
//
// PublishWithAck adds acknowledgment-based publishing with automatic retries
// and dead letter support. Failed messages are routed to dead_letter.<topic>
// for offline investigation.
package wotanClient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Retry constants for PublishWithAck.
const (
	ackMaxRetries     = 3
	ackInitialBackoff = 100 * time.Millisecond
	ackBackoffFactor  = 2
)

// Prometheus metrics for Wotan client reliability.
var (
	wotanBufferedMessages = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wotan_buffered_messages",
			Help: "Current number of messages in the fallback queue",
		},
	)

	wotanDeadLettersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wotan_dead_letters_total",
			Help: "Total number of messages sent to dead letter topics",
		},
		[]string{"topic"},
	)

	wotanPublishRetriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wotan_publish_retries_total",
			Help: "Total number of publish retry attempts",
		},
		[]string{"topic"},
	)

	wotanPublishAckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wotan_publish_ack_total",
			Help: "Total PublishWithAck calls by outcome",
		},
		[]string{"topic", "result"}, // result: "success", "dead_letter", "error"
	)
)

// DeadLetterMessage represents a message that exhausted all retry attempts.
// It is published to the dead_letter.<original_topic> topic for offline
// investigation and manual replay.
type DeadLetterMessage struct {
	OriginalTopic string    `json:"original_topic"`
	Payload       []byte    `json:"payload"`
	Error         string    `json:"error"`
	Attempts      int       `json:"attempts"`
	FirstAttempt  time.Time `json:"first_attempt"`
	LastAttempt   time.Time `json:"last_attempt"`
}

// ErrRetriesExhausted is returned when PublishWithAck exhausts all retry attempts.
var ErrRetriesExhausted = fmt.Errorf("publish retries exhausted")

// PublishWithAck publishes a message and confirms delivery via the server
// response. On failure it retries up to 3 times with exponential backoff
// (100ms, 200ms, 400ms). If all retries are exhausted, the message is
// published to the dead letter topic "dead_letter.<original_topic>".
//
// Returns nil on success, or an error if all retries and dead letter
// publication fail.
func (c *Client) PublishWithAck(ctx context.Context, topic string, data []byte) error {
	if topic == "" {
		return fmt.Errorf("topic cannot be empty")
	}
	if len(data) == 0 {
		return fmt.Errorf("payload cannot be empty")
	}

	c.mu.RLock()
	sub, ok := c.subscribers[topic]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("not subscribed to topic %s", topic)
	}
	if sub.Status != "approved" {
		return ErrSubscriptionPending
	}

	firstAttempt := time.Now()
	backoff := ackInitialBackoff
	var lastErr error

	for attempt := 1; attempt <= ackMaxRetries; attempt++ {
		networkErr, appErr := c.doPublish(ctx, topic, sub.SubscriberID, data)

		if networkErr == nil && appErr == nil {
			// Success on this attempt.
			wotanPublishAckTotal.WithLabelValues(topic, "success").Inc()
			return nil
		}

		// Application-level errors (403, 404, etc.) are not retryable.
		if appErr != nil {
			wotanPublishAckTotal.WithLabelValues(topic, "error").Inc()
			return appErr
		}

		// Network error — record retry and backoff.
		lastErr = networkErr
		if attempt < ackMaxRetries {
			wotanPublishRetriesTotal.WithLabelValues(topic).Inc()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= ackBackoffFactor
			}
		}
	}

	// All retries exhausted — send to dead letter topic.
	lastAttempt := time.Now()
	c.publishDeadLetter(ctx, topic, data, lastErr, ackMaxRetries, firstAttempt, lastAttempt)
	wotanPublishAckTotal.WithLabelValues(topic, "dead_letter").Inc()

	return fmt.Errorf("%w: %v", ErrRetriesExhausted, lastErr)
}

// publishDeadLetter sends a failed message to the dead letter topic.
// It uses best-effort delivery — if the dead letter publish itself fails,
// the message is buffered in the fallback queue.
func (c *Client) publishDeadLetter(ctx context.Context, originalTopic string, payload []byte, lastErr error, attempts int, firstAttempt, lastAttempt time.Time) {
	dlMsg := DeadLetterMessage{
		OriginalTopic: originalTopic,
		Payload:       payload,
		Error:         lastErr.Error(),
		Attempts:      attempts,
		FirstAttempt:  firstAttempt,
		LastAttempt:   lastAttempt,
	}

	dlPayload, err := json.Marshal(dlMsg)
	if err != nil {
		// Cannot marshal dead letter — buffer for fallback queue.
		c.enqueueFallback(DeadLetterTopic(originalTopic), payload)
		atomic.AddInt64(&c.deadLetterCount, 1)
		wotanDeadLettersTotal.WithLabelValues(originalTopic).Inc()
		return
	}

	dlTopic := DeadLetterTopic(originalTopic)

	// Check if we're subscribed to the dead letter topic. If not, just buffer it.
	c.mu.RLock()
	dlSub, dlOK := c.subscribers[dlTopic]
	c.mu.RUnlock()

	if dlOK && dlSub.Status == "approved" {
		networkErr, appErr := c.doPublish(ctx, dlTopic, dlSub.SubscriberID, dlPayload)
		if networkErr != nil {
			c.enqueueFallback(dlTopic, dlPayload)
		}
		if appErr != nil {
			c.enqueueFallback(dlTopic, dlPayload)
		}
	} else {
		// Not subscribed to dead letter topic — buffer for later delivery.
		c.enqueueFallback(dlTopic, dlPayload)
	}

	atomic.AddInt64(&c.deadLetterCount, 1)
	wotanDeadLettersTotal.WithLabelValues(originalTopic).Inc()
}

// DeadLetterTopic returns the dead letter topic name for a given topic.
func DeadLetterTopic(topic string) string {
	return "dead_letter." + topic
}

// DeadLetterCount returns the total number of messages sent to dead letter.
func (c *Client) DeadLetterCount() int64 {
	return atomic.LoadInt64(&c.deadLetterCount)
}

// updateFallbackMetrics updates the Prometheus gauge for buffered messages.
// Called internally after enqueue/flush operations.
func (c *Client) updateFallbackMetrics() {
	c.fallbackMu.Lock()
	n := len(c.fallbackQueue)
	c.fallbackMu.Unlock()
	wotanBufferedMessages.Set(float64(n))
}
