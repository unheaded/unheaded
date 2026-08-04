<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Staging Ladder — the decision queue

Phase 15. **These are not churn.** Each needs a call that an unattended run should
not make. The job here was to make each decision cheap to make — options costed,
recommendation stated, consequence named — not to make it.

Ordered by how much is blocked behind them.

---

## Decision procedure (Stevie, 2026-08-04)

Where a decision below is blocked only on Stevie's input, it may instead be settled by a
**2/3 vote of `unheaded-developer`, `unheaded-architect`, `unheaded-micromanager`**. Items
blocked on something else — account access (D9), live hardware (D8), a live-traffic
availability trade (D2) — are not unblocked by a vote and stay queued.

**First use — where unattended commits land. Unanimous 3/3: stay on `staging`.**

- *Developer* ("git is truth"): these fixes edit files whose current content exists only on
  `staging`. Basing them on `develop` at `b39fb207` means patching text that isn't there.
- *Architect* ("architectural decisions are irreversible"): re-parenting 46 commits is a
  history rewrite Stevie owns. Zero benefit while he is asleep, real blast radius.
- *Micromanager* (one-reviewable-unit-per-commit): what gets reviewed is a linear stack of
  self-contained commits. The branch *name* is not the deliverable.

**Consequence — first agenda item for the in-person session:** `develop` and `staging` have
diverged in the wrong direction for the flow Stevie described (`develop → staging → main`,
promoted commit by commit). Reconciling them is a branch-pointer move, and it is his.

---

## D1 — `BLE001`: 134 blind exception handlers

**Blocks:** removing the last rule-ID exclusion from the ruff ratchet.
**Full analysis:** `docs/security/exception-handling-triage-2026-08-03.md`

Of the 130 non-notebook sites, **71 are silent** — they catch everything, neither log
nor re-raise, and hand the caller a fallback with no record it happened. 59 already log.
**62 of the 134 are in `raft/zhen_app.py` alone.**

| Option | Cost | Result |
|---|---|---|
| **A — narrow the 71 silent ones**, annotate the 59 loud ones | Largest. A full attended session for `zhen_app.py` alone. | The bug class actually goes away. |
| B — make all 71 loud, `noqa` the rule to zero | Low, mechanical, safe unattended | Every swallow visible, but handlers still blind, and you'd be ratcheting a rule you suppressed |
| C — exclude by rule ID, leave the code | None | Nothing improves, but the backlog is recorded honestly |

**Recommendation: C now (done — the ratchet ships with `--ignore BLE001`), A later**, as
the first real exercise of the ADR-090 sweep, since 62 of them live in one file that
sweep will read line by line anyway.

**Explicitly not B.** Adding logging to all 71 and calling the rule clean suppresses 134
findings behind annotations — the exact pattern the ratchet policy forbids for gitleaks
and gosec.

> Narrowing a handler is the only change in this whole ladder that can turn a currently
> working path into a crash. It needs someone who can say "yes, that endpoint is allowed
> to 500."

---

## D2 — `shield-ebpf` PQC fast path has neither producer nor consumer

**Carried over from the previous handoff. Unchanged — nothing in this run touched it.**
**Options:** `docs/security/shield-ebpf-pqc-fast-path-unwired-2026-08-03.md`

Nothing calls `pqc_fast_path_check`; nothing writes `PQC_SIG_STATUS`. The XDP-layer PQC
enforcement that appears to exist has never executed once.

Wiring it changes the verifier budget **and** forces a choice on cache hit:

- **PESSIMISTIC** — drop. Fails closed. An unverified packet never passes; a bug in the
  cache path becomes an outage.
- **OPTIMISTIC** — warn. Fails open. Never causes an outage; an unverified packet can
  pass while you are reading the log.

That is a live-traffic availability-vs-security trade against the actual WEST/EAST
topology. Not mine.

---

## D3 — `scripts/bpf-verifier-check.sh` computes a build result and never checks it

**CLOSED 2026-08-04 (`f80576ea`).** The blocking question was whether wiring it in turns
CI red on landing. It does not: `cd ebpf && cargo build --release` exits 0 with zero
`error[` lines, so the fix is verified inert on the current baseline. That made this a
decision with only one live branch, so it was taken rather than queued.

`BUILD_EXIT` is now the authoritative signal — it is non-zero for every failure mode,
including the ones neither grep matches (`error:` with no code, a panicking build script,
a malformed `Cargo.toml`). `ERRORS`/`LINK_ERRORS` were demoted to selecting a diagnostic,
with the build tail printed when neither matches so a failure can never be silent.

Both paths exercised: real tree → `GATE: PASSED`, exit 0; a `cargo` stub exiting 101 →
`GATE: FAILED`, exit 1. Before the fix that same stub produced `GATE: PASSED` — that is
the regression this closes. `ascend-linux` rebuilt afterwards: 901,888 bytes, unchanged.

---

## D4 — bandit `B310`: 35 `urlopen` call sites

**Blocks:** flipping bandit to gating.

The underlying risk — `urlopen` accepting `file://` — **is already fixed at the boundary**
(`71f43a11`): `zhen_app.py`'s env-derived service URLs are scheme-checked once on entry
and raise otherwise. Verified: `file:///etc/passwd` rejected, http/https unchanged.

The 35 call sites are all downstream of that guard. Skipping the rule on that basis is
defensible — but it is a security judgement about whether the boundary is the *only*
entry point, not a lint call.

**Recommendation:** skip by rule ID once you have satisfied yourself that no other code
path constructs a URL for those calls from outside the guard.

---

## D5 — bandit `B104`: bind-all-interfaces, 2 sites

`raft/zhen_app.py:2440` runs `app.run(host='0.0.0.0', port=20103)`; `vault-to-runway.py`
has a second. Changing a bind address is a **real exposure change that can break a
deployment**, and has to be decided against the actual WEST/EAST topology and lxdbr0
layout rather than in the abstract.

---

## D6 — bandit `B615`: unpinned HuggingFace revision, 2 sites

`raft/scripts/08_train_qlora.py` pulls a model without pinning a revision. Pinning is
correct supply-chain hygiene and **changes which weights get fetched** — so it has to name
the *right* artifact. Relevant history: the Gemma-4 GGUF was deleted on 2026-07-31 for
being an 8.7 GiB quant rather than the 3.2 GB Q4 the plan specified. Pinning the wrong
thing here reproduces that.

---

## D7 — `defusedxml` as a dependency? (ADR-004) — **ready to apply, 2 minutes**

`12_process_stackoverflow.py` and `14_extract_wikipedia.py` stream-parse the StackOverflow
and Wikipedia dumps with `xml.etree`. XXE does not apply (`xml.etree` does not resolve
external entities), but entity-expansion DoS does — impact being that a batch job you
started crashes.

**Everything that was uncertain here has now been measured (2026-08-04).**

The **3-skill vote is unanimous for approval**: *Developer* — all inputs hostile, and the
amplification is real; *Architect* — ADR-004's "established orgs OK with approval" fits
exactly (PSF licence, **zero transitive dependencies**, author Christian Heimes,
`christian@python.org`, a CPython core developer); *Micromanager* — two import lines,
verified, so cheap that deferring costs more than doing.

Evidence, run rather than assumed:

- **`defusedxml` supports `iterparse`**, which was the open question — both call sites use
  it. `defusedxml==0.7.1` provides it with `forbid_entities=True` by default.
- **Identical on well-formed input.** Same file, both parsers: 3 elements, 10 body bytes.
- **The amplification is real.** A billion-laughs DTD at only `lol5`: stdlib expanded a
  ~500-byte file to **300,000 bytes**; `defusedxml` raised `EntitiesForbidden`. `lol9` is
  gigabytes.
- Both files use *only* `iterparse`, so `import defusedxml.ElementTree as ET` and
  `from defusedxml.ElementTree import iterparse` are clean drop-ins. (`defusedxml` does
  not re-export `Element`/`SubElement`/`ElementTree` — neither file uses them.)

**Why it is not already committed:** `defusedxml` is not installed in `~/.venv/zhen`, the
environment those scripts run in. Making the swap without it turns a theoretical DoS on
trusted archives into an immediate `ModuleNotFoundError` on every run — introducing the
failure the change claims to prevent. `raft/requirements.txt` now exists (`85466119`), so
there is finally somewhere to declare it, which is what blocked this before.

**To apply:**

```bash
~/.venv/zhen/bin/pip install defusedxml==0.7.1
# then add to raft/requirements.txt, and in the two scripts:
#   12_process_stackoverflow.py: from xml.etree.ElementTree import iterparse
#                             -> from defusedxml.ElementTree import iterparse
#   14_extract_wikipedia.py:     from xml.etree import ElementTree as ET
#                             -> import defusedxml.ElementTree as ET
```

Keep the new import on its own line after a blank line, or ruff's `I001` will fire on the
import block.

---

## D8 — trivy KSV-0014 (5 HIGH), KSV-0041/0046 (2 CRITICAL)

Carried over. Need a live kind cluster to verify a fix; cannot be closed from a laptop.

---

## D9 — GitHub repo settings

ADR-089 is **aspirational server-side until this is done**, and enforced only by local
convention:

- default branch → `develop`
- branch protection on `main`: no direct pushes, signed commits required
- CI already runs on `main`/`develop`/`staging`, so nothing is needed there

Needs your account. Everything else in ADR-089 is in force already.

---

## D10 — ADR-090's three open questions

1. **Vendored code scope** — `llama.cpp/` and `crates/xv6-mbc/upstream/` are skipped by
   default. Confirm. (xv6 upstream may be special: Phases 2.2-2.4 have been progressively
   replacing it with Unheaded-authored code.)
2. **`docs/`** — 345K lines, larger than any code surface. In scope for a separate sweep,
   or out entirely? ADR-090 covers source only.
3. ~~**The 28 `#[ignore]`d `zhenai-forge` tests**~~ — **the premise was wrong, closed
   2026-08-04.** They are not "neither running nor removed": every one carries a reason
   and a way to run it, e.g.

   > `#[ignore] // heavy: loads ~9 GB Gemma-4 GGUF + uploads to GPU — OOM risk on 14 GB dev box; run on east/west or via cargo test -- --ignored`

   That is a deliberate resource guard, not abandonment — and the other **104 tests in
   the crate pass**, which nobody could have known, because *no CI job ran this crate at
   all* until `22f180da`. Nothing to decide: keep them, run them on east/west.

---

## D15 — `omitempty` on struct fields: fix all 66, or leave them?

**Found 2026-08-04**, while fixing `services/timeguru` (`081e5152`).
**Full analysis:** `docs/security/omitempty-on-structs-2026-08-04.md`

`encoding/json` never omits a struct value, so `time.Time` tagged `omitempty`
always serialises — an unset date emits `"0001-01-01T00:00:00Z"`. **66 fields
across 24 files** state an intent the encoder silently ignores. 55 are
`time.Time`; 11 are whole nested config structs that emit a populated tree of
zero values.

**Not a logic fault.** Go code guards these with `.IsZero()`, which is correct.
It only shows at the serialisation boundary, which is why every test passes.
`Secret.IsExpired()` and `Lease.IsExpired()` were both checked specifically and
are fine.

| Option | Cost | Result |
|---|---|---|
| **A — convert all 66 to pointers** | Large. ~100 `.IsZero()` call sites become nil checks, package by package, `-race` after each. | The APIs stop emitting year-1 dates |
| **B — drop the misleading `omitempty` tags** | Small, mechanical | Output unchanged, but the code stops claiming something untrue |
| **C — leave, record** | None | Honest backlog; consumers keep seeing `0001-01-01` |

**Recommendation: A, one package at a time, attended.** Not unattended — the
conversion has a real hazard that bit within a minute in timeguru:
`time.Time.IsZero` has a *value* receiver, so `!x.IsZero()` on a nil pointer
panics. The existing tests caught it there; they may not everywhere.

Highest-value packages first, on the basis of who reads the JSON:
`cmd/dashboard-backend`, `cmd/kanban-app`, `pkg/alerting`, `pkg/secrets`.

---

## D14 — should `rust-audit` and `security-scan` block a merge?

**Found 2026-08-04.** `ci-protocol.yml`'s `ci-gate` depends on six of its eleven
jobs and printed `✅ All CI checks passed — ready to merge`. The five it omits:

| job | what it does | advisory? |
|---|---|---|
| `security-scan` | Go Security Scan | **no** — real steps, no `continue-on-error` |
| `rust-audit` | Rust Security Audit | **no** |
| `proto-lint` | Proto Lint | no |
| `integration-test` | its own gate; nothing needs it | no |
| `benchmark` | PR-only | no |

None is marked advisory, so each can fail while the gate still reports success.
**The message is fixed** — it now enumerates what it aggregated and names what it
did not, matching `security.yml`'s `security-gate` and `ci.yml`'s `ci-gate`, both
of which already did this.

**The decision is whether the two security jobs should move into `needs`.** Not
taken here for the D3 reason: adding a job to `needs` makes it blocking, and
whether `security-scan` and `rust-audit` currently pass **cannot be verified from
this machine** — they need a GitHub runner. Wiring them in blind is exactly how
you land a red gate.

Cheap to settle: push the branch once, read those two jobs, then add them to
`needs` if green. Related to **D9** — until branch protection exists, no gate is
enforced server-side anyway, so this is about the message being truthful more
than about enforcement.

---

## D13 — `lxd/` predates the Port Authority migration, wholesale

**Found 2026-08-04.** The LXD container definitions use an entirely different port
scheme from every other deployment surface — 50051–50067 for the services,
plus 8080 (dashboard-backend), 8443 (gateway), 3001/3002 (frontends). Only
`doom.yaml` (16680) is in the Doom Range.

It is **internally consistent**, which is why this is a decision and not a bug:
`lxd/` is a coherent pre-migration world, not a tree with stragglers. Fixing one
file would make it inconsistent with the other nineteen.

Found while chasing `GATEWAY_PORT`, which is read by nothing anywhere —
`services/gateway/config/config.go:216` reads **`GATEWAY_HTTP_PORT`**. Three
places set the wrong name:

| where | value | effect |
|---|---|---|
| `kubernetes/.../gateway/deployment.yaml` | 21000 | **fixed** — renamed; the value already matched containerPort, both probes and the Service, so it was dead but harmless |
| `lxd/containers/gateway.yaml` | 8443 | still wrong name. Renaming would make 8443 *take effect*, diverging from the Doom Range 21000/21443 — so it needs the migration decision below, not a rename |
| `docker/hosts/host-b/docker-compose.yml` | 8080 | same, and that stack cannot start anyway (D11) |

**The decision:** migrate `lxd/` to the Doom Range, or declare it a deliberately
separate scheme and document why. Either is defensible — LXD containers get their
own IPs, so the ports need not match the Doom Range to avoid collisions — but
right now nothing says which it is, and `pkg/ports/ports.go` claims to be the
single source of truth for the whole Kingdom.

Not taken unattended: it is 20 files of port changes across a deployment surface
that cannot be tested from here.

---

## D12 — two parallel Kubernetes trees, neither declared canonical

**Found 2026-08-04.** `deploy/k8s/` (2026-03-05, "SK8 Convergence") and
`kubernetes/` (2026-06-26, "Kubernetes the hard way") are both live — the most
recent commit touching Kubernetes at all, `7153f47a`, touched both.

**Nothing is broken.** Verified: **zero overlapping `(kind, namespace, name)`
tuples**, and they use disjoint namespaces, so applying both to one cluster does
not collide. This is a duplication-of-effort question, not a defect, which is why
it is here and not fixed.

| | `deploy/k8s/` | `kubernetes/` |
|---|---|---|
| Shape | raw manifests by Kingdom tier | kustomize `base/` + `overlays/` |
| Namespaces | `unheaded-armory/-gnostic/-presentation/-system/-ebpf` | `unheaded`, `haproxy-controller` |
| Has | Gatekeeper `ConstraintTemplate`s, `CiliumNetworkPolicy`, PDBs, `ServiceMonitor`s | service-for-service mirror of the Docker stack, HAProxy ingress edge |
| Own docs | 1 | 7 |
| Cited by other docs | 12+ — ADR-064, runbooks, compliance control matrices, K8s threat model | 1 |

Consolidating loses something either way: `kubernetes/` has the structure and the
documentation, `deploy/k8s/` has the policy layer and every external reference.
The 3-skill vote came out 2/3 for **documenting, not consolidating** —

- *Architect*: kustomize with base/overlays is the industry-standard shape, and
  ADR-088's whole premise is practising industry-standard substrates. Would make
  `kubernetes/` canonical.
- *Developer*: `deploy/k8s/`'s governance layer has no equivalent in `kubernetes/`.
  Picking a winner now deletes real work. Merge later, don't choose now.
- *Micromanager*: two parallel implementations is a maintenance cost, but neither
  is broken and both are referenced — not an unattended call.

**Done instead:** a cross-reference README in each tree, so nobody has to rediscover
that the other exists. Neither is declared canonical.

**If you do consolidate**, the migration is "port `deploy/k8s/`'s policy layer onto
`kubernetes/`'s kustomize base, then re-point the 12 external references" — the
references are the expensive half, and ADR-088 should record the outcome.

---

## D11 — `docker/hosts/host-{a,b}` have never been able to start

**Found 2026-08-04.** Both stacks are unusable, and have been since the files were
created (`695d0ba4`, 2026-02-26). The LXD path for this tier is complete and
works — `lxd/containers/{bird,ipfire}.yaml`, `lxd/profiles/unheaded-firewall.yaml`.

**host-a — invalid compose.** `shield` and `unheaded-daemon` each declare *both*
`network_mode: host` and a `networks:` block with a static IPv6. Mutually
exclusive, so `docker compose config` refuses the whole project — it does not
parse, let alone run.

**This needs you, and it is the D5 class: an exposure decision.** Both readings
are defensible and neither is safe-by-default:

| keep | argument | cost |
|---|---|---|
| `network_mode: host` | `shield`'s `cap_add: [BPF, NET_ADMIN, SYS_ADMIN, SYS_RESOURCE]` exists to attach XDP, and XDP inside a container netns sees only the veth, not the host NIC. Packet-zero visibility is the point of Shield. | Breaks `NATS_URL: nats://wotan:4222` — no Docker DNS in the host namespace — and makes every `ports:` mapping meaningless. Puts both services directly on the host's interfaces. |
| `networks:` + static IPv6 | Docker DNS resolves, `ports:` means something, services stay on the fabric at `fd00:dead:beef:1::201/202`. | Shield cannot do the one job the capability set was granted for. |

The 3-skill vote split — Architect for host mode on architectural grounds,
Micromanager against deciding exposure unattended, Developer noting neither is
verifiable without standing the stack up. **Documented, not changed.** Nothing
regresses by leaving it: it has never run.

**Both hosts — missing build contexts.** `opnsense`/`frr` (host-a) and
`ipfire`/`bird` (host-b) build from `../firewall/*`. **`docker/hosts/firewall/`
has never existed in this repo** — `949ed857` added the compose files claiming
the capability without ever adding the build contexts. compose builds every
`build:` service before starting anything, so these take the whole stack down
with them, including host-b's `suricata`, which is otherwise fine.

`routing/frr/Dockerfile` and `routing/bird/Dockerfile` do exist and look like the
answer. **They are not.** Both do `COPY . /src/<name>` and build the daemon from
upstream source — their own comments say `~/tmp/frr-master/` and
`~/tmp/bird-master/` — so they need a context holding that source tree, not the
config directory. Repointing the contexts would swap one failure for another.
No equivalent exists anywhere for `opnsense` or `ipfire`, which are KVM appliance
images.

**Fixed in passing** (same off-by-one as the `suricata` rules mount, and both
target files exist): `${FRR_CONF:-./../../routing/frr/frr.conf}` and
`${BIRD_CONF:-./../../routing/bird/bird.conf}` were resolving under `docker/`
rather than the repo root. Now `../../../`.

---

## Two smaller ones — both CLOSED 2026-08-04

- **`tomb/provision.sh --verbose` and `scripts/pre-flight-check.sh --strict`** — wired up
  (`8724db32`) rather than dropped from `--help`, since in both cases the advertised
  behaviour is the useful one. `_log` mirrors to stderr under `VERBOSE=1` (one site covers
  every SSH/SCP trace); `--strict` folds `optional_failed` into the blocking count, and its
  case arm — which never even set `STRICT_MODE` — now does. Default behaviour unchanged:
  of the eight `(strict, required, optional)` combinations exactly one verdict differs, and
  only with `--strict` passed.

- **`scripts/doom-test.sh` `${pixel_8000}`** — **the previous entry here was wrong**
  (`cb9496db`). `git blame` shows `c7831cad` (2026-03-03) assigns it at line 624; the read
  has worked for five months. What was actually broken is that `c7831cad` left the `local`
  line naming the *old* `pixel_32000`, so `pixel_8000` leaked into the caller's scope on
  every invocation. `ff8d090a` deleted the stale name correctly and then drew the wrong
  conclusion about the one that replaced it. Fixed by declaring it; no output change.

  Worth generalising: that note asserted a runtime symptom (`always renders as '??'`) from
  reading a declaration line alone. Cheap to check, and `git blame` on the *assignment*
  would have caught it. Treat "flagged, not fixed" annotations from the ladder as claims
  needing verification, not as findings.
