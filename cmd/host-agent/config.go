// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the huginn YAML configuration. All fields are optional; missing
// values fall back to the defaults defined in defaultConfig(). CLI flags
// override whatever the config file sets.
//
// Config file path: /etc/huginn.yaml (override with -config flag).
type Config struct {
	Listen    string `yaml:"listen"`
	HostLabel string `yaml:"host_label"`

	Collection CollectionConfig `yaml:"collection"`
	Sinks      SinksConfig      `yaml:"sinks"`
	Databases  []DBConfig       `yaml:"databases"`
}

type CollectionConfig struct {
	// Interval is the default polling interval for all collectors.
	Interval duration `yaml:"interval"`
	// DiskInterval overrides Interval for filesystem stats (disks change slowly).
	DiskInterval duration `yaml:"disk_interval"`
	// ProcessInterval overrides Interval for process counts.
	ProcessInterval duration `yaml:"process_interval"`
}

type SinksConfig struct {
	VictoriaMetrics VMSinkConfig `yaml:"victoria_metrics"`
}

type VMSinkConfig struct {
	Enabled      bool     `yaml:"enabled"`
	URL          string   `yaml:"url"`
	PushInterval duration `yaml:"push_interval"`
}

// duration is a yaml-unmarshallable wrapper around time.Duration.
type duration struct{ time.Duration }

func (d *duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func defaultConfig() Config {
	return Config{
		Listen:    ":9110",
		HostLabel: "",
		Collection: CollectionConfig{
			Interval:        duration{10 * time.Second},
			DiskInterval:    duration{30 * time.Second},
			ProcessInterval: duration{15 * time.Second},
		},
		Sinks: SinksConfig{
			VictoriaMetrics: VMSinkConfig{
				Enabled:      true,
				URL:          "http://localhost:8428",
				PushInterval: duration{10 * time.Second},
			},
		},
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // config file is optional
		}
		return cfg, err
	}
	return cfg, yaml.Unmarshal(data, &cfg)
}
