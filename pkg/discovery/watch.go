// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/netip"
	"time"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	wotanClient "unheaded/pkg/wotan-client"
)

// MaxDiscovered bounds a reader's registry when Admit.MaxEntries is 0.
const MaxDiscovered = 256

// Streamer is the part of the process's Wotan client a reader needs.
type Streamer interface {
	Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error)
	StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error)
}

// Admit is what a reader lets into its registry (ADR-147 Phase 9, D3 = A):
// an announcement whose name is the sender Wotan verified, from an address
// inside AllowSubnets (a hostname is resolved and the checked IP kept), at
// most MaxEntries names.
type Admit struct {
	AllowSubnets []netip.Prefix
	MaxEntries   int                                                          // 0: MaxDiscovered
	Resolve      func(ctx context.Context, host string) ([]netip.Addr, error) // nil: the system resolver
}

var discoveryRefused = auto.NewCounterVec(prom.CounterOpts{
	Name: "unheaded_discovery_refused_total",
	Help: "system.discovery announcements a reader refused, by reason (sender, address, full, decode)",
}, []string{"reason"})

// Watch reads system.discovery on c, the process's one Wotan client, as
// name, and keeps the announcements a admits in the returned Registry
// (reaped DefaultTTL after the last announcement). One stream, joined in
// the background until ctx ends. No subnets is an error: discovery stays
// announce-only.
func Watch(ctx context.Context, c Streamer, name string, a Admit) (*Registry, error) {
	return watch(ctx, c, name, a, DefaultTTL, ReapInterval)
}

func watch(ctx context.Context, c Streamer, name string, a Admit, ttl, reapEvery time.Duration) (*Registry, error) {
	if len(a.AllowSubnets) == 0 {
		return nil, errors.New("discovery: no subnets to admit announcements from")
	}
	if c == nil {
		return nil, errors.New("discovery: no Wotan client")
	}
	if a.MaxEntries <= 0 {
		a.MaxEntries = MaxDiscovered
	}
	if a.Resolve == nil {
		a.Resolve = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	reg := New(nil)
	reg.ttl = ttl
	go func() { // #nosec G118 -- runs until ctx ends
		ch := joinAndStream(ctx, c, name)
		if ch == nil {
			return
		}
		tick := time.NewTicker(reapEvery)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				reg.reapStale()
			case m, ok := <-ch:
				if !ok {
					return
				}
				if m != nil && !m.Deleted && !replayed(m.CreatedAt, time.Now()) {
					admit(ctx, reg, a, m)
				}
			}
		}
	}()
	return reg, nil
}

// joinAndStream joins system.discovery as name (1 s doubling to 1 min)
// and opens the one stream; nil when ctx ends first.
func joinAndStream(ctx context.Context, c Streamer, name string) <-chan *wotanClient.Message {
	backoff, failures := time.Second, 0
	for {
		sub, err := c.Subscribe(ctx, DiscoveryTopic, name)
		if err == nil && sub.Status != "approved" {
			err = errors.New("pending (not on topics.auto_approve?)")
		}
		var ch <-chan *wotanClient.Message
		if err == nil {
			ch, err = c.StreamMessages(ctx, DiscoveryTopic)
		}
		if err == nil {
			return ch
		}
		if failures == 0 {
			log.Printf("[discovery] %s: not reading %s yet, retrying: %v", name, DiscoveryTopic, err)
		}
		failures++
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > time.Minute {
			backoff = time.Minute
		}
	}
}

// admit applies the three rules and keeps an admitted entry.
func admit(ctx context.Context, reg *Registry, a Admit, m *wotanClient.Message) {
	var e ServiceEntry
	if err := json.Unmarshal([]byte(m.Payload), &e); err != nil || e.Name == "" || e.Port <= 0 || e.Port > 65535 {
		discoveryRefused.WithLabelValues("decode").Inc()
		return
	}
	if m.VerifiedSender == "" || e.Name != m.VerifiedSender {
		discoveryRefused.WithLabelValues("sender").Inc()
		return
	}
	ip, ok := checkedAddr(ctx, a, e.Address)
	if !ok {
		discoveryRefused.WithLabelValues("address").Inc()
		return
	}
	e.Address = ip.String()
	e.LastSeen = time.Now()
	reg.mu.Lock()
	if _, known := reg.services[e.Name]; !known && len(reg.services) >= a.MaxEntries {
		reg.mu.Unlock()
		discoveryRefused.WithLabelValues("full").Inc()
		return
	}
	reg.services[e.Name] = &e
	reg.mu.Unlock()
	reg.notifyWatchers(e)
}

// checkedAddr is addr's IP (or its first resolved IP) when it lies inside
// one of the subnets.
func checkedAddr(ctx context.Context, a Admit, addr string) (netip.Addr, bool) {
	if addr == "" || len(addr) > 253 {
		return netip.Addr{}, false
	}
	inside := func(ip netip.Addr) bool {
		ip = ip.Unmap()
		for _, p := range a.AllowSubnets {
			if p.Contains(ip) {
				return true
			}
		}
		return false
	}
	if ip, err := netip.ParseAddr(addr); err == nil {
		return ip.Unmap(), inside(ip)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ips, err := a.Resolve(rctx, addr)
	if err != nil {
		return netip.Addr{}, false
	}
	for _, ip := range ips {
		if inside(ip) {
			return ip.Unmap(), true
		}
	}
	return netip.Addr{}, false
}
