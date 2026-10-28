// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Service key files (ADR-103 §2). Each service holds one Ed25519 private key
// as a file, /etc/unheaded/keys/<service>.key (0400, its own uid); the public
// half goes in a trust directory, /etc/unheaded/trust.d/<service>.pub (0444),
// which every verifier reads. Keys are generated at install, never stored in
// the repo, never passed in the environment.

const (
	// seedPEMType is the PEM block type of a private key file: the raw
	// 32-byte RFC 8032 seed. It is our own type so no tool mistakes the file
	// for a PKCS#8 key it may print or convert.
	seedPEMType = "UNHEADED ED25519 SEED"
	// pubPEMType is the PEM block type of a public key file: PKIX DER, the
	// form openssl reads (`openssl pkey -pubin -in x.pub -text`).
	pubPEMType = "PUBLIC KEY"

	// maxKeyFileBytes bounds what a key or .pub file read may allocate.
	maxKeyFileBytes = 4096
	// maxKeyNameLen bounds a service name used as a file name.
	maxKeyNameLen = 64
	// maxTrustFiles bounds how many .pub files one trust directory may hold.
	maxTrustFiles = 1024
)

// Errors returned by the key-file functions.
var (
	// ErrKeyPermissions: a private key file is readable or writable by
	// group or others. Fix with chmod 0400 (owner: the service's uid).
	ErrKeyPermissions = errors.New("auth: private key file is accessible to group or others (want 0400 or 0600)")
	// ErrBadKeyName: a service name that is not usable as a key file name.
	ErrBadKeyName = errors.New("auth: bad key name (want [a-z0-9][a-z0-9_-]*, at most 64 bytes)")
)

// ValidKeyName reports whether name may name a key: lower-case letters,
// digits, '-' and '_', starting with a letter or digit. No dots, so that
// "<name>.<anything>.pub" in a trust directory always binds to <name>.
func ValidKeyName(name string) bool {
	if name == "" || len(name) > maxKeyNameLen {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case (c == '-' || c == '_') && i > 0:
		default:
			return false
		}
	}
	return true
}

// KeyID returns the key id of an Ed25519 public key: the first 16 hex
// characters of SHA-256 over the 32 raw key bytes.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])[:16]
}

// GenerateKeyPair creates a new Ed25519 key pair for service name in dir:
// <name>.key (0400, the 32-byte seed) and <name>.pub (0444, PKIX). It never
// overwrites an existing file. It returns the key id.
func GenerateKeyPair(dir, name string) (string, error) {
	if !ValidKeyName(name) {
		return "", fmt.Errorf("%w: %q", ErrBadKeyName, name)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("auth: generate ed25519 key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("auth: marshal public key: %w", err)
	}

	keyPath := filepath.Join(dir, name+".key")
	pubPath := filepath.Join(dir, name+".pub")
	seedPEM := pem.EncodeToMemory(&pem.Block{Type: seedPEMType, Bytes: priv.Seed()})
	if err := writeNewFile(keyPath, seedPEM, 0o400); err != nil {
		return "", err
	}
	if err := writeNewFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: pubPEMType, Bytes: pubDER}), 0o444); err != nil {
		_ = os.Remove(keyPath)
		return "", err
	}
	return KeyID(pub), nil
}

// writeNewFile creates path exclusively with exactly mode (umask aside,
// fixed with an explicit chmod), writes data and syncs it.
func writeNewFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- operator-chosen key directory, name validated
	if err != nil {
		return fmt.Errorf("auth: create %s: %w", path, err)
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("auth: chmod %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("auth: write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("auth: sync %s: %w", path, err)
	}
	return f.Close()
}

// LoadPrivateKey reads a service's private key file. It refuses a file that
// group or others can read or write (ErrKeyPermissions), anything but a
// regular file, and anything but exactly one seed block.
func LoadPrivateKey(path string) (ed25519.PrivateKey, string, error) {
	f, err := os.Open(path) // #nosec G304 -- key path from service configuration, not from a request
	if err != nil {
		return nil, "", fmt.Errorf("auth: open private key: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only
	// Stat the open file, not the path, so the check is about what is read.
	st, err := f.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("auth: stat private key: %w", err)
	}
	if !st.Mode().IsRegular() {
		return nil, "", fmt.Errorf("auth: private key %s is not a regular file", path)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, "", fmt.Errorf("%w: %s is %04o", ErrKeyPermissions, path, st.Mode().Perm())
	}
	raw, err := readCapped(f)
	if err != nil {
		return nil, "", fmt.Errorf("auth: read private key %s: %w", path, err)
	}
	blk, rest := pem.Decode(raw)
	if blk == nil || blk.Type != seedPEMType || len(blk.Bytes) != ed25519.SeedSize || len(blk.Headers) != 0 {
		return nil, "", fmt.Errorf("auth: %s is not an %s PEM file", path, seedPEMType)
	}
	if len(strings.TrimSpace(string(rest))) != 0 {
		return nil, "", fmt.Errorf("auth: %s has data after the key block", path)
	}
	priv := ed25519.NewKeyFromSeed(blk.Bytes)
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, "", errors.New("auth: private key is not Ed25519")
	}
	return priv, KeyID(pub), nil
}

// ParsePublicKeyPEM parses one PKIX "PUBLIC KEY" PEM block holding an
// Ed25519 key, with nothing after it.
func ParsePublicKeyPEM(raw []byte) (ed25519.PublicKey, error) {
	blk, rest := pem.Decode(raw)
	if blk == nil || blk.Type != pubPEMType || len(blk.Headers) != 0 {
		return nil, errors.New("auth: not a PUBLIC KEY PEM block")
	}
	if len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("auth: data after the public key block")
	}
	k, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("auth: parse public key: %w", err)
	}
	pub, ok := k.(ed25519.PublicKey)
	if !ok || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("auth: public key is not Ed25519")
	}
	return pub, nil
}

func readCapped(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxKeyFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxKeyFileBytes {
		return nil, fmt.Errorf("larger than %d bytes", maxKeyFileBytes)
	}
	return b, nil
}

// trustedKey is one public key from a trust directory and the service name
// its file binds it to.
type trustedKey struct {
	pub  ed25519.PublicKey
	name string
}

// TrustDir is the set of service public keys a verifier accepts, read from
// a directory of "<name>.pub" files (rotation adds "<name>.<anything>.pub";
// the name is everything before the first dot). Removing a file and calling
// Reload revokes that key. Safe for concurrent use.
type TrustDir struct {
	path string
	mu   sync.RWMutex
	keys map[string]trustedKey // kid -> key
}

// NewTrustDir loads the trust directory at path. It fails if the directory
// cannot be read or holds anything Reload refuses.
func NewTrustDir(path string) (*TrustDir, error) {
	td := &TrustDir{path: path}
	if err := td.Reload(); err != nil {
		return nil, err
	}
	return td, nil
}

// Path returns the directory the set is read from.
func (t *TrustDir) Path() string { return t.path }

// Reload re-reads the directory. It refuses (and keeps the previous set) if
// the directory holds a .key file (a private key where only public keys
// belong), a .pub file that is not one Ed25519 public key, a .pub whose name
// is not a valid key name, or one key under two names. Files with other
// suffixes and dot-files are ignored.
func (t *TrustDir) Reload() error {
	entries, err := os.ReadDir(t.path)
	if err != nil {
		return fmt.Errorf("auth: read trust dir: %w", err)
	}
	keys := make(map[string]trustedKey)
	n := 0
	for _, e := range entries {
		fn := e.Name()
		if strings.HasPrefix(fn, ".") {
			continue // dot-files, and Kubernetes' ..data symlink farm
		}
		if strings.HasSuffix(fn, ".key") {
			return fmt.Errorf("auth: trust dir %s holds a private key file %q; only .pub files belong there", t.path, fn)
		}
		if !strings.HasSuffix(fn, ".pub") {
			continue
		}
		if n++; n > maxTrustFiles {
			return fmt.Errorf("auth: trust dir %s holds more than %d .pub files", t.path, maxTrustFiles)
		}
		name, _, _ := strings.Cut(fn, ".")
		if !ValidKeyName(name) {
			return fmt.Errorf("auth: trust dir file %q: %w", fn, ErrBadKeyName)
		}
		pub, err := readPubFile(filepath.Join(t.path, fn))
		if err != nil {
			return fmt.Errorf("auth: trust dir file %q: %w", fn, err)
		}
		kid := KeyID(pub)
		if prev, dup := keys[kid]; dup && prev.name != name {
			return fmt.Errorf("auth: trust dir %s: key %s is filed under both %q and %q", t.path, kid, prev.name, name)
		}
		keys[kid] = trustedKey{pub: pub, name: name}
	}
	t.mu.Lock()
	t.keys = keys
	t.mu.Unlock()
	return nil
}

// readPubFile reads one .pub file, following a symlink (Docker and
// Kubernetes mount secrets and config maps that way) to a regular file.
func readPubFile(path string) (ed25519.PublicKey, error) {
	f, err := os.Open(path) // #nosec G304 -- trust directory from service configuration
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	raw, err := readCapped(f)
	if err != nil {
		return nil, err
	}
	return ParsePublicKeyPEM(raw)
}

// Lookup returns the public key with key id kid.
func (t *TrustDir) Lookup(kid string) (ed25519.PublicKey, bool) {
	pub, _, ok := t.LookupName(kid)
	return pub, ok
}

// LookupName returns the public key with key id kid and the service name its
// file binds it to. A verifier must check a token's subject against that
// name: the key proves who signed, the name says who that is.
func (t *TrustDir) LookupName(kid string) (ed25519.PublicKey, string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	k, ok := t.keys[kid]
	if !ok {
		return nil, "", false
	}
	return k.pub, k.name, true
}

// Kids returns the trusted key ids, sorted.
func (t *TrustDir) Kids() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]string, 0, len(t.keys))
	for kid := range t.keys {
		out = append(out, kid)
	}
	sort.Strings(out)
	return out
}
