---
date: 2026-01-30
day: Thursday
created: 2026-01-29T23:45:00Z
updated: 2026-01-29T23:45:00Z
---

# 2026-01-30 (Thursday)

## Plans
<!-- What needs to happen today -->

- [ ] **Real LXD Integration** - The Cuirass Awakens
  ```go
  // Package lxd provides LXD orchestration for the Unheaded control plane
  // THE CITADEL FORGES - Where NixOS containers are born and managed
  ```
  - Replace mock `LXDClient` with real implementation
  - Use `github.com/lxc/lxd/client`
  - Container lifecycle: create/start/stop/delete
  - Snapshot management for rollbacks
  - Integration tests with real containers

- [ ] **NixOS Citadels Foundation**
  - Initialize `flake.nix` structure
  - Create `nix/containers/` directory
  - Define base container module with hardening defaults

## Notes
<!-- Quick captures, thoughts, context -->

**Context from Jan 29 session:**
- Fae Chamber Protocols complete (`pkg/events/`)
- All services already wired to Busboy
- Kingdom at ~30% progress
- Alpha target: Feb 8, 2026

**Start here:**
```bash
cd cmd/unheaded-daemon/internal/lxd/
cat client.go  # Current mock implementation
# Implement real client
```

## Blockers
<!-- Anything preventing progress -->

- LXD must be installed on dev environment
- Need kernel >= 5.15 for eBPF (parallel track)

## Wins
<!-- Celebrate what got done -->

(Capture tomorrow's victories here)

---
_Last updated by: unheaded-calendar_
