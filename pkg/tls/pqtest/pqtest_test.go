// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqtest

import (
	"crypto/tls"
	"fmt"
	"strings"
	"testing"

	pqtls "unheaded/pkg/tls"
)

// fakeTB records failures instead of stopping the test, so the helper's own
// failure path can be asserted (ADR-097: evidence must be able to fail).
type fakeTB struct {
	testing.TB
	failed bool
	msg    string
}

func (f *fakeTB) Helper() {}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}

func TestRequireHybridFailsOnClassical(t *testing.T) {
	f := &fakeTB{}
	RequireHybrid(f, tls.ConnectionState{HandshakeComplete: true, Version: tls.VersionTLS13, CurveID: tls.X25519})
	if !f.failed {
		t.Fatal("RequireHybrid passed an X25519 session")
	}
	if !strings.Contains(f.msg, "X25519") {
		t.Errorf("failure message %q does not name the group", f.msg)
	}
}

func TestRequireHybridFailsOnTLS12AndIncomplete(t *testing.T) {
	for name, cs := range map[string]tls.ConnectionState{
		"tls12":      {HandshakeComplete: true, Version: tls.VersionTLS12, CurveID: tls.X25519MLKEM768},
		"incomplete": {CurveID: tls.X25519MLKEM768},
	} {
		f := &fakeTB{}
		RequireHybrid(f, cs)
		if !f.failed {
			t.Errorf("%s: RequireHybrid passed", name)
		}
	}
}

func TestHandshakeStrictNegotiatesHybrid(t *testing.T) {
	cert := SelfSigned(t)
	srv := pqtls.ApplyStrict(&tls.Config{Certificates: []tls.Certificate{cert}})
	cli := &tls.Config{RootCAs: Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS13}
	cs, err := Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	RequireHybrid(t, cs)
	if cs.CurveID != tls.X25519MLKEM768 {
		t.Errorf("group = %v, want X25519MLKEM768", cs.CurveID)
	}
}

func TestHandshakeReportsAFailure(t *testing.T) {
	cert := SelfSigned(t)
	srv := pqtls.ApplyStrict(&tls.Config{Certificates: []tls.Certificate{cert}})
	cli := &tls.Config{RootCAs: Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519}}
	if _, err := Handshake(t, srv, cli); err == nil {
		t.Fatal("a classical-only client completed a handshake with a Strict server")
	}
}

func TestHandshakeNilConfig(t *testing.T) {
	if _, err := Handshake(t, nil, &tls.Config{}); err == nil {
		t.Fatal("nil server config accepted")
	}
}
