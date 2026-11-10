<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-E1-LIVE BATTLE PLAN — 19 Sittings, 306 Steps

**Date**: 2026-11-04
**Sprint**: S-E1-LIVE — the attended plan: paper and accounts, E1 stack up, then the doors, keys and first nights. Stevie present for everything except Evening 0.
**Prerequisite**: `SPRINT-SERIES-2026-11-04.md` and the pack's "Answers recorded" read by whoever sits with you. S-HOST-TIDY Phases 0-17 merged. Wave 1 lanes merged or idle before Evening 4 (`make binaries` and the image builds want the box).
**Target**: "done = running live with alerts" (2026-11-03): both compose stacks up on TLS, alert-mailer sending from bellis.tech, FND-001 closed, Well backups green on both hosts, ADR-103 and ADR-106 accepted; then east and west behind their firewalls, sshd finished, keys in custody, and the first Zhen night run.
**Estimated Duration**: 18 sittings, about 34 hours of your time, listed below. Each stands alone: stop after any one, and nothing is left half-open.
**Branch**: `sprint/s-e1-live` in `/home/govan/tmp/wt/S-E1-LIVE` for repo edits. Live commands run from `/home/govan/tmp/unheaded` on develop, because `.env`, `configs/keys`, `configs/certs` and the compose project live there. Each evening ends with a rule 15 merge to develop, and an edit used live merges first. You push.
**Agent Strategy**: you and Claude together, in order. No swarm, no model tags. Evening 0 is Claude alone, but rule 13 still binds: it edits firewall rulesets, a root-run gate and key runbooks, so it is Opus, never a cheaper agent. Steps tagged `[CLAUDE]` are typed by Claude while you watch; `[STEVIE]` needs your hands, eyes or a browser.
**Commit Cadence**: a [C] step every 3-5 steps, never inside an open firewall or sshd window, and at each evening's end, format `[PLAN S-E1-LIVE] Steps X-Y: ...` plus the two trailers.
**Bound by**: series rules 1-17 (`docs/battle-plans/SPRINT-SERIES-2026-11-04.md`), which win over anything here.
**Stuck Protocol**: 3x the estimate or two failed debug attempts: roll back with that step's Rollback, mark [STUCK], go to the next independent step, or end the evening. Output the plan did not predict on a live host means the same: roll back first, understand it later.
**Scope fence**: nothing outside the pack. New ideas become parked `Proposed` ADRs. No new package, image or action without your word in the room.

## HOW TO READ THIS

Each step says what to do, what you should see (**Expect**), what to do otherwise (**Else**) and, for any live change, how to undo it (**Rollback**). If it asks you to run something, the block is exact. Code blocks sit inside list items: drop the two leading spaces of each line before pasting. Your password never goes in chat, a file in the repo, or a command line. New secrets go in `~/tmp/secrets-2026-11.md` (mode 0600, outside any repository); Step 30 makes it.

**Three rules for every evening.** 1. Keep a second terminal logged in on west for the whole sitting and do not close it. 2. Firewall and sshd changes always have a timer armed before they happen; if you lose the connection, wait for the timer. 3. West and east sit in your office: a keyboard and monitor are the last resort, and that is why these evenings are attended.

## WORDS USED HERE

| Word | Meaning |
|---|---|
| KEX | key exchange: how an SSH or TLS session agrees its keys; hybrid means post-quantum plus classical |
| commit-confirm | a firewall apply that undoes itself unless you confirm in time |
| hold session | an SSH connection kept open so a bad change cannot lock the only way in |
| SLO | a service target; alert mail is sent only when one burns its budget |
| sysusers | systemd's file of service accounts |
| trust.d | the directory of public keys a service accepts tokens from |
| LUKS | Linux disk encryption; here one file, opened by a YubiKey |
| cask | a Sealed Cask: a signed release image |
| journalmatch | the filter that tells fail2ban which log lines to read |
| DP-301 | your rule: a runbook is approved case by case when first needed |
| Erda pass | a review by the database skill |
| RFC desk | the review by the RFC editor skill before a spec changes |

## THE EVENINGS

| # | Sitting | Pack ids | Your time | Have ready |
|---|---|---|---|---|
| 0 | Bench work, you are not needed | code for 710, 22, 23, 05, 06, 03 | none (Claude, ~4 h) | nothing |
| 1 | Paper | DP-07, DP-604 | 2 h | mail client on stevie@bellis.tech; an hour of reading time; GPG cached on west |
| 2 | GitHub, Console, Wi-Fi | DP-602, DP-701, DP-714, exposed secrets | 1.5 h | at the desk; laptop browser logged in to GitHub as owner of `unheaded/unheaded`, and to console.anthropic.com; `gh` on the laptop; router admin login; every Wi-Fi device to hand |
| 3 | Credentials | DP-01 | 2 h | Pi-hole admin access; time to look at each row; a place to keep `~/tmp/secrets-2026-11.md` safe |
| 4 | E1, part one: keys, certs, trust.d, the Well | E1, DP-605 | 2.5 h | west free of builders; GPG cached |
| 5 | E1, part two: TLS, both stacks, mail, backups | E1, DP-23, DP-19, DP-710, FND-001, ADR-103/106 | 4 h | Google Workspace admin for bellis.tech; Namecheap DNS login; your phone to see a test alert |
| 6 | East door | DP-22 (east), DP-11 port | 1.5 h | be at the desk: east's keyboard and monitor within reach |
| 7 | West firewall | DP-22 (west) | 1.5 h | at the desk, E1 live and quiet for 2 days; laptop and phone on the LAN |
| 8 | West sshd | DP-04, DP-15 | 1 h | laptop; the devices behind .135 and .137 named; laptop `ssh -V` |
| 9 | PSK timer | DP-05 | 45 m | nothing special |
| 10 | Custody and signing | DP-06, DP-14, Heimdall, Wotan keys | 4 h | both YubiKeys and their FIDO2 PINs (no PIN yet: set one in Chrome, Settings, Privacy and security, Manage security keys); GPG cached |
| 10B | East's debs and TLS | finishes DP-23 | 2 h | at the desk |
| 11 | East as a node | DP-206 | 2 h | at the desk |
| 12 | Browser and Lich | DP-505 | 1 h | a browser on west or a tunnel; GPU free |
| 13 | First Zhen nights | DP-303 | 30 m + 30 m next day, twice | a card or two for Zhen; Anthropic key in the env file |
| 14 | Internet-Drafts | DP-606 | 2 h | datatracker.ietf.org login |
| 15 | PG 18 | DP-03 | 1.5 h | E1 stable a full week, Sunday drill green; a quiet evening |
| 16 | Log store, measured week | Log store answer | 1 h start + 1 h a week later | E1 live; Erda pass on the table |
| 17 | Leftovers | 25.3, 25.4, 25.5, 25.8 (west) | 2 h | at the desk; no lanes running |

## WHAT THE TREE AND THE HOSTS SAID (read-only, 2026-11-04, develop `fdd8f931`)

| Pack or brief | Found | What this plan does |
|---|---|---|
| DP-710: per-hour cap, every send logged, ADR-116 expectation | `cmd/alert-mailer` has `max_per_day` only; no per-hour cap; sends go to a 50-entry outbox and no log line; `config/monitoring/expectations.yaml` does not exist | Evening 0 builds them |
| DP-02 Gmail vs DP-710 bellis.tech | `monitoring/.env` sends as `stevenrbellis@gmail.com`; bellis.tech MX is Google, SPF already includes Google, DMARC `p=none`, no `google._domainkey`; `wiki/Alerting.md` already describes the Workspace route | use `smtp.gmail.com:587` with a bellis.tech Workspace account. Send-only, nothing listens. **"Configured from here" cannot be literal:** DNS is Namecheap with no API on west, so those are your hands |
| DP-23: compose still plaintext | compose already sets `UNHEADED_TLS_ENABLED=true` on all first-party services; VictoriaMetrics scrapes https. **Prometheus does not**: `wotan`, `dashboard-backend`, `unheaded-daemon` are plain, so the first SLOs would burn from a healthy stack | Evening 0 fixes them with per-target `__scheme__` |
| DP-23: all internal links on TLS at E1 | east runs 1.0.0 debs with no TLS; `unheaded-wotan` crash-loops on a bad flag (100,212 restarts), `unheaded-timeguru` dies on `mkdir data`; the SLO reads wotan on both hosts | **not finished at E1, by sequencing:** east upgrades from the signed repo in 10B. East's old source still works, and 10B's steps need no YubiKey, so they could run right after Evening 5 if you prefer. East's Wotan leaves the SLO until then; FND-017 stays open for east. **Your OK: Step 99** |
| E1 startup | `kingdom-startup.sh` runs one `docker compose up -d`, which gives no order for auth and kanban-app; Sword is in the `deploy` profile and stays off, as pinned | bring-up is staged by hand here; `kingdom-startup.sh` is proven afterwards |
| Exposed in a session transcript | the Wi-Fi passphrase and the start of `ALERT_MAILER` (the Gmail app password) | both rotate: Evening 2, netplan on west included, and Evening 5 |
| `.env` | lacks `APP_AUTH_PASSWORD` and `RETENTION_RUNNER_PASSWORD`; `monitoring/.env` lacks `HEARTBEAT_HOSTS` and `HEARTBEAT_LISTEN`; `configs/keys`, `configs/certs`, `/etc/unheaded/tls`, `/etc/unheaded/trust.d` do not exist | Evening 4 makes them |
| Migrations | S-ZHEN-POLISH adds 044, 045 and 046 to the batch; develop since added 047 (`zhen_actions.status`, 2026-11-08) and 049 (S-ANSWERED) | Evening 4's count grows if those merged first |
| Host units | installed `chaos-controller`, `kev-poller`, `trace-collector` differ from `deploy/systemd`; new ones want TLS and keys | Evening 5 |
| Hosts' CA | `deploy/systemd/README.md` says hosts use the Kingdom CA; compose uses the dev CA | E1 uses the compose CA on hosts too, so clients trust the servers they actually reach. Kingdom CA stays a follow-up |
| Grafana admin rotation | `GF_SECURITY_ADMIN_PASSWORD` applies only to an empty database; both volumes exist | Evening 5 uses `grafana cli admin reset-admin-password` |
| DP-01: 25 ignore entries | `.gitleaksignore` holds **16**, all in the tree: 8 OPNsense `curl -u` lines in `docs/bare-metal`, Grafana README, packer password, a plan's bearer header, an eval doc, four test fixtures. LXD has no trust password set (5.21). 20 history fingerprints cannot leave without a rewrite | table at Evening 3 |
| DP-03: `pg_upgrade --link` | runbook `well-pg18-upgrade.yaml` is **dump and restore**, not `pg_upgrade`, on purpose; its `thaw` curls `http://` which TLS will refuse | follow the runbook; Evening 0 fixes the curls |
| DP-05: command-scoped key on east | `rotate-psk.sh` drives east with `sudo -n sh -s` fed from stdin, an arbitrary root shell, so `command=` alone scopes nothing. Target is `govan@east`, so `PermitRootLogin no` is fine | Evening 0 writes a gate that matches exact commands and a pinned stdin hash |
| DP-06: YubiKeys unlock the seed | `build-key-ceremony.yaml` keeps a plain 0400 seed and says no copy. No `ykman`, `pcscd`, `scdaemon`, `age-plugin-yubikey` on west; **`cryptsetup`, `systemd-cryptenroll` and `libfido2-1` are** | a LUKS2 file with both tokens enrolled by FIDO2, the encrypted file copied to east: no new package. Runbook rewritten in Evening 0 |
| DP-06 accounts | `unheaded-buildkey` and `unheaded-heimdall` do not exist on west | sysusers in Evening 4 |
| DP-14 | live repo is `/var/lib/apt-repo` (`noble`), `scripts/apt-repo.sh` wants `/srv/unheaded-apt` (`unheaded`); nginx root is the old path; `reprepro` state unknown | Evening 10 moves it, by the runbook |
| DP-22 | `west.nft` allows only 7375, 80, 443 from the LAN. Your 2026-11-03 scope also has apt 18888, UIs on direct ports, 8443 for `.132` only. `east.nft` opens 22. `unheaded-nft.service` is not installed. **Docker-published ports never reach the input chain, and `monitoring_monitoring` has IPv6 with `ip6tables` on, while west holds a global IPv6 address: Grafana 3000 may be open to the internet** | Evening 0 edits, including a forward-chain rule; Evenings 6 and 7 install; a phone on cellular proves it |
| DP-11 port | both hosts use `ssh.socket`; a port change needs `daemon-reload` and a socket restart. `offsite-copy.sh` has no port option and its `known_hosts` is keyed on bare `192.168.13.1`, so **nightly offsite backups would break** | `/etc/ssh/ssh_config.d/50-unheaded-east.conf` with `Port` and `HostKeyAlias` fixes every client at once |
| West fail2ban | it works, on the wrong port: `port = ssh` is 22, sshd is 7375. It banned `.132` on 2026-10-29, and `.132` logged in on 7375 a minute later | Evening 8, with the whole LAN range ignored first, since a typo ban becomes real |
| DP-206 | answered; the S-ASCEND-2 east-node plan ran S0-S4 on 2026-10-28 and passed; S-STAGE-2B Appendix D is the confirmatory list, and its abort line uses `pkill -f` | Evening 11 runs it after 10B changed east, and stops by pid |
| Log store | ADR-117 says ClickHouse only; your round-3 answer asks for a measured week of both, Postgres favoured if close. **Pinned Vector 0.43.1 has no `postgres` sink** (tested), and no table exists | Evening 16 opens with a [DECIDE]: a Vector bump (your word, rule 4) or a small first-party writer behind the `http` sink |
| DP-602 | default branch already `develop`; `.github/BRANCH_PROTECTION.md` asks for 2 approvals, impossible alone, and for checks that cannot be selected until CI has run once | 0 approvals; required checks added after DP-701 |
| DP-701 | `ci.yml` has no `workflow_dispatch`: a run needs a push. `docker`, `ebpf`, `release` also `disabled_manually` | enable, then your next push is the run |
| Compose ceilings | main 6.5 GiB + monitoring 3.3 GiB against 14 GiB and about 9 available | watch `free` through Evening 5; ceilings are not usage |

## LEGEND
[B] bash · [V] verify (must pass) · [D] debug (only on failure) · [W] write/edit file · [R] read · [S] sudo · [C] commit checkpoint · [CLAUDE] Claude types it · [STEVIE] your hands or eyes · [DECIDE] a call with a pre-seeded answer · [STUCK] skipped, needs thought · [BLOCKED] waits on a [STUCK]

---

## EVENING 0: BENCH WORK, NO STEVIE NEEDED (Steps 1-17)

**Goal**: everything the later evenings need that is code or text, tested, merged to develop.
**Prerequisite**: none; may run while lanes run (one builder).
**Time**: about 4 h, Claude alone. **Have ready**: nothing.

### Worktree

- [ ] **Step 1** [CLAUDE][B] ~1m: Worktree and progress file.
  ```bash
  git -C /home/govan/tmp/unheaded worktree add -b sprint/s-e1-live /home/govan/tmp/wt/S-E1-LIVE develop 2>&1 | tail -1; ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects; cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '<!-- SPDX-License-Identifier: GPL-3.0-or-later -->' '# S-E1-LIVE progress' '' '## For Stevie' '' '## Evidence' '' > docs/battle-plans/progress/S-E1-LIVE-progress.md && grep -c . docs/battle-plans/progress/S-E1-LIVE-progress.md
  ```
  Expect: `HEAD is now at ...`, `4`. Else: [STUCK].

### DP-710: per-hour cap, send log, volume expectation

- [ ] **Step 2** [CLAUDE][W] ~5m: Test first, in `cmd/alert-mailer/mailer_test.go`, appended at the end of the file.
  ```go
  func TestHourCapHolds(t *testing.T) {
  	m, f, c, _ := newTest(t, "send", 6)
  	m.cfg.MaxPerHour = 2
  	send := func() string {
  		m.mu.Lock()
  		defer m.mu.Unlock()
  		o, err := m.deliver(Message{Urgency: "page", Subject: "s", Body: "b"})
  		if err != nil {
  			t.Fatal(err)
  		}
  		return o
  	}
  	if a, b := send(), send(); a != "sent" || b != "sent" {
  		t.Fatalf("first two: %s %s", a, b)
  	}
  	if got := send(); got != "capped-hour" {
  		t.Fatalf("third in the hour: %s", got)
  	}
  	if len(f.sent) != 2 {
  		t.Fatalf("sent %d, want 2", len(f.sent))
  	}
  	c.t = c.t.Add(time.Hour)
  	if got := send(); got != "sent" {
  		t.Fatalf("next hour: %s", got)
  	}
  }

  // A page held by the hourly cap must not count as delivered, or it is lost.
  func TestHourCappedPageResends(t *testing.T) {
  	m, f, c, _ := newTest(t, "send", 6)
  	m.cfg.MaxPerHour = 1
  	if err := page(m, alert("firing", "slo-a", "fast")); err != nil {
  		t.Fatal(err)
  	}
  	if err := page(m, alert("firing", "slo-a", "fast"), alert("firing", "slo-b", "fast")); err != nil {
  		t.Fatal(err)
  	}
  	if len(f.sent) != 1 {
  		t.Fatalf("sent %d in the hour, want 1", len(f.sent))
  	}
  	c.t = c.t.Add(time.Hour)
  	if err := page(m, alert("firing", "slo-a", "fast"), alert("firing", "slo-b", "fast")); err != nil {
  		t.Fatal(err)
  	}
  	if len(f.sent) != 2 || !strings.Contains(f.sent[1].Subject, "slo-b") {
  		t.Fatalf("the held page was not re-sent: %d mails", len(f.sent))
  	}
  }
  ```
  Run: `cd /home/govan/tmp/wt/S-E1-LIVE && go test ./cmd/alert-mailer/ -run 'TestHourCapHolds|TestHourCappedPageResends' 2>&1 | tail -3`. Expect red: `m.cfg.MaxPerHour undefined`. Else: read the error; two tries, [STUCK].
- [ ] **Step 3** [CLAUDE][W] ~10m: Implement in `cmd/alert-mailer/mailer.go`. `Settings` gains `MaxPerHour int`. `state` gains `Hour string `json:"hour"`` and `HourSent int `json:"hour_sent"``. Add:
  ```go
  // rollHour resets the hourly count when the hour in Settings.Loc changes.
  func (m *Mailer) rollHour() {
  	if h := m.now().In(m.cfg.Loc).Format("2006-01-02T15"); h != m.st.Hour {
  		m.st.Hour, m.st.HourSent = h, 0
  	}
  }
  ```
  In `deliver`, directly after `m.rollDay()`:
  ```go
  	m.rollHour()
  	if m.cfg.MaxPerHour > 0 && m.st.HourSent >= m.cfg.MaxPerHour {
  		m.st.Held++
  		m.st.HeldSinceDigest++
  		m.record(msg, "capped-hour")
  		return "capped-hour", m.save()
  	}
  ```
  After `m.st.Sent = n` add `m.st.HourSent++`. In `record`, as its first line, the send log (subject only, never the body):
  ```go
  	log.Info().Str("urgency", msg.Urgency).Str("outcome", outcome).Str("subject", msg.Subject).Msg("alert-mail")
  ```
  Import `"github.com/rs/zerolog/log"` if `mailer.go` lacks it. In `OutboxEntry`'s comment add `capped-hour`. Held pages and digests must keep their news. Add `func held(o string) bool { return o == "capped" || o == "capped-hour" }`; in `Page` change `if err == nil && outcome != "capped" {` to `if err == nil && !held(outcome) {`; in `Tick` change `if outcome != "capped" {` to `if !held(outcome) {`; change `held back by the daily cap since` to `held back by a cap since`. Then `cd /home/govan/tmp/wt/S-E1-LIVE && go test ./cmd/alert-mailer/ 2>&1 | tail -1`. Expect: `ok`. Else: fix within this file; two tries, [STUCK].
- [ ] **Step 4** [CLAUDE][W] ~5m: Plumb the setting. `cmd/alert-mailer/main.go` policy struct: `MaxPerHour int `yaml:"max_per_hour"``, `s := Settings{Mode: p.Mode, MaxPerDay: p.MaxPerDay, MaxPerHour: p.MaxPerHour}`, and after the `max_per_day` check `if s.MaxPerHour < 1 || s.MaxPerHour > s.MaxPerDay { return s, fmt.Errorf("mailer.max_per_hour %d: want 1..max_per_day", s.MaxPerHour) }`. `cmd/alert-policy/main.go` struct: the same field, and `if p.Mailer.MaxPerHour < 1 || p.Mailer.MaxPerHour > p.Mailer.MaxPerDay { bad("mailer.max_per_hour %d: want 1..max_per_day", p.Mailer.MaxPerHour) }`. `observability/alerting/policy.yaml`, under `max_per_day: 6`:
  ```yaml
  # DP-710: outbound mail is closely watched. Past this in one hour, mail is
  # held and counted, like the daily cap.
  max_per_hour: 3
  ```
  Then `cd /home/govan/tmp/wt/S-E1-LIVE && go run ./cmd/alert-policy && go run ./cmd/alert-policy --check && GOMAXPROCS=4 go test -race ./cmd/alert-mailer/ ./cmd/alert-policy/ 2>&1 | tail -2`. Expect: two `ok` lines. Else: `TestSettingsRefusals` may need `max_per_hour` in its fixture: add `MaxPerHour`/`max_per_hour: 1` there, nothing else; two tries, [STUCK].
- [ ] **Step 5** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 1-5: Evening 0, checked: Plumb the setting. cmd/alert-mailer/main.go policy struct: MaxPerHour int yaml:max_per_hou.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 1-5: Checkpoint in Evening 0: Plumb the setting. cmd/alert-mailer/main.go policy struct: MaxPerHour int yaml:max_per_hou' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 6** [CLAUDE][W] ~10m: The ADR-116 expectation, and a rule that sees it. New file `config/monitoring/expectations.yaml` (ADR-116 R2's first row; the warden that reads it is not built):
  ```yaml
  # SPDX-License-Identifier: GPL-3.0-or-later
  # ADR-116 R2: what we expect to see, so its absence or excess is a finding.
  # Read by people and by monitoring/prometheus/rules/detection.yml until the
  # warden exists.
  expectations:
    - signal: alert-mail-volume
      source: unheaded_alert_mailer_messages_total{outcome="sent"}
      expected: a handful a week; the daily cap is 6, the hourly cap 3
      over: any hour at the cap, or any capped-hour outcome, is a finding
      owner: stevie
      decided: DP-710, 2026-11-04
  ```
  It must reach someone, so in `observability/alerting/policy.yaml` under `promote:` add, then re-run `go run ./cmd/alert-policy`:
  ```yaml
    - alert: AlertMailVolumeHigh
      urgency: ticket
      reason: >-
        DP-710: outbound mail is watched. No SLO sees the mailer's own volume.
  ```
  In `monitoring/prometheus/rules/detection.yml`, directly before `- alert: OutboundVolumeDeviation`, at the same indent:
  ```yaml
      - alert: AlertMailVolumeHigh
        expr: sum(increase(unheaded_alert_mailer_messages_total{outcome=~"sent|capped-hour"}[1h])) >= 3
        labels:
          urgency: none
        annotations:
          summary: "alert-mailer reached its hourly cap"
          description: "DP-710: outbound mail is watched. Read /v1/outbox and the alert-mail log lines."
          runbook_url: "runbooks/security/outbound-volume-deviation.yaml"
  ```
  Add a line to the header comment list naming it. Then `cd /home/govan/tmp/wt/S-E1-LIVE && docker run --rm -v "$PWD/monitoring/prometheus:/p:ro" --entrypoint promtool prom/prometheus:v3.15.0 check rules /p/rules/detection.yml 2>&1 | tail -2; python3 -m pytest -q tests/runbooks/test_alert_runbooks.py 2>&1 | tail -1`. Expect: `SUCCESS: ...`, `... passed`. Else: copy the labels block from `OutboundVolumeDeviation` exactly; two tries, [STUCK].
- [ ] **Step 7** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 6-7: alert-mailer holds past max_per_hour 3, logs every send, rule AlertMailVolumeHigh.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md cmd/alert-mailer/mailer.go cmd/alert-mailer/mailer_test.go cmd/alert-mailer/main.go cmd/alert-policy/main.go observability/alerting/policy.yaml monitoring/prometheus/prometheus.yml monitoring/prometheus/rules/slo.yml monitoring/alertmanager/alertmanager.yml monitoring/prometheus/rules/detection.yml config/monitoring/expectations.yaml && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 6-7: DP-710: hourly cap, send log, volume expectation' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.

### DP-23: Prometheus scrapes TLS where TLS is

- [ ] **Step 8** [CLAUDE][W] ~10m: In `monitoring/prometheus/prometheus.yml`, per-target scheme. Replace the three jobs:
  ```yaml
  - job_name: unheaded-daemon
    metrics_path: /metrics
    tls_config: {ca_file: /etc/prometheus-tls/ca.pem, server_name: localhost}
    static_configs:
      - targets: ['[fd00:dead:beef::1]:19006']
        labels: {host: west, __scheme__: https}
      - targets: ['[fd00:dead:beef::2]:17000']
        labels: {host: east}
  ```
  ```yaml
  - job_name: dashboard-backend
    tls_config: {ca_file: /etc/prometheus-tls/ca.pem, server_name: localhost}
    static_configs:
      - targets: ['[fd00:dead:beef::1]:20000']
        labels: {service: "dashboard-backend", __scheme__: https}
      - targets: ['[fd00:dead:beef::2]:20000']
        labels: {service: "dashboard-backend"}
  ```
  ```yaml
  # Wotan: west only until east's debs are upgraded (Evening 10B); east's
  # 1.0.0 unit crash-loops on a flag, which would burn this SLO from day one.
  - job_name: wotan
    tls_config: {ca_file: /etc/prometheus-tls/ca.pem, server_name: localhost}
    static_configs:
      - targets: ['[fd00:dead:beef::1]:18000']
        labels: {service: "wotan", __scheme__: https}
  ```
  In `monitoring/docker-compose.yml`, prometheus `volumes:`, after `- prometheus_data:/prometheus` (not under `/etc/prometheus`: that mount is read-only):
  ```yaml
      - ../configs/certs/dev/ca.pem:/etc/prometheus-tls/ca.pem:ro
  ```
  Then `cd /home/govan/tmp/wt/S-E1-LIVE && docker run --rm -v "$PWD/monitoring/prometheus:/etc/prometheus:ro" --entrypoint promtool prom/prometheus:v3.15.0 check config --syntax-only /etc/prometheus/prometheus.yml 2>&1 | tail -1; scripts/check-compose-bind-nesting.sh 2>&1 | tail -1 | cut -c1-90`. Expect: `SUCCESS...`, a pass line. Else: two tries, [STUCK]; Evening 5 is then [BLOCKED].

### DP-22: rulesets and the east client file

- [ ] **Step 9** [CLAUDE][W] ~5m: `deploy/nftables/east.nft`: the guard line moves.
  Old: `iifname "enp2s0" tcp dport 22 accept comment "ssh from anywhere on the P2P link (lockout guard)"`
  New: `iifname "enp2s0" tcp dport 7375 accept comment "ssh from anywhere on the P2P link (lockout guard)"`
  `deploy/nftables/west.nft`: Old:
  ```
  		# Home LAN: ssh and the haproxy edge. Nothing else (lxd 8443,
  		# node_exporter, huginn, dockerd metrics, BGP) is offered to the LAN,
  		# and nothing at all to the ISP IPv6 prefix except ssh over link-local.
  		iifname "wlp17s0" ip saddr 192.168.69.128/25 tcp dport { 7375, 80, 443 } accept
  ```
  New:
  ```
  		# Home LAN (Stevie 2026-11-03): ssh, the haproxy edge, the apt repo
  		# (plain HTTP on 18888), and the web UIs on their direct ports. DNS
  		# rides the wlp17s0 DNAT to Pi-hole, so it never reaches this chain.
  		# Docker-published ports are not filtered here either: they DNAT in
  		# Docker's chains. LXD's 8443 is the laptop only. Nothing else
  		# (node_exporter, huginn, dockerd metrics, BGP) is offered to the LAN,
  		# and nothing at all to the ISP IPv6 prefix except ssh over link-local.
  		iifname "wlp17s0" ip saddr 192.168.69.128/25 tcp dport { 7375, 80, 443, 18888, 16666, 20000, 20001, 20002, 20103 } accept
  		iifname "wlp17s0" ip saddr 192.168.69.132 tcp dport 8443 accept comment "LXD: Stevie's laptop only"
  ```
  Also in `west.nft`: `8428` joins both sets, because Prometheus remote-writes to it over the host (`iifname "docker0" tcp dport { 8428, 9100, 9110, 9323, 16666-26666 } accept`, the `br-*` line likewise). Add `iifname "lxdbr0" tcp dport 514 accept` and the `udp` twin, for S-LOG-HARVEST's relay. `docker/compose.cuirass-lxd.yml` wants 8443 from `br-*`: this plan never uses that overlay, so it stays closed; the game-day runbook names it.

  **The serious one.** Published ports DNAT in Docker's chains and never reach `input`. In `chain forward`, directly after `ct state invalid drop`, add:
  ```
  		# Docker-published ports (2026-11-04 review): DNAT skips input, so scope
  		# them here. Nothing published reaches the ISP IPv6 prefix; the LAN gets
  		# only the agreed UIs and Pi-hole's DNS; everything else is dropped.
  		iifname "wlp17s0" ct status dnat meta nfproto ipv6 counter drop comment "no published port on the ISP prefix"
  		iifname "wlp17s0" ct status dnat ip saddr 192.168.69.128/25 meta l4proto { tcp, udp } ct original proto-dst { 53, 3000, 3001, 16666, 20000, 20001, 20002, 20103 } accept
  		iifname "wlp17s0" ct status dnat counter drop comment "published ports stay off the LAN unless agreed"
  ```
  Wotan's 18000/18001 and the service ports leave the LAN by this; east reaches them on `enp16s0`. Then `cd /home/govan/tmp/wt/S-E1-LIVE && GOMAXPROCS=4 go test ./deploy/nftables/ 2>&1 | tail -1; unshare -Urn nft -c -f deploy/nftables/east.nft && unshare -Urn nft -c -f deploy/nftables/west.nft && echo NFT_OK`. Expect: `ok`, `NFT_OK`. A private namespace, no root, nothing changed. Else: two tries, [STUCK].
- [ ] **Step 10** [CLAUDE][W] ~2m: New file `deploy/ssh/50-unheaded-east.conf`, the one client change that covers `ssh east`, root's units and `offsite-copy.sh`.
  ```
  # SPDX-License-Identifier: GPL-3.0-or-later
  # West, /etc/ssh/ssh_config.d/. East's sshd is on 7375 (DP-11). HostKeyAlias
  # keeps every existing known_hosts entry valid, including
  # /etc/unheaded/well-offsite/known_hosts, which is keyed on the bare address.
  Host east 192.168.13.1
      Port 7375
      HostKeyAlias 192.168.13.1
  ```
- [ ] **Step 11** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 8-11: Rulesets carry the 2026-11-03 scope, nft -c clean. Prometheus https per target, east wotan out of the SLO until Evening 10B.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md deploy/nftables/east.nft deploy/nftables/west.nft deploy/ssh/50-unheaded-east.conf monitoring/prometheus/prometheus.yml monitoring/docker-compose.yml && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 8-11: DP-22, DP-23: LAN scope in west.nft, 7375 in east.nft, client file, Prometheus TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.

### DP-05: the gate

- [ ] **Step 12** [CLAUDE][W] ~20m: New file `scripts/wireguard/psk-gate.sh`, the forced command for the rotation key on east. `rotate-psk.sh` stays unchanged. It allows exactly the commands the script sends, and pins the stdin script by hash, so the key is not a root shell.
  ```bash
  #!/usr/bin/env bash
  # SPDX-License-Identifier: GPL-3.0-or-later
  # Copyright (c) 2026 Stevie Bellis.
  #
  # psk-gate.sh: forced command for the WireGuard rotation key on east (DP-05).
  # authorized_keys: command="/usr/local/lib/unheaded/wireguard/psk-gate.sh",
  # no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-pty <key>
  # rotate-psk.sh feeds its side script on stdin to `sudo -n sh -s`; that is a
  # root shell, so its hash is pinned here and anything else is refused.
  set -euo pipefail
  SIDE_SHA=/usr/local/lib/unheaded/wireguard/side.sha256
  c="${SSH_ORIGINAL_COMMAND:-}"
  P='[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]='
  logger -t unheaded-psk-gate -- "request: ${c:0:120}" || true
  case "$c" in
  true) exit 0 ;;
  'sudo -n wg show wg0 public-key') exec sudo -n wg show wg0 public-key ;;
  "sudo -n sh -c 'umask 077; cat >/etc/wireguard/wg0.psk.new'") exec sudo -n sh -c 'umask 077; cat >/etc/wireguard/wg0.psk.new' ;;
  "sudo -n sh -c 'sha256sum </etc/wireguard/wg0.psk.new'") exec sudo -n sh -c 'sha256sum </etc/wireguard/wg0.psk.new' ;;
  esac
  if [[ "$c" =~ ^sudo\ -n\ sh\ -s\ --\ (check|stage-old|apply|restore|commit|cleanup)\ wg0\ /etc/wireguard\ ($P)$ ]]; then
  	body="$(cat; echo x)"
  	body="${body%x}"
  	[ "$(printf '%s' "$body" | sha256sum | cut -d' ' -f1)" = "$(cat "$SIDE_SHA")" ] || { echo "psk-gate: side script hash mismatch" >&2; exit 3; }
  	rc=0
  	printf '%s' "$body" | sudo -n sh -s -- "${BASH_REMATCH[1]}" wg0 /etc/wireguard "${BASH_REMATCH[2]}" || rc=$?
  	exit "$rc"
  fi
  echo "psk-gate: refused" >&2
  exit 3
  ```
  Add cases to `scripts/wireguard/test-rotate-psk.sh`: `SSH_ORIGINAL_COMMAND='id'` expects exit 3; `true` expects 0; and with a stub `sudo` first in `PATH` and `SIDE_SHA` pointed at a temp hash file (make the path overridable: `SIDE_SHA=${PSK_GATE_SIDE_SHA:-/usr/local/lib/unheaded/wireguard/side.sha256}`), a `sudo -n sh -s -- check wg0 /etc/wireguard <key>` request with the matching stdin expects exit 0 and the stub called, and a changed stdin expects exit 3. Then `cd /home/govan/tmp/wt/S-E1-LIVE && chmod +x scripts/wireguard/psk-gate.sh && bash -n scripts/wireguard/psk-gate.sh && SSH_ORIGINAL_COMMAND=id scripts/wireguard/psk-gate.sh; echo rc=$?; SSH_ORIGINAL_COMMAND=true scripts/wireguard/psk-gate.sh; echo rc=$?; scripts/wireguard/test-rotate-psk.sh 2>&1 | tail -1`. Expect: `psk-gate: refused`, `rc=3`, `rc=0`, a pass line. Else: two tries, [STUCK]; Evening 9 is then [BLOCKED].

### DP-06, DP-03: runbooks

- [ ] **Step 13** [CLAUDE][W] ~20m: `runbooks/security/build-key-ceremony.yaml`. Keep `status: draft`, `approved_by: ""`: approval is your act on the evening (DP-301). Remove the "WAITS ON S-IRON-GATE E1" paragraph. Replace the "Custody, as drafted" bullet on the seed with: `The seed lives inside /var/lib/unheaded/custody/buildkey.luks, a LUKS2 file with two FIDO2 slots and no password slot (DP-06); it is opened only for a ceremony and closed after. The encrypted file is copied to east; it is useless without a token. Losing both tokens means a new key and a revocation entry.` Replace step `generate-build-key`'s and `sign-a-test-cask`'s commands, and `audit-watch`'s path, with those of Evening 10, Steps 181 to 186, verbatim; the key path becomes `/run/unheaded-buildkey/${KEY_NAME}.key`. Then `cd /home/govan/tmp/wt/S-E1-LIVE && python3 -m pytest -q tests/runbooks/test_build_key_ceremony.py 2>&1 | tail -1 && scripts/runbook-manifest.sh --write && git diff --stat runbooks/MANIFEST.sha256 | tail -1`. Expect: passed, a one-file diff. Else: fix the test's expected step names to match; two tries, [STUCK].
- [ ] **Step 14** [CLAUDE][W] ~5m: `runbooks/data/well-pg18-upgrade.yaml`, step `thaw`. Old: `curl -fsS http://127.0.0.1:20000/health && curl -fsS http://127.0.0.1:20001/health`. New: `curl -fsS --cacert configs/certs/dev/ca.pem https://localhost:20000/health && curl -fsS --cacert configs/certs/dev/ca.pem https://localhost:20001/health`. `freeze-writers` lists `dashboard-backend kanban-app`: add `timeguru auth` (they hold Well logins too; Sword stays off), and the same names to `thaw`'s `docker compose up -d` and to `back-to-pg16`'s. Regenerate: `cd /home/govan/tmp/wt/S-E1-LIVE && scripts/runbook-manifest.sh --write && python3 scripts/run-runbook.py --dry-run runbooks/data/well-pg18-upgrade.yaml 2>&1 | tail -1`. Expect: `[SUCCESS] Runbook completed.`. Else: [STUCK].
- [ ] **Step 15** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 12-15: psk-gate.sh tested, ceremony runbook rewritten, PG18 thaw uses https.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md scripts/wireguard/psk-gate.sh scripts/wireguard/test-rotate-psk.sh runbooks/security/build-key-ceremony.yaml runbooks/data/well-pg18-upgrade.yaml runbooks/MANIFEST.sha256 && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 12-15: DP-05 gate; ceremony runbook on LUKS and FIDO2; PG18 runbook on TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 16** [CLAUDE][V] ~10m: **EVENING 0 EXIT GATE** — the gates, then merge.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && test -z "$(gofmt -l cmd/alert-mailer cmd/alert-policy deploy/nftables)" && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && ./scripts/check-gosec-ratchet.sh 2>&1 | tail -1 | cut -c1-80 && git status --short | wc -l
  ```
  Expect: a gosec pass line, `0`. Else: fix within this evening; the merge step waits.
- [ ] **Step 17** [C] ~6m: **MERGE (rule 15): Evening 0: bench work.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 0: bench work' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 0: bench work' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 1: PAPER (Steps 18-29)

**Goal**: DP-07 notice sent; DP-604 retention ratified and three attestations signed. Both before E1.
**Prerequisite**: Evening 0.
**Time**: 2 h. **Have ready**: mail client on stevie@bellis.tech; an hour of reading; GPG cached on west (`git commit-tree -S` must work).

- [ ] **Step 18** [CLAUDE] ~20m: **DP-07 Barrister pass.** Run the `unheaded-barrister` skill on `docs/compliance/export-control-notice.md` and on Part E of `docs/legal/PQC-DEPENDENCY-LICENSING-ASSESSMENT.md`, against the notice's own drafting note: is the 742.15(b) email still required for standard cryptography after 86 FR 16482; is XChaCha20 standard; are the addresses `crypt@bis.doc.gov` and `enc@nsa.gov` current; replace the wrong 740.9 and 740.17 citations in Part E. Remember the Barrister is a skill, not your lawyer: the note recorded is its reading.
- [ ] **Step 19** [CLAUDE] ~10m: **Librarian pass.** Run `unheaded-librarian` over the notice's product claims against the tree of the day. Known already: it names "the WAF", and `pkg/waf` leaves in S-CLEAN-CUT (DP-401); confirm which of `pkg/tls/`, `crates/zhend/`, `pkg/secrets/encryption/`, `scripts/wireguard/` still exist: `cd /home/govan/tmp/wt/S-E1-LIVE && ls -d pkg/tls crates/zhend pkg/secrets/encryption scripts/wireguard pkg/waf crates/waf 2>&1`. Correct the notice's list and the email text to what exists.
- [ ] **Step 20** [STEVIE] ~5m: Read the notice's "The email" block as corrected. Say yes or change it.
- [ ] **Step 21** [STEVIE] ~5m: Send it from stevie@bellis.tech, to the addresses the Barrister confirmed, subject and body exactly as in the file, `<address the email is sent from>` filled in. Keep the sent mail. **Expect:** no bounce in ten minutes. **Else:** a bounce from one address: note it, send to the other, tell Claude; a bounce from both: [STUCK], the Barrister finds current addresses.
- [ ] **Step 22** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 18-22: Evening 1, checked: Send it from steviebellis.tech, to the addresses the Barrister confirmed, subject and body.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 18-22: Checkpoint in Evening 1: Send it from steviebellis.tech, to the addresses the Barrister confirmed, subject and body' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 23** [CLAUDE][W] ~3m: In `docs/compliance/export-control-notice.md`, table: `Notification sent` → the date; `Sent by` → `Stevie Bellis, stevie@bellis.tech`; `Reviewed by (Barrister)` → `2026-MM-DD, skill pass; note in this file`. `**Status:**` line → `sent <date>`. Note the row in the control matrix named under "After sending".
- [ ] **Step 24** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 23-24: Export notice reviewed by Barrister and Librarian skills and sent.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md docs/compliance/export-control-notice.md docs/legal/PQC-DEPENDENCY-LICENSING-ASSESSMENT.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 23-24: DP-07: notice corrected and sent' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 25** [STEVIE] ~60m: **DP-604: read.** `docs/policy/retention-policy-v1-2026-05-06.md` §11 (the table is what you ratify); the three drafts in `docs/compliance/drafts/attestations/` and the documents each names in its README: infosec policy and single-operator posture; IR plan v2, tabletop, contingency plan; the access list. The access review wants you to walk the accounts on both hosts, as they stand after S-HOST-TIDY. Say what you would change; any change goes in now.
- [ ] **Step 26** [STEVIE][W] ~2m: Ratify. This is your act, so it is your signed commit on develop, not on the sprint branch.
  ```bash
  cd /home/govan/tmp/unheaded && git status --short | wc -l && sed -i 's/^ratified: false$/ratified: true/' compliance/retention/classes.yaml && sed -n 24p compliance/retention/classes.yaml && git add compliance/retention/classes.yaml && git commit -S -q -m 'retention: ratify policy v1 classes (DP-604)' && git log -1 --format='%h %G?'
  ```
  Expect: `0`, `ratified: true`, `<hash> G`. Else: a non-zero first number: a tracked file is dirty: ask Claude, do not stash. `N` at the end: cache GPG and `git commit --amend -S --no-edit`.
  **Rollback:** `git revert <hash>`. The timer is not installed yet, so nothing has run.
- [ ] **Step 27** [STEVIE] ~5m: Sign the three, dry run first for each.
  ```bash
  cd /home/govan/tmp/unheaded && for a in security-policy-review ir-plan-review access-review; do scripts/compliance/sign-attestation.sh --dry-run "$a" | head -6; done
  ```
  ```bash
  cd /home/govan/tmp/unheaded && for a in security-policy-review ir-plan-review access-review; do scripts/compliance/sign-attestation.sh "$a"; done; git log -3 --format='%h %G? %s'
  ```
  Expect: three lines `<hash> G attest: <name> reviewed <date>`. Else: the script refuses any identity but `stevie@bellis.tech`: `git config user.email`. Other: [STUCK].
- [ ] **Step 28** [V] ~1m: **EVENING 1 EXIT GATE**.
  ```bash
  cd /home/govan/tmp/unheaded && ls compliance/attestations/ | grep -cE '^(security-policy-review|ir-plan-review|access-review)\.yaml$'; git -C /home/govan/tmp/wt/S-E1-LIVE log --oneline -1 | cut -c1-60
  ```
  Expect: `3`, a line. Else: [STUCK]. The retention timer itself waits for Evening 5 (migrations 042-043).
- [ ] **Step 29** [C] ~6m: **MERGE (rule 15): Evening 1: paper.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 1: paper' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 1: paper' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 2: GITHUB, CONSOLE, WI-FI (Steps 30-50)

**Goal**: DP-602, DP-701, DP-714, and the two secrets a session transcript exposed: the Wi-Fi passphrase and the start of `ALERT_MAILER`.
**Prerequisite**: Evening 0 merged.
**Time**: 1.5 h. **Have ready**: at the desk; laptop browser logged in to GitHub as owner of `unheaded/unheaded` and to console.anthropic.com; `gh` on the laptop; router admin login; every Wi-Fi device to hand.

- [ ] **Step 30** [STEVIE] ~1m: Make the file. `~/tmp` is not a git repository.
  ```bash
  git -C ~/tmp rev-parse 2>&1 | head -1 | cut -c1-40; install -m 0600 /dev/null ~/tmp/secrets-2026-11.md && stat -c '%a %n' ~/tmp/secrets-2026-11.md
  ```
  Expect: `fatal: not a git repository...`, `600 /home/govan/tmp/secrets-2026-11.md`. Else: pick `~/secrets-2026-11.md` instead, and use that path everywhere.
- [ ] **Step 31** [STEVIE] ~5m: **DP-602.** GitHub, repo Settings, Advanced Security (or Code security): turn on **Secret Protection** with **Push protection**. Expect both switches on. Existing findings may appear under Security: expected, they are DP-01's rows. Else: on a free plan for public repos these are available: if the setting is missing, [STUCK], note what the page shows.
- [ ] **Step 32** [STEVIE] ~5m: Settings, Branches (or Rules, Rulesets): protect `main`: require a pull request with **0** approvals, block force pushes, block deletion. `.github/BRANCH_PROTECTION.md` says 2 approvals: you cannot approve yourself, so it would lock you out of main. Do not add required checks yet: they cannot be selected until CI has run. The default branch is already `develop`: nothing to do.
- [ ] **Step 33** [STEVIE][V] ~2m: From the laptop:
  ```bash
  gh api repos/unheaded/unheaded/branches/main/protection --jq '.allow_force_pushes.enabled, .allow_deletions.enabled, .required_pull_request_reviews.required_approving_review_count'
  ```
  Expect: `false`, `false`, `0`. Else: if you used a Ruleset the API path differs: `gh api repos/unheaded/unheaded/rulesets --jq '.[].name'` shows it; fine.
  **Rollback:** Settings, Branches, delete the rule.
- [ ] **Step 34** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 30-34: Evening 2, checked: From the laptop:.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 30-34: Checkpoint in Evening 2: From the laptop:' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 35** [STEVIE] ~3m: **DP-701.** Actions tab, workflow **CI**, the `...` menu, **Enable workflow**. Then push develop (yours alone): `git push origin develop`. `ci.yml` has no manual trigger, so the push is the run.
- [ ] **Step 36** [CLAUDE][V] ~20m: Watch one run from west (there is no `gh` here; this is public).
  ```bash
  curl -s 'https://api.github.com/repos/unheaded/unheaded/actions/workflows/ci.yml/runs?per_page=1' | python3 -c "import sys,json; r=json.load(sys.stdin)['workflow_runs'][0]; print(r['head_sha'][:8], r['status'], r['conclusion'], r['html_url'])"
  ```
  Expect: your head, `completed success`. Else: `completed failure`: do not enable the others. Claude reproduces it in a clean clone (`reference_ci_repro_recipe`), fixes on the branch, you push again; two rounds, then [STUCK] and the rest of this step waits for another evening.
- [ ] **Step 37** [STEVIE] ~5m: Once green: enable **Docker**, **eBPF & Rust**, **Release** the same way. In the branch rule for `main`, add required checks from the names CI now shows.
- [ ] **Step 38** [CLAUDE][W] ~5m: `.github/BRANCH_PROTECTION.md`: status line → `APPLIED <date>`; **2 approvals** → **0 approvals (solo project; the PR requirement stays)**; remove the `production` deployment line; list the checks you chose; record secret scanning and push protection on.
- [ ] **Step 39** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 35-39: Evening 2, checked: Watch one run from west there is no gh here this is public..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 35-39: Checkpoint in Evening 2: Watch one run from west there is no gh here this is public.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 40** [STEVIE] ~5m: **DP-714, find the April key.** On west, last four characters only:
  ```bash
  sudo bash -c '. /etc/unheaded/anthropic.env; printf "anthropic.env ...%s\n" "${ANTHROPIC_API_KEY: -4}"'; grep -c 'sk-ant-' ~/.bashrc
  ```
  In console.anthropic.com, API keys: match by the visible hint. Expect: the April key is the one to revoke, and you can see which file holds the key that stays. Else: if the only live key is in `anthropic.env` with an April date, it **is** the April key: make a new one first, put it in `/etc/unheaded/anthropic.env` and in `~/.bashrc`'s line, confirm the new one answers in the Console's usage page after one call, then revoke.
  **Rollback:** none for a revoked key; make a new one.
- [ ] **Step 41** [STEVIE] ~2m: Revoke it in the Console.
- [ ] **Step 42** [STEVIE][W] ~2m: Drop `ALERT_MAILER` from `~/.bashrc`. Its value is the Gmail app password; the 0400 file is its home. **That password is exposed:** a session transcript holds its start. Evening 5 replaces the sender, and Step 108 revokes it. If Evening 5 is more than a few days off, revoke it now: nothing sends mail while the stack is down.
  ```bash
  cp -p ~/.bashrc ~/.bashrc.bak-2026-11 && sed -i '/^export ALERT_MAILER=/d' ~/.bashrc && grep -c '^export ALERT_MAILER=' ~/.bashrc; shred -u ~/.bashrc.bak-2026-11
  ```
  Expect: `0`. Else: [STUCK]. Open shells keep the variable until they exit.
- [ ] **Step 43** [STEVIE][R] ~2m: **Wi-Fi passphrase: it is exposed in a session transcript, so it changes tonight.** West joins Wi-Fi through netplan, so west changes too. Which file, by name only. Never paste the value into chat or a command line.
  ```bash
  sudo grep -l 'access-points' /etc/netplan/*.yaml
  ```
  Expect: one path. Else: none: west is not on Wi-Fi by netplan: tell Claude before changing the router.
- [ ] **Step 44** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 40-44: Evening 2, checked: Watch one run from west there is no gh here this is public..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 40-44: Checkpoint in Evening 2: Watch one run from west there is no gh here this is public.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 45** [STEVIE][W] ~5m: New passphrase, 20 characters or more, into your secrets file by hand. Back up the old netplan file, edit its `password:` value in your editor, and check it parses. `generate` restarts nothing, so west stays on the old network for now.
  ```bash
  N=$(sudo grep -l 'access-points' /etc/netplan/*.yaml); sudo cp -p "$N" /root/netplan-wifi.bak-e1 && sudo ${EDITOR:-nano} "$N" && sudo netplan generate && echo NETPLAN_OK; sudo stat -c '%a %U' "$N"
  ```
  Expect: `NETPLAN_OK`, `600 root`. Else: `sudo cp -p /root/netplan-wifi.bak-e1 "$N"`, stop: nothing has changed yet.
  **Rollback:** `sudo cp -p /root/netplan-wifi.bak-e1 "$N" && sudo netplan generate`
- [ ] **Step 46** [STEVIE] ~5m: At the router, set the new passphrase. West and every device drop off now. On **west's console**, not over SSH:
  ```bash
  sudo netplan apply; sleep 15; ip -br addr show wlp17s0 | cut -c1-50; ping -c3 -W2 192.168.69.129 | tail -2 | head -1 | cut -c1-50; ip -br addr show enp16s0 | cut -c1-40; ssh -o BatchMode=yes -o ConnectTimeout=8 east 'echo EAST_OK'
  ```
  Use your router's address for the ping. Expect: `wlp17s0 UP ... 192.168.69.184/25`, replies, `enp16s0 UP`, `EAST_OK`. Else: wait a minute and re-run the check; still down: Rollback: put the old passphrase back on the router, then `sudo cp -p /root/netplan-wifi.bak-e1 "$N" && sudo netplan apply`.
  **Rollback:** the Else line, then pick another evening.
- [ ] **Step 47** [STEVIE][V] ~15m: Every other device: laptop, phone, TV, the rest. From the laptop `ssh -p 7375 govan@192.168.69.184 true`. On west, DNS for the LAN still flows: `sudo nft list ruleset | grep -E 'wlp17s0.*dport 53' | head -1 | cut -c1-90` shows the counter moving between two looks. Then destroy the old value.
  ```bash
  sudo shred -u /root/netplan-wifi.bak-e1; sudo test -e /root/netplan-wifi.bak-e1; echo gone_rc=$?
  ```
  Expect: `gone_rc=1`. Else: [STUCK].
- [ ] **Step 48** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 45-48: GitHub settings on and recorded, CI green with docker, ebpf, release enabled, April key revoked, ALERT_MAILER out of bashrc, Wi-Fi passphrase rotated on the router and in west netplan.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md .github/BRANCH_PROTECTION.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 45-48: DP-602, DP-701, DP-714: settings recorded, CI on, April key revoked' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 49** [V] ~1m: **EVENING 2 EXIT GATE**.
  ```bash
  curl -s https://api.github.com/repos/unheaded/unheaded/actions/workflows | python3 -c "import sys,json; print(sum(w['state']=='disabled_manually' for w in json.load(sys.stdin)['workflows']))"
  ```
  Expect: `0`. Else: a number above 0 with CI red is a fine place to stop; say so in the progress file.
- [ ] **Step 50** [C] ~6m: **MERGE (rule 15): Evening 2: GitHub and Console.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 2: GitHub and Console' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 2: GitHub and Console' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 3: CREDENTIALS (Steps 51-62)

**Goal**: DP-01: every baselined credential rotated or proved dead, new values recorded outside the repo, the baseline shrunk.
**Prerequisite**: Evening 0.
**Time**: 2 h. **Have ready**: Pi-hole admin access; the secrets file from Evening 2.

| `.gitleaksignore` rows (16, all in the tree) | What they are | What happens |
|---|---|---|
| `docs/bare-metal/BOOT_SEQUENCE_HOST_A.md` 283, 298, 304, 310, 369; `BOOT_SEQUENCE_HOST_B.md` 94; `WIREGUARD_TUNNEL.md` 459, 487 | `curl -u` against OPNsense at 192.168.1.1 and 192.168.2.1 | the appliances are gone: prove nothing answers, replace the text with `"$OPNSENSE_API_KEY:$OPNSENSE_API_SECRET"`, row out |
| `monitoring/grafana/README.md` 187 | Grafana auth header example | new admin passwords (Evening 5 applies them), text to a variable, row out |
| `nix/yggdrasil/packer/template.pkr.hcl` 127 | packer `ssh_password` | to a `var.`, row out |
| `docs/battle-plans/tools/anamnesis-lite-battle-plan.md` 1543 | bearer example on a local API | to `$TOKEN`, row out |
| `eval/coding-gate/probe-2026-05-02/A2-agent-adversarial.md` 115 | key in an eval transcript | check it was never real, replace with `REDACTED`, row out |
| `cmd/lich-security/campaigns/d1_auth_bypass.go` 143, `pkg/auth/apikey_test.go` 26, 199, `tests/e2e/security_e2e_test.go` 330 | test fixtures, never credentials | build the string at run time so the scanner stops matching, row out |

Live secrets outside that table but in the pack's sentence: Grafana admin x2, LXD trust (none set on 5.21.9: confirmed empty, nothing to rotate), Pi-hole admin, the heartbeat token. Well role passwords were rotated 2026-10-27 and are not baselined: left alone.

- [ ] **Step 51** [CLAUDE][B] ~5m: List what history holds, redacted, so the 20 older fingerprints get names.
  ```bash
  cd /home/govan/tmp/unheaded && gitleaks git --redact --no-banner --report-format json --report-path /var/tmp/gl-history.json . 2>&1 | tail -1; python3 -c "import json,collections; c=collections.Counter((f['RuleID'],f['File']) for f in json.load(open('/var/tmp/gl-history.json'))); [print(n,r,f) for (r,f),n in c.most_common(30)]"
  ```
  Expect: a list of rule and path, with no values. Else: [STUCK] on history; the tree rows still go. Delete the report at the end: Step 59.
- [ ] **Step 52** [STEVIE] ~10m: Go through that list with Claude and mark each: **live** (rotate), **dead** (say why). History cannot be shrunk without a rewrite, which is not in the pack: a dead or rotated secret in history is the end state.
- [ ] **Step 53** [CLAUDE][V] ~3m: The OPNsense appliances are dead.
  ```bash
  for a in 192.168.1.1 192.168.2.1; do timeout 4 curl -sk -o /dev/null -w "$a %{http_code}\n" "https://$a/" || echo "$a no answer"; done
  ```
  Expect: `no answer` or `000` for both. Else: something answers: find out what before calling it dead. That row becomes **live**.
- [ ] **Step 54** [STEVIE] ~5m: Pi-hole admin password. You type it; it never reaches chat. Then add it to the secrets file.
  ```bash
  /snap/bin/lxc exec pihole --project default -- pihole setpassword
  ```
  Expect: `New password set`. Else: [STUCK]; the DNS keeps working regardless.
  **Rollback:** run it again with a new value.
- [ ] **Step 55** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 51-55: Evening 3, checked: The OPNsense appliances are dead..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 51-55: Checkpoint in Evening 3: The OPNsense appliances are dead.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 56** [STEVIE][W] ~3m: New Grafana admin passwords into the two `.env` files, without showing them. Evening 5 applies them to the running databases.
  ```bash
  cd /home/govan/tmp/unheaded && for f in .env monitoring/.env; do p="$(openssl rand -base64 24 | tr -d '/+=')"; sed -i "s|^GRAFANA_ADMIN_PASSWORD=.*|GRAFANA_ADMIN_PASSWORD=$p|" "$f"; printf 'grafana admin (%s): %s\n' "$f" "$p" >> ~/tmp/secrets-2026-11.md; done; unset p; grep -c '^GRAFANA_ADMIN_PASSWORD=' .env monitoring/.env
  ```
  Expect: `.env:1`, `monitoring/.env:1`. Else: re-run this step; the stacks are down, so no consumer is affected.
- [ ] **Step 57** [STEVIE] ~3m: Heartbeat token, same bytes on both hosts, never printed.
  ```bash
  t="$(openssl rand -hex 32)"; printf '%s' "$t" | sudo install -m 0400 -o 16710 -g 16710 /dev/stdin /etc/unheaded/alert-mailer/heartbeat-token; printf '%s' "$t" | ssh east 'sudo install -m 0400 -o root -g root /dev/stdin /etc/unheaded/heartbeat.token'; unset t; sudo sha256sum /etc/unheaded/alert-mailer/heartbeat-token | cut -c1-12; ssh east 'sudo sha256sum /etc/unheaded/heartbeat.token | cut -c1-12'
  ```
  Expect: two equal strings. Else: re-run.
- [ ] **Step 58** [CLAUDE][W] ~20m: In the worktree, rewrite the 16 lines per the table. The OPNsense lines become `-u "$OPNSENSE_API_KEY:$OPNSENSE_API_SECRET"`. Fixtures: build the string so no literal matches, for example `"uhk_" + strings.Repeat("a", 40)`, keeping each test's meaning. Then, with every row handled, empty the two lists.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && grep -c . .gitleaksignore docs/security/gitleaks-baseline-fingerprints.txt; sed -i '/^[^#]/d' .gitleaksignore && : > docs/security/gitleaks-baseline-fingerprints.txt; ./scripts/check-gitleaks.sh 2>&1 | tail -1 | cut -c1-90; ./scripts/check-secrets-baseline.sh 2>&1 | tail -1 | cut -c1-90; GOMAXPROCS=4 go test ./pkg/auth/ ./cmd/lich-security/... 2>&1 | grep -vE '^ok|no test files' | wc -l
  ```
  Expect: pass lines from both scripts, `0`. Else: `check-gitleaks.sh` names what it still finds: fix that line, not the list. A row you could not finish: restore just that fingerprint from `git show HEAD:docs/security/gitleaks-baseline-fingerprints.txt`, it stays, and it goes in the progress file.
- [ ] **Step 59** [CLAUDE][B] ~1m: `shred -u /var/tmp/gl-history.json`. Update `docs/security/PRE-PUBLIC-BLOCKERS.md` and the register's SP-01 and CG-17 rows named in `docs/sweeps/OUTSTANDING-2026-10-27.md` M-010.
- [ ] **Step 60** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 56-60: 16 tree rows gone; Pi-hole, Grafana, heartbeat rotated; new values only in ~/tmp/secrets-2026-11.md; LXD trust confirmed unset.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md .gitleaksignore docs/security/gitleaks-baseline-fingerprints.txt docs/security/PRE-PUBLIC-BLOCKERS.md docs/bare-metal/BOOT_SEQUENCE_HOST_A.md docs/bare-metal/BOOT_SEQUENCE_HOST_B.md docs/bare-metal/WIREGUARD_TUNNEL.md monitoring/grafana/README.md nix/yggdrasil/packer/template.pkr.hcl docs/battle-plans/tools/anamnesis-lite-battle-plan.md eval/coding-gate/probe-2026-05-02/A2-agent-adversarial.md cmd/lich-security/campaigns/d1_auth_bypass.go pkg/auth/apikey_test.go tests/e2e/security_e2e_test.go && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 56-60: DP-01: baseline emptied, credentials rotated or proved dead' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 61** [V] ~1m: **EVENING 3 EXIT GATE**.
  ```bash
  grep -c . ~/tmp/secrets-2026-11.md; stat -c '%a' ~/tmp/secrets-2026-11.md
  ```
  Expect: `3` or more, `600`. Else: [STUCK].
- [ ] **Step 62** [C] ~6m: **MERGE (rule 15): Evening 3: credentials.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 3: credentials' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 3: credentials' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 4: E1 PART ONE: KEYS, CERTS, TRUST.D, THE WELL (Steps 63-80)

**Goal**: everything compose needs on disk, and the Well up with migrations 030-043 applied (DP-605). It ends with only `postgres` running: a clean place to stop.
**Prerequisite**: Evenings 0 and 3. No builder running.
**Time**: 2.5 h. **Have ready**: GPG cached; the secrets file. Do not recreate kanban-app or sword tonight: both refuse an unmigrated Well.

- [ ] **Step 63** [CLAUDE][V] ~2m: Preflight.
  ```bash
  cd /home/govan/tmp/unheaded && git branch --show-current && git status --short | wc -l && free -g | awk '/Mem:/{print "avail", $7}' && pgrep -cx 'go|cargo|rustc|llama-server'; docker ps -q | wc -l; test -s .env && test -s monitoring/.env && echo ENV_OK
  ```
  Expect: `develop`, `0`, `avail 8` or more, `0`, `0`, `ENV_OK`. Else: stop what is running. Do not count swap. Under 8 available: find out why before going on.
- [ ] **Step 64** [CLAUDE][S] ~1m: Accounts. West's sysusers is behind, so `unheaded-buildkey` and `unheaded-heimdall` are missing.
  ```bash
  cd /home/govan/tmp/unheaded && sudo cp -p /etc/sysusers.d/unheaded.conf /var/backups/unheaded-sysusers.conf.bak-e1 && sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers unheaded.conf; id -u unheaded-buildkey; id -u unheaded-heimdall
  ```
  Expect: `16737`, `16736`. Else: [STUCK]; Evening 10 needs them, tonight does not.
  West reads `/etc/sysusers.d`, which beats a copy under `/usr/lib`. S-HOST-TIDY Phase 18 may have done this: same result.
  **Rollback:** `sudo cp -p /var/backups/unheaded-sysusers.conf.bak-e1 /etc/sysusers.d/unheaded.conf`; accounts with no files are harmless.
- [ ] **Step 65** [CLAUDE][B] ~15m: Build every binary the units run.
  ```bash
  cd /home/govan/tmp/unheaded && GOMAXPROCS=4 make binaries 2>&1 | tail -2; ls bin/release | wc -l; bin/release/unheaded-keygen --version
  ```
  Expect: a count above 30, `unheaded-keygen (unheaded) <version>`. Else: fix, or end the evening here: nothing live changed.
- [ ] **Step 66** [STEVIE][S] ~3m: Service keys and trust.d. The script prints `dev password: ...` **once**: copy it into `~/tmp/secrets-2026-11.md` by hand, line `dashboard login dev: <value>`. Claude does not need to see it: run this yourself in your own terminal.
  ```bash
  cd /home/govan/tmp/unheaded && sudo scripts/gen-dev-keys.sh | tee /dev/tty | grep -v '^dev password' | tail -3; ls configs/keys/dev | wc -l; ls configs/trust.d/dev | wc -l; test -s configs/auth/dev-users.json && echo USERS_OK
  ```
  Expect: the kid table, two equal counts near 15, `USERS_OK`. Else: an existing key is never overwritten, so a re-run is safe. Lost the password: `sudo scripts/gen-dev-keys.sh --reset-users`.
  **Rollback:** `sudo rm -rf configs/keys/dev configs/trust.d/dev configs/auth/dev-users.json` (nothing uses them yet)
- [ ] **Step 67** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 63-67: Evening 4, checked: Preflight..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 63-67: Checkpoint in Evening 4: Preflight.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 68** [CLAUDE][S] ~3m: TLS certificates: once as you, once with sudo so each key belongs to its uid.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/gen-dev-certs.sh 2>&1 | tail -1 | cut -c1-80 && sudo scripts/gen-dev-certs.sh 2>&1 | tail -1 | cut -c1-80; sudo openssl x509 -in configs/certs/dev/wotan/wotan.pem -noout -ext subjectAltName | tail -1 | cut -c1-90; sudo stat -c '%a %u' configs/certs/dev/wotan/wotan-key.pem; git status --short --untracked-files=no | wc -l
  ```
  Expect: SANs with `wotan`, `localhost`, `127.0.0.1`; `600 <wotan's uid>`; `0` (all gitignored). Else: a non-zero last number means a tracked file changed: stop and read it; never commit a key.
  **Rollback:** `sudo rm -rf configs/certs/dev`
- [ ] **Step 69** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 68-69: sysusers refreshed, make binaries, gen-dev-keys, gen-dev-certs; nothing generated is tracked.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 68-69: E1 prep: accounts, binaries, keys, certs generated' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 70** [STEVIE][W] ~2m: The two passwords `.env` lacks, without showing them. `well_backup` needs none.
  ```bash
  cd /home/govan/tmp/unheaded && for v in APP_AUTH_PASSWORD RETENTION_RUNNER_PASSWORD; do grep -q "^$v=" .env || { p="$(openssl rand -base64 24 | tr -d '/+=')"; printf '%s=%s\n' "$v" "$p" >> .env; printf '%s: %s\n' "$v" "$p" >> ~/tmp/secrets-2026-11.md; }; done; unset p; grep -c '^APP_AUTH_PASSWORD=\|^RETENTION_RUNNER_PASSWORD=' .env; stat -c '%a' .env
  ```
  Expect: `2`, `600`. Else: [STUCK].
- [ ] **Step 71** [CLAUDE][V] ~1m: Compose accepts both files, secrets and all.
  ```bash
  cd /home/govan/tmp/unheaded && docker compose config --quiet; echo main=$?; docker compose -f monitoring/docker-compose.yml config --quiet 2>/dev/null; echo mon=$?
  ```
  Expect: `main=0`, `mon=0`. Else: the first error names the missing variable or file.
- [ ] **Step 72** [CLAUDE][S] ~2m: **The Well up, alone.**
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d postgres 2>&1 | tail -1 | cut -c1-70; for i in $(seq 1 30); do s=$(docker inspect -f '{{.State.Health.Status}}' unheaded-postgres); [ "$s" = healthy ] && break; sleep 2; done; echo "$s"; docker inspect -f '{{range .Mounts}}{{.Name}} {{end}}' unheaded-postgres | tr ' ' '\n' | grep -c '^unheaded-dev_postgres-data$'
  ```
  Expect: `healthy`, `1`. Else: `docker logs --tail 20 unheaded-postgres`. **If the volume count is 0, stop: it created an empty Well.** `sudo docker compose stop postgres` and find out why.
  **Rollback:** `sudo docker compose stop postgres`
- [ ] **Step 73** [CLAUDE][V] ~2m: What is pending, dry.
  ```bash
  cd /home/govan/tmp/unheaded && set -a && . ./.env && set +a && WELL_MIGRATE_PASSWORD=$POSTGRES_PASSWORD WELL_MIGRATE_BIN=bin/release/well-migrate scripts/well/batch-apply.sh --dry-run --from 030 2>&1 | tail -18
  ```
  Expect: `[batch] pending (15):`, the fifteen entries from `030_ops_llm_requests.sql` to `043_ops_retention.sql` (042 twice), or 20 with `044` to `047` and `049` (all on develop since 2026-11-08), `[batch] dry run: nothing backed up, nothing applied`. Else: `refusing: pending file(s) numbered below 030` means the live ledger disagrees with the repo: stop, read it, do not apply. If `bin/release/well-migrate` is missing, unset `WELL_MIGRATE_BIN`.
- [ ] **Step 74** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 70-74: Evening 4, checked: What is pending, dry..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 70-74: Checkpoint in Evening 4: What is pending, dry.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 75** [CLAUDE][S] ~5m: **DP-605: apply**, behind the backup the script takes seconds before. `/srv/backups/unheaded-well` is `root:root`, so the script gets a directory of its own.
  ```bash
  sudo install -d -o govan -g govan -m 0700 /srv/backups/unheaded-well/pre-batch && cd /home/govan/tmp/unheaded && set -a && . ./.env && set +a && WELL_BATCH_BACKUP_ROOT=/srv/backups/unheaded-well/pre-batch WELL_MIGRATE_PASSWORD=$POSTGRES_PASSWORD WELL_MIGRATE_BIN=bin/release/well-migrate scripts/well/batch-apply.sh --from 030 2>&1 | tail -22
  ```
  Expect: `[batch] backing up into /srv/backups/unheaded-well/pre-batch/pre-batch-<time>`, the ledger rows, `[batch] done; status clean. Pre-batch backup: ...`. Copy that path to the progress file. Else: it stops at the first failure. **Do not re-run.** Use the printed rollback.
  **Rollback:** `bash scripts/well/batch-rollback.sh --yes /srv/backups/unheaded-well/pre-batch/pre-batch-<time>`
- [ ] **Step 76** [CLAUDE] ~1m: Passwords for the roles those migrations made. Values reach psql through the environment.
  ```bash
  cd /home/govan/tmp/unheaded && set -a && . ./.env && set +a && scripts/well/set-role-passwords.sh 2>&1 | tail -12 | cut -c1-70
  ```
  Expect: one line per role with a variable set, including `app_auth` and `retention_runner`. Else: [STUCK]; auth will not start without `app_auth`.
- [ ] **Step 77** [CLAUDE][V] ~3m: The ledger is clean and the live schema equals a fresh one.
  ```bash
  cd /home/govan/tmp/unheaded && set -o pipefail && set -a && . ./.env && set +a && WELL_MIGRATE_PASSWORD=$POSTGRES_PASSWORD bin/release/well-migrate -dir db/migrations status 2>&1 | tail -3 | cut -c1-80; echo rc=$?; scripts/well/test-converge-live-app.sh --live 2>&1 | tail -1 | cut -c1-90
  ```
  Expect: no `pending` lines; `rc=0`; a pass line. Else: the Rollback of Step 75, [STUCK].
- [ ] **Step 78** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 75-78: Well up alone, 15 migrations applied behind a pre-batch set, roles set, schema equals a fresh bootstrap. Pre-batch path in this file.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 75-78: DP-605: migrations 030-043 applied live' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 79** [V] ~1m: **EVENING 4 EXIT GATE** — only the Well runs, and tonight's backup will work.
  ```bash
  docker ps --format '{{.Names}}'; sudo systemctl reset-failed unheaded-well-backup.service; sudo systemctl start unheaded-well-backup.service; systemctl show -p Result --value unheaded-well-backup.service; for i in $(seq 1 60); do [ "$(sed -n 's/^set=//p' /srv/backups/unheaded-well/OFFSITE)" = "$(cat /srv/backups/unheaded-well/LATEST)" ] && break; sleep 5; done; sed -n 's/^set=//p' /srv/backups/unheaded-well/OFFSITE; cat /srv/backups/unheaded-well/LATEST
  ```
  Expect: `unheaded-postgres`, `success`, then the same set name twice (the offsite copy runs after the backup and takes minutes). Else: a failed offsite after a good backup: `journalctl -u unheaded-well-offsite -n 20`. Leaving the Well running is the right state either way.
- [ ] **Step 80** [C] ~6m: **MERGE (rule 15): Evening 4: E1 part one.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 4: E1 part one' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 4: E1 part one' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 5: E1 PART TWO: TLS, BOTH STACKS, MAIL, BACKUPS (Steps 81-117)

**Goal**: "running live with alerts". Both compose stacks as pinned (DP-19), every link TLS that can be (DP-23), alert-mailer sending from bellis.tech, watched (DP-710), FND-001 closed, Well backups green on both hosts, ADR-103 and ADR-106 accepted.
**Prerequisite**: Evening 4. The Well running.
**Time**: 4 h; the DNS step first, since it takes time to spread. **Have ready**: Google Workspace admin for bellis.tech; Namecheap DNS login; your phone or mail on stevie@bellis.tech.

### Mail: DNS first (your hands: DNS is Namecheap, there is no API on west)

- [ ] **Step 81** [STEVIE] ~10m: admin.google.com, Apps, Google Workspace, Gmail, Authenticate email: generate a 2048-bit DKIM key, selector `google`. In Namecheap, Advanced DNS, add TXT host `google._domainkey` with the value shown. Back in Google press **Start authentication** once it validates, which may take a while: go on and come back.
- [ ] **Step 82** [STEVIE] ~5m: On the Workspace account stevie@bellis.tech: 2-Step Verification on, then an app password at myaccount.google.com/apppasswords. Store it where it never touches history or chat; type it at the silent prompt.
  ```bash
  sudo bash -c 'umask 077; read -rs p && printf %s "$p" > /etc/unheaded/alert-mailer/smtp-password.new' && sudo chown 16710:16710 /etc/unheaded/alert-mailer/smtp-password.new && sudo chmod 0400 /etc/unheaded/alert-mailer/smtp-password.new && sudo mv /etc/unheaded/alert-mailer/smtp-password /etc/unheaded/alert-mailer/smtp-password.gmail && sudo mv /etc/unheaded/alert-mailer/smtp-password.new /etc/unheaded/alert-mailer/smtp-password && sudo stat -c '%a %u %n' /etc/unheaded/alert-mailer/smtp-password
  ```
  Expect: `400 16710 /etc/unheaded/alert-mailer/smtp-password`. Else: [STUCK].
  **Rollback:** `sudo mv /etc/unheaded/alert-mailer/smtp-password.gmail /etc/unheaded/alert-mailer/smtp-password`
- [ ] **Step 83** [CLAUDE][W] ~2m: `monitoring/.env`: sender and the heartbeat receiver.
  ```bash
  cd /home/govan/tmp/unheaded && sed -i 's/^SMTP_USERNAME=.*/SMTP_USERNAME=stevie@bellis.tech/; s/^ALERT_MAIL_FROM=.*/ALERT_MAIL_FROM=stevie@bellis.tech/; s/^ALERT_MAIL_TO=.*/ALERT_MAIL_TO=stevie@bellis.tech/' monitoring/.env && grep -q '^HEARTBEAT_HOSTS=' monitoring/.env || printf 'HEARTBEAT_HOSTS=east\nHEARTBEAT_LISTEN=192.168.13.2\n' >> monitoring/.env; grep -E '^(SMTP_SMARTHOST|SMTP_USERNAME|ALERT_MAIL_FROM|ALERT_MAIL_TO|HEARTBEAT_HOSTS|HEARTBEAT_LISTEN)=' monitoring/.env
  ```
  Expect: `smtp.gmail.com:587`, the three bellis.tech lines, `east`, `192.168.13.2`. These are not secrets. Else: fix the file.

### Images and host material

- [ ] **Step 84** [CLAUDE][B] ~40m: Build the images, two at a time so the box keeps its memory. Go back to DNS while it runs.
  ```bash
  cd /home/govan/tmp/unheaded && COMPOSE_PARALLEL_LIMIT=2 sudo -E docker compose build 2>&1 | tail -3 | cut -c1-90; sudo docker compose -f monitoring/docker-compose.yml build alert-mailer 2>&1 | tail -1 | cut -c1-90; free -g | awk '/Mem:/{print "avail", $7}'
  ```
  Expect: built lines, `avail 6` or more. Else: a build error: fix and rebuild that one service.
- [ ] **Step 85** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 81-85: Evening 5, checked: Build the images, two at a time so the box keeps its memory. Go back to DNS while it runs..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 81-85: Checkpoint in Evening 5: Build the images, two at a time so the box keeps its memory. Go back to DNS while it runs.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 86** [CLAUDE][S] ~5m: TLS and trust on the host, for units that dial Wotan. E1 uses the compose CA so that clients trust the servers they reach; Kingdom CA is a later job.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -d -m 0755 /etc/unheaded/tls /etc/unheaded/trust.d && sudo install -m 0644 configs/certs/dev/ca.pem /etc/unheaded/tls/ca.pem && for m in chaos-controller:chaos-controller kev-poller:kev-poller trace-collector:trace-collector baseline:unheaded-baseline; do u=${m%%:*}; i=${m##*:}; o=$(sed -n 's/^User=//p' deploy/systemd/unheaded-$u.service | head -1); sudo bin/release/unheaded-keygen -dir /etc/unheaded/keys -trust-dir configs/trust.d/dev -owner "${o:-root}" "$i" 2>&1 | tail -1 | cut -c1-70; done; sudo chmod 0711 /etc/unheaded/keys; sudo cp -p configs/trust.d/dev/*.pub /etc/unheaded/trust.d/; ls configs/trust.d/dev | wc -l; sudo ls /etc/unheaded/trust.d | wc -l
  ```
  Expect: a line per identity, two equal counts. Else: [STUCK] on host units; the stack can still come up, since these four only dial it. A public key added to `configs/trust.d/dev` needs Wotan to read it, so this comes before Wotan starts.
  **Rollback:** `sudo rm -rf /etc/unheaded/tls /etc/unheaded/trust.d`; `sudo rm -f /etc/unheaded/keys/{chaos-controller,kev-poller,trace-collector,unheaded-baseline}.{key,pub}`
- [ ] **Step 87** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 86-87: DKIM and DMARC requested, Workspace app password in place, images built, host CA and trust.d installed.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 86-87: E1: mail settings, images, host TLS material' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.

### Bring-up, in order. `kingdom-startup.sh` is not used: it would start everything at once.

- [ ] **Step 88** [CLAUDE][S] ~5m: Monitoring first, then silence the warm-up.
  ```bash
  cd /home/govan/tmp/unheaded/monitoring && sudo docker compose up -d 2>&1 | tail -2 | cut -c1-70; for i in $(seq 1 30); do curl -fsS --max-time 2 http://127.0.0.1:9093/-/ready >/dev/null 2>&1 && break; sleep 2; done; cd .. && scripts/monitoring/maintenance.sh 180m "E1 bring-up" 2>&1 | tail -1 | cut -c1-80; docker compose -f monitoring/docker-compose.yml ps --format '{{.Name}} {{.Status}}' | cut -c1-60
  ```
  Expect: Prometheus, Alertmanager, alert-mailer, Grafana, Loki, VictoriaMetrics `Up`. Else: `docker logs --tail 20 <name>`. alert-mailer exits with `mode send needs the relay settings` if `monitoring/.env` is wrong.
  **Rollback:** `cd monitoring && sudo docker compose down` (volumes kept)
- [ ] **Step 89** [CLAUDE][S] ~5m: Infrastructure of the main stack.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d traefik victoria clickhouse clickhouse-schema vector grafana coredns 2>&1 | tail -2 | cut -c1-70; sleep 30; docker compose ps --format '{{.Name}} {{.Status}}' | cut -c1-60
  ```
  Expect: all `Up`, `clickhouse-schema` exited 0, `postgres` `Up (healthy)`. Else: logs of that one; do not go on until `clickhouse` is healthy.
- [ ] **Step 90** [CLAUDE][S] ~3m: Wotan, TLS.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d wotan 2>&1 | tail -1 | cut -c1-70; sleep 10; curl -fsS --cacert configs/certs/dev/ca.pem https://localhost:18000/health | cut -c1-100; openssl s_client -connect 127.0.0.1:18001 -groups X25519MLKEM768 -CAfile configs/certs/dev/ca.pem </dev/null 2>&1 | grep -E 'Negotiated TLS1.3 group|Verify return code: 0' | head -2; curl -s -m3 -o /dev/null -w '%{http_code}\n' http://localhost:18000/health
  ```
  Expect: a health body; `Negotiated TLS1.3 group: X25519MLKEM768`; `Verify return code: 0 (ok)`; `000` or a refusal on plain http. Else: `docker logs --tail 30 unheaded-wotan`; [STUCK] stops the stack here: `sudo docker compose stop wotan`.
- [ ] **Step 91** [CLAUDE][S] ~3m: Auth before its dependants; then timeguru and kanban-app together (kanban's timeline call carries a token).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d auth 2>&1 | tail -1 | cut -c1-60; sleep 8; sudo docker compose up -d timeguru kanban-app 2>&1 | tail -2 | cut -c1-60; sleep 10; for p in 19000 20001; do curl -fsS -o /dev/null -w "$p %{http_code}\n" --cacert configs/certs/dev/ca.pem https://localhost:$p/health; done
  ```
  Expect: `19000 200`, `20001 200`. Else: kanban-app refuses an unmigrated Well: `docker logs --tail 20 unheaded-kanban`.
- [ ] **Step 92** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 88-92: Evening 5, checked: Auth before its dependants then timeguru and kanban-app together kanbans timeline call car.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 88-92: Checkpoint in Evening 5: Auth before its dependants then timeguru and kanban-app together kanbans timeline call car' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 93** [CLAUDE][S] ~5m: The rest. Sword stays off: it is in the `deploy` profile, as pinned, and needs `/etc/unheaded/sword/sword.env`.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d captain architect micromanager akira monad sophia dashboard-backend cuirass 2>&1 | tail -3 | cut -c1-60; sleep 20; for p in 19000 19001 19002 19003 19006 19100 20000 20001; do curl -s -o /dev/null -m4 -w "$p %{http_code}\n" --cacert configs/certs/dev/ca.pem https://localhost:$p/health; done; docker compose ps --status exited --format '{{.Name}} {{.Service}}' | grep -v -E 'volume-init|clickhouse-schema' | wc -l; free -g | awk '/Mem:/{print "avail", $7}'
  ```
  Expect: eight `200`s, `0`, `avail 2` or more. Else: a service down: its logs; two tries, then `sudo docker compose stop <service>`, [STUCK], go on. Under 2 GiB available: stop the monitoring Grafana and Loki and tell Claude: that is DP-19 option B, and the plan stops to ask.
- [ ] **Step 94** [STEVIE][V] ~5m: Log in. `https://192.168.69.184:20000`. A warning about the dev CA is expected. User `dev`, password from your secrets file. **Expect:** the dashboard renders with live events. The BFF calls the issuer server-side, so `auth` need not be published. **Else:** `docker logs --tail 20 unheaded-dashboard`; two tries, then [STUCK]: the stack is fine, the browser leg needs a fix.
- [ ] **Step 95** [CLAUDE][S] ~3m: Apply the rotated Grafana admin passwords to the existing databases: the env value only works on an empty one. The value goes over stdin.
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 's/^GRAFANA_ADMIN_PASSWORD=//p' .env | sudo docker exec -i unheaded-grafana grafana cli admin reset-admin-password --password-from-stdin 2>&1 | tail -1 | cut -c1-70; sed -n 's/^GRAFANA_ADMIN_PASSWORD=//p' monitoring/.env | sudo docker exec -i unheaded-monitoring-grafana grafana cli admin reset-admin-password --password-from-stdin 2>&1 | tail -1 | cut -c1-70
  ```
  Expect: `Admin password changed successfully` twice. Else: [STUCK]; the old password still works, DP-01 stays open for this row.
- [ ] **Step 96** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 93-96: Monitoring, then the main stack in order. Wotan negotiates X25519MLKEM768. All first-party /health 200 over TLS.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 93-96: E1: both stacks up on TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.

### Hosts, scrapes, east

- [ ] **Step 97** [CLAUDE][S] ~10m: West's host units: new binaries and units, one at a time. West's chaos, kev-poller and trace-collector now dial TLS Wotan.
  ```bash
  cd /home/govan/tmp/unheaded && for u in kev-poller:kev-poller chaos-controller:chaos-controller trace-collector:trace-collector-go; do s=${u%%:*}; b=${u##*:}; sudo cp -p /etc/systemd/system/unheaded-$s.service /var/backups/unheaded-$s.service.bak-e1; sudo cp -p /usr/local/bin/$b /var/backups/$b.bak-e1; sudo install -m 0755 bin/release/$b /usr/local/bin/$b; sudo install -m 0644 deploy/systemd/unheaded-$s.service /etc/systemd/system/; done; sudo systemctl daemon-reload; for s in kev-poller chaos-controller trace-collector; do sudo systemctl restart unheaded-$s; sleep 3; echo "$s $(systemctl is-active unheaded-$s)"; done
  ```
  Expect: three `active`. Else: Rollback for that unit, [STUCK].
  **Rollback:** `sudo cp -p /var/backups/unheaded-<unit>.service.bak-e1 /etc/systemd/system/unheaded-<unit>.service && sudo cp -p /var/backups/<binary>.bak-e1 /usr/local/bin/<binary> && sudo systemctl daemon-reload && sudo systemctl restart unheaded-<unit>`
- [ ] **Step 98** [CLAUDE][V] ~3m: Which scrape targets are down.
  ```bash
  curl -s 'http://127.0.0.1:9090/api/v1/targets?state=active' | python3 -c "import sys,json; [print(t['labels']['job'], t['scrapeUrl'], t['lastError'][:60]) for t in json.load(sys.stdin)['data']['activeTargets'] if t['health']!='up']"; curl -s 'http://127.0.0.1:9090/api/v1/query?query=slo:error_ratio:5m' | python3 -c "import sys,json; print('slo series', len(json.load(sys.stdin)['data']['result']))"
  ```
  Expect: only jobs that are not running yet or have no listener: `doom_services`, `heimdall`, `zhen-agentd`, `frr_exporter`, `bird_exporter`, `routing_health`; `slo series` above 0 within a few minutes. Else: a first-party target with a TLS error: check `server_name` and the `/etc/prometheus-tls/ca.pem` mount.
- [ ] **Step 99** [STEVIE][DECIDE] ~3m: **Your word needed.** East's `unheaded-wotan` has crash-looped for weeks (100,212 restarts) on a flag the 1.0.0 binary refuses, and `unheaded-timeguru` dies on `mkdir data`; that loop is what fills east's journal. Both are fixed by the debs in Evening 10B. **RECOMMENDATION: stop them until then.** East's other first-party units stay as they are, plaintext and unable to reach TLS Wotan, so **FND-017 stays open for east until Evening 10B**: the "all internal links" of DP-23 is true for west tonight and for both after 10B. Override ONLY IF you want the debs moved up to tonight.
  ```bash
  ssh east 'sudo systemctl disable --now unheaded-wotan unheaded-timeguru; systemctl is-active unheaded-wotan unheaded-timeguru; sudo systemctl reset-failed'
  ```
  Expect: `inactive` twice. Else: [STUCK].
  **Rollback:** `ssh east 'sudo systemctl enable --now unheaded-wotan unheaded-timeguru'`
- [ ] **Step 100** [CLAUDE][V] ~3m: East's heartbeat reaches the mailer.
  ```bash
  cd /home/govan/tmp/unheaded/monitoring && sudo docker compose up -d alert-mailer 2>&1 | tail -1 | cut -c1-60; ss -tln | grep -c '192.168.13.2:19131 '; ssh east 'sudo systemctl reset-failed unheaded-heartbeat.service; sudo systemctl start unheaded-heartbeat.service; systemctl show -p Result --value unheaded-heartbeat.service'; sleep 5; curl -s 127.0.0.1:19130/metrics | grep -c 'heartbeat'
  ```
  Expect: `1`, `success`, a count above 0. Else: `401` in east's journal means the two tokens differ: Evening 3's token step.
- [ ] **Step 101** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 97-101: West host units restarted on TLS; scrape targets reviewed; east wotan and timeguru stopped by word; heartbeat success.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 97-101: E1: host units on TLS, east crash loops stopped, heartbeat live' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.

### DP-710: outbound, proved and watched

- [ ] **Step 102** [STEVIE] ~3m: DKIM has validated in Google (DMARC with `adkim=s` before that would quarantine your own mail). In Namecheap change TXT host `_dmarc` to (DP-710's option A; `p=reject` after two clean weeks of reports):
  ```
  v=DMARC1; p=quarantine; sp=quarantine; adkim=s; aspf=s; pct=100; rua=mailto:dmarc-reports@bellis.tech
  ```
  SPF stays as it is: `v=spf1 include:_spf.google.com -all`.
  **Rollback:** put back `v=DMARC1; p=none; pct=100; rua=mailto:dmarc-reports@bellis.tech`
- [ ] **Step 103** [CLAUDE][V] ~2m: Send-only: nothing listens for mail anywhere.
  ```bash
  ss -tln | grep -cE ':(25|465|587|2525) '; docker port unheaded-alert-mailer | sort; curl -s 127.0.0.1:19130/v1/status | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['mode'], d['max_per_day'])"; dig +short TXT _dmarc.bellis.tech; dig +short TXT google._domainkey.bellis.tech | cut -c1-30
  ```
  Expect: `0`; only `19130/tcp -> 127.0.0.1:19130` and `19131/tcp -> 192.168.13.2:19131`; `send 6`; the `p=quarantine` record; a `v=DKIM1` start. Else: DKIM not visible yet: finish the rest, re-check, and do not close FND-001 until the headers pass.
- [ ] **Step 104** [CLAUDE][V] ~75m: **FND-001 for real.** A real alert end to end, without hurting anything: stop east's heartbeat for eleven minutes. Prometheus fires `HeartbeatMissing`, Alertmanager routes the page, alert-mailer sends. First wait: the burn windows run an hour, and ending the silence during warm-up could spend the 3-an-hour cap on false pages. An hour after the last service came up, check nothing would page. For this one mail the recipient is outside the domain, because Google's internal delivery may not show SPF, DKIM and DMARC.
  ```bash
  curl -s 'http://127.0.0.1:9090/api/v1/query?query=ALERTS{alertstate="firing",urgency="page"}' | python3 -c "import sys,json; print('pages firing', len(json.load(sys.stdin)['data']['result']))"
  ```
  Expect: `pages firing 0`. Else: wait, or read what is burning; do not go on.
  ```bash
  cd /home/govan/tmp/unheaded && sed -i 's/^ALERT_MAIL_TO=.*/ALERT_MAIL_TO=stevenrbellis@gmail.com/' monitoring/.env && (cd monitoring && sudo docker compose up -d alert-mailer 2>&1 | tail -1 | cut -c1-60); scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-70; ssh east 'sudo systemctl stop unheaded-heartbeat.timer'; date -u +%H:%M
  ```
  Then wait 15 minutes: `curl -s 127.0.0.1:19130/v1/outbox | python3 -c "import sys,json; [print(e['time'][:16], e['urgency'], e['outcome'], e['subject'][:50]) for e in json.load(sys.stdin)['outbox'][-3:]]"`.
  Expect: a line `page sent ...HeartbeatMissing...`. Else: `capped` means it hit a cap: read `/v1/status`. No line after 20 minutes: `curl -s 127.0.0.1:9090/api/v1/alerts | grep -c HeartbeatMissing`, then the Alertmanager UI through `ssh -L 9093:127.0.0.1:9093`.
  **Rollback:** `ssh east 'sudo systemctl start unheaded-heartbeat.timer'`
- [ ] **Step 105** [STEVIE][V] ~3m: The mail reached your Gmail inbox. Open the original (`Show original`). **Expect:** From stevie@bellis.tech; `SPF: PASS`, `DKIM: PASS` with domain `bellis.tech`, `DMARC: PASS`. **Else:** DKIM pending: wait for Google, then send one more page tomorrow; note that FND-001 stays open until it passes. In spam: mark it not spam and note it.
- [ ] **Step 106** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 102-106: Evening 5, checked: The mail reached your Gmail inbox. Open the original Show original. Expect: From steviebel.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 102-106: Checkpoint in Evening 5: The mail reached your Gmail inbox. Open the original Show original. Expect: From steviebel' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 107** [CLAUDE][V] ~2m: Every send is logged. Then bring the heartbeat back and see the all-clear.
  ```bash
  ssh east 'sudo systemctl start unheaded-heartbeat.timer; systemctl is-active unheaded-heartbeat.timer'; sleep 420; cd /home/govan/tmp/unheaded && sed -i 's/^ALERT_MAIL_TO=.*/ALERT_MAIL_TO=stevie@bellis.tech/' monitoring/.env && (cd monitoring && sudo docker compose up -d alert-mailer 2>&1 | tail -1 | cut -c1-60); docker logs unheaded-alert-mailer 2>&1 | grep -c '"message":"alert-mail"'; curl -s 127.0.0.1:19130/metrics | grep 'unheaded_alert_mailer_messages_total' | head -3 | cut -c1-80
  ```
  Expect: `active`, a count (recreating the container restarts its log, so `0` here with the outbox line above is fine; `journalctl` does not hold these, `docker logs` does), counters with `outcome="sent"`. Else: [STUCK]. The recipient is back on stevie@bellis.tech.
- [ ] **Step 108** [STEVIE] ~3m: Revoke the old Gmail app password at myaccount.google.com/apppasswords on stevenrbellis@gmail.com, then shred the copy. A session transcript holds its start (Evening 2).
  ```bash
  sudo shred -u /etc/unheaded/alert-mailer/smtp-password.gmail; sudo ls /etc/unheaded/alert-mailer
  ```
  Expect: `heartbeat-token smtp-password`. Else: [STUCK].
- [ ] **Step 109** [CLAUDE][W] ~5m: FND-001 closes. In `compliance/findings/register.yaml`, under `id: FND-001`, before `decision:`, add (date and evidence as they happened):
  ```yaml
      resolved: {on: <date>, evidence: "real HeartbeatMissing page through Prometheus, Alertmanager and alert-mailer to stevie@bellis.tech, SPF, DKIM and DMARC pass; DP-710 caps and send log live"}
  ```
  `wiki/Alerting.md`: the `**Mode is send**` paragraph: sender is now the Workspace account; add the hourly cap. `python3 -c "import yaml; yaml.safe_load(open('compliance/findings/register.yaml'))"`. Expect: silence. Else: fix the line.

### Backups, retention, acceptance

- [ ] **Step 110** [CLAUDE][S] ~5m: **Well backups green on both hosts.**
  ```bash
  sudo systemctl start unheaded-well-backup.service; systemctl show -p Result --value unheaded-well-backup.service; for i in $(seq 1 60); do [ "$(sed -n 's/^set=//p' /srv/backups/unheaded-well/OFFSITE)" = "$(cat /srv/backups/unheaded-well/LATEST)" ] && break; sleep 5; done; sed -n 's/^set=//p' /srv/backups/unheaded-well/OFFSITE; cat /srv/backups/unheaded-well/LATEST; ssh east 'sudo systemctl reset-failed unheaded-well-offsite-check.service; sudo systemctl start unheaded-well-offsite-check.service; systemctl show -p Result --value unheaded-well-offsite-check.service; sudo cat /var/backups/unheaded-well-west/sets/LATEST'
  ```
  Expect: `success`, the same set named for today twice, `success`, that set again. Else: `journalctl -u <unit> -n 20`; [STUCK]. E1 is not "done" until this is green.
- [ ] **Step 111** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 107-111: Evening 5, checked: Every send is logged. Then bring the heartbeat back and see the all-clear..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 107-111: Checkpoint in Evening 5: Every send is logged. Then bring the heartbeat back and see the all-clear.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 112** [CLAUDE][S] ~8m: Retention, now that 042-043 are live and the policy is ratified. The timer is not installed on west.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -d -m 0755 /etc/unheaded/well && sudo install -m 0644 compliance/retention/classes.yaml /etc/unheaded/well/retention-classes.yaml && sed -n 's/^RETENTION_RUNNER_PASSWORD=//p' .env | tr -d '\n' | sudo install -m 0400 -o root -g root /dev/stdin /etc/unheaded/well/retention.password && sudo install -m 0755 bin/release/well-retention /usr/local/bin/well-retention && sudo install -m 0644 deploy/systemd/unheaded-well-retention.service deploy/systemd/unheaded-well-retention.timer /etc/systemd/system/ && sudo systemctl daemon-reload && sudo setfacl -m u:unheaded-well-retention:rwx /var/lib/unheaded/textfile; sudo systemd-run --wait --pipe --uid=unheaded-well-retention -p LoadCredential=well-password:/etc/unheaded/well/retention.password /usr/local/bin/well-retention -classes /etc/unheaded/well/retention-classes.yaml -dry-run 2>&1 | tail -4 | cut -c1-90
  ```
  Expect: per-class counts, nothing deleted. Read them: are the purge numbers what you expect? Else: [STUCK]; retention can wait. `setfacl` missing means the `acl` package: tell Claude, do not install on your own.
  **Rollback:** `sudo rm -f /etc/systemd/system/unheaded-well-retention.{service,timer} && sudo systemctl daemon-reload`
- [ ] **Step 113** [STEVIE] ~1m: The dry run looks right: enable the weekly timer.
  ```bash
  sudo systemctl enable --now unheaded-well-retention.timer; systemctl is-enabled unheaded-well-retention.timer
  ```
  Expect: `enabled`. Else: [STUCK].
  **Rollback:** `sudo systemctl disable --now unheaded-well-retention.timer`
- [ ] **Step 114** [STEVIE][W] ~5m: **Accept ADR-103 and ADR-106** (DP-603). Your word; Claude edits. ADR-106's first night comes later; DP-603 set both at E1. Line 8 of each:
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && D=$(date -u +%F) && sed -i "8s/^\*\*Status:\*\* Implemented in code,/**Status:** Accepted $D at E1 (DP-603). Implemented in code,/" docs/adr/ADR-103-service-auth-token-distribution.md && sed -i "8s/^\*\*Status:\*\* Proposed (Stevie to accept)\./**Status:** Accepted $D at E1 (DP-603); first unattended night follows (DP-303)./" docs/adr/ADR-106-two-brains-one-gate.md && sed -n 8p docs/adr/ADR-103-service-auth-token-distribution.md | cut -c1-60; sed -n 8p docs/adr/ADR-106-two-brains-one-gate.md | cut -c1-60; grep -nE '^\| \[(103|106)\]' docs/adr/ADR-INDEX.md | cut -c1-60
  ```
  Expect: both lines begin `**Status:** Accepted`. Then in `docs/adr/ADR-INDEX.md` and `wiki/ADR-Index.md` set those two rows' status to Accepted. Else: the line number moved: find `**Status:**` and do the same by hand.
- [ ] **Step 115** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 112-115: FND-001 resolved by a real page; backups green on west and east; retention dry run read and timer on; ADR-103 and ADR-106 accepted.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md compliance/findings/register.yaml wiki/Alerting.md docs/adr/ADR-103-service-auth-token-distribution.md docs/adr/ADR-106-two-brains-one-gate.md docs/adr/ADR-INDEX.md wiki/ADR-Index.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 112-115: FND-001 closed, retention live, ADR-103 and ADR-106 accepted' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 116** [V] ~3m: **EVENING 5 EXIT GATE: "running live with alerts".**
  ```bash
  cd /home/govan/tmp/unheaded && scripts/kingdom-startup.sh --dry-run --docker-only 2>&1 | tail -3 | cut -c1-70; scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-60; docker compose ps --format '{{.Status}}' | grep -vc '^Up\|^Exited (0)'; docker compose -f monitoring/docker-compose.yml ps --format '{{.Status}}' | grep -vc '^Up\|^Exited (0)'; systemctl --failed --no-legend | wc -l; ssh east 'systemctl --failed --no-legend | wc -l'; free -g | awk '/Mem:/{print "avail", $7}'
  ```
  Expect: `Kingdom Startup Complete` (its dry run still makes a real 15-minute silence, ended on the next line), `0`, `0`, `0`, `0`, an availability you are happy with. Else: name each exception in the progress file. Start the clock for DP-03: a stable week from tonight. Watch `free` tomorrow: 14 GiB against about 10 GiB of ceilings.
- [ ] **Step 117** [C] ~6m: **MERGE (rule 15): Evening 5: E1 live with alerts.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 5: E1 live with alerts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 5: E1 live with alerts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 6: EAST DOOR (Steps 118-135)

**Goal**: DP-22 on east with commit-confirm, then east's sshd to 7375 and the ruleset line with it, the client side fixed so offsite backups do not break.
**Prerequisite**: Evening 5. S-HOST-TIDY Phases 8-9 done (password on, fail2ban lists 7375).
**Time**: 1.5 h. **Have ready**: be at the desk, east's keyboard and monitor in reach. Second terminal open on east as well as on west.

Why it is safe: `ip saddr 192.168.13.2 accept` lets west in on any port, so the firewall does not depend on which port sshd uses; each apply rolls back by itself; each sshd stage has an undo timer.

- [ ] **Step 118** [STEVIE][V] ~3m: **First, while nothing has moved:** Appendix A1's console path needs east's password. From west: `ssh -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive east true`, and type it. **Expect:** it logs in. **Else:** if the password fails you may not know east's `govan` password, last set 2026-02-28: set one at this session with `sudo passwd govan`.
- [ ] **Step 119** [CLAUDE][B] ~2m: Silence, hold session, baseline of what must survive.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 90m "east firewall and ssh port" 2>&1 | tail -1 | cut -c1-70; ssh -fNM -o ServerAliveInterval=30 -S /run/user/1000/e1-east.sock east; ssh -S /run/user/1000/e1-east.sock -O check east 2>&1; sudo wg show wg0 latest-handshakes | wc -l; curl -s -m3 'http://[fd00:dead:beef::2]:9100/metrics' | grep -c '^node_uname_info'; ssh east 'sudo birdc show protocols 2>/dev/null | grep -c Established'
  ```
  Expect: a silence line, `Master running`, `1`, `1`, `1` or more. Else: an already failing link is not tonight's to fix: stop.
- [ ] **Step 120** [CLAUDE][S] ~2m: Script and ruleset onto east at stable paths: the rollback timer calls the script by path.
  ```bash
  cd /home/govan/tmp/unheaded && scp -q scripts/host/nft-apply-confirm.sh deploy/nftables/east.nft deploy/systemd/unheaded-nft.service east:/tmp/ && ssh -S /run/user/1000/e1-east.sock east 'sudo install -D -m 0755 /tmp/nft-apply-confirm.sh /usr/local/lib/unheaded/nft-apply-confirm.sh && sudo install -D -m 0644 /tmp/east.nft /usr/local/lib/unheaded/nft/east.nft && sudo nft -c -f /usr/local/lib/unheaded/nft/east.nft && grep -c "tcp dport 7375" /usr/local/lib/unheaded/nft/east.nft'
  ```
  Expect: `1`. Else: [STUCK], nothing is live.
- [ ] **Step 121** [CLAUDE][S] ~1m: **Apply, 10 minutes to confirm.**
  ```bash
  ssh -S /run/user/1000/e1-east.sock east 'sudo /usr/local/lib/unheaded/nft-apply-confirm.sh apply /usr/local/lib/unheaded/nft/east.nft 600'
  ```
  Expect: `nft-apply-confirm: applied ...; rolls back in 600s unless you run: ... confirm`. Else: `nothing applied`: read why.
  **Rollback:** automatic at 600 s, or now: `ssh -S /run/user/1000/e1-east.sock east 'sudo /usr/local/lib/unheaded/nft-apply-confirm.sh rollback'`
- [ ] **Step 122** [CLAUDE][V] ~2m: **Inside the window**, from west: new login, overlay, scrape, BGP, chaos guard, services.
  ```bash
  ssh -o BatchMode=yes -o ControlPath=none -o ConnectTimeout=8 east 'echo FRESH_KEY_LOGIN_OK; sudo nft list chain inet unheaded input | grep -c "policy drop"'; ping -6 -c2 -W1 fd00:dead:beef::2 | tail -2 | head -1 | cut -c1-50; curl -s -m3 'http://[fd00:dead:beef::2]:9100/metrics' | grep -c '^node_uname_info'; curl -s -m3 http://192.168.13.1:9110/metrics | grep -c . ; ssh east 'sudo birdc show protocols | grep -c Established; sudo systemctl start unheaded-heartbeat.service; systemctl show -p Result --value unheaded-heartbeat.service'
  ```
  Expect: `FRESH_KEY_LOGIN_OK`, `1`, `2 packets transmitted, 2 received`, `1`, a number above 0, `1` or more, `success`. Else: do nothing: it rolls back by itself. Write down what failed.
- [ ] **Step 123** [CLAUDE][S] ~1m: Confirm, and install the boot unit, or the firewall vanishes at the next reboot.
  ```bash
  ssh -S /run/user/1000/e1-east.sock east 'sudo /usr/local/lib/unheaded/nft-apply-confirm.sh confirm && sudo install -m 0644 /tmp/unheaded-nft.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable unheaded-nft.service 2>&1 | tail -1 | cut -c1-60; sudo systemctl start unheaded-nft.service; systemctl is-active unheaded-nft.service; sudo nft list tables | grep -c "inet unheaded$"'
  ```
  Expect: `nft-apply-confirm: confirmed; ...`, `active`, `1`. Else: a failed unit start leaves the ruleset in place; `journalctl -u unheaded-nft -n 10`, [STUCK], the host keeps its firewall until reboot.
  **Rollback:** `ssh east 'sudo systemctl disable unheaded-nft.service; sudo systemctl stop unheaded-nft.service'` (`stop` removes the table)
- [ ] **Step 124** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 118-124: East input policy drop through commit-confirm; login, overlay, scrape, BGP, heartbeat proven in the window.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 118-124: DP-22 east: default deny live, boot unit enabled' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 125** [CLAUDE][S] ~1m: **Port, stage 1: listen on both.** Undo timer first.
  ```bash
  ssh -S /run/user/1000/e1-east.sock east "sudo systemd-run --unit=unheaded-e1-port-undo --on-active=1800 --timer-property=AccuracySec=1s /bin/sh -c 'rm -f /etc/ssh/sshd_config.d/05-unheaded-port.conf; systemctl daemon-reload; systemctl restart ssh.socket' 2>&1 | head -1; printf '%s\n' '# DP-11: sshd on 7375, as on west.' 'Port 22' 'Port 7375' | sudo install -m 0600 -o root -g root /dev/stdin /etc/ssh/sshd_config.d/05-unheaded-port.conf && sudo sshd -t && sudo systemctl daemon-reload && sudo systemctl restart ssh.socket && sleep 2; ss -tln | grep -cE ':(22|7375) '"
  ```
  Expect: `Running timer as unit: unheaded-e1-port-undo.timer`, `4`. Else: the hold session may have dropped: in your second terminal on east run the Rollback. The timer does it at 30 minutes otherwise.
  **Rollback:** `sudo rm -f /etc/ssh/sshd_config.d/05-unheaded-port.conf; sudo systemctl daemon-reload; sudo systemctl restart ssh.socket; sudo systemctl stop unheaded-e1-port-undo.timer`
- [ ] **Step 126** [CLAUDE][V] ~1m: 7375 answers with the same host key, and the hold session survived the socket restart.
  ```bash
  ssh -p 7375 -o BatchMode=yes -o ControlPath=none -o HostKeyAlias=192.168.13.1 east 'echo FRESH_7375_OK'; ssh -S /run/user/1000/e1-east.sock -O check east 2>&1
  ```
  Expect: `FRESH_7375_OK`, `Master running`. Else: a host-key complaint: `ssh-keyscan -p 7375 192.168.13.1` and compare; do not accept a different key. Hold session gone: reopen it now.
- [ ] **Step 127** [CLAUDE][S] ~2m: West's clients, all at once: `~/.ssh/config`, root's units and `offsite-copy.sh` all read this file.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -m 0644 deploy/ssh/50-unheaded-east.conf /etc/ssh/ssh_config.d/50-unheaded-east.conf && ssh -G east | grep -E '^(port|hostkeyalias) '; sudo ssh -G 192.168.13.1 | grep -E '^port '; ssh -o BatchMode=yes -o ControlPath=none east 'echo FRESH_EAST_OK'
  ```
  Expect: `port 7375`, `hostkeyalias 192.168.13.1`, `port 7375`, `FRESH_EAST_OK`. Else: Rollback, then the stage 1 Rollback.
  **Rollback:** `sudo rm -f /etc/ssh/ssh_config.d/50-unheaded-east.conf`
- [ ] **Step 128** [CLAUDE][V] ~5m: **Every consumer**: the offsite copy is the one that would have broken at 03:48.
  ```bash
  sudo systemctl start unheaded-well-offsite.service; systemctl show -p Result --value unheaded-well-offsite.service; systemctl --user start unheaded-git-mirror.service; systemctl --user show -p Result --value unheaded-git-mirror.service; cd /home/govan/tmp/unheaded && sudo scripts/wireguard/rotate-psk.sh 2>&1 | tail -2 | cut -c1-80
  ```
  Expect: `success`, `success`, and the rotate dry run ends with its plan, naming `post-quantum hybrid`. Else: stop here with both ports open: harmless, and the timer must be stopped, or it strands the client file: `ssh east 'sudo systemctl stop unheaded-e1-port-undo.timer'`. Fix the consumer another evening.
- [ ] **Step 129** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 125-129: Evening 6, checked: Every consumer: the offsite copy is the one that would have broken at 03:48..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 125-129: Checkpoint in Evening 6: Every consumer: the offsite copy is the one that would have broken at 03:48.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 130** [CLAUDE][S] ~2m: **Stage 2: 7375 only.** The 30-minute timer from stage 1 is still armed. If it would fire before you finish, the client file must come off too: `sudo rm /etc/ssh/ssh_config.d/50-unheaded-east.conf`.
  ```bash
  ssh -S /run/user/1000/e1-east.sock east "printf '%s\n' '# DP-11: sshd on 7375, as on west.' 'Port 7375' | sudo install -m 0600 -o root -g root /dev/stdin /etc/ssh/sshd_config.d/05-unheaded-port.conf && sudo sshd -t && sudo systemctl daemon-reload && sudo systemctl restart ssh.socket && sleep 2; ss -tln | grep -cE ':22 '; ss -tln | grep -cE ':7375 '"; ssh -o BatchMode=yes -o ControlPath=none east 'echo FRESH_EAST_OK; sudo sshd -T | grep ^port'
  ```
  Expect: `0`, `2`, `FRESH_EAST_OK`, `port 7375`. Else: the timer restores port 22 by removing the file; your client file then needs removing too.
- [ ] **Step 131** [CLAUDE][S] ~1m: Stop the timer. The move stays.
  ```bash
  ssh east 'sudo systemctl stop unheaded-e1-port-undo.timer; systemctl is-active unheaded-e1-port-undo.timer; sudo fail2ban-client get sshd action nftables port'; ssh -S /run/user/1000/e1-east.sock -O exit east 2>&1
  ```
  Expect: `inactive`, `22,7375`, `Exit request sent.`. Else: [STUCK].
- [ ] **Step 132** [CLAUDE][W] ~5m: Docs and the east comment. `deploy/systemd/unheaded-well-offsite.service` line 11: `ssh-keyscan -t ed25519 192.168.13.1` → `ssh-keyscan -p 7375 -t ed25519 192.168.13.1 | sed 's/^\[192.168.13.1\]:7375/192.168.13.1/'`. `wiki/Security.md` S-HOST-TIDY paragraph: `on port 22 (the move to 7375 is attended, S-E1-LIVE)` → `on port 7375`. **Hosts today** line at `wiki/Security.md:31`: east default deny since this date. `compliance/findings/register.yaml`: `host-probe:east:firewall-inbound-deny` gets `resolved: {on: <date>, evidence: "inet unheaded input policy drop, confirmed ruleset, unheaded-nft enabled"}`.
- [ ] **Step 133** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 130-133: East 7375 only; client file on west; offsite, git mirror and rotate dry run proven; password path tested first.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md deploy/systemd/unheaded-well-offsite.service wiki/Security.md compliance/findings/register.yaml && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 130-133: DP-11: east sshd on 7375, clients moved, consumers proven' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 134** [V] ~1m: **EVENING 6 EXIT GATE**.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-60; systemctl show -p Result --value unheaded-well-offsite.service
  ```
  Expect: `success`. Else: [STUCK]. Tomorrow morning check that the 03:48 backup and offsite ran.
- [ ] **Step 135** [C] ~6m: **MERGE (rule 15): Evening 6: east door.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 6: east door' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 6: east door' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 7: WEST FIREWALL (Steps 136-150)

**Goal**: DP-22 on west, after E1 is live: default deny with the LAN scope you set on 2026-11-03.
**Prerequisite**: Evening 5 live and quiet for at least two days.
**Time**: 1.5 h. **Have ready**: at the desk; laptop and phone on the LAN; west's console in reach. West is the only path to east: this one has the widest blast.

- [ ] **Step 136** [CLAUDE][V] ~3m: Baseline from west: what listens beyond loopback, so nothing surprises. Published ports (`3000`, `3001`, `5353`, `8428`, `18000`, `18001`, `19000-19100`, `20000`, `20001`, `50051`) skip the input chain; the forward-chain rule from Evening 0 scopes them.
  ```bash
  ss -tln | awk 'NR>1{print $4}' | grep -vE '^(127\.|\[::1\])' | sort -u | tr '\n' ' '; echo; grep -c 'tcp dport { 7375, 80, 443, 18888' /home/govan/tmp/unheaded/deploy/nftables/west.nft; sudo nft list ruleset | grep -E 'wlp17s0.*dport 53' | wc -l
  ```
  Expect: the list, `1`, `2`. Else: an unknown listener: name it before going on. The ruleset takes no notice of it, so a needed one would silently stop.
- [ ] **Step 137** [CLAUDE][R] ~1m: For the record only, your LAN scope as answered on 2026-11-03, now in the ruleset.
  ```bash
  grep -nE 'wlp17s0.*(dport|proto-dst)' /home/govan/tmp/unheaded/deploy/nftables/west.nft | cut -c1-130
  ```
  Expect: the input line with `7375, 80, 443, 18888, 16666, 20000, 20001, 20002, 20103`, the `.132` line for 8443, and the three forward lines. Else: the bench edit did not merge: stop.
- [ ] **Step 138** [STEVIE][V] ~3m: **The IPv6 test, before.** West's global address: `ip -6 -br addr show wlp17s0 scope global`. On your phone, **Wi-Fi off, on cellular**, open `http://[that address]:3000/` (a browser, or `nc -vz` from a shell app). Note what happens: if a Grafana login appears, it has been open to the internet. Say so; it goes in the progress file either way.
- [ ] **Step 139** [CLAUDE][S] ~2m: Script and ruleset to stable paths; silence.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 60m "west firewall" 2>&1 | tail -1 | cut -c1-60; sudo install -D -m 0755 scripts/host/nft-apply-confirm.sh /usr/local/lib/unheaded/nft-apply-confirm.sh && sudo install -D -m 0644 deploy/nftables/west.nft /usr/local/lib/unheaded/nft/west.nft && sudo nft -c -f /usr/local/lib/unheaded/nft/west.nft && echo NFT_OK
  ```
  Expect: `NFT_OK`. Else: [STUCK].
- [ ] **Step 140** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 136-140: Evening 7, checked: The IPv6 test, before. Wests global address: ip -6 -br addr show wlp17s0 scope global. On.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 136-140: Checkpoint in Evening 7: The IPv6 test, before. Wests global address: ip -6 -br addr show wlp17s0 scope global. On' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 141** [CLAUDE][S] ~1m: **Apply, 10 minutes.** Walk the next two steps inside that.
  ```bash
  sudo /usr/local/lib/unheaded/nft-apply-confirm.sh apply /usr/local/lib/unheaded/nft/west.nft 600
  ```
  Expect: `... rolls back in 600s unless you run: ... confirm`. Else: read it.
  **Rollback:** automatic at 600 s, or `sudo /usr/local/lib/unheaded/nft-apply-confirm.sh rollback`
- [ ] **Step 142** [STEVIE][V] ~3m: **Inside the window, from the laptop**, in a new terminal: a new `ssh -p 7375 govan@192.168.69.184` logs in; `https://192.168.69.184/` loads; `nc -vz -w3 192.168.69.184 9100` times out; `http://192.168.69.184:3001/` loads; `nc -vz -w3 192.168.69.184 18000` times out. Phone on Wi-Fi: DNS works, a page loads. **Phone on cellular: the Step 138 test now fails.** **Expect:** all. **Else:** do nothing, it rolls back; write down what failed. A cellular test that still connects means the fix did not take: do not confirm.
- [ ] **Step 143** [CLAUDE][V] ~2m: **Inside the window, from west**: east, LXD, Docker, Pi-hole.
  ```bash
  ssh -o BatchMode=yes -o ControlPath=none east 'echo EAST_OK; sudo apt-get update 2>&1 | tail -1 | cut -c1-40; resolvectl query archive.ubuntu.com | head -1 | cut -c1-40'; dig +short +time=2 @10.10.20.252 archive.ubuntu.com A | head -1; /snap/bin/lxc exec pihole --project default -- ping -c1 -W2 1.1.1.1 | tail -2 | head -1 | cut -c1-50; docker exec unheaded-prometheus wget -qO- -T3 'http://[fd00:dead:beef::2]:9100/metrics' | grep -c '^node_uname_info'; docker logs --since 3m unheaded-prometheus 2>&1 | grep -ci 'remote_write.*\(error\|failed\)'; sudo nft list chain inet unheaded forward | grep -c 'ct status dnat'; curl -s 'http://127.0.0.1:9090/api/v1/query?query=up' | python3 -c "import sys,json; r=json.load(sys.stdin)['data']['result']; print('up', sum(x['value'][1]=='1' for x in r), 'of', len(r))"; sudo nft list chain inet unheaded input | grep -c 'policy drop'
  ```
  Expect: `EAST_OK`, a reading-lists line, an address on each of the next two, `1 packets transmitted, 1 received`, `1`, `0` (remote_write to 8428 unharmed), `3`, the same `up` count as the morning, `1`. Else: leave it: it rolls back.
- [ ] **Step 144** [CLAUDE][S] ~1m: Confirm and install the boot unit.
  ```bash
  cd /home/govan/tmp/unheaded && sudo /usr/local/lib/unheaded/nft-apply-confirm.sh confirm && sudo install -m 0644 deploy/systemd/unheaded-nft.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable unheaded-nft.service 2>&1 | tail -1 | cut -c1-60; sudo systemctl start unheaded-nft.service; systemctl is-active unheaded-nft.service; sudo nft list tables | grep -c 'inet unheaded$'
  ```
  Expect: `confirmed`, `active`, `1`. Else: as on east.
  **Rollback:** `sudo systemctl disable unheaded-nft.service; sudo systemctl stop unheaded-nft.service`
- [ ] **Step 145** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 141-145: Evening 7, checked: Inside the window, from west: east, LXD, Docker, Pi-hole..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 141-145: Checkpoint in Evening 7: Inside the window, from west: east, LXD, Docker, Pi-hole.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 146** [CLAUDE][V] ~10m: Ten quiet minutes: nothing you depend on is in the drop log, and Docker kept its rules.
  ```bash
  sudo journalctl -k --since '-10 min' --no-pager | grep -E 'unheaded-(drop|fwd-drop)' | sed -E 's/.*(IN=[^ ]*).*(SRC=[^ ]*).*(DPT=[^ ]*).*/\1 \2 \3/' | sort | uniq -c | sort -rn | head; docker ps -q | wc -l
  ```
  Expect: scanner-like noise or nothing; the running-container count unchanged. Else: a real client on the list: add it to the ruleset with a fresh apply.
- [ ] **Step 147** [CLAUDE][W] ~3m: `compliance/findings/register.yaml`: `host-probe:west:firewall-inbound-deny` gets `resolved: {on: <date>, evidence: "inet unheaded input policy drop, published ports scoped in forward, phone on cellular refused on the global IPv6 address, unheaded-nft enabled"}`. **Only if the cellular test passed: without it this finding stays open.** Record what Step 138 showed under For Stevie: if it was open, it was open from the internet for as long as the monitoring stack ran, and that is an entry in the findings register. `wiki/Security.md:31`: west too. `wiki/Overlay-Network.md`: one line on how published ports are scoped.
- [ ] **Step 148** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 146-148: West input policy drop; published ports scoped; phone on cellular refused; laptop, east, LXD, Docker, Pi-hole proven in the window; boot unit enabled.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md compliance/findings/register.yaml wiki/Security.md wiki/Overlay-Network.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 146-148: DP-22 west: default deny live' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 149** [V] ~1m: **EVENING 7 EXIT GATE**.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-60
  ```
  Expect: the line that the silence ended.
- [ ] **Step 150** [C] ~6m: **MERGE (rule 15): Evening 7: west firewall.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 7: west firewall' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 7: west firewall' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 8: WEST SSHD (Steps 151-166)

**Goal**: DP-04 hybrid-only after naming .135 and .137; DP-15 laptop key, then key-only; west's fail2ban moved to the port sshd really uses.
**Prerequisite**: Evening 7 helpful, not required.
**Time**: 1 h. **Have ready**: laptop; the devices behind .135 and .137 in hand; a terminal at west's console or the second session kept open throughout. Order matters: after key-only, **any device without a key is out**.

- [ ] **Step 151** [STEVIE] ~5m: **Name them.** Which devices are these? Claude shows what is known.
  ```bash
  sudo journalctl -u ssh --since '90 days ago' --no-pager | grep -E 'Accepted .* from 192\.168\.69\.13[57]' | tail -5 | cut -c1-110; ip neigh | grep -E '192\.168\.69\.13[257] ' | cut -c1-70
  ```
  Write down each: laptop, phone, tablet, and its SSH app. Else: if you cannot name one, it is treated as unknown: it gets no key, and it will stop working. If it was not you, that is an incident: `runbooks/security/ssh-login-new-source.yaml`.
- [ ] **Step 152** [STEVIE][V] ~3m: Each client can do hybrid. Laptop: `ssh -V; ssh -Q kex | grep -E 'mlkem768x25519|sntrup761x25519'`. **Expect:** OpenSSH 9.9 or newer and at least one line. Phone or tablet: its app's KEX list, or test after the next step. **Else:** a client that cannot: DP-04 says identify first. Decide: upgrade it, or drop it, before going on.
- [ ] **Step 153** [CLAUDE][S] ~2m: **DP-04.** Undo timer, then east's exact line.
  ```bash
  sudo systemd-run --unit=unheaded-e1-kex-undo --on-active=300 --timer-property=AccuracySec=1s /bin/sh -c 'rm -f /etc/ssh/sshd_config.d/00-unheaded-kex.conf; sshd -t && systemctl reload ssh' 2>&1 | head -1; printf '%s\n' 'KexAlgorithms mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com' | sudo install -m 0600 -o root -g root /dev/stdin /etc/ssh/sshd_config.d/00-unheaded-kex.conf && sudo sshd -t && sudo systemctl reload ssh && sudo sshd -T | grep ^kexalgorithms
  ```
  Expect: the timer line; `kexalgorithms mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com`. Else: Rollback.
  **Rollback:** `sudo rm -f /etc/ssh/sshd_config.d/00-unheaded-kex.conf; sudo sshd -t && sudo systemctl reload ssh; sudo systemctl stop unheaded-e1-kex-undo.timer`
- [ ] **Step 154** [STEVIE][V] ~3m: From the laptop, a **new** connection, and each other device: `ssh -v -p 7375 govan@192.168.69.184 true 2>&1 | grep 'kex: algorithm'`. **Expect:** `mlkem768x25519-sha256`, and a login. Claude checks a classical client is refused: `ssh -p 7375 -o KexAlgorithms=curve25519-sha256 -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null localhost true 2>&1 | tail -1` → `no matching key exchange method found`. **Else:** Rollback. If all is well: `sudo systemctl stop unheaded-e1-kex-undo.timer`.
- [ ] **Step 155** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 151-155: Evening 8, checked: From the laptop, a new connection, and each other device: ssh -v -p 7375 govan192.168.69.1.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 151-155: Checkpoint in Evening 8: From the laptop, a new connection, and each other device: ssh -v -p 7375 govan192.168.69.1' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 156** [STEVIE] ~3m: **DP-15. On the laptop**, with a passphrase you will remember:
  ```bash
  ssh-keygen -t ed25519 -a 100 -f ~/.ssh/id_ed25519_west -C 'stevie laptop to west'
  ssh-copy-id -i ~/.ssh/id_ed25519_west.pub -p 7375 govan@192.168.69.184
  cat >> ~/.ssh/config <<'EOF'

  Host west
      HostName 192.168.69.184
      Port 7375
      User govan
      IdentityFile ~/.ssh/id_ed25519_west
      IdentitiesOnly yes
  EOF
  ```
  **Expect:** `Number of key(s) added: 1` after your password. The `Host` block matters: without it a plain `ssh` offers no key and looks like a lockout after key-only. Then prove the key alone: `ssh -o PreferredAuthentications=publickey west 'echo KEY_LOGIN_OK'` → `KEY_LOGIN_OK`. **Else:** on west `stat -c '%a' ~ ~/.ssh ~/.ssh/authorized_keys` should be at most `750 700 600`. Do not go on until this prints.
  **Rollback:** remove the line from `~/.ssh/authorized_keys` on west.
- [ ] **Step 157** [STEVIE] ~5m: Each other device you named that must keep access: its key now, the same way. A device left without one is out after the next step.
- [ ] **Step 158** [CLAUDE][S] ~2m: **Key-only.** Keep your open session. `05-` wins over `50-cloud-init` and `99-unheaded`.
  ```bash
  sudo systemd-run --unit=unheaded-e1-keyonly-undo --on-active=300 --timer-property=AccuracySec=1s /bin/sh -c 'rm -f /etc/ssh/sshd_config.d/05-unheaded-keyonly.conf; sshd -t && systemctl reload ssh' 2>&1 | head -1; printf '%s\n' '# DP-15: the laptop has a key; no passwords on west.' 'PasswordAuthentication no' 'KbdInteractiveAuthentication no' | sudo install -m 0600 -o root -g root /dev/stdin /etc/ssh/sshd_config.d/05-unheaded-keyonly.conf && sudo sshd -t && sudo systemctl reload ssh && sudo sshd -T | grep -E '^(passwordauthentication|kbdinteractiveauthentication|permitrootlogin) '
  ```
  Expect: `permitrootlogin no`, `passwordauthentication no`, `kbdinteractiveauthentication no`. Else: Rollback.
  **Rollback:** `sudo rm -f /etc/ssh/sshd_config.d/05-unheaded-keyonly.conf; sudo sshd -t && sudo systemctl reload ssh; sudo systemctl stop unheaded-e1-keyonly-undo.timer`
- [ ] **Step 159** [STEVIE][V] ~2m: **From the laptop, new connection:** `ssh west 'echo KEY_LOGIN_OK'`. **Expect:** `KEY_LOGIN_OK`. Then `ssh -o PubkeyAuthentication=no -p 7375 govan@192.168.69.184 true` → `Permission denied (publickey).`. Then on west: `sudo systemctl stop unheaded-e1-keyonly-undo.timer`. **Else:** do not stop the timer: it undoes this at 300 s. West's console is at your elbow.
- [ ] **Step 160** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 156-160: Evening 8, checked: From the laptop, new connection: ssh west echo KEY_LOGIN_OK. Expect: KEY_LOGIN_OK. Then ss.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 156-160: Checkpoint in Evening 8: From the laptop, new connection: ssh west echo KEY_LOGIN_OK. Expect: KEY_LOGIN_OK. Then ss' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 161** [STEVIE][DECIDE] ~1m: **West's fail2ban** bans the wrong port: `port = ssh` is 22, sshd is 7375. It banned your laptop on 2026-10-29 and you logged in on 7375 a minute later. Key-only makes it less needed, not useless. **RECOMMENDATION: fix it now**, with the whole LAN range in `ignoreip` first: `.132`, `.135` and `.137` all log in as govan, and with the right port a typo ban becomes real. Not in the pack, so your word. Override ONLY IF you would rather leave it.
- [ ] **Step 162** [CLAUDE][S] ~3m: Only on your yes.
  ```bash
  sudo tee /etc/fail2ban/jail.d/unheaded-sshd.local >/dev/null <<'EOF'
  # West's sshd jail: the port in use, and the addresses that must never be banned.
  [DEFAULT]
  ignoreself = true
  ignoreip = 127.0.0.1/8 ::1 192.168.69.128/25 192.168.13.0/30 fd00:dead:beef::/48 fe80::/10

  [sshd]
  port = 7375
  EOF
  sudo systemctl restart fail2ban; sudo fail2ban-client get sshd action nftables port; sudo fail2ban-client get sshd ignoreip | grep -c '192.168.69.128/25'
  ```
  Expect: `7375`, `1`. Else: Rollback.
  **Rollback:** `sudo rm -f /etc/fail2ban/jail.d/unheaded-sshd.local; sudo systemctl restart fail2ban`
- [ ] **Step 163** [CLAUDE][W] ~5m: Register: remove rows `host-sshd:west:kbdinteractiveauthentication=no` and `host-sshd:west:passwordauthentication=no` if the next collector passes them. `docs/security/pq-host-escalations.md` E2: west done. `wiki/Post-Quantum-Key-Exchange.md` and the CLAUDE.md line "west's sshd (east is hybrid-only since 2026-10-27)". `wiki/Security.md`: west key-only and hybrid-only.
- [ ] **Step 164** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 161-164: .135 and .137 named in this file; laptop key proven before key-only; classical KEX refused.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md compliance/findings/register.yaml docs/security/pq-host-escalations.md wiki/Post-Quantum-Key-Exchange.md wiki/Security.md CLAUDE.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 161-164: DP-04, DP-15: west hybrid-only and key-only' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 165** [V] ~1m: **EVENING 8 EXIT GATE**.
  ```bash
  systemctl list-timers --all --no-legend 'unheaded-e1-*' | wc -l
  ```
  Expect: `0`. Else: stop the leftover timer once its change has passed.
- [ ] **Step 166** [C] ~6m: **MERGE (rule 15): Evening 8: west sshd.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 8: west sshd' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 8: west sshd' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 9: PSK TIMER (Steps 167-177)

**Goal**: DP-05: weekly rotation enabled, by a key on west that can do only rotation on east. Goes through `govan@east`, so `PermitRootLogin no` holds.
**Prerequisite**: Evenings 0 and 6.
**Time**: 45 m. **Have ready**: nothing special. A rotation re-handshakes the tunnel; the P2P link still carries SSH.

- [ ] **Step 167** [CLAUDE][S] ~2m: The key, on west, in `/etc` because the unit's home is read-only.
  ```bash
  sudo install -d -m 0700 /etc/unheaded/wg-rotate && sudo ssh-keygen -q -t ed25519 -N '' -C wg-rotate@west -f /etc/unheaded/wg-rotate/id_ed25519 && sudo sh -c "ssh-keyscan -p 7375 -t ed25519 192.168.13.1 2>/dev/null | sed 's/^\[192.168.13.1\]:7375/192.168.13.1/' > /etc/unheaded/wg-rotate/known_hosts"; sudo ssh-keygen -lf /etc/unheaded/wg-rotate/known_hosts | cut -d' ' -f2; ssh east 'ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub | cut -d" " -f2'
  ```
  Expect: two identical `SHA256:` values. Else: **stop**: a different host key is not a typo.
  **Rollback:** `sudo rm -rf /etc/unheaded/wg-rotate`
- [ ] **Step 168** [CLAUDE][S] ~3m: The gate on east, its pinned hash, and the key line.
  ```bash
  cd /home/govan/tmp/unheaded && H=$(awk "/^\tcat <<'SIDE'\$/{f=1;next} /^SIDE\$/{f=0} f" scripts/wireguard/rotate-psk.sh | sha256sum | cut -d' ' -f1); scp -q scripts/wireguard/psk-gate.sh east:/tmp/ && sudo cat /etc/unheaded/wg-rotate/id_ed25519.pub | ssh east "sudo install -D -m 0755 /tmp/psk-gate.sh /usr/local/lib/unheaded/wireguard/psk-gate.sh && printf '%s\n' $H | sudo install -m 0644 /dev/stdin /usr/local/lib/unheaded/wireguard/side.sha256 && cp -p ~/.ssh/authorized_keys ~/.ssh/authorized_keys.bak-e1 && { printf 'command=\"/usr/local/lib/unheaded/wireguard/psk-gate.sh\",no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-pty '; cat; } >> ~/.ssh/authorized_keys; grep -c psk-gate ~/.ssh/authorized_keys"
  ```
  Expect: `1`. Else: Rollback.
  **Rollback:** `ssh east 'cp -p ~/.ssh/authorized_keys.bak-e1 ~/.ssh/authorized_keys'`
- [ ] **Step 169** [CLAUDE][V] ~2m: The key does what is allowed and nothing else.
  ```bash
  O='-i /etc/unheaded/wg-rotate/id_ed25519 -o IdentitiesOnly=yes -o BatchMode=yes -o UserKnownHostsFile=/etc/unheaded/wg-rotate/known_hosts'; sudo ssh $O govan@east true; echo true_rc=$?; sudo ssh $O govan@east id; echo id_rc=$?; sudo ssh $O govan@east 'sudo -n sh -s -- apply wg0 /etc/wireguard x' </dev/null; echo sh_rc=$?
  ```
  Expect: `true_rc=0`; `psk-gate: refused`, `id_rc=3`; `psk-gate: refused`, `sh_rc=3`. Else: **a successful `id` means the key is a shell**: Rollback, stop.
- [ ] **Step 170** [CLAUDE][S] ~3m: Unit drop-in, then a dry run through it.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -D -m 0755 scripts/wireguard/rotate-psk.sh /usr/local/lib/unheaded/wireguard/rotate-psk.sh && sudo install -m 0644 deploy/systemd/unheaded-wg-psk-rotate.service deploy/systemd/unheaded-wg-psk-rotate.timer /etc/systemd/system/ && sudo install -d /etc/systemd/system/unheaded-wg-psk-rotate.service.d && printf '%s\n' '[Service]' 'Environment="WG_PSK_SSH_OPTS=-i /etc/unheaded/wg-rotate/id_ed25519 -o IdentitiesOnly=yes -o UserKnownHostsFile=/etc/unheaded/wg-rotate/known_hosts"' | sudo tee /etc/systemd/system/unheaded-wg-psk-rotate.service.d/10-key.conf >/dev/null && sudo systemctl daemon-reload; sudo env WG_PSK_SSH_OPTS='-i /etc/unheaded/wg-rotate/id_ed25519 -o IdentitiesOnly=yes -o UserKnownHostsFile=/etc/unheaded/wg-rotate/known_hosts' /usr/local/lib/unheaded/wireguard/rotate-psk.sh 2>&1 | tail -3 | cut -c1-80
  ```
  Expect: the plan, naming `(post-quantum hybrid)`, exit 0. Else: `refusing:` lines say why; a gate refusal: check `journalctl -t unheaded-psk-gate` on east, then the hash in `side.sha256`.
  **Rollback:** `sudo rm -rf /etc/systemd/system/unheaded-wg-psk-rotate.service.d && sudo systemctl daemon-reload`
- [ ] **Step 171** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 167-171: Evening 9, checked: The key does what is allowed and nothing else..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 167-171: Checkpoint in Evening 9: The key does what is allowed and nothing else.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 172** [CLAUDE][S] ~5m: One live rotation through the real unit, watched. Up to about 3 minutes.
  ```bash
  sudo systemctl start unheaded-wg-psk-rotate.service; systemctl show -p Result --value unheaded-wg-psk-rotate.service; sudo journalctl -t unheaded-wg-psk --since '-6 min' --no-pager | tail -2 | cut -c1-90; ping -6 -c2 -W1 fd00:dead:beef::2 | tail -2 | head -1 | cut -c1-50
  ```
  Expect: `success`; `psk-rotated <time> wg0`; `2 received`. Else: it rolls itself back and logs `psk-rotation-rolled-back`. `MemoryDenyWriteExecute` or `ProtectHome` blocking ssh shows in `journalctl -u unheaded-wg-psk-rotate -n 20`. The P2P link carries SSH regardless.
- [ ] **Step 173** [STEVIE] ~1m: Enable the weekly timer.
  ```bash
  sudo systemctl enable --now unheaded-wg-psk-rotate.timer; systemctl list-timers --no-legend unheaded-wg-psk-rotate.timer | cut -c1-40
  ```
  Expect: a next-run date. Else: [STUCK].
  **Rollback:** `sudo systemctl disable --now unheaded-wg-psk-rotate.timer`
- [ ] **Step 174** [CLAUDE][W] ~3m: `deploy/systemd/unheaded-wg-psk-rotate.timer` line 2 and `.service` header: enabled, DP-05; `docs/security/pq-host-escalations.md` E3 done; register FND-016 `resolved:`; `wiki/Overlay-Network.md`.
- [ ] **Step 175** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 172-175: Rotation key refused id and a raw root shell, rotated once live, timer enabled.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md deploy/systemd/unheaded-wg-psk-rotate.timer deploy/systemd/unheaded-wg-psk-rotate.service docs/security/pq-host-escalations.md compliance/findings/register.yaml wiki/Overlay-Network.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 172-175: DP-05: weekly PSK rotation on, key scoped by the gate' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 176** [V] ~1m: **EVENING 9 EXIT GATE**.
  ```bash
  systemctl is-enabled unheaded-wg-psk-rotate.timer
  ```
  Expect: `enabled`. Else: [STUCK].
- [ ] **Step 177** [C] ~6m: **MERGE (rule 15): Evening 9: PSK timer.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 9: PSK timer' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 9: PSK timer' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 10: CUSTODY AND SIGNING (Steps 178-205)

**Goal**: DP-06: build-key seed encrypted at rest, opened only with a YubiKey present. DP-14: apt repo signed by a host-held key, published, `trusted=yes` gone. Heimdall and Wotan daemon keys on the hosts, public halves published.
**Prerequisite**: Evening 5. `unheaded-buildkey`, `unheaded-heimdall` exist (Evening 4).
**Time**: 4 h. **Have ready**: both YubiKeys and their FIDO2 PINs; GPG cached. Unplug everything but one YubiKey when a step says so: `fido2-device=auto` refuses two. These runbooks are drafts, so at each one you approve it first (DP-301): `status: approved`, `approved_by: stevie`, `approved_at: <date>`, then `scripts/runbook-manifest.sh --write`.

Method, no new package: a LUKS2 file with the seed inside and both tokens enrolled through FIDO2 (`cryptsetup`, `systemd-cryptenroll`, `libfido2-1` are already on west; confirmed `/usr/bin/systemd-cryptsetup`). Two tokens cover a lost key; the encrypted file copied to east covers a lost disk. Losing both tokens means a new key and a revocation entry.

- [ ] **Step 178** [STEVIE][V] ~2m: Plug in YubiKey 1 only.
  ```bash
  sudo systemd-cryptenroll --fido2-device=list
  ```
  Expect: one row with `Yubico`. Else: nothing listed: try the other port. A key without FIDO2 cannot serve this method: stop, the rest of custody waits.
- [ ] **Step 179** [STEVIE][S] ~10m: **Rehearse on a throwaway file** with both keys. Nothing here is the real seed.
  ```bash
  sudo install -d -m 0700 /var/lib/unheaded/custody && sudo truncate -s 32M /var/lib/unheaded/custody/rehearsal.luks && sudo sh -c 'umask 077; head -c 48 /dev/urandom | base64 -w0 > /run/custody.tmpkey' && sudo cryptsetup luksFormat --batch-mode --type luks2 --key-file /run/custody.tmpkey /var/lib/unheaded/custody/rehearsal.luks && L=$(sudo losetup --find --show /var/lib/unheaded/custody/rehearsal.luks) && echo "$L" && sudo systemd-cryptenroll --unlock-key-file=/run/custody.tmpkey --fido2-device=auto --fido2-with-client-pin=yes --fido2-with-user-presence=yes "$L"
  ```
  Type the PIN, touch the key. Expect: `New FIDO2 token enrolled as key slot 1.` Swap to YubiKey 2 and repeat the last command: `... key slot 2.`. Then with key 1 alone, then key 2 alone, then neither:
  ```bash
  sudo /usr/bin/systemd-cryptsetup attach custody-rehearsal "$L" none fido2-device=auto && ls /dev/mapper/custody-rehearsal && sudo /usr/bin/systemd-cryptsetup detach custody-rehearsal
  ```
  Expect: opens with each key; fails with neither. Else: **stop here**, do not make the real one. `sudo losetup -d "$L"`, then the Rollback.
  **Rollback:** `sudo /usr/bin/systemd-cryptsetup detach custody-rehearsal; sudo losetup -d "$L"; sudo rm -f /var/lib/unheaded/custody/rehearsal.luks; sudo shred -u /run/custody.tmpkey`
- [ ] **Step 180** [CLAUDE][S] ~1m: Clear the rehearsal. This shell did not make the loop device, so find it by file.
  ```bash
  L=$(sudo losetup -j /var/lib/unheaded/custody/rehearsal.luks | cut -d: -f1); sudo losetup -d "$L"; sudo rm -f /var/lib/unheaded/custody/rehearsal.luks; sudo shred -u /run/custody.tmpkey; sudo ls /var/lib/unheaded/custody | wc -l
  ```
  Expect: `0`. Else: [STUCK].
- [ ] **Step 181** [STEVIE][S] ~10m: **The real file.** Approve `runbooks/security/build-key-ceremony.yaml` first. Same as the rehearsal, enrol key 1, swap, enrol key 2, then wipe the temporary passphrase so only the tokens open it.
  ```bash
  sudo truncate -s 32M /var/lib/unheaded/custody/buildkey.luks && sudo sh -c 'umask 077; head -c 48 /dev/urandom | base64 -w0 > /run/custody.tmpkey' && sudo cryptsetup luksFormat --batch-mode --type luks2 --key-file /run/custody.tmpkey /var/lib/unheaded/custody/buildkey.luks && L=$(sudo losetup --find --show /var/lib/unheaded/custody/buildkey.luks) && sudo systemd-cryptenroll --unlock-key-file=/run/custody.tmpkey --fido2-device=auto --fido2-with-client-pin=yes --fido2-with-user-presence=yes "$L"
  ```
  Swap keys, the same `systemd-cryptenroll` line again. **Before the password slot goes, open the real file with each token, alone.** Key 2 is in now; run this, swap to key 1, run it again:
  ```bash
  L=$(sudo losetup -j /var/lib/unheaded/custody/buildkey.luks | cut -d: -f1); sudo /usr/bin/systemd-cryptsetup attach unheaded-buildkey "$L" none fido2-device=auto && ls /dev/mapper/unheaded-buildkey && sudo /usr/bin/systemd-cryptsetup detach unheaded-buildkey && echo TOKEN_OPENS
  ```
  Expect: `TOKEN_OPENS` for each. Else: **do not wipe**; re-enrol that token with the temporary key. Then:
  ```bash
  L=$(sudo losetup -j /var/lib/unheaded/custody/buildkey.luks | cut -d: -f1); sudo systemd-cryptenroll --wipe-slot=password "$L"; sudo shred -u /run/custody.tmpkey; sudo cryptsetup luksDump "$L" | grep -cE '^\s+[0-9]+: luks2'; sudo cryptsetup luksDump "$L" | grep -c 'systemd-fido2'
  ```
  Expect: `2`, `2`. Else: [STUCK] before any seed exists: the Rollback costs nothing.
  **Rollback:** before a seed exists: `sudo losetup -d "$L"; sudo rm -f /var/lib/unheaded/custody/buildkey.luks`
- [ ] **Step 182** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 178-182: Evening 10, checked: Plug in YubiKey 1 only..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 178-182: Checkpoint in Evening 10: Plug in YubiKey 1 only.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 183** [STEVIE][S] ~5m: Open, make the filesystem, make the key inside it. The seed is never on a plain disk.
  ```bash
  cd /home/govan/tmp/unheaded && L=$(sudo losetup -j /var/lib/unheaded/custody/buildkey.luks | cut -d: -f1) && sudo /usr/bin/systemd-cryptsetup attach unheaded-buildkey "$L" none fido2-device=auto && sudo mkfs.ext4 -q -L buildkey /dev/mapper/unheaded-buildkey && sudo install -d -m 0750 -o root -g unheaded-buildkey /run/unheaded-buildkey && sudo mount -o nodev,nosuid,noexec /dev/mapper/unheaded-buildkey /run/unheaded-buildkey && sudo bin/release/unheaded-keygen -algo mldsa65 -dir /run/unheaded-buildkey -owner unheaded-buildkey build-evidence-2026q4 2>&1 | tail -2 | cut -c1-90; sudo stat -c '%a %U' /run/unheaded-buildkey/build-evidence-2026q4.key
  ```
  Expect: `400 unheaded-buildkey`. Else: close (Step 186), [STUCK]; a key name is never reused, so a failed run takes `-2026q4b`.
- [ ] **Step 184** [STEVIE][W] ~5m: Publish the public half: your signed commit. Claude first writes the sha256, key id and date into `SECURITY.md`, as the runbook's `print-public-key` and `publish-public-key` steps say; you read it.
  ```bash
  cd /home/govan/tmp/unheaded && install -d compliance/keys && sudo cat /run/unheaded-buildkey/build-evidence-2026q4.pub > compliance/keys/build.pubkey && sha256sum compliance/keys/build.pubkey | cut -c1-16 && git add compliance/keys/build.pubkey SECURITY.md && git commit -S -q -m 'custody: publish build.pubkey (DP-06)' && git log -1 --format='%h %G?'
  ```
  Expect: a hash, `<hash> G`. Else: if the file is named differently, `sudo ls /run/unheaded-buildkey /etc/unheaded/trust.d`: keygen may have put the public half in the trust directory.
- [ ] **Step 185** [STEVIE][V] ~5m: The runbook's proof: a cask signed with the new key verifies; one appended byte is refused.
  ```bash
  cd /home/govan/tmp/unheaded && D=$(mktemp -d) && UNHEADED_BUILD_KEY=/run/unheaded-buildkey/build-evidence-2026q4.key UNHEADED_BUILD_SIGN_AS=unheaded-buildkey scripts/build-sealed-cask.sh --no-build --no-encrypt --output "$D/cask.tar.gz" 2>&1 | tail -1 | cut -c1-80; bin/release/cask-verify verify --cask "$D/cask.tar.gz" --pubkey compliance/keys/build.pubkey | tail -1 | cut -c1-70; printf x >> "$D/cask.tar.gz"; bin/release/cask-verify verify --cask "$D/cask.tar.gz" --pubkey compliance/keys/build.pubkey >/dev/null 2>&1; echo tamper_rc=$?; rm -rf "$D"
  ```
  Expect: a verify pass, `tamper_rc` not `0`. Else: [STUCK]; FND-018 stays open.
- [ ] **Step 186** [STEVIE][S] ~5m: Close it, then prove the **other** token still opens the volume now the seed exists. With the token out, nothing on west can read the seed.
  ```bash
  L=$(sudo losetup -j /var/lib/unheaded/custody/buildkey.luks | cut -d: -f1); sudo umount /run/unheaded-buildkey && sudo /usr/bin/systemd-cryptsetup detach unheaded-buildkey && echo CLOSED
  ```
  Swap to the other key, then:
  ```bash
  L=$(sudo losetup -j /var/lib/unheaded/custody/buildkey.luks | cut -d: -f1); sudo /usr/bin/systemd-cryptsetup attach unheaded-buildkey "$L" none fido2-device=auto && sudo mount -o ro,nodev,nosuid,noexec /dev/mapper/unheaded-buildkey /run/unheaded-buildkey && sudo test -s /run/unheaded-buildkey/build-evidence-2026q4.key && echo SECOND_TOKEN_READS; sudo umount /run/unheaded-buildkey; sudo /usr/bin/systemd-cryptsetup detach unheaded-buildkey; sudo losetup -d "$L"; ls /dev/mapper | grep -c unheaded-buildkey; echo '-w /var/lib/unheaded/custody/buildkey.luks -p rwa -k unheaded-buildkey' | sudo tee /etc/audit/rules.d/unheaded-buildkey.rules >/dev/null; sudo augenrules --load 2>&1 | tail -1 | cut -c1-50; sudo auditctl -l | grep -c unheaded-buildkey
  ```
  Expect: `SECOND_TOKEN_READS`, `0`, a rules line, `1`. Unplug both keys and keep them apart. Else: [STUCK].
- [ ] **Step 187** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 183-187: Evening 10, checked: The runbooks proof: a cask signed with the new key verifies one appended byte is refused..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 183-187: Checkpoint in Evening 10: The runbooks proof: a cask signed with the new key verifies one appended byte is refused.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 188** [CLAUDE][S] ~3m: A lost disk must not lose the key: the **encrypted** file goes to east, useless without a token. DP-06 supersedes the runbook's old "no copy".
  ```bash
  sudo sha256sum /var/lib/unheaded/custody/buildkey.luks | cut -c1-16; sudo cat /var/lib/unheaded/custody/buildkey.luks | ssh east 'sudo install -d -m 0700 /var/backups/unheaded-custody && sudo install -m 0400 /dev/stdin /var/backups/unheaded-custody/buildkey.luks && sudo sha256sum /var/backups/unheaded-custody/buildkey.luks | cut -c1-16'
  ```
  Expect: two equal strings. Else: [STUCK]; the single copy stands until it is fixed.
  **Rollback:** `ssh east 'sudo rm -rf /var/backups/unheaded-custody'`
- [ ] **Step 189** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 188-189: Rehearsed with both tokens and with none; real volume opened by each before the password slot went; seed inside; test cask verifies, tamper refused; second token reads it; encrypted copy on east. Next quarterly rotation 2027-02.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 188-189: DP-06: build key in a FIDO2-unlocked LUKS file, public half published' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.

### DP-14: the apt repo

- [ ] **Step 190** [CLAUDE][W] ~3m: Add a signer account under the DP-06 daemon-key rule. `deploy/sysusers.d/unheaded.conf`, after the `nginx` line: `u     unheaded-apt           -   "Unheaded apt repo signer"              /    /usr/sbin/nologin`. Then `sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers unheaded.conf; id -u unheaded-apt`. Expect: a uid. Else: [STUCK].
- [ ] **Step 191** [STEVIE][S] ~3m: The repo key, on west, in a home only that account can read. No passphrase: the host signs unattended, by DP-14's rule; rotation is quarterly.
  ```bash
  sudo install -d -m 0700 -o unheaded-apt -g unheaded-apt /var/lib/unheaded/apt-gpg && sudo -u unheaded-apt env GNUPGHOME=/var/lib/unheaded/apt-gpg gpg --batch --passphrase '' --quick-generate-key 'Unheaded apt repository <stevie@bellis.tech>' ed25519 sign 1y 2>&1 | tail -1 | cut -c1-70; sudo -u unheaded-apt env GNUPGHOME=/var/lib/unheaded/apt-gpg gpg --list-keys --with-colons | awk -F: '/^fpr/{print $10; exit}'
  ```
  Expect: a 40-hex fingerprint: write it down as `KEYID`. Else: [STUCK].
  **Rollback:** `sudo rm -rf /var/lib/unheaded/apt-gpg`
- [ ] **Step 192** [CLAUDE][W] ~2m: `deploy/nginx/unheaded-apt.conf`: `root /var/lib/apt-repo;` becomes `root /srv/unheaded-apt;`. The runbook installs this file from the live checkout, so it merges first.
- [ ] **Step 193** [C] ~6m: **MERGE (rule 15): Evening 10: nginx root and accounts for the apt step.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10: nginx root and accounts for the apt step' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10: nginx root and accounts for the apt step' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.
- [ ] **Step 194** [STEVIE] ~45m: Approve `runbooks/deploy/apt-repo-publish.yaml` (DP-301). Claude walks it with `KEYID`, `GNUPGHOME=/var/lib/unheaded/apt-gpg`, `APT_REPO_BASE=/srv/unheaded-apt`, `TAG` = a signed tag on develop: `build-debs` (20 minutes or more), `init-repo`, `publish`. That `GNUPGHOME` is 0700 for `unheaded-apt`, so `build-debs` runs **without** `DEB_SIGN_KEY`, and `SHA256SUMS` is signed afterwards as that account: `sudo -u unheaded-apt env GNUPGHOME=/var/lib/unheaded/apt-gpg gpg --batch --detach-sign -o dist/debs/$TAG/SHA256SUMS.gpg dist/debs/$TAG/SHA256SUMS`; the output directory needs that account's write access for the moment. `init-repo` and `publish` run under `sudo` and read it as root. The runbook reloads nginx; on 11-03 nginx needed a **restart** for this vhost. **Expect:** each step's `verify` passes: `/conf/distributions` 403, `InRelease` begins `BEGIN PGP SIGNED MESSAGE`. **Else:** the runbook's rollback; east keeps the old repo, which still answers from `/var/lib/apt-repo` until nginx moves.
  **Rollback:** restore the old `root` line, `sudo systemctl restart nginx`.
- [ ] **Step 195** [CLAUDE][S] ~3m: Runbook step `east-source`: pin the key, drop `trusted=yes`. Put your fingerprint in place of `KEYID_HERE`: the source only changes if it matches. `install -m 0644` because east's umask would leave the keyring unreadable to `_apt`.
  ```bash
  ssh east 'sudo install -d -m 0755 /etc/apt/keyrings && curl -fsS http://192.168.13.2:18888/unheaded-apt.gpg | sudo install -m 0644 /dev/stdin /etc/apt/keyrings/unheaded.gpg; fpr=$(gpg --show-keys --with-colons /etc/apt/keyrings/unheaded.gpg | awk -F: "/^fpr/{print \$10; exit}"); echo "$fpr"; [ "$fpr" = "KEYID_HERE" ] || { echo WRONG_KEY; sudo rm -f /etc/apt/keyrings/unheaded.gpg; exit 1; }; sudo mv /etc/apt/sources.list.d/unheaded.sources /var/backups/unheaded.sources.bak-e1; sudo rm -f /etc/apt/sources.list.d/unheaded.list.bak; echo "deb [signed-by=/etc/apt/keyrings/unheaded.gpg] http://192.168.13.2:18888 unheaded main" | sudo tee /etc/apt/sources.list.d/unheaded.list >/dev/null; sudo apt-get update 2>&1 | grep -ciE "unheaded.*(err|fail|not signed)"; sudo grep -rc "trusted" /etc/apt/sources.list.d/ | grep -v ":0" | wc -l'
  ```
  Expect: your `KEYID`, `0`, `0`. Else: `WRONG_KEY`: stop, do not trust it; nothing moved.
  **Rollback:** `ssh east 'sudo rm -f /etc/apt/sources.list.d/unheaded.list; sudo mv /var/backups/unheaded.sources.bak-e1 /etc/apt/sources.list.d/unheaded.sources; sudo apt-get update'`
- [ ] **Step 196** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 194-196: Repo at /srv/unheaded-apt signed by a host key under unheaded-apt; InRelease proven; east uses signed-by; trusted=yes gone.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md deploy/sysusers.d/unheaded.conf deploy/nginx/unheaded-apt.conf runbooks/deploy/apt-repo-publish.yaml runbooks/MANIFEST.sha256 && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 194-196: DP-14: apt repo signed, east pins the key' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.

### Daemon keys: Heimdall, Wotan

- [ ] **Step 197** [CLAUDE][W] ~5m: Compose mounts the repo's `configs/wotan.yaml`, where `keys: []` must stay, while the runbook enrols in a host copy. In the worktree, `docker-compose.yml`. Old: `- ./configs/wotan.yaml:/etc/wotan/wotan.yaml:ro`. New: `- ${WOTAN_CONFIG:-./configs/wotan.yaml}:/etc/wotan/wotan.yaml:ro`. In `akira`: add `HEALTH_SIGNING_KEY_FILE: /run/secrets/health_signing_key`, a `health_signing_key` secret on `./configs/keys/dev/akira-health.key`, and remove the comment that says there is none.
- [ ] **Step 198** [C] ~6m: **MERGE (rule 15): Evening 10: compose reads the host Wotan config.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10: compose reads the host Wotan config' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10: compose reads the host Wotan config' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.
- [ ] **Step 199** [CLAUDE][S] ~2m: The host copy.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -m 0644 -o root -g root configs/wotan.yaml /etc/unheaded/wotan.yaml && grep -q '^WOTAN_CONFIG=' .env || echo 'WOTAN_CONFIG=/etc/unheaded/wotan.yaml' >> .env; docker compose config --quiet; echo rc=$?
  ```
  Expect: `rc=0`. Else: [STUCK]. The old April file at that path is replaced on purpose.
- [ ] **Step 200** [STEVIE] ~20m: Approve `runbooks/security/heimdall-enrol-drift-key.yaml`. Claude walks it for `NODE=west`, then `NODE=east`: key generated **on** each node with `-algo mldsa65 -owner unheaded-heimdall`, base64 public key into `/etc/unheaded/wotan.yaml` under `drift.`, `sudo docker compose up -d --force-recreate wotan`, start `unheaded-heimdall.service`, provoke drift in a scratch copy. West needs the binary and unit installed first (`bin/release/heimdall-daemon`, `deploy/systemd/unheaded-heimdall.service`); east's comes with Evening 10B's debs, so east's half of this step waits there. **Expect:** the runbook's `heimdall-active-and-publishing` check passes; a drift event lands signed. **Else:** the runbook's rollback, FND-019 stays open.
  **Rollback:** `sudo systemctl disable --now unheaded-heimdall`, remove the key from `/etc/unheaded/wotan.yaml`, recreate wotan.
- [ ] **Step 201** [CLAUDE][S] ~10m: Akira's health-signing key, the other live producer. `audit.` and `alerts.critical.` have **no first-party producer yet**: nothing to enrol, they stay `keys: []`, which refuses every publish.
  ```bash
  cd /home/govan/tmp/unheaded && sudo bin/release/unheaded-keygen -algo mldsa65 -dir configs/keys/dev -trust-dir configs/trust.d/dev -owner "$(grep -m1 -oE 'user: "[0-9]+' <(awk '/^  akira:$/,/^  monad:$/' docker-compose.yml) | grep -oE '[0-9]+')" akira-health 2>&1 | tail -2 | cut -c1-90
  ```
  Add its printed public key under `system.health.consensus.` and `system.health.remediate.` in `/etc/unheaded/wotan.yaml`. `sudo docker compose up -d --force-recreate wotan akira`. Expect: `docker logs --tail 20 unheaded-akira` shows no signing refusal; a Wotan subscribe to `system.health.consensus` sees a signed message. Else: `git revert` the compose commit through the worktree, [STUCK].
- [ ] **Step 202** [CLAUDE][W] ~5m: Publish public halves: `compliance/keys/heimdall-west.pub`, `compliance/keys/akira-health.pub`. Register FND-018 and FND-019 `resolved:` lines with this evening's evidence (east's drift key closes FND-019's east half in Evening 10B). Add to `wiki/Security.md`, under `## Compliance`, one paragraph: which seed is where, and the rotation month (2027-02).
- [ ] **Step 203** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 199-203: Heimdall west signing drift; Akira signing health; audit and alerts.critical have no producer yet and stay refused.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md compliance/keys/heimdall-west.pub compliance/keys/akira-health.pub compliance/findings/register.yaml wiki/Security.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 199-203: DP-06: daemon keys on hosts, public halves published' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 204** [V] ~1m: **EVENING 10 EXIT GATE**.
  ```bash
  sudo test -e /dev/mapper/unheaded-buildkey; echo closed_rc=$?; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18888/dists/unheaded/InRelease
  ```
  Expect: `closed_rc=1`, `200`. Else: [STUCK].
- [ ] **Step 205** [C] ~6m: **MERGE (rule 15): Evening 10: custody and signing.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10: custody and signing' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10: custody and signing' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 10B: EAST'S DEBS AND TLS (Steps 206-219)

**Goal**: east off the 1.0.0 debs and onto TLS, which is what finishes DP-23 and closes FND-017 for both hosts; east's Wotan back in the SLO.
**Prerequisite**: Evening 10.
**Time**: 2 h. **Have ready**: at the desk. West's first-party services are not touched.

- [ ] **Step 206** [CLAUDE][B] ~10m: Certificates for east's identities, signed by the CA west's clients already trust, naming what callers dial. `cert-gen` has no host-name option, so plain `openssl`. No key leaves in the clear in a repo: all under `/var/tmp/e1-east-tls`, shredded after.
  ```bash
  cd /home/govan/tmp/unheaded && W=/var/tmp/e1-east-tls && install -d -m 0700 $W && for i in wotan timeguru captain architect micromanager akira unheaded-daemon dashboard-backend kanban-app auth; do openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -keyout $W/$i-key.pem -subj "/CN=$i" -out $W/$i.csr 2>/dev/null && openssl x509 -req -in $W/$i.csr -CA configs/certs/dev/ca.pem -CAkey configs/certs/dev/ca-key.pem -CAcreateserial -days 365 -extfile <(printf 'subjectAltName=DNS:east,DNS:%s,DNS:localhost,IP:127.0.0.1,IP:::1,IP:192.168.13.1,IP:fd00:dead:beef::2\n' "$i") -out $W/$i.pem; done; ls $W/*.pem | wc -l
  ```
  Expect: `20`. Else: [STUCK], east stays as it is.
  **Rollback:** `shred -u /var/tmp/e1-east-tls/*; rmdir /var/tmp/e1-east-tls`
- [ ] **Step 207** [CLAUDE][S] ~10m: Install per `deploy/systemd/README.md#tls` on east. The directory and group come from each deb's unit, so read them from the units: `grep -h 'UNHEADED_TLS_CERT' deploy/systemd/unheaded-*.service | sort -u`. Copy the CA to `/etc/unheaded/tls/ca.pem` and `<identity>/ca.pem`; cert 0644; key 0640 `root:unheaded-<unit>`. Accounts exist on east (sysusers refreshed 11-03). Expect: `ssh east 'sudo find /etc/unheaded/tls -name "*-key.pem" | wc -l'` → `10`. Else: [STUCK].
  **Rollback:** `ssh east 'sudo rm -rf /etc/unheaded/tls'`
- [ ] **Step 208** [CLAUDE][S] ~15m: Service keys on east, public halves to both trust directories, so east's tokens verify on west and the reverse. Per identity on east: `sudo /opt/unheaded/bin/unheaded-keygen -owner unheaded-<unit> <identity>`, after `unheaded-keygen` is installed in the next step: so run the next step's `apt-get install unheaded-keygen` first. Copy each `.pub` to west's `configs/trust.d/dev/` and `/etc/unheaded/trust.d/`, and west's `.pub`s to east's `/etc/unheaded/trust.d/`.
- [ ] **Step 209** [STEVIE][S] ~15m: **Runbook `install-east`.** East's services restart onto TLS. Three units are renamed; the deb's preinst carries enabled state, but `unheaded-daemon` may need enabling by hand. Your second terminal stays on east.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 60m "east debs" 2>&1 | tail -1 | cut -c1-60; ssh east 'sudo apt-get update 2>&1 | tail -1 | cut -c1-40; sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y unheaded-keygen unheaded-configs unheaded-wotan unheaded-daemon unheaded-dashboard-backend unheaded-kanban-app unheaded-akira unheaded-timeguru unheaded-captain unheaded-architect unheaded-micromanager unheaded-auth 2>&1 | tail -3 | cut -c1-90; dpkg -l "unheaded-*" | awk "/^ii/{print \$2, \$3}"'
  ```
  Expect: no `Errors were encountered`; new versions. Else: read the postinst warnings: a missing TLS file leaves that service on its old binary. The runbook's `east-previous-version` step is the Rollback.
  **Rollback:** runbook step `east-previous-version`
- [ ] **Step 210** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 206-210: Evening 10B, checked: Runbook install-east. Easts services restart onto TLS. Three units are renamed the debs pr.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 206-210: Checkpoint in Evening 10B: Runbook install-east. Easts services restart onto TLS. Three units are renamed the debs pr' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 211** [CLAUDE][S] ~5m: Enable what should run, including the two stopped at Evening 5.
  ```bash
  ssh east 'for u in unheaded-wotan unheaded-timeguru unheaded-daemon unheaded-dashboard unheaded-kanban unheaded-captain unheaded-architect unheaded-micromanager unheaded-akira; do sudo systemctl enable --now $u 2>/dev/null; done; sleep 15; for u in unheaded-wotan unheaded-timeguru unheaded-daemon unheaded-dashboard unheaded-kanban; do echo "$u $(systemctl is-active $u) $(systemctl show -p NRestarts --value $u)"; done; systemctl --failed --no-legend | wc -l'
  ```
  Expect: each `active 0`, `0`. Else: `journalctl -u <unit> -n 20`. A crash-looping unit: `sudo systemctl disable --now <unit>`, tell the progress file, go on.
- [ ] **Step 212** [CLAUDE][W] ~3m: In the worktree, `monitoring/prometheus/prometheus.yml`: add `__scheme__: https` to east's `unheaded-daemon` and `dashboard-backend` targets (east's daemon port per the new unit), and put `['[fd00:dead:beef::2]:18000']` back in the `wotan` job with `__scheme__: https`. East's certificates carry the overlay address.
- [ ] **Step 213** [C] ~6m: **MERGE (rule 15): Evening 10B: Prometheus sees east on TLS.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10B: Prometheus sees east on TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10B: Prometheus sees east on TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.
- [ ] **Step 214** [CLAUDE][V] ~3m: `docker kill -s HUP unheaded-prometheus`, then the targets query from Evening 5. Expect: east's three up. Else: `lastError` says why; revert that target through the worktree.
- [ ] **Step 215** [STEVIE] ~10m: East's Heimdall half of Evening 10: the drift-key runbook for `NODE=east`. Expect: its check passes. Else: rollback in the runbook.
- [ ] **Step 216** [CLAUDE][W] ~3m: Register `FND-017` and FND-019 `resolved:`. `shred -u /var/tmp/e1-east-tls/*; rmdir /var/tmp/e1-east-tls`. `configs/` and CLAUDE.md note east's versions if they name them.
- [ ] **Step 217** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 214-217: East on current debs from the signed repo, TLS on, FND-017 closed for both hosts.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md compliance/findings/register.yaml && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 214-217: DP-23 finished: east on TLS, east Wotan back in the SLO' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 218** [V] ~1m: **EVENING 10B EXIT GATE**.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-60; curl -fsS -o /dev/null -w '%{http_code}\n' --cacert configs/certs/dev/ca.pem --resolve east:18000:192.168.13.1 https://east:18000/health
  ```
  Expect: `200`. Else: [STUCK].
- [ ] **Step 219** [C] ~6m: **MERGE (rule 15): Evening 10B: east debs and TLS.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10B: east debs and TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 10B: east debs and TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 11: EAST AS A NODE (Steps 220-233)

**Goal**: DP-206, answered: all three approved, under the guard. The S-ASCEND-2 plan ran S0-S4 across hosts on 2026-10-28, PASS; `M-228` reads "DONE in part". Tonight re-runs S-STAGE-2B's Appendix D because 10B changed east under it, and adds its one unrun leg, `cartridge-ebpf` in a netns on east. LICH-015's full arm needs a harness that does not exist yet (`diff_harness_full`) and stays open.
**Prerequisite**: Evenings 6 and 10B; S-STAGE-2B merged. After DP-22: west accepts all of east; the scratch Wotan's 26601-26602 on 192.168.13.2 are inside that rule.
**Time**: 2 h. **Have ready**: at the desk, east's keyboard and monitor in reach.

- [ ] **Step 220** [CLAUDE][S] ~15m: **Stage (S0).** Exact commands: `docs/battle-plans/S-ASCEND-2-EAST-NODE-plan.md` Stages. Builds on west as you, into `/var/tmp/s6-stage`, lab key fresh, never production keys, `AUTH_KEY_FILE` at a missing file. `scp` to `east:/tmp/s6/` and the guard to `east:/tmp/s6-guard/`; compare sha256 on both ends. Expect: equal. Else: stop, nothing is running. East's `/tmp` keeps `exec`: S-HOST-TIDY left it alone for this.
- [ ] **Step 221** [CLAUDE][S] ~1m: **Arm the guard.**
  ```bash
  ssh east 'sudo bash /tmp/s6-guard/node-guard.sh arm --lease 1200; sudo bash /tmp/s6-guard/node-guard.sh status | tail -1 | cut -c1-70'
  ```
  Expect: armed. Else: do not go on.
  **Rollback:** `ssh east 'sudo bash /tmp/s6-guard/node-guard.sh disarm'`
- [ ] **Step 222** [CLAUDE][B] ~3m: Scratch Wotan on the P2P link, lab only, auth off, **its pid recorded**: rule 14, and the 10-28 teardown missed it by path.
  ```bash
  nohup /var/tmp/s6-stage/wotan -bind 192.168.13.2 -http-port 26601 -grpc-port 26602 -topic-config /var/tmp/s6-stage/wotan-lab.yaml >/var/tmp/s6-stage/wotan.log 2>&1 & echo $! > /var/tmp/s6-stage/wotan.pid; sleep 2; ss -ltn | grep -E ':2660[12] ' | awk '{print $4}' | sort -u
  ```
  Expect: `192.168.13.2:26601`, `192.168.13.2:26602` only. Else: `kill "$(cat /var/tmp/s6-stage/wotan.pid)"`, stop.
  **Rollback:** `kill "$(cat /var/tmp/s6-stage/wotan.pid)"`
- [ ] **Step 223** [CLAUDE][S] ~1m: East's real Heimdall from 10B holds the same hooks: stop it for the run.
  ```bash
  ssh east 'sudo systemctl stop unheaded-heimdall.service; systemctl is-active unheaded-heimdall.service'
  ```
  Expect: `inactive`. Else: [STUCK], go to teardown.
  **Rollback:** `ssh east 'sudo systemctl start unheaded-heimdall.service'`
- [ ] **Step 224** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 220-224: Evening 11, checked: Easts real Heimdall from 10B holds the same hooks: stop it for the run..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 220-224: Checkpoint in Evening 11: Easts real Heimdall from 10B holds the same hooks: stop it for the run.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 225** [CLAUDE][S] ~20m: **S1, kprobes only.** Lab Heimdall as root on east, its pid written to `/tmp/s6/heimdall.pid`. Prove as the plan lists: seal verified; 20 drifts reach the scratch Wotan signed; a tampered seal stops the daemon. Else: `ssh east 'sudo kill -KILL "$(cat /tmp/s6/heimdall.pid)"'`, go to teardown.
- [ ] **Step 226** [CLAUDE][S] ~15m: **S2, XDP generic on `enp2s0`.** Renew the guard: `ssh east 'sudo bash /tmp/s6-guard/node-guard.sh renew'`. Restart heimdall with `--xdp-iface enp2s0 --xdp-mode generic`. For two minutes from west: `ping -c120 -i1 -q 192.168.13.1 | tail -2`, `sudo wg show wg0 latest-handshakes`, a fresh `ssh east true`. Expect: 0% loss, a recent handshake, login. Send the horns. Stop heimdall by pid; `ssh east 'ip link show enp2s0 | grep -c xdp'` → `0`. Else: the guard acts by itself on 10 s of lost pings. Do not fight it: if east goes dark, wait 30 s, then the console.
- [ ] **Step 227** [CLAUDE][S] ~15m: **S3, and the leg never run.** Weights publish on west, fetch on east, same `cart tick` output. Then `cartridge-ebpf` **in a netns on east, never on `enp2s0`**.
- [ ] **Step 228** [CLAUDE][S] ~5m: **S4, teardown.**
  ```bash
  kill "$(cat /var/tmp/s6-stage/wotan.pid)"; sleep 1; ss -ltn | grep -cE ':2660[12] '; ssh east 'sudo bash /tmp/s6-guard/node-guard.sh disarm; sudo rm -rf /tmp/s6 /tmp/s6-guard; pgrep -xc heimdall-daemon; ip link show enp2s0 | grep -c xdp; sudo bpftool prog show | grep -cE "heimdall|gjallarhorn|cart_"; sudo systemctl start unheaded-heimdall.service; systemctl is-active unheaded-heimdall.service'; rm -rf /var/tmp/s6-stage
  ```
  Expect: `0`, disarm line, `0`, `0`, `0`, `active`. Else: stop each leftover by pid; the emergency line below. **Abort at any point:** `ssh east 'sudo bash /tmp/s6-guard/node-guard.sh disarm; sudo kill -KILL "$(cat /tmp/s6/heimdall.pid)"; sudo ip link set dev enp2s0 xdpgeneric off'`. S-STAGE-2B's Appendix D shows a `pkill -f`: do not use it (rule 14).
- [ ] **Step 229** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 225-229: Evening 11, checked: S4, teardown..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 225-229: Checkpoint in Evening 11: S4, teardown.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 230** [CLAUDE][W] ~5m: `docs/sweeps/OUTSTANDING-2026-10-27.md` M-228; `docs/adr/ADR-043-*.md` wording; `references/s-ascend-2-log.md`, one paragraph with tonight's numbers. LICH-015's full arm stays open with its reason.
- [ ] **Step 231** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 230-231: Appendix D re-run under the guard, including cartridge-ebpf in a netns. LICH-015 full arm still needs diff_harness_full.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md docs/sweeps/OUTSTANDING-2026-10-27.md references/s-ascend-2-log.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 230-231: DP-206: east as a node re-run after the firewall, port and deb changes' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 232** [V] ~1m: **EVENING 11 EXIT GATE**.
  ```bash
  ssh east 'ip link show enp2s0 | grep -c xdp; systemctl --failed --no-legend | wc -l'
  ```
  Expect: `0`, `0`. Else: [STUCK].
- [ ] **Step 233** [C] ~6m: **MERGE (rule 15): Evening 11: east as a node.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 11: east as a node' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 11: east as a node' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 12: BROWSER AND LICH (Steps 234-243)

**Goal**: DP-505, M-231: browser xterm end to end, Lich live local-model e2e. Recipe: `wiki/Linux-on-UPC.md` "Attended end-to-end".
**Prerequisite**: Evening 5.
**Time**: 1 h. **Have ready**: a browser on the laptop; the GPU free.

- [ ] **Step 234** [CLAUDE][V] ~2m: Room for it. Never count swap.
  ```bash
  free -g | awk '/Mem:/{print "avail", $7}'; pgrep -cx llama-server; systemctl is-active unheaded-upc-netns
  ```
  Expect: `avail 4` or more, `0`, `active`. Else: free the memory first; one model at a time.
- [ ] **Step 235** [CLAUDE][B] ~2m: Bridge, pid recorded.
  ```bash
  cd /home/govan/tmp/unheaded/cmd/upc-tty-bridge && go build -o /var/tmp/upc-tty-bridge . && { nohup /var/tmp/upc-tty-bridge --listen 127.0.0.1:26100 --listen-unix /tmp/upc-tty.sock >/var/tmp/upc-tty-bridge.log 2>&1 & echo $! > /var/tmp/upc-tty-bridge.pid; }; sleep 2; ss -tln | grep -c '127.0.0.1:26100 '
  ```
  Expect: `1`. Else: read the log.
  **Rollback:** `kill "$(cat /var/tmp/upc-tty-bridge.pid)"`
- [ ] **Step 236** [STEVIE][S] ~3m: In a west terminal, the console, instance 111 (`sudo` because of the netns):
  ```bash
  cd /home/govan/tmp/unheaded/ebpf && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 UPC_TTY_BRIDGE_SOCKET=/tmp/upc-tty.sock ../cmd/upc-bootctl/target/release/upc-bootctl console --instance 111 --kernel ../crates/xv6-mbc/upstream/target/xv6-mbc.mbc --ramdisk ../crates/xv6-mbc/upstream/target/fs.img --userland ../crates/xv6-mbc/upstream/target/init.mbc
  ```
  Expect: it attaches and waits. Else: the wiki section's Options; S-STAGE-2B may have changed the build path: `ls ../cmd/upc-bootctl/target/release/upc-bootctl`.
- [ ] **Step 237** [STEVIE][V] ~5m: On the laptop: `ssh -L 26100:127.0.0.1:26100 west`, open `http://127.0.0.1:26100/upc-console.html`, instance `111`, type `echo peace and love`. **Expect:** the echo comes back. **Else:** note what the page shows; [STUCK], go on to the Lich half.
- [ ] **Step 238** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 234-238: Evening 12, checked: On the laptop: ssh -L 26100:127.0.0.1:26100 west, open http://127.0.0.1:26100/upc-console..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 234-238: Checkpoint in Evening 12: On the laptop: ssh -L 26100:127.0.0.1:26100 west, open http://127.0.0.1:26100/upc-console.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 239** [CLAUDE][B] ~10m: Lich: a model up, then the e2e, then the model down.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/switch-model.sh qwen-7b 2>&1 | tail -1 | cut -c1-80; LICH_E2E=1 go test ./pkg/lichtools/ -run E2E -count=1 2>&1 | tail -2 | cut -c1-90
  ```
  Expect: `ok  	unheaded/pkg/lichtools`. Else: `FAIL` with llama-server up is a real finding: keep the output. Then stop the model the way `scripts/switch-model.sh` says.
- [ ] **Step 240** [CLAUDE][W] ~3m: Stop the bridge by pid; `Ctrl-C` the console. `docs/sweeps/OUTSTANDING-2026-10-27.md` M-231 → DONE; `docs/battle-plans/S-ASCEND-2-battle-plan.md` Step 291 ticked with the observed lines; `wiki/Linux-on-UPC.md`: "(not run live yet)" → the date.
- [ ] **Step 241** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 239-241: Echo seen in a browser; LICH_E2E test result recorded.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md docs/sweeps/OUTSTANDING-2026-10-27.md docs/battle-plans/S-ASCEND-2-battle-plan.md wiki/Linux-on-UPC.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 239-241: DP-505: browser xterm and Lich e2e run live' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 242** [V] ~1m: **EVENING 12 EXIT GATE**.
  ```bash
  ss -tln | grep -c ':26100 '; pgrep -cx llama-server
  ```
  Expect: `0`, `0`. Else: stop what remains by pid.
- [ ] **Step 243** [C] ~6m: **MERGE (rule 15): Evening 12: browser and Lich.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 12: browser and Lich' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 12: browser and Lich' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 13: FIRST ZHEN NIGHTS (Steps 244-257)

**Goal**: DP-303: night one with a fake Claude leg, night two live at $3; cards owned by `zhen` with a ```zhen block only; `-audit-well` on; the confirm queue on; no auto-approve beyond `unheaded-baseline` and chaos-controller.
**Prerequisite**: Evening 5 live with alerts; S-ZHEN-POLISH merged; S-HOST-TIDY Phase 18 done. The batch is 044, 045, 046, 047 and 049 (Step 75 covers it if they merged first; otherwise run `scripts/well/batch-apply.sh --from 044`), then restart zhen-agentd. 047 makes the Champion's `accepted` and `denied_*` verdicts land in `zhen_actions`; before it they stay `planned`.
**Time**: 30 m now, 30 m next morning, twice. **Have ready**: one or two small cards you would be glad to see done; an Anthropic key.

- [ ] **Step 244** [STEVIE][CLAUDE][S] ~15m: Start the DP-712 units: vor on 20110, zhen-agentd on 20111. The binary comes from `make binaries`; TLS is in place since Evening 5. **First the unit's environment file, yours to type.** The unit names it without a `-` and passes `-memory-store pg`, so a missing file or a missing `WELL_APP_ZHEN_DSN` stops it at startup. The next step's `~/.config` file is not read by this unit.
  ```bash
  sudo install -m 0640 -o root -g zhen-agentd /dev/null /etc/zhen-agentd.env; sudo ${EDITOR:-nano} /etc/zhen-agentd.env; sudo stat -c '%a %U %G' /etc/zhen-agentd.env; sudo grep -c '^WELL_APP_ZHEN_DSN=.' /etc/zhen-agentd.env
  ```
  Lines: `WELL_APP_ZHEN_DSN=` (required), `WELL_DSN=`, `WELL_KANBAN_DSN=`, `VOR_URL=http://127.0.0.1:20110`, `LLAMA_URL=http://127.0.0.1:8081`, `ZHEN_CODE_INDEX_DIR=/var/zhen/code-index`. The DSN form is in the next step; `deploy/systemd/zhen-agentd.env.example` names each one's role. Expect: `640 root zhen-agentd`, `1`. Else: fix the file; do not start the unit.
  ```bash
  cd /home/govan/tmp/unheaded && GOMAXPROCS=4 make binaries 2>&1 | tail -1 | cut -c1-70; sudo install -m 0755 bin/release/zhen-agentd /usr/local/bin/zhen-agentd; sudo systemctl enable --now unheaded-vor.service zhen-agentd.service; sleep 5; systemctl is-active unheaded-vor.service zhen-agentd.service; ss -tln | grep -cE ':(20110|20111) '
  ```
  Expect: `active` twice, `2`. Else: `journalctl -u <unit> -n 20`. `vor` is built in its own repository: if `/usr/local/bin/vor` is missing, that is S-ZHEN-POLISH's handoff, [STUCK].
  **Rollback:** `sudo systemctl disable --now unheaded-vor.service zhen-agentd.service`
- [ ] **Step 244a** [STEVIE][CLAUDE][S] ~25m: **The Zhen web UI's TLS front (ADR-138, accepted 2026-11-09; step added 2026-11-10, lettered so the numbering holds).** The UI is https only, hybrid ML-KEM key exchange only, through `unheaded-zhen-ui-front.service`; the app binds a Unix socket and no TCP port. Reference: `deploy/systemd/README.md#zhen-ui-front`. If an old `zhen_app.py` still holds 20103 (`ss -tlnp | grep ':20103 '`), stop it by its pid first, not with `pkill -f`. Account, binary (from Step 244's `make binaries`), certificate from the compose CA as in Step 86:
  ```bash
  cd /home/govan/tmp/unheaded && sudo cp -p /etc/sysusers.d/unheaded.conf /var/backups/unheaded-sysusers.conf.bak-adr138 && sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers unheaded.conf; id -u unheaded-zhen-ui-front; sudo install -m 0755 bin/release/zhen-ui-front /usr/local/bin/zhen-ui-front; scripts/gen-dev-certs.sh zhen-ui-front 2>&1 | tail -1 | cut -c1-70; sudo install -d -m 0755 /etc/unheaded/tls/zhen-ui-front && sudo install -m 0644 configs/certs/dev/zhen-ui-front/zhen-ui-front.pem /etc/unheaded/tls/zhen-ui-front/zhen-ui-front.pem && sudo install -m 0640 -o root -g unheaded-zhen-ui-front configs/certs/dev/zhen-ui-front/zhen-ui-front-key.pem /etc/unheaded/tls/zhen-ui-front/zhen-ui-front-key.pem; sudo stat -c '%a %U %G' /etc/unheaded/tls/zhen-ui-front/zhen-ui-front-key.pem
  ```
  Expect: `16755`, a `Generated 1 service certificates` line, `640 root unheaded-zhen-ui-front`. Else: `permission denied` under `configs/certs/dev` means Evening 4's sudo pass owns the directory: rerun that one command with `sudo`. No CA file goes in the front's directory: its clients are browsers.
  The socket directory. Run it as the account that will run `raft/start-zhen.sh`: the line names that account as owner, and the front's own group with the setgid bit.
  ```bash
  echo "d /run/unheaded-zhen-ui 2750 $(id -un) unheaded-zhen-ui-front -" | sudo tee /etc/tmpfiles.d/unheaded-zhen-ui.conf && sudo systemd-tmpfiles --create /etc/tmpfiles.d/unheaded-zhen-ui.conf; stat -c '%a %U %G' /run/unheaded-zhen-ui
  ```
  Expect: the line echoed, then `2750 govan unheaded-zhen-ui-front`. Else: do not start the app; it refuses a directory its group or others can write, and the front refuses a socket with any bit for others.
  The unit, then the app.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -m 0644 deploy/systemd/unheaded-zhen-ui-front.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-zhen-ui-front.service; sleep 3; systemctl is-active unheaded-zhen-ui-front.service; ss -tln | grep -cE '127\.0\.0\.1:(20103|20112) '; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:20112/ready
  ```
  Expect: `active`, `2`, `503` (the app is not up yet). Else: `journalctl -u unheaded-zhen-ui-front -n 20`; a missing certificate or key stops it at startup.
  ```bash
  cd /home/govan/tmp/unheaded/raft && ./start-zhen.sh 2>&1 | tail -6 | cut -c1-100; stat -c '%a %U %G' /run/unheaded-zhen-ui/app.sock; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:20112/ready; curl -V | head -1 | cut -c1-80; cd .. && curl -s -o /dev/null -w '%{http_code}\n' --cacert configs/certs/dev/ca.pem https://localhost:20103/health; curl -s -o /dev/null -w '%{http_code}\n' --curves X25519 --cacert configs/certs/dev/ca.pem https://localhost:20103/health; curl -s http://127.0.0.1:20112/metrics | grep 'client_hellos_total{'
  ```
  Expect: `start-zhen.sh` says the unit is running and leaves it alone; `660 govan unheaded-zhen-ui-front`; `200`; the curl version; `200`; `000` (a classical-only client fails the handshake); the `hybrid` and `classical` counters, classical at 1 or more. Else: the first https line `000` with curl on OpenSSL older than 3.5 is the client, not the front: check from a current browser instead. `/ready` still `503`: the app is not on `/run/unheaded-zhen-ui/app.sock` (`start-zhen.sh` prints the socket it used).
  Then the three things that read the UI's health:
  - **Dashboard.** `config/dashboard/services.txt` has `zhenai=172.28.1.1:20112`. The front's ops listener is loopback only, so the host's gateway proxy that carries vor's 20110 has to carry 20112 too. That proxy is host configuration, not in the repository. Until it does, the `zhenai` card reads down while the UI is up.
  - **Akira.** Its `zhenai` target is `https://localhost:20103/health` through the front, checked against `UNHEADED_TLS_CA_CERT`. The compose CA issued the front's pair, so nothing more is needed while E1 uses that CA.
  - **Runbooks.** Five runbooks changed with the front and `runbooks/MANIFEST.sha256` was rewritten, not sealed: re-seal before the next live run that uses one (`scripts/runbook-manifest.sh --seal`, yours).
  **[DECIDE] LAN.** Evening 7's ruleset opens 20103 to `192.168.69.128/25`, and the shipped unit listens on loopback only. A LAN listener is a drop-in (`ExecStart=` cleared, then set whole with a second `--listen <west's LAN address>:20103`; that subnet in `IPAddressAllow=`), the name or address browsers type in `ZHEN_ALLOWED_HOSTS` in the app's environment, and a certificate that names it. `cert-gen` has no flag for extra names, so the dev pair names `localhost` only. Pre-seeded answer: stay on loopback and use `ssh -L 20103:127.0.0.1:20103 west` with `https://localhost:20103` until a certificate names the LAN address.
  **Rollback:** `sudo systemctl disable --now unheaded-zhen-ui-front.service; sudo rm -f /etc/systemd/system/unheaded-zhen-ui-front.service /etc/tmpfiles.d/unheaded-zhen-ui.conf; sudo systemctl daemon-reload; sudo rm -rf /etc/unheaded/tls/zhen-ui-front /usr/local/bin/zhen-ui-front; sudo cp -p /var/backups/unheaded-sysusers.conf.bak-adr138 /etc/sysusers.d/unheaded.conf`. This removes the front; it does not bring plain http back on 20103, which the app no longer has. Without the unit, `raft/start-zhen.sh` runs its own copy of the front as you (the dev layout).
- [ ] **Step 245** [STEVIE][W] ~5m: Your environment file, yours alone, outside the repo. Values typed in your editor, not in chat.
  ```bash
  install -D -m 0600 /dev/null ~/.config/unheaded/zhen-agentd.env; ${EDITOR:-nano} ~/.config/unheaded/zhen-agentd.env; stat -c '%a' ~/.config/unheaded/zhen-agentd.env
  ```
  Lines: `ANTHROPIC_API_KEY=...`, `WELL_APP_ZHEN_DSN=postgres://app_zhen:PASSWORD@127.0.0.1:5432/unheaded_app?sslmode=require`, `WELL_OPS_DSN=postgres://ops_writer:PASSWORD@127.0.0.1:5432/unheaded_ops?sslmode=require`, `LLAMA_URL=http://127.0.0.1:8081`, `ZHEN_TAUGHT_DIR=/var/zhen/projects/zhen-taught`. Passwords from `.env`; URL-encode any `/` or `+`. Approving a queued write is `zhen-claude confirm -allowed-roots /var/zhen/projects/default approve <id>`. Expect: `600`. Else: the unit refuses any other mode.
- [ ] **Step 246** [STEVIE] ~5m: Approve `runbooks/observe/zhen-overnight-abort.yaml` (DP-301); you will want it to exist. Kanban: make one or two cards owned by `zhen` with a ```zhen block.
- [ ] **Step 247** [CLAUDE][W] ~3m: **Night one: fake.** A drop-in replaces the unit's `-claude live`. ExecStart must be cleared, then set whole: copy it from `deploy/systemd/user/zhen-agentd-overnight.service`, with `-claude live` changed to `-claude fake`.
  ```bash
  cd /home/govan/tmp/unheaded && make build-zhen-agentd 2>&1 | tail -1 | cut -c1-60; systemctl --user link /home/govan/tmp/unheaded/deploy/systemd/user/zhen-agentd-overnight.service /home/govan/tmp/unheaded/deploy/systemd/user/zhen-agentd-overnight.timer; install -d ~/.config/systemd/user/zhen-agentd-overnight.service.d; { echo '[Service]'; echo 'ExecStart='; awk '/^ExecStart=/{f=1} f{print; if(!/\\$/) exit}' deploy/systemd/user/zhen-agentd-overnight.service | sed 's/-claude live/-claude fake/'; } > ~/.config/systemd/user/zhen-agentd-overnight.service.d/10-fake.conf; systemctl --user daemon-reload; systemctl --user cat zhen-agentd-overnight.service | grep -c -- '-claude fake'; grep -c -- '-audit-well' ~/.config/systemd/user/zhen-agentd-overnight.service.d/10-fake.conf
  ```
  Expect: `1`, `1`. Else: fix the drop-in by hand.
- [ ] **Step 248** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 244-248: Evening 13, checked: Night one: fake. A drop-in replaces the units -claude live. ExecStart must be cleared, the.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 244-248: Checkpoint in Evening 13: Night one: fake. A drop-in replaces the units -claude live. ExecStart must be cleared, the' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 249** [STEVIE][S] ~2m: Model up for local legs, linger, timer on. Tomorrow's 01:00 is the first night.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/switch-model.sh qwen-coder-14b 2>&1 | tail -1 | cut -c1-70; sudo loginctl enable-linger govan; systemctl --user enable --now zhen-agentd-overnight.timer; systemctl --user list-timers --no-legend | grep -c zhen-agentd-overnight
  ```
  Expect: `1`. Else: `scripts/switch-model.sh` names the models; the memory floor may refuse the 14B with the stack up: `qwen-7b` then. Never force it.
  **Rollback / kill switch:** `touch ~/.local/state/unheaded/zhen-agentd.stop; systemctl --user disable --now zhen-agentd-overnight.timer`
- [ ] **Step 250** [STEVIE][V] ~15m: **Next morning.** `scripts/zhen-morning-report.sh`, then `~/.local/state/unheaded/zhen-reports/<date>.md`. **Expect:** cards worked into local `zhen/<card>` branches, nothing pushed, `llm_requests` and `audit_events` rows present, no spend. **Else:** an abort reason in the report: fix it before night two; a write outside `zhen/*` is an incident: disable the timer.
- [ ] **Step 251** [STEVIE][W] ~3m: **Night two: live, $3.** Remove the drop-in; the unit's own ledger caps at $3.
  ```bash
  rm ~/.config/systemd/user/zhen-agentd-overnight.service.d/10-fake.conf; systemctl --user daemon-reload; systemctl --user cat zhen-agentd-overnight.service | grep -c -- '-claude live'
  ```
  Expect: `1`. Next morning: the same report, and the spend line at or under $3. Else: the kill switch above.
- [ ] **Step 252** [STEVIE] ~2m: After night two: keep it on, or `systemctl --user disable --now zhen-agentd-overnight.timer`, and the model down if you want the memory back. Say which.
- [ ] **Step 253** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 249-253: Evening 13, checked: Next morning. scripts/zhen-morning-report.sh, then /.local/state/unheaded/zhen-reports/dat.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 249-253: Checkpoint in Evening 13: Next morning. scripts/zhen-morning-report.sh, then /.local/state/unheaded/zhen-reports/dat' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 254** [CLAUDE][W] ~3m: `docs/battle-plans/progress/S-CHAMPION-HARDENING-approvals.md` E2 answered with the two nights' numbers. `wiki/Zhen-Agents.md`: the first-night date. ADR-106's status line: first night done.
- [ ] **Step 255** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 254-255: Night one fake, night two live under $3, audit in the Well, confirm queue on, no pushes.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md docs/battle-plans/progress/S-CHAMPION-HARDENING-approvals.md wiki/Zhen-Agents.md docs/adr/ADR-106-two-brains-one-gate.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 254-255: DP-303: first two Zhen nights run' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 256** [V] ~1m: **EVENING 13 EXIT GATE**.
  ```bash
  git -C /home/govan/tmp/unheaded for-each-ref --format='%(refname:short)' 'refs/heads/zhen/*' | wc -l
  ```
  Expect: a count you recognise. Else: [STUCK].
- [ ] **Step 257** [C] ~6m: **MERGE (rule 15): Evening 13: first Zhen nights.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 13: first Zhen nights' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 13: first Zhen nights' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 14: INTERNET-DRAFTS (Steps 258-266)

**Goal**: DP-606: the expired -00 drafts updated and resubmitted as new versions. DP-715's four spec questions go to the RFC desk here.
**Prerequisite**: S-WIRE-HONEST merged (DP-715's decisions land in the specs). Anything touching wire format passes the RFC desk.
**Time**: 2 h. **Have ready**: datatracker.ietf.org login.

- [ ] **Step 258** [STEVIE][V] ~5m: Which were uploaded? The pack says six; `ietf-submission/` holds **eight** (`anamnesis`, `mbc-isa`, `pqc-authentication`, `protocol-foundation`, `protocol-overview`, `shim`, `sophia-dictionary`, `wotan-memory`). Look at datatracker, person page: list which exist, and their last revision. **Expect:** a list. A draft never uploaded starts at `-00`; one uploaded goes up one. Do you want all eight?
- [ ] **Step 259** [CLAUDE] ~40m: Run `unheaded-rfceditor` over each chosen draft against the tree and against `docs/sweeps/TRUE-WIRE-DECISIONS.md`: the four spec questions, boilerplate, dates, references, BCP 14, DP-411's PQC text, DP-412's option registry note. Changes to the frozen v0x01 format: none, or the desk stops it.
- [ ] **Step 260** [CLAUDE][W] ~10m: New versions. For each draft to bump, copy `-00` to `-01`, set `docName`, update `ietf-submission/Makefile`'s `DRAFTS` list.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE/ietf-submission && for d in shim; do git mv draft-bellis-unheaded-$d-00.xml draft-bellis-unheaded-$d-01.xml; git mv draft-bellis-unheaded-$d-00.md draft-bellis-unheaded-$d-01.md; sed -i "s/docName=\"draft-bellis-unheaded-$d-00\"/docName=\"draft-bellis-unheaded-$d-01\"/" draft-bellis-unheaded-$d-01.xml; sed -i "s/draft-bellis-unheaded-$d-00/draft-bellis-unheaded-$d-01/g" Makefile draft-bellis-unheaded-*.xml draft-bellis-unheaded-*.md; done; make all 2>&1 | tail -2 | cut -c1-80
  ```
  Put your list of names in `for d in ...`. Expect: `=== Build complete ===`. Else: xml2rfc names the line. `idnits` is not on west: datatracker runs it at submission.
- [ ] **Step 261** [STEVIE] ~20m: Upload at datatracker.ietf.org/submit, each `.xml`. **Expect:** idnits clean or warnings only; a confirmation mail you approve. **Else:** an error names the draft and line: Claude fixes, re-upload.
- [ ] **Step 262** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 258-262: Evening 14, checked: Which were uploaded The pack says six ietf-submission/ holds eight anamnesis, mbc-isa, pqc.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 258-262: Checkpoint in Evening 14: Which were uploaded The pack says six ietf-submission/ holds eight anamnesis, mbc-isa, pqc' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 263** [CLAUDE][W] ~5m: Record URLs and date in `ietf-submission/` and M-147; note the Track call you make.
- [ ] **Step 264** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 263-264: Listed revisions submitted; URLs recorded; four DP-715 questions answered at the RFC desk.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md ietf-submission && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 263-264: DP-606: drafts resubmitted' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 265** [V] ~1m: **EVENING 14 EXIT GATE**.
  ```bash
  git -C /home/govan/tmp/wt/S-E1-LIVE status --short --untracked-files=no | wc -l
  ```
  Expect: `0`: everything is committed. Else: commit named files.
- [ ] **Step 266** [C] ~6m: **MERGE (rule 15): Evening 14: Internet-Drafts.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 14: Internet-Drafts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 14: Internet-Drafts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 15: POSTGRESQL 18 (Steps 267-275)

**Goal**: DP-03: the Well on 18 for hybrid TLS (`ssl_groups`), FND-015 closed.
**Prerequisite**: E1 stable a full week since Evening 5, and one Sunday drill green.
**Time**: 1.5 h. **Have ready**: a quiet evening. Downtime is about a backup plus two restores plus two minutes. The method is **dump and restore**, as `runbooks/data/well-pg18-upgrade.yaml` says; the pack's `pg_upgrade --link` is out of date: the official images carry one major each and the old volume stays untouched as the rollback.

- [ ] **Step 267** [CLAUDE][V] ~2m: A stable week, in numbers.
  ```bash
  systemctl show -p Result --value unheaded-well-backup.service unheaded-well-offsite.service unheaded-well-drill.service; curl -s 127.0.0.1:19130/v1/status | python3 -c "import sys,json; d=json.load(sys.stdin); print('sent today', d['sent_today'])"; curl -s 'http://127.0.0.1:9090/api/v1/query?query=ALERTS{alertstate="firing",urgency="page"}' | python3 -c "import sys,json; print('pages firing', len(json.load(sys.stdin)['data']['result']))"; docker ps --format '{{.Names}} {{.Status}}' | grep -cE 'Restarting|unhealthy'
  ```
  Expect: `success` x3, `pages firing 0`, `0`. Else: not stable: stop, next week.
- [ ] **Step 268** [STEVIE] ~3m: Approve `runbooks/data/well-pg18-upgrade.yaml` (DP-301). `postgres:18.6-alpine` is a new image pin your word admits: DP-03 is that word.
- [ ] **Step 269** [CLAUDE][B] ~15m: The automatic steps, nothing live.
  ```bash
  cd /home/govan/tmp/unheaded && python3 scripts/run-runbook.py runbooks/data/well-pg18-upgrade.yaml 2>&1 | grep -E 'pull-image|prove-in-throwaway|manifest-check|SUCCESS|FAIL' | cut -c1-80
  ```
  Expect: the three pass, and the runner prints the manual steps. Else: a hybrid FAIL means the include or the image moved: stop. If the tag has moved, re-run the proof on the new digest.
- [ ] **Step 270** [CLAUDE] ~40m: The manual steps **in order, as written in the runbook, each `verify` read aloud before the next**: `operator-env`, `ledger-status`, `silence-alerts`, `freeze-writers`, `final-backup`, `drill-on-18`, `restore-into-new-volume`, `stop-pg16`, `config-commit` (your signed commit), `start-pg18`, `role-passwords`, `verify-hybrid-live`, `verify-ledger-and-schema`, `thaw`, `backup-and-drill-on-18`. From `stop-pg16` the Well is down. Expect: `verify-hybrid-live` prints `Negotiated TLS1.3 group: X25519MLKEM768`, `Verify return code: 0 (ok)`. Else: anything after `stop-pg16`: the Rollback, which is lossless before `thaw`.
  **Rollback:** the runbook's `back-to-pg16`: `sudo docker compose stop postgres`, `git revert <config-commit>`, `sudo docker compose up -d postgres dashboard-backend kanban-app timeguru auth`, `scripts/monitoring/maintenance.sh --end`. After `thaw`, rows written on 18 must be exported first.
- [ ] **Step 271** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 267-271: Evening 15, checked: A stable week, in numbers..' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 267-271: Checkpoint in Evening 15: A stable week, in numbers.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 272** [CLAUDE][W] ~3m: FND-015 `resolved:`; `docs/policy/pq-kex-baseline.txt` drops `docker-compose.yml postgres-pre18`; regenerate the CBOM; `wiki/The-Well.md`. Not tonight: `retire-pg16`, after 14 nightly sets and a Sunday drill on 18; add it to next.md with its date.
- [ ] **Step 273** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 272-273: Dump and restore into a new volume; live port negotiates X25519MLKEM768; PG 16 volume kept as rollback until retire-pg16.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md compliance/findings/register.yaml docs/policy/pq-kex-baseline.txt compliance/crypto/cbom.cdx.json wiki/The-Well.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 272-273: DP-03: the Well on PG 18, hybrid TLS' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 274** [V] ~1m: **EVENING 15 EXIT GATE**.
  ```bash
  docker exec unheaded-postgres psql -X -At -U unheaded -d postgres -c "SELECT current_setting('server_version')"
  ```
  Expect: `18.6`. Else: [STUCK].
- [ ] **Step 275** [C] ~6m: **MERGE (rule 15): Evening 15: PG 18.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 15: PG 18' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 15: PG 18' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 16: LOG STORE, THE MEASURED WEEK (Steps 276-288)

**Goal**: round 3's answer: ClickHouse through E1, then a measured week shipping to both, Postgres favoured if close. A decision, with numbers, recorded in ADR-117.
**Prerequisite**: Evening 5. Better after Evening 15.
**Time**: 1.5 h to start, 1 h a week later. **Have ready**: Step 276 answered, ahead of the sitting if you can.

Today nothing can ship logs to Postgres: no table, and **pinned Vector 0.43.1 has no `postgres` sink** (tested with `vector list`). The Well is also the system of record, so a log flood must not reach it: capped, with an off switch.

- [x] **Step 276** [STEVIE][DECIDE] ~5m: **ANSWERED 2026-11-04: (a), "Build our own".** If Postgres wins the week, the writer stays, so build it to last: its own nologin account, tests, no throwaway shortcuts. **How the logs get there.** (a) **A small first-party writer behind Vector's `http` sink.** No third-party change, Vector stays pinned for the audit path, and it is a throwaway for one week. (b) Bump Vector to a version with a `postgres` sink: a third-party change (rule 4), and it moves the live audit pipeline for a seven-day test. Override ONLY IF you would rather bump.
- [ ] **Step 277** [CLAUDE] ~20m: Erda pass (`unheaded-erda`) on the table before anything is written: `unheaded_ops.log_events` partitioned by day, columns matching Vector's ClickHouse schema, a 14-day drop, its own login role `log_writer`, no indexes beyond time and host; or a separate database so the Well's backups do not swell. The pass names the choice.
- [x] **Step 278** [STEVIE][DECIDE] ~3m: **ANSWERED 2026-11-10: the bar as written below, "keep measuring".** Archimedes reckoned it before the data (progress file): disk and backup time are already lost for Postgres by arithmetic; the week confirms and gives the query numbers. **What counts as "close"?** Stevie, 2026-11-04: call `unheaded-archimedes` to set this bar during Evening 16 prep, before any data exists. Starting point, written down before the data: Postgres wins if over the week its disk per day is within **2x** of ClickHouse's, its p95 on the three queries below within **2x**, the Well's own p95 and backup time move under **10%**, and zero Vector buffer stalls. Override with your own numbers now, not after.
- [ ] **Step 279** [CLAUDE][W] ~60m: In the worktree, before the sitting if you can. Migration `db/migrations/048_ops_log_events.sql` (S-ZHEN-POLISH owns 044-046; 047 is `zhen_actions.status`) and its `MANIFEST` row, as Erda decided. If you chose (a): `cmd/logstore-writer`, test first: `POST /ingest` takes newline-delimited JSON, batches into one `COPY`, answers 429 past a row cap per minute, plus `/health`, `/ready`, `/metrics`, `cmdline.Parse`, `ReadHeaderTimeout`, `MemoryMax`, its own uid in `pkg/uids`, a compose service on the `data` network with no published port. A second sink in `config/vector/vector.yaml`: `type: http` to it (or `type: postgres` after the bump in (b)), buffer `when_full: drop_newest` **for this sink only**: the audit sink stays `block`. Passwords from the environment, never the file. Expect: `GOMAXPROCS=4 go test -race ./cmd/logstore-writer/` `ok`; gates green. Else: two tries, [STUCK].
- [ ] **Step 280** [C] ~6m: **MERGE (rule 15): Evening 16: migration, writer and sink reach the live checkout.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/logstore-writer/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 16: migration, writer and sink reach the live checkout' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 16: migration, writer and sink reach the live checkout' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.
- [ ] **Step 281** [CLAUDE][S] ~10m: Live: `scripts/well/batch-apply.sh`, role password by `scripts/well/set-role-passwords.sh`, `sudo docker compose up -d logstore-writer vector`. Expect: `SELECT count(*)` rising, ClickHouse's rate unchanged, `docker logs unheaded-vector` free of audit-sink stalls. Else: the off switch.
  **Rollback (off switch):** remove the sink block through the worktree, merge, `sudo docker compose up -d vector`, `sudo docker compose stop logstore-writer`; `DROP TABLE` after the week.
- [ ] **Step 282** [CLAUDE][W] ~5m: The daily reading, as a script `scripts/logstore-measure.sh` in the repo, run by you once a day or by a user timer: rows and bytes per day on each; the same three queries on each (last hour by host; a text match over 24 h; a 7-day count by unit) with timings; Vector's buffer events; `unheaded_well` p95. It appends to `docs/battle-plans/progress/S-E1-LIVE-logstore.tsv`. No log content in the file.
- [ ] **Step 283** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 281-283: Route chosen by Stevie; second sink on with drop_newest; close defined before the data.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md scripts/logstore-measure.sh && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 281-283: Log store week started: both sinks, off switch, daily reading' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 284** [STEVIE][V] ~45m: **A week later.** Read `S-E1-LIVE-logstore.tsv` with Claude against the rule you wrote. Say which store stays. **Expect:** a decision. **Else:** a broken reading: one more week.
- [ ] **Step 285** [CLAUDE][W] ~20m: Off switch for the loser. ADR-117: amendment with the numbers and the decision, parked work adjusted. If Postgres won, a migration plan is **a parked ADR, not tonight's work**: Vector to ClickHouse stays until a sprint moves it (rule: new work is parked). Register unaffected.
- [ ] **Step 286** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 284-286: Decision and numbers in ADR-117.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md docs/adr/ADR-117-one-grafana-one-metrics-store-one-log-store.md config/vector/vector.yaml docs/battle-plans/progress/S-E1-LIVE-logstore.tsv && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 284-286: Log store decided on a measured week' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 287** [V] ~1m: **EVENING 16 EXIT GATE**.
  ```bash
  git -C /home/govan/tmp/wt/S-E1-LIVE status --short --untracked-files=no | wc -l
  ```
  Expect: `0`: everything is committed. Else: commit named files.
- [ ] **Step 288** [C] ~6m: **MERGE (rule 15): Evening 16: log store.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 16: log store' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 16: log store' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

## EVENING 17: LEFTOVERS, THEN A REBOOT (Steps 289-306)

**Goal**: the DP-25 rows S-HOST-TIDY moved here: 25.3, 25.4, 25.5, 25.8 on west; then one reboot of each host to prove what was built survives it, and the RTC check of DP-609 item 7.
**Prerequisite**: every lane idle. Evenings 5, 6 and 7 done.
**Time**: 2 h. **Have ready**: at the desk; a monitor on west.

- [ ] **Step 289** [STEVIE][DECIDE] ~3m: **25.8 on west.** S-HOST-TIDY's check failed: 66 repo bind mounts read by 20 service uids. **RECOMMENDATION: accept 0002 until the bind mounts go**, a 90-day acceptance in the register; the pack's answer "027 after a check" assumed the check could pass. Override ONLY IF you want the mounts changed first. Claude adds the `accepted:` line to `host-probe:west:default-umask`.
- [ ] **Step 290** [CLAUDE][V] ~2m: **25.4.** Read S-HOST-TIDY's trial result. Green and no lane running: go on. Otherwise skip to 25.3 and say so.
  ```bash
  grep -n '25.4' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-HOST-TIDY-progress.md | cut -c1-120; pgrep -cx 'go|cargo|rustc'
  ```
  Expect: `trial green`, `0`. Else: leave `/tmp` as it is; accept with a reason.
- [ ] **Step 291** [STEVIE][S] ~5m: Environment first, then the remount, then the boot unit.
  ```bash
  install -d /var/tmp/govan-go /var/tmp/govan-tmp; go env -w GOTMPDIR=/var/tmp/govan-go; grep -q 'TMPDIR=/var/tmp/govan-tmp' ~/.profile || echo 'export TMPDIR=/var/tmp/govan-tmp' >> ~/.profile; export TMPDIR=/var/tmp/govan-tmp; sudo install -d /etc/systemd/system/tmp.mount.d; printf '%s\n' '[Mount]' 'Options=mode=1777,strictatime,nosuid,nodev,noexec,size=50%%,nr_inodes=1m' | sudo tee /etc/systemd/system/tmp.mount.d/10-noexec.conf >/dev/null; sudo systemctl daemon-reload; sudo mount -o remount,noexec /tmp; findmnt -no OPTIONS /tmp | grep -c noexec; cd /home/govan/tmp/unheaded && GOMAXPROCS=4 go test -count=1 ./pkg/cmdline/ 2>&1 | tail -1 | cut -c1-70; systemctl --user start unheaded-compliance-evidence.service; systemctl --user show -p Result --value unheaded-compliance-evidence.service
  ```
  Expect: `1`, `ok`, `success`. Else: Rollback. Anything root runs from a `mktemp` under `sudo` loses `TMPDIR`: watch tomorrow's timers.
  **Rollback:** `sudo mount -o remount,exec /tmp; sudo rm -rf /etc/systemd/system/tmp.mount.d; sudo systemctl daemon-reload; go env -u GOTMPDIR; sed -i '/TMPDIR=\/var\/tmp\/govan-tmp/d' ~/.profile`
- [ ] **Step 292** [STEVIE][S] ~10m: **25.3, east first.** East has no Docker or LXD, so it proves little about west: say so in the row.
  ```bash
  ssh east 'sudo mount -o remount,nodev,nosuid /var && findmnt -no OPTIONS /var | cut -c1-40; systemctl --failed --no-legend | wc -l; sudo -n true && echo SUDO_OK'
  ```
  Expect: `rw,nosuid,nodev,...`, `0`, `SUDO_OK`. Else: Rollback.
  **Rollback:** `ssh east 'sudo mount -o remount,dev,suid /var'`
- [ ] **Step 293** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 289-293: Evening 17, checked: 25.4. Read S-HOST-TIDYs trial result. Green and no lane running: go on. Otherwise skip to.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 289-293: Checkpoint in Evening 17: 25.4. Read S-HOST-TIDYs trial result. Green and no lane running: go on. Otherwise skip to' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 294** [STEVIE][S] ~10m: **25.3, west, live trial.** `/var` holds Docker's overlays and LXD's `dir` pool, Pi-hole included. A throwaway instance and a container prove it. If DNS flickers, Rollback at once.
  ```bash
  sudo mount -o remount,nodev,nosuid /var; docker run --rm --pull=never alpine:3.24 sh -c 'id -u; ls -l /dev/null | cut -c1-10'; /snap/bin/lxc launch steel-echo-v2 tidy-t1 --project unheaded 2>&1 | tail -1 | cut -c1-60; sleep 5; /snap/bin/lxc exec tidy-t1 --project unheaded -- ls -l /dev/null | cut -c1-10; /snap/bin/lxc delete -f tidy-t1 --project unheaded; /snap/bin/lxc restart pihole --project default; sleep 8; dig +short +time=2 @10.10.20.252 archive.ubuntu.com A | head -1; docker ps --format '{{.Status}}' | grep -c '^Up'
  ```
  Expect: `0`, `crw-rw-rw-`, an instance line, `crw-rw-rw-`, an address, the usual count. Else: **Rollback now**, then 25.3 on west takes an acceptance with this output as the reason.
  **Rollback:** `sudo mount -o remount,dev,suid /var`
- [ ] **Step 295** [STEVIE][W] ~3m: If both passed, make it permanent: fstab on both hosts. Old on west: `/dev/disk/by-uuid/d6901807-3130-4fdc-b5f2-9fe329a5dff3 /var ext4 defaults 0 1`. New: `... /var ext4 defaults,nodev,nosuid 0 1`. East's `/var` line likewise. `sudo findmnt --verify 2>&1 | tail -1`. Expect: `0 parse errors, 0 errors`. Else: restore the line. **A bad fstab stops the boot:** nothing else this evening until this prints.
- [ ] **Step 296** [STEVIE][R] ~10m: **25.5.** What purging X would take.
  ```bash
  sudo apt-get -s purge 'xserver-xorg*' 'xserver-common' 2>&1 | grep -E '^Remv' | wc -l; sudo apt-get -s purge 'xserver-xorg*' 'xserver-common' 2>&1 | grep -E '^Remv' | grep -E 'rocm|amd|hip|docker|lxd|orb|fbdev' | cut -c1-60; systemctl get-default
  ```
  Read the list with Claude. Expect: no ROCm, Docker or `orb` dependency; `graphical.target`. Else: do not purge; set only the target; note why. orb's SNES mode uses fbdev, so it should be unaffected: check it after the reboot.
- [ ] **Step 297** [STEVIE][S] ~5m: Purge and change the default, if the list was clean. Never `autoremove`.
  ```bash
  sudo DEBIAN_FRONTEND=noninteractive apt-get -y purge 'xserver-xorg*' 'xserver-common' 2>&1 | tail -1 | cut -c1-70; sudo systemctl set-default multi-user.target; systemctl get-default
  ```
  Expect: `multi-user.target`. Else: [STUCK].
  **Rollback:** `sudo systemctl set-default graphical.target; sudo apt-get -y install xserver-xorg`
- [ ] **Step 298** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 294-298: Evening 17, checked: 25.4. Read S-HOST-TIDYs trial result. Green and no lane running: go on. Otherwise skip to.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 294-298: Checkpoint in Evening 17: 25.4. Read S-HOST-TIDYs trial result. Green and no lane running: go on. Otherwise skip to' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 299** [CLAUDE][W] ~3m: Register rows for 25.3, 25.4, 25.5, 25.8. Then the reboots.
- [ ] **Step 300** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 299-300: 25.3, 25.4, 25.5, 25.8 recorded as they came out.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md compliance/findings/register.yaml && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 299-300: DP-25 leftovers: rows closed or accepted with reasons' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 301** [STEVIE][S] ~15m: **Reboot east.** Silence first.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 40m "reboot test" 2>&1 | tail -1 | cut -c1-60; ssh east 'sudo systemctl reboot'; sleep 120; ssh -o ConnectTimeout=8 east 'uptime -p; sudo nft list tables | grep -c "inet unheaded$"; ss -tln | grep -c ":7375 "; resolvectl dns enp2s0; systemctl is-active fail2ban unheaded-nft; systemctl --failed --no-legend | wc -l'
  ```
  Expect: up a couple of minutes, `1`, `2`, `... 10.10.20.252 1.1.1.1 8.8.8.8`, `active` x2, `0`. Else: east's console, then Appendix A.
- [ ] **Step 302** [STEVIE][S] ~20m: **Reboot west, at the desk.** Everything goes down for a few minutes; LAN DNS with it. Before you do, tell the household.
  ```bash
  sudo systemctl reboot
  ```
  After: log in, then Claude runs:
  ```bash
  journalctl -k -b 0 | grep 'RTC time' | cut -c1-90; uptime -s; sudo nft list tables | grep -c 'inet unheaded$'; /snap/bin/lxc list pihole --project default -c s --format csv; cd /home/govan/tmp/unheaded && sudo scripts/kingdom-startup.sh --force --docker-only 2>&1 | tail -4 | cut -c1-70; docker ps --format '{{.Status}}' | grep -c '^Up'; curl -s 127.0.0.1:19130/health | cut -c1-40
  ```
  Expect: the RTC line against `uptime -s` (DP-609 item 7: if 6 h apart, look in the BIOS); `1`; `RUNNING`; startup complete; the usual count; healthy. Else: Appendix A3.
- [ ] **Step 303** [CLAUDE][V] ~10m: End to end after a cold start: first-party health over TLS, a backup, heartbeat, offsite tomorrow.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-60; for p in 18000 19000 20000 20001; do curl -s -o /dev/null -m4 -w "$p %{http_code}\n" --cacert configs/certs/dev/ca.pem https://localhost:$p/health; done; sudo systemctl start unheaded-well-backup.service; systemctl show -p Result --value unheaded-well-backup.service unheaded-well-offsite.service; ssh east 'systemctl show -p Result --value unheaded-heartbeat.service'
  ```
  Expect: `200` x4, `success` x3 (read the offsite line again in ten minutes: it runs after the backup). Else: name it in the progress file: this is what the reboot was for.
- [ ] **Step 304** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && printf '%s\n' '- Steps 301-304: East and west rebooted; rulesets from unheaded-nft; RTC reading recorded; stack up through kingdom-startup; backups green.' >> docs/battle-plans/progress/S-E1-LIVE-progress.md && git add docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -q -m '[PLAN S-E1-LIVE] Steps 301-304: Reboot proof: both hosts came back with firewalls, sshd, resolvers and the stack' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 305** [V] ~2m: **EVENING 17 EXIT GATE. S-E1-LIVE DONE.**
  ```bash
  grep -c 'STUCK' /home/govan/tmp/wt/S-E1-LIVE/docs/battle-plans/progress/S-E1-LIVE-progress.md
  ```
  Expect: a number you have read each line of. Fold the progress file into `~/tmp/next.md`.
- [ ] **Step 306** [C] ~6m: **MERGE (rule 15): Evening 17: leftovers and the reboot proof.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-E1-LIVE && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-e1-live' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./cmd/alert-mailer/ ./cmd/alert-policy/ && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 17: leftovers and the reboot proof' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-e1-live -m '[PLAN S-E1-LIVE] Evening 17: leftovers and the reboot proof' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-E1-LIVE] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.

---

## Appendix A — Emergency procedures

**A1. You lost SSH to east after a firewall or port change.** Wait: the firewall rollback is 10 minutes, the sshd undo 30. Then `ssh -p 22 -o HostKeyAlias=192.168.13.1 east true` as well as `ssh east true`. Still out: east's keyboard: `sudo nft delete table inet unheaded; sudo rm -f /etc/ssh/sshd_config.d/05-unheaded-port.conf; sudo systemctl daemon-reload; sudo systemctl restart ssh.socket`.

**A2. You lost SSH to west.** Wait 10 minutes for the firewall timer, or 5 for an sshd one. West's console: `sudo /usr/local/lib/unheaded/nft-apply-confirm.sh rollback`, or `sudo nft delete table inet unheaded`; `sudo rm -f /etc/ssh/sshd_config.d/05-unheaded-keyonly.conf /etc/ssh/sshd_config.d/00-unheaded-kex.conf; sudo sshd -t && sudo systemctl reload ssh`. Docker's and LXD's tables are not touched by any of this.

**A3. The stack will not come up.** One service at a time: `docker compose ps`, `docker logs --tail 30 <name>`. A missing key or certificate stops a service by design: Evening 4's steps are safe to re-run (an existing key is never overwritten). Memory: `free -g`, never count swap; stop the monitoring Grafana and Loki first. The Well's data is in `unheaded-dev_postgres-data`; if a container shows an empty Well, **stop it** before anything writes.

**A4. A migration failed.** Do not re-run. `bash scripts/well/batch-rollback.sh --yes <the printed pre-batch path>`, fix the file, rehearse, new evening.

**A5. Alert mail runs away.** The caps hold at 3 an hour and 6 a day. To stop it at once: `cd ~/tmp/unheaded/monitoring && sudo docker compose stop alert-mailer`. Read `curl -s 127.0.0.1:19130/v1/outbox` first if it answers. A real leak of the app password: revoke it at myaccount.google.com/apppasswords.

**A6. A YubiKey or a disk is lost.** One key left: rotate at the next ceremony, enrolling a replacement. West's disk lost: restore `/var/backups/unheaded-custody/buildkey.luks` from east and open it with either key. Both keys lost: the seed is gone by design. New key under a new name, revocation entry in `SECURITY.md`, as the runbook says; old casks stay verifiable by the public key.

**A7. A secret reached a file, chat or commit.** Rotate it first; then remove it. If committed and not pushed: Claude rewrites the unpushed commit; if pushed: `runbooks/security/git-history-secret-purge.yaml`.

**A8. Zhen does something you did not expect.** `touch ~/.local/state/unheaded/zhen-agentd.stop; systemctl --user disable --now zhen-agentd-overnight.timer`. Its work is in local `zhen/*` branches; nothing was pushed.

## Appendix B — Order and dependencies

| Evening | Needs | Blocks |
|---|---|---|
| 0 | none | 5, 6, 7, 9, 10, 15 |
| 1, 2, 3 | 0 | 4 by your order; nothing technical except 3's `.env` values |
| 4 | 0, 3 | 5 |
| 5 | 4 | 6-17 |
| 6 | 5, S-HOST-TIDY 8-9 | 9, 11 |
| 7 | 5 + 2 quiet days | 17 |
| 8 | helps after 7 | none |
| 9 | 0, 6 | none |
| 10, 10B | 5 | FND-017 east, FND-018, FND-019, 11 |
| 11 | 6, 10B, S-STAGE-2B | none |
| 12 | 5 | none |
| 13 | 5, S-ZHEN-POLISH, S-HOST-TIDY 18 | none |
| 14 | S-WIRE-HONEST | none |
| 15 | 5 + a week + a drill | none |
| 16 | 5 | none |
| 17 | 5, 6, 7, lanes idle | none |

Critical path to "live with alerts": 0 → 3 → 4 → 5.

---

*S-E1-LIVE Battle Plan — Forged 2026-11-04*
*19 Sittings. 306 Steps. Every door opened with the owner in the room, every lock changed with the old key still in his hand.*
*Sit down, read the step, do the step. Peace and love.*
