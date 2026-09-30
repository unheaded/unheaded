# Attestations

Signed, expiring human statements for controls no automated check can
evidence: policy approval, periodic reviews, training (ADR-097). They are
**self-attested**: the dashboard labels them so, and they never stand in for
machine evidence where a check can exist.

One file per attestation, named as the control's evidence ref names it:

```yaml
statement: The information security policy (docs/...) was reviewed and approved.
attested_by: Stevie Bellis
attested_at: 2026-10-01   # YYYY-MM-DD; the evidence date (freshness applies)
expires_at: 2027-10-01    # after this date the control FAILS
```

Rules the collector (`pkg/compliance/crosswalk/collect.Attestations`) enforces:

- **Signed.** The latest commit touching the file must carry a good GPG
  signature (`git log -1 %G? == G`). Unsigned = FAIL: a claim nobody vouched
  for.
- **Unexpired.** On or after `expires_at` = FAIL.
- **Not future-dated.** `attested_at` after today = no evidence.
- **Missing file** = no evidence (the control stays NOT_ASSESSED).
- Unknown fields are rejected; 64 KiB cap.

Only the person accountable for the statement should write and sign it.
Controls currently waiting for one: see `evidence` entries of kind
`attestation` in `../catalog/controls.yaml`.
