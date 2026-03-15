// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package iac

import "fmt"

// ChefRenderer produces Chef cookbooks and recipes.
type ChefRenderer struct{}

func (r *ChefRenderer) Backend() Backend { return BackendChef }

func (r *ChefRenderer) Render(config ServiceConfig) (*RenderOutput, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	out := &RenderOutput{
		Backend: BackendChef,
		Files:   make(map[string]string),
	}

	out.Files[fmt.Sprintf("cookbooks/%s/recipes/default.rb", config.Name)] = r.renderRecipe(config)
	out.Files[fmt.Sprintf("cookbooks/%s/attributes/default.rb", config.Name)] = r.renderAttributes(config)
	out.Files[fmt.Sprintf("cookbooks/%s/metadata.rb", config.Name)] = r.renderMetadata(config)

	return out, nil
}

func (r *ChefRenderer) RenderAll(configs []ServiceConfig) (*RenderOutput, error) {
	out := &RenderOutput{
		Backend: BackendChef,
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

	out.Files["Policyfile.rb"] = r.renderPolicyfile(configs)
	return out, nil
}

func (r *ChefRenderer) Validate(config ServiceConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	out, err := r.Render(config)
	if err != nil {
		return fmt.Errorf("validate render: %w", err)
	}
	return ValidateRenderedOutput(out, config)
}

func (r *ChefRenderer) Diff(current, desired ServiceConfig) (string, error) {
	if err := current.Validate(); err != nil {
		return "", fmt.Errorf("current config invalid: %w", err)
	}
	if err := desired.Validate(); err != nil {
		return "", fmt.Errorf("desired config invalid: %w", err)
	}
	return DiffConfigs(current, desired), nil
}

func (r *ChefRenderer) renderRecipe(config ServiceConfig) string {
	binary := config.Binary
	if binary == "" {
		binary = fmt.Sprintf("/opt/unheaded/bin/%s", config.Name)
	}

	return fmt.Sprintf(`#
# Cookbook:: %s
# Recipe:: default
#
# Deploys and configures the Unheaded %s service.

systemd_unit '%s.service' do
  content <<~UNIT
    [Unit]
    Description=Unheaded %s Service
    After=network.target

    [Service]
    Type=simple
    ExecStart=%s
    Restart=always
    RestartSec=5s
    NoNewPrivileges=true
    ProtectSystem=strict
    ProtectHome=true
    PrivateTmp=true

    [Install]
    WantedBy=multi-user.target
  UNIT
  action [:create, :enable, :start]
end

http_request 'health_check_%s' do
  url "http://127.0.0.1:#{node['%s']['port']}%s"
  retry_delay 2
  retries 30
  action :get
  ignore_failure true
end
`, config.Name, config.Name, config.Name, config.Name, binary,
		config.Name, config.Name, config.HealthPath)
}

func (r *ChefRenderer) renderAttributes(config ServiceConfig) string {
	s := fmt.Sprintf(`# Default attributes for %s
default['%s']['port'] = %d
default['%s']['enabled'] = true
`, config.Name, config.Name, config.Port, config.Name)

	for k, v := range config.Environment {
		s += fmt.Sprintf("default['%s']['%s'] = '%s'\n", config.Name, k, v)
	}

	return s
}

func (r *ChefRenderer) renderMetadata(config ServiceConfig) string {
	return fmt.Sprintf(`name '%s'
maintainer 'Unheaded Project'
maintainer_email 'stevie@bellis.tech'
license 'MIT'
description 'Deploys the Unheaded %s service'
version '0.1.0'
chef_version '>= 16.0'

supports 'ubuntu', '>= 22.04'
supports 'debian', '>= 12'
`, config.Name, config.Name)
}

func (r *ChefRenderer) renderPolicyfile(configs []ServiceConfig) string {
	s := `# Policyfile.rb — Unheaded infrastructure policy
name 'unheaded'

default_source :supermarket

`
	for _, cfg := range configs {
		s += fmt.Sprintf("run_list 'recipe[%s::default]'\n", cfg.Name)
	}
	s += "\n"
	for _, cfg := range configs {
		s += fmt.Sprintf("cookbook '%s', path: 'cookbooks/%s'\n", cfg.Name, cfg.Name)
	}
	return s
}
