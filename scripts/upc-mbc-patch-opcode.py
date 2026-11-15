#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Rewrite one opcode in a copy of an MBC image (S-STAGE-2C Phase 4, ADR-135 D17).

The RV32I to MBC translator emits no atomic (XCHG, CAS, LR_W, SC_W), so a probe
that needs one cannot be compiled. This tool makes one from a compiled probe: it
finds the marker (a word that sits just before the instruction to change: a
literal, or with --marker-mask an instruction such as the ADDI that completes an
address), then the first instruction after it whose opcode is --from, and writes
--to in its place. Destination, source and imm16 are kept. The result is a hostile image for
a test, never a thing to ship: bootctl's opcode scan refuses it.

An MBC image is little-endian 32-bit words: opcode in bits 31-24, dst 23-20,
src 19-16, imm16 15-0 (ebpf/monad-common, MbcInsn).

    upc-mbc-patch-opcode.py IMAGE --word-of-marker 0x1D000400 --marker-mask 0xFF00FFFF \
        --from 0x30 --to 0x3D --out COPY

(that marker is "ADDI any register, 0x0400", the last step of loading 0x00800400).

Exit codes: 0 written, 1 marker or instruction not found, 2 usage.
"""
import argparse
import os
import struct
import sys


def number(text):
    return int(text, 0)


def patch(words, marker, old, new, mask=0xFFFFFFFF):
    """Return (index, patched words). Raises LookupError with the reason."""
    at = next((i for i, w in enumerate(words) if w & mask == marker & mask), None)
    if at is None:
        raise LookupError(f"marker 0x{marker:08X} (mask 0x{mask:08X}) matches no word of the image")
    for i in range(at + 1, len(words)):
        if words[i] >> 24 == old:
            out = list(words)
            out[i] = (new << 24) | (words[i] & 0x00FFFFFF)
            return i, out
    raise LookupError(f"no instruction with opcode 0x{old:02X} after the marker (word {at})")


def main(argv):
    ap = argparse.ArgumentParser(description="Rewrite one opcode in a copy of an MBC image.")
    ap.add_argument("image")
    ap.add_argument("--word-of-marker", type=number, required=True, metavar="WORD",
                    help="32-bit word that precedes the instruction to change")
    ap.add_argument("--marker-mask", type=number, default=0xFFFFFFFF, metavar="MASK",
                    help="bits of the marker that must match (default: all)")
    ap.add_argument("--from", dest="old", type=number, required=True, metavar="OPCODE")
    ap.add_argument("--to", dest="new", type=number, required=True, metavar="OPCODE")
    ap.add_argument("--out", required=True, help="where the copy is written; never the image itself")
    args = ap.parse_args(argv)

    for name, value in (("--from", args.old), ("--to", args.new)):
        if not 0 <= value <= 0xFF:
            ap.error(f"{name} must be one byte")
    if not 0 <= args.word_of_marker <= 0xFFFFFFFF or not 0 < args.marker_mask <= 0xFFFFFFFF:
        ap.error("--word-of-marker and --marker-mask must be 32-bit values")
    if os.path.exists(args.out) and os.path.samefile(args.image, args.out):
        ap.error("--out is the image itself; this tool only writes a copy")

    with open(args.image, "rb") as f:
        raw = f.read()
    if len(raw) % 4:
        print(f"upc-mbc-patch-opcode: {args.image} is not a whole number of words", file=sys.stderr)
        return 1
    words = list(struct.unpack(f"<{len(raw) // 4}I", raw))
    try:
        index, out = patch(words, args.word_of_marker, args.old, args.new, args.marker_mask)
    except LookupError as e:
        print(f"upc-mbc-patch-opcode: {e}", file=sys.stderr)
        return 1
    with open(args.out, "wb") as f:
        f.write(struct.pack(f"<{len(out)}I", *out))
    print(f"word {index}: 0x{words[index]:08X} -> 0x{out[index]:08X}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
