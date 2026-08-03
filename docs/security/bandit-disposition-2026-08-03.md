<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# bandit disposition — 2026-08-03

Phase 9/10 of the Staging Ladder. Baseline at `b39fb207` was **212**; it is **173** after
the `B110` findings cleared with Phase 8b's logging work and `B105`/`B108` were resolved.

This document dispositions the remainder **as groups**, because 137 of them are three
rules saying "you used subprocess" and annotating each site individually would add 137
lines of noise without adding a single bit of information.

---

## First: CI's bandit invocation violates the ratchet policy

`.github/workflows/static-analysis.yml` runs:

```yaml
- run: bandit -r . -ll -x ./llama.cpp,./target,./node_modules
```

**`-ll` is a severity filter.** It suppresses every LOW finding — currently 132 of the
173 — and the standing rule (ADR-073,
`docs/security/findings-remediation-2026-07-29.md`, and the header of the workflow itself)
is: *fix all severities; ratchet by rule-ID exclusion, never by severity filter.*

The job is also `continue-on-error: true`, so nothing has been gating either way. But when
Phase 14 flips it, `-ll` must go and be replaced by explicit rule-ID skips, each with a
reason — which is what the rest of this document supplies.

---

## Resolved (no longer present)

| Rule | Was | Disposition |
|---|---|---|
| `B110` try_except_pass | 28 LOW | **Fixed.** Phase 8b added logging to every silent handler. |
| `B105` hardcoded_password_string | 1 LOW | **False positive.** `PASS = "\033[92mPASS\033[0m"` is an ANSI status label; bandit matched the variable name. |
| `B108` hardcoded_tmp_directory | 10 MED | **Dispositioned per site.** All were fixed cross-process interface paths or test fixture strings — see commit `e4461566`. |

---

## Group dispositions

### `B603` (45) + `B607` (31) + `B404` (26) — "you used subprocess"

**102 of the 173.** This repository's entire job is driving other software: `cargo`, `go`,
`bpftool`, `ip`, `lxc`, `ssh`, `docker`, `rsync`, `dpkg`, `rocm-smi`. subprocess is the
correct instrument, and `B404` fires merely on *importing* it.

The questions that actually matter are two, and both were checked:

1. **Is any argument attacker-influenced?** No. Every call site builds its argument list
   from literals, repo-relative paths, or operator-supplied CLI flags. There is no path
   from network input to an argv here — these are build and lab-operations scripts, not
   request handlers.
2. **Is the binary resolved by full path where it matters?** `B607` flags partial paths.
   For scripts that already require a specific toolchain on PATH (cargo, go, bpftool),
   pinning absolute paths would break every machine whose layout differs and buys nothing:
   an attacker who can rewrite PATH for these processes can already run code as that user.

**Disposition: skip by rule ID, with this reasoning, rather than 102 annotations.**
Revisit if any of these scripts ever takes input from a network source.

### `B311` (21) — non-cryptographic `random`

15 of the 21 are in `ebpf/fuzz/generate_seeds.py`, where a fast non-cryptographic PRNG is
the *correct* choice — fuzzing wants throughput and reproducible seeds, not entropy. The
remaining 6 are corpus sampling (`05_generate_qa.py`, `07_prepare_training.py`,
`15_rebuild_corpus_v2.py`) and jitter in the doom tick loop.

**Checked explicitly, since this is the one that would matter:** none of the 21 feeds a
token, key, nonce, session identifier, or anything else where predictability is a
security property.

**Disposition: skip by rule ID.** If any future code uses `random` for a credential, that
is a `secrets` module case and this skip must not cover it — which is why the rule ID
skip should carry this note in the config.

### `B605` (2) — `shell=True`

Both are the same call in `scripts/doom-tick.py` and its `scripts/doom/tick.py` twin:

```python
os.popen("ip link show | grep '^[0-9]' | awk '{print $2}'").read()
```

A **fixed literal pipeline with no interpolation of any kind** — there is no injection
surface. The shell is there because the pipeline is the point. Rewriting it as three
chained `subprocess.Popen` objects would be strictly worse to read for zero security gain.

**Disposition: skip by rule ID**, on the strength of there being no variable in the
string. This one is worth re-checking if the command ever gains an f-string.

### `B101` (5) — `assert`

Asserts vanish under `python -O`. Checked: none of the five guards a security property or
validates external input; they are test-file assertions and internal invariants.

**Disposition: skip by rule ID.**

### `B405` (2) + `B314` (2) — XML parsing

`12_process_stackoverflow.py` and `14_extract_wikipedia.py` stream-parse the StackOverflow
and Wikipedia dumps with `xml.etree`. Those are large third-party archives, so the input
is not authored here — but it is also not attacker-supplied in any meaningful sense: it is
a file the operator deliberately downloaded from a known source and unpacked by hand, and
the parse is a one-off batch job on a lab machine.

`xml.etree` does not resolve external entities, so XXE does not apply. It *is* vulnerable
to entity-expansion DoS (billion laughs), whose impact here is that a batch script you
started crashes.

`defusedxml` would close it and is the textbook answer, **but it is a new third-party
dependency** and ADR-004's policy requires approval for those.

**Disposition: leave open, flagged.** Not skipped — this is the one group in the LOW/MED
remainder where a real fix exists and is not being applied only because of a dependency
decision. See "Waiting on Stevie" below.

---

## Waiting on Stevie — do not skip these

| Rule | Sites | Why it needs a decision |
|---|---|---|
| `B310` urlopen | 35 | The underlying risk — `urlopen` accepting `file://` — is **already fixed at the boundary** (`71f43a11`: env-derived URLs are scheme-checked once, on entry, and raise otherwise). The 35 call sites are downstream of that guard. They can reasonably be skipped by rule ID *given the boundary check*, but that is a security judgement, not a lint one. |
| `B104` bind-all | 2 | `zhen_app.py` runs `app.run(host='0.0.0.0', port=20103)` and `vault-to-runway.py` has a second site. Changing a bind address is a real exposure change that can break a deployment; it has to be decided against the actual WEST/EAST topology. |
| `B615` unpinned HF download | 2 | `08_train_qlora.py` pulls a model without a pinned revision. Pinning is correct supply-chain hygiene and changes *which weights get fetched* — so it needs to name the right artifact, and the Gemma-4 GGUF was already deleted once for being the wrong quant. |
| `B405`/`B314` XML | 4 | Needs a yes/no on adding `defusedxml` as a dependency (ADR-004). |

---

## Proposed Phase 14 configuration

Drop `-ll`. Skip by rule ID with reasons, exactly as the gosec ratchet does:

```yaml
# bandit: all severities, ratcheted by rule ID. -ll is deliberately NOT used —
# a severity filter hides 132 LOW findings and is what the ratchet policy forbids.
# Each skip below is dispositioned in docs/security/bandit-disposition-2026-08-03.md.
- run: >
    bandit -r . -x ./llama.cpp,./target,./node_modules
    --skip B603,B607,B404,B311,B605,B101
```

That leaves `B310`, `B104`, `B615`, `B314` and `B405` **reported and unskipped** — the
four groups above that genuinely await a decision — which is the honest state to gate on.
