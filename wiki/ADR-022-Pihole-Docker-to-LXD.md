# ADR-022: Migrate Pi-hole from Docker to LXD Container

**Status:** IMPLEMENTED — Pi-hole runs as an LXD instance in project `default` on west (S-REAL-STEEL: Cuirass's `unheaded` project cannot see it). Runbook: runbooks/network/dns-pihole-lxd.yaml. DNSSEC validation enabled 2026-10-29 (`dns.dnssec = true`). LAN devices and east have not used Pi-hole since 2026-08-22 (router-side change; `docs/security/ioc-sweep-2026-10-30.md`). Corrected 2026-10-30.  
**Date:** 2026-03-29

> Wiki stub generated 2026-05-06 by Marshal overnight sweep (NORTH-STAR Appendix A B5). See the canonical ADR for full text and rationale.

## Canonical

[docs/adr/ADR-022-pihole-docker-to-lxd.md](../docs/adr/ADR-022-pihole-docker-to-lxd.md)

## Cross-references

- [ADR Index](ADR-Index.md)
- [Architecture overview](Architecture.md)
