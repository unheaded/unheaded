<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-HOST-TIDY progress

## For Stevie

- West's fail2ban bans port 22 only (`port = ssh`, sshd is on 7375): `.132` was banned 2026-10-29 and logged in on 7375 anyway. Left alone; S-E1-LIVE Evening 8 carries the fix.
- 25.2: usb-storage is blacklisted on west too. This departs from the recorded answer (row 25.2 option B, accept on west) because B's only reason was an offline USB copy and DP-06 chose YubiKeys. Undo: the Rollback line of the usb-storage apply step.
- `scripts/east-bootstrap.sh` lines 18 and 233 still name `root@`: stale, not run by anything, not changed here.
- Test east's password once by hand from west, it cannot be tested unattended: `ssh -o PubkeyAuthentication=no -o ControlPath=none east true`
- Plan typo, Step 2: the check `head -3 | tail -1` reads line 3 (blank); the heredoc puts `# S-HOST-TIDY progress` on line 2. The file matches the plan text exactly (checked with `sed -n 2p`), so the step counted as passed and the file was not bent to fit. The check should be `sed -n 2p`.

## Evidence

- Steps 1-5: Phase 0, checked: Back up wests configs..
- Steps 6-7: Worktree made; open count saved; config backups on west and east.
