// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// API keys for automation (ADR-103 §5): `uhk_` + 32 random bytes in
// base64url, so a leaked key is greppable (gitleaks rule unheaded-api-key).
// A key is shown once, at creation; a store keeps only its SHA-256, the
// first 12 characters (to name it in lists and revocations), owner, role
// and dates. Every key expires.
const (
	// APIKeyPrefix starts every key.
	APIKeyPrefix = "uhk_"
	// APIKeyDisplayPrefixLen is how much of a key a store keeps in clear:
	// "uhk_" and 8 characters (48 bits), enough to tell keys apart in a list.
	APIKeyDisplayPrefixLen = 12
	// DefaultAPIKeyLifetime is the expiry unheaded-cli gives a key unless
	// told otherwise.
	DefaultAPIKeyLifetime = 90 * 24 * time.Hour
	// MaxAPIKeyLifetime caps how far ahead a key may expire.
	MaxAPIKeyLifetime = 365 * 24 * time.Hour
	// DefaultAPIKeyFile is the file store for hosts without the Well.
	DefaultAPIKeyFile = "/etc/unheaded/auth/apikeys.jsonl" // #nosec G101 -- a file path (the store holds hashes), not a credential

	apiKeyBodyLen = 43 // base64url of 32 bytes, unpadded
	// touchEvery bounds last_used_at writes to one per key per minute.
	touchEvery = time.Minute
)

// API key errors. Authentication failures wrap ErrUnauthenticated too, so
// the middleware answers 401.
var (
	ErrAPIKeyExpired          = errors.New("auth: API key expired")
	ErrAPIKeyRevoked          = errors.New("auth: API key revoked")
	ErrAPIKeyNotFound         = errors.New("auth: no such API key")
	ErrAPIKeyAmbiguous        = errors.New("auth: API key prefix matches more than one key; revoke by id")
	ErrAPIKeyInvalid          = errors.New("auth: invalid API key request")
	ErrAPIKeyStorePermissions = errors.New("auth: API key store is group- or world-writable, or world-readable")
)

// APIKeyRoles are the roles a key can carry: the RBAC roles a caller can
// hold (rbac.go). There is no "guest" key.
var APIKeyRoles = []string{RoleAdmin, RoleOperator, RoleObserver, RoleService}

var apiKeyOwnerRE = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,128}$`)

// APIKeyRecord is what a store holds for one key: never the key itself.
type APIKeyRecord struct {
	ID         string     `json:"id"`
	KeyHash    string     `json:"key_hash"` // hex SHA-256 of the whole key
	Prefix     string     `json:"prefix"`   // first APIKeyDisplayPrefixLen characters
	Owner      string     `json:"owner"`
	Role       string     `json:"role"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// APIKeyStore creates, checks, revokes and lists API keys. Implementations:
// NewMemAPIKeyStore (tests), NewFileAPIKeyStore (hosts without the Well),
// NewWellAPIKeyStore (unheaded_app.api_keys, migration 036). Pick one from
// a spec with OpenAPIKeyStore.
type APIKeyStore interface {
	// Create makes a key for owner with role, expiring at expiresAt, and
	// returns it with its record. The key is not recoverable afterwards.
	Create(ctx context.Context, owner, role string, expiresAt time.Time) (key string, rec APIKeyRecord, err error)
	// Authenticate returns the identity a key stands for, or an error
	// wrapping ErrUnauthenticated (unknown, malformed, expired, revoked).
	Authenticate(ctx context.Context, key string) (*Identity, error)
	// Revoke revokes the key whose prefix or id is prefixOrID. Revoking a
	// revoked key changes nothing.
	Revoke(ctx context.Context, prefixOrID string) (APIKeyRecord, error)
	// List returns every record, oldest first.
	List(ctx context.Context) ([]APIKeyRecord, error)
}

// APIKeyStoreOption configures a store.
type APIKeyStoreOption func(*apiKeyStore)

// WithAPIKeyClock sets the clock a store checks expiry with (tests).
func WithAPIKeyClock(now func() time.Time) APIKeyStoreOption {
	return func(s *apiKeyStore) { s.now = now }
}

// NewAPIKey returns a fresh uhk_ key.
func NewAPIKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("auth: generate API key: %w", err)
	}
	return APIKeyPrefix + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// IsAPIKey reports whether s has the shape of a uhk_ key.
func IsAPIKey(s string) bool {
	if len(s) != len(APIKeyPrefix)+apiKeyBodyLen || !strings.HasPrefix(s, APIKeyPrefix) {
		return false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(s[len(APIKeyPrefix):])
	return err == nil && len(raw) == 32
}

// APIKeyHash is the hex SHA-256 a store keeps for key.
func APIKeyHash(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func validAPIKeyRole(role string) bool {
	for _, r := range APIKeyRoles {
		if r == role {
			return true
		}
	}
	return false
}

// validate checks a record read back from a store, so a hand-edited or
// corrupted one fails closed.
func (r APIKeyRecord) validate() error {
	if r.ID == "" || len(r.KeyHash) != 64 || !isLowerHex(r.KeyHash) ||
		len(r.Prefix) != APIKeyDisplayPrefixLen || !strings.HasPrefix(r.Prefix, APIKeyPrefix) ||
		!apiKeyOwnerRE.MatchString(r.Owner) || !validAPIKeyRole(r.Role) || r.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: malformed record %q", ErrAPIKeyInvalid, r.ID)
	}
	return nil
}

func isLowerHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// newUUID returns a random (version 4) UUID.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}

// apiKeyBackend is where records live. apiKeyStore holds the rules
// (format, roles, expiry, revocation) so every backend applies the same.
type apiKeyBackend interface {
	insert(ctx context.Context, rec APIKeyRecord) error
	byHash(ctx context.Context, hash string) (APIKeyRecord, bool, error)
	byPrefixOrID(ctx context.Context, s string) ([]APIKeyRecord, error)
	all(ctx context.Context) ([]APIKeyRecord, error)
	// revoke sets revoked_at if it is unset and returns the record as stored.
	revoke(ctx context.Context, id string, at time.Time) (APIKeyRecord, error)
	// touch records a use. A backend may skip it (the file store does:
	// rewriting the file per request is not worth a timestamp).
	touch(ctx context.Context, id string, at time.Time) error
}

type apiKeyStore struct {
	b   apiKeyBackend
	now func() time.Time
}

func newAPIKeyStore(b apiKeyBackend, opts []APIKeyStoreOption) *apiKeyStore {
	s := &apiKeyStore{b: b, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *apiKeyStore) Create(ctx context.Context, owner, role string, expiresAt time.Time) (string, APIKeyRecord, error) {
	now := s.now().UTC()
	switch {
	case !apiKeyOwnerRE.MatchString(owner):
		return "", APIKeyRecord{}, fmt.Errorf("%w: owner %q must be 1-128 of [A-Za-z0-9._@-]", ErrAPIKeyInvalid, owner)
	case !validAPIKeyRole(role):
		return "", APIKeyRecord{}, fmt.Errorf("%w: role %q (want one of %s)", ErrAPIKeyInvalid, role, strings.Join(APIKeyRoles, ", "))
	case !expiresAt.After(now):
		return "", APIKeyRecord{}, fmt.Errorf("%w: expiry %s is not in the future", ErrAPIKeyInvalid, expiresAt.Format(time.RFC3339))
	case expiresAt.Sub(now) > MaxAPIKeyLifetime:
		return "", APIKeyRecord{}, fmt.Errorf("%w: expiry more than %s ahead", ErrAPIKeyInvalid, MaxAPIKeyLifetime)
	}
	// A prefix collision (48 bits) is near impossible; try again rather
	// than have two keys answer to one name.
	for attempt := 0; attempt < 3; attempt++ {
		key, err := NewAPIKey()
		if err != nil {
			return "", APIKeyRecord{}, err
		}
		prefix := key[:APIKeyDisplayPrefixLen]
		clash, err := s.b.byPrefixOrID(ctx, prefix)
		if err != nil {
			return "", APIKeyRecord{}, err
		}
		if len(clash) > 0 {
			continue
		}
		id, err := newUUID()
		if err != nil {
			return "", APIKeyRecord{}, err
		}
		rec := APIKeyRecord{
			ID:        id,
			KeyHash:   APIKeyHash(key),
			Prefix:    prefix,
			Owner:     owner,
			Role:      role,
			CreatedAt: now,
			ExpiresAt: expiresAt.UTC(),
		}
		if err := s.b.insert(ctx, rec); err != nil {
			return "", APIKeyRecord{}, err
		}
		return key, rec, nil
	}
	return "", APIKeyRecord{}, errors.New("auth: could not draw an API key with an unused prefix")
}

func (s *apiKeyStore) Authenticate(ctx context.Context, key string) (*Identity, error) {
	if !IsAPIKey(key) {
		return nil, fmt.Errorf("%w: not an API key", ErrUnauthenticated)
	}
	rec, ok, err := s.b.byHash(ctx, APIKeyHash(key))
	if err != nil {
		// A store that cannot answer refuses (fail closed).
		return nil, fmt.Errorf("%w: API key store: %w", ErrUnauthenticated, err)
	}
	if !ok {
		return nil, fmt.Errorf("%w: unknown API key", ErrUnauthenticated)
	}
	now := s.now()
	if rec.RevokedAt != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthenticated, ErrAPIKeyRevoked)
	}
	if !now.Before(rec.ExpiresAt) {
		return nil, fmt.Errorf("%w: %w", ErrUnauthenticated, ErrAPIKeyExpired)
	}
	if rec.LastUsedAt == nil || now.Sub(*rec.LastUsedAt) >= touchEvery {
		// Best effort: a failed timestamp write does not refuse the caller.
		_ = s.b.touch(ctx, rec.ID, now.UTC())
	}
	return &Identity{
		Subject: rec.Owner,
		Roles:   []string{rec.Role},
		Extra: map[string]string{
			"auth_method":   "apikey",
			"apikey_id":     rec.ID,
			"apikey_prefix": rec.Prefix,
		},
	}, nil
}

func (s *apiKeyStore) Revoke(ctx context.Context, prefixOrID string) (APIKeyRecord, error) {
	if prefixOrID == "" {
		return APIKeyRecord{}, ErrAPIKeyNotFound
	}
	recs, err := s.b.byPrefixOrID(ctx, prefixOrID)
	if err != nil {
		return APIKeyRecord{}, err
	}
	switch len(recs) {
	case 0:
		return APIKeyRecord{}, fmt.Errorf("%w: %q", ErrAPIKeyNotFound, prefixOrID)
	case 1:
	default:
		return APIKeyRecord{}, fmt.Errorf("%w: %q", ErrAPIKeyAmbiguous, prefixOrID)
	}
	if recs[0].RevokedAt != nil {
		return recs[0], nil
	}
	return s.b.revoke(ctx, recs[0].ID, s.now().UTC())
}

func (s *apiKeyStore) List(ctx context.Context) ([]APIKeyRecord, error) {
	return s.b.all(ctx)
}

// ---- memory ----

// NewMemAPIKeyStore returns a store that lives in memory (tests, and the
// CLI's tests).
func NewMemAPIKeyStore(opts ...APIKeyStoreOption) APIKeyStore {
	return newAPIKeyStore(&memAPIKeys{}, opts)
}

type memAPIKeys struct {
	mu   sync.Mutex
	recs []APIKeyRecord
}

func (m *memAPIKeys) insert(_ context.Context, rec APIKeyRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.recs {
		if r.KeyHash == rec.KeyHash || r.ID == rec.ID {
			return fmt.Errorf("%w: duplicate key", ErrAPIKeyInvalid)
		}
	}
	m.recs = append(m.recs, rec)
	return nil
}

func (m *memAPIKeys) byHash(_ context.Context, hash string) (APIKeyRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return findByHash(m.recs, hash)
}

func (m *memAPIKeys) byPrefixOrID(_ context.Context, s string) ([]APIKeyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return findByPrefixOrID(m.recs, s), nil
}

func (m *memAPIKeys) all(_ context.Context) ([]APIKeyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return copyRecords(m.recs), nil
}

func (m *memAPIKeys) revoke(_ context.Context, id string, at time.Time) (APIKeyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return revokeIn(m.recs, id, at)
}

func (m *memAPIKeys) touch(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.recs {
		if m.recs[i].ID == id {
			t := at
			m.recs[i].LastUsedAt = &t
		}
	}
	return nil
}

// Helpers shared by the memory and file backends.

func findByHash(recs []APIKeyRecord, hash string) (APIKeyRecord, bool, error) {
	for _, r := range recs {
		if r.KeyHash == hash {
			return copyRecord(r), true, nil
		}
	}
	return APIKeyRecord{}, false, nil
}

func findByPrefixOrID(recs []APIKeyRecord, s string) []APIKeyRecord {
	var out []APIKeyRecord
	for _, r := range recs {
		if r.Prefix == s || r.ID == s {
			out = append(out, copyRecord(r))
		}
	}
	return out
}

func revokeIn(recs []APIKeyRecord, id string, at time.Time) (APIKeyRecord, error) {
	for i := range recs {
		if recs[i].ID != id {
			continue
		}
		if recs[i].RevokedAt == nil {
			t := at
			recs[i].RevokedAt = &t
		}
		return copyRecord(recs[i]), nil
	}
	return APIKeyRecord{}, fmt.Errorf("%w: id %q", ErrAPIKeyNotFound, id)
}

func copyRecord(r APIKeyRecord) APIKeyRecord {
	if r.RevokedAt != nil {
		t := *r.RevokedAt
		r.RevokedAt = &t
	}
	if r.LastUsedAt != nil {
		t := *r.LastUsedAt
		r.LastUsedAt = &t
	}
	return r
}

func copyRecords(recs []APIKeyRecord) []APIKeyRecord {
	out := make([]APIKeyRecord, 0, len(recs))
	for _, r := range recs {
		out = append(out, copyRecord(r))
	}
	return out
}

// ---- choosing a store ----

// ParseAPIKeyStoreSpec splits an AUTH_APIKEY_STORE value: "well", or
// "file:<absolute path>".
func ParseAPIKeyStoreSpec(spec string) (kind, path string, err error) {
	switch {
	case spec == "well":
		return "well", "", nil
	case strings.HasPrefix(spec, "file:"):
		p := strings.TrimPrefix(spec, "file:")
		if !strings.HasPrefix(p, "/") {
			return "", "", fmt.Errorf("auth: AUTH_APIKEY_STORE %q: the file path must be absolute", spec)
		}
		return "file", p, nil
	}
	return "", "", fmt.Errorf("auth: AUTH_APIKEY_STORE %q: want \"well\" or \"file:<absolute path>\"", spec)
}

// DefaultAPIKeyStoreSpec is the store an operator tool uses:
// AUTH_APIKEY_STORE if set, else the Well when WELL_DB or
// AUTH_WELL_PASSWORD is set, else DefaultAPIKeyFile. Services do not
// default; they use a store only when AUTH_APIKEY_STORE names one.
func DefaultAPIKeyStoreSpec() string {
	if v := os.Getenv("AUTH_APIKEY_STORE"); v != "" {
		return v
	}
	if os.Getenv("WELL_DB") != "" || os.Getenv("AUTH_WELL_PASSWORD") != "" {
		return "well"
	}
	return "file:" + DefaultAPIKeyFile
}

// OpenAPIKeyStore opens the store a spec names (ParseAPIKeyStoreSpec).
func OpenAPIKeyStore(ctx context.Context, spec string, opts ...APIKeyStoreOption) (APIKeyStore, error) {
	kind, path, err := ParseAPIKeyStoreSpec(spec)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "file":
		return NewFileAPIKeyStore(path, opts...)
	default:
		return openWellAPIKeyStore(ctx, opts...)
	}
}
