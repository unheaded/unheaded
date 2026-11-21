// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sessions

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMem_Suite(t *testing.T) {
	storeSuite(t, func(*testing.T) world {
		m := NewMem()
		clock := func() time.Time { return t0 }
		m.SetClock(clock)
		return world{a: m.Front("front-a"), b: m.Front("front-b"), admin: m, frontA: "front-a", frontB: "front-b", storeNow: clock}
	})
}

func TestKeyHash_IsTheSHA256AndNeverTheValue(t *testing.T) {
	const cookie = "xZ8Yw3o0pQ1rS2tU4vW5xY6zA7bC8dE9fG0hI1jK2lM"
	h := KeyHash(cookie)
	if !ValidKeyHash(h) || strings.Contains(h, cookie) {
		t.Fatalf("hash %q", h)
	}
	if h != KeyHash(cookie) || h == KeyHash(cookie+"x") {
		t.Fatal("hash is not a function of the value alone")
	}
	if ValidKeyHash(cookie) {
		t.Fatal("a cookie value passes for a hash: a store could be handed the secret")
	}
}

func TestSession_StateAndID(t *testing.T) {
	s := sess("front-a", 1, t0)
	if s.State(t0) != "active" || s.State(t0.Add(30*time.Minute)) != "expired" {
		t.Fatalf("states %q %q", s.State(t0), s.State(t0.Add(30*time.Minute)))
	}
	// The maximum age ends a session whatever its idle expiry says.
	s.ExpiresAt = s.AbsoluteExpiresAt.Add(time.Hour)
	if s.Live(s.AbsoluteExpiresAt) {
		t.Fatal("live at its maximum age")
	}
	at := t0.Add(time.Minute)
	s.RevokedAt = &at
	if s.State(t0.Add(2*time.Minute)) != "revoked" || s.Live(t0) {
		t.Fatal("a revoked session is not revoked")
	}
	if len(s.ID()) != 16 || !strings.HasPrefix(s.KeyHash, s.ID()) {
		t.Fatalf("id %q", s.ID())
	}
}

// A Mem that was given no clock has no second clock to disagree with.
func TestMem_WithoutAClockSkewIsNotChecked(t *testing.T) {
	st := NewMem().Front("front-a")
	if _, err := st.Create(context.Background(), sess("front-a", 1, t0), CreateLimits{MaxLive: 5, MaxSkew: time.Second}); err != nil {
		t.Fatalf("no clock given: %v", err)
	}
}

func TestCLIActor(t *testing.T) {
	for who, want := range map[string]string{
		"stevie@west":       "cli:stevie@west",
		"":                  "cli:unknown",
		"root user@my host": "cli:root_user@my_host",
		"a\nb;DROP":         "cli:a_b_DROP",
		"jürgen@west":       "cli:j_rgen@west",
	} {
		got := CLIActor(who)
		if got != want || !ValidActor(got) {
			t.Errorf("CLIActor(%q) = %q (valid %v), want %q", who, got, ValidActor(got), want)
		}
	}
	if long := CLIActor(strings.Repeat("a", 500)); !ValidActor(long) || len(long) > 140 {
		t.Errorf("a long name: %d characters, valid %v", len(long), ValidActor(long))
	}
	for _, by := range []string{ByLogout, ByLogin, ByLimit} {
		if !ValidActor(by) {
			t.Errorf("%q is not a valid actor", by)
		}
	}
}

func TestValidSubject(t *testing.T) {
	for _, ok := range []string{"dev", "stevie@bellis.tech", "auth0|12345", "jürgen", strings.Repeat("a", MaxSubjectLen)} {
		if !ValidSubject(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", " ", "a b", "a\tb", "a\x00b", "a b", "\xff\xfe", strings.Repeat("a", MaxSubjectLen+1)} {
		if ValidSubject(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestWellRole_NamesOneFront(t *testing.T) {
	if got := WellRole("dashboard-backend"); got != "bff_dashboard_backend" {
		t.Fatalf("role %q", got)
	}
	// Two fronts must never share a role: that is why a front name has no
	// underscore, and why it is short enough for a role name (63 bytes).
	if ValidFront("dashboard_backend") || ValidFront("") || ValidFront("-x") || ValidFront(strings.Repeat("a", 60)) {
		t.Fatal("a front name that could share or overflow a role name is accepted")
	}
	if len(WellRole(strings.Repeat("a", 59))) > 63 {
		t.Fatal("the longest front's role does not fit a role name")
	}
}

func TestMem_ConcurrentUse(t *testing.T) {
	m := NewMem()
	st := m.Front("front-a")
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := sess("front-a", i, t0)
			_, _ = st.Create(ctx, s, CreateLimits{MaxLive: 8, MaxPerSubject: 6})
			_, _, _ = st.Get(ctx, s.KeyHash)
			_, _ = st.Touch(ctx, s.KeyHash, t0.Add(time.Minute), t0.Add(31*time.Minute))
			_, _ = m.List(ctx, Filter{})
			_ = st.Revoke(ctx, s.KeyHash, t0.Add(2*time.Minute), ByLogout)
			_, _ = st.Sweep(ctx, t0.Add(time.Hour), 2)
		}(i)
	}
	wg.Wait()
	if left, _ := m.List(ctx, Filter{}); len(left) != 0 {
		t.Fatalf("%d sessions not revoked after every one was", len(left))
	}
}

// The cap holds under a burst: no more than maxLive are created.
func TestMem_CapHoldsUnderConcurrentCreates(t *testing.T) {
	m := NewMem()
	st := m.Front("front-a")
	var wg sync.WaitGroup
	var mu sync.Mutex
	full := 0
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := st.Create(context.Background(), sess("front-a", i, t0), CreateLimits{MaxLive: 5}); errors.Is(err, ErrFull) {
				mu.Lock()
				full++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if m.Len() != 5 || full != 27 {
		t.Fatalf("%d created, %d refused; want 5 and 27", m.Len(), full)
	}
}
