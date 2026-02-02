package metrics_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"unheaded/pkg/metrics"
)

// Example_basic demonstrates basic counter and gauge usage.
func Example_basic() {
	// Create a counter
	requestCounter := metrics.NewCounter(
		"http_requests_total",
		"Total number of HTTP requests",
		nil,
	)

	// Increment the counter
	requestCounter.Inc()
	requestCounter.Add(5)

	fmt.Printf("Request count: %.0f\n", requestCounter.Value())

	// Create a gauge
	activeConnections := metrics.NewGauge(
		"active_connections",
		"Number of active connections",
		nil,
	)

	// Set and modify the gauge
	activeConnections.Set(100)
	activeConnections.Inc()
	activeConnections.Dec()

	fmt.Printf("Active connections: %.0f\n", activeConnections.Value())

	// Output:
	// Request count: 6
	// Active connections: 100
}

// Example_labels demonstrates using labels with metrics.
func Example_labels() {
	// Create a counter vector with variable labels
	requestCounter := metrics.NewCounterVec(
		"http_requests_total",
		"Total number of HTTP requests",
		metrics.Labels{"service": "api"}, // constant labels
		[]string{"method", "path"},       // variable label names
	)

	// Use the counter with specific labels
	requestCounter.WithLabels(metrics.Labels{"method": "GET", "path": "/"}).Inc()
	requestCounter.WithLabels(metrics.Labels{"method": "POST", "path": "/users"}).Add(3)
	requestCounter.WithLabels(metrics.Labels{"method": "GET", "path": "/"}).Inc()

	// Check values
	getRoot := requestCounter.WithLabels(metrics.Labels{"method": "GET", "path": "/"})
	postUsers := requestCounter.WithLabels(metrics.Labels{"method": "POST", "path": "/users"})

	fmt.Printf("GET /: %.0f\n", getRoot.Value())
	fmt.Printf("POST /users: %.0f\n", postUsers.Value())

	// Output:
	// GET /: 2
	// POST /users: 3
}

// Example_histogram demonstrates histogram usage.
func Example_histogram() {
	// Create a histogram with custom buckets
	requestDuration := metrics.NewHistogram(metrics.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
	})

	// Observe some values
	requestDuration.Observe(0.005)
	requestDuration.Observe(0.02)
	requestDuration.Observe(0.08)
	requestDuration.Observe(0.3)
	requestDuration.Observe(2.5)

	fmt.Println("Histogram recorded 5 observations")

	// Output:
	// Histogram recorded 5 observations
}

// Example_timer demonstrates the Timer helper.
func Example_timer() {
	// Create a histogram for timing
	duration := metrics.NewHistogram(metrics.HistogramOpts{
		Name:    "operation_duration_seconds",
		Help:    "Duration of operations in seconds",
		Buckets: metrics.DefaultBuckets,
	})

	// Time an operation
	timer := metrics.NewTimer(duration.Observe)
	time.Sleep(10 * time.Millisecond) // Simulate work
	elapsed := timer.ObserveDuration()

	fmt.Printf("Operation took: %v\n", elapsed >= 10*time.Millisecond)

	// Output:
	// Operation took: true
}

// Example_registry demonstrates the registry and HTTP handler.
func Example_registry() {
	// Create a new registry
	registry := metrics.NewRegistry()

	// Create and register metrics
	requests := metrics.NewCounter("myapp_requests_total", "Total requests", nil)
	connections := metrics.NewGauge("myapp_connections", "Active connections", nil)

	registry.MustRegister(requests)
	registry.MustRegister(connections)

	// Update metrics
	requests.Add(42)
	connections.Set(10)

	// Create HTTP handler and test it
	handler := registry.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Check response
	fmt.Printf("Status: %d\n", w.Code)
	fmt.Printf("Content-Type: %s\n", w.Header().Get("Content-Type"))

	// Output:
	// Status: 200
	// Content-Type: text/plain; version=0.0.4; charset=utf-8
}

// Example_httpInstrumentation demonstrates HTTP handler instrumentation.
func Example_httpInstrumentation() {
	// Create metrics for HTTP instrumentation
	requestDuration := metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: metrics.DefaultBuckets,
		},
		[]string{"handler", "method"},
	)

	requestCount := metrics.NewCounterVec(
		"http_requests_total",
		"Total HTTP requests",
		nil,
		[]string{"handler", "method"},
	)

	// Create a simple handler
	myHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	})

	// Wrap it with instrumentation
	instrumentedHandler := metrics.InstrumentHandler(
		"hello",
		myHandler,
		requestDuration,
		requestCount,
	)

	// Make a test request
	req := httptest.NewRequest("GET", "/hello", nil)
	w := httptest.NewRecorder()
	instrumentedHandler.ServeHTTP(w, req)

	// Check that metrics were recorded
	labels := metrics.Labels{"handler": "hello", "method": "GET"}
	count := requestCount.WithLabels(labels).Value()

	fmt.Printf("Request count: %.0f\n", count)

	// Output:
	// Request count: 1
}

// Example_buildInfo demonstrates build info gauge.
func Example_buildInfo() {
	info := metrics.BuildInfo{
		Version:   "1.2.3",
		Revision:  "abc123def",
		Branch:    "main",
		BuildDate: "2024-01-15T10:00:00Z",
		GoVersion: "go1.21.5",
	}

	gauge := metrics.NewBuildInfoGauge("kingdom", info)

	fmt.Printf("Build info gauge value: %.0f\n", gauge.Value())

	// Output:
	// Build info gauge value: 1
}

// Example_prometheusFormat demonstrates the Prometheus exposition format output.
func Example_prometheusFormat() {
	registry := metrics.NewRegistry()

	counter := metrics.NewCounter(
		"example_counter",
		"An example counter",
		metrics.Labels{"env": "test"},
	)
	counter.Add(42)
	registry.MustRegister(counter)

	gauge := metrics.NewGauge(
		"example_gauge",
		"An example gauge",
		nil,
	)
	gauge.Set(3.14)
	registry.MustRegister(gauge)

	// Write to stdout
	registry.Gather(os.Stdout)

	// Output:
	// # HELP example_counter An example counter
	// # TYPE example_counter counter
	// example_counter{env="test"} 42
	// # HELP example_gauge An example gauge
	// # TYPE example_gauge gauge
	// example_gauge 3.14
}

// Example_customBuckets demonstrates creating custom histogram buckets.
func Example_customBuckets() {
	// Linear buckets: start=0, width=10, count=5 -> [0, 10, 20, 30, 40]
	linear := metrics.LinearBuckets(0, 10, 5)
	fmt.Printf("Linear buckets: %v\n", linear)

	// Exponential buckets: start=1, factor=2, count=5 -> [1, 2, 4, 8, 16]
	exponential := metrics.ExponentialBuckets(1, 2, 5)
	fmt.Printf("Exponential buckets: %v\n", exponential)

	// Output:
	// Linear buckets: [0 10 20 30 40]
	// Exponential buckets: [1 2 4 8 16]
}
