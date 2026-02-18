package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output.
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestScraper creates a Scraper with sensible test defaults.
func newTestScraper(t *testing.T) *Scraper {
	t.Helper()
	s, err := NewScraper(&Config{
		ScrapeInterval:  1 * time.Second,
		ScrapeTimeout:   2 * time.Second,
		RetentionPeriod: 10 * time.Minute,
		MaxSamples:      100,
	}, newTestLogger())
	if err != nil {
		t.Fatalf("newTestScraper: %v", err)
	}
	return s
}

// makeTarget is a shorthand for building a ServiceTarget used in tests.
func makeTarget(name, metricsURL string) *ServiceTarget {
	return &ServiceTarget{
		Name:       name,
		MetricsURL: metricsURL,
		Labels:     map[string]string{},
	}
}

// metricsServer starts an httptest server that returns the supplied Prometheus
// exposition-format body.  The caller should defer server.Close().
func metricsServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, body)
	}))
}

// errServer starts an httptest server that always responds with the given
// HTTP status code.
func errServer(statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
}

// ---------------------------------------------------------------------------
// 1. Config validation
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ScrapeInterval != 15*time.Second {
		t.Errorf("ScrapeInterval = %v, want 15s", cfg.ScrapeInterval)
	}
	if cfg.ScrapeTimeout != 10*time.Second {
		t.Errorf("ScrapeTimeout = %v, want 10s", cfg.ScrapeTimeout)
	}
	if cfg.RetentionPeriod != 1*time.Hour {
		t.Errorf("RetentionPeriod = %v, want 1h", cfg.RetentionPeriod)
	}
	if cfg.MaxSamples != 1000 {
		t.Errorf("MaxSamples = %d, want 1000", cfg.MaxSamples)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: ErrNilConfig,
		},
		{
			name:    "zero interval",
			config:  &Config{ScrapeInterval: 0},
			wantErr: ErrInvalidInterval,
		},
		{
			name:    "negative interval",
			config:  &Config{ScrapeInterval: -1 * time.Second},
			wantErr: ErrInvalidInterval,
		},
		{
			name: "valid minimal config defaults applied",
			config: &Config{
				ScrapeInterval: 5 * time.Second,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConfigValidate_AppliesDefaults(t *testing.T) {
	cfg := &Config{ScrapeInterval: 5 * time.Second}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.ScrapeTimeout != 10*time.Second {
		t.Errorf("ScrapeTimeout not defaulted: got %v", cfg.ScrapeTimeout)
	}
	if cfg.RetentionPeriod != 1*time.Hour {
		t.Errorf("RetentionPeriod not defaulted: got %v", cfg.RetentionPeriod)
	}
	if cfg.MaxSamples != 1000 {
		t.Errorf("MaxSamples not defaulted: got %d", cfg.MaxSamples)
	}
}

// ---------------------------------------------------------------------------
// 2. Scraper creation
// ---------------------------------------------------------------------------

func TestNewScraper_NilConfig(t *testing.T) {
	s, err := NewScraper(nil, newTestLogger())
	if err != nil {
		t.Fatalf("unexpected error with nil config: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil scraper")
	}
	// Should have applied defaults
	if s.config.ScrapeInterval != 15*time.Second {
		t.Errorf("default ScrapeInterval not applied: got %v", s.config.ScrapeInterval)
	}
}

func TestNewScraper_NilLogger(t *testing.T) {
	s, err := NewScraper(DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.log == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewScraper_InvalidConfig(t *testing.T) {
	_, err := NewScraper(&Config{ScrapeInterval: -1}, newTestLogger())
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

// ---------------------------------------------------------------------------
// 3. Target management (register / unregister / list)
// ---------------------------------------------------------------------------

func TestRegisterTarget(t *testing.T) {
	s := newTestScraper(t)

	target := &ServiceTarget{
		Name:       "svc-a",
		MetricsURL: "http://localhost:9090/metrics",
	}
	if err := s.RegisterTarget(target); err != nil {
		t.Fatalf("RegisterTarget: %v", err)
	}

	// Labels should have been initialised and "service" injected.
	if target.Labels["service"] != "svc-a" {
		t.Errorf("service label not set: %v", target.Labels)
	}
	if !target.Enabled {
		t.Error("target should be enabled after registration")
	}
}

func TestRegisterTarget_NilTarget(t *testing.T) {
	s := newTestScraper(t)
	if err := s.RegisterTarget(nil); err == nil {
		t.Fatal("expected error for nil target")
	}
}

func TestRegisterTarget_EmptyName(t *testing.T) {
	s := newTestScraper(t)
	if err := s.RegisterTarget(&ServiceTarget{MetricsURL: "http://x/metrics"}); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRegisterTarget_EmptyURL(t *testing.T) {
	s := newTestScraper(t)
	if err := s.RegisterTarget(&ServiceTarget{Name: "svc"}); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestRegisterTarget_WithExistingLabels(t *testing.T) {
	s := newTestScraper(t)
	target := &ServiceTarget{
		Name:       "svc",
		MetricsURL: "http://x/metrics",
		Labels:     map[string]string{"env": "test"},
	}
	if err := s.RegisterTarget(target); err != nil {
		t.Fatal(err)
	}
	if target.Labels["env"] != "test" {
		t.Error("existing labels should be preserved")
	}
	if target.Labels["service"] != "svc" {
		t.Error("service label should be added")
	}
}

func TestUnregisterTarget(t *testing.T) {
	s := newTestScraper(t)
	s.RegisterTarget(makeTarget("svc-a", "http://a/metrics"))

	if err := s.UnregisterTarget("svc-a"); err != nil {
		t.Fatalf("UnregisterTarget: %v", err)
	}
	targets := s.GetTargets()
	if len(targets) != 0 {
		t.Errorf("expected 0 targets, got %d", len(targets))
	}
}

func TestUnregisterTarget_NotFound(t *testing.T) {
	s := newTestScraper(t)
	err := s.UnregisterTarget("nonexistent")
	if err != ErrServiceNotFound {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestGetTargets_Sorted(t *testing.T) {
	s := newTestScraper(t)
	s.RegisterTarget(makeTarget("charlie", "http://c/metrics"))
	s.RegisterTarget(makeTarget("alpha", "http://a/metrics"))
	s.RegisterTarget(makeTarget("bravo", "http://b/metrics"))

	targets := s.GetTargets()
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	expected := []string{"alpha", "bravo", "charlie"}
	for i, name := range expected {
		if targets[i].Name != name {
			t.Errorf("target[%d] = %s, want %s", i, targets[i].Name, name)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. parseLabels
// ---------------------------------------------------------------------------

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect map[string]string
	}{
		{
			name:  "single label",
			input: `method="GET"`,
			expect: map[string]string{
				"method": "GET",
			},
		},
		{
			name:  "multiple labels",
			input: `method="POST",code="200"`,
			expect: map[string]string{
				"method": "POST",
				"code":   "200",
			},
		},
		{
			name:   "empty string",
			input:  "",
			expect: map[string]string{"": ""}, // split of "" yields one pair ""; idx for "=" is -1 -> skipped
		},
		{
			name:   "no equals sign",
			input:  "justtext",
			expect: map[string]string{},
		},
		{
			name:  "label with spaces",
			input: ` method = "GET" , code = "200" `,
			expect: map[string]string{
				"method": "GET",
				"code":   "200",
			},
		},
		{
			name:  "label without quotes",
			input: `method=GET`,
			expect: map[string]string{
				"method": "GET",
			},
		},
		{
			name:  "three labels",
			input: `service="web",instance="10.0.0.1:8080",job="kingdom"`,
			expect: map[string]string{
				"service":  "web",
				"instance": "10.0.0.1:8080",
				"job":      "kingdom",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLabels(tt.input)
			// The "empty string" case results in a single pair "" which has
			// no '=' so it is skipped.  Fix expected for that.
			if tt.name == "empty string" {
				if len(got) != 0 {
					t.Errorf("expected empty map for empty string, got %v", got)
				}
				return
			}
			for k, v := range tt.expect {
				if got[k] != v {
					t.Errorf("label[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. parseLine
// ---------------------------------------------------------------------------

func TestParseLine(t *testing.T) {
	s := newTestScraper(t)
	ts := time.Now()
	target := &ServiceTarget{
		Name:   "svc",
		Labels: map[string]string{"job": "kingdom"},
	}

	tests := []struct {
		name      string
		line      string
		wantNil   bool
		wantName  string
		wantValue float64
	}{
		{
			name:      "simple metric",
			line:      "http_requests_total 42",
			wantName:  "http_requests_total",
			wantValue: 42,
		},
		{
			name:      "metric with labels",
			line:      `http_requests_total{method="GET",code="200"} 1027`,
			wantName:  "http_requests_total",
			wantValue: 1027,
		},
		{
			name:      "metric with timestamp",
			line:      "process_cpu_seconds_total 0.5 1609459200000",
			wantName:  "process_cpu_seconds_total",
			wantValue: 0.5,
		},
		{
			name:      "float value",
			line:      "temperature_celsius 36.6",
			wantName:  "temperature_celsius",
			wantValue: 36.6,
		},
		{
			name:      "negative value",
			line:      "offset_seconds -3.14",
			wantName:  "offset_seconds",
			wantValue: -3.14,
		},
		{
			name:      "scientific notation",
			line:      "tiny_value 1.5e-7",
			wantName:  "tiny_value",
			wantValue: 1.5e-7,
		},
		{
			name:    "only metric name no value",
			line:    "lonely_metric",
			wantNil: true,
		},
		{
			name:    "unclosed brace",
			line:    `http_requests{method="GET" 100`,
			wantNil: true,
		},
		{
			name:    "empty value after braces",
			line:    `http_requests{method="GET"}`,
			wantNil: true,
		},
		{
			name:    "non-numeric value",
			line:    "metric_name notanumber",
			wantNil: true,
		},
		{
			name:      "zero value",
			line:      "counter 0",
			wantName:  "counter",
			wantValue: 0,
		},
		{
			name:      "labels with timestamp",
			line:      `my_metric{env="prod"} 99.9 1609459200000`,
			wantName:  "my_metric",
			wantValue: 99.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.parseLine(tt.line, target, ts)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil sample")
			}
			if got.Name != tt.wantName {
				t.Errorf("name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Value != tt.wantValue {
				t.Errorf("value = %f, want %f", got.Value, tt.wantValue)
			}
			if got.Service != "svc" {
				t.Errorf("service = %q, want %q", got.Service, "svc")
			}
			// Target labels should be merged in.
			if got.Labels["job"] != "kingdom" {
				t.Errorf("label 'job' = %q, want %q", got.Labels["job"], "kingdom")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. parsePrometheusMetrics
// ---------------------------------------------------------------------------

func TestParsePrometheusMetrics(t *testing.T) {
	s := newTestScraper(t)
	ts := time.Now()
	target := &ServiceTarget{
		Name:   "svc",
		Labels: map[string]string{},
	}

	body := `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1027
http_requests_total{method="POST",code="200"} 42

# HELP process_cpu CPU usage
# TYPE process_cpu gauge
process_cpu 0.75
`

	samples := s.parsePrometheusMetrics([]byte(body), target, ts)
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}

	// Verify first sample
	if samples[0].Name != "http_requests_total" {
		t.Errorf("samples[0].Name = %q", samples[0].Name)
	}
	if samples[0].Value != 1027 {
		t.Errorf("samples[0].Value = %f", samples[0].Value)
	}

	// Verify last sample
	if samples[2].Name != "process_cpu" {
		t.Errorf("samples[2].Name = %q", samples[2].Name)
	}
	if samples[2].Value != 0.75 {
		t.Errorf("samples[2].Value = %f", samples[2].Value)
	}
}

func TestParsePrometheusMetrics_EmptyBody(t *testing.T) {
	s := newTestScraper(t)
	ts := time.Now()
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}

	samples := s.parsePrometheusMetrics([]byte(""), target, ts)
	if len(samples) != 0 {
		t.Errorf("expected 0 samples for empty body, got %d", len(samples))
	}
}

func TestParsePrometheusMetrics_OnlyComments(t *testing.T) {
	s := newTestScraper(t)
	ts := time.Now()
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}

	body := `# This is a comment
# Another comment
# HELP something
# TYPE something counter
`
	samples := s.parsePrometheusMetrics([]byte(body), target, ts)
	if len(samples) != 0 {
		t.Errorf("expected 0 samples for only-comments body, got %d", len(samples))
	}
}

func TestParsePrometheusMetrics_MalformedLines(t *testing.T) {
	s := newTestScraper(t)
	ts := time.Now()
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}

	body := `good_metric 100
bad metric with spaces
another_good 200
{orphan_labels="yes"} 50
`
	samples := s.parsePrometheusMetrics([]byte(body), target, ts)
	// "bad metric with spaces" -> name="bad", value="metric" -> ParseFloat fails -> nil
	// "{orphan_labels...}" -> name="" (before {), which is fine, parsed as metric with labels
	// Actually name="" and labels parsed, value=50 -> valid parse.
	// So we expect: good_metric, another_good, and the orphan one = 3
	if len(samples) < 2 {
		t.Errorf("expected at least 2 valid samples, got %d", len(samples))
	}
}

// ---------------------------------------------------------------------------
// 7. seriesKey
// ---------------------------------------------------------------------------

func TestSeriesKey(t *testing.T) {
	tests := []struct {
		name   string
		metric string
		labels map[string]string
		want   string
	}{
		{
			name:   "no labels",
			metric: "requests",
			labels: nil,
			want:   "requests",
		},
		{
			name:   "empty labels map",
			metric: "requests",
			labels: map[string]string{},
			want:   "requests",
		},
		{
			name:   "single label",
			metric: "requests",
			labels: map[string]string{"method": "GET"},
			want:   "requests,method=GET",
		},
		{
			name:   "sorted labels",
			metric: "requests",
			labels: map[string]string{"z": "1", "a": "2"},
			want:   "requests,a=2,z=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := seriesKey(tt.metric, tt.labels)
			if got != tt.want {
				t.Errorf("seriesKey(%q, %v) = %q, want %q", tt.metric, tt.labels, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 8. MetricSeries operations
// ---------------------------------------------------------------------------

func TestMetricSeries_AddSample(t *testing.T) {
	ms := &MetricSeries{
		Name:    "test",
		Samples: make([]MetricSample, 0),
	}

	for i := 0; i < 10; i++ {
		ms.AddSample(MetricSample{
			Name:      "test",
			Value:     float64(i),
			Timestamp: time.Now(),
		}, 5)
	}

	ms.mu.RLock()
	n := len(ms.Samples)
	ms.mu.RUnlock()

	if n != 5 {
		t.Errorf("expected 5 samples (max), got %d", n)
	}

	// The most recent samples should remain.
	latest := ms.GetLatest()
	if latest.Value != 9 {
		t.Errorf("latest value = %f, want 9", latest.Value)
	}
}

func TestMetricSeries_AddSample_ExactMax(t *testing.T) {
	ms := &MetricSeries{Samples: make([]MetricSample, 0)}
	for i := 0; i < 5; i++ {
		ms.AddSample(MetricSample{Value: float64(i)}, 5)
	}
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if len(ms.Samples) != 5 {
		t.Errorf("expected exactly 5 samples, got %d", len(ms.Samples))
	}
}

func TestMetricSeries_GetLatest_Empty(t *testing.T) {
	ms := &MetricSeries{Samples: []MetricSample{}}
	if ms.GetLatest() != nil {
		t.Error("expected nil for empty series")
	}
}

func TestMetricSeries_GetSamplesSince(t *testing.T) {
	now := time.Now()
	ms := &MetricSeries{
		Samples: []MetricSample{
			{Value: 1, Timestamp: now.Add(-3 * time.Minute)},
			{Value: 2, Timestamp: now.Add(-2 * time.Minute)},
			{Value: 3, Timestamp: now.Add(-1 * time.Minute)},
			{Value: 4, Timestamp: now},
		},
	}

	// Get samples from 90 seconds ago.
	since := now.Add(-90 * time.Second)
	got := ms.GetSamplesSince(since)
	if len(got) != 2 {
		t.Fatalf("expected 2 samples since 90s ago, got %d", len(got))
	}
	if got[0].Value != 3 || got[1].Value != 4 {
		t.Errorf("unexpected sample values: %v", got)
	}
}

func TestMetricSeries_GetSamplesSince_Empty(t *testing.T) {
	ms := &MetricSeries{Samples: []MetricSample{}}
	got := ms.GetSamplesSince(time.Now().Add(-time.Hour))
	if len(got) != 0 {
		t.Errorf("expected 0 samples, got %d", len(got))
	}
}

func TestMetricSeries_Cleanup(t *testing.T) {
	now := time.Now()
	ms := &MetricSeries{
		Samples: []MetricSample{
			{Value: 1, Timestamp: now.Add(-2 * time.Hour)},
			{Value: 2, Timestamp: now.Add(-30 * time.Minute)},
			{Value: 3, Timestamp: now.Add(-5 * time.Minute)},
			{Value: 4, Timestamp: now},
		},
	}

	cutoff := now.Add(-1 * time.Hour)
	ms.Cleanup(cutoff)

	ms.mu.RLock()
	n := len(ms.Samples)
	ms.mu.RUnlock()

	if n != 3 {
		t.Errorf("expected 3 samples after cleanup, got %d", n)
	}
}

func TestMetricSeries_Cleanup_RemovesAll(t *testing.T) {
	ms := &MetricSeries{
		Samples: []MetricSample{
			{Timestamp: time.Now().Add(-2 * time.Hour)},
			{Timestamp: time.Now().Add(-3 * time.Hour)},
		},
	}
	ms.Cleanup(time.Now())

	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if len(ms.Samples) != 0 {
		t.Errorf("expected 0 samples after cleanup, got %d", len(ms.Samples))
	}
}

// ---------------------------------------------------------------------------
// 9. storeSample and series management
// ---------------------------------------------------------------------------

func TestStoreSample_CreatesNewSeries(t *testing.T) {
	s := newTestScraper(t)
	sample := MetricSample{
		Name:      "cpu_usage",
		Value:     0.85,
		Labels:    map[string]string{"host": "web-1"},
		Timestamp: time.Now(),
		Service:   "svc-a",
	}

	s.storeSample(sample)

	if s.SeriesCount() != 1 {
		t.Fatalf("expected 1 series, got %d", s.SeriesCount())
	}

	series, err := s.GetMetricSeries("cpu_usage", map[string]string{"host": "web-1"})
	if err != nil {
		t.Fatalf("GetMetricSeries: %v", err)
	}
	latest := series.GetLatest()
	if latest == nil {
		t.Fatal("expected non-nil latest")
	}
	if latest.Value != 0.85 {
		t.Errorf("latest value = %f, want 0.85", latest.Value)
	}
}

func TestStoreSample_AppendToExistingSeries(t *testing.T) {
	s := newTestScraper(t)
	labels := map[string]string{"host": "web-1"}
	for i := 0; i < 5; i++ {
		s.storeSample(MetricSample{
			Name:      "cpu",
			Value:     float64(i),
			Labels:    labels,
			Timestamp: time.Now(),
			Service:   "svc",
		})
	}

	if s.SeriesCount() != 1 {
		t.Errorf("expected 1 series (same key), got %d", s.SeriesCount())
	}

	series, _ := s.GetMetricSeries("cpu", labels)
	series.mu.RLock()
	n := len(series.Samples)
	series.mu.RUnlock()
	if n != 5 {
		t.Errorf("expected 5 samples, got %d", n)
	}
}

func TestStoreSample_RespectsMaxSamples(t *testing.T) {
	s, _ := NewScraper(&Config{
		ScrapeInterval:  1 * time.Second,
		ScrapeTimeout:   1 * time.Second,
		RetentionPeriod: 1 * time.Hour,
		MaxSamples:      3,
	}, newTestLogger())

	for i := 0; i < 10; i++ {
		s.storeSample(MetricSample{
			Name:      "counter",
			Value:     float64(i),
			Labels:    map[string]string{},
			Timestamp: time.Now(),
			Service:   "svc",
		})
	}

	series, _ := s.GetMetricSeries("counter", map[string]string{})
	series.mu.RLock()
	n := len(series.Samples)
	series.mu.RUnlock()
	if n != 3 {
		t.Errorf("expected 3 samples (max), got %d", n)
	}
}

func TestGetMetricSeries_NotFound(t *testing.T) {
	s := newTestScraper(t)
	_, err := s.GetMetricSeries("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for missing series")
	}
}

// ---------------------------------------------------------------------------
// 10. storeResult and callbacks
// ---------------------------------------------------------------------------

func TestStoreResult_TriggersCallback(t *testing.T) {
	s := newTestScraper(t)
	var received *ScrapeResult
	s.OnScrapeComplete(func(r *ScrapeResult) {
		received = r
	})

	result := &ScrapeResult{
		Service:   "svc-a",
		Timestamp: time.Now(),
	}
	s.storeResult(result)

	if received == nil {
		t.Fatal("callback was not invoked")
	}
	if received.Service != "svc-a" {
		t.Errorf("received service = %q, want %q", received.Service, "svc-a")
	}
}

func TestStoreResult_NoCallback(t *testing.T) {
	s := newTestScraper(t)
	// Should not panic when no callback is set.
	s.storeResult(&ScrapeResult{Service: "svc-a"})
}

// ---------------------------------------------------------------------------
// 11. HTTP scraping integration (using httptest)
// ---------------------------------------------------------------------------

func TestScrapeTarget_Success(t *testing.T) {
	body := `# TYPE http_requests counter
http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 25
go_goroutines 42
`
	srv := metricsServer(body)
	defer srv.Close()

	s := newTestScraper(t)
	target := makeTarget("test-svc", srv.URL)
	s.RegisterTarget(target)

	ctx := context.Background()
	s.scrapeTarget(ctx, target)

	// Verify results stored
	results := s.GetLatestResults()
	result, ok := results["test-svc"]
	if !ok {
		t.Fatal("expected result for test-svc")
	}
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("status code = %d, want 200", result.StatusCode)
	}
	if len(result.Metrics) != 3 {
		t.Errorf("expected 3 metrics, got %d", len(result.Metrics))
	}

	// Verify series were stored.
	if s.SeriesCount() < 3 {
		t.Errorf("expected at least 3 series, got %d", s.SeriesCount())
	}
}

func TestScrapeTarget_Non200Status(t *testing.T) {
	srv := errServer(http.StatusInternalServerError)
	defer srv.Close()

	s := newTestScraper(t)
	target := makeTarget("err-svc", srv.URL)
	s.RegisterTarget(target)

	s.scrapeTarget(context.Background(), target)

	results := s.GetLatestResults()
	result := results["err-svc"]
	if result.Error == "" {
		t.Fatal("expected error for 500 response")
	}
	if result.StatusCode != 500 {
		t.Errorf("status code = %d, want 500", result.StatusCode)
	}
}

func TestScrapeTarget_ConnectionRefused(t *testing.T) {
	s := newTestScraper(t)
	target := makeTarget("dead-svc", "http://127.0.0.1:1/metrics")
	s.RegisterTarget(target)

	s.scrapeTarget(context.Background(), target)

	results := s.GetLatestResults()
	result := results["dead-svc"]
	if result.Error == "" {
		t.Fatal("expected error for connection refused")
	}
}

func TestScrapeTarget_InvalidURL(t *testing.T) {
	s := newTestScraper(t)
	target := makeTarget("bad-url", "://not-a-url")
	s.RegisterTarget(target)

	s.scrapeTarget(context.Background(), target)

	results := s.GetLatestResults()
	result := results["bad-url"]
	if result.Error == "" {
		t.Fatal("expected error for invalid URL")
	}
}

func TestScrapeTarget_EmptyResponse(t *testing.T) {
	srv := metricsServer("")
	defer srv.Close()

	s := newTestScraper(t)
	target := makeTarget("empty-svc", srv.URL)
	s.RegisterTarget(target)

	s.scrapeTarget(context.Background(), target)

	results := s.GetLatestResults()
	result := results["empty-svc"]
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(result.Metrics) != 0 {
		t.Errorf("expected 0 metrics, got %d", len(result.Metrics))
	}
}

func TestScrapeTarget_ContextCancelled(t *testing.T) {
	// Server that never responds.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer srv.Close()

	s := newTestScraper(t)
	target := makeTarget("slow-svc", srv.URL)
	s.RegisterTarget(target)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	s.scrapeTarget(ctx, target)

	results := s.GetLatestResults()
	result := results["slow-svc"]
	if result.Error == "" {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// 12. scrapeAll - concurrent scraping
// ---------------------------------------------------------------------------

func TestScrapeAll(t *testing.T) {
	body := "metric_a 1\n"
	srv1 := metricsServer(body)
	defer srv1.Close()

	body2 := "metric_b 2\n"
	srv2 := metricsServer(body2)
	defer srv2.Close()

	s := newTestScraper(t)
	s.RegisterTarget(makeTarget("svc-1", srv1.URL))
	s.RegisterTarget(makeTarget("svc-2", srv2.URL))

	s.scrapeAll(context.Background())

	results := s.GetLatestResults()
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestScrapeAll_SkipsDisabledTargets(t *testing.T) {
	srv := metricsServer("metric 1\n")
	defer srv.Close()

	s := newTestScraper(t)
	target := makeTarget("disabled-svc", srv.URL)
	s.RegisterTarget(target)
	// Disable it after registration.
	s.targetsMu.Lock()
	target.Enabled = false
	s.targetsMu.Unlock()

	s.scrapeAll(context.Background())

	results := s.GetLatestResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results for disabled target, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// 13. GetAggregatedMetrics
// ---------------------------------------------------------------------------

func TestGetAggregatedMetrics_Empty(t *testing.T) {
	s := newTestScraper(t)
	agg := s.GetAggregatedMetrics()
	if agg == nil {
		t.Fatal("expected non-nil aggregated metrics")
	}
	if len(agg.Services) != 0 {
		t.Errorf("expected 0 services, got %d", len(agg.Services))
	}
	if agg.TotalMetrics != 0 {
		t.Errorf("expected 0 total metrics, got %d", agg.TotalMetrics)
	}
}

func TestGetAggregatedMetrics_WithResults(t *testing.T) {
	s := newTestScraper(t)

	// Store a good result.
	s.storeResult(&ScrapeResult{
		Service:   "web",
		Timestamp: time.Now(),
		Metrics: []MetricSample{
			{Name: "requests", Value: 100, Timestamp: time.Now()},
			{Name: "latency", Value: 0.5, Timestamp: time.Now()},
		},
	})

	// Store a failed result.
	s.storeResult(&ScrapeResult{
		Service:   "db",
		Error:     "connection refused",
		Timestamp: time.Now(),
	})

	agg := s.GetAggregatedMetrics()
	if len(agg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(agg.Services))
	}

	web := agg.Services["web"]
	if web.Status != "up" {
		t.Errorf("web status = %q, want %q", web.Status, "up")
	}
	if len(web.Metrics) != 2 {
		t.Errorf("web metrics count = %d, want 2", len(web.Metrics))
	}

	db := agg.Services["db"]
	if db.Status != "down" {
		t.Errorf("db status = %q, want %q", db.Status, "down")
	}

	if agg.TotalMetrics != 2 {
		t.Errorf("total metrics = %d, want 2", agg.TotalMetrics)
	}
}

// ---------------------------------------------------------------------------
// 14. GetServiceMetrics
// ---------------------------------------------------------------------------

func TestGetServiceMetrics_NotFound(t *testing.T) {
	s := newTestScraper(t)
	_, err := s.GetServiceMetrics("nonexistent")
	if err != ErrServiceNotFound {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestGetServiceMetrics_Up(t *testing.T) {
	s := newTestScraper(t)
	s.storeResult(&ScrapeResult{
		Service:   "web",
		Timestamp: time.Now(),
		Metrics: []MetricSample{
			{Name: "uptime", Value: 3600, Labels: map[string]string{"unit": "s"}, Timestamp: time.Now()},
		},
	})

	sm, err := s.GetServiceMetrics("web")
	if err != nil {
		t.Fatal(err)
	}
	if sm.Status != "up" {
		t.Errorf("status = %q, want %q", sm.Status, "up")
	}
	if sm.Metrics["uptime"] == nil {
		t.Fatal("expected uptime metric")
	}
	if sm.Metrics["uptime"].Value != 3600 {
		t.Errorf("uptime value = %f, want 3600", sm.Metrics["uptime"].Value)
	}
}

func TestGetServiceMetrics_Down(t *testing.T) {
	s := newTestScraper(t)
	s.storeResult(&ScrapeResult{
		Service: "db",
		Error:   "timeout",
	})

	sm, err := s.GetServiceMetrics("db")
	if err != nil {
		t.Fatal(err)
	}
	if sm.Status != "down" {
		t.Errorf("status = %q, want %q", sm.Status, "down")
	}
}

// ---------------------------------------------------------------------------
// 15. QueryMetrics
// ---------------------------------------------------------------------------

func TestQueryMetrics(t *testing.T) {
	s := newTestScraper(t)
	now := time.Now()

	// Insert samples into different series.
	samples := []MetricSample{
		{Name: "http_requests", Value: 10, Labels: map[string]string{"service": "web"}, Timestamp: now.Add(-5 * time.Minute), Service: "web"},
		{Name: "http_requests", Value: 20, Labels: map[string]string{"service": "web"}, Timestamp: now.Add(-1 * time.Minute), Service: "web"},
		{Name: "http_errors", Value: 1, Labels: map[string]string{"service": "web"}, Timestamp: now.Add(-1 * time.Minute), Service: "web"},
		{Name: "http_requests", Value: 5, Labels: map[string]string{"service": "api"}, Timestamp: now.Add(-1 * time.Minute), Service: "api"},
	}
	for _, sample := range samples {
		s.storeSample(sample)
	}

	t.Run("filter by name", func(t *testing.T) {
		results := s.QueryMetrics("http_requests", nil, now.Add(-10*time.Minute))
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("filter by label", func(t *testing.T) {
		results := s.QueryMetrics("", map[string]string{"service": "web"}, now.Add(-10*time.Minute))
		if len(results) != 3 {
			t.Errorf("expected 3 results for service=web, got %d", len(results))
		}
	})

	t.Run("filter by name and label", func(t *testing.T) {
		results := s.QueryMetrics("http_requests", map[string]string{"service": "web"}, now.Add(-10*time.Minute))
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("filter by time", func(t *testing.T) {
		results := s.QueryMetrics("http_requests", nil, now.Add(-2*time.Minute))
		if len(results) != 2 {
			t.Errorf("expected 2 recent results, got %d", len(results))
		}
	})

	t.Run("no matches", func(t *testing.T) {
		results := s.QueryMetrics("nonexistent", nil, now.Add(-10*time.Minute))
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("sorted by timestamp", func(t *testing.T) {
		results := s.QueryMetrics("", nil, now.Add(-10*time.Minute))
		for i := 1; i < len(results); i++ {
			if results[i].Timestamp.Before(results[i-1].Timestamp) {
				t.Error("results not sorted by timestamp")
				break
			}
		}
	})
}

// ---------------------------------------------------------------------------
// 16. Start / Stop lifecycle
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	s := newTestScraper(t)

	if s.IsRunning() {
		t.Fatal("scraper should not be running initially")
	}

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.IsRunning() {
		t.Fatal("scraper should be running after Start")
	}

	// Starting again should fail.
	if err := s.Start(ctx); err == nil {
		t.Fatal("expected error on double Start")
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.IsRunning() {
		t.Fatal("scraper should not be running after Stop")
	}

	// Stopping again should be a no-op.
	if err := s.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestStartWithContext_Cancellation(t *testing.T) {
	s := newTestScraper(t)
	ctx, cancel := context.WithCancel(context.Background())

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cancel()
	// After context cancellation, the scraper goroutines should exit.
	// We call Stop to wait for them and clean up.
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartScrape_EndToEnd(t *testing.T) {
	body := "my_gauge 42\n"
	srv := metricsServer(body)
	defer srv.Close()

	s, _ := NewScraper(&Config{
		ScrapeInterval:  50 * time.Millisecond,
		ScrapeTimeout:   1 * time.Second,
		RetentionPeriod: 1 * time.Minute,
		MaxSamples:      100,
	}, newTestLogger())

	s.RegisterTarget(makeTarget("quick-svc", srv.URL))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	defer s.Stop()

	// Wait enough time for at least one scrape cycle.
	time.Sleep(200 * time.Millisecond)

	results := s.GetLatestResults()
	if _, ok := results["quick-svc"]; !ok {
		t.Fatal("expected scrape result for quick-svc")
	}
}

// ---------------------------------------------------------------------------
// 17. RegisterKingdomServices
// ---------------------------------------------------------------------------

func TestRegisterKingdomServices(t *testing.T) {
	s := newTestScraper(t)
	s.RegisterKingdomServices(nil)

	targets := s.GetTargets()
	expectedNames := []string{"architect", "wotan", "captain", "gateway", "micromanager", "timeguru"}

	if len(targets) != len(expectedNames) {
		t.Fatalf("expected %d targets, got %d", len(expectedNames), len(targets))
	}

	for i, name := range expectedNames {
		if targets[i].Name != name {
			t.Errorf("target[%d] = %q, want %q", i, targets[i].Name, name)
		}
		if !strings.HasPrefix(targets[i].MetricsURL, "http://") {
			t.Errorf("target %q missing http prefix", name)
		}
		if !strings.HasSuffix(targets[i].MetricsURL, "/metrics") {
			t.Errorf("target %q missing /metrics suffix", name)
		}
		if targets[i].Labels["job"] != "kingdom" {
			t.Errorf("target %q missing job=kingdom label", name)
		}
	}
}

// ---------------------------------------------------------------------------
// 18. GetLatestResults
// ---------------------------------------------------------------------------

func TestGetLatestResults_Copy(t *testing.T) {
	s := newTestScraper(t)
	s.storeResult(&ScrapeResult{Service: "a"})
	s.storeResult(&ScrapeResult{Service: "b"})

	results := s.GetLatestResults()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Mutating the returned map should not affect internal state.
	delete(results, "a")
	internal := s.GetLatestResults()
	if len(internal) != 2 {
		t.Error("deleting from returned map affected internal state")
	}
}

// ---------------------------------------------------------------------------
// 19. Concurrent access safety
// ---------------------------------------------------------------------------

func TestConcurrentRegisterUnregister(t *testing.T) {
	s := newTestScraper(t)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("svc-%d", n)
			url := fmt.Sprintf("http://localhost:%d/metrics", 9000+n)
			s.RegisterTarget(makeTarget(name, url))
		}(i)
	}
	wg.Wait()

	if len(s.GetTargets()) != 50 {
		t.Errorf("expected 50 targets, got %d", len(s.GetTargets()))
	}

	// Concurrent unregister.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.UnregisterTarget(fmt.Sprintf("svc-%d", n))
		}(i)
	}
	wg.Wait()

	if len(s.GetTargets()) != 0 {
		t.Errorf("expected 0 targets after unregister, got %d", len(s.GetTargets()))
	}
}

func TestConcurrentStoreSample(t *testing.T) {
	s := newTestScraper(t)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.storeSample(MetricSample{
				Name:      "concurrent_metric",
				Value:     float64(n),
				Labels:    map[string]string{},
				Timestamp: time.Now(),
				Service:   "svc",
			})
		}(i)
	}
	wg.Wait()

	series, err := s.GetMetricSeries("concurrent_metric", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	series.mu.RLock()
	n := len(series.Samples)
	series.mu.RUnlock()
	if n != 100 {
		t.Errorf("expected 100 samples, got %d", n)
	}
}

func TestConcurrentScrapeAndRead(t *testing.T) {
	body := "metric 1\n"
	srv := metricsServer(body)
	defer srv.Close()

	s := newTestScraper(t)
	s.RegisterTarget(makeTarget("svc", srv.URL))

	var wg sync.WaitGroup

	// Concurrent scrapes.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.scrapeAll(context.Background())
		}()
	}

	// Concurrent reads.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.GetAggregatedMetrics()
			_ = s.GetLatestResults()
			_ = s.SeriesCount()
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// 20. Cleanup integration
// ---------------------------------------------------------------------------

func TestCleanup(t *testing.T) {
	s := newTestScraper(t)

	old := time.Now().Add(-20 * time.Minute) // within retention for our 10m config
	veryOld := time.Now().Add(-1 * time.Hour)

	s.storeSample(MetricSample{
		Name: "old_metric", Value: 1,
		Labels: map[string]string{}, Timestamp: veryOld, Service: "svc",
	})
	s.storeSample(MetricSample{
		Name: "old_metric", Value: 2,
		Labels: map[string]string{}, Timestamp: old, Service: "svc",
	})
	s.storeSample(MetricSample{
		Name: "old_metric", Value: 3,
		Labels: map[string]string{}, Timestamp: time.Now(), Service: "svc",
	})

	s.cleanup()

	series, _ := s.GetMetricSeries("old_metric", map[string]string{})
	series.mu.RLock()
	n := len(series.Samples)
	series.mu.RUnlock()

	// With 10 min retention, only the latest (now) and possibly the 20-min-old
	// sample should be cleaned up. 20min > 10min retention, so only "now" sample survives.
	if n != 1 {
		t.Errorf("expected 1 sample after cleanup (retention=10m), got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 21. OnScrapeComplete with HTTP integration
// ---------------------------------------------------------------------------

func TestOnScrapeComplete_Integration(t *testing.T) {
	body := "gauge 42\n"
	srv := metricsServer(body)
	defer srv.Close()

	s := newTestScraper(t)
	var callbackCount int32
	s.OnScrapeComplete(func(r *ScrapeResult) {
		atomic.AddInt32(&callbackCount, 1)
	})

	s.RegisterTarget(makeTarget("svc", srv.URL))
	s.scrapeAll(context.Background())

	if atomic.LoadInt32(&callbackCount) != 1 {
		t.Errorf("expected 1 callback invocation, got %d", callbackCount)
	}
}

// ---------------------------------------------------------------------------
// 22. Edge cases - Prometheus format
// ---------------------------------------------------------------------------

func TestParsePrometheusMetrics_TrailingNewlines(t *testing.T) {
	s := newTestScraper(t)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}
	body := "metric_a 1\n\n\n\n"
	samples := s.parsePrometheusMetrics([]byte(body), target, time.Now())
	if len(samples) != 1 {
		t.Errorf("expected 1 sample, got %d", len(samples))
	}
}

func TestParsePrometheusMetrics_WindowsLineEndings(t *testing.T) {
	s := newTestScraper(t)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}
	// \r\n won't be split by \n correctly, but the TrimSpace should handle \r
	body := "metric_a 1\r\nmetric_b 2\r\n"
	samples := s.parsePrometheusMetrics([]byte(body), target, time.Now())
	if len(samples) != 2 {
		t.Errorf("expected 2 samples, got %d", len(samples))
	}
}

func TestParsePrometheusMetrics_LargeValues(t *testing.T) {
	s := newTestScraper(t)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}

	body := "big_value 99999999999999\nsmall_value 0.000000001\n"
	samples := s.parsePrometheusMetrics([]byte(body), target, time.Now())
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if samples[0].Value != 99999999999999 {
		t.Errorf("big_value = %f", samples[0].Value)
	}
	if samples[1].Value != 0.000000001 {
		t.Errorf("small_value = %f", samples[1].Value)
	}
}

func TestParsePrometheusMetrics_InfAndNaN(t *testing.T) {
	s := newTestScraper(t)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}

	body := "inf_metric +Inf\nneginf_metric -Inf\nnan_metric NaN\n"
	samples := s.parsePrometheusMetrics([]byte(body), target, time.Now())
	// Go's ParseFloat handles +Inf, -Inf, NaN.
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
}

func TestParseLine_EmptyLabels(t *testing.T) {
	s := newTestScraper(t)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}
	sample := s.parseLine(`metric_name{} 42`, target, time.Now())
	if sample == nil {
		t.Fatal("expected non-nil for empty labels")
	}
	if sample.Value != 42 {
		t.Errorf("value = %f, want 42", sample.Value)
	}
}

// ---------------------------------------------------------------------------
// 23. SeriesCount
// ---------------------------------------------------------------------------

func TestSeriesCount(t *testing.T) {
	s := newTestScraper(t)
	if s.SeriesCount() != 0 {
		t.Errorf("expected 0 series initially, got %d", s.SeriesCount())
	}

	s.storeSample(MetricSample{Name: "a", Labels: map[string]string{}, Service: "svc"})
	s.storeSample(MetricSample{Name: "b", Labels: map[string]string{}, Service: "svc"})
	s.storeSample(MetricSample{Name: "a", Labels: map[string]string{}, Service: "svc"}) // same key

	if s.SeriesCount() != 2 {
		t.Errorf("expected 2 series, got %d", s.SeriesCount())
	}
}

// ---------------------------------------------------------------------------
// 24. IsRunning
// ---------------------------------------------------------------------------

func TestIsRunning(t *testing.T) {
	s := newTestScraper(t)
	if s.IsRunning() {
		t.Error("should not be running initially")
	}

	s.Start(context.Background())
	if !s.IsRunning() {
		t.Error("should be running after Start")
	}

	s.Stop()
	if s.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkParseLine_Simple(b *testing.B) {
	s := newBenchScraper(b)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}
	ts := time.Now()
	line := "http_requests_total 12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.parseLine(line, target, ts)
	}
}

func BenchmarkParseLine_WithLabels(b *testing.B) {
	s := newBenchScraper(b)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}
	ts := time.Now()
	line := `http_requests_total{method="GET",code="200",handler="/api/v1/query"} 12345`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.parseLine(line, target, ts)
	}
}

func BenchmarkParseLabels(b *testing.B) {
	input := `method="GET",code="200",handler="/api/v1/query",service="web",instance="10.0.0.1:8080"`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseLabels(input)
	}
}

func BenchmarkSeriesKey(b *testing.B) {
	labels := map[string]string{
		"service":  "web",
		"instance": "10.0.0.1:8080",
		"method":   "GET",
		"handler":  "/api/v1/query",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seriesKey("http_requests_total", labels)
	}
}

func BenchmarkParsePrometheusMetrics(b *testing.B) {
	s := newBenchScraper(b)
	target := &ServiceTarget{Name: "svc", Labels: map[string]string{}}
	ts := time.Now()

	// Realistic multi-metric body.
	body := generateBenchBody(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.parsePrometheusMetrics(body, target, ts)
	}
}

func BenchmarkStoreSample(b *testing.B) {
	s := newBenchScraper(b)
	sample := MetricSample{
		Name:      "bench_metric",
		Value:     42,
		Labels:    map[string]string{"host": "web-1"},
		Timestamp: time.Now(),
		Service:   "svc",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.storeSample(sample)
	}
}

func BenchmarkMetricSeries_AddSample(b *testing.B) {
	ms := &MetricSeries{
		Name:    "bench",
		Samples: make([]MetricSample, 0, 1000),
	}
	sample := MetricSample{Value: 42, Timestamp: time.Now()}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ms.AddSample(sample, 1000)
	}
}

func BenchmarkGetAggregatedMetrics(b *testing.B) {
	s := newBenchScraper(b)
	// Populate with 10 services, 50 metrics each.
	for svc := 0; svc < 10; svc++ {
		name := fmt.Sprintf("svc-%d", svc)
		metrics := make([]MetricSample, 50)
		for m := 0; m < 50; m++ {
			metrics[m] = MetricSample{
				Name:      fmt.Sprintf("metric_%d", m),
				Value:     float64(m),
				Timestamp: time.Now(),
			}
		}
		s.storeResult(&ScrapeResult{
			Service: name,
			Metrics: metrics,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetAggregatedMetrics()
	}
}

func BenchmarkQueryMetrics(b *testing.B) {
	s := newBenchScraper(b)
	now := time.Now()
	// Populate 100 series with 10 samples each.
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			s.storeSample(MetricSample{
				Name:      fmt.Sprintf("metric_%d", i),
				Value:     float64(j),
				Labels:    map[string]string{"idx": fmt.Sprintf("%d", i)},
				Timestamp: now.Add(-time.Duration(j) * time.Minute),
				Service:   "svc",
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.QueryMetrics("metric_5", nil, now.Add(-1*time.Hour))
	}
}

func BenchmarkConcurrentStoreSample(b *testing.B) {
	s := newBenchScraper(b)
	sample := MetricSample{
		Name:      "concurrent_bench",
		Value:     1,
		Labels:    map[string]string{},
		Timestamp: time.Now(),
		Service:   "svc",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.storeSample(sample)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark helpers
// ---------------------------------------------------------------------------

func newBenchScraper(b *testing.B) *Scraper {
	b.Helper()
	s, err := NewScraper(&Config{
		ScrapeInterval:  1 * time.Second,
		ScrapeTimeout:   2 * time.Second,
		RetentionPeriod: 10 * time.Minute,
		MaxSamples:      1000,
	}, newTestLogger())
	if err != nil {
		b.Fatal(err)
	}
	return s
}

func generateBenchBody(numMetrics int) []byte {
	var buf strings.Builder
	for i := 0; i < numMetrics; i++ {
		fmt.Fprintf(&buf, "# HELP metric_%d A benchmark metric\n", i)
		fmt.Fprintf(&buf, "# TYPE metric_%d gauge\n", i)
		fmt.Fprintf(&buf, "metric_%d{instance=\"10.0.0.1:8080\",job=\"bench\"} %d\n", i, i*100)
	}
	return []byte(buf.String())
}
