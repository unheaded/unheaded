// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides the status command for THE GAUNTLETS CLI.
package cmd

import (
	"flag"
	"fmt"
	"net/http"
	"sort"
	"time"

	"unheaded/cmd/unheaded-cli/output"
)

// SystemStatus is what status could actually read. A source that did not
// answer is reported as an error, never filled in.
type SystemStatus struct {
	Context    string         `json:"context"`
	Version    string         `json:"cli_version"`
	CheckedAt  time.Time      `json:"checked_at"`
	Services   []akiraVerdict `json:"services,omitempty"`
	ServiceErr string         `json:"services_error,omitempty"`
	Containers map[string]int `json:"containers,omitempty"` // by actual status
	ContainErr string         `json:"containers_error,omitempty"`
}

// NewStatusCommand creates the status command.
func NewStatusCommand() *Command {
	var (
		watch    bool
		interval int
	)

	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.BoolVar(&watch, "watch", false, "Watch status continuously")
	flags.BoolVar(&watch, "w", false, "Watch (short)")
	flags.IntVar(&interval, "interval", 5, "Watch interval in seconds")
	flags.IntVar(&interval, "i", 5, "Interval (short)")

	return &Command{
		Name:  "status",
		Short: "Show system status overview",
		Long: `Overview of the Unheaded Kingdom:

  - Service health: Akira's consensus over every reporter's checks
  - Containers: Cuirass's desired containers, counted by actual status

Each part says so when its service does not answer.`,
		Usage:   "unheaded status [flags]",
		Aliases: []string{"stat"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if watch {
				return watchStatus(ctx, interval)
			}
			return showStatus(ctx)
		},
	}
}

// showStatus prints what it could read and fails if any part did not answer.
func showStatus(ctx *Context) error {
	status := gatherSystemStatus(ctx)
	var err error
	if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
		err = ctx.Output.Write(status)
	} else {
		err = renderStatus(ctx, status)
	}
	if err != nil {
		return err
	}
	if status.ServiceErr != "" || status.ContainErr != "" {
		return fmt.Errorf("status incomplete: a source did not answer")
	}
	return nil
}

func watchStatus(ctx *Context, interval int) error {
	if interval < 1 {
		return fmt.Errorf("--interval must be at least 1")
	}
	ctx.Output.WriteInfo("Watching status (Ctrl+C to stop)...")
	_ = ctx.Output.WriteStringln("")

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	if err := showStatus(ctx); err != nil {
		return err
	}
	for range ticker.C {
		_ = ctx.Output.WriteString("\033[H\033[2J")
		if err := showStatus(ctx); err != nil {
			ctx.Output.WriteError("Failed to get status: %v", err)
		}
	}
	return nil
}

func gatherSystemStatus(ctx *Context) *SystemStatus {
	status := &SystemStatus{Context: ctx.Config.CurrentContext, Version: version, CheckedAt: time.Now()}
	if ctx.Context != "" {
		status.Context = ctx.Context
	}

	if v, err := akiraVerdicts(ctx); err != nil {
		status.ServiceErr = err.Error()
	} else {
		status.Services = v
	}

	target, err := cuirassURL(ctx, "/api/v1/containers")
	var list []ContainerInfo
	if err == nil {
		err = callJSON(http.MethodGet, target, nil, &list)
	}
	if err != nil {
		status.ContainErr = "cuirass: " + err.Error()
	} else {
		status.Containers = map[string]int{}
		for _, c := range list {
			status.Containers[c.status()]++
		}
	}
	return status
}

func renderStatus(ctx *Context, status *SystemStatus) error {
	w := ctx.Output
	color := w.Color()

	_ = w.WriteStringln(output.Banner("Unheaded Kingdom Status - "+status.CheckedAt.Format("2006-01-02 15:04:05"), color))
	kv := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(2)
	kv.Add("Context", status.Context)
	kv.Add("CLI version", status.Version)
	kv.Render()

	_ = w.WriteStringln("")
	_ = w.WriteStringln(output.Bold("Services (Akira consensus)"))
	if status.ServiceErr != "" {
		w.WriteError("  %s", status.ServiceErr)
	} else if len(status.Services) == 0 {
		_ = w.WriteStringln("  Akira reports no services")
	} else {
		t := output.NewTable(w.Out()).SetColor(color)
		(&ServiceListResult{Services: status.Services}).WriteTable(t, false)
		t.Render()
	}

	_ = w.WriteStringln("")
	_ = w.WriteStringln(output.Bold("Containers (Cuirass desired state)"))
	if status.ContainErr != "" {
		w.WriteError("  %s", status.ContainErr)
	} else if len(status.Containers) == 0 {
		_ = w.WriteStringln("  none desired")
	} else {
		keys := make([]string, 0, len(status.Containers))
		for k := range status.Containers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ckv := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(2)
		for _, k := range keys {
			ckv.Add(k, fmt.Sprintf("%d", status.Containers[k]))
		}
		ckv.Render()
	}
	return nil
}
