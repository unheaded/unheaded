#!/usr/bin/env python3
"""Pretty-print CPU_MAP state."""
import struct
import subprocess
import sys

result = subprocess.run(
    ['bpftool', 'map', 'lookup', 'pinned', '/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP',
     'key', '0x00', '0x00', '0x00', '0x00'],
    capture_output=True, text=True, check=False
)

if result.returncode != 0:
    print(f"bpftool failed: {result.stderr.strip()}")
    sys.exit(1)

lines = result.stdout.strip().split('\n')
hex_bytes = []
in_value = False
for line in lines:
    if 'value:' in line:
        in_value = True
        parts = line.split('value:')[1].strip().split()
        hex_bytes.extend(parts)
    elif in_value and line.strip() and not line.startswith('key:') and not line.startswith('Found'):
        hex_bytes.extend(line.strip().split())

if len(hex_bytes) < 104:
    print(f"Not enough bytes: got {len(hex_bytes)}, need 104")
    sys.exit(1)

bs = bytes(int(b, 16) for b in hex_bytes[:104])
regs = struct.unpack_from('<16I', bs, 0)
pc = struct.unpack_from('<I', bs, 64)[0]
flags, halted, stalled, pad = struct.unpack_from('<BBBB', bs, 68)
sleep_ns = struct.unpack_from('<Q', bs, 72)[0]
insn_count = struct.unpack_from('<Q', bs, 80)[0]
cache_hits = struct.unpack_from('<Q', bs, 88)[0]
cache_misses = struct.unpack_from('<Q', bs, 96)[0]

print(f'PC = 0x{pc:x} ({pc})')
print(f'insn_count = {insn_count:,}')
print(f'halted = {halted}, stalled = {stalled}')
print(f'flags = 0x{flags:02x}')
print(f'cache_hits = {cache_hits:,}, cache_misses = {cache_misses:,}')
print(f'SP (r15) = 0x{regs[15]:x}')
print('Registers:')
for i, r in enumerate(regs):
    if r != 0:
        print(f'  r{i} = 0x{r:08x} ({r})')
