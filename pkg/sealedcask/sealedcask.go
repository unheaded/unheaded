// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package sealedcask signs, verifies and extracts Sealed Casks (ADR-010):
// the gzip tarball scripts/build-sealed-cask.sh writes, one top-level
// sealed-cask-<stamp>/ directory holding the payload, BINDING_RUNE.sha256
// (a `sha256sum` manifest of every regular file) and BINDING_RUNE.sig.
//
// The Binding Rune is signed, not just hashed (S-IRON-GATE Phase 13,
// FND-018): BINDING_RUNE.sig is a gungnir evidence seal (ML-DSA-65, FIPS
// 204 context unheaded/gungnir/evidence/v1, purpose sealed-cask-v1, JSON
// with the signing key's id) over the manifest's exact bytes, made with the
// build key. A cask verifies only if the seal is a trusted build key's, and
// the cask's regular files are exactly the manifest's, byte for byte.
//
// A cask arrives from outside, so it is hostile until verified. The reader
// streams (nothing is held but hashes, the manifest and the seal), admits
// only directories and regular files with clean relative names under the
// one root, refuses every link and device, trips size and entry caps from
// the tar headers before reading data, and refuses trailing bytes after the
// archive. Extract verifies the whole cask first and writes nothing until
// that passes; it then writes through an os.Root into a new directory and
// re-checks every hash as it writes.
package sealedcask

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
)

const (
	// ManifestName is the Binding Rune: `sha256sum` lines for every file.
	ManifestName = "BINDING_RUNE.sha256"
	// SigName is the build key's evidence seal over ManifestName.
	SigName = "BINDING_RUNE.sig"
	// Purpose is the evidence purpose a cask seal is issued for. Gungnir
	// purposes are lowercase tokens, so the plan's "sealed-cask/v1" is
	// spelled sealed-cask-v1.
	Purpose = "sealed-cask-v1"
)

// Limits bounds what one cask may cost to read.
type Limits struct {
	MaxMember   int64 // bytes per regular file
	MaxTotal    int64 // bytes across all regular files
	MaxEntries  int   // tar entries of any type
	MaxManifest int64 // bytes of BINDING_RUNE.sha256
}

// DefaultLimits allows 2 GiB per member, 8 GiB in all, 200,000 entries, a 16 MiB
// manifest (about 160,000 lines).
var DefaultLimits = Limits{MaxMember: 2 << 30, MaxTotal: 8 << 30, MaxEntries: 200000, MaxManifest: 16 << 20}

// maxPadding bounds the zero bytes allowed after the tar end-of-archive
// marker (tar pads to its record size, 10 KiB by default).
const maxPadding = 1 << 20

var (
	// ErrFormat: not a gzip tar, truncated, or bytes after the archive.
	ErrFormat = errors.New("sealedcask: not a well-formed gzip tar cask")
	// ErrEntry: a tar member that is unsafe or not allowed in a cask.
	ErrEntry = errors.New("sealedcask: cask member refused")
	// ErrTooLarge: a size or entry cap tripped.
	ErrTooLarge = errors.New("sealedcask: cask exceeds a size cap")
	// ErrManifest: BINDING_RUNE.sha256 is missing or malformed.
	ErrManifest = errors.New("sealedcask: malformed Binding Rune")
	// ErrUnsigned: no BINDING_RUNE.sig.
	ErrUnsigned = errors.New("UNSIGNED CASK: " + SigName + " is missing (build with UNHEADED_BUILD_KEY set)")
	// ErrSignature: the seal is malformed, for another purpose, or not a
	// trusted build key's signature over this manifest.
	ErrSignature = errors.New("sealedcask: Binding Rune signature does not verify")
	// ErrMismatch: the cask's files differ from the manifest (changed,
	// missing or extra).
	ErrMismatch = errors.New("sealedcask: cask contents do not match the Binding Rune")
)

// rootRe is the one top-level directory build-sealed-cask.sh writes.
var rootRe = regexp.MustCompile(`^sealed-cask-[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// manifestLine is one sha256sum line: lowercase hex, two spaces (or space
// and '*' for binary mode), a name starting "./".
var manifestLine = regexp.MustCompile(`^([0-9a-f]{64}) [ *](\./.+)$`)

// Manifest is a parsed Binding Rune: path below the root (no "./") to digest.
type Manifest struct {
	Files map[string][sha256.Size]byte
}

// ParseManifest parses BINDING_RUNE.sha256 strictly: every line LF-ended,
// clean "./" paths, no duplicates, no entry for the rune or its seal, no file
// that is also another file's parent directory.
func ParseManifest(b []byte) (*Manifest, error) {
	bad := func(format string, a ...any) (*Manifest, error) {
		return nil, fmt.Errorf("%w: "+format, append([]any{ErrManifest}, a...)...)
	}
	if len(b) == 0 {
		return bad("empty")
	}
	if b[len(b)-1] != '\n' {
		return bad("last line has no newline")
	}
	m := &Manifest{Files: map[string][sha256.Size]byte{}}
	for i, line := range strings.Split(string(b[:len(b)-1]), "\n") {
		sub := manifestLine.FindStringSubmatch(line)
		if sub == nil {
			return bad("line %d is not a `sha256sum` line", i+1)
		}
		name := strings.TrimPrefix(sub[2], "./")
		if err := checkRelName(name); err != nil {
			return bad("line %d: %v", i+1, err)
		}
		if name == ManifestName || name == SigName {
			return bad("line %d lists %s", i+1, name)
		}
		if _, dup := m.Files[name]; dup {
			return bad("line %d: %s listed twice", i+1, name)
		}
		var sum [sha256.Size]byte
		if _, err := hex.Decode(sum[:], []byte(sub[1])); err != nil {
			return bad("line %d: %v", i+1, err)
		}
		m.Files[name] = sum
	}
	for name := range m.Files {
		for d := path.Dir(name); d != "."; d = path.Dir(d) {
			if _, ok := m.Files[d]; ok {
				return bad("%s is both a file and the parent of %s", d, name)
			}
		}
	}
	return m, nil
}

// checkRelName accepts a clean, relative, slash-separated name with no ".."
// component, no backslash, NUL or control character.
func checkRelName(name string) error {
	switch {
	case name == "" || name == ".":
		return errors.New("empty name")
	case strings.ContainsAny(name, "\\\x00"):
		return errors.New("backslash or NUL in name")
	case strings.IndexFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0:
		return errors.New("control character in name")
	case path.IsAbs(name):
		return errors.New("absolute path")
	}
	for _, c := range strings.Split(name, "/") {
		if c == ".." {
			return errors.New("'..' component")
		}
	}
	if path.Clean(name) != name {
		return errors.New("not a clean path")
	}
	return nil
}

// Sign seals a Binding Rune with the build key. It refuses to sign a
// manifest that does not parse. The result is JSON plus a newline.
func Sign(manifest []byte, signer *gungnir.Signer) ([]byte, error) {
	if _, err := ParseManifest(manifest); err != nil {
		return nil, err
	}
	seal, err := signer.SignEvidence(manifest, Purpose)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(seal)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Result describes a verified cask.
type Result struct {
	Root       string // the sealed-cask-* directory
	Files      int    // regular files the manifest lists
	KeyID      string // the build key that signed it
	RuneSha256 string // sha256 of BINDING_RUNE.sha256, hex
}

// fileSink receives each payload file's bytes while it is hashed; nil for
// verify-only.
type fileSink func(name string, mode int64) (io.WriteCloser, error)

type scanned struct {
	root     string
	hashes   map[string][sha256.Size]byte
	manifest []byte
	sig      []byte
}

// scan reads the whole cask, enforcing every member rule and cap. It does
// not judge the contents against the manifest; check does.
func scan(r io.Reader, lim Limits, sink fileSink) (*scanned, error) {
	br := bufio.NewReader(r)
	if magic, err := br.Peek(2); err != nil || magic[0] != 0x1f || magic[1] != 0x8b {
		return nil, fmt.Errorf("%w: no gzip header", ErrFormat)
	}
	gz, err := gzip.NewReader(br)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	defer func() { _ = gz.Close() }() // a reader: nothing to flush
	tr := tar.NewReader(gz)
	s := &scanned{hashes: map[string][sha256.Size]byte{}}
	seen := map[string]bool{}
	var total int64
	entries := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrFormat, err)
		}
		entries++
		if entries > lim.MaxEntries {
			return nil, fmt.Errorf("%w: more than %d entries", ErrTooLarge, lim.MaxEntries)
		}
		rel, isRoot, err := s.memberName(hdr)
		if err != nil {
			return nil, err
		}
		key := rel
		if isRoot {
			key = "."
		}
		if seen[key] {
			return nil, fmt.Errorf("%w: %q appears twice", ErrEntry, hdr.Name)
		}
		seen[key] = true
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		case tar.TypeSymlink, tar.TypeLink:
			return nil, fmt.Errorf("%w: %q is a link to %q (a cask holds only signed regular files)", ErrEntry, hdr.Name, hdr.Linkname)
		default:
			return nil, fmt.Errorf("%w: %q is not a regular file or directory (type %q)", ErrEntry, hdr.Name, hdr.Typeflag)
		}
		if isRoot {
			return nil, fmt.Errorf("%w: %q: the root is a file", ErrEntry, hdr.Name)
		}
		if hdr.Size < 0 || hdr.Size > lim.MaxMember {
			return nil, fmt.Errorf("%w: %q is %d bytes, per-member cap %d", ErrTooLarge, hdr.Name, hdr.Size, lim.MaxMember)
		}
		if total+hdr.Size > lim.MaxTotal {
			return nil, fmt.Errorf("%w: total exceeds %d bytes at %q", ErrTooLarge, lim.MaxTotal, hdr.Name)
		}
		total += hdr.Size
		if err := s.member(tr, hdr, rel, lim, sink); err != nil {
			return nil, err
		}
	}
	if s.root == "" {
		return nil, fmt.Errorf("%w: empty archive", ErrFormat)
	}
	// After the end-of-archive marker only zero padding may follow, then the
	// end of the gzip stream, then nothing.
	n, err := io.Copy(zeroWriter{}, io.LimitReader(gz, maxPadding+1))
	if err != nil {
		return nil, fmt.Errorf("%w: trailing data after the archive: %v", ErrFormat, err)
	}
	if n > maxPadding {
		return nil, fmt.Errorf("%w: more than %d bytes after the archive", ErrFormat, maxPadding)
	}
	return s, nil
}

// memberName checks a tar name and returns its path below the root.
func (s *scanned) memberName(hdr *tar.Header) (rel string, isRoot bool, err error) {
	raw := hdr.Name
	name := raw
	if hdr.Typeflag == tar.TypeDir {
		name = strings.TrimSuffix(name, "/")
	}
	if err := checkRelName(name); err != nil {
		return "", false, fmt.Errorf("%w: %q: %v", ErrEntry, raw, err)
	}
	top, rest, _ := strings.Cut(name, "/")
	if !rootRe.MatchString(top) {
		return "", false, fmt.Errorf("%w: %q is not under a sealed-cask-* directory", ErrEntry, raw)
	}
	if s.root == "" {
		s.root = top
	} else if top != s.root {
		return "", false, fmt.Errorf("%w: %q is outside the cask root %s", ErrEntry, raw, s.root)
	}
	return rest, rest == "", nil
}

// member reads one regular file: the rune and seal into memory (capped),
// every other file through SHA-256 and, when extracting, to the sink.
func (s *scanned) member(tr io.Reader, hdr *tar.Header, rel string, lim Limits, sink fileSink) error {
	switch rel {
	case ManifestName, SigName:
		limit := lim.MaxManifest
		if rel == SigName {
			limit = gungnir.MaxEvidenceSealBytes
		}
		if hdr.Size > limit {
			return fmt.Errorf("%w: %s is %d bytes, cap %d", ErrTooLarge, rel, hdr.Size, limit)
		}
		b, err := readExact(tr, hdr)
		if err != nil {
			return err
		}
		if rel == ManifestName {
			s.manifest = b
		} else {
			s.sig = b
		}
		if sink != nil {
			return writeTo(sink, rel, hdr.Mode, bytes.NewReader(b))
		}
		return nil
	}
	h := sha256.New()
	var dst io.Writer = h
	var wc io.WriteCloser
	if sink != nil {
		var err error
		if wc, err = sink(rel, hdr.Mode); err != nil {
			return err
		}
		dst = io.MultiWriter(h, wc)
	}
	n, err := io.Copy(dst, io.LimitReader(tr, hdr.Size))
	if wc != nil {
		if cerr := wc.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		return fmt.Errorf("%w: %q: %v", ErrFormat, hdr.Name, err)
	}
	if n != hdr.Size {
		return fmt.Errorf("%w: %q: read %d bytes, header says %d", ErrFormat, hdr.Name, n, hdr.Size)
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	s.hashes[rel] = sum
	return nil
}

func readExact(r io.Reader, hdr *tar.Header) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, hdr.Size))
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %v", ErrFormat, hdr.Name, err)
	}
	if int64(len(b)) != hdr.Size {
		return nil, fmt.Errorf("%w: %q: read %d bytes, header says %d", ErrFormat, hdr.Name, len(b), hdr.Size)
	}
	return b, nil
}

func writeTo(sink fileSink, name string, mode int64, r io.Reader) error {
	wc, err := sink(name, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(wc, r)
	if cerr := wc.Close(); err == nil {
		err = cerr
	}
	return err
}

// zeroWriter accepts only zero bytes.
type zeroWriter struct{}

func (zeroWriter) Write(p []byte) (int, error) {
	for _, c := range p {
		if c != 0 {
			return 0, errors.New("non-zero bytes after the end-of-archive marker")
		}
	}
	return len(p), nil
}

// check judges a scanned cask: a signature by a trusted key, then the files
// against the manifest.
func (s *scanned) check(trusted []*mldsa65.PublicKey) (*Result, error) {
	if s.manifest == nil {
		return nil, fmt.Errorf("%w: %s missing from the cask", ErrManifest, ManifestName)
	}
	if s.sig == nil {
		return nil, ErrUnsigned
	}
	if len(trusted) == 0 {
		return nil, fmt.Errorf("%w: no trusted build public key given", ErrSignature)
	}
	seal, err := gungnir.ParseEvidenceSeal(s.sig)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSignature, SigName, err)
	}
	var pub *mldsa65.PublicKey
	for _, k := range trusted {
		if k != nil && gungnir.KeyID(k) == seal.KeyId {
			pub = k
			break
		}
	}
	if pub == nil {
		return nil, fmt.Errorf("%w: signed by key %q, which is not a trusted build key", ErrSignature, seal.KeyId)
	}
	if err := gungnir.VerifyEvidence(s.manifest, seal, pub, Purpose); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSignature, err)
	}
	m, err := ParseManifest(s.manifest)
	if err != nil {
		return nil, err
	}
	var problems []string
	for name, want := range m.Files {
		got, ok := s.hashes[name]
		switch {
		case !ok:
			problems = append(problems, "missing "+name)
		case got != want:
			problems = append(problems, "modified "+name)
		}
	}
	for name := range s.hashes {
		if _, ok := m.Files[name]; !ok {
			problems = append(problems, "extra "+name)
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		if len(problems) > 20 {
			problems = append(problems[:20], fmt.Sprintf("... and %d more", len(problems)-20))
		}
		return nil, fmt.Errorf("%w: %s", ErrMismatch, strings.Join(problems, "; "))
	}
	rs := sha256.Sum256(s.manifest)
	return &Result{Root: s.root, Files: len(m.Files), KeyID: seal.KeyId, RuneSha256: hex.EncodeToString(rs[:])}, nil
}

// Verify reads a cask from r and checks it against the trusted build keys
// (more than one during a rotation overlap). It writes nothing.
func Verify(r io.Reader, trusted []*mldsa65.PublicKey, lim Limits) (*Result, error) {
	s, err := scan(r, lim, nil)
	if err != nil {
		return nil, err
	}
	return s.check(trusted)
}

// VerifyFile is Verify on a regular file.
func VerifyFile(file string, trusted []*mldsa65.PublicKey, lim Limits) (*Result, error) {
	f, err := openRegular(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only
	return Verify(f, trusted, lim)
}

func openRegular(file string) (*os.File, error) {
	f, err := os.Open(file) // #nosec G304 -- operator-named cask; its contents are verified
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%s: not a regular file", file)
	}
	return f, nil
}

// ExtractFile verifies the cask at file, then writes its contents (the
// root's children, including the rune and seal) into dest, which must not
// exist. Nothing is written unless the first, read-only pass verifies. The
// second pass writes through an os.Root into dest (mode 0700 until done),
// re-hashing as it goes; on any failure dest is removed. Files get 0755 if
// any execute bit was set, else 0644; setuid, setgid and sticky never
// survive, and ownership is the caller's.
func ExtractFile(file, dest string, trusted []*mldsa65.PublicKey, lim Limits) (*Result, error) {
	f, err := openRegular(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only
	if _, err := Verify(f, trusted, lim); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dest, 0o700); err != nil {
		return nil, fmt.Errorf("sealedcask: extract destination: %w", err)
	}
	res, err := extractInto(f, dest, trusted, lim)
	if err != nil {
		_ = os.RemoveAll(dest)
		return nil, err
	}
	if err := os.Chmod(dest, 0o755); err != nil { // #nosec G302 -- the extracted cask is readable, like the tarball
		_ = os.RemoveAll(dest)
		return nil, err
	}
	return res, nil
}

func extractInto(r io.Reader, dest string, trusted []*mldsa65.PublicKey, lim Limits) (*Result, error) {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	sink := func(name string, mode int64) (io.WriteCloser, error) {
		if dir := path.Dir(name); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("sealedcask: mkdir %s: %w", dir, err)
			}
		}
		perm := os.FileMode(0o644)
		if mode&0o111 != 0 {
			perm = 0o755
		}
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if err != nil {
			return nil, fmt.Errorf("sealedcask: create %s: %w", name, err)
		}
		return f, nil
	}
	s, err := scan(r, lim, sink)
	if err != nil {
		return nil, err
	}
	return s.check(trusted)
}
