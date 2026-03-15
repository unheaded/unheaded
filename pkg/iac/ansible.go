// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package iac

import "fmt"

// AnsibleRenderer produces Ansible playbooks, roles, and inventory.
type AnsibleRenderer struct{}

func (r *AnsibleRenderer) Backend() Backend { return BackendAnsible }

func (r *AnsibleRenderer) Render(config ServiceConfig) (*RenderOutput, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	out := &RenderOutput{
		Backend: BackendAnsible,
		Files:   make(map[string]string),
	}

	out.Files[fmt.Sprintf("roles/%s/tasks/main.yml", config.Name)] = r.renderTasks(config)
	out.Files[fmt.Sprintf("roles/%s/defaults/main.yml", config.Name)] = r.renderDefaults(config)
	out.Files[fmt.Sprintf("roles/%s/handlers/main.yml", config.Name)] = r.renderHandlers(config)
	out.Files[fmt.Sprintf("roles/%s/templates/%s.service.j2", config.Name, config.Name)] = r.renderSystemdTemplate(config)

	return out, nil
}

func (r *AnsibleRenderer) RenderAll(configs []ServiceConfig) (*RenderOutput, error) {
	out := &RenderOutput{
		Backend: BackendAnsible,
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

	out.Files["site.yml"] = r.renderPlaybook(configs)
	out.Files["inventory/hosts.yml"] = r.renderInventory(configs)

	return out, nil
}

func (r *AnsibleRenderer) Validate(config ServiceConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	out, err := r.Render(config)
	if err != nil {
		return fmt.Errorf("validate render: %w", err)
	}
	return ValidateRenderedOutput(out, config)
}

func (r *AnsibleRenderer) Diff(current, desired ServiceConfig) (string, error) {
	if err := current.Validate(); err != nil {
		return "", fmt.Errorf("current config invalid: %w", err)
	}
	if err := desired.Validate(); err != nil {
		return "", fmt.Errorf("desired config invalid: %w", err)
	}
	return DiffConfigs(current, desired), nil
}

func (r *AnsibleRenderer) renderTasks(config ServiceConfig) string {
	binary := config.Binary
	if binary == "" {
		binary = fmt.Sprintf("/opt/unheaded/bin/%s", config.Name)
	}

	s := fmt.Sprintf(`---
# Ansible tasks for %s service
- name: Ensure %s binary exists
  stat:
    path: "%s"
  register: binary_stat

- name: Create %s systemd unit
  template:
    src: %s.service.j2
    dest: /etc/systemd/system/%s.service
    owner: root
    group: root
    mode: "0644"
  notify: restart %s

- name: Enable and start %s
  systemd:
    name: %s
    enabled: true
    state: started
    daemon_reload: true

- name: Wait for %s health check
  uri:
    url: "http://127.0.0.1:{{ %s_port }}%s"
    status_code: 200
  register: health_result
  until: health_result.status == 200
  retries: 30
  delay: 1
`, config.Name, config.Name, binary,
		config.Name, config.Name, config.Name,
		config.Name, config.Name, config.Name,
		config.Name, config.Name, config.HealthPath)

	return s
}

func (r *AnsibleRenderer) renderDefaults(config ServiceConfig) string {
	s := fmt.Sprintf(`---
# Default variables for %s
%s_port: %d
`, config.Name, config.Name, config.Port)

	if config.GRPCPort > 0 {
		s += fmt.Sprintf("%s_grpc_port: %d\n", config.Name, config.GRPCPort)
	}

	for k, v := range config.Environment {
		s += fmt.Sprintf("%s_%s: \"%s\"\n", config.Name, k, v)
	}

	return s
}

func (r *AnsibleRenderer) renderHandlers(config ServiceConfig) string {
	return fmt.Sprintf(`---
# Handlers for %s
- name: restart %s
  systemd:
    name: %s
    state: restarted
    daemon_reload: true
`, config.Name, config.Name, config.Name)
}

func (r *AnsibleRenderer) renderSystemdTemplate(config ServiceConfig) string {
	binary := config.Binary
	if binary == "" {
		binary = fmt.Sprintf("/opt/unheaded/bin/%s", config.Name)
	}

	s := fmt.Sprintf(`[Unit]
Description=Unheaded %s Service
After=network.target
Wants=network.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=5s
`, config.Name, binary)

	if config.Hardening.NoNewPrivs {
		s += "NoNewPrivileges=true\n"
	}
	if config.Hardening.ReadOnlyFS {
		s += "ProtectSystem=strict\nProtectHome=true\nPrivateTmp=true\n"
	}
	if config.Hardening.RunAsUser > 0 {
		s += fmt.Sprintf("User=unheaded\nGroup=unheaded\n")
	}

	// Environment
	for k, v := range config.Environment {
		s += fmt.Sprintf("Environment=%s=%s\n", k, v)
	}

	s += "\n[Install]\nWantedBy=multi-user.target\n"
	return s
}

func (r *AnsibleRenderer) renderPlaybook(configs []ServiceConfig) string {
	s := "---\n# Unheaded service deployment playbook\n- hosts: unheaded_servers\n  become: true\n  roles:\n"
	for _, cfg := range configs {
		s += fmt.Sprintf("    - %s\n", cfg.Name)
	}
	return s
}

func (r *AnsibleRenderer) renderInventory(configs []ServiceConfig) string {
	s := "---\nall:\n  children:\n    unheaded_servers:\n      hosts:\n        unheaded-host:\n          ansible_host: 10.10.10.1\n"
	s += "      vars:\n"
	for _, cfg := range configs {
		s += fmt.Sprintf("        %s_port: %d\n", cfg.Name, cfg.Port)
	}
	return s
}
