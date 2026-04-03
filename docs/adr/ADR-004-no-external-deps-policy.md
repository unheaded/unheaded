# ADR-004: No-External-Dependencies Policy (Self-Hosted Everything)

## Status: Accepted

## Date: 2026-01-26

## Context

Unheaded is an infrastructure platform that promises users "production-ready infrastructure in hours, not months." The platform itself must embody the reliability and self-sufficiency it sells. A platform that depends on dozens of external libraries and services for its own operation cannot credibly claim to deliver independent, hardened infrastructure to users.

During early development, the codebase accumulated several external Go dependencies:

- `prometheus/client_golang` for metrics exposition
- `rs/zerolog` for structured logging
- `vishvananda/netlink` for netlink socket management
- `cilium/ebpf` for eBPF program loading
- `grpc-go` for gRPC transport

Each dependency introduced:

1. **Supply chain risk**: A compromised upstream can inject malicious code (cf. event-stream, ua-parser-js, colors.js incidents).
2. **Version churn**: Upstream breaking changes force reactive maintenance unrelated to our application.
3. **License exposure**: Transitive dependencies may introduce incompatible licenses.
4. **Binary bloat**: Each dependency adds to the final binary size and attack surface.
5. **Philosophical contradiction**: Selling self-hosted infrastructure while depending on dozens of third-party packages undermines the brand.

The question was whether to continue with external dependencies for development speed or invest in internal replacements.

## Decision

We adopt a **no-unknown-author-dependencies policy for production code**. All shipping binaries must depend only on:

- The Go standard library (`pkg.go.dev/std`)
- `golang.org/x/sys` (syscall wrappers, quasi-standard)
- The Rust standard library (for eBPF/trace-collector)
- Internal Kingdom packages (`pkg/*`)
- **Approved exceptions**: Libraries from established major organizations (Google, Cloudflare, HashiCorp, etc.) or official language ecosystem packages, subject to case-by-case owner approval

The policy targets **supply chain risk from unknown code authors**, not ideological purity. Code from organizations with professional security teams, CI/CD pipelines, and public audit trails carries fundamentally different risk than a single-author npm package with 12 stars.

### Dependency Age Requirement

**All dependencies must have a GitHub repo created BEFORE July 2019.** No exceptions without owner approval.

Mature software has survived multiple release cycles, community scrutiny, and real-world battle testing. A package created in 2023 hasn't proven itself. If a package is too new — we build our own.

**Verification:**
```bash
# Check repo creation date via GitHub API
curl -s https://api.github.com/repos/OWNER/REPO | jq '.created_at'
# Must be before 2019-07-01
```

**Combined rule:** A dependency must pass BOTH checks:
1. From an established organization (Google, Cloudflare, etc.)
2. Repository created before July 2019

If either check fails → build our own replacement (in Rust if we can outperform, Go if equivalent).

Specifically, we replace external dependencies with internal implementations:

| External Dependency | Internal Replacement | Package | LOC |
|--------------------|--------------------|---------|-----|
| `prometheus/client_golang` | Custom Prometheus-compatible metrics | `pkg/metrics/` | 1,168 |
| `rs/zerolog` | Custom structured logger (zero-alloc) | `pkg/logger/` | 1,533 |
| `vishvananda/netlink` | Custom RTNetlink + XDP attachment | `pkg/netlink/` | 2,136 |
| `cilium/ebpf` | Custom BPF syscalls + ELF parsing | `pkg/ebpf/` | 3,937 |
| `grpc-go` | **APPROVED EXCEPTION** — Google-maintained, battle-tested | `google.golang.org/grpc` | N/A |
| `gorilla/websocket` | Pure Go WebSocket | `cmd/dashboard-backend/internal/websocket/` | -- |

**Development placeholders**: Files that still use external deps are marked with a `.dev` extension and excluded from production builds. They exist only for rapid prototyping and must be replaced before shipping.

The policy extends to the frontend: no npm, no node_modules, no webpack, no React/Vue/Angular. All UI is vanilla HTML + CSS + JavaScript served via Go's `embed` directive (see ADR-006).

## Consequences

### Positive

- **Zero supply chain risk**: No upstream compromise can affect the production build. The attack surface is limited to the Go/Rust standard libraries and the kernel.
- **Full understanding**: Every line of code in production is written and maintained by the team. There are no "black box" dependencies where behavior is understood only through documentation.
- **Minimal binary size**: Production binaries contain only the code they need. No unused transitive dependencies inflate the attack surface.
- **License clarity**: The only licenses in production are Go's BSD license, Rust's MIT/Apache-2.0, and Unheaded's own license. The `LICENSES/THIRD_PARTY.md` file is for development tooling only.
- **Brand alignment**: Self-hosting validation extends to dependencies. A platform that builds its own metrics, logging, netlink, and eBPF libraries demonstrates deep systems expertise.
- **Operational independence**: The platform can be built, deployed, and operated on an air-gapped network with no internet access after the initial Go/Rust toolchain install.

### Negative

- **Higher upfront cost**: Writing `pkg/ebpf/` (3,937 LOC), `pkg/netlink/` (2,136 LOC), `pkg/logger/` (1,533 LOC), and `pkg/metrics/` (1,168 LOC) represents weeks of engineering that could have been spent on product features.
- **Maintenance burden**: Bug fixes and performance improvements in upstream libraries (e.g., prometheus/client_golang) must be independently discovered and implemented.
- **Potential for bugs**: Battle-tested libraries like zerolog and cilium/ebpf have thousands of users finding edge cases. Our internal replacements have a user base of one.
- **Hiring friction**: Developers accustomed to using established libraries may find the policy unusual or unnecessary. The rationale must be clearly communicated.
- **Development speed**: The `.dev` placeholder pattern means some development workflows temporarily use external deps, creating a gap between dev and prod environments.

## Approved Exceptions Register

Dependencies from established organizations, approved by owner on a case-by-case basis:

| Dependency | Organization | Justification | Approved |
|-----------|-------------|---------------|----------|
| `google.golang.org/grpc` | Google | Industry-standard gRPC, professionally maintained | 2026-04-03 |
| `google.golang.org/protobuf` | Google | Protobuf codec, required by gRPC | 2026-04-03 |
| `cloudflare/circl` | Cloudflare | PQ cryptography (SLH-DSA, ML-KEM), FIPS 205 | 2026-03-15 |
| `lib/pq` | Go community (widely used) | Pure Go PostgreSQL driver | 2026-03-15 |

**Approval process:** New exceptions require owner sign-off. Target: Kanban board approval workflow (see ADR-025).

## Long-Term Replacement Strategy

Approved exceptions are not permanent concessions — they are pragmatic acknowledgments that reimplementing battle-tested code in the same language rarely yields better results. However, **reimplementation in a lower-level language (Rust, assembly) that can outperform the original IS a valid long-term goal**.

The bar for replacement: the Kingdom version must be **as fast or faster** than the dependency it replaces. We will not ship slower code for ideological purity.

**Realistic candidates for replacement:**
| Dependency | Language | Why replacement could win |
|-----------|---------|--------------------------|
| `pkg/metrics/` (already done) | Go | Prometheus-compatible, zero-alloc, tailored to our use case |
| `pkg/logger/` (already done) | Go | Zero-alloc structured logger, no interface overhead |
| `pkg/ebpf/` (already done) | Go | Direct BPF syscalls, no abstraction layers |
| `pkg/netlink/` (already done) | Go | Minimal RTNetlink, no unused features |

**Unlikely to outperform (keep as exceptions):**
| Dependency | Why |
|-----------|-----|
| `google.golang.org/grpc` | Massive engineering team, HTTP/2 optimized at protocol level, Go-native. Writing faster gRPC in Go is not realistic. |
| `google.golang.org/protobuf` | Code-generated, heavily optimized. Protobuf IS the spec. |
| `cloudflare/circl` | PQ cryptography (ML-KEM, SLH-DSA) requires deep crypto expertise and FIPS validation. |

**Potential future wins via Rust/assembly:**
- Custom TLS 1.3 implementation (Rust) — could outperform Go's crypto/tls for our specific cipher suites
- Custom HTTP/3 + QUIC (Rust) — if we need kernel-bypass networking, Go's net package is the bottleneck
- eBPF-native metrics aggregation — bypass userspace entirely for hot-path metrics

The Computermancer, BlackMage, and Developer skills have the assembly/Rust expertise to pursue these when the time is right. The principle: **replace with something better, or don't replace at all.**

### GPL-3.0 License Compatibility

All approved exceptions use permissive licenses that are one-way compatible with GPL-3.0:
- Apache-2.0 (grpc-go, protobuf) — GPL-compatible per FSF
- BSD-3-Clause (circl) — GPL-compatible
- MIT (lib/pq) — GPL-compatible

The dependency policy is legally sound. No GPL conflicts exist with current exceptions. The only blocked license categories would be: proprietary, CDDL, EPL-1.0, or any copyleft that conflicts with GPL-3.0.

## References

- `docs/PROJECT_STRUCTURE.md` -- Dependency Philosophy section
- `pkg/metrics/metrics.go` -- Internal Prometheus-compatible metrics (1,168 LOC)
- `pkg/logger/logger.go` -- Internal structured logger (1,533 LOC)
- `pkg/ebpf/loader.go` -- Internal eBPF loader with BPF syscalls (3,937 LOC)
- `pkg/netlink/netlink.go` -- Internal RTNetlink implementation (2,136 LOC)
- `LICENSES/THIRD_PARTY.md` -- Development-only dependency attribution
