<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-095 — Slow-roll review: flag everything, then one at a time

Two tracks, one method: **structure** (nested control flow that should be a
type, a table or a function) and **supply chain** (every external package the
build pulls in). Both are flagged into a register in one pass, then reviewed
one entry at a time.

**Status:** Proposed — deferred. Starts after the current develop↔staging
sprint is promoted to `main`, which gives it a clean baseline. It is a
precondition for promoting Unheaded publicly, not for anything current.
**Date:** 2026-09-24
**Related:** ADR-004 (no-external-deps policy and its exceptions register),
ADR-093 (what shape is this thing; reachability first),
`docs/LIVE-PATHS.md`, ADR-094 (the `deriveMemstats` / `deriveProcess`
extractions are the worked example below).

## Context

The repository is public on GitHub and on Stevie's résumé, but it has never
been promoted anywhere. The reason given is that there is not yet a true
proof of concept of the vision. When there is one, people will read the code,
and some of it reads badly. The worst of it is long functions of nested
`if`/`switch` that do several jobs at once. A type with methods, an
interface, or a lookup table would say the same thing more plainly.

This ADR records the goal and fixes the method now, so that when the work
starts it runs in a way that finds real problems rather than producing churn.

### How big the problem is (measured, not guessed)

`golangci-lint` v2 with only `gocognit` (> 30), `nestif` (> 5) and `gocyclo`
(> 30) enabled, over the root module (`cmd/ pkg/ services/ tomb/`),
2026-09-24:

| | findings |
|---|---|
| `gocognit` > 30 | 141 |
| `nestif` > 5 | 101 |
| production files affected | **83 of 668** |
| production findings | 173 (cmd 45, pkg 110, services 18) |
| test-file findings | 69 |

Highest cognitive complexity in production code:

| score | function | reach (`LIVE-PATHS.md`) |
|---|---|---|
| 103 | `cmd/trace-collector-go/main.go` `runUnifiedMode` | TOOL |
| 99 | `pkg/waf/detection/sqli.go` `(*SQLTokenizer).Tokenize` | — |
| 93 | `pkg/http/router.go` `(*radixNode).getValue` | — |
| 92 | `pkg/ebpf/loader.go` `parseELF` | — |
| 89 | `pkg/deploy/strategy/rolling.go` `(*RollingStrategy).Execute` | — |
| 84 | `pkg/ebpf/loader.go` `(*NativeLoader).Load` | — |
| 82 | `pkg/waf/rules/dsl.go` `(*DSLParser).tokenize` | — |
| 76 | `cmd/protocol-api/anamnesis.go` `streamMock` | ORPHAN |
| 76 | `cmd/kanban-app/main.go` `main` | CONTAINER |
| 73 | `pkg/nix/builder.go` `ParseFlake` | — |

Rust (`crates/`, `ebpf/`) is not measured yet; `clippy::cognitive_complexity`
is the equivalent and must be run before the register is built. By ADR-093's
count `crates/` is the busiest directory by 3×, so leaving it out would
review the smaller half.

A score is a pointer to a place worth reading, not a verdict. Two of the top
three are tokenizers. A tokenizer is a state machine over bytes, and a
`switch` on the current byte is usually its clearest honest form. Some
entries will end in "leave it", and that is a valid outcome.

## Decision

### 1. The vocabulary, translated to the languages we write

The object-oriented vocabulary (class, object, attribute, method,
constructor; encapsulation, abstraction, inheritance, polymorphism) is the
right way to *think* about this. But Go and Rust have no classes and no
inheritance, so the review uses what each concept becomes in them:

| concept | Go | Rust |
|---|---|---|
| class | a named type (usually a struct) with methods | struct/enum + `impl` |
| object | a value of that type | same |
| attribute / field | struct field | struct field |
| method | func with a receiver | fn in an `impl` block |
| constructor | `NewX(...)` returning a valid value | `X::new(...)` |
| encapsulation | unexported fields, exported methods | private fields, `pub` methods |
| abstraction | a small interface at the point of use | a trait |
| inheritance | **none — use composition** (embedding, fields) | **none — use composition** and trait default methods |
| polymorphism | interfaces | traits; `enum` + `match` for closed sets |

"Replace it with a class" therefore means one of these, in rough order of
preference:

1. **Guard clauses / early return.** Most deep nesting is an `if err == nil`
   ladder. It flattens with no new types at all.
2. **Extract a pure function.** Pull the computation out of the I/O. Today's
   example is ADR-094: `deriveMemstats` and `deriveProcess` were pulled out of
   collectors that read the runtime and `/proc`, and that turned four bugs
   invisible at runtime into bugs a unit test catches.
3. **A lookup table instead of a `switch`.** Use a map or slice when the
   cases differ only in data (name → handler, opcode → width), not in control
   flow.
4. **A type with methods.** Use one when several functions pass the same five
   values around and check the same invariants. The values become fields,
   the checks move into the constructor, and the functions become methods.
5. **An interface (Go) or trait/enum (Rust) for dispatch.** Use one when a
   `switch kind { ... }` is repeated in several places over the same set of
   kinds. Each kind becomes a type implementing one method, and adding a kind
   stops meaning "find every switch". `ForgeBackend` (ADR-048) is this
   repository's existing precedent.

Never inheritance-shaped hierarchies emulated through embedding. Never an
interface with one implementation and no test double. Never a new file for a
30-line switch that was already clear.

### 2. Flag everything first, in one pass

Before any code changes, build a **register**
(`docs/review/STRUCTURAL-REGISTER.md`): one row per flagged function, with the
file:line, the measured scores, its reach classification from
`LIVE-PATHS.md`, and a status of `open`. The measurement above is its seed.
Flagging is mechanical and happens once. It decides nothing about what to do.

### 3. Then one at a time — work in progress is limited to one

This is the core of the decision. The register is **not** a batch to churn
through. Exactly one entry is `in-review` at any time, and the next one does
not open until the current one is closed.

Reviewing one entry means drilling all the way down:

- Read the whole file, not only the flagged function, and find every caller.
- Confirm it is reached (ADR-093 rule 3). An unreached function is a
  candidate for **deletion**, not refactoring. Polishing dead code is the
  worst possible use of this time.
- If behaviour is not already pinned by tests, write characterization tests
  first, and watch them fail against a planted bug (ADR-093 rule 1).
- Decide: **refactor**, **leave** (with the reason recorded; see the
  tokenizer note above), or **delete**.
- If refactoring, the commit changes structure only, with no behaviour change
  mixed in. Bugs found along the way are recorded and fixed in their own
  commit, before or after, never inside the refactor.
- Close the entry in the register with the outcome and the commit hash.

The reason for one at a time is that nearly every serious defect this repo
has turned up came from reading one thing closely, not from a sweep. The
sprint that produced ADR-093 found fifteen instances of "a correct
implementation nothing reaches", each by following one path to its end. A
batch refactor across forty files would be exactly the kind of change
nobody can review, and it would carry that defect class forward.

### 4. Ordering

Live paths first (CONTAINER, SUPERVISED), highest score first within them.
Then libraries imported by live paths. TOOL and ORPHAN binaries last, and for
ORPHAN the first question is whether the binary should exist at all. Test
files are last of all: a long table-driven test is often fine.

### 5. A ratchet, once the review starts

When the first entry opens, add a gate in the style of
`check-tmp-log-baseline.sh`: the set of flagged functions may only shrink. A
new function over the thresholds fails CI, and one closed as "leave" is
listed with its reason. The gate must be registered in
`check-gates-can-fail.sh` and watched failing before it counts.

### 6. Track two — supply chain: own it, or vet it and record why

ADR-004 already sets the rule: production code depends only on the standard
libraries, internal packages, and **approved exceptions**. Those come from
established organisations, with repositories created before July 2019, and
each one needs an owner sign-off recorded in its register. The rule stands
and is not restated here. What this track adds is the finding that **the
tree does not match that register**, and a method for closing the gap.

**Measured 2026-09-24:**

| ecosystem | what is pulled in |
|---|---|
| Go (root module) | **21 direct** modules, 113 in the full graph |
| Rust | **411 unique crates** from crates.io across 18 `Cargo.lock` files (`crates/zhend` alone: 340) |
| Python | 4 requirements files: 2 fully pinned (`raft/`, `sbom/`), 2 unpinned (`notebooks/`, `tomb/grimoire/rag/`); **none hash-pinned** |
| npm | 1 devDependency (root `package.json`, lint only) |
| Container bases | 9 distinct `FROM` images; **none pinned by digest** |
| GitHub Actions | 36 distinct `uses:`; **7 pinned to a commit SHA**, 29 to a mutable tag |

**ADR-004's register against the 21 direct Go modules:**

- **Recorded:** `grpc`, `protobuf`, `circl`, `lib/pq`, and `x/sys` by name in
  the policy.
- **In use with no approval record (12):** `BurntSushi/toml`, `cilium/ebpf`,
  `fsnotify`, `google/uuid`, `gorilla/mux`, `gorilla/websocket`,
  `prometheus/client_golang`, `rs/zerolog`, `sony/gobreaker`,
  `yuin/goldmark`, `gopkg.in/yaml.v3`, `modernc.org/sqlite`. Also
  `x/crypto`, `x/text` and `x/time`, which are quasi-standard but not named
  the way `x/sys` is.
- **ADR-004 says these were already replaced, but production code still
  imports them:** `rs/zerolog` (32 files; `pkg/logger/` exists alongside it),
  `cilium/ebpf` (5; alongside `pkg/ebpf/`), and `gorilla/websocket` (2;
  alongside `cmd/dashboard-backend/internal/websocket/`). This is ADR-093's
  defect class again: a correct replacement that nothing reaches. ADR-004's
  "zero supply chain risk" consequence is not true of the tree today.
- `prometheus/client_golang` is already being retired under ADR-094.

**Method — the same as track one:**

1. **Flag everything into the register in one pass.** One row per direct
   dependency, per ecosystem, with: what imports it (production vs test
   vs tool), its transitive count, owner organisation, repository creation
   date (ADR-004's age check), licence, and pin strength (version, lockfile
   hash, digest, SHA).
2. **One entry at a time.** For each, decide:
   - **own**: an internal replacement exists or is worth writing, so migrate
     and delete the import. ADR-094 is the model: parity-tested against the
     real thing before any call site moves.
   - **vet and keep**: record who maintains it, why replacement is not
     worth it, how it is pinned, and which code paths reach it. Then add it
     to ADR-004's register with the owner's sign-off.
   - **remove**: the import is dead or trivially replaced by the standard
     library.
3. **Rust counts.** 411 crates is the largest surface in the repository and
   the least examined. Flag it per workspace, starting with the binaries on
   live paths (`LIVE-PATHS.md`), not with the fuzz and tool workspaces.
4. **Pinning is cheap and can go first.** Pinning actions to SHAs and base
   images to digests changes no code and closes the tag-hijack class
   outright. These are the only entries allowed to go in a batch, because
   each is a one-line mechanical change with nothing to review underneath.
5. **Ratchet, once started.** Add a gate in the style of
   `check-secrets-baseline.sh`: the set of direct dependencies per ecosystem
   may only shrink unless a register entry exists for the new one. It must be
   registered in `check-gates-can-fail.sh` and watched failing.

**Also flagged for amendment under the community-first doctrine:** ADR-004's
Context says "Selling self-hosted infrastructure while depending on dozens
of third-party packages undermines the brand." That is commercial framing.

## Consequences

**Good.** The code people read after promotion is the code that was read
closely first. Dead code gets deleted rather than polished. Every refactor is
small enough to review, because each one is a single entry.

**Costs.** This is slow on purpose. At one entry at a time, 173 structural
findings plus about 20 Go and 411 Rust dependencies is months of background
work. That is acceptable because it is not on the
critical path: the proof of concept is.

**Not decided here.** The thresholds (30 / 5) are golangci-lint's
conventional values and may need tuning once real entries have been reviewed.
The Rust complexity measurement is outstanding. Whether `crates/zhend`'s 340
crates can be vetted per crate, or need a per-workspace decision instead,
is left to its register entry.
