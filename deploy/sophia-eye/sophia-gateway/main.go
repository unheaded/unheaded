// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// SophiaGateway integrates the AI stack with Unheaded's infrastructure
type SophiaGateway struct {
	protocolAPI string
	wotanAddr   string
	vllmAddr    string
	qwenAddr    string
	qdrantAddr  string
	embeddAddr  string
	serviceName string
	gatewayPort string
	metricsPort string
	logLevel    string

	// Prometheus metrics
	inferenceLatency   prometheus.Histogram
	inferenceErrors    prometheus.Counter
	tokensGenerated    prometheus.Counter
	tokensPrompted     prometheus.Counter
	requestsTotal      prometheus.Counter
	registrationStatus prometheus.Gauge

	// Track registration state for readiness checks
	registered bool

	mu sync.RWMutex
}

// SophiaServiceEntry represents a service in the Sophia dictionary
type SophiaServiceEntry struct {
	Name      string            `json:"name"`
	Endpoint  string            `json:"endpoint"`
	Model     string            `json:"model,omitempty"`
	Type      string            `json:"type"`
	Timestamp int64             `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// InferenceRecord represents an inference request recorded in Anamnesis
type InferenceRecord struct {
	ServiceName     string `json:"service_name"`
	Model           string `json:"model"`
	PromptTokens    int    `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	LatencyMs       float64 `json:"latency_ms"`
	Timestamp       int64  `json:"timestamp"`
	Status          string `json:"status"`
	ErrorMessage    string `json:"error_message,omitempty"`
}

// WotanMetric represents an observability event for Wotan
type WotanMetric struct {
	Timestamp   int64             `json:"timestamp"`
	ServiceName string            `json:"service_name"`
	EventType   string            `json:"event_type"`
	Model       string            `json:"model"`
	Tokens      int               `json:"tokens"`
	LatencyMs   float64           `json:"latency_ms"`
	Metadata    map[string]string `json:"metadata"`
}

// InferenceRequest represents a chat completion request
type InferenceRequest struct {
	Model       string        `json:"model"`
	Messages    []interface{} `json:"messages"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// InferenceResponse represents a chat completion response
type InferenceResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
}

func init() {
	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "sophia_gateway_info",
			Help: "Sophia's Eye gateway information",
		},
		func() float64 { return 1 },
	))
}

func NewSophiaGateway() *SophiaGateway {
	sg := &SophiaGateway{
		protocolAPI: os.Getenv("UNHEADED_PROTOCOL_API"),
		wotanAddr:   os.Getenv("WOTAN_ENDPOINT"),
		vllmAddr:    os.Getenv("VLLM_ENDPOINT"),
		qwenAddr:    os.Getenv("QWEN_ENDPOINT"),
		qdrantAddr:  os.Getenv("QDRANT_ENDPOINT"),
		embeddAddr:  os.Getenv("EMBEDDINGS_ENDPOINT"),
		serviceName: os.Getenv("SOPHIA_SERVICE_NAME"),
		gatewayPort: os.Getenv("SOPHIA_GATEWAY_PORT"),
		metricsPort: os.Getenv("METRICS_PORT"),
		logLevel:    os.Getenv("LOG_LEVEL"),
	}

	if sg.protocolAPI == "" {
		sg.protocolAPI = "http://bare-metal:17100"
	}
	if sg.wotanAddr == "" {
		sg.wotanAddr = "bare-metal:18001"
	}
	if sg.vllmAddr == "" {
		sg.vllmAddr = "http://vllm:8000"
	}
	if sg.qwenAddr == "" {
		sg.qwenAddr = "http://qwen-coder:8000"
	}
	if sg.qdrantAddr == "" {
		sg.qdrantAddr = "http://qdrant:6333"
	}
	if sg.embeddAddr == "" {
		sg.embeddAddr = "http://embeddings:80"
	}
	if sg.serviceName == "" {
		sg.serviceName = "sophia-eye"
	}
	if sg.gatewayPort == "" {
		sg.gatewayPort = "20105"
	}
	if sg.metricsPort == "" {
		sg.metricsPort = "9090"
	}
	if sg.logLevel == "" {
		sg.logLevel = "info"
	}

	// Initialize Prometheus metrics
	sg.inferenceLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "sophia_inference_latency_ms",
			Help:    "Inference request latency in milliseconds",
			Buckets: []float64{10, 50, 100, 250, 500, 1000, 2500, 5000},
		},
	)
	sg.inferenceErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sophia_inference_errors_total",
			Help: "Total number of inference errors",
		},
	)
	sg.tokensGenerated = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sophia_tokens_generated_total",
			Help: "Total tokens generated by inference engine",
		},
	)
	sg.tokensPrompted = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sophia_tokens_prompted_total",
			Help: "Total prompt tokens received",
		},
	)
	sg.requestsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sophia_requests_total",
			Help: "Total inference requests",
		},
	)
	sg.registrationStatus = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "sophia_registration_status",
			Help: "Registration status in Sophia dictionary (1=registered, 0=not registered)",
		},
	)

	prometheus.MustRegister(sg.inferenceLatency, sg.inferenceErrors, sg.tokensGenerated, sg.tokensPrompted, sg.requestsTotal, sg.registrationStatus)

	return sg
}

func (sg *SophiaGateway) logf(level string, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	fmt.Printf("[%s] [%s] %s\n", timestamp, level, fmt.Sprintf(format, args...))
}

func (sg *SophiaGateway) registerInSophia(ctx context.Context) error {
	sg.logf("INFO", "Registering services in Sophia dictionary at %s", sg.protocolAPI)

	entries := []SophiaServiceEntry{
		{
			Name:      "sophia-eye/vllm",
			Endpoint:  sg.vllmAddr,
			Model:     "deepseek-r1-7b",
			Type:      "llm",
			Timestamp: time.Now().Unix(),
			Metadata: map[string]string{
				"gpu":         "amdgpu",
				"max_tokens":  "32768",
				"memory_util": "0.85",
			},
		},
		{
			Name:      "sophia-eye/qwen",
			Endpoint:  sg.qwenAddr,
			Model:     "qwen2.5-coder-7b",
			Type:      "code",
			Timestamp: time.Now().Unix(),
			Metadata: map[string]string{
				"gpu":         "amdgpu",
				"max_tokens":  "16384",
				"memory_util": "0.80",
			},
		},
		{
			Name:      "sophia-eye/qdrant",
			Endpoint:  sg.qdrantAddr,
			Type:      "vector-db",
			Timestamp: time.Now().Unix(),
			Metadata: map[string]string{
				"storage": "/fast/qdrant",
				"grpc":    "6334",
			},
		},
		{
			Name:      "sophia-eye/embeddings",
			Endpoint:  sg.embeddAddr,
			Model:     "bge-m3",
			Type:      "embeddings",
			Timestamp: time.Now().Unix(),
			Metadata: map[string]string{
				"dimension": "1024",
			},
		},
	}

	for _, entry := range entries {
		body, err := json.Marshal(entry)
		if err != nil {
			sg.logf("ERROR", "Failed to marshal entry %s: %v", entry.Name, err)
			return err
		}

		req, err := http.NewRequestWithContext(ctx, "POST", sg.protocolAPI+"/api/sophia/register", bytes.NewReader(body))
		if err != nil {
			sg.logf("ERROR", "Failed to create registration request for %s: %v", entry.Name, err)
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			sg.logf("ERROR", "Registration request failed for %s: %v", entry.Name, err)
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			sg.logf("ERROR", "Registration failed for %s: status %d, body: %s", entry.Name, resp.StatusCode, string(body))
			return fmt.Errorf("registration failed with status %d", resp.StatusCode)
		}

		sg.logf("INFO", "Registered service: %s at %s", entry.Name, entry.Endpoint)
	}

	sg.registrationStatus.Set(1)
	sg.registered = true
	return nil
}

func (sg *SophiaGateway) recordInferenceToAnamnesis(ctx context.Context, record InferenceRecord) error {
	body, err := json.Marshal(record)
	if err != nil {
		sg.logf("ERROR", "Failed to marshal inference record: %v", err)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sg.protocolAPI+"/api/anamnesis/record", bytes.NewReader(body))
	if err != nil {
		sg.logf("ERROR", "Failed to create anamnesis record request: %v", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		sg.logf("WARN", "Anamnesis record request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sg.logf("WARN", "Anamnesis record returned status %d", resp.StatusCode)
	}

	return nil
}

func (sg *SophiaGateway) reportToWotan(ctx context.Context, metric WotanMetric) error {
	body, err := json.Marshal(metric)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+sg.wotanAddr+"/metrics/ingest", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		sg.logf("WARN", "Wotan metric report failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (sg *SophiaGateway) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	startTime := time.Now()
	sg.requestsTotal.Inc()

	var req InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sg.logf("ERROR", "Failed to decode chat request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		sg.inferenceErrors.Inc()
		return
	}

	var endpoint string
	switch req.Model {
	case "deepseek-r1-7b", "deepseek":
		endpoint = sg.vllmAddr
	case "qwen2.5-coder-7b", "qwen":
		endpoint = sg.qwenAddr
	default:
		sg.logf("ERROR", "Unknown model: %s", req.Model)
		http.Error(w, "Unknown model", http.StatusBadRequest)
		sg.inferenceErrors.Inc()
		return
	}

	// Create a new request to the inference service
	clientReq, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/v1/chat/completions", bytes.NewReader(mustMarshal(req)))
	if err != nil {
		sg.logf("ERROR", "Failed to create inference request: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		sg.inferenceErrors.Inc()
		return
	}
	clientReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(clientReq)
	if err != nil {
		sg.logf("ERROR", "Inference request to %s failed: %v", endpoint, err)
		http.Error(w, "Inference service unavailable", http.StatusServiceUnavailable)
		sg.inferenceErrors.Inc()
		return
	}
	defer resp.Body.Close()

	latency := time.Since(startTime)
	sg.inferenceLatency.Observe(float64(latency.Milliseconds()))

	// Parse response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		sg.logf("ERROR", "Failed to read inference response: %v", err)
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		sg.inferenceErrors.Inc()
		return
	}

	var inferenceResp InferenceResponse
	if err := json.Unmarshal(respBody, &inferenceResp); err != nil {
		sg.logf("ERROR", "Failed to parse inference response: %v", err)
		http.Error(w, "Failed to parse response", http.StatusInternalServerError)
		sg.inferenceErrors.Inc()
		return
	}

	// Update token metrics
	sg.tokensPrompted.Add(float64(inferenceResp.Usage.PromptTokens))
	sg.tokensGenerated.Add(float64(inferenceResp.Usage.CompletionTokens))

	// Record in Anamnesis
	record := InferenceRecord{
		ServiceName:      sg.serviceName,
		Model:            req.Model,
		PromptTokens:     inferenceResp.Usage.PromptTokens,
		CompletionTokens: inferenceResp.Usage.CompletionTokens,
		LatencyMs:        float64(latency.Milliseconds()),
		Timestamp:        time.Now().Unix(),
		Status:           "success",
	}
	go sg.recordInferenceToAnamnesis(context.Background(), record)

	// Report to Wotan
	metric := WotanMetric{
		Timestamp:   time.Now().UnixMilli(),
		ServiceName: sg.serviceName,
		EventType:   "inference_complete",
		Model:       req.Model,
		Tokens:      inferenceResp.Usage.TotalTokens,
		LatencyMs:   float64(latency.Milliseconds()),
		Metadata: map[string]string{
			"prompt_tokens":     fmt.Sprintf("%d", inferenceResp.Usage.PromptTokens),
			"completion_tokens": fmt.Sprintf("%d", inferenceResp.Usage.CompletionTokens),
		},
	}
	go sg.reportToWotan(context.Background(), metric)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (sg *SophiaGateway) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

func (sg *SophiaGateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "healthy",
		"service":    sg.serviceName,
		"timestamp":  time.Now().Unix(),
		"registered": sg.registrationStatus,
	})
}

func (sg *SophiaGateway) handleReady(w http.ResponseWriter, r *http.Request) {
	if sg.registered {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
}

func (sg *SophiaGateway) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models": []map[string]string{
			{"name": "deepseek-r1-7b", "type": "llm", "endpoint": sg.vllmAddr},
			{"name": "qwen2.5-coder-7b", "type": "code", "endpoint": sg.qwenAddr},
		},
	})
}

func (sg *SophiaGateway) Start(ctx context.Context) error {
	// Register in Sophia on startup
	if err := sg.registerInSophia(ctx); err != nil {
		sg.logf("ERROR", "Failed to register in Sophia: %v", err)
		// Continue anyway, but mark as not registered
		sg.registrationStatus.Set(0)
	}

	// Start HTTP server for inference requests
	go func() {
		http.HandleFunc("/v1/chat/completions", sg.handleChat)
		http.HandleFunc("/health", sg.handleHealth)
		http.HandleFunc("/ready", sg.handleReady)
		http.HandleFunc("/models", sg.handleModels)

		server := &http.Server{
			Addr:         ":" + sg.gatewayPort,
			ReadTimeout:  120 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  30 * time.Second,
		}

		sg.logf("INFO", "Starting Sophia's Eye gateway on port %s", sg.gatewayPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sg.logf("ERROR", "Gateway server error: %v", err)
		}
	}()

	// Start metrics server
	go func() {
		http.Handle("/metrics", http.HandlerFunc(sg.handleMetrics))
		server := &http.Server{
			Addr:         ":" + sg.metricsPort,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		sg.logf("INFO", "Starting metrics server on port %s", sg.metricsPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sg.logf("ERROR", "Metrics server error: %v", err)
		}
	}()

	sg.logf("INFO", "Sophia's Eye gateway initialized successfully")
	return nil
}

func mustMarshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

func main() {
	gateway := NewSophiaGateway()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := gateway.Start(ctx); err != nil {
		gateway.logf("ERROR", "Failed to start gateway: %v", err)
		cancel()
		os.Exit(1)
	}
	cancel()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	gateway.logf("INFO", "Shutting down Sophia's Eye gateway")
}
