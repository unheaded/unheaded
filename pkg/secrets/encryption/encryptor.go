// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package encryption provides cryptographic operations for the Crystal Grotto.
package encryption

import (
	"crypto/rand"
	"crypto/sha3"
	"errors"
	"fmt"
	"hash"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// Encryptor is the interface for encryption/decryption operations.
type Encryptor interface {
	// Encrypt encrypts plaintext data.
	Encrypt(plaintext []byte) ([]byte, error)

	// Decrypt decrypts ciphertext data.
	Decrypt(ciphertext []byte) ([]byte, error)
}

// SimpleEncryptor provides simple symmetric encryption using XChaCha20-Poly1305.
// Its only callers are the tests of the secrets manager and the file store.
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
	h := hkdf.New(newSHA3256, []byte(password), salt, []byte("encryption-key"))
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

// newSHA3256 adapts the standard library's SHA3-256 (which returns
// *sha3.SHA3) to the func() hash.Hash HKDF takes. Same digest as the
// golang.org/x/crypto/sha3 constructor it replaces.
func newSHA3256() hash.Hash { return sha3.New256() }
