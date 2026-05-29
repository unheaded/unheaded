# Unheaded Documentation

**Configuration management automation platform.**

Unheaded is a configuration management automation platform built on eBPF-based observability, declarative infrastructure, and a novel IPv6 protocol extension for per-packet metadata. User brings their app ("the head"), we provide everything else.

## Directory Guide

| Directory | Contents |
|-----------|----------|
| `protocol/` | Unheaded protocol specifications (Monad, Sophia, Wotan), IANA registry drafts, and alignment notes |
| `architecture/` | System architecture diagrams and component breakdowns |
| `adr/` | Architecture Decision Records |
| `research/` | Technical research (IPv6 metrics, eBPF performance, protocol design space) |
| `security/` | Security audit reports, fuzzing campaign setup, threat models |
| `compliance/` | Regulatory and license compliance documentation |
| `legal/` | IP inventory, licensing, contributor agreements |
| `runbooks/` | Operational runbooks for deployment and incident response |
| `bare-metal/` | Bare metal host provisioning and configuration |
| `infrastructure/` | Network design, service topology, monitoring setup |
| `archive/` | Historical documents preserved for reference |
| `internal/` | Internal development docs (session handoffs, sprint plans). See `INTERNAL.md` |

## Key Documents

- [ARCHITECTURE.md](ARCHITECTURE.md) -- Full system architecture
- [VISION.md](VISION.md) -- Project vision and roadmap
- [protocol/](protocol/) -- Protocol specifications and drafts
- [EXTERNAL_DEPENDENCIES.md](EXTERNAL_DEPENDENCIES.md) -- Third-party dependency inventory
