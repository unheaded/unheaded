// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/auth"
)

// testParams keeps argon2id cheap in tests; production hashes use
// defaultParams (64 MiB, t=3, p=2).
var testParams = argonParams{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestMain(m *testing.M) {
	minArgonMemoryKiB = 8
	os.Exit(m.Run())
}

type fixture struct {
	dir       string
	keyFile   string
	usersFile string
	kid       string
	pub       ed25519.PublicKey
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	kid, err := auth.GenerateKeyPair(dir, "unheaded-auth")
	if err != nil {
		t.Fatal(err)
	}
	priv, _, err := auth.LoadPrivateKey(filepath.Join(dir, "unheaded-auth.key"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		dir:       dir,
		keyFile:   filepath.Join(dir, "unheaded-auth.key"),
		usersFile: filepath.Join(dir, "users.json"),
		kid:       kid,
		pub:       priv.Public().(ed25519.PublicKey),
	}
	f.writeUsers(t, map[string][]string{"dev": {"operator"}, "ro": {"viewer"}}, "correct horse battery")
	return f
}

func (f *fixture) writeUsers(t *testing.T, users map[string][]string, password string) {
	t.Helper()
	var uf usersFile
	for name, roles := range users {
		h, err := hashPassword(password, testParams)
		if err != nil {
			t.Fatal(err)
		}
		uf.Users = append(uf.Users, userRecord{Username: name, Hash: h, Roles: roles})
	}
	raw, err := json.Marshal(uf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.usersFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f.usersFile, 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeClock is a settable clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// serve starts the issuer behind an httptest server whose URL is the
// issuer URL. It returns the server and the issuer.
func serve(t *testing.T, f *fixture, mod func(*issuerConfig)) (*httptest.Server, *issuer) {
	t.Helper()
	var h http.Handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(ts.Close)
	cfg := issuerConfig{
		IssuerURL:    ts.URL,
		KeyFile:      f.keyFile,
		UsersFile:    f.usersFile,
		DevPlaintext: true,
	}
	if mod != nil {
		mod(&cfg)
	}
	iss, err := newIssuer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h = iss.Handler()
	return ts, iss
}

// reply is what the tests read from a response; the body is consumed and
// closed inside the helpers, so no caller holds one open.
type reply struct {
	StatusCode int
	Header     http.Header
}

func getJSON(t *testing.T, url string, out any) reply {
	t.Helper()
	resp, err := http.Get(url) // #nosec G107 -- test server URL
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return reply{resp.StatusCode, resp.Header}
}

func login(t *testing.T, ts *httptest.Server, body string) (reply, map[string]any) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/login", "application/json", strings.NewReader(body)) // #nosec G107 -- test server URL
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return reply{resp.StatusCode, resp.Header}, out
}

func TestDiscovery(t *testing.T) {
	ts, _ := serve(t, newFixture(t), nil)
	var d map[string]any
	resp := getJSON(t, ts.URL+"/.well-known/openid-configuration", &d)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if d["issuer"] != ts.URL || d["jwks_uri"] != ts.URL+"/jwks.json" || d["token_endpoint"] != ts.URL+"/login" {
		t.Errorf("discovery %v", d)
	}
	rts, _ := d["response_types_supported"].([]any)
	if len(rts) != 1 || rts[0] != "token" {
		t.Errorf("response_types_supported %v, want [token]", d["response_types_supported"])
	}
}

func TestJWKS(t *testing.T) {
	f := newFixture(t)
	ts, _ := serve(t, f, nil)
	var set struct {
		Keys []map[string]string `json:"keys"`
	}
	getJSON(t, ts.URL+"/jwks.json", &set)
	if len(set.Keys) != 1 {
		t.Fatalf("keys %v, want one", set.Keys)
	}
	k := set.Keys[0]
	if k["kty"] != "OKP" || k["crv"] != "Ed25519" || k["kid"] != f.kid || k["alg"] != "EdDSA" || k["use"] != "sig" {
		t.Errorf("jwk %v", k)
	}
	x, err := base64.RawURLEncoding.DecodeString(k["x"])
	if err != nil || !bytes.Equal(x, f.pub) {
		t.Errorf("x does not decode to the issuer's public key")
	}
}

func verifier(t *testing.T, ts *httptest.Server, aud string) *auth.JWKSVerifier {
	t.Helper()
	v, err := auth.NewJWKSVerifier(ts.URL, aud, ts.Client(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func authenticate(v *auth.JWKSVerifier, token string) (*auth.Identity, error) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return v.Authenticate(context.Background(), r)
}

func TestLoginOK(t *testing.T) {
	ts, _ := serve(t, newFixture(t), nil)
	resp, out := login(t, ts, `{"username":"dev","password":"correct horse battery"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %v", resp.StatusCode, out)
	}
	if out["token_type"] != "Bearer" || out["expires_in"] != float64(900) {
		t.Errorf("response %v", out)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control %q", cc)
	}
	tok, _ := out["access_token"].(string)
	id, err := authenticate(verifier(t, ts, "unheaded"), tok)
	if err != nil {
		t.Fatalf("JWKS verify: %v", err)
	}
	if id.Subject != "dev" || len(id.Roles) != 1 || id.Roles[0] != "operator" {
		t.Errorf("identity %+v", id)
	}
	parts := strings.Split(tok, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatal(err)
	}
	if c["iss"] != ts.URL || c["aud"] != "unheaded" || c["sub"] != "dev" {
		t.Errorf("claims %v", c)
	}
	exp, _ := c["exp"].(float64)
	iat, _ := c["iat"].(float64)
	if exp-iat != 900 {
		t.Errorf("exp-iat = %v, want 900", exp-iat)
	}
	// A token for another audience does not verify here.
	if _, err := authenticate(verifier(t, ts, "wotan"), tok); err == nil {
		t.Error("token verified for audience wotan")
	}
}

func TestLoginAudience(t *testing.T) {
	ts, _ := serve(t, newFixture(t), func(c *issuerConfig) { c.Audiences = []string{"unheaded", "dashboard-backend"} })
	resp, out := login(t, ts, `{"username":"dev","password":"correct horse battery","audience":"dashboard-backend"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if _, err := authenticate(verifier(t, ts, "dashboard-backend"), out["access_token"].(string)); err != nil {
		t.Errorf("verify for dashboard-backend: %v", err)
	}
	if resp, _ := login(t, ts, `{"username":"dev","password":"correct horse battery","audience":"wotan"}`); resp.StatusCode != 400 {
		t.Errorf("unlisted audience: status %d, want 400", resp.StatusCode)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	ts, _ := serve(t, newFixture(t), nil)
	r1, b1 := login(t, ts, `{"username":"dev","password":"wrong"}`)
	r2, b2 := login(t, ts, `{"username":"nobody","password":"wrong"}`)
	if r1.StatusCode != 401 || r2.StatusCode != 401 {
		t.Fatalf("status %d / %d, want 401", r1.StatusCode, r2.StatusCode)
	}
	// Wrong password and unknown user look the same.
	if b1["error"] != b2["error"] {
		t.Errorf("bodies differ: %v vs %v", b1, b2)
	}
	if b1["access_token"] != nil {
		t.Error("token on failure")
	}
}

func loginFrom(h http.Handler, ip, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	r.RemoteAddr = ip + ":40000"
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestLockout(t *testing.T) {
	f := newFixture(t)
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	cfg := issuerConfig{IssuerURL: "http://127.0.0.1:19020", KeyFile: f.keyFile, UsersFile: f.usersFile, DevPlaintext: true, Clock: clock.Now}
	iss, err := newIssuer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := iss.Handler()
	bad := `{"username":"dev","password":"nope"}`
	good := `{"username":"dev","password":"correct horse battery"}`
	for i := 1; i <= 5; i++ {
		if w := loginFrom(h, "10.0.0.1", bad); w.Code != 401 {
			t.Fatalf("failure %d: status %d, want 401", i, w.Code)
		}
	}
	w := loginFrom(h, "10.0.0.1", good)
	if w.Code != 429 {
		t.Fatalf("6th attempt: status %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}
	// Another source address is not locked out of this user.
	if w := loginFrom(h, "10.0.0.2", good); w.Code != 200 {
		t.Errorf("other IP: status %d, want 200", w.Code)
	}
	clock.Add(9 * time.Minute)
	if w := loginFrom(h, "10.0.0.1", good); w.Code != 429 {
		t.Errorf("after 9 min: status %d, want 429", w.Code)
	}
	clock.Add(2 * time.Minute)
	if w := loginFrom(h, "10.0.0.1", good); w.Code != 200 {
		t.Errorf("after 11 min: status %d, want 200", w.Code)
	}
}

func TestLockoutPerSourceAcrossUsers(t *testing.T) {
	// One address spraying many usernames is stopped too.
	f := newFixture(t)
	iss, err := newIssuer(issuerConfig{IssuerURL: "http://127.0.0.1:19020", KeyFile: f.keyFile, UsersFile: f.usersFile, DevPlaintext: true})
	if err != nil {
		t.Fatal(err)
	}
	h := iss.Handler()
	locked := false
	for i := 0; i < maxFailuresPerSource+1; i++ {
		body := `{"username":"user` + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `","password":"x"}`
		if w := loginFrom(h, "10.0.0.9", body); w.Code == 429 {
			locked = true
			break
		}
	}
	if !locked {
		t.Errorf("no 429 after %d failures from one address", maxFailuresPerSource+1)
	}
}

func TestStartRefusesExposedFiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		file func(*fixture) string
		mode os.FileMode
	}{
		{"users world-readable", func(f *fixture) string { return f.usersFile }, 0o644},
		{"users group-readable", func(f *fixture) string { return f.usersFile }, 0o640},
		{"key world-readable", func(f *fixture) string { return f.keyFile }, 0o644},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if err := os.Chmod(tc.file(f), tc.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := newIssuer(issuerConfig{IssuerURL: "http://127.0.0.1:19020", KeyFile: f.keyFile, UsersFile: f.usersFile, DevPlaintext: true}); err == nil {
				t.Error("issuer started")
			}
		})
	}
}

func TestStartRefusesBadConfig(t *testing.T) {
	f := newFixture(t)
	for name, cfg := range map[string]issuerConfig{
		"no issuer url":     {KeyFile: f.keyFile, UsersFile: f.usersFile},
		"http non-loopback": {IssuerURL: "http://auth.example:19020", KeyFile: f.keyFile, UsersFile: f.usersFile},
		"issuer with path":  {IssuerURL: "https://auth.example/x?y=1", KeyFile: f.keyFile, UsersFile: f.usersFile},
		"missing users":     {IssuerURL: "https://auth.example", KeyFile: f.keyFile, UsersFile: filepath.Join(f.dir, "none.json")},
		"bad audience":      {IssuerURL: "https://auth.example", KeyFile: f.keyFile, UsersFile: f.usersFile, Audiences: []string{"Bad Aud"}},
	} {
		if _, err := newIssuer(cfg); err == nil {
			t.Errorf("%s: issuer started", name)
		}
	}
}

func TestPlaintextRefusedUnlessDev(t *testing.T) {
	ts, _ := serve(t, newFixture(t), func(c *issuerConfig) { c.DevPlaintext = false })
	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if resp := getJSON(t, ts.URL+p, nil); resp.StatusCode != 200 {
			t.Errorf("%s: status %d, want 200", p, resp.StatusCode)
		}
	}
	for _, p := range []string{"/jwks.json", "/.well-known/openid-configuration"} {
		if resp := getJSON(t, ts.URL+p, nil); resp.StatusCode != 403 {
			t.Errorf("%s over plaintext: status %d, want 403", p, resp.StatusCode)
		}
	}
	if resp, _ := login(t, ts, `{"username":"dev","password":"correct horse battery"}`); resp.StatusCode != 403 {
		t.Errorf("login over plaintext: status %d, want 403", resp.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	ts, _ := serve(t, newFixture(t), nil)
	for _, p := range []string{"/health", "/jwks.json", "/nope"} {
		resp := getJSON(t, ts.URL+p, nil)
		h := resp.Header
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" ||
			h.Get("Referrer-Policy") != "no-referrer" || !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Errorf("%s: headers %v", p, h)
		}
		if h.Get("Strict-Transport-Security") != "" {
			t.Errorf("%s: HSTS over plaintext", p)
		}
	}
}

func TestLoginRequestLimits(t *testing.T) {
	ts, _ := serve(t, newFixture(t), nil)
	big := `{"username":"dev","password":"` + strings.Repeat("a", 5000) + `"}`
	if resp, _ := login(t, ts, big); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("5 KB body: status %d, want 413", resp.StatusCode)
	}
	for _, body := range []string{
		``, `{`, `[]`, `{"username":"dev"}`, `{"username":"","password":"x"}`,
		`{"username":"dev","password":"x","extra":1}`,
		`{"username":"dev","password":"x"}{"username":"dev","password":"x"}`,
	} {
		if resp, _ := login(t, ts, body); resp.StatusCode != 400 {
			t.Errorf("body %q: status %d, want 400", body, resp.StatusCode)
		}
	}
	resp := getJSON(t, ts.URL+"/login", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /login: status %d, want 405", resp.StatusCode)
	}
	r, err := http.Post(ts.URL+"/login", "text/plain", strings.NewReader(`{"username":"dev","password":"correct horse battery"}`)) // #nosec G107 -- test server URL
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain login: status %d, want 415", r.StatusCode)
	}
}

func TestReloadUsers(t *testing.T) {
	f := newFixture(t)
	ts, iss := serve(t, f, nil)
	f.writeUsers(t, map[string][]string{"new": {"viewer"}}, "another password")
	if err := iss.ReloadUsers(); err != nil {
		t.Fatal(err)
	}
	if resp, _ := login(t, ts, `{"username":"new","password":"another password"}`); resp.StatusCode != 200 {
		t.Errorf("new user after reload: %d", resp.StatusCode)
	}
	if resp, _ := login(t, ts, `{"username":"dev","password":"correct horse battery"}`); resp.StatusCode != 401 {
		t.Errorf("removed user after reload: %d", resp.StatusCode)
	}
	// A broken file keeps the users already loaded.
	if err := os.WriteFile(f.usersFile, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := iss.ReloadUsers(); err == nil {
		t.Error("reload accepted a broken file")
	}
	if resp, _ := login(t, ts, `{"username":"new","password":"another password"}`); resp.StatusCode != 200 {
		t.Errorf("after a failed reload: %d", resp.StatusCode)
	}
}
