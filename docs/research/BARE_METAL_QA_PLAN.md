# Bare Metal QA Plan — Unheaded Age 2

## 1. Baseline Protocol

Before any tuning, establish baselines. Every optimization must be measured against these numbers.

### 1.1 System State Capture (run before any test)

Exact commands to snapshot system state:

```bash
# CPU topology
lscpu
nproc --all
cat /proc/cpuinfo | grep "model name" | uniq

# Memory
free -h
cat /proc/meminfo

# Disk
lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT,ROTA,SCHED
cat /sys/block/*/queue/scheduler

# Network interfaces
ip link show
ethtool <iface> | grep -E "Speed|Duplex|Link"
cat /proc/net/dev

# Kernel
uname -r
sysctl -a 2>/dev/null | grep -E "net.core|net.ipv4|vm\."

# Process state
ps aux --sort=-%mem | head -20
systemctl list-units --state=running
```

### 1.2 Baseline Metrics (record before any service starts)

- **Idle CPU**: should be <2% system, <1% user
- **Idle RAM**: record exact free/available/cached split
- **Disk idle IOPS**: baseline with `iostat -x 1 10`
- **Network idle**: baseline with `sar -n DEV 1 5`

---

## 2. Hypothesis Matrix

List exactly 8 falsifiable hypotheses. For each:
- **H[n]**: The hypothesis (specific, falsifiable)
- **Prediction**: What the measurement will show IF true (quantitative)
- **Test**: Exact commands/tools
- **Pass/Fail**: Binary gate

### Hypotheses

**H1: Full Unheaded stack (25 services) fits within 8GB RAM on Host-B with headroom**
- Prediction: Total RSS across all services ≤ 6GB (leaving 2GB headroom for kernel, buffers)
- Test: `ps aux | awk '{sum+=$6} END {print "Total RSS: " sum/1024 " MB"}'` after all services stable
- Pass: RSS ≤ 6144 MB
- Fail: RSS > 6144 MB

**H2: eBPF XDP programs load and pass kernel verifier on kernel 6.x within 5 seconds**
- Prediction: Each program loads in <5s, all verifier checks pass, 0 EPERM errors
- Test: `time ip link set dev <iface> xdp obj <prog.o> sec .text` on both hosts
- Pass: Load time <5s, exit code 0, zero EPERM/SIGKILL
- Fail: Load time ≥5s or verifier rejection

**H3: Monad register round-trip (inject → XDP process → BPF map read → userspace) < 50µs at 10k pps**
- Prediction: P99 latency ≤50µs, sustained over 60s at 10k pps
- Test: Send 10k tagged packets, measure BPF ring buffer timestamps vs userspace receive
- Pass: P99 ≤50µs, P95 ≤30µs, 0 drops
- Fail: P99 >50µs or drop rate >0.1%

**H4: Wotan message bus (gRPC + NATS or direct) delivers events between hosts < 5ms over WireGuard**
- Prediction: Cross-host RPC calls RTT ≤5ms P99, sustained load of 1000 msg/s
- Test: Send request from Host-B to Host-A service, measure one-way latency from Prometheus histogram
- Pass: P99 RTT ≤5ms at 1000 msg/s
- Fail: P99 RTT >5ms or timeouts >0.5%

**H5: vLLM + ROCm (RX 7700 XT) achieves > 20 tok/s on DeepSeek-R1-7B without starving other services**
- Prediction: Sustained tok/s ≥20, CPU on other cores remains <30%, mem pressure steady
- Test: Run vLLM inference for 60s at batch_size=4, measure from `/metrics` endpoint
- Pass: tok/s ≥20, non-GPU CPU <30%, mem growth <50MB
- Fail: tok/s <20 or CPU spikes >50%

**H6: Disk IOPS for Anamnesis event log does not exceed 80% of device capacity under load**
- Prediction: Peak IOPS during Phase 3 ≤80% of `iostat -x` max capacity
- Test: Run Phase 3, capture `iostat -x 1 60` in parallel, measure peak %util
- Pass: Peak disk %util ≤80%
- Fail: Peak disk %util >80%

**H7: WireGuard tunnel adds < 1ms RTT overhead between Host-A and Host-B control plane**
- Prediction: ping over wg0 RTT is <1ms more than direct network RTT (measured at baseline)
- Test: `ping -c 100 <Host-B-wg-ip>` and compare vs direct IP, measure delta
- Pass: WireGuard RTT delta <1ms
- Fail: WireGuard RTT delta ≥1ms

**H8: Process count stays < 500 total across all namespaces under full load**
- Prediction: `pgrep -c . | head -1` after Phase 4 <500 processes
- Test: Record process count before Phase 1, after each phase, peak during Phase 3-4
- Pass: Peak process count <500
- Fail: Process count ≥500

---

## 3. Measurement Stack

### 3.1 Instruments (what we use for each metric)

| Metric | Tool | Collection Interval | Retention |
|--------|------|-------------------|-----------|
| CPU (per-core) | `perf stat`, `mpstat` | 1s | 90 days |
| RAM (per-process) | `/proc/[pid]/status`, `smaps` | 5s | 90 days |
| Disk IOPS | `iostat -x` | 1s | 90 days |
| Net (per-iface) | `sar -n DEV`, `ethtool -S` | 1s | 90 days |
| Process count | `pgrep -c .`, `/proc/*/status` | 5s | 90 days |
| eBPF latency | `bpf_ktime_get_ns()` in XDP prog | per-packet | ring buffer |
| Service latency | Prometheus histograms | per-request | 90 days |
| GPU utilization | `rocm-smi` | 1s | 90 days |
| WireGuard RTT | `ping`, `wg show` | 10s | 90 days |

### 3.2 Prometheus Exporters Required

- **node_exporter**: CPU, RAM, disk, net, process count (metrics port: 9100)
- **dcgm_exporter** or **rocm_exporter**: GPU metrics (port: 9400)
- **custom eBPF exporter**: XDP latency from BPF ring buffer (port: 9401)
- **vLLM native metrics**: `/metrics` endpoint on inference port
- **All Unheaded services**: expose `/metrics` on their standard port +1

### 3.3 Storage

- **Prometheus scrape interval**: 5s
- **VictoriaMetrics**: for long-term storage (1 year retention, compressed ~20:1)
- **Loki**: for structured log aggregation (zerolog → Loki via promtail)

---

## 4. Load Test Protocol

### 4.1 Phases (run in order, do not skip)

#### Phase 0 — Idle baseline (5 minutes, no services)

Record all baseline metrics from Section 1.2. This is your control.

**Checklist:**
- [ ] System idle for 5 min before capture
- [ ] All metrics logged with timestamp
- [ ] Record in lab notebook with date/host
- [ ] Commit baseline snapshot to git

#### Phase 1 — Bring up services one at a time

Start each service, wait 30s, record delta in RAM/CPU/disk/net.
This gives you exact cost of each service.

**Service startup order:**
1. etcd (control plane)
2. consul (service discovery)
3. nats (message bus)
4. prometheus (metrics collection)
5. loki (log aggregation)
6. grafana (visualization)
7. monad-core (eBPF orchestrator)
8. monad-agent (on Host-B)
9. wotan-gateway (API gateway)
10. wotan-router (message router)
11. anamnesis (event log)
12. memoria (cache layer)
13. syllogism (inference control)
14. vllm-runtime (on Host-A only)
15. rocm-supervisor (on Host-A only)
16. +9 supporting services (specify in deployment manifest)

**For each service:**
- [ ] Record: `free -h`, `ps aux --sort=-%mem | grep <service>`, disk usage
- [ ] Note: startup latency, any errors in logs
- [ ] Verify: service responds to health check, exposes metrics

#### Phase 2 — Synthetic packet load

Use `iperf3` (TCP/UDP) or custom Go packet sender. Ramp from 100 → 1K → 10K → 100K pps.
At each level: run for 60s, record P50/P95/P99 latency, drop rate, CPU per-core.

**Packet generator script outline:**
```bash
#!/bin/bash
for pps in 100 1000 10000 100000; do
  echo "Testing at ${pps} pps"
  timeout 60 iperf3 -c <Host-B-IP> -u -b ${pps}Kbit -l 1472 -J > /tmp/iperf_${pps}.json
  iostat -x 1 60 > /tmp/iostat_${pps}.txt &
  mpstat 1 60 > /tmp/mpstat_${pps}.txt &
  wait
done
```

**Record for each pps level:**
- [ ] Min/Max/Mean/P50/P95/P99 latency
- [ ] Drop rate (%)
- [ ] CPU per-core (%)
- [ ] Memory delta
- [ ] Disk IOPS

#### Phase 3 — LLM inference under network load

While running Phase 2 at 10K pps, start vLLM inference at 50% capacity.
Record: tok/s degradation, P99 latency regression, GPU memory, CPU contention.

**Inference load:**
```bash
# On Host-A, in parallel with Phase 2 at 10K pps
for i in {1..5}; do
  curl -s -X POST http://localhost:8000/v1/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"deepseek-r1-7b","prompt":"","max_tokens":100,"temperature":0.7}' &
done
wait
```

**Record:**
- [ ] vLLM tok/s (from /metrics, compare to H5 prediction)
- [ ] P99 inference latency
- [ ] GPU memory usage (MB)
- [ ] CPU contention: % on non-GPU cores
- [ ] Network impact: % latency increase vs Phase 2 alone

#### Phase 4 — Fault injection

Kill individual services. Measure recovery time.
Kill WireGuard. Measure control plane failover time.
OOM one process. Measure OOM killer behavior and recovery.

**Fault sequence:**
1. Kill monad-agent on Host-B: measure recovery time from systemd restart
2. Kill NATS: measure Wotan message bus failover (should queue/retry)
3. Kill eBPF program: measure reload time and packet drop
4. `ip link del wg0`: measure control plane failover time, recovery after restart
5. Trigger OOM on Host-B: `echo 3 > /proc/sys/vm/drop_caches` + `stress-ng --vm 1 --vm-bytes 90%`
   - Measure which process gets OOM killed
   - Measure systemd recovery

**Record for each fault:**
- [ ] Time to detect failure
- [ ] Recovery time
- [ ] Data loss (if any)
- [ ] Impact on other services

#### Phase 5 — Endurance (24 hours)

Run at 70% max load for 24 hours. Record:
- Memory leak rate (RSS growth over time)
- File descriptor leak rate (`lsof | wc -l` delta)
- Goroutine count (Go pprof `/debug/pprof/goroutine`)
- Disk fill rate
- Error rate in logs

**Load profile for 24h endurance:**
```bash
# Phase 2 sustained at 7K pps
iperf3 -c <Host-B-IP> -u -b 7000Kbit -l 1472 -t 86400 -J > /tmp/iperf_24h.json

# vLLM inference at 35% capacity (3 concurrent requests)
while true; do
  for i in {1..3}; do
    curl -s -X POST http://localhost:8000/v1/completions ... &
  done
  sleep 5
done
```

**Hourly checkpoint (automated):**
```bash
#!/bin/bash
for hour in {1..24}; do
  sleep 3600
  echo "=== Hour $hour ===" >> /tmp/endurance.log
  free -h >> /tmp/endurance.log
  ps aux --sort=-%mem | head -10 >> /tmp/endurance.log
  lsof | wc -l >> /tmp/endurance.log
  curl -s http://localhost:6060/debug/pprof/goroutine | wc -l >> /tmp/endurance.log
done
```

**Pass criteria:**
- [ ] RSS growth <50MB/hour (total <1.2GB over 24h)
- [ ] FD count stable (delta <10)
- [ ] Goroutine count stable (delta <50)
- [ ] Disk growth <50GB
- [ ] Error rate in logs <0.01%
- [ ] Zero OOM events (`dmesg | grep -c "Out of memory"`)

---

## 5. Tuning Protocol (only after baselines established)

### 5.1 Tuning is forbidden until:

- [ ] All Phase 0-2 measurements are recorded
- [ ] All H1-H8 hypotheses have a result (confirmed/falsified)
- [ ] Bottleneck is identified via flame graph (not guessed)

Run before tuning:
```bash
# CPU flame graph during Phase 2 at 10K pps
perf record -F 99 -g -p <pid> -- sleep 60
perf script | stackcollapse-perf.pl | flamegraph.pl > cpu.svg

# Memory profile during Phase 3
curl -s http://localhost:6060/debug/pprof/heap > heap.pb.gz
go tool pprof -http=:8080 heap.pb.gz
```

### 5.2 Tuning targets (ordered by expected impact)

1. **Kernel network stack**: `net.core.rmem_max`, `net.core.wmem_max`, `net.core.netdev_max_backlog`
2. **CPU scheduler**: `isolcpus=` for XDP/eBPF cores, `rps_cpus` for NIC queues
3. **Memory**: `vm.swappiness=1`, huge pages for vLLM, `mlock` for BPF maps
4. **Disk**: I/O scheduler (none for NVMe, mq-deadline for HDD), `readahead`
5. **Go runtime**: `GOGC`, `GOMEMLIMIT`, `GOMAXPROCS`

### 5.3 Each tuning change requires:

1. **Before measurement** (5 min stable): Run Phase 2 at 10K pps, capture metrics
2. **Apply ONE change**: Example: `sysctl -w net.core.rmem_max=134217728`
3. **After measurement** (5 min stable): Run identical Phase 2, capture metrics
4. **Commit to git** with exact change:
   ```bash
   git add tuning.log
   git commit -m "Tune: increase net.core.rmem_max to 128MB, P99 latency: X -> Y µs"
   ```
5. **Decision**: 
   - If P99 improves: keep
   - If P99 regresses by >5%: revert immediately
   - If P99 regresses <5%: accept if other metric improves (throughput, drop rate)

**Revert template:**
```bash
sysctl -w net.core.rmem_max=<previous_value>
git revert HEAD
```

---

## 6. Security Regression Gates

Each Phase must pass these before proceeding to next Phase:

- [ ] `gosec` report unchanged (no new HIGH/CRITICAL)
  ```bash
  gosec -no-fail ./... > gosec_phase_N.json
  diff gosec_phase_0.json gosec_phase_N.json  # should be empty
  ```

- [ ] Auth middleware still enforcing (curl without token → 401)
  ```bash
  curl -s -X GET http://localhost:8080/api/v1/services | grep -q 401
  ```

- [ ] Rate limiter still functioning (flood → 429)
  ```bash
  for i in {1..1000}; do curl -s -X GET http://localhost:8080/api/v1/health; done \
    | grep -c 429 | grep -q "[0-9]\+"  # should be >0
  ```

- [ ] eBPF verifier accepting all programs (no SIGKILL/EPERM)
  ```bash
  dmesg | tail -100 | grep -c "EPERM\|SIGKILL"  # should be 0
  ```

- [ ] No unexpected listening ports
  ```bash
  ss -tlnp > listening_phase_N.txt
  diff <(cat listening_phase_0.txt | awk '{print $4}' | sort) \
       <(cat listening_phase_N.txt | awk '{print $4}' | sort)  # should be empty
  ```

---

## 7. Pass/Fail Criteria for Age 2 Ship

| Gate | Threshold | Measured by |
|------|-----------|------------|
| Stack RSS < 12GB | Host-A | `/proc/meminfo` after Phase 1 |
| eBPF load time < 10s | Both hosts | `time ip link set dev X xdp obj` |
| Monad RTT P99 < 100µs | Both hosts | BPF `ktime_get_ns` timestamps (H3) |
| vLLM throughput > 15 tok/s | Host-A | vLLM `/metrics` histogram (H5) |
| WireGuard RTT < 5ms | Cross-host | `ping` P99 over `wg0` (H7) |
| 24h endurance: 0 OOM events | Both hosts | `dmesg grep "Out of memory"` |
| 24h endurance: 0 goroutine leak | Both hosts | pprof `/debug/pprof/goroutine` delta |
| All H1-H8 confirmed or documented | Both hosts | Lab notebook + git logs |

**All gates must pass before Code Freeze.**

---

## 8. Lab Notebook Template

Record all results in a single git-tracked file per phase:

```
# QA Run #1 — Date: 2026-02-25 — Host-A: 64c/256GB/NVMe, Host-B: 16c/64GB/SSD

## Phase 0 — Idle Baseline
- CPU idle: 1.2% sys, 0.3% user
- RAM free: 248GB / 256GB (12GB cached)
- Disk idle IOPS: 0
- Network idle: 0 pps

## Phase 1 — Service startup
- Service | Startup time | RSS increase | CPU peak
- etcd | 2.1s | 45MB | 15%
- consul | 1.8s | 32MB | 8%
...
- Total RSS after all services: 5.2GB (✓ H1 passes)

## Phase 2 — Packet load (10K pps)
- P50 latency: 120µs
- P95 latency: 240µs
- P99 latency: 340µs
- Drop rate: 0.002%
- CPU peak: 45%
- (✓ H3 P99 < 50µs fails, but measured for baseline)

## Phase 3 — LLM + network
- vLLM tok/s: 22.5 (✓ H5 passes)
- CPU non-GPU: 28% (✓ passes <30%)
- Memory delta: 320MB
- Network latency regression: +85µs P99

## Phase 4 — Fault injection
- Monad-agent restart: 4.2s recovery
- NATS restart: 3.8s, zero message loss
- WireGuard restart: 2.1s, failover transparent
- OOM event: systemd killed memory consumer, auto-restart in 2s

## Phase 5 — 24h endurance
- RSS growth rate: 12MB/24h (✓ passes <50GB)
- FD count delta: +3 (stable)
- Goroutine delta: +1 (stable)
- Error rate: 0.0001% (zero errors in app, only net drops)
- Disk growth: 2.3GB (logs)
- OOM events: 0 ✓

## Summary
- H1: ✓ Confirmed (5.2GB < 6GB)
- H2: ✓ Confirmed (eBPF load 4.1s < 10s)
- H3: ✗ Falsified (P99 340µs > 50µs — investigate XDP prog inefficiency)
- H4: ✓ Confirmed (WireGuard RTT 3.2ms < 5ms)
- H5: ✓ Confirmed (22.5 tok/s > 15 tok/s)
- H6: ✓ Confirmed (disk %util 62% < 80%)
- H7: ✓ Confirmed (WireGuard delta 0.8ms < 1ms)
- H8: ✓ Confirmed (process count 487 < 500)

## Next steps
- H3 falsified: profile XDP program, optimize verifier-friendly code
- Schedule tuning run after H3 fix
```

---

## 9. Git Commit Standards

Every measurement, tuning, and fix must be committed.

**Baseline commit:**
```bash
git add docs/research/BASELINE_RUN_1.md
git commit -m "QA: Baseline run #1, all Phase 0-2 complete

- Host-A: 64c/256GB/NVMe
- Host-B: 16c/64GB/SSD
- All H1-H8 hypotheses measured
- Phase 0 idle baseline recorded
- Phase 1 service startup costs logged
- Phase 2 packet load 10K pps tested

See lab notebook for detailed metrics."
```

**Tuning commit:**
```bash
git add sysctl.conf tuning.log
git commit -m "Tune: net.core.rmem_max 128MB → P99 latency 340µs → 310µs

Before: P99 340µs at 10K pps
After:  P99 310µs at 10K pps
Impact: +0.9% throughput, -8.8% latency
Status: ✓ keep (improvement within +5% regression threshold)"
```

**Regression revert:**
```bash
git add sysctl.conf
git commit -m "Revert: vm.swappiness tuning (P99 latency +12%, exceeds +5% threshold)

Before: P99 310µs
After:  P99 347µs (+12% regression)
Decision: Revert to baseline, investigate alternative tuning"
```

---

## 10. Failure Mode Response

If a hypothesis fails or metric regresses:

1. **Do not skip to next phase**
2. **Root cause analysis** (flame graph + dmesg + logs)
3. **Fix or document** (if fix required, commit, re-test)
4. **If unfixable before ship**: document trade-off, escalate to review board

Example:
```
H3 Failed: Monad RTT P99 = 340µs (target <50µs)

Root cause: eBPF program not verifier-optimized, extra bounds checks
            inserted by kernel, excessive stack usage.

Fix: Rewrite XDP register injection to avoid bounds check unrolling.
     Test on Phase 2 again (commit: "Fix H3 false failure, unroll bounds checks").
     New result: P99 = 48µs ✓

Ship decision: ✓ H3 confirmed (after fix)
```

---

## Summary Checklist Before Ship

- [ ] All 8 hypotheses H1-H8 pass or documented with root cause
- [ ] Phase 0-5 complete on both hosts
- [ ] All security regression gates pass
- [ ] 24h endurance test: 0 OOM, 0 goroutine leaks, RSS drift <50MB/h
- [ ] All tuning commits auditable in git
- [ ] Lab notebook complete with quantitative results
- [ ] Flame graphs, heap profiles, and performance artifacts archived
- [ ] Code review: all tuning changes reviewed and approved
- [ ] Prometheus + Grafana dashboards configured for production monitoring
- [ ] Post-ship runbook documented (how to roll back, how to monitor)

**Do not ship without all checkboxes green.**

