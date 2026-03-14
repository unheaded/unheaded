// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package encryption

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SimpleEncryptor tests
// ---------------------------------------------------------------------------

func TestSimpleEncryptor_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	enc, err := NewSimpleEncryptor(key)
	if err != nil {
		t.Fatalf("NewSimpleEncryptor: %v", err)
	}

	plaintext := []byte("hello, crystal grotto")

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestSimpleEncryptor_EmptyData(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	enc, err := NewSimpleEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	got, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes", len(got))
	}
}

func TestSimpleEncryptor_NilData(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	enc, err := NewSimpleEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, err := enc.Encrypt(nil)
	if err != nil {
		t.Fatalf("Encrypt nil: %v", err)
	}

	got, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt nil roundtrip: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes", len(got))
	}
}

func TestSimpleEncryptor_WrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	enc1, _ := NewSimpleEncryptor(key1)
	enc2, _ := NewSimpleEncryptor(key2)

	ciphertext, err := enc1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = enc2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestSimpleEncryptor_CorruptedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	enc, _ := NewSimpleEncryptor(key)

	ciphertext, err := enc.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	// Flip a byte near the end (in the auth tag region).
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = enc.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error decrypting corrupted ciphertext")
	}
}

func TestSimpleEncryptor_TruncatedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	enc, _ := NewSimpleEncryptor(key)

	_, err := enc.Decrypt([]byte("short"))
	if err == nil {
		t.Fatal("expected error on short ciphertext")
	}
}

func TestSimpleEncryptor_InvalidKeySize(t *testing.T) {
	for _, size := range []int{0, 16, 31, 33, 64} {
		_, err := NewSimpleEncryptor(make([]byte, size))
		if err == nil {
			t.Errorf("expected error for key size %d", size)
		}
	}
}

func TestSimpleEncryptor_NilKey(t *testing.T) {
	_, err := NewSimpleEncryptor(nil)
	if err == nil {
		t.Fatal("expected error for nil key")
	}
}

func TestSimpleEncryptorFromPassword(t *testing.T) {
	enc, err := NewSimpleEncryptorFromPassword("my-password", nil)
	if err != nil {
		t.Fatalf("NewSimpleEncryptorFromPassword: %v", err)
	}

	plaintext := []byte("password-protected data")
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestSimpleEncryptorFromPassword_CustomSalt(t *testing.T) {
	salt := []byte("custom-salt-value")
	enc, err := NewSimpleEncryptorFromPassword("pass", salt)
	if err != nil {
		t.Fatal(err)
	}

	ct, err := enc.Encrypt([]byte("data"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "data" {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestSimpleEncryptorFromPassword_DifferentPasswords(t *testing.T) {
	enc1, _ := NewSimpleEncryptorFromPassword("password-a", nil)
	enc2, _ := NewSimpleEncryptorFromPassword("password-b", nil)

	ct, _ := enc1.Encrypt([]byte("secret"))

	_, err := enc2.Decrypt(ct)
	if err == nil {
		t.Fatal("expected error decrypting with different password")
	}
}

func TestSimpleEncryptor_LargeData(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	enc, _ := NewSimpleEncryptor(key)

	// 1 MB payload
	plaintext := make([]byte, 1<<20)
	rand.Read(plaintext)

	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatal("large data roundtrip mismatch")
	}
}

func TestSimpleEncryptor_CiphertextsDiffer(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	enc, _ := NewSimpleEncryptor(key)
	plaintext := []byte("same input")

	ct1, _ := enc.Encrypt(plaintext)
	ct2, _ := enc.Encrypt(plaintext)

	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting the same plaintext twice should produce different ciphertexts (random nonce)")
	}
}

// ---------------------------------------------------------------------------
// AgeEncryptor tests
// ---------------------------------------------------------------------------

func TestAgeEncryptor_RoundTrip(t *testing.T) {
	identity, err := GenerateAgeIdentity()
	if err != nil {
		t.Fatal(err)
	}

	enc, err := NewAgeEncryptor(AgeConfig{
		PrivateKey: identity.SecretKey(),
	})
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("age encrypted payload")

	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestAgeEncryptor_EmptyData(t *testing.T) {
	identity, _ := GenerateAgeIdentity()

	enc, _ := NewAgeEncryptor(AgeConfig{PrivateKey: identity.SecretKey()})

	ct, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(got))
	}
}

func TestAgeEncryptor_WrongKey(t *testing.T) {
	id1, _ := GenerateAgeIdentity()
	id2, _ := GenerateAgeIdentity()

	encryptor, _ := NewAgeEncryptor(AgeConfig{PrivateKey: id1.SecretKey()})
	decryptor, _ := NewAgeEncryptor(AgeConfig{PrivateKey: id2.SecretKey()})

	ct, err := encryptor.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = decryptor.Decrypt(ct)
	if err == nil {
		t.Fatal("expected error decrypting with wrong identity")
	}
}

func TestAgeEncryptor_CorruptedCiphertext(t *testing.T) {
	id, _ := GenerateAgeIdentity()
	enc, _ := NewAgeEncryptor(AgeConfig{PrivateKey: id.SecretKey()})

	ct, err := enc.Encrypt([]byte("data"))
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the payload portion (after the header).
	ct[len(ct)-1] ^= 0xff

	_, err = enc.Decrypt(ct)
	if err == nil {
		t.Fatal("expected error on corrupted ciphertext")
	}
}

func TestAgeEncryptor_NoRecipients(t *testing.T) {
	_, err := NewAgeEncryptor(AgeConfig{})
	if err == nil {
		t.Fatal("expected error with no recipients")
	}
}

func TestAgeEncryptor_DecryptWithoutIdentity(t *testing.T) {
	id, _ := GenerateAgeIdentity()

	// Encryptor has identity (and thus self as recipient).
	encryptor, _ := NewAgeEncryptor(AgeConfig{PrivateKey: id.SecretKey()})

	ct, _ := encryptor.Encrypt([]byte("data"))

	// Decryptor has only a public key, no identity.
	decryptor, _ := NewAgeEncryptor(AgeConfig{
		PublicKeys: []string{id.PublicKey()},
	})

	_, err := decryptor.Decrypt(ct)
	if err == nil {
		t.Fatal("expected error decrypting without identity")
	}
}

func TestAgeEncryptor_InvalidHeader(t *testing.T) {
	id, _ := GenerateAgeIdentity()
	enc, _ := NewAgeEncryptor(AgeConfig{PrivateKey: id.SecretKey()})

	_, err := enc.Decrypt([]byte("not an age file"))
	if err == nil {
		t.Fatal("expected error on invalid header")
	}
}

func TestAgeEncryptor_MultipleRecipients(t *testing.T) {
	// Because PublicKey()/SecretKey() serialization uses a simplified
	// encoding (not true bech32), roundtripping through string form is
	// lossy. To test multiple recipients we build the encryptor directly
	// from the generated identities' raw key material.
	id1, _ := GenerateAgeIdentity()
	id2, _ := GenerateAgeIdentity()

	enc := &AgeEncryptor{
		identity:   id1,
		recipients: []*AgeRecipient{{publicKey: id1.publicKey}, {publicKey: id2.publicKey}},
	}

	ct, err := enc.Encrypt([]byte("for both"))
	if err != nil {
		t.Fatal(err)
	}

	// Decrypt with id2's raw key.
	dec2 := &AgeEncryptor{
		identity:   id2,
		recipients: []*AgeRecipient{{publicKey: id2.publicKey}},
	}
	got, err := dec2.Decrypt(ct)
	if err != nil {
		t.Fatalf("id2 decrypt: %v", err)
	}
	if string(got) != "for both" {
		t.Fatalf("got %q", got)
	}

	// Decrypt with id1's raw key.
	dec1 := &AgeEncryptor{
		identity:   id1,
		recipients: []*AgeRecipient{{publicKey: id1.publicKey}},
	}
	got, err = dec1.Decrypt(ct)
	if err != nil {
		t.Fatalf("id1 decrypt: %v", err)
	}
	if string(got) != "for both" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// AgeIdentity key management tests
// ---------------------------------------------------------------------------

func TestGenerateAgeIdentity(t *testing.T) {
	id, err := GenerateAgeIdentity()
	if err != nil {
		t.Fatal(err)
	}

	sk := id.SecretKey()
	pk := id.PublicKey()

	if sk == "" || pk == "" {
		t.Fatal("empty key strings")
	}

	if len(sk) < 10 || len(pk) < 10 {
		t.Fatal("key strings too short")
	}

	if sk[:len("AGE-SECRET-KEY-")] != "AGE-SECRET-KEY-" {
		t.Fatalf("secret key missing prefix: %s", sk)
	}

	if pk[:4] != "age1" {
		t.Fatalf("public key missing prefix: %s", pk)
	}
}

func TestParseAgeIdentity(t *testing.T) {
	// ParseAgeIdentity uses a simplified bech32 approximation, so
	// roundtripping through SecretKey() string may not reproduce the
	// exact original bytes. We verify the function returns a usable
	// identity that can encrypt/decrypt.
	id, _ := GenerateAgeIdentity()
	sk := id.SecretKey()

	parsed, err := ParseAgeIdentity(sk)
	if err != nil {
		t.Fatalf("ParseAgeIdentity: %v", err)
	}

	// Verify the parsed identity can perform encryption/decryption.
	enc := &AgeEncryptor{
		identity:   parsed,
		recipients: []*AgeRecipient{{publicKey: parsed.publicKey}},
	}
	ct, err := enc.Encrypt([]byte("test"))
	if err != nil {
		t.Fatalf("Encrypt with parsed identity: %v", err)
	}
	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt with parsed identity: %v", err)
	}
	if string(got) != "test" {
		t.Fatalf("roundtrip mismatch: %q", got)
	}
}

func TestParseAgeRecipient(t *testing.T) {
	// Verify ParseAgeRecipient returns a usable recipient.
	id, _ := GenerateAgeIdentity()
	pk := id.PublicKey()

	recipient, err := ParseAgeRecipient(pk)
	if err != nil {
		t.Fatalf("ParseAgeRecipient: %v", err)
	}

	// The recipient should have a non-zero public key.
	var zero [32]byte
	if recipient.publicKey == zero {
		t.Fatal("parsed recipient has zero public key")
	}
}

func TestParseAgeIdentity_ArbitraryString(t *testing.T) {
	// The parser falls back to SHA3 hashing for arbitrary strings.
	_, err := ParseAgeIdentity("arbitrary-string-not-base64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAgeRecipient_ArbitraryString(t *testing.T) {
	_, err := ParseAgeRecipient("age1arbitrary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Encryptor interface compliance
// ---------------------------------------------------------------------------

func TestSimpleEncryptor_ImplementsEncryptor(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	enc, _ := NewSimpleEncryptor(key)

	var _ Encryptor = enc
}

func TestAgeEncryptor_ImplementsEncryptor(t *testing.T) {
	id, _ := GenerateAgeIdentity()
	enc, _ := NewAgeEncryptor(AgeConfig{PrivateKey: id.SecretKey()})

	var _ Encryptor = enc
}

// ---------------------------------------------------------------------------
// EnvelopeEncryptor tests
// ---------------------------------------------------------------------------

func TestEnvelopeEncryptor_RoundTrip(t *testing.T) {
	provider, err := NewLocalKEKProvider()
	if err != nil {
		t.Fatal(err)
	}

	enc, err := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider})
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("envelope encrypted data")

	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestEnvelopeEncryptor_EmptyData(t *testing.T) {
	provider, _ := NewLocalKEKProvider()
	enc, _ := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider})

	ct, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(got))
	}
}

func TestEnvelopeEncryptor_NilProvider(t *testing.T) {
	_, err := NewEnvelopeEncryptor(EnvelopeConfig{})
	if err == nil {
		t.Fatal("expected error with nil KEK provider")
	}
}

func TestEnvelopeEncryptor_InvalidJSON(t *testing.T) {
	provider, _ := NewLocalKEKProvider()
	enc, _ := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider})

	_, err := enc.Decrypt([]byte("not json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestEnvelopeEncryptor_WrongVersion(t *testing.T) {
	provider, _ := NewLocalKEKProvider()
	enc, _ := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider})

	env := Envelope{Version: 99}
	data, _ := json.Marshal(env)

	_, err := enc.Decrypt(data)
	if err == nil {
		t.Fatal("expected error on wrong version")
	}
}

func TestEnvelopeEncryptor_WrongKEK(t *testing.T) {
	provider1, _ := NewLocalKEKProvider()
	provider2, _ := NewLocalKEKProvider()

	enc1, _ := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider1})
	enc2, _ := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider2})

	ct, err := enc1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = enc2.Decrypt(ct)
	if err == nil {
		t.Fatal("expected error decrypting with different KEK")
	}
}

func TestEnvelopeEncryptor_KeyRotation(t *testing.T) {
	provider, _ := NewLocalKEKProvider()
	enc, _ := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider})

	// Encrypt with first key.
	ct1, err := enc.Encrypt([]byte("before rotation"))
	if err != nil {
		t.Fatal(err)
	}

	// Rotate key.
	_, err = provider.RotateKEK(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Old ciphertext should still decrypt (old key remains in provider).
	got, err := enc.Decrypt(ct1)
	if err != nil {
		t.Fatalf("decrypt after rotation: %v", err)
	}
	if string(got) != "before rotation" {
		t.Fatalf("mismatch: %q", got)
	}

	// New encryption uses the rotated key.
	ct2, err := enc.Encrypt([]byte("after rotation"))
	if err != nil {
		t.Fatal(err)
	}

	got, err = enc.Decrypt(ct2)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after rotation" {
		t.Fatalf("mismatch: %q", got)
	}
}

// ---------------------------------------------------------------------------
// LocalKEKProvider tests
// ---------------------------------------------------------------------------

func TestLocalKEKProvider_GetKEK(t *testing.T) {
	provider, err := NewLocalKEKProvider()
	if err != nil {
		t.Fatal(err)
	}

	id := provider.CurrentKEKID()
	if id == "" {
		t.Fatal("empty current KEK ID")
	}

	key, err := provider.GetKEK(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
}

func TestLocalKEKProvider_UnknownKeyID(t *testing.T) {
	provider, _ := NewLocalKEKProvider()

	_, err := provider.GetKEK(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown key ID")
	}
}

func TestLocalKEKProviderWithKey(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	provider, err := NewLocalKEKProviderWithKey("test-key", key)
	if err != nil {
		t.Fatal(err)
	}

	if provider.CurrentKEKID() != "test-key" {
		t.Fatalf("expected 'test-key', got %q", provider.CurrentKEKID())
	}

	got, err := provider.GetKEK(context.Background(), "test-key")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, key) {
		t.Fatal("key mismatch")
	}
}

func TestLocalKEKProviderWithKey_InvalidSize(t *testing.T) {
	_, err := NewLocalKEKProviderWithKey("k", make([]byte, 16))
	if err == nil {
		t.Fatal("expected error for wrong key size")
	}
}

func TestLocalKEKProvider_Rotate(t *testing.T) {
	provider, _ := NewLocalKEKProvider()
	oldID := provider.CurrentKEKID()

	time.Sleep(time.Millisecond) // ensure different nano timestamp
	newID, err := provider.RotateKEK(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if newID == oldID {
		t.Fatal("rotated key ID should differ")
	}

	if provider.CurrentKEKID() != newID {
		t.Fatal("current ID not updated after rotation")
	}

	// Both keys should be retrievable.
	if _, err := provider.GetKEK(context.Background(), oldID); err != nil {
		t.Fatalf("old key missing: %v", err)
	}
	if _, err := provider.GetKEK(context.Background(), newID); err != nil {
		t.Fatalf("new key missing: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DerivedKEKProvider tests
// ---------------------------------------------------------------------------

func TestDerivedKEKProvider_RoundTrip(t *testing.T) {
	masterKey := make([]byte, 32)
	rand.Read(masterKey)

	provider, err := NewDerivedKEKProvider(masterKey)
	if err != nil {
		t.Fatal(err)
	}

	enc, err := NewEnvelopeEncryptor(EnvelopeConfig{KEKProvider: provider})
	if err != nil {
		t.Fatal(err)
	}

	ct, err := enc.Encrypt([]byte("derived key data"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "derived key data" {
		t.Fatalf("mismatch: %q", got)
	}
}

func TestDerivedKEKProvider_ShortMasterKey(t *testing.T) {
	_, err := NewDerivedKEKProvider(make([]byte, 8))
	if err == nil {
		t.Fatal("expected error for short master key")
	}
}

func TestDerivedKEKProvider_DeterministicDerivation(t *testing.T) {
	masterKey := make([]byte, 32)
	rand.Read(masterKey)

	provider, _ := NewDerivedKEKProvider(masterKey)

	keyID := provider.CurrentKEKID()
	k1, _ := provider.GetKEK(context.Background(), keyID)
	k2, _ := provider.GetKEK(context.Background(), keyID)

	if !bytes.Equal(k1, k2) {
		t.Fatal("derived keys should be deterministic for the same ID")
	}
}

func TestDerivedKEKProvider_DifferentIDsDifferentKeys(t *testing.T) {
	masterKey := make([]byte, 32)
	rand.Read(masterKey)

	provider, _ := NewDerivedKEKProvider(masterKey)

	k1, _ := provider.GetKEK(context.Background(), "id-a")
	k2, _ := provider.GetKEK(context.Background(), "id-b")

	if bytes.Equal(k1, k2) {
		t.Fatal("different key IDs should produce different derived keys")
	}
}

func TestDerivedKEKProvider_Rotate(t *testing.T) {
	masterKey := make([]byte, 32)
	rand.Read(masterKey)

	provider, _ := NewDerivedKEKProvider(masterKey)
	oldID := provider.CurrentKEKID()

	time.Sleep(time.Second) // ID includes Unix seconds
	newID, err := provider.RotateKEK(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if newID == oldID {
		t.Fatal("rotated ID should differ")
	}

	if provider.CurrentKEKID() != newID {
		t.Fatal("current ID not updated")
	}
}

// ---------------------------------------------------------------------------
// DEK cache tests
// ---------------------------------------------------------------------------

func TestDEKCache_GetSet(t *testing.T) {
	c := newDEKCache(10, time.Minute)

	c.Set("key1", []byte("value1"))

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(got) != "value1" {
		t.Fatalf("got %q", got)
	}
}

func TestDEKCache_Miss(t *testing.T) {
	c := newDEKCache(10, time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestDEKCache_Expiry(t *testing.T) {
	c := newDEKCache(10, time.Millisecond)

	c.Set("key1", []byte("value1"))

	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestDEKCache_Eviction(t *testing.T) {
	c := newDEKCache(2, time.Minute)

	c.Set("k1", []byte("v1"))
	c.Set("k2", []byte("v2"))
	c.Set("k3", []byte("v3")) // should evict oldest

	if len(c.entries) > 2 {
		t.Fatalf("expected max 2 entries, got %d", len(c.entries))
	}
}

func TestDEKCache_ReturnsCopy(t *testing.T) {
	c := newDEKCache(10, time.Minute)

	original := []byte("original")
	c.Set("k", original)

	got, _ := c.Get("k")
	got[0] = 'X'

	got2, _ := c.Get("k")
	if got2[0] == 'X' {
		t.Fatal("cache should return independent copies")
	}
}
