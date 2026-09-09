#!/usr/bin/env python3
"""
Fast ROM/RV2MBC loader using BPF syscall directly (no subprocess per entry).
Works on aarch64 (SYS_BPF=280) and x86_64 (SYS_BPF=321).
"""
import ctypes
import ctypes.util
import os
import platform
import struct
import sys
import time

# BPF commands
BPF_MAP_UPDATE_ELEM = 2
BPF_OBJ_GET = 7

# Syscall number
arch = platform.machine()
if arch == 'aarch64':
    SYS_BPF = 280
elif arch in ('x86_64', 'AMD64'):
    SYS_BPF = 321
else:
    print(f"Unknown arch: {arch}")
    sys.exit(1)

libc = ctypes.CDLL(ctypes.util.find_library("c"), use_errno=True)

def bpf(cmd, attr_buf, attr_size):
    """Raw bpf() syscall."""
    r = libc.syscall(ctypes.c_long(SYS_BPF), ctypes.c_int(cmd),
                     ctypes.byref(attr_buf), ctypes.c_uint(attr_size))
    if r < 0:
        e = ctypes.get_errno()
        raise OSError(e, os.strerror(e))
    return r

def open_pinned(path):
    """BPF_OBJ_GET: open a pinned map, return fd."""
    path_b = path.encode('utf-8') + b'\x00'
    path_buf = ctypes.create_string_buffer(path_b)
    # union bpf_attr { struct { __aligned_u64 pathname; ... } }
    # On both arches, pathname is at offset 0 as a u64 pointer
    attr = (ctypes.c_char * 120)()
    struct.pack_into('=Q', attr, 0, ctypes.addressof(path_buf))
    return bpf(BPF_OBJ_GET, attr, 120)

def load_map(fd, data, entry_size=4, label="entries"):
    """Load a flat binary into a BPF array map. Key = u32 index, value = entry_size bytes."""
    n = len(data) // entry_size
    print(f"  Loading {n} {label} ({len(data)} bytes) ...")

    # Pre-allocate key and value buffers
    key_buf = (ctypes.c_char * 4)()
    val_buf = (ctypes.c_char * entry_size)()

    # Build the attr once, just update key/value contents each iteration
    attr = (ctypes.c_char * 120)()
    struct.pack_into('=I', attr, 0, fd)                          # map_fd
    struct.pack_into('=Q', attr, 8, ctypes.addressof(key_buf))   # key ptr
    struct.pack_into('=Q', attr, 16, ctypes.addressof(val_buf))  # value ptr
    struct.pack_into('=Q', attr, 24, 0)                          # flags (ANY)

    start = time.time()
    errors = 0
    for i in range(n):
        struct.pack_into('<I', key_buf, 0, i)
        ctypes.memmove(val_buf, data[i*entry_size:(i+1)*entry_size], entry_size)
        try:
            bpf(BPF_MAP_UPDATE_ELEM, attr, 120)
        except OSError as e:
            errors += 1
            if errors <= 5:
                print(f"    Error at [{i}]: {e}")
            if errors == 6:
                print("    (suppressing further errors)")

        if (i+1) % 20000 == 0 or i == n-1:
            elapsed = time.time() - start
            rate = (i+1) / elapsed if elapsed > 0 else 0
            pct = ((i+1) * 100) // n
            print(f"    [{pct:3d}%] {i+1}/{n} ({rate:.0f} entries/s, {elapsed:.1f}s)")

    elapsed = time.time() - start
    rate = n / elapsed if elapsed > 0 else 0
    print(f"  Done: {n} {label}, {errors} errors, {elapsed:.1f}s ({rate:.0f}/s)")
    return n, errors

def main():
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <rom.mbc> [rv2mbc_file]")
        sys.exit(1)

    rom_path = sys.argv[1]
    rv2mbc_path = sys.argv[2] if len(sys.argv) > 2 else None

    # Load ROM
    print(f"=== Loading ROM from {rom_path} ===")
    with open(rom_path, 'rb') as f:
        rom_data = f.read()
    rom_fd = open_pinned('/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP')
    print(f"  ROM_MAP fd={rom_fd}")
    rom_n, rom_err = load_map(rom_fd, rom_data, 4, "instructions")
    os.close(rom_fd)

    # Load RV2MBC if provided
    if rv2mbc_path:
        print(f"\n=== Loading RV2MBC from {rv2mbc_path} ===")
        with open(rv2mbc_path, 'rb') as f:
            rv2mbc_data = f.read()
        rv2mbc_fd = open_pinned('/sys/fs/bpf/unheaded/doom-ring/maps/RV2MBC_MAP')
        print(f"  RV2MBC_MAP fd={rv2mbc_fd}")
        rv2mbc_n, rv2mbc_err = load_map(rv2mbc_fd, rv2mbc_data, 4, "translations")
        os.close(rv2mbc_fd)

    print("\n=== SUMMARY ===")
    print(f"  ROM: {rom_n} instructions loaded ({rom_err} errors)")
    if rv2mbc_path:
        print(f"  RV2MBC: {rv2mbc_n} translations loaded ({rv2mbc_err} errors)")

if __name__ == '__main__':
    main()
