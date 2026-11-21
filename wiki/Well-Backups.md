# Well Backups

How [[The Well|The-Well]] is backed up, where every copy lives, and how to restore. Decision record: ADR-016 amendment 2026-10-15 (Accepted): nightly dumps, **RPO 24 h**, two weeks kept, no point-in-time recovery yet.

## Where the copies live

| Copy | Host | Path | Disk | Kept |
|---|---|---|---|---|
| Live data | west | `/var/lib/docker/volumes/unheaded-dev_postgres-data` | sdb (`/var`) | n/a |
| Backup sets | west | `/srv/backups/unheaded-well/<YYYY-MM-DD>/well-<HHMMSS>Z/` | NVMe (`/`) | newest 14 |
| Offsite copies | east | `/var/backups/unheaded-well-west/sets/` | sda | newest 14 |
| Legacy DB archive | west | `~/backups/well/2026-10-15/unheaded-legacy-final.dump` | NVMe | kept |

The volume `unheaded-well-data` (a Postgres cluster of March 2026 that nothing mounted, 77M) was deleted on west on 2026-11-21 (ADR-147 decision 24). The Well's only data volume is `unheaded-dev_postgres-data`.

The sets are kept on a different physical disk from the data, so one dead disk cannot take both. The backup unit sets `WELL_BACKUP_OTHER_DISK=1`, and the backup refuses to run if the two ever share a disk.

`LATEST` (world-readable) in each root names the newest set; the sets themselves are root-only (0700/0600).

## What a set holds

`globals.sql` (roles, **no password hashes**), one `pg_dump -Fc` per database, `cluster.env` (superuser name, server version), `rowcounts.tsv` (exact counts in each dump's snapshot), `SHA256SUMS`. A set appears only when every step passed.

**Training captures are in it.** Once migration 033 is applied, `unheaded_app.zhen_captures` holds the prompts and answers of both brains (Kingdom data, T0/T1, redacted and re-scanned at ingest; only `app_zhen` and the owner can read them) and `zhen_capture_labels` their outcomes. Both are in the nightly `unheaded_app` dump and so in the east offsite copy, for 14 sets. A capture removed by `zhen_captures_purge()` (retention: 180 days unless an export references it; quarantined captures at once) is still in the sets taken before the purge until they rotate out, two weeks later. To get one out of the backups sooner, the set has to be deleted. See [[Zhen Training Data|Zhen-Training-Data]].

## Schedule

West is not up 24/7. Every timer is `Persistent=`, so a run missed while west was off starts within ~20 min of the next boot.

| Unit (west unless noted) | When | Does |
|---|---|---|
| `unheaded-well-backup.timer` | nightly 03:30 | `db-backup.sh` as `well_backup` |
| `unheaded-well-offsite.service` | after each good backup | verifies the set, SFTPs it to east |
| `unheaded-well-drill.timer` | Sundays 05:00 | restores the newest set into a throwaway container, compares every row count |
| `unheaded-well-offsite-check.timer` (east) | daily 06:00 | verifies every set's SHA256SUMS (a set without one is bad), newest < 36 h, keeps the newest 14 that verify |

East's account `well-backup-copy` is nologin, key-only and chrooted, and it can add files but never delete or rename them. A compromised west cannot wipe east. East's check fails while west has been off for more than 36 h, because the offsite copy really is stale.

## Alerts

Each job writes a `.prom` file to `/var/lib/unheaded/textfile` on success only; Huginn forwards it. `monitoring/prometheus/rules/well.yml`: backup late (> 26 h), missing (> 50 h), offsite late (> 26 h), drill late (> 8 days), metrics absent. Each waits 45 min so the boot catch-up does not fire them.

The monitoring Prometheus on west evaluates these rules (FND-001: stack running since 2026-10-20). They are all `urgency: none`, so none mails anyone; what notifies is the `well-backups` SLO, as a ticket in the daily digest, once alert-mailer leaves dry-run ([[Alerting]]). By hand: `journalctl -u unheaded-well-backup` and `curl -s localhost:9110/metrics | grep unheaded_well_`.

## Commands

```bash
sudo systemctl start unheaded-well-backup.service          # a set now (also copies to east)
sudo systemctl start unheaded-well-drill.service           # prove the newest set restores
journalctl -u unheaded-well-backup -u unheaded-well-offsite -u unheaded-well-drill
sudo cat /srv/backups/unheaded-well/LATEST
ssh govan@east sudo systemctl start unheaded-well-offsite-check.service
```

## Restore

1. Prove the set first: `sudo /usr/local/lib/unheaded/well/restore-drill.sh /srv/backups/unheaded-well/<set>`.
2. Restore into a server bootstrapped with the superuser named in the set's `cluster.env` (globals grant `GRANTED BY` it): globals, then `pg_restore` each dump.
3. Set role passwords again from `.env`: `scripts/well/set-role-passwords.sh` (sets carry none).
4. If west's disk is gone, copy the set back from east's `/var/backups/unheaded-well-west/sets/` first.

Runbooks: `runbooks/deploy/emergency-rollback.yaml`, `runbooks/data/postgresql-migration.yaml`.

---

> **Source:** [THE_WELL_OPERATIONS.md](../docs/architecture/THE_WELL_OPERATIONS.md) · [ADR-016](../docs/adr/ADR-016-postgres-the-well.md) · [db-backup.sh](../scripts/db-backup.sh) · [deploy/systemd/](../deploy/systemd/)

## ClickHouse and VictoriaMetrics

ADR-147 decision 6 (S-SEE-AND-SURVIVE Phases 9 and 13): the two stores that had no copy get the Well's pattern. Code and units are in the repository; **installed on west 2026-11-18** (Phase 13), the three timers enabled.

Measured on west at install (2026-11-18 20:13Z, data on `sdb`, sets on the NVMe):

| Store | Size of the store | Set | Backup | Restore drill |
|---|---|---|---|---|
| ClickHouse | `unheaded_logs` 431 MiB, 6.19 million rows | 452,494,148 bytes, 6,176,230 rows in the counted window | 10 s | passed, 6,176,230 rows, 12 s |
| VictoriaMetrics | 15,208 series at the cutoff | 275,060,550 bytes | 15 s | passed, 15,208 series, 5 s |

On a host whose `install` is uutils coreutils (west: 0.2.2), `install -d -o 16707 -g 16707` refuses the numeric owner because uid 16707 has no passwd entry there: make the directory with `install -d -m 0700` and then `chown 16707:16707` it.

| Store | What | Set root (NVMe) | Set | Kept |
|---|---|---|---|---|
| ClickHouse | database `unheaded_logs` (audit logs, 455 days) | `/srv/backups/unheaded-clickhouse` (0700, uid 16707) | `ch-<UTC>/` written by ClickHouse's own `BACKUP DATABASE ... TO File()` | newest 14 |
| VictoriaMetrics | the whole storage (SLO window, host series) | `/srv/backups/unheaded-victoria` (0700, root) | `vm-<UTC>/`, a snapshot copied with links followed | newest 14 |

Offsite: SS5 A (with plan 6, east as second home: its chroot account gets two more directories); until then the sets are local only.

**What a set holds.** Beside each set: `<name>.SHA256SUMS` (every file of the set), `<name>.meta.env` (ClickHouse: `NAME`, `DB`, `TABLE`, `FROM`, `CUTOFF`, `ROWS`, `IMAGE`, `BYTES`; VictoriaMetrics: `NAME`, `CUTOFF`, `SERIES`, `SNAPSHOT`, `IMAGE`, `BYTES`) and `LATEST` (0644, the newest set's name). ClickHouse's `ROWS` is `count()` `FINAL` of the logs between `CUTOFF` - 89 days and `CUTOFF` (5 minutes before the backup): inside every TTL, so merges and TTL passes in a restored copy cannot change it. VictoriaMetrics' `SERIES` is `count({__name__!=""})` at `CUTOFF` (two minutes before the snapshot).

**Schedule.** `unheaded-clickhouse-backup.timer` 04:15, `unheaded-victoria-backup.timer` 04:45 (after the Well's 03:30), `unheaded-store-drill.timer` Sundays 05:40 (after the Well's drill at 05:00). All `Persistent=`.

**The drill.** `scripts/backup/store-drill.sh` runs both drills every time and fails if either does. Each checks the set against its checksums ("set damaged"), reads the meta through a strict parser, restores into a throwaway container of the image that made it (ClickHouse: no network, read-only, 2 GiB, scratch `/srv/backups/.drill-clickhouse`; VictoriaMetrics: 1 GiB on `127.0.0.1:26428`, scratch `/srv/backups/.drill-victoria`, no scrape config) and compares the count. A trap removes the container and the scratch.

**Commands.**

```bash
sudo systemctl start unheaded-clickhouse-backup.service    # a set now
sudo systemctl start unheaded-victoria-backup.service
sudo systemctl start unheaded-store-drill.service          # both drills
journalctl -u unheaded-clickhouse-backup -u unheaded-victoria-backup -u unheaded-store-drill -n 40
sudo cat /srv/backups/unheaded-clickhouse/LATEST /srv/backups/unheaded-victoria/LATEST
```

The VictoriaMetrics admin key (volume `unheaded-dev_victoria-auth`) reaches `curl` on stdin, never argv; never print it. A killed run can leave a snapshot behind: S-SEE-AND-SURVIVE Appendix A 5 lists and deletes it.

**Alerts and SLO.** `rules/backups.yml`: StoreBackupLate (over 26 h, for 45m), StoreBackupMissing (over 50 h), StoreDrillLate (over 8 days); the `store-backups` SLO (99%, tickets) is what notifies. Each job writes `store_backup_<store>.prom` or `store_drill_<store>.prom` in `/var/lib/unheaded/textfile` on success only. Runbook: `runbooks/data/store-backup-late.yaml`. Tested end to end by `scripts/backup/test-store-backups.sh` (CI, `security.yml`).

**Other-disk rule.** Each backup refuses ("refusing ... share disk") when its set root is on the same physical disk as the store's data (`CH_BACKUP_OTHER_DISK=1`, `VM_BACKUP_OTHER_DISK=1` in the units). Today the data is on west's `/var` (sdb) and the sets on the NVMe. **After S-WEST-GROUND moves the database volumes to the NVMe, the sets must leave it:** change `CH_BACKUP_DIR` and `VM_BACKUP_DIR` (one `Environment=` line each in the units) and `CLICKHOUSE_BACKUP_DIR` (compose's `.env`, the bind source of ClickHouse's `/backups`), recreate ClickHouse, run both backups and the drill again. The units refuse to run until then.

## Not The Well: the repository (FND-005)

Commits not yet pushed to GitHub exist only on west's disk. `scripts/backup/git-mirror.sh push` copies every ref to a bare repository on east every 30 minutes (user timer `deploy/systemd/user/unheaded-git-mirror.timer`). It never prunes, and the bare repository keeps reflogs forever and refuses deletions, so a branch deleted or rewritten on west (re-signing) stays recoverable on east. `git-mirror.sh check` fails while any ref is behind, and a push fails once the mirror passes `GIT_MIRROR_MAX_MB` (default 2048 MiB), so growth shows up as a failed unit. **Installed 2026-10-22**: bare repository `govan@east:~/backups/unheaded.git` (156 MiB at install), destination in `~/.config/unheaded/git-mirror.env`; install steps in the service file's header.
