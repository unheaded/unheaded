// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package wotanClient — message ordering for per-destination FIFO delivery.
//
// OrderedPublisher ensures that messages sent to the same destination
// are delivered in order. Each destination gets its own queue; messages
// are published sequentially per queue so that PublishWithAck for
// message A completes (or fails) before message B begins.
package wotanClient

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for ordered publishing.
var (
	wotanOrderedQueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wotan_ordered_queue_depth",
			Help: "Current number of pending messages per destination queue",
		},
		[]string{"destination"},
	)
	wotanOrderedDeliveredTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wotan_ordered_delivered_total",
			Help: "Total messages delivered via ordered publishing",
		},
		[]string{"destination", "result"}, // result: "success", "error"
	)
)

// orderedMessage is a message waiting in a per-destination queue.
type orderedMessage struct {
	topic   string
	payload []byte
	result  chan error
}

// destinationQueue is a per-destination FIFO queue that processes
// messages sequentially through PublishWithAck.
type destinationQueue struct {
	destination string
	ch          chan orderedMessage
	done        chan struct{}
}

// OrderedPublisher wraps a Client and provides per-destination FIFO ordering.
// Messages to the same destination are delivered sequentially: message A
// completes (success or DLQ) before message B starts.
type OrderedPublisher struct {
	client *Client
	queues map[string]*destinationQueue
	mu     sync.Mutex
	closed bool
}

// NewOrderedPublisher creates an OrderedPublisher backed by the given client.
func NewOrderedPublisher(client *Client) *OrderedPublisher {
	return &OrderedPublisher{
		client: client,
		queues: make(map[string]*destinationQueue),
	}
}

// Publish enqueues a message for ordered delivery to the given destination.
// The destination is typically the topic name. Messages to the same destination
// are delivered in FIFO order. The call blocks until the message is published
// (or fails).
func (op *OrderedPublisher) Publish(ctx context.Context, destination, topic string, payload []byte) error {
	q := op.getOrCreateQueue(destination)

	msg := orderedMessage{
		topic:   topic,
		payload: payload,
		result:  make(chan error, 1),
	}

	// Enqueue and update depth metric.
	select {
	case q.ch <- msg:
		wotanOrderedQueueDepth.WithLabelValues(destination).Inc()
	case <-ctx.Done():
		return ctx.Err()
	case <-q.done:
		return ErrNotConnected
	}

	// Wait for delivery result.
	select {
	case err := <-msg.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close shuts down all destination queues. In-flight messages are drained.
func (op *OrderedPublisher) Close() {
	op.mu.Lock()
	defer op.mu.Unlock()

	if op.closed {
		return
	}
	op.closed = true

	for _, q := range op.queues {
		close(q.done)
	}
}

// QueueDepth returns the number of pending messages for a destination.
func (op *OrderedPublisher) QueueDepth(destination string) int {
	op.mu.Lock()
	q, ok := op.queues[destination]
	op.mu.Unlock()
	if !ok {
		return 0
	}
	return len(q.ch)
}

// getOrCreateQueue returns the queue for a destination, creating it if needed.
func (op *OrderedPublisher) getOrCreateQueue(destination string) *destinationQueue {
	op.mu.Lock()
	defer op.mu.Unlock()

	if q, ok := op.queues[destination]; ok {
		return q
	}

	q := &destinationQueue{
		destination: destination,
		ch:          make(chan orderedMessage, 1024),
		done:        make(chan struct{}),
	}
	op.queues[destination] = q

	go op.processQueue(q)
	return q
}

// processQueue drains a destination queue sequentially.
// Each message is published via PublishWithAck before the next starts.
func (op *OrderedPublisher) processQueue(q *destinationQueue) {
	for {
		select {
		case <-q.done:
			// Drain remaining messages with error.
			for {
				select {
				case msg := <-q.ch:
					msg.result <- ErrNotConnected
					wotanOrderedQueueDepth.WithLabelValues(q.destination).Dec()
				default:
					return
				}
			}
		case msg := <-q.ch:
			wotanOrderedQueueDepth.WithLabelValues(q.destination).Dec()

			err := op.client.PublishWithAck(context.Background(), msg.topic, msg.payload)
			msg.result <- err

			if err != nil {
				wotanOrderedDeliveredTotal.WithLabelValues(q.destination, "error").Inc()
			} else {
				wotanOrderedDeliveredTotal.WithLabelValues(q.destination, "success").Inc()
			}
		}
	}
}
