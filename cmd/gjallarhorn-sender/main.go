// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Gjallarhorn sender — CLI to emit UPC trigger packets.
// Phase 8-10 of Mímir's Law spike.
//
// Usage:
//   gjallarhorn-sender --kind bootstrap --cluster 1 --manifest 0xCAFE --multicast
//   gjallarhorn-sender --kind reverify  --cluster 1 --target east --unicast
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"unheaded/pkg/gjallarhorn"
)

const (
	// Link-local IPv6 multicast group for Gjallarhorn bootstrap discovery.
	// ff02::1:abba is in the "variable scope multicast" range
	gjallarhornMcast = "ff02::1:abba"
	gjallarhornPort  = 16901 // in the Doom Range infrastructure tier
)

func main() {
	var (
		kindFlag    = flag.String("kind", "bootstrap", "bootstrap | reverify")
		clusterID   = flag.Uint("cluster", 1, "cluster ID")
		manifestHex = flag.String("manifest", "0xCAFEBABE", "Mjölnir manifest pointer (hex)")
		multicast   = flag.Bool("multicast", false, "multicast to local segment (bootstrap)")
		target      = flag.String("target", "", "unicast target host (reverify)")
		iface       = flag.String("iface", "", "network interface for multicast")
	)
	flag.Parse()

	var kind gjallarhorn.TriggerKind
	switch *kindFlag {
	case "bootstrap":
		kind = gjallarhorn.BootstrapBroadcast
	case "reverify":
		kind = gjallarhorn.ReverifyUnicast
	default:
		fatal("unknown kind: %s", *kindFlag)
	}

	manifestPtr := parseHex(*manifestHex)

	pkt := &gjallarhorn.Packet{
		Magic:       gjallarhorn.Magic,
		Kind:        kind,
		ClusterID:   uint32(*clusterID),
		ManifestPtr: manifestPtr,
	}
	payload := pkt.Marshal()

	fmt.Printf("Gjallarhorn: kind=%s cluster=%d manifest=0x%x\n", *kindFlag, *clusterID, manifestPtr)
	fmt.Printf("Payload (20 bytes): %x\n", payload)

	var addr *net.UDPAddr
	if *multicast {
		ip := net.ParseIP(gjallarhornMcast)
		if ip == nil {
			fatal("invalid multicast address: %s", gjallarhornMcast)
		}
		addr = &net.UDPAddr{IP: ip, Port: gjallarhornPort, Zone: *iface}
	} else {
		if *target == "" {
			fatal("--target required for unicast")
		}
		ips, err := net.LookupIP(*target)
		if err != nil || len(ips) == 0 {
			fatal("lookup %s: %v", *target, err)
		}
		// Prefer IPv6 if available, else IPv4
		var chosen net.IP
		for _, ip := range ips {
			if ip.To4() == nil {
				chosen = ip
				break
			}
		}
		if chosen == nil {
			chosen = ips[0]
		}
		addr = &net.UDPAddr{IP: chosen, Port: gjallarhornPort}
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fatal("dial: %v", err)
	}
	defer conn.Close()

	n, err := conn.Write(payload)
	if err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("Sent %d bytes to %s\n", n, addr)
}

func parseHex(s string) uint64 {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		fatal("parse hex %q: %v", s, err)
	}
	return v
}

func fatal(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "gjallarhorn-sender: "+f+"\n", args...)
	os.Exit(1)
}
