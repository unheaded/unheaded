// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package anomaly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sync/atomic"
)

// ScoreTopic carries flow scores into the service (ADR-108; provisional
// name until the RFC Editor answers docs/protocol/RFC-EDITOR-REQUESTS-S5.md
// item 3).
const ScoreTopic = "anomaly.scores"

// MaxScoreMessage bounds one anomaly.scores payload.
const MaxScoreMessage = 4096

// ErrBadScore marks every reason a score message is refused.
var ErrBadScore = errors.New("anomaly: bad score message")

// ScoreMessage is one anomaly.scores payload: flow metadata and a score.
// It has no field that could carry packet payload (ADR-108: the cartridge
// ABI never exposes payload bytes, and this schema rejects unknown fields).
type ScoreMessage struct {
	FlowKey     FlowKey `json:"flow_key"`
	Score       int     `json:"score"`    // 0..100
	ModelID     uint32  `json:"model_id"` // scorer model id
	Source      string  `json:"source"`   // e.g. "cartridge:2" (slot); becomes ServiceID
	PacketCount uint64  `json:"packet_count,omitempty"`
	TimestampMS int64   `json:"timestamp_ms,omitempty"`
}

// scoreCounters are the intake's metrics (per Service).
type scoreCounters struct {
	ingested, rejected atomic.Int64
}

func (s *Service) badScore(format string, a ...any) error {
	s.scores.rejected.Add(1)
	return fmt.Errorf("%w: "+format, append([]any{ErrBadScore}, a...)...)
}

// IngestScore validates one anomaly.scores payload and records it. Input is
// hostile: size-capped, strict JSON (unknown fields and trailing data
// refused), score range, addresses parsed.
func (s *Service) IngestScore(payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxScoreMessage {
		return s.badScore("%d bytes (1..%d)", len(payload), MaxScoreMessage)
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var m ScoreMessage
	if err := dec.Decode(&m); err != nil {
		return s.badScore("%v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return s.badScore("trailing data")
	}
	if m.Score < 0 || m.Score > 100 {
		return s.badScore("score %d outside 0..100", m.Score)
	}
	for _, ip := range []string{m.FlowKey.SrcIP, m.FlowKey.DstIP} {
		if _, err := netip.ParseAddr(ip); err != nil {
			return s.badScore("address %q", ip)
		}
	}
	if len(m.Source) > 64 {
		return s.badScore("source longer than 64 bytes")
	}
	s.RecordFlowScore(&FlowScore{
		FlowKey:      m.FlowKey,
		AnomalyScore: m.Score,
		PacketCount:  m.PacketCount,
		ServiceID:    m.Source,
	})
	s.scores.ingested.Add(1)
	return nil
}

// listenForScores consumes anomaly.scores from Wotan into IngestScore.
func (s *Service) listenForScores(ctx context.Context) {
	if s.wotan == nil {
		return
	}
	ch, err := s.wotan.StreamMessages(ctx, ScoreTopic)
	if err != nil {
		s.log.Warn().Err(err).Str("topic", ScoreTopic).Msg("failed to stream scores")
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if msg == nil {
				continue
			}
			if err := s.IngestScore([]byte(msg.Payload)); err != nil {
				s.log.Warn().Err(err).Str("message_id", msg.MessageID).Msg("score refused")
			}
		}
	}
}
