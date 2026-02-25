// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	chatpb "unheaded/services/wotan/proto"
	"unheaded/services/wotan/internal/wotan"
	"unheaded/services/wotan/internal/logger"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/room"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ChatService implements the gRPC ChatStream service
type ChatService struct {
	chatpb.UnimplementedChatStreamServer
	roomManager   *room.Manager
	memberManager *member.Manager
	wotan        *wotan.Wotan
}

// NewChatService creates a new ChatService
func NewChatService(
	roomManager *room.Manager,
	memberManager *member.Manager,
	msgWotan *wotan.Wotan,
) *ChatService {
	return &ChatService{
		roomManager:   roomManager,
		memberManager: memberManager,
		wotan:        msgWotan,
	}
}

// getCreatorName looks up the member name for a given creator ID
func (s *ChatService) getCreatorName(creatorID uuid.UUID) string {
	member, err := s.memberManager.Get(creatorID)
	if err != nil {
		return "Unknown"
	}
	return member.Name
}

// StreamMessages implements the gRPC StreamMessages method
func (s *ChatService) StreamMessages(
	req *chatpb.StreamRequest,
	stream chatpb.ChatStream_StreamMessagesServer,
) error {
	ctx := stream.Context()

	// Parse member ID
	memberID, err := uuid.Parse(req.MemberId)
	if err != nil {
		logger.FromContext(ctx).Error().
			Err(err).
			Str("member_id", req.MemberId).
			Str("room_id", req.RoomId).
			Msg("invalid_member_id")
		return status.Error(codes.InvalidArgument, "invalid member ID")
	}

	// Verify member exists and is approved
	mbr, err := s.memberManager.Get(memberID)
	if err != nil {
		logger.FromContext(ctx).Error().
			Err(err).
			Str("member_id", memberID.String()).
			Str("room_id", req.RoomId).
			Msg("member_not_found")
		return status.Error(codes.NotFound, "member not found")
	}

	if mbr.Status != member.StatusApproved {
		logger.FromContext(ctx).Warn().
			Str("member_id", memberID.String()).
			Str("room_id", req.RoomId).
			Str("status", string(mbr.Status)).
			Msg("member_not_approved")
		return status.Error(codes.PermissionDenied, "member not approved")
	}

	// Verify room exists
	rm, err := s.roomManager.Get(req.RoomId)
	if err != nil {
		logger.FromContext(ctx).Error().
			Err(err).
			Str("member_id", memberID.String()).
			Str("room_id", req.RoomId).
			Msg("room_not_found")
		return status.Error(codes.NotFound, "room not found")
	}

	logger.FromContext(ctx).Info().
		Str("member_id", memberID.String()).
		Str("room_id", req.RoomId).
		Msg("stream_started")

	// Subscribe to wotan for live messages
	subscription := s.wotan.Subscribe(req.RoomId, memberID, 100)
	defer s.wotan.Unsubscribe(subscription)

	// Update metrics
	metrics.StreamsActive.WithLabelValues(req.RoomId).Inc()
	defer metrics.StreamsActive.WithLabelValues(req.RoomId).Dec()

	// Send historical messages if requested
	if req.Since != nil && req.Since.AsTime().Before(time.Now()) {
		historicalMessages := rm.GetMessagesSince(req.Since.AsTime())
		for _, msg := range historicalMessages {
			event := &chatpb.MessageEvent{
				Id:          msg.ID.String(),
				CreatorId:   msg.CreatorID.String(),
				RoomId:      msg.RoomID,
				Content:     msg.Content,
				Timestamp:   timestamppb.New(msg.Timestamp),
				Type:        chatpb.EventType_MESSAGE_CREATED,
				CreatorName: s.getCreatorName(msg.CreatorID),
			}

			if err := stream.Send(event); err != nil {
				logger.FromContext(ctx).Error().
					Err(err).
					Str("member_id", memberID.String()).
					Str("room_id", req.RoomId).
					Str("message_id", msg.ID.String()).
					Msg("stream_send_error_historical")
				metrics.StreamErrors.WithLabelValues(req.RoomId, "send_error").Inc()
				return status.Error(codes.Aborted, "failed to send message")
			}

			metrics.StreamMessagesSent.WithLabelValues(req.RoomId, "MESSAGE_CREATED").Inc()
		}
	}

	// Stream live messages
	for {
		select {
		case <-ctx.Done():
			logger.FromContext(ctx).Info().
				Str("member_id", memberID.String()).
				Str("room_id", req.RoomId).
				Msg("stream_closed_by_client")
			return status.Error(codes.Canceled, "client disconnected")

		case event, ok := <-subscription.Channel:
			if !ok {
				logger.FromContext(ctx).Info().
					Str("member_id", memberID.String()).
					Str("room_id", req.RoomId).
					Msg("stream_closed_by_server")
				return status.Error(codes.Unavailable, "stream closed by server")
			}

			// Convert wotan event to protobuf event
			eventType := chatpb.EventType_MESSAGE_CREATED
			if event.Type == wotan.EventMessageDeleted {
				eventType = chatpb.EventType_MESSAGE_DELETED
			}

			pbEvent := &chatpb.MessageEvent{
				Id:          event.Message.ID.String(),
				CreatorId:   event.Message.CreatorID.String(),
				RoomId:      event.Message.RoomID,
				Content:     event.Message.Content,
				Timestamp:   timestamppb.New(event.Timestamp),
				Type:        eventType,
				CreatorName: s.getCreatorName(event.Message.CreatorID),
			}

			// Send to client
			if err := stream.Send(pbEvent); err != nil {
				logger.FromContext(ctx).Error().
					Err(err).
					Str("member_id", memberID.String()).
					Str("room_id", req.RoomId).
					Str("event_type", string(event.Type)).
					Msg("stream_send_error")
				metrics.StreamErrors.WithLabelValues(req.RoomId, "send_error").Inc()
				return status.Error(codes.Aborted, "failed to send message")
			}

			logger.FromContext(ctx).Debug().
				Str("member_id", memberID.String()).
				Str("room_id", req.RoomId).
				Str("message_id", event.Message.ID.String()).
				Str("event_type", string(event.Type)).
				Msg("stream_message_sent")

			metrics.StreamMessagesSent.WithLabelValues(req.RoomId, string(event.Type)).Inc()
		}
	}
}

// Ping implements the gRPC Ping method for keep-alive
func (s *ChatService) Ping(ctx context.Context, req *chatpb.PingRequest) (*chatpb.PongResponse, error) {
	logger.FromContext(ctx).Debug().Msg("ping_received")

	return &chatpb.PongResponse{
		Timestamp: timestamppb.Now(),
		Status:    "ok",
	}, nil
}

// String returns a string representation of the service
func (s *ChatService) String() string {
	return fmt.Sprintf("ChatService{rooms=%d, members=%d, subscriptions=%d}",
		s.roomManager.Count(),
		s.memberManager.Count(),
		s.wotan.GetTotalSubscriptions(),
	)
}
