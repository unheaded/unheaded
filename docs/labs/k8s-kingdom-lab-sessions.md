# K8s Mock-Prod Lab — Session Ladder

**Companion to:** `docs/labs/k8s-kingdom-lab.md` (scope) · `docs/labs/k8s-kingdom-lab-plan.md` (numbered steps)

This is the calendar. Each row is one sit-down session. Time budgets include reading + typing + verification + extraction notes (NOT including breaks, debugging, dependency hell).

**Pace assumption:** 5h max per weekend session, 2-3h per evening session. Total lab = ~22h of focused work. Realistic calendar = **3-4 weeks** at a moderate clip.

---

## Recommended Cadence

| # | Session | Phases | Duration | When | Output |
|---|---------|--------|----------|------|--------|
| 1 | **Foundation** | Pre-flight + P0 + P1 (KTHW walkthrough) | ~5h | Sat AM | working single-cp cluster, 2 workers Ready |
| 2 | **HA + CNI** | P2 (HA control plane) + P3 (Cilium) | ~3h | Sat or weekday eve | 3-member etcd, kube-proxy gone, Hubble UI |
| 3 | **Storage + Ingress** | P4 (Longhorn) + P5 (cert-manager) | ~2.5h | Weekday eve | persistent volumes + auto TLS |
| 4 | **Observability + GitOps** | P6 (Prom/Loki) + P7 (Argo) | ~3h | Sat AM | dashboards live, Argo reconciling |
| 5 | **Real Workload** | P8 (Online Boutique) + P9 (NetPol+RBAC) | ~2h | Weekday eve | storefront live, default-deny + RBAC |
| 6 | **Day-2 Drills** | P10 + P11 (chaos drills) | ~3h | Sat AM | muscle memory + DR procedures written down |
| 7 | **Comparison** | P12 (write `k8s-vs-unheaded-notes.md`) | ~1h | Weekday eve | the doc you'll cite for years |
| 8 | **Operator** | P13 (custom operator, OPTIONAL) | ~3h | Sat AM | working `BaselineManifest` CRD |

**Min path (no operator):** Sessions 1-7 = ~20h over ~3 weekends.
**Full path (with operator):** Sessions 1-8 = ~23h over ~4 weekends.

---

## Pre-Session Routine (do every time)

Two minutes. Always.

1. `~/tmp/unheaded/scripts/k8s-lab-up.sh` — bring containers up if down (idempotent)
2. `kubectl get nodes` — confirm cluster from last session is still there (it should be — LXD persists state)
3. `free -h` on WEST + EAST — confirm RAM headroom for what you're about to do
4. Open scope doc + plan doc in two panes
5. Note start time in a session-N.md scratch file

If `kubectl get nodes` fails, you have a Phase 1/2 recovery problem — don't panic, debug systemd units on cp-1 first. Most likely: kube-apiserver not running because containerd is down because kubelet is sad.

---

## Post-Session Routine (do every time)

Three minutes. Always.

1. **Write extraction notes** for the phases you touched (in the plan doc)
2. **etcd snapshot** if you completed Phase 10 already (cheap insurance)
3. Decide: leave containers UP (cluster stays warm, ~9 GB RAM held) or DOWN (re-bootstrap next session)
   - **Up** is right for sessions <1 week apart
   - **Down** (`scripts/k8s-lab-down.sh --yes`) right if next session is >1 week off OR you need RAM for forge/Doom
4. Commit any docs/notes touched: `git add docs/labs/ && git commit -m "lab: session N notes"`

---

## Spike Daemon Coexistence

Mímir's Law spike daemons live on EAST at `/opt/spike-mimirs/`. The lab plan + scripts are spike-safe: lab containers use the `k8s-` prefix and never touch `/opt/spike-mimirs/`.

**However** — RAM pressure is real. EAST is 8 GB. Lab containers eat ~5 GB on EAST. Spike daemons eat another ~1 GB. Leaves <2 GB for kernel + LXD overhead. **This is tight.**

If you see EAST OOM-killer events during the lab:
1. Stop spike daemons temporarily: `ssh govan@east 'systemctl --user stop heimdall-daemon gjallarhorn-listener'` (verify exact unit names against `~/spike-snapshot.txt` from PF.4)
2. Continue lab work
3. Restart spike when lab is torn down

Never edit anything under `/opt/spike-mimirs/`. The down script will refuse to touch non-`k8s-` containers.

---

## When to Skip Ahead

If you only have ONE session and want maximum interview value:

- **Session 1 only** (foundation, no HA): you can answer "explain a K8s control plane" fluently. Worth ~5h.
- **Sessions 1+5** (foundation + sample app): you can run a real workload and answer "what does K8s do for you in practice." Worth ~7h, skips HA which is the slowest payoff.
- **Sessions 1+6+7** (foundation + chaos + comparison): you have the war stories AND the doc. Worth ~9h, the most-bang path if you can't do the full ladder.

Do NOT skip Phase 12 (comparison). The whole lab's ROI lives in that doc.

---

## When to Bail

If after Session 1 you find the LXD substrate is fighting you (networking flaky, snap LXD broken on EAST, IPv6 pain), bail to k3s on a single host:

```
curl -sfL https://get.k3s.io | sh -
```

Sessions 3-12 still work mostly the same on k3s. You lose the HA story (Session 2) and the cross-host failure-domain story, but you gain ~4 hours and a cluster that just works. Note the bail in the comparison doc — it's data ("when LXD-as-K8s-substrate breaks down").

---

*Session ladder — forged 2026-04-17 alongside scope + plan docs.*
