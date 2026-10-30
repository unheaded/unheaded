<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-113: Third-party verdicts as a Kingdom service ("_____", name TBD)

**Status:** Proposed (pitch, parked under ADR-112). Brainstorm before acceptance; nothing is built.
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

## Open questions for the brainstorm

1. Name (R1).
2. Schema in `unheaded_config`, or its own database (R2)? Is per-application schema the new Well pattern?
3. Verdict vocabulary: are `keep` / `replace-native` / `cut` / `undecided` enough? Should a verdict carry a deadline, e.g. "replace by"?
4. Does the CI gate block, or only report, at first (R5)? Which file is canonical: the export, `dependency-watch.md`, or the dependency baseline?
5. gRPC-first or HTTP + Wotan (R6)? Is `/metrics` open or authenticated?
6. Are writes always audited, with debug adding the read trail (R8)? Or everything gated on debug, as the pitch reads literally?
7. Who besides Stevie may write? An operator role for "undecided → proposed" suggestions, with only admin able to decide?
8. Should Zhen and Claude sessions query it through the MCP server (read-only), so agents check verdicts before suggesting a dependency?

## Decision

None yet. Parked under ADR-112.
