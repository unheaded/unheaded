# Unheaded Container Test Suite

TDD test suite for NixOS container stack validation.

## Overview

Comprehensive testing of the Unheaded container orchestration layer:
- **Unit tests**: Configuration validation (IPs, ports, dependencies)
- **Integration tests**: Lifecycle management, health checks, startup ordering
- **Security tests**: Seccomp, capabilities, filesystem isolation, privilege escalation
- **Network tests**: Connectivity, isolation, firewall rules
- **Observability tests**: Metrics endpoints, structured logging
- **Performance tests**: Resource limits (CPU, memory)

## Running Tests

### Quick Tests (Unit Only)
```bash
cd nix/tests
go test -v -short
```

### Full Test Suite (Requires LXD)
```bash
cd nix/tests
go test -v -race
```

### Via Nix (Automated VM)
```bash
# From repo root
nix build .#hydraJobs.tests
```

### Via Makefile
```bash
# From repo root
make test-containers
```

## Test Structure

```
container_test.go
├── Unit Tests
│   ├── TestContainerConfiguration_ValidIPs
│   ├── TestContainerConfiguration_ValidPorts
│   ├── TestContainerConfiguration_ValidDependencies
│   └── TestContainerConfiguration_NoCyclicDependencies
│
├── Integration Tests
│   ├── TestContainerLifecycle_StartupOrder
│   ├── TestContainerLifecycle_HealthCheck
│   └── TestContainerLifecycle_ConcurrentStartup
│
├── Security Tests
│   ├── TestSecurity_ContainerHasSeccomp
│   ├── TestSecurity_ContainerHasMinimalCapabilities
│   ├── TestSecurity_ContainerFilesystemReadOnly
│   └── TestSecurity_NoPrivilegeEscalation
│
├── Network Tests
│   ├── TestNetwork_ContainerCanReachWotan
│   ├── TestNetwork_ContainerIsolation
│   └── TestNetwork_FirewallRulesActive
│
├── Observability Tests
│   ├── TestObservability_MetricsEndpoint
│   └── TestObservability_StructuredLogging
│
└── Performance Tests
    ├── TestPerformance_MemoryLimits
    └── TestPerformance_CPUQuota
```

## Test Containers

| Container | IP | Port | Dependencies |
|-----------|-----|------|--------------|
| wotan | 10.10.10.10 | 8080 | - |
| timeguru | 10.10.10.20 | 8000 | wotan |
| captain | 10.10.10.21 | 8001 | wotan |
| micromanager | 10.10.10.22 | 8002 | wotan |
| architect | 10.10.10.23 | 8003 | wotan |
| developer | 10.10.10.24 | 8004 | wotan |
| kanban | 10.10.10.200 | 8080 | timeguru |
| dashboard | 10.10.10.201 | 8081 | wotan |

## Writing New Tests

### Test Pattern
```go
func TestFeature_Scenario(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Setup
    container := testContainers[0]

    // Execute
    result, err := checkFeature(container)

    // Assert
    if err != nil {
        t.Errorf("feature check failed: %v", err)
    }
    if result != expected {
        t.Errorf("got %v, want %v", result, expected)
    }
}
```

### Defensive Testing Rules
1. **Always check errors** - Never `_, _ = function()`
2. **Use t.Helper()** - Mark helper functions
3. **Test failure paths** - Not just happy path
4. **Race detection** - Always run with `-race`
5. **Timeout contexts** - Never block forever
6. **Cleanup** - Use `defer` for teardown

## CI/CD Integration

Tests run automatically on:
- Every commit (unit tests)
- Pull requests (full suite)
- Nightly (full suite + load tests)

## Debugging Failed Tests

### Check Container Logs
```bash
lxc exec unheaded-wotan -- journalctl -u wotan -f
```

### Check Network Connectivity
```bash
lxc exec unheaded-timeguru -- curl -v http://10.10.10.10:8080/health
```

### Check Security Settings
```bash
lxc exec unheaded-wotan -- grep Seccomp /proc/1/status
lxc exec unheaded-wotan -- getpcaps 1
```

### Check Resource Limits
```bash
systemctl show unheaded-wotan | grep -E '(Memory|CPU)'
```

## Coverage Goals

- **Unit tests**: 100% (configuration validation)
- **Integration tests**: 90%+ (all critical paths)
- **Security tests**: 100% (every hardening setting)
- **Network tests**: 95%+ (all isolation rules)

## Performance Benchmarks

Expected startup times:
- Wotan: <5s
- Services: <3s (after wotan)
- Apps: <5s (after dependencies)

Health check latency:
- Target: <50ms p99
- Acceptable: <100ms p99
- Alert: >200ms p99

## Related Documentation

- [CLAUDE.md](/CLAUDE.md) - Development standards
- [ARCHITECTURE.md](/docs/ARCHITECTURE.md) - System design
- [nix/modules/hardening.nix](/nix/modules/hardening.nix) - Security config
- [nix/modules/networking.nix](/nix/modules/networking.nix) - Network config
