#!/usr/bin/env python3
"""
Doom-over-IPv6 Packet Injection Tool for Monad Ring

Crafts and injects IPv6 + Hop-by-Hop + Monad register packets into the
Doom-over-IPv6 ring for CPU tick distribution and service coordination.

Packet Structure:
  - Ethernet frame (dst_mac + src_mac + 0x86DD)
  - IPv6 header (version 6, TC 0, flow_label, HBH next header)
  - Hop-by-Hop extension (next=ICMPv6, opt_type=0x3E, 20-byte Monad register)
  - Monad register (20 bytes with CRC-16/CCITT-FALSE checksum)

Monad Register Layout (20 bytes):
  Offset  Field
  0x00    version (0x01)
  0x01    src_service_id
  0x02    dst_service_id
  0x03    hop_count
  0x04-07 trace_id (u32 big-endian)
  0x08    qos_class
  0x09    flow_action
  0x0A    circuit_state
  0x0B    flags (0x02 = CUSTOM for CPU tick)
  0x0C-0D latency_budget_us (u16 big-endian)
  0x0E    deployment_ring
  0x0F    mesh_flags
  0x10-11 reserved (zero)
  0x12-13 checksum (CRC-16/CCITT over bytes 0x00-0x11)

References:
  - doom-ring.sh inline packet injection (lines 471-532)
  - Wotan service integration
  - IPv6 Hop-by-Hop extension (RFC 8200)
"""

import argparse
import os
import random
import signal
import socket
import struct
import sys
import time


class CRC16:
    """CRC-16/CCITT-FALSE calculator (poly=0x1021, init=0xFFFF, no reflect)."""

    def __init__(self, poly: int = 0x1021, init: int = 0xFFFF):
        self.poly = poly
        self.init = init
        self.table = self._build_table()

    def _build_table(self) -> list[int]:
        """Build lookup table for fast CRC computation."""
        table = []
        for i in range(256):
            crc = i << 8
            for _ in range(8):
                crc <<= 1
                if crc & 0x10000:
                    crc ^= self.poly
            table.append(crc & 0xFFFF)
        return table

    def compute(self, data: bytes) -> int:
        """Compute CRC-16 checksum for given data."""
        crc = self.init
        for byte in data:
            crc = ((crc << 8) ^ self.table[(crc >> 8) ^ byte]) & 0xFFFF
        return crc


class DoomPacket:
    """Constructs IPv6 + HBH + Monad packets for the Doom-over-IPv6 ring."""

    # Ethernet constants
    ETH_TYPE_IPV6 = 0x86DD

    # IPv6 constants
    IPV6_VERSION = 6
    IPV6_TC = 0
    IPV6_PAYLOAD_LEN = 24  # HBH extension header (8 bytes) + Monad register (20 bytes)
    IPV6_NEXT_HBH = 0  # Hop-by-Hop
    IPV6_HOP_LIMIT = 255

    # IPv6 addresses
    SRC_ADDR = "fd00:3f:75:0::1"
    DST_ADDR = "fd00:dead::1"  # Outside connected subnets (forces forwarding)

    # Hop-by-Hop extension header
    HBH_NEXT = 59  # ICMPv6
    HBH_EXT_LEN = 2  # (length + 1) * 8 = 24 bytes total
    HBH_OPT_TYPE = 0x3E  # Unrecognized, skip-on-unknown
    HBH_OPT_DATA_LEN = 20  # Monad register size

    # Monad register layout
    MONAD_VERSION = 0x01
    MONAD_FLAGS_CUSTOM = 0x02  # CPU tick flag
    MONAD_SIZE = 20

    # CRC-16/CCITT-FALSE
    crc_engine = CRC16(poly=0x1021, init=0xFFFF)

    def __init__(
        self,
        src_service_id: int = 0xFF,
        dst_service_id: int = 0xFF,
        hop_count: int = 1,
        trace_id: int = 0,
        qos_class: int = 0,
        flow_action: int = 0,
        circuit_state: int = 0,
        latency_budget_us: int = 0,
        deployment_ring: int = 0,
        mesh_flags: int = 0,
        flow_label: int = 0xDE,
    ):
        """
        Initialize packet generator.

        Args:
            src_service_id: Source service ID (1 byte)
            dst_service_id: Destination service ID (1 byte)
            hop_count: Hop count (1 byte)
            trace_id: Trace ID (4 bytes, big-endian)
            qos_class: QoS class (1 byte)
            flow_action: Flow action (1 byte)
            circuit_state: Circuit state (1 byte)
            latency_budget_us: Latency budget in microseconds (2 bytes, big-endian)
            deployment_ring: Deployment ring (1 byte)
            mesh_flags: Mesh flags (1 byte)
            flow_label: IPv6 flow label (20 bits, default 0xDE)
        """
        self.src_service_id = src_service_id
        self.dst_service_id = dst_service_id
        self.hop_count = hop_count
        self.trace_id = trace_id
        self.qos_class = qos_class
        self.flow_action = flow_action
        self.circuit_state = circuit_state
        self.latency_budget_us = latency_budget_us
        self.deployment_ring = deployment_ring
        self.mesh_flags = mesh_flags
        self.flow_label = flow_label & 0xFFFFF  # 20-bit field

    def build_monad_register(self) -> bytes:
        """
        Build the 20-byte Monad register with CRC-16 checksum.

        Returns:
            20 bytes of Monad register data
        """
        # Build register (without checksum)
        register = bytearray(20)
        register[0x00] = self.MONAD_VERSION
        register[0x01] = self.src_service_id
        register[0x02] = self.dst_service_id
        register[0x03] = self.hop_count
        register[0x04:0x08] = struct.pack(">I", self.trace_id)
        register[0x08] = self.qos_class
        register[0x09] = self.flow_action
        register[0x0A] = self.circuit_state
        register[0x0B] = self.MONAD_FLAGS_CUSTOM
        register[0x0C:0x0E] = struct.pack(">H", self.latency_budget_us)
        register[0x0E] = self.deployment_ring
        register[0x0F] = self.mesh_flags
        register[0x10:0x12] = b"\x00\x00"

        # Compute CRC-16 over bytes 0x00-0x11 (18 bytes)
        crc_data = bytes(register[0x00:0x12])
        checksum = self.crc_engine.compute(crc_data)
        register[0x12:0x14] = struct.pack(">H", checksum)

        return bytes(register)

    def build_hbh_extension(self, monad_data: bytes) -> bytes:
        """
        Build Hop-by-Hop extension header.

        Args:
            monad_data: 20-byte Monad register

        Returns:
            HBH extension header (8 bytes header + 20 bytes Monad = 28 bytes total)
        """
        hbh = bytearray(28)
        hbh[0] = self.HBH_NEXT
        hbh[1] = self.HBH_EXT_LEN
        hbh[2] = self.HBH_OPT_TYPE
        hbh[3] = self.HBH_OPT_DATA_LEN
        hbh[4:24] = monad_data
        # Padding to 8-byte boundary (already at 28 bytes)
        return bytes(hbh)

    def build_ipv6_header(self, payload: bytes) -> bytes:
        """
        Build IPv6 header.

        Args:
            payload: HBH extension header + Monad register

        Returns:
            40-byte IPv6 header
        """
        # IPv6 version (4 bits) + traffic class (8 bits) + flow label (20 bits)
        version_tc_label = (
            (self.IPV6_VERSION << 28)
            | (self.IPV6_TC << 20)
            | self.flow_label
        )

        src_addr = socket.inet_pton(socket.AF_INET6, self.SRC_ADDR)
        dst_addr = socket.inet_pton(socket.AF_INET6, self.DST_ADDR)

        header = struct.pack(
            ">IHB B 16s 16s",
            version_tc_label,
            len(payload),  # Payload length (HBH + Monad)
            self.IPV6_NEXT_HBH,
            self.IPV6_HOP_LIMIT,
            src_addr,
            dst_addr,
        )
        return header

    def build_ethernet_frame(
        self, ipv6_packet: bytes, dst_mac: bytes, src_mac: bytes
    ) -> bytes:
        """
        Build Ethernet frame.

        Args:
            ipv6_packet: Complete IPv6 packet (header + payload)
            dst_mac: Destination MAC address (6 bytes)
            src_mac: Source MAC address (6 bytes)

        Returns:
            Complete Ethernet frame
        """
        return dst_mac + src_mac + struct.pack(">H", self.ETH_TYPE_IPV6) + ipv6_packet

    def build(self, dst_mac: bytes, src_mac: bytes) -> bytes:
        """
        Build complete packet.

        Args:
            dst_mac: Destination MAC address (6 bytes)
            src_mac: Source MAC address (6 bytes)

        Returns:
            Complete Ethernet + IPv6 + HBH + Monad packet
        """
        monad = self.build_monad_register()
        hbh = self.build_hbh_extension(monad)
        ipv6 = self.build_ipv6_header(hbh)
        ipv6_packet = ipv6 + hbh
        ethernet = self.build_ethernet_frame(ipv6_packet, dst_mac, src_mac)
        return ethernet

    @staticmethod
    def hex_dump(data: bytes, width: int = 16) -> str:
        """Format bytes as hex dump with ASCII."""
        lines = []
        for i in range(0, len(data), width):
            chunk = data[i : i + width]
            hex_part = " ".join(f"{b:02x}" for b in chunk)
            ascii_part = "".join(chr(b) if 32 <= b < 127 else "." for b in chunk)
            lines.append(f"{i:04x}  {hex_part:<{width * 3}}  {ascii_part}")
        return "\n".join(lines)


class PacketInjector:
    """Injects packets into network interface via AF_PACKET socket."""

    def __init__(self, interface: str, namespace: str | None = None):
        """
        Initialize packet injector.

        Args:
            interface: Network interface name
            namespace: Network namespace (e.g., 'monad0')
        """
        self.interface = interface
        self.namespace = namespace
        self.socket = None
        self.if_index = None

    def __enter__(self):
        """Context manager entry."""
        self._open_socket()
        return self

    def __exit__(self, *args):
        """Context manager exit."""
        self.close()

    def _open_socket(self):
        """Open AF_PACKET socket (requires root)."""
        try:
            self.socket = socket.socket(
                socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x800)
            )
            self.if_index = socket.if_nametoindex(self.interface)
        except OSError as e:
            raise RuntimeError(
                f"Failed to open packet socket on {self.interface}: {e}\n"
                f"Note: AF_PACKET requires root privileges and valid interface.\n"
                f"Available interfaces: {self._list_interfaces()}"
            )

    def close(self):
        """Close the socket."""
        if self.socket:
            self.socket.close()

    @staticmethod
    def _list_interfaces() -> str:
        """List available network interfaces."""
        try:
            result = os.popen("ip link show | grep '^[0-9]' | awk '{print $2}'").read()
            return result.strip()
        except Exception:
            return "(unable to enumerate)"

    def inject(self, packet: bytes) -> None:
        """
        Inject packet into network interface.

        Args:
            packet: Complete Ethernet frame to send
        """
        if not self.socket:
            raise RuntimeError("Socket not open")

        try:
            self.socket.sendto(packet, (self.interface, 0))
        except OSError as e:
            raise RuntimeError(f"Failed to inject packet: {e}")

    def inject_namespace(self, packet: bytes) -> None:
        """
        Inject packet into network namespace.

        Args:
            packet: Complete Ethernet frame to send
        """
        if not self.namespace:
            self.inject(packet)
            return

        # Use ip netns exec to inject in namespace
        import subprocess
        import tempfile

        with tempfile.NamedTemporaryFile(delete=False, suffix=".bin") as f:
            f.write(packet)
            packet_file = f.name

        try:
            cmd = [
                "ip",
                "netns",
                "exec",
                self.namespace,
                "python3",
                "-c",
                (
                    "import socket; s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW); "
                    f"s.sendto(open('{packet_file}', 'rb').read(), ('{self.interface}', 0))"
                ),
            ]
            subprocess.run(cmd, check=True, capture_output=True)
        finally:
            os.unlink(packet_file)


class Statistics:
    """Track injection statistics."""

    def __init__(self):
        self.packets_sent = 0
        self.bytes_sent = 0
        self.start_time = time.time()
        self.errors = 0

    def record(self, packet_size: int):
        """Record successful packet injection."""
        self.packets_sent += 1
        self.bytes_sent += packet_size

    def error(self):
        """Record injection error."""
        self.errors += 1

    def elapsed(self) -> float:
        """Elapsed time in seconds."""
        return time.time() - self.start_time

    def rate(self) -> float:
        """Packets per second."""
        elapsed = self.elapsed()
        return self.packets_sent / elapsed if elapsed > 0 else 0

    def report(self) -> str:
        """Generate statistics report."""
        elapsed = self.elapsed()
        pps = self.rate()
        mbps = (self.bytes_sent * 8) / (elapsed * 1_000_000) if elapsed > 0 else 0
        return (
            f"\n=== Injection Complete ===\n"
            f"Packets sent:  {self.packets_sent}\n"
            f"Bytes sent:    {self.bytes_sent}\n"
            f"Errors:        {self.errors}\n"
            f"Elapsed:       {elapsed:.3f}s\n"
            f"Rate:          {pps:.1f} pps\n"
            f"Throughput:    {mbps:.2f} Mbps\n"
        )


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Doom-over-IPv6 Packet Injection Tool",
        epilog="Example: ./doom-tick.py --count 10 --rate 100 --dry-run",
    )

    # Packet parameters
    parser.add_argument(
        "--flow-label",
        type=lambda x: int(x, 0),
        default=0xDE,
        help="IPv6 flow label (hex, default: 0xDE)",
    )
    parser.add_argument(
        "--src-service",
        type=lambda x: int(x, 0),
        default=0xFF,
        help="Source service ID (hex, default: 0xFF)",
    )
    parser.add_argument(
        "--dst-service",
        type=lambda x: int(x, 0),
        default=0xFF,
        help="Destination service ID (hex, default: 0xFF)",
    )
    parser.add_argument(
        "--hop-count",
        type=int,
        default=1,
        help="Initial hop count (default: 1)",
    )
    parser.add_argument(
        "--trace-id",
        type=lambda x: int(x, 0),
        default=0,
        help="Trace ID (hex, default: random)",
    )
    parser.add_argument(
        "--qos",
        type=lambda x: int(x, 0),
        default=0,
        help="QoS class (hex, default: 0)",
    )
    parser.add_argument(
        "--latency-budget",
        type=int,
        default=0,
        help="Latency budget in microseconds (default: 0)",
    )

    # Injection parameters
    parser.add_argument(
        "--count",
        type=int,
        default=1,
        help="Number of packets to send (default: 1)",
    )
    parser.add_argument(
        "--rate",
        type=float,
        default=0,
        help="Target rate in packets/second (0 = unlimited, default: 0)",
    )
    parser.add_argument(
        "--burst",
        action="store_true",
        help="Send all packets as fast as possible (ignores --rate)",
    )

    # Network parameters
    parser.add_argument(
        "--interface",
        default="veth01",
        help="Network interface (default: veth01)",
    )
    parser.add_argument(
        "--namespace",
        default="monad0",
        help="Network namespace (default: monad0)",
    )
    parser.add_argument(
        "--src-mac",
        default="02:42:ac:11:00:02",
        help="Source MAC address (default: 02:42:ac:11:00:02)",
    )
    parser.add_argument(
        "--dst-mac",
        default="02:42:ac:11:00:03",
        help="Destination MAC address (default: 02:42:ac:11:00:03)",
    )

    # Modes
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print hex packet without sending",
    )
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="Verbose output",
    )
    parser.add_argument(
        "-q", "--quiet",
        action="store_true",
        help="Suppress statistics output",
    )

    args = parser.parse_args()

    # Validate arguments
    if args.flow_label > 0xFFFFF:
        parser.error("--flow-label must be <= 0xFFFFF (20 bits)")
    if args.count < 1:
        parser.error("--count must be >= 1")
    if args.rate < 0:
        parser.error("--rate must be >= 0")

    # Parse MAC addresses
    try:
        src_mac = bytes.fromhex(args.src_mac.replace(":", ""))
        dst_mac = bytes.fromhex(args.dst_mac.replace(":", ""))
        if len(src_mac) != 6 or len(dst_mac) != 6:
            raise ValueError("MAC addresses must be 6 bytes")
    except ValueError as e:
        parser.error(f"Invalid MAC address: {e}")

    # Use random trace_id if not specified
    if args.trace_id == 0:
        args.trace_id = random.randint(1, 0xFFFFFFFF)

    # Create packet generator
    packet_gen = DoomPacket(
        src_service_id=args.src_service,
        dst_service_id=args.dst_service,
        hop_count=args.hop_count,
        trace_id=args.trace_id,
        qos_class=args.qos,
        latency_budget_us=args.latency_budget,
        flow_label=args.flow_label,
    )

    # Build first packet for dry-run or statistics
    packet = packet_gen.build(dst_mac, src_mac)

    # Dry-run mode
    if args.dry_run:
        print("=== Doom Packet (Dry-Run) ===")
        print(f"Flow Label:  0x{args.flow_label:05x}")
        print(f"Trace ID:    0x{args.trace_id:08x}")
        print(f"Src Service: 0x{args.src_service:02x}")
        print(f"Dst Service: 0x{args.dst_service:02x}")
        print(f"Packet Size: {len(packet)} bytes\n")
        print(DoomPacket.hex_dump(packet))
        print("\n=== Hex String ===")
        print(packet.hex())
        return

    # Normal injection mode
    stats = Statistics()
    injector = None

    def signal_handler(sig, frame):
        """Handle SIGINT gracefully."""
        nonlocal injector
        print("\n\nInterrupted by user")
        if injector:
            injector.close()
        if not args.quiet:
            print(stats.report())
        sys.exit(0)

    signal.signal(signal.SIGINT, signal_handler)

    try:
        # Open socket
        if not args.dry_run:
            try:
                injector = PacketInjector(args.interface, args.namespace)
                injector._open_socket()
            except RuntimeError as e:
                print(f"Error: {e}", file=sys.stderr)
                sys.exit(1)

        # Injection loop
        interval = (1.0 / args.rate) if args.rate > 0 else 0
        last_send = time.time()

        for i in range(args.count):
            # Rate limiting
            if not args.burst and args.rate > 0:
                elapsed = time.time() - last_send
                if elapsed < interval:
                    time.sleep(interval - elapsed)
                last_send = time.time()

            # Regenerate packet with incremented trace_id for variety
            if i > 0:
                packet_gen.trace_id = (args.trace_id + i) & 0xFFFFFFFF
                packet = packet_gen.build(dst_mac, src_mac)

            # Inject
            try:
                injector.inject(packet)
                stats.record(len(packet))

                if args.verbose:
                    print(
                        f"[{i+1}/{args.count}] Sent {len(packet)} bytes (trace_id: 0x{packet_gen.trace_id:08x})"
                    )
            except RuntimeError as e:
                print(f"Error: {e}", file=sys.stderr)
                stats.error()

        # Report statistics
        if not args.quiet:
            print(stats.report())

    finally:
        if injector:
            injector.close()


if __name__ == "__main__":
    main()
