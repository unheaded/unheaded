#!/usr/bin/env python3
"""
doom-tick.py — Continuous tick injector for Doom-over-IPv6.

Injects Monad CPU tick packets at high rate into the doom ring.
Each packet circulates through 6 namespaces, executing N MBC instructions
per hop. With hop_limit=255 and N=16, each packet yields ~4,080 instructions.

Must be run inside monad0 namespace:
  sudo ip netns exec monad0 python3 scripts/doom-tick.py [--count N] [--rate HZ]
"""

import socket
import struct
import sys
import time
import argparse
import fcntl
import os


def get_mac(iface):
    """Get MAC address of an interface via ioctl."""
    SIOCGIFHWADDR = 0x8927
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    info = fcntl.ioctl(sock.fileno(), SIOCGIFHWADDR,
                       struct.pack('256s', iface.encode('utf-8')[:15]))
    sock.close()
    return info[18:24]


def build_tick_packet(src_mac, dst_mac, flow_label=0, hop_limit=255):
    """Build a raw Ethernet + IPv6 + HBH + Monad tick packet."""
    # Ethernet header
    eth = dst_mac + src_mac + struct.pack('!H', 0x86DD)

    # IPv6 header
    version_tc_fl = (6 << 28) | (0 << 20) | (flow_label & 0xFFFFF)
    payload_len = 24  # HBH extension header
    next_header = 0   # Hop-by-Hop
    ipv6 = struct.pack('!IHBB', version_tc_fl, payload_len, next_header, hop_limit)

    # Source: fd00:3f:75:0::1
    src_addr = bytes([0xfd, 0x00, 0x00, 0x3f, 0x00, 0x75, 0x00, 0x00,
                      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01])
    # Destination: fd00:dead::1 (outside all connected /64 prefixes)
    dst_addr = bytes([0xfd, 0x00, 0xde, 0xad, 0x00, 0x00, 0x00, 0x00,
                      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01])
    ipv6 += src_addr + dst_addr

    # HBH header: next_header=59 (No Next Header), hdr_ext_len=2, opt_type=0x3E, opt_len=20
    hbh_prefix = struct.pack('BBBB', 59, 2, 0x3E, 20)

    # Monad: version=1, flags=0x02 (CUSTOM), rest zeroed
    monad = struct.pack('BBBBBBBB', 0x01, 0, 0, 0, 0, 0, 0, 0x02)
    monad += struct.pack('!H', 0)      # latency_hint
    monad += struct.pack('BBBB', 0, 0, 0, 0)  # deploy_ring, mesh_flags, prefixes
    monad += struct.pack('BBBB', 0, 0, 0, 0)  # scratch
    monad += struct.pack('BB', 0, 0)           # checksum

    return eth + ipv6 + hbh_prefix + monad


def main():
    parser = argparse.ArgumentParser(description='Doom tick injector')
    parser.add_argument('--count', type=int, default=1000,
                        help='Number of tick packets to inject (default: 1000)')
    parser.add_argument('--rate', type=float, default=0,
                        help='Target injection rate in Hz (0=max speed, default: 0)')
    parser.add_argument('--hop-limit', type=int, default=255,
                        help='IPv6 hop limit (default: 255)')
    parser.add_argument('--flow-label', type=int, default=0,
                        help='Flow label (low 8 bits = instance_id, default: 0)')
    parser.add_argument('--iface', type=str, default='veth01',
                        help='Interface to inject on (default: veth01)')
    parser.add_argument('--insn-per-tick', type=int, default=16,
                        help='MAX_INSN_PER_TICK for estimate (default: 16)')
    args = parser.parse_args()

    # Get real MAC addresses
    src_mac = get_mac(args.iface)
    # Get peer MAC: the peer is in the next namespace, named <iface>p
    # Use neighbor table or just get from the other side
    # For veth pairs, we can get the peer MAC by reading the neighbor table
    # Simpler: just use the src_mac as dst_mac (veth delivers regardless of dst MAC
    # as long as it's unicast)
    # Actually we need the real peer MAC for proper forwarding
    # Let's read it from the neighbor table
    import subprocess
    peer_iface = args.iface + 'p'
    result = subprocess.run(
        ['ip', 'link', 'show', args.iface],
        capture_output=True, text=True
    )
    # Try to get peer's MAC from neighbor discovery
    # For now, use a simpler approach: the packet just needs a valid unicast dst MAC
    # The veth driver delivers based on the link, not MAC filtering
    # But the kernel's IPv6 forwarding checks the dst MAC matches the interface MAC
    # So we need the actual MAC of veth01 (our interface) as src, and veth01p (peer) as dst
    # Since we're in monad0 and can't see monad1's interfaces directly,
    # let's use the neighbor table
    result = subprocess.run(
        ['ip', '-6', 'neigh', 'show', 'dev', args.iface],
        capture_output=True, text=True
    )
    dst_mac = None
    for line in result.stdout.strip().split('\n'):
        if 'lladdr' in line:
            parts = line.split()
            idx = parts.index('lladdr')
            mac_str = parts[idx + 1]
            dst_mac = bytes(int(b, 16) for b in mac_str.split(':'))
            break

    if dst_mac is None:
        # Fallback: ping the peer to populate neighbor table, then retry
        subprocess.run(['ping6', '-c', '1', '-W', '1', 'fd00:3f:75:0::2'],
                      capture_output=True, timeout=3)
        result = subprocess.run(
            ['ip', '-6', 'neigh', 'show', 'dev', args.iface],
            capture_output=True, text=True
        )
        for line in result.stdout.strip().split('\n'):
            if 'lladdr' in line:
                parts = line.split()
                idx = parts.index('lladdr')
                mac_str = parts[idx + 1]
                dst_mac = bytes(int(b, 16) for b in mac_str.split(':'))
                break

    if dst_mac is None:
        print("[TICK] WARNING: Could not determine peer MAC, using broadcast")
        dst_mac = b'\xff\xff\xff\xff\xff\xff'

    print(f"[TICK] src_mac={src_mac.hex(':')}, dst_mac={dst_mac.hex(':')}")

    insns_per_pkt = args.hop_limit * args.insn_per_tick
    packet = build_tick_packet(src_mac, dst_mac, args.flow_label, args.hop_limit)

    sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
    sock.bind((args.iface, 0))

    interval = 1.0 / args.rate if args.rate > 0 else 0

    print(f"[TICK] Injecting {args.count} packets (hop_limit={args.hop_limit}, "
          f"~{args.count * insns_per_pkt:,} instructions)")

    start = time.time()
    for i in range(args.count):
        sock.send(packet)
        if interval > 0:
            time.sleep(interval)
        if (i + 1) % 100 == 0:
            elapsed = time.time() - start
            rate = (i + 1) / elapsed if elapsed > 0 else 0
            est_insns = (i + 1) * insns_per_pkt
            print(f"\r[TICK] {i+1}/{args.count} packets ({rate:.0f} pkt/s, "
                  f"~{est_insns:,} instructions)", end='', flush=True)

    elapsed = time.time() - start
    total_insns = args.count * insns_per_pkt
    rate = args.count / elapsed if elapsed > 0 else 0
    print(f"\n[TICK] Done: {args.count} packets in {elapsed:.2f}s "
          f"({rate:.0f} pkt/s, ~{total_insns:,} estimated instructions)")

    sock.close()


if __name__ == '__main__':
    main()
