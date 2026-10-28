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

---

## Applied S-TRUE-WIRE Steps 359-363 (2026-10-28): the job gates

bandit 1.9.4 (the hash-locked `.github/ci-python` set) with the six skips above found
**72**, not 43: the four groups grew (B615 4, B314 6, B405 3) and three rule IDs were new
since August (B105 20, B103 1, B608 1). Per Appendix A.8 the new IDs were triaged per site
before the gate flipped. With the configuration in `static-analysis.yml` the run is `{}`,
so `continue-on-error` is gone.

| Rule | Sites | What was done |
|---|---|---|
| `B310` urlopen | 35 | **Skipped by rule ID**, on a condition this change makes true. The `_http_url_from_env` guard covered only `raft/zhen_app.py` (22 sites) and its scheduler client (fed `ZHEN_AGENTD_URL` through it). The three `tomb/cerydwyn` scripts read `OLLAMA_URL` (and the daemon's `ollama_url` config key) with no check, so a `file://` value read a local file: each now refuses a non-http(s) URL where it enters. `scripts/compliance/import-frameworks.py` fetches only its fixed https constants; `raft/tests/test_memory_poison.py` talks to localhost. Tests that fail if a guard goes: `raft/tests/test_zhen_url_scheme.py`, `tests/scripts/test_cerydwyn_url_scheme.py`. Residual: a future `urlopen` on an unguarded URL is a review item the skip no longer reports. |
| `B104` bind-all | 2 | `raft/zhen_app.py` binds `ZHEN_BIND_ADDR`, **127.0.0.1 when unset**, and exits on a value that is not an IP address (`bind_address()`, `raft/tests/test_zhen_bind_addr.py`); `raft/start-zhen.sh` exports it explicitly and prints it. This is an exposure change: the next start of the UI on west listens on loopback only unless `ZHEN_BIND_ADDR` is set (for example `10.10.20.1`, west's lxdbr0). Whether west sets it is S-TRUE-WIRE E5 item 7. The running process was not touched. `scripts/vault-to-runway.py`: `--host 0.0.0.0` is vLLM inside its container's network namespace; `# nosec B104` with that reason (its `-p` mapping publishes on every host interface, noted for review, unchanged). |
| `B615` HF download | 4 | `raft/scripts/08_train_qlora.py` pins `mistralai/Mistral-7B-Instruct-v0.2` to commit `63a8b081895390a26e140280378bc85ec8bce07a` (west's Hugging Face cache `refs/main`, the weights this pipeline used) for both the tokenizer and the model (it also passes `trust_remote_code=True`, which makes the pin matter more); `HF_REVISION` overrides it and must be a 40-hex commit, never a branch or tag. `scripts/tokenize-coding-for-gemma4.py` and `scripts/two-brains/test_export_roundtrip.py` load a local tokenizer directory: now `local_files_only=True` plus `# nosec B615` with that reason. |
| `B405`/`B314` XML | 9 | No `defusedxml` (a new dependency). ElementTree resolves no external entities, and expat 2.4.1+ caps entity amplification: each of the three scripts now exits on an older expat (west and CI have 2.7.1). `import-frameworks.py` parses `.xlsx` members of a fixed-URL, 64 MiB-capped download: each member is now refused when it inflates past the same cap (`xlsx_member`, `tests/scripts/test_import_frameworks_xlsx.py`). `raft/scripts/12_process_stackoverflow.py` and `14_extract_wikipedia.py` stream the operator's own multi-GB dumps by design, so a size cap would defeat them; their nosec says so. |
| `B105` hardcoded password | 20 | All in `eval/champion-runbooks/fixtures/*/validate_runbook.py`: twenty generated copies of `build.py`'s validator, frozen by the eval's PREREG (`validate_runbook.py` "cannot be changed"). The flagged string is the `SECRET` regex that *detects* inline credentials. The directory is excluded with `-x`, as `scripts/check-ruff.sh` already excludes `eval/*/fixtures`. |
| `B103` chmod | 1 | `eval/champion-runbooks/build.py`: the fixture's `check.sh` must be executable (0o755 in the eval tree). `# nosec B103` with that reason. |
| `B608` SQL string | 1 | `scripts/kanban/render-genesis-sql.py` renders a migration file, not a query: each value is dollar-quoted with a sha256-derived tag the renderer checks does not occur in it. `# nosec B608` on the closing line of the template (bandit reads nosec anywhere in the string's line range). `scripts/check-kanban-genesis.sh` still matches. |

The "Waiting on Stevie" table above is answered by this section except for B104 on west's
deployment (E5 item 7). Any new rule ID is triaged here per site before it is skipped.
