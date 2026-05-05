# WAVE17 — Overnight K8s Substrate Bring-Up

**Date:** 2026-05-05 (commencing)
**Owner:** unheaded-warmonger (battle plan), unheaded-marshal (lane enforcement)
**Status:** EXECUTING (unattended overnight)
**Trigger:** Stevie's directive 2026-05-05: *"churn all night unattended - have /unheaded-marshal lead sprint"* — building on his earlier note that *"we should get k8 stuff running and use it to swap from active/passive to active/active with near 0 downtime"*.

---

## Mission

Wake-up deliverable: a kind 3-node cluster + the existing Unheaded helm chart deployed locally, with at least the Wotan + Timeguru + dashboard-backend pods passing health probes. The goal is the *substrate* for the eventual ADR-064 active/active migration; we are NOT implementing ADR-064 itself tonight.

Per `feedback_overnight_churn_pattern.md` (saved 2026-05-05 from WAVE16): plan-in-repo, phase-by-phase commits, restore-state-at-end, wake-up summary doc.

---

## Phases

### Phase 0 — Pre-flight ✓ (completed 2026-05-05 ~06:10 UTC)

- kind/kubectl/helm/docker installed and working
- 9 of the 10 service images already in local docker daemon as `unheaded-dev-*:latest`
- helm chart lints clean (991 lines emitted, 9 Deployments + 9 Services + 1 Namespace + 3 NetworkPolicies)
- No live kind cluster (clean slate)

### Phase 1 — Cluster + image loading ✓ (this commit batch)

- `deploy/k8s/kind/cluster-config.yaml` — 3-node config (1 control + 2 workers); NodePort mappings for wotan HTTP+gRPC
- `deploy/k8s/kind/values-local.yaml` — helm overrides pointing at the local `unheaded-dev-*` images
- `deploy/k8s/kind/bring-up.sh` — idempotent script: kind delete → kind create → image-load → helm-install
- 3-node kind cluster up; all 9 unheaded-dev-* images loaded

### Phase 2 — First helm install attempt + triage

Run `helm upgrade --install` and capture pod state. Expected outcome: most services come up, some don't (e.g. services that need PostgreSQL won't have one in the kind cluster yet). Whatever fails is data; document it.

### Phase 3 — Triage failing services

For each pod stuck in CrashLoopBackOff / Pending / ImagePullBackOff:
1. Check pod events + logs
2. Determine root cause (missing dep / missing config / image mismatch)
3. Either fix in values-local.yaml OR document as known-gap

The bar is **partial success acceptance** — we want the substrate working for ≥3 services, not all 9 at once.

### Phase 4 — Smoke probes + integration test

For services that come up, smoke their /health from the host:
```bash
curl -s http://127.0.0.1:30080/health    # wotan via NodePort
kubectl exec -n unheaded ... -- curl localhost:18000/health
kubectl logs -n unheaded -l app=wotan --tail=20
```

Capture every output to a results doc under `eval/k8s-bringup/<date>/`.

### Phase 5 — Document the runbook

Write `runbooks/infra/local-k8s-bringup.yaml` so an adopter (or future-Stevie) can run this exact recipe with one command. Include teardown.

### Phase 6 — Cluster torn down OR left running

Default: tear down at end so wake-up state is clean. The bring-up script is idempotent, so re-running tomorrow is one command.

If during the run anything looks promising enough to keep, leave running and note in wake-up summary.

### Phase 7+ — If time permits, additional churn from the queue

Pickable autonomous-safe items (per the kanban + round-table):

- Audit any other doctrine-violating files outside the round-table's scope
- Update `references/timeline.md` Age 3 with WAVE17 entry
- File `air-gap-egress-validation` runbook stub (referenced from Zhen On-Prem but doesn't exist yet)
- Verify the `runbooks/cluster/wotan-active-active-cutover.yaml` referenced from ADR-064 exists or stub it
- Look for any low-hanging ADR-INDEX cleanups (date refresh, status sync)

---

## Marshal halt protocol

**HARD HALT** triggers (any one):
- kind cluster fails to come up after 2 retries (deeper docker/system issue)
- helm install corrupts state requiring manual `helm uninstall` + namespace cleanup beyond what the script handles
- The 9-service stack on Stevie's host (postgres, llama-server, vor, wotan-bare-metal, etc.) regresses — i.e. anything we touch breaks the pre-existing live operator surface
- gpg-agent timeout (per `feedback_unsigned_commits_when_afk.md`: use `--no-gpg-sign` and continue, don't halt)

**SOFT REDIRECT** triggers:
- A service won't come up despite triage — abort that one service, log the gap, move on
- A phase exceeds 30 minutes wall-time — abort that phase, log overrun, move on

On HARD HALT: leave a halt note at the top of `WAVE17-OVERNIGHT-K8S-SUBSTRATE.md`, run `kind delete cluster --name unheaded` to clean state, exit cleanly.

---

## Restore-at-end checklist

Before writing the wake-up summary:
- [ ] kind cluster torn down (default) OR left running with explicit note
- [ ] Live operator surface still 9/9 services green (postgres, llama-server, vor, wotan, shield, dashboard-backend, kanban-app, wiki-server, zhen_app, zhen-agentd)
- [ ] llama-server still serving qwen2.5-coder-7b-instruct (the canonical default)
- [ ] zhen_app reports the same model

---

## References

- `docs/adr/ADR-040-kubernetes-ecosystem-strategy.md` — K8s positioning
- `docs/adr/ADR-064-wotan-active-active-cluster-k8s-native.md` — what tonight's substrate enables (deferred implementation)
- `helm/unheaded/` — the chart being installed
- `feedback_overnight_churn_pattern.md` — the recipe this run follows
- `WAVE16-OVERNIGHT-MODEL-VETTING.md` — the predecessor run; same pattern, different target
