<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: well-restore-from-east-offsite (runbooks/data/well-restore-from-east-offsite.yaml)

**Scenario.** West's backup disk or the Well's data volume is gone, so the
newest good backup set must come back from east's offsite copies. Same-host
restore is `runbooks/data/postgresql-backup-restore.yaml`: link it in notes.
Locations and schedule: `wiki/Well-Backups.md`.

**Owner** erda. **Risk** high. **requires_sudo** true. Duration about 60m.

**Facts (all in the repo):**
- East keeps the newest 14 sets at `/var/backups/unheaded-well-west/sets/<YYYY-MM-DD>/well-<HHMMSS>Z/`,
  with `/var/backups/unheaded-well-west/sets/LATEST` naming the newest. Each set:
  `SHA256SUMS`, `globals.sql` (roles, no password hashes), one `pg_dump -Fc`
  `<db>.dump` per database, `rowcounts.tsv`, `cluster.env` (superuser name,
  server version). Sets are root-only (0700/0600).
- East verifies sets daily (`scripts/well/offsite-check.sh`,
  `unheaded-well-offsite-check.service`); run it now on east:
  `sudo systemctl start unheaded-well-offsite-check.service`.
- The drill `scripts/well/restore-drill.sh "$SET"` (installed on west as
  `/usr/local/lib/unheaded/well/restore-drill.sh`) checks SHA256SUMS, restores
  into a throwaway container bootstrapped with the set's superuser, compares
  every row count with `rowcounts.tsv`; it never touches the Well.
- Role passwords are not in sets: `scripts/well/set-role-passwords.sh`
  sets them from the environment (`.env`).
- `go run ./cmd/well-migrate status` must be clean after restore (the ledger
  table came with the dump).
- Parameters: `SET` (relative set path `<YYYY-MM-DD>/well-<HHMMSS>Z`, no
  default; read from east's LATEST), `STAGE` default `/srv/backups/unheaded-well-restore`.

**Order:**
1. `host: east`: verify checksums on east first:
   `cd /var/backups/unheaded-well-west/sets/$SET && sudo sha256sum -c SHA256SUMS`
   and `cat /var/backups/unheaded-well-west/sets/LATEST`.
2. Manual: copy the set to west (`scp -r` as root from east to `$STAGE/$SET`;
   the offsite account is add-only and cannot be used to read back).
3. `host: west`: re-check `sha256sum -c SHA256SUMS` in `$STAGE/$SET`.
4. `host: west`: drill: `sudo /usr/local/lib/unheaded/well/restore-drill.sh "$STAGE/$SET"`
   (must exit 0) before any real restore.
5. Manual, irreversible: bootstrap a fresh Well with the superuser from
   `cluster.env`, `psql -f globals.sql`, `pg_restore -d <db> <db>.dump` per
   database. **Never restore over a running Well; never `pg_restore --clean`
   into the live Well.**
6. Manual: `scripts/well/set-role-passwords.sh`.
7. Verify (manual, needs the env): `well-migrate status` clean; row counts
   equal to the set's `rowcounts.tsv`.

**Rollback:** the old data volume, if it still exists, is never deleted by
this runbook; stop the new server and start the old one.
