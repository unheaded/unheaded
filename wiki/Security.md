# Security

## Policy

Security is not optional. Every decision is evaluated through the security lens:

- Does this access customer data? → **NO**
- Does this weaken isolation? → **NO**
- Does this skip hardening? → **NO**

## Key Principles

- eBPF traceability from packet zero
- Zero customer data access — architectural isolation enforced
- Container hardening: seccomp, capabilities, read-only FS
- Network policies: explicit allow, default deny
- TLS 1.3 minimum for external traffic
- Secrets: never in code, environment, or logs

## Related Pages

- [[Security Audit|Security-Audit]]
- [[Security TODOs|Security-TODOs]]
- [[LICH Campaigns|LICH-Campaigns]]
- [[Dark Grimoire|Dark-Grimoire]]

---

> **Sources:** [SECURITY.md](../SECURITY.md) · [docs/SECURITY.md](../docs/SECURITY.md)
