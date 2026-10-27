# Security Policy

## Supported Versions

There are no versioned releases yet. Fixes are made on branches, in this order:

| Branch    | Supported | Notes |
|-----------|-----------|-------|
| `develop` | Yes       | Every fix lands here first. |
| `staging` | Yes       | Receives fixes when a batch is promoted from `develop`. |
| `main`    | Yes       | Receives fixes when Stevie promotes `staging`. |

Older commits and forks are not patched.

## Verifying a Release

Every file in a GitHub release is signed with keyless cosign by this repository's release workflow and covered by build provenance. Verify before you run anything: `scripts/verify-release.sh <tag> --download` (details: `wiki/Releases.md`).

## Reporting a Vulnerability

We take the security of Unheaded seriously. If you believe you have found a security vulnerability, please report it to us as described below.

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

### How to Report

Email your findings to: **stevie@bellis.tech**

The same contact is published in machine-readable form (RFC 9116) at
`/.well-known/security.txt` on the dashboard backend; the source is
[`cmd/dashboard-backend/internal/server/security.txt`](cmd/dashboard-backend/internal/server/security.txt).

Include the following information to help us better understand and resolve the issue:

- **Type of vulnerability** (e.g., buffer overflow, injection, authentication bypass, etc.)
- **Location** (file path, function name, line number if applicable)
- **Description** of the vulnerability and potential impact
- **Proof of concept** or reproducible steps
- **Affected versions** (which versions of Unheaded are impacted)
- **Your contact information** (optional, but recommended for follow-up)

## Disclosure Timeline

We aim to address all security vulnerabilities within our vulnerability disclosure timeline:

- **Immediate (within 24 hours)**: Acknowledge receipt of your vulnerability report
- **Up to 7 days**: Initial assessment and confirmation of the vulnerability
- **Up to 90 days**: Development and testing of the fix
- **90 days**: Public disclosure of the vulnerability and availability of the patch

During this time, we will:
1. Confirm the vulnerability and determine its scope
2. Develop and test a fix
3. Release a patch update with the fix
4. Publish a security advisory

If you have a preferred disclosure timeline (e.g., you represent a large enterprise or need more time), please mention it in your report.

## What to Include in Security Reports

A good security report should include:

- **Summary**: A brief description of the issue
- **Vulnerability Type**: Classification (OWASP Top 10, CWE, etc.)
- **Affected Component**: Which service, module, or feature is impacted
- **Severity Assessment**: Your assessment of the severity (Critical, High, Medium, Low)
- **Steps to Reproduce**: Detailed instructions to reproduce the vulnerability
- **Potential Impact**: What an attacker could do if this vulnerability is exploited
- **Mitigation**: Any workarounds or mitigation strategies you've identified

## What NOT to Report

Please do **not** report the following as security vulnerabilities:

- **Bugs in dependencies**: Report these to the dependency maintainers
- **Issues in user applications**: These are outside the scope of Unheaded platform security
- **Social engineering or phishing**: Contact your local law enforcement
- **DoS attacks or performance issues** that don't involve a security vulnerability
- **Information disclosure from public sources** (e.g., published documentation, public repositories)
- **Features you wish existed** (submit as feature requests instead)

## Scope

### In Scope

The Unheaded platform infrastructure includes:

- **Core services**: Wotan (message bus), Timeguru (timeline), Captain (vision), Architect (ADRs), Micromanager (tasks)
- **State management**: Monad (unified state), Sophia (knowledge graph)
- **API gateway**: Gateway (public access point)
- **System components**: Dashboard backend, eBPF infrastructure, trace collection
- **Configuration management**: All configuration files and environment handling
- **Authentication and authorization**: Token handling, permission enforcement
- **Data protection**: Encryption, key management, secure deletion
- **Network security**: TLS/SSL configuration, network isolation, firewall rules
- **Access control**: Role-based access control (RBAC), audit logging

### Out of Scope

The following are **not** considered within the security scope of Unheaded:

- **User applications** built on top of Unheaded
- **Third-party integrations** that use Unheaded APIs
- **User data stored in user databases** (responsibility of user)
- **Infrastructure security** of user deployments
- **Vulnerabilities in dependencies** (report to dependency maintainers)
- **Social engineering** targeting users of Unheaded
- **Physical security** of infrastructure

## General Security Practices

While using Unheaded, please follow these security best practices:

### Deployment

- Keep Unheaded and all dependencies updated to the latest version
- Run services with minimal required permissions
- Use TLS/SSL for all network communication
- Implement network segmentation and firewall rules
- Enable audit logging and monitor for suspicious activity
- Use strong authentication mechanisms (mTLS, API keys, tokens)

### Configuration

- Never commit secrets to version control
- Use environment variables or secure vaults for sensitive configuration
- Review and harden default security settings
- Implement rate limiting on public endpoints
- Enable CORS only for trusted origins
- Implement request body size limits on HTTP servers

### Monitoring

- Monitor system logs for security events
- Set up alerts for failed authentication attempts
- Monitor resource usage for signs of attacks
- Implement distributed tracing to detect anomalies
- Use eBPF monitoring for deep system visibility

## Security Headers

Unheaded services implement the following security headers:

- `Strict-Transport-Security (HSTS)`: Enforce HTTPS
- `X-Content-Type-Options: nosniff`: Prevent MIME type sniffing
- `X-Frame-Options: DENY`: Prevent clickjacking
- `Content-Security-Policy`: Restrict resource loading
- `X-XSS-Protection`: Enable XSS protection

## Cryptography

Decisions and their reasons: [ADR-105](docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md).
Operator guide: [Post-Quantum Key Exchange](wiki/Post-Quantum-Key-Exchange.md).

**Key exchange: hybrid post-quantum on first-party TLS.** Every TLS server
and first-party mTLS client we build runs TLS 1.3 and negotiates
`X25519MLKEM768` (ML-KEM-768, FIPS 203, combined with X25519), so a session
recorded today stays confidential if either half holds. Two profiles in
`pkg/tls/pq.go`:

| Profile | Groups | Used where | A classical-only peer |
|---|---|---|---|
| Strict | `X25519MLKEM768`, `SecP256r1MLKEM768`, `SecP384r1MLKEM1024` only | both ends are ours: Wotan gRPC and wotan-client, `pkg/transport/mtls` | handshake fails |
| Preferred | Go's default (hybrids first, classical fallback) | servers third-party clients reach: Wotan HTTP and `/metrics`, the dashboard, load-balancer frontends, the WAF, zhend QUIC | gets X25519 |

Rust TLS (the WAF, zhend QUIC) uses rustls on the `aws-lc-rs` provider, which
offers `X25519MLKEM768` first; the `ring` provider (no ML-KEM) is gone.
Third-party outbound clients (SMTP relays, the CISA KEV feed, Splunk HEC,
load-balancer health checks to user backends) keep a TLS 1.2 floor: their
servers choose, and a modern one negotiates the hybrid. The load balancer's
`min_version`/`max_version: "1.2"` remain explicit operator opt-ins.
Handshake tests on every first-party surface assert the negotiated group.

**Signatures.** ML-DSA-65 (FIPS 204) signs Wotan `config.*` and `drift.*`
topic messages (per-prefix keys, verified by Wotan and re-verifiable by
subscribers), Gungnir configuration seals, and Yggdrasil evidence packs
(non-expiring evidence seals plus a signed chain of custody,
`yggdrasil-evidence verify`).

**Inventory and gates.** `compliance/crypto/cbom.cdx.json` is a CycloneDX 1.6
cryptographic bill of materials of every TLS, SSH, WireGuard, Postgres and
signature site, regenerated with
`go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json`.
`scripts/check-crypto-inventory.sh` fails when it is stale and
`scripts/check-pq-kex.sh` fails on any new classical-only key-exchange
configuration (a shrink-only baseline holds today's exceptions); both run in
CI and are proven able to fail by the meta-gate.

**Still classical, on purpose or until a decision:**
- The Well (PostgreSQL 16) negotiates P-256 only, until it moves to
  PostgreSQL 18 with `ssl_groups` (prepared, awaiting the owner).
- Host SSH offers `mlkem768x25519-sha256` first, but still accepts classical
  key exchange; making it hybrid-only is a lockout-relevant owner decision.
- WireGuard is X25519 plus a pre-shared key; the PSK is the post-quantum
  part and is only as strong as its secrecy and rotation.
- Certificates and handshake signatures (ECDSA, RSA, Ed25519): not exposed to
  harvest-now-decrypt-later; PQ certificates wait for the ecosystem.
- Links that run without TLS today (Wotan inside the compose stack, the edge
  in development) are plaintext, not merely classical; turning TLS on makes
  them hybrid with no further change.

**Not claimed:** CNSA 2.0 (it requires ML-KEM-1024 and ML-DSA-87; we use
ML-KEM-768 and ML-DSA-65).

Other primitives: TLS 1.3 cipher suites (AES-GCM, ChaCha20-Poly1305), X.509
certificates validated against configured roots, SHA-256 or stronger.

## Threat documentation

Honest, current-state threat-status docs live in `docs/security/`:

- **[`docs/security/application-threat-model.md`](docs/security/application-threat-model.md)** — the agent runtime + chat surface threat catalog (T1–T10), with status (CLOSED / RESIDUAL / OPEN-DOCUMENTED), evidence (commits, tests, audit-log entries), and explicit residual-risk rationale where defenses aren't fully closed today. Read this first if you're evaluating the AI agent surface.
- **[`docs/security/threat-register.md`](docs/security/threat-register.md)** — CVE-feed posture (CISA KEV + NIST NVD ingest), supply-chain advisories, refresh log per Round Table sprint.
- **[`docs/security/zhen-pq-threat-model.md`](docs/security/zhen-pq-threat-model.md)** — cryptographic primitive threats specific to Zhen Layer 0 (PQ signing, ML-DSA-65 enforcement on `config.*` topics).

The application threat model is **public on purpose**: every threat in the catalog is named with status, evidence, and (where applicable) the rationale for why a residual risk is acceptable today. We do not hide OPEN threats in fine print. Public-repo posture means a security researcher reading the doc can see the gaps.

## Acknowledgments

We appreciate the security research community and acknowledge researchers who report vulnerabilities responsibly. Upon request and with your permission, we will credit you in our security advisories.

## Contact

For security-related questions or concerns:

- **Security vulnerability reports**: stevie@bellis.tech
- **General inquiries**: See repository README for contact information

---

**Last updated**: 2026-05-02
**Version**: 1.1 (added application threat model link; T6 mutation-path closure documented)
