<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Docker host configuration

`daemon.json` is the canonical Docker daemon config for a Kingdom host.
`scripts/bootstrap-llm-lab.sh` installs it from here; it used to carry its own
copy as a heredoc, which is how the two drifted apart.

## `fixed-cidr-v6` — why it is not `fd00:dead:beef::/48`

The Kingdom's WireGuard overlay owns `fd00:dead:beef::/48` (WEST
`fd00:dead:beef::1`, EAST `::2` — see `runbooks/network/wireguard-overlay.yaml`).
Handing Docker that same `/48` makes the daemon allocate container addresses out
of the overlay's range, and the two then disagree about who owns a given
address.

Docker therefore gets a separate `/48`, `fd00:d0c0:e700::/48`. This matches
what the running hosts use and what the Phase 5 exit gate in
`references/archive/battle-plan-post-reboot-doom.md` checks for by name:

> Should show `fd00:d0c0:e700::/48` NOT `fd00:dead:beef::/48`

A narrowed carve-out (`fd00:dead:beef:0:d::/80`) was considered and is what an
older comment in `docker-compose.yml` still suggested. The separate `/48` won
because it keeps the two allocators in disjoint space rather than adjacent
inside one prefix.

## Log caps

`log-driver` / `log-opts` here are the backstop for anything started outside
compose. Compose declares its own tighter caps per service (ADR-092), enforced
by `scripts/check-compose-log-caps.sh`, and those take precedence for the stack.
