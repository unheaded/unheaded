# S77 — Phase 5: Interface Contracts

**Sprint:** S77 (Age 2 Acceleration)
**Phase:** 5 — Interchangeability Documentation
**Status:** Shipped — both Renderer + Adapter interfaces in-tree with all backends

---

## Why interfaces

The Kingdom is committed to anti-lock-in. Any user can choose any IaC backend (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt) and any observability backend (Prometheus, ELK, Jaeger, Loki, Fluentd, Grafana, Nagios). Two trait-shaped interfaces enforce drop-in interchangeability.

## IaC Renderer — `pkg/iac/iac.go`

```go
// Renderer interface emits configuration artifacts in a backend-specific
// format from a backend-agnostic ServiceConfig.
type Renderer interface {
    Render(config ServiceConfig) (RenderedOutput, error)
    RenderAll(configs []ServiceConfig) (map[string]RenderedOutput, error)
    Validate(config ServiceConfig) error
    Diff(current, desired ServiceConfig) (DiffReport, error)
}
```

### Backends

| Backend | File | Output |
|---------|------|--------|
| **Ansible** | `pkg/iac/ansible.go` | YAML playbooks, roles, inventory |
| **Terraform** | `pkg/iac/terraform.go` | HCL modules + providers + state |
| **Puppet** | `pkg/iac/puppet.go` | Manifests, Hiera data, modules |
| **Kubernetes** | `pkg/iac/kubernetes.go` | Manifests + Helm charts + operators |
| **Chef** | `pkg/iac/chef.go` | Cookbooks + recipes + data bags |
| **Salt** | `pkg/iac/salt.go` | States, pillars, grains |

### Validation + diff

`pkg/iac/validate.go` exports `ValidateRenderedOutput` (parse-and-lint per backend) and `DiffConfigs` (semantic diff for drift detection). All Renderer implementations call into these helpers so validation consistency is enforced.

### Tests

`pkg/iac/integration_test.go` smoke-tests every backend with the same fixture ServiceConfig and asserts `Render → Validate → Diff(current, current) == empty` for each.

## Observability Adapter — `pkg/observability/observability.go`

```go
// Adapter interface emits observability signals to a backend-specific store.
// One Pipeline composes multiple Adapters for fan-out.
type Adapter interface {
    EmitMetric(m Metric) error
    EmitLog(l LogEvent) error
    EmitTrace(t TraceSpan) error
    EmitAlert(a AlertEvent) error
    Pipeline() string
}
```

### Adapters

| Adapter | File | Backend |
|---------|------|---------|
| **prometheus** | `pkg/observability/prometheus.go` | Prometheus + Alertmanager |
| **ELK** | `pkg/observability/elk.go` | Elasticsearch + Logstash + Kibana |
| **Grafana** | `pkg/observability/grafana.go` | Grafana Cloud / OSS |
| **Fluentd** | `pkg/observability/fluentd.go` | Fluentd + Fluent Bit |
| **Jaeger** | `pkg/observability/jaeger.go` | Jaeger tracing |
| **Nagios** | `pkg/observability/nagios.go` | Nagios alerting |
| **Loki** | `pkg/observability/loki.go` | Grafana Loki logs |
| **zerolog** | `pkg/observability/zerolog.go` | Local stdout (default for development) |

### Tests

`pkg/observability/integration_test.go` verifies each adapter against a mock backend, plus the `Pipeline` composer for fan-out semantics.

## Selection at runtime

Both interfaces are selected via config:

```yaml
iac:
  renderer: ansible       # ansible | terraform | puppet | kubernetes | chef | salt

observability:
  adapters:
    - prometheus
    - jaeger
    - loki
```

Switching renderer / adding adapter is a single line edit. No code changes required.

## Interchangeability Documentation Pattern

This is the canonical reference for the Pattern (per Librarian skill — `unheaded-librarian`). Every plug-in architecture in the Kingdom (transport-cascade, authenticator, log-format, container-runtime, etc.) uses the same pattern: one Go interface, multiple backend impls, one integration test that runs the same fixture through every backend.

## References

- `pkg/iac/iac.go`, `pkg/iac/validate.go`, `pkg/iac/integration_test.go`.
- `pkg/observability/observability.go`, `pkg/observability/integration_test.go`.
- `tests/s77/s77_verification_test.go::TestPhase5_*`.
- CLAUDE.md "Backends" sections (containers, IaC, observability) — full backend matrix.
