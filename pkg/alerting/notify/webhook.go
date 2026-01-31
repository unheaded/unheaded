// Package notify provides webhook notification channel implementation.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/unheaded/pkg/alerting"
)

// WebhookConfig holds webhook channel configuration.
type WebhookConfig struct {
	ChannelConfig
	URL            string            `json:"url"`
	HTTPMethod     string            `json:"http_method"`
	Headers        map[string]string `json:"headers"`
	BasicAuth      *BasicAuth        `json:"basic_auth,omitempty"`
	TLSInsecure    bool              `json:"tls_insecure"`
	Timeout        time.Duration     `json:"timeout"`
	MaxRetries     int               `json:"max_retries"`
	RetryInterval  time.Duration     `json:"retry_interval"`
}

// BasicAuth holds basic authentication credentials.
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// WebhookPayload represents the JSON payload sent to webhooks.
type WebhookPayload struct {
	Version      string            `json:"version"`
	GroupKey     string            `json:"group_key"`
	Status       string            `json:"status"`
	Receiver     string            `json:"receiver"`
	GroupLabels  map[string]string `json:"group_labels"`
	CommonLabels map[string]string `json:"common_labels"`
	Alerts       []WebhookAlert    `json:"alerts"`
	ExternalURL  string            `json:"external_url"`
}

// WebhookAlert represents an alert in the webhook payload.
type WebhookAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"starts_at"`
	EndsAt       time.Time         `json:"ends_at"`
	Fingerprint  string            `json:"fingerprint"`
	GeneratorURL string            `json:"generator_url,omitempty"`
}

// WebhookChannel implements the NotificationChannel interface for webhooks.
type WebhookChannel struct {
	config WebhookConfig
	client *http.Client
}

// NewWebhookChannel creates a new webhook notification channel.
func NewWebhookChannel(config WebhookConfig) *WebhookChannel {
	if config.HTTPMethod == "" {
		config.HTTPMethod = http.MethodPost
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryInterval == 0 {
		config.RetryInterval = 1 * time.Second
	}

	return &WebhookChannel{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Name returns the channel name.
func (w *WebhookChannel) Name() string {
	return w.config.Name
}

// Send sends alerts to the webhook endpoint.
func (w *WebhookChannel) Send(ctx context.Context, alerts []*alerting.Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	payload := w.buildPayload(alerts)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal payload: %v", alerting.ErrNotificationError, err)
	}

	var lastErr error
	for attempt := 0; attempt <= w.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.config.RetryInterval):
			}
		}

		err := w.doRequest(ctx, data)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("%w: %v", alerting.ErrNotificationError, lastErr)
}

// doRequest performs the HTTP request.
func (w *WebhookChannel) doRequest(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, w.config.HTTPMethod, w.config.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "UnheadedKingdom-Alerting/1.0")

	for k, v := range w.config.Headers {
		req.Header.Set(k, v)
	}

	if w.config.BasicAuth != nil {
		req.SetBasicAuth(w.config.BasicAuth.Username, w.config.BasicAuth.Password)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// buildPayload builds the webhook payload from alerts.
func (w *WebhookChannel) buildPayload(alerts []*alerting.Alert) WebhookPayload {
	webhookAlerts := make([]WebhookAlert, len(alerts))
	commonLabels := make(map[string]string)
	groupLabels := make(map[string]string)

	// Determine status
	status := "resolved"
	for i, alert := range alerts {
		if alert.State == alerting.AlertStateFiring {
			status = "firing"
		}

		webhookAlerts[i] = WebhookAlert{
			Status:       string(alert.State),
			Labels:       alert.Labels,
			Annotations:  alert.Annotations,
			StartsAt:     alert.StartsAt,
			EndsAt:       alert.EndsAt,
			Fingerprint:  alert.Fingerprint,
			GeneratorURL: alert.GeneratorURL,
		}

		// Find common labels
		if i == 0 {
			for k, v := range alert.Labels {
				commonLabels[k] = v
			}
		} else {
			for k, v := range commonLabels {
				if alert.Labels[k] != v {
					delete(commonLabels, k)
				}
			}
		}
	}

	// Group labels are typically the common labels
	for k, v := range commonLabels {
		groupLabels[k] = v
	}

	return WebhookPayload{
		Version:      "4",
		GroupKey:     alerts[0].Fingerprint,
		Status:       status,
		Receiver:     w.config.Name,
		GroupLabels:  groupLabels,
		CommonLabels: commonLabels,
		Alerts:       webhookAlerts,
	}
}

// Test tests the webhook connection.
func (w *WebhookChannel) Test(ctx context.Context) error {
	testAlert := &alerting.Alert{
		ID:          "test-alert",
		Name:        "Test Alert",
		Description: "This is a test alert",
		State:       alerting.AlertStateFiring,
		Severity:    alerting.SeverityInfo,
		Labels:      map[string]string{"alertname": "TestAlert", "test": "true"},
		Annotations: map[string]string{"summary": "Test notification"},
		StartsAt:    time.Now(),
		Fingerprint: "test-fingerprint",
	}

	return w.Send(ctx, []*alerting.Alert{testAlert})
}

// WebhookChannelBuilder provides a fluent interface for building webhook channels.
type WebhookChannelBuilder struct {
	config WebhookConfig
}

// NewWebhookChannelBuilder creates a new webhook channel builder.
func NewWebhookChannelBuilder(name, url string) *WebhookChannelBuilder {
	return &WebhookChannelBuilder{
		config: WebhookConfig{
			ChannelConfig: ChannelConfig{
				Name:         name,
				SendResolved: true,
			},
			URL:        url,
			HTTPMethod: http.MethodPost,
			Headers:    make(map[string]string),
		},
	}
}

// WithMethod sets the HTTP method.
func (b *WebhookChannelBuilder) WithMethod(method string) *WebhookChannelBuilder {
	b.config.HTTPMethod = method
	return b
}

// WithHeader adds a custom header.
func (b *WebhookChannelBuilder) WithHeader(key, value string) *WebhookChannelBuilder {
	b.config.Headers[key] = value
	return b
}

// WithBasicAuth sets basic authentication.
func (b *WebhookChannelBuilder) WithBasicAuth(username, password string) *WebhookChannelBuilder {
	b.config.BasicAuth = &BasicAuth{
		Username: username,
		Password: password,
	}
	return b
}

// WithTimeout sets the request timeout.
func (b *WebhookChannelBuilder) WithTimeout(timeout time.Duration) *WebhookChannelBuilder {
	b.config.Timeout = timeout
	return b
}

// WithRetries sets the retry configuration.
func (b *WebhookChannelBuilder) WithRetries(maxRetries int, interval time.Duration) *WebhookChannelBuilder {
	b.config.MaxRetries = maxRetries
	b.config.RetryInterval = interval
	return b
}

// Build creates the webhook channel.
func (b *WebhookChannelBuilder) Build() *WebhookChannel {
	return NewWebhookChannel(b.config)
}
