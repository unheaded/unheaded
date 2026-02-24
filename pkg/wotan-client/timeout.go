// Package wotanClient — per-message timeout enforcement.
//
// PublishWithTimeout wraps PublishWithAck with an explicit per-message
// deadline. If delivery (including all retries) does not complete within
// the timeout, the publish is cancelled and an error returned.
package wotanClient

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Default message timeout.
const DefaultMessageTimeout = 30 * time.Second

// Prometheus metrics for message timeouts.
var (
	wotanMessageTimeouts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wotan_message_timeouts_total",
			Help: "Total number of messages that exceeded their delivery timeout",
		},
		[]string{"topic"},
	)
)

// ErrMessageTimeout is returned when a message exceeds its delivery deadline.
var ErrMessageTimeout = fmt.Errorf("message delivery timeout exceeded")

// PublishWithTimeout publishes a message with an explicit per-message timeout.
// It wraps PublishWithAck with a context deadline. If delivery (including all
// retries) does not complete within the timeout, the context is cancelled and
// ErrMessageTimeout is returned.
func (c *Client) PublishWithTimeout(ctx context.Context, topic string, data []byte, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultMessageTimeout
	}

	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := c.PublishWithAck(tctx, topic, data)
	if err != nil && tctx.Err() == context.DeadlineExceeded {
		wotanMessageTimeouts.WithLabelValues(topic).Inc()
		return fmt.Errorf("%w: %s after %s", ErrMessageTimeout, topic, timeout)
	}
	return err
}
