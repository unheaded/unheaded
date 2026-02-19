#!/usr/bin/env python3
"""
doom-loader-core.py — Bulk-load data into BPF maps via bpf() syscall.

Uses the BPF_MAP_UPDATE_ELEM syscall directly via ctypes for speed.
Loading 1M entries via individual bpftool calls would take hours;
direct syscalls take seconds.

Usage:
  python3 doom-loader-core.py rom <rom_pin_path> <mbc_file>
  python3 doom-loader-core.py ram <ram_pin_path> <data_file> <start_byte_addr_hex>
  python3 doom-loader-core.py cpu <cpu_pin_path>
"""

import sys
import os
import struct
import ctypes
import ctypes.util
import time
import mmap as mmap_mod

# BPF syscall constants
BPF_MAP_UPDATE_ELEM = 2
BPF_MAP_GET_FD_BY_ID = 14
BPF_OBJ_GET = 7
BPF_ANY = 0

# Syscall number for bpf() varies by architecture
import platform
_arch = platform.machine()
if _arch == 'aarch64':
    SYS_BPF = 280
elif _arch == 'x86_64':
    SYS_BPF = 321
else:
    raise RuntimeError(f"Unsupported architecture: {_arch}")


class BpfAttrMapElem(ctypes.Structure):
    """bpf_attr for BPF_MAP_UPDATE_ELEM / BPF_MAP_LOOKUP_ELEM"""
    _fields_ = [
        ("map_fd", ctypes.c_uint32),
        ("key", ctypes.c_uint64),
        ("value_or_next_key", ctypes.c_uint64),
        ("flags", ctypes.c_uint64),
    ]


class BpfAttrObjGet(ctypes.Structure):
    """bpf_attr for BPF_OBJ_GET (get fd from pinned path)"""
    _fields_ = [
        ("pathname", ctypes.c_uint64),
        ("bpf_fd", ctypes.c_uint32),
        ("file_flags", ctypes.c_uint32),
    ]


libc = ctypes.CDLL(ctypes.util.find_library("c"), use_errno=True)


def bpf_syscall(cmd, attr, size):
    """Raw bpf() syscall."""
    r = libc.syscall(SYS_BPF, cmd, ctypes.byref(attr), size)
    if r < 0:
        errno = ctypes.get_errno()
        raise OSError(errno, f"bpf() cmd={cmd} failed: {os.strerror(errno)}")
    return r


def bpf_obj_get(pin_path):
    """Get a BPF map FD from a pinned path."""
    path_bytes = pin_path.encode('utf-8') + b'\0'
    path_buf = ctypes.create_string_buffer(path_bytes)
    attr = BpfAttrObjGet()
    attr.pathname = ctypes.addressof(path_buf)
    attr.bpf_fd = 0
    attr.file_flags = 0
    fd = bpf_syscall(BPF_OBJ_GET, attr, ctypes.sizeof(attr))
    return fd


def bpf_map_update_elem(map_fd, key_bytes, value_bytes, flags=BPF_ANY):
    """Update a single element in a BPF map."""
    key_buf = ctypes.create_string_buffer(key_bytes)
    val_buf = ctypes.create_string_buffer(value_bytes)
    attr = BpfAttrMapElem()
    attr.map_fd = map_fd
    attr.key = ctypes.addressof(key_buf)
    attr.value_or_next_key = ctypes.addressof(val_buf)
    attr.flags = flags
    bpf_syscall(BPF_MAP_UPDATE_ELEM, attr, ctypes.sizeof(attr))


def load_rom(rom_pin, mbc_file):
    """Load doom.mbc into ROM_MAP (Array<u32>)."""
    print(f"[ROM] Opening {mbc_file}...")
    with open(mbc_file, 'rb') as f:
        data = f.read()

    num_words = len(data) // 4
    print(f"[ROM] {num_words} words to load")

    fd = bpf_obj_get(rom_pin)
    print(f"[ROM] Map FD: {fd}")

    start = time.time()
    batch_size = 10000
    for i in range(0, num_words, batch_size):
        end = min(i + batch_size, num_words)
        for idx in range(i, end):
            key = struct.pack('<I', idx)
            val = data[idx * 4:(idx + 1) * 4]
            bpf_map_update_elem(fd, key, val)
        elapsed = time.time() - start
        pct = (end / num_words) * 100
        rate = end / elapsed if elapsed > 0 else 0
        print(f"\r[ROM] {end}/{num_words} ({pct:.1f}%) — {rate:.0f} entries/sec", end='', flush=True)

    elapsed = time.time() - start
    print(f"\n[ROM] Done: {num_words} words in {elapsed:.1f}s ({num_words/elapsed:.0f} entries/sec)")
    os.close(fd)


def load_ram(ram_pin, data_file, start_byte_addr):
    """Load binary data into RAM_MAP (HashMap<u32,u32>) at given byte address."""
    print(f"[RAM] Opening {data_file}...")
    with open(data_file, 'rb') as f:
        data = f.read()

    # Pad to 4-byte alignment
    if len(data) % 4 != 0:
        data += b'\x00' * (4 - len(data) % 4)

    num_words = len(data) // 4
    start_word_addr = start_byte_addr >> 2
    print(f"[RAM] {num_words} words at word_addr 0x{start_word_addr:x} (byte_addr 0x{start_byte_addr:x})")

    fd = bpf_obj_get(ram_pin)
    print(f"[RAM] Map FD: {fd}")

    start = time.time()
    skipped = 0
    written = 0
    batch_size = 10000

    for i in range(0, num_words, batch_size):
        end = min(i + batch_size, num_words)
        for j in range(i, end):
            word_addr = start_word_addr + j
            val = data[j * 4:(j + 1) * 4]
            # Skip zero words (HashMap returns 0 for missing keys)
            if val == b'\x00\x00\x00\x00':
                skipped += 1
                continue
            key = struct.pack('<I', word_addr)
            bpf_map_update_elem(fd, key, val)
            written += 1
        elapsed = time.time() - start
        pct = (end / num_words) * 100
        rate = end / elapsed if elapsed > 0 else 0
        print(f"\r[RAM] {end}/{num_words} ({pct:.1f}%) — {rate:.0f} entries/sec, {written} written, {skipped} zero-skipped", end='', flush=True)

    elapsed = time.time() - start
    print(f"\n[RAM] Done: {written} words written, {skipped} zero-skipped in {elapsed:.1f}s")
    os.close(fd)


def load_rv2mbc(rv2mbc_pin, rv2mbc_file):
    """Load doom.rv2mbc into RV2MBC_MAP (Array<u32>).

    Same format as ROM loading: flat array of little-endian u32 values.
    Index = RV32I word index, Value = MBC PC index.
    """
    print(f"[RV2MBC] Opening {rv2mbc_file}...")
    with open(rv2mbc_file, 'rb') as f:
        data = f.read()

    num_words = len(data) // 4
    print(f"[RV2MBC] {num_words} entries to load")

    fd = bpf_obj_get(rv2mbc_pin)
    print(f"[RV2MBC] Map FD: {fd}")

    start = time.time()
    batch_size = 10000
    for i in range(0, num_words, batch_size):
        end = min(i + batch_size, num_words)
        for idx in range(i, end):
            key = struct.pack('<I', idx)
            val = data[idx * 4:(idx + 1) * 4]
            bpf_map_update_elem(fd, key, val)
        elapsed = time.time() - start
        pct = (end / num_words) * 100
        rate = end / elapsed if elapsed > 0 else 0
        print(f"\r[RV2MBC] {end}/{num_words} ({pct:.1f}%) — {rate:.0f} entries/sec", end='', flush=True)

    elapsed = time.time() - start
    print(f"\n[RV2MBC] Done: {num_words} entries in {elapsed:.1f}s ({num_words/elapsed:.0f} entries/sec)")
    os.close(fd)


def load_cpu(cpu_pin):
    """Initialize CPU_MAP with starting state for instance 0."""
    print("[CPU] Initializing CPU state...")

    fd = bpf_obj_get(cpu_pin)
    print(f"[CPU] Map FD: {fd}")

    # MbcCpuState layout (from monad-common/src/lib.rs, #[repr(C)]):
    #   regs: [u32; 16]    = 64 bytes (registers r0-r15)
    #   pc: u32             = 4 bytes  (offset 64)
    #   flags: u8           = 1 byte   (offset 68)
    #   halted: u8          = 1 byte   (offset 69)
    #   stalled: u8         = 1 byte   (offset 70)
    #   _pad: u8            = 1 byte   (offset 71)
    #   sleep_until_ns: u64 = 8 bytes  (offset 72)
    #   insn_count: u64     = 8 bytes  (offset 80)
    #   cache_hits: u64     = 8 bytes  (offset 88)
    #   cache_misses: u64   = 8 bytes  (offset 96)
    #   Total: 104 bytes

    regs = [0] * 16
    regs[15] = 0x710000  # SP = stack top (byte address from linker script)
    pc = 0               # Start at beginning of ROM
    flags = 0
    halted = 0
    stalled = 0
    pad = 0
    insn_count = 0
    sleep_until_ns = 0
    cache_hits = 0
    cache_misses = 0

    # Pack the struct
    cpu_state = b''
    for r in regs:
        cpu_state += struct.pack('<I', r)
    cpu_state += struct.pack('<I', pc)
    cpu_state += struct.pack('<BBBB', flags, halted, stalled, pad)
    cpu_state += struct.pack('<Q', sleep_until_ns)
    cpu_state += struct.pack('<Q', insn_count)
    cpu_state += struct.pack('<Q', cache_hits)
    cpu_state += struct.pack('<Q', cache_misses)

    print(f"[CPU] State size: {len(cpu_state)} bytes")

    # Key = instance_id (u32), Value = MbcCpuState
    instance_id = 0
    key = struct.pack('<I', instance_id)
    bpf_map_update_elem(fd, key, cpu_state)

    print(f"[CPU] Instance {instance_id}: PC=0x{pc:x}, SP=0x{regs[15]:x}, halted={halted}")
    os.close(fd)


def main():
    if len(sys.argv) < 3:
        print("Usage:")
        print("  python3 doom-loader-core.py rom <rom_pin> <mbc_file>")
        print("  python3 doom-loader-core.py ram <ram_pin> <data_file> <start_byte_addr_hex>")
        print("  python3 doom-loader-core.py rv2mbc <rv2mbc_pin> <rv2mbc_file>")
        print("  python3 doom-loader-core.py cpu <cpu_pin>")
        sys.exit(1)

    cmd = sys.argv[1]

    if cmd == 'rom':
        if len(sys.argv) != 4:
            print("Usage: python3 doom-loader-core.py rom <rom_pin> <mbc_file>")
            sys.exit(1)
        load_rom(sys.argv[2], sys.argv[3])

    elif cmd == 'ram':
        if len(sys.argv) != 5:
            print("Usage: python3 doom-loader-core.py ram <ram_pin> <data_file> <start_byte_addr_hex>")
            sys.exit(1)
        start_addr = int(sys.argv[4], 0)
        load_ram(sys.argv[2], sys.argv[3], start_addr)

    elif cmd == 'rv2mbc':
        if len(sys.argv) != 4:
            print("Usage: python3 doom-loader-core.py rv2mbc <rv2mbc_pin> <rv2mbc_file>")
            sys.exit(1)
        load_rv2mbc(sys.argv[2], sys.argv[3])

    elif cmd == 'cpu':
        if len(sys.argv) != 3:
            print("Usage: python3 doom-loader-core.py cpu <cpu_pin>")
            sys.exit(1)
        load_cpu(sys.argv[2])

    else:
        print(f"Unknown command: {cmd}")
        sys.exit(1)


if __name__ == '__main__':
    main()
