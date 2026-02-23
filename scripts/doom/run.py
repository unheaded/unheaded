#!/usr/bin/env python3
"""
Doom-over-IPv6 execution driver.

Runs the inject→halt→clear→inject loop continuously, driving
D_DoomLoop through the title screen, demo cycle, and beyond.

The CPU halts on SYSCALL instructions (DG_DrawFrame, DG_GetTicksMs, etc.).
This script clears the halt and continues execution.

Usage:
  sudo ip netns exec monad0 python3 scripts/doom/run.py
  sudo ip netns exec monad0 python3 scripts/doom/run.py --frames 100
  sudo ip netns exec monad0 python3 scripts/doom/run.py --batch 500 --forever
"""
import argparse
import ctypes
import ctypes.util
import os
import platform
import signal
import socket
import struct
import sys
import time

# --- BPF constants ---
BPF_MAP_LOOKUP_ELEM = 1
BPF_MAP_UPDATE_ELEM = 2
BPF_OBJ_GET = 7

ARCH_SYSCALL = {"aarch64": 280, "x86_64": 321, "AMD64": 321}
CPU_STATE_SIZE = 104
DEFAULT_MAP_DIR = "/sys/fs/bpf/unheaded/doom-ring/maps"

_shutdown = False

def _handle_signal(signum, frame):
    global _shutdown
    _shutdown = True


signal.signal(signal.SIGINT, _handle_signal)
signal.signal(signal.SIGTERM, _handle_signal)


class BPFHelper:
    """Direct BPF syscall helper."""

    def __init__(self):
        arch = platform.machine()
        if arch not in ARCH_SYSCALL:
            raise RuntimeError(f"Unsupported arch: {arch}")
        self.sys_bpf = ARCH_SYSCALL[arch]
        self.libc = ctypes.CDLL(ctypes.util.find_library("c"), use_errno=True)

    def _bpf(self, cmd, attr_buf, attr_size):
        r = self.libc.syscall(
            ctypes.c_long(self.sys_bpf),
            ctypes.c_int(cmd),
            ctypes.byref(attr_buf),
            ctypes.c_uint(attr_size),
        )
        if r < 0:
            e = ctypes.get_errno()
            raise OSError(e, os.strerror(e))
        return r

    def open_pinned(self, path):
        path_b = path.encode("utf-8") + b"\x00"
        path_buf = ctypes.create_string_buffer(path_b)
        attr = (ctypes.c_char * 120)()
        struct.pack_into("=Q", attr, 0, ctypes.addressof(path_buf))
        return self._bpf(BPF_OBJ_GET, attr, 120)

    def lookup(self, fd, key_bytes, value_size):
        key_buf = (ctypes.c_char * len(key_bytes))(*key_bytes)
        val_buf = (ctypes.c_char * value_size)()
        attr = (ctypes.c_char * 120)()
        struct.pack_into("=I", attr, 0, fd)
        struct.pack_into("=Q", attr, 8, ctypes.addressof(key_buf))
        struct.pack_into("=Q", attr, 16, ctypes.addressof(val_buf))
        self._bpf(BPF_MAP_LOOKUP_ELEM, attr, 120)
        return bytes(val_buf)

    def update(self, fd, key_bytes, value_bytes):
        key_buf = (ctypes.c_char * len(key_bytes))(*key_bytes)
        val_buf = (ctypes.c_char * len(value_bytes))(*value_bytes)
        attr = (ctypes.c_char * 120)()
        struct.pack_into("=I", attr, 0, fd)
        struct.pack_into("=Q", attr, 8, ctypes.addressof(key_buf))
        struct.pack_into("=Q", attr, 16, ctypes.addressof(val_buf))
        struct.pack_into("=Q", attr, 24, 0)  # flags = BPF_ANY
        self._bpf(BPF_MAP_UPDATE_ELEM, attr, 120)


def build_packet():
    """Build a 78-byte Doom ring packet."""
    flow_label = 0xDE
    src_mac = bytes([0x02, 0x42, 0xAC, 0x11, 0x00, 0x02])
    dst_mac = bytes([0x02, 0x42, 0xAC, 0x11, 0x00, 0x03])
    version_tc_fl = (6 << 28) | (flow_label & 0xFFFFF)
    eth = dst_mac + src_mac + struct.pack(">H", 0x86DD)
    src_addr = bytes([0xFD,0x00,0x00,0x3F,0x00,0x75,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x01])
    dst_addr = bytes([0xFD,0x00,0xDE,0xAD,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x01])
    ipv6 = struct.pack(">IHBB", version_tc_fl, 24, 0, 255) + src_addr + dst_addr
    hbh = struct.pack("BBBB", 59, 2, 0x3E, 20)
    monad = struct.pack("BBBBBBBB", 0x01, 0, 0, 0, 0, 0, 0, 0x02)
    monad += struct.pack(">H", 0) + struct.pack("BBBB", 0, 0, 0, 0)
    monad += struct.pack("BBBB", 0, 0, 0, 0) + struct.pack("BB", 0, 0)
    return eth + ipv6 + hbh + monad


def main():
    parser = argparse.ArgumentParser(description="Doom-over-IPv6 execution driver")
    parser.add_argument("--map-dir", default=DEFAULT_MAP_DIR, help="BPF map pin directory")
    parser.add_argument("--iface", default="veth01", help="Injection interface")
    parser.add_argument("--batch", type=int, default=200, help="Packets per burst")
    parser.add_argument("--frames", type=int, default=0, help="Stop after N frames (0=unlimited)")
    parser.add_argument("--forever", action="store_true", help="Run until interrupted")
    parser.add_argument("--instance", type=lambda x: int(x, 0), default=0xDE, help="CPU instance")
    parser.add_argument("--namespace", default="monad0", help="Network namespace for injection")
    args = parser.parse_args()

    bpf = BPFHelper()
    cpu_fd = bpf.open_pinned(os.path.join(args.map_dir, "CPU_MAP"))
    key = struct.pack("<I", args.instance)

    # Open injection socket
    # We may be running in root namespace and need to enter monad0 for injection.
    # Try binding directly first, then fall back to creating socket in namespace.
    sock = None
    ns_fd = None
    try:
        sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
        sock.bind((args.iface, 0))
    except OSError:
        # Interface not in current namespace — try entering monad0
        if sock:
            sock.close()
        ns_path = f"/var/run/netns/{args.namespace}"
        if not os.path.exists(ns_path):
            print(f"ERROR: Cannot find namespace {args.namespace} and interface {args.iface}", file=sys.stderr)
            print("Run with: sudo python3 scripts/doom/run.py", file=sys.stderr)
            sys.exit(1)
        # Enter the namespace using setns
        ns_fd = os.open(ns_path, os.O_RDONLY)
        libc = ctypes.CDLL(ctypes.util.find_library("c"), use_errno=True)
        CLONE_NEWNET = 0x40000000
        if libc.setns(ns_fd, CLONE_NEWNET) != 0:
            e = ctypes.get_errno()
            print(f"ERROR: setns failed: {os.strerror(e)}", file=sys.stderr)
            sys.exit(1)
        sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
        sock.bind((args.iface, 0))

    packet = build_packet()
    frames = 0
    total_insns = 0
    total_pkts = 0
    halts = 0
    start = time.time()

    print(f"=== Doom Execution Driver ===")
    print(f"  Batch: {args.batch} pkts, Instance: 0x{args.instance:X}")
    print(f"  Target frames: {'unlimited' if args.frames == 0 else args.frames}")
    print()

    max_frames = args.frames if args.frames > 0 else (999999999 if args.forever else 1000)

    while frames < max_frames and not _shutdown:
        # Inject a burst
        for _ in range(args.batch):
            sock.send(packet)
        total_pkts += args.batch

        # Check CPU state
        raw = bpf.lookup(cpu_fd, key, CPU_STATE_SIZE)
        pc = struct.unpack_from("<I", raw, 64)[0]
        halted = raw[69]
        insn_count = struct.unpack_from("<Q", raw, 80)[0]

        if halted:
            halts += 1
            frames += 1
            total_insns = insn_count

            # Clear halt flag — write byte 69 back to 0
            state = bytearray(raw)
            state[69] = 0  # halted = 0
            bpf.update(cpu_fd, key, bytes(state))

            elapsed = time.time() - start
            fps = frames / elapsed if elapsed > 0 else 0
            if frames % 10 == 0 or frames <= 5:
                print(
                    f"  Frame {frames}: PC=0x{pc:X}, insns={insn_count:,}, "
                    f"pkts={total_pkts:,}, fps={fps:.1f}",
                    flush=True,
                )
        else:
            # CPU still running, inject more
            time.sleep(0.0001)  # Brief pause

    sock.close()
    if ns_fd is not None:
        os.close(ns_fd)
    os.close(cpu_fd)

    elapsed = time.time() - start
    fps = frames / elapsed if elapsed > 0 else 0
    print(f"\n=== SUMMARY ===")
    print(f"  Frames: {frames}")
    print(f"  Halts: {halts}")
    print(f"  Packets: {total_pkts:,}")
    print(f"  Instructions: {total_insns:,}")
    print(f"  Time: {elapsed:.1f}s")
    print(f"  FPS: {fps:.1f}")
    if _shutdown:
        print("  (interrupted by signal)")


if __name__ == "__main__":
    main()
