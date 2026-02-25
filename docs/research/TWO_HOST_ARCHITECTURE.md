# Two-Host Unheaded Architecture

## 1. Host Inventory

### Host-A (The Forge)

**Role:** Primary. Full stack. LLM inference. eBPF XDP.

**Services (25 + LLM stack):**
- protocol-api (16666)
- dashboard-backend (16667)
- kanban-app (16668)
- wotan (16680, message bus/gRPC)
- gateway (16681)
- trace-collector (16672)
- node-exporter (9100)
- prometheus (9090)
- loki (3100)
- vllm-deepseek (20100, OpenAI-compatible)
- sophia-eye (20105, semantic search)
- rocm-smi-exporter (9445)
- (+ 14 additional core Unheaded services)

**Port range:** 16666-26666 (Doom Range)

**Kingdom IPv6:**
- Prefix: fd00:dead:beef::/48
- Host-A subnet: fd00:dead:beef:a::/64
- Service addresses: fd00:dead:beef:a::16666 through ::26666

**WireGuard:**
- Endpoint: <HOST_A_PHYSICAL_IP>:51820
- WireGuard address: fd00:dead:beef:wg::a/64
- MTU: 1420

**GPU:**
- AMD Radeon RX 7700 XT
- VRAM: 12 GB
- ROCm 6.2.x (HSA driver)
- PyTorch 2.3 (ROCm build)
- vLLM 0.4.x inference engine

**RAM:** ≥16 GB system RAM

**CPU:** ≥16 cores (pinned allocation strategy in §4.1)

---

### Host-B (The Outpost)

**Role:** Secondary. Lightweight Unheaded suite. Control plane mirror.

**Services:**
- protocol-api (16666)
- dashboard-backend (16667)
- kanban-app (16668)
- wotan (16680, message bus/gRPC)
- gateway (16681)
- trace-collector (16672)
- node-exporter (9100)
- prometheus (9090)
- loki (3100)
- (no vLLM, sophia-eye, or GPU services)

**Port range:** 16666-26666 (same)

**Kingdom IPv6:**
- Host-B subnet: fd00:dead:beef:b::/64
- Service addresses: fd00:dead:beef:b::16666 through ::26666

**WireGuard:**
- Endpoint: <HOST_B_PHYSICAL_IP>:51820
- WireGuard address: fd00:dead:beef:wg::b/64
- MTU: 1420

**GPU:** None

**RAM:** ≥8 GB

**CPU:** ≥8 cores

---

## 2. WireGuard Control Plane Bridge

### 2.1 Design Decisions (with reasoning)

#### Why WireGuard (not VXLAN, not IPSec, not Tailscale)

**WireGuard selected for:**
1. **Kernel integration:** In Linux kernel since 5.6+. No external userspace daemons or FUSE layers. Native module load `modprobe wireguard`.
2. **Cryptographic minimalism:** ~400 lines of audited Rust crypto code. ChaCha20-Poly1305 + Curve25519. Minimal attack surface vs IPSec's 20KLOC or VXLAN's encap/decap state.
3. **Handshake latency:** Sub-1 RTT after initial key exchange. Perfect forward secrecy by default. No IKE negotiation overhead.
4. **Transport agnostic:** Works over IPv4 underlay, IPv6 underlay, any IP. Payload is also IPv6. Nested IP agnosticity.
5. **Verifiable:** Code review audits: Trail of Bits (2020), Cure53 (2020). Open-source crypto is prerequisite.

**Why NOT Tailscale:** External coordination server (Tailscale SaaS). CGNAT traversal adds complexity. Not self-hosted. Auth via OAuth. Overkill for intra-datacenter PoC.

**Why NOT Nebula:** Requires additional Go binary dependency. Constellation-based gossip adds latency variance. Not in kernel.

**Why NOT IPSec (IKEv2 + ESP):** Configuration sprawl (IKE daemon, keying material, SAs, SPD). Kernel mode AH/ESP code is older, larger. IPSec is stateful (SAs live in kernel memory). WireGuard is stateless per packet (derive nonce from timestamp).

**Why NOT VXLAN:** Layer 2 encapsulation. Adds 50 bytes overhead (8 byte VXLAN header + IP/UDP). No encryption by default. Would require Wireguard OR IPSec on top.

---

### 2.2 Exact WireGuard Configuration

#### Host-A /etc/wireguard/wg0.conf

```ini
[Interface]
PrivateKey = <HOST_A_PRIVATE_KEY>
Address = fd00:dead:beef:wg::a/64
Address = 10.100.0.1/30
ListenPort = 51820
MTU = 1420

[Peer]
PublicKey = <HOST_B_PUBLIC_KEY>
AllowedIPs = fd00:dead:beef:b::/64
AllowedIPs = 10.100.0.2/32
Endpoint = <HOST_B_PHYSICAL_IP>:51820
PersistentKeepalive = 25
```

**MTU Justification:** 1500 bytes (standard Ethernet) - 80 bytes (IPv6 40 + UDP 8 + WireGuard 32) = 1420. Prevents IP fragmentation on WireGuard tunnel.

**IPv4 backup:** 10.100.0.0/30 for fallback if IPv6 stack is compromised.

**PersistentKeepalive = 25:** Send keepalive every 25 seconds to maintain NAT traversal state (though both hosts are bare metal, this is good practice).

---

#### Host-B /etc/wireguard/wg0.conf

```ini
[Interface]
PrivateKey = <HOST_B_PRIVATE_KEY>
Address = fd00:dead:beef:wg::b/64
Address = 10.100.0.2/30
ListenPort = 51820
MTU = 1420

[Peer]
PublicKey = <HOST_A_PUBLIC_KEY>
AllowedIPs = fd00:dead:beef:a::/64
AllowedIPs = 10.100.0.1/32
Endpoint = <HOST_A_PHYSICAL_IP>:51820
PersistentKeepalive = 25
```

---

#### Key Generation and Enablement

On both hosts:

```bash
# Generate keys
wg genkey | tee /etc/wireguard/private.key | wg pubkey > /etc/wireguard/public.key
chmod 600 /etc/wireguard/private.key

# Retrieve public key
cat /etc/wireguard/public.key

# Bring up interface
wg-quick up wg0

# Verify
wg show
ping -6 fd00:dead:beef:wg::b  # Host-A → Host-B
ping -6 fd00:dead:beef:wg::a  # Host-B → Host-A
```

**Persistence:** 
```bash
systemctl enable wg-quick@wg0
systemctl start wg-quick@wg0
```

---

### 2.3 Control Plane Traffic Over WireGuard

**Only these services cross the wg0 bridge. All others are isolated per host.**

| Service | Source | Dest | Port | Protocol | Freq | Payload |
|---------|--------|------|------|----------|------|---------|
| Wotan (message bus) | Host-A | Host-B | 16680 | gRPC (HTTP/2) | 10+ Hz | distributed state, spans |
| Wotan heartbeat | Host-B | Host-A | 16680 | gRPC | 1 Hz | replica ack |
| trace-collector (Jaeger) | Host-B | Host-A | 16672 | gRPC | 0.5 Hz batched | distributed traces |
| Prometheus scrape | Host-A scrapes | Host-B:9100 | 9100 | HTTP/1.1 | 15s interval | node metrics |
| Loki push | Host-B | Host-A:3100 | 3100 | HTTP POST | 30s batched | log lines |
| Dashboard health | Host-A | Host-B:16667 | 16667 | HTTP/1.1 | 5s interval | /health JSON |
| (future) Model inference | Host-B | Host-A:20100 | 20100 | HTTP (OpenAI) | on-demand | prompt/response |

**Firewall rules (on Host-B):**
```bash
# Allow inbound from Host-A only
ufw allow in on wg0 from fd00:dead:beef:wg::a/64 to fd00:dead:beef:wg::b/64 port 16680,9100,3100 proto tcp
ufw allow in on wg0 from fd00:dead:beef:wg::a/64 to fd00:dead:beef:wg::b/64 port 16680 proto udp
ufw allow in on wg0 from fd00:dead:beef:wg::a/64 to fd00:dead:beef:wg::b/64 port 16667 proto tcp

# Outbound to Host-A
ufw allow out on wg0 to fd00:dead:beef:wg::a/64 port 16672,16680,3100 proto tcp
ufw allow out on wg0 to fd00:dead:beef:wg::a/64 port 16680 proto udp
```

**Bandwidth estimate:** ~2-5 Mbps sustained (trace + metric batches). BW not constraint.

---

### 2.4 Kingdom Mode IPv6 Addressing

**Full addressing plan:**

```
Kingdom /48 prefix:     fd00:dead:beef::/48
├── Host-A LAN:         fd00:dead:beef:a::/64       (4.7e18 addresses)
│   ├── gateway:        fd00:dead:beef:a::1/128
│   ├── services:       fd00:dead:beef:a::16666 .. ::26666
│   └── node-exporter:  fd00:dead:beef:a::9100/128
│
├── Host-B LAN:         fd00:dead:beef:b::/64
│   ├── gateway:        fd00:dead:beef:b::1/128
│   ├── services:       fd00:dead:beef:b::16666 .. ::26666
│   └── node-exporter:  fd00:dead:beef:b::9100/128
│
├── WireGuard tunnel:    fd00:dead:beef:wg::/64
│   ├── Host-A ep:      fd00:dead:beef:wg::a/128
│   └── Host-B ep:      fd00:dead:beef:wg::b/128
│
└── Loopback:           fd00:dead:beef::1/128        (unheaded-daemon virtual)
```

**Why fd00::/8 (Unique Local Addresses):**
1. Deterministic prefix (not SLAAC/DHCP random).
2. Not routed to internet (RFC 4193). No need for IANA registration.
3. Stable across reboots and resets.
4. No collision with ISP-assigned addresses.
5. No privacy concerns (internal deployment).

Alternative (link-local fe80::/10) would be unstable across interface resets and unmanageable at scale.

---

## 3. LLM Stack on Host-A

### 3.1 Component Stack

```
RX 7700 XT (12 GB GDDR6)
  ↓
ROCm 6.2.x (HSA runtime)
  ├── rocm-smi (monitoring)
  ├── rocm-core (kernel modules)
  └── roctracer (trace library)
       ↓
PyTorch 2.3 (ROCm compute backend)
  └── torch.cuda (HIP API abstraction)
       ↓
vLLM 0.4.x (inference engine)
  ├── paged attention optimization
  ├── TRTLLM/VLLM scheduler
  └── KV cache quantization (optional)
       ↓
DeepSeek-R1-Distill-Qwen-7B (quantized Q4_K_M)
  ├── ~3.8B parameters (effective)
  ├── ~4.5 GB VRAM loaded
  └── ~400 ms / token @ batch=1
       ↓
HTTP/JSON API (OpenAI-compatible)
  ├── POST /v1/completions
  ├── POST /v1/chat/completions
  └── port 20100
```

**Model choice:** DeepSeek-R1-Distill-Qwen-7B (distilled reasoning model, 7B params, quantized to Q4_K_M = 4-bit blocks). Full R1 (671B) would be 100+ GB VRAM. Distill trades reasoning speed for fit.

---

### 3.2 Memory Budget (Host-A)

**VRAM (12 GB GPU):**

| Component | Allocation |
|-----------|-----------|
| Model weights (DeepSeek-7B Q4_K_M) | 4.5 GB |
| KV cache (context length 2048, batch size 8) | 1.2 GB |
| Attention buffers + temp allocs | 0.8 GB |
| ROCm runtime (HIP context, malloc pools) | 0.5 GB |
| vLLM internal queues | 0.3 GB |
| **Headroom / safety margin** | **4.7 GB** |
| **Total** | **12.0 GB** |

**System RAM (≥16 GB):**

| Component | Allocation |
|-----------|-----------|
| OS + kernel + drivers | 1.5 GB |
| Unheaded 25 services (25 Go processes ~120MB each) | 3.0 GB |
| vLLM host-side buffers (scheduler, tokenizer, etc) | 2.0 GB |
| Protocol buffers (gRPC), TLS handshakes | 0.5 GB |
| eBPF maps + verifier state | 0.2 GB |
| Prometheus + tsdb | 0.4 GB |
| Anamnesis event log (in-memory ring buffer) | 0.8 GB |
| Linux page cache (kernel recommends) | 1.5 GB |
| **Headroom / swap buffer** | **4.5 GB** |
| **Total** | **16.0 GB** |

**Validation:** Assuming Host-A has 16 GB RAM + 12 GB VRAM (RX 7700 XT spec), memory utilization stays under 85%. No OOM kills expected during nominal load.

---

### 3.3 ROCm Setup

**Prerequisites:**
```bash
# Install ROCm 6.2
sudo apt install -y hip-runtime-amd hip-dev
sudo apt install -y rocm-core rocminfo rocm-smi

# Verify GPU presence
rocm-smi --showproductname
# Output: GPU 0: AMD INSTINCT MI308X or Radeon RX 7700 XT

# List compute capabilities
rocminfo | grep -A 2 "Agent" | grep "Name"

# Test compute
python3 -c "import torch; print(torch.cuda.is_available()); print(torch.cuda.get_device_name(0))"
# Expected: True / AMD Radeon RX 7700 XT
```

**PyTorch + vLLM installation:**
```bash
# PyTorch with ROCm
pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/rocm6.2

# vLLM (bleeding edge, or pin to 0.4.x)
pip install vllm==0.4.2

# Model download (auto, first startup)
# Or pre-cache:
huggingface-cli download deepseek-ai/DeepSeek-R1-Distill-Qwen-7B
```

**Launch vLLM:**
```bash
python3 -m vllm.entrypoints.openai.api_server \
  --model deepseek-ai/DeepSeek-R1-Distill-Qwen-7B \
  --dtype float16 \
  --quantization "awq" \
  --gpu-memory-utilization 0.85 \
  --port 20100 \
  --host 0.0.0.0 \
  --disable-log-requests \
  --max-model-len 2048 \
  --enable-prefix-caching
```

**Flags explained:**
- `--dtype float16`: Half precision. DeepSeek performs fine at FP16. Reduces VRAM by 2x vs FP32.
- `--quantization awq`: Activation-aware quantization (alternate: gptq). Minimal accuracy loss.
- `--gpu-memory-utilization 0.85`: Use up to 85% of VRAM. Leaves headroom for spikes.
- `--max-model-len 2048`: Context window. Prevents OOM on long prompts.
- `--enable-prefix-caching`: Reuse KV for repeated prefixes (repeated system prompts, few-shot examples).

**Monitor GPU:**
```bash
# Watch VRAM, compute utilization, throttling
watch -n 1 'rocm-smi --json | python3 -m json.tool | grep -E "\"(gpu_id|vram|compute|sclk|mclk|gpu_temp|power_usage)\"'

# Or use prometheus exporter
pip install rocm-smi-exporter
python3 -m rocm_smi_exporter --port 9445
# Prometheus scrapes :9445/metrics
```

---

## 4. Resource Isolation Strategy

### 4.1 CPU Pinning (on Host-A)

**Allocation logic:**
```
Total CPUs: 16 (e.g., 2x8-core EPYC)

Isolation groups:
  CPUs 0-3:   eBPF/XDP processing (interrupt-disabled)
  CPUs 4-11:  vLLM inference threads
  CPUs 12-15: Unheaded services + system
```

**NixOS kernel boot parameters:**
```nix
boot.kernelParams = [
  "isolcpus=0-11"           # Exclude CPUs 0-11 from kernel scheduler
  "rcu_nocbs=0-11"          # Disable RCU callbacks on isolated CPUs
  "irqaffinity=12-15"       # Route interrupts to CPUs 12-15 only
  "nohz_full=0-11"          # Disable timer ticks on isolated CPUs
];
```

**Manual verification:**
```bash
# View isolated CPUs
cat /sys/devices/system/cpu/isolated

# Pin eBPF loader to CPU 0
taskset -c 0 /usr/local/bin/load-xdp-program eth0

# Pin vLLM to CPUs 4-11
taskset -c 4-11 python3 -m vllm.entrypoints.openai.api_server ...
```

**Rationale:** eBPF programs run in soft-IRQ context. Isolated CPUs with NohzFull prevent timer interrupts, allowing ~99.95% CPU utilization for XDP ingress packet processing without jitter from kernel housekeeping.

---

### 4.2 Memory Limits (systemd cgroup v2)

**Critical services (protocol-api, wotan):**
```ini
[Service]
MemoryMax=1G
MemoryHigh=900M
OOMPolicy=kill
```

**Dashboard services (dashboard-backend, kanban-app):**
```ini
[Service]
MemoryMax=512M
MemoryHigh=450M
OOMPolicy=kill
```

**vLLM service:**
```ini
[Service]
MemoryMax=10G
MemoryHigh=9G
OOMPolicy=terminate
```

**Benefits:**
1. Service OOM does not trigger host OOM.
2. Kernel OOM killer targets highest memory consumer, not random PID.
3. cgroup v2 accounting is byte-accurate (vs cgroup v1's coarse page-based).

---

### 4.3 Disk I/O (cgroup blkio)

**Device mapping:**
```
/dev/nvme0n1p1: OS root (/)
/dev/nvme0n1p2: vLLM cache (/mnt/vllm-cache)
/dev/nvme1n1:   Anamnesis event log (/mnt/anamnesis-data)
/dev/nvme2n1:   Prometheus TSDB (/mnt/prometheus-data)
```

**I/O weights:**
```bash
# vLLM: weight 500 (high priority, model reads)
echo "259:1 500" > /sys/fs/cgroup/io.weight.device

# Anamnesis: weight 300 (medium, sequential append)
echo "259:2 300" > /sys/fs/cgroup/io.weight.device

# Prometheus: weight 100 (low, batch compactions)
echo "259:3 100" > /sys/fs/cgroup/io.weight.device
```

**Write-ahead log and eBPF maps:**
```bash
# tmpfs (RAM-backed, no disk I/O)
mount -t tmpfs -o size=2G,noatime tmpfs /var/lib/unheaded/bpf-maps
mount -t tmpfs -o size=1G,noatime tmpfs /var/log/unheaded
```

---

## 5. Failure Modes and Recovery

| Failure Scenario | Detection | RTO | Recovery Procedure |
|---|---|---|---|
| **WireGuard tunnel drops** | Ping monitor fails to fd00:dead:beef:wg::b from Host-A | <5s | `systemctl restart wg-quick@wg0` on both hosts; Anamnesis logs event |
| **Host-B network partition** | Host-A loses Prometheus target on 9100; trace-collector timeout | 15s | Control plane operates on Host-A only; Host-B services continue independently; Wotan enters async mode |
| **vLLM process crash** | Exit code logged by systemd; HTTP :20100 unreachable | 2s | Systemd auto-restart (Restart=on-failure); KV cache rebuilt on startup; no state loss (stateless) |
| **vLLM OOM (model + context)** | dmesg: `Out of Memory: Kill process vllm (PID 1234)` | 5s | systemd kills vLLM (cgroup MemoryMax); Anamnesis records OOM event; sophia-eye becomes unresponsive until restart |
| **eBPF program verifier reject** | `bpf: kernel.unprivileged_bpf_disabled` or verifier EPERM | <1s | Fallback to TC (traffic control) filter; log via Anamnesis; alert via Prometheus (custom metric) |
| **GPU hang / compute stall** | vLLM hangs on model.forward(); rocm-smi times out | 30s | Trigger GPU reset: `rocm-smi --gpureset 0`; all vLLM requests timeout and retry; CPU-bound tasks unaffected |
| **Disk full (Anamnesis)** | `df /mnt/anamnesis-data > 85%`; write() ENOSPC | <10s | Anamnesis pruning: delete oldest events until <70% full; alert operator; no service hangs (Anamnesis is async) |
| **Prometheus TSDB corruption** | `prometheus --check-metrics`: error reading chunks | 60s | Delete corrupted blocks in `/mnt/prometheus-data/wal/*`; restart prometheus; metrics gap = block age (typically 2h) |
| **Host-A power loss** | Immediate | 120s | Host-B continues independently (Wotan enters solo mode); on Host-A reboot: WireGuard auto-connects (systemd enable), services restart, vLLM rebuilds cache |
| **Host-B power loss** | Host-A detects tunnel timeout | 25s | All services on Host-A continue; traces/metrics queue locally in Anamnesis; upon Host-B restart: replay queued events to Host-B |

---

## 6. Demo Topology Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                   Host-A (The Forge)                         │
│              fd00:dead:beef:a::/64                           │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  Unheaded Control Plane (Services 1-9)             │    │
│  │                                                     │    │
│  │  protocol-api (:16666)  dashboard-backend (:16667) │    │
│  │  kanban-app (:16668)    gateway (:16681)           │    │
│  │  wotan (:16680, leader) node-exporter (:9100)      │    │
│  │  prometheus (:9090)     loki (:3100)               │    │
│  │  trace-collector (:16672)                          │    │
│  └────────────────┬────────────────────────────────────┘    │
│                   │                                          │
│  ┌────────────────▼────────────────────────────────────┐    │
│  │         LLM Stack (GPUs-only services)             │    │
│  │                                                     │    │
│  │  vLLM (:20100)         sophia-eye (:20105)         │    │
│  │  rocm-smi-exporter (:9445)                         │    │
│  │                                                     │    │
│  │      RX 7700 XT [12GB VRAM]                        │    │
│  │      ├─ DeepSeek-R1-7B-Q4 (4.5 GB)                 │    │
│  │      ├─ KV Cache (1.2 GB)                          │    │
│  │      └─ ROCm runtime + buffers (2.1 GB)            │    │
│  └────────────────┬────────────────────────────────────┘    │
│                   │                                          │
│  ┌────────────────▼────────────────────────────────────┐    │
│  │  eBPF XDP (eth0 ingress)                           │    │
│  │  ├─ Shield (DDoS detection)                        │    │
│  │  ├─ Shim (packet redirection)                      │    │
│  │  └─ CPUs 0-3 isolated, irqaffinity=12-15           │    │
│  └────────────────┬────────────────────────────────────┘    │
│                   │                                          │
│                   │  wg0 (WireGuard tunnel)                 │
│                   │  fd00:dead:beef:wg::/64                 │
│                   │  ChaCha20-Poly1305                      │
│                   │  MTU 1420                               │
│                   │  PersistentKeepalive 25s                │
│                   │                                          │
└───────────────────┼──────────────────────────────────────────┘
                    │
                    │
┌───────────────────┼──────────────────────────────────────────┐
│                   │                                          │
│  ┌────────────────▼────────────────────────────────────┐    │
│  │  eBPF XDP (eth0 ingress)                           │    │
│  │  ├─ Shield (DDoS detection)                        │    │
│  │  ├─ Shim (packet redirection)                      │    │
│  │  └─ CPUs 0-7 isolated                              │    │
│  └────────────────┬────────────────────────────────────┘    │
│                   │                                          │
│  ┌────────────────▼────────────────────────────────────┐    │
│  │  Unheaded Control Plane (Services 1-9)             │    │
│  │                                                     │    │
│  │  protocol-api (:16666)  dashboard-backend (:16667) │    │
│  │  kanban-app (:16668)    gateway (:16681)           │    │
│  │  wotan (:16680, mirror) node-exporter (:9100)      │    │
│  │  prometheus (:9090)     loki (:3100)               │    │
│  │  trace-collector (:16672)                          │    │
│  │                                                     │    │
│  │  (No vLLM, no GPU services)                         │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│            Host-B (The Outpost)                             │
│         fd00:dead:beef:b::/64                               │
│         (8GB RAM, 8 CPU cores)                              │
└──────────────────────────────────────────────────────────────┘
```

---

## 7. Deployment Checklist

### Pre-deployment (Host-A)

- [ ] Verify GPU: `rocm-smi --showproductname`
- [ ] Check VRAM: `rocm-smi --showmemory`
- [ ] Isolate CPUs: verify `/sys/devices/system/cpu/isolated` contains 0-11
- [ ] Create vLLM cache directory: `mkdir -p /mnt/vllm-cache`
- [ ] Pre-download model: `huggingface-cli download deepseek-ai/DeepSeek-R1-Distill-Qwen-7B --cache-dir /mnt/vllm-cache`
- [ ] Create tmpfs mounts: `mount -t tmpfs -o size=2G tmpfs /var/lib/unheaded/bpf-maps`
- [ ] Test vLLM startup (dry run, no systemd yet)
- [ ] Generate WireGuard keys

### Pre-deployment (Host-B)

- [ ] Verify network connectivity to Host-A
- [ ] Create directories: `/mnt/prometheus-data /mnt/anamnesis-data`
- [ ] Generate WireGuard keys (match Host-A's pubkey in config)

### Deployment

- [ ] Start WireGuard on both hosts: `systemctl start wg-quick@wg0`
- [ ] Verify tunnel: `ping -6 fd00:dead:beef:wg::b` (from Host-A)
- [ ] Start services on Host-B first (protocol-api, wotan, etc)
- [ ] Verify Wotan cluster formation: `grpcurl -plaintext localhost:16680 list`
- [ ] Start services on Host-A
- [ ] Start vLLM on Host-A
- [ ] Verify vLLM health: `curl -s http://localhost:20100/health`
- [ ] Verify cross-host traces: curl to Host-B dashboard, check traces from Host-A

### Post-deployment (monitoring)

- [ ] Dashboard: http://fd00:dead:beef:b:b::16667 (external-accessible)
- [ ] Prometheus: http://localhost:9090 (Host-A only, or expose via dashboard)
- [ ] vLLM logs: `journalctl -u vllm.service -f`
- [ ] WireGuard status: `wg show` (bidirectional)
- [ ] GPU utilization: `watch -n 1 rocm-smi` or Prometheus rocm_smi_exporter metrics

---

## 8. Performance Targets (PoC)

| Metric | Target | Acceptance Criteria |
|--------|--------|---|
| Model inference latency (7B, Q4, batch=1) | <500ms | <1s at p99 |
| WireGuard tunnel throughput | >100 Mbps | <150 Mbps measured |
| Trace propagation (Host-B → Host-A) | <50ms | <100ms p99 |
| eBPF XDP packet processing | <100us | <1ms with Shield enabled |
| Wotan consensus (2 nodes) | <10ms | <50ms with network jitter |
| Prometheus scrape interval | 15s | All targets in sync |
| Dashboard response time | <200ms | <500ms p99 |
| Host-A memory utilization | <85% | No OOM, swap inactive |
| Host-A GPU utilization | 50-80% (inference idle time) | Predictable, not pegged |

---

## 9. Rationale Summary

**Two-host split:**
- Host-A: Compute-intensive (LLM, eBPF XDP). Justifies high-spec hardware (RX 7700 XT, 16GB RAM).
- Host-B: Low-spec commodity hardware. Mirrors control plane for HA but offloads inference to Host-A.
- **Trade-off:** Network latency (<1ms on LANs) is acceptable cost for cost savings on Host-B.

**WireGuard as control plane bridge:**
- Kernel module, ~400 LOC, audited crypto.
- No external dependency. Deterministic behavior.
- PFS by default. Sub-1 RTT handshake.

**IPv6 Kingdom Mode:**
- fd00::/8 deterministic. No IANA coordination needed.
- /48 prefix supports 65536 subnets. Overkill for 2 hosts, but scales to 10+ sites.
- Addresses are stable, predictable, readable in logs and dashboards.

**Resource isolation (CPU pinning + cgroup limits):**
- eBPF programs run on isolated, no-tick CPUs. Achieves <100us jitter.
- cgroup v2 memory limits prevent cascading OOM.
- Disk I/O weights prioritize vLLM cache reads over logging writes.

**Model choice (DeepSeek-R1-Distill-7B-Q4):**
- 7B parameters (vs full 671B) fit in 12GB VRAM with headroom.
- Q4_K_M quantization loses <1% accuracy on benchmarks.
- Sufficient reasoning for PoC demos. Fastest distilled reasoning model as of 2025.

