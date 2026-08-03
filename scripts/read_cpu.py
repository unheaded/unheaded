#!/usr/bin/env python3
"""Read and parse CPU state for instance 0xDE."""
import re
import struct
import subprocess
import sys

result = subprocess.run(['sudo', 'bpftool', 'map', 'lookup', 'pinned',
    '/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP', 'key', '222', '0', '0', '0'],
    capture_output=True, text=True, check=False)

output = result.stdout
# Try 0xNN format first
hex_vals = re.findall(r'0x([0-9a-fA-F]{2})', output)
if not hex_vals:
    # Try raw hex format (NN NN NN) - extract from value: line onwards
    val_match = re.search(r'value:\s*\n([\s0-9a-fA-F]+)', output)
    if val_match:
        hex_vals = re.findall(r'([0-9a-fA-F]{2})', val_match.group(1))

if not hex_vals:
    print("ERROR: Could not read CPU state")
    print("Raw:", output[:500])
    print("Stderr:", result.stderr[:500])
    sys.exit(1)

raw = bytes(int(h, 16) for h in hex_vals)
print(f"=== CPU STATE (instance 0xDE) === ({len(raw)} bytes)")
for i in range(16):
    if i*4+4 <= len(raw):
        val = struct.unpack_from('<I', raw, i*4)[0]
        if val != 0 or i in (0, 1, 2, 3, 15):
            print(f"  r{i} = {val} (0x{val:08X})")
if len(raw) >= 68:
    print(f"  PC = {struct.unpack_from('<I', raw, 64)[0]}")
if len(raw) >= 69:
    print(f"  flags = {raw[68]}")
if len(raw) >= 70:
    print(f"  halted = {raw[69]}")
if len(raw) >= 71:
    print(f"  stalled = {raw[70]}")
if len(raw) >= 88:
    print(f"  insn_count = {struct.unpack_from('<Q', raw, 80)[0]}")
if len(raw) >= 96:
    print(f"  cache_hits = {struct.unpack_from('<Q', raw, 88)[0]}")
if len(raw) >= 104:
    print(f"  cache_misses = {struct.unpack_from('<Q', raw, 96)[0]}")
