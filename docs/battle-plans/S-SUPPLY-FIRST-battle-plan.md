<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-SUPPLY-FIRST BATTLE PLAN — 15 Phases, 123 Steps

**Date**: 2026-11-04
**Sprint**: S-SUPPLY-FIRST — security first across the supply chain: DP-722, DP-719, DP-415, DP-405, DP-08, DP-721 stage 1a (`SPRINT-SERIES-2026-11-04.md`, Lane B)
**Prerequisite**: develop builds on west (Go 1.27.3); docker, cargo-audit and govulncheck installed; outbound network to the Go proxy, PyPI, download.pytorch.org, huggingface.co and registries (all confirmed 2026-11-04). The answers in `DECISION-PACK-2026-11-03.md` override its prompts.
**Target**: Go and Rust carry no known vulnerability; in Python only two things stay open, torch (fixable, deferred to after E1 under FND-013) and ChromaDB (no fixed release, FND-032); Grimoire runs on current libraries; Actions wait 14 days under Dependabot; every first-party licence statement outside Stevie's veto list reads GPL-3.0-or-later; `cmd/vulnmatch` matches lock data against local OSV and KEV files and has no outbound code
**Estimated Duration**: 6-8 hours of agent time, about 3 hours wall clock with the swarm table below
**Branch**: `sprint/s-supply-first` in `/home/govan/tmp/wt/s-supply-first`. Lanes are `sprint/s-supply-first-<lane>` in `/home/govan/tmp/wt/s-supply-first-<lane>`, so parallel agents never share an index or see each other's half edits (the S-LOW-FRUIT lane pattern, inside rule 1's one branch per plan). Phase 14 merges them. Never `git push`: Stevie pushes.
**Agent Strategy**: Phase 0 first, alone. Phases 1-13 run as eight lanes on disjoint files (swarm table). Phase 14 last, alone, on Opus.
**Commit Cadence**: every 4-5 steps and at every exit gate, signed, through `/var/tmp/s-supply-first/ci.sh` (Step 2), which stages named files only and adds the trailers. Format `[PLAN S-SUPPLY-FIRST] Steps X-Y: ...`. Bump phases are the exception: one commit per ecosystem (DP-722), made only after that ecosystem's tests pass, so a half-bumped tree is never committed. `ci.sh` falls back to `--no-gpg-sign` and logs the hash to `/var/tmp/s-supply-first/unsigned.txt`; say so in the handoff, not in the message.
**Stuck Protocol**: 3x the estimate or two failed debug attempts: mark [STUCK], commit what is verified, move on. Haiku and Sonnet agents treat any output this plan did not predict the same way: mark the step [STUCK], paste the output into the progress file, go to the next independent step. Never improvise, and never stop to ask: questions go under "For Stevie" in `/home/govan/tmp/wt/s-supply-first/docs/battle-plans/progress/S-SUPPLY-FIRST-progress.md`, oldest first. Lanes do not have that file in their own worktree: use this absolute path, and only ever append (`>>`).
**Rule 13 stop line**: Haiku and Sonnet never write or edit a script or command that runs as root, deletes outside a worktree, rewrites git history, or touches sshd, PAM, sudoers, firewalls, resolvers, keys, credentials or a live host. An agent that finds it needs one stops at that step and marks [STUCK]. That is why Phase 4 is Opus.
**Rule 14**: no step uses sudo, and every `docker run` this plan writes passes `--user`. One exception sits inside an existing gate: `check-hadolint.sh` (Step 67) runs its image as container root on a read-only mount. Never `pkill -f`; stop a process by its recorded pid. Scratch is never deleted by a cheap agent: helpers overwrite their own files, and Step 123 (Opus) clears `/var/tmp/s-supply-first`.
**Scratch**: `/var/tmp/s-supply-first`, private to this plan (rule 1). Every Go command below sets `TMPDIR` and `GOTMPDIR` to its `tmp/`, because `/tmp` is a tmpfs that parallel builds have filled (rule 3).
**Scope fence (Marshal)**: no new module, crate, package, action or image. No `compose up`, no reboot, no firewall, no host change. No wire format. Trivy stays pinned. The raft and ROCm torch stays where it is (FND-013, after E1); only Grimoire's CPU floor rises. New ideas become parked `Proposed` ADRs.

## WHAT THE TREE SAYS (measured 2026-11-04, develop `735bdd43` to `ba87679b`)

Nothing below is assumed. Each row was run or read on west while this plan was forged.

| The brief said | What is there | Effect on this plan |
|---|---|---|
| DP-415: re-pin trivy-action in docker.yml and security.yml | **Already done.** `51b3683c` on develop; all 5 `uses:` lines carry `8f28faec2651`; `6abccf398f78` survives only in two history docs | Verify only (Step 57). No edit. |
| DP-405: add Dependabot for github-actions | **Config exists** with `default-days: 7`, and nothing auto-merges | One value and its comment change, 7 to 14 (Phase 5) |
| DP-405: ADR-004's commercial wording | The quoted sentence ("Selling self-hosted infrastructure...") **was reworded 2026-10-27** (`aad966c9`). Two bullets still carry the framing: "Brand alignment", "Hiring friction" | Amend those two (Phase 5) |
| DP-08: about 290 MIT headers, "Steven" in LICENSE | **Done by S-CLEAN-SLATE** (`4c15bc76`, 279 files). 0 MIT SPDX headers remain; `LICENSE` line 2 reads Stevie. `scripts/relicense-mit-to-gpl.sh` reports 0 targets. "Steven Bellis" survives only in history docs | Do not re-run. The real residue is what that script could not see, next rows |
| (not in the brief) | **25 Dockerfiles ship `org.opencontainers.image.licenses="MIT"`**: 24 in `docker/services/`, plus `docker/base/Dockerfile.base` | Phase 8 |
| (not in the brief) | **15 `# License: MIT` comment headers** (9 here, 5 in `lxd/containers/`, 1 plain line) and about 20 prose claims the SPDX script could not match | Phase 8; `lxd/containers/*.yaml` is DP-401's to delete, so left alone |
| DP-08: check Cargo licence fields | `crates/monad-mbc` and `cmd/trace-collector` say MIT, `crates/lich-fuzz` UNLICENSED, `crates/zhend` and `crates/zhenai-forge` GPL-3.0-only, against SPDX headers that say GPL-3.0-or-later. **Stevie ruled 2026-11-04 (pack round 4): zhend and zhenai-forge move, with their README and SECURITY text.** zhend repeats `GPL-3.0-only` in 16 more lines: README, SECURITY, THIRD_PARTY and ten doc comments | Phase 9 moves them all. `crates/monad-mbc/fuzz` links GPL-2.0-only `monad-common`, so it is a veto row. `cmd/waf` leaves in S-CLEAN-CUT, so it is not edited. GPL-2.0 eBPF crates untouched |
| (not in the brief) | **`BUSL-1.1` on two copies of `design-system.css`**; `api/openapi.yaml` and two Nix files declare MIT; `pkg/iac/chef.go:133` writes `license 'MIT'` into generated cookbooks; the Librarian's template says "MIT-licensed" | Candidates list (Phase 7), untouched |
| ruff 0.17.2 is the held bump | Pin is 0.17.1. `check-tool-freshness` already calls it stale against **0.17.3** (2026-10-27): 0.17.2 would leave that gate red. pytest 9.1.1 and oxlint 1.89.0 are stale too | Bump to 0.17.3; [DECIDE] at Step 30. 0.17.3 previewed: 0 findings under the exact gate invocation |
| modernc sqlite v1.61.0 held | **Already in** (`go.mod`). v1.61.1 exists, carries no advisory, is 4 days old | No action |
| Grimoire: bump ChromaDB, sentence-transformers, transformers | **No ChromaDB release is free of the advisories**: 1.5.9, the newest, is `last_affected` on PYSEC-2026-3813/3814/3815 and PYSEC-2026-311. All four are HTTP server endpoints or the RBAC provider; Grimoire uses the embedded `PersistentClient`. sentence-transformers 6.2.0, transformers 5.20.2, tokenizers 0.23.3 audit clean | Bump all; `chromadb==1.5.9`, tracked as FND-032 (Step 43, For Stevie 1) |
| Bumps need code patched to fit | Proven by running the real `rag-index.py` and `rag-query.py` in a container: `except ValueError` must become `NotFoundError` (2 sites, a crash on first index), `get_sentence_embedding_dimension` is renamed (2 sites), and `chroma_db_impl="duckdb+parquet"` raises `ValueError` while `client.persist()` no longer exists. That dead config is in **two** places: the playbook and the `rag-query.py` embedded in `tomb/provision.sh` | Phase 4, exact patches, smoke output measured |
| Phase 4 could be Sonnet | Every Grimoire file is on a root path: `setup-grimoire.sh` refuses to run except as root and runs `pip install -r requirements.txt` and `rag-index.py`; the playbook is `become: true`; `tomb/provision.sh` installs through `sudo -n` | Rule 13: Phase 4 is Opus. Its steps stay exact |
| (not in the brief) | The playbook's `files/rag-query.py` does not exist (`tomb/ansible/files/` holds only `kingdom-alerts.yml`), hidden by `failed_when: false` | Pre-existing, left alone, under "For Stevie" |
| `scripts/check-python-audit.sh` passes | It audits only fully pinned files: **`tomb/grimoire/rag/requirements.txt` and `notebooks/requirements.txt` are invisible to it.** That is why it is green over Grimoire's seven Dependabot alerts. Notebooks' `requirements.in` audited by hand: clean | Audited by hand in Phase 4 |
| `sbom/` as a source | `sbom/go-dependencies.json` was last generated **2026-03-15**: it lists x/crypto v0.43.0, the tree has v0.58.0. `check-sbom-trend.sh` counts live lock data for that reason | The scanner reads lock data first; Phase 14 regenerates |
| Worktrees under `~/tmp/wt/` | `go.mod` has `replace github.com/unheaded/doomgeneric => ../projects/doomgeneric/unheaded`: **every Go build in a worktree there fails** without `~/tmp/wt/projects` | Step 4 makes the symlink |
| `cargo audit` clean with four ignores | The ignored RUSTSEC-2026-0097 **no longer matches anything**: audited outside the repo, so `audit.toml` does not load, all four locks report 0 | Phase 2 removes three; `cmd/trace-collector`'s goes with DP-210 |

### Vulnerable today

| Ecosystem | Tool | Finding | Reachable | Fix | Phase |
|---|---|---|---|---|---|
| Go | govulncheck 1.9.0, DB 2026-11-02 | GO-2026-6866, GO-2026-6860 in golang.org/x/net v0.61.0 | no (`x/net/html` not imported) | v0.62.0 (2026-10-27) | 1 |
| Go | govulncheck | GO-2026-5932 in golang.org/x/crypto v0.58.0 (openpgp) | no (not imported) | `v0.58.1-0.20261009181718-2f1f834330b2`; no tag yet; its only difference from v0.58.0 is that `openpgp/` is gone | 1 |
| Go | go.mod read | Same two modules, indirect, in `tools/gosec/go.mod` | n/a | same versions, proven to build | 1 |
| Rust | cargo-audit 0.22.1, DB `965bf9ad` (2026-11-03) | **0 across all 24 lock files** | n/a | none; 9 "unmaintained" warnings have no patched version (zhend: bincode, fxhash, instant, paste, 4 pqcrypto-*; plus one in a test fixture) | 2 |
| Python, gated | pip-audit 2.10.1 | torch 2.10.0: PYSEC-2025-194, PYSEC-2026-139 (`raft/requirements.txt`, `sbom/python-pip-freeze.txt`), accepted | local only | FND-013, after E1. torch 2.14.1 audits clean, so the accepted line's "no fixed release yet" is out of date | noted |
| Python, ungated | pip-audit on `tomb/grimoire/rag/requirements.in` | chromadb 0.6.3: PYSEC-2026-3813, -3814, -3815 | no server runs | none exists; 1.5.9 adds PYSEC-2026-311 to the list and clears nothing | 4 |
| Python, ungated | same | sentence-transformers 2.7.0: PYSEC-2026-4164 | model load | 6.2.0 | 4 |
| Python, ungated | same | transformers 4.57.6: PYSEC-2025-217, PYSEC-2026-2288, -2289, -2290, -3929, -4174 | model load | 5.20.2 | 4 |
| Python, ungated | pip-audit on `notebooks/requirements.in` | none | | | none |
| npm | `git ls-files` | no package.json or lockfile in the tree | | | none |
| Images | not measured | Stack is down and local trivy (0.72.0) is behind the pin (0.76.1) | ? | measure with the pinned image; Phase 6 covers HIGH and CRITICAL with a fix, narrower than DP-722's "anything" | 6 |

Stale, not vulnerable (`check-tool-freshness` exit 1 today): ruff, pytest, oxlint.

**DP-721 recommendation: a new `cmd/vulnmatch` on two new packages next to `pkg/threatfeed/kev`.** Reasons: `cmd/depaudit` answers a different question (release age, Go only, from stdin) and its one job is what `check-dep-staleness.sh` relies on; `cmd/kev-poller` is a long-running hardened daemon that must not grow a tree walker; `cmd/crypto-inventory` is scoped to crypto assets. The catalog parser stays reused, not forked: `kev.Parse` reads kev-poller's `latest.json`. Later stages add inventory readers without touching the matcher. No new module: `golang.org/x/mod` is not in `go.mod`, so version comparison is written here against the stdlib and table-tested. **A reference implementation of everything in Phases 10-12 was built and run on west on 2026-11-04**: `go test -race`, golangci-lint 2.15.0 and gosec v2.29.0 all clean. Appendix C carries those exact files, and the expected red and green output below is what they produced.

**This is stage 1a, not DP-721's stage 1.** DP-721's first stage is image and filesystem matching. Stage 1a is lock data only and replaces no trivy use yet; trivy stays pinned.

**OSV is an outside data source the third-party sitting has not confirmed, so nothing outbound enters the tree.** There is no fetch code, no flag and no API address. Fixtures are synthetic (`FIXTURE-*`, CVE-2099-*), records come only from a local directory, and Step 107 asserts this plan's packages import no network package and `vulnmatch` calls only `kev.Parse` from the package it shares with kev-poller. Live fetching is a later change, after the sitting.

## FOR STEVIE (also written to the progress file at Step 12)

1. **ChromaDB cannot be cleared by a bump.** Default taken: `chromadb==1.5.9`, tracked as FND-032. `check-python-audit.sh` still does not audit Grimoire's file, because torch, markdown, beautifulsoup4, tqdm and pyyaml are ranges, so `python-audit-accepted.txt` does not grow (rule 6, shrink-only). Say the word and Appendix D pins those five too, which brings the file under the gate and adds four accepted lines; the alternative is replacing ChromaDB in Grimoire. Dependabot #119 and #120 stay open either way.
2. **Veto list**: `docs/legal/RELICENSE-CANDIDATES-2026-11-04.md`, ten rows, left untouched.
3. **Grimoire's full hash lock** still needs 101 ADR-096 admissions, or a CPU-index decision that drops the 19 CUDA wheels. Not done here.
4. **x/crypto sits on an untagged pseudo-version** until the next tag, because DP-722 says bump anything with a vulnerability and no tag has the fix.
5. **Images**: Phase 6 lists what needs a live window (`postgres` waits on a backup and restore drill, ADR-102).
6. **`tomb/ansible/playbook-grimoire.yml` deploys `files/rag-query.py`, which does not exist**, and `failed_when: false` hides it. Pre-existing, left alone: this plan fixes the Chroma calls and nothing else in that playbook.
7. **torch stays on a fixable version, against DP-722.** PYSEC-2025-194 is fixed in 2.13.0, and pip-audit reports 2.14.1 clean of both accepted ids. `raft/` and the pip freeze stay on 2.10.0 because DP-25.7 put the ROCm move and its training smoke on a forge evening after E1. Grimoire's CPU floor rises to 2.14.1 here. If DP-722 should win, say so; the forge moving to `unheaded-forge` may also change where that smoke runs.
8. **Lanes go past the letter of rule 1.** One plan branch plus eight `sprint/s-supply-first-<lane>` branches, merged in Phase 14, removed in Step 123, on S-LOW-FRUIT's precedent. They stop parallel agents sharing an index.
9. **ruff, pytest and oxlint are not vulnerability bumps**, so rule 4's approval does not literally cover them. They are the held bumps DP-722 named, or the same class, and add no package name.
10. **The embedded Grimoire scripts cannot work air-gapped as written.** `collection.add(documents=...)` in the playbook's indexer and `query_texts=` in `tomb/provision.sh` use Chroma's default embedding, which downloads an ONNX model on first use. The Tomb VM has no route to it. Probably older than this bump, not verified. Left alone.

## LEGEND

[B] bash · [V] verify (must pass) · [D] debug (only on failure) · [W] write or edit a file · [R] read · [S] sudo (no step here uses it) · [P] parallelizable · [C] commit checkpoint · [DECIDE] decision with its recommendation pre-seeded (proceed, do not ask) · [STUCK] skipped under the Stuck Protocol · [BLOCKED] blocked by an upstream [STUCK] only

`Lane` names the worktree. Commands open with `cd` because an agent's working directory resets between calls. A command with no stated output needs only exit 0. **"Other output → [STUCK]" means**: mark the step, paste the output into the progress file, move to the next independent step.

## SWARM TABLE

Rule 3 holds: at most three builders at once. Doc-only lanes do not count.

| Wave | Lanes running at once | Phases | Files (disjoint) | Builder? |
|---|---|---|---|---|
| 0 | coordinator | 0 | `/var/tmp/s-supply-first`, worktrees, progress file | one |
| 1 | `go` | 1 | `go.mod`, `go.sum`, `tools/gosec/go.mod`, `tools/gosec/go.sum`, `docs/policy/dependency-watch.md` | **builder** |
| 1 | `scanner` | 10, 11, 12 in order | `pkg/threatfeed/osv/`, `pkg/threatfeed/inventory/`, `cmd/vulnmatch/` | **builder** |
| 1 | `grimoire` | 4 | `tomb/grimoire/rag/`, `tomb/ansible/playbook-grimoire.yml`, `tomb/provision.sh`, `tomb/cerydwyn/setup-cerydwyn.sh`, `tomb/Makefile`, `docs/policy/python-audit-accepted.txt`, `compliance/findings/register.yaml` | **builder** (docker), Opus |
| 1 | `rust` | 2 | three `.cargo/audit.toml` | no: reads lock files |
| 1 | `ci` | 5 | `.github/dependabot.yml`, `wiki/Security.md`, ADR-004, ADR-095 | no |
| 1 | `licence` | 7, 8, 9 in order | `docs/legal/`, `monitoring/`, `lxd/` except `lxd/containers/`, `routing/`, `docs/network/`, `docker/`, `crates/zhend/`, `crates/zhenai-forge/Cargo.toml`, `cmd/ebpf-collector/collector/Cargo.toml`, listed docs | no |
| 2 | `pyci` | 3 | `.github/ci-python/`, `scripts/lib/tool-pins.sh` | **builder** (starts when a wave 1 builder ends) |
| 2 | `images` | 6 | Dockerfiles' `FROM`, compose tags | **builder** (same) |
| 2 | `scanner` | 13 | `wiki/SBOM.md`, `man/vulnmatch.1`, `docs/LIVE-PATHS.md`, `scripts/vulnmatch-fixture.sh` | no |
| 3 | coordinator | 14 | merges, `compliance/crypto/cbom.cdx.json` | one |

`compliance/findings/register.yaml`: FND-031 is S-WIRE-HONEST's and FND-032 is this plan's (series, reserved numbers). This plan inserts above FND-029, leaving FND-030's block between it and any entry placed above FND-030, so the two do not touch.

Overlaps that are safe: `wiki/Security.md` is edited by `ci` (Dependabot row) and `scanner` (scanner row) on different lines; `docker/` Dockerfiles by `licence` (labels) and `images` (`FROM`) on different lines. If `images` takes a Dockerfile with a label on the next line, git may conflict: Phase 14 resolves it.

---

## PHASE 0: PREFLIGHT (Steps 1-14)

**Goal**: helpers, worktrees and a known-failures baseline exist.
**Prerequisite**: none
**Time**: 35m (Step 10 is most of it)
**Agent**: Coordinator · **Model**: Sonnet

- [ ] **Step 1** [B] ~1m: note the starting commit.
```bash
cd /home/govan/tmp/unheaded && git rev-parse --short develop && git worktree list | grep -c s-supply-first
```
  Expected: a short hash, then `0`. If the second line is not `0`, this is a resumed run: go to Step 6.

- [ ] **Step 2** [W] ~3m: write the three helpers.
```bash
mkdir -p /var/tmp/s-supply-first/tmp && cd /var/tmp/s-supply-first
cat > ci.sh <<'EOF'
#!/usr/bin/env bash
# ci.sh "<subject>" FILE...: stage the named files and commit with the trailers.
set -euo pipefail
msg=$1; shift
git add -- "$@"
body=$(printf '%s\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nClaude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp\n' "$msg")
if ! timeout 30 git commit -S -q -m "$body"; then
	git commit --no-gpg-sign -q -m "$body"
	git rev-parse --short HEAD >>/var/tmp/s-supply-first/unsigned.txt
fi
git log -1 --format='%h %G? %s' | cut -c1-110
EOF
cat > rl.py <<'EOF'
#!/usr/bin/env python3
# rl.py [--check] SPEC: apply "file ||| old ||| new" lines, relative to the cwd.
# \n in old or new is a newline. Every old must match its file exactly once, or
# nothing at all is written. --check writes nothing.
import sys
args = sys.argv[1:]
check = "--check" in args
args = [a for a in args if a != "--check"]
edits, bad = [], 0
for raw in open(args[0], encoding="utf-8"):
    raw = raw.rstrip("\n")
    if not raw.strip():
        continue
    f, old, new = raw.split(" ||| ", 2)
    old, new = old.replace("\\n", "\n"), new.replace("\\n", "\n")
    try:
        n = open(f, encoding="utf-8").read().count(old)
    except OSError as e:
        print("FAIL", f, e); bad += 1; continue
    if n != 1:
        print(f"FAIL {f}: {n} matches for: {old[:60]!r}"); bad += 1
    edits.append((f, old, new))
if bad:
    sys.exit(1)
if not check:
    for f, old, new in edits:
        s = open(f, encoding="utf-8").read()
        open(f, "w", encoding="utf-8").write(s.replace(old, new))
print(f"OK {len(edits)} replacements" + (" (check only)" if check else ""))
EOF
cat > extract.py <<'EOF'
#!/usr/bin/env python3
# extract.py PATH...: copy each file's block from Appendix C of the plan to PATH,
# relative to the cwd, creating directories. The block is the source of truth.
import pathlib, re, sys
plan = "/home/govan/tmp/unheaded/docs/battle-plans/S-SUPPLY-FIRST-battle-plan.md"
text = open(plan, encoding="utf-8").read()
for rel in sys.argv[1:]:
    m = re.search(r"<!-- file: " + re.escape(rel) + r" -->\n+````[a-z]*\n(.*?)\n````\n", text, re.S)
    if not m:
        sys.exit(f"FAIL no block for {rel}")
    p = pathlib.Path(rel)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(m.group(1) + "\n", encoding="utf-8")
    print("wrote", rel)
EOF
chmod +x ci.sh rl.py extract.py
```

- [ ] **Step 3** [V] ~1m: **HELPERS WORK**.
```bash
mkdir -p /var/tmp/s-supply-first/selftest && cd /var/tmp/s-supply-first/selftest && printf 'a one\nb two\n' > t.txt && printf 't.txt ||| a one ||| a 1\n' > t.tsv && python3 -I ../rl.py t.tsv && head -1 t.txt; printf 't.txt ||| nope ||| x\n' > t2.tsv; python3 -I ../rl.py t2.tsv; echo rc=$?
```
  Expected, exactly:
```
OK 1 replacements
a 1
FAIL t.txt: 0 matches for: 'nope'
rc=1
```
  Other output → re-run Step 2 once; a second failure → [STUCK].

- [ ] **Step 4** [B] ~1m: make `go.mod`'s relative replace resolve under `~/tmp/wt/`.
```bash
[ -e /home/govan/tmp/wt/projects ] || ln -s /home/govan/tmp/projects /home/govan/tmp/wt/projects; ls /home/govan/tmp/wt/projects/doomgeneric/unheaded/go.mod
```
  Expected: that path printed. Other output → [STUCK]; Steps 9 onward are [BLOCKED].

- [ ] **Step 5** [B] ~3m: one main worktree and eight lanes, all from develop.
```bash
cd /home/govan/tmp/unheaded && git worktree add -q -b sprint/s-supply-first /home/govan/tmp/wt/s-supply-first develop && for l in go rust pyci grimoire ci images licence scanner; do git worktree add -q -b sprint/s-supply-first-$l /home/govan/tmp/wt/s-supply-first-$l develop; done
```

- [ ] **Step 6** [V] ~1m: **NINE WORKTREES**.
```bash
cd /home/govan/tmp/unheaded && git worktree list | grep -c 's-supply-first'
```
  Expected: `9`. If lower → Step 7.

- [ ] **Step 7** [D] ~2m: `git worktree list | grep s-supply-first` and add only the missing lane with `git worktree add -q -b sprint/s-supply-first-<lane> /home/govan/tmp/wt/s-supply-first-<lane> develop`. If git says the branch already exists, drop `-b sprint/s-supply-first-<lane>` and append that branch name. Still not 9 → [STUCK].

- [ ] **Step 8** [B] ~3m: the pinned CI tools (recipe from `~/tmp/next.md`).
```bash
python3 -m venv /var/tmp/s-supply-first/civenv && /var/tmp/s-supply-first/civenv/bin/pip install --quiet --require-hashes --no-deps --only-binary=:all: -r /home/govan/tmp/wt/s-supply-first/.github/ci-python/requirements.txt && /var/tmp/s-supply-first/civenv/bin/ruff --version && /var/tmp/s-supply-first/civenv/bin/pip-audit --version
```
  Expected: `ruff 0.17.1` then `pip-audit 2.10.1`. Other output → [STUCK].

- [ ] **Step 9** [V] ~4m: **GO BUILDS IN A WORKTREE**.
```bash
cd /home/govan/tmp/wt/s-supply-first && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go build ./... && go list -m all | tail -n +2 | wc -l
```
  Expected: `149`. If `replacement directory ../projects/doomgeneric/unheaded does not exist` → back to Step 4. A different count means develop moved: record it in the progress file and use it as the number at Step 20.

- [ ] **Step 10** [B] ~20m: known-failures baseline. Run it in the background and continue with the next steps; the Phase 0 exit gate waits for it, so wave 1 never starts a fourth builder.
```bash
cd /home/govan/tmp/wt/s-supply-first && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race ./... 2>&1 | grep -vE '^ok|no test files|^\?' > /var/tmp/s-supply-first/go-baseline.txt; wc -l < /var/tmp/s-supply-first/go-baseline.txt; touch /var/tmp/s-supply-first/go-baseline.done
```
  Expected: `0`, but any count is fine: this file IS the baseline.

- [ ] **Step 11** [V] ~4m: **THE GO LIST STILL HOLDS**.
```bash
cd /home/govan/tmp/wt/s-supply-first && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp govulncheck -show verbose ./... 2>&1 | grep -E '^Vulnerability #|Your code is affected'
```
  Expected, exactly:
```
Vulnerability #1: GO-2026-6866
Vulnerability #2: GO-2026-6860
Vulnerability #3: GO-2026-5932
Your code is affected by 0 vulnerabilities.
```
  A new GO- id, or any count but 0 on the last line → write the output under "For Stevie" and tell the coordinator: Phase 1's versions may not cover it. Phases 1-13 continue.

- [ ] **Step 12** [W] ~2m: start the progress file in the main worktree.
```bash
mkdir -p /home/govan/tmp/wt/s-supply-first/docs/battle-plans/progress && cat > /home/govan/tmp/wt/s-supply-first/docs/battle-plans/progress/S-SUPPLY-FIRST-progress.md <<'EOF'
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-SUPPLY-FIRST progress

Plan: `docs/battle-plans/S-SUPPLY-FIRST-battle-plan.md`.

## For Stevie

1. ChromaDB has no release free of PYSEC-2026-311, -3813, -3814, -3815 (`last_affected` 1.5.9). All four are server endpoints or the RBAC provider; Grimoire runs the embedded client. Default taken: `chromadb==1.5.9`, tracked as FND-032. The file stays outside `check-python-audit.sh` because five other lines are ranges, so `python-audit-accepted.txt` does not grow. Appendix D of the plan pins those five and adds four accepted lines if you say so; the other way out is replacing ChromaDB in Grimoire. Dependabot #119 and #120 stay open either way.
2. Veto list: `docs/legal/RELICENSE-CANDIDATES-2026-11-04.md`, untouched until you answer.
3. Grimoire's full hash lock needs 101 ADR-096 admissions, or a CPU-index decision that drops the 19 CUDA wheels.
4. golang.org/x/crypto is on an untagged pseudo-version until the next tag (no tag carries the GO-2026-5932 fix; `openpgp/` removal is the only difference from v0.58.0).
5. `tomb/ansible/playbook-grimoire.yml` deploys `files/rag-query.py`, which does not exist; `failed_when: false` hides it. Pre-existing, left alone.
6. torch stays on 2.10.0 in `raft/` and the pip freeze although 2.14.1 audits clean, against DP-722's wording: DP-25.7 put the ROCm move and its training smoke after E1. Grimoire's CPU floor rose to 2.14.1.
7. This plan runs eight lane branches inside one plan branch, past the letter of rule 1, on S-LOW-FRUIT's precedent.
8. ruff, pytest and oxlint are not vulnerability bumps, so rule 4's approval does not literally cover them.
9. The embedded Grimoire scripts use Chroma's default embedding, which downloads an ONNX model on first use and cannot work on an air-gapped VM. Probably older than this bump, not verified.

## Stuck

(none yet)

## Lane reports

EOF
```

- [ ] **Step 13** [C] ~1m: commit the progress file.
```bash
cd /home/govan/tmp/wt/s-supply-first && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 1-13: preflight, progress file' docs/battle-plans/progress/S-SUPPLY-FIRST-progress.md
```
  Expected: a line ending in that subject, second field `G` (or `N` if gpg-agent timed out).

- [ ] **Step 14** [V] ~1m: **PHASE 0 EXIT GATE**.
```bash
ls /var/tmp/s-supply-first/{ci.sh,rl.py,extract.py,civenv/bin/ruff,go-baseline.done} && git -C /home/govan/tmp/unheaded worktree list | grep -c s-supply-first
```
  Expected: five paths, then `9`. Steps 9 and 11 passed. If only `go-baseline.done` is missing, Step 10 is still running: wait for it. Other output → do not start the lanes.

---

## PHASE 1: GO (Steps 15-24)

**Goal**: `govulncheck` reports nothing at symbol, package or module level.
**Prerequisite**: Phase 0 exit gate
**Time**: 35m
**Agent**: Agent [P] · **Lane**: `go` · **Model**: Sonnet

- [ ] **Step 15** [B] ~2m: bump the root module. `GOFLAGS=-mod=mod` applies to this command only.
```bash
cd /home/govan/tmp/wt/s-supply-first-go && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp GOFLAGS=-mod=mod go get golang.org/x/net@v0.62.0 golang.org/x/crypto@v0.58.1-0.20261009181718-2f1f834330b2 && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp GOFLAGS=-mod=mod go mod tidy && git diff -U0 -- go.mod | grep -E '^[-+]' | grep -vE '^(\+\+\+|---)'
```
  Expected, exactly:
```
-	golang.org/x/crypto v0.58.0
+	golang.org/x/crypto v0.58.1-0.20261009181718-2f1f834330b2
-	golang.org/x/net v0.61.0 // indirect
+	golang.org/x/net v0.62.0 // indirect
```
  `go get` also prints lines starting `go:` (`go: upgraded ...`, `go: downloading ...`) above these: those are not extra. Any other `-` or `+` line means a dependency moved with it → [STUCK], and `git checkout -- go.mod go.sum`.

- [ ] **Step 16** [B] ~2m: the same two in the gosec tool module.
```bash
cd /home/govan/tmp/wt/s-supply-first-go/tools/gosec && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp GOFLAGS=-mod=mod go get golang.org/x/net@v0.62.0 golang.org/x/crypto@v0.58.1-0.20261009181718-2f1f834330b2 && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp GOFLAGS=-mod=mod go mod tidy && git diff -U0 -- go.mod | grep -E '^[-+]' | grep -vE '^(\+\+\+|---)' && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go build -o /var/tmp/s-supply-first/gosec github.com/securego/gosec/v2/cmd/gosec && echo built
```
  Expected, exactly:
```
-	golang.org/x/crypto v0.58.0 // indirect
+	golang.org/x/crypto v0.58.1-0.20261009181718-2f1f834330b2 // indirect
-	golang.org/x/net v0.61.0 // indirect
+	golang.org/x/net v0.62.0 // indirect
built
```
  Lines starting `go:` are not extra. Other output → [STUCK]; Step 15's work stands on its own.

- [ ] **Step 17** [V] ~4m: **GOVULNCHECK CLEAN**.
```bash
cd /home/govan/tmp/wt/s-supply-first-go && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go build ./... && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp govulncheck -show verbose ./... 2>&1 | tail -2
```
  Expected: the last non-blank line is `No vulnerabilities found.`. If a GO- id is listed → Step 18.

- [ ] **Step 18** [D] ~5m: `GOMAXPROCS=4 govulncheck -show verbose ./... 2>&1 | grep -E 'Vulnerability #|Found in|Fixed in'`. A `Fixed in` with a version newer than go.mod's on a module this plan did not name: `GOFLAGS=-mod=mod go get <module>@<that version>`, then re-run Step 17 once. Anything else, or still listed → [STUCK].

- [ ] **Step 19** [V] ~20m: **NO NEW TEST FAILURES** (needs Step 10 finished).
```bash
cd /home/govan/tmp/wt/s-supply-first-go && gofmt -l cmd pkg services internal; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go vet ./... 2>&1 | tail -5; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race ./... 2>&1 | grep -vE '^ok|no test files|^\?' > /var/tmp/s-supply-first/go-after.txt; diff /var/tmp/s-supply-first/go-baseline.txt /var/tmp/s-supply-first/go-after.txt && echo SAME
```
  Expected: `SAME`, with nothing from `gofmt` or `go vet` above it. Lines differing only in a timing are the same failure. A new `--- FAIL` → [STUCK] with the package name; do not commit this phase.

- [ ] **Step 20** [V] ~4m: **GATES**.
```bash
cd /home/govan/tmp/wt/s-supply-first-go && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-dependency-baseline.sh | tail -1; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-sbom-trend.sh | grep go_components
```
  Expected: `[PASS] 590 third-party packages, all in the baseline`, then `OK go_components: 149`. `check-mod-integrity` compares against what is committed, so it runs at the exit gate. Other output → [STUCK].

- [ ] **Step 21** [W] ~2m: `docs/policy/dependency-watch.md`: the x/crypto row and the Bumps line (DP-405).
```bash
cd /home/govan/tmp/wt/s-supply-first-go && cat > /var/tmp/s-supply-first/p1.spec <<'EOF'
docs/policy/dependency-watch.md ||| | golang.org/x/crypto | v0.58.0 | v0.58.0 (2026-10-09) | 0 | crypto | Go team | none | keep | ||| | golang.org/x/crypto | v0.58.1-0.20261009181718-2f1f834330b2 | v0.58.0 (2026-10-09) | 0 | crypto | Go team | none | keep; untagged pin for GO-2026-5932 (`openpgp/` removed), back to a tag at the next release |
docs/policy/dependency-watch.md ||| (Go and Rust wait 14 days, Actions and images 7) ||| (Go, Rust and Actions wait 14 days, images 7)
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p1.spec
```
  Expected: `OK 2 replacements`. Other output → [STUCK].

- [ ] **Step 22** [V] ~2m: **STALENESS GATE READS THE NEW ROW** (needs the module proxy).
```bash
cd /home/govan/tmp/wt/s-supply-first-go && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-dep-staleness.sh | tail -1
```
  Expected: `OK: 3 stale direct modules, each with a recorded decision`. Other output → [STUCK].

- [ ] **Step 23** [C] ~1m: **ONE COMMIT FOR GO**.
```bash
cd /home/govan/tmp/wt/s-supply-first-go && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 15-23: go: x/net v0.62.0, x/crypto past GO-2026-5932; govulncheck clean' go.mod go.sum tools/gosec/go.mod tools/gosec/go.sum docs/policy/dependency-watch.md
```

- [ ] **Step 24** [V] ~1m: **PHASE 1 EXIT GATE**.
```bash
cd /home/govan/tmp/wt/s-supply-first-go && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-mod-integrity.sh | tail -1 && git status --short | wc -l
```
  Expected: `OK: go mod verify; 26 lock files as committed`, then `0`. Steps 17 and 19 passed. Other output → do not report the lane done.

---

## PHASE 2: RUST (Steps 25-29)

**Goal**: prove 0 advisories on all 24 lock files without the stale ignores.
**Prerequisite**: Phase 0 exit gate
**Time**: 10m
**Agent**: Agent [P] · **Lane**: `rust` · **Model**: Haiku

- [ ] **Step 25** [W] ~1m: write the audit loop (CI's loop, with counts).
```bash
cat > /var/tmp/s-supply-first/rust-audit.sh <<'EOF'
#!/usr/bin/env bash
# Audit every tracked Cargo.lock from its own directory, as security.yml does.
cd "$1" || exit 2
locks=0; bad=0
while read -r d; do
	locks=$((locks + 1))
	if ! out=$(cd "$d" && cargo audit -q 2>&1); then
		bad=$((bad + 1)); echo "VULN $d"; printf '%s\n' "$out" | tail -20
	fi
done < <(git ls-files '*/Cargo.lock' | xargs -n1 dirname | sort -u)
echo "TOTAL locks=$locks failing=$bad"
[ "$bad" -eq 0 ]
EOF
chmod +x /var/tmp/s-supply-first/rust-audit.sh
```

- [ ] **Step 26** [V] ~4m: **CLEAN BEFORE THE CHANGE** (needs the RustSec database on GitHub).
```bash
/var/tmp/s-supply-first/rust-audit.sh /home/govan/tmp/wt/s-supply-first-rust; echo rc=$?
```
  Expected, exactly: `TOTAL locks=24 failing=0` then `rc=0`. Other output → [STUCK]: a new RUSTSEC id needs an Opus look; leave Steps 27-29 [BLOCKED].

- [ ] **Step 27** [W] ~1m: delete the three stale ignore files. Each holds only that one ignore. `cmd/trace-collector/.cargo/audit.toml` stays: DP-210 removes that crate in S-CLEAN-CUT.
```bash
cd /home/govan/tmp/wt/s-supply-first-rust && rm cmd/ebpf-collector/.cargo/audit.toml crates/doom-runner/.cargo/audit.toml crates/zhend/.cargo/audit.toml && git status --short
```
  Expected, exactly (order may differ):
```
 D cmd/ebpf-collector/.cargo/audit.toml
 D crates/doom-runner/.cargo/audit.toml
 D crates/zhend/.cargo/audit.toml
```

- [ ] **Step 28** [V] ~4m: **CLEAN WITHOUT THEM**.
```bash
/var/tmp/s-supply-first/rust-audit.sh /home/govan/tmp/wt/s-supply-first-rust; echo rc=$?
```
  Expected, exactly: `TOTAL locks=24 failing=0` then `rc=0`. Other output → `cd /home/govan/tmp/wt/s-supply-first-rust && git checkout -- cmd/ebpf-collector/.cargo/audit.toml crates/doom-runner/.cargo/audit.toml crates/zhend/.cargo/audit.toml`, then [STUCK].

- [ ] **Step 29** [C] ~1m: **PHASE 2 EXIT GATE + ONE COMMIT FOR RUST** (Step 28 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-rust && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 25-29: rust: 0 advisories on 24 locks; three stale RUSTSEC-2026-0097 ignores removed' cmd/ebpf-collector/.cargo/audit.toml crates/doom-runner/.cargo/audit.toml crates/zhend/.cargo/audit.toml && git status --short | wc -l
```
  Expected: the commit line, then `0`. The 9 "unmaintained" warnings stay: they have no patched version and `cargo audit` does not fail on them.

---

## PHASE 3: CI TOOLS AND HELD BUMPS (Steps 30-41)

**Goal**: `check-tool-freshness` exits 0; ruff, pytest and oxlint are current.
**Prerequisite**: Phase 0 exit gate; a builder slot (wave 2)
**Time**: 25m
**Agent**: Agent [P] · **Lane**: `pyci` · **Model**: Sonnet

- [ ] **Step 30** [DECIDE] ~1m: ruff goes to **0.17.3, not 0.17.2**. RECOMMENDATION: 0.17.3. Rationale: `check-tool-freshness` fails while a release at least 7 days old is newer than the pin, and 0.17.3 (2026-10-27) already is, so 0.17.2 leaves the gate red; 0.17.3 has been run on this tree under the gate's exact flags with 0 findings. Override ONLY if PyPI no longer offers 0.17.3.

- [ ] **Step 31** [W] ~1m: bump the two exact pins.
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && cat > /var/tmp/s-supply-first/p3.spec <<'EOF'
.github/ci-python/requirements.in ||| ruff==0.17.1 ||| ruff==0.17.3
.github/ci-python/requirements.in ||| pytest==9.1.1 ||| pytest==9.2.0
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p3.spec
```
  Expected: `OK 2 replacements`. Other output → [STUCK].

- [ ] **Step 32** [B] ~3m: re-lock in the repo's throwaway container (the lock never runs package code on the host).
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && scripts/ci-python-lock.sh && git diff -U0 -- .github/ci-python/requirements.txt | grep -E '^[-+][a-z0-9A-Z_.-]+==' | sed 's/ .*//'
```
  Expected: `[ci-python-lock] 50 packages pinned in .github/ci-python/requirements.txt`, then `-`/`+` pairs for ruff and pytest. On 2026-11-04 six transitive packages moved with them (certifi 2026.10.23, filelock 4.2.4, msgpack 1.2.4, pip 26.2.2, platformdirs 4.13.6, pycparser 3.12); later dates may move more. A `+` line with a name that has no `-` line is a new package → `git checkout -- .github/ci-python/`, then [STUCK].

- [ ] **Step 33** [V] ~1m: **NO NEW PACKAGE NAME**.
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && ./scripts/check-dependency-baseline.sh | tail -1 && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-sbom-trend.sh | grep python_components
```
  Expected: `[PASS] 590 third-party packages, all in the baseline` and `OK python_components: 77`. Other output → [STUCK].

- [ ] **Step 34** [B] ~3m: rebuild the tools from the new lock.
```bash
python3 -m venv /var/tmp/s-supply-first/civenv2 && /var/tmp/s-supply-first/civenv2/bin/pip install --quiet --require-hashes --no-deps --only-binary=:all: -r /home/govan/tmp/wt/s-supply-first-pyci/.github/ci-python/requirements.txt && /var/tmp/s-supply-first/civenv2/bin/ruff --version
```
  Expected: `ruff 0.17.3`. Other output → [STUCK].

- [ ] **Step 35** [V] ~3m: **RUFF AND PIP-AUDIT GATES ON THE NEW TOOLS**.
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && RUFF=/var/tmp/s-supply-first/civenv2/bin/ruff ./scripts/check-ruff.sh | tail -1; PIP_AUDIT=/var/tmp/s-supply-first/civenv2/bin/pip-audit ./scripts/check-python-audit.sh | tail -1
```
  Expected: `PASS: ruff 0 findings (with the exclusions above).` and `[PASS] 3 pinned requirements files; 2 known vulnerabilities, all accepted with a reason`. A ruff finding → [STUCK] with the rule id; do not add an exclusion.

- [ ] **Step 36** [C] ~1m: **ONE COMMIT FOR PYTHON CI**.
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 30-36: python ci: ruff 0.17.3, pytest 9.2.0, re-locked' .github/ci-python/requirements.in .github/ci-python/requirements.txt
```

- [ ] **Step 37** [W] ~1m: oxlint 1.90.0. The sha256 is GitHub's asset digest for `oxlint-x86_64-unknown-linux-musl.tar.gz`, read from the release on 2026-11-04.
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && cat > /var/tmp/s-supply-first/p3b.spec <<'EOF'
scripts/lib/tool-pins.sh ||| OXLINT_VERSION="1.89.0" ||| OXLINT_VERSION="1.90.0"
scripts/lib/tool-pins.sh ||| OXLINT_SHA256="eaccbadd29973d37cedc95ec1f6e889fdae13c694e1cef4b6cbfcfe1c650e7b8" ||| OXLINT_SHA256="70c5c44f0738b5d6a8716058e82aceff40524b476a6f533f9a94887352cb8f30"
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p3b.spec
```
  Expected: `OK 2 replacements`. Other output → [STUCK].

- [ ] **Step 38** [V] ~2m: **JS LINT ON 1.90.0** (fetches the tool and checks its sha256).
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && ./scripts/check-js-lint.sh | tail -1
```
  Expected: `[PASS] oxlint: 21 files, 57 rules, no findings`. The rule count may rise with a new release; that is fine with `no findings`. Any finding → `git checkout -- scripts/lib/tool-pins.sh`, [STUCK], and Step 39 will show oxlint STALE.

- [ ] **Step 39** [V] ~2m: **FRESHNESS** (reads the GitHub and PyPI APIs).
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && ./scripts/check-tool-freshness.sh | grep -E 'STALE|FAIL'; ./scripts/check-tool-freshness.sh >/dev/null; echo rc=$?
```
  Expected: no STALE line, then `rc=0`. syft 1.56.0, grype 0.121.1 and ansible-core 2.22.0 were inside their cooldown on 2026-11-04 and will flip to STALE as it ends. A STALE line for one of those three → append it under "For Stevie" and carry on: a bump there needs a sha256 or a re-lock this plan did not measure. Any other STALE line → [STUCK].

- [ ] **Step 40** [C] ~1m: commit oxlint.
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 37-40: oxlint 1.90.0' scripts/lib/tool-pins.sh
```

- [ ] **Step 41** [V] ~4m: **PHASE 3 EXIT GATE** — the gates still bite (rule 6; needs the clean tree the commits just left).
```bash
cd /home/govan/tmp/wt/s-supply-first-pyci && git status --short | wc -l && PATH=/var/tmp/s-supply-first/civenv2/bin:$PATH ./scripts/check-gates-can-fail.sh --only check-tool-freshness 2>&1 | tail -2; PATH=/var/tmp/s-supply-first/civenv2/bin:$PATH ./scripts/check-gates-can-fail.sh --only check-ruff 2>&1 | tail -2
```
  Expected: `0`, then each run ending `PASS: 1 gate(s) proven to fail when violated`. Other output → [STUCK]; do not report the lane done.

---

## PHASE 4: GRIMOIRE (Steps 42-56)

**Goal**: Grimoire stays (DP-719) on current libraries, with its code patched and proven end to end.
**Prerequisite**: Phase 0 exit gate
**Time**: 40m
**Agent**: Agent [P] · **Lane**: `grimoire` · **Model**: Opus

Opus by rule 13, not by difficulty: these files run as root on the Tomb VM (`setup-grimoire.sh`, `become: true`, `sudo -n`). The steps stay exact. Nothing here touches a live host: all proof is a throwaway non-root container on west.

Network: PyPI, download.pytorch.org and huggingface.co (`all-MiniLM-L6-v2`, already the configured model), all from inside a throwaway container. Package code never runs on the host (`ci-python-lock.sh`'s rule). Only Grimoire's CPU torch floor moves here, to the version the smoke proves. The GPU stack move is FND-013, a forge evening after E1 (pack row 25.7), and `raft/` is not touched.

- [ ] **Step 42** [V] ~2m: **THE ADVISORIES ARE AS MEASURED**.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && /var/tmp/s-supply-first/civenv/bin/pip-audit --no-deps --disable-pip --progress-spinner off -f json -r tomb/grimoire/rag/requirements.in 2>/dev/null | python3 -I -c "
import json,sys
print(sorted({d['name']+' '+v['id'] for d in json.load(sys.stdin)['dependencies'] for v in d.get('vulns',[])}))"
```
  Expected, exactly one line listing 10 entries: `chromadb` with PYSEC-2026-3813, -3814, -3815; `sentence-transformers` with PYSEC-2026-4164; `transformers` with PYSEC-2025-217, PYSEC-2026-2288, -2289, -2290, -3929, -4174. An id not in that list → copy it under "For Stevie" and continue.

- [ ] **Step 43** [W] ~2m: `requirements.txt`, plus the accepted list's out-of-date reason. The file stays outside `check-python-audit.sh` because torch, markdown, beautifulsoup4, tqdm and pyyaml remain ranges. The accepted list keeps its two rows; only a comment changes.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && cat > /var/tmp/s-supply-first/p4a.spec <<'EOF'
tomb/grimoire/rag/requirements.txt ||| chromadb>=0.4.22,<1.0.0 ||| chromadb==1.5.9
tomb/grimoire/rag/requirements.txt ||| torch>=2.1.0,<3.0.0 ||| torch>=2.14.1,<3.0.0
tomb/grimoire/rag/requirements.txt ||| sentence-transformers>=2.2.2,<3.0.0 ||| sentence-transformers==6.2.0
tomb/grimoire/rag/requirements.txt ||| transformers>=4.36.0,<5.0.0 ||| transformers==5.20.2
tomb/grimoire/rag/requirements.txt ||| tokenizers>=0.15.0,<1.0.0 ||| tokenizers==0.23.3
docs/policy/python-audit-accepted.txt ||| local deserialization in the pt2 loader; no fixed release yet. Load only ||| local deserialization in the pt2 loader; pip-audit reports 2.14.1 clean (2026-11-04), the FND-013 move clears it. Load only
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p4a.spec
```
  Expected: `OK 6 replacements`. Other output → [STUCK].

- [ ] **Step 44** [W] ~2m: `requirements.in`, whole file.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && cat > tomb/grimoire/rag/requirements.in <<'EOF'
# Direct dependencies of the Grimoire RAG pipeline, exact versions
# (S-TRUE-WIRE Step 374; bumped by S-SUPPLY-FIRST 2026-11-04, DP-719). Lock with:
#   scripts/ci-python-lock.sh tomb/grimoire/rag
#
# NOT LOCKED YET: the lock pulls in about 127 packages, 101 absent from
# docs/policy/dependency-baseline.txt (ADR-096 admissions), 19 of them
# CUDA/NVIDIA wheels and triton that a CPU-only Tomb VM does not need
# (PyTorch's CPU index would drop them). Until then requirements.txt keeps
# exact pins for what was bumped and ranges for the rest, and those ranges
# keep it outside scripts/check-python-audit.sh.
#
# chromadb 1.5.9 is the newest release and is still inside PYSEC-2026-311,
# -3813, -3814 and -3815 (last_affected 1.5.9, no fixed release). All four are
# HTTP server endpoints or the RBAC provider; Grimoire runs the embedded
# PersistentClient and no Chroma server (FND-032).
# torch 2.14.1 is the version the smoke proves, CPU only. The GPU stack move is
# FND-013, after E1.
chromadb==1.5.9
sentence-transformers==6.2.0
torch==2.14.1
transformers==5.20.2
tokenizers==0.23.3
markdown==3.11.2
beautifulsoup4==4.15.0
tqdm==4.70.1
pyyaml==6.0.3
EOF
head -1 tomb/grimoire/rag/requirements.in
```
  Expected: `# Direct dependencies of the Grimoire RAG pipeline, exact versions`.

- [ ] **Step 45** [V] ~2m: **ONLY CHROMADB REMAINS**.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && /var/tmp/s-supply-first/civenv/bin/pip-audit --no-deps --disable-pip --progress-spinner off -f json -r tomb/grimoire/rag/requirements.in 2>/dev/null | python3 -I -c "
import json,sys
print(sorted({d['name']+' '+v['id'] for d in json.load(sys.stdin)['dependencies'] for v in d.get('vulns',[])}))"
```
  Expected, exactly: `['chromadb PYSEC-2026-311', 'chromadb PYSEC-2026-3813', 'chromadb PYSEC-2026-3814', 'chromadb PYSEC-2026-3815']`. A non-chromadb entry means a newer advisory → [STUCK].

- [ ] **Step 46** [W] ~2m: patch the code to fit. Each `old` is unique.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && cat > /var/tmp/s-supply-first/p4b.spec <<'EOF'
tomb/grimoire/rag/rag-index.py |||     import chromadb\n    from sentence_transformers import SentenceTransformer |||     import chromadb\n    from chromadb.errors import NotFoundError\n    from sentence_transformers import SentenceTransformer
tomb/grimoire/rag/rag-index.py |||     except ValueError:\n        pass |||     except NotFoundError:\n        pass
tomb/grimoire/rag/rag-index.py ||| logger.info("Embedding model loaded (dimension: %d)", model.get_sentence_embedding_dimension()) ||| logger.info("Embedding model loaded (dimension: %d)", model.get_embedding_dimension())
tomb/grimoire/rag/rag-index.py ||| logger.info("  Embedding dim: %d", model.get_sentence_embedding_dimension()) ||| logger.info("  Embedding dim: %d", model.get_embedding_dimension())
tomb/grimoire/rag/rag-query.py |||         # Query each collection and merge results |||         from chromadb.errors import NotFoundError\n\n        # Query each collection and merge results
tomb/grimoire/rag/rag-query.py |||             except ValueError:\n                logger.warning("Collection not found: %s", coll_name) |||             except NotFoundError:\n                logger.warning("Collection not found: %s", coll_name)
tomb/ansible/playbook-grimoire.yml |||           - chromadb\n          - sentence-transformers\n |||           - chromadb==1.5.9\n          - sentence-transformers==6.2.0\n
tomb/ansible/playbook-grimoire.yml |||               client = chromadb.Client(Settings(\n                  chroma_db_impl="duckdb+parquet",\n                  persist_directory=CHROMA_DIR,\n                  anonymized_telemetry=False,\n              ))\n |||               client = chromadb.PersistentClient(\n                  path=CHROMA_DIR,\n                  settings=Settings(anonymized_telemetry=False),\n              )\n
tomb/ansible/playbook-grimoire.yml |||               client.persist()\n ||| 
tomb/provision.sh |||     client = chromadb.Client(Settings(\n        chroma_db_impl=\"duckdb+parquet\",\n        persist_directory=chroma_dir,\n        anonymized_telemetry=False,\n    ))\n |||     client = chromadb.PersistentClient(\n        path=chroma_dir,\n        settings=Settings(anonymized_telemetry=False),\n    )\n
tomb/provision.sh ||| --find-links=${TOMB_STAGING}/rag-deps/ chromadb sentence-transformers ||| --find-links=${TOMB_STAGING}/rag-deps/ chromadb==1.5.9 sentence-transformers==6.2.0
tomb/provision.sh ||| pip download -d tomb/packages/grimoire/rag-deps/ chromadb sentence-transformers ||| pip download -d tomb/packages/grimoire/rag-deps/ chromadb==1.5.9 sentence-transformers==6.2.0
tomb/provision.sh ||| pip3 install chromadb sentence-transformers ||| pip3 install chromadb==1.5.9 sentence-transformers==6.2.0
tomb/Makefile ||| pip download -d $(PACKAGES_DIR)/grimoire/rag-deps/ chromadb sentence-transformers ||| pip download -d $(PACKAGES_DIR)/grimoire/rag-deps/ chromadb==1.5.9 sentence-transformers==6.2.0
tomb/cerydwyn/setup-cerydwyn.sh |||             chromadb \\n            sentence-transformers \\n |||             chromadb==1.5.9 \\n            sentence-transformers==6.2.0 \\n
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p4b.spec
```
  Expected: `OK 15 replacements`. Other output → [STUCK]; nothing was written.

- [ ] **Step 47** [V] ~2m: **PYTHON, YAML AND THE EMBEDDED SCRIPT STILL PARSE**. `tomb/provision.sh` carries its `rag-query.py` inside a double-quoted shell string, so the check undoes that quoting before compiling it.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && RUFF=/var/tmp/s-supply-first/civenv/bin/ruff ./scripts/check-ruff.sh | tail -1 && cat > /var/tmp/s-supply-first/p4f.py <<'EOF'
import re
import yaml
doc = yaml.safe_load(open('tomb/ansible/playbook-grimoire.yml'))
tasks = [t for p in doc for t in p.get('tasks', [])]
src = [t['ansible.builtin.copy']['content'] for t in tasks if 'grimoire-index.py' in str(t.get('ansible.builtin.copy', {}).get('dest', ''))]
compile(src[0], 'grimoire-index.py', 'exec')
print('playbook ok', len(src), 'duckdb' in src[0], 'persist()' in src[0])
s = open('tomb/provision.sh').read()
body = s.split("<< 'RAG_QUERY_EOF'", 1)[1].split('RAG_QUERY_EOF', 1)[0]
compile(re.sub(r'\\(["\\$`])', r'\1', body), 'rag-query.py', 'exec')
print('provision ok', 'duckdb' in s, 'chromadb==1.5.9' in s, 'PersistentClient' in body)
EOF
/var/tmp/s-supply-first/civenv/bin/python -I /var/tmp/s-supply-first/p4f.py; bash -n tomb/provision.sh && echo syntax ok
```
  Expected, exactly:
```
PASS: ruff 0 findings (with the exclusions above).
playbook ok 1 False False
provision ok False True True
syntax ok
```
  Other output → [STUCK].

- [ ] **Step 48** [W] ~2m: stage the smoke: the patched files, two small documents, and the script.
```bash
D=/var/tmp/s-supply-first/grimoire-smoke; mkdir -p $D/rag $D/docs && cd /home/govan/tmp/wt/s-supply-first-grimoire/tomb/grimoire/rag && cp rag-index.py rag-query.py rag-config.json $D/rag/ && sed -i 's#/opt/tomb/grimoire/rag/chroma.db#/tmp/chroma.db#' $D/rag/rag-config.json
cat > $D/docs/alpha.md <<'EOF'
# Wotan message bus

Wotan is the message bus of the Kingdom. It carries topics between services and keeps a ring buffer of recent events for late subscribers to replay.

## Topics

Every service publishes structured logs and metrics to its own topic so the dashboard can show a live view of the whole system at once.
EOF
cat > $D/docs/beta.md <<'EOF'
# The Well

The Well is the PostgreSQL database that holds kanban cards and audit events. Backups run nightly to a second disk and are proven by a restore drill.

## Roles

Each database has a nologin owner role and least privilege login roles, and connections use TLS 1.3 only.
EOF
cat > $D/smoke.sh <<'EOF'
set -e
export PIP_DISABLE_PIP_VERSION_CHECK=1 HOME=/tmp
python -m venv /tmp/v
/tmp/v/bin/pip install --quiet --no-cache-dir --only-binary=:all: --index-url https://download.pytorch.org/whl/cpu torch==2.14.1
/tmp/v/bin/pip install --quiet --no-cache-dir --only-binary=:all: chromadb==1.5.9 sentence-transformers==6.2.0 transformers==5.20.2 tokenizers==0.23.3 markdown==3.11.2 beautifulsoup4==4.15.0 tqdm==4.70.1 pyyaml==6.0.3
P="/tmp/v/bin/python -W error::FutureWarning"
C="--config /s/rag/rag-config.json"
$P /s/rag/rag-index.py $C --source-dir /s/docs --collection kingdom-docs --source-type kingdom 2>&1 | grep -E "Embedding dim|INDEXING COMPLETE|Traceback"
$P /s/rag/rag-index.py $C --source-dir /s/docs --collection kingdom-docs --source-type kingdom 2>&1 | grep -E "Deleted existing|Traceback"
$P /s/rag/rag-query.py -q x $C --list-collections 2>&1 | grep -E '"kingdom-docs"|Traceback'
$P /s/rag/rag-query.py -q "which database holds kanban cards" $C -k 1 2>&1 | grep -E '"source_file"|Traceback'
$P /s/rag/rag-query.py -q x $C -c nope 2>&1 | grep -E 'Collection not found|Traceback'
echo SMOKE-END
EOF
ls $D $D/rag | tr '\n' ' '
```
  Expected: `docs rag smoke.sh` and the three rag files listed.

- [ ] **Step 49** [V] ~6m: **GRIMOIRE RUNS** (throwaway container, as the normal user; `-W error::FutureWarning` fails on the renamed method).
```bash
D=/var/tmp/s-supply-first/grimoire-smoke; timeout 560 docker run --rm --user "$(id -u):$(id -g)" --network bridge -v $D:/s:ro python:3.12.15-slim sh /s/smoke.sh 2>&1 | sed -E 's/^[0-9TZ:-]+ //' | cut -c1-80 | tail -7; echo rc=${PIPESTATUS[0]}
```
  Expected, exactly:
```
[INFO] INDEXING COMPLETE
[INFO]   Embedding dim: 384
[INFO] Deleted existing collection: kingdom-docs
    "kingdom-docs"
        "source_file": "/s/docs/beta.md",
[WARNING] Collection not found: nope
SMOKE-END
rc=0
```
  A `Traceback` line or a missing line → Step 50.

- [ ] **Step 50** [D] ~5m: re-run without the filter into a file: `docker run --rm --user "$(id -u):$(id -g)" --network bridge -v /var/tmp/s-supply-first/grimoire-smoke:/s:ro python:3.12.15-slim sh /s/smoke.sh > /var/tmp/s-supply-first/grimoire-smoke.log 2>&1; tail -30 /var/tmp/s-supply-first/grimoire-smoke.log`. A pip or network error is an outside failure: retry once. A Python traceback → [STUCK] with the traceback; do not commit the bump.

- [ ] **Step 51** [V] ~2m: **GATES AGREE**.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && ./scripts/check-dependency-baseline.sh | tail -1; PIP_AUDIT=/var/tmp/s-supply-first/civenv/bin/pip-audit ./scripts/check-python-audit.sh | tail -1; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-sbom-trend.sh | grep python_components
```
  Expected: `[PASS] 590 third-party packages, all in the baseline`, `[PASS] 3 pinned requirements files; 2 known vulnerabilities, all accepted with a reason`, `OK python_components: 77`. Other output → [STUCK].

- [ ] **Step 52** [C] ~1m: **ONE COMMIT FOR GRIMOIRE**.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 42-52: grimoire: chromadb 1.5.9, sentence-transformers 6.2.0, transformers 5.20.2; code patched to fit' tomb/grimoire/rag/requirements.txt tomb/grimoire/rag/requirements.in tomb/grimoire/rag/rag-index.py tomb/grimoire/rag/rag-query.py tomb/ansible/playbook-grimoire.yml tomb/provision.sh tomb/cerydwyn/setup-cerydwyn.sh tomb/Makefile docs/policy/python-audit-accepted.txt
```

- [ ] **Step 53** [W] ~2m: track ChromaDB in the findings register, above FND-029. FND-032 is reserved for this plan.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && grep -c 'FND-032' compliance/findings/register.yaml; cat > /var/tmp/s-supply-first/p4c.spec <<'EOF'
compliance/findings/register.yaml |||   - id: FND-029\n |||   - id: FND-032\n    title: >-\n      No ChromaDB release is free of PYSEC-2026-311, -3813, -3814 and -3815 (last_affected 1.5.9); Grimoire pins\n      1.5.9, and check-python-audit does not audit its requirements.txt because other lines there are ranges\n    verify: every requirement in tomb/grimoire/rag/requirements.txt is an exact pin and scripts/check-python-audit.sh passes\n    severity: low\n    class: package\n    opened: 2026-11-04\n    exposure: >-\n      none on west or east: Grimoire runs only on a provisioned air-gapped Tomb VM, through the embedded\n      PersistentClient; all four advisories need a Chroma HTTP server or its RBAC provider\n    lockout: none\n    step: 3\n    plan: >-\n      Stevie's call (S-SUPPLY-FIRST For Stevie 1): pin the remaining ranges with four accepted lines, take a fixed\n      release when one ships, or replace ChromaDB in Grimoire.\n\n  - id: FND-029\n
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p4c.spec
```
  Expected: `0`, then `OK 1 replacements`. Other output → [STUCK].

- [ ] **Step 54** [V] ~2m: **THE REGISTER STILL LOADS**.
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test ./cmd/compliance-evidence/ 2>&1 | tail -1 | sed -E 's/[[:space:]]+(\(cached\)|[0-9.]+s)$//'; /var/tmp/s-supply-first/civenv/bin/python -I -c "
import yaml
print([f.get('id') for f in yaml.safe_load(open('compliance/findings/register.yaml'))['findings'] if f.get('id') in ('FND-030', 'FND-032')])"
```
  Expected: `ok  	unheaded/cmd/compliance-evidence`, then `['FND-030', 'FND-032']`. Other output → `git checkout -- compliance/findings/register.yaml`, then [STUCK].

- [ ] **Step 55** [V] ~2m: **THE FINDINGS TABLE GAINED ONE** (rule 16; the go test above uses synthetic registers).
```bash
E=/home/govan/tmp/unheaded/var/compliance/evidence.json; opencount() { (cd "$1" && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go run ./cmd/compliance-evidence findings -evidence "$E" | grep -oE '\*\*[0-9]+ open' | tr -dc 0-9); }; a=$(opencount /home/govan/tmp/wt/s-supply-first); b=$(opencount /home/govan/tmp/wt/s-supply-first-grimoire); echo "delta=$((b - a))"
```
  Expected: `delta=1`. It must never be below 0: this plan closes no finding. Other output → [STUCK].

- [ ] **Step 56** [C] ~1m: **PHASE 4 EXIT GATE + COMMIT** (Steps 45, 49, 51, 54, 55 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-grimoire && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 53-56: FND-032 tracks the unfixable chromadb advisories' compliance/findings/register.yaml && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 5: ACTIONS AND ADR WORDING (Steps 57-62)

**Goal**: DP-415 confirmed; DP-405's 14-day cooldown and ADR-004's wording land.
**Prerequisite**: Phase 0 exit gate
**Time**: 10m
**Agent**: Agent [P] · **Lane**: `ci` · **Model**: Haiku

- [ ] **Step 57** [V] ~1m: **DP-415 ALREADY DONE**. No edit follows.
```bash
cd /home/govan/tmp/wt/s-supply-first-ci && grep -c 'trivy-action@8f28faec26515256b6d22074d1d2a8986f4cbd87' .github/workflows/docker.yml .github/workflows/security.yml | sort; git grep -c '6abccf398f78' -- .github | wc -l
```
  Expected, exactly:
```
.github/workflows/docker.yml:2
.github/workflows/security.yml:3
0
```
  Other output → [STUCK]. Trivy stays pinned at this SHA.

- [ ] **Step 58** [W] ~1m: the 14-day cooldown. `auto-merge` is already absent: `git grep -n 'auto-merge' -- .github/workflows` prints nothing.
```bash
cd /home/govan/tmp/wt/s-supply-first-ci && cat > /var/tmp/s-supply-first/p5a.spec <<'EOF'
.github/dependabot.yml ||| # Step 323), Cargo since S-LOW-FRUIT. Actions and images wait out the 7-day\n# cooldown the gates enforce (ADR-096); Go and Rust code waits 14. Nothing\n ||| # Step 323), Cargo since S-LOW-FRUIT. Images wait out the 7-day cooldown the\n# gates enforce (ADR-096); Actions (DP-405), Go and Rust code wait 14. Nothing\n
.github/dependabot.yml |||       prefix: ci\n    cooldown:\n      default-days: 7\n |||       prefix: ci\n    cooldown:\n      default-days: 14\n
wiki/Security.md ||| | Dependabot | Grouped weekly PRs for actions, base images and Go modules, each after a 7-day cooldown | `.github/dependabot.yml` | ||| | Dependabot | Grouped weekly PRs for actions, base images, Go modules and Rust crates; base images wait 7 days, the rest 14; nothing auto-merges | `.github/dependabot.yml` |
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p5a.spec
```
  Expected: `OK 3 replacements`. Other output → [STUCK].

- [ ] **Step 59** [V] ~1m: **COOLDOWNS READ BACK**.
```bash
cd /home/govan/tmp/wt/s-supply-first-ci && /var/tmp/s-supply-first/civenv/bin/python -I -c "
import yaml
for u in yaml.safe_load(open('.github/dependabot.yml'))['updates']: print(u['package-ecosystem'], u['cooldown']['default-days'])"; git grep -n 'automerge\|auto-merge' -- .github/workflows | wc -l
```
  Expected, exactly:
```
github-actions 14
docker 7
gomod 14
cargo 14
0
```
  Other output → [STUCK].

- [ ] **Step 60** [W] ~2m: ADR-004. The Context sentence was reworded 2026-10-27; these two bullets are what is left, plus a record of the amendment.
```bash
cd /home/govan/tmp/wt/s-supply-first-ci && cat > /var/tmp/s-supply-first/p5b.spec <<'EOF'
docs/adr/ADR-004-no-external-deps-policy.md ||| - **Brand alignment**: Self-hosting validation extends to dependencies. A platform that builds its own metrics, logging, netlink, and eBPF libraries demonstrates deep systems expertise. ||| - **Self-hosting proof**: Self-hosting validation extends to dependencies. A platform that builds its own metrics, logging, netlink, and eBPF libraries can be read, audited and fixed by the people who run it.
docs/adr/ADR-004-no-external-deps-policy.md ||| - **Hiring friction**: Developers accustomed to using established libraries may find the policy unusual or unnecessary. The rationale must be clearly communicated. ||| - **Contributor friction**: Contributors accustomed to established libraries may find the policy unusual or unnecessary. The rationale must be clearly communicated.
docs/adr/ADR-004-no-external-deps-policy.md ||| ## Approved Exceptions Register\n ||| ## Amended 2026-11-04 (DP-405 Q3)\n\nCommercial framing removed under the community-first doctrine: the Context sentence on 2026-10-27 (`aad966c9`), the "Brand alignment" and "Hiring friction" bullets here. The policy itself is unchanged.\n\n## Approved Exceptions Register\n
docs/adr/ADR-095-slow-roll-structural-review.md ||| Q3 ADR-004's commercial wording is amended to the community-first doctrine (DP-405). ||| Q3 ADR-004's commercial wording is amended to the community-first doctrine (DP-405); done 2026-11-04, see ADR-004 "Amended 2026-11-04".
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p5b.spec
```
  Expected: `OK 4 replacements`. Other output → [STUCK].

- [ ] **Step 61** [V] ~1m: **NO COMMERCIAL WORD LEFT IN ADR-004**.
```bash
cd /home/govan/tmp/wt/s-supply-first-ci && grep -cEi 'brand|hiring|selling|sell ' docs/adr/ADR-004-no-external-deps-policy.md; ./scripts/check-actions-pinned.sh | tail -1
```
  Expected: `1` (the amendment note quotes "Brand alignment"), then `[PASS] all 149 uses: lines pinned to a commit SHA`. Other output → [STUCK].

- [ ] **Step 62** [C] ~1m: **PHASE 5 EXIT GATE + COMMIT** (Steps 57, 59, 61 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-ci && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 57-62: dependabot actions wait 14 days; ADR-004 wording; trivy-action pin confirmed' .github/dependabot.yml wiki/Security.md docs/adr/ADR-004-no-external-deps-policy.md docs/adr/ADR-095-slow-roll-structural-review.md && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 6: IMAGES (Steps 63-68)

**Goal**: measure what the pinned images carry at HIGH and CRITICAL with a fix available. That is narrower than DP-722's "anything", which this phase does not claim to close.
**Prerequisite**: Phase 0 exit gate; a builder slot (wave 2)
**Time**: 45m
**Agent**: Agent [P] · **Lane**: `images` · **Model**: Opus

Judgement phase: whether a tag bump is safe depends on the image. Network: registries, plus the trivy DB. Uses the pinned trivy image (`scripts/lib/tool-pins.sh`), never the local 0.72.0. No `compose up`, nothing restarts.

- [ ] **Step 63** [B] ~20m: scan the pinned third-party images from their registries; trivy pulls them itself, so no docker socket is mounted.
```bash
cd /home/govan/tmp/wt/s-supply-first-images && . scripts/lib/tool-pins.sh && mkdir -p /var/tmp/s-supply-first/trivy-cache /var/tmp/s-supply-first/img && for i in debian:bookworm-slim gcr.io/distroless/static-debian12:nonroot alpine:3.24 ubuntu:24.04 golang:1.27.3-alpine3.24 golang:1.27.3-bookworm python:3.12.15-slim clickhouse/clickhouse-server:24.12.6.70 coredns/coredns:1.12.0 grafana/grafana:11.5.1 haproxy:2.9.15-alpine nginx:1.25.5-alpine postgres:16.15-alpine timberio/vector:0.43.1-debian traefik:v3.3.7 victoriametrics/victoria-metrics:v1.108.1 grafana/loki:3.0.0 prom/alertmanager:v0.34.1 prom/prometheus:v3.15.0 qdrant/qdrant:v1.19.2 ghcr.io/huggingface/text-embeddings-inference:cpu-1.5 ghcr.io/open-webui/open-webui:main grafana/promtail:2.9.0 prom/node-exporter:v1.8.0 grafana/grafana:10.3.3 grafana/loki:2.9.4 grafana/promtail:2.9.4 prom/prometheus:v2.50.1 busybox:1.36.1; do n=$(echo "$i" | tr '/:' '__'); timeout 600 docker run --rm --user "$(id -u):$(id -g)" -e HOME=/cache -v /var/tmp/s-supply-first/trivy-cache:/cache -v /var/tmp/s-supply-first/img:/out "aquasec/trivy:${TRIVY_VERSION}@${TRIVY_IMAGE_DIGEST}" image --cache-dir /cache --quiet --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed --exit-code 0 -f json -o "/out/$n.json" "$i" && echo "scanned $i" || echo "FAILED $i"; done
```
  Left out on purpose: `rocm/pytorch` (FND-013), `docker/hosts/` (DP-401 deletes it), `vllm/vllm-openai:latest-rocm` (DP-713), and the rest of the k8s manifests (S-MOVE-OUT).

- [ ] **Step 64** [R] ~5m: summarise.
```bash
python3 -I - <<'EOF'
import glob, json, os
for f in sorted(glob.glob('/var/tmp/s-supply-first/img/*.json')):
    j = json.load(open(f)); c = h = 0; ex = []
    for r in j.get('Results') or []:
        for v in r.get('Vulnerabilities') or []:
            c += v['Severity'] == 'CRITICAL'; h += v['Severity'] == 'HIGH'
            ex.append(f"{v['PkgName']} {v['VulnerabilityID']}")
    print(f"{os.path.basename(f)[:-5]:55} critical={c} high={h} {sorted(set(ex))[:3]}")
EOF
```

- [ ] **Step 65** [DECIDE] ~10m: bump rule. RECOMMENDATION: take a tag only if it stays in the same minor series (patch level), the registry's tag is at least 14 days old and it removes a CRITICAL or HIGH finding. Prove each exists and read its age with `docker buildx imagetools inspect <image>:<tag> --format '{{json .Image}}'` (the `created` field; `docker manifest inspect` shows no age). Rationale: the stack is down, nothing can be exercised live here, and a minor upgrade of a data store is not an unattended change. Override ONLY if a CRITICAL has a KEV entry: then record it under "For Stevie" as urgent. **Never** bump `postgres` (ADR-102: backup and restore drill first; S-E1-LIVE), and leave anything else needing a minor or major move, or a data migration (grafana, clickhouse, nginx 1.25, for instance), as a "For Stevie" row with its counts.

- [ ] **Step 66** [W] ~5m: apply the tags that passed, every occurrence of an image at once (ADR-102 rule 3, one version per image), edits named by file. `git grep -n '<old tag>'` lists them first.

- [ ] **Step 67** [V] ~3m: **IMAGE GATES**.
```bash
cd /home/govan/tmp/wt/s-supply-first-images && ./scripts/check-image-tags.sh 2>&1 | tail -5; ./scripts/check-hadolint.sh 2>&1 | tail -1
```
  `check-image-tags.sh` already exits 1 on develop, so expect exactly these three stale rows under one `[FAIL]` line, and fail only on anything else: `docker/firewall/ipfire/Dockerfile ubuntu:24.04`, `docker/firewall/opnsense/Dockerfile ubuntu:24.04`, `kubernetes/manifests/base/haproxy-ingress/controller.yaml haproxytech/kubernetes-ingress:3.1`. Do not run `--update` (rule 17): S-CLEAN-CUT Step 118 owns that cleanup. `check-hadolint.sh` must pass. Any other failure → revert that image's edit, record why under "For Stevie".

- [ ] **Step 68** [C] ~2m: **PHASE 6 EXIT GATE + ONE COMMIT FOR IMAGES**. Write the summary table from Step 64 into `docs/security/image-scan-2026-11-04.md` (SPDX header, date, tool and DB version, table, what moved, what waits). Commit that file and any changed Dockerfiles or compose files, by name, with `/var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 63-68: images measured; patch-level bumps' <files>`. The gate: every image scanned or its failure recorded, `git status --short | wc -l` prints `0`.

---
## PHASE 7: LICENCE CANDIDATES (Steps 69-71)

**Goal**: Stevie's veto list exists before any licence text changes.
**Prerequisite**: Phase 0 exit gate
**Time**: 5m
**Agent**: Agent [P] · **Lane**: `licence` · **Model**: Haiku

- [ ] **Step 69** [W] ~2m: write the list. Every row here stays untouched by Phases 8 and 9.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && cat > docs/legal/RELICENSE-CANDIDATES-2026-11-04.md <<'EOF'
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# Relicense candidates for Stevie's veto (DP-08)

DP-08 relicensed first-party files to GPL-3.0-or-later and asked for anything that looks deliberately different to be listed first. S-SUPPLY-FIRST left every row below exactly as it found it. Answer in the last column; "yes" means change it to GPL-3.0-or-later.

| # | Where | Says | Why it might be on purpose | Recommendation | Stevie |
|---|---|---|---|---|---|
| 1 | `dashboard/css/design-system.css:1`, `cmd/dashboard-backend/static/css/design-system.css:1` | `BUSL-1.1` | Not permissive and not free. The next line reads "Soul of unheaded.org, engineering of bellis.tech", so it may have come from a stylesheet shared with a site under other terms | Change, unless it is shared with something outside this repo | |
| 2 | `api/openapi.yaml:36` | `license: name: MIT` | An API description is what other people generate clients from; permissive would be a reasonable choice | Change, or match `LICENSE-PROTOCOLS` (GPL-3.0 / Apache-2.0) if client reuse was the point | |
| 3 | `crates/monad-mbc/Cargo.toml:6` | `license = "MIT"` | The MBC ISA is a spec other implementations may want. All 21 source headers already say GPL-3.0-or-later, so field and headers disagree | Change | |
| 4 | `nix/packages/wotan.nix:24`, `nix/containers/wotan.nix:132` | MIT | They describe `github.com/unheaded/wotan`, once a separate repository. The in-tree `services/wotan` is GPL-3.0-or-later | Change, unless that repository stays MIT | |
| 5 | `crates/lich-fuzz/Cargo.toml:17` | `license = "UNLICENSED"`, `publish = false` | Offensive tooling may be held back on purpose. The source headers say GPL-3.0-or-later | Change | |
| 6 | `crates/doom-runner/Cargo.toml:5` and six of its sources | `GPL-3.0-only` | Copyleft, not permissive, but deliberately narrower than or-later. It leaves for `unheaded-doom` in S-MOVE-OUT | Leave it for that repository | |
| 7 | `github.com/unheaded/doomgeneric` (outside this repo; `.github/workflows/ci.yml:203` says "SPDX MIT in every pkg/doom file") | MIT | First-party, and a go-licenses ignore rests on this | Decide with S-MOVE-OUT | |
| 8 | `crates/monad-mbc/fuzz/Cargo.toml` | no `license` field | It links `monad-common`, whose manifest says GPL-2.0-only, and it is a child of row 3 | Decide with row 3 | |
| 9 | `pkg/iac/chef.go:133` | writes `license 'MIT'` into generated cookbooks | Generated output belongs to the adopter, so this is a product question, not a header | Stevie's call: what a generated cookbook should declare | |
| 10 | `skills/unheaded-librarian/references/interchangeability-template.md:131` | "Unheaded adapter configs are MIT-licensed." | A template other docs are stamped from; adapter configs may have been meant to be copied freely | Change | |

## Not candidates, deliberately unchanged

- eBPF and kernel-side code that needs GPL-2.0: `ebpf/Cargo.toml`, `ebpf/af-xdp`, `cmd/ebpf-loader`, `cmd/ebpf-load-check`, `arch/mbc/`, and the crates with no licence field that compile into BPF (`ebpf/*`, `cmd/ebpf-collector/ebpf-programs`, `cmd/ebpf-collector/common`, `crates/heimdall-bpf`).
- `docs/protocol/`: dual GPL-3.0 / Apache-2.0 under `LICENSE-PROTOCOLS`.
- Vendored upstream code: `crates/xv6-mbc/upstream/` (MIT), `llama.cpp/`, `doom/`.
- Third-party licence tables: `LICENSES/`, `THIRD_PARTY.md`, `docs/compliance/`, `docs/legal/`.
- History: `docs/archive/`, `docs/battle-plans/`, `docs/sweeps/`.
EOF
head -5 docs/legal/RELICENSE-CANDIDATES-2026-11-04.md | tail -1
```
  Expected: `# Relicense candidates for Stevie's veto (DP-08)`.

- [ ] **Step 70** [V] ~1m: **TEN ROWS, AND EACH STILL SAYS WHAT THE LIST CLAIMS**.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && grep -cE '^\| ([1-9]|10) \|' docs/legal/RELICENSE-CANDIDATES-2026-11-04.md; git grep -c 'BUSL-1.1' -- dashboard/css/design-system.css cmd/dashboard-backend/static/css/design-system.css; grep -n 'name: MIT' api/openapi.yaml; grep -n '^license' crates/monad-mbc/Cargo.toml crates/lich-fuzz/Cargo.toml crates/doom-runner/Cargo.toml | sort; grep -c '^license' crates/monad-mbc/fuzz/Cargo.toml; grep -n "license 'MIT'" pkg/iac/chef.go; grep -c 'adapter configs are MIT-licensed' skills/unheaded-librarian/references/interchangeability-template.md
```
  Expected, exactly:
```
10
cmd/dashboard-backend/static/css/design-system.css:1
dashboard/css/design-system.css:1
36:    name: MIT
crates/doom-runner/Cargo.toml:5:license = "GPL-3.0-only"
crates/lich-fuzz/Cargo.toml:17:license = "UNLICENSED"
crates/monad-mbc/Cargo.toml:6:license     = "MIT"
0
133:license 'MIT'
1
```
  Other output → [STUCK].

- [ ] **Step 71** [C] ~1m: **PHASE 7 EXIT GATE + COMMIT** (Step 70 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 69-71: relicense candidates listed for veto' docs/legal/RELICENSE-CANDIDATES-2026-11-04.md && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 8: LICENCE WORDING THE SPDX PASS COULD NOT SEE (Steps 72-78)

**Goal**: comment headers, image labels and prose stop claiming MIT.
**Prerequisite**: Phase 7 exit gate
**Time**: 15m
**Agent**: Agent [P] · **Lane**: `licence` · **Model**: Haiku

Do not touch `lxd/containers/`: DP-401 deletes those YAML files in S-CLEAN-CUT, and Phase 14 sweeps anything left. Rule 13 holds here because nothing that executes changes: only `LABEL` values, comments and prose. The diff check proves it for the Dockerfiles.

- [ ] **Step 72** [B] ~1m: 25 image labels. These ship in every built image.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && git grep -l 'org.opencontainers.image.licenses="MIT"' > /var/tmp/s-supply-first/labels.txt; wc -l < /var/tmp/s-supply-first/labels.txt; xargs sed -i 's/org\.opencontainers\.image\.licenses="MIT"/org.opencontainers.image.licenses="GPL-3.0-or-later"/' < /var/tmp/s-supply-first/labels.txt
```
  Expected: `25`. Other count → `git checkout -- docker`, then [STUCK].

- [ ] **Step 73** [V] ~1m: **LABELS CHANGED AND NOTHING ELSE**.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && git grep -c 'image.licenses="MIT"' | wc -l; git diff -U0 -- docker | grep -E '^[-+] ' | sed -E 's/^([-+]) +/\1/' | sed -E 's/ \\$//' | sort | uniq -c
```
  Expected, exactly:
```
0
     25 +org.opencontainers.image.licenses="GPL-3.0-or-later"
     25 -org.opencontainers.image.licenses="MIT"
```
  Other output → [STUCK].

- [ ] **Step 74** [W] ~2m: nine comment headers, and the one plain line.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && cat > /var/tmp/s-supply-first/p8a.spec <<'EOF'
monitoring/docker-compose.yml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
monitoring/loki/loki-security.yaml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
monitoring/loki/loki.yaml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
monitoring/prometheus/prometheus-host-b.yml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
monitoring/prometheus/prometheus.yml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
monitoring/promtail/promtail-host-a.yaml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
monitoring/promtail/promtail-host-b.yaml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
monitoring/victoriametrics/vm-config.yml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
lxd/profiles/unheaded-observability.yaml ||| # License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| # SPDX-License-Identifier: GPL-3.0-or-later\n# Copyright (c) 2024-2026 Stevie Bellis.
docs/planning/FILES_CREATED.txt ||| License: MIT, Copyright (c) 2024-2026 Stevie Bellis ||| License: GPL-3.0-or-later, Copyright (c) 2024-2026 Stevie Bellis
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p8a.spec
```
  Expected: `OK 10 replacements`. Other output → [STUCK]; nothing was written.

- [ ] **Step 75** [W] ~3m: the prose claims. Third-party statements on neighbouring lines (OPNsense, IPFire, Suricata) stay.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && cat > /var/tmp/s-supply-first/p8b.spec <<'EOF'
lxd/NEW_FILES_CREATED.md ||| All Unheaded Kingdom LXD infrastructure files are released under the MIT License. ||| All Unheaded Kingdom LXD infrastructure files are released under GPL-3.0-or-later.
lxd/README.md ||| All LXD infrastructure files, scripts, and configurations are released under the MIT License. ||| All LXD infrastructure files, scripts, and configurations are released under GPL-3.0-or-later.
lxd/hosts/ARCHITECTURE.md ||| All configuration files are licensed under MIT License: ||| All configuration files are licensed under GPL-3.0-or-later:
lxd/hosts/FILE_MANIFEST.md ||| All configuration files are licensed under MIT License: ||| All configuration files are licensed under GPL-3.0-or-later:
lxd/CREATED_FILES.md ||| All profiles include SPDX MIT license headers and comprehensive comments. ||| All profiles include SPDX GPL-3.0-or-later license headers and comprehensive comments.
lxd/CREATED_FILES.md ||| All cloud-init templates include SPDX MIT license headers. ||| All cloud-init templates include SPDX GPL-3.0-or-later license headers.
lxd/INDEX.md ||| **SPDX-License-Identifier:** MIT ||| **SPDX-License-Identifier:** GPL-3.0-or-later
routing/FILE_PATHS.txt ||| License: MIT (all files) ||| License: GPL-3.0-or-later (all files)
routing/INDEX.md ||| **License**: MIT ||| **License**: GPL-3.0-or-later
routing/MANIFEST.md ||| **License**: MIT ||| **License**: GPL-3.0-or-later
docs/network/INGRESS_EGRESS_PORTS.md ||| **License**: MIT ||| **License**: GPL-3.0-or-later
docs/network/MONAD_HBH_FIREWALL_RULES.md ||| **License**: MIT ||| **License**: GPL-3.0-or-later
docs/network/README.md ||| - **License**: MIT ||| - **License**: GPL-3.0-or-later
docs/network/README.md ||| - **Unheaded**: MIT (copyleft compatible) ||| - **Unheaded**: GPL-3.0-or-later (copyleft)
docs/network/README.md ||| - Compliance and licensing (BSD 2-Clause, GPL v3, MIT) ||| - Compliance and licensing (BSD 2-Clause, GPL v3)
docs/network/FIREWALL_TOPOLOGY.md ||| - Unheaded (MIT) remains MIT; OPNsense binaries retain BSD 2-Clause ||| - Unheaded (GPL-3.0-or-later) stays GPL-3.0-or-later; OPNsense binaries retain BSD 2-Clause
docs/network/FIREWALL_TOPOLOGY.md ||| - Unheaded (MIT) is NOT linked to IPFire; remains MIT ||| - Unheaded (GPL-3.0-or-later) is NOT linked to IPFire; stays GPL-3.0-or-later
docs/network/FIREWALL_TOPOLOGY.md ||| **License**: MIT (Unheaded), BSD 2-Clause (OPNsense components), GPL v3 (IPFire components) ||| **License**: GPL-3.0-or-later (Unheaded), BSD 2-Clause (OPNsense components), GPL v3 (IPFire components)
docs/research/FUTURE_INTEGRATIONS.md ||| Suricata is GPL-2.0. Unheaded is MIT. Suricata runs as a separate process/container ||| Suricata is GPL-2.0. Unheaded is GPL-3.0-or-later. Suricata runs as a separate process/container
nix/README.md ||| MIT - See [LICENSE](/LICENSE) ||| GPL-3.0-or-later - See [LICENSE](/LICENSE)
docker/INDEX.md ||| - Proper licensing (MIT) ||| - Proper licensing (GPL-3.0-or-later)
docker/MANIFEST.md ||| - licenses (MIT), vendor, source URL ||| - licenses (GPL-3.0-or-later), vendor, source URL
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p8b.spec
```
  Expected: `OK 22 replacements`. Other output → [STUCK]; nothing was written.

- [ ] **Step 76** [V] ~1m: **ONLY DP-401'S FILES STILL SAY MIT**.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && git grep -lIE '^[#!;/ *-]*License: MIT|released under the MIT|licensed under MIT|\*\*License\*\*: MIT|Unheaded \(MIT\)|Unheaded is MIT|image\.licenses="MIT"' -- . ':!llama.cpp' ':!doom' ':!crates/xv6-mbc/upstream' ':!docs/battle-plans' ':!docs/archive' ':!docs/sweeps' ':!LICENSES' ':!sbom-results' ':!docs/legal' ':!docs/compliance' ':!docs/sessions' ':!references' | sort
```
  Expected, exactly:
```
lxd/containers/grafana.yaml
lxd/containers/loki.yaml
lxd/containers/prometheus.yaml
lxd/containers/promtail.yaml
lxd/containers/victoriametrics.yaml
```
  Other output → [STUCK] with the extra paths.

- [ ] **Step 77** [V] ~2m: **YAML STILL PARSES; SPDX GATE AGREES; CANDIDATES UNTOUCHED**.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && /var/tmp/s-supply-first/civenv/bin/python -I -c "
import glob, yaml
n = 0
for f in glob.glob('monitoring/**/*.y*ml', recursive=True) + ['lxd/profiles/unheaded-observability.yaml']:
    list(yaml.safe_load_all(open(f))); n += 1
print('yaml ok', n)"; ./scripts/check-spdx.sh --langs go,rs --forbid MIT | tail -1 | sed -E 's/OK [0-9]+ file/OK N file/'; git diff --stat -- api dashboard cmd/dashboard-backend nix/packages nix/containers crates | wc -l
```
  Expected: a line starting `yaml ok`, then `check-spdx: OK N file(s) carry an SPDX header, none declares MIT (langs: go,rs)`, then `0`. A YAML error → [STUCK] with the file.

- [ ] **Step 78** [C] ~1m: **PHASE 8 EXIT GATE + COMMIT** (Steps 73, 76, 77 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 72-78: GPL-3.0-or-later in image labels, comment headers and prose the SPDX pass missed' $(cat /var/tmp/s-supply-first/labels.txt) $(awk -F' \\|\\|\\| ' '{print $1}' /var/tmp/s-supply-first/p8a.spec /var/tmp/s-supply-first/p8b.spec | sort -u) && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 9: CARGO LICENCE FIELDS AND ZHEND'S TEXT (Steps 79-82)

**Goal**: `zhend` and `zhenai-forge` read GPL-3.0-or-later everywhere (Stevie, pack round 4), and `collector` gains its field.
**Prerequisite**: Phase 8 exit gate
**Time**: 5m
**Agent**: Agent [P] · **Lane**: `licence` · **Model**: Haiku

Not touched: `crates/monad-mbc`, `crates/monad-mbc/fuzz`, `crates/lich-fuzz`, `crates/doom-runner` (candidates); `cmd/waf` (leaves in S-CLEAN-CUT); `cmd/trace-collector` (DP-210; Phase 14 checks); everything GPL-2.0 or kernel-side (the "Not candidates" list). The doc-comment lines below are comments: no code changes.

- [ ] **Step 79** [W] ~2m: three manifests and sixteen text lines.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && cat > /var/tmp/s-supply-first/p9.spec <<'EOF'
crates/zhend/Cargo.toml ||| license = "GPL-3.0-only" ||| license = "GPL-3.0-or-later"
crates/zhenai-forge/Cargo.toml ||| license = "GPL-3.0-only" ||| license = "GPL-3.0-or-later"
cmd/ebpf-collector/collector/Cargo.toml ||| edition = "2021"\n ||| edition = "2021"\nlicense = "GPL-3.0-or-later"\n
crates/zhend/README.md ||| GPL-3.0-only -- The Library cannot be enclosed. ||| GPL-3.0-or-later -- The Library cannot be enclosed.
crates/zhend/SECURITY.md ||| **License**: GPL-3.0-only ||| **License**: GPL-3.0-or-later
crates/zhend/THIRD_PARTY.md ||| **License**: GPL-3.0-only ||| **License**: GPL-3.0-or-later
crates/zhend/THIRD_PARTY.md ||| All dependencies are compatible with GPL-3.0-only. ||| All dependencies are compatible with GPL-3.0-or-later.
crates/zhend/THIRD_PARTY.md ||| GPL-3.0-only (zhend) ||| GPL-3.0-or-later (zhend)
crates/zhend/THIRD_PARTY.md ||| allowing GPL-3.0-only distribution ||| allowing GPL-3.0-or-later distribution
crates/zhend/proto/zhen.proto ||| // GPL-3.0-only ||| // GPL-3.0-or-later
crates/zhend/src/lib.rs ||| //! GPL-3.0-only — The Library cannot be enclosed. ||| //! GPL-3.0-or-later — The Library cannot be enclosed.
crates/zhend/src/main.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
crates/zhend/src/api/grpc.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
crates/zhend/src/api/quic.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
crates/zhend/tests/phase2_exit_gate.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
crates/zhend/tests/phase6_grpc.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
crates/zhend/tests/phase8_quic.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
crates/zhend/tests/proptest_codec_fuzz.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
crates/zhend/tests/proptest_lifecycle.rs ||| //! GPL-3.0-only ||| //! GPL-3.0-or-later
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p9.spec
```
  Expected: `OK 19 replacements`. Other output → [STUCK]; nothing was written.

- [ ] **Step 80** [V] ~2m: **CARGO READS THEM, NOTHING CONTRADICTS, LOCKS UNCHANGED**.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && for m in crates/zhend crates/zhenai-forge cmd/ebpf-collector/collector; do cargo metadata --no-deps --offline --locked --format-version 1 --manifest-path $m/Cargo.toml | python3 -I -c "
import json, sys
m = sys.argv[1] + '/Cargo.toml'
print(*[p['name'] + ' ' + str(p['license']) for p in json.load(sys.stdin)['packages'] if p['manifest_path'].endswith(m)])" $m; done; git grep -c 'GPL-3.0-only' -- crates/zhend crates/zhenai-forge | wc -l; git status --short | grep -c 'Cargo.lock'; git diff -U0 -- crates/zhend/src crates/zhend/tests crates/zhend/proto | grep -E '^[-+]' | grep -vE '^(\+\+\+|---)' | grep -cv 'GPL-3.0'
```
  Expected, exactly:
```
zhend GPL-3.0-or-later
zhenai-forge GPL-3.0-or-later
collector GPL-3.0-or-later
0
0
0
```
  Other output → [STUCK].

- [ ] **Step 81** [V] ~1m: **GPL-2.0 FIELDS UNTOUCHED**.
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && grep -h '^license' ebpf/Cargo.toml ebpf/af-xdp/Cargo.toml cmd/ebpf-loader/Cargo.toml cmd/ebpf-load-check/Cargo.toml | sort | uniq -c
```
  Expected, exactly: `      4 license = "GPL-2.0-only"`. Other output → [STUCK].

- [ ] **Step 82** [C] ~1m: **PHASE 9 EXIT GATE + COMMIT** (Steps 80, 81 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-licence && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 79-82: zhend and zhenai-forge to GPL-3.0-or-later with their text; collector licence field' $(awk -F' \\|\\|\\| ' '{print $1}' /var/tmp/s-supply-first/p9.spec | sort -u) && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 10: OSV RECORDS AND VERSION ORDER (Steps 83-88)

**Goal**: `pkg/threatfeed/osv` parses untrusted records, compares versions per ecosystem and decides range membership.
**Prerequisite**: Phase 0 exit gate
**Time**: 20m
**Agent**: Agent [P] · **Lane**: `scanner` · **Model**: Sonnet

Signatures (bodies in Appendix C, tested on west 2026-11-04):

```go
package osv
const ( MaxRecordBytes = 4 << 20; EcoGo = "Go"; EcoCrates = "crates.io"; EcoPyPI = "PyPI" )
var ErrInvalid error
type Event struct{ Introduced, Fixed, LastAffected string }
type Range struct{ Type string; Events []Event }
type Package struct{ Name, Ecosystem string }
type Affected struct{ Package Package; Ranges []Range; Versions []string }
type Vuln struct{ ID string; Aliases []string; Summary, Withdrawn string; Affected []Affected }
func Parse(data []byte) (*Vuln, error)
func LoadDir(dir string) ([]Vuln, error)
func NormalizeName(eco, name string) string
func (a Affected) Matches(eco, name, version string) (bool, error)
func (a Affected) FirstFixed(eco string) string
func Compare(eco, a, b string) (int, error)
```

- [ ] **Step 83** [B] ~1m: test first.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && python3 -I /var/tmp/s-supply-first/extract.py pkg/threatfeed/osv/osv_test.go
```
  Expected: `wrote pkg/threatfeed/osv/osv_test.go`. The test holds: `TestCompare` (17 version pairs across Go, crates.io and PEP 440, including the pseudo-version and `2.14.1+cpu`), `TestParseRejects` (7 hostile inputs), `TestMatches` (11 range cases), `TestLoadDir` (empty directory and duplicate id refused).

- [ ] **Step 84** [V] ~1m: **RED**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && out=$(GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test ./pkg/threatfeed/osv/ 2>&1); echo "$out" | grep 'build failed'; echo "$out" | grep -m1 -o 'undefined: EcoGo'
```
  Expected, exactly: `FAIL	unheaded/pkg/threatfeed/osv [build failed]`, then `undefined: EcoGo`. Other output → [STUCK].

- [ ] **Step 85** [B] ~1m: the code.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && python3 -I /var/tmp/s-supply-first/extract.py pkg/threatfeed/osv/osv.go pkg/threatfeed/osv/version.go
```

- [ ] **Step 86** [V] ~2m: **GREEN**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && gofmt -l pkg/threatfeed/osv; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go vet ./pkg/threatfeed/osv/ && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race -v ./pkg/threatfeed/osv/ 2>&1 | grep -cE '^--- PASS' ; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race ./pkg/threatfeed/osv/ 2>&1 | tail -1 | sed -E 's/[[:space:]]+(\(cached\)|[0-9.]+s)$//'
```
  Expected: `4`, then `ok  	unheaded/pkg/threatfeed/osv`. Other output → compare the failing file with its Appendix C block: `git diff --no-index` against a fresh extract. Still failing → [STUCK].

- [ ] **Step 87** [B] ~3m: build gosec once for this lane from the module's own pins.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go -C tools/gosec build -o /var/tmp/s-supply-first/gosec-scan github.com/securego/gosec/v2/cmd/gosec && echo built
```
  Expected: `built`.

- [ ] **Step 88** [C] ~1m: **PHASE 10 EXIT GATE + COMMIT** (Step 86 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 83-88: pkg/threatfeed/osv: records, version order, range match' pkg/threatfeed/osv/osv_test.go pkg/threatfeed/osv/osv.go pkg/threatfeed/osv/version.go && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 11: INVENTORY FROM LOCK DATA (Steps 89-94)

**Goal**: `pkg/threatfeed/inventory` reads what a tree pins, and says what it could not check.
**Prerequisite**: Phase 10 exit gate
**Time**: 15m
**Agent**: Agent [P] · **Lane**: `scanner` · **Model**: Sonnet

Fixture files are named `fixture.go-mod`, `fixture.cargo-lock` and `fixture.reqs` on purpose: a tracked `Cargo.lock` or `requirements.txt` would count as a real package in `check-dependency-baseline.sh` and fail it.

```go
package inventory
type Component struct{ Ecosystem, Name, Version, Source string }
func ReadGoMod(r io.Reader, source string) ([]Component, error)         // skips modules replaced by a local path
func ReadCargoLock(r io.Reader, source string) ([]Component, error)     // registry sources only
func ReadRequirements(r io.Reader, source string) (comps []Component, unpinned []string, err error)
func ReadSBOMJSON(r io.Reader, source string) ([]Component, error)
func Discover(root string) ([]string, error)
func ScanTree(root string) (comps []Component, unpinned []string, err error) // unique, sorted
```

- [ ] **Step 89** [B] ~1m: test and fixtures first.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && python3 -I /var/tmp/s-supply-first/extract.py pkg/threatfeed/inventory/inventory_test.go pkg/threatfeed/inventory/testdata/fixture.go-mod pkg/threatfeed/inventory/testdata/fixture.cargo-lock pkg/threatfeed/inventory/testdata/fixture.reqs pkg/threatfeed/inventory/testdata/fixture.sbom-json
```
  Expected: five `wrote` lines. Five tests: one per reader, plus `TestScanTree` (dedup across files, `testdata/` and `target/` skipped, the ranged line reported).

- [ ] **Step 90** [V] ~1m: **RED**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && out=$(GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test ./pkg/threatfeed/inventory/ 2>&1); echo "$out" | grep 'build failed'; echo "$out" | grep -m1 -o 'undefined: ReadGoMod'
```
  Expected, exactly: `FAIL	unheaded/pkg/threatfeed/inventory [build failed]`, then `undefined: ReadGoMod`. Other output → [STUCK].

- [ ] **Step 91** [B] ~1m: the code.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && python3 -I /var/tmp/s-supply-first/extract.py pkg/threatfeed/inventory/inventory.go
```

- [ ] **Step 92** [V] ~2m: **GREEN**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && gofmt -l pkg/threatfeed/inventory; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go vet ./pkg/threatfeed/inventory/ && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race -v ./pkg/threatfeed/inventory/ 2>&1 | grep -cE '^--- PASS'; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race ./pkg/threatfeed/inventory/ 2>&1 | tail -1 | sed -E 's/[[:space:]]+(\(cached\)|[0-9.]+s)$//'
```
  Expected: `5`, then `ok  	unheaded/pkg/threatfeed/inventory`. Other output → [STUCK].

- [ ] **Step 93** [V] ~1m: **THE FIXTURES ADD NOTHING TO THE BASELINES**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && git add -N pkg/threatfeed/inventory && ./scripts/check-dependency-baseline.sh | tail -1 && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-sbom-trend.sh | grep -c '^OK'
```
  Expected: `[PASS] 590 third-party packages, all in the baseline`, then `3`. Other output → [STUCK].

- [ ] **Step 94** [C] ~1m: **PHASE 11 EXIT GATE + COMMIT** (Steps 92, 93 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 89-94: pkg/threatfeed/inventory: components from lock data' pkg/threatfeed/inventory/inventory_test.go pkg/threatfeed/inventory/inventory.go pkg/threatfeed/inventory/testdata/fixture.go-mod pkg/threatfeed/inventory/testdata/fixture.cargo-lock pkg/threatfeed/inventory/testdata/fixture.reqs pkg/threatfeed/inventory/testdata/fixture.sbom-json && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 12: CMD/VULNMATCH (Steps 95-101)

**Goal**: the matcher runs on local files, with KEV marks and no outbound code.
**Prerequisite**: Phase 11 exit gate
**Time**: 20m
**Agent**: Agent [P] · **Lane**: `scanner` · **Model**: Sonnet

```go
package main
type config struct{ root, osvDir, kevFile, sbomFile string; jsonOut, failOnFindings bool }
type Finding struct{ KEV bool; ID string; Aliases []string; Ecosystem, Package, Version, FixedIn, Source, Summary string }
func match(comps []inventory.Component, vulns []osv.Vuln, kevIDs map[string]bool) ([]Finding, error)
func loadKEV(path string) (map[string]bool, error)          // kev.Parse; "" means no catalog
func run(cfg config, stdout, stderr io.Writer) (int, error)
```

Contract: no `--osv-dir` is an error, so a scan with no data never reads clean. Withdrawn records never match. Lines that are not exact pins go to stderr as `not checked`, never as clean. Findings sort KEV first. Exit 0 complete, 1 findings with `--fail-on-findings`, 2 usage or scan error (`cmdline.Fail`).

- [ ] **Step 95** [B] ~1m: fixtures and test first. Everything is synthetic: `FIXTURE-*` ids, CVE-2099-*, packages that do not exist. No OSV data is copied in.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && python3 -I /var/tmp/s-supply-first/extract.py pkg/threatfeed/osv/testdata/records/FIXTURE-GO-0001.json pkg/threatfeed/osv/testdata/records/FIXTURE-PY-0001.json pkg/threatfeed/osv/testdata/records/FIXTURE-RS-0001.json pkg/threatfeed/osv/testdata/records/FIXTURE-WD-0001.json pkg/threatfeed/osv/testdata/kev.json cmd/vulnmatch/main_test.go
```
  Expected: six `wrote` lines. Three tests: `TestRunFindsAndOrders` (3 findings, KEV first, no-fix record shows no fix, summary line exact), `TestFailOnFindings`, `TestNoDataIsAnError`.

- [ ] **Step 96** [V] ~1m: **RED**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && out=$(GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test ./cmd/vulnmatch/ 2>&1); echo "$out" | grep 'build failed'; echo "$out" | grep -m1 -o 'undefined: config'
```
  Expected, exactly: `FAIL	unheaded/cmd/vulnmatch [build failed]`, then `undefined: config`. Other output → [STUCK].

- [ ] **Step 97** [B] ~1m: the code.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && python3 -I /var/tmp/s-supply-first/extract.py cmd/vulnmatch/main.go
```

- [ ] **Step 98** [V] ~3m: **GREEN, AND THE CMDLINE POLICY PASSES**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && gofmt -l cmd/vulnmatch; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go vet ./cmd/vulnmatch/ && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race -v ./cmd/vulnmatch/ 2>&1 | grep -cE '^--- PASS'; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race ./cmd/vulnmatch/ ./pkg/cmdline/ ./pkg/threatfeed/... 2>&1 | sed -E 's/[[:space:]]+(\(cached\)|[0-9.]+s)$//'
```
  Expected, exactly:
```
3
ok  	unheaded/cmd/vulnmatch
ok  	unheaded/pkg/cmdline
ok  	unheaded/pkg/threatfeed/inventory
ok  	unheaded/pkg/threatfeed/kev
ok  	unheaded/pkg/threatfeed/osv
```
  Other output → [STUCK].

- [ ] **Step 99** [V] ~3m: **LINT AND GOSEC CLEAN**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ~/.cache/unheaded-tools/bin/golangci-lint run --timeout 4m ./cmd/vulnmatch/... ./pkg/threatfeed/osv/... ./pkg/threatfeed/inventory/... 2>&1 | tail -1; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp /var/tmp/s-supply-first/gosec-scan -quiet ./cmd/vulnmatch/... ./pkg/threatfeed/osv/... ./pkg/threatfeed/inventory/...; echo gosec=$?
```
  Expected: `0 issues.`, then `gosec=0`. Other output → [STUCK] with the finding. Do not add a `#nosec`.

- [ ] **Step 100** [V] ~2m: **BEHAVES AS A KINGDOM BINARY; NO DATA, NO PASS**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go build -o /var/tmp/s-supply-first/vulnmatch ./cmd/vulnmatch && /var/tmp/s-supply-first/vulnmatch --version | cut -d' ' -f1,2; /var/tmp/s-supply-first/vulnmatch --root .; echo rc=$?; /var/tmp/s-supply-first/vulnmatch --root . --osv-dir pkg/threatfeed/osv/testdata/records --kev-file pkg/threatfeed/osv/testdata/kev.json 2>&1 | tail -2 | sed -E 's/^vulnmatch: [0-9]+ components/vulnmatch: N components/'; echo rc=${PIPESTATUS[0]}
```
  Expected, exactly:
```
vulnmatch (unheaded)
vulnmatch: --osv-dir is required
Try 'vulnmatch --help' for more information.
rc=2
KEV  ID  ECOSYSTEM  PACKAGE  VERSION  FIXED  SOURCE
vulnmatch: N components, 4 records, 0 findings (0 in KEV), 20 not an exact pin
rc=0
```
  `N` was 713 on 2026-11-04. Other output → [STUCK].

- [ ] **Step 101** [C] ~1m: **PHASE 12 EXIT GATE + COMMIT** (Steps 98, 99, 100 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 95-101: cmd/vulnmatch: DP-721 stage 1a, local files only, no outbound code' cmd/vulnmatch/main.go cmd/vulnmatch/main_test.go pkg/threatfeed/osv/testdata/kev.json pkg/threatfeed/osv/testdata/records/FIXTURE-GO-0001.json pkg/threatfeed/osv/testdata/records/FIXTURE-PY-0001.json pkg/threatfeed/osv/testdata/records/FIXTURE-RS-0001.json pkg/threatfeed/osv/testdata/records/FIXTURE-WD-0001.json && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 13: SCANNER RECORDS AND DOCS (Steps 102-108)

**Goal**: the new binary passes the repo's inventories, and the wiki says why OSV is not fetched.
**Prerequisite**: Phase 12 exit gate
**Time**: 15m
**Agent**: Agent [P] · **Lane**: `scanner` · **Model**: Haiku

- [ ] **Step 102** [W] ~1m: the local runner. It also gives `docs/LIVE-PATHS.md` a TOOL reference instead of an ORPHAN.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && cat > scripts/vulnmatch-fixture.sh <<'EOF'
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# vulnmatch-fixture.sh: run cmd/vulnmatch over this tree against the synthetic
# OSV and KEV fixtures. Local files only; it proves the reader and matcher end
# to end until the third-party sitting confirms a real OSV source (DP-721).
set -euo pipefail
cd "$(dirname "$0")/.."
exec go run ./cmd/vulnmatch --root . \
	--osv-dir pkg/threatfeed/osv/testdata/records \
	--kev-file pkg/threatfeed/osv/testdata/kev.json "$@"
EOF
chmod +x scripts/vulnmatch-fixture.sh && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/vulnmatch-fixture.sh 2>&1 | tail -1 | sed -E 's/^vulnmatch: [0-9]+ components/vulnmatch: N components/'
```
  Expected: `vulnmatch: N components, 4 records, 0 findings (0 in KEV), 20 not an exact pin`. Other output → [STUCK].

- [ ] **Step 103** [B] ~5m: the man page. `--write` restamps every page's `.TH` line, so keep only the new page.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp scripts/gen-man.sh --write | tail -1; git ls-files -m man | xargs -r git checkout --; git status --short man
```
  Expected: the last line is `?? man/vulnmatch.1`. Other output → [STUCK].

- [ ] **Step 104** [V] ~4m: **MAN GATE**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp scripts/gen-man.sh | tail -1 | sed -E 's/[0-9]+ binaries/N binaries/'
```
  Expected: `OK: man/ matches N binaries`. Other output → [STUCK].

- [ ] **Step 105** [B] ~1m: regenerate the live-path inventory, as its own header says to.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && git add -N scripts/vulnmatch-fixture.sh && scripts/live-path-inventory.sh --report > docs/LIVE-PATHS.md; grep -n 'cmd/vulnmatch' docs/LIVE-PATHS.md | sed -E 's/^[0-9]+://'; scripts/live-path-inventory.sh | tail -1
```
  Expected: `` | `cmd/vulnmatch` | TOOL | ``, then `[PASS] live-path inventory matches docs/LIVE-PATHS.md`. Other output → [STUCK].

- [ ] **Step 106** [W] ~2m: the wiki. No third-party inventory row: that table lists services the code calls, and this code calls none.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && cat > /var/tmp/s-supply-first/p13.spec <<'EOF'
wiki/SBOM.md ||| ## Output Formats\n ||| ## Native CVE matcher\n\n`cmd/vulnmatch` reads the components a tree pins (`go.mod`, `Cargo.lock`, `requirements*.txt`, `sbom/python-pip-freeze.txt`, optionally `sbom/*-dependencies.json`), matches them against OSV records in a local directory, and marks any CVE in kev-poller's catalog. It is stage 1a of DP-721: lock data only, replacing no trivy use yet. Trivy stays pinned until each stage replaces its use.\n\nToday it runs on synthetic fixtures only (`scripts/vulnmatch-fixture.sh`). OSV is an outside data source the third-party sitting has not confirmed, so the tool has no network code: it reads records from a directory and nothing else. Requirement lines that are not exact pins are reported as not checked, never as clean.\n\n## Output Formats\n
wiki/Security.md ||| | Dependency baseline | No third-party package ||| | vulnmatch | Native matcher, stage 1a of replacing trivy: lock data against local OSV records and the KEV catalog; no network code | `cmd/vulnmatch`, `scripts/vulnmatch-fixture.sh` |\n| Dependency baseline | No third-party package
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/p13.spec
```
  Expected: `OK 2 replacements`. Other output → [STUCK]; nothing was written.

- [ ] **Step 107** [V] ~1m: **NOTHING OUTBOUND**.
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && grep -rlE '"net/http"|"net"|api\.osv\.dev' cmd/vulnmatch pkg/threatfeed/osv pkg/threatfeed/inventory | wc -l; grep -n 'kev\.' cmd/vulnmatch/main.go | grep -v 'kev-file\|kev-poller' | sed -E 's/^[0-9]+:[[:space:]]*//'; grep -c 'Native CVE matcher' wiki/SBOM.md
```
  Expected, exactly:
```
0
c, err := kev.Parse(b)
1
```
  The binary links `pkg/threatfeed/kev`, which holds kev-poller's fetcher, so `net/http` is in it. What this proves is that this plan's packages import no network package and that `vulnmatch` calls only `kev.Parse`. Other output → [STUCK].

- [ ] **Step 108** [C] ~1m: **PHASE 13 EXIT GATE + COMMIT** (Steps 104, 105, 107 green).
```bash
cd /home/govan/tmp/wt/s-supply-first-scanner && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 102-108: vulnmatch man page, live paths, wiki' scripts/vulnmatch-fixture.sh man/vulnmatch.1 docs/LIVE-PATHS.md wiki/SBOM.md wiki/Security.md && git status --short | wc -l
```
  Expected: the commit line, then `0`.

---

## PHASE 14: INTEGRATE (Steps 109-123)

**Goal**: one green branch, merged into develop, with the final vulnerable list on record.
**Prerequisite**: every lane's exit gate passed or marked [STUCK] with a reason
**Time**: 1-2 h (the slow gates)
**Agent**: Coordinator · **Model**: Opus

Judgement phase: merge conflicts, whatever the lanes left [STUCK], and what S-CLEAN-CUT changed underneath this plan.

- [ ] **Step 109** [R] ~5m: read every lane's [STUCK] entries in the progress file and `/var/tmp/s-supply-first/unsigned.txt`. A lane with an unfinished exit gate is merged only if what it committed passed its own [V] steps.

- [ ] **Step 110** [W] ~2m: the merge helper. A signing failure falls back to unsigned and logs it; a conflict aborts and exits 3, so no merge is left half-done (rule 15).
```bash
cat > /var/tmp/s-supply-first/safe-merge.sh <<'EOF'
#!/usr/bin/env bash
# safe-merge.sh REF MSG: run in the worktree that receives the merge.
ref=$1; msg=$2
git merge --no-ff -S -q -m "$msg" "$ref" && exit 0
if [ -n "$(git diff --name-only --diff-filter=U)" ]; then
	git merge --abort
	echo "STUCK conflict merging $ref"
	exit 3
fi
git merge --abort 2>/dev/null
if ! git merge --no-ff --no-gpg-sign -q -m "$msg" "$ref"; then
	git merge --abort
	echo "STUCK conflict merging $ref"
	exit 3
fi
git rev-parse --short HEAD >>/var/tmp/s-supply-first/unsigned.txt
EOF
chmod +x /var/tmp/s-supply-first/safe-merge.sh
```

- [ ] **Step 111** [B] ~5m: merge the lanes, smallest risk first.
```bash
cd /home/govan/tmp/wt/s-supply-first && for l in ci rust licence go pyci grimoire scanner images; do /var/tmp/s-supply-first/safe-merge.sh sprint/s-supply-first-$l "[PLAN S-SUPPLY-FIRST] merge lane $l" || { echo "stopped at $l"; break; }; done; git log --oneline --merges --first-parent develop..HEAD | wc -l
```
  Expected: `8`. On `stopped at <lane>`, this is a conflict inside this plan, so Opus resolves it here: run `git merge --no-ff --no-commit sprint/s-supply-first-<lane>`, keep both sides (likely sites: the `wiki/Security.md` table, Dockerfile labels next to `FROM` lines), commit with `ci.sh` naming the files, then re-run the loop for the remaining lanes. Rule 15's [STUCK] applies to the next step.

- [ ] **Step 112** [B] ~5m: pick up what landed on develop meanwhile, S-CLEAN-CUT above all. A conflict here is [STUCK], no merge (rule 15).
```bash
cd /home/govan/tmp/wt/s-supply-first && /var/tmp/s-supply-first/safe-merge.sh develop "[PLAN S-SUPPLY-FIRST] merge develop" || exit 1; git status --short | wc -l; for f in cmd/trace-collector/Cargo.toml cmd/trace-collector/.cargo/audit.toml lxd/containers/grafana.yaml lxd/containers/loki.yaml lxd/containers/prometheus.yaml lxd/containers/promtail.yaml lxd/containers/victoriametrics.yaml; do [ -e "$f" ] && echo "STILL HERE $f"; done; grep -oE 'id: FND-03[0-9]' compliance/findings/register.yaml | sort | uniq -d
```

- [ ] **Step 113** [DECIDE] ~10m: residue. RECOMMENDATION: if S-CLEAN-CUT has merged, nothing should print. If it has not: leave those files and add a "For Stevie" line naming them, because they are that plan's to delete. If a file survived a merged S-CLEAN-CUT on purpose: fix it here (`license = "GPL-3.0-or-later"` in `cmd/trace-collector/Cargo.toml`, a `# SPDX-License-Identifier: GPL-3.0-or-later` header in the YAML, delete the `audit.toml`). An FND id printed by the `uniq -d` is a collision with the series' reserved numbers: stop and tell the coordinator. Override ONLY if the file is gone.

- [ ] **Step 114** [V] ~3m: **THE CBOM MATCHES THE MERGED TREE**. Post-merge on purpose: S-CLEAN-CUT regenerates the same file. This runs after develop is merged in, so nothing hand-merges it; on any later conflict, regenerate rather than hand-merge. `check-pq-kex` must stay green.
```bash
cd /home/govan/tmp/wt/s-supply-first && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-crypto-inventory.sh | tail -1 && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/check-pq-kex.sh | tail -1
```
  Expected: `[PASS] compliance/crypto/cbom.cdx.json matches the tree`, then the `check-pq-kex` pass line. Any other output: read it; the file is generated, so never hand-edit it.

- [ ] **Step 115** [V] ~25m: **THE MERGED TREE TESTS CLEAN**.
```bash
cd /home/govan/tmp/wt/s-supply-first && gofmt -l cmd pkg services internal; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go vet ./... 2>&1 | tail -5; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go test -race ./... 2>&1 | grep -vE '^ok|no test files|^\?' > /var/tmp/s-supply-first/go-final.txt; diff /var/tmp/s-supply-first/go-baseline.txt /var/tmp/s-supply-first/go-final.txt && echo SAME; ./scripts/check-gosec-ratchet.sh | tail -1
```
  `SAME` is the bar, unless develop's own merged changes moved a line: then read the diff. A new failure caused by this plan is fixed here or the lane is reverted.

- [ ] **Step 116** [V] ~10m: **THE FINAL VULNERABLE LIST**.
```bash
cd /home/govan/tmp/wt/s-supply-first && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp govulncheck -show verbose ./... 2>&1 | tail -1; /var/tmp/s-supply-first/rust-audit.sh "$PWD" | tail -1; PATH=/var/tmp/s-supply-first/civenv2/bin:$PATH ./scripts/check-python-audit.sh | tail -1; ./scripts/check-tool-freshness.sh | grep -cE 'STALE'; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp ./scripts/vulnmatch-fixture.sh 2>&1 | tail -1
```
  Bar: `No vulnerabilities found.`, `TOTAL locks=N failing=0`, the audit `[PASS]` with only the two torch ids accepted, a STALE count of 0 or only rows already under "For Stevie", and the vulnmatch line with `0 findings`.

- [ ] **Step 117** [B] ~10m: regenerate the Go and Rust SBOM JSON, seven months stale. The script would overwrite the pip freeze from a venv this plan does not own, so the freeze is kept as is (FND-013 owns that regeneration).
```bash
cd /home/govan/tmp/wt/s-supply-first && cp sbom/python-pip-freeze.txt /var/tmp/s-supply-first/freeze.keep && GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp PYTHON_VENV=/nonexistent ./scripts/generate-sbom.sh | tail -3; cp /var/tmp/s-supply-first/freeze.keep sbom/python-pip-freeze.txt; git diff --stat -- sbom | tail -5; grep -A1 '"golang.org/x/net"' sbom/go-dependencies.json | tail -1
```
  Read `sbom/SBOM.md`'s diff: a Python count that fell to 0 is the script's view of the missing venv, so restore the true count by hand or keep that file out of the commit. The last line must show `v0.62.0`. If the script fails, `git checkout -- sbom` and add a "For Stevie" line; this step does not gate the sprint.

- [ ] **Step 118** [W] ~10m: finish the progress file: lane reports, the final list from Step 116, Phase 6's table reference, unsigned hashes, and any new "For Stevie" rows. CLAUDE.md gets no rewrite here.

- [ ] **Step 119** [C] ~1m: commit.
```bash
cd /home/govan/tmp/wt/s-supply-first && /var/tmp/s-supply-first/ci.sh '[PLAN S-SUPPLY-FIRST] Steps 109-119: lanes merged, SBOM JSON regenerated, final list recorded' docs/battle-plans/progress/S-SUPPLY-FIRST-progress.md sbom/go-dependencies.json sbom/rust-dependencies.json sbom/SBOM.md compliance/crypto/cbom.cdx.json
```

- [ ] **Step 120** [V] ~60m: **THE META-GATE** (clean tree; with the CI tools on PATH, so nothing reads INCONCLUSIVE).
```bash
cd /home/govan/tmp/wt/s-supply-first && git status --short | wc -l && PATH=/var/tmp/s-supply-first/civenv2/bin:$PATH GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp timeout 3500 ./scripts/check-gates-can-fail.sh --quick 2>&1 | tail -4
```
  Bar: `0`, then `PASS: N gate(s) proven to fail when violated`. `--quick` skips every gate marked `slow` (24 today, among them `check-clippy`, `bpf-verifier-check`, `check-hadolint`, `check-trivy-misconfig`, `check-ebpf-insns` and the Well suites), so a pass here says nothing about those; run `--only <gate>` for any this plan touched, and write that limit in the progress file. An INCONCLUSIVE gate is one already failing on a clean tree: `check-image-tags` stays so until S-CLEAN-CUT Step 118 lands. Fix any other, or say which and why in the progress file.

- [ ] **Step 121** [B] ~2m: merge into develop (rule 15). Stevie pushes; Claude never does. Run this only if the develop checkout is on `develop` with a clean index; otherwise leave the branch ready and tell the coordinator.
```bash
cd /home/govan/tmp/unheaded && git merge-base --is-ancestor develop sprint/s-supply-first || { echo 'develop moved: back to Step 112'; exit 1; }; git merge --no-ff -S -q -m '[PLAN S-SUPPLY-FIRST] merge sprint/s-supply-first' sprint/s-supply-first || { git merge --abort; git merge --no-ff --no-gpg-sign -q -m '[PLAN S-SUPPLY-FIRST] merge sprint/s-supply-first' sprint/s-supply-first && echo "- develop merge is unsigned: $(git rev-parse --short HEAD)" >> /home/govan/tmp/wt/s-supply-first/docs/battle-plans/progress/S-SUPPLY-FIRST-progress.md; }; git log -1 --format='%h %G? %s' | cut -c1-90; GOMAXPROCS=4 TMPDIR=/var/tmp/s-supply-first/tmp GOTMPDIR=/var/tmp/s-supply-first/tmp go build -o /dev/null ./cmd/compliance-evidence/ && echo builds
```
  Expected: the merge line, then `builds`. The compliance-evidence user timer runs `go run` from this checkout, so a merge changes what it compiles. `develop moved` means another plan merged during Step 120: go back to Step 112, and re-run the steps after it.

- [ ] **Step 122** [V] ~2m: **PHASE 14 EXIT GATE**.
```bash
cd /home/govan/tmp/unheaded && git branch --contains sprint/s-supply-first | grep -c develop; ls docs/legal/RELICENSE-CANDIDATES-2026-11-04.md docs/battle-plans/progress/S-SUPPLY-FIRST-progress.md cmd/vulnmatch/main.go
```
  Expected: `1`, then the three paths. Tell the coordinator the progress file is ready to fold into `~/tmp/next.md`.

- [ ] **Step 123** [B] ~2m: remove the lanes and the scratch (Opus only: this deletes outside a worktree). The guards stop it unless the sprint is in develop and the directory is really this plan's.
```bash
cd /home/govan/tmp/unheaded && git merge-base --is-ancestor sprint/s-supply-first develop && for l in go rust pyci grimoire ci images licence scanner; do git worktree remove /home/govan/tmp/wt/s-supply-first-$l && git branch -d sprint/s-supply-first-$l; done
d=/var/tmp/s-supply-first; [ ! -L "$d" ] && [ "$(realpath -e "$d")" = "$d" ] && [ -f "$d/ci.sh" ] && rm -r -- "$d"
```
  `git worktree remove` refuses a dirty lane and `git branch -d` refuses an unmerged one: either refusal is a finding, so stop and read it. Do not force.

---

## APPENDIX A: EMERGENCY PROCEDURES

**`go build` in a worktree says the doomgeneric replacement does not exist.** Run Step 4 again. Keep lane worktrees directly under `~/tmp/wt/`: the path in `go.mod` is relative.

**`rl.py` prints FAIL.** Nothing was written: it checks every line first. The tree moved, usually through another lane or a merge. Run `git grep -n '<old text>'` and report it as [STUCK]; do not loosen the match.

**`extract.py` prints `FAIL no block`.** The plan file must be read from the develop checkout's path, and the coordinator may have moved it. Fix the path at the top of `/var/tmp/s-supply-first/extract.py`.

**gpg-agent times out.** `ci.sh` already falls back and logs the hash. Say so in the handoff. `scripts/resign-unpushed.py` re-signs later, after Stevie caches the key; it rewrites history, so it is Opus or attended work, never a Haiku or Sonnet step (rule 13).

**A lane's commit picked up something unexpected.** `git show --stat HEAD`. `ci.sh` stages only what it is given, so the extra file was already staged. Haiku and Sonnet: leave the commit, mark [STUCK], name the file; undoing a commit rewrites history (rule 13). Phase 14 handles it with `git revert` or a fix commit. Never `git add -A`.

**A new advisory appears mid-sprint.** The lists above were measured on 2026-11-04. A new id is outside a Haiku or Sonnet step's bounds: record it and go on. Phase 14's Step 116 is where it is handled.

**A process needs stopping.** Use its recorded pid: `kill <pid>`. Never `pkill -f`: it matches the caller's own shell (rule 14).

**PyPI, the pytorch index or huggingface.co fails during Step 49.** Outside failure: retry once, then [STUCK]. The bump stays uncommitted until the smoke passes: never commit it on faith.

**`/tmp` fills.** It is a 7.2 GiB tmpfs, and Go's build temp and `mktemp -d` land there unless told otherwise. Every Go command here sets `TMPDIR` and `GOTMPDIR` to `/var/tmp/s-supply-first/tmp`. A command that fails with `no space left on device` has lost those: add them. `go env GOCACHE` is on disk. Do not `rm -rf` a worktree to make room.

**Something needs a reversal.** Each ecosystem is one commit on its lane, so `git revert <hash>` on the lane, or leave that lane out of Step 111 (the expected count falls with it).

## APPENDIX B: AGENT TABLE

| Phase | Title | Model | Lane | Steps | Depends on | Time |
|---|---|---|---|---|---|---|
| 0 | Preflight | Sonnet | coordinator | 1-14 | none | 35m |
| 1 | Go | Sonnet | `go` | 15-24 | 0 | 35m |
| 2 | Rust | Haiku | `rust` | 25-29 | 0 | 10m |
| 3 | CI tools | Sonnet | `pyci` | 30-41 | 0 | 25m |
| 4 | Grimoire | Opus | `grimoire` | 42-56 | 0 | 40m |
| 5 | Actions, ADR wording | Haiku | `ci` | 57-62 | 0 | 10m |
| 6 | Images | Opus | `images` | 63-68 | 0 | 45m |
| 7 | Licence candidates | Haiku | `licence` | 69-71 | 0 | 5m |
| 8 | Licence wording | Haiku | `licence` | 72-78 | 7 | 15m |
| 9 | Cargo fields, zhend text | Haiku | `licence` | 79-82 | 8 | 5m |
| 10 | OSV package | Sonnet | `scanner` | 83-88 | 0 | 20m |
| 11 | Inventory package | Sonnet | `scanner` | 89-94 | 10 | 15m |
| 12 | cmd/vulnmatch | Sonnet | `scanner` | 95-101 | 11 | 20m |
| 13 | Scanner docs | Haiku | `scanner` | 102-108 | 12 | 15m |
| 14 | Integrate, clean up | Opus | coordinator | 109-123 | all | 1-2 h |

**Critical path**: 0 → 10 → 11 → 12 → 13 → 14. **Skip Protocol acknowledged**: every agent reads the Stuck Protocol line in the header before Step 1 of its lane.

## APPENDIX C: REFERENCE FILES

Built and tested on west on 2026-11-04 against develop `338f2fe4`: `go test -race` green (12 tests), golangci-lint 2.15.0 `0 issues`, gosec v2.29.0 clean. `extract.py` copies these blocks verbatim. Do not retype them.

### pkg/threatfeed/osv/osv_test.go

<!-- file: pkg/threatfeed/osv/osv_test.go -->
````go
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package osv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		eco, a, b string
		want      int
	}{
		{EcoGo, "v0.61.0", "0.62.0", -1},
		{EcoGo, "0.62.0", "v0.62.0", 0},
		{EcoGo, "1.2.3-rc.1", "1.2.3", -1},
		{EcoGo, "1.2.3-rc.2", "1.2.3-rc.10", -1},
		{EcoGo, "1.2.3-alpha", "1.2.3-1", 1},
		{EcoGo, "1.0.0+incompatible", "1.0.0", 0},
		{EcoGo, "0.58.1-0.20261009181718-2f1f834330b2", "0.58.0", 1},
		{EcoCrates, "0.10.3", "0.8.8", 1},
		{EcoPyPI, "5.20.2", "5.8.1", 1},
		{EcoPyPI, "1.0.dev1", "1.0a1", -1},
		{EcoPyPI, "1.0a1", "1.0b1", -1},
		{EcoPyPI, "1.0rc1", "1.0", -1},
		{EcoPyPI, "1.0", "1.0.post1", -1},
		{EcoPyPI, "1.0a1.dev1", "1.0a1", -1},
		{EcoPyPI, "1!0.5", "2.0", 1},
		{EcoPyPI, "2.14.1+cpu", "2.14.1", 0},
		{EcoPyPI, "5.0.0rc3", "5.0.0", -1},
	}
	for _, c := range cases {
		got, err := Compare(c.eco, c.a, c.b)
		if err != nil {
			t.Fatalf("Compare(%s, %s, %s): %v", c.eco, c.a, c.b, err)
		}
		if sign(got) != c.want {
			t.Errorf("Compare(%s, %s, %s) = %d, want %d", c.eco, c.a, c.b, got, c.want)
		}
	}
	if _, err := Compare("npm", "1", "2"); !errors.Is(err, ErrInvalid) {
		t.Errorf("unsupported ecosystem: err = %v, want ErrInvalid", err)
	}
	if _, err := Compare(EcoPyPI, "not a version", "1"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad version: err = %v, want ErrInvalid", err)
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func TestParseRejects(t *testing.T) {
	bad := map[string]string{
		"not json":      `{`,
		"trailing":      `{"id":"A-1","affected":[{"package":{"name":"x","ecosystem":"Go"}}]} {}`,
		"bad id":        `{"id":"a b","affected":[{"package":{"name":"x","ecosystem":"Go"}}]}`,
		"no affected":   `{"id":"A-1"}`,
		"no package":    `{"id":"A-1","affected":[{"package":{"name":"","ecosystem":"Go"}}]}`,
		"two-field evt": `{"id":"A-1","affected":[{"package":{"name":"x","ecosystem":"Go"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0","fixed":"1"}]}]}]}`,
	}
	for name, in := range bad {
		if _, err := Parse([]byte(in)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := Parse(make([]byte, MaxRecordBytes+1)); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversize: err = %v, want ErrInvalid", err)
	}
}

func TestMatches(t *testing.T) {
	a := Affected{
		Package: Package{Name: "Sentence_Transformers", Ecosystem: EcoPyPI},
		Ranges: []Range{
			{Type: "GIT", Events: []Event{{Introduced: "0"}}},
			{Type: "ECOSYSTEM", Events: []Event{{Introduced: "2.0"}, {Fixed: "5.6.0"}}},
		},
	}
	b := Affected{
		Package: Package{Name: "chromadb", Ecosystem: EcoPyPI},
		Ranges:  []Range{{Type: "ECOSYSTEM", Events: []Event{{LastAffected: "1.5.9"}, {Introduced: "0.4.17"}}}},
	}
	c := Affected{Package: Package{Name: "golang.org/x/net", Ecosystem: EcoGo}, Versions: []string{"0.61.0"}}
	d := Affected{
		Package: Package{Name: "golang.org/x/net", Ecosystem: EcoGo},
		Ranges:  []Range{{Type: "SEMVER", Events: []Event{{Introduced: "0"}, {Fixed: "0.62.0"}}}},
	}
	cases := []struct {
		a         Affected
		eco, n, v string
		want      bool
	}{
		{a, EcoPyPI, "sentence-transformers", "2.7.0", true},
		{a, EcoPyPI, "sentence-transformers", "5.6.0", false},
		{a, EcoPyPI, "sentence-transformers", "1.9", false},
		{a, EcoGo, "sentence-transformers", "2.7.0", false},
		{b, EcoPyPI, "chromadb", "1.5.9", true},
		{b, EcoPyPI, "chromadb", "1.5.10", false},
		{b, EcoPyPI, "chromadb", "0.4.16", false},
		{c, EcoGo, "golang.org/x/net", "v0.61.0", true},
		{c, EcoGo, "golang.org/x/net", "v0.62.0", false},
		{d, EcoGo, "golang.org/x/net", "v0.61.0", true},
		{d, EcoGo, "golang.org/x/net", "v0.62.0", false},
	}
	for i, tc := range cases {
		got, err := tc.a.Matches(tc.eco, tc.n, tc.v)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got != tc.want {
			t.Errorf("case %d (%s %s): got %v, want %v", i, tc.n, tc.v, got, tc.want)
		}
	}
	if got := d.FirstFixed(EcoGo); got != "0.62.0" {
		t.Errorf("FirstFixed = %q", got)
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	rec := `{"id":"A-1","affected":[{"package":{"name":"x","ecosystem":"Go"}}]}`
	if _, err := LoadDir(dir); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty dir: err = %v, want ErrInvalid", err)
	}
	for _, n := range []string{"a.json", "b.json"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(rec), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "duplicate id A-1") {
		t.Errorf("duplicate: err = %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "b.json")); err != nil {
		t.Fatal(err)
	}
	vs, err := LoadDir(dir)
	if err != nil || len(vs) != 1 || vs[0].ID != "A-1" {
		t.Errorf("LoadDir = %v, %v", vs, err)
	}
}
````

### pkg/threatfeed/osv/osv.go

<!-- file: pkg/threatfeed/osv/osv.go -->
````go
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package osv parses Open Source Vulnerability records and decides whether a
// component version falls inside an affected range (DP-721 stage 1a). Records
// are untrusted input, so each one is validated before use. This package reads
// local files only; there is no network code here.
package osv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MaxRecordBytes caps one record.
const MaxRecordBytes = 4 << 20

// maxRecords bounds a directory of records.
const maxRecords = 500000

// Ecosystem names as OSV spells them.
const (
	EcoGo     = "Go"
	EcoCrates = "crates.io"
	EcoPyPI   = "PyPI"
)

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid OSV record")

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Event is one point of a range; exactly one field is set.
type Event struct {
	Introduced   string `json:"introduced,omitempty"`
	Fixed        string `json:"fixed,omitempty"`
	LastAffected string `json:"last_affected,omitempty"`
}

// Range is an ordered set of events.
type Range struct {
	Type   string  `json:"type"`
	Events []Event `json:"events"`
}

// Package names what a record affects.
type Package struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// Affected is one package's affected versions.
type Affected struct {
	Package  Package  `json:"package"`
	Ranges   []Range  `json:"ranges,omitempty"`
	Versions []string `json:"versions,omitempty"`
}

// Vuln is one OSV record.
type Vuln struct {
	ID        string     `json:"id"`
	Aliases   []string   `json:"aliases,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	Withdrawn string     `json:"withdrawn,omitempty"`
	Affected  []Affected `json:"affected"`
}

// Parse decodes and validates one record.
func Parse(data []byte) (*Vuln, error) {
	if len(data) > MaxRecordBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrInvalid, len(data), MaxRecordBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var v Vuln
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after the record", ErrInvalid)
	}
	if !idPattern.MatchString(v.ID) {
		return nil, fmt.Errorf("%w: bad id %q", ErrInvalid, v.ID)
	}
	if len(v.Affected) == 0 {
		return nil, fmt.Errorf("%w: %s has no affected packages", ErrInvalid, v.ID)
	}
	for _, a := range v.Affected {
		if a.Package.Name == "" || a.Package.Ecosystem == "" {
			return nil, fmt.Errorf("%w: %s has an affected entry without a package", ErrInvalid, v.ID)
		}
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				n := 0
				for _, s := range []string{e.Introduced, e.Fixed, e.LastAffected} {
					if s != "" {
						n++
					}
				}
				if n != 1 {
					return nil, fmt.Errorf("%w: %s has an event with %d fields set", ErrInvalid, v.ID, n)
				}
			}
		}
	}
	return &v, nil
}

// LoadDir parses every *.json in dir, sorted by name. A duplicate id is an
// error, and so is an empty directory: a scan with no data must not read clean.
func LoadDir(dir string) ([]Vuln, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: no *.json records in %s", ErrInvalid, dir)
	}
	if len(paths) > maxRecords {
		return nil, fmt.Errorf("%w: %d records exceeds %d", ErrInvalid, len(paths), maxRecords)
	}
	sort.Strings(paths)
	seen := make(map[string]bool, len(paths))
	out := make([]Vuln, 0, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p) // #nosec G304 -- operator-supplied record directory
		if err != nil {
			return nil, err
		}
		v, err := Parse(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		if seen[v.ID] {
			return nil, fmt.Errorf("%w: duplicate id %s", ErrInvalid, v.ID)
		}
		seen[v.ID] = true
		out = append(out, *v)
	}
	return out, nil
}

// NormalizeName folds a package name the way its ecosystem compares names.
func NormalizeName(eco, name string) string {
	if eco != EcoPyPI {
		return name
	}
	return strings.ToLower(regexp.MustCompile(`[-_.]+`).ReplaceAllString(name, "-"))
}

// Matches reports whether eco/name at version is affected by this entry.
func (a Affected) Matches(eco, name, version string) (bool, error) {
	if a.Package.Ecosystem != eco || NormalizeName(eco, a.Package.Name) != NormalizeName(eco, name) {
		return false, nil
	}
	want := strings.TrimPrefix(version, "v")
	for _, v := range a.Versions {
		if strings.TrimPrefix(v, "v") == want {
			return true, nil
		}
	}
	for _, r := range a.Ranges {
		if r.Type != "SEMVER" && r.Type != "ECOSYSTEM" {
			continue
		}
		hit, err := inRange(eco, version, r.Events)
		if err != nil {
			return false, err
		}
		if hit {
			return true, nil
		}
	}
	return false, nil
}

func eventVersion(e Event) string {
	switch {
	case e.Introduced != "":
		return e.Introduced
	case e.Fixed != "":
		return e.Fixed
	}
	return e.LastAffected
}

// inRange walks the events in version order: at or past an introduced event
// the version is affected, at or past a fixed event it is not, and past a
// last_affected event it is not. "0" sorts first.
func inRange(eco, version string, events []Event) (bool, error) {
	sorted := append([]Event(nil), events...)
	var sortErr error
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := eventVersion(sorted[i]), eventVersion(sorted[j])
		if a == "0" || b == "0" {
			return a == "0" && b != "0"
		}
		c, err := Compare(eco, a, b)
		if err != nil {
			sortErr = err
		}
		return c < 0
	})
	if sortErr != nil {
		return false, sortErr
	}
	affected := false
	for _, e := range sorted {
		ev := eventVersion(e)
		if e.Introduced == "0" {
			affected = true
			continue
		}
		c, err := Compare(eco, version, ev)
		if err != nil {
			return false, err
		}
		switch {
		case e.Introduced != "" && c >= 0:
			affected = true
		case e.Fixed != "" && c >= 0:
			affected = false
		case e.LastAffected != "" && c > 0:
			affected = false
		}
	}
	return affected, nil
}

// FirstFixed returns the lowest fixed version in the entry, or "".
func (a Affected) FirstFixed(eco string) string {
	best := ""
	for _, r := range a.Ranges {
		for _, e := range r.Events {
			if e.Fixed == "" {
				continue
			}
			if best == "" {
				best = e.Fixed
				continue
			}
			if c, err := Compare(eco, e.Fixed, best); err == nil && c < 0 {
				best = e.Fixed
			}
		}
	}
	return best
}
````

### pkg/threatfeed/osv/version.go

<!-- file: pkg/threatfeed/osv/version.go -->
````go
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package osv

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Compare orders two versions of eco: negative, zero or positive. Go and
// crates.io follow Semantic Versioning 2.0 (a leading v and +build are
// ignored); PyPI follows PEP 440.
func Compare(eco, a, b string) (int, error) {
	switch eco {
	case EcoGo, EcoCrates:
		return compareSemver(a, b)
	case EcoPyPI:
		return comparePEP440(a, b)
	}
	return 0, fmt.Errorf("%w: unsupported ecosystem %q", ErrInvalid, eco)
}

func splitSemver(v string) (core []int, pre []string, err error) {
	s := strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = strings.Split(s[i+1:], ".")
		s = s[:i]
	}
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, nil, fmt.Errorf("%w: bad semver %q", ErrInvalid, v)
		}
		core = append(core, n)
	}
	return core, pre, nil
}

func compareInts(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func compareSemver(a, b string) (int, error) {
	ca, pa, err := splitSemver(a)
	if err != nil {
		return 0, err
	}
	cb, pb, err := splitSemver(b)
	if err != nil {
		return 0, err
	}
	if c := compareInts(ca, cb); c != 0 {
		return c, nil
	}
	switch {
	case len(pa) == 0 && len(pb) == 0:
		return 0, nil
	case len(pa) == 0:
		return 1, nil
	case len(pb) == 0:
		return -1, nil
	}
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, xerr := strconv.Atoi(pa[i])
		y, yerr := strconv.Atoi(pb[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				return compareInts([]int{x}, []int{y}), nil
			}
		case xerr == nil:
			return -1, nil
		case yerr == nil:
			return 1, nil
		default:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c, nil
			}
		}
	}
	return compareInts([]int{len(pa)}, []int{len(pb)}), nil
}

var pep440 = regexp.MustCompile(`^(?:(\d+)!)?(\d+(?:\.\d+)*)(?:[-_.]?(a|alpha|b|beta|c|rc|pre|preview)[-_.]?(\d*))?(?:[-_.]?(post|rev|r)[-_.]?(\d*))?(?:[-_.]?(dev)[-_.]?(\d*))?$`)

// pep440Key is the sort key: epoch, then release, then these four in order.
type pep440Key struct {
	epoch   int
	release []int
	tail    [4]int // pre kind, pre number, post number, dev number
}

func num(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func parsePEP440(v string) (pep440Key, error) {
	s := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	m := pep440.FindStringSubmatch(s)
	if m == nil {
		return pep440Key{}, fmt.Errorf("%w: bad PEP 440 version %q", ErrInvalid, v)
	}
	k := pep440Key{epoch: num(m[1])}
	for _, p := range strings.Split(m[2], ".") {
		k.release = append(k.release, num(p))
	}
	hasPost, hasDev := m[5] != "", m[7] != ""
	switch m[3] {
	case "a", "alpha":
		k.tail[0], k.tail[1] = -3, num(m[4])
	case "b", "beta":
		k.tail[0], k.tail[1] = -2, num(m[4])
	case "c", "rc", "pre", "preview":
		k.tail[0], k.tail[1] = -1, num(m[4])
	default:
		if hasDev && !hasPost {
			k.tail[0] = -4 // 1.0.dev1 sorts before 1.0a1
		}
	}
	k.tail[2] = -1
	if hasPost {
		k.tail[2] = num(m[6])
	}
	k.tail[3] = math.MaxInt
	if hasDev {
		k.tail[3] = num(m[8])
	}
	return k, nil
}

func comparePEP440(a, b string) (int, error) {
	ka, err := parsePEP440(a)
	if err != nil {
		return 0, err
	}
	kb, err := parsePEP440(b)
	if err != nil {
		return 0, err
	}
	if c := compareInts([]int{ka.epoch}, []int{kb.epoch}); c != 0 {
		return c, nil
	}
	if c := compareInts(ka.release, kb.release); c != 0 {
		return c, nil
	}
	return compareInts(ka.tail[:], kb.tail[:]), nil
}
````

### pkg/threatfeed/inventory/inventory_test.go

<!-- file: pkg/threatfeed/inventory/inventory_test.go -->
````go
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestReadGoMod(t *testing.T) {
	got, err := ReadGoMod(open(t, "fixture.go-mod"), "go.mod")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{"Go", "example.test/alpha", "1.2.3", "go.mod"},
		{"Go", "example.test/beta/v2", "2.0.1", "go.mod"},
		{"Go", "example.test/gamma", "0.9.0", "go.mod"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestReadCargoLock(t *testing.T) {
	got, err := ReadCargoLock(open(t, "fixture.cargo-lock"), "Cargo.lock")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{{"crates.io", "serde", "1.0.230", "Cargo.lock"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestReadRequirements(t *testing.T) {
	got, unpinned, err := ReadRequirements(open(t, "fixture.reqs"), "r.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{"PyPI", "sentence-transformers", "6.2.0", "r.txt"},
		{"PyPI", "ruff", "0.17.3", "r.txt"},
		{"PyPI", "uvicorn", "0.40.0", "r.txt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if !reflect.DeepEqual(unpinned, []string{"r.txt: chromadb>=1.5.9,<2.0.0"}) {
		t.Errorf("unpinned = %q", unpinned)
	}
}

func TestReadSBOMJSON(t *testing.T) {
	got, err := ReadSBOMJSON(open(t, "fixture.sbom-json"), "s.json")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{"Go", "example.test/alpha", "1.2.3", "s.json"},
		{"crates.io", "serde", "1.0.230", "s.json"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if _, err := ReadSBOMJSON(strings.NewReader(`{"packages":[{"version":"1"}]}`), "s.json"); err == nil {
		t.Error("a package with no name was accepted")
	}
}

func TestScanTree(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module m\n\nrequire example.test/a v1.0.0\n")
	write("sub/requirements.txt", "ruff==0.17.3\nchromadb>=1\n")
	write("other/requirements-dev.txt", "ruff==0.17.3\n")
	write("sbom/python-pip-freeze.txt", "tqdm==4.70.1\n")
	write("testdata/go.mod", "module x\n\nrequire example.test/skipped v1.0.0\n")
	write("target/Cargo.lock", "[[package]]\nname = \"skipped\"\nversion = \"1\"\nsource = \"registry+x\"\n")
	comps, unpinned, err := ScanTree(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range comps {
		names = append(names, c.Ecosystem+"/"+c.Name+"@"+c.Version)
	}
	want := []string{"Go/example.test/a@1.0.0", "PyPI/ruff@0.17.3", "PyPI/tqdm@4.70.1"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("components = %q", names)
	}
	if !reflect.DeepEqual(unpinned, []string{"sub/requirements.txt: chromadb>=1"}) {
		t.Errorf("unpinned = %q", unpinned)
	}
}
````

### pkg/threatfeed/inventory/inventory.go

<!-- file: pkg/threatfeed/inventory/inventory.go -->
````go
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package inventory reads the components a tree pins from its own lock data:
// go.mod, Cargo.lock, pinned requirements files and the repo's SBOM JSON
// (DP-721 stage 1a). Only the standard library is used.
package inventory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"unheaded/pkg/threatfeed/osv"
)

// maxLine bounds one manifest line; hash-locked requirements stay far under it.
const maxLine = 1 << 20

// Component is one third-party package at one version.
type Component struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Source    string `json:"source"`
}

func scanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64<<10), maxLine)
	return s
}

// ReadGoMod returns every required module. A module replaced by a local path
// is first-party code and is skipped. Versions lose their leading v.
func ReadGoMod(r io.Reader, source string) ([]Component, error) {
	var req [][2]string
	local := map[string]bool{}
	block := ""
	s := scanner(r)
	for s.Scan() {
		line := s.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if block != "" {
			if f[0] == ")" {
				block = ""
				continue
			}
			f = append([]string{block}, f...)
		} else if len(f) == 2 && f[1] == "(" {
			block = f[0]
			continue
		}
		switch f[0] {
		case "require":
			if len(f) >= 3 {
				req = append(req, [2]string{f[1], f[2]})
			}
		case "replace":
			for i, w := range f {
				if w == "=>" && i+1 < len(f) && (strings.HasPrefix(f[i+1], "./") || strings.HasPrefix(f[i+1], "../")) {
					local[f[1]] = true
				}
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	var out []Component
	for _, m := range req {
		if !local[m[0]] {
			out = append(out, Component{osv.EcoGo, m[0], strings.TrimPrefix(m[1], "v"), source})
		}
	}
	return out, nil
}

// ReadCargoLock returns every package with a registry source; path and
// workspace crates have none and are skipped.
func ReadCargoLock(r io.Reader, source string) ([]Component, error) {
	var out []Component
	name, version, src := "", "", ""
	flush := func() {
		if name != "" && version != "" && strings.HasPrefix(src, "registry+") {
			out = append(out, Component{osv.EcoCrates, name, version, source})
		}
		name, version, src = "", "", ""
	}
	unquote := func(l string) string {
		_, v, _ := strings.Cut(l, "=")
		return strings.Trim(strings.TrimSpace(v), `"`)
	}
	s := scanner(r)
	for s.Scan() {
		l := s.Text()
		switch {
		case l == "[[package]]":
			flush()
		case strings.HasPrefix(l, "name = "):
			name = unquote(l)
		case strings.HasPrefix(l, "version = "):
			version = unquote(l)
		case strings.HasPrefix(l, "source = "):
			src = unquote(l)
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	flush()
	return out, nil
}

var pinned = regexp.MustCompile(`^([A-Za-z0-9._-]+)(?:\[[^\]]*\])?==([^=\s]+)$`)

// ReadRequirements returns the name==version lines and, separately, the
// requirement lines that are not exact pins: what those install changes by
// the day, so the caller reports them rather than claiming they were checked.
func ReadRequirements(r io.Reader, source string) (comps []Component, unpinned []string, err error) {
	s := scanner(r)
	for s.Scan() {
		l := s.Text()
		if i := strings.IndexByte(l, '#'); i >= 0 {
			l = l[:i]
		}
		if i := strings.IndexByte(l, ';'); i >= 0 {
			l = l[:i]
		}
		l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), `\`))
		if l == "" || strings.HasPrefix(l, "-") {
			continue
		}
		if m := pinned.FindStringSubmatch(l); m != nil {
			comps = append(comps, Component{osv.EcoPyPI, osv.NormalizeName(osv.EcoPyPI, m[1]), m[2], source})
			continue
		}
		unpinned = append(unpinned, source+": "+l)
	}
	if err := s.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", source, err)
	}
	return comps, unpinned, nil
}

// ReadSBOMJSON reads sbom/go-dependencies.json or sbom/rust-dependencies.json.
func ReadSBOMJSON(r io.Reader, source string) ([]Component, error) {
	var doc struct {
		Packages []struct {
			Module  string `json:"module"`
			Crate   string `json:"crate"`
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	out := make([]Component, 0, len(doc.Packages))
	for _, p := range doc.Packages {
		switch {
		case p.Module != "" && p.Version != "":
			out = append(out, Component{osv.EcoGo, p.Module, strings.TrimPrefix(p.Version, "v"), source})
		case p.Crate != "" && p.Version != "":
			out = append(out, Component{osv.EcoCrates, p.Crate, p.Version, source})
		default:
			return nil, fmt.Errorf("%s: a package has no module or crate name, or no version", source)
		}
	}
	return out, nil
}

// skipDirs are vendored trees, build output and frozen fixtures.
var skipDirs = map[string]bool{
	".git": true, ".claude": true, "target": true, "node_modules": true, "testdata": true,
	"fixtures": true, "llama.cpp": true, "doom": true, "upstream": true, "vendor": true,
}

// isManifest reports whether a file name is lock data this package reads.
func isManifest(name string) bool {
	switch name {
	case "go.mod", "Cargo.lock", "python-pip-freeze.txt":
		return true
	}
	return strings.Contains(name, "requirements") && strings.HasSuffix(name, ".txt")
}

// Discover lists the lock data under root, relative and sorted.
func Discover(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if isManifest(d.Name()) {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// ScanTree reads everything Discover finds under root. Components are unique
// by ecosystem, name and version, keeping the first source, sorted.
func ScanTree(root string) (comps []Component, unpinned []string, err error) {
	paths, err := Discover(root)
	if err != nil {
		return nil, nil, err
	}
	seen := map[[3]string]bool{}
	for _, rel := range paths {
		got, un, err := readFile(filepath.Join(root, rel), rel)
		if err != nil {
			return nil, nil, err
		}
		unpinned = append(unpinned, un...)
		for _, c := range got {
			k := [3]string{c.Ecosystem, c.Name, c.Version}
			if !seen[k] {
				seen[k] = true
				comps = append(comps, c)
			}
		}
	}
	sort.Slice(comps, func(i, j int) bool {
		a, b := comps[i], comps[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	return comps, unpinned, nil
}

func readFile(path, rel string) ([]Component, []string, error) {
	f, err := os.Open(path) // #nosec G304 -- path comes from Discover under the operator's root
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	switch filepath.Base(rel) {
	case "go.mod":
		c, err := ReadGoMod(f, rel)
		return c, nil, err
	case "Cargo.lock":
		c, err := ReadCargoLock(f, rel)
		return c, nil, err
	}
	return ReadRequirements(f, rel)
}
````

### pkg/threatfeed/inventory/testdata/fixture.go-mod

<!-- file: pkg/threatfeed/inventory/testdata/fixture.go-mod -->
````text
module example.test/app

go 1.27

require (
	example.test/alpha v1.2.3
	example.test/local v0.0.0
	example.test/beta/v2 v2.0.1 // indirect
)

require example.test/gamma v0.9.0

replace example.test/local => ../local

replace (
	example.test/other => ./other
)
````

### pkg/threatfeed/inventory/testdata/fixture.cargo-lock

<!-- file: pkg/threatfeed/inventory/testdata/fixture.cargo-lock -->
````text
# This file is automatically @generated by Cargo.
version = 4

[[package]]
name = "first-party"
version = "0.1.0"
dependencies = [
 "serde",
]

[[package]]
name = "serde"
version = "1.0.230"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "00"

[[package]]
name = "git-dep"
version = "0.3.0"
source = "git+https://example.test/git-dep#abc"
````

### pkg/threatfeed/inventory/testdata/fixture.reqs

<!-- file: pkg/threatfeed/inventory/testdata/fixture.reqs -->
````text
# a comment
Sentence_Transformers==6.2.0
ruff==0.17.3 \
    --hash=sha256:00 \
    --hash=sha256:11
uvicorn[standard]==0.40.0 ; python_version >= "3.11"
chromadb>=1.5.9,<2.0.0
-r other.txt
````

### pkg/threatfeed/inventory/testdata/fixture.sbom-json

<!-- file: pkg/threatfeed/inventory/testdata/fixture.sbom-json -->
````json
{"packages":[{"module":"example.test/alpha","version":"v1.2.3","license":"MIT"},{"crate":"serde","version":"1.0.230","license":"MIT"}]}
````

### pkg/threatfeed/osv/testdata/records/FIXTURE-GO-0001.json

<!-- file: pkg/threatfeed/osv/testdata/records/FIXTURE-GO-0001.json -->
````json
{"id":"FIXTURE-GO-0001","aliases":["CVE-2099-0001"],"summary":"Synthetic record: Go module, fixed release","affected":[{"package":{"name":"example.test/alpha","ecosystem":"Go"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0"},{"fixed":"1.3.0"}]}]}]}
````

### pkg/threatfeed/osv/testdata/records/FIXTURE-PY-0001.json

<!-- file: pkg/threatfeed/osv/testdata/records/FIXTURE-PY-0001.json -->
````json
{"id":"FIXTURE-PY-0001","aliases":["CVE-2099-0002"],"summary":"Synthetic record: PyPI package, no fixed release, in the KEV fixture","affected":[{"package":{"name":"chroma-fixture","ecosystem":"PyPI"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0.4.17"},{"last_affected":"1.5.9"}]}]}]}
````

### pkg/threatfeed/osv/testdata/records/FIXTURE-RS-0001.json

<!-- file: pkg/threatfeed/osv/testdata/records/FIXTURE-RS-0001.json -->
````json
{"id":"FIXTURE-RS-0001","summary":"Synthetic record: crate, listed versions only","affected":[{"package":{"name":"serde-fixture","ecosystem":"crates.io"},"versions":["1.0.230"]}]}
````

### pkg/threatfeed/osv/testdata/records/FIXTURE-WD-0001.json

<!-- file: pkg/threatfeed/osv/testdata/records/FIXTURE-WD-0001.json -->
````json
{"id":"FIXTURE-WD-0001","withdrawn":"2026-11-04T00:00:00Z","summary":"Synthetic record: withdrawn, must never match","affected":[{"package":{"name":"example.test/alpha","ecosystem":"Go"},"versions":["1.2.3"]}]}
````

### pkg/threatfeed/osv/testdata/kev.json

<!-- file: pkg/threatfeed/osv/testdata/kev.json -->
````json
{"title":"Synthetic KEV fixture","catalogVersion":"2026.11.04","dateReleased":"2026-11-04T00:00:00Z","count":1,"vulnerabilities":[{"cveID":"CVE-2099-0002","vendorProject":"fixture","product":"chroma-fixture","vulnerabilityName":"Synthetic entry","dateAdded":"2026-11-04"}]}
````

### cmd/vulnmatch/main_test.go

<!-- file: cmd/vulnmatch/main_test.go -->
````go
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	recordsDir = "../../pkg/threatfeed/osv/testdata/records"
	kevFixture = "../../pkg/threatfeed/osv/testdata/kev.json"
)

// tree writes a small tree with one affected component per ecosystem.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":           "module m\n\nrequire (\n\texample.test/alpha v1.2.3\n\texample.test/clean v9.9.9\n)\n",
		"requirements.txt": "chroma-fixture==1.5.9\nranged>=1\n",
		"Cargo.lock":       "[[package]]\nname = \"serde-fixture\"\nversion = \"1.0.230\"\nsource = \"registry+x\"\n",
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(root, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRunFindsAndOrders(t *testing.T) {
	var out, errb bytes.Buffer
	cfg := config{root: tree(t), osvDir: recordsDir, kevFile: kevFixture, jsonOut: true}
	code, err := run(cfg, &out, &errb)
	if err != nil || code != 0 {
		t.Fatalf("run = %d, %v", code, err)
	}
	var got []Finding
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d findings: %+v", len(got), got)
	}
	if !got[0].KEV || got[0].ID != "FIXTURE-PY-0001" || got[0].FixedIn != "" {
		t.Errorf("first finding %+v, want the KEV one with no fix", got[0])
	}
	if got[1].ID != "FIXTURE-GO-0001" || got[1].KEV || got[1].FixedIn != "1.3.0" {
		t.Errorf("second finding %+v", got[1])
	}
	if got[2].ID != "FIXTURE-RS-0001" {
		t.Errorf("third finding %+v", got[2])
	}
	log := errb.String()
	if !strings.Contains(log, "not checked, not an exact pin: requirements.txt: ranged>=1") {
		t.Errorf("stderr lacks the unpinned line: %q", log)
	}
	if !strings.Contains(log, "4 components, 4 records, 3 findings (1 in KEV), 1 not an exact pin") {
		t.Errorf("stderr lacks the summary: %q", log)
	}
}

func TestFailOnFindings(t *testing.T) {
	var out, errb bytes.Buffer
	cfg := config{root: tree(t), osvDir: recordsDir, failOnFindings: true}
	code, err := run(cfg, &out, &errb)
	if err != nil || code != 1 {
		t.Fatalf("run = %d, %v, want 1", code, err)
	}
	if !strings.Contains(out.String(), "FIXTURE-GO-0001") || strings.Contains(out.String(), "FIXTURE-WD-0001") {
		t.Errorf("table: %q", out.String())
	}
}

func TestNoDataIsAnError(t *testing.T) {
	var out, errb bytes.Buffer
	if _, err := run(config{root: tree(t)}, &out, &errb); err == nil ||
		!strings.Contains(err.Error(), "--osv-dir is required") {
		t.Errorf("err = %v", err)
	}
}
````

### cmd/vulnmatch/main.go

<!-- file: cmd/vulnmatch/main.go -->
````go
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Command vulnmatch matches the components a tree pins against OSV records and
// flags the ones whose CVE is in the CISA KEV catalog (DP-721 stage 1a: lock
// data only, the first step of the native trivy replacement; it replaces no
// trivy use yet, and trivy stays pinned until each stage does). It reads local
// files only and has no network code: OSV is an outside data source that waits
// on the third-party sitting.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/threatfeed/inventory"
	"unheaded/pkg/threatfeed/kev"
	"unheaded/pkg/threatfeed/osv"
)

// config is everything run needs; main fills it from the flags.
type config struct {
	root           string
	osvDir         string
	kevFile        string
	sbomFile       string
	jsonOut        bool
	failOnFindings bool
}

// Finding is one component inside one record's affected range.
type Finding struct {
	KEV       bool     `json:"kev"`
	ID        string   `json:"id"`
	Aliases   []string `json:"aliases,omitempty"`
	Ecosystem string   `json:"ecosystem"`
	Package   string   `json:"package"`
	Version   string   `json:"version"`
	FixedIn   string   `json:"fixed_in,omitempty"`
	Source    string   `json:"source"`
	Summary   string   `json:"summary,omitempty"`
}

// match returns every affected component, KEV entries first, then by id,
// package and version. A withdrawn record never matches.
func match(comps []inventory.Component, vulns []osv.Vuln, kevIDs map[string]bool) ([]Finding, error) {
	var out []Finding
	for _, v := range vulns {
		if v.Withdrawn != "" {
			continue
		}
		inKEV := kevIDs[v.ID]
		for _, a := range v.Aliases {
			inKEV = inKEV || kevIDs[a]
		}
		for _, c := range comps {
			for _, a := range v.Affected {
				hit, err := a.Matches(c.Ecosystem, c.Name, c.Version)
				if err != nil {
					return nil, fmt.Errorf("%s against %s %s: %w", v.ID, c.Name, c.Version, err)
				}
				if hit {
					out = append(out, Finding{inKEV, v.ID, v.Aliases, c.Ecosystem, c.Name, c.Version,
						a.FirstFixed(c.Ecosystem), c.Source, v.Summary})
					break
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.KEV != b.KEV {
			return a.KEV
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		return a.Version < b.Version
	})
	return out, nil
}

// loadKEV returns the catalog's CVE ids; an empty path means no catalog.
func loadKEV(path string) (map[string]bool, error) {
	ids := map[string]bool{}
	if path == "" {
		return ids, nil
	}
	b, err := os.ReadFile(path) // #nosec G304 -- operator-supplied catalog path
	if err != nil {
		return nil, err
	}
	c, err := kev.Parse(b)
	if err != nil {
		return nil, err
	}
	for _, v := range c.Vulnerabilities {
		ids[v.CVEID] = true
	}
	return ids, nil
}

func report(w io.Writer, fs []Finding) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEV\tID\tECOSYSTEM\tPACKAGE\tVERSION\tFIXED\tSOURCE")
	for _, f := range fs {
		k, fixed := "-", f.FixedIn
		if f.KEV {
			k = "KEV"
		}
		if fixed == "" {
			fixed = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", k, f.ID, f.Ecosystem, f.Package, f.Version, fixed, f.Source)
	}
	return tw.Flush()
}

// run scans and reports. It returns the exit code; an error means the scan
// itself failed.
func run(cfg config, stdout, stderr io.Writer) (int, error) {
	if cfg.osvDir == "" {
		return 0, errors.New("--osv-dir is required")
	}
	comps, unpinned, err := inventory.ScanTree(cfg.root)
	if err != nil {
		return 0, err
	}
	if cfg.sbomFile != "" {
		f, err := os.Open(cfg.sbomFile) // #nosec G304 -- operator-supplied SBOM path
		if err != nil {
			return 0, err
		}
		defer func() { _ = f.Close() }()
		more, err := inventory.ReadSBOMJSON(f, cfg.sbomFile)
		if err != nil {
			return 0, err
		}
		comps = append(comps, more...)
	}
	vulns, err := osv.LoadDir(cfg.osvDir)
	if err != nil {
		return 0, err
	}
	kevIDs, err := loadKEV(cfg.kevFile)
	if err != nil {
		return 0, err
	}
	found, err := match(comps, vulns, kevIDs)
	if err != nil {
		return 0, err
	}
	for _, u := range unpinned {
		fmt.Fprintf(stderr, "vulnmatch: not checked, not an exact pin: %s\n", u)
	}
	if cfg.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(found); err != nil {
			return 0, err
		}
	} else if err := report(stdout, found); err != nil {
		return 0, err
	}
	inKEV := 0
	for _, f := range found {
		if f.KEV {
			inKEV++
		}
	}
	fmt.Fprintf(stderr, "vulnmatch: %d components, %d records, %d findings (%d in KEV), %d not an exact pin\n",
		len(comps), len(vulns), len(found), inKEV, len(unpinned))
	if cfg.failOnFindings && len(found) > 0 {
		return 1, nil
	}
	return 0, nil
}

func main() {
	var cfg config
	flag.StringVar(&cfg.root, "root", ".", "tree to read lock data from")
	flag.StringVar(&cfg.osvDir, "osv-dir", "", "directory of OSV records, one *.json each")
	flag.StringVar(&cfg.kevFile, "kev-file", "", "CISA KEV catalog, kev-poller's latest.json")
	flag.StringVar(&cfg.sbomFile, "sbom", "", "also read sbom/go-dependencies.json or sbom/rust-dependencies.json")
	flag.BoolVar(&cfg.jsonOut, "json", false, "print findings as JSON")
	flag.BoolVar(&cfg.failOnFindings, "fail-on-findings", false, "exit 1 when anything matched")
	spec := cmdline.Spec{
		Name:    "vulnmatch",
		Purpose: "Match the components a tree pins against OSV records and the CISA KEV catalog.",
		Examples: []string{
			"vulnmatch --root . --osv-dir pkg/threatfeed/osv/testdata/records --kev-file pkg/threatfeed/osv/testdata/kev.json",
			"vulnmatch --osv-dir /var/tmp/osv --kev-file /var/lib/unheaded/threat-feeds/cisa-kev/latest.json --fail-on-findings",
		},
		Details: "Exit codes: 0 scan complete, 1 findings (with --fail-on-findings), 2 usage or scan error.\nReads local files only.",
	}
	cmdline.Parse(spec)

	code, err := run(cfg, os.Stdout, os.Stderr)
	if err != nil {
		cmdline.Fail(spec, "%s", strings.TrimSpace(err.Error()))
	}
	os.Exit(code)
}
````


## APPENDIX D: HELD FOR STEVIE, NOT A STEP

Run only on Stevie's word that ChromaDB may be accepted (For Stevie 1). Pinning the five remaining ranges brings the file under `check-python-audit.sh`, which then needs four accepted lines. That grows a shrink-only list, which rule 6 otherwise forbids, so no agent runs it unattended.

```bash
cd <develop checkout> && cat > /var/tmp/s-supply-first/hold.spec <<'EOF'
tomb/grimoire/rag/requirements.txt ||| torch>=2.14.1,<3.0.0 ||| torch==2.14.1
tomb/grimoire/rag/requirements.txt ||| markdown>=3.5,<4.0 ||| markdown==3.11.2
tomb/grimoire/rag/requirements.txt ||| beautifulsoup4>=4.12.0,<5.0.0 ||| beautifulsoup4==4.15.0
tomb/grimoire/rag/requirements.txt ||| tqdm>=4.66.0,<5.0.0 ||| tqdm==4.70.1
tomb/grimoire/rag/requirements.txt ||| pyyaml>=6.0,<7.0 ||| pyyaml==6.0.3
docs/policy/python-audit-accepted.txt ||| # <package> <advisory id>  # reason; the finding that tracks the fix\n ||| # <package> <advisory id>  # reason; the finding that tracks the fix\nchromadb PYSEC-2026-311    # server endpoint; Grimoire runs the embedded client, no Chroma server. No fixed release (FND-032)\nchromadb PYSEC-2026-3813   # as above (FND-032)\nchromadb PYSEC-2026-3814   # as above (FND-032)\nchromadb PYSEC-2026-3815   # RBAC provider, not used (FND-032)\n
EOF
python3 -I /var/tmp/s-supply-first/rl.py /var/tmp/s-supply-first/hold.spec
```

Then `check-python-audit.sh` reports 4 pinned files and 6 accepted advisories, and FND-032's `verify` passes. Close FND-032 with the reason.

---

*S-SUPPLY-FIRST Battle Plan — Forged 2026-11-04*
*15 Phases. 123 Steps. Most of the list was already done or could not be done by a bump; what remains is measured, and every pin now has a reason behind it.*
*The native scanner starts on its own fixtures, and nothing in it can reach outside until the sitting says it may.*
