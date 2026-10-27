// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package wotanClient

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc/credentials"

	"unheaded/pkg/auth"
)

// WotanAudience is the aud every Wotan service token is minted for: Wotan's
// own identity (auth.LoadServiceAuthConfig("wotan")).
const WotanAudience = "wotan"

// tokenMinter is what a client mints its Wotan tokens with;
// *auth.ServiceTokenMinter is the production one.
type tokenMinter interface {
	Mint(target string, opts ...auth.MintOption) (string, error)
}

// processMinter is the identity every client in this process presents when
// it was not given one of its own (SetServiceIdentity). A process holds
// one service key, so one identity: pkg/healthreport, pkg/discovery and
// pkg/logagg build their own clients and present it without being told.
var processMinter atomic.Pointer[auth.ServiceTokenMinter]

// SetServiceIdentity makes m the identity every Wotan client in this
// process presents (Authorization: Bearer <token aud=wotan> on every HTTP
// request and gRPC call), unless the client was given its own with
// WithServiceToken. Nil removes it. Clients built before the call use it
// from their next request (S-IRON-GATE Phase 7, ADR-103 §6).
func SetServiceIdentity(m *auth.ServiceTokenMinter) {
	processMinter.Store(m)
}

// ServiceIdentity returns the process identity SetServiceIdentity set, or nil.
func ServiceIdentity() *auth.ServiceTokenMinter {
	return processMinter.Load()
}

// UseServiceIdentity loads this binary's key and makes it the process
// identity (SetServiceIdentity). name is the binary's auth identity, the
// name its key is filed under (unheaded-keygen <name>): AUTH_KEY_FILE, else
// /etc/unheaded/keys/<name>.key. A missing key is not an error: the binary
// runs without a Wotan token (a Wotan with auth off still takes it; one
// with auth on answers 401 and the warning says why). Any other key
// problem (mode, format) is returned: a key that is there but unusable is
// a misconfiguration, not a host before unheaded-keygen.
func UseServiceIdentity(name string) (*auth.ServiceTokenMinter, error) {
	keyFile := os.Getenv("AUTH_KEY_FILE")
	if keyFile == "" {
		keyFile = filepath.Join(auth.DefaultKeyDir, name+".key")
	}
	m, err := auth.LoadServiceTokenMinter(name, keyFile)
	if errors.Is(err, fs.ErrNotExist) {
		log.Printf("[WARN] wotan_client: no service key at %s; Wotan calls carry no token as %q (a Wotan with AUTH_ENABLED=true refuses them; run unheaded-keygen %s, ADR-103)", keyFile, name, name)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("wotan client identity %q: %w", name, err)
	}
	SetServiceIdentity(m)
	return m, nil
}

// InitServiceIdentity is UseServiceIdentity for a binary's main(): it logs
// a key it cannot use and carries on without a token, so a service whose
// Wotan link is optional does not crash-loop over it (Wotan with auth on
// refuses the calls, and this log line says why).
func InitServiceIdentity(name string) {
	if _, err := UseServiceIdentity(name); err != nil {
		log.Printf("[ERROR] wotan_client: %v; Wotan calls carry no token (ADR-103)", err)
	}
}

// AuthorizeRequest sets Authorization on a hand-built HTTP request to Wotan
// from the process identity (SetServiceIdentity); with none, it leaves the
// request as is. For callers that speak Wotan's REST API without a Client.
func AuthorizeRequest(req *http.Request) error {
	return (&tokenAuth{}).authorize(req)
}

// ClientOption configures a Client (NewClient, NewClientWithGRPC,
// NewClientWithTLS) or a GRPCClient (NewGRPCClient).
type ClientOption func(*clientOptions)

type clientOptions struct {
	minter     tokenMinter
	insecureOK bool
}

// WithServiceToken makes the client present m's tokens instead of the
// process identity.
func WithServiceToken(m *auth.ServiceTokenMinter) ClientOption {
	return func(o *clientOptions) {
		if m != nil {
			o.minter = m
		}
	}
}

// WithInsecureTransport lets the gRPC token travel over a plaintext
// connection without the one-time warning (tests, loopback).
func WithInsecureTransport() ClientOption {
	return func(o *clientOptions) { o.insecureOK = true }
}

// newTokenAuth builds the authorizer for a client from its options;
// plaintext says its gRPC connection is not TLS.
func newTokenAuth(opts []ClientOption, plaintext bool) *tokenAuth {
	var o clientOptions
	for _, f := range opts {
		if f != nil {
			f(&o)
		}
	}
	return &tokenAuth{minter: o.minter, plaintext: plaintext, quiet: o.insecureOK}
}

// isPlaintext reports whether creds send in the clear.
func isPlaintext(creds credentials.TransportCredentials) bool {
	return creds == nil || creds.Info().SecurityProtocol == "insecure"
}

// tokenAuth authorizes a client's requests to Wotan: its own minter, else
// the process identity, else nothing (Wotan with auth off still answers).
type tokenAuth struct {
	minter tokenMinter
	// plaintext: the gRPC connection this serves is not TLS.
	plaintext bool
	quiet     bool
}

// header returns the Authorization value, "" when the client has no identity.
func (t *tokenAuth) header() (string, error) {
	var m tokenMinter
	if t != nil && t.minter != nil {
		m = t.minter
	} else if p := processMinter.Load(); p != nil {
		m = p
	}
	if m == nil {
		return "", nil
	}
	tok, err := m.Mint(WotanAudience)
	if err != nil {
		return "", fmt.Errorf("wotan service token: %w", err)
	}
	return "Bearer " + tok, nil
}

// authorize sets Authorization on an HTTP request to Wotan.
func (t *tokenAuth) authorize(req *http.Request) error {
	h, err := t.header()
	if err != nil {
		return err
	}
	if h != "" {
		req.Header.Set("Authorization", h)
	}
	return nil
}

var plaintextWarnOnce sync.Once

// GetRequestMetadata implements credentials.PerRPCCredentials: the token in
// the "authorization" metadata of every unary and stream call.
func (t *tokenAuth) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	h, err := t.header()
	if err != nil {
		return nil, err
	}
	if h == "" {
		return nil, nil
	}
	if t.plaintext && !t.quiet {
		plaintextWarnOnce.Do(func() {
			log.Printf("[WARN] wotan_client: service token sent over plaintext gRPC; set UNHEADED_GRPC_TLS=1 once Wotan serves TLS (S-IRON-GATE Phase 8)")
		})
	}
	return map[string]string{"authorization": h}, nil
}

// RequireTransportSecurity implements credentials.PerRPCCredentials. A
// client that dials TLS never sends its token on anything else; one that
// dials plaintext (no Wotan TLS before S-IRON-GATE Phase 8) sends it there,
// warning once: a 5-minute token bound to aud=wotan, the same exposure as
// the HTTP control plane on the same network.
func (t *tokenAuth) RequireTransportSecurity() bool {
	return !t.plaintext
}

var _ credentials.PerRPCCredentials = (*tokenAuth)(nil)
