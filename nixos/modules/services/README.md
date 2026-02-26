# Unheaded Kingdom Services Module

This directory contains 25 NixOS systemd service modules for the Unheaded Kingdom project.

## Services by Layer

### Layer 1: Core Services (5 services)
- `monad.nix` - Monad register (20-byte IPv6 HbH extension header) [gRPC :50051]
- `sophia.nix` - Sophia BPF dictionary maps [gRPC :50052]
- `wotan.nix` - Wotan message bus & event broker (NATS) [gRPC :50053, NATS :4222]
- `anamnesis.nix` - Anamnesis event log & stream [gRPC :50054]
- `shield.nix` - Shield eBPF policy enforcement [gRPC :50055]

### Layer 2: Control Plane (1 service)
- `unheaded-daemon.nix` - Unheaded daemon Cuirass control plane reconciler [internal]

### Layer 3: Frontend & Dashboard (2 services)
- `dashboard-backend.nix` - Dashboard backend Cape & Cloak [HTTP :8080]
- `dashboard-frontend.nix` - Dashboard frontend UI static serve [HTTP :3001]

### Layer 4: Protocol & API (3 services)
- `protocol-api.nix` - Protocol API wire format validation & Gauntlets [gRPC :50056]
- `trace-collector.nix` - Trace collector Zipkin-compatible [HTTP :9411]
- `gateway.nix` - Gateway API gateway & ingress [HTTP :8443]

### Layer 5: Infrastructure Services (2 services)
- `service-discovery.nix` - Service discovery registry & Greaves [gRPC :50057, HTTP :8500]
- `doom.nix` - DOOM BPF compute substrate [TCP :16680]

### Layer 6: Red Team & Security (1 service)
- `lich.nix` - Lich automated adversary & red team continuous [internal]

### Layer 7: Strategy & Leadership (5 services)
- `captain.nix` - Captain executive strategy [gRPC :50058]
- `micromanager.nix` - Micromanager engineering leadership [gRPC :50059]
- `timeguru.nix` - Timeguru timeline & roadmap [gRPC :50060, HTTP :8600]
- `architect.nix` - Architect infrastructure architecture [gRPC :50061]
- `developer.nix` - Developer development assistance [gRPC :50062]

### Layer 8: Coordination & Organization (4 services)
- `kanban.nix` - Kanban board UI & API [HTTP :3002]
- `lore.nix` - Lore naming & mythology [gRPC :50063]
- `busboy.nix` - Busboy coordinator & glue [gRPC :50064]
- `kingdom.nix` - Kingdom hierarchy reference [gRPC :50065]

### Layer 9: Security & Compliance (2 services)
- `blackmage.nix` - Blackmage security & offensive [gRPC :50066]
- `moatghost.nix` - Moatghost compliance & audit [gRPC :50067]

## Usage

Import this module in your NixOS configuration:

```nix
imports = [ ./modules/services ];
```

Then enable individual services:

```nix
services.unheaded.monad.enable = true;
services.unheaded.sophia.enable = true;
services.unheaded.wotan.enable = true;
# ... etc for other services
```

## Port Allocation Summary

### gRPC Ports (Port Range 50051-50067)
- 50051: monad
- 50052: sophia
- 50053: wotan
- 50054: anamnesis
- 50055: shield
- 50056: protocol-api
- 50057: service-discovery
- 50058: captain
- 50059: micromanager
- 50060: timeguru
- 50061: architect
- 50062: developer
- 50063: lore
- 50064: busboy
- 50065: kingdom
- 50066: blackmage
- 50067: moatghost

### HTTP/HTTPS Ports
- 3001: dashboard-frontend
- 3002: kanban
- 8080: dashboard-backend
- 8443: gateway
- 8500: service-discovery (Consul-compatible)
- 8600: timeguru
- 9411: trace-collector (Zipkin-compatible)

### Special Ports
- 4222: wotan (NATS)
- 16680: doom (TCP compute substrate)

## Security Features

### Common Across All Services
- Dedicated `unheaded` user/group
- NoNewPrivileges = true (except eBPF services)
- PrivateTmp = true
- ProtectSystem = "strict"
- ProtectHome = true
- Strict ReadWritePaths enforcement
- Capability bounding set enforcement

### eBPF Services (Special)
- **shield.nix**: CAP_BPF, CAP_NET_ADMIN, CAP_SYS_ADMIN, CAP_NET_BIND_SERVICE, CAP_SYS_RESOURCE
- **unheaded-daemon.nix**: CAP_BPF, CAP_NET_ADMIN, CAP_SYS_ADMIN, CAP_SYS_RESOURCE
- Access to /sys/kernel/debug and /sys/kernel/security
- NoNewPrivileges = false

### Network Services (Special)
- **lich.nix**: CAP_NET_RAW, CAP_NET_ADMIN, CAP_SYS_ADMIN
- 2GB memory allocation
- 400% CPU quota

## Resource Allocation Strategy

### Minimal (256M memory, 100% CPU)
- dashboard-frontend (static file serving)
- lore (naming lookups)

### Small (512M memory, 200% CPU)
- Most gRPC services: monad, protocol-api, gateway, etc.

### Medium (768M memory, 200-300% CPU)
- Sophia (BPF dictionary operations)
- Developer (development assistance)
- Blackmage (security operations)
- Trace-collector (trace processing)

### Large (1G memory, 200-300% CPU)
- Wotan (message bus, highest throughput)
- Anamnesis (event log streaming)
- Unheaded-daemon (control plane)
- DOOM (compute substrate)
- Gateway (ingress processing)

### Extra Large (2G memory, 400% CPU)
- Lich (automated adversary/red team)

## Systemd Service Names

All services follow the naming convention: `unheaded-<servicename>`

Examples:
- `systemctl status unheaded-monad`
- `systemctl start unheaded-wotan`
- `systemctl logs unheaded-shield`

## Environment Variables

Each service exports environment variables prefixed with its service name in uppercase:

Examples from services:
- MONAD_PORT, MONAD_LOG_LEVEL, WOTAN_ADDR
- SOPHIA_PORT, SOPHIA_LOG_LEVEL
- WOTAN_GRPC_PORT, WOTAN_NATS_PORT, WOTAN_LOG_LEVEL
- SHIELD_PORT, SHIELD_LOG_LEVEL
- DASHBOARD_HTTP_PORT, DASHBOARD_LOG_LEVEL
- etc.

## State Directories

Each service has a dedicated state directory:
- `/var/lib/unheaded/<servicename>/` (owned by unheaded:unheaded, mode 0750)
- `/var/log/unheaded/` (shared, owned by unheaded:unheaded, mode 0750)

Created and managed by systemd.tmpfiles.rules

## License

All files: SPDX-License-Identifier: MIT
Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

## Architecture Notes

The 25 services span 9 distinct armor layers:
1. Core networking and low-level services
2. Control plane orchestration
3. User-facing dashboard components
4. Protocol validation and API gateway
5. Infrastructure coordination
6. Offensive security testing
7. Strategic decision-making services
8. Organizational coordination
9. Security, compliance, and audit

This modular approach allows selective deployment and independent scaling of service groups.
