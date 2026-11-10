// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	pqtls "unheaded/pkg/tls"
	"unheaded/pkg/tls/pqtest"
)

// seen is what the app behind the front received.
type seen struct {
	Method string
	Path   string
	Query  string
	Host   string
	Header http.Header
	Body   string
}

// fakeApp is a plain-HTTP server on a Unix socket, standing in for the Flask
// app. It records every request and answers with its JSON.
type fakeApp struct {
	socket string
	mu     sync.Mutex
	reqs   []seen
}

func (a *fakeApp) last(t *testing.T) seen {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.reqs) == 0 {
		t.Fatal("the app saw no request")
	}
	return a.reqs[len(a.reqs)-1]
}

func (a *fakeApp) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.reqs)
}

// socketDir is a short private directory: a Unix socket path is capped near
// 108 bytes, which t.TempDir can exceed.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "zf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func startApp(t *testing.T, mode os.FileMode) *fakeApp {
	t.Helper()
	a := &fakeApp{socket: filepath.Join(socketDir(t), "app.sock")}
	ln, err := net.Listen("unix", a.socket)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(a.socket, mode); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := seen{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Host: r.Host, Header: r.Header.Clone(), Body: string(body)}
		a.mu.Lock()
		a.reqs = append(a.reqs, s)
		a.mu.Unlock()
		w.Header().Set("Server", "Werkzeug/0 Python/0")
		w.Header().Set("Set-Cookie", "sid=1; Path=/; HttpOnly")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTeapot)
		_ = json.NewEncoder(w).Encode(s)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return a
}

type testFront struct {
	*front
	addr string
	cert tls.Certificate
}

// startFront serves the front on an ephemeral loopback port with a test
// certificate loaded the way the binary loads it: from the environment.
func startFront(t *testing.T, socket string, mut func(*config)) *testFront {
	t.Helper()
	cert := pqtest.SelfSigned(t)
	certFile, keyFile := pqtest.WritePEM(t, t.TempDir(), cert)
	tlsCfg, err := loadTLS(envMap{pqtls.EnvCert: certFile, pqtls.EnvKeyFile: keyFile}.get)
	if err != nil {
		t.Fatalf("loadTLS: %v", err)
	}
	cfg := config{socket: socket, timeout: 2 * time.Second, maxBody: defaultMaxBody, maxConns: defaultMaxConns}
	if mut != nil {
		mut(&cfg)
	}
	f := newFront(cfg, zerolog.Nop())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := f.server(tlsCfg)
	go func() { _ = srv.ServeTLS(f.limit(ln), "", "") }()
	t.Cleanup(func() { _ = srv.Close() })
	return &testFront{front: f, addr: ln.Addr().String(), cert: cert}
}

func (f *testFront) clientTLS(mut func(*tls.Config)) *tls.Config {
	c := &tls.Config{RootCAs: pqtest.Pool(f.cert), ServerName: "localhost", MinVersion: tls.VersionTLS12}
	if mut != nil {
		mut(c)
	}
	return c
}

func (f *testFront) client() *http.Client {
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: f.clientTLS(nil), DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (f *testFront) dial(mut func(*tls.Config)) (tls.ConnectionState, error) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", f.addr, f.clientTLS(mut))
	if err != nil {
		return tls.ConnectionState{}, err
	}
	defer func() { _ = conn.Close() }()
	return conn.ConnectionState(), nil
}

type envMap map[string]string

func (e envMap) get(k string) string { return e[k] }

// ADR-138: a client that offers only classical key exchange fails the
// handshake; a hybrid client gets TLS 1.3 on a hybrid group.
func TestHandshakeIsHybridOnly(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, nil)

	for _, tc := range []struct {
		name string
		mut  func(*tls.Config)
	}{
		{"x25519 only", func(c *tls.Config) { c.CurvePreferences = []tls.CurveID{tls.X25519} }},
		{"p256 only", func(c *tls.Config) { c.CurvePreferences = []tls.CurveID{tls.CurveP256} }},
		{"every classical group", func(c *tls.Config) {
			c.CurvePreferences = []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521}
		}},
		{"tls 1.2", func(c *tls.Config) { c.MaxVersion = tls.VersionTLS12 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if cs, err := f.dial(tc.mut); err == nil {
				t.Fatalf("handshake succeeded (version %#x, group %v); it must be refused", cs.Version, cs.CurveID)
			}
		})
	}

	for _, tc := range []struct {
		name string
		mut  func(*tls.Config)
	}{
		{"go default client", nil},
		{"x25519mlkem768 only", func(c *tls.Config) { c.CurvePreferences = []tls.CurveID{tls.X25519MLKEM768} }},
		{"hybrid listed after classical", func(c *tls.Config) {
			c.CurvePreferences = []tls.CurveID{tls.X25519, tls.X25519MLKEM768}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := f.dial(tc.mut)
			if err != nil {
				t.Fatalf("hybrid handshake: %v", err)
			}
			pqtest.RequireHybrid(t, cs)
		})
	}

	if got := app.count(); got != 0 {
		t.Errorf("the app saw %d requests from bare handshakes", got)
	}
	if got := f.m.hellos.WithLabelValues(kexClassical).Value(); got < 3 {
		t.Errorf("classical ClientHello counter %v, want at least 3", got)
	}
	if got := f.m.hellos.WithLabelValues(kexHybrid).Value(); got < 3 {
		t.Errorf("hybrid ClientHello counter %v, want at least 3", got)
	}
}

// The front speaks HTTP/1.1 only, whatever the client offers.
func TestHandshakeNegotiatesHTTP11(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, nil)
	cs, err := f.dial(func(c *tls.Config) { c.NextProtos = []string{"h2", "http/1.1"} })
	if err != nil {
		t.Fatal(err)
	}
	if cs.NegotiatedProtocol != "http/1.1" {
		t.Errorf("ALPN %q, want http/1.1", cs.NegotiatedProtocol)
	}
}

func TestLoadTLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := pqtest.WritePEM(t, dir, pqtest.SelfSigned(t))
	junk := filepath.Join(dir, "junk.pem")
	if err := os.WriteFile(junk, []byte("not pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "absent.pem")

	for _, tc := range []struct {
		name    string
		env     envMap
		wantErr string
	}{
		{"nothing set", envMap{}, pqtls.EnvCert},
		{"enabled alone is not a certificate", envMap{pqtls.EnvEnabled: "true"}, pqtls.EnvCert},
		{"certificate without key", envMap{pqtls.EnvCert: certFile}, pqtls.EnvKeyFile},
		{"key without certificate", envMap{pqtls.EnvKeyFile: keyFile}, pqtls.EnvCert},
		{"blank values", envMap{pqtls.EnvCert: "  ", pqtls.EnvKeyFile: " "}, pqtls.EnvCert},
		{"missing certificate file", envMap{pqtls.EnvCert: missing, pqtls.EnvKeyFile: keyFile}, "certificate"},
		{"missing key file", envMap{pqtls.EnvCert: certFile, pqtls.EnvKeyFile: missing}, "certificate"},
		{"not pem", envMap{pqtls.EnvCert: junk, pqtls.EnvKeyFile: junk}, "certificate"},
		{"key that does not match", envMap{pqtls.EnvCert: certFile, pqtls.EnvKeyFile: certFile}, "certificate"},
		{"relative path", envMap{pqtls.EnvCert: "cert.pem", pqtls.EnvKeyFile: keyFile}, "absolute"},
		{"good", envMap{pqtls.EnvCert: certFile, pqtls.EnvKeyFile: keyFile}, ""},
		// The profile and TLS switches other services read cannot weaken or
		// disable this listener.
		{"preferred profile is ignored", envMap{pqtls.EnvCert: certFile, pqtls.EnvKeyFile: keyFile, pqtls.EnvProfile: "preferred"}, ""},
		{"tls off is ignored", envMap{pqtls.EnvCert: certFile, pqtls.EnvKeyFile: keyFile, pqtls.EnvEnabled: "false"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadTLS(tc.env.get)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("no error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not mention %q", err, tc.wantErr)
				}
				if cfg != nil {
					t.Fatal("a config came back with the error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MinVersion != tls.VersionTLS13 {
				t.Errorf("MinVersion %#x", cfg.MinVersion)
			}
			if len(cfg.CurvePreferences) == 0 {
				t.Fatal("CurvePreferences is empty: Go's default list accepts classical groups")
			}
			for _, id := range cfg.CurvePreferences {
				if !pqtls.IsHybrid(id) {
					t.Errorf("group %v is not hybrid", id)
				}
			}
			if cfg.ClientAuth != tls.NoClientCert {
				t.Errorf("ClientAuth %v: a browser has no client certificate", cfg.ClientAuth)
			}
		})
	}
}

// run refuses to start without a certificate and key, before it listens.
func TestRunFailsWithoutCertificate(t *testing.T) {
	t.Setenv(pqtls.EnvCert, "")
	t.Setenv(pqtls.EnvKeyFile, "")
	t.Setenv(pqtls.EnvEnabled, "true")
	cfg, err := parseFlags([]string{"--listen", "127.0.0.1:0", "--metrics", "[::1]:0", "--socket", "/nonexistent/app.sock"}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, cfg, zerolog.Nop())
	if err == nil || !strings.Contains(err.Error(), pqtls.EnvCert) {
		t.Fatalf("run: %v, want an error naming %s", err, pqtls.EnvCert)
	}
	if ctx.Err() != nil {
		t.Fatal("run waited for the context instead of failing at once")
	}
}

// run serves until its context ends, then returns nil.
func TestRunServesAndStops(t *testing.T) {
	app := startApp(t, 0o600)
	cert := pqtest.SelfSigned(t)
	certFile, keyFile := pqtest.WritePEM(t, t.TempDir(), cert)
	t.Setenv(pqtls.EnvCert, certFile)
	t.Setenv(pqtls.EnvKeyFile, keyFile)

	free := func() string {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ln.Close() }()
		return ln.Addr().String()
	}
	listen, ops := free(), free()
	cfg, err := parseFlags([]string{"--listen", listen, "--metrics", ops, "--socket", app.socket}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, zerolog.Nop()) }()

	hc := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pqtest.Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS13},
		DisableKeepAlives: true,
	}}
	var resp *http.Response
	for range 100 {
		if resp, err = hc.Get("https://" + listen + "/x"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("the front never answered: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status %d through run", resp.StatusCode)
	}
	plain, err := http.Get("http://" + ops + "/ready")
	if err != nil {
		t.Errorf("ops listener: %v", err)
	} else {
		_ = plain.Body.Close()
		if plain.StatusCode != http.StatusOK {
			t.Errorf("/ready %d", plain.StatusCode)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v after cancel", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after cancel")
	}
}

// A plaintext request to the TLS port gets no answer from the app.
func TestPlaintextIsNotProxied(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, nil)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get("http://" + f.addr + "/health")
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Errorf("plaintext request answered %d", resp.StatusCode)
		}
	}
	if app.count() != 0 {
		t.Error("a plaintext request reached the app")
	}
}

func TestProxyPassesTheRequest(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, nil)

	req, err := http.NewRequest(http.MethodPost, "https://"+f.addr+"/api/v1/query?k=3&q=a%2Fb", strings.NewReader(`{"q":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "zhen.example:20103"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "sid=abc")
	req.Header.Set("Origin", "https://zhen.example:20103")
	req.Header.Set("X-CSRF-Token", "tok")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := f.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status %d, want the app's 418", resp.StatusCode)
	}
	if got := resp.Header.Get("Set-Cookie"); got != "sid=1; Path=/; HttpOnly" {
		t.Errorf("Set-Cookie %q", got)
	}
	if got := resp.Header.Get("Server"); got != "" {
		t.Errorf("Server %q leaks the app's versions", got)
	}
	for h := range resp.Header {
		if strings.HasPrefix(strings.ToLower(h), "strict-transport") {
			t.Errorf("%s set: HSTS on a name the host's plain services share", h)
		}
	}
	got := app.last(t)
	if got.Method != http.MethodPost || got.Path != "/api/v1/query" || got.Query != "k=3&q=a%2Fb" || got.Body != `{"q":"hi"}` {
		t.Errorf("app saw %s %s?%s body %q", got.Method, got.Path, got.Query, got.Body)
	}
	// The app checks Host and Origin against each other (CSRF), so Host
	// arrives as the client sent it.
	if got.Host != "zhen.example:20103" {
		t.Errorf("Host %q", got.Host)
	}
	for h, want := range map[string]string{
		"Content-Type":   "application/json",
		"Cookie":         "sid=abc",
		"Origin":         "https://zhen.example:20103",
		"X-Csrf-Token":   "tok",
		"Sec-Fetch-Site": "same-origin",
		clientAddrHeader: "127.0.0.1",
	} {
		if v := got.Header.Get(h); v != want {
			t.Errorf("%s %q, want %q", h, v, want)
		}
	}
}

// Nothing a client sends can state where it came from or how it arrived.
func TestProxyStripsForwardingHeaders(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, nil)

	hostile := map[string]string{
		"Forwarded":              "for=10.9.9.9;proto=https",
		"X-Forwarded-For":        "10.9.9.9",
		"X-Forwarded-Proto":      "https",
		"X-Forwarded-Host":       "evil.example",
		"X-Forwarded-Port":       "443",
		"X-Forwarded-Ssl":        "on",
		"X-Forwarded-Prefix":     "/x",
		"X-Forwarded-Scheme":     "https",
		"X-Forwarded":            "for=10.9.9.9",
		"x-forwarded-anything":   "1",
		"X_Forwarded_Proto":      "https",
		"X_FORWARDED_FOR":        "10.9.9.9",
		"X-Real-Ip":              "10.9.9.9",
		"X_Real_IP":              "10.9.9.9",
		"X-Client-Ip":            "10.9.9.9",
		"X-Url-Scheme":           "https",
		"X-Scheme":               "https",
		"X-Https":                "on",
		"Front-End-Https":        "on",
		"X-Original-Url":         "/admin",
		"X-Rewrite-Url":          "/admin",
		"X-Unheaded-Client-Addr": "10.9.9.9",
		"X_Unheaded_Client_Addr": "10.9.9.9",
		"x-unheaded-client-addr": "10.9.9.8",
		"X-Unheaded-Tls":         "1",
		"X-Unheaded_Client-Addr": "10.9.9.9",
		"Upgrade":                "websocket",
	}
	for name, value := range hostile {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://"+f.addr+"/health", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header[name] = []string{value} // as written, not canonicalised
			req.Header.Set("X-Keep", "kept")
			if name == "Upgrade" {
				req.Header.Set("Connection", "Upgrade")
			}
			resp, err := f.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusTeapot {
				t.Fatalf("status %d", resp.StatusCode)
			}
			got := app.last(t)
			if got.Header.Get("X-Keep") != "kept" {
				t.Error("an ordinary header was dropped")
			}
			if addrs := got.Header.Values(clientAddrHeader); len(addrs) != 1 || addrs[0] != "127.0.0.1" {
				t.Errorf("%s %v, want exactly the TCP peer", clientAddrHeader, addrs)
			}
			for h, vals := range got.Header {
				if h == clientAddrHeader {
					continue
				}
				if untrustedHeader(h) {
					t.Errorf("the app received %s: %v", h, vals)
				}
				for _, v := range vals {
					if strings.Contains(v, "10.9.9.") {
						t.Errorf("the client's claim reached the app in %s: %q", h, v)
					}
				}
			}
		})
	}
}

func TestUntrustedHeader(t *testing.T) {
	for name, want := range map[string]bool{
		"Forwarded":              true,
		"forwarded":              true,
		"X-Forwarded":            true,
		"X-Forwarded-For":        true,
		"x_forwarded_for":        true,
		"X-FORWARDED-PROTO":      true,
		"X-Forwardedfor":         true,
		"X-Real-IP":              true,
		"X-Client-IP":            true,
		"X-Unheaded-Client-Addr": true,
		"X_UNHEADED_ANYTHING":    true,
		"Front-End-Https":        true,
		"X-Https":                true,
		"X-Scheme":               true,
		"X-Url-Scheme":           true,
		"X-Original-URL":         true,
		"X-Rewrite-URL":          true,
		"Upgrade":                true,
		"Cookie":                 false,
		"Origin":                 false,
		"Host":                   false,
		"X-CSRF-Token":           false,
		"Content-Type":           false,
		"Forwarded-Not":          false,
		"Accept":                 false,
		"Sec-Fetch-Site":         false,
		"Authorization":          false,
	} {
		if got := untrustedHeader(name); got != want {
			t.Errorf("untrustedHeader(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestProxyRefusesMethods(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, nil)
	for method, want := range map[string]int{
		http.MethodGet:     http.StatusTeapot,
		http.MethodHead:    http.StatusTeapot,
		http.MethodPost:    http.StatusTeapot,
		http.MethodPut:     http.StatusTeapot,
		http.MethodPatch:   http.StatusTeapot,
		http.MethodDelete:  http.StatusTeapot,
		http.MethodOptions: http.StatusTeapot,
		http.MethodTrace:   http.StatusMethodNotAllowed,
		"PROPFIND":         http.StatusMethodNotAllowed,
		"BREW":             http.StatusMethodNotAllowed,
	} {
		t.Run(method, func(t *testing.T) {
			before := app.count()
			req, err := http.NewRequest(method, "https://"+f.addr+"/x", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := f.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != want {
				t.Errorf("status %d, want %d", resp.StatusCode, want)
			}
			if reached := app.count() > before; reached != (want == http.StatusTeapot) {
				t.Errorf("reached the app: %v", reached)
			}
		})
	}
}

// chunked hides the length from the client's Content-Length.
type chunked struct{ io.Reader }

func TestProxyCapsTheBody(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, func(c *config) { c.maxBody = 1024 })

	for _, tc := range []struct {
		name string
		body io.Reader
		want int
	}{
		{"at the cap", bytes.NewReader(bytes.Repeat([]byte("a"), 1024)), http.StatusTeapot},
		{"declared over the cap", bytes.NewReader(bytes.Repeat([]byte("a"), 1025)), http.StatusRequestEntityTooLarge},
		{"chunked under the cap", chunked{bytes.NewReader(bytes.Repeat([]byte("a"), 100))}, http.StatusTeapot},
		{"chunked over the cap", chunked{bytes.NewReader(bytes.Repeat([]byte("a"), 64<<10))}, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := app.count()
			req, err := http.NewRequest(http.MethodPost, "https://"+f.addr+"/api/v1/query", tc.body)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := f.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want != http.StatusTeapot && app.count() > before {
				if got := app.last(t); len(got.Body) > 1024 {
					t.Errorf("the app received %d body bytes past the cap", len(got.Body))
				}
			}
		})
	}
}

func TestProxyRefusesLargeHeaders(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, nil)
	req, err := http.NewRequest(http.MethodGet, "https://"+f.addr+"/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Big", strings.Repeat("a", maxHeaderBytes+8192))
	resp, err := f.client().Do(req)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
			t.Errorf("status %d, want 431", resp.StatusCode)
		}
	}
	if app.count() != 0 {
		t.Error("an oversized header block reached the app")
	}
}

// What the front says when the app is not there, and that it says nothing
// about why.
func TestProxyUpstreamFailures(t *testing.T) {
	dir := socketDir(t)
	notSocket := filepath.Join(dir, "file")
	if err := os.WriteFile(notSocket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dead := filepath.Join(dir, "dead.sock")
	ln, err := net.Listen("unix", dead)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = ln.Close()
	if err := os.Chmod(dead, 0o600); err != nil {
		t.Fatal(err)
	}
	open := startApp(t, 0o666)
	slow := startApp(t, 0o600)

	for _, tc := range []struct {
		name   string
		socket string
		path   string
		want   int
		reason string
	}{
		{"no socket", filepath.Join(dir, "absent.sock"), "/x", http.StatusBadGateway, reasonSocket},
		{"not a socket", notSocket, "/x", http.StatusBadGateway, reasonSocket},
		{"nobody listening", dead, "/x", http.StatusBadGateway, reasonDial},
		{"socket open to every local user", open.socket, "/x", http.StatusBadGateway, reasonSocket},
		{"app too slow", slow.socket, "/slow", http.StatusGatewayTimeout, reasonTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := startFront(t, tc.socket, func(c *config) { c.timeout = 300 * time.Millisecond })
			before := f.m.upstreamErrors.WithLabelValues(tc.reason).Value()
			resp, err := f.client().Get("https://" + f.addr + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.want)
			}
			if strings.Contains(string(body), dir) || strings.Contains(string(body), "sock") || strings.Contains(string(body), "dial") {
				t.Errorf("the error body describes the host: %q", body)
			}
			if got := f.m.upstreamErrors.WithLabelValues(tc.reason).Value(); got != before+1 {
				t.Errorf("upstream_errors{reason=%q} went %v to %v", tc.reason, before, got)
			}
		})
	}
	if open.count() != 0 {
		t.Error("the front used a socket any local user can open")
	}
}

func TestOpsHandler(t *testing.T) {
	app := startApp(t, 0o660)
	open := startApp(t, 0o666)
	dir := socketDir(t)

	for _, tc := range []struct {
		name   string
		socket string
		path   string
		method string
		want   int
		body   string
	}{
		{"health does not depend on the app", filepath.Join(dir, "absent.sock"), "/health", http.MethodGet, http.StatusOK, `"healthy"`},
		{"ready with the app up", app.socket, "/ready", http.MethodGet, http.StatusOK, `"ready"`},
		{"not ready without the socket", filepath.Join(dir, "absent.sock"), "/ready", http.MethodGet, http.StatusServiceUnavailable, `"unavailable"`},
		{"not ready on a world-open socket", open.socket, "/ready", http.MethodGet, http.StatusServiceUnavailable, `"unavailable"`},
		{"metrics", app.socket, "/metrics", http.MethodGet, http.StatusOK, "unheaded_zhen_ui_front_tls_client_hellos_total"},
		{"metrics carry the http middleware", app.socket, "/metrics", http.MethodGet, http.StatusOK, "unheaded_http_requests_total"},
		{"post refused", app.socket, "/health", http.MethodPost, http.StatusMethodNotAllowed, ""},
		{"the ops port is not a proxy", app.socket, "/api/v1/query", http.MethodGet, http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(config{socket: tc.socket, timeout: time.Second, maxBody: defaultMaxBody, maxConns: defaultMaxConns}, zerolog.Nop())
			f.m.hellos.WithLabelValues(kexHybrid).Add(0)
			// The middleware's series exist once the TLS handler has answered
			// something. A refused method is answered without the app, so the
			// count below stays zero and no other test has to run first.
			f.handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodTrace, "/", nil))
			srv := newLocalServer(t, f.opsHandler())
			req, err := http.NewRequest(tc.method, srv+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.body != "" && !strings.Contains(string(body), tc.body) {
				t.Errorf("body lacks %q: %.300s", tc.body, body)
			}
			if strings.Contains(string(body), dir) {
				t.Errorf("the body names a path: %s", body)
			}
		})
	}
	if app.count() != 0 || open.count() != 0 {
		t.Error("a readiness probe sent the app a request")
	}
}

func newLocalServer(t *testing.T, h http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

func TestParseFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
		check   func(*testing.T, config)
	}{
		{"defaults", nil, "", func(t *testing.T, c config) {
			if len(c.listens) != 1 || c.listens[0].String() != "127.0.0.1:20103" {
				t.Errorf("listens %v", c.listens)
			}
			if c.metrics.String() != "127.0.0.1:20112" {
				t.Errorf("metrics %v", c.metrics)
			}
			if c.socket != "/run/unheaded-zhen-ui/app.sock" {
				t.Errorf("socket %q", c.socket)
			}
			if c.timeout != defaultTimeout || c.maxBody != defaultMaxBody || c.maxConns != defaultMaxConns {
				t.Errorf("limits %v %d %d", c.timeout, c.maxBody, c.maxConns)
			}
		}},
		{"two listeners", []string{"--listen", "127.0.0.1:20103", "--listen", "[::1]:20103"}, "", func(t *testing.T, c config) {
			if len(c.listens) != 2 {
				t.Errorf("listens %v", c.listens)
			}
		}},
		{"lan address", []string{"--listen", "10.10.20.2:20103"}, "", nil},
		{"wildcard v4", []string{"--listen", "0.0.0.0:20103"}, "wildcard", nil},
		{"wildcard v6", []string{"--listen", "[::]:20103"}, "wildcard", nil},
		{"bare port", []string{"--listen", ":20103"}, "--listen", nil},
		{"hostname", []string{"--listen", "localhost:20103"}, "--listen", nil},
		{"same listener twice", []string{"--listen", "127.0.0.1:20103", "--listen", "127.0.0.1:20103"}, "twice", nil},
		{"metrics off loopback", []string{"--metrics", "10.10.20.2:20112"}, "loopback", nil},
		{"metrics wildcard", []string{"--metrics", "0.0.0.0:20112"}, "--metrics", nil},
		{"metrics on the tls port", []string{"--metrics", "127.0.0.1:20103"}, "--metrics", nil},
		{"relative socket", []string{"--socket", "run/app.sock"}, "absolute", nil},
		{"empty socket", []string{"--socket", ""}, "absolute", nil},
		{"socket path too long", []string{"--socket", "/" + strings.Repeat("a", 120)}, "long", nil},
		{"socket path not clean", []string{"--socket", "/run/x/../app.sock"}, "clean", nil},
		{"zero timeout", []string{"--timeout", "0s"}, "--timeout", nil},
		{"huge timeout", []string{"--timeout", "48h"}, "--timeout", nil},
		{"zero body", []string{"--max-body", "0"}, "--max-body", nil},
		{"huge body", []string{"--max-body", "1099511627776"}, "--max-body", nil},
		{"zero conns", []string{"--max-conns", "0"}, "--max-conns", nil},
		{"operand", []string{"extra"}, "", nil},
		{"no tls flags exist", []string{"--cert", "/x"}, "", nil},
		{"no plaintext switch exists", []string{"--insecure"}, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer
			c, err := parseFlags(tc.args, io.Discard, &errOut)
			if tc.wantErr == "" && tc.check == nil && tc.name != "lan address" {
				if err == nil {
					t.Fatal("accepted")
				}
				return
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("accepted")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v (%s)", err, errOut.String())
			}
			if tc.check != nil {
				tc.check(t, c)
			}
		})
	}
}

func TestClientAddr(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:5555":          "127.0.0.1",
		"10.10.20.7:1":            "10.10.20.7",
		"[::1]:443":               "::1",
		"[::ffff:10.0.0.1]:9":     "10.0.0.1",
		"[fe80::1%eth0]:9":        "fe80::1",
		"[fd00:dead:beef::2]:123": "fd00:dead:beef::2",
		"":                        "",
		"@":                       "",
		"not-an-address":          "",
		"10.0.0.1":                "",
	} {
		if got := clientAddr(in); got != want {
			t.Errorf("clientAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

// The listener stops accepting at --max-conns and takes the next client when
// one leaves.
func TestConnectionLimit(t *testing.T) {
	app := startApp(t, 0o600)
	f := startFront(t, app.socket, func(c *config) { c.maxConns = 2 })

	hold := func() net.Conn {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", f.addr, f.clientTLS(nil))
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	a, b := hold(), hold()
	defer func() { _ = b.Close() }()

	// The third TCP connection is queued by the kernel, never served.
	third, err := net.DialTimeout("tcp", f.addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = third.Close() }()
	tc := tls.Client(third, f.clientTLS(nil))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	err = tc.HandshakeContext(ctx)
	cancel()
	if err == nil {
		t.Fatal("a third connection was served past --max-conns 2")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("third handshake: %v", err)
	}

	_ = a.Close()
	var ok bool
	for range 50 {
		if _, err := f.dial(nil); err == nil {
			ok = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		t.Fatal("no slot came free after a client left")
	}
}

func TestErrorLogWriterDoesNotFlood(t *testing.T) {
	var buf bytes.Buffer
	w := newErrorLogWriter(zerolog.New(&buf))
	for i := range 500 {
		if _, err := fmt.Fprintf(w, "http: TLS handshake error from 10.0.0.%d:1: bad\n", i%250); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Count(buf.String(), "\n")
	if lines == 0 {
		t.Fatal("nothing logged")
	}
	if lines > errorLogBurst {
		t.Errorf("%d lines for 500 handshake errors, want at most %d", lines, errorLogBurst)
	}
	if !strings.Contains(buf.String(), "TLS handshake error") {
		t.Errorf("the message was lost: %s", buf.String())
	}
}
