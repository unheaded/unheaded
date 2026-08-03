#!/usr/bin/env python3
"""
Phase 10 Stress Cannon — Flood IPv6+Monad through Shield eBPF on tap-tomb.

Uses kernel UDP6 sockets with IPV6_HOPOPTS to inject Hop-by-Hop Options
containing Monad register files. Packets flow through the normal kernel
routing path so they hit XDP (ingress on remote) and TC (egress on local).

AF_PACKET/scapy sendp() bypasses TC/XDP hooks — this tool uses kernel sockets.

Usage:
    sudo python3 scripts/stress-cannon.py --rate 1000
    sudo python3 scripts/stress-cannon.py --rate 0          # wire speed
    sudo python3 scripts/stress-cannon.py --mode ramp       # auto-escalate
    sudo python3 scripts/stress-cannon.py --mode baseline   # plain UDP6 (no Monad)
    sudo python3 scripts/stress-cannon.py --threads 4       # multi-socket
"""

import argparse
import os
import socket
import struct
import threading
import time
from collections import deque

# ─── Monad Wire Format (v0x01, 20 bytes) ──────────────────────────────────
# Matches ebpf/monad-common/src/lib.rs
MONAD_VERSION = 0x01
MONAD_SIZE = 20
MONAD_OPT_TYPE = 0x3E  # From monad-common: MONAD_OPT_TYPE (action=00, change-en-route=1)

# Monad flags (from monad-common/src/lib.rs)
FLAG_TRACED  = 0x20  # T bit
FLAG_SAMPLED = 0x08  # S bit
FLAG_CHAOS   = 0x80  # C bit

# HBH header: 2 (fixed) + 2 (TLV header) + 20 (Monad) = 24 bytes = 3 * 8
# HdrExtLen = 3 - 1 = 2
HBH_TOTAL_LEN = 24


def build_monad_register(flow_id=0x0001, flags=FLAG_TRACED | FLAG_SAMPLED,
                         src_svc=0x01, dst_svc=0x02, hop_count=0,
                         ttl=64, seq=0, trace_id=0):
    """Build a 20-byte Monad register file."""
    return struct.pack(
        ">BBBB HBB I Q",
        MONAD_VERSION, src_svc, dst_svc, flags,
        flow_id, hop_count, ttl,
        seq,
        trace_id,
    )


def build_hbh_option(monad_bytes):
    """Build a 24-byte HBH option for IPV6_HOPOPTS setsockopt.

    Layout:
      [0]   Next Header (17 = UDP, kernel may overwrite)
      [1]   Hdr Ext Len = 2 (24 bytes total)
      [2]   Opt Type = 0x3E (MONAD_OPT_TYPE)
      [3]   Opt Data Len = 20
      [4-23] Monad register (20 bytes)
    """
    hbh = bytearray(HBH_TOTAL_LEN)
    hbh[0] = 17            # Next Header = UDP
    hbh[1] = 2             # Hdr Ext Len
    hbh[2] = MONAD_OPT_TYPE  # 0x3E
    hbh[3] = MONAD_SIZE    # 20
    hbh[4:24] = monad_bytes
    return bytes(hbh)


class StressMetrics:
    """Thread-safe send performance tracker."""
    def __init__(self):
        self.total_sent = 0
        self.total_bytes = 0
        self.total_errors = 0
        self.start_time = time.monotonic()
        self._lock = threading.Lock()
        self.window = deque(maxlen=10000)
        self.last_report = time.monotonic()

    def record(self, count, nbytes, errors=0):
        now = time.monotonic()
        with self._lock:
            self.total_sent += count
            self.total_bytes += nbytes
            self.total_errors += errors
            self.window.append((now, count, nbytes))

    def report(self, force=False):
        now = time.monotonic()
        if not force and (now - self.last_report) < 1.0:
            return
        self.last_report = now

        with self._lock:
            elapsed = now - self.start_time
            if elapsed < 0.001:
                return

            cutoff = now - 1.0
            while self.window and self.window[0][0] < cutoff:
                self.window.popleft()

            window_pkts = sum(w[1] for w in self.window)
            window_bytes = sum(w[2] for w in self.window)

            avg_pps = self.total_sent / elapsed
            avg_mbps = (self.total_bytes * 8) / elapsed / 1_000_000
            cur_pps = window_pkts
            cur_mbps = (window_bytes * 8) / 1_000_000
            total = self.total_sent
            errors = self.total_errors

        err_str = f"  err={errors}" if errors > 0 else ""
        print(
            f"\r[{elapsed:7.1f}s] "
            f"sent={total:>10,}  "
            f"cur={cur_pps:>8,} pps ({cur_mbps:>6.1f} Mbps)  "
            f"avg={avg_pps:>8,.0f} pps ({avg_mbps:>6.1f} Mbps)"
            f"{err_str}",
            end="", flush=True
        )

    def final_report(self):
        elapsed = time.monotonic() - self.start_time
        print("\n\n=== Final Stats ===")
        print(f"  Total packets: {self.total_sent:,}")
        print(f"  Total bytes:   {self.total_bytes:,}")
        print(f"  Total errors:  {self.total_errors:,}")
        print(f"  Duration:      {elapsed:.1f}s")
        if elapsed > 0:
            print(f"  Avg rate:      {self.total_sent/elapsed:,.0f} pps")
            print(f"  Avg throughput: {self.total_bytes*8/elapsed/1_000_000:.1f} Mbps")


def create_monad_socket(src_addr, dst_addr, dst_port, flow_id=1, seq=0):
    """Create a UDP6 socket with Monad HBH options set."""
    s = socket.socket(socket.AF_INET6, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 4 * 1024 * 1024)

    monad = build_monad_register(
        flow_id=flow_id,
        flags=FLAG_TRACED | FLAG_SAMPLED,
        seq=seq,
        trace_id=seq,
    )
    hbh = build_hbh_option(monad)
    s.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_HOPOPTS, hbh)

    s.bind((src_addr, 0, 0, 0))
    return s


def update_monad_on_socket(sock, seq, flow_id=1):
    """Update the Monad HBH option with new sequence number."""
    monad = build_monad_register(
        flow_id=flow_id,
        flags=FLAG_TRACED | FLAG_SAMPLED,
        seq=seq,
        trace_id=seq,
    )
    hbh = build_hbh_option(monad)
    sock.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_HOPOPTS, hbh)


def worker_thread(thread_id, args, metrics, stop_event):
    """Send packets in a tight loop."""
    src_addr = args.src_ip
    dst_addr = args.dst_ip
    dst_port = args.port
    rate = args.rate
    batch = args.batch
    monad_mode = args.mode != "baseline"
    update_interval = args.update_seq

    sock = socket.socket(socket.AF_INET6, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 4 * 1024 * 1024)
    sock.bind((src_addr, 0, 0, 0))

    if monad_mode:
        monad = build_monad_register(
            flow_id=thread_id + 1,
            flags=FLAG_TRACED | FLAG_SAMPLED,
            seq=0,
            trace_id=thread_id,
        )
        hbh = build_hbh_option(monad)
        sock.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_HOPOPTS, hbh)

    payload = os.urandom(args.payload)
    pkt_size = args.payload + 40 + (HBH_TOTAL_LEN if monad_mode else 0) + 8  # IPv6 + HBH + UDP
    dest = (dst_addr, dst_port, 0, 0)

    seq = 0
    sent_this_second = 0
    second_start = time.monotonic()

    # Per-thread rate (divide evenly)
    thread_rate = rate // args.threads if rate > 0 else 0

    # Ramp mode state
    if args.mode == "ramp":
        current_rate = args.ramp_start // args.threads
        ramp_last = time.monotonic()
    else:
        current_rate = thread_rate

    while not stop_event.is_set():
        # Ramp: double rate periodically
        if args.mode == "ramp":
            now = time.monotonic()
            if now - ramp_last >= args.ramp_interval:
                current_rate = min(current_rate * 2, 500_000)
                ramp_last = now
                if thread_id == 0:
                    print(f"\n  >>> RAMP: {current_rate * args.threads:,} pps total")

        errors = 0

        # Send batch
        for _ in range(batch):
            try:
                sock.sendto(payload, dest)
                seq += 1
            except OSError:
                errors += 1

        metrics.record(batch - errors, pkt_size * (batch - errors), errors)

        # Update Monad seq periodically (setsockopt is expensive)
        if monad_mode and update_interval > 0 and seq % update_interval == 0:
            update_monad_on_socket(sock, seq, flow_id=thread_id + 1)

        # Rate limiting
        if current_rate > 0:
            now = time.monotonic()
            expected = second_start + (sent_this_second + batch) / current_rate
            if now < expected:
                time.sleep(expected - now)
            sent_this_second += batch
            if now - second_start >= 1.0:
                sent_this_second = 0
                second_start = now

    sock.close()


def read_bpf_stats():
    """Read STATS map via bpftool for comparison."""
    import subprocess
    try:
        out = subprocess.check_output(
            ["bpftool", "map", "dump", "name", "STATS"],
            stderr=subprocess.DEVNULL, timeout=2
        ).decode()
        stats = {}
        for line in out.splitlines():
            if line.startswith("key:"):
                parts = line.split()
                # Little-endian u32 key
                k = int(parts[4] + parts[3] + parts[2] + parts[1], 16)
                # Little-endian u64 value
                v = int(parts[13] + parts[12] + parts[11] + parts[10] +
                        parts[9] + parts[8] + parts[7] + parts[6], 16)
                stats[k] = v
        return stats
    except Exception:
        return {}


STAT_NAMES = {
    0:  "TOTAL",
    1:  "IPV6",
    2:  "BIRTHS",
    3:  "DEATHS",
    4:  "BLOCKED",
    5:  "EXT_STRIPPED",
    6:  "ANOMALIES",
    7:  "RING_DROPS",
    8:  "DST_OPT_PROC",
    9:  "DST_OPT_SKIP",
    10: "DST_OPT_DISCARD",
    11: "DST_OPT_ICMP",
    12: "DST_OPT_ICMP_MC",
    13: "TC_ENTRY",
    14: "TC_IPV6",
    15: "TC_HBH",
}


def run_flood(args):
    """Main flood coordinator."""
    print("=== Phase 10 Stress Cannon (kernel sockets) ===")
    print(f"  Mode:      {args.mode}")
    print(f"  Rate:      {'wire speed' if args.rate == 0 else f'{args.rate} pps'}")
    print(f"  Threads:   {args.threads}")
    print(f"  Batch:     {args.batch}")
    print(f"  Payload:   {args.payload} bytes")
    print(f"  Src IPv6:  {args.src_ip}")
    print(f"  Dst IPv6:  {args.dst_ip}")
    print(f"  Dst Port:  {args.port}")
    if args.mode == "ramp":
        print(f"  Ramp:      {args.ramp_start} pps → doubling every {args.ramp_interval}s")
    monad_mode = args.mode != "baseline"
    pkt_size = args.payload + 40 + (HBH_TOTAL_LEN if monad_mode else 0) + 8
    print(f"  Est pkt:   {pkt_size} bytes ({'IPv6+HBH+UDP' if monad_mode else 'IPv6+UDP'})")

    # Snapshot BPF stats before
    stats_before = read_bpf_stats()
    if stats_before:
        print("\n  BPF STATS (before):")
        for k in sorted(stats_before.keys()):
            name = STAT_NAMES.get(k, f"UNKNOWN_{k}")
            print(f"    {name}: {stats_before[k]:,}")

    print("\nPress Ctrl+C to stop\n")

    metrics = StressMetrics()
    stop_event = threading.Event()
    threads = []

    for i in range(args.threads):
        t = threading.Thread(target=worker_thread, args=(i, args, metrics, stop_event), daemon=True)
        threads.append(t)
        t.start()

    try:
        while True:
            time.sleep(0.1)
            metrics.report()
    except KeyboardInterrupt:
        stop_event.set()
        for t in threads:
            t.join(timeout=2)

    metrics.report(force=True)
    metrics.final_report()

    # Snapshot BPF stats after
    stats_after = read_bpf_stats()
    if stats_after:
        print("\n  BPF STATS (after):")
        for k in sorted(stats_after.keys()):
            name = STAT_NAMES.get(k, f"UNKNOWN_{k}")
            before = stats_before.get(k, 0)
            after = stats_after[k]
            delta = after - before
            print(f"    {name}: {after:,} (+{delta:,})")


def main():
    parser = argparse.ArgumentParser(
        description="Phase 10 Stress Cannon — kernel-routed IPv6+Monad flood"
    )
    parser.add_argument("--rate", type=int, default=1000,
                        help="Total packets/sec across all threads (0=wire speed)")
    parser.add_argument("--threads", type=int, default=1,
                        help="Number of sender threads")
    parser.add_argument("--batch", type=int, default=32,
                        help="Packets per send loop iteration")
    parser.add_argument("--payload", type=int, default=64,
                        help="UDP payload bytes")
    parser.add_argument("--port", type=int, default=31337,
                        help="Destination UDP port")
    parser.add_argument("--src-ip", default="fd00:13:6::1",
                        help="Source IPv6 (ULA on br-tomb)")
    parser.add_argument("--dst-ip", default="fd00:13:6::6",
                        help="Destination IPv6 (Tomb ULA)")
    parser.add_argument("--mode",
                        choices=["ipv6-monad", "baseline", "ramp"],
                        default="ipv6-monad",
                        help="ipv6-monad: Monad HBH, baseline: plain UDP6, ramp: auto-escalate")
    parser.add_argument("--ramp-start", type=int, default=100,
                        help="Ramp mode starting rate (pps)")
    parser.add_argument("--ramp-interval", type=float, default=10.0,
                        help="Seconds between ramp doublings")
    parser.add_argument("--update-seq", type=int, default=1000,
                        help="Update Monad sequence every N packets (0=never, setsockopt is slow)")
    args = parser.parse_args()
    run_flood(args)


if __name__ == "__main__":
    main()
