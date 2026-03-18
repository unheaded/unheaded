# Unheaded v0.1.0-alpha — The Alpha Ascension

## Release Date
February 23, 2026

## Summary
First alpha release of the Unheaded infrastructure automation platform.
Production-ready infrastructure in hours, not months.

## Highlights
- **eBPF packet tracing**: XDP packet_marker, TC flow_tracker, tracepoint latency_probe, syscall_tracer, plus Doom compute engine (monad-cpu-ebpf, hop-ebpf, shield-ebpf, yaldabaoth-ebpf)
- **Real-time trace correlation** and dashboard visualization with packet-flow UI
- **23 microservices** with health/ready/metrics endpoints (timeguru, captain, architect, micromanager, monad, sophia, wotan, gateway, anamnesis, kenoma, pleroma, yaldabaoth, shield, sword, cape, cloak, cuirass, gauntlets, hauberk, pauldrons, sabatons, tassets, vambraces)
- **JWT + API key + service token authentication** on all endpoints
- **mTLS service mesh** (TLS 1.3, per-service certificates, circuit breaking, load balancing, resilience policies)
- **Wotan message bus** with reliability guarantees (ring buffer, event bus, protocol RAM)
- **Wotan-based service discovery** with health-check integration
- **Cross-platform container support** (LXD, containerd, NixOS, Docker)
- **Interchangeable IaC backends** (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt)
- **Web Application Firewall** with detection, inspection, IP filtering, rate limiting, response filtering
- **Audit logging** with export, query, and storage subsystems
- **Compliance framework** with controls and standards modules
- **Deployment pipelines** with rollback and strategy support (blue-green, canary)
- **Doom-over-IPv6 computational completeness proof** — full Doom running on BPF packet-circulation ring
- **6 Lich offensive security campaigns** (D1-D6) with 41 findings and full remediation

## Architecture
- **6 layers**: Infrastructure (L0) through User Interface (L5)
- **Wotan message bus** as the central nervous system (gRPC + ring buffer + event bus)
- **Bridge network**: lxdbr0 (10.10.10.0/24) with explicit firewall rules
- **Triple format strategy**: MD (source of truth) + JSON (API) + YAML (IaC)
- **Default-deny networking** with explicit allow policies

## Known Limitations
- eBPF programs require Linux 5.15+ with BPF/XDP enabled
- Production deployment not yet tested at scale
- SOPS secrets management requires manual setup
- Container image scanning not yet automated
- E2E smoke test with all 23 services running simultaneously not yet validated
- Doom-over-IPv6 requires dedicated Linux host with CAP_BPF/CAP_SYS_ADMIN

## Security Posture
- All endpoints require authentication (JWT, API key, or service token)
- mTLS between all services with per-service certificate issuance
- 128-bit UUIDv7 trace IDs for eBPF correlation
- BPF map freeze for ROM protection (TOCTOU mitigation)
- SYSCALL validation with default-deny policy
- Rate limiting on all endpoints (WAF + per-service)
- Web Application Firewall with SQL injection, XSS, and command injection detection
- Audit logging on all state-changing operations
- 6 Lich campaigns executed: ROM injection, framebuffer exfiltration, flow label collision, input injection, protocol amplification, race conditions
- All 2 CRITICAL and 6 HIGH findings remediated or design-verified

## Statistics
- **740K+ lines** of code (Go, Rust, JavaScript, Nix, YAML, Shell, C)
- **151 Go test packages** passing (0 failures)
- **293 Rust tests** passing (unit + integration + demo + doc tests)
- **23 microservices** operational
- **91 Go module dependencies** (see sbom-go-modules.txt)
- **155 commits** over 28 days (January 26 - February 23, 2026)

## Next Steps (Age 2: Beta)
- Production deployment testing at scale
- Container image scanning automation
- Automated certificate rotation
- Service mesh observability dashboard
- Performance tuning for 1000+ req/s sustained
- Post-alpha service breakout (monorepo to individual repos)
- Public accessibility with optional authentication
- Sub-50ms latency target (packet to browser)
