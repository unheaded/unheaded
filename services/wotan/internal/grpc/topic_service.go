// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"unheaded/pkg/auth"
	"unheaded/services/wotan/internal/logger"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/ringbuffer"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/store"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// TopicStreamsLabel is the room_id label under which open topic streams
// count in wotan_streams_active and wotan_streams_total. The pattern is
// chosen by the client, so it is not used as a label value (unbounded
// cardinality); room streams keep their room ID.
const TopicStreamsLabel = "_topic_streams"

// TopicService implements the TopicStream gRPC service for topic-based pub/sub.
// This is the gRPC equivalent of the HTTP topic handlers in api/topics.go.
// It uses the same underlying wotan.Wotan engine, room.Manager, and member.Manager.
type TopicService struct {
	chatpb.UnimplementedTopicStreamServer
	roomManager   *room.Manager
	memberManager *member.Manager
	wotan         *wotan.Wotan

	// NodeID identifies this Wotan instance in cluster mode
	NodeID string

	// Track active streams for metrics/TopicPing
	activeStreams atomic.Int64

	// Signing policy: which topic prefixes take only ML-DSA-65 signed
	// messages and which keys are trusted for each (ADR-043 #2, ADR-105).
	// Nil still refuses unsigned config.*/drift.* and verifies nothing.
	policy *signing.Policy

	// AutoApprove gates anonymous publishers the same way the HTTP path gates
	// subscribers (configs/wotan.yaml -> topics.auto_approve). Nil means "no
	// allowlist configured", which is treated as approve-all so that embedding
	// callers and tests keep working; cmd/wotan always wires it.
	AutoApprove AutoApprover

	// RequireIdentity: AUTH_ENABLED=true. Every call must carry the
	// identity AuthServerOptions verified; one without it (an embedding
	// without the interceptors) is Unauthenticated, never approved by
	// display name.
	RequireIdentity bool

	// Store is the persistent message store the HTTP publish path also
	// writes to (nil: memory-only, as in tests). Without it, gRPC-only
	// publishers (signed config.* and drift.*) lived only in the ring buffer
	// and were gone after a restart. The store keeps no signature (FND-020),
	// so a replayed signed message no longer verifies: it fails closed.
	Store store.MessageStore
}

// persistTimeout bounds one background store write for a published
// message (the HTTP path uses the same bound).
const persistTimeout = 10 * time.Second

// defaultPublisherDisplayName matches the HTTP path's default for a caller
// that supplies no name (services/wotan/internal/api/topics.go).
const defaultPublisherDisplayName = "service"

// AutoApprover reports whether a caller is on Wotan's auto-approve
// allowlist. *api.TopicConfig satisfies it; declaring the interface here
// keeps internal/grpc from importing internal/api.
//
// identity is the caller auth verified (nil with auth off): when set, its
// subject decides and displayName, which is self-reported, is a label
// only (ADR-103 §6).
type AutoApprover interface {
	IsAutoApproved(identity *auth.Identity, displayName string) bool
}

// callerIdentity returns the verified caller, or an Unauthenticated error
// when the service requires one and the call has none.
func (s *TopicService) callerIdentity(ctx context.Context) (*auth.Identity, error) {
	id := auth.IdentityFromContext(ctx)
	if id == nil && s.RequireIdentity {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	return id, nil
}

// NewTopicService creates a new TopicService. signingKeys are the ML-DSA-65
// public keys trusted to sign config.* messages; with none, every config.*
// publish is refused. drift.* trusts no key until SetSigningPolicy names
// some, so every drift.* publish is refused.
func NewTopicService(
	roomManager *room.Manager,
	memberManager *member.Manager,
	msgWotan *wotan.Wotan,
	signingKeys ...[]byte,
) *TopicService {
	policy, _ := signing.NewPolicy(signingKeys) // cannot fail without configured prefixes; a nil policy fails closed anyway
	return &TopicService{
		roomManager:   roomManager,
		memberManager: memberManager,
		wotan:         msgWotan,
		policy:        policy,
	}
}

// SetSigningPolicy replaces the signing policy (built from configs/wotan.yaml
// topics.signing_keys and topics.signed_prefixes). Call it before serving.
func (s *TopicService) SetSigningPolicy(p *signing.Policy) {
	s.policy = p
}

// messageEvent is the TopicEvent for a stored message. A signed message
// carries its signature, key, algorithm and signing time, and its sender_id
// is the sender the signature covers, so a subscriber can re-verify it with
// wotansig.VerifyEvent (ADR-105). Before, the signature was checked and
// dropped: no subscriber ever received one.
func (s *TopicService) messageEvent(topic string, msg *ringbuffer.Message, createdAt time.Time, typ chatpb.TopicEventType) *chatpb.TopicEvent {
	ev := &chatpb.TopicEvent{
		Topic:     topic,
		MessageId: msg.ID.String(),
		SenderId:  msg.CreatorID.String(),
		Payload:   []byte(msg.Content),
		CreatedAt: timestamppb.New(createdAt),
		Seq:       msg.Seq,
		Type:      typ,
		NodeId:    s.NodeID,
	}
	if sg := msg.Signed; sg != nil && typ == chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED {
		ev.SenderId = sg.Sender
		ev.Signature = sg.Signature
		ev.PublicKey = sg.PublicKey
		ev.Algorithm = sg.Algorithm
		ev.SignedAtUnixMs = sg.SignedAtUnixMs
	}
	return ev
}

// StreamTopics implements the gRPC StreamTopics method for topic-based streaming.
// Subscribes to topics matching the pattern and streams published messages.
func (s *TopicService) StreamTopics(
	req *chatpb.TopicStreamRequest,
	stream chatpb.TopicStream_StreamTopicsServer,
) error {
	streamCtx := stream.Context()

	// Validate request
	if req.TopicPattern == "" {
		logger.FromContext(streamCtx).Warn().Msg("topic_pattern_required")
		return status.Error(codes.InvalidArgument, "topic_pattern is required")
	}

	// Wrap the stream context in a cancellable context so that all spawned
	// goroutines exit when StreamTopics returns (e.g. on historical send error).
	ctx, cancel := context.WithCancel(streamCtx)
	defer cancel()

	identity, err := s.callerIdentity(streamCtx)
	if err != nil {
		return err
	}

	// Set default display name. With auth on the member is the verified
	// subject; the display name is a label.
	displayName := req.DisplayName
	if displayName == "" {
		displayName = "grpc-subscriber"
	}
	memberName := displayName
	if identity != nil {
		memberName = identity.Subject
	}

	// Create a temporary topic for the subscription (use a pattern-based name)
	// This allows multiple subscriptions to the same pattern
	subscriptionTopic := fmt.Sprintf("_topic_pattern_%s_%s", req.TopicPattern, uuid.New().String())

	// Auto-create and approve subscriber member
	newMember, err := s.memberManager.RequestJoin(subscriptionTopic, memberName, "", 24*time.Hour)
	if err != nil {
		logger.FromContext(ctx).Warn().Err(err).Str("topic_pattern", req.TopicPattern).Msg("subscriber_refused")
		return status.Error(codes.ResourceExhausted, err.Error())
	}
	if err := s.memberManager.Approve(newMember.ID, "topic-auto-approve"); err != nil {
		logger.FromContext(ctx).Error().
			Err(err).
			Str("topic_pattern", req.TopicPattern).
			Msg("failed_to_approve_topic_subscriber")
		return status.Error(codes.Internal, "failed to create subscriber")
	}

	memberID := newMember.ID
	// The member exists for this stream only (its room name is unique to it):
	// without this, every reconnect left one behind for good.
	defer s.memberManager.Remove(memberID)

	logger.FromContext(ctx).Info().
		Str("member_id", memberID.String()).
		Str("topic_pattern", req.TopicPattern).
		Str("member", memberName).
		Str("display_name", displayName).
		Msg("topic_stream_started")

	s.activeStreams.Add(1)
	defer s.activeStreams.Add(-1)
	// wotan_streams_active counted only room streams; every service streams
	// topics, so it read 0 with the kingdom subscribed.
	metrics.StreamsActive.WithLabelValues(TopicStreamsLabel).Inc()
	defer metrics.StreamsActive.WithLabelValues(TopicStreamsLabel).Dec()
	metrics.StreamsTotal.WithLabelValues(TopicStreamsLabel).Inc()

	// Every message from here on has a higher seq. A topic the watcher finds
	// later replays its messages above this, which it took before the
	// watcher subscribed (see watchTopicsForPattern).
	startSeq := s.roomManager.LastSeq()

	// Get initial list of topics matching the pattern
	allRooms := s.roomManager.List()
	var matchingTopics []string
	for _, rm := range allRooms {
		if MatchTopic(req.TopicPattern, rm.ID) {
			matchingTopics = append(matchingTopics, rm.ID)
		}
	}

	// Track active subscriptions per topic. The watcher adds to the map, so
	// teardown first stops it and every forwarder (cancel, then wait), and
	// only then unsubscribes. Cleaning up while they ran raced on the map and
	// left any subscription the watcher made meanwhile in Wotan for good.
	subscriptions := make(map[string]*wotan.Subscription)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
		for _, sub := range subscriptions {
			s.wotan.Unsubscribe(sub)
		}
	}()

	// Subscribe to initial matching topics
	for _, topicName := range matchingTopics {
		sub := s.wotan.Subscribe(topicName, memberID, 100)
		subscriptions[topicName] = sub
		logger.FromContext(ctx).Debug().
			Str("topic", topicName).
			Msg("subscribed_to_topic")
	}

	// Create a shared channel for fanning in events from all subscriptions.
	// It is never closed: closing it here (the last defer, so the first to
	// run, before cancel) let a forwarder still holding an event send on a
	// closed channel, a panic that took all of Wotan down.
	eventChan := make(chan *topicEvent, 100)

	// Start a goroutine for each subscription to forward events to the shared channel
	for topicName, sub := range subscriptions {
		workers.Add(1)
		go func() {
			defer workers.Done()
			s.forwardTopicEvents(ctx, topicName, sub, eventChan)
		}()
	}

	// Start dynamic watcher goroutine to discover new topics matching the pattern
	workers.Add(1)
	go func() {
		defer workers.Done()
		s.watchTopicsForPattern(ctx, memberID, req.TopicPattern, startSeq, subscriptions, eventChan, &workers)
	}()

	// Replay historical messages if requested (SinceSeq >= 0 replays from that sequence)
	if req.SinceSeq >= 0 {
		// Seqs are global across topics and in memory. A cursor above the
		// newest seq this process has assigned is from before a restart:
		// replay what is held, or messages published between the restart and
		// the reconnect are lost. (Before seqs were global this could only be
		// judged for single-topic streams.)
		upper := s.roomManager.LastSeq()
		since := req.SinceSeq
		if since > upper {
			since = 0
		}

		// Every matching topic's messages after the cursor, merged in seq
		// order, so a replay cut short never leaves a lower seq unsent behind
		// the client's cursor. Seqs above upper may still be landing in a
		// ring already read; the live subscription delivers them.
		type held struct {
			topic string
			msg   *ringbuffer.Message
		}
		var replay []held
		for _, topicName := range matchingTopics {
			rm, err := s.roomManager.Get(topicName)
			if err != nil {
				continue
			}
			for _, msg := range rm.Buffer.GetAfter(since, 0) {
				if msg.Seq > upper {
					break
				}
				replay = append(replay, held{topicName, msg})
			}
		}
		sort.Slice(replay, func(i, j int) bool { return replay[i].msg.Seq < replay[j].msg.Seq })

		for _, h := range replay {
			topicName, msg := h.topic, h.msg
			event := s.messageEvent(topicName, msg, msg.Timestamp, chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED)

			if err := stream.Send(event); err != nil {
				logger.FromContext(ctx).Error().
					Err(err).
					Str("topic", topicName).
					Msg("stream_send_error_historical")
				metrics.StreamErrors.WithLabelValues(topicName, "send_error").Inc()
				return status.Error(codes.Aborted, "failed to send message")
			}

			metrics.StreamMessagesSent.WithLabelValues(topicName, "MESSAGE_PUBLISHED").Inc()
		}
	}

	// Stream live messages
	for {
		select {
		case <-ctx.Done():
			logger.FromContext(ctx).Info().
				Str("member_id", memberID.String()).
				Str("topic_pattern", req.TopicPattern).
				Msg("stream_closed_by_client")
			return status.Error(codes.Canceled, "client disconnected")

		case event, ok := <-eventChan:
			if !ok {
				logger.FromContext(ctx).Info().
					Str("member_id", memberID.String()).
					Str("topic_pattern", req.TopicPattern).
					Msg("stream_closed_by_server")
				return status.Error(codes.Unavailable, "stream closed by server")
			}

			// Send to client
			if err := stream.Send(event.pbEvent); err != nil {
				logger.FromContext(ctx).Error().
					Err(err).
					Str("member_id", memberID.String()).
					Str("topic", event.topic).
					Msg("stream_send_error")
				metrics.StreamErrors.WithLabelValues(event.topic, "send_error").Inc()
				return status.Error(codes.Aborted, "failed to send message")
			}

			logger.FromContext(ctx).Debug().
				Str("member_id", memberID.String()).
				Str("topic", event.topic).
				Str("message_id", event.pbEvent.MessageId).
				Msg("stream_message_sent")

			metrics.StreamMessagesSent.WithLabelValues(event.topic, "MESSAGE_PUBLISHED").Inc()
		}
	}
}

// topicEvent is a helper struct for fan-in of events from multiple subscriptions
type topicEvent struct {
	topic   string
	pbEvent *chatpb.TopicEvent
}

// forwardTopicEvents forwards events from a subscription to a shared channel
func (s *TopicService) forwardTopicEvents(
	ctx context.Context,
	topic string,
	sub *wotan.Subscription,
	eventChan chan<- *topicEvent,
) {
	s.forwardTopicEventsAfter(ctx, topic, sub, eventChan, 0)
}

// forwardTopicEventsAfter is forwardTopicEvents skipping created messages
// with seq <= skipThrough: ones already replayed from the ring.
func (s *TopicService) forwardTopicEventsAfter(
	ctx context.Context,
	topic string,
	sub *wotan.Subscription,
	eventChan chan<- *topicEvent,
	skipThrough int64,
) {
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-sub.Channel:
			if !ok {
				return
			}
			if skipThrough > 0 && event.Type == wotan.EventMessageCreated && event.Message.Seq <= skipThrough {
				continue
			}

			// The seq the ring buffer assigned. A counter bumped here, once
			// per forwarding subscriber, gave each subscriber different seqs.
			seq := event.Message.Seq

			// A deletion carries only the message ID; it was sent as a
			// PUBLISHED event, a phantom empty message with seq 0.
			eventType := chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED
			if event.Type == wotan.EventMessageDeleted {
				eventType = chatpb.TopicEventType_TOPIC_MESSAGE_DELETED
			}

			pbEvent := s.messageEvent(topic, event.Message, event.Timestamp, eventType)
			pbEvent.Seq = seq

			// Non-blocking send to event channel
			select {
			case eventChan <- &topicEvent{
				topic:   topic,
				pbEvent: pbEvent,
			}:
				// Event sent successfully
			case <-ctx.Done():
				return
			default:
				// Event channel buffer full (the client is slower than the
				// topic), drop event. Acceptable for streaming pub/sub, but
				// counted: a drop was invisible before.
				if m := metrics.Get(); m != nil {
					m.StreamErrors.WithLabelValues(topic, "stream_buffer_full").Inc()
				}
			}
		}
	}
}

// watchTopicsForPattern periodically checks for new topics matching the pattern
// and subscribes to them, sending TOPIC_SUBSCRIPTION_ADDED events. Each
// forwarder it starts is counted in workers, so StreamTopics can wait for
// all of them before tearing the subscriptions down.
//
// A topic's room is created by its first publish, and found here up to a
// tick later, so what it took before the subscription (its first message
// at least) had no subscriber to reach. Those messages, every one above
// startSeq, are replayed from the ring before its forwarder starts; the
// forwarder skips them if they also arrive live.
func (s *TopicService) watchTopicsForPattern(
	ctx context.Context,
	memberID uuid.UUID,
	pattern string,
	startSeq int64,
	subscriptions map[string]*wotan.Subscription,
	eventChan chan<- *topicEvent,
	workers *sync.WaitGroup,
) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// Get current list of all topics
			allRooms := s.roomManager.List()

			// Check for new topics matching the pattern
			for _, rm := range allRooms {
				if MatchTopic(pattern, rm.ID) {
					// Check if we're already subscribed
					if _, exists := subscriptions[rm.ID]; !exists {
						// Subscribe to this new topic
						sub := s.wotan.Subscribe(rm.ID, memberID, 100)
						subscriptions[rm.ID] = sub

						logger.FromContext(ctx).Info().
							Str("topic", rm.ID).
							Str("pattern", pattern).
							Msg("discovered_new_topic")

						// Send TOPIC_SUBSCRIPTION_ADDED event
						event := &chatpb.TopicEvent{
							Topic:     rm.ID,
							Type:      chatpb.TopicEventType_TOPIC_SUBSCRIPTION_ADDED,
							CreatedAt: timestamppb.Now(),
						}

						// Non-blocking send
						select {
						case eventChan <- &topicEvent{
							topic:   rm.ID,
							pbEvent: event,
						}:
							// Event sent successfully
						case <-ctx.Done():
							return
						default:
							// Event channel buffer full, skip notification
						}

						// Replay after subscribing: a message pushed before the
						// subscription is in the ring; one pushed after it
						// arrives live (and is skipped if both).
						var through int64
						for _, msg := range rm.Buffer.GetAfter(startSeq, 0) {
							select {
							case eventChan <- &topicEvent{topic: rm.ID, pbEvent: s.messageEvent(rm.ID, msg, msg.Timestamp, chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED)}:
							case <-ctx.Done():
								return
							}
							through = msg.Seq
						}

						// Start forwarding events from this subscription
						workers.Add(1)
						go func(topic string) {
							defer workers.Done()
							s.forwardTopicEventsAfter(ctx, topic, sub, eventChan, through)
						}(rm.ID)
					}
				}
			}
		}
	}
}

// PublishTopic implements the gRPC PublishTopic method.
// Publishes a message to a topic and returns metadata including message ID and sequence number.
func (s *TopicService) PublishTopic(
	ctx context.Context,
	req *chatpb.TopicPublishRequest,
) (*chatpb.TopicPublishResponse, error) {
	// Validate request
	if req.Topic == "" {
		logger.FromContext(ctx).Warn().Msg("topic_required")
		return nil, status.Error(codes.InvalidArgument, "topic is required")
	}

	if len(req.Payload) == 0 {
		logger.FromContext(ctx).Warn().
			Str("topic", req.Topic).
			Msg("payload_required")
		return nil, status.Error(codes.InvalidArgument, "payload is required")
	}

	identity, err := s.callerIdentity(ctx)
	if err != nil {
		return nil, err
	}

	// Authorize the sender first (read-only checks; nothing is created yet).
	var senderID uuid.UUID
	var autoPublisher string // set when the caller sent no sender_id
	if req.SenderId != "" {
		var err error
		senderID, err = uuid.Parse(req.SenderId)
		if err != nil {
			logger.FromContext(ctx).Warn().
				Err(err).
				Str("topic", req.Topic).
				Str("sender_id", req.SenderId).
				Msg("invalid_sender_id")
			return nil, status.Error(codes.InvalidArgument, "invalid sender_id")
		}

		// Verify sender is approved
		if !s.memberManager.IsApproved(senderID) {
			logger.FromContext(ctx).Warn().
				Str("topic", req.Topic).
				Str("sender_id", senderID.String()).
				Msg("sender_not_approved")
			return nil, status.Error(codes.PermissionDenied, "sender not approved")
		}
		// A sender_id is no secret (every subscriber sees it as a
		// message's sender_id): with auth on, a caller publishes only
		// through its own members, which carry its verified subject.
		if identity != nil {
			if m, err := s.memberManager.Get(senderID); err != nil || m.Name != identity.Subject {
				logger.FromContext(ctx).Warn().
					Str("topic", req.Topic).
					Str("caller", identity.Subject).
					Msg("sender_belongs_to_another_identity")
				return nil, status.Error(codes.PermissionDenied, "sender belongs to another identity")
			}
		}
	} else {
		// No sender_id. Previously this branch created a member and approved it
		// unconditionally, so the IsApproved check above was bypassed simply by
		// omitting the field. Gate it on the same allowlist the HTTP path uses,
		// with the same "service" default for an unnamed internal caller.
		//
		// With auth on, the verified subject is the publisher: it decides
		// approval and names the member (and is the sender a signature must
		// cover); display_name is a label only (ADR-103 §6).
		displayName := req.Metadata["display_name"]
		if displayName == "" {
			displayName = defaultPublisherDisplayName
		}
		if s.AutoApprove != nil && !s.AutoApprove.IsAutoApproved(identity, displayName) {
			logger.FromContext(ctx).Warn().
				Str("topic", req.Topic).
				Str("display_name", displayName).
				Msg("publisher_not_auto_approved")
			return nil, status.Error(codes.PermissionDenied, "publisher not approved")
		}

		autoPublisher = displayName
		if identity != nil {
			autoPublisher = identity.Subject
		}
	}

	// ADR-043 #2 / ADR-105: config.* and drift.* (and any configured signed
	// prefix) take only ML-DSA-65 signed messages, each prefix from its own
	// keys. Checked only after the sender is authorized, so a caller the
	// allowlist refuses never costs an ML-DSA verify (LICH-012 L12b: trusted
	// keys are public, so anyone can make a request reach it). The
	// signature is recorded as used (Accept) only once the message is taken,
	// below.
	//
	// The sender a signature covers is the request's sender_id or, for a
	// publisher authorized by display_name, that name: the signature binds
	// the identity Wotan authorized, and subscribers get it back as the
	// event's sender_id to re-verify against.
	var signed *signing.Signed
	var verifier *signing.TopicVerifier
	signedSender := req.SenderId
	if signedSender == "" {
		signedSender = autoPublisher
	}
	if prefix := s.policy.PrefixFor(req.Topic); prefix != "" {
		if len(req.Signature) == 0 || len(req.PublicKey) == 0 {
			logger.FromContext(ctx).Warn().
				Str("topic", req.Topic).
				Msg("signed_topic_unsigned")
			return nil, status.Error(codes.InvalidArgument, prefix+"* topics require ML-DSA-65 signature")
		}
		// Fail closed. No verifier (no policy) refuses signed-prefix
		// publishes, never waves them through unchecked.
		var ok bool
		verifier, ok = s.policy.VerifierFor(req.Topic)
		if !ok {
			logger.FromContext(ctx).Error().
				Str("topic", req.Topic).
				Msg("signed_topic_verifier_unavailable")
			return nil, status.Error(codes.PermissionDenied, "signature verification unavailable")
		}
		signed = &signing.Signed{
			Signature: req.Signature, PublicKey: req.PublicKey, Algorithm: req.Algorithm, SignedAtUnixMs: req.SignedAtUnixMs,
		}
		if err := verifier.Verify(req.Topic, signedSender, req.Payload, *signed); err != nil {
			logger.FromContext(ctx).Warn().
				Err(err).
				Str("topic", req.Topic).
				Msg("signed_topic_signature_invalid")
			return nil, status.Error(codes.PermissionDenied, "signature verification failed: "+err.Error())
		}
	}

	// The unnamed publisher's member: one per (topic, name), made only for a
	// publish that passed every check above.
	if autoPublisher != "" {
		publisher, err := s.memberManager.EnsureApproved(req.Topic, autoPublisher, "topic-auto-approve")
		if err != nil {
			logger.FromContext(ctx).Warn().Err(err).Str("topic", req.Topic).Msg("publisher_refused")
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		}
		senderID = publisher.ID
	}

	if signed != nil {
		if err := verifier.Accept(*signed); err != nil {
			logger.FromContext(ctx).Warn().Err(err).Str("topic", req.Topic).Msg("signed_topic_replay")
			return nil, status.Error(codes.PermissionDenied, "signature verification failed: "+err.Error())
		}
	}

	// The room exists only once a publish is authorized: an unauthorized
	// caller must not be able to create rooms by naming topics.
	rm, err := s.roomManager.GetOrCreate(req.Topic, req.Topic)
	if err != nil {
		logger.FromContext(ctx).Warn().Err(err).Str("topic", req.Topic).Msg("room_refused")
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}

	// Send message to room's ring buffer, with its signature if it has one.
	var msg *ringbuffer.Message
	var overwritten bool
	if signed != nil {
		msg, overwritten = rm.SendSignedMessage(senderID, string(req.Payload), &ringbuffer.Signature{
			Sender:         signedSender,
			Signature:      bytes.Clone(signed.Signature),
			PublicKey:      bytes.Clone(signed.PublicKey),
			Algorithm:      signed.Algorithm,
			SignedAtUnixMs: signed.SignedAtUnixMs,
		})
	} else {
		msg, overwritten = rm.SendMessage(senderID, string(req.Payload))
	}

	// Persist like the HTTP path: async, so the caller never waits on disk,
	// and on a context of its own, because the RPC's is cancelled when this
	// returns, before the write lands. A store error is logged, not returned:
	// the message is already in the ring buffer and on its way out.
	if s.Store != nil {
		storeMsg := &store.Message{ID: msg.ID, CreatorID: msg.CreatorID, RoomID: msg.RoomID, Content: msg.Content, Timestamp: msg.Timestamp}
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
		log := logger.FromContext(ctx)
		topic := req.Topic
		go func() {
			defer cancel()
			if _, _, err := s.Store.Push(pctx, storeMsg); err != nil {
				log.Warn().Err(err).Str("topic", topic).Msg("failed to persist message to store")
			}
		}()
	}

	// Publish event through wotan pub/sub
	s.wotan.PublishMessageCreated(msg)

	// The seq the ring buffer assigned: the same number replay pages by.
	seq := msg.Seq

	logger.FromContext(ctx).Debug().
		Str("topic", req.Topic).
		Str("message_id", msg.ID.String()).
		Str("sender_id", senderID.String()).
		Int64("seq", seq).
		Msg("topic_message_published")

	if m := metrics.Get(); m != nil {
		m.RecordMessageCreated(req.Topic, overwritten)
	}

	return &chatpb.TopicPublishResponse{
		MessageId: msg.ID.String(),
		Seq:       seq,
		CreatedAt: timestamppb.New(msg.Timestamp),
	}, nil
}

// TopicPing implements the gRPC TopicPing method for keep-alive and stats.
// Returns status, active stream count, and total topic count.
func (s *TopicService) TopicPing(ctx context.Context, req *chatpb.TopicPingRequest) (*chatpb.TopicPongResponse, error) {
	logger.FromContext(ctx).Debug().Msg("topic_ping_received")

	topicCount := s.roomManager.Count()
	activeStreams := s.activeStreams.Load()

	return &chatpb.TopicPongResponse{
		Status:        "ok",
		Timestamp:     timestamppb.Now(),
		ActiveStreams: activeStreams,
		TotalTopics:   int64(topicCount),
	}, nil
}

// String returns a string representation of the service
func (s *TopicService) String() string {
	return fmt.Sprintf("TopicService{topics=%d, active_streams=%d, subscriptions=%d}",
		s.roomManager.Count(),
		s.activeStreams.Load(),
		s.wotan.GetTotalSubscriptions(),
	)
}
