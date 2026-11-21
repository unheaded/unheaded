// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sessions

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// The store contract, run against Mem always and against the Well when a
// throwaway server is given (well_test.go, build tag wellint): pkg/bff is
// tested on Mem, so Mem and the Well must answer alike.

// world gives the suite two fronts' stores and an Admin over one set of
// sessions that starts empty.
type world struct {
	a, b   Store // fronts "front-a" and "front-b"
	admin  Admin
	frontA string
	frontB string
}

var t0 = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func key(n int) string { return KeyHash(fmt.Sprintf("cookie-%d", n)) }

func sess(front string, n int, created time.Time) Session {
	return Session{
		KeyHash: key(n), Front: front, Subject: "dev", Roles: []string{"operator", "viewer"},
		CreatedAt: created, LastUsedAt: created,
		ExpiresAt: created.Add(30 * time.Minute), AbsoluteExpiresAt: created.Add(12 * time.Hour),
	}
}

func mustCreate(t *testing.T, st Store, s Session) {
	t.Helper()
	if err := st.Create(context.Background(), s, 100); err != nil {
		t.Fatalf("create: %v", err)
	}
}

func storeSuite(t *testing.T, fresh func(t *testing.T) world) {
	ctx := context.Background()

	t.Run("a created session reads back as written", func(t *testing.T) {
		w := fresh(t)
		want := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, want)
		got, ok, err := w.a.Get(ctx, want.KeyHash)
		if err != nil || !ok {
			t.Fatalf("get: %v %v", ok, err)
		}
		if got.KeyHash != want.KeyHash || got.Front != want.Front || got.Subject != "dev" ||
			fmt.Sprint(got.Roles) != "[operator viewer]" || !got.CreatedAt.Equal(t0) || !got.LastUsedAt.Equal(t0) ||
			!got.ExpiresAt.Equal(want.ExpiresAt) || !got.AbsoluteExpiresAt.Equal(want.AbsoluteExpiresAt) || got.RevokedAt != nil {
			t.Fatalf("got %+v", got)
		}
		if _, ok, err := w.a.Get(ctx, key(2)); ok || err != nil {
			t.Fatalf("unknown key: %v %v", ok, err)
		}
		noRoles := sess(w.frontA, 3, t0)
		noRoles.Roles = nil
		mustCreate(t, w.a, noRoles)
		if got, _, _ := w.a.Get(ctx, noRoles.KeyHash); len(got.Roles) != 0 {
			t.Fatalf("no roles read back as %q", got.Roles)
		}
	})

	t.Run("one front's session is no session to another", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		if _, ok, err := w.b.Get(ctx, s.KeyHash); ok || err != nil {
			t.Fatalf("front b read front a's session: %v %v", ok, err)
		}
		if ok, _ := w.b.Touch(ctx, s.KeyHash, t0.Add(time.Minute), t0.Add(31*time.Minute)); ok {
			t.Fatal("front b extended front a's session")
		}
		if err := w.b.Revoke(ctx, s.KeyHash, t0.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if got, _, _ := w.a.Get(ctx, s.KeyHash); got.RevokedAt != nil {
			t.Fatal("front b revoked front a's session")
		}
		if n, _ := w.b.Sweep(ctx, t0.Add(100*time.Hour), 100); n != 0 {
			t.Fatalf("front b swept %d of front a's sessions", n)
		}
		// A front cannot write a session in another front's name.
		forged := sess(w.frontA, 2, t0)
		if err := w.b.Create(ctx, forged, 100); err == nil {
			t.Fatal("front b created a session of front a")
		}
		if _, ok, _ := w.a.Get(ctx, forged.KeyHash); ok {
			t.Fatal("the forged session exists for front a")
		}
	})

	t.Run("touch extends a live session, never past its maximum age", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		now := t0.Add(20 * time.Minute)
		if ok, err := w.a.Touch(ctx, s.KeyHash, now, now.Add(30*time.Minute)); !ok || err != nil {
			t.Fatalf("touch: %v %v", ok, err)
		}
		got, _, _ := w.a.Get(ctx, s.KeyHash)
		if !got.LastUsedAt.Equal(now) || !got.ExpiresAt.Equal(now.Add(30*time.Minute)) {
			t.Fatalf("after touch: last used %v expires %v", got.LastUsedAt, got.ExpiresAt)
		}
		late := t0.Add(11*time.Hour + 50*time.Minute)
		// Keep it alive up to there first.
		for at := now; at.Before(late); at = at.Add(25 * time.Minute) {
			if ok, _ := w.a.Touch(ctx, s.KeyHash, at, at.Add(30*time.Minute)); !ok {
				t.Fatalf("touch at %v refused", at)
			}
		}
		if ok, _ := w.a.Touch(ctx, s.KeyHash, late, late.Add(30*time.Minute)); !ok {
			t.Fatal("touch near the maximum age refused")
		}
		got, _, _ = w.a.Get(ctx, s.KeyHash)
		if !got.ExpiresAt.Equal(s.AbsoluteExpiresAt) {
			t.Fatalf("expiry %v, want it held at the maximum age %v", got.ExpiresAt, s.AbsoluteExpiresAt)
		}
		if !got.AbsoluteExpiresAt.Equal(s.AbsoluteExpiresAt) || !got.CreatedAt.Equal(t0) {
			t.Fatal("touch moved the maximum age or the creation time")
		}
	})

	t.Run("touch does not revive an expired or revoked session", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		after := t0.Add(31 * time.Minute)
		if ok, _ := w.a.Touch(ctx, s.KeyHash, after, after.Add(30*time.Minute)); ok {
			t.Fatal("touch revived a session past its idle expiry")
		}
		if got, _, _ := w.a.Get(ctx, s.KeyHash); !got.ExpiresAt.Equal(s.ExpiresAt) {
			t.Fatal("the refused touch still moved the expiry")
		}
		r := sess(w.frontA, 2, t0)
		mustCreate(t, w.a, r)
		if err := w.a.Revoke(ctx, r.KeyHash, t0.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if ok, _ := w.a.Touch(ctx, r.KeyHash, t0.Add(2*time.Minute), t0.Add(32*time.Minute)); ok {
			t.Fatal("touch revived a revoked session")
		}
		if ok, _ := w.a.Touch(ctx, key(99), t0, t0.Add(time.Minute)); ok {
			t.Fatal("touch of an unknown key reported a change")
		}
	})

	// Two requests on one session race to record their use. Whichever
	// write lands second carries the earlier time. It must still find the
	// session live: "not touched" means ended, and a live session is not.
	t.Run("touches that arrive out of order both find the session live", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		later, earlier := t0.Add(2*time.Minute), t0.Add(2*time.Minute-time.Second)
		if ok, err := w.a.Touch(ctx, s.KeyHash, later, later.Add(30*time.Minute)); !ok || err != nil {
			t.Fatalf("first write: %v %v", ok, err)
		}
		if ok, err := w.a.Touch(ctx, s.KeyHash, earlier, earlier.Add(30*time.Minute)); !ok || err != nil {
			t.Fatalf("the write that landed second, with the earlier time: %v %v, want the session found live", ok, err)
		}
		got, _, _ := w.a.Get(ctx, s.KeyHash)
		if !got.LastUsedAt.Equal(later) || !got.ExpiresAt.Equal(later.Add(30*time.Minute)) {
			t.Fatalf("the late write moved the record backwards: last used %v, expires %v", got.LastUsedAt, got.ExpiresAt)
		}
		// The same instant twice is no different.
		if ok, _ := w.a.Touch(ctx, s.KeyHash, later, later.Add(30*time.Minute)); !ok {
			t.Fatal("a second write at the same instant found no session")
		}
		// An out-of-order write still cannot revive what has ended, nor
		// pass the maximum age.
		gone := t0.Add(40 * time.Minute)
		if ok, _ := w.a.Touch(ctx, s.KeyHash, gone, gone.Add(30*time.Minute)); ok {
			t.Fatal("a write after the idle expiry found the session live")
		}
		r := sess(w.frontA, 2, t0)
		mustCreate(t, w.a, r)
		if err := w.a.Revoke(ctx, r.KeyHash, later); err != nil {
			t.Fatal(err)
		}
		if ok, _ := w.a.Touch(ctx, r.KeyHash, earlier, earlier.Add(30*time.Minute)); ok {
			t.Fatal("a write carrying a time before the revocation revived the session")
		}
	})

	t.Run("revoke is once and for all", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		first := t0.Add(time.Minute)
		if err := w.a.Revoke(ctx, s.KeyHash, first); err != nil {
			t.Fatal(err)
		}
		if err := w.a.Revoke(ctx, s.KeyHash, t0.Add(time.Hour)); err != nil {
			t.Fatalf("second revoke: %v", err)
		}
		got, ok, _ := w.a.Get(ctx, s.KeyHash)
		if !ok || got.RevokedAt == nil || !got.RevokedAt.Equal(first) || got.Live(first) {
			t.Fatalf("after revoke: %+v", got)
		}
		if err := w.a.Revoke(ctx, key(99), first); err != nil {
			t.Fatalf("revoking nothing: %v", err)
		}
		// A revocation stamped before the creation time (two clocks) still
		// revokes; the stamp is held to the creation time.
		early := sess(w.frontA, 2, t0)
		mustCreate(t, w.a, early)
		if err := w.a.Revoke(ctx, early.KeyHash, t0.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if got, _, _ := w.a.Get(ctx, early.KeyHash); got.RevokedAt == nil || got.RevokedAt.Before(t0) {
			t.Fatalf("early revocation: %+v", got.RevokedAt)
		}
	})

	t.Run("the cap counts live sessions of the front only", func(t *testing.T) {
		w := fresh(t)
		for i := 1; i <= 2; i++ {
			if err := w.a.Create(ctx, sess(w.frontA, i, t0), 2); err != nil {
				t.Fatalf("create %d: %v", i, err)
			}
		}
		if err := w.a.Create(ctx, sess(w.frontA, 3, t0), 2); !errors.Is(err, ErrFull) {
			t.Fatalf("third session at a cap of 2: %v, want ErrFull", err)
		}
		if err := w.b.Create(ctx, sess(w.frontB, 4, t0), 2); err != nil {
			t.Fatalf("the other front is not full: %v", err)
		}
		// An ended session frees its place.
		if err := w.a.Revoke(ctx, key(1), t0.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := w.a.Create(ctx, sess(w.frontA, 5, t0.Add(time.Minute)), 2); err != nil {
			t.Fatalf("after a revocation: %v", err)
		}
		if err := w.a.Create(ctx, sess(w.frontA, 6, t0.Add(40*time.Minute)), 2); err != nil {
			t.Fatalf("after the others expired: %v", err)
		}
	})

	t.Run("a key is used once", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0))
		if err := w.a.Create(ctx, sess(w.frontA, 1, t0.Add(time.Minute)), 100); err == nil {
			t.Fatal("a second session under the same key was accepted")
		}
	})

	t.Run("create refuses what a session may not hold", func(t *testing.T) {
		w := fresh(t)
		for name, mod := range map[string]func(*Session){
			"a key that is no hash":          func(s *Session) { s.KeyHash = "cookie-value" },
			"an upper-case hash":             func(s *Session) { s.KeyHash = "A" + s.KeyHash[1:] },
			"an empty subject":               func(s *Session) { s.Subject = "" },
			"a subject with a space":         func(s *Session) { s.Subject = "de v" },
			"a subject with a newline":       func(s *Session) { s.Subject = "dev\nadmin" },
			"a role with a comma":            func(s *Session) { s.Roles = []string{"viewer,admin"} },
			"an upper-case role":             func(s *Session) { s.Roles = []string{"Admin"} },
			"too many roles":                 func(s *Session) { s.Roles = make([]string, MaxRoles+1); fillRoles(s.Roles) },
			"expiry past the maximum age":    func(s *Session) { s.ExpiresAt = s.AbsoluteExpiresAt.Add(time.Second) },
			"expiry at creation":             func(s *Session) { s.ExpiresAt = s.CreatedAt },
			"a maximum age over the bound":   func(s *Session) { s.AbsoluteExpiresAt = s.CreatedAt.Add(MaxLifetime + time.Hour) },
			"last used before creation":      func(s *Session) { s.LastUsedAt = s.CreatedAt.Add(-time.Second) },
			"born revoked":                   func(s *Session) { at := s.CreatedAt; s.RevokedAt = &at },
			"a front with an underscore":     func(s *Session) { s.Front = "front_a" },
			"another front than the store's": func(s *Session) { s.Front = "front-zz" },
		} {
			s := sess(w.frontA, 1, t0)
			mod(&s)
			if err := w.a.Create(ctx, s, 100); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
		if _, ok, _ := w.a.Get(ctx, key(1)); ok {
			t.Error("a refused session was stored")
		}
	})

	t.Run("sweep deletes what ended before the cutoff, within its limit", func(t *testing.T) {
		w := fresh(t)
		for i := 1; i <= 5; i++ {
			mustCreate(t, w.a, sess(w.frontA, i, t0)) // expire at t0+30m
		}
		live := sess(w.frontA, 6, t0.Add(2*time.Hour))
		mustCreate(t, w.a, live)
		revoked := sess(w.frontA, 7, t0.Add(2*time.Hour))
		mustCreate(t, w.a, revoked)
		if err := w.a.Revoke(ctx, revoked.KeyHash, t0.Add(2*time.Hour+time.Minute)); err != nil {
			t.Fatal(err)
		}
		mustCreate(t, w.b, sess(w.frontB, 8, t0))

		if n, err := w.a.Sweep(ctx, t0.Add(20*time.Minute), 100); n != 0 || err != nil {
			t.Fatalf("sweep before anything ended: %d %v", n, err)
		}
		cutoff := t0.Add(2*time.Hour + 5*time.Minute)
		if n, err := w.a.Sweep(ctx, cutoff, 3); n != 3 || err != nil {
			t.Fatalf("sweep with a limit of 3: %d %v", n, err)
		}
		if n, err := w.a.Sweep(ctx, cutoff, 100); n != 3 || err != nil { // two expired, one revoked
			t.Fatalf("second sweep: %d %v, want 3", n, err)
		}
		if _, ok, _ := w.a.Get(ctx, live.KeyHash); !ok {
			t.Fatal("sweep deleted a live session")
		}
		if _, ok, _ := w.b.Get(ctx, key(8)); !ok {
			t.Fatal("front a's sweep deleted front b's session")
		}
		if n, _ := w.a.Sweep(ctx, cutoff, 0); n != 0 {
			t.Fatal("a limit of 0 deleted something")
		}
	})

	t.Run("admin lists, newest first, and filters", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0))
		other := sess(w.frontB, 2, t0.Add(time.Minute))
		other.Subject = "alice"
		mustCreate(t, w.b, other)
		mustCreate(t, w.b, sess(w.frontB, 3, t0.Add(2*time.Minute)))
		now := t0.Add(3 * time.Minute)
		all, err := w.admin.List(ctx, Filter{}, now)
		if err != nil || len(all) != 3 || all[0].KeyHash != key(3) || all[2].KeyHash != key(1) {
			t.Fatalf("list: %d %v", len(all), err)
		}
		if got, _ := w.admin.List(ctx, Filter{Subject: "alice"}, now); len(got) != 1 || got[0].Front != w.frontB {
			t.Fatalf("by subject: %+v", got)
		}
		if got, _ := w.admin.List(ctx, Filter{Front: w.frontB}, now); len(got) != 2 {
			t.Fatalf("by front: %d", len(got))
		}
		if got, _ := w.admin.List(ctx, Filter{Subject: "dev", Front: w.frontA}, now); len(got) != 1 {
			t.Fatalf("by both: %d", len(got))
		}
		// Ended sessions are left out unless asked for.
		later := t0.Add(31*time.Minute + 30*time.Second)
		if got, _ := w.admin.List(ctx, Filter{}, later); len(got) != 1 || got[0].KeyHash != key(3) {
			t.Fatalf("live at %v: %d", later, len(got))
		}
		if got, _ := w.admin.List(ctx, Filter{Ended: true}, later); len(got) != 3 {
			t.Fatalf("with ended: %d", len(got))
		}
	})

	t.Run("admin revokes one session by its id", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		mustCreate(t, w.b, sess(w.frontB, 2, t0))
		now := t0.Add(time.Minute)
		got, err := w.admin.Revoke(ctx, s.ID(), now)
		if err != nil || got.KeyHash != s.KeyHash || got.RevokedAt == nil {
			t.Fatalf("revoke: %+v %v", got, err)
		}
		if after, _, _ := w.a.Get(ctx, s.KeyHash); after.Live(now) {
			t.Fatal("the front still sees it live")
		}
		if after, _, _ := w.b.Get(ctx, key(2)); !after.Live(now) {
			t.Fatal("another session was revoked with it")
		}
		if _, err := w.admin.Revoke(ctx, s.ID(), now); !errors.Is(err, ErrNotFound) {
			t.Fatalf("revoking it again: %v, want ErrNotFound", err)
		}
		if _, err := w.admin.Revoke(ctx, "0123456789abcdef", now); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown id: %v", err)
		}
		for _, bad := range []string{"", "abc", "zzzzzzzz", "ABCDEF01", s.KeyHash + "0", "%%%%%%%%", "abcdef0_"} {
			if _, err := w.admin.Revoke(ctx, bad, now); !errors.Is(err, ErrInvalid) {
				t.Errorf("id %q: %v, want ErrInvalid", bad, err)
			}
		}
	})

	t.Run("admin refuses a prefix that names two sessions", func(t *testing.T) {
		w := fresh(t)
		a, b := twoKeysSharing(t)
		sa, sb := sess(w.frontA, 0, t0), sess(w.frontA, 0, t0)
		sa.KeyHash, sb.KeyHash = a, b
		mustCreate(t, w.a, sa)
		mustCreate(t, w.a, sb)
		if _, err := w.admin.Revoke(ctx, a[:MinPrefix], t0.Add(time.Minute)); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("shared prefix: %v, want ErrAmbiguous", err)
		}
		if got, _, _ := w.a.Get(ctx, a); got.RevokedAt != nil {
			t.Fatal("an ambiguous revoke still revoked")
		}
		if _, err := w.admin.Revoke(ctx, a, t0.Add(time.Minute)); err != nil {
			t.Fatalf("the full hash: %v", err)
		}
	})

	t.Run("admin revokes every session of a subject", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0))
		mustCreate(t, w.a, sess(w.frontA, 2, t0))
		mustCreate(t, w.b, sess(w.frontB, 3, t0))
		alice := sess(w.frontA, 4, t0)
		alice.Subject = "alice"
		mustCreate(t, w.a, alice)
		now := t0.Add(time.Minute)
		if n, err := w.admin.RevokeSubject(ctx, "dev", w.frontA, now); n != 2 || err != nil {
			t.Fatalf("dev on front a: %d %v", n, err)
		}
		if got, _, _ := w.b.Get(ctx, key(3)); !got.Live(now) {
			t.Fatal("front b's session of dev was revoked by a front a revocation")
		}
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", now); n != 1 || err != nil {
			t.Fatalf("dev everywhere: %d %v", n, err)
		}
		if got, _, _ := w.a.Get(ctx, alice.KeyHash); !got.Live(now) {
			t.Fatal("alice's session was revoked with dev's")
		}
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", now); n != 0 || err != nil {
			t.Fatalf("again: %d %v", n, err)
		}
		if _, err := w.admin.RevokeSubject(ctx, "", "", now); !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty subject: %v, want ErrInvalid (never every session by accident)", err)
		}
	})

	// The admin's clock is not the front's. A session that looks expired
	// from where the command runs may be live at the front, so revocation
	// never asks the clock.
	t.Run("admin revocation does not depend on the admin's clock", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0)) // expires t0+30m by the front's clock
		mustCreate(t, w.a, sess(w.frontA, 2, t0))
		fast := t0.Add(10 * time.Hour) // the admin's clock runs ten hours ahead
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", fast); n != 2 || err != nil {
			t.Fatalf("revoke subject with a fast clock: %d %v, want both sessions", n, err)
		}
		frontNow := t0.Add(time.Minute)
		for _, k := range []string{key(1), key(2)} {
			if got, _, _ := w.a.Get(ctx, k); got.Live(frontNow) || got.RevokedAt == nil {
				t.Fatalf("a session survived a revocation issued under a fast clock: %+v", got)
			}
		}
		mustCreate(t, w.a, sess(w.frontA, 3, t0))
		if _, err := w.admin.Revoke(ctx, key(3)[:16], fast); err != nil {
			t.Fatalf("revoke by id with a fast clock: %v", err)
		}
		if got, _, _ := w.a.Get(ctx, key(3)); got.Live(frontNow) {
			t.Fatal("a session survived a revocation by id issued under a fast clock")
		}
		// And a slow clock: the stamp is held to the creation time.
		mustCreate(t, w.a, sess(w.frontA, 4, t0))
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", t0.Add(-10*time.Hour)); n != 1 || err != nil {
			t.Fatalf("revoke subject with a slow clock: %d %v", n, err)
		}
		if got, _, _ := w.a.Get(ctx, key(4)); got.Live(frontNow) {
			t.Fatal("a session survived a revocation issued under a slow clock")
		}
	})
}

func fillRoles(r []string) {
	for i := range r {
		r[i] = fmt.Sprintf("role%d", i)
	}
}

// twoKeysSharing returns two valid hashes with the same first MinPrefix
// characters. They are not hashes of anything: only their shape matters.
func twoKeysSharing(t *testing.T) (string, string) {
	t.Helper()
	a := key(1)
	last := byte('0')
	if a[len(a)-1] == '0' {
		last = '1'
	}
	b := a[:len(a)-1] + string(last)
	if a == b || a[:MinPrefix] != b[:MinPrefix] || !ValidKeyHash(b) {
		t.Fatal("could not make two keys with a shared prefix")
	}
	return a, b
}
