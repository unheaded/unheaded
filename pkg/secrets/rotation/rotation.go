// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package rotation provides secret rotation functionality for the Crystal Grotto.
package rotation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
	"time"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/secrets"
)

// Common errors for rotation operations.
var (
	ErrRotationInProgress = errors.New("rotation already in progress")
	ErrNoRotator          = errors.New("no rotator registered for secret type")
	ErrRotationFailed     = errors.New("rotation failed")
)

// Rotator rotates secrets of a specific type.
type Rotator interface {
	// Type returns the secret type this rotator handles.
	Type() secrets.SecretType

	// Rotate generates new secret data.
	Rotate(ctx context.Context, current *secrets.Secret, config RotationConfig) (*secrets.Secret, error)

	// Verify validates that the new secret is working.
	Verify(ctx context.Context, secret *secrets.Secret) error

	// Rollback reverts to the previous secret if rotation fails.
	Rollback(ctx context.Context, current, previous *secrets.Secret) error
}

// RotationConfig configures a rotation operation.
type RotationConfig struct {
	// TTL is the lifetime of the new secret.
	TTL time.Duration

	// Parameters are type-specific rotation parameters.
	Parameters map[string]interface{}

	// KeepPrevious indicates whether to keep the previous version.
	KeepPrevious bool

	// GracePeriod is how long both old and new secrets should be valid.
	GracePeriod time.Duration
}

// RotationResult contains the result of a rotation operation.
type RotationResult struct {
	// Success indicates if the rotation succeeded.
	Success bool

	// NewSecret is the newly rotated secret.
	NewSecret *secrets.Secret

	// PreviousSecret is the previous secret (if retained).
	PreviousSecret *secrets.Secret

	// Error is the error if rotation failed.
	Error error

	// Duration is how long the rotation took.
	Duration time.Duration

	// Timestamp is when the rotation occurred.
	Timestamp time.Time
}

// RotationManager manages secret rotation.
type RotationManager struct {
	store      secrets.SecretStore
	rotators   map[secrets.SecretType]Rotator
	inProgress map[string]bool
	log        *logger.Logger
	mu         sync.RWMutex
}

// Package-level metrics (registered once)
var (
	rotationMetricsOnce  sync.Once
	rotationsTotal       *metrics.CounterVec
	rotationDuration     *metrics.HistogramVec
	rotationErrors       *metrics.CounterVec
)

// RotationManagerConfig configures the rotation manager.
type RotationManagerConfig struct {
	// Store is the secret store.
	Store secrets.SecretStore

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewRotationManager creates a new rotation manager.
func NewRotationManager(config RotationManagerConfig) (*RotationManager, error) {
	if config.Store == nil {
		return nil, errors.New("store is required")
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	rm := &RotationManager{
		store:      config.Store,
		rotators:   make(map[secrets.SecretType]Rotator),
		inProgress: make(map[string]bool),
		log:        log.With().Str("component", "rotation-manager").Logger(),
	}

	// Initialize metrics (only once per process)
	rotationMetricsOnce.Do(func() {
		rotationsTotal = metrics.NewCounterVec(
			"unheaded_secrets_rotations_total",
			"Total secret rotations",
			nil,
			[]string{"type", "status"},
		)
		_ = metrics.Register(rotationsTotal)

		rotationDuration = metrics.NewHistogramVec(
			metrics.HistogramOpts{
				Name:    "unheaded_secrets_rotation_duration_seconds",
				Help:    "Secret rotation duration",
				Buckets: []float64{.1, .5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"type"},
		)
		_ = metrics.Register(rotationDuration)

		rotationErrors = metrics.NewCounterVec(
			"unheaded_secrets_rotation_errors_total",
			"Total secret rotation errors",
			nil,
			[]string{"type", "error_type"},
		)
		_ = metrics.Register(rotationErrors)
	})

	// Register default rotators
	rm.RegisterRotator(&StaticSecretRotator{})
	rm.RegisterRotator(&CertificateRotator{})
	rm.RegisterRotator(&SSHKeyRotator{})

	return rm, nil
}

// RegisterRotator registers a rotator for a secret type.
func (rm *RotationManager) RegisterRotator(r Rotator) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.rotators[r.Type()] = r
	rm.log.Info().Str("type", string(r.Type())).Msg("rotator registered")
}

// Rotate rotates a secret.
func (rm *RotationManager) Rotate(ctx context.Context, path string, config RotationConfig) (*RotationResult, error) {
	start := time.Now()
	result := &RotationResult{
		Timestamp: start,
	}

	// Check if rotation is already in progress
	rm.mu.Lock()
	if rm.inProgress[path] {
		rm.mu.Unlock()
		return nil, ErrRotationInProgress
	}
	rm.inProgress[path] = true
	rm.mu.Unlock()

	defer func() {
		rm.mu.Lock()
		delete(rm.inProgress, path)
		rm.mu.Unlock()
		result.Duration = time.Since(start)
	}()

	// Get current secret
	current, err := rm.store.Get(ctx, path)
	if err != nil {
		result.Error = fmt.Errorf("get current secret: %w", err)
		rotationErrors.WithLabels(metrics.Labels{"type": "unknown", "error_type": "get_failed"}).Inc()
		return result, result.Error
	}

	// Find rotator
	rm.mu.RLock()
	rotator, ok := rm.rotators[current.Type]
	rm.mu.RUnlock()

	if !ok {
		result.Error = fmt.Errorf("%w: %s", ErrNoRotator, current.Type)
		rotationErrors.WithLabels(metrics.Labels{"type": string(current.Type), "error_type": "no_rotator"}).Inc()
		return result, result.Error
	}

	rm.log.Info().
		Str("path", path).
		Str("type", string(current.Type)).
		Int64("current_version", current.Version).
		Msg("starting rotation")

	// Rotate
	newSecret, err := rotator.Rotate(ctx, current, config)
	if err != nil {
		result.Error = fmt.Errorf("rotate: %w", err)
		rotationErrors.WithLabels(metrics.Labels{"type": string(current.Type), "error_type": "rotate_failed"}).Inc()
		return result, result.Error
	}

	// Verify new secret
	if err := rotator.Verify(ctx, newSecret); err != nil {
		rm.log.Warn().Err(err).Str("path", path).Msg("verification failed, rolling back")
		rotationErrors.WithLabels(metrics.Labels{"type": string(current.Type), "error_type": "verify_failed"}).Inc()

		if rbErr := rotator.Rollback(ctx, newSecret, current); rbErr != nil {
			rm.log.Error().Err(rbErr).Str("path", path).Msg("rollback failed")
		}

		result.Error = fmt.Errorf("verify: %w", err)
		return result, result.Error
	}

	// Store new secret
	newSecret.RotatedAt = time.Now()
	if err := rm.store.Set(ctx, path, newSecret); err != nil {
		rm.log.Warn().Err(err).Str("path", path).Msg("store failed, rolling back")
		rotationErrors.WithLabels(metrics.Labels{"type": string(current.Type), "error_type": "store_failed"}).Inc()

		if rbErr := rotator.Rollback(ctx, newSecret, current); rbErr != nil {
			rm.log.Error().Err(rbErr).Str("path", path).Msg("rollback failed")
		}

		result.Error = fmt.Errorf("store: %w", err)
		return result, result.Error
	}

	// Success
	result.Success = true
	result.NewSecret = newSecret
	if config.KeepPrevious {
		result.PreviousSecret = current
	}

	rotationsTotal.WithLabels(metrics.Labels{"type": string(current.Type), "status": "success"}).Inc()
	rotationDuration.WithLabels(metrics.Labels{"type": string(current.Type)}).Observe(time.Since(start).Seconds())

	rm.log.Info().
		Str("path", path).
		Str("type", string(current.Type)).
		Int64("old_version", current.Version).
		Int64("new_version", newSecret.Version).
		Dur("duration", result.Duration).
		Msg("rotation complete")

	return result, nil
}

// StaticSecretRotator rotates static secrets (passwords, API keys).
type StaticSecretRotator struct{}

// Type returns the secret type.
func (r *StaticSecretRotator) Type() secrets.SecretType {
	return secrets.SecretTypeStatic
}

// Rotate generates new secret data.
func (r *StaticSecretRotator) Rotate(ctx context.Context, current *secrets.Secret, config RotationConfig) (*secrets.Secret, error) {
	newSecret := current.Clone()

	// Get length from parameters or use default
	length := 32
	if l, ok := config.Parameters["length"].(int); ok {
		length = l
	}

	// Get charset from parameters
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	if cs, ok := config.Parameters["charset"].(string); ok {
		charset = cs
	}

	// Generate new value
	value := make([]byte, length)
	for i := range value {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return nil, err
		}
		value[i] = charset[n.Int64()]
	}

	newSecret.Data["value"] = value

	// Set expiration
	if config.TTL > 0 {
		newSecret.ExpiresAt = time.Now().Add(config.TTL)
	}

	return newSecret, nil
}

// Verify validates the new secret.
func (r *StaticSecretRotator) Verify(ctx context.Context, secret *secrets.Secret) error {
	if _, ok := secret.Data["value"]; !ok {
		return errors.New("secret missing value")
	}
	return nil
}

// Rollback reverts to the previous secret.
func (r *StaticSecretRotator) Rollback(ctx context.Context, current, previous *secrets.Secret) error {
	// Static secrets don't need external rollback
	return nil
}

// CertificateRotator rotates TLS certificates.
type CertificateRotator struct{}

// Type returns the secret type.
func (r *CertificateRotator) Type() secrets.SecretType {
	return secrets.SecretTypeCertificate
}

// Rotate generates a new certificate.
func (r *CertificateRotator) Rotate(ctx context.Context, current *secrets.Secret, config RotationConfig) (*secrets.Secret, error) {
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
	if ks, ok := config.Parameters["key_size"].(int); ok {
		keySize = ks
	}

	validity := 365 * 24 * time.Hour
	if config.TTL > 0 {
		validity = config.TTL
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
	}

	// Check if we should use a CA to sign
	var parent *x509.Certificate
	var signerKey interface{}

	if caCert, ok := current.Data["ca_cert"]; ok {
		if caKey, ok := current.Data["ca_key"]; ok {
			// Parse CA cert
			block, _ := pem.Decode(caCert)
			if block != nil {
				parent, err = x509.ParseCertificate(block.Bytes)
				if err != nil {
					return nil, fmt.Errorf("parse CA cert: %w", err)
				}
			}

			// Parse CA key
			block, _ = pem.Decode(caKey)
			if block != nil {
				signerKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
				if err != nil {
					// Try PKCS1
					signerKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
					if err != nil {
						return nil, fmt.Errorf("parse CA key: %w", err)
					}
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

	// Copy CA cert if present
	if caCert, ok := current.Data["ca_cert"]; ok {
		newSecret.Data["ca_cert"] = caCert
	}

	return newSecret, nil
}

// Verify validates the certificate.
func (r *CertificateRotator) Verify(ctx context.Context, secret *secrets.Secret) error {
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
func (r *CertificateRotator) Rollback(ctx context.Context, current, previous *secrets.Secret) error {
	// Certificates don't need external rollback
	return nil
}

// SSHKeyRotator rotates SSH key pairs.
type SSHKeyRotator struct{}

// Type returns the secret type.
func (r *SSHKeyRotator) Type() secrets.SecretType {
	return secrets.SecretTypeSSHKey
}

// Rotate generates a new SSH key pair.
func (r *SSHKeyRotator) Rotate(ctx context.Context, current *secrets.Secret, config RotationConfig) (*secrets.Secret, error) {
	newSecret := current.Clone()

	// Get key type from parameters
	keyType := "ed25519"
	if kt, ok := config.Parameters["key_type"].(string); ok {
		keyType = kt
	}

	comment := ""
	if c, ok := config.Parameters["comment"].(string); ok {
		comment = c
	}

	var privateKey, publicKey []byte
	var err error

	switch keyType {
	case "ed25519":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ed25519 key: %w", err)
		}

		// Convert to OpenSSH format
		sshPub, err := ssh.NewPublicKey(pub)
		if err != nil {
			return nil, err
		}
		publicKey = ssh.MarshalAuthorizedKey(sshPub)
		if comment != "" {
			publicKey = append(publicKey[:len(publicKey)-1], []byte(" "+comment+"\n")...)
		}

		// Marshal private key
		privPEM, err := ssh.MarshalPrivateKey(priv, comment)
		if err != nil {
			return nil, err
		}
		privateKey = pem.EncodeToMemory(privPEM)

	case "rsa":
		keySize := 4096
		if ks, ok := config.Parameters["key_size"].(int); ok {
			keySize = ks
		}

		rsaKey, err := rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			return nil, fmt.Errorf("generate rsa key: %w", err)
		}

		// Convert to OpenSSH format
		sshPub, err := ssh.NewPublicKey(&rsaKey.PublicKey)
		if err != nil {
			return nil, err
		}
		publicKey = ssh.MarshalAuthorizedKey(sshPub)
		if comment != "" {
			publicKey = append(publicKey[:len(publicKey)-1], []byte(" "+comment+"\n")...)
		}

		// Marshal private key
		privPEM, err := ssh.MarshalPrivateKey(rsaKey, comment)
		if err != nil {
			return nil, err
		}
		privateKey = pem.EncodeToMemory(privPEM)

	default:
		return nil, fmt.Errorf("unsupported key type: %s", keyType)
	}

	if err != nil {
		return nil, err
	}

	newSecret.Data["private"] = privateKey
	newSecret.Data["public"] = publicKey

	if config.TTL > 0 {
		newSecret.ExpiresAt = time.Now().Add(config.TTL)
	}

	return newSecret, nil
}

// Verify validates the SSH key pair.
func (r *SSHKeyRotator) Verify(ctx context.Context, secret *secrets.Secret) error {
	privatePEM, ok := secret.Data["private"]
	if !ok {
		return errors.New("private key missing")
	}

	publicKey, ok := secret.Data["public"]
	if !ok {
		return errors.New("public key missing")
	}

	// Parse private key
	privKey, err := ssh.ParsePrivateKey(privatePEM)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// Parse public key
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	// Verify keys match
	if string(privKey.PublicKey().Marshal()) != string(pubKey.Marshal()) {
		return errors.New("public key does not match private key")
	}

	return nil
}

// Rollback reverts to the previous key.
func (r *SSHKeyRotator) Rollback(ctx context.Context, current, previous *secrets.Secret) error {
	// SSH keys don't need external rollback
	return nil
}
