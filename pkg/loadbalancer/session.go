// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SessionStore manages session persistence.
type SessionStore interface {
	// Get retrieves the backend name for a session.
	Get(key string) (backendName string, found bool)
	// Set stores a session-to-backend mapping.
	Set(key, backendName string, ttl time.Duration)
	// Delete removes a session.
	Delete(key string)
	// Cleanup removes expired sessions.
	Cleanup()
	// Size returns the number of stored sessions.
	Size() int
}

// MemorySessionStore is an in-memory session store.
type MemorySessionStore struct {
	mu         sync.RWMutex
	sessions   map[string]*sessionEntry
	maxEntries int
	secretKey  []byte
}

type sessionEntry struct {
	backendName string
	expiresAt   time.Time
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore(maxEntries int) *MemorySessionStore {
	key := make([]byte, 32)
	rand.Read(key)

	return &MemorySessionStore{
		sessions:   make(map[string]*sessionEntry),
		maxEntries: maxEntries,
		secretKey:  key,
	}
}

// Get retrieves the backend name for a session.
func (m *MemorySessionStore) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sessions[key]
	if !ok {
		return "", false
	}

	if time.Now().After(entry.expiresAt) {
		return "", false
	}

	return entry.backendName, true
}

// Set stores a session-to-backend mapping.
func (m *MemorySessionStore) Set(key, backendName string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check size limit
	if len(m.sessions) >= m.maxEntries {
		// Evict oldest entry
		m.evictOldest()
	}

	m.sessions[key] = &sessionEntry{
		backendName: backendName,
		expiresAt:   time.Now().Add(ttl),
	}
}

// evictOldest removes the oldest expired entry, or oldest entry if none expired.
func (m *MemorySessionStore) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	now := time.Now()

	for key, entry := range m.sessions {
		// Prefer expired entries
		if now.After(entry.expiresAt) {
			delete(m.sessions, key)
			return
		}

		if oldestKey == "" || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
		}
	}

	if oldestKey != "" {
		delete(m.sessions, oldestKey)
	}
}

// Delete removes a session.
func (m *MemorySessionStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, key)
}

// Cleanup removes expired sessions.
func (m *MemorySessionStore) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, entry := range m.sessions {
		if now.After(entry.expiresAt) {
			delete(m.sessions, key)
		}
	}
}

// Size returns the number of stored sessions.
func (m *MemorySessionStore) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// GenerateSessionID generates a signed session ID.
func (m *MemorySessionStore) GenerateSessionID(backendName string) string {
	// Random bytes
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)

	// Create payload
	payload := fmt.Sprintf("%s:%s", backendName, hex.EncodeToString(randomBytes))

	// Sign with HMAC
	mac := hmac.New(sha256.New, m.secretKey)
	mac.Write([]byte(payload))
	signature := mac.Sum(nil)

	// Combine payload and signature
	combined := fmt.Sprintf("%s:%s", payload, hex.EncodeToString(signature[:8]))
	return base64.URLEncoding.EncodeToString([]byte(combined))
}

// ValidateSessionID validates a session ID and extracts the backend name.
func (m *MemorySessionStore) ValidateSessionID(sessionID string) (string, bool) {
	decoded, err := base64.URLEncoding.DecodeString(sessionID)
	if err != nil {
		return "", false
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return "", false
	}

	payload := parts[0] + ":" + parts[1]
	signature, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", false
	}

	// Verify signature
	mac := hmac.New(sha256.New, m.secretKey)
	mac.Write([]byte(payload))
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(signature, expectedSig[:8]) {
		return "", false
	}

	return parts[0], true
}

// SessionManager manages session persistence for load balancing.
type SessionManager struct {
	config SessionPersistenceConfig
	store  SessionStore
	pool   *BackendPool

	// For LRU cleanup
	cleanupTicker *time.Ticker
	stopCh        chan struct{}
}

// NewSessionManager creates a new session manager.
func NewSessionManager(config SessionPersistenceConfig, pool *BackendPool) *SessionManager {
	var store SessionStore
	if config.MaxEntries > 0 {
		store = NewMemorySessionStore(config.MaxEntries)
	} else {
		store = NewMemorySessionStore(100000)
	}

	sm := &SessionManager{
		config: config,
		store:  store,
		pool:   pool,
		stopCh: make(chan struct{}),
	}

	return sm
}

// Start starts the session manager's cleanup routine.
func (sm *SessionManager) Start() {
	// Cleanup every TTL/10 or every minute, whichever is larger
	cleanupInterval := sm.config.TTL / 10
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}

	sm.cleanupTicker = time.NewTicker(cleanupInterval)

	go func() {
		for {
			select {
			case <-sm.stopCh:
				return
			case <-sm.cleanupTicker.C:
				sm.store.Cleanup()
			}
		}
	}()
}

// Stop stops the session manager.
func (sm *SessionManager) Stop() {
	close(sm.stopCh)
	if sm.cleanupTicker != nil {
		sm.cleanupTicker.Stop()
	}
}

// GetBackend returns the backend for a request based on session persistence.
func (sm *SessionManager) GetBackend(r *http.Request) (*Backend, bool) {
	if sm.config.Type == SessionPersistenceNone {
		return nil, false
	}

	key := sm.extractKey(r)
	if key == "" {
		return nil, false
	}

	backendName, found := sm.store.Get(key)
	if !found {
		return nil, false
	}

	backend, ok := sm.pool.Get(backendName)
	if !ok {
		// Backend no longer exists
		sm.store.Delete(key)
		return nil, false
	}

	// Check if backend is healthy
	if !backend.IsHealthy() {
		return nil, false
	}

	return backend, true
}

// SetBackend associates a request with a backend.
func (sm *SessionManager) SetBackend(r *http.Request, w http.ResponseWriter, backend *Backend) {
	if sm.config.Type == SessionPersistenceNone {
		return
	}

	key := sm.extractKey(r)

	// For cookie persistence, set the cookie
	if sm.config.Type == SessionPersistenceCookie {
		memStore, ok := sm.store.(*MemorySessionStore)
		if ok {
			sessionID := memStore.GenerateSessionID(backend.Config.Name)
			sm.setCookie(w, sessionID)
			key = sessionID
		}
	}

	if key != "" {
		sm.store.Set(key, backend.Config.Name, sm.config.TTL)
	}
}

// extractKey extracts the session key from a request.
func (sm *SessionManager) extractKey(r *http.Request) string {
	switch sm.config.Type {
	case SessionPersistenceCookie:
		return sm.extractCookieKey(r)
	case SessionPersistenceSourceIP:
		return sm.extractIPKey(r)
	case SessionPersistenceHeader:
		return sm.extractHeaderKey(r)
	default:
		return ""
	}
}

// extractCookieKey extracts the session key from a cookie.
func (sm *SessionManager) extractCookieKey(r *http.Request) string {
	cookie, err := r.Cookie(sm.config.CookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// extractIPKey extracts the session key from the client IP.
func (sm *SessionManager) extractIPKey(r *http.Request) string {
	// Try X-Forwarded-For first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Try X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// extractHeaderKey extracts the session key from a header.
func (sm *SessionManager) extractHeaderKey(r *http.Request) string {
	return r.Header.Get(sm.config.HeaderName)
}

// setCookie sets the session cookie on the response.
func (sm *SessionManager) setCookie(w http.ResponseWriter, value string) {
	cookie := &http.Cookie{
		Name:     sm.config.CookieName,
		Value:    value,
		Path:     sm.config.CookiePath,
		MaxAge:   int(sm.config.TTL.Seconds()),
		HttpOnly: sm.config.CookieHTTPOnly,
		Secure:   sm.config.CookieSecure,
	}

	switch strings.ToLower(sm.config.CookieSameSite) {
	case "strict":
		cookie.SameSite = http.SameSiteStrictMode
	case "lax":
		cookie.SameSite = http.SameSiteLaxMode
	case "none":
		cookie.SameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, cookie)
}

// Stats returns session store statistics.
func (sm *SessionManager) Stats() SessionStats {
	return SessionStats{
		ActiveSessions: sm.store.Size(),
		Type:           sm.config.Type,
	}
}

// SessionStats holds session statistics.
type SessionStats struct {
	ActiveSessions int
	Type           SessionPersistenceType
}

// IPHashSessionManager provides IP-hash based session persistence.
// Unlike the cookie-based approach, this doesn't require storing state.
type IPHashSessionManager struct {
	pool *BackendPool
}

// NewIPHashSessionManager creates a new IP hash session manager.
func NewIPHashSessionManager(pool *BackendPool) *IPHashSessionManager {
	return &IPHashSessionManager{pool: pool}
}

// GetBackend returns the backend for a client IP using consistent hashing.
func (im *IPHashSessionManager) GetBackend(clientIP string) (*Backend, error) {
	healthy := im.pool.Healthy()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	// Hash the client IP
	h := fnv.New32a()
	h.Write([]byte(clientIP))
	hash := h.Sum32()

	// Select backend based on hash
	idx := int(hash) % len(healthy)
	return healthy[idx], nil
}

// ShardedSessionStore distributes sessions across multiple shards for better concurrency.
type ShardedSessionStore struct {
	shards    []*MemorySessionStore
	shardMask uint32
}

// NewShardedSessionStore creates a new sharded session store.
func NewShardedSessionStore(numShards, maxEntriesPerShard int) *ShardedSessionStore {
	// Round up to power of 2 for efficient modulo
	n := uint32(1)
	for n < uint32(numShards) {
		n *= 2
	}

	shards := make([]*MemorySessionStore, n)
	for i := range shards {
		shards[i] = NewMemorySessionStore(maxEntriesPerShard)
	}

	return &ShardedSessionStore{
		shards:    shards,
		shardMask: n - 1,
	}
}

// getShard returns the shard for a key.
func (ss *ShardedSessionStore) getShard(key string) *MemorySessionStore {
	h := fnv.New32a()
	h.Write([]byte(key))
	idx := h.Sum32() & ss.shardMask
	return ss.shards[idx]
}

// Get retrieves the backend name for a session.
func (ss *ShardedSessionStore) Get(key string) (string, bool) {
	return ss.getShard(key).Get(key)
}

// Set stores a session-to-backend mapping.
func (ss *ShardedSessionStore) Set(key, backendName string, ttl time.Duration) {
	ss.getShard(key).Set(key, backendName, ttl)
}

// Delete removes a session.
func (ss *ShardedSessionStore) Delete(key string) {
	ss.getShard(key).Delete(key)
}

// Cleanup removes expired sessions from all shards.
func (ss *ShardedSessionStore) Cleanup() {
	for _, shard := range ss.shards {
		shard.Cleanup()
	}
}

// Size returns the total number of stored sessions.
func (ss *ShardedSessionStore) Size() int {
	total := 0
	for _, shard := range ss.shards {
		total += shard.Size()
	}
	return total
}

// SessionAffinity tracks session affinity for L4 connections.
type SessionAffinity struct {
	mu       sync.RWMutex
	mappings map[string]*affinityEntry
	ttl      time.Duration
	maxSize  int
}

type affinityEntry struct {
	backend   *Backend
	expiresAt time.Time
	lastUsed  time.Time
}

// NewSessionAffinity creates a new session affinity tracker.
func NewSessionAffinity(ttl time.Duration, maxSize int) *SessionAffinity {
	return &SessionAffinity{
		mappings: make(map[string]*affinityEntry),
		ttl:      ttl,
		maxSize:  maxSize,
	}
}

// Get retrieves the backend for a client.
func (sa *SessionAffinity) Get(clientAddr string) (*Backend, bool) {
	sa.mu.RLock()
	entry, ok := sa.mappings[clientAddr]
	sa.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		sa.Delete(clientAddr)
		return nil, false
	}

	if !entry.backend.IsHealthy() {
		return nil, false
	}

	// Update last used
	sa.mu.Lock()
	if e, ok := sa.mappings[clientAddr]; ok {
		e.lastUsed = time.Now()
	}
	sa.mu.Unlock()

	return entry.backend, true
}

// Set associates a client with a backend.
func (sa *SessionAffinity) Set(clientAddr string, backend *Backend) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	// Check size limit
	if len(sa.mappings) >= sa.maxSize {
		sa.evictOldest()
	}

	now := time.Now()
	sa.mappings[clientAddr] = &affinityEntry{
		backend:   backend,
		expiresAt: now.Add(sa.ttl),
		lastUsed:  now,
	}
}

// Delete removes a client mapping.
func (sa *SessionAffinity) Delete(clientAddr string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	delete(sa.mappings, clientAddr)
}

// evictOldest removes the least recently used entry.
func (sa *SessionAffinity) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	now := time.Now()

	for key, entry := range sa.mappings {
		// Prefer expired entries
		if now.After(entry.expiresAt) {
			delete(sa.mappings, key)
			return
		}

		if oldestKey == "" || entry.lastUsed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastUsed
		}
	}

	if oldestKey != "" {
		delete(sa.mappings, oldestKey)
	}
}

// Cleanup removes expired mappings.
func (sa *SessionAffinity) Cleanup() {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	now := time.Now()
	for key, entry := range sa.mappings {
		if now.After(entry.expiresAt) {
			delete(sa.mappings, key)
		}
	}
}

// Size returns the number of active mappings.
func (sa *SessionAffinity) Size() int {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return len(sa.mappings)
}

// Touch updates the last used time for a client.
func (sa *SessionAffinity) Touch(clientAddr string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if entry, ok := sa.mappings[clientAddr]; ok {
		entry.lastUsed = time.Now()
		entry.expiresAt = time.Now().Add(sa.ttl)
	}
}
