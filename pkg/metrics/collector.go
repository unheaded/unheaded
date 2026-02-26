// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
)

// Sample represents a single metric sample from a collector.
type Sample struct {
	// Name is the metric name (e.g., "unheaded_memory_bytes").
	Name string
	// Help is the metric description.
	Help string
	// Type is the metric type (counter, gauge, histogram, summary).
	Type MetricType
	// Labels contains the metric's label dimensions.
	Labels map[string]string
	// Value is the metric value.
	Value float64
	// Timestamp is the Unix timestamp in milliseconds (0 means current time).
	Timestamp int64
}

// SystemCollector is the interface that all metric collectors must implement.
type SystemCollector interface {
	// Collect gathers metrics from the source and returns them as samples.
	// It should be defensive about missing data and not fail entirely if some
	// sources are unavailable. Context can be used to cancel long operations.
	Collect(ctx context.Context) ([]Sample, error)

	// Name returns the unique name of this collector (e.g., "baremetal", "lxd").
	Name() string

	// Describe returns human-readable descriptions of the metrics this collector produces.
	Describe() []string
}

// CollectorRegistry holds a collection of collectors and coordinates their execution.
type CollectorRegistry struct {
	mu         sync.RWMutex
	collectors map[string]SystemCollector
}

// NewCollectorRegistry creates a new collector registry.
func NewCollectorRegistry() *CollectorRegistry {
	return &CollectorRegistry{
		collectors: make(map[string]SystemCollector),
	}
}

// Register adds a collector to the registry.
// Returns an error if a collector with the same name is already registered.
func (r *CollectorRegistry) Register(c SystemCollector) error {
	if c == nil {
		return fmt.Errorf("cannot register nil collector")
	}

	name := c.Name()
	if name == "" {
		return fmt.Errorf("collector must have a non-empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.collectors[name]; exists {
		return fmt.Errorf("collector %q is already registered", name)
	}

	r.collectors[name] = c
	return nil
}

// CollectAll invokes all registered collectors in parallel and merges their results.
// It adds the "host" label to all samples using the system hostname.
// If a collector fails, it logs a warning and continues with other collectors.
// Returns a combined slice of all samples.
func (r *CollectorRegistry) CollectAll(ctx context.Context) ([]Sample, error) {
	r.mu.RLock()
	collectors := make(map[string]SystemCollector)
	for name, c := range r.collectors {
		collectors[name] = c
	}
	r.mu.RUnlock()

	if len(collectors) == 0 {
		return []Sample{}, nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Collect samples from all collectors in parallel
	type result struct {
		name    string
		samples []Sample
		err     error
	}

	resultChan := make(chan result, len(collectors))
	var wg sync.WaitGroup

	for name, collector := range collectors {
		wg.Add(1)
		go func(name string, c SystemCollector) {
			defer wg.Done()
			samples, err := c.Collect(ctx)
			resultChan <- result{
				name:    name,
				samples: samples,
				err:     err,
			}
		}(name, collector)
	}

	// Wait for all collectors to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Merge results
	var allSamples []Sample
	var errs []error

	for res := range resultChan {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("collector %q: %w", res.name, res.err))
			continue
		}

		for i := range res.samples {
			// Add host label to each sample
			if res.samples[i].Labels == nil {
				res.samples[i].Labels = make(map[string]string)
			}
			res.samples[i].Labels["host"] = hostname
		}

		allSamples = append(allSamples, res.samples...)
	}

	// If we got any errors, log them but don't fail the entire collection
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	// Sort samples by name for deterministic output
	sort.Slice(allSamples, func(i, j int) bool {
		if allSamples[i].Name != allSamples[j].Name {
			return allSamples[i].Name < allSamples[j].Name
		}
		// If names are equal, sort by labels
		iLabelStr := labelsToString(allSamples[i].Labels)
		jLabelStr := labelsToString(allSamples[j].Labels)
		return iLabelStr < jLabelStr
	})

	return allSamples, nil
}

// labelsToString creates a deterministic string representation of labels for sorting.
func labelsToString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result string
	for i, k := range keys {
		if i > 0 {
			result += ","
		}
		result += k + "=" + labels[k]
	}
	return result
}
