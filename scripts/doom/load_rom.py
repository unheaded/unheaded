#!/usr/bin/env python3
"""
Load ROM/RV2MBC binaries into BPF maps for Doom-over-IPv6.

Supports two backends:
  - fast (default): Direct BPF syscall (no subprocess per entry)
  - slow: bpftool CLI per entry (fallback for debugging)

Usage:
  sudo python3 scripts/doom/load_rom.py doom.mbc
  sudo python3 scripts/doom/load_rom.py doom.mbc --rv2mbc rv2mbc.bin
  sudo python3 scripts/doom/load_rom.py doom.mbc --dry-run
  sudo python3 scripts/doom/load_rom.py doom.mbc --backend slow
"""
import argparse
import ctypes
import ctypes.util
import os
import platform
import struct
import subprocess
import sys
import time

# BPF constants
BPF_MAP_UPDATE_ELEM = 2
BPF_OBJ_GET = 7

# Default map paths
DEFAULT_ROM_MAP = "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP"
DEFAULT_RV2MBC_MAP = "/sys/fs/bpf/unheaded/doom-ring/maps/RV2MBC_MAP"

# Architecture → BPF syscall number
ARCH_SYSCALL = {
    "aarch64": 280,
    "x86_64": 321,
    "AMD64": 321,
}


def get_sys_bpf():
    """Resolve BPF syscall number for current architecture."""
    arch = platform.machine()
    if arch not in ARCH_SYSCALL:
        raise RuntimeError(f"Unsupported architecture: {arch}")
    return ARCH_SYSCALL[arch]


class FastLoader:
    """Load entries via direct BPF syscall — ~100x faster than bpftool."""

    def __init__(self):
        self.sys_bpf = get_sys_bpf()
        self.libc = ctypes.CDLL(ctypes.util.find_library("c"), use_errno=True)

    def _bpf(self, cmd, attr_buf, attr_size):
        r = self.libc.syscall(
            ctypes.c_long(self.sys_bpf),
            ctypes.c_int(cmd),
            ctypes.byref(attr_buf),
            ctypes.c_uint(attr_size),
        )
        if r < 0:
            e = ctypes.get_errno()
            raise OSError(e, os.strerror(e))
        return r

    def open_pinned(self, path):
        path_b = path.encode("utf-8") + b"\x00"
        path_buf = ctypes.create_string_buffer(path_b)
        attr = (ctypes.c_char * 120)()
        struct.pack_into("=Q", attr, 0, ctypes.addressof(path_buf))
        return self._bpf(BPF_OBJ_GET, attr, 120)

    def load(self, map_path, data, entry_size=4, label="entries"):
        n = len(data) // entry_size
        if n == 0:
            print(f"  WARNING: No {label} to load (0 bytes)")
            return 0, 0

        fd = self.open_pinned(map_path)
        print(f"  {map_path} fd={fd}")
        print(f"  Loading {n} {label} ({len(data)} bytes) ...")

        key_buf = (ctypes.c_char * 4)()
        val_buf = (ctypes.c_char * entry_size)()
        attr = (ctypes.c_char * 120)()
        struct.pack_into("=I", attr, 0, fd)
        struct.pack_into("=Q", attr, 8, ctypes.addressof(key_buf))
        struct.pack_into("=Q", attr, 16, ctypes.addressof(val_buf))
        struct.pack_into("=Q", attr, 24, 0)

        start = time.time()
        errors = 0
        for i in range(n):
            struct.pack_into("<I", key_buf, 0, i)
            ctypes.memmove(val_buf, data[i * entry_size : (i + 1) * entry_size], entry_size)
            try:
                self._bpf(BPF_MAP_UPDATE_ELEM, attr, 120)
            except OSError as e:
                errors += 1
                if errors <= 5:
                    print(f"    Error at [{i}]: {e}")
                if errors == 6:
                    print("    (suppressing further errors)")

            if (i + 1) % 20000 == 0 or i == n - 1:
                elapsed = time.time() - start
                rate = (i + 1) / elapsed if elapsed > 0 else 0
                pct = ((i + 1) * 100) // n
                print(f"    [{pct:3d}%] {i+1}/{n} ({rate:.0f} entries/s, {elapsed:.1f}s)")

        os.close(fd)
        elapsed = time.time() - start
        rate = n / elapsed if elapsed > 0 else 0
        print(f"  Done: {n} {label}, {errors} errors, {elapsed:.1f}s ({rate:.0f}/s)")
        return n, errors


class SlowLoader:
    """Load entries via bpftool CLI — slow but useful for debugging."""

    def load(self, map_path, data, entry_size=4, label="entries"):
        n = len(data) // entry_size
        if n == 0:
            print(f"  WARNING: No {label} to load (0 bytes)")
            return 0, 0

        print(f"  Loading {n} {label} ({len(data)} bytes) via bpftool ...")
        start = time.time()
        errors = 0
        for i in range(n):
            word = struct.unpack_from("<I", data, i * entry_size)[0]
            val_bytes = list(word.to_bytes(4, "little"))
            key_bytes = list(i.to_bytes(4, "little"))
            cmd = (
                ["sudo", "bpftool", "map", "update", "pinned", map_path, "key"]
                + [str(x) for x in key_bytes]
                + ["value"]
                + [str(x) for x in val_bytes]
            )
            result = subprocess.run(cmd, capture_output=True, text=True)
            if result.returncode != 0:
                errors += 1
                if errors <= 5:
                    print(f"    Error at [{i}]: {result.stderr.strip()}")

            if (i + 1) % 5000 == 0 or i == n - 1:
                elapsed = time.time() - start
                rate = (i + 1) / elapsed if elapsed > 0 else 0
                pct = ((i + 1) * 100) // n
                print(f"    [{pct:3d}%] {i+1}/{n} ({rate:.0f} entries/s, {elapsed:.1f}s)")

        elapsed = time.time() - start
        rate = n / elapsed if elapsed > 0 else 0
        print(f"  Done: {n} {label}, {errors} errors, {elapsed:.1f}s ({rate:.0f}/s)")
        return n, errors


def main():
    parser = argparse.ArgumentParser(
        description="Load ROM/RV2MBC binaries into BPF maps for Doom-over-IPv6"
    )
    parser.add_argument("rom", help="Path to ROM .mbc binary")
    parser.add_argument("--rv2mbc", help="Path to RV2MBC translation table binary")
    parser.add_argument(
        "--rom-map", default=DEFAULT_ROM_MAP, help="BPF map pin path for ROM"
    )
    parser.add_argument(
        "--rv2mbc-map", default=DEFAULT_RV2MBC_MAP, help="BPF map pin path for RV2MBC"
    )
    parser.add_argument(
        "--backend",
        choices=["fast", "slow"],
        default="fast",
        help="Loader backend (default: fast = direct BPF syscall)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Validate files without loading into BPF maps",
    )
    parser.add_argument(
        "--entry-size", type=int, default=4, help="Bytes per entry (default: 4)"
    )
    args = parser.parse_args()

    # Validate ROM file
    if not os.path.isfile(args.rom):
        print(f"ERROR: ROM file not found: {args.rom}", file=sys.stderr)
        sys.exit(1)

    with open(args.rom, "rb") as f:
        rom_data = f.read()

    rom_n = len(rom_data) // args.entry_size
    print(f"=== ROM: {args.rom} ({rom_n} instructions, {len(rom_data)} bytes) ===")

    rv2mbc_data = None
    if args.rv2mbc:
        if not os.path.isfile(args.rv2mbc):
            print(f"ERROR: RV2MBC file not found: {args.rv2mbc}", file=sys.stderr)
            sys.exit(1)
        with open(args.rv2mbc, "rb") as f:
            rv2mbc_data = f.read()
        rv2mbc_n = len(rv2mbc_data) // args.entry_size
        print(f"=== RV2MBC: {args.rv2mbc} ({rv2mbc_n} translations, {len(rv2mbc_data)} bytes) ===")

    if args.dry_run:
        print("\n[DRY RUN] Files validated. No BPF maps modified.")
        return

    # Verify map paths exist
    if not os.path.exists(args.rom_map):
        print(f"ERROR: ROM map not pinned at {args.rom_map}", file=sys.stderr)
        print("Is the doom ring active? Run: sudo ./scripts/doom/ring.sh setup", file=sys.stderr)
        sys.exit(1)

    loader = FastLoader() if args.backend == "fast" else SlowLoader()

    print(f"\n--- Loading ROM ({args.backend} backend) ---")
    rom_loaded, rom_errors = loader.load(args.rom_map, rom_data, args.entry_size, "instructions")

    rv2mbc_loaded, rv2mbc_errors = 0, 0
    if rv2mbc_data:
        if not os.path.exists(args.rv2mbc_map):
            print(f"ERROR: RV2MBC map not pinned at {args.rv2mbc_map}", file=sys.stderr)
            sys.exit(1)
        print(f"\n--- Loading RV2MBC ({args.backend} backend) ---")
        rv2mbc_loaded, rv2mbc_errors = loader.load(
            args.rv2mbc_map, rv2mbc_data, args.entry_size, "translations"
        )

    print("\n=== SUMMARY ===")
    print(f"  ROM: {rom_loaded} instructions loaded ({rom_errors} errors)")
    if rv2mbc_data:
        print(f"  RV2MBC: {rv2mbc_loaded} translations loaded ({rv2mbc_errors} errors)")

    total_errors = rom_errors + rv2mbc_errors
    if total_errors > 0:
        print(f"\nWARNING: {total_errors} total errors during load", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
