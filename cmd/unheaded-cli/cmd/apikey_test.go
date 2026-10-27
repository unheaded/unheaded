// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/auth"
)

// withMemStore points the apikey commands at one in-memory store.
func withMemStore(t *testing.T) auth.APIKeyStore {
	t.Helper()
	store := auth.NewMemAPIKeyStore()
	prev := openAPIKeyStore
	openAPIKeyStore = func(context.Context) (auth.APIKeyStore, string, error) { return store, "mem", nil }
	t.Cleanup(func() { openAPIKeyStore = prev })
	return store
}

var keyRE = regexp.MustCompile(`uhk_[A-Za-z0-9_-]{43}`)

func TestAPIKey_CreatePrintsKeyOnceAndStoresHash(t *testing.T) {
	store := withMemStore(t)
	ctx := testContext(DefaultConfig())
	root := NewAPIKeyCommand()

	if err := run(t, ctx, root, "create", "--owner", "ci-github", "--role", "operator", "--expires", "30d"); err != nil {
		t.Fatal(err)
	}
	out := testBuf.String()
	keys := keyRE.FindAllString(out, -1)
	if len(keys) != 1 {
		t.Fatalf("want the key printed exactly once, got %d in:\n%s", len(keys), out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], "store this now; it is not shown again") {
		t.Errorf("first line %q lacks the warning", lines[0])
	}
	id, err := store.Authenticate(context.Background(), keys[0])
	if err != nil || id.Subject != "ci-github" || !id.HasRole("operator") {
		t.Fatalf("printed key does not authenticate as ci-github/operator: %+v %v", id, err)
	}
	list, _ := store.List(context.Background())
	if len(list) != 1 {
		t.Fatalf("records: %+v", list)
	}
	if d := time.Until(list[0].ExpiresAt); d < 29*24*time.Hour || d > 30*24*time.Hour+time.Minute {
		t.Errorf("--expires 30d gave %s", d)
	}

	// list shows the prefix, never the key.
	testBuf.Reset()
	if err := run(t, ctx, root, "list"); err != nil {
		t.Fatal(err)
	}
	if keyRE.MatchString(testBuf.String()) || !strings.Contains(testBuf.String(), list[0].Prefix) ||
		!strings.Contains(testBuf.String(), "ci-github") {
		t.Errorf("list output:\n%s", testBuf.String())
	}
}

func TestAPIKey_CreateDefaultsAndRefusals(t *testing.T) {
	store := withMemStore(t)
	ctx := testContext(DefaultConfig())
	root := NewAPIKeyCommand()

	if err := run(t, ctx, root, "create", "--owner", "bot"); err != nil {
		t.Fatal(err)
	}
	list, _ := store.List(context.Background())
	if len(list) != 1 || list[0].Role != auth.RoleService {
		t.Fatalf("default role: %+v", list)
	}
	if d := time.Until(list[0].ExpiresAt); d < auth.DefaultAPIKeyLifetime-time.Minute || d > auth.DefaultAPIKeyLifetime {
		t.Errorf("default expiry %s", d)
	}

	for name, args := range map[string][]string{
		"no owner":        {"--role", "service"},
		"unknown role":    {"--owner", "x", "--role", "root"},
		"bad expiry":      {"--owner", "x", "--expires", "soon"},
		"zero expiry":     {"--owner", "x", "--expires", "0d"},
		"past max":        {"--owner", "x", "--expires", "400d"},
		"stray argument":  {"--owner", "x", "extra"},
		"negative expiry": {"--owner", "x", "--expires", "-5d"},
	} {
		root := NewAPIKeyCommand()
		if err := run(t, ctx, root, "create", args...); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if list, _ := store.List(context.Background()); len(list) != 1 {
		t.Errorf("refused creates stored keys: %d", len(list))
	}
}

func TestAPIKey_RevokeByPrefix(t *testing.T) {
	store := withMemStore(t)
	ctx := testContext(DefaultConfig())
	key, rec, err := store.Create(context.Background(), "ci", auth.RoleService, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	root := NewAPIKeyCommand()
	if err := run(t, ctx, root, "revoke", rec.Prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(context.Background(), key); !errors.Is(err, auth.ErrAPIKeyRevoked) {
		t.Errorf("key still works after revoke: %v", err)
	}
	if !strings.Contains(testBuf.String(), rec.Prefix) {
		t.Errorf("revoke output:\n%s", testBuf.String())
	}
	if err := run(t, ctx, NewAPIKeyCommand(), "revoke", "uhk_nothere0"); !errors.Is(err, auth.ErrAPIKeyNotFound) {
		t.Errorf("unknown prefix: %v", err)
	}
	if err := run(t, ctx, NewAPIKeyCommand(), "revoke"); err == nil {
		t.Error("revoke with no argument accepted")
	}

	// JSON list carries revoked_at and no key material.
	testBuf.Reset()
	ctx.Output.SetFormat(output.FormatJSON)
	if err := run(t, ctx, NewAPIKeyCommand(), "list"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Keys []auth.APIKeyRecord `json:"keys"`
	}
	if err := json.Unmarshal(testBuf.Bytes(), &got); err != nil {
		t.Fatalf("list -o json: %v\n%s", err, testBuf.String())
	}
	if len(got.Keys) != 1 || got.Keys[0].RevokedAt == nil {
		t.Errorf("json list: %+v", got.Keys)
	}
	if keyRE.Match(testBuf.Bytes()) {
		t.Error("json list carries a key")
	}
}

func TestParseKeyExpiry(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"90d":        now.Add(90 * 24 * time.Hour),
		"12h":        now.Add(12 * time.Hour),
		"2026-12-31": time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	} {
		got, err := parseKeyExpiry(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("%q: %v %v, want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "d", "1.5d", "x", "-1d", "0h", "2026-13-01"} {
		if _, err := parseKeyExpiry(bad, now); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestRoot_HasAPIKeyCommand(t *testing.T) {
	if _, ok := NewRootCommand().SubCommands["apikey"]; !ok {
		t.Fatal("unheaded apikey is not registered")
	}
}
