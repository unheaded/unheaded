// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package integration

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	unheadedtls "unheaded/pkg/tls"
)

// setupMTLSServer creates a TLS test server with mTLS enabled. Returns the
// listener address, a cleanup function, and the CA used to sign the server cert.
func setupMTLSServer(t *testing.T) (addr string, ca *unheadedtls.CA, serverCert *unheadedtls.ServiceCert) {
	t.Helper()

	var err error
	ca, err = unheadedtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	serverCert, err = ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "test-server",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert(server): %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	tlsCfg, err := unheadedtls.NewServerTLSConfig(unheadedtls.ServerConfig{
		CertPEM:           serverCert.CertPEM,
		KeyPEM:            serverCert.KeyPEM,
		CAPEM:             ca.CertPEM(),
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}

	srv := &http.Server{
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	return ln.Addr().String(), ca, serverCert
}

// buildClient creates an HTTP client with the given TLS config.
func buildClient(t *testing.T, tlsCfg *tls.Config) *http.Client {
	t.Helper()
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}
}

func TestMTLS_ValidCerts_Returns200(t *testing.T) {
	addr, ca, _ := setupMTLSServer(t)

	// Issue a client cert from the same CA.
	clientCert, err := ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "test-client",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert(client): %v", err)
	}

	tlsCfg, err := unheadedtls.NewClientTLSConfig(unheadedtls.ClientConfig{
		CertPEM: clientCert.CertPEM,
		KeyPEM:  clientCert.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewClientTLSConfig: %v", err)
	}

	client := buildClient(t, tlsCfg)
	resp, err := client.Get("https://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want %q", body["status"], "healthy")
	}

	// Verify TLS 1.3 is used.
	if resp.TLS == nil {
		t.Fatal("TLS state is nil")
	}
	if resp.TLS.Version != tls.VersionTLS13 {
		t.Errorf("TLS version = 0x%04x, want TLS 1.3 (0x%04x)", resp.TLS.Version, tls.VersionTLS13)
	}
}

func TestMTLS_NoCert_Rejected(t *testing.T) {
	addr, ca, _ := setupMTLSServer(t)

	// Client with CA trust but NO client certificate.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.CertPEM())

	tlsCfg := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS13,
	}

	client := buildClient(t, tlsCfg)
	resp, err := client.Get("https://" + addr + "/health")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error when client presents no certificate")
	}
}

func TestMTLS_WrongCA_Rejected(t *testing.T) {
	addr, _, _ := setupMTLSServer(t)

	// Create a completely different CA and issue a client cert from it.
	wrongCA, err := unheadedtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA(wrong): %v", err)
	}

	wrongCert, err := wrongCA.IssueCert(unheadedtls.CertRequest{
		ServiceName: "imposter",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert(wrong): %v", err)
	}

	tlsCfg, err := unheadedtls.NewClientTLSConfig(unheadedtls.ClientConfig{
		CertPEM: wrongCert.CertPEM,
		KeyPEM:  wrongCert.KeyPEM,
		CAPEM:   wrongCA.CertPEM(), // trust the wrong CA
	})
	if err != nil {
		t.Fatalf("NewClientTLSConfig: %v", err)
	}

	client := buildClient(t, tlsCfg)
	resp, err := client.Get("https://" + addr + "/health")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error when client cert is signed by wrong CA")
	}
}

func TestMTLS_ExpiredCert_Rejected(t *testing.T) {
	addr, ca, _ := setupMTLSServer(t)

	// Issue a cert with a TTL of 1ms — it will expire immediately.
	expiredCert, err := ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "expired-client",
		TTL:         1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("IssueCert(expired): %v", err)
	}

	// Wait for cert to expire.
	time.Sleep(20 * time.Millisecond)

	tlsCfg, err := unheadedtls.NewClientTLSConfig(unheadedtls.ClientConfig{
		CertPEM: expiredCert.CertPEM,
		KeyPEM:  expiredCert.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewClientTLSConfig: %v", err)
	}

	client := buildClient(t, tlsCfg)
	resp, err := client.Get("https://" + addr + "/health")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error when client cert is expired")
	}
}

func TestMTLS_TLS12Rejected(t *testing.T) {
	addr, ca, _ := setupMTLSServer(t)

	clientCert, err := ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "tls12-client",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	cert, err := tls.X509KeyPair(clientCert.CertPEM, clientCert.KeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.CertPEM())

	// Force TLS 1.2 max — server requires 1.3.
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
	}

	client := buildClient(t, tlsCfg)
	resp, err := client.Get("https://" + addr + "/health")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error when client uses TLS 1.2 (server requires 1.3)")
	}
}

func TestMTLS_ServerConfigFactory(t *testing.T) {
	ca, err := unheadedtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	srvCert, err := ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "factory-server",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	srv, err := unheadedtls.NewTLSServer("127.0.0.1:0", handler, unheadedtls.ServerConfig{
		CertPEM:           srvCert.CertPEM,
		KeyPEM:            srvCert.KeyPEM,
		CAPEM:             ca.CertPEM(),
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("NewTLSServer: %v", err)
	}

	// Verify the server was configured correctly.
	if srv.TLSConfig == nil {
		t.Fatal("TLSConfig is nil")
	}
	if srv.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = 0x%04x, want TLS 1.3", srv.TLSConfig.MinVersion)
	}
	if srv.TLSConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", srv.TLSConfig.ClientAuth)
	}
}

func TestMTLS_ClientConfigFactory(t *testing.T) {
	ca, err := unheadedtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	clientCert, err := ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "factory-client",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	client, err := unheadedtls.NewTLSHTTPClient(unheadedtls.ClientConfig{
		CertPEM: clientCert.CertPEM,
		KeyPEM:  clientCert.KeyPEM,
		CAPEM:   ca.CertPEM(),
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTLSHTTPClient: %v", err)
	}

	if client.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", client.Timeout)
	}
}

func TestMTLS_GRPCCredentials(t *testing.T) {
	ca, err := unheadedtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	srvCert, err := ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "grpc-server",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert(server): %v", err)
	}

	creds, err := unheadedtls.NewServerTransportCredentials(unheadedtls.ServerConfig{
		CertPEM:           srvCert.CertPEM,
		KeyPEM:            srvCert.KeyPEM,
		CAPEM:             ca.CertPEM(),
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("NewServerTransportCredentials: %v", err)
	}

	if creds == nil {
		t.Fatal("server credentials are nil")
	}

	clientCert, err := ca.IssueCert(unheadedtls.CertRequest{
		ServiceName: "grpc-client",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert(client): %v", err)
	}

	clientCreds, err := unheadedtls.NewClientTransportCredentials(unheadedtls.ClientConfig{
		CertPEM: clientCert.CertPEM,
		KeyPEM:  clientCert.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewClientTransportCredentials: %v", err)
	}

	if clientCreds == nil {
		t.Fatal("client credentials are nil")
	}
}

func TestMTLS_SetupDisabled(t *testing.T) {
	// When UNHEADED_TLS_ENABLED is not set, MaybeWrapServer returns plain server.
	t.Setenv("UNHEADED_TLS_ENABLED", "false")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv, tlsEnabled, err := unheadedtls.MaybeWrapServer(":0", handler)
	if err != nil {
		t.Fatalf("MaybeWrapServer: %v", err)
	}
	if tlsEnabled {
		t.Error("expected TLS disabled when UNHEADED_TLS_ENABLED=false")
	}
	if srv.TLSConfig != nil {
		t.Error("expected nil TLSConfig when TLS disabled")
	}
}

func TestMTLS_EndToEnd_MultipleServices(t *testing.T) {
	// Simulate a mesh of services all trusting the same CA.
	ca, err := unheadedtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	services := []string{"timeguru", "captain", "architect"}
	certs := make(map[string]*unheadedtls.ServiceCert)

	for _, svc := range services {
		cert, err := ca.IssueCert(unheadedtls.CertRequest{
			ServiceName: svc,
			TTL:         1 * time.Hour,
		})
		if err != nil {
			t.Fatalf("IssueCert(%s): %v", svc, err)
		}
		certs[svc] = cert
	}

	// Start a server as "timeguru".
	srvCert := certs["timeguru"]
	tlsCfg, err := unheadedtls.NewServerTLSConfig(unheadedtls.ServerConfig{
		CertPEM:           srvCert.CertPEM,
		KeyPEM:            srvCert.KeyPEM,
		CAPEM:             ca.CertPEM(),
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/timeline", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"phase": "alpha"})
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	// Each other service should be able to connect.
	for _, clientSvc := range []string{"captain", "architect"} {
		t.Run(clientSvc+"->timeguru", func(t *testing.T) {
			clientTLS, err := unheadedtls.NewClientTLSConfig(unheadedtls.ClientConfig{
				CertPEM: certs[clientSvc].CertPEM,
				KeyPEM:  certs[clientSvc].KeyPEM,
				CAPEM:   ca.CertPEM(),
			})
			if err != nil {
				t.Fatalf("NewClientTLSConfig: %v", err)
			}

			client := &http.Client{
				Timeout:   5 * time.Second,
				Transport: &http.Transport{TLSClientConfig: clientTLS},
			}

			resp, err := client.Get("https://" + ln.Addr().String() + "/api/v1/timeline")
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
		})
	}
}

// Ensure unused import is referenced.
var _ = net.IPv4
