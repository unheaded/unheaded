// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

//! Userspace eBPF event collector.
//!
//! Loads compiled eBPF programs (XDP packet counter, kprobe TCP latency),
//! reads events from ring buffers, converts to JSON matching the dashboard
//! backend's types.go schema, and publishes to Wotan via gRPC (primary)
//! or HTTP (fallback), every request with ebpf-collector's EdDSA service
//! token (src/auth.rs, ADR-103 §6).

mod auth;

use std::collections::HashMap;
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
use tonic::service::interceptor::InterceptedService;
use tonic::transport::{Channel, Endpoint};

use ebpf_common::{
    LatencyEvent, PacketEvent, AF_INET, AF_INET6, FLOW_FLAG_EST, FLOW_FLAG_FIN_SEEN, FLOW_FLAG_OOO,
    FLOW_FLAG_RETRANSMIT, FLOW_FLAG_RST, FLOW_FLAG_SYN_SEEN, NAT_DNAT, NAT_MASQUERADE, NAT_SNAT,
    OP_TCP_CONNECT, OP_TCP_RECV, OP_TCP_SEND,
};

// Generated client code returns tonic::Status (176 bytes) in Err; that
// shape is tonic's, not ours to box.
#[allow(clippy::result_large_err)]
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

    /// Wotan HTTP address for fallback publishing (Wotan's HTTP port, 18000)
    #[arg(long, default_value = "http://localhost:18000")]
    wotan_http: String,

    /// Wotan gRPC address for primary publishing (Wotan's gRPC port, 18001)
    #[arg(long, default_value = "http://localhost:18001")]
    wotan_grpc: String,

    /// Path to compiled XDP program ELF
    #[arg(
        long,
        default_value = "../ebpf-programs/target/bpfel-unknown-none/release/packet-counter"
    )]
    xdp_program: String,

    /// Path to compiled kprobe program ELF
    #[arg(
        long,
        default_value = "../ebpf-programs/target/bpfel-unknown-none/release/tcp-latency"
    )]
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
}

// ---------------------------------------------------------------------------
// Wotan Publisher (gRPC primary, HTTP fallback)
// ---------------------------------------------------------------------------

/// The gRPC client: every call goes through WotanAuth, which adds the
/// service token (ADR-103 §6).
type WotanGrpc = TopicStreamClient<InterceptedService<Channel, auth::WotanAuth>>;

/// The gRPC publish request. No sender_id (Wotan takes only a member UUID
/// there): with auth on, Wotan approves the publish on the verified token
/// subject (configs/wotan.yaml auto_approve); display_name is a label.
fn grpc_publish_request(topic: &str, payload: Vec<u8>) -> TopicPublishRequest {
    TopicPublishRequest {
        topic: topic.to_string(),
        sender_id: String::new(),
        payload,
        metadata: [("display_name".to_string(), auth::IDENTITY.to_string())].into(),
    }
}

struct WotanPublisher {
    grpc_addr: String,
    http_addr: String,
    auth: auth::WotanAuth,
    grpc_client: tokio::sync::Mutex<Option<WotanGrpc>>,
    /// HTTP fallback: the approved subscriber id per topic, from
    /// POST /api/v1/topics/<topic>/subscribe; publishes name it.
    http_subscribers: std::sync::Mutex<HashMap<String, String>>,
    buffer: EventBuffer,
}

impl WotanPublisher {
    fn new(grpc_addr: String, http_addr: String, auth: auth::WotanAuth) -> Self {
        Self {
            grpc_addr,
            http_addr,
            auth,
            grpc_client: tokio::sync::Mutex::new(None),
            http_subscribers: std::sync::Mutex::new(HashMap::new()),
            buffer: EventBuffer::new(1000),
        }
    }

    async fn connect_grpc(&self) -> bool {
        let mut client = self.grpc_client.lock().await;
        if client.is_some() {
            return true;
        }
        let channel = match Endpoint::from_shared(self.grpc_addr.clone()) {
            Ok(e) => e.connect().await,
            Err(e) => {
                warn!("invalid Wotan gRPC address {}: {}", self.grpc_addr, e);
                return false;
            }
        };
        match channel {
            Ok(ch) => {
                info!("connected to Wotan gRPC at {}", self.grpc_addr);
                *client = Some(TopicStreamClient::with_interceptor(ch, self.auth.clone()));
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
                let req = tonic::Request::new(grpc_publish_request(topic, payload.clone()));
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

    /// POST body to Wotan's HTTP API with the service token; the response
    /// body on a 2xx.
    fn http_post(&self, path: &str, body: &serde_json::Value) -> Result<String, String> {
        let url = format!("{}{}", self.http_addr.trim_end_matches('/'), path);
        let mut req = ureq::post(&url).header("Content-Type", "application/json");
        if let Some(h) = self.auth.header().map_err(|e| e.to_string())? {
            req = req.header("Authorization", &h);
        }
        let mut resp = req
            .send(&serde_json::to_vec(body).map_err(|e| e.to_string())?[..])
            .map_err(|e| e.to_string())?;
        resp.body_mut().read_to_string().map_err(|e| e.to_string())
    }

    /// The approved subscriber id this collector publishes topic under,
    /// joining the topic as ebpf-collector on first use.
    fn http_subscriber(&self, topic: &str) -> Result<String, String> {
        if let Some(id) = self.http_subscribers.lock().unwrap().get(topic) {
            return Ok(id.clone());
        }
        let body = self.http_post(
            &format!("/api/v1/topics/{topic}/subscribe"),
            &serde_json::json!({ "display_name": auth::IDENTITY }),
        )?;
        let v: serde_json::Value =
            serde_json::from_str(&body).map_err(|e| format!("subscribe response: {e}"))?;
        let sub = &v["subscriber"];
        let (Some(id), Some("approved")) = (sub["subscriber_id"].as_str(), sub["status"].as_str())
        else {
            return Err(format!(
                "join {topic} as {}: status {} (is it on Wotan's auto_approve?)",
                auth::IDENTITY,
                sub["status"]
            ));
        };
        self.http_subscribers
            .lock()
            .unwrap()
            .insert(topic.to_string(), id.to_string());
        Ok(id.to_string())
    }

    /// One HTTP publish: Wotan's {"subscriber_id", "payload"} body. A
    /// refusal drops the subscriber id so the next attempt joins again
    /// (Wotan restarted, or the member expired).
    fn publish_http_once(&self, topic: &str, payload: &[u8]) -> Result<(), String> {
        let id = self.http_subscriber(topic)?;
        let body = serde_json::json!({
            "subscriber_id": id,
            "payload": String::from_utf8_lossy(payload),
        });
        self.http_post(&format!("/api/v1/topics/{topic}/publish"), &body)
            .map(|_| ())
            .inspect_err(|_| {
                self.http_subscribers.lock().unwrap().remove(topic);
            })
    }

    async fn publish_http_with_retry(&self, topic: &str, payload: &[u8]) {
        let mut backoff = Duration::from_millis(100);
        let max_retries = 5;

        for attempt in 0..max_retries {
            match self.publish_http_once(topic, payload) {
                Ok(()) => return,
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

    let latency_hint_ns = u32::from_be_bytes([dst_addr[4], dst_addr[5], dst_addr[6], dst_addr[7]]);

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
    // Service token for Wotan: AUTH_KEY_FILE, else
    // /etc/unheaded/keys/ebpf-collector.key (missing: one warning, no token).
    let publisher = Arc::new(WotanPublisher::new(
        args.wotan_grpc.clone(),
        args.wotan_http.clone(),
        auth::WotanAuth::load(),
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

    // --- Wotan service token (ADR-103 §6) ---------------------------------

    use std::io::{BufRead, BufReader, Read as _, Write as _};
    use wotan_proto::topic_stream_server::{TopicStream, TopicStreamServer};
    use wotan_proto::{
        TopicEvent, TopicPingRequest, TopicPongResponse, TopicPublishResponse, TopicStreamRequest,
    };

    fn test_auth() -> auth::WotanAuth {
        // A seed made at run time, never a literal key.
        let mut seed = [0u8; 32];
        for (i, b) in seed.iter_mut().enumerate() {
            *b = (i as u8).wrapping_mul(29).wrapping_add(3);
        }
        auth::WotanAuth::with_minter(svc_token::Minter::from_seed(auth::IDENTITY, &seed).unwrap())
    }

    /// Wotan's gRPC topic service, recording each publish and the
    /// authorization metadata it came with.
    /// A publish as the mock saw it: authorization metadata and request.
    type SeenPublish = (Option<String>, TopicPublishRequest);

    #[derive(Clone, Default)]
    struct MockWotan {
        seen: Arc<std::sync::Mutex<Vec<SeenPublish>>>,
    }

    #[tonic::async_trait]
    impl TopicStream for MockWotan {
        type StreamTopicsStream = std::pin::Pin<
            Box<
                dyn tonic::codegen::tokio_stream::Stream<Item = Result<TopicEvent, tonic::Status>>
                    + Send,
            >,
        >;

        async fn stream_topics(
            &self,
            _: tonic::Request<TopicStreamRequest>,
        ) -> Result<tonic::Response<Self::StreamTopicsStream>, tonic::Status> {
            Err(tonic::Status::unimplemented("mock"))
        }

        async fn publish_topic(
            &self,
            req: tonic::Request<TopicPublishRequest>,
        ) -> Result<tonic::Response<TopicPublishResponse>, tonic::Status> {
            let authz = req
                .metadata()
                .get("authorization")
                .map(|v| v.to_str().unwrap().to_string());
            self.seen.lock().unwrap().push((authz, req.into_inner()));
            Ok(tonic::Response::new(TopicPublishResponse::default()))
        }

        async fn topic_ping(
            &self,
            _: tonic::Request<TopicPingRequest>,
        ) -> Result<tonic::Response<TopicPongResponse>, tonic::Status> {
            Err(tonic::Status::unimplemented("mock"))
        }
    }

    async fn grpc_publish_with(auth: auth::WotanAuth) -> Vec<SeenPublish> {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let mock = MockWotan::default();
        let seen = mock.seen.clone();
        let incoming =
            tonic::transport::server::TcpIncoming::from_listener(listener, true, None).unwrap();
        let server = tokio::spawn(
            tonic::transport::Server::builder()
                .add_service(TopicStreamServer::new(mock))
                .serve_with_incoming(incoming),
        );
        let p = WotanPublisher::new(format!("http://{addr}"), "http://127.0.0.1:1".into(), auth);
        assert!(p.connect_grpc().await, "connect to mock Wotan");
        p.publish("ebpf.packet.events", br#"{"x":1}"#.to_vec())
            .await;
        server.abort();
        let got = seen.lock().unwrap().clone();
        got
    }

    #[tokio::test]
    async fn grpc_publish_carries_service_token_and_no_sender_id() {
        let auth = test_auth();
        let got = grpc_publish_with(auth.clone()).await;
        assert_eq!(got.len(), 1, "one publish reached Wotan");
        let (authz, req) = &got[0];
        // The token the minter has cached for aud=wotan: sub=ebpf-collector.
        assert_eq!(authz.as_deref(), auth.header().unwrap().as_deref());
        assert!(authz.as_deref().unwrap().starts_with("Bearer ey"));
        assert_eq!(req.topic, "ebpf.packet.events");
        // Wotan takes only a member UUID as sender_id; an unnamed publish
        // is approved on the token's subject.
        assert_eq!(req.sender_id, "");
        assert_eq!(
            req.metadata.get("display_name").map(String::as_str),
            Some("ebpf-collector")
        );
        assert_eq!(req.payload, br#"{"x":1}"#);
    }

    #[tokio::test]
    async fn grpc_publish_without_key_sends_no_token() {
        let got = grpc_publish_with(auth::WotanAuth::default()).await;
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].0, None);
    }

    /// One HTTP request as the mock saw it.
    #[derive(Debug, Clone)]
    struct SeenHttp {
        line: String,
        authorization: Option<String>,
        body: String,
    }

    /// A minimal Wotan HTTP API: answers `n` connections (subscribe: an
    /// approved member; publish: 201) and returns what it saw.
    fn mock_wotan_http(n: usize) -> (String, std::thread::JoinHandle<Vec<SeenHttp>>) {
        let l = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let base = format!("http://{}", l.local_addr().unwrap());
        let h = std::thread::spawn(move || {
            let mut seen = Vec::new();
            for _ in 0..n {
                let (stream, _) = l.accept().unwrap();
                let mut r = BufReader::new(stream.try_clone().unwrap());
                let mut line = String::new();
                r.read_line(&mut line).unwrap();
                let (mut len, mut authz) = (0usize, None);
                loop {
                    let mut h = String::new();
                    r.read_line(&mut h).unwrap();
                    let h = h.trim_end();
                    if h.is_empty() {
                        break;
                    }
                    let (k, v) = h.split_once(':').unwrap();
                    match k.to_ascii_lowercase().as_str() {
                        "content-length" => len = v.trim().parse().unwrap(),
                        "authorization" => authz = Some(v.trim().to_string()),
                        _ => {}
                    }
                }
                let mut body = vec![0u8; len];
                r.read_exact(&mut body).unwrap();
                let reply = if line.contains("/subscribe ") {
                    r#"{"subscriber":{"subscriber_id":"6f1c2b9e-0000-4000-8000-000000000001","status":"approved"}}"#
                } else {
                    "{}"
                };
                let mut w = stream;
                write!(
                    w,
                    "HTTP/1.1 201 Created\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    reply.len(),
                    reply
                )
                .unwrap();
                seen.push(SeenHttp {
                    line: line.trim_end().to_string(),
                    authorization: authz,
                    body: String::from_utf8(body).unwrap(),
                });
            }
            seen
        });
        (base, h)
    }

    #[test]
    fn http_fallback_joins_then_publishes_with_token() {
        let (base, server) = mock_wotan_http(3);
        let auth = test_auth();
        let p = WotanPublisher::new("http://127.0.0.1:1".into(), base, auth.clone());
        p.publish_http_once("ebpf.latency.events", br#"{"rtt":5}"#)
            .unwrap();
        // The subscriber id is reused: no second join.
        p.publish_http_once("ebpf.latency.events", br#"{"rtt":6}"#)
            .unwrap();
        let seen = server.join().unwrap();
        let want = auth.header().unwrap();
        assert!(want.is_some());
        for s in &seen {
            assert_eq!(s.authorization, want, "{}", s.line);
        }
        assert!(seen[0]
            .line
            .starts_with("POST /api/v1/topics/ebpf.latency.events/subscribe "));
        let join: serde_json::Value = serde_json::from_str(&seen[0].body).unwrap();
        assert_eq!(join["display_name"], "ebpf-collector");
        for (s, rtt) in seen[1..].iter().zip([5, 6]) {
            assert!(s
                .line
                .starts_with("POST /api/v1/topics/ebpf.latency.events/publish "));
            let b: serde_json::Value = serde_json::from_str(&s.body).unwrap();
            assert_eq!(b["subscriber_id"], "6f1c2b9e-0000-4000-8000-000000000001");
            assert_eq!(b["payload"], format!(r#"{{"rtt":{rtt}}}"#));
        }
    }

    #[test]
    fn http_fallback_without_key_sends_no_authorization() {
        let (base, server) = mock_wotan_http(2);
        let p = WotanPublisher::new(
            "http://127.0.0.1:1".into(),
            base,
            auth::WotanAuth::default(),
        );
        p.publish_http_once("ebpf.packet.events", b"{}").unwrap();
        let seen = server.join().unwrap();
        assert!(seen.iter().all(|s| s.authorization.is_none()));
    }

    #[test]
    fn http_join_not_approved_is_an_error() {
        let l = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let base = format!("http://{}", l.local_addr().unwrap());
        let h = std::thread::spawn(move || {
            let (mut s, _) = l.accept().unwrap();
            let mut buf = [0u8; 4096];
            let _ = s.read(&mut buf).unwrap();
            let reply = r#"{"subscriber":{"subscriber_id":"x","status":"pending"}}"#;
            write!(
                s,
                "HTTP/1.1 201 Created\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                reply.len(),
                reply
            )
            .unwrap();
        });
        let p = WotanPublisher::new("http://127.0.0.1:1".into(), base, test_auth());
        let err = p
            .publish_http_once("ebpf.packet.events", b"{}")
            .unwrap_err();
        h.join().unwrap();
        assert!(
            err.contains("pending") && err.contains("auto_approve"),
            "{err}"
        );
    }

    #[test]
    fn default_wotan_ports_are_wotans() {
        let a = Args::parse_from(["ebpf-collector"]);
        assert_eq!(a.wotan_http, "http://localhost:18000");
        assert_eq!(a.wotan_grpc, "http://localhost:18001");
    }
}
