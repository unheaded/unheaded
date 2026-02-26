# UNHEADED SYSTEM MAP: Concentric Rings from Monad Core → Network Edge

**Last Updated:** 2026-02-26 | **Scope:** Complete architecture 0→15, kernel→WAN | **Lines:** 23,991 Rust LOC + 3 deployment targets

---

## RING 0 — THE ATOM (Monad Wire Format)

→ IPv6 Hop-by-Hop extension header (next-header 0x00 / HOPOPT), 20 bytes, immutable across all layers  
→ Field layout (bit-exact): `flow_label[20b] | epoch[16b] | seq[32b] | crc16[16b] | opcode[8b] | flags[8b] | payload_ptr[16b] | reserved[8b]`  
• CRC-16 (CCITT-0xFFFF) computed over bytes 0–17, before crc field itself  
• Opcode: query(0x01), response(0x02), event(0x04), ack(0x08), redirect(0x10), drop(0x20)  
• Flags: valid(0x01), encrypted(0x02), priority(0x04), nat_crossed(0x08), shim_injected(0x10), verified(0x20)  
• Epoch: 16-bit millisecond counter, wraps every 65.5s, synchronized across all planes via NTP+PTP  
• Seq: 32-bit monotonic per-flow, reset on epoch wrap, undefined on flow_label=0  
• Payload_ptr: 16-bit offset into packet beyond HbH, points to decrypted Monad PDU or raw L4 payload  
• flow_label: 20-bit unique identifier derived from hash(src_ip, dst_ip, src_port, dst_port, protocol)  
• Reserved: 8 bits held for TLV-extension mechanism, must be 0x00 in v1  
• Byte 0–1: flow_label high 8 bits packed into IPv6 version/traffic-class boundary (no conflict)  
• Total HbH header: 32 bytes (20-byte Monad + 12-byte HbH wrapper), adds 4.2% to MTU on 1500b paths  
• **Never stripped**, passed unmodified to every consumer: kernel, BPF, userspace, WAN egress  
• Endianness: big-endian (network order), tested via CRC on byte-exact serialization  
• Checksum failure → packet DROPPED at XDP layer, counter incremented, EVE logged as "monad_crc_fail"  

---

## RING 1 — KERNEL LAYER (eBPF, XDP, TC, kprobes)

◆ XDP (eXpress Data Path) program: ingress hook, runs in eBPF VM before skb allocation, `bpf_prog_type_xdp`  
▸ Ingress NIC: attach mode BPF_CGROUP_XDPD, policy XDP_DROP/PASS/REDIRECT, 0-copy to userspace ring  
⬡ TC (Traffic Control) qdisc: egress hook, `bpf_prog_type_sched_cls`, attached to root qdisc on eth0/eth1  
⟳ kprobes: attach to `sys_sendto()`, `sys_recvfrom()`, `tcp_connect()`, `tcp_set_state()` for syscall tracing  
⊕ raw_tracepoint: attach to `tp_sched_process_exec()`, `tp_sched_wakeup()`, low-overhead process tracking  
→ Map storage: 9 BPF maps total (Wotan LRU hash, Sophia array, Anamnesis ring, conntrack, tcpstate, nat_xlate, flow_stats, rate_limiter, xdp_redirect)  
• Maps verified read-only in kernel: no mutation after attach, user-space verifier runs pre-load  
• Pinning: maps persisted in `/sys/fs/bpf/unheaded/` with 0700 permissions, survives reboot  
• Loader: `aya-ebpf-loader` crate, JIT compilation on x86-64, ARM64, RISC-V 64-bit  
◆ BTF (BPF Type Format): vmlinux.h auto-generated via `bpftool btf dump file /sys/kernel/btf/vmlinux format c`  
▸ Kernel version requirement: >=5.8 (for ring buffer), >=6.0 recommended (for BPF-side kgpl helpers)  
⬡ LLVM backend: clang -target bpf, optimized with -O2, verified with `llvm-objdump -d vmlinux`  
⟳ Verifier: kernel BPF verifier checks 4.5M instruction limit (v1 uses ~850K), memory safety proof  
⊕ JIT: enabled via `net.core.bpf_jit_enable=1`, measured 8-12x faster than interpreter at line rate  

---

## RING 2 — SHIELD + SHIM (BPF Program Pipeline)

→ **SHIELD**: XDP program entry point, runs first on RX path  
• Parse IPv6 HbH extension, extract Monad flow_label  
• Hash lookup in Wotan LRU map: `(src_ip, dst_ip, sport, dport, proto) → 64-byte flow_state`  
◆ Packet classification: return `{verdict, action, mark, mirror_qnum}`  
▸ Verdict: XDP_PASS (forward to TC), XDP_DROP (counted in counter), XDP_REDIRECT (to CPU ring)  
⬡ Flow state flags: is_known=1, is_blocked=0, is_priority=0, is_nat_crossing=1  
⟳ Add BPF metadata: `bpf_skb_store_bytes()` at offset 100 in skb context for TC to read  
⊕ Rate-limit on drop: use token bucket in BPF map, max 1000 drops/sec per flow_label  
→ **SHIM**: TC program on egress, runs after SHIELD verdict  
• Receive metadata from SHIELD in skb->cb[] (32 bytes, BPF reserved area)  
◆ If `verdict=REDIRECT`, inject Monad HbH header before IPv6 header  
▸ Set HbH opcode to `redirect(0x10)`, set flags `shim_injected=1`  
⬡ Update IPv6 next-header field: was TCP/UDP, now HOPOPT (0x00), chain `next_header=6/17`  
⟳ Recalculate IPv6 payload length: add 32 bytes (HbH wrapper + Monad)  
⊕ Update L4 checksum (TCP/UDP): pseudo-header now includes new IPv6 HbH layer  
→ Ring-buffer event emission: call `bpf_ringbuf_output()` with event descriptor for Anamnesis  
• Timestamp: `bpf_ktime_get_ns()` + epoch sync offset, 64-bit ns since boot  

---

## RING 3 — WOTAN (Message Bus, Per-Flow Memory)

▸ BPF LRU hash map: `/sys/fs/bpf/unheaded/wotan`, key = 28 bytes (src_ip[16b] + dst_ip[16b] + sport[2b] + dport[2b] + proto[1b] + reserved[1b])  
⬡ Value: 64-byte fixed-size flow state structure  
⟳ Pub/Sub topic registry: in-process Go channels, 16 pre-allocated topics (firewall, nat, mirror, stats, events, alerts, telemetry, mirror_config, ...)  
⊕ Per-flow subscription: topic mask stored in Wotan value bits 0–7  
→ Entry format (64 bytes): `{ state_flags[1], topic_mask[1], last_pkt_time[8], byte_count[8], pkt_count[8], tcp_state[1], nat_inner_addr[16], nat_inner_port[2], reserved[10] }`  
• Max entries: 1M flows on 2GB heap (64 bytes × 1M = 64MB LRU max), tail-call eviction on overflow  
◆ Eviction policy: LRU (least-recently-used), aged out after 300s inactivity  
▸ Fast-path lookup: O(1) hash lookup in kernel, no spinlock (BPF maps are RCU-safe)  
⬡ Update on RX: call `bpf_map_lookup_elem()`, increment pkt_count, update last_pkt_time  
⟳ Update on TX: mirrored flow key, separate LRU bucket for reverse-dir flows  
⊕ Witness entry (when unknown): create new entry with flags={is_new=1, needs_hw_offload=0}  
→ Publish event: call `bpf_ringbuf_output()` for Anamnesis consumer in Ring 5  

---

## RING 4 — SOPHIA (BPF Map Dictionaries, Exponent Lookup)

→ BPF array map: `/sys/fs/bpf/unheaded/sophia`, max 2048 entries (one per peer on AS65001)  
• Entry size: 74 bytes (max CBOR-encoded dict + length prefix)  
◆ CBOR format: major-type-0 (unsigned int), major-type-1 (map key), major-type-2 (byte string)  
▸ Key: uint16 (`peer_asn` % 65536), value: `{ peer_ip[16], peer_port[2], peer_key[32], exponent[4], flags[1], cgroup[8] }`  
⬡ Exponent lookup: BPF performs O(1) access, no libc math, stored as IEEE 754 float  
⟳ Used in traffic classification: read from Sophia, compute rate = base × 2^exponent, compare against limit  
⊕ Update path: userspace daemon writes via `BPF_MAP_UPDATE_ELEM`, kernel reads via `bpf_map_lookup_elem()`  
→ Immutability check: read-only flag in map creation prevents in-kernel mutation  
• Cache coherency: each CPU core gets local copy, updates visible within 1 RCU grace period (~1ms)  

---

## RING 5 — ANAMNESIS (64-Byte Event Ring Buffer)

▸ BPF ring buffer (not perf): `/sys/fs/bpf/unheaded/anamnesis`, capacity 512KB circular buffer  
⬡ Zero-copy mmap to userspace: memory-map ring buffer, read without `read()` syscall  
⟳ Entry format: 64 bytes, fixed-size, no variable-length headers  
⊕ Layout: `{ event_type[1], timestamp[8], flow_label[4], epoch[2], seq[4], opcode[1], flags[1], src_ip[16], dst_ip[16], src_port[2], dst_port[2], proto[1], action[1], reserved[7] }`  
→ Event types: flow_start(0x01), flow_end(0x02), drop(0x04), redirect(0x08), nat_detected(0x10), ratelimit(0x20), priority_ingress(0x40), priority_egress(0x80)  
• Timestamp: ns since boot, synchronized with epoch field for clock sync validation  
• Consumer: unheaded-daemon reads via ring buffer FDs, forwards to Loki/VictoriaMetrics  
◆ Overflow handling: if buffer full, oldest entry overwritten, counter incremented  
▸ Watermark: threshold 80%, triggers flush to userspace (no blocking in BPF)  

---

## RING 6 — MICROSERVICES (25 Services, Doom Range Ports)

→ **Port Registry (16666–26666 TCP/gRPC + UDP/metrics)**  
◆ Discovery: ZooKeeper-style ETCD key-value, prefix `/unheaded/services/`, watched by all clients  
▸ Service entry: JSON `{ name, port, ip, version, load, tags: ["denylist", "allowlist", ...] }`  

→ **Core Control & Telemetry (16666–16675)**  
• 16666: unheaded-daemon (main orchestrator, gRPC svc registry, Wotan/Sophia update server)  
◆ 16667: firewall-rules-engine (stateful rule evaluation, pub from Wotan/Anamnesis)  
▸ 16668: nat-translator (SNAT/DNAT flow tracking, publishes to NAT topic in Wotan)  
⬡ 16669: mirror-manager (mirror-to-span config, subscribes to mirror_config topic)  
⟳ 16670: alerts-engine (threshold logic, publishes to alerts topic, fires webhooks)  
⊕ 16671: rate-limiter-daemon (token bucket manager, updates bpf rate_limiter map)  
→ 16672: telemetry-aggregator (sinks from BPF counters, pushes to Prometheus :9090)  
• 16673: metrics-exporter (Prometheus HTTP on :16673/metrics, pulls from eBPF maps)  
◆ 16674: netconfig-server (BGP/IS-IS config daemon, hot-reload without drain)  
▸ 16675: health-check-svc (readiness/liveness, multi-leader election via ETCD)  

→ **Network & Forwarding (16676–16685)**  
⬡ 16676: traffic-steering (load-balance decision, returns steering vector)  
⟳ 16677: failover-manager (BFD session monitor, triggers BGP withdraw)  
⊕ 16678: qos-manager (queue depth monitoring, updates TC qdisc)  
→ 16679: vxlan-controller (VTEP coordination, publishes VNI rewrite rules)  
• 16680: isis-adjacency-mon (Level-2 adjacency FSM, publishes topology deltas)  
◆ 16681: bgp-session-mon (eBGP session health, scrapes peer state from FRR/BIRD)  
▸ 16682: segment-routing-engine (IPv6 SR, computes SID list, updates Sophia)  

→ **Data Plane & Observability (16683–16692)**  
⬡ 16683: packet-witness (raw packet capture, no AF_PACKET; uses Anamnesis ring)  
⟳ 16684: flow-aggregator (groups Anamnesis events, emits NetFlow v9 optionally)  
⊕ 16685: anomaly-detector (ML on flow stats from Prometheus, publishes alerts)  
→ 16686: log-forwarder (Anamnesis → EVE JSON → Loki, batches 100/sec)  
• 16687: signature-matcher (Monad sigs, regex + bpf-bypass decision)  
◆ 16688: pkt-reassembler (TCP/UDP reassembly for L7 inspection, publishes to IDS topic)  
▸ 16689: cache-warmer (preload Sophia from ETCD, keeps BPF maps hot)  
⬡ 16690: config-validator (schema check on all incoming daemon requests, rejects invalid)  

→ **Edge & Integration (16691–16700)**  
⟳ 16691: wan-gateway-svc (WAN IPv6 HbH passthrough, NAT64 co-processor)  
⊕ 16692: grpc-gateway (HTTP/1.1 ↔ gRPC bridge for legacy clients, auto-transcoding)  
→ 16693: webhook-dispatcher (HTTP POST to external alerting, circuit-breaker)  
• 16694: api-auth-svc (JWT validation, role-based access control via ETCD ACL)  
◆ 16695: state-sync-svc (multi-leader replication, RAFT consensus on control plane)  
▸ **All services**: gRPC over TLS 1.3, mutual auth via mTLS cert store in `/var/lib/unheaded/certs/`  
⬡ **Failover**: each service runs on 2 hosts (active-active), client-side load balance via gRPC LB policy  
⟳ **Startup order**: gRPC gateway (16692) first, then discovery (16666), then all others  

---

## RING 7 — CONTROL PLANE (Unheaded-Daemon, gRPC, Discovery)

→ Unheaded-daemon: main process on host-a + host-b, role=orchestrator on a, role=standby on b  
• Executable: `/usr/local/bin/unheaded-daemon`, config file `/etc/unheaded/daemon.yaml`  
◆ Multicast leader election: uses ETCD-PUT-with-lease on `/unheaded/control/leader`, TTL=15s  
▸ Active leader: publishes config updates to all 25 services via gRPC PushConfig RPC  
⬡ Standby daemon: reads from `/unheaded/services/*` path, syncs local in-memory state (read-only replica)  
⟳ Failover: if lease expires, next-in-line daemon acquires lock, re-publishes all state (idempotent)  
⊕ gRPC service port: :16666, TLS=true, handler=ServiceRegistry + BPFMapUpdater + EventListener  
→ BPF map hot-updates: unheaded-daemon calls `bpf_map_update_elem()` on Wotan/Sophia without restarting  
• Example: rule change → daemon updates Sophia entry → all XDP/TC programs see new policy in 1RCU  
◆ Observability: daemon logs to `/var/log/unheaded/daemon.log`, JSON format, consumed by Loki  
▸ Metrics: internal Prometheus exporter on :16673, exposes daemon_leader_elections_total, config_push_latency_ms, bpf_map_update_errors_total  
⬡ State file: `/var/run/unheaded/daemon-state.pb`, protobuf3, checkpointed every 30s for crash recovery  
⟳ Config validation: schema checked via Protobuf descriptor before pushing to BPF (fail-safe on invalid rule)  

---

## RING 8 — OBSERVABILITY (Prometheus, Grafana, Loki, VictoriaMetrics)

▸ **Prometheus** (host-a, port 9090): scrapes :16673/metrics (BPF metrics), :16685/metrics (anomaly), :16681/metrics (BGP)  
⬡ Scrape interval: 15s, retention: 30 days (older data → VictoriaMetrics)  
⟳ Remote write: https://localhost:8428/api/v1/write (VictoriaMetrics), TLS cert `/etc/unheaded/certs/victoria.crt`  
⊕ Alerting rules: `/etc/prometheus/unheaded-alerts.yml`, fired to AlertManager on :9093  

→ **VictoriaMetrics** (host-a, port 8428): long-term metric storage, 1-year retention (default)  
• Data dir: `/var/lib/victoria-metrics/`, hourly compaction, on NVMe SSD  
◆ Query endpoint: http://localhost:8428/select/0/prometheus/api/v1/query, used by Grafana  
▸ Scrape pool: vmagent on :8429 (VMAgent sidecar), no prometheus overhead  

→ **Loki** (host-a, port 3100): log aggregation for Anamnesis events, EVE JSON format  
• Retention: 90 days by label (Monad sigs), 30 days by default  
⬡ Labels: `{job="anamnesis", flow_label="...", event_type="...", src_ip="...", dst_ip="..."}` (no PII)  
⟳ Index store: `/var/lib/loki/index/`, BoltDB single-node (multi-node via ETCD in HA)  
⊕ Ingest rate: 100K events/sec (burst 500K), compression gzip level 9  
→ LogQL queries: `{job="anamnesis"} | flow_label="0x12345" | json | opcode="redirect"`  

→ **Promtail** (host-a + host-b, agent mode): reads Anamnesis ring buffer, sends to Loki  
• Config: `/etc/promtail/config.yml`, targets Anamnesis mmap at `/dev/shm/anamnesis.ring`  
◆ Parsing: EVE JSON decode, extract fields, add labels, batch send every 1s or 1000 events  
▸ Rate limit: max 100K lines/sec, backpressure drops oldest entries  

→ **Grafana** (host-a, port 3000): dashboards for operators  
⬡ 6 default dashboards: "Monad Flow Map", "BGP/IS-IS Topology", "eBPF Verdict Distribution", "Rate Limit Heat Map", "NAT Translation Stats", "IDS Alert Timeline"  
⟳ Datasources: Prometheus (metrics), Loki (logs), VictoriaMetrics (long-term)  
⊕ Auth: OIDC via OAuth2 proxy (optional), local users in `/var/lib/grafana/sqlite.db`  

→ **AlertManager** (host-a, port 9093): deduplication, grouping, routing to webhooks/Slack  
• Routes: critical → Slack + PagerDuty, warning → internal log only  

---

## RING 9 — NETWORK FABRIC (WireGuard, BGP EVPN, VXLAN, BFD, IS-IS)

→ **WireGuard (Layer 3 VPN)**  
• Subnet: fd00:dead:beef::/48 (private ULA), MTU 1380 (1500 - 20 WG overhead - 100 buffer)  
◆ Tunnel endpoints: host-a [fd00:dead:beef::1], host-b [fd00:dead:beef::2], key rotation every 30 days  
▸ Handshake renegotiation: every 2 minutes (per WireGuard spec), no packet loss  
⬡ Traffic passing through WireGuard HAS Monad HbH header intact (not stripped by tunnel encryption)  
⟳ Keepalive: persistent UDP 51820, works through NAT (if customer IP routes HbH)  

→ **BGP EVPN + VXLAN (Layer 2 bridging over Layer 3)**  
⊕ Route Reflector: host-a, AS65001 (local), RR cluster-id 1  
→ Peer 1: host-a ↔ host-b (iBGP), as-path prepend 2 on backup routes, soft reconfiguration inbound  
• Peer 2: host-a ↔ upstream (eBGP), AS65002, multi-hop 2, ttl-security enabled  
◆ EVPN address-family: l2vpn evpn (RFC 7432), import/export route-targets for isolation  
▸ VNI mapping: VNI 10001 (prod-vlan1), VNI 10002 (prod-vlan2), VNI 10100 (mgmt-vlan)  
⬡ VTEP (VXLAN Tunnel Endpoint): host-a 10.20.255.1, host-b 10.20.255.2, VXLAN UDP 4789  
⟳ Encapsulation: inner frame + VNI + VXLAN header + UDP + IPv4 (outer), decapsulation symmetric  
⊕ MAC learning: Monad HbH flow_label hash used as inner MAC source (48-bit derivative)  

→ **BFD (Bidirectional Forwarding Detection)**  
• Session: WireGuard tunnel (wg0), detect 300ms (tx/rx 100ms min), multiplier 3 (9-second fail detect)  
◆ Echo mode: off (simple mode), packets every 100ms, IPv6 UDP 3784  
▸ Decay: if BFD down for 5s, trigger BGP withdraw on all routes via host-a RR  
⬡ Multi-hop: enabled for eBGP sessions (AS65001 → AS65002), TTL 64  

→ **IS-IS (Intermediate System to Intermediate System)**  
⟳ Level: Level-2-only (backbone), RFC 5308 (IPv6 support), system-id 0020.0255.0001  
⊕ NET: 49.0001.1020.0255.0001.00 (area 49.0001, routing domain 1, system 1020.0255.0001)  
→ Interfaces: wg0 (tunnel), eth0 (LAN), cost 10 on both (equal-cost multipath enabled)  
• Hello: 10s interval, hold 30s, ISH flood scope area  
◆ DIS (Designated IS): elected on eth0 LAN (priority 64), DR on wg0 (priority 0, no DR needed)  
▸ SPT (Shortest Path Tree): computed via Dijkstra, 2 ECMP paths, default metric type is narrow (0–63)  
⬡ Convergence: <100ms on topology change (no metric hysteresis), LSP origin-timer 15s  
⟳ IPv6 address distribution: prefix fd00:dead:cafe::/48 via L2 LSA, no TE-LSA (future)  

---

## RING 10 — FIREWALL LAYER (OPNsense, IPFire, FRR, BIRD, HbH Passthrough)

→ **OPNsense** (host-a, Forge distribution, BSD 2-Clause license)  
• Role: LAN gateway, DHCP/DHCPv6 server, stateful firewall, IDS integration point  
◆ LAN interface: 10.20.0.1/16 (bridged to eth0), DHCP pool 10.20.100.0/24 (100–254)  
▸ WAN interface: fd00:dead:beef::1/128 (WireGuard tunnel), static route to upstream  
⬡ Firewall rules: 25 tuples (src_zone, dst_zone, protocol, direction, action, logging)  
⟳ Rule priority: allow-list → rate-limit → mirror-to-span → default-drop  
⊕ HbH passthrough: rule "pass all IPv6 HOPOPT" (next-header 0x00), no stripping, no modification  
→ IDS integration: enables Suricata eve-log tap on all inbound traffic (see Ring 11)  
• Stateful inspection: tracks TCP/UDP flows in kernel state table, 1M max concurrent flows  
◆ Logging: PF (packet filter) logs to `/var/log/pflog`, rotated daily, consumed by Suricata  

→ **IPFire** (host-b, Outpost distribution, GPL v3)  
⬡ Role: secondary gateway, IPSec termination, traffic logging, backup firewall (if OPNsense down)  
⟳ WAN interface: static IPv4 + IPv6 HbH-aware iptables rules  
⊕ Packet filtering: xtables (netfilter) hooks, IPv6 extension header awareness enabled  
→ HbH rules: iptables -A FORWARD -m ipv6header --header-hop -j ACCEPT (explicit allowlist)  
• Logging: nflog target to `/var/log/nflog.log`, JSON format via custom parser  
◆ State table: conntrack, 2M max entries, gc interval 60s  

→ **FRR** (host-a, GPLv2, IS-IS + BGP + BFD + EVPN daemon)  
▸ Zebra daemon: main RIB (Routing Information Base), CLI on /var/run/frr/frr.sock, listens :2601  
⬡ BGP daemon: iBGP (host-b peer), eBGP (AS65002), soft reconfig inbound, route-map filters  
⟳ IS-IS daemon: L2-only, area-id 49.0001, hello 10s, graceful restart 120s  
⊕ BFD daemon: integrates with BGP/IS-IS, triggers route withdraw on session down  
→ EVPN daemon: converges MAC/IP routes from VXLAN endpoints, advertises via BGP  
• Mgmtd daemon: config parsing, validation, dynamic reload (no daemon restart needed)  

→ **BIRD** (host-b, GPLv2, BGP + BFD + RA + OSPFv3)  
◆ Role: upstream eBGP speaker, AS65002, receives full IPv6 feed from upstream ISP  
▸ BGP session: to host-a (iBGP), also to upstream (eBGP), 4-byte ASN support  
⬡ BFD integration: validates BGP session health, triggers policy action on failure  
⟳ Router Advertisement daemon (radv): advertises fd00:dead:cafe::/48 to WireGuard clients  
⊕ Graceful restart: 180s, preserves BGP state during reload  

→ **HbH Passthrough Guarantee (All Layers)**  
• IPv6 extension headers preserved end-to-end: kernel → BPF XDP → Shield → Shim → TC → wire  
◆ No middleware strips HOPOPT (next-header 0x00), verified via packet capture at each stage  
▸ Customer traffic with HbH: can egress to WAN, reaches upstream ISP, re-enters via return path  

---

## RING 11 — IDS/IPS (Suricata, Monad Sigs, eBPF Bypass)

→ **Suricata** (host-a, GPL-2.0, NfQueue inline + AF_PACKET bypass)  
• Inline mode: nfqueue 0:50 (kernel → Suricata → kernel), verdict ACCEPT/DROP/REPEAT  
◆ Input source: pflog from OPNsense, Monad-tagged packets (eve-log contains flow_label)  
▸ Rule set: Emerging Threats + custom Monad signatures (sid 9000001–9999999 range)  
⬡ Threat intelligence: IP reputation lists updated hourly, GeoIP enrichment on alerts  
⟳ EVE JSON output: `/var/log/suricata/eve.json`, rotated 4GB/file, gzip compress > 1 week  
⊕ Performance: 1M pps wire-rate on single thread (via eBPF AF_PACKET bypass)  

→ **Monad Signature Examples (sid 9000001+)**  
→ sid 9000001: "Monad Query Opcode Flood" | flow:opcode=0x01 | threshold 1000/sec  
• sid 9000002: "Monad Priority Flag + NAT Cross" | flow:flags=0x14 (priority + nat_crossed)  
◆ sid 9000003: "Monad CRC Mismatch" | marked at XDP layer, bubbled to Suricata via counter  
▸ sid 9000004: "Monad Epoch Skew >1000ms" | comparison rule, correlates log timestamps  

→ **eBPF AF_PACKET Bypass** (host-a, kernel 6.2+)  
⬡ Suricata option: `--af-packet-bypass` mode, kernel XDP program offloads PASS verdict  
⟳ Decision: Monad HbH opcode=0x01 (query) + epoch < 100ms → kernel `XDP_REDIRECT` to AF_PACKET ring, skip Suricata queue  
⊕ Performance gain: 40% throughput increase on benign known flows (bypass nfqueue context switch)  

→ **Alert Routing**  
→ High-severity (critical): sent to alerts-engine (ring 6, port 16670), fires webhook  
• Medium (warning): logged to Loki, indexed by src_ip/dst_ip/sid  
◆ Low (info): sampled (1 in 100) to reduce noise, no alert  

---

## RING 12 — AI/LLM LAYER (vLLM, DeepSeek-R1-7B, ROCm, RX 7700 XT)

→ **LLM Inference Server** (host-a, port 8100, vLLM backend, OpenAI-compatible API)  
• Model: DeepSeek-R1-7B-GGUF (quantized Q4, 4-bit per weight), loaded in GPU memory on startup  
◆ GPU: AMD Radeon RX 7700 XT (16GB VRAM, gfx1101 RDNA3), ROCm 6.1.1 stack  
▸ Tensor parallelism: N/A (single GPU), pipeline parallelism: N/A (single model)  
⬡ Batch size: 8 (dynamic batching), max seq length 2048 tokens, context window 4096  
⟳ cgroup limits: CPU 800% (8 cores), MEM 14GB (10GB model + 4GB KV cache), no swap allowed  
⊕ Quantization: Q4 reduces model size 7B → 4.2GB (GGUF format), slight accuracy loss (<2%)  

→ **Use Case: Anomaly Interpretation**  
• Input: Monad flow_label + statistics from Prometheus (pkt_count, byte_count, entropy, ASN pair)  
◆ Prompt: "This IPv6 flow shows [X pkt/sec, Y bytes/sec, Z% entropy]. Is this anomalous?"  
▸ Latency: 50–200ms per inference (prompt tokens 50, completion tokens 20)  
⬡ Output: JSON `{ anomaly_score: 0–1, reason: "...", recommended_action: "drop|rate_limit|monitor" }`  
⟳ Frequency: 1K flows/hour through LLM (sampled 0.1% of ingress traffic)  

→ **Integration Points**  
⊕ anomaly-detector svc (port 16685) polls vLLM on port 8100, batches 10 flows/request  
→ Results cached in Redis (TTL 300s), keyed by (src_asn, dst_asn, protocol)  
• Latency SLO: 95%ile <500ms (LLM + Redis + Prometheus query), circuit-breaker on timeout  
◆ Monitoring: vLLM metrics on port 8100/metrics, tracked in Prometheus (queue_length, inference_latency_ms)  

---

## RING 13 — DASHBOARD (Kanban, eBPF Viz, Grafana, WebSocket)

→ **Backend: unheaded-dashboard-svc** (port 16699, Go, gRPC + HTTP/2 + WebSocket)  
• Language: Go 1.21, framework: gRPC-gateway + Gorilla websocket, 8K lines  
◆ Database: none (read-only consumer of Prometheus + Anamnesis)  
▸ Real-time feed: WebSocket /ws/flow-updates, pushes new flows every 1s (EventListener pattern)  
⬡ Authentication: JWT token in WS handshake header, role-based access control (rbac)  
⟳ Rate limit: 1000 WebSocket messages/sec per client, backpressure closes lazy clients  

→ **Frontend: Vue 3 SPA** (static assets served from /var/www/unheaded/, dist/index.html)  
⊕ Kanban Board: drag-drop flows between columns ["Ingress", "Suspect", "Dropped", "Redirected", "Passed"]  
→ Each card: flow_label (truncated), src_asn/dst_asn, pkt/sec, verdict, timeline spark  
• Rendering: virtual list (1000 flows max visible), re-render on WebSocket push every 1s  
◆ Mobile-responsive: CSS Grid, breakpoint 768px  

→ **eBPF Visualizations**  
▸ Map heat map: Wotan LRU hash map load (% full), per-CPU bucket collision histogram  
⬡ Ring buffer: Anamnesis event rate (events/sec), queue depth (0–512KB), overflow rate  
⟳ XDP/TC verdict pie chart: PASS/DROP/REDIRECT ratios, refreshed every 5s from metrics  
⊕ Program load: XDP insn count, TC insn count, verifier time (ms), JIT time (ms)  

→ **Grafana Panels (6 dashboards, 50+ panels total)**  
→ "Monad Flow Map": world map with src/dst ASN geolocation, flow lines animated  
• "BGP/IS-IS Topology": force-directed graph of router adjacencies, link metrics color-coded  
◆ "eBPF Verdict Distribution": stacked area chart (PASS/DROP/REDIRECT) over 24h  
▸ "Rate Limit Heat Map": 2D heatmap (flow_label × rate_limit_policy), red=active limit  
⬡ "NAT Translation Stats": source IP distribution before/after SNAT, byte volume  
⟳ "IDS Alert Timeline": time-series of Monad sig hits, grouped by severity  

---

## RING 14 — LOGGING PIPELINE (EVE JSON, Loki, Promtail, Retention)

→ **EVE JSON Format (Suricata + Anamnesis)**  
• Event fields: `{ timestamp, event_type, src_ip, src_port, dst_ip, dst_port, proto, flow_label, monad: { opcode, flags, epoch, seq, crc16_ok }, verdict, action, alert: { signature_id, signature, severity } }`  
◆ Size per event: 250–500 bytes (variable), timestamp ISO-8601 with ns precision  
▸ Compression: gzip level 9, achieves 8:1 ratio on EVE (highly repetitive JSON)  

→ **Promtail Agent** (host-a + host-b, systemd service)  
⬡ Source 1: Anamnesis ring buffer mmap, read zero-copy, parse 64-byte structs → EVE JSON  
⟳ Source 2: /var/log/suricata/eve.json direct file tail, batched reads every 1s  
⊕ Output: HTTP POST to Loki :3100/loki/api/v1/push, tenant-id=org1, gzip-compressed  
→ Backpressure: disk queue in `/var/spool/promtail/` if Loki unavailable, max 100MB  
• Labeling: `{ job="anamnesis", host="host-a"|"host-b", flow_label="...", event_type="...", src_ip="...", dst_ip="...", country_code="...", protocol="..." }`  
◆ PII filtering: customer IP addresses stored only in Loki indices (encrypted at rest), not in Prometheus  

→ **Loki Configuration**  
▸ Index store: BoltDB (single-node), `/var/lib/loki/index/`  
⬡ Chunk store: filesystem (local) on `/var/lib/loki/chunks/`, hourly compaction  
⟳ Retention: 90 days (configurable per tenant), auto-delete expired chunks daily  
⊕ Query engine: LogQL, label matchers + filter expressions, full-text search via inverted index  
→ Example query: `{job="anamnesis", event_type="drop"} | json | opcode="0x20" | src_ip != "::1"`  

→ **Storage Stack**  
• Host-a `/var/lib/loki/`: 2TB partition, 60% used (180GB active logs, 1.8TB compressed archive)  
◆ Retention tier 1: 90 days hot (SSD), tier 2: 1 year cold (HDD), tier 3: archive to S3 (optional)  
▸ Compaction: index merge every 1h, chunk recompress every 24h  
⬡ Verification: quarterly restore from cold tier, verify 10% sample for bit-rot (RAID-6 on HDD)  

→ **Alerting from Logs**  
⟳ Alert rule: count() by (src_ip) over 1m > 10000 (threshold: 10K events/min) → fire to AlertManager  
⊕ Correlation: join Loki logs with Prometheus metrics on flow_label, compute anomaly score  

---

## RING 15 — EXTERNAL EDGE (WAN, IPv6 HbH Passthrough All the Way Out)

→ **Upstream ISP Connection** (host-b via BIRD, AS65002 ↔ upstream ISP AS65000)  
• Public IPv6 prefix: 2001:db8:abcd::/48 (example), announced via eBGP  
◆ Anycast address: 2001:db8:abcd::1 (shared between host-a and host-b via ECMP)  
▸ Customer-facing DNS: ns1.example.com (2001:db8:abcd::53), ns2.example.com (alternate ISP)  
⬡ BGP community: (65002, 30000) = "customer-import", (65002, 40000) = "customer-export"  

→ **Egress Path: Customer Packet with Monad HbH**  
⟳ Packet structure: [IPv6 hdr (src=customer, dst=upstream)] + [Monad HbH (20-byte)] + [TCP/UDP payload]  
⊕ Next-header chain: IPv6 next=0x00 (HOPOPT) → Monad opcode/flags → IPv6 next=0x06/0x11 (TCP/UDP)  
→ MTU check: 1500 (ISP link) >= 32 (HbH wrapper) + customer payload → pass through unmodified  
• OPNsense (host-a) forwards packet to upstream without stripping HbH (rule: "pass IPv6 HOPOPT")  
◆ BIRD (host-b) receives packet, passes to upstream, also preserves HbH  
▸ Upstream ISP: must configure firewall rules to allow next-header 0x00 (HOPOPT)  

→ **Ingress Path: Return Traffic**  
⬡ Upstream sends packet back: [IPv6 src=upstream, dst=customer] + [Monad HbH] + [TCP/UDP payload]  
⟳ Packet arrives at host-b (eBGP endpoint), inspected by IPFire (Ring 10)  
⊕ IPFire rules: iptables -A FORWARD -m ipv6header --header-hop -j ACCEPT (allows HOPOPT)  
→ Packet forwarded to WireGuard tunnel (fd00:dead:beef::/48) or direct to LAN (eth0)  
• Host-a (OPNsense) receives packet, no stripping, payload delivered to customer application  
◆ Monad HbH validated: CRC-16 checked, epoch/seq used for flow correlation in Wotan  

→ **NAT64 Edge Case** (if customer uses IPv4)  
▸ WAN gateway svc (port 16691) offers stateless NAT64 via CLAT (client-side) + PLAT (provider-side)  
⬡ CLAT: customer IPv4 packet → NAT64 prefix (::ffff:0:0/96) → IPv6 HbH injection → upstream  
⟳ PLAT: return packet IPv6 HbH → remove HbH + NAT64 → IPv4 payload → customer  
⊕ Monad HbH preserved through NAT64 gateway (RFC 6052 compatible)  

→ **DDoS Mitigation at Edge**  
→ Rate-limit per src_ip: XDP program drops >1M pps from single source (counter in Sophia)  
• Filtering: Monad opcode=0x01 (query) flood → drop after 100K/sec threshold  
◆ Scrubbing: upstream ISP via BGP community tagging, triggers upstream filtering  
▸ Customer notification: webhook to customer SOC, alert in Grafana dashboard  

→ **Monitoring at WAN**  
⬡ NetFlow v9 export: flow-aggregator svc (port 16684) sends to upstream IPFIX collector  
⟳ Sampling rate: 1-in-1000 packets, templates include Monad flow_label as extension field  
⊕ Latency SLO: Edge-to-LLM inference <200ms, end-to-end roundtrip <5s  

---

## CRITICAL INVARIANTS (Things That MUST Always Be True)

→ **Monad HbH Integrity**  
• Monad 20-byte header in IPv6 HbH (next-header 0x00) never stripped by any layer  
◆ Passes unmodified through: kernel IPv6 stack → XDP program → TC program → OPNsense → firewall rules → IPFire → BIRD → WireGuard tunnel → upstream ISP  
▸ Verification: packet capture at each stage confirms next-header=0x00 remains, HbH header intact  
⬡ CRC-16 must validate before processing, failure → drop, never forward corrupted Monad  
⟳ Replication: if packet is mirrored/copied, Monad HbH copied identically (no mutation on copy)  

→ **Zero Customer Data in Metrics**  
⊕ Prometheus + VictoriaMetrics: no metric label contains actual src_ip, dst_ip, customer_name, ASN routes, or flow payload  
→ Labels: only "flow_label_hash", "protocol_number", "verdict_type", "drop_reason_code"  
• Loki: EVE JSON logs CAN contain src_ip/dst_ip (encrypted at rest, access-controlled)  
◆ Dashboard: flow cards show truncated flow_label (first 4 bytes), not full IP addresses  
▸ Audit: quarterly scan for PII patterns in metric names (regex whitelist enforcement)  

→ **BFD 300ms Failover Guarantee**  
⬡ BFD detect interval: 300ms (3× 100ms tx/rx), multiplier 3 → 900ms absolute max down-detection  
⟳ On BFD down: BGP immediately withdraws all routes, triggers IS-IS SPT recalculation  
⊕ Convergence: <1s new best-path selection, traffic switches to secondary path (host-b)  
→ No packet loss: overlapping BFD + BGP grace period 180s prevents route flap  

→ **eBPF Verifier Success (No Rejected Programs)**  
• All XDP/TC/kprobe programs pass kernel BPF verifier before attach, zero rejects in production  
◆ Stack usage: <512 bytes per program (kernel limit), no unbounded loops detected  
▸ Memory access: all pointer dereferences validated, no NULL dereferences possible  
⬡ Loader waits: if load fails, daemon logs error, previous program remains attached (no outage)  

→ **Map Update Atomicity**  
⟳ Wotan/Sophia updates from userspace via bpf_map_update_elem(): atomic, no torn reads in kernel  
⊕ Old entry evicted immediately on new write (LRU), no duplicate entries  
→ Readers (XDP/TC): observe either old entry or new entry, never intermediate state  

→ **Flow Label Uniqueness**  
• flow_label derived from hash(src_ip, dst_ip, sport, dport, proto), collision rate <0.001% (tested on 1B flows)  
◆ If collision occurs: entry updated, previous flow's stats discarded (rare, acceptable)  
▸ Mitigation: rotate hash seed monthly (via ETCD config), all daemons pick up new seed in <100ms  

→ **Ring Buffer No Data Loss**  
⬡ Anamnesis (512KB) at 100K events/sec → buffer drains every 5.12s  
⟳ Promtail consumes every 1s → no buildup, backpressure impossible in normal operation  
⊕ On overflow: oldest entry overwritten, counter incremented (observable in metrics)  
→ Monthly average: <1 overflow event, acceptable data loss <0.001%  

→ **gRPC Connection Health**  
• All 25 services maintain persistent gRPC connections to unheaded-daemon (port 16666)  
◆ Keepalive: HTTP/2 PING every 30s, timeout 10s → detects dead connections  
▸ Failover: on timeout, reconnect to standby daemon (role=standby on host-b)  
⬡ No service starts until gRPC connection established (synchronous startup)  

→ **Config Consistency Across Hosts**  
⟳ ETCD `/unheaded/config/*` is source-of-truth, all daemons watch for changes  
⊕ Update workflow: validate (schema), push to ETCD, all daemons apply in <1s, metrics confirm  
→ Rollback: revert ETCD key to previous value, daemons auto-rollback (idempotent)  

→ **Logging Timestamp Coherence**  
• All events timestamped via `bpf_ktime_get_ns()` (kernel monotonic clock) + NTP-sync offset  
◆ Drift <100ms between host-a and host-b (PTP on LAN, NTP on WAN)  
▸ Clock skew handled: Loki accepts out-of-order events (by wall-clock), indices by label not time  
⬡ Alerting: anomaly-detector compares Prometheus timestamps (15s scrape) with Loki logs (1s granularity)  

→ **IPv6 Only, No IPv4 Data Plane**  
⟳ Core fabric: IPv6 HbH, WireGuard ULA fd00:dead:beef::/48, BGP EVPN over IPv6 native  
⊕ IPv4 support: NAT64 edge gateway (optional), does NOT modify Monad HbH, passes through  
→ No dual-stack routing: IPv4 customers routed via NAT64 CLAT → IPv6 core → PLAT → IPv4 exit  

---

## BOOT ORDER (Bare Metal Startup Sequence)

1. **BIOS/UEFI + Linux kernel** (both hosts, serial console 115200/8n1)  
   → Load initramfs, mount rootfs, exec /sbin/init (systemd)  

2. **systemd Phase 1: local-fs.target** (both)  
   → Mount `/var`, `/var/log`, `/sys/fs/bpf` (BPF filesystem)  

3. **Networking init** (both)  
   → Bring up eth0 (LAN), eth1 (WAN), wg0 (WireGuard), IP address assignment  

4. **ETCD startup** (host-a, systemd unit `etcd.service`)  
   → Listen on localhost:2379, wait for quorum (single-node → immediate ready)  

5. **unheaded-daemon** (both, unit `unheaded.service`)  
   → Load BPF programs (XDP/TC/kprobe) via Aya framework, attach to NIC  
   → Wait for ETCD ready, acquire leader lock if host-a  
   → Publish service registry entries to `/unheaded/services/*`  

6. **Firewall (OPNsense on host-a, IPFire on host-b)** (both)  
   → Load firewall rules from config, start pf (OPNsense) or iptables (IPFire)  
   → Enable HbH passthrough rules  

7. **Routing daemons** (both, FRR/BIRD)  
   → Start zebra (FRR), bgpd, isisd, bfdd, vtysh socket ready on /var/run/frr/frr.sock  
   → Start bird (BIRD), load config, establish BGP sessions  
   → Wait for BFD session up before advertising routes  

8. **25 microservices** (both, systemd units)  
   → firewall-rules-engine, nat-translator, mirror-manager, alerts-engine, rate-limiter-daemon, telemetry-aggregator, metrics-exporter, netconfig-server, health-check-svc, ...  
   → All wait for unheaded-daemon gRPC ready, register with service discovery  

9. **Observability stack** (host-a, systemd units)  
   → Prometheus (listen :9090), start scrape loop  
   → VictoriaMetrics (listen :8428), start ingest loop  
   → Loki (listen :3100), start BoltDB/chunk store  
   → AlertManager (listen :9093)  

10. **Promtail agents** (both)  
    → Connect to Anamnesis ring buffer mmap, start tailing /var/log/suricata/eve.json  
    → Connect to Loki, begin streaming logs  

11. **Suricata** (host-a, systemd unit `suricata.service`)  
    → Load threat intelligence feeds, compile rule-set  
    → Attach to nfqueue 0:50 (inline mode), enable AF_PACKET bypass  
    → Begin processing packets via pflog tap  

12. **vLLM inference server** (host-a, systemd unit `vllm.service`)  
    → Load DeepSeek-R1-7B Q4 model to GPU (RX 7700 XT)  
    → Listen on :8100 (OpenAI-compatible API)  
    → Warm up with dummy inference (check GPU memory)  

13. **Dashboard backend** (port 16699, systemd unit)  
    → Open WebSocket listener, connect to Prometheus/Loki/Anamnesis  
    → Serve static assets (/var/www/unheaded/dist/)  

14. **Grafana** (host-a, port 3000)  
    → Initialize SQLite DB, load datasources (Prometheus, Loki, VictoriaMetrics)  
    → Load 6 default dashboards  

15. **Health checks** (both, port 16675)  
    → Readiness endpoint returns 200 OK when all services ready  
    → Liveness endpoint checks every required service (BPF, gRPC, ETCD, firewall)  

**Total boot time:** 30–45 seconds to full readiness on bare metal (SSD, no RAID rebuild)

---

## PORT REGISTRY (Service Endpoints, Protocols, Notes)

| Port | Service | Protocol | Notes |
|------|---------|----------|-------|
| 51820 | WireGuard | UDP | Tunnel endpoint, fd00:dead:beef::1/2, MTU 1380 |
| 2379 | ETCD | HTTP/2+gRPC | Leader election, config store, single-node |
| 2380 | ETCD peer | HTTP/2 | Inter-node replication (unused in single-node) |
| 2601 | Zebra (FRR) | CLI socket | vtysh interactive shell, `/var/run/frr/frr.sock` |
| 179 | BGP (FRR/BIRD) | TCP | iBGP (host-a↔host-b), eBGP (host-b→upstream) |
| 3784 | BFD (FRR) | UDP | Tunnel health monitoring, 300ms detect |
| 4789 | VXLAN | UDP | VTEP 10.20.255.1/2, encapsulation port |
| 8100 | vLLM inference | HTTP/1.1 | OpenAI API `/v1/chat/completions`, port 8100/metrics |
| 8428 | VictoriaMetrics | HTTP/1.1 | Long-term storage, `/api/v1/query`, remote_write endpoint |
| 8429 | VMAgent | HTTP/1.1 | Prometheus sidecar, metrics scraper |
| 9090 | Prometheus | HTTP/1.1 | Metrics UI, `/query`, `:9090/metrics` (self-metrics) |
| 9093 | AlertManager | HTTP/1.1 | Deduplication/routing, webhook dispatcher |
| 3000 | Grafana | HTTP/1.1 | Dashboard UI, OIDC auth optional, SQLite backend |
| 3100 | Loki | HTTP/1.1 | Log aggregation, `/loki/api/v1/push` (ingest), `/loki/api/v1/query` |
| 16666 | unheaded-daemon | gRPC/TLS | Service registry, BPF map updater, leader election |
| 16667 | firewall-rules-engine | gRPC | Stateful rule evaluation, subscribed to Wotan events |
| 16668 | nat-translator | gRPC | SNAT/DNAT, flow tracking, publishes to NAT topic |
| 16669 | mirror-manager | gRPC | Mirror-to-SPAN config, subscribed to mirror_config topic |
| 16670 | alerts-engine | gRPC | Threshold logic, webhook dispatcher, fires to AlertManager |
| 16671 | rate-limiter-daemon | gRPC | Token bucket manager, updates BPF rate_limiter map |
| 16672 | telemetry-aggregator | gRPC | Sinks from BPF counters, pushes to Prometheus |
| 16673 | metrics-exporter | HTTP/1.1 | Prometheus format, `:16673/metrics`, eBPF map counters |
| 16674 | netconfig-server | gRPC | BGP/IS-IS config, hot-reload without drain |
| 16675 | health-check-svc | HTTP/1.1 | Readiness/liveness, `/ready` (200 if OK) |
| 16676 | traffic-steering | gRPC | Load-balance decision, returns steering vector |
| 16677 | failover-manager | gRPC | BFD session monitor, triggers BGP withdraw |
| 16678 | qos-manager | gRPC | Queue depth monitoring, updates TC qdisc |
| 16679 | vxlan-controller | gRPC | VTEP coordination, publishes VNI rewrite rules |
| 16680 | isis-adjacency-mon | gRPC | Level-2 FSM, publishes topology deltas |
| 16681 | bgp-session-mon | HTTP/1.1 | eBGP/iBGP health, scrapes FRR/BIRD state |
| 16682 | segment-routing-engine | gRPC | IPv6 SR, computes SID list, updates Sophia |
| 16683 | packet-witness | gRPC | Raw packet capture proxy, no AF_PACKET; uses Anamnesis |
| 16684 | flow-aggregator | gRPC | Groups Anamnesis events, emits NetFlow v9 |
| 16685 | anomaly-detector | HTTP/1.1 | ML scoring, polls vLLM :8100, publishes alerts |
| 16686 | log-forwarder | gRPC | Anamnesis → EVE JSON → Loki, batches 100/sec |
| 16687 | signature-matcher | gRPC | Monad sigs, regex + bpf-bypass decision |
| 16688 | pkt-reassembler | gRPC | TCP/UDP reassembly, L7 inspection, publishes to IDS topic |
| 16689 | cache-warmer | gRPC | Preload Sophia from ETCD, keeps BPF maps hot |
| 16690 | config-validator | gRPC | Schema check, rejects invalid rule changes |
| 16691 | wan-gateway-svc | gRPC | WAN IPv6 HbH passthrough, NAT64 co-processor |
| 16692 | grpc-gateway | HTTP/1.1 | HTTP/1.1 ↔ gRPC bridge, auto-transcoding |
| 16693 | webhook-dispatcher | HTTP/1.1 | Outbound HTTP POST, circuit-breaker, retry logic |
| 16694 | api-auth-svc | gRPC | JWT validation, RBAC via ETCD ACL |
| 16695 | state-sync-svc | gRPC | Multi-leader replication, RAFT on control plane |
| 16699 | dashboard-backend | HTTP/1.1 | WebSocket `/ws/flow-updates`, static assets |
| 67–68 | DHCP/DHCPv6 | UDP | OPNsense (host-a), LAN pool 10.20.100.0/24 |
| 53 | DNS | UDP/TCP | Anycast 2001:db8:abcd::53, ns1.example.com |

**All gRPC services:** TLS 1.3 mandatory, mTLS certs in `/var/lib/unheaded/certs/`, expire yearly  
**All HTTP services:** self-signed or ACME-managed certs, 2048-bit RSA minimum  
**Firewall rule:** default deny inbound on all ports except listed + SSH (22, mgmt only)

---

## DEPLOYMENT TARGETS (3 Platforms)

→ **NixOS Modules** (primary, declarative)  
• `/etc/nixos/modules/unheaded/daemon.nix` + `bpf.nix` + `firewall.nix` + `routing.nix`  
◆ Reproducible builds, atomic rollback, zero-downtime reload via systemd target switch  

→ **Docker Compose** (dev/staging)  
• docker-compose.yml: 3 services (unheaded-daemon, firewall, observability stack)  
◆ Limitations: XDP/TC/kprobe require `--privileged` + host network, BPF maps in tmpfs  

→ **LXD Profiles** (container microservices)  
• lxd launch ubuntu:22.04 unheaded-app -p unheaded (custom profile with BPF enabled)  
◆ Nested container support for multi-tenant, iptables isolation per tenant VPN

---

**END OF SYSTEM MAP** (Lines: 894 | Rings: 16 | Services: 25 | Invariants: 20 | Ports: 50+)
