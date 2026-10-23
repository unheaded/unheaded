// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"unheaded/pkg/storage/wal"
	chatpb "unheaded/services/wotan/proto"
)

// replicationPeer runs a ReplicationServer over a WAL holding n entries and
// returns a client to it.
func replicationPeer(t *testing.T, n int) chatpb.WotanReplicationClient {
	t.Helper()
	w, err := wal.New(wal.Config{Path: t.TempDir(), SegmentSize: 1 << 20, MaxSegments: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	for i := 1; i <= n; i++ {
		if _, err := w.Write(context.Background(), []byte(fmt.Sprintf("entry-%d", i))); err != nil {
			t.Fatal(err)
		}
	}

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	chatpb.RegisterWotanReplicationServer(gs, NewReplicationServer(w, "primary-1", 1))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return chatpb.NewWotanReplicationClient(conn)
}

// TransferSnapshot is not implemented, and says so. It used to send one empty
// chunk marked Final with LastSeq = the primary's current sequence, which a
// standby would read as "you are up to date" and so skip every message.
func TestTransferSnapshot_IsUnimplemented(t *testing.T) {
	client := replicationPeer(t, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.TransferSnapshot(ctx, &chatpb.SnapshotRequest{NodeId: "standby-1"})
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := stream.Recv()
	if chunk != nil {
		t.Fatalf("got a snapshot chunk %+v (final=%v last_seq=%d), want none", chunk, chunk.Final, chunk.LastSeq)
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("err = %v, want code Unimplemented", err)
	}
}

// A fresh standby catches up from seq 0 and receives every entry the primary
// holds, in order: the path a standby must use while snapshots are
// unimplemented.
func TestCatchUp_FromZeroDeliversEverything(t *testing.T) {
	const n = 5
	client := replicationPeer(t, n)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.CatchUp(ctx, &chatpb.CatchUpRequest{NodeId: "standby-1", SinceSeq: 0, Epoch: 1})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	var last uint64
	for {
		e, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("recv after %d entries: %v", len(got), err)
		}
		if e.Sequence <= last {
			t.Fatalf("sequence %d after %d: out of order", e.Sequence, last)
		}
		last = e.Sequence
		got = append(got, string(e.Data))
		if len(got) == n {
			break
		}
	}
	if len(got) != n || got[0] != "entry-1" || got[n-1] != fmt.Sprintf("entry-%d", n) {
		t.Errorf("caught up %v, want entry-1..entry-%d", got, n)
	}
}
