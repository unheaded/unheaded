#!/usr/bin/env python3
"""
Doom-over-IPv6 execution driver.

Runs the inject→halt→clear→inject loop continuously, driving
D_DoomLoop through the title screen, demo cycle, and beyond.

The CPU halts on SYSCALL instructions (DG_DrawFrame, DG_GetTicksMs, etc.)
and on ROM faults (out-of-bounds PC, stack corruption). This script
distinguishes between the two: syscall halts are resumed, fault halts
trigger a CPU reset and restart.

Usage:
  sudo python3 scripts/doom/run.py
  sudo python3 scripts/doom/run.py --frames 100
  sudo python3 scripts/doom/run.py --batch 500 --forever
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

# ROM bounds (from translation output)
ROM_MAX_PC = 76128

# Diagnostic sentinel addresses (word addresses in RAM_MAP)
DIAG_SENTINEL_WORD = 0xE0000 >> 2   # 0x38000
DIAG_OPCODE_WORD   = 0xE0004 >> 2   # 0x38001
DIAG_PC_WORD       = 0xE0008 >> 2   # 0x38002
DIAG_VALUE_WORD    = 0xE000C >> 2   # 0x38003
DIAG_SP_WORD       = 0xE0010 >> 2   # 0x38004
DIAG_EXTRA_WORD    = 0xE0014 >> 2   # 0x38005

# Sentinel values
DEAD_RESTART  = 0xDEAD0001
DEAD_UNMAPPED = 0xDEAD0002
DEAD_CALLR    = 0xDEAD0003

INITIAL_SP = 0x3F00000

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


def is_rom_fault(pc):
    """Check if PC is outside valid ROM bounds."""
    return pc >= ROM_MAX_PC


def read_diagnostics(bpf, ram_fd):
    """Read the diagnostic area at 0xE0000 from RAM_MAP."""
    diag = {}
    for name, word_addr in [
        ("sentinel", DIAG_SENTINEL_WORD),
        ("opcode", DIAG_OPCODE_WORD),
        ("pc", DIAG_PC_WORD),
        ("value", DIAG_VALUE_WORD),
        ("sp", DIAG_SP_WORD),
        ("extra", DIAG_EXTRA_WORD),
    ]:
        key = struct.pack("<I", word_addr)
        try:
            raw = bpf.lookup(ram_fd, key, 4)
            diag[name] = struct.unpack("<I", raw)[0]
        except OSError:
            diag[name] = 0
    return diag


def clear_diagnostics(bpf, ram_fd):
    """Clear the diagnostic area."""
    for word_addr in [DIAG_SENTINEL_WORD, DIAG_OPCODE_WORD, DIAG_PC_WORD,
                      DIAG_VALUE_WORD, DIAG_SP_WORD, DIAG_EXTRA_WORD]:
        key = struct.pack("<I", word_addr)
        bpf.update(ram_fd, key, struct.pack("<I", 0))


def reset_cpu(bpf, cpu_fd, key):
    """Reset CPU state to initial values (PC=0, SP=INITIAL_SP, halted=0)."""
    state = bytearray(CPU_STATE_SIZE)
    # regs[15] (SP) at offset 60 (15 * 4)
    struct.pack_into("<I", state, 60, INITIAL_SP)
    # PC at offset 64 = 0
    # halted at offset 69 = 0
    # insn_count at offset 80 = 0
    bpf.update(cpu_fd, key, bytes(state))


def main():
    parser = argparse.ArgumentParser(description="Doom-over-IPv6 execution driver")
    parser.add_argument("--map-dir", default=DEFAULT_MAP_DIR, help="BPF map pin directory")
    parser.add_argument("--iface", default="veth01", help="Injection interface")
    parser.add_argument("--batch", type=int, default=200, help="Packets per burst")
    parser.add_argument("--frames", type=int, default=0, help="Stop after N frames (0=unlimited)")
    parser.add_argument("--forever", action="store_true", help="Run until interrupted")
    parser.add_argument("--instance", type=lambda x: int(x, 0), default=0xDE, help="CPU instance")
    parser.add_argument("--namespace", default="monad0", help="Network namespace for injection")
    parser.add_argument("--auto-restart", action="store_true", default=True,
                        help="Auto-restart CPU on ROM fault (default: true)")
    parser.add_argument("--no-auto-restart", dest="auto_restart", action="store_false",
                        help="Stop on ROM fault instead of restarting")
    args = parser.parse_args()

    bpf = BPFHelper()
    cpu_fd = bpf.open_pinned(os.path.join(args.map_dir, "CPU_MAP"))
    ram_fd = bpf.open_pinned(os.path.join(args.map_dir, "RAM_MAP"))
    key = struct.pack("<I", args.instance)

    # Open injection socket
    sock = None
    ns_fd = None
    try:
        sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
        sock.bind((args.iface, 0))
    except OSError:
        if sock:
            sock.close()
        ns_path = f"/var/run/netns/{args.namespace}"
        if not os.path.exists(ns_path):
            print(f"ERROR: Cannot find namespace {args.namespace} and interface {args.iface}", file=sys.stderr)
            print("Run with: sudo python3 scripts/doom/run.py", file=sys.stderr)
            sys.exit(1)
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
    faults = 0
    restarts = 0
    start = time.time()
    last_insn_count = 0
    stall_count = 0

    print(f"=== Doom Execution Driver ===")
    print(f"  Batch: {args.batch} pkts, Instance: 0x{args.instance:X}")
    print(f"  Target frames: {'unlimited' if args.frames == 0 else args.frames}")
    print(f"  ROM size: {ROM_MAX_PC} instructions")
    print(f"  Auto-restart: {args.auto_restart}")
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
        sp = struct.unpack_from("<I", raw, 60)[0]
        insn_count = struct.unpack_from("<Q", raw, 80)[0]

        if halted:
            # Check diagnostic sentinel to distinguish syscall halt from fault halt.
            # ROM faults (RET-to-0, unmapped JMPR/CALLR, out-of-bounds PC) write
            # a sentinel to 0xE0000 before halting. Syscall halts don't.
            diag = read_diagnostics(bpf, ram_fd)
            sentinel = diag.get("sentinel", 0)
            is_fault = is_rom_fault(pc) or sentinel in (DEAD_RESTART, DEAD_UNMAPPED, DEAD_CALLR)

            if is_fault:
                faults += 1
                sentinel_names = {
                    DEAD_RESTART: "RET-to-0/restart",
                    DEAD_UNMAPPED: "unmapped JMPR",
                    DEAD_CALLR: "unmapped CALLR",
                }
                fault_name = sentinel_names.get(sentinel, f"ROM out-of-bounds")
                print(
                    f"\n  *** ROM FAULT #{faults}: {fault_name} ***\n"
                    f"      PC=0x{pc:X}, SP=0x{sp:X}, insns={insn_count:,}\n"
                    f"      diag: opc=0x{diag.get('opcode',0):02X}, "
                    f"fault_pc=0x{diag.get('pc',0):X}, "
                    f"val=0x{diag.get('value',0):X}, "
                    f"sp=0x{diag.get('sp',0):X}, "
                    f"extra=0x{diag.get('extra',0):X}",
                    flush=True,
                )

                if args.auto_restart:
                    restarts += 1
                    print(f"      -> Auto-restarting CPU (restart #{restarts})...", flush=True)
                    reset_cpu(bpf, cpu_fd, key)
                    clear_diagnostics(bpf, ram_fd)
                    stall_count = 0
                    last_insn_count = 0
                    print(f"      -> CPU reset: PC=0, SP=0x{INITIAL_SP:X}\n", flush=True)
                else:
                    print("      -> Stopping (use --auto-restart to continue)", flush=True)
                    break
            else:
                # Normal syscall halt — resume execution
                halts += 1
                frames += 1
                total_insns = insn_count

                # Clear halt flag
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

                # Detect instruction stall (same insn count for many frames)
                if insn_count == last_insn_count:
                    stall_count += 1
                    if stall_count >= 100:
                        print(
                            f"\n  *** STALL DETECTED: insn count stuck at {insn_count:,} "
                            f"for {stall_count} frames, PC=0x{pc:X} ***",
                            flush=True,
                        )
                        stall_count = 0  # Reset to avoid spam
                else:
                    stall_count = 0
                last_insn_count = insn_count
        else:
            # CPU still running, inject more
            time.sleep(0.0001)  # Brief pause

    sock.close()
    if ns_fd is not None:
        os.close(ns_fd)
    os.close(cpu_fd)
    os.close(ram_fd)

    elapsed = time.time() - start
    fps = frames / elapsed if elapsed > 0 else 0
    print(f"\n=== SUMMARY ===")
    print(f"  Frames: {frames}")
    print(f"  Halts: {halts}")
    print(f"  ROM faults: {faults}")
    print(f"  Restarts: {restarts}")
    print(f"  Packets: {total_pkts:,}")
    print(f"  Instructions: {total_insns:,}")
    print(f"  Time: {elapsed:.1f}s")
    print(f"  FPS: {fps:.1f}")
    if _shutdown:
        print("  (interrupted by signal)")


if __name__ == "__main__":
    main()
