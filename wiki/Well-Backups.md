# Well Backups

How [[The Well|The-Well]] is backed up, where every copy lives, and how to restore. Decision record: ADR-016 amendment 2026-10-15 (Accepted): nightly dumps, **RPO 24 h**, two weeks kept, no point-in-time recovery yet.

## Where the copies live

| Copy | Host | Path | Disk | Kept |
|---|---|---|---|---|
| Live data | west | `/var/lib/docker/volumes/unheaded-dev_postgres-data` | sdb (`/var`) | n/a |
| Backup sets | west | `/srv/backups/unheaded-well/<YYYY-MM-DD>/well-<HHMMSS>Z/` | NVMe (`/`) | newest 14 |
| Offsite copies | east | `/var/backups/unheaded-well-west/sets/` | sda | newest 14 |
| Legacy DB archive | west | `~/backups/well/2026-10-15/unheaded-legacy-final.dump` | NVMe | kept |

The sets are kept on a different physical disk from the data, so one dead disk cannot take both. The backup unit sets `WELL_BACKUP_OTHER_DISK=1`, and the backup refuses to run if the two ever share a disk.

`LATEST` (world-readable) in each root names the newest set; the sets themselves are root-only (0700/0600).

## What a set holds

`globals.sql` (roles, **no password hashes**), one `pg_dump -Fc` per database, `cluster.env` (superuser name, server version), `rowcounts.tsv` (exact counts in each dump's snapshot), `SHA256SUMS`. A set appears only when every step passed.

## Schedule

West is not up 24/7. Every timer is `Persistent=`, so a run missed while west was off starts within ~20 min of the next boot.

| Unit (west unless noted) | When | Does |
|---|---|---|
| `unheaded-well-backup.timer` | nightly 03:30 | `db-backup.sh` as `well_backup` |
| `unheaded-well-offsite.service` | after each good backup | verifies the set, SFTPs it to east |
| `unheaded-well-drill.timer` | Sundays 05:00 | restores the newest set into a throwaway container, compares every row count |
| `unheaded-well-offsite-check.timer` (east) | daily 06:00 | verifies every set's SHA256SUMS, newest < 36 h, prunes to 14 |

East's account `well-backup-copy` is nologin, key-only and chrooted, and it can add files but never delete or rename them. A compromised west cannot wipe east. East's check fails while west has been off for more than 36 h, because the offsite copy really is stale.

## Alerts

Each job writes a `.prom` file to `/var/lib/unheaded/textfile` on success only; Huginn forwards it. `monitoring/prometheus/rules/well.yml`: backup late (> 26 h), missing (> 50 h), offsite late (> 26 h), drill late (> 8 days), metrics absent. Each waits 45 min so the boot catch-up does not fire them.

**Today nothing evaluates these rules:** the monitoring stack (VictoriaMetrics, Prometheus/Alertmanager) is not running on west, and Alertmanager's receiver is `null` (finding FND-001). Until it is, check by hand: `journalctl -u unheaded-well-backup` and `curl -s localhost:9110/metrics | grep unheaded_well_`.

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
