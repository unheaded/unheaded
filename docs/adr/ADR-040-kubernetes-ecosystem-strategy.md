# ADR-040: Kubernetes Ecosystem Strategy — Kingdom Operator

## Status: PLANNED

## Date: 2026-04-05
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)

---

## Context

Several Unheaded components fill gaps in the Kubernetes ecosystem. This ADR
documents what could attract K8s tooling attention and the strategy for
extracting a standalone "Kingdom Operator" Helm chart.

## What Catches K8s Radar

### 1. Wotan — Lightweight Message Bus Alternative

K8s operators fight with NATS/Kafka/RabbitMQ complexity. Wotan's triple-role
(ring buffer + event bus + protocol RAM) with topic auto-approval and gRPC-first
transport is simpler than anything in the CNCF landscape. Combined with Akira's
consensus health system (66.67% threshold), it's a real differentiator for
in-cluster service mesh messaging.

### 2. Monad Wire Format — eBPF-Native Observability

Frozen v0x01, 20 bytes in IPv6 HbH. eBPF-native observability from packet zero —
no sidecar proxies, no Envoy, no Istio overhead. The K8s observability space is
drowning in sidecar complexity. A protocol that embeds telemetry in the wire
format itself, traced by eBPF XDP programs, would turn heads at KubeCon.

### 3. Zhenai — AI Operator/Controller Pattern

A local AI that executes runbooks, monitors health via consensus, and
auto-remediates. This is what every K8s platform team wants but builds poorly
with bash scripts and PagerDuty. The trust level system (L1 read-only → L2
write with approval → L3 autonomous) maps directly to K8s RBAC. Package as
a Helm chart with MCP server and runbook engine.

### 4. Bare Metal First — Edge/On-Prem K8s

.deb packaging + APT repo pipeline for bare metal K8s nodes. Most K8s tooling
assumes cloud. Unheaded's bare-metal-first approach (WEST/EAST, P2P links,
local APT repo, systemd units) is what edge computing and on-prem K8s shops need.

### 5. Supply Chain Security (ADR-004)

"No deps from unknown authors, everything pre-July 2019, build our own in Rust."
The supply chain security crowd would love this philosophy applied to K8s
operators. Every CTO who's been burned by a compromised npm/PyPI package
understands this.

## Decision

### Phase 1: Extract Kingdom Operator (Helm Chart)

Extract Wotan + Akira + runbook engine as a standalone Helm-deployable
"Kingdom Operator" that provides:
- Consensus-based health monitoring (Akira, 66.67% threshold)
- Automated runbook execution (51+ YAML runbooks)
- gRPC message bus (Wotan, topic pub/sub)
- PostgreSQL persistence (The Well)
- Trust-level RBAC integration

### Phase 2: KubeCon Readiness

- Monad eBPF observability demo (no sidecars)
- Zhenai AI operator demo (local inference, autonomous runbooks)
- Benchmark: Wotan vs NATS vs Kafka for in-cluster messaging

## Consequences

### Positive
- Fills real gaps in K8s ecosystem (consensus health, AI ops, bare metal)
- Standalone value — doesn't require full Unheaded stack
- CNCF sandbox candidacy potential

### Negative
- Extraction requires decoupling from monorepo
- Helm chart maintenance is a separate burden
- K8s community expects cloud-native (our bare-metal-first is contrarian)
