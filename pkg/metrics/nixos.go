// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// NixOSCollector gathers metrics from systemd and journalctl on NixOS systems.
// It implements the Collector interface and is defensive about missing systemctl.
type NixOSCollector struct {
	// systemctlPath is the path to systemctl (default: checks PATH)
	systemctlPath string
	// journalctlPath is the path to journalctl (default: checks PATH)
	journalctlPath string
	// timeout for command execution
	timeout time.Duration
}

// NewNixOSCollector creates a new NixOS collector.
func NewNixOSCollector() *NixOSCollector {
	return &NixOSCollector{
		systemctlPath:  "systemctl",
		journalctlPath: "journalctl",
		timeout:        10 * time.Second,
	}
}

// Name returns the collector's name.
func (n *NixOSCollector) Name() string {
	return "nixos"
}

// Describe returns descriptions of the metrics this collector produces.
func (n *NixOSCollector) Describe() []string {
	return []string{
		"unheaded_systemd_units_total - Total systemd units by active state",
		"unheaded_systemd_available - Whether systemd metrics are available (0 or 1)",
		"unheaded_journalctl_errors_total - Total error-level log entries in journal",
	}
}

// Collect gathers metrics from systemd and journalctl.
func (n *NixOSCollector) Collect(ctx context.Context) ([]Sample, error) {
	var samples []Sample
	baseLabels := map[string]string{"collector": "nixos"}

	// Collect systemd unit statistics
	unitSamples := n.collectSystemdUnits(ctx)
	samples = append(samples, unitSamples...)

	// Collect journalctl error count
	errorSamples := n.collectJournalErrors(ctx)
	samples = append(samples, errorSamples...)

	// Add availability indicator
	if len(unitSamples) > 0 {
		samples = append(samples, Sample{
			Name:   "unheaded_systemd_available",
			Help:   "Systemd metrics availability",
			Type:   TypeGauge,
			Labels: baseLabels,
			Value:  1,
		})
	} else {
		samples = append(samples, Sample{
			Name:   "unheaded_systemd_available",
			Help:   "Systemd metrics availability",
			Type:   TypeGauge,
			Labels: baseLabels,
			Value:  0,
		})
	}

	return samples, nil
}

// systemdUnit represents a systemd unit from list-units output.
type systemdUnit struct {
	Unit       string `json:"unit"`
	Load       string `json:"load"`
	ActiveState string `json:"active"`
	SubState   string `json:"sub"`
	Description string `json:"description"`
}

// collectSystemdUnits gathers systemd unit statistics.
func (n *NixOSCollector) collectSystemdUnits(ctx context.Context) []Sample {
	var samples []Sample

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, n.systemctlPath, "list-units", "--output=json", "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: systemctl failed: %v\n", err)
		return samples
	}

	units, err := n.parseSystemdUnits(string(output))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse systemd units: %v\n", err)
		return samples
	}

	// Count units by state
	stateCounts := make(map[string]int)
	for _, unit := range units {
		state := strings.ToLower(unit.ActiveState)
		stateCounts[state]++
	}

	// Create samples for each state
	for state, count := range stateCounts {
		labels := map[string]string{
			"collector": "nixos",
			"state":     state,
		}

		samples = append(samples, Sample{
			Name:   "unheaded_systemd_units_total",
			Help:   "Total systemd units by state",
			Type:   TypeGauge,
			Labels: labels,
			Value:  float64(count),
		})
	}

	return samples
}

// parseSystemdUnits parses the JSON output from systemctl list-units.
func (n *NixOSCollector) parseSystemdUnits(output string) ([]systemdUnit, error) {
	var units []systemdUnit

	output = strings.TrimSpace(output)
	if output == "" || output == "[]" {
		return units, nil
	}

	if err := json.Unmarshal([]byte(output), &units); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return units, nil
}

// journalEntry represents a journal entry.
type journalEntry struct {
	Message string `json:"MESSAGE"`
	Priority int `json:"PRIORITY,string"`
}

// collectJournalErrors counts error-level log entries.
func (n *NixOSCollector) collectJournalErrors(ctx context.Context) []Sample {
	var samples []Sample
	baseLabels := map[string]string{"collector": "nixos"}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, n.journalctlPath, "-p", "err", "-n", "0", "--output=json", "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		// journalctl might not be available, which is OK
		fmt.Fprintf(os.Stderr, "Warning: journalctl failed: %v\n", err)
		return samples
	}

	errorCount, err := n.parseJournalErrors(string(output))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse journal errors: %v\n", err)
		return samples
	}

	samples = append(samples, Sample{
		Name:   "unheaded_journalctl_errors_total",
		Help:   "Total error-level entries in systemd journal",
		Type:   TypeCounter,
		Labels: baseLabels,
		Value:  float64(errorCount),
	})

	return samples
}

// parseJournalErrors parses JSON output from journalctl and counts entries.
func (n *NixOSCollector) parseJournalErrors(output string) (int, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "[]" {
		return 0, nil
	}

	var entries []journalEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		return 0, fmt.Errorf("parsing JSON: %w", err)
	}

	return len(entries), nil
}
