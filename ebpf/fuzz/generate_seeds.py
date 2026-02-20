#!/usr/bin/env python3
"""
LICH Fuzzing Campaign Seed Corpus Generator

Generates seed corpus files for the following LICH campaigns:
- LICH-007: MBC Bytecode Instruction Fuzzer
- LICH-008: Wotan L1 Cache Race Condition Harness
- LICH-009: Flow Label Birthday Attack Harness
- LICH-010: WAL Compaction Race Harness

Each seed is a carefully crafted binary file representing edge cases,
common patterns, and adversarial inputs for each fuzzing target.
"""

import os
import struct
import random
from pathlib import Path


def ensure_dir(path):
    """Create directory if it doesn't exist."""
    os.makedirs(path, exist_ok=True)


def write_seed(path, data):
    """Write binary seed file."""
    with open(path, 'wb') as f:
        f.write(data)


# ============================================================================
# LICH-007: MBC Bytecode Instruction Seeds
# ============================================================================

def generate_lich_007_seeds():
    """
    Generate 20 seed corpus files for MBC bytecode fuzzer.
    Each instruction is 4 bytes. Seeds vary from 1 to 16 instructions.
    """
    seed_dir = Path('/sessions/hopeful-kind-lovelace/mnt/tmp/unheaded/ebpf/fuzz/seeds/lich_007_mbc')
    ensure_dir(seed_dir)

    seeds = []

    # Seed 0: NOP instruction (opcode 0x00, no operands)
    seeds.append(bytes([0x00, 0x00, 0x00, 0x00]))

    # Seed 1: LOAD_IMM r0, 42 (opcode 0x10, reg=0, imm=2)
    seeds.append(bytes([0x10, 0x20, 0x00, 0x00]))

    # Seed 2: STORE r0, [0x100] (opcode 0x30, reg=0, offset=0x0100)
    seeds.append(bytes([0x30, 0x00, 0x01, 0x00]))

    # Seed 3: ADD r0, r1 (opcode 0x70, r0=0, r1=1)
    seeds.append(bytes([0x70, 0x10, 0x00, 0x00]))

    # Seed 4: JMP +4 (opcode 0x01, offset=+4)
    seeds.append(struct.pack('<BxxxI', 0x01, 4))

    # Seed 5: JEQ r0, 0, +2 (opcode 0x02, offset=+2)
    seeds.append(struct.pack('<BxxxI', 0x02, 2))

    # Seed 6: Multi-instruction: LOAD + STORE (2 instructions)
    seeds.append(bytes([0x10, 0x20, 0x00, 0x00,  # LOAD_IMM r0, 42
                        0x30, 0x00, 0x01, 0x00]))  # STORE r0, [0x100]

    # Seed 7: Multi-instruction: ADD + STORE (2 instructions)
    seeds.append(bytes([0x70, 0x10, 0x00, 0x00,  # ADD r0, r1
                        0x30, 0x00, 0x02, 0x00]))  # STORE r0, [0x200]

    # Seed 8: Control flow: JMP + NOP (2 instructions)
    seeds.append(struct.pack('<BxxxI', 0x01, 4) + bytes([0x00, 0x00, 0x00, 0x00]))

    # Seed 9: Stack operation: PUSH (opcode 0x50)
    seeds.append(bytes([0x50, 0x00, 0x00, 0x00]))

    # Seed 10: Stack operation: POP (opcode 0x51)
    seeds.append(bytes([0x51, 0x00, 0x00, 0x00]))

    # Seed 11: Register arithmetic: SUB (opcode 0x71)
    seeds.append(bytes([0x71, 0x21, 0x00, 0x00]))

    # Seed 12: Register arithmetic: MUL (opcode 0x72)
    seeds.append(bytes([0x72, 0x32, 0x00, 0x00]))

    # Seed 13: 4-instruction sequence: LOAD + ADD + STORE + RET
    seeds.append(bytes([0x10, 0x20, 0x00, 0x00,  # LOAD_IMM r0, 42
                        0x70, 0x10, 0x00, 0x00,  # ADD r0, r1
                        0x30, 0x00, 0x04, 0x00,  # STORE r0, [0x400]
                        0xFF, 0x00, 0x00, 0x00]))  # RET (opcode 0xFF)

    # Seed 14: Edge case: Jump to boundary (offset near u32 max)
    seeds.append(struct.pack('<BxxxI', 0x01, 0xFFFFFFFF))

    # Seed 15: Edge case: Jump backward (negative offset)
    seeds.append(struct.pack('<Bxxxi', 0x01, -100))

    # Seed 16: Memory load with max register (r15)
    seeds.append(bytes([0x30, 0x0F, 0x10, 0x00]))  # LOAD r15, [0x1000]

    # Seed 17: Edge case: All register operations r0-r15
    ops = b''
    for i in range(4):  # 4 operations of register pairs
        ops += bytes([0x70, 0x00 + (i << 4) + ((i+1) % 16), 0x00, 0x00])
    seeds.append(ops)

    # Seed 18: Complex: Alternating read/write memory ops
    ops = b''
    for i in range(4):
        if i % 2 == 0:
            ops += bytes([0x30, i & 0x0F, (i << 6) & 0xFF, 0x00])  # STORE
        else:
            ops += bytes([0x31, i & 0x0F, (i << 6) & 0xFF, 0x00])  # LOAD
    seeds.append(ops)

    # Seed 19: Maximum length seed (16 instructions, 64 bytes)
    ops = b''
    for i in range(16):
        ops += bytes([0x10 + (i % 8), (i << 4) & 0xFF, i & 0xFF, (i >> 8) & 0xFF])
    seeds.append(ops)

    # Write all seeds
    for i, seed in enumerate(seeds):
        write_seed(seed_dir / f'seed_{i:03d}.bin', seed)

    print(f'[LICH-007] Generated {len(seeds)} MBC bytecode seeds')
    return len(seeds)


# ============================================================================
# LICH-008: Wotan Cache Operation Seeds
# ============================================================================

def generate_lich_008_seeds():
    """
    Generate 20 seed corpus files for Wotan cache fuzzer.
    Format: [op_type:1][cache_line:4][data:N]
    op_type: 0=read, 1=write, 2=CAS
    """
    seed_dir = Path('/sessions/hopeful-kind-lovelace/mnt/tmp/unheaded/ebpf/fuzz/seeds/lich_008_wotan_cache')
    ensure_dir(seed_dir)

    seeds = []
    random.seed(42)  # Deterministic randomness

    # Seed 0: Single read operation
    seeds.append(bytes([0x00]) + struct.pack('<I', 0x1000) + b'data')

    # Seed 1: Single write operation
    seeds.append(bytes([0x01]) + struct.pack('<I', 0x1000) + b'data')

    # Seed 2: CAS (Compare-and-Swap) operation
    seeds.append(bytes([0x02]) + struct.pack('<I', 0x1000) + b'data')

    # Seed 3: Sequential reads on same cache line
    op = b''
    for i in range(4):
        op += bytes([0x00]) + struct.pack('<I', 0x1000) + b'data'
    seeds.append(op)

    # Seed 4: Sequential writes on same cache line
    op = b''
    for i in range(4):
        op += bytes([0x01]) + struct.pack('<I', 0x1000) + b'data'
    seeds.append(op)

    # Seed 5: Read-Write-Read pattern
    seeds.append(bytes([0x00]) + struct.pack('<I', 0x1000) + b'r1' +
                 bytes([0x01]) + struct.pack('<I', 0x1000) + b'w1' +
                 bytes([0x00]) + struct.pack('<I', 0x1000) + b'r2')

    # Seed 6: Multiple cache lines, random ops
    op = b''
    for i in range(8):
        addr = 0x1000 + (i * 64)
        op_type = random.randint(0, 2)
        op += bytes([op_type]) + struct.pack('<I', addr) + f'line_{i}'.encode()
    seeds.append(op)

    # Seed 7: High contention same line (12 operations)
    op = b''
    for i in range(12):
        op += bytes([1]) + struct.pack('<I', 0x2000) + struct.pack('<H', i)
    seeds.append(op)

    # Seed 8: CAS-heavy workload
    op = b''
    for i in range(10):
        op += bytes([2]) + struct.pack('<I', 0x3000) + struct.pack('<I', i)
    seeds.append(op)

    # Seed 9: Edge case: address boundary
    seeds.append(bytes([0x01]) + struct.pack('<I', 0x00000000) + b'boundary')  # Min address

    # Seed 10: Edge case: max address within data memory (0xBFFF for 48KB)
    seeds.append(bytes([0x01]) + struct.pack('<I', 0x0000BFFF) + b'maxaddr')

    # Seed 11: Alternating read/write on adjacent cache lines
    op = b''
    for i in range(8):
        addr = 0x4000 + (i * 64)
        op_type = i % 2  # Alternate 0=read, 1=write
        op += bytes([op_type]) + struct.pack('<I', addr) + f'adj_{i}'.encode()
    seeds.append(op)

    # Seed 12: Cache line collision: same address, different data
    op = b''
    for i in range(6):
        op += bytes([0x01]) + struct.pack('<I', 0x5000) + f'collision_{i}'.encode()
    seeds.append(op)

    # Seed 13: Long sequence: mixed operations (20 ops)
    op = b''
    for i in range(20):
        addr = 0x6000 + ((i % 4) * 128)
        op_type = i % 3  # Cycle through 0,1,2
        op += bytes([op_type]) + struct.pack('<I', addr) + struct.pack('<B', i)
    seeds.append(op)

    # Seed 14: Cache warming: sequential write then read
    op = b''
    for i in range(10):
        op += bytes([0x01]) + struct.pack('<I', 0x7000 + i*64) + f'warm_{i}'.encode()
    for i in range(10):
        op += bytes([0x00]) + struct.pack('<I', 0x7000 + i*64) + b'read'
    seeds.append(op)

    # Seed 15: Cache trashing: overlapping addresses
    op = b''
    for i in range(16):
        addr = 0x8000 + (i % 8) * 8  # Overlapping by 8 bytes
        op += bytes([i % 3]) + struct.pack('<I', addr) + struct.pack('<H', i)
    seeds.append(op)

    # Seed 16: Memory barrier simulation (CAS heavy then reads)
    op = bytes([0x02]) * 8 + struct.pack('<I', 0x9000) + b'barrier'
    op += bytes([0x00]) * 4 + struct.pack('<I', 0x9000) + b'after'
    seeds.append(op)

    # Seed 17: Sparse access pattern
    op = b''
    addrs = [0x0A00, 0x2B00, 0x5C00, 0x9D00, 0xAE00]
    for addr in addrs:
        op += bytes([1]) + struct.pack('<I', addr & 0xBFFF) + f'sparse_{addr:04x}'.encode()
    seeds.append(op)

    # Seed 18: Rapid fire writes (burst traffic)
    op = b''
    for i in range(32):
        op += bytes([1]) + struct.pack('<I', 0xA000) + struct.pack('<I', i)
    seeds.append(op)

    # Seed 19: All operation types mixed
    op = b''
    for i in range(15):
        op += bytes([i % 3]) + struct.pack('<I', (0xB000 + i*64) & 0xBFFF) + f'mixed_{i:02d}'.encode()
    seeds.append(op)

    # Write all seeds
    for i, seed in enumerate(seeds):
        write_seed(seed_dir / f'seed_{i:03d}.bin', seed)

    print(f'[LICH-008] Generated {len(seeds)} Wotan cache operation seeds')
    return len(seeds)


# ============================================================================
# LICH-009: Flow Label Collision Seeds
# ============================================================================

def generate_lich_009_seeds():
    """
    Generate 50 seed corpus files for flow label birthday attack fuzzer.
    Each seed is one or more IPv4 5-tuples: [src_ip:4][dst_ip:4][src_port:2][dst_port:2][proto:1][pad:3]
    Total 16 bytes per 5-tuple.
    """
    seed_dir = Path('/sessions/hopeful-kind-lovelace/mnt/tmp/unheaded/ebpf/fuzz/seeds/lich_009_flow_collision')
    ensure_dir(seed_dir)

    seeds = []
    random.seed(42)

    def make_5tuple(src_ip, dst_ip, src_port, dst_port, proto):
        """Create a 16-byte 5-tuple."""
        return (struct.pack('<I', src_ip) + struct.pack('<I', dst_ip) +
                struct.pack('<H', src_port) + struct.pack('<H', dst_port) +
                bytes([proto]) + bytes([0, 0, 0]))

    # Seed 0: Single 5-tuple, localhost loopback
    seeds.append(make_5tuple(0x7F000001, 0x7F000001, 8000, 8000, 6))  # 127.0.0.1, TCP

    # Seed 1: Single 5-tuple, random IPs
    seeds.append(make_5tuple(0x0A000001, 0x0A000002, 1000, 2000, 17))  # 10.0.0.1-2, UDP

    # Seed 2: Same subnet variations (10.0.0.x)
    op = b''
    for i in range(4):
        op += make_5tuple(0x0A000001 + i, 0x0A000101 + i, 3000 + i, 4000 + i, 6)
    seeds.append(op)

    # Seed 3: Same port different IPs
    op = b''
    for i in range(4):
        op += make_5tuple(0xC0A80001 + i, 0xC0A80101 + i, 5000, 5000, 6)  # 192.168.x
    seeds.append(op)

    # Seed 4: Same IPs different ports
    op = b''
    for i in range(6):
        op += make_5tuple(0x08080808, 0x08080404, 6000 + i*10, 7000 + i*10, 6)
    seeds.append(op)

    # Seed 5: Systematic collision candidates (Hamming distance ≤ 4)
    # Birthday attack targets: similar-looking flow labels
    base_ip = 0x7F000001
    op = b''
    for xor_mask in [0x00000001, 0x00000002, 0x00000004, 0x00000008, 0x00000010]:
        op += make_5tuple(base_ip, base_ip ^ xor_mask, 9000, 9000, 6)
    seeds.append(op)

    # Seed 6: High-entropy combinations
    op = b''
    for i in range(8):
        src = random.randint(0, 0xFFFFFFFF)
        dst = random.randint(0, 0xFFFFFFFF)
        src_port = random.randint(1024, 65535)
        dst_port = random.randint(1024, 65535)
        proto = random.choice([6, 17, 1])  # TCP, UDP, ICMP
        op += make_5tuple(src, dst, src_port, dst_port, proto)
    seeds.append(op)

    # Seed 7: Class A addresses
    op = b''
    for i in range(4):
        src = 0x01000000 + (i << 8)
        dst = 0x02000000 + (i << 8)
        op += make_5tuple(src, dst, 1000 + i, 2000 + i, 6)
    seeds.append(op)

    # Seed 8: Class B addresses (172.16.0.0/12)
    op = b''
    for i in range(4):
        src = 0xAC100000 + (i << 8)
        dst = 0xAC100000 + ((i+1) << 8)
        op += make_5tuple(src, dst, 3000 + i, 4000 + i, 6)
    seeds.append(op)

    # Seed 9: Class C addresses (192.168.1.0/24)
    op = b''
    for i in range(8):
        src = 0xC0A80100 + i
        dst = 0xC0A80100 + ((i+1) % 8)
        op += make_5tuple(src, dst, 5000 + i, 6000 + i, 17)  # UDP
    seeds.append(op)

    # Seed 10: Multicast addresses (224.0.0.0/4)
    op = b''
    for i in range(4):
        src = 0xE0000000 + (i << 8)
        dst = 0xE0000000 + ((i+1) << 8)
        op += make_5tuple(src, dst, 5353, 5353, 17)  # mDNS
    seeds.append(op)

    # Seed 11: Broadcast addresses
    op = b''
    for i in range(3):
        op += make_5tuple(0xFFFFFFFF, 0xC0A8FFFF, 67, 68, 17)  # DHCP
    seeds.append(op)

    # Seed 12: All zeros/all ones
    seeds.append(make_5tuple(0x00000000, 0x00000000, 0, 0, 0))

    # Seed 13: All ones
    seeds.append(make_5tuple(0xFFFFFFFF, 0xFFFFFFFF, 65535, 65535, 255))

    # Seed 14: Sequential addresses
    op = b''
    for i in range(10):
        op += make_5tuple(0x0A000001 + i, 0x0A000011 + i, 10000 + i, 20000 + i, 6)
    seeds.append(op)

    # Seed 15-49: Additional diverse patterns (35 seeds)
    # Birthday attack seeds: carefully selected pairs with high collision probability
    for seed_idx in range(35):
        op = b''
        pattern = seed_idx % 7

        if pattern == 0:  # Private address space 10.0.0.0/8
            for j in range(5):
                ip = 0x0A000000 + random.randint(0, 0x00FFFFFF)
                op += make_5tuple(ip, ip ^ 0x00010101, 10000+j, 20000+j, 6)
        elif pattern == 1:  # Private address space 172.16.0.0/12
            for j in range(5):
                ip = 0xAC100000 + random.randint(0, 0x0000FFFF)
                op += make_5tuple(ip, ip ^ 0x00001111, 10000+j, 20000+j, 6)
        elif pattern == 2:  # Private address space 192.168.0.0/16
            for j in range(5):
                ip = 0xC0A80000 + random.randint(0, 0x0000FFFF)
                op += make_5tuple(ip, ip ^ 0x00000101, 10000+j, 20000+j, 6)
        elif pattern == 3:  # Link-local 169.254.0.0/16
            for j in range(5):
                ip = 0xA9FE0000 + random.randint(0, 0x0000FFFF)
                op += make_5tuple(ip, ip ^ 0x00000202, 10000+j, 20000+j, 6)
        elif pattern == 4:  # Loop-back 127.0.0.0/8
            for j in range(5):
                ip = 0x7F000000 + random.randint(0, 0x00FFFFFF)
                op += make_5tuple(ip, ip ^ 1, 10000+j, 20000+j, 6)
        elif pattern == 5:  # Common protocols and ports (DNS, HTTP, HTTPS, etc)
            common_ports = [53, 80, 443, 8080, 3306, 5432, 27017]
            for port in common_ports[:5]:
                op += make_5tuple(0xC0A80001, 0x08080808, port, port, 6 if port > 100 else 17)
        else:  # Random pattern with deliberate hash function input similarity
            base = random.randint(0, 0xFFFFFF00)
            for j in range(5):
                src = base + j
                dst = base + j + 0x00000100
                op += make_5tuple(src, dst, 60000 + j*100, 60000 + j*100, 6)

        seeds.append(op)

    # Write all seeds
    for i, seed in enumerate(seeds):
        write_seed(seed_dir / f'seed_{i:03d}.bin', seed)

    print(f'[LICH-009] Generated {len(seeds)} flow collision 5-tuple seeds')
    return len(seeds)


# ============================================================================
# LICH-010: WAL Integrity Seeds
# ============================================================================

def generate_lich_010_seeds():
    """
    Generate 30 seed corpus files for WAL compaction race fuzzer.
    Each Anamnesis event is 32 bytes:
    [timestamp_ns:8][event_type:1][hop_index:1][reserved:2][monad_before:20]
    """
    seed_dir = Path('/sessions/hopeful-kind-lovelace/mnt/tmp/unheaded/ebpf/fuzz/seeds/lich_010_wal_integrity')
    ensure_dir(seed_dir)

    seeds = []
    random.seed(42)

    def make_wal_event(timestamp_ns, event_type, hop_index, monad_before_data):
        """Create a 32-byte WAL event."""
        monad = monad_before_data[:20] if isinstance(monad_before_data, bytes) else b'\x00' * 20
        monad = (monad + b'\x00' * 20)[:20]  # Pad or truncate to 20 bytes
        return (struct.pack('<Q', timestamp_ns) +
                bytes([event_type]) +
                bytes([hop_index]) +
                bytes([0, 0]) +
                monad)

    # Seed 0: Single event, timestamp 0
    seeds.append(make_wal_event(0, 1, 0, b'monad_00'))

    # Seed 1: Single event, timestamp 1000
    seeds.append(make_wal_event(1000, 2, 1, b'monad_01'))

    # Seed 2: Sequential events with increasing timestamps
    op = b''
    for i in range(10):
        op += make_wal_event(i * 1000, (i % 4) + 1, i % 256, f'event_{i:02d}'.encode())
    seeds.append(op)

    # Seed 3: Rapid-fire events (same timestamp)
    op = b''
    for i in range(16):
        op += make_wal_event(5000, i % 4 + 1, i % 256, f'rapid_{i:02d}'.encode())
    seeds.append(op)

    # Seed 4: Wrap-around: maximum uint64 timestamp
    seeds.append(make_wal_event(0xFFFFFFFFFFFFFFFF, 1, 0, b'max_timestamp'))

    # Seed 5: Wrap-around: near max with multiple events
    op = b''
    for i in range(8):
        ts = 0xFFFFFFFFFFFFFFFF - (7 - i) * 100
        op += make_wal_event(ts, (i % 4) + 1, i, f'wrap_{i:02d}'.encode())
    seeds.append(op)

    # Seed 6: Timestamp resets (backward jumps)
    op = b''
    timestamps = [1000, 5000, 3000, 8000, 2000, 9000, 1500]
    for i, ts in enumerate(timestamps):
        op += make_wal_event(ts, (i % 4) + 1, i, f'reset_{i:02d}'.encode())
    seeds.append(op)

    # Seed 7: Events with all event types (0-255)
    op = b''
    for i in range(16):  # Limit to 16 to keep reasonable size
        op += make_wal_event(i * 1000, i % 256, i % 256, f'type_{i:02d}'.encode())
    seeds.append(op)

    # Seed 8: All hop indices (0-255)
    op = b''
    for i in range(16):
        op += make_wal_event(i * 1000, 1, i % 256, f'hop_{i:02d}'.encode())
    seeds.append(op)

    # Seed 9: Concurrent writer simulation (interleaved timestamps)
    op = b''
    for writer_id in range(3):
        for event_id in range(5):
            ts = (writer_id * 1000000) + (event_id * 100)
            op += make_wal_event(ts, writer_id + 1, event_id, f'writer_{writer_id}_{event_id}'.encode())
    seeds.append(op)

    # Seed 10: Compaction trigger pattern (many events then gap)
    op = b''
    for i in range(20):
        op += make_wal_event(i * 500, (i % 4) + 1, i % 256, f'compact_{i:02d}'.encode())
    # Gap of 10000ns
    for i in range(5):
        op += make_wal_event(10000 + (i * 500), (i % 4) + 1, i, f'after_compact_{i:02d}'.encode())
    seeds.append(op)

    # Seed 11: Power failure recovery pattern (incomplete event)
    op = b''
    for i in range(15):
        op += make_wal_event(i * 1000, (i % 4) + 1, i, f'before_crash_{i:02d}'.encode())
    # Incomplete final event (truncated)
    op += make_wal_event(15000, 3, 15, b'crash')[:20]
    seeds.append(op)

    # Seed 12: High-frequency writes
    op = b''
    for i in range(30):
        op += make_wal_event(i * 10, (i % 4) + 1, i % 256, f'hfreq_{i:02d}'.encode())
    seeds.append(op)

    # Seed 13: Low-frequency writes (sparse)
    op = b''
    for i in range(5):
        op += make_wal_event(i * 100000000, (i % 4) + 1, i, f'sparse_{i:02d}'.encode())
    seeds.append(op)

    # Seed 14: Monad data variations
    op = b''
    monad_patterns = [
        b'\x00' * 20,
        b'\xFF' * 20,
        b'\xAA\xBB' * 10,
        b'\x01\x02\x03\x04\x05' * 4,
        bytes(range(20)),
    ]
    for i, monad in enumerate(monad_patterns):
        op += make_wal_event(i * 1000, i % 4 + 1, i, monad)
    seeds.append(op)

    # Seed 15: Edge case: single large timestamp jump
    seeds.append(make_wal_event(100, 1, 0, b'small'))
    seeds.append(make_wal_event(0xFFFFFFFF00000000, 1, 0, b'huge_jump'))
    seeds.append(b'')  # Combine into single seed
    op = seeds.pop() + seeds.pop() + seeds.pop()
    seeds.append(op)

    # Seed 16-29: Stress patterns (14 seeds)
    for seed_idx in range(14):
        op = b''
        pattern = seed_idx % 7

        if pattern == 0:  # Increasing pattern
            for i in range(25):
                op += make_wal_event(i * 1000 + seed_idx * 100000, (i % 4) + 1, i % 256, f'inc_{i:02d}'.encode())
        elif pattern == 1:  # Decreasing pattern
            for i in range(25):
                op += make_wal_event((25 - i) * 1000 + seed_idx * 100000, (i % 4) + 1, i % 256, f'dec_{i:02d}'.encode())
        elif pattern == 2:  # Burst pattern
            for burst_num in range(5):
                for i in range(5):
                    ts = seed_idx * 1000000 + burst_num * 10000 + i
                    op += make_wal_event(ts, (i % 4) + 1, burst_num % 256, f'burst_{burst_num}_{i}'.encode())
        elif pattern == 3:  # Alternating event types
            for i in range(20):
                op += make_wal_event(i * 2000 + seed_idx * 100000, 1 + (i % 4), i % 256, f'alt_{i:02d}'.encode())
        elif pattern == 4:  # Random timestamps
            for i in range(20):
                ts = random.randint(0, 0x7FFFFFFFFFFFFFFF)
                op += make_wal_event(ts, random.randint(1, 4), random.randint(0, 255), f'rand_{i:02d}'.encode())
        elif pattern == 5:  # Clustered timestamps
            cluster_base = seed_idx * 1000000
            for cluster_id in range(4):
                for i in range(5):
                    ts = cluster_base + cluster_id * 10000 + i
                    op += make_wal_event(ts, (i % 4) + 1, cluster_id * 64 % 256, f'cluster_{cluster_id}_{i}'.encode())
        else:  # Pathological: rapid same-timestamp events
            for i in range(30):
                op += make_wal_event(seed_idx * 1000000, (i % 256) % 4 + 1, i % 256, f'same_ts_{i:02d}'.encode())

        seeds.append(op)

    # Write all seeds
    for i, seed in enumerate(seeds):
        write_seed(seed_dir / f'seed_{i:03d}.bin', seed)

    print(f'[LICH-010] Generated {len(seeds)} WAL integrity seeds')
    return len(seeds)


# ============================================================================
# Main
# ============================================================================

def main():
    """Generate all LICH fuzzing campaign seed corpora."""
    print('=' * 70)
    print('LICH Fuzzing Campaign Seed Corpus Generator')
    print('=' * 70)
    print()

    total_seeds = 0

    # Generate LICH-007 seeds
    count_007 = generate_lich_007_seeds()
    total_seeds += count_007
    print()

    # Generate LICH-008 seeds
    count_008 = generate_lich_008_seeds()
    total_seeds += count_008
    print()

    # Generate LICH-009 seeds
    count_009 = generate_lich_009_seeds()
    total_seeds += count_009
    print()

    # Generate LICH-010 seeds
    count_010 = generate_lich_010_seeds()
    total_seeds += count_010
    print()

    print('=' * 70)
    print(f'Total seeds generated: {total_seeds}')
    print(f'  LICH-007 (MBC):           {count_007:3d} seeds')
    print(f'  LICH-008 (Cache):         {count_008:3d} seeds')
    print(f'  LICH-009 (Flow Collision): {count_009:3d} seeds')
    print(f'  LICH-010 (WAL Integrity):  {count_010:3d} seeds')
    print('=' * 70)
    print()
    print('Seed corpus generation complete!')
    print('Next steps:')
    print('  1. Verify seeds: ls -lh ebpf/fuzz/seeds/*/seed_*.bin')
    print('  2. Run LICH-007: cargo +nightly fuzz run lich_007_mbc ebpf/fuzz/seeds/lich_007_mbc')
    print('  3. Run LICH-008: cargo +nightly fuzz run lich_008_wotan_cache ebpf/fuzz/seeds/lich_008_wotan_cache')
    print('  4. Run LICH-009: cargo +nightly fuzz run lich_009_flow_collision ebpf/fuzz/seeds/lich_009_flow_collision')
    print('  5. Run LICH-010: cargo +nightly fuzz run lich_010_wal_integrity ebpf/fuzz/seeds/lich_010_wal_integrity')


if __name__ == '__main__':
    main()
