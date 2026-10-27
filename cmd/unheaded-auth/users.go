// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/crypto/argon2"
)

// argonParams are argon2id's cost parameters (RFC 9106). Memory is KiB.
type argonParams struct {
	Memory  uint32
	Time    uint32
	Threads uint8
	SaltLen int
	KeyLen  int
}

// defaultParams hash every new password: 64 MiB, 3 passes, 2 lanes, 16-byte
// salt, 32-byte key (S-IRON-GATE Step 129; above RFC 9106's second
// recommended option and OWASP's floor).
var defaultParams = argonParams{Memory: 64 * 1024, Time: 3, Threads: 2, SaltLen: 16, KeyLen: 32}

// Bounds on parameters read from a users file, so a hostile or corrupt file
// cannot make one login allocate gigabytes or spin for minutes.
// minArgonMemoryKiB is a variable so tests can hash cheaply.
var minArgonMemoryKiB uint32 = 19 * 1024

const (
	maxArgonMemoryKiB = 256 * 1024
	maxArgonTime      = 10
	maxArgonThreads   = 16
	minSaltLen        = 16
	maxSaltLen        = 64
	minKeyLen         = 16
	maxKeyLen         = 64

	// maxUsersFileBytes caps what a users file read may allocate.
	maxUsersFileBytes = 1 << 20
	// maxUsers caps how many users one file may hold.
	maxUsers = 1000
	// maxUsernameLen and maxPasswordLen bound login input.
	maxUsernameLen = 64
	maxPasswordLen = 1024
)

// allowedRoles are the roles a user may hold (pkg/auth RBAC names plus the
// ADR-103 "viewer"). "service" is not one: it is what service tokens carry,
// and a person holding it would pass service-only policies.
var allowedRoles = map[string]bool{
	"admin": true, "operator": true, "viewer": true, "observer": true, "guest": true,
}

var errBadHash = errors.New("not an argon2id hash this issuer accepts")

// hashPassword returns the PHC string
// $argon2id$v=19$m=<KiB>,t=<n>,p=<n>$<salt>$<key> (unpadded base64).
func hashPassword(password string, p argonParams) (string, error) {
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(p.KeyLen)) // #nosec G115 -- KeyLen bounded by parseHash / defaultParams
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// parsedHash is a decoded PHC string.
type parsedHash struct {
	p    argonParams
	salt []byte
	key  []byte
}

// parseHash decodes and bounds-checks a PHC argon2id string.
func parseHash(s string) (parsedHash, error) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return parsedHash{}, errBadHash
	}
	var ph parsedHash
	fields := strings.Split(parts[3], ",")
	if len(fields) != 3 {
		return parsedHash{}, errBadHash
	}
	num := func(f, prefix string, lo, hi uint64) (uint64, error) {
		v, ok := strings.CutPrefix(f, prefix)
		if !ok {
			return 0, errBadHash
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || n < lo || n > hi {
			return 0, errBadHash
		}
		return n, nil
	}
	m, err := num(fields[0], "m=", uint64(minArgonMemoryKiB), maxArgonMemoryKiB)
	if err != nil {
		return parsedHash{}, err
	}
	t, err := num(fields[1], "t=", 1, maxArgonTime)
	if err != nil {
		return parsedHash{}, err
	}
	th, err := num(fields[2], "p=", 1, maxArgonThreads)
	if err != nil {
		return parsedHash{}, err
	}
	ph.p = argonParams{Memory: uint32(m), Time: uint32(t), Threads: uint8(th)} // #nosec G115 -- bounded above
	if ph.salt, err = base64.RawStdEncoding.Strict().DecodeString(parts[4]); err != nil || len(ph.salt) < minSaltLen || len(ph.salt) > maxSaltLen {
		return parsedHash{}, errBadHash
	}
	if ph.key, err = base64.RawStdEncoding.Strict().DecodeString(parts[5]); err != nil || len(ph.key) < minKeyLen || len(ph.key) > maxKeyLen {
		return parsedHash{}, errBadHash
	}
	ph.p.SaltLen, ph.p.KeyLen = len(ph.salt), len(ph.key)
	return ph, nil
}

// verify reports whether password matches, comparing in constant time.
func (ph parsedHash) verify(password string) bool {
	got := argon2.IDKey([]byte(password), ph.salt, ph.p.Time, ph.p.Memory, ph.p.Threads, uint32(len(ph.key))) // #nosec G115 -- key length bounded by parseHash
	return subtle.ConstantTimeCompare(got, ph.key) == 1
}

// userRecord is one user in the users file.
type userRecord struct {
	Username string   `json:"username"`
	Hash     string   `json:"argon2id_hash"`
	Roles    []string `json:"roles"`
}

// usersFile is the users file: {"users":[...]}, 0600, owned by the
// issuer's account.
type usersFile struct {
	Users []userRecord `json:"users"`
}

// user is a validated, parsed user.
type user struct {
	name  string
	hash  parsedHash
	roles []string
}

func validUsername(s string) bool {
	if s == "" || len(s) > maxUsernameLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case (c == '-' || c == '_' || c == '.' || c == '@') && i > 0:
		default:
			return false
		}
	}
	return true
}

// validateRoles checks roles against allowedRoles and returns them sorted
// and de-duplicated. At least one role is required.
func validateRoles(roles []string) ([]string, error) {
	if len(roles) == 0 {
		return nil, errors.New("at least one role is required")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		if !allowedRoles[r] {
			return nil, fmt.Errorf("role %q not allowed (admin, operator, viewer, observer, guest)", r)
		}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out, nil
}

// parseUsers decodes a users file strictly: one JSON object, no unknown
// fields, valid unique usernames, accepted hashes, allowed roles.
func parseUsers(raw []byte) (map[string]user, error) {
	if len(raw) > maxUsersFileBytes {
		return nil, fmt.Errorf("users file larger than %d bytes", maxUsersFileBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var uf usersFile
	if err := dec.Decode(&uf); err != nil {
		return nil, fmt.Errorf("users file: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("users file: data after the JSON object")
	}
	if len(uf.Users) > maxUsers {
		return nil, fmt.Errorf("users file: more than %d users", maxUsers)
	}
	out := make(map[string]user, len(uf.Users))
	for i, u := range uf.Users {
		if !validUsername(u.Username) {
			return nil, fmt.Errorf("users file: user %d: bad username", i)
		}
		if _, dup := out[u.Username]; dup {
			return nil, fmt.Errorf("users file: user %q listed twice", u.Username)
		}
		ph, err := parseHash(u.Hash)
		if err != nil {
			return nil, fmt.Errorf("users file: user %q: %w", u.Username, err)
		}
		roles, err := validateRoles(u.Roles)
		if err != nil {
			return nil, fmt.Errorf("users file: user %q: %w", u.Username, err)
		}
		out[u.Username] = user{name: u.Username, hash: ph, roles: roles}
	}
	return out, nil
}

// readPrivateFile reads a regular file that group and others cannot
// access (0600 or 0400), capped at limit bytes.
func readPrivateFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-configured path
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is %04o; it must not be accessible to group or others (chmod 0600)", path, st.Mode().Perm())
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, limit)
	}
	return raw, nil
}

// loadUsers reads and parses the users file, refusing one that group or
// others can read.
func loadUsers(path string) (map[string]user, error) {
	raw, err := readPrivateFile(path, maxUsersFileBytes)
	if err != nil {
		return nil, err
	}
	return parseUsers(raw)
}

// readUsersFile returns the raw records for editing (user add/del/list).
// A missing file is an empty list.
func readUsersFile(path string) (usersFile, error) {
	raw, err := readPrivateFile(path, maxUsersFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return usersFile{}, nil
	}
	if err != nil {
		return usersFile{}, err
	}
	if _, err := parseUsers(raw); err != nil {
		return usersFile{}, err
	}
	var uf usersFile
	if err := json.Unmarshal(raw, &uf); err != nil {
		return usersFile{}, err
	}
	return uf, nil
}

// writeUsersFile replaces path atomically (temp file in the same
// directory, 0600, fsync, rename). An existing file's owner is kept, so
// root editing the issuer's file leaves it readable by the issuer.
func writeUsersFile(path string, uf usersFile) error {
	raw, err := json.MarshalIndent(uf, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if _, err := parseUsers(raw); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".users-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if st, err := os.Stat(path); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 {
			if err := tmp.Chown(int(sys.Uid), int(sys.Gid)); err != nil {
				_ = tmp.Close()
				return err
			}
		}
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
