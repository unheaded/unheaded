# IaC Backends — The Forge of Many Tongues

Unheaded generates configuration artifacts for the customer's preferred toolchain. The control plane maintains a single desired-state model; IaC backends are interchangeable output renderers.

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
Desired State (Git)
       │
       ▼
  pkg/iac/renderer.go   ◄── IaCRenderer interface
       │
       ├── ansible/      → playbook.yml, roles/, inventory/
       ├── terraform/    → main.tf, variables.tf, modules/
       ├── puppet/       → manifests/, hiera/, modules/
       ├── kubernetes/   → manifests/, charts/, operators/
       ├── chef/         → cookbooks/, recipes/, data_bags/
       └── salt/         → states/, pillars/, grains/
```

The IaC layer consumes the same core packages (`pkg/`) and generates output in the customer's dialect. Adding a new backend is writing an output renderer — the control plane, eBPF layer, and Wotan integration don't change.

## CLI Usage

```bash
# Generate Ansible playbooks from current desired state
unheaded generate --backend=ansible --output=./deploy/ansible/

# Generate Terraform modules
unheaded generate --backend=terraform --output=./deploy/terraform/

# Generate Kubernetes manifests + Helm charts
unheaded generate --backend=k8s --output=./deploy/k8s/

# Generate for all backends at once
unheaded generate --backend=all --output=./deploy/
```

## Design Principles

Each renderer is a **pure function**: desired state in, config artifacts out. No side effects, no network calls, no state mutation. This means renderers are trivially testable — feed in a desired state, assert the output is valid and lintable for that tool.

## Status

Epoch 1.4b (The Forge of Many Tongues) — scaffolded, `IaCRenderer` interface defined. Full implementation planned for Age 2 (Beta).

---

> **Source:** [CLAUDE.md](../CLAUDE.md) · [references/timeline.md](../references/timeline.md)
