package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// APIKeyConfig configures the API key authenticator.
type APIKeyConfig struct {
	// Keys is the list of valid API keys (current first, then previous for rotation).
	// If empty, KeyFile is used instead.
	Keys []string

	// KeyFile is the path to a file containing API keys (one per line).
	// The first non-empty line is the current key, subsequent lines are
	// rotated (previous) keys that are still accepted.
	KeyFile string

	// HeaderName is the HTTP header to extract the key from.
	// Defaults to "X-API-Key".
	HeaderName string

	// DefaultRoles are the roles assigned to API key authenticated identities.
	DefaultRoles []string

	// RateLimit configures per-key rate limiting.
	// Zero = no rate limit.
	RateLimitPerMinute int
}

// APIKeyAuthenticator validates API keys from the X-API-Key header.
type APIKeyAuthenticator struct {
	// keys stores SHA-256 hashes of valid keys (never store plaintext)
	keys         [][]byte
	headerName   string
	defaultRoles []string

	// Rate limiting
	rateLimitPerMinute int
	rateMu             sync.Mutex
	rateCounters       map[string]*rateCounter
}

// rateCounter tracks request counts per key hash for rate limiting.
type rateCounter struct {
	count    int
	windowAt time.Time
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
func NewAPIKeyAuthenticator(cfg APIKeyConfig) (*APIKeyAuthenticator, error) {
	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "X-API-Key"
	}

	defaultRoles := cfg.DefaultRoles
	if len(defaultRoles) == 0 {
		defaultRoles = []string{RoleService}
	}

	a := &APIKeyAuthenticator{
		headerName:         headerName,
		defaultRoles:       defaultRoles,
		rateLimitPerMinute: cfg.RateLimitPerMinute,
		rateCounters:       make(map[string]*rateCounter),
	}

	// Load keys from config or file
	var rawKeys []string
	if len(cfg.Keys) > 0 {
		rawKeys = cfg.Keys
	} else if cfg.KeyFile != "" {
		loaded, err := loadKeysFromFile(cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load keys from %s: %w", cfg.KeyFile, err)
		}
		rawKeys = loaded
	}

	if len(rawKeys) == 0 {
		return nil, fmt.Errorf("no API keys configured")
	}

	// Hash all keys (never store plaintext)
	for _, key := range rawKeys {
		if key == "" {
			continue
		}
		a.keys = append(a.keys, hashKey(key))
	}

	if len(a.keys) == 0 {
		return nil, fmt.Errorf("no valid API keys after filtering")
	}

	return a, nil
}

// Authenticate validates the API key from the request header.
func (a *APIKeyAuthenticator) Authenticate(_ context.Context, r *http.Request) (*Identity, error) {
	key := r.Header.Get(a.headerName)
	if key == "" {
		return nil, ErrUnauthenticated
	}

	keyHash := hashKey(key)

	// Check if key matches any of the configured keys
	matched := false
	for _, validHash := range a.keys {
		if hmac.Equal(keyHash, validHash) {
			matched = true
			break
		}
	}

	if !matched {
		return nil, fmt.Errorf("%w: invalid API key", ErrUnauthenticated)
	}

	// Rate limiting check
	if a.rateLimitPerMinute > 0 {
		keyID := fmt.Sprintf("%x", keyHash[:8]) // use first 8 bytes as identifier
		if !a.checkRateLimit(keyID) {
			return nil, ErrRateLimited
		}
	}

	return &Identity{
		Subject: "apikey",
		Roles:   a.defaultRoles,
		Extra:   map[string]string{"auth_method": "apikey"},
	}, nil
}

// checkRateLimit returns true if the request is within rate limits.
func (a *APIKeyAuthenticator) checkRateLimit(keyID string) bool {
	a.rateMu.Lock()
	defer a.rateMu.Unlock()

	now := time.Now()
	counter, exists := a.rateCounters[keyID]

	if !exists || now.Sub(counter.windowAt) > time.Minute {
		// New window
		a.rateCounters[keyID] = &rateCounter{
			count:    1,
			windowAt: now,
		}
		return true
	}

	counter.count++
	return counter.count <= a.rateLimitPerMinute
}

// loadKeysFromFile reads API keys from a file (one per line).
func loadKeysFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keys = append(keys, line)
	}

	return keys, nil
}

// hashKey returns the SHA-256 hash of a key.
func hashKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}
