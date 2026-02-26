# Future Monorepo Split: Strategic Planning Document

**STATUS: STRATEGIC PLANNING ONLY — DO NOT IMPLEMENT NOW**

---

## CRITICAL DECISION RECORD

This monorepo split initiative is **explicitly deferred to post-public-release**. Do not begin this work until ALL of the following conditions are met:

1. **Public release completed** — Unheaded has shipped to the open-source community
2. **Stable API v1.0** — Core daemon, protocol, and SDK APIs are finalized and backwards-compatible
3. **Test coverage 80%+** — Unit, integration, and E2E tests cover critical paths with high confidence
4. **Documentation complete** — Architecture docs, API reference, contributor guides are polished and comprehensive
5. **Dedicated DevOps engineer assigned** — CI/CD infrastructure overhaul requires sustained focus

**Timeline estimate:** T+6 to T+12 months after public release.

If these conditions are not met, the split will cause more harm than good: contributor confusion, circular dependencies, fragmented CI/CD, and maintenance burden that outweighs the benefits of modularity.

---

## 1. RATIONALE: WHY SPLIT THE MONOREPO?

The Unheaded monorepo is currently a single, comprehensive codebase covering the daemon core, protocols, infrastructure, observability, and security subsystems. This unified structure is appropriate for the current development phase, but faces scalability challenges as the project matures.

### Pain Points of the Current Monorepo

**Contributor Onboarding.** New contributors must clone and understand the entire ecosystem (Go daemon, Rust eBPF, protocol specs, IaC, dashboards) even if they only care about, say, writing a routing plugin. This creates a high cognitive load and slow feedback loops during initial setup.

**CI/CD Bloat.** Every commit runs the full test suite across all subsystems. Protocol changes trigger dashboard and eBPF rebuilds. IaC validation blocks daemon releases. This creates bottlenecks and long CI times (currently 15–20 minutes per PR), which frustrates rapid iteration and reduces developer velocity.

**Access Control Granularity.** The monorepo uses a single CODEOWNERS model. If we want external contributors to help with routing configs or observability dashboards, we must grant repo-wide access. Security-sensitive code (protocols, firewall rules) sits alongside permissive areas.

**Dependency Management Complexity.** The single go.mod file means all internal packages share a version space. Protocol updates risk breaking the SDK. Firewall rule changes might conflict with routing updates. There's no clean separation of concerns, only internal package boundaries (fragile).

**Search and Navigation.** GitHub code search, IDE jumping, and git blame become slower and noisier. Finding all uses of a protocol feature requires grepping across daemon, tests, IaC, and docs.

### Benefits of Splitting

**Faster CI Pipelines.** Each repo has its own test suite and CI workflow. Protocol changes no longer trigger daemon rebuilds. Observability dashboards can be deployed independently of the core daemon.

**Focused Onboarding.** A contributor who wants to write observability dashboards clones only unheaded/observability, its dependencies, and a README. No unnecessary complexity.

**Clean Dependency Graphs.** Go modules create explicit versioning: core → sdk → iac. If iac breaks, core continues to work. Changes flow in one direction, eliminating cycles.

**Cleaner Permissions Model.** The routing team can own unheaded/routing with their own CODEOWNERS and branch protection rules. External teams can fork routing configs without full Unheaded access.

**Scalable Governance.** Each repo can have its own release cadence, semantic versioning, and maintenance team.

---

## 2. PROPOSED REPOSITORY TOPOLOGY

The monorepo will be split into **nine specialized repositories**, each with a clear scope and ownership model. The proposed structure reflects the current directory layout and architectural boundaries.

### unheaded/core
**Purpose:** The main Unheaded daemon, daemon-agnostic utility packages, and core runtime libraries.

**Contents:**
- `pkg/` — Public-facing packages (daemonize, logging, config, etc.)
- `internal/` — Private implementation details
- `cmd/unheaded` — Main daemon binary
- `cmd/unheaded-ctl` — Control plane utilities
- `tests/` — Integration tests (unit tests colocate with code)
- `go.mod`, `go.sum` — Daemon dependencies

**Ownership:** Core maintainers. High stability bar.

**Not included:** Routing plugins, firewall rules, IaC, observability exporters, eBPF programs, protocol specs, documentation.

---

### unheaded/protocol
**Purpose:** Protocol specifications, wire formats, and RFC documentation.

**Contents:**
- `docs/protocol/` — Monad, Sophia, Wotan RFCs, versioning
- `proto/` — Protobuf definitions (if applicable)
- `docs/protocol/wire-format.md` — Binary serialization specs
- `docs/protocol/version-history.md` — Evolution of protocols
- `examples/` — Minimal protocol implementations for reference

**Ownership:** Protocol committee / steering council. Must maintain semver compatibility once public.

**Not included:** Daemon implementation, Go code (only specs and reference implementations).

---

### unheaded/iac
**Purpose:** Infrastructure as Code: NixOS, Docker, and LXD deployment templates.

**Contents:**
- `nixos/` — NixOS modules (unchanged from monorepo)
- `docker/` — Dockerfile and docker-compose templates (generic IaC, not subsystem-specific)
- `lxd/` — LXD container profiles and image definitions
- `scripts/provision/` — Setup and deployment scripts
- `docs/` — IaC architecture, deployment guides

**Ownership:** DevOps and infrastructure team.

**Not included:** Subsystem-specific Dockerfiles (firewall, routing) — those go to their respective repos.

---

### unheaded/routing
**Purpose:** Routing subsystem: FRR/BIRD configs, BGP/OSPF policies, and routing plugins.

**Contents:**
- `routing/` — Routing daemon configs (unchanged from monorepo)
- `cmd/frr-exporter/`, `cmd/bird-exporter/` — Routing metrics exporters
- `scripts/routing/` — Deployment and debug scripts
- `tests/routing/` — Integration tests for routing policies
- `docker/routing/` — Dockerfile for routing container
- `docs/` — Routing architecture, plugin development guide

**Ownership:** Networking team.

**Dependencies:** Imports from unheaded/core for logging, config, metrics APIs.

---

### unheaded/firewall
**Purpose:** Firewall subsystem: iptables/nftables rules, firewall policies, and Docker/LXD container security.

**Contents:**
- `docker/firewall/` — Firewall sidecar container Dockerfile
- `lxd/firewall/` — LXD firewall profiles
- `nixos/modules/firewall*` — NixOS firewall module
- `scripts/firewall/` — Rule generation and testing scripts
- `tests/firewall/` — Rule validation tests
- `docs/` — Firewall architecture, rule DSL, policy examples

**Ownership:** Security and networking team.

**Dependencies:** Imports from unheaded/core for config and metrics.

---

### unheaded/observability
**Purpose:** Monitoring, metrics, tracing, and dashboards.

**Contents:**
- `monitoring/` — Prometheus rules, alerting policies, example dashboards
- `cmd/ebpf-exporter/` — Metrics exporter for eBPF events (if applicable)
- `dashboard/` — Grafana/Kibana dashboards and templates
- `docs/` — Observability architecture, metric reference, alarm runbooks

**Ownership:** Observability and SRE team.

**Dependencies:** Imports metrics APIs from unheaded/core.

---

### unheaded/docs
**Purpose:** Central documentation, project vision, and GitHub wikis.

**Contents:**
- `docs/` — Comprehensive documentation (moved from root)
- `wiki/` — GitHub wiki source (synchronized to all repo wikis via git subtree)
- `README.md` — Master project README
- `VISION.md` — Project vision and long-term strategy
- `CHANGELOG.md` — Consolidated changelog
- `CONTRIBUTING.md` — Contributor guide (central authority)

**Ownership:** Documentation maintainers and steering council.

**Visibility:** Public and highly accessible. Central hub for cross-repo navigation.

---

### unheaded/security
**Purpose:** Security policies, scanning, and threat detection.

**Contents:**
- `docs/security/` — Security policies, threat model, attack surface analysis
- `scripts/security/` — SBOM generation, vulnerability scanning, hardening scripts
- `suricata/` — Suricata IDS/IPS rule sets and configurations
- `tests/security/` — Security-focused integration tests
- `docs/` — Security architecture, compliance checklists

**Ownership:** Security team.

**Dependencies:** May import from core for logging/config APIs.

---

### unheaded/ebpf
**Purpose:** eBPF programs, kernel interfaces, and low-level monitoring.

**Contents:**
- `ebpf/` — Rust eBPF programs (Shield, Shim, event collectors)
- `ebpf/maps/` — Shared BPF map definitions and schemas
- `Cargo.toml`, `Cargo.lock` — Rust dependencies
- `tests/ebpf/` — eBPF verification and unit tests
- `docs/` — eBPF architecture, syscall hooks, map design

**Ownership:** Systems and kernel team.

**Dependencies:** Minimal (pure kernel interfaces). Consumed by core daemon and observability exporters.

---

### unheaded/sdk (Future)
**Purpose:** Official SDK for third-party integrations.

**Contents:**
- `sdk/go/` — Go client library (generated from protocol specs)
- `sdk/rust/` — Rust client library (if applicable)
- `sdk/examples/` — Example integrations (routing plugin, custom metric exporter)
- `docs/` — SDK architecture, API reference, integration guide

**Ownership:** Product and ecosystem team.

**Maturity:** This repo is created later, once the daemon and protocol APIs are stable and documented. It may initially be empty or marked as beta.

---

## 3. WIKI STRATEGY: DOCUMENTATION ACROSS REPOS

GitHub wikis are a lightweight way to maintain per-repo documentation, but they can become fragmented without a clear governance model.

### Central Wiki Hub

The **unheaded/docs** repository will be the source of truth. Its wiki serves as the master navigation point:

- **Home.md** — Landing page with links to all eight repositories
- **_Sidebar.md** — Global navigation across all repos (Home, Protocol, Core, IaC, Routing, Firewall, Observability, Security, eBPF)
- **_Footer.md** — Links to vision, changelog, contributing guide, security policy
- **Getting Started** — Onboarding flow (read docs repo first, then branch to specific repos)

### Per-Repository Wiki Pattern

Each of the eight main repositories will have its own GitHub wiki with a standard structure:

**Home.md**
```
# [Repo] Wiki

[Brief description of subsystem]

## Quick Links
- [Architecture](Architecture)
- [Getting Started](Getting-Started)
- [API Reference](API-Reference)
- [Contributing](Contributing)
- [Troubleshooting](Troubleshooting)

## Global Links
[Footer template linking back to central docs]
```

**_Sidebar.md** (per repo)
```
## [Subsystem Name]
- [[Home]]
- [[Architecture]]
- [[Getting Started]]
- [[API Reference]]
- [[Troubleshooting]]

## Other Repos
- [Unheaded Central](https://github.com/unheaded/docs/wiki)
- [Core](https://github.com/unheaded/core/wiki)
- [Routing](https://github.com/unheaded/routing/wiki)
- [Firewall](https://github.com/unheaded/firewall/wiki)
- ...
```

### Cross-Wiki Linking

Absolute GitHub URLs ensure links work from any wiki:
```markdown
See the [Protocol RFC](https://github.com/unheaded/protocol/wiki/Monad-RFC) for details.
Firewall rules are documented in [unheaded/firewall](https://github.com/unheaded/firewall/wiki).
```

### Wiki Synchronization via Git Subtree

GitHub wikis are actually git repositories (`.wiki.git` suffix). To keep docs/wiki/ in sync with the published GitHub wiki, use a git subtree workflow:

```bash
# In unheaded/docs repo:
git subtree pull --prefix wiki origin wiki --squash
# (Edit wiki files locally)
git subtree push --prefix wiki origin wiki --squash
```

This allows documentation to be reviewed via pull requests before publication.

### What Stays in the Core Monorepo (Not a Wiki)

High-context, session-specific information remains in the root of unheaded/core:

- **CLAUDE.md** — AI context and team handoffs
- **battle-plan-*.md** — Sprint-level battle plans
- **RELEASE_CHECKLIST.md** — Release procedures
- **SPRINT-ORCHESTRATOR.md** — Sprint planning framework

These are internal working documents, not for public consumption. They live in the core repo because they document the ongoing effort to build Unheaded. Wikis are polished, public-facing documentation; battle plans are raw, evolving artifacts.

---

## 4. MIGRATION STRATEGY: HOW TO EXTRACT REPOS

The extraction process is high-risk if done wrong. Premature splitting, incomplete tests, or unresolved dependencies will create a cascading failure. This section outlines a phased, conservative approach.

### Phase A: Preparation (T+0 to T+3 months)

Before any repo extraction, the monorepo must be battle-hardened:

- Tag v1.0 release with full changelog and API stability guarantee
- Achieve 80%+ test coverage across daemon, protocol, IaC, and observability subsystems
- Document all public APIs (daemon gRPC/REST, routing plugins, firewall rule DSL, eBPF maps)
- Green CI/CD pipeline with no known flakes
- Establish governance (OWNERS files, CODEOWNERS, release process)
- Set up GitHub organization structure (unheaded/core, unheaded/protocol, etc. as empty placeholders)

During this phase, developers familiarize themselves with the extraction tooling and practice extraction on feature branches.

### Phase B: Extract Protocol and Docs Repos (T+3 to T+6 months)

The two lowest-risk repos to extract first:

**unheaded/protocol:** Contains no Go code dependencies, only specs and RFCs. Extract via:

```bash
git clone --bare https://github.com/unheaded/monorepo unheaded-filter.git
cd unheaded-filter.git
git filter-repo --path docs/protocol --path proto --path LICENSE-PROTOCOLS
# Review history is correct
git push --mirror https://github.com/unheaded/protocol
```

**unheaded/docs:** Central documentation and wikis. Extract with:

```bash
git filter-repo --path docs --path wiki --path README.md --path VISION.md --path CHANGELOG.md --path CONTRIBUTING.md
```

After extraction, create git submodule pointers in unheaded/core if needed, or simply update documentation links in the monorepo to point to the extracted repos.

### Phase C: Extract Subsystem Repos (T+6 to T+9 months)

Once protocol and docs are stable, extract subsystem repos in dependency order (bottom-up):

1. **unheaded/ebpf** — No Go dependencies, cleanest extraction
2. **unheaded/iac** — Depends on nothing; pure infrastructure code
3. **unheaded/security** — Mostly scripts and configs; minimal Go
4. **unheaded/routing** — Depends on core daemon APIs; requires careful replace directives
5. **unheaded/firewall** — Similar to routing; depends on core

Each extraction uses git-filter-repo to preserve history while removing other subsystem code.

### Phase D: Update Cross-References (T+9 to T+10 months)

Once repos are extracted:

- Update all import paths in core: `internal/routing` → `github.com/unheaded/routing`
- Replace direct code references with gRPC/REST calls where appropriate
- Add explicit `replace` directives in go.mod files to point to extracted repos
- Update CI/CD: GitHub Actions per-repo instead of monorepo CI
- Test all integrations: core daemon starts, firewall rules load, routing updates work
- Document new workflow (which repos to clone, in which order)

### Phase E: Sunset the Monorepo (T+10 to T+12 months)

Once all extractions are complete and tested:

- Tag final release of monorepo (v1.0-final-monorepo)
- Archive monorepo as read-only (disable pushes, allow clones for historical reference)
- Redirect all docs/issues/PRs to new repos
- Update main README to point to unheaded/docs as primary source

---

## 5. GO MODULE SPLIT PLAN

Currently, Unheaded uses a single `go.mod` at the root. After splitting, each repo needs its own Go module to enable independent versioning.

### Current State

```
go.mod: github.com/unheaded/unheaded
├── pkg/daemonize
├── pkg/config
├── pkg/logging
├── internal/daemon
├── cmd/ebpf-exporter
├── ...
```

Every package is importable as `github.com/unheaded/unheaded/pkg/daemonize`, creating tight coupling.

### Post-Split Modules

**github.com/unheaded/core**
```go
require github.com/unheaded/ebpf v0.1.0
require github.com/unheaded/routing v0.1.0
```

**github.com/unheaded/routing**
```go
require github.com/unheaded/core v0.1.0
```

**github.com/unheaded/ebpf** (minimal dependencies)
```go
// No Unheaded module dependencies; only standard library and kernel interfaces
```

### Import Path Changes

All references to `github.com/unheaded/unheaded/pkg/...` become:

- Logging/config APIs shared between modules → `github.com/unheaded/core/pkg/...`
- Routing plugin interface → `github.com/unheaded/routing/pkg/plugin/...`
- eBPF map schemas → `github.com/unheaded/ebpf/maps/...`

This requires a coordinated refactoring phase after extraction.

### Versioning Strategy

- **Pre-1.0 split:** All modules at v0.x.y (alpha/beta)
- **Post-1.0 split:** Core and public packages at v1.0+; internal libraries at v0.x
- **SDK stabilization:** Once SDK reaches v1.0, no breaking changes without major version bump
- **Go module proxies:** Use proxy.golang.org for caching; consider private proxy for internal modules

---

## 6. CI/CD IMPLICATIONS: PIPELINES AND WORKFLOWS

The monorepo currently has one CI pipeline (GitHub Actions). After splitting, we need nine parallel pipelines plus a shared workflow repository.

### Per-Repository CI

Each of the eight main repositories gets its own `/.github/workflows/` directory:

**unheaded/core/.github/workflows/test.yml**
```yaml
name: Test & Lint
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./...
      - run: golangci-lint run
```

**unheaded/routing/.github/workflows/test.yml**
```yaml
# Similar structure
# Imports core via: go get github.com/unheaded/core@latest
```

**unheaded/ebpf/.github/workflows/build.yml**
```yaml
# Rust cargo build/test
```

### Shared Workflow Repository

A new **unheaded/.github** repository (or workflows/ directory) contains reusable workflows:

```
.github/
├── workflows/
│  ├── release.yml          # Shared release workflow
│  ├── notify-slack.yml     # Shared notifications
│  ├── publish-docs.yml     # Shared wiki sync
│  └── security-scan.yml    # Shared SAST/dependency checks
```

Each repo's CI references these shared workflows:

```yaml
jobs:
  release:
    uses: unheaded/.github/.github/workflows/release.yml@main
```

### Cross-Repo Dependency Management

When routing.v0.2.0 is released, core/go.mod can immediately upgrade:

```bash
cd core && go get -u github.com/unheaded/routing@v0.2.0
```

The Go module proxy ensures consistent builds and version pinning.

### Release Train (Optional)

For coordinated releases, establish a sequential order:

1. **core** — Released first (most dependencies); tagged and pushed
2. **sdk**, **routing**, **firewall** — Released next (depend on core)
3. **iac**, **observability** — Released last (consume stable APIs)
4. **docs** — Updated with release notes

This prevents a dependency chain where routing.v0.2.0 is released but core.v0.2.0 isn't yet available.

---

## 7. GITHUB WIKI STRUCTURE TEMPLATE

Each repository will use a consistent wiki structure for navigability.

### Home.md Template

```markdown
# [Repository Name] Wiki

[One sentence describing the subsystem]

## Quick Navigation

- [Architecture](Architecture) — System design and data flow
- [Getting Started](Getting-Started) — Setup and basic usage
- [API Reference](API-Reference) — Public interfaces and examples
- [Troubleshooting](Troubleshooting) — Common issues and fixes
- [Contributing](Contributing) — How to contribute (see main CONTRIBUTING.md)

## Global Resources

- [Unheaded Central Docs](https://github.com/unheaded/docs/wiki)
- [Project Vision](https://github.com/unheaded/docs/wiki/Vision)
- [Contributing Guide](https://github.com/unheaded/docs/wiki/Contributing)
- [Security Policy](https://github.com/unheaded/security/wiki)

## Key Concepts

[2–3 paragraphs explaining the core concepts of this subsystem]

## Related Repositories

[Links to related repos that depend on or are depended upon by this repo]
```

### Architecture.md Template

```markdown
# Architecture: [Subsystem]

## Overview

[Diagram of major components]

## Components

### Component A
- Purpose: [Description]
- Inputs: [Data types]
- Outputs: [Data types]

### Component B
- Purpose: [Description]
- ...

## Data Flow

[Sequence or flow diagram]

## External Dependencies

- Depends on: [other repos]
- Depended on by: [other repos]
```

### API Reference Template

```markdown
# API Reference: [Subsystem]

## Functions / Endpoints

### function_name(args) -> ReturnType

**Description:** [What does this do?]

**Example:**
\`\`\`go
// Example code
\`\`\`

**Related:** [Links to other functions or docs]
```

### Troubleshooting.md Template

```markdown
# Troubleshooting: [Subsystem]

## Common Issues

### Issue: [Description]

**Symptoms:** [What does the error look like?]

**Root cause:** [Why does this happen?]

**Fix:**
1. Step 1
2. Step 2

**Prevention:** [How to avoid this in the future]

---

## Issue: ...
```

---

## 8. RISKS AND MITIGATIONS

Monorepo splits are notoriously risky. This section catalogs the major risks and how to prevent them.

### Risk: Premature Split Causes Contributor Chaos

**Problem:** If you split the monorepo before v1.0 is stable, contributors will be confused. They won't know which repo to open issues in, which repo to clone, or how the repos depend on each other. Onboarding time increases. Bugs get reported in the wrong place.

**Mitigation:** 
- **Wait for v1.0 stable API.** Don't split before the daemon and protocol APIs are backwards-compatible.
- **Complete contributor docs first.** Ensure CONTRIBUTING.md, README, and architecture docs are clear.
- **Test the onboarding flow.** Before splitting, have new contributors follow the "clone X repo, build, run tests" flow. Fix friction points.
- **Communicate a migration guide.** When splitting happens, publish a "Where did my favorite code go?" guide.

### Risk: Circular Dependencies Between Repos

**Problem:** If core imports routing, and routing imports core, you have a cycle. Go modules will refuse to build. Even worse, you might have a subtle cycle (core → sdk → routing → core) that only shows up in integration tests.

**Mitigation:**
- **Draw a dependency diagram before splitting.** Make sure all edges point in one direction.
- **Enforce layering rules.** Use go-import-boss or similar tooling to prevent upward imports.
- **CI enforcement:** Add a check to GitHub Actions: `go-import-boss .` must pass before merge.
- **Code review discipline:** Reviewers must watch for new cross-repo imports and question them.

### Risk: GitHub Wiki Becomes Stale

**Problem:** If the wiki is separate from code, it rots. Protocol changes without wiki updates. New API without docs. Contributors don't know to update wiki after code changes.

**Mitigation:**
- **Docs-as-code:** Keep markdown in the git repo, not the wiki. Use git subtree to sync to GitHub wiki.
- **Wiki reviews:** Changes to docs/ must be reviewed (via PR) before syncing to wiki.
- **Automation:** CI can check for code comments that reference wiki pages and verify they exist.
- **Incentivize:** Code review checklist: "Does this PR require a wiki update? If yes, include it."

### Risk: Search Becomes Fragmented

**Problem:** With nine repos, GitHub's code search no longer finds all references. IDEs with multiple git remotes get confused.

**Mitigation:**
- **GitHub code search works across org repos.** Use `org:unheaded` in GitHub search to find code across all repos.
- **Monorepo for local development (optional).** Advanced developers can use git subtrees or workspace tools to clone all nine repos locally.
- **Linters at publish time:** Before releasing a new version, run a linter that validates all doc links and references are correct.

### Risk: Release Coordination Becomes Complex

**Problem:** If core v0.2.0 requires routing v0.2.0, but routing is released before core, users get version mismatches. Or changelog becomes fragmented across nine repos.

**Mitigation:**
- **Release train:** Establish a sequential release order (core → sdk → routing → firewall → iac).
- **Consolidated changelog:** The unheaded/docs repo maintains a master CHANGELOG.md that links to per-repo changelogs.
- **Semantic versioning:** All repos must follow semver strictly. Major version bumps are coordinated across repos.
- **Renovate or Dependabot:** Automate dependency updates across repos to catch mismatches.

---

## 9. TIMELINE: ROUGH MILESTONES

This timeline is deliberately loose. The actual split will take longer if teams are context-switching or if unexpected blocker issues arise.

### T+0: Public Release (Current)
- Unheaded v1.0 ships
- Monorepo tagged and stable
- Official documentation published

### T+1 to T+3 months: Stabilization Phase
- Gather feedback from early adopters
- Fix critical bugs
- Finalize API contracts (daemon gRPC/REST, plugin interfaces, eBPF maps)
- Bring test coverage to 80%+
- Document all subsystems

### T+3 months: Readiness Review
- Steering council meets to evaluate split readiness
- Technical debt audit (are there any architectural dependencies that need cleanup?)
- Team capacity check (do we have DevOps capacity to manage 9 repos?)

### T+3 to T+6 months: Extract Low-Risk Repos
- Extract unheaded/protocol (no Go code, no dependencies)
- Extract unheaded/docs (central hub for wiki and docs)
- Test extraction process; iterate on tooling
- Update monorepo to point to extracted repos

### T+6 to T+9 months: Extract Subsystem Repos
- Extract unheaded/ebpf (lowest Go dependencies)
- Extract unheaded/iac (no Go dependencies)
- Extract unheaded/security (minimal Go code)
- Extract unheaded/routing (depends on core)
- Extract unheaded/firewall (depends on core)

### T+9 to T+10 months: Integration and Testing
- Update core to import from extracted repos
- Test all integrations end-to-end (daemon starts, all subsystems work)
- Verify CI/CD pipelines work per-repo and cross-repo
- Update documentation and onboarding guides

### T+10 to T+12 months: Sunset and Archive
- Final monorepo release (v1.0-final-monorepo)
- Archive monorepo as read-only
- Migrate all issues and PRs to new repos
- Publish migration guide for contributors
- Update main GitHub org README to point to new repos

### T+12+ months: Post-Split Stabilization
- Monitor for dependency issues, CI flakes, contributor confusion
- Iterate on workflows, documentation, and tooling
- Consider extraction of unheaded/sdk once ecosystem stabilizes

---

## 10. TOOLS AND AUTOMATION

### git-filter-repo (Repository Extraction)

The gold standard for monorepo splitting. Unlike git filter-branch, it's fast and handles large histories well.

```bash
pip install git-filter-repo

# Usage:
git clone --bare https://github.com/unheaded/unheaded unheaded-mono.git
cd unheaded-mono.git
git filter-repo --path docs/protocol --path proto
# Then push to new repo
```

**Advantages:** Preserves author metadata, handles renames well, deterministic output.

### go-import-boss (Import Validation)

Enforces import layering rules in Go. Prevents upward imports and cycles.

```bash
go install github.com/shurcooL/go-import-boss@latest
go-import-boss ./...
# Fails if any package imports from layers above it
```

### GitHub Wiki Sync (git subtree)

Synchronizes markdown documentation from git repo to GitHub wiki.

```bash
# Pull latest wiki
git subtree pull --prefix wiki origin wiki --squash

# Edit wiki files
vim wiki/Home.md

# Push to wiki
git subtree push --prefix wiki origin wiki --squash
```

### Renovate or Dependabot (Dependency Updates)

Automatically updates Go module dependencies across repos.

```yaml
# renovate.json
{
  "extends": ["config:base"],
  "goModTidy": true,
  "prConcurrentLimit": 3
}
```

### GitHub CLI (Repository and Org Management)

Automate creation of new repos, branch protection, and access rules.

```bash
gh repo create unheaded/core --public --template unheaded/repo-template
gh repo edit unheaded/core --default-branch main
gh api repos/unheaded/core/branches/main/protection -f required_status_checks='{contexts:["GitHub Actions"]}'
```

---

## SUMMARY: STRATEGIC PRINCIPLES

The split of Unheaded into nine specialized repositories is a **long-term structural change** that enables growth and community contribution. It should not be rushed. The following principles guide the effort:

1. **Wait for stability.** Don't split until v1.0 is public and stable. Premature splitting causes chaos.

2. **Plan the dependency graph.** Ensure all edges point downward (core at the bottom). No cycles.

3. **Docs-as-code.** Keep wikis in git; automate sync to GitHub. Prevents stale documentation.

4. **Per-repo CI.** Each repo has its own test suite and release cadence. No monorepo-wide CI bottleneck.

5. **Clear ownership.** Each repo has a clear CODEOWNERS and team. Autonomy and accountability.

6. **Conservative extraction.** Start with low-risk repos (protocol, docs), then move to subsystems. Iterate on tooling.

7. **Test integrations.** After extraction, verify all subsystems still work together end-to-end. No surprises in production.

This plan positions Unheaded for sustainable growth without sacrificing the cohesion and clarity that a well-organized monorepo provides today.

---

**Document Version:** 1.0  
**Last Updated:** 2026-02-26  
**Owner:** Architecture and Steering Council  
**Status:** Strategic Planning — Do Not Implement Now
