// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pqtls "unheaded/pkg/tls"
	"unheaded/pkg/transport/mtls"
	"unheaded/services/wotan/internal/signing"
	chatpb "unheaded/services/wotan/proto"
)

// ReplicationClient connects to the primary and receives WAL entries (standby side).
type ReplicationClient struct {
	peerAddr string
	nodeID   string
	epoch    int64
	pkiDir   string // mTLS certificate directory (empty = insecure)

	conn   *grpc.ClientConn
	client chatpb.WotanReplicationClient

	lastAppliedSeq atomic.Uint64
	// haltedAtSeq is the sequence of the entry whose rejection ended
	// replication (0: none).
	haltedAtSeq atomic.Uint64
	entriesRecv atomic.Uint64

	// policy decides which topics need a signature and which keys sign
	// them (FND-020): every entry is checked by VerifyEntry before
	// ApplyFunc sees it. Nil still refuses unsigned or unverifiable entries
	// on the built-in signed prefixes (it trusts no key).
	policy *signing.Policy

	// ApplyFunc is called for each received WAL entry that passed
	// VerifyEntry. The standby wires this to write entries to its local store.
	ApplyFunc func(ctx context.Context, entry *chatpb.WALEntry) error
}

// SetSigningPolicy sets the policy replicated entries are verified against:
// the same one this node enforces on publish (configs/wotan.yaml
// topics.signing_keys and topics.signed_prefixes). Call it before
// StartReplication.
func (c *ReplicationClient) SetSigningPolicy(p *signing.Policy) {
	c.policy = p
}

// NewReplicationClient creates a standby-side replication client.
func NewReplicationClient(peerAddr, nodeID, pkiDir string, epoch int64) *ReplicationClient {
	return &ReplicationClient{
		peerAddr: peerAddr,
		nodeID:   nodeID,
		pkiDir:   pkiDir,
		epoch:    epoch,
	}
}

// Connect establishes gRPC connection to the primary.
// Uses mTLS if pkiDir is set, falls back to insecure for development.
//
// ctx is accepted for callers that previously relied on DialContext's
// dial-deadline semantics. With grpc.NewClient connection is lazy, so the
// ctx is now consumed by downstream RPCs (CatchUp / StartReplication
// loop) rather than the dial itself.
func (c *ReplicationClient) Connect(_ context.Context) error {
	// No silent plaintext fallback (S-IRON-GATE Phase 8, FND-017): a PKI
	// directory that cannot be loaded is an error, and plaintext without
	// one is refused under UNHEADED_TLS_ENABLED=true.
	var dialOpt grpc.DialOption
	if c.pkiDir != "" {
		tlsCfg, err := mtls.SetupServiceClientTLS("wotan", c.pkiDir)
		if err != nil {
			return fmt.Errorf("replication_client: mTLS setup from %s: %w", c.pkiDir, err)
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
	} else {
		creds, err := pqtls.PlaintextGRPCCredentials("wotan replication (no -cluster-pki-dir)")
		if err != nil {
			return fmt.Errorf("replication_client: %w", err)
		}
		dialOpt = grpc.WithTransportCredentials(creds)
	}

	// grpc.NewClient (lazy/non-blocking) replaces deprecated DialContext +
	// WithBlock. The first RPC will surface any connection errors; the
	// outer reconnection loop in StartReplication handles retries.
	conn, err := grpc.NewClient(c.peerAddr, dialOpt)
	if err != nil {
		return fmt.Errorf("replication_client: connect %s: %w", c.peerAddr, err)
	}

	c.conn = conn
	c.client = chatpb.NewWotanReplicationClient(conn)

	log.Info().
		Str("peer", c.peerAddr).
		Str("node", c.nodeID).
		Msg("replication: connected to primary")

	return nil
}

// StartReplication begins the replication loop. Blocks until ctx is cancelled.
// Automatically reconnects on disconnect with exponential backoff.
func (c *ReplicationClient) StartReplication(ctx context.Context) error {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Ensure connected
		if c.client == nil {
			if err := c.Connect(ctx); err != nil {
				log.Warn().Err(err).Dur("backoff", backoff).Msg("replication: connect failed, retrying")
				time.Sleep(backoff)
				backoff = min(backoff*2, maxBackoff)
				continue
			}
			backoff = 1 * time.Second // Reset on success
		}

		// Request CatchUp from the entry after the last one handled.
		// since_seq is inclusive (wal.Scan) and WAL sequences start at 1;
		// asking from the last handled seq re-delivered it every round.
		sinceSeq := c.lastAppliedSeq.Load() + 1
		stream, err := c.client.CatchUp(ctx, &chatpb.CatchUpRequest{
			SinceSeq: sinceSeq,
			NodeId:   c.nodeID,
			Epoch:    c.epoch,
		})
		if err != nil {
			log.Warn().Err(err).Uint64("since_seq", sinceSeq).Msg("replication: CatchUp request failed")
			c.disconnect()
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		// Receive entries
		backoff = 1 * time.Second
		for {
			entry, err := stream.Recv()
			if err == io.EOF {
				// CatchUp complete — all historical entries received.
				// Wait and retry for new entries.
				log.Debug().Uint64("last_seq", c.lastAppliedSeq.Load()).Msg("replication: CatchUp stream ended, will retry")
				break
			}
			if err != nil {
				log.Warn().Err(err).Msg("replication: stream receive error")
				c.disconnect()
				break
			}

			if err := c.handleEntry(ctx, entry); err != nil {
				c.disconnect()
				return err
			}
		}

		// Brief pause before reconnecting for more entries
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// handleEntry verifies one replicated entry and applies it. A rejected
// entry halts replication: it is counted, logged with its topic and
// sequence, and returned as the error StartReplication ends with, and
// lastAppliedSeq stays below it. A standby that skipped it would advance
// past a signed message it never stored and diverge from the primary with
// only a counter to show for it; one that stops is behind, which is
// visible, and resumes from the same entry once the policy or the WAL is
// put right. An apply error (the local store) is logged and retried on the
// next round, as before.
func (c *ReplicationClient) handleEntry(ctx context.Context, entry *chatpb.WALEntry) error {
	if entry.Sequence <= c.lastAppliedSeq.Load() {
		return nil // already handled (a primary that re-sends from an inclusive seq)
	}
	if err := VerifyEntry(c.policy, entry); err != nil {
		reason, topic := RejectSignature, ""
		var re *RejectError
		if errors.As(err, &re) {
			reason, topic = re.Reason, re.Topic
		}
		replicationRejected.WithLabelValues(reason).Inc()
		c.haltedAtSeq.Store(entry.Sequence)
		log.Error().Err(err).
			Str("reason", reason).
			Str("topic", topic).
			Uint64("seq", entry.Sequence).
			Str("peer_node", entry.NodeId).
			Msg("replication: entry rejected, replication halted")
		return err
	}
	if c.ApplyFunc != nil {
		if err := c.ApplyFunc(ctx, entry); err != nil {
			log.Error().Err(err).Uint64("seq", entry.Sequence).Msg("replication: apply failed")
			return nil // retried on the next round
		}
	}
	c.lastAppliedSeq.Store(entry.Sequence)
	c.entriesRecv.Add(1)
	return nil
}

// Ping sends a health check to the primary.
func (c *ReplicationClient) Ping(ctx context.Context) (*chatpb.ReplicationPongResponse, error) {
	if c.client == nil {
		return nil, fmt.Errorf("replication_client: not connected")
	}
	return c.client.ReplicationPing(ctx, &chatpb.ReplicationPingRequest{
		NodeId:  c.nodeID,
		Role:    "standby",
		LastSeq: c.lastAppliedSeq.Load(),
		Epoch:   c.epoch,
	})
}

// disconnect closes the gRPC connection.
func (c *ReplicationClient) disconnect() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.client = nil
	}
}

// Close shuts down the replication client.
func (c *ReplicationClient) Close() error {
	c.disconnect()
	return nil
}

// Stats returns replication client statistics.
func (c *ReplicationClient) Stats() map[string]interface{} {
	return map[string]interface{}{
		"node_id":          c.nodeID,
		"role":             "standby",
		"peer":             c.peerAddr,
		"epoch":            c.epoch,
		"last_applied_seq": c.lastAppliedSeq.Load(),
		"halted_at_seq":    c.haltedAtSeq.Load(),
		"entries_received": c.entriesRecv.Load(),
		"connected":        c.client != nil,
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
