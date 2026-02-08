package httputil

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ServiceMetrics holds Prometheus metrics for an HTTP service.
type ServiceMetrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
}

// NewServiceMetrics creates a standard set of Prometheus metrics for a service.
func NewServiceMetrics(serviceName string) *ServiceMetrics {
	return &ServiceMetrics{
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "unheaded_http_requests_total",
				Help:        "Total HTTP requests",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"method", "path", "status"},
		),
		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "unheaded_http_request_duration_seconds",
				Help:        "HTTP request latency",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
			},
			[]string{"method", "path"},
		),
	}
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// InstrumentHandler wraps an http.HandlerFunc with Prometheus metrics recording.
// It captures the actual response status code for accurate metrics.
func InstrumentHandler(metrics *ServiceMetrics, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		handler(rec, r)

		duration := time.Since(start).Seconds()
		status := http.StatusText(rec.status)
		if status == "" {
			status = "unknown"
		}

		metrics.RequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
		metrics.RequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
	}
}
