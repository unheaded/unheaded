# Compliance Paperwork

The written half of the compliance program (ADR-097/098): policies, plans, the generated system security plan, the privacy pack, and the three attestations only Stevie signs. The machine half (catalog, collectors, findings register) is in `compliance/catalog/` and ADR-098.

## Where things are

| What | File |
|---|---|
| Information security policy | `compliance/policies/information-security-policy.md` |
| Single-operator posture (accepted gaps, compensating controls) | `compliance/policies/single-operator-posture.md` |
| Contingency plan (assets, key custody, RPO/RTO, recovery order) | `compliance/policies/contingency-plan.md` |
| Risk management strategy (appetite, categorization, criticality) | `compliance/policies/risk-management-strategy.md` |
| Incident response plan v2 | `runbooks/security/ir-plan-v2-2026-10-27.md` (v1 kept, superseded) |
| First tabletop (Wotan OOM replayed) | `docs/incidents/tabletop-2026-10-27.md` |
| Host rebuild exercise (draft, not yet run) | `runbooks/infra/host-rebuild-exercise.yaml` |
| System security plan, generated | `docs/compliance/SSP-generated.md` |
| Crosswalk spot-check | `docs/compliance/crosswalk-review-2026-10-27.md` |
| Privacy pack | `docs/compliance/privacy/` (PII inventory, notice, processing record, DPIA, processor template, suppliers) |
| Open decisions with defaults | `docs/compliance/privacy/decisions-for-stevie.md` |
| Attestation drafts + access-review walk | `docs/compliance/drafts/attestations/` |

The May 2026 matrices under `docs/compliance/control-matrix/` are historical snapshots; the live crosswalk is `compliance/catalog/controls.yaml`.

## Regenerate the SSP

```bash
go run ./cmd/compliance-evidence                                   # collect evidence first (var/compliance/evidence.json)
go run ./cmd/compliance-evidence ssp -out docs/compliance/SSP-generated.md
```

One section per control (status with the date that decided it, statement, rationale, evidence sources, mappings) and `## SoA`, the framework coverage table. It prints no record details or commits, so a regeneration diff shows only what changed. `-at YYYY-MM-DD` evaluates at the end of that UTC day.

## Sign an attestation (Stevie only)

```bash
scripts/compliance/sign-attestation.sh --dry-run access-review   # show the file
scripts/compliance/sign-attestation.sh access-review             # write, sign, commit
```

Refuses unless `git config user.email` is the owner's and signing works. Writes `compliance/attestations/<name>.yaml` dated today (UTC), expiring after the control's freshness window (365 days; access review 90), and commits that file alone. The three controls (UH-POL-01, UH-IR-01, UH-AC-01) read NOT_ASSESSED until then, and FAIL once an attestation expires.
