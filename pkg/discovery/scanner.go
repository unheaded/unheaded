// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultBasePath is the default directory for convention-based service configs.
const DefaultBasePath = "/opt/unheaded"

// Scanner implements convention-based service discovery (Layer 1).
// It scans a base directory for service config files at:
//
//	<basePath>/<service>/config.yaml
//
// Each config.yaml must declare the service name, port, and protocol.
type Scanner struct {
	basePath string
}

// scannerServiceConfig represents a service config.yaml discovered by the scanner.
type scannerServiceConfig struct {
	Name   string `yaml:"name"`
	Port   int    `yaml:"port"`
	Proto  string `yaml:"proto"`
	Health string `yaml:"health"` // health endpoint path, e.g. "/health"
}

// NewScanner creates a new convention-based scanner rooted at basePath.
// If basePath is empty, DefaultBasePath is used.
func NewScanner(basePath string) *Scanner {
	if basePath == "" {
		basePath = DefaultBasePath
	}
	return &Scanner{basePath: basePath}
}

// Scan walks the base directory looking for service config.yaml files.
// Returns all discovered services. Directories without a valid config.yaml
// are silently skipped.
func (s *Scanner) Scan() ([]ServiceEntry, error) {
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("scan base path %s: %w", s.basePath, err)
	}

	var services []ServiceEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configPath := filepath.Join(s.basePath, entry.Name(), "config.yaml")
		svc, err := s.loadServiceConfig(configPath)
		if err != nil {
			continue // skip invalid/missing configs
		}

		name := svc.Name
		if name == "" {
			name = entry.Name() // fall back to directory name
		}

		services = append(services, ServiceEntry{
			Name:    name,
			Address: fmt.Sprintf("localhost:%d", svc.Port),
			Port:    svc.Port,
			Health:  "unknown",
		})
	}
	return services, nil
}

// ScanService looks up a single service by name using the convention path.
func (s *Scanner) ScanService(name string) (*ServiceEntry, error) {
	configPath := filepath.Join(s.basePath, name, "config.yaml")
	svc, err := s.loadServiceConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s (not found via convention scan)", ErrServiceNotFound, name)
	}

	svcName := svc.Name
	if svcName == "" {
		svcName = name
	}

	return &ServiceEntry{
		Name:    svcName,
		Address: fmt.Sprintf("localhost:%d", svc.Port),
		Port:    svc.Port,
		Health:  "unknown",
	}, nil
}

// loadServiceConfig reads and parses a config.yaml file.
func (s *Scanner) loadServiceConfig(path string) (*scannerServiceConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied configuration file path
	if err != nil {
		return nil, err
	}

	var cfg scannerServiceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Port <= 0 {
		return nil, fmt.Errorf("invalid or missing port in %s", path)
	}
	return &cfg, nil
}
