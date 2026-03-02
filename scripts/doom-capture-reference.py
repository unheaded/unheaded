#!/usr/bin/env python3
"""doom-capture-reference.py — Capture and compare golden reference frames.

Modes:
  capture: Read SCREEN_MAP + CPU state, save screen.bin + screen.ppm + meta.json
  compare: Read SCREEN_MAP, diff against reference, report pixel difference %

Storage: tests/doom-reference/<name>/
  screen.bin   — raw 64000-byte screen data
  screen.ppm   — PPM image with DOOM palette
  meta.json    — CPU state, build hash, pixel stats

Usage:
  sudo python3 scripts/doom-capture-reference.py capture [--ref-dir DIR] [--name NAME]
  sudo python3 scripts/doom-capture-reference.py compare [--ref-dir DIR] [--name NAME] \
       [--tolerance 10.0]

Exit: 0 if compare passes within tolerance, 1 if too different

Dependencies: bpftool, python3 standard library, hashlib
"""

import argparse
import hashlib
import json
import os
import struct
import subprocess
import sys
import time

# ── Constants ────────────────────────────────────────────────────────────────

SCREEN_WIDTH = 320
SCREEN_HEIGHT = 200
SCREEN_SIZE = SCREEN_WIDTH * SCREEN_HEIGHT  # 64000

MAP_PIN_DIR = "/sys/fs/bpf/unheaded/doom-ring/maps"
CPU_MAP_PIN = f"{MAP_PIN_DIR}/CPU_MAP"
SCREEN_MAP_PIN = f"{MAP_PIN_DIR}/SCREEN_MAP"

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.dirname(SCRIPT_DIR)
DEFAULT_REF_DIR = os.path.join(PROJECT_ROOT, "tests", "doom-reference")

# DOOM ELF paths for build hash
DOOM_ELF_PATHS = [
    os.path.join(PROJECT_ROOT, "doom", "doomgeneric", "doomgeneric", "doom.elf"),
    "/tmp/doom-build/doom.elf",
]

# ── DOOM PLAYPAL — 256 RGB entries from doom1.wad ───────────────────────────

DOOM_PALETTE = [
    0,0,0, 31,23,11, 23,15,7, 75,75,75, 255,255,255, 27,27,27, 19,19,19, 11,11,11,
    7,7,7, 47,55,31, 35,43,15, 23,31,7, 15,23,0, 79,59,43, 71,51,35, 63,43,27,
    255,183,183, 247,171,171, 243,163,163, 235,151,151, 231,143,143, 223,135,135, 219,123,123, 211,115,115,
    203,107,107, 199,99,99, 191,91,91, 187,87,87, 179,79,79, 175,71,71, 167,63,63, 163,59,59,
    155,51,51, 151,47,47, 143,43,43, 139,35,35, 131,31,31, 127,27,27, 119,23,23, 115,19,19,
    107,15,15, 103,11,11, 95,7,7, 91,7,7, 83,7,7, 79,0,0, 71,0,0, 67,0,0,
    255,235,223, 255,227,211, 255,219,199, 255,211,187, 255,207,179, 255,199,167, 255,191,155, 255,187,147,
    255,179,131, 247,171,123, 239,163,115, 231,155,107, 223,147,99, 215,139,91, 207,131,83, 203,127,79,
    191,123,75, 179,115,71, 171,111,67, 163,107,63, 155,99,59, 143,95,55, 135,87,51, 127,83,47,
    119,79,43, 107,71,39, 95,67,35, 83,63,31, 75,55,27, 63,47,23, 51,43,19, 43,35,15,
    239,239,239, 231,231,231, 223,223,223, 219,219,219, 211,211,211, 203,203,203, 199,199,199, 191,191,191,
    183,183,183, 179,179,179, 171,171,171, 167,167,167, 159,159,159, 151,151,151, 147,147,147, 139,139,139,
    131,131,131, 127,127,127, 119,119,119, 111,111,111, 107,107,107, 99,99,99, 91,91,91, 87,87,87,
    79,79,79, 71,71,71, 67,67,67, 59,59,59, 55,55,55, 47,47,47, 39,39,39, 35,35,35,
    119,255,111, 111,239,103, 103,223,95, 95,207,87, 91,191,79, 83,175,71, 75,159,63, 67,147,55,
    63,131,47, 55,115,43, 47,99,35, 39,83,27, 31,67,23, 23,51,15, 19,35,11, 11,23,7,
    191,167,143, 183,159,135, 175,151,127, 167,143,119, 159,135,111, 155,127,107, 147,123,99, 139,115,91,
    131,107,87, 123,99,79, 119,95,75, 111,87,67, 103,83,63, 95,75,55, 87,67,51, 83,63,47,
    159,131,99, 143,119,83, 131,107,75, 119,95,63, 103,83,51, 91,71,43, 79,59,35, 67,51,27,
    123,127,99, 111,115,87, 103,107,79, 91,99,71, 83,87,59, 71,79,51, 63,71,43, 55,63,39,
    255,255,115, 235,219,87, 215,187,67, 195,155,47, 175,123,31, 155,91,19, 135,67,7, 115,43,0,
    255,255,255, 255,219,219, 255,187,187, 255,155,155, 255,123,123, 255,95,95, 255,63,63, 255,31,31,
    255,0,0, 239,0,0, 227,0,0, 215,0,0, 203,0,0, 191,0,0, 179,0,0, 167,0,0,
    155,0,0, 139,0,0, 127,0,0, 115,0,0, 103,0,0, 91,0,0, 79,0,0, 67,0,0,
    231,231,255, 199,199,255, 171,171,255, 143,143,255, 115,115,255, 83,83,255, 55,55,255, 27,27,255,
    0,0,255, 0,0,227, 0,0,203, 0,0,179, 0,0,155, 0,0,131, 0,0,107, 0,0,83,
    255,255,255, 255,235,219, 255,215,187, 255,199,155, 255,179,123, 255,163,91, 255,143,59, 255,127,27,
    243,115,23, 235,111,15, 223,103,15, 215,95,11, 203,87,7, 195,79,0, 183,71,0, 175,67,0,
    255,255,255, 255,255,215, 255,255,179, 255,255,143, 255,255,107, 255,255,71, 255,255,35, 255,255,0,
    167,63,0, 159,55,0, 147,47,0, 135,35,0, 79,59,39, 67,47,27, 55,35,19, 47,27,11,
    0,0,83, 0,0,71, 0,0,59, 0,0,47, 0,0,35, 0,0,23, 0,0,11, 0,0,0,
    255,159,67, 255,231,75, 255,123,255, 255,0,255, 207,0,207, 159,0,155, 111,0,107, 167,107,107,
]


# ── BPF Map Reading ──────────────────────────────────────────────────────────

def read_screen_map():
    """Read SCREEN_MAP from BPF maps via bpftool batch dump."""
    if not os.path.exists(SCREEN_MAP_PIN):
        print(f"ERROR: SCREEN_MAP not pinned at {SCREEN_MAP_PIN}", file=sys.stderr)
        sys.exit(1)

    result = subprocess.run(
        ["bpftool", "map", "dump", "pinned", SCREEN_MAP_PIN],
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode != 0:
        print(f"ERROR: bpftool dump failed: {result.stderr.strip()}", file=sys.stderr)
        sys.exit(1)

    pixels = bytearray(SCREEN_SIZE)
    for line in result.stdout.splitlines():
        line = line.strip()
        if not line or line.startswith("Found"):
            continue
        if "key:" in line and "value:" in line:
            try:
                key_part = line.split("key:")[1].split("value:")[0].strip()
                val_part = line.split("value:")[1].strip()
                key_bytes = bytes(int(b, 16) for b in key_part.split())
                idx = struct.unpack("<I", key_bytes)[0]
                val = int(val_part.split()[0], 16)
                if idx < SCREEN_SIZE:
                    pixels[idx] = val
            except (ValueError, IndexError, struct.error):
                continue

    return bytes(pixels)


def read_cpu_state():
    """Read CPU state from CPU_MAP at key 0xDE."""
    if not os.path.exists(CPU_MAP_PIN):
        return None

    result = subprocess.run(
        ["bpftool", "map", "lookup", "pinned", CPU_MAP_PIN,
         "key", "hex", "de", "00", "00", "00"],
        capture_output=True, text=True, timeout=10,
    )
    if result.returncode != 0:
        return None

    hex_bytes = []
    in_value = False
    for line in result.stdout.strip().split("\n"):
        if "value:" in line:
            in_value = True
            parts = line.split("value:")[1].strip().split()
            hex_bytes.extend(parts)
        elif in_value and line.strip() and not line.startswith("key:") and not line.startswith("Found"):
            hex_bytes.extend(line.strip().split())

    if len(hex_bytes) < 104:
        return None

    bs = bytes(int(b, 16) for b in hex_bytes[:104])
    regs = struct.unpack_from("<16I", bs, 0)
    pc = struct.unpack_from("<I", bs, 64)[0]
    flags, halted, stalled, _ = struct.unpack_from("<BBBB", bs, 68)
    insn_count = struct.unpack_from("<Q", bs, 80)[0]

    return {
        "pc": pc,
        "sp": regs[15],
        "halted": halted,
        "stalled": stalled,
        "insn_count": insn_count,
        "flags": flags,
        "regs": list(regs),
    }


# ── Helpers ──────────────────────────────────────────────────────────────────

def compute_build_hash():
    """SHA256 of doom.elf for build identification."""
    for path in DOOM_ELF_PATHS:
        if os.path.exists(path):
            h = hashlib.sha256()
            with open(path, "rb") as f:
                for chunk in iter(lambda: f.read(8192), b""):
                    h.update(chunk)
            return h.hexdigest()
    return "unknown"


def pixel_stats(pixels):
    """Compute pixel statistics."""
    nonzero = sum(1 for p in pixels if p != 0)
    hist = [0] * 256
    for p in pixels:
        hist[p] += 1

    # Top 10 indices
    top = sorted(range(256), key=lambda i: hist[i], reverse=True)[:10]

    return {
        "nonzero_count": nonzero,
        "nonzero_pct": round(nonzero / SCREEN_SIZE * 100.0, 2),
        "top_indices": [{"index": i, "count": hist[i]} for i in top],
        "histogram": hist,
    }


def save_ppm(pixels, path):
    """Save screen as PPM image using DOOM palette."""
    with open(path, "wb") as f:
        f.write(f"P6\n{SCREEN_WIDTH} {SCREEN_HEIGHT}\n255\n".encode())
        for p in pixels:
            idx = p * 3
            f.write(bytes([
                DOOM_PALETTE[idx],
                DOOM_PALETTE[idx + 1],
                DOOM_PALETTE[idx + 2],
            ]))


# ── Capture Mode ─────────────────────────────────────────────────────────────

def do_capture(ref_dir, name):
    """Capture golden reference frame."""
    out_dir = os.path.join(ref_dir, name)
    os.makedirs(out_dir, exist_ok=True)

    print(f"Capturing reference frame '{name}'...")

    # Read screen
    pixels = read_screen_map()
    stats = pixel_stats(pixels)

    # Read CPU state
    cpu = read_cpu_state()

    # Build hash
    build_hash = compute_build_hash()

    # Save screen.bin
    bin_path = os.path.join(out_dir, "screen.bin")
    with open(bin_path, "wb") as f:
        f.write(pixels)

    # Save screen.ppm
    ppm_path = os.path.join(out_dir, "screen.ppm")
    save_ppm(pixels, ppm_path)

    # Save meta.json
    meta = {
        "name": name,
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "build_hash": build_hash,
        "screen": {
            "width": SCREEN_WIDTH,
            "height": SCREEN_HEIGHT,
            "size": SCREEN_SIZE,
            "nonzero_pct": stats["nonzero_pct"],
            "nonzero_count": stats["nonzero_count"],
            "top_indices": stats["top_indices"],
        },
    }

    if cpu:
        meta["cpu"] = {
            "pc": cpu["pc"],
            "sp": cpu["sp"],
            "halted": cpu["halted"],
            "insn_count": cpu["insn_count"],
        }

    meta_path = os.path.join(out_dir, "meta.json")
    with open(meta_path, "w") as f:
        json.dump(meta, f, indent=2)

    print(f"  screen.bin: {len(pixels)} bytes")
    print(f"  screen.ppm: {os.path.getsize(ppm_path)} bytes")
    print(f"  meta.json:  {os.path.getsize(meta_path)} bytes")
    print(f"  Non-zero pixels: {stats['nonzero_count']}/{SCREEN_SIZE} ({stats['nonzero_pct']}%)")
    print(f"  Build hash: {build_hash[:16]}...")
    if cpu:
        print(f"  CPU: pc=0x{cpu['pc']:X} sp=0x{cpu['sp']:X} insn={cpu['insn_count']} halted={cpu['halted']}")
    print(f"  Saved to: {out_dir}")


# ── Compare Mode ─────────────────────────────────────────────────────────────

def do_compare(ref_dir, name, tolerance):
    """Compare current screen against reference frame."""
    ref_path = os.path.join(ref_dir, name)
    bin_path = os.path.join(ref_path, "screen.bin")
    meta_path = os.path.join(ref_path, "meta.json")

    if not os.path.exists(bin_path):
        print(f"ERROR: Reference not found: {bin_path}", file=sys.stderr)
        print(f"Run: sudo python3 {sys.argv[0]} capture --name {name}", file=sys.stderr)
        sys.exit(1)

    # Read reference
    with open(bin_path, "rb") as f:
        ref_pixels = f.read()

    if len(ref_pixels) != SCREEN_SIZE:
        print(f"ERROR: Reference size mismatch: {len(ref_pixels)} != {SCREEN_SIZE}", file=sys.stderr)
        sys.exit(1)

    # Read current screen
    print(f"Comparing current screen against reference '{name}'...")
    current_pixels = read_screen_map()

    # Compute diff
    diff_count = sum(1 for a, b in zip(current_pixels, ref_pixels) if a != b)
    diff_pct = (diff_count / SCREEN_SIZE) * 100.0
    passed = diff_pct <= tolerance

    # Load meta for context
    meta = {}
    if os.path.exists(meta_path):
        with open(meta_path) as f:
            meta = json.load(f)

    current_stats = pixel_stats(current_pixels)
    ref_stats = pixel_stats(ref_pixels)

    print(f"  Reference: {name} (captured {meta.get('timestamp', 'unknown')})")
    print(f"  Build hash: {meta.get('build_hash', 'unknown')[:16]}...")
    print(f"  Reference non-zero: {ref_stats['nonzero_pct']}%")
    print(f"  Current non-zero:   {current_stats['nonzero_pct']}%")
    print(f"  Different pixels:   {diff_count}/{SCREEN_SIZE} ({diff_pct:.2f}%)")
    print(f"  Tolerance:          {tolerance}%")
    print(f"  Result:             {'PASS' if passed else 'FAIL'}")

    if not passed:
        # Save diff visualization
        diff_dir = os.path.join(ref_path, "last-diff")
        os.makedirs(diff_dir, exist_ok=True)

        # Save current screen for comparison
        with open(os.path.join(diff_dir, "current.bin"), "wb") as f:
            f.write(current_pixels)
        save_ppm(current_pixels, os.path.join(diff_dir, "current.ppm"))

        # Save diff map as PPM (red = different, green = same)
        diff_ppm = os.path.join(diff_dir, "diff.ppm")
        with open(diff_ppm, "wb") as f:
            f.write(f"P6\n{SCREEN_WIDTH} {SCREEN_HEIGHT}\n255\n".encode())
            for a, b in zip(current_pixels, ref_pixels):
                if a != b:
                    f.write(bytes([255, 0, 0]))  # Red = different
                else:
                    f.write(bytes([0, 64, 0]))    # Dark green = same

        print(f"  Diff saved: {diff_dir}/")

    sys.exit(0 if passed else 1)


# ── Main ─────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Capture and compare golden reference frames"
    )
    parser.add_argument("mode", choices=["capture", "compare"],
                        help="capture: save reference frame; compare: diff against reference")
    parser.add_argument("--ref-dir", default=DEFAULT_REF_DIR,
                        help=f"Reference storage directory (default: {DEFAULT_REF_DIR})")
    parser.add_argument("--name", default="baseline",
                        help="Reference name (default: baseline)")
    parser.add_argument("--tolerance", type=float, default=10.0,
                        help="Max pixel difference %% for compare (default: 10.0)")
    args = parser.parse_args()

    if args.mode == "capture":
        do_capture(args.ref_dir, args.name)
    elif args.mode == "compare":
        do_compare(args.ref_dir, args.name, args.tolerance)


if __name__ == "__main__":
    main()
