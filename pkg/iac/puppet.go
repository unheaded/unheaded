// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package iac

import "fmt"

// PuppetRenderer produces Puppet manifests and Hiera data.
type PuppetRenderer struct{}

func (r *PuppetRenderer) Backend() Backend { return BackendPuppet }

func (r *PuppetRenderer) Render(config ServiceConfig) (*RenderOutput, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	out := &RenderOutput{
		Backend: BackendPuppet,
		Files:   make(map[string]string),
	}

	out.Files[fmt.Sprintf("modules/%s/manifests/init.pp", config.Name)] = r.renderManifest(config)
	out.Files[fmt.Sprintf("modules/%s/manifests/config.pp", config.Name)] = r.renderConfig(config)
	out.Files[fmt.Sprintf("data/%s.yaml", config.Name)] = r.renderHiera(config)

	return out, nil
}

func (r *PuppetRenderer) RenderAll(configs []ServiceConfig) (*RenderOutput, error) {
	out := &RenderOutput{
		Backend: BackendPuppet,
		Files:   make(map[string]string),
	}

	for _, cfg := range configs {
		single, err := r.Render(cfg)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", cfg.Name, err)
		}
		for k, v := range single.Files {
			out.Files[k] = v
		}
	}

	out.Files["manifests/site.pp"] = r.renderSite(configs)
	return out, nil
}

func (r *PuppetRenderer) Validate(config ServiceConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	out, err := r.Render(config)
	if err != nil {
		return fmt.Errorf("validate render: %w", err)
	}
	return ValidateRenderedOutput(out, config)
}

func (r *PuppetRenderer) Diff(current, desired ServiceConfig) (string, error) {
	if err := current.Validate(); err != nil {
		return "", fmt.Errorf("current config invalid: %w", err)
	}
	if err := desired.Validate(); err != nil {
		return "", fmt.Errorf("desired config invalid: %w", err)
	}
	return DiffConfigs(current, desired), nil
}

func (r *PuppetRenderer) renderManifest(config ServiceConfig) string {
	binary := config.Binary
	if binary == "" {
		binary = fmt.Sprintf("/opt/unheaded/bin/%s", config.Name)
	}

	return fmt.Sprintf(`# Puppet manifest for %s service
class %s (
  Integer $port = %d,
  String  $binary = '%s',
  Boolean $enabled = true,
) {
  file { '/etc/systemd/system/%s.service':
    ensure  => file,
    content => epp('%s/%s.service.epp'),
    notify  => Service['%s'],
  }

  service { '%s':
    ensure    => running,
    enable    => $enabled,
    subscribe => File['/etc/systemd/system/%s.service'],
  }
}
`, config.Name, config.Name, config.Port, binary,
		config.Name, config.Name, config.Name, config.Name,
		config.Name, config.Name)
}

func (r *PuppetRenderer) renderConfig(config ServiceConfig) string {
	s := fmt.Sprintf(`# Configuration management for %s
class %s::config {
  file { '/opt/unheaded/%s/config.yaml':
    ensure  => file,
    owner   => 'unheaded',
    group   => 'unheaded',
    mode    => '0640',
    content => epp('%s/config.yaml.epp'),
  }
}
`, config.Name, config.Name, config.Name, config.Name)

	return s
}

func (r *PuppetRenderer) renderHiera(config ServiceConfig) string {
	s := fmt.Sprintf(`---
# Hiera data for %s
%s::port: %d
%s::enabled: true
`, config.Name, config.Name, config.Port, config.Name)

	for k, v := range config.Environment {
		s += fmt.Sprintf("%s::%s: '%s'\n", config.Name, k, v)
	}

	return s
}

func (r *PuppetRenderer) renderSite(configs []ServiceConfig) string {
	s := "# Unheaded site manifest\nnode default {\n"
	for _, cfg := range configs {
		s += fmt.Sprintf("  include %s\n", cfg.Name)
	}
	s += "}\n"
	return s
}
