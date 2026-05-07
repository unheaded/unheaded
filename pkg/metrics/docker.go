// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// DockerCollector gathers metrics from the Docker daemon via its Unix socket.
// It operates in graceful degraded mode: if the socket is unavailable,
// it returns availability=0 instead of failing.
type DockerCollector struct {
	// socketPath is the path to the Docker Unix socket (default: "/var/run/docker.sock")
	socketPath string
	// timeout for HTTP requests to Docker
	timeout time.Duration
}

// NewDockerCollector creates a new Docker collector.
func NewDockerCollector() *DockerCollector {
	return &DockerCollector{
		socketPath: "/var/run/docker.sock",
		timeout:    5 * time.Second,
	}
}

// Name returns the collector's name.
func (d *DockerCollector) Name() string {
	return "docker"
}

// Describe returns descriptions of the metrics this collector produces.
func (d *DockerCollector) Describe() []string {
	return []string{
		"unheaded_docker_available - Whether Docker metrics are available (0 or 1)",
		"unheaded_docker_cpu_percent - CPU usage percentage per container",
		"unheaded_docker_memory_usage - Memory usage in bytes per container",
		"unheaded_docker_memory_limit - Memory limit in bytes per container",
		"unheaded_docker_network_rx_bytes - Network received bytes per container",
		"unheaded_docker_network_tx_bytes - Network transmitted bytes per container",
	}
}

// Collect gathers metrics from Docker or returns availability=0 if unavailable.
func (d *DockerCollector) Collect(ctx context.Context) ([]Sample, error) {
	var samples []Sample
	baseLabels := map[string]string{"collector": "docker"}

	// Get list of containers
	containers, err := d.listContainers(ctx)
	if err != nil {
		// Graceful degradation
		fmt.Fprintf(os.Stderr, "Warning: Docker metrics unavailable: %v\n", err)
		samples = append(samples, Sample{
			Name:   "unheaded_docker_available",
			Help:   "Docker metrics availability",
			Type:   TypeGauge,
			Labels: baseLabels,
			Value:  0,
		})
		return samples, nil
	}

	// Collect stats for each container
	for _, container := range containers {
		containerID := container.ID
		if len(containerID) > 12 {
			containerID = containerID[:12]
		}

		stats, err := d.getContainerStats(ctx, container.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get stats for container %s: %v\n", container.ID, err)
			continue
		}

		labels := map[string]string{
			"collector":      "docker",
			"container_id":   containerID,
			"container_name": strings.TrimPrefix(container.Names[0], "/"),
			"image":          container.Image,
		}

		if stats.CPUPercent > 0 {
			samples = append(samples, Sample{
				Name:   "unheaded_docker_cpu_percent",
				Help:   "CPU usage percentage",
				Type:   TypeGauge,
				Labels: labels,
				Value:  stats.CPUPercent,
			})
		}

		if stats.MemoryUsage > 0 {
			samples = append(samples, Sample{
				Name:   "unheaded_docker_memory_usage",
				Help:   "Memory usage in bytes",
				Type:   TypeGauge,
				Labels: labels,
				Value:  float64(stats.MemoryUsage),
			})
		}

		if stats.MemoryLimit > 0 {
			samples = append(samples, Sample{
				Name:   "unheaded_docker_memory_limit",
				Help:   "Memory limit in bytes",
				Type:   TypeGauge,
				Labels: labels,
				Value:  float64(stats.MemoryLimit),
			})
		}

		if stats.NetworkRXBytes > 0 {
			samples = append(samples, Sample{
				Name:   "unheaded_docker_network_rx_bytes",
				Help:   "Network received bytes",
				Type:   TypeCounter,
				Labels: labels,
				Value:  float64(stats.NetworkRXBytes),
			})
		}

		if stats.NetworkTXBytes > 0 {
			samples = append(samples, Sample{
				Name:   "unheaded_docker_network_tx_bytes",
				Help:   "Network transmitted bytes",
				Type:   TypeCounter,
				Labels: labels,
				Value:  float64(stats.NetworkTXBytes),
			})
		}
	}

	// Add availability=1 indicator
	samples = append(samples, Sample{
		Name:   "unheaded_docker_available",
		Help:   "Docker metrics availability",
		Type:   TypeGauge,
		Labels: baseLabels,
		Value:  1,
	})

	return samples, nil
}

// dockerContainer represents a Docker container from the list endpoint.
type dockerContainer struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	Image string   `json:"Image"`
}

// listContainers retrieves the list of running containers.
func (d *DockerCollector) listContainers(ctx context.Context) ([]dockerContainer, error) {
	// Check if socket exists
	if _, err := os.Stat(d.socketPath); err != nil {
		return nil, fmt.Errorf("socket not accessible: %w", err)
	}

	client := d.newHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/containers/json", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Docker returned status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var containers []dockerContainer
	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return containers, nil
}

// dockerStats represents the stats response from Docker.
type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"cpu_stats"`
	PreviousCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RXBytes uint64 `json:"rx_bytes"`
		TXBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

// ContainerStats represents parsed container statistics.
type ContainerStats struct {
	CPUPercent     float64
	MemoryUsage    uint64
	MemoryLimit    uint64
	NetworkRXBytes uint64
	NetworkTXBytes uint64
}

// getContainerStats retrieves stats for a specific container.
func (d *DockerCollector) getContainerStats(ctx context.Context, containerID string) (*ContainerStats, error) {
	client := d.newHTTPClient()

	url := fmt.Sprintf("http://unix/containers/%s/stats?stream=false", containerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Docker returned status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var stats dockerStats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("parsing stats: %w", err)
	}

	return &ContainerStats{
		CPUPercent:     d.calculateCPUPercent(stats),
		MemoryUsage:    stats.MemoryStats.Usage,
		MemoryLimit:    stats.MemoryStats.Limit,
		NetworkRXBytes: d.sumNetworkRX(stats),
		NetworkTXBytes: d.sumNetworkTX(stats),
	}, nil
}

// calculateCPUPercent calculates the CPU usage percentage.
func (d *DockerCollector) calculateCPUPercent(stats dockerStats) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreviousCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreviousCPUStats.SystemCPUUsage)

	if sysDelta == 0 {
		return 0
	}

	cpuPercent := (cpuDelta / sysDelta) * 100.0
	return cpuPercent
}

// sumNetworkRX sums received bytes across all network interfaces.
func (d *DockerCollector) sumNetworkRX(stats dockerStats) uint64 {
	var total uint64
	for _, net := range stats.Networks {
		total += net.RXBytes
	}
	return total
}

// sumNetworkTX sums transmitted bytes across all network interfaces.
func (d *DockerCollector) sumNetworkTX(stats dockerStats) uint64 {
	var total uint64
	for _, net := range stats.Networks {
		total += net.TXBytes
	}
	return total
}

// newHTTPClient creates an HTTP client with Unix socket transport.
func (d *DockerCollector) newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: d.timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", d.socketPath)
			},
		},
	}
}
