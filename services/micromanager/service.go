package micromanager

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	"github.com/rs/zerolog/log"
)

// Service manages micromanager business logic and Busboy integration
type Service struct {
	store          *Store
	busboy         *busboyClient.Client
	taskIDCounter  int64
	subscriptions  map[string]bool
	alertListener  chan *busboyClient.Message
}

// NewService creates a new micromanager service
func NewService(store *Store, busboy *busboyClient.Client) *Service {
	return &Service{
		store:         store,
		busboy:        busboy,
		taskIDCounter: 0,
		subscriptions: make(map[string]bool),
		alertListener: make(chan *busboyClient.Message, 100),
	}
}

// Start initializes the service (connects to Busboy, subscribes to topics)
func (s *Service) Start(ctx context.Context) error {
	if s.busboy == nil {
		log.Info().Msg("busboy client not configured, skipping integration")
		return nil
	}

	// Subscribe to alerts
	sub, err := s.busboy.Subscribe(ctx, "alerts.critical", "micromanager-service")
	if err != nil {
		log.Warn().Err(err).Msg("failed to subscribe to alerts.critical, continuing without subscription")
		return nil
	}

	log.Info().
		Str("subscriber_id", sub.SubscriberID).
		Str("status", sub.Status).
		Msg("subscribed to alerts.critical")

	s.subscriptions["alerts.critical"] = true

	// Start alert listener goroutine
	go s.listenForAlerts(ctx)

	return nil
}

// Stop gracefully shuts down the service
func (s *Service) Stop(ctx context.Context) error {
	if s.busboy != nil {
		if err := s.busboy.Close(); err != nil {
			log.Error().Err(err).Msg("failed to close busboy client")
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

// PublishTaskCreated publishes a task.created event to Busboy
func (s *Service) PublishTaskCreated(taskID string, task *Task) error {
	if s.busboy == nil {
		log.Debug().Str("task_id", taskID).Msg("busboy client not configured, skipping publish")
		return nil
	}

	if !s.busboy.IsApproved("tasks.created") {
		log.Warn().Msg("not approved to publish to tasks.created, skipping event")
		return nil
	}

	event := map[string]interface{}{
		"task_id":     taskID,
		"event_type":  "task.created",
		"timestamp":   time.Now().Unix(),
		"title":       task.Title,
		"owner":       task.Owner,
		"status":      task.Status,
		"priority":    task.Priority,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.busboy.Publish(ctx, "tasks.created", payload); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.created")
		return err
	}

	log.Info().Str("task_id", taskID).Msg("published task.created event")
	return nil
}

// PublishTaskUpdated publishes a task.updated event to Busboy
func (s *Service) PublishTaskUpdated(taskID string, task *Task) error {
	if s.busboy == nil {
		log.Debug().Str("task_id", taskID).Msg("busboy client not configured, skipping publish")
		return nil
	}

	if !s.busboy.IsApproved("tasks.updated") {
		log.Warn().Msg("not approved to publish to tasks.updated, skipping event")
		return nil
	}

	event := map[string]interface{}{
		"task_id":     taskID,
		"event_type":  "task.updated",
		"timestamp":   time.Now().Unix(),
		"title":       task.Title,
		"status":      task.Status,
		"priority":    task.Priority,
		"assignee":    task.Assignee,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.busboy.Publish(ctx, "tasks.updated", payload); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.updated")
		return err
	}

	log.Info().Str("task_id", taskID).Msg("published task.updated event")
	return nil
}

// PublishTaskCompleted publishes a task.completed event
func (s *Service) PublishTaskCompleted(taskID string, task *Task) error {
	if s.busboy == nil {
		log.Debug().Str("task_id", taskID).Msg("busboy client not configured, skipping publish")
		return nil
	}

	if !s.busboy.IsApproved("tasks.completed") {
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

	if err := s.busboy.Publish(ctx, "tasks.completed", payload); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.completed")
		return err
	}

	log.Info().Str("task_id", taskID).Msg("published task.completed event")
	return nil
}

// listenForAlerts subscribes to critical alerts
func (s *Service) listenForAlerts(ctx context.Context) {
	if s.busboy == nil {
		return
	}

	msgCh, err := s.busboy.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		log.Error().Err(err).Msg("failed to stream alerts")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-msgCh:
			if msg == nil {
				return
			}

			log.Warn().
				Str("message_id", msg.MessageID).
				Str("payload", msg.Payload).
				Msg("received critical alert")

			// Parse alert
			var alert map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &alert); err != nil {
				log.Error().Err(err).Msg("failed to parse alert payload")
				continue
			}

			// Handle alert (can be extended with routing logic)
			s.handleAlert(alert)
		}
	}
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
		"service":      "micromanager",
		"status":       "healthy",
		"tasks_count":  s.store.Count(),
		"timestamp":    time.Now().Unix(),
	}

	if s.busboy != nil {
		status["busboy_connected"] = true
		status["subscriptions"] = len(s.subscriptions)
	} else {
		status["busboy_connected"] = false
	}

	return status
}
