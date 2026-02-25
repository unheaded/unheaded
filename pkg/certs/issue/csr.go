package issue

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"net/url"
)

// CSRRequest contains parameters for CSR generation.
type CSRRequest struct {
	CommonName         string
	Organization       string
	OrganizationalUnit string
	Country            string
	DNSNames           []string
	IPAddresses        []string
	URIs               []string
	SPIFFEID           string
}

// CSRResult contains the generated CSR and private key.
type CSRResult struct {
	CSRPEM        []byte
	PrivateKeyPEM []byte
}

// GenerateCSR creates a new certificate signing request.
func GenerateCSR(req *CSRRequest) (*CSRResult, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// Parse IP addresses
	var ips []net.IP
	for _, ipStr := range req.IPAddresses {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			ips = append(ips, ip)
		}
	}

	// Parse URIs
	var uris []*url.URL
	if req.SPIFFEID != "" {
		spiffeURL, err := url.Parse(req.SPIFFEID)
		if err == nil {
			uris = append(uris, spiffeURL)
		}
	}
	for _, uriStr := range req.URIs {
		uri, err := url.Parse(uriStr)
		if err == nil {
			uris = append(uris, uri)
		}
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         req.CommonName,
			Organization:       []string{req.Organization},
			OrganizationalUnit: []string{req.OrganizationalUnit},
			Country:            []string{req.Country},
		},
		DNSNames:    req.DNSNames,
		IPAddresses: ips,
		URIs:        uris,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		return nil, err
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return &CSRResult{
		CSRPEM:        csrPEM,
		PrivateKeyPEM: keyPEM,
	}, nil
}

// ParseCSR parses a PEM-encoded CSR.
func ParseCSR(csrPEM []byte) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, ErrCSRParseFailed
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, err
	}

	// Verify CSR signature
	if err := csr.CheckSignature(); err != nil {
		return nil, err
	}

	return csr, nil
}

// ValidateCSR validates a CSR for security requirements.
func ValidateCSR(csr *x509.CertificateRequest) error {
	// Verify signature
	if err := csr.CheckSignature(); err != nil {
		return err
	}

	// Check key algorithm
	switch csr.PublicKeyAlgorithm {
	case x509.ECDSA:
		// Allowed
	case x509.RSA:
		// Allowed but check key size
	default:
		return ErrInvalidRequest
	}

	return nil
}
