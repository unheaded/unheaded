// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package main is the entry point for wotan-ctl
// wotan-ctl is the operator's tool for loading programs and data into the
// Doom-over-IPv6 compute engine (Wotan). It provides commands for:
//   - load-rom: Load MBC bytecode into BPF rom_map (CPU instruction memory)
//   - load-mem: Load binary data into Wotan's L2 ring buffer (RAM)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"unheaded/pkg/logger"
)

// Version information - set at build time
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

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
	Logger    *logger.Logger
	Verbose   bool
	Quiet     bool
	MapPinDir string // BPF map pin directory
}

// NewRootCommand creates the root command for wotan-ctl.
func NewRootCommand() *Command {
	root := &Command{
		Name:  "wotan-ctl",
		Short: "Wotan compute engine control tool",
		Long: `wotan-ctl is the operator's tool for the Doom-over-IPv6 compute engine.

Load MBC bytecode and binary data into the Wotan BPF compute engine:
  - load-rom: Load MBC bytecode into BPF rom_map (instruction memory)
  - load-mem: Load binary data into L2 ring buffer (RAM)
  - doom:     Doom-over-IPv6 management (load, status, input, reset)
  - boot:     UPC boot protocol (load kernel, init IVT, set CPU state)

Use "wotan-ctl <command> --help" for more information about a command.`,
		Usage:       "wotan-ctl [command] [flags]",
		SubCommands: make(map[string]*Command),
	}

	// Add subcommands
	root.SubCommands["load-rom"] = newLoadRomCmd()
	root.SubCommands["load-mem"] = newLoadMemCmd()
	root.SubCommands["doom"] = newDoomCmd()
	root.SubCommands["boot"] = newBootCmd()
	root.SubCommands["mkfs"] = newMkfsCmd()

	return root
}

// Execute runs the command with the given arguments.
func (c *Command) Execute(args []string) error {
	// Skip program name if present
	if len(args) > 0 && filepath.Base(args[0]) == filepath.Base(os.Args[0]) {
		args = args[1:]
	}

	ctx := &Context{
		Logger:    logger.New(os.Stderr),
		MapPinDir: "/sys/fs/bpf",
	}
	ctx.Logger.SetLevel(logger.InfoLevel)

	return c.executeInternal(ctx, args)
}

func (c *Command) executeInternal(ctx *Context, args []string) error {
	// Help flag
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		c.printHelp()
		return nil
	}

	// Check for version at root level
	if len(args) > 0 && args[0] == "version" && c.RunFunc == nil {
		fmt.Printf("wotan-ctl %s (commit: %s, built: %s)\n", Version, Commit, BuildTime)
		return nil
	}

	// Check for subcommand
	if len(args) > 0 {
		if subCmd, ok := c.SubCommands[args[0]]; ok {
			return subCmd.executeInternal(ctx, args[1:])
		}
	}

	// If this command has a RunFunc, invoke it with remaining args
	if c.RunFunc != nil {
		return c.RunFunc(ctx, args)
	}

	// No RunFunc and no matching subcommand
	if len(args) == 0 {
		c.printHelp()
		return nil
	}

	if strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(os.Stderr, "Error: unknown flag %s\n", args[0])
	} else {
		fmt.Fprintf(os.Stderr, "Error: unknown command %s\n", args[0])
	}
	c.printHelp()
	os.Exit(1)
	return nil
}

func (c *Command) printHelp() {
	fmt.Printf("Usage: %s\n\n", c.Usage)
	fmt.Printf("%s\n\n", c.Long)

	if len(c.SubCommands) > 0 {
		fmt.Println("Available commands:")
		var names []string
		for name := range c.SubCommands {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			cmd := c.SubCommands[name]
			fmt.Printf("  %-12s %s\n", name, cmd.Short)
		}
		fmt.Println()
	}

	if c.Flags != nil && c.Flags.NFlag() > 0 {
		fmt.Println("Flags:")
		c.Flags.PrintDefaults()
	}

	fmt.Printf("Use \"%s <command> --help\" for more information about a command.\n", c.Name)
}

func main() {
	root := NewRootCommand()
	if err := root.Execute(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
