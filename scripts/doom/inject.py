#!/usr/bin/env python3
"""
Packet injector for Doom-over-IPv6 ring. Run inside monad0 namespace.

Supports steady-state (fixed delay) and burst (Netflix model) injection modes.

Usage:
  sudo ip netns exec monad0 python3 scripts/doom/inject.py
  sudo ip netns exec monad0 python3 scripts/doom/inject.py --count 5000 --delay 1500
  sudo ip netns exec monad0 python3 scripts/doom/inject.py --mode burst --batch-size 100
  sudo ip netns exec monad0 python3 scripts/doom/inject.py --mode fast --count 10000
"""
import argparse
import signal
import socket
import struct
import sys
import time

# Graceful shutdown
_shutdown = False


def _handle_signal(signum, frame):
    global _shutdown
    _shutdown = True
    print(f"\nSignal {signum} received — finishing current batch...")


signal.signal(signal.SIGINT, _handle_signal)
signal.signal(signal.SIGTERM, _handle_signal)


def mac_bytes(mac_str):
    """Parse 'aa:bb:cc:dd:ee:ff' to bytes."""
    return bytes(int(b, 16) for b in mac_str.split(":"))


def build_packet(flow_label, src_mac, dst_mac):
    """Build a 78-byte Doom ring packet (Ethernet + IPv6 + HBH + Monad)."""
    version_tc_fl = (6 << 28) | (flow_label & 0xFFFFF)
    eth = dst_mac + src_mac + struct.pack(">H", 0x86DD)

    src_addr = bytes(
        [0xFD, 0x00, 0x00, 0x3F, 0x00, 0x75, 0x00, 0x00,
         0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01]
    )
    dst_addr = bytes(
        [0xFD, 0x00, 0xDE, 0xAD, 0x00, 0x00, 0x00, 0x00,
         0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01]
    )
    ipv6 = struct.pack(">IHBB", version_tc_fl, 24, 0, 255) + src_addr + dst_addr

    hbh = struct.pack("BBBB", 59, 2, 0x3E, 20)

    # Monad register: exactly 20 bytes
    monad = struct.pack("BBBBBBBB", 0x01, 0, 0, 0, 0, 0, 0, 0x02)  # 8B: header
    monad += struct.pack(">H", 0)           # 2B: latency_hint
    monad += struct.pack("BBBB", 0, 0, 0, 0)  # 4B: deploy_ring, mesh_flags, prefixes
    monad += struct.pack("BBBB", 0, 0, 0, 0)  # 4B: scratch
    monad += struct.pack("BB", 0, 0)           # 2B: checksum
    assert len(monad) == 20, f"Monad size mismatch: {len(monad)}"

    packet = eth + ipv6 + hbh + monad
    assert len(packet) == 78, f"Packet size mismatch: {len(packet)}"
    return packet


def inject_steady(sock, packet, count, delay_us):
    """Steady-state injection with fixed inter-packet delay."""
    delay_s = delay_us / 1_000_000.0
    start = time.time()
    sent = 0

    for i in range(count):
        if _shutdown:
            break
        sock.send(packet)
        sent += 1
        if delay_s > 0:
            time.sleep(delay_s)
        if (i + 1) % 1000 == 0:
            elapsed = time.time() - start
            rate = (i + 1) / elapsed if elapsed > 0 else 0
            insns = (i + 1) * 4080
            print(
                f"  [{(i+1)*100//count:3d}%] {i+1}/{count} pkt "
                f"({rate:.0f} pkt/s, ~{insns:,} insns)",
                flush=True,
            )

    return sent, time.time() - start


def inject_burst(sock, packet, count, batch_size):
    """Netflix-model burst injection: fire batch, brief pause, repeat."""
    start = time.time()
    sent = 0
    batches = 0

    while sent < count and not _shutdown:
        batch = min(batch_size, count - sent)
        for _ in range(batch):
            sock.send(packet)
        sent += batch
        batches += 1

        # Brief drain pause between batches (100us)
        time.sleep(0.0001)

        if batches % 10 == 0 or sent >= count:
            elapsed = time.time() - start
            rate = sent / elapsed if elapsed > 0 else 0
            print(
                f"  [{sent*100//count:3d}%] {sent}/{count} pkt "
                f"({rate:.0f} pkt/s, batch={batch_size})",
                flush=True,
            )

    return sent, time.time() - start


def inject_fast(sock, packet, count):
    """Zero-delay injection — maximum throughput."""
    start = time.time()
    sent = 0

    for i in range(count):
        if _shutdown:
            break
        sock.send(packet)
        sent += 1

    return sent, time.time() - start


def main():
    parser = argparse.ArgumentParser(
        description="Packet injector for Doom-over-IPv6 ring"
    )
    parser.add_argument(
        "--count", "-n", type=int, default=1000, help="Number of packets (default: 1000)"
    )
    parser.add_argument(
        "--flow-label", type=lambda x: int(x, 0), default=0xDE,
        help="IPv6 flow label (default: 0xDE)",
    )
    parser.add_argument(
        "--src-mac", default="02:42:ac:11:00:02", help="Source MAC address"
    )
    parser.add_argument(
        "--dst-mac", default="02:42:ac:11:00:03", help="Destination MAC address"
    )
    parser.add_argument(
        "--iface", default="veth01", help="Interface to inject on (default: veth01)"
    )
    parser.add_argument(
        "--mode",
        choices=["steady", "burst", "fast"],
        default="steady",
        help="Injection mode (default: steady)",
    )
    parser.add_argument(
        "--delay", type=int, default=3000,
        help="Inter-packet delay in microseconds for steady mode (default: 3000)",
    )
    parser.add_argument(
        "--batch-size", type=int, default=100,
        help="Packets per burst for burst mode (default: 100)",
    )
    args = parser.parse_args()

    src_mac = mac_bytes(args.src_mac)
    dst_mac = mac_bytes(args.dst_mac)
    packet = build_packet(args.flow_label, src_mac, dst_mac)

    sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
    try:
        sock.bind((args.iface, 0))
    except OSError as e:
        print(f"ERROR: Cannot bind to {args.iface}: {e}", file=sys.stderr)
        print("Are you running inside a monad namespace?", file=sys.stderr)
        print(f"  sudo ip netns exec monad0 python3 {sys.argv[0]} ...", file=sys.stderr)
        sys.exit(1)

    print(f"=== Doom Ring Injector ===")
    print(f"  Mode: {args.mode}")
    print(f"  Count: {args.count}")
    print(f"  Flow label: 0x{args.flow_label:X}")
    print(f"  Interface: {args.iface}")
    if args.mode == "steady":
        print(f"  Delay: {args.delay}us")
    elif args.mode == "burst":
        print(f"  Batch size: {args.batch_size}")
    print()

    if args.mode == "steady":
        sent, elapsed = inject_steady(sock, packet, args.count, args.delay)
    elif args.mode == "burst":
        sent, elapsed = inject_burst(sock, packet, args.count, args.batch_size)
    else:
        sent, elapsed = inject_fast(sock, packet, args.count)

    sock.close()

    rate = sent / elapsed if elapsed > 0 else 0
    total_insns = sent * 4080
    print(f"\nInjected {sent} packets in {elapsed:.1f}s ({rate:.0f} pkt/s, ~{total_insns:,} insns)")
    if _shutdown:
        print("(interrupted by signal)")


if __name__ == "__main__":
    main()
