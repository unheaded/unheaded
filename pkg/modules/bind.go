// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package modules

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// The bind rule of ADR-149 decision 6 (S-ZHEN-SWITCH D5 (a)): a module's
// listener is on loopback, or on a private address this host lists in
// UNHEADED_MODULE_BIND_ALLOW. A wildcard and a globally routable address are
// refused always, even when listed: a module never answers the public side of
// a host (ADR-147 decision 15 (c)), and on a host whose interfaces hold global
// IPv6 addresses a wildcard bind is a public bind. Link-local is refused too:
// it is reachable from anything on the segment and names no interface here.

// EnvBindAllow is the host's comma list of private prefixes a module may bind.
const EnvBindAllow = "UNHEADED_MODULE_BIND_ALLOW"

// maxBindAllow bounds the allowlist; a real one has one to three entries.
const maxBindAllow = 32

// isPrivate reports whether a is in a private range (RFC 1918, RFC 4193).
func isPrivate(a netip.Addr) bool { return a.Unmap().IsPrivate() }

// lastAddr is the highest address of p.
func lastAddr(p netip.Prefix) netip.Addr {
	a := p.Masked().Addr()
	b := a.AsSlice()
	bits := p.Bits()
	for i := range b {
		lo := i * 8
		switch {
		case bits <= lo:
			b[i] = 0xff
		case bits < lo+8:
			b[i] |= byte(0xff) >> (bits - lo) // bits-lo is in 1..7 here
		}
	}
	out, _ := netip.AddrFromSlice(b)
	return out
}

// ParseBindAllow parses a comma list of prefixes. Each prefix must lie wholly
// inside a private range and not be link-local; "" is an empty list.
func ParseBindAllow(s string) ([]netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	if len(parts) > maxBindAllow {
		return nil, fmt.Errorf("%s: %d entries, at most %d", EnvBindAllow, len(parts), maxBindAllow)
	}
	out := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		p, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a prefix (address/bits): %w", EnvBindAllow, part, err)
		}
		p = p.Masked()
		first, last := p.Addr(), lastAddr(p)
		if first.Unmap().IsLinkLocalUnicast() || !isPrivate(first) || !isPrivate(last) {
			return nil, fmt.Errorf("%s: %s is not private (only RFC 1918 and RFC 4193 prefixes, never link-local)", EnvBindAllow, p)
		}
		out = append(out, p)
	}
	return out, nil
}

// CheckBind reports why hostport is not a place a module may listen, or nil.
// hostport must hold an IP literal; allow comes from ParseBindAllow.
func CheckBind(hostport string, allow []netip.Prefix) error {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return fmt.Errorf("%q: want host:port: %w", hostport, err)
	}
	if host == "" {
		return fmt.Errorf("%s: wildcard address (every interface)", hostport)
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%s: the host must be an IP literal, not a name", hostport)
	}
	a = a.Unmap()
	switch {
	case a.IsUnspecified():
		return fmt.Errorf("%s: wildcard address (every interface)", hostport)
	case a.IsLoopback():
		return nil
	case a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast():
		return fmt.Errorf("%s: link-local address", hostport)
	case isPrivate(a):
		bare := a.WithZone("")
		for _, p := range allow {
			if p.Contains(bare) {
				return nil
			}
		}
		return fmt.Errorf("%s: private address not on this host's allowlist (%s)", hostport, EnvBindAllow)
	default:
		return fmt.Errorf("%s: globally routable address", hostport)
	}
}
