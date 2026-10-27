// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package cmd

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/auth"
)

// `unheaded token --aud <service>` prints the CLI's service token for a
// shell call (curl -H "Authorization: Bearer $(unheaded token --aud wotan)").
func TestToken_PrintsCLITokenTheServiceAccepts(t *testing.T) {
	keys, trust := t.TempDir(), t.TempDir()
	if _, err := auth.GenerateKeyPair(keys, cliKeyName); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(keys, cliKeyName+".pub"), filepath.Join(trust, cliKeyName+".pub")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_KEY_FILE", filepath.Join(keys, cliKeyName+".key"))
	// UNHEADED_TOKEN is a credential for calls the CLI makes; `token`
	// always mints from the key.
	t.Setenv("UNHEADED_TOKEN", "uhk_not-printed")

	ctx := testContext(DefaultConfig())
	if err := run(t, ctx, NewTokenCommand(), "", "--aud", "wotan"); err != nil {
		t.Fatal(err)
	}
	tok := strings.TrimSpace(testBuf.String())
	if strings.Contains(tok, "uhk_") || strings.Count(tok, ".") != 2 || strings.ContainsAny(tok, " \n") {
		t.Fatalf("output is not one bare token: %q", tok)
	}

	td, err := auth.NewTrustDir(trust)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.NewEdDSAVerifier(auth.EdDSAVerifierConfig{
		Trust: td, Issuer: auth.EdDSAServiceIssuer, Audience: "wotan", AllowCallers: []string{cliKeyName},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://wotan/api/v1/topics", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	id, err := v.Authenticate(context.Background(), r)
	if err != nil || id.Subject != cliKeyName {
		t.Fatalf("Wotan-side verify: identity %+v, err %v", id, err)
	}
}

func TestToken_Refusals(t *testing.T) {
	keys := t.TempDir()
	if _, err := auth.GenerateKeyPair(keys, cliKeyName); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_KEY_FILE", filepath.Join(keys, cliKeyName+".key"))
	ctx := testContext(DefaultConfig())

	for _, args := range [][]string{{}, {"--aud", ""}, {"--aud", "Wotan"}, {"--aud", "a.b"}, {"--aud", "wotan", "extra"}} {
		if err := run(t, ctx, NewTokenCommand(), "", args...); err == nil {
			t.Errorf("token %v: want an error", args)
		}
	}

	// No key anywhere: an error naming the fix, never an empty token.
	t.Setenv("AUTH_KEY_FILE", "")
	t.Setenv("HOME", t.TempDir())
	prev := cliHostKeyFile
	cliHostKeyFile = filepath.Join(t.TempDir(), "absent.key")
	t.Cleanup(func() { cliHostKeyFile = prev })
	testBuf.Reset()
	err := run(t, ctx, NewTokenCommand(), "", "--aud", "wotan")
	if err == nil || !strings.Contains(err.Error(), "unheaded-keygen") {
		t.Errorf("no key: err %v, want one naming unheaded-keygen", err)
	}
	if testBuf.Len() != 0 {
		t.Errorf("no key printed %q", testBuf.String())
	}

	// A key file that cannot be used is an error too.
	open := filepath.Join(t.TempDir(), "cli.key")
	if err := os.WriteFile(open, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_KEY_FILE", open)
	if err := run(t, ctx, NewTokenCommand(), "", "--aud", "wotan"); err == nil {
		t.Error("group-readable key: want an error")
	}
}
