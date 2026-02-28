#!/usr/bin/env python3
"""
Fast ring buffer drain — zero-copy consumer that advances consumer_pos
without parsing events. Used for stress testing to prevent RING_DROPS.

For production, trace-collector should do the actual parsing.
This is the "floor" benchmark — how fast can we drain the ring buffer.
"""

import ctypes
import ctypes.util
import mmap
import os
import signal
import struct
import sys
import time

PAGE_SIZE = 4096
BPF_RINGBUF_BUSY_BIT = 1 << 31
BPF_RINGBUF_DISCARD_BIT = 1 << 30
BPF_RINGBUF_LEN_MASK = (1 << 28) - 1
SYS_BPF = 321
BPF_OBJ_GET = 7


class BpfAttrObj(ctypes.Structure):
    _fields_ = [
        ('pathname', ctypes.c_uint64),
        ('bpf_fd', ctypes.c_uint32),
        ('file_flags', ctypes.c_uint32),
    ]


def open_bpf_map(path: str) -> int:
    libc = ctypes.CDLL(ctypes.util.find_library('c'), use_errno=True)
    attr = BpfAttrObj()
    path_buf = ctypes.create_string_buffer(path.encode() + b'\x00')
    attr.pathname = ctypes.addressof(path_buf)
    fd = libc.syscall(SYS_BPF, BPF_OBJ_GET, ctypes.byref(attr), ctypes.sizeof(attr))
    if fd < 0:
        errno = ctypes.get_errno()
        raise OSError(errno, f"BPF_OBJ_GET failed: {os.strerror(errno)}")
    return fd


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "/sys/fs/bpf/unheaded/shield-ebpf/ANAMNESIS"
    print(f"Opening ring buffer: {path}")

    fd = open_bpf_map(path)
    print(f"fd={fd}")

    # mmap consumer page (offset 0, read-write)
    cons_mm = mmap.mmap(fd, PAGE_SIZE, mmap.MAP_SHARED, mmap.PROT_READ | mmap.PROT_WRITE, offset=0)
    # mmap producer page (offset PAGE_SIZE, read-only)
    prod_mm = mmap.mmap(fd, PAGE_SIZE, mmap.MAP_SHARED, mmap.PROT_READ, offset=PAGE_SIZE)

    # Get data size from map info (we know it's 8MB)
    data_size = 8 * 1024 * 1024
    mask = data_size - 1

    # mmap data pages (offset 2*PAGE_SIZE, read-only, 2x for wrap)
    data_mm = mmap.mmap(fd, data_size * 2, mmap.MAP_SHARED, mmap.PROT_READ, offset=2 * PAGE_SIZE)

    running = True
    def handler(sig, frame):
        nonlocal running
        running = False
    signal.signal(signal.SIGINT, handler)
    signal.signal(signal.SIGTERM, handler)

    total_events = 0
    total_bytes = 0
    start = time.monotonic()
    last_report = start

    print("Draining ring buffer... (Ctrl+C to stop)")

    while running:
        cons_pos = struct.unpack('<Q', cons_mm[:8])[0]
        prod_pos = struct.unpack('<Q', prod_mm[:8])[0]

        if cons_pos >= prod_pos:
            # No data, busy-wait briefly
            time.sleep(0.0001)  # 100μs
            continue

        # Drain all available records
        batch = 0
        while cons_pos < prod_pos:
            offset = cons_pos & mask
            header = struct.unpack('<I', data_mm[offset:offset+4])[0]

            if header & BPF_RINGBUF_BUSY_BIT:
                break  # Record still being written

            length = header & BPF_RINGBUF_LEN_MASK
            record_size = 8 + ((length + 7) & ~7)  # 8-byte header + aligned payload

            cons_pos += record_size
            batch += 1

        # Advance consumer position
        if batch > 0:
            struct.pack_into('<Q', cons_mm, 0, cons_pos)
            total_events += batch
            total_bytes += batch * 40  # Approximate

        # Report
        now = time.monotonic()
        if now - last_report >= 1.0:
            elapsed = now - start
            rate = total_events / elapsed if elapsed > 0 else 0
            print(f"\r[{elapsed:7.1f}s] drained={total_events:>12,}  rate={rate:>10,.0f} events/s  "
                  f"cons=0x{cons_pos:x} prod=0x{prod_pos:x}", end="", flush=True)
            last_report = now

    elapsed = time.monotonic() - start
    print(f"\n\nFinal: {total_events:,} events in {elapsed:.1f}s = {total_events/elapsed:,.0f} events/s")

    cons_mm.close()
    prod_mm.close()
    data_mm.close()
    os.close(fd)


if __name__ == "__main__":
    main()
