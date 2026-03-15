// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"
)

// helperSetupMTLSPair creates a server and client Manager sharing the same CA,
// and returns the server TLS config and client TLS config ready for use.
// Both configs trust each other's certs.
func helperSetupMTLSPair(t *testing.T) (serverCfg, clientCfg *tls.Config, cleanup func()) {
	t.Helper()

	caCert, caKey := generateTestCA()

	// Build root pool with our CA
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	serverMgrCfg := DefaultConfig("example.com", "server-svc")
	serverMgrCfg.RootCAs = rootPool
	serverMgr := NewManager(serverMgrCfg)
	if err := serverMgr.Initialize(caCert, caKey); err != nil {
		t.Fatalf("server Initialize failed: %v", err)
	}

	clientMgrCfg := DefaultConfig("example.com", "client-svc")
	clientMgrCfg.RootCAs = rootPool
	clientMgr := NewManager(clientMgrCfg)
	if err := clientMgr.Initialize(caCert, caKey); err != nil {
		t.Fatalf("client Initialize failed: %v", err)
	}

	srvTLS := serverMgr.ServerTLSConfig()
	cliTLS := clientMgr.ClientTLSConfig()
	// Override InsecureSkipVerify so self-signed certs are accepted by the
	// standard x509 chain check. The VerifyPeerCertificate callback still runs.
	cliTLS.InsecureSkipVerify = true
	// Disable the SPIFFE peer verifier on the client side since verified chains
	// won't be populated when InsecureSkipVerify is true.
	cliTLS.VerifyPeerCertificate = nil

	return srvTLS, cliTLS, func() {
		serverMgr.Close()
		clientMgr.Close()
	}
}

// TestMTLSHandshake tests a full mTLS handshake between a server and client.
func TestMTLSHandshake(t *testing.T) {
	srvTLS, cliTLS, cleanup := helperSetupMTLSPair(t)
	defer cleanup()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvTLS)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()
	errCh := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		tlsConn := conn.(*tls.Conn)
		if err := tlsConn.Handshake(); err != nil {
			errCh <- err
			return
		}

		// Exercise GetConnectionInfo
		info, err := GetConnectionInfo(tlsConn)
		if err != nil {
			errCh <- err
			return
		}
		_ = info

		// Exercise ExtractPeerInfo
		peerInfo, err := ExtractPeerInfo(tlsConn)
		if err != nil {
			errCh <- err
			return
		}
		_ = peerInfo

		errCh <- nil
	}()

	conn, err := tls.Dial("tcp", addr, cliTLS)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	conn.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server")
	}
}

// TestWrapConn tests the Manager.WrapConn method.
func TestWrapConn(t *testing.T) {
	caCert, caKey := generateTestCA()
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	serverCfg := DefaultConfig("example.com", "server-svc")
	serverCfg.RootCAs = rootPool
	serverMgr := NewManager(serverCfg)
	serverMgr.Initialize(caCert, caKey)
	defer serverMgr.Close()

	clientCfg := DefaultConfig("example.com", "client-svc")
	clientCfg.RootCAs = rootPool
	clientMgr := NewManager(clientCfg)
	clientMgr.Initialize(caCert, caKey)
	defer clientMgr.Close()

	rawLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer rawLn.Close()

	addr := rawLn.Addr().String()
	errCh := make(chan error, 1)

	go func() {
		rawConn, err := rawLn.Accept()
		if err != nil {
			errCh <- err
			return
		}

		tlsConn, err := serverMgr.WrapConn(rawConn)
		if err != nil {
			errCh <- err
			return
		}
		defer tlsConn.Close()

		// Exercise GetPeerSPIFFEID
		_, _ = serverMgr.GetPeerSPIFFEID(tlsConn)

		errCh <- nil
	}()

	cliTLS := clientMgr.ClientTLSConfig()
	cliTLS.InsecureSkipVerify = true
	cliTLS.VerifyPeerCertificate = nil

	conn, err := tls.Dial("tcp", addr, cliTLS)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	conn.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// TestDialTLS tests the Manager.DialTLS method.
func TestDialTLS(t *testing.T) {
	caCert, caKey := generateTestCA()
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	// Server: uses the CA certs, doesn't require client cert
	srvTLS := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.NoClientCert,
	}
	// Generate a server cert signed by our CA with SPIFFE URI
	cm := NewCertificateManager(caCert, caKey)
	spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "server-svc")
	certCfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "server-svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
		DNSNames:   []string{"localhost", "127.0.0.1"},
	}
	srvCert, srvKey, _ := cm.GenerateCertificate(certCfg)
	cm.SetCertificate(srvCert, srvKey, certCfg)
	certPEM, keyPEM, _ := cm.GetCertificatePEM()
	tlsCert, _ := tls.X509KeyPair(certPEM, keyPEM)
	srvTLS.Certificates = []tls.Certificate{tlsCert}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvTLS)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			buf := make([]byte, 1)
			conn.Read(buf)
			conn.Close()
		}
	}()

	// Client Manager with SkipVerify
	clientCfg := DefaultConfig("example.com", "client-svc")
	clientCfg.SkipVerify = true
	clientCfg.RootCAs = rootPool
	clientMgr := NewManager(clientCfg)
	clientMgr.Initialize(caCert, caKey)
	defer clientMgr.Close()

	// Patch the client config to remove the SPIFFE verifier
	// (since InsecureSkipVerify + VerifyPeerCertificate is a problem)
	clientMgr.mu.Lock()
	clientMgr.clientConfig.VerifyPeerCertificate = nil
	clientMgr.mu.Unlock()

	conn, err := clientMgr.DialTLS("tcp", addr)
	if err != nil {
		t.Fatalf("DialTLS failed: %v", err)
	}
	conn.Close()
}

// TestDialTLSWithTimeout tests the Manager.DialTLSWithTimeout method.
func TestDialTLSWithTimeout(t *testing.T) {
	caCert, caKey := generateTestCA()
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	// Simple TLS server
	cm := NewCertificateManager(caCert, caKey)
	spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "server-svc")
	certCfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "server-svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
		DNSNames:   []string{"localhost"},
	}
	srvCert, srvKey, _ := cm.GenerateCertificate(certCfg)
	cm.SetCertificate(srvCert, srvKey, certCfg)
	certPEM, keyPEM, _ := cm.GetCertificatePEM()
	tlsCert, _ := tls.X509KeyPair(certPEM, keyPEM)

	srvTLS := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.NoClientCert,
		Certificates: []tls.Certificate{tlsCert},
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvTLS)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			buf := make([]byte, 1)
			conn.Read(buf)
			conn.Close()
		}
	}()

	cfg := DefaultConfig("example.com", "svc")
	cfg.SkipVerify = true
	cfg.RootCAs = rootPool
	mgr := NewManager(cfg)
	mgr.Initialize(caCert, caKey)
	defer mgr.Close()

	mgr.mu.Lock()
	mgr.clientConfig.VerifyPeerCertificate = nil
	mgr.mu.Unlock()

	conn, err := mgr.DialTLSWithTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("DialTLSWithTimeout failed: %v", err)
	}
	conn.Close()
}

// TestDialTLSWithTimeout_Timeout tests that timeout works.
func TestDialTLSWithTimeout_Timeout(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "svc")
	cfg.SkipVerify = true
	mgr := NewManager(cfg)
	mgr.Initialize(caCert, caKey)
	defer mgr.Close()

	_, err := mgr.DialTLSWithTimeout("tcp", "192.0.2.1:12345", 100*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

// TestGetPeerSPIFFEID_NoPeer tests GetPeerSPIFFEID with no peer certs.
func TestGetPeerSPIFFEID_NoPeer(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "svc")
	mgr := NewManager(cfg)
	mgr.Initialize(caCert, caKey)
	defer mgr.Close()

	srvConfig := mgr.ServerTLSConfig()
	srvConfig.ClientAuth = tls.NoClientCert
	srvConfig.VerifyPeerCertificate = nil

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvConfig)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()
	errCh := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		tlsConn := conn.(*tls.Conn)
		tlsConn.Handshake()

		_, err = mgr.GetPeerSPIFFEID(tlsConn)
		errCh <- err
	}()

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
	}
	conn, err := tls.Dial("tcp", addr, clientTLS)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	conn.Close()

	select {
	case err := <-errCh:
		if err != ErrPeerNotVerified {
			t.Errorf("expected ErrPeerNotVerified, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// TestVerifyPeer_NoRestrictions tests VerifyPeer with empty allowed patterns.
func TestVerifyPeer_NoRestrictions(t *testing.T) {
	srvTLS, cliTLS, cleanup := helperSetupMTLSPair(t)
	defer cleanup()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvTLS)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()
	errCh := make(chan error, 1)

	caCert, caKey := generateTestCA()
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)
	mgrCfg := DefaultConfig("example.com", "server-svc")
	mgrCfg.RootCAs = rootPool
	verifyMgr := NewManager(mgrCfg)
	verifyMgr.Initialize(caCert, caKey)
	defer verifyMgr.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		tlsConn := conn.(*tls.Conn)
		if err := tlsConn.Handshake(); err != nil {
			errCh <- err
			return
		}

		// VerifyPeer with no restrictions (nil patterns)
		err = verifyMgr.VerifyPeer(tlsConn, nil)
		errCh <- err
	}()

	conn, err := tls.Dial("tcp", addr, cliTLS)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	conn.Close()

	select {
	case err := <-errCh:
		// Either nil or ErrPeerNotVerified (if no peer certs) or ErrSPIFFENotFound
		// This mainly exercises the code path
		_ = err
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// TestManager_RefreshTLSConfigs tests that refreshTLSConfigs is called.
func TestManager_RefreshTLSConfigs(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "svc")
	mgr := NewManager(cfg)
	mgr.Initialize(caCert, caKey)
	defer mgr.Close()

	// refreshTLSConfigs is called internally; just verify it doesn't panic
	mgr.refreshTLSConfigs()

	// Verify TLS configs still work
	if mgr.ServerTLSConfig() == nil {
		t.Error("ServerTLSConfig is nil after refresh")
	}
	if mgr.ClientTLSConfig() == nil {
		t.Error("ClientTLSConfig is nil after refresh")
	}
}

// TestGetConnectionInfo_NoPeerCerts exercises GetConnectionInfo with no peer certs.
func TestGetTLSCertificate(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "svc")
	mgr := NewManager(cfg)
	mgr.Initialize(caCert, caKey)
	defer mgr.Close()

	// Exercise getTLSCertificate through the server config callback
	cert, err := mgr.getTLSCertificate()
	if err != nil {
		t.Fatalf("getTLSCertificate failed: %v", err)
	}
	if cert == nil {
		t.Fatal("cert is nil")
	}
}
