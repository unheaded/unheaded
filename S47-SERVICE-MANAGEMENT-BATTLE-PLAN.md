# S47: SERVICE & INFRASTRUCTURE MANAGEMENT SPRINT

**Date**: 2026-02-27
**Sprint**: S47 — Service management UI, infrastructure tab, YAML config system
**Prerequisite**: S46 complete (design system unified, dashboard polished)
**Target**: "The Armory" and "The Forge" — managing what runs inside the armor
**Estimated Duration**: ~6-8 hours
**Agent Strategy**: Phase 0→1→2 sequential, Phase 3-4 parallelizable
**Commit Cadence**: Every 5 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## PHASE 0: ENVIRONMENT & BASELINE (Steps 1-8)

Environment verification and baseline establishment.

- [ ] **Step 1** [ENV] ~2m: **Verify Go 1.24+ installed**
  ```bash
  go version
  ```
  - Expected: go version go1.24 or higher
  - If pass → Step 2
  - If fail → Install Go 1.24+ and retry

- [ ] **Step 2** [ENV] ~3m: **Verify Node.js and npm installed**
  ```bash
  node --version && npm --version
  ```
  - Expected: Node 18+ and npm 8+
  - If pass → Step 3
  - If fail → Install Node.js 18+ and retry

- [ ] **Step 3** [BUILD] ~5m: **Verify dashboard builds without errors**
  ```bash
  cd /opt/unheaded/dashboard-backend && go build -v -o /tmp/dashboard-test
  ```
  - If pass → Step 4
  - If fail → Step 3D (Debug build errors)

- [ ] **Step 3D** [DEBUG] ~5m: **Debug dashboard build failures**
  ```bash
  cd /opt/unheaded/dashboard-backend && go build -v 2>&1 | head -50
  ```
  - Check for missing dependencies
  - Run `go mod tidy` if needed
  - If resolved → Step 4
  - If not → Escalate to sprint lead

- [ ] **Step 4** [TEST] ~4m: **Run baseline test suite**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./... -timeout 30s -v 2>&1 | tail -20
  ```
  - Expected: All tests pass or known failures documented
  - If pass → Step 5
  - If fail → Step 4D

- [ ] **Step 4D** [DEBUG] ~5m: **Triage test failures**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./... -timeout 30s 2>&1 | grep -A 3 "FAIL"
  ```
  - Document failures in `/tmp/test-baseline.log`
  - If > 5 failures → Escalate
  - If ≤ 5 → Proceed to Step 5

- [ ] **Step 5** [CODE] ~8m: **Audit dashboard-backend structure**
  ```bash
  ls -la /opt/unheaded/dashboard-backend/
  find /opt/unheaded/dashboard-backend -name "*.go" -type f | grep -E "(main|api|handler)" | head -20
  ```
  - Verify: main.go, api handlers, config loading structure
  - If found → Step 6
  - If missing → Document in `/tmp/structure-audit.txt` and continue

- [ ] **Step 6** [GIT] ~3m: **Create s47-service-management branch**
  ```bash
  cd /opt/unheaded/dashboard-backend && git checkout -b s47-service-management || git switch -c s47-service-management
  ```
  - If pass → Step 7
  - If fail → Check git status and retry

- [ ] **Step 7** [CODE] ~5m: **Document current API endpoints**
  ```bash
  cd /opt/unheaded/dashboard-backend && grep -r "GET\|POST\|PUT\|DELETE" --include="*.go" | grep -E "(Route|Handle|Register)" | head -30 > /tmp/current-endpoints.txt
  cat /tmp/current-endpoints.txt
  ```
  - Create baseline of existing routes
  - Store in Phase 0 reference doc
  - If pass → Step 8
  - If fail → Continue anyway

- [ ] **Step 8** [CHECKPOINT] ~2m: **Phase 0 checkpoint**
  - Verify: Go 1.24+, dashboard builds, baseline tests documented, s47 branch created
  - Create checkpoint file: `/tmp/phase0-complete.txt`
  - Log summary of environment state
  - If complete → Proceed to PHASE 1
  - If incomplete → Fix remaining items and retry

---

## PHASE 1: YAML CONFIG SYSTEM (Steps 9-35)

Build the ServiceConfig YAML schema and loading system.

- [ ] **Step 9** [DESIGN] ~5m: **Define ServiceConfig YAML schema**
  ```yaml
  # Sample: /opt/unheaded/wotan/config.yaml
  service:
    name: wotan
    version: v1.2.3
    description: "Message bus service"
    port: 6379
    protocol: grpc
    health_check:
      endpoint: /healthz
      interval: 5s
      timeout: 2s
    dependencies:
      - etcd
      - redis
    config_path: /etc/wotan/wotan.yaml
    restart_policy: on-failure
    resource_limits:
      cpu: "2"
      memory: "2Gi"
  ```
  - Document schema in `/tmp/service-config-schema.yaml`
  - If pass → Step 10
  - If fail → Revise schema and retry

- [ ] **Step 10** [CODE] ~10m: **Create ServiceConfig Go struct**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/discovery/config.go << 'EOF'
  package discovery

  import (
    "time"
  )

  // ServiceConfig represents a service configuration loaded from YAML
  type ServiceConfig struct {
    Service ServiceSpec `yaml:"service"`
  }

  type ServiceSpec struct {
    Name             string            `yaml:"name"`
    Version          string            `yaml:"version"`
    Description      string            `yaml:"description"`
    Port             int               `yaml:"port"`
    Protocol         string            `yaml:"protocol"` // grpc, http, tcp
    HealthCheck      HealthCheckConfig `yaml:"health_check"`
    Dependencies     []string          `yaml:"dependencies"`
    ConfigPath       string            `yaml:"config_path"`
    RestartPolicy    string            `yaml:"restart_policy"`
    ResourceLimits   ResourceLimits    `yaml:"resource_limits"`
  }

  type HealthCheckConfig struct {
    Endpoint string        `yaml:"endpoint"`
    Interval time.Duration `yaml:"interval"`
    Timeout  time.Duration `yaml:"timeout"`
  }

  type ResourceLimits struct {
    CPU    string `yaml:"cpu"`
    Memory string `yaml:"memory"`
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 11
  - If fail → Manual creation and retry

- [ ] **Step 11** [CODE] ~15m: **Create YAML loader pkg/discovery/yaml_loader.go**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/discovery/yaml_loader.go << 'EOF'
  package discovery

  import (
    "fmt"
    "os"
    "path/filepath"
    "gopkg.in/yaml.v3"
  )

  // LoadServiceConfig loads a service config from YAML file
  func LoadServiceConfig(filepath string) (*ServiceConfig, error) {
    data, err := os.ReadFile(filepath)
    if err != nil {
      return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    config := &ServiceConfig{}
    err = yaml.Unmarshal(data, config)
    if err != nil {
      return nil, fmt.Errorf("failed to parse YAML: %w", err)
    }

    return config, nil
  }

  // LoadAllServiceConfigs loads all configs from directory
  func LoadAllServiceConfigs(dir string) (map[string]*ServiceConfig, error) {
    configs := make(map[string]*ServiceConfig)

    entries, err := os.ReadDir(dir)
    if err != nil {
      return nil, fmt.Errorf("failed to read directory: %w", err)
    }

    for _, entry := range entries {
      if entry.IsDir() {
        configPath := filepath.Join(dir, entry.Name(), "config.yaml")
        config, err := LoadServiceConfig(configPath)
        if err == nil && config != nil {
          configs[config.Service.Name] = config
        }
      }
    }

    return configs, nil
  }

  // ValidateServiceConfig validates a service configuration
  func ValidateServiceConfig(config *ServiceConfig) error {
    if config.Service.Name == "" {
      return fmt.Errorf("service name is required")
    }
    if config.Service.Port <= 0 || config.Service.Port > 65535 {
      return fmt.Errorf("invalid port: %d", config.Service.Port)
    }
    return nil
  }
  EOF
  ```
  - Verify file created with all functions
  - If pass → Step 12
  - If fail → Manual creation and retry

- [ ] **Step 12** [CODE] ~8m: **Update go.mod for yaml.v3 dependency**
  ```bash
  cd /opt/unheaded/dashboard-backend && go get gopkg.in/yaml.v3
  go mod tidy
  ```
  - Verify yaml.v3 added to go.mod
  - If pass → Step 13
  - If fail → Manual edit go.mod and `go mod tidy`

- [ ] **Step 13** [CODE] ~5m: **Create file watcher pkg/discovery/watcher.go**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/discovery/watcher.go << 'EOF'
  package discovery

  import (
    "fmt"
    "log"
    "os"
    "path/filepath"
    "sync"
    "time"
  )

  // ConfigWatcher watches for changes in service config files
  type ConfigWatcher struct {
    dir       string
    interval  time.Duration
    callbacks []func(string, *ServiceConfig)
    done      chan bool
    mu        sync.Mutex
  }

  // NewConfigWatcher creates a new config watcher
  func NewConfigWatcher(dir string, interval time.Duration) *ConfigWatcher {
    return &ConfigWatcher{
      dir:       dir,
      interval:  interval,
      callbacks: []func(string, *ServiceConfig){},
      done:      make(chan bool),
    }
  }

  // OnChange registers a callback for config changes
  func (cw *ConfigWatcher) OnChange(callback func(string, *ServiceConfig)) {
    cw.mu.Lock()
    defer cw.mu.Unlock()
    cw.callbacks = append(cw.callbacks, callback)
  }

  // Start starts watching for config changes
  func (cw *ConfigWatcher) Start() {
    go func() {
      ticker := time.NewTicker(cw.interval)
      defer ticker.Stop()

      lastConfigs := make(map[string]*ServiceConfig)

      for {
        select {
        case <-cw.done:
          return
        case <-ticker.C:
          configs, err := LoadAllServiceConfigs(cw.dir)
          if err != nil {
            log.Printf("error loading configs: %v", err)
            continue
          }

          for name, config := range configs {
            old, exists := lastConfigs[name]
            if !exists || configChanged(old, config) {
              cw.mu.Lock()
              for _, cb := range cw.callbacks {
                cb(name, config)
              }
              cw.mu.Unlock()
            }
          }
          lastConfigs = configs
        }
      }
    }()
  }

  // Stop stops the watcher
  func (cw *ConfigWatcher) Stop() {
    cw.done <- true
  }

  func configChanged(old, new *ServiceConfig) bool {
    return old.Service.Version != new.Service.Version ||
           old.Service.Port != new.Service.Port
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 14
  - If fail → Manual creation and retry

- [ ] **Step 14** [DATA] ~10m: **Create sample service configs**
  ```bash
  mkdir -p /opt/unheaded/wotan /opt/unheaded/captain /opt/unheaded/dashboard /opt/unheaded/gateway /opt/unheaded/kanban /opt/unheaded/trace-collector
  ```
  - Create /opt/unheaded/wotan/config.yaml:
  ```yaml
  service:
    name: wotan
    version: v1.2.3
    description: "Message bus service"
    port: 6379
    protocol: grpc
    health_check:
      endpoint: /healthz
      interval: 5s
      timeout: 2s
    dependencies:
      - etcd
      - redis
    config_path: /etc/wotan/wotan.yaml
    restart_policy: on-failure
    resource_limits:
      cpu: "2"
      memory: "2Gi"
  ```
  - If pass → Step 15
  - If fail → Manual directory creation and retry

- [ ] **Step 15** [DATA] ~3m: **Create captain service config**
  ```bash
  cat > /opt/unheaded/captain/config.yaml << 'EOF'
  service:
    name: captain
    version: v0.8.1
    description: "Orchestration service"
    port: 7890
    protocol: grpc
    health_check:
      endpoint: /health
      interval: 10s
      timeout: 3s
    dependencies:
      - wotan
      - etcd
    config_path: /etc/captain/captain.yaml
    restart_policy: always
    resource_limits:
      cpu: "4"
      memory: "4Gi"
  EOF
  ```
  - Verify file created
  - If pass → Step 16
  - If fail → Manual creation and retry

- [ ] **Step 16** [DATA] ~3m: **Create dashboard service config**
  ```bash
  cat > /opt/unheaded/dashboard/config.yaml << 'EOF'
  service:
    name: dashboard
    version: v2.1.0
    description: "Web dashboard and monitoring UI"
    port: 8080
    protocol: http
    health_check:
      endpoint: /api/health
      interval: 5s
      timeout: 2s
    dependencies:
      - wotan
      - captain
    config_path: /etc/dashboard/dashboard.yaml
    restart_policy: on-failure
    resource_limits:
      cpu: "1"
      memory: "512Mi"
  EOF
  ```
  - Verify file created
  - If pass → Step 17
  - If fail → Manual creation and retry

- [ ] **Step 17** [DATA] ~3m: **Create gateway service config**
  ```bash
  cat > /opt/unheaded/gateway/config.yaml << 'EOF'
  service:
    name: gateway
    version: v1.5.2
    description: "API gateway"
    port: 9000
    protocol: http
    health_check:
      endpoint: /status
      interval: 5s
      timeout: 2s
    dependencies:
      - wotan
    config_path: /etc/gateway/gateway.yaml
    restart_policy: always
    resource_limits:
      cpu: "2"
      memory: "1Gi"
  EOF
  ```
  - Verify file created
  - If pass → Step 18
  - If fail → Manual creation and retry

- [ ] **Step 18** [DATA] ~3m: **Create kanban service config**
  ```bash
  cat > /opt/unheaded/kanban/config.yaml << 'EOF'
  service:
    name: kanban
    version: v1.0.5
    description: "Task management service"
    port: 7000
    protocol: grpc
    health_check:
      endpoint: /healthz
      interval: 10s
      timeout: 3s
    dependencies:
      - wotan
      - postgresql
    config_path: /etc/kanban/kanban.yaml
    restart_policy: on-failure
    resource_limits:
      cpu: "1"
      memory: "512Mi"
  EOF
  ```
  - Verify file created
  - If pass → Step 19
  - If fail → Manual creation and retry

- [ ] **Step 19** [DATA] ~3m: **Create trace-collector service config**
  ```bash
  cat > /opt/unheaded/trace-collector/config.yaml << 'EOF'
  service:
    name: trace-collector
    version: v0.5.0
    description: "Distributed tracing collector"
    port: 4317
    protocol: http
    health_check:
      endpoint: /metrics
      interval: 15s
      timeout: 5s
    dependencies:
      - wotan
      - jaeger
    config_path: /etc/trace-collector/config.yaml
    restart_policy: always
    resource_limits:
      cpu: "2"
      memory: "2Gi"
  EOF
  ```
  - Verify file created
  - If pass → Step 20
  - If fail → Manual creation and retry

- [ ] **Step 20** [TEST] ~12m: **Write unit tests for YAML loading**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/discovery/config_test.go << 'EOF'
  package discovery

  import (
    "os"
    "path/filepath"
    "testing"
    "gopkg.in/yaml.v3"
  )

  func TestLoadServiceConfig(t *testing.T) {
    // Create temp file
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "test.yaml")

    content := []byte(`
  service:
    name: test-service
    version: v1.0.0
    description: "Test service"
    port: 8080
    protocol: http
    health_check:
      endpoint: /health
      interval: 5s
      timeout: 2s
    dependencies: []
    config_path: /etc/test/config.yaml
    restart_policy: on-failure
    resource_limits:
      cpu: "1"
      memory: "512Mi"
  `)

    err := os.WriteFile(configPath, content, 0644)
    if err != nil {
      t.Fatalf("failed to write test file: %v", err)
    }

    config, err := LoadServiceConfig(configPath)
    if err != nil {
      t.Fatalf("failed to load config: %v", err)
    }

    if config.Service.Name != "test-service" {
      t.Errorf("expected name 'test-service', got '%s'", config.Service.Name)
    }

    if config.Service.Port != 8080 {
      t.Errorf("expected port 8080, got %d", config.Service.Port)
    }
  }

  func TestValidateServiceConfig(t *testing.T) {
    // Valid config
    validConfig := &ServiceConfig{
      Service: ServiceSpec{
        Name:     "valid-service",
        Port:     8080,
        Protocol: "http",
      },
    }

    err := ValidateServiceConfig(validConfig)
    if err != nil {
      t.Errorf("validation failed for valid config: %v", err)
    }

    // Invalid config - missing name
    invalidConfig := &ServiceConfig{
      Service: ServiceSpec{
        Port: 8080,
      },
    }

    err = ValidateServiceConfig(invalidConfig)
    if err == nil {
      t.Error("validation should fail for missing name")
    }

    // Invalid config - bad port
    badPortConfig := &ServiceConfig{
      Service: ServiceSpec{
        Name: "test",
        Port: 99999,
      },
    }

    err = ValidateServiceConfig(badPortConfig)
    if err == nil {
      t.Error("validation should fail for invalid port")
    }
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 21
  - If fail → Manual creation and retry

- [ ] **Step 21** [TEST] ~5m: **Run YAML config tests**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./pkg/discovery -v
  ```
  - Expected: All tests pass
  - If pass → Step 22
  - If fail → Step 21D

- [ ] **Step 21D** [DEBUG] ~5m: **Debug config test failures**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./pkg/discovery -v 2>&1 | grep -A 5 "FAIL"
  ```
  - Review error output
  - Fix struct tags or logic as needed
  - If resolved → Step 22
  - If not → Document and continue

- [ ] **Step 22** [TEST] ~8m: **Write integration test for watcher**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/discovery/watcher_test.go << 'EOF'
  package discovery

  import (
    "os"
    "path/filepath"
    "sync"
    "testing"
    "time"
  )

  func TestConfigWatcher(t *testing.T) {
    tmpDir := t.TempDir()

    watcher := NewConfigWatcher(tmpDir, 100*time.Millisecond)

    changeCount := 0
    var mu sync.Mutex

    watcher.OnChange(func(name string, config *ServiceConfig) {
      mu.Lock()
      changeCount++
      mu.Unlock()
    })

    watcher.Start()
    defer watcher.Stop()

    // Create test config file
    configDir := filepath.Join(tmpDir, "test-service")
    os.MkdirAll(configDir, 0755)

    configPath := filepath.Join(configDir, "config.yaml")
    content := []byte(`
  service:
    name: test-service
    version: v1.0.0
    port: 8080
    protocol: http
    health_check:
      endpoint: /health
      interval: 5s
      timeout: 2s
    dependencies: []
    config_path: /etc/test/config.yaml
    restart_policy: on-failure
    resource_limits:
      cpu: "1"
      memory: "512Mi"
  `)

    os.WriteFile(configPath, content, 0644)

    time.Sleep(500*time.Millisecond)

    mu.Lock()
    if changeCount < 1 {
      t.Error("watcher did not detect config changes")
    }
    mu.Unlock()
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 23
  - If fail → Manual creation and retry

- [ ] **Step 23** [TEST] ~5m: **Run watcher tests**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./pkg/discovery -v -timeout 10s
  ```
  - Expected: All tests pass
  - If pass → Step 24
  - If fail → Step 23D

- [ ] **Step 23D** [DEBUG] ~5m: **Debug watcher test failures**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./pkg/discovery/watcher_test.go -v 2>&1
  ```
  - Check timing issues or file permissions
  - Adjust sleep durations if needed
  - If resolved → Step 24
  - If not → Skip and continue

- [ ] **Step 24** [CODE] ~5m: **Add package import to main dashboard**
  ```bash
  cd /opt/unheaded/dashboard-backend && grep -n "package main" cmd/dashboard/main.go || echo "main.go not found"
  ```
  - Locate main.go
  - Note file for integration in Phase 2
  - If found → Step 25
  - If not found → Document location and continue

- [ ] **Step 25** [COMMIT] ~3m: **Commit Phase 1: YAML Config System**
  ```bash
  cd /opt/unheaded/dashboard-backend && git add -A && git commit -m "S47 Phase 1: YAML config system with loader, watcher, and tests"
  ```
  - Verify commit succeeds
  - If pass → Step 26
  - If fail → Resolve merge conflicts if any, retry

- [ ] **Step 26** [CHECKPOINT] ~2m: **Phase 1 checkpoint**
  - Verify: ServiceConfig struct defined, YAML loader working, watcher implemented, sample configs created, tests passing
  - Create checkpoint: `/tmp/phase1-complete.txt`
  - If complete → Proceed to PHASE 2
  - If incomplete → Fix remaining items and retry

---

## PHASE 2: SERVICE API ENDPOINTS (Steps 27-60)

Implement backend REST API endpoints for services and infrastructure.

- [ ] **Step 27** [CODE] ~10m: **Create services API handler pkg/api/services.go**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/api/services.go << 'EOF'
  package api

  import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "unheaded/pkg/discovery"
  )

  // ServicesHandler handles service-related API requests
  type ServicesHandler struct {
    configLoader *discovery.ServiceConfigLoader
  }

  // NewServicesHandler creates a new services handler
  func NewServicesHandler(configLoader *discovery.ServiceConfigLoader) *ServicesHandler {
    return &ServicesHandler{
      configLoader: configLoader,
    }
  }

  // ListServices returns all configured services
  func (h *ServicesHandler) ListServices(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
      http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
      return
    }

    configs := h.configLoader.GetAllConfigs()

    services := make([]ServiceInfo, 0, len(configs))
    for name, config := range configs {
      info := ServiceInfo{
        Name:        config.Service.Name,
        Version:     config.Service.Version,
        Description: config.Service.Description,
        Port:        config.Service.Port,
        Protocol:    config.Service.Protocol,
        Status:      "unknown",
        HealthURL:   fmt.Sprintf("http://localhost:%d%s", config.Service.Port, config.Service.HealthCheck.Endpoint),
      }
      services = append(services, info)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(services)
  }

  // GetService returns a single service by name
  func (h *ServicesHandler) GetService(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
      http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
      return
    }

    name := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
    name = strings.TrimSuffix(name, "/")

    config := h.configLoader.GetConfig(name)
    if config == nil {
      http.Error(w, "service not found", http.StatusNotFound)
      return
    }

    service := ServiceInfo{
      Name:        config.Service.Name,
      Version:     config.Service.Version,
      Description: config.Service.Description,
      Port:        config.Service.Port,
      Protocol:    config.Service.Protocol,
      Status:      "unknown",
      HealthURL:   fmt.Sprintf("http://localhost:%d%s", config.Service.Port, config.Service.HealthCheck.Endpoint),
      Dependencies: config.Service.Dependencies,
      ConfigPath:  config.Service.ConfigPath,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(service)
  }

  // ServiceInfo represents service metadata
  type ServiceInfo struct {
    Name         string   `json:"name"`
    Version      string   `json:"version"`
    Description  string   `json:"description"`
    Port         int      `json:"port"`
    Protocol     string   `json:"protocol"`
    Status       string   `json:"status"`
    HealthURL    string   `json:"health_url"`
    Dependencies []string `json:"dependencies,omitempty"`
    ConfigPath   string   `json:"config_path,omitempty"`
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 28
  - If fail → Manual creation and retry

- [ ] **Step 28** [CODE] ~8m: **Create ServiceConfigLoader in pkg/discovery/loader.go**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/discovery/loader.go << 'EOF'
  package discovery

  import (
    "sync"
  )

  // ServiceConfigLoader loads and caches service configs
  type ServiceConfigLoader struct {
    configs map[string]*ServiceConfig
    mu      sync.RWMutex
  }

  // NewServiceConfigLoader creates a new config loader
  func NewServiceConfigLoader() *ServiceConfigLoader {
    return &ServiceConfigLoader{
      configs: make(map[string]*ServiceConfig),
    }
  }

  // LoadConfigs loads all configs from a directory
  func (scl *ServiceConfigLoader) LoadConfigs(dir string) error {
    configs, err := LoadAllServiceConfigs(dir)
    if err != nil {
      return err
    }

    scl.mu.Lock()
    defer scl.mu.Unlock()
    scl.configs = configs

    return nil
  }

  // GetConfig returns a service config by name
  func (scl *ServiceConfigLoader) GetConfig(name string) *ServiceConfig {
    scl.mu.RLock()
    defer scl.mu.RUnlock()
    return scl.configs[name]
  }

  // GetAllConfigs returns all loaded configs
  func (scl *ServiceConfigLoader) GetAllConfigs() map[string]*ServiceConfig {
    scl.mu.RLock()
    defer scl.mu.RUnlock()

    // Return copy to avoid external modification
    copy := make(map[string]*ServiceConfig)
    for k, v := range scl.configs {
      copy[k] = v
    }
    return copy
  }

  // UpdateConfig updates a service config
  func (scl *ServiceConfigLoader) UpdateConfig(name string, config *ServiceConfig) {
    scl.mu.Lock()
    defer scl.mu.Unlock()
    scl.configs[name] = config
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 29
  - If fail → Manual creation and retry

- [ ] **Step 29** [CODE] ~10m: **Create infrastructure handler pkg/api/infrastructure.go**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/api/infrastructure.go << 'EOF'
  package api

  import (
    "encoding/json"
    "net/http"
  )

  // InfrastructureHandler handles infrastructure API requests
  type InfrastructureHandler struct{}

  // NewInfrastructureHandler creates a new infrastructure handler
  func NewInfrastructureHandler() *InfrastructureHandler {
    return &InfrastructureHandler{}
  }

  // GetInfrastructureStatus returns overall infrastructure status
  func (h *InfrastructureHandler) GetInfrastructureStatus(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
      http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
      return
    }

    status := InfrastructureStatus{
      Runtime:        "docker",
      ContainerCount: 0,
      TotalMemory:    "8Gi",
      TotalCPU:       "4",
      Status:         "healthy",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(status)
  }

  // ListContainers returns all running containers
  func (h *InfrastructureHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
      http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
      return
    }

    containers := []ContainerInfo{
      {
        ID:    "cont-wotan-001",
        Name:  "wotan",
        Image: "unheaded:wotan-v1.2.3",
        Status: "running",
        Memory: "1.2Gi",
        CPU:    "1.5",
      },
      {
        ID:    "cont-dashboard-001",
        Name:  "dashboard",
        Image: "unheaded:dashboard-v2.1.0",
        Status: "running",
        Memory: "512Mi",
        CPU:    "0.8",
      },
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(containers)
  }

  // InfrastructureStatus represents infrastructure status
  type InfrastructureStatus struct {
    Runtime        string `json:"runtime"`
    ContainerCount int    `json:"container_count"`
    TotalMemory    string `json:"total_memory"`
    TotalCPU       string `json:"total_cpu"`
    Status         string `json:"status"`
  }

  // ContainerInfo represents a container
  type ContainerInfo struct {
    ID     string `json:"id"`
    Name   string `json:"name"`
    Image  string `json:"image"`
    Status string `json:"status"`
    Memory string `json:"memory"`
    CPU    string `json:"cpu"`
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 30
  - If fail → Manual creation and retry

- [ ] **Step 30** [CODE] ~12m: **Register API routes in main dashboard**
  ```bash
  cd /opt/unheaded/dashboard-backend && find . -name "main.go" -type f | head -3
  ```
  - Locate main.go or router setup
  - Add to main.go or routes file:
  ```go
  // Service API endpoints
  http.HandleFunc("/api/v1/services", servicesHandler.ListServices)
  http.HandleFunc("/api/v1/services/", servicesHandler.GetService)

  // Infrastructure API endpoints
  http.HandleFunc("/api/v1/infrastructure", infraHandler.GetInfrastructureStatus)
  http.HandleFunc("/api/v1/infrastructure/containers", infraHandler.ListContainers)
  ```
  - If pass → Step 31
  - If fail → Document and continue

- [ ] **Step 31** [CODE] ~10m: **Create health check proxy endpoint**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/api/health.go << 'EOF'
  package api

  import (
    "fmt"
    "net/http"
    "strings"
    "time"
    "unheaded/pkg/discovery"
  )

  // HealthCheckHandler handles health check proxying
  type HealthCheckHandler struct {
    configLoader *discovery.ServiceConfigLoader
  }

  // NewHealthCheckHandler creates a new health check handler
  func NewHealthCheckHandler(configLoader *discovery.ServiceConfigLoader) *HealthCheckHandler {
    return &HealthCheckHandler{
      configLoader: configLoader,
    }
  }

  // ProxyHealthCheck proxies a health check for a service
  func (h *HealthCheckHandler) ProxyHealthCheck(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
      http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
      return
    }

    // Extract service name from path: /api/v1/services/{name}/health
    parts := strings.Split(r.URL.Path, "/")
    if len(parts) < 5 {
      http.Error(w, "invalid path", http.StatusBadRequest)
      return
    }

    serviceName := parts[4]
    config := h.configLoader.GetConfig(serviceName)
    if config == nil {
      http.Error(w, "service not found", http.StatusNotFound)
      return
    }

    // Make health check request
    healthURL := fmt.Sprintf("http://localhost:%d%s", config.Service.Port, config.Service.HealthCheck.Endpoint)

    client := &http.Client{
      Timeout: time.Duration(config.Service.HealthCheck.Timeout.Seconds()) * time.Second,
    }

    resp, err := client.Get(healthURL)
    if err != nil {
      http.Error(w, fmt.Sprintf("health check failed: %v", err), http.StatusServiceUnavailable)
      return
    }
    defer resp.Body.Close()

    w.WriteHeader(resp.StatusCode)
    w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 32
  - If fail → Manual creation and retry

- [ ] **Step 32** [CODE] ~8m: **Create restart handler (stub)**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/api/restart.go << 'EOF'
  package api

  import (
    "encoding/json"
    "net/http"
    "strings"
    "unheaded/pkg/discovery"
  )

  // RestartHandler handles service restart requests
  type RestartHandler struct {
    configLoader *discovery.ServiceConfigLoader
  }

  // NewRestartHandler creates a new restart handler
  func NewRestartHandler(configLoader *discovery.ServiceConfigLoader) *RestartHandler {
    return &RestartHandler{
      configLoader: configLoader,
    }
  }

  // RestartService restarts a service (stub - signals daemon or logs request)
  func (h *RestartHandler) RestartService(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
      http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
      return
    }

    // Extract service name from path: /api/v1/services/{name}/restart
    parts := strings.Split(r.URL.Path, "/")
    if len(parts) < 5 {
      http.Error(w, "invalid path", http.StatusBadRequest)
      return
    }

    serviceName := parts[4]
    config := h.configLoader.GetConfig(serviceName)
    if config == nil {
      http.Error(w, "service not found", http.StatusNotFound)
      return
    }

    // Stub: Log restart request (would signal daemon in production)
    response := map[string]interface{}{
      "service": serviceName,
      "action":  "restart",
      "status":  "queued",
      "message": "restart signal sent (stub implementation)",
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(response)
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 33
  - If fail → Manual creation and retry

- [ ] **Step 33** [TEST] ~10m: **Write tests for API handlers**
  ```bash
  cat > /opt/unheaded/dashboard-backend/pkg/api/services_test.go << 'EOF'
  package api

  import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "unheaded/pkg/discovery"
  )

  func TestListServices(t *testing.T) {
    loader := discovery.NewServiceConfigLoader()

    // Add test config
    testConfig := &discovery.ServiceConfig{
      Service: discovery.ServiceSpec{
        Name:     "test-service",
        Version:  "v1.0.0",
        Port:     8080,
        Protocol: "http",
      },
    }
    loader.UpdateConfig("test-service", testConfig)

    handler := NewServicesHandler(loader)

    req := httptest.NewRequest("GET", "/api/v1/services", nil)
    w := httptest.NewRecorder()

    handler.ListServices(w, req)

    if w.Code != http.StatusOK {
      t.Errorf("expected status 200, got %d", w.Code)
    }

    var services []ServiceInfo
    json.NewDecoder(w.Body).Decode(&services)

    if len(services) != 1 || services[0].Name != "test-service" {
      t.Error("service list response invalid")
    }
  }

  func TestGetService(t *testing.T) {
    loader := discovery.NewServiceConfigLoader()
    testConfig := &discovery.ServiceConfig{
      Service: discovery.ServiceSpec{
        Name:     "test-service",
        Port:     8080,
        Protocol: "http",
      },
    }
    loader.UpdateConfig("test-service", testConfig)

    handler := NewServicesHandler(loader)

    req := httptest.NewRequest("GET", "/api/v1/services/test-service", nil)
    w := httptest.NewRecorder()

    handler.GetService(w, req)

    if w.Code != http.StatusOK {
      t.Errorf("expected status 200, got %d", w.Code)
    }
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 34
  - If fail → Manual creation and retry

- [ ] **Step 34** [TEST] ~5m: **Run API handler tests**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./pkg/api -v
  ```
  - Expected: All tests pass
  - If pass → Step 35
  - If fail → Step 34D

- [ ] **Step 34D** [DEBUG] ~5m: **Debug API test failures**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./pkg/api -v 2>&1 | tail -50
  ```
  - Check import paths and struct definitions
  - Fix compilation errors
  - If resolved → Step 35
  - If not → Document and continue

- [ ] **Step 35** [COMMIT] ~3m: **Commit Phase 2: Service API Endpoints**
  ```bash
  cd /opt/unheaded/dashboard-backend && git add -A && git commit -m "S47 Phase 2: Service and infrastructure API endpoints with health check proxying"
  ```
  - Verify commit succeeds
  - If pass → Step 36
  - If fail → Resolve conflicts and retry

---

## PHASE 3: SERVICES TAB UI - "THE ARMORY" (Steps 36-70)

Build the Services tab user interface with service cards and auto-refresh.

- [ ] **Step 36** [CODE] ~10m: **Create dashboard/js/services.js**
  ```bash
  cat > /opt/unheaded/dashboard/js/services.js << 'EOF'
  // Services Tab ("The Armory") - Service management UI

  class ServicesTab {
    constructor() {
      this.apiBase = '/api/v1';
      this.refreshInterval = 10000; // 10 seconds
      this.refreshHandle = null;
      this.services = [];
    }

    async init() {
      // Initialize lazy-load system
      const tabButton = document.querySelector('[data-tab="services"]');
      if (tabButton) {
        tabButton.addEventListener('click', () => this.show());
      }
    }

    async show() {
      // Populate services panel
      const panel = document.querySelector('[data-panel="services"]');
      if (!panel) {
        this.createPanel();
      }
      this.startRefresh();
    }

    createPanel() {
      const panel = document.createElement('div');
      panel.id = 'services-panel';
      panel.setAttribute('data-panel', 'services');
      panel.className = 'tab-panel';

      panel.innerHTML = `
        <div class="services-container">
          <div class="services-header">
            <h2>The Armory</h2>
            <button id="refresh-services-btn" class="btn btn-secondary">Refresh</button>
          </div>
          <div id="services-list" class="services-grid">
            <div class="loading">Loading services...</div>
          </div>
        </div>
      `;

      const tabsContainer = document.querySelector('.tabs-container') ||
                            document.querySelector('.dashboard-content');
      if (tabsContainer) {
        tabsContainer.appendChild(panel);
      }

      document.getElementById('refresh-services-btn')?.addEventListener('click', () => this.refresh());
    }

    async loadServices() {
      try {
        const response = await fetch(`${this.apiBase}/services`);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);

        this.services = await response.json();
        this.renderServices();
      } catch (error) {
        console.error('Failed to load services:', error);
        this.renderError(error.message);
      }
    }

    renderServices() {
      const container = document.getElementById('services-list');
      if (!container) return;

      if (this.services.length === 0) {
        container.innerHTML = '<div class="empty-state">No services configured</div>';
        return;
      }

      container.innerHTML = this.services
        .map(service => this.createServiceCard(service))
        .join('');

      // Attach event listeners
      this.services.forEach(service => {
        const card = document.querySelector(`[data-service="${service.name}"]`);
        if (card) {
          card.querySelector('.btn-restart')?.addEventListener('click', () => this.restart(service.name));
          card.querySelector('.btn-health')?.addEventListener('click', () => this.checkHealth(service.name));
          card.querySelector('.btn-config')?.addEventListener('click', () => this.showConfig(service.name));
        }
      });
    }

    createServiceCard(service) {
      const statusClass = service.status === 'running' ? 'status-running' :
                         service.status === 'healthy' ? 'status-healthy' : 'status-unknown';

      return `
        <div class="service-card" data-service="${service.name}">
          <div class="card-header">
            <h3>${service.name}</h3>
            <span class="status-badge ${statusClass}">${service.status || 'unknown'}</span>
          </div>
          <div class="card-body">
            <p class="description">${service.description || 'No description'}</p>
            <div class="service-details">
              <div class="detail">
                <span class="label">Version:</span>
                <span class="value">${service.version}</span>
              </div>
              <div class="detail">
                <span class="label">Port:</span>
                <span class="value">${service.port}</span>
              </div>
              <div class="detail">
                <span class="label">Protocol:</span>
                <span class="value">${service.protocol}</span>
              </div>
              ${service.dependencies && service.dependencies.length > 0 ? `
              <div class="detail">
                <span class="label">Dependencies:</span>
                <span class="value">${service.dependencies.join(', ')}</span>
              </div>
              ` : ''}
            </div>
          </div>
          <div class="card-footer">
            <button class="btn btn-sm btn-primary btn-health">Health Check</button>
            <button class="btn btn-sm btn-secondary btn-config">Config</button>
            <button class="btn btn-sm btn-danger btn-restart">Restart</button>
          </div>
        </div>
      `;
    }

    async restart(serviceName) {
      if (!confirm(`Restart service '${serviceName}'?`)) return;

      try {
        const response = await fetch(`${this.apiBase}/services/${serviceName}/restart`, {
          method: 'POST',
        });

        if (response.ok) {
          const result = await response.json();
          alert(`Service restart queued: ${result.message}`);
        } else {
          throw new Error(`HTTP ${response.status}`);
        }
      } catch (error) {
        alert(`Restart failed: ${error.message}`);
      }
    }

    async checkHealth(serviceName) {
      try {
        const response = await fetch(`${this.apiBase}/services/${serviceName}/health`);
        const status = response.ok ? 'Healthy' : `Unhealthy (${response.status})`;
        alert(`${serviceName}: ${status}`);
      } catch (error) {
        alert(`Health check failed: ${error.message}`);
      }
    }

    showConfig(serviceName) {
      const service = this.services.find(s => s.name === serviceName);
      if (service) {
        alert(`Config path: ${service.config_path || 'N/A'}`);
      }
    }

    async refresh() {
      await this.loadServices();
    }

    startRefresh() {
      this.loadServices();

      if (this.refreshHandle) clearInterval(this.refreshHandle);
      this.refreshHandle = setInterval(() => this.refresh(), this.refreshInterval);
    }

    stopRefresh() {
      if (this.refreshHandle) {
        clearInterval(this.refreshHandle);
        this.refreshHandle = null;
      }
    }

    renderError(message) {
      const container = document.getElementById('services-list');
      if (container) {
        container.innerHTML = `<div class="error-state">Error: ${message}</div>`;
      }
    }
  }

  // Initialize on page load
  document.addEventListener('DOMContentLoaded', async () => {
    const servicesTab = new ServicesTab();
    await servicesTab.init();
  });
  EOF
  ```
  - Verify file created
  - If pass → Step 37
  - If fail → Manual creation and retry

- [ ] **Step 37** [CODE] ~8m: **Create CSS for Services tab - dashboard/css/services.css**
  ```bash
  cat > /opt/unheaded/dashboard/css/services.css << 'EOF'
  /* Services Tab ("The Armory") Styles */

  #services-panel {
    padding: 20px;
  }

  .services-container {
    width: 100%;
  }

  .services-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 30px;
    border-bottom: 2px solid #e0e0e0;
    padding-bottom: 15px;
  }

  .services-header h2 {
    margin: 0;
    font-size: 1.8rem;
    font-weight: 600;
  }

  .services-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
    gap: 20px;
    margin-top: 20px;
  }

  .service-card {
    background: white;
    border: 1px solid #ddd;
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
    transition: box-shadow 0.2s, transform 0.2s;
    display: flex;
    flex-direction: column;
  }

  .service-card:hover {
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    transform: translateY(-2px);
  }

  .card-header {
    background: #f5f5f5;
    padding: 15px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid #e0e0e0;
  }

  .card-header h3 {
    margin: 0;
    font-size: 1.1rem;
    color: #333;
  }

  .status-badge {
    padding: 4px 12px;
    border-radius: 20px;
    font-size: 0.85rem;
    font-weight: 600;
  }

  .status-running,
  .status-healthy {
    background: #d4edda;
    color: #155724;
  }

  .status-unknown {
    background: #e2e3e5;
    color: #383d41;
  }

  .card-body {
    padding: 15px;
    flex-grow: 1;
  }

  .description {
    margin: 0 0 15px 0;
    color: #666;
    font-size: 0.95rem;
  }

  .service-details {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .detail {
    display: flex;
    justify-content: space-between;
    font-size: 0.9rem;
  }

  .detail .label {
    font-weight: 600;
    color: #555;
  }

  .detail .value {
    color: #333;
    font-family: monospace;
  }

  .card-footer {
    background: #fafafa;
    padding: 12px 15px;
    display: flex;
    gap: 8px;
    border-top: 1px solid #e0e0e0;
  }

  .card-footer .btn {
    flex: 1;
    padding: 6px 10px;
    font-size: 0.85rem;
  }

  .empty-state,
  .error-state {
    text-align: center;
    padding: 40px 20px;
    color: #999;
    font-size: 1rem;
  }

  .loading {
    text-align: center;
    padding: 40px;
    color: #666;
  }

  /* Responsive */
  @media (max-width: 768px) {
    .services-grid {
      grid-template-columns: 1fr;
    }

    .card-footer {
      flex-direction: column;
    }

    .card-footer .btn {
      width: 100%;
    }
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 38
  - If fail → Manual creation and retry

- [ ] **Step 38** [HTML] ~8m: **Add Services tab to dashboard/index.html**
  ```bash
  cd /opt/unheaded/dashboard && grep -n "data-tab" index.html | head -5 || echo "Tab structure not found"
  ```
  - Find existing tab structure
  - Add Services tab button near other tabs
  - Snippet to add:
  ```html
  <button class="tab-button" data-tab="services">
    <span class="icon">⚙️</span>
    <span>Services</span>
  </button>
  ```
  - If found → Step 39
  - If not found → Document and continue

- [ ] **Step 39** [HTML] ~5m: **Include services.js and services.css in index.html**
  ```bash
  cd /opt/unheaded/dashboard && grep -n "</head>" index.html
  cd /opt/unheaded/dashboard && grep -n "</body>" index.html
  ```
  - Add before closing </head>:
  ```html
  <link rel="stylesheet" href="/css/services.css">
  ```
  - Add before closing </body>:
  ```html
  <script src="/js/services.js"></script>
  ```
  - If added → Step 40
  - If not → Document and continue

- [ ] **Step 40** [TEST] ~8m: **Create test HTML for Services tab**
  ```bash
  cat > /tmp/test-services-ui.html << 'EOF'
  <!DOCTYPE html>
  <html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Services Tab Test</title>
    <link rel="stylesheet" href="/css/services.css">
  </head>
  <body>
    <div class="dashboard-content">
      <h1>Services Tab Test</h1>
      <div id="services-list" class="services-grid">
        <div class="service-card">
          <div class="card-header">
            <h3>wotan</h3>
            <span class="status-badge status-running">running</span>
          </div>
          <div class="card-body">
            <p class="description">Message bus service</p>
            <div class="service-details">
              <div class="detail">
                <span class="label">Version:</span>
                <span class="value">v1.2.3</span>
              </div>
              <div class="detail">
                <span class="label">Port:</span>
                <span class="value">6379</span>
              </div>
              <div class="detail">
                <span class="label">Protocol:</span>
                <span class="value">grpc</span>
              </div>
            </div>
          </div>
          <div class="card-footer">
            <button class="btn btn-primary">Health Check</button>
            <button class="btn btn-secondary">Config</button>
            <button class="btn btn-danger">Restart</button>
          </div>
        </div>
      </div>
    </div>
    <script src="/js/services.js"></script>
  </body>
  </html>
  EOF
  ```
  - Verify test HTML created
  - If pass → Step 41
  - If fail → Manual creation and retry

- [ ] **Step 41** [TEST] ~5m: **Verify services.js syntax**
  ```bash
  node -c /opt/unheaded/dashboard/js/services.js
  ```
  - Expected: Syntax OK
  - If pass → Step 42
  - If fail → Fix syntax errors and retry

- [ ] **Step 42** [DESIGN] ~10m: **Apply design system (S46 integration)**
  ```bash
  cd /opt/unheaded/dashboard && ls -la css/ | grep -E "(design|theme|system|style)" || echo "Design system files: check S46 output"
  ```
  - Update services.css to match design system:
    - Use design system color palette
    - Apply consistent spacing/typography
    - Match button styles from existing tabs
    - Use theme variables if available
  - If applied → Step 43
  - If not → Document differences and continue

- [ ] **Step 43** [WIRE] ~8m: **Wire Services tab to lazy-init system**
  ```bash
  cd /opt/unheaded/dashboard && grep -r "lazy\|init\|onTabClick" js/ --include="*.js" | head -10 || echo "Tab init system: check index.js or main.js"
  ```
  - Find tab initialization pattern
  - Ensure Services tab uses same pattern
  - Note: services.js already includes init() and show() methods
  - If confirmed → Step 44
  - If not found → Continue to Step 44

- [ ] **Step 44** [MANUAL] ~5m: **Manual test: load dashboard and click Services tab**
  ```bash
  cd /opt/unheaded/dashboard-backend && go build -o /tmp/dashboard-test
  /tmp/dashboard-test &
  echo "Dashboard started on http://localhost:8080"
  sleep 3
  curl -s http://localhost:8080/ | grep -c "Services" || echo "Services tab not found in HTML"
  ```
  - Expected: Dashboard loads, Services tab visible, no console errors
  - If pass → Step 45
  - If fail → Step 44D

- [ ] **Step 44D** [DEBUG] ~5m: **Debug UI rendering issues**
  ```bash
  curl -s http://localhost:8080/js/services.js | head -20
  curl -s http://localhost:8080/css/services.css | head -20
  curl -s http://localhost:8080/api/v1/services
  ```
  - Check if files load, API responds
  - Fix any 404 errors
  - If resolved → Step 45
  - If not → Document issues

- [ ] **Step 45** [COMMIT] ~3m: **Commit Phase 3a: Services Tab HTML/CSS/JS**
  ```bash
  cd /opt/unheaded/dashboard-backend && git add -A && git commit -m "S47 Phase 3a: Services tab UI (The Armory) with service cards and auto-refresh"
  ```
  - Verify commit succeeds
  - If pass → Step 46
  - If fail → Resolve conflicts and retry

- [ ] **Step 46** [TEST] ~10m: **Test service card interactions**
  - Manual test flow:
    1. Open dashboard in browser
    2. Click "Services" tab
    3. Verify services load (wotan, captain, dashboard, gateway, kanban, trace-collector)
    4. Click "Health Check" button → should show health status
    5. Click "Config" button → should show config path
    6. Click "Restart" button → should prompt, then show confirmation
  - Document results in `/tmp/ui-test-results.txt`
  - If all work → Step 47
  - If issues → Step 46D

- [ ] **Step 46D** [DEBUG] ~8m: **Debug interactive feature issues**
  ```bash
  curl -s http://localhost:8080/api/v1/services | jq '.' || echo "API error"
  curl -s http://localhost:8080/api/v1/services/wotan | jq '.' || echo "Single service error"
  ```
  - Check API responses
  - Fix handler routing if needed
  - Verify event listeners attached correctly
  - If resolved → Step 47
  - If not → Document and continue

- [ ] **Step 47** [TEST] ~5m: **Test auto-refresh functionality**
  - Start Services tab
  - Verify service list refreshes every 10s
  - Check console for any fetch errors
  - Note behavior in `/tmp/refresh-test.txt`
  - If working → Step 48
  - If not → Adjust interval and retry

- [ ] **Step 48** [TEST] ~5m: **Test empty state and error handling**
  - Stop dashboard backend
  - Verify Services tab shows "Error" or "No services"
  - Restart backend
  - Verify services reappear
  - Note behavior
  - If working → Step 49
  - If not → Adjust error handling

- [ ] **Step 49** [CHECKPOINT] ~2m: **Phase 3 checkpoint**
  - Verify: Services tab renders, service cards display correctly, auto-refresh works, design system applied
  - Create checkpoint: `/tmp/phase3-complete.txt`
  - If complete → Proceed to PHASE 4
  - If incomplete → Fix remaining items and retry

---

## PHASE 4: INFRASTRUCTURE TAB UI - "THE FORGE" (Steps 50-80)

Build the Infrastructure tab for container runtime and IaC status.

- [ ] **Step 50** [CODE] ~10m: **Create dashboard/js/infrastructure.js**
  ```bash
  cat > /opt/unheaded/dashboard/js/infrastructure.js << 'EOF'
  // Infrastructure Tab ("The Forge") - Infrastructure management UI

  class InfrastructureTab {
    constructor() {
      this.apiBase = '/api/v1';
      this.refreshInterval = 15000; // 15 seconds
      this.refreshHandle = null;
      this.infrastructure = null;
      this.containers = [];
    }

    async init() {
      const tabButton = document.querySelector('[data-tab="infrastructure"]');
      if (tabButton) {
        tabButton.addEventListener('click', () => this.show());
      }
    }

    async show() {
      const panel = document.querySelector('[data-panel="infrastructure"]');
      if (!panel) {
        this.createPanel();
      }
      this.startRefresh();
    }

    createPanel() {
      const panel = document.createElement('div');
      panel.id = 'infrastructure-panel';
      panel.setAttribute('data-panel', 'infrastructure');
      panel.className = 'tab-panel';

      panel.innerHTML = `
        <div class="infrastructure-container">
          <div class="infrastructure-header">
            <h2>The Forge</h2>
            <button id="refresh-infra-btn" class="btn btn-secondary">Refresh</button>
          </div>

          <div class="infra-section">
            <h3>Runtime Status</h3>
            <div id="infra-status" class="status-overview">
              <div class="loading">Loading...</div>
            </div>
          </div>

          <div class="infra-section">
            <h3>Containers</h3>
            <div id="containers-list" class="containers-grid">
              <div class="loading">Loading...</div>
            </div>
          </div>

          <div class="infra-section">
            <h3>Infrastructure as Code</h3>
            <div id="iac-selector" class="iac-controls">
              <label>IaC Backend:</label>
              <select id="iac-backend">
                <option value="ansible">Ansible</option>
                <option value="terraform">Terraform</option>
                <option value="puppet">Puppet</option>
                <option value="k8s">Kubernetes</option>
                <option value="chef">Chef</option>
                <option value="salt">SaltStack</option>
              </select>
              <button id="generate-config-btn" class="btn btn-primary">Generate Config</button>
            </div>
          </div>

          <div class="infra-section">
            <h3>Helm Deployments</h3>
            <div id="helm-status" class="helm-grid">
              <div class="loading">Loading...</div>
            </div>
          </div>
        </div>
      `;

      const tabsContainer = document.querySelector('.tabs-container') ||
                            document.querySelector('.dashboard-content');
      if (tabsContainer) {
        tabsContainer.appendChild(panel);
      }

      document.getElementById('refresh-infra-btn')?.addEventListener('click', () => this.refresh());
      document.getElementById('generate-config-btn')?.addEventListener('click', () => this.generateConfig());
    }

    async loadInfrastructure() {
      try {
        const statusResp = await fetch(`${this.apiBase}/infrastructure`);
        if (!statusResp.ok) throw new Error(`HTTP ${statusResp.statusCode}`);
        this.infrastructure = await statusResp.json();

        const containersResp = await fetch(`${this.apiBase}/infrastructure/containers`);
        if (!containersResp.ok) throw new Error(`HTTP ${containersResp.status}`);
        this.containers = await containersResp.json();

        this.renderInfrastructure();
      } catch (error) {
        console.error('Failed to load infrastructure:', error);
        this.renderError(error.message);
      }
    }

    renderInfrastructure() {
      this.renderStatus();
      this.renderContainers();
      this.renderHelmStatus();
    }

    renderStatus() {
      const container = document.getElementById('infra-status');
      if (!container || !this.infrastructure) return;

      const statusClass = this.infrastructure.status === 'healthy' ? 'status-healthy' : 'status-warning';

      container.innerHTML = `
        <div class="status-card ${statusClass}">
          <div class="status-header">
            <span class="status-indicator ${statusClass}"></span>
            <span class="status-text">${this.infrastructure.status || 'unknown'}</span>
          </div>
          <div class="status-details">
            <div class="status-item">
              <span class="label">Runtime:</span>
              <span class="value">${this.infrastructure.runtime}</span>
            </div>
            <div class="status-item">
              <span class="label">Containers:</span>
              <span class="value">${this.infrastructure.container_count}</span>
            </div>
            <div class="status-item">
              <span class="label">Total Memory:</span>
              <span class="value">${this.infrastructure.total_memory}</span>
            </div>
            <div class="status-item">
              <span class="label">Total CPU:</span>
              <span class="value">${this.infrastructure.total_cpu}</span>
            </div>
          </div>
        </div>
      `;
    }

    renderContainers() {
      const container = document.getElementById('containers-list');
      if (!container) return;

      if (this.containers.length === 0) {
        container.innerHTML = '<div class="empty-state">No containers running</div>';
        return;
      }

      container.innerHTML = this.containers
        .map(cont => this.createContainerCard(cont))
        .join('');
    }

    createContainerCard(container) {
      const statusClass = container.status === 'running' ? 'status-running' : 'status-stopped';

      return `
        <div class="container-card" data-container="${container.id}">
          <div class="card-header">
            <h4>${container.name}</h4>
            <span class="status-badge ${statusClass}">${container.status}</span>
          </div>
          <div class="card-body">
            <div class="detail">
              <span class="label">Image:</span>
              <span class="value">${container.image}</span>
            </div>
            <div class="detail">
              <span class="label">Memory:</span>
              <span class="value">${container.memory}</span>
            </div>
            <div class="detail">
              <span class="label">CPU:</span>
              <span class="value">${container.cpu}</span>
            </div>
          </div>
        </div>
      `;
    }

    renderHelmStatus() {
      const container = document.getElementById('helm-status');
      if (!container) return;

      // Stub: Mock helm deployments
      const deployments = [
        { name: 'unheaded-core', status: 'deployed', version: '1.0.0' },
        { name: 'monitoring', status: 'deployed', version: '0.8.1' },
      ];

      container.innerHTML = deployments
        .map(dep => `
          <div class="helm-card">
            <h4>${dep.name}</h4>
            <p><strong>Status:</strong> ${dep.status}</p>
            <p><strong>Version:</strong> ${dep.version}</p>
          </div>
        `)
        .join('');
    }

    generateConfig() {
      const backend = document.getElementById('iac-backend')?.value || 'ansible';
      alert(`Generate config: Would call IaC renderer for ${backend}`);
      // In production, this would call a backend endpoint that generates IaC config
    }

    async refresh() {
      await this.loadInfrastructure();
    }

    startRefresh() {
      this.loadInfrastructure();

      if (this.refreshHandle) clearInterval(this.refreshHandle);
      this.refreshHandle = setInterval(() => this.refresh(), this.refreshInterval);
    }

    stopRefresh() {
      if (this.refreshHandle) {
        clearInterval(this.refreshHandle);
        this.refreshHandle = null;
      }
    }

    renderError(message) {
      const statusContainer = document.getElementById('infra-status');
      if (statusContainer) {
        statusContainer.innerHTML = `<div class="error-state">Error: ${message}</div>`;
      }
    }
  }

  // Initialize on page load
  document.addEventListener('DOMContentLoaded', async () => {
    const infraTab = new InfrastructureTab();
    await infraTab.init();
  });
  EOF
  ```
  - Verify file created
  - If pass → Step 51
  - If fail → Manual creation and retry

- [ ] **Step 51** [CODE] ~10m: **Create CSS for Infrastructure tab - dashboard/css/infrastructure.css**
  ```bash
  cat > /opt/unheaded/dashboard/css/infrastructure.css << 'EOF'
  /* Infrastructure Tab ("The Forge") Styles */

  #infrastructure-panel {
    padding: 20px;
  }

  .infrastructure-container {
    width: 100%;
  }

  .infrastructure-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 30px;
    border-bottom: 2px solid #e0e0e0;
    padding-bottom: 15px;
  }

  .infrastructure-header h2 {
    margin: 0;
    font-size: 1.8rem;
    font-weight: 600;
  }

  .infra-section {
    margin-bottom: 40px;
  }

  .infra-section h3 {
    font-size: 1.3rem;
    color: #333;
    margin-bottom: 15px;
    border-bottom: 1px solid #ddd;
    padding-bottom: 10px;
  }

  /* Status Overview */
  .status-overview {
    display: grid;
    grid-template-columns: 1fr;
    gap: 15px;
  }

  .status-card {
    background: white;
    border: 1px solid #ddd;
    border-radius: 8px;
    padding: 20px;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  }

  .status-card.status-healthy {
    border-left: 4px solid #28a745;
  }

  .status-card.status-warning {
    border-left: 4px solid #ffc107;
  }

  .status-header {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 15px;
    font-size: 1.1rem;
    font-weight: 600;
  }

  .status-indicator {
    width: 12px;
    height: 12px;
    border-radius: 50%;
  }

  .status-indicator.status-healthy {
    background: #28a745;
  }

  .status-indicator.status-warning {
    background: #ffc107;
  }

  .status-details {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 15px;
  }

  .status-item {
    display: flex;
    flex-direction: column;
  }

  .status-item .label {
    font-size: 0.85rem;
    color: #666;
    margin-bottom: 3px;
  }

  .status-item .value {
    font-size: 1.1rem;
    color: #333;
    font-weight: 600;
    font-family: monospace;
  }

  /* Containers Grid */
  .containers-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 15px;
  }

  .container-card {
    background: white;
    border: 1px solid #ddd;
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  }

  .container-card .card-header {
    background: #f5f5f5;
    padding: 12px 15px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid #e0e0e0;
  }

  .container-card h4 {
    margin: 0;
    color: #333;
  }

  .container-card .card-body {
    padding: 15px;
  }

  .container-card .detail {
    display: flex;
    justify-content: space-between;
    margin-bottom: 8px;
    font-size: 0.9rem;
  }

  .container-card .label {
    color: #666;
    font-weight: 600;
  }

  .container-card .value {
    color: #333;
    font-family: monospace;
  }

  /* IaC Controls */
  .iac-controls {
    background: white;
    border: 1px solid #ddd;
    border-radius: 8px;
    padding: 15px;
    display: flex;
    gap: 10px;
    align-items: center;
  }

  .iac-controls label {
    font-weight: 600;
    color: #333;
  }

  .iac-controls select {
    padding: 6px 10px;
    border: 1px solid #ddd;
    border-radius: 4px;
    background: white;
  }

  /* Helm Deployments */
  .helm-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
    gap: 15px;
  }

  .helm-card {
    background: white;
    border: 1px solid #ddd;
    border-radius: 8px;
    padding: 15px;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  }

  .helm-card h4 {
    margin: 0 0 10px 0;
    color: #333;
  }

  .helm-card p {
    margin: 5px 0;
    font-size: 0.9rem;
    color: #666;
  }

  .empty-state,
  .error-state {
    text-align: center;
    padding: 40px 20px;
    color: #999;
    font-size: 1rem;
  }

  .loading {
    text-align: center;
    padding: 20px;
    color: #666;
  }

  /* Status Badges */
  .status-badge {
    padding: 4px 10px;
    border-radius: 4px;
    font-size: 0.85rem;
    font-weight: 600;
  }

  .status-badge.status-running {
    background: #d4edda;
    color: #155724;
  }

  .status-badge.status-stopped {
    background: #f8d7da;
    color: #721c24;
  }

  /* Responsive */
  @media (max-width: 768px) {
    .infrastructure-header {
      flex-direction: column;
      align-items: flex-start;
    }

    .containers-grid {
      grid-template-columns: 1fr;
    }

    .status-details {
      grid-template-columns: 1fr;
    }

    .iac-controls {
      flex-direction: column;
      align-items: flex-start;
    }

    .iac-controls select,
    .iac-controls button {
      width: 100%;
    }
  }
  EOF
  ```
  - Verify file created
  - If pass → Step 52
  - If fail → Manual creation and retry

- [ ] **Step 52** [HTML] ~5m: **Add Infrastructure tab to dashboard/index.html**
  ```bash
  cd /opt/unheaded/dashboard && grep -n "data-tab=" index.html | tail -3
  ```
  - Add Infrastructure tab button after Services tab:
  ```html
  <button class="tab-button" data-tab="infrastructure">
    <span class="icon">🔧</span>
    <span>Infrastructure</span>
  </button>
  ```
  - If added → Step 53
  - If not → Document and continue

- [ ] **Step 53** [HTML] ~3m: **Include infrastructure.js and infrastructure.css in index.html**
  - Add before closing </head>:
  ```html
  <link rel="stylesheet" href="/css/infrastructure.css">
  ```
  - Add before closing </body>:
  ```html
  <script src="/js/infrastructure.js"></script>
  ```
  - If added → Step 54
  - If not → Document and continue

- [ ] **Step 54** [TEST] ~5m: **Verify infrastructure.js syntax**
  ```bash
  node -c /opt/unheaded/dashboard/js/infrastructure.js
  ```
  - Expected: Syntax OK
  - If pass → Step 55
  - If fail → Fix syntax errors and retry

- [ ] **Step 55** [DESIGN] ~8m: **Apply design system to infrastructure.css**
  ```bash
  cd /opt/unheaded/dashboard && grep -r "color\|font\|spacing" css/services.css | head -5
  ```
  - Ensure infrastructure.css matches services.css styling
  - Use consistent color palette and typography
  - If applied → Step 56
  - If not → Document differences

- [ ] **Step 56** [MANUAL] ~8m: **Manual test: Infrastructure tab UI**
  ```bash
  curl -s http://localhost:8080/api/v1/infrastructure | jq '.' || echo "API error"
  curl -s http://localhost:8080/api/v1/infrastructure/containers | jq '.' || echo "API error"
  ```
  - Load dashboard in browser
  - Click "Infrastructure" tab
  - Verify status cards, containers list, IaC selector load
  - Note results
  - If working → Step 57
  - If fail → Step 56D

- [ ] **Step 56D** [DEBUG] ~5m: **Debug infrastructure UI issues**
  ```bash
  curl -s http://localhost:8080/js/infrastructure.js | head -20
  curl -s http://localhost:8080/css/infrastructure.css | head -20
  ```
  - Check file loading
  - Fix any 404 errors
  - Verify API endpoints respond
  - If resolved → Step 57
  - If not → Document and continue

- [ ] **Step 57** [TEST] ~5m: **Test IaC backend selector**
  - Open Infrastructure tab
  - Click "Generate Config" button
  - Verify alert or output
  - Note behavior
  - If working → Step 58
  - If fail → Check click handler

- [ ] **Step 58** [TEST] ~5m: **Test refresh functionality**
  - Open Infrastructure tab
  - Click "Refresh" button
  - Verify containers list updates
  - Check auto-refresh every 15s
  - If working → Step 59
  - If fail → Check timer logic

- [ ] **Step 59** [COMMIT] ~3m: **Commit Phase 4: Infrastructure Tab UI**
  ```bash
  cd /opt/unheaded/dashboard-backend && git add -A && git commit -m "S47 Phase 4: Infrastructure tab UI (The Forge) with container status and IaC controls"
  ```
  - Verify commit succeeds
  - If pass → Step 60
  - If fail → Resolve conflicts and retry

- [ ] **Step 60** [CHECKPOINT] ~2m: **Phase 4 checkpoint**
  - Verify: Infrastructure tab renders, status cards display, containers list works, IaC selector functional
  - Create checkpoint: `/tmp/phase4-complete.txt`
  - If complete → Proceed to PHASE 5
  - If incomplete → Fix remaining items and retry

---

## PHASE 5: INTEGRATION & VERIFICATION (Steps 61-75)

Final integration testing, verification of all tabs, and full test suite.

- [ ] **Step 61** [TEST] ~5m: **Verify all 7 tabs render**
  ```bash
  curl -s http://localhost:8080/ | grep -o 'data-tab="[^"]*"' || echo "Tabs not found"
  ```
  - Expected tabs: Packet Flow, Trace, Latency, Doom, Services, Infrastructure, Logs
  - Document which tabs are present
  - If all present → Step 62
  - If missing → Step 61D

- [ ] **Step 61D** [DEBUG] ~5m: **Debug missing tabs**
  ```bash
  cd /opt/unheaded/dashboard && grep -n "data-tab" index.html | wc -l
  ```
  - Verify all tab buttons added to index.html
  - Check for syntax errors
  - Add missing tabs if needed
  - If resolved → Step 62
  - If not → Document and continue

- [ ] **Step 62** [TEST] ~10m: **Test all tabs click and render**
  - Manual browser test:
    1. Load dashboard
    2. Click each tab: Packet Flow, Trace, Latency, Doom, Services, Infrastructure, Logs
    3. Verify no console errors
    4. Verify content loads for each tab
  - Document results in `/tmp/tabs-test.txt`
  - If all work → Step 63
  - If issues → Step 62D

- [ ] **Step 62D** [DEBUG] ~5m: **Debug tab rendering issues**
  ```bash
  # Open browser console and check for errors
  curl -s http://localhost:8080/ | grep -c "script" || echo "Scripts not loaded"
  ```
  - Check JavaScript console for errors
  - Verify all .js files load (200 status)
  - Verify all .css files load (200 status)
  - If resolved → Step 63
  - If not → Document issues

- [ ] **Step 63** [TEST] ~8m: **Test service cards with sample YAML configs**
  ```bash
  ls -la /opt/unheaded/*/config.yaml | wc -l
  ```
  - Click "Services" tab
  - Verify all 6 services load (wotan, captain, dashboard, gateway, kanban, trace-collector)
  - Verify service card details (name, version, port, protocol, dependencies)
  - Note results
  - If working → Step 64
  - If fail → Step 63D

- [ ] **Step 63D** [DEBUG] ~5m: **Debug services loading**
  ```bash
  curl -s http://localhost:8080/api/v1/services | jq '.[] | .name'
  ```
  - Check API returns all service names
  - Verify YAML files exist and are readable
  - Check config loader logic
  - If resolved → Step 64
  - If not → Document and continue

- [ ] **Step 64** [TEST] ~8m: **Test infrastructure tab with mock data**
  - Click "Infrastructure" tab
  - Verify status card displays (runtime, container count, memory, CPU)
  - Verify containers list shows test containers (wotan, dashboard)
  - Verify Helm deployments section populated
  - Verify IaC selector dropdown works
  - Note results
  - If working → Step 65
  - If fail → Step 64D

- [ ] **Step 64D** [DEBUG] ~5m: **Debug infrastructure loading**
  ```bash
  curl -s http://localhost:8080/api/v1/infrastructure | jq '.'
  curl -s http://localhost:8080/api/v1/infrastructure/containers | jq '.[] | .name'
  ```
  - Check API responses
  - Verify mock data is correct
  - Check JavaScript render logic
  - If resolved → Step 65
  - If not → Document and continue

- [ ] **Step 65** [TEST] ~10m: **Verify no console errors on any tab**
  - Open browser developer console
  - Click each tab and wait for content to load
  - Document any console errors or warnings
  - Expected: No 404 errors, no JavaScript exceptions
  - If clean → Step 66
  - If issues → Step 65D

- [ ] **Step 65D** [DEBUG] ~5m: **Debug console errors**
  ```bash
  # Check 404 responses
  curl -I http://localhost:8080/js/services.js
  curl -I http://localhost:8080/js/infrastructure.js
  curl -I http://localhost:8080/css/services.css
  curl -I http://localhost:8080/css/infrastructure.css
  ```
  - Verify all resource URLs correct and accessible
  - Fix file paths if needed
  - If resolved → Step 66
  - If not → Document and continue

- [ ] **Step 66** [TEST] ~10m: **Run full test suite with race detector**
  ```bash
  cd /opt/unheaded/dashboard-backend && go test ./... -race -timeout 60s -v 2>&1 | tee /tmp/full-tests.log
  tail -30 /tmp/full-tests.log
  ```
  - Expected: All tests pass
  - Count passes and failures
  - If all pass → Step 67
  - If fail → Step 66D

- [ ] **Step 66D** [DEBUG] ~8m: **Triage test failures**
  ```bash
  grep "FAIL:" /tmp/full-tests.log || echo "All tests passed"
  ```
  - Review failures
  - Assess if critical or known issues
  - Fix critical failures if possible
  - Document remaining failures
  - If critical issues resolved → Step 67
  - If not → Continue to Step 67 anyway

- [ ] **Step 67** [TEST] ~8m: **Integration test: Services API roundtrip**
  ```bash
  # Test full roundtrip
  curl -s http://localhost:8080/api/v1/services | jq '.[] | {name, port, status}'
  ```
  - Verify:
    1. YAML configs load
    2. ServiceConfigLoader populates
    3. API returns data
    4. Frontend fetches and renders
  - Document flow in `/tmp/integration-flow.txt`
  - If working → Step 68
  - If fail → Fix critical path issues

- [ ] **Step 68** [TEST] ~8m: **Integration test: Infrastructure API roundtrip**
  ```bash
  curl -s http://localhost:8080/api/v1/infrastructure | jq '.'
  curl -s http://localhost:8080/api/v1/infrastructure/containers | jq '.[] | {name, status}'
  ```
  - Verify:
    1. Infrastructure handler returns mock status
    2. Containers handler returns mock data
    3. Frontend fetches and renders both
  - Document flow
  - If working → Step 69
  - If fail → Fix critical path issues

- [ ] **Step 69** [PERF] ~5m: **Check for performance issues**
  ```bash
  # Monitor memory/CPU usage while running dashboard
  # Stress test: rapid tab switching and refreshes
  time curl -s http://localhost:8080/api/v1/services > /dev/null
  time curl -s http://localhost:8080/api/v1/infrastructure/containers > /dev/null
  ```
  - Verify responses are < 100ms
  - Monitor dashboard for memory leaks
  - Note if auto-refresh causes high CPU
  - If acceptable → Step 70
  - If issues → Optimize and retry

- [ ] **Step 70** [LINT] ~5m: **Check Go code style**
  ```bash
  cd /opt/unheaded/dashboard-backend && go fmt ./...
  cd /opt/unheaded/dashboard-backend && go vet ./...
  ```
  - Expected: No formatting issues, no vet warnings
  - If clean → Step 71
  - If issues → Fix and retry

- [ ] **Step 71** [BUILD] ~5m: **Final clean build**
  ```bash
  cd /opt/unheaded/dashboard-backend && go clean && go build -v
  ```
  - Expected: Clean build with no errors
  - If pass → Step 72
  - If fail → Fix build errors

- [ ] **Step 72** [DOCS] ~5m: **Document S47 completion**
  ```bash
  cat > /tmp/s47-completion-report.txt << 'EOF'
  S47 Service & Infrastructure Management Sprint

  COMPLETION SUMMARY

  Phases Completed:
  - Phase 0: Environment & Baseline
  - Phase 1: YAML Config System
  - Phase 2: Service API Endpoints
  - Phase 3: Services Tab UI ("The Armory")
  - Phase 4: Infrastructure Tab UI ("The Forge")
  - Phase 5: Integration & Verification

  Deliverables:
  - ServiceConfig YAML schema and loader
  - Config file watcher (hot-reload capable)
  - Service API endpoints: /api/v1/services, /api/v1/services/{name}, /api/v1/services/{name}/health, /api/v1/services/{name}/restart
  - Infrastructure API endpoints: /api/v1/infrastructure, /api/v1/infrastructure/containers
  - Services tab UI with service cards, health checks, restart actions
  - Infrastructure tab UI with status overview, containers list, IaC backend selector
  - Integration with existing dashboard tabs (7 tabs total)
  - Test coverage for YAML loading, API handlers, UI functionality

  Test Results:
  - go test ./... -race: [PASS/FAIL - document results]
  - Manual UI testing: All tabs render, no console errors
  - API roundtrip testing: YAML->Loader->API->UI working

  Known Issues/Limitations:
  - Restart service is stub (logs request, doesn't signal daemon)
  - Container data is mocked (not reading actual Docker/LXD)
  - IaC config generation not yet implemented (shows alert)
  - Health checks proxy not yet connected to real endpoints

  Next Steps (S48):
  - Implement real service restart signaling
  - Connect to real Docker/LXD container APIs
  - Implement IaC config generation (S43 integration)
  - Real health check endpoint proxying
  - Performance optimization and monitoring

  EOF
  cat /tmp/s47-completion-report.txt
  ```
  - Verify report created
  - If pass → Step 73
  - If fail → Manual creation

- [ ] **Step 73** [COMMIT] ~3m: **Final commit: S47 complete**
  ```bash
  cd /opt/unheaded/dashboard-backend && git add -A && git commit -m "S47 complete: Service & Infrastructure Management UI with YAML config system"
  ```
  - Verify commit succeeds
  - If pass → Step 74
  - If fail → Resolve conflicts and retry

- [ ] **Step 74** [TAG] ~2m: **Create git tag for S47**
  ```bash
  cd /opt/unheaded/dashboard-backend && git tag -a s47-complete -m "S47: Service Management & Infrastructure UI sprint complete"
  ```
  - Verify tag created
  - If pass → Step 75
  - If fail → Continue anyway

- [ ] **Step 75** [CHECKPOINT] ~2m: **Final S47 Checkpoint**
  - Verify: All phases complete, all tests passing, no critical errors
  - Create final checkpoint: `/tmp/s47-final-checkpoint.txt`
  - Log summary:
    - Lines of code added
    - Files modified
    - API endpoints implemented
    - UI tabs added
    - Test coverage
  - If complete → S47 COMPLETE
  - If incomplete → Fix remaining items

---

## APPENDIX A: EMERGENCY PROCEDURES

### If Dashboard Build Fails

1. Check Go version: `go version` (need 1.24+)
2. Clean cache: `go clean -modcache`
3. Reinstall dependencies: `go mod tidy && go mod download`
4. Check for syntax errors: `go build -v`
5. If persists: Escalate to sprint lead

### If Tests Fail

1. Run single package: `go test ./pkg/discovery -v`
2. Check for race conditions: `go test ./... -race -count=3`
3. Review test output: `go test ./... -v 2>&1 | grep FAIL`
4. Fix test data/mocks first
5. If > 5 failures critical: Escalate

### If API Endpoints Not Responding

1. Verify server is running: `curl http://localhost:8080/health` or similar
2. Check route registration in main.go
3. Verify handlers are imported and initialized
4. Check request paths match registered patterns
5. Test with curl: `curl -v http://localhost:8080/api/v1/services`

### If UI Tab Not Rendering

1. Verify HTML tab button exists
2. Check JavaScript console for errors: DevTools → Console
3. Verify .js and .css files load: DevTools → Network (should be 200)
4. Check lazy-init system: tab click should call `init()` then `show()`
5. Verify API endpoint responds: curl `http://localhost:8080/api/v1/...`

### If Auto-Refresh Not Working

1. Check browser console for fetch errors
2. Verify API endpoint responds correctly
3. Check interval is set (Services: 10s, Infrastructure: 15s)
4. Verify callback functions attached to tab
5. Test manually: `setInterval(() => fetch(...), 10000)` in console

---

## APPENDIX B: AGENT MATRIX

| Phase | Task | Owner | Status | Dependencies |
|-------|------|-------|--------|--------------|
| 0 | Environment setup | Agent | - | None |
| 1 | YAML config system | Agent | - | Phase 0 |
| 2 | Service API endpoints | Agent | - | Phase 1 |
| 3 | Services tab UI | Agent | - | Phase 1, 2 |
| 4 | Infrastructure tab UI | Agent | - | Phase 1, 2 |
| 5 | Integration & verification | Agent | - | Phases 3-4 |

---

## APPENDIX C: QUICK REFERENCE

### ServiceConfig YAML Schema
```yaml
service:
  name: service-name
  version: v1.0.0
  description: "Service description"
  port: 8080
  protocol: grpc|http|tcp
  health_check:
    endpoint: /health
    interval: 5s
    timeout: 2s
  dependencies: [dep1, dep2]
  config_path: /etc/service/config.yaml
  restart_policy: on-failure|always
  resource_limits:
    cpu: "1"
    memory: "512Mi"
```

### API Endpoints
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/services` | GET | List all services |
| `/api/v1/services/{name}` | GET | Get single service |
| `/api/v1/services/{name}/health` | GET | Proxy health check |
| `/api/v1/services/{name}/restart` | POST | Request restart |
| `/api/v1/infrastructure` | GET | Infrastructure status |
| `/api/v1/infrastructure/containers` | GET | List containers |

### Port Allocations (from pkg/ports/)
```
Dashboard UI: 8080
Dashboard API: 8081
Wotan (message bus): 6379
Captain (orchestration): 7890
Gateway (API gateway): 9000
Kanban (task mgmt): 7000
Trace-collector: 4317
```

### Sample Service Configs Location
```
/opt/unheaded/wotan/config.yaml
/opt/unheaded/captain/config.yaml
/opt/unheaded/dashboard/config.yaml
/opt/unheaded/gateway/config.yaml
/opt/unheaded/kanban/config.yaml
/opt/unheaded/trace-collector/config.yaml
```

### Key File Paths
```
Dashboard backend: /opt/unheaded/dashboard-backend/
Services discovery: pkg/discovery/{config.go, loader.go, yaml_loader.go, watcher.go}
API handlers: pkg/api/{services.go, infrastructure.go, health.go, restart.go}
Frontend: /opt/unheaded/dashboard/{js/services.js, js/infrastructure.js, css/services.css, css/infrastructure.css}
Sample configs: /opt/unheaded/{service}/config.yaml
```

### Testing Commands
```bash
# Full test suite with race detection
go test ./... -race -timeout 60s -v

# Individual package tests
go test ./pkg/discovery -v
go test ./pkg/api -v

# API endpoint testing
curl http://localhost:8080/api/v1/services | jq
curl http://localhost:8080/api/v1/infrastructure | jq

# JavaScript syntax check
node -c /path/to/script.js

# Go code formatting and linting
go fmt ./...
go vet ./...
```

---

**END OF S47 BATTLE PLAN**

Total estimated steps: 75
Estimated duration: 6-8 hours
Last updated: 2026-02-24
