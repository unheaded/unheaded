// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// BareMetalCollector gathers system metrics from /proc and /sys filesystems.
// It implements the Collector interface and is defensive about missing files.
type BareMetalCollector struct {
	// procRoot allows overriding the /proc path for testing (default: "/proc")
	procRoot string
	// sysRoot allows overriding the /sys path for testing (default: "/sys")
	sysRoot string
}

// NewBareMetalCollector creates a new bare metal collector.
func NewBareMetalCollector() *BareMetalCollector {
	return &BareMetalCollector{
		procRoot: "/proc",
		sysRoot:  "/sys",
	}
}

// Name returns the collector's name.
func (b *BareMetalCollector) Name() string {
	return "baremetal"
}

// Describe returns descriptions of the metrics this collector produces.
func (b *BareMetalCollector) Describe() []string {
	return []string{
		"unheaded_memory_total - Total memory in bytes",
		"unheaded_memory_free - Free memory in bytes",
		"unheaded_memory_available - Available memory in bytes",
		"unheaded_memory_buffers - Memory used by buffers in bytes",
		"unheaded_memory_cached - Memory used by page cache in bytes",
		"unheaded_load_average_1m - 1-minute load average",
		"unheaded_load_average_5m - 5-minute load average",
		"unheaded_load_average_15m - 15-minute load average",
		"unheaded_cpu_seconds_total - CPU time spent in various modes",
		"unheaded_network_rx_bytes - Network received bytes per interface",
		"unheaded_network_rx_packets - Network received packets per interface",
		"unheaded_network_tx_bytes - Network transmitted bytes per interface",
		"unheaded_network_tx_packets - Network transmitted packets per interface",
	}
}

// Collect gathers metrics from the system.
func (b *BareMetalCollector) Collect(ctx context.Context) ([]Sample, error) {
	var samples []Sample

	// Collect memory metrics
	memSamples := b.collectMemory()
	samples = append(samples, memSamples...)

	// Collect load average
	loadSamples := b.collectLoadAverage()
	samples = append(samples, loadSamples...)

	// Collect CPU metrics
	cpuSamples := b.collectCPU()
	samples = append(samples, cpuSamples...)

	// Collect network metrics
	netSamples := b.collectNetwork()
	samples = append(samples, netSamples...)

	return samples, nil
}

// collectMemory reads memory metrics from /proc/meminfo.
func (b *BareMetalCollector) collectMemory() []Sample {
	var samples []Sample
	baseLabels := map[string]string{"collector": "baremetal"}

	path := filepath.Join(b.procRoot, "meminfo")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", path, err)
		return samples
	}

	memMetrics := b.parseProcMeminfo(string(data))
	for name, value := range memMetrics {
		samples = append(samples, Sample{
			Name:   "unheaded_memory_" + name,
			Help:   "Memory metric " + name,
			Type:   TypeGauge,
			Labels: baseLabels,
			Value:  value,
		})
	}

	return samples
}

// parseProcMeminfo parses the content of /proc/meminfo.
func (b *BareMetalCollector) parseProcMeminfo(content string) map[string]float64 {
	result := make(map[string]float64)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.TrimSuffix(parts[0], ":")
		key = strings.ToLower(key)

		// Extract specific metrics (convert from KB to bytes)
		switch key {
		case "memtotal":
			if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
				result["total"] = v * 1024
			}
		case "memfree":
			if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
				result["free"] = v * 1024
			}
		case "memavailable":
			if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
				result["available"] = v * 1024
			}
		case "buffers":
			if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
				result["buffers"] = v * 1024
			}
		case "cached":
			if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
				result["cached"] = v * 1024
			}
		}
	}

	return result
}

// collectLoadAverage reads load average from /proc/loadavg.
func (b *BareMetalCollector) collectLoadAverage() []Sample {
	var samples []Sample
	baseLabels := map[string]string{"collector": "baremetal"}

	path := filepath.Join(b.procRoot, "loadavg")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", path, err)
		return samples
	}

	loads := b.parseProcLoadavg(string(data))
	if len(loads) >= 3 {
		samples = append(samples,
			Sample{
				Name:   "unheaded_load_average_1m",
				Help:   "1-minute load average",
				Type:   TypeGauge,
				Labels: baseLabels,
				Value:  loads[0],
			},
			Sample{
				Name:   "unheaded_load_average_5m",
				Help:   "5-minute load average",
				Type:   TypeGauge,
				Labels: baseLabels,
				Value:  loads[1],
			},
			Sample{
				Name:   "unheaded_load_average_15m",
				Help:   "15-minute load average",
				Type:   TypeGauge,
				Labels: baseLabels,
				Value:  loads[2],
			},
		)
	}

	return samples
}

// parseProcLoadavg parses the content of /proc/loadavg.
func (b *BareMetalCollector) parseProcLoadavg(content string) []float64 {
	var result []float64

	parts := strings.Fields(strings.TrimSpace(content))
	if len(parts) >= 3 {
		for i := 0; i < 3; i++ {
			if v, err := strconv.ParseFloat(parts[i], 64); err == nil {
				result = append(result, v)
			}
		}
	}

	return result
}

// collectCPU reads CPU metrics from /proc/stat.
func (b *BareMetalCollector) collectCPU() []Sample {
	var samples []Sample
	baseLabels := map[string]string{"collector": "baremetal"}

	path := filepath.Join(b.procRoot, "stat")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", path, err)
		return samples
	}

	cpuTimes := b.parseProcStat(string(data))
	modeNames := []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq"}

	for i, modeName := range modeNames {
		if i < len(cpuTimes) {
			labels := baseLabels
			if labels == nil {
				labels = make(map[string]string)
			}
			labels = map[string]string{
				"collector": "baremetal",
				"mode":      modeName,
			}

			samples = append(samples, Sample{
				Name:   "unheaded_cpu_seconds_total",
				Help:   "CPU time by mode",
				Type:   TypeCounter,
				Labels: labels,
				Value:  cpuTimes[i],
			})
		}
	}

	return samples
}

// parseProcStat parses the first "cpu" line from /proc/stat.
func (b *BareMetalCollector) parseProcStat(content string) []float64 {
	var result []float64

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		// Parse "cpu user nice system idle iowait irq softirq ..."
		parts := strings.Fields(line)
		if len(parts) < 8 {
			break
		}

		// Skip the "cpu" label and convert tick values to seconds
		// Assuming CONFIG_HZ = 100 (standard on most systems)
		const ticksPerSecond = 100.0

		for i := 1; i < 8 && i < len(parts); i++ {
			if v, err := strconv.ParseFloat(parts[i], 64); err == nil {
				result = append(result, v/ticksPerSecond)
			} else {
				result = append(result, 0)
			}
		}

		break
	}

	return result
}

// collectNetwork reads network metrics from /sys/class/net.
func (b *BareMetalCollector) collectNetwork() []Sample {
	var samples []Sample
	baseLabels := map[string]string{"collector": "baremetal"}

	netPath := filepath.Join(b.sysRoot, "class", "net")
	entries, err := ioutil.ReadDir(netPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", netPath, err)
		return samples
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		ifName := entry.Name()
		stats := b.parseNetStats(netPath, ifName)

		labels := map[string]string{
			"collector":  "baremetal",
			"interface":  ifName,
		}

		if v, ok := stats["rx_bytes"]; ok {
			samples = append(samples, Sample{
				Name:   "unheaded_network_rx_bytes",
				Help:   "Bytes received",
				Type:   TypeCounter,
				Labels: labels,
				Value:  v,
			})
		}

		if v, ok := stats["rx_packets"]; ok {
			samples = append(samples, Sample{
				Name:   "unheaded_network_rx_packets",
				Help:   "Packets received",
				Type:   TypeCounter,
				Labels: labels,
				Value:  v,
			})
		}

		if v, ok := stats["tx_bytes"]; ok {
			samples = append(samples, Sample{
				Name:   "unheaded_network_tx_bytes",
				Help:   "Bytes transmitted",
				Type:   TypeCounter,
				Labels: labels,
				Value:  v,
			})
		}

		if v, ok := stats["tx_packets"]; ok {
			samples = append(samples, Sample{
				Name:   "unheaded_network_tx_packets",
				Help:   "Packets transmitted",
				Type:   TypeCounter,
				Labels: labels,
				Value:  v,
			})
		}
	}

	return samples
}

// parseNetStats reads network statistics for a specific interface.
func (b *BareMetalCollector) parseNetStats(netPath string, ifName string) map[string]float64 {
	result := make(map[string]float64)

	statsPath := filepath.Join(netPath, ifName, "statistics")

	metrics := []string{"rx_bytes", "rx_packets", "tx_bytes", "tx_packets"}
	for _, metric := range metrics {
		filePath := filepath.Join(statsPath, metric)
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			continue
		}

		if v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil {
			result[metric] = v
		}
	}

	return result
}
