// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// The abort switch (S-REAL-STEEL D-18), three independent paths:
//
//  1. POST /api/v1/chaos/abort on chaos-controller (behind auth);
//  2. a message on Wotan's chaos.abort (whoever Wotan lets join the topic);
//  3. the file /run/unheaded/chaos/ABORT: it works with Wotan and HTTP auth
//     both down (`touch` it as root or unheaded-chaos), and while it exists
//     no experiment starts.
//
// Any path cancels every running experiment; each run then reverts its
// fault through the journal. An abort only ever takes chaos away, so a
// chaos.abort message that does not parse is still an abort.

// AbortSource subscribes to a Wotan topic (wotan-client's SubscribeWithRetry).
type AbortSource interface {
	SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*wotanClient.Message)) error
}

// WotanName is the subscriber name chaos-controller joins Wotan as.
const WotanName = "chaos-controller"

// Abort cancels every running experiment and returns how many it stopped.
func (s *Service) Abort(path, reason string) int {
	s.countAbort(path)
	s.mu.RLock()
	ids := make([]string, 0, len(s.runs))
	for id := range s.runs {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	for _, id := range ids {
		s.cancelRun(id, path, reason)
	}
	s.audit.Record(AuditEntry{Timestamp: time.Now().UTC(), Action: "abort", RuleSpec: path + ": " + reason, Result: "stopped " + strconv.Itoa(len(ids))})
	if len(ids) > 0 {
		s.log.Warn().Str("path", path).Str("reason", reason).Int("experiments", len(ids)).Msg("chaos: abort")
	}
	return len(ids)
}

// abortOne cancels one experiment (TTL, a stopped game day).
func (s *Service) abortOne(id, path, reason string) {
	if s.cancelRun(id, path, reason) {
		s.countAbort(path)
	}
}

func (s *Service) cancelRun(id, path, reason string) bool {
	s.mu.RLock()
	r := s.runs[id]
	s.mu.RUnlock()
	if r == nil {
		return false
	}
	r.cancel(errors.New("aborted via " + path + ": " + reason))
	return true
}

func (s *Service) countAbort(path string) {
	s.abortTotal.WithLabelValues(path).Inc()
	s.abortsMu.Lock()
	s.aborts[path]++
	s.abortsMu.Unlock()
}

func (s *Service) abortCount(path string) int {
	s.abortsMu.Lock()
	defer s.abortsMu.Unlock()
	return s.aborts[path]
}

func (s *Service) abortFileExists() bool {
	if s.config.AbortFile == "" {
		return false
	}
	_, err := os.Stat(s.config.AbortFile)
	if err == nil {
		return true
	}
	// Anything but "it is not there" (a permission error, say) counts as
	// present: the abort path fails safe.
	return !errors.Is(err, fs.ErrNotExist)
}

// watchAbortFile polls the abort file (no inotify dependency).
func (s *Service) watchAbortFile(ctx context.Context) {
	t := time.NewTicker(s.config.AbortPoll)
	defer t.Stop()
	for {
		present := s.abortFileExists()
		s.abortPresent.Store(present)
		if present {
			s.mu.RLock()
			n := len(s.runs)
			s.mu.RUnlock()
			if n > 0 {
				s.Abort("file", "abort file "+s.config.AbortFile)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) subscribeAbort(ctx context.Context) {
	err := s.deps.Subscriber.SubscribeWithRetry(ctx, TopicAbort, WotanName, s.HandleAbortMessage)
	if err != nil && ctx.Err() == nil {
		s.log.Warn().Err(err).Msg("chaos: chaos.abort subscription ended")
	}
}

// abortReplayWindow: a chaos.abort created this long before the
// controller started is history, not a request.
const (
	abortReplayWindow = 10 * time.Minute
	maxSeenAborts     = 10000
)

// replayed reports whether msg is a chaos.abort this controller already
// handled, or one from well before it started. Wotan's HTTP polling starts
// at seq 0, so each resubscribe replays the topic's history; without this
// an old abort would stop experiments running now. A message with neither
// an ID nor a time is never treated as a replay: when in doubt, abort.
func (s *Service) replayed(msg *wotanClient.Message) bool {
	if !msg.CreatedAt.IsZero() && msg.CreatedAt.Before(s.startedAt.Add(-abortReplayWindow)) {
		return true
	}
	if msg.MessageID == "" {
		return false
	}
	s.seenMu.Lock()
	defer s.seenMu.Unlock()
	if s.seenAborts[msg.MessageID] {
		return true
	}
	s.seenAborts[msg.MessageID] = true
	s.seenOrder = append(s.seenOrder, msg.MessageID)
	if len(s.seenOrder) > maxSeenAborts {
		delete(s.seenAborts, s.seenOrder[0])
		s.seenOrder = s.seenOrder[1:]
	}
	return false
}

// HandleAbortMessage is the Wotan chaos.abort path.
func (s *Service) HandleAbortMessage(msg *wotanClient.Message) {
	if msg == nil || s.replayed(msg) {
		return
	}
	reason := "chaos.abort"
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(msg.Payload), &body); err == nil && body.Reason != "" {
		if len(body.Reason) > 200 {
			body.Reason = body.Reason[:200]
		}
		reason = body.Reason
	}
	s.Abort("wotan", reason)
}
