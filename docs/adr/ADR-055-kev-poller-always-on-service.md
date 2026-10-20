# ADR-055: KEV Poller — Always-On Kingdom Service (and K8s Cross-Host Pilot)

**Status**: **Planned** (hard requirement per Stevie 2026-04-27: *"KEV JSON fetch service MUST ALWAYS be running in the Kingdom — we can play with k8's across metal host with things like that"*)
**Date**: 2026-04-27
**Deciders**: Stevie (initiating), MoatGhost (compliance/threat owner), Sentinel (blue team consumer), Architect (k8s placement), Computermancer (DaemonSet/Service-Mesh integration)
**Related ADRs**: ADR-040 (Kubernetes Ecosystem Strategy), ADR-047 (K8s Honest Assessment + East/West K8s Lab), ADR-69420 (Sleipnir BGP + Unheaded OS — references CVE poller as TODO), ADR-052 (drift policy — applies to threat-register freshness)
**Folds in**: the orphaned `docs/legal-planning` branch's `b35abdbf docs: ADR-69420 TODO — CVE poller service (weekly/daily/hourly configurable)` intent

---

## Context

The 2026-04-27 Round Table sprint surfaced two correlated facts:

1. **Compliance refresh failed locally** because the Cowork sandbox proxy blocked CISA KEV egress (HTTP 403). Stevie manually downloaded the JSON and uploaded it, which worked but is not sustainable as policy.
2. **The orphan `docs/legal-planning` branch** (audited and verdict KILL in this same sprint) contained a TODO for a "CVE poller service (weekly/daily/hourly configurable)" but never made it to main.

Stevie's directive synthesizes the lesson: **threat intel feed retrieval must be a first-class always-on Kingdom service**, not a manual ritual every Round Table.

This ADR also takes the opportunity to make this service the **first cross-host K8s pilot** in the Kingdom. ADR-040 (K8s strategy) and ADR-047 (K8s honest assessment) already exist as planning docs; what we lack is a real, small, low-risk service to actually run on K8s across WEST + EAST and learn from. The KEV poller is exactly that:

- Tiny scope (one HTTP fetch, one parser, one publisher)
- Idempotent (re-running causes no harm)
- Cross-host failover is a natural exercise (one host pulls; other validates)
- Failure mode is graceful (stale data is annotated, not catastrophic)
- Output is a downstream-consumer-friendly file in the repo

---

## Decision

### 1. Always-on service requirement (HARD)

A service named **`kev-poller`** MUST be deployed and continuously running across the Kingdom from the moment this ADR ships. It MUST:

- Pull `https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json` on a configurable schedule (default: **hourly**; tunable to `15m / 1h / 6h / 24h` via service config)
- Stamp each pull with timestamp + HTTP response metadata
- Publish updates to **Wotan topic `threats.cisa-kev.updates`** (per Wotan message bus convention)
- Persist the latest snapshot to a Kingdom-known path (`/var/lib/unheaded/threat-feeds/cisa-kev/latest.json` on each host running the service)
- Diff against the previous snapshot; emit a delta event on Wotan topic `threats.cisa-kev.delta` containing newly added CVEs
- Fail gracefully: HTTP errors → log + retry with backoff; never crash the service
- Expose Prometheus metrics: pull_success_total, pull_failure_total, last_pull_timestamp, kev_total_count, kev_delta_count
- Expose `/health` and `/ready` endpoints per Kingdom convention

### 2. Cross-host K8s pilot framing

`kev-poller` is the **first production K8s workload** running across WEST + EAST bare metal (per ADR-047's "East/West K8s Lab"). It MUST:

- Run as a **K8s DaemonSet** initially (one instance per host) — simplest cross-host topology
- After 30 days of successful DaemonSet operation, optionally migrate to **K8s Deployment with leader election** (one active poller + N standbys) — the "real" production pattern for non-redundant work
- Use the existing K8s lab cluster (per ADR-047). If the cluster doesn't yet exist on WEST + EAST, this ADR's first phase is to bring it up.
- Deployment manifests live at `k8s/kev-poller/` (new directory). Helm chart optional Phase 2.

### 3. Threat-register auto-update integration

Per ADR-052 (drift policy), `docs/security/threat-register.md` carries a "Last refreshed" timestamp. The KEV poller MUST close that loop:

- After each successful pull, append a short refresh-log entry to `threat-register.md` via a Wotan-subscribed updater process (or a periodic job that consumes `threats.cisa-kev.delta` events)
- Format mirrors the 2026-04-27 manual entry format
- Only entries with non-zero stack-relevant deltas trigger automatic threat-register updates (to avoid spamming the file)

This keeps `threat-register.md` ≤ 7 days fresh by construction once the service is live, satisfying ADR-052's drift policy automatically.

### 4. NIST NVD parity

`kev-poller` is named for its primary feed but the service interface is generalized. **Phase 2** adds a sibling poller for NIST NVD, then GitHub Security Advisories. All share:

- Same DaemonSet/Deployment skeleton
- Same Wotan topic naming convention (`threats.<feed>.updates`, `threats.<feed>.delta`)
- Same Prometheus metric naming convention
- Same threat-register integration

The first instance is `kev-poller`; the *pattern* it establishes scales to 5–10 feeds.

### 5. Service home in the Kingdom hierarchy

Per Kingdom lore (Lore + Kingdom skills), this service maps to:

- **Pleroma layer**: not yet (Phase 3 if elevated to higher-level coordination)
- **Kenoma layer**: yes — it's a concrete service with a specific job
- **Armory placement**: under `services/sentinel-feeds/` (sub-directory of services/, where Sentinel-aligned feed-pulling services congregate). Sentinel is the natural primary consumer.
- **Norse name candidate**: **Munin** (one of Odin's two ravens — gathers knowledge from far places, returns it home). Apt for "fetch threat intel from afar, deliver to the Kingdom."
- Lore-keeper finalizes the name when the K8s manifest lands.

---

## Consequences

### Positive
- Compliance refresh stops being a manual ritual; threat-register auto-stays-fresh per ADR-052 by construction
- First real cross-host K8s workload in the Kingdom — turns ADR-040 + ADR-047 from planning docs into operational experience
- Generalizable: the service skeleton becomes the template for NIST NVD, GitHub Advisories, AMD Security Bulletins, etc.
- Failure-graceful: stale-data is annotated, not catastrophic; CISA outage doesn't break Kingdom posture
- Wotan topic integration teaches us cross-host pub/sub patterns at low risk
- Sentinel + MoatGhost get a structured, queryable threat feed instead of one-shot manual pulls

### Negative
- One more service to operate, monitor, alert on. Pull-side concerns: rate limiting, IP block, TLS cert pinning, gracefully handling CISA's CDN.
- DaemonSet across WEST + EAST means duplicate pulls (acceptable initially, optimize via leader election in Phase 2)
- Slight on-disk growth: each host accumulates KEV snapshots. Retention policy needed (default: keep last 30 days, prune older).

### Conditional
- **If Track A locked**: this ADR ships in Sprint May-Q1 alongside WAVE14 work. K8s pilot piggybacks on whatever GPU infrastructure the box already has.
- **If Track B locked**: this ADR ships earlier — the always-on threat feed makes the public-launch security posture defensible.
- **If Track C locked**: this ADR ships in the launch thread, since it's launch-relevant.
- **If Stevie's K8s lab on WEST+EAST isn't yet live**: ADR-055 first phase is "bring up the lab cluster" (per ADR-047 plan). Estimated +1 week delay.

---

## Implementation outline (Phase 0 → Phase 4)

### Phase 0 — K8s lab readiness
- Verify K8s cluster runs across WEST + EAST per ADR-047 plan
- If not running: bring it up (out of scope of THIS ADR but is the gating prerequisite)
- Document cluster bootstrap state in `runbooks/k8s/east-west-cluster-bootstrap.md`

### Phase 1 — `kev-poller` skeleton (Go service)
- New module: `services/sentinel-feeds/kev-poller/`
- Standard Kingdom service template: pkg/service/ scaffold, Wotan client, Prometheus metrics, /health + /ready
- Configuration: env vars `KEV_FEED_URL`, `KEV_PULL_INTERVAL`, `KEV_OUTPUT_PATH`
- Smoke-tested locally on a Linux dev box with `--once` flag (one pull, exit) for validation

### Phase 2 — K8s DaemonSet
- `k8s/kev-poller/daemonset.yaml`, `k8s/kev-poller/configmap.yaml`, `k8s/kev-poller/rbac.yaml`
- `k8s/kev-poller/servicemonitor.yaml` (Prometheus Operator integration)
- Deploy to East/West cluster, observe for 7 days

### Phase 3 — threat-register auto-updater
- New module: `services/sentinel-feeds/threat-register-updater/` (or a small Wotan subscriber inside `kev-poller`)
- Subscribes to `threats.cisa-kev.delta` topic
- For non-empty deltas: append a refresh-log row to `docs/security/threat-register.md` via git-commit-bot identity
- Marshal-citable per ADR-052 (this CLOSES the drift loop)

### Phase 4 — NIST NVD parity poller
- New module: `services/sentinel-feeds/nvd-poller/` (mirrors kev-poller skeleton)
- DaemonSet alongside kev-poller in same namespace
- Same threat-register integration

### Phase 5 — Migrate kev-poller from DaemonSet to Deployment + leader election (optional)
- After 30 days of DaemonSet stability
- Run as 2-replica Deployment with K8s lease-based leader election
- One pulls, others stand by
- Documented as a learning artifact: "DaemonSet → Deployment migration runbook"

---

## Progress

| phase | state (2026-10-19) | where |
|---|---|---|
| 0. K8s lab on WEST+EAST | **absent**: no k3s, kubelet or microk8s on west or east (checked 2026-10-19) | ADR-047 |
| 1. service | **built and tested**; `--once` against the live feed: catalog 2026.10.16, 1748 entries; second pull a 304; seeded with the 2026-04-27 archive it reports the 165 CVEs added since | `cmd/kev-poller`, `pkg/threatfeed/kev` |
| 2. DaemonSet | **manifest written, not applied** (no cluster) | `deploy/k8s/kev-poller/daemonset.yaml` |
| 2b. systemd | **installed and enabled on west and east, 2026-10-20** (always on, as `unheaded-kev-poller`; first pulls succeeded on both; scraped by monitoring/prometheus as job `kev_poller`, both up) | `deploy/systemd/unheaded-kev-poller.service` |
| 3. threat-register updater | **report, not a bot**: `--stack config/kev-poller/stack.yaml` tags added entries the Kingdom runs (KEV's own vendor/product names, exact match; categories follow the register), logs them as warnings, counts them (`unheaded_kev_stack_count`, `unheaded_kev_stack_added_total`) and marks them in delta messages; `kev-poller report --since DATE` renders the register's refresh entry. A person commits it: a bot with push rights to the repo is more than this needs. First refresh 2026-10-19 found FND-011 | `config/kev-poller/stack.yaml`, `pkg/threatfeed/kev/{stack,report}.go` |
| 4. NVD poller | open; **lower value than it looked** (2026-10-19): what NVD would add is already watched elsewhere: our Go, Rust and Python dependencies by govulncheck, cargo audit and pip-audit (CI, gating), container images by trivy and grype (CI), host packages by Ubuntu's own feed (`pro fix`, unattended-upgrades). KEV stays the high-signal feed: exploited, not merely published. Revisit if a component we run is in none of those | |
| 5. leader election | open | |

Built differently from the text above, and why:

- **Location.** `cmd/kev-poller` + `pkg/threatfeed/kev` (the repo's layout for
  binaries and libraries), not `services/sentinel-feeds/`. `k8s/` does not exist;
  manifests live under `deploy/k8s/` where the `pkg/uids` gate checks them.
- **Name.** `kev-poller`, not Munin: ADR-086 already took Muninn for the
  observability pipeline.
- **Port 19120** (`pkg/ports.KEVPoller`), uid 16732 (`pkg/uids`), host
  account `unheaded-kev-poller` (sysusers), Wotan display name `kev-poller`
  (auto-approved in `configs/wotan.yaml`).
- **Interval** is any duration from 15m to 24h (default 1h), from
  `--interval` or `KEV_PULL_INTERVAL`; `KEV_FEED_URL` and `KEV_OUTPUT_PATH`
  as specified.
- **Polite pulls.** Conditional GET (ETag / If-Modified-Since): an unchanged
  catalog costs a 304. https only, no redirect off https, 32 MiB body cap.
- **Validation before replacing the last good copy:** count matches the
  list, every cveID is `CVE-YYYY-NNNN+` and unique, no trailing data. A bad
  feed, an HTTP error or a write failure keeps the previous catalog; retries
  back off from 1m to the interval.
- **Storage.** `latest.json` (the bytes as published), `state.json`
  (sha256, version, count, validators), `snapshots/<UTC>-<sha8>.json` only
  when the content changes, pruned after 30 days (`--keep`), newest always
  kept. Writes are temp-file + rename.
- **Wotan.** `threats.cisa-kev.updates` on every good pull (version, count,
  sha256, changed, added/removed counts). `threats.cisa-kev.delta` only when
  something changed, in chunks of 50 entries (a backfill can be hundreds;
  Wotan bodies are capped at 1 MB), removed IDs in the first chunk. The first
  catalog a poller ever stores is a baseline: no delta.
- **Metrics:** `unheaded_kev_pull_{success,failure}_total`,
  `unheaded_kev_last_pull_timestamp_seconds`,
  `unheaded_kev_last_success_timestamp_seconds`, `unheaded_kev_total_count`,
  `unheaded_kev_delta_count`, plus `unheaded_http_*`. Alerts
  `KEVFeedStale` (6h), `KEVFeedExpired` (7 days, ADR-052), `KEVPollerDown`
  in `monitoring/prometheus/rules/threat-feeds.yml` (promtool-tested), scrape
  job `kev_poller`.
- `/ready` is 503 until a valid catalog is stored (pulled, or loaded at
  start); `GET /api/v1/kev` shows the stored state, the last error and the
  next pull.

To make it always on (Stevie: host change):
`go build -o kev-poller ./cmd/kev-poller && sudo install -m 0755 kev-poller /usr/local/bin/`,
then `sudo systemd-sysusers` with the repo's `deploy/sysusers.d/unheaded.conf`,
install the unit, `systemctl enable --now unheaded-kev-poller`. Wotan
needs a restart to pick up the new auto-approve entry.

---

## Alternatives considered

1. **Continue manual pulls per Round Table**. Rejected — Stevie's directive explicitly requires always-on; manual is what failed in this Round Table when the proxy 403'd.
2. **Cron job on each host (no K8s)**. Acceptable as a v0; deliberately rejected because it doesn't advance the K8s ladder. Cron job would solve the immediate problem but waste the pilot opportunity.
3. **Single host (just WEST or just EAST)**. Rejected because it loses cross-host failover and doesn't validate the K8s thesis.
4. **Use an off-the-shelf KEV scraper (e.g., Rapid7's KEV-CLI)**. Rejected because Kingdom convention is to own threat-intel ingestion; bringing in a 3rd-party tool adds dependency surface for trivial functionality. The whole pull is one HTTP GET + one JSON parse.
5. **Defer to Sprint May-Q3+ as a low-priority polish item**. Rejected because Stevie said *MUST ALWAYS be running*. That's a hard requirement, not a polish.

---

## Acceptance criteria

This ADR is "Planned" until each of the following lands. It flips to "Accepted" when:

- [ ] Phase 0 verified (K8s cluster exists on WEST + EAST, or Phase 0 sub-plan is in-flight)
- [x] Phase 1 service runnable as `kev-poller --once` and passes smoke test (2026-10-19)
- [ ] Phase 2 DaemonSet deployed; 24h of clean operation observed
- [ ] Phase 3 threat-register-updater closes the auto-refresh loop (2026-10-19: `kev-poller report` renders the entry; committing it is still a person's step)
- [ ] ADR-INDEX updated: ADR-055 status Planned → Accepted
- [ ] Phase 4 (NIST NVD) added to backlog as ADR-056 (separate ADR; same pattern)
- [x] CLAUDE.md updated with kev-poller in service inventory + port allocation in The Doom Range (port registry table, 2026-10-19)

---

## Sign-off (Phase 0 → Acceptance)

- [ ] Stevie — initiating directive captured; will sign off on Phase 2 deployment
- [ ] Architect — DaemonSet placement + Wotan topic naming approved
- [ ] Computermancer — K8s manifests reviewed
- [ ] Developer — Go service skeleton reviewed (security-first per Kingdom convention)
- [ ] MoatGhost — threat-register integration acceptable
- [ ] Sentinel — primary consumer; subscribes to delta topic
- [ ] Marshal — ADR-052 drift loop closure confirmed
- [ ] Lore-keeper — Norse name (Munin / alternatives) finalized
- [ ] RFC-Editor — ADR text reviewed; promoted Planned → Accepted

---

## Naming notes (Lore reservation)

- **Munin** (knowledge raven; pairs with Hugin for "thought") — primary candidate, fits "fetches knowledge from afar"
- **Sleipnir** (8-legged horse, fast traveler) — already used in ADR-69420 for BGP, conflict
- **Geri / Freki** (Odin's wolves) — alternate raven-equivalents; less apt for fetching
- **Heimdall** (the watchman) — already used in `cmd/heimdall-daemon` for drift detection, semantic conflict
- **Mímir** (well of wisdom) — already partially used for Mímir's Law (Gleipnir Phase 0)

Pool reserves: **Munin** is the front-runner. Lore-keeper finalizes when `services/sentinel-feeds/<name>-poller/` directory is created.

---

*ADR-055 forged 2026-04-27 from Cowork-on-Macbook capturing Stevie's directive. Planned status; activates whenever Track decision unblocks the K8s lab work. The Kingdom's first ravens take flight.*
*<3 The Munin remembers. <3*
