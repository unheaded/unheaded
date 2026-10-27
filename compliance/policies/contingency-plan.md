<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Contingency Plan

**Version:** 1 (draft 2026-10-27)
**Owner:** Stevie Bellis. Reviewed with the [IR plan v2](../../runbooks/security/ir-plan-v2-2026-10-27.md) (UH-IR-01 attestation).
**Covers:** 800-53 CP-2, CP-4, CP-9, CP-10; 800-171 03.08.09 (backup), CSF 2.0 RC.RP; ISO 27001:2022 A.5.29, A.5.30, A.8.13; SOC 2 A1.2, A1.3 (availability criteria, outside the Security-only scope of `compliance/catalog`). Evidenced in the catalog by UH-BACKUP-01 only.

Objectives below are taken from what exists and has been measured. Where nothing was measured, the plan says so.

## 1. Assets

| Asset | Where | Protected by | Can be rebuilt from git alone |
|---|---|---|---|
| west | dev box, NVMe (`/`) + sdb (`/var`) | nothing bit-level; configuration in git | partly (host bootstrap runbook) |
| east | 4 cores, 8 GB, sda | nothing bit-level | partly |
| The Well | west, docker volume `unheaded-dev_postgres-data` on sdb | nightly dump to `/srv/backups/unheaded-well/` (NVMe, another disk), copied to east `/var/backups/unheaded-well-west/sets/` (append-only account); 14 sets each | no: data |
| Repository | GitHub `unheaded/unheaded`, west working copy | every-30-min mirror of all refs to a bare repo on east (`scripts/backup/git-mirror.sh`, user timer) | n/a |
| `/var/lib/unheaded` (west, east) | host state: remediation journals and undo scripts, textfile metrics, Sword history (`pkg/statefile`), chaos revert journal, baseline agent state | none | no: journals are history |
| `/etc/unheaded` (west, east) | host configuration and secrets: `keys/`, `trust.d/`, `pki/`, `well-tls/`, `well-offsite/`, `auth.env`, alert-mailer SMTP password, heartbeat token | none | no: secrets are re-issued, not restored |
| Wotan | west and east containers | none needed: no persistent state; rooms are in memory and lost on restart (accepted) | yes |
| Monitoring history | Prometheus, VictoriaMetrics, Loki volumes on west | none | no: history lost, accepted |
| Shipped logs | ClickHouse `unheaded_logs.logs` on west (90 d, 455 d audit rows) | none | no: see gap C2 |
| Models | `/var/zhen/models` on west | sha256 pins (`scripts/lib/model-pins.sh`); files re-downloadable | yes, by download |
| LXD | west, project `unheaded` (Sword/Cuirass instances, images) | none | yes: images rebuilt, instances redeployed by Sword |

### Keys and custody

| Key | Held | If lost | If stolen |
|---|---|---|---|
| Commit signing GPG key (Stevie) | Stevie; gpg-agent on west | new key; old signatures stay valid | revoke, new key, re-sign unpushed commits (`scripts/resign-unpushed.py`) |
| Service token keys (Ed25519, ADR-103) | `/etc/unheaded/keys/`, public halves in `trust.d/` | re-issue with `unheaded-keygen` | remove the `.pub` from `trust.d` (revokes), re-issue |
| Internal PKI (CA, service certs) | `/etc/unheaded/pki` | re-issue | `runbooks/pki/ca-compromise-root-rotation.yaml` |
| Well TLS | `/etc/unheaded/well-tls` | re-issue (`scripts/well/make-tls.sh`) | re-issue, restart clients |
| Offsite copy SSH key | `/etc/unheaded/well-offsite/id_ed25519` (west) | new key on east's `well-backup-copy` | the account can only add files; replace the key |
| WireGuard keys and PSK | `/etc/wireguard` on both hosts | re-key both ends | re-key both ends |
| ML-DSA-65 drift and evidence keys | per ADR-105 (E4, E5 open) | re-key, redistribute public keys | re-key; events signed after theft are suspect |
| Release signatures | none: keyless cosign | n/a | n/a |

No key has an offline backup. All except the GPG identity are re-issuable; the GPG key's loss changes the signer, not the history.

## 2. Objectives

| Service | RPO | RTO | Basis |
|---|---|---|---|
| The Well | 24 h (nightly dump; no point-in-time recovery) | data restore 18 s measured (weekly drill, `unheaded_well_drill_restore_seconds` on 2026-10-25, 26 tables); end-to-end service RTO not measured, target 1 h | ADR-016 amendment 2026-10-15; `monitoring/prometheus/rules/well.yml` `WellDrillLate` |
| Repository | 30 min for unpushed commits (mirror timer); 0 for pushed | minutes (clone from GitHub or east) | FND-005 |
| Wotan | none (in-memory, loss accepted) | minutes (container start) | 2026-10-18 postmortem |
| Dashboard, Kanban, timeguru | as The Well (stateless otherwise) | minutes after The Well | |
| Monitoring stack | history not protected | minutes (compose up, `kingdom-startup.sh`) | |
| west host | as its parts above | not measured: the rebuild exercise (section 5) has not run | |
| east host | as its parts above | not measured | |

## 3. Recovery order

1. **Network and access:** console or SSH to the surviving host; P2P link (192.168.13.1 east, .2 west).
2. **The Well:** restore the newest good set (`runbooks/data/postgresql-backup-restore.yaml`; from east if west's disks are gone: `runbooks/data/well-restore-from-east-offsite.yaml`). Prove it with the drill script before pointing services at it.
3. **Wotan**, then services in `scripts/kingdom-startup.sh` order (monitoring stack first, so recovery is watched).
4. **Secrets:** re-issue what lived in `/etc/unheaded` (section 1 keys), never copy them off a suspect host.
5. **Host state:** `/var/lib/unheaded` journals cannot be restored; record the gap in the incident file and re-record current state with `unheaded-remediate --record-existing` once that exists.
6. **Close out:** IR plan v2 section 5 checklist.

## 4. Dependencies

The Well depends on west's sdb; its backups on west's NVMe and east's sda. Every service depends on The Well or Wotan. Alerting depends on west (Prometheus, Alertmanager, alert-mailer) and on mail delivery; west's own death is not alerted (tabletop 2026-10-27, G1). Rebuilding either host depends on GitHub or the east mirror, Ubuntu and Docker Hub images, and the pins in `scripts/lib/tool-pins.sh`.

## 5. Host rebuild exercise

Defined as `runbooks/infra/host-rebuild-exercise.yaml` (status draft). **Scheduled, not yet run.** It needs a host window (S-WELL-SPRING E-D or later) and rebuilds east, the smaller blast radius, from git and the runbooks, timing each phase. Its measured times replace "not measured" in section 2.

## 6. Gaps

| # | Gap | State |
|---|---|---|
| C1 | No bit-level or `/etc/unheaded` + `/var/lib/unheaded` backup of either host | open; secrets are re-issued by design, journals are lost |
| C2 | ClickHouse logs (incident evidence) live on west only | open |
| C3 | Host RTO unmeasured | open until the rebuild exercise runs |
| C4 | No alternate site; both hosts in one house | accepted (lab) |

## 7. Testing and review

Weekly restore drill (automatic). Host rebuild exercise once, then yearly. This plan is reviewed with the IR plan.
