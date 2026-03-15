// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package metrics provides the Vambraces - the Kingdom's observability layer.
// This is a custom metrics library that outputs Prometheus-compatible exposition format
// without depending on prometheus/client_golang.
//
// The Vambraces are forearm armor that grant visibility into the Kingdom's operations,
// allowing operators to see the health and performance of all services.
package metrics

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType represents the type of a metric.
type MetricType string

const (
	// TypeCounter is a monotonically increasing value.
	TypeCounter MetricType = "counter"
	// TypeGauge is a value that can go up and down.
	TypeGauge MetricType = "gauge"
	// TypeHistogram tracks value distributions with configurable buckets.
	TypeHistogram MetricType = "histogram"
	// TypeSummary calculates quantiles over a sliding time window.
	TypeSummary MetricType = "summary"
)

// Labels represents key-value pairs for metric dimensions.
// Labels are immutable after creation for thread safety.
type Labels map[string]string

// String returns a deterministic string representation of labels
// suitable for use as a map key.
func (l Labels) String() string {
	if len(l) == 0 {
		return ""
	}

	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(escapeLabel(l[k]))
	}
	return buf.String()
}

// Merge creates a new Labels combining the receiver with other labels.
// Other labels override receiver labels with the same key.
func (l Labels) Merge(other Labels) Labels {
	result := make(Labels, len(l)+len(other))
	for k, v := range l {
		result[k] = v
	}
	for k, v := range other {
		result[k] = v
	}
	return result
}

// Clone creates a deep copy of the labels.
func (l Labels) Clone() Labels {
	result := make(Labels, len(l))
	for k, v := range l {
		result[k] = v
	}
	return result
}

// escapeLabel escapes a label value for Prometheus exposition format.
func escapeLabel(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// formatLabels formats labels for Prometheus exposition format.
func formatLabels(labels Labels) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(k)
		buf.WriteString("=\"")
		buf.WriteString(escapeLabel(labels[k]))
		buf.WriteByte('"')
	}
	buf.WriteByte('}')
	return buf.String()
}

// Metric is the interface all metric types must implement.
type Metric interface {
	// Describe returns the metric descriptor.
	Describe() *Desc
	// Write writes the metric to the given writer in Prometheus format.
	Write(w io.Writer) error
}

// Desc is a metric descriptor containing metadata.
type Desc struct {
	// Name is the metric name (must match [a-zA-Z_:][a-zA-Z0-9_:]*).
	Name string
	// Help is the metric description.
	Help string
	// Type is the metric type.
	Type MetricType
	// ConstLabels are labels that are constant for all instances.
	ConstLabels Labels
	// VariableLabels are the names of labels that can vary.
	VariableLabels []string
}

// NewDesc creates a new metric descriptor.
func NewDesc(name, help string, metricType MetricType, constLabels Labels, variableLabels []string) *Desc {
	return &Desc{
		Name:           name,
		Help:           help,
		Type:           metricType,
		ConstLabels:    constLabels,
		VariableLabels: variableLabels,
	}
}

// Counter is a monotonically increasing metric.
// It can only increase or be reset to zero.
type Counter struct {
	desc   *Desc
	labels Labels
	value  uint64 // stored as bits of float64 for atomic operations
}

// NewCounter creates a new counter with the given name and help text.
func NewCounter(name, help string, constLabels Labels) *Counter {
	return &Counter{
		desc: NewDesc(name, help, TypeCounter, constLabels, nil),
		labels: constLabels,
	}
}

// Describe returns the counter's descriptor.
func (c *Counter) Describe() *Desc {
	return c.desc
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	c.Add(1)
}

// Add adds the given value to the counter.
// The value must be non-negative.
func (c *Counter) Add(delta float64) {
	if delta < 0 {
		panic("counter cannot decrease")
	}
	for {
		old := atomic.LoadUint64(&c.value)
		newVal := math.Float64frombits(old) + delta
		if atomic.CompareAndSwapUint64(&c.value, old, math.Float64bits(newVal)) {
			break
		}
	}
}

// Value returns the current counter value.
func (c *Counter) Value() float64 {
	return math.Float64frombits(atomic.LoadUint64(&c.value))
}

// Write writes the counter to the given writer in Prometheus format.
func (c *Counter) Write(w io.Writer) error {
	labelStr := formatLabels(c.labels)
	_, err := fmt.Fprintf(w, "%s%s %s\n", c.desc.Name, labelStr, formatFloat(c.Value()))
	return err
}

// CounterVec is a collection of counters with the same name but different labels.
type CounterVec struct {
	desc     *Desc
	mu       sync.RWMutex
	counters map[string]*Counter
}

// NewCounterVec creates a new counter vector.
func NewCounterVec(name, help string, constLabels Labels, labelNames []string) *CounterVec {
	return &CounterVec{
		desc:     NewDesc(name, help, TypeCounter, constLabels, labelNames),
		counters: make(map[string]*Counter),
	}
}

// Describe returns the counter vector's descriptor.
func (cv *CounterVec) Describe() *Desc {
	return cv.desc
}

// WithLabels returns the counter for the given labels.
func (cv *CounterVec) WithLabels(labels Labels) *Counter {
	key := labels.String()

	cv.mu.RLock()
	counter, ok := cv.counters[key]
	cv.mu.RUnlock()

	if ok {
		return counter
	}

	cv.mu.Lock()
	defer cv.mu.Unlock()

	// Double-check after acquiring write lock
	if counter, ok = cv.counters[key]; ok {
		return counter
	}

	allLabels := cv.desc.ConstLabels.Merge(labels)
	counter = &Counter{
		desc:   cv.desc,
		labels: allLabels,
	}
	cv.counters[key] = counter
	return counter
}

// Write writes all counters to the given writer in Prometheus format.
func (cv *CounterVec) Write(w io.Writer) error {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	for _, counter := range cv.counters {
		if err := counter.Write(w); err != nil {
			return err
		}
	}
	return nil
}

// Gauge is a metric that can go up and down.
type Gauge struct {
	desc   *Desc
	labels Labels
	value  uint64 // stored as bits of float64 for atomic operations
}

// NewGauge creates a new gauge with the given name and help text.
func NewGauge(name, help string, constLabels Labels) *Gauge {
	return &Gauge{
		desc:   NewDesc(name, help, TypeGauge, constLabels, nil),
		labels: constLabels,
	}
}

// Describe returns the gauge's descriptor.
func (g *Gauge) Describe() *Desc {
	return g.desc
}

// Set sets the gauge to the given value.
func (g *Gauge) Set(val float64) {
	atomic.StoreUint64(&g.value, math.Float64bits(val))
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() {
	g.Add(1)
}

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() {
	g.Add(-1)
}

// Add adds the given value to the gauge.
func (g *Gauge) Add(delta float64) {
	for {
		old := atomic.LoadUint64(&g.value)
		newVal := math.Float64frombits(old) + delta
		if atomic.CompareAndSwapUint64(&g.value, old, math.Float64bits(newVal)) {
			break
		}
	}
}

// Sub subtracts the given value from the gauge.
func (g *Gauge) Sub(delta float64) {
	g.Add(-delta)
}

// Value returns the current gauge value.
func (g *Gauge) Value() float64 {
	return math.Float64frombits(atomic.LoadUint64(&g.value))
}

// SetToCurrentTime sets the gauge to the current Unix timestamp.
func (g *Gauge) SetToCurrentTime() {
	g.Set(float64(time.Now().Unix()))
}

// Write writes the gauge to the given writer in Prometheus format.
func (g *Gauge) Write(w io.Writer) error {
	labelStr := formatLabels(g.labels)
	_, err := fmt.Fprintf(w, "%s%s %s\n", g.desc.Name, labelStr, formatFloat(g.Value()))
	return err
}

// GaugeVec is a collection of gauges with the same name but different labels.
type GaugeVec struct {
	desc   *Desc
	mu     sync.RWMutex
	gauges map[string]*Gauge
}

// NewGaugeVec creates a new gauge vector.
func NewGaugeVec(name, help string, constLabels Labels, labelNames []string) *GaugeVec {
	return &GaugeVec{
		desc:   NewDesc(name, help, TypeGauge, constLabels, labelNames),
		gauges: make(map[string]*Gauge),
	}
}

// Describe returns the gauge vector's descriptor.
func (gv *GaugeVec) Describe() *Desc {
	return gv.desc
}

// WithLabels returns the gauge for the given labels.
func (gv *GaugeVec) WithLabels(labels Labels) *Gauge {
	key := labels.String()

	gv.mu.RLock()
	gauge, ok := gv.gauges[key]
	gv.mu.RUnlock()

	if ok {
		return gauge
	}

	gv.mu.Lock()
	defer gv.mu.Unlock()

	if gauge, ok = gv.gauges[key]; ok {
		return gauge
	}

	allLabels := gv.desc.ConstLabels.Merge(labels)
	gauge = &Gauge{
		desc:   gv.desc,
		labels: allLabels,
	}
	gv.gauges[key] = gauge
	return gauge
}

// Write writes all gauges to the given writer in Prometheus format.
func (gv *GaugeVec) Write(w io.Writer) error {
	gv.mu.RLock()
	defer gv.mu.RUnlock()

	for _, gauge := range gv.gauges {
		if err := gauge.Write(w); err != nil {
			return err
		}
	}
	return nil
}

// HistogramOpts configures a histogram.
type HistogramOpts struct {
	Name        string
	Help        string
	ConstLabels Labels
	Buckets     []float64
}

// DefaultBuckets are the default histogram buckets.
var DefaultBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// LinearBuckets creates count buckets starting at start with the given width.
func LinearBuckets(start, width float64, count int) []float64 {
	buckets := make([]float64, count)
	for i := 0; i < count; i++ {
		buckets[i] = start + float64(i)*width
	}
	return buckets
}

// ExponentialBuckets creates count buckets starting at start,
// where each bucket is factor times the previous one.
func ExponentialBuckets(start, factor float64, count int) []float64 {
	buckets := make([]float64, count)
	for i := 0; i < count; i++ {
		buckets[i] = start * math.Pow(factor, float64(i))
	}
	return buckets
}

// histogramCounts holds the atomic counters for a histogram.
type histogramCounts struct {
	buckets []uint64 // count for each bucket (atomic)
	count   uint64   // total count (atomic)
	sum     uint64   // sum stored as float64 bits (atomic)
}

// Histogram tracks value distributions with configurable buckets.
type Histogram struct {
	desc         *Desc
	labels       Labels
	bucketBounds []float64 // upper bounds for each bucket
	counts       histogramCounts
}

// NewHistogram creates a new histogram with the given options.
func NewHistogram(opts HistogramOpts) *Histogram {
	buckets := opts.Buckets
	if len(buckets) == 0 {
		buckets = DefaultBuckets
	}

	// Ensure buckets are sorted
	sortedBuckets := make([]float64, len(buckets))
	copy(sortedBuckets, buckets)
	sort.Float64s(sortedBuckets)

	return &Histogram{
		desc:         NewDesc(opts.Name, opts.Help, TypeHistogram, opts.ConstLabels, nil),
		labels:       opts.ConstLabels,
		bucketBounds: sortedBuckets,
		counts: histogramCounts{
			buckets: make([]uint64, len(sortedBuckets)),
		},
	}
}

// Describe returns the histogram's descriptor.
func (h *Histogram) Describe() *Desc {
	return h.desc
}

// Observe adds a value to the histogram.
func (h *Histogram) Observe(val float64) {
	// Increment bucket counters
	for i, bound := range h.bucketBounds {
		if val <= bound {
			atomic.AddUint64(&h.counts.buckets[i], 1)
		}
	}

	// Increment total count
	atomic.AddUint64(&h.counts.count, 1)

	// Add to sum (using CAS for atomic float addition)
	for {
		old := atomic.LoadUint64(&h.counts.sum)
		newSum := math.Float64frombits(old) + val
		if atomic.CompareAndSwapUint64(&h.counts.sum, old, math.Float64bits(newSum)) {
			break
		}
	}
}

// Write writes the histogram to the given writer in Prometheus format.
func (h *Histogram) Write(w io.Writer) error {
	baseName := h.desc.Name

	// Write bucket values
	for i, bound := range h.bucketBounds {
		bucketLabels := h.labels.Clone()
		bucketLabels["le"] = formatFloat(bound)
		labelStr := formatLabels(bucketLabels)
		count := atomic.LoadUint64(&h.counts.buckets[i])
		if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", baseName, labelStr, count); err != nil {
			return err
		}
	}

	// Write +Inf bucket
	infLabels := h.labels.Clone()
	infLabels["le"] = "+Inf"
	labelStr := formatLabels(infLabels)
	totalCount := atomic.LoadUint64(&h.counts.count)
	if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", baseName, labelStr, totalCount); err != nil {
		return err
	}

	// Write sum
	baseLabels := formatLabels(h.labels)
	sum := math.Float64frombits(atomic.LoadUint64(&h.counts.sum))
	if _, err := fmt.Fprintf(w, "%s_sum%s %s\n", baseName, baseLabels, formatFloat(sum)); err != nil {
		return err
	}

	// Write count
	if _, err := fmt.Fprintf(w, "%s_count%s %d\n", baseName, baseLabels, totalCount); err != nil {
		return err
	}

	return nil
}

// HistogramVec is a collection of histograms with the same name but different labels.
type HistogramVec struct {
	desc       *Desc
	opts       HistogramOpts
	mu         sync.RWMutex
	histograms map[string]*Histogram
}

// NewHistogramVec creates a new histogram vector.
func NewHistogramVec(opts HistogramOpts, labelNames []string) *HistogramVec {
	return &HistogramVec{
		desc:       NewDesc(opts.Name, opts.Help, TypeHistogram, opts.ConstLabels, labelNames),
		opts:       opts,
		histograms: make(map[string]*Histogram),
	}
}

// Describe returns the histogram vector's descriptor.
func (hv *HistogramVec) Describe() *Desc {
	return hv.desc
}

// WithLabels returns the histogram for the given labels.
func (hv *HistogramVec) WithLabels(labels Labels) *Histogram {
	key := labels.String()

	hv.mu.RLock()
	histogram, ok := hv.histograms[key]
	hv.mu.RUnlock()

	if ok {
		return histogram
	}

	hv.mu.Lock()
	defer hv.mu.Unlock()

	if histogram, ok = hv.histograms[key]; ok {
		return histogram
	}

	allLabels := hv.desc.ConstLabels.Merge(labels)
	buckets := hv.opts.Buckets
	if len(buckets) == 0 {
		buckets = DefaultBuckets
	}
	sortedBuckets := make([]float64, len(buckets))
	copy(sortedBuckets, buckets)
	sort.Float64s(sortedBuckets)

	histogram = &Histogram{
		desc:         hv.desc,
		labels:       allLabels,
		bucketBounds: sortedBuckets,
		counts: histogramCounts{
			buckets: make([]uint64, len(sortedBuckets)),
		},
	}
	hv.histograms[key] = histogram
	return histogram
}

// Write writes all histograms to the given writer in Prometheus format.
func (hv *HistogramVec) Write(w io.Writer) error {
	hv.mu.RLock()
	defer hv.mu.RUnlock()

	for _, histogram := range hv.histograms {
		if err := histogram.Write(w); err != nil {
			return err
		}
	}
	return nil
}

// SummaryOpts configures a summary.
type SummaryOpts struct {
	Name        string
	Help        string
	ConstLabels Labels
	Objectives  map[float64]float64 // quantile -> allowed error
	MaxAge      time.Duration
	AgeBuckets  int
}

// Summary calculates quantiles over a sliding time window.
// Note: This is a simplified implementation. For production use with high precision,
// consider using histograms instead.
type Summary struct {
	desc       *Desc
	labels     Labels
	objectives map[float64]float64
	maxAge     time.Duration
	mu         sync.Mutex
	samples    []timedSample
	count      uint64
	sum        uint64
}

type timedSample struct {
	value float64
	time  time.Time
}

// NewSummary creates a new summary with the given options.
func NewSummary(opts SummaryOpts) *Summary {
	objectives := opts.Objectives
	if objectives == nil {
		objectives = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	}

	maxAge := opts.MaxAge
	if maxAge == 0 {
		maxAge = 10 * time.Minute
	}

	return &Summary{
		desc:       NewDesc(opts.Name, opts.Help, TypeSummary, opts.ConstLabels, nil),
		labels:     opts.ConstLabels,
		objectives: objectives,
		maxAge:     maxAge,
		samples:    make([]timedSample, 0, 1000),
	}
}

// Describe returns the summary's descriptor.
func (s *Summary) Describe() *Desc {
	return s.desc
}

// Observe adds a value to the summary.
func (s *Summary) Observe(val float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Add new sample
	s.samples = append(s.samples, timedSample{value: val, time: now})

	// Increment count
	atomic.AddUint64(&s.count, 1)

	// Add to sum
	for {
		old := atomic.LoadUint64(&s.sum)
		newSum := math.Float64frombits(old) + val
		if atomic.CompareAndSwapUint64(&s.sum, old, math.Float64bits(newSum)) {
			break
		}
	}

	// Expire old samples
	cutoff := now.Add(-s.maxAge)
	newStart := 0
	for i, sample := range s.samples {
		if sample.time.After(cutoff) {
			newStart = i
			break
		}
	}
	if newStart > 0 {
		s.samples = s.samples[newStart:]
	}
}

// quantile calculates the given quantile from the samples.
func (s *Summary) quantile(q float64) float64 {
	if len(s.samples) == 0 {
		return math.NaN()
	}

	values := make([]float64, len(s.samples))
	for i, sample := range s.samples {
		values[i] = sample.value
	}
	sort.Float64s(values)

	index := q * float64(len(values)-1)
	lower := int(index)
	upper := lower + 1
	if upper >= len(values) {
		return values[len(values)-1]
	}

	fraction := index - float64(lower)
	return values[lower]*(1-fraction) + values[upper]*fraction
}

// Write writes the summary to the given writer in Prometheus format.
func (s *Summary) Write(w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	baseName := s.desc.Name

	// Write quantile values
	quantiles := make([]float64, 0, len(s.objectives))
	for q := range s.objectives {
		quantiles = append(quantiles, q)
	}
	sort.Float64s(quantiles)

	for _, q := range quantiles {
		quantileLabels := s.labels.Clone()
		quantileLabels["quantile"] = formatFloat(q)
		labelStr := formatLabels(quantileLabels)
		value := s.quantile(q)
		if _, err := fmt.Fprintf(w, "%s%s %s\n", baseName, labelStr, formatFloat(value)); err != nil {
			return err
		}
	}

	// Write sum and count
	baseLabels := formatLabels(s.labels)
	sum := math.Float64frombits(atomic.LoadUint64(&s.sum))
	count := atomic.LoadUint64(&s.count)

	if _, err := fmt.Fprintf(w, "%s_sum%s %s\n", baseName, baseLabels, formatFloat(sum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s_count%s %d\n", baseName, baseLabels, count); err != nil {
		return err
	}

	return nil
}

// Collector is the interface for types that can collect metrics.
type Collector interface {
	// Describe returns all possible descriptors.
	Describe() *Desc
	// Write writes all metrics to the writer.
	Write(w io.Writer) error
}

// Registry holds all registered metrics.
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
}

// NewRegistry creates a new metric registry.
func NewRegistry() *Registry {
	return &Registry{
		collectors: make(map[string]Collector),
	}
}

// DefaultRegistry is the default global registry.
var DefaultRegistry = NewRegistry()

// Register adds a collector to the registry.
func (r *Registry) Register(c Collector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	desc := c.Describe()
	if desc == nil {
		return fmt.Errorf("collector returned nil descriptor")
	}

	if _, exists := r.collectors[desc.Name]; exists {
		return fmt.Errorf("metric %q already registered", desc.Name)
	}

	r.collectors[desc.Name] = c
	return nil
}

// MustRegister registers a collector and panics on error.
func (r *Registry) MustRegister(c Collector) {
	if err := r.Register(c); err != nil {
		panic(err)
	}
}

// Unregister removes a collector from the registry.
func (r *Registry) Unregister(c Collector) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	desc := c.Describe()
	if desc == nil {
		return false
	}

	if _, exists := r.collectors[desc.Name]; !exists {
		return false
	}

	delete(r.collectors, desc.Name)
	return true
}

// Gather collects all metrics and writes them to the writer.
func (r *Registry) Gather(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect metric names for deterministic ordering
	names := make([]string, 0, len(r.collectors))
	for name := range r.collectors {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c := r.collectors[name]
		desc := c.Describe()

		// Write HELP line
		if desc.Help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", desc.Name, desc.Help); err != nil {
				return err
			}
		}

		// Write TYPE line
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", desc.Name, desc.Type); err != nil {
			return err
		}

		// Write metric values
		if err := c.Write(w); err != nil {
			return err
		}
	}

	return nil
}

// Handler returns an HTTP handler that exposes metrics in Prometheus format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		if err := r.Gather(w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// Register adds a collector to the default registry.
func Register(c Collector) error {
	return DefaultRegistry.Register(c)
}

// MustRegister registers a collector with the default registry and panics on error.
func MustRegister(c Collector) {
	DefaultRegistry.MustRegister(c)
}

// Handler returns an HTTP handler using the default registry.
func Handler() http.Handler {
	return DefaultRegistry.Handler()
}

// PushConfig configures push gateway settings.
type PushConfig struct {
	URL      string
	Job      string
	Instance string
	Grouping map[string]string
	Client   *http.Client
}

// Push pushes metrics to a push gateway.
func (r *Registry) Push(config PushConfig) error {
	if config.Client == nil {
		config.Client = http.DefaultClient
	}

	var buf bytes.Buffer
	if err := r.Gather(&buf); err != nil {
		return fmt.Errorf("gathering metrics: %w", err)
	}

	// Build URL with grouping keys
	url := strings.TrimSuffix(config.URL, "/")
	url += "/metrics/job/" + config.Job

	if config.Instance != "" {
		url += "/instance/" + config.Instance
	}

	for k, v := range config.Grouping {
		url += "/" + k + "/" + v
	}

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	resp, err := config.Client.Do(req)
	if err != nil {
		return fmt.Errorf("pushing metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push gateway returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// MetricFamily groups related metrics together.
type MetricFamily struct {
	Name        string
	Help        string
	Type        MetricType
	ConstLabels Labels
	mu          sync.RWMutex
	metrics     []Collector
}

// NewMetricFamily creates a new metric family.
func NewMetricFamily(name, help string, metricType MetricType, constLabels Labels) *MetricFamily {
	return &MetricFamily{
		Name:        name,
		Help:        help,
		Type:        metricType,
		ConstLabels: constLabels,
		metrics:     make([]Collector, 0),
	}
}

// Add adds a metric to the family.
func (mf *MetricFamily) Add(c Collector) {
	mf.mu.Lock()
	defer mf.mu.Unlock()
	mf.metrics = append(mf.metrics, c)
}

// Describe returns the family's descriptor.
func (mf *MetricFamily) Describe() *Desc {
	return &Desc{
		Name:        mf.Name,
		Help:        mf.Help,
		Type:        mf.Type,
		ConstLabels: mf.ConstLabels,
	}
}

// Write writes all metrics in the family to the writer.
func (mf *MetricFamily) Write(w io.Writer) error {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	for _, m := range mf.metrics {
		if err := m.Write(w); err != nil {
			return err
		}
	}
	return nil
}

// Timer is a helper for measuring durations.
type Timer struct {
	observer func(float64)
	start    time.Time
}

// NewTimer creates a new timer that will observe the duration when stopped.
func NewTimer(observer func(float64)) *Timer {
	return &Timer{
		observer: observer,
		start:    time.Now(),
	}
}

// ObserveDuration stops the timer and observes the duration.
func (t *Timer) ObserveDuration() time.Duration {
	d := time.Since(t.start)
	if t.observer != nil {
		t.observer(d.Seconds())
	}
	return d
}

// formatFloat formats a float64 for Prometheus output.
func formatFloat(v float64) string {
	if math.IsNaN(v) {
		return "NaN"
	}
	if math.IsInf(v, 1) {
		return "+Inf"
	}
	if math.IsInf(v, -1) {
		return "-Inf"
	}
	return fmt.Sprintf("%g", v)
}

// BuildInfo creates standard build info metrics.
type BuildInfo struct {
	Version   string
	Revision  string
	Branch    string
	BuildDate string
	GoVersion string
}

// NewBuildInfoGauge creates a gauge with build information as labels.
func NewBuildInfoGauge(namespace string, info BuildInfo) *Gauge {
	labels := Labels{
		"version":    info.Version,
		"revision":   info.Revision,
		"branch":     info.Branch,
		"build_date": info.BuildDate,
		"go_version": info.GoVersion,
	}

	name := namespace
	if name != "" {
		name += "_"
	}
	name += "build_info"

	gauge := NewGauge(name, "Build information for the application", labels)
	gauge.Set(1)
	return gauge
}

// ProcessCollector collects metrics about the current process.
type ProcessCollector struct {
	namespace   string
	cpuSeconds  *Counter
	openFDs     *Gauge
	maxFDs      *Gauge
	virtualMem  *Gauge
	residentMem *Gauge
	startTime   *Gauge
}

// NewProcessCollector creates a new process collector.
func NewProcessCollector(namespace string) *ProcessCollector {
	pc := &ProcessCollector{
		namespace: namespace,
	}

	prefix := ""
	if namespace != "" {
		prefix = namespace + "_"
	}

	pc.cpuSeconds = NewCounter(prefix+"process_cpu_seconds_total", "Total user and system CPU time spent in seconds", nil)
	pc.openFDs = NewGauge(prefix+"process_open_fds", "Number of open file descriptors", nil)
	pc.maxFDs = NewGauge(prefix+"process_max_fds", "Maximum number of open file descriptors", nil)
	pc.virtualMem = NewGauge(prefix+"process_virtual_memory_bytes", "Virtual memory size in bytes", nil)
	pc.residentMem = NewGauge(prefix+"process_resident_memory_bytes", "Resident memory size in bytes", nil)
	pc.startTime = NewGauge(prefix+"process_start_time_seconds", "Start time of the process since unix epoch in seconds", nil)

	return pc
}

// Describe returns the process collector's descriptor.
func (pc *ProcessCollector) Describe() *Desc {
	return &Desc{
		Name: pc.namespace + "_process",
		Help: "Process metrics",
		Type: TypeGauge,
	}
}

// Write writes all process metrics.
func (pc *ProcessCollector) Write(w io.Writer) error {
	// Note: Actual process metrics would require platform-specific code.
	// This is a placeholder implementation.
	return nil
}

// InstrumentHandler wraps an HTTP handler to record request metrics.
func InstrumentHandler(name string, handler http.Handler, duration *HistogramVec, requests *CounterVec) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		labels := Labels{
			"handler": name,
			"method":  r.Method,
		}

		timer := NewTimer(func(d float64) {
			duration.WithLabels(labels).Observe(d)
		})

		handler.ServeHTTP(w, r)

		timer.ObserveDuration()
		requests.WithLabels(labels).Inc()
	})
}

// StatusRecorder wraps ResponseWriter to capture the status code.
type StatusRecorder struct {
	http.ResponseWriter
	Status int
}

// NewStatusRecorder creates a new StatusRecorder.
func NewStatusRecorder(w http.ResponseWriter) *StatusRecorder {
	return &StatusRecorder{
		ResponseWriter: w,
		Status:         http.StatusOK,
	}
}

// WriteHeader captures the status code and writes the header.
func (sr *StatusRecorder) WriteHeader(code int) {
	sr.Status = code
	sr.ResponseWriter.WriteHeader(code)
}

// InstrumentHandlerWithStatus wraps an HTTP handler to record request metrics including status.
func InstrumentHandlerWithStatus(name string, handler http.Handler, duration *HistogramVec, requests *CounterVec) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := NewStatusRecorder(w)

		timer := NewTimer(nil)

		handler.ServeHTTP(recorder, r)

		elapsed := timer.ObserveDuration()

		labels := Labels{
			"handler": name,
			"method":  r.Method,
			"code":    fmt.Sprintf("%d", recorder.Status),
		}

		duration.WithLabels(labels).Observe(elapsed.Seconds())
		requests.WithLabels(labels).Inc()
	})
}
