// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides service commands for THE GAUNTLETS CLI.
package cmd

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"unheaded/cmd/unheaded-cli/output"
)

// akiraVerdict mirrors pkg/health.Verdict: Akira's consensus on one
// service, from every reporter's latest health check (ADR-029).
type akiraVerdict struct {
	Service          string   `json:"service"`
	Reporters        int      `json:"reporters"`
	Failing          int      `json:"failing"`
	FailureRate      float64  `json:"failure_rate"`
	Severity         string   `json:"severity"`
	Remediate        bool     `json:"remediate"`
	FailingReporters []string `json:"failing_reporters,omitempty"`
}

// ServiceListResult represents the output for service list.
type ServiceListResult struct {
	Services []akiraVerdict `json:"services"`
}

// WriteTable implements output.TableWriter.
func (r *ServiceListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("NAME", "SEVERITY", "FAILING", "REPORTERS", "FAILURE %", "FAILING REPORTERS")
	} else {
		t.SetHeaders("NAME", "SEVERITY", "FAILING", "REPORTERS")
	}
	for _, v := range r.Services {
		sev := output.StatusOutput{Status: severityStatus(v.Severity)}.Colorize(t.Color)
		if wide {
			t.AddRow(v.Service, sev, strconv.Itoa(v.Failing), strconv.Itoa(v.Reporters),
				fmt.Sprintf("%.1f", 100*v.FailureRate), joinOrDash(v.FailingReporters))
		} else {
			t.AddRow(v.Service, sev, strconv.Itoa(v.Failing), strconv.Itoa(v.Reporters))
		}
	}
}

// severityStatus maps a consensus band to a word output.StatusOutput colours.
func severityStatus(s string) string {
	switch s {
	case "OK":
		return "Healthy"
	case "WARN":
		return "Warning"
	case "":
		return "Unknown"
	default:
		return "Error"
	}
}

func joinOrDash(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	out := s[0]
	for _, x := range s[1:] {
		out += ", " + x
	}
	return out
}

// NewServiceCommand creates the service command.
func NewServiceCommand() *Command {
	cmd := &Command{
		Name:  "service",
		Short: "Service health and logs",
		Long: `Service health from Akira's consensus (the context's akira endpoint) and
logs from the dashboard's log buffer (the context's dashboard endpoint).

Deploy, restart and scale fail: no Unheaded service does them yet.`,
		Aliases:     []string{"svc", "services"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newServiceListCommand())
	cmd.AddCommand(newServiceStatusCommand())
	cmd.AddCommand(newServiceLogsCommand())
	for _, c := range []struct{ name, short, usage string }{
		{"deploy", "Deploy a service", "unheaded service deploy <name>"},
		{"restart", "Restart a service", "unheaded service restart <name>"},
		{"scale", "Scale a service", "unheaded service scale <name> <replicas>"},
	} {
		cmd.AddCommand(noAPICommand("service "+c.name, c.short, c.usage,
			"services run under docker compose (docker compose up -d <service>) or systemd on the hosts"))
	}

	return cmd
}

// akiraVerdicts fetches Akira's current verdicts, sorted by service.
func akiraVerdicts(ctx *Context) ([]akiraVerdict, error) {
	c, err := ctx.cluster()
	if err != nil {
		return nil, err
	}
	target, err := serviceURL(c.Akira, "/api/v1/status")
	if err != nil {
		return nil, fmt.Errorf("akira endpoint: %w", err)
	}
	var states map[string]akiraVerdict
	if err := callJSON(http.MethodGet, target, nil, &states); err != nil {
		return nil, fmt.Errorf("akira: %w", err)
	}
	out := make([]akiraVerdict, 0, len(states))
	for name, v := range states {
		if v.Service == "" {
			v.Service = name
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out, nil
}

func newServiceListCommand() *Command {
	return &Command{
		Name:    "list",
		Short:   "List services Akira checks, with their consensus health",
		Usage:   "unheaded service list",
		Aliases: []string{"ls"},
		RunFunc: func(ctx *Context, args []string) error {
			verdicts, err := akiraVerdicts(ctx)
			if err != nil {
				return err
			}
			return ctx.Output.Write(&ServiceListResult{Services: verdicts})
		},
	}
}

func newServiceStatusCommand() *Command {
	return &Command{
		Name:    "status",
		Short:   "Show one service's consensus health",
		Usage:   "unheaded service status <name>",
		Aliases: []string{"info"},
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly one service name required")
			}
			verdicts, err := akiraVerdicts(ctx)
			if err != nil {
				return err
			}
			for _, v := range verdicts {
				if v.Service != args[0] {
					continue
				}
				if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
					return ctx.Output.Write(v)
				}
				w := ctx.Output
				_ = w.WriteStringln(output.Bold("Service: " + v.Service))
				kv := output.NewKeyValueTable(w.Out()).SetColor(w.Color())
				kv.AddWithStatus("Severity", severityStatus(v.Severity))
				kv.Add("Band", v.Severity)
				kv.Add("Failing", fmt.Sprintf("%d of %d reporters (%.1f%%)", v.Failing, v.Reporters, 100*v.FailureRate))
				kv.Add("Failing reporters", joinOrDash(v.FailingReporters))
				kv.Add("Remediation threshold", strconv.FormatBool(v.Remediate))
				kv.Render()
				return nil
			}
			return fmt.Errorf("service %s: Akira does not check it (unheaded service list)", args[0])
		},
	}
}

// logEntry mirrors pkg/logagg.LogEntry.
type logEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Service   string         `json:"service"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
}

func newServiceLogsCommand() *Command {
	var (
		tail   int
		level  string
		search string
		since  string
	)

	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.IntVar(&tail, "tail", 100, "Number of entries (max 10000)")
	flags.IntVar(&tail, "n", 100, "Number of entries (short)")
	flags.StringVar(&level, "level", "", "Only this level (debug, info, warn, error)")
	flags.StringVar(&search, "search", "", "Full-text search")
	flags.StringVar(&since, "since", "", "Only entries newer than this (e.g. 10m, 2h, 1d)")

	return &Command{
		Name:  "logs",
		Short: "Show a service's logs from the dashboard's log buffer",
		Usage: "unheaded service logs <name> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly one service name required")
			}
			if tail < 1 || tail > 10000 {
				return fmt.Errorf("--tail must be 1..10000")
			}
			q := url.Values{"service": {args[0]}, "limit": {strconv.Itoa(tail)}, "newest": {"true"}}
			if level != "" {
				q.Set("level", level)
			}
			if search != "" {
				q.Set("search", search)
			}
			if since != "" {
				d, err := parseDuration(since)
				if err != nil || d <= 0 {
					return fmt.Errorf("invalid --since %q", since)
				}
				q.Set("from", strconv.FormatInt(time.Now().Add(-d).Unix(), 10))
			}
			c, err := ctx.cluster()
			if err != nil {
				return err
			}
			target, err := serviceURL(c.Dashboard, "/api/v1/logs?"+q.Encode())
			if err != nil {
				return fmt.Errorf("dashboard endpoint: %w", err)
			}
			var entries []logEntry
			if err := callJSON(http.MethodGet, target, nil, &entries); err != nil {
				return fmt.Errorf("logs: %w", err)
			}
			if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
				return ctx.Output.Write(entries)
			}
			for _, e := range entries {
				_ = ctx.Output.WriteStringln(fmt.Sprintf("%s %-5s %s", e.Timestamp.Format(time.RFC3339), e.Level, e.Message))
			}
			return nil
		},
	}
}
