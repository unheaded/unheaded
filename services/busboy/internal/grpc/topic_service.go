package grpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	chatpb "unheaded/services/busboy/proto"
	"unheaded/services/busboy/internal/busboy"
	"unheaded/services/busboy/internal/logger"
	"unheaded/services/busboy/internal/member"
	"unheaded/services/busboy/internal/metrics"
	"unheaded/services/busboy/internal/room"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TopicSequenceCounter provides monotonic sequence numbers per topic.
// Used to assign sequence numbers to published messages in topic streams.
type TopicSequenceCounter struct {
	mu       sync.RWMutex
	counters map[string]*atomic.Int64
}

// NewTopicSequenceCounter creates a new topic sequence counter.
func NewTopicSequenceCounter() *TopicSequenceCounter {
	return &TopicSequenceCounter{
		counters: make(map[string]*atomic.Int64),
	}
}

// Next returns the next sequence number for a topic.
// Creates a new counter for the topic if it doesn't exist.
func (c *TopicSequenceCounter) Next(topic string) int64 {
	c.mu.RLock()
	counter, ok := c.counters[topic]
	c.mu.RUnlock()

	if ok {
		return counter.Add(1)
	}

	c.mu.Lock()
	// Double-check after acquiring write lock
	if counter, ok = c.counters[topic]; ok {
		c.mu.Unlock()
		return counter.Add(1)
	}
	counter = &atomic.Int64{}
	c.counters[topic] = counter
	c.mu.Unlock()

	return counter.Add(1)
}

// TopicService implements the TopicStream gRPC service for topic-based pub/sub.
// This is the gRPC equivalent of the HTTP topic handlers in api/topics.go.
// It uses the same underlying busboy.Busboy engine, room.Manager, and member.Manager.
type TopicService struct {
	chatpb.UnimplementedTopicStreamServer
	roomManager   *room.Manager
	memberManager *member.Manager
	busboy        *busboy.Busboy
	seqCounter    *TopicSequenceCounter

	// Track active streams for metrics/TopicPing
	activeStreams atomic.Int64
}

// NewTopicService creates a new TopicService.
func NewTopicService(
	roomManager *room.Manager,
	memberManager *member.Manager,
	msgBusboy *busboy.Busboy,
) *TopicService {
	return &TopicService{
		roomManager:   roomManager,
		memberManager: memberManager,
		busboy:        msgBusboy,
		seqCounter:    NewTopicSequenceCounter(),
	}
}

// NewTopicServiceWithCounter creates a new TopicService with a shared sequence counter.
func NewTopicServiceWithCounter(
	roomManager *room.Manager,
	memberManager *member.Manager,
	msgBusboy *busboy.Busboy,
	seqCounter *TopicSequenceCounter,
) *TopicService {
	return &TopicService{
		roomManager:   roomManager,
		memberManager: memberManager,
		busboy:        msgBusboy,
		seqCounter:    seqCounter,
	}
}

// StreamTopics implements the gRPC StreamTopics method for topic-based streaming.
// Subscribes to topics matching the pattern and streams published messages.
func (s *TopicService) StreamTopics(
	req *chatpb.TopicStreamRequest,
	stream chatpb.TopicStream_StreamTopicsServer,
) error {
	ctx := stream.Context()

	// Validate request
	if req.TopicPattern == "" {
		logger.FromContext(ctx).Warn().Msg("topic_pattern_required")
		return status.Error(codes.InvalidArgument, "topic_pattern is required")
	}

	// Set default display name
	displayName := req.DisplayName
	if displayName == "" {
		displayName = "grpc-subscriber"
	}

	// Create a temporary topic for the subscription (use a pattern-based name)
	// This allows multiple subscriptions to the same pattern
	subscriptionTopic := fmt.Sprintf("_topic_pattern_%s_%s", req.TopicPattern, uuid.New().String())

	// Auto-create and approve subscriber member
	newMember := s.memberManager.RequestJoin(subscriptionTopic, displayName, "", 24*time.Hour)
	if err := s.memberManager.Approve(newMember.ID, "topic-auto-approve"); err != nil {
		logger.FromContext(ctx).Error().
			Err(err).
			Str("topic_pattern", req.TopicPattern).
			Msg("failed_to_approve_topic_subscriber")
		return status.Error(codes.Internal, "failed to create subscriber")
	}

	memberID := newMember.ID

	logger.FromContext(ctx).Info().
		Str("member_id", memberID.String()).
		Str("topic_pattern", req.TopicPattern).
		Str("display_name", displayName).
		Msg("topic_stream_started")

	s.activeStreams.Add(1)
	defer s.activeStreams.Add(-1)

	// Get initial list of topics matching the pattern
	allRooms := s.roomManager.List()
	var matchingTopics []string
	for _, rm := range allRooms {
		if MatchTopic(req.TopicPattern, rm.ID) {
			matchingTopics = append(matchingTopics, rm.ID)
		}
	}

	// Track active subscriptions per topic
	subscriptions := make(map[string]*busboy.Subscription)
	defer func() {
		// Cleanup all subscriptions
		for _, sub := range subscriptions {
			s.busboy.Unsubscribe(sub)
		}
	}()

	// Subscribe to initial matching topics
	for _, topicName := range matchingTopics {
		sub := s.busboy.Subscribe(topicName, memberID, 100)
		subscriptions[topicName] = sub
		logger.FromContext(ctx).Debug().
			Str("topic", topicName).
			Msg("subscribed_to_topic")
	}

	// Create a shared channel for fanning in events from all subscriptions
	eventChan := make(chan *topicEvent, 100)
	defer close(eventChan)

	// Start a goroutine for each subscription to forward events to the shared channel
	for topicName, sub := range subscriptions {
		go s.forwardTopicEvents(ctx, topicName, sub, eventChan)
	}

	// Start dynamic watcher goroutine to discover new topics matching the pattern
	watcherCtx, watcherCancel := context.WithCancel(ctx)
	defer watcherCancel()

	go s.watchTopicsForPattern(watcherCtx, memberID, req.TopicPattern, subscriptions, eventChan)

	// Replay historical messages if requested
	if req.SinceSeq > 0 {
		for _, topicName := range matchingTopics {
			rm, err := s.roomManager.Get(topicName)
			if err != nil {
				continue
			}

			// Get all messages from the room
			msgs := rm.GetMessages()

			for i, msg := range msgs {
				// Only send messages after the requested seq
				if int64(i+1) > req.SinceSeq {
					event := &chatpb.TopicEvent{
						Topic:     topicName,
						MessageId: msg.ID.String(),
						SenderId:  msg.CreatorID.String(),
						Payload:   []byte(msg.Content),
						CreatedAt: timestamppb.New(msg.Timestamp),
						Seq:       int64(i + 1),
						Type:      chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED,
					}

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
	sub *busboy.Subscription,
	eventChan chan<- *topicEvent,
) {
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-sub.Channel:
			if !ok {
				return
			}

			// Get sequence number for this message
			seq := s.seqCounter.Next(topic)

			pbEvent := &chatpb.TopicEvent{
				Topic:     topic,
				MessageId: event.Message.ID.String(),
				SenderId:  event.Message.CreatorID.String(),
				Payload:   []byte(event.Message.Content),
				CreatedAt: timestamppb.New(event.Timestamp),
				Seq:       seq,
				Type:      chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED,
			}

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
				// Event channel buffer full, drop event
				// This is acceptable for streaming pub/sub
			}
		}
	}
}

// watchTopicsForPattern periodically checks for new topics matching the pattern
// and subscribes to them, sending TOPIC_SUBSCRIPTION_ADDED events.
func (s *TopicService) watchTopicsForPattern(
	ctx context.Context,
	memberID uuid.UUID,
	pattern string,
	subscriptions map[string]*busboy.Subscription,
	eventChan chan<- *topicEvent,
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
						sub := s.busboy.Subscribe(rm.ID, memberID, 100)
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

						// Start forwarding events from this subscription
						go s.forwardTopicEvents(ctx, rm.ID, sub, eventChan)
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

	// Get or create the room for this topic
	rm := s.roomManager.GetOrCreate(req.Topic, req.Topic)

	// Get or create the sender member
	var senderID uuid.UUID
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
	} else {
		// Create a default publisher member
		defaultMember := s.memberManager.RequestJoin(req.Topic, "grpc-publisher", "", 24*time.Hour)
		if err := s.memberManager.Approve(defaultMember.ID, "topic-auto-approve"); err != nil {
			logger.FromContext(ctx).Error().
				Err(err).
				Str("topic", req.Topic).
				Msg("failed_to_create_default_publisher")
			return nil, status.Error(codes.Internal, "failed to create publisher")
		}
		senderID = defaultMember.ID
	}

	// Send message to room's ring buffer
	msg, _ := rm.SendMessage(senderID, string(req.Payload))

	// Publish event through busboy pub/sub
	s.busboy.PublishMessageCreated(msg)

	// Assign sequence number
	seq := s.seqCounter.Next(req.Topic)

	logger.FromContext(ctx).Debug().
		Str("topic", req.Topic).
		Str("message_id", msg.ID.String()).
		Str("sender_id", senderID.String()).
		Int64("seq", seq).
		Msg("topic_message_published")

	metrics.MessagesCreated.WithLabelValues(req.Topic).Inc()

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
		Status:       "ok",
		Timestamp:    timestamppb.Now(),
		ActiveStreams: activeStreams,
		TotalTopics:  int64(topicCount),
	}, nil
}

// String returns a string representation of the service
func (s *TopicService) String() string {
	return fmt.Sprintf("TopicService{topics=%d, active_streams=%d, subscriptions=%d}",
		s.roomManager.Count(),
		s.activeStreams.Load(),
		s.busboy.GetTotalSubscriptions(),
	)
}
