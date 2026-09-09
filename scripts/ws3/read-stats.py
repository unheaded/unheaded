#!/usr/bin/env python3
"""Read STATS map counters from the Doom ring BPF program.

Reads both the standard counters (keys 0-10) and the Phase 2 bounce
timestamp counters (keys 11-15) if available.

Usage:
    sudo python3 scripts/ws3/read-stats.py
    sudo python3 scripts/ws3/read-stats.py --bounce-only
    sudo python3 scripts/ws3/read-stats.py --json

Requires:
    - BPF maps pinned at /sys/fs/bpf/unheaded/doom-ring/maps/STATS
    - Root privileges
"""
import argparse
import json
import os
import struct
import sys

# Try ctypes-based BPF map access.
try:
    import ctypes
    import ctypes.util
    HAS_CTYPES = True
except ImportError:
    HAS_CTYPES = False

# STATS map key definitions (from ebpf/monad-cpu-ebpf/src/main.rs).
STAT_KEYS = {
    0: "PACKETS_TOTAL",
    1: "CPU_TICKS",
    2: "INSNS_EXECUTED",
    3: "HALTED",
    4: "SLEEPING",
    5: "NO_STATE",
    6: "MEM_FAULTS",
    7: "SYSCALLS",
    8: "ROM_FAULT",
    9: "MEM_STORES",
    10: "MEM_LOADS",
}

# Phase 2 bounce timestamp keys (proposed extension).
BOUNCE_KEYS = {
    11: "LAST_BOUNCE_NS",
    12: "FIRST_BOUNCE_NS",
    13: "BOUNCE_COUNT",
    14: "MAX_BOUNCE_NS",
    15: "AVG_BOUNCE_NS",
}

BPF_MAP_LOOKUP_ELEM = 1
BPF_OBJ_GET = 7
STATS_MAP_PATH = "/sys/fs/bpf/unheaded/doom-ring/maps/STATS"


class BPFHelper:
    """Minimal BPF map access via raw syscalls."""

    def __init__(self):
        if not HAS_CTYPES:
            raise RuntimeError("ctypes not available")
        lib_name = ctypes.util.find_library("c")
        if lib_name is None:
            raise RuntimeError("libc not found")
        self.libc = ctypes.CDLL(lib_name, use_errno=True)

    def open_pinned(self, path):
        """Open a pinned BPF map by path, returning a file descriptor."""
        path_b = path.encode("utf-8") + b"\x00"
        path_buf = ctypes.create_string_buffer(path_b)
        attr = (ctypes.c_char * 120)()
        struct.pack_into("=Q", attr, 0, ctypes.addressof(path_buf))
        fd = self.libc.syscall(321, BPF_OBJ_GET, ctypes.byref(attr), 120)
        if fd < 0:
            errno = ctypes.get_errno()
            raise OSError(errno, f"bpf(OBJ_GET, {path}): {os.strerror(errno)}")
        return fd

    def lookup(self, fd, key_id, value_size=8):
        """Look up a u32 key in the map, returning raw bytes."""
        key_bytes = struct.pack("<I", key_id)
        key_buf = (ctypes.c_char * 4)(*key_bytes)
        val_buf = (ctypes.c_char * value_size)()
        attr = (ctypes.c_char * 120)()
        struct.pack_into("=I", attr, 0, fd)
        struct.pack_into("=Q", attr, 8, ctypes.addressof(key_buf))
        struct.pack_into("=Q", attr, 16, ctypes.addressof(val_buf))
        ret = self.libc.syscall(321, BPF_MAP_LOOKUP_ELEM, ctypes.byref(attr), 120)
        if ret != 0:
            return None
        return bytes(val_buf)


def read_all_stats(bpf, fd, keys):
    """Read all stat counters, returning {name: value} dict."""
    results = {}
    for key_id, name in sorted(keys.items()):
        raw = bpf.lookup(fd, key_id)
        if raw is not None:
            value = struct.unpack("<Q", raw)[0]
            results[name] = value
        else:
            results[name] = None
    return results


def format_ns(ns):
    """Format nanoseconds as human-readable."""
    if ns is None:
        return "(not set)"
    if ns == 0:
        return "0"
    if ns < 1000:
        return f"{ns} ns"
    if ns < 1_000_000:
        return f"{ns:,} ns ({ns/1e3:.1f} us)"
    return f"{ns:,} ns ({ns/1e6:.2f} ms)"


def main():
    parser = argparse.ArgumentParser(description="Read Doom ring STATS map")
    parser.add_argument("--bounce-only", action="store_true",
                        help="Only show bounce timestamp keys (11-15)")
    parser.add_argument("--json", action="store_true",
                        help="Output as JSON")
    parser.add_argument("--map-path", default=STATS_MAP_PATH,
                        help="Path to pinned STATS map")
    args = parser.parse_args()

    if not os.path.exists(args.map_path):
        print(f"ERROR: STATS map not found at {args.map_path}", file=sys.stderr)
        print("Is the doom ring deployed? Run: sudo scripts/doom-ring.sh", file=sys.stderr)
        sys.exit(1)

    try:
        bpf = BPFHelper()
    except RuntimeError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)

    fd = bpf.open_pinned(args.map_path)

    results = {}
    if not args.bounce_only:
        results.update(read_all_stats(bpf, fd, STAT_KEYS))
    results.update(read_all_stats(bpf, fd, BOUNCE_KEYS))

    os.close(fd)

    if args.json:
        # Replace None with -1 for JSON serialization.
        json_results = {k: (v if v is not None else -1) for k, v in results.items()}
        print(json.dumps(json_results, indent=2))
        return

    # Human-readable output.
    if not args.bounce_only:
        print("=== Doom Ring STATS ===\n")
        for key_id in sorted(STAT_KEYS.keys()):
            name = STAT_KEYS[key_id]
            val = results.get(name)
            if val is not None:
                print(f"  [{key_id:2d}] {name:20s}: {val:>15,}")
            else:
                print(f"  [{key_id:2d}] {name:20s}: (error)")
        print()

    print("=== Bounce Cycle Measurements ===\n")
    for key_id in sorted(BOUNCE_KEYS.keys()):
        name = BOUNCE_KEYS[key_id]
        val = results.get(name)
        if val is not None:
            if key_id in (14, 15):  # Timing values in nanoseconds.
                print(f"  [{key_id:2d}] {name:20s}: {format_ns(val)}")
            else:
                print(f"  [{key_id:2d}] {name:20s}: {val:>15,}")
        else:
            print(f"  [{key_id:2d}] {name:20s}: (not available)")

    # Derived metrics.
    packets = results.get("PACKETS_TOTAL")
    insns = results.get("INSNS_EXECUTED")
    if packets and insns and packets > 0:
        print("\nDerived metrics:")
        print(f"  Insns per packet: {insns / packets:.0f}")
        max_bounce = results.get("MAX_BOUNCE_NS")
        if max_bounce and max_bounce > 0:
            print(f"  Max bounce cycle: {format_ns(max_bounce)}")
            theoretical_pps = 1_000_000_000 / max_bounce
            print(f"  Theoretical max PPS (from bounce): {theoretical_pps:.0f}")


if __name__ == "__main__":
    main()
