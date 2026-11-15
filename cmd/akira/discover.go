// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"

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
	akira.SetDiscovered(func() []health.ServiceTarget {
		entries := reg.ResolveAll()
		out := make([]health.ServiceTarget, 0, len(entries))
		for _, e := range entries {
			// The ballot name, so a configured target ("dashboard") and its
			// announcement ("dashboard-backend") are one entry; the
			// announced name is the TLS server name.
			out = append(out, health.ServiceTarget{Name: healthreport.BallotNameOf(e.Name), Host: e.Address, Port: e.Port, ServerName: e.Name})
		}
		return out
	})
	log.Info().Int("subnets", len(subnets)).Msg("service discovery: checking admitted announcements too")
}
