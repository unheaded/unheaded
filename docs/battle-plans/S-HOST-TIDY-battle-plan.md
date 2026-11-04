<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-HOST-TIDY BATTLE PLAN — 19 Phases, 136 Steps

**Date**: 2026-11-04
**Sprint**: S-HOST-TIDY — live host changes that are safe unattended: every one reversible, none able to lock anyone out
**Prerequisite**: `docs/battle-plans/SPRINT-SERIES-2026-11-04.md` rules 1-17 read; develop checked out at `~/tmp/unheaded`; `sudo -n true` works on west and on east (both are `(ALL) NOPASSWD: ALL`, measured 2026-11-04); `ssh east true` works from west.
**Target**: DP-09, DP-11 (password login back on east, fail2ban on east; not the port), DP-12, DP-15 (`PermitRootLogin no`), DP-708, DP-711, DP-712 (units and paths, disabled), DP-713 (the image), DP-723 follow-up and the safe rows of DP-25 done on the hosts, each with evidence in `docs/battle-plans/progress/S-HOST-TIDY-progress.md`.
**Estimated Duration**: 4-5 hours in slices between merges; no single host phase runs over 30 minutes.
**Branch**: `sprint/s-host-tidy` in the worktree `/home/govan/tmp/wt/S-HOST-TIDY`. It merges to develop with `git merge --no-ff -S` at the Phase 17 gate. Never `git push`.
**Agent Strategy**: the coordinator runs this plan between merges (series Order). Rule 13: every phase that runs as root, edits the root-run fix model or touches a live host is `**Model**: Opus`; only register and doc edits are cheaper (Phases 1 and 16, Haiku). One live-host agent per host at a time. Stop line for every model: anything unpredicted on a live host means roll back with the step's Rollback line, mark `[STUCK]`, record the output in the progress file, go to the next independent step. A Haiku agent that finds it needs root or a host stops and marks `[STUCK]`.
**Commit Cadence**: a [C] step every 3-5 steps on the sprint branch, never inside an open undo window (gpg delay must not eat the timer); each appends one evidence line to the progress file. Format `[PLAN S-HOST-TIDY] Steps X-Y: ...` plus the two trailers. Stage named files only.
**Bound by**: series rules 1-17 (`docs/battle-plans/SPRINT-SERIES-2026-11-04.md`), which win over anything here.
**Stuck Protocol**: 3x the estimate or two failed debug attempts, mark `[STUCK]`, commit, move on; questions go under "For Stevie" in the progress file, oldest first; never stop to ask.
**Scope fence**: no firewall apply, no sshd port or KEX change, no key generation, no `compose up`, no reboot (rule 5). West's fail2ban jail bans the wrong port (below) and stays untouched here: fixing it unattended could really ban Stevie's laptop. New ideas become parked ADRs.

## WHAT THE HOSTS SAID (measured read-only 2026-11-04, develop `fdd8f931`)

| Pack or brief assumed | Measured | Effect on this plan |
|---|---|---|
| DP-09: journals have no cap | both hosts already carry `/etc/systemd/journald.conf.d/60-unheaded.conf`, 1G/200M, since 2026-10-13; both journals sit at 1G | west gets a second drop-in raising it to 4G/500M; east is already at target, verify only |
| DP-09: two world-writable ROCm logs | both are `0640 root:root`, dirs `0755` (fixed 2026-10-13) | verify only |
| DP-09: retire `remediate.sh` | retired 2026-10-27 (`d4cc446b`); what is left is a 6-line stub that exits 2 | delete the stub; the "Managed by scripts/host/remediate.sh" comments stay on purpose (`fixes.yaml` header: they are in the hashed file content) |
| 25.1: `unheaded-remediate -record-existing` | the binary is on neither host, `/usr/share/unheaded/remediation/fixes.yaml` is on neither host, and no `journal.jsonl` exists; only the 2026-10-01 backup directories | Phase 3 installs binary and a root-owned model on both hosts first. As root it refuses a model in the repo: the tree is group-writable |
| `journald-caps`, `kmod-cis` are editable | both are pinned by `cmd/unheaded-remediate/testdata/parity` goldens, and `Fix.SHA256()` hashes the note too | new fixes `journald-caps-west` and `kmod-usb-storage` with their own files; `kmod-cis` is not touched at all, its stale note stays; two tests count fixes (16 to 18); Ansible regenerated |
| DP-11: remove `10-unheaded-keys-only.conf` | that file also holds `PermitRootLogin prohibit-password`. With it gone: password `yes` (from `50-cloud-init.conf`), keyboard-interactive `yes` (east's main `sshd_config` line 85), root falls to the default | the same window adds `05-unheaded-permitroot.conf`; expected methods become `publickey,password,keyboard-interactive` |
| DP-11: fail2ban "same package as west" | package 1.1.0-8, one package, `python3-systemd` already on east. The sshd filter already matches `sshd-session`, and in a journalmatch `+` means OR, so Ubuntu's default sees it too | east's jail names `sshd-session` explicitly; harmless and clear |
| West has working fail2ban | it works, on the wrong port: `port = ssh` is 22, sshd is on 7375. It banned `.132` on 2026-10-29 at 03:24, and `.132` logged in on 7375 a minute later while banned. No ignoreip | the fix belongs in S-E1-LIVE with Stevie present, since it would make a typo ban real. Filed under For Stevie |
| East fail2ban guards something | every source east can see is west (192.168.13.2, overlay, link-local); the LAN cannot start connections to east | those must all be in `ignoreip` or one ban closes east's only door. The jail is live and sees failures, but every source it can see today is ignored; it is depth for a routing change, and the plan says so |
| DP-12: `apt remove lxd-installer` is one package | `apt-get -s` removes **`ubuntu-server` too** (metapackage that depends on it) and leaves a long autoremove list | no `autoremove`, ever. Nothing critical is on that list (`openssh-server` is manual; chrony, netplan, wireguard, sudo, bird2 are not listed) |
| DP-15: nothing logs in as root | `root` is locked (`L`) on both, `/root/.ssh/authorized_keys` is empty on both, 0 root logins in what the journals hold (west from 2026-10-23, east from 2026-10-26; `last` is not installed): west 33 logins all `govan` by keyboard-interactive, east 644 all `govan` by publickey. One documented path names root: `runbooks/data/well-restore-from-east-offsite.yaml` line 56, dead already because root has no key | Phase 2 moves that runbook to a `govan` tar pipe. DP-05's future key is for `govan@east`, not root |
| `ssh.service` | `ssh.socket` is the enabled unit on both hosts, `ssh.service` is disabled but active, `KillMode=process` | `systemctl reload ssh` applies config here; the port move in S-E1-LIVE needs the socket path |
| DP-708 | east: 5/5 ping to 10.10.20.252 via 192.168.13.2 at 0.1 ms; `dig` UDP and TCP answer; netplan file `/etc/netplan/01-enp2s0.yaml` holds `[1.1.1.1, 8.8.8.8]`; stub in place | proof steps are written to pass. `netplan apply` is banned here: enp2s0 is east's only link |
| 25.2 | no USB disk on either host (`lsblk`: sata and nvme only), `usb_storage` not loaded on either | blacklist on both, by the coordinator's ruling. This departs from the recorded answer ("all thirteen rows as recommended" makes 25.2 option B, accept on west, for an offline USB copy); DP-06 replaced that copy with YubiKeys, which are not mass storage. For Stevie line in Step 2 |
| 25.8 | `UMASK` is absent from `/etc/login.defs` on both; session umask `0002`; `USERGROUPS_ENAB yes`, so `UMASK 027` gives `0007` for govan. West's compose stacks bind 60 + 6 repo paths into containers running as 20 service uids, and new files at 027 would be unreadable to them | **check fails on west**, passes on east: east only |
| 25.4 | `/tmp` is `rw,nosuid,nodev` tmpfs; 8 scripts `go build` into a `mktemp -d` and run the result; tests exec fakes from `t.TempDir()`; lanes hold environments set before any change | trial in a private mount namespace only; host change deferred to S-E1-LIVE even if the trial is green |
| 25.3, 25.5 | `/var` holds Docker and LXD's `dir` pool, including the live Pi-hole; X purge has no dry run here | doubt: both go to S-E1-LIVE |
| DP-711 | `steel-echo-v2-0/1/2` RUNNING in project `unheaded`, proxy device on 127.0.0.1:26601, image `steel-echo-v2` stays in the store | exported, then deleted |
| DP-713 | `rocm/vllm:latest` `e26f3c1684f3`, 30.7 GB, no container uses it; `rocm/pytorch:latest` is separate and stays | remove by name |
| DP-723 | the three denied hosts answer `0.0.0.0` from Pi-hole | verify only |
| East side finding, not in the pack | `unheaded-wotan` on east crash-loops on a bad flag, `NRestarts=100212`; `unheaded-timeguru` too (old 1.0.0 debs); `unheaded-well-offsite-check` and `unheaded-heartbeat` fail until the Well is up | left alone (S-E1-LIVE). The crash loop is what fills east's 1G journal |

## WHY NOTHING HERE CAN LOCK ANYONE OUT

| Item | Why it cannot lock anyone out |
|---|---|
| DP-09 journald, ROCm, stub | journald and file modes are not on any login path; west's journal only grows |
| DP-11 password on east | adds a method; the key path that all 644 logins use is unchanged and proven by a fresh login before the undo timer is stopped |
| DP-11 fail2ban on east | every address west can use toward east is in `ignoreip`, written before the package is installed; a provoked failure from west proves the ignore |
| DP-12 lxd-installer | a stub for a snap east never runs; sshd, sudo, networking are untouched and the simulation lists exactly two packages |
| DP-15 `PermitRootLogin no` | root cannot log in today (locked, no keys, 0 logins in the journals' window), so nobody loses a path; the one runbook naming `root@east` moves to `govan`; undo timer armed |
| DP-708 resolver | sshd is reached by IP and does not need DNS; a dead resolver costs names only; public fallbacks stay in the list; netplan is not applied, so enp2s0 is never bounced |
| DP-711, DP-713 | LXD project `unheaded` and an unused image; Pi-hole is in project `default` and is checked afterwards |
| 25.1 | `-record-existing` writes a journal line and changes nothing else |
| 25.2 usb-storage | no boot or data disk is USB, nothing is unloaded, and keyboards, mice and YubiKeys are HID or CCID, not mass storage |
| 25.4 trial | a private mount namespace dies with the command |
| 25.8 umask (east) | only the mode of files created afterwards; authentication never reads it |
| 25.9 pam_wheel | `su` is already useless to non-root (root is locked); `sudo` and `sudo-i` use their own PAM services, which do not include `su`, and sudo is proven before and after; console rescue uses `sulogin`; undo timer armed |
| 25.10, 25.11 | text in a register |

## LEGEND
[B] bash · [V] verify (must pass) · [D] debug (only on failure) · [W] write/edit file · [R] read · [S] sudo · [P] parallelizable · [C] commit checkpoint · [DECIDE] decision with a pre-seeded answer · [STUCK] skipped, needs a human · [BLOCKED] blocked by an upstream [STUCK]

Code blocks sit inside list items: remove the two leading spaces of every line before running them (heredoc terminators must reach column 0). Every live-change step carries **Rollback:**. **Else:** says what to do on any output other than **Expect:**. Paths: worktree `/home/govan/tmp/wt/S-HOST-TIDY`; backups `/var/backups/unheaded-host-tidy-2026-11-04` on each host, root 0700; hold socket `/run/user/1000/host-tidy-east.sock`; `lxc` is `/snap/bin/lxc`. Never use `pkill -f`.

## SWARM TABLE

| Runs together | Agent | Files or host | Note |
|---|---|---|---|
| Phase 1 and Phase 2 | Haiku, Opus | `register.yaml`, `scripts/host/` / `fixes.yaml`, the two tests, `deploy/ansible/`, `wiki/Security.md` | disjoint files, one worktree; `git commit` one at a time. Phase 2 counts as one builder; Phase 1 is doc-only |
| Phase 10 (west) beside Phases 7-9 (east) | Opus, Opus | west / east | allowed; the coordinator may also run them serially |
| Phase 14 alone | Opus | west | needs no other builder running |

Critical path: 0 → 2 → 3 → 6 → 7 → 8 → 9 → 11 → 12 → 13 → 15 → 16 → 17 (about 3 hours).

---

## PHASE 0: PREFLIGHT AND BACKUPS (Steps 1-12)

**Goal**: worktree, progress file, a config backup on each host, an open session to east, and proof the hosts still match the table above.
**Prerequisite**: none.
**Time**: 15m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 1** [B] ~1m: Create the worktree.
  ```bash
  git -C /home/govan/tmp/unheaded worktree add -b sprint/s-host-tidy /home/govan/tmp/wt/S-HOST-TIDY develop 2>&1 | tail -1; ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects
  ```
  Expect: `HEAD is now at <hash> ...`. Else: if `a branch named 'sprint/s-host-tidy' already exists`, rerun without `-b`; anything else, [STUCK].
- [ ] **Step 2** [W] ~1m: Create the progress file.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && cat > docs/battle-plans/progress/S-HOST-TIDY-progress.md <<'EOF'
  <!-- SPDX-License-Identifier: GPL-3.0-or-later -->
  # S-HOST-TIDY progress

  ## For Stevie

  - West's fail2ban bans port 22 only (`port = ssh`, sshd is on 7375): `.132` was banned 2026-10-29 and logged in on 7375 anyway. Left alone; S-E1-LIVE Evening 8 carries the fix.
  - 25.2: usb-storage is blacklisted on west too. This departs from the recorded answer (row 25.2 option B, accept on west) because B's only reason was an offline USB copy and DP-06 chose YubiKeys. Undo: the Rollback line of the usb-storage apply step.
  - `scripts/east-bootstrap.sh` lines 18 and 233 still name `root@`: stale, not run by anything, not changed here.
  - Test east's password once by hand from west, it cannot be tested unattended: `ssh -o PubkeyAuthentication=no -o ControlPath=none east true`

  ## Evidence

  EOF
  head -3 docs/battle-plans/progress/S-HOST-TIDY-progress.md | tail -1
  ```
  Expect: `# S-HOST-TIDY progress`. Else: [STUCK].
- [ ] **Step 3** [B] ~2m: Findings baseline for rule 16: the header's open count, kept for the close-out.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && install -d /var/tmp/host-tidy && go run ./cmd/compliance-evidence findings -evidence /home/govan/tmp/unheaded/var/compliance/evidence.json | grep -m1 -oE '[0-9]+ open' | head -1 | tee /var/tmp/host-tidy/open-before
  ```
  Expect: a line such as `154 open`. Else: a much smaller number means the evidence path is wrong: [STUCK].
- [ ] **Step 4** [B][S] ~1m: Back up west's configs.
  ```bash
  sudo install -d -m 0700 /var/backups/unheaded-host-tidy-2026-11-04 && sudo cp -a /etc/ssh/sshd_config.d /etc/pam.d/su /etc/login.defs /etc/systemd/journald.conf.d /etc/modprobe.d /var/backups/unheaded-host-tidy-2026-11-04/ && sudo sh -c 'sshd -T > /var/backups/unheaded-host-tidy-2026-11-04/sshd-T.before' && sudo ls /var/backups/unheaded-host-tidy-2026-11-04 | wc -l
  ```
  Expect: `6`. Else: [STUCK] and stop the plan: no backup, no host change.
  **Rollback:** `sudo rm -rf /var/backups/unheaded-host-tidy-2026-11-04`
- [ ] **Step 5** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 1-5: Phase 0, checked: Back up wests configs..' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 1-5: Checkpoint in Phase 0: Back up wests configs.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 6** [B][S] ~1m: Back up east's configs.
  ```bash
  ssh east 'sudo install -d -m 0700 /var/backups/unheaded-host-tidy-2026-11-04 && sudo cp -a /etc/ssh/sshd_config.d /etc/pam.d/su /etc/login.defs /etc/systemd/journald.conf.d /etc/modprobe.d /etc/netplan /var/backups/unheaded-host-tidy-2026-11-04/ && sudo sh -c "sshd -T > /var/backups/unheaded-host-tidy-2026-11-04/sshd-T.before; resolvectl status > /var/backups/unheaded-host-tidy-2026-11-04/resolvectl.before" && sudo ls /var/backups/unheaded-host-tidy-2026-11-04 | wc -l'
  ```
  Expect: `8`. Else: [STUCK] and skip every east phase.
  **Rollback:** `ssh east 'sudo rm -rf /var/backups/unheaded-host-tidy-2026-11-04'`
- [ ] **Step 7** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 6-7: Worktree made; open count saved; config backups on west and east.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 6-7: Preflight: worktree, findings baseline, backups on both hosts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 8** [B] ~1m: Open the session that stays up through every east change. A later plain `ssh east` makes a fresh login; `-S` rides this one.
  ```bash
  ssh -fNM -o ServerAliveInterval=30 -S /run/user/1000/host-tidy-east.sock east; ssh -S /run/user/1000/host-tidy-east.sock -O check east
  ```
  Expect: `Master running (pid=<n>)`. Else: [STUCK]; Phases 8, 9 and 11's east half do not run without it. If it dies later, rerun this step before the next east sshd or PAM step.
- [ ] **Step 9** [V] ~1m: **FACTS GATE** — east still matches the table.
  ```bash
  ssh east 'sudo sshd -T | grep -E "^(port|permitrootlogin|passwordauthentication|kbdinteractiveauthentication) "; ls /etc/ssh/sshd_config.d/; sudo sh -c "test ! -s /root/.ssh/authorized_keys && test ! -e /root/.ssh/authorized_keys2 && echo NO_ROOT_KEYS"; sudo passwd -S root | cut -d" " -f2; dpkg-query -W -f="\${Status}\n" fail2ban 2>&1 | tail -1 | cut -c1-40; dpkg-query -W -f="\${Status}\n" lxd-installer; sudo grep "addresses: \[" /etc/netplan/01-enp2s0.yaml'
  ```
  Expect, in order: `port 22`, `permitrootlogin without-password`, `passwordauthentication no`, `kbdinteractiveauthentication no`; the files `00-unheaded-kex.conf 00-unheaded.conf 10-unheaded-keys-only.conf 50-cloud-init.conf 60-unheaded-well-offsite.conf`; `NO_ROOT_KEYS`; `L`; a line saying fail2ban is not installed; `install ok installed`; a line with `[1.1.1.1, 8.8.8.8]`. Else: the row that differs names a phase whose assumptions moved: mark that phase [STUCK], continue with the others.
- [ ] **Step 10** [V] ~1m: **FACTS GATE** — west still matches the table.
  ```bash
  sudo sshd -T | grep -E '^(port|permitrootlogin|passwordauthentication|kbdinteractiveauthentication) '; ls /etc/ssh/sshd_config.d/; sudo sh -c 'test ! -s /root/.ssh/authorized_keys && test ! -e /root/.ssh/authorized_keys2 && echo NO_ROOT_KEYS'; sudo passwd -S root | cut -d' ' -f2; docker images rocm/vllm --format '{{.ID}}'; /snap/bin/lxc list --project unheaded -c n --format csv
  ```
  Expect, in order: `port 7375`, `permitrootlogin without-password`, `passwordauthentication no`, `kbdinteractiveauthentication yes`; `50-cloud-init.conf 99-unheaded.conf`; `NO_ROOT_KEYS`; `L`; `e26f3c1684f3`; `steel-echo-v2-0`, `-1`, `-2`. Else: as the step above.
- [ ] **Step 11** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 8-11: Preflight done. Backups on west and east, hold session open, FACTS GATE passed.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 8-11: Preflight: backups on both hosts, hold session, facts match' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 12** [V] ~1m: **PHASE 0 EXIT GATE** — the backup directory exists on both hosts and the hold session answers.
  ```bash
  sudo test -d /var/backups/unheaded-host-tidy-2026-11-04 && ssh -S /run/user/1000/host-tidy-east.sock east 'sudo test -d /var/backups/unheaded-host-tidy-2026-11-04 && echo PHASE0_OK'
  ```
  Expect: `PHASE0_OK`. Else: do not start host phases.

## PHASE 1: REGISTER ROWS AND THE STUB (Steps 13-19)

**Goal**: DP-25 rows 25.10 and 25.11 accepted in the register, east's password rows carry the DP-11 reason, the retired stub is gone.
**Prerequisite**: Phase 0.
**Time**: 15m
**Agent**: Agent [P]
**Model**: Haiku

- [ ] **Step 13** [W] ~2m: Accept 25.10 (2 rows) and 25.11 (15 rows). The script refuses to touch a row that is already accepted.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && python3 - <<'PY'
  import re
  p = 'compliance/findings/register.yaml'
  t = open(p).read()
  comp = 'reachable only from the home LAN and the west-east P2P link; the ISP edge drops unsolicited inbound (FND-004)'
  a10 = '    accepted: {reason: "DP-25 row 25.10, Stevie 2026-11-04: these are Ubuntu\'s flags=(unconfined) profiles, which only grant user namespaces to named programs; moving them changes those programs", compensating: "%s", accepted_on: 2026-11-04, until: 2027-02-02}\n' % comp
  a11 = '    accepted: {reason: "DP-25 row 25.11, Stevie 2026-11-04: CIS L2 separate mounts need a partition and east has no unallocated disk (sda is all /var, sdb is /boot/efi and /); options on these mounts come with the separate filesystems", compensating: "east is reachable only through west and runs no user workloads", accepted_on: 2026-11-04, until: 2027-02-02}\n'
  rows = {'host-probe:west:apparmor-profiles-enforced': a10, 'host-probe:east:apparmor-profiles-enforced': a10}
  for m in ('home-separate', 'home-nodev', 'home-nosuid',
            'var-log-separate', 'var-log-nodev', 'var-log-noexec', 'var-log-nosuid',
            'var-log-audit-separate', 'var-log-audit-nodev', 'var-log-audit-noexec', 'var-log-audit-nosuid',
            'var-tmp-separate', 'var-tmp-nodev', 'var-tmp-noexec', 'var-tmp-nosuid'):
      rows['host-probe:east:mount-' + m] = a11
  n = 0
  for src, acc in rows.items():
      m = re.search(r'  - source: "' + re.escape(src) + r'"\n((?:    .*\n|      .*\n)+)', t)
      assert m, src
      blk = m.group(0)
      assert 'accepted:' not in blk, src
      new = re.sub(r'^    decision: .*\n', '', blk, flags=re.M)
      new = re.sub(r'^    plan: ', lambda _: acc + '    plan: ', new, count=1, flags=re.M)
      t = t.replace(blk, new)
      n += 1
  open(p, 'w').write(t)
  print(n, 'rows accepted')
  PY
  grep -c '^    accepted:' compliance/findings/register.yaml
  ```
  Expect: `17 rows accepted`, then `79` (62 before). Else: `git checkout -- compliance/findings/register.yaml`, [STUCK], record the assertion text.
- [ ] **Step 14** [W] ~1m: East's two password rows: the reason is now DP-11, the date stays.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && python3 - <<'PY'
  import re
  p = 'compliance/findings/register.yaml'
  t = open(p).read()
  old = 'Stevie 2026-10-01: lab environment, SSH and passwords stay as they are; reported, not changed'
  new = 'DP-11, Stevie 2026-11-03: there is no laptop key, so east keeps password login beside keys, with fail2ban and a key preferred'
  n = 0
  for src in ('host-sshd:east:passwordauthentication=no', 'host-sshd:east:kbdinteractiveauthentication=no'):
      m = re.search(r'  - source: "' + re.escape(src) + r'"\n((?:    .*\n)+)', t)
      assert m and old in m.group(0), src
      t = t.replace(m.group(0), m.group(0).replace(old, new)); n += 1
  open(p, 'w').write(t)
  print(n, 'rows updated')
  PY
  ```
  Expect: `2 rows updated`. Else: `git checkout -- compliance/findings/register.yaml`, redo the step above, [STUCK] on this one.
- [ ] **Step 15** [V] ~2m: The register still parses and its tests pass.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && python3 -c "import yaml; print(len(yaml.safe_load(open('compliance/findings/register.yaml'))['findings']))" && GOMAXPROCS=4 go test ./cmd/compliance-evidence/ 2>&1 | tail -1; go run ./cmd/compliance-evidence findings -evidence /home/govan/tmp/unheaded/var/compliance/evidence.json | grep -m1 -oE '[0-9]+ open' | head -1; cat /var/tmp/host-tidy/open-before
  ```
  Expect: a number; `ok  	unheaded/cmd/compliance-evidence ...`; two equal `N open` lines (an accepted row is still counted, rule 16). Else: `git checkout -- compliance/findings/register.yaml`, [STUCK]; a Haiku agent does not re-run the host side.
- [ ] **Step 16** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 12-16: Phase 1, checked: The register still parses and its tests pass..' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 12-16: Checkpoint in Phase 1: The register still parses and its tests pass.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 17** [W] ~1m: Delete the stub; nothing runs it.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && git grep -n 'scripts/host/remediate\.sh' -- '*.sh' Makefile '.github/*' 'compliance/baseline/*' 'compliance/catalog/*' | grep -v '^scripts/host/remediate.sh:' | wc -l && git rm -q scripts/host/remediate.sh && ls scripts/host/
  ```
  Expect: `0`, then `nft-apply-confirm.sh`. Else: any count above 0 means something still names it: `git checkout HEAD -- scripts/host/remediate.sh`, [STUCK], list the hits in the progress file.
- [ ] **Step 18** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 17-18: 17 rows accepted until 2027-02-02, east password rows carry DP-11, stub deleted.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md compliance/findings/register.yaml && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 17-18: Register: 25.10, 25.11 accepted, DP-11 reasons; remediate.sh stub deleted' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 19** [V] ~1m: **PHASE 1 EXIT GATE**.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && git show --stat --format= HEAD | grep -c 'register.yaml\|remediate.sh'
  ```
  Expect: `2`. Else: [STUCK].

## PHASE 2: TWO NEW FIXES IN THE MODEL (Steps 20-28)

**Goal**: `journald-caps-west` and `kmod-usb-storage` exist in the model, Ansible regenerated, tests green. The parity-pinned fixes stay byte-identical. Opus because the model runs as root (rule 13).
**Prerequisite**: Phase 0.
**Time**: 25m
**Agent**: Agent [P]
**Model**: Opus

- [ ] **Step 20** [W] ~3m: Add both fixes. `kmod-cis` is not edited: its hash covers the note, and changing it would mark the recorded fix as changed.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && python3 - <<'PY'
  p = 'compliance/remediation/fixes.yaml'
  t = open(p).read()
  add = '''  - id: journald-caps-west
      title: West's larger journal caps while the journal is its only log store (DP-09)
      controls: [UH-LOG-02]
      closes: []
      risk: low
      class: service
      note: >-
        DP-09, Stevie 2026-11-03: west 4G/500M, east stays at 1G/200M. A second drop-in, because the file journald-caps
        writes is pinned by the parity goldens. journald reads 60- then 61- and the last value wins.
      actions:
        - file:
            path: /etc/systemd/journald.conf.d/61-unheaded-west.conf
            mode: "0644"
            parts:
              - text: |
                  # DP-09: west's caps; overrides 60-unheaded.conf (last value wins).
                  [Journal]
                  SystemMaxUse=4G
                  RuntimeMaxUse=500M
        - run: "systemctl restart systemd-journald.service"

    - id: kmod-usb-storage
      title: USB mass storage cannot load (CIS 1.1.1.9)
      controls: [UH-HOST-02]
      closes: ["host-probe:*:kmod-usb-storage-disabled"]
      risk: none
      class: kmod
      note: >-
        DP-25 row 25.2, both hosts: no disk on either host is USB (lsblk 2026-11-04), and custody uses YubiKeys (DP-06),
        which are HID and CCID devices, not storage. Its own file because kmod-cis is pinned by the parity goldens.
      actions:
        - file:
            path: /etc/modprobe.d/unheaded-usb-storage.conf
            mode: "0644"
            parts:
              - text: |
                  # DP-25 row 25.2 / CIS Ubuntu 24.04 1.1.1.9.
                  install usb-storage /bin/false
                  blacklist usb-storage

'''
  anchor = '  - id: time-p2p\n'
  assert t.count(anchor) == 1
  t = t.replace(anchor, add + anchor)
  open(p, 'w').write(t)
  print('fixes added')
  PY
  ```
  Expect: `fixes added`. Else: `git checkout -- compliance/remediation/fixes.yaml`, [STUCK].
- [ ] **Step 21** [W] ~1m: The two tests that count fixes, and the wiki, say 18.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && sed -i 's/strings.Count(out, "\\n") != 16 || !strings.Contains(out, "aide-install")/strings.Count(out, "\\n") != 18 || !strings.Contains(out, "aide-install")/' cmd/unheaded-remediate/main_test.go && sed -i 's/if len(m.Fixes) != 16 {/if len(m.Fixes) != 18 {/; s/"%d fixes, want 16 (the 13 remediate.sh carried, plus rocm-log-perms, aide-install, apparmor-enforce)"/"%d fixes, want 18 (the 13 remediate.sh carried, plus rocm-log-perms, aide-install, apparmor-enforce, journald-caps-west, kmod-usb-storage)"/' pkg/compliance/remediation/model_test.go && sed -i 's/(`compliance\/remediation\/fixes.yaml`, 16 fixes)/(`compliance\/remediation\/fixes.yaml`, 18 fixes)/' wiki/Security.md && grep -c '!= 18 ||' cmd/unheaded-remediate/main_test.go && grep -c 'want 18' pkg/compliance/remediation/model_test.go && grep -c '18 fixes' wiki/Security.md
  ```
  Expect: `1`, `1`, `1`. Else: `git checkout -- cmd/unheaded-remediate/main_test.go pkg/compliance/remediation/model_test.go wiki/Security.md`, [STUCK].
- [ ] **Step 22** [W] ~2m: DP-15: the restore runbook stops needing root on east. It is in the manifest, so regenerate that.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && python3 - <<'PY'
  p = 'runbooks/data/well-restore-from-east-offsite.yaml'
  t = open(p).read()
  old = '      sudo scp -rp root@east:"/var/backups/unheaded-well-west/sets/${SET}" "${STAGE}/$(dirname "${SET}")/"\n'
  new = "      ssh east \"sudo tar -C /var/backups/unheaded-well-west/sets -cf - '${SET}'\" | sudo tar -C \"${STAGE}\" -xf -\n"
  assert t.count(old) == 1
  t = t.replace(old, new)
  oldc = '      # On west, as root (the offsite account on east is add-only and cannot read back):\n'
  newc = '      # On west, as govan (the offsite account on east is add-only and cannot read back):\n'
  assert t.count(oldc) == 1
  open(p, 'w').write(t.replace(oldc, newc))
  print('runbook updated')
  PY
  scripts/runbook-manifest.sh --write 2>&1 | tail -1 | cut -c1-70; git status --short runbooks | wc -l; git grep -nE 'root@(east|\$\{?EAST|fd00)' -- scripts deploy runbooks | cut -d: -f1,2
  ```
  Expect: `runbook updated`, a manifest line, `2`, then only `scripts/east-bootstrap.sh:18` and `:233`. Else: `git checkout -- runbooks`, [STUCK].
- [ ] **Step 23** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 19-23: Phase 2, checked: DP-15: the restore runbook stops needing root on east. It is in the manifest, so regenerat.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 19-23: Checkpoint in Phase 2: DP-15: the restore runbook stops needing root on east. It is in the manifest, so regenerat' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 24** [B] ~2m: Regenerate Ansible from the model.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && go run ./cmd/compliance-evidence ansible 2>&1 | tail -2; git status --short deploy/ansible | cut -c1-70
  ```
  Expect: `25 files written to deploy/ansible/compliance` (dry-run on an export of develop, 2026-11-04), and the status lists only ` M .../profile-all.yml`, `?? .../roles/journald-caps-west/`, `?? .../roles/kmod-usb-storage/`. Else: `git checkout -- deploy/ansible && git clean -fd deploy/ansible`, [STUCK], record the list.
- [ ] **Step 25** [V] ~5m: Tests, including parity.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && gofmt -l pkg/compliance cmd/unheaded-remediate; GOMAXPROCS=4 go test -race ./pkg/compliance/... ./cmd/unheaded-remediate/ ./cmd/compliance-evidence/ 2>&1 | grep -vE '^ok|no test files' | head -20; echo DONE
  ```
  Expect: `DONE` alone. Else: Step 26 first.
- [ ] **Step 26** [D] ~5m: If a failure names `closes`, `class`, `controls` or an allowlist, read the assertion and compare the new blocks with `journald-caps` (lines above `time-p2p`) field by field. A parity failure means `journald-caps` or `kmod-cis` content moved: `git diff compliance/remediation/fixes.yaml` must show only the two new blocks. Two attempts, then [STUCK]; Phases 3 (journald) and 4 are then [BLOCKED].
- [ ] **Step 27** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 24-27: Two fixes added; parity goldens unchanged; restore runbook off root@east; tests green.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md compliance/remediation/fixes.yaml runbooks/data/well-restore-from-east-offsite.yaml runbooks/MANIFEST.sha256 cmd/unheaded-remediate/main_test.go pkg/compliance/remediation/model_test.go wiki/Security.md deploy/ansible/compliance/profile-all.yml deploy/ansible/compliance/roles/journald-caps-west deploy/ansible/compliance/roles/kmod-usb-storage && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 24-27: Model: journald-caps-west and kmod-usb-storage, Ansible regenerated' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 28** [V] ~1m: **PHASE 2 EXIT GATE** — the new fixes list.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && go run ./cmd/unheaded-remediate -model compliance/remediation/fixes.yaml list | grep -cE '^(journald-caps-west|kmod-usb-storage) '
  ```
  Expect: `2`. Else: [STUCK].

## PHASE 3: REMEDIATE INSTALLED; JOURNALD AND ROCM (Steps 29-37)

**Goal**: `unheaded-remediate` and a root-owned model on both hosts; west's journal at 4G/500M; east's and the ROCm logs proven already right.
**Prerequisite**: Phase 2.
**Time**: 20m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 29** [B] ~2m: Build the tool outside `/tmp`.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && install -d /var/tmp/host-tidy && GOMAXPROCS=4 go build -o /var/tmp/host-tidy/unheaded-remediate ./cmd/unheaded-remediate && /var/tmp/host-tidy/unheaded-remediate --version
  ```
  Expect: `unheaded-remediate (unheaded) <version>`. Else: [STUCK].
- [ ] **Step 30** [B][S] ~1m: Install on west.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && sudo install -d -m 0755 -o root -g root /usr/share/unheaded /usr/share/unheaded/remediation && sudo install -m 0644 -o root -g root compliance/remediation/fixes.yaml /usr/share/unheaded/remediation/fixes.yaml && sudo install -m 0755 -o root -g root /var/tmp/host-tidy/unheaded-remediate /usr/local/sbin/unheaded-remediate && sudo unheaded-remediate list | wc -l
  ```
  Expect: `18`. Else: [D] if `refusing the model`, a parent is group-writable: `stat -c '%a %U %n' /usr/share/unheaded /usr/share/unheaded/remediation`, expect `755 root`. Then [STUCK].
  **Rollback:** `sudo rm -rf /usr/share/unheaded/remediation /usr/local/sbin/unheaded-remediate`
- [ ] **Step 31** [B][S] ~2m: Install on east.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && scp -q /var/tmp/host-tidy/unheaded-remediate compliance/remediation/fixes.yaml east:/tmp/ && ssh east 'sudo install -d -m 0755 -o root -g root /usr/share/unheaded /usr/share/unheaded/remediation && sudo install -m 0644 -o root -g root /tmp/fixes.yaml /usr/share/unheaded/remediation/fixes.yaml && sudo install -m 0755 -o root -g root /tmp/unheaded-remediate /usr/local/sbin/unheaded-remediate && rm -f /tmp/fixes.yaml /tmp/unheaded-remediate && sudo unheaded-remediate list | wc -l'
  ```
  Expect: `18`. Else: as above.
  **Rollback:** `ssh east 'sudo rm -rf /usr/share/unheaded/remediation /usr/local/sbin/unheaded-remediate'`
- [ ] **Step 32** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 28-32: Phase 3, checked: Install on east..' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 28-32: Checkpoint in Phase 3: Install on east.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 33** [B][S] ~2m: DP-09: raise west's caps through the tool, so the change is journaled and has a backup.
  ```bash
  set -o pipefail; sudo unheaded-remediate plan journald-caps-west 2>&1 | tail -4; sudo unheaded-remediate -model-commit "$(git -C /home/govan/tmp/wt/S-HOST-TIDY rev-parse HEAD)" apply journald-caps-west 2>&1 | tail -3; echo rc=$?
  ```
  Expect: the plan names `/etc/systemd/journald.conf.d/61-unheaded-west.conf`; `rc=0`. Else: run the Rollback, [STUCK].
  **Rollback:** `sudo sh -c 'unheaded-remediate undo "$(ls -dt /var/lib/unheaded/remediate/*-journald-caps-west | head -1)"' && sudo systemctl restart systemd-journald.service`
- [ ] **Step 34** [V] ~1m: West's journald took the new caps and is healthy.
  ```bash
  systemd-analyze cat-config systemd/journald.conf | grep -E '^(SystemMaxUse|RuntimeMaxUse)='; systemctl is-active systemd-journald; sudo unheaded-remediate check journald-caps; sudo unheaded-remediate check journald-caps-west; logger -t host-tidy journald-proof-1 && sleep 1 && journalctl -t host-tidy -n 1 --no-pager | tail -1 | cut -c1-60
  ```
  Expect: `SystemMaxUse=1G`, `RuntimeMaxUse=200M`, `SystemMaxUse=4G`, `RuntimeMaxUse=500M`; `active`; `journald-caps on west: in place`; `journald-caps-west on west: in place`; a line ending `journald-proof-1`. Else: run Step above's Rollback, [STUCK].
- [ ] **Step 35** [V] ~1m: East is already at 1G/200M: no change. ROCm logs are already right: no change.
  ```bash
  ssh east 'grep -E "^(SystemMaxUse|RuntimeMaxUse)=" /etc/systemd/journald.conf.d/60-unheaded.conf'; stat -c '%a %U:%G %n' /var/log/rocm_smi_lib/ROCm-SMI-lib.log /var/log/amd_smi_lib/AMD-SMI-lib.log
  ```
  Expect: `SystemMaxUse=1G`, `RuntimeMaxUse=200M`, then two lines starting `640 root:root`. Else: for a different mode run `sudo unheaded-remediate apply rocm-log-perms` and re-run this step; for different caps [STUCK].
- [ ] **Step 36** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 33-36: unheaded-remediate on west and east; west caps 4G/500M; east already 1G/200M; ROCm logs already 0640.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 33-36: DP-09: west journald 4G/500M through unheaded-remediate; tool on both hosts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 37** [V] ~1m: **PHASE 3 EXIT GATE**.
  ```bash
  sudo test -s /var/lib/unheaded/remediate/journal.jsonl && sudo tail -1 /var/lib/unheaded/remediate/journal.jsonl | grep -c journald-caps-west
  ```
  Expect: `1`. Else: [STUCK]; a missing journal is a defect in the tool, keep the change, tell Stevie.

## PHASE 4: USB STORAGE (Steps 38-42)

**Goal**: DP-25 row 25.2: `usb-storage` cannot load on west or east.
**Prerequisite**: Phase 3.
**Time**: 10m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 38** [V] ~1m: No disk on either host is USB, and the module is not loaded.
  ```bash
  lsblk -dno TRAN | grep -c usb; lsmod | grep -cE '^(usb_storage|uas) '; ssh east 'lsblk -dno TRAN | grep -c usb; lsmod | grep -cE "^(usb_storage|uas) "'
  ```
  Expect: `0`, `0`, `0`, `0`. Else: any 1 means a USB disk or the module is live: skip this phase on that host, [STUCK]. Never blacklist under a mounted USB disk.
- [ ] **Step 39** [B][S] ~1m: Apply on east, then west. New loads are refused at once; nothing is unloaded; no reboot.
  ```bash
  C="$(git -C /home/govan/tmp/wt/S-HOST-TIDY rev-parse HEAD)"; ssh east "sudo unheaded-remediate -model-commit $C apply kmod-usb-storage" 2>&1 | tail -1; sudo unheaded-remediate -model-commit "$C" apply kmod-usb-storage 2>&1 | tail -1
  ```
  Expect: two lines that report the fix applied, exit 0. Else: Rollback on the host that failed, [STUCK].
  **Rollback:** `sudo sh -c 'unheaded-remediate undo "$(ls -dt /var/lib/unheaded/remediate/*-kmod-usb-storage | head -1)"'` (on east inside `ssh east '...'`)
- [ ] **Step 40** [V] ~1m: Both refuse the module; USB input devices stay.
  ```bash
  modprobe -n -v usb-storage; sudo unheaded-remediate check kmod-usb-storage; lsusb | grep -cE '046d:(c063|c34b)'; ssh east 'modprobe -n -v usb-storage; sudo unheaded-remediate check kmod-usb-storage'
  ```
  Expect: `install /bin/false`, `kmod-usb-storage on west: in place`, `2` (mouse and keyboard), `install /bin/false`, `kmod-usb-storage on east: in place`. Else: Rollback on that host, [STUCK].
- [ ] **Step 41** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 37-41: usb-storage install /bin/false on west and east; no USB disks; input devices unaffected.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 37-41: DP-25 row 25.2: usb-storage refused on both hosts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 42** [V] ~1m: **PHASE 4 EXIT GATE** — Step above passed for both hosts or the host is marked [STUCK] in the progress file.
  ```bash
  grep -c 'usb-storage' /home/govan/tmp/wt/S-HOST-TIDY/docs/battle-plans/progress/S-HOST-TIDY-progress.md
  ```
  Expect: `1` or more. Else: finish the commit step.

## PHASE 5: IMAGE, STEEL ECHO, VIZIO (Steps 43-51)

**Goal**: DP-713 image gone, DP-711 instances gone with an export kept, DP-723 denies confirmed.
**Prerequisite**: Phase 0.
**Time**: 20m
**Agent**: Agent [P]
**Model**: Opus

- [ ] **Step 43** [V] ~1m: Nothing uses the image, and Cuirass is stopped so nothing recreates the instances mid-step.
  ```bash
  docker ps -a --filter ancestor=rocm/vllm:latest --format '{{.Names}}' | wc -l; docker image inspect rocm/vllm:latest --format '{{index .RepoDigests 0}}'; docker ps -a --filter name='^unheaded-cuirass$' --format '{{.Status}}' | cut -c1-6
  ```
  Expect: `0`, `rocm/vllm@sha256:e7f02dd2ce3824959658bc0391296f6158638e3ebce164f6c019c4eca8150ec7`, `Exited`. Else: a container or a different digest: skip the image, [STUCK]. Cuirass running: skip the instances, [STUCK].
- [ ] **Step 44** [B] ~2m: DP-713: remove the image, by name only. `rocm/pytorch:latest` stays.
  ```bash
  df -h --output=used /var | tail -1; docker rmi rocm/vllm:latest 2>&1 | tail -1 | cut -c1-30; df -h --output=used /var | tail -1
  ```
  Expect: a `Deleted: sha256:` line, the used figure lower by about 30 GB. Else: [STUCK].
  **Rollback:** `docker pull rocm/vllm@sha256:e7f02dd2ce3824959658bc0391296f6158638e3ebce164f6c019c4eca8150ec7` (30.7 GB; only if Stevie asks)
- [ ] **Step 45** [V] ~1m: Gone, and its neighbour is not.
  ```bash
  docker images rocm/vllm --format '{{.ID}}' | wc -l; docker images rocm/pytorch --format '{{.ID}}'
  ```
  Expect: `0`, `652f5092ddb7`. Else: [STUCK].
- [ ] **Step 46** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 42-46: Phase 5, checked: Gone, and its neighbour is not..' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 42-46: Checkpoint in Phase 5: Gone, and its neighbour is not.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 47** [B] ~3m: DP-711: stop and export each instance under `~/tmp` (the lxc snap can write there).
  ```bash
  install -d -m 0700 /home/govan/tmp/host-tidy-2026-11-04 && for i in 0 1 2; do /snap/bin/lxc stop steel-echo-v2-$i --project unheaded && /snap/bin/lxc export steel-echo-v2-$i /home/govan/tmp/host-tidy-2026-11-04/steel-echo-v2-$i.tar.gz --project unheaded 2>&1 | tail -1; done; ls /home/govan/tmp/host-tidy-2026-11-04 | wc -l
  ```
  Expect: `Backup exported successfully!` three times, then `3`. Else: `/snap/bin/lxc start steel-echo-v2-$i --project unheaded` for each stopped one, [STUCK].
  **Rollback:** `for i in 0 1 2; do /snap/bin/lxc start steel-echo-v2-$i --project unheaded; done`
- [ ] **Step 48** [B] ~1m: Delete them, only if each export reads back. Cuirass recreates from image `steel-echo-v2` on demand.
  ```bash
  bad=0; for i in 0 1 2; do tar -tzf /home/govan/tmp/host-tidy-2026-11-04/steel-echo-v2-$i.tar.gz >/dev/null || { echo BAD_EXPORT $i; bad=1; }; done; test $bad = 0 && for i in 0 1 2; do /snap/bin/lxc delete steel-echo-v2-$i --project unheaded; done; /snap/bin/lxc list --project unheaded -c n --format csv | wc -l; ss -tln | grep -c ':26601 '
  ```
  Expect: no `BAD_EXPORT`, `0`, `0`. Else: `BAD_EXPORT`: stop before the delete loop runs, start the instances again, [STUCK].
  **Rollback:** `for i in 0 1 2; do /snap/bin/lxc import /home/govan/tmp/host-tidy-2026-11-04/steel-echo-v2-$i.tar.gz --project unheaded && /snap/bin/lxc start steel-echo-v2-$i --project unheaded; done`
- [ ] **Step 49** [V] ~1m: Pi-hole untouched; the image store still holds v2; DP-723 denies stand.
  ```bash
  /snap/bin/lxc list pihole --project default -c s --format csv; /snap/bin/lxc image list steel-echo-v2 -c l --format csv; for d in kinesis.us-west-2.amazonaws.com firehose.us-east-2.amazonaws.com o401343.ingest.us.sentry.io; do dig +short +time=2 @10.10.20.252 "$d" A | head -1; done
  ```
  Expect: `RUNNING`, `steel-echo-v2`, `0.0.0.0` three times. Else: Pi-hole not `RUNNING`: `/snap/bin/lxc start pihole --project default` and tell Stevie in the progress file; a real address: the deny was lost, [STUCK], do not re-add it here.
- [ ] **Step 50** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 47-50: rocm/vllm removed (digest in plan). steel-echo-v2 x3 deleted, exports in ~/tmp/host-tidy-2026-11-04. Vizio denies answer 0.0.0.0.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 47-50: DP-713, DP-711, DP-723: image removed, steel-echo-v2 x3 exported and deleted, denies confirmed' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 51** [V] ~1m: **PHASE 5 EXIT GATE**.
  ```bash
  docker images rocm/vllm --format '{{.ID}}' | wc -l; /snap/bin/lxc list --project unheaded -c n --format csv | wc -l
  ```
  Expect: `0`, `0`. Else: [STUCK].

## PHASE 6: EAST RESOLVER THROUGH PI-HOLE (Steps 52-63)

**Goal**: DP-708: east asks 10.10.20.252 first, then 1.1.1.1, then 8.8.8.8, through the `127.0.0.53` stub, proven before and after the switch.
**Prerequisite**: Phase 0; Phase 5 finished or not running (it holds west's LXD, where Pi-hole lives).
**Time**: 25m
**Agent**: Coordinator
**Model**: Opus

Why it cannot lock anyone out: sshd is reached at 192.168.13.1 by address, so a dead resolver costs names only; the public servers stay in the list; `netplan apply` is forbidden here because enp2s0 is east's only link. The YAML changes what the next boot uses; `resolvectl` changes the running link without touching it.

- [ ] **Step 52** [V] ~1m: **PROOF 1** — the path to Pi-hole goes through west.
  ```bash
  ssh east 'ip route get 10.10.20.252 | head -1; ping -c5 -W1 10.10.20.252 | tail -2 | head -1'
  ```
  Expect: `10.10.20.252 via 192.168.13.2 dev enp2s0 src 192.168.13.1 uid 1000`, then `5 packets transmitted, 5 received, 0% packet loss, ...`. Else: Phase 6 [STUCK], nothing changed, skip to Phase 7.
- [ ] **Step 53** [V] ~2m: **PROOF 2** — twenty UDP and twenty TCP queries, no failure. This is the flake DP-708 exists for.
  ```bash
  ssh east 'f=0; for i in $(seq 1 20); do dig +short +time=2 +tries=1 @10.10.20.252 archive.ubuntu.com A | grep -q . || f=$((f+1)); dig +short +tcp +time=2 +tries=1 @10.10.20.252 security.ubuntu.com A | grep -q . || f=$((f+1)); done; echo failures=$f'
  ```
  Expect: `failures=0`. Else: Phase 6 [STUCK], nothing changed.
- [ ] **Step 54** [V] ~1m: **PROOF 3** — it is really Pi-hole answering: a denied name returns its sinkhole, and a public resolver does not.
  ```bash
  ssh east 'dig +short +time=2 +tries=1 @10.10.20.252 o401343.ingest.us.sentry.io A | head -1; dig +short +time=3 +tries=1 @1.1.1.1 o401343.ingest.us.sentry.io A | tail -1 | grep -c "^0\.0\.0\.0$"'
  ```
  Expect: `0.0.0.0`, then `0`. Else: [STUCK], nothing changed.
- [ ] **Step 55** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 51-55: Phase 6, checked: PROOF 3  it is really Pi-hole answering: a denied name returns its sinkhole, and a public.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 51-55: Checkpoint in Phase 6: PROOF 3  it is really Pi-hole answering: a denied name returns its sinkhole, and a public' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 56** [V] ~1m: **PROOF 4** — time keeps working: east's selected source is found by name, so each configured name must resolve through Pi-hole.
  ```bash
  ssh east 'chronyc -n sources | grep -c "^\^\*"; b=0; for n in $(grep -hE "^(pool|server) " /etc/chrony/chrony.conf /etc/chrony/sources.d/*.sources 2>/dev/null | awk "{print \$2}" | grep -vE "^[0-9.]+$|^west$"); do a=$(dig +short +time=2 +tries=1 @10.10.20.252 "$n" A | tail -1); case "$a" in ""|0.0.0.0) b=$((b+1));; esac; done; echo blocked=$b'
  ```
  Expect: `1`, `blocked=0` (measured 2026-11-04). Else: a blocked name: [STUCK], nothing changed.
- [ ] **Step 57** [W][S] ~2m: Change the boot config. `netplan generate` only rewrites `/run/systemd/network`; it restarts nothing.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo sed -i "s/addresses: \[1\.1\.1\.1, 8\.8\.8\.8\]/addresses: [10.10.20.252, 1.1.1.1, 8.8.8.8]/" /etc/netplan/01-enp2s0.yaml && sudo grep -n "addresses: \[" /etc/netplan/01-enp2s0.yaml && sudo netplan generate && sudo grep -E "^DNS=" /run/systemd/network/10-netplan-enp2s0.network'
  ```
  Expect: a line with `addresses: [10.10.20.252, 1.1.1.1, 8.8.8.8]`, then `DNS=10.10.20.252`, `DNS=1.1.1.1`, `DNS=8.8.8.8`. Else: Rollback, [STUCK].
  **Rollback:** `ssh -S /run/user/1000/host-tidy-east.sock east 'sudo cp -p /var/backups/unheaded-host-tidy-2026-11-04/netplan/01-enp2s0.yaml /etc/netplan/01-enp2s0.yaml && sudo netplan generate && sudo resolvectl dns enp2s0 1.1.1.1 8.8.8.8'`
- [ ] **Step 58** [B][S] ~1m: Apply to the running link.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo resolvectl dns enp2s0 10.10.20.252 1.1.1.1 8.8.8.8 && resolvectl dns enp2s0 && readlink -f /etc/resolv.conf && grep ^nameserver /etc/resolv.conf'
  ```
  Expect: `Link 2 (enp2s0): 10.10.20.252 1.1.1.1 8.8.8.8`, `/run/systemd/resolve/stub-resolv.conf`, `nameserver 127.0.0.53`. Else: previous step's Rollback, [STUCK].
- [ ] **Step 59** [V] ~2m: **AFTER** — the stub reaches Pi-hole, names resolve, the login path is untouched.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'resolvectl query o401343.ingest.us.sentry.io 2>&1 | head -1; resolvectl query archive.ubuntu.com 2>&1 | head -1 | cut -c1-40; sudo apt-get update 2>&1 | grep -ciE "misbehaving|temporary failure|could not resolve"; resolvectl status enp2s0 | grep "Current DNS"; chronyc -n sources | grep -c "^\^\*"'; ssh -o BatchMode=yes -o ControlPath=none east 'echo FRESH_KEY_LOGIN_OK'
  ```
  Expect: `o401343.ingest.us.sentry.io: 0.0.0.0 ...`; `archive.ubuntu.com: <address>`; `0`; `Current DNS Server: 10.10.20.252`; `1`; `FRESH_KEY_LOGIN_OK`. Else: the Rollback of the netplan step, [STUCK].
- [ ] **Step 60** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 56-60: Phase 6, checked: AFTER  the stub reaches Pi-hole, names resolve, the login path is untouched..' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 56-60: Checkpoint in Phase 6: AFTER  the stub reaches Pi-hole, names resolve, the login path is untouched.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 61** [R] ~1m: Note for the record, nothing to run: systemd-resolved stays on a fallback server after a failure. After a Pi-hole outage east keeps using 1.1.1.1 until `sudo systemctl restart systemd-resolved`. Not a fault.
- [ ] **Step 62** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 61-62: East: reach proven (route, 5/5 ping, 40 queries, sinkhole answer), netplan yaml and live link set, apt update clean, key login unaffected.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 61-62: DP-708: east resolves through Pi-hole first, public fallbacks, stub kept' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 63** [V] ~1m: **PHASE 6 EXIT GATE**.
  ```bash
  ssh east 'resolvectl dns enp2s0'
  ```
  Expect: `Link 2 (enp2s0): 10.10.20.252 1.1.1.1 8.8.8.8`. Else: [STUCK].

## PHASE 7: LXD-INSTALLER OFF EAST (Steps 64-68)

**Goal**: DP-12.
**Prerequisite**: Phase 6 or its [STUCK] (apt works either way).
**Time**: 10m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 64** [V] ~1m: The simulation still removes exactly two packages.
  ```bash
  ssh east 'sudo apt-get -s remove lxd-installer 2>&1 | grep -E "^Remv|^[0-9]+ upgraded"; apt-mark showmanual | grep -c "^openssh-server$"'
  ```
  Expect: `0 upgraded, 0 newly installed, 2 to remove and 0 not upgraded.`, `Remv ubuntu-server [1.560]`, `Remv lxd-installer [13ubuntu0]`, `1`. Else: a third `Remv`, or `0` at the end: Phase 7 [STUCK], nothing changed.
- [ ] **Step 65** [B][S] ~2m: Remove it. `ubuntu-server` goes with it because it is a metapackage that depends on it; it holds no files that matter. **Never run `apt autoremove` on east in this plan.**
  ```bash
  ssh east 'sudo env DEBIAN_FRONTEND=noninteractive apt-get -y remove lxd-installer 2>&1 | tail -1 | cut -c1-70; dpkg-query -W -f="\${Status}\n" lxd-installer; test ! -e /usr/sbin/lxd && echo LXD_STUB_GONE; getent group lxd | cut -d: -f1,3'
  ```
  Expect: the status line says `deinstall ok config-files`, then `LXD_STUB_GONE`, `lxd:101`. Else: Rollback, [STUCK].
  **Rollback:** `ssh east 'sudo apt-get -y install lxd-installer ubuntu-server'`
- [ ] **Step 66** [V] ~1m: Nothing that keeps east reachable is queued for autoremove, and login still works.
  ```bash
  ssh east 'sudo apt-get -s autoremove 2>&1 | grep -E "^Remv (openssh|chrony|netplan|wireguard|bird|sudo|systemd|nftables|auditd)" | wc -l'; ssh -o BatchMode=yes -o ControlPath=none east 'echo FRESH_KEY_LOGIN_OK'
  ```
  Expect: `0`, `FRESH_KEY_LOGIN_OK`. Else: a non-zero count: `ssh east 'sudo apt-mark manual <each named package>'`, re-run; login failure: Rollback, [STUCK].
- [ ] **Step 67** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 63-67: lxd-installer and the ubuntu-server metapackage removed; no autoremove run; gid 101 kept.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 63-67: DP-12: lxd-installer removed from east' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 68** [V] ~1m: **PHASE 7 EXIT GATE**.
  ```bash
  ssh east 'dpkg-query -W -f="\${Status}\n" lxd-installer | cut -d" " -f1'
  ```
  Expect: `deinstall`. Else: [STUCK].

## PHASE 8: EAST SSHD: PASSWORD BACK, ROOT OFF (Steps 69-76)

**Goal**: DP-11 first half and DP-15 on east: `10-unheaded-keys-only.conf` removed so password login works, `PermitRootLogin no`. **The port stays 22** (attended, S-E1-LIVE).
**Prerequisite**: hold session up (Step 8).
**Time**: 15m
**Agent**: Coordinator
**Model**: Opus

Why it cannot lock anyone out: a method is added, none removed for `govan`; root has no way in today; a timer restores the old files five minutes after arming unless a fresh key login proves the change; the hold session stays open throughout. `well-backup-copy`'s `Match` block is untouched.

- [ ] **Step 69** [V] ~1m: The hold session is alive; nobody needs root by ssh.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock -O check east 2>&1; ssh -S /run/user/1000/host-tidy-east.sock east 'sudo journalctl -q -o short-iso --no-pager | head -1 | cut -c1-10; sudo journalctl -u ssh --no-pager | grep -c "Accepted .* for root"; sudo sh -c "test ! -s /root/.ssh/authorized_keys && test ! -e /root/.ssh/authorized_keys2 && echo NO_ROOT_KEYS"'; cd /home/govan/tmp/wt/S-HOST-TIDY && git grep -nE 'root@(east|\$\{?EAST|fd00)' -- scripts deploy runbooks | grep -vc '^scripts/east-bootstrap.sh'
  ```
  Expect: `Master running (pid=<n>)`; the journal's first date (the real window, 2026-10-26 when measured); `0`; `NO_ROOT_KEYS`; `0`. Else: no master: rerun Step 8. Any root login, key or new `root@` use in the tree: Phase 8 [STUCK], nothing changed.
- [ ] **Step 70** [B][S] ~1m: Arm the undo timer first.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo systemd-run --unit=unheaded-tidy-sshd-undo --on-active=300 --timer-property=AccuracySec=1s /bin/sh -c "rm -f /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf; cp -p /var/backups/unheaded-host-tidy-2026-11-04/sshd_config.d/10-unheaded-keys-only.conf /etc/ssh/sshd_config.d/; sshd -t && systemctl reload ssh" 2>&1 | head -1; systemctl is-active unheaded-tidy-sshd-undo.timer'
  ```
  Expect: `Running timer as unit: unheaded-tidy-sshd-undo.timer`, `active`. Else: nothing has changed yet: Phase 8 [STUCK].
- [ ] **Step 71** [W][S] ~1m: Write the root file, remove the keys-only file, test the config. sshd takes the first value it reads, so `05-` beats everything after it.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east "printf '%s\n' '# DP-15, Stevie 2026-11-03: nothing logs in as root.' 'PermitRootLogin no' | sudo install -m 0600 -o root -g root /dev/stdin /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf && sudo rm /etc/ssh/sshd_config.d/10-unheaded-keys-only.conf && sudo sshd -t && echo SSHD_T_OK && sudo sshd -T | grep -E '^(port|permitrootlogin|pubkeyauthentication|passwordauthentication|kbdinteractiveauthentication|kexalgorithms) '"
  ```
  Expect, in this order: `SSHD_T_OK`, `port 22`, `permitrootlogin no`, `pubkeyauthentication yes`, `passwordauthentication yes`, `kbdinteractiveauthentication yes`, `kexalgorithms mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com`. Else: Rollback, [STUCK]. sshd has not been reloaded, so nothing live changed.
  **Rollback:** `ssh -S /run/user/1000/host-tidy-east.sock east 'sudo rm -f /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf; sudo cp -p /var/backups/unheaded-host-tidy-2026-11-04/sshd_config.d/10-unheaded-keys-only.conf /etc/ssh/sshd_config.d/; sudo sshd -t && sudo systemctl reload ssh; sudo systemctl stop unheaded-tidy-sshd-undo.timer'`
- [ ] **Step 72** [B][S] ~1m: Reload. Open sessions survive (`KillMode=process`).
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo systemctl reload ssh && systemctl is-active ssh'
  ```
  Expect: `active`. Else: the Rollback above, [STUCK].
- [ ] **Step 73** [V] ~1m: **LOCKOUT GATE** — a brand-new key login works and the server now offers password.
  ```bash
  ssh -o BatchMode=yes -o ConnectTimeout=8 -o ControlPath=none east 'echo FRESH_KEY_LOGIN_OK'; ssh -o BatchMode=yes -o ConnectTimeout=8 -o ControlPath=none -o PreferredAuthentications=none east true 2>&1 | tail -1; ssh -S /run/user/1000/host-tidy-east.sock -O check east 2>&1
  ```
  Expect: `FRESH_KEY_LOGIN_OK`; `govan@192.168.13.1: Permission denied (publickey,password,keyboard-interactive).`; `Master running (pid=<n>)`. Else: do not stop the timer. Run the Rollback now through the hold session, then [STUCK]. If the hold session is also gone, the timer undoes it at 300 s: wait, then retry `ssh east true`.
- [ ] **Step 74** [B][S] ~1m: Stop the timer. The change stays.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo systemctl stop unheaded-tidy-sshd-undo.timer; systemctl is-active unheaded-tidy-sshd-undo.timer; ls /etc/ssh/sshd_config.d/; stat -c "%a %n" /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf'
  ```
  Expect: `inactive`; `00-unheaded-kex.conf 00-unheaded.conf 05-unheaded-permitroot.conf 50-cloud-init.conf 60-unheaded-well-offsite.conf`; `600 /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf`. Else: a `10-` file means the timer fired: the host is back on the old config; re-run from Step 70 once, then [STUCK].
- [ ] **Step 75** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 68-75: East sshd: keys-only file removed, 05-unheaded-permitroot.conf added. Fresh key login proven, methods publickey,password,keyboard-interactive. Password itself not testable unattended.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 68-75: DP-11, DP-15 on east: password login back, PermitRootLogin no, port unchanged' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 76** [V] ~1m: **PHASE 8 EXIT GATE** — the well-offsite account is still key-only.
  ```bash
  ssh east "sudo sshd -T -C user=well-backup-copy,host=west,addr=192.168.13.2 | grep -E '^(passwordauthentication|authenticationmethods) '"
  ```
  Expect: `passwordauthentication no`, `authenticationmethods publickey`. Else: Rollback from this phase, [STUCK].

## PHASE 9: FAIL2BAN ON EAST (Steps 77-85)

**Goal**: DP-11 second half: fail2ban installed on east, sshd jail on the port in use, and on 7375 so the attended port move needs no jail edit.
**Prerequisite**: Phase 8.
**Time**: 15m
**Agent**: Coordinator
**Model**: Opus

Why it cannot lock anyone out: all of east's traffic arrives as west (192.168.13.2), the overlay or link-local, and every one of those is in `ignoreip`, in a file written **before** the package exists, so the first start already honours it. Honest note: the jail is live and sees failures, but every source that can reach east today is ignored; it is depth for the day routing changes.

- [ ] **Step 77** [W][S] ~1m: Write the jail first. A `.local` file in `jail.d` is read after every `.conf`, so it wins over Ubuntu's `defaults-debian.conf`.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo install -d -m 0755 /etc/fail2ban /etc/fail2ban/jail.d && sudo tee /etc/fail2ban/jail.d/unheaded-sshd.local >/dev/null <<"EOF"
  # DP-11. Every address east can be reached from is ignored: a ban on any of
  # them closes the only door to this host. 7375 is listed ahead of the
  # attended port move.
  [DEFAULT]
  ignoreself = true
  ignoreip = 127.0.0.1/8 ::1 192.168.13.0/30 fd00:dead:beef::/48 fe80::/10

  [sshd]
  enabled = true
  backend = systemd
  journalmatch = _SYSTEMD_UNIT=ssh.service + _COMM=sshd-session
  port = 22,7375
  maxretry = 5
  findtime = 600
  bantime = 3600
  EOF
  sudo grep -c "" /etc/fail2ban/jail.d/unheaded-sshd.local'
  ```
  Expect: `15`. Else: `ssh east 'sudo rm -rf /etc/fail2ban'`, [STUCK].
  **Rollback:** `ssh east 'sudo rm -rf /etc/fail2ban'`
- [ ] **Step 78** [B][S] ~2m: Install. One package, no recommends (DP-11 approved it; west runs the same version).
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo apt-get -s install --no-install-recommends fail2ban 2>&1 | grep -E "^Inst|^[0-9]+ upgraded"; sudo env DEBIAN_FRONTEND=noninteractive apt-get -y install --no-install-recommends fail2ban 2>&1 | tail -1 | cut -c1-70; systemctl is-active fail2ban; sudo fail2ban-client ping'
  ```
  Expect: `0 upgraded, 1 newly installed, 0 to remove and 0 not upgraded.`, `Inst fail2ban (1.1.0-8 ...`, then `active`, `Server replied: pong`. Else: a second `Inst` line: stop before the install if you can, otherwise Rollback; [STUCK].
  **Rollback:** `ssh east 'sudo apt-get -y purge fail2ban && sudo rm -rf /etc/fail2ban && sudo nft delete table inet f2b-table 2>/dev/null; true'`
- [ ] **Step 79** [V] ~1m: **LOCKOUT GATE 1** — the ignore list, the match and the ports are what was written.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo fail2ban-client get sshd ignoreip; sudo fail2ban-client get sshd journalmatch; sudo fail2ban-client get sshd action nftables port; sudo fail2ban-client get sshd bantime'
  ```
  Expect: the five networks including `192.168.13.0/30` and `fd00:dead:beef::/48`; `_SYSTEMD_UNIT=ssh.service + _COMM=sshd-session`; `22,7375`; `3600`. Else: Rollback, [STUCK].
- [ ] **Step 80** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 76-80: Phase 9, checked: LOCKOUT GATE 1  the ignore list, the match and the ports are what was written..' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 76-80: Checkpoint in Phase 9: LOCKOUT GATE 1  the ignore list, the match and the ports are what was written.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 81** [V] ~1m: **LOCKOUT GATE 2** — a real failure from west is seen and ignored. A nonexistent user with no valid key: harmless.
  ```bash
  ssh -o BatchMode=yes -o ConnectTimeout=8 -o ControlPath=none -o IdentitiesOnly=yes -o IdentityFile=/dev/null -o PreferredAuthentications=publickey tidy-probe@192.168.13.1 true 2>&1 | tail -1; sleep 3; ssh -S /run/user/1000/host-tidy-east.sock east 'sudo grep -c "Ignore 192.168.13.2 by ip" /var/log/fail2ban.log; sudo fail2ban-client status sshd | grep -E "Currently banned"'; ssh -o BatchMode=yes -o ControlPath=none east 'echo FRESH_KEY_LOGIN_OK'
  ```
  Expect: `tidy-probe@192.168.13.1: Permission denied (...)`; `1` or more; `Currently banned:	0`; `FRESH_KEY_LOGIN_OK`. Else: a count of `0` means the match sees nothing: Step 82. A ban of west: `ssh -S /run/user/1000/host-tidy-east.sock east 'sudo fail2ban-client unban --all'`, Rollback, [STUCK].
- [ ] **Step 82** [D] ~5m: Only if the count was 0: `ssh east 'sudo journalctl -u ssh -n 5 -o json --no-pager | grep -o "\"_COMM\":\"[^\"]*\"" | sort -u'`. Expect `"_COMM":"sshd-session"`. If so, read `/var/log/fail2ban.log` for a filter error. Two attempts, then keep the install, [STUCK], tell Stevie.
- [ ] **Step 83** [V] ~1m: A ban works on a documentation address, and clears.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo fail2ban-client set sshd banip 203.0.113.9 >/dev/null; sudo nft list table inet f2b-table | grep -c 203.0.113.9; sudo fail2ban-client set sshd unbanip 203.0.113.9 >/dev/null; sudo fail2ban-client status sshd | grep "Currently banned"'
  ```
  Expect: `1`, `Currently banned:	0`. Else: [STUCK]; keep the install, a jail that cannot ban locks nobody out.
- [ ] **Step 84** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 81-84: fail2ban 1.1.0-8 on east. journalmatch sshd-session, ignoreip proven by a provoked failure from west, banip/unbanip proven on 203.0.113.9.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 81-84: DP-11: fail2ban on east, jail on 22 and 7375, west and overlay ignored' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 85** [V] ~1m: **PHASE 9 EXIT GATE** — enabled at boot, and a fresh login still works.
  ```bash
  ssh -o BatchMode=yes -o ControlPath=none east 'systemctl is-enabled fail2ban; echo FRESH_KEY_LOGIN_OK'
  ```
  Expect: `enabled`, `FRESH_KEY_LOGIN_OK`. Else: [STUCK].

## PHASE 10: WEST: PERMITROOTLOGIN NO (Steps 86-91)

**Goal**: DP-15 on west, nothing else. West's password path (keyboard-interactive) is **not** touched: the laptop has no key yet.
**Prerequisite**: Phase 0.
**Time**: 10m
**Agent**: Coordinator
**Model**: Opus

Why it cannot lock anyone out: every login west's journal holds is `govan` (33 since 2026-10-23; `.132`, `.135`, `.137`), root is locked with no keys, and this file names only root. The agent runs on west, so its session does not depend on sshd. An undo timer is armed anyway.

- [ ] **Step 86** [V] ~1m: No root logins; the methods are the baseline.
  ```bash
  sudo journalctl -q -o short-iso --no-pager | head -1 | cut -c1-10; sudo journalctl -u ssh --no-pager | grep -c 'Accepted .* for root'; ssh -p 7375 -o BatchMode=yes -o PreferredAuthentications=none -o ControlPath=none -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null localhost true 2>&1 | tail -1
  ```
  Expect: the journal's first date (the real window, 2026-10-23 when measured), `0`, `govan@localhost: Permission denied (publickey,keyboard-interactive).`. Else: Phase 10 [STUCK], nothing changed.
- [ ] **Step 87** [B][S] ~1m: Arm the undo timer.
  ```bash
  sudo systemd-run --unit=unheaded-tidy-sshd-undo --on-active=300 --timer-property=AccuracySec=1s /bin/sh -c 'rm -f /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf; sshd -t && systemctl reload ssh' 2>&1 | head -1; systemctl is-active unheaded-tidy-sshd-undo.timer
  ```
  Expect: `Running timer as unit: unheaded-tidy-sshd-undo.timer`, `active`. Else: Phase 10 [STUCK].
- [ ] **Step 88** [W][S] ~1m: Write the file, test, reload.
  ```bash
  printf '%s\n' '# DP-15, Stevie 2026-11-03: nothing logs in as root.' 'PermitRootLogin no' | sudo install -m 0600 -o root -g root /dev/stdin /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf && sudo sshd -t && sudo sshd -T | grep -E '^(port|permitrootlogin|passwordauthentication|kbdinteractiveauthentication) ' && sudo systemctl reload ssh && systemctl is-active ssh
  ```
  Expect: `port 7375`, `permitrootlogin no`, `passwordauthentication no`, `kbdinteractiveauthentication yes`, `active`. Else: Rollback, [STUCK].
  **Rollback:** `sudo rm -f /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf && sudo sshd -t && sudo systemctl reload ssh; sudo systemctl stop unheaded-tidy-sshd-undo.timer`
- [ ] **Step 89** [V] ~1m: **LOCKOUT GATE** — the server offers exactly what it offered, then stop the timer.
  ```bash
  ssh -p 7375 -o BatchMode=yes -o PreferredAuthentications=none -o ControlPath=none -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null localhost true 2>&1 | grep -q 'Permission denied (publickey,keyboard-interactive)' && sudo systemctl stop unheaded-tidy-sshd-undo.timer; systemctl is-active unheaded-tidy-sshd-undo.timer; ss -tln | grep -c ':7375 '
  ```
  Expect: `inactive`, `2`. The timer stops only if the methods matched. Else: `active` means the methods differ: Rollback, [STUCK].
- [ ] **Step 90** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 85-90: West 05-unheaded-permitroot.conf in place; methods unchanged; port 7375 listening.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 85-90: DP-15 on west: PermitRootLogin no; keyboard-interactive untouched' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 91** [V] ~1m: **PHASE 10 EXIT GATE**.
  ```bash
  sudo sshd -T | grep '^permitrootlogin '
  ```
  Expect: `permitrootlogin no`. Else: [STUCK].

## PHASE 11: PAM_WHEEL ON SU (Steps 92-99)

**Goal**: DP-25 row 25.9: `su` restricted on both hosts, `sudo` proven first and after. East first.
**Prerequisite**: Phase 8 (hold session).
**Time**: 20m
**Agent**: Coordinator
**Model**: Opus

Why it cannot lock anyone out: `sudo` is `sudo-rs` on PAM services `sudo` and `sudo-i`, which include only the `common-*` stacks, not `/etc/pam.d/su`; govan's rule is `NOPASSWD`. `root` is locked on both hosts, so `su` to root by password already fails. Root keeps `su` through `pam_rootok`, which stays first. The console's rescue shell is `sulogin`, which does not use this file. `git grep` finds no script that calls `su`.

- [ ] **Step 92** [V] ~1m: **SUDO PROOF** — before any change, on both hosts.
  ```bash
  sudo -n true && echo WEST_SUDO_OK; grep -l pam_wheel /etc/pam.d/* | tr '\n' ' '; echo; grep -c 'pam.d/su\|include su' /etc/pam.d/sudo /etc/pam.d/sudo-i; ssh -S /run/user/1000/host-tidy-east.sock east 'sudo -n true && echo EAST_SUDO_OK; grep -c "include su" /etc/pam.d/sudo /etc/pam.d/sudo-i'; cd /home/govan/tmp/wt/S-HOST-TIDY && git grep -nE '(^|[ ;(])su (-s|-c|-l|- )' -- scripts deploy runbooks | wc -l
  ```
  Expect: `WEST_SUDO_OK`; `/etc/pam.d/su`; `/etc/pam.d/sudo:0`, `/etc/pam.d/sudo-i:0`; `EAST_SUDO_OK`; both east counts `0`; `0`. Else: Phase 11 [STUCK], nothing changed.
- [ ] **Step 93** [B][S] ~1m: East: arm the undo timer, add the empty group, add the line right after `pam_rootok`.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east "sudo systemd-run --unit=unheaded-tidy-pam-undo --on-active=300 --timer-property=AccuracySec=1s /bin/sh -c 'cp -p /var/backups/unheaded-host-tidy-2026-11-04/su /etc/pam.d/su' 2>&1 | head -1; sudo groupadd --system sugroup && sudo sed -i '/^auth       sufficient pam_rootok.so\$/a auth       required   pam_wheel.so use_uid group=sugroup' /etc/pam.d/su && grep -n '^auth' /etc/pam.d/su; getent group sugroup"
  ```
  Expect: `Running timer as unit: unheaded-tidy-pam-undo.timer`; `6:auth       sufficient pam_rootok.so`; `7:auth       required   pam_wheel.so use_uid group=sugroup`; `sugroup:x:<gid>:` with nothing after the last colon. Else: Rollback, [STUCK].
  **Rollback:** `ssh -S /run/user/1000/host-tidy-east.sock east 'sudo cp -p /var/backups/unheaded-host-tidy-2026-11-04/su /etc/pam.d/su; sudo groupdel sugroup; sudo systemctl stop unheaded-tidy-pam-undo.timer'`
- [ ] **Step 94** [V] ~1m: **LOCKOUT GATE** on east: sudo works, root keeps su, govan does not.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo -n true && echo SUDO_OK; sudo su -s /bin/sh -c "id -un" root; sudo su -s /bin/sh -c "id -un" nobody'; ssh -o BatchMode=yes -o ControlPath=none east 'sudo -n true && echo FRESH_LOGIN_SUDO_OK'
  ```
  Expect: `SUDO_OK`, `root`, `nobody`, `FRESH_LOGIN_SUDO_OK`. `su` from govan fails before and after (root is locked), so it proves nothing; the `su-restricted` probe in Phase 15 proves the rule. Else: Rollback, [STUCK].
- [ ] **Step 95** [B][S] ~1m: East: stop the timer. West: the same three moves.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock east 'sudo systemctl stop unheaded-tidy-pam-undo.timer; systemctl is-active unheaded-tidy-pam-undo.timer'; sudo systemd-run --unit=unheaded-tidy-pam-undo --on-active=300 --timer-property=AccuracySec=1s /bin/sh -c 'cp -p /var/backups/unheaded-host-tidy-2026-11-04/su /etc/pam.d/su' 2>&1 | head -1; sudo groupadd --system sugroup && sudo sed -i '/^auth       sufficient pam_rootok.so$/a auth       required   pam_wheel.so use_uid group=sugroup' /etc/pam.d/su && grep -n '^auth' /etc/pam.d/su; getent group sugroup
  ```
  Expect: `inactive`; `Running timer as unit: unheaded-tidy-pam-undo.timer`; lines 6 and 7 as on east; `sugroup:x:<gid>:`. Else: Rollback, [STUCK].
  **Rollback:** `sudo cp -p /var/backups/unheaded-host-tidy-2026-11-04/su /etc/pam.d/su; sudo groupdel sugroup; sudo systemctl stop unheaded-tidy-pam-undo.timer`
- [ ] **Step 96** [V] ~1m: **LOCKOUT GATE** on west, then stop the timer.
  ```bash
  sudo -n true && echo SUDO_OK && sudo su -s /bin/sh -c 'id -un' root && sudo su -s /bin/sh -c 'id -un' nobody && sudo systemctl stop unheaded-tidy-pam-undo.timer; systemctl is-active unheaded-tidy-pam-undo.timer
  ```
  Expect: `SUDO_OK`, `root`, `nobody`, `inactive`. Else: Rollback, [STUCK]. If the timer fired first, `grep -c pam_wheel /etc/pam.d/su` is `0` and the exit gate catches it.
- [ ] **Step 97** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 91-97: Phase 11, checked: west sudo, su to root and nobody after pam_wheel; undo timer stopped before this commit.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 91-97: Checkpoint in Phase 11: west gate passed, PAM undo timer stopped' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 98** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 98-98: sugroup empty on both, pam_wheel after pam_rootok; sudo, sudo su and fresh login proven.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 98-98: DP-25 row 25.9: pam_wheel on su, both hosts; sudo proven before and after' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 99** [V] ~1m: **PHASE 11 EXIT GATE**.
  ```bash
  grep -c '^auth       required   pam_wheel.so use_uid group=sugroup$' /etc/pam.d/su; ssh east "grep -c '^auth       required   pam_wheel.so use_uid group=sugroup\$' /etc/pam.d/su"
  ```
  Expect: `1`, `1`. Else: [STUCK].

## PHASE 12: RECORD WHAT IS ALREADY THERE (Steps 100-104)

**Goal**: DP-25 row 25.1: the 2026-10 fixes journaled. Run after Phases 8-11 so `sshd-safe`'s file-mode check sees the final files. Writes journal lines; changes no host file.
**Prerequisite**: Phase 3.
**Time**: 10m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 100** [B][S] ~3m: West. `check` is the gate; only what is in place is recorded. Measured as non-root on 2026-11-04: `sysctl-cis`, `apport-off`, `kmod-cis`, `banner-cis`, `apparmor-utils`, `journald-caps`, `time-p2p`, `rocm-log-perms` in place; `cron-perms`, `auditd`, `sudo-cis` need root to read.
  ```bash
  C="$(git -C /home/govan/tmp/wt/S-HOST-TIDY rev-parse HEAD)"; for f in sysctl-cis apport-off kmod-cis cron-perms banner-cis apparmor-utils auditd sudo-cis; do sudo unheaded-remediate -model-commit "$C" -record-existing "$f" -applied-by "remediate.sh (2026-10-01)" 2>&1 | tail -1 | cut -c1-90; done; for f in journald-caps time-p2p; do sudo unheaded-remediate -model-commit "$C" -record-existing "$f" -applied-by "remediate.sh (2026-10-13)" 2>&1 | tail -1 | cut -c1-90; done; sudo unheaded-remediate -model-commit "$C" -record-existing rocm-log-perms -applied-by "installer modes read 2026-10-27" 2>&1 | tail -1 | cut -c1-90
  ```
  Expect: eleven lines, each naming the fix as recorded. Else: a fix reported not in place is **not an error here**: copy its line to the progress file under For Stevie and do **not** apply it. Any other error: [STUCK] for that fix, go on.
  **Rollback:** journal lines are append-only and hash-chained; nothing on the host changed, so there is nothing to undo.
- [ ] **Step 101** [B][S] ~3m: East, with its `lab-hardened` extras.
  ```bash
  C="$(git -C /home/govan/tmp/wt/S-HOST-TIDY rev-parse HEAD)"; ssh east "for f in sysctl-cis apport-off kmod-cis cron-perms banner-cis apparmor-utils auditd sudo-cis; do sudo unheaded-remediate -model-commit $C -record-existing \$f -applied-by 'remediate.sh (2026-10-01)' 2>&1 | tail -1 | cut -c1-90; done; for f in auditd-rules sshd-safe; do sudo unheaded-remediate -model-commit $C -record-existing \$f -applied-by 'remediate.sh (2026-10-12)' 2>&1 | tail -1 | cut -c1-90; done; for f in journald-caps time-p2p; do sudo unheaded-remediate -model-commit $C -record-existing \$f -applied-by 'remediate.sh (2026-10-13)' 2>&1 | tail -1 | cut -c1-90; done; for f in rocm-log-perms aide-install apparmor-enforce; do sudo unheaded-remediate -model-commit $C -record-existing \$f -applied-by 'S-WELL-SPRING host window (2026-10-27)' 2>&1 | tail -1 | cut -c1-90; done"
  ```
  Expect: fifteen lines as above. `aide-install` and `apparmor-enforce` may report not in place: that is an answer, not a failure; their row then stays open. Else: as above.
  **Rollback:** as above.
- [ ] **Step 102** [V] ~1m: Both journals hold the rows.
  ```bash
  sudo grep -c '"backfilled":true' /var/lib/unheaded/remediate/journal.jsonl; ssh east 'sudo grep -c "\"backfilled\":true" /var/lib/unheaded/remediate/journal.jsonl'
  ```
  Expect: a count equal to the fixes recorded in each step above. Else: [STUCK].
- [ ] **Step 103** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 99-103: Backfilled journal rows on west and east; fixes not in place listed under For Stevie, none applied.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 99-103: DP-25 row 25.1: existing fixes journaled on both hosts' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 104** [V] ~1m: **PHASE 12 EXIT GATE** — the backfill changed no host file.
  ```bash
  sudo find /etc -newer /var/backups/unheaded-host-tidy-2026-11-04/sshd-T.before -type f 2>/dev/null | grep -vE 'sshd_config.d/05-|journald.conf.d/61-|modprobe.d/unheaded-usb|pam.d/su$|group|gshadow|passwd|shadow|ld.so.cache|docker|lxd' | wc -l
  ```
  Expect: `0`. Else: list the files in the progress file; [STUCK] if any is not explained by a step in this plan.

## PHASE 13: UMASK (Steps 105-111)

**Goal**: DP-25 row 25.8 "after the check": east gets 027; west keeps 0002 because the check fails there.
**Prerequisite**: Phases 8-12 (this changes the mode of files `sudo` creates on east, so it runs late).
**Time**: 10m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 105** [V] ~1m: **THE CHECK**, west. Containers run as their own uids and read repo files through bind mounts, so a repo file written at 027 is unreadable to them.
  ```bash
  cd /home/govan/tmp/unheaded && grep -cE '^\s+- \./' docker-compose.yml; grep -cE '^\s+- \.\.?/' monitoring/docker-compose.yml; grep -cE 'user: "1[0-9]{4}' docker-compose.yml; stat -c '%a %n' monitoring/prometheus/prometheus.yml
  ```
  Expect: `60`, `6`, `20`, a mode ending in `4` (world-readable today). Else: record the numbers; the decision below holds while the first three are above 0.
- [ ] **Step 106** [DECIDE] ~1m: West stays at 0002. **RECOMMENDATION: do not set UMASK on west.** Rationale: the next `git merge` or `git checkout` would write config files mode 0640 and Prometheus, Grafana, Vector and the rest would fail to read them at E1. Override ONLY IF Stevie has replaced those bind mounts. Add to the progress file, For Stevie: `25.8: west stays 0002, the check failed (66 repo bind mounts read by service uids); finding host-probe:west:default-umask stays open; S-E1-LIVE Evening 17 asks for accept or a bind-mount change.`
- [ ] **Step 107** [W][S] ~1m: East, which binds nothing. `/etc/login.defs` feeds `pam_umask` and satisfies the probe; with `USERGROUPS_ENAB yes` that gives `0007` for govan, so the `profile.d` line makes login shells a true `0027`.
  ```bash
  ssh east "printf 'UMASK 027\n' | sudo tee -a /etc/login.defs >/dev/null; printf '%s\n' '# DP-25 row 25.8.' 'umask 027' | sudo install -m 0644 -o root -g root /dev/stdin /etc/profile.d/05-unheaded-umask.sh; grep -n '^UMASK' /etc/login.defs"
  ```
  Expect: one line `<n>:UMASK 027`. Else: Rollback, [STUCK].
  **Rollback:** `ssh east "sudo sed -i '/^UMASK 027\$/d' /etc/login.defs; sudo rm -f /etc/profile.d/05-unheaded-umask.sh"`
- [ ] **Step 108** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 104-108: Phase 13, checked: THE CHECK, west. Containers run as their own uids and read repo files through bind mounts,.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 104-108: Checkpoint in Phase 13: THE CHECK, west. Containers run as their own uids and read repo files through bind mounts,' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 109** [V] ~1m: New sessions pick it up; login and sudo are unaffected. Record what `sudo` inherits: S-E1-LIVE's keyring step uses `install -m 0644` because of this.
  ```bash
  ssh -o BatchMode=yes -o ControlPath=none east 'umask; bash -lc umask; sudo -n true && echo SUDO_OK; sudo sh -c umask'
  ```
  Expect: `0007`, `0027`, `SUDO_OK`, then a `sudo` umask: write it in the progress file (reasoned as `0027`, not measured). Else: Rollback, [STUCK].
- [ ] **Step 110** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 109-110: East login.defs and profile.d set. West not changed: 66 repo bind mounts read by service uids.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 109-110: DP-25 row 25.8: umask 027 on east; west check failed and stays 0002' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 111** [V] ~1m: **PHASE 13 EXIT GATE**.
  ```bash
  ssh east "grep -c '^UMASK 027\$' /etc/login.defs"; grep -c '^UMASK' /etc/login.defs
  ```
  Expect: `1`, `0`. Else: [STUCK].

## PHASE 14: NOEXEC /TMP: TRIAL ONLY (Steps 112-118)

**Goal**: DP-25 row 25.4: prove or disprove that west builds survive `noexec` on `/tmp`, in a private mount namespace. The host's `/tmp` is not touched in this plan. Rule 14: the remount is a named root step, and the builds inside it run as `govan`.
**Prerequisite**: no other builder running.
**Time**: 15m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 112** [V] ~1m: Nothing is compiling, and the known exposure still stands.
  ```bash
  pgrep -cx 'go|cargo|rustc'; cd /home/govan/tmp/wt/S-HOST-TIDY && git grep -lE 'go build -o "?\$\{?(TMP|tmp|tmpd)' -- scripts | wc -l
  ```
  Expect: `0`, `8`. Else: a builder running: come back at the next gap, the phase is optional. A different count: record it.
- [ ] **Step 113** [V] ~3m: **CONTROL, NAMED ROOT STEP** — `sudo unshare` makes a private namespace, remounts `/tmp` noexec there, and drops to `govan` before any build. With no redirect `go test` must fail, or the trial proves nothing.
  ```bash
  sudo unshare --mount --propagation private sh -c 'mount -o remount,noexec /tmp && findmnt -no OPTIONS /tmp | cut -c1-22 && exec setpriv --reuid=govan --regid=govan --init-groups env -i HOME=/home/govan PATH=/usr/local/go/bin:/home/govan/.cargo/bin:/usr/bin:/bin bash -c "cd /home/govan/tmp/wt/S-HOST-TIDY && GOMAXPROCS=4 go test -count=1 ./pkg/cmdline/ 2>&1 | tail -2 | cut -c1-110"'; findmnt -no OPTIONS /tmp | grep -c noexec
  ```
  Expect: `rw,nosuid,nodev,noexec`, a failure mentioning `permission denied`, then `0` (the host's `/tmp` never changed). Else: the last line above 0 means the namespace leaked: `sudo mount -o remount,exec /tmp` at once, [STUCK]. A pass means the control failed: [STUCK].
- [ ] **Step 114** [V] ~8m: **TRIAL, NAMED ROOT STEP** — the same namespace as `govan`, `GOTMPDIR` and `TMPDIR` under `/var/tmp`: Go tests, a helper that execs a `mktemp` build, cargo with a real proc-macro build.
  ```bash
  install -d /var/tmp/govan-go /var/tmp/govan-tmp /var/tmp/host-tidy/cargo; sudo unshare --mount --propagation private sh -c 'mount -o remount,noexec /tmp && exec setpriv --reuid=govan --regid=govan --init-groups env -i HOME=/home/govan PATH=/usr/local/go/bin:/home/govan/.cargo/bin:/usr/bin:/bin GOTMPDIR=/var/tmp/govan-go TMPDIR=/var/tmp/govan-tmp CARGO_TARGET_DIR=/var/tmp/host-tidy/cargo bash -c "cd /home/govan/tmp/wt/S-HOST-TIDY && GOMAXPROCS=4 go test -count=1 ./pkg/cmdline/ ./cmd/unheaded-remediate/ 2>&1 | tail -3 | cut -c1-100; scripts/check-go-env.sh 2>&1 | tail -1 | cut -c1-90; scripts/check-zhen-mcp.sh 2>&1 | tail -1 | cut -c1-90; cargo test -j4 --manifest-path crates/monad-mbc/Cargo.toml --lib 2>&1 | tail -2 | cut -c1-90"'
  ```
  Expect: two `ok` lines, a pass line from each script, a cargo `test result: ok.`. Else: record the failing line in the progress file: that is the answer, not a stuck step.
- [ ] **Step 115** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 111-115: Phase 14, checked: TRIAL, NAMED ROOT STEP  the same namespace as govan, GOTMPDIR and TMPDIR under /var/tmp: G.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 111-115: Checkpoint in Phase 14: TRIAL, NAMED ROOT STEP  the same namespace as govan, GOTMPDIR and TMPDIR under /var/tmp: G' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 116** [DECIDE] ~1m: **RECOMMENDATION: defer the host change to S-E1-LIVE either way.** Rationale: a green trial covers shells with `TMPDIR` set. It cannot cover lanes already running with old environments, user timers, or `sudo` (`env_reset` drops `TMPDIR`). Applying it mid-series could fail other agents' builds. Override ONLY IF Stevie says so with the series finished. Add to For Stevie: `25.4: trial <green|failed at ...>; host /tmp unchanged; S-E1-LIVE Evening 17 applies it when no lane runs.`
- [ ] **Step 117** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 116-117: Control failed as it must; trial result recorded under For Stevie; deferred to S-E1-LIVE Evening 17.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 116-117: DP-25 row 25.4: noexec trial recorded, host /tmp unchanged' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 118** [V] ~1m: **PHASE 14 EXIT GATE** — west's `/tmp` is as it started.
  ```bash
  findmnt -no OPTIONS /tmp | grep -c noexec; rm -rf /var/tmp/host-tidy/cargo; go env GOTMPDIR | wc -c
  ```
  Expect: `0`, `1`. Else: `sudo mount -o remount,exec /tmp`, [STUCK].

## PHASE 15: EVIDENCE (Steps 119-120)

**Goal**: the collector, which uses `sudo -n` and ssh to east, agrees with the plan. Run from the worktree, because develop's model does not know the two new fixes yet; the user timer compiles from develop, so its next run after the merge agrees.
**Prerequisite**: Phases 3-13 finished or [STUCK].
**Time**: 10m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 119** [B] ~5m: Run the collector into the shared evidence file.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && GOMAXPROCS=4 go run ./cmd/compliance-evidence -out /home/govan/tmp/unheaded/var/compliance/evidence.json 2>&1 | tail -1 | cut -c1-90; stat -c '%y' /home/govan/tmp/unheaded/var/compliance/evidence.json | cut -c1-16
  ```
  Expect: a summary line, a time within the last few minutes. Else: one retry, then [STUCK]; Phase 16 waits.
- [ ] **Step 120** [V] ~1m: **PHASE 15 EXIT GATE** — write the list of phases marked [STUCK] for Phase 16.
  ```bash
  grep -n 'STUCK' /home/govan/tmp/wt/S-HOST-TIDY/docs/battle-plans/progress/S-HOST-TIDY-progress.md | wc -l
  ```
  Expect: `0`. Else: Phase 16's wiki step will stop for the coordinator, which is the right outcome.

## PHASE 16: REGISTER AND WIKI CLOSE-OUT (Steps 121-125)

**Goal**: rows that now pass leave the register; ops facts land in the wiki. Files in the worktree only: no sudo, no ssh.
**Prerequisite**: Phase 15.
**Time**: 15m
**Agent**: Agent
**Model**: Haiku

- [ ] **Step 121** [W] ~2m: Remove from the register only the rows whose probe now passes.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && python3 - <<'PY'
  import json, re
  ev = json.load(open('/home/govan/tmp/unheaded/var/compliance/evidence.json'))['records']
  verdict = {r['source']['kind'] + ':' + r['source']['ref']: r['verdict'] for r in ev}
  want = ['host-sshd:west:permitrootlogin=no', 'host-sshd:east:permitrootlogin=no',
          'host-probe:west:kmod-usb-storage-disabled', 'host-probe:east:kmod-usb-storage-disabled',
          'host-probe:west:su-restricted', 'host-probe:east:su-restricted',
          'host-probe:east:default-umask',
          'host-journal:west:apply-journal', 'host-journal:east:apply-journal']
  p = 'compliance/findings/register.yaml'
  t = open(p).read()
  for src in want:
      v = verdict.get(src, 'missing')
      if v != 'pass':
          print('KEEP', src, v); continue
      m = re.search(r'  - source: "' + re.escape(src) + r'"\n(?:    .*\n|      .*\n)+\n?', t)
      assert m, src
      t = t.replace(m.group(0), '')
      print('GONE', src)
  open(p, 'w').write(t)
  PY
  python3 -c "import yaml; yaml.safe_load(open('compliance/findings/register.yaml')); print('PARSES')"; GOMAXPROCS=4 go test ./cmd/compliance-evidence/ 2>&1 | tail -1 | cut -c1-60; go run ./cmd/compliance-evidence findings -evidence /home/govan/tmp/unheaded/var/compliance/evidence.json | grep -m1 -oE '[0-9]+ open' | head -1; cat /var/tmp/host-tidy/open-before
  ```
  Expect: `GONE` lines, `PARSES`, an `ok` line, then the open count, which fell by no more than the `GONE` lines from the baseline below it (rule 16; `apply-journal` rows may be `KEEP` if a fix was not in place). Else: a `KEEP` row stays open and its name goes in the progress file under For Stevie with its verdict: not a failure. A parse error: `git checkout -- compliance/findings/register.yaml`, [STUCK].
- [ ] **Step 122** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 118-122: Phase 16, checked: Remove from the register only the rows whose probe now passes..' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 118-122: Checkpoint in Phase 16: Remove from the register only the rows whose probe now passes.' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 123** [W] ~3m: Ops facts into the wiki. **Stop line:** if Phase 15's exit gate printed anything but `0`, do not run this: mark it [STUCK]; the coordinator edits the sentence for the stuck phase.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && python3 - <<'PY'
  p = 'wiki/Security.md'
  t = open(p).read()
  add = '''**Hosts after S-HOST-TIDY (2026-11-04).** West's journal is capped at 4G/500M (`61-unheaded-west.conf` over `60-unheaded.conf`), east's at 1G/200M. East answers password and key on port 22 (the move to 7375 is attended, S-E1-LIVE); `PermitRootLogin no` on both (`05-unheaded-permitroot.conf`). fail2ban on east watches `sshd-session` on 22 and 7375 and ignores every address west can use, because a ban on one would close east's only door; west's own jail bans port 22 only until S-E1-LIVE Evening 8. East resolves through Pi-hole 10.10.20.252, then 1.1.1.1 and 8.8.8.8, via the `127.0.0.53` stub (`/etc/netplan/01-enp2s0.yaml`; never `netplan apply` on east, enp2s0 is its only link). `usb-storage` cannot load on either host. `su` needs root on both (empty `sugroup`). East's umask is 027; west's stays 0002 because its compose stacks read repo files through bind mounts. `unheaded-remediate` lives in `/usr/local/sbin` with its model at `/usr/share/unheaded/remediation/fixes.yaml`, root-owned as it requires. Never run `apt autoremove` on east without reading the list: removing `lxd-installer` also removed the `ubuntu-server` metapackage.

'''
  anchor = '## Security Testing Infrastructure\n'
  assert t.count(anchor) == 1
  open(p, 'w').write(t.replace(anchor, add + anchor))
  p = 'wiki/Log-Aggregation.md'
  t = open(p).read()
  old = '- **Cap**: journald `SystemMaxUse=1G`, `RuntimeMaxUse=200M` (compliance fix `journald-caps`).'
  new = '- **Cap**: journald `SystemMaxUse=1G`, `RuntimeMaxUse=200M` (compliance fix `journald-caps`), raised on west to 4G/500M (`journald-caps-west`, DP-09).'
  assert t.count(old) == 1
  open(p, 'w').write(t.replace(old, new))
  print('wiki updated')
  PY
  ```
  Expect: `wiki updated`. Else: `git checkout -- wiki/Security.md wiki/Log-Aggregation.md`, [STUCK].
- [ ] **Step 124** [C] ~1m: **COMMIT CHECKPOINT** (only after the last [V] passed).
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && printf '%s\n' '- Steps 123-124: Collector run; passing rows removed; wiki/Security.md carries the host facts.' >> docs/battle-plans/progress/S-HOST-TIDY-progress.md && git add docs/battle-plans/progress/S-HOST-TIDY-progress.md compliance/findings/register.yaml wiki/Security.md wiki/Log-Aggregation.md && git commit -S -q -m '[PLAN S-HOST-TIDY] Steps 123-124: Close-out: passing rows out of the register, host facts in the wiki' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' && git log -1 --format='%h %G?'
  ```
  Expect: one line, a short hash then `G`. A gpg timeout: rerun with `--no-gpg-sign` in place of `-S` and say so in the progress file, not in the message.
- [ ] **Step 125** [V] ~1m: **PHASE 16 EXIT GATE**.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && git status --short --untracked-files=no | wc -l
  ```
  Expect: `0`. Else: commit named files or restore them; never `git add -A`.

## PHASE 17: MERGE AND CLEAN UP (Steps 126-129)

**Goal**: the branch on develop, the hold session closed, nothing armed.
**Prerequisite**: Phase 16.
**Time**: 10m
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 126** [V] ~1m: No undo timer is still armed on either host.
  ```bash
  systemctl list-timers --all --no-legend 'unheaded-tidy-*' | wc -l; ssh east "systemctl list-timers --all --no-legend 'unheaded-tidy-*' | wc -l"
  ```
  Expect: `0`, `0`. Else: find which; if it would undo a change that passed its gate, `sudo systemctl stop <name>.timer`.
- [ ] **Step 127** [B] ~1m: Close the hold session.
  ```bash
  ssh -S /run/user/1000/host-tidy-east.sock -O exit east 2>&1; ssh -o BatchMode=yes -o ControlPath=none east 'echo FRESH_KEY_LOGIN_OK'
  ```
  Expect: `Exit request sent.`, `FRESH_KEY_LOGIN_OK`. Else: a failed fresh login is an emergency: Appendix A1.
- [ ] **Step 128** [C] ~6m: **MERGE (rule 15): Phase 17: S-HOST-TIDY to develop.** First develop into the sprint worktree, rebuild, gate. Then the guarded merge; the main checkout is never left half-merged.
  ```bash
  cd /home/govan/tmp/wt/S-HOST-TIDY && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-host-tidy' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-host-tidy' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && GOMAXPROCS=4 go build ./... && test "$(GOMAXPROCS=4 go test -race ./pkg/compliance/... ./cmd/unheaded-remediate/ ./cmd/compliance-evidence/ 2>&1 | grep -vcE '^ok|no test files')" = 0 && echo SPRINT_GATE_OK
  ```
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-host-tidy -m '[PLAN S-HOST-TIDY] Phase 17: S-HOST-TIDY to develop' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-host-tidy -m '[PLAN S-HOST-TIDY] Phase 17: S-HOST-TIDY to develop' -m 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp' || { git merge --abort; echo MERGE_CONFLICT; }; }; }; git log -1 --format='%h %G? %s' | cut -c1-70
  ```
  Expect: `SPRINT_GATE_OK`; then `<hash> G [PLAN S-HOST-TIDY] ...`. Else: `MERGE_CONFLICT` or a failed gate: [STUCK], no merge. A dirty tracked file in the main checkout: nothing merges and the line shows the old head: find its owner. `N` instead of `G`: an unsigned merge: say so in the progress file; Stevie re-signs.
- [ ] **Step 129** [V] ~1m: **PHASE 17 EXIT GATE** — the coordinator folds `docs/battle-plans/progress/S-HOST-TIDY-progress.md` into `~/tmp/next.md`.
  ```bash
  git -C /home/govan/tmp/unheaded log --oneline -1 --grep='PLAN S-HOST-TIDY. Phase 17' | wc -l
  ```
  Expect: `1`. Else: [STUCK].

## PHASE 18: ZHEN HOST UNITS AND PATHS (Steps 130-136)

**Goal**: DP-712's **two** host units installed on west, disabled, with every path they name in place: `zhen-agentd.service` (20111) and `unheaded-vor.service` (20110, uid 16738). There is no third unit: round 4 keeps the Python embedder and parks the embed service in ADR-122. They start in S-E1-LIVE Evening 13 because they need the Well and TLS material.
**Prerequisite**: S-ZHEN-POLISH merged. A separate run; Phases 0-17 do not wait for it.
**Time**: 25m
**Agent**: Coordinator
**Model**: Opus

Why these paths: both units name them without a `-` prefix, so a missing one stops the unit at start. `/var/zhen/projects` and `/etc/unheaded/zhen` do not exist on west today. `/etc/zhen-agentd.env` is written in S-E1-LIVE Evening 13, before the unit starts, because it holds the DSNs. `/usr/local/bin/vor` does.

- [ ] **Step 130** [V] ~1m: **GATE** — the merge happened and the files exist.
  ```bash
  cd /home/govan/tmp/unheaded && git log --oneline --grep='PLAN S-ZHEN-POLISH' -1 | wc -l; test -f deploy/systemd/zhen-agentd.service && test -f deploy/systemd/unheaded-vor.service && test -f scripts/vor-export-source.sh && echo FILES_OK; ls deploy/systemd | grep -c embed; git grep -cE 'ZhenVor|ZhenAgentd' -- pkg/ports/ports.go; test -x /usr/local/bin/vor && echo VOR_BIN_OK
  ```
  Expect: `1`, `FILES_OK`, `0`, a count of `2` or more, `VOR_BIN_OK`. Else: `0` first: it is not time yet. A third unit in the tree: stop, it contradicts round 4: [STUCK]. Anything else: [STUCK], nothing installed.
- [ ] **Step 131** [B][S] ~2m: Accounts. West reads `/etc/sysusers.d/unheaded.conf`, which beats a copy under `/usr/lib`, so that is the file to replace.
  ```bash
  cd /home/govan/tmp/unheaded && sudo cp -p /etc/sysusers.d/unheaded.conf /var/backups/unheaded-sysusers.conf.bak-host-tidy && sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers unheaded.conf; id -u unheaded-vor; id -u unheaded-buildkey; id -u unheaded-heimdall; id -u zhen-agentd
  ```
  Expect: `16738`, `16737`, `16736`, a uid. Else: Rollback, [STUCK].
  **Rollback:** `sudo cp -p /var/backups/unheaded-sysusers.conf.bak-host-tidy /etc/sysusers.d/unheaded.conf` (accounts with no files are harmless and stay)
- [ ] **Step 132** [B][S] ~6m: Paths, the vor export and the code index. The `sudo` lines are root; the last lines run as `govan`, not root, because `/var/zhen` is govan's and git refuses root on a repo govan owns.
  ```bash
  cd /home/govan/tmp/unheaded
  sudo install -d -o root -g root -m 0755 /var/zhen/projects
  sudo install -d -o zhen-agentd -g zhen-agentd -m 0700 /var/zhen/projects/default
  git archive HEAD runbooks scripts/run-runbook.py | sudo tar -x --no-same-owner -C /var/zhen/projects/default && sudo chown -R zhen-agentd:zhen-agentd /var/zhen/projects/default
  sudo install -d -o govan -g unheaded-vor -m 2750 /var/zhen/projects/zhen-taught
  sudo install -d -o root -g zhen-agentd -m 0750 /etc/unheaded/zhen
  sudo install -m 0640 -o root -g zhen-agentd configs/zhen-schedules.yaml /etc/unheaded/zhen/schedules.yaml
  install -d -m 0755 /var/zhen/vor-source /var/zhen/code-index                 # as govan: /var/zhen is govan's, and git refuses root here
  scripts/vor-export-source.sh /var/zhen/vor-source/unheaded                   # as govan
  make build-zhen-codeindex install-hooks && bin/zhen-codeindex --repo . --out /var/zhen/code-index   # as govan
  stat -c '%a %U %G %n' /var/zhen/projects/default /var/zhen/projects/zhen-taught /etc/unheaded/zhen /etc/unheaded/zhen/schedules.yaml; test -f /var/zhen/vor-source/unheaded/.vor-export && test -f /var/zhen/code-index/CURRENT && test -L .git/hooks/post-merge && echo ready
  ```
  Expect: `700 zhen-agentd zhen-agentd ...`, `2750 govan unheaded-vor ...`, `750 root zhen-agentd ...`, `640 root zhen-agentd ...`, then `ready`. Else: Rollback, [STUCK].
  **Rollback:** `cd /home/govan/tmp/unheaded && sudo rm -rf /var/zhen/projects/default; sudo rmdir /var/zhen/projects/zhen-taught /var/zhen/projects; sudo rm -f /etc/unheaded/zhen/schedules.yaml; sudo rmdir /etc/unheaded/zhen; rm -f .git/hooks/post-commit .git/hooks/post-merge; rm -rf /var/zhen/vor-source /var/zhen/code-index`
- [ ] **Step 133** [B][S] ~2m: The units. Not enabled, not started.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -m 0644 deploy/systemd/unheaded-vor.service deploy/systemd/zhen-agentd.service /etc/systemd/system/ && sudo systemctl daemon-reload; systemctl is-enabled unheaded-vor.service zhen-agentd.service
  ```
  Expect: `disabled` twice. Else: Rollback, [STUCK].
  **Rollback:** `sudo rm -f /etc/systemd/system/unheaded-vor.service /etc/systemd/system/zhen-agentd.service; sudo systemctl daemon-reload`
- [ ] **Step 134** [V] ~2m: Each unit verifies.
  ```bash
  for u in unheaded-vor zhen-agentd; do systemd-analyze verify /etc/systemd/system/$u.service 2>&1 | grep -vc 'is not executable'; done
  ```
  Expect: `0`, `0`. Else: [STUCK]. A binary not yet in `/usr/local/bin` is normal and filtered out: S-E1-LIVE installs `zhen-agentd`.
- [ ] **Step 135** [W] ~1m: This phase runs after the merge and changes no repo file: no commit. Append to `~/tmp/next.md` under START HERE: `DP-712: unheaded-vor and zhen-agentd installed on west, disabled, paths made; start them in S-E1-LIVE Evening 13.`
- [ ] **Step 136** [V] ~1m: **PHASE 18 EXIT GATE** — nothing started.
  ```bash
  systemctl is-active unheaded-vor.service zhen-agentd.service | sort -u
  ```
  Expect: `inactive`. Else: `sudo systemctl stop <unit>`, [STUCK].

---

## Appendix A — Emergency procedures

**A1. A fresh `ssh east` fails.**
1. Do not close the hold session. `ssh -S /run/user/1000/host-tidy-east.sock east 'sudo sshd -t; sudo fail2ban-client status sshd 2>/dev/null | grep -A2 Actions; ls /etc/ssh/sshd_config.d/'`.
2. A ban on west: `sudo fail2ban-client unban --all`, then Phase 9 Rollback.
3. A bad config: restore `sudo cp -a /var/backups/unheaded-host-tidy-2026-11-04/sshd_config.d/. /etc/ssh/sshd_config.d/; sudo rm -f /etc/ssh/sshd_config.d/05-unheaded-permitroot.conf; sudo sshd -t && sudo systemctl reload ssh`.
4. Hold session gone too: an armed undo timer fires within 5 minutes; wait 6 and retry.
5. Still out: the network is the suspect, not sshd: `ping -c3 192.168.13.1`, `sudo wg show`. East sits beside west: keyboard and monitor are the last resort; write it under For Stevie and stop every east phase. Do not retry logins in a loop.

**A2. `sudo` fails on a host.** Do not touch PAM further. In the hold session or on west's console: `cp -p /var/backups/unheaded-host-tidy-2026-11-04/su /etc/pam.d/su` needs root, so if there is no root path the armed `unheaded-tidy-pam-undo` timer is the recovery: wait 6 minutes and retry `sudo -n true`.

**A3. East cannot resolve names.** `ssh east 'sudo resolvectl dns enp2s0 1.1.1.1 8.8.8.8'` restores the old live state at once, then the Phase 6 Rollback. Logins are unaffected.

**A4. West's journald will not start.** `sudo rm -f /etc/systemd/journald.conf.d/61-unheaded-west.conf && sudo systemctl restart systemd-journald`.

**A5. Pi-hole stopped after Phase 5.** `/snap/bin/lxc start pihole --project default`; LAN DNS depends on it, so do this before anything else.

## Appendix B — Agent assignment

| Phase | Model | Host | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Opus | both, read and back up | none | 15m |
| 1 Register, stub | Haiku | repo only | 0 | 15m |
| 2 Model | Opus | repo (root-run model) | 0 | 25m |
| 3 Tool, journald, ROCm | Opus | both | 2 | 20m |
| 4 usb-storage | Opus | both | 3 | 10m |
| 5 Image, LXD, Vizio | Opus | west | 0 | 20m |
| 6 East resolver | Opus | east | 0 | 20m |
| 7 lxd-installer | Opus | east | 6 | 10m |
| 8 East sshd | Opus | east | 7 | 15m |
| 9 East fail2ban | Opus | east | 8 | 15m |
| 10 West root | Opus | west | 0 | 10m |
| 11 pam_wheel | Opus | both | 8 | 20m |
| 12 Record existing | Opus | both | 3, 8-11 | 10m |
| 13 umask | Opus | east | 12 | 10m |
| 14 noexec trial | Opus | west | no builder running | 15m |
| 15 Evidence | Opus | both, read | 3-13 | 10m |
| 16 Register, wiki | Haiku | repo only | 15 | 15m |
| 17 Merge | Opus | repo, closes the hold session | 16 | 10m |
| 18 Units and paths | Opus | west | S-ZHEN-POLISH | 25m |

Moved to S-E1-LIVE, with reasons: 25.3 `nodev,nosuid` on `/var` (Docker and LXD storage, Pi-hole lives there), 25.5 X purge (no dry run here), 25.4 host change, 25.8 on west, the fix to west's fail2ban port.

---

*S-HOST-TIDY Battle Plan — Forged 2026-11-04*
*19 Phases. 136 Steps. Every door checked from the outside before the old key is put away.*
*Sweep the floor; leave the locks to the one holding them.*
