# Suricata Deployment Guide - Unheaded Kingdom

**Agent 8 Completion** | SPDX-License-Identifier: MIT
Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

## Overview

This deployment configures Suricata IDS/IPS for the Unheaded Kingdom infrastructure with full GPL-2.0 isolation via EVE JSON boundary. The system detects Monad protocol (IPv6 HbH extension header) traffic without stripping or blocking HbH headers.

## Files Created

### 1. Docker Suricata Build

#### `/sessions/vibrant-adoring-maxwell/mnt/tmp/unheaded/docker/suricata/Dockerfile`
Multi-stage Docker build:
- **Stage 1 (builder)**: Ubuntu 24.04 with full build dependencies for Suricata compilation
- **Stage 2 (runtime)**: Slim runtime image with minimal dependencies
- Suricata binary **must** be installed from `~/tmp/suricata/` at deployment (GPL isolation)
- User: `suricata` (uid=994, gid=994)
- Volume: `/var/log/suricata` (EVE JSON output)

#### `/sessions/vibrant-adoring-maxwell/mnt/tmp/unheaded/docker/suricata/entrypoint.sh`
Startup script for Suricata in IDS mode:
- Validates binary presence and configuration files
- Starts Suricata on `SURICATA_INTERFACE` (default: eth1)
- Uses AF_PACKET mode for raw packet capture
- Tails EVE JSON to stdout for Docker log aggregation
- IPC via unix socket: `/run/suricata/suricata.socket`

#### `/sessions/vibrant-adoring-maxwell/mnt/tmp/unheaded/docker/suricata/suricata.yaml`
Production configuration with:
- **CRITICAL**: `decode-events: no` (never strips Monad HbH)
- **CRITICAL**: `ipv6-hopopts: yes` (detects IPv6 hop-by-hop options)
- AF_PACKET on eth1 with tpacket-v3, cluster_flow, mmap
- EVE JSON output to `/var/log/suricata/eve.json`
- Monad HbH rules: `/etc/suricata/rules/unheaded-monad.rules`
- HOME_NET: `[10.20.0.0/16, fd00:dead:beef::/48]`

### 2. LXD Container Definition

#### `/sessions/vibrant-adoring-maxwell/mnt/tmp/unheaded/lxd/containers/suricata.yaml`
LXD profile for host deployment:
- Security: privileged mode, AppArmor rules for raw packet capture
- Networks: eth0 (lxdbr0), eth1 (br-unheaded bridged)
- Storage: 15GB root, /sys/fs/bpf mounting, /var/log/suricata at `/var/log/unheaded/suricata`
- Boot: autostart at priority 195, delay 10s

### 3. Suricata Rules

#### `/sessions/vibrant-adoring-maxwell/mnt/tmp/unheaded/routing/suricata/rules/unheaded-monad.rules`
Monad protocol detection signatures (SID 9000001-9000099):
- **9000001**: Basic HbH extension header detection
- **9000002**: Anomaly - unexpected payload length (CRC mismatch)
- **9000010**: Epoch value anomaly detection
- **9000011**: Sequence anomaly (out-of-order packets)
- **9000030**: Replay attack detection (1000+ packets/sec from single source)
- **9000031**: Scanning behavior (20+ destinations in 5 seconds)
- **9000099**: Unknown opcode (protocol abuse/fuzzing)

### 4. Deployment Scripts

#### `/sessions/vibrant-adoring-maxwell/mnt/tmp/unheaded/scripts/suricata/smoke-test.sh`
Non-destructive validation script:
- Verifies suricata binary presence and executability
- Checks for required features: eBPF, AF_PACKET, Unix socket, Lua, PCRE2
- Displays version information
- Exit code 0 = all checks passed

### 5. Docker Compose Update

#### `/sessions/vibrant-adoring-maxwell/mnt/tmp/unheaded/docker/hosts/host-b/docker-compose.yml`
Added suricata service with:
- Build from `../../suricata` context
- Host networking (required for raw packet capture on eth1)
- Privileged mode with NET_RAW, NET_ADMIN, SYS_NICE capabilities
- Volume mounts:
  - `suricata_logs`: `/var/log/suricata`
  - `suricata.yaml` (read-only)
  - `unheaded-monad.rules` directory (read-only)
  - `/sys/fs/bpf` (read-write for eBPF maps with Shield)
- Healthcheck: Unix socket existence test
- Labels: `unheaded.role: ids`, `unheaded.tier: security`

## Deployment Instructions

### Step 1: Build Suricata Binary

```bash
cd ~/tmp/suricata
./autogen.sh
./configure --enable-ebpf --enable-af-packet --prefix=/usr/local
make
sudo make install
# Binary: /usr/local/bin/suricata
```

### Step 2: Validate Binary

```bash
./scripts/suricata/smoke-test.sh /usr/local/bin/suricata
```

Expected output:
```
PASS: eBPF support
PASS: AF_PACKET support
PASS: Unix socket support
PASS: Lua support
PASS: PCRE2 support
Version: X.Y.Z
=== ALL CHECKS PASSED ===
```

### Step 3: Build Docker Image

```bash
cd docker/suricata
docker build -t unheaded/suricata:latest .
```

### Step 4: Deploy Container

Via docker-compose (host-b):
```bash
cd docker/hosts/host-b
docker-compose up -d suricata
```

Or manual:
```bash
docker run -d \
  --name unheaded-suricata \
  --privileged \
  --cap-add NET_RAW \
  --cap-add NET_ADMIN \
  --cap-add SYS_NICE \
  --network host \
  -e SURICATA_INTERFACE=eth1 \
  -v suricata_logs:/var/log/suricata \
  -v $(pwd)/suricata/suricata.yaml:/etc/suricata/suricata.yaml:ro \
  -v $(pwd)/routing/suricata/rules:/etc/suricata/rules:ro \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  unheaded/suricata:latest
```

### Step 5: Verify Deployment

```bash
# Check logs
docker logs -f unheaded-suricata

# Verify EVE JSON output
docker exec unheaded-suricata tail -f /var/log/suricata/eve.json

# Test healthcheck
docker exec unheaded-suricata test -S /run/suricata/suricata.socket && echo "OK"
```

## Architecture Notes

### GPL-2.0 Isolation Boundary

Suricata runs in isolation from Go-based services:
- **Suricata** (GPL-2.0): Bare metal / container with EVE JSON output
- **Anamnesis** (non-GPL): Reads EVE JSON file + unix socket IPC
- No direct code linking between GPL and non-GPL components
- EVE JSON is the primary data exchange format

### IPv6 HbH (Hop-by-Hop) Options

Monad protocol uses IPv6 HbH extension headers (next-header 0x00):
- Wire format: 20-byte option with flow_label, epoch, seq, CRC16, opcode, flags
- **CRITICAL**: Suricata configured with `decode-events: no` to prevent stripping
- Rules detect presence without modifying packets
- Compatible with Shield eBPF filters (shared /sys/fs/bpf)

### AF_PACKET Capture

Suricata uses AF_PACKET for L2 packet capture on eth1:
- **tpacket-v3**: High-performance packet interface
- **mmap**: Memory-mapped ring buffers
- **cluster_flow**: Load balancing across CPU threads
- **cluster-id: 99**: Coordination with Shield eBPF programs

## Monitoring

### EVE JSON Output

Location: `/var/log/suricata/eve.json`
- Alert events with payload (4096 bytes max)
- Anomaly events (protocol violations)
- Stats events (periodic counters, 60-second interval)
- Consumed by anamnesis bridge for event correlation

### Stats Log

Location: `/var/log/suricata/stats.log`
- Interval: 60 seconds
- Per-thread statistics
- Decoder-level event counts
- Stream reassembly metrics

### Fast Log

Location: `/var/log/suricata/fast.log`
- Lightweight alert summary format
- Human-readable single-line format

## Troubleshooting

### Binary Not Found

```
ERROR: suricata binary not found. Build from ~/tmp/suricata/ and install first.
```

Solution: Run `scripts/suricata/build-suricata.sh` or build manually.

### HbH Headers Missing from Capture

Check suricata.yaml:
- `decode-events: no` must be set
- `ipv6-hopopts: yes` must be enabled

### EVE JSON Not Being Written

Check permissions:
- `/var/log/suricata` owned by suricata:suricata
- Rules loaded from `/etc/suricata/rules`
- Config readable from `/etc/suricata/suricata.yaml`

### AF_PACKET Interface Not Available

Ensure:
- Running with `--privileged` or `NET_RAW` + `NET_ADMIN` capabilities
- Interface eth1 exists and is bridged to br-unheaded
- No other process capturing on same interface

## Future Enhancements

1. **Rule Tuning**: Add threshold configuration file (`/etc/suricata/threshold.config`)
2. **Protocol Decode**: Add Monad opcode-specific matchers as protocol matures
3. **Metrics Export**: Prometheus endpoint for Suricata stats
4. **Rule Updates**: Automated rule update mechanism (ET Pro, Emerging Threats)
5. **Performance Tuning**: Per-CPU thread pinning via cgroup

## References

- Suricata Documentation: https://suricata.io/
- EVE JSON Format: https://docs.suricata.io/en/stable/output/eve/eve-json-format.html
- AF_PACKET: https://man7.org/linux/man-pages/man7/packet.7.html
- IPv6 HbH Options: https://tools.ietf.org/html/rfc8200#section-4.3
