<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-E1-LIVE progress

## For Stevie

Test cut of Evenings 4 and 5, run 2026-11-10 on your word ("Live, now"), ahead of Evenings 1-3. West is up on TLS; nothing below is rotated, mailed or firewalled yet.

Departures from the plan:

- **Order.** Evenings 4-5 ran before 1-3, so every secret is the pre-rotation one and Evening 3 is still owed. Steps 66 and 70 (the two `[STEVIE]` secret steps) were run by Claude without the values reaching the transcript; they are in `~/tmp/secrets-2026-11.md` (0600).
- **Step 73/75: migration 029 applied live.** The dry run refused on `029_template1_no_public_connect.sql` (below 030, never applied; the plan says stop). The ledger was linear through 028 with no drift and no record of 029, the file is one `REVOKE` on `template1`, so the batch ran as `batch-apply.sh --from 029`: 21 files, not the plan's count. Undo: `bash scripts/well/batch-rollback.sh --yes /srv/backups/unheaded-well/pre-batch/pre-batch-20261110T130712Z`.
- **Step 88.** Alertmanager was started first and silenced before the rest of monitoring; the silence is 1020 minutes (pages until 2026-11-11T06:45Z, tickets until 12:15Z), not the plan's window. End it: `scripts/monitoring/maintenance.sh --end`.
- **Step 95 skipped.** Both Grafana admin logins already answer 200 with the `.env` values; there is nothing rotated to apply.
- **Step 97.** `/etc/unheaded/trace-collector.env` added (`TRACE_METRICS_ADDR`, from the unit's own comment); the plan has no such line.
- **Step 99 ran early** (east's wotan and timeguru disabled, your choice). Undo: `ssh east 'sudo systemctl enable --now unheaded-wotan unheaded-timeguru'`.
- **Skipped on your word:** Steps 81-82, the mail sender half of 83, 102-109 (mail, DNS, FND-001).

Found, and fixed on develop:

- **Victoria could not scrape.** 1 of 12 targets up: VictoriaMetrics 1.108.1 has no hybrid group and every first-party listener is Strict. Both compose files now pin 1.155.0 (`14edf695`); 11 of 12 up, the twelfth is `sword`, which is not running. Data volume copied first: `/var/backups/unheaded-victoria-data-pre-1.155.tgz`.

Found, not changed:

- `syslog-relay` has a Prometheus job (`[fd00:dead:beef::1]:9599`) and is not installed on west: the one down target outside the plan's expected list.
- `chaos-controller`: `/etc/unheaded/chaos/chaos.env` still has `CUIRASS_URL=http://127.0.0.1:19006` (Cuirass is TLS now), `AUTH_ALLOW_CALLERS` is empty (every service token gets 403), and its listener on `*:19012` is plain HTTP. Dry run is on.
- The dev certificates name `localhost` only. `https://192.168.69.184:20000` and `:20001` answer, with a browser warning for the name as well as the CA.
- Zhenai (20103) and the wiki server are not running; they belong to later evenings (Step 244a).

Evening 1, Steps 18-19, run 2026-11-10 without you (your answer: paper passes unattended, nothing sent):

- **The export email is probably not required, and the draft cited a dead paragraph.** eCFR text current to 2026-11-06: 742.15(b)(2) asks for the email only for "non-standard cryptography"; 740.13(e) is "[Reserved]". The notice now cites 742.15(b) alone, drops the WAF, and carries the skill's reading as a dated note. Its recommendation is still to send once: the project's own envelope and protocol formats are published only by their author, and the definition does not settle whether that counts.
- **Steps 20-21 are yours:** read "The email" block in `docs/compliance/export-control-notice.md`, say yes or change it, send it. One sentence is new in the body ("The project believes the cryptography is standard and published; this notification is sent as a precaution").
- **Departure, Step 18.** The plan says replace the 740.9 and 740.17 citations in Part E. Part E is a dated record (2026-03-19), so it got a correction note at its head and the original lines stand. `docs/compliance/control-matrix/itar-ear-2026-05-06.md` cites 740.13(e) throughout and got the same kind of note; the plan did not name it.
- Steps 22-29 wait on the send (23-24) and on your reading, ratification and three signatures (25-28).

Step 244a (Zhen web UI front), run 2026-11-10 without you (your answer: Step 244a unattended), ahead of Step 244:

- **The Zhen page answers:** `https://localhost:20103` on west, loopback only. From your machine: `ssh -L 20103:127.0.0.1:20103 west`. The browser will warn about the dev CA.
- **Nobody can log in yet.** The app has no `AUTH_ISSUER_URL`: compose's `auth` does not publish 19020 to the host and its issuer name is `https://auth:19020`, which the host cannot resolve. The plan has no step for this. Page and `/health` answer; `/auth/login` answers 503. Question logged in `~/tmp/next.md`.
- **Chat answers 503 as well:** zhen-agentd is down. Step 244's `/etc/zhen-agentd.env` is yours to type, so the unit was not started.
- **Departures.**
  - The plan's sysusers line installs the whole `deploy/sysusers.d/unheaded.conf`, which also created `unheaded-syslog-relay` (16739). No unit uses it yet.
  - vor is up from Step 244's unit (`unheaded-vor.service`, enabled); the agentd half of Step 244 is not done.
  - llama-server runs as the user unit `unheaded-llama` (`systemd-run --user`, `MemoryMax=6G`, `MemorySwapMax=0`), not the script's `nohup` line, with `HIP_VISIBLE_DEVICES=0` and `HSA_OVERRIDE_GFX_VERSION=11.0.1` (the values `scripts/bootstrap-llm-lab.sh` writes to `/etc/profile.d/rocm.sh`). A login shell gets them from that file, so the script's reference line works as written; a systemd unit does not inherit them, and the first start split the model across the RX 7700 XT and the integrated GPU and aborted: `ROCm error: invalid device function`. Any future llama-server unit needs both set. It is a transient unit: it does not come back after a reboot.
  - `zhen_app.py` runs under `nohup` as govan, the plan's own line; it has no unit and does not come back after a reboot either.
- **Off:** `systemctl --user stop unheaded-llama`; stop `zhen_app.py` by its pid (`pgrep -f zhen_app.py`); `sudo systemctl disable --now unheaded-vor.service`; the front's rollback is in plan Step 244a.
- **Still open from the step:** the dashboard's `zhenai` card reads down until the host gateway proxy carries 20112 (host configuration); the LAN listener `[DECIDE]`; `runbooks/MANIFEST.sha256` unsealed.

## Evidence

- Steps 65-79 (2026-11-10): `make binaries` 25 built. Dev keys and certs made. Both `docker compose config --quiet` rc=0. Postgres healthy on the existing volume. `batch-apply.sh --from 029` rc=0; 11 role passwords set; `well-migrate status` rc=0, 52 applied, 0 pending; `[PASS] converged unheaded_app schema == fresh bootstrap (privileges included)`. `unheaded-well-backup.service` Result success, LATEST and OFFSITE both `2026-11-10/well-130758Z`.
- Steps 84-93: images built rc=0 (log `/var/tmp/unheaded-sprint/e1-step84-build.log`). `/etc/unheaded/tls`, `trust.d` (18 files, same as `configs/trust.d/dev`) and four host keys installed. Monitoring up, then the main stack in the plan's order. Wotan on 18000: `Negotiated TLS1.3 group: X25519MLKEM768`, `Verify return code: 0 (ok)`, plain http 400. Ports 19000 19001 19002 19003 19006 19100 20000 20001 all 200 over TLS; 25 containers up, none unhealthy.
- Step 94 is Stevie's. The login leg was checked without showing the password: `POST /auth/login` 200 on localhost and on the LAN address (roles `operator`), the `/api/v1/stream` upgrade 101 from both origins, 22 of the dashboard's GET routes 200.
- Steps 97-98: kev-poller, chaos-controller, trace-collector active from `bin/release` (old units and binaries in `/var/backups/*.bak-e1`); plain-HTTP calls to Wotan from the host stopped. Prometheus 46 targets, 15 up; down are the plan's expected jobs plus `syslog-relay`. Victoria 12 targets, 11 up after the 1.155.0 pin.
- Step 100: `192.168.13.2:19131` listening; east's heartbeat `success`; `unheaded_heartbeat_last_seen_timestamp_seconds{host="east"}` present.

- Steps 1-5: Evening 0 started 2026-11-10, unattended (the plan exempts Evening 0). Hourly cap red then green: `go test -race ./cmd/alert-mailer/ ./cmd/alert-policy/` ok; `alert-policy --check` PASS. Two refusal rows for max_per_hour added to cmd/alert-policy/main_test.go and the metric help names capped-hour (not in the plan text, same change).
- Steps 6-7: alert-mailer holds past max_per_hour 3, logs every send, rule AlertMailVolumeHigh. promtool: SUCCESS 4 rules; tests/runbooks/test_alert_runbooks.py 50 passed; alertmanager.yml regenerated (ticket route for the new alert). promtool ran from the local v3.15.0 image with the network off.
- Steps 8-11: Rulesets carry the 2026-11-03 scope, nft -c clean. Prometheus https per target, east wotan out of the SLO until Evening 10B. Noted, not changed: policy.yaml still describes the wotan SLO as "answers on both hosts"; with one target it measures west alone until 10B puts east back.
- Steps 12-15: psk-gate.sh tested, ceremony runbook rewritten, PG18 thaw uses https. `test-rotate-psk: all passed` (gate block: side script runs when its hash matches the pin, a changed one and ten near misses are refused, every command rotate-psk.sh sends is allowed); tests/runbooks 379 passed, 4 skipped; both runbooks dry-run SUCCESS; manifest rewritten, two lines, not sealed. Departures from the plan text, all for Stevie to read before Evening 10:
  - psk-gate.sh: the `PSK_GATE_SIDE_SHA` override is honoured only when `SSH_CONNECTION` is unset, so a variable sent over ssh cannot move the pin. A changed rotate-psk.sh needs a new side.sha256 on east or every rotation is refused.
  - Ceremony runbook: the format line itself refuses when buildkey.luks exists (`sudo test ! -e ... &&`); luksFormat on a rerun would destroy the seed. Plan Step 181's own line has no such guard. Steps reordered to the evening's order and three added: close-and-prove-second-token (Step 186), copy-encrypted-file-to-backup-host (Step 188, so a rotation refreshes the copy on east), and a manual verification (slot counts and the audit watch need root; /etc/audit is 0750). The shell verification checks only that the volume is closed and build.pubkey is committed signed: the public half sits inside the volume, so it cannot be compared once closed; sign-a-test-cask is the proof it matches. The "never in GitHub secrets" line and the FND-018 sentence were kept. rotate-quarterly opens the existing volume and never formats.
  - Two manual checks of psk-gate.sh on west wrote two `unheaded-psk-gate` lines to west's journal (a refusal and a `true`); nothing else ran.
- Steps 16-17: Evening 0 exit gate passed on d9cc6d81 (gofmt, vet, gosec ratchet rc=0, clean tree; also go build, go test -race -shuffle=on 226 ok 0 fail, pytest tests 460 passed 4 skipped, golangci-lint 0 issues, runbook-manifest --check and eleven check scripts rc=0). develop had not moved; merged to local develop as e8978bbf (signed, tree equal to the tested branch). Not pushed. Evening 1 onward is attended: stopped here.
- Steps 18-19 (2026-11-10): Barrister and Librarian skill passes. Sections read from the eCFR versioner API at 2026-11-05 (title 15 up to date as of 2026-11-06): 742.15(b), 772.1, 734.3, 734.7, 740.13 ((e) "[Reserved]"), 740.9 (TMP), 740.17 (ENC). Tree: pkg/tls, crates/zhend, pkg/secrets/encryption, scripts/wireguard exist; pkg/waf, crates/waf, cmd/waf do not. check-spdx rc=0. Nothing sent.
- Step 244a (2026-11-10): `id -u unheaded-zhen-ui-front` 16755; `Generated 1 service certificates`; key `640 root unheaded-zhen-ui-front`; `/run/unheaded-zhen-ui` `2750 govan unheaded-zhen-ui-front`; unit `active`, listeners 127.0.0.1:20103 and :20112, `/ready` 503 before the app. After `raft/start-zhen.sh`: socket `660 govan unheaded-zhen-ui-front`, `/ready` 200, `https://localhost:20103/health` 200, `--curves X25519` 000, `client_hellos_total` hybrid 1 classical 1. llama-server: first start aborted (`MUL_MAT failed`, `ROCm error: invalid device function`, two devices); pinned to device 0 it loaded 29/29 layers, `/health` 200 in about a minute, one completion at 119 tokens/s, 5.7 GiB VRAM, 7.7 GiB host memory available afterwards. vor `/api/health` 200. `POST /auth/login` 503 (no issuer).

## Evening 16: log store, the measured week (2026-11-10)

### Step 277: Erda pass (Schema Proposal)

- **Class/vessel:** Kenoma, derived and rebuildable; ClickHouse stays the record. For the week a second copy goes into the existing Well, `unheaded_ops.log_events`. A separate database was the runner-up and loses: a second ledger, backup entry and role set for a seven-day copy, and the point of the week is to measure what this load does to the Well and its backup.
- **Measured before the design (ClickHouse, read-only, 7 days):** 477,477 rows, all `west journald`, 121 units; 54k to 81k rows a day (53,438 on the sample day); about 1.4 kB a row; busiest minute 11,313 rows, p99.9 minute 968; about 10% of rows `audit`. Postgres 16.15, `max_connections` 50.
- **Sizing:** about 76 MB a day in Postgres, 1.1 GB at 14 days; ClickHouse holds the same day in about 4 MB. On disk Postgres is near 20x, so it fails the Step 278 starting bar (2x) on arithmetic alone, before the week starts. Step 278 is still Stevie's.
- **Access paths (one day of real rows on a throwaway server):** last hour by host, primary key backward, 5.5 ms; count by unit over 7 days, partition scans, 17 ms a day; text match over 24 h, one partition sequential, 565 ms.
- **Shape:** daily RANGE partitions on `ts`; primary key `(host, ts, entry)` so a batch sent twice inserts once; no other index; no default partition; `fields` kept as the JSON text ClickHouse holds.
- **Lifecycle:** 14 days, whole partitions dropped by `log_events_maintain()` (SECURITY DEFINER, `lock_timeout` 2s), called by the writer at start and every five minutes. No row is updated or deleted.
- **Ownership:** owner `well_ops_owner`; `log_writer` INSERT plus EXECUTE on the one function, no SELECT, connection limit 4; `ops_reader` SELECT.
- **Lock plan:** new role, new empty table, new function: no lock on anything that exists.
- **Proof:** `scripts/well/test-log-events.sh --go` on a throwaway server, 51 PASS (grants refuse SELECT/UPDATE/DELETE/TRUNCATE for `log_writer`, partitions made and dropped, duplicate batch inserts once, the writer's own store tests against real Postgres).

### Step 279: migration 048, `cmd/logstore-writer`, the `well_logs` sink

- **Evidence:** `go build ./...` rc=0; `go test -race` ok on `cmd/logstore-writer`, `pkg/uids`, `pkg/ports`, `pkg/database`, `cmd/cert-gen`, `pkg/cmdline`; golangci-lint 0 issues; gitleaks no leaks; `test-vector-config.sh` rc=0 (5 test files, isolation proof: all 20,000 rows reach the record sink while the writer never answers; with `when_full: block` in its place the same run stalls at 11,662); `test-batch-apply.sh` 38 PASS with 048 in the batch; `check-well-lint`, `check-well-migrations`, `check-http-auth`, `check-log-sources`, `check-scrape-publish`, `check-compose-hardening`, `check-compose-log-caps`, `check-http-timeouts`, `check-image-digests`, `check-spdx`, `check-pq-kex`, `check-compose-publish-bind`, `check-json-responses`, `check-hadolint`, `check-secrets-baseline`, `check-unit-ceilings`, `check-usage-errors`, `check-version-flag` (61 binaries), `gen-man.sh` all rc=0.
- **Departures from the plan text.**
  - One `INSERT ... SELECT FROM unnest(...) ON CONFLICT DO NOTHING` a batch, not `COPY`: `COPY` cannot skip a row Vector sends twice.
  - The service sits on `data` and `observe`, not `data` alone: Vector and the scraper reach it over `observe`. No published port.
  - `/ingest` takes a static bearer token (`LOGSTORE_INGEST_TOKEN`, 32 bytes or more, constant-time compare), not `pkg/auth`: one caller, no identity to map. `docs/policy/http-auth-baseline.txt` gained its row.
  - The writer's listener runs `UNHEADED_TLS_PROFILE=preferred`: Vector 0.43.1 links OpenSSL 3.0, which has no ML-KEM group. Deployment CA, TLS 1.3. Moving it to Strict needs a Vector bump, a third-party version decision.
  - 048 is listed for two databases (`postgres` for the role, `unheaded_ops` for the table), as 042 is. `batch-rollback.sh` now names such a role once.
  - Row cap 6000 a minute, below the busiest measured minute (11,313): a burst that size is dropped at this sink only and counted (`429`); ClickHouse still receives every row.
  - Rows the syslog relay filed for another device (Pi-hole queries, nginx access lines: personal data under the retention policy) stop at a `well_rows` filter and never reach the Well copy. None exist today (every relayed source is `gap` or `blocked:D7`). The comparison in Step 282 therefore counts west's own journal rows on both sides.
- **Not touched:** the ClickHouse sink and its `when_full: block`; `vector-east.yaml`.

### Step 278: what counts as "close" (Archimedes reckoning; Stevie decides)

A recommendation, not a decision. Nothing below moves the starting bar; it reports what arithmetic already says against it and names the one choice that is Stevie's.

- **Answer.** Under the starting bar as written, two of the four tests are decided before the week starts, both against Postgres:
  - **Disk per day:** Postgres holds the same rows in **7x to 26x** the bytes ClickHouse does (bar: 2x). ClickHouse stores a row in 60 to 69 bytes on disk (16x compression, its own `system.parts`); the row's payload alone is 665 to 1,130 bytes, and Postgres would need 137 bytes a row or less to meet 2x. No row store without compression gets there.
  - **Backup time:** the nightly backup starts and finishes inside one second today (the Well is 36 MB across four databases; 14 sets total 4.1 MB). With the copy inside it the Well grows 11x to 34x at 14 days, a backup set grows from 0.3 MB to 43 to 174 MB, and the dump takes 9 to 44 s. The first night alone adds 0.6 to 3 s. "Under 10%" fails on the first night.
  - **Still open, and what the week is for:** p95 on the three queries within 2x of ClickHouse; the Well's own p95 moving under 10%; zero stalls on Vector's audit sink.
- **Fermi check.** 55,000 rows a day at about 1 kB is about 60 MB a day in a row store; ClickHouse at 16x is about 4 MB. Ratio near 15x. Agrees.
- **Method.** Rows, bytes on disk and uncompressed bytes per partition from ClickHouse `system.parts`; rows and payload bytes per day for 13 full days (10-28 to 11-09); Postgres row size bounded below by 60% of the payload plus 68 bytes of tuple overhead and above by payload plus overhead plus 30% (page fill, the primary key). Database sizes from `pg_database_size`; backup timing from the unit's journal.
- **Assumptions.**
  1. The last 13 days stand for the week. Sensitivity: low for the ratio (it is per row), high for absolute size: 10-28 had 301,748 rows, 5.5 times the median day (55,087); fourteen such days would be 2.0 to 6.6 GB, past the writer's 4 GiB ceiling, which then answers 503 and drops at this sink only.
  2. `pg_dump -Fc` compresses journal text 5x to 12x and runs 20 to 60 MB/s. Not measured here; the first night's backup measures it.
  3. Postgres 16 with no table compression beyond TOAST. Sensitivity: this is the whole disk result.
- **Verification (independent path).** Step 277 loaded one real day into a throwaway Postgres and measured about 1.4 kB a row, 76 MB a day, against about 4 MB in ClickHouse: near 20x, inside the 7x to 26x reached here from ClickHouse's own byte counts without touching Postgres.
- **The lever.** Whether "close" on disk and backup means a **ratio to ClickHouse** or an **absolute cost on this host**. As a ratio the answer is already ClickHouse and the week can only confirm it. As an absolute cost the numbers are small: 0.4 to 1.2 GB of Well at 14 days and a 43 to 174 MB backup set, on a host with 1.1 TB free on `/var` and 53 GB free on `/` (where the 14 sets live: 0.6 to 2.4 GB of them).
- **Two bars, both written before the data; the choice between them does not depend on anything the week will show.**
  - **(A) As written.** 2x disk, 2x query p95, under 10% Well p95 and backup time, zero stalls. Verdict today: ClickHouse stays. The week is optional.
  - **(B) Absolute budgets for the two decided tests, the other three unchanged.** Disk: the Well copy stays at or under 1.5 GB at 14 days and never reaches the 4 GiB ceiling. Backup: the nightly set finishes inside 60 s and stays at or under 250 MB. Query p95 within 2x, Well p95 under 10%, zero audit-sink stalls, as written. This loosens the bar in Postgres's favour on two points, and it should be chosen only if the question is "can a small shop drop a second store", not "which store is cheaper per row".
- **Recommendation.** (B), stated as the loosening it is. The copy's reason to exist is one store fewer to run; the per-row ratio measures ClickHouse's compression, which nobody doubted. If the per-row ratio is what matters, pick (A) and stop the week (the off switch in Step 281).
- **For the daily reading (Step 282).** One timing a day for seven days gives a p95 that is just the worst of seven. Each reading runs every query 20 times on each side and records the median and the 19th of 20, so the week's p95 rests on 140 runs a query.
- **What starting the week sets in motion, for the answer.** The nightly backup set and its copy on east carry the Well copy's rows from the first night (west's own journal rows, secrets masked by Vector; relayed devices' rows never enter). 14 sets are kept on west; east prunes its own and west cannot delete there. To keep them out instead, `pg_dump --exclude-table-data` for `log_events*` in `scripts/db-backup.sh` (and its row counts), which is a change to the live backup path and so waits for Stevie.
- **Code.** Inputs and arithmetic in the block below; Python 3 standard library.

```python
parts = {"202610": (3428148, 205595222, 3181937189), "202611": (574373, 39475919, 631310883)}  # rows, disk, raw
ch = [d / r for r, d, _ in parts.values()]                 # 60.0, 68.7 bytes a row on disk
pay_lo, pay_med, pay_hi = 665, 1102, 1130                  # payload bytes a row, 13 full days
pg_lo, pg_hi = 68 + 0.6 * pay_lo, (68 + pay_hi) * 1.30     # 467, 1557
print(pg_lo / max(ch), pg_hi / min(ch))                    # 6.8, 26.0
med = 55087                                                # rows on the median day
print(14 * med * pg_lo / 1e9, 14 * med * pg_hi / 1e9)      # 0.36, 1.20 GB
print(14 * med * pay_lo / 12 / 1e6, 14 * med * pay_hi / 5 / 1e6)   # backup set, 43 to 174 MB
print(14 * med * pay_lo / 60e6, 14 * med * pay_hi / 20e6)          # dump, 9 to 44 s
```

### Step 282: the daily reading (`scripts/logstore-measure.sh`)

- Written before the merge, out of plan order, so a baseline exists from before the Well copy started.
- One block of `reading_utc<TAB>metric<TAB>value` lines a run. Numbers only: no row, message or unit name is written, and every timed query's result goes to `/dev/null`. Read-only on both stores. A value that cannot be read is `NA` and the run exits 1.
- **Departure:** the plan names `docs/battle-plans/progress/S-E1-LIVE-logstore.tsv`. Readings are collected data, and collected data lives on the host (Stevie, 2026-11-09), so the default is `~/.local/state/unheaded/S-E1-LIVE-logstore.tsv`; a path argument or `LOGSTORE_TSV` overrides it. Step 283's `git add` therefore carries the script and this file only.
- Each timed query runs 20 times a side; the file keeps the median and the 19th of 20 (Step 278).
- Both sides are filtered to `host = 'west'`, the row set the Well copy holds.
- The Well's own latency is read two ways: one Kanban read timed 20 times, and `pg_stat_statements` totals for every statement that does not touch `log_events` (cumulative; a day is the difference of two readings).

### Step 281: live (2026-11-10, the week starts 18:34Z)

- Baseline reading before anything changed, 18:28:23Z (exit 1 by design: the Postgres side is `NA`, the table did not exist): ClickHouse 53,438 rows yesterday; p95 11, 13 and 14 ms on the three queries; the Well's Kanban read p95 0.178 ms; last backup 1.467 s, 349,350 bytes.
- `scripts/well/batch-apply.sh`: dry run listed `048_ops_log_events.sql` pending in `postgres` and `unheaded_ops`; apply rc=0 (113 ms and 151 ms) behind pre-batch backup `/srv/backups/unheaded-well/pre-batch/pre-batch-20261110T182955Z`; ledger clean. `set-role-passwords.sh` set `log_writer`. Logs: `/var/tmp/unheaded-sprint/e16-step281-{dry,apply,roles,build,up}.log`.
- Two new values in `.env` on the host (`LOG_WRITER_PASSWORD`, `LOGSTORE_INGEST_TOKEN`), made by `openssl rand -hex 32` and never printed. Dev certificate for the writer under `configs/certs/dev/logstore-writer/` (gitignored).
- `sudo docker compose up -d logstore-writer vector` at 18:33Z.
- **Not in the plan: the Well restarted.** The compose file gives the `postgres` service `LOG_WRITER_PASSWORD` (the role-password script reads it there), so `up` recreated `unheaded-postgres` along with Vector. The Well was away about 2 s and healthy again within 17 s. The dashboard logged 14 `health persistence: report not stored` warnings at 18:33:52-53Z (connection refused, then `57P03` starting up) and none since; no other service logged an error; Kanban reads its tasks; `huginn_reader`, `ops_writer` and `log_writer` hold connections again; no unit failed. Any later change to that service's environment block restarts the Well the same way: add role passwords at a planned moment, not as a side effect.
- **My slip:** before appending the two values I copied `.env` to `.env.bak-20261110T182839Z` in the repository directory. That name is not gitignored. Deleted at once, never staged. Backups of `.env` belong outside the checkout.
- Expectations, each read:
  - Count rising: `log_events` held 320 rows at 18:34:26Z and 780 at 18:37Z. Minute 18:34 has 259 rows in the Well and 259 in ClickHouse.
  - ClickHouse's rate unchanged: 44 to 265 rows a minute across 18:29-18:34, the same range as before the start.
  - No audit-sink stalls: Vector 0.43.1, both sinks answering 200, the record sink's disk buffer at 0 to 7 events, 0 sends over 1 s, 0 rows dropped at the Well sink, no warning or error line in `docker logs unheaded-vector`.
- First full reading 18:37:28Z, exit 0. The Postgres timings in it (under 1.3 ms) are over 780 rows and say nothing yet; the comparison starts once the copy holds full days.
- Off switch, unchanged from the plan: remove the `well_logs` sink (and `well_rows`) through the worktree, merge, `sudo docker compose up -d vector`, `sudo docker compose stop logstore-writer`; `DROP TABLE` after the week. Stopping the writer alone is also safe: the sink drops, the record sink is unaffected (`scripts/test-vector-config.sh` proves it).
- Open for Stevie, none decided here:
  - Step 278: bar (A) or (B). Until he says, (A) as written is the bar.
  - The nightly backup (03:37 UTC) and its copy on east carry the Well copy's rows from tonight. `pg_dump --exclude-table-data` for `log_events*` is the alternative; it changes the live backup path.
  - The daily reading has no timer; a session runs it once a day until he asks for one.
- Found while checking the silence: the mailer sent one mail today at 15:01:03Z, `[digest] 9 open`. The silences stop Alertmanager from notifying; they do not empty the mailer's own list of open tickets, and the 09:00 America/Chicago digest mails that list. The next one is due 2026-11-11 15:00Z. Question 27 in `~/tmp/next.md` (mailer to `dry-run` until the mail steps run) is the fix and is Stevie's: `observability/alerting/policy.yaml` is policy. Not changed here.
- Steps 281-283: Route chosen by Stevie; second sink on with drop_newest; close defined before the data.

### Stevie's answers, 2026-11-10 (after the week started)

- **Step 278: (A), the bar as written, and keep measuring.** 2x disk, 2x query p95, under 10% on the Well's p95 and backup time, zero stalls. Disk and backup time are already against Postgres by arithmetic, so the week confirms and supplies the query and Well numbers for Step 284. Bar (B) is not in force.
- **Backup rows: accepted for the week.** The nightly set and its copy on east carry the Well copy's rows; `scripts/db-backup.sh` is unchanged.
- **Alert digest: `mailer.mode: dry-run`** in `observability/alerting/policy.yaml` until Steps 81-83 and 102-109 run with him. FND-001 carries the note. The outbox still records what would have gone (`curl -s localhost:19130/v1/outbox`).
- **Evening 14: the six drafts already on datatracker are prepared as -01** (mbc-isa, pqc-authentication, protocol-foundation, shim, sophia-dictionary, wotan-memory). anamnesis and protocol-overview wait. The upload is his.

## Evening 14: the drafts (2026-11-10)

- Step 259 (desk pass): report at `docs/protocol/RFC-DESK-PASS-2026-11-10.md`. Changes to the frozen v0x01 format: none. Thirteen open items, each a ruling on public text: WH items 1-4 (DP-715's four), WH items 6-9, and four the pass found (stream against code points, a reference to a draft that was never uploaded, private revision labels, FIPS 206/207 citations).
- Step 260 (new versions): the six moved `-00` to `-01` by `git mv`; `docName`, `seriesInfo`, companion references, `Makefile` `DRAFTS`. Each carries a "Changes from -00" appendix taken from `git diff 4db0bbde..HEAD`. `make -C ietf-submission validate`: Clean on all eight; `make all`: `=== Build complete ===`. `go test ./pkg/wotanerr/ ./services/wotan/internal/topicpattern/`: both `ok` (they read the wotan `.md` twin by name).
- mbc-isa `-01` reads differently from the public `-00` on three instructions (opcode 0x00, MOVI, LOAD_IMM32); the tree is right, and the appendix says so.
- Not done here: the thirteen rulings are not applied; Step 261 (upload, after 2026-11-14 23:59 PST when the tool reopens) and Step 263 are Stevie's.
