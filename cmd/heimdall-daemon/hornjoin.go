// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"net"

	"unheaded/pkg/gjallarhorn"
)

// joinHornGroup joins the Gjallarhorn bootstrap group on ifname and returns
// the socket that holds the membership; close it to leave. Heimdall observes
// horns with XDP and never reads this socket: the join is there because a
// real NIC drops frames for multicast groups no socket on the host joined,
// before XDP runs (S-ASCEND-2 E3, found on east; a veth does not filter).
func joinHornGroup(ifname string) (*net.UDPConn, error) {
	ifi, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, fmt.Errorf("join %s on %s: %w", gjallarhorn.MulticastGroup, ifname, err)
	}
	c, err := net.ListenMulticastUDP("udp6", ifi, &net.UDPAddr{IP: net.ParseIP(gjallarhorn.MulticastGroup), Port: gjallarhorn.Port})
	if err != nil {
		return nil, fmt.Errorf("join %s on %s: %w", gjallarhorn.MulticastGroup, ifname, err)
	}
	return c, nil
}
