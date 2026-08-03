#!/usr/bin/env python3
"""
Unified CPU state tool for Doom-over-IPv6 ring.

Subcommands:
  read   — Read and display CPU_MAP state
  reset  — Reset CPU to initial state (PC=0, SP=0xFFFF0000)
  dump   — Raw hex dump of CPU_MAP (for scripting/piping)

Usage:
  sudo python3 scripts/doom/cpu_state.py read
  sudo python3 scripts/doom/cpu_state.py read --json
  sudo python3 scripts/doom/cpu_state.py read --instance 0xDE
  sudo python3 scripts/doom/cpu_state.py reset
  sudo python3 scripts/doom/cpu_state.py dump
"""
import argparse
import json
import re
import struct
import subprocess
import sys

DEFAULT_CPU_MAP = "/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP"
DEFAULT_INSTANCE = 0xDE
CPU_STATE_SIZE = 104  # bytes


def bpftool_lookup(map_path, key_bytes):
    """Read a BPF map entry via bpftool, return raw bytes."""
    key_args = [str(b) for b in key_bytes]
    cmd = [
        "bpftool", "map", "lookup", "pinned", map_path, "key"
    ] + key_args
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        print(f"ERROR: bpftool failed: {result.stderr.strip()}", file=sys.stderr)
        sys.exit(1)

    output = result.stdout

    # Try 0xNN format first
    hex_vals = re.findall(r"0x([0-9a-fA-F]{2})", output)
    if not hex_vals:
        # Try raw hex format from value: line
        val_match = re.search(r"value:\s*\n([\s0-9a-fA-F]+)", output)
        if val_match:
            hex_vals = re.findall(r"([0-9a-fA-F]{2})", val_match.group(1))

    if not hex_vals:
        # Try space-separated hex after "value:"
        lines = output.strip().split("\n")
        for line in lines:
            if "value:" in line:
                parts = line.split("value:")[1].strip().split()
                hex_vals.extend(parts)
            elif hex_vals and line.strip() and not line.startswith("key:"):
                hex_vals.extend(line.strip().split())

    if len(hex_vals) < CPU_STATE_SIZE:
        print(f"ERROR: Not enough bytes: got {len(hex_vals)}, need {CPU_STATE_SIZE}", file=sys.stderr)
        print(f"Raw output: {output[:500]}", file=sys.stderr)
        sys.exit(1)

    return bytes(int(h, 16) for h in hex_vals[:CPU_STATE_SIZE])


def instance_key(instance):
    """Convert instance ID to 4-byte little-endian key."""
    return list(instance.to_bytes(4, "little"))


def parse_cpu_state(raw):
    """Parse 104-byte CPU state into a dict."""
    regs = list(struct.unpack_from("<16I", raw, 0))
    pc = struct.unpack_from("<I", raw, 64)[0]
    flags = raw[68]
    halted = raw[69]
    stalled = raw[70]
    sleep_ns = struct.unpack_from("<Q", raw, 72)[0]
    insn_count = struct.unpack_from("<Q", raw, 80)[0]
    cache_hits = struct.unpack_from("<Q", raw, 88)[0]
    cache_misses = struct.unpack_from("<Q", raw, 96)[0]

    return {
        "registers": {f"r{i}": v for i, v in enumerate(regs)},
        "pc": pc,
        "flags": flags,
        "halted": halted,
        "stalled": stalled,
        "sleep_ns": sleep_ns,
        "insn_count": insn_count,
        "cache_hits": cache_hits,
        "cache_misses": cache_misses,
    }


def cmd_read(args):
    """Read and display CPU state."""
    key = instance_key(args.instance)
    raw = bpftool_lookup(args.map_path, key)
    state = parse_cpu_state(raw)

    if args.json:
        print(json.dumps(state, indent=2))
        return

    print(f"=== CPU STATE (instance 0x{args.instance:X}) === ({len(raw)} bytes)")
    print(f"  PC       = 0x{state['pc']:X} ({state['pc']})")
    print(f"  insns    = {state['insn_count']:,}")
    print(f"  halted   = {state['halted']}")
    print(f"  stalled  = {state['stalled']}")
    print(f"  flags    = 0x{state['flags']:02X}")
    print(f"  cache    = {state['cache_hits']:,} hits / {state['cache_misses']:,} misses")
    print(f"  SP (r15) = 0x{state['registers']['r15']:X}")
    print("  Registers:")
    for i in range(16):
        v = state["registers"][f"r{i}"]
        if v != 0 or i in (0, 1, 2, 3, 15):
            print(f"    r{i:2d} = 0x{v:08X} ({v})")


def cmd_reset(args):
    """Reset CPU to initial state."""
    state = bytearray(CPU_STATE_SIZE)
    # SP (r15) at offset 60 = 0xFFFF0000 little-endian
    struct.pack_into("<I", state, 60, 0xFFFF0000)

    key = instance_key(args.instance)
    cmd = (
        ["sudo", "bpftool", "map", "update", "pinned", args.map_path, "key"]
        + [str(k) for k in key]
        + ["value"]
        + [str(b) for b in state]
    )
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        print(f"ERROR: {result.stderr.strip()}", file=sys.stderr)
        sys.exit(1)

    print(f"CPU state reset for instance 0x{args.instance:X} (PC=0, SP=0xFFFF0000, halted=0)")
    if args.json:
        print(json.dumps({"instance": args.instance, "pc": 0, "sp": 0xFFFF0000, "halted": 0}))


def cmd_dump(args):
    """Raw hex dump of CPU state."""
    key = instance_key(args.instance)
    raw = bpftool_lookup(args.map_path, key)
    state = parse_cpu_state(raw)

    if args.json:
        print(json.dumps(state, indent=2))
    else:
        print(" ".join(f"{b:02x}" for b in raw))


def main():
    parser = argparse.ArgumentParser(
        description="CPU state tool for Doom-over-IPv6 ring"
    )
    parser.add_argument(
        "--map-path", default=DEFAULT_CPU_MAP, help="BPF CPU_MAP pin path"
    )
    parser.add_argument(
        "--instance", type=lambda x: int(x, 0), default=DEFAULT_INSTANCE,
        help="CPU instance ID (default: 0xDE)",
    )
    parser.add_argument(
        "--json", action="store_true", help="Output as JSON"
    )

    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("read", help="Read and display CPU state")
    sub.add_parser("reset", help="Reset CPU to initial state")
    sub.add_parser("dump", help="Raw dump of CPU state")

    args = parser.parse_args()

    if args.command == "read":
        cmd_read(args)
    elif args.command == "reset":
        cmd_reset(args)
    elif args.command == "dump":
        cmd_dump(args)


if __name__ == "__main__":
    main()
