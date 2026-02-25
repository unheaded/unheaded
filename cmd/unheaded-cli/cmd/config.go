// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package cmd provides config commands for THE GAUNTLETS CLI.
package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"unheaded/cmd/unheaded-cli/output"
	"gopkg.in/yaml.v3"
)

// ConfigListResult represents the output for config list.
type ConfigListResult struct {
	Items []ConfigItem `json:"items"`
}

// ConfigItem represents a configuration key-value pair.
type ConfigItem struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Source      string `json:"source"` // file, env, default
	Description string `json:"description,omitempty"`
}

// WriteTable implements output.TableWriter.
func (r *ConfigListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("KEY", "VALUE", "SOURCE", "DESCRIPTION")
	} else {
		t.SetHeaders("KEY", "VALUE", "SOURCE")
	}

	for _, item := range r.Items {
		if wide {
			t.AddRow(item.Key, item.Value, item.Source, item.Description)
		} else {
			t.AddRow(item.Key, item.Value, item.Source)
		}
	}
}

// NewConfigCommand creates the config command.
func NewConfigCommand() *Command {
	cmd := &Command{
		Name:        "config",
		Short:       "Manage CLI configuration",
		Long:        "Commands for managing the Unheaded CLI configuration.",
		Aliases:     []string{"cfg"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newConfigGetCommand())
	cmd.AddCommand(newConfigSetCommand())
	cmd.AddCommand(newConfigListCommand())
	cmd.AddCommand(newConfigValidateCommand())
	cmd.AddCommand(newConfigCurrentContextCommand())
	cmd.AddCommand(newConfigUseContextCommand())
	cmd.AddCommand(newConfigViewCommand())
	cmd.AddCommand(newConfigInitCommand())

	return cmd
}

func newConfigGetCommand() *Command {
	return &Command{
		Name:  "get",
		Short: "Get a configuration value",
		Usage: "unheaded config get <key>",
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("configuration key required")
			}
			key := args[0]

			value, err := getConfigValue(ctx.Config, key)
			if err != nil {
				return err
			}

			if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
				return ctx.Output.Write(map[string]string{key: value})
			}

			ctx.Output.WriteStringln(value)
			return nil
		},
	}
}

func newConfigSetCommand() *Command {
	return &Command{
		Name:  "set",
		Short: "Set a configuration value",
		Usage: "unheaded config set <key> <value>",
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("configuration key and value required")
			}
			key := args[0]
			value := args[1]

			if err := setConfigValue(ctx.Config, key, value); err != nil {
				return err
			}

			if err := SaveConfig(ctx.Config); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			ctx.Output.WriteSuccess("Configuration updated: %s = %s", key, value)
			return nil
		},
	}
}

func newConfigListCommand() *Command {
	return &Command{
		Name:    "list",
		Short:   "List all configuration values",
		Usage:   "unheaded config list",
		Aliases: []string{"ls"},
		RunFunc: func(ctx *Context, args []string) error {
			result := &ConfigListResult{
				Items: []ConfigItem{
					{Key: "current_context", Value: ctx.Config.CurrentContext, Source: "file", Description: "Current context"},
					{Key: "defaults.output_format", Value: ctx.Config.Defaults.OutputFormat, Source: "file", Description: "Default output format"},
					{Key: "defaults.namespace", Value: ctx.Config.Defaults.Namespace, Source: "file", Description: "Default namespace"},
				},
			}

			// Add context-specific settings
			if cluster, err := ctx.Config.GetCurrentCluster(); err == nil {
				result.Items = append(result.Items,
					ConfigItem{Key: "context.endpoint", Value: cluster.Endpoint, Source: "file", Description: "API endpoint"},
					ConfigItem{Key: "context.wotan_addr", Value: cluster.WotanAddr, Source: "file", Description: "Wotan address"},
				)
			}

			// Add environment overrides
			if endpoint := os.Getenv("UNHEADED_ENDPOINT"); endpoint != "" {
				result.Items = append(result.Items, ConfigItem{Key: "endpoint", Value: endpoint, Source: "env", Description: "API endpoint (from env)"})
			}

			return ctx.Output.Write(result)
		},
	}
}

func newConfigValidateCommand() *Command {
	return &Command{
		Name:  "validate",
		Short: "Validate configuration",
		Usage: "unheaded config validate",
		RunFunc: func(ctx *Context, args []string) error {
			w := ctx.Output
			errors := validateConfig(ctx.Config)

			if len(errors) == 0 {
				w.WriteSuccess("Configuration is valid")
				return nil
			}

			w.WriteError("Configuration has %d error(s):", len(errors))
			for _, err := range errors {
				w.WriteStringln(fmt.Sprintf("  - %s", err))
			}
			return fmt.Errorf("configuration validation failed")
		},
	}
}

func newConfigCurrentContextCommand() *Command {
	return &Command{
		Name:    "current-context",
		Short:   "Display the current context",
		Usage:   "unheaded config current-context",
		Aliases: []string{"ctx"},
		RunFunc: func(ctx *Context, args []string) error {
			ctx.Output.WriteStringln(ctx.Config.CurrentContext)
			return nil
		},
	}
}

func newConfigUseContextCommand() *Command {
	return &Command{
		Name:  "use-context",
		Short: "Set the current context",
		Usage: "unheaded config use-context <context>",
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("context name required")
			}
			contextName := args[0]

			// Verify context exists
			if _, ok := ctx.Config.Contexts[contextName]; !ok {
				// List available contexts
				var available []string
				for name := range ctx.Config.Contexts {
					available = append(available, name)
				}
				sort.Strings(available)
				return fmt.Errorf("context '%s' not found. Available contexts: %s", contextName, strings.Join(available, ", "))
			}

			ctx.Config.CurrentContext = contextName
			if err := SaveConfig(ctx.Config); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			ctx.Output.WriteSuccess("Switched to context '%s'", contextName)
			return nil
		},
	}
}

func newConfigViewCommand() *Command {
	var minify bool

	flags := flag.NewFlagSet("view", flag.ContinueOnError)
	flags.BoolVar(&minify, "minify", false, "Minify output")

	return &Command{
		Name:  "view",
		Short: "View the full configuration",
		Usage: "unheaded config view [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if ctx.Output.Format() == output.FormatJSON {
				return ctx.Output.WriteJSON(ctx.Config)
			}

			data, err := yaml.Marshal(ctx.Config)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			ctx.Output.WriteStringln(string(data))
			return nil
		},
	}
}

func newConfigInitCommand() *Command {
	var force bool

	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.BoolVar(&force, "force", false, "Overwrite existing configuration")
	flags.BoolVar(&force, "f", false, "Force overwrite (short)")

	return &Command{
		Name:  "init",
		Short: "Initialize CLI configuration",
		Usage: "unheaded config init [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}

			configDir := filepath.Join(homeDir, ".unheaded")
			configPath := filepath.Join(configDir, "config.yaml")

			// Check if config exists
			if _, err := os.Stat(configPath); err == nil && !force {
				return fmt.Errorf("configuration already exists at %s. Use --force to overwrite", configPath)
			}

			// Create config directory
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			// Create default configuration
			defaultConfig := DefaultConfig()

			data, err := yaml.Marshal(defaultConfig)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			// Add header comment
			header := `# Unheaded CLI Configuration
# Generated by: unheaded config init
#
# Contexts define different cluster/environment configurations.
# Use 'unheaded config use-context <name>' to switch between contexts.
#

`
			if err := os.WriteFile(configPath, []byte(header+string(data)), 0600); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}

			ctx.Output.WriteSuccess("Configuration initialized at %s", configPath)
			return nil
		},
	}
}

// getConfigValue retrieves a configuration value by key.
func getConfigValue(config *Config, key string) (string, error) {
	switch key {
	case "current_context":
		return config.CurrentContext, nil
	case "defaults.output_format":
		return config.Defaults.OutputFormat, nil
	case "defaults.namespace":
		return config.Defaults.Namespace, nil
	default:
		// Check if it's a context-specific key
		if strings.HasPrefix(key, "contexts.") {
			parts := strings.SplitN(key, ".", 3)
			if len(parts) < 3 {
				return "", fmt.Errorf("invalid context key: %s", key)
			}
			ctxName := parts[1]
			field := parts[2]

			ctx, ok := config.Contexts[ctxName]
			if !ok {
				return "", fmt.Errorf("context not found: %s", ctxName)
			}

			switch field {
			case "name":
				return ctx.Name, nil
			case "endpoint":
				return ctx.Endpoint, nil
			case "wotan_addr":
				return ctx.WotanAddr, nil
			default:
				return "", fmt.Errorf("unknown context field: %s", field)
			}
		}
		return "", fmt.Errorf("unknown configuration key: %s", key)
	}
}

// setConfigValue sets a configuration value by key.
func setConfigValue(config *Config, key, value string) error {
	switch key {
	case "current_context":
		if _, ok := config.Contexts[value]; !ok {
			return fmt.Errorf("context not found: %s", value)
		}
		config.CurrentContext = value
	case "defaults.output_format":
		if _, err := output.ParseFormat(value); err != nil {
			return err
		}
		config.Defaults.OutputFormat = value
	case "defaults.namespace":
		config.Defaults.Namespace = value
	default:
		// Check if it's a context-specific key
		if strings.HasPrefix(key, "contexts.") {
			parts := strings.SplitN(key, ".", 3)
			if len(parts) < 3 {
				return fmt.Errorf("invalid context key: %s", key)
			}
			ctxName := parts[1]
			field := parts[2]

			ctx, ok := config.Contexts[ctxName]
			if !ok {
				// Create new context
				ctx = Cluster{Name: ctxName}
			}

			switch field {
			case "endpoint":
				ctx.Endpoint = value
			case "wotan_addr":
				ctx.WotanAddr = value
			default:
				return fmt.Errorf("unknown context field: %s", field)
			}

			config.Contexts[ctxName] = ctx
		} else {
			return fmt.Errorf("unknown configuration key: %s", key)
		}
	}
	return nil
}

// validateConfig validates the configuration and returns any errors.
func validateConfig(config *Config) []string {
	var errors []string

	// Check current context exists
	if _, ok := config.Contexts[config.CurrentContext]; !ok {
		errors = append(errors, fmt.Sprintf("current_context '%s' does not exist", config.CurrentContext))
	}

	// Validate contexts
	for name, ctx := range config.Contexts {
		if ctx.Endpoint == "" {
			errors = append(errors, fmt.Sprintf("context '%s' has no endpoint", name))
		}
	}

	// Validate output format
	if config.Defaults.OutputFormat != "" {
		if _, err := output.ParseFormat(config.Defaults.OutputFormat); err != nil {
			errors = append(errors, fmt.Sprintf("invalid defaults.output_format: %s", config.Defaults.OutputFormat))
		}
	}

	return errors
}
