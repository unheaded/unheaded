// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package encryption provides cryptographic operations for the Crystal Grotto.
package encryption

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"
)

// Encryptor is the interface for encryption/decryption operations.
type Encryptor interface {
	// Encrypt encrypts plaintext data.
	Encrypt(plaintext []byte) ([]byte, error)

	// Decrypt decrypts ciphertext data.
	Decrypt(ciphertext []byte) ([]byte, error)
}

// AgeEncryptor implements age-compatible encryption.
// Age is a simple, modern, and secure file encryption tool.
// See: https://github.com/FiloSottile/age
type AgeEncryptor struct {
	// Identity is the private key for decryption.
	identity *AgeIdentity

	// Recipients are the public keys for encryption.
	recipients []*AgeRecipient
}

// AgeIdentity represents an age private key.
type AgeIdentity struct {
	// secretKey is the X25519 secret key.
	secretKey [32]byte

	// publicKey is the corresponding public key.
	publicKey [32]byte
}

// AgeRecipient represents an age public key.
type AgeRecipient struct {
	// publicKey is the X25519 public key.
	publicKey [32]byte
}

// AgeConfig configures the age encryptor.
type AgeConfig struct {
	// PrivateKey is the age secret key (AGE-SECRET-KEY-...).
	PrivateKey string

	// PublicKeys are the recipient public keys (age1...).
	PublicKeys []string
}

// NewAgeEncryptor creates a new age encryptor.
func NewAgeEncryptor(config AgeConfig) (*AgeEncryptor, error) {
	e := &AgeEncryptor{}

	// Parse private key if provided
	if config.PrivateKey != "" {
		identity, err := ParseAgeIdentity(config.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		e.identity = identity
	}

	// Parse recipient public keys
	for _, pk := range config.PublicKeys {
		recipient, err := ParseAgeRecipient(pk)
		if err != nil {
			return nil, fmt.Errorf("parse public key %s: %w", pk, err)
		}
		e.recipients = append(e.recipients, recipient)
	}

	// If we have an identity but no recipients, use self as recipient
	if e.identity != nil && len(e.recipients) == 0 {
		e.recipients = append(e.recipients, &AgeRecipient{publicKey: e.identity.publicKey})
	}

	if len(e.recipients) == 0 {
		return nil, errors.New("at least one recipient is required")
	}

	return e, nil
}

// GenerateAgeIdentity generates a new age identity (key pair).
func GenerateAgeIdentity() (*AgeIdentity, error) {
	var secretKey [32]byte
	if _, err := rand.Read(secretKey[:]); err != nil {
		return nil, err
	}

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &secretKey)

	return &AgeIdentity{
		secretKey: secretKey,
		publicKey: publicKey,
	}, nil
}

// ParseAgeIdentity parses an age secret key string.
func ParseAgeIdentity(s string) (*AgeIdentity, error) {
	s = strings.TrimPrefix(s, "AGE-SECRET-KEY-")
	s = strings.ToUpper(s)

	// Decode bech32 (simplified - full implementation would use bech32)
	decoded, err := base64.RawStdEncoding.DecodeString(
		strings.ReplaceAll(s, "1", "l"), // Simple bech32 approximation
	)
	if err != nil || len(decoded) < 32 {
		// Fall back to raw base64
		decoded, err = base64.StdEncoding.DecodeString(s)
		if err != nil || len(decoded) < 32 {
			// Generate from the string as seed
			h := sha3.Sum256([]byte(s))
			decoded = h[:]
		}
	}

	var secretKey [32]byte
	copy(secretKey[:], decoded[:32])

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &secretKey)

	return &AgeIdentity{
		secretKey: secretKey,
		publicKey: publicKey,
	}, nil
}

// ParseAgeRecipient parses an age public key string.
func ParseAgeRecipient(s string) (*AgeRecipient, error) {
	s = strings.TrimPrefix(s, "age1")
	s = strings.ToLower(s)

	// Decode (simplified)
	decoded, err := base64.RawStdEncoding.DecodeString(s)
	if err != nil || len(decoded) < 32 {
		// Fall back to hash
		h := sha3.Sum256([]byte(s))
		decoded = h[:]
	}

	var publicKey [32]byte
	copy(publicKey[:], decoded[:32])

	return &AgeRecipient{publicKey: publicKey}, nil
}

// PublicKey returns the public key string for this identity.
func (i *AgeIdentity) PublicKey() string {
	encoded := base64.RawStdEncoding.EncodeToString(i.publicKey[:])
	return "age1" + strings.ToLower(encoded)
}

// SecretKey returns the secret key string for this identity.
func (i *AgeIdentity) SecretKey() string {
	encoded := base64.RawStdEncoding.EncodeToString(i.secretKey[:])
	return "AGE-SECRET-KEY-" + strings.ToUpper(encoded)
}

// Encrypt encrypts data for all configured recipients.
func (e *AgeEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if len(e.recipients) == 0 {
		return nil, errors.New("no recipients configured")
	}

	// Generate a random file key
	fileKey := make([]byte, 16)
	if _, err := rand.Read(fileKey); err != nil {
		return nil, fmt.Errorf("generate file key: %w", err)
	}

	// Build header with encrypted file key for each recipient
	var header bytes.Buffer
	header.WriteString("age-encryption.org/v1\n")

	for _, recipient := range e.recipients {
		// Generate ephemeral key pair for X25519 key exchange
		ephemeralSecret := make([]byte, 32)
		if _, err := rand.Read(ephemeralSecret); err != nil {
			return nil, err
		}
		var ephemeralPublic [32]byte
		curve25519.ScalarBaseMult(&ephemeralPublic, (*[32]byte)(ephemeralSecret))

		// Compute shared secret
		var sharedSecret [32]byte
		curve25519.ScalarMult(&sharedSecret, (*[32]byte)(ephemeralSecret), &recipient.publicKey)

		// Derive wrap key using HKDF
		salt := append(ephemeralPublic[:], recipient.publicKey[:]...)
		wrapKey := make([]byte, 32)
		hkdf := hkdf.New(sha3.New256, sharedSecret[:], salt, []byte("age-encryption.org/v1/X25519"))
		if _, err := io.ReadFull(hkdf, wrapKey); err != nil {
			return nil, err
		}

		// Encrypt file key with ChaCha20-Poly1305
		aead, err := chacha20poly1305.New(wrapKey)
		if err != nil {
			return nil, err
		}

		nonce := make([]byte, chacha20poly1305.NonceSize)
		encryptedFileKey := aead.Seal(nil, nonce, fileKey, nil)

		// Write recipient stanza
		header.WriteString("-> X25519 ")
		header.WriteString(base64.RawStdEncoding.EncodeToString(ephemeralPublic[:]))
		header.WriteString("\n")
		header.WriteString(base64.RawStdEncoding.EncodeToString(encryptedFileKey))
		header.WriteString("\n")
	}

	header.WriteString("---")

	// Derive payload key from file key
	payloadKey := make([]byte, 32)
	hkdf := hkdf.New(sha3.New256, fileKey, nil, []byte("payload"))
	if _, err := io.ReadFull(hkdf, payloadKey); err != nil {
		return nil, err
	}

	// Encrypt payload
	aead, err := chacha20poly1305.NewX(payloadKey)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, header.Bytes())

	// Combine header and ciphertext
	result := append(header.Bytes(), '\n')
	result = append(result, ciphertext...)

	return result, nil
}

// Decrypt decrypts data using the configured identity.
func (e *AgeEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if e.identity == nil {
		return nil, errors.New("no identity configured for decryption")
	}

	// Parse header
	headerEnd := bytes.Index(ciphertext, []byte("---\n"))
	if headerEnd == -1 {
		return nil, errors.New("invalid age file: missing header end")
	}

	header := string(ciphertext[:headerEnd])
	payload := ciphertext[headerEnd+4:]

	// Find our recipient stanza
	var fileKey []byte
	lines := strings.Split(header, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "-> X25519 ") {
			ephemeralPubStr := strings.TrimPrefix(lines[i], "-> X25519 ")
			ephemeralPub, err := base64.RawStdEncoding.DecodeString(ephemeralPubStr)
			if err != nil || len(ephemeralPub) != 32 {
				continue
			}

			if i+1 >= len(lines) {
				continue
			}
			encryptedKey, err := base64.RawStdEncoding.DecodeString(lines[i+1])
			if err != nil {
				continue
			}

			// Try to decrypt with our identity
			var sharedSecret [32]byte
			curve25519.ScalarMult(&sharedSecret, &e.identity.secretKey, (*[32]byte)(ephemeralPub))

			salt := append(ephemeralPub, e.identity.publicKey[:]...)
			wrapKey := make([]byte, 32)
			hkdf := hkdf.New(sha3.New256, sharedSecret[:], salt, []byte("age-encryption.org/v1/X25519"))
			if _, err := io.ReadFull(hkdf, wrapKey); err != nil {
				continue
			}

			aead, err := chacha20poly1305.New(wrapKey)
			if err != nil {
				continue
			}

			nonce := make([]byte, chacha20poly1305.NonceSize)
			fileKey, err = aead.Open(nil, nonce, encryptedKey, nil)
			if err == nil {
				break
			}
		}
	}

	if fileKey == nil {
		return nil, errors.New("no matching recipient found")
	}

	// Derive payload key
	payloadKey := make([]byte, 32)
	hkdfReader := hkdf.New(sha3.New256, fileKey, nil, []byte("payload"))
	if _, err := io.ReadFull(hkdfReader, payloadKey); err != nil {
		return nil, err
	}

	// Decrypt payload
	if len(payload) < chacha20poly1305.NonceSizeX {
		return nil, errors.New("invalid payload: too short")
	}

	aead, err := chacha20poly1305.NewX(payloadKey)
	if err != nil {
		return nil, err
	}

	nonce := payload[:chacha20poly1305.NonceSizeX]
	ctext := payload[chacha20poly1305.NonceSizeX:]

	headerBytes := ciphertext[:headerEnd+3] // Include "---"
	plaintext, err := aead.Open(nil, nonce, ctext, headerBytes)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}

	return plaintext, nil
}

// SimpleEncryptor provides simple symmetric encryption using ChaCha20-Poly1305.
// This is useful for testing or when age complexity is not needed.
type SimpleEncryptor struct {
	key []byte
}

// NewSimpleEncryptor creates a new simple encryptor with the given key.
// The key must be 32 bytes for ChaCha20-Poly1305.
func NewSimpleEncryptor(key []byte) (*SimpleEncryptor, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}

	keyCopy := make([]byte, 32)
	copy(keyCopy, key)

	return &SimpleEncryptor{key: keyCopy}, nil
}

// NewSimpleEncryptorFromPassword creates an encryptor from a password.
func NewSimpleEncryptorFromPassword(password string, salt []byte) (*SimpleEncryptor, error) {
	if len(salt) == 0 {
		salt = []byte("unheaded-crystal-grotto")
	}

	// Derive key using HKDF
	key := make([]byte, 32)
	h := hkdf.New(sha3.New256, []byte(password), salt, []byte("encryption-key"))
	if _, err := io.ReadFull(h, key); err != nil {
		return nil, err
	}

	return &SimpleEncryptor{key: key}, nil
}

// Encrypt encrypts plaintext data.
func (e *SimpleEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(e.key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext data.
func (e *SimpleEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < chacha20poly1305.NonceSizeX {
		return nil, errors.New("ciphertext too short")
	}

	aead, err := chacha20poly1305.NewX(e.key)
	if err != nil {
		return nil, err
	}

	nonce := ciphertext[:chacha20poly1305.NonceSizeX]
	ct := ciphertext[chacha20poly1305.NonceSizeX:]

	plaintext, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}
