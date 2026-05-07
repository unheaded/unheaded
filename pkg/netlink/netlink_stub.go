// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux
// +build !linux

// Package netlink provides a from-scratch implementation of Linux netlink sockets.
// This stub file exists for non-Linux platforms to allow package discovery,
// but netlink functionality is only available on Linux.
package netlink

import "errors"

// ErrNotSupported is returned when netlink operations are attempted on non-Linux platforms.
var ErrNotSupported = errors.New("netlink is only supported on Linux")

// Socket is a stub for non-Linux platforms.
type Socket struct{}

// NewSocket returns an error on non-Linux platforms.
func NewSocket(family int) (*Socket, error) {
	return nil, ErrNotSupported
}

// Close is a stub for non-Linux platforms.
func (s *Socket) Close() error {
	return ErrNotSupported
}

// LinkInfo contains parsed information about a network interface.
type LinkInfo struct {
	Index      int32
	Name       string
	MTU        uint32
	Flags      uint32
	OperState  uint8
	TxQueueLen uint32
	Qdisc      string
	Master     int32
}

// AddrInfo contains parsed information about an interface address.
type AddrInfo struct {
	Index     uint32
	Family    uint8
	PrefixLen uint8
	Scope     uint8
	Label     string
}

// RouteInfo contains parsed information about a route.
type RouteInfo struct {
	Family   uint8
	DstLen   uint8
	SrcLen   uint8
	Table    uint8
	Protocol uint8
	Scope    uint8
	Type     uint8
	OifIndex int32
	IifIndex int32
	Priority uint32
}

// GenlSocket is a stub for non-Linux platforms.
type GenlSocket struct {
	*Socket
}

// NewGenlSocket returns an error on non-Linux platforms.
func NewGenlSocket() (*GenlSocket, error) {
	return nil, ErrNotSupported
}
