// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package probenet confines health and steady-state probes to the LXD bridge
// subnet (S-CLEAN-SLATE M-039).
//
// Cuirass and Yaldabaoth probe http://<instance address>:<port><path>, where
// the address is whatever the instance reports through LXD. A container
// chooses its own interface addresses, so without a subnet check one can
// report any reachable address (the P2P peer 192.168.13.1, a host service on
// another interface) and have a privileged daemon send requests there. Only
// addresses inside the configured bridge subnets are probed.
package probenet

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// DefaultCIDR is lxdbr0 as scripts/setup-host.sh creates it. A host whose
// bridge differs (west's lxdbr0 is 10.10.20.1/24) configures its own list.
const DefaultCIDR = "10.10.10.0/24"

// Narrowest-allowed bounds: a probe subnet is a bridge, not a network.
const (
	minBitsV4 = 16
	minBitsV6 = 48
)

// ParseCSV parses a comma-separated CIDR list (an env var); empty is the
// default.
func ParseCSV(s string) ([]netip.Prefix, error) {
	if strings.TrimSpace(s) == "" {
		return Parse([]string{DefaultCIDR})
	}
	return Parse(strings.Split(s, ","))
}

// Parse checks a CIDR list. An empty list, a prefix broader than /16 (IPv4)
// or /48 (IPv6), or one in loopback, link-local, multicast or unspecified
// space is an error: the list exists to narrow, never to say "everything".
func Parse(list []string) ([]netip.Prefix, error) {
	if len(list) == 0 {
		return nil, errors.New("probe subnets: empty list")
	}
	out := make([]netip.Prefix, 0, len(list))
	for _, raw := range list {
		s := strings.TrimSpace(raw)
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("probe subnets: %q is not a CIDR prefix", s)
		}
		p = p.Masked()
		a := p.Addr()
		minBits := minBitsV4
		if a.Is6() {
			minBits = minBitsV6
		}
		switch {
		case p.Bits() < minBits:
			return nil, fmt.Errorf("probe subnets: %s is broader than /%d", p, minBits)
		case a.IsLoopback(), a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast(),
			a.IsMulticast(), a.IsInterfaceLocalMulticast(), a.IsUnspecified():
			return nil, fmt.Errorf("probe subnets: %s is not an instance network", p)
		}
		out = append(out, p)
	}
	return out, nil
}

// Allowed returns the probe address for ip when it lies inside one of
// subnets (an IPv4-mapped IPv6 address counts as its IPv4 address), or an
// error saying why it is never probed. Zoned addresses are refused.
func Allowed(subnets []netip.Prefix, ip string) (string, error) {
	if len(subnets) == 0 {
		return "", errors.New("no probe subnets configured; not probing")
	}
	a, err := netip.ParseAddr(ip)
	if err != nil || a.Zone() != "" {
		return "", fmt.Errorf("no usable address (%q)", ip)
	}
	a = a.Unmap()
	for _, p := range subnets {
		if p.Contains(a) {
			return a.String(), nil
		}
	}
	return "", fmt.Errorf("address %s is outside the probe subnets %v; not probing it", a, subnets)
}
