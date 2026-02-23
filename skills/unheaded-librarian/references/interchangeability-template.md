# Interchangeability Documentation Template

Quick-reference template for documenting a new interchangeable backend system.
Copy and fill in the `[PLACEHOLDERS]`.

---

## 1. CLAUDE.md — Core Capabilities Line

```markdown
- ✅ Interchangeable [CATEGORY] ([Backend1], [Backend2], [Backend3], [Backend4])
```

## 2. CLAUDE.md — Technology Stack Row

```markdown
| [Category] | **[Backend1] / [Backend2] / [Backend3] / [Backend4]** | Interchangeable [description]; same [interface] baseline |
```

## 3. CLAUDE.md — Strategy Section

```markdown
### [Category] Strategy

Unheaded [emits/generates/provides] [what]. Customers plug in their preferred [category] stack
via interchangeable [adapters/renderers/runtimes]. Same pattern as [other systems] — your tools,
our [data model/state model/signal schema].

| Sub-category | Supported Backends | Unheaded Default (Future) |
|-------------|-------------------|--------------------------|
| **[Sub1]** | [Backend1], [Backend2] | Custom Wotan [sub1] |
| **[Sub2]** | [Backend3], [Backend4] | Custom Wotan [sub2] |
```

## 4. battle-plan.md — Architect Section

```markdown
**[Category] Architecture**: [1-2 sentences describing the interface → adapter pattern
and listing supported backends. Reference the `[Interface]` Go interface if applicable.]
```

## 5. references/timeline.md — Sacred Pillar

```markdown
### Sacred Pillar: "[Lore Name]"
[CATEGORY]: Your tools, our [data/state/signal] model. Interchangeable [adapters/renderers/runtimes].
[Backend1], [Backend2], [Backend3], [Backend4] — or our own custom versions (long-term roadmap).
```

## 6. references/timeline.md — Epoch

```markdown
#### Epoch X.Y[letter] — [Lore Name] ([Category])
| Backend | Output | Status |
|---------|--------|--------|
| [Backend1] | [output format] | Planned |
| [Backend2] | [output format] | Planned |
| [Backend3] | [output format] | Planned |
| [Backend4] | [output format] | Planned |

Tasks:
- [ ] Define `[Interface]` interface in Go
- [ ] Implement [Backend1] [adapter/renderer]
- [ ] Implement [Backend2] [adapter/renderer]
- [ ] Implement [Backend3] [adapter/renderer]
- [ ] Implement [Backend4] [adapter/renderer]
- [ ] CLI: `unheaded [command] --backend=[options]`
- [ ] Integration tests for all backends
- [ ] Documentation across all layers
```

## 7. docs/VISION.md — Core Capabilities

```markdown
- Interchangeable [CATEGORY] ([Backend1], [Backend2], [Backend3], [Backend4]) — your tools, our [model]
```

## 8. README.md — One-liner

```markdown
[Brief sentence about interchangeable [CATEGORY] — e.g., "Plug in your preferred [category]
stack or use our tailored defaults."]
```

## 9. wiki/[Category]-Backends.md — Full Page

```markdown
# [Category] — [Lore Subtitle]

[Summary of the interchangeability pattern and what this page covers.]

## Supported Backends

| Category | Drop-In Backends | Unheaded Default (Future) |
|----------|-----------------|--------------------------|
| **[Sub1]** | [Backend1], [Backend2] | Custom Wotan [sub1] |
| **[Sub2]** | [Backend3], [Backend4] | Custom Wotan [sub2] |

## How It Works

` ` `
[Data Source] → [Buffer/Bus] → [Interface]
                                    │
                                    ├── [backend1]/  → [output description]
                                    ├── [backend2]/  → [output description]
                                    ├── [backend3]/  → [output description]
                                    └── [backend4]/  → [output description]
` ` `

Each [adapter/renderer] is a [config generator/output renderer] — it consumes Unheaded's
[schema/model] and produces valid, working [configs/artifacts] for the target tool.

## CLI Usage

` ` `bash
# Generate [category] configs for specific backend
unheaded [command] --backend=[backend1],[backend2] --output=./[dir]/

# Generate all backends
unheaded [command] --backend=all --output=./[dir]/
` ` `

## Phased Roadmap

**Phase 1 — [Adapter/Config] Mirrors ([Alpha/Beta]):** Drop-in configs.
**Phase 2 — Wotan-Native Defaults ([Release]):** Custom implementations leveraging eBPF data plane.
**Phase 3 — Full Suite ([Scale]):** Purpose-built replacements.

## Licensing

All supported backends are open source. Unheaded adapter configs are MIT-licensed.

---

> **Source:** [CLAUDE.md](../CLAUDE.md) · [references/timeline.md](../references/timeline.md)
```

## 10. wiki/Home.md — Infrastructure Line

```markdown
- [[[Category]|[Category]-Backends]] — Interchangeable [description]. [Backend1], [Backend2], [Backend3], [Backend4]
```

## 11. wiki/_Sidebar.md — Navigation Entry

```markdown
- [[[Category]|[Category]-Backends]]
```

## 12. wiki/Architecture.md — Tech Stack + Strategy Section

Add row to tech stack table + brief strategy section matching CLAUDE.md.

---

## Existing Interchangeable Systems (for reference)

| System | Wiki Page | Interface Pattern | Backends |
|--------|-----------|------------------|----------|
| Containers | Containers.md | `RuntimeClient` | LXD, containerd, NixOS, Docker |
| IaC | IaC-Backends.md | `IaCRenderer` | Ansible, Terraform, Puppet, K8s, Chef, Salt |
| Observability | Observability-Backends.md | `ObservabilityAdapter` | Prometheus, Grafana, ELK, Fluentd, Jaeger, Nagios+ |
