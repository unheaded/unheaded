<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-113: Third-party verdicts as a Kingdom service ("_____", name TBD)

**Status:** Proposed (pitch, parked under ADR-112). Brainstorm before acceptance; nothing is built. Addenda 2026-10-30: accuracy check, SBOM documentation, daily project + environment checks.
**Date:** 2026-10-30
**Grounded on:** `churn/2026-10-30` at `efa90e18`
**Related:**
- ADR-096 (dependency gate)
- ADR-101 (Well migration ledger)
- ADR-103 (service auth)
- ADR-092 (log pipeline, retention classes)
- ADR-006 (vanilla JS)
- ADR-112 (plan freeze)
- `docs/policy/third-party-inventory-2026-10-29.md`
- `docs/policy/dependency-watch.md`

## The pitch (Stevie, 2026-10-30, verbatim)

> make ADR to port "Unheaded Third-Party Verdicts" artifact to a integrated unheaded service/api/web page - the unheaded repo version should interact with the database/well to mirror these - this means service account and schema scoped to the application.... if other services want that information they must interact with the "_____(NEED TO PICK A NAME WILL DO WHEN WE START ADR)" to retrive info or interact with it - interaction should require API key or authenticated user.....all this should be verbosly logged and auditable if logging level is set to debug. note in ADR to brain storm further - include some claude created recs at this time as a spring board for that discussion using todays session context

## Context

**The policy.** Since 2026-10-29, no new third-party library, tool, application, action or code enters without discussion and Stevie's confirmation. Native first.

**The inventory.** `docs/policy/third-party-inventory-2026-10-29.md` (`f41eaff9`, corrections `648a2501`) lists:
- 216 direct items in nine sections: Go 18, Rust 67, Python 45, CDN browser assets 4, GitHub Actions 23, container images 25, external tools 34, foreign code 5, outside services 9.
- For each item: version, where it is used, the first commit that added it, and whether that commit was assistant co-authored (156 were).

The **Verdict** column (keep / replace with our own / cut) is empty. The generator that built it lives only in scratch (`/tmp/claude-1000/inv.py.keep`).

**The artifact.** "Unheaded Third-Party Verdicts" is a private claude.ai page (`https://claude.ai/artifact/RBRbskJtxgeEvF3UX7XjWx`). It was published 2026-10-29 to collect verdicts.
- Its `verdicts` collection is empty.
- It lives on Anthropic's servers, not in the Kingdom.
- Stevie put it, the inventory and the generator on hold the next day ("why are you publishing artifacts"; transparency is paramount). Deleting it is his call.

This ADR is the on-box replacement.

**What already exists to build on.** Nothing below is new third-party:
- **The Well.** PostgreSQL 16, TLS 1.3 only, loopback. Three databases (`unheaded_app`, `unheaded_ops`, `unheaded_config`), each with a NOLOGIN owner and least-privilege login roles.
  - Migrations go through `cmd/well-migrate` and the ledger (ADR-101); the latest is `043`.
  - Driver: `github.com/lib/pq`, already direct.
  - Today, tables live in `public`, and access is scoped per role, not per schema.
- **Append-only history.** Migration `015` shows the pattern: `audit_events` with a trigger, and UPDATE/DELETE revoked.
- **Auth (ADR-103).**
  - Per-service Ed25519 identities; callers must be in `trust.d` and `AUTH_ALLOW_CALLERS`.
  - Humans use `uhk_` API keys (hashed, Well store, `unheaded-cli apikey`) or OIDC tokens from `unheaded-auth` (19020).
  - RBAC: observer reads; admin, operator or service writes.
  - Browser pages go through a BFF (`pkg/bff`) with an HttpOnly session cookie.
- **Service plumbing.**
  - `pkg/httputil` (`unheaded_http_*` metrics), `pkg/cmdline` (ADR-111), `pkg/ports`, `pkg/uids`.
  - sysusers accounts, unit ceilings (`check-unit-ceilings.sh`).
  - Wotan's signed `audit.` prefix, which has no first-party producer yet.
- **Logs.** zerolog JSON goes to journald, then Vector, then ClickHouse. The `audit` retention class is 455 days (ADR-092).
- **Adjacent gates.**
  - `check-dependency-baseline.sh` refuses additions.
  - `check-sbom-trend.sh` fails on growth.
  - `cmd/depaudit` flags stale modules.
  - `docs/policy/dependency-watch.md` holds keep-decisions for stale modules.

## Requirements taken from the pitch

1. **Port the artifact** to a first-party service with an API and a web page. Its data is mirrored in the Well.
2. **A service account and a schema scoped to this application.** No other role reads or writes the schema directly.
3. **Every other consumer goes through the service.** That covers services, CI, CLI and Zhen.
4. **Every interaction needs an API key or an authenticated user.** Service-to-service calls carry an ADR-103 service token.
5. **Verbose, auditable logging at debug level.**

## Brainstorm (springboard; Claude's recommendations from the 2026-10-30 session; nothing decided)

### R1. Name

The blank stays blank until we start. Candidates, in the Kingdom's Norse register:
- **Forseti**, god of justice and settled disputes: verdicts.
- **Vár**, goddess of oaths and agreements: licences are agreements. Too close to `vor`, the existing search index.
- **Syn**, goddess who refuses entry and denies at trials: it fits the gate role, but collides with TCP SYN in every packet discussion.
- A Gnostic name for "the one who judges what may enter the Pleroma".

Lore decides. Whatever name is chosen also names the uid, the unit, the schema and the binary.

### R2. Where the data lives

- **A schema `<name>` in `unheaded_config`.** Verdicts are desired state (what may be in the tree), not history.
- **Ownership.** The schema is owned by NOLOGIN `<name>_owner`. The only login role is `<name>_svc`, which the service uses. `REVOKE ALL ON SCHEMA <name> FROM PUBLIC`; no grants to `ops_reader`, the retention runner or anyone else.
- **This is the Well's first per-application schema.** It sets a pattern other services could follow later. Decide whether that is wanted (Erda).
- **The alternative** is a separate database `unheaded_<name>`: stronger isolation, but one more backup set and connection pool. Recommend the schema unless isolation needs a separate database.

### R3. Data model (first sketch)

- **`items`**: ecosystem (go, rust, python, cdn, action, image, tool, foreign-code, service), name, where used, first-seen commit, assistant-added flag, first and last seen.
- **`item_versions`**: item, version or pin, commit observed.
- **`verdicts`**: append-only, enforced by trigger as in `015`. Columns:
  - item, verdict (`keep` | `replace-native` | `cut` | `undecided`), rationale;
  - decided_by (the authenticated subject), decided_at;
  - supersedes (the previous verdict id).

  The current verdict is a view over the latest row. A change of mind is a new row, never an edit.
- **`inventory_runs`**: when the inventory was scanned, at which commit, by which generator version, and the row count.

### R4. Feeding the inventory

- Port the scratch generator to Go as a subcommand of the new binary, e.g. `<name> scan --repo .`.
- It reads `go.mod`, the Cargo locks, the requirements files, the workflows, the compose images, the tool list and the CDN references. It uses stdlib only, so it adds no new third party.
- Runs on demand and from CI.
- New items arrive as `undecided`.

### R5. Making verdicts bite

- **CI gate:** fail when a direct item has no `keep` verdict, or has `cut`. This turns the 2026-10-29 policy into a check instead of a promise.
- **CI cannot reach the Well, so it reads a signed export.**
  - The service writes `docs/policy/third-party-verdicts.json`, signed with ML-DSA-65 through `pkg/gungnir`, as heimdall and yggdrasil-evidence already do.
  - The gate verifies the signature, then the content.
- `check-dependency-baseline.sh` and `dependency-watch.md` decisions could fold into this, or reference it. Decide which is canonical.

### R6. API shape

- **Reads:**
  - `GET /api/v1/items` with filters: ecosystem, verdict, assistant-added, undecided;
  - `GET /api/v1/items/{id}` and `/{id}/verdicts` (the history);
  - `GET /api/v1/export` (the signed JSON).
- **Writes:**
  - `POST /api/v1/items/{id}/verdicts`;
  - `POST /api/v1/runs` (the scanner's ingest).
- **Auth:**
  - **No `OpenReads`:** the pitch requires a key or user for every interaction. Only `/health`, `/ready` and `/metrics` stay open, per house convention. Confirm whether `/metrics` should stay open.
  - **RBAC:** verdict writes need **admin**, and only Stevie holds admin. Scanner ingest needs the scanner's service identity. Reads need observer or a listed service.
- **Events:** each verdict publishes a signed `audit.<name>.verdict` event on Wotan. Other services learn without polling, and this is the first producer on the `audit.` prefix.
- **Transport:** CLAUDE.md asks for gRPC-first. Recent services are HTTP with a Wotan publish. Decide.
- **Port and account:**
  - Port **19021** is free next to `unheaded-auth`.
  - Account: the next free uid in `pkg/uids`, as a nologin sysusers account with `MemoryMax` + `GOMEMLIMIT`.

### R7. Web page

- **A dashboard page.** Vanilla JS (ADR-006) behind the existing BFF: the browser never holds a token, and the BFF calls the service with its service token plus the user's identity.
- **No CDN assets.** The inventory itself lists four CDN assets as a problem.
- **The page** is a table with filters, a verdict control per row, a rationale field and the history drawer. Same rows the artifact had.

### R8. Logging and audit

The pitch says "verbose and auditable at debug". Recommendation: split it.
- **Writes are always audited, at every log level.** Each verdict and each ingest goes to a `verdict_audit` table in the schema (append-only), plus an `info` log line and the Wotan `audit.` event.
  - Rationale: compliance controls AU-2 and AU-12 expect a security-relevant change to be recorded whatever the verbosity.
- **Debug adds the read trail:** every request's subject (key id or user `sub`, never the key), route, item ids, result, row count, latency and trace_id.
- **Redaction:** Vector's existing rules already apply, and no secrets ever enter the log.
- **Retention:** audit rows and audit-class logs keep 455 days (ADR-092).
- **Question:** should debug read logging be switchable at runtime (SIGHUP, like Wotan's allowlists) without a restart?

### R9. Threats (BlackMage / MoatGhost first pass)

| Threat | Mitigation |
|---|---|
| Verdict tampering in the DB | Append-only trigger; the service role cannot ALTER or DROP; owner is NOLOGIN |
| Tampering on the way to CI | Signed export, verified in the gate |
| A service quietly reading the table | Schema revoked from PUBLIC; only `<name>_svc` |
| Forged verdicts | Admin-only writes, bound to the OIDC subject, and audited |
| Scanner as a supply-chain input | Stdlib parsers, fuzzed. Lich campaign candidate: hostile `go.mod`, `Cargo.lock`, YAML |
| The web page | BFF cookie, SameSite=Strict, CSP without inline script |

### R10. Migration and cut-over

- The artifact's `verdicts` are empty, so nothing moves from it.
- Seed `items` from the 2026-10-29 inventory doc, then rescan to catch the drift since.
- Once the service is live and Stevie agrees, delete the claude.ai artifact. That closes the transparency concern that put it on hold.

### R11. Compliance mapping (MoatGhost to confirm)

- NIST 800-53 CM-8 (component inventory), SR-3 and SR-5 (supply-chain controls), SA-4 (acquisition), AU-2 and AU-12 (audit).
- CIS Controls 2.1–2.3 (software inventory, authorized software).
- Wire the controls into `compliance/catalog/controls.yaml` when built.

### R12. Scope guard

- This ADR adds **no** third party.
- Under ADR-112 it stays parked. No plan, branch or code until the freeze lifts and the open questions below are answered.

## Addendum (Stevie, 2026-10-30): it is an SBOM, so it must be accurate and thoroughly documented

> Unheaded Third-Party Verdicts is a super cool sbom but we should ensure it is accurate and document it throughly

### Accuracy check, 2026-10-30 (read-only, against `churn/2026-10-30`)

**Reproducible, but incomplete.** Re-running the scratch generator (`/tmp/claude-1000/inv.py.keep`) on today's tree gives the same rows in every section as the 2026-10-29 doc (Rust transitive 428 → 423). That proves the generator agrees with itself. Independent cross-checks show what it misses:

| Section | Doc | Independent check | Gap |
|---|---|---|---|
| Container images | 25 | `image:` in every YAML + `FROM` in every Dockerfile | **4 third-party images missing:** `busybox` (k8s init containers), `haproxytech/kubernetes-ingress`, `ipfire-2.29-core199` and `opnsense-26.1.2` (LXD VM images). First-party images sit in **two registries**: `ghcr.io/stevenrbellis/unheaded/*` (12 refs) and `ghcr.io/unheaded/*` (13). |
| External tools | 34 | install/run patterns (`go run …@`, `cargo install`, `pip install`, `curl … \| sh`) | **At least 8 missing,** because the tool list is curated, so anything not on it is invisible: `bpf-linker`, `cargo-generate`, the rustup installer, `go-licenses` (`go run` in the Makefile), `cargo-license` (Makefile), `git-filter-repo`, the Nix installer, the Pi-hole installer. Several are `curl \| sh`, the riskiest kind of pull. |
| Outside services | 9 | `https://` hosts in non-test code and scripts | **At least 10 package sources missing:** `download.pytorch.org`, `download.docker.com`, `repo.radeon.com`, `deb.frrouting.org` (FRR on west), `apt.vector.dev` / `repositories.timber.io` / `sh.vector.dev`, `get.helm.sh`, `dl.smallstep.com`, `ollama.com`, `nixos.org`, `sh.rustup.rs`, `pypi.org`. |
| Python (direct) | header 45 | unique names | **45 rows, 39 unique packages.** The same package in several requirements files is counted once per file. |
| Go, Rust, GitHub Actions, CDN | 18 / 67 / 23 / 4 | `go.mod`, the Cargo manifests, `uses:` in workflows, script/link URLs | Match. |

**A second, older inventory disagrees with it.** `sbom/SBOM.md` (from `scripts/generate-sbom.sh`) was last generated **2026-03-15**. Every one of its headline numbers is stale or wrong:
- Go: 99 modules / 21 direct (today 149 / 18).
- Rust: 346 (today 423).
- "JavaScript 0", yet four CDN browser assets exist.
- Python 108, which is a `pip freeze` of the whole Zhen venv, not a manifest.

Its JSON files say "SPDX-2.3 compatible", but they are a custom shape, not a validated SPDX document. **Two inventories that disagree is the main accuracy risk.** Fixing one leaves the other wrong.

### Scope: broader than a classic SBOM

A software BOM lists the libraries a build links. This inventory also covers CI actions, container images, build and ops tools, installers, CDN assets, foreign code and outside services. In CycloneDX terms:
- **components** cover libraries, images and foreign code;
- **services** (the SaaSBOM profile) cover outside services and package sources;
- **formulation** (since 1.5) covers how it was built: tools, actions, installers;
- the existing CBOM (`compliance/crypto/cbom.cdx.json`, ADR-105) is the crypto slice of the same model.

That is a strength. Say so in the docs, and do not let the "SBOM" label shrink the scope.

### Accuracy requirements (recommendations, springboard)

1. **One source of truth, many renderings.** The verdicts service's Well schema (R2/R3) holds the inventory. From it, generate:
   - the signed CycloneDX document (components + services + formulation, plus VEX for vulnerability status);
   - an SPDX rendering if wanted;
   - the human doc (`docs/policy/third-party-inventory.md`);
   - the CI export (R5).

   `generate-sbom.sh` and `sbom/SBOM.md` are retired in favour of that, or rebuilt on it. No second hand-run inventory.
2. **Read manifests, not text, wherever one exists:**
   - `go list -m -json all` / `go mod graph`;
   - `cargo metadata` per workspace and `Cargo.lock`;
   - the Python lock files (the CI lock is hash-pinned);
   - parsed compose / k8s / LXD YAML and Dockerfiles;
   - parsed workflow `uses:`.

   Grep stays only for what has no manifest (tools, installers, URLs). There it is a *detector* that fails the gate on anything unknown, never a curated allowlist that silently misses what is not on it.
3. **A completeness gate.** Any new install or run pattern (`go run …@`, `go install`, `cargo install`, `pip install`, `npx`, `curl|sh`, an apt source, an image reference, an outside URL) not in the inventory fails CI. That turns the gaps above into errors instead of surprises.
4. **Identity per item:**
   - a Package URL (purl);
   - the exact version or digest (image `@sha256`, action commit SHA as the trivy-action pin already does);
   - a content hash where one exists;
   - the SPDX license expression (`go-licenses` / cargo metadata already produce them).
5. **Minimum elements met and stated.** NTIA 2021 asks for:
   - supplier, component name, version, other unique identifiers;
   - dependency relationship, SBOM author, timestamp.

   CISA's 2025 update (a draft for comment; comments closed 2025-10-03, final not confirmed) adds component hash, license, tool name and generation context. Meet the draft's set, and record which version of the guidance was met.
6. **Provenance stays.** "First added", by which commit, and whether it was assistant co-authored remain the inventory's distinctive columns. Document the method's limits: pickaxe finds the first diff that adds the name; trailers do not exist before the trailer era.
7. **Drift is explained, not absorbed.** Each regeneration diffs against the last signed inventory. Added, removed or changed rows show in the PR and need a verdict (new → `undecided`, and the third-party gate holds it).
8. **Periodic independent audit.** Repeat checks like the one above (manifest counts versus inventory, an install-pattern sweep, a host sweep) on a schedule. Record each audit in the doc's change log with its result.
9. **Signed and timestamped.** The inventory and its renderings are signed ML-DSA-65 through `pkg/gungnir` and verified in CI (R5), so the published inventory is the one the service produced.

### Documentation requirements ("document it thoroughly")

- **Method:** `docs/policy/third-party-inventory-METHOD.md`:
  - what counts as third party;
  - every section and its source of truth (manifest, parser or detector);
  - field definitions;
  - how first-added and assistant-added are computed, and their limits;
  - how verdicts work;
  - how to regenerate and how to verify the signature;
  - which standards and minimum elements are met;
  - known gaps.
- **Wiki:** a "Third-Party Inventory" page (plain language: what it is, why it exists, how to read a verdict), linked from Home and the sidebar. The existing SBOM wiki page points to it, not to the stale `SBOM.md`.
- **The inventory doc itself:** generated. Header with generation time, commit, tool version, signature reference and coverage statement. A change log at the bottom recording each regeneration and each audit.
- **Adopter-facing** (ties to VISION "Who It Is For"): a short "what your Unheaded install pulls from the internet, and why" page that a shop owner can read. This is the transparency Stevie asked for when the artifact went on hold.

### Daily checks, covering the project *and* the environment (Stevie, 2026-10-30)

> we need to be doing daily SBOM checks as part of this application
>
> we need to know our environment and project

The inventory has two scopes, and the service owns both.

**1. Project:** what the repo declares and pulls (everything above).

**2. Environment:** what is actually installed and running on every host. Today's session alone found five places where the two differ, none of which a repo SBOM would show:
- **apt repo on west:** nginx serves 18888 on all addresses from an old `sites-available/apt-repo`; the repo's hardened `deploy/nginx/unheaded-apt.conf` was never installed.
- **Pi-hole:** Core 6.4.1 / FTL 6.6, behind on published advisories (IOC sweep).
- **east snaps:** `core22` and `snapd` are sideloaded (revision `x1`, 2026-03-19).
- **`lxd-installer` on east:** it installs software when an operator merely types `lxc`.
- **steel-echo containers:** three running in LXD project `unheaded` that plain `lxc list` does not show.

Environment collectors, read-only and first-party, run on each host by the same pattern as `unheaded-compliance-evidence` and Huginn:

| What | Source |
|---|---|
| OS packages | `dpkg-query` (name, version, arch, source), apt sources and their signing keys |
| Snaps | `snap list --all` (revision, publisher, `x` = sideloaded) |
| Python environments | each venv's `pip list --format=json` (the Zhen venv, raft) |
| Containers | running and stopped containers with **image digests** (Docker), LXD instances in **every** project (`/snap/bin/lxc list --all-projects`), with image fingerprints |
| Inside important containers | packages in Pi-hole and similar (`dpkg-query` via `lxc exec`), and app-reported versions (`pihole -v`) |
| Our own binaries | every installed Unheaded binary and unit: path, `--version`, sha256, compared with the signed release / apt repo |
| Third-party binaries outside packages | `/usr/local/bin`, `~/go/bin`, `~/.cargo/bin`, `~/.local/bin`, with hash and version (the stale `gosec` and `golangci-lint` builds found earlier are this kind) |
| Running config vs repo config | the deploy files the repo ships (nginx, nftables, units, haproxy) diffed against what is installed |
| Kernel and firmware | `uname -r`, microcode, and firmware where readable |

**The daily run (recommendation):**
1. **Collect.** Each host collects, signs its snapshot (host key, ML-DSA-65) and ships it to the service. The service rescans the project at the current `develop` head.
2. **Diff.** Against yesterday's signed snapshot, and environment against project:
   - new or removed items;
   - version changes;
   - unknown items, i.e. installed but declared nowhere (each becomes `undecided` and goes to the verdict queue);
   - deployed config that differs from the repo.
3. **Match vulnerabilities.** Every item by purl or version against OSV data and the CISA KEV feed `kev-poller` already pulls. Status is recorded as VEX ("affected / not affected / fixed / under investigation") with a reason, so the same finding is not rediscovered daily.
4. **Report.**
   - Signed daily record into the schema, audited per R8.
   - Wotan `audit.<name>.inventory` event.
   - Alerts only on change: a new unknown item, a KEV match, an unexplained config drift, a sideloaded package. That goes to alert-mailer and a findings-register entry (ADR-098).
   - A plain-language daily line: "nothing changed", or what changed and why it matters.
5. **Gate.** Stays as R5. CI blocks on the project. The environment produces findings, not blocks, so a host is never broken by its own inventory.

**Fit:**
- **Schedule:** the Sentinel daily loop (03:00 UTC) or its own timer.
- **Retention:** daily snapshots in the audit class (455 days, ADR-092).
- **Third party:** collectors are stdlib Go reading the tools already on the hosts (`dpkg-query`, `snap`, `lxc`, `docker`), so none is new.
- **Vulnerability data:** OSV and KEV are outside *data* services. They belong in the "outside services" section of the inventory itself, and the third-party sitting should confirm them.

### Sources (addendum, retrieved 2026-10-30)

- [CISA: 2025 Minimum Elements for an SBOM (draft resource page)](https://www.cisa.gov/resources-tools/resources/2025-minimum-elements-software-bill-materials-sbom) and [the PDF](https://www.cisa.gov/sites/default/files/2025-08/2025_CISA_SBOM_Minimum_Elements.pdf)
- [CMS summary of the CISA draft](https://security.cms.gov/posts/new-cybersecurity-guidance-cisa-software-bill-materials-sbom)
- [CycloneDX specification (releases: 1.6 April 2024, 1.7 October 2025)](https://github.com/CycloneDX/specification/)

## Open questions for the brainstorm

1. Name (R1).
2. Schema in `unheaded_config`, or its own database (R2)? Is per-application schema the new Well pattern?
3. Verdict vocabulary: are `keep` / `replace-native` / `cut` / `undecided` enough? Should a verdict carry a deadline, e.g. "replace by"?
4. Does the CI gate block, or only report, at first (R5)? Which file is canonical: the export, `dependency-watch.md`, or the dependency baseline?
5. gRPC-first or HTTP + Wotan (R6)? Is `/metrics` open or authenticated?
6. Are writes always audited, with debug adding the read trail (R8)? Or everything gated on debug, as the pitch reads literally?
7. Who besides Stevie may write? An operator role for "undecided → proposed" suggestions, with only admin able to decide?
8. Should Zhen and Claude sessions query it through the MCP server (read-only), so agents check verdicts before suggesting a dependency?
9. One source of truth: retire `generate-sbom.sh` / `sbom/SBOM.md`, or rebuild them on the service? CycloneDX as the primary format, SPDX as a rendering, or both first-class?
10. Completeness gate (accuracy R3): block CI on any unknown install pattern, URL or image, from day one or after a report-only period?
11. Environment scope: which hosts and containers first (west, east, Pi-hole, steel-echo)? Is an unknown package on a host a finding (recommended), or only a report line?
12. Vulnerability data: OSV + CISA KEV as the daily match sources (both outside data services, for the third-party sitting)? VEX statements written by whom, Stevie only or operator too?
13. Daily schedule: inside the Sentinel 03:00 loop, or its own timer? And who reads the daily line: email, the dashboard, or both?

## Decision

None yet. Parked under ADR-112.
