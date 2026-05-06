# Sensor Deployment Audit + Kindnet Enforcement Test Recipe

**Date:** 2026-05-06
**Author:** Marshal (overnight, in response to scrutiny findings SEN5 + SEN8 in
`docs/compliance/control-matrix/01-scrutiny-2026-05-06.md`)
**Owners (handoff):** MoatGhost (matrix corrections), BlackMage (kindnet
test execution), Architect (CNI replacement decision if test fails),
Developer (deployment runbook updates).
**Status:** *first pass* — written from repo evidence only. No live host
SSH was performed during authoring; every "unverified" claim is unverified
*by this document* and requires operator action to resolve.

---

## Why this exists

Two concrete adversarial findings in tonight's scrutiny pass put two
families of MAPPED matrix entries on shaky ground:

- **SEN5 (Sentinel)** — multiple matrices cite Suricata IDS as evidence
  for **DE.CM-01, AU-2, SI-3, RA-5, A.5.7, CIS 13.3, PCI 11.5**. The repo
  has `pkg/anamnesis/suricata.go` (267 LOC) plus a NixOS module at
  `nixos/modules/suricata.nix`, a Docker build at `docker/suricata/`, and
  a deployment runbook at `docs/security/SURICATA_DEPLOYMENT.md`. **None
  of this proves the daemon is actually running on WEST or EAST today.**
  Many MAPPED claims rest on a hypothetical Suricata deployment.

- **SEN8 (Sentinel)** — `docs/security/k8s-threat-model-2026-05-06.md`
  §3.3 explicitly states *"NetworkPolicy enforcement is unproven on
  kindnet — the chart's `default-deny-all` may be advisory; an attacker
  exploiting any pod can lateral-move freely if so."* Yet matrices cite
  NetworkPolicy as MAPPED for **AC-4, SC-7, SC-7(5), CIS 13.4, A.8.20,
  A.8.22, ZTA Tenet 2, PCI 1.3**, and more (~12 entries). The threat
  model and the matrices contradict each other.

This document closes both loops with:

1. A 4-column **deployment audit table** for every detection-class sensor
   the matrix family cites, marking deployment status truthfully.
2. A **runnable kindnet enforcement smoke test** that produces a
   pass/fail signal in <10 minutes.

The audit is deliberately brutal. Where the answer is "the integration
code exists but I cannot prove the sensor is running anywhere right now,"
this doc says **UNVERIFIED** out loud. That is the honest state today.
Resolving it requires SSH onto WEST + EAST + a kind cluster and running
the verification commands in column 4.

---

# Part 1 — Sensor deployment audit

**Definitions:**

- **DEPLOYED** — service is configured to start (NixOS module enabled in
  the host config, systemd unit installed, OR a script that loads it has
  been observed to run on the target host). Verification command in
  column 4 confirms it *now*.
- **UNVERIFIED** — integration code exists, deployment surface exists
  (Nix module, Docker compose, systemd unit, script), but no first-hand
  confirmation that the sensor is currently running on the target host.
  This is the audit's most common verdict and the honest one.
- **NOT-DEPLOYED** — code exists but no deployment surface ships it; OR
  deployment surface exists but is explicitly disabled in the host
  configuration; OR the code only runs in CI / dev contexts.
- **N/A** — sensor is platform-specific and the target host can't run it
  (e.g. Linux-side eBPF on the macOS dev box).

**Hosts in scope:**

- **WEST** — bare-metal Linux, online (per CLAUDE.md). NixOS host config
  approximation: `nixos/hosts/host-a/`.
- **EAST** — bare-metal Linux, online (per CLAUDE.md). NixOS host config
  approximation: `nixos/hosts/host-b/`. Reachable as `govan@east`.
- **kind** — local 3-node K8s cluster brought up by
  `deploy/k8s/kind/bring-up.sh`. Tear-down between sessions per
  `docs/security/k8s-threat-model-2026-05-06.md`.
- **darwin dev box** — Stevie's macOS workstation. eBPF / Linux-only
  sensors are N/A here.

> **Caveat on host-a/host-b ↔ WEST/EAST mapping.** The NixOS reference
> configs at `nixos/hosts/host-a/` and `nixos/hosts/host-b/` were
> authored as deployment templates. They are **not necessarily a
> bit-identical reflection of what is currently running on WEST or
> EAST.** A NixOS module being `enable = true` in `host-a/configuration.nix`
> demonstrates *intent to deploy*, not *currently deployed*. Confirming
> "currently deployed" requires running column 4's verification command
> on the live host. This caveat is the load-bearing reason most rows
> below are UNVERIFIED rather than DEPLOYED.

## 1.1 Detection-class sensors (Suricata / eBPF / drift / IDS)

| Sensor | Integration code (path) | Deployed today | Verification command | Coverage gap |
|---|---|---|---|---|
| **Suricata IDS** (Monad HbH detection, alert mode by default) | `pkg/anamnesis/suricata.go`; `nixos/modules/suricata.nix`; `docker/suricata/{Dockerfile,suricata.yaml,entrypoint.sh}`; rules in `routing/suricata/rules/unheaded-monad.rules`; deployment doc `docs/security/SURICATA_DEPLOYMENT.md`; smoke-test `scripts/suricata/smoke-test.sh` | **UNVERIFIED.** `host-b/configuration.nix` sets `services.unheaded.suricata.enable = true` but the module's own comment states *"Suricata binary must be built from `~/tmp/suricata/` on bare metal — NOT in this module."* So even if NixOS evaluates the unit, `ExecStart` resolves to `/usr/local/bin/suricata` which only exists if the operator built it manually. **Not present in `host-a/configuration.nix`** — i.e. WEST is configured WITHOUT Suricata. kind cluster has no Suricata pod (chart at `helm/unheaded/templates/` does not include one). darwin: N/A (Linux-only). | `ssh govan@east 'sudo systemctl status suricata && sudo test -S /run/suricata/suricata.socket && sudo tail -5 /var/log/suricata/eve.json'` — exit 0 + recent EVE JSON entries = DEPLOYED. **Plus** `ssh govan@west 'sudo systemctl status suricata'` — expected to fail per host-a config. | **WEST has no Suricata.** Even if EAST runs it, IDS coverage is single-host; cross-host attack traffic on the WireGuard overlay or P2P link will not be observed. CIS 13.3 / PCI 11.5 / NIST SI-3 require coverage of *the network*, not *one host on the network*. **Action:** confirm EAST is live; add Suricata to host-a; add a Suricata DaemonSet to the helm chart for the future K8s production substrate; downgrade matrices to PARTIAL until WEST + kind also deploy. |
| **Anamnesis Lite (event-history service)** | `services/anamnesis/anamnesis.go`; `nixos/modules/services/anamnesis.nix`; bundled tool at `cmd/tools/anamnesis-lite/` (curation only — README + COMPONENTS, no separate binary tonight per WAVE17 lane 4) | **UNVERIFIED.** The Nix module is *imported* in BOTH `host-a/configuration.nix` AND `host-b/configuration.nix` (see `imports = [ ../../modules/services/anamnesis.nix ]`). But the module requires `services.unheaded.anamnesis.enable = true` to actually emit a unit, and **neither host config sets `enable = true` for the anamnesis service explicitly** (grep for `services.unheaded.anamnesis = {` returns nothing in either host). Service binary at `/opt/unheaded/bin/anamnesis` per `ExecStart` — existence on WEST/EAST not confirmed by this audit. | `ssh govan@east 'sudo systemctl status unheaded-anamnesis 2>&1 \| head -10 && curl -fsS http://localhost:50054/health 2>&1 \| head'` (and same for WEST). gRPC port 50054 must be reachable; HTTP health endpoint or systemd "active (running)" line confirms DEPLOYED. | Module imported but not enabled = no unit emitted. AU-2 / SOC2 CC7.2 / PCI 10.1 detection-of-events claims that cite Anamnesis are not backed by an *operating* event-history service. **Action:** confirm whether services.unheaded.anamnesis.enable is set somewhere this audit missed, OR add `enable = true` and rebuild WEST + EAST. |
| **Anamnesis Lite eBPF firehose** (the user-space half — `trace-collector` / `trace-collector-go` reading ring buffers) | `cmd/trace-collector/` (Rust); `cmd/trace-collector-go/` (Go); `nixos/modules/services/trace-collector.nix` | **UNVERIFIED.** `host-a/configuration.nix` *imports* `../../modules/services/trace-collector.nix` AND opens port 9411 (Zipkin-compatible) in the firewall. But again, `services.unheaded.traceCollector.enable = true` is not visible in this audit's grep. host-b does NOT import the trace-collector module. kind cluster: no trace-collector pod in the chart. darwin: N/A. | `ssh govan@west 'sudo systemctl status unheaded-trace-collector && curl -fsS http://localhost:9411/health'` — exit 0 = DEPLOYED on WEST. Same on EAST is expected to fail. **Plus** `ssh govan@west 'sudo bpftool prog list \| grep -E "(packet_marker\|flow_tracker\|latency_probe\|syscall_tracer)"'` — non-empty output = at least one eBPF program loaded. | If `bpftool prog list` returns no Unheaded eBPF programs, the entire eBPF traceability story (cited as AU-2, AU-12, DE.CM-01, SOC2 CC7.2) is hypothetical. The `scripts/load-ebpf.sh` runbook itself flags it as "exit 0 with FAILED>0 is normal under degraded host kernels" — i.e. the load script is permissive about partial failure. **Action:** capture `bpftool prog list` output as deployment evidence on each host. |
| **packet-marker eBPF (XDP)** | `ebpf/packet-marker/` (Rust/Aya). Loaded by `scripts/load-ebpf.sh` which expects pre-built artefacts at `ebpf/target/bpfel-unknown-none/release/packet-marker`. | **UNVERIFIED.** `scripts/load-ebpf.sh` is a manual operator-run script. There is **no NixOS module that auto-runs it on boot**; no systemd unit; no kind/helm equivalent. Per `cmd/tools/anamnesis-lite/README.md`: *"the existing 0.1.1 BPF crates need an upgrade pass to load against `bpftool 7.7+ / libbpf 1.7+` (tracked as kanban `ebpf-aya-upgrade-mn05`)."* Translation: even if someone runs the script today, it may fail to load against modern kernels. | `sudo bpftool prog show \| grep -E "xdp"` (looking for the packet-marker XDP program); `sudo ls /sys/fs/bpf/unheaded/packet-marker/` (looking for pinned program + maps); `sudo ip -d link show <iface>` (looking for `xdp/xdpgeneric prog/id N`). | XDP packet-marker is the *packet-zero* trace ID injection. Without it loaded, every "packet-zero observability" / "L2 trace coverage" claim in the matrices reduces to "we have userspace HTTP middleware tracing" — which dozens of tools provide and is not a kingdom differentiator. **Action:** include `load-ebpf.sh` in `kingdom-startup.service` OR add a NixOS module that loads + pins the eBPF programs. |
| **flow-tracker eBPF (TC)** | `ebpf/flow-tracker/` (Rust/Aya). Same loader path as packet-marker. | **UNVERIFIED.** Same caveats as packet-marker. The TC layer is independent of XDP — it's possible for one to load and the other to fail silently. | `sudo tc filter show dev <iface> ingress` — non-empty output with a `bpf` filter referencing `flow-tracker` = DEPLOYED. `sudo ls /sys/fs/bpf/unheaded/flow-tracker/` for pin verification. | Connection-tracking layer for the trace pipeline; without it, flow context is missing from traces. **Action:** same as packet-marker. |
| **latency-probe eBPF (kprobe/kretprobe)** | `ebpf/latency-probe/` (Rust/Aya). Loader script attempts kprobe attach on `tcp_v4_connect`. | **UNVERIFIED.** Loader script's own message: *"Could not attach kprobe (may need manual perf_event setup)."* — the script itself anticipates failure. | `sudo bpftool perf show` (looking for kprobe `tcp_v4_connect` attached to a BPF program); `sudo ls /sys/fs/bpf/unheaded/latency-probe/`. | RTT-measurement coverage. SC-12 latency-related claims and SLA monitoring rest on this. **Action:** same as packet-marker. |
| **syscall-tracer eBPF (raw_tracepoint)** | `ebpf/syscall-tracer/` (Rust/Aya). Loaded via `bpftool prog attach raw_tracepoint sys_enter`. | **UNVERIFIED.** Loader script: *"Could not attach raw_tracepoint (may need manual setup)."* | `sudo cat /sys/kernel/debug/tracing/available_events \| grep raw_syscalls/sys_enter`; `sudo bpftool prog show \| grep raw_tracepoint`. | Syscall-timeline visibility. AU-2, AU-12, AC-6(9) syscall-audit-style claims rest on this. **Action:** same as packet-marker. |
| **anomaly-ebpf** (XDP-side ML anomaly inference, quantized decision tree) | `ebpf/anomaly-ebpf/` (Rust/Aya). | **NOT-DEPLOYED.** No reference in `scripts/load-ebpf.sh` (which only loads 4 programs: packet-marker, flow-tracker, latency-probe, syscall-tracer). No reference in any NixOS module. No systemd unit. No helm template. No tests of deployment. The code compiles in CI (per `.github/workflows/ebpf.yml`) and that is the only proof of life. | n/a — no deployment surface to verify against. The closest a host gets to it: build the Rust workspace, copy the artefact, run a one-off `bpftool prog load`. | The matrix family's "ML-augmented detection" framing — to the extent any matrix entry leans on it for SI-4 (information-system monitoring) or SI-3 (malicious-code protection) — is unbacked by deployment. **Action:** either (a) add anomaly-ebpf to `scripts/load-ebpf.sh` and do a real-metal test, or (b) remove it from any matrix citation and mark it "research artefact, not operating sensor." |
| **Mímir / heimdall-daemon** (filesystem drift sentry, alerts-only) | `cmd/heimdall-daemon/main.go`; `pkg/{enkrateia,gungnir,gjallarhorn}/`; `crates/heimdall-bpf/` (Aya kprobe scaffold); doc `cmd/tools/mimir/README.md` | **PARTIALLY-DEPLOYED on EAST.** Per CLAUDE.md (2026-04-11 Mímir's Law / Gleipnir Phase 0 entry): *"REAL-METAL VALIDATED on EAST: zero false positives, 100% drift detection accuracy, alerts-only confirmed."* No NixOS module ships this as a unit; no systemd unit in `systemd/`; no helm template. The validation appears to have been a manual operator run. **WEST status: not stated.** | `ps aux \| grep heimdall-daemon` (looking for a running process); `journalctl --user -u heimdall-daemon -n 20 2>&1` (if user-unit) or `sudo journalctl -u heimdall-daemon -n 20`; **plus** `wotan-ctl topic tail drift.detected --max 5 --timeout 5s` (looking for recent drift events from the host's node_id). | One-off validation from April is not "operating sensor today." CIS 1.1 / SI-7 / NIST 800-53 SI-7(1) FIM claims need *continuous* operation evidence, not point-in-time validation. **Action:** package heimdall-daemon as a systemd unit + Nix module; install on WEST + EAST; record `wotan-ctl topic tail drift.detected` evidence over a 7-day window. |
| **Sentinel scripts** (daily detection summary cited in matrices) | The matrix family's reference to "Sentinel" appears to mean the kingdom **skill** (`skills/unheaded-sentinel/`-style instructions for an AI agent), not a daemon. There is **no `pkg/sentinel/` directory and no `cmd/sentinel/` binary**. CI workflow `.github/workflows/security.yml` runs daily at 06:00 UTC and runs govulncheck + gosec — the closest thing to a "sentinel daily run" the repo has. | **MISCITED.** Matrices saying "Sentinel daily detection loop" appear to be conflating: (a) the Sentinel skill (an AI persona that scrutinises evidence), with (b) `.github/workflows/security.yml` (a CI scanner), with (c) "we will, eventually, have a daily SOC-style detection-summary daemon." Today (c) does not exist. | `gh run list --workflow=security.yml --limit 30 \| head` — for what (b) actually ran. There is nothing to verify for (a) or (c). | Matrix entries citing "Sentinel daily detection" as MAPPED for SOC2 CC7.2 / PCI 10.1 / DE.CM-01 should be downgraded. The CI scanner is RA-5 / SI-5 evidence (vuln-scanning), not detection-monitoring evidence. **Action:** rewrite matrix citations to specify the actual artefact being cited (CI workflow vs aspirational daemon vs the skill); add a real `cmd/sentinel-daemon/` if the matrix is to be defended. |
| **NVD + CISA KEV consumption (Sentinel MCP)** | `raft/zhen_mcp_server.py` exposes 7 tools to Claude Code; the matrices cite NVD/KEV consumption via this MCP server. | **UNVERIFIED.** The MCP server is the conduit; the consumption is operator-driven (a Claude Code session running `corpus_search` against NVD-indexed content). There is **no scheduled/automated NVD ingestion pipeline visible in the repo**. SEN6 critique applies in full. | `ps aux \| grep zhen_mcp_server` on the operator's machine; OR confirm the MCP server is reachable via the Claude Code MCP config. *No automated cadence to verify.* | Per SEN6, "consumption is not vulnerability management." The matrices currently overclaim RA-5 / SI-5 / 03.11.02 of 800-171 / 6.3.1 of CIS / 7.x by treating "operator can ask Zhen about CVEs via MCP" as continuous monitoring. **Action:** build a scheduled NVD/KEV ingest pipeline that emits topic events; until then, downgrade these citations to PARTIAL. |
| **trace-collector (Rust)** vs **trace-collector-go (Go)** | Both exist; `cmd/trace-collector/` is the Rust variant, `cmd/trace-collector-go/` is the Go variant (the canonical one per WAVE17 work and the `nixos/modules/services/trace-collector.nix` module). | **UNVERIFIED** (see Anamnesis Lite eBPF firehose row above; same Nix module governs both). Only one of the two ought to run on a given host. | Same commands as the eBPF firehose row. | If both are running, eBPF ringbuffer reads will race; if neither is running, the trace pipeline is broken silently. **Action:** declare ONE canonical collector per ADR; document the deprecation of the other; verify only one runs on each host. |
| **ebpf-exporter** (Prometheus exporter for BPF map metrics) | `cmd/ebpf-exporter/main.go`; `nixos/modules/ebpf-exporter.nix` — imported by both host-a and host-b. | **UNVERIFIED.** Module imported in both host configs; firewall opens port 9435 on host-a. Whether `services.unheaded.ebpfExporter.enable = true` is set is not visible in the host-level `services.unheaded` block grep this audit ran. | `curl -fsS http://localhost:9435/metrics \| head -20` on each host; non-empty Prometheus exposition with `unheaded_*` series = DEPLOYED. | If unset, BPF metrics are not exposed for Prometheus / Grafana. SI-4 / DE.CM-01 metric-reporting evidence is then absent. **Action:** confirm setting + verify endpoint. |
| **shield-ebpf** (firewall enforcement) | `ebpf/shield-ebpf/`; `cmd/shield/`; `nixos/modules/services/shield.nix` | **UNVERIFIED.** Mentioned as sharing the `cluster-id: 99` with Suricata in the Nix module. Module not visibly enabled in either host. | `sudo bpftool prog show \| grep -i shield`; `ps aux \| grep shield-` (userspace half). | If unloaded, the eBPF-side firewall does not run; SC-7 / AC-4 / CIS 13.x boundary-protection claims that cite Shield are unbacked. |

## 1.2 Network-policy + boundary-protection citations

The most-cited "MAPPED" detection mechanism in the matrices is actually
not a detection sensor at all but a *preventive control*: the helm chart's
NetworkPolicy templates. **Tested separately in Part 2 of this document.**

| Control / sensor | Where deployed | Verification | Coverage gap |
|---|---|---|---|
| `default-deny-all` NetworkPolicy | `helm/unheaded/templates/networkpolicy.yaml` (chart-level), applied to namespace `unheaded` when `networkPolicy.enabled: true` | `kubectl -n unheaded get networkpolicies` should list `default-deny-all` AND `allow-internal` AND optionally `allow-gateway-ingress`. | **Existence of the NetworkPolicy resource is not enforcement** — see Part 2. Default kindnet does not enforce NetworkPolicies. The chart resources are advisory under kindnet. |
| `allow-internal` NetworkPolicy | Same file; allows intra-namespace any-to-any + DNS UDP/TCP 53 | Same `kubectl get networkpolicies` evidence | Even if enforced, intra-namespace allow-internal is *permissive*. Any pod can reach all 8 siblings. SEN8 + the K8s threat model §3.3 already noted this. |
| `allow-gateway-ingress` NetworkPolicy | Same file, conditional on `gateway.enabled` | Same | Production-only concern; kind exposes via NodePort 30080/30081 mapped on the kind container. |
| Suricata HbH-detection on the WireGuard overlay | n/a — not deployed | n/a | The overlay carries cross-host traffic between WEST and EAST; no IDS observes it. The "we detect Monad protocol abuse" claim is undermined by the absence of an IDS sensor on the actual cross-host path. |

## 1.3 What this audit does NOT cover

In the spirit of the Scientist's S8 critique (*"the audit cannot prove
its own completeness"*), this section enumerates **known omissions of
this audit**. They are out of scope for tonight; future audit revisions
should pick them up.

- **WAF** (`cmd/waf/`) — referenced in matrices but not enumerated as a
  sensor here. Probably a SC-7 / AC-4 boundary control, not a detective
  sensor; treat as PARTIAL pending its own deployment audit.
- **chaos-controller** (`cmd/chaos-controller/`) — adversarial validation
  tooling, not a sensor.
- **lich-security** (`cmd/lich-security/`) — campaign-execution tooling
  for offensive validation, not a deployed sensor.
- **routing-health** (`cmd/routing-health/`) — operational health check,
  not a security sensor.
- **wotan topic-signing** — preventive control, not a sensor; covered
  elsewhere (BM3 in scrutiny, ML-DSA-65 work).
- **Prometheus / Grafana / Loki / Jaeger** — interchangeable adapters per
  CLAUDE.md "Observable by Default" section. Their deployment is in
  scope for an *observability audit* that complements this *sensor
  audit*.
- **Champion gate audit log** — preventive + recording, but the recording
  side is a sensor of operator actions. Audit at `pkg/champion/`. UNVERIFIED
  whether any operator session's audit log has ever been read by a
  reviewer.
- **Wotan log aggregation ring buffer** — `pkg/logagg/`. The buffer
  exists and is wired (per CLAUDE.md S36 Four Pillars), but the
  *retention* problem (SEN3) means it is detection-floor not detection-coverage.

## 1.4 Audit summary — sensor deployment scorecard

| Sensor | WEST | EAST | kind | darwin |
|---|---|---|---|---|
| Suricata IDS | NOT-DEPLOYED (host-a omits) | UNVERIFIED (host-b enabled, but binary build manual) | NOT-DEPLOYED | N/A |
| Anamnesis (event-history service) | UNVERIFIED | UNVERIFIED | NOT-DEPLOYED | N/A |
| trace-collector (eBPF firehose user-space) | UNVERIFIED | NOT-DEPLOYED (module not imported) | NOT-DEPLOYED | N/A |
| packet-marker (XDP eBPF) | UNVERIFIED (manual loader) | UNVERIFIED (manual loader) | NOT-DEPLOYED | N/A |
| flow-tracker (TC eBPF) | UNVERIFIED (manual loader) | UNVERIFIED (manual loader) | NOT-DEPLOYED | N/A |
| latency-probe (kprobe eBPF) | UNVERIFIED | UNVERIFIED | NOT-DEPLOYED | N/A |
| syscall-tracer (raw_tracepoint eBPF) | UNVERIFIED | UNVERIFIED | NOT-DEPLOYED | N/A |
| anomaly-ebpf | NOT-DEPLOYED | NOT-DEPLOYED | NOT-DEPLOYED | N/A |
| Mímir / heimdall-daemon | UNVERIFIED | PARTIALLY-DEPLOYED (April spike validation) | NOT-DEPLOYED | N/A |
| Sentinel daemon (as cited in matrices) | NOT-DEPLOYED (does not exist as a daemon) | NOT-DEPLOYED | NOT-DEPLOYED | NOT-DEPLOYED |
| NVD/KEV ingestion pipeline | UNVERIFIED (operator-driven via MCP) | UNVERIFIED | UNVERIFIED | UNVERIFIED |
| ebpf-exporter (Prometheus) | UNVERIFIED | UNVERIFIED | NOT-DEPLOYED | N/A |
| shield-ebpf | UNVERIFIED | UNVERIFIED | NOT-DEPLOYED | N/A |

**Bottom line:** of 13 named sensors across 3 in-scope hosts (39 cells),
**zero are confirmed-DEPLOYED by this audit's evidence**. **24 are
UNVERIFIED.** **15 are NOT-DEPLOYED** (mostly kind cluster gaps, plus
the Sentinel daemon that doesn't exist and anomaly-ebpf which has no
deployment surface at all). The matrix family's MAPPED claims that lean
on these sensors should be downgraded to **PARTIAL** until the column-4
verification commands are run on the live hosts and produce the expected
evidence.

The most defensible posture today is to:

1. Run the verification commands tonight (operator-action item).
2. For each cell that produces the expected output, mark CONFIRMED
   alongside the timestamp + command output.
3. For each cell that does not, add the deployment work to the kanban
   backlog with an owner and a target sprint.
4. Update the matrices accordingly. **No matrix entry citing one of
   these sensors should remain MAPPED until at least 1-of-N verification
   passes.**

---

# Part 2 — Kindnet enforcement test recipe

**Goal of this test:** answer the binary question, *"Does kindnet
enforce the chart's NetworkPolicies, or are they advisory?"*

**Why it matters:** ~12 MAPPED matrix entries depend on the answer being
"yes." If the answer is "no," those entries must be downgraded to
PARTIAL and a CNI replacement (Calico or Cilium) must land before the
matrices can be honestly remapped.

**Pre-flight:** this test runs against a **fresh** kind cluster, brought
up via `deploy/k8s/kind/bring-up.sh`. Existing clusters should be torn
down first to avoid state contamination.

**Time budget:** ~10 minutes including cluster bring-up.

**Test interpretation:** any single test (b) or (c) succeeding when it
should fail = enforcement is broken = CNI replacement required.

## 2.1 Procedure

### Step 0 — clean slate

```bash
# Tear down any existing Unheaded kind cluster.
kind delete cluster --name unheaded || true

# Confirm kubectl is not pointed at a stale cluster.
kubectl config current-context  # should not include "unheaded" yet
```

### Step 1 — bring up cluster + helm install (~90s)

```bash
cd /Users/govan/tmp/unheaded
./deploy/k8s/kind/bring-up.sh

# At end, expect:
#   Pods: 9/9 Running across 2 worker nodes
#   Services: wotan, timeguru, captain, micromanager, architect, monad,
#             sophia, dashboard-backend, kanban-app
```

If `bring-up.sh` warns about missing images, build them first:

```bash
docker compose build
./deploy/k8s/kind/bring-up.sh
```

### Step 2 — confirm the NetworkPolicy resources exist

```bash
kubectl -n unheaded get networkpolicies

# Expected output (3 resources):
#   NAME                     POD-SELECTOR   AGE
#   default-deny-all         <none>          ~1m
#   allow-internal           <none>          ~1m
#   allow-gateway-ingress    app=gateway     ~1m   (only if gateway enabled)

# If 0 NetworkPolicies are listed, the chart did not install them.
# Re-check helm/unheaded/values-local.yaml for `networkPolicy.enabled: true`
# and re-run bring-up.sh. STOP — the test is meaningless without policies.
```

### Step 3 — set up an "attacker" namespace and a target pod outside `unheaded`

```bash
# Create a separate namespace whose pods should be unable to reach
# pods in the unheaded namespace per default-deny-all.
kubectl create namespace attacker

# Run a curl/nc tool pod inside `attacker`.
kubectl -n attacker run attack-pod \
  --image=nicolaka/netshoot \
  --restart=Never \
  --command -- sleep 3600

# Wait for it to be Running.
kubectl -n attacker wait --for=condition=Ready pod/attack-pod --timeout=60s

# Also create a target pod in the `unheaded` namespace (use existing
# wotan service IP for cleanest test).
WOTAN_POD_IP=$(kubectl -n unheaded get pods \
  -l app.kubernetes.io/name=wotan \
  -o jsonpath='{.items[0].status.podIP}')
echo "Wotan pod IP: $WOTAN_POD_IP"

# Pick any sibling pod in `unheaded` to test intra-namespace allow-internal:
TIMEGURU_POD_IP=$(kubectl -n unheaded get pods \
  -l app.kubernetes.io/name=timeguru \
  -o jsonpath='{.items[0].status.podIP}')
echo "Timeguru pod IP: $TIMEGURU_POD_IP"
```

### Step 4 — Test (a): intra-namespace pod-to-pod (should PASS)

```bash
# Run nc from one pod inside `unheaded` to a sibling pod inside `unheaded`.
# allow-internal NetworkPolicy permits this.

# Exec into a wotan-namespace pod and curl a sibling.
kubectl -n unheaded exec -it deployment/wotan -- \
  /bin/sh -c "nc -zv -w 5 $TIMEGURU_POD_IP 19000 2>&1"

# OR if nc not in image:
kubectl -n unheaded exec -it deployment/wotan -- \
  /bin/sh -c "wget -q --timeout=5 --tries=1 \
    -O- http://$TIMEGURU_POD_IP:19000/health 2>&1 \| head -5"

# PASS criterion: connection succeeds OR the curl returns an HTTP response
# (any HTTP status from the target indicates the connection was permitted).
# FAIL: connection refused / timeout — the chart's allow-internal is
# misconfigured.
```

**Expected outcome:** PASS — both pods are in the `unheaded` namespace and
`allow-internal` permits intra-namespace traffic.

### Step 5 — Test (b): cross-namespace pod (should FAIL under enforcement)

```bash
# Try to reach a pod in `unheaded` from a pod in `attacker`.
# default-deny-all should block this.

kubectl -n attacker exec -it attack-pod -- \
  nc -zv -w 5 $WOTAN_POD_IP 18000 2>&1

# Also try via the service ClusterIP (more realistic attacker behaviour):
WOTAN_SVC_IP=$(kubectl -n unheaded get svc wotan -o jsonpath='{.spec.clusterIP}')
kubectl -n attacker exec -it attack-pod -- \
  nc -zv -w 5 $WOTAN_SVC_IP 18000 2>&1
```

**PASS criterion (i.e. enforcement is working):** both `nc` calls
**fail** with "connection refused" or "timeout — no route to host."

**FAIL criterion (i.e. kindnet does NOT enforce NetworkPolicy):** either
`nc` call **succeeds** (`Connection to ... 18000 port [tcp/*] succeeded!`).
This is the SEN8 hypothesis confirmed.

### Step 6 — Test (c): external internet egress (should FAIL under enforcement)

```bash
# From a pod inside `unheaded`, try to reach 1.1.1.1:80.
# allow-internal egress only permits intra-namespace + DNS;
# default-deny-all egress should block 1.1.1.1.

kubectl -n unheaded exec -it deployment/wotan -- \
  /bin/sh -c "nc -zv -w 5 1.1.1.1 80 2>&1"

# Alternative if nc not in image:
kubectl -n unheaded exec -it deployment/wotan -- \
  /bin/sh -c "wget --timeout=5 --tries=1 -O- http://1.1.1.1/ 2>&1 \| head -5"
```

**PASS criterion (enforcement working):** `nc` fails / `wget` times out.

**FAIL criterion (kindnet does NOT enforce egress):** `nc` reports
`Connection to 1.1.1.1 80 port [tcp/http] succeeded!` or `wget` returns
content from 1.1.1.1.

### Step 7 — Bonus: cross-namespace from `attacker` to external

```bash
# Sanity check — `attacker` namespace has NO NetworkPolicy applied to it,
# so this SHOULD succeed regardless of kindnet enforcement. This proves
# the test pod is not artificially network-isolated and external egress
# from a kind container in general works.

kubectl -n attacker exec -it attack-pod -- \
  nc -zv -w 5 1.1.1.1 80 2>&1
```

**Expected:** succeeds. If this fails, there's a deeper kind/networking
problem that invalidates the rest of the test. Re-investigate before
trusting Step 5 / Step 6 results.

### Step 8 — Tear down

```bash
kubectl delete namespace attacker
kind delete cluster --name unheaded
```

## 2.2 Pass / fail summary table

| Test | What it does | Expected (enforcement on) | Observed | Verdict |
|---|---|---|---|---|
| (a) intra-ns | wotan → timeguru in `unheaded` | succeed | _fill in_ | _fill in_ |
| (b) cross-ns | attacker → wotan pod IP | fail | _fill in_ | _fill in_ |
| (b') cross-ns svc | attacker → wotan svc ClusterIP | fail | _fill in_ | _fill in_ |
| (c) external egress | wotan → 1.1.1.1:80 | fail | _fill in_ | _fill in_ |
| (sanity) attacker external | attacker → 1.1.1.1:80 | succeed | _fill in_ | _fill in_ |

**Overall verdict logic:**

- **All "expected" outcomes match observed → kindnet enforces
  NetworkPolicy.** SEN8 hypothesis falsified. Matrix entries for AC-4 /
  SC-7 / SC-7(5) / CIS 13.4 / A.8.20 / A.8.22 / ZTA Tenet 2 / PCI 1.3
  citing NetworkPolicy may remain MAPPED for the kind substrate.
- **(b), (b'), or (c) succeeds when expected to fail → kindnet does NOT
  enforce NetworkPolicy.** SEN8 hypothesis confirmed. **All matrix
  entries citing NetworkPolicy as evidence must be downgraded to PARTIAL
  with a footnote: "advisory under kindnet; pending Calico/Cilium
  replacement."**
- **(a) fails when expected to pass → chart's allow-internal is
  misconfigured.** Independently of kindnet enforcement, the chart needs
  a fix.
- **(sanity) fails → test apparatus is broken; re-run with fresh
  cluster.**

## 2.3 Interpretation guide

The threat model §3.3 already framed the question. This test answers
it definitively. The three possible end-states:

### End-state A — kindnet enforces (least common in upstream kind)

Document the result, take a screenshot of the test output, file it as
evidence next to the matrix entries. Add to the verification section of
each citing matrix: *"Verified 2026-MM-DD on kind v0.X.Y under kindnet
N.N: tests (a) PASS, (b) FAIL, (c) FAIL — enforcement confirmed."*

### End-state B — kindnet does not enforce (most common, expected outcome)

This is the SEN8 prediction. Actions:

1. **Immediately downgrade matrix entries** citing NetworkPolicy from
   MAPPED → PARTIAL across:
   - `nist-800-53-2026-05-06.md` — AC-4, SC-7, SC-7(5)
   - `cis-controls-v8-2026-05-06.md` — CIS 13.4
   - `iso-27001-27002-2026-05-06.md` — A.8.20, A.8.22
   - `nist-800-207-2026-05-06.md` — ZTA Tenet 2
   - `pci-dss-2026-05-06.md` — PCI 1.3
   - `nist-csf-2-2026-05-06.md` — PR.AC-5
   - `fedramp-moderate-2026-05-06.md` — AC-4, SC-7
   - `soc2-2026-05-06.md` — CC6.1, CC6.6
   Add a footnote: *"NetworkPolicy resource exists in chart; kindnet
   does not enforce it (verified 2026-MM-DD test recipe pass/fail
   results in `docs/security/sensor-deployment-audit-2026-05-06.md`).
   Production substrate must replace kindnet with Calico or Cilium per
   ADR-064 follow-on."*

2. **File the CNI-replacement ADR.** The kind cluster-config explicitly
   notes *"Calico/Cilium are future ADR if we need NetworkPolicy
   enforcement guarantees."* That ADR is no longer optional.

3. **Re-run this test recipe** with the new CNI installed. If it passes,
   re-promote the matrix entries to MAPPED (production-substrate-only)
   while leaving the kind-substrate footnote in place.

### End-state C — partial enforcement (e.g. ingress enforced, egress not)

Document precisely which axis is enforced. Downgrade only the unenforced
axes. This is unusual but not impossible — some CNIs in some kernel
configurations enforce one direction.

## 2.4 Remediation: installing Calico

If the test produces End-state B, the simplest remediation path uses
Calico. The cluster config currently uses default kindnet; switching
requires:

```bash
# Tear down current kind cluster.
kind delete cluster --name unheaded

# Edit deploy/k8s/kind/cluster-config.yaml to disable default CNI:
#   networking:
#     disableDefaultCNI: true
#     ipFamily: ipv4
#     podSubnet: "10.244.0.0/16"

# Bring up cluster (no CNI yet — pods will be Pending).
kind create cluster --config deploy/k8s/kind/cluster-config.yaml

# Install Calico.
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.27.0/manifests/tigera-operator.yaml
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.27.0/manifests/custom-resources.yaml

# Wait for Calico to be Ready.
kubectl -n calico-system wait --for=condition=Ready pods --all --timeout=300s

# Now run the helm chart and re-run this test recipe.
helm upgrade --install unheaded helm/unheaded/ \
  --namespace unheaded --create-namespace \
  --values deploy/k8s/kind/values-local.yaml \
  --wait --timeout 120s
```

Cilium is the alternative; choice between them is the work of the
follow-on ADR. The test recipe in §2.1 is identical for either CNI —
only the install step changes.

## 2.5 What this test does NOT cover

- **Production K8s substrates** (GKE / EKS / AKS / self-managed) — each
  has its own NetworkPolicy enforcement story and must be tested
  separately. This recipe is kind-only.
- **L7 policy** (e.g. NetworkPolicy with HTTP-method filtering) —
  default kubectl NetworkPolicy is L3/L4 only. Cilium adds L7; this test
  doesn't exercise that surface.
- **Egress to specific IPs** beyond 1.1.1.1 — production should test a
  representative set including DNS-based egress targets.
- **Ingress from outside the cluster** — kind exposes NodePorts mapped
  on the kind container; production ingress posture is governed by the
  cloud LB or a `gateway` Ingress resource and is out of scope here.
- **Pod-to-host (node) traffic** — NetworkPolicy in vanilla form does
  not cover pod-to-host. Calico has additional GlobalNetworkPolicy
  resources for this. Test separately.

---

# Part 3 — Hand-off checklist

This document hands off to three downstream owners.

### To MoatGhost (matrix corrections)

1. After Part 1 verification commands run on WEST + EAST + kind, update
   each matrix entry citing the affected sensors with the verification
   timestamp and observed evidence.
2. After Part 2 test runs, downgrade the ~12 NetworkPolicy entries per
   §2.3 End-state B (or document End-state A confirmation).
3. Cite this document by absolute path in each updated matrix's
   Verification section.

### To BlackMage (test execution)

1. Run Part 2 test recipe end-to-end. Record output verbatim. Identify
   which end-state the kind cluster lands in.
2. If End-state B, attempt the same test against Calico per §2.4 to
   confirm the remediation works.
3. File results as a follow-on document
   `docs/security/kindnet-enforcement-test-results-YYYY-MM-DD.md`.

### To Architect (CNI decision if End-state B)

1. Author the Calico-vs-Cilium ADR (parked as "future ADR" in
   `deploy/k8s/kind/cluster-config.yaml`).
2. Update the cluster config + bring-up script + helm chart values to
   match the chosen CNI.
3. Run Part 2 test recipe against the new CNI and confirm End-state A.

### To Developer (deployment runbooks)

1. For every UNVERIFIED row in §1.4: either add a NixOS module that
   actually emits a systemd unit (set `enable = true` in host configs),
   OR document why the sensor is intentionally not deployed and remove
   the matrix citations.
2. Add `scripts/load-ebpf.sh` to `kingdom-startup.service` (or write a
   real Nix module that loads + pins the eBPF programs).
3. Resolve the `ebpf-aya-upgrade-mn05` kanban item so the eBPF programs
   load against modern bpftool/libbpf.

---

# Provenance

This audit is a desk review of the repo as of 2026-05-06. Sources
inspected:

- `pkg/anamnesis/suricata.go`
- `cmd/heimdall-daemon/main.go`
- `cmd/trace-collector-go/` and `cmd/trace-collector/`
- `cmd/ebpf-loader/`
- `ebpf/{packet-marker,flow-tracker,latency-probe,syscall-tracer,anomaly-ebpf,shield-ebpf}/`
- `scripts/load-ebpf.sh`, `scripts/unload-ebpf.sh`, `scripts/suricata/`
- `nixos/modules/{suricata,ebpf-exporter}.nix` and
  `nixos/modules/services/{anamnesis,trace-collector,shield}.nix`
- `nixos/hosts/host-a/configuration.nix` and
  `nixos/hosts/host-b/configuration.nix`
- `helm/unheaded/templates/networkpolicy.yaml`
- `deploy/k8s/kind/{cluster-config.yaml,bring-up.sh,values-local.yaml}`
- `docs/security/SURICATA_DEPLOYMENT.md`
- `docs/security/k8s-threat-model-2026-05-06.md`
- `docs/battle-plans/WAVE17-WAKEUP-SUMMARY.md`
- `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md`
- `cmd/tools/{anamnesis-lite,mimir,zhen-on-prem}/{README,COMPONENTS,BUILD}.md`
- `.github/workflows/security.yml` and `ebpf.yml`
- CLAUDE.md (build-state of WEST + EAST + Mímir's Law April spike)

No live host SSH performed during authoring. No kind cluster brought up
during authoring. The Part 2 test recipe is designed to be run by an
operator (BlackMage) who *will* perform live actions.

The badge is fair. UNVERIFIED is honest.
