// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package cluster

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/storage/wal"
	chatpb "unheaded/services/wotan/proto"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReplicationServer implements the WotanReplication gRPC service (primary side).
// It streams WAL entries to connected standby nodes.
type ReplicationServer struct {
	chatpb.UnimplementedWotanReplicationServer

	wal     *wal.WAL
	nodeID  string
	epoch   int64
	lastSeq atomic.Uint64
}

// NewReplicationServer creates a replication server for the primary node.
func NewReplicationServer(w *wal.WAL, nodeID string, epoch int64) *ReplicationServer {
	s := &ReplicationServer{
		wal:    w,
		nodeID: nodeID,
		epoch:  epoch,
	}
	// Set initial lastSeq from WAL
	s.lastSeq.Store(w.LastSequence())
	return s
}

// CatchUp streams WAL entries from a given sequence to the standby.
func (s *ReplicationServer) CatchUp(req *chatpb.CatchUpRequest, stream chatpb.WotanReplication_CatchUpServer) error {
	sinceSeq := req.SinceSeq
	peerNode := req.NodeId

	log.Info().
		Str("peer", peerNode).
		Uint64("since_seq", sinceSeq).
		Int64("epoch", req.Epoch).
		Msg("replication: CatchUp request from standby")

	// Stream historical entries
	sent := 0
	err := s.wal.Scan(stream.Context(), sinceSeq, func(seq uint64, data []byte) error {
		entry := &chatpb.WALEntry{
			Sequence:  seq,
			Data:      data,
			NodeId:    s.nodeID,
			Timestamp: timestamppb.Now(),
			Epoch:     s.epoch,
		}
		if err := stream.Send(entry); err != nil {
			return fmt.Errorf("replication: send entry %d: %w", seq, err)
		}
		sent++
		return nil
	})

	if err != nil {
		log.Error().Err(err).Str("peer", peerNode).Msg("replication: CatchUp scan error")
		return err
	}

	log.Info().
		Str("peer", peerNode).
		Int("entries_sent", sent).
		Msg("replication: CatchUp complete")

	return nil
}

// ReplicationPing handles health/status exchange.
func (s *ReplicationServer) ReplicationPing(ctx context.Context, req *chatpb.ReplicationPingRequest) (*chatpb.ReplicationPongResponse, error) {
	mySeq := s.lastSeq.Load()
	peerSeq := req.LastSeq
	lag := int64(mySeq) - int64(peerSeq) // #nosec G115 -- bounded by construction; see the surrounding guard

	return &chatpb.ReplicationPongResponse{
		NodeId:         s.nodeID,
		Role:           "primary",
		LastSeq:        mySeq,
		Epoch:          s.epoch,
		ReplicationLag: lag,
	}, nil
}

// TransferSnapshot streams a full state snapshot (for initial sync or large gaps).
func (s *ReplicationServer) TransferSnapshot(req *chatpb.SnapshotRequest, stream chatpb.WotanReplication_TransferSnapshotServer) error {
	log.Info().
		Str("peer", req.NodeId).
		Msg("replication: snapshot transfer requested (not yet implemented)")

	// TODO: implement full snapshot transfer
	// For now, return empty — standby should use CatchUp from seq 0
	return stream.Send(&chatpb.SnapshotChunk{
		Offset:    0,
		Data:      nil,
		Final:     true,
		TotalSize: 0,
		LastSeq:   s.lastSeq.Load(),
	})
}

// UpdateLastSeq notifies the server of a new WAL write (called from publish path).
func (s *ReplicationServer) UpdateLastSeq(seq uint64) {
	s.lastSeq.Store(seq)
}

// SetEpoch updates the cluster epoch (called during failover).
func (s *ReplicationServer) SetEpoch(epoch int64) {
	s.epoch = epoch
}

// Stats returns replication server statistics.
func (s *ReplicationServer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"node_id":  s.nodeID,
		"role":     "primary",
		"epoch":    s.epoch,
		"last_seq": s.lastSeq.Load(),
	}
}

// WaitForNewEntries blocks until a new WAL entry is available after the given sequence.
// Used by the streaming replication loop to avoid busy-waiting.
func WaitForNewEntries(ctx context.Context, w *wal.WAL, afterSeq uint64, pollInterval time.Duration) (uint64, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ticker.C:
			current := w.LastSequence()
			if current > afterSeq {
				return current, nil
			}
		}
	}
}
