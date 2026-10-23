// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// timerNotFiredCtx is a query context at the instant its deadline has passed
// but its timer has not run yet: Deadline() is in the past, Err() is nil.
// That instant is real. The server ends the stream at (or just after) the
// same deadline, and grpc can deliver that end to Recv before the client's
// context timer goroutine runs.
type timerNotFiredCtx struct {
	context.Context
	deadline time.Time
}

func (c timerNotFiredCtx) Deadline() (time.Time, bool) { return c.deadline, true }

// The pkg/wotan-client TestGRPCClient_GetMessages flake (about 2 in 20 runs):
// at the 200 ms query timeout GetMessages failed with
//
//	receive from stream: rpc error: code = DeadlineExceeded desc = stream terminated by RST_STREAM with error code: CANCEL
//	receive from stream: rpc error: code = Canceled desc = context canceled
//
// both of which are the query's own deadline, reported by the server before
// queryCtx.Err() was set. They are the normal end of the query.
func TestQueryEnded_ServerReportsOurDeadlineFirst(t *testing.T) {
	past := timerNotFiredCtx{Context: context.Background(), deadline: time.Now().Add(-time.Millisecond)}
	if past.Err() != nil {
		t.Fatal("fixture: Err must still be nil")
	}
	for _, err := range []error{
		status.Error(codes.DeadlineExceeded, "stream terminated by RST_STREAM with error code: CANCEL"),
		status.Error(codes.Canceled, "context canceled"),
	} {
		if !queryEnded(context.Background(), past, err) {
			t.Errorf("queryEnded(deadline passed, timer not fired, %v) = false, want true (the query's own timeout)", err)
		}
	}
}

// What must still be reported as Wotan failing the query.
func TestQueryEnded_RealFailuresStillReported(t *testing.T) {
	future := timerNotFiredCtx{Context: context.Background(), deadline: time.Now().Add(time.Hour)}
	past := timerNotFiredCtx{Context: context.Background(), deadline: time.Now().Add(-time.Millisecond)}
	cases := []struct {
		name string
		ctx  context.Context
		err  error
	}{
		{"deadline-exceeded before our deadline", future, status.Error(codes.DeadlineExceeded, "upstream deadline")},
		{"canceled before our deadline", future, status.Error(codes.Canceled, "server shut down the stream")},
		{"unavailable before our deadline", future, status.Error(codes.Unavailable, "transport is closing")},
		{"unavailable after our deadline", past, status.Error(codes.Unavailable, "transport is closing")},
		{"permission denied after our deadline", past, status.Error(codes.PermissionDenied, "not approved")},
		{"plain error after our deadline", past, errors.New("boom")},
	}
	for _, tc := range cases {
		if queryEnded(context.Background(), tc.ctx, tc.err) {
			t.Errorf("%s: queryEnded = true, want false (Wotan failed the query)", tc.name)
		}
	}
}

func TestQueryEnded_NormalEnds(t *testing.T) {
	if !queryEnded(context.Background(), context.Background(), io.EOF) {
		t.Error("io.EOF: want true")
	}
	expired, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()
	if !queryEnded(context.Background(), expired, status.Error(codes.Unavailable, "x")) {
		t.Error("query context done: want true")
	}
	caller, ccancel := context.WithCancel(context.Background())
	ccancel()
	if !queryEnded(caller, context.Background(), status.Error(codes.Unavailable, "x")) {
		t.Error("caller cancelled: want true")
	}
}
