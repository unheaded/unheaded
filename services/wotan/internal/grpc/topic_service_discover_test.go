// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// A message deleted from a topic (HTTP /api/v1/messages/delete works on any
// room, and topics are rooms) reached gRPC topic streams as a
// TOPIC_MESSAGE_PUBLISHED event with an empty payload and seq 0: a phantom
// new message, while TOPIC_MESSAGE_DELETED went unused.
func TestForwardTopicEvents_DeletionIsADeletedEvent(t *testing.T) {
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := svc.wotan.Subscribe("del.topic", uuid.New(), 10)
	defer svc.wotan.Unsubscribe(sub)
	out := make(chan *topicEvent, 10)
	go svc.forwardTopicEvents(ctx, "del.topic", sub, out)

	id := uuid.New()
	svc.wotan.PublishMessageDeleted("del.topic", id)
	select {
	case ev := <-out:
		if ev.pbEvent.Type != chatpb.TopicEventType_TOPIC_MESSAGE_DELETED || ev.pbEvent.MessageId != id.String() {
			t.Fatalf("got %v for message %s, want TOPIC_MESSAGE_DELETED for %s", ev.pbEvent.Type, ev.pbEvent.MessageId, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event forwarded")
	}
}

// A topic's room is created by its first publish. A pattern stream finds
// the new room on its next watcher tick (up to 1 s later) and only then
// subscribes, so whatever was published to the topic before that, its
// first message at least, never reached the stream: the room had no
// subscriber when it arrived and nothing replayed it. East's kanban-app
// streams tasks.*, and no tasks.* topic existed yet (2026-10-23): the
// first task event would have been lost to it.
//
// The discovered topic's messages since the stream started must arrive,
// once each and in order, followed by its live ones.
func TestStreamTopics_NewTopicsFirstMessagesArrive(t *testing.T) {
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan())
	pub := func(payload string) {
		t.Helper()
		if _, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{Topic: "tasks.created", Payload: []byte(payload)}); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := newMockTopicStream(ctx)
	done := make(chan error, 1)
	go func() {
		done <- svc.StreamTopics(&chatpb.TopicStreamRequest{TopicPattern: "tasks.*", SinceSeq: -1}, stream)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for svc.activeStreams.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	// The room does not exist yet: these create it and land before the
	// watcher can know about it.
	pub("first")
	pub("second")

	payloads := func() []string {
		var got []string
		for _, ev := range stream.getEvents() {
			if ev.Type == chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED {
				got = append(got, string(ev.Payload))
			}
		}
		return got
	}
	deadline = time.Now().Add(3 * time.Second)
	for len(payloads()) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	pub("third") // live, after discovery
	deadline = time.Now().Add(2 * time.Second)
	for len(payloads()) < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // a duplicate would show up by now

	if got, want := fmt.Sprint(payloads()), "[first second third]"; got != want {
		t.Fatalf("stream got %s, want %s", got, want)
	}
	cancel()
	<-done
}
