#!/usr/bin/env python3
"""doom-screen-check.py — Validate SCREEN_MAP pixel data from BPF maps.

Checks:
  - Non-zero pixel percentage (>50% expected during gameplay)
  - WAD pollution detection: 4+ byte ASCII runs (Bug 24 signature)
  - Palette index distribution histogram
  - Optional: save as PPM image using real DOOM palette
  - Optional: compare against reference binary blob

Usage:
  sudo python3 scripts/doom-screen-check.py [--save FILE] [--reference FILE] \
       [--tolerance PCT] [--json]

Exit: 0 if valid, 1 if corrupted

Dependencies: bpftool (for BPF map access), python3 standard library only
"""

import argparse
import json as json_mod
import os
import struct
import subprocess
import sys

# ── Constants ────────────────────────────────────────────────────────────────

SCREEN_WIDTH = 320
SCREEN_HEIGHT = 200
SCREEN_SIZE = SCREEN_WIDTH * SCREEN_HEIGHT  # 64000

MAP_PIN_DIR = "/sys/fs/bpf/unheaded/doom-ring/maps"
SCREEN_MAP_PIN = f"{MAP_PIN_DIR}/SCREEN_MAP"

# Minimum non-zero pixel percentage for a valid gameplay frame
MIN_NONZERO_PCT = 50.0

# Maximum consecutive ASCII bytes before flagging WAD pollution
MAX_ASCII_RUN = 4

# Known WAD texture names that indicate Bug 24 pollution
WAD_SIGNATURES = [
    b"COMPTALL", b"BROWN96", b"STARTAN", b"FLOOR4_8",
    b"CEIL3_5", b"FLAT14", b"TEKWALL", b"STEP1",
    b"DOOR1", b"LITE5", b"SW1BRNG", b"NUKAGE",
]

# ── DOOM PLAYPAL — 256 RGB entries from doom1.wad ───────────────────────────
# Extracted from demos/doom/index.html lines 261-293

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
    """Read SCREEN_MAP from BPF maps via bpftool batch dump.

    Returns bytes of length SCREEN_SIZE (64000), one byte per pixel.
    """
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

    # Parse bpftool output: "key: XX XX XX XX  value: XX\n" per entry
    pixels = bytearray(SCREEN_SIZE)
    for line in result.stdout.splitlines():
        line = line.strip()
        if not line or line.startswith("Found"):
            continue
        # Format: key: 00 00 00 00  value: 2a
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


# ── Analysis Functions ───────────────────────────────────────────────────────

def check_nonzero_pct(pixels):
    """Check percentage of non-zero pixels."""
    nonzero = sum(1 for p in pixels if p != 0)
    pct = (nonzero / SCREEN_SIZE) * 100.0
    return pct, nonzero


def check_wad_pollution(pixels):
    """Detect WAD texture name pollution (Bug 24 signature).

    Scans for 4+ consecutive printable ASCII bytes, which indicates
    WAD lump names being written to screen memory through corrupted pointers.
    """
    found = []

    # Check for known WAD signatures
    for sig in WAD_SIGNATURES:
        idx = pixels.find(sig)
        if idx >= 0:
            found.append({
                "signature": sig.decode("ascii", errors="replace"),
                "offset": idx,
                "x": idx % SCREEN_WIDTH,
                "y": idx // SCREEN_WIDTH,
            })

    # Scan for generic ASCII runs
    ascii_runs = 0
    run_start = -1
    run_len = 0

    for i, b in enumerate(pixels):
        if 0x20 <= b <= 0x7E:  # Printable ASCII
            if run_start < 0:
                run_start = i
                run_len = 1
            else:
                run_len += 1
        else:
            if run_len >= MAX_ASCII_RUN:
                ascii_runs += 1
                text = pixels[run_start:run_start + run_len].decode("ascii", errors="replace")
                if len(found) < 10:  # Limit reported instances
                    found.append({
                        "signature": text[:32],
                        "offset": run_start,
                        "x": run_start % SCREEN_WIDTH,
                        "y": run_start // SCREEN_WIDTH,
                    })
            run_start = -1
            run_len = 0

    # Check final run
    if run_len >= MAX_ASCII_RUN:
        ascii_runs += 1

    return ascii_runs, found


def palette_histogram(pixels):
    """Compute palette index distribution."""
    hist = [0] * 256
    for p in pixels:
        hist[p] += 1
    return hist


def save_ppm(pixels, filename):
    """Save screen as PPM image using DOOM palette."""
    with open(filename, "wb") as f:
        f.write(f"P6\n{SCREEN_WIDTH} {SCREEN_HEIGHT}\n255\n".encode())
        for p in pixels:
            idx = p * 3
            f.write(bytes([DOOM_PALETTE[idx], DOOM_PALETTE[idx + 1], DOOM_PALETTE[idx + 2]]))
    return filename


def compare_reference(pixels, ref_path, tolerance):
    """Compare screen pixels against a reference binary blob."""
    if not os.path.exists(ref_path):
        return None, f"Reference file not found: {ref_path}"

    with open(ref_path, "rb") as f:
        ref_pixels = f.read()

    if len(ref_pixels) != SCREEN_SIZE:
        return None, f"Reference size mismatch: {len(ref_pixels)} != {SCREEN_SIZE}"

    diff_count = sum(1 for a, b in zip(pixels, ref_pixels) if a != b)
    diff_pct = (diff_count / SCREEN_SIZE) * 100.0
    passed = diff_pct <= tolerance

    return {
        "diff_pixels": diff_count,
        "diff_pct": diff_pct,
        "tolerance": tolerance,
        "passed": passed,
    }, None


# ── Main ─────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Validate SCREEN_MAP pixel data from BPF maps"
    )
    parser.add_argument("--save", metavar="FILE", help="Save screen as PPM image")
    parser.add_argument("--reference", metavar="FILE", help="Compare against reference .bin")
    parser.add_argument("--tolerance", type=float, default=10.0,
                        help="Max pixel difference %% for reference compare (default: 10.0)")
    parser.add_argument("--json", action="store_true", help="Output results as JSON")
    args = parser.parse_args()

    # Read screen data
    pixels = read_screen_map()

    # Run checks
    nonzero_pct, nonzero_count = check_nonzero_pct(pixels)
    ascii_runs, wad_hits = check_wad_pollution(pixels)
    hist = palette_histogram(pixels)

    # Determine overall verdict
    valid = True
    issues = []

    if nonzero_pct < MIN_NONZERO_PCT:
        valid = False
        issues.append(f"Low non-zero pixel count: {nonzero_pct:.1f}% (need >{MIN_NONZERO_PCT}%)")

    if ascii_runs > 0:
        valid = False
        issues.append(f"WAD pollution detected: {ascii_runs} ASCII runs (Bug 24)")

    # Optional: save PPM
    saved_path = None
    if args.save:
        saved_path = save_ppm(pixels, args.save)

    # Optional: reference compare
    ref_result = None
    ref_error = None
    if args.reference:
        ref_result, ref_error = compare_reference(pixels, args.reference, args.tolerance)
        if ref_error:
            issues.append(ref_error)
        elif ref_result and not ref_result["passed"]:
            valid = False
            issues.append(f"Reference diff: {ref_result['diff_pct']:.1f}% > {args.tolerance}%")

    # Top 10 most used palette indices
    top_indices = sorted(range(256), key=lambda i: hist[i], reverse=True)[:10]

    # Output
    result = {
        "valid": valid,
        "nonzero_pct": round(nonzero_pct, 2),
        "nonzero_count": nonzero_count,
        "total_pixels": SCREEN_SIZE,
        "ascii_runs": ascii_runs,
        "wad_hits": [h["signature"] for h in wad_hits],
        "top_palette_indices": [{"index": i, "count": hist[i], "pct": round(hist[i] / SCREEN_SIZE * 100, 2)} for i in top_indices],
        "issues": issues,
    }

    if saved_path:
        result["saved_ppm"] = saved_path

    if ref_result:
        result["reference_compare"] = ref_result

    if args.json:
        print(json_mod.dumps(result, indent=2))
    else:
        # Human-readable output
        print(f"{'PASS' if valid else 'FAIL'}: SCREEN_MAP validation")
        print(f"  Non-zero pixels: {nonzero_count}/{SCREEN_SIZE} ({nonzero_pct:.1f}%)")
        print(f"  ASCII runs (Bug 24): {ascii_runs}")
        if wad_hits:
            for h in wad_hits[:5]:
                print(f"    WAD pollution at ({h['x']},{h['y']}): \"{h['signature']}\"")
        print(f"  Top palette indices: {', '.join(f'{i}({hist[i]})' for i in top_indices[:5])}")
        if saved_path:
            print(f"  Saved PPM: {saved_path}")
        if ref_result:
            status = "PASS" if ref_result["passed"] else "FAIL"
            print(f"  Reference compare: {status} ({ref_result['diff_pct']:.1f}% diff, tolerance {args.tolerance}%)")
        if issues:
            print(f"  Issues:")
            for issue in issues:
                print(f"    - {issue}")

    sys.exit(0 if valid else 1)


if __name__ == "__main__":
    main()
