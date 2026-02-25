// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package cmd provides service commands for THE GAUNTLETS CLI.
package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
)

// ServiceListResult represents the output for service list.
type ServiceListResult struct {
	Services []ServiceInfo `json:"services"`
}

// ServiceInfo represents service information for output.
type ServiceInfo struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Replicas  string    `json:"replicas"`
	Image     string    `json:"image"`
	Endpoint  string    `json:"endpoint"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WriteTable implements output.TableWriter.
func (r *ServiceListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("NAME", "STATUS", "REPLICAS", "IMAGE", "ENDPOINT", "AGE", "UPDATED")
	} else {
		t.SetHeaders("NAME", "STATUS", "REPLICAS", "ENDPOINT", "AGE")
	}

	for _, s := range r.Services {
		status := output.StatusOutput{Status: s.Status}.Colorize(t.Color)
		age := formatAge(s.CreatedAt)

		if wide {
			updated := formatAge(s.UpdatedAt)
			t.AddRow(s.Name, status, s.Replicas, s.Image, s.Endpoint, age, updated)
		} else {
			t.AddRow(s.Name, status, s.Replicas, s.Endpoint, age)
		}
	}
}

// NewServiceCommand creates the service command.
func NewServiceCommand() *Command {
	cmd := &Command{
		Name:        "service",
		Short:       "Manage services",
		Long:        "Commands for managing services in the Unheaded Kingdom.",
		Aliases:     []string{"svc", "services"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newServiceListCommand())
	cmd.AddCommand(newServiceDeployCommand())
	cmd.AddCommand(newServiceStatusCommand())
	cmd.AddCommand(newServiceLogsCommand())
	cmd.AddCommand(newServiceRestartCommand())
	cmd.AddCommand(newServiceScaleCommand())

	return cmd
}

func newServiceListCommand() *Command {
	var namespace string

	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.StringVar(&namespace, "namespace", "default", "Namespace to list services from")
	flags.StringVar(&namespace, "n", "default", "Namespace (short)")

	return &Command{
		Name:    "list",
		Short:   "List services",
		Usage:   "unheaded service list [flags]",
		Aliases: []string{"ls"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			// In a real implementation, this would query the daemon API
			// For now, return mock data to demonstrate the output format

			result := &ServiceListResult{
				Services: []ServiceInfo{
					{
						Name:      "timeguru",
						Status:    "Running",
						Replicas:  "1/1",
						Image:     "unheaded/timeguru:latest",
						Endpoint:  "http://localhost:19000",
						CreatedAt: time.Now().Add(-24 * time.Hour),
						UpdatedAt: time.Now().Add(-2 * time.Hour),
					},
					{
						Name:      "captain",
						Status:    "Running",
						Replicas:  "1/1",
						Image:     "unheaded/captain:latest",
						Endpoint:  "http://localhost:19002",
						CreatedAt: time.Now().Add(-24 * time.Hour),
						UpdatedAt: time.Now().Add(-1 * time.Hour),
					},
					{
						Name:      "micromanager",
						Status:    "Running",
						Replicas:  "2/2",
						Image:     "unheaded/micromanager:latest",
						Endpoint:  "http://localhost:19003",
						CreatedAt: time.Now().Add(-24 * time.Hour),
						UpdatedAt: time.Now().Add(-30 * time.Minute),
					},
					{
						Name:      "architect",
						Status:    "Pending",
						Replicas:  "0/1",
						Image:     "unheaded/architect:latest",
						Endpoint:  "http://localhost:19001",
						CreatedAt: time.Now().Add(-1 * time.Hour),
						UpdatedAt: time.Now().Add(-1 * time.Hour),
					},
					{
						Name:      "wotan",
						Status:    "Running",
						Replicas:  "1/1",
						Image:     "unheaded/wotan:latest",
						Endpoint:  "http://localhost:18000",
						CreatedAt: time.Now().Add(-48 * time.Hour),
						UpdatedAt: time.Now().Add(-24 * time.Hour),
					},
				},
			}

			return ctx.Output.Write(result)
		},
	}
}

func newServiceDeployCommand() *Command {
	var (
		image    string
		replicas int
		env      string
		port     int
		dryRun   bool
	)

	flags := flag.NewFlagSet("deploy", flag.ContinueOnError)
	flags.StringVar(&image, "image", "", "Container image to deploy")
	flags.StringVar(&image, "i", "", "Container image (short)")
	flags.IntVar(&replicas, "replicas", 1, "Number of replicas")
	flags.IntVar(&replicas, "r", 1, "Replicas (short)")
	flags.StringVar(&env, "env", "", "Environment variables (KEY=VALUE,...)")
	flags.StringVar(&env, "e", "", "Environment variables (short)")
	flags.IntVar(&port, "port", 19000, "Container port to expose")
	flags.IntVar(&port, "p", 19000, "Port (short)")
	flags.BoolVar(&dryRun, "dry-run", false, "Show what would be deployed without actually deploying")

	return &Command{
		Name:  "deploy",
		Short: "Deploy a service",
		Usage: "unheaded service deploy <name> --image <image> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("service name required")
			}
			name := args[0]

			if image == "" {
				return fmt.Errorf("--image flag is required")
			}

			// Parse environment variables
			envMap := make(map[string]string)
			if env != "" {
				for _, pair := range strings.Split(env, ",") {
					parts := strings.SplitN(pair, "=", 2)
					if len(parts) == 2 {
						envMap[parts[0]] = parts[1]
					}
				}
			}

			if dryRun {
				w := ctx.Output
				w.WriteStringln(output.Bold("Dry Run - Service Deployment Plan:"))
				w.WriteStringln("")

				kv := output.NewKeyValueTable(w.Out()).SetColor(w.Color())
				kv.Add("Name", name)
				kv.Add("Image", image)
				kv.Add("Replicas", fmt.Sprintf("%d", replicas))
				kv.Add("Port", fmt.Sprintf("%d", port))
				if len(envMap) > 0 {
					for k, v := range envMap {
						kv.Add(fmt.Sprintf("ENV[%s]", k), v)
					}
				}
				kv.Render()

				w.WriteStringln("")
				w.WriteInfo("No changes made (dry run)")
				return nil
			}

			ctx.Logger.Info().
				Str("name", name).
				Str("image", image).
				Int("replicas", replicas).
				Msg("deploying service")

			// In real implementation, this would call the daemon API

			ctx.Output.WriteSuccess("Service '%s' deployment initiated", name)
			ctx.Output.WriteInfo("Use 'unheaded service status %s' to check deployment progress", name)
			return nil
		},
	}
}

func newServiceStatusCommand() *Command {
	return &Command{
		Name:    "status",
		Short:   "Show service status",
		Usage:   "unheaded service status <name>",
		Aliases: []string{"describe"},
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("service name required")
			}
			name := args[0]

			// Mock service status
			service := ServiceInfo{
				Name:      name,
				Status:    "Running",
				Replicas:  "2/2",
				Image:     fmt.Sprintf("unheaded/%s:latest", name),
				Endpoint:  fmt.Sprintf("http://localhost:1900%d", len(name)%10),
				CreatedAt: time.Now().Add(-24 * time.Hour),
				UpdatedAt: time.Now().Add(-30 * time.Minute),
			}

			if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
				return ctx.Output.Write(service)
			}

			w := ctx.Output
			color := w.Color()

			w.WriteStringln(output.Bold("Service: " + name))
			w.WriteStringln("")

			kv := output.NewKeyValueTable(w.Out()).SetColor(color)
			kv.Add("Name", service.Name)
			kv.AddWithStatus("Status", service.Status)
			kv.Add("Replicas", service.Replicas)
			kv.Add("Image", service.Image)
			kv.Add("Endpoint", service.Endpoint)
			kv.Add("Created", service.CreatedAt.Format(time.RFC3339))
			kv.Add("Updated", service.UpdatedAt.Format(time.RFC3339))
			kv.Render()

			w.WriteStringln("")
			w.WriteStringln(output.Bold("Instances:"))

			table := output.NewTable(w.Out()).SetColor(color)
			table.SetHeaders("INSTANCE", "STATUS", "NODE", "IP", "AGE")
			table.AddRow(name+"-1", output.StatusOutput{Status: "Running"}.Colorize(color), "node-1", "localhost", "24h")
			table.AddRow(name+"-2", output.StatusOutput{Status: "Running"}.Colorize(color), "node-1", "localhost", "2h")
			table.Render()

			w.WriteStringln("")
			w.WriteStringln(output.Bold("Recent Events:"))

			events := output.NewListTable(w.Out()).SetColor(color)
			events.Add(fmt.Sprintf("%s - Instance %s-2 started", time.Now().Add(-2*time.Hour).Format("15:04"), name))
			events.Add(fmt.Sprintf("%s - Scaling from 1 to 2 replicas", time.Now().Add(-2*time.Hour).Format("15:04")))
			events.Add(fmt.Sprintf("%s - Health check passed", time.Now().Add(-24*time.Hour).Format("15:04")))
			events.Render()

			return nil
		},
	}
}

func newServiceLogsCommand() *Command {
	var (
		follow    bool
		tail      int
		since     string
		container string
	)

	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.BoolVar(&follow, "follow", false, "Follow log output")
	flags.BoolVar(&follow, "f", false, "Follow log output (short)")
	flags.IntVar(&tail, "tail", 100, "Number of lines to show")
	flags.IntVar(&tail, "n", 100, "Number of lines (short)")
	flags.StringVar(&since, "since", "", "Show logs since timestamp (e.g., 2024-01-01T00:00:00Z) or relative (e.g., 1h)")
	flags.StringVar(&container, "container", "", "Specific container instance")
	flags.StringVar(&container, "c", "", "Container instance (short)")

	return &Command{
		Name:  "logs",
		Short: "View service logs",
		Usage: "unheaded service logs <name> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("service name required")
			}
			name := args[0]

			ctx.Logger.Debug().
				Str("service", name).
				Bool("follow", follow).
				Int("tail", tail).
				Str("since", since).
				Msg("fetching logs")

			// In a real implementation, this would stream logs from the service
			// For demo, show mock log output

			w := ctx.Output

			mockLogs := []string{
				fmt.Sprintf("[%s] %s | INFO  | Service started on port 19000", time.Now().Add(-30*time.Minute).Format("2006-01-02 15:04:05"), name),
				fmt.Sprintf("[%s] %s | INFO  | Connected to Wotan at localhost:18001", time.Now().Add(-30*time.Minute).Format("2006-01-02 15:04:05"), name),
				fmt.Sprintf("[%s] %s | INFO  | Health check endpoint ready", time.Now().Add(-29*time.Minute).Format("2006-01-02 15:04:05"), name),
				fmt.Sprintf("[%s] %s | INFO  | Processing request GET /api/v1/status", time.Now().Add(-15*time.Minute).Format("2006-01-02 15:04:05"), name),
				fmt.Sprintf("[%s] %s | DEBUG | Request completed in 12ms", time.Now().Add(-15*time.Minute).Format("2006-01-02 15:04:05"), name),
				fmt.Sprintf("[%s] %s | INFO  | Processing request POST /api/v1/events", time.Now().Add(-10*time.Minute).Format("2006-01-02 15:04:05"), name),
				fmt.Sprintf("[%s] %s | DEBUG | Event published to Wotan", time.Now().Add(-10*time.Minute).Format("2006-01-02 15:04:05"), name),
				fmt.Sprintf("[%s] %s | INFO  | Health check: OK", time.Now().Add(-5*time.Minute).Format("2006-01-02 15:04:05"), name),
			}

			// Show only tail lines
			start := 0
			if tail > 0 && tail < len(mockLogs) {
				start = len(mockLogs) - tail
			}

			for _, log := range mockLogs[start:] {
				w.WriteStringln(log)
			}

			if follow {
				w.WriteInfo("Following logs... (press Ctrl+C to stop)")
				// In real implementation, would start log streaming here
			}

			return nil
		},
	}
}

func newServiceRestartCommand() *Command {
	var rolling bool

	flags := flag.NewFlagSet("restart", flag.ContinueOnError)
	flags.BoolVar(&rolling, "rolling", true, "Perform rolling restart")
	flags.BoolVar(&rolling, "r", true, "Rolling restart (short)")

	return &Command{
		Name:  "restart",
		Short: "Restart a service",
		Usage: "unheaded service restart <name> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("service name required")
			}
			name := args[0]

			ctx.Logger.Info().
				Str("service", name).
				Bool("rolling", rolling).
				Msg("restarting service")

			if rolling {
				ctx.Output.WriteInfo("Performing rolling restart of '%s'...", name)
			} else {
				ctx.Output.WriteWarning("Performing hard restart of '%s' (service will be unavailable)", name)
			}

			// In real implementation, would call daemon API
			time.Sleep(500 * time.Millisecond) // Simulate work

			ctx.Output.WriteSuccess("Service '%s' restart initiated", name)
			return nil
		},
	}
}

func newServiceScaleCommand() *Command {
	return &Command{
		Name:  "scale",
		Short: "Scale a service",
		Usage: "unheaded service scale <name> <replicas>",
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("service name and replica count required")
			}
			name := args[0]
			replicas := args[1]

			ctx.Logger.Info().
				Str("service", name).
				Str("replicas", replicas).
				Msg("scaling service")

			// In real implementation, would call daemon API

			ctx.Output.WriteSuccess("Service '%s' scaled to %s replicas", name, replicas)
			return nil
		},
	}
}

// checkServiceHealth performs a health check on a service endpoint.
func checkServiceHealth(endpoint string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/health", nil)
	if err != nil {
		return false, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	_ = body // For debug if needed

	return resp.StatusCode == http.StatusOK, nil
}
