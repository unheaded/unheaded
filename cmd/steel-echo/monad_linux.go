// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"unheaded/pkg/monadpkt"
)

// rawSocket is an AF_PACKET SOCK_RAW socket bound to one lab interface.
type rawSocket struct {
	fd  int
	sll *unix.SockaddrLinklayer
	mac [6]byte
}

func htons(v uint16) uint16 {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return binary.NativeEndian.Uint16(b[:])
}

func openRaw(iface string) (*rawSocket, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("interface %s: %w", iface, err)
	}
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, int(htons(unix.ETH_P_IPV6)))
	if err != nil {
		return nil, fmt.Errorf("socket(AF_PACKET): %w (needs CAP_NET_RAW: run with sudo)", err)
	}
	sll := &unix.SockaddrLinklayer{Protocol: htons(unix.ETH_P_IPV6), Ifindex: ifi.Index}
	if err := unix.Bind(fd, sll); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("bind %s: %w", iface, err)
	}
	s := &rawSocket{fd: fd, sll: sll}
	copy(s.mac[:], ifi.HardwareAddr)
	return s, nil
}

func (s *rawSocket) Close() error { return unix.Close(s.fd) }

// Write sends one frame on the bound interface.
func (s *rawSocket) Write(b []byte) error {
	return unix.Sendto(s.fd, b, 0, s.sll)
}

// Read returns the next frame's first monadpkt.HeaderLen bytes, or nil on a
// poll timeout, so the caller can check ctx between frames.
func (s *rawSocket) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fds := []unix.PollFd{{Fd: int32(s.fd), Events: unix.POLLIN}} // #nosec G115 -- fd fits int32
	n, err := unix.Poll(fds, 200)
	if err != nil {
		if errors.Is(err, unix.EINTR) {
			return nil, nil
		}
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, monadpkt.HeaderLen)
	// MSG_TRUNC: the kernel reports the frame's full length, we keep the headers.
	m, _, err := unix.Recvfrom(s.fd, buf, unix.MSG_TRUNC)
	if err != nil {
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			return nil, nil
		}
		return nil, err
	}
	if m > len(buf) {
		m = len(buf)
	}
	return buf[:m], nil
}

func runMonadSend(args []string) error {
	c, err := parseSendFlags(args)
	if err != nil {
		return err
	}
	s, err := openRaw(c.iface)
	if err != nil {
		return err
	}
	defer s.Close()
	frame, err := c.frame(s.mac)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()
	sent, err := sendFrames(ctx, s, frame, c.count, c.interval)
	fmt.Fprintf(os.Stderr, "monad-send: %d/%d frames on %s flow label 0x%x in %v\n", sent, c.count, c.iface, c.flowLabel, time.Since(start).Round(time.Millisecond))
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runMonadCount(args []string) error {
	c, err := parseCountFlags(args)
	if err != nil {
		return err
	}
	s, err := openRaw(c.iface)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	r := newCountReport(c.iface, c.seconds)
	err = countFrames(ctx, s, r, time.Duration(c.seconds)*time.Second)
	fmt.Fprintf(os.Stderr, "monad-count: %s\n", r.summary())
	if werr := r.write(os.Stdout); werr != nil {
		return werr
	}
	return err
}
