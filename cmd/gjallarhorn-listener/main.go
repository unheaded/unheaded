// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Gjallarhorn listener — receives UPC trigger packets and prints them.
// Phase 9 validation tool. The full daemon will fold this into heimdall-daemon.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"unheaded/pkg/gjallarhorn"
)

const gjallarhornPort = 16901

func main() {
	var (
		multicast = flag.Bool("multicast", false, "join link-local multicast group")
		mcastAddr = flag.String("mcast-addr", "ff02::1:abba", "multicast group")
		iface     = flag.String("iface", "eth0", "interface for multicast")
		listenIP  = flag.String("listen", "::", "listen address (unicast)")
	)
	flag.Parse()

	var conn *net.UDPConn
	var err error

	if *multicast {
		ifi, ierr := net.InterfaceByName(*iface)
		if ierr != nil {
			fatal("interface %s: %v", *iface, ierr)
		}
		group := &net.UDPAddr{
			IP:   net.ParseIP(*mcastAddr),
			Port: gjallarhornPort,
		}
		conn, err = net.ListenMulticastUDP("udp6", ifi, group)
		if err != nil {
			fatal("multicast listen: %v", err)
		}
		fmt.Printf("Listening on multicast [%s]:%d via %s\n", *mcastAddr, gjallarhornPort, *iface)
	} else {
		addr := &net.UDPAddr{IP: net.ParseIP(*listenIP), Port: gjallarhornPort}
		conn, err = net.ListenUDP("udp6", addr)
		if err != nil {
			fatal("listen: %v", err)
		}
		fmt.Printf("Listening on unicast [%s]:%d\n", *listenIP, gjallarhornPort)
	}
	defer conn.Close()

	buf := make([]byte, gjallarhorn.PacketSize+64)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read: %v\n", err)
			continue
		}
		fmt.Printf("[%s] %d bytes from %s: %x\n", "RECV", n, src, buf[:n])

		pkt, err := gjallarhorn.Unmarshal(buf[:n])
		if err != nil {
			fmt.Printf("  invalid: %v\n", err)
			continue
		}
		kindName := "BootstrapBroadcast"
		if pkt.Kind == gjallarhorn.ReverifyUnicast {
			kindName = "ReverifyUnicast"
		}
		fmt.Printf("  ✓ Gjallarhorn: kind=%s cluster=%d manifest=0x%x\n",
			kindName, pkt.ClusterID, pkt.ManifestPtr)
	}
}

func fatal(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "gjallarhorn-listener: "+f+"\n", args...)
	os.Exit(1)
}
