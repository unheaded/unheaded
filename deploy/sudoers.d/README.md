# deploy/sudoers.d

Draft sudo rules. Nothing here is installed on any host.

## 60-unheaded-pull.draft

Status: draft, 2026-11-09. Syntax checked (`visudo -c -f`, sudo-rs 0.2.8). Matching not proved.

Closes, once installed and proved: findings `host-probe:east:sudo-password-required` and
`host-probe:west:sudo-password-required` (`compliance/findings/register.yaml`; ruling 2026-11-09:
narrow, east first, attended).

### What the log showed

East, `/var/log/auth.log` and `auth.log.1`, 2026-11-01 to 2026-11-09, sudo by the operator account:

| | lines | distinct commands |
|---|---|---|
| all | 8679 | 312 |
| the compliance pull (seen on 4 or more days) | 8279 | 24 shapes |
| everything else (seen on 3 or fewer days) | 400 | 234 |

No line had a TTY. The pull (`cmd/compliance-evidence`, the hourly `unheaded-compliance-evidence`
user timer on west; probes in `pkg/compliance/crosswalk/collect`, over ssh, `sudo -n`) is the only
recurring caller. Of its lines, 5533 were one probe (`acct-noshell-locked`) running one awk per
account.

The other 400 are one-off administration, mostly from unattended sessions: `sh -c`, `grep`,
`journalctl`, `unheaded-remediate` (27), `apt-get` (15), `install` (10), `systemctl` (8) and
others. No narrow rule covers these. They are the open question below.

### What changed in code first

sudo-rs allows no wildcards in arguments, so a command with a changing argument cannot be named.
Three probes had one (`cc833a97`, `f75a9ead`):

- `acct-noshell-locked`: one awk per account, now one fixed awk for all accounts.
- `pw-change-past`: today's day number in the privileged awk, now compared outside it.
- `dot-files`: one find per home, now one fixed loop that reads `user home` pairs on stdin.

Outputs were compared on west, old against new: identical. Hosts need the new collector before
the rule goes in, or these three probes report `unreadable`.

### What the draft is

23 rules for a new account `unheaded-pull`, one per exact command: 20 as logged, 3 in the new
fixed form. Read only, except that several are `sh -c '<fixed script>'`.

Known limits:

1. Matching is unproved. sudo-rs documents "arguments must match exactly"; whether a rule
   matches an argument that itself holds spaces (`sh -c '...'`) has to be seen on a host. If it
   does not, the fallback is one root-owned helper with the scripts inside it and a rule with
   plain one-word arguments. That is a design change and is not built.
2. `sudo -n -Hiu root env` is logged as `/usr/bin/bash -c env`; the rule is written that way and
   is the least certain of the 23.
3. `dot-files` trusts the home paths on stdin: the account can list dot-file names and modes in
   any directory. `root-path` reads a PATH on stdin the same way (directory owner and mode only).
4. A command the pull only runs on some host states (a branch not taken in nine days) is not in
   the list. A missing rule shows as a probe reporting `unreadable` or failing, not as silence.
5. The remediation path (`unheaded-remediate`, 27 runs) is not in the draft. It changes the
   host and should keep needing a person.

### Install order (attended; east first)

Each step leaves the operator's own rule untouched until the last one, so no step can lock
anyone out.

1. Deploy the collector with the three fixed probes (any build at or after `f75a9ead`).
2. Create the account: system uid, shell `/bin/sh`, password locked, ssh key only, the key line
   in `authorized_keys` prefixed `restrict` and `from="<the collector host>"`.
3. Open a second root shell and keep it open to the end.
4. `install -m 0440 -o root -g root 60-unheaded-pull.draft /etc/sudoers.d/60-unheaded-pull`
   then `visudo -c`. A failure here: delete the file from the second shell.
5. Prove matching, as the new account: run the pull against the host with the new account and
   compare its records with a pull as the operator. Any probe that differs names its rule.
   Then one negative: `sudo -n /usr/bin/id` must be refused.
6. Point the scheduled pull (`unheaded-compliance-evidence`) at the new account.
7. Only then, and only once the operator password is known to work for sudo (`sudo -k; sudo -v`
   in a fresh shell): replace `NOPASSWD: ALL` on the operator's line in `/etc/sudoers` with
   `visudo`. Rollback: restore the line from the second root shell.
8. Run the pull. `sudo-password-required` should now pass on that host.
9. West: same steps, after a week of east without a failed probe.

### Open questions for Stevie

- Unattended sessions on east use `sudo -n` as the operator (the 400 lines). After step 7 they
  cannot. Either they stop needing root on east, or they get their own narrowed account, or
  step 7 waits.
- Exact rules in sudoers, or the helper (limit 1)? The helper is sturdier and is more code.
- `unheaded-remediate`: password every time, or its own rule.
