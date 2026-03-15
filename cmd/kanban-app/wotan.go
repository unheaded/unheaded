// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package main - Wotan integration layer
// Handles all Wotan client operations with defensive error handling
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	// Using package-level log from main.go (Kingdom's native logger)
)

// Wotan topic constants
const (
	TopicTasksCreated = "tasks.created"
	TopicTasksUpdated = "tasks.updated"
	TopicTasksDeleted = "tasks.deleted"
	TopicTasks        = "tasks.*" // Wildcard subscription

	// Timeline topics - THE META MOMENT
	TopicTimelineUpdates = "timeline.updates"
)

// Common errors
var (
	ErrNilClient          = errors.New("wotan client cannot be nil")
	ErrNilContext         = errors.New("context cannot be nil")
	ErrEmptyTopic         = errors.New("topic cannot be empty")
	ErrInvalidTask        = errors.New("task validation failed")
	ErrMarshalFailed      = errors.New("failed to marshal task")
	ErrPublishFailed      = errors.New("failed to publish to wotan")
	ErrSubscribeFailed    = errors.New("failed to subscribe to wotan")
	ErrStreamFailed       = errors.New("failed to stream from wotan")
	ErrTaskNotFound       = errors.New("task not found")
	ErrEmptyTaskID        = errors.New("task ID cannot be empty")
	ErrTaskAlreadyExists  = errors.New("task with ID already exists")
)

// WotanClient interface for testing
type WotanClient interface {
	Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error)
	Publish(ctx context.Context, topic string, payload []byte) error
	GetMessages(ctx context.Context, topic string, afterSeq int64, limit int) ([]*wotanClient.Message, error)
	StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error)
	Close() error
}

// TaskManager handles task operations with Wotan integration
type TaskManager struct {
	client WotanClient
	store  TaskStore // SQLite or Postgres persistence — the memory
	tasks  map[string]*Task
	mu     sync.RWMutex

	// Subscription state
	subscribed         bool
	timelineSubscribed bool
	subMu              sync.RWMutex

	// Wait/Drain — track inflight async publishes so shutdown doesn't lose data
	inflight sync.WaitGroup

	// Timeline manager for THE META MOMENT
	timelineManager *TimelineManager

	// SSE broadcast function
	broadcast func(eventType string, data interface{})
}

// NewTaskManager creates a task manager with Wotan client and optional task store.
// If store is nil, TaskManager works in-memory only (no persistence across restarts).
func NewTaskManager(client WotanClient, broadcast func(string, interface{}), store TaskStore) (*TaskManager, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	if broadcast == nil {
		return nil, errors.New("broadcast function cannot be nil")
	}

	tm := &TaskManager{
		client:          client,
		store:           store,
		tasks:           make(map[string]*Task),
		broadcast:       broadcast,
		timelineManager: NewTimelineManager(broadcast),
	}

	return tm, nil
}

// Initialize loads initial tasks and subscribes to Wotan
func (tm *TaskManager) Initialize(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Load tasks: prefer Store (SQLite L1) → fall back to hardcoded seed
	if tm.store != nil {
		// Seed Store on first run (Genesis Event), no-op if already populated
		if _, err := tm.store.SeedIfEmpty(); err != nil {
			log.Error().Err(err).Msg("store seed failed, falling back to hardcoded tasks")
			tm.store = nil // disable broken store
		}
	}

	if tm.store != nil {
		// Load from SQLite — persisted state survives restarts
		storeTasks, err := tm.store.GetAllTasks()
		if err != nil {
			log.Error().Err(err).Msg("store load failed, falling back to hardcoded tasks")
		} else {
			for i := range storeTasks {
				task := &storeTasks[i]
				tm.tasks[task.ID] = task
			}
			log.Info().
				Int("count", len(tm.tasks)).
				Msg("loaded tasks from SQLite L1 store")
		}
	}

	// Fallback: hardcoded initial tasks (no persistence)
	if len(tm.tasks) == 0 {
		initialTasks := getInitialTasks()
		for i := range initialTasks {
			task := &initialTasks[i]
			if err := tm.validateTask(task); err != nil {
				log.Warn().
					Err(err).
					Str("task_id", task.ID).
					Msg("skipping invalid initial task")
				continue
			}
			tm.tasks[task.ID] = task
		}
		log.Info().
			Int("count", len(tm.tasks)).
			Msg("loaded initial tasks (in-memory fallback)")
	}

	// Subscribe to task updates
	if err := tm.subscribeToTasks(ctx); err != nil {
		log.Error().Err(err).Msg("failed to subscribe to wotan tasks")
		return fmt.Errorf("subscribe tasks: %w", err)
	}

	// Subscribe to timeline updates - THE META MOMENT
	if err := tm.subscribeToTimeline(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to subscribe to timeline updates (non-fatal)")
		// Non-fatal: Kanban can work without timeline integration
	}

	return nil
}

// subscribeToTasks subscribes to all task topics
func (tm *TaskManager) subscribeToTasks(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}

	// Subscribe to task events
	sub, err := tm.client.Subscribe(ctx, TopicTasks, "kanban-app")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSubscribeFailed, err)
	}

	log.Info().
		Str("topic", sub.Topic).
		Str("status", sub.Status).
		Str("subscriber_id", sub.SubscriberID).
		Msg("subscribed to wotan topic")

	tm.subMu.Lock()
	tm.subscribed = true
	tm.subMu.Unlock()

	// Start message stream
	go tm.streamMessages(ctx)

	return nil
}

// subscribeToTimeline subscribes to timeline updates from Timeguru
// This is THE META MOMENT - Kanban tracking its own timeline
func (tm *TaskManager) subscribeToTimeline(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}

	// Subscribe to timeline updates
	sub, err := tm.client.Subscribe(ctx, TopicTimelineUpdates, "kanban-app-timeline")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSubscribeFailed, err)
	}

	log.Info().
		Str("topic", sub.Topic).
		Str("status", sub.Status).
		Str("subscriber_id", sub.SubscriberID).
		Msg("subscribed to timeline updates - THE META MOMENT BEGINS")

	tm.subMu.Lock()
	tm.timelineSubscribed = true
	tm.subMu.Unlock()

	// Start timeline message stream
	go tm.streamTimelineMessages(ctx)

	return nil
}

// streamTimelineMessages listens for timeline updates from Wotan
func (tm *TaskManager) streamTimelineMessages(ctx context.Context) {
	msgCh, err := tm.client.StreamMessages(ctx, TopicTimelineUpdates)
	if err != nil {
		log.Error().
			Err(err).
			Msg("failed to start timeline message stream")
		return
	}

	log.Info().
		Str("topic", TopicTimelineUpdates).
		Msg("started timeline message stream - THE META MOMENT IS LIVE")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("timeline message stream stopped (context cancelled)")
			return

		case msg, ok := <-msgCh:
			if !ok {
				log.Warn().Msg("timeline message channel closed")
				return
			}

			// Process timeline update
			if err := tm.handleTimelineMessage(msg); err != nil {
				log.Error().
					Err(err).
					Str("message_id", msg.MessageID).
					Msg("failed to handle timeline message")
			}
		}
	}
}

// handleTimelineMessage processes a timeline update from Wotan
func (tm *TaskManager) handleTimelineMessage(msg *wotanClient.Message) error {
	if msg == nil {
		return errors.New("message cannot be nil")
	}

	log.Info().
		Str("message_id", msg.MessageID).
		Int64("seq", msg.Seq).
		Msg("received timeline update from Timeguru")

	// Delegate to TimelineManager
	if tm.timelineManager != nil {
		return tm.timelineManager.HandleTimelineUpdate([]byte(msg.Payload))
	}

	return nil
}

// GetTimelineTasks returns tasks derived from the timeline
func (tm *TaskManager) GetTimelineTasks() []*Task {
	if tm.timelineManager == nil {
		return nil
	}
	return tm.timelineManager.GetTimelineTasks()
}

// GetTimeline returns the current timeline
func (tm *TaskManager) GetTimeline() *Timeline {
	if tm.timelineManager == nil {
		return nil
	}
	return tm.timelineManager.GetTimeline()
}

// IsTimelineSubscribed returns timeline subscription status
func (tm *TaskManager) IsTimelineSubscribed() bool {
	tm.subMu.RLock()
	defer tm.subMu.RUnlock()
	return tm.timelineSubscribed
}

// streamMessages listens for Wotan messages and updates tasks
func (tm *TaskManager) streamMessages(ctx context.Context) {
	msgCh, err := tm.client.StreamMessages(ctx, TopicTasks)
	if err != nil {
		log.Error().
			Err(err).
			Msg("failed to start message stream")
		return
	}

	log.Info().
		Str("topic", TopicTasks).
		Msg("started wotan message stream")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("message stream stopped (context cancelled)")
			return

		case msg, ok := <-msgCh:
			if !ok {
				log.Warn().Msg("message channel closed")
				return
			}

			if err := tm.handleMessage(msg); err != nil {
				log.Error().
					Err(err).
					Str("message_id", msg.MessageID).
					Str("topic", msg.Topic).
					Msg("failed to handle message")
			}
		}
	}
}

// handleMessage processes a Wotan message
func (tm *TaskManager) handleMessage(msg *wotanClient.Message) error {
	if msg == nil {
		return errors.New("message cannot be nil")
	}

	var task Task
	if err := json.Unmarshal([]byte(msg.Payload), &task); err != nil {
		return fmt.Errorf("unmarshal task: %w", err)
	}

	if err := tm.validateTask(&task); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTask, err)
	}

	// Determine event type from topic
	var eventType string
	switch msg.Topic {
	case TopicTasksCreated:
		eventType = "task.created"
		tm.addTask(&task)
		if tm.store != nil {
			if err := tm.store.SaveTask(&task); err != nil {
				log.Warn().Err(err).Str("task_id", task.ID).Msg("failed to persist inbound create to SQLite L1")
			}
		}
	case TopicTasksUpdated:
		eventType = "task.updated"
		tm.updateTask(&task)
		if tm.store != nil {
			if err := tm.store.SaveTask(&task); err != nil {
				log.Warn().Err(err).Str("task_id", task.ID).Msg("failed to persist inbound update to SQLite L1")
			}
		}
	case TopicTasksDeleted:
		eventType = "task.deleted"
		tm.deleteTask(task.ID)
		if tm.store != nil {
			if err := tm.store.DeleteTask(task.ID); err != nil {
				log.Warn().Err(err).Str("task_id", task.ID).Msg("failed to persist inbound delete to SQLite L1")
			}
		}
	default:
		// Infer from task existence
		if tm.taskExists(task.ID) {
			eventType = "task.updated"
			tm.updateTask(&task)
		} else {
			eventType = "task.created"
			tm.addTask(&task)
		}
		if tm.store != nil {
			if err := tm.store.SaveTask(&task); err != nil {
				log.Warn().Err(err).Str("task_id", task.ID).Msg("failed to persist inbound task to SQLite L1")
			}
		}
	}

	// Broadcast to SSE clients
	tm.broadcast(eventType, &task)

	log.Info().
		Str("event", eventType).
		Str("task_id", task.ID).
		Str("message_id", msg.MessageID).
		Msg("processed wotan message")

	return nil
}

// CreateTask creates a new task and publishes to Wotan
func (tm *TaskManager) CreateTask(ctx context.Context, task *Task) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if task == nil {
		return ErrInvalidTask
	}
	if task.ID == "" {
		return ErrEmptyTaskID
	}

	if err := tm.validateTask(task); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTask, err)
	}

	// Check if task already exists
	if tm.taskExists(task.ID) {
		return ErrTaskAlreadyExists
	}

	// Set timestamps
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now

	// Add to local map
	tm.addTask(task)

	// Persist to SQLite L1 store (if available)
	if tm.store != nil {
		if err := tm.store.SaveTask(task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to persist task to SQLite L1")
		}
	}

	// Publish to Wotan L2 (best-effort async — don't fail the create if bus is down)
	// Snapshot task to avoid data race with caller mutating the pointer after return
	taskSnap := *task
	tm.inflight.Add(1)
	go func() {
		defer tm.inflight.Done()
		if err := tm.publishTask(ctx, TopicTasksCreated, &taskSnap); err != nil {
			log.Warn().
				Err(err).
				Str("task_id", taskSnap.ID).
				Str("topic", TopicTasksCreated).
				Msg("failed to publish task creation to wotan (local create persisted)")
		}
	}()

	return nil
}

// UpdateTask updates an existing task and publishes to Wotan
func (tm *TaskManager) UpdateTask(ctx context.Context, task *Task) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if task == nil {
		return ErrInvalidTask
	}
	if task.ID == "" {
		return ErrEmptyTaskID
	}

	if err := tm.validateTask(task); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTask, err)
	}

	// Check if task exists
	if !tm.taskExists(task.ID) {
		return ErrTaskNotFound
	}

	// Update timestamp
	task.UpdatedAt = time.Now()

	// Update local map
	tm.updateTask(task)

	// Persist to SQLite L1 store (if available)
	if tm.store != nil {
		if err := tm.store.SaveTask(task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to persist task update to SQLite L1")
		}
	}

	// Publish to Wotan L2 (best-effort async — don't fail the update if bus is down)
	// Snapshot task to avoid data race with caller mutating the pointer after return
	taskSnap := *task
	tm.inflight.Add(1)
	go func() {
		defer tm.inflight.Done()
		if err := tm.publishTask(ctx, TopicTasksUpdated, &taskSnap); err != nil {
			log.Warn().
				Err(err).
				Str("task_id", taskSnap.ID).
				Str("topic", TopicTasksUpdated).
				Msg("failed to publish task update to wotan (local update persisted)")
		}
	}()

	return nil
}

// DeleteTask deletes a task and publishes to Wotan
func (tm *TaskManager) DeleteTask(ctx context.Context, taskID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if taskID == "" {
		return ErrEmptyTaskID
	}

	// Check if task exists
	task := tm.getTask(taskID)
	if task == nil {
		return ErrTaskNotFound
	}

	// Delete from local map
	tm.deleteTask(taskID)

	// Delete from SQLite L1 store (if available)
	if tm.store != nil {
		if err := tm.store.DeleteTask(taskID); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("failed to delete task from SQLite L1")
		}
	}

	// Publish to Wotan L2 (best-effort async — don't fail the delete if bus is down)
	// Snapshot task to avoid data race with caller mutating the pointer after return
	taskSnap := *task
	tm.inflight.Add(1)
	go func() {
		defer tm.inflight.Done()
		if err := tm.publishTask(ctx, TopicTasksDeleted, &taskSnap); err != nil {
			log.Warn().
				Err(err).
				Str("task_id", taskSnap.ID).
				Str("topic", TopicTasksDeleted).
				Msg("failed to publish task deletion to wotan (local delete persisted)")
		}
	}()

	return nil
}

// GetTask retrieves a task by ID
func (tm *TaskManager) GetTask(taskID string) (*Task, error) {
	if taskID == "" {
		return nil, ErrEmptyTaskID
	}

	task := tm.getTask(taskID)
	if task == nil {
		return nil, ErrTaskNotFound
	}

	return task, nil
}

// GetAllTasks retrieves all tasks
func (tm *TaskManager) GetAllTasks() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]*Task, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		tasks = append(tasks, task)
	}

	return tasks
}

// IsSubscribed returns subscription status
func (tm *TaskManager) IsSubscribed() bool {
	tm.subMu.RLock()
	defer tm.subMu.RUnlock()
	return tm.subscribed
}

// Close gracefully shuts down TaskManager.
// Drains inflight Wotan publishes (5s timeout), then closes the client.
func (tm *TaskManager) Close() error {
	if tm.client == nil {
		return ErrNilClient
	}

	// Wait for inflight publishes to complete (or timeout)
	done := make(chan struct{})
	go func() {
		tm.inflight.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("all inflight Wotan publishes drained")
	case <-time.After(5 * time.Second):
		log.Warn().Msg("timed out waiting for inflight Wotan publishes (5s) — closing anyway")
	}

	return tm.client.Close()
}

// ============================================================================
// INTERNAL HELPERS
// ============================================================================

func (tm *TaskManager) publishTask(ctx context.Context, topic string, task *Task) error {
	if topic == "" {
		return ErrEmptyTopic
	}
	if task == nil {
		return ErrInvalidTask
	}

	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMarshalFailed, err)
	}

	if err := tm.client.Publish(ctx, topic, payload); err != nil {
		return fmt.Errorf("%w: %v", ErrPublishFailed, err)
	}

	log.Info().
		Str("topic", topic).
		Str("task_id", task.ID).
		Int("payload_size", len(payload)).
		Msg("published task to wotan")

	return nil
}

func (tm *TaskManager) addTask(task *Task) {
	if task == nil || task.ID == "" {
		return
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	// Store a copy to prevent external modifications affecting stored data
	taskCopy := *task
	tm.tasks[task.ID] = &taskCopy
}

func (tm *TaskManager) updateTask(task *Task) {
	if task == nil || task.ID == "" {
		return
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	// Store a copy to prevent external modifications affecting stored data
	taskCopy := *task
	tm.tasks[task.ID] = &taskCopy
}

func (tm *TaskManager) deleteTask(taskID string) {
	if taskID == "" {
		return
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.tasks, taskID)
}

func (tm *TaskManager) getTask(taskID string) *Task {
	if taskID == "" {
		return nil
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tasks[taskID]
}

func (tm *TaskManager) taskExists(taskID string) bool {
	if taskID == "" {
		return false
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	_, exists := tm.tasks[taskID]
	return exists
}

// validateTask validates task fields
func (tm *TaskManager) validateTask(task *Task) error {
	if task == nil {
		return errors.New("task cannot be nil")
	}
	if task.ID == "" {
		return errors.New("task.ID cannot be empty")
	}
	if task.Title == "" {
		return errors.New("task.Title cannot be empty")
	}
	if task.Status == "" {
		return errors.New("task.Status cannot be empty")
	}

	// Validate status enum
	validStatuses := map[string]bool{
		"todo":        true,
		"in-progress": true,
		"review":      true,
		"done":        true,
	}
	if !validStatuses[task.Status] {
		return fmt.Errorf("invalid status: %s (must be todo, in-progress, or done)", task.Status)
	}

	// Validate progress range
	if task.Progress < 0 || task.Progress > 100 {
		return fmt.Errorf("invalid progress: %d (must be 0-100)", task.Progress)
	}

	return nil
}
