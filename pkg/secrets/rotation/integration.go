// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rotation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/secrets"
)

// Common integration errors.
var (
	ErrDatabaseConnectionFailed = errors.New("database connection failed")
	ErrAPIKeyGenerationFailed   = errors.New("API key generation failed")
	ErrCertificateInvalid       = errors.New("certificate is invalid")
)

// Package-level integration metrics (registered once)
var (
	integrationMetricsOnce sync.Once
	rotationIntegrations   *metrics.CounterVec
	integrationErrors      *metrics.CounterVec
	integrationLatency     *metrics.HistogramVec
)

// initIntegrationMetrics initializes integration metrics.
func initIntegrationMetrics() {
	integrationMetricsOnce.Do(func() {
		rotationIntegrations = metrics.NewCounterVec(
			"unheaded_secrets_rotation_integrations_total",
			"Total rotation integrations by type",
			nil,
			[]string{"type", "status"},
		)
		_ = metrics.Register(rotationIntegrations)

		integrationErrors = metrics.NewCounterVec(
			"unheaded_secrets_rotation_integration_errors_total",
			"Total rotation integration errors",
			nil,
			[]string{"type", "error_type"},
		)
		_ = metrics.Register(integrationErrors)

		integrationLatency = metrics.NewHistogramVec(
			metrics.HistogramOpts{
				Name:    "unheaded_secrets_rotation_integration_duration_seconds",
				Help:    "Rotation integration operation latency",
				Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"type", "operation"},
		)
		_ = metrics.Register(integrationLatency)
	})
}

// CertificateIntegration handles certificate rotation integration.
type CertificateIntegration struct {
	manager  *RotationManager
	store    secrets.SecretStore
	log      *logger.Logger
	caConfig *CAConfig
	mu       sync.RWMutex
}

// CAConfig configures the certificate authority for issuing certificates.
type CAConfig struct {
	// CACert is the CA certificate PEM.
	CACert []byte

	// CAKey is the CA private key PEM.
	CAKey []byte

	// DefaultValidity is the default certificate validity period.
	DefaultValidity time.Duration

	// DefaultKeySize is the default RSA key size.
	DefaultKeySize int

	// DefaultOrganization is the default organization for certificates.
	DefaultOrganization string
}

// CertificateIntegrationConfig configures the certificate integration.
type CertificateIntegrationConfig struct {
	// Manager is the rotation manager.
	Manager *RotationManager

	// Store is the secret store.
	Store secrets.SecretStore

	// CAConfig configures the certificate authority.
	CAConfig *CAConfig

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewCertificateIntegration creates a new certificate integration.
func NewCertificateIntegration(config CertificateIntegrationConfig) (*CertificateIntegration, error) {
	if config.Manager == nil {
		return nil, errors.New("rotation manager is required")
	}
	if config.Store == nil {
		return nil, errors.New("secret store is required")
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	initIntegrationMetrics()

	ci := &CertificateIntegration{
		manager:  config.Manager,
		store:    config.Store,
		log:      log.With().Str("component", "cert-integration").Logger(),
		caConfig: config.CAConfig,
	}

	// Register certificate rotator if not already registered
	ci.manager.RegisterRotator(&CertificateRotatorWithCA{ca: config.CAConfig})

	return ci, nil
}

// RotateCertificate rotates a certificate.
func (ci *CertificateIntegration) RotateCertificate(ctx context.Context, path string, opts CertificateOptions) (*RotationResult, error) {
	start := time.Now()
	defer func() {
		integrationLatency.WithLabels(metrics.Labels{"type": "certificate", "operation": "rotate"}).Observe(time.Since(start).Seconds())
	}()

	config := RotationConfig{
		TTL: opts.Validity,
		Parameters: map[string]interface{}{
			"common_name": opts.CommonName,
			"dns_names":   opts.DNSNames,
			"ip_addresses": opts.IPAddresses,
			"key_size":    opts.KeySize,
			"is_ca":       opts.IsCA,
		},
	}

	result, err := ci.manager.Rotate(ctx, path, config)
	if err != nil {
		integrationErrors.WithLabels(metrics.Labels{"type": "certificate", "error_type": "rotation_failed"}).Inc()
		return result, err
	}

	if result.Success {
		rotationIntegrations.WithLabels(metrics.Labels{"type": "certificate", "status": "success"}).Inc()
	} else {
		rotationIntegrations.WithLabels(metrics.Labels{"type": "certificate", "status": "failed"}).Inc()
	}

	return result, nil
}

// CertificateOptions contains options for certificate rotation.
type CertificateOptions struct {
	// CommonName is the certificate common name.
	CommonName string

	// DNSNames are additional DNS names for the certificate.
	DNSNames []string

	// IPAddresses are IP addresses for the certificate.
	IPAddresses []string

	// Validity is how long the certificate is valid.
	Validity time.Duration

	// KeySize is the RSA key size.
	KeySize int

	// IsCA indicates if this is a CA certificate.
	IsCA bool
}

// GetCertificateInfo retrieves information about a certificate.
func (ci *CertificateIntegration) GetCertificateInfo(ctx context.Context, path string) (*CertificateInfo, error) {
	secret, err := ci.store.Get(ctx, path)
	if err != nil {
		return nil, err
	}

	certPEM, ok := secret.Data["cert"]
	if !ok {
		return nil, ErrCertificateInvalid
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, ErrCertificateInvalid
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	return &CertificateInfo{
		Path:         path,
		CommonName:   cert.Subject.CommonName,
		Organization: cert.Subject.Organization,
		DNSNames:     cert.DNSNames,
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		SerialNumber: cert.SerialNumber.String(),
		IsCA:         cert.IsCA,
		Version:      secret.Version,
	}, nil
}

// CertificateInfo contains information about a certificate.
type CertificateInfo struct {
	Path         string    `json:"path"`
	CommonName   string    `json:"common_name"`
	Organization []string  `json:"organization"`
	DNSNames     []string  `json:"dns_names"`
	NotBefore    time.Time `json:"not_before"`
	NotAfter     time.Time `json:"not_after"`
	SerialNumber string    `json:"serial_number"`
	IsCA         bool      `json:"is_ca"`
	Version      int64     `json:"version"`
}

// ListExpiringCertificates returns certificates expiring within the threshold.
func (ci *CertificateIntegration) ListExpiringCertificates(ctx context.Context, threshold time.Duration) ([]*CertificateInfo, error) {
	paths, err := ci.store.List(ctx, "certificates/")
	if err != nil {
		return nil, err
	}

	var expiring []*CertificateInfo
	cutoff := time.Now().Add(threshold)

	for _, path := range paths {
		info, err := ci.GetCertificateInfo(ctx, path)
		if err != nil {
			continue
		}

		if info.NotAfter.Before(cutoff) {
			expiring = append(expiring, info)
		}
	}

	return expiring, nil
}

// CertificateRotatorWithCA is a certificate rotator that uses a CA.
type CertificateRotatorWithCA struct {
	ca *CAConfig
}

// Type returns the secret type.
func (r *CertificateRotatorWithCA) Type() secrets.SecretType {
	return secrets.SecretTypeCertificate
}

// Rotate generates a new certificate.
func (r *CertificateRotatorWithCA) Rotate(ctx context.Context, current *secrets.Secret, config RotationConfig) (*secrets.Secret, error) {
	newSecret := current.Clone()

	// Get certificate parameters
	commonName := "localhost"
	if cn, ok := config.Parameters["common_name"].(string); ok {
		commonName = cn
	}

	dnsNames := []string{commonName}
	if names, ok := config.Parameters["dns_names"].([]string); ok {
		dnsNames = names
	}

	keySize := 2048
	if r.ca != nil && r.ca.DefaultKeySize > 0 {
		keySize = r.ca.DefaultKeySize
	}
	if ks, ok := config.Parameters["key_size"].(int); ok {
		keySize = ks
	}

	validity := 365 * 24 * time.Hour
	if r.ca != nil && r.ca.DefaultValidity > 0 {
		validity = r.ca.DefaultValidity
	}
	if config.TTL > 0 {
		validity = config.TTL
	}

	isCA := false
	if ic, ok := config.Parameters["is_ca"].(bool); ok {
		isCA = ic
	}

	// Generate new key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(validity)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		DNSNames:              dnsNames,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}

	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	// Determine parent and signer
	var parent *x509.Certificate
	var signerKey interface{}

	if r.ca != nil && len(r.ca.CACert) > 0 && len(r.ca.CAKey) > 0 {
		// Use CA to sign
		block, _ := pem.Decode(r.ca.CACert)
		if block != nil {
			parent, err = x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parse CA cert: %w", err)
			}
		}

		block, _ = pem.Decode(r.ca.CAKey)
		if block != nil {
			signerKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				signerKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
				if err != nil {
					return nil, fmt.Errorf("parse CA key: %w", err)
				}
			}
		}
	}

	if parent == nil {
		// Self-signed
		parent = &template
		signerKey = privateKey
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, parent, &privateKey.PublicKey, signerKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	newSecret.Data["cert"] = certPEM
	newSecret.Data["key"] = keyPEM
	newSecret.ExpiresAt = notAfter

	// Include CA cert if we have one
	if r.ca != nil && len(r.ca.CACert) > 0 {
		newSecret.Data["ca_cert"] = r.ca.CACert
	}

	return newSecret, nil
}

// Verify validates the certificate.
func (r *CertificateRotatorWithCA) Verify(ctx context.Context, secret *secrets.Secret) error {
	certPEM, ok := secret.Data["cert"]
	if !ok {
		return errors.New("certificate missing")
	}

	keyPEM, ok := secret.Data["key"]
	if !ok {
		return errors.New("private key missing")
	}

	// Parse certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return errors.New("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}

	// Parse private key
	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return errors.New("failed to parse key PEM")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// Verify key matches certificate
	if cert.PublicKey.(*rsa.PublicKey).N.Cmp(key.N) != 0 {
		return errors.New("private key does not match certificate")
	}

	// Check expiration
	if time.Now().After(cert.NotAfter) {
		return errors.New("certificate is expired")
	}

	return nil
}

// Rollback reverts to the previous certificate.
func (r *CertificateRotatorWithCA) Rollback(ctx context.Context, current, previous *secrets.Secret) error {
	return nil
}

// DatabaseCredentialIntegration handles database credential rotation.
type DatabaseCredentialIntegration struct {
	manager *RotationManager
	store   secrets.SecretStore
	log     *logger.Logger
	drivers map[string]DatabaseDriver
	mu      sync.RWMutex
}

// DatabaseDriver is the interface for database-specific operations.
type DatabaseDriver interface {
	// Connect establishes a connection to the database.
	Connect(ctx context.Context, config DatabaseConfig) (*sql.DB, error)

	// RotateCredentials rotates the database credentials.
	RotateCredentials(ctx context.Context, db *sql.DB, username, newPassword string) error

	// ValidateCredentials validates credentials work.
	ValidateCredentials(ctx context.Context, config DatabaseConfig) error
}

// DatabaseConfig contains database connection configuration.
type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"`
}

// DatabaseCredentialIntegrationConfig configures the database credential integration.
type DatabaseCredentialIntegrationConfig struct {
	// Manager is the rotation manager.
	Manager *RotationManager

	// Store is the secret store.
	Store secrets.SecretStore

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewDatabaseCredentialIntegration creates a new database credential integration.
func NewDatabaseCredentialIntegration(config DatabaseCredentialIntegrationConfig) (*DatabaseCredentialIntegration, error) {
	if config.Manager == nil {
		return nil, errors.New("rotation manager is required")
	}
	if config.Store == nil {
		return nil, errors.New("secret store is required")
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	initIntegrationMetrics()

	return &DatabaseCredentialIntegration{
		manager: config.Manager,
		store:   config.Store,
		log:     log.With().Str("component", "db-credential-integration").Logger(),
		drivers: make(map[string]DatabaseDriver),
	}, nil
}

// RegisterDriver registers a database driver.
func (dci *DatabaseCredentialIntegration) RegisterDriver(name string, driver DatabaseDriver) {
	dci.mu.Lock()
	defer dci.mu.Unlock()
	dci.drivers[name] = driver
	dci.log.Info().Str("driver", name).Msg("registered database driver")
}

// RotateCredentials rotates database credentials.
func (dci *DatabaseCredentialIntegration) RotateCredentials(ctx context.Context, path, driverName string, opts CredentialRotationOptions) (*RotationResult, error) {
	start := time.Now()
	defer func() {
		integrationLatency.WithLabels(metrics.Labels{"type": "database", "operation": "rotate"}).Observe(time.Since(start).Seconds())
	}()

	dci.mu.RLock()
	driver, ok := dci.drivers[driverName]
	dci.mu.RUnlock()

	if !ok {
		integrationErrors.WithLabels(metrics.Labels{"type": "database", "error_type": "driver_not_found"}).Inc()
		return nil, fmt.Errorf("database driver not found: %s", driverName)
	}

	// Get current credentials
	secret, err := dci.store.Get(ctx, path)
	if err != nil {
		integrationErrors.WithLabels(metrics.Labels{"type": "database", "error_type": "get_secret_failed"}).Inc()
		return nil, err
	}

	// Parse current config
	config := DatabaseConfig{
		Host:     string(secret.Data["host"]),
		Database: string(secret.Data["database"]),
		Username: string(secret.Data["username"]),
		Password: string(secret.Data["password"]),
		SSLMode:  string(secret.Data["ssl_mode"]),
	}
	if port, ok := secret.Data["port"]; ok {
		fmt.Sscanf(string(port), "%d", &config.Port)
	}

	// Generate new password
	newPassword, err := generateSecurePassword(opts.PasswordLength, opts.PasswordCharset)
	if err != nil {
		integrationErrors.WithLabels(metrics.Labels{"type": "database", "error_type": "password_generation_failed"}).Inc()
		return nil, fmt.Errorf("generate password: %w", err)
	}

	// Connect to database
	db, err := driver.Connect(ctx, config)
	if err != nil {
		integrationErrors.WithLabels(metrics.Labels{"type": "database", "error_type": "connection_failed"}).Inc()
		return nil, fmt.Errorf("%w: %v", ErrDatabaseConnectionFailed, err)
	}
	defer db.Close()

	// Rotate credentials
	if err := driver.RotateCredentials(ctx, db, config.Username, newPassword); err != nil {
		integrationErrors.WithLabels(metrics.Labels{"type": "database", "error_type": "rotation_failed"}).Inc()
		return nil, err
	}

	// Validate new credentials
	newConfig := config
	newConfig.Password = newPassword
	if err := driver.ValidateCredentials(ctx, newConfig); err != nil {
		// Rollback
		db2, _ := driver.Connect(ctx, config)
		if db2 != nil {
			driver.RotateCredentials(ctx, db2, config.Username, config.Password)
			db2.Close()
		}
		integrationErrors.WithLabels(metrics.Labels{"type": "database", "error_type": "validation_failed"}).Inc()
		return nil, fmt.Errorf("validate new credentials: %w", err)
	}

	// Update secret
	newSecret := secret.Clone()
	newSecret.Data["password"] = []byte(newPassword)
	newSecret.RotatedAt = time.Now()

	rotationConfig := RotationConfig{
		KeepPrevious: opts.KeepPrevious,
		GracePeriod:  opts.GracePeriod,
	}

	result := &RotationResult{
		Success:        true,
		NewSecret:      newSecret,
		PreviousSecret: secret,
		Timestamp:      time.Now(),
	}

	if rotationConfig.KeepPrevious {
		result.PreviousSecret = secret
	}

	if err := dci.store.Set(ctx, path, newSecret); err != nil {
		result.Success = false
		result.Error = err
		integrationErrors.WithLabels(metrics.Labels{"type": "database", "error_type": "store_failed"}).Inc()
		return result, err
	}

	rotationIntegrations.WithLabels(metrics.Labels{"type": "database", "status": "success"}).Inc()
	dci.log.Info().Str("path", path).Str("driver", driverName).Msg("rotated database credentials")

	return result, nil
}

// CredentialRotationOptions contains options for credential rotation.
type CredentialRotationOptions struct {
	PasswordLength  int
	PasswordCharset string
	KeepPrevious    bool
	GracePeriod     time.Duration
}

// APIKeyIntegration handles API key rotation.
type APIKeyIntegration struct {
	manager    *RotationManager
	store      secrets.SecretStore
	log        *logger.Logger
	generators map[string]APIKeyGenerator
	mu         sync.RWMutex
}

// APIKeyGenerator generates API keys.
type APIKeyGenerator interface {
	// Generate creates a new API key.
	Generate(ctx context.Context, opts APIKeyOptions) (string, error)

	// Revoke revokes an API key.
	Revoke(ctx context.Context, key string) error

	// Validate validates an API key.
	Validate(ctx context.Context, key string) error
}

// APIKeyOptions contains options for API key generation.
type APIKeyOptions struct {
	// Length is the key length.
	Length int

	// Prefix is the key prefix (e.g., "sk_").
	Prefix string

	// Scopes are the key scopes/permissions.
	Scopes []string

	// ExpiresAt is when the key expires.
	ExpiresAt time.Time

	// Metadata is additional key metadata.
	Metadata map[string]string
}

// APIKeyIntegrationConfig configures the API key integration.
type APIKeyIntegrationConfig struct {
	// Manager is the rotation manager.
	Manager *RotationManager

	// Store is the secret store.
	Store secrets.SecretStore

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewAPIKeyIntegration creates a new API key integration.
func NewAPIKeyIntegration(config APIKeyIntegrationConfig) (*APIKeyIntegration, error) {
	if config.Manager == nil {
		return nil, errors.New("rotation manager is required")
	}
	if config.Store == nil {
		return nil, errors.New("secret store is required")
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	initIntegrationMetrics()

	aki := &APIKeyIntegration{
		manager:    config.Manager,
		store:      config.Store,
		log:        log.With().Str("component", "api-key-integration").Logger(),
		generators: make(map[string]APIKeyGenerator),
	}

	// Register default generator
	aki.RegisterGenerator("default", &DefaultAPIKeyGenerator{})

	return aki, nil
}

// RegisterGenerator registers an API key generator.
func (aki *APIKeyIntegration) RegisterGenerator(name string, gen APIKeyGenerator) {
	aki.mu.Lock()
	defer aki.mu.Unlock()
	aki.generators[name] = gen
	aki.log.Info().Str("generator", name).Msg("registered API key generator")
}

// RotateAPIKey rotates an API key.
func (aki *APIKeyIntegration) RotateAPIKey(ctx context.Context, path, generatorName string, opts APIKeyOptions) (*RotationResult, error) {
	start := time.Now()
	defer func() {
		integrationLatency.WithLabels(metrics.Labels{"type": "api_key", "operation": "rotate"}).Observe(time.Since(start).Seconds())
	}()

	aki.mu.RLock()
	gen, ok := aki.generators[generatorName]
	aki.mu.RUnlock()

	if !ok {
		if generatorName == "" {
			generatorName = "default"
			gen = &DefaultAPIKeyGenerator{}
		} else {
			integrationErrors.WithLabels(metrics.Labels{"type": "api_key", "error_type": "generator_not_found"}).Inc()
			return nil, fmt.Errorf("API key generator not found: %s", generatorName)
		}
	}

	// Get current secret (may not exist)
	current, _ := aki.store.Get(ctx, path)

	// Generate new key
	newKey, err := gen.Generate(ctx, opts)
	if err != nil {
		integrationErrors.WithLabels(metrics.Labels{"type": "api_key", "error_type": "generation_failed"}).Inc()
		return nil, fmt.Errorf("%w: %v", ErrAPIKeyGenerationFailed, err)
	}

	// Validate new key
	if err := gen.Validate(ctx, newKey); err != nil {
		integrationErrors.WithLabels(metrics.Labels{"type": "api_key", "error_type": "validation_failed"}).Inc()
		return nil, fmt.Errorf("validate API key: %w", err)
	}

	// Revoke old key
	if current != nil {
		oldKey := string(current.Data["key"])
		if oldKey != "" {
			if err := gen.Revoke(ctx, oldKey); err != nil {
				aki.log.Warn().Err(err).Str("path", path).Msg("failed to revoke old API key")
			}
		}
	}

	// Create new secret
	now := time.Now()
	newSecret := &secrets.Secret{
		Path: path,
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{
			"key": []byte(newKey),
		},
		Metadata: map[string]string{
			"generator": generatorName,
		},
		CreatedAt: now,
		UpdatedAt: now,
		RotatedAt: now,
	}

	if !opts.ExpiresAt.IsZero() {
		newSecret.ExpiresAt = opts.ExpiresAt
	}

	for k, v := range opts.Metadata {
		newSecret.Metadata[k] = v
	}

	if len(opts.Scopes) > 0 {
		newSecret.Metadata["scopes"] = strings.Join(opts.Scopes, ",")
	}

	if current != nil {
		newSecret.Version = current.Version + 1
		newSecret.CreatedAt = current.CreatedAt
	} else {
		newSecret.Version = 1
	}

	// Store
	if err := aki.store.Set(ctx, path, newSecret); err != nil {
		// Revoke new key on store failure
		gen.Revoke(ctx, newKey)
		integrationErrors.WithLabels(metrics.Labels{"type": "api_key", "error_type": "store_failed"}).Inc()
		return nil, err
	}

	result := &RotationResult{
		Success:   true,
		NewSecret: newSecret,
		Timestamp: now,
	}

	rotationIntegrations.WithLabels(metrics.Labels{"type": "api_key", "status": "success"}).Inc()
	aki.log.Info().Str("path", path).Str("generator", generatorName).Msg("rotated API key")

	return result, nil
}

// DefaultAPIKeyGenerator is a simple API key generator.
type DefaultAPIKeyGenerator struct{}

// Generate creates a new API key.
func (g *DefaultAPIKeyGenerator) Generate(ctx context.Context, opts APIKeyOptions) (string, error) {
	length := opts.Length
	if length <= 0 {
		length = 32
	}

	keyBytes := make([]byte, length)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", err
	}

	key := base64.URLEncoding.EncodeToString(keyBytes)
	if opts.Prefix != "" {
		key = opts.Prefix + key
	}

	return key[:len(opts.Prefix)+length], nil
}

// Revoke revokes an API key (no-op for default generator).
func (g *DefaultAPIKeyGenerator) Revoke(ctx context.Context, key string) error {
	return nil
}

// Validate validates an API key.
func (g *DefaultAPIKeyGenerator) Validate(ctx context.Context, key string) error {
	if len(key) < 16 {
		return errors.New("API key too short")
	}
	return nil
}

// ExternalAPIKeyGenerator generates keys through an external API.
type ExternalAPIKeyGenerator struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewExternalAPIKeyGenerator creates a new external API key generator.
func NewExternalAPIKeyGenerator(baseURL, authToken string) *ExternalAPIKeyGenerator {
	return &ExternalAPIKeyGenerator{
		baseURL:   baseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Generate creates a new API key through the external API.
func (g *ExternalAPIKeyGenerator) Generate(ctx context.Context, opts APIKeyOptions) (string, error) {
	// This is a placeholder - real implementation would call external API
	return (&DefaultAPIKeyGenerator{}).Generate(ctx, opts)
}

// Revoke revokes an API key through the external API.
func (g *ExternalAPIKeyGenerator) Revoke(ctx context.Context, key string) error {
	// This is a placeholder - real implementation would call external API
	return nil
}

// Validate validates an API key through the external API.
func (g *ExternalAPIKeyGenerator) Validate(ctx context.Context, key string) error {
	// This is a placeholder - real implementation would call external API
	return nil
}

// generateSecurePassword generates a secure random password.
func generateSecurePassword(length int, charset string) (string, error) {
	if length <= 0 {
		length = 32
	}
	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	}

	password := make([]byte, length)
	for i := range password {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		password[i] = charset[n.Int64()]
	}

	return string(password), nil
}

// RotationMetricsCollector collects rotation metrics.
type RotationMetricsCollector struct {
	manager  *RotationManager
	tracker  *HistoryTracker
	interval time.Duration
	stopCh   chan struct{}
	log      *logger.Logger
}

// RotationMetricsCollectorConfig configures the metrics collector.
type RotationMetricsCollectorConfig struct {
	// Manager is the rotation manager.
	Manager *RotationManager

	// Tracker is the history tracker.
	Tracker *HistoryTracker

	// Interval is how often to collect metrics.
	Interval time.Duration

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewRotationMetricsCollector creates a new metrics collector.
func NewRotationMetricsCollector(config RotationMetricsCollectorConfig) *RotationMetricsCollector {
	interval := config.Interval
	if interval <= 0 {
		interval = time.Minute
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	return &RotationMetricsCollector{
		manager:  config.Manager,
		tracker:  config.Tracker,
		interval: interval,
		stopCh:   make(chan struct{}),
		log:      log.With().Str("component", "rotation-metrics").Logger(),
	}
}

// Start starts the metrics collector.
func (c *RotationMetricsCollector) Start() {
	go c.collect()
}

// Stop stops the metrics collector.
func (c *RotationMetricsCollector) Stop() {
	close(c.stopCh)
}

// collect periodically collects metrics.
func (c *RotationMetricsCollector) collect() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.collectMetrics()
		case <-c.stopCh:
			return
		}
	}
}

// collectMetrics collects rotation metrics.
func (c *RotationMetricsCollector) collectMetrics() {
	if c.tracker == nil {
		return
	}

	ctx := context.Background()

	// Get stats for last hour
	stats, err := c.tracker.GetStats(ctx, "", time.Now().Add(-time.Hour))
	if err != nil {
		c.log.Warn().Err(err).Msg("failed to get rotation stats")
		return
	}

	// Log metrics
	c.log.Debug().
		Int("total", stats.TotalRotations).
		Int("successful", stats.SuccessfulRotations).
		Int("failed", stats.FailedRotations).
		Float64("success_rate", stats.SuccessRate).
		Dur("avg_duration", stats.AverageDuration).
		Msg("rotation metrics collected")
}
