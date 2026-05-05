# ADR-064 — Wotan Active/Active Cluster (3-node minimum, K8s-native, broadcast replication)

**Status:** Proposed (architecture spec; phased implementation begins on Stevie's go-ahead)
**Date:** 2026-05-05
**Deciders:** Stevie Bellis + unheaded-architect (system shape) + unheaded-developer (impl) + unheaded-blackmage (split-brain / Byzantine threat model) + unheaded-marshal (phase-gate enforcement)
**Supersedes:** ADR-035 — Wotan Active-Passive Redundancy (Accepted, Phases 0-2 complete; superseded but the WAL-replication code from Phase 1 stays as-is, repurposed)
**Aligns with:** ADR-040 (Kubernetes Ecosystem Strategy — Wotan as the lightweight K8s-native message bus); ADR-029 (Akira consensus health, 66.67% threshold); ADR-005 (Wotan as the Kingdom's message backbone); ADR-062 (Lich framework — opens LICH-014/015/016 for cluster threats)

**Triggered by:** Stevie's directive 2026-05-05: *"active/active with anycast/broadcast - events, alerting, upc, service to service communication, etc all work via wotan - it must always be up and redundant ( min 3 node cluser - hop on k8 bandwagon )"*

---

## Context

Today's Wotan is single-instance per host with a partial active-passive replication (ADR-035 Phases 0-2). The pieces that exist:

- `services/wotan/internal/cluster/config.go` — typed `Mode` / `Role`, validation, no consensus primitives
- `services/wotan/internal/cluster/failover.go` — `FailoverManager` state machine, **driven externally by Akira's 66.67% consensus** (ADR-029); doesn't elect leaders itself
- `services/wotan/internal/cluster/replication_server.go` — gRPC stream pushing `*wal.WAL` entries by sequence
- `services/wotan/internal/cluster/replication_client.go` — pulls + applies entries, mTLS-auth
- `pkg/storage/wal/` — WAL primitive used both for local persistence and replication
- `helm/unheaded/templates/wotan.yaml` — single-pod K8s deployment template (NOT a StatefulSet, no replicas)
- ADR-035 status: ACCEPTED, Phases 0-2 complete (failover wiring, mTLS replication, basic switchover)

The shape today is: one writable primary (WEST), one read-replica (EAST), Akira coordinates failover via Wotan messages. The directive moves us to:

1. **Active/active**: every node accepts writes; no static primary/standby split
2. **Anycast publish**: a publisher hits the cluster IP, K8s load-balances to *any* pod, the cluster handles replication internally
3. **Broadcast subscribe**: a subscriber on any node receives every message published to a subscribed topic, regardless of which node accepted the publish
4. **Always up**: 1 of 3 nodes can fail with no observable degradation
5. **Min 3 nodes**: explicit floor — quorum-based consensus needs ≥3 to tolerate 1 failure
6. **K8s-native**: StatefulSet, headless Service, PVCs, PodDisruptionBudget — the standard K8s pattern for stateful clustered services

Wotan is the bus everything else sits on (events, alerting, UPC packets, service-to-service comms). Its uptime is a Kingdom property, not a service-level concern.

---

## Decision

**Wotan v2: 3-node-minimum active/active cluster on K8s, with Raft for cluster membership + topic-leader election, quorum-acked writes, and broadcast replication to all replicas.**

The full ADR-035 active-passive design is replaced. The implementation pieces from ADR-035 Phases 0-2 are not discarded — they are repurposed:

- WAL replication stream (Phase 1) → becomes the per-topic replication mechanism between leaders and followers; protocol mostly unchanged
- mTLS gRPC channel (Phase 1) → becomes the inter-pod transport; cert provisioning shifts from manual to K8s-issued via cert-manager
- FailoverManager (Phase 0) → becomes the local FailoverState machine driven by the new Raft layer's election events; the Akira-driven external trigger (ADR-029) becomes a fallback/sanity-check signal rather than the primary control plane

---

## Architecture

### Cluster topology

```
                                   ┌────────────────────┐
                          publish  │  Client (any pod   │  subscribe
                          ───────▶│   wanting Wotan)   │ ◀───────
                                   └────────┬───────────┘
                                            │
                              ClusterIP svc │ headless DNS
                              (anycast      │ (all-pods round
                               load-balance)│  robin / sticky)
                                            ▼
                       ┌────────────────────────────────────────┐
                       │  K8s Service: wotan.unheaded.svc       │
                       └────────┬──────────┬──────────┬─────────┘
                                │          │          │
                  pod-anti-     ▼          ▼          ▼
                  affinity   ┌─────┐    ┌─────┐    ┌─────┐
                  (different │wotan│    │wotan│    │wotan│
                   K8s nodes)│ -0  │    │ -1  │    │ -2  │
                             └──┬──┘    └──┬──┘    └──┬──┘
                                │          │          │
                                ▼          ▼          ▼
                            PVC (WAL) PVC(WAL)   PVC(WAL)
                                │          │          │
                                └─── Raft consensus ──┘
                                  (gRPC + mTLS, 18002)
```

### Consensus model — Raft for cluster, topic-leader for ordering

We use **Raft** (via `github.com/hashicorp/raft` or `etcd-io/raft`) for two distinct purposes:

1. **Cluster membership**: who is in the cluster, who is the leader, what's the current term. Raft handles join/leave, leader election, and config-change consensus. Standard machinery; we don't reinvent it.

2. **Topic-leader election**: each topic has exactly one leader at any moment (the node that accepts writes for that topic). Raft assigns topic leadership across nodes when topics are created. A topic's leader election is a Raft log entry; followers learn of the assignment by replaying the log.

This is the **NATS JetStream / Kafka KRaft model**, well-trodden ground. We are NOT inventing a new consensus algorithm.

### Write path (publish)

1. Client hits ClusterIP service → K8s load-balances to (say) pod-1
2. Pod-1 looks up topic-leader for the topic from its local Raft state. If pod-1 is leader: write to local WAL, broadcast `topic.message` Raft log entry to pods 0+2. If pod-1 is NOT leader: forward the publish to the leader pod.
3. Quorum write: leader needs an ack from at least ⌈N/2+1⌉ peers (2 of 3) before responding to the client. This is the **CP** corner of CAP — partitioned-and-not-quorate nodes refuse writes rather than accept and risk diverging.
4. Once acked, the message is durably committed; subscribers on every node can see it.

### Read path (subscribe)

1. Client opens a stream to a specific pod (via headless DNS — `wotan-0.wotan.unheaded.svc`) OR via ClusterIP (any pod).
2. Subscribed messages stream from the pod's local replicated state. Every pod has every committed message.
3. Reads are *local* — no cross-node hops, low latency.

### Replication semantics

- **Per-topic monotonic ordering** (Raft log gives this for free per topic-partition).
- **No global total order across topics** — topics are independent partitions (otherwise we'd bottleneck through one Raft group).
- **At-least-once delivery** — subscribers may see a message twice if a leader fails mid-broadcast and the new leader re-emits; client subscriptions de-dupe by `message_id`.
- **Quorum committed** — a publish doesn't return success until the message has been written to ⌈N/2+1⌉ pods' WAL.

### Failure model

| Scenario | Behaviour |
|---|---|
| 1 of 3 pods down | quorum (2 of 3) maintained → write+read both work; degraded write latency on topics whose leader was on the dead pod (until re-election ~2-3s) |
| 2 of 3 pods down | quorum lost → cluster goes **read-only**; in-flight writes either succeed (committed before the failure) or fail (returned as `cluster_quorum_lost` error). Reads continue from local replicated state. |
| Network partition (1 vs 2) | minority side rejects writes (CAP: CP); majority side continues; healing rejoins minority's stale state via Raft log replay on partition heal |
| All 3 down | hard failure; messages in WAL replay on first quorum-restore; topics with non-replicated in-flight writes lose those writes (acceptable for at-least-once semantics) |

### Anycast vs broadcast — what each means in K8s

- **Anycast publish**: ClusterIP service load-balances to any one of N pods. The publisher doesn't pick; K8s' kube-proxy + IPVS does. *One physical message reaches one pod; the cluster fans it out.*
- **Broadcast subscribe**: each pod's local subscription stream sees every committed message. *Every subscriber on every pod gets every message.*
- The **headless service** (`wotan-headless.unheaded.svc`) gives DNS records for each pod individually — used by inter-pod gRPC for Raft + replication, and by clients that need sticky-pod connections (e.g. long-lived bidi streams that should stay on one pod for the connection's lifetime).

### Load-balancing — round-robin at every dispatch layer

> Stevie 2026-05-05: *"roundrobin"*

Round-robin at every dispatch point. Predictable distribution, no hot-pod surprises, easy to verify load is actually balanced.

| Dispatch layer | Mechanism | Configuration |
|---|---|---|
| **Client → cluster (publish)** | K8s ClusterIP service via kube-proxy in IPVS mode | `kube-proxy --proxy-mode=ipvs --ipvs-scheduler=rr` (NOT iptables; iptables uses random selection, not RR. IPVS-rr gives strict round-robin per the K8s docs.) Cluster MUST be deployed with kube-proxy in IPVS mode for this guarantee. |
| **Client → cluster (subscribe)** | Headless service DNS rotation | Standard K8s headless DNS already returns A records in rotated order on each query. Clients that re-resolve DNS per connection get round-robin pod selection. Long-lived streams stay sticky to one pod. |
| **Inter-pod (Raft AppendEntries)** | Parallel fan-out from leader to all followers | Not RR — leader sends to *every* follower simultaneously; quorum ack determines commit. RR doesn't apply here. |
| **Topic-leader assignment at create-time** | Round-robin across cluster members | When a new topic is created, the cluster's Raft leader assigns it a topic-leader pod by walking the member list in order, modulo the topic count. This balances write load across pods rather than letting a hot-topic pile up on one. The assignment is durable in the Raft log. Re-balancing on cluster scale-up: opt-in (avoid forced churn). |
| **WAL replication catch-up after a partition** | Sequential per-peer; not RR | Each follower catches up independently from the leader; not a load-balanced operation. |

**The hot-publish path (line 1 above) is the round-robin Stevie is asking about.** Without it, kube-proxy's default iptables mode would distribute traffic via random selection — statistically uniform but with worse tail behaviour under low traffic counts. IPVS-rr is the right choice and is the default for new K8s clusters built post-1.20.

### K8s manifests (StatefulSet + Services + PDB + NetworkPolicy)

```yaml
# A sketch — full manifests live at deploy/k8s/wotan/ when this ADR activates
apiVersion: v1
kind: Service
metadata:
  name: wotan
spec:
  type: ClusterIP
  selector: {app: wotan}
  ports:
    - {name: http, port: 18000}
    - {name: grpc, port: 18001}
---
apiVersion: v1
kind: Service
metadata:
  name: wotan-headless
spec:
  clusterIP: None  # headless — DNS returns each pod's IP
  selector: {app: wotan}
  ports:
    - {name: replication, port: 18002}
    - {name: raft,        port: 18003}
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: wotan
spec:
  serviceName: wotan-headless
  replicas: 3
  selector: {matchLabels: {app: wotan}}
  template:
    metadata: {labels: {app: wotan}}
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector: {matchLabels: {app: wotan}}
              topologyKey: kubernetes.io/hostname  # one pod per K8s node
      containers:
        - name: wotan
          image: unheaded/wotan:vX
          args:
            - --cluster-mode=cluster
            - --cluster-replicas=3
            - --cluster-peer-discovery=k8s-headless
            - --cluster-peer-service=wotan-headless.unheaded.svc.cluster.local
          ports:
            - {containerPort: 18000, name: http}
            - {containerPort: 18001, name: grpc}
            - {containerPort: 18002, name: replication}
            - {containerPort: 18003, name: raft}
          volumeMounts:
            - {name: wal, mountPath: /var/lib/unheaded/wotan/data}
          readinessProbe:
            grpc: {port: 18001, service: wotan}
          livenessProbe:
            httpGet: {path: /health, port: 18000}
  volumeClaimTemplates:
    - metadata: {name: wal}
      spec:
        accessModes: [ReadWriteOnce]
        resources: {requests: {storage: 50Gi}}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: wotan
spec:
  minAvailable: 2  # never disrupt more than 1 of 3 — quorum preserved
  selector: {matchLabels: {app: wotan}}
```

(Plus a `NetworkPolicy` restricting ingress on 18002/18003 to peer pods only.)

### Existing flag re-use

The cluster flags from ADR-035 (`--cluster-mode`, `--cluster-role`, `--cluster-peer`, `--cluster-replication-port`, `--cluster-pki-dir`) stay. New flags:

- `--cluster-replicas N` — expected replica count (used to compute quorum threshold)
- `--cluster-peer-discovery [static|k8s-headless]` — how peers are discovered; `static` for non-K8s deployments (specifies peer addrs), `k8s-headless` for K8s (pulls from headless service DNS)
- `--cluster-peer-service` — DNS name to resolve for peer discovery when `k8s-headless`
- `--cluster-raft-port` — Raft RPC port (default 18003)

The `--cluster-role={primary|standby}` flag is kept for backwards-compatibility with the active-passive deployment but is **deprecated** in active-active mode (every node is both, dynamically). When `--cluster-mode=cluster` and `--cluster-replicas≥3`, the role flag is ignored with a one-time deprecation log line.

---

## BlackMage threat model — new T-numbers

| ID | Threat | Mitigation |
|---|---|---|
| **T21** | Split-brain — network partition allows two minorities to both think they're authoritative | Raft's quorum rule: only the majority partition can elect a leader. Minority partitions go read-only. CP, not AP. |
| **T22** | Byzantine peer (compromised pod returns malicious replication entries) | Raft's standard log-matching property catches divergent entries; mTLS with cert-manager-issued certs (rotated daily) prevents trivial peer impersonation; entries are signed with ML-DSA-65 per ADR's wotan-topic-signing baseline. |
| **T23** | Replay attack on join — an old pod's WAL replayed at rejoin time corrupts the active log | Raft's term + index monotonicity rejects stale entries. WAL contents are advisory after a term transition; the cluster's consensus log is the source of truth. |
| **T24** | DoS via topic-leader-thrashing — attacker repeatedly causes leader pod to crash, forcing constant re-election | Akira's existing 66.67% consensus health (ADR-029) detects flapping; `FailoverManager` cooldown prevents elections faster than every 30s. |
| **T25** | Resource exhaustion via unbounded topic creation | Topic auto-approval allowlist (already in `configs/wotan.yaml`); new topics outside the allowlist require operator approval — same gate as ADR-035. |
| **T26** | K8s API compromise → attacker scales StatefulSet to 1 → quorum lost → cluster bricked | PodDisruptionBudget with `minAvailable: 2` blocks the kube-apiserver from evicting more than one pod at once; Stevie's existing K8s RBAC limits StatefulSet edits to specific principals (separate ADR for K8s RBAC). |
| **T27** | Cert rotation gap — cert-manager-issued mTLS certs expire mid-flight, replication breaks silently | Pre-emptive renewal at 50% of cert lifetime (cert-manager default); alerting on `cert_expiry < 24h` Prometheus metric; replication-channel-down alert fires if peers lose mTLS connectivity. |

### Pre-registered Lich campaigns (per ADR-062 framework)

- **LICH-014** — split-brain probe: kill networkpolicy between pod-0 and (pod-1 + pod-2). Verify pod-0 rejects writes within 5s; verify quorum side continues; verify rejoin replays cleanly.
- **LICH-015** — Byzantine peer: replace pod-2's `wotan` binary with one that returns garbage `AppendEntries` responses. Verify cluster's `log_inconsistency_detected` metric fires; verify pod-2 is forcibly removed from quorum.
- **LICH-016** — K8s pod-killing chaos: kubectl delete pod wotan-{0,1,2} repeatedly. Verify PDB blocks 2-of-3 deletions; verify single-pod-loss is recovered with no observable client error.

Each campaign gets a `tomb/lich/LICH-NNN-<name>/` skeleton when this ADR's Phase 1 starts.

---

## Phased delivery

This is a 4-phase rollout. **No phase ships without H0 coding-gate clean across the change**.

### Phase 0 — Spec + bench harness (this ADR + a 3-pod kind cluster)

- This ADR lands and gets reviewed (Stevie + crew)
- `kind` cluster with 3 nodes spun up under `tomb/lich/cluster-spike/` for benching
- Existing `services/wotan/internal/cluster/` reorganised into `services/wotan/internal/raft/` + `services/wotan/internal/replication/` (renaming, not new code)

### Phase 1 — Raft membership + leader election (no traffic switch yet)

- `services/wotan/internal/raft/` integrates `hashicorp/raft` (decision: hashicorp's lib over etcd's because of its snapshot-store abstraction simpler match for our WAL backend)
- Pods start in cluster-mode with peer discovery via `wotan-headless` DNS
- Cluster elects a leader; topic-leader assignments published to a special `__cluster.topics` topic
- Existing ADR-035 mTLS replication channel **kept unchanged** for the WAL stream
- Acceptance: 3-node kind cluster boots, elects leader within 5s, leadership transfer takes <2s on pod kill

### Phase 2 — Quorum-acked publish

- Publish API path becomes leader-routed: any pod accepting a publish either commits (if leader for that topic) or forwards to the leader
- Writes block until quorum-acked; new error code `cluster_quorum_lost` for partition cases
- LICH-014 (split-brain) opened
- Acceptance: H0 coding gate clean against the cluster; LICH-014 passes split-brain probe

### Phase 3 — K8s manifests + cert-manager + PDB

- `deploy/k8s/wotan/` ships StatefulSet + Services + NetworkPolicy + PDB
- cert-manager Issuer + Certificate resources for mTLS
- `helm/unheaded/templates/wotan.yaml` migrates from Deployment to StatefulSet
- LICH-016 (K8s pod-kill chaos) opened
- Acceptance: kubectl delete pod wotan-1 → cluster recovers within 30s with no client-observable errors

### Phase 4 — Akira integration + ADR-035 deprecation

- Akira's 66.67% consensus continues to run as a sanity check; an Akira "primary down" assertion now triggers a Raft-level health probe rather than directly switching FailoverManager state
- ADR-035 status flipped: ACCEPTED → SUPERSEDED by ADR-064
- Migration runbook in `runbooks/cluster/wotan-active-active-cutover.yaml` for the bare-metal WEST+EAST → K8s cluster transition
- Acceptance: WEST + EAST can be retired (or kept as fallback) once the K8s cluster has run clean for 7 days

Per ADR-052 (timeline source-of-truth policy): the timeline updates at each phase exit; this ADR is referenced from `references/timeline.md`'s Age 3 section.

---

## What this ADR does NOT cover

- **Specific CNCF Operator pattern** (controller-runtime, OperatorSDK). Phase 3 ships plain StatefulSet manifests; an Operator wraps them later if/when configurable cluster-resize becomes worth it. Out of scope here; track separately.
- **Cross-cluster federation** (Wotan in cluster A talks to Wotan in cluster B). Per-cluster operation only; federation is its own ADR if Stevie wants it.
- **Subscribe filtering / fan-out optimisation**. Every pod gets every message in v1; if subscriber load justifies it later, partitioning by topic name across pod subsets is a future optimisation (own ADR).
- **Wire-format changes**. The Monad protocol stays frozen at v0x01; the cluster ships internally between pods using the existing replication gRPC. No external wire-format changes.
- **Storage backend changes**. WAL stays primary; PostgreSQL persistence per ADR-035 stays optional per-pod. No new pkg/* packages introduced (the existing `pkg/storage/wal/` is reused).

---

## Migration path from ADR-035 (active-passive)

The WEST + EAST bare-metal pair from ADR-035 stays running during the K8s cluster bring-up. Cutover is operator-driven, not big-bang:

1. **Coexistence period** (Phases 1-3): K8s cluster brought up alongside existing WEST/EAST. Both serve the same topics; clients can use either. Akira monitors both.
2. **Cutover** (Phase 4): a runbook (`runbooks/cluster/wotan-active-active-cutover.yaml`) drains traffic from WEST/EAST to the K8s cluster, with an explicit rollback path (re-enable WEST/EAST, drain K8s).
3. **Decommission** (post-Phase 4 + 7-day soak): WEST/EAST take a backup snapshot of their PostgreSQL message store, then are released back to general use. The active-passive code stays in-tree but is marked `// Deprecated: superseded by ADR-064` for one release cycle, then removed.

No data is lost in this migration: every message that landed in WEST's WAL/PG also lives in EAST's via the existing replication; the K8s cluster picks up new traffic from the cutover point forward; historical messages continue to be served from WEST/EAST until decommission.

---

## Why active/active over active-passive (decision rationale)

> Stevie 2026-05-05 (verbatim): *"active passive works but doesn't scale"*

That's the headline reason. Active-passive is correct (we verified that — Phases 0-2 of ADR-035 shipped, the failover state machine works, the Akira consensus drives it cleanly). What it isn't is scalable:

- **Throughput ceiling = 1 pod**. Every publish goes through WEST. The standby (EAST) does no work in steady state — it just receives WAL replays. As Wotan absorbs UPC traffic, alerting pipes, service-to-service messaging, and the eBPF flow stream, "all writes through one pod" becomes the binding constraint long before any other component breaks.
- **Failover gap ≠ zero**. Active-passive has a measurable unavailability window: Akira detects primary down → 66.67%-consensus consensus reached → FailoverManager promotes EAST → ~10-30s during which writes either error or queue. Active/active has no promotion event — node loss is invisible to clients because every node is already serving.
- **Hardcoded WEST primary doesn't compose with K8s replica counts**. StatefulSet's `replicas: N` semantic assumes peer-symmetric pods. ADR-035's "WEST is always primary by divine right" was correct for two named bare-metal hosts; it doesn't translate to "one of these 3 pods is more equal than the others, but which one rotates with K8s scheduling." Either we encode primary-election in the pod template (which is what active/active does) or we fight K8s.
- **CNCF differentiator (per ADR-040)**: "lightweight 3-node active-active message bus with embedded protocol semantics" is a story for the K8s ecosystem audience. "WEST is always primary" is not.

The active-passive design (ADR-035) was the right call when there were exactly two bare-metal hosts and Stevie wanted survival of one. The exit criteria was always "good enough until we need to scale." We're at that point.

---

## References

- ADR-005 — Wotan Message Backbone (the role this ADR supports)
- ADR-019 — Zhen Champion Agent (the gate that protects every wotan-published topic)
- ADR-029 — Wotan Consensus Health / Akira (the 66.67% threshold this ADR continues to honour as fallback)
- ADR-034 — gRPC mTLS Default Transport (the inter-pod transport for Raft + replication)
- ADR-035 — Wotan Active-Passive Redundancy (THIS ADR SUPERSEDES; Phase 0-2 code is reused)
- ADR-040 — Kubernetes Ecosystem Strategy (the K8s-native fit this ADR realises)
- ADR-052 — Timeline source-of-truth policy (timeline updates at each phase gate)
- ADR-062 — Lich framework (LICH-014/015/016 pre-registered above)
- `services/wotan/internal/cluster/` — existing pieces; reorganised in Phase 0
- `pkg/storage/wal/` — WAL primitive; unchanged
- `helm/unheaded/templates/wotan.yaml` — migrates Deployment → StatefulSet in Phase 3
- Stevie's directive (verbatim, 2026-05-05): *"active/active with anycast/broadcast - events, alerting, upc, service to service communication, etc all work via wotan - it must always be up and redundant ( min 3 node cluser - hop on k8 bandwagon )"*
