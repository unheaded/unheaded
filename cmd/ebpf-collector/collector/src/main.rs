//! Userspace eBPF event collector.
//!
//! Loads compiled eBPF programs (XDP packet counter, kprobe TCP latency),
//! reads events from ring buffers, converts to JSON matching the dashboard
//! backend's types.go schema, and publishes to Wotan via gRPC (primary)
//! or HTTP (fallback).

use std::net::Ipv4Addr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use aya::maps::RingBuf;
use aya::programs::{KProbe, Xdp, XdpFlags};
use aya::Ebpf;
use clap::Parser;
use log::{error, info, warn};
use tokio::signal;
use tokio::sync::Notify;

use ebpf_common::{
    LatencyEvent, PacketEvent, AF_INET, AF_INET6, FLOW_FLAG_EST, FLOW_FLAG_FIN_SEEN,
    FLOW_FLAG_OOO, FLOW_FLAG_RETRANSMIT, FLOW_FLAG_RST, FLOW_FLAG_SYN_SEEN,
    NAT_DNAT, NAT_MASQUERADE, NAT_SNAT, OP_TCP_CONNECT, OP_TCP_RECV, OP_TCP_SEND,
};

pub mod wotan_proto {
    tonic::include_proto!("chat");
}

use wotan_proto::topic_stream_client::TopicStreamClient;
use wotan_proto::TopicPublishRequest;

// ---------------------------------------------------------------------------
// CLI
// ---------------------------------------------------------------------------

#[derive(Parser, Debug)]
#[command(name = "ebpf-collector", about = "eBPF event collector for Unheaded")]
struct Args {
    /// Network interface to attach XDP program to (auto-detects default)
    #[arg(long, default_value = "")]
    iface: String,

    /// Wotan HTTP address for fallback publishing
    #[arg(long, default_value = "http://localhost:9080")]
    wotan_http: String,

    /// Wotan gRPC address for primary publishing
    #[arg(long, default_value = "http://localhost:9090")]
    wotan_grpc: String,

    /// Path to compiled XDP program ELF
    #[arg(long, default_value = "../ebpf-programs/target/bpfel-unknown-none/release/packet-counter")]
    xdp_program: String,

    /// Path to compiled kprobe program ELF
    #[arg(long, default_value = "../ebpf-programs/target/bpfel-unknown-none/release/tcp-latency")]
    kprobe_program: String,

    /// Rate limit: max events per second per topic
    #[arg(long, default_value = "100")]
    rate_limit: u64,
}

// ---------------------------------------------------------------------------
// Rate Limiter (token bucket)
// ---------------------------------------------------------------------------

struct RateLimiter {
    capacity: u64,
    tokens: std::sync::Mutex<(u64, Instant)>,
    published: AtomicU64,
    dropped: AtomicU64,
}

impl RateLimiter {
    fn new(events_per_sec: u64) -> Self {
        Self {
            capacity: events_per_sec,
            tokens: std::sync::Mutex::new((events_per_sec, Instant::now())),
            published: AtomicU64::new(0),
            dropped: AtomicU64::new(0),
        }
    }

    fn try_acquire(&self) -> bool {
        let mut guard = self.tokens.lock().unwrap();
        let (ref mut tokens, ref mut last_refill) = *guard;
        let now = Instant::now();
        let elapsed = now.duration_since(*last_refill);

        // Refill tokens based on elapsed time
        let refill = (elapsed.as_secs_f64() * self.capacity as f64) as u64;
        if refill > 0 {
            *tokens = (*tokens + refill).min(self.capacity);
            *last_refill = now;
        }

        if *tokens > 0 {
            *tokens -= 1;
            self.published.fetch_add(1, Ordering::Relaxed);
            true
        } else {
            self.dropped.fetch_add(1, Ordering::Relaxed);
            false
        }
    }

    fn stats(&self) -> (u64, u64) {
        (
            self.published.load(Ordering::Relaxed),
            self.dropped.load(Ordering::Relaxed),
        )
    }
}

// ---------------------------------------------------------------------------
// Event Buffer (bounded ring for retry buffering)
// ---------------------------------------------------------------------------

struct EventBuffer {
    buf: std::sync::Mutex<std::collections::VecDeque<Vec<u8>>>,
    capacity: usize,
}

impl EventBuffer {
    fn new(capacity: usize) -> Self {
        Self {
            buf: std::sync::Mutex::new(std::collections::VecDeque::with_capacity(capacity)),
            capacity,
        }
    }

    fn push(&self, data: Vec<u8>) {
        let mut buf = self.buf.lock().unwrap();
        if buf.len() >= self.capacity {
            buf.pop_front(); // evict oldest
        }
        buf.push_back(data);
    }

    fn drain(&self) -> Vec<Vec<u8>> {
        let mut buf = self.buf.lock().unwrap();
        buf.drain(..).collect()
    }

    #[cfg(test)]
    fn len(&self) -> usize {
        self.buf.lock().unwrap().len()
    }
}

// ---------------------------------------------------------------------------
// Wotan Publisher (gRPC primary, HTTP fallback)
// ---------------------------------------------------------------------------

struct WotanPublisher {
    grpc_addr: String,
    http_addr: String,
    grpc_client: tokio::sync::Mutex<Option<TopicStreamClient<tonic::transport::Channel>>>,
    buffer: EventBuffer,
}

impl WotanPublisher {
    fn new(grpc_addr: String, http_addr: String) -> Self {
        Self {
            grpc_addr,
            http_addr,
            grpc_client: tokio::sync::Mutex::new(None),
            buffer: EventBuffer::new(1000),
        }
    }

    async fn connect_grpc(&self) -> bool {
        let mut client = self.grpc_client.lock().await;
        if client.is_some() {
            return true;
        }
        match TopicStreamClient::connect(self.grpc_addr.clone()).await {
            Ok(c) => {
                info!("connected to Wotan gRPC at {}", self.grpc_addr);
                *client = Some(c);
                true
            }
            Err(e) => {
                warn!("gRPC connect failed: {}, will use HTTP fallback", e);
                false
            }
        }
    }

    async fn publish(&self, topic: &str, payload: Vec<u8>) {
        // Try gRPC first
        {
            let mut client = self.grpc_client.lock().await;
            if let Some(ref mut c) = *client {
                let req = tonic::Request::new(TopicPublishRequest {
                    topic: topic.to_string(),
                    sender_id: "ebpf-collector".to_string(),
                    payload: payload.clone(),
                    metadata: Default::default(),
                });
                match c.publish_topic(req).await {
                    Ok(_) => return,
                    Err(e) => {
                        warn!("gRPC publish failed: {}, falling back to HTTP", e);
                        *client = None; // reset, will reconnect next time
                    }
                }
            }
        }

        // HTTP fallback with retry
        self.publish_http_with_retry(topic, &payload).await;
    }

    async fn publish_http_with_retry(&self, topic: &str, payload: &[u8]) {
        let url = format!(
            "{}/api/v1/topics/{}/publish",
            self.http_addr.trim_end_matches('/'),
            topic
        );
        let mut backoff = Duration::from_millis(100);
        let max_retries = 5;

        for attempt in 0..max_retries {
            match ureq::post(&url)
                .set("Content-Type", "application/json")
                .send_bytes(payload)
            {
                Ok(_) => return,
                Err(e) => {
                    if attempt < max_retries - 1 {
                        warn!(
                            "HTTP publish attempt {}/{} failed: {}, retrying in {:?}",
                            attempt + 1,
                            max_retries,
                            e,
                            backoff
                        );
                        tokio::time::sleep(backoff).await;
                        backoff = (backoff * 2).min(Duration::from_secs(10));
                    } else {
                        error!(
                            "HTTP publish failed after {} retries, buffering event",
                            max_retries
                        );
                        self.buffer.push(payload.to_vec());
                    }
                }
            }
        }
    }

    async fn drain_buffer(&self, topic: &str) {
        let events = self.buffer.drain();
        if !events.is_empty() {
            info!("draining {} buffered events", events.len());
            for payload in events {
                self.publish(topic, payload).await;
            }
        }
    }
}

// ---------------------------------------------------------------------------
// JSON Formatting (matches dashboard-backend types.go)
// ---------------------------------------------------------------------------

/// Format a PacketEvent to JSON matching types.go schema.
fn format_packet_event(event: &PacketEvent) -> serde_json::Value {
    let src_addr = format_addr(&event.src_addr, event.af);
    let dst_addr = format_addr(&event.dst_addr, event.af);

    let direction = match event.direction {
        0 => "ingress",
        _ => "egress",
    };

    let protocol = event.protocol;

    let mut json = serde_json::json!({
        "timestamp_ns": event.timestamp_ns,
        "trace_id": { "high": 0u64, "low": 0u64 },
        "flow_key": {
            "src_addr": src_addr,
            "dst_addr": dst_addr,
            "src_port": event.src_port,
            "dst_port": event.dst_port,
            "protocol": protocol,
        },
        "packet_len": event.packet_len,
        "action": "pass",
        "direction": direction,
    });

    // Extract mesh metadata for IPv4 packets
    if event.af == AF_INET {
        if let Some(mesh) = extract_mesh_meta(&event.src_addr, &event.dst_addr) {
            json.as_object_mut()
                .unwrap()
                .insert("mesh".to_string(), mesh);
        }
    }

    json
}

/// Format a LatencyEvent to JSON matching types.go schema.
fn format_latency_event(event: &LatencyEvent) -> serde_json::Value {
    let operation = match event.operation {
        OP_TCP_SEND => "tcp_send",
        OP_TCP_RECV => "tcp_recv",
        OP_TCP_CONNECT => "tcp_connect",
        _ => "unknown",
    };

    serde_json::json!({
        "timestamp_ns": event.timestamp_ns,
        "trace_id": { "high": 0u64, "low": 0u64 },
        "pid": event.pid,
        "tid": event.tid,
        "latency_ns": event.latency_ns,
        "operation": operation,
    })
}

/// Format address bytes to string based on address family.
fn format_addr(addr: &[u8; 16], af: u8) -> String {
    match af {
        AF_INET => {
            // IPv4 is in bytes [12..16]
            Ipv4Addr::new(addr[12], addr[13], addr[14], addr[15]).to_string()
        }
        AF_INET6 => format_ipv6(addr),
        _ => "unknown".to_string(),
    }
}

/// Format 16-byte IPv6 address to string with zero compression.
fn format_ipv6(addr: &[u8; 16]) -> String {
    // Check for IPv4-mapped IPv6 (::ffff:x.x.x.x)
    if addr[0..10] == [0; 10] && addr[10] == 0xFF && addr[11] == 0xFF {
        return Ipv4Addr::new(addr[12], addr[13], addr[14], addr[15]).to_string();
    }

    let groups: [u16; 8] = [
        u16::from_be_bytes([addr[0], addr[1]]),
        u16::from_be_bytes([addr[2], addr[3]]),
        u16::from_be_bytes([addr[4], addr[5]]),
        u16::from_be_bytes([addr[6], addr[7]]),
        u16::from_be_bytes([addr[8], addr[9]]),
        u16::from_be_bytes([addr[10], addr[11]]),
        u16::from_be_bytes([addr[12], addr[13]]),
        u16::from_be_bytes([addr[14], addr[15]]),
    ];

    // Find longest run of zero groups for :: compression
    let mut best_start = None;
    let mut best_len = 0usize;
    let mut cur_start = None;
    let mut cur_len = 0usize;

    for (i, &g) in groups.iter().enumerate() {
        if g == 0 {
            if cur_start.is_none() {
                cur_start = Some(i);
                cur_len = 1;
            } else {
                cur_len += 1;
            }
        } else {
            if cur_len > best_len && cur_len >= 2 {
                best_start = cur_start;
                best_len = cur_len;
            }
            cur_start = None;
            cur_len = 0;
        }
    }
    if cur_len > best_len && cur_len >= 2 {
        best_start = cur_start;
        best_len = cur_len;
    }

    let mut parts = Vec::new();
    let mut i = 0;
    let mut compressed = false;
    while i < 8 {
        if Some(i) == best_start && !compressed {
            parts.push(String::new()); // marker for ::
            if i == 0 {
                parts.push(String::new()); // leading :: needs two empty parts
            }
            i += best_len;
            compressed = true;
            if i >= 8 {
                parts.push(String::new()); // trailing ::
            }
            continue;
        }
        parts.push(format!("{:x}", groups[i]));
        i += 1;
    }

    parts.join(":")
}

/// Extract mesh metadata from IPv4-mapped address prefix bytes.
fn extract_mesh_meta(src_addr: &[u8; 16], dst_addr: &[u8; 16]) -> Option<serde_json::Value> {
    let version = src_addr[0];
    if version == 0 {
        return None; // No mesh metadata stamped
    }

    let flow_flags = src_addr[3];
    let mut flags = Vec::new();
    if flow_flags & FLOW_FLAG_SYN_SEEN != 0 {
        flags.push("syn_seen");
    }
    if flow_flags & FLOW_FLAG_EST != 0 {
        flags.push("est");
    }
    if flow_flags & FLOW_FLAG_FIN_SEEN != 0 {
        flags.push("fin_seen");
    }
    if flow_flags & FLOW_FLAG_RST != 0 {
        flags.push("rst");
    }
    if flow_flags & FLOW_FLAG_RETRANSMIT != 0 {
        flags.push("retransmit");
    }
    if flow_flags & FLOW_FLAG_OOO != 0 {
        flags.push("ooo");
    }

    let trace_hash = u32::from_be_bytes([src_addr[4], src_addr[5], src_addr[6], src_addr[7]]);
    let qos_class = u16::from_be_bytes([src_addr[8], src_addr[9]]);

    let nat_type = match dst_addr[1] {
        NAT_SNAT => "snat",
        NAT_DNAT => "dnat",
        NAT_MASQUERADE => "masquerade",
        _ => "none",
    };

    let latency_hint_ns =
        u32::from_be_bytes([dst_addr[4], dst_addr[5], dst_addr[6], dst_addr[7]]);

    Some(serde_json::json!({
        "version": version,
        "src_service_id": src_addr[1],
        "dst_service_id": dst_addr[0],
        "hop_count": src_addr[2],
        "flow_flags": flags,
        "trace_hash": format!("{:08x}", trace_hash),
        "qos_class": qos_class,
        "nat_type": nat_type,
        "latency_hint_ns": latency_hint_ns,
    }))
}

/// Auto-detect the default network interface.
fn detect_default_iface() -> String {
    // Read /proc/net/route, find interface with destination 00000000 (default route)
    if let Ok(contents) = std::fs::read_to_string("/proc/net/route") {
        for line in contents.lines().skip(1) {
            let fields: Vec<&str> = line.split_whitespace().collect();
            if fields.len() >= 2 && fields[1] == "00000000" {
                return fields[0].to_string();
            }
        }
    }
    "eth0".to_string()
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    env_logger::init();

    let args = Args::parse();

    let iface = if args.iface.is_empty() {
        detect_default_iface()
    } else {
        args.iface.clone()
    };
    info!("using interface: {}", iface);

    // Load and attach XDP program
    info!("loading XDP program from {}", args.xdp_program);
    let mut xdp_ebpf = Ebpf::load_file(&args.xdp_program)?;

    let xdp_prog: &mut Xdp = xdp_ebpf.program_mut("packet_counter").unwrap().try_into()?;
    xdp_prog.load()?;
    xdp_prog.attach(&iface, XdpFlags::default())?;
    info!("XDP program attached to {}", iface);

    // Load and attach kprobe programs
    info!("loading kprobe program from {}", args.kprobe_program);
    let mut kprobe_ebpf = Ebpf::load_file(&args.kprobe_program)?;

    let sendmsg_enter: &mut KProbe = kprobe_ebpf
        .program_mut("tcp_sendmsg_enter")
        .unwrap()
        .try_into()?;
    sendmsg_enter.load()?;
    sendmsg_enter.attach("tcp_sendmsg", 0)?;

    let sendmsg_exit: &mut KProbe = kprobe_ebpf
        .program_mut("tcp_sendmsg_exit")
        .unwrap()
        .try_into()?;
    sendmsg_exit.load()?;
    sendmsg_exit.attach("tcp_sendmsg", 0)?;

    let recvmsg_enter: &mut KProbe = kprobe_ebpf
        .program_mut("tcp_recvmsg_enter")
        .unwrap()
        .try_into()?;
    recvmsg_enter.load()?;
    recvmsg_enter.attach("tcp_recvmsg", 0)?;

    let recvmsg_exit: &mut KProbe = kprobe_ebpf
        .program_mut("tcp_recvmsg_exit")
        .unwrap()
        .try_into()?;
    recvmsg_exit.load()?;
    recvmsg_exit.attach("tcp_recvmsg", 0)?;
    info!("kprobe programs attached to tcp_sendmsg and tcp_recvmsg");

    // Setup publisher
    let publisher = Arc::new(WotanPublisher::new(
        args.wotan_grpc.clone(),
        args.wotan_http.clone(),
    ));
    publisher.connect_grpc().await;

    // Setup rate limiters
    let packet_limiter = Arc::new(RateLimiter::new(args.rate_limit));
    let latency_limiter = Arc::new(RateLimiter::new(args.rate_limit));

    // Shutdown signal
    let shutdown = Arc::new(Notify::new());

    // Take owned maps from the Ebpf objects so they can be moved into spawned tasks
    let packet_map = xdp_ebpf.take_map("EVENTS").unwrap();
    let packet_ring = RingBuf::try_from(packet_map)?;
    let pub_clone = publisher.clone();
    let lim_clone = packet_limiter.clone();
    let shutdown_clone = shutdown.clone();
    let packet_task = tokio::spawn(async move {
        read_packet_events(packet_ring, pub_clone, lim_clone, shutdown_clone).await;
    });

    let latency_map = kprobe_ebpf.take_map("LATENCY_EVENTS").unwrap();
    let latency_ring = RingBuf::try_from(latency_map)?;
    let pub_clone = publisher.clone();
    let lim_clone = latency_limiter.clone();
    let shutdown_clone = shutdown.clone();
    let latency_task = tokio::spawn(async move {
        read_latency_events(latency_ring, pub_clone, lim_clone, shutdown_clone).await;
    });

    // Spawn stats reporter
    let pkt_lim = packet_limiter.clone();
    let lat_lim = latency_limiter.clone();
    let shutdown_clone = shutdown.clone();
    let stats_task = tokio::spawn(async move {
        let mut interval = tokio::time::interval(Duration::from_secs(10));
        loop {
            tokio::select! {
                _ = interval.tick() => {
                    let (pkt_pub, pkt_drop) = pkt_lim.stats();
                    let (lat_pub, lat_drop) = lat_lim.stats();
                    info!(
                        "stats: packets published={} dropped={}, latency published={} dropped={}",
                        pkt_pub, pkt_drop, lat_pub, lat_drop
                    );
                }
                _ = shutdown_clone.notified() => break,
            }
        }
    });

    // Spawn gRPC reconnector
    let pub_clone = publisher.clone();
    let shutdown_clone = shutdown.clone();
    let reconnect_task = tokio::spawn(async move {
        let mut interval = tokio::time::interval(Duration::from_secs(30));
        loop {
            tokio::select! {
                _ = interval.tick() => {
                    if pub_clone.connect_grpc().await {
                        pub_clone.drain_buffer("ebpf.packet.events").await;
                    }
                }
                _ = shutdown_clone.notified() => break,
            }
        }
    });

    // Wait for shutdown signal
    info!("ebpf-collector running. Press Ctrl+C to stop.");
    signal::ctrl_c().await?;
    info!("shutdown signal received");
    shutdown.notify_waiters();

    // Print final stats
    let (pkt_pub, pkt_drop) = packet_limiter.stats();
    let (lat_pub, lat_drop) = latency_limiter.stats();
    info!("final stats:");
    info!("  packets: published={}, dropped={}", pkt_pub, pkt_drop);
    info!("  latency: published={}, dropped={}", lat_pub, lat_drop);

    // Wait for tasks
    let _ = tokio::join!(packet_task, latency_task, stats_task, reconnect_task);

    info!("ebpf-collector stopped");
    Ok(())
}

async fn read_packet_events(
    mut ring: RingBuf<aya::maps::MapData>,
    publisher: Arc<WotanPublisher>,
    limiter: Arc<RateLimiter>,
    shutdown: Arc<Notify>,
) {
    loop {
        tokio::select! {
            _ = shutdown.notified() => break,
            _ = tokio::time::sleep(Duration::from_millis(1)) => {
                while let Some(item) = ring.next() {
                    if item.len() < std::mem::size_of::<PacketEvent>() {
                        continue;
                    }
                    let event: PacketEvent = unsafe {
                        std::ptr::read_unaligned(item.as_ptr() as *const PacketEvent)
                    };

                    if !limiter.try_acquire() {
                        continue;
                    }

                    let json = format_packet_event(&event);
                    let payload = serde_json::to_vec(&json).unwrap();
                    publisher.publish("ebpf.packet.events", payload).await;
                }
            }
        }
    }
}

async fn read_latency_events(
    mut ring: RingBuf<aya::maps::MapData>,
    publisher: Arc<WotanPublisher>,
    limiter: Arc<RateLimiter>,
    shutdown: Arc<Notify>,
) {
    loop {
        tokio::select! {
            _ = shutdown.notified() => break,
            _ = tokio::time::sleep(Duration::from_millis(1)) => {
                while let Some(item) = ring.next() {
                    if item.len() < std::mem::size_of::<LatencyEvent>() {
                        continue;
                    }
                    let event: LatencyEvent = unsafe {
                        std::ptr::read_unaligned(item.as_ptr() as *const LatencyEvent)
                    };

                    if !limiter.try_acquire() {
                        continue;
                    }

                    let json = format_latency_event(&event);
                    let payload = serde_json::to_vec(&json).unwrap();
                    publisher.publish("ebpf.latency.events", payload).await;
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ipv4_format() {
        let mut addr = [0u8; 16];
        addr[10] = 0xFF;
        addr[11] = 0xFF;
        addr[12] = 192;
        addr[13] = 168;
        addr[14] = 1;
        addr[15] = 1;
        assert_eq!(format_addr(&addr, AF_INET), "192.168.1.1");
    }

    #[test]
    fn test_ipv6_format() {
        let mut addr = [0u8; 16];
        addr[15] = 1;
        assert_eq!(format_addr(&addr, AF_INET6), "::1");
    }

    #[test]
    fn test_ipv6_full() {
        let mut addr = [0u8; 16];
        addr[0] = 0x20;
        addr[1] = 0x01;
        addr[2] = 0x0d;
        addr[3] = 0xb8;
        addr[15] = 1;
        assert_eq!(format_addr(&addr, AF_INET6), "2001:db8::1");
    }

    #[test]
    fn test_ipv4_mapped_v6() {
        // ::ffff:10.0.0.1 with af=2 should format as IPv4
        let mut addr = [0u8; 16];
        addr[10] = 0xFF;
        addr[11] = 0xFF;
        addr[12] = 10;
        addr[13] = 0;
        addr[14] = 0;
        addr[15] = 1;
        assert_eq!(format_addr(&addr, AF_INET), "10.0.0.1");
    }

    #[test]
    fn test_port_conversion() {
        // Network byte order bytes [0x00, 0x50] = port 80
        let bytes: [u8; 2] = [0x00, 0x50];
        let port = u16::from_be_bytes(bytes);
        assert_eq!(port, 80);
    }

    #[test]
    fn test_packet_event_json() {
        let mut event = PacketEvent {
            timestamp_ns: 1234567890,
            src_addr: [0u8; 16],
            dst_addr: [0u8; 16],
            src_port: 12345,
            dst_port: 80,
            protocol: 6,
            af: AF_INET,
            _pad: [0; 2],
            packet_len: 1500,
            direction: 0,
            _pad2: [0; 3],
        };
        // Set IPv4 addresses
        event.src_addr[10] = 0xFF;
        event.src_addr[11] = 0xFF;
        event.src_addr[12] = 1;
        event.src_addr[13] = 2;
        event.src_addr[14] = 3;
        event.src_addr[15] = 4;

        event.dst_addr[10] = 0xFF;
        event.dst_addr[11] = 0xFF;
        event.dst_addr[12] = 5;
        event.dst_addr[13] = 6;
        event.dst_addr[14] = 7;
        event.dst_addr[15] = 8;

        let json = format_packet_event(&event);
        assert_eq!(json["flow_key"]["src_addr"], "1.2.3.4");
        assert_eq!(json["flow_key"]["dst_addr"], "5.6.7.8");
        assert_eq!(json["flow_key"]["src_port"], 12345);
        assert_eq!(json["flow_key"]["dst_port"], 80);
        assert_eq!(json["flow_key"]["protocol"], 6);
        assert_eq!(json["packet_len"], 1500);
        assert_eq!(json["action"], "pass");
        assert_eq!(json["direction"], "ingress");
        assert_eq!(json["timestamp_ns"], 1234567890u64);
        assert_eq!(json["trace_id"]["high"], 0u64);
    }

    #[test]
    fn test_packet_event_json_with_mesh() {
        let mut event = PacketEvent {
            timestamp_ns: 1234567890,
            src_addr: [0u8; 16],
            dst_addr: [0u8; 16],
            src_port: 443,
            dst_port: 8080,
            protocol: 6,
            af: AF_INET,
            _pad: [0; 2],
            packet_len: 500,
            direction: 0,
            _pad2: [0; 3],
        };
        // Stamp mesh meta
        event.src_addr[0] = 1; // version
        event.src_addr[1] = 3; // service_id
        event.src_addr[2] = 2; // hop_count
        event.src_addr[3] = FLOW_FLAG_SYN_SEEN | FLOW_FLAG_EST;
        event.src_addr[4] = 0xa1;
        event.src_addr[5] = 0xb2;
        event.src_addr[6] = 0xc3;
        event.src_addr[7] = 0xd4;
        event.src_addr[10] = 0xFF;
        event.src_addr[11] = 0xFF;
        event.src_addr[12] = 10;
        event.src_addr[13] = 10;
        event.src_addr[14] = 10;
        event.src_addr[15] = 20;

        event.dst_addr[0] = 1; // dst_service_id
        event.dst_addr[10] = 0xFF;
        event.dst_addr[11] = 0xFF;
        event.dst_addr[12] = 10;
        event.dst_addr[13] = 10;
        event.dst_addr[14] = 10;
        event.dst_addr[15] = 10;

        let json = format_packet_event(&event);
        assert!(json.get("mesh").is_some());
        let mesh = &json["mesh"];
        assert_eq!(mesh["version"], 1);
        assert_eq!(mesh["src_service_id"], 3);
        assert_eq!(mesh["dst_service_id"], 1);
        assert_eq!(mesh["hop_count"], 2);
        assert_eq!(mesh["trace_hash"], "a1b2c3d4");
        assert_eq!(mesh["nat_type"], "none");
    }

    #[test]
    fn test_mesh_meta_extraction_ipv4() {
        let mut src = [0u8; 16];
        let dst = [0u8; 16];
        src[0] = 1; // version
        src[1] = 5; // service_id
        src[2] = 3; // hop_count
        src[3] = FLOW_FLAG_EST | FLOW_FLAG_FIN_SEEN;
        src[4] = 0xDE;
        src[5] = 0xAD;
        src[6] = 0xBE;
        src[7] = 0xEF;
        src[8] = 0x00;
        src[9] = 0x42; // qos_class = 66

        let mesh = extract_mesh_meta(&src, &dst).unwrap();
        assert_eq!(mesh["version"], 1);
        assert_eq!(mesh["src_service_id"], 5);
        assert_eq!(mesh["hop_count"], 3);
        assert_eq!(mesh["trace_hash"], "deadbeef");
        assert_eq!(mesh["qos_class"], 66);
        let flags = mesh["flow_flags"].as_array().unwrap();
        assert!(flags.contains(&serde_json::json!("est")));
        assert!(flags.contains(&serde_json::json!("fin_seen")));
    }

    #[test]
    fn test_mesh_meta_ignored_ipv6() {
        // af=10 with no mesh version stamped
        let src = [0u8; 16]; // version=0
        let dst = [0u8; 16];
        assert!(extract_mesh_meta(&src, &dst).is_none());
    }

    #[test]
    fn test_flow_flags_bitfield() {
        let mut src = [0u8; 16];
        src[0] = 1; // version
        src[3] = FLOW_FLAG_SYN_SEEN | FLOW_FLAG_EST; // 0b00000011
        let dst = [0u8; 16];

        let mesh = extract_mesh_meta(&src, &dst).unwrap();
        let flags = mesh["flow_flags"].as_array().unwrap();
        assert_eq!(flags.len(), 2);
        assert!(flags.contains(&serde_json::json!("syn_seen")));
        assert!(flags.contains(&serde_json::json!("est")));
    }

    #[test]
    fn test_latency_event_json() {
        let event = LatencyEvent {
            timestamp_ns: 9999999999,
            pid: 1234,
            tid: 5678,
            latency_ns: 45000,
            operation: OP_TCP_SEND,
            _pad: [0; 7],
        };

        let json = format_latency_event(&event);
        assert_eq!(json["timestamp_ns"], 9999999999u64);
        assert_eq!(json["pid"], 1234);
        assert_eq!(json["tid"], 5678);
        assert_eq!(json["latency_ns"], 45000);
        assert_eq!(json["operation"], "tcp_send");
        assert_eq!(json["trace_id"]["high"], 0u64);
    }

    #[test]
    fn test_rate_limiter_allows() {
        let limiter = RateLimiter::new(100);
        let mut allowed = 0;
        for _ in 0..100 {
            if limiter.try_acquire() {
                allowed += 1;
            }
        }
        assert_eq!(allowed, 100);
    }

    #[test]
    fn test_rate_limiter_drops() {
        let limiter = RateLimiter::new(100);
        let mut allowed = 0;
        let mut dropped = 0;
        for _ in 0..200 {
            if limiter.try_acquire() {
                allowed += 1;
            } else {
                dropped += 1;
            }
        }
        assert_eq!(allowed, 100);
        assert_eq!(dropped, 100);
    }

    #[test]
    fn test_rate_limiter_counters() {
        let limiter = RateLimiter::new(50);
        for _ in 0..100 {
            limiter.try_acquire();
        }
        let (published, dropped) = limiter.stats();
        assert_eq!(published, 50);
        assert_eq!(dropped, 50);
    }

    #[test]
    fn test_wotan_buffer_eviction() {
        let buffer = EventBuffer::new(3);
        buffer.push(vec![1]);
        buffer.push(vec![2]);
        buffer.push(vec![3]);
        buffer.push(vec![4]); // evicts [1]

        let drained = buffer.drain();
        assert_eq!(drained.len(), 3);
        assert_eq!(drained[0], vec![2]);
        assert_eq!(drained[1], vec![3]);
        assert_eq!(drained[2], vec![4]);
    }

    #[test]
    fn test_detect_default_iface() {
        // Should return something (eth0 fallback if /proc not available)
        let iface = detect_default_iface();
        assert!(!iface.is_empty());
    }
}
