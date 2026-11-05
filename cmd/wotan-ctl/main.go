// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the GNU General Public License v3.0 or later.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package main is the entry point for wotan-ctl
// wotan-ctl is the operator's tool for loading programs and data into the
// Doom-over-IPv6 compute engine (Wotan). It provides commands for:
//   - load-rom: Load MBC bytecode into BPF rom_map (CPU instruction memory)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"unheaded/pkg/cmdline"
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
		Long: `wotan-ctl is the operator's tool for the UPC compute engine.

Load MBC bytecode and binary data into the Wotan BPF compute engine:
  - load-rom: Load MBC bytecode into BPF rom_map (instruction memory)
  - boot:     UPC boot protocol (load kernel, init IVT, set CPU state)

Use "wotan-ctl <command> --help" for more information about a command.`,
		Usage:       "wotan-ctl [command] [flags]",
		SubCommands: make(map[string]*Command),
	}

	// Add subcommands
	root.SubCommands["load-rom"] = newLoadRomCmd()
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

	// Root-level manual page (scripts/gen-man.sh)
	if len(args) == 1 && cmdline.IsManArg(args[0]) && c.RunFunc == nil {
		c.printMan()
		return nil
	}

	// Check for version at root level
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-V") && c.RunFunc == nil {
		cmdline.PrintVersion(os.Stdout, cmdline.Spec{Name: "wotan-ctl", Version: Version})
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
		fmt.Fprintf(os.Stderr, "wotan-ctl: unrecognized option '%s'\n", args[0])
	} else {
		fmt.Fprintf(os.Stderr, "wotan-ctl: unknown command '%s'\n", args[0])
	}
	fmt.Fprintln(os.Stderr, "Try 'wotan-ctl --help' for more information.")
	os.Exit(2)
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

// printMan writes the root command's man page from its subcommand table.
func (c *Command) printMan() {
	names := make([]string, 0, len(c.SubCommands))
	for name := range c.SubCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	cmds := make([]cmdline.Command, 0, len(names))
	for _, name := range names {
		cmds = append(cmds, cmdline.Command{Name: name, Purpose: c.SubCommands[name].Short})
	}
	cmdline.Man(os.Stdout, flag.NewFlagSet(c.Name, flag.ContinueOnError), cmdline.Spec{
		Name:     c.Name,
		Purpose:  "Load MBC bytecode, memory images and boot state into the Wotan BPF compute engine.",
		Operands: "COMMAND [OPTION]...",
		Commands: cmds,
		Examples: []string{
			"wotan-ctl load-rom -file prog.mbc         load bytecode into rom_map",
			"wotan-ctl boot --help                     UPC boot options",
		},
		Details: `Run "wotan-ctl COMMAND --help" for a command's options. BPF maps are read from /sys/fs/bpf.`,
		Version: Version,
	})
}

func main() {
	root := NewRootCommand()
	if err := root.Execute(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "wotan-ctl: %s\n", cmdline.Translate(err, os.Args[1:]))
		os.Exit(1)
	}
}
