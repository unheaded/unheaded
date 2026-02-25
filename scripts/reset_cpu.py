#!/usr/bin/env python3
"""Reset CPU state for instance 0xDE via bpftool."""
import subprocess

state = bytearray(104)
# SP (r15) at offset 60 = 0xFFFF0000 little-endian
state[60] = 0x00
state[61] = 0x00
state[62] = 0xFF
state[63] = 0xFF
key = [222, 0, 0, 0]  # 0xDE
cmd = ['sudo', 'bpftool', 'map', 'update', 'pinned',
       '/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP',
       'key'] + [str(k) for k in key] + ['value'] + [str(b) for b in state]
subprocess.run(cmd, check=True)
print("CPU state reset for instance 0xDE (PC=0, SP=0xFFFF0000, halted=0)")
