// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/critalert"
	"unheaded/pkg/metrics"
	pqtls "unheaded/pkg/tls"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotansig"
	chatpb "unheaded/services/wotan/proto"
)

// Enrolling alert-mailer on alerts.critical is configuration only: with the
// environment monitoring/docker-compose.yml gives the container, the
// publisher critFromEnv builds reaches a Wotan that serves TLS 1.3 Strict
// with a certificate that does not name the address dialled (the host's
// overlay address in a deployment), presents alert-mailer's service token
// and publishes signed.

// tlsWotan is the slice of Wotan the mailer talks to, over TLS on addr: the
// REST join and the gRPC publish, each recording what arrived.
type tlsWotan struct {
	chatpb.UnimplementedTopicStreamServer
	httpAddr, grpcAddr string

	mu        sync.Mutex
	joinAuth  []string
	published []*chatpb.TopicPublishRequest
	pubAuth   []string
}

func (w *tlsWotan) PublishTopic(ctx context.Context, req *chatpb.TopicPublishRequest) (*chatpb.TopicPublishResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	authz := ""
	if v := md.Get("authorization"); len(v) > 0 {
		authz = v[0]
	}
	w.mu.Lock()
	w.published = append(w.published, req)
	w.pubAuth = append(w.pubAuth, authz)
	w.mu.Unlock()
	return &chatpb.TopicPublishResponse{MessageId: "m", Seq: 1}, nil
}

func (w *tlsWotan) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/subscribe") {
		http.NotFound(rw, r)
		return
	}
	w.mu.Lock()
	w.joinAuth = append(w.joinAuth, r.Header.Get("Authorization"))
	w.mu.Unlock()
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(rw).Encode(map[string]any{"subscriber": map[string]any{
		"subscriber_id": "11111111-1111-1111-1111-111111111111", "topic": critalert.Topic,
		"display_name": "alert-mailer", "status": "approved"}})
}

func (w *tlsWotan) seen() (joins []string, pubs []*chatpb.TopicPublishRequest, pubAuth []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.joinAuth...), append([]*chatpb.TopicPublishRequest(nil), w.published...), append([]string(nil), w.pubAuth...)
}

// startTLSWotan serves on 127.0.0.2 with a certificate issued as the dev
// certificate is (pkg/tls.IssueCert: wotan, localhost, 127.0.0.1, ::1): a
// loopback address the certificate does not carry stands in for the
// overlay address. It returns the fake and the CA file.
func startTLSWotan(t *testing.T) (*tlsWotan, string) {
	t.Helper()
	ca, err := pqtls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ca.IssueCert(pqtls.CertRequest{ServiceName: "wotan", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	srvCfg, err := pqtls.NewServerTLSConfig(pqtls.ServerConfig{CertPEM: sc.CertPEM, KeyPEM: sc.KeyPEM})
	if err != nil {
		t.Fatal(err)
	}
	pqtls.ApplyStrict(srvCfg)

	w := &tlsWotan{}
	hl, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Skipf("listen on 127.0.0.2: %v", err)
	}
	hs := httptest.NewUnstartedServer(http.HandlerFunc(w.serveHTTP))
	_ = hs.Listener.Close()
	hs.Listener = hl
	hs.TLS = srvCfg.Clone()
	hs.StartTLS()
	t.Cleanup(hs.Close)
	w.httpAddr = hl.Addr().String()

	gl, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(srvCfg.Clone())))
	chatpb.RegisterTopicStreamServer(gs, w)
	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(gs, h)
	go func() { _ = gs.Serve(gl) }()
	t.Cleanup(gs.Stop)
	w.grpcAddr = gl.Addr().String()
	return w, caFile
}

// enrolEnv sets the container's environment as the compose entry does and
// returns the signing key's public half.
func enrolEnv(t *testing.T, w *tlsWotan, caFile string, f *authtest.Fixture) []byte {
	t.Helper()
	var seed [mldsa65.SeedSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "alerts-critical.key")
	if err := os.WriteFile(keyFile, seed[:], 0o400); err != nil {
		t.Fatal(err)
	}
	signer, err := wotansig.LoadSignerFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	am, _ := fakeAlertmanager(t, amAlerts, http.StatusOK)
	t.Setenv("WOTAN_ADDR", "https://"+w.httpAddr)
	t.Setenv("WOTAN_GRPC_ADDR", w.grpcAddr)
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", caFile)
	t.Setenv("UNHEADED_TLS_CERT", "")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "")
	t.Setenv("UNHEADED_GRPC_TLS", "")
	t.Setenv(wotanClient.EnvTLSServerName, "wotan")
	t.Setenv("AUTH_KEY_FILE", filepath.Join(f.Keys, "alert-mailer.key"))
	t.Setenv("ALERTS_SIGNING_KEY_FILE", keyFile)
	t.Setenv("ALERTMANAGER_URL", am.URL)
	t.Cleanup(func() { wotanClient.SetServiceIdentity(nil) })
	return signer.PublicKey()
}

// closeSink closes the Wotan client a publisher built by critFromEnv holds.
func closeSink(t *testing.T, p *critPublisher) {
	t.Helper()
	if s, ok := p.sink.(*wotanCritSink); ok {
		t.Cleanup(func() { _ = s.c.Close() })
	}
}

func TestEnrolledPublisherReachesWotan(t *testing.T) {
	w, caFile := startTLSWotan(t)
	f := authtest.New(t, "wotan", "alert-mailer")
	pub := enrolEnv(t, w, caFile, f)
	verify, err := auth.NewServiceAuthenticator(f.Config(wotanClient.WotanAudience, "alert-mailer"))
	if err != nil {
		t.Fatal(err)
	}
	subject := func(authz string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", authz)
		id, err := verify.Authenticate(context.Background(), req)
		if err != nil {
			t.Fatalf("token %q does not verify for aud=wotan: %v", authz, err)
		}
		return id.Subject
	}

	p, err := critFromEnv(os.Getenv, metrics.NewRegistry())
	if err != nil || p == nil || p.sink == nil {
		t.Fatalf("critFromEnv = %v, %v; want a publisher with a sink", p, err)
	}
	closeSink(t, p)
	p.Snapshot() // reads the fake Alertmanager (fp1 fires), then publishes

	if e, r, ok := p.errors.Value(), p.refused.Value(), p.published.Value(); e != 0 || r != 0 || ok != 1 {
		t.Fatalf("errors=%v refused=%v published=%v, want 0, 0, 1", e, r, ok)
	}
	joins, pubs, pubAuth := w.seen()
	if len(joins) != 1 || subject(joins[0]) != "alert-mailer" {
		t.Fatalf("join over https carried %q, want one token of alert-mailer", joins)
	}
	if len(pubs) != 1 || subject(pubAuth[0]) != "alert-mailer" {
		t.Fatalf("%d publish(es), authorization %q; want one with a token of alert-mailer", len(pubs), pubAuth)
	}
	got := pubs[0]
	if got.Topic != critalert.Topic || !strings.Contains(string(got.Payload), `"fingerprint":"fp1"`) {
		t.Fatalf("published %s on %q, want the snapshot with fp1 on %s", got.Payload, got.Topic, critalert.Topic)
	}
	sg := wotansig.Signed{Signature: got.Signature, PublicKey: got.PublicKey, Algorithm: got.Algorithm, SignedAtUnixMs: got.SignedAtUnixMs}
	if err := wotansig.VerifyEvent(got.Topic, "alert-mailer", got.Payload, sg, [][]byte{pub}); err != nil {
		t.Fatalf("the published message does not verify against the enrolled key as sender alert-mailer: %v", err)
	}
}

// Each piece of the enrolment is needed, and a missing one fails closed:
// nothing reaches Wotan and the failure is counted.
func TestEnrolmentFailsClosed(t *testing.T) {
	t.Run("no server name: the certificate does not name the address", func(t *testing.T) {
		w, caFile := startTLSWotan(t)
		enrolEnv(t, w, caFile, authtest.New(t, "wotan", "alert-mailer"))
		t.Setenv(wotanClient.EnvTLSServerName, "")
		p, err := critFromEnv(os.Getenv, metrics.NewRegistry())
		if err != nil || p == nil {
			t.Fatalf("critFromEnv = %v, %v", p, err)
		}
		closeSink(t, p)
		p.Snapshot()
		if e, ok := p.errors.Value(), p.published.Value(); e != 1 || ok != 0 {
			t.Fatalf("errors=%v published=%v, want 1, 0", e, ok)
		}
		if joins, pubs, _ := w.seen(); len(joins)+len(pubs) != 0 {
			t.Fatalf("Wotan saw %d join(s) and %d publish(es) from a client that must refuse its certificate", len(joins), len(pubs))
		}
	})
	t.Run("no CA: no client, no plaintext", func(t *testing.T) {
		w, caFile := startTLSWotan(t)
		enrolEnv(t, w, caFile, authtest.New(t, "wotan", "alert-mailer"))
		t.Setenv("UNHEADED_TLS_CA_CERT", filepath.Join(t.TempDir(), "absent.pem"))
		p, err := critFromEnv(os.Getenv, metrics.NewRegistry())
		if err == nil || p != nil {
			t.Fatalf("critFromEnv = %v, %v; want no publisher and an error naming the CA", p, err)
		}
		if !strings.Contains(err.Error(), "UNHEADED_TLS_CA_CERT") {
			t.Fatalf("error %q does not name UNHEADED_TLS_CA_CERT", err)
		}
	})
	t.Run("no signing key file: refused, never unsigned", func(t *testing.T) {
		w, caFile := startTLSWotan(t)
		enrolEnv(t, w, caFile, authtest.New(t, "wotan", "alert-mailer"))
		t.Setenv("ALERTS_SIGNING_KEY_FILE", filepath.Join(t.TempDir(), "absent.key"))
		p, err := critFromEnv(os.Getenv, metrics.NewRegistry())
		if err == nil || p == nil || p.sink != nil {
			t.Fatalf("critFromEnv = %v, %v; want a publisher without a sink and an error", p, err)
		}
		p.Snapshot()
		if r := p.refused.Value(); r != 1 {
			t.Fatalf("refused=%v, want 1", r)
		}
		if joins, pubs, _ := w.seen(); len(joins)+len(pubs) != 0 {
			t.Fatalf("Wotan saw %d join(s) and %d publish(es) without a signing key", len(joins), len(pubs))
		}
	})
}

// monitoring/docker-compose.yml sets UNHEADED_TLS_ENABLED for the Wotan
// client. alert-mailer's own listeners (the webhooks on 19130, the
// heartbeat on 19131) stay plain HTTP: Alertmanager, the compose
// healthcheck and the heartbeat senders speak http to them. They are plain
// because main() starts them with net/http alone; this fails the day the
// binary takes its listeners' TLS from the same variable (pkg/tls), which
// must come with certificates in the compose entry and https in those
// three callers.
func TestListenersDoNotReadTheTLSVariable(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		files++
		for _, imp := range file.Imports {
			if path, _ := strconv.Unquote(imp.Path.Value); path == "unheaded/pkg/tls" {
				t.Errorf("%s imports unheaded/pkg/tls: UNHEADED_TLS_ENABLED is set in the compose entry for the Wotan client only", name)
			}
		}
	}
	if files == 0 {
		t.Fatal("parsed no source file: this test checks nothing")
	}
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), ".ListenAndServe()"); n != 2 {
		t.Errorf("main.go starts %d listeners with ListenAndServe(), want 2 (webhooks, heartbeat)", n)
	}
	for _, tlsStart := range []string{"ListenAndServeTLS", "ServeTLS", "TLSConfig"} {
		if strings.Contains(string(src), tlsStart) {
			t.Errorf("main.go mentions %s: a listener of alert-mailer is TLS; see this test's comment", tlsStart)
		}
	}
}
