// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// Signed drift events (ADR-105, ADR-043 #2): each drift alert is published
// to Wotan topic drift.detected.<node_id>, signed with Heimdall's own
// ML-DSA-65 drift key (never the manifest key: ADR-043 #4). Wotan refuses
// unsigned or wrongly-keyed drift.* publishes, and any subscriber can
// re-verify an event with pkg/wotansig.VerifyEvent. Publishing is
// best-effort and never blocks the scan: a bounded queue drops its oldest
// record when Wotan is slow or down, and every drop and failure is counted.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/enkrateia"
	"unheaded/pkg/gungnir"
	"unheaded/pkg/metrics"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotansig"
)

// heimdallSender is the name Heimdall joins drift topics under: the
// display_name Wotan authorizes (topics.auto_approve) and the sender its
// signatures cover. The node is named by the topic and the per-node key.
const heimdallSender = "heimdall"

// maxNodeIDLen bounds a node ID (a DNS name is at most 253 bytes).
const maxNodeIDLen = 253

// publishTimeout bounds one publish, so a hung Wotan costs one record's
// slot, not the queue.
const publishTimeout = 5 * time.Second

// ErrKeyReuse refuses a drift key that is also a manifest key (ADR-043 #4).
var ErrKeyReuse = errors.New("the drift signing key must be separate from the manifest keys (ADR-043 #4)")

// ErrBadNodeID refuses a node ID with nothing left after normalization.
var ErrBadNodeID = errors.New("node ID is empty after normalization to [a-z0-9-]")

// DriftPublisher publishes a signed message (wotanClient.GRPCClient).
type DriftPublisher interface {
	PublishSigned(ctx context.Context, topic string, payload []byte, signer *wotansig.TopicSigner) error
}

// DriftRecord is the JSON body of a drift.detected.<node_id> event.
type DriftRecord struct {
	NodeID          string    `json:"node_id"`
	Path            string    `json:"path"`
	Kind            string    `json:"kind"` // "missing" or "modified"
	ExpectedSHA256  string    `json:"expected_sha256"`
	ActualSHA256    string    `json:"actual_sha256"`
	DetectedAt      time.Time `json:"detected_at"`
	ManifestName    string    `json:"manifest_name"`
	ManifestVersion string    `json:"manifest_version"`
}

// NormalizeNodeID maps a node ID (usually a hostname) to one topic segment
// of [a-z0-9-]: lower-cased, every other character becomes "-", runs of "-"
// collapse and the ends are trimmed. A hostname with dots, wildcards or
// uppercase cannot add or alter topic segments.
func NormalizeNodeID(id string) (string, error) {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(id) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "", ErrBadNodeID
	}
	if len(out) > maxNodeIDLen {
		return "", fmt.Errorf("node ID longer than %d bytes", maxNodeIDLen)
	}
	return out, nil
}

// DriftTopic is the topic a normalized node publishes drift on.
func DriftTopic(nodeID string) string { return "drift.detected." + nodeID }

// loadDriftSigner loads the drift key seed (0600, gungnir.LoadSeedFile) and
// refuses it when it is the manifest signing key (signKeyPath) or the key
// that verifies the manifest (manifestPub): ADR-043 #4.
func loadDriftSigner(path, signKeyPath string, manifestPub *mldsa65.PublicKey) (*wotansig.TopicSigner, error) {
	priv, err := gungnir.LoadSeedFile(path)
	if err != nil {
		return nil, fmt.Errorf("drift key: %w", err)
	}
	pub, ok := priv.Public().(*mldsa65.PublicKey)
	if !ok {
		return nil, errors.New("drift key: no public key")
	}
	id := gungnir.KeyID(pub)
	if manifestPub != nil && gungnir.KeyID(manifestPub) == id {
		return nil, fmt.Errorf("%w: --drift-sign-key is the --pubkey key %s", ErrKeyReuse, id)
	}
	if signKeyPath != "" {
		other, err := gungnir.LoadSeedFile(signKeyPath)
		if err != nil {
			return nil, fmt.Errorf("--sign-key: %w", err)
		}
		if op, ok := other.Public().(*mldsa65.PublicKey); ok && gungnir.KeyID(op) == id {
			return nil, fmt.Errorf("%w: --drift-sign-key is the --sign-key key %s", ErrKeyReuse, id)
		}
	}
	s, err := pqc.NewMLDSASignerFromPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("drift key: %w", err)
	}
	return wotansig.NewTopicSigner(s), nil
}

// driftSink is the bounded, drop-oldest queue between the alert drain and
// Wotan.
type driftSink struct {
	pub    DriftPublisher
	signer *wotansig.TopicSigner
	topic  string
	size   int

	mu    sync.Mutex
	queue []DriftRecord
	wake  chan struct{}

	published *metrics.Counter
	failed    *metrics.Counter
	dropped   *metrics.Counter
}

// newDriftSink counts into m's counters (the ones /metrics serves); a nil m
// gets a private set, for tests that read the counters directly.
func newDriftSink(pub DriftPublisher, signer *wotansig.TopicSigner, topic string, size int, m *heimdallMetrics) *driftSink {
	if size < 1 {
		size = 1
	}
	if m == nil {
		m = newHeimdallMetrics()
	}
	return &driftSink{
		pub: pub, signer: signer, topic: topic, size: size,
		wake:      make(chan struct{}, 1),
		published: m.published,
		failed:    m.failed,
		dropped:   m.dropped,
	}
}

// Enqueue queues a record without waiting; when the queue is full the
// oldest record is dropped and counted.
func (s *driftSink) Enqueue(r DriftRecord) {
	s.mu.Lock()
	if len(s.queue) >= s.size {
		s.queue = s.queue[1:]
		s.dropped.Inc()
	}
	s.queue = append(s.queue, r)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Len is the number of queued records.
func (s *driftSink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

func (s *driftSink) next() (DriftRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return DriftRecord{}, false
	}
	r := s.queue[0]
	s.queue = s.queue[1:]
	return r, true
}

// run publishes queued records until ctx ends.
func (s *driftSink) run(ctx context.Context) {
	for {
		r, ok := s.next()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
				continue
			}
		}
		body, err := json.Marshal(r)
		if err != nil {
			s.failed.Inc()
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, publishTimeout)
		err = s.pub.PublishSigned(pctx, s.topic, body, s.signer)
		cancel()
		if err != nil {
			s.failed.Inc()
			if ctx.Err() != nil {
				return
			}
			continue
		}
		s.published.Inc()
	}
}

// maxPublishQueue bounds --publish-queue.
const maxPublishQueue = 1 << 16

// setupDriftPublishing connects the drift sink to Wotan when --wotan-grpc
// is given. Without it drift goes to stdout only, and the startup line says
// so. --wotan-grpc without --drift-sign-key is refused: Wotan takes only
// signed drift.* events, so an unsigned publisher would fail every alert.
func setupDriftPublishing(ctx context.Context, addr, keyPath, signKeyPath string, manifestPub *mldsa65.PublicKey, nodeID string, queue int, m *heimdallMetrics) (*driftSink, error) {
	switch {
	case addr == "" && keyPath == "":
		fmt.Println("Drift alerts are NOT published to Wotan (no --wotan-grpc): stdout only")
		return nil, nil
	case addr == "":
		return nil, errors.New("--drift-sign-key given without --wotan-grpc")
	case keyPath == "":
		return nil, errors.New("--wotan-grpc needs --drift-sign-key: drift.* takes only signed events")
	case queue < 1 || queue > maxPublishQueue:
		return nil, fmt.Errorf("--publish-queue %d outside [1, %d]", queue, maxPublishQueue)
	}
	signer, err := loadDriftSigner(keyPath, signKeyPath, manifestPub)
	if err != nil {
		return nil, err
	}
	gc, err := wotanClient.NewGRPCClient(addr)
	if err != nil {
		return nil, fmt.Errorf("wotan: %w", err)
	}
	topic := DriftTopic(nodeID)
	if _, err := gc.Subscribe(ctx, topic, heimdallSender); err != nil {
		return nil, fmt.Errorf("wotan: join %s: %w", topic, err)
	}
	fmt.Printf("Drift alerts published to %s via %s as %q, signed (ML-DSA-65)\n", topic, addr, heimdallSender)
	return newDriftSink(gc, signer, topic, queue, m), nil
}

// driftRecord turns an alert into the published record.
func (h *Heimdall) driftRecord(a enkrateia.Alert) DriftRecord {
	r := DriftRecord{
		NodeID:         h.nodeID,
		Path:           a.Event.Path,
		Kind:           "modified",
		ExpectedSHA256: string(a.Event.HashExpected),
		ActualSHA256:   string(a.Event.HashActual),
		DetectedAt:     a.Event.DetectedAt.UTC(),
	}
	if len(a.Event.HashActual) == 0 {
		r.Kind = "missing"
	}
	h.mu.RLock()
	if h.manifest != nil {
		r.ManifestName = h.manifest.Metadata.Name
		r.ManifestVersion = h.manifest.Metadata.Version
	}
	h.mu.RUnlock()
	return r
}
