<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# CTF round journal (ADR-063) — design-only, not activated

When CTF mode is activated (and only then — see the activation rule below), each
summoned round writes a journal here. **Nothing writes here this sprint**: the
`cmd/akira` summon path is hard-gated on the activation rule and returns
`ErrCTFNotActivated`.

## Activation rule (unmet today)

Akira may summon the Lich against a live eligible service only when **LICH-001,
LICH-002, and one of LICH-012/013 have run clean with zero net-new findings for
7 consecutive days**. **LICH-012 is OPEN**, so the rule is NOT met and CTF stays
design-only. Activation is one sentence in a future commit that wires a real
clean-soak source into `CTFActivation` — the guard then opens on its own. It is
read from campaign clean-soak data, never from a flag.

## Journal layout (per round, once activated)

```
tomb/lich/ctf-rounds/<date>-<service>/
  round.json        # target, duration {15|30|60}min, started/ended, verdict,
                    #   hard-halt reason if aborted
  findings/         # staged findings from the round (never auto-published)
```

## Findings disposition

A CTF-round finding is autofiled toward the kanban/register through the
**Champion gate**: the `lich-ctf-finding` justification triggers ADR-104 Rule 2
(mutating, no retrieval justification) → **pending-confirmation** → Stevie
approves out of band. Never auto-published.

## Wotan topics (need ML-DSA-65 signing before activation)

- `lich.ctf.summoned` — a round opened (target, duration).
- `lich.ctf.findings` — a staged finding from a round.
- `lich.ctf.aborted` — a HARD-HALT fired (with reason).

Per the Wotan topic-signing discipline, these `config.*`/control topics must be
registered for ML-DSA-65 signing before activation (ADR-043 pattern).

## HARD-HALT (Marshal discipline, reuses the Phase 2 kill switch)

A round aborts immediately on any of: target `/health` non-200 past the budget
(60s), a non-eligible service implicated (lateral movement), `lich.ctf.abort`
published, or Akira heartbeat loss. See `RoundStatus.ShouldHardHalt` in
`cmd/akira/lich_summon.go` (unit-tested).
