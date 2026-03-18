# Battle Plan: S61-S65 — The Age 2 Bare Metal Awakening
## Convened: 2026-02-25 | Reason: Five-Session Sprint for Bare Metal Production Deployment
## Kingdom State: Age 1 Complete (S60 Gate Passed), Entering Age 2 — Bare Metal Production Era
## Goal: Deploy Unheaded on two physical hosts, verify telemetry stack, eBPF programs, WireGuard bridge, LLM inference, full end-to-end integration

---

## The Origin Myth — Epoch 6 Unfolds

**February 25, 2026** — The Alpha Gate stands open. The castle proved itself in silicon. Sixty sessions to this moment. 10K+ LOC added in S51-S60. All services singing in harmony on mock hardware.

Now: **The kingdom must leave the laboratory.**

Five sessions. Two physical hosts. One bare metal deployment. One 24-hour ordeal that separates toy from tool.

**Age 2 — The Bare Metal Trials** — separates software that "works in CI" from software that "works in production." The bare metal court demands:

1. **First Light** (S61) — NixOS on both hosts, telemetry stack live, kernel eBPF enabled
2. **Forge Ignites** (S62) — Full Unheaded stack compiled and deployed on high-powered host, eBPF programs loaded and verified
3. **Outpost Rises** (S63) — Minimal stack on low-powered host, WireGuard bridge established, cross-host telemetry
4. **Sophia's Eye** (S64) — LLM inference on GPU, cgroup isolation, combined load test, no service starvation
5. **First Kingdom Demo** (S65) — Alpha demo video, 24-hour endurance test, all verdicts recorded, handoff to S66+

**The campaign name: The First Kingdom Lives.**

---

## LEGEND

```
[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint
```

**Commit Cadence**: Every 4-5 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts
**Exit Gate Convention**: Each session ends with a gate. If the gate fails, escalate to Round Table

---

## ROUND TABLE — ALL 9 SEATS REPORT

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: The Alpha Gate is open. Unheaded is no longer a prototype — it's a thesis. Age 2 proves the thesis works on hardware not owned by the engineer.

**North Star for Age 2**: Ship a complete, recorded, reproducible demonstration of Unheaded running on bare metal: (1) real eBPF tracing real packets, (2) live metrics dashboard showing both hosts, (3) LLM inference running with resource isolation, (4) 24-hour uptime validation, (5) video proof.

**Key Decision**: All five sessions in Age 2 are sequential hard dependencies. Each session ships a gate. The campaign succeeds only if S65 completes with all gates passed.

**Risk Management**: 
- Hardware may not arrive on schedule (mitigation: mock telemetry in S61-S62 if needed)
- eBPF verifier may reject production programs (mitigation: validated Doom programs as fallback)
- Network isolation may be required for security testing (mitigation: USB Ethernet adapters as backup)

### The Ledger Records (Micromanager — Execution & QA)

**Campaign Status**: S60 GATE PASSED. Entering Age 2.

**Priority Stack (5 Sessions)**:

1. **P0 — S61: First Light on Bare Metal** — NixOS on both hosts, telemetry stack (Prometheus, VictoriaMetrics, Loki, Grafana), kernel eBPF verification, baseline metrics capture
2. **P0 — S62: The Forge Ignites** — Full Unheaded source build on Host-A (high-powered), all 25 services deployed, XDP programs loaded, H1/H2/H6/H8 verdicts confirmed
3. **P0 — S63: The Outpost Rises** — Minimal Unheaded on Host-B (low-powered), WireGuard bridge (fd00:dead:beef::/48), cross-host telemetry, H4/H7 verdicts confirmed
4. **P0 — S64: Sophia's Eye + Inference Engine** — vLLM + ROCm on Host-A GPU, cgroup v2 isolation, H5 verdict, 30-minute combined load test, no service starved
5. **P0 — S65: The First Kingdom Demo** — Alpha demo video, 24-hour endurance test, all H1-H8 verdicts final, executed notebooks committed, Age 2 marked LIVE

**Acceptance Criteria (Campaign-Level)**: 
- Both hosts running NixOS with Unheaded services
- All eBPF programs verified and loaded successfully
- Cross-host telemetry flowing through unified Grafana dashboard
- LLM inference running with measurable performance
- 24-hour endurance test passed with <0.1% error rate
- Demo video proof recorded and archived

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Hardware Specification**:

**Host-A (High-Powered)**:
- CPU: 16+ cores (Ryzen 7 7700X or equivalent)
- RAM: 64 GB DDR5
- GPU: AMD RX 7700 XT 12GB VRAM
- Storage: 512GB NVMe (SSD)
- Network: 10GbE or 2.5GbE (Ethernet)
- Role: Primary compute, LLM inference, full Unheaded stack

**Host-B (Low-Powered)**:
- CPU: 8 cores (consumer grade, Ryzen 5 or Intel i5)
- RAM: 8 GB DDR4
- GPU: None
- Storage: 256GB SSD
- Network: 1GbE (Ethernet)
- Role: Secondary edge node, minimal Unheaded stack, WireGuard endpoint

**Service Deployment Strategy**:
- S61: NixOS + telemetry infrastructure only
- S62: Daemon + core services + eBPF programs on Host-A (25 services total)
- S63: Minimal daemon + metrics exporters on Host-B (8 services total)
- S64: Add vLLM + sophia-eye to Host-A with cgroup isolation
- S65: Full integration test + demo + 24h endurance

---

## UNIFIED BATTLE PLAN — 5 SESSIONS, 205+ STEPS

---

# S61 — FIRST LIGHT ON BARE METAL
**Objective**: Deploy NixOS on both hosts, establish telemetry infrastructure, verify kernel eBPF support, capture baseline metrics
**Duration**: 36 hours | **Steps**: 41

## PHASE 61.1 — Host-A NixOS Installation (16+ cores, 64GB RAM, RX 7700 XT)

**61.1** [B][V] Boot Host-A into live Linux, run lshw to verify 16+ cores, 64GB RAM, RX 7700 XT present
- Verify: Output shows correct CPU/RAM/GPU

**61.2** [B] Download NixOS 24.11 minimal ISO, verify checksum, write to USB stick
- Verify: USB stick ready for boot

**61.3** [S][V] Boot Host-A from USB, enter BIOS, set boot order to USB, reboot to NixOS live environment
- Verify: Root shell at Host-A, hostname "nixos"

**61.4** [B] Configure ethernet: `sudo ip link set dev eth0 up && sudo dhclient eth0`, ping 8.8.8.8
- Verify: `ping` responds from Host-A

**61.5** [S][B] Partition /dev/nvme0n1: create ESP (512M) and root (remainder) partitions via parted
- Verify: `parted /dev/nvme0n1 print` shows 2 partitions

**61.6** [S][B] Format: `mkfs.fat -F 32 /dev/nvme0n1p1`, `mkfs.ext4 -F /dev/nvme0n1p2`, mount to /mnt
- Verify: `df /mnt` shows available space

**61.7** [B] Generate NixOS config: `sudo nixos-generate-config --root /mnt`
- Verify: Files at /mnt/etc/nixos/configuration.nix and hardware-configuration.nix

**61.8** [W] Write custom /mnt/etc/nixos/configuration.nix with: systemd-boot, kernel 6.x, eBPF modules, SSH enabled, dev tools
- Verify: Syntax valid with `nix flake show`

**61.9** [S][B] Run `sudo nixos-install --root /mnt`, confirm, reboot to Host-A NixOS boot
- Verify: Host-A boots to login prompt after ~10 minutes

**61.10** [B][V] Verify: `uname -a` shows kernel 6.x, `bpftool version` works
- Verify: Kernel 6.x confirmed, bpftool available

## PHASE 61.2 — Host-B NixOS Installation (8 cores, 8GB RAM)

**61.11** [B][V] Boot Host-B, run lshw, verify 8 cores and 8GB RAM
- Verify: Specs match expected Host-B configuration

**61.12** [B] Use same NixOS 24.11 ISO, prepare USB stick for Host-B
- Verify: USB stick ready

**61.13** [S][V] Boot Host-B from USB, NixOS live shell
- Verify: Root shell at Host-B

**61.14** [B] Configure ethernet: `sudo ip link set dev eth0 up && sudo dhclient eth0`
- Verify: Host-B has internet (ping 8.8.8.8)

**61.15** [S][B] Partition /dev/sda (256GB): create ESP (512M) + root, verify with parted
- Verify: Two partitions visible

**61.16** [S][B] Format /dev/sda1 (FAT32) and /dev/sda2 (ext4), mount to /mnt
- Verify: Partitions mounted and formatted

**61.17** [B] Generate config: `sudo nixos-generate-config --root /mnt`
- Verify: Config files created

**61.18** [W] Write /mnt/etc/nixos/configuration.nix for Host-B (lighter, no GPU): boot, kernel, eBPF, SSH, basic tools
- Verify: Config valid

**61.19** [S][B] Run `sudo nixos-install --root /mnt`, reboot to Host-B NixOS login
- Verify: Host-B boots successfully (~10 min)

**61.20** [B][V] Verify Host-B: `uname -a` kernel 6.x, `hostnamectl` shows host-b-unheaded
- Verify: NixOS 24.11, kernel 6.x

## PHASE 61.3 — Telemetry Stack Deployment (Prometheus, VictoriaMetrics, Loki, Grafana)

**61.21** [B] Enable node_exporter on Host-A: add to /etc/nixos/configuration.nix, `nixos-rebuild switch`, verify listening :9100
- Verify: `curl http://localhost:9100/metrics` returns metrics

**61.22** [B] Enable node_exporter on Host-B: add to configuration.nix, rebuild, verify :9100
- Verify: Host-B node_exporter responds

**61.23** [W][B] Write Prometheus config at /etc/prometheus/prometheus.yml: scrape node-host-a and node-host-b targets
- Verify: `curl http://localhost:9090/api/v1/query?query=up` returns JSON with both hosts

**61.24** [W][B] Deploy VictoriaMetrics on Host-A: add to nixos config, enable service on :8428
- Verify: `curl http://localhost:8428/metrics` returns VM metrics

**61.25** [W][B] Deploy Loki on Host-A: write /etc/loki/loki-config.yml, enable service, verify :3100
- Verify: Loki health endpoint responds

**61.26** [W][B] Deploy Grafana on Host-A: add to configuration.nix, enable on :3000, add Prometheus+VictoriaMetrics+Loki datasources
- Verify: Grafana UI accessible at :3000, datasources listed

**61.27** [V][B] Verify all telemetry: curl health endpoints for Prometheus, VictoriaMetrics, Loki, Grafana (expect 200s)
- Verify: All four services respond healthy

## PHASE 61.4 — eBPF Verification & Baseline Metrics

**61.28** [B][V] Verify kernel eBPF BTF: `uname -r`, `bpftool version`, compile tiny test XDP, load via `bpftool prog load`, verify success
- Verify: eBPF test program loads without verifier error

**61.29** [B][V] On Host-B: verify `uname -r` kernel 6.x, `bpftool version` works
- Verify: Host-B eBPF support confirmed

**61.30** [W] Create /usr/local/bin/capture-baseline.sh: collect uname, lscpu, free, df, ip, node_exporter metrics, save to /var/lib/unheaded/baselines/
- Verify: Script is executable

**61.31** [B] Run capture-baseline.sh on Host-A
- Verify: baseline-YYYY-MM-DD_HH:MM:SS.txt created with system metrics

**61.32** [B] Run capture-baseline.sh on Host-B (via SSH or local)
- Verify: Baseline file created on Host-B

**61.33** [W] Write /root/notebooks/01-system-baseline.py: capture CPU count, memory, kernel, node_exporter metrics to JSON
- Verify: Notebook is executable

**61.34** [B] Run Notebook 01, save baseline JSON to /tmp/baseline-01.json
- Verify: JSON contains system metrics

**61.35** [C][B] Commit baseline files and notebook to notebooks/executed/
- Verify: Git commit successful with message "S61: Baseline metrics captured..."

**61.36** [V][B] Query Prometheus: `curl -s 'http://localhost:9090/api/v1/query?query=up'`, verify both node-host-a and node-host-b targets up=1
- Verify: Both hosts scrape targets visible

**61.37** [B][V] Check WireGuard kernel module: `modinfo wireguard` on both hosts
- Verify: WireGuard module available

**61.38** [W] Create /var/lib/unheaded/HARDWARE_INVENTORY.md: document Host-A specs (16 cores, 64GB, RX 7700 XT), Host-B specs (8 cores, 8GB)
- Verify: File readable and accurate

**61.39** [B][V] SSH test: From Host-A, `ssh root@<HOST-B-IP> hostname` returns host-b-unheaded; bidirectional SSH works
- Verify: SSH connectivity both directions confirmed

**61.40** [W] Create /var/lib/unheaded/S61_SESSION_LOG.txt: document completion of all 11 tasks, gates passed, baseline metrics captured
- Verify: Log file created

**61.41** [V] S61 EXIT GATE — Verify all gates:
- Kernel 6.x on both hosts ✓
- eBPF BTF support verified ✓
- Telemetry stack operational ✓
- Prometheus scrapes both hosts ✓
- Baseline metrics captured ✓
**GATE STATUS: PASS or FAIL to escalate**

---

# S62 — THE FORGE IGNITES
**Objective**: Build all 25 Unheaded services on Host-A, deploy via NixOS, load eBPF XDP programs, verify H1/H2/H6/H8 verdicts
**Duration**: 42 hours | **Steps**: 42

## PHASE 62.1 — Source Build

**62.1** [B] Clone repo: `cd /root && git clone https://github.com/ORG/unheaded.git && cd unheaded && git log --oneline -1`
- Verify: Recent S60 commits visible

**62.2** [W] Update /etc/nixos/configuration.nix: add Go 1.22, Rust, Clang, LLVM, protobuf, pkg-config to systemPackages
- Verify: `nixos-rebuild switch` succeeds

**62.3** [B] Build daemon: `cd /root/unheaded/cmd/daemon && go build -o unheaded-daemon .`
- Verify: Binary exists, ~15-20MB

**62.4** [B] Build Wotan: `cd /root/unheaded/services/wotan && go build -o wotan .`
- Verify: Binary exists

**62.5** [B][P] Build all remaining Go services: loop over services/ dirs, `go build` each
- Verify: All service binaries exist

**62.6** [B] Build eBPF programs: `cd pkg/ebpf/whispering-void && cargo build --target bpfel-unknown-none --release`
- Verify: .bpf.o files in target/bpfel-unknown-none/release/

**62.7** [B][V] Verify eBPF: loop over .bpf.o files, `bpftool prog load` each, expect all to pass verifier
- Verify: All eBPF programs verify without error

**62.8** [B] Build dashboard: `cd web/dashboard && npm install && npm run build`
- Verify: dist/ directory created with webpack bundle

**62.9** [B] Rebuild daemon with embedded dashboard: `cd cmd/daemon && go build -o unheaded-daemon .`
- Verify: Daemon binary includes dashboard assets

## PHASE 62.2 — NixOS Service Deployment

**62.10** [W] Create /etc/nixos/unheaded/daemon.nix: systemd service for unheaded-daemon on :17000
- Verify: File valid Nix syntax

**62.11** [W] Create /etc/nixos/unheaded/wotan.nix: systemd service for Wotan on :18000/:18001
- Verify: File valid

**62.12** [W][P] Create service modules for 20+ services (monad, sophia, timeguru, captain, architect, etc.): each on appropriate port
- Verify: All service.nix files exist in /etc/nixos/unheaded/services/

**62.13** [W] Update /etc/nixos/configuration.nix: add imports for all service modules
- Verify: `nixos-rebuild switch` succeeds, all services start

**62.14** [V][B] Verify services running: `systemctl | grep unheaded`, all show running/active
- Verify: 25 services all show "running"

## PHASE 62.3 — eBPF Program Loading

**62.15** [W] Create /etc/nixos/unheaded/ebpf-loader.nix: systemd service (Type=oneshot) to load XDP programs before daemon
- Verify: File valid

**62.16** [B] Build loader: `cd cmd/ebpf-loader && go build -o ebpf-loader .`
- Verify: Binary exists

**62.17** [B] Load XDP on eth0: `ip link set dev eth0 xdp obj packet_marker.bpf.o sec xdp`
- Verify: `ip link show eth0` shows xdp attachment

**62.18** [V][B] Test XDP: tcpdump on eth0, ping from Host-B, expect 5+ ICMP packets captured
- Verify: Packets seen by XDP program

**62.19** [B][V] Check eBPF maps: `bpftool map list`, see trace_map, flow_map, etc.; dump a map with `bpftool map dump name trace_map`
- Verify: Maps populated with entries

## PHASE 62.4 — Verdict Collection (H1, H2, H6, H8)

**62.20** [W] Write /root/notebooks/02-load-test-verdicts.py: collect H1 (RAM), H2 (eBPF CPU), H6 (disk IOPS), H8 (process count) to JSON
- Verify: Script executable, imports available

**62.21** [B] Run Notebook 02, capture verdicts to /tmp/verdicts-02.json
- Verify: JSON contains H1/H2/H6/H8 measurements

**62.22** [B] Generate synthetic packet stream: start iperf3 or UDP packet generator for 60 seconds
- Verify: Packets flowing on eth0 (tcpdump confirms)

**62.23** [V][B] Query eBPF maps during traffic: `bpftool map dump name trace_map | wc -l`, expect >100 entries
- Verify: Maps populated with live traffic data

**62.24** [V][B] Query Prometheus: `curl -s 'http://localhost:9090/api/v1/query?query=packet_count'`, expect metric data
- Verify: Dashboard shows live trace panel

## PHASE 62.5 — Health & Stress Testing

**62.25** [B][V] Service health: curl http://localhost:17000/health, expect {status: "UP"}
- Verify: All services report healthy

**62.26** [V][B] Port availability: nc -zv on all critical ports (17000, 17001, 18000, 18001, etc.), expect all open
- Verify: All ports accepting connections

**62.27** [B][V] Wotan test: curl http://localhost:18000/health, expect {status: "SERVING"}
- Verify: Wotan operational

**62.28** [B] 30-minute stress test: Python script generates 7500 pps for 1800s, monitor CPU/memory
- Verify: Test runs full duration without crash

**62.29** [V][B] Check service logs: `journalctl -u unheaded-daemon.service --since="30 minutes ago" | grep -i error | wc -l`, expect <5
- Verify: No critical errors during stress

**62.30** [B] Re-run Notebook 02 after stress: capture final verdicts
- Verify: Final verdicts in /tmp/verdicts-02-final.json

**62.31** [V][B] H1 check: grep "used_gb" verdicts-02-final.json, expect <48.0
- Verify: H1 CONFIRMED or escalate

**62.32** [V][B] H2 check: compare baseline CPU vs stressed CPU, expect <5% overhead
- Verify: H2 CONFIRMED or escalate

**62.33** [V][B] H6 check: run fio write test, measure p95 latency, expect <50ms
- Verify: H6 CONFIRMED or escalate

**62.34** [V][B] H8 check: `ps aux | wc -l`, expect <500
- Verify: H8 CONFIRMED or escalate

**62.35** [W] Create /var/lib/unheaded/VERDICTS_S62.txt: summarize H1/H2/H6/H8 results
- Verify: File readable, all verdicts documented

**62.36** [C] Commit S62 work: add binaries, verdicts, service modules
- Verify: Commit with message "S62: Full Unheaded stack deployed..."

**62.37-62.42** [Reserved for additional testing, hot-reload, failover, emergency procedures, final health check, session log, exit gate]

---

# S63 — THE OUTPOST RISES
**Objective**: Deploy minimal Unheaded on Host-B, establish WireGuard bridge, verify H4/H7 verdicts, cross-host telemetry
**Duration**: 40 hours | **Steps**: 40

[Similar structure to S62, focusing on:
- Minimal service set on Host-B (8 services: daemon, wotan, monad, trace-collector, trace-metrics, node_exporter, prometheus scrape, exporter)
- WireGuard configuration: Host-A wg0 fd00:dead:beef::1/64, Host-B wg0 fd00:dead:beef::2/64
- H4 verdict: WireGuard RTT <5ms via ping over tunnel
- H7 verdict: WireGuard CPU overhead <2% via perf/ss
- Cross-host Prometheus scrape verification: Host-A scrapes Host-B exporter via wg0
- Unified Grafana dashboard showing both hosts
- Final H4/H7 verdicts committed]

---

# S64 — SOPHIA'S EYE + THE INFERENCE ENGINE
**Objective**: Deploy vLLM with DeepSeek-R1-7B, cgroup v2 isolation, measure H5 verdict, run 30-minute combined load test
**Duration**: 42 hours | **Steps**: 42

[Structure:
- Install ROCm 6.x on Host-A for RX 7700 XT
- Deploy vLLM service on :20100, load DeepSeek-R1-7B Q4_K_M quantized model
- Deploy sophia-eye service (semantic search) on :20105
- Configure cgroup v2: vLLM gets CPUQuota=800% (8 cores), MemoryMax=14GB
- H5 verdict: vLLM throughput >50 tokens/sec at P95 latency <500ms, VRAM <11GB
- Combined 30-min load: Unheaded APIs + WireGuard traffic + LLM inference simultaneously
- Verify Unheaded service P99 latency regression <20% with LLM running
- Final H5 verdict committed]

---

# S65 — THE FIRST KINGDOM DEMO
**Objective**: Record alpha demo video, run 24-hour endurance test, finalize all H1-H8 verdicts, handoff to S66+
**Duration**: 38 hours | **Steps**: 38

[Final session:
- Execute docs/demo/ALPHA_DEMO_SCRIPT.md: demo video recording (10-15 min) of:
  - Host-A and Host-B running Unheaded
  - Grafana dashboard showing both hosts, eBPF traces, WireGuard metrics, vLLM throughput
  - Live query to LLM model (sophia-eye semantic search demo)
  - Wotan message bus activity
- Run 24-hour endurance test per BARE_METAL_QA_PLAN.md Phase 5:
  - All services continuously running
  - Synthetic load: 5K pps, LLM inference 10 req/min
  - Monitor for errors, crashes, resource leaks
  - Log all metrics to time series DB
- Final H1-H8 verdict collection from all notebooks (01-05)
- Commit executed notebooks with real hardware data
- Update wiki/Home.md: Age 2 status = LIVE
- Update references/timeline.md: Age 2 complete dates
- Prepare S66+ roadmap handoff document]

---

## APPENDIX A: EMERGENCY PROCEDURES

### eBPF verifier rejects program
```bash
sudo bpftool prog load program.o /sys/fs/bpf/test type xdp 2>&1 | tee /tmp/verifier.log
# Check for unbounded loops, stack overflow, uninitialized map access
# Reduce complexity, split into two programs if needed
# Fallback: use Doom-validated eBPF suite
```

### Service won't start
```bash
journalctl -u unheaded-daemon.service -n 50
ss -tlnp | grep 17000
ls -la /root/unheaded/cmd/daemon/unheaded-daemon
ping 8.8.8.8
cd /root/unheaded/cmd/daemon && go build -o unheaded-daemon .
```

### WireGuard tunnel not passing traffic
```bash
ping <HOST-B-IP>
ip link show wg0
bridge fdb show dev wg0
sudo iptables -L -n | grep 51820
sudo tcpdump -i eth0 port 51820
```

### vLLM OOM on RX 7700 XT
```bash
rocm-smi --showmeminfo vram
vllm serve --max-model-len 2048  # Reduce context
# Use quantized model: GPTQ or AWQ 4-bit
# Fallback: llama.cpp with partial offload
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Session | Primary Agent | Phases | Steps | Hours | Dependencies |
|---------|--------------|--------|-------|-------|--------------|
| S61 First Light | Architect + Developer | 4 | 41 | 36 | Hardware available |
| S62 Forge Ignites | Developer + Architect | 5 | 42 | 42 | S61 Gate, Host-A up |
| S63 Outpost Rises | Architect + Developer | 4 | 40 | 40 | S62 Gate, Host-B up |
| S64 Sophia's Eye | Developer + Architect | 4 | 42 | 42 | S63 Gate, GPU available |
| S65 First Kingdom | Captain + Developer + Librarian | 4 | 40 | 38 | S64 Gate |
| **TOTAL** | | **21** | **205+** | **198** | |

**Critical Path**: S61 → S62 → S63 → S64 → S65 (all sequential)

**Fallback**: If hardware delayed, use mock data in S61-S62, re-execute S63-S65 when hardware available

---

## APPENDIX C: PORT REGISTRY (Age 2)

```
16666  doom-bridge         18000  wotan HTTP          20000  dashboard
16667  doom-go-injector    18001  wotan gRPC          20100  vllm-deepseek (S64)
16670  trace-collector     19000  timeguru            20105  sophia-eye (S64)
16671  trace-metrics       19001  architect           20102  qdrant
17000  daemon HTTP         19002  captain             9090   prometheus (S61)
17001  daemon gRPC         19003  micromanager        8428   victoria-metrics (S61)
17010  cli-server          19004  monad               3100   loki (S61)
17020  metrics-exporter    19005  sophia              3000   grafana (S61)
21000  gateway HTTP                                   9100   node-exporter
```

---

## APPENDIX D: VERDICTS REFERENCE

| Verdict | Metric | Target | Tool | Status |
|---------|--------|--------|------|--------|
| H1 | RAM utilization | <48GB | /proc/meminfo | S62 |
| H2 | eBPF CPU overhead | <5% | perf, bpftool | S62 |
| H4 | WireGuard RTT | <5ms | ping | S63 |
| H5 | vLLM throughput | >50 tok/sec | vllm/metrics | S64 |
| H6 | Disk latency p95 | <50ms | fio, iostat | S62 |
| H7 | WireGuard overhead | <2% CPU | perf, ss | S63 |
| H8 | Process count | <500 | ps aux | S62 |
| H9 | 24h availability | 99.9% uptime | systemd logs | S65 |

---

*Battle Plan S61-S65 forged 2026-02-25*
*5 sessions. 2 hosts. 205+ steps. One gate at a time.*
*Age 2 begins.*

