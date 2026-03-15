// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rotation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/secrets"
)

// Common hook errors.
var (
	ErrHookFailed       = errors.New("hook execution failed")
	ErrHookTimeout      = errors.New("hook execution timed out")
	ErrVerificationFailed = errors.New("verification hook failed")
	ErrRollbackFailed   = errors.New("rollback failed")
)

// HookType represents the type of rotation hook.
type HookType string

const (
	// HookTypePreRotation runs before rotation starts.
	HookTypePreRotation HookType = "pre_rotation"

	// HookTypePostRotation runs after successful rotation.
	HookTypePostRotation HookType = "post_rotation"

	// HookTypeVerification verifies the new secret works.
	HookTypeVerification HookType = "verification"

	// HookTypeRollback runs when rotation fails.
	HookTypeRollback HookType = "rollback"

	// HookTypeNotification sends notifications about rotation events.
	HookTypeNotification HookType = "notification"
)

// HookResult contains the result of a hook execution.
type HookResult struct {
	// HookID is the hook that was executed.
	HookID string `json:"hook_id"`

	// Type is the hook type.
	Type HookType `json:"type"`

	// Success indicates if the hook succeeded.
	Success bool `json:"success"`

	// Error contains the error message if the hook failed.
	Error string `json:"error,omitempty"`

	// Duration is how long the hook took to execute.
	Duration time.Duration `json:"duration"`

	// Output contains any output from the hook.
	Output map[string]interface{} `json:"output,omitempty"`

	// Timestamp is when the hook was executed.
	Timestamp time.Time `json:"timestamp"`
}

// Hook is the interface for rotation hooks.
type Hook interface {
	// ID returns the hook identifier.
	ID() string

	// Type returns the hook type.
	Type() HookType

	// Execute runs the hook.
	Execute(ctx context.Context, event *RotationEvent) (*HookResult, error)

	// Priority returns the hook priority (higher = runs first).
	Priority() int
}

// RotationEvent contains information about a rotation operation.
type RotationEvent struct {
	// Path is the secret path.
	Path string `json:"path"`

	// SecretType is the type of secret.
	SecretType secrets.SecretType `json:"secret_type"`

	// OldSecret is the previous secret (nil if new).
	OldSecret *secrets.Secret `json:"old_secret,omitempty"`

	// NewSecret is the new secret (nil for pre-rotation).
	NewSecret *secrets.Secret `json:"new_secret,omitempty"`

	// Policy is the policy that triggered rotation (if any).
	Policy *Policy `json:"policy,omitempty"`

	// Reason is why rotation was triggered.
	Reason string `json:"reason"`

	// RotationID is a unique identifier for this rotation operation.
	RotationID string `json:"rotation_id"`

	// Timestamp is when the rotation started.
	Timestamp time.Time `json:"timestamp"`

	// Metadata contains additional context.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// HookManager manages rotation hooks.
type HookManager struct {
	hooks    map[string]Hook
	byType   map[HookType][]Hook
	log      *logger.Logger
	mu       sync.RWMutex
	timeout  time.Duration
}

// HookManagerConfig configures the hook manager.
type HookManagerConfig struct {
	// Logger is the logger to use.
	Logger *logger.Logger

	// DefaultTimeout is the default timeout for hook execution.
	DefaultTimeout time.Duration
}

// NewHookManager creates a new hook manager.
func NewHookManager(config HookManagerConfig) *HookManager {
	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	timeout := config.DefaultTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &HookManager{
		hooks:   make(map[string]Hook),
		byType:  make(map[HookType][]Hook),
		log:     log.With().Str("component", "hook-manager").Logger(),
		timeout: timeout,
	}
}

// RegisterHook registers a hook.
func (hm *HookManager) RegisterHook(hook Hook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.hooks[hook.ID()] = hook
	hm.byType[hook.Type()] = append(hm.byType[hook.Type()], hook)

	// Sort hooks by priority (highest first)
	hooks := hm.byType[hook.Type()]
	for i := len(hooks) - 1; i > 0; i-- {
		if hooks[i].Priority() > hooks[i-1].Priority() {
			hooks[i], hooks[i-1] = hooks[i-1], hooks[i]
		}
	}

	hm.log.Info().
		Str("hook_id", hook.ID()).
		Str("type", string(hook.Type())).
		Int("priority", hook.Priority()).
		Msg("hook registered")
}

// UnregisterHook removes a hook.
func (hm *HookManager) UnregisterHook(hookID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hook, ok := hm.hooks[hookID]
	if !ok {
		return
	}

	delete(hm.hooks, hookID)

	// Remove from byType
	typeHooks := hm.byType[hook.Type()]
	for i, h := range typeHooks {
		if h.ID() == hookID {
			hm.byType[hook.Type()] = append(typeHooks[:i], typeHooks[i+1:]...)
			break
		}
	}

	hm.log.Info().Str("hook_id", hookID).Msg("hook unregistered")
}

// ExecuteHooks executes all hooks of a given type.
func (hm *HookManager) ExecuteHooks(ctx context.Context, hookType HookType, event *RotationEvent) ([]*HookResult, error) {
	hm.mu.RLock()
	hooks := hm.byType[hookType]
	hm.mu.RUnlock()

	if len(hooks) == 0 {
		return nil, nil
	}

	results := make([]*HookResult, 0, len(hooks))
	var firstError error

	for _, hook := range hooks {
		result, err := hm.executeHook(ctx, hook, event)
		results = append(results, result)

		if err != nil && firstError == nil {
			firstError = err
		}

		// For pre-rotation and verification hooks, fail fast
		if !result.Success && (hookType == HookTypePreRotation || hookType == HookTypeVerification) {
			break
		}
	}

	return results, firstError
}

// executeHook executes a single hook with timeout.
func (hm *HookManager) executeHook(ctx context.Context, hook Hook, event *RotationEvent) (*HookResult, error) {
	start := time.Now()

	// Create timeout context
	hookCtx, cancel := context.WithTimeout(ctx, hm.timeout)
	defer cancel()

	resultCh := make(chan *HookResult, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := hook.Execute(hookCtx, event)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case result := <-resultCh:
		result.Duration = time.Since(start)
		result.Timestamp = start

		hm.log.Info().
			Str("hook_id", hook.ID()).
			Str("type", string(hook.Type())).
			Bool("success", result.Success).
			Dur("duration", result.Duration).
			Msg("hook executed")

		if !result.Success {
			return result, fmt.Errorf("%w: %s - %s", ErrHookFailed, hook.ID(), result.Error)
		}
		return result, nil

	case err := <-errCh:
		result := &HookResult{
			HookID:    hook.ID(),
			Type:      hook.Type(),
			Success:   false,
			Error:     err.Error(),
			Duration:  time.Since(start),
			Timestamp: start,
		}

		hm.log.Error().
			Err(err).
			Str("hook_id", hook.ID()).
			Str("type", string(hook.Type())).
			Msg("hook failed")

		return result, fmt.Errorf("%w: %s - %v", ErrHookFailed, hook.ID(), err)

	case <-hookCtx.Done():
		result := &HookResult{
			HookID:    hook.ID(),
			Type:      hook.Type(),
			Success:   false,
			Error:     "execution timed out",
			Duration:  time.Since(start),
			Timestamp: start,
		}

		hm.log.Warn().
			Str("hook_id", hook.ID()).
			Str("type", string(hook.Type())).
			Dur("timeout", hm.timeout).
			Msg("hook timed out")

		return result, ErrHookTimeout
	}
}

// ListHooks returns all registered hooks.
func (hm *HookManager) ListHooks() map[HookType][]string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[HookType][]string)
	for hookType, hooks := range hm.byType {
		for _, h := range hooks {
			result[hookType] = append(result[hookType], h.ID())
		}
	}
	return result
}

// ServiceNotifyHook notifies services before/after rotation.
type ServiceNotifyHook struct {
	id           string
	hookType     HookType
	priority     int
	services     []string
	notifyFunc   func(ctx context.Context, service string, event *RotationEvent) error
}

// NewServiceNotifyHook creates a new service notification hook.
func NewServiceNotifyHook(id string, hookType HookType, priority int, services []string, notifyFunc func(ctx context.Context, service string, event *RotationEvent) error) *ServiceNotifyHook {
	return &ServiceNotifyHook{
		id:         id,
		hookType:   hookType,
		priority:   priority,
		services:   services,
		notifyFunc: notifyFunc,
	}
}

func (h *ServiceNotifyHook) ID() string       { return h.id }
func (h *ServiceNotifyHook) Type() HookType   { return h.hookType }
func (h *ServiceNotifyHook) Priority() int    { return h.priority }

func (h *ServiceNotifyHook) Execute(ctx context.Context, event *RotationEvent) (*HookResult, error) {
	result := &HookResult{
		HookID:    h.id,
		Type:      h.hookType,
		Success:   true,
		Output:    make(map[string]interface{}),
		Timestamp: time.Now(),
	}

	notified := make([]string, 0, len(h.services))
	var notifyErrors []string

	for _, service := range h.services {
		if err := h.notifyFunc(ctx, service, event); err != nil {
			notifyErrors = append(notifyErrors, fmt.Sprintf("%s: %v", service, err))
		} else {
			notified = append(notified, service)
		}
	}

	result.Output["notified"] = notified
	result.Output["total"] = len(h.services)

	if len(notifyErrors) > 0 {
		result.Success = false
		result.Error = fmt.Sprintf("failed to notify services: %v", notifyErrors)
		return result, fmt.Errorf("notification failed: %v", notifyErrors)
	}

	return result, nil
}

// HTTPVerificationHook verifies a secret by making an HTTP request.
type HTTPVerificationHook struct {
	id           string
	priority     int
	verifyFunc   func(ctx context.Context, secret *secrets.Secret) error
}

// NewHTTPVerificationHook creates a new HTTP verification hook.
func NewHTTPVerificationHook(id string, priority int, verifyFunc func(ctx context.Context, secret *secrets.Secret) error) *HTTPVerificationHook {
	return &HTTPVerificationHook{
		id:         id,
		priority:   priority,
		verifyFunc: verifyFunc,
	}
}

func (h *HTTPVerificationHook) ID() string       { return h.id }
func (h *HTTPVerificationHook) Type() HookType   { return HookTypeVerification }
func (h *HTTPVerificationHook) Priority() int    { return h.priority }

func (h *HTTPVerificationHook) Execute(ctx context.Context, event *RotationEvent) (*HookResult, error) {
	result := &HookResult{
		HookID:    h.id,
		Type:      HookTypeVerification,
		Timestamp: time.Now(),
	}

	if event.NewSecret == nil {
		result.Success = false
		result.Error = "no new secret to verify"
		return result, ErrVerificationFailed
	}

	if err := h.verifyFunc(ctx, event.NewSecret); err != nil {
		result.Success = false
		result.Error = err.Error()
		return result, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}

	result.Success = true
	return result, nil
}

// CallbackHook executes a callback function.
type CallbackHook struct {
	id         string
	hookType   HookType
	priority   int
	callback   func(ctx context.Context, event *RotationEvent) error
}

// NewCallbackHook creates a new callback hook.
func NewCallbackHook(id string, hookType HookType, priority int, callback func(ctx context.Context, event *RotationEvent) error) *CallbackHook {
	return &CallbackHook{
		id:       id,
		hookType: hookType,
		priority: priority,
		callback: callback,
	}
}

func (h *CallbackHook) ID() string       { return h.id }
func (h *CallbackHook) Type() HookType   { return h.hookType }
func (h *CallbackHook) Priority() int    { return h.priority }

func (h *CallbackHook) Execute(ctx context.Context, event *RotationEvent) (*HookResult, error) {
	result := &HookResult{
		HookID:    h.id,
		Type:      h.hookType,
		Timestamp: time.Now(),
	}

	if err := h.callback(ctx, event); err != nil {
		result.Success = false
		result.Error = err.Error()
		return result, err
	}

	result.Success = true
	return result, nil
}

// RollbackManager manages rollback operations for failed rotations.
type RollbackManager struct {
	store    secrets.SecretStore
	log      *logger.Logger
	history  map[string][]*secrets.Secret // path -> previous secrets
	maxVersions int
	mu       sync.RWMutex
}

// RollbackManagerConfig configures the rollback manager.
type RollbackManagerConfig struct {
	// Store is the secret store.
	Store secrets.SecretStore

	// Logger is the logger to use.
	Logger *logger.Logger

	// MaxVersions is the maximum number of versions to keep.
	MaxVersions int
}

// NewRollbackManager creates a new rollback manager.
func NewRollbackManager(config RollbackManagerConfig) *RollbackManager {
	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	maxVersions := config.MaxVersions
	if maxVersions <= 0 {
		maxVersions = 5
	}

	return &RollbackManager{
		store:       config.Store,
		log:         log.With().Str("component", "rollback-manager").Logger(),
		history:     make(map[string][]*secrets.Secret),
		maxVersions: maxVersions,
	}
}

// SaveForRollback saves a secret version for potential rollback.
func (rm *RollbackManager) SaveForRollback(secret *secrets.Secret) {
	if secret == nil {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	clone := secret.Clone()
	rm.history[secret.Path] = append(rm.history[secret.Path], clone)

	// Trim to max versions
	if len(rm.history[secret.Path]) > rm.maxVersions {
		rm.history[secret.Path] = rm.history[secret.Path][1:]
	}

	rm.log.Debug().
		Str("path", secret.Path).
		Int64("version", secret.Version).
		Int("history_size", len(rm.history[secret.Path])).
		Msg("saved secret for rollback")
}

// Rollback restores the previous version of a secret.
func (rm *RollbackManager) Rollback(ctx context.Context, path string) (*secrets.Secret, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	history, ok := rm.history[path]
	if !ok || len(history) == 0 {
		return nil, fmt.Errorf("no rollback history for path: %s", path)
	}

	// Get the most recent saved version
	previous := history[len(history)-1]
	history = history[:len(history)-1]
	rm.history[path] = history

	// Restore to store
	if err := rm.store.Set(ctx, path, previous); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRollbackFailed, err)
	}

	rm.log.Info().
		Str("path", path).
		Int64("version", previous.Version).
		Msg("rolled back to previous version")

	return previous, nil
}

// GetRollbackHistory returns the rollback history for a path.
func (rm *RollbackManager) GetRollbackHistory(path string) []*secrets.Secret {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	history, ok := rm.history[path]
	if !ok {
		return nil
	}

	// Return clones
	result := make([]*secrets.Secret, len(history))
	for i, s := range history {
		result[i] = s.Clone()
	}
	return result
}

// ClearHistory clears the rollback history for a path.
func (rm *RollbackManager) ClearHistory(path string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.history, path)
	rm.log.Debug().Str("path", path).Msg("cleared rollback history")
}

// GracePeriodManager manages grace periods during rotation.
type GracePeriodManager struct {
	activePeriods map[string]*gracePeriod
	mu            sync.RWMutex
}

type gracePeriod struct {
	OldSecret *secrets.Secret
	NewSecret *secrets.Secret
	ExpiresAt time.Time
}

// NewGracePeriodManager creates a new grace period manager.
func NewGracePeriodManager() *GracePeriodManager {
	gpm := &GracePeriodManager{
		activePeriods: make(map[string]*gracePeriod),
	}
	go gpm.cleanup()
	return gpm
}

// StartGracePeriod starts a grace period where both old and new secrets are valid.
func (gpm *GracePeriodManager) StartGracePeriod(path string, old, new *secrets.Secret, duration time.Duration) {
	gpm.mu.Lock()
	defer gpm.mu.Unlock()

	gpm.activePeriods[path] = &gracePeriod{
		OldSecret: old.Clone(),
		NewSecret: new.Clone(),
		ExpiresAt: time.Now().Add(duration),
	}
}

// IsInGracePeriod checks if a path is in a grace period.
func (gpm *GracePeriodManager) IsInGracePeriod(path string) bool {
	gpm.mu.RLock()
	defer gpm.mu.RUnlock()

	gp, ok := gpm.activePeriods[path]
	if !ok {
		return false
	}

	return time.Now().Before(gp.ExpiresAt)
}

// GetGracePeriodSecrets returns both secrets during a grace period.
func (gpm *GracePeriodManager) GetGracePeriodSecrets(path string) (*secrets.Secret, *secrets.Secret, bool) {
	gpm.mu.RLock()
	defer gpm.mu.RUnlock()

	gp, ok := gpm.activePeriods[path]
	if !ok || time.Now().After(gp.ExpiresAt) {
		return nil, nil, false
	}

	return gp.OldSecret.Clone(), gp.NewSecret.Clone(), true
}

// EndGracePeriod ends a grace period early.
func (gpm *GracePeriodManager) EndGracePeriod(path string) {
	gpm.mu.Lock()
	defer gpm.mu.Unlock()

	delete(gpm.activePeriods, path)
}

// cleanup removes expired grace periods.
func (gpm *GracePeriodManager) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		gpm.mu.Lock()
		now := time.Now()
		for path, gp := range gpm.activePeriods {
			if now.After(gp.ExpiresAt) {
				delete(gpm.activePeriods, path)
			}
		}
		gpm.mu.Unlock()
	}
}
