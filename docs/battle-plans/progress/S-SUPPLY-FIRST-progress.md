<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-SUPPLY-FIRST progress

Plan: `docs/battle-plans/S-SUPPLY-FIRST-battle-plan.md`.

## For Stevie

1. ChromaDB has no release free of PYSEC-2026-311, -3813, -3814, -3815 (`last_affected` 1.5.9). All four are server endpoints or the RBAC provider; Grimoire runs the embedded client. Default taken: `chromadb==1.5.9`, tracked as FND-032. The file stays outside `check-python-audit.sh` because five other lines are ranges, so `python-audit-accepted.txt` does not grow. Appendix D of the plan pins those five and adds four accepted lines if you say so; the other way out is replacing ChromaDB in Grimoire. Dependabot #119 and #120 stay open either way.
2. Veto list: `docs/legal/RELICENSE-CANDIDATES-2026-11-04.md`, untouched until you answer.
3. Grimoire's full hash lock needs 101 ADR-096 admissions, or a CPU-index decision that drops the 19 CUDA wheels.
4. golang.org/x/crypto is on an untagged pseudo-version until the next tag (no tag carries the GO-2026-5932 fix; `openpgp/` removal is the only difference from v0.58.0).
5. `tomb/ansible/playbook-grimoire.yml` deploys `files/rag-query.py`, which does not exist; `failed_when: false` hides it. Pre-existing, left alone.
6. torch stays on 2.10.0 in `raft/` and the pip freeze although 2.14.1 audits clean, against DP-722's wording: DP-25.7 put the ROCm move and its training smoke after E1. Grimoire's CPU floor rose to 2.14.1.
7. This plan runs eight lane branches inside one plan branch, past the letter of rule 1, on S-LOW-FRUIT's precedent.
8. ruff, pytest and oxlint are not vulnerability bumps, so rule 4's approval does not literally cover them.
9. The embedded Grimoire scripts use Chroma's default embedding, which downloads an ONNX model on first use and cannot work on an air-gapped VM. Probably older than this bump, not verified.

## Stuck

(none yet)

## Lane reports

