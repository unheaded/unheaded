// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestParseProcAddr(t *testing.T) {
	for in, want := range map[string]string{
		"0100007F:4651":                         "127.0.0.1:18001",
		"0201A8C0:01BB":                         "192.168.1.2:443",
		"00000000000000000000000001000000:0050": "[::1]:80",
		"0000000000000000FFFF00000100007F:4651": "127.0.0.1:18001", // v4-mapped
	} {
		got, err := parseProcAddr(in)
		if err != nil || got.String() != want {
			t.Errorf("parseProcAddr(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0100007F", "zz00007F:4651", "0100007F:zzzz", "01:4651"} {
		if _, err := parseProcAddr(bad); err == nil {
			t.Errorf("parseProcAddr(%q) accepted", bad)
		}
	}
}

// A fake /proc/self: two sockets of ours, one row that is someone else's.
func TestSelfFilter_FakeProc(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "fd"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "net"), 0o700); err != nil {
		t.Fatal(err)
	}
	for fd, target := range map[string]string{"3": "socket:[111]", "4": "socket:[222]", "5": "/dev/null"} {
		if err := os.Symlink(target, filepath.Join(dir, "fd", fd)); err != nil {
			t.Fatal(err)
		}
	}
	tcp := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:DD3C 0100007F:4651 01 00000000:00000000 00:00000000 00000000  1000        0 111 1 0 20 4 30 10 -1
   1: 0100007F:4651 0100007F:DD3C 01 00000000:00000000 00:00000000 00000000  1000        0 999 1 0 20 4 30 10 -1
`
	tcp6 := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:9C40 00000000000000000000000001000000:4650 01 00000000:00000000 00:00000000 00000000  1000        0 222 1 0 20 4 30 10 -1
`
	if err := os.WriteFile(filepath.Join(dir, "net", "tcp"), []byte(tcp), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "net", "tcp6"), []byte(tcp6), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newSelfFilter(dir)
	if err := f.refresh(); err != nil {
		t.Fatal(err)
	}
	lo := net.ParseIP("127.0.0.1")
	if !f.drop(lo, 0xDD3C, lo, 18001) { // our client socket to Wotan
		t.Error("our own connection was not dropped")
	}
	if !f.isOwn(net.ParseIP("::1"), 0x9C40) {
		t.Error("our IPv6 socket not recognised")
	}
	if f.drop(lo, 50000, lo, 18001) { // someone else's connection to the same Wotan
		t.Error("another process's connection was dropped")
	}
	if f.isOwn(lo, 18001) {
		t.Error("Wotan's listening side (inode 999, not ours) counted as ours")
	}
	if f.dropped.Load() != 1 {
		t.Errorf("dropped = %d, want 1", f.dropped.Load())
	}
}

// Against the real /proc/self: a connection this test opens is ours.
func TestSelfFilter_RealProc(t *testing.T) {
	if _, err := os.Stat("/proc/self/net/tcp"); err != nil {
		t.Skip("no /proc/self/net/tcp")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			defer c.Close()
			buf := make([]byte, 1)
			_, _ = c.Read(buf)
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	f := newSelfFilter("/proc/self")
	if err := f.refresh(); err != nil {
		t.Fatal(err)
	}
	local := netip.MustParseAddrPort(conn.LocalAddr().String())
	if !f.isOwn(net.IP(local.Addr().AsSlice()), local.Port()) {
		t.Errorf("own connection %s not found in /proc/self", local)
	}
	if f.isOwn(net.ParseIP("127.0.0.1"), 1) {
		t.Error("port 1 counted as ours")
	}
}
