// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package auth

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// trustFixture is a trust directory with keys a and b and a clock the test
// moves by hand.
type trustFixture struct {
	dir        string
	td         *TrustDir
	kidA, kidB string
	at         time.Time
}

func newTrustFixture(t *testing.T) *trustFixture {
	t.Helper()
	f := &trustFixture{dir: t.TempDir(), at: time.Unix(1_000_000, 0)}
	keys := t.TempDir()
	var err error
	if f.kidA, err = GenerateKeyPair(keys, "a"); err != nil {
		t.Fatal(err)
	}
	if f.kidB, err = GenerateKeyPair(keys, "b"); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(keys, "a.pub"), filepath.Join(f.dir, "a.pub"))
	copyFile(t, filepath.Join(keys, "b.pub"), filepath.Join(f.dir, "b.pub"))
	if f.td, err = NewTrustDir(f.dir); err != nil {
		t.Fatal(err)
	}
	f.td.now = func() time.Time { return f.at }
	f.td.checked = f.at
	return f
}

func (f *trustFixture) trusted(kid string) bool {
	_, ok := f.td.Lookup(kid)
	return ok
}

// L12: a removed .pub stops being trusted with no restart and no Reload.
func TestTrustDir_RemovedKeyExpiresOnItsOwn(t *testing.T) {
	f := newTrustFixture(t)
	if err := os.Remove(filepath.Join(f.dir, "b.pub")); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(trustReloadInterval)
	if f.trusted(f.kidB) {
		t.Fatal("removed b.pub still trusted after the reload interval")
	}
	if !f.trusted(f.kidA) {
		t.Fatal("a.pub lost with b.pub")
	}
}

// L12: a new .pub is picked up the same way.
func TestTrustDir_AddedKeyArrivesOnItsOwn(t *testing.T) {
	f := newTrustFixture(t)
	keys := t.TempDir()
	kid, err := GenerateKeyPair(keys, "c")
	if err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(keys, "c.pub"), filepath.Join(f.dir, "c.pub"))
	f.at = f.at.Add(trustReloadInterval)
	if !f.trusted(kid) {
		t.Fatal("added c.pub not trusted after the reload interval")
	}
}

// L12: a directory Reload refuses does not keep a removed key alive, and
// does not cost the keys still on disk.
func TestTrustDir_RemovedKeyExpiresWhenReloadFails(t *testing.T) {
	f := newTrustFixture(t)
	if err := os.Remove(filepath.Join(f.dir, "b.pub")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "broken.pub"), []byte("not a key"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(trustReloadInterval)
	if f.trusted(f.kidB) {
		t.Fatal("removed b.pub still trusted while Reload fails")
	}
	if !f.trusted(f.kidA) {
		t.Fatal("a.pub, still on disk, dropped while Reload fails")
	}
}

// L12: a directory that has gone trusts nothing.
func TestTrustDir_MissingDirTrustsNothing(t *testing.T) {
	f := newTrustFixture(t)
	if err := os.RemoveAll(f.dir); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(trustReloadInterval)
	if f.trusted(f.kidA) || f.trusted(f.kidB) {
		t.Fatal("keys still trusted with the trust directory gone")
	}
}

// Inside the interval a lookup does not touch the disk.
func TestTrustDir_LookupInsideIntervalIsCached(t *testing.T) {
	f := newTrustFixture(t)
	if err := os.Remove(filepath.Join(f.dir, "b.pub")); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(trustReloadInterval - time.Second)
	if !f.trusted(f.kidB) {
		t.Fatal("reloaded before the interval")
	}
}

func TestTrustDir_ConcurrentLookups(t *testing.T) {
	f := newTrustFixture(t)
	f.td.now = time.Now
	f.td.mu.Lock()
	f.td.checked = time.Time{}
	f.td.mu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.trusted(f.kidA)
		}()
	}
	wg.Wait()
}

// jwksClock points v at a clock the test moves by hand.
func jwksClock(v *JWKSVerifier) *time.Time {
	at := time.Now()
	v.now = func() time.Time { return at }
	return &at
}

// L12: cached keys ride out an issuer outage for a bounded time, then stop
// being trusted, so a key the issuer has dropped cannot outlive the outage.
func TestJWKSVerifier_KeysExpireWhileFetchesFail(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	at := jwksClock(v)
	ctx := context.Background()
	if _, ok := v.keyFor(ctx, kid); !ok {
		t.Fatal("key not fetched")
	}
	fi.srv.Close()

	*at = at.Add(v.ttl + time.Second)
	if _, ok := v.keyFor(ctx, kid); !ok {
		t.Fatal("key dropped as soon as a refresh failed: a short outage must not stop traffic")
	}
	*at = at.Add(v.ttl * jwksMaxStaleTTLs)
	if _, ok := v.keyFor(ctx, kid); ok {
		t.Fatal("key still trusted past the stale limit with every fetch failing")
	}
}

// L12: while the issuer is down, requests do not each wait on a fetch.
func TestJWKSVerifier_DeadIssuerIsNotRetriedPerRequest(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	at := jwksClock(v)
	ctx := context.Background()
	if _, ok := v.keyFor(ctx, kid); !ok {
		t.Fatal("key not fetched")
	}
	fetches := 0
	v.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		fetches++
		return nil, os.ErrDeadlineExceeded
	})

	*at = at.Add(v.ttl)
	for i := 0; i < 5; i++ {
		if _, ok := v.keyFor(ctx, kid); !ok {
			t.Fatal("key dropped during a short outage")
		}
	}
	if fetches != 1 {
		t.Fatalf("%d fetches for 5 requests, want 1", fetches)
	}
	*at = at.Add(jwksFailRetryInterval)
	if _, ok := v.keyFor(ctx, kid); !ok {
		t.Fatal("key dropped during a short outage")
	}
	if fetches != 2 {
		t.Fatalf("%d fetches after the retry interval, want 2", fetches)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
