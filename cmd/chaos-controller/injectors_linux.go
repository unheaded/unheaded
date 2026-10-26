// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"os"

	"unheaded/services/yaldabaoth"
	"unheaded/services/yaldabaoth/packet"
)

// newPacketBackend is the kernel for packet_chaos; tests swap it.
var newPacketBackend = func() packet.Backend { return packet.NewLinuxBackend() }

func init() {
	// packet_chaos (S-REAL-STEEL Phase 13) is the first real injector. The
	// Phase 11/12 netem and systemd injectors register here when they land.
	registerInjectors = func(s *yaldabaoth.Service) {
		s.RegisterInjector(yaldabaoth.ExpTypePacketChaos,
			packet.New(newPacketBackend(), packet.Options{Object: packet.ObjectPath(os.Getenv)}))
	}
}
