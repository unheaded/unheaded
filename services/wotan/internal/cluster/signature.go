// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package cluster

import (
	"bytes"
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
// message with its ML-DSA-65 signature (store.Message.Signed); the primary
// copies that signature onto the WALEntry it streams, and the standby
// verifies it against its own trusted keys (the same signing policy the
// primary enforces at publish) before the entry is applied. Whether a topic
// needs a signature is the standby's policy, never the sender's say: an
// entry on a signed topic without one is rejected, whoever sent it.

// replicationRejected counts WAL entries a standby refused to apply, by
// reason: decode (data is not a stored message, so its topic is unknown),
// unsigned (a signed topic, no signature), signature (untrusted key, wrong
// algorithm or a signature that does not verify), mismatch (the signature
// fields disagree with the signature stored in the entry's data).
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
	RejectMismatch  = "mismatch"
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

// walEntry builds the WALEntry the primary streams for one WAL record. A
// record holding a signed stored message carries its signature in the
// signature fields; anything else (unsigned, or not a stored message at all)
// goes out without them, and the standby's policy decides.
func walEntry(seq uint64, data []byte, nodeID string, epoch int64) *chatpb.WALEntry {
	e := &chatpb.WALEntry{
		Sequence:  seq,
		Data:      data,
		NodeId:    nodeID,
		Timestamp: timestamppb.Now(),
		Epoch:     epoch,
	}
	if msg, err := store.DecodeMessage(data); err == nil && msg.Signed != nil {
		e.Signature = msg.Signed.Signature
		e.PublicKey = msg.Signed.PublicKey
		e.Algorithm = msg.Signed.Algorithm
		e.SignedAtUnixMs = msg.Signed.SignedAtUnixMs
		e.SenderId = msg.Signed.Sender
	}
	return e
}

// hasSignatureFields reports whether e carries any signature field.
func hasSignatureFields(e *chatpb.WALEntry) bool {
	return len(e.Signature) > 0 || len(e.PublicKey) > 0 || e.Algorithm != "" || e.SignedAtUnixMs != 0 || e.SenderId != ""
}

// VerifyEntry decides whether a standby may apply a replicated entry under
// policy p (nil: the built-in signed prefixes, no trusted key, so every
// signed-topic entry is refused). An entry on an unsigned topic is applied
// as it is; one on a signed topic must carry a signature, by a key p trusts
// for that topic, over the topic, sender and content in its data, and equal
// to the signature stored in that data. Nothing here reads the clock or a
// replay cache: the primary checked freshness and replay when it accepted
// the message, and a standby sees old entries again on every catch-up.
func VerifyEntry(p *signing.Policy, e *chatpb.WALEntry) error {
	msg, err := store.DecodeMessage(e.GetData())
	if err != nil {
		return &RejectError{Reason: RejectDecode, Err: err}
	}
	topic := msg.RoomID
	if !p.RequiresSignature(topic) {
		return nil
	}
	if !hasSignatureFields(e) {
		return &RejectError{Reason: RejectUnsigned, Topic: topic, Err: errors.New("signed topic, entry carries no signature")}
	}
	v, ok := p.VerifierFor(topic)
	if !ok {
		v = nil // fail closed: VerifyStored on a nil verifier trusts no key
	}
	sg := signing.Signed{Signature: e.Signature, PublicKey: e.PublicKey, Algorithm: e.Algorithm, SignedAtUnixMs: e.SignedAtUnixMs}
	if err := v.VerifyStored(topic, e.SenderId, []byte(msg.Content), sg); err != nil {
		return &RejectError{Reason: RejectSignature, Topic: topic, Err: err}
	}
	s := msg.Signed
	if s == nil || s.Sender != e.SenderId || s.Algorithm != e.Algorithm || s.SignedAtUnixMs != e.SignedAtUnixMs ||
		!bytes.Equal(s.Signature, e.Signature) || !bytes.Equal(s.PublicKey, e.PublicKey) {
		return &RejectError{Reason: RejectMismatch, Topic: topic, Err: errors.New("signature fields differ from the signature stored in data")}
	}
	return nil
}
