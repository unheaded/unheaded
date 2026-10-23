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
| 162 [S][TEST] | done | 17/17 PASS, 1m14s, container count 0 (results below). Docker ran without sudo on west. |
| 163 [D] | not needed | Both images are Alpine 3.24: PG 18.6 links libssl3 3.5.9, PG 16.15 links 3.5.8. So the Well is classical because PG 16's `ssl_ecdh_curve` sets one curve, not because of its OpenSSL. |
| 164 [W] | done | `provoke_well_pq_groups` + REGISTRY row `well/test-pq-groups` (slow). |

## Images (ADR-102 exact tags, pinned by digest)

| Image | Digest | Why |
|---|---|---|
| `postgres:16.15-alpine` | `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` | The image already on west (the Well's tag). Not re-pulled: the tag was re-pushed 2026-10-22 (Alpine rebuild) and pulling would have moved west's local tag under the live stack. |
| `postgres:18.6-alpine` | `sha256:898c60cb390d1085bc97bdd6a1ae968757421df394b071848b52b1c09c89d6a5` | Newest 18.x (18.6, released 2026-08-13, past the 7-day cooldown). Same family as an already-admitted image; exact tag + digest, used only by the throwaway suite until E1. This digest is an Alpine rebuild pushed 2026-10-22. |

PG 18 images change the data layout: `PGDATA=/var/lib/postgresql/18/docker`, `VOLUME /var/lib/postgresql` (PG 16: `/var/lib/postgresql/data`). The upgrade must mount a NEW volume at `/var/lib/postgresql`; the runbook does.

## Step 162 result (2026-10-23, west, host OpenSSL 3.5.3)

```
[PASS] pg16 server: 16.15 ecdh=prime256v1 min=TLSv1.3
[PASS] pg16 openssl offer hybrid,X25519,P-256: openssl=secp256r1 wire=secp256r1
[PASS] pg16 openssl offer hybrid only (refused): openssl=refused wire=none
[PASS] pg16 libpq 16 (psql, PG16 image): wire=secp256r1 ssl=TLSv1.3/prime256v1
[PASS] pg16 libpq 18 (psql, PG18 image): wire=secp256r1 ssl=TLSv1.3/prime256v1
[PASS] pg18 server: 18.6 groups=X25519MLKEM768:X25519:prime256v1 min=TLSv1.3 checksums=on
[PASS] pg18 openssl offer hybrid,X25519,P-256: openssl=X25519MLKEM768 wire=X25519MLKEM768
[PASS] pg18 openssl offer hybrid only: openssl=X25519MLKEM768 wire=X25519MLKEM768
[PASS] pg18 openssl offer X25519 only (classical fallback): openssl=X25519 wire=X25519
[PASS] pg18 openssl offer P-256 only (classical fallback): openssl=secp256r1 wire=secp256r1
[PASS] pg18 libpq 18 (psql, PG18 image): wire=X25519MLKEM768 ssl=TLSv1.3/X25519MLKEM768:X25519:prime256v1
[PASS] pg18 libpq 16 (psql, PG16 image): wire=X25519MLKEM768 ssl=TLSv1.3/X25519MLKEM768:X25519:prime256v1
[PASS] pg18 init.sh: unheaded_app ledger has the same 10 rows as pg16
[PASS] pg18 init.sh: unheaded_ops ledger has the same 7 rows as pg16
[PASS] pg18 init.sh: unheaded_config ledger has the same 9 rows as pg16
[PASS] backup set from pg16 (27 rowcount lines)
[PASS] pg16 set restores on pg18: PASS: 4 databases, 26 tables, 86 rows match; restore took 10s
all checks passed
```

Reading: clients need no change. libpq 16 and 18 (both on OpenSSL 3.5) already offer X25519MLKEM768 first; only the server's group list decides. After the run: no `qa-` container, and the docker volume, network and container lists equal the pre-run snapshot.

Also changed: `scripts/well/restore-drill.sh` takes `WELL_DRILL_NAME` (default unchanged, `well-drill-<pid>`), so the suite's drill container is `qa-pg18-drill-<pid>` like everything else it starts. The root-owned copy on west (`/usr/local/lib/unheaded/well/`) is not touched; it keeps working as before.
