// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// selfFilter recognises the collector's own TCP connections. Every batch it
// publishes to Wotan is packets the tracer reports, which the next batch
// published, and so on: on east (2026-10-23) the whole packet stream was
// loopback traffic to Wotan's 18000/18001, the collector's among it. Its
// own sockets come from /proc/self: socket inodes in fd/, their local
// addresses in net/tcp and net/tcp6.
type selfFilter struct {
	procSelf string // "/proc/self"; tests point it elsewhere

	mu      sync.RWMutex
	own     map[netip.AddrPort]struct{}
	dropped atomic.Uint64
}

var selfDroppedTotal = auto.NewCounter(prom.CounterOpts{
	Name: "trace_collector_self_dropped_total",
	Help: "Packets and flows left out because an endpoint is one of the collector's own sockets",
})

func newSelfFilter(procSelf string) *selfFilter {
	return &selfFilter{procSelf: procSelf, own: map[netip.AddrPort]struct{}{}}
}

// refresh re-reads the collector's sockets.
func (f *selfFilter) refresh() error {
	inodes, err := f.socketInodes()
	if err != nil {
		return err
	}
	own := map[netip.AddrPort]struct{}{}
	for _, name := range []string{"tcp", "tcp6"} {
		if err := f.readTable(filepath.Join(f.procSelf, "net", name), inodes, own); err != nil {
			return err
		}
	}
	f.mu.Lock()
	f.own = own
	f.mu.Unlock()
	return nil
}

func (f *selfFilter) socketInodes() (map[string]struct{}, error) {
	dir := filepath.Join(f.procSelf, "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	inodes := map[string]struct{}{}
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // closed since ReadDir
		}
		if ino, ok := strings.CutPrefix(target, "socket:["); ok {
			inodes[strings.TrimSuffix(ino, "]")] = struct{}{}
		}
	}
	return inodes, nil
}

// readTable adds the local address of every row of a /proc/net/tcp{,6}
// table whose inode is one of ours.
func (f *selfFilter) readTable(path string, inodes map[string]struct{}, own map[netip.AddrPort]struct{}) error {
	file, err := os.Open(path) // #nosec G304 -- fixed /proc path
	if err != nil {
		if os.IsNotExist(err) { // no IPv6, say
			return nil
		}
		return err
	}
	defer func() { _ = file.Close() }()
	sc := bufio.NewScanner(file)
	sc.Scan() // header
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		if _, ok := inodes[fields[9]]; !ok {
			continue
		}
		ap, err := parseProcAddr(fields[1])
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		own[ap] = struct{}{}
	}
	return sc.Err()
}

// parseProcAddr reads /proc/net/tcp's "0100007F:4651": the address as
// 32-bit words in host (little-endian) order, the port in hex.
func parseProcAddr(s string) (netip.AddrPort, error) {
	hexAddr, hexPort, ok := strings.Cut(s, ":")
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("bad address %q", s)
	}
	raw, err := hex.DecodeString(hexAddr)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.AddrPort{}, fmt.Errorf("bad address %q", s)
	}
	for i := 0; i < len(raw); i += 4 {
		raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("bad port %q", s)
	}
	addr, _ := netip.AddrFromSlice(raw)
	return netip.AddrPortFrom(addr.Unmap(), uint16(port)), nil
}

// isOwn reports whether ip:port is one of the collector's own sockets.
func (f *selfFilter) isOwn(ip net.IP, port uint16) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, own := f.own[netip.AddrPortFrom(addr.Unmap(), port)]
	return own
}

// drop reports, and counts, a packet or flow with an endpoint of ours.
func (f *selfFilter) drop(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) bool {
	if f.isOwn(srcIP, srcPort) || f.isOwn(dstIP, dstPort) {
		f.dropped.Add(1)
		selfDroppedTotal.Inc()
		return true
	}
	return false
}

// run refreshes every interval until ctx ends; a reconnect gets a new
// local port, which the next refresh picks up.
func (f *selfFilter) run(ctx context.Context, every time.Duration, onErr func(error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := f.refresh(); err != nil && onErr != nil {
			onErr(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
