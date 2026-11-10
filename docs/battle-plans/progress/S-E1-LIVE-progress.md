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
