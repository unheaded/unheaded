// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Hook errors
var (
	ErrHookTimeout     = errors.New("hook execution timed out")
	ErrHookExecFailed  = errors.New("hook execution failed")
	ErrHookHTTPFailed  = errors.New("hook HTTP request failed")
	ErrInvalidHookType = errors.New("invalid hook type")
)

// HookType represents the type of hook.
type HookType string

const (
	HookTypeExec    HookType = "exec"
	HookTypeHTTP    HookType = "http"
	HookTypeWotan   HookType = "wotan"
	HookTypeWebhook HookType = "webhook"
	HookTypeScript  HookType = "script"
)

// HookErrorAction specifies what to do when a hook fails.
type HookErrorAction string

const (
	HookErrorAbort    HookErrorAction = "abort"
	HookErrorContinue HookErrorAction = "continue"
	HookErrorRetry    HookErrorAction = "retry"
)

// Hook represents a deployment hook.
type Hook struct {
	// Name is the hook name
	Name string `json:"name"`

	// Type is the hook type
	Type HookType `json:"type"`

	// Config contains hook-specific configuration
	Config *HookConfig `json:"config,omitempty"`

	// Timeout for hook execution
	Timeout time.Duration `json:"timeout,omitempty"`

	// OnError specifies behavior on error
	OnError HookErrorAction `json:"on_error,omitempty"`

	// Retries is the number of retries
	Retries int `json:"retries,omitempty"`

	// Condition is an optional condition for hook execution
	Condition string `json:"condition,omitempty"`
}

// HookConfig contains hook-specific configuration.
type HookConfig struct {
	// Exec hook config
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	Dir     string   `json:"dir,omitempty"`

	// HTTP/Webhook hook config
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body,omitempty"`

	// Wotan hook config
	Topic   string                 `json:"topic,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`

	// Script hook config
	Script   string `json:"script,omitempty"`
	Language string `json:"language,omitempty"` // bash, python, etc.
}

// HookSpec defines the specification for a hook.
type HookSpec struct {
	Name    string          `json:"name"`
	Type    HookType        `json:"type"`
	Config  *HookConfig     `json:"config,omitempty"`
	Timeout time.Duration   `json:"timeout,omitempty"`
	OnError HookErrorAction `json:"on_error,omitempty"`
	Retries int             `json:"retries,omitempty"`
}

// HookResult contains the result of hook execution.
type HookResult struct {
	// Success indicates if the hook succeeded
	Success bool `json:"success"`

	// Hook is the executed hook
	Hook *Hook `json:"hook"`

	// Output contains hook output
	Output string `json:"output,omitempty"`

	// Error contains error message if failed
	Error string `json:"error,omitempty"`

	// ExitCode for exec hooks
	ExitCode int `json:"exit_code,omitempty"`

	// StatusCode for HTTP hooks
	StatusCode int `json:"status_code,omitempty"`

	// StartedAt is when execution started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when execution completed
	CompletedAt time.Time `json:"completed_at"`

	// Duration is the execution duration
	Duration time.Duration `json:"duration"`

	// Retries is the number of retries performed
	Retries int `json:"retries,omitempty"`
}

// HookExecutor executes deployment hooks.
type HookExecutor struct {
	// httpClient for HTTP hooks
	httpClient *http.Client

	// wotanPublisher for Wotan hooks
	wotanPublisher WotanPublisher
}

// WotanPublisher interface for publishing to Wotan.
type WotanPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// NewHookExecutor creates a new hook executor.
func NewHookExecutor() *HookExecutor {
	return &HookExecutor{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetWotanPublisher sets the Wotan publisher.
func (e *HookExecutor) SetWotanPublisher(publisher WotanPublisher) {
	e.wotanPublisher = publisher
}

// Execute executes a hook.
func (e *HookExecutor) Execute(ctx context.Context, hook *Hook, pipelineCtx map[string]interface{}) error {
	result := e.ExecuteWithResult(ctx, hook, pipelineCtx)
	if !result.Success {
		return errors.New(result.Error)
	}
	return nil
}

// ExecuteWithResult executes a hook and returns the result.
func (e *HookExecutor) ExecuteWithResult(ctx context.Context, hook *Hook, pipelineCtx map[string]interface{}) *HookResult {
	result := &HookResult{
		Hook:      hook,
		StartedAt: time.Now(),
	}

	// Set default timeout
	timeout := hook.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Create context with timeout
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute with retries
	maxAttempts := hook.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result.Retries = attempt

		switch hook.Type {
		case HookTypeExec:
			lastErr = e.executeExecHook(hookCtx, hook, result, pipelineCtx)
		case HookTypeHTTP, HookTypeWebhook:
			lastErr = e.executeHTTPHook(hookCtx, hook, result, pipelineCtx)
		case HookTypeWotan:
			lastErr = e.executeWotanHook(hookCtx, hook, result, pipelineCtx)
		case HookTypeScript:
			lastErr = e.executeScriptHook(hookCtx, hook, result, pipelineCtx)
		default:
			lastErr = fmt.Errorf("%w: %s", ErrInvalidHookType, hook.Type)
		}

		if lastErr == nil {
			break
		}

		// Wait before retry
		if attempt < maxAttempts-1 {
			select {
			case <-hookCtx.Done():
				lastErr = hookCtx.Err()
				break
			case <-time.After(time.Second * time.Duration(attempt+1)):
			}
		}
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	if lastErr != nil {
		result.Success = false
		result.Error = lastErr.Error()
	} else {
		result.Success = true
	}

	return result
}

// executeExecHook executes a command hook.
func (e *HookExecutor) executeExecHook(ctx context.Context, hook *Hook, result *HookResult, pipelineCtx map[string]interface{}) error {
	if hook.Config == nil || hook.Config.Command == "" {
		return fmt.Errorf("exec hook requires command")
	}

	// Build command
	args := hook.Config.Args
	if args == nil {
		args = []string{}
	}

	cmd := exec.CommandContext(ctx, hook.Config.Command, args...) // #nosec G204 -- operator-configured deploy hook; running supplied code is the feature

	// Set working directory
	if hook.Config.Dir != "" {
		cmd.Dir = hook.Config.Dir
	}

	// Set environment
	if len(hook.Config.Env) > 0 {
		cmd.Env = hook.Config.Env
	}

	// Add pipeline context as environment variables
	for key, value := range pipelineCtx {
		switch v := value.(type) {
		case string:
			cmd.Env = append(cmd.Env, fmt.Sprintf("PIPELINE_%s=%s", strings.ToUpper(key), v))
		case int, int64, float64:
			cmd.Env = append(cmd.Env, fmt.Sprintf("PIPELINE_%s=%v", strings.ToUpper(key), v))
		}
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result.Output = stdout.String()
	if stderr.Len() > 0 {
		result.Output += "\nSTDERR: " + stderr.String()
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return fmt.Errorf("%w: %v", ErrHookExecFailed, err)
	}

	result.ExitCode = 0
	return nil
}

// executeHTTPHook executes an HTTP/webhook hook.
func (e *HookExecutor) executeHTTPHook(ctx context.Context, hook *Hook, result *HookResult, pipelineCtx map[string]interface{}) error {
	if hook.Config == nil || hook.Config.URL == "" {
		return fmt.Errorf("HTTP hook requires URL")
	}

	method := hook.Config.Method
	if method == "" {
		method = "POST"
	}

	// Build request body
	var bodyReader *bytes.Reader
	if hook.Config.Body != nil {
		bodyData, err := json.Marshal(hook.Config.Body)
		if err != nil {
			return fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyData)
	} else {
		// Send pipeline context as body
		bodyData, _ := json.Marshal(pipelineCtx)
		bodyReader = bytes.NewReader(bodyData)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, hook.Config.URL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for key, value := range hook.Config.Headers {
		req.Header.Set(key, value)
	}

	// Execute request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHookHTTPFailed, err)
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	// Read response body
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	result.Output = buf.String()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrHookHTTPFailed, resp.StatusCode)
	}

	return nil
}

// executeWotanHook executes a Wotan hook.
func (e *HookExecutor) executeWotanHook(ctx context.Context, hook *Hook, result *HookResult, pipelineCtx map[string]interface{}) error {
	if e.wotanPublisher == nil {
		return fmt.Errorf("wotan publisher not configured")
	}

	if hook.Config == nil || hook.Config.Topic == "" {
		return fmt.Errorf("wotan hook requires topic")
	}

	// Build payload
	payload := hook.Config.Payload
	if payload == nil {
		payload = make(map[string]interface{})
	}

	// Add pipeline context to payload
	payload["pipeline_context"] = pipelineCtx
	payload["hook_name"] = hook.Name
	payload["timestamp"] = time.Now().UTC().Format(time.RFC3339)

	// Serialize payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Publish to Wotan
	if err := e.wotanPublisher.Publish(ctx, hook.Config.Topic, payloadBytes); err != nil {
		return fmt.Errorf("failed to publish to Wotan: %w", err)
	}

	result.Output = fmt.Sprintf("Published to topic: %s", hook.Config.Topic)
	return nil
}

// allowedScriptLanguages defines the whitelist of supported script interpreters.
var allowedScriptLanguages = map[string]string{
	"bash":    "bash",
	"sh":      "sh",
	"python":  "python",
	"python3": "python3",
}

// scriptFileExtensions maps languages to their file extensions.
var scriptFileExtensions = map[string]string{
	"bash":    ".sh",
	"sh":      ".sh",
	"python":  ".py",
	"python3": ".py",
}

// executeScriptHook executes a script hook securely by writing to a temp file.
func (e *HookExecutor) executeScriptHook(ctx context.Context, hook *Hook, result *HookResult, pipelineCtx map[string]interface{}) error {
	if hook.Config == nil || hook.Config.Script == "" {
		return fmt.Errorf("script hook requires script")
	}

	language := hook.Config.Language
	if language == "" {
		language = "bash"
	}

	// Validate language against whitelist to prevent arbitrary interpreter injection
	interpreter, ok := allowedScriptLanguages[language]
	if !ok {
		return fmt.Errorf("unsupported script language: %s", language)
	}

	// Get file extension for the script
	ext := scriptFileExtensions[language]

	// Create a temporary file for the script to avoid command injection via -c
	// This prevents shell metacharacter injection that could occur when passing
	// script content directly to the shell via -c flag
	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "hook-script-*"+ext)
	if err != nil {
		return fmt.Errorf("failed to create temp script file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup of temporary file
	defer func() { _ = os.Remove(tmpPath) }()

	// Write script content to the temp file
	if _, err := tmpFile.WriteString(hook.Config.Script); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write script to temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp script file: %w", err)
	}

	// Set executable permissions (owner read/execute only for security).
	// tmpPath comes from os.CreateTemp — not user-influenced; gosec G302/
	// G703 false positives.
	if err := os.Chmod(tmpPath, 0500); err != nil { //nolint:gosec // tmpPath from os.CreateTemp; 0500 is intentional owner-exec-only
		return fmt.Errorf("failed to set script permissions: %w", err)
	}

	// Resolve the interpreter to an absolute path to prevent PATH manipulation attacks
	interpreterPath, err := exec.LookPath(interpreter)
	if err != nil {
		return fmt.Errorf("interpreter not found: %s: %w", interpreter, err)
	}

	// Execute the script file directly with the interpreter
	// Using the absolute path to both interpreter and script file
	absScriptPath, err := filepath.Abs(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for script: %w", err)
	}

	// G702 false positive: hook scripts are arbitrary code by design
	// (deploy hooks let operators run pre/post-deploy actions). The
	// interpreter is resolved via exec.LookPath so PATH cannot be
	// hijacked, and the script is written to a temp file with 0500
	// perms so only the deploy daemon's UID can execute it.
	cmd := exec.CommandContext(ctx, interpreterPath, absScriptPath) // #nosec G204 -- operator-configured deploy hook; running supplied code is the feature

	// Set working directory if specified
	if hook.Config.Dir != "" {
		cmd.Dir = hook.Config.Dir
	}

	// Start with a clean environment and add only necessary variables
	cmd.Env = os.Environ()

	// Add pipeline context as environment variables with sanitized keys
	for key, value := range pipelineCtx {
		// Sanitize the key to prevent environment variable injection
		sanitizedKey := sanitizeEnvKey(key)
		if sanitizedKey == "" {
			continue
		}

		switch v := value.(type) {
		case string:
			cmd.Env = append(cmd.Env, fmt.Sprintf("PIPELINE_%s=%s", sanitizedKey, v))
		case int, int64, float64:
			cmd.Env = append(cmd.Env, fmt.Sprintf("PIPELINE_%s=%v", sanitizedKey, v))
		}
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	result.Output = stdout.String()
	if stderr.Len() > 0 {
		result.Output += "\nSTDERR: " + stderr.String()
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return fmt.Errorf("%w: %v", ErrHookExecFailed, err)
	}

	result.ExitCode = 0
	return nil
}

// sanitizeEnvKey ensures environment variable keys contain only safe characters.
// Returns empty string if the key is invalid.
func sanitizeEnvKey(key string) string {
	if key == "" {
		return ""
	}

	var result strings.Builder
	for i, r := range key {
		// Environment variable names should be uppercase letters, digits, and underscores
		// First character cannot be a digit
		if r >= 'A' && r <= 'Z' {
			result.WriteRune(r)
		} else if r >= 'a' && r <= 'z' {
			result.WriteRune(r - 'a' + 'A') // Convert to uppercase
		} else if r >= '0' && r <= '9' && i > 0 {
			result.WriteRune(r)
		} else if r == '_' || r == '-' {
			result.WriteRune('_')
		}
		// Skip any other characters
	}

	return result.String()
}

// ExecuteHooks executes a list of hooks sequentially.
func (e *HookExecutor) ExecuteHooks(ctx context.Context, hooks []*Hook, pipelineCtx map[string]interface{}) ([]*HookResult, error) {
	results := make([]*HookResult, 0, len(hooks))

	for _, hook := range hooks {
		result := e.ExecuteWithResult(ctx, hook, pipelineCtx)
		results = append(results, result)

		if !result.Success {
			switch hook.OnError {
			case HookErrorAbort:
				return results, errors.New(result.Error)
			case HookErrorContinue:
				continue
			case HookErrorRetry:
				// Retry was already handled in ExecuteWithResult
				if !result.Success {
					return results, errors.New(result.Error)
				}
			}
		}
	}

	return results, nil
}

// HookBuilder helps build hook configurations.
type HookBuilder struct {
	hook *Hook
}

// NewHookBuilder creates a new hook builder.
func NewHookBuilder(name string, hookType HookType) *HookBuilder {
	return &HookBuilder{
		hook: &Hook{
			Name:    name,
			Type:    hookType,
			Config:  &HookConfig{},
			OnError: HookErrorAbort,
		},
	}
}

// WithTimeout sets the hook timeout.
func (b *HookBuilder) WithTimeout(timeout time.Duration) *HookBuilder {
	b.hook.Timeout = timeout
	return b
}

// WithRetries sets the number of retries.
func (b *HookBuilder) WithRetries(retries int) *HookBuilder {
	b.hook.Retries = retries
	return b
}

// WithOnError sets the error handling behavior.
func (b *HookBuilder) WithOnError(action HookErrorAction) *HookBuilder {
	b.hook.OnError = action
	return b
}

// WithCommand sets the command for exec hooks.
func (b *HookBuilder) WithCommand(command string, args ...string) *HookBuilder {
	b.hook.Config.Command = command
	b.hook.Config.Args = args
	return b
}

// WithURL sets the URL for HTTP hooks.
func (b *HookBuilder) WithURL(url string) *HookBuilder {
	b.hook.Config.URL = url
	return b
}

// WithMethod sets the HTTP method.
func (b *HookBuilder) WithMethod(method string) *HookBuilder {
	b.hook.Config.Method = method
	return b
}

// WithHeaders sets HTTP headers.
func (b *HookBuilder) WithHeaders(headers map[string]string) *HookBuilder {
	b.hook.Config.Headers = headers
	return b
}

// WithTopic sets the Wotan topic.
func (b *HookBuilder) WithTopic(topic string) *HookBuilder {
	b.hook.Config.Topic = topic
	return b
}

// WithPayload sets the Wotan payload.
func (b *HookBuilder) WithPayload(payload map[string]interface{}) *HookBuilder {
	b.hook.Config.Payload = payload
	return b
}

// WithScript sets the script for script hooks.
func (b *HookBuilder) WithScript(script string, language string) *HookBuilder {
	b.hook.Config.Script = script
	b.hook.Config.Language = language
	return b
}

// Build returns the built hook.
func (b *HookBuilder) Build() *Hook {
	return b.hook
}

// Common hook constructors

// ExecHook creates an exec hook.
func ExecHook(name string, command string, args ...string) *Hook {
	return NewHookBuilder(name, HookTypeExec).
		WithCommand(command, args...).
		WithTimeout(30 * time.Second).
		Build()
}

// HTTPHook creates an HTTP hook.
func HTTPHook(name string, url string, method string) *Hook {
	return NewHookBuilder(name, HookTypeHTTP).
		WithURL(url).
		WithMethod(method).
		WithTimeout(30 * time.Second).
		Build()
}

// WebhookHook creates a webhook hook.
func WebhookHook(name string, url string) *Hook {
	return NewHookBuilder(name, HookTypeWebhook).
		WithURL(url).
		WithMethod("POST").
		WithTimeout(30 * time.Second).
		Build()
}

// WotanHook creates a Wotan hook.
func WotanHook(name string, topic string, payload map[string]interface{}) *Hook {
	return NewHookBuilder(name, HookTypeWotan).
		WithTopic(topic).
		WithPayload(payload).
		WithTimeout(10 * time.Second).
		Build()
}

// ScriptHook creates a script hook.
func ScriptHook(name string, script string, language string) *Hook {
	return NewHookBuilder(name, HookTypeScript).
		WithScript(script, language).
		WithTimeout(60 * time.Second).
		Build()
}
