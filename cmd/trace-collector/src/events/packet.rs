//! Packet event parsing.
//!
//! Handles network packet traces from eBPF programs attached to
//! XDP, TC, or socket filters.

use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

use serde::{Deserialize, Serialize};

use super::EventError;

/// Maximum command name length (matches Linux TASK_COMM_LEN)
const TASK_COMM_LEN: usize = 16;

/// Packet direction
#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum PacketDirection {
    Ingress = 0,
    Egress = 1,
}

impl From<u8> for PacketDirection {
    fn from(v: u8) -> Self {
        match v {
            1 => PacketDirection::Egress,
            _ => PacketDirection::Ingress,
        }
    }
}

/// IP protocol
#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum IpProtocol {
    Tcp = 6,
    Udp = 17,
    Icmp = 1,
    Icmpv6 = 58,
    Other(u8),
}

impl From<u8> for IpProtocol {
    fn from(v: u8) -> Self {
        match v {
            6 => IpProtocol::Tcp,
            17 => IpProtocol::Udp,
            1 => IpProtocol::Icmp,
            58 => IpProtocol::Icmpv6,
            _ => IpProtocol::Other(v),
        }
    }
}

/// TCP flags
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct TcpFlags {
    pub fin: bool,
    pub syn: bool,
    pub rst: bool,
    pub psh: bool,
    pub ack: bool,
    pub urg: bool,
    pub ece: bool,
    pub cwr: bool,
}

impl From<u8> for TcpFlags {
    fn from(v: u8) -> Self {
        Self {
            fin: (v & 0x01) != 0,
            syn: (v & 0x02) != 0,
            rst: (v & 0x04) != 0,
            psh: (v & 0x08) != 0,
            ack: (v & 0x10) != 0,
            urg: (v & 0x20) != 0,
            ece: (v & 0x40) != 0,
            cwr: (v & 0x80) != 0,
        }
    }
}

/// Raw packet event structure from BPF (matches C struct)
#[repr(C, packed)]
struct RawPacketEvent {
    /// Process ID
    pid: u32,
    /// Thread ID
    tid: u32,
    /// Interface index
    ifindex: u32,
    /// Packet length
    pkt_len: u32,
    /// Source IP (IPv4 or lower 32 bits of IPv6)
    src_ip: [u8; 16],
    /// Destination IP
    dst_ip: [u8; 16],
    /// Source port
    src_port: u16,
    /// Destination port
    dst_port: u16,
    /// IP version (4 or 6)
    ip_version: u8,
    /// Protocol (TCP=6, UDP=17, etc.)
    protocol: u8,
    /// Direction (0=ingress, 1=egress)
    direction: u8,
    /// TCP flags (if TCP)
    tcp_flags: u8,
    /// Command name
    comm: [u8; TASK_COMM_LEN],
}

/// Parsed packet event
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PacketEvent {
    /// Process ID
    pub pid: u32,
    /// Thread ID
    pub tid: u32,
    /// Command name
    pub comm: String,
    /// Interface index
    pub ifindex: u32,
    /// Interface name (resolved if possible)
    pub ifname: Option<String>,
    /// Packet length in bytes
    pub pkt_len: u32,
    /// Source IP address
    pub src_ip: IpAddr,
    /// Destination IP address
    pub dst_ip: IpAddr,
    /// Source port
    pub src_port: u16,
    /// Destination port
    pub dst_port: u16,
    /// IP protocol
    pub protocol: IpProtocol,
    /// Packet direction
    pub direction: PacketDirection,
    /// TCP flags (if applicable)
    pub tcp_flags: Option<TcpFlags>,
}

impl PacketEvent {
    /// Parse packet event from raw bytes
    pub fn from_bytes(data: &[u8]) -> Result<Self, EventError> {
        let size = std::mem::size_of::<RawPacketEvent>();
        if data.len() < size {
            return Err(EventError::InsufficientData {
                expected: size,
                actual: data.len(),
            });
        }

        // SAFETY: We've verified the length
        let raw: RawPacketEvent =
            unsafe { std::ptr::read_unaligned(data.as_ptr() as *const RawPacketEvent) };

        // Parse command name
        let comm = parse_comm(&raw.comm);

        // Parse IP addresses
        let (src_ip, dst_ip) = if raw.ip_version == 6 {
            (
                IpAddr::V6(Ipv6Addr::from(raw.src_ip)),
                IpAddr::V6(Ipv6Addr::from(raw.dst_ip)),
            )
        } else {
            (
                IpAddr::V4(Ipv4Addr::new(
                    raw.src_ip[0],
                    raw.src_ip[1],
                    raw.src_ip[2],
                    raw.src_ip[3],
                )),
                IpAddr::V4(Ipv4Addr::new(
                    raw.dst_ip[0],
                    raw.dst_ip[1],
                    raw.dst_ip[2],
                    raw.dst_ip[3],
                )),
            )
        };

        let protocol = IpProtocol::from(raw.protocol);
        let tcp_flags = if matches!(protocol, IpProtocol::Tcp) {
            Some(TcpFlags::from(raw.tcp_flags))
        } else {
            None
        };

        Ok(Self {
            pid: raw.pid,
            tid: raw.tid,
            comm,
            ifindex: raw.ifindex,
            ifname: resolve_ifname(raw.ifindex),
            pkt_len: raw.pkt_len,
            src_ip,
            dst_ip,
            src_port: u16::from_be(raw.src_port),
            dst_port: u16::from_be(raw.dst_port),
            protocol,
            direction: PacketDirection::from(raw.direction),
            tcp_flags,
        })
    }

    /// Check if this is a TCP SYN packet
    pub fn is_syn(&self) -> bool {
        self.tcp_flags.map(|f| f.syn && !f.ack).unwrap_or(false)
    }

    /// Check if this is a TCP FIN packet
    pub fn is_fin(&self) -> bool {
        self.tcp_flags.map(|f| f.fin).unwrap_or(false)
    }

    /// Check if this is a TCP RST packet
    pub fn is_rst(&self) -> bool {
        self.tcp_flags.map(|f| f.rst).unwrap_or(false)
    }

    /// Get a connection tuple string
    pub fn connection_tuple(&self) -> String {
        format!(
            "{}:{} -> {}:{}",
            self.src_ip, self.src_port, self.dst_ip, self.dst_port
        )
    }
}

/// Parse null-terminated command name
fn parse_comm(comm: &[u8; TASK_COMM_LEN]) -> String {
    let end = comm.iter().position(|&c| c == 0).unwrap_or(TASK_COMM_LEN);
    String::from_utf8_lossy(&comm[..end]).to_string()
}

/// Resolve interface index to name
fn resolve_ifname(ifindex: u32) -> Option<String> {
    if ifindex == 0 {
        return None;
    }

    // Try to read from /sys/class/net/*/ifindex
    // This is a simplified version - in production you'd cache this
    let net_path = std::path::Path::new("/sys/class/net");
    if let Ok(entries) = std::fs::read_dir(net_path) {
        for entry in entries.flatten() {
            let ifindex_path = entry.path().join("ifindex");
            if let Ok(content) = std::fs::read_to_string(&ifindex_path) {
                if let Ok(idx) = content.trim().parse::<u32>() {
                    if idx == ifindex {
                        return entry.file_name().to_str().map(|s| s.to_string());
                    }
                }
            }
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tcp_flags() {
        let flags = TcpFlags::from(0x12); // SYN + ACK
        assert!(flags.syn);
        assert!(flags.ack);
        assert!(!flags.fin);
        assert!(!flags.rst);
    }

    #[test]
    fn test_parse_comm() {
        let mut comm = [0u8; TASK_COMM_LEN];
        comm[0..4].copy_from_slice(b"curl");
        assert_eq!(parse_comm(&comm), "curl");
    }

    #[test]
    fn test_packet_direction() {
        assert_eq!(PacketDirection::from(0), PacketDirection::Ingress);
        assert_eq!(PacketDirection::from(1), PacketDirection::Egress);
    }
}
