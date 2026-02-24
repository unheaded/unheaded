# Repo Separation Targets

**Status**: Mono-repo NOW. Separate when GitHub org has multiple contributors.
**Decision Date**: February 18, 2026 (Protocol Awakening session)

---

## Current: Mono-repo (`unheaded/`)

Everything lives in a single repo. This is correct for a solo/small-team phase:
- Single `go.mod`, single CI pipeline, single `git log`
- No cross-repo dependency management overhead
- Fast iteration, atomic commits across layers

## Future: Multi-repo (GitHub org `unheaded-kingdom/`)

When the team grows and contributors specialize, split along layer boundaries:

| Repo | Layer | Contents | Language |
|------|-------|----------|----------|
| `unheaded/protocol` | 0 | RFC, wire format spec, reference implementation, Internet-Draft | Markdown, ABNF |
| `unheaded/void` | 1 | eBPF programs (shield, hop, yaldabaoth, monad-cpu, observability) | Rust/aya-rs |
| `unheaded/wotan` | 2 | Central Core (ring buffer reader, BPF map writer, gRPC bridge) | Go + Rust |
| `unheaded/kingdom` | 3 | Go services, dashboard, Kanban, CLI, gateway | Go + HTML/CSS/JS |
| `unheaded/sophia` | cross | Dictionary management tools, exponent compiler, hot-swap tooling | Go + Rust |

### Separation Criteria (when to split)

- 3+ regular contributors working on different layers
- CI pipeline exceeds 15 minutes due to full workspace builds
- Release cadence differs between layers (eBPF ships weekly, Kingdom ships daily)
- External consumers need to import a specific layer without the full monolith

### Shared Dependencies (post-split)

- `monad-common` crate → published to crates.io or vendored as git submodule
- `unheaded-common` Go package → published module or git submodule
- Proto definitions → separate `unheaded/proto` repo or buf.build registry
- Nix flake → either unified flake with inputs or per-repo flakes

### Migration Steps (when ready)

1. `git subtree split` for each target directory
2. Set up cross-repo CI triggers (void build triggers wotan integration test)
3. Publish `monad-common` to crates.io
4. Set up buf.build for proto definitions
5. Update Makefile to support `make pull-deps` for local development

---

*Note: "busboy" was renamed to "wotan" on February 18, 2026. The skill remains `unheaded-busboy` (the vibe, the coordinator). The codebase uses `wotan` (the true name, the all-father).*
