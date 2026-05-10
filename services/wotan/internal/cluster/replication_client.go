// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package cluster

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"unheaded/pkg/transport/mtls"
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
	entriesRecv    atomic.Uint64

	// ApplyFunc is called for each received WAL entry.
	// The standby wires this to write entries to its local store.
	ApplyFunc func(ctx context.Context, entry *chatpb.WALEntry) error
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
	var dialOpt grpc.DialOption
	if c.pkiDir != "" {
		tlsCfg, err := mtls.SetupServiceClientTLS("wotan", c.pkiDir)
		if err != nil {
			log.Warn().Err(err).Msg("replication: mTLS setup failed, falling back to insecure")
			dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
		} else {
			dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
		}
	} else {
		dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
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

		// Request CatchUp from last applied sequence
		sinceSeq := c.lastAppliedSeq.Load()
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

			// Apply the entry
			if c.ApplyFunc != nil {
				if err := c.ApplyFunc(ctx, entry); err != nil {
					log.Error().Err(err).Uint64("seq", entry.Sequence).Msg("replication: apply failed")
					continue // Don't stop replication on apply error
				}
			}

			c.lastAppliedSeq.Store(entry.Sequence)
			c.entriesRecv.Add(1)
		}

		// Brief pause before reconnecting for more entries
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
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
		c.conn.Close()
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
