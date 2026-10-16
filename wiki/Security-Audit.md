# Security Audit

The 2026-02-06 audit (TASK-061) verifying the zero-user-data-access claim (user-facing: NIST 800-207 Zero Trust Architecture) across the whole source tree, and where its warnings stand now. Current findings live in the register, not here: see [[Security]].

## Result: PASS with warnings (18 pass, 4 warn, 0 fail)

Method: pattern search across all source, architectural review of the isolation boundary, review of what observability actually captures, secrets handling review.

| Area | Findings | Result |
|------|----------|--------|
| Isolation | F-001–005: no service reads user app data; eBPF captures metadata (IPs, ports, sizes, flags), not payloads; dashboard collects metrics only; secrets manager is infrastructure-only | PASS |
| Network | F-006–009: default-deny firewall, services talk via Wotan, uniform container hardening, isolated compose network | PASS |
| Credentials | F-010 Grafana default password (LOW), F-011 BGP password in plaintext config (MEDIUM); F-012–013 pass | WARN |
| Logging | F-014–016: no request bodies, PII or secret values in logs | PASS |
| TLS | F-017 `InsecureSkipVerify` (MEDIUM); F-018 defaults secure | WARN |
| Data flow | F-019–020: Wotan topics carry ops data only; WAF inspects for leakage | PASS |
| Compliance | F-021–022: framework mapping correct; audit logs record operations, not data | PASS |
| Cross-ref | F-023 injection findings from the 2026-01-30 audit still open (HIGH) | WARN |

## The four warnings, 2026-10-16

| ID | Then | Now |
|----|------|-----|
| F-010 | Grafana shipped a default admin password | Fixed: compose fails to start unless `GRAFANA_ADMIN_PASSWORD` is set in `.env` |
| F-011 | BGP neighbor password in plaintext config | Partly: `pkg/network/policy_controller.go` still writes `neighbor … password …` into the FRR config, now mode 0640 (not world-readable). FRR encrypted passwords not adopted |
| F-017 | Hardcoded `InsecureSkipVerify` in the LB health checker | Kept, documented: `pkg/loadbalancer/health.go` health probes only (`#nosec G402` with rationale); proxy traffic uses `pkg/mesh/mtls`. The other four uses are LICH campaigns probing TLS posture on purpose |
| F-023 | Injection findings (deploy hooks, WAF templates, health commands, shell-based network policy) | Worked through by the 2026-07-29 remediation program ("fix all findings", gosec rule-ID ratchet); see `docs/security/findings-remediation-2026-07-29.md` |

## Recommendations still open

- mTLS for all inter-service traffic (ADR-034 default; not everywhere yet).
- Runtime check that a service can only subscribe to topics in its scope.
- Turn the audit's grep patterns (user data, PII, `InsecureSkipVerify`) into a CI gate. gosec and gitleaks gates cover part of it.

---

> **Source:** [docs/security/SECURITY_AUDIT.md](../docs/security/SECURITY_AUDIT.md) (2026-02-06) · `compliance/findings/register.yaml` · [findings-remediation-2026-07-29.md](../docs/security/findings-remediation-2026-07-29.md)
