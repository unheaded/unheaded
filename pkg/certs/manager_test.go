package certs

import (
	"context"
	"testing"
	"time"

	"github.com/unheaded/unheaded/pkg/certs/ca"
	"github.com/unheaded/unheaded/pkg/certs/rotation"
	"github.com/unheaded/unheaded/pkg/certs/store"
)

func TestNewManager(t *testing.T) {
	config := &ManagerConfig{
		CAConfig:        ca.DefaultConfig(),
		StoreConfig:     store.DefaultConfig(),
		RotationConfig:  rotation.DefaultConfig(),
		DefaultDuration: 1 * time.Hour,
		MaxDuration:     24 * time.Hour,
		EnableRotation:  false,
	}

	m, err := NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer m.Close()

	if m.ca == nil {
		t.Error("CA not initialized")
	}
	if m.issuer == nil {
		t.Error("issuer not initialized")
	}
	if m.store == nil {
		t.Error("store not initialized")
	}
}

func TestIssueCertificate(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	tests := []struct {
		name    string
		req     *CertRequest
		wantErr bool
	}{
		{
			name: "server certificate",
			req: &CertRequest{
				CommonName:   "test-server.unheaded.local",
				DNSNames:     []string{"test-server.unheaded.local", "localhost"},
				IPAddresses:  []string{"127.0.0.1"},
				Type:         CertTypeServer,
				Organization: "Unheaded Kingdom",
				Duration:     1 * time.Hour,
			},
			wantErr: false,
		},
		{
			name: "client certificate",
			req: &CertRequest{
				CommonName:   "test-client",
				Type:         CertTypeClient,
				Organization: "Unheaded Kingdom",
				Duration:     30 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "mutual TLS certificate",
			req: &CertRequest{
				CommonName:   "test-service",
				DNSNames:     []string{"test-service.unheaded.local"},
				Type:         CertTypeMutual,
				Organization: "Unheaded Kingdom",
				Duration:     2 * time.Hour,
			},
			wantErr: false,
		},
		{
			name: "with SPIFFE ID",
			req: &CertRequest{
				CommonName:   "workload",
				Type:         CertTypeMutual,
				SPIFFEID:     "spiffe://unheaded.local/ns/default/sa/workload",
				Organization: "Unheaded Kingdom",
				Duration:     1 * time.Hour,
			},
			wantErr: false,
		},
		{
			name: "empty request",
			req: &CertRequest{
				Duration: 1 * time.Hour,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := m.IssueCertificate(ctx, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cert.Serial == "" {
				t.Error("certificate has empty serial")
			}
			if cert.Certificate == nil {
				t.Error("certificate is nil")
			}
			if len(cert.CertificatePEM) == 0 {
				t.Error("certificate PEM is empty")
			}
			if len(cert.PrivateKeyPEM) == 0 {
				t.Error("private key PEM is empty")
			}
			if cert.IssuedAt.IsZero() {
				t.Error("issued at is zero")
			}
			if cert.ExpiresAt.IsZero() {
				t.Error("expires at is zero")
			}
			if !cert.IsValid() {
				t.Error("certificate should be valid")
			}
		})
	}
}

func TestRevokeCertificate(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	// Issue a certificate
	cert, err := m.IssueCertificate(ctx, &CertRequest{
		CommonName:   "revoke-test",
		Type:         CertTypeServer,
		Organization: "Unheaded Kingdom",
		Duration:     1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to issue certificate: %v", err)
	}

	// Revoke it
	err = m.RevokeCertificate(ctx, cert.Serial)
	if err != nil {
		t.Fatalf("failed to revoke certificate: %v", err)
	}

	// Verify it's revoked
	revoked, err := m.GetCertificate(ctx, cert.Serial)
	if err != nil {
		t.Fatalf("failed to get revoked certificate: %v", err)
	}
	if !revoked.IsRevoked() {
		t.Error("certificate should be revoked")
	}
	if revoked.IsValid() {
		t.Error("revoked certificate should not be valid")
	}
}

func TestGetCertificate(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	// Issue a certificate
	cert, err := m.IssueCertificate(ctx, &CertRequest{
		CommonName:   "get-test",
		Type:         CertTypeServer,
		Organization: "Unheaded Kingdom",
		Duration:     1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to issue certificate: %v", err)
	}

	// Get it
	retrieved, err := m.GetCertificate(ctx, cert.Serial)
	if err != nil {
		t.Fatalf("failed to get certificate: %v", err)
	}

	if retrieved.Serial != cert.Serial {
		t.Errorf("serial mismatch: got %s, want %s", retrieved.Serial, cert.Serial)
	}

	// Try to get non-existent
	_, err = m.GetCertificate(ctx, "nonexistent")
	if err != ErrCertificateNotFound {
		t.Errorf("expected ErrCertificateNotFound, got %v", err)
	}
}

func TestListCertificates(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	// Issue multiple certificates
	for i := 0; i < 3; i++ {
		_, err := m.IssueCertificate(ctx, &CertRequest{
			CommonName:   "list-test",
			Type:         CertTypeServer,
			Organization: "Unheaded Kingdom",
			Duration:     1 * time.Hour,
		})
		if err != nil {
			t.Fatalf("failed to issue certificate: %v", err)
		}
	}

	// List all
	certs, err := m.ListCertificates(ctx)
	if err != nil {
		t.Fatalf("failed to list certificates: %v", err)
	}

	if len(certs) != 3 {
		t.Errorf("expected 3 certificates, got %d", len(certs))
	}
}

func TestRenewCertificate(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	// Issue a certificate
	cert, err := m.IssueCertificate(ctx, &CertRequest{
		CommonName:         "renew-test",
		DNSNames:           []string{"renew.unheaded.local"},
		Type:               CertTypeServer,
		Organization:       "Unheaded Kingdom",
		OrganizationalUnit: "Test",
		Duration:           1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to issue certificate: %v", err)
	}

	// Renew it
	renewed, err := m.RenewCertificate(ctx, cert.Serial)
	if err != nil {
		t.Fatalf("failed to renew certificate: %v", err)
	}

	// Verify new certificate
	if renewed.Serial == cert.Serial {
		t.Error("renewed certificate should have different serial")
	}
	if renewed.Certificate.Subject.CommonName != cert.Certificate.Subject.CommonName {
		t.Error("renewed certificate should have same common name")
	}
	if !renewed.IsValid() {
		t.Error("renewed certificate should be valid")
	}

	// Old certificate should be revoked
	old, err := m.GetCertificate(ctx, cert.Serial)
	if err != nil {
		t.Fatalf("failed to get old certificate: %v", err)
	}
	if !old.IsRevoked() {
		t.Error("old certificate should be revoked")
	}
}

func TestGetCACertificate(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	caCert, err := m.GetCACertificate(ctx)
	if err != nil {
		t.Fatalf("failed to get CA certificate: %v", err)
	}

	if caCert.Certificate == nil {
		t.Error("CA certificate is nil")
	}
	if !caCert.Certificate.IsCA {
		t.Error("CA certificate should have IsCA=true")
	}
	if caCert.Type != CertTypeCA {
		t.Error("CA certificate should have CertTypeCA type")
	}
}

func TestVerifyCertificate(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	// Issue a certificate
	cert, err := m.IssueCertificate(ctx, &CertRequest{
		CommonName:   "verify-test",
		Type:         CertTypeServer,
		Organization: "Unheaded Kingdom",
		Duration:     1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to issue certificate: %v", err)
	}

	// Verify it
	err = m.VerifyCertificate(ctx, cert.CertificatePEM)
	if err != nil {
		t.Errorf("certificate verification failed: %v", err)
	}

	// Verify with invalid PEM
	err = m.VerifyCertificate(ctx, []byte("invalid"))
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestCertificateValidity(t *testing.T) {
	cert := &Certificate{
		IssuedAt:  time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if cert.IsExpired() {
		t.Error("certificate should not be expired")
	}
	if cert.IsRevoked() {
		t.Error("certificate should not be revoked")
	}
	if !cert.IsValid() {
		t.Error("certificate should be valid")
	}

	ttl := cert.TimeToExpiry()
	if ttl <= 0 {
		t.Error("time to expiry should be positive")
	}

	// Test expired
	expiredCert := &Certificate{
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if !expiredCert.IsExpired() {
		t.Error("certificate should be expired")
	}
	if expiredCert.IsValid() {
		t.Error("expired certificate should not be valid")
	}

	// Test revoked
	now := time.Now()
	revokedCert := &Certificate{
		IssuedAt:  time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		RevokedAt: &now,
	}
	if !revokedCert.IsRevoked() {
		t.Error("certificate should be revoked")
	}
	if revokedCert.IsValid() {
		t.Error("revoked certificate should not be valid")
	}
}

func TestManagerClose(t *testing.T) {
	m := createTestManager(t)

	err := m.Close()
	if err != nil {
		t.Fatalf("failed to close manager: %v", err)
	}

	// Operations should fail after close
	ctx := context.Background()
	_, err = m.IssueCertificate(ctx, &CertRequest{
		CommonName: "test",
		Type:       CertTypeServer,
	})
	if err == nil {
		t.Error("expected error after close")
	}

	// Double close should be safe
	err = m.Close()
	if err != nil {
		t.Errorf("double close should not error: %v", err)
	}
}

func TestDurationClamping(t *testing.T) {
	m := createTestManager(t)
	defer m.Close()

	ctx := context.Background()

	// Request duration longer than max
	cert, err := m.IssueCertificate(ctx, &CertRequest{
		CommonName:   "duration-test",
		Type:         CertTypeServer,
		Organization: "Unheaded Kingdom",
		Duration:     1000 * time.Hour, // Way over max
	})
	if err != nil {
		t.Fatalf("failed to issue certificate: %v", err)
	}

	// Should be clamped to max
	duration := cert.ExpiresAt.Sub(cert.IssuedAt)
	if duration > m.config.MaxDuration {
		t.Errorf("duration %v exceeds max %v", duration, m.config.MaxDuration)
	}
}

// createTestManager creates a manager for testing.
func createTestManager(t *testing.T) *Manager {
	t.Helper()

	config := &ManagerConfig{
		CAConfig:        ca.DefaultConfig(),
		StoreConfig:     store.DefaultConfig(),
		RotationConfig:  rotation.DefaultConfig(),
		DefaultDuration: 1 * time.Hour,
		MaxDuration:     24 * time.Hour,
		EnableRotation:  false,
	}

	m, err := NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	return m
}
