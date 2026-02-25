package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helper utilities
// ---------------------------------------------------------------------------

// newTestAuthority creates a fresh Authority with default config for testing.
func newTestAuthority(t *testing.T) *Authority {
	t.Helper()
	a, err := New(nil) // nil -> DefaultConfig
	if err != nil {
		t.Fatalf("New(nil) failed: %v", err)
	}
	return a
}

// issueLeafCert creates a leaf certificate signed by the given Authority's
// intermediate CA so that Verify() can be exercised.
func issueLeafCert(t *testing.T, a *Authority) []byte {
	t.Helper()

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "test-leaf.example.com",
		},
		NotBefore:             now,
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{"test-leaf.example.com"},
	}

	signingCert, _ := a.GetCertificate()
	signingKey := a.GetSigningKey()

	certDER, err := x509.CreateCertificate(rand.Reader, template, signingCert, &leafKey.PublicKey, signingKey)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

// selfSignedCert creates a self-signed certificate that is NOT part of the
// Authority's chain (for negative verification tests).
func selfSignedCert(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "rogue"},
		NotBefore:    now,
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create self-signed cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.CommonName == "" {
		t.Error("DefaultConfig CommonName should not be empty")
	}
	if cfg.Organization == "" {
		t.Error("DefaultConfig Organization should not be empty")
	}
	if cfg.Country == "" {
		t.Error("DefaultConfig Country should not be empty")
	}
	if cfg.Duration <= 0 {
		t.Error("DefaultConfig Duration must be positive")
	}
	if cfg.KeySize <= 0 {
		t.Error("DefaultConfig KeySize must be positive")
	}
}

// ---------------------------------------------------------------------------
// Authority construction (New)
// ---------------------------------------------------------------------------

func TestNew_NilConfig(t *testing.T) {
	a, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) should succeed: %v", err)
	}
	if a == nil {
		t.Fatal("New(nil) returned nil authority")
	}
}

func TestNew_DefaultConfig(t *testing.T) {
	a, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New(DefaultConfig()) failed: %v", err)
	}
	if a == nil {
		t.Fatal("authority is nil")
	}
}

func TestNew_CustomConfig(t *testing.T) {
	cfg := &Config{
		CommonName:   "Custom Root CA",
		Organization: "Custom Org",
		Country:      "US",
		Duration:     365 * 24 * time.Hour,
		KeySize:      256,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New(custom) failed: %v", err)
	}
	rootCert, _ := a.GetRootCertificate()
	if rootCert.Subject.CommonName != "Custom Root CA" {
		t.Errorf("root CN = %q, want %q", rootCert.Subject.CommonName, "Custom Root CA")
	}
}

func TestNew_WithExistingRoot(t *testing.T) {
	// Generate root PEM material first.
	rootCertPEM, rootKeyPEM, err := GenerateRootCA(nil)
	if err != nil {
		t.Fatalf("GenerateRootCA: %v", err)
	}

	cfg := &Config{
		CommonName:   "Loaded Root",
		Organization: "Test",
		Country:      "XX",
		Duration:     365 * 24 * time.Hour,
		RootCertPEM:  rootCertPEM,
		RootKeyPEM:   rootKeyPEM,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New with existing root: %v", err)
	}
	if a == nil {
		t.Fatal("authority is nil")
	}
}

func TestNew_WithExistingRootAndIntermediate(t *testing.T) {
	// Step 1 - generate root.
	rootCertPEM, rootKeyPEM, err := GenerateRootCA(nil)
	if err != nil {
		t.Fatalf("GenerateRootCA: %v", err)
	}
	rootCert, rootKey, err := LoadRootCA(rootCertPEM, rootKeyPEM)
	if err != nil {
		t.Fatalf("LoadRootCA: %v", err)
	}

	// Step 2 - generate intermediate signed by root.
	intermCertPEM, intermKeyPEM, err := GenerateIntermediateCA(nil, rootCert, rootKey)
	if err != nil {
		t.Fatalf("GenerateIntermediateCA: %v", err)
	}

	cfg := &Config{
		CommonName:          "Full",
		Organization:        "Test",
		Country:             "XX",
		Duration:            365 * 24 * time.Hour,
		RootCertPEM:         rootCertPEM,
		RootKeyPEM:          rootKeyPEM,
		IntermediateCertPEM: intermCertPEM,
		IntermediateKeyPEM:  intermKeyPEM,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New with full load: %v", err)
	}
	if a == nil {
		t.Fatal("authority nil")
	}
}

func TestNew_InvalidRootCertPEM(t *testing.T) {
	cfg := &Config{
		CommonName:   "bad",
		Organization: "bad",
		Country:      "XX",
		Duration:     time.Hour,
		RootCertPEM:  []byte("not a certificate"),
		RootKeyPEM:   []byte("not a key"),
	}
	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for invalid root PEM")
	}
}

func TestNew_InvalidIntermediateCertPEM(t *testing.T) {
	rootCertPEM, rootKeyPEM, _ := GenerateRootCA(nil)

	cfg := &Config{
		CommonName:          "bad-inter",
		Organization:        "bad",
		Country:             "XX",
		Duration:            time.Hour,
		RootCertPEM:         rootCertPEM,
		RootKeyPEM:          rootKeyPEM,
		IntermediateCertPEM: []byte("garbage"),
		IntermediateKeyPEM:  []byte("garbage"),
	}
	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for invalid intermediate PEM")
	}
}

// ---------------------------------------------------------------------------
// Getters
// ---------------------------------------------------------------------------

func TestGetCertificate(t *testing.T) {
	a := newTestAuthority(t)
	cert, pemBytes := a.GetCertificate()
	if cert == nil {
		t.Fatal("intermediate certificate is nil")
	}
	if len(pemBytes) == 0 {
		t.Fatal("intermediate PEM is empty")
	}
	if !cert.IsCA {
		t.Error("intermediate certificate should be a CA")
	}
}

func TestGetRootCertificate(t *testing.T) {
	a := newTestAuthority(t)
	cert, pemBytes := a.GetRootCertificate()
	if cert == nil {
		t.Fatal("root certificate is nil")
	}
	if len(pemBytes) == 0 {
		t.Fatal("root PEM is empty")
	}
	if !cert.IsCA {
		t.Error("root certificate should be a CA")
	}
}

func TestGetChain(t *testing.T) {
	a := newTestAuthority(t)
	chain := a.GetChain()
	if len(chain) == 0 {
		t.Fatal("chain PEM is empty")
	}

	// Should contain at least two PEM blocks (intermediate + root).
	block, rest := pem.Decode(chain)
	if block == nil {
		t.Fatal("first PEM block is nil")
	}
	block, _ = pem.Decode(rest)
	if block == nil {
		t.Fatal("second PEM block is nil (chain incomplete)")
	}
}

func TestGetSigningKey(t *testing.T) {
	a := newTestAuthority(t)
	key := a.GetSigningKey()
	if key == nil {
		t.Fatal("signing key is nil")
	}
	if _, ok := key.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("signing key is %T, want *ecdsa.PrivateKey", key)
	}
}

// ---------------------------------------------------------------------------
// Verify
// ---------------------------------------------------------------------------

func TestVerify_ValidLeaf(t *testing.T) {
	a := newTestAuthority(t)
	leafPEM := issueLeafCert(t, a)

	if err := a.Verify(leafPEM); err != nil {
		t.Fatalf("Verify valid leaf: %v", err)
	}
}

func TestVerify_InvalidPEM(t *testing.T) {
	a := newTestAuthority(t)
	if err := a.Verify([]byte("not PEM")); err != ErrInvalidCertificate {
		t.Fatalf("Verify bad PEM: got %v, want %v", err, ErrInvalidCertificate)
	}
}

func TestVerify_UntrustedCert(t *testing.T) {
	a := newTestAuthority(t)
	roguePEM := selfSignedCert(t)

	if err := a.Verify(roguePEM); err == nil {
		t.Fatal("Verify rogue cert should fail")
	}
}

func TestVerify_RevokedCert(t *testing.T) {
	a := newTestAuthority(t)
	leafPEM := issueLeafCert(t, a)

	// Decode to get serial.
	block, _ := pem.Decode(leafPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	serial := cert.SerialNumber.String()
	if err := a.Revoke(serial); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if err := a.Verify(leafPEM); err == nil {
		t.Fatal("Verify revoked cert should fail")
	}
}

// ---------------------------------------------------------------------------
// Revoke / IsRevoked
// ---------------------------------------------------------------------------

func TestRevoke_And_IsRevoked(t *testing.T) {
	a := newTestAuthority(t)

	serial := "12345"
	if a.IsRevoked(serial) {
		t.Fatal("serial should not be revoked before Revoke()")
	}

	if err := a.Revoke(serial); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if !a.IsRevoked(serial) {
		t.Fatal("serial should be revoked after Revoke()")
	}
}

func TestIsRevoked_NotFound(t *testing.T) {
	a := newTestAuthority(t)
	if a.IsRevoked("nonexistent-serial") {
		t.Fatal("nonexistent serial should not be revoked")
	}
}

func TestRevoke_Multiple(t *testing.T) {
	a := newTestAuthority(t)
	serials := []string{"aaa", "bbb", "ccc"}
	for _, s := range serials {
		if err := a.Revoke(s); err != nil {
			t.Fatalf("Revoke(%q): %v", s, err)
		}
	}
	for _, s := range serials {
		if !a.IsRevoked(s) {
			t.Errorf("serial %q should be revoked", s)
		}
	}
	if a.IsRevoked("ddd") {
		t.Error("serial ddd should NOT be revoked")
	}
}

// ---------------------------------------------------------------------------
// Root CA standalone helpers (root.go)
// ---------------------------------------------------------------------------

func TestGenerateRootCA_NilConfig(t *testing.T) {
	certPEM, keyPEM, err := GenerateRootCA(nil)
	if err != nil {
		t.Fatalf("GenerateRootCA(nil): %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("PEM output must not be empty")
	}
}

func TestGenerateRootCA_CustomConfig(t *testing.T) {
	cfg := &RootCAConfig{
		CommonName:   "My Root",
		Organization: "My Org",
		Country:      "US",
		Province:     "CA",
		Locality:     "SF",
		Duration:     365 * 24 * time.Hour,
		KeyCurve:     elliptic.P256(),
	}
	certPEM, keyPEM, err := GenerateRootCA(cfg)
	if err != nil {
		t.Fatalf("GenerateRootCA(custom): %v", err)
	}

	cert, _, err := LoadRootCA(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadRootCA: %v", err)
	}
	if cert.Subject.CommonName != "My Root" {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, "My Root")
	}
	if !cert.IsCA {
		t.Error("root cert should be CA")
	}
}

func TestDefaultRootCAConfig(t *testing.T) {
	cfg := DefaultRootCAConfig()
	if cfg.CommonName == "" {
		t.Error("CommonName empty")
	}
	if cfg.KeyCurve == nil {
		t.Error("KeyCurve nil")
	}
	if cfg.Duration <= 0 {
		t.Error("Duration must be positive")
	}
}

func TestLoadRootCA_Roundtrip(t *testing.T) {
	certPEM, keyPEM, err := GenerateRootCA(nil)
	if err != nil {
		t.Fatalf("GenerateRootCA: %v", err)
	}

	cert, key, err := LoadRootCA(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadRootCA: %v", err)
	}
	if cert == nil {
		t.Fatal("certificate nil")
	}
	if key == nil {
		t.Fatal("key nil")
	}
}

func TestLoadRootCA_InvalidCertPEM(t *testing.T) {
	_, _, err := LoadRootCA([]byte("bad"), []byte("bad"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRootCA_InvalidKeyPEM(t *testing.T) {
	certPEM, _, err := GenerateRootCA(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = LoadRootCA(certPEM, []byte("bad-key"))
	if err == nil {
		t.Fatal("expected error for bad key PEM")
	}
}

func TestVerifyRootCA_Valid(t *testing.T) {
	certPEM, keyPEM, _ := GenerateRootCA(nil)
	cert, _, _ := LoadRootCA(certPEM, keyPEM)

	if err := VerifyRootCA(cert); err != nil {
		t.Fatalf("VerifyRootCA: %v", err)
	}
}

func TestVerifyRootCA_NotCA(t *testing.T) {
	// Craft a certificate that is not a CA.
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "not-a-ca"},
		NotBefore:             now,
		NotAfter:              now.Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	if err := VerifyRootCA(cert); err != ErrInvalidCertificate {
		t.Fatalf("VerifyRootCA(notCA): got %v, want %v", err, ErrInvalidCertificate)
	}
}

func TestVerifyRootCA_NoCertSign(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "no-certsign"},
		NotBefore:             now,
		NotAfter:              now.Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature, // no CertSign
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	if err := VerifyRootCA(cert); err != ErrInvalidCertificate {
		t.Fatalf("VerifyRootCA(noCertSign): got %v, want %v", err, ErrInvalidCertificate)
	}
}

// ---------------------------------------------------------------------------
// Intermediate CA standalone helpers (intermediate.go)
// ---------------------------------------------------------------------------

func TestDefaultIntermediateCAConfig(t *testing.T) {
	cfg := DefaultIntermediateCAConfig()
	if cfg.CommonName == "" {
		t.Error("CommonName empty")
	}
	if cfg.KeyCurve == nil {
		t.Error("KeyCurve nil")
	}
	if cfg.Duration <= 0 {
		t.Error("Duration must be positive")
	}
}

func TestGenerateIntermediateCA_NilConfig(t *testing.T) {
	rootCertPEM, rootKeyPEM, _ := GenerateRootCA(nil)
	rootCert, rootKey, _ := LoadRootCA(rootCertPEM, rootKeyPEM)

	certPEM, keyPEM, err := GenerateIntermediateCA(nil, rootCert, rootKey)
	if err != nil {
		t.Fatalf("GenerateIntermediateCA(nil): %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("PEM output must not be empty")
	}
}

func TestGenerateIntermediateCA_CustomConfig(t *testing.T) {
	rootCertPEM, rootKeyPEM, _ := GenerateRootCA(nil)
	rootCert, rootKey, _ := LoadRootCA(rootCertPEM, rootKeyPEM)

	cfg := &IntermediateCAConfig{
		CommonName:   "Custom Intermediate",
		Organization: "Custom Org",
		Country:      "US",
		Duration:     365 * 24 * time.Hour,
		KeyCurve:     elliptic.P384(),
	}
	certPEM, keyPEM, err := GenerateIntermediateCA(cfg, rootCert, rootKey)
	if err != nil {
		t.Fatalf("GenerateIntermediateCA(custom): %v", err)
	}

	cert, _, err := LoadIntermediateCA(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIntermediateCA: %v", err)
	}
	if cert.Subject.CommonName != "Custom Intermediate" {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, "Custom Intermediate")
	}
}

func TestLoadIntermediateCA_Roundtrip(t *testing.T) {
	rootCertPEM, rootKeyPEM, _ := GenerateRootCA(nil)
	rootCert, rootKey, _ := LoadRootCA(rootCertPEM, rootKeyPEM)

	certPEM, keyPEM, _ := GenerateIntermediateCA(nil, rootCert, rootKey)
	cert, key, err := LoadIntermediateCA(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIntermediateCA: %v", err)
	}
	if cert == nil || key == nil {
		t.Fatal("cert or key nil")
	}
}

func TestVerifyIntermediateCA_Valid(t *testing.T) {
	rootCertPEM, rootKeyPEM, _ := GenerateRootCA(nil)
	rootCert, rootKey, _ := LoadRootCA(rootCertPEM, rootKeyPEM)

	intermCertPEM, intermKeyPEM, _ := GenerateIntermediateCA(nil, rootCert, rootKey)
	intermCert, _, _ := LoadIntermediateCA(intermCertPEM, intermKeyPEM)

	if err := VerifyIntermediateCA(intermCert, rootCert); err != nil {
		t.Fatalf("VerifyIntermediateCA: %v", err)
	}
}

func TestVerifyIntermediateCA_WrongRoot(t *testing.T) {
	// Generate two independent root CAs.
	rootCertPEM1, rootKeyPEM1, _ := GenerateRootCA(nil)
	rootCert1, rootKey1, _ := LoadRootCA(rootCertPEM1, rootKeyPEM1)

	rootCertPEM2, rootKeyPEM2, _ := GenerateRootCA(&RootCAConfig{
		CommonName:   "Other Root",
		Organization: "Other",
		Country:      "XX",
		Duration:     10 * 365 * 24 * time.Hour,
		KeyCurve:     elliptic.P384(),
	})
	rootCert2, _, _ := LoadRootCA(rootCertPEM2, rootKeyPEM2)

	// Intermediate signed by root 1.
	intermCertPEM, intermKeyPEM, _ := GenerateIntermediateCA(nil, rootCert1, rootKey1)
	intermCert, _, _ := LoadIntermediateCA(intermCertPEM, intermKeyPEM)

	// Verify against root 2 should fail.
	if err := VerifyIntermediateCA(intermCert, rootCert2); err == nil {
		t.Fatal("expected verification failure with wrong root")
	}
}

func TestVerifyIntermediateCA_NotCA(t *testing.T) {
	// Build a non-CA certificate.
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "leaf"},
		NotBefore:             now,
		NotAfter:              now.Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	rootCertPEM, rootKeyPEM, _ := GenerateRootCA(nil)
	rootCert, _, _ := LoadRootCA(rootCertPEM, rootKeyPEM)

	if err := VerifyIntermediateCA(cert, rootCert); err != ErrInvalidCertificate {
		t.Fatalf("got %v, want %v", err, ErrInvalidCertificate)
	}
}

// ---------------------------------------------------------------------------
// Edge / integration-level tests
// ---------------------------------------------------------------------------

func TestAuthority_IntermediateSignedByRoot(t *testing.T) {
	a := newTestAuthority(t)

	rootCert, _ := a.GetRootCertificate()
	intermCert, _ := a.GetCertificate()

	// The intermediate issuer should match the root's subject.
	if intermCert.Issuer.CommonName != rootCert.Subject.CommonName {
		t.Errorf("intermediate issuer CN = %q, root subject CN = %q",
			intermCert.Issuer.CommonName, rootCert.Subject.CommonName)
	}
}

func TestAuthority_RootIsSelfSigned(t *testing.T) {
	a := newTestAuthority(t)
	rootCert, _ := a.GetRootCertificate()

	if rootCert.Issuer.CommonName != rootCert.Subject.CommonName {
		t.Errorf("root should be self-signed: issuer=%q subject=%q",
			rootCert.Issuer.CommonName, rootCert.Subject.CommonName)
	}
}

func TestAuthority_ChainContainsBothCerts(t *testing.T) {
	a := newTestAuthority(t)

	chain := a.GetChain()
	_, intermPEM := a.GetCertificate()
	_, rootPEM := a.GetRootCertificate()

	if len(chain) != len(intermPEM)+len(rootPEM) {
		t.Errorf("chain length %d, expected %d (intermediate %d + root %d)",
			len(chain), len(intermPEM)+len(rootPEM), len(intermPEM), len(rootPEM))
	}
}

func TestAuthority_ConcurrentAccess(t *testing.T) {
	a := newTestAuthority(t)
	done := make(chan struct{})

	// Concurrent reads and writes.
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			a.GetCertificate()
			a.GetRootCertificate()
			a.GetChain()
			a.GetSigningKey()
		}
	}()

	for i := 0; i < 100; i++ {
		serial := big.NewInt(int64(i)).String()
		_ = a.Revoke(serial)
		_ = a.IsRevoked(serial)
	}

	<-done
}
