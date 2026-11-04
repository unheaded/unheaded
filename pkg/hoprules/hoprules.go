// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package hoprules is the writer of hop-ebpf's check maps (ADR-125): it reads
// a rules file and turns it into HOP_VALIDATORS, HMAC_KEYS and RETRY_TOKENS
// entries, byte for byte what pkg/protocol/bpfschema mirrors.
//
//	validators:
//	  - hop: 0                 # CONFIG key 0 on that hop
//	    namespace: 0           # the Monad's deploy_ring
//	    version_min: 1         # both 0: no version rule
//	    version_max: 1
//	    max_hop_count: 8       # 0: no cap
//	    require_crc: true
//	    require_hmac: true
//	    require_retry: false
//	hmac_keys:
//	  - flow: 0x2211           # src_service_id | dst_service_id << 8
//	    namespace: 0
//	    key_file: /etc/unheaded/hop/a.key
//	    ttl: 24h               # absent: never expires
//	retry:
//	  - flow: 0x2211
//	    source: fd00::1
//	    ttl: 30m
//
// Key bytes come only from key_file, never from the rules file, and appear in
// no error and no log. hop-ebpf reads a key's presence and expiry, not the
// key: this proves nothing about where a packet came from (ADR-125, FND-031).
//
// Writing rules does not turn them on: CONFIG key 4 does, and nothing here
// touches it.
package hoprules

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/protocol/bpfschema"
)

// KeySize is the length of an HMAC_KEYS key, and of a key_file.
const KeySize = 32

// maxRulesBytes caps a rules file: every map full is well under it.
const maxRulesBytes = 1 << 20

// retryFlag is HopValidatorValue.Flags bit 0 (VALIDATOR_FLAG_RETRY).
const retryFlag = 1

var (
	// ErrKeyFileMode is returned for a key_file not mode 0400 or 0600.
	ErrKeyFileMode = errors.New("hoprules: key_file mode")

	// ErrKeyFileSize is returned for a key_file not KeySize bytes long.
	ErrKeyFileSize = errors.New("hoprules: key_file size")
)

// File is one rules file. Parse fills it, Load reads its keys, Apply writes it.
type File struct {
	Validators []Validator `yaml:"validators"`
	HMACKeys   []HMACKey   `yaml:"hmac_keys"`
	Retry      []Retry     `yaml:"retry"`
}

// Validator is one HOP_VALIDATORS entry. allowed_dict_ids is absent on
// purpose: hop-ebpf does not evaluate it, so a file must not be able to ask.
type Validator struct {
	Hop          uint32 `yaml:"hop"`
	Namespace    uint8  `yaml:"namespace"`
	VersionMin   uint8  `yaml:"version_min"`
	VersionMax   uint8  `yaml:"version_max"`
	RequireCRC   bool   `yaml:"require_crc"`
	RequireHMAC  bool   `yaml:"require_hmac"`
	RequireRetry bool   `yaml:"require_retry"`
	MaxHopCount  uint8  `yaml:"max_hop_count"`
}

// HMACKey is one HMAC_KEYS entry. A TTL of zero never expires.
type HMACKey struct {
	Flow      uint16        `yaml:"flow"`
	Namespace uint8         `yaml:"namespace"`
	KeyFile   string        `yaml:"key_file"`
	TTL       time.Duration `yaml:"ttl"`

	// key is a closure because fmt walks unexported fields and follows a
	// pointer on a bad verb, so an array or a pointer to one can print. A
	// func prints as an address. Nil until Load.
	key func() [KeySize]byte
}

// Retry is one RETRY_TOKENS entry: Source was validated for Flow. A TTL of
// zero never expires. The map keys on the low 48 bits of Source alone.
type Retry struct {
	Flow   uint16        `yaml:"flow"`
	Source netip.Addr    `yaml:"source"`
	TTL    time.Duration `yaml:"ttl"`
}

// MapWriter writes one entry to the named map.
type MapWriter interface {
	Update(mapName string, key, value []byte) error
}

// Applied counts the entries written, including those written before a failure.
type Applied struct {
	Validators int
	HMACKeys   int
	Retry      int
}

// Parse reads a rules file strictly: an unknown field, a second document or an
// entry that could never do what it says is an error. It reads no key_file.
func Parse(r io.Reader) (*File, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxRulesBytes+1))
	if err != nil {
		return nil, fmt.Errorf("hoprules: read: %w", err)
	}
	if len(data) > maxRulesBytes {
		return nil, fmt.Errorf("hoprules: rules file is over %d bytes", maxRulesBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("hoprules: empty rules file")
		}
		return nil, fmt.Errorf("hoprules: %w", err)
	}
	var more yaml.Node
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return nil, errors.New("hoprules: want one YAML document")
	}
	if len(f.Validators)+len(f.HMACKeys)+len(f.Retry) == 0 {
		return nil, errors.New("hoprules: no entries")
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// validate refuses what would be written wrongly or lost. Two entries with
// one map key would leave the first silently overwritten. It runs in Apply as
// well, for a File built without Parse.
func (f *File) validate() error {
	validators := make(map[[2]uint32]bool, len(f.Validators))
	for i, v := range f.Validators {
		if v.VersionMin > v.VersionMax {
			return fmt.Errorf("hoprules: validators[%d]: version_min %d is over version_max %d", i, v.VersionMin, v.VersionMax)
		}
		k := [2]uint32{v.Hop, uint32(v.Namespace)}
		if validators[k] {
			return fmt.Errorf("hoprules: validators[%d]: same hop and namespace as an earlier entry", i)
		}
		validators[k] = true
	}
	keys := make(map[[2]uint16]bool, len(f.HMACKeys))
	for i := range f.HMACKeys {
		h := &f.HMACKeys[i]
		// The value is not repeated: what sits here may be a pasted key.
		if h.KeyFile == "" || !filepath.IsAbs(h.KeyFile) {
			return fmt.Errorf("hoprules: hmac_keys[%d]: key_file must be an absolute path", i)
		}
		if err := checkTTL(h.TTL); err != nil {
			return fmt.Errorf("hoprules: hmac_keys[%d]: %w", i, err)
		}
		k := [2]uint16{h.Flow, uint16(h.Namespace)}
		if keys[k] {
			return fmt.Errorf("hoprules: hmac_keys[%d]: same flow and namespace as an earlier entry", i)
		}
		keys[k] = true
	}
	retries := make(map[bpfschema.RetryTokenKey]bool, len(f.Retry))
	for i, r := range f.Retry {
		if !r.Source.Is6() {
			return fmt.Errorf("hoprules: retry[%d]: source must be an IPv6 address", i)
		}
		if r.Source.Zone() != "" {
			return fmt.Errorf("hoprules: retry[%d]: source must have no zone", i)
		}
		if err := checkTTL(r.TTL); err != nil {
			return fmt.Errorf("hoprules: retry[%d]: %w", i, err)
		}
		k := retryKey(r)
		if retries[k] {
			return fmt.Errorf("hoprules: retry[%d]: same flow and low 48 source bits as an earlier entry", i)
		}
		retries[k] = true
	}
	return nil
}

func checkTTL(ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("ttl is negative")
	}
	if ttl%time.Second != 0 {
		return errors.New("ttl must be whole seconds: expiry is kept in seconds")
	}
	return nil
}

// Load reads every entry's key_file. It stops at the first refusal.
func (f *File) Load() error {
	for i := range f.HMACKeys {
		key, err := readKey(f.HMACKeys[i].KeyFile)
		if err != nil {
			return fmt.Errorf("hoprules: hmac_keys[%d]: %w", i, err)
		}
		f.HMACKeys[i].key = key
	}
	return nil
}

// readKey reads a key_file. Lstat refuses a symlink, whose target's mode the
// open would otherwise vouch for; the mode and size are then read from the
// open file, so what is checked is what is read.
func readKey(path string) (func() [KeySize]byte, error) {
	link, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !link.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	fh, err := os.Open(path) // #nosec G304 -- the operator's key_file; refused below unless 0400 or 0600
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	st, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(link, st) {
		return nil, fmt.Errorf("%s changed while being opened", path)
	}
	if perm := st.Mode().Perm(); perm != 0o400 && perm != 0o600 {
		return nil, fmt.Errorf("%w: %s is %04o, want 0400 or 0600", ErrKeyFileMode, path, perm)
	}
	if st.Size() != KeySize {
		return nil, fmt.Errorf("%w: %s is %d bytes, want %d", ErrKeyFileSize, path, st.Size(), KeySize)
	}
	var key [KeySize]byte
	if _, err := io.ReadFull(fh, key[:]); err != nil {
		return nil, fmt.Errorf("%w: %s: short read", ErrKeyFileSize, path)
	}
	return func() [KeySize]byte { return key }, nil
}

// entry is one encoded map write, and the counter it adds to.
type entry struct {
	mapName    string
	key, value []byte
	count      *int
}

// Apply writes f through w. boottime is the current CLOCK_BOOTTIME: the maps
// hold times on that clock (ADR-125), so a TTL becomes created_at plus TTL
// here. Everything is encoded before the first write, so a refusal writes
// nothing. Keys and retry entries go first: in enforce mode a rule that lands
// before its key drops traffic. A map failure stops the walk, and the returned
// Applied says what had landed.
func Apply(w MapWriter, f *File, boottime func() time.Duration) (Applied, error) {
	var done Applied
	if err := f.validate(); err != nil {
		return done, err
	}
	if boottime == nil {
		return done, errors.New("hoprules: no boottime clock")
	}
	now, err := bootSeconds(boottime())
	if err != nil {
		return done, err
	}
	entries, err := encode(f, now, &done)
	if err != nil {
		return done, err
	}
	for _, e := range entries {
		if err := w.Update(e.mapName, e.key, e.value); err != nil {
			return done, fmt.Errorf("hoprules: %s: %w", e.mapName, err)
		}
		*e.count++
	}
	return done, nil
}

func bootSeconds(d time.Duration) (uint32, error) {
	s := int64(d / time.Second)
	if s < 0 || s > math.MaxUint32 {
		return 0, fmt.Errorf("hoprules: boottime %d s does not fit the maps' u32", s)
	}
	return uint32(s), nil // #nosec G115 -- range checked above
}

// expiry is now plus ttl, or 0 (never) for no ttl.
func expiry(now uint32, ttl time.Duration) (uint32, error) {
	if ttl == 0 {
		return 0, nil
	}
	s := int64(now) + int64(ttl/time.Second)
	if s > math.MaxUint32 {
		return 0, errors.New("ttl runs past the maps' u32 seconds")
	}
	return uint32(s), nil // #nosec G115 -- range checked above
}

func encode(f *File, now uint32, done *Applied) ([]entry, error) {
	var entries []entry
	add := func(mapName string, key, value any, count *int) error {
		k, err := binary.Append(nil, binary.LittleEndian, key)
		if err != nil {
			return fmt.Errorf("hoprules: %s key: %w", mapName, err)
		}
		v, err := binary.Append(nil, binary.LittleEndian, value)
		if err != nil {
			return fmt.Errorf("hoprules: %s value: %w", mapName, err)
		}
		entries = append(entries, entry{mapName, k, v, count})
		return nil
	}
	for i := range f.HMACKeys {
		h := &f.HMACKeys[i]
		if h.key == nil {
			return nil, fmt.Errorf("hoprules: hmac_keys[%d]: key not loaded", i)
		}
		expires, err := expiry(now, h.TTL)
		if err != nil {
			return nil, fmt.Errorf("hoprules: hmac_keys[%d]: %w", i, err)
		}
		key := bpfschema.HMACKeyKey{FlowID: h.Flow, Namespace: uint16(h.Namespace)}
		value := bpfschema.HMACKeyValue{Key: h.key(), CreatedAt: now, ExpiresAt: expires}
		if err := add(bpfschema.MapHMACKeys, key, value, &done.HMACKeys); err != nil {
			return nil, err
		}
	}
	for i, r := range f.Retry {
		expires, err := expiry(now, r.TTL)
		if err != nil {
			return nil, fmt.Errorf("hoprules: retry[%d]: %w", i, err)
		}
		value := bpfschema.RetryTokenValue{IssuedAt: now, ExpiresAt: expires}
		if err := add(bpfschema.MapRetryTokens, retryKey(r), value, &done.Retry); err != nil {
			return nil, err
		}
	}
	for _, v := range f.Validators {
		key := bpfschema.HopValidatorKey{HopID: v.Hop, Namespace: uint32(v.Namespace)}
		if err := add(bpfschema.MapHopValidators, key, validatorValue(v), &done.Validators); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func validatorValue(v Validator) bpfschema.HopValidatorValue {
	out := bpfschema.HopValidatorValue{
		AllowedVersionMin: v.VersionMin,
		AllowedVersionMax: v.VersionMax,
		MaxHopCount:       v.MaxHopCount,
	}
	if v.RequireCRC {
		out.RequireCRC = 1
	}
	if v.RequireHMAC {
		out.RequireHMAC = 1
	}
	if v.RequireRetry {
		out.Flags |= retryFlag
	}
	return out
}

// retryKey holds the source's last six bytes least significant first, as
// RetryTokenKey says: make_retry_key reads them big-endian into a u64.
func retryKey(r Retry) bpfschema.RetryTokenKey {
	a := r.Source.As16()
	k := bpfschema.RetryTokenKey{FlowID: r.Flow}
	for i := range k.SrcLow48 {
		k.SrcLow48[i] = a[15-i]
	}
	return k
}
