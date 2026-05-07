// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides IaC generation commands for THE GAUNTLETS CLI.
package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"unheaded/pkg/iac"
)

// NewGenerateCommand creates the generate command.
func NewGenerateCommand() *Command {
	cmd := &Command{
		Name:        "generate",
		Short:       "Generate IaC artifacts",
		Long:        "Generate Infrastructure-as-Code artifacts for Unheaded services.\n\nSupported backends: ansible, terraform, puppet, kubernetes, chef, salt",
		Aliases:     []string{"gen"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newGenerateIaCCommand())
	cmd.AddCommand(newGenerateListCommand())

	return cmd
}

func newGenerateIaCCommand() *Command {
	flags := flag.NewFlagSet("iac", flag.ContinueOnError)
	backend := flags.String("backend", "", "IaC backend: ansible, terraform, puppet, kubernetes, chef, salt")
	services := flags.String("services", "", "Comma-separated list of services (default: all core services)")
	outDir := flags.String("out", "./generated", "Output directory for generated files")
	dryRun := flags.Bool("dry-run", false, "Show what would be generated without writing files")

	return &Command{
		Name:  "iac",
		Short: "Generate IaC configuration files",
		Long:  "Generate IaC configuration files for the specified backend and services.",
		Usage: "unheaded generate iac --backend <backend> [--services <svc1,svc2>] [--out <dir>]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if *backend == "" {
				return fmt.Errorf("--backend is required (ansible, terraform, puppet, kubernetes, chef, salt)")
			}

			renderer, err := iac.NewRenderer(iac.Backend(*backend))
			if err != nil {
				return fmt.Errorf("unknown backend %q: %w", *backend, err)
			}

			configs := resolveServiceConfigs(*services)

			out, err := renderer.RenderAll(configs)
			if err != nil {
				return fmt.Errorf("render failed: %w", err)
			}

			if *dryRun {
				fmt.Fprintf(os.Stdout, "Backend: %s\n", out.Backend)
				fmt.Fprintf(os.Stdout, "Files (%d):\n", len(out.Files))
				for path := range out.Files {
					fmt.Fprintf(os.Stdout, "  %s\n", filepath.Join(*outDir, path))
				}
				return nil
			}

			written := 0
			for path, content := range out.Files {
				fullPath := filepath.Join(*outDir, string(out.Backend), path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("mkdir %s: %w", dir, err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					return fmt.Errorf("write %s: %w", fullPath, err)
				}
				written++
			}

			fmt.Fprintf(os.Stdout, "Generated %d files for %s backend in %s/\n", written, out.Backend, filepath.Join(*outDir, string(out.Backend)))
			return nil
		},
	}
}

func newGenerateListCommand() *Command {
	return &Command{
		Name:    "list",
		Short:   "List available IaC backends",
		Aliases: []string{"ls"},
		RunFunc: func(ctx *Context, args []string) error {
			fmt.Fprintln(os.Stdout, "Available IaC backends:")
			for _, b := range iac.AllBackends() {
				desc := backendDescription(b)
				fmt.Fprintf(os.Stdout, "  %-12s  %s\n", b, desc)
			}
			return nil
		},
	}
}

func backendDescription(b iac.Backend) string {
	switch b {
	case iac.BackendAnsible:
		return "Playbooks, roles, inventory, systemd templates"
	case iac.BackendTerraform:
		return "HCL modules, Docker container resources"
	case iac.BackendPuppet:
		return "Manifests, Hiera data, site.pp"
	case iac.BackendKubernetes:
		return "Deployments, Services, NetworkPolicies, Kustomization"
	case iac.BackendChef:
		return "Cookbooks, recipes, Policyfile"
	case iac.BackendSalt:
		return "States, pillars, top.sls"
	default:
		return ""
	}
}

// resolveServiceConfigs builds ServiceConfig list from comma-separated names.
func resolveServiceConfigs(services string) []iac.ServiceConfig {
	type svcDef struct {
		name string
		port int
	}

	allServices := []svcDef{
		{"wotan", 18000},
		{"timeguru", 19000},
		{"architect", 19001},
		{"captain", 19002},
		{"micromanager", 19003},
		{"monad", 19004},
		{"sophia", 19005},
		{"dashboard-backend", 20000},
		{"kanban-app", 20001},
		{"unheaded-daemon", 17000},
	}

	var wanted map[string]bool
	if services != "" {
		wanted = make(map[string]bool)
		for _, s := range strings.Split(services, ",") {
			wanted[strings.TrimSpace(s)] = true
		}
	}

	var configs []iac.ServiceConfig
	for _, s := range allServices {
		if wanted != nil && !wanted[s.name] {
			continue
		}
		configs = append(configs, iac.DefaultServiceConfig(s.name, s.port))
	}
	return configs
}
