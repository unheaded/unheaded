// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package store_test

import (
	"context"
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
