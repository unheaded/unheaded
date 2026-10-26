// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package main

import (
	"context"
	"errors"

	"unheaded/services/yaldabaoth/packet"
)

type noBackend struct{}

func (noBackend) Present(string) bool                             { return false }
func (noBackend) Load(string, string) error                       { return errNoBPF }
func (noBackend) Attach(string, string) error                     { return errNoBPF }
func (noBackend) WriteTargets(string, map[uint32]uint64) error    { return errNoBPF }
func (noBackend) WriteConfig(string, uint32, uint64) error        { return errNoBPF }
func (noBackend) ReadStats(string) (map[uint32]uint64, error)     { return nil, errNoBPF }
func (noBackend) Detach(string) error                             { return errNoBPF }
func (noBackend) DeleteTargets(string, []uint32) error            { return errNoBPF }
func (noBackend) Unpin(string) error                              { return errNoBPF }
func (noBackend) Pinned(string) bool                              { return false }
func (noBackend) Linked(string) bool                              { return false }
func (noBackend) TC(context.Context, []string) error              { return errNoBPF }
func (noBackend) RootQdisc(context.Context, string) (bool, error) { return false, errNoBPF }

var errNoBPF = errors.New("packet_chaos needs Linux")

// newPacketBackend has no kernel to talk to off Linux.
var newPacketBackend = func() packet.Backend { return noBackend{} }
