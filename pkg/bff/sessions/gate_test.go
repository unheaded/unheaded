// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sessions

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"

	"unheaded/pkg/database"
)

var (
	errDown    = errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")
	errRefused = errors.New("password authentication failed")
)

func isRefused(err error) bool { return errors.Is(err, errRefused) }

// closeCounting is a front's Mem store that records its Close.
type closeCounting struct {
	Store
	closed atomic.Int32
}

func (c *closeCounting) Close() error { c.closed.Add(1); return nil }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGate_AWellThatAnswersIsUsedAtOnce(t *testing.T) {
	inner := &closeCounting{Store: NewMem().Front("front-a")}
	g, err := newGate(context.Background(), "front-a", func(context.Context) (Store, error) { return inner, nil }, isRefused, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Create(context.Background(), sess("front-a", 1, t0), 10); err != nil {
		t.Fatalf("create through the gate: %v", err)
	}
	if _, ok, err := g.Get(context.Background(), key(1)); !ok || err != nil {
		t.Fatalf("get through the gate: %v %v", ok, err)
	}
	if err := g.Close(); err != nil || inner.closed.Load() != 1 {
		t.Fatalf("close: %v, inner closed %d times", err, inner.closed.Load())
	}
}

// A Well that says no at the start stops the front from starting.
func TestGate_ARefusalAtTheStartIsAnError(t *testing.T) {
	g, err := newGate(context.Background(), "front-a", func(context.Context) (Store, error) { return nil, fmt.Errorf("as bff_front_a: %w", errRefused) }, isRefused, time.Hour, time.Hour)
	if !errors.Is(err, errRefused) || g != nil {
		t.Fatalf("got %v, %v; want the refusal and no store", g, err)
	}
}

// A Well that is not there is waited for, and until it comes nothing is
// accepted: every call is ErrUnavailable, never an empty success.
func TestGate_AWellThatIsDownFailsClosedUntilItArrives(t *testing.T) {
	inner := &closeCounting{Store: NewMem().Front("front-a")}
	var up atomic.Bool
	var calls atomic.Int32
	connect := func(context.Context) (Store, error) {
		calls.Add(1)
		if !up.Load() {
			return nil, errDown
		}
		return inner, nil
	}
	g, err := newGate(context.Background(), "front-a", connect, isRefused, time.Millisecond, 4*time.Millisecond)
	if err != nil {
		t.Fatalf("a Well that is down must not stop the start: %v", err)
	}
	defer g.Close()
	ctx := context.Background()
	if err := g.Create(ctx, sess("front-a", 1, t0), 10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("create while down: %v", err)
	}
	if _, ok, err := g.Get(ctx, key(1)); ok || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("get while down: %v %v", ok, err)
	}
	if ok, err := g.Touch(ctx, key(1), t0, t0.Add(time.Minute)); ok || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("touch while down: %v %v", ok, err)
	}
	if err := g.Revoke(ctx, key(1), t0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revoke while down: %v", err)
	}
	if n, err := g.Sweep(ctx, t0, 10); n != 0 || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sweep while down: %d %v", n, err)
	}
	waitFor(t, "a retry", func() bool { return calls.Load() >= 3 })
	up.Store(true)
	waitFor(t, "the Well to be attached", func() bool { return g.Create(ctx, sess("front-a", 1, t0), 10) == nil })
	if _, ok, err := g.Get(ctx, key(1)); !ok || err != nil {
		t.Fatalf("get after the Well arrived: %v %v", ok, err)
	}
}

// A refusal met while waiting cannot stop the process any more; the gate
// stays shut and keeps asking.
func TestGate_ARefusalWhileWaitingKeepsTheGateShut(t *testing.T) {
	var mu sync.Mutex
	answers := []error{errDown, errRefused, errRefused}
	var calls atomic.Int32
	connect := func(context.Context) (Store, error) {
		mu.Lock()
		defer mu.Unlock()
		calls.Add(1)
		if len(answers) > 1 {
			e := answers[0]
			answers = answers[1:]
			return nil, e
		}
		return nil, answers[0]
	}
	g, err := newGate(context.Background(), "front-a", connect, isRefused, time.Millisecond, 2*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "retries after the refusal", func() bool { return calls.Load() >= 4 })
	if _, ok, err := g.Get(context.Background(), key(1)); ok || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("get behind a refusing Well: %v %v", ok, err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	n := calls.Load()
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != n {
		t.Fatal("the gate kept dialling after Close")
	}
}

func TestGate_CloseWhileWaitingStopsAndClosesALateStore(t *testing.T) {
	release := make(chan struct{})
	inner := &closeCounting{Store: NewMem().Front("front-a")}
	var calls atomic.Int32
	connect := func(context.Context) (Store, error) {
		if calls.Add(1) == 1 {
			return nil, errDown
		}
		<-release // the dial is in flight when Close comes
		return inner, nil
	}
	g, err := newGate(context.Background(), "front-a", connect, isRefused, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the second dial", func() bool { return calls.Load() >= 2 })
	done := make(chan error, 1)
	go func() { done <- g.Close() }()
	time.Sleep(5 * time.Millisecond)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if inner.closed.Load() != 1 {
		t.Fatalf("a store that arrived during Close was closed %d times, want 1", inner.closed.Load())
	}
	if _, _, err := g.Get(context.Background(), key(1)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("get after Close: %v", err)
	}
}

// What counts as the Well saying no, and what as the Well not being there.
func TestRefusedByWell(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"wrong password":                    {&pq.Error{Code: "28P01"}, true},
		"no such role":                      {&pq.Error{Code: "28000"}, true},
		"no such database":                  {&pq.Error{Code: "3D000"}, true},
		"view missing (un-migrated)":        {fmt.Errorf("sessions: not readable: %w", &pq.Error{Code: "42P01"}), true},
		"no grant on the view":              {&pq.Error{Code: "42501"}, true},
		"no password configured":            {fmt.Errorf("sessions: %w", database.ErrMissingCredentials), true},
		"weak sslmode":                      {database.ErrWeakTLS, true},
		"unusable CA":                       {database.ErrMissingCA, true},
		"a front name the Well cannot hold": {fmt.Errorf("%w: front", ErrInvalid), true},
		"server starting up":                {&pq.Error{Code: "57P03"}, false},
		"server shutting down":              {&pq.Error{Code: "57P01"}, false},
		"too many connections":              {&pq.Error{Code: "53300"}, false},
		"connection failure":                {&pq.Error{Code: "08006"}, false},
		"connection refused":                {&net.OpError{Op: "dial", Err: errors.New("connection refused")}, false},
		"deadline":                          {context.DeadlineExceeded, false},
	} {
		if got := refusedByWell(tc.err); got != tc.want {
			t.Errorf("%s: refused = %v, want %v", name, got, tc.want)
		}
	}
}

func TestOpenWell_RefusesAFrontNameTheWellCannotHold(t *testing.T) {
	if _, err := OpenWell(context.Background(), "dashboard_backend"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

// No password is a configuration error, found before any connection.
func TestOpenWell_WithoutAPasswordRefusesToStart(t *testing.T) {
	t.Setenv("AUTH_SESSION_WELL_PASSWORD", "")
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")
	_, err := OpenWell(context.Background(), "front-a")
	if !errors.Is(err, database.ErrMissingCredentials) {
		t.Fatalf("got %v, want ErrMissingCredentials", err)
	}
}

// A Well that is not listening does not stop the start, and nothing is
// accepted meanwhile.
func TestOpenWell_AWellThatIsNotListeningIsWaitedFor(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close() // nothing listens there now
	t.Setenv("AUTH_SESSION_WELL_PASSWORD", "not-a-real-password")
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", port)
	t.Setenv("WELL_SSLMODE", "disable")
	st, err := OpenWell(context.Background(), "front-a")
	if err != nil {
		t.Fatalf("a Well that is down stopped the start: %v", err)
	}
	defer st.Close()
	if _, ok, err := st.Get(context.Background(), key(1)); ok || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("get with no Well: %v %v", ok, err)
	}
	if err := st.Create(context.Background(), sess("front-a", 1, t0), 10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("create with no Well: %v", err)
	}
}
