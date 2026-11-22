<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-ZHEN-FRONT progress

Plan: `docs/battle-plans/S-ZHEN-FRONT-battle-plan.md` (13 phases, 178 steps).

## Decisions

Answered by Stevie 2026-11-12 (round 2, and Step 141).

| # | Question | Answer |
|---|---|---|
| A-16 | The open highlighter and `session_id` on `/health` | Close `POST /api/v1/highlight` behind the login, drop `session_id` from `/health`, prove it, then open on the LAN |
| A-36 | Units for llama-server and the app | System units, each under its own nologin account |
| A-27 | Where the compliance page reads from | (a) the Well |
| Step 141 | Boot behaviour of the two new units | Installed disabled until Stevie says |

## Baseline (2026-11-15, worktree `sprint/s-zhen-front` off develop `7b8c09bf`)

- Raft suite: 585 passed, 2 skipped.
- Go (`cmd/zhen-agentd`, `cmd/zhen-ui-front`, `pkg/uids`, `cmd/dashboard-backend`, race on): all ok.
- Pause state: `zhen-agentd`, `unheaded-zhen-ui-front` and `unheaded-vor` are active (system units). No user unit `unheaded-llama` or `unheaded-zhen-app`. Listening: `127.0.0.1:20103`, `127.0.0.1:20111`, `[fd00:dead:beef::1]:20111`. Nothing on 8081: llama-server is down, the pause stands.

## Step log

| Date | Steps | Note |
|---|---|---|
| 2026-11-15 | 1-10 | Phase 0. The coordinator made the worktree (Steps 1-3). Baselines and pause state above. |
| 2026-11-15 | 11-27 | Phase 1 (`99417e5d`). Red first: six new tests failed for the new reasons only. `POST /api/v1/highlight` left `_LOGIN_OPEN` and `_SCRIPTED_ROUTES` and joined `_READ_POSTS`; `/health` lost `session_id` and answers `startup_failed` in place of the error text, which is now on `/api/v1/system/state` (`webui.startup_error`). `raft/tests/zhen_routes.golden` lists 40 routes (the night session counted 45; each of the 40 is a real route). The two login-route CSRF tests ask on loopback, because a password is taken over plain http only there (Step 18). Raft suite 591 passed. |
| 2026-11-15 | 28-34 | Phase 1 (`642e1d84`). `scripts/zhen-lan-proof.sh` and its plan test (120 requests over 40 routes). shellcheck clean. `tests/scripts/test_zhen_lan_proof.py` runs the shell test, so the CI job that runs `tests/scripts` runs it (not in the plan). **Phase 1 gate: pass.** |
| 2026-11-15 | 35-44 | Phase 2 (`eee789ed`). Red first (four tests). `askResponse` and the stream's `done` event carry `tokens_input` and `tokens_output`, summed per ask in the request context; `openapi.json` lists both. The page reads `tokens_output`. `go test -race ./cmd/zhen-agentd/...` ok; raft suite 596 passed. |
| 2026-11-15 | 45-52 | Phase 2 (`c0a16c82`). `eval/grounding/` (two prompts, README, `run.sh`). The session is a 0600 curl cookie jar; the script refuses any other mode. `cmd/zhen-eval` tests untouched and ok. **Phase 2 gate: pass.** The live run is Step 160. |
| 2026-11-15 | 53-64 | Phase 3 (`4fa356a3`). `refreshStats()` is the one place the sidebar and the welcome get stats; before login they say "log in to see". A 401 on the chat send puts the question back in the box. A login after the session ended keeps the page; the first login of a page load still reloads it (the page was drawn for a stranger). The model switch keeps its own stats poll. oxlint passes (it reads `.js` files only; the page's inline script was parsed with `node --check`). Raft suite 598 passed. **Phase 3 gate: pass.** The browser check is Step 162. |
| 2026-11-15 | 65-74 | Phase 4 (`dec17d98`, `f0cd1445`). `-gated` runs `bin/release/zhen-agentd` and exits 1 naming `make binaries` when it is missing. PID files in `$XDG_RUNTIME_DIR/unheaded-zhen/`; the stop line names them, or `systemctl stop` for a unit. The script says so and leaves alone `unheaded-llama.service` and `unheaded-zhen-app.service` when they are active. A PID file is trusted only when that PID is still the named program. Step 72 found two notes in `zhen_app.py` and the stop section of `raft/CLAUDE.md`; fixed. shellcheck: no warning (two `SC1091` infos predate this). **Phase 4 gate: pass.** The script was not run: it needs llama-server, and the pause stands. |
| 2026-11-15 | 75-86 | Phase 5 (`506d7109`). Red first (seven tests). `/api/v1/source` answers 404 `no such topic` unless vor's answer carries the asked `name` (live vor checked read-only: `nosuchtopic123` answers `WOTAN_TOPICS`). `zhen_markdown.render(..., highlight=False)` for `external` sheets; the render cache key carries the flag, with a test that a canonical rendering is not served to an external sheet of the same text. Raft suite 606 passed. **Phase 5 gate: pass.** |
| 2026-11-15 | 87-92 | Phase 6 (`8c825ef1`). Red first. `_project_root()` reads `ZHEN_PROJECT_ROOT` (default `~/tmp/unheaded`, expanded once) and feeds `PROJECT_ROOT`, `SKILLS_DIR`, `RUNBOOKS_DIR`, `CHAMPION_ALLOWED`, the vestigial `index_dir`/`corpus_dir` and the `UNHEADED_ROOT` fallback. A root that is not absolute stops the app at import. The tests import the app in a fresh interpreter, because the paths are fixed at import; they use `/srv/zhen-root`, not the plan's `/tmp/x`, to keep bandit's B108 quiet. The `~/.config/vor` string S-RESIDUE owns was not touched. Raft suite 613 passed; ruff and bandit clean. |
| 2026-11-15 | 93-99 | Phase 6 (`a317f95f`). Red first: both subtests failed with "cannot assign requested address". `listenFront` sets `IP_FREEBIND` or `IPV6_FREEBIND` on a non-loopback address and warns once when no interface carries it; a loopback address that cannot be bound still fails the start (own test). Race tests ok; the repo's pinned gosec finds nothing in `cmd/zhen-ui-front` or `cmd/zhen-agentd`, with no suppression added. |
| 2026-11-15 | 100-108 | Phase 6 (`a3bca221`). `zhen-agentd.service`: `StartLimitIntervalSec=0`, `RestartSec=10s`, `After=docker.service wg-quick@wg0.service`. Three unit gates pass (49 units verify, 86 flags, 25 ceilings); `units_test.go` reads only the front's unit and passes; `go vet` clean. **Phase 6 gate: pass.** |
| 2026-11-15 | 109-114 | Phase 7 (`2c6b1eaa`). **`unheaded-zhen-app` is uid 16764, not the plan's 16758.** S-WIKI-VOR took 16758, the last free number of the Kingdom tier, for `unheaded-vor-refresh` the same day; 16764 is the next free number its own note names. Checked free in `pkg/uids`, sysusers and west's and east's passwd. `unheaded-llama` is 16792 as planned, with `m` lines for `render` and `video`. `go test -race ./pkg/uids/...` ok; `systemd-sysusers --dry-run` reads the file. |
| 2026-11-15 | 115-119 | Phase 7. **`[STUCK]`, Stevie's decision** (see "For Stevie"). The lock resolved (65 packages, every version equal to `~/.venv/zhen`'s once 23 transitive versions were held), but a tracked lock file fails two gates that need his recorded approval. Nothing was committed. |
| 2026-11-15 | 120-132 | Phase 7 (`28992cf7`). `unheaded-llama.service`, `unheaded-zhen-app.service`, `deploy/tmpfiles.d/unheaded-zhen-ui.conf`, `zhen-app.env.example`, `scripts/write-zhen-app-env.sh` and its test (the password is in the file and in nothing printed; refusals write nothing). Four unit gates pass: 51 units verify, 36 as their own account; `DeviceAllow=char-drm` verifies as written. Beyond the plan's list: a `StateDirectory` and `HOME` for llama-server, `ReadWritePaths=/run/unheaded-zhen-ui` for the app (it could not create its socket under `ProtectSystem=strict` otherwise), and no `@resources` denial in either (torch and the GPU runtime set thread affinity). Both units ship disabled; nothing was installed or started. `cde58e3c` adds `test_zhen_paths.py` to the raft CI job (a gate fails a raft test no job runs). **Phase 7 gate: Steps 113, 127, 129 pass; Step 117 is stuck with 115-119.** |
| 2026-11-15 | 133-138 | Phase 8 (`a071996f`). `WELL_OPS_PASSWORD` and `WELL_CONFIG_PASSWORD` on `dashboard-backend`, each with `:?`. `check-compose-hardening.sh` and `check-gitleaks.sh` pass; the dashboard's compliance tests pass under race. **Phase 8 gate: pass.** The live check is Step 147. |
| 2026-11-15 | close | At `a071996f`: `go build ./...`; race tests of `cmd/zhen-agentd`, `cmd/zhen-ui-front`, `pkg/uids`, `cmd/dashboard-backend`, `cmd/zhen-eval` ok; raft suite 613 passed, 2 skipped (585 at the start); ruff, bandit (CI's flags, the touched files), gosec (the two touched packages), shellcheck, gen-man, netip, http-timeouts and SPDX all pass. Phases 9-12 are the coordinator's and Stevie's. |
| 2026-11-15 | 139 follow-up | Three Go tests the full local CI found, fixed on the branch. (1) `cmd/unheaded-auth` counted the app's unit as a third `pkg/bff` backend. It is a third login front with its own budgets (`raft/zhen_login.py`, 5 and 5), sized with the two backends' on 2026-11-09: the sums are 9 and 23, under the issuer's 10 and 25. No budget changed; `TestBFF_ShippedUnitsFitTheBackendCount` now adds the app's budgets to the sum and still fails on a unit it cannot account for. (2) `config/logging/sources.yaml` has entries for both new units, excluded until the attended install. (3) `unheaded-wiki-server.service` has `LogsDirectory=unheaded/wiki-server`, mode 0750 (red on develop since the 2026-11-11 wiki unit); the two new units are not auth units and the test passes them by. |
| 2026-11-15 | 139-142 | Phase 9. `scripts/ci-local.sh` on the branch with develop merged in: 26 passed, 3 failed, 2 skipped. Go tests: three failures fixed in `df3ca470` (the issuer budget test now adds the Zhen app's own budgets; two units entered in `sources.yaml`; the wiki unit's `LogsDirectory`), one load flake (`TestE2E_BPFDashboard_FlowCorrelation`, passes alone). gosec: `cmd/wiki-server/nav.go:197`, fixed on develop by the gate-fix merge. govulncheck: GO-2026-6859 (`net/http` in go1.27.3), S-EVIDENCE-RUNS Phase 3's. Step 141 as answered (installed disabled). Merged to develop as `c488afa7`, signed. **Phase 9 exit gate met**, with govulncheck still red on develop. The test list also showed `sources.yaml` and budget tests green after the merge. `scripts/ci-local.sh` rewrites tracked files under `sbom/`; they were restored, not committed. |

## Parked

- **The 15-minute login.** The lifetime is the issuer's (`cmd/unheaded-auth/issuer.go:34`, `tokenLifetime = 15 * time.Minute`) and every page's, not Zhen's: `pkg/bff` sessions end with the token too. The fix is ADR-114 N5 (Well-backed sessions, idle 30 minutes, absolute 12 hours) and the SSO ADR (backlog row D-08). This plan only makes the expiry kinder: the question stays in the box.
- **vor's own answer for an unknown id** (the nearest sheet) and its substring matching stay with backlog row D-11 (S-WIKI-VOR). The app no longer shows the wrong sheet, whatever vor answers.
- **The daemon giving up after five quick failures was a forecast**, never seen: the journal since 2026-11-12 03:35Z shows no failed start. The unit now cannot give up. Step 172 checks the next boot.
- **An attached file is not kept across an expired session.** The page drops the attachment when it sends; only the typed question comes back.

## For Stevie

1. **The runtime lock for the app (Steps 115-119) needs your word.** A hash-locked `raft/runtime/requirements.txt` is how the unit's venv would be built (Step 151). It adds no package the app does not already run: all 65 are in `~/.venv/zhen` at the same versions. But once the file is tracked, two gates fail, and both say the approval is yours:
   - `check-dependency-baseline.sh`: 44 names are not in `docs/policy/dependency-baseline.txt`. They are the transitive packages no manifest has listed before (`werkzeug`, `tokenizers`, `huggingface-hub`, `scikit-learn`, `sympy`, `triton`, 16 `nvidia-*-cu12` wheels and others).
   - `check-sbom-trend.sh`: the Python component count goes from 72 to 115, and the rule is that it only falls.
   The two files are kept outside the repo at `/var/tmp/unheaded-sprint/s-zhen-front-runtime-lock/` (`requirements.in`, `requirements.txt`). Your choices: (a) approve the 44 names and the new count, and the files go in as written; (b) build the unit's venv another way (for example a copy of `~/.venv/zhen`), with no lock in the repo; (c) leave the app on `start-zhen.sh` until the torch question below is settled.
2. **torch 2.10.0 has two advisories**, found by `pip-audit` on that lock: PYSEC-2026-139 (no fixed version listed) and PYSEC-2025-194 (fixed in 2.13.0). This is the torch that runs today in `~/.venv/zhen`. The pip-audit gate does not see it.
3. **torch's build.** The lock is PyPI's Linux build, which pulls 16 NVIDIA CUDA wheels onto an AMD host (what runs today). A CPU-only wheel comes from another index, which is a new source: your decision (plan Step 118).
4. **Phase 10 (install on west, attended).** Step 144 expects uid 16764 for `unheaded-zhen-app`, not 16758. Step 151 needs the lock from item 1. Step 148 installs llama-server under `/opt/unheaded/llama`. The unit allows loopback only: if the Well or the issuer is not on loopback for this host, it needs a drop-in with the address in `IPAddressAllow`.
5. **Phase 11 (the pause lifts).** Your word starts it. Then: start the two units, `scripts/zhen-lan-proof.sh` on the live front (Step 163), `eval/grounding/run.sh` with a session cookie jar you make by logging in (Step 160), the browser check of the login panel and stats (Step 162), and only then the LAN opening.
7. **The installed wiki unit on west needs the same two lines** (`LogsDirectory=unheaded/wiki-server`, `LogsDirectoryMode=0750`): a deploy step for the attended window (copy the unit, `daemon-reload`, restart).
8. **The wiki is a fourth login front the budget sum does not count.** `unheaded-wiki-server` logs people in through `pkg/bff` when `/etc/unheaded/wiki-server.env` names an issuer, as it does on west since 2026-11-11. Its unit keeps the issuer URL in the env file, so no test sees it. With it the sums are 11 and 32, over the issuer's 10 and 25: one client address can lock a username on that address. Resizing means changing your 2026-11-09 numbers (five tries on Zhen), so it is yours.
6. **Enable at boot?** Step 141's ruling stands: both units are installed disabled until you say.

## Stuck

- **Steps 115-119 `[STUCK]`** (the runtime lock): waits on Stevie, item 1 above. Tried: the resolution itself works and matches today's venv. Not tried, on purpose: adding 44 names to the dependency baseline or raising the SBOM count, which only he may do.
- **Step 117 `[BLOCKED]`** by it, and with it the last line of Phase 7's exit gate.
- **Step 151 `[BLOCKED]`** (Phase 10: the venv from the lock).

## Found on the way, not this plan's

- (Fixed 2026-11-15, `d1bbd94c`.) `tests/scripts/test_go_root_ci.py::test_a_change_to_a_root_test_package_starts_the_workflow` failed on develop (`cmd/trace-collector-go` is not in the eBPF workflow's paths). It fails the same way in `~/tmp/unheaded`; it came with the trace collector fix.

### 2026-11-22 23:05Z: Steps 115-119 unstuck by DECISION-PACK-2026-11-18 row 30

Row 30: "Lock what runs today, all 44." The lock resolved on 2026-11-15 (kept at `/var/tmp/unheaded-sprint/s-zhen-front-runtime-lock/`) is now tracked as `raft/runtime/requirements.in` and `requirements.txt` (65 packages, 875 hashes), on develop (no sprint worktree is left). Step 117: empty against `~/.venv/zhen` (every version equal). Gates: `check-dependency-baseline.sh` PASS (492 packages, all in the baseline already), `check-python-audit.sh` PASS (3 pinned files; torch 2.10.0's PYSEC-2025-194 and PYSEC-2026-139 were already accepted under FND-013), `check-sbom-trend.sh` PASS (the committed SBOM is not regenerated here; the next regeneration raises the Python count, which row 30 approves), ruff, SPDX and the crypto inventory pass. torch stays PyPI's build (Step 118's default). Step 151 (Phase 10) is no longer blocked by the lock; Phases 10 and 11 are attended.
