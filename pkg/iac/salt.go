// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package iac

import "fmt"

// SaltRenderer produces Salt states and pillars.
type SaltRenderer struct{}

func (r *SaltRenderer) Backend() Backend { return BackendSalt }

func (r *SaltRenderer) Render(config ServiceConfig) (*RenderOutput, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	out := &RenderOutput{
		Backend: BackendSalt,
		Files:   make(map[string]string),
	}

	out.Files[fmt.Sprintf("salt/%s/init.sls", config.Name)] = r.renderState(config)
	out.Files[fmt.Sprintf("pillar/%s/init.sls", config.Name)] = r.renderPillar(config)

	return out, nil
}

func (r *SaltRenderer) RenderAll(configs []ServiceConfig) (*RenderOutput, error) {
	out := &RenderOutput{
		Backend: BackendSalt,
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

	out.Files["salt/top.sls"] = r.renderTop(configs)
	out.Files["pillar/top.sls"] = r.renderPillarTop(configs)
	return out, nil
}

func (r *SaltRenderer) Validate(config ServiceConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	out, err := r.Render(config)
	if err != nil {
		return fmt.Errorf("validate render: %w", err)
	}
	return ValidateRenderedOutput(out, config)
}

func (r *SaltRenderer) Diff(current, desired ServiceConfig) (string, error) {
	if err := current.Validate(); err != nil {
		return "", fmt.Errorf("current config invalid: %w", err)
	}
	if err := desired.Validate(); err != nil {
		return "", fmt.Errorf("desired config invalid: %w", err)
	}
	return DiffConfigs(current, desired), nil
}

func (r *SaltRenderer) renderState(config ServiceConfig) string {
	binary := config.Binary
	if binary == "" {
		binary = fmt.Sprintf("/opt/unheaded/bin/%s", config.Name)
	}

	s := fmt.Sprintf(`# Salt state for %s service

%s_binary:
  file.managed:
    - name: %s
    - mode: '0755'
    - user: unheaded
    - group: unheaded

%s_systemd_unit:
  file.managed:
    - name: /etc/systemd/system/%s.service
    - contents: |
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
`, config.Name, config.Name, binary,
		config.Name, config.Name, config.Name, binary)

	for k, v := range config.Environment {
		s += fmt.Sprintf("        Environment=%s=%s\n", k, v)
	}

	s += fmt.Sprintf(`
        [Install]
        WantedBy=multi-user.target

%s_service:
  service.running:
    - name: %s
    - enable: True
    - watch:
      - file: %s_systemd_unit
    - require:
      - file: %s_binary
`, config.Name, config.Name, config.Name, config.Name)

	return s
}

func (r *SaltRenderer) renderPillar(config ServiceConfig) string {
	s := fmt.Sprintf(`# Pillar data for %s
%s:
  port: %d
  enabled: true
`, config.Name, config.Name, config.Port)

	if config.GRPCPort > 0 {
		s += fmt.Sprintf("  grpc_port: %d\n", config.GRPCPort)
	}

	if len(config.Environment) > 0 {
		s += "  environment:\n"
		for k, v := range config.Environment {
			s += fmt.Sprintf("    %s: '%s'\n", k, v)
		}
	}

	return s
}

func (r *SaltRenderer) renderTop(configs []ServiceConfig) string {
	s := "base:\n  'unheaded-*':\n"
	for _, cfg := range configs {
		s += fmt.Sprintf("    - %s\n", cfg.Name)
	}
	return s
}

func (r *SaltRenderer) renderPillarTop(configs []ServiceConfig) string {
	s := "base:\n  'unheaded-*':\n"
	for _, cfg := range configs {
		s += fmt.Sprintf("    - %s\n", cfg.Name)
	}
	return s
}
