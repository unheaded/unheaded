<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Attestation drafts

Drafts of the three human attestations the catalog waits on. They live here,
outside `compliance/attestations/`, so the collector never reads them: the
controls stay NOT_ASSESSED until Stevie signs.

| Draft | Control | Freshness | Read before signing |
|---|---|---|---|
| `security-policy-review.yaml` | UH-POL-01 | 365 d | [infosec policy](../../../../compliance/policies/information-security-policy.md), [single-operator posture](../../../../compliance/policies/single-operator-posture.md) |
| `ir-plan-review.yaml` | UH-IR-01 | 365 d | [IR plan v2](../../../../runbooks/security/ir-plan-v2-2026-10-27.md), [tabletop](../../../incidents/tabletop-2026-10-27.md), [contingency plan](../../../../compliance/policies/contingency-plan.md) |
| `access-review.yaml` | UH-AC-01 | 90 d | the access list below, walked on west and east |

## Signing

```bash
scripts/compliance/sign-attestation.sh --dry-run access-review   # print what would be committed
scripts/compliance/sign-attestation.sh access-review             # copy, date, sign, commit
```

The script refuses unless `git config user.email` is `stevie@bellis.tech`
and a signed `git commit-tree -S` succeeds. It writes
`compliance/attestations/<name>.yaml` with `attested_at` = today (UTC) and
`expires_at` = today + the control's `freshness_days`, commits that one file
alone (`attest: <name> reviewed <date>`), and prints the commit's `%G?`
(must be `G`). The collector checks the last commit touching the file, so
nothing else rides in that commit. Re-signing next cycle is the same command.

## Access list (UH-AC-01)

Read-only; run each on west, then `ssh govan@east` and repeat. Remove what
has no purpose before signing; the statement says so.

```bash
# Login accounts (anything with a real shell)
getent passwd | awk -F: '$7 !~ /(nologin|false)$/ {print $1, $3, $6, $7}'
# SSH keys that can log in
for h in /root /home/*; do sudo test -f "$h/.ssh/authorized_keys" && { echo "== $h"; sudo awk '{print $1, $NF}' "$h/.ssh/authorized_keys"; }; done
# sudo rights
sudo grep -rhv '^\s*\(#\|$\)' /etc/sudoers /etc/sudoers.d/
# Groups that are root-equivalent or read every log
getent group sudo docker lxd systemd-journal adm
# Service accounts (expected: one nologin user per service, pkg/uids)
getent passwd | awk -F: '$3 >= 16600 && $3 <= 16799 {print $1, $3, $7}'
```

The Well (west only):

```bash
sudo docker exec unheaded-postgres psql -X -At -U unheaded -d postgres \
  -c "SELECT rolname, rolcanlogin, rolsuper FROM pg_roles WHERE rolname !~ '^pg_' ORDER BY 1"
```

GitHub (`unheaded/unheaded`): Settings, Collaborators; Deploy keys; your
account's SSH and GPG keys, sessions and authorized OAuth apps.
