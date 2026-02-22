#\!/usr/bin/env python3
"""Fast bulk packet injector for Doom ring. Run inside monad0 namespace.
Adds a small delay between packets to prevent drops."""
import socket, struct, sys, time

flow_label = int(sys.argv[1], 0) if len(sys.argv) > 1 else 0xDE
count = int(sys.argv[2]) if len(sys.argv) > 2 else 1000
src_mac_str = sys.argv[3] if len(sys.argv) > 3 else None
dst_mac_str = sys.argv[4] if len(sys.argv) > 4 else None
delay_us = int(sys.argv[5]) if len(sys.argv) > 5 else 3000

def mac_bytes(mac_str):
    return bytes(int(b, 16) for b in mac_str.split(":"))

src_mac = mac_bytes(src_mac_str) if src_mac_str else bytes([0x02, 0x42, 0xac, 0x11, 0x00, 0x02])
dst_mac = mac_bytes(dst_mac_str) if dst_mac_str else bytes([0x02, 0x42, 0xac, 0x11, 0x00, 0x03])

version_tc_fl = (6 << 28) | (0 << 20) | (flow_label & 0xFFFFF)
eth = dst_mac + src_mac + struct.pack(">H", 0x86DD)
src_addr = bytes([0xfd, 0x00, 0x00, 0x3f, 0x00, 0x75, 0x00, 0x00,
                  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01])
dst_addr = bytes([0xfd, 0x00, 0xde, 0xad, 0x00, 0x00, 0x00, 0x00,
                  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01])
ipv6 = struct.pack(">IHBB", version_tc_fl, 24, 0, 255) + src_addr + dst_addr
hbh = struct.pack("BBBB", 59, 2, 0x3E, 20)
# Monad register: exactly 20 bytes (matching doom-ring.sh)
monad = struct.pack("BBBBBBBB", 0x01, 0, 0, 0, 0, 0, 0, 0x02)  # 8 bytes
monad += struct.pack(">H", 0)                                     # 2 bytes: latency_hint
monad += struct.pack("BBBB", 0, 0, 0, 0)                         # 4 bytes: deploy_ring, mesh_flags, src_prefix, dst_prefix
monad += struct.pack("BBBB", 0, 0, 0, 0)                         # 4 bytes: scratch[0..3]
monad += struct.pack("BB", 0, 0)                                  # 2 bytes: checksum[0..1]
assert len(monad) == 20, f"Monad size mismatch: {len(monad)}"
packet = eth + ipv6 + hbh + monad
assert len(packet) == 78, f"Packet size mismatch: {len(packet)}"

sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
sock.bind(("veth01", 0))

delay_s = delay_us / 1_000_000.0
start = time.time()
for i in range(count):
    sock.send(packet)
    if delay_s > 0:
        time.sleep(delay_s)
    if (i + 1) % 1000 == 0:
        elapsed = time.time() - start
        rate = (i + 1) / elapsed
        insns = (i + 1) * 4080
        print(f"  [{(i+1)*100//count:3d}%] {i+1}/{count} pkt ({rate:.0f} pkt/s, ~{insns:,} insns)", flush=True)

elapsed = time.time() - start
rate = count / elapsed if elapsed > 0 else 0
total_insns = count * 4080
sock.close()
print(f"Injected {count} packets in {elapsed:.1f}s ({rate:.0f} pkt/s, ~{total_insns:,} insns expected)")
