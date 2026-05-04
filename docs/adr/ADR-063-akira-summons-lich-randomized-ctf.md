# ADR-063 — Akira Randomly Summons the Lich for "CTF Mode"

**Status:** Pipe Dream (note-to-self; activates after the Lich framework of ADR-062 has at least 3 campaigns running cleanly)
**Date:** 2026-05-04
**Deciders:** Stevie Bellis + unheaded-blackmage (offensive owner) + unheaded-yaldabaoth (chaos owner) + unheaded-marshal (lane enforcement during runs)
**Triggered by:** Stevie's kanban entry `adr-akira-scheduler-should-randomly-summon-litch-t-mnlbyp3j`: *"ADR - akira scheduler should randomly summon litch to 'play CTF' and try to find exploits"*

---

## Context

Two capabilities already exist in the Kingdom:

1. **Akira** (ADR-029, `cmd/akira/`) — the Wotan consensus-health scheduler. Akira already runs heartbeat + aliveness checks on a tick; she is the natural cron of the Kingdom.
2. **The Lich** (ADR-062, `tomb/lich/`) — the offensive-adversary framework. Lich runs continuously against build artifacts in CI, but it does NOT run against the *live* deployed Kingdom at random.

The gap: the Lich today fuzzes parsers and replays seed corpora on a schedule, but a real attacker doesn't pick a parser — they pick a service, a moment, and a payload. Stevie's idea closes that gap: Akira occasionally "summons" the Lich to do an out-of-band, randomized adversarial probe against a randomly-chosen Kingdom service, as if the Lich were a chaos-engineering agent with offensive intent.

This is **chaos engineering with malicious flavour**. Yaldabaoth (the existing chaos service) does fault injection — this proposal adds *exploitation attempts* to that surface. Where Yaldabaoth crashes a service to test recovery, the Lich-CTF mode tries to *take over* a service to test that the gate, the audit log, and the crash-handler all behave when something genuinely tries.

The "CTF" framing matters: the Lich is *trying to win* (find an exploit), not just trying to break (cause a crash). Different mindset, different tooling.

---

## Decision

**Build `cmd/akira` to occasionally invoke the Lich against a randomly-chosen Kingdom service, with strict scoping to prevent the chaos from leaving the LAN posture.** Capture every CTF round in a journal under `tomb/lich/ctf-rounds/<date>-<service>/` so findings are auditable.

This is **NOT** active chaos engineering of arbitrary scope — it's bounded, time-budgeted, opt-in, and produces evidence. The Marshal protocol applies: HARD HALT on any finding that escapes the targeted service's blast radius.

### Round mechanics

A "CTF round" is:

```
1. Akira picks a random Kingdom service from a curated, opt-in list
   (services in `configs/lich-ctf-eligible.yaml`). Eligibility is
   per-service explicit consent — not all services are CTF-eligible.

2. Akira picks a random duration from {15min, 30min, 60min} —
   bounded so a misbehaving probe can't run all night.

3. Akira POSTs to the Lich runner with:
   - target service (HTTP/gRPC endpoint or BPF attachment)
   - duration
   - opt-out tripwire (a Wotan topic the service can publish to mid-run
     to abort the round; e.g., if it detects its OWN exploit, it can
     yell "stop testing, I'm broken")

4. The Lich loads the relevant LICH-NNN campaign(s) for that service
   (e.g., LICH-013 for /api/v1/tool/exec, LICH-001 for Monad parsers).
   The campaigns run their normal mutator/fuzzer harnesses, but
   pointed at the LIVE service rather than a unit-test build.

5. Findings — crashes, unexpected 500s, audit-log gaps, gate bypasses
   — get filed under tomb/lich/ctf-rounds/<date>-<service>/findings/
   and Wotan-published on topic `lich.ctf.findings`.

6. Round ends at duration timeout OR opt-out tripwire OR Marshal HARD
   HALT (see below). Akira logs round metadata (start, end, target,
   findings count) to her own state.
```

### Eligibility list (initial pass — locked before activation)

Each service must explicitly opt in via `configs/lich-ctf-eligible.yaml`. Lean: start small, expand as services prove robust.

| Service | Eligible? | Rationale |
|---|---|---|
| zhen-agentd `/api/v1/tool/exec` | YES | already gated by Champion; LICH-013 campaign exists |
| shield WAF (cmd/shield) | YES | the WAF *should* survive adversarial input — that's the point |
| wotan HTTP API | YES | the message bus is hardened by design |
| Sophia BPF maps | NO (initial) | kernel-level; a misbehaving Lich could panic the host |
| postgres | NO | data integrity; out of scope for randomized exploitation |
| llama-server | NO | inference path; unrelated to security testing |
| kanban-app | YES | hop-4 wired, audit trail in place |

The list lives in `configs/lich-ctf-eligible.yaml` (not in this ADR) so it can be updated without an ADR amendment as services mature.

### Frequency

Akira summons the Lich **at most once per 24h, with 50% probability per day** (so on average ~3.5 rounds per week). The randomness matters — making the schedule predictable would let a real attacker time their actions to avoid the CTF window.

```
Pseudocode (lives in cmd/akira/lich_summon.go when activated):

every day at 03:00 UTC:
  if rand_float() < 0.5:
    target := random_choice(eligible_services)
    duration := random_choice([15min, 30min, 60min])
    summon_lich(target, duration)
```

Frequency is tuneable via `configs/akira.yaml`'s `lich_summon_probability` and `lich_summon_hour` keys (defaults: 0.5 + 03:00 UTC).

### Marshal halt protocol (during a CTF round)

The Lich is, by design, doing destructive things to a live service. The Marshal lane-enforcement applies:

**HARD HALT** triggers (during any round):

- The targeted service's `/health` endpoint stops returning 200 for >60s. The Lich is supposed to *find* exploits, not *cause persistent outage*; if it does, that's an emergency.
- A finding implicates a service NOT in the eligible list (lateral movement detected). Halt + alert; the Lich found something we didn't expect, and we don't want it kept running while we're confused.
- Wotan publishes `lich.ctf.abort` from any source (operator-initiated kill switch).
- Akira's heartbeat to herself stops — if Akira is dead, the round can't be safely terminated by the scheduler.

On HARD HALT:
1. Akira sends SIGTERM to the Lich runner.
2. Lich runner has 10s to clean up; SIGKILL after.
3. Round is marked `ABORTED` in the journal with the trigger.
4. Wotan topic `lich.ctf.aborted` published.
5. Email/page alert (when alerting is wired up — currently not).

### Findings disposition

Every CTF round produces a findings report. Severity ladder:

- **CRITICAL** (gate bypass, RCE, audit-log evasion) → Stevie paged immediately; the affected service is taken out of CTF rotation until fixed.
- **HIGH** (DoS that exceeds the round's time budget, info disclosure) → kanban task auto-filed; remains in rotation but flagged.
- **MEDIUM/LOW** (slow path, log noise, edge-case behaviour) → logged in the round journal; no auto-action.

The "auto-file kanban task" path goes through Champion (so Akira can't write directly to the kanban without the gate). The justification chain is `lich-ctf-finding`, not `direct-user` — which means kanban_create here goes through the standard Rule-2 trust-escalation: untrusted-justification → pending-confirmation → Stevie reviews and approves before the task lands.

This is deliberate: even when our own automated adversary finds something, the operator stays in the loop for the artifact that survives the round.

---

## Why this is parked, not started

Three reasons hold this back:

1. **ADR-062 (Lich framework) is freshly minted.** Until the framework has at least 3 campaigns running cleanly with documented exit criteria, randomly invoking the Lich against live services is premature. Activation criterion: LICH-001, LICH-002, and at least one of LICH-012/013 have continuous CI runs producing zero net-new findings for 7 consecutive days. Then this ADR activates.

2. **Akira's existing scope is heartbeat + consensus health.** Adding a `summon_lich` codepath is a real capability extension (~200 LOC + tests + the eligibility config plumbing). Worth doing, but it's its own sprint.

3. **Wotan topic taxonomy needs a small update.** `lich.ctf.findings`, `lich.ctf.aborted`, `lich.ctf.summoned` — these are new topics that need ML-DSA-65 signing per ADR's wotan-topic-signing baseline. Trivial (4 entries in `configs/wotan.yaml`'s topic-signing list) but it's a deliberate change with audit-trail implications.

When Stevie wants to activate this, the trigger is one sentence in a future commit: *"ADR-063 activated; opening LICH-CTF-001 round-zero."*

---

## Consequences

### Positive

- The Kingdom develops *immune-system memory* of its own real-world failure modes. A finding from a CTF round is more valuable than a finding from CI fuzzing because it exercised the actual deployed service, not a unit-test build.
- Stevie's "is the gate actually doing its job?" question gets a continuous, evidence-backed answer rather than a "well, the unit tests pass" answer.
- The audit-log surface gets exercised under genuine attack pressure. Audit gaps that survive a CTF round are gaps that would survive a real attacker.

### Negative

- Risk of taking a service down. Mitigated by the eligibility list + duration cap + HARD HALT on `/health` regression, but not eliminated. Operator must accept that CTF rounds will occasionally cause real outages on services that opted in.
- Findings volume could swamp triage. Mitigated by the severity ladder + Champion-gated kanban-task autofiling — only Stevie can approve findings into the queue, so a noisy round produces noise but doesn't burn his attention budget.
- "Akira summons the Lich" sounds whimsical. It's not. The framing matters: Akira is the operational scheduler, Lich is the adversary, the language pulls its weight by reminding the operator what's actually happening when the round fires. Don't soften.

### Neutral

- Long-term, this proposal converges on a "blue team / red team continuous engagement" — which is where mature security shops end up. Recording that direction here so future-Stevie has the bridge already drawn.

---

## References

- ADR-029 — Wotan Consensus Health (Akira's existing scope)
- ADR-062 — Fuzz / Red-Team / Pentest Framework (the Lich's home; this ADR's prerequisite)
- ADR-019 — Zhen Champion Agent (the gate that auto-filed CTF findings flow through)
- BlackMage skill — Lich pattern + offensive mindset
- Yaldabaoth (gnostic chaos service) — adjacent: chaos engineering without exploitation intent. This ADR is the *malicious* sibling.
- Stevie's kanban directive (verbatim): *"ADR - akira scheduler should randomly summon litch to 'play CTF' and try to find exploits"*
