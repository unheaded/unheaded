// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/rs/zerolog"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestActivationFDsUnsetMeansDirectBind(t *testing.T) {
	fds, err := activationFDs(env(nil), 42)
	if err != nil || fds != nil {
		t.Errorf("no LISTEN_* at all: %v, %v; want direct bind", fds, err)
	}
}

func TestActivationFDs(t *testing.T) {
	fds, err := activationFDs(env(map[string]string{
		"LISTEN_PID":     "42",
		"LISTEN_FDS":     "2",
		"LISTEN_FDNAMES": "syslog-udp:syslog-tcp",
	}), 42)
	if err != nil {
		t.Fatal(err)
	}
	want := []inheritedFD{{fd: 3, name: "syslog-udp"}, {fd: 4, name: "syslog-tcp"}}
	if len(fds) != len(want) {
		t.Fatalf("got %v, want %v", fds, want)
	}
	for i := range want {
		if fds[i] != want[i] {
			t.Errorf("fd %d = %+v, want %+v", i, fds[i], want[i])
		}
	}
}

// sd_listen_fds(3): with no LISTEN_FDNAMES every fd is "unknown".
func TestActivationFDsWithoutNames(t *testing.T) {
	fds, err := activationFDs(env(map[string]string{"LISTEN_PID": "7", "LISTEN_FDS": "1"}), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(fds) != 1 || fds[0] != (inheritedFD{fd: 3, name: "unknown"}) {
		t.Errorf("got %+v", fds)
	}
}

func TestActivationFDsRefuses(t *testing.T) {
	for name, e := range map[string]map[string]string{
		// The fds were passed to another process (a parent that started us
		// without clearing them): never take them.
		"another pid":      {"LISTEN_PID": "41", "LISTEN_FDS": "1"},
		"pid not a number": {"LISTEN_PID": "x", "LISTEN_FDS": "1"},
		"pid with sign":    {"LISTEN_PID": "+42", "LISTEN_FDS": "1"},
		"fds without pid":  {"LISTEN_FDS": "1"},
		"pid without fds":  {"LISTEN_PID": "42"},
		"names alone":      {"LISTEN_FDNAMES": "syslog"},
		"zero fds":         {"LISTEN_PID": "42", "LISTEN_FDS": "0"},
		"negative fds":     {"LISTEN_PID": "42", "LISTEN_FDS": "-1"},
		"fds not a number": {"LISTEN_PID": "42", "LISTEN_FDS": "two"},
		"empty fds":        {"LISTEN_PID": "42", "LISTEN_FDS": ""},
		"too many fds":     {"LISTEN_PID": "42", "LISTEN_FDS": strconv.Itoa(maxListenFDs + 1)},
		"huge fds":         {"LISTEN_PID": "42", "LISTEN_FDS": "99999999999999999999"},
		"fewer names":      {"LISTEN_PID": "42", "LISTEN_FDS": "2", "LISTEN_FDNAMES": "syslog"},
		"more names":       {"LISTEN_PID": "42", "LISTEN_FDS": "1", "LISTEN_FDNAMES": "a:b"},
		"empty name":       {"LISTEN_PID": "42", "LISTEN_FDS": "2", "LISTEN_FDNAMES": "a:"},
		"control in name":  {"LISTEN_PID": "42", "LISTEN_FDS": "1", "LISTEN_FDNAMES": "a\nb"},
		"name over 255":    {"LISTEN_PID": "42", "LISTEN_FDS": "1", "LISTEN_FDNAMES": strings.Repeat("a", 256)},
	} {
		if fds, err := activationFDs(env(e), 42); err == nil {
			t.Errorf("%s: accepted %+v", name, fds)
		}
	}
}

// socketFD returns a duplicate of c's descriptor, which inheritListeners
// takes ownership of, as it does of systemd's. A raw dup, not File(): an
// *os.File's finalizer would close the number again after it was reused.
func socketFD(t *testing.T, c syscall.Conn) int {
	t.Helper()
	rc, err := c.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	fd := -1
	var dupErr error
	if err := rc.Control(func(s uintptr) { fd, dupErr = syscall.Dup(int(s)) }); err != nil {
		t.Fatal(err)
	}
	if dupErr != nil {
		t.Fatal(dupErr)
	}
	syscall.CloseOnExec(fd)
	return fd
}

// pair binds a UDP and a TCP socket on 127.0.0.1, as a .socket unit would,
// and returns them as inherited fds plus the --listen lines they answer.
func pair(t *testing.T) ([]inheritedFD, []listenAddr) {
	t.Helper()
	u, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fds := []inheritedFD{{fd: socketFD(t, u), name: "syslog"}, {fd: socketFD(t, l), name: "syslog"}}
	listens := []listenAddr{
		{"udp", u.LocalAddr().(*net.UDPAddr).AddrPort()},
		{"tcp", l.Addr().(*net.TCPAddr).AddrPort()},
	}
	return fds, listens
}

func TestInheritListeners(t *testing.T) {
	fds, listens := pair(t)
	// The order of --listen need not follow the fds'.
	udp, tcp, err := inheritListeners(fds, []listenAddr{listens[1], listens[0]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, c := range udp {
			_ = c.Close()
		}
		for _, l := range tcp {
			_ = l.Close()
		}
	}()
	if len(udp) != 1 || len(tcp) != 1 {
		t.Fatalf("udp %d tcp %d", len(udp), len(tcp))
	}
	if got := udp[0].LocalAddr().(*net.UDPAddr).AddrPort(); got != listens[0].addr {
		t.Errorf("udp %v, want %v", got, listens[0].addr)
	}
	if got := tcp[0].Addr().(*net.TCPAddr).AddrPort(); got != listens[1].addr {
		t.Errorf("tcp %v, want %v", got, listens[1].addr)
	}
}

// Every inherited socket must be one --listen names, and every --listen must
// arrive: the unit and the command line are one contract, and a socket that
// matches neither would listen where nobody decided it should.
func TestInheritListenersRefuses(t *testing.T) {
	other := netip.MustParseAddrPort("127.0.0.1:1")
	cases := map[string]func(l []listenAddr) []listenAddr{
		"udp fd where tcp listen": func(l []listenAddr) []listenAddr { return []listenAddr{{"tcp", l[0].addr}, l[1]} },
		"tcp fd where udp listen": func(l []listenAddr) []listenAddr { return []listenAddr{l[0], {"udp", l[1].addr}} },
		"another address":         func(l []listenAddr) []listenAddr { return []listenAddr{{"udp", other}, l[1]} },
		"an fd no listen names":   func(l []listenAddr) []listenAddr { return l[:1] },
		"a listen no fd answers":  func(l []listenAddr) []listenAddr { return append(l, listenAddr{"udp", other}) },
		"one listen, two fds":     func(l []listenAddr) []listenAddr { return []listenAddr{l[0], l[0]} },
	}
	for name, mutate := range cases {
		// inheritListeners owns every fd it is given and closes them all
		// when it refuses.
		fds, listens := pair(t)
		if udp, tcp, err := inheritListeners(fds, mutate(listens)); err == nil {
			t.Errorf("%s: accepted (udp %d tcp %d)", name, len(udp), len(tcp))
		}
	}
}

func TestInheritListenersRefusesANonSocket(t *testing.T) {
	var p [2]int
	if err := syscall.Pipe2(p[:], syscall.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(p[1])
	listen := []listenAddr{{"udp", netip.MustParseAddrPort("127.0.0.1:514")}}
	if _, _, err := inheritListeners([]inheritedFD{{fd: p[0], name: "pipe"}}, listen); err == nil {
		t.Error("took a pipe as a socket")
	}
}

// A stream socket nobody called listen(2) on is not a listener.
func TestInheritListenersRefusesAConnectedStream(t *testing.T) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	c, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fd := socketFD(t, c)
	listen := []listenAddr{{"tcp", c.LocalAddr().(*net.TCPAddr).AddrPort()}}
	if _, _, err := inheritListeners([]inheritedFD{{fd: fd, name: "conn"}}, listen); err == nil {
		t.Error("took a connected stream as a listener")
	}
}

// End to end: sockets bound elsewhere (by systemd) serve as if the relay
// bound them.
func TestRelayServesInheritedSockets(t *testing.T) {
	fds, listens := pair(t)
	h := startWith(t, func(c *config) { c.listens = listens }, fds)
	h.sendUDP(knownAddr, "<13>1 - - app - - - via systemd")
	wantField(t, h.next(), "MESSAGE", "via systemd")
	c := h.dialTCP(knownAddr)
	if _, err := io.WriteString(c, "<13>1 - - app - - - tcp via systemd\n"); err != nil {
		t.Fatal(err)
	}
	wantField(t, h.next(), "MESSAGE", "tcp via systemd")
}

// run reads the environment systemd sets; fds meant for another process stop
// it before it touches descriptor 3.
func TestRunRefusesAnotherProcesssFDs(t *testing.T) {
	h := start(t, nil)
	t.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()+1))
	t.Setenv("LISTEN_FDS", "1")
	err := run(context.Background(), h.r.cfg, zerolog.Nop())
	if err == nil || !strings.Contains(err.Error(), "LISTEN_PID") {
		t.Errorf("err = %v", err)
	}
}
