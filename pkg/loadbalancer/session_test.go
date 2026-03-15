// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- MemorySessionStore tests ---

func TestMemorySessionStore_GetSet(t *testing.T) {
	store := NewMemorySessionStore(100)

	store.Set("key1", "backend1", time.Minute)

	name, found := store.Get("key1")
	if !found {
		t.Error("expected to find key1")
	}
	if name != "backend1" {
		t.Errorf("expected backend1, got %s", name)
	}
}

func TestMemorySessionStore_GetNotFound(t *testing.T) {
	store := NewMemorySessionStore(100)

	_, found := store.Get("nonexistent")
	if found {
		t.Error("expected not to find nonexistent key")
	}
}

func TestMemorySessionStore_GetExpired(t *testing.T) {
	store := NewMemorySessionStore(100)

	store.Set("key1", "backend1", time.Nanosecond)
	time.Sleep(time.Millisecond)

	_, found := store.Get("key1")
	if found {
		t.Error("expected expired entry to not be found")
	}
}

func TestMemorySessionStore_Delete(t *testing.T) {
	store := NewMemorySessionStore(100)

	store.Set("key1", "backend1", time.Minute)
	store.Delete("key1")

	_, found := store.Get("key1")
	if found {
		t.Error("expected deleted entry to not be found")
	}
}

func TestMemorySessionStore_Cleanup(t *testing.T) {
	store := NewMemorySessionStore(100)

	store.Set("expired", "backend1", time.Nanosecond)
	store.Set("valid", "backend2", time.Minute)
	time.Sleep(time.Millisecond)

	store.Cleanup()

	if store.Size() != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", store.Size())
	}

	_, found := store.Get("valid")
	if !found {
		t.Error("expected valid entry to survive cleanup")
	}
}

func TestMemorySessionStore_Size(t *testing.T) {
	store := NewMemorySessionStore(100)

	if store.Size() != 0 {
		t.Errorf("expected size 0, got %d", store.Size())
	}

	store.Set("k1", "b1", time.Minute)
	store.Set("k2", "b2", time.Minute)

	if store.Size() != 2 {
		t.Errorf("expected size 2, got %d", store.Size())
	}
}

func TestMemorySessionStore_MaxEntries_Eviction(t *testing.T) {
	store := NewMemorySessionStore(2)

	store.Set("k1", "b1", time.Minute)
	store.Set("k2", "b2", time.Minute)
	store.Set("k3", "b3", time.Minute) // Should trigger eviction

	if store.Size() != 2 {
		t.Errorf("expected size 2 after eviction, got %d", store.Size())
	}
}

func TestMemorySessionStore_MaxEntries_EvictsExpiredFirst(t *testing.T) {
	store := NewMemorySessionStore(2)

	store.Set("expired", "b1", time.Nanosecond)
	store.Set("valid", "b2", time.Minute)
	time.Sleep(time.Millisecond)

	store.Set("new", "b3", time.Minute) // Should evict expired entry first

	_, found := store.Get("valid")
	if !found {
		t.Error("expected valid entry to survive eviction")
	}
	_, found = store.Get("new")
	if !found {
		t.Error("expected new entry to exist")
	}
}

func TestMemorySessionStore_GenerateAndValidateSessionID(t *testing.T) {
	store := NewMemorySessionStore(100)

	sessionID := store.GenerateSessionID("backend1")
	if sessionID == "" {
		t.Error("expected non-empty session ID")
	}

	name, valid := store.ValidateSessionID(sessionID)
	if !valid {
		t.Error("expected session ID to be valid")
	}
	if name != "backend1" {
		t.Errorf("expected backend1, got %s", name)
	}
}

func TestMemorySessionStore_ValidateSessionID_Invalid(t *testing.T) {
	store := NewMemorySessionStore(100)

	_, valid := store.ValidateSessionID("invalid-base64")
	if valid {
		t.Error("expected invalid for bad session ID")
	}

	_, valid = store.ValidateSessionID("")
	if valid {
		t.Error("expected invalid for empty session ID")
	}
}

func TestMemorySessionStore_ConcurrentAccess(t *testing.T) {
	store := NewMemorySessionStore(10000)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			store.Set(key, "backend", time.Minute)
			store.Get(key)
			store.Size()
		}(i)
	}
	wg.Wait()
}

// --- ShardedSessionStore tests ---

func TestShardedSessionStore_Basic(t *testing.T) {
	store := NewShardedSessionStore(4, 100)

	store.Set("key1", "backend1", time.Minute)

	name, found := store.Get("key1")
	if !found {
		t.Error("expected to find key1")
	}
	if name != "backend1" {
		t.Errorf("expected backend1, got %s", name)
	}
}

func TestShardedSessionStore_Delete(t *testing.T) {
	store := NewShardedSessionStore(4, 100)

	store.Set("key1", "backend1", time.Minute)
	store.Delete("key1")

	_, found := store.Get("key1")
	if found {
		t.Error("expected deleted entry to not be found")
	}
}

func TestShardedSessionStore_Size(t *testing.T) {
	store := NewShardedSessionStore(4, 100)

	store.Set("k1", "b1", time.Minute)
	store.Set("k2", "b2", time.Minute)
	store.Set("k3", "b3", time.Minute)

	if store.Size() != 3 {
		t.Errorf("expected size 3, got %d", store.Size())
	}
}

func TestShardedSessionStore_Cleanup(t *testing.T) {
	store := NewShardedSessionStore(4, 100)

	store.Set("expired", "b1", time.Nanosecond)
	store.Set("valid", "b2", time.Minute)
	time.Sleep(time.Millisecond)

	store.Cleanup()

	if store.Size() != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", store.Size())
	}
}

func TestShardedSessionStore_PowerOfTwoShards(t *testing.T) {
	// numShards=3 should be rounded to 4
	store := NewShardedSessionStore(3, 100)

	// Verify it works with non-power-of-two input
	store.Set("k1", "b1", time.Minute)
	name, found := store.Get("k1")
	if !found || name != "b1" {
		t.Error("expected sharded store to work with non-power-of-two shard count")
	}
}

func TestShardedSessionStore_ConcurrentAccess(t *testing.T) {
	store := NewShardedSessionStore(8, 10000)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			store.Set(key, "backend", time.Minute)
			store.Get(key)
			store.Size()
		}(i)
	}
	wg.Wait()
}

// --- SessionAffinity tests ---

func TestSessionAffinity_GetSet(t *testing.T) {
	sa := NewSessionAffinity(time.Minute, 100)

	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	sa.Set("client1", b)

	got, found := sa.Get("client1")
	if !found {
		t.Error("expected to find client1")
	}
	if got != b {
		t.Error("expected same backend")
	}
}

func TestSessionAffinity_GetExpired(t *testing.T) {
	sa := NewSessionAffinity(time.Nanosecond, 100)

	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	sa.Set("client1", b)
	time.Sleep(time.Millisecond)

	_, found := sa.Get("client1")
	if found {
		t.Error("expected expired entry to not be found")
	}
}

func TestSessionAffinity_GetUnhealthyBackend(t *testing.T) {
	sa := NewSessionAffinity(time.Minute, 100)

	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	sa.Set("client1", b)
	b.SetState(BackendStateUnhealthy)

	_, found := sa.Get("client1")
	if found {
		t.Error("expected unhealthy backend to not be returned")
	}
}

func TestSessionAffinity_Delete(t *testing.T) {
	sa := NewSessionAffinity(time.Minute, 100)

	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	sa.Set("client1", b)
	sa.Delete("client1")

	_, found := sa.Get("client1")
	if found {
		t.Error("expected deleted entry to not be found")
	}
}

func TestSessionAffinity_MaxSize_Eviction(t *testing.T) {
	sa := NewSessionAffinity(time.Minute, 2)

	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	sa.Set("c1", b)
	sa.Set("c2", b)
	sa.Set("c3", b) // Should trigger eviction

	if sa.Size() != 2 {
		t.Errorf("expected size 2, got %d", sa.Size())
	}
}

func TestSessionAffinity_Cleanup(t *testing.T) {
	sa := NewSessionAffinity(time.Nanosecond, 100)

	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	sa.Set("c1", b)
	sa.Set("c2", b)
	time.Sleep(time.Millisecond)

	sa.Cleanup()

	if sa.Size() != 0 {
		t.Errorf("expected 0 after cleanup, got %d", sa.Size())
	}
}

func TestSessionAffinity_Touch(t *testing.T) {
	sa := NewSessionAffinity(time.Minute, 100)

	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	sa.Set("client1", b)
	sa.Touch("client1")

	// Just verify no panic and entry still exists
	_, found := sa.Get("client1")
	if !found {
		t.Error("expected entry to exist after touch")
	}
}

func TestSessionAffinity_Touch_Nonexistent(t *testing.T) {
	sa := NewSessionAffinity(time.Minute, 100)
	sa.Touch("nonexistent") // Should not panic
}

// --- SessionManager tests ---

func makeSessionManagerPool(t *testing.T) *BackendPool {
	t.Helper()
	c := DefaultConfig()
	c.Backends = []BackendConfig{
		{Name: "b1", Address: "127.0.0.1:8081", Weight: 1, Enabled: true},
		{Name: "b2", Address: "127.0.0.1:8082", Weight: 1, Enabled: true},
	}
	pool, err := NewBackendPool(c)
	if err != nil {
		t.Fatal(err)
	}
	// Mark backends healthy
	for _, b := range pool.All() {
		b.SetState(BackendStateHealthy)
	}
	return pool
}

func TestSessionManager_None(t *testing.T) {
	pool := makeSessionManagerPool(t)
	sm := NewSessionManager(SessionPersistenceConfig{Type: SessionPersistenceNone}, pool)

	req := httptest.NewRequest("GET", "/", nil)
	_, found := sm.GetBackend(req)
	if found {
		t.Error("expected no backend for none persistence")
	}
}

func TestSessionManager_SourceIP(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceSourceIP,
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	b1, _ := pool.Get("b1")
	w := httptest.NewRecorder()
	sm.SetBackend(req, w, b1)

	got, found := sm.GetBackend(req)
	if !found {
		t.Error("expected to find backend for source-IP session")
	}
	if got != b1 {
		t.Error("expected same backend from session")
	}
}

func TestSessionManager_SourceIP_XForwardedFor(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceSourceIP,
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")

	b1, _ := pool.Get("b1")
	w := httptest.NewRecorder()
	sm.SetBackend(req, w, b1)

	got, found := sm.GetBackend(req)
	if !found {
		t.Error("expected to find backend")
	}
	if got != b1 {
		t.Error("expected same backend")
	}
}

func TestSessionManager_SourceIP_XRealIP(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceSourceIP,
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.99")

	b1, _ := pool.Get("b1")
	w := httptest.NewRecorder()
	sm.SetBackend(req, w, b1)

	// Second request with same X-Real-IP
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	req2.Header.Set("X-Real-IP", "203.0.113.99")

	got, found := sm.GetBackend(req2)
	if !found {
		t.Error("expected to find backend via X-Real-IP")
	}
	if got != b1 {
		t.Error("expected same backend")
	}
}

func TestSessionManager_Header(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceHeader,
		HeaderName: "X-Session-ID",
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Session-ID", "sess123")

	b2, _ := pool.Get("b2")
	w := httptest.NewRecorder()
	sm.SetBackend(req, w, b2)

	got, found := sm.GetBackend(req)
	if !found {
		t.Error("expected to find backend for header session")
	}
	if got != b2 {
		t.Error("expected same backend")
	}
}

func TestSessionManager_GetBackend_BackendRemoved(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceSourceIP,
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	b1, _ := pool.Get("b1")
	w := httptest.NewRecorder()
	sm.SetBackend(req, w, b1)

	// Remove the backend from pool
	pool.Remove("b1")

	_, found := sm.GetBackend(req)
	if found {
		t.Error("expected not found when backend removed")
	}
}

func TestSessionManager_GetBackend_BackendUnhealthy(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceSourceIP,
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	b1, _ := pool.Get("b1")
	w := httptest.NewRecorder()
	sm.SetBackend(req, w, b1)

	b1.SetState(BackendStateUnhealthy)

	_, found := sm.GetBackend(req)
	if found {
		t.Error("expected not found when backend unhealthy")
	}
}

func TestSessionManager_Cookie(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceCookie,
		CookieName: "SERVERID",
		CookiePath: "/",
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	req := httptest.NewRequest("GET", "/", nil)
	b1, _ := pool.Get("b1")
	w := httptest.NewRecorder()
	sm.SetBackend(req, w, b1)

	// Extract cookie from response
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected cookie to be set")
	}

	// Make second request with cookie
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(cookies[0])

	got, found := sm.GetBackend(req2)
	if !found {
		t.Error("expected to find backend via cookie")
	}
	if got != b1 {
		t.Error("expected same backend from cookie session")
	}
}

func TestSessionManager_Stats(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceSourceIP,
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	stats := sm.Stats()
	if stats.ActiveSessions != 0 {
		t.Errorf("expected 0 active sessions, got %d", stats.ActiveSessions)
	}
	if stats.Type != SessionPersistenceSourceIP {
		t.Errorf("expected source-ip type, got %s", stats.Type)
	}
}

func TestSessionManager_StartStop(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type:       SessionPersistenceSourceIP,
		TTL:        time.Minute,
		MaxEntries: 100,
	}
	sm := NewSessionManager(config, pool)

	sm.Start()
	time.Sleep(10 * time.Millisecond)
	sm.Stop()
	// Just verify no panic
}

func TestSessionManager_DefaultMaxEntries(t *testing.T) {
	pool := makeSessionManagerPool(t)
	config := SessionPersistenceConfig{
		Type: SessionPersistenceSourceIP,
		TTL:  time.Minute,
	}
	sm := NewSessionManager(config, pool)
	_ = sm // Just verify it creates successfully
}

func TestSessionManager_SetBackend_NoneType_NoOp(t *testing.T) {
	pool := makeSessionManagerPool(t)
	sm := NewSessionManager(SessionPersistenceConfig{Type: SessionPersistenceNone}, pool)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	b1, _ := pool.Get("b1")

	sm.SetBackend(req, w, b1) // Should be no-op

	resp := w.Result()
	if len(resp.Cookies()) > 0 {
		t.Error("expected no cookies for none persistence")
	}
}

// --- IPHashSessionManager tests ---

func TestIPHashSessionManager_GetBackend(t *testing.T) {
	pool := makeSessionManagerPool(t)

	im := NewIPHashSessionManager(pool)

	b, err := im.GetBackend("192.168.1.1")
	if err != nil {
		t.Fatalf("GetBackend failed: %v", err)
	}
	if b == nil {
		t.Error("expected non-nil backend")
	}

	// Same IP should get same backend (consistent hash)
	b2, err := im.GetBackend("192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if b2 != b {
		t.Error("expected same backend for same IP")
	}
}

func TestIPHashSessionManager_NoHealthyBackends(t *testing.T) {
	pool := makeSessionManagerPool(t)
	for _, b := range pool.All() {
		b.SetState(BackendStateUnhealthy)
	}

	im := NewIPHashSessionManager(pool)
	_, err := im.GetBackend("192.168.1.1")
	if err != ErrNoHealthyBackends {
		t.Errorf("expected ErrNoHealthyBackends, got: %v", err)
	}
}

// --- setCookie sameSite tests ---

func TestSessionManager_CookieSameSite(t *testing.T) {
	pool := makeSessionManagerPool(t)

	tests := []struct {
		sameSite string
		want     http.SameSite
	}{
		{"strict", http.SameSiteStrictMode},
		{"lax", http.SameSiteLaxMode},
		{"none", http.SameSiteNoneMode},
		{"", 0}, // default
	}

	for _, tt := range tests {
		t.Run(tt.sameSite, func(t *testing.T) {
			config := SessionPersistenceConfig{
				Type:           SessionPersistenceCookie,
				CookieName:     "SRV",
				CookiePath:     "/",
				CookieSameSite: tt.sameSite,
				TTL:            time.Minute,
				MaxEntries:     100,
			}
			sm := NewSessionManager(config, pool)

			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			b1, _ := pool.Get("b1")
			sm.SetBackend(req, w, b1)

			resp := w.Result()
			cookies := resp.Cookies()
			if len(cookies) == 0 {
				t.Fatal("expected cookie")
			}
			if cookies[0].SameSite != tt.want {
				t.Errorf("expected SameSite %v, got %v", tt.want, cookies[0].SameSite)
			}
		})
	}
}
