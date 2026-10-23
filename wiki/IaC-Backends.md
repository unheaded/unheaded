# IaC Backends — The Forge of Many Tongues

Unheaded generates configuration artifacts for the user's preferred toolchain. The control plane maintains a single desired-state model; IaC backends are interchangeable output renderers.

## Supported Backends

| Backend | Output Format | Use Case |
|---------|--------------|----------|
| **Ansible** | Playbooks, roles, inventory | Agentless push-based config management |
| **Terraform** | HCL modules, providers, state | Cloud infrastructure provisioning |
| **Puppet** | Manifests, Hiera data, modules | Agent-based declarative configuration |
| **Kubernetes** | Manifests, Helm charts, operators | Container orchestration at scale |
| **Chef** | Cookbooks, recipes, data bags | Ruby-based configuration management |
| **Salt** | States, pillars, grains | Event-driven, high-speed configuration |

## How It Works

```
ServiceConfig (name, port, health/ready/metrics paths, …)
       │
       ▼
  pkg/iac/iac.go   ◄── Renderer interface: Render, RenderAll, Validate, Diff
       │
       ├── ansible.go     ├── terraform.go   ├── puppet.go
       ├── kubernetes.go  ├── chef.go        └── salt.go
```

Each backend is one renderer in `pkg/iac/`; `validate.go` checks output structure. Adding a backend is writing a renderer: the control plane, eBPF layer and Wotan integration don't change. The renderers take per-service configs, not the control plane's desired state; the CLI feeds them a fixed list of the ten core services and their Doom Range ports (`cmd/unheaded-cli/cmd/generate.go`).

## CLI Usage

```bash
unheaded generate list                                       # backends
unheaded generate iac --backend ansible                      # all core services -> ./generated/ansible/
unheaded generate iac --backend kubernetes --services wotan,timeguru --out ./deploy-out
unheaded generate iac --backend terraform --dry-run          # list files, write nothing
```

Backends: `ansible`, `terraform`, `puppet`, `kubernetes`, `chef`, `salt` (no `all`, no `k8s` alias).

## Design Principles

Each renderer is a **pure function**: desired state in, config artifacts out. No side effects, no network calls, no state mutation. This means renderers are trivially testable — feed in a desired state, assert the output is valid and lintable for that tool.

## Status

All six renderers exist with tests (`pkg/iac/iac_test.go`, `integration_test.go`). `remediation_ansible.go` also renders the compliance remediation model as Ansible (ADR-099).

---

> **Source:** [CLAUDE.md](../CLAUDE.md) · [references/timeline.md](../references/timeline.md)
