// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides the command structure for THE GAUNTLETS CLI.
// This implements a subcommand pattern similar to kubectl/docker without external frameworks.
package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/logger"
	"gopkg.in/yaml.v3"
)

// Version information (set by main.go from build flags).
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// SetVersionInfo sets the version information.
func SetVersionInfo(v, c, b string) {
	version, commit, buildTime = v, c, b
}

// Command represents a CLI command or subcommand.
type Command struct {
	Name        string
	Short       string
	Long        string
	Usage       string
	Aliases     []string
	Flags       *flag.FlagSet
	SubCommands map[string]*Command
	RunFunc     func(ctx *Context, args []string) error
	parent      *Command
}

// Context holds the runtime context for command execution.
type Context struct {
	Config *Config
	Output *output.Writer
	Logger *logger.Logger

	// Global flags
	OutputFormat string
	Verbose      bool
	Quiet        bool
	NoColor      bool
	Context      string // Cluster/environment context
}

// Config represents the CLI configuration file (~/.unheaded/config.yaml).
type Config struct {
	CurrentContext string             `yaml:"current_context"`
	Contexts       map[string]Cluster `yaml:"contexts"`
	Defaults       Defaults           `yaml:"defaults"`
}

// Cluster represents a cluster/environment configuration.
type Cluster struct {
	Name       string `yaml:"name"`
	Endpoint   string `yaml:"endpoint"`
	WotanAddr string `yaml:"wotan_addr"`
	TLSCert    string `yaml:"tls_cert"`
	TLSKey     string `yaml:"tls_key"`
	TLSCA      string `yaml:"tls_ca"`
}

// Defaults holds default values for the CLI.
type Defaults struct {
	OutputFormat string `yaml:"output_format"`
	Namespace    string `yaml:"namespace"`
}

// NewRootCommand creates the root command for the CLI.
func NewRootCommand() *Command {
	root := &Command{
		Name:  "unheaded",
		Short: "THE GAUNTLETS - Unheaded Kingdom CLI",
		Long: `THE GAUNTLETS - Command-line interface for the Unheaded Kingdom

The CLI provides commands for managing:
  - Containers: lifecycle, execution, monitoring
  - Services: deployment, status, logs
  - Configuration: get, set, validate settings
  - Secrets: secure credential management
  - Network: policies and routing
  - Generate: IaC artifact generation (Ansible, Terraform, K8s, etc.)
  - Observe: observability backend configuration
  - Status: system-wide overview

Use "unheaded <command> --help" for more information about a command.`,
		Usage:       "unheaded [command] [flags]",
		SubCommands: make(map[string]*Command),
	}

	// Register all subcommands
	root.AddCommand(NewContainerCommand())
	root.AddCommand(NewServiceCommand())
	root.AddCommand(NewConfigCommand())
	root.AddCommand(NewSecretCommand())
	root.AddCommand(NewNetworkCommand())
	root.AddCommand(NewStatusCommand())
	root.AddCommand(NewVersionCommand())
	root.AddCommand(NewCompletionCommand())
	root.AddCommand(NewGenerateCommand())
	root.AddCommand(NewObserveCommand())

	return root
}

// AddCommand adds a subcommand.
func (c *Command) AddCommand(sub *Command) {
	sub.parent = c
	c.SubCommands[sub.Name] = sub
	for _, alias := range sub.Aliases {
		c.SubCommands[alias] = sub
	}
}

// Execute runs the command with the given arguments.
func (c *Command) Execute(args []string) error {
	// Create context
	ctx := &Context{
		Output: output.NewWriter(),
	}

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		// Config file is optional, use defaults
		config = DefaultConfig()
	}
	ctx.Config = config

	// Check for help flag at the root level (no subcommand)
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		return c.showHelp(ctx)
	}

	// Setup global flags
	globalFlags := flag.NewFlagSet("global", flag.ContinueOnError)
	globalFlags.SetOutput(io.Discard) // Suppress default error output
	var showHelp bool
	globalFlags.BoolVar(&showHelp, "help", false, "Show help")
	globalFlags.BoolVar(&showHelp, "h", false, "Show help (short)")
	globalFlags.StringVar(&ctx.OutputFormat, "output", config.Defaults.OutputFormat, "Output format: table, json, yaml, wide")
	globalFlags.StringVar(&ctx.OutputFormat, "o", config.Defaults.OutputFormat, "Output format (short)")
	globalFlags.BoolVar(&ctx.Verbose, "verbose", false, "Enable verbose output")
	globalFlags.BoolVar(&ctx.Verbose, "v", false, "Enable verbose output (short)")
	globalFlags.BoolVar(&ctx.Quiet, "quiet", false, "Suppress non-essential output")
	globalFlags.BoolVar(&ctx.Quiet, "q", false, "Suppress non-essential output (short)")
	globalFlags.BoolVar(&ctx.NoColor, "no-color", false, "Disable color output")
	globalFlags.StringVar(&ctx.Context, "context", config.CurrentContext, "Context to use")

	// Parse global flags from the beginning of args
	if err := globalFlags.Parse(args); err != nil {
		return err
	}

	if showHelp {
		return c.showHelp(ctx)
	}
	args = globalFlags.Args()

	// Apply global settings to output writer
	if format, err := output.ParseFormat(ctx.OutputFormat); err == nil {
		ctx.Output.SetFormat(format)
	}
	ctx.Output.SetColor(!ctx.NoColor && output.IsTerminal())

	// Setup logger
	logConfig := logger.DefaultConfig()
	if ctx.Verbose {
		logConfig.Level = logger.DebugLevel
	} else if ctx.Quiet {
		logConfig.Level = logger.WarnLevel
	}
	logConfig.ConsoleMode = true
	logConfig.ColorEnabled = !ctx.NoColor && output.IsTerminal()
	ctx.Logger = logger.NewWithConfig(logConfig)

	// No args - show help
	if len(args) == 0 {
		return c.showHelp(ctx)
	}

	// Find subcommand
	subName := args[0]
	sub, ok := c.SubCommands[subName]
	if !ok {
		return fmt.Errorf("unknown command: %s\nRun 'unheaded --help' for usage", subName)
	}

	// Execute subcommand
	return sub.executeWithContext(ctx, args[1:])
}

// executeWithContext runs the command with context.
func (c *Command) executeWithContext(ctx *Context, args []string) error {
	// Check for help flag
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return c.showHelp(ctx)
		}
	}

	// If this command has subcommands and no run function, look for subcommand
	if len(c.SubCommands) > 0 && c.RunFunc == nil {
		if len(args) == 0 {
			return c.showHelp(ctx)
		}

		subName := args[0]
		sub, ok := c.SubCommands[subName]
		if !ok {
			return fmt.Errorf("unknown subcommand: %s\nRun 'unheaded %s --help' for usage", subName, c.Name)
		}
		return sub.executeWithContext(ctx, args[1:])
	}

	// Parse command-specific flags
	if c.Flags != nil {
		c.Flags.SetOutput(io.Discard)
		if err := c.Flags.Parse(args); err != nil {
			return err
		}
		args = c.Flags.Args()
	}

	// Run the command
	if c.RunFunc == nil {
		return c.showHelp(ctx)
	}
	return c.RunFunc(ctx, args)
}

// showHelp displays help for the command.
func (c *Command) showHelp(ctx *Context) error {
	w := ctx.Output

	// Build full command path
	path := c.fullPath()

	// Header
	if c.Long != "" {
		w.WriteStringln(c.Long)
	} else if c.Short != "" {
		w.WriteStringln(c.Short)
	}
	w.WriteStringln("")

	// Usage
	if c.Usage != "" {
		w.WriteStringln("Usage:")
		w.WriteStringln("  " + c.Usage)
	} else {
		w.WriteStringln("Usage:")
		if len(c.SubCommands) > 0 {
			w.WriteStringln("  " + path + " [command]")
		} else {
			w.WriteStringln("  " + path + " [flags]")
		}
	}
	w.WriteStringln("")

	// Aliases
	if len(c.Aliases) > 0 {
		w.WriteStringln("Aliases:")
		w.WriteStringln("  " + strings.Join(c.Aliases, ", "))
		w.WriteStringln("")
	}

	// Subcommands
	if len(c.SubCommands) > 0 {
		w.WriteStringln("Available Commands:")

		// Collect unique commands (filter aliases)
		seen := make(map[*Command]bool)
		var cmds []*Command
		for _, sub := range c.SubCommands {
			if !seen[sub] {
				seen[sub] = true
				cmds = append(cmds, sub)
			}
		}

		// Sort by name
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].Name < cmds[j].Name
		})

		// Print commands
		maxLen := 0
		for _, sub := range cmds {
			if len(sub.Name) > maxLen {
				maxLen = len(sub.Name)
			}
		}
		for _, sub := range cmds {
			w.WriteStringln(fmt.Sprintf("  %-*s  %s", maxLen, sub.Name, sub.Short))
		}
		w.WriteStringln("")
	}

	// Flags
	if c.Flags != nil {
		w.WriteStringln("Flags:")
		c.Flags.SetOutput(os.Stdout)
		c.Flags.PrintDefaults()
		w.WriteStringln("")
	}

	// Global flags (only for root)
	if c.parent == nil {
		w.WriteStringln("Global Flags:")
		w.WriteStringln("  -o, --output string    Output format: table, json, yaml, wide (default \"table\")")
		w.WriteStringln("  -v, --verbose          Enable verbose output")
		w.WriteStringln("  -q, --quiet            Suppress non-essential output")
		w.WriteStringln("      --no-color         Disable color output")
		w.WriteStringln("      --context string   Context to use")
		w.WriteStringln("")
	}

	// Footer
	if len(c.SubCommands) > 0 {
		w.WriteStringln(fmt.Sprintf("Use \"%s [command] --help\" for more information about a command.", path))
	}

	return nil
}

// fullPath returns the full command path (e.g., "unheaded container list").
func (c *Command) fullPath() string {
	if c.parent == nil {
		return c.Name
	}
	return c.parent.fullPath() + " " + c.Name
}

// LoadConfig loads the CLI configuration from the default location.
func LoadConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ".unheaded", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig saves the CLI configuration to the default location.
func SaveConfig(config *Config) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(homeDir, ".unheaded")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.yaml")
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		CurrentContext: "local",
		Contexts: map[string]Cluster{
			"local": {
				Name:       "local",
				Endpoint:   "http://localhost:17000",
				WotanAddr: "localhost:18001",
			},
		},
		Defaults: Defaults{
			OutputFormat: "table",
			Namespace:    "default",
		},
	}
}

// GetCurrentCluster returns the current cluster configuration.
func (c *Config) GetCurrentCluster() (*Cluster, error) {
	cluster, ok := c.Contexts[c.CurrentContext]
	if !ok {
		return nil, fmt.Errorf("context not found: %s", c.CurrentContext)
	}
	return &cluster, nil
}
