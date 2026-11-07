<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-137: The Doom Range port check and the dashboard's links move to uhl

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-06
**Related:** ADR-081 (Unheaded Linux, `uhl`), ADR-097, S-MOVE-OUT.

## The pitch (Stevie, 2026-11-06)

> leave the check, reassign to uhl, which should replace doom links on dash once it boots and we get TTY from websocket view like with doom

## What is there

- **The check.** `.github/workflows/security.yml`, job `doom-security` (`Doom Range Port Security`), step `Verify Doom Range port compliance`. It greps Go source for a short list of ports outside 16666-26666 and prints a warning. It cannot fail, so `compliance/catalog/controls.yaml` does not count it as evidence. It stays exactly as it is until this ADR is taken up.
- **The range.** The Doom Range is the port range in `pkg/ports/ports.go`, not the game. It stays when the game has left.
- **The game.** S-MOVE-OUT moved Doom to `unheaded-doom`, and no link to it is left under `cmd/dashboard-backend/static/`.
- **The TTY.** `pkg/ports` keeps `UPCTtyBridge` (26100), the bridge for a browser terminal.

## Decision direction

1. **Owner.** The check belongs to `uhl`: what it guards is the port plan of the system that boots, and that is `uhl`'s to keep.
2. **Dashboard.** Once `uhl` boots and gives a TTY over a websocket, the way Doom did, the dashboard links to that view, in the place the Doom links held.
3. **Order.** `uhl` boots first, then the TTY view, then the dashboard link, then the check.

## Open questions

1. When the check moves, does it become able to fail? That needs its false positives listed first, since it matches any of its ports in any Go line.
2. Does the TTY view use `UPCTtyBridge`, and through which login (ADR-114)?

## What this does not do

It does not change the check, the workflow or the dashboard today, and it sets no date.
