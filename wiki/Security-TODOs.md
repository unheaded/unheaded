# Security TODOs

Open security work is no longer a hand-kept list. It is the **findings register** (ADR-098): evidence decides what is open, and the register only triages (severity, lockout risk, remediation order, accepted risk with an expiry).

| What | Where |
|---|---|
| Open findings + triage | [compliance/findings/register.yaml](../compliance/findings/register.yaml) |
| Live view | dashboard `:20000/compliance.html` |
| Process | [ADR-098](../docs/adr/ADR-098-findings-register-and-baseline-modes.md), [ADR-099](../docs/adr/ADR-099-remediation-as-code-and-control-profiles.md) |
| CI-side findings | gosec ratchet, gitleaks baseline, govulncheck, trivy, cargo-audit (see [[Security]]) |

The February 2026 list ("no auth on services, permissive CORS, no inter-service TLS, privileged cuirass, hardcoded passwords") is historical. Most of it was worked through in the 2026-08 → 09 remediation program; see the source below.

---

> **Sources:** [docs/security/SECURITY_TODOs_2-9-26.md](../docs/security/SECURITY_TODOs_2-9-26.md) (historical) · [compliance/findings/register.yaml](../compliance/findings/register.yaml)
