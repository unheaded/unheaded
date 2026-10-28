// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NewFileAPIKeyStore returns a store kept in a JSON-lines file, one record
// per line, for hosts without the Well (ADR-103 Q4). The file is written
// 0600; a store that is group- or world-writable, or world-readable, is
// refused (ErrAPIKeyStorePermissions), at open and on every read after.
// A missing file is an empty store; the first Create makes it (and its
// directory, 0750).
//
// Every read checks whether the file changed and reloads it, so a service
// sees keys created and revoked by unheaded-cli without a restart. Writers
// (Create, Revoke) serialise on <path>.lock. last_used_at is not recorded:
// rewriting the file on every request is not worth a timestamp.
func NewFileAPIKeyStore(path string, opts ...APIKeyStoreOption) (APIKeyStore, error) {
	if path == "" {
		return nil, errors.New("auth: API key store path is empty")
	}
	f := &fileAPIKeys{path: filepath.Clean(path)}
	f.mu.Lock()
	err := f.refreshLocked()
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return newAPIKeyStore(f, opts), nil
}

type fileStamp struct {
	size  int64
	mtime time.Time
	ino   uint64
}

type fileAPIKeys struct {
	path   string
	mu     sync.Mutex
	recs   []APIKeyRecord
	stamp  fileStamp
	loaded bool
}

// refreshLocked re-reads the file when it changed. Callers hold f.mu.
// Permissions are checked on every call, not only on a change: a chmod
// does not change size or mtime.
func (f *fileAPIKeys) refreshLocked() error {
	st, err := os.Lstat(f.path) // #nosec G703 -- operator-configured store path; type, owner and mode are checked below
	if errors.Is(err, fs.ErrNotExist) {
		f.recs, f.stamp, f.loaded = nil, fileStamp{}, true
		return nil
	}
	if err != nil {
		return fmt.Errorf("auth: API key store: %w", err)
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("auth: API key store %s is not a regular file", f.path)
	}
	if perm := st.Mode().Perm(); perm&0o022 != 0 || perm&0o004 != 0 {
		return fmt.Errorf("%w: %s is %#o", ErrAPIKeyStorePermissions, f.path, perm)
	}
	stamp := fileStamp{size: st.Size(), mtime: st.ModTime(), ino: inodeOf(st)}
	if f.loaded && stamp == f.stamp {
		return nil
	}
	data, err := os.ReadFile(f.path) // #nosec G304,G703 -- operator-configured store path, checked by the Lstat above
	if err != nil {
		return fmt.Errorf("auth: API key store: %w", err)
	}
	recs, err := parseAPIKeyLines(data)
	if err != nil {
		return fmt.Errorf("auth: API key store %s: %w", f.path, err)
	}
	f.recs, f.stamp, f.loaded = recs, stamp, true
	return nil
}

func parseAPIKeyLines(data []byte) ([]APIKeyRecord, error) {
	var recs []APIKeyRecord
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), 64*1024)
	line := 0
	for sc.Scan() {
		line++
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		var r APIKeyRecord
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := r.validate(); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if seen["id:"+r.ID] || seen["hash:"+r.KeyHash] {
			return nil, fmt.Errorf("line %d: duplicate id or key", line)
		}
		seen["id:"+r.ID], seen["hash:"+r.KeyHash] = true, true
		recs = append(recs, r)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

func (f *fileAPIKeys) read(fn func(recs []APIKeyRecord)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refreshLocked(); err != nil {
		return err
	}
	fn(f.recs)
	return nil
}

// write runs fn with the file locked against other writers and the
// records freshly read; fn returns the bytes to append, or the whole new
// content when replace is set.
func (f *fileAPIKeys) write(fn func(recs []APIKeyRecord) (data []byte, replace bool, err error)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(f.path), 0o750); err != nil {
		return fmt.Errorf("auth: API key store: %w", err)
	}
	unlock, err := lockFile(f.path + ".lock")
	if err != nil {
		return fmt.Errorf("auth: API key store lock: %w", err)
	}
	defer unlock()
	f.loaded = false // re-read under the lock
	if err := f.refreshLocked(); err != nil {
		return err
	}
	data, replace, err := fn(f.recs)
	if err != nil {
		return err
	}
	if replace {
		err = replaceFile(f.path, data)
	} else {
		err = appendFile(f.path, data)
	}
	if err != nil {
		return fmt.Errorf("auth: API key store: %w", err)
	}
	f.loaded = false
	return f.refreshLocked()
}

func appendFile(path string, data []byte) error {
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600) // #nosec G304 -- operator-configured store path, not from a request
	if err != nil {
		return err
	}
	if _, err := fh.Write(data); err != nil {
		_ = fh.Close()
		return err
	}
	if err := fh.Sync(); err != nil {
		_ = fh.Close()
		return err
	}
	return fh.Close()
}

func replaceFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".apikeys-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // already gone after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
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

func encodeAPIKeyLines(recs []APIKeyRecord) ([]byte, error) {
	var buf bytes.Buffer
	for _, r := range recs {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func (f *fileAPIKeys) insert(_ context.Context, rec APIKeyRecord) error {
	return f.write(func(recs []APIKeyRecord) ([]byte, bool, error) {
		for _, r := range recs {
			if r.KeyHash == rec.KeyHash || r.ID == rec.ID {
				return nil, false, fmt.Errorf("%w: duplicate key", ErrAPIKeyInvalid)
			}
		}
		b, err := encodeAPIKeyLines([]APIKeyRecord{rec})
		return b, false, err
	})
}

func (f *fileAPIKeys) byHash(_ context.Context, hash string) (rec APIKeyRecord, ok bool, err error) {
	err = f.read(func(recs []APIKeyRecord) { rec, ok, _ = findByHash(recs, hash) })
	return rec, ok, err
}

func (f *fileAPIKeys) byPrefixOrID(_ context.Context, s string) (out []APIKeyRecord, err error) {
	err = f.read(func(recs []APIKeyRecord) { out = findByPrefixOrID(recs, s) })
	return out, err
}

func (f *fileAPIKeys) all(_ context.Context) (out []APIKeyRecord, err error) {
	err = f.read(func(recs []APIKeyRecord) { out = copyRecords(recs) })
	return out, err
}

func (f *fileAPIKeys) revoke(_ context.Context, id string, at time.Time) (out APIKeyRecord, err error) {
	err = f.write(func(recs []APIKeyRecord) ([]byte, bool, error) {
		recs = copyRecords(recs)
		r, err := revokeIn(recs, id, at)
		if err != nil {
			return nil, false, err
		}
		out = r
		b, err := encodeAPIKeyLines(recs)
		return b, true, err
	})
	return out, err
}

func (f *fileAPIKeys) touch(context.Context, string, time.Time) error { return nil }
