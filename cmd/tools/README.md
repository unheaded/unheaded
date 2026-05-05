# `cmd/tools/` — Curated standalone build targets

**Purpose:** organise existing in-tree source into named, individually-buildable
"tools" that the community can adopt without taking the whole monorepo. Each
subdirectory is a **curation pointer** — it does NOT duplicate code; it
explains what existing packages compose into a tool and how to build it
standalone. License is GPL-3.0 (per the repo `LICENSE`); the LICENSE file
is the source of truth for licensing semantics.

## Origin

Per `docs/battle-plans/ROUND-TABLE-2026-04-30-practical-tooling.md` Immediate
Action: *"Architect — create `cmd/tools/` directory structure with one stub
per chosen tool."*

The round-table picked three P0 tools to publish first:

| Tool | What it is | Composing source |
|---|---|---|
| [`mimir/`](mimir/README.md) | Config drift sentry — alerts-only, REAL-METAL validated zero false positive on EAST | `cmd/heimdall-daemon/`, `pkg/enkrateia/`, `pkg/gjallarhorn/`, `pkg/gungnir/` |
| [`anamnesis-lite/`](anamnesis-lite/README.md) | Packet-zero APM — eBPF marker → trace collector → Wotan → dashboard | `services/anamnesis/`, `cmd/trace-collector-go/`, `cmd/ebpf-collector/`, `pkg/anamnesis/` |
| [`zhen-on-prem/`](zhen-on-prem/README.md) | Air-gapped SRE assistant — RAG over local model + 31 runbooks | `cmd/zhen-rag/`, `cmd/zhen-cli/`, `cmd/zhen-agent/`, `cmd/zhen-agentd/`, `runbooks/`, `raft/` |

Every tool is a **subset build** of the monorepo; tools do NOT fork.

## Curation invariants

A tool subdirectory in `cmd/tools/<name>/` MUST contain at minimum:

1. **`README.md`** — what the tool is, who it's for, doctrine context, the
   licensing surface, and adopter quickstart.
2. **`BUILD.md`** — explicit `go build` invocations that produce the tool's
   binaries from the existing source tree, plus the `scripts/build-sealed-
   cask.sh` invocation that produces a verified release artifact.
3. **`COMPONENTS.md`** — the inventory of in-tree paths the tool composes,
   with one-line summaries. So an adopter reading the source can navigate
   from "tool" back to "code" without git-archaeology.

Tools MAY additionally contain:

- **`MANIFEST.yaml`** — machine-readable component inventory (for sealed-cask
  pipeline ingestion when that lands).
- **`COMPLIANCE-EVIDENCE.md`** — pre-baked compliance-evidence pack
  (SOC2/HIPAA/PCI/etc. control mapping per the round-table's MoatGhost
  perspective). For tools where compliance is the wedge.

What tools MUST NOT contain:

- New `.go` source — tools curate; they don't reimplement.
- License variations from the standard repo licensing — uniformly GPL-3.0
  (or component-license-compatible).
- Commercial framing of any kind — see `CLAUDE.md` for the doctrine.

## Adopter quickstart (any tool)

```bash
git clone https://github.com/unheaded/unheaded
cd unheaded/cmd/tools/<tool-name>
cat BUILD.md   # explicit build commands
make           # if Makefile present, else follow BUILD.md verbatim
```

The end-state: a single binary (or small binary set) the adopter can run on
their own infrastructure, with the full GPL-3.0 source available for
inspection / modification / redistribution.

## See also

- `CLAUDE.md` — community-first doctrine
- `docs/battle-plans/ROUND-TABLE-2026-04-30-practical-tooling.md` — origin
  round-table
- `scripts/build-sealed-cask.sh` — the Sealed Cask build pipeline (per
  ADR-010); produces deterministic, signed artifacts for any tool
- `docs/adr/ADR-040-kubernetes-ecosystem-strategy.md` — K8s positioning;
  tools that ship as Helm charts compose with this strategy
