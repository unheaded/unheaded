<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-ZHEN-POLISH progress

## Log

- DP-302 cap headroom confirmed: one worst case is reserved before send; default cap 5 USD, max_tokens 16000.

- Phase 6 (Steps 53-68) done: redaction matches stay local (runAuto and zhenroute); zhen-claude check go|json|cite shipped. TestCaptureEscalatedTurn changed per Step 57.

- Phase 4 (Steps 29-38) done, commit 702b083b (signed): 9876 and 20105 greps return 0 outside the excluded paths; gofmt clean; go test ok for zhen-mcp, zhen-cli, zhen-rag, zhen-agent; openapi.json parses; pytest 14 passed (url_scheme, scheduler_client).

- Phase 5 (Steps 39-52) done, commits 00e21478 and 48251cb2 (both signed): account unheaded-vor uid 16738; deploy/systemd/unheaded-vor.service new, zhen-agentd.service amended; scripts/vor-export-source.sh (clean, rc=2, rc=2 as written; a second export over its own marker also replaces cleanly); runbooks, start-zhen.sh and the scrape on 20110/20111 with a fresh manifest; log row added. check-systemd-units, check-unit-ceilings, check-unit-binaries, check-log-sources, runbook-manifest --check, pkg/logsources and pkg/uids pass on the committed tree. The gates glob the filesystem, so they covered the new unit. systemd-analyze verify is clean on it; exposure 1.1 (vor), 1.2 (agentd). Not run: promtool is not on west, so rules/tests/zhen_test.yml (instance label port only) was not executed here; the units were not run as their real accounts (no install, S-HOST-TIDY).

## Measurements

## Stuck

- Steps 29-38 (Phase 4, commit 702b083b): Step 37 [D] [STUCK]. Its trigger holds: the redirected man/zhen-agentd.1 header carries the build stamp (`unheaded v0.0.0-...+dirty`, 2026-11-04), so it differs from gen-man output. The prescribed `scripts/gen-man.sh --write` regenerates every man page in the tree, which is outside the named files, so it was not run. Coordinator to decide.

## For Stevie
- **E1 file-backed shard.** ADR-056/057 need pgvector and tree-sitter; the 10-29 rule forbids them here. The shard is lexical and file-backed. A Well-backed or embedded one is a third-party sitting item.
- **E2 migration batch.** DP-605 lists 030-043 for E1; 044, 045 and 046 join it. Nothing breaks before they apply: rows lose their estimate, session saving answers 503, `/teach` cannot queue. Both probes run once at startup, so zhen-agentd needs a restart after the batch.
- **E3 manual vor.** After the merge the default is 20110, so a hand-started vor must be `cs serve 20110` until the unit is live. While vor is down or restarting, every mutating agent call is held; reads and answers continue.
- **E4 runbook manifest.** Editing `west-startup-order` re-hashes `runbooks/MANIFEST.sha256`. It has `requires_sudo: true` and no `status: approved`, so nothing is revoked.
- **E5 one text, two labels.** Repo text is `external` through vor and `local` through the shard, as ADR-057 says. So a mutating call justified by the same sentence passes Rule 2 by one route and not the other. ADR-057 is still Proposed: say if you want the shard `external` too. Every untrusted search hit now reaches the gate, so more writes that touch repo docs are held than before. That is the safe direction, and overnight work may stop on it more often.
- **E6 ADR-120's open questions and reach.** Answered for this build in Phase 8: idle is one read at request start; embeddings are not recorded. A CPU-served model reads near zero under `gpu_sampled`. The reporting view waits for the routing evals. Energy stays out of routing until a defensible Claude figure exists. The installed daemon writes no `llm_requests` rows at all: its unit has no `-audit-well` and its env file no `WELL_OPS_DSN`. `zhen-claude` with `-audit-well` and the overnight runs do write rows, so those carry estimates. Say if the daemon should too; it is one flag and one line.
- **E7 sibling plans.** Nothing outstanding: `cf839d59` carries the handoff into S-HOST-TIDY Step 132 and S-E1-LIVE Steps 244-245. See the HANDOFF section.
- **E8 `zhen_actions.status` (found by the review, pre-existing).** `db/migrations/009_zhen_actions.sql` allows five values at VARCHAR(20), while the gate writes `accepted` and `denied_*`, up to 30 characters, so those `UpdateAction` calls fail on a real Well; the code only logs a warning, and the row stays `planned`. Fix: one migration, `ALTER COLUMN status TYPE VARCHAR(40)`, dropping `zhen_actions_status_check` and adding it back `NOT VALID` with `accepted` and `status LIKE 'denied\_%'`, then `VALIDATE`. Small, but not in this plan's scope: say if you want it folded in as 047, which the series holds for it.
- **E9 `call` bodies stay unredacted.** Only `teach_write` is checked before it is queued. Other rows in `zhen_pending_confirmations` keep their full `call` until 031's seven-day blanking migration exists; nobody has written it. Say when you want it.
- **E10 agentd's own in-root rows.** `confirm approve` runs as you, and `/var/zhen/projects/default` is 0700 zhen-agentd, so a queued write inside it fails on approval. Approval now stays in its row's root, so rows outside it fail too. Decide whether agentd's in-root writes should be approvable at all, or whether a daemon-side executor should run them.
- **Phase 5: `wiki/Systemd-Units.md` has no `unheaded-vor.service` row.** It lists `zhen-agentd.service`, and no step in this plan adds vor's (series rule 7, ops facts in the wiki). `wiki/**` is L-DOC-B's lane, so Phase 5 left it. Say if Phase 18 should carry the row.
