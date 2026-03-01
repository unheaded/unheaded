# Unheaded Kingdom Threat Model

**Version:** 1.0
**Date:** February 28, 2026
**Classification:** Internal — Grimoire Knowledge Base
**Author:** BlackMage (Tomb of Knowledge)
**Review Status:** Initial assessment, pending Lich campaign validation

---

## 1. System Overview

The Unheaded Kingdom is a configuration management automation platform with the following critical components:

- **10 microservices** communicating via Wotan message bus (gRPC primary, HTTP fallback)
- **eBPF data plane** providing L2-L7 packet tracing
- **Shield boundary** (edge XDP programs) enforcing protocol containment
- **Monad wire protocol** (20 bytes) embedded in IPv6 Hop-by-Hop extension headers
- **Sophia dictionary system** providing exponent-encoded field semantics
- **LXD/containerd/NixOS containers** with hardened seccomp profiles
- **HAProxy edge/internal LBs** and Nginx per-app sidecars
- **Bare metal hosts** (WEST: production, EAST: staging) connected via WireGuard overlay

### Network Topology

```
Internet
    |
    X (Shield / WAF / TLS termination)
    |
    +-- HAProxy Edge (21080/21443) -- TLS 1.3, rate limiting
    |       |
    |       +-- HAProxy Internal (21081) -- service routing
    |               |
    +-- lxdbr0 (10.10.10.0/24)
    |       |
    |       +-- Gateway (10.10.10.100:21000/21443)
    |       +-- Wotan (10.10.10.10:18000/18001)
    |       +-- Services (10.10.10.20-30:19000-19005)
    |       +-- Apps (10.10.10.200+:20000-20002)
    |       +-- Nginx sidecars (per-app proxy)
    |
    +-- WireGuard (fd00:dead:beef::/48)
    |       |
    |       +-- WEST (fd00:dead:beef::1) -- dev/prod
    |       +-- EAST (fd00:dead:beef::2) -- staging
    |       +-- Tomb (fd00:dead:beef::3) -- security testing
```

### Port Surface (Doom Range: 16666-26666)

| Port Range | Services | Exposure Level |
|------------|----------|---------------|
| 16666-16671 | protocol-api, dashboard-backend, kanban-app, trace-collector | Internal only |
| 17000-17001 | unheaded-daemon (HTTP + gRPC) | Internal only |
| 18000-18001 | Wotan (HTTP + gRPC) | Internal only |
| 19000-19005 | timeguru, architect, captain, micromanager, monad, sophia | Internal only |
| 20000-20002 | dashboard, kanban, wiki | Internal + optional external |
| 21000-21443 | Gateway (HTTP/HTTPS) | External-facing |
| 26000-26666 | User applications | External-facing |

---

## 2. Threat Actors

### 2.1 External Attacker (Untrusted Internet)

**Motivation:** Data theft, service disruption, infrastructure compromise
**Capability:** Moderate to advanced (automated scanning, exploit kits, APT tools)
**Access:** Only through Shield boundary (gateway ports 21000/21443)
**MITRE ATT&CK alignment:** TA0001 (Initial Access), TA0040 (Impact)

### 2.2 Compromised Container

**Motivation:** Lateral movement, privilege escalation, data exfiltration
**Capability:** Limited by seccomp profile, capability bounding set, network policy
**Access:** Internal network (lxdbr0, 10.10.10.0/24), Wotan topics
**MITRE ATT&CK alignment:** TA0008 (Lateral Movement), TA0004 (Privilege Escalation)

### 2.3 Malicious Insider / Compromised Developer

**Motivation:** Supply chain attack, backdoor insertion, credential theft
**Capability:** Git commit access, CI/CD pipeline access, service configuration
**Access:** Full repo access, deployment credentials, WireGuard tunnel
**MITRE ATT&CK alignment:** TA0001 (Initial Access via trusted supply chain), TA0003 (Persistence)

### 2.4 Rogue BPF Program

**Motivation:** Protocol manipulation, silent data exfiltration, observability evasion
**Capability:** Kernel-level access (within BPF verifier constraints)
**Access:** Monad register file (read/write), Anamnesis ring buffers, Sophia maps
**MITRE ATT&CK alignment:** TA0005 (Defense Evasion), TA0009 (Collection)

### 2.5 Quantum Adversary (Future)

**Motivation:** Cryptographic break of classical key exchange, identity spoofing
**Capability:** Quantum computing (Shor's algorithm against RSA/ECDH)
**Access:** Network-level (passive collection now, active decryption later)
**MITRE ATT&CK alignment:** TA0006 (Credential Access)

---

## 3. Attack Surface Analysis

### 3.1 Network Attack Surface

#### AS-NET-01: Gateway TLS Termination

**Component:** HAProxy edge (21443) + Gateway (21000/21443)
**Threat:** TLS downgrade, certificate impersonation, protocol confusion
**MITRE:** T1557 (Adversary-in-the-Middle)
**Current mitigations:**
- TLS 1.3 minimum enforced
- HSTS headers
- Certificate pinning (planned)
**Residual risk:** MEDIUM — depends on certificate management hygiene
**Lich campaign:** LICH-002 (Web vulnerability discovery)

#### AS-NET-02: lxdbr0 Bridge Network

**Component:** Internal bridge network 10.10.10.0/24
**Threat:** ARP spoofing, traffic interception between containers, unauthorized service access
**MITRE:** T1557.002 (ARP Cache Poisoning), T1046 (Network Service Discovery)
**Current mitigations:**
- Default-deny firewall rules per container
- eBPF packet marking (when deployed)
**Residual risk:** HIGH — flat L2 network allows broadcast-domain attacks if a container is compromised
**Lich campaign:** LICH-001 (Network reconnaissance), LICH-010 (Lateral movement)

#### AS-NET-03: WireGuard Tunnel

**Component:** fd00:dead:beef::/48 overlay between WEST, EAST, Tomb
**Threat:** Key compromise leading to tunnel injection, replay attacks
**MITRE:** T1090 (Proxy), T1572 (Protocol Tunneling)
**Current mitigations:**
- WireGuard uses Noise protocol (Curve25519, ChaCha20, Poly1305)
- Pre-shared symmetric keys
- Point-to-point /30 subnet limiting blast radius
**Residual risk:** LOW — WireGuard's cryptographic design is strong; key management is the risk factor

#### AS-NET-04: Doom Range Port Exposure

**Component:** All services on ports 16666-26666
**Threat:** Port scanning reveals service fingerprints, version information
**MITRE:** T1046 (Network Service Discovery), T1592 (Gather Victim Host Information)
**Current mitigations:**
- High ports reduce accidental conflict with standard services
- Firewall rules restrict external access to gateway only
**Residual risk:** LOW — only gateway ports exposed externally

### 3.2 Service Attack Surface

#### AS-SVC-01: Wotan Message Bus

**Component:** Wotan (18000/18001), gRPC + HTTP
**Threat:** Topic injection, message spoofing, denial of service via message flooding, unauthorized subscription to sensitive topics
**MITRE:** T1071 (Application Layer Protocol), T1498 (Network Denial of Service)
**Current mitigations:**
- Auth middleware on all endpoints (NoopAuthenticator in dev, APIKeyAuthenticator in prod)
- Topic-level ACLs (planned)
- Ring buffer bounds (10,000 entries) prevent unbounded memory growth
**Residual risk:** HIGH — Wotan is the nervous system; compromise here cascades everywhere
**Lich campaign:** LICH-002, LICH-008 (Data exfiltration via Wotan topics)

#### AS-SVC-02: Sophia Dictionary System

**Component:** BPF hash maps (kernel), structured tables (userspace)
**Threat:** Dictionary poisoning (changing field semantics), hot-swap race conditions, dictionary exfiltration revealing protocol internals
**MITRE:** T1565 (Data Manipulation), T1005 (Data from Local System)
**Current mitigations:**
- Sophia dictionaries loaded from versioned config files
- BPF map access requires CAP_BPF capability
- Dictionary changes emit events to Anamnesis
**Residual risk:** MEDIUM — hot-swap mechanism is a time-of-check/time-of-use risk

#### AS-SVC-03: Unheaded Daemon Control Plane

**Component:** unheaded-daemon (17000/17001)
**Threat:** Unauthorized state manipulation, drift injection, reconciliation bypass
**MITRE:** T1543 (Create or Modify System Process), T1036 (Masquerading)
**Current mitigations:**
- Auth middleware
- State changes logged to Wotan
- Desired state in Git (immutable source of truth)
- Drift detection every 30 seconds
**Residual risk:** MEDIUM — daemon has broad system privileges for reconciliation

#### AS-SVC-04: Service Discovery

**Component:** pkg/discovery/ (four-layer: Wotan/port-scan/convention/static)
**Threat:** Service impersonation via rogue registration, TTL manipulation, convention-scan path traversal
**MITRE:** T1036 (Masquerading), T1584 (Compromise Infrastructure)
**Current mitigations:**
- TTL-based registration with automatic reaping
- Convention scan reads from fixed /opt/unheaded/ paths
- Static fallback in configs/services.yaml
**Residual risk:** MEDIUM — Wotan registration lacks cryptographic identity verification

#### AS-SVC-05: Log Aggregation Pipeline

**Component:** pkg/logagg/ (ring buffer, zerolog hook, dashboard live tail)
**Threat:** Log injection (forged trace_ids, misleading messages), ring buffer overflow to suppress evidence, SSE stream hijacking
**MITRE:** T1070 (Indicator Removal), T1565.001 (Stored Data Manipulation)
**Current mitigations:**
- Structured JSON logging (harder to inject than plain text)
- Ring buffer capacity of 10,000 entries
- Log forwarding via zerolog.Hook
**Residual risk:** MEDIUM — no log signing or tamper detection

### 3.3 Authentication Attack Surface

#### AS-AUTH-01: Authentication Framework

**Component:** pkg/auth/ (Noop, APIKey, JWT, RBAC)
**Threat:** Default NoopAuthenticator left in production, API key leakage, JWT algorithm confusion (alg=none), role escalation
**MITRE:** T1078 (Valid Accounts), T1550.001 (Application Access Token)
**Current mitigations:**
- Pluggable authenticator design (swap via config)
- RBAC authorizer layer
- Audit logger for security events
- MaxHeaderBytes 1<<20 on protocol-api, doom-bridge, trace-collector-go
- Rate limiter with TrustedProxies guard on X-Forwarded-For
**Residual risk:** HIGH — NoopAuthenticator as dev default is a deployment misconfiguration risk

#### AS-AUTH-02: Secrets Management

**Component:** SOPS + age encryption for secrets
**Threat:** Age private key compromise, SOPS metadata leakage, secrets mounted as files readable by compromised process
**MITRE:** T1552 (Unsecured Credentials), T1555 (Credentials from Password Stores)
**Current mitigations:**
- Secrets never in environment variables (mounted as files)
- Secrets never in Git or logs
- ReadOnlyPaths enforced on containers
**Residual risk:** MEDIUM — file-mounted secrets are readable by any process in the container

### 3.4 Container Attack Surface

#### AS-CTR-01: Container Escape

**Component:** LXD/containerd/NixOS containers
**Threat:** Kernel exploit leading to host breakout, capability abuse, device access
**MITRE:** T1611 (Escape to Host)
**Current mitigations:**
- CapabilityBoundingSet restricted to CAP_NET_BIND_SERVICE
- NoNewPrivileges = true
- PrivateDevices, ProtectKernelTunables, ProtectControlGroups
- Seccomp filter: @system-service minus @privileged minus @resources
- ProtectSystem = strict, ProtectHome = true
**Residual risk:** LOW — defense-in-depth hardening is comprehensive; kernel 0-day remains theoretical risk

#### AS-CTR-02: NixOS Immutability Bypass

**Component:** NixOS container definitions (nix/containers/)
**Threat:** Nix store corruption, runtime overlay mounts bypassing read-only filesystem
**MITRE:** T1027 (Obfuscated Files), T1547 (Boot or Logon Autostart Execution)
**Current mitigations:**
- ReadOnlyPaths = ["/etc", "/usr"]
- ReadWritePaths limited to /opt/unheaded/references (data only)
- SystemCallFilter blocks mount-related syscalls
**Residual risk:** LOW — NixOS immutability is structurally enforced

### 3.5 eBPF Attack Surface

#### AS-BPF-01: Malicious BPF Program Loading

**Component:** eBPF programs (packet_marker, flow_tracker, latency_probe)
**Threat:** Unauthorized BPF program injection, verifier bypass exploits, BPF map corruption
**MITRE:** T1014 (Rootkit), T1543 (Create or Modify System Process)
**Current mitigations:**
- BPF program loading requires CAP_BPF (restricted capability)
- BPF verifier enforces safety properties
- Programs loaded from compiled Rust/Aya binaries (not JIT from untrusted input)
**Residual risk:** MEDIUM — BPF verifier bugs are occasionally discovered (CVE history exists)

#### AS-BPF-02: Anamnesis Ring Buffer Exfiltration

**Component:** BPF ring buffers recording packet events
**Threat:** Reading ring buffer data to reconstruct packet flows, timing analysis, metadata extraction
**MITRE:** T1040 (Network Sniffing), T1005 (Data from Local System)
**Current mitigations:**
- Ring buffer file descriptors restricted to trace-collector process
- Container isolation prevents cross-container access
**Residual risk:** LOW — requires host-level access or trace-collector compromise

---

## 4. STRIDE Analysis

| Threat | Category | Target | Severity | Mitigation Status |
|--------|----------|--------|----------|------------------|
| Shield bypass (direct internal access) | Spoofing | Gateway | CRITICAL | Firewall rules, Shield XDP |
| Wotan message spoofing | Spoofing | Message bus | HIGH | Auth middleware (planned: mTLS) |
| Service identity impersonation | Spoofing | Discovery | HIGH | Planned: PQ identity binding |
| Monad register tampering | Tampering | Wire protocol | HIGH | CRC-16 integrity check |
| Log injection / deletion | Tampering | Observability | MEDIUM | Structured logging (no signing) |
| Sophia dictionary swap | Tampering | Semantics | MEDIUM | Versioned config, audit events |
| Anamnesis data leak | Information Disclosure | Traces | MEDIUM | FD restrictions, container isolation |
| API key exposure in logs | Information Disclosure | Auth | HIGH | Secret filtering in zerolog |
| Wotan topic enumeration | Information Disclosure | Architecture | MEDIUM | Topic ACLs (planned) |
| Wotan flooding (msg storm) | Denial of Service | Message bus | HIGH | Ring buffer bounds, rate limiting |
| BPF program CPU exhaustion | Denial of Service | Data plane | MEDIUM | BPF verifier instruction limits |
| HAProxy connection exhaustion | Denial of Service | Edge | HIGH | Rate limiting, connection limits |
| NoopAuthenticator in prod | Elevation of Privilege | All services | CRITICAL | Config-driven auth selection |
| Container capability abuse | Elevation of Privilege | Host | LOW | Minimal capability set |
| BPF map write from userspace | Elevation of Privilege | Protocol | MEDIUM | CAP_BPF restriction |
| Dictionary poisoning | Repudiation | Semantics | MEDIUM | Anamnesis audit trail |

---

## 5. Critical Attack Paths

### Path 1: Gateway to Wotan (External Attacker)

```
Internet
  -> HAProxy edge (TLS termination)
    -> Gateway HTTP handler (input validation weakness?)
      -> Internal service request (SSRF?)
        -> Wotan gRPC (18001) direct access
          -> Subscribe to system.discovery topic
            -> Full service map enumerated
              -> Targeted attack on individual services
```

**Likelihood:** Medium
**Impact:** Critical (full service map disclosure enables targeted attacks)
**Mitigations needed:** SSRF protection on gateway, Wotan topic ACLs, network segmentation

### Path 2: Compromised Container Lateral Movement

```
Container A compromised (e.g., kanban-app via XSS)
  -> ARP scan on lxdbr0 (10.10.10.0/24)
    -> Discover all service IPs
      -> Connect to Wotan (18001) from internal network
        -> Subscribe to all topics
          -> Exfiltrate Monad protocol state
            -> Read Sophia dictionaries
              -> Understand protocol semantics
```

**Likelihood:** Medium
**Impact:** High (protocol internals exposed)
**Mitigations needed:** Per-container network policy (deny all, allow specific), mTLS between services

### Path 3: Supply Chain via Git

```
Malicious commit to unheaded repo
  -> Modified service binary (backdoored timeguru)
    -> Deployed via CI/CD
      -> Runs with valid auth credentials
        -> Publishes to Wotan as trusted service
          -> Injects false metrics / logs
            -> Masks further attack activity
```

**Likelihood:** Low (requires write access to repo)
**Impact:** Critical (trusted code path, full service privileges)
**Mitigations needed:** Mandatory code review, SBOM verification, binary reproducibility, signed commits

### Path 4: BPF Program Injection

```
Host compromise (kernel exploit or SSH key theft)
  -> Load malicious BPF program at XDP hook
    -> Intercept all Monad protocol bytes
      -> Modify trace_id to correlate with external C2
        -> Exfiltrate data via side-channel in Monad fields
          -> Shield egress does not detect (data hidden in legitimate protocol bytes)
```

**Likelihood:** Low (requires host access)
**Impact:** Critical (invisible data exfiltration within protocol)
**Mitigations needed:** BPF program attestation, runtime integrity monitoring, host IDS

### Path 5: Auth Misconfiguration

```
Production deployment with NoopAuthenticator (default dev config)
  -> All endpoints accept unauthenticated requests
    -> Attacker discovers /api/v1/ endpoints
      -> Direct access to unheaded-daemon control plane
        -> Modify desired state
          -> Deploy attacker-controlled containers
            -> Full infrastructure compromise
```

**Likelihood:** Medium (configuration mistake during deployment)
**Impact:** Critical (complete infrastructure takeover)
**Mitigations needed:** Deployment checklist enforcing auth config, smoke test for auth, alerting on NoopAuthenticator in prod

---

## 6. Risk Summary

| Risk ID | Description | Likelihood | Impact | Overall | Priority |
|---------|-------------|-----------|--------|---------|----------|
| R-01 | NoopAuthenticator deployed to production | Medium | Critical | CRITICAL | P0 |
| R-02 | Wotan message bus accessed from compromised container | Medium | High | HIGH | P1 |
| R-03 | lxdbr0 flat network enables lateral movement | Medium | High | HIGH | P1 |
| R-04 | Log injection masking attack activity | Medium | Medium | MEDIUM | P2 |
| R-05 | Sophia dictionary hot-swap race condition | Low | High | MEDIUM | P2 |
| R-06 | Service discovery poisoning via rogue registration | Medium | Medium | MEDIUM | P2 |
| R-07 | Gateway SSRF to internal services | Low | Critical | MEDIUM | P2 |
| R-08 | BPF program injection on compromised host | Low | Critical | MEDIUM | P3 |
| R-09 | WireGuard key compromise | Low | High | MEDIUM | P3 |
| R-10 | Quantum attack on classical key exchange | Low (future) | High | LOW | P4 |

---

## 7. Recommended Lich Campaigns

Based on this threat model, the following Lich campaigns should be prioritized:

| Campaign | Target | Validates Risk |
|----------|--------|---------------|
| LICH-001 | Network reconnaissance of lxdbr0 | R-03 |
| LICH-002 | Gateway and web service vulnerability scan | R-07 |
| LICH-005 | API key brute force against AuthenticatorAPI | R-01 |
| LICH-008 | Data exfiltration via Wotan topic subscription | R-02 |
| LICH-010 | Lateral movement from compromised container | R-02, R-03 |

---

## 8. Appendix: MITRE ATT&CK Mapping

| Kingdom Component | Relevant Techniques |
|-------------------|-------------------|
| Shield/Gateway | T1190, T1557, T1071, T1090 |
| Wotan | T1071, T1498, T1565, T1105 |
| Sophia | T1565, T1005, T1027 |
| Containers | T1611, T1610, T1053, T1543 |
| eBPF/BPF | T1014, T1543, T1040, T1205 |
| Auth Framework | T1078, T1550, T1552, T1556 |
| Service Discovery | T1046, T1036, T1018, T1135 |
| Log Pipeline | T1070, T1565, T1530 |
| WireGuard | T1572, T1090, T1573 |

---

**Next review:** After LICH-001, LICH-002, LICH-010 campaign execution
**Owner:** BlackMage (Tomb of Knowledge)
