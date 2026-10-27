// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Environment variables every service reads (S-IRON-GATE Phase 8, FND-017).
const (
	EnvEnabled = "UNHEADED_TLS_ENABLED"
	EnvCACert  = "UNHEADED_TLS_CA_CERT"
	EnvCert    = "UNHEADED_TLS_CERT"
	EnvKeyFile = "UNHEADED_TLS_KEY_FILE"
	EnvMTLS    = "UNHEADED_TLS_MTLS_ENABLED"
	// EnvProfile picks the listener's key-exchange profile (ADR-105):
	// "strict" (default: hybrid ML-KEM only, a classical-only client is
	// refused) or "preferred" (hybrid first, classical fallback) for a
	// listener third-party clients without ML-KEM must reach.
	EnvProfile = "UNHEADED_TLS_PROFILE"
)

// ServiceTLSConfig is loaded from environment variables. All fields are optional.
// When Enabled is false, the TLS layer is a no-op and services run over plain HTTP.
type ServiceTLSConfig struct {
	// Enabled is true when UNHEADED_TLS_ENABLED=true.
	Enabled bool
	// CertFile is the path to the PEM-encoded service certificate (UNHEADED_TLS_CERT).
	CertFile string
	// KeyFile is the path to the PEM-encoded service private key (UNHEADED_TLS_KEY_FILE).
	KeyFile string
	// CAFile is the path to the PEM-encoded CA certificate (UNHEADED_TLS_CA_CERT).
	CAFile string
	// RequireClientCert enables mTLS when true (UNHEADED_TLS_MTLS_ENABLED, default true).
	RequireClientCert bool
	// Profile is UNHEADED_TLS_PROFILE: "" or "strict", or "preferred".
	Profile string
}

// LoadServiceTLSConfig reads TLS configuration from environment variables.
// Returns a config with Enabled=false when UNHEADED_TLS_ENABLED is not "true",
// allowing non-TLS operation with zero code changes.
func LoadServiceTLSConfig() ServiceTLSConfig {
	cfg := ServiceTLSConfig{
		Enabled:           Enabled(),
		CertFile:          strings.TrimSpace(os.Getenv(EnvCert)),
		KeyFile:           strings.TrimSpace(os.Getenv(EnvKeyFile)),
		CAFile:            strings.TrimSpace(os.Getenv(EnvCACert)),
		RequireClientCert: os.Getenv(EnvMTLS) != "false",
		Profile:           strings.ToLower(strings.TrimSpace(os.Getenv(EnvProfile))),
	}
	return cfg
}

// Enabled reports whether UNHEADED_TLS_ENABLED is exactly "true".
func Enabled() bool { return os.Getenv(EnvEnabled) == "true" }

// Scheme is "https" when UNHEADED_TLS_ENABLED=true, else "http": the scheme
// for a URL to another first-party service.
func Scheme() string {
	if Enabled() {
		return "https"
	}
	return "http"
}

// BaseURL turns a host:port into a URL with Scheme(). A value that already
// has a scheme is kept as written (minus a trailing slash): an explicit
// http:// names a third-party plaintext endpoint on purpose.
func BaseURL(hostport string) string {
	s := strings.TrimRight(strings.TrimSpace(hostport), "/")
	if strings.Contains(s, "://") {
		return s
	}
	return Scheme() + "://" + s
}

// EndpointURL turns an endpoint written as "host:port" or
// "scheme://host:port" into a base URL. An explicit scheme wins; otherwise
// a first-party listener (tlsCapable: it serves through pkg/tls) gets
// Scheme(), and anything else plain http.
func EndpointURL(endpoint string, tlsCapable bool) string {
	s := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.Contains(s, "://") {
		return s
	}
	if tlsCapable {
		return Scheme() + "://" + s
	}
	return "http://" + s
}

// serverTLSConfig builds the listener config from cfg, or explains what is
// missing. It never returns a nil config without an error.
func (cfg ServiceTLSConfig) serverTLSConfig() (*tls.Config, error) {
	if cfg.CertFile == "" {
		return nil, fmt.Errorf("tls: %s=true but %s is not set", EnvEnabled, EnvCert)
	}
	if cfg.KeyFile == "" {
		return nil, fmt.Errorf("tls: %s=true but %s is not set", EnvEnabled, EnvKeyFile)
	}
	certPEM, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		return nil, fmt.Errorf("tls: read %s: %w", EnvCert, err)
	}
	keyPEM, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("tls: read %s: %w", EnvKeyFile, err)
	}
	var caPEM []byte
	if cfg.RequireClientCert {
		if cfg.CAFile == "" {
			return nil, fmt.Errorf("tls: mTLS (%s) needs %s", EnvMTLS, EnvCACert)
		}
		if caPEM, err = os.ReadFile(cfg.CAFile); err != nil {
			return nil, fmt.Errorf("tls: read %s: %w", EnvCACert, err)
		}
	}
	tlsCfg, err := NewServerTLSConfig(ServerConfig{
		CertPEM:           certPEM,
		KeyPEM:            keyPEM,
		CAPEM:             caPEM,
		RequireClientCert: cfg.RequireClientCert,
	})
	if err != nil {
		return nil, err
	}
	switch cfg.Profile {
	case "", "strict":
		ApplyStrict(tlsCfg)
	case "preferred":
		ApplyPreferred(tlsCfg)
	default:
		return nil, fmt.Errorf("tls: %s=%q: want strict or preferred", EnvProfile, cfg.Profile)
	}
	return tlsCfg, nil
}

// ConfigureServer installs TLS on srv from the environment. With
// UNHEADED_TLS_ENABLED=true it sets srv.TLSConfig (TLS 1.3, Strict hybrid
// key exchange unless UNHEADED_TLS_PROFILE=preferred) and returns true; any
// missing or unreadable file is an error, never a plaintext listener. With
// TLS off it leaves srv alone and, when srv.Addr is not a loopback address,
// logs a WARN naming FND-017 so a plaintext service on the network stays
// visible. Serve or ListenAndServe then start the right kind of listener.
func ConfigureServer(srv *http.Server) (bool, error) {
	if srv == nil {
		return false, errors.New("tls: ConfigureServer: nil server")
	}
	cfg := LoadServiceTLSConfig()
	if !cfg.Enabled {
		if !IsLoopbackListenAddr(srv.Addr) {
			warnPlaintext(srv.Addr)
		}
		return false, nil
	}
	tlsCfg, err := cfg.serverTLSConfig()
	if err != nil {
		return false, err
	}
	srv.TLSConfig = tlsCfg
	return true, nil
}

// ListenAndServe starts srv as TLS when it has a TLSConfig
// (ConfigureServer), else as plain HTTP.
func ListenAndServe(srv *http.Server) error {
	if srv.TLSConfig != nil {
		return srv.ListenAndServeTLS("", "")
	}
	return srv.ListenAndServe()
}

// Serve is ListenAndServe on a listener the caller opened.
func Serve(srv *http.Server, ln net.Listener) error {
	if srv.TLSConfig != nil {
		return srv.ServeTLS(ln, "", "")
	}
	return srv.Serve(ln)
}

var plaintextWarned sync.Map

func warnPlaintext(addr string) {
	if _, dup := plaintextWarned.LoadOrStore(addr, true); dup {
		return
	}
	log.Warn().Str("addr", addr).
		Msg("plaintext listener on non-loopback address (FND-017): set UNHEADED_TLS_ENABLED=true with UNHEADED_TLS_CERT/UNHEADED_TLS_KEY_FILE/UNHEADED_TLS_CA_CERT")
}

// IsLoopbackListenAddr reports whether a listen address binds loopback
// only. ":port", "0.0.0.0" and "[::]" bind every interface.
func IsLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return IsLoopbackHost(host)
}

// IsLoopbackHost reports whether host (a name or IP, no port) is loopback.
func IsLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// MaybeWrapServer creates an *http.Server for addr with TLS from the
// environment (ConfigureServer). With TLS off it is a plain server with
// the same timeouts; with TLS asked for but misconfigured it returns an
// error and no server.
//
// Usage:
//
//	srv, tlsEnabled, err := tls.MaybeWrapServer(":8443", handler)
//	if err != nil { log.Fatal(err) }  // never fall back to plaintext
//	tls.ListenAndServe(srv)
func MaybeWrapServer(addr string, handler http.Handler) (srv *http.Server, tlsEnabled bool, err error) {
	// The TLS path in pkg/tls/server.go sets these; this plaintext path did
	// not, leaving the non-TLS listener exposed to slowloris. gosec G112.
	srv = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	on, err := ConfigureServer(srv)
	if err != nil {
		return nil, false, err
	}
	return srv, on, nil
}

// ClientTLSConfigFromEnv is the client side of an internal link: nil and
// false with TLS off; with UNHEADED_TLS_ENABLED=true a Strict TLS 1.3
// config that trusts only the deployment CA (UNHEADED_TLS_CA_CERT, required)
// and presents the service certificate when UNHEADED_TLS_CERT and
// UNHEADED_TLS_KEY_FILE are both set (mTLS). The server name is verified
// against the dialled host, so the certificate's SANs must name it.
func ClientTLSConfigFromEnv() (*tls.Config, bool, error) {
	cfg := LoadServiceTLSConfig()
	if !cfg.Enabled {
		return nil, false, nil
	}
	if cfg.CAFile == "" {
		return nil, true, fmt.Errorf("tls: %s=true but %s is not set", EnvEnabled, EnvCACert)
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, true, fmt.Errorf("tls: read %s: %w", EnvCACert, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, true, fmt.Errorf("tls: %s holds no PEM certificate", EnvCACert)
	}
	tlsCfg := ApplyStrict(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool})
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, true, fmt.Errorf("tls: client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	return tlsCfg, true, nil
}

// ClientFromEnv returns the *http.Client a service uses for internal links
// (another first-party service): over TLS with ClientTLSConfigFromEnv when
// UNHEADED_TLS_ENABLED=true, else plain. It is a fresh client with the
// given timeout (0: 30 s), never http.DefaultClient. Use it with BaseURL or
// an https:// URL; it trusts only the deployment CA, so it is not for
// public endpoints.
func ClientFromEnv(timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	tr, err := TransportFromEnv()
	if err != nil {
		return nil, err
	}
	return &http.Client{Timeout: timeout, Transport: tr}, nil
}

// TransportFromEnv is ClientFromEnv's transport: a clone of
// http.DefaultTransport with the internal-link TLS config when TLS is on.
func TransportFromEnv() (*http.Transport, error) {
	tlsCfg, _, err := ClientTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	tr := base.Clone()
	if tlsCfg != nil {
		tr.TLSClientConfig = tlsCfg
	}
	return tr, nil
}

// TransportTrustingDeploymentCA is for a link that may lead to a public
// endpoint or to an internal one, such as an OIDC issuer (the built-in
// unheaded-auth or another provider): a clone of http.DefaultTransport
// that, when UNHEADED_TLS_ENABLED=true, trusts the system roots plus the
// deployment CA (UNHEADED_TLS_CA_CERT, required). Key exchange stays Go's
// default (hybrid first), since a public peer may not offer ML-KEM.
func TransportTrustingDeploymentCA() (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	tr := base.Clone()
	cfg := LoadServiceTLSConfig()
	if !cfg.Enabled {
		return tr, nil
	}
	if cfg.CAFile == "" {
		return nil, fmt.Errorf("tls: %s=true but %s is not set", EnvEnabled, EnvCACert)
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("tls: read %s: %w", EnvCACert, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("tls: %s holds no PEM certificate", EnvCACert)
	}
	tr.TLSClientConfig = ApplyPreferred(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool})
	return tr, nil
}

// GRPCClientCredentialsFromEnv returns TLS transport credentials for a gRPC
// client of an internal service when UNHEADED_TLS_ENABLED=true (nil, false
// otherwise). The caller decides what a plaintext dial means
// (PlaintextGRPCCredentials).
func GRPCClientCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsCfg, on, err := ClientTLSConfigFromEnv()
	if err != nil || !on {
		return nil, on, err
	}
	return credentials.NewTLS(tlsCfg), true, nil
}

var plaintextGRPCWarned sync.Map

// PlaintextGRPCCredentials is the one place a first-party gRPC client gets
// plaintext transport credentials. It is refused (an error) when
// UNHEADED_TLS_ENABLED=true: a service that was told to use TLS never
// dials in the clear. Otherwise it logs a WARN once per reason.
func PlaintextGRPCCredentials(reason string) (credentials.TransportCredentials, error) {
	if Enabled() {
		return nil, fmt.Errorf("tls: plaintext gRPC (%s) refused: %s=true", reason, EnvEnabled)
	}
	if _, dup := plaintextGRPCWarned.LoadOrStore(reason, true); !dup {
		log.Warn().Str("link", reason).Msg("gRPC over plaintext (FND-017): set UNHEADED_TLS_ENABLED=true and UNHEADED_TLS_CA_CERT")
	}
	return insecure.NewCredentials(), nil
}

// MaybeCreateClient creates an *http.Client for internal links: TLS from
// the environment when enabled (ClientFromEnv), else a plain client.
func MaybeCreateClient() (*http.Client, error) {
	return ClientFromEnv(0)
}
