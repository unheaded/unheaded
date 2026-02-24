# ADR-004: No-External-Dependencies Policy (Self-Hosted Everything)

## Status: Accepted

## Date: 2026-01-26

## Context

Unheaded is an infrastructure platform that promises customers "production-ready infrastructure in hours, not months." The platform itself must embody the reliability and self-sufficiency it sells. A platform that depends on dozens of external libraries and services for its own operation cannot credibly claim to deliver independent, hardened infrastructure to customers.

During early development, the codebase accumulated several external Go dependencies:

- `prometheus/client_golang` for metrics exposition
- `rs/zerolog` for structured logging
- `vishvananda/netlink` for netlink socket management
- `cilium/ebpf` for eBPF program loading
- `grpc-go` for gRPC transport

Each dependency introduced:

1. **Supply chain risk**: A compromised upstream can inject malicious code (cf. event-stream, ua-parser-js, colors.js incidents).
2. **Version churn**: Upstream breaking changes force reactive maintenance unrelated to our product.
3. **License exposure**: Transitive dependencies may introduce incompatible licenses.
4. **Binary bloat**: Each dependency adds to the final binary size and attack surface.
5. **Philosophical contradiction**: Selling self-hosted infrastructure while depending on dozens of third-party packages undermines the brand.

The question was whether to continue with external dependencies for development speed or invest in internal replacements.

## Decision

We adopt a **no-external-dependencies policy for production code**. All shipping binaries must depend only on:

- The Go standard library (`pkg.go.dev/std`)
- `golang.org/x/sys` (syscall wrappers, quasi-standard)
- The Rust standard library (for eBPF/trace-collector)
- Internal Kingdom packages (`pkg/*`)

Specifically, we replace external dependencies with internal implementations:

| External Dependency | Internal Replacement | Package | LOC |
|--------------------|--------------------|---------|-----|
| `prometheus/client_golang` | Custom Prometheus-compatible metrics | `pkg/metrics/` | 1,168 |
| `rs/zerolog` | Custom structured logger (zero-alloc) | `pkg/logger/` | 1,533 |
| `vishvananda/netlink` | Custom RTNetlink + XDP attachment | `pkg/netlink/` | 2,136 |
| `cilium/ebpf` | Custom BPF syscalls + ELF parsing | `pkg/ebpf/` | 3,937 |
| `grpc-go` | Standard library HTTP/2 | (planned) | -- |
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

## References

- `docs/PROJECT_STRUCTURE.md` -- Dependency Philosophy section
- `pkg/metrics/metrics.go` -- Internal Prometheus-compatible metrics (1,168 LOC)
- `pkg/logger/logger.go` -- Internal structured logger (1,533 LOC)
- `pkg/ebpf/loader.go` -- Internal eBPF loader with BPF syscalls (3,937 LOC)
- `pkg/netlink/netlink.go` -- Internal RTNetlink implementation (2,136 LOC)
- `LICENSES/THIRD_PARTY.md` -- Development-only dependency attribution
