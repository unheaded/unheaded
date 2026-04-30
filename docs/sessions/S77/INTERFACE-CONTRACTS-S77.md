# Interface Contracts -- S77: IaC + Observability

**Sprint:** S77 Phase 5
**Date:** 2026-03-05
**Status:** Complete

---

## IaC Renderer Interface

### Location

`pkg/iac/iac.go`

### Interface Definition

```go
type Renderer interface {
    Backend() Backend
    Render(config ServiceConfig) (*RenderOutput, error)
    RenderAll(configs []ServiceConfig) (*RenderOutput, error)
    Validate(config ServiceConfig) error
    Diff(current, desired ServiceConfig) (string, error)
}
```

### Methods

| Method | Purpose | Returns |
|--------|---------|---------|
| `Backend()` | Returns the backend identifier (e.g. `"ansible"`, `"terraform"`) | `Backend` |
| `Render(config)` | Produces IaC artifacts for a single service | `*RenderOutput, error` |
| `RenderAll(configs)` | Produces IaC artifacts for multiple services with shared orchestration files | `*RenderOutput, error` |
| `Validate(config)` | Renders then structurally validates the output (required fields, syntax) | `error` |
| `Diff(current, desired)` | Compares two configs and returns a human-readable change summary | `string, error` |

### Usage Pattern

```go
// 1. Create a renderer via the factory
renderer, err := iac.NewRenderer(iac.BackendAnsible)

// 2. Define the desired state
svc := iac.DefaultServiceConfig("timeguru", 19000)
svc.GRPCPort = 19001
svc.Image = "unheaded/timeguru:v0.1.0"

// 3. Validate before rendering
if err := renderer.Validate(svc); err != nil {
    log.Fatalf("invalid config: %v", err)
}

// 4. Render IaC artifacts
output, err := renderer.Render(svc)
// output.Files is a map[string]string of filepath -> content

// 5. Diff two configs to preview changes
diff, err := renderer.Diff(currentCfg, desiredCfg)
if diff != "" {
    fmt.Println(diff)
}
```

### Current Implementations

| Backend | File | Output Artifacts |
|---------|------|------------------|
| **Ansible** | `pkg/iac/ansible.go` | Playbooks (`site.yml`), roles (tasks/defaults/handlers), systemd templates, inventory |
| **Terraform** | `pkg/iac/terraform.go` | HCL modules (`main.tf`, `variables.tf`, `outputs.tf`), root module, versions |
| **Kubernetes** | `pkg/iac/kubernetes.go` | Deployments, Services, NetworkPolicies, Namespace, Kustomization |
| **Puppet** | `pkg/iac/puppet.go` | Manifests (`init.pp`, `config.pp`), Hiera data, site manifest |
| **Chef** | `pkg/iac/chef.go` | Cookbooks (recipes, attributes, metadata), Policyfile |
| **Salt** | `pkg/iac/salt.go` | States (`init.sls`), pillars, top files |

### Adding a New Backend

1. Create `pkg/iac/newbackend.go`
2. Define a struct implementing `Renderer`
3. Add a `Backend` constant in `iac.go`
4. Register in `NewRenderer()` factory switch
5. Add to `AllBackends()` slice
6. Add backend-specific validation in `pkg/iac/validate.go`
7. Write tests in `pkg/iac/iac_test.go` or a dedicated test file

### Validation

`pkg/iac/validate.go` provides:

- `ValidateRenderedOutput(output, config)` -- structural validation (non-empty files, backend-specific checks)
- `DiffConfigs(current, desired)` -- human-readable config comparison

Each backend's `Validate()` method calls `ValidateRenderedOutput` after rendering, ensuring the output is structurally sound.

---

## Observability Adapter Interface

### Location

`pkg/observability/observability.go`

### Interface Definition

```go
type Adapter interface {
    Backend() Backend
    Supports() []SignalType
    EmitMetric(ctx context.Context, m Metric) error
    EmitLog(ctx context.Context, e LogEntry) error
    EmitTrace(ctx context.Context, s Span) error
    EmitAlert(ctx context.Context, a Alert) error
    Close() error
}
```

### Signal Types

| Signal | Type Struct | Description |
|--------|------------|-------------|
| `SignalMetric` | `Metric` | Counter, gauge, histogram, summary data points |
| `SignalLog` | `LogEntry` | Structured log events with level, service, trace_id |
| `SignalTrace` | `Span` | Distributed trace spans with parent/child relationships |
| `SignalAlert` | `Alert` | Alert events with severity (info/warning/error/critical) |

### Pipeline Fan-Out

The `Pipeline` struct fans out signals to multiple adapters simultaneously:

```go
// Create adapters
prom := observability.NewPrometheusAdapter()
elk := observability.NewELKAdapter("unheaded-logs", 10000)
jaeger := observability.NewJaegerAdapter(5000)

// Create pipeline -- signals fan out to all capable adapters
pipeline := observability.NewPipeline(prom, elk, jaeger)

// Metric goes to Prometheus (only adapter supporting SignalMetric)
pipeline.EmitMetric(ctx, observability.Metric{
    Name:   "http_requests_total",
    Value:  1,
    Labels: map[string]string{"service": "timeguru"},
    Type:   observability.MetricCounter,
})

// Log goes to ELK (supports SignalLog)
pipeline.EmitLog(ctx, observability.LogEntry{
    Level:   "info",
    Message: "request processed",
    Service: "timeguru",
    TraceID: "trace-abc",
})

// Trace goes to Jaeger (supports SignalTrace)
pipeline.EmitTrace(ctx, observability.Span{
    TraceID:   "trace-abc",
    SpanID:    "span-001",
    Operation: "HTTP GET /api/v1/timeline",
    Service:   "timeguru",
    Duration:  5 * time.Millisecond,
    Status:    observability.SpanOK,
})

// Dynamic adapter addition
nagios := observability.NewNagiosAdapter()
pipeline.AddAdapter(nagios)

// Cleanup
pipeline.Close()
```

### Current Implementations

| Backend | File | Signals Supported | Description |
|---------|------|-------------------|-------------|
| **Prometheus** | `prometheus.go` | Metric | Exposition format, counter accumulation, gauge last-value |
| **Zerolog** | `zerolog.go` | Log | Structured JSON log buffering with ring buffer |
| **ELK** | `elk.go` | Log | Elasticsearch bulk API, Logstash pipeline, Kibana index patterns |
| **Grafana** | `grafana.go` | Metric, Alert | Dashboard generation, alert annotations |
| **Fluentd** | `fluentd.go` | Log | Tagged event forwarding, Fluent Bit config generation |
| **Jaeger** | `jaeger.go` | Trace | Span collection, collector payload, parent-child references |
| **Nagios** | `nagios.go` | Metric, Alert | Check results, NSCA payload, service definitions |
| **Loki** | `loki.go` | Log | Stream-based log aggregation, push API payload |

### Concurrency

- `Pipeline` is safe for concurrent use (protected by `sync.RWMutex`)
- Individual adapters use `sync.RWMutex` or `sync/atomic` for thread safety
- `AddAdapter` can be called while other goroutines emit signals
- `Close()` prevents further emissions and flushes all adapters

### Adding a New Adapter

1. Create `pkg/observability/newadapter.go`
2. Implement all `Adapter` interface methods
3. Return appropriate `Backend` constant
4. Declare supported `SignalType` values in `Supports()`
5. For unsupported signals, return `nil` (no-op)
6. Use `sync/atomic.Bool` for closed state tracking
7. Add tests in `pkg/observability/adapters_test.go`

---

## Test Coverage

### IaC Tests

| File | Coverage |
|------|----------|
| `pkg/iac/iac_test.go` | ServiceConfig validation, defaults, all-backend render, content checks |
| `pkg/iac/integration_test.go` | Multi-service RenderAll, Validate, Diff, factory pattern, ValidateRenderedOutput edge cases |

### Observability Tests

| File | Coverage |
|------|----------|
| `pkg/observability/observability_test.go` | Prometheus metrics, zerolog logs, pipeline fan-out, close, concurrent safety, timestamp defaults |
| `pkg/observability/adapters_test.go` | Grafana, ELK, Fluentd, Jaeger, Nagios, Loki -- individual adapter tests + full-stack pipeline |
| `pkg/observability/integration_test.go` | Prometheus+ELK fan-out, multi-adapter log fan-out, concurrent metrics/logs/traces, close behavior, dynamic adapter addition |

---

## Key Design Decisions

1. **Fan-out, not fan-in:** Pipeline distributes signals to all capable adapters. Each adapter decides what it supports via `Supports()`.

2. **Backend-agnostic desired state:** `ServiceConfig` is the single source of truth. All IaC renderers consume the same model.

3. **Validate-then-render:** The `Validate` method renders internally and checks the output, ensuring structural validity before deployment.

4. **Diff for drift detection:** `Diff` compares two configs without touching real infrastructure, enabling dry-run workflows.

5. **Ring buffers everywhere:** Adapters that buffer data (ELK, Loki, Zerolog, Jaeger) use configurable ring buffers to prevent unbounded memory growth.

6. **Graceful degradation:** Unsupported signal types return `nil` (no-op), not errors. This allows heterogeneous adapter sets without error handling overhead.
