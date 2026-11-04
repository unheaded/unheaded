<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-HOST-TIDY progress

## For Stevie

- West's fail2ban bans port 22 only (`port = ssh`, sshd is on 7375): `.132` was banned 2026-10-29 and logged in on 7375 anyway. Left alone; S-E1-LIVE Evening 8 carries the fix.
- 25.2: usb-storage is blacklisted on west too. This departs from the recorded answer (row 25.2 option B, accept on west) because B's only reason was an offline USB copy and DP-06 chose YubiKeys. Undo: the Rollback line of the usb-storage apply step.
- `scripts/east-bootstrap.sh` lines 18 and 233 still name `root@`: stale, not run by anything, not changed here.
- Test east's password once by hand from west, it cannot be tested unattended: `ssh -o PubkeyAuthentication=no -o ControlPath=none east true`
- Plan typo, Step 2: the check `head -3 | tail -1` reads line 3 (blank); the heredoc puts `# S-HOST-TIDY progress` on line 2. The file matches the plan text exactly (checked with `sed -n 2p`), so the step counted as passed and the file was not bent to fit. The check should be `sed -n 2p`.
- DP-708 not done, Phase 6 [STUCK] at Step 59 and rolled back; east is on `1.1.1.1 8.8.8.8` as before. The list took (`DNS Servers: 10.10.20.252 1.1.1.1 8.8.8.8`) but `Current DNS Server` stayed `1.1.1.1`, so the sinkhole name got a real address. Likely cause, not tested: `1.1.1.1` was already current (`resolvectl.before` shows it), it stayed in the new list, and systemd-resolved keeps a current server that is still listed; its journal was empty, so this was no failover. Step 58 has nothing that moves it to the first server. The restart in Step 61 might, but the plan does not name it for this failure, so it was not tried. Decide: add a tested step that moves resolved to the first server after Step 58, then rerun Steps 57-63. Proofs 1-4 passed and need no change.
- Plan text, Steps 60 and 62: both are worded for success (Step 62 says the netplan yaml and live link are set), so neither was run as written after Step 59 failed. One [STUCK] commit stands in for them.
- DP-708 retry: the resolver fix worked, but Step 59's first check failed as written, so Phase 6 is [STUCK] again and rolled back; east is on `1.1.1.1 8.8.8.8`. With Pi-hole set alone first, `Current DNS Server` moved to `10.10.20.252` and stayed there after the full list went on, so the first run's cause is confirmed. The miss: `resolvectl query` prints the A and AAAA answers in varying order, and `head -1` read `::` where Expect says `0.0.0.0`. Measured before rollback: A `0.0.0.0` and AAAA `::`, through the stub and direct from Pi-hole; a later full query printed `0.0.0.0` first. A rerun could have passed on ordering alone, so it was neither rerun nor waived. Decide: change that check to `resolvectl query -t A o401343.ingest.us.sentry.io`, or accept `::` as well, then rerun Steps 57-63.
- Plan text, Step 58, if the change is kept: after Pi-hole is set alone, `resolvectl status` shows no `Current DNS Server` line until a lookup happens. The step needs one lookup before that check (`resolvectl query archive.ubuntu.com`).
- East after DP-12: `apt-get -s autoremove` now offers 106 packages (among them `jq`, `tmux`, `htop`, `fwupd`, `open-vm-tools`), so a hand-run `apt autoremove` would take them. None is on Step 66's reachability list, and no `Unattended-Upgrade::Remove*` key is set on east. Left in place, as the plan says.
- Plan text, Step 67: it labels the commit and evidence line `Steps 63-67`, but Phase 7 is Steps 64-68 and Step 63 belongs to Phase 6. Run as written.
- DP-11 first half and DP-15 not done on east: Phase 8 [STUCK] at Step 69, nothing changed. Step 69's last check counts `root@` lines outside `scripts/east-bootstrap.sh`; Expect `0`, got `1`: `runbooks/data/well-restore-from-east-offsite.yaml:56`. It is the known line, not a new one, but Step 22 was meant to move it to `govan` first and Phase 2 has not run on this branch, so the runbook still says `root@east`. Phase 8's Prerequisite names only the hold session; the critical path has Phase 2 before it. The other four checks passed. Reading the line as "not new" would be waiving a gate in front of a live sshd change, so it was neither waived nor fixed here (Step 22 is Phase 2's). Decide: run Phase 2, or at least Step 22, then rerun Phase 8 from Step 69; or rule that this one line may stay and Step 69 may expect `1`. Phases 9, 11's east half and 13 wait on this.
- Plan text, Step 75: it labels the evidence line and commit `Steps 68-75`, but Phase 8 is Steps 69-76 and Step 68 is Phase 7's exit gate.

## Evidence

- Steps 1-5: Phase 0, checked: Back up wests configs..
- Steps 6-7: Worktree made; open count saved; config backups on west and east.
- Steps 8-11: Preflight done. Backups on west and east, hold session open, FACTS GATE passed.
- Steps 12-16: Phase 1, checked: The register still parses and its tests pass..
- Steps 17-18: 17 rows accepted until 2027-02-02, east password rows carry DP-11, stub deleted.
- Steps 51-55: Phase 6, checked: PROOF 3  it is really Pi-hole answering: a denied name returns its sinkhole, and a public.
- Steps 56-59: Phase 6 [STUCK] at Step 59. Proof 4 passed (`1`, `blocked=0`); Steps 57 and 58 matched Expect. Step 59 got `o401343.ingest.us.sentry.io: 34.160.81.0`, `Current DNS Server: 1.1.1.1` (Expect `0.0.0.0`, `10.10.20.252`); `archive.ubuntu.com` resolved, apt errors `0`, chrony `1`, `FRESH_KEY_LOGIN_OK`.
- Rollback run and checked: `/etc/netplan/01-enp2s0.yaml` matches the backup (`cmp`), `DNS=1.1.1.1`, `DNS=8.8.8.8`, `Link 2 (enp2s0): 1.1.1.1 8.8.8.8`, stub kept, names resolve, chrony `1`, fresh key login and hold session up.
- Step 63 exit gate: FAILED, `Link 2 (enp2s0): 1.1.1.1 8.8.8.8`. Steps 60 and 62 not run as written. Phase 7 accepts this [STUCK].
- Steps 56-59 retry: Step 58 changed by the coordinator: `sudo resolvectl dns enp2s0 10.10.20.252` first, confirm `Current DNS Server`, then the step's command. No resolved restart, no `netplan apply`. Added beyond that instruction: `Current DNS Server` was absent after the single-server set, so one `resolvectl query archive.ubuntu.com` ran, and the line then read `10.10.20.252`.
- Step 56 passed (`1`, `blocked=0`). Step 57 matched Expect. Step 58 matched Expect, `Current DNS Server: 10.10.20.252` with all three listed.
- Step 59 [STUCK]: got `o401343.ingest.us.sentry.io: ::` (Expect `0.0.0.0 ...`). The other five matched: `archive.ubuntu.com: 91.189.91.82`, apt errors `0`, `Current DNS Server: 10.10.20.252`, chrony `1`, `FRESH_KEY_LOGIN_OK`.
- Rollback run and checked: `/etc/netplan/01-enp2s0.yaml` matches the backup (`cmp`), `DNS=1.1.1.1`, `DNS=8.8.8.8`, `Link 2 (enp2s0): 1.1.1.1 8.8.8.8`, `Current DNS Server: 1.1.1.1`, stub kept, names resolve, chrony `1`, fresh key login and hold session up.
- Step 63 exit gate: FAILED, `Link 2 (enp2s0): 1.1.1.1 8.8.8.8`. Steps 60 and 62 not run as written; this commit stands in for them. Phase 7 accepts this [STUCK].
- Steps 63-67: lxd-installer and the ubuntu-server metapackage removed; no autoremove run; gid 101 kept.
- Step 69 [STUCK]: `Master running (pid=3290473)`, journal from `2026-10-26`, root logins `0`, `NO_ROOT_KEYS`, `root@` count `1` (Expect `0`): the restore runbook, line 56. Steps 70-76 not run; undo timer never armed.
- East unchanged, checked: `/etc/ssh/sshd_config.d` matches the backup (`diff -r`), `unheaded-tidy-sshd-undo.timer` inactive, `sshd -T` still `port 22`, `permitrootlogin without-password`, `passwordauthentication no`, `kbdinteractiveauthentication no`. Hold session left up.
