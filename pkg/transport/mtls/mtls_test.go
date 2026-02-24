package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateRootCA(t *testing.T) {
	certPEM, keyPEM, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatalf("GenerateRootCA: %v", err)
	}

	// Verify cert is valid PEM.
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("invalid cert PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	if !cert.IsCA {
		t.Error("cert is not a CA")
	}
	if cert.Subject.CommonName != "Unheaded Root CA" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}

	// Verify key is valid PEM.
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		t.Fatal("invalid key PEM")
	}
}

func TestGenerateServiceCert(t *testing.T) {
	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	certPEM, keyPEM, err := GenerateServiceCert(caCert, caKey, "gateway", 12*time.Hour)
	if err != nil {
		t.Fatalf("GenerateServiceCert: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("invalid cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse service cert: %v", err)
	}

	if cert.Subject.CommonName != "gateway.services.internal" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}

	// Check SANs.
	foundDNS := false
	for _, dns := range cert.DNSNames {
		if dns == "gateway.services.internal" {
			foundDNS = true
		}
	}
	if !foundDNS {
		t.Error("missing SAN for gateway.services.internal")
	}

	foundIP := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			foundIP = true
		}
	}
	if !foundIP {
		t.Error("missing SAN for 127.0.0.1")
	}

	// Verify cert is signed by CA.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)
	opts := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("cert verification failed: %v", err)
	}

	// Verify key is valid.
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		t.Fatal("invalid service key PEM")
	}
}

func TestWritePKI(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	services := []string{"gateway", "wotan", "dashboard"}
	if err := WritePKI(dir, caCert, caKey, services, 12*time.Hour); err != nil {
		t.Fatalf("WritePKI: %v", err)
	}

	// Verify files exist.
	for _, f := range []string{"root-ca.crt", "root-ca.key"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	for _, svc := range services {
		for _, ext := range []string{".crt", ".key"} {
			path := filepath.Join(dir, svc+ext)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("missing %s: %v", path, err)
			}
		}
	}

	// Verify permissions.
	info, _ := os.Stat(filepath.Join(dir, "root-ca.key"))
	if info.Mode().Perm() != 0600 {
		t.Errorf("CA key permissions: got %o, want 0600", info.Mode().Perm())
	}
}

func TestServerClientTLSConfig(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"server", "client"}, 12*time.Hour); err != nil {
		t.Fatal(err)
	}

	serverCfg := Config{PKIDir: dir, ServiceName: "server"}
	serverTLS, err := ServerTLSConfig(serverCfg)
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}

	clientCfg := Config{PKIDir: dir, ServiceName: "client"}
	clientTLS, err := ClientTLSConfig(clientCfg)
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}

	// Start a TLS server.
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	go srv.Serve(ln)
	defer srv.Close()

	// Connect as mTLS client.
	clientTLS.ServerName = "server.services.internal"
	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: clientTLS}}

	resp, err := httpClient.Get("https://" + ln.Addr().String() + "/health")
	if err != nil {
		t.Fatalf("client request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCertRotator(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"test"}, 12*time.Hour); err != nil {
		t.Fatal(err)
	}

	cfg := Config{PKIDir: dir, ServiceName: "test"}
	rotator, err := NewCertRotator(cfg)
	if err != nil {
		t.Fatalf("NewCertRotator: %v", err)
	}

	// Get certificate.
	cert, err := rotator.GetCertificate(nil)
	if err != nil || cert == nil {
		t.Fatalf("GetCertificate: %v", err)
	}

	// Get client certificate.
	clientCert, err := rotator.GetClientCertificate(nil)
	if err != nil || clientCert == nil {
		t.Fatalf("GetClientCertificate: %v", err)
	}

	// CA pool should be non-nil.
	pool := rotator.CAPool()
	if pool == nil {
		t.Error("CAPool returned nil")
	}

	// Start and stop should not panic.
	rotator.Start(100 * time.Millisecond)
	time.Sleep(250 * time.Millisecond)
	rotator.Stop()
}
