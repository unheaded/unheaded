// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	pb "unheaded/proto/unheaded/v1"
)

// dialTestGRPC serves newGRPCServer(cfg) on loopback and returns a connection.
func dialTestGRPC(t *testing.T, cfg auth.ServiceAuthConfig) *grpc.ClientConn {
	t.Helper()
	// Stop waits for handlers: they read the globals ensureMockMode restores.
	srv, err := newGRPCServer(cfg, grpc.WaitForHandlers(true))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// streamCode opens Anamnesis.Stream and returns the status its first Recv
// ends with: OK when the stream was let through and is waiting for events.
func streamCode(t *testing.T, conn *grpc.ClientConn, md metadata.MD) codes.Code {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if md != nil {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	st, err := pb.NewAnamnesisServiceClient(conn).Stream(ctx, &pb.StreamRequest{})
	if err != nil {
		return status.Code(err)
	}
	_, err = st.Recv()
	if status.Code(err) == codes.DeadlineExceeded {
		return codes.OK
	}
	return status.Code(err)
}

// L5: the server stream takes the same credentials as the unary calls.
func TestGRPCStream_RequiresAuth(t *testing.T) {
	ensureMockMode(t)
	f := authtest.New(t, "protocol-api", "unheaded-cli", "kanban-app")
	conn := dialTestGRPC(t, f.Config("protocol-api", "unheaded-cli"))

	if c := streamCode(t, conn, nil); c != codes.Unauthenticated {
		t.Errorf("Stream anonymous: %v, want Unauthenticated", c)
	}
	if c := streamCode(t, conn, metadata.Pairs("authorization", f.Bearer("kanban-app", "protocol-api"))); c != codes.PermissionDenied {
		t.Errorf("Stream as not allowlisted kanban-app: %v, want PermissionDenied", c)
	}
	if c := streamCode(t, conn, metadata.Pairs("authorization", f.Bearer("unheaded-cli", "protocol-api"))); c != codes.OK {
		t.Errorf("Stream as unheaded-cli: %v, want OK", c)
	}
}

// L5: an API key guards gRPC as it guards REST, on unary calls and streams.
func TestGRPC_APIKey(t *testing.T) {
	ensureMockMode(t)
	globalAPIKey = "sample"
	conn := dialTestGRPC(t, auth.ServiceAuthConfig{})

	if c := streamCode(t, conn, nil); c != codes.Unauthenticated {
		t.Errorf("Stream with no key: %v, want Unauthenticated", c)
	}
	if c := streamCode(t, conn, metadata.Pairs("x-api-key", "wrong")); c != codes.Unauthenticated {
		t.Errorf("Stream with a wrong key: %v, want Unauthenticated", c)
	}
	if c := streamCode(t, conn, metadata.Pairs("x-api-key", "sample")); c != codes.OK {
		t.Errorf("Stream with the key: %v, want OK", c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := pb.NewSophiaServiceClient(conn).List(ctx, &pb.DictionaryListRequest{})
	if c := status.Code(err); c != codes.Unauthenticated {
		t.Errorf("unary call with no key: %v, want Unauthenticated", c)
	}
}

type endingStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *endingStream) Context() context.Context { return s.ctx }
func (s *endingStream) Send(*pb.Event) error     { return nil }

// L5: a stream that ends leaves no subscriber behind.
func TestStreamMock_RemovesSubscriberOnExit(t *testing.T) {
	as := &anamnesisServer{}
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- as.streamMock(&pb.StreamRequest{}, &endingStream{ctx: ctx}) }()
		for deadline := time.Now().Add(2 * time.Second); ; {
			as.subMu.RLock()
			n := len(as.subscribers)
			as.subMu.RUnlock()
			if n == 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("round %d: %d subscribers while one stream is open, want 1", i, n)
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
		<-done
	}
	as.subMu.RLock()
	defer as.subMu.RUnlock()
	if n := len(as.subscribers); n != 0 {
		t.Fatalf("%d subscribers after every stream ended, want 0", n)
	}
}

// L5: a key can come from a file or the environment, not only argv.
func TestResolveAPIKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(file, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == apiKeyEnv {
				return v
			}
			return ""
		}
	}
	for _, tc := range []struct {
		name, flagKey, file, env, want string
		fails                          bool
	}{
		{name: "none"},
		{name: "flag", flagKey: "from-flag", want: "from-flag"},
		{name: "env", env: "from-env", want: "from-env"},
		{name: "file", file: file, want: "from-file"},
		{name: "file over env and flag", flagKey: "from-flag", file: file, env: "from-env", want: "from-file"},
		{name: "env over flag", flagKey: "from-flag", env: "from-env", want: "from-env"},
		{name: "missing file", file: file + ".gone", env: "from-env", fails: true},
		{name: "empty file", file: empty, fails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAPIKey(tc.flagKey, tc.file, env(tc.env))
			if (err != nil) != tc.fails {
				t.Fatalf("err = %v, fails = %v", err, tc.fails)
			}
			if got != tc.want {
				t.Fatalf("key = %q, want %q", got, tc.want)
			}
		})
	}
}
