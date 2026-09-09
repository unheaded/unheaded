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

**Found this run.** The script runs `cargo build --release`, captures `BUILD_EXIT=$?` and
a count of `^error[` lines, then reads **neither**. Only `LINK_ERRORS` feeds `FAILURES`.

**A BPF program that fails to compile with an ordinary `error[E0433]` leaves this gate
reporting success.**

Both variables were kept and annotated rather than deleted — they are the only remaining
evidence the check was intended.

**Why it is not already fixed:** wiring it in can turn CI red the moment it lands, and
whether that is acceptable depends on whether anything currently fails to build. Cheap to
find out (`cd ebpf && cargo build --release 2>&1 | grep -c '^error\['`), but the
consequence of a red gate is yours to accept.

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

## D7 — `defusedxml` as a dependency? (ADR-004)

`12_process_stackoverflow.py` and `14_extract_wikipedia.py` stream-parse the StackOverflow
and Wikipedia dumps with `xml.etree`. XXE does not apply (`xml.etree` does not resolve
external entities), but entity-expansion DoS does — impact being that a batch job you
started crashes.

`defusedxml` closes it and is the textbook answer. It is a new third-party dependency,
which ADR-004 requires approval for. **Deliberately left unskipped in the bandit config**
so it stays visible: this is the one group in the remainder where a real fix exists and is
blocked only on a dependency decision.

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
3. **The 28 `#[ignore]`d `zhenai-forge` tests** — restore (needs the right Gemma-4 GGUF
   re-acquired) or delete? They are currently neither running nor removed, which is the
   worst of both.

---

## Two smaller ones, for completeness

- **`tomb/provision.sh --verbose` and `scripts/pre-flight-check.sh --strict`** are both
  documented in usage, parsed into a variable, and never read. Either wire them up or drop
  them from `--help`; right now the help text is lying.
- **`scripts/doom-test.sh` prints `${pixel_8000}`**, which nothing assigns — that
  SCREEN_MAP diagnostic has always shown `??`. The declaration named `pixel_32000`, so a
  third sample read was intended and never written.
