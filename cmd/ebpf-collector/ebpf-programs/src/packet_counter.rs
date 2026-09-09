// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! XDP packet counter — captures packet metadata and emits to ring buffer.
//!
//! Attaches to a network interface via XDP. Parses Ethernet → IPv4/IPv6
//! → TCP/UDP headers, extracts the 5-tuple + packet length, stamps mesh
//! metadata into the IPv4-mapped address prefix, and emits a PacketEvent
//! to a ring buffer for userspace consumption.
//!
//! Always returns XDP_PASS — purely observational, never drops packets.

#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    macros::{map, xdp},
    maps::RingBuf,
    programs::XdpContext,
};
use ebpf_common::{
    PacketEvent, AF_INET, AF_INET6, FLOW_FLAG_EST, FLOW_FLAG_SYN_SEEN, RING_BUF_SIZE,
};

#[map]
static EVENTS: RingBuf = RingBuf::with_byte_size(RING_BUF_SIZE, 0);

// Ethernet header (14 bytes)
const ETH_HDR_LEN: usize = 14;
const ETH_P_IP: u16 = 0x0800;
const ETH_P_IPV6: u16 = 0x86DD;

// IP protocol numbers
const IPPROTO_TCP: u8 = 6;
const IPPROTO_UDP: u8 = 17;

// TCP flags
const TCP_SYN: u8 = 0x02;
const TCP_ACK: u8 = 0x10;

#[xdp]
pub fn packet_counter(ctx: XdpContext) -> u32 {
    match try_packet_counter(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_packet_counter(ctx: &XdpContext) -> Result<u32, ()> {
    let data = ctx.data();
    let data_end = ctx.data_end();

    // Bounds check: need at least Ethernet header
    if data + ETH_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // Parse Ethernet header — read ethertype at offset 12
    let eth_proto = u16::from_be(unsafe { *((data + 12) as *const u16) });

    let mut event = PacketEvent {
        timestamp_ns: unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() },
        src_addr: [0u8; 16],
        dst_addr: [0u8; 16],
        src_port: 0,
        dst_port: 0,
        protocol: 0,
        af: 0,
        _pad: [0u8; 2],
        packet_len: (data_end - data) as u32,
        direction: 0, // ingress (XDP is always ingress)
        _pad2: [0u8; 3],
    };

    let transport_offset;

    match eth_proto {
        ETH_P_IP => {
            // IPv4: minimum header is 20 bytes
            let ip_offset = data + ETH_HDR_LEN;
            if ip_offset + 20 > data_end {
                return Ok(xdp_action::XDP_PASS);
            }

            let ihl = (unsafe { *(ip_offset as *const u8) } & 0x0F) as usize * 4;
            if ip_offset + ihl > data_end {
                return Ok(xdp_action::XDP_PASS);
            }

            // Protocol at offset 9
            event.protocol = unsafe { *((ip_offset + 9) as *const u8) };
            event.af = AF_INET;

            // Source IP at offset 12 (4 bytes)
            let src_ip = unsafe { *((ip_offset + 12) as *const [u8; 4]) };
            // Destination IP at offset 16 (4 bytes)
            let dst_ip = unsafe { *((ip_offset + 16) as *const [u8; 4]) };

            // IPv4-mapped format: [mesh_meta(10)][0xFF,0xFF][IPv4(4)]
            // Stamp mesh metadata version
            event.src_addr[0] = 1; // mesh_version = 1
            event.src_addr[2] = 1; // hop_count = 1 (this XDP program)
            event.src_addr[10] = 0xFF;
            event.src_addr[11] = 0xFF;
            event.src_addr[12] = src_ip[0];
            event.src_addr[13] = src_ip[1];
            event.src_addr[14] = src_ip[2];
            event.src_addr[15] = src_ip[3];

            event.dst_addr[10] = 0xFF;
            event.dst_addr[11] = 0xFF;
            event.dst_addr[12] = dst_ip[0];
            event.dst_addr[13] = dst_ip[1];
            event.dst_addr[14] = dst_ip[2];
            event.dst_addr[15] = dst_ip[3];

            transport_offset = ip_offset + ihl;
        }
        ETH_P_IPV6 => {
            // IPv6: fixed header is 40 bytes
            let ip6_offset = data + ETH_HDR_LEN;
            if ip6_offset + 40 > data_end {
                return Ok(xdp_action::XDP_PASS);
            }

            // Next header (protocol) at offset 6
            event.protocol = unsafe { *((ip6_offset + 6) as *const u8) };
            event.af = AF_INET6;

            // Source addr at offset 8 (16 bytes)
            if ip6_offset + 24 > data_end {
                return Ok(xdp_action::XDP_PASS);
            }
            let src_addr = unsafe { *((ip6_offset + 8) as *const [u8; 16]) };
            event.src_addr = src_addr;

            // Dest addr at offset 24 (16 bytes)
            if ip6_offset + 40 > data_end {
                return Ok(xdp_action::XDP_PASS);
            }
            let dst_addr = unsafe { *((ip6_offset + 24) as *const [u8; 16]) };
            event.dst_addr = dst_addr;

            transport_offset = ip6_offset + 40;
        }
        _ => {
            // Not IP — skip
            return Ok(xdp_action::XDP_PASS);
        }
    }

    // Parse TCP/UDP ports
    match event.protocol {
        IPPROTO_TCP => {
            // TCP header: min 20 bytes, ports at offset 0 and 2
            if transport_offset + 14 > data_end {
                return Ok(xdp_action::XDP_PASS);
            }
            event.src_port = u16::from_be(unsafe { *(transport_offset as *const u16) });
            event.dst_port = u16::from_be(unsafe { *((transport_offset + 2) as *const u16) });

            // TCP flags at offset 13
            let flags = unsafe { *((transport_offset + 13) as *const u8) };
            if event.af == AF_INET {
                // Stamp flow flags into mesh meta
                let mut flow_flags: u8 = 0;
                if flags & TCP_SYN != 0 {
                    flow_flags |= FLOW_FLAG_SYN_SEEN;
                }
                if flags & TCP_ACK != 0 && flags & TCP_SYN == 0 {
                    flow_flags |= FLOW_FLAG_EST;
                }
                event.src_addr[3] = flow_flags;
            }
        }
        IPPROTO_UDP => {
            // UDP header: 8 bytes, ports at offset 0 and 2
            if transport_offset + 4 > data_end {
                return Ok(xdp_action::XDP_PASS);
            }
            event.src_port = u16::from_be(unsafe { *(transport_offset as *const u16) });
            event.dst_port = u16::from_be(unsafe { *((transport_offset + 2) as *const u16) });
        }
        _ => {
            // Other protocols — emit event with zero ports
        }
    }

    // Emit event to ring buffer
    if let Some(mut buf) = EVENTS.reserve::<PacketEvent>(0) {
        buf.write(event);
        buf.submit(0);
    }

    Ok(xdp_action::XDP_PASS)
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
