# Security Audit: Zero Customer Data Access Verification

**Task:** TASK-061 -- Security Audit -- Zero Customer Data Access Verification
**Date:** February 6, 2026
**Auditor:** Claude Opus 4.6 (automated)
**Priority:** P0
**Scope:** Full source code audit of ~/tmp/unheaded/ (all source files)
**Previous Audit:** docs/history/SECURITY-AUDIT-2026-01-30.md (injection/XSS focused)

---

## Executive Summary

This audit verifies the architectural claim that **no code path allows platform engineers to access customer data**. The codebase was exhaustively searched for patterns that could leak customer data, shared credentials, PII in logs, and violations of the isolation boundary between the platform zone and the customer zone.

**Overall Result: PASS with WARNINGS**

The Unheaded platform fundamentally upholds the zero customer data access principle. The architecture is sound: eBPF observability captures packet metadata (IPs, ports, sizes, flags) not packet contents, services communicate through Busboy rather than direct connections, and container hardening enforces strict isolation. However, several warnings require attention before production deployment.

---

## Verification Methodology

### 1. Source Code Pattern Search
- Searched all `.go`, `.rs`, `.nix`, `.toml`, `.yaml`, `.yml`, `.js` files
- Pattern categories: `customer`, `user_data`, `personal`, `PII`, `email`, `password`, `token`, `secret`, `credential`
- Searched for log statements containing sensitive field names
- Searched for `InsecureSkipVerify`, `skip_verify`, `insecure` patterns
- Searched for request body logging patterns
- Searched for shared credentials between platform and customer zones

### 2. Architectural Review
- Reviewed network segmentation in `nix/modules/networking.nix` and `nix/modules/hardening.nix`
- Reviewed container isolation in `nix/containers/base.nix` and per-service configs
- Reviewed `docker-compose.yml` for network segregation
- Reviewed Busboy message bus topic structure for data leak paths
- Reviewed eBPF trace-collector for packet content exposure

### 3. Observability Data Review
- Reviewed `cmd/trace-collector/src/events/packet.rs` for what data is captured
- Reviewed `cmd/dashboard-backend/internal/scraper/scraper.go` for metrics collection
- Reviewed `cmd/dashboard-backend/internal/packetflow/generator.go` for data model
- Reviewed `cmd/dashboard-backend/internal/metrics/aggregator.go` for stored data types

### 4. Secrets Management Review
- Reviewed `pkg/secrets/secrets.go` for data isolation enforcement
- Reviewed `cmd/unheaded-daemon/internal/config/config.go` for separation of customer/operator namespaces
- Reviewed secret store implementations (memory, file, vault)

---

## Findings

### CATEGORY 1: Customer Data Access Isolation

#### [PASS] F-001: No Service Reads Customer Application Data
**Severity:** N/A
**Files reviewed:** All files under `services/`, `cmd/`, `pkg/`

No service in the codebase reads, processes, or stores customer application data. All services operate exclusively on platform-internal data: timeline entries, task assignments, strategy documents, architecture decisions, and system metrics. The `demo-app` (10.10.10.254) exists only as a customer simulation target and has no code in the repository that accesses Unheaded internals.

#### [PASS] F-002: eBPF Traces Metadata, Not Content
**Severity:** N/A
**File:** `cmd/trace-collector/src/events/packet.rs`

The `PacketEvent` struct captures only:
- Process ID, Thread ID, Command name
- Interface index
- Packet length (size in bytes, not contents)
- Source/Destination IP addresses and ports
- IP protocol, direction, TCP flags

**Critically, no packet payload/content is captured.** The `RawPacketEvent` C struct has no buffer for packet data beyond headers. This is architecturally correct: observability sees packet counts and connection metadata, not packet contents.

#### [PASS] F-003: Dashboard Backend Collects Metrics Only
**Severity:** N/A
**Files:** `cmd/dashboard-backend/internal/scraper/scraper.go`, `cmd/dashboard-backend/internal/metrics/aggregator.go`

The scraper exclusively polls `/metrics` endpoints in Prometheus exposition format. It collects only numeric time-series data (counters, gauges, histograms) with labels. No request bodies, response bodies, or application data is collected. The `PacketFlow` data model contains only trace IDs, IPs, protocols, methods, paths, status codes, and latency measurements.

#### [PASS] F-004: Data Isolation Config Enforces Namespace Separation
**Severity:** N/A
**File:** `cmd/unheaded-daemon/internal/config/config.go` (lines 142-160)

The `DataIsolationConfig` struct enforces:
- Separate namespaces: `CustomerDataNamespace` ("customer") vs `OperatorDataNamespace` ("operator")
- Separate encryption keys: `CustomerKeyPath` vs `OperatorKeyPath`
- Audit logging enabled by default for all data access

#### [PASS] F-005: Secrets Manager Enforces Infrastructure-Only Secrets
**Severity:** N/A
**File:** `pkg/secrets/secrets.go` (line 4)

Package documentation explicitly states: "The Sacred Law: ZERO customer data in secrets - only infrastructure credentials." The secrets system handles only SecretTypeStatic (API keys), SecretTypeDynamic, SecretTypeCertificate, SecretTypeSSHKey, and SecretTypeKEK. No customer data types exist.

---

### CATEGORY 2: Network Segmentation and Isolation

#### [PASS] F-006: Firewall Default Deny with Explicit Allow
**Severity:** N/A
**Files:** `nix/modules/hardening.nix`, `nix/modules/networking.nix`

Both modules enforce default-deny firewall policies:
- INPUT chain drops all traffic not explicitly allowed
- Only container network (10.10.10.0/24) and loopback are permitted
- Each service only exposes its own service port
- FORWARD chain policy is DROP
- IP forwarding disabled (`net.ipv4.ip_forward = 0`)

#### [PASS] F-007: Services Communicate via Busboy, Not Direct Connections
**Severity:** N/A
**Files:** `docker-compose.yml`, `docs/ARCHITECTURE.md`, `docs/MICROSERVICES.md`

The architecture mandates all inter-service communication through the Busboy message bus. Services connect to Busboy at 10.10.10.10:9090 (or 172.28.1.1:5555 in Docker Compose). Direct service-to-service HTTP calls exist only for:
- Captain -> Timeguru (documented dependency for strategy/timeline coordination)
- Micromanager -> Timeguru and Captain (documented dependency)
These are intra-platform connections, not customer data paths.

#### [PASS] F-008: Container Hardening Applied Uniformly
**Severity:** N/A
**Files:** `nix/modules/hardening.nix`, `nix/containers/base.nix`

All containers receive:
- `NoNewPrivileges = true`
- `ProtectSystem = "strict"` (read-only /usr, /boot, /etc)
- `ProtectHome = true`
- `PrivateTmp = true`, `PrivateDevices = true`
- `ProtectKernelTunables/Modules/Logs = true`
- `MemoryDenyWriteExecute = true`
- `RestrictNamespaces = true`, `RestrictSUIDSGID = true`
- Seccomp filter: `@system-service ~@privileged ~@resources ~@obsolete ~@debug ~@mount ~@reboot ~@swap ~@module ~@raw-io`
- Kernel hardening: `dmesg_restrict=1`, `kptr_restrict=2`, `ptrace_scope=2`

#### [PASS] F-009: Docker Compose Uses Isolated Network
**Severity:** N/A
**File:** `docker-compose.yml`

All services are on a dedicated bridge network (`kingdom`, 172.28.0.0/16) with static IP assignments. The `cuirass` (control plane) service correctly:
- Sets `privileged: false`
- Uses `cap_drop: [MKNOD, AUDIT_WRITE]`
- Enforces `security_opt: [no-new-privileges:true]`
- Mounts Docker socket as read-only (`:ro`)

---

### CATEGORY 3: Credential and Secret Safety

#### [WARN] F-010: Grafana Default Password in Docker Compose
**Severity:** LOW
**File:** `docker-compose.yml` (line 471)

```yaml
GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_PASSWORD:-unheaded}
```

The Grafana admin password defaults to "unheaded" if the `GRAFANA_PASSWORD` environment variable is not set. While this is in an optional observability profile and intended for development, it should not ship to production.

**Recommendation:** Remove the default fallback value. Require `GRAFANA_PASSWORD` to be set explicitly, or generate a random password at first startup.

#### [WARN] F-011: BGP Peer Password Written to Config in Plaintext
**Severity:** MEDIUM
**File:** `pkg/network/policy_controller.go` (lines 1754-1756)

```go
if peer.Password != "" {
    sb.WriteString(fmt.Sprintf("  neighbor %s password %s\n", peer.Address, peer.Password))
}
```

BGP peer passwords are written to generated FRR configuration in plaintext. While this is standard for FRR configuration format, the generated config file should have restricted permissions.

**Recommendation:** Ensure generated config files have 0600 permissions. Consider using FRR's encrypted password support.

#### [PASS] F-012: Secrets Never Logged with Values
**Severity:** N/A
**Files:** `pkg/secrets/store/file.go`, `pkg/secrets/store/vault.go`, `pkg/secrets/store/memory.go`

All secret store implementations log only the secret path and operation type, never the secret value itself:
```go
s.log.Debug().Str("path", path).Msg("secret stored")
s.log.Debug().Str("path", path).Msg("secret deleted")
```

#### [PASS] F-013: No Shared Credentials Between Platform and Customer Zones
**Severity:** N/A

Exhaustive search for patterns `shared.*secret`, `shared.*credential`, `common.*key`, `global.*key` found no instances of credentials shared between platform and customer zones. The only `shared secret` references are in the age encryption implementation (Curve25519 key agreement), which is correct cryptographic usage.

---

### CATEGORY 4: Logging and PII Exposure

#### [PASS] F-014: Structured Logging Does Not Include Request Bodies
**Severity:** N/A
**Files:** All service middleware and handlers

Logging across all services uses structured fields (zerolog-style `.Str()`, `.Int()`, `.Dur()`) with only operational metadata:
- `service`, `request_id`, `trace_id`, `method`, `path`, `status`, `duration`
- No fields for request body, response body, headers containing auth tokens, or cookies

#### [PASS] F-015: No PII Patterns in Log Output
**Severity:** N/A

Searched all `.go` and `.rs` files for log statements containing email regex patterns, credit card patterns, or SSN patterns. No log statements include PII data. The WAF response inspector (`pkg/waf/inspection/response.go`) detects PII patterns (SSN, credit cards, email lists) in HTTP responses for security alerting, but redacts sensitive matches before logging:
```go
Redacted: redactSensitive(string(match)),
Context:  redactContext(getContext(body, match), string(match)),
```

#### [PASS] F-016: Secret CLI Commands Do Not Log Secret Values
**Severity:** N/A
**File:** `cmd/unheaded-cli/cmd/secret.go`

The CLI secret commands log only path, type, and version -- never the actual secret data:
```go
ctx.Logger.Info().Str("path", path).Str("type", secretType).Msg("setting secret")
ctx.Logger.Info().Str("path", path).Msg("deleting secret")
```

---

### CATEGORY 5: TLS and Transport Security

#### [WARN] F-017: Multiple InsecureSkipVerify Instances
**Severity:** MEDIUM
**Files:**
- `pkg/health/aggregator.go:942` -- gRPC health checks use insecure credentials when TLS is disabled
- `pkg/loadbalancer/health.go:54` -- Health checker hardcodes `InsecureSkipVerify: true`
- `pkg/audit/export/splunk.go:84` -- Splunk exporter can disable SSL verification
- `pkg/mesh/mtls/mtls.go:311` -- mTLS config has a `SkipVerify` option
- `pkg/deploy/pipeline/healthgate.go:160-185` -- Health gate has `InsecureSkipVerify` option
- `pkg/alerting/notify/email.go:193` -- Email notifier has `TLSInsecure` option

While most of these are configurable options (not hardcoded to true), the load balancer health checker hardcodes `InsecureSkipVerify: true` with the comment "Health checks may use self-signed certs." This is acceptable for internal health checks within the container network but should be configurable.

**Recommendation:**
1. Make `pkg/loadbalancer/health.go` InsecureSkipVerify configurable rather than hardcoded.
2. Ensure all InsecureSkipVerify options default to `false` and are only enabled explicitly.
3. Log a warning when InsecureSkipVerify is enabled in production.

#### [PASS] F-018: TLS Configuration Defaults Are Secure
**Severity:** N/A
**File:** `cmd/unheaded-daemon/internal/config/config.go`

Default configuration enables mTLS (`MTLSEnabled: true`) and seccomp (`SeccompEnabled: true`). The LXD config has a `SkipVerify` field but it defaults to `false`.

---

### CATEGORY 6: Busboy Message Bus Data Safety

#### [PASS] F-019: Busboy Topics Carry Operational Data Only
**Severity:** N/A
**File:** `docs/ARCHITECTURE.md` (topic table)

Busboy pub/sub topics carry only:
- `network.traces` -- eBPF packet metadata (IPs, ports, latency)
- `system.metrics` -- Container health metrics
- `timeline.updates` -- Roadmap changes
- `strategy.decisions` -- Strategic guidance
- `tasks.assignments` -- Task tracking
- `design.proposals` -- Architecture decisions
- `alerts.critical` -- System alerts
- `logs.aggregated` -- Centralized operational logs

No topic carries customer application data.

#### [PASS] F-020: WAF Inspects Outbound Responses for Data Leakage
**Severity:** N/A
**File:** `pkg/waf/inspection/response.go`

The WAF response inspector actively detects sensitive data in outbound HTTP responses:
- SSN patterns (`\b\d{3}-\d{2}-\d{4}\b`)
- Credit card numbers (Visa, Mastercard, Amex, Discover)
- Bulk email addresses (5+ in a response)
- Passwords, API keys, private keys, AWS access keys

This is a defense-in-depth measure that would catch accidental data exposure.

---

### CATEGORY 7: Compliance Framework Alignment

#### [PASS] F-021: Compliance Standards Reference Personal Data Correctly
**Severity:** N/A
**Files:** `pkg/compliance/standards/gdpr.go`, `pkg/compliance/standards/soc2.go`, `pkg/compliance/standards/pci.go`

The compliance framework correctly models GDPR, SOC2, and PCI-DSS requirements around personal data handling. These are compliance check definitions (control descriptions), not actual data processing. The platform does not process personal data; these controls exist to validate that customers using the platform can meet their compliance obligations.

#### [PASS] F-022: Audit System Logs Operations, Not Data
**Severity:** N/A
**File:** `pkg/secrets/secrets.go` (AuditEvent struct)

Audit events record:
- Timestamp, Operation type, Path, Success/failure, Error message, Actor, Metadata
- Secret values are never included in audit events

---

### CATEGORY 8: Previously Identified Vulnerabilities (Cross-Reference)

#### [WARN] F-023: Command Injection Vulnerabilities Still Open
**Severity:** HIGH (from previous audit)
**Reference:** `docs/history/SECURITY-AUDIT-2026-01-30.md`

The following P0/P1 vulnerabilities from the January 30 audit remain relevant:
1. **Command injection via hook system** (`pkg/deploy/pipeline/hooks.go:405`) -- bash -c with unsanitized input
2. **XSS in WAF response pages** (`pkg/waf/response/response.go:237`) -- unescaped requestID
3. **Template injection** (`pkg/waf/response/response.go:113-120`) -- runtime template parsing
4. **Health check command execution** (`pkg/health/aggregator.go:948`) -- arbitrary command exec
5. **Network policy command injection** (`pkg/network/policy_controller.go:1621,1653,1725`) -- shell metacharacters

While these are not customer data access violations, they represent attack vectors that could potentially be used to escalate privileges and breach isolation boundaries.

**Recommendation:** Address P0 items before production deployment. These are preconditions for the isolation guarantee to hold under adversarial conditions.

---

## Summary Table

| ID | Finding | Category | Result | Severity |
|----|---------|----------|--------|----------|
| F-001 | No service reads customer app data | Isolation | PASS | -- |
| F-002 | eBPF traces metadata, not content | Isolation | PASS | -- |
| F-003 | Dashboard collects metrics only | Isolation | PASS | -- |
| F-004 | Data isolation config enforces namespaces | Isolation | PASS | -- |
| F-005 | Secrets manager infrastructure-only | Isolation | PASS | -- |
| F-006 | Firewall default deny | Network | PASS | -- |
| F-007 | Services communicate via Busboy | Network | PASS | -- |
| F-008 | Container hardening applied uniformly | Network | PASS | -- |
| F-009 | Docker Compose isolated network | Network | PASS | -- |
| F-010 | Grafana default password | Credentials | WARN | LOW |
| F-011 | BGP password in plaintext config | Credentials | WARN | MEDIUM |
| F-012 | Secrets never logged with values | Credentials | PASS | -- |
| F-013 | No shared credentials cross-zone | Credentials | PASS | -- |
| F-014 | No request bodies in logs | Logging | PASS | -- |
| F-015 | No PII patterns in log output | Logging | PASS | -- |
| F-016 | CLI does not log secret values | Logging | PASS | -- |
| F-017 | InsecureSkipVerify instances | TLS | WARN | MEDIUM |
| F-018 | TLS defaults are secure | TLS | PASS | -- |
| F-019 | Busboy topics carry ops data only | Data Flow | PASS | -- |
| F-020 | WAF inspects for data leakage | Data Flow | PASS | -- |
| F-021 | Compliance framework correct | Compliance | PASS | -- |
| F-022 | Audit logs ops, not data | Compliance | PASS | -- |
| F-023 | Prior injection vulns still open | Cross-ref | WARN | HIGH |

**Results: 18 PASS, 4 WARN, 0 FAIL**

---

## Recommendations

### Before Production (P0)

1. **Fix injection vulnerabilities from January 30 audit** (F-023). While these are not direct customer data access paths, they could be exploited to bypass isolation under adversarial conditions. Specifically:
   - Sandbox hook execution in deploy pipeline
   - Use `html/template` for WAF response pages
   - Allowlist health check commands
   - Use native Go libraries instead of shell for network policy

2. **Remove Grafana default password** (F-010). Require explicit password configuration or auto-generate.

### Before General Availability (P1)

3. **Make load balancer health check TLS verification configurable** (F-017). Replace the hardcoded `InsecureSkipVerify: true` with a configuration option defaulting to `false`.

4. **Secure BGP config generation** (F-011). Ensure generated FRR config files have 0600 permissions and consider FRR encrypted password support.

5. **Add runtime validation** that services cannot subscribe to topics outside their authorized scope in Busboy.

6. **Implement mTLS** for inter-container communication as noted in the architecture docs ("future" item). This adds cryptographic enforcement to the network isolation.

### Ongoing

7. **Automate this audit** as a CI/CD gate. Grep-based checks for new patterns (customer data, PII, InsecureSkipVerify) should fail the build.

8. **Regular rotation** of all credentials including Grafana admin, BGP peers, and TLS certificates.

---

## Conclusion

The Unheaded platform's zero customer data access claim is **architecturally valid**. The eBPF layer captures packet metadata (IPs, ports, sizes, flags) without payload content. The Busboy message bus carries only operational data (metrics, traces, tasks, timeline). Container hardening with seccomp, capability restrictions, and network firewalls enforces isolation at the OS level. Secrets management separates customer and operator namespaces with audit logging.

The four warnings identified are configuration hygiene issues (default passwords, TLS skip options) and pre-existing injection vulnerabilities from the January 30 audit. None represent an active path for customer data access. However, the injection vulnerabilities (F-023) must be remediated before production because they could theoretically be leveraged by an attacker to escape the isolation boundary.

**Audit verdict: The Sacred Law holds. Zero customer data access is enforced by architecture, not just policy.**

---

*Generated: February 6, 2026*
*Auditor: Claude Opus 4.6*
*Next scheduled audit: Before production deployment*
