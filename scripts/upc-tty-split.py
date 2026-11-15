#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""upc-tty-split.py: split a upc-bootctl TTY dump into its two writers.

Reads a boot's output on stdin (the "  hex:" lines of the TTY dump) and prints
the probe tokens scripts/upc-rung0-assert.awk reads, one per line.

Why two streams. Kernel printf stores a word per character, so its bytes reach
the TTY ring as the character and two NULs. A user write is a byte store: one
byte, no NUL (since rung 3 it is the kernel's uartputc_byte, ADR-135 D19;
before it, BPF's console loop). A probe kernel (-DUPC_STAGE2_PROBE) prints a
"UT:" line on every tagged trap, and with rung 3 on each character a user
program prints is such a trap, so the two writers interleave character by
character. Dropping the NULs, as the S-STAGE-2B decoder did, shreds both.
Splitting on "followed by a NUL" gives each writer its own text back.

Output: the kernel stream's UT:/SC:/SP:/PG: tokens, then the byte stream's
"XX: key[=value]" tokens (GL, GM, GF, GX, FL, HP, UP).
Exit 0 always; an empty output is the caller's failure to judge.
"""
import re
import sys

KERNEL_TOKEN = re.compile(rb'(?:UT|SC|SP|PG):[^ "\n]*;')
USER_TOKEN = re.compile(rb"(?:GL|GM|GF|GX|FL|HP|UP): [a-z0-9-]+(?:=[0-9a-z]+)?")


def tty_bytes(text: bytes) -> bytes:
    out = bytearray()
    for line in text.split(b"\n"):
        if not line.startswith(b"  hex:"):
            continue
        digits = bytes(c for c in line[6:] if c in b"0123456789abcdefABCDEF")
        out += bytes.fromhex(digits[: len(digits) // 2 * 2].decode())
    return bytes(out)


def split(raw: bytes) -> tuple[bytes, bytes]:
    kernel, user = bytearray(), bytearray()
    i, n = 0, len(raw)
    while i < n:
        c = raw[i]
        if c == 0:
            i += 1
            continue
        j = i + 1
        while j < n and raw[j] == 0:
            j += 1
        (kernel if j > i + 1 else user).append(c)
        i = j
    return bytes(kernel), bytes(user)


def main() -> int:
    kernel, user = split(tty_bytes(sys.stdin.buffer.read()))
    out = sys.stdout.buffer
    for token in KERNEL_TOKEN.findall(kernel):
        out.write(token + b"\n")
    for token in USER_TOKEN.findall(user):
        out.write(token + b"\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
