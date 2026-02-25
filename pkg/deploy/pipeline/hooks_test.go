// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewHookExecutor tests hook executor creation.
func TestNewHookExecutor(t *testing.T) {
	executor := NewHookExecutor()

	if executor == nil {
		t.Fatal("NewHookExecutor returned nil")
	}
	if executor.httpClient == nil {
		t.Error("httpClient should be initialized")
	}
}

// TestHookTypeConstants tests hook type constant values.
func TestHookTypeConstants(t *testing.T) {
	tests := []struct {
		hookType HookType
		expected string
	}{
		{HookTypeExec, "exec"},
		{HookTypeHTTP, "http"},
		{HookTypeWotan, "wotan"},
		{HookTypeWebhook, "webhook"},
		{HookTypeScript, "script"},
	}

	for _, tt := range tests {
		t.Run(string(tt.hookType), func(t *testing.T) {
			if string(tt.hookType) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.hookType))
			}
		})
	}
}

// TestHookErrorActionConstants tests hook error action constant values.
func TestHookErrorActionConstants(t *testing.T) {
	tests := []struct {
		action   HookErrorAction
		expected string
	}{
		{HookErrorAbort, "abort"},
		{HookErrorContinue, "continue"},
		{HookErrorRetry, "retry"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.action))
			}
		})
	}
}

// TestHookBuilderBasic tests basic hook builder functionality.
func TestHookBuilderBasic(t *testing.T) {
	hook := NewHookBuilder("test-hook", HookTypeHTTP).
		WithTimeout(30 * time.Second).
		WithRetries(3).
		WithOnError(HookErrorContinue).
		Build()

	if hook.Name != "test-hook" {
		t.Errorf("expected name 'test-hook', got %s", hook.Name)
	}
	if hook.Type != HookTypeHTTP {
		t.Errorf("expected type %s, got %s", HookTypeHTTP, hook.Type)
	}
	if hook.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", hook.Timeout)
	}
	if hook.Retries != 3 {
		t.Errorf("expected retries 3, got %d", hook.Retries)
	}
	if hook.OnError != HookErrorContinue {
		t.Errorf("expected onError %s, got %s", HookErrorContinue, hook.OnError)
	}
}

// TestHookBuilderWithCommand tests command hook building.
func TestHookBuilderWithCommand(t *testing.T) {
	hook := NewHookBuilder("exec-hook", HookTypeExec).
		WithCommand("echo", "hello", "world").
		Build()

	if hook.Config == nil {
		t.Fatal("config should not be nil")
	}
	if hook.Config.Command != "echo" {
		t.Errorf("expected command 'echo', got %s", hook.Config.Command)
	}
	if len(hook.Config.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(hook.Config.Args))
	}
}

// TestHookBuilderWithURL tests URL hook building.
func TestHookBuilderWithURL(t *testing.T) {
	hook := NewHookBuilder("http-hook", HookTypeHTTP).
		WithURL("http://example.com/webhook").
		WithMethod("POST").
		WithHeaders(map[string]string{"Authorization": "Bearer token"}).
		Build()

	if hook.Config.URL != "http://example.com/webhook" {
		t.Errorf("expected URL 'http://example.com/webhook', got %s", hook.Config.URL)
	}
	if hook.Config.Method != "POST" {
		t.Errorf("expected method 'POST', got %s", hook.Config.Method)
	}
	if hook.Config.Headers["Authorization"] != "Bearer token" {
		t.Errorf("expected Authorization header, got %v", hook.Config.Headers)
	}
}

// TestHookBuilderWithTopic tests Wotan hook building.
func TestHookBuilderWithTopic(t *testing.T) {
	payload := map[string]interface{}{
		"key": "value",
	}

	hook := NewHookBuilder("wotan-hook", HookTypeWotan).
		WithTopic("deployment.events").
		WithPayload(payload).
		Build()

	if hook.Config.Topic != "deployment.events" {
		t.Errorf("expected topic 'deployment.events', got %s", hook.Config.Topic)
	}
	if hook.Config.Payload["key"] != "value" {
		t.Errorf("expected payload key 'value', got %v", hook.Config.Payload)
	}
}

// TestHookBuilderWithScript tests script hook building.
func TestHookBuilderWithScript(t *testing.T) {
	hook := NewHookBuilder("script-hook", HookTypeScript).
		WithScript("echo 'Hello World'", "bash").
		Build()

	if hook.Config.Script != "echo 'Hello World'" {
		t.Errorf("expected script 'echo Hello World', got %s", hook.Config.Script)
	}
	if hook.Config.Language != "bash" {
		t.Errorf("expected language 'bash', got %s", hook.Config.Language)
	}
}

// TestExecHookConstructor tests ExecHook constructor.
func TestExecHookConstructor(t *testing.T) {
	hook := ExecHook("test", "ls", "-la", "/tmp")

	if hook.Name != "test" {
		t.Errorf("expected name 'test', got %s", hook.Name)
	}
	if hook.Type != HookTypeExec {
		t.Errorf("expected type %s, got %s", HookTypeExec, hook.Type)
	}
	if hook.Config.Command != "ls" {
		t.Errorf("expected command 'ls', got %s", hook.Config.Command)
	}
	if len(hook.Config.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(hook.Config.Args))
	}
	if hook.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", hook.Timeout)
	}
}

// TestHTTPHookConstructor tests HTTPHook constructor.
func TestHTTPHookConstructor(t *testing.T) {
	hook := HTTPHook("test", "http://example.com", "POST")

	if hook.Name != "test" {
		t.Errorf("expected name 'test', got %s", hook.Name)
	}
	if hook.Type != HookTypeHTTP {
		t.Errorf("expected type %s, got %s", HookTypeHTTP, hook.Type)
	}
	if hook.Config.URL != "http://example.com" {
		t.Errorf("expected URL 'http://example.com', got %s", hook.Config.URL)
	}
	if hook.Config.Method != "POST" {
		t.Errorf("expected method 'POST', got %s", hook.Config.Method)
	}
}

// TestWebhookHookConstructor tests WebhookHook constructor.
func TestWebhookHookConstructor(t *testing.T) {
	hook := WebhookHook("test", "http://example.com/webhook")

	if hook.Name != "test" {
		t.Errorf("expected name 'test', got %s", hook.Name)
	}
	if hook.Type != HookTypeWebhook {
		t.Errorf("expected type %s, got %s", HookTypeWebhook, hook.Type)
	}
	if hook.Config.Method != "POST" {
		t.Errorf("expected method 'POST', got %s", hook.Config.Method)
	}
}

// TestWotanHookConstructor tests WotanHook constructor.
func TestWotanHookConstructor(t *testing.T) {
	payload := map[string]interface{}{"test": "value"}
	hook := WotanHook("test", "test.topic", payload)

	if hook.Name != "test" {
		t.Errorf("expected name 'test', got %s", hook.Name)
	}
	if hook.Type != HookTypeWotan {
		t.Errorf("expected type %s, got %s", HookTypeWotan, hook.Type)
	}
	if hook.Config.Topic != "test.topic" {
		t.Errorf("expected topic 'test.topic', got %s", hook.Config.Topic)
	}
}

// TestScriptHookConstructor tests ScriptHook constructor.
func TestScriptHookConstructor(t *testing.T) {
	hook := ScriptHook("test", "echo hello", "bash")

	if hook.Name != "test" {
		t.Errorf("expected name 'test', got %s", hook.Name)
	}
	if hook.Type != HookTypeScript {
		t.Errorf("expected type %s, got %s", HookTypeScript, hook.Type)
	}
	if hook.Timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", hook.Timeout)
	}
}

// TestHookExecutorSetWotanPublisher tests setting Wotan publisher.
func TestHookExecutorSetWotanPublisher(t *testing.T) {
	executor := NewHookExecutor()

	mockPublisher := &MockWotanPublisher{}
	executor.SetWotanPublisher(mockPublisher)

	if executor.wotanPublisher == nil {
		t.Error("Wotan publisher should be set")
	}
}

// MockWotanPublisher implements WotanPublisher for testing.
type MockWotanPublisher struct {
	PublishedMessages []struct {
		Topic   string
		Payload []byte
	}
	Err error
}

func (m *MockWotanPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	if m.Err != nil {
		return m.Err
	}
	m.PublishedMessages = append(m.PublishedMessages, struct {
		Topic   string
		Payload []byte
	}{topic, payload})
	return nil
}

// TestHTTPHookExecution tests HTTP hook execution.
func TestHTTPHookExecution(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "http-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "POST",
		},
	}

	ctx := context.Background()
	pipelineCtx := map[string]interface{}{
		"deployment_id": "deploy-123",
	}

	result := executor.ExecuteWithResult(ctx, hook, pipelineCtx)

	if !result.Success {
		t.Errorf("hook should succeed, got error: %s", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
}

// TestHTTPHookExecutionWithHeaders tests HTTP hook with custom headers.
func TestHTTPHookExecutionWithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "custom-value" {
			t.Errorf("expected custom header, got %s", r.Header.Get("X-Custom-Header"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "http-headers-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "POST",
			Headers: map[string]string{
				"X-Custom-Header": "custom-value",
			},
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if !result.Success {
		t.Errorf("hook should succeed, got error: %s", result.Error)
	}
}

// TestHTTPHookExecutionFailure tests HTTP hook failure handling.
func TestHTTPHookExecutionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "http-fail-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "POST",
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Success {
		t.Error("hook should fail for 500 status")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", result.StatusCode)
	}
}

// TestHTTPHookWithCustomBody tests HTTP hook with custom body.
func TestHTTPHookWithCustomBody(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "http-body-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "POST",
			Body: map[string]string{
				"custom_key": "custom_value",
			},
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if !result.Success {
		t.Errorf("hook should succeed, got error: %s", result.Error)
	}
	if receivedBody["custom_key"] != "custom_value" {
		t.Errorf("expected custom_key, got %v", receivedBody)
	}
}

// TestWotanHookExecution tests Wotan hook execution.
func TestWotanHookExecution(t *testing.T) {
	executor := NewHookExecutor()

	mockPublisher := &MockWotanPublisher{}
	executor.SetWotanPublisher(mockPublisher)

	hook := &Hook{
		Name:    "wotan-test",
		Type:    HookTypeWotan,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			Topic: "test.events",
			Payload: map[string]interface{}{
				"event": "deploy_started",
			},
		},
	}

	ctx := context.Background()
	pipelineCtx := map[string]interface{}{
		"service": "test-service",
	}

	result := executor.ExecuteWithResult(ctx, hook, pipelineCtx)

	if !result.Success {
		t.Errorf("hook should succeed, got error: %s", result.Error)
	}
	if len(mockPublisher.PublishedMessages) != 1 {
		t.Errorf("expected 1 published message, got %d", len(mockPublisher.PublishedMessages))
	}
	if mockPublisher.PublishedMessages[0].Topic != "test.events" {
		t.Errorf("expected topic 'test.events', got %s", mockPublisher.PublishedMessages[0].Topic)
	}
}

// TestWotanHookWithoutPublisher tests Wotan hook without publisher.
func TestWotanHookWithoutPublisher(t *testing.T) {
	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "wotan-no-publisher",
		Type:    HookTypeWotan,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			Topic: "test.events",
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Success {
		t.Error("hook should fail without publisher")
	}
	if !strings.Contains(result.Error, "Wotan publisher not configured") {
		t.Errorf("expected error about publisher, got %s", result.Error)
	}
}

// TestWotanHookWithoutTopic tests Wotan hook without topic.
func TestWotanHookWithoutTopic(t *testing.T) {
	executor := NewHookExecutor()

	mockPublisher := &MockWotanPublisher{}
	executor.SetWotanPublisher(mockPublisher)

	hook := &Hook{
		Name:    "wotan-no-topic",
		Type:    HookTypeWotan,
		Timeout: 10 * time.Second,
		Config:  &HookConfig{},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Success {
		t.Error("hook should fail without topic")
	}
	if !strings.Contains(result.Error, "requires topic") {
		t.Errorf("expected error about topic, got %s", result.Error)
	}
}

// TestInvalidHookType tests execution with invalid hook type.
func TestInvalidHookType(t *testing.T) {
	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "invalid-type",
		Type:    HookType("invalid"),
		Timeout: 10 * time.Second,
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Success {
		t.Error("hook should fail for invalid type")
	}
	if !strings.Contains(result.Error, "invalid hook type") {
		t.Errorf("expected error about invalid type, got %s", result.Error)
	}
}

// TestHookTimeout tests hook timeout handling.
func TestHookTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "timeout-test",
		Type:    HookTypeHTTP,
		Timeout: 100 * time.Millisecond,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "GET",
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Success {
		t.Error("hook should fail due to timeout")
	}
	if result.Duration < 100*time.Millisecond {
		t.Errorf("duration should be at least 100ms, got %v", result.Duration)
	}
}

// TestHookWithRetries tests hook retry functionality.
func TestHookWithRetries(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "retry-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Retries: 3,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "GET",
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if !result.Success {
		t.Errorf("hook should succeed after retries, got error: %s", result.Error)
	}
	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
	if result.Retries < 2 {
		t.Errorf("expected at least 2 retries, got %d", result.Retries)
	}
}

// TestExecuteHooksSequential tests sequential hook execution.
func TestExecuteHooksSequential(t *testing.T) {
	executionOrder := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookName := r.URL.Query().Get("hook")
		executionOrder = append(executionOrder, hookName)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hooks := []*Hook{
		{
			Name:    "hook-1",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-1",
				Method: "GET",
			},
		},
		{
			Name:    "hook-2",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-2",
				Method: "GET",
			},
		},
	}

	ctx := context.Background()
	results, err := executor.ExecuteHooks(ctx, hooks, nil)

	if err != nil {
		t.Errorf("ExecuteHooks failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if len(executionOrder) != 2 {
		t.Errorf("expected 2 hooks executed, got %d", len(executionOrder))
	}
	if executionOrder[0] != "hook-1" || executionOrder[1] != "hook-2" {
		t.Errorf("hooks executed in wrong order: %v", executionOrder)
	}
}

// TestExecuteHooksWithFailure tests hook failure handling in batch.
func TestExecuteHooksWithFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookName := r.URL.Query().Get("hook")
		if hookName == "hook-fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hooks := []*Hook{
		{
			Name:    "hook-success",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			OnError: HookErrorContinue,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-success",
				Method: "GET",
			},
		},
		{
			Name:    "hook-fail",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			OnError: HookErrorAbort,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-fail",
				Method: "GET",
			},
		},
		{
			Name:    "hook-after-fail",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-after-fail",
				Method: "GET",
			},
		},
	}

	ctx := context.Background()
	results, err := executor.ExecuteHooks(ctx, hooks, nil)

	if err == nil {
		t.Error("ExecuteHooks should return error on abort")
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results (stopped at failure), got %d", len(results))
	}
}

// TestExecuteHooksWithContinueOnError tests continue on error behavior.
func TestExecuteHooksWithContinueOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookName := r.URL.Query().Get("hook")
		if hookName == "hook-fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hooks := []*Hook{
		{
			Name:    "hook-1",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			OnError: HookErrorContinue,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-1",
				Method: "GET",
			},
		},
		{
			Name:    "hook-fail",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			OnError: HookErrorContinue,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-fail",
				Method: "GET",
			},
		},
		{
			Name:    "hook-3",
			Type:    HookTypeHTTP,
			Timeout: 10 * time.Second,
			OnError: HookErrorContinue,
			Config: &HookConfig{
				URL:    server.URL + "?hook=hook-3",
				Method: "GET",
			},
		},
	}

	ctx := context.Background()
	results, err := executor.ExecuteHooks(ctx, hooks, nil)

	if err != nil {
		t.Errorf("ExecuteHooks should not return error on continue: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

// TestHTTPHookMissingURL tests HTTP hook without URL.
func TestHTTPHookMissingURL(t *testing.T) {
	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "no-url",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config:  &HookConfig{},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Success {
		t.Error("hook should fail without URL")
	}
	if !strings.Contains(result.Error, "requires URL") {
		t.Errorf("expected error about URL, got %s", result.Error)
	}
}

// TestHookResultFields tests HookResult field values.
func TestHookResultFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "result-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "GET",
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Hook != hook {
		t.Error("result should reference the hook")
	}
	if result.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if result.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
	if result.Output == "" {
		t.Error("Output should contain response body")
	}
}

// TestExecuteWrapper tests Execute wrapper function.
func TestExecuteWrapper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "execute-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "GET",
		},
	}

	ctx := context.Background()
	err := executor.Execute(ctx, hook, nil)

	if err != nil {
		t.Errorf("Execute should succeed: %v", err)
	}
}

// TestExecuteWrapperFailure tests Execute wrapper on failure.
func TestExecuteWrapperFailure(t *testing.T) {
	executor := NewHookExecutor()

	hook := &Hook{
		Name:    "execute-fail-test",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config:  &HookConfig{},
	}

	ctx := context.Background()
	err := executor.Execute(ctx, hook, nil)

	if err == nil {
		t.Error("Execute should return error")
	}
}

// TestSanitizeEnvKey tests environment variable key sanitization.
func TestSanitizeEnvKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "SIMPLE"},
		{"with_underscore", "WITH_UNDERSCORE"},
		{"with-dash", "WITH_DASH"},
		{"with spaces", "WITHSPACES"},
		{"MixedCase", "MIXEDCASE"},
		{"with123numbers", "WITH123NUMBERS"},
		{"123startsWithNumber", "23STARTSWITHNUMBER"},
		{"special!@#chars", "SPECIALCHARS"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeEnvKey(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestAllowedScriptLanguages tests allowed script languages.
func TestAllowedScriptLanguages(t *testing.T) {
	validLanguages := []string{"bash", "sh", "python", "python3"}

	for _, lang := range validLanguages {
		if _, ok := allowedScriptLanguages[lang]; !ok {
			t.Errorf("expected %s to be allowed", lang)
		}
	}

	invalidLanguages := []string{"ruby", "perl", "javascript"}
	for _, lang := range invalidLanguages {
		if _, ok := allowedScriptLanguages[lang]; ok {
			t.Errorf("expected %s to not be allowed", lang)
		}
	}
}

// TestScriptFileExtensions tests script file extension mapping.
func TestScriptFileExtensions(t *testing.T) {
	tests := []struct {
		language  string
		extension string
	}{
		{"bash", ".sh"},
		{"sh", ".sh"},
		{"python", ".py"},
		{"python3", ".py"},
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			ext, ok := scriptFileExtensions[tt.language]
			if !ok {
				t.Errorf("expected extension for %s", tt.language)
			}
			if ext != tt.extension {
				t.Errorf("expected %s, got %s", tt.extension, ext)
			}
		})
	}
}

// TestWotanHookPublishError tests Wotan hook with publish error.
func TestWotanHookPublishError(t *testing.T) {
	executor := NewHookExecutor()

	mockPublisher := &MockWotanPublisher{
		Err: errors.New("publish failed"),
	}
	executor.SetWotanPublisher(mockPublisher)

	hook := &Hook{
		Name:    "wotan-error",
		Type:    HookTypeWotan,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			Topic: "test.events",
		},
	}

	ctx := context.Background()
	result := executor.ExecuteWithResult(ctx, hook, nil)

	if result.Success {
		t.Error("hook should fail on publish error")
	}
	if !strings.Contains(result.Error, "publish") {
		t.Errorf("expected error about publish, got %s", result.Error)
	}
}

// BenchmarkHTTPHookExecution benchmarks HTTP hook execution.
func BenchmarkHTTPHookExecution(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewHookExecutor()
	hook := &Hook{
		Name:    "bench-hook",
		Type:    HookTypeHTTP,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			URL:    server.URL,
			Method: "GET",
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.ExecuteWithResult(ctx, hook, nil)
	}
}

// BenchmarkWotanHookExecution benchmarks Wotan hook execution.
func BenchmarkWotanHookExecution(b *testing.B) {
	executor := NewHookExecutor()
	mockPublisher := &MockWotanPublisher{}
	executor.SetWotanPublisher(mockPublisher)

	hook := &Hook{
		Name:    "bench-hook",
		Type:    HookTypeWotan,
		Timeout: 10 * time.Second,
		Config: &HookConfig{
			Topic: "test.events",
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.ExecuteWithResult(ctx, hook, nil)
	}
}
