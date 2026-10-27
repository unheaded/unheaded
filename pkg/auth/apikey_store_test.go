// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testClock is a settable clock shared by a store and its test.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestNewAPIKey_Format(t *testing.T) {
	k1, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	k2, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k2 {
		t.Fatal("two keys are equal")
	}
	for _, k := range []string{k1, k2} {
		if !strings.HasPrefix(k, APIKeyPrefix) || len(k) != len(APIKeyPrefix)+43 {
			t.Fatalf("key %q: want uhk_ + 43 chars", k)
		}
		raw, err := base64.RawURLEncoding.DecodeString(k[len(APIKeyPrefix):])
		if err != nil || len(raw) != 32 {
			t.Fatalf("key body is not 32 bytes of base64url: %v (%d bytes)", err, len(raw))
		}
		if !IsAPIKey(k) {
			t.Fatalf("IsAPIKey(%q) = false", k)
		}
	}
	for _, bad := range []string{"", "uhk_", "uhk_short", "UHK_" + k1[4:], k1 + "x", "uhk_" + strings.Repeat("+", 43), "xx" + k1[2:]} {
		if IsAPIKey(bad) {
			t.Errorf("IsAPIKey(%q) = true", bad)
		}
	}
}

// apiKeyStoreSuite is what every APIKeyStore must do, whatever holds the
// records: memory, a file, or the Well (apikey_well_test.go).
func apiKeyStoreSuite(t *testing.T, newStore func(t *testing.T, now func() time.Time) APIKeyStore) {
	ctx := context.Background()

	t.Run("CreateStoresOnlyTheHash", func(t *testing.T) {
		clk := newTestClock()
		s := newStore(t, clk.Now)
		key, rec, err := s.Create(ctx, "ci-github", RoleOperator, clk.Now().Add(90*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if !IsAPIKey(key) {
			t.Fatalf("Create returned %q, not a uhk_ key", key)
		}
		sum := sha256.Sum256([]byte(key))
		if rec.KeyHash != hex.EncodeToString(sum[:]) {
			t.Errorf("KeyHash %q is not sha256(key)", rec.KeyHash)
		}
		if rec.Prefix != key[:APIKeyDisplayPrefixLen] || len(rec.Prefix) != 12 {
			t.Errorf("Prefix %q, want %q", rec.Prefix, key[:12])
		}
		if rec.Owner != "ci-github" || rec.Role != RoleOperator || rec.ID == "" ||
			!rec.CreatedAt.Equal(clk.Now()) || rec.RevokedAt != nil {
			t.Errorf("record %+v", rec)
		}
		list, err := s.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != rec.ID {
			t.Fatalf("List = %+v", list)
		}
		blob, _ := json.Marshal(list)
		if strings.Contains(string(blob), key[len(APIKeyPrefix):]) {
			t.Fatal("the stored records contain the key itself")
		}
	})

	t.Run("AuthenticateGivesOwnerAndRole", func(t *testing.T) {
		clk := newTestClock()
		s := newStore(t, clk.Now)
		key, rec, err := s.Create(ctx, "stevie", RoleAdmin, clk.Now().Add(24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		id, err := s.Authenticate(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if id.Subject != "stevie" || len(id.Roles) != 1 || id.Roles[0] != RoleAdmin {
			t.Errorf("identity %+v", id)
		}
		if id.Extra["auth_method"] != "apikey" || id.Extra["apikey_prefix"] != rec.Prefix || id.Extra["apikey_id"] != rec.ID {
			t.Errorf("identity extra %v", id.Extra)
		}
	})

	t.Run("RefusesUnknownAndMalformed", func(t *testing.T) {
		clk := newTestClock()
		s := newStore(t, clk.Now)
		if _, _, err := s.Create(ctx, "ci", RoleService, clk.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		other, _ := NewAPIKey()
		for _, k := range []string{other, "", "not-a-key", "uhk_short"} {
			if _, err := s.Authenticate(ctx, k); !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("Authenticate(%q) err = %v, want ErrUnauthenticated", k, err)
			}
		}
	})

	t.Run("RefusesExpired", func(t *testing.T) {
		clk := newTestClock()
		s := newStore(t, clk.Now)
		key, _, err := s.Create(ctx, "ci", RoleService, clk.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Hour)
		_, err = s.Authenticate(ctx, key)
		if !errors.Is(err, ErrAPIKeyExpired) || !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrAPIKeyExpired and ErrUnauthenticated", err)
		}
	})

	t.Run("RevokeByPrefixOrID", func(t *testing.T) {
		clk := newTestClock()
		s := newStore(t, clk.Now)
		k1, r1, err := s.Create(ctx, "ci", RoleService, clk.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		k2, r2, err := s.Create(ctx, "bot", RoleObserver, clk.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Minute)
		got, err := s.Revoke(ctx, r1.Prefix)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != r1.ID || got.RevokedAt == nil || !got.RevokedAt.Equal(clk.Now()) {
			t.Errorf("revoked record %+v", got)
		}
		if _, err := s.Authenticate(ctx, k1); !errors.Is(err, ErrAPIKeyRevoked) || !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("revoked key: err = %v", err)
		}
		if _, err := s.Authenticate(ctx, k2); err != nil {
			t.Errorf("the other key stopped working: %v", err)
		}
		// Revoking again changes nothing and is not an error.
		clk.Advance(time.Minute)
		again, err := s.Revoke(ctx, r1.Prefix)
		if err != nil || again.RevokedAt == nil || !again.RevokedAt.Equal(got.RevokedAt.UTC()) {
			t.Errorf("second revoke: %+v %v", again, err)
		}
		if _, err := s.Revoke(ctx, r2.ID); err != nil {
			t.Errorf("revoke by id: %v", err)
		}
		if _, err := s.Authenticate(ctx, k2); !errors.Is(err, ErrAPIKeyRevoked) {
			t.Errorf("key revoked by id: err = %v", err)
		}
		if _, err := s.Revoke(ctx, "uhk_ZZZZZZZZ"); !errors.Is(err, ErrAPIKeyNotFound) {
			t.Errorf("unknown prefix: err = %v", err)
		}
		if _, err := s.Revoke(ctx, ""); !errors.Is(err, ErrAPIKeyNotFound) {
			t.Errorf("empty prefix: err = %v", err)
		}
		list, err := s.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 || list[0].RevokedAt == nil || list[1].RevokedAt == nil {
			t.Errorf("List after revoke = %+v", list)
		}
	})

	t.Run("CreateValidates", func(t *testing.T) {
		clk := newTestClock()
		s := newStore(t, clk.Now)
		now := clk.Now()
		cases := []struct {
			name, owner, role string
			exp               time.Time
		}{
			{"unknown role", "ci", "root", now.Add(time.Hour)},
			{"viewer is not a role here", "ci", "viewer", now.Add(time.Hour)},
			{"empty owner", "", RoleService, now.Add(time.Hour)},
			{"owner with a space", "c i", RoleService, now.Add(time.Hour)},
			{"owner too long", strings.Repeat("a", 129), RoleService, now.Add(time.Hour)},
			{"expired at birth", "ci", RoleService, now},
			{"no expiry", "ci", RoleService, time.Time{}},
			{"past the maximum lifetime", "ci", RoleService, now.Add(MaxAPIKeyLifetime + time.Second)},
		}
		for _, c := range cases {
			if _, _, err := s.Create(ctx, c.owner, c.role, c.exp); err == nil {
				t.Errorf("%s: Create accepted it", c.name)
			}
		}
		list, err := s.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Errorf("refused creates left records: %+v", list)
		}
		if _, _, err := s.Create(ctx, "ci.bot@west", RoleService, now.Add(MaxAPIKeyLifetime)); err != nil {
			t.Errorf("maximum lifetime refused: %v", err)
		}
	})
}

func TestAPIKeyStore_Mem(t *testing.T) {
	apiKeyStoreSuite(t, func(t *testing.T, now func() time.Time) APIKeyStore {
		return NewMemAPIKeyStore(WithAPIKeyClock(now))
	})
}

func TestAPIKeyStore_File(t *testing.T) {
	apiKeyStoreSuite(t, func(t *testing.T, now func() time.Time) APIKeyStore {
		s, err := NewFileAPIKeyStore(filepath.Join(t.TempDir(), "auth", "apikeys.jsonl"), WithAPIKeyClock(now))
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestAPIKeyStore_FileModeAndContent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "apikeys.jsonl")
	s, err := NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := s.Create(ctx, "ci", RoleService, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("store file mode %v, want 0600", st.Mode().Perm())
	}
	data, err := os.ReadFile(path) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), key[len(APIKeyPrefix):]) {
		t.Fatal("the store file contains the key")
	}
	if n := strings.Count(string(data), "\n"); n != 1 {
		t.Errorf("want one JSON line, got %d", n)
	}
}

func TestAPIKeyStore_FileRefusesLooseMode(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []os.FileMode{0o644, 0o620, 0o666, 0o602} {
		path := filepath.Join(t.TempDir(), "apikeys.jsonl")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := NewFileAPIKeyStore(path); !errors.Is(err, ErrAPIKeyStorePermissions) {
			t.Errorf("mode %v: open err = %v, want ErrAPIKeyStorePermissions", mode, err)
		}
	}
	// Loosened after open: authentication fails closed.
	path := filepath.Join(t.TempDir(), "apikeys.jsonl")
	s, err := NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := s.Create(ctx, "ci", RoleService, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, key); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("world-writable store still authenticates: %v", err)
	}
	// Group read is allowed (a service group reading the operator's file).
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, key); err != nil {
		t.Errorf("0640 store: %v", err)
	}
}

// A service's store sees what the CLI's store writes, without a restart.
func TestAPIKeyStore_FileSharedBetweenProcesses(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "apikeys.jsonl")
	service, err := NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	key, rec, err := cli.Create(ctx, "ci", RoleService, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, key); err != nil {
		t.Fatalf("service does not see a new key: %v", err)
	}
	if _, err := cli.Revoke(ctx, rec.Prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, key); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("service does not see the revocation: %v", err)
	}
}

func TestAPIKeyStore_FileMalformedFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apikeys.jsonl")
	if err := os.WriteFile(path, []byte("{\"id\":\"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileAPIKeyStore(path); err == nil {
		t.Fatal("a malformed store opened")
	}
	if err := os.WriteFile(path, []byte(`{"id":"a","key_hash":"zz","prefix":"uhk_aaaaaaaa","owner":"x","role":"admin"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileAPIKeyStore(path); err == nil {
		t.Fatal("a record with a bad hash opened")
	}
}

func TestAPIKeyStore_FileMissingIsEmpty(t *testing.T) {
	s, err := NewFileAPIKeyStore(filepath.Join(t.TempDir(), "none", "apikeys.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	k, _ := NewAPIKey()
	if _, err := s.Authenticate(context.Background(), k); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("err = %v", err)
	}
	list, err := s.List(context.Background())
	if err != nil || len(list) != 0 {
		t.Errorf("List = %v, %v", list, err)
	}
}

func TestAPIKeyAuthenticator_Store(t *testing.T) {
	ctx := context.Background()
	store := NewMemAPIKeyStore()
	key, rec, err := store.Create(ctx, "ci-github", RoleOperator, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAPIKeyAuthenticator(APIKeyConfig{Store: store})
	if err != nil {
		t.Fatalf("a store alone is a valid configuration: %v", err)
	}
	h := Middleware(a)(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	r.Header.Set("X-API-Key", key)
	if c := codeOf(h, r); c != http.StatusOK {
		t.Errorf("X-API-Key: %d", c)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	r.Header.Set("Authorization", "Bearer "+key)
	if c := codeOf(h, r); c != http.StatusOK {
		t.Errorf("Bearer uhk_: %d", c)
	}
	id, err := a.Authenticate(ctx, r)
	if err != nil || id.Subject != "ci-github" || !id.HasRole(RoleOperator) {
		t.Errorf("identity %+v %v", id, err)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	r.Header.Set("Authorization", "Bearer not-an-api-key")
	if c := codeOf(h, r); c != http.StatusUnauthorized {
		t.Errorf("other bearer: %d", c)
	}

	if _, err := store.Revoke(ctx, rec.Prefix); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	r.Header.Set("X-API-Key", key)
	if c := codeOf(h, r); c != http.StatusUnauthorized {
		t.Errorf("revoked key: %d", c)
	}
}

// The plain list (AUTH_API_KEYS / AUTH_API_KEY_FILE) keeps working beside a
// store for one release.
func TestAPIKeyAuthenticator_StoreAndLegacyList(t *testing.T) {
	ctx := context.Background()
	store := NewMemAPIKeyStore()
	key, _, err := store.Create(ctx, "ci", RoleService, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAPIKeyAuthenticator(APIKeyConfig{Store: store, Keys: []string{"legacy-key"}, DefaultRoles: []string{RoleObserver}})
	if err != nil {
		t.Fatal(err)
	}
	for k, sub := range map[string]string{key: "ci", "legacy-key": "apikey"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-API-Key", k)
		id, err := a.Authenticate(ctx, r)
		if err != nil || id.Subject != sub {
			t.Errorf("%s: %+v %v", sub, id, err)
		}
	}
	if _, err := NewAPIKeyAuthenticator(APIKeyConfig{}); err == nil {
		t.Error("neither keys nor a store must still be refused")
	}
}

func TestAPIKeyAuthenticator_StoreRateLimited(t *testing.T) {
	ctx := context.Background()
	store := NewMemAPIKeyStore()
	key, _, err := store.Create(ctx, "ci", RoleService, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAPIKeyAuthenticator(APIKeyConfig{Store: store, RateLimitPerMinute: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", key)
	if _, err := a.Authenticate(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, r); !errors.Is(err, ErrRateLimited) {
		t.Errorf("second request: %v", err)
	}
}

func TestParseAPIKeyStoreSpec(t *testing.T) {
	for spec, want := range map[string]string{
		"well":                       "well",
		"file:/etc/unheaded/x.jsonl": "file",
	} {
		kind, _, err := ParseAPIKeyStoreSpec(spec)
		if err != nil || kind != want {
			t.Errorf("%q: %q %v", spec, kind, err)
		}
	}
	for _, bad := range []string{"", "file:", "file:relative/path", "sqlite:x", "WELL"} {
		if _, _, err := ParseAPIKeyStoreSpec(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestDefaultAPIKeyStoreSpec(t *testing.T) {
	t.Setenv("AUTH_APIKEY_STORE", "")
	t.Setenv("WELL_DB", "")
	os.Unsetenv("WELL_DB")
	if got := DefaultAPIKeyStoreSpec(); got != "file:"+DefaultAPIKeyFile {
		t.Errorf("no env: %q", got)
	}
	t.Setenv("WELL_DB", "unheaded_app")
	if got := DefaultAPIKeyStoreSpec(); got != "well" {
		t.Errorf("WELL_DB set: %q", got)
	}
	t.Setenv("AUTH_APIKEY_STORE", "file:/tmp/k.jsonl")
	if got := DefaultAPIKeyStoreSpec(); got != "file:/tmp/k.jsonl" {
		t.Errorf("AUTH_APIKEY_STORE wins: %q", got)
	}
}

func TestSetup_APIKeyStoreFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "apikeys.jsonl")
	cli, err := NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := cli.Create(ctx, "ci", RoleOperator, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_TRUST_DIR", "")
	t.Setenv("AUTH_APIKEY_STORE", "file:"+path)
	cfg := LoadServiceAuthConfig("cuirass")
	cfg.TrustDir = ""
	if cfg.APIKeyStore != "file:"+path {
		t.Fatalf("APIKeyStore %q", cfg.APIKeyStore)
	}
	mw, err := NewServiceMiddleware(cfg)
	if err != nil {
		t.Fatalf("a store alone is enough credentials: %v", err)
	}
	h := mw(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/api/v1/containers", nil)
	r.Header.Set("X-API-Key", key)
	if c := codeOf(h, r); c != http.StatusOK {
		t.Errorf("uhk_ key: %d", c)
	}
	r = httptest.NewRequest(http.MethodPost, "/api/v1/containers", nil)
	if c := codeOf(h, r); c != http.StatusUnauthorized {
		t.Errorf("no key: %d", c)
	}

	cfg.APIKeyStore = "sqlite:/x"
	if _, err := NewServiceMiddleware(cfg); err == nil {
		t.Error("a bad AUTH_APIKEY_STORE must refuse to start")
	}
	loose := filepath.Join(t.TempDir(), "loose.jsonl")
	if err := os.WriteFile(loose, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o666); err != nil {
		t.Fatal(err)
	}
	cfg.APIKeyStore = "file:" + loose
	if _, err := NewServiceMiddleware(cfg); !errors.Is(err, ErrAPIKeyStorePermissions) {
		t.Errorf("world-writable store: %v", err)
	}
}
