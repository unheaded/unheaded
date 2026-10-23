<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-QUANTUM-ARMOR: Phase 8 (The Well) progress

Track: Well (Phase 8, Steps 159-172) of `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`.
Branch base: develop @ `424edf47`. Executed 2026-10-23 by a parallel agent (Erda + Developer + Marshal).

**Skip Protocol acknowledged:** I will skip any step after 3x its estimate or 2 failed debug attempts, mark it [STUCK] with what I tried, commit known-good state first, continue with the next step that does not depend on it, and finish with a STUCK/ESCALATE report ordered by how many steps each item unblocks. [ESCALATE] steps are never attempted; they are reported.

**Boundaries (Marshal):** the live Well (container `unheaded-postgres`, its volume, roles, backups, timers, `/srv/backups`) is not started, stopped, connected to or modified. Throwaway containers, networks and volumes only, all named `qa-pg18-*`. No other track's paths.

## Steps

| Step | Status | Note |
|---|---|---|
| 159 [R] | done | Read `db/postgresql.conf`, `db/pg_hba.conf`, compose `postgres`, `wiki/The-Well.md`, `wiki/Well-Backups.md`, ADR-101, ADR-102, `scripts/well/test-config.sh`, `make-tls.sh`, `restore-drill.sh`, `test-backup-restore.sh`, `db-backup.sh`, meta-gate registry |
| 160 [DECIDE] | done | RECOMMENDATION taken: compose stays `postgres:16.15-alpine`; PG 18 config ships as `db/postgresql.pg18.conf.d/10-pq-groups.conf`, loaded only by the upgrade (it mounts the dir at `/etc/postgresql/conf.d` and appends `include_dir` to `db/postgresql.conf`). Override condition (Stevie already approved PG 18) not met. |
| 161 [W][TEST] | done | `scripts/well/test-pq-groups.sh`. Deviation from the plan text: `scripts/probe-tls-groups.sh` is Phase 6's file and does not exist on this base, so the suite reads the group itself, twice (s_client's report + the ServerHello key_share via an unprivileged relay, which is also how the libpq clients are observed). Adds two checks the runbook depends on: the migration ledger bootstraps on PG 18, and a PG 16 backup set restores on PG 18 (`restore-drill.sh --image`). |
| 162 [S][TEST] | done | 17/17 PASS (1m14s), then 19/19 with the Go client observer (1m27s); container count 0 (results below). Docker ran without sudo on west. Provocation proven in a scratch clone of `4f524a01`: hybrid dropped from the include -> 5 FAIL, rc=1. |
| 163 [D] | not needed | Both images are Alpine 3.24: PG 18.6 links libssl3 3.5.9, PG 16.15 links 3.5.8. So the Well is classical because PG 16's `ssl_ecdh_curve` sets one curve, not because of its OpenSSL. |
| 164 [W] | done | `provoke_well_pq_groups` + REGISTRY row `well/test-pq-groups` (slow). |
| 165 [C] | done | `4f524a01` |
| 166 [W] | done | `runbooks/data/well-pg18-upgrade.yaml`. [DECIDE] (Erda) dump/restore instead of `pg_upgrade --check`: RECOMMENDATION restore the final backup set into a NEW volume (`restore-drill.sh --into-volume`, new option, refuses an existing volume) and point compose at it; Rationale: the official images carry one major each (pg_upgrade needs both: a new, unadmitted image), the Well is small, dump/restore is the path the Sunday drill already proves, and the PG 16 volume is never written, so rollback is lossless until thaw; Override ONLY if the Well grows past what a restore can do inside an acceptable window (the drill's `unheaded_well_drill_restore_seconds` says when). The suite rehearses the whole data step (restore into a volume, PG 18 on it with the deployed config, role passwords, ledger status over hybrid TLS, app role reads its rows) and checks the refusal. Three automatic steps (image pull, throwaway proof, offline manifest check); every step that reads or changes the live Well is `action: manual`. |
| 167 [V] | done | YAML OK, `check-manifest-yaml.sh` OK, `run-runbook.py --dry-run` rc=0 (19 steps, no unset parameters). Meta-gate coverage replica: no unregistered gate (the runbook makes `well/test-pq-groups` discoverable; it is registered). |
| 168 [ESCALATE] | logged | **E1**: live Well PG 16 -> PG 18 with `ssl_groups`. Stevie's call (data-bearing major upgrade, downtime, timing). Not attempted. Everything it needs is prepared: include, proof suite, runbook, the restore-into-volume tool. |
| 169 [AUDIT-UPDATE] | logged here | For Phase 13's register (not this track's file): "Well TLS key exchange is classical P-256 (PG 16 `ssl_ecdh_curve`, one curve); fix = E1 (`runbooks/data/well-pg18-upgrade.yaml`); verify = `scripts/well/test-pq-groups.sh` + `openssl s_client -starttls postgres -groups X25519MLKEM768` on 127.0.0.1:5432 (or `probe-tls-groups.sh` once Phase 6 merges)". Planned id FND-015. |
| 170 [B] | DEFERRED | `[DEFERRED: lead regenerates after Phase 4 merges]`: `docs/policy/pq-kex-baseline.txt` does not exist on this base. When it does, the `docker-compose.yml postgres-pre18` line gets `# accepted until E1 (ADR-105, runbooks/data/well-pg18-upgrade.yaml)` above it. |
| 171 [C] | done | `41335fcd`: runbook + suite rehearsal + drill option + this log (baseline part deferred with 170) |
| 172 [V] | PASS | PHASE 8 EXIT GATE: suite proves PG 16 classical and PG 18 hybrid (24/24); meta-gate lists `well/test-pq-groups`; runbook dry-runs rc=0; E1 logged; `git diff --stat 424edf47 -- db/postgresql.conf db/pg_hba.conf docker-compose.yml` = 0 lines; no `qa-` container, no `qa-pg18` volume or network; container, volume and network lists equal the pre-run snapshot. |

## Images (ADR-102 exact tags, pinned by digest)

| Image | Digest | Why |
|---|---|---|
| `postgres:16.15-alpine` | `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` | The image already on west (the Well's tag). Not re-pulled: the tag was re-pushed 2026-10-22 (Alpine rebuild) and pulling would have moved west's local tag under the live stack. |
| `postgres:18.6-alpine` | `sha256:898c60cb390d1085bc97bdd6a1ae968757421df394b071848b52b1c09c89d6a5` | Newest 18.x (18.6, released 2026-08-13, past the 7-day cooldown). Same family as an already-admitted image; exact tag + digest, used only by the throwaway suite until E1. This digest is an Alpine rebuild pushed 2026-10-22. |

PG 18 images change the data layout: `PGDATA=/var/lib/postgresql/18/docker`, `VOLUME /var/lib/postgresql` (PG 16: `/var/lib/postgresql/data`). The upgrade must mount a NEW volume at `/var/lib/postgresql`; the runbook does.

## Step 162 result (2026-10-23, west, host OpenSSL 3.5.3; Go client added after the first commit, 19/19, 1m27s)

```
[PASS] pg16 server: 16.15 ecdh=prime256v1 min=TLSv1.3
[PASS] pg16 openssl offer hybrid,X25519,P-256: openssl=secp256r1 wire=secp256r1
[PASS] pg16 openssl offer hybrid only (refused): openssl=refused wire=none
[PASS] pg16 libpq 16 (psql, PG16 image): wire=secp256r1 ssl=TLSv1.3/prime256v1
[PASS] pg16 libpq 18 (psql, PG18 image): wire=secp256r1 ssl=TLSv1.3/prime256v1
[PASS] pg16 Go lib/pq (well-migrate status, go1.27.3): wire=secp256r1 rc=0
[PASS] pg18 server: 18.6 groups=X25519MLKEM768:X25519:prime256v1 min=TLSv1.3 checksums=on
[PASS] pg18 openssl offer hybrid,X25519,P-256: openssl=X25519MLKEM768 wire=X25519MLKEM768
[PASS] pg18 openssl offer hybrid only: openssl=X25519MLKEM768 wire=X25519MLKEM768
[PASS] pg18 openssl offer X25519 only (classical fallback): openssl=X25519 wire=X25519
[PASS] pg18 openssl offer P-256 only (classical fallback): openssl=secp256r1 wire=secp256r1
[PASS] pg18 libpq 18 (psql, PG18 image): wire=X25519MLKEM768 ssl=TLSv1.3/X25519MLKEM768:X25519:prime256v1
[PASS] pg18 libpq 16 (psql, PG16 image): wire=X25519MLKEM768 ssl=TLSv1.3/X25519MLKEM768:X25519:prime256v1
[PASS] pg18 Go lib/pq (well-migrate status, go1.27.3): wire=X25519MLKEM768 rc=0
[PASS] pg18 init.sh: unheaded_app ledger has the same 10 rows as pg16
[PASS] pg18 init.sh: unheaded_ops ledger has the same 7 rows as pg16
[PASS] pg18 init.sh: unheaded_config ledger has the same 9 rows as pg16
[PASS] backup set from pg16 (27 rowcount lines)
[PASS] pg16 set restores on pg18: PASS: 4 databases, 26 tables, 86 rows match; restore took 10s
all checks passed
```

Reading: clients need no change. libpq 16 and 18 (both on OpenSSL 3.5) and the Go services' lib/pq (Go 1.27 crypto/tls defaults) already offer X25519MLKEM768 first; only the server's group list decides. After the run: no `qa-` container, and the docker volume, network and container lists equal the pre-run snapshot.

Also changed: `scripts/well/restore-drill.sh` takes `WELL_DRILL_NAME` (default unchanged, `well-drill-<pid>`), so the suite's drill container is `qa-pg18-drill-<pid>` like everything else it starts, and `--into-volume VOLUME` (the upgrade's data step: restores into a new volume and keeps it; refuses an existing volume; deletes the one it made on failure). Without options it behaves as before: `test-backup-restore.sh` (its gate, including the three broken-set refusals) passed 28/28 against the changed script, run from a scratch copy with its container names prefixed `qa-pg18-` so this session's naming rule held. The root-owned copy on west (`/usr/local/lib/unheaded/well/`) is not touched.

## Final suite result (after the rehearsal was added: 24/24, 1m58s)

```
[PASS] restore into new volume: 4 lines, restore took 11s
[PASS] upgraded volume: server: 18.6 checksums=on
[PASS] upgraded volume: well-migrate status (ledger intact): wire=X25519MLKEM768 rc=0
[PASS] upgraded volume: app_kanban reads its rows over TLS: 25
[PASS] --into-volume refuses an existing volume
all checks passed
```
(the other 19 lines as above)

## DoD (Phase 8)

1. Every step executed or logged: 159-169, 171-172 done; 168 ESCALATE (E1); 170 DEFERRED (Phase 4 baseline absent). No STUCK.
2. Every [V] passed; exit gate output above.
3. Zero new failures: `test-backup-restore.sh` 28/28 against the changed drill script; `check-well-lint.sh`, `check-image-tags.sh`, `check-manifest-yaml.sh` pass. (`check-well-lint.sh` notes findings went down against its baseline; not caused by this track's files, left for the lint's owner.)
4. Shell: shellcheck -S warning clean on the suite, the drill and the meta-gate. No Go or Rust changed.
5. Commits after the gate: `4f524a01`, `41335fcd`, plus this log.
6. No new dependency. One new image tag for the throwaway suite (`postgres:18.6-alpine`, exact tag + digest, the already-admitted postgres image); compose, k8s and CI still pin 16.15 until E1.
7. Attack surface: no new listener (throwaway servers on 127.0.0.1 ephemeral ports, gone); no key, password or PSK on disk or in a log (per-run `openssl rand` values in a 0700 temp dir that the trap deletes).
8. Zero user data access: the suite's data is its own fixture rows.

Erda sign-off: the Well's shape, ledger and backups are unchanged; the upgrade path is dump/restore into a new volume with the old one kept as the rollback, and every live step is manual.

## Marshal shift report

- Mission: Phase 8 only. Result: gate PASS, E1 prepared and escalated.
- Steps: 12 done, 1 ESCALATE (168), 1 DEFERRED (170), 0 STUCK.
- Commits: 3 (unsigned, `--no-gpg-sign`; never pushed).
- Plan amendments (logged above): no `probe-tls-groups.sh` (Phase 6's file), so the suite observes groups itself; `pg_upgrade --check` replaced by a dump/restore [DECIDE]; the suite also rehearses the runbook's data step, because the PG 18 image's new data path and initdb on an empty volume would otherwise have made a written-only runbook wrong.
- Scope held: no other track's paths; `scripts/check-gates-can-fail.sh` touched only by appending one REGISTRY row and one provoke function (plan's conflict rule: keep all rows on merge).
