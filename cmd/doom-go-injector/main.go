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
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

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
		unix.Close(fd)
		return -1, nil, fmt.Errorf("interface %q: %w", ifaceName, err)
	}

	// Bind to the interface.
	sll := &unix.SockaddrLinklayer{
		Protocol: htons(unix.ETH_P_IPV6),
		Ifindex:  iface.Index,
	}

	if err := unix.Bind(fd, sll); err != nil {
		unix.Close(fd)
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
	return *(*uint16)(unsafe.Pointer(&b[0]))
}

// injectSteady sends packets with a fixed inter-packet delay.
func injectSteady(fd int, sll *unix.SockaddrLinklayer, pkt []byte, count int, delayUS int, shutdown *atomic.Bool) (uint64, time.Duration) {
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
		if (i+1)%1000 == 0 {
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
// then fire again.
func injectBurst(fd int, sll *unix.SockaddrLinklayer, pkt []byte, count, batchSize int, shutdown *atomic.Bool) (uint64, time.Duration) {
	start := time.Now()
	var sent uint64
	batches := 0

	for sent < uint64(count) && !shutdown.Load() {
		batch := batchSize
		remaining := uint64(count) - sent
		if uint64(batch) > remaining {
			batch = int(remaining)
		}

		for j := 0; j < batch; j++ {
			if err := unix.Sendto(fd, pkt, 0, sll); err != nil {
				fmt.Fprintf(os.Stderr, "sendto: %v (packet %d)\n", err, sent+uint64(j))
				return sent + uint64(j), time.Since(start)
			}
		}
		sent += uint64(batch)
		batches++

		// Brief drain pause between batches (100us) to let the XDP ring
		// process the burst before we fire the next one.
		time.Sleep(100 * time.Microsecond)

		if batches%10 == 0 || sent >= uint64(count) {
			elapsed := time.Since(start).Seconds()
			pps := float64(sent) / elapsed
			fmt.Printf("  [%3d%%] %d/%d pkt (%.0f pkt/s, batch=%d)\n",
				sent*100/uint64(count), sent, count, pps, batchSize)
		}
	}
	return sent, time.Since(start)
}

// injectFast sends packets with zero delay for maximum throughput measurement.
func injectFast(fd int, sll *unix.SockaddrLinklayer, pkt []byte, count int, shutdown *atomic.Bool) (uint64, time.Duration) {
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
	}
	return sent, time.Since(start)
}

func main() {
	count := flag.Int("count", 1000, "Number of packets to inject")
	batchSize := flag.Int("batch", 100, "Packets per burst (burst mode only)")
	ifaceName := flag.String("iface", "veth01", "Network interface to inject on")
	srcMACStr := flag.String("src-mac", "02:42:ac:11:00:02", "Source MAC address")
	dstMACStr := flag.String("dst-mac", "02:42:ac:11:00:03", "Destination MAC address")
	flowLabel := flag.Uint("flow-label", DefaultFlowLabel, "IPv6 flow label (identifies Doom instance)")
	mode := flag.String("mode", "burst", "Injection mode: steady, burst, fast")
	delayUS := flag.Int("delay", 3000, "Inter-packet delay in microseconds (steady mode only)")

	flag.Parse()

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
	pktArr := buildPacket(uint32(*flowLabel), srcMAC, dstMAC)
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
	fmt.Printf("  Mode:       %s\n", *mode)
	fmt.Printf("  Count:      %d\n", *count)
	fmt.Printf("  Interface:  %s\n", *ifaceName)
	fmt.Printf("  Flow label: 0x%X\n", *flowLabel)
	switch *mode {
	case "steady":
		fmt.Printf("  Delay:      %d us\n", *delayUS)
	case "burst":
		fmt.Printf("  Batch size: %d\n", *batchSize)
	case "fast":
		fmt.Printf("  Delay:      0 (max throughput)\n")
	}
	fmt.Println()

	// Run the selected injection mode.
	var sent uint64
	var elapsed time.Duration

	switch *mode {
	case "steady":
		sent, elapsed = injectSteady(fd, sll, pkt, *count, *delayUS, &shutdown)
	case "burst":
		sent, elapsed = injectBurst(fd, sll, pkt, *count, *batchSize, &shutdown)
	case "fast":
		sent, elapsed = injectFast(fd, sll, pkt, *count, &shutdown)
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown mode %q (use: steady, burst, fast)\n", *mode)
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
