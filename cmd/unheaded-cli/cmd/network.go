// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides network commands for THE GAUNTLETS CLI.
package cmd

// No Unheaded service manages network policy or routes yet. These commands
// used to print sample policies and firewall rules ("DENY all other
// traffic") whatever the host really had; now they say so.

const networkInstead = "host firewalls are deploy/nftables/<host>.nft (nft list ruleset); routes: ip route, ip -6 route"

// NewNetworkCommand creates the network command.
func NewNetworkCommand() *Command {
	cmd := &Command{
		Name:        "network",
		Short:       "Network policies and routes (not available)",
		Long:        "No Unheaded service manages network policy or routes yet; " + networkInstead + ".",
		Aliases:     []string{"net"},
		SubCommands: make(map[string]*Command),
	}

	policy := &Command{Name: "policy", Short: "Network policies (not available)", Aliases: []string{"pol", "policies"}, SubCommands: make(map[string]*Command)}
	policy.AddCommand(noAPICommand("network policy list", "List network policies", "unheaded network policy list", networkInstead, "ls"))
	policy.AddCommand(noAPICommand("network policy create", "Create a network policy", "unheaded network policy create <name>", networkInstead))
	policy.AddCommand(noAPICommand("network policy delete", "Delete a network policy", "unheaded network policy delete <name>", networkInstead, "rm"))
	policy.AddCommand(noAPICommand("network policy apply", "Apply a network policy from file", "unheaded network policy apply <file>", networkInstead))

	route := &Command{Name: "route", Short: "Network routes (not available)", Aliases: []string{"routes"}, SubCommands: make(map[string]*Command)}
	route.AddCommand(noAPICommand("network route list", "List network routes", "unheaded network route list", networkInstead, "ls"))
	route.AddCommand(noAPICommand("network route add", "Add a network route", "unheaded network route add <name>", networkInstead))
	route.AddCommand(noAPICommand("network route delete", "Delete a network route", "unheaded network route delete <name>", networkInstead, "rm"))

	cmd.AddCommand(policy)
	cmd.AddCommand(route)
	cmd.AddCommand(noAPICommand("network inspect", "Inspect network configuration", "unheaded network inspect", networkInstead, "info"))
	return cmd
}
