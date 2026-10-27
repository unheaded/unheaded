// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package micromanager

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	wotanClient "unheaded/pkg/wotan-client"
)

// Service manages micromanager business logic and Wotan integration
type Service struct {
	store         *Store
	wotan         *wotanClient.Client
	taskIDCounter int64
	subscriptions map[string]bool
	subMu         sync.RWMutex
	alertListener chan *wotanClient.Message

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// NewService creates a new micromanager service
func NewService(store *Store, wotan *wotanClient.Client) *Service {
	return &Service{
		store:         store,
		wotan:         wotan,
		taskIDCounter: 0,
		subscriptions: make(map[string]bool),
		alertListener: make(chan *wotanClient.Message, 100),
	}
}

// wotanName is the name micromanager joins Wotan topics as (on Wotan's
// topics.auto_approve list).
const wotanName = "micromanager-service"

// publishTopics are the topics PublishTask* publish to. The Wotan client
// refuses to publish to a topic it has not joined, and nothing joined these,
// so every task event failed "not subscribed to topic tasks.created".
var publishTopics = []string{"tasks.created", "tasks.updated", "tasks.completed"}

// Start joins micromanager's Wotan topics and starts the alert listener in
// the background, retrying until ctx ends (pkg/wotan-client
// SubscribeWithRetry), so a Wotan that comes up after micromanager is still
// joined. It does not block on Wotan. Before, it subscribed once and a
// refused join stayed refused for the life of the process.
func (s *Service) Start(ctx context.Context) error {
	if s.wotan == nil {
		log.Warn().Msg("no wotan client configured: running without Wotan")
		return nil
	}
	for _, topic := range publishTopics {
		go s.join(ctx, topic)
	}
	go s.listenForAlerts(ctx)
	return nil
}

// join joins topic with retry and records it once Wotan approves.
func (s *Service) join(ctx context.Context, topic string) {
	if err := s.wotan.SubscribeWithRetry(ctx, topic, wotanName, nil); err != nil {
		return // ctx ended
	}
	s.subMu.Lock()
	s.subscriptions[topic] = true
	s.subMu.Unlock()
	log.Info().Str("topic", topic).Msg("joined wotan topic")
}

// Stop gracefully shuts down the service
func (s *Service) Stop(ctx context.Context) error {
	if s.wotan != nil {
		if err := s.wotan.Close(); err != nil {
			log.Error().Err(err).Msg("failed to close wotan client")
			return err
		}
	}
	close(s.alertListener)
	return nil
}

// GenerateTaskID creates a unique task ID
func (s *Service) GenerateTaskID() string {
	counter := atomic.AddInt64(&s.taskIDCounter, 1)
	return fmt.Sprintf("task-%d-%d", time.Now().Unix(), counter)
}

// PublishTaskCreated publishes a task.created event to Wotan
func (s *Service) PublishTaskCreated(taskID string, task *Task) error {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		log.Warn().Str("task_id", taskID).Msg("wotan unavailable — task.created event dropped (degraded mode)")
		return nil
	}

	if !s.wotan.IsApproved("tasks.created") {
		log.Warn().Msg("not approved to publish to tasks.created, skipping event")
		return nil
	}

	event := map[string]interface{}{
		"task_id":    taskID,
		"event_type": "task.created",
		"timestamp":  time.Now().Unix(),
		"title":      task.Title,
		"owner":      task.Owner,
		"status":     task.Status,
		"priority":   task.Priority,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(ctx, "tasks.created", payload); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.created")
		return err
	}

	log.Info().Str("task_id", taskID).Msg("published task.created event")
	return nil
}

// PublishTaskUpdated publishes a task.updated event to Wotan
func (s *Service) PublishTaskUpdated(taskID string, task *Task) error {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		log.Warn().Str("task_id", taskID).Msg("wotan unavailable — task.updated event dropped (degraded mode)")
		return nil
	}

	if !s.wotan.IsApproved("tasks.updated") {
		log.Warn().Msg("not approved to publish to tasks.updated, skipping event")
		return nil
	}

	event := map[string]interface{}{
		"task_id":    taskID,
		"event_type": "task.updated",
		"timestamp":  time.Now().Unix(),
		"title":      task.Title,
		"status":     task.Status,
		"priority":   task.Priority,
		"assignee":   task.Assignee,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(ctx, "tasks.updated", payload); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.updated")
		return err
	}

	log.Info().Str("task_id", taskID).Msg("published task.updated event")
	return nil
}

// PublishTaskCompleted publishes a task.completed event
func (s *Service) PublishTaskCompleted(taskID string, task *Task) error {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		log.Warn().Str("task_id", taskID).Msg("wotan unavailable — task.completed event dropped (degraded mode)")
		return nil
	}

	if !s.wotan.IsApproved("tasks.completed") {
		log.Warn().Msg("not approved to publish to tasks.completed, skipping event")
		return nil
	}

	event := map[string]interface{}{
		"task_id":      taskID,
		"event_type":   "task.completed",
		"timestamp":    time.Now().Unix(),
		"title":        task.Title,
		"completed_at": task.CompletedAt,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(ctx, "tasks.completed", payload); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.completed")
		return err
	}

	log.Info().Str("task_id", taskID).Msg("published task.completed event")
	return nil
}

// listenForAlerts joins alerts.critical and streams it to onAlert until ctx
// ends, rejoining and reopening the stream whenever Wotan is away.
func (s *Service) listenForAlerts(ctx context.Context) {
	if s.wotan == nil {
		return
	}
	s.join(ctx, "alerts.critical")
	_ = s.wotan.SubscribeWithRetry(ctx, "alerts.critical", wotanName, s.onAlert)
}

// onAlert handles one alerts.critical message.
func (s *Service) onAlert(msg *wotanClient.Message) {
	log.Warn().
		Str("message_id", msg.MessageID).
		Str("payload", msg.Payload).
		Msg("received critical alert")

	var alert map[string]interface{}
	if err := json.Unmarshal([]byte(msg.Payload), &alert); err != nil {
		log.Error().Err(err).Msg("failed to parse alert payload")
		return
	}
	s.handleAlert(alert)
}

// handleAlert processes incoming alerts
func (s *Service) handleAlert(alert map[string]interface{}) {
	// This is where we would implement alert routing logic
	// For now, just log it
	log.Info().
		Interface("alert", alert).
		Msg("processing critical alert")
}

// HealthStatus returns service health information
func (s *Service) HealthStatus() map[string]interface{} {
	status := map[string]interface{}{
		"service":     "micromanager",
		"status":      "healthy",
		"tasks_count": s.store.Count(),
		"timestamp":   time.Now().Unix(),
	}

	if s.wotan != nil {
		status["wotan_connected"] = true
		s.subMu.RLock()
		status["subscriptions"] = len(s.subscriptions)
		s.subMu.RUnlock()
	} else {
		status["wotan_connected"] = false
	}

	return status
}
