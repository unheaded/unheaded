// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

// Socket activation (sd_listen_fds(3)), standard library only: systemd binds
// 514 in unheaded-syslog-relay.socket and passes the sockets, so the relay
// holds no capability (ADR-124, answer #18 of 2026-11-09). With nothing
// passed the relay binds --listen itself, which is how tests and a dev run
// work.

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const (
	// listenFDsStart is SD_LISTEN_FDS_START: the first passed descriptor.
	listenFDsStart = 3
	// maxListenFDs bounds LISTEN_FDS. The unit passes two; the bound only
	// keeps a garbage value from naming half the descriptor table.
	maxListenFDs = 16
	// maxFDNameLen is systemd's limit on a FileDescriptorName=.
	maxFDNameLen = 255
	// ipprotoMPTCP is IPPROTO_MPTCP (linux/in.h), which package syscall
	// does not name. Go's own TCP listeners are MPTCP by default; a .socket
	// unit's are plain TCP unless SocketProtocol=mptcp. Both serve TCP.
	ipprotoMPTCP = 262
)

// activationEnv is what systemd sets for a socket-activated service.
var activationEnv = []string{"LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES"}

// inheritedFD is one descriptor systemd passed, with its FileDescriptorName=.
type inheritedFD struct {
	fd   int
	name string
}

// activationFDs reads LISTEN_PID, LISTEN_FDS and LISTEN_FDNAMES. None set:
// nil, and the relay binds its own listeners. Any of them set, they must be
// whole and meant for pid. sd_listen_fds(3) quietly ignores fds addressed to
// another process; the relay refuses to start instead, because under systemd
// that is a broken unit, and binding directly would hide it.
func activationFDs(getenv func(string) string, pid int) ([]inheritedFD, error) {
	pidVar, fdsVar, namesVar := getenv("LISTEN_PID"), getenv("LISTEN_FDS"), getenv("LISTEN_FDNAMES")
	if pidVar == "" && fdsVar == "" && namesVar == "" {
		return nil, nil
	}
	target, ok := decimal(pidVar)
	if !ok {
		return nil, fmt.Errorf("socket activation: LISTEN_PID %q is not a process id", pidVar)
	}
	if target != pid {
		return nil, fmt.Errorf("socket activation: LISTEN_PID %d is not this process (%d); the sockets were passed to another", target, pid)
	}
	n, ok := decimal(fdsVar)
	if !ok || n < 1 || n > maxListenFDs {
		return nil, fmt.Errorf("socket activation: LISTEN_FDS %q is not 1..%d", fdsVar, maxListenFDs)
	}
	names := make([]string, n)
	if namesVar == "" {
		for i := range names {
			names[i] = "unknown" // what systemd calls an unnamed fd
		}
	} else {
		names = strings.Split(namesVar, ":")
		if len(names) != n {
			return nil, fmt.Errorf("socket activation: LISTEN_FDNAMES names %d sockets, LISTEN_FDS says %d", len(names), n)
		}
		for _, name := range names {
			if !validFDName(name) {
				return nil, fmt.Errorf("socket activation: LISTEN_FDNAMES holds an invalid name %q", name)
			}
		}
	}
	fds := make([]inheritedFD, n)
	for i := range fds {
		fds[i] = inheritedFD{fd: listenFDsStart + i, name: names[i]}
	}
	return fds, nil
}

// decimal reads a non-negative decimal with no sign, spaces or overflow.
func decimal(s string) (int, bool) {
	if s == "" || len(s) > 10 || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// validFDName is systemd's rule: 1..255 printable ASCII bytes, no colon.
func validFDName(name string) bool {
	if name == "" || len(name) > maxFDNameLen {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; c < 0x20 || c > 0x7e || c == ':' {
			return false
		}
	}
	return true
}

// inheritListeners turns the passed descriptors into the relay's listeners.
// Each must be a UDP socket or a listening TCP socket bound to exactly one
// --listen, and each --listen must arrive: the unit and the command line are
// one contract, so a socket nobody named, or a name with no socket, stops the
// relay. It owns every fd it is given; on error it closes all of them.
func inheritListeners(fds []inheritedFD, want []listenAddr) ([]*net.UDPConn, []*net.TCPListener, error) {
	var (
		udp  []*net.UDPConn
		tcp  []*net.TCPListener
		next int // fds[next:] are still raw descriptors
	)
	abort := func(err error) ([]*net.UDPConn, []*net.TCPListener, error) {
		for _, f := range fds[next:] {
			_ = syscall.Close(f.fd)
		}
		for _, c := range udp {
			_ = c.Close()
		}
		for _, l := range tcp {
			_ = l.Close()
		}
		return nil, nil, err
	}
	used := make([]bool, len(want))
	for next < len(fds) {
		f := fds[next]
		got, err := describe(f.fd)
		if err != nil {
			return abort(fmt.Errorf("passed socket %q (fd %d): %w", f.name, f.fd, err))
		}
		i := -1
		for j, l := range want {
			if !used[j] && l.network == got.network && normalAddrPort(l.addr) == got.addr {
				i = j
				break
			}
		}
		if i < 0 {
			return abort(fmt.Errorf("passed socket %q (fd %d) is %s://%s, which no --listen names", f.name, f.fd, got.network, got.addr))
		}
		used[i] = true
		file := os.NewFile(uintptr(f.fd), f.name)
		next++ // file owns the descriptor now
		switch got.network {
		case "udp":
			pc, err := net.FilePacketConn(file)
			_ = file.Close()
			if err != nil {
				return abort(fmt.Errorf("passed socket %q: %w", f.name, err))
			}
			c, ok := pc.(*net.UDPConn)
			if !ok {
				_ = pc.Close()
				return abort(fmt.Errorf("passed socket %q is not UDP", f.name))
			}
			udp = append(udp, c)
		case "tcp":
			ln, err := net.FileListener(file)
			_ = file.Close()
			if err != nil {
				return abort(fmt.Errorf("passed socket %q: %w", f.name, err))
			}
			l, ok := ln.(*net.TCPListener)
			if !ok {
				_ = ln.Close()
				return abort(fmt.Errorf("passed socket %q is not TCP", f.name))
			}
			tcp = append(tcp, l)
		}
	}
	for i, l := range want {
		if !used[i] {
			return abort(fmt.Errorf("--listen %s://%s: systemd passed no socket for it (the .socket unit and ExecStart= disagree)", l.network, l.addr))
		}
	}
	return udp, tcp, nil
}

type socketInfo struct {
	network string // udp or tcp
	addr    netip.AddrPort
}

var errNotListener = errors.New("not a UDP socket or a listening TCP socket")

// describe asks the kernel what fd is, before anything trusts it.
func describe(fd int) (socketInfo, error) {
	typ, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_TYPE)
	if err != nil {
		return socketInfo{}, fmt.Errorf("not a socket: %w", err)
	}
	proto, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_PROTOCOL)
	if err != nil {
		return socketInfo{}, err
	}
	var info socketInfo
	switch {
	case typ == syscall.SOCK_DGRAM && proto == syscall.IPPROTO_UDP:
		info.network = "udp"
	case typ == syscall.SOCK_STREAM && (proto == syscall.IPPROTO_TCP || proto == ipprotoMPTCP):
		listening, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_ACCEPTCONN)
		if err != nil {
			return socketInfo{}, err
		}
		if listening != 1 {
			return socketInfo{}, errNotListener
		}
		info.network = "tcp"
	default:
		return socketInfo{}, errNotListener
	}
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		return socketInfo{}, err
	}
	var ip netip.Addr
	var port int
	switch sa := sa.(type) {
	case *syscall.SockaddrInet4:
		ip, port = netip.AddrFrom4(sa.Addr), sa.Port
	case *syscall.SockaddrInet6:
		ip, port = netip.AddrFrom16(sa.Addr), sa.Port
	default:
		return socketInfo{}, errNotListener
	}
	if port < 0 || port > 0xFFFF {
		return socketInfo{}, errNotListener
	}
	info.addr = netip.AddrPortFrom(ip, uint16(port))
	info.addr = normalAddrPort(info.addr)
	return info, nil
}

func normalAddrPort(a netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(normal(a.Addr()), a.Port())
}
