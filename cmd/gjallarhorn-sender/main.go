// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Gjallarhorn sender — CLI to emit UPC trigger packets.
// Phase 8-10 of Mímir's Law spike.
//
// Usage:
//
//	gjallarhorn-sender --kind bootstrap --cluster 1 --manifest 0xCAFE --multicast
//	gjallarhorn-sender --kind reverify  --cluster 1 --target east --unicast
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/gjallarhorn"
)

const (
	// Link-local IPv6 multicast group for Gjallarhorn bootstrap discovery.
	// ff02::1:abba is in the "variable scope multicast" range
	gjallarhornMcast = gjallarhorn.MulticastGroup
	gjallarhornPort  = gjallarhorn.Port
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
	cmdline.Parse(cmdline.Spec{
		Name:     "gjallarhorn-sender",
		Purpose:  "Emit UPC trigger packets (Gjallarhorn) for Mímir's Law.",
		Examples: []string{"gjallarhorn-sender --kind bootstrap --cluster 1 --manifest 0xCAFE --multicast"},
	})

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
		ClusterID:   uint32(*clusterID), // #nosec G115 -- bounded by construction; see the surrounding guard
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
		a, err := unicastAddr(*target)
		if err != nil {
			fatal("%v", err)
		}
		addr = a
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

// unicastAddr resolves a reverify target. An IPv6 literal may carry a zone
// (fe80::…%iface), which a link-local-only link needs; a zone on any other
// address is refused. Hostnames prefer an IPv6 address, else the first one.
func unicastAddr(target string) (*net.UDPAddr, error) {
	host, zone, _ := strings.Cut(target, "%")
	if ip := net.ParseIP(host); ip != nil {
		if zone != "" && !ip.IsLinkLocalUnicast() {
			return nil, fmt.Errorf("zone %q on %s: only link-local addresses take a zone", zone, host)
		}
		return &net.UDPAddr{IP: ip, Port: gjallarhornPort, Zone: zone}, nil
	}
	if zone != "" {
		return nil, fmt.Errorf("zone %q on hostname %s", zone, host)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("lookup %s: %v", host, err)
	}
	chosen := ips[0]
	for _, ip := range ips {
		if ip.To4() == nil {
			chosen = ip
			break
		}
	}
	return &net.UDPAddr{IP: chosen, Port: gjallarhornPort}, nil
}

// parseHexErr is the pure-function variant of parseHex — returns (value, error)
// without process-exiting. Extracted for unit testability.
func parseHexErr(s string) (uint64, error) {
	stripped := strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	return strconv.ParseUint(stripped, 16, 64)
}

func parseHex(s string) uint64 {
	v, err := parseHexErr(s)
	if err != nil {
		fatal("parse hex %q: %v", s, err)
	}
	return v
}

func fatal(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "gjallarhorn-sender: "+f+"\n", args...)
	os.Exit(1)
}
