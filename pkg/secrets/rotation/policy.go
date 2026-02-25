// Package rotation provides secret rotation functionality for the Crystal Grotto.
package rotation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/secrets"
)

// Common policy errors.
var (
	ErrPolicyNotFound     = errors.New("rotation policy not found")
	ErrPolicyViolation    = errors.New("rotation policy violation")
	ErrInvalidPolicy      = errors.New("invalid rotation policy")
	ErrUsageLimitExceeded = errors.New("usage limit exceeded")
)

// PolicyType represents the type of rotation policy.
type PolicyType string

const (
	// PolicyTypeTimeBased triggers rotation based on time intervals.
	PolicyTypeTimeBased PolicyType = "time_based"

	// PolicyTypeUsageBased triggers rotation based on usage count.
	PolicyTypeUsageBased PolicyType = "usage_based"

	// PolicyTypeHybrid combines time and usage-based policies.
	PolicyTypeHybrid PolicyType = "hybrid"

	// PolicyTypeOnDemand only rotates when explicitly requested.
	PolicyTypeOnDemand PolicyType = "on_demand"

	// PolicyTypeExpiry rotates before secret expiry.
	PolicyTypeExpiry PolicyType = "expiry"
)

// Policy defines rotation policy for a secret.
type Policy struct {
	// ID is the unique policy identifier.
	ID string `json:"id"`

	// Name is a human-readable name for the policy.
	Name string `json:"name"`

	// Type is the policy type.
	Type PolicyType `json:"type"`

	// SecretPath is the path pattern this policy applies to (supports wildcards).
	SecretPath string `json:"secret_path"`

	// TimeConfig contains time-based rotation settings.
	TimeConfig *TimeBasedConfig `json:"time_config,omitempty"`

	// UsageConfig contains usage-based rotation settings.
	UsageConfig *UsageBasedConfig `json:"usage_config,omitempty"`

	// ExpiryConfig contains expiry-based rotation settings.
	ExpiryConfig *ExpiryBasedConfig `json:"expiry_config,omitempty"`

	// RotationConfig is the configuration to use when rotating.
	RotationConfig RotationConfig `json:"rotation_config"`

	// Enabled indicates if this policy is active.
	Enabled bool `json:"enabled"`

	// Priority determines which policy takes precedence (higher = more priority).
	Priority int `json:"priority"`

	// CreatedAt is when the policy was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the policy was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// TimeBasedConfig configures time-based rotation.
type TimeBasedConfig struct {
	// RotationInterval is how often to rotate.
	RotationInterval time.Duration `json:"rotation_interval"`

	// MinAge is the minimum age before rotation is allowed.
	MinAge time.Duration `json:"min_age"`

	// MaxAge is the maximum age after which rotation is required.
	MaxAge time.Duration `json:"max_age"`

	// RotationWindow specifies allowed rotation times (optional).
	RotationWindow *RotationWindow `json:"rotation_window,omitempty"`
}

// RotationWindow defines when rotations are allowed.
type RotationWindow struct {
	// DaysOfWeek specifies allowed days (0=Sunday, 6=Saturday).
	DaysOfWeek []int `json:"days_of_week"`

	// StartHour is the start of the rotation window (0-23).
	StartHour int `json:"start_hour"`

	// EndHour is the end of the rotation window (0-23).
	EndHour int `json:"end_hour"`

	// Timezone is the timezone for the window.
	Timezone string `json:"timezone"`
}

// UsageBasedConfig configures usage-based rotation.
type UsageBasedConfig struct {
	// MaxUsageCount is the maximum number of uses before rotation.
	MaxUsageCount int64 `json:"max_usage_count"`

	// UsageWindow is the time window for counting usage.
	UsageWindow time.Duration `json:"usage_window"`

	// ResetOnRotation indicates if usage count resets after rotation.
	ResetOnRotation bool `json:"reset_on_rotation"`
}

// ExpiryBasedConfig configures expiry-based rotation.
type ExpiryBasedConfig struct {
	// RotateBeforeExpiry is how long before expiry to trigger rotation.
	RotateBeforeExpiry time.Duration `json:"rotate_before_expiry"`

	// MinRemainingValidity is the minimum validity period after rotation.
	MinRemainingValidity time.Duration `json:"min_remaining_validity"`
}

// UsageTracker tracks secret usage for usage-based policies.
type UsageTracker struct {
	counts     map[string]*usageCount
	mu         sync.RWMutex
	cleanupTTL time.Duration
}

type usageCount struct {
	count     atomic.Int64
	windowEnd time.Time
	lastUsed  time.Time
}

// NewUsageTracker creates a new usage tracker.
func NewUsageTracker(cleanupTTL time.Duration) *UsageTracker {
	if cleanupTTL <= 0 {
		cleanupTTL = time.Hour
	}

	ut := &UsageTracker{
		counts:     make(map[string]*usageCount),
		cleanupTTL: cleanupTTL,
	}

	go ut.cleanup()
	return ut
}

// IncrementUsage increments the usage count for a secret.
func (ut *UsageTracker) IncrementUsage(path string, window time.Duration) int64 {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	uc, ok := ut.counts[path]
	if !ok {
		uc = &usageCount{
			windowEnd: time.Now().Add(window),
		}
		ut.counts[path] = uc
	}

	// Check if window expired
	now := time.Now()
	if now.After(uc.windowEnd) {
		uc.count.Store(0)
		uc.windowEnd = now.Add(window)
	}

	uc.lastUsed = now
	return uc.count.Add(1)
}

// GetUsage returns the current usage count for a secret.
func (ut *UsageTracker) GetUsage(path string) int64 {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	uc, ok := ut.counts[path]
	if !ok {
		return 0
	}

	// Check if window expired
	if time.Now().After(uc.windowEnd) {
		return 0
	}

	return uc.count.Load()
}

// ResetUsage resets the usage count for a secret.
func (ut *UsageTracker) ResetUsage(path string) {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	if uc, ok := ut.counts[path]; ok {
		uc.count.Store(0)
		uc.windowEnd = time.Now().Add(ut.cleanupTTL)
	}
}

// cleanup removes expired usage counts.
func (ut *UsageTracker) cleanup() {
	ticker := time.NewTicker(ut.cleanupTTL)
	defer ticker.Stop()

	for range ticker.C {
		ut.mu.Lock()
		now := time.Now()
		for path, uc := range ut.counts {
			if now.Sub(uc.lastUsed) > ut.cleanupTTL {
				delete(ut.counts, path)
			}
		}
		ut.mu.Unlock()
	}
}

// PolicyManager manages rotation policies.
type PolicyManager struct {
	policies     map[string]*Policy
	pathPolicies map[string][]*Policy // path pattern -> policies
	usageTracker *UsageTracker
	store        secrets.SecretStore
	mu           sync.RWMutex
}

// NewPolicyManager creates a new policy manager.
func NewPolicyManager(store secrets.SecretStore) *PolicyManager {
	return &PolicyManager{
		policies:     make(map[string]*Policy),
		pathPolicies: make(map[string][]*Policy),
		usageTracker: NewUsageTracker(time.Hour),
		store:        store,
	}
}

// AddPolicy adds a rotation policy.
func (pm *PolicyManager) AddPolicy(policy *Policy) error {
	if err := pm.validatePolicy(policy); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = now
	}
	policy.UpdatedAt = now

	pm.policies[policy.ID] = policy
	pm.pathPolicies[policy.SecretPath] = append(pm.pathPolicies[policy.SecretPath], policy)

	// Sort by priority (highest first)
	policies := pm.pathPolicies[policy.SecretPath]
	for i := len(policies) - 1; i > 0; i-- {
		if policies[i].Priority > policies[i-1].Priority {
			policies[i], policies[i-1] = policies[i-1], policies[i]
		}
	}

	return nil
}

// RemovePolicy removes a rotation policy.
func (pm *PolicyManager) RemovePolicy(policyID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	policy, ok := pm.policies[policyID]
	if !ok {
		return ErrPolicyNotFound
	}

	delete(pm.policies, policyID)

	// Remove from path policies
	pathPolicies := pm.pathPolicies[policy.SecretPath]
	for i, p := range pathPolicies {
		if p.ID == policyID {
			pm.pathPolicies[policy.SecretPath] = append(pathPolicies[:i], pathPolicies[i+1:]...)
			break
		}
	}

	return nil
}

// GetPolicy retrieves a policy by ID.
func (pm *PolicyManager) GetPolicy(policyID string) (*Policy, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	policy, ok := pm.policies[policyID]
	if !ok {
		return nil, ErrPolicyNotFound
	}

	return policy, nil
}

// GetPoliciesForPath returns all policies applicable to a path.
func (pm *PolicyManager) GetPoliciesForPath(path string) []*Policy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []*Policy

	// Check exact match
	if policies, ok := pm.pathPolicies[path]; ok {
		result = append(result, policies...)
	}

	// Check wildcard matches
	for pattern, policies := range pm.pathPolicies {
		if pattern != path && matchPath(pattern, path) {
			result = append(result, policies...)
		}
	}

	return result
}

// ListPolicies returns all policies.
func (pm *PolicyManager) ListPolicies() []*Policy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	policies := make([]*Policy, 0, len(pm.policies))
	for _, p := range pm.policies {
		policies = append(policies, p)
	}
	return policies
}

// ShouldRotate checks if a secret should be rotated based on policies.
func (pm *PolicyManager) ShouldRotate(ctx context.Context, path string) (bool, *Policy, string) {
	policies := pm.GetPoliciesForPath(path)
	if len(policies) == 0 {
		return false, nil, ""
	}

	secret, err := pm.store.Get(ctx, path)
	if err != nil {
		return false, nil, ""
	}

	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}

		should, reason := pm.evaluatePolicy(secret, policy)
		if should {
			return true, policy, reason
		}
	}

	return false, nil, ""
}

// RecordUsage records a secret access for usage-based policies.
func (pm *PolicyManager) RecordUsage(path string) (bool, int64) {
	policies := pm.GetPoliciesForPath(path)

	for _, policy := range policies {
		if !policy.Enabled || policy.UsageConfig == nil {
			continue
		}

		count := pm.usageTracker.IncrementUsage(path, policy.UsageConfig.UsageWindow)
		if count >= policy.UsageConfig.MaxUsageCount {
			return true, count // Rotation needed
		}
		return false, count
	}

	return false, 0
}

// evaluatePolicy checks if a secret should be rotated based on a specific policy.
func (pm *PolicyManager) evaluatePolicy(secret *secrets.Secret, policy *Policy) (bool, string) {
	switch policy.Type {
	case PolicyTypeTimeBased:
		return pm.evaluateTimeBased(secret, policy)
	case PolicyTypeUsageBased:
		return pm.evaluateUsageBased(secret, policy)
	case PolicyTypeHybrid:
		if should, reason := pm.evaluateTimeBased(secret, policy); should {
			return should, reason
		}
		return pm.evaluateUsageBased(secret, policy)
	case PolicyTypeExpiry:
		return pm.evaluateExpiry(secret, policy)
	case PolicyTypeOnDemand:
		return false, ""
	default:
		return false, ""
	}
}

// evaluateTimeBased checks time-based rotation criteria.
func (pm *PolicyManager) evaluateTimeBased(secret *secrets.Secret, policy *Policy) (bool, string) {
	if policy.TimeConfig == nil {
		return false, ""
	}

	cfg := policy.TimeConfig
	now := time.Now()

	// Check rotation window
	if cfg.RotationWindow != nil && !pm.inRotationWindow(now, cfg.RotationWindow) {
		return false, ""
	}

	// Use RotatedAt if available, otherwise UpdatedAt
	lastRotation := secret.RotatedAt
	if lastRotation.IsZero() {
		lastRotation = secret.UpdatedAt
	}
	if lastRotation.IsZero() {
		lastRotation = secret.CreatedAt
	}

	age := now.Sub(lastRotation)

	// Check minimum age
	if cfg.MinAge > 0 && age < cfg.MinAge {
		return false, ""
	}

	// Check maximum age (required rotation)
	if cfg.MaxAge > 0 && age >= cfg.MaxAge {
		return true, fmt.Sprintf("secret age (%v) exceeds maximum age (%v)", age.Round(time.Minute), cfg.MaxAge)
	}

	// Check rotation interval
	if cfg.RotationInterval > 0 && age >= cfg.RotationInterval {
		return true, fmt.Sprintf("rotation interval (%v) reached", cfg.RotationInterval)
	}

	return false, ""
}

// evaluateUsageBased checks usage-based rotation criteria.
func (pm *PolicyManager) evaluateUsageBased(secret *secrets.Secret, policy *Policy) (bool, string) {
	if policy.UsageConfig == nil {
		return false, ""
	}

	cfg := policy.UsageConfig
	count := pm.usageTracker.GetUsage(secret.Path)

	if count >= cfg.MaxUsageCount {
		return true, fmt.Sprintf("usage count (%d) exceeds limit (%d)", count, cfg.MaxUsageCount)
	}

	return false, ""
}

// evaluateExpiry checks expiry-based rotation criteria.
func (pm *PolicyManager) evaluateExpiry(secret *secrets.Secret, policy *Policy) (bool, string) {
	if policy.ExpiryConfig == nil || secret.ExpiresAt.IsZero() {
		return false, ""
	}

	cfg := policy.ExpiryConfig
	now := time.Now()
	timeUntilExpiry := secret.ExpiresAt.Sub(now)

	if timeUntilExpiry <= cfg.RotateBeforeExpiry {
		return true, fmt.Sprintf("secret expires in %v (threshold: %v)", timeUntilExpiry.Round(time.Minute), cfg.RotateBeforeExpiry)
	}

	return false, ""
}

// inRotationWindow checks if the current time is within the rotation window.
func (pm *PolicyManager) inRotationWindow(t time.Time, window *RotationWindow) bool {
	// Parse timezone
	loc := time.UTC
	if window.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(window.Timezone)
		if err != nil {
			loc = time.UTC
		}
	}

	localTime := t.In(loc)

	// Check day of week
	if len(window.DaysOfWeek) > 0 {
		dayOK := false
		weekday := int(localTime.Weekday())
		for _, d := range window.DaysOfWeek {
			if d == weekday {
				dayOK = true
				break
			}
		}
		if !dayOK {
			return false
		}
	}

	// Check hour range
	hour := localTime.Hour()
	if window.StartHour <= window.EndHour {
		return hour >= window.StartHour && hour < window.EndHour
	}
	// Handle overnight windows (e.g., 22:00 - 06:00)
	return hour >= window.StartHour || hour < window.EndHour
}

// validatePolicy validates a policy configuration.
func (pm *PolicyManager) validatePolicy(policy *Policy) error {
	if policy.ID == "" {
		return errors.New("policy ID is required")
	}
	if policy.SecretPath == "" {
		return errors.New("secret path is required")
	}

	switch policy.Type {
	case PolicyTypeTimeBased:
		if policy.TimeConfig == nil {
			return errors.New("time_config is required for time-based policy")
		}
		if policy.TimeConfig.RotationInterval <= 0 && policy.TimeConfig.MaxAge <= 0 {
			return errors.New("rotation_interval or max_age must be positive")
		}
	case PolicyTypeUsageBased:
		if policy.UsageConfig == nil {
			return errors.New("usage_config is required for usage-based policy")
		}
		if policy.UsageConfig.MaxUsageCount <= 0 {
			return errors.New("max_usage_count must be positive")
		}
	case PolicyTypeHybrid:
		if policy.TimeConfig == nil && policy.UsageConfig == nil {
			return errors.New("at least one of time_config or usage_config is required for hybrid policy")
		}
	case PolicyTypeExpiry:
		if policy.ExpiryConfig == nil {
			return errors.New("expiry_config is required for expiry-based policy")
		}
		if policy.ExpiryConfig.RotateBeforeExpiry <= 0 {
			return errors.New("rotate_before_expiry must be positive")
		}
	case PolicyTypeOnDemand:
		// No additional validation needed
	default:
		return fmt.Errorf("unknown policy type: %s", policy.Type)
	}

	return nil
}

// OnRotationComplete handles post-rotation tasks for policies.
func (pm *PolicyManager) OnRotationComplete(path string, success bool) {
	if !success {
		return
	}

	policies := pm.GetPoliciesForPath(path)
	for _, policy := range policies {
		if policy.UsageConfig != nil && policy.UsageConfig.ResetOnRotation {
			pm.usageTracker.ResetUsage(path)
		}
	}
}

// matchPath checks if a path matches a pattern (supports * wildcard).
func matchPath(pattern, path string) bool {
	// Simple wildcard matching
	if pattern == "*" || pattern == "**" {
		return true
	}

	// Handle trailing wildcard
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(path) >= len(prefix) && path[:len(prefix)] == prefix
	}

	// Handle prefix wildcard
	if len(pattern) > 1 && pattern[0] == '*' {
		suffix := pattern[1:]
		return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
	}

	return pattern == path
}

// PolicySnapshot returns a JSON snapshot of all policies.
func (pm *PolicyManager) PolicySnapshot() ([]byte, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	policies := make([]*Policy, 0, len(pm.policies))
	for _, p := range pm.policies {
		policies = append(policies, p)
	}

	return json.MarshalIndent(policies, "", "  ")
}

// RestorePolicies restores policies from a JSON snapshot.
func (pm *PolicyManager) RestorePolicies(data []byte) error {
	var policies []*Policy
	if err := json.Unmarshal(data, &policies); err != nil {
		return fmt.Errorf("unmarshal policies: %w", err)
	}

	for _, policy := range policies {
		if err := pm.AddPolicy(policy); err != nil {
			return fmt.Errorf("add policy %s: %w", policy.ID, err)
		}
	}

	return nil
}
