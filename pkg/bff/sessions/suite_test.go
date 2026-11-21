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
	a, b   Store // the stores of frontA and frontB
	admin  Admin
	frontA string
	frontB string
	// storeNow is the store's own clock, which CreateLimits.MaxSkew is
	// measured against: the Well's now(), or the clock Mem was given.
	storeNow func() time.Time
}

var t0 = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

const (
	testActor = "cli:tester@host"
	roomy     = 100 // a front cap no case here reaches by accident
)

func key(n int) string { return KeyHash(fmt.Sprintf("cookie-%d", n)) }

func sess(front string, n int, created time.Time) Session {
	return Session{
		KeyHash: key(n), Front: front, Subject: "dev", Roles: []string{"operator", "viewer"},
		CreatedAt: created, LastUsedAt: created,
		ExpiresAt: created.Add(30 * time.Minute), AbsoluteExpiresAt: created.Add(12 * time.Hour),
	}
}

func sessOf(front, subject string, n int, created time.Time) Session {
	s := sess(front, n, created)
	s.Subject = subject
	return s
}

func mustCreate(t *testing.T, st Store, s Session) {
	t.Helper()
	if _, err := st.Create(context.Background(), s, CreateLimits{MaxLive: roomy}); err != nil {
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
			!got.ExpiresAt.Equal(want.ExpiresAt) || !got.AbsoluteExpiresAt.Equal(want.AbsoluteExpiresAt) ||
			got.RevokedAt != nil || got.RevokedBy != "" {
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
		if err := w.b.Revoke(ctx, s.KeyHash, t0.Add(time.Minute), ByLogout); err != nil {
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
		if _, err := w.b.Create(ctx, forged, CreateLimits{MaxLive: roomy}); err == nil {
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
		if err := w.a.Revoke(ctx, r.KeyHash, t0.Add(time.Minute), ByLogout); err != nil {
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
		if err := w.a.Revoke(ctx, r.KeyHash, later, ByLogout); err != nil {
			t.Fatal(err)
		}
		if ok, _ := w.a.Touch(ctx, r.KeyHash, earlier, earlier.Add(30*time.Minute)); ok {
			t.Fatal("a write carrying a time before the revocation revived the session")
		}
	})

	t.Run("revoke is once and for all, and says what did it", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		first := t0.Add(time.Minute)
		if err := w.a.Revoke(ctx, s.KeyHash, first, ByLogout); err != nil {
			t.Fatal(err)
		}
		if err := w.a.Revoke(ctx, s.KeyHash, t0.Add(time.Hour), ByLogin); err != nil {
			t.Fatalf("second revoke: %v", err)
		}
		got, ok, _ := w.a.Get(ctx, s.KeyHash)
		if !ok || got.RevokedAt == nil || !got.RevokedAt.Equal(first) || got.RevokedBy != ByLogout || got.Live(first) {
			t.Fatalf("after revoke: %+v", got)
		}
		if err := w.a.Revoke(ctx, key(99), first, ByLogout); err != nil {
			t.Fatalf("revoking nothing: %v", err)
		}
		// A revocation stamped before the creation time (two clocks) still
		// revokes; the stamp is held to the creation time.
		early := sess(w.frontA, 2, t0)
		mustCreate(t, w.a, early)
		if err := w.a.Revoke(ctx, early.KeyHash, t0.Add(-time.Minute), ByLogout); err != nil {
			t.Fatal(err)
		}
		if got, _, _ := w.a.Get(ctx, early.KeyHash); got.RevokedAt == nil || got.RevokedAt.Before(t0) {
			t.Fatalf("early revocation: %+v", got.RevokedAt)
		}
		// What did it must be sayable: nothing is revoked by "" or by
		// something a row cannot hold.
		named := sess(w.frontA, 3, t0)
		mustCreate(t, w.a, named)
		for _, by := range []string{"", "Log Out", "cli:", "cli:a b", "x:" + string(make([]byte, 200))} {
			if err := w.a.Revoke(ctx, named.KeyHash, first, by); !errors.Is(err, ErrInvalid) {
				t.Errorf("revoke by %q: %v, want ErrInvalid", by, err)
			}
		}
		if got, _, _ := w.a.Get(ctx, named.KeyHash); got.RevokedAt != nil {
			t.Fatal("a revocation with no usable actor still revoked")
		}
	})

	t.Run("the cap counts live sessions of the front only", func(t *testing.T) {
		w := fresh(t)
		two := CreateLimits{MaxLive: 2}
		for i := 1; i <= 2; i++ {
			if _, err := w.a.Create(ctx, sessOf(w.frontA, fmt.Sprintf("user%d", i), i, t0), two); err != nil {
				t.Fatalf("create %d: %v", i, err)
			}
		}
		if _, err := w.a.Create(ctx, sessOf(w.frontA, "user3", 3, t0), two); !errors.Is(err, ErrFull) {
			t.Fatalf("third session at a cap of 2: %v, want ErrFull", err)
		}
		if _, err := w.b.Create(ctx, sess(w.frontB, 4, t0), two); err != nil {
			t.Fatalf("the other front is not full: %v", err)
		}
		// An ended session frees its place.
		if err := w.a.Revoke(ctx, key(1), t0.Add(time.Second), ByLogout); err != nil {
			t.Fatal(err)
		}
		if _, err := w.a.Create(ctx, sessOf(w.frontA, "user5", 5, t0.Add(time.Minute)), two); err != nil {
			t.Fatalf("after a revocation: %v", err)
		}
		if _, err := w.a.Create(ctx, sessOf(w.frontA, "user6", 6, t0.Add(40*time.Minute)), two); err != nil {
			t.Fatalf("after the others expired: %v", err)
		}
	})

	// One account logging in again and again does not fill the front: past
	// its own cap, its oldest session is ended to make room. Other users
	// still get in, and the user itself is never refused.
	t.Run("a subject past its cap loses its oldest session, not its login", func(t *testing.T) {
		w := fresh(t)
		lim := CreateLimits{MaxLive: 4, MaxPerSubject: 2}
		for i := 1; i <= 6; i++ {
			ended, err := w.a.Create(ctx, sess(w.frontA, i, t0.Add(time.Duration(i)*time.Second)), lim)
			if err != nil {
				t.Fatalf("login %d of one subject at a per-subject cap of 2 and a front cap of 4: %v", i, err)
			}
			if want := map[bool]int{true: 1, false: 0}[i > 2]; ended != want {
				t.Fatalf("login %d ended %d older sessions, want %d", i, ended, want)
			}
		}
		now := t0.Add(10 * time.Second)
		for i := 1; i <= 6; i++ {
			got, ok, _ := w.a.Get(ctx, key(i))
			switch {
			case !ok:
				t.Fatalf("session %d is gone: an ended session stays on record", i)
			case i <= 4 && (got.Live(now) || got.RevokedBy != ByLimit):
				t.Fatalf("session %d should have been ended by the cap: %+v", i, got)
			case i > 4 && !got.Live(now):
				t.Fatalf("session %d, one of the newest two, is not live: %+v", i, got)
			}
		}
		// The front has room for others: the one subject holds two of four.
		for i, who := range []string{"alice", "bob"} {
			if _, err := w.a.Create(ctx, sessOf(w.frontA, who, 10+i, now), lim); err != nil {
				t.Fatalf("%s's login after one subject logged in six times: %v", who, err)
			}
		}
		// Now it is full, by four sessions of three users: a fourth user is
		// refused, and nothing of anyone's was ended for it.
		if _, err := w.a.Create(ctx, sessOf(w.frontA, "carol", 12, now), lim); !errors.Is(err, ErrFull) {
			t.Fatalf("a fourth user at a full front: %v, want ErrFull", err)
		}
		for _, k := range []int{5, 6, 10, 11} {
			if got, _, _ := w.a.Get(ctx, key(k)); !got.Live(now) {
				t.Fatalf("session %d was ended by a login that was refused", k)
			}
		}
		// The subject at its cap still logs in at a full front: its own
		// oldest makes the room.
		if ended, err := w.a.Create(ctx, sess(w.frontA, 13, now.Add(time.Second)), lim); err != nil || ended != 1 {
			t.Fatalf("the capped subject at a full front: ended %d, %v", ended, err)
		}
		// The cap is per front, and counts live sessions only.
		if _, err := w.b.Create(ctx, sess(w.frontB, 20, now), lim); err != nil {
			t.Fatalf("the same subject on another front: %v", err)
		}
		if got, _, _ := w.a.Get(ctx, key(13)); !got.Live(now.Add(2 * time.Second)) {
			t.Fatal("a login on another front ended this front's session")
		}
		later := now.Add(time.Hour) // every session above has run out
		if ended, err := w.a.Create(ctx, sess(w.frontA, 21, later), lim); err != nil || ended != 0 {
			t.Fatalf("after the subject's sessions expired: ended %d, %v; expired sessions do not count", ended, err)
		}
		// No cap asked, no cap applied.
		for i := 30; i < 35; i++ {
			if ended, err := w.a.Create(ctx, sessOf(w.frontA, "many", i, later), CreateLimits{MaxLive: roomy}); err != nil || ended != 0 {
				t.Fatalf("without a per-subject cap: ended %d, %v", ended, err)
			}
		}
	})

	// The trim and the write stand or fall together: a login the front
	// refuses ends none of the user's sessions. (A front can be over its
	// cap when the cap was lowered under it.)
	t.Run("a login refused at a full front ends none of the subject's sessions", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0))
		mustCreate(t, w.a, sess(w.frontA, 2, t0.Add(time.Second)))
		mustCreate(t, w.a, sessOf(w.frontA, "alice", 3, t0.Add(2*time.Second)))
		at := t0.Add(3 * time.Second)
		ended, err := w.a.Create(ctx, sess(w.frontA, 4, at), CreateLimits{MaxLive: 2, MaxPerSubject: 2})
		if !errors.Is(err, ErrFull) || ended != 0 {
			t.Fatalf("a login at a front holding 3 of 2: ended %d, %v; want ErrFull and nothing ended", ended, err)
		}
		for i := 1; i <= 3; i++ {
			if got, _, _ := w.a.Get(ctx, key(i)); !got.Live(at) || got.RevokedBy != "" {
				t.Fatalf("session %d was ended by a login that was refused: %+v", i, got)
			}
		}
		if _, ok, _ := w.a.Get(ctx, key(4)); ok {
			t.Fatal("the refused session was stored")
		}
	})

	// The front's clock judges expiry afterwards, so a front whose clock is
	// far from the store's must not start sessions.
	t.Run("a session whose creation time is far from the store's clock is refused", func(t *testing.T) {
		w := fresh(t)
		lim := CreateLimits{MaxLive: roomy, MaxSkew: time.Minute}
		at := func(d time.Duration, n int) Session { return sess(w.frontA, n, w.storeNow().Add(d)) }
		if _, err := w.a.Create(ctx, at(10*time.Minute, 1), lim); !errors.Is(err, ErrClockSkew) {
			t.Fatalf("a clock ten minutes fast, bound one minute: %v, want ErrClockSkew", err)
		}
		if _, err := w.a.Create(ctx, at(-10*time.Minute, 2), lim); !errors.Is(err, ErrClockSkew) {
			t.Fatalf("a clock ten minutes slow: %v, want ErrClockSkew", err)
		}
		for _, n := range []int{1, 2} {
			if _, ok, _ := w.a.Get(ctx, key(n)); ok {
				t.Fatalf("refused session %d was stored", n)
			}
		}
		if _, err := w.a.Create(ctx, at(5*time.Second, 3), lim); err != nil {
			t.Fatalf("five seconds off, bound one minute: %v", err)
		}
		// A refused login ends nothing of the subject's.
		capped := CreateLimits{MaxLive: roomy, MaxSkew: time.Minute, MaxPerSubject: 1}
		if _, err := w.a.Create(ctx, at(10*time.Minute, 4), capped); !errors.Is(err, ErrClockSkew) {
			t.Fatalf("skewed and capped: %v", err)
		}
		if got, _, _ := w.a.Get(ctx, key(3)); got.RevokedAt != nil {
			t.Fatal("a login refused for clock skew still ended the subject's older session")
		}
		// No bound asked, none applied.
		if _, err := w.a.Create(ctx, at(10*time.Minute, 5), CreateLimits{MaxLive: roomy}); err != nil {
			t.Fatalf("without a bound: %v", err)
		}
	})

	t.Run("a key is used once", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0))
		if _, err := w.a.Create(ctx, sess(w.frontA, 1, t0.Add(time.Minute)), CreateLimits{MaxLive: roomy}); err == nil {
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
			"born revoked":                   func(s *Session) { at := s.CreatedAt; s.RevokedAt, s.RevokedBy = &at, ByLogout },
			"born with a revoker":            func(s *Session) { s.RevokedBy = ByLogout },
			"a front with an underscore":     func(s *Session) { s.Front = "front_a" },
			"another front than the store's": func(s *Session) { s.Front = "front-zz" },
		} {
			s := sess(w.frontA, 1, t0)
			mod(&s)
			if _, err := w.a.Create(ctx, s, CreateLimits{MaxLive: roomy}); err == nil {
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
		if err := w.a.Revoke(ctx, revoked.KeyHash, t0.Add(2*time.Hour+time.Minute), ByLogout); err != nil {
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

	// The listing does not judge expiry: the clock that does is the
	// front's, and a session that looks expired from where the command
	// runs may be live. It shows everything not revoked, and the revoked
	// too when asked.
	t.Run("admin lists, newest first, and filters", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0))
		mustCreate(t, w.b, sessOf(w.frontB, "alice", 2, t0.Add(time.Minute)))
		mustCreate(t, w.b, sess(w.frontB, 3, t0.Add(2*time.Minute)))
		all, err := w.admin.List(ctx, Filter{})
		if err != nil || len(all) != 3 || all[0].KeyHash != key(3) || all[2].KeyHash != key(1) {
			t.Fatalf("list: %d %v", len(all), err)
		}
		if got, _ := w.admin.List(ctx, Filter{Subject: "alice"}); len(got) != 1 || got[0].Front != w.frontB {
			t.Fatalf("by subject: %+v", got)
		}
		if got, _ := w.admin.List(ctx, Filter{Front: w.frontB}); len(got) != 2 {
			t.Fatalf("by front: %d", len(got))
		}
		if got, _ := w.admin.List(ctx, Filter{Subject: "dev", Front: w.frontA}); len(got) != 1 {
			t.Fatalf("by both: %d", len(got))
		}
		// Sessions long past their expiry by any clock are still listed:
		// the caller marks them, it does not hide them.
		old := sess(w.frontA, 4, t0.Add(-48*time.Hour))
		mustCreate(t, w.a, old)
		got, _ := w.admin.List(ctx, Filter{})
		if len(got) != 4 || got[3].KeyHash != old.KeyHash || got[3].State(t0) != "expired" {
			t.Fatalf("with an expired session: %d listed", len(got))
		}
		// Revoked ones are left out unless asked for, and say who.
		if err := w.b.Revoke(ctx, key(3), t0.Add(3*time.Minute), ByLogout); err != nil {
			t.Fatal(err)
		}
		if got, _ := w.admin.List(ctx, Filter{}); len(got) != 3 {
			t.Fatalf("after a revocation: %d listed, want 3", len(got))
		}
		withRevoked, _ := w.admin.List(ctx, Filter{Revoked: true})
		if len(withRevoked) != 4 || withRevoked[0].KeyHash != key(3) || withRevoked[0].RevokedBy != ByLogout {
			t.Fatalf("with revoked: %d, first %+v", len(withRevoked), withRevoked[0])
		}
	})

	t.Run("admin revokes one session by its id, and it is recorded who did", func(t *testing.T) {
		w := fresh(t)
		s := sess(w.frontA, 1, t0)
		mustCreate(t, w.a, s)
		mustCreate(t, w.b, sess(w.frontB, 2, t0))
		now := t0.Add(time.Minute)
		got, err := w.admin.Revoke(ctx, s.ID(), now, testActor)
		if err != nil || got.KeyHash != s.KeyHash || got.RevokedAt == nil || got.RevokedBy != testActor {
			t.Fatalf("revoke: %+v %v", got, err)
		}
		if after, _, _ := w.a.Get(ctx, s.KeyHash); after.Live(now) || after.RevokedBy != testActor {
			t.Fatalf("the front's view after the revocation: %+v", after)
		}
		if after, _, _ := w.b.Get(ctx, key(2)); !after.Live(now) {
			t.Fatal("another session was revoked with it")
		}
		if _, err := w.admin.Revoke(ctx, s.ID(), now, testActor); !errors.Is(err, ErrNotFound) {
			t.Fatalf("revoking it again: %v, want ErrNotFound", err)
		}
		if _, err := w.admin.Revoke(ctx, "0123456789abcdef", now, testActor); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown id: %v", err)
		}
		for _, bad := range []string{"", "abc", "zzzzzzzz", "ABCDEF01", s.KeyHash + "0", "%%%%%%%%", "abcdef0_"} {
			if _, err := w.admin.Revoke(ctx, bad, now, testActor); !errors.Is(err, ErrInvalid) {
				t.Errorf("id %q: %v, want ErrInvalid", bad, err)
			}
		}
		if _, err := w.admin.Revoke(ctx, key(2)[:16], now, ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("a revocation by nobody: %v, want ErrInvalid", err)
		}
		if after, _, _ := w.b.Get(ctx, key(2)); !after.Live(now) {
			t.Fatal("a revocation by nobody still revoked")
		}
	})

	t.Run("admin refuses a prefix that names two sessions", func(t *testing.T) {
		w := fresh(t)
		a, b := twoKeysSharing(t)
		sa, sb := sess(w.frontA, 0, t0), sess(w.frontA, 0, t0)
		sa.KeyHash, sb.KeyHash = a, b
		mustCreate(t, w.a, sa)
		mustCreate(t, w.a, sb)
		if _, err := w.admin.Revoke(ctx, a[:MinPrefix], t0.Add(time.Minute), testActor); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("shared prefix: %v, want ErrAmbiguous", err)
		}
		if got, _, _ := w.a.Get(ctx, a); got.RevokedAt != nil {
			t.Fatal("an ambiguous revoke still revoked")
		}
		if _, err := w.admin.Revoke(ctx, a, t0.Add(time.Minute), testActor); err != nil {
			t.Fatalf("the full hash: %v", err)
		}
	})

	t.Run("admin revokes every session of a subject", func(t *testing.T) {
		w := fresh(t)
		mustCreate(t, w.a, sess(w.frontA, 1, t0))
		mustCreate(t, w.a, sess(w.frontA, 2, t0))
		mustCreate(t, w.b, sess(w.frontB, 3, t0))
		alice := sessOf(w.frontA, "alice", 4, t0)
		mustCreate(t, w.a, alice)
		now := t0.Add(time.Minute)
		if n, err := w.admin.RevokeSubject(ctx, "dev", w.frontA, now, testActor); n != 2 || err != nil {
			t.Fatalf("dev on front a: %d %v", n, err)
		}
		if got, _, _ := w.a.Get(ctx, key(1)); got.RevokedBy != testActor {
			t.Fatalf("the revocation does not say who: %+v", got)
		}
		if got, _, _ := w.b.Get(ctx, key(3)); !got.Live(now) {
			t.Fatal("front b's session of dev was revoked by a front a revocation")
		}
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", now, testActor); n != 1 || err != nil {
			t.Fatalf("dev everywhere: %d %v", n, err)
		}
		if got, _, _ := w.a.Get(ctx, alice.KeyHash); !got.Live(now) {
			t.Fatal("alice's session was revoked with dev's")
		}
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", now, testActor); n != 0 || err != nil {
			t.Fatalf("again: %d %v", n, err)
		}
		if _, err := w.admin.RevokeSubject(ctx, "", "", now, testActor); !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty subject: %v, want ErrInvalid (never every session by accident)", err)
		}
		if _, err := w.admin.RevokeSubject(ctx, "alice", "", now, "nobody at all"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("an actor a row cannot hold: %v, want ErrInvalid", err)
		}
		if got, _, _ := w.a.Get(ctx, alice.KeyHash); !got.Live(now) {
			t.Fatal("a refused revocation still revoked")
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
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", fast, testActor); n != 2 || err != nil {
			t.Fatalf("revoke subject with a fast clock: %d %v, want both sessions", n, err)
		}
		frontNow := t0.Add(time.Minute)
		for _, k := range []string{key(1), key(2)} {
			if got, _, _ := w.a.Get(ctx, k); got.Live(frontNow) || got.RevokedAt == nil {
				t.Fatalf("a session survived a revocation issued under a fast clock: %+v", got)
			}
		}
		mustCreate(t, w.a, sess(w.frontA, 3, t0))
		if _, err := w.admin.Revoke(ctx, key(3)[:16], fast, testActor); err != nil {
			t.Fatalf("revoke by id with a fast clock: %v", err)
		}
		if got, _, _ := w.a.Get(ctx, key(3)); got.Live(frontNow) {
			t.Fatal("a session survived a revocation by id issued under a fast clock")
		}
		// And a slow clock: the stamp is held to the creation time.
		mustCreate(t, w.a, sess(w.frontA, 4, t0))
		if n, err := w.admin.RevokeSubject(ctx, "dev", "", t0.Add(-10*time.Hour), testActor); n != 1 || err != nil {
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
