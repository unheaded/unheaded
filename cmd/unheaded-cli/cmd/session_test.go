// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/bff/sessions"
)

// withMemSessions points the session commands at one in-memory set of
// sessions, as the Well's table would hold them.
func withMemSessions(t *testing.T) *sessions.Mem {
	t.Helper()
	mem := sessions.NewMem()
	prev := openSessionAdmin
	openSessionAdmin = func(context.Context) (sessions.Admin, error) { return mem, nil }
	t.Cleanup(func() { openSessionAdmin = prev })
	return mem
}

// seed logs subject in to front and returns the session.
func seed(t *testing.T, mem *sessions.Mem, front, subject, cookie string, age time.Duration, roles ...string) sessions.Session {
	t.Helper()
	created := time.Now().Add(-age)
	s := sessions.Session{
		KeyHash: sessions.KeyHash(cookie), Front: front, Subject: subject, Roles: roles,
		CreatedAt: created, LastUsedAt: created,
		ExpiresAt: created.Add(30 * time.Minute), AbsoluteExpiresAt: created.Add(12 * time.Hour),
	}
	if err := mem.Front(front).Create(context.Background(), s, 100); err != nil {
		t.Fatal(err)
	}
	return s
}

func liveCount(t *testing.T, mem *sessions.Mem, f sessions.Filter) int {
	t.Helper()
	list, err := mem.List(context.Background(), f, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

func TestSession_ListShowsWhoWhereAndStateNeverTheCookie(t *testing.T) {
	mem := withMemSessions(t)
	const cookie = "kJ3xQ9vT2mN8bV5cX1zL7pR4sD6fG0hA2wE9yU3iO5t"
	a := seed(t, mem, "dashboard-backend", "dev", cookie, time.Minute, "operator", "viewer")
	seed(t, mem, "kanban-app", "alice", "another-cookie-value", 2*time.Minute, "admin")
	old := seed(t, mem, "wiki-server", "bob", "an-expired-one", time.Hour, "viewer")
	ctx := testContext(DefaultConfig())

	if err := run(t, ctx, NewSessionCommand(), "list"); err != nil {
		t.Fatal(err)
	}
	out := testBuf.String()
	for _, want := range []string{a.ID(), "dev", "dashboard-backend", "alice", "kanban-app", "active"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, cookie) || strings.Contains(out, a.KeyHash) {
		t.Errorf("list shows a cookie value or a whole hash:\n%s", out)
	}
	if strings.Contains(out, "bob") || strings.Contains(out, old.ID()) {
		t.Errorf("list shows an expired session without --all:\n%s", out)
	}

	testBuf.Reset()
	if err := run(t, ctx, NewSessionCommand(), "list", "--all"); err != nil {
		t.Fatal(err)
	}
	if out := testBuf.String(); !strings.Contains(out, "bob") || !strings.Contains(out, "expired") {
		t.Errorf("list --all lacks the expired session:\n%s", out)
	}

	testBuf.Reset()
	if err := run(t, ctx, NewSessionCommand(), "list", "--subject", "alice"); err != nil {
		t.Fatal(err)
	}
	if out := testBuf.String(); !strings.Contains(out, "alice") || strings.Contains(out, "dev") {
		t.Errorf("list --subject alice:\n%s", out)
	}

	testBuf.Reset()
	if err := run(t, ctx, NewSessionCommand(), "list", "--front", "dashboard-backend"); err != nil {
		t.Fatal(err)
	}
	if out := testBuf.String(); !strings.Contains(out, "dev") || strings.Contains(out, "alice") {
		t.Errorf("list --front dashboard-backend:\n%s", out)
	}

	if err := run(t, ctx, NewSessionCommand(), "list", "extra"); err == nil {
		t.Error("list accepted an operand")
	}
}

func TestSession_ListJSONCarriesNoHashBeyondTheID(t *testing.T) {
	mem := withMemSessions(t)
	s := seed(t, mem, "dashboard-backend", "dev", "cookie-one", time.Minute, "operator")
	ctx := testContext(DefaultConfig())
	ctx.Output.SetFormat(output.FormatJSON)
	if err := run(t, ctx, NewSessionCommand(), "list"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.Unmarshal(testBuf.Bytes(), &got); err != nil || len(got.Sessions) != 1 {
		t.Fatalf("json: %v\n%s", err, testBuf.String())
	}
	row := got.Sessions[0]
	if row["id"] != s.ID() || row["subject"] != "dev" || row["front"] != "dashboard-backend" || row["state"] != "active" {
		t.Errorf("row: %v", row)
	}
	if strings.Contains(testBuf.String(), s.KeyHash) {
		t.Errorf("json carries the whole hash:\n%s", testBuf.String())
	}

	// An empty table is an empty list, not null.
	empty := withMemSessions(t)
	_ = empty
	ctx = testContext(DefaultConfig())
	ctx.Output.SetFormat(output.FormatJSON)
	if err := run(t, ctx, NewSessionCommand(), "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(testBuf.String(), " ", ""), `"sessions":[]`) {
		t.Errorf("empty list:\n%s", testBuf.String())
	}
}

func TestSession_RevokeOneByID(t *testing.T) {
	mem := withMemSessions(t)
	a := seed(t, mem, "dashboard-backend", "dev", "cookie-one", time.Minute, "operator")
	b := seed(t, mem, "dashboard-backend", "dev", "cookie-two", time.Minute, "operator")
	ctx := testContext(DefaultConfig())

	if err := run(t, ctx, NewSessionCommand(), "revoke", a.ID()); err != nil {
		t.Fatal(err)
	}
	if out := testBuf.String(); !strings.Contains(out, a.ID()) || !strings.Contains(out, "dev") || !strings.Contains(out, "dashboard-backend") {
		t.Errorf("revoke output:\n%s", out)
	}
	list, _ := mem.List(context.Background(), sessions.Filter{}, time.Now())
	if len(list) != 1 || list[0].KeyHash != b.KeyHash {
		t.Fatalf("after revoke: %d live", len(list))
	}

	// Again, or an id that is nobody's: an error, and it says how to look.
	err := run(t, ctx, NewSessionCommand(), "revoke", a.ID())
	if !errors.Is(err, sessions.ErrNotFound) {
		t.Errorf("revoking it twice: %v", err)
	}
	for _, args := range [][]string{{}, {"abc"}, {a.ID(), b.ID()}, {"not-hex-at-all!"}, {strings.Repeat("a", 65)}} {
		if err := run(t, ctx, NewSessionCommand(), "revoke", args...); err == nil {
			t.Errorf("revoke %q accepted", args)
		}
	}
	if liveCount(t, mem, sessions.Filter{}) != 1 {
		t.Fatal("a refused revoke ended a session")
	}
}

func TestSession_RevokeAllForASubject(t *testing.T) {
	mem := withMemSessions(t)
	seed(t, mem, "dashboard-backend", "dev", "c1", time.Minute, "operator")
	seed(t, mem, "kanban-app", "dev", "c2", time.Minute, "operator")
	seed(t, mem, "wiki-server", "dev", "c3", time.Minute, "operator")
	seed(t, mem, "dashboard-backend", "alice", "c4", time.Minute, "admin")
	ctx := testContext(DefaultConfig())

	// One front first.
	if err := run(t, ctx, NewSessionCommand(), "revoke-all", "--subject", "dev", "--front", "kanban-app"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(testBuf.String(), "1 session") {
		t.Errorf("output:\n%s", testBuf.String())
	}
	if n := liveCount(t, mem, sessions.Filter{Subject: "dev"}); n != 2 {
		t.Fatalf("dev has %d live sessions, want 2", n)
	}
	// Then everywhere.
	testBuf.Reset()
	if err := run(t, ctx, NewSessionCommand(), "revoke-all", "--subject", "dev"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(testBuf.String(), "2 sessions") {
		t.Errorf("output:\n%s", testBuf.String())
	}
	if n := liveCount(t, mem, sessions.Filter{Subject: "dev"}); n != 0 {
		t.Fatalf("dev still has %d live sessions", n)
	}
	if n := liveCount(t, mem, sessions.Filter{Subject: "alice"}); n != 1 {
		t.Fatalf("alice has %d live sessions, want 1", n)
	}
	// Nothing to revoke is said, not an error.
	testBuf.Reset()
	if err := run(t, ctx, NewSessionCommand(), "revoke-all", "--subject", "dev"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(testBuf.String(), "0 sessions") {
		t.Errorf("output:\n%s", testBuf.String())
	}
}

// Every session of everyone is never one mistyped command away.
func TestSession_RevokeAllNeedsASubject(t *testing.T) {
	mem := withMemSessions(t)
	seed(t, mem, "dashboard-backend", "dev", "c1", time.Minute, "operator")
	ctx := testContext(DefaultConfig())
	for _, args := range [][]string{{}, {"--subject", ""}, {"--front", "dashboard-backend"}, {"--subject", "dev", "extra"}, {"--subject", "de v"}} {
		if err := run(t, ctx, NewSessionCommand(), "revoke-all", args...); err == nil {
			t.Errorf("revoke-all %q accepted", args)
		}
	}
	if liveCount(t, mem, sessions.Filter{}) != 1 {
		t.Fatal("a refused revoke-all ended a session")
	}
}

func TestSession_StoreErrorsAreReported(t *testing.T) {
	prev := openSessionAdmin
	openSessionAdmin = func(context.Context) (sessions.Admin, error) {
		return nil, fmt.Errorf("sessions: the Well, as app_auth (set AUTH_WELL_PASSWORD): %w", errors.New("connection refused"))
	}
	t.Cleanup(func() { openSessionAdmin = prev })
	ctx := testContext(DefaultConfig())
	for _, c := range [][]string{{"list"}, {"revoke", "0123456789abcdef"}, {"revoke-all", "--subject", "dev"}} {
		err := run(t, ctx, NewSessionCommand(), c[0], c[1:]...)
		if err == nil || !strings.Contains(err.Error(), "AUTH_WELL_PASSWORD") {
			t.Errorf("%v: %v, want the store's error", c, err)
		}
	}
}

func TestSession_IsARootCommandWithCompletions(t *testing.T) {
	root := NewRootCommand()
	c, ok := root.SubCommands["session"]
	if !ok {
		t.Fatal("unheaded has no session command")
	}
	for _, sub := range []string{"list", "revoke", "revoke-all"} {
		if _, ok := c.SubCommands[sub]; !ok {
			t.Errorf("session has no %s", sub)
		}
	}
	for name, script := range map[string]string{"bash": generateBashCompletion(), "zsh": generateZshCompletion(), "fish": generateFishCompletion()} {
		if !strings.Contains(script, "session") || !strings.Contains(script, "revoke-all") {
			t.Errorf("%s completion does not know session revoke-all", name)
		}
	}
}
