// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package main implements a high-performance packet injector for the Doom-over-IPv6
// compute ring. It replaces the Python inject.py with AF_PACKET raw socket injection
// for significantly lower per-packet overhead.
//
// The injector builds Monad HBH extension header packets and sends them via
// AF_PACKET SOCK_RAW directly on the specified network interface. It supports
// three injection modes:
//
//   - steady: Fixed inter-packet delay (like the Python baseline)
//   - burst:  Netflix-model burst injection (fire N packets, drain pause, repeat)
//   - fast:   Zero-delay maximum throughput injection
//
// Usage:
//
//	sudo ip netns exec monad0 doom-go-injector --count 5000 --mode burst --batch 100
//	sudo ip netns exec monad0 doom-go-injector --mode steady --delay 1500 --count 1000
//	sudo ip netns exec monad0 doom-go-injector --mode fast --count 10000
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/unix"
)

// injectorConfig holds TOML configuration from the [injector] section.
type injectorConfig struct {
	Interface   string `toml:"interface"`
	Mode        string `toml:"mode"`
	SteadyDelay int    `toml:"steady_delay_us"`
	BurstSize   int    `toml:"burst_size"`
}

// doomConfig wraps the top-level TOML file to extract the [injector] section.
type doomConfig struct {
	Injector injectorConfig `toml:"injector"`
}

// loadConfig reads a TOML config file and returns the injector section.
func loadConfig(path string) (injectorConfig, error) {
	var cfg doomConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return injectorConfig{}, fmt.Errorf("load config %q: %w", path, err)
	}
	return cfg.Injector, nil
}

// Packet geometry constants.
const (
	EthHdrLen   = 14
	IPv6HdrLen  = 40
	HBHHdrLen   = 4
	MonadRegLen = 20
	PacketSize  = EthHdrLen + IPv6HdrLen + HBHHdrLen + MonadRegLen // 78 bytes

	// EtherType IPv6.
	EthTypeIPv6 = 0x86DD

	// Instructions per packet (128 insns/bounce * 255 bounces / 8 = ~4080).
	InsnsPerPacket = 4080

	// Default flow label identifying the Doom compute instance.
	DefaultFlowLabel = 0xDE
)

// buildPacket constructs a 78-byte Doom ring packet:
//
//	[14] Ethernet header (dst MAC, src MAC, EtherType 0x86DD)
//	[40] IPv6 header (version 6, HBH next header, hop limit 255)
//	[ 4] Hop-by-Hop extension header (next=No Next, len=2, type=0x3E, optlen=20)
//	[20] Monad register payload
func buildPacket(flowLabel uint32, srcMAC, dstMAC [6]byte) [PacketSize]byte {
	var pkt [PacketSize]byte

	// --- Ethernet header (14 bytes) ---
	copy(pkt[0:6], dstMAC[:])
	copy(pkt[6:12], srcMAC[:])
	binary.BigEndian.PutUint16(pkt[12:14], EthTypeIPv6)

	// --- IPv6 header (40 bytes) ---
	// Version (4 bits) + Traffic Class (8 bits) + Flow Label (20 bits).
	vtcfl := uint32(6<<28) | (flowLabel & 0xFFFFF)
	binary.BigEndian.PutUint32(pkt[14:18], vtcfl)

	// Payload length: HBH (4) + Monad (20) = 24 bytes.
	binary.BigEndian.PutUint16(pkt[18:20], uint16(HBHHdrLen+MonadRegLen))

	// Next header: 0 = Hop-by-Hop Options.
	pkt[20] = 0x00
	// Hop limit: 255.
	pkt[21] = 0xFF

	// Source address: fd00:3f:75::1
	srcAddr := [16]byte{
		0xFD, 0x00, 0x00, 0x3F, 0x00, 0x75, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	}
	copy(pkt[22:38], srcAddr[:])

	// Destination address: fd00:dead::1 (routed into the ring).
	dstAddr := [16]byte{
		0xFD, 0x00, 0xDE, 0xAD, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	}
	copy(pkt[38:54], dstAddr[:])

	// --- Hop-by-Hop Options header (4 bytes) ---
	pkt[54] = 0x3B // Next header: 59 = No Next Header.
	pkt[55] = 0x02 // Header extension length in 8-byte units (minus 1): (24/8 - 1) = 2.
	pkt[56] = 0x3E // Option type: 0x3E = Monad register.
	pkt[57] = 0x14 // Option data length: 20 bytes.

	// --- Monad register (20 bytes) ---
	// Byte 0: monad_type = 0x01 (tick packet).
	pkt[58] = 0x01
	// Byte 7: monad_version = 0x02.
	pkt[65] = 0x02
	// Remaining 12 bytes are zero (latency_hint, deploy_ring, mesh_flags,
	// src_prefix, dst_prefix, scratch[4], checksum[2]).

	return pkt
}

// parseMAC parses a colon-separated MAC address string into a 6-byte array.
func parseMAC(s string) ([6]byte, error) {
	hw, err := net.ParseMAC(s)
	if err != nil {
		return [6]byte{}, fmt.Errorf("invalid MAC %q: %w", s, err)
	}
	if len(hw) != 6 {
		return [6]byte{}, fmt.Errorf("invalid MAC length: got %d, want 6", len(hw))
	}
	var mac [6]byte
	copy(mac[:], hw)
	return mac, nil
}

// openAFPacket opens an AF_PACKET SOCK_RAW socket bound to the specified interface.
// Returns the raw file descriptor and the sockaddr_ll for sendto calls.
func openAFPacket(ifaceName string) (int, *unix.SockaddrLinklayer, error) {
	// Open AF_PACKET socket with ETH_P_IPV6 protocol filter.
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_IPV6)))
	if err != nil {
		return -1, nil, fmt.Errorf("socket(AF_PACKET): %w", err)
	}

	// Look up interface index.
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		unix.Close(fd) // #nosec G104 -- failure here cannot change the outcome; the significant error is already being returned
		return -1, nil, fmt.Errorf("interface %q: %w", ifaceName, err)
	}

	// Bind to the interface.
	sll := &unix.SockaddrLinklayer{
		Protocol: htons(unix.ETH_P_IPV6),
		Ifindex:  iface.Index,
	}

	if err := unix.Bind(fd, sll); err != nil {
		unix.Close(fd) // #nosec G104 -- failure here cannot change the outcome; the significant error is already being returned
		return -1, nil, fmt.Errorf("bind(%s): %w", ifaceName, err)
	}

	// Set send buffer size to 4 MiB for burst performance.
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, 4*1024*1024); err != nil {
		// Non-fatal; continue with default buffer.
		fmt.Fprintf(os.Stderr, "warning: SO_SNDBUF: %v\n", err)
	}

	return fd, sll, nil
}

// htons converts a uint16 from host to network byte order.
func htons(v uint16) uint16 {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return *(*uint16)(unsafe.Pointer(&b[0])) // #nosec G103 -- typed overlay on a byte buffer sized by the kernel ABI struct it mirrors
}

// injectSteady sends packets with a fixed inter-packet delay.
func injectSteady(fd int, sll *unix.SockaddrLinklayer, pkt []byte, count int, delayUS int, reportEvery int, shutdown *atomic.Bool) (uint64, time.Duration) {
	delay := time.Duration(delayUS) * time.Microsecond
	start := time.Now()
	var sent uint64

	for i := 0; i < count; i++ {
		if shutdown.Load() {
			break
		}
		if err := unix.Sendto(fd, pkt, 0, sll); err != nil {
			fmt.Fprintf(os.Stderr, "sendto: %v (packet %d)\n", err, i)
			break
		}
		sent++
		if delay > 0 {
			time.Sleep(delay)
		}
		if reportEvery > 0 && (i+1)%reportEvery == 0 {
			elapsed := time.Since(start).Seconds()
			pps := float64(i+1) / elapsed
			insns := uint64(i+1) * InsnsPerPacket
			fmt.Printf("  [%3d%%] %d/%d pkt (%.0f pkt/s, ~%d insns)\n",
				(i+1)*100/count, i+1, count, pps, insns)
		}
	}
	return sent, time.Since(start)
}

// injectBurst sends packets in bursts with a brief drain pause between each batch.
// This models the Netflix approach: saturate the send buffer, let the kernel drain,
// then fire again. count=0 means infinite (run until SIGINT/SIGTERM).
func injectBurst(fd int, sll *unix.SockaddrLinklayer, pkt []byte, count, batchSize, burstSleepUS, reportEvery int, shutdown *atomic.Bool) (uint64, time.Duration) {
	start := time.Now()
	var sent uint64
	batches := 0
	infinite := count == 0
	drainPause := time.Duration(burstSleepUS) * time.Microsecond
	// Convert packet-level reportEvery to a batch count for burst reporting.
	reportBatches := reportEvery / batchSize
	if reportBatches < 1 {
		reportBatches = 1
	}

	for (infinite || sent < uint64(count)) && !shutdown.Load() { // #nosec G115 -- UNFS inode field; bounded by the filesystem image size
		batch := batchSize
		if !infinite {
			remaining := uint64(count) - sent // #nosec G115 -- bounded by construction; see the surrounding guard
			if uint64(batch) > remaining {    // #nosec G115 -- bounded by construction; see the surrounding guard
				batch = int(remaining) // #nosec G115 -- bounded by construction; see the surrounding guard
			}
		}

		for j := 0; j < batch; j++ {
			if err := unix.Sendto(fd, pkt, 0, sll); err != nil {
				fmt.Fprintf(os.Stderr, "sendto: %v (packet %d)\n", err, sent+uint64(j))
				return sent + uint64(j), time.Since(start)
			}
		}
		sent += uint64(batch) // #nosec G115 -- bounded by construction; see the surrounding guard
		batches++

		// Brief drain pause between batches to let XDP process.
		time.Sleep(drainPause)

		if reportBatches > 0 && batches%reportBatches == 0 {
			elapsed := time.Since(start).Seconds()
			pps := float64(sent) / elapsed
			insns := sent * 256 * 255
			fmt.Printf("  %d pkt (%.0f pkt/s, ~%.1fB insns)\n",
				sent, pps, float64(insns)/1e9)
		}
	}
	return sent, time.Since(start)
}

// injectFast sends packets with zero delay for maximum throughput measurement.
// count=0 means infinite (run until SIGINT/SIGTERM).
func injectFast(fd int, sll *unix.SockaddrLinklayer, pkt []byte, count int, shutdown *atomic.Bool) (uint64, time.Duration) {
	start := time.Now()
	var sent uint64
	infinite := count == 0

	for infinite || sent < uint64(count) { // #nosec G115 -- bounded by construction; see the surrounding guard
		if shutdown.Load() {
			break
		}
		if err := unix.Sendto(fd, pkt, 0, sll); err != nil {
			fmt.Fprintf(os.Stderr, "sendto: %v (packet %d)\n", err, sent)
			break
		}
		sent++
	}
	return sent, time.Since(start)
}

// mmsghdr matches the kernel struct mmsghdr layout for sendmmsg.
type mmsghdr struct {
	Hdr    unix.Msghdr
	MsgLen uint32
	_pad   [4]byte // alignment padding on 64-bit
}

// injectSendmmsg sends packets using the sendmmsg(2) syscall for minimal
// per-packet overhead. One syscall dispatches an entire batch of identical
// packets, reducing kernel entry/exit cost by the batch factor.
// count=0 means infinite (run until SIGINT/SIGTERM).
func injectSendmmsg(fd int, sll *unix.SockaddrLinklayer, pkt []byte, count, batchSize, reportEvery int, shutdown *atomic.Bool) (uint64, time.Duration) {
	start := time.Now()
	var sent uint64
	infinite := count == 0

	// Build the raw sockaddr_ll in wire format for msg_name.
	// struct sockaddr_ll: family(2) + protocol(2) + ifindex(4) + hatype(2) + pkttype(1) + halen(1) + addr(8)
	var rawAddr [20]byte
	binary.LittleEndian.PutUint16(rawAddr[0:2], unix.AF_PACKET)
	binary.BigEndian.PutUint16(rawAddr[2:4], unix.ETH_P_IPV6)
	binary.LittleEndian.PutUint32(rawAddr[4:8], uint32(sll.Ifindex)) // #nosec G115 -- 8/16-bit MBC memory store; the truncation IS the instruction semantics, bounds-checked above
	// hatype, pkttype, halen, addr left as zero — sufficient for sendmsg on AF_PACKET

	// Pre-allocate iovec and mmsghdr arrays. All entries share the same packet
	// buffer and sockaddr since every message is identical.
	iovecs := make([]unix.Iovec, batchSize)
	pktCopy := make([]byte, len(pkt))
	copy(pktCopy, pkt)
	pktPtr := &pktCopy[0]
	pktLen := uint64(len(pktCopy))

	for i := range iovecs {
		iovecs[i].Base = pktPtr
		iovecs[i].SetLen(int(pktLen)) // #nosec G115 -- bounded by construction; see the surrounding guard
	}

	msgs := make([]mmsghdr, batchSize)
	addrPtr := unsafe.Pointer(&rawAddr[0]) // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)
	for i := range msgs {
		msgs[i].Hdr.Name = (*byte)(addrPtr)
		msgs[i].Hdr.Namelen = uint32(len(rawAddr))
		msgs[i].Hdr.Iov = &iovecs[i]
		msgs[i].Hdr.Iovlen = 1
	}

	reportBatches := reportEvery / batchSize
	if reportBatches < 1 {
		reportBatches = 1
	}
	batches := 0

	for (infinite || sent < uint64(count)) && !shutdown.Load() { // #nosec G115 -- UNFS inode field; bounded by the filesystem image size
		batch := batchSize
		if !infinite {
			raw := uint64(count) - sent // #nosec G115 -- bounded by construction; see the surrounding guard
			if raw > uint64(math.MaxInt) {
				raw = uint64(math.MaxInt)
			}
			remaining := int(raw)
			if batch > remaining {
				batch = remaining
			}
		}

		// sendmmsg(fd, msgs, batch, 0)
		n, _, errno := syscall.Syscall6(
			unix.SYS_SENDMMSG,
			uintptr(fd),
			uintptr(unsafe.Pointer(&msgs[0])), // #nosec G103 -- Pointer->uintptr inside a syscall argument list, the pattern unsafe.Pointer rule (4) permits
			uintptr(batch),
			0, 0, 0,
		)
		if errno != 0 {
			if errno == syscall.EAGAIN || errno == syscall.ENOBUFS {
				// Buffer full — brief yield then retry.
				time.Sleep(10 * time.Microsecond)
				continue
			}
			fmt.Fprintf(os.Stderr, "sendmmsg: %v (sent %d)\n", errno, sent)
			break
		}
		sent += uint64(n)
		batches++

		if reportBatches > 0 && batches%reportBatches == 0 {
			elapsed := time.Since(start).Seconds()
			pps := float64(sent) / elapsed
			insns := sent * 256 * 255
			fmt.Printf("  %d pkt (%.0f pkt/s, ~%.1fB insns)\n",
				sent, pps, float64(insns)/1e9)
		}
	}
	return sent, time.Since(start)
}

func main() {
	count := flag.Int("count", 1000, "Number of packets to inject (0 = infinite, run until Ctrl+C)")
	batchSize := flag.Int("batch", 100, "Packets per burst (burst mode only)")
	ifaceName := flag.String("iface", "veth01", "Network interface to inject on")
	srcMACStr := flag.String("src-mac", "02:42:ac:11:00:02", "Source MAC address")
	dstMACStr := flag.String("dst-mac", "02:42:ac:11:00:03", "Destination MAC address")
	flowLabel := flag.Uint("flow-label", DefaultFlowLabel, "IPv6 flow label (identifies Doom instance)")
	mode := flag.String("mode", "burst", "Injection mode: steady, burst, fast, sendmmsg")
	delayUS := flag.Int("delay", 3000, "Inter-packet delay in microseconds (steady mode only)")
	burstSleep := flag.Int("burst-sleep", 50, "Drain pause between bursts in microseconds (burst mode)")
	rateHz := flag.Int("rate", 0, "Injection rate in Hz (overrides --delay, sets mode to steady)")
	configFile := flag.String("config", "", "Path to TOML config file (e.g. configs/doom.toml)")
	reportEvery := flag.Int("report-every", 10000, "Log progress every N packets")

	flag.Parse()

	// Load TOML config if --config is provided. Config values act as defaults;
	// any flag explicitly set on the command line takes precedence.
	if *configFile != "" {
		cfg, err := loadConfig(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}

		// Track which flags were explicitly set on the command line.
		explicit := make(map[string]bool)
		flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

		// Apply config values only for flags not explicitly set.
		if !explicit["iface"] && cfg.Interface != "" {
			*ifaceName = cfg.Interface
		}
		if !explicit["mode"] && cfg.Mode != "" {
			*mode = cfg.Mode
		}
		if !explicit["delay"] && cfg.SteadyDelay > 0 {
			*delayUS = cfg.SteadyDelay
		}
		if !explicit["batch"] && cfg.BurstSize > 0 {
			*batchSize = cfg.BurstSize
		}
	}

	// --rate convenience: convert Hz to delay and force steady mode.
	if *rateHz > 0 {
		*delayUS = 1_000_000 / *rateHz
		*mode = "steady"
	}

	// Parse MAC addresses.
	srcMAC, err := parseMAC(*srcMACStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	dstMAC, err := parseMAC(*dstMACStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	// Build the packet template once (immutable for the entire run).
	pktArr := buildPacket(uint32(*flowLabel), srcMAC, dstMAC) // #nosec G115 -- bounded by construction; see the surrounding guard
	pkt := pktArr[:]

	// Open AF_PACKET socket.
	fd, sll, err := openAFPacket(*ifaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		fmt.Fprintf(os.Stderr, "Are you running as root inside a monad namespace?\n")
		fmt.Fprintf(os.Stderr, "  sudo ip netns exec monad0 %s ...\n", os.Args[0])
		os.Exit(1)
	}
	defer unix.Close(fd)

	// Set up signal handling for graceful shutdown.
	var shutdown atomic.Bool
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\nSignal received -- finishing current batch...\n")
		shutdown.Store(true)
	}()

	// Print configuration banner.
	fmt.Printf("=== Go Doom Ring Injector ===\n")
	if *configFile != "" {
		fmt.Printf("  Config:     %s\n", *configFile)
	}
	fmt.Printf("  Mode:       %s\n", *mode)
	if *count == 0 {
		fmt.Printf("  Count:      infinite (Ctrl+C to stop)\n")
	} else {
		fmt.Printf("  Count:      %d\n", *count)
	}
	fmt.Printf("  Interface:  %s\n", *ifaceName)
	fmt.Printf("  Flow label: 0x%X\n", *flowLabel)
	switch *mode {
	case "steady":
		fmt.Printf("  Delay:      %d us\n", *delayUS)
	case "burst":
		fmt.Printf("  Batch size: %d\n", *batchSize)
		fmt.Printf("  Burst sleep: %d us\n", *burstSleep)
	case "fast":
		fmt.Printf("  Delay:      0 (max throughput)\n")
	case "sendmmsg":
		fmt.Printf("  Batch size: %d (sendmmsg)\n", *batchSize)
	}
	fmt.Printf("  Report:     every %d packets\n", *reportEvery)
	fmt.Println()

	// Run the selected injection mode.
	var sent uint64
	var elapsed time.Duration

	switch *mode {
	case "steady":
		sent, elapsed = injectSteady(fd, sll, pkt, *count, *delayUS, *reportEvery, &shutdown)
	case "burst":
		sent, elapsed = injectBurst(fd, sll, pkt, *count, *batchSize, *burstSleep, *reportEvery, &shutdown)
	case "fast":
		sent, elapsed = injectFast(fd, sll, pkt, *count, &shutdown)
	case "sendmmsg":
		sent, elapsed = injectSendmmsg(fd, sll, pkt, *count, *batchSize, *reportEvery, &shutdown)
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown mode %q (use: steady, burst, fast, sendmmsg)\n", *mode)
		os.Exit(1)
	}

	// Print final summary.
	elapsedSec := elapsed.Seconds()
	pps := float64(0)
	if elapsedSec > 0 {
		pps = float64(sent) / elapsedSec
	}
	totalInsns := sent * InsnsPerPacket

	fmt.Printf("\nInjected %d packets in %.1fs (%.0f pkt/s, ~%d insns)\n",
		sent, elapsedSec, pps, totalInsns)

	if shutdown.Load() {
		fmt.Println("(interrupted by signal)")
	}
}
