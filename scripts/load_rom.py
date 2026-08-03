#!/usr/bin/env python3
"""Load an MBC binary into the BPF ROM_MAP via bpftool."""
import struct
import subprocess
import sys


def load_rom(path):
    with open(path, 'rb') as f:
        data = f.read()
    n = len(data) // 4
    print(f"Loading {n} instructions from {path}")
    for i in range(n):
        word = struct.unpack_from('<I', data, i*4)[0]
        b = list(word.to_bytes(4, 'little'))
        key = list(i.to_bytes(4, 'little'))
        cmd = ['sudo', 'bpftool', 'map', 'update', 'pinned',
               '/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP',
               'key'] + [str(x) for x in key] + ['value'] + [str(x) for x in b]
        subprocess.run(cmd, check=True, capture_output=True)
    print(f"Loaded {n} instructions successfully")

if __name__ == '__main__':
    load_rom(sys.argv[1])
