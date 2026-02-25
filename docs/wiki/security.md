# Security Overview

## Principles

### Zero User Data Access

The foundational security principle of the Unheaded Kingdom: **zero user data access** is enforced architecturally, not by policy. User applications ("the head") are isolated by design -- they communicate only through the gateway and never have access to infrastructure internals.

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

### Container Hardening

Every service container runs with NixOS-enforced hardening (see Architecture for full list):
- `NoNewPrivileges = true`
- Seccomp: `@system-service` allowlist, `~@privileged` denied
- Filesystem: `ProtectSystem = "strict"`, `PrivateTmp`, `ProtectHome`
- Capabilities: minimum required only (`CAP_NET_BIND_SERVICE`)
- Process isolation: `PrivateDevices`, `ProtectKernelTunables`, `ProtectControlGroups`

### Secrets Management

- Secrets are never hard-coded, logged, or stored in Git
- SOPS + age for encrypted secrets at rest
- Secrets mounted as files, not environment variables (not visible to `ps`)
- Regular rotation enforced by policy
- Access audited

---

## Lich Fuzzing Campaign (LICH-007)

Sprint S30 launched the LICH-007 fuzzing campaign against the MBC bytecode interpreter. Three fuzz targets ran for 72 hours on February 21, 2026:

1. **Decode fuzzer:** Random byte sequences fed to the MBC instruction decoder. Tests that all 256 opcode values are handled without panic.
2. **Execute fuzzer:** Random MBC instructions executed against a CPU state. Tests that no instruction combination crashes the interpreter or causes unbounded execution.
3. **Roundtrip fuzzer:** Encode-decode-execute pipeline. Tests that encoding an instruction and decoding it produces the same result, and that execution is deterministic.

Fuzz targets are located in `crates/monad-mbc/fuzz/`. Campaign PIDs and logs were written to `/tmp/lich007-*.log`.

---

## Lich Campaign D1-D6 (Planned, Sprint S33)

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

## Compliance Targets

The Unheaded architecture is designed to satisfy multiple compliance frameworks:

| Framework | Status | Key Controls |
|-----------|--------|-------------|
| SOC 2 Type II | Planned | Access controls, audit logging, encryption |
| FedRAMP | Planned | FIPS 140-2 crypto, boundary protection |
| NIST 800-53 | Planned | Continuous monitoring, incident response |
| PCI-DSS | Planned | Network segmentation, key management |
| HIPAA | Planned | PHI isolation, audit trails |
| ITAR | Planned | Data residency, access controls |
| GDPR | Planned | Data minimization, right to erasure |

The zero user data access principle provides a strong foundation: if the infrastructure cannot access user data, most data protection requirements are satisfied by architecture.

---

## Security Testing Infrastructure

| Tool | Purpose | Location |
|------|---------|----------|
| gosec | Go static security analysis | `Makefile` lint target |
| Fuzz targets | MBC interpreter fuzzing | `crates/monad-mbc/fuzz/` |
| Lich campaigns | Targeted attack testing | `tests/security/doom/` (planned) |
| Seccomp profiles | Syscall filtering | `nix/modules/hardening.nix` |
| Network policies | Firewall rules | `nix/containers/*.nix` |

---

## Key Security Files

| File | Purpose |
|------|---------|
| `SECURITY.md` | Vulnerability reporting policy |
| `docs/SECURITY.md` | Security architecture overview |
| `docs/SECURITY_AUDIT.md` | Audit findings and remediations |
| `docs/SECURITY_TODOs_2-9-26.md` | Outstanding security items |
| `LICH_FUZZING_SETUP.md` | Fuzz campaign setup guide |
| `nix/modules/` | NixOS hardening modules |

---

*See also: [Architecture](architecture.md) | [Protocol Specifications](protocol-specs.md) | [Roadmap](roadmap.md)*
