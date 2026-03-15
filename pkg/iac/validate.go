// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package iac

import (
	"fmt"
	"strings"
)

// ValidateRenderedOutput performs structural validation on rendered IaC output.
// It checks that all files are non-empty and that the output contains the
// expected service-related content.
func ValidateRenderedOutput(output *RenderOutput, config ServiceConfig) error {
	if output == nil {
		return fmt.Errorf("%w: nil render output", ErrInvalidConfig)
	}
	if len(output.Files) == 0 {
		return fmt.Errorf("%w: rendered output has no files", ErrInvalidConfig)
	}

	for path, content := range output.Files {
		if path == "" {
			return fmt.Errorf("%w: empty file path in rendered output", ErrInvalidConfig)
		}
		if strings.TrimSpace(content) == "" {
			return fmt.Errorf("%w: empty content for file %s", ErrInvalidConfig, path)
		}
	}

	// Backend-specific validation
	switch output.Backend {
	case BackendAnsible:
		return validateAnsibleOutput(output, config)
	case BackendTerraform:
		return validateTerraformOutput(output, config)
	case BackendKubernetes:
		return validateKubernetesOutput(output, config)
	case BackendPuppet:
		return validatePuppetOutput(output, config)
	case BackendChef:
		return validateChefOutput(output, config)
	case BackendSalt:
		return validateSaltOutput(output, config)
	}

	return nil
}

func validateAnsibleOutput(output *RenderOutput, config ServiceConfig) error {
	tasksKey := fmt.Sprintf("roles/%s/tasks/main.yml", config.Name)
	if tasks, ok := output.Files[tasksKey]; ok {
		if !strings.Contains(tasks, "systemd") {
			return fmt.Errorf("%w: Ansible tasks missing systemd reference", ErrInvalidConfig)
		}
		if !strings.Contains(tasks, config.Name) {
			return fmt.Errorf("%w: Ansible tasks missing service name", ErrInvalidConfig)
		}
	}
	return nil
}

func validateTerraformOutput(output *RenderOutput, config ServiceConfig) error {
	mainKey := fmt.Sprintf("modules/%s/main.tf", config.Name)
	if main, ok := output.Files[mainKey]; ok {
		if !strings.Contains(main, "resource") && !strings.Contains(main, "module") {
			return fmt.Errorf("%w: Terraform main.tf missing resource or module block", ErrInvalidConfig)
		}
		if !strings.Contains(main, "healthcheck") {
			return fmt.Errorf("%w: Terraform main.tf missing healthcheck", ErrInvalidConfig)
		}
	}
	return nil
}

func validateKubernetesOutput(output *RenderOutput, config ServiceConfig) error {
	deployKey := fmt.Sprintf("%s/deployment.yaml", config.Name)
	if deploy, ok := output.Files[deployKey]; ok {
		requiredKinds := []string{"kind: Deployment", "apiVersion:"}
		for _, req := range requiredKinds {
			if !strings.Contains(deploy, req) {
				return fmt.Errorf("%w: Kubernetes deployment missing %q", ErrInvalidConfig, req)
			}
		}
		if !strings.Contains(deploy, "livenessProbe") {
			return fmt.Errorf("%w: Kubernetes deployment missing livenessProbe", ErrInvalidConfig)
		}
	}
	return nil
}

func validatePuppetOutput(output *RenderOutput, config ServiceConfig) error {
	manifestKey := fmt.Sprintf("modules/%s/manifests/init.pp", config.Name)
	if manifest, ok := output.Files[manifestKey]; ok {
		if !strings.Contains(manifest, "class") {
			return fmt.Errorf("%w: Puppet manifest missing class definition", ErrInvalidConfig)
		}
		if !strings.Contains(manifest, "service") {
			return fmt.Errorf("%w: Puppet manifest missing service resource", ErrInvalidConfig)
		}
	}
	return nil
}

func validateChefOutput(output *RenderOutput, config ServiceConfig) error {
	recipeKey := fmt.Sprintf("cookbooks/%s/recipes/default.rb", config.Name)
	if recipe, ok := output.Files[recipeKey]; ok {
		if !strings.Contains(recipe, "systemd_unit") {
			return fmt.Errorf("%w: Chef recipe missing systemd_unit resource", ErrInvalidConfig)
		}
	}
	return nil
}

func validateSaltOutput(output *RenderOutput, config ServiceConfig) error {
	stateKey := fmt.Sprintf("salt/%s/init.sls", config.Name)
	if state, ok := output.Files[stateKey]; ok {
		if !strings.Contains(state, "service.running") {
			return fmt.Errorf("%w: Salt state missing service.running", ErrInvalidConfig)
		}
	}
	return nil
}

// DiffConfigs compares two ServiceConfigs and returns a human-readable
// diff of fields that have changed.
func DiffConfigs(current, desired ServiceConfig) string {
	var diffs []string

	if current.Name != desired.Name {
		diffs = append(diffs, fmt.Sprintf("  name: %q -> %q", current.Name, desired.Name))
	}
	if current.Port != desired.Port {
		diffs = append(diffs, fmt.Sprintf("  port: %d -> %d", current.Port, desired.Port))
	}
	if current.GRPCPort != desired.GRPCPort {
		diffs = append(diffs, fmt.Sprintf("  grpc_port: %d -> %d", current.GRPCPort, desired.GRPCPort))
	}
	if current.Image != desired.Image {
		diffs = append(diffs, fmt.Sprintf("  image: %q -> %q", current.Image, desired.Image))
	}
	if current.Binary != desired.Binary {
		diffs = append(diffs, fmt.Sprintf("  binary: %q -> %q", current.Binary, desired.Binary))
	}
	if current.HealthPath != desired.HealthPath {
		diffs = append(diffs, fmt.Sprintf("  health_path: %q -> %q", current.HealthPath, desired.HealthPath))
	}
	if current.ReadyPath != desired.ReadyPath {
		diffs = append(diffs, fmt.Sprintf("  ready_path: %q -> %q", current.ReadyPath, desired.ReadyPath))
	}
	if current.Replicas != desired.Replicas {
		diffs = append(diffs, fmt.Sprintf("  replicas: %d -> %d", current.Replicas, desired.Replicas))
	}
	if current.Resources.CPULimit != desired.Resources.CPULimit {
		diffs = append(diffs, fmt.Sprintf("  resources.cpu_limit: %q -> %q", current.Resources.CPULimit, desired.Resources.CPULimit))
	}
	if current.Resources.MemLimit != desired.Resources.MemLimit {
		diffs = append(diffs, fmt.Sprintf("  resources.mem_limit: %q -> %q", current.Resources.MemLimit, desired.Resources.MemLimit))
	}
	if current.Hardening.ReadOnlyFS != desired.Hardening.ReadOnlyFS {
		diffs = append(diffs, fmt.Sprintf("  hardening.read_only_fs: %v -> %v", current.Hardening.ReadOnlyFS, desired.Hardening.ReadOnlyFS))
	}
	if current.Hardening.NoNewPrivs != desired.Hardening.NoNewPrivs {
		diffs = append(diffs, fmt.Sprintf("  hardening.no_new_privs: %v -> %v", current.Hardening.NoNewPrivs, desired.Hardening.NoNewPrivs))
	}
	if current.Hardening.RunAsUser != desired.Hardening.RunAsUser {
		diffs = append(diffs, fmt.Sprintf("  hardening.run_as_user: %d -> %d", current.Hardening.RunAsUser, desired.Hardening.RunAsUser))
	}
	if current.Network.InternalOnly != desired.Network.InternalOnly {
		diffs = append(diffs, fmt.Sprintf("  network.internal_only: %v -> %v", current.Network.InternalOnly, desired.Network.InternalOnly))
	}

	// Environment diffs
	for k, cv := range current.Environment {
		dv, ok := desired.Environment[k]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("  env.%s: %q -> (removed)", k, cv))
		} else if cv != dv {
			diffs = append(diffs, fmt.Sprintf("  env.%s: %q -> %q", k, cv, dv))
		}
	}
	for k, dv := range desired.Environment {
		if _, ok := current.Environment[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("  env.%s: (added) -> %q", k, dv))
		}
	}

	if len(diffs) == 0 {
		return ""
	}
	return "Changes:\n" + strings.Join(diffs, "\n") + "\n"
}
