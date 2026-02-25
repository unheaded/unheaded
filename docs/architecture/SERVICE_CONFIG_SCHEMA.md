# Service Configuration Schema

## Overview

Every Unheaded service is configured via a YAML file located at `/opt/unheaded/<service>/config.yaml`. This document defines the complete schema, field specifications, validation rules, and examples.

## Root-Level Fields

### `service` (required)
Contains service identity and metadata.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | — | Unique service identifier (lowercase, alphanumeric + hyphens). Used as directory name and service identifier. |
| `display_name` | string | no | `name` (title-cased) | Human-readable service name for UI display. |
| `description` | string | no | "" | Brief description of service purpose and responsibilities. |
| `version` | string | no | "0.1.0" | Semantic version of this service's API/behavior. |

**Example:**
```yaml
service:
  name: wotan
  display_name: "Wotan"
  description: "Per-flow memory model and ephemeral ring buffer for state management"
  version: "0.2.1"
```

### `network` (required)
Defines service networking, ports, and communication protocols.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `port` | int | yes | — | Primary service port (gRPC if protocol is grpc, else HTTP). Must be in port tier range (see Port Tiers section). |
| `http_port` | int | no | `port + 100` | HTTP fallback or secondary HTTP port. Used for health checks, metrics, or HTTP bridges. |
| `protocol` | string | no | "grpc" | Communication protocol: `grpc`, `http`, or `both`. Determines how clients communicate with this service. |
| `health_endpoint` | string | no | "/health" | HTTP endpoint for health checks. Responds with 200 OK if healthy. |
| `metrics_endpoint` | string | no | "/metrics" | HTTP endpoint for Prometheus-format metrics. Must be accessible via `http_port`. |

**Allowed protocol values:**
- `grpc`: Service uses gRPC only
- `http`: Service uses HTTP/REST only
- `both`: Service supports both gRPC and HTTP

**Example:**
```yaml
network:
  port: 18001
  http_port: 18101
  protocol: grpc
  health_endpoint: /health
  metrics_endpoint: /metrics
```

### `deployment` (required)
Configures how the service is deployed and managed.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tier` | string | yes | — | Deployment tier: `application`, `infrastructure`, `control`, or `presentation`. Determines resource constraints and scheduling. |
| `replicas` | int | no | 1 | Desired number of running instances. Control plane services typically use 1 or 3 (for HA). |
| `restart_policy` | string | no | "always" | Restart behavior: `always`, `on-failure`, or `never`. |
| `depends_on` | list[string] | no | [] | List of service names that must be healthy before this service starts. |

**Allowed tier values:**
- `control`: Control plane services (port 17000–17999). Single instance, critical to platform.
- `infrastructure`: Core infrastructure services (port 18000–18999). Highly available, mesh-integrated.
- `presentation`: Frontend and user-facing services (port 19000–19999). Multiple replicas, stateless.
- `application`: User workload services (port 20000–20999). Customer applications running on Unheaded.

**Allowed restart_policy values:**
- `always`: Service restarts automatically on any exit (recommended for production).
- `on-failure`: Service restarts only on non-zero exit code.
- `never`: Service is not automatically restarted.

**Example:**
```yaml
deployment:
  tier: infrastructure
  replicas: 2
  restart_policy: always
  depends_on:
    - service-discovery
    - log-aggregator
```

### `runtime` (required)
Specifies how the service binary is executed.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `binary` | string | yes | — | Name of executable in PATH or absolute path to binary. Resolved during service startup. |
| `args` | list[string] | no | [] | Command-line arguments passed to the binary at startup. |
| `env` | map[string]string | no | {} | Environment variables set for the process. Merged with system environment. |

**Example:**
```yaml
runtime:
  binary: wotan
  args:
    - "--config"
    - "/opt/unheaded/wotan/config.yaml"
    - "--log-format=json"
  env:
    LOG_LEVEL: info
    WOTAN_RING_SIZE: "1024"
    GOMAXPROCS: "4"
```

### `health` (optional)
Configures service health checking and monitoring.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `check_interval` | string | no | "10s" | Time between health checks (Go duration format: "10s", "1m", etc.). |
| `timeout` | string | no | "5s" | Max time to wait for a health check response before timing out. |
| `retries` | int | no | 3 | Number of consecutive failed checks before marking service as unhealthy. |

**Example:**
```yaml
health:
  check_interval: 10s
  timeout: 5s
  retries: 3
```

### `labels` (optional)
Arbitrary key-value labels for categorization, monitoring, and organizational purposes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `kingdom_tier` | string | no | — | Unheaded armor metaphor tier: `armory`, `gnostic`, `royal-court`, or `presentation`. |
| `armor_piece` | string | no | — | Which armor piece this service represents (e.g., "wotan", "helm", "boots"). |

**Allowed kingdom_tier values (Unheaded metaphor):**
- `armory`: Core runtime services (binary, build, package management)
- `gnostic`: Knowledge and state services (discovery, metrics, logging, observability)
- `royal-court`: Command and coordination services (orchestration, scheduling, networking)
- `presentation`: User-facing and application services (dashboard, APIs, load balancers)

**Example:**
```yaml
labels:
  kingdom_tier: gnostic
  armor_piece: wotan
  team: platform
  env: production
```

---

## Complete Schema Reference

```yaml
service:
  name: string                    # REQUIRED
  display_name: string            # optional
  description: string             # optional
  version: string                 # optional

network:                          # REQUIRED
  port: integer                   # REQUIRED
  http_port: integer              # optional
  protocol: string                # optional (default: "grpc")
  health_endpoint: string         # optional (default: "/health")
  metrics_endpoint: string        # optional (default: "/metrics")

deployment:                       # REQUIRED
  tier: string                    # REQUIRED
  replicas: integer               # optional (default: 1)
  restart_policy: string          # optional (default: "always")
  depends_on:                     # optional
    - string

runtime:                          # REQUIRED
  binary: string                  # REQUIRED
  args:                           # optional
    - string
  env:                            # optional
    KEY: value

health:                           # optional
  check_interval: string          # optional (default: "10s")
  timeout: string                 # optional (default: "5s")
  retries: integer                # optional (default: 3)

labels:                           # optional
  kingdom_tier: string            # optional
  armor_piece: string             # optional
  # arbitrary additional labels
```

---

## Validation Rules

1. **service.name**: Must match regex `^[a-z][a-z0-9-]*$` (lowercase alphanumeric and hyphens, starts with letter).
2. **network.port**: Must be integer in range [1024, 65535] and match service's tier port range.
3. **deployment.tier**: Must be one of: `control`, `infrastructure`, `presentation`, `application`.
4. **deployment.restart_policy**: Must be one of: `always`, `on-failure`, `never`.
5. **health.check_interval**, **health.timeout**: Must be valid Go duration strings.
6. **labels.kingdom_tier**: If specified, must be one of: `armory`, `gnostic`, `royal-court`, `presentation`.

---

## Service Examples

### Example 1: Wotan (gRPC Infrastructure Service)

```yaml
service:
  name: wotan
  display_name: "Wotan"
  description: "Per-flow memory model and ephemeral ring buffer for stateful processing"
  version: "0.2.1"

network:
  port: 18001
  http_port: 18101
  protocol: grpc
  health_endpoint: /health
  metrics_endpoint: /metrics

deployment:
  tier: infrastructure
  replicas: 2
  restart_policy: always
  depends_on:
    - service-discovery
    - log-aggregator

runtime:
  binary: wotan
  args:
    - "--config=/opt/unheaded/wotan/config.yaml"
    - "--log-format=json"
  env:
    LOG_LEVEL: info
    WOTAN_RING_SIZE: "1024"
    WOTAN_FLOW_TIMEOUT: "5m"
    GOMAXPROCS: "4"

health:
  check_interval: 10s
  timeout: 5s
  retries: 3

labels:
  kingdom_tier: gnostic
  armor_piece: wotan
  team: platform
  env: production
```

### Example 2: Dashboard Backend (HTTP Control Service)

```yaml
service:
  name: dashboard-backend
  display_name: "Dashboard Backend"
  description: "REST API serving Unheaded dashboard UI with service discovery and status APIs"
  version: "1.0.0"

network:
  port: 17200
  http_port: 17200
  protocol: http
  health_endpoint: /api/v1/health
  metrics_endpoint: /metrics

deployment:
  tier: control
  replicas: 1
  restart_policy: always
  depends_on:
    - service-discovery
    - log-aggregator
    - metrics-server

runtime:
  binary: dashboard-backend
  args:
    - "-config=/opt/unheaded/dashboard-backend/config.yaml"
    - "-port=17200"
  env:
    LOG_LEVEL: info
    DISCOVERY_DIR: "/opt/unheaded"
    ENABLE_CORS: "true"
    CORS_ORIGINS: "http://localhost:3000"

health:
  check_interval: 5s
  timeout: 3s
  retries: 2

labels:
  kingdom_tier: royal-court
  armor_piece: dashboard-backend
  team: platform
  env: production
```

### Example 3: Shield WAF (HTTP Presentation Service)

```yaml
service:
  name: shield-waf
  display_name: "Shield WAF"
  description: "Web Application Firewall protecting ingress traffic with ModSecurity and custom rules"
  version: "0.5.0"

network:
  port: 19001
  http_port: 19101
  protocol: both
  health_endpoint: /health
  metrics_endpoint: /metrics

deployment:
  tier: presentation
  replicas: 3
  restart_policy: always
  depends_on:
    - service-discovery
    - log-aggregator
    - shield-rules-store

runtime:
  binary: shield-waf
  args:
    - "--config=/opt/unheaded/shield-waf/config.yaml"
    - "--rules-dir=/etc/shield-waf/rules"
  env:
    LOG_LEVEL: warn
    SHIELD_BACKEND: pauldrons:19100
    SHIELD_TIMEOUT: "30s"
    GOMAXPROCS: "8"

health:
  check_interval: 15s
  timeout: 10s
  retries: 3

labels:
  kingdom_tier: presentation
  armor_piece: shield
  team: platform
  env: production
  security_tier: critical
```

---

## Port Tier Reference

Services are grouped into port ranges by deployment tier:

| Tier | Port Range | Count | Typical Services |
|------|-----------|-------|------------------|
| control | 17000–17999 | 1000 | dashboard-backend, control-api, metrics-server |
| infrastructure | 18000–18999 | 1000 | wotan, captain, micromanager, architect, service-discovery, log-aggregator |
| presentation | 19000–19999 | 1000 | shield-waf, pauldrons, kanban-app, ui-gateway |
| application | 20000–20999 | 1000 | user workloads, customer services |

---

## Loading and Validation

Services are loaded by the `discovery` package:

```go
import "github.com/unheaded/pkg/discovery"

// Load a single service config
cfg, err := discovery.LoadServiceConfig("/opt/unheaded/wotan/config.yaml")

// Load all services from directory
services, err := discovery.LoadServiceDirectory("/opt/unheaded")

// Validate a config
err := discovery.ValidateConfig(cfg)
```

For details, see `/pkg/discovery/yaml_loader.go` and `/pkg/discovery/config.go`.

---

## FAQ

**Q: Can I use HTTP for infrastructure services?**
A: Yes, but gRPC is recommended for internal services for performance. HTTP is better for integration with external systems.

**Q: What if `http_port` is not specified?**
A: Defaults to `port + 100`. You can override this explicitly if needed.

**Q: How are environment variables merged with the system environment?**
A: Service env vars override system env vars of the same name. Other system env vars are passed through unchanged.

**Q: Can a service depend on another service in a different tier?**
A: Yes, though cross-tier dependencies should be minimized. Service discovery ensures dependency ordering during startup.

**Q: What happens if a service fails health checks?**
A: The service is marked as `unhealthy` in the control plane. No automatic restart occurs; restart policy only applies to process exits.

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-24 | Initial schema documentation |
