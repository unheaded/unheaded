// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/metrics"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// Stream health (S-REAL-STEEL D-12): whether each stream this client holds is
// alive, as opposed to how recently it carried a message. An idle topic is
// not a dead stream, so message age alone would cry wolf; a stream that
// Wotan ended, or polls that fail, is the failure that used to be invisible
// (a service's /health said 200 while it heard nothing).

// defaultStreamUpAfter is how long a new gRPC stream must stay open without
// an error before it counts as up when no message has arrived yet. A stream
// that delivers a message is up at once.
const defaultStreamUpAfter = 2 * time.Second

var wotanClientStreamUp = auto.NewGaugeVec(
	prom.GaugeOpts{
		Name: "unheaded_wotan_client_stream_up",
		Help: "1 while the Wotan client's stream for a topic is alive, 0 while it is down",
	},
	[]string{"topic"},
)

// streamState is one topic's stream liveness.
type streamState struct {
	up      atomic.Bool
	lastMsg atomic.Int64 // unix nanos of the last message delivered; 0 = none
}

// Health is the client's runtime view of Wotan for a service's /health
// detail. Degraded is not dead: a service keeps answering 200 and shows this.
type Health struct {
	Transport   string          `json:"transport"`    // "gRPC" or "HTTP"
	GRPCHealthy bool            `json:"grpc_healthy"` // last ProbeGRPC / stream outcome
	Streams     map[string]bool `json:"streams"`      // topic -> alive
	// LastMessageAge is how long ago each topic last delivered a message;
	// topics that have not delivered one are absent.
	LastMessageAge        map[string]time.Duration `json:"-"`
	LastMessageAgeSeconds map[string]float64       `json:"last_message_age_seconds"`
}

// streamFor returns topic's state, creating it.
func (c *Client) streamFor(topic string) *streamState {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	if c.streams == nil {
		c.streams = make(map[string]*streamState)
	}
	st, ok := c.streams[topic]
	if !ok {
		st = &streamState{}
		c.streams[topic] = st
	}
	return st
}

// setStreamUp records whether topic's stream is alive.
// isClosed reports whether Close has been called.
func (c *Client) isClosed() bool {
	c.flusherMu.Lock()
	defer c.flusherMu.Unlock()
	return c.closed
}

func (c *Client) setStreamUp(topic string, up bool) {
	if c.isClosed() {
		return // a stream torn down by Close is not an outage either
	}
	c.streamFor(topic).up.Store(up)
	v := 0.0
	if up {
		v = 1
	}
	wotanClientStreamUp.WithLabelValues(topic).Set(v)
}

// streamMessage records a message delivered on topic; a delivering stream is up.
func (c *Client) streamMessage(topic string) {
	st := c.streamFor(topic)
	st.lastMsg.Store(time.Now().UnixNano())
	if !st.up.Swap(true) {
		wotanClientStreamUp.WithLabelValues(topic).Set(1)
	}
}

// forgetStream drops topic's state when the caller ended the stream
// (cancelled its context, or Close): that is not an outage.
func (c *Client) forgetStream(topic string) {
	c.streamsMu.Lock()
	delete(c.streams, topic)
	c.streamsMu.Unlock()
	wotanClientStreamUp.DeletePartialMatch(metrics.Labels{"topic": topic})
}

// Health returns the transport, gRPC health and every live stream's state.
func (c *Client) Health() Health {
	h := Health{
		Transport:             c.Transport().String(),
		GRPCHealthy:           c.IsGRPCHealthy(),
		Streams:               map[string]bool{},
		LastMessageAge:        map[string]time.Duration{},
		LastMessageAgeSeconds: map[string]float64{},
	}
	now := time.Now()
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	for topic, st := range c.streams {
		h.Streams[topic] = st.up.Load()
		if t := st.lastMsg.Load(); t > 0 {
			age := now.Sub(time.Unix(0, t))
			h.LastMessageAge[topic] = age
			h.LastMessageAgeSeconds[topic] = age.Seconds()
		}
	}
	return h
}

func (c *Client) streamUpAfterOr() time.Duration {
	if c.streamUpAfter > 0 {
		return c.streamUpAfter
	}
	return defaultStreamUpAfter
}

// attemptState reports one gRPC stream attempt's liveness in order: once the
// attempt has failed, a late "up" from its grace timer is ignored.
type attemptState struct {
	mu     sync.Mutex
	failed bool
	report func(up bool)
}

func (a *attemptState) up() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.failed {
		a.report(true)
	}
}

// end closes the attempt without a report: the caller ended the stream.
func (a *attemptState) end() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failed = true
}

func (a *attemptState) down() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failed = true
	a.report(false)
}
