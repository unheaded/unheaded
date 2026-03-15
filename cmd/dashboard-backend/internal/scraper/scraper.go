// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package scraper provides metrics scraping from Kingdom services.
// It polls /metrics endpoints from all registered services and aggregates
// the results for the dashboard.
package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
)

var (
	// ErrNilConfig indicates nil configuration
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrInvalidInterval indicates invalid scrape interval
	ErrInvalidInterval = errors.New("scrape interval must be > 0")
	// ErrServiceNotFound indicates service not found
	ErrServiceNotFound = errors.New("service not found")
	// ErrScraperStopped indicates scraper has been stopped
	ErrScraperStopped = errors.New("scraper has been stopped")
)

// Config holds scraper configuration
type Config struct {
	// ScrapeInterval is how often to scrape metrics
	ScrapeInterval time.Duration
	// ScrapeTimeout is timeout for each scrape request
	ScrapeTimeout time.Duration
	// RetentionPeriod is how long to keep metrics
	RetentionPeriod time.Duration
	// MaxSamples is maximum samples per metric series
	MaxSamples int
}

// DefaultConfig returns default scraper configuration
func DefaultConfig() *Config {
	return &Config{
		ScrapeInterval:  15 * time.Second,
		ScrapeTimeout:   10 * time.Second,
		RetentionPeriod: 1 * time.Hour,
		MaxSamples:      1000,
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.ScrapeInterval <= 0 {
		return ErrInvalidInterval
	}
	if c.ScrapeTimeout == 0 {
		c.ScrapeTimeout = 10 * time.Second
	}
	if c.RetentionPeriod == 0 {
		c.RetentionPeriod = 1 * time.Hour
	}
	if c.MaxSamples == 0 {
		c.MaxSamples = 1000
	}
	return nil
}

// ServiceTarget represents a service to scrape
type ServiceTarget struct {
	// Name is the service name (e.g., "timeguru", "captain")
	Name string
	// MetricsURL is the full URL to the /metrics endpoint
	MetricsURL string
	// HealthURL is the full URL to the /health endpoint
	HealthURL string
	// Labels are additional labels to add to scraped metrics
	Labels map[string]string
	// Enabled indicates if scraping is enabled
	Enabled bool
}

// MetricSample represents a single metric sample
type MetricSample struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
	Timestamp time.Time         `json:"timestamp"`
	Service   string            `json:"service"`
}

// MetricSeries holds samples for a metric over time
type MetricSeries struct {
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels"`
	Service string            `json:"service"`
	Samples []MetricSample    `json:"samples"`
	mu      sync.RWMutex
}

// AddSample adds a sample to the series
func (ms *MetricSeries) AddSample(sample MetricSample, maxSamples int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.Samples = append(ms.Samples, sample)
	if len(ms.Samples) > maxSamples {
		ms.Samples = ms.Samples[len(ms.Samples)-maxSamples:]
	}
}

// GetLatest returns the latest sample
func (ms *MetricSeries) GetLatest() *MetricSample {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if len(ms.Samples) == 0 {
		return nil
	}
	return &ms.Samples[len(ms.Samples)-1]
}

// GetSamplesSince returns samples since a given time
func (ms *MetricSeries) GetSamplesSince(since time.Time) []MetricSample {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	var result []MetricSample
	for _, s := range ms.Samples {
		if s.Timestamp.After(since) {
			result = append(result, s)
		}
	}
	return result
}

// Cleanup removes samples older than cutoff
func (ms *MetricSeries) Cleanup(cutoff time.Time) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var valid []MetricSample
	for _, s := range ms.Samples {
		if s.Timestamp.After(cutoff) {
			valid = append(valid, s)
		}
	}
	ms.Samples = valid
}

// ScrapeResult contains results from a single scrape
type ScrapeResult struct {
	Service    string         `json:"service"`
	Metrics    []MetricSample `json:"metrics"`
	Error      string         `json:"error,omitempty"`
	Duration   time.Duration  `json:"duration"`
	Timestamp  time.Time      `json:"timestamp"`
	StatusCode int            `json:"status_code"`
}

// AggregatedMetrics represents the aggregated view of all metrics
type AggregatedMetrics struct {
	Timestamp    time.Time                  `json:"timestamp"`
	Services     map[string]*ServiceMetrics `json:"services"`
	TotalMetrics int                        `json:"total_metrics"`
	TotalSeries  int                        `json:"total_series"`
}

// ServiceMetrics holds metrics for a single service
type ServiceMetrics struct {
	Name        string                  `json:"name"`
	Status      string                  `json:"status"` // up, down, unknown
	LastScrape  time.Time               `json:"last_scrape"`
	ScrapeCount int64                   `json:"scrape_count"`
	ErrorCount  int64                   `json:"error_count"`
	Metrics     map[string]*MetricValue `json:"metrics"`
}

// MetricValue represents a metric with its current value and metadata
type MetricValue struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Type      string            `json:"type,omitempty"` // counter, gauge, histogram, summary
}

// Scraper scrapes metrics from Kingdom services
type Scraper struct {
	config  *Config
	log     *logger.Logger
	client  *http.Client
	targets map[string]*ServiceTarget
	series  map[string]*MetricSeries
	results map[string]*ScrapeResult

	targetsMu sync.RWMutex
	seriesMu  sync.RWMutex
	resultsMu sync.RWMutex

	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	runMu    sync.RWMutex
	wg       sync.WaitGroup

	// Callbacks
	onScrapeComplete func(result *ScrapeResult)
}

// NewScraper creates a new metrics scraper
func NewScraper(config *Config, log *logger.Logger) (*Scraper, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if log == nil {
		log = logger.New(nil)
	}

	return &Scraper{
		config: config,
		log:    log,
		client: &http.Client{
			Timeout: config.ScrapeTimeout,
		},
		targets: make(map[string]*ServiceTarget),
		series:  make(map[string]*MetricSeries),
		results: make(map[string]*ScrapeResult),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}, nil
}

// RegisterTarget registers a service target for scraping
func (s *Scraper) RegisterTarget(target *ServiceTarget) error {
	if target == nil {
		return errors.New("target cannot be nil")
	}
	if target.Name == "" {
		return errors.New("target name cannot be empty")
	}
	if target.MetricsURL == "" {
		return errors.New("target metrics URL cannot be empty")
	}

	s.targetsMu.Lock()
	defer s.targetsMu.Unlock()

	if target.Labels == nil {
		target.Labels = make(map[string]string)
	}
	target.Labels["service"] = target.Name
	target.Enabled = true

	s.targets[target.Name] = target

	s.log.Info().
		Str("service", target.Name).
		Str("metrics_url", target.MetricsURL).
		Msg("registered scrape target")

	return nil
}

// UnregisterTarget removes a service target
func (s *Scraper) UnregisterTarget(name string) error {
	s.targetsMu.Lock()
	defer s.targetsMu.Unlock()

	if _, exists := s.targets[name]; !exists {
		return ErrServiceNotFound
	}

	delete(s.targets, name)

	s.log.Info().
		Str("service", name).
		Msg("unregistered scrape target")

	return nil
}

// GetTargets returns all registered targets
func (s *Scraper) GetTargets() []*ServiceTarget {
	s.targetsMu.RLock()
	defer s.targetsMu.RUnlock()

	targets := make([]*ServiceTarget, 0, len(s.targets))
	for _, t := range s.targets {
		targets = append(targets, t)
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	return targets
}

// OnScrapeComplete sets a callback for scrape completion
func (s *Scraper) OnScrapeComplete(fn func(*ScrapeResult)) {
	s.onScrapeComplete = fn
}

// Start starts the scraper
func (s *Scraper) Start(ctx context.Context) error {
	s.runMu.Lock()
	if s.running {
		s.runMu.Unlock()
		return errors.New("scraper already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.runMu.Unlock()

	s.wg.Add(2)
	go s.scrapeLoop(ctx)
	go s.cleanupLoop(ctx)

	s.log.Info().
		Dur("interval", s.config.ScrapeInterval).
		Msg("scraper started")

	return nil
}

// Stop stops the scraper
func (s *Scraper) Stop() error {
	s.runMu.Lock()
	if !s.running {
		s.runMu.Unlock()
		return nil
	}
	s.running = false
	close(s.stopCh)
	s.runMu.Unlock()

	s.wg.Wait()
	close(s.doneCh)

	s.log.Info().Msg("scraper stopped")
	return nil
}

// scrapeLoop runs the periodic scrape
func (s *Scraper) scrapeLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.ScrapeInterval)
	defer ticker.Stop()

	// Initial scrape
	s.scrapeAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.scrapeAll(ctx)
		}
	}
}

// cleanupLoop runs periodic cleanup of old samples
func (s *Scraper) cleanupLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.RetentionPeriod / 10)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// scrapeAll scrapes all registered targets
func (s *Scraper) scrapeAll(ctx context.Context) {
	s.targetsMu.RLock()
	targets := make([]*ServiceTarget, 0, len(s.targets))
	for _, t := range s.targets {
		if t.Enabled {
			targets = append(targets, t)
		}
	}
	s.targetsMu.RUnlock()

	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t *ServiceTarget) {
			defer wg.Done()
			s.scrapeTarget(ctx, t)
		}(target)
	}
	wg.Wait()
}

// scrapeTarget scrapes a single target
func (s *Scraper) scrapeTarget(ctx context.Context, target *ServiceTarget) {
	start := time.Now()
	result := &ScrapeResult{
		Service:   target.Name,
		Timestamp: start,
		Metrics:   make([]MetricSample, 0),
	}

	req, err := http.NewRequestWithContext(ctx, "GET", target.MetricsURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("create request: %v", err)
		result.Duration = time.Since(start)
		s.storeResult(result)
		return
	}

	resp, err := s.client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Duration = time.Since(start)
		s.storeResult(result)
		return
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		result.Duration = time.Since(start)
		s.storeResult(result)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read body: %v", err)
		result.Duration = time.Since(start)
		s.storeResult(result)
		return
	}

	// Parse Prometheus format metrics
	samples := s.parsePrometheusMetrics(body, target, start)
	result.Metrics = samples
	result.Duration = time.Since(start)

	// Store samples in series
	for _, sample := range samples {
		s.storeSample(sample)
	}

	s.storeResult(result)

	s.log.Debug().
		Str("service", target.Name).
		Int("metrics", len(samples)).
		Dur("duration", result.Duration).
		Msg("scraped target")
}

// parsePrometheusMetrics parses Prometheus exposition format
func (s *Scraper) parsePrometheusMetrics(data []byte, target *ServiceTarget, ts time.Time) []MetricSample {
	var samples []MetricSample
	lines := bytes.Split(data, []byte("\n"))

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		sample := s.parseLine(string(line), target, ts)
		if sample != nil {
			samples = append(samples, *sample)
		}
	}

	return samples
}

// parseLine parses a single Prometheus metric line
func (s *Scraper) parseLine(line string, target *ServiceTarget, ts time.Time) *MetricSample {
	// Format: metric_name{label1="value1",label2="value2"} value [timestamp]
	// Or: metric_name value [timestamp]

	// Find the end of metric name/labels
	var name string
	var labels map[string]string
	var valueStr string

	if idx := strings.Index(line, "{"); idx != -1 {
		name = line[:idx]
		endIdx := strings.Index(line, "}")
		if endIdx == -1 {
			return nil
		}
		labelsStr := line[idx+1 : endIdx]
		labels = parseLabels(labelsStr)
		valueStr = strings.TrimSpace(line[endIdx+1:])
	} else {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil
		}
		name = parts[0]
		valueStr = parts[1]
		labels = make(map[string]string)
	}

	// Parse value (may have timestamp after it)
	valueParts := strings.Fields(valueStr)
	if len(valueParts) == 0 {
		return nil
	}

	value, err := strconv.ParseFloat(valueParts[0], 64)
	if err != nil {
		return nil
	}

	// Add service labels
	for k, v := range target.Labels {
		labels[k] = v
	}

	return &MetricSample{
		Name:      name,
		Value:     value,
		Labels:    labels,
		Timestamp: ts,
		Service:   target.Name,
	}
}

// parseLabels parses label string like: label1="value1",label2="value2"
func parseLabels(s string) map[string]string {
	labels := make(map[string]string)
	pairs := strings.Split(s, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		idx := strings.Index(pair, "=")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(pair[:idx])
		value := strings.Trim(strings.TrimSpace(pair[idx+1:]), "\"")
		labels[key] = value
	}

	return labels
}

// seriesKey generates a unique key for a metric series
func seriesKey(name string, labels map[string]string) string {
	key := name
	if len(labels) > 0 {
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			key += fmt.Sprintf(",%s=%s", k, labels[k])
		}
	}
	return key
}

// storeSample stores a metric sample in the appropriate series
func (s *Scraper) storeSample(sample MetricSample) {
	key := seriesKey(sample.Name, sample.Labels)

	s.seriesMu.Lock()
	series, exists := s.series[key]
	if !exists {
		series = &MetricSeries{
			Name:    sample.Name,
			Labels:  sample.Labels,
			Service: sample.Service,
			Samples: make([]MetricSample, 0, s.config.MaxSamples),
		}
		s.series[key] = series
	}
	s.seriesMu.Unlock()

	series.AddSample(sample, s.config.MaxSamples)
}

// IngestSample stores a metric sample directly into the series store,
// bypassing the scrape loop. Used by the server to inject host metrics
// so they are queryable via /api/v1/metrics/query.
func (s *Scraper) IngestSample(sample MetricSample) {
	s.storeSample(sample)
}

// storeResult stores a scrape result and triggers callback
func (s *Scraper) storeResult(result *ScrapeResult) {
	s.resultsMu.Lock()
	s.results[result.Service] = result
	s.resultsMu.Unlock()

	if s.onScrapeComplete != nil {
		s.onScrapeComplete(result)
	}
}

// cleanup removes old samples
func (s *Scraper) cleanup() {
	cutoff := time.Now().Add(-s.config.RetentionPeriod)

	s.seriesMu.RLock()
	seriesList := make([]*MetricSeries, 0, len(s.series))
	for _, series := range s.series {
		seriesList = append(seriesList, series)
	}
	s.seriesMu.RUnlock()

	for _, series := range seriesList {
		series.Cleanup(cutoff)
	}

	s.log.Debug().
		Time("cutoff", cutoff).
		Msg("metrics cleanup complete")
}

// GetAggregatedMetrics returns aggregated metrics from all services
func (s *Scraper) GetAggregatedMetrics() *AggregatedMetrics {
	s.resultsMu.RLock()
	defer s.resultsMu.RUnlock()

	agg := &AggregatedMetrics{
		Timestamp: time.Now(),
		Services:  make(map[string]*ServiceMetrics),
	}

	for name, result := range s.results {
		sm := &ServiceMetrics{
			Name:       name,
			LastScrape: result.Timestamp,
			Metrics:    make(map[string]*MetricValue),
		}

		if result.Error != "" {
			sm.Status = "down"
			sm.ErrorCount++
		} else {
			sm.Status = "up"
			sm.ScrapeCount++
		}

		for _, sample := range result.Metrics {
			sm.Metrics[sample.Name] = &MetricValue{
				Name:      sample.Name,
				Value:     sample.Value,
				Labels:    sample.Labels,
				Timestamp: sample.Timestamp,
			}
			agg.TotalMetrics++
		}

		agg.Services[name] = sm
	}

	s.seriesMu.RLock()
	agg.TotalSeries = len(s.series)
	s.seriesMu.RUnlock()

	return agg
}

// GetServiceMetrics returns metrics for a specific service
func (s *Scraper) GetServiceMetrics(serviceName string) (*ServiceMetrics, error) {
	s.resultsMu.RLock()
	result, exists := s.results[serviceName]
	s.resultsMu.RUnlock()

	if !exists {
		return nil, ErrServiceNotFound
	}

	sm := &ServiceMetrics{
		Name:       serviceName,
		LastScrape: result.Timestamp,
		Metrics:    make(map[string]*MetricValue),
	}

	if result.Error != "" {
		sm.Status = "down"
	} else {
		sm.Status = "up"
	}

	for _, sample := range result.Metrics {
		sm.Metrics[sample.Name] = &MetricValue{
			Name:      sample.Name,
			Value:     sample.Value,
			Labels:    sample.Labels,
			Timestamp: sample.Timestamp,
		}
	}

	return sm, nil
}

// GetMetricSeries returns a specific metric series
func (s *Scraper) GetMetricSeries(name string, labels map[string]string) (*MetricSeries, error) {
	key := seriesKey(name, labels)

	s.seriesMu.RLock()
	series, exists := s.series[key]
	s.seriesMu.RUnlock()

	if !exists {
		return nil, errors.New("series not found")
	}

	return series, nil
}

// QueryMetrics queries metrics matching the given filters
func (s *Scraper) QueryMetrics(nameFilter string, labelFilters map[string]string, since time.Time) []MetricSample {
	s.seriesMu.RLock()
	defer s.seriesMu.RUnlock()

	var results []MetricSample

	for _, series := range s.series {
		// Filter by name
		if nameFilter != "" && !strings.Contains(series.Name, nameFilter) {
			continue
		}

		// Filter by labels
		match := true
		for k, v := range labelFilters {
			if series.Labels[k] != v {
				match = false
				break
			}
		}
		if !match {
			continue
		}

		// Get samples since time
		samples := series.GetSamplesSince(since)
		results = append(results, samples...)
	}

	// Sort by timestamp
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results
}

// GetLatestResults returns the latest scrape results for all services
func (s *Scraper) GetLatestResults() map[string]*ScrapeResult {
	s.resultsMu.RLock()
	defer s.resultsMu.RUnlock()

	results := make(map[string]*ScrapeResult, len(s.results))
	for k, v := range s.results {
		results[k] = v
	}
	return results
}

// SeriesCount returns the number of active metric series
func (s *Scraper) SeriesCount() int {
	s.seriesMu.RLock()
	defer s.seriesMu.RUnlock()
	return len(s.series)
}

// IsRunning returns whether the scraper is running
func (s *Scraper) IsRunning() bool {
	s.runMu.RLock()
	defer s.runMu.RUnlock()
	return s.running
}

// DefaultServiceEndpoints returns the default LXD bridge addresses for Kingdom services.
var DefaultServiceEndpoints = map[string]string{
	"timeguru":     "localhost:19000",
	"captain":      "localhost:19002",
	"architect":    "localhost:19001",
	"micromanager": "localhost:19003",
	"wotan":        "localhost:18001",
	"gateway":      "localhost:21000",
}

// RegisterKingdomServices registers the standard Kingdom services as scrape targets.
// If overrides is non-nil, entries override the default addresses (format: "host:port").
func (s *Scraper) RegisterKingdomServices(overrides map[string]string) {
	endpoints := make(map[string]string, len(DefaultServiceEndpoints))
	for k, v := range DefaultServiceEndpoints {
		endpoints[k] = v
	}
	for k, v := range overrides {
		endpoints[k] = v
	}

	for name, hostPort := range endpoints {
		target := &ServiceTarget{
			Name:       name,
			MetricsURL: fmt.Sprintf("http://%s/metrics", hostPort),
			HealthURL:  fmt.Sprintf("http://%s/health", hostPort),
			Labels: map[string]string{
				"job":      "kingdom",
				"instance": hostPort,
			},
		}
		s.RegisterTarget(target)
	}
}
