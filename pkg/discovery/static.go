// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// StaticConfig provides a last-resort static service map.
// Loaded from configs/services.yaml when Wotan is unavailable.
type StaticConfig struct {
	Services map[string]StaticEntry `yaml:"services"`
}

// StaticEntry is a service entry in the static config.
type StaticEntry struct {
	Host  string `yaml:"host"`
	Port  int    `yaml:"port"`
	Proto string `yaml:"proto"`
}

// LoadStaticConfig loads the static service map from a YAML file.
func LoadStaticConfig(path string) (*StaticConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied configuration file path
	if err != nil {
		return nil, fmt.Errorf("read static config: %w", err)
	}

	var cfg StaticConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse static config: %w", err)
	}

	return &cfg, nil
}

// Resolve looks up a service by name in the static config.
func (sc *StaticConfig) Resolve(name string) (*ServiceEntry, error) {
	entry, ok := sc.Services[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s (not in static config)", ErrServiceNotFound, name)
	}

	host := entry.Host
	if host == "" {
		host = "localhost"
	}

	return &ServiceEntry{
		Name:    name,
		Address: fmt.Sprintf("%s:%d", host, entry.Port),
		Port:    entry.Port,
		Health:  "unknown",
	}, nil
}

// ResolveAll returns all services in the static config.
func (sc *StaticConfig) ResolveAll() []ServiceEntry {
	entries := make([]ServiceEntry, 0, len(sc.Services))
	for name, e := range sc.Services {
		host := e.Host
		if host == "" {
			host = "localhost"
		}
		entries = append(entries, ServiceEntry{
			Name:    name,
			Address: fmt.Sprintf("%s:%d", host, e.Port),
			Port:    e.Port,
			Health:  "unknown",
		})
	}
	return entries
}
