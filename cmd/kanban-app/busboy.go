// Package main - Busboy integration layer
// Handles all Busboy client operations with defensive error handling
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	// Using package-level log from main.go (Kingdom's native logger)
)

// Busboy topic constants
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
	ErrNilClient          = errors.New("busboy client cannot be nil")
	ErrNilContext         = errors.New("context cannot be nil")
	ErrEmptyTopic         = errors.New("topic cannot be empty")
	ErrInvalidTask        = errors.New("task validation failed")
	ErrMarshalFailed      = errors.New("failed to marshal task")
	ErrPublishFailed      = errors.New("failed to publish to busboy")
	ErrSubscribeFailed    = errors.New("failed to subscribe to busboy")
	ErrStreamFailed       = errors.New("failed to stream from busboy")
	ErrTaskNotFound       = errors.New("task not found")
	ErrEmptyTaskID        = errors.New("task ID cannot be empty")
	ErrTaskAlreadyExists  = errors.New("task with ID already exists")
)

// BusboyClient interface for testing
type BusboyClient interface {
	Subscribe(ctx context.Context, topic, displayName string) (*busboyClient.Subscriber, error)
	Publish(ctx context.Context, topic string, payload []byte) error
	GetMessages(ctx context.Context, topic string, afterSeq int64, limit int) ([]*busboyClient.Message, error)
	StreamMessages(ctx context.Context, topic string) (<-chan *busboyClient.Message, error)
	Close() error
}

// TaskManager handles task operations with Busboy integration
type TaskManager struct {
	client BusboyClient
	tasks  map[string]*Task
	mu     sync.RWMutex

	// Subscription state
	subscribed         bool
	timelineSubscribed bool
	subMu              sync.RWMutex

	// Timeline manager for THE META MOMENT
	timelineManager *TimelineManager

	// SSE broadcast function
	broadcast func(eventType string, data interface{})
}

// NewTaskManager creates a task manager with Busboy client
func NewTaskManager(client BusboyClient, broadcast func(string, interface{})) (*TaskManager, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	if broadcast == nil {
		return nil, errors.New("broadcast function cannot be nil")
	}

	tm := &TaskManager{
		client:          client,
		tasks:           make(map[string]*Task),
		broadcast:       broadcast,
		timelineManager: NewTimelineManager(broadcast),
	}

	return tm, nil
}

// Initialize loads initial tasks and subscribes to Busboy
func (tm *TaskManager) Initialize(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Load hardcoded initial tasks
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
		Msg("loaded initial tasks")

	// Subscribe to task updates
	if err := tm.subscribeToTasks(ctx); err != nil {
		log.Error().Err(err).Msg("failed to subscribe to busboy tasks")
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
		Msg("subscribed to busboy topic")

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

// streamTimelineMessages listens for timeline updates from Busboy
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

// handleTimelineMessage processes a timeline update from Busboy
func (tm *TaskManager) handleTimelineMessage(msg *busboyClient.Message) error {
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

// streamMessages listens for Busboy messages and updates tasks
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
		Msg("started busboy message stream")

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

// handleMessage processes a Busboy message
func (tm *TaskManager) handleMessage(msg *busboyClient.Message) error {
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
	case TopicTasksUpdated:
		eventType = "task.updated"
		tm.updateTask(&task)
	case TopicTasksDeleted:
		eventType = "task.deleted"
		tm.deleteTask(task.ID)
	default:
		// Infer from task existence
		if tm.taskExists(task.ID) {
			eventType = "task.updated"
			tm.updateTask(&task)
		} else {
			eventType = "task.created"
			tm.addTask(&task)
		}
	}

	// Broadcast to SSE clients
	tm.broadcast(eventType, &task)

	log.Info().
		Str("event", eventType).
		Str("task_id", task.ID).
		Str("message_id", msg.MessageID).
		Msg("processed busboy message")

	return nil
}

// CreateTask creates a new task and publishes to Busboy
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

	// Add to local store
	tm.addTask(task)

	// Publish to Busboy
	if err := tm.publishTask(ctx, TopicTasksCreated, task); err != nil {
		// Rollback local add
		tm.deleteTask(task.ID)
		return err
	}

	return nil
}

// UpdateTask updates an existing task and publishes to Busboy
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

	// Store old task for rollback (getTask returns a copy since addTask/updateTask store copies)
	oldTask := tm.getTask(task.ID)

	// Update local store
	tm.updateTask(task)

	// Publish to Busboy
	if err := tm.publishTask(ctx, TopicTasksUpdated, task); err != nil {
		// Rollback update
		if oldTask != nil {
			tm.updateTask(oldTask)
		}
		return err
	}

	return nil
}

// DeleteTask deletes a task and publishes to Busboy
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

	// Delete from local store
	tm.deleteTask(taskID)

	// Publish to Busboy
	if err := tm.publishTask(ctx, TopicTasksDeleted, task); err != nil {
		// Rollback delete
		tm.addTask(task)
		return err
	}

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

// Close closes the Busboy client
func (tm *TaskManager) Close() error {
	if tm.client == nil {
		return ErrNilClient
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
		Msg("published task to busboy")

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
