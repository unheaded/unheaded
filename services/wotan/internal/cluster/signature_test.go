// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package cluster

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/storage/wal"
	"unheaded/pkg/wotansig"
	"unheaded/services/wotan/internal/ringbuffer"
	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/store"
	chatpb "unheaded/services/wotan/proto"
)

// FND-020: a signed message replicated from the primary reaches the standby
// with its signature, and the standby verifies it before applying.

// testKey generates an ML-DSA-65 signer at test time (no key material in the
// repository).
func testKey(t *testing.T) *wotansig.TopicSigner {
	t.Helper()
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	return wotansig.NewTopicSigner(s)
}

// signedMessage is a store.Message as Wotan persists a verified signed
// publish: the signature covers topic, sender, signing time and content.
func signedMessage(t *testing.T, ts *wotansig.TopicSigner, topic, sender, content string) *store.Message {
	t.Helper()
	sg, err := ts.Sign(topic, sender, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return &store.Message{
		ID: uuid.New(), CreatorID: uuid.New(), RoomID: topic, Content: content, Timestamp: time.Now().UTC(),
		Signed: &ringbuffer.Signature{
			Sender: sender, Signature: sg.Signature, PublicKey: sg.PublicKey,
			Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
		},
	}
}

func unsignedMessage(topic, content string) *store.Message {
	return &store.Message{ID: uuid.New(), CreatorID: uuid.New(), RoomID: topic, Content: content, Timestamp: time.Now().UTC()}
}

// primaryWith runs a ReplicationServer over a WAL holding msgs (encoded as
// the WAL store encodes them) and returns a connected client.
func primaryWith(t *testing.T, msgs ...*store.Message) chatpb.WotanReplicationClient {
	t.Helper()
	w, err := wal.New(wal.Config{Path: t.TempDir(), SegmentSize: 1 << 20, MaxSegments: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	for _, m := range msgs {
		if _, err := w.Write(context.Background(), mustEncode(t, m)); err != nil {
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

// standby is a ReplicationClient wired to client, trusting the given keys
// for config.*, recording every entry it applies.
type standby struct {
	*ReplicationClient
	mu      sync.Mutex
	applied []*chatpb.WALEntry
}

func newStandby(t *testing.T, client chatpb.WotanReplicationClient, trusted ...[]byte) *standby {
	t.Helper()
	p, err := signing.NewPolicy(trusted)
	if err != nil {
		t.Fatal(err)
	}
	sb := &standby{ReplicationClient: NewReplicationClient("bufnet", "standby-1", "", 1)}
	sb.client = client
	sb.SetSigningPolicy(p)
	sb.ApplyFunc = func(_ context.Context, e *chatpb.WALEntry) error {
		sb.mu.Lock()
		defer sb.mu.Unlock()
		sb.applied = append(sb.applied, e)
		return nil
	}
	return sb
}

// catchUp runs the standby's replication loop until it has seen wantSeq.
func (sb *standby) catchUp(t *testing.T, wantSeq uint64) []*chatpb.WALEntry {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sb.StartReplication(ctx) }()
	deadline := time.Now().Add(9 * time.Second)
	for sb.lastAppliedSeq.Load() < wantSeq {
		if time.Now().After(deadline) {
			t.Fatalf("standby reached seq %d, want %d", sb.lastAppliedSeq.Load(), wantSeq)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("StartReplication: %v", err)
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return append([]*chatpb.WALEntry(nil), sb.applied...)
}

func rejected(reason string) float64 {
	return replicationRejected.WithLabelValues(reason).Value()
}

func TestReplication_SignedEntryKeepsItsSignatureAndVerifies(t *testing.T) {
	key := testKey(t)
	msg := signedMessage(t, key, "config.x", "heimdall-01", `{"baseline":"v1"}`)
	sb := newStandby(t, primaryWith(t, msg), key.PublicKey())
	before := rejected("signature")

	applied := sb.catchUp(t, 1)

	if len(applied) != 1 {
		t.Fatalf("applied %d entries, want 1", len(applied))
	}
	e := applied[0]
	if len(e.Signature) == 0 || len(e.PublicKey) == 0 || e.Algorithm != wotansig.Algorithm || e.SignedAtUnixMs == 0 || e.SenderId != "heimdall-01" {
		t.Fatalf("standby copy lost its signature: sig=%d key=%d alg=%q at=%d sender=%q",
			len(e.Signature), len(e.PublicKey), e.Algorithm, e.SignedAtUnixMs, e.SenderId)
	}
	got, err := store.DecodeMessage(e.Data)
	if err != nil {
		t.Fatal(err)
	}
	sg := wotansig.Signed{Signature: e.Signature, PublicKey: e.PublicKey, Algorithm: e.Algorithm, SignedAtUnixMs: e.SignedAtUnixMs}
	if err := wotansig.VerifyEvent(got.RoomID, e.SenderId, []byte(got.Content), sg, [][]byte{key.PublicKey()}); err != nil {
		t.Fatalf("standby copy does not verify: %v", err)
	}
	if got.Signed == nil || got.Signed.Sender != "heimdall-01" {
		t.Fatalf("applied message carries no signature: %+v", got.Signed)
	}
	if d := rejected("signature") - before; d != 0 {
		t.Fatalf("rejected{signature} grew by %v, want 0", d)
	}
}

func TestReplication_TamperedEntryIsRejectedAndCounted(t *testing.T) {
	key := testKey(t)
	tampered := signedMessage(t, key, "config.x", "heimdall-01", `{"baseline":"v1"}`)
	tampered.Content = `{"baseline":"evil"}`
	good := unsignedMessage("logs.x", "hello")
	sb := newStandby(t, primaryWith(t, tampered, good), key.PublicKey())
	before := rejected("signature")

	applied := sb.catchUp(t, 2)

	if len(applied) != 1 || applied[0].Sequence != 2 {
		t.Fatalf("applied %d entries, want only seq 2 (the unsigned logs.x one)", len(applied))
	}
	if d := rejected("signature") - before; d != 1 {
		t.Fatalf("rejected{signature} grew by %v, want 1", d)
	}
}

// The standby asks for the entries after the last one it handled. CatchUp's
// since_seq is inclusive (wal.Scan), so asking from the last applied seq
// re-delivered that entry on every 500 ms round: applied twice, and a
// rejected one counted again each round.
func TestReplication_EachEntryHandledOnceAcrossRounds(t *testing.T) {
	key := testKey(t)
	tampered := signedMessage(t, key, "config.x", "s", "v1")
	tampered.Content = "v2"
	sb := newStandby(t, primaryWith(t, unsignedMessage("logs.x", "a"), tampered), key.PublicKey())
	before := rejected("signature")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sb.StartReplication(ctx) }()
	time.Sleep(1300 * time.Millisecond) // at least two catch-up rounds
	cancel()
	<-done

	sb.mu.Lock()
	n := len(sb.applied)
	sb.mu.Unlock()
	if n != 1 {
		t.Fatalf("applied %d entries over the rounds, want 1 (seq 1 once)", n)
	}
	if d := rejected("signature") - before; d != 1 {
		t.Fatalf("rejected{signature} grew by %v over the rounds, want 1", d)
	}
}

func TestReplication_WrongKeyIsRejected(t *testing.T) {
	signer, other := testKey(t), testKey(t)
	msg := signedMessage(t, signer, "drift.host", "heimdall-01", "drift")
	p, err := signing.NewPolicy(nil, signing.PrefixKeys{Prefix: "drift.", Keys: [][]byte{other.PublicKey()}})
	if err != nil {
		t.Fatal(err)
	}
	e := walEntry(1, mustEncode(t, msg), "primary-1", 1)
	if err := VerifyEntry(p, e); !IsRejected(err, "signature") {
		t.Fatalf("VerifyEntry with an untrusted key = %v, want a signature rejection", err)
	}
}

func TestReplication_UnsignedEntryOnSigningTopicIsRejected(t *testing.T) {
	key := testKey(t)
	p, err := signing.NewPolicy([][]byte{key.PublicKey()})
	if err != nil {
		t.Fatal(err)
	}
	// An old primary that sends no signature fields: the policy decides,
	// not the sender.
	e := walEntry(1, mustEncode(t, unsignedMessage("config.x", "v1")), "primary-1", 1)
	if err := VerifyEntry(p, e); !IsRejected(err, "unsigned") {
		t.Fatalf("VerifyEntry(unsigned config.x) = %v, want an unsigned rejection", err)
	}
	// A nil policy still knows the built-in signed prefixes and trusts no key.
	signed := walEntry(1, mustEncode(t, signedMessage(t, key, "config.x", "s", "v1")), "primary-1", 1)
	if err := VerifyEntry(nil, signed); !IsRejected(err, "signature") {
		t.Fatalf("VerifyEntry(nil policy) = %v, want a signature rejection (fail closed)", err)
	}
}

func TestReplication_UnsignedEntryOnPlainTopicApplies(t *testing.T) {
	e := walEntry(1, mustEncode(t, unsignedMessage("logs.x", "hello")), "primary-1", 1)
	if err := VerifyEntry(nil, e); err != nil {
		t.Fatalf("VerifyEntry(unsigned logs.x) = %v, want nil", err)
	}
	if len(e.Signature) != 0 || e.Algorithm != "" || e.SenderId != "" {
		t.Fatalf("unsigned entry grew signature fields: %+v", e)
	}
}

// The signature fields must agree with the signature inside the entry's
// data: the data is what the standby stores and later serves.
func TestReplication_FieldsThatDisagreeWithDataAreRejected(t *testing.T) {
	a, b := testKey(t), testKey(t)
	e := walEntry(1, mustEncode(t, signedMessage(t, a, "config.x", "s", "v1")), "primary-1", 1)
	other := signedMessage(t, b, "config.x", "s", "v1")
	e.Signature, e.PublicKey, e.SignedAtUnixMs = other.Signed.Signature, other.Signed.PublicKey, other.Signed.SignedAtUnixMs
	p, err := signing.NewPolicy([][]byte{a.PublicKey(), b.PublicKey()})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEntry(p, e); !IsRejected(err, "mismatch") {
		t.Fatalf("VerifyEntry(fields from another signature) = %v, want a mismatch rejection", err)
	}
}

func TestReplication_UndecodableEntryIsRejected(t *testing.T) {
	e := walEntry(1, []byte("entry-1"), "primary-1", 1)
	if err := VerifyEntry(nil, e); !IsRejected(err, "decode") {
		t.Fatalf("VerifyEntry(raw bytes) = %v, want a decode rejection", err)
	}
}

func mustEncode(t *testing.T, m *store.Message) []byte {
	t.Helper()
	b, err := store.EncodeMessage(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
