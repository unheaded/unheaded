# Security Overview

## Principles

### NIST 800-207 Zero Trust Architecture

The foundational security principle of the Unheaded Kingdom: **NIST 800-207 Zero Trust Architecture** is enforced architecturally, not by policy. User applications ("the head") are isolated by design -- they communicate only through the gateway and never have access to infrastructure internals.

**Isolation enforcement:**
- User containers cannot subscribe to Wotan infrastructure topics
- User network traffic is firewalled from the internal service mesh (10.10.10.0/24)
- eBPF traces are read-only from user perspective (observability data flows outward, never inward)
- No shared storage between user and infrastructure containers
- User apps talk to the gateway; the gateway talks to services; services talk to Wotan

### Default Deny

Every container starts with all ports closed and all capabilities dropped. Access is explicitly granted:

```nix
networking.firewall = {
  enable = true;
  allowedTCPPorts = [ ]; # Explicit allow only
  extraCommands = ''
    iptables -A INPUT -s 10.10.10.0/24 -j ACCEPT
    iptables -A INPUT -j DROP
  '';
};
```

### Service Isolation

One service, one account (2026-10-18): every service runs as its own system
account with no shell, no home and no group shared with another service.
Never root, never `nobody`/`www-data`, never uid 1000 (a human account on the
hosts).

- **systemd units** (`deploy/systemd`): accounts from `deploy/sysusers.d/unheaded.conf`,
  writable state only in the unit's own `StateDirectory`, `UMask=0077`, no
  capabilities, `ProtectSystem=strict`, `ProtectProc=invisible`, `@system-service`
  minus `@privileged @resources`. Details: [[Systemd Units|Systemd-Units]].
- **Containers** (`docker-compose.yml`): one numeric uid each from `pkg/uids`
  (Unheaded services 16720-16759, telemetry 16700-16719, infrastructure 16780-16799,
  postgres the image's 70), `read_only`, `cap_drop: ALL`, `no-new-privileges`.
  No container holds the Docker socket (traefik reads its routes from a file).
- **Enforced**: `pkg/uids` tests hold compose, the Dockerfile and the k8s
  manifests to the registry; `scripts/check-compose-hardening.sh` and
  `scripts/check-systemd-units.sh` run in CI.
- **Not yet** (FND-010): the baseline agent, the Doom units and the Well's
  backup jobs still start as root; the undeployed alternate compose files
  (FND-009) are baselined, shrink-only.

### Secrets Management

- **Credentials are never stored in the repo**: not in code, manifests, scripts, docs or test fixtures.
- They come from environment variables with fail-loud guards (`${VAR:?...}` in compose, a startup error in Go), never a shipped default. On west they live in the repo-root `.env` (0600, gitignored) and per-service env files such as `/etc/huginn.env` (root 0600).
- Enforced by CI: `gitleaks` on every push, `scripts/check-secrets-baseline.sh` (`.gitleaksignore` may only shrink), `trivy --scanners secret`.
- The Well's passwords were all rotated on 2026-10-15; backup sets carry no password hashes.

---

## Lich Fuzzing Campaign (LICH-007)

Sprint S30 launched the LICH-007 fuzzing campaign against the MBC bytecode interpreter. Three fuzz targets ran for 72 hours on February 21, 2026:

1. **Decode fuzzer:** Random byte sequences fed to the MBC instruction decoder. Tests that all 256 opcode values are handled without panic.
2. **Execute fuzzer:** Random MBC instructions executed against a CPU state. Tests that no instruction combination crashes the interpreter or causes unbounded execution.
3. **Roundtrip fuzzer:** Encode-decode-execute pipeline. Tests that encoding an instruction and decoding it produces the same result, and that execution is deterministic.

Fuzz targets are located in `crates/monad-mbc/fuzz/`. Campaign PIDs and logs were written to `/tmp/lich007-*.log`.

---

## Lich Campaign D1-D6 (S33 plan, 2026-03)

Six targeted security campaigns are planned against the live Doom PoC to measure the attack surface and validate mitigations. These campaigns run as WS2 (Agent D, BlackMage role) during March 1-5, 2026.

### D1: ROM Injection via ROM_MAP

**Attack:** An adversary with BPF map write access modifies ROM_MAP (the bytecode) after the program is loaded but before execution. Malicious MBC instructions are injected at runtime.

**Severity:** HIGH. If ROM_MAP is writable after load, arbitrary code execution is possible inside the eBPF compute environment.

**Mitigation:** ROM_MAP should be set read-only after initial load using `BPF_F_RDONLY` flag. The loader must verify the flag is set before starting execution.

### D2: Framebuffer Exfiltration via RAM_MAP

**Attack:** An unprivileged process (running as `nobody`) attempts to read RAM_MAP via the `bpf()` syscall. If successful, the attacker can read the Doom framebuffer, CPU state, and all virtual memory contents.

**Severity:** MEDIUM. Framebuffer data is observability-only (no secrets), but unauthorized BPF map access indicates a privilege boundary failure.

**Mitigation:** BPF maps should require `CAP_SYS_ADMIN` or `CAP_BPF` for access. Process-level isolation (`BPF_F_RDONLY_PROG`) restricts reads to the owning program.

### D3: Keyboard Injection via SYSCALL Topic

**Attack:** An untrusted service publishes a fake keyboard event to the Wotan SYSCALL topic (`{syscall: 30, char: 'q'}`). If Doom processes the event, the attacker can control game input remotely.

**Severity:** MEDIUM. In the Doom context, this is a nuisance. In a production context, SYSCALL topic injection could manipulate control plane behavior.

**Mitigation:** Sign SYSCALL messages with service identity. The BPF program (or doom-bridge intermediary) verifies the publisher's identity before processing the message. Reject messages from unauthorized sources.

### D4: Flow Label Collision (Birthday Attack)

**Attack:** The IPv6 flow label is 20 bits, providing ~1 million unique values. With 500 simultaneous flows, the birthday paradox predicts a ~12% collision probability. At 1,000 flows, collision probability exceeds 50%.

**Severity:** HIGH. Flow label collisions cause trace ID ambiguity -- two different packet flows are attributed to the same trace, corrupting observability data.

**Mitigation:** Production must use 128-bit UUIDv7 for trace IDs (stored in Monad register file or Wotan metadata), not the 20-bit flow label. Flow labels remain useful for fast-path routing but must not be the sole trace identifier.

### D5: SYSCALL Fuzzing

**Attack:** Send 10,000+ random SYSCALL messages (syscall numbers 0-255) to the BPF program. Measure crash rate, halt rate, and any unexpected behavior.

**Severity:** HIGH. If invalid syscall numbers cause the BPF program to crash, hang, or behave unpredictably, an attacker can deny service to the entire compute ring.

**Mitigation:** Bounds-check the syscall number before dispatching. Unrecognized syscall numbers return an error code and increment a counter. The default case must be safe.

### D6: ROM TOCTOU (Time-Of-Check-Time-Of-Use)

**Attack:** Parallelize ROM loading and XDP program execution. While the CPU executes instructions from ROM_MAP, a concurrent thread modifies ROM_MAP entries. This creates a race condition where the CPU reads different bytecode than what was verified at load time.

**Severity:** HIGH. A successful TOCTOU attack allows arbitrary code injection after verification, bypassing any load-time integrity checks.

**Mitigation:** Snapshot ROM into an immutable memory segment after loading. Unmap the original writable map after the CPU starts. Alternatively, use BPF map freeze (`BPF_MAP_FREEZE`) to make the map immutable after population.

---

## Compliance

Evidence, not intent (ADR-097): common controls are mapped many-to-many onto each framework's full requirement list. Every control is backed by evidence that can fail: CI job results, gate scripts, signed commits, and live host posture on west and east.

| Framework | State |
|-----------|-------|
| NIST 800-53 Rev 5.2.0, FedRAMP Low/Moderate/High | Crosswalked, evidenced on the lab hosts |
| SOC 2, ISO 27001 Annex A, NIST CSF 2.0, 800-171 r3, SSDF | Crosswalked, evidenced on the lab hosts |
| CIS Ubuntu 24.04 L1 Server | Probed per host (`cmd/unheaded-baseline`, audit-only) |
| PCI-DSS, HIPAA, ITAR, GDPR | Not crosswalked |

- **Findings register** (ADR-098): `compliance/findings/register.yaml` triages; evidence decides what is open. It runs in audit mode only.
- **Remediation as code** (ADR-099): one fix model rendered as `remediate.sh`, `cmd/unheaded-remediate` (backup, undo, hash-chained journal) and Ansible. Profiles: `none` / `lab` / `cis-l1-server` / `all`.
- State lives in The Well (`unheaded_ops.compliance_*`, `unheaded_config.compliance_*`). The dashboard's compliance page (`:20000/compliance.html`) shows each framework against its full denominator.

West and east are a dev/lab environment: lab-only gaps (for example, no disk encryption) are recorded and accepted, not remediated.

-----------|--------|-------------|
| SOC 2 Type II | Planned | Access controls, audit logging, encryption |
| FedRAMP | Planned | FIPS 140-2 crypto, boundary protection |
| NIST 800-53 | Planned | Continuous monitoring, incident response |
| PCI-DSS | Planned | Network segmentation, key management |
| HIPAA | Planned | PHI isolation, audit trails |
| ITAR | Planned | Data residency, access controls |
| GDPR | Planned | Data minimization, right to erasure |

The NIST 800-207 Zero Trust Architecture principle provides a strong foundation: if the infrastructure cannot access user data, most data protection requirements are satisfied by architecture.

---

## Security Testing Infrastructure

| Tool | Purpose | Where |
|------|---------|-------|
| gosec | Go static analysis, ratcheted per rule | `scripts/check-gosec-ratchet.sh`, CI |
| govulncheck, cargo-audit, pip-audit, grype, trivy | Dependency and image vulnerabilities; pip-audit's accepted findings may only shrink | `.github/workflows/`, `scripts/check-python-audit.sh` |
| Dependency baseline | No third-party package (go.sum, Cargo.lock, requirements, package.json) enters unrecorded (ADR-096) | `scripts/check-dependency-baseline.sh` |
| Image tags | No new floating image tag; the existing ones may only shrink (ADR-102) | `scripts/check-image-tags.sh` |
| Unit binaries / accounts | Every systemd unit runs a binary the repo builds, as its own sysusers account | `scripts/check-unit-binaries.sh`, `scripts/check-systemd-units.sh` |
| gitleaks (pinned binary, sha256) + shrink-only baseline | Secrets in the tracked tree | `scripts/check-gitleaks.sh`, `scripts/check-secrets-baseline.sh` |
| ruff, shellcheck, clippy | Python, shell and Rust lint (gating) | CI |
| oxlint (pinned binary, sha256) | JavaScript lint (gating); replaced eslint, the only npm use (ADR-006) | `scripts/check-js-lint.sh` |
| release freshness | Pinned tools (oxlint, gitleaks) fail once a newer release is 7 days old | `scripts/check-tool-freshness.sh` |
| hadolint | Dockerfile lint: a Dockerfile it cannot parse fails; (rule, file) counts may only shrink | `scripts/check-hadolint.sh` |
| bandit | Python security lint (reporting only) | CI |
| syft | SBOM | CI |
| Meta-gate | Plants a violation in every gate and requires it to fail | `scripts/check-gates-can-fail.sh` |
| Go fuzz targets | Every package with Fuzz functions, 10 s each, on every security run | `.github/workflows/security.yml` (lich-fuzz) |
| LICH campaigns | Fuzz / red team (ADR-062); LICH-005 fuzzes `.bpf.o` through the real loader into `bpf(2)` | `tomb/lich/`, `crates/*/fuzz/` |
| Fuzz targets | MBC interpreter fuzzing | `crates/monad-mbc/fuzz/` |
| Seccomp profiles | Syscall filtering | `nix/modules/hardening.nix` |
| Network policies | Firewall rules | `nix/containers/*.nix` |

---

## Key Security Files

| File | Purpose |
|------|---------|
| `SECURITY.md` | Vulnerability reporting policy |
| `compliance/findings/register.yaml` | Open findings, triage (ADR-098) |
| `docs/security/SECURITY_AUDIT.md` | 2026-02 audit findings (historical) |
| `docs/security/application-threat-model.md` | Application threat model |
| `docs/security/LICH_FUZZING_SETUP.md` | Fuzz campaign setup guide |
| `nix/modules/` | NixOS hardening modules |

---

*See also: [[Architecture]] | [[Protocol Specifications|Protocol-Specs]] | [[Roadmap]] | [[Security TODOs|Security-TODOs]]*
