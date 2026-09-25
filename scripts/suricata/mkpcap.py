#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
"""Write one pcap per test case for scripts/check-suricata-rules.sh.

Each case is built byte by byte (no scapy), and CASES records which Monad rule
SIDs it must fire, so the expectation sits next to the packets it describes.
The Monad option is type 0x3E, data length 20, as the eBPF parsers match it.

Usage: mkpcap.py <outdir>   # writes <case>.pcap and prints "<case> <sids>"
"""

import os
import struct
import sys

SRC = b"\xfd" + b"\x00" * 14 + b"\x01"
DST = b"\xfd" + b"\x00" * 14 + b"\x02"
ETH = b"\x02" * 6 + b"\x04" * 6 + b"\x86\xdd"

MONAD = bytes([0x3E, 20]) + bytes(range(1, 21))
ROUTER_ALERT = bytes([0x05, 2, 0, 0])
PADN2 = bytes([1, 0])  # PadN with 0 bytes of padding: a 2-byte option


def ipv6(nh, payload):
    return struct.pack("!IHBB", 6 << 28, len(payload), nh, 64) + SRC + DST + payload


def udp(data):
    return struct.pack("!HHHH", 40000, 16666, 8 + len(data), 0) + data


def ext(nh, opts):
    """A Hop-by-Hop or Destination Options header, padded to 8 octets."""
    body = bytes([nh, 0]) + opts
    pad = (-len(body)) % 8
    if pad == 1:
        body += b"\x00"
    elif pad:
        body += bytes([1, pad - 2]) + b"\x00" * (pad - 2)
    return bytes([nh, len(body) // 8 - 1]) + body[2:]


def one(pkt, t=0.0):
    return [(t, pkt)]


def burst(pkt, n, span):
    return [(i * span / n, pkt) for i in range(n)]


monad_pkt = ipv6(0, ext(17, MONAD) + udp(b"hello"))

# case -> (packets, SIDs that must fire; exactly this set)
CASES = {
    "monad": (one(monad_pkt), {9000001}),
    "monad_after_padding": (
        one(ipv6(0, ext(17, PADN2 + MONAD) + udp(b"hello"))),
        {9000001},
    ),
    "monad_oversized": (
        one(ipv6(0, ext(17, MONAD) + udp(b"x" * 1600))),
        {9000001, 9000002},
    ),
    "monad_20_in_4s": (burst(monad_pkt, 20, 4.0), {9000001, 9000031}),
    "monad_1000_in_half_s": (burst(monad_pkt, 1000, 0.5), {9000001, 9000030, 9000031}),
    "other_hbh_option": (one(ipv6(0, ext(17, ROUTER_ALERT) + udp(b"hello"))), set()),
    "plain_ipv6": (one(ipv6(17, udp(b"hello"))), set()),
    "magic_in_udp_payload": (one(ipv6(17, udp(b"xx" + MONAD))), set()),
    "magic_behind_other_hbh": (
        one(ipv6(0, ext(17, ROUTER_ALERT) + udp(b"x" + MONAD))),
        set(),
    ),
    "monad_in_dest_options": (one(ipv6(60, ext(17, MONAD) + udp(b"hello"))), set()),
}


def write(path, packets):
    with open(path, "wb") as f:
        f.write(struct.pack("<IHHiIII", 0xA1B2C3D4, 2, 4, 0, 0, 65535, 1))
        for t, pkt in packets:
            frame = ETH + pkt
            sec = 1700000000 + int(t)
            usec = round((t - int(t)) * 1e6)
            f.write(struct.pack("<IIII", sec, usec, len(frame), len(frame)) + frame)


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: mkpcap.py <outdir>")
    out = sys.argv[1]
    os.makedirs(out, exist_ok=True)
    for name, (packets, sids) in CASES.items():
        write(os.path.join(out, name + ".pcap"), packets)
        print(name, " ".join(str(s) for s in sorted(sids)))


if __name__ == "__main__":
    main()
