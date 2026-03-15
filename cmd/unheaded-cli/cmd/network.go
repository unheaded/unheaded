// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides network commands for THE GAUNTLETS CLI.
package cmd

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
)

// NetworkPolicyListResult represents the output for network policy list.
type NetworkPolicyListResult struct {
	Policies []NetworkPolicyInfo `json:"policies"`
}

// NetworkPolicyInfo represents network policy information for output.
type NetworkPolicyInfo struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Selector  string    `json:"selector"`
	Ingress   int       `json:"ingress_rules"`
	Egress    int       `json:"egress_rules"`
	CreatedAt time.Time `json:"created_at"`
}

// WriteTable implements output.TableWriter.
func (r *NetworkPolicyListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("NAME", "NAMESPACE", "SELECTOR", "INGRESS", "EGRESS", "AGE")
	} else {
		t.SetHeaders("NAME", "NAMESPACE", "INGRESS", "EGRESS", "AGE")
	}

	for _, p := range r.Policies {
		age := formatAge(p.CreatedAt)
		if wide {
			t.AddRow(p.Name, p.Namespace, p.Selector, fmt.Sprintf("%d", p.Ingress), fmt.Sprintf("%d", p.Egress), age)
		} else {
			t.AddRow(p.Name, p.Namespace, fmt.Sprintf("%d", p.Ingress), fmt.Sprintf("%d", p.Egress), age)
		}
	}
}

// RouteListResult represents the output for route list.
type RouteListResult struct {
	Routes []RouteInfo `json:"routes"`
}

// RouteInfo represents route information for output.
type RouteInfo struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Protocol    string `json:"protocol"`
	Port        int    `json:"port"`
	Weight      int    `json:"weight"`
	Status      string `json:"status"`
}

// WriteTable implements output.TableWriter.
func (r *RouteListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("NAME", "SOURCE", "DESTINATION", "PROTOCOL", "PORT", "WEIGHT", "STATUS")
	} else {
		t.SetHeaders("NAME", "DESTINATION", "PORT", "STATUS")
	}

	for _, route := range r.Routes {
		status := output.StatusOutput{Status: route.Status}.Colorize(t.Color)
		if wide {
			t.AddRow(route.Name, route.Source, route.Destination, route.Protocol, fmt.Sprintf("%d", route.Port), fmt.Sprintf("%d%%", route.Weight), status)
		} else {
			t.AddRow(route.Name, route.Destination, fmt.Sprintf("%d", route.Port), status)
		}
	}
}

// NewNetworkCommand creates the network command.
func NewNetworkCommand() *Command {
	cmd := &Command{
		Name:        "network",
		Short:       "Manage network policies and routes",
		Long:        "Commands for managing network policies and routing in the Unheaded Kingdom.",
		Aliases:     []string{"net"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newNetworkPolicyCommand())
	cmd.AddCommand(newNetworkRouteCommand())
	cmd.AddCommand(newNetworkInspectCommand())

	return cmd
}

// Network Policy subcommands

func newNetworkPolicyCommand() *Command {
	cmd := &Command{
		Name:        "policy",
		Short:       "Manage network policies",
		Aliases:     []string{"pol", "policies"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newNetworkPolicyListCommand())
	cmd.AddCommand(newNetworkPolicyCreateCommand())
	cmd.AddCommand(newNetworkPolicyDeleteCommand())
	cmd.AddCommand(newNetworkPolicyApplyCommand())

	return cmd
}

func newNetworkPolicyListCommand() *Command {
	var namespace string

	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.StringVar(&namespace, "namespace", "", "Filter by namespace")
	flags.StringVar(&namespace, "n", "", "Namespace (short)")

	return &Command{
		Name:    "list",
		Short:   "List network policies",
		Usage:   "unheaded network policy list [flags]",
		Aliases: []string{"ls"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			// Mock data for demonstration
			result := &NetworkPolicyListResult{
				Policies: []NetworkPolicyInfo{
					{
						Name:      "default-deny-all",
						Namespace: "default",
						Selector:  "all",
						Ingress:   0,
						Egress:    0,
						CreatedAt: time.Now().Add(-48 * time.Hour),
					},
					{
						Name:      "allow-internal",
						Namespace: "default",
						Selector:  "app=internal",
						Ingress:   3,
						Egress:    2,
						CreatedAt: time.Now().Add(-24 * time.Hour),
					},
					{
						Name:      "allow-wotan",
						Namespace: "services",
						Selector:  "app=wotan",
						Ingress:   5,
						Egress:    1,
						CreatedAt: time.Now().Add(-24 * time.Hour),
					},
					{
						Name:      "gateway-ingress",
						Namespace: "ingress",
						Selector:  "app=gateway",
						Ingress:   10,
						Egress:    5,
						CreatedAt: time.Now().Add(-12 * time.Hour),
					},
				},
			}

			// Filter by namespace
			if namespace != "" {
				filtered := &NetworkPolicyListResult{}
				for _, p := range result.Policies {
					if p.Namespace == namespace {
						filtered.Policies = append(filtered.Policies, p)
					}
				}
				result = filtered
			}

			return ctx.Output.Write(result)
		},
	}
}

func newNetworkPolicyCreateCommand() *Command {
	var (
		namespace string
		selector  string
		ingress   string
		egress    string
	)

	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.StringVar(&namespace, "namespace", "default", "Namespace")
	flags.StringVar(&namespace, "n", "default", "Namespace (short)")
	flags.StringVar(&selector, "selector", "", "Pod selector (label=value)")
	flags.StringVar(&selector, "s", "", "Selector (short)")
	flags.StringVar(&ingress, "ingress", "", "Ingress rules (from:port,...)")
	flags.StringVar(&ingress, "i", "", "Ingress (short)")
	flags.StringVar(&egress, "egress", "", "Egress rules (to:port,...)")
	flags.StringVar(&egress, "e", "", "Egress (short)")

	return &Command{
		Name:  "create",
		Short: "Create a network policy",
		Usage: "unheaded network policy create <name> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("policy name required")
			}
			name := args[0]

			ctx.Logger.Info().
				Str("name", name).
				Str("namespace", namespace).
				Str("selector", selector).
				Msg("creating network policy")

			// In real implementation, would call daemon API

			ctx.Output.WriteSuccess("Network policy '%s' created in namespace '%s'", name, namespace)
			return nil
		},
	}
}

func newNetworkPolicyDeleteCommand() *Command {
	var (
		namespace string
		force     bool
	)

	flags := flag.NewFlagSet("delete", flag.ContinueOnError)
	flags.StringVar(&namespace, "namespace", "default", "Namespace")
	flags.StringVar(&namespace, "n", "default", "Namespace (short)")
	flags.BoolVar(&force, "force", false, "Force deletion")
	flags.BoolVar(&force, "f", false, "Force (short)")

	return &Command{
		Name:    "delete",
		Short:   "Delete a network policy",
		Usage:   "unheaded network policy delete <name> [flags]",
		Aliases: []string{"rm"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("policy name required")
			}
			name := args[0]

			if !force {
				ctx.Output.WriteWarning("This will delete network policy '%s'. Use --force to confirm.", name)
				return nil
			}

			ctx.Logger.Info().
				Str("name", name).
				Str("namespace", namespace).
				Msg("deleting network policy")

			ctx.Output.WriteSuccess("Network policy '%s' deleted", name)
			return nil
		},
	}
}

func newNetworkPolicyApplyCommand() *Command {
	return &Command{
		Name:  "apply",
		Short: "Apply a network policy from file",
		Usage: "unheaded network policy apply -f <file>",
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("policy file required")
			}
			file := args[0]

			ctx.Logger.Info().Str("file", file).Msg("applying network policy from file")

			// In real implementation, would read file and call daemon API

			ctx.Output.WriteSuccess("Network policy applied from '%s'", file)
			return nil
		},
	}
}

// Network Route subcommands

func newNetworkRouteCommand() *Command {
	cmd := &Command{
		Name:        "route",
		Short:       "Manage network routes",
		Aliases:     []string{"routes"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newNetworkRouteListCommand())
	cmd.AddCommand(newNetworkRouteAddCommand())
	cmd.AddCommand(newNetworkRouteDeleteCommand())

	return cmd
}

func newNetworkRouteListCommand() *Command {
	return &Command{
		Name:    "list",
		Short:   "List network routes",
		Usage:   "unheaded network route list",
		Aliases: []string{"ls"},
		RunFunc: func(ctx *Context, args []string) error {
			// Mock data for demonstration
			result := &RouteListResult{
				Routes: []RouteInfo{
					{
						Name:        "gateway-to-timeguru",
						Source:      "gateway",
						Destination: "timeguru.services.local",
						Protocol:    "HTTP",
						Port:        19000,
						Weight:      100,
						Status:      "Active",
					},
					{
						Name:        "gateway-to-captain",
						Source:      "gateway",
						Destination: "captain.services.local",
						Protocol:    "HTTP",
						Port:        19002,
						Weight:      100,
						Status:      "Active",
					},
					{
						Name:        "gateway-to-micromanager",
						Source:      "gateway",
						Destination: "micromanager.services.local",
						Protocol:    "HTTP",
						Port:        19003,
						Weight:      100,
						Status:      "Active",
					},
					{
						Name:        "services-to-wotan",
						Source:      "services.*",
						Destination: "wotan.local",
						Protocol:    "gRPC",
						Port:        18001,
						Weight:      100,
						Status:      "Active",
					},
					{
						Name:        "canary-architect",
						Source:      "gateway",
						Destination: "architect-canary.services.local",
						Protocol:    "HTTP",
						Port:        19001,
						Weight:      10,
						Status:      "Pending",
					},
				},
			}

			return ctx.Output.Write(result)
		},
	}
}

func newNetworkRouteAddCommand() *Command {
	var (
		protocol string
		port     int
		weight   int
	)

	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.StringVar(&protocol, "protocol", "HTTP", "Protocol: HTTP, gRPC, TCP")
	flags.StringVar(&protocol, "p", "HTTP", "Protocol (short)")
	flags.IntVar(&port, "port", 21000, "Destination port")
	flags.IntVar(&weight, "weight", 100, "Traffic weight (0-100)")
	flags.IntVar(&weight, "w", 100, "Weight (short)")

	return &Command{
		Name:  "add",
		Short: "Add a network route",
		Usage: "unheaded network route add <name> --source <source> --dest <destination> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("route name required")
			}
			name := args[0]

			// Parse source and destination from remaining args
			var source, dest string
			for i, arg := range args[1:] {
				if arg == "--source" && i+1 < len(args)-1 {
					source = args[i+2]
				}
				if arg == "--dest" && i+1 < len(args)-1 {
					dest = args[i+2]
				}
			}

			if source == "" || dest == "" {
				return fmt.Errorf("--source and --dest are required")
			}

			ctx.Logger.Info().
				Str("name", name).
				Str("source", source).
				Str("destination", dest).
				Str("protocol", protocol).
				Int("port", port).
				Int("weight", weight).
				Msg("adding network route")

			ctx.Output.WriteSuccess("Route '%s' added: %s -> %s:%d (%s, weight=%d%%)", name, source, dest, port, protocol, weight)
			return nil
		},
	}
}

func newNetworkRouteDeleteCommand() *Command {
	return &Command{
		Name:    "delete",
		Short:   "Delete a network route",
		Usage:   "unheaded network route delete <name>",
		Aliases: []string{"rm"},
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("route name required")
			}
			name := args[0]

			ctx.Logger.Info().Str("name", name).Msg("deleting network route")

			ctx.Output.WriteSuccess("Route '%s' deleted", name)
			return nil
		},
	}
}

func newNetworkInspectCommand() *Command {
	return &Command{
		Name:    "inspect",
		Short:   "Inspect network configuration",
		Usage:   "unheaded network inspect [container]",
		Aliases: []string{"info"},
		RunFunc: func(ctx *Context, args []string) error {
			w := ctx.Output
			color := w.Color()

			w.WriteStringln(output.Bold("Network Overview"))
			w.WriteStringln("")

			// Bridge info
			w.WriteStringln(output.Bold("Bridge Configuration:"))
			kv := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(2)
			kv.Add("Name", "lxdbr0")
			kv.Add("IPv4 Address", "10.10.10.1")
			kv.Add("IPv4 CIDR", "10.10.10.0/24")
			kv.Add("IPv4 DHCP", "10.10.10.2-10.10.10.254")
			kv.Add("DNS Domain", "unheaded.local")
			kv.AddWithStatus("Status", "Active")
			kv.Render()

			w.WriteStringln("")
			w.WriteStringln(output.Bold("Key Endpoints:"))

			endpoints := output.NewTable(w.Out()).SetColor(color)
			endpoints.SetHeaders("SERVICE", "IP", "PORT", "STATUS")
			endpoints.AddRow("Gateway", "localhost", "21443", output.StatusOutput{Status: "Running"}.Colorize(color))
			endpoints.AddRow("Wotan", "localhost", "18000", output.StatusOutput{Status: "Running"}.Colorize(color))
			endpoints.AddRow("Timeguru", "localhost", "19000", output.StatusOutput{Status: "Running"}.Colorize(color))
			endpoints.AddRow("Captain", "localhost", "19002", output.StatusOutput{Status: "Running"}.Colorize(color))
			endpoints.AddRow("Micromanager", "localhost", "19003", output.StatusOutput{Status: "Running"}.Colorize(color))
			endpoints.AddRow("Architect", "localhost", "19001", output.StatusOutput{Status: "Pending"}.Colorize(color))
			endpoints.Render()

			w.WriteStringln("")
			w.WriteStringln(output.Bold("Firewall Rules:"))
			rules := output.NewListTable(w.Out()).SetColor(color).SetBullet(string(output.IconTriangle))
			rules.Add("ALLOW internal (10.10.10.0/24) -> all")
			rules.Add("ALLOW external -> gateway:21443 (TLS)")
			rules.Add("ALLOW gateway -> services:19000-19010")
			rules.Add("ALLOW services -> wotan:18001")
			rules.Add("DENY all other traffic")
			rules.Render()

			if len(args) > 0 {
				container := args[0]
				w.WriteStringln("")
				w.WriteStringln(output.Bold(fmt.Sprintf("Container Network: %s", container)))
				w.WriteInfo("Container-specific network info not yet implemented")
			}

			return nil
		},
	}
}

// parseNetworkPolicy parses a network policy from YAML content.
func parseNetworkPolicy(content string) (map[string]interface{}, error) {
	// Simple parser for demo purposes
	policy := make(map[string]interface{})

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			policy[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return policy, nil
}
