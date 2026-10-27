// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"unheaded/pkg/auth"
)

// M-019 (a): no listener without auth, unless -no-auth on loopback.
func TestListenerAuth_RefusesWithoutToken(t *testing.T) {
	for name, cfg := range map[string]auth.ServiceAuthConfig{
		"auth off":           {},
		"enabled, no keys":   {Enabled: true},
		"keys but not on":    {APIKeys: []string{"k"}},
		"jwt key but not on": {JWTSigningKey: []byte("0123456789abcdef0123456789abcdef")},
	} {
		mw, err := listenerAuth(cfg, false, "127.0.0.1")
		if err == nil || mw != nil {
			t.Errorf("%s: started (err %v)", name, err)
			continue
		}
		for _, want := range []string{"no auth token set", "AUTH_API_KEYS", "-no-auth"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q lacks %q", name, err, want)
			}
		}
	}
}

func TestListenerAuth_AcceptsEachCredential(t *testing.T) {
	for name, cfg := range map[string]auth.ServiceAuthConfig{
		"api keys": {Enabled: true, APIKeys: []string{"k1"}},
		"key file": {Enabled: true, APIKeyFile: "/nonexistent/keys"},
		"jwt":      {Enabled: true, JWTSigningKey: []byte("0123456789abcdef0123456789abcdef")},
	} {
		mw, err := listenerAuth(cfg, false, "0.0.0.0")
		if err != nil || mw == nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}

func TestListenerAuth_NoAuthOnlyOnLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", "localhost", "127.0.0.2"} {
		mw, err := listenerAuth(auth.ServiceAuthConfig{}, true, host)
		if err != nil || mw != nil {
			t.Errorf("-no-auth on %s: mw %v err %v, want no middleware and no error", host, mw != nil, err)
		}
	}
	for _, host := range []string{"0.0.0.0", "", "::", "10.10.10.5", "192.168.13.2", "example.org"} {
		if _, err := listenerAuth(auth.ServiceAuthConfig{}, true, host); err == nil || !strings.Contains(err.Error(), "loopback") {
			t.Errorf("-no-auth on %q accepted (err %v)", host, err)
		}
	}
}

func TestListenerAuth_NoAuthConflictsWithAuthEnabled(t *testing.T) {
	_, err := listenerAuth(auth.ServiceAuthConfig{Enabled: true, APIKeys: []string{"k"}}, true, "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "AUTH_ENABLED") {
		t.Errorf("-no-auth with AUTH_ENABLED=true: err %v", err)
	}
}

// The middleware listenerAuth returns is the real one: a key is needed for
// the API, the spec and probes stay open.
func TestListenerAuth_MiddlewareGuardsTheAPI(t *testing.T) {
	mw, err := listenerAuth(auth.ServiceAuthConfig{Enabled: true, APIKeys: []string{"right"}, ServiceName: "zhen-agentd"}, false, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := mw(ok)
	for _, c := range []struct {
		path, key string
		want      int
	}{
		{"/api/v1/tool/exec", "", http.StatusUnauthorized},
		{"/api/v1/tool/exec", "wrong", http.StatusUnauthorized},
		{"/api/v1/tool/exec", "right", http.StatusOK},
		{"/api/v1/agent/ask", "", http.StatusUnauthorized},
		{"/api/v1/openapi.json", "", http.StatusOK},
		{"/health", "", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodPost, c.path, nil)
		if c.key != "" {
			req.Header.Set("X-API-Key", c.key)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s key %q: %d, want %d", c.path, c.key, rec.Code, c.want)
		}
	}
}

// M-019 (b): the per-IP limiter is on unless the operator turns it off.
func TestRateLimitDefaultIsOn(t *testing.T) {
	if defaultRateLimitRPS <= 0 {
		t.Fatalf("defaultRateLimitRPS = %v, want > 0", defaultRateLimitRPS)
	}
	if newRateLimiter(defaultRateLimitRPS, defaultRateBurst) == nil {
		t.Fatal("the default rate limit builds no limiter")
	}
}

// The real binary: main exits 2 before opening anything when auth is
// missing or -no-auth is asked for off loopback. Runs main in a child
// process (this test binary re-executed).
func TestMain_RefusesToStartUnauthenticated(t *testing.T) {
	if os.Getenv("ZHEN_AGENTD_MAIN_CHILD") == "1" {
		flag.CommandLine = flag.NewFlagSet("zhen-agentd", flag.ExitOnError)
		os.Args = append([]string{"zhen-agentd"}, strings.Fields(os.Getenv("ZHEN_AGENTD_MAIN_ARGS"))...)
		main()
		return
	}
	for name, args := range map[string]string{
		"no token":          "-port 1 -project-root " + t.TempDir(),
		"-no-auth off loop": "-no-auth -host 0.0.0.0 -port 1 -project-root " + t.TempDir(),
	} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestMain_RefusesToStartUnauthenticated$") // #nosec G204 -- re-executes this test binary
		cmd.Env = append(cleanAuthEnv(), "ZHEN_AGENTD_MAIN_CHILD=1", "ZHEN_AGENTD_MAIN_ARGS="+args)
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 2 {
			t.Errorf("%s: exit %v, want 2\n%s", name, err, out)
		}
		if name == "no token" && !strings.Contains(string(out), "no auth token set") {
			t.Errorf("%s: output lacks the reason:\n%s", name, out)
		}
	}
}

// cleanAuthEnv is the test's environment without AUTH_* variables.
func cleanAuthEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "AUTH_") {
			env = append(env, kv)
		}
	}
	return env
}
