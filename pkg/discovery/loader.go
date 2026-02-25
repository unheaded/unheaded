package discovery

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// ServiceConfigLoader provides thread-safe caching and management of service configs.
// It loads configs from the filesystem via LoadServiceConfig/LoadServiceDirectory
// and keeps an in-memory copy for fast lookups by the API layer.
type ServiceConfigLoader struct {
	configs map[string]*ServiceConfig
	mu      sync.RWMutex
}

// NewServiceConfigLoader creates a new empty config loader.
func NewServiceConfigLoader() *ServiceConfigLoader {
	return &ServiceConfigLoader{
		configs: make(map[string]*ServiceConfig),
	}
}

// LoadFromDirectory scans a directory for service config.yaml files and caches them.
// Existing configs for services not found in the directory are preserved.
func (l *ServiceConfigLoader) LoadFromDirectory(dir string) error {
	configs, err := LoadServiceDirectory(dir)
	if err != nil {
		return fmt.Errorf("loading service directory %s: %w", dir, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, cfg := range configs {
		l.configs[cfg.Service.Name] = cfg
	}

	return nil
}

// LoadFromManifest loads configs from a bundled YAML manifest (services.yaml).
func (l *ServiceConfigLoader) LoadFromManifest(data []byte) error {
	configs, err := LoadBundledManifest(data)
	if err != nil {
		return fmt.Errorf("loading bundled manifest: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, cfg := range configs {
		l.configs[cfg.Service.Name] = cfg
	}

	return nil
}

// GetConfig returns a single service config by name, or nil if not found.
func (l *ServiceConfigLoader) GetConfig(name string) *ServiceConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.configs[name]
}

// GetAllConfigs returns a shallow copy of all cached configs.
func (l *ServiceConfigLoader) GetAllConfigs() map[string]*ServiceConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make(map[string]*ServiceConfig, len(l.configs))
	for k, v := range l.configs {
		out[k] = v
	}
	return out
}

// UpdateConfig adds or replaces a config in the cache.
func (l *ServiceConfigLoader) UpdateConfig(name string, cfg *ServiceConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.configs[name] = cfg
}

// DeleteConfig removes a service config from the cache.
func (l *ServiceConfigLoader) DeleteConfig(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.configs[name]; !ok {
		return false
	}
	delete(l.configs, name)
	return true
}

// Count returns the number of cached configs.
func (l *ServiceConfigLoader) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.configs)
}

// MarshalConfig serializes a ServiceConfig to YAML bytes.
func MarshalConfig(cfg *ServiceConfig) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	return data, nil
}
