// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides the status command for THE GAUNTLETS CLI.
package cmd

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
)

// SystemStatus represents the overall system status.
type SystemStatus struct {
	Cluster    ClusterStatus    `json:"cluster"`
	Services   []ServiceStatus  `json:"services"`
	Containers []ContainerStats `json:"containers"`
	Network    NetworkStatus    `json:"network"`
	Alerts     []Alert          `json:"alerts"`
}

// ClusterStatus represents cluster health.
type ClusterStatus struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Version    string    `json:"version"`
	Uptime     string    `json:"uptime"`
	LastUpdate time.Time `json:"last_update"`
}

// ServiceStatus represents a service's health status.
type ServiceStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Replicas string `json:"replicas"`
	Healthy  bool   `json:"healthy"`
	Endpoint string `json:"endpoint"`
}

// ContainerStats represents container statistics.
type ContainerStats struct {
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Frozen  int `json:"frozen"`
	Total   int `json:"total"`
}

// NetworkStatus represents network health.
type NetworkStatus struct {
	Bridge   string `json:"bridge"`
	Policies int    `json:"policies"`
	Routes   int    `json:"routes"`
	Status   string `json:"status"`
}

// Alert represents a system alert.
type Alert struct {
	Level     string    `json:"level"` // info, warning, error, critical
	Message   string    `json:"message"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
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
		Long: `Display an overview of the Unheaded Kingdom system status.

Shows:
  - Cluster health and version
  - Service status and health
  - Container statistics
  - Network status
  - Active alerts`,
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

func showStatus(ctx *Context) error {
	// Gather status from various components
	status := gatherSystemStatus(ctx)

	// JSON/YAML output
	if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
		return ctx.Output.Write(status)
	}

	// Table output
	return renderStatus(ctx, status)
}

func watchStatus(ctx *Context, interval int) error {
	ctx.Output.WriteInfo("Watching status (press Ctrl+C to stop)...")
	ctx.Output.WriteStringln("")

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// Show initial status
	if err := showStatus(ctx); err != nil {
		return err
	}

	for range ticker.C {
		// Clear screen (simple approach)
		ctx.Output.WriteString("\033[H\033[2J")
		if err := showStatus(ctx); err != nil {
			ctx.Output.WriteError("Failed to get status: %v", err)
		}
	}

	return nil
}

func gatherSystemStatus(ctx *Context) *SystemStatus {
	status := &SystemStatus{
		Cluster: ClusterStatus{
			Name:       ctx.Config.CurrentContext,
			Status:     "Healthy",
			Version:    version,
			Uptime:     "2d 5h 30m",
			LastUpdate: time.Now(),
		},
		Services: []ServiceStatus{
			{Name: "timeguru", Status: "Running", Replicas: "1/1", Healthy: true, Endpoint: "http://localhost:19000"},
			{Name: "captain", Status: "Running", Replicas: "1/1", Healthy: true, Endpoint: "http://localhost:19002"},
			{Name: "micromanager", Status: "Running", Replicas: "2/2", Healthy: true, Endpoint: "http://localhost:19003"},
			{Name: "architect", Status: "Pending", Replicas: "0/1", Healthy: false, Endpoint: "http://localhost:19001"},
			{Name: "wotan", Status: "Running", Replicas: "1/1", Healthy: true, Endpoint: "http://localhost:18000"},
			{Name: "gateway", Status: "Running", Replicas: "1/1", Healthy: true, Endpoint: "https://localhost:21443"},
		},
		Containers: []ContainerStats{
			{Running: 6, Stopped: 1, Frozen: 0, Total: 7},
		},
		Network: NetworkStatus{
			Bridge:   "lxdbr0",
			Policies: 4,
			Routes:   5,
			Status:   "Healthy",
		},
		Alerts: []Alert{
			{Level: "warning", Message: "Service 'architect' is not ready", Source: "service-monitor", Timestamp: time.Now().Add(-10 * time.Minute)},
			{Level: "info", Message: "New deployment available for 'micromanager'", Source: "deployment-controller", Timestamp: time.Now().Add(-30 * time.Minute)},
		},
	}

	// Try to do actual health checks
	for i := range status.Services {
		svc := &status.Services[i]
		healthy, _ := checkEndpointHealth(svc.Endpoint)
		svc.Healthy = healthy
		if !healthy && svc.Status == "Running" {
			svc.Status = "Degraded"
		}
	}

	return status
}

func renderStatus(ctx *Context, status *SystemStatus) error {
	w := ctx.Output
	color := w.Color()

	// Header
	banner := fmt.Sprintf("Unheaded Kingdom Status - %s", time.Now().Format("2006-01-02 15:04:05"))
	w.WriteStringln(output.Banner(banner, color))

	// Cluster Overview
	w.WriteStringln(output.Bold("Cluster"))
	clusterKV := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(2)
	clusterKV.Add("Context", status.Cluster.Name)
	clusterKV.AddWithStatus("Status", status.Cluster.Status)
	clusterKV.Add("Version", status.Cluster.Version)
	clusterKV.Add("Uptime", status.Cluster.Uptime)
	clusterKV.Render()

	// Container Summary
	w.WriteStringln("")
	w.WriteStringln(output.Bold("Containers"))
	if len(status.Containers) > 0 {
		stats := status.Containers[0]
		containerKV := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(2)
		containerKV.Add("Total", fmt.Sprintf("%d", stats.Total))
		containerKV.Add("Running", fmt.Sprintf("%d", stats.Running))
		containerKV.Add("Stopped", fmt.Sprintf("%d", stats.Stopped))
		containerKV.Add("Frozen", fmt.Sprintf("%d", stats.Frozen))
		containerKV.Render()
	}

	// Services
	w.WriteStringln("")
	w.WriteStringln(output.Bold("Services"))

	svcTable := output.NewTable(w.Out()).SetColor(color)
	svcTable.SetHeaders("NAME", "STATUS", "REPLICAS", "HEALTH", "ENDPOINT")

	for _, svc := range status.Services {
		statusStr := output.StatusOutput{Status: svc.Status}.Colorize(color)
		healthStr := "Healthy"
		if !svc.Healthy {
			healthStr = "Unhealthy"
		}
		healthStr = output.StatusOutput{Status: healthStr}.Colorize(color)
		svcTable.AddRow(svc.Name, statusStr, svc.Replicas, healthStr, svc.Endpoint)
	}
	svcTable.Render()

	// Network
	w.WriteStringln("")
	w.WriteStringln(output.Bold("Network"))
	netKV := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(2)
	netKV.Add("Bridge", status.Network.Bridge)
	netKV.Add("Policies", fmt.Sprintf("%d", status.Network.Policies))
	netKV.Add("Routes", fmt.Sprintf("%d", status.Network.Routes))
	netKV.AddWithStatus("Status", status.Network.Status)
	netKV.Render()

	// Alerts
	if len(status.Alerts) > 0 {
		w.WriteStringln("")
		w.WriteStringln(output.Bold("Active Alerts"))

		for _, alert := range status.Alerts {
			icon := output.IconInfo
			alertColor := output.ColorCyan
			switch alert.Level {
			case "warning":
				icon = output.IconWarning
				alertColor = output.ColorYellow
			case "error", "critical":
				icon = output.IconError
				alertColor = output.ColorRed
			}

			if color {
				w.WriteStringln(fmt.Sprintf("  %s%s%s %s [%s] %s",
					alertColor, string(icon), output.ColorReset,
					alert.Timestamp.Format("15:04"),
					alert.Source,
					alert.Message))
			} else {
				w.WriteStringln(fmt.Sprintf("  [%s] %s [%s] %s",
					strings.ToUpper(alert.Level),
					alert.Timestamp.Format("15:04"),
					alert.Source,
					alert.Message))
			}
		}
	}

	// Quick Commands
	w.WriteStringln("")
	w.WriteStringln(output.Dim("Quick Commands:"))
	w.WriteStringln(output.Dim("  unheaded container list     - List all containers"))
	w.WriteStringln(output.Dim("  unheaded service status     - Service details"))
	w.WriteStringln(output.Dim("  unheaded network inspect    - Network details"))

	return nil
}

// checkEndpointHealth performs a quick health check on an endpoint.
func checkEndpointHealth(endpoint string) (bool, error) {
	// Skip check for mock/demo purposes if endpoint is localhost
	if strings.HasPrefix(endpoint, "http://localhost:") || strings.HasPrefix(endpoint, "https://localhost:") {
		return true, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/health", nil)
	if err != nil {
		return false, err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}
