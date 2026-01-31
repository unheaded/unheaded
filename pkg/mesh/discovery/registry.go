package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

// RegistryClient is a client for a service registry.
type RegistryClient struct {
	baseURL    string
	httpClient *http.Client
	token      string

	mu       sync.RWMutex
	cache    map[string]*registryCache
	watchers map[string][]chan *Service
	done     chan struct{}
}

type registryCache struct {
	service   *Service
	version   string
	expiresAt time.Time
}

// RegistryConfig configures the registry client.
type RegistryConfig struct {
	// BaseURL of the service registry
	BaseURL string
	// Token for authentication
	Token string
	// Timeout for HTTP requests
	Timeout time.Duration
	// RefreshInterval for polling
	RefreshInterval time.Duration
}

// NewRegistryClient creates a new registry client.
func NewRegistryClient(config *RegistryConfig) *RegistryClient {
	client := &http.Client{
		Timeout: config.Timeout,
	}
	if config.Timeout == 0 {
		client.Timeout = 10 * time.Second
	}

	rc := &RegistryClient{
		baseURL:    config.BaseURL,
		httpClient: client,
		token:      config.Token,
		cache:      make(map[string]*registryCache),
		watchers:   make(map[string][]chan *Service),
		done:       make(chan struct{}),
	}

	// Start background refresh
	refreshInterval := config.RefreshInterval
	if refreshInterval == 0 {
		refreshInterval = 30 * time.Second
	}
	go rc.refreshLoop(refreshInterval)

	return rc
}

// Discover fetches service information from the registry.
func (rc *RegistryClient) Discover(ctx context.Context, service, namespace string) (*Service, error) {
	key := namespace + "/" + service

	// Check cache
	rc.mu.RLock()
	if cached, ok := rc.cache[key]; ok && time.Now().Before(cached.expiresAt) {
		rc.mu.RUnlock()
		return cached.service, nil
	}
	rc.mu.RUnlock()

	// Fetch from registry
	svc, version, err := rc.fetchService(ctx, service, namespace)
	if err != nil {
		return nil, err
	}

	// Update cache
	rc.mu.Lock()
	rc.cache[key] = &registryCache{
		service:   svc,
		version:   version,
		expiresAt: time.Now().Add(30 * time.Second),
	}
	rc.mu.Unlock()

	return svc, nil
}

// Watch returns a channel for service updates.
func (rc *RegistryClient) Watch(ctx context.Context, service, namespace string) (<-chan *Service, error) {
	key := namespace + "/" + service
	ch := make(chan *Service, 10)

	rc.mu.Lock()
	rc.watchers[key] = append(rc.watchers[key], ch)
	rc.mu.Unlock()

	// Send current state
	go func() {
		svc, err := rc.Discover(ctx, service, namespace)
		if err == nil {
			select {
			case ch <- svc:
			case <-ctx.Done():
			}
		}
	}()

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		rc.mu.Lock()
		watchers := rc.watchers[key]
		for i, w := range watchers {
			if w == ch {
				rc.watchers[key] = append(watchers[:i], watchers[i+1:]...)
				close(ch)
				break
			}
		}
		rc.mu.Unlock()
	}()

	return ch, nil
}

// Close shuts down the registry client.
func (rc *RegistryClient) Close() error {
	close(rc.done)
	return nil
}

// Register registers a service with the registry.
func (rc *RegistryClient) Register(ctx context.Context, svc *Service) error {
	url := rc.baseURL + "/v1/services/" + svc.Namespace + "/" + svc.Name

	body, err := json.Marshal(svc)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	rc.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.New("registry error: " + resp.Status)
	}

	return nil
}

// Deregister removes a service from the registry.
func (rc *RegistryClient) Deregister(ctx context.Context, service, namespace string) error {
	url := rc.baseURL + "/v1/services/" + namespace + "/" + service

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	rc.setHeaders(req)

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return errors.New("registry error: " + resp.Status)
	}

	return nil
}

// Heartbeat sends a heartbeat for a service.
func (rc *RegistryClient) Heartbeat(ctx context.Context, service, namespace string) error {
	url := rc.baseURL + "/v1/services/" + namespace + "/" + service + "/heartbeat"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	rc.setHeaders(req)

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.New("registry error: " + resp.Status)
	}

	return nil
}

func (rc *RegistryClient) fetchService(ctx context.Context, service, namespace string) (*Service, string, error) {
	url := rc.baseURL + "/v1/services/" + namespace + "/" + service

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	rc.setHeaders(req)

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrServiceNotFound
	}

	if resp.StatusCode >= 400 {
		return nil, "", errors.New("registry error: " + resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	var svc Service
	if err := json.Unmarshal(body, &svc); err != nil {
		return nil, "", err
	}

	version := resp.Header.Get("X-Version")
	return &svc, version, nil
}

func (rc *RegistryClient) setHeaders(req *http.Request) {
	if rc.token != "" {
		req.Header.Set("Authorization", "Bearer "+rc.token)
	}
	req.Header.Set("Accept", "application/json")
}

func (rc *RegistryClient) refreshLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-rc.done:
			return
		case <-ticker.C:
			rc.refreshAll()
		}
	}
}

func (rc *RegistryClient) refreshAll() {
	rc.mu.RLock()
	keys := make([]string, 0, len(rc.watchers))
	for key := range rc.watchers {
		keys = append(keys, key)
	}
	versions := make(map[string]string)
	for key, cached := range rc.cache {
		versions[key] = cached.version
	}
	rc.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, key := range keys {
		parts := splitKey(key)
		if len(parts) != 2 {
			continue
		}
		namespace, service := parts[0], parts[1]

		svc, version, err := rc.fetchService(ctx, service, namespace)
		if err != nil {
			continue
		}

		// Check if changed
		if versions[key] == version {
			continue
		}

		// Update cache
		rc.mu.Lock()
		rc.cache[key] = &registryCache{
			service:   svc,
			version:   version,
			expiresAt: time.Now().Add(30 * time.Second),
		}
		watchers := rc.watchers[key]
		rc.mu.Unlock()

		// Notify watchers
		for _, ch := range watchers {
			select {
			case ch <- svc:
			default:
			}
		}
	}
}

func splitKey(key string) []string {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return []string{key}
}

// ServiceInstance represents a single service instance for registration.
type ServiceInstance struct {
	ID        string            `json:"id"`
	Service   string            `json:"service"`
	Namespace string            `json:"namespace"`
	Address   string            `json:"address"`
	Port      int               `json:"port"`
	Tags      []string          `json:"tags,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	Health    HealthStatus      `json:"health"`
}

// HealthStatus represents the health of a service instance.
type HealthStatus string

const (
	HealthPassing  HealthStatus = "passing"
	HealthWarning  HealthStatus = "warning"
	HealthCritical HealthStatus = "critical"
)

// InstanceRegistrar manages instance-level registration.
type InstanceRegistrar struct {
	client   *RegistryClient
	instance *ServiceInstance
	interval time.Duration
	done     chan struct{}
}

// NewInstanceRegistrar creates a new instance registrar.
func NewInstanceRegistrar(client *RegistryClient, instance *ServiceInstance, heartbeatInterval time.Duration) *InstanceRegistrar {
	return &InstanceRegistrar{
		client:   client,
		instance: instance,
		interval: heartbeatInterval,
		done:     make(chan struct{}),
	}
}

// Start begins the registration and heartbeat loop.
func (ir *InstanceRegistrar) Start(ctx context.Context) error {
	// Initial registration
	if err := ir.register(ctx); err != nil {
		return err
	}

	// Start heartbeat loop
	go ir.heartbeatLoop()

	return nil
}

// Stop stops the registrar and deregisters the instance.
func (ir *InstanceRegistrar) Stop(ctx context.Context) error {
	close(ir.done)
	return ir.deregister(ctx)
}

func (ir *InstanceRegistrar) register(ctx context.Context) error {
	url := ir.client.baseURL + "/v1/instances/" + ir.instance.ID

	body, err := json.Marshal(ir.instance)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	ir.client.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ir.client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.New("registration failed: " + resp.Status)
	}

	return nil
}

func (ir *InstanceRegistrar) deregister(ctx context.Context) error {
	url := ir.client.baseURL + "/v1/instances/" + ir.instance.ID

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	ir.client.setHeaders(req)

	resp, err := ir.client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (ir *InstanceRegistrar) heartbeatLoop() {
	ticker := time.NewTicker(ir.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ir.done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			ir.heartbeat(ctx)
			cancel()
		}
	}
}

func (ir *InstanceRegistrar) heartbeat(ctx context.Context) error {
	url := ir.client.baseURL + "/v1/instances/" + ir.instance.ID + "/heartbeat"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	ir.client.setHeaders(req)

	resp, err := ir.client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// UpdateHealth updates the health status of the instance.
func (ir *InstanceRegistrar) UpdateHealth(ctx context.Context, status HealthStatus) error {
	ir.instance.Health = status

	url := ir.client.baseURL + "/v1/instances/" + ir.instance.ID + "/health"

	body, _ := json.Marshal(map[string]string{"status": string(status)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	ir.client.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ir.client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
