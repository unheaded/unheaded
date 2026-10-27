// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"unheaded/pkg/auth"
)

// RT-03 (S-CHAMPION-HARDENING Step 46): a pending token is bound to the
// authenticated principal that caused it ("ui:"+subject); only that
// principal redeems it, and an unauthenticated daemon confirms nothing.

type subjectAuth string

func (s subjectAuth) Authenticate(context.Context, *http.Request) (*auth.Identity, error) {
	return &auth.Identity{Subject: string(s)}, nil
}

// testAuth authenticates every request as subject "stevie": the
// end-to-end test daemons run as one signed-in user, so a token they
// are issued is theirs to confirm.
func testAuth(h http.Handler) http.Handler {
	return auth.Middleware(subjectAuth("stevie"))(h)
}

// as serves h as subject ("" = no auth middleware, the -no-auth daemon).
func as(subject string, h http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	var handler http.Handler = h
	if subject != "" {
		handler = auth.Middleware(subjectAuth(subject))(h)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(b)))
	return rec
}

func issueToken(t *testing.T, srv *server, root, subject string) string {
	t.Helper()
	rec := as(subject, srv.handleToolExec, http.MethodPost, "/api/v1/tool/exec", map[string]any{
		"tool": "write_file", "args": map[string]any{"path": filepath.Join(root, "a.txt"), "content": "x"},
		"justification": untrustedRefs(),
	})
	var resp toolExecResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != http.StatusForbidden || resp.PendingToken == "" {
		t.Fatalf("tool/exec as %q: %d %s", subject, rec.Code, rec.Body.String())
	}
	return resp.PendingToken
}

func confirmAs(srv *server, subject, token string) (int, confirmResponse) {
	rec := as(subject, srv.handleConfirm, http.MethodPost, "/api/v1/agent/confirm", map[string]any{"token": token})
	var resp confirmResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func TestConfirm_TokenBoundToRequester(t *testing.T) {
	srv, root := toolExecServer(t, nil)
	tok := issueToken(t, srv, root, "stevie")
	code, resp := confirmAs(srv, "mallory", tok)
	if code != http.StatusForbidden || resp.Status != "denied" {
		t.Fatalf("another principal redeemed the token: %d %+v", code, resp)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("the borrowed token wrote the file")
	}
	if code, resp := confirmAs(srv, "stevie", tok); code != http.StatusOK {
		t.Fatalf("the requester's own confirm: %d %+v", code, resp)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
		t.Fatalf("file not written after the requester confirmed: %v", err)
	}
}

func TestConfirm_RefusedWithoutAuthenticatedPrincipal(t *testing.T) {
	srv, root := toolExecServer(t, nil)
	tok := issueToken(t, srv, root, "")
	code, resp := confirmAs(srv, "", tok)
	if code != http.StatusForbidden || !strings.Contains(resp.Reason, "authenticated principal") {
		t.Fatalf("an unauthenticated confirm: %d %+v", code, resp)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("an unauthenticated confirm wrote the file")
	}
}

func TestAskLegacyToken_BoundToCaller(t *testing.T) {
	srv, root := askServer(t, nil, true)
	rec := as("stevie", srv.handleAsk, http.MethodPost, "/api/v1/agent/ask", map[string]any{"goal": "write a.txt"})
	m := regexp.MustCompile(`"pending_token":"([0-9a-f]+)"`).FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("ask: %d %s", rec.Code, rec.Body.String())
	}
	if code, _ := confirmAs(srv, "mallory", m[1]); code != http.StatusForbidden {
		t.Fatalf("another principal redeemed an /ask token: %d", code)
	}
	if code, resp := confirmAs(srv, "stevie", m[1]); code != http.StatusOK {
		t.Fatalf("the asker's own confirm: %d %+v", code, resp)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}
