#\!/usr/bin/env python3
"""Ultra-fast packet injector with no delay."""
import socket
import struct
import sys
import time

count = int(sys.argv[1]) if len(sys.argv) > 1 else 10000
flow_label = 0xDE

src_mac = bytes([0x02,0x42,0xac,0x11,0x00,0x02])
dst_mac = bytes([0x02,0x42,0xac,0x11,0x00,0x03])
version_tc_fl = (6 << 28) | (flow_label & 0xFFFFF)
eth = dst_mac + src_mac + struct.pack(">H", 0x86DD)
src_addr = bytes([0xfd,0x00,0x00,0x3f,0x00,0x75,0x00,0x00,
                  0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x01])
dst_addr = bytes([0xfd,0x00,0xde,0xad,0x00,0x00,0x00,0x00,
                  0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x01])
ipv6 = struct.pack(">IHBB", version_tc_fl, 24, 0, 255) + src_addr + dst_addr
hbh = struct.pack("BBBB", 59, 2, 0x3E, 20)
monad = struct.pack("BBBBBBBB", 0x01, 0, 0, 0, 0, 0, 0, 0x02)
monad += struct.pack(">H", 0) + struct.pack("BBBB", 0,0,0,0)
monad += struct.pack("BBBB", 0,0,0,0) + struct.pack("BB", 0, 0)
packet = eth + ipv6 + hbh + monad

sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
sock.bind(("veth01", 0))

start = time.time()
for i in range(count):
    sock.send(packet)
elapsed = time.time() - start
rate = count / elapsed if elapsed > 0 else 0
print(f"{count} pkts in {elapsed:.2f}s ({rate:.0f} pkt/s, {count*16:,} insns)")
sock.close()
