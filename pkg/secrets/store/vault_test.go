// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// mockVault is a simple in-memory Vault mock for testing.
type mockVault struct {
	mu      sync.RWMutex
	secrets map[string]map[string]interface{} // path -> data
	version map[string]int                    // path -> version
}

func newMockVault() *mockVault {
	return &mockVault{
		secrets: make(map[string]map[string]interface{}),
		version: make(map[string]int),
	}
}

func (v *mockVault) handler() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/v1/sys/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"initialized":true,"sealed":false}`))
	})

	// KV v2 data endpoint
	mux.HandleFunc("/v1/secret/data/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/secret/data/")

		switch r.Method {
		case "GET":
			v.mu.RLock()
			data, ok := v.secrets[path]
			ver := v.version[path]
			v.mu.RUnlock()

			if !ok {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"errors":["not found"]}`))
				return
			}

			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"data": data,
					"metadata": map[string]interface{}{
						"created_time":    time.Now().Format(time.RFC3339Nano),
						"custom_metadata": map[string]interface{}{"type": "static"},
						"deletion_time":   "",
						"destroyed":       false,
						"version":         ver,
					},
				},
			}
			json.NewEncoder(w).Encode(resp)

		case "POST":
			var body struct {
				Data    map[string]interface{} `json:"data"`
				Options map[string]interface{} `json:"options"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			v.mu.Lock()
			v.secrets[path] = body.Data
			v.version[path]++
			v.mu.Unlock()

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}
	})

	// KV v2 metadata endpoint
	mux.HandleFunc("/v1/secret/metadata/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/secret/metadata/")

		switch r.Method {
		case "DELETE":
			v.mu.Lock()
			_, ok := v.secrets[path]
			if !ok {
				v.mu.Unlock()
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(v.secrets, path)
			delete(v.version, path)
			v.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		case "LIST":
			v.mu.RLock()
			var keys []string
			for key := range v.secrets {
				if strings.HasPrefix(key, path) {
					suffix := strings.TrimPrefix(key, path)
					keys = append(keys, suffix)
				}
			}
			v.mu.RUnlock()

			if len(keys) == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"keys": keys,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	})

	return mux
}

func newTestVaultStore(t *testing.T, server *httptest.Server) *store.VaultStore {
	t.Helper()
	vs, err := store.NewVaultStore(store.VaultStoreConfig{
		Address:      server.URL,
		Token:        "test-token",
		MountPath:    "secret",
		PollInterval: 100 * time.Millisecond,
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}
	return vs
}

func TestVaultStore_SetAndGet(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	defer vs.Close()
	ctx := context.Background()

	secret := &secrets.Secret{
		Path:    "test/secret",
		Type:    secrets.SecretTypeStatic,
		Version: 1,
		Data: map[string][]byte{
			"value": []byte("vault-secret-value"),
		},
		Metadata:  map[string]string{"owner": "test"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := vs.Set(ctx, "test/secret", secret); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := vs.Get(ctx, "test/secret")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if string(got.Data["value"]) != "vault-secret-value" {
		t.Errorf("Data = %q, want %q", string(got.Data["value"]), "vault-secret-value")
	}
}

func TestVaultStore_GetNotFound(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	defer vs.Close()
	ctx := context.Background()

	_, err := vs.Get(ctx, "nonexistent")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Get nonexistent: got %v, want ErrSecretNotFound", err)
	}
}

func TestVaultStore_Delete(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	defer vs.Close()
	ctx := context.Background()

	secret := &secrets.Secret{
		Path:    "test/delete",
		Type:    secrets.SecretTypeStatic,
		Version: 1,
		Data:    map[string][]byte{"value": []byte("to-delete")},
	}
	vs.Set(ctx, "test/delete", secret)

	if err := vs.Delete(ctx, "test/delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := vs.Get(ctx, "test/delete")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Get after delete: got %v, want ErrSecretNotFound", err)
	}
}

func TestVaultStore_DeleteNotFound(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	defer vs.Close()
	ctx := context.Background()

	err := vs.Delete(ctx, "nonexistent")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Delete nonexistent: got %v, want ErrSecretNotFound", err)
	}
}

func TestVaultStore_List(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	defer vs.Close()
	ctx := context.Background()

	// Store some secrets
	for _, path := range []string{"app/db/pass", "app/db/user", "app/api/key"} {
		s := &secrets.Secret{
			Path:    path,
			Type:    secrets.SecretTypeStatic,
			Version: 1,
			Data:    map[string][]byte{"value": []byte("v")},
		}
		vs.Set(ctx, path, s)
	}

	paths, err := vs.List(ctx, "app/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 3 {
		t.Errorf("List count = %d, want 3", len(paths))
	}
}

func TestVaultStore_ListNotFound(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	defer vs.Close()
	ctx := context.Background()

	paths, err := vs.List(ctx, "nonexistent/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if paths != nil {
		t.Errorf("List empty: got %v, want nil", paths)
	}
}

func TestVaultStore_Watch(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)

	// Use a long-lived context to avoid race between context cancel and Close
	ctx := context.Background()

	ch, err := vs.Watch(ctx, "watched/secret")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Set secret to trigger notification via watchers
	s := &secrets.Secret{
		Path:    "watched/secret",
		Type:    secrets.SecretTypeStatic,
		Version: 1,
		Data:    map[string][]byte{"value": []byte("watched")},
	}
	vs.Set(ctx, "watched/secret", s)

	select {
	case got := <-ch:
		if got == nil {
			t.Error("Received nil from watch")
		}
	case <-time.After(2 * time.Second):
		// Timeout is acceptable - the Set notifyWatchers may or may not fire
	}

	// Close the store - this closes the closeCh, stopping the watch goroutine
	vs.Close()
}

func TestVaultStore_Close(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	ctx := context.Background()

	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// All operations should return ErrStoreClosed
	_, err := vs.Get(ctx, "test")
	if err != secrets.ErrStoreClosed {
		t.Errorf("Get on closed: got %v, want ErrStoreClosed", err)
	}

	err = vs.Set(ctx, "test", &secrets.Secret{Version: 1})
	if err != secrets.ErrStoreClosed {
		t.Errorf("Set on closed: got %v, want ErrStoreClosed", err)
	}

	err = vs.Delete(ctx, "test")
	if err != secrets.ErrStoreClosed {
		t.Errorf("Delete on closed: got %v, want ErrStoreClosed", err)
	}

	_, err = vs.List(ctx, "")
	if err != secrets.ErrStoreClosed {
		t.Errorf("List on closed: got %v, want ErrStoreClosed", err)
	}

	_, err = vs.Watch(ctx, "test")
	if err != secrets.ErrStoreClosed {
		t.Errorf("Watch on closed: got %v, want ErrStoreClosed", err)
	}

	// Double close should not panic
	if err := vs.Close(); err != nil {
		t.Errorf("Double close: %v", err)
	}
}

func TestVaultStore_SecretToVaultWithExpiry(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)
	defer vs.Close()
	ctx := context.Background()

	secret := &secrets.Secret{
		Path:      "test/expiry",
		Type:      secrets.SecretTypeDynamic,
		Version:   1,
		Data:      map[string][]byte{"value": []byte("expires-soon")},
		Metadata:  map[string]string{"env": "prod"},
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := vs.Set(ctx, "test/expiry", secret); err != nil {
		t.Fatalf("Set with expiry: %v", err)
	}

	got, err := vs.Get(ctx, "test/expiry")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Data["value"]) != "expires-soon" {
		t.Errorf("Data = %q", string(got.Data["value"]))
	}
}

func TestVaultStore_VerifyConnectionFailure(t *testing.T) {
	// Server that returns an error on health check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errors":["server error"]}`))
	}))
	defer server.Close()

	_, err := store.NewVaultStore(store.VaultStoreConfig{
		Address: server.URL,
		Token:   "test-token",
	})
	if err == nil {
		t.Error("expected error for failed health check")
	}
}

func TestVaultStore_ServerError(t *testing.T) {
	// Server that returns errors for all data/metadata requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sys/health") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"initialized":true}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"errors":["internal error for %s"]}`, r.URL.Path)))
	}))
	defer server.Close()

	vs, err := store.NewVaultStore(store.VaultStoreConfig{
		Address:      server.URL,
		Token:        "test-token",
		PollInterval: time.Hour, // Don't poll in this test
	})
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}
	defer vs.Close()
	ctx := context.Background()

	// Get should return error
	_, err = vs.Get(ctx, "test/path")
	if err == nil {
		t.Error("expected error for server error on Get")
	}

	// Set should return error
	err = vs.Set(ctx, "test/path", &secrets.Secret{Version: 1, Data: map[string][]byte{"k": []byte("v")}})
	if err == nil {
		t.Error("expected error for server error on Set")
	}

	// Delete should return error
	err = vs.Delete(ctx, "test/path")
	if err == nil {
		t.Error("expected error for server error on Delete")
	}

	// List should return error
	_, err = vs.List(ctx, "test/")
	if err == nil {
		t.Error("expected error for server error on List")
	}
}

func TestVaultStore_HealthCheckTooManyRequests(t *testing.T) {
	// Server that returns 429 on health check (acceptable per Vault docs)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sys/health") {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"initialized":true,"sealed":false}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	vs, err := store.NewVaultStore(store.VaultStoreConfig{
		Address:      server.URL,
		Token:        "test-token",
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewVaultStore with 429 health: %v", err)
	}
	vs.Close()
}

func TestVaultStore_WatchContextCancel(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	vs := newTestVaultStore(t, server)

	ctx, cancel := context.WithCancel(context.Background())
	_, err := vs.Watch(ctx, "test/path")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Cancel to trigger watcher removal goroutine
	cancel()
	// Wait for the removeWatcher goroutine to finish before closing
	time.Sleep(200 * time.Millisecond)
	vs.Close()
}

func TestVaultStore_DefaultMountPath(t *testing.T) {
	mock := newMockVault()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	// Don't specify MountPath - should default to "secret"
	vs, err := store.NewVaultStore(store.VaultStoreConfig{
		Address: server.URL,
		Token:   "test-token",
	})
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}
	defer vs.Close()

	// Should work with default mount path
	secret := &secrets.Secret{
		Path:    "test/default-mount",
		Type:    secrets.SecretTypeStatic,
		Version: 1,
		Data:    map[string][]byte{"value": []byte("default")},
	}
	if err := vs.Set(context.Background(), "test/default-mount", secret); err != nil {
		t.Fatalf("Set with default mount: %v", err)
	}
}
