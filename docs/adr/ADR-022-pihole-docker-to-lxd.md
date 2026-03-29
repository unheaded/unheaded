# ADR-022: Migrate Pi-hole from Docker to LXD Container

**Status:** PROPOSED
**Date:** 2026-03-29
**Author:** Stevie
**Deciders:** Captain, Architect

---

## Context

Pi-hole runs as a Docker container on WEST (`/home/govan/tmp/pihole/docker-compose.yml`)
in `network_mode: host`. This causes port 53 conflicts with LXD's dnsmasq instances
whenever LXD is running. The current workaround is to disable LXD at boot and kill
all dnsmasq processes before Pi-hole can bind port 53.

### Current State (WEST)

- Pi-hole: Docker container, `network_mode: host`, image `pihole/pihole:latest`
- Web admin: `https://192.168.69.184:8053/admin` (password: unheaded)
- Upstream DNS: 8.8.8.8, 1.1.1.3, 8.8.4.4
- Volumes: `./etc-pihole`, `./etc-dnsmasq.d`, `./custom.list`
- Conflict: LXD dnsmasq on lxcbr0 (10.0.3.1:53) and lxdbr0 (10.10.10.1:53)

### Problems with Docker Approach

1. `network_mode: host` competes with system dnsmasq for port 53
2. LXD must be disabled at boot to avoid conflicts
3. Docker's networking stack adds unnecessary overhead for a DNS server
4. No integration with LXD's managed networking (custom.list, DHCP)
5. Health checks fail when port 53 can't bind

## Decision

Migrate Pi-hole from Docker to an LXD container with a dedicated network profile.

### Benefits of LXD

- LXD manages network bridges natively — Pi-hole gets its own IP, no host port conflicts
- LXD containers are lighter than Docker for long-running services
- LXD profiles can assign static IPs on the host bridge
- LXD and Pi-hole can coexist — Pi-hole listens on its container IP, LXD dnsmasq on bridge IPs
- `lxc exec pihole -- pihole` for management (no `docker exec`)

### Migration Plan

1. Create LXD container from Ubuntu 24.04 image
2. Install Pi-hole via `curl -sSL https://install.pi-hole.net | bash`
3. Copy config from Docker volumes (`etc-pihole/`, `etc-dnsmasq.d/`, `custom.list`)
4. Assign static IP on lxdbr0 (e.g., 10.10.10.53)
5. Configure host DNS to point to 10.10.10.53
6. Verify resolution, ad blocking, custom domains
7. Remove Docker container and compose file
8. Re-enable LXD at boot

### Rollback

Keep Docker compose file until LXD migration is verified (72 hours).
If LXD Pi-hole fails, `docker compose up -d` restores service immediately.

## Services Disabled on WEST (2026-03-29)

To unblock Pi-hole in Docker, the following were disabled at boot:

| Service | Action | Reason | Re-enable When |
|---------|--------|--------|----------------|
| `lxc-net` | masked | LXC dnsmasq on 10.0.3.1:53 conflicts | After Pi-hole migrates to LXD |
| LXD snap | disabled | LXD dnsmasq on 10.10.10.1:53 conflicts | After Pi-hole migrates to LXD |
| `systemd-resolved` DNSStubListener | set to `no` | Stub on 127.0.0.53:53 could conflict | Keep disabled (Pi-hole replaces it) |

## Compliance Notes

- Pi-hole is GPL-2.0+ — compatible with Kingdom licensing
- No PII stored (DNS logs are local, not forwarded)
- DNSSEC: currently disabled, should be enabled post-migration
