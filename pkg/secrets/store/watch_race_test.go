// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package store_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// Sends and closes on a watcher channel must never overlap: a send on a
// closed channel panics the whole service. Run with -race.
func TestFileStore_NotifyDuringClose(t *testing.T) {
	for i := 0; i < 50; i++ {
		fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: t.TempDir(), PollInterval: time.Hour})
		if err != nil {
			t.Fatalf("NewFileStore: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		secret := &secrets.Secret{Path: "a/b", Type: secrets.SecretTypeStatic, Data: map[string][]byte{"v": []byte("x")}}
		if err := fs.Set(ctx, secret.Path, secret); err != nil {
			t.Fatalf("Set: %v", err)
		}
		for w := 0; w < 8; w++ {
			if _, err := fs.Watch(ctx, secret.Path); err != nil {
				t.Fatalf("Watch: %v", err)
			}
		}
		var wg sync.WaitGroup
		for n := 0; n < 4; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 20; j++ {
					_ = fs.Set(context.Background(), secret.Path, secret)
				}
			}()
		}
		cancel() // removeWatcher closes channels while Set is notifying them.
		_ = fs.Close()
		wg.Wait()
	}
}

// Close closes every watcher. A context cancelled afterwards must not close
// the same channel again.
func TestFileStore_CancelAfterClose(t *testing.T) {
	for i := 0; i < 200; i++ {
		fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: t.TempDir(), PollInterval: time.Hour})
		if err != nil {
			t.Fatalf("NewFileStore: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := fs.Watch(ctx, "a/b")
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
		// Both of the watcher goroutine's select cases are ready at once.
		cancel()
		if err := fs.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		select {
		case _, open := <-ch:
			if open {
				t.Fatal("closed watcher delivered a value")
			}
		case <-time.After(time.Second):
			t.Fatal("watcher channel not closed by Close")
		}
	}
}

// fakeVault answers the health check and accepts every write.
func fakeVault(t *testing.T) *store.VaultStore {
	t.Helper()
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(hs.Close)
	vs, err := store.NewVaultStore(store.VaultStoreConfig{Address: hs.URL, Token: "test-token", PollInterval: time.Hour})
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}
	return vs
}

// L1: the Vault store holds to the same rule as the file store.
func TestVaultStore_NotifyDuringClose(t *testing.T) {
	for i := 0; i < 50; i++ {
		vs := fakeVault(t)
		ctx, cancel := context.WithCancel(context.Background())
		secret := &secrets.Secret{Path: "a/b", Type: secrets.SecretTypeStatic, Data: map[string][]byte{"v": []byte("x")}}
		for w := 0; w < 8; w++ {
			if _, err := vs.Watch(ctx, secret.Path); err != nil {
				t.Fatalf("Watch: %v", err)
			}
		}
		var wg sync.WaitGroup
		for n := 0; n < 4; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 20; j++ {
					_ = vs.Set(context.Background(), secret.Path, secret)
				}
			}()
		}
		cancel()
		_ = vs.Close()
		wg.Wait()
	}
}

func TestVaultStore_CancelAfterClose(t *testing.T) {
	for i := 0; i < 200; i++ {
		vs := fakeVault(t)
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := vs.Watch(ctx, "a/b")
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
		cancel()
		if err := vs.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		expectClosedWatcher(t, ch)
	}
}

func TestMemoryStore_CancelAfterClose(t *testing.T) {
	for i := 0; i < 200; i++ {
		ms := store.NewMemoryStore(store.MemoryStoreConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := ms.Watch(ctx, "a/b")
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
		cancel()
		if err := ms.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		expectClosedWatcher(t, ch)
	}
}

func expectClosedWatcher(t *testing.T, ch <-chan *secrets.Secret) {
	t.Helper()
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("closed watcher delivered a value")
		}
	case <-time.After(time.Second):
		t.Fatal("watcher channel not closed by Close")
	}
}
