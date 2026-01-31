// Package proxy provides reverse proxy functionality for the API Gateway.
package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/services/gateway/config"
	"unheaded/services/gateway/middleware"
)

// ReverseProxy handles proxying requests to backend services.
type ReverseProxy struct {
	cfg           *config.RouteConfig
	log           *logger.Logger
	metrics       *metrics.Registry
	loadBalancer  LoadBalancer
	circuitBreaker *CircuitBreaker
	transport     http.RoundTripper
	bufferPool    sync.Pool
}

// NewReverseProxy creates a new reverse proxy.
func NewReverseProxy(
	cfg *config.RouteConfig,
	log *logger.Logger,
	metricsReg *metrics.Registry,
	lb LoadBalancer,
	cb *CircuitBreaker,
) *ReverseProxy {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: cfg.Timeout,
	}

	return &ReverseProxy{
		cfg:            cfg,
		log:            log,
		metrics:        metricsReg,
		loadBalancer:   lb,
		circuitBreaker: cb,
		transport:      transport,
		bufferPool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 32*1024)
				return &buf
			},
		},
	}
}

// ServeHTTP handles the proxy request.
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := middleware.GetTraceID(ctx)

	// Get backend from load balancer
	backend, err := p.loadBalancer.Next()
	if err != nil {
		p.log.Error("No available backends",
			"route", p.cfg.Name,
			"trace_id", traceID,
			"error", err.Error(),
		)
		writeProxyError(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Check circuit breaker
	if p.circuitBreaker != nil && !p.circuitBreaker.Allow() {
		p.log.Warn("Circuit breaker open",
			"route", p.cfg.Name,
			"backend", backend,
			"trace_id", traceID,
		)
		writeProxyError(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	// Proxy the request with retry logic
	var lastErr error
	for attempt := 0; attempt <= p.cfg.RetryCount; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(p.cfg.RetryDelay)

			// Get a potentially different backend for retry
			backend, err = p.loadBalancer.Next()
			if err != nil {
				lastErr = err
				continue
			}
		}

		err = p.proxyRequest(w, r, backend)
		if err == nil {
			// Success - record to circuit breaker
			if p.circuitBreaker != nil {
				p.circuitBreaker.RecordSuccess()
			}
			return
		}

		lastErr = err
		p.log.Warn("Proxy request failed",
			"route", p.cfg.Name,
			"backend", backend,
			"attempt", attempt+1,
			"trace_id", traceID,
			"error", err.Error(),
		)

		// Record failure to circuit breaker
		if p.circuitBreaker != nil {
			p.circuitBreaker.RecordFailure()
		}

		// Mark backend as unhealthy
		p.loadBalancer.MarkUnhealthy(backend)
	}

	// All retries exhausted
	p.log.Error("All retries exhausted",
		"route", p.cfg.Name,
		"trace_id", traceID,
		"error", lastErr.Error(),
	)
	writeProxyError(w, "Backend service error", http.StatusBadGateway)
}

// proxyRequest performs the actual proxy operation.
func (p *ReverseProxy) proxyRequest(w http.ResponseWriter, r *http.Request, backend string) error {
	ctx := r.Context()
	traceID := middleware.GetTraceID(ctx)

	// Build target URL
	targetURL, err := p.buildTargetURL(r, backend)
	if err != nil {
		return err
	}

	// Create outgoing request
	outReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		return err
	}

	// Copy headers
	copyHeaders(outReq.Header, r.Header)

	// Add proxy headers
	outReq.Header.Set("X-Forwarded-For", getClientIP(r))
	outReq.Header.Set("X-Forwarded-Host", r.Host)
	outReq.Header.Set("X-Forwarded-Proto", getScheme(r))
	outReq.Header.Set("X-Real-IP", getClientIP(r))

	// Inject tracing headers
	middleware.InjectHeaders(ctx, outReq.Header)

	// Set timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	outReq = outReq.WithContext(timeoutCtx)

	// Record start time for metrics
	start := time.Now()

	// Make the request
	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Record metrics
	duration := time.Since(start)
	if p.metrics != nil {
		labels := map[string]string{
			"route":   p.cfg.Name,
			"backend": backend,
			"method":  r.Method,
			"status":  statusCodeClass(resp.StatusCode),
		}
		p.metrics.HistogramWithLabels("gateway_backend_request_duration_seconds", labels).
			Observe(duration.Seconds())
		p.metrics.CounterWithLabels("gateway_backend_requests_total", labels).Inc()
	}

	p.log.Debug("Backend request completed",
		"route", p.cfg.Name,
		"backend", backend,
		"status", resp.StatusCode,
		"duration_ms", duration.Milliseconds(),
		"trace_id", traceID,
	)

	// Copy response headers
	copyHeaders(w.Header(), resp.Header)

	// Set trace ID in response
	w.Header().Set(middleware.TraceIDHeader, traceID)

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	bufp := p.bufferPool.Get().(*[]byte)
	defer p.bufferPool.Put(bufp)
	_, err = io.CopyBuffer(w, resp.Body, *bufp)

	return err
}

// buildTargetURL builds the target URL for the backend.
func (p *ReverseProxy) buildTargetURL(r *http.Request, backend string) (*url.URL, error) {
	targetURL, err := url.Parse(backend)
	if err != nil {
		return nil, err
	}

	// Determine the path
	path := r.URL.Path
	if p.cfg.StripPrefix && strings.HasPrefix(path, p.cfg.PathPrefix) {
		path = strings.TrimPrefix(path, p.cfg.PathPrefix)
		if path == "" || path[0] != '/' {
			path = "/" + path
		}
	}

	targetURL.Path = singleJoiningSlash(targetURL.Path, path)
	targetURL.RawQuery = r.URL.RawQuery

	return targetURL, nil
}

// singleJoiningSlash joins two paths with a single slash.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

// copyHeaders copies headers from src to dst.
func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		// Skip hop-by-hop headers
		if isHopByHopHeader(k) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// isHopByHopHeader checks if a header is a hop-by-hop header.
func isHopByHopHeader(header string) bool {
	hopByHopHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}
	return hopByHopHeaders[header]
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// getScheme returns the request scheme.
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}

// statusCodeClass returns the status code class.
func statusCodeClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500:
		return "5xx"
	default:
		return "unknown"
	}
}

// writeProxyError writes a proxy error response.
func writeProxyError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"proxy_error","message":"` + message + `"}`))
}

// Close closes the proxy and releases resources.
func (p *ReverseProxy) Close() error {
	if transport, ok := p.transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}
