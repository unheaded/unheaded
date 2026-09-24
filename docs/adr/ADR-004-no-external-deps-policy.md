# ADR-004: No-External-Dependencies Policy (Self-Hosted Everything)

## Status: Accepted — admission gate amended by [ADR-096](ADR-096-dependency-gate-cost-to-fake.md) (proposed 2026-09-24)

## Date: 2026-01-26

## Context

Unheaded is an OSS infrastructure platform that promises adopters "configuration management automation platform." The platform itself must embody the reliability and self-sufficiency it ships. A platform that depends on dozens of external libraries and services for its own operation cannot credibly claim to deliver independent, hardened infrastructure to its users.

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
| `google.golang.org/protobuf` | Google | Protobuf codec, required by gRPC. BSD-3-Clause | 2026-04-03 |
| `cloudflare/circl` | Cloudflare | PQ cryptography: ML-KEM (FIPS 203), ML-DSA (FIPS 204), SLH-DSA (FIPS 205). Scope widened to record actual use, audit 2026-09-24 | 2026-03-15 |
| `lib/pq` | Go community (widely used) | Pure Go PostgreSQL driver | 2026-03-15 |

**Approval process:** New exceptions require owner sign-off. Target: Kanban board approval workflow (see ADR-025).

### Register audit — 2026-09-24

Each exception was checked against this ADR's own rules, then against how it
is actually used and whether it has known vulnerabilities. Method:
GitHub API for owner and creation date, the `LICENSE` in the module cache,
`git grep` of production imports, `go mod graph` / `go mod why` for
transitive modules, and `govulncheck ./...`, which reports only
vulnerabilities on code paths we call.

| exception | org / created | licence | reachable vulns | verdict |
|---|---|---|---|---|
| `google.golang.org/grpc` v1.82.1 | grpc org / 2014-12 | Apache-2.0 | **2** (see finding 1) | passes the rules; **upgraded to v1.83.2** |
| `google.golang.org/protobuf` v1.36.11 | protocolbuffers org / 2019-03-26 | **BSD-3-Clause** (register said Apache-2.0) | 0 | passes; register corrected below |
| `cloudflare/circl` v1.6.3 | cloudflare org / 2018-09 | BSD-3-Clause (two notices: Cloudflare, Go Authors) | 0 | passes; **scope wider than approved** |
| `lib/pq` v1.10.9 | `lib` community org / 2012-03 | MIT | 0 | **fails "established organisation"; upstream in maintenance mode** |
| `golang.org/x/sys` v0.46.0 | golang org / 2014-12 | BSD-3-Clause | 0 | passes |

**Findings, most severe first:**

1. **gRPC v1.82.1 had two vulnerabilities that govulncheck reports as
   reachable. Upgraded to v1.83.2 the same day.**
   - GO-2026-6443: a request with neither `:authority` nor `Host`. The panic
     it describes needs **xDS routing**: it is in
     `internal/xds/server.RouteAndProcess`, which indexes the empty
     authority. Wotan does not use xDS. govulncheck flags it because the
     transport functions it names (`http2Server.operateHeaders`) are on
     Wotan's call path, but the crashing code is not. Measured against
     Wotan's own binaries: on 1.82.1 the malformed request got **no
     response**, and the stream hung until the client timed out, with the
     process alive and `/health` 200. On 1.83.2 it gets `:status 400`,
     `grpc-status 13`. Fixed in **1.82.2 and 1.83.2 — not 1.83.1**, which is
     still inside the affected range; 1.84.0 is affected too.
   - GO-2026-6348: OOM through HTTP/2 DATA-frame fragmentation, in the plain
     transport Wotan does use (server, replication, `pkg/wotan-client`,
     dashboard-backend). Fixed in v1.83.1.

   **Correction (2026-09-24):** the first version of this finding said any
   client could crash the message bus before authentication ran, and that
   ≥ v1.83.1 closed both. Both were wrong. The crash claim came from
   reading govulncheck's symbol-level "reachable" as "exploitable here"
   without reading the advisory; a probe against the real binary disproved
   it. Lesson, in the ADR-093 spirit: **read the advisory's trigger
   conditions before rating severity, and test the live binary before
   stating impact.** Approval was still a one-time event that nothing
   re-checked; see finding 5.
2. **`lib/pq` does not meet the rule it was approved under.** `lib` is a
   volunteer GitHub organisation, not an organisation with a security team.
   The register already hedged ("Go community (widely used)"). Its own README
   now says it is in maintenance mode and recommends pgx "for reliable
   resolution of reported bugs". pgx (`jackc/pgx`) is a single-maintainer
   repository and fails the organisation rule too. So the options are keep
   and record the risk, or own a minimal PostgreSQL wire client. That is a
   decision for the owner; it is not made here.
3. **`circl` is used beyond its approval.** Approved for "SLH-DSA, ML-KEM,
   FIPS 205". Production also imports ML-DSA 44/65/87 (FIPS 204), in
   `pkg/crypto/pqc` and `pkg/gungnir`, and ML-KEM (FIPS 203). The use is
   legitimate; the register now records it. The SIDH/SIKE code in the module,
   which is broken cryptography, is imported nowhere.
4. **Transitive modules are trusted but not recorded.** gRPC pulls in
   `google.golang.org/genproto/googleapis/rpc` (via `grpc/status`) and
   `golang.org/x/net` (via `x/net/trace`). Both are Google or Go-team code,
   which is low risk, but they should be recorded, not implied.
5. **Nothing was watching.** The CI job that would have caught finding 1
   does run and does fail: `Security Scan (Daily + PR)` → "Go Vulnerability
   Check". But that workflow has **not passed once in its last 100 runs**, on
   `main` included, so a new failure is indistinguishable from the old ones.
   The main `CI` workflow (`ci.yml`), whose `govulncheck` job
   `.github/BRANCH_PROTECTION.md` lists as required, has been
   **`disabled_manually` since 2026-02-20**. `CI (Protocol Foundation)`,
   `Docker`, `eBPF & Rust` and `Release` are disabled as well. A gate that is
   always red is not a gate (ADR-093 rule 1).

**Outside the register, same scan:** the unregistered `cilium/ebpf` v0.20.0
has a reachable BTF integer overflow (GO-2026-6238, fixed in v0.22.0; reached
from `cmd/trace-collector-go`, a TOOL). Toolchain `go1.25.12` has six
reachable standard-library vulnerabilities fixed in go1.25.13 (net/http ×2,
net/url, html/template, crypto/tls, encoding/asn1). The `Rust Cargo Audit` and
gitleaks jobs are also failing; not investigated here. ADR-095's
supply-chain track covers the unregistered modules.

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
- Apache-2.0 (grpc-go) — GPL-3.0-compatible per FSF (not GPL-2.0-only)
- BSD-3-Clause (protobuf-go) — GPL-compatible
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
