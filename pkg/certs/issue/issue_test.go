// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package issue

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"unheaded/pkg/certs/ca"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestAuthority creates a fresh CA Authority for test use.
func newTestAuthority(t *testing.T) *ca.Authority {
	t.Helper()
	a, err := ca.New(nil)
	if err != nil {
		t.Fatalf("ca.New: %v", err)
	}
	return a
}

// newTestIssuer creates an Issuer backed by a fresh Authority.
func newTestIssuer(t *testing.T) *Issuer {
	t.Helper()
	return New(newTestAuthority(t), 24*time.Hour, 365*24*time.Hour)
}

// ---------------------------------------------------------------------------
// Issuer construction
// ---------------------------------------------------------------------------

func TestNew_Issuer(t *testing.T) {
	iss := newTestIssuer(t)
	if iss == nil {
		t.Fatal("issuer is nil")
	}
}

// ---------------------------------------------------------------------------
// Issue - basic
// ---------------------------------------------------------------------------

func TestIssue_MinimalRequest(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "test.example.com",
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if res.Serial == "" {
		t.Error("serial is empty")
	}
	if res.Certificate == nil {
		t.Error("certificate is nil")
	}
	if len(res.CertificatePEM) == 0 {
		t.Error("CertificatePEM is empty")
	}
	if len(res.PrivateKeyPEM) == 0 {
		t.Error("PrivateKeyPEM should be set when no CSR provided")
	}
	if len(res.ChainPEM) == 0 {
		t.Error("ChainPEM is empty")
	}
}

func TestIssue_WithDNSNames(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "web.example.com",
		DNSNames:   []string{"web.example.com", "www.example.com"},
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(res.Certificate.DNSNames) != 2 {
		t.Errorf("DNSNames count = %d, want 2", len(res.Certificate.DNSNames))
	}
}

func TestIssue_WithIPAddresses(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName:  "ip-service",
		IPAddresses: []string{"10.10.10.20", "127.0.0.1"},
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(res.Certificate.IPAddresses) != 2 {
		t.Errorf("IPAddresses count = %d, want 2", len(res.Certificate.IPAddresses))
	}
}

func TestIssue_WithIPv6(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName:  "ipv6-service",
		IPAddresses: []string{"::1", "fe80::1"},
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(res.Certificate.IPAddresses) != 2 {
		t.Errorf("IPAddresses count = %d, want 2", len(res.Certificate.IPAddresses))
	}
}

func TestIssue_WithSPIFFEID(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "workload",
		SPIFFEID:   "spiffe://unheaded.kingdom/workload/timeguru",
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(res.Certificate.URIs) == 0 {
		t.Fatal("URIs should contain SPIFFE ID")
	}
	if res.Certificate.URIs[0].String() != "spiffe://unheaded.kingdom/workload/timeguru" {
		t.Errorf("URI = %q, want SPIFFE URI", res.Certificate.URIs[0].String())
	}
}

func TestIssue_WithURIs(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "uri-svc",
		URIs:       []string{"https://example.com/id/123"},
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(res.Certificate.URIs) != 1 {
		t.Errorf("URIs count = %d, want 1", len(res.Certificate.URIs))
	}
}

func TestIssue_WithSPIFFEAndURIs(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "combined",
		SPIFFEID:   "spiffe://example.com/svc",
		URIs:       []string{"https://other.example.com/id/1"},
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(res.Certificate.URIs) != 2 {
		t.Errorf("URIs count = %d, want 2", len(res.Certificate.URIs))
	}
}

func TestIssue_WithKeyUsage(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName:  "server",
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if res.Certificate.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("missing DigitalSignature key usage")
	}
}

func TestIssue_WithOrganization(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName:         "org-test",
		Organization:       "MyOrg",
		OrganizationalUnit: "MyUnit",
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(res.Certificate.Subject.Organization) == 0 || res.Certificate.Subject.Organization[0] != "MyOrg" {
		t.Errorf("Organization = %v, want [MyOrg]", res.Certificate.Subject.Organization)
	}
}

// ---------------------------------------------------------------------------
// Issue - duration clamping
// ---------------------------------------------------------------------------

func TestIssue_DefaultDuration(t *testing.T) {
	iss := newTestIssuer(t) // default = 24h
	req := &Request{CommonName: "default-dur"}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	duration := res.Certificate.NotAfter.Sub(res.Certificate.NotBefore)
	// Allow small clock skew.
	if duration < 23*time.Hour || duration > 25*time.Hour {
		t.Errorf("duration = %v, expected ~24h", duration)
	}
}

func TestIssue_ExplicitDuration(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "custom-dur",
		Duration:   48 * time.Hour,
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	duration := res.Certificate.NotAfter.Sub(res.Certificate.NotBefore)
	if duration < 47*time.Hour || duration > 49*time.Hour {
		t.Errorf("duration = %v, expected ~48h", duration)
	}
}

func TestIssue_DurationClamped(t *testing.T) {
	// maxDuration is 365 days.
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "long-dur",
		Duration:   10 * 365 * 24 * time.Hour, // 10 years, way over max
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	duration := res.Certificate.NotAfter.Sub(res.Certificate.NotBefore)
	maxExpected := 366 * 24 * time.Hour // 365 + small leeway
	if duration > maxExpected {
		t.Errorf("duration = %v, should be clamped to ~365 days", duration)
	}
}

// ---------------------------------------------------------------------------
// Issue - CSR path
// ---------------------------------------------------------------------------

func TestIssue_WithCSR(t *testing.T) {
	csrResult, err := GenerateCSR(&CSRRequest{
		CommonName:   "csr-test.example.com",
		Organization: "CSROrg",
		DNSNames:     []string{"csr-test.example.com"},
	})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}

	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "csr-test.example.com",
		DNSNames:   []string{"csr-test.example.com"},
		CSR:        csrResult.CSRPEM,
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue with CSR: %v", err)
	}
	if res.Certificate == nil {
		t.Fatal("certificate nil")
	}
	// When CSR is provided, the issuer should NOT return a private key.
	if len(res.PrivateKeyPEM) != 0 {
		t.Error("PrivateKeyPEM should be empty when CSR is provided")
	}
}

func TestIssue_WithInvalidCSR(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		CommonName: "bad-csr",
		CSR:        []byte("not a csr"),
	}
	_, err := iss.Issue(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid CSR")
	}
}

// ---------------------------------------------------------------------------
// Issue - validation
// ---------------------------------------------------------------------------

func TestIssue_NilRequest(t *testing.T) {
	iss := newTestIssuer(t)
	_, err := iss.Issue(context.Background(), nil)
	if err != ErrInvalidRequest {
		t.Fatalf("got %v, want %v", err, ErrInvalidRequest)
	}
}

func TestIssue_EmptyRequest(t *testing.T) {
	iss := newTestIssuer(t)
	_, err := iss.Issue(context.Background(), &Request{})
	if err != ErrInvalidRequest {
		t.Fatalf("got %v, want %v", err, ErrInvalidRequest)
	}
}

func TestIssue_OnlyDNSNames(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		DNSNames: []string{"dns-only.example.com"},
	}
	_, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("should allow request with only DNSNames: %v", err)
	}
}

func TestIssue_OnlyIPAddresses(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{
		IPAddresses: []string{"10.0.0.1"},
	}
	_, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("should allow request with only IPAddresses: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Issue - chain building
// ---------------------------------------------------------------------------

func TestIssue_ChainContainsLeafAndCA(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{CommonName: "chain-test"}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Decode all PEM blocks in the chain.
	var count int
	rest := res.ChainPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		count++
	}
	// Expect leaf + intermediate + root = 3 certificates.
	if count != 3 {
		t.Errorf("chain contains %d certs, want 3 (leaf + intermediate + root)", count)
	}
}

func TestIssue_CertificateIsNotCA(t *testing.T) {
	iss := newTestIssuer(t)
	req := &Request{CommonName: "leaf-check"}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if res.Certificate.IsCA {
		t.Error("issued certificate should not be a CA")
	}
}

// ---------------------------------------------------------------------------
// Issue - certificate verifiable by authority
// ---------------------------------------------------------------------------

func TestIssue_VerifiableByAuthority(t *testing.T) {
	auth := newTestAuthority(t)
	iss := New(auth, 24*time.Hour, 365*24*time.Hour)

	req := &Request{
		CommonName: "verifiable.example.com",
		DNSNames:   []string{"verifiable.example.com"},
		KeyUsage:   x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	res, err := iss.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := auth.Verify(res.CertificatePEM); err != nil {
		t.Fatalf("Verify issued cert failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// KeyUsageFromType / ExtKeyUsageFromType
// ---------------------------------------------------------------------------

func TestKeyUsageFromType(t *testing.T) {
	tests := []struct {
		certType int
		wantBits x509.KeyUsage
	}{
		{0, x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment}, // Server
		{1, x509.KeyUsageDigitalSignature},                               // Client
		{2, x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment}, // Mutual
		{99, x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment}, // Default
	}
	for _, tt := range tests {
		got := KeyUsageFromType(tt.certType)
		if got != tt.wantBits {
			t.Errorf("KeyUsageFromType(%d) = %v, want %v", tt.certType, got, tt.wantBits)
		}
	}
}

func TestExtKeyUsageFromType(t *testing.T) {
	tests := []struct {
		certType int
		want     []x509.ExtKeyUsage
	}{
		{0, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}},
		{1, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}},
		{2, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}},
		{99, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}},
	}
	for _, tt := range tests {
		got := ExtKeyUsageFromType(tt.certType)
		if len(got) != len(tt.want) {
			t.Errorf("ExtKeyUsageFromType(%d): len = %d, want %d", tt.certType, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ExtKeyUsageFromType(%d)[%d] = %v, want %v", tt.certType, i, got[i], tt.want[i])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// CSR generation and parsing (csr.go)
// ---------------------------------------------------------------------------

func TestGenerateCSR_Minimal(t *testing.T) {
	result, err := GenerateCSR(&CSRRequest{
		CommonName: "test-csr",
	})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}
	if len(result.CSRPEM) == 0 {
		t.Error("CSRPEM is empty")
	}
	if len(result.PrivateKeyPEM) == 0 {
		t.Error("PrivateKeyPEM is empty")
	}
}

func TestGenerateCSR_Full(t *testing.T) {
	result, err := GenerateCSR(&CSRRequest{
		CommonName:         "full-csr.example.com",
		Organization:       "TestOrg",
		OrganizationalUnit: "TestUnit",
		Country:            "US",
		DNSNames:           []string{"full-csr.example.com", "alias.example.com"},
		IPAddresses:        []string{"10.0.0.1", "::1"},
		URIs:               []string{"https://example.com/id/42"},
		SPIFFEID:           "spiffe://example.com/workload/test",
	})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}

	csr, err := ParseCSR(result.CSRPEM)
	if err != nil {
		t.Fatalf("ParseCSR: %v", err)
	}
	if csr.Subject.CommonName != "full-csr.example.com" {
		t.Errorf("CN = %q", csr.Subject.CommonName)
	}
	if len(csr.DNSNames) != 2 {
		t.Errorf("DNSNames = %d, want 2", len(csr.DNSNames))
	}
	if len(csr.IPAddresses) != 2 {
		t.Errorf("IPAddresses = %d, want 2", len(csr.IPAddresses))
	}
	// SPIFFE ID + one explicit URI = 2.
	if len(csr.URIs) != 2 {
		t.Errorf("URIs = %d, want 2", len(csr.URIs))
	}
}

func TestParseCSR_Valid(t *testing.T) {
	result, _ := GenerateCSR(&CSRRequest{CommonName: "parse-test"})
	csr, err := ParseCSR(result.CSRPEM)
	if err != nil {
		t.Fatalf("ParseCSR: %v", err)
	}
	if csr.Subject.CommonName != "parse-test" {
		t.Errorf("CN = %q", csr.Subject.CommonName)
	}
}

func TestParseCSR_InvalidPEM(t *testing.T) {
	_, err := ParseCSR([]byte("not pem data"))
	if err != ErrCSRParseFailed {
		t.Fatalf("got %v, want %v", err, ErrCSRParseFailed)
	}
}

func TestParseCSR_GarbageDER(t *testing.T) {
	garbage := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: []byte("garbage DER"),
	})
	_, err := ParseCSR(garbage)
	if err == nil {
		t.Fatal("expected error for garbage DER inside PEM")
	}
}

func TestValidateCSR_ECDSA(t *testing.T) {
	result, _ := GenerateCSR(&CSRRequest{CommonName: "validate-ecdsa"})
	csr, err := ParseCSR(result.CSRPEM)
	if err != nil {
		t.Fatalf("ParseCSR: %v", err)
	}
	if err := ValidateCSR(csr); err != nil {
		t.Fatalf("ValidateCSR: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Template tests (template.go)
// ---------------------------------------------------------------------------

func baseTemplateOpts() *TemplateOptions {
	return &TemplateOptions{
		Serial:             big.NewInt(1),
		CommonName:         "template-test",
		Organization:       "Org",
		OrganizationalUnit: "Unit",
		Country:            "XX",
		Duration:           24 * time.Hour,
		IPAddresses:        []net.IP{net.ParseIP("10.0.0.1")},
	}
}

func TestNewServerTemplate(t *testing.T) {
	opts := baseTemplateOpts()
	tmpl := NewServerTemplate(opts)

	if tmpl.IsCA {
		t.Error("server template should not be CA")
	}
	if tmpl.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("missing DigitalSignature")
	}
	if tmpl.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Error("missing KeyEncipherment")
	}
	if len(tmpl.ExtKeyUsage) != 1 || tmpl.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Error("ExtKeyUsage should be [ServerAuth]")
	}
	if tmpl.Subject.CommonName != "template-test" {
		t.Errorf("CN = %q", tmpl.Subject.CommonName)
	}
}

func TestNewClientTemplate(t *testing.T) {
	opts := baseTemplateOpts()
	tmpl := NewClientTemplate(opts)

	if tmpl.IsCA {
		t.Error("client template should not be CA")
	}
	if tmpl.KeyUsage != x509.KeyUsageDigitalSignature {
		t.Errorf("KeyUsage = %v, want DigitalSignature only", tmpl.KeyUsage)
	}
	if len(tmpl.ExtKeyUsage) != 1 || tmpl.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Error("ExtKeyUsage should be [ClientAuth]")
	}
}

func TestNewMutualTemplate(t *testing.T) {
	opts := baseTemplateOpts()
	tmpl := NewMutualTemplate(opts)

	if tmpl.IsCA {
		t.Error("mutual template should not be CA")
	}
	if len(tmpl.ExtKeyUsage) != 2 {
		t.Fatalf("ExtKeyUsage len = %d, want 2", len(tmpl.ExtKeyUsage))
	}
	if tmpl.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("ExtKeyUsage[0] = %v, want ServerAuth", tmpl.ExtKeyUsage[0])
	}
	if tmpl.ExtKeyUsage[1] != x509.ExtKeyUsageClientAuth {
		t.Errorf("ExtKeyUsage[1] = %v, want ClientAuth", tmpl.ExtKeyUsage[1])
	}
}

func TestNewWorkloadTemplate(t *testing.T) {
	opts := baseTemplateOpts()
	opts.SPIFFEID = "spiffe://example.com/workload/test"
	tmpl := NewWorkloadTemplate(opts)

	if tmpl.IsCA {
		t.Error("workload template should not be CA")
	}
	if len(tmpl.URIs) == 0 {
		t.Fatal("workload template URIs should contain SPIFFE ID")
	}
	if tmpl.URIs[0].String() != "spiffe://example.com/workload/test" {
		t.Errorf("URIs[0] = %q", tmpl.URIs[0].String())
	}
}

func TestNewWorkloadTemplate_WithExistingURIs(t *testing.T) {
	extraURI, _ := url.Parse("https://extra.example.com")
	opts := baseTemplateOpts()
	opts.SPIFFEID = "spiffe://example.com/wl"
	opts.URIs = []*url.URL{extraURI}

	tmpl := NewWorkloadTemplate(opts)
	// SPIFFE + extra = 2 URIs.
	if len(tmpl.URIs) != 2 {
		t.Errorf("URIs count = %d, want 2", len(tmpl.URIs))
	}
}

func TestNewWorkloadTemplate_NoSPIFFE(t *testing.T) {
	opts := baseTemplateOpts()
	opts.SPIFFEID = ""
	tmpl := NewWorkloadTemplate(opts)
	// No SPIFFE and no explicit URIs -> the opts.URIs (nil) should result in nil/empty.
	if len(tmpl.URIs) != 0 {
		t.Errorf("URIs count = %d, want 0", len(tmpl.URIs))
	}
}

func TestTemplates_Duration(t *testing.T) {
	opts := baseTemplateOpts()
	opts.Duration = 48 * time.Hour

	tmpl := NewServerTemplate(opts)
	dur := tmpl.NotAfter.Sub(tmpl.NotBefore)
	if dur < 47*time.Hour || dur > 49*time.Hour {
		t.Errorf("duration = %v, expected ~48h", dur)
	}
}

func TestTemplates_IPAddresses(t *testing.T) {
	opts := baseTemplateOpts()
	opts.IPAddresses = []net.IP{net.ParseIP("10.10.10.20"), net.ParseIP("::1")}

	tmpl := NewServerTemplate(opts)
	if len(tmpl.IPAddresses) != 2 {
		t.Errorf("IPAddresses = %d, want 2", len(tmpl.IPAddresses))
	}
}

// ---------------------------------------------------------------------------
// ShortLivedTemplate
// ---------------------------------------------------------------------------

func TestShortLivedTemplate_Server(t *testing.T) {
	opts := baseTemplateOpts()
	opts.Duration = 2 * time.Hour
	tmpl := ShortLivedTemplate(TemplateServer, opts)
	dur := tmpl.NotAfter.Sub(tmpl.NotBefore)
	if dur < time.Hour || dur > 3*time.Hour {
		t.Errorf("duration = %v, expected ~2h", dur)
	}
	if len(tmpl.ExtKeyUsage) != 1 || tmpl.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Error("expected ServerAuth")
	}
}

func TestShortLivedTemplate_Client(t *testing.T) {
	opts := baseTemplateOpts()
	opts.Duration = 30 * time.Minute
	tmpl := ShortLivedTemplate(TemplateClient, opts)
	if len(tmpl.ExtKeyUsage) != 1 || tmpl.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Error("expected ClientAuth")
	}
}

func TestShortLivedTemplate_Mutual(t *testing.T) {
	opts := baseTemplateOpts()
	opts.Duration = time.Hour
	tmpl := ShortLivedTemplate(TemplateMutual, opts)
	if len(tmpl.ExtKeyUsage) != 2 {
		t.Errorf("ExtKeyUsage len = %d, want 2", len(tmpl.ExtKeyUsage))
	}
}

func TestShortLivedTemplate_Workload(t *testing.T) {
	opts := baseTemplateOpts()
	opts.SPIFFEID = "spiffe://test/wl"
	opts.Duration = time.Hour
	tmpl := ShortLivedTemplate(TemplateWorkload, opts)
	if len(tmpl.URIs) == 0 {
		t.Error("workload template should have SPIFFE URI")
	}
}

func TestShortLivedTemplate_DefaultType(t *testing.T) {
	opts := baseTemplateOpts()
	opts.Duration = time.Hour
	tmpl := ShortLivedTemplate(TemplateType(99), opts)
	// Default falls through to Mutual.
	if len(tmpl.ExtKeyUsage) != 2 {
		t.Errorf("default type should produce mutual template, ExtKeyUsage len = %d", len(tmpl.ExtKeyUsage))
	}
}

func TestShortLivedTemplate_ClampOver24h(t *testing.T) {
	opts := baseTemplateOpts()
	opts.Duration = 48 * time.Hour // Over 24h
	tmpl := ShortLivedTemplate(TemplateServer, opts)

	dur := tmpl.NotAfter.Sub(tmpl.NotBefore)
	if dur > 25*time.Hour {
		t.Errorf("duration = %v, should be clamped to 24h", dur)
	}
}

func TestShortLivedTemplate_ZeroDuration(t *testing.T) {
	opts := baseTemplateOpts()
	opts.Duration = 0
	tmpl := ShortLivedTemplate(TemplateServer, opts)

	dur := tmpl.NotAfter.Sub(tmpl.NotBefore)
	// Should default to 1 hour.
	if dur < 59*time.Minute || dur > 61*time.Minute {
		t.Errorf("duration = %v, expected ~1h for zero input", dur)
	}
}

// ---------------------------------------------------------------------------
// TemplateType constants
// ---------------------------------------------------------------------------

func TestTemplateTypeConstants(t *testing.T) {
	if TemplateServer != 0 {
		t.Errorf("TemplateServer = %d, want 0", TemplateServer)
	}
	if TemplateClient != 1 {
		t.Errorf("TemplateClient = %d, want 1", TemplateClient)
	}
	if TemplateMutual != 2 {
		t.Errorf("TemplateMutual = %d, want 2", TemplateMutual)
	}
	if TemplateWorkload != 3 {
		t.Errorf("TemplateWorkload = %d, want 3", TemplateWorkload)
	}
}
