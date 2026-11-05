// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package store_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/encryption"
	"unheaded/pkg/secrets/store"
)

func encryptedFileStore(t *testing.T, dir string) *store.FileStore {
	t.Helper()
	enc, err := encryption.NewSimpleEncryptor(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: dir, Encryptor: enc, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	return fs
}

func staticSecret(path, value string) *secrets.Secret {
	return &secrets.Secret{Path: path, Type: secrets.SecretTypeStatic, Data: map[string][]byte{"value": []byte(value)}}
}

// L2: storing in the clear is a decision, not what a forgotten field gives.
func TestFileStore_NoEncryptorNeedsAllowPlaintext(t *testing.T) {
	if _, err := store.NewFileStore(store.FileStoreConfig{BasePath: t.TempDir()}); err == nil {
		t.Fatal("a store with no encryptor started without AllowPlaintext")
	}
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: t.TempDir(), AllowPlaintext: true})
	if err != nil {
		t.Fatalf("AllowPlaintext: %v", err)
	}
	_ = fs.Close()
}

// L2: two secret paths never share a file. Those that could are refused.
func TestFileStore_DistinctPathsNeverShareAFile(t *testing.T) {
	ctx := context.Background()
	fs := encryptedFileStore(t, t.TempDir())
	if err := fs.Set(ctx, "a/b", staticSecret("a/b", "kept")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"a__b", "x_/y", "x/_y"} {
		if err := fs.Set(ctx, path, staticSecret(path, "other")); !errors.Is(err, secrets.ErrInvalidPath) {
			t.Errorf("Set(%q) = %v, want ErrInvalidPath", path, err)
		}
		if _, err := fs.Get(ctx, path); !errors.Is(err, secrets.ErrInvalidPath) {
			t.Errorf("Get(%q) = %v, want ErrInvalidPath", path, err)
		}
		if err := fs.Delete(ctx, path); !errors.Is(err, secrets.ErrInvalidPath) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidPath", path, err)
		}
	}
	got, err := fs.Get(ctx, "a/b")
	if err != nil || string(got.Data["value"]) != "kept" {
		t.Fatalf("a/b after the refused writes = %v, %v", got, err)
	}
	// A single underscore inside a name is fine.
	if err := fs.Set(ctx, "db_password/main_key", staticSecret("db_password/main_key", "v")); err != nil {
		t.Fatalf("Set with single underscores: %v", err)
	}
	list, err := fs.List(ctx, "db_")
	if err != nil || len(list) != 1 || list[0] != "db_password/main_key" {
		t.Fatalf("List = %v, %v", list, err)
	}
}

// L2: concurrent writers of one path never expose a partial secret.
func TestFileStore_ConcurrentSetsNeverTear(t *testing.T) {
	ctx := context.Background()
	fs := encryptedFileStore(t, t.TempDir())
	const writers, rounds, size = 8, 40, 1 << 16
	if err := fs.Set(ctx, "shared", staticSecret("shared", strings.Repeat("0", size))); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 2; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				got, err := fs.Get(ctx, "shared")
				if err != nil {
					t.Errorf("Get during writes: %v", err)
					return
				}
				v := got.Data["value"]
				if len(v) != size || strings.Trim(string(v), string(v[:1])) != "" {
					t.Errorf("torn secret: %d bytes", len(v))
					return
				}
			}
		}()
	}
	var writersWG sync.WaitGroup
	for w := 0; w < writers; w++ {
		writersWG.Add(1)
		go func() {
			defer writersWG.Done()
			value := strings.Repeat(fmt.Sprint(w), size)
			for i := 0; i < rounds; i++ {
				if err := fs.Set(ctx, "shared", staticSecret("shared", value)); err != nil {
					t.Errorf("Set: %v", err)
					return
				}
			}
		}()
	}
	writersWG.Wait()
	close(stop)
	readers.Wait()
}

// L2: a stored secret answers only to its own path, even under a store's
// valid encryption.
func TestFileStore_FileMovedToAnotherPathIsRefused(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs := encryptedFileStore(t, dir)
	if err := fs.Set(ctx, "app/one", staticSecret("app/one", "first")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Set(ctx, "app/two", staticSecret("app/two", "second")); err != nil {
		t.Fatal(err)
	}
	one, two := filepath.Join(dir, "app__one.json"), filepath.Join(dir, "app__two.json")
	moved, err := os.ReadFile(two) // #nosec G304 -- test file under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(one, moved, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := fs.Get(ctx, "app/one"); err == nil {
		t.Fatalf("app/one answered with %q from app/two's file", got.Data["value"])
	}
}

// The binding is the store's: a caller's Secret.Path cannot weaken it.
func TestFileStore_SetBindsTheGivenPath(t *testing.T) {
	ctx := context.Background()
	fs := encryptedFileStore(t, t.TempDir())
	s := staticSecret("", "v")
	if err := fs.Set(ctx, "app/three", s); err != nil {
		t.Fatal(err)
	}
	if s.Path != "" {
		t.Errorf("Set changed the caller's secret: Path = %q", s.Path)
	}
	got, err := fs.Get(ctx, "app/three")
	if err != nil || got.Path != "app/three" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}
