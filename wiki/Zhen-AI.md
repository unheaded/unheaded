# Zhen AI

Zhen is Unheaded's AI-assisted development system. It combines retrieval-augmented generation (RAG) with fine-tuned language models to provide codebase-aware assistance for development, documentation, and operations.

## Architecture

```
┌─────────────────────────────────────────┐
│              Zhen Pipeline              │
├─────────────┬───────────┬───────────────┤
│  Question   │   vor     │  llama.cpp    │
│  shaping    │  :20110    │  :8081        │
├─────────────┼───────────┼───────────────┤
│ pkg/vorquery│ ~2.3k md  │  model from   │
│ zhen_rag.py │  topics   │  the selector │
└─────────────┴───────────┴───────────────┘
```

## RAG Pipeline

Since WAVE15 (2026-05) retrieval is `vor` (`vor serve`, :20110), not the old FAISS index: Unheaded's markdown (docs, ADRs, wiki) plus vor's own cheatsheets, each labelled with a source trust.

- **Question shaping** (`raft/scripts/zhen_rag.py` for the web UI; `pkg/vorquery` for zhen-rag, zhen-agent, zhen-agentd, zhen-cli; a test keeps them identical): question words are dropped before the search, and topics whose *name* carries every query term (`Service-Wotan` for "what is wotan") are interleaved with vor's hits. Agent-worktree copies (`.claude/worktrees/`) are skipped.
- **Fetch:** the top 5 unique topics, by exact name (`/api/topics/<name>`; vor 2044c0f fixed mixed-case names resolving to a different topic), in the hit's category (`?category=`, `pkg/vorclient` `TopicFor`). A repository file's category is its directory, and since vor f8eebff (2026-11-22) `?category=` takes one: segments joined by `/`, each not empty, not `.`, without `..`, `\` or NUL, no leading or trailing `/`, at most 256 bytes in all, else 400. So `docs/adr/README` and `wiki/README` are two topics. The top of an export (`.`) is still read by name alone, and a vor from before f8eebff answers a directory with 400, on which `TopicFor` reads the bare name as before.
- **Liveness:** vor answers `GET /health` (since vor 6108b1b, 2026-11-22; before that only `/api/health`, and `/health` was 404). `configs/health-sweep-west.targets` lists it as `vor 20110 http` for the service health sweep; a vor older than 6108b1b fails that probe until the new binary is installed (S-WIKI-VOR Phase 8).- **Run vor with swap off.** A search scans every topic; when west swaps, a paged-out index turns 0.1 s into 20 s. Start it as `systemd-run --user --scope --collect -p MemorySwapMax=0 --unit=unheaded-vor vor serve &`. The web UI waits up to 60 s for vor.
- **Memory recall:** `all-MiniLM-L6-v2` embeddings of past questions, display only.

## Models and training

- **Inference:** llama.cpp `llama-server` with a model chosen from the UI's selector (ADR-060). Keys come from `scripts/switch-model.sh`: `qwen-7b`, `deepseek`, `deepseek-cpu`, `qwen-coder-14b`. A swap goes through a Champion-gated `model_switch`.
- **Model pins:** every GGUF's sha256 is pinned in `scripts/lib/model-pins.sh` (`MODEL_SHA256[<key>]`). A swap, and `switch-model.sh --dry-run <key>`, hashes the file and refuses with exit 4 when it differs or the key has no pin; `--check` stays a fast fit-only answer and hashes nothing. Adding a model = put the file in `/var/zhen/models`, add its `MODEL_FILE`/`MODEL_FLAGS`/`MODEL_NAME` in `switch-model.sh` and its hash in `model-pins.sh`. `--no-verify` is for local experiments only; it is logged (stderr and syslog tag `switch-model`). The memory embedder is pinned the same way (`EMBEDDER_SHA256`, `EMBEDDER_REVISION`): `raft/start-zhen.sh` exports them as `ZHEN_EMBEDDER_SHA256`/`ZHEN_EMBEDDER_REVISION` and the web UI refuses to start on a mismatch.
- **System prompt:** one copy, `pkg/zhenrag` (`SystemPrompt(Chat|Web|Agent)`), used by `cmd/zhen-rag`, `cmd/zhen-cli`, `pkg/agent` and, through the generated `raft/scripts/zhen_prompt_generated.py`, the web UI. The destructive-verb list the prompts name and the Champion's Rule 3 regexp come from the same table (`zhenrag.Verbs`). After editing either, run `go generate ./pkg/zhenrag/...` and `make coding-gate-ci`; `go run ./pkg/zhenrag/gen -dump chat|web|agent` prints a rendered prompt.
- **Runbooks before execute:** a live `runbook_execute` runs only a runbook whose sha256 is in `runbooks/MANIFEST.sha256` (`scripts/runbook-manifest.sh --write` after editing a runbook; `scripts/check-runbooks.sh` fails CI when it is stale), needs that manifest sealed by a person (`scripts/runbook-manifest.sh --seal`, verified with `/etc/unheaded/zhen/runbook-manifest.pub` or `ZHEN_RUNBOOK_MANIFEST_KEY`: absolute, outside every sandbox root, in a directory others cannot write), refuses an unapproved runbook whose executed commands match the destructive-verb pattern, and runs a private copy of the bytes it checked. Dry runs skip all four.
- **Training (back burner):** the Rust LoRA trainer moved to `unheaded-forge` 2026-11-04: training is unworkable on west. Mistral-7B was the original target; on the 14 GB APU it never learned (Wave 10D).
- **Results so far:** WAVE12 Kingdom RAFT LoRA cut held-out eval loss from 21.10 to 6.78 (ADR-050). WAVE13 Phase 2's quality verdict was **RETRAIN**: 0 of 8 prompts better than base (ADR-051). The WAVE14 retrain (≥ 3 epochs) is still pending.

## Components

| Piece | Where |
|---|---|
| Web UI + RAG (Flask) | `raft/zhen_app.py`, on a Unix socket only (`ZHEN_UI_SOCKET`, default `/run/unheaded-zhen-ui/app.sock`). Its two pages (`raft/pages/chat.html` at `/`, `raft/pages/source-view.html` at `/view/source`) are constant files with no inline script, style or handler; scripts and styles are in `raft/static/`. Both are sent with one Content-Security-Policy (`default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`), and every other response with `default-src 'none'`. No page loads anything from another host: fonts are the system's, and no font file is in the repository (bringing the typefaces back means font files with a licence row, which is Stevie's call, and `font-src 'self'` in `_PAGE_CSP`). `raft/tests/test_zhen_page_policy.py` fails on an external font, script or style, on an inline handler, or on a page without the policy |
| Web UI TLS front | `cmd/zhen-ui-front` (port 20103, ADR-138): TLS 1.3 with hybrid ML-KEM key exchange only, no plaintext mode; `/metrics`, `/health`, `/ready` on `127.0.0.1:20112`. A client needs `X25519MLKEM768`: a current browser, curl on OpenSSL 3.5 or later |
| Champion (gated actions: kanban, files, runbooks, model switch) | `pkg/champion/` (ADR-019) |
| Agent daemon | `cmd/zhen-agentd` (actions + kanban in The Well as `app_zhen`). The web UI's chat goes through it (`/api/v1/agent/ask`): with the daemon down the UI answers 503 "the gate is required", never falling back to llama-server. The daemon refuses to start without auth (`AUTH_API_KEYS`, or `-no-auth` on loopback only); a logged-in person's calls carry their token from the issuer, so the daemon needs `AUTH_OIDC_ISSUER_URL` and `unheaded-auth -audiences` must list `zhen-agentd` (`raft/start-zhen.sh -gated` sets the first from `AUTH_ISSUER_URL`). Everything but the page, `/health`, the highlighter and `/auth/*` takes a login; writes take an operator or admin role; a held runbook or model switch is redeemed at the UI's `/api/v1/agent/confirm`. The UI holds no key into the daemon. Every login crosses `zhen-ui-front`'s TLS; the app has no TLS or TCP listener of its own and takes only the client address from the front (`X-Unheaded-Client-Addr`, which the front overwrites), no other forwarding header. A run that has read tool output is held for a person before any change outside a `zhen/*` worktree, and a live runbook needs `runbooks/MANIFEST.sha256` sealed by a person (`scripts/runbook-manifest.sh --seal`). Browser POSTs carry a CSRF token; the UI sets no CORS headers |
| CLI | `cmd/zhen-cli` (ADR-059) |
| Claude API bridge (`-route auto`: local model first, Claude on failure) | `cmd/zhen-claude`, `pkg/claudebridge` (ADR-104) |
| Training capture (opt-in, redacted JSONL) | `pkg/zhencapture` ([format](../docs/architecture/zhen-capture.md)) |
| MCP server for Claude Code | `cmd/zhen-mcp` (every call through the Champion; stdio transport `pkg/mcpserver`, shared with `cmd/lich-mcp`), registered in `.mcp.json` (server `zhen`, launched by `scripts/zhen-mcp.sh`) |
| Forge (training) | `unheaded-forge` repository (ADR-048 backend trait) |

**On west today (2026-10-15):** none of these is running. Zhen is started on demand.

### Using the MCP server from Claude Code

The repo's `.mcp.json` registers the server as `zhen`. Claude Code asks once
whether to trust project servers; after that the tools appear as
`mcp__zhen__<tool>`. It runs read-only: `file_write`, `file_patch` and
`runbook_execute` stay hidden unless the server's environment sets
`ZHEN_MCP_ALLOW_WRITE=1` / `ZHEN_MCP_ALLOW_EXEC=1`, which `.mcp.json` does not.

Every tool runs through the Champion (`pkg/champion`): file and runbook tools
through `Dispatch`, `corpus_search` (vor, `VOR_URL`, default
`http://127.0.0.1:20110`), `service_health` and `local_llm` after
`AcceptToolCall`. So the path allowlist (symlinks resolved, `.env`,
`secrets`, `credentials`, keys denied) and the destructive-verb rule apply to
every call, including a `local_llm` prompt. A flag is the operator's grant for
that one server process: an enabled write carries a `direct-user` reference
naming the flag, and still meets the write cap, `openat2` writes and, for a
live `runbook_execute`, `runbooks/MANIFEST.sha256`. A refused or failed call is
an `isError` result (`ACCESS DENIED: ...` for a gate refusal). The Champion's
action rows and one line per call go to the server's stderr as JSON.

`local_llm` lets Claude ask the local model directly: one chat completion from
llama-server's OpenAI-compatible API (`ZHEN_LOCAL_LLM_URL`, default
`http://127.0.0.1:8081`). It never starts or swaps a model; if nothing is
listening the call fails with a message saying so. Start the model yourself
with `scripts/switch-model.sh <key>` (it runs the memory fit check). Set
`ZHEN_CAPTURE_DIR` in the server's environment to record each call, redacted,
for training ([format](../docs/architecture/zhen-capture.md)).

`scripts/zhen-mcp.sh` runs `bin/zhen-mcp`; build it with
`make build-zhen-mcp` (and again after pulling changes to it). Without the
binary the script exits non-zero naming that target (`/mcp` in Claude Code
shows the server as failed). `scripts/check-zhen-mcp.sh` runs the handshake
Claude Code performs against a fresh build; CI runs it. The Python server
(`raft/zhen_mcp_server.py`) was retired 2026-10-27 (S-CHAMPION-HARDENING
Phase 10).

## Source shard

`zhen-codeindex` keeps one flat index of committed source in `/var/zhen/code-index`: `index-a.json`, `index-b.json` and `CURRENT`, which names the live slot. Go is chunked with `go/parser` and has a call graph; Rust and Python have definitions only. Search is by word; there are no embeddings. Doom, Lich and Grimoire paths are not indexed. `make build-zhen-codeindex install-hooks` turns on refresh: `scripts/git-hooks/post-commit`, also linked as post-merge, runs it in the background when `develop` moves.

`zhen-codeindex --corpus` builds a second index of the same shape from plain files: the Grimoire's threat models, Kingdom docs and MITRE ATT&CK STIX bundles. It needs no git, model or network, so it runs on the air-gapped Tomb VM, where `--search` answers as JSON and the binary links no network code. Only `threat-models/` and `kingdom-docs/` are `local`; everything else, STIX included, is `external`, so it reaches the model and never justifies a write, and past `k` it still reaches the gate with no text. Filler words in a question do not count. `-corpus-index` must name a `corpus-index` directory, the one the file tools are denied. Nothing it does not index is opened, so an NVD feed cannot stop the build. The corpus keeps its own `--out`: neither mode accepts the source shard's directory.

zhen-agentd reads the shard with `-code-index` and queries vor and the shard together, each bounded by `-retrieve-deadline` (2 s). `-corpus-index` adds the Grimoire corpus beside them; unset, it is not asked. When it is first set (Stevie, 2026-11-09), the index holds `threat-models/` and `kingdom-docs/` only: with ATT&CK in it, most questions share a word with STIX text and Rule 2 would hold most writes. ATT&CK joins later, with a cap on external references per call. Each backend gets its share of `k`, the rest go by trust then score, and an untrusted reference past `k` stays in the justification chain with no text, so Rule 2 always sees it. MCP tools `code_definition` and `code_callers` read the same index. Calls are matched by name, with no type resolution.

A hand-started vor needs its port: `cs serve 20110`. The unit gets its corpus from `scripts/vor-export-source.sh` (tracked files only, run as the repo's owner) and reads it once at startup, so a new export or new taught text needs a vor restart. Develop's export refreshes itself: after every commit or merge on develop the post-commit hook (`make install-hooks`) exports develop, and on a host with `unheaded-vor-refresh.path` installed the new export's stamp restarts vor, after a settle delay of `VOR_REFRESH_SETTLE_SEC` (120 s by default, an `Environment=` value of `unheaded-vor-refresh.service`; a host changes it in a drop-in, `deploy/systemd/README.md` "vor refresh"): a burst of commits or merges becomes one restart instead of tripping the unit's limit of 6 starts in 10 minutes, and vor serves a new export up to two minutes later. By hand it is still `scripts/vor-export-source.sh <dest> develop`.

`zhen-cli` shows `[memory hit: 0.06]` above an answer (`--no-memory-recall` hides it); the number is Postgres `ts_rank`, not a similarity. It saves the session to `zhen_conversations` on exit (`--no-persist`), redacted, once migration 045 is applied.

`/teach <title> | <text>` queues a file for `/var/zhen/projects/zhen-taught` and prints a row id. Nothing is written until Stevie runs `zhen-claude confirm -allowed-roots <root> approve <id>` at a terminal: it shows the whole title and text, then asks for a one-time code. That command needs `ZHEN_TAUGHT_DIR`, is the only writer, and runs as him. zhen-agentd has no route to the directory, and no model-driven process can write `zhen-taught`, `code-index`, `corpus-index` or `vor-source`. A secret-shaped or invisible text is refused before it is queued, and taught text stays `external`. Invisible means a character the approver cannot see and a model can read, whatever its Unicode category: controls other than tab and line ends, format characters (zero-width, bidi, tags), private use, everything from U+E0000 up, and since 2026-11-21 Unicode's `Other_Default_Ignorable_Code_Point` (the four Hangul fillers U+115F, U+1160, U+3164 and U+FFA0, the combining grapheme joiner, the Khmer inherent vowels, reserved points) and the blank Braille pattern U+2800 (`teachBadTextRune` in `pkg/champion/teach.go`). The variation selectors U+FE00 to U+FE0F are not refused: ordinary emoji carry them. It needs migration 046 and a vor restart to be served.

Rule 2 fails closed. A retrieval source that does not answer adds an `unavailable` reference, so while vor is down or restarting, reads and answers continue and every mutating agent call is held.

## Scheduler

Zhen runs approved runbooks on a schedule from inside `zhen-agentd`
(`-scheduler`), so every run meets the Champion gate, the runbook manifest and
the audit log, under the daemon's own unit and nologin user. The Python
scheduler (`raft/zhen_scheduler.py`, ADR-028) was retired 2026-10-27
(S-CHAMPION-HARDENING Phase 8).

**Unit.** `deploy/systemd/zhen-agentd.service` runs
`-scheduler -schedules /etc/unheaded/zhen/schedules.yaml
-schedules-state /var/lib/zhen-agentd/schedules-state.json`; `-metrics-addr`
(env `ZHEN_METRICS_ADDR`, west: `[fd00:dead:beef::1]:20111`) serves `/metrics`
alone on the WireGuard address for Prometheus (job `zhen-agentd`); the API
stays on `127.0.0.1:20111`.

**Config.** `configs/zhen-schedules.yaml`, installed as
`/etc/unheaded/zhen/schedules.yaml` (`root:zhen-agentd`, 0640; the daemon never
writes it):

```yaml
version: 1
jobs:
  - name: service-health-sweep          # [a-z0-9-]{1,64}
    runbook: observe/service-health-sweep # category/name under runbooks/
    every: 6h                           # <N>m|<N>h (5m..744h), daily@HH:MM, weekly@sun@HH:MM
    enabled: true                       # default
    catch_up: false                     # default; true: a slot the daemon was down for runs at the next start
```

A job may name only a runbook whose `metadata.status` is `approved`, or that
carries no status (ADR-024 "Authorship and approval"). A draft or stub makes
the file refuse to load, and the status is checked again before every run.
Times are the host's local time; an interval counts from the last run, with no
catch-up burst after downtime. A job with `catch_up: true` is the exception: the
time of its last run is kept in `-schedules-state`, and when the daemon starts
after one of its slots went by (the host was off at 02:00), it runs once at the
first tick, then keeps its cadence. A job that has never run is due at once. Use
it for a job that must happen "daily, whenever the host is on".

Each job's last successful run is kept in `-schedules-state` as well and exported
as `zhen_scheduler_job_last_success_timestamp_seconds{schedule}`, so it survives
a restart; for a job that has never succeeded it is the time the job was first
loaded. An alert on "no success for N days" reads this gauge.

**The daily indicator-of-compromise audit.** `runbooks/security/ioc-audit.yaml`
(job `ioc-audit`, `daily@02:00`, `catch_up: true`; commented out in the shipped
schedule until the runbook is approved, since a draft makes the file refuse to
load). It reads VictoriaMetrics on loopback and nothing else: SSH logins by
host, account, source and method, authentication failures by host and kind, and
host events by kind (`unheaded_host_events_total`, counted by Vector from the
journal), over the last two days. The run fails, and its card lands in Review,
on a login from outside the operator networks (`IOC_OPERATOR_NETS`, by default
every private range), a login as root, or failures over the limits; it also
fails when it could not see (VictoriaMetrics down, the journal shipped in no
hour of the window), so blind never reads as clean. New sources inside the
operator networks, logins not by key and hosts that ship no journal are notes.
Three days without a successful run is the alert `IOCAuditNotSucceeding` (a
ticket in `observability/alerting/policy.yaml`).

Host events and how the audit judges them (Stevie's defaults of 2026-11-12;
each indicator has a switch that makes it a note):

| Kind | What is counted | Judgment | Switch |
|---|---|---|---|
| `account` | useradd, usermod, userdel, groupadd, groupmod, groupdel, gpasswd, passwd, chpasswd, chage, systemd-sysusers: the tool and the account or group it named | indicator | `IOC_ACCOUNT_CHANGE_IS_INDICATOR=0` |
| `cron_edit` | a crontab replaced or deleted: who, and whose | indicator | `IOC_CRON_EDIT_IS_INDICATOR=0` |
| `kernel` / `module_untrusted` | an out-of-tree or unsigned module load, with the module's name | indicator, unless the module is in `IOC_KNOWN_MODULES` | `IOC_MODULE_UNTRUSTED_IS_INDICATOR=0` |
| `sudo_command` | commands sudo ran, per host, user and target account | note (a count) | none |
| `kernel` / `oom_kill`, `segfault` | out-of-memory kills and segfaults | note | none |

It reads counters, not log text: which command sudo ran, what a crontab now
holds and which process was killed stay in the journal. An install that creates
an account, your own crontab edit, and a boot that loads an out-of-tree module
(west loads `spl` for ZFS at every boot) each fail the run for the two-day
window unless the switch or `IOC_KNOWN_MODULES` says otherwise. The account
tools and the kernel are counted on fields only root or the kernel can produce.
sudo, crontab, passwd, gpasswd and chage log under the invoking uid and are
counted on the program's name, plus its path when journald has one: a program
that logs and exits at once is often gone before journald reads `/proc`, and
its line arrives without `_EXE`, sometimes without `_COMM` (seen on west,
2026-11-15). Those lines count; a path that is present and wrong is dropped.
The trade is deliberate: a real edit is not missed, and a local account can
forge a crontab, sudo or passwd count with a short-lived process. So a count is
a reason to read that host's journal, not proof (`config/vector/vector.yaml`,
the comment above `journal_host`). The stronger source for crontab, an audit
watch on the cron spool, is parked with S-EVIDENCE-RUNS's `lab-audited`
profile. A host that ships its journal but counted no host event in the window
is named in a note.

**Adding a job.** Edit the file and restart the daemon, or in `zhen-cli` type
`/schedule add <name> <every> [runbook]`, `/schedule remove <name>` or
`/schedule resume`, then `/confirm <token>`. These are the mutating tool
`schedule_update`: they answer to a person whatever the justification, a model
cannot schedule anything, and they persist to `-schedules-state`.

**Stopping.** `/schedule pause`, or `emergency stop` in the web UI, takes effect
at once and persists. Resuming is `/schedule resume` in `zhen-cli`. The web UI
also has `list schedules`; its `schedule`, `unschedule` and `resume schedules`
are held, and the browser cannot redeem the token, so they point at `zhen-cli`.

**Where runs show up.** Each run is one Kanban card (type `scheduled-run`,
owner `zhen-scheduler`; done on success, review on failure, the worker never
takes it) and one `zhen_agent_runs` row with `mode = 'scheduled'` (migration
037; no branch, never reviewed). A runbook that exits non-zero, including a
failed `verification:` section, is a failed run with its output's tail in
`error`. Cards need `-kanban-store=pg` with `WELL_KANBAN_DSN` (the board's
writer, `app_kanban`); run rows need `WELL_APP_ZHEN_DSN`. Without them a run
still happens and is kept in memory (`GET /api/v1/scheduler/list` shows the
last run of each job); `zhen_scheduler_record_failures_total` counts what the
Well refused. Events also go to `audit_events` and Wotan `zhen.agent.runs`
(`-audit-well`, `-audit-wotan`).

**Heartbeat.** Every 60 s tick, paused or not, sets
`zhen_scheduler_heartbeat_timestamp_seconds` and publishes
`zhen.scheduler.heartbeat`. `ZhenSchedulerStalled`
(`modules/zhen/prometheus/rules/zhen.yml`, loaded only while the Zhen module is on) fires when it is absent or stale for
15 minutes; runbook `runbooks/observe/zhen-scheduler-stalled.yaml`.

## Scripts

| Script | What |
|---|---|
| `scripts/write-zhen-app-env.sh --db-host HOST:PORT` | As root: writes `/etc/unheaded/zhen-app.env` for `unheaded-zhen-app.service`, copying `APP_ZHEN_PASSWORD` from `/etc/unheaded/compose.env` without ever printing it or passing it as an argument |
| `scripts/zhen-lan-proof.sh [--dry-run] BASE_URL` | Proves no Zhen route answers a stranger (D-01): every route in `raft/tests/zhen_routes.golden` off the open list must answer 401 or 403 bare, same-origin without a session, and cross-site |
| `scripts/zhen-agent-preflight.sh` | `make zhen-agent-up`'s check before launch: `bin/zhen-agent`, vor at `VOR_URL`, llama-server at `LLAMA_URL`, the vor sources directory |
| `scripts/vor-fuzz.sh` | Quick parser fuzz of vor's `GET /api/search?q=`; every input, status and body head to a CSV |
| `scripts/session-to-runbook.py` | Turns the shell commands in a session log into a draft runbook YAML (ADR-024 Phase 3) |
| `scripts/two-brains/author-runbook.sh CATEGORY/NAME` | The runbook authoring pipeline (S-TWO-BRAINS Step 277): local draft in an agent worktree, the repository's checker decides, Claude reviews and patches, authorship fields written by the script, status `draft` ([[Zhen-Routing]]) |
| `scripts/two-brains/split.py TASKS` | The two-brains eval's fixed tune/hold-out split: `sha256(id) % 2` ([[Zhen-Routing]]) |
| `scripts/run-coding-gate.sh` | The 14-prompt coding-gate eval (`eval/coding-gate/prompts.jsonl`) through `bin/zhen-rag`, greedy, raw outputs to `eval/coding-gate/results-<date>.md` for hand grading against `RUBRIC.md` |
| `scripts/run-coding-gate-via-webui.sh`, `scripts/coding-gate-agent.sh`, `scripts/coding-gate-probe.sh`, `scripts/coding-gate-ci.sh` | The same gate through the web UI's `/api/v1/query`, through `zhen-agent`, with per-run seed and system-prompt settings, and over 5 fixed seeds with a variance band (`eval/coding-gate/CONTRIBUTING.md`) |
| `scripts/probe-auto-grade.py`, `scripts/probe-sanitization-check.sh` | A heuristic first-pass grader for the 2026-05-02 probe runs (suggestions only; it reads fixed paths under one home directory), and a check that every adversarial finding doc under `eval/coding-gate/probe-*/` is marked `sanitized: true` |

## Vision

Zhen is designed to progressively build itself and Unheaded:

- **Self-improvement:** Zhen builds its own pipeline improvements
- **Codebase generation:** Zhen generates and validates Unheaded code
- **Frontier delegation:** Opus handles tasks beyond Zhen's current capability
- **Autonomous loops:** Inspired by Karpathy's autoresearch pattern -- constrained experiment loops

## Integration

- **20 skills** leverage Zhen's RAG pipeline for context-aware responses
- **Session handoffs** are indexed for continuity across development sessions
- **Protocol awareness** is embedded in all skill interactions

---

*Last updated: 2026-10-27*

---

> **Source:** [ADR-017](../docs/adr/ADR-017-zhen-hybrid-inference.md) · [ADR-060](../docs/adr/ADR-060-zhenai-multi-model-selector.md) · [docs/architecture/ZHEN_ARCHITECTURE.md](../docs/architecture/ZHEN_ARCHITECTURE.md)
