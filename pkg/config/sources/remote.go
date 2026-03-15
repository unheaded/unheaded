// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// RemoteSource loads configuration from a remote HTTP endpoint.
type RemoteSource struct {
	url            string
	refreshInterval time.Duration
	client         *http.Client
	mu             sync.RWMutex
	cachedData     map[string]any
	lastFetch      time.Time
	headers        map[string]string
	timeout        time.Duration
	retries        int
	retryDelay     time.Duration
}

// RemoteOption is a functional option for RemoteSource.
type RemoteOption func(*RemoteSource)

// NewRemoteSource creates a new remote configuration source.
func NewRemoteSource(url string, refreshInterval time.Duration, opts ...RemoteOption) *RemoteSource {
	rs := &RemoteSource{
		url:             url,
		refreshInterval: refreshInterval,
		client:          &http.Client{Timeout: 10 * time.Second},
		headers:         make(map[string]string),
		timeout:         10 * time.Second,
		retries:         3,
		retryDelay:      time.Second,
	}

	for _, opt := range opts {
		opt(rs)
	}

	return rs
}

// WithHeaders sets custom headers for the remote request.
func WithHeaders(headers map[string]string) RemoteOption {
	return func(rs *RemoteSource) {
		for k, v := range headers {
			rs.headers[k] = v
		}
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) RemoteOption {
	return func(rs *RemoteSource) {
		rs.timeout = timeout
		rs.client.Timeout = timeout
	}
}

// WithRetries sets the number of retry attempts.
func WithRetries(retries int, delay time.Duration) RemoteOption {
	return func(rs *RemoteSource) {
		rs.retries = retries
		rs.retryDelay = delay
	}
}

// WithBasicAuth sets basic authentication.
func WithBasicAuth(username, password string) RemoteOption {
	return func(rs *RemoteSource) {
		// Basic auth will be set in the request
		rs.headers["Authorization"] = "Basic " + basicAuth(username, password)
	}
}

// basicAuth encodes basic authentication credentials.
func basicAuth(username, password string) string {
	auth := username + ":" + password
	// Simple base64 encoding without importing encoding/base64
	return base64Encode([]byte(auth))
}

// base64Encode performs base64 encoding.
func base64Encode(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte

	for i := 0; i < len(data); i += 3 {
		var n uint32
		remaining := len(data) - i

		if remaining >= 3 {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result = append(result,
				alphabet[n>>18&0x3F],
				alphabet[n>>12&0x3F],
				alphabet[n>>6&0x3F],
				alphabet[n&0x3F],
			)
		} else if remaining == 2 {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result = append(result,
				alphabet[n>>18&0x3F],
				alphabet[n>>12&0x3F],
				alphabet[n>>6&0x3F],
				'=',
			)
		} else {
			n = uint32(data[i]) << 16
			result = append(result,
				alphabet[n>>18&0x3F],
				alphabet[n>>12&0x3F],
				'=',
				'=',
			)
		}
	}

	return string(result)
}

// WithBearerToken sets bearer token authentication.
func WithBearerToken(token string) RemoteOption {
	return func(rs *RemoteSource) {
		rs.headers["Authorization"] = "Bearer " + token
	}
}

// Load loads configuration from the remote endpoint.
func (rs *RemoteSource) Load() (map[string]any, error) {
	rs.mu.RLock()
	if rs.cachedData != nil && time.Since(rs.lastFetch) < rs.refreshInterval {
		data := rs.cachedData
		rs.mu.RUnlock()
		return data, nil
	}
	rs.mu.RUnlock()

	return rs.fetch()
}

// fetch fetches configuration from the remote endpoint with retries.
func (rs *RemoteSource) fetch() (map[string]any, error) {
	var lastErr error

	for attempt := 0; attempt <= rs.retries; attempt++ {
		if attempt > 0 {
			time.Sleep(rs.retryDelay * time.Duration(attempt))
		}

		data, err := rs.doFetch()
		if err == nil {
			rs.mu.Lock()
			rs.cachedData = data
			rs.lastFetch = time.Now()
			rs.mu.Unlock()
			return data, nil
		}

		lastErr = err
	}

	// Return cached data if available
	rs.mu.RLock()
	if rs.cachedData != nil {
		data := rs.cachedData
		rs.mu.RUnlock()
		return data, nil
	}
	rs.mu.RUnlock()

	return nil, fmt.Errorf("failed to fetch remote config after %d attempts: %w", rs.retries+1, lastErr)
}

// doFetch performs a single fetch attempt.
func (rs *RemoteSource) doFetch() (map[string]any, error) {
	req, err := http.NewRequest("GET", rs.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	for k, v := range rs.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := rs.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// SECURITY: Enforce body size limit (10MB) to prevent denial-of-service
	// from a malicious or misconfigured remote config source.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return data, nil
}

// Name returns the source name.
func (rs *RemoteSource) Name() string {
	return "remote:" + rs.url
}

// Priority returns the source priority.
func (rs *RemoteSource) Priority() int {
	return 75 // Between env and file
}

// Refresh forces a refresh of the cached data.
func (rs *RemoteSource) Refresh() error {
	_, err := rs.fetch()
	return err
}

// LastFetch returns the time of the last successful fetch.
func (rs *RemoteSource) LastFetch() time.Time {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.lastFetch
}

// SetURL updates the remote URL.
func (rs *RemoteSource) SetURL(url string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.url = url
	rs.cachedData = nil
}

// RemoteWatcher watches a remote configuration source for changes.
type RemoteWatcher struct {
	source   *RemoteSource
	interval time.Duration
	stopCh   chan struct{}
	events   chan map[string]any
	running  bool
	mu       sync.Mutex
}

// NewRemoteWatcher creates a new remote configuration watcher.
func NewRemoteWatcher(source *RemoteSource, interval time.Duration) *RemoteWatcher {
	return &RemoteWatcher{
		source:   source,
		interval: interval,
		stopCh:   make(chan struct{}),
		events:   make(chan map[string]any, 1),
	}
}

// Start starts watching for configuration changes.
func (rw *RemoteWatcher) Start() error {
	rw.mu.Lock()
	if rw.running {
		rw.mu.Unlock()
		return nil
	}
	rw.running = true
	rw.mu.Unlock()

	go rw.watch()
	return nil
}

// watch polls the remote source for changes.
func (rw *RemoteWatcher) watch() {
	ticker := time.NewTicker(rw.interval)
	defer ticker.Stop()

	var lastHash string

	for {
		select {
		case <-rw.stopCh:
			return
		case <-ticker.C:
			data, err := rw.source.Load()
			if err != nil {
				continue
			}

			hash := hashData(data)
			if hash != lastHash {
				lastHash = hash
				select {
				case rw.events <- data:
				default:
					// Channel full, skip update
				}
			}
		}
	}
}

// hashData creates a simple hash of the data for comparison.
func hashData(data map[string]any) string {
	bytes, _ := json.Marshal(data)
	// Simple hash
	var hash uint64
	for _, b := range bytes {
		hash = hash*31 + uint64(b)
	}
	return fmt.Sprintf("%x", hash)
}

// Events returns the channel of configuration change events.
func (rw *RemoteWatcher) Events() <-chan map[string]any {
	return rw.events
}

// Stop stops watching for configuration changes.
func (rw *RemoteWatcher) Stop() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if !rw.running {
		return
	}

	close(rw.stopCh)
	rw.running = false
}

// ConsulSource provides configuration from Consul KV store.
type ConsulSource struct {
	*RemoteSource
	prefix string
}

// NewConsulSource creates a new Consul configuration source.
func NewConsulSource(addr, prefix string, refreshInterval time.Duration) *ConsulSource {
	url := fmt.Sprintf("%s/v1/kv/%s?recurse=true", addr, prefix)
	return &ConsulSource{
		RemoteSource: NewRemoteSource(url, refreshInterval),
		prefix:       prefix,
	}
}

// Load loads configuration from Consul, converting KV pairs to nested map.
func (cs *ConsulSource) Load() (map[string]any, error) {
	data, err := cs.RemoteSource.Load()
	if err != nil {
		return nil, err
	}

	// Consul returns an array of KV pairs
	// Convert to nested map structure
	return cs.convertConsulData(data), nil
}

// convertConsulData converts Consul KV response to nested map.
func (cs *ConsulSource) convertConsulData(data map[string]any) map[string]any {
	// Placeholder for Consul-specific conversion
	// In production, this would handle the Consul API response format
	return data
}

// EtcdSource provides configuration from etcd.
type EtcdSource struct {
	*RemoteSource
	prefix string
}

// NewEtcdSource creates a new etcd configuration source.
func NewEtcdSource(endpoints []string, prefix string, refreshInterval time.Duration) *EtcdSource {
	// For simplicity, use first endpoint
	url := fmt.Sprintf("%s/v3/kv/range", endpoints[0])
	return &EtcdSource{
		RemoteSource: NewRemoteSource(url, refreshInterval),
		prefix:       prefix,
	}
}

// HTTPPollingSource provides configuration via HTTP polling with configurable format.
type HTTPPollingSource struct {
	url      string
	format   string
	interval time.Duration
	client   *http.Client
	mu       sync.RWMutex
	data     map[string]any
	stopCh   chan struct{}
	events   chan map[string]any
}

// NewHTTPPollingSource creates a new HTTP polling configuration source.
func NewHTTPPollingSource(url string, format string, interval time.Duration) *HTTPPollingSource {
	return &HTTPPollingSource{
		url:      url,
		format:   format,
		interval: interval,
		client:   &http.Client{Timeout: 10 * time.Second},
		stopCh:   make(chan struct{}),
		events:   make(chan map[string]any, 1),
	}
}

// Load loads configuration from the HTTP endpoint.
func (h *HTTPPollingSource) Load() (map[string]any, error) {
	h.mu.RLock()
	if h.data != nil {
		data := h.data
		h.mu.RUnlock()
		return data, nil
	}
	h.mu.RUnlock()

	return h.fetch()
}

// fetch fetches configuration from the HTTP endpoint.
func (h *HTTPPollingSource) fetch() (map[string]any, error) {
	resp, err := h.client.Get(h.url)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	// SECURITY: Enforce body size limit (10MB) to prevent denial-of-service
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var data map[string]any

	switch h.format {
	case "json":
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("json parse: %w", err)
		}
	case "toml":
		data, err = parseTOML(body)
		if err != nil {
			return nil, fmt.Errorf("toml parse: %w", err)
		}
	case "yaml":
		data, err = parseYAML(body)
		if err != nil {
			return nil, fmt.Errorf("yaml parse: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported format: %s", h.format)
	}

	h.mu.Lock()
	h.data = data
	h.mu.Unlock()

	return data, nil
}

// Name returns the source name.
func (h *HTTPPollingSource) Name() string {
	return "http:" + h.url
}

// Priority returns the source priority.
func (h *HTTPPollingSource) Priority() int {
	return 80
}

// StartPolling starts polling for configuration changes.
func (h *HTTPPollingSource) StartPolling() {
	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		var lastHash string

		for {
			select {
			case <-h.stopCh:
				return
			case <-ticker.C:
				data, err := h.fetch()
				if err != nil {
					continue
				}

				hash := hashData(data)
				if hash != lastHash {
					lastHash = hash
					select {
					case h.events <- data:
					default:
					}
				}
			}
		}
	}()
}

// StopPolling stops polling.
func (h *HTTPPollingSource) StopPolling() {
	close(h.stopCh)
}

// Events returns the configuration change events channel.
func (h *HTTPPollingSource) Events() <-chan map[string]any {
	return h.events
}
