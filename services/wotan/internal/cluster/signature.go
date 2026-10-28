// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package cluster

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/store"
	chatpb "unheaded/services/wotan/proto"
)

// FND-020: replication keeps signatures. Wotan persists a signed topic
// message with its ML-DSA-65 signature (store.Message.Signed), the WAL
// record streamed to a standby is that encoding, and the standby verifies
// the signature it finds inside against its own trusted keys (the same
// signing policy the primary enforces at publish) before the entry is
// applied. Whether a topic needs a signature is the standby's policy, never
// the sender's say: an entry on a signed topic without one is rejected,
// whoever sent it.
//
// The signature covers the payload bytes the publisher sent; the stored
// Content is a string that the JSON encoding would rewrite if it were not
// valid UTF-8. PublishTopic therefore refuses a non-UTF-8 payload on a
// signed topic, so what the standby verifies is byte for byte what was
// signed.
//
// Open before cluster mode is wired (PR #2 review, 2026-10-28): a standby
// verifies history under today's policy. Records written before signatures
// were persisted, and records signed by a key since retired, fail here and
// halt replication (see ReplicationClient.handleEntry). A cutover sequence
// or a retired-but-trusted key set is needed before the first standby
// replays a WAL that predates this code.

// replicationRejected counts WAL entries a standby refused to apply, by
// reason: decode (data is not a stored message, so its topic is unknown),
// unsigned (a signed topic, no signature), signature (untrusted key, wrong
// algorithm or a signature that does not verify).
var replicationRejected = auto.NewCounterVec(
	prom.CounterOpts{
		Name: "unheaded_wotan_replication_rejected_total",
		Help: "WAL entries a Wotan standby refused to apply, by reason",
	},
	[]string{"reason"},
)

// Rejection reasons (the label values of unheaded_wotan_replication_rejected_total).
const (
	RejectDecode    = "decode"
	RejectUnsigned  = "unsigned"
	RejectSignature = "signature"
)

// RejectError is why a standby refused a replicated entry.
type RejectError struct {
	Reason string
	Topic  string
	Err    error
}

func (e *RejectError) Error() string {
	return fmt.Sprintf("replication: entry rejected (%s, topic %q): %v", e.Reason, e.Topic, e.Err)
}

func (e *RejectError) Unwrap() error { return e.Err }

// IsRejected reports whether err is a RejectError with the given reason.
func IsRejected(err error, reason string) bool {
	var re *RejectError
	return errors.As(err, &re) && re.Reason == reason
}

// walEntry builds the WALEntry the primary streams for one WAL record: the
// record as stored. A signed message carries its signature inside data.
func walEntry(seq uint64, data []byte, nodeID string, epoch int64) *chatpb.WALEntry {
	return &chatpb.WALEntry{
		Sequence:  seq,
		Data:      data,
		NodeId:    nodeID,
		Timestamp: timestamppb.Now(),
		Epoch:     epoch,
	}
}

// VerifyEntry decides whether a standby may apply a replicated entry under
// policy p (nil: the built-in signed prefixes, no trusted key, so every
// signed-topic entry is refused). An entry on an unsigned topic is applied
// as it is; one on a signed topic must carry, inside its data, a signature
// by a key p trusts for that topic over the topic, sender and content.
// Nothing here reads the clock or a replay cache: the primary checked
// freshness and replay when it accepted the message, and a standby sees old
// entries again on every catch-up.
func VerifyEntry(p *signing.Policy, e *chatpb.WALEntry) error {
	msg, err := store.DecodeMessage(e.GetData())
	if err != nil {
		return &RejectError{Reason: RejectDecode, Err: err}
	}
	topic := msg.RoomID
	if !p.RequiresSignature(topic) {
		return nil
	}
	s := msg.Signed
	if s == nil {
		return &RejectError{Reason: RejectUnsigned, Topic: topic, Err: errors.New("signed topic, stored message carries no signature")}
	}
	v, ok := p.VerifierFor(topic)
	if !ok {
		v = nil // fail closed: VerifyStored on a nil verifier trusts no key
	}
	sg := signing.Signed{Signature: s.Signature, PublicKey: s.PublicKey, Algorithm: s.Algorithm, SignedAtUnixMs: s.SignedAtUnixMs}
	if err := v.VerifyStored(topic, s.Sender, []byte(msg.Content), sg); err != nil {
		return &RejectError{Reason: RejectSignature, Topic: topic, Err: err}
	}
	return nil
}
