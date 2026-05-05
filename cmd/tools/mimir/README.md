# Mímir — Config Drift Sentry

**License:** GPL-3.0 (component-uniform)
**Status:** REAL-METAL validated zero false positive on EAST host (2026-04-08, ADR-043)

## What it is

Mímir watches a host's filesystem for drift from a known-good baseline. When
drift is detected, it publishes an alert to Wotan (or stdout, if you don't run
Wotan) — and **does nothing else**. Alerts-only is the design principle, not
a missing feature: operators stay in control of remediation. Auto-restore is
explicitly out of scope.

In Norse mythology, Mímir is the keeper of wisdom — the well-bound head that
answers Odin's questions about what was. The drift sentry is the keeper of
"what was supposed to be," and answers questions about "what changed."

## Who it's for

- SOC2 / HIPAA / PCI shops that need continuous-monitoring evidence for
  CC7.1 (system-monitoring), §164.312(b) (audit controls), or PCI 11.5
  (file-integrity-monitoring) without taking on a commercial FIM tool.
- Regulated infrastructure where auto-remediation is a compliance risk
  (alerts-only is the *correct* posture, not a degraded one).
- Anyone running infrastructure who wants to know when something quietly
  changed underneath them.

Compliance evidence that ships alongside the tool (per
`COMPLIANCE-EVIDENCE.md` when filed): SOC2 CC7.1, HIPAA §164.312, PCI 11.5,
NIST SP 800-53 SI-7, CIS 1.1.

## What's in the box

Three Go binaries + an Ansible role:

| Binary | What it does |
|---|---|
| `heimdall-daemon` | the watching daemon — Mjölnir-driven scan + drift detection, publishes to Wotan |
| `gjallarhorn-sender` | UPC trigger CLI — for manually firing a 20-byte Monad packet that requests an immediate scan |
| `gjallarhorn-listener` | (for non-UPC environments) listens on UDP for `gjallarhorn` triggers |

Plus the Mjölnir scan policy lives in `tomb/ansible/roles/heimdall/`.

## Differentiator vs commercial FIM

| | Mímir | Tripwire / OSSEC / Wazuh |
|---|---|---|
| License | GPL-3.0, free, redistributable | proprietary or AGPL-with-cloud-paywall |
| Auto-remediate? | NO (deliberately) | YES (which can be a CC7.1 anti-pattern) |
| Wire format for triggers | Monad / UPC (20-byte packet) — eBPF-native | proprietary or HTTP/JSON |
| Compliance evidence | source code + runbook + REAL-METAL validation log | vendor-claimed |
| Alert delivery | Wotan topic OR stdout/file | proprietary agent OR SIEM integration |

## Build + adopter quickstart

See `BUILD.md` for explicit invocations. The short version:

```bash
go build -o bin/heimdall-daemon       ./cmd/heimdall-daemon/
go build -o bin/gjallarhorn-sender    ./cmd/gjallarhorn-sender/
go build -o bin/gjallarhorn-listener  ./cmd/gjallarhorn-listener/
```

## See also

- `cmd/tools/README.md` — curation pattern these tools share
- `docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md` — the
  PoC ADR that proved this works on real metal
- `runbooks/security/` — drift response runbooks
- `tomb/lich/LICH-012-config-convergence/` — adversarial campaign against
  this exact tool's contract
