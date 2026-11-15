// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"unheaded/pkg/discovery"
	"unheaded/pkg/health"
	"unheaded/pkg/healthreport"
)

// discoverConfig is the config file's discover block (ADR-147 Phase 9,
// D3 = A): Akira reads system.discovery and checks every admitted
// announcement it has no target for.
type discoverConfig struct {
	Discover struct {
		Subnets []string `yaml:"subnets"`
	} `yaml:"discover"`
}

// discoverSubnets is --discover-subnets (comma-separated) if set, else the
// config's discover.subnets; nil means discovery is off. A prefix broader
// than /16 (IPv4) or /48 (IPv6) is refused.
func discoverSubnets(configPath, flagVal string) ([]netip.Prefix, error) {
	var raw []string
	switch {
	case strings.TrimSpace(flagVal) != "":
		raw = strings.Split(flagVal, ",")
	case configPath != "":
		data, err := os.ReadFile(configPath) // #nosec G304 -- the operator's --config path
		if err != nil {
			return nil, err
		}
		var cfg discoverConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", configPath, err)
		}
		raw = cfg.Discover.Subnets
	}
	var out []netip.Prefix
	for _, s := range raw {
		p, err := netip.ParsePrefix(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("discover subnet %q: %w", s, err)
		}
		if (p.Addr().Is6() && p.Bits() < 48) || (p.Addr().Is4() && p.Bits() < 16) {
			return nil, fmt.Errorf("discover subnet %s is broader than /16 (IPv6 /48)", p)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// startDiscovery reads system.discovery on c and makes each sweep check the
// admitted announcements (https to the checked IP, the service's name as
// the TLS server name). Off without subnets or a client.
func startDiscovery(ctx context.Context, c discovery.Streamer, subnets []netip.Prefix, akira *health.Akira) {
	if len(subnets) == 0 || c == nil {
		return
	}
	reg, err := discovery.Watch(ctx, c, "akira", discovery.Admit{AllowSubnets: subnets})
	if err != nil {
		log.Warn().Err(err).Msg("service discovery: not reading")
		return
	}
	mem := newDiscoverMemory(time.Now, func(format string, args ...any) { log.Warn().Msgf(format, args...) })
	akira.SetDiscovered(func() []health.ServiceTarget { return mem.targets(reg.ResolveAll()) })
	log.Info().Int("subnets", len(subnets)).Msg("service discovery: checking admitted announcements too")
}

// discoverKeep is how long a discovered service that stopped announcing
// (and was reaped from the registry after discovery.DefaultTTL) is still
// checked at its last address: a dead one fails its checks and goes red
// instead of vanishing from the sweep. Then it is dropped, with a log line.
const discoverKeep = 10 * discovery.DefaultTTL

// discoverMemory remembers each discovered target's last announcement, at
// most discovery.MaxDiscovered (the oldest goes first).
type discoverMemory struct {
	now  func() time.Time
	logf func(format string, args ...any)

	mu   sync.Mutex
	seen map[string]rememberedTarget // by ballot name
}

type rememberedTarget struct {
	t    health.ServiceTarget
	last time.Time
}

func newDiscoverMemory(now func() time.Time, logf func(string, ...any)) *discoverMemory {
	return &discoverMemory{now: now, logf: logf, seen: map[string]rememberedTarget{}}
}

// targets is the sweep's discovered list: the live announcements, plus
// the ones reaped less than discoverKeep ago at their last address.
func (m *discoverMemory) targets(live []discovery.ServiceEntry) []health.ServiceTarget {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range live {
		// The ballot name, so a configured target ("dashboard") and its
		// announcement ("dashboard-backend") are one entry; the announced
		// name is the TLS server name.
		name := healthreport.BallotNameOf(e.Name)
		m.seen[name] = rememberedTarget{t: health.ServiceTarget{Name: name, Host: e.Address, Port: e.Port, ServerName: e.Name}, last: now}
	}
	for name, r := range m.seen {
		if gone := now.Sub(r.last); gone >= discoverKeep {
			delete(m.seen, name)
			m.logf("service discovery: %s (%s at %s) has not announced for %s; no longer checked", name, r.t.ServerName, r.t.Host, gone.Truncate(time.Second))
		}
	}
	if over := len(m.seen) - discovery.MaxDiscovered; over > 0 {
		names := make([]string, 0, len(m.seen))
		for n := range m.seen {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool { return m.seen[names[i]].last.Before(m.seen[names[j]].last) })
		for _, n := range names[:over] {
			m.logf("service discovery: %s (%s) dropped: more than %d discovered services", n, m.seen[n].t.ServerName, discovery.MaxDiscovered)
			delete(m.seen, n)
		}
	}
	out := make([]health.ServiceTarget, 0, len(m.seen))
	for _, r := range m.seen {
		out = append(out, r.t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
