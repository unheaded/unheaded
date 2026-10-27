// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/auth"
)

// FuzzLoginBody: no body panics the login handler or earns a token.
func FuzzLoginBody(f *testing.F) {
	for _, seed := range []string{
		`{"username":"dev","password":"wrong"}`,
		`{"username":"dev","password":"x","audience":"unheaded"}`,
		`{"username":"","password":""}`,
		`{"username":"dev"}`,
		`[]`, `{`, `null`, `{"username":1,"password":true}`,
		`{"username":"dev","password":"x"}{}`,
		`{"username":"` + strings.Repeat("a", 70) + `","password":"x"}`,
	} {
		f.Add(seed)
	}
	fx := newFuzzFixture(f)
	iss, err := newIssuer(issuerConfig{IssuerURL: "http://127.0.0.1:19020", KeyFile: fx.keyFile, UsersFile: fx.usersFile, DevPlaintext: true})
	if err != nil {
		f.Fatal(err)
	}
	h := iss.Handler()
	f.Fuzz(func(t *testing.T, body string) {
		r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == http.StatusOK {
			t.Fatalf("body %q earned a token", body)
		}
		if strings.Contains(w.Body.String(), "access_token") {
			t.Fatalf("body %q: token in a %d response", body, w.Code)
		}
	})
}

// FuzzUsersFile: no users file panics the parser, and whatever it accepts
// holds only valid names, bounded hashes and allowed roles.
func FuzzUsersFile(f *testing.F) {
	h, err := hashPassword("pw", testParams)
	if err != nil {
		f.Fatal(err)
	}
	good, _ := json.Marshal(usersFile{Users: []userRecord{{Username: "dev", Hash: h, Roles: []string{"operator"}}}})
	for _, seed := range [][]byte{
		good,
		[]byte(`{"users":[]}`),
		[]byte(`{"users":[{"username":"a","argon2id_hash":"$argon2id$v=19$m=4194304,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","roles":["admin"]}]}`),
		[]byte(`{"users":[{"username":"a","argon2id_hash":"x","roles":["service"]}]}`),
		[]byte(`{"users":null,"extra":1}`),
		[]byte(`{}`), []byte(``), []byte(`[`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		users, err := parseUsers(raw)
		if err != nil {
			return
		}
		for name, u := range users {
			if !validUsername(name) || u.name != name {
				t.Fatalf("accepted bad username %q", name)
			}
			if u.hash.p.Memory > maxArgonMemoryKiB || u.hash.p.Time > maxArgonTime || u.hash.p.Threads > maxArgonThreads {
				t.Fatalf("accepted unbounded params %+v", u.hash.p)
			}
			if len(u.roles) == 0 {
				t.Fatalf("user %q with no roles", name)
			}
			for _, r := range u.roles {
				if !allowedRoles[r] {
					t.Fatalf("accepted role %q", r)
				}
			}
		}
	})
}

type fuzzFixture struct{ keyFile, usersFile string }

// newFuzzFixture is newFixture for *testing.F.
func newFuzzFixture(f *testing.F) fuzzFixture {
	f.Helper()
	dir := f.TempDir()
	fx := fuzzFixture{keyFile: filepath.Join(dir, "unheaded-auth.key"), usersFile: filepath.Join(dir, "users.json")}
	if _, err := auth.GenerateKeyPair(dir, "unheaded-auth"); err != nil {
		f.Fatal(err)
	}
	h, err := hashPassword("correct horse battery", testParams)
	if err != nil {
		f.Fatal(err)
	}
	raw, _ := json.Marshal(usersFile{Users: []userRecord{{Username: "dev", Hash: h, Roles: []string{"operator"}}}})
	if err := os.WriteFile(fx.usersFile, raw, 0o600); err != nil {
		f.Fatal(err)
	}
	return fx
}
