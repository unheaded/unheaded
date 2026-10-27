// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"log"
	"time"
)

// defaultReprobeInterval is how often a degraded client asks Wotan's
// grpc.health.v1 whether gRPC is back.
const defaultReprobeInterval = 30 * time.Second

// reprobeCloseWait caps how long Close waits for the loop to exit; one
// probe is bounded by grpcProbeTimeout, so the loop normally exits at once.
const reprobeCloseWait = time.Second

func (c *Client) reprobeEveryOr() time.Duration {
	if c.reprobeEvery > 0 {
		return c.reprobeEvery
	}
	return defaultReprobeInterval
}

// ensureReprobe starts the re-probe loop unless one is running, the client
// is closed, or it has no gRPC address. Callers mark the client unhealthy
// before calling it, so a loop that is just deciding to exit sees that and
// keeps going (it decides under reprobeMu).
func (c *Client) ensureReprobe() {
	if c.grpcAddr == "" {
		return
	}
	c.reprobeMu.Lock()
	defer c.reprobeMu.Unlock()
	if c.reprobeOn || c.reprobeClosed {
		return
	}
	if c.reprobeStop == nil {
		c.reprobeStop = make(chan struct{})
	}
	c.reprobeOn = true
	done := make(chan struct{})
	c.reprobeDone = done
	go c.reprobeLoop(c.reprobeStop, done, c.reprobeEveryOr())
}

// reprobeLoop probes gRPC every interval while the client is degraded and
// exits once it is healthy again or the client closes. Streams already
// polling over HTTP keep polling; streams opened after the promotion use
// gRPC.
//
// Before this loop the client probed once, at construction: a service that
// started before Wotan's gRPC port answered stayed on HTTP polling for the
// life of the process.
func (c *Client) reprobeLoop(stop <-chan struct{}, done chan struct{}, every time.Duration) {
	defer close(done)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			c.reprobeExit()
			return
		case <-t.C:
		}
		if c.isClosed() { // Close has begun: do not recreate the gRPC client
			c.reprobeExit()
			return
		}
		// grpc-go backs off reconnects up to 2 minutes after repeated
		// failures; without a reset a Wotan that came up would still look
		// down to every probe inside that window.
		c.resetGRPCBackoff()
		if err := c.ProbeGRPC(); err != nil {
			continue
		}
		c.reprobeMu.Lock()
		if !c.IsGRPCHealthy() { // degraded again since the probe
			c.reprobeMu.Unlock()
			continue
		}
		c.reprobeOn = false
		c.reprobeMu.Unlock()
		log.Printf("[INFO] wotan_client: re-probe promoted gRPC back to primary (addr=%s)", c.grpcAddr)
		return
	}
}

func (c *Client) reprobeExit() {
	c.reprobeMu.Lock()
	c.reprobeOn = false
	c.reprobeMu.Unlock()
}

// resetGRPCBackoff makes the next probe try to connect now.
func (c *Client) resetGRPCBackoff() {
	c.grpcMu.RLock()
	defer c.grpcMu.RUnlock()
	if c.grpcClient != nil && c.grpcClient.conn != nil {
		c.grpcClient.conn.ResetConnectBackoff()
	}
}

// stopReprobe ends the loop for good and waits up to reprobeCloseWait for
// it to exit. Safe to call more than once.
func (c *Client) stopReprobe() {
	c.reprobeMu.Lock()
	if c.reprobeClosed {
		c.reprobeMu.Unlock()
		return
	}
	c.reprobeClosed = true
	if c.reprobeStop != nil {
		close(c.reprobeStop)
	}
	done := c.reprobeDone
	c.reprobeMu.Unlock()
	if done == nil {
		return
	}
	t := time.NewTimer(reprobeCloseWait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	}
}
