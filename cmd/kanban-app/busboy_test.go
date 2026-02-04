package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	"unheaded/pkg/busboy-client/mock"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

func createTestTaskManager(t *testing.T) (*TaskManager, *mock.MockClient) {
	t.Helper()

	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}

	tm, err := NewTaskManager(mockClient, broadcast)
	if err != nil {
		t.Fatalf("failed to create task manager: %v", err)
	}

	return tm, mockClient
}

func createTestTask(id string) *Task {
	return &Task{
		ID:          id,
		Title:       "Test Task " + id,
		Description: "Test description",
		Status:      "todo",
		Type:        "task",
		Owner:       "tester",
		Progress:    0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// ============================================================================
// CONSTRUCTOR TESTS
// ============================================================================

func TestNewTaskManager_ValidInputs(t *testing.T) {
	mockClient := mock.NewMockClient()
	broadcast := func(eventType string, data interface{}) {}

	tm, err := NewTaskManager(mockClient, broadcast)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if tm == nil {
		t.Fatal("expected non-nil task manager")
	}

	if tm.client != mockClient {
		t.Error("client not set correctly")
	}

	if tm.tasks == nil {
		t.Error("tasks map not initialized")
	}
}

func TestNewTaskManager_NilClient_ReturnsError(t *testing.T) {
	broadcast := func(eventType string, data interface{}) {}

	tm, err := NewTaskManager(nil, broadcast)
	if err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}

	if tm != nil {
		t.Error("expected nil task manager on error")
	}
}

func TestNewTaskManager_NilBroadcast_ReturnsError(t *testing.T) {
	mockClient := mock.NewMockClient()

	tm, err := NewTaskManager(mockClient, nil)
	if err == nil {
		t.Error("expected error for nil broadcast function")
	}

	if tm != nil {
		t.Error("expected nil task manager on error")
	}
}

// ============================================================================
// INITIALIZATION TESTS
// ============================================================================

func TestTaskManager_Initialize_LoadsInitialTasks(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	ctx := context.Background()
	if err := tm.Initialize(ctx); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tasks := tm.GetAllTasks()
	if len(tasks) == 0 {
		t.Error("expected initial tasks to be loaded")
	}
}

func TestTaskManager_Initialize_SubscribesToBusboy(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)

	ctx := context.Background()
	if err := tm.Initialize(ctx); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !tm.IsSubscribed() {
		t.Error("expected subscribed status to be true")
	}

	if mockClient.GetSubscribeCount() == 0 {
		t.Error("expected at least one Subscribe call")
	}
}

func TestTaskManager_Initialize_NilContext_UsesBackground(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	// Pass nil context - should not panic
	if err := tm.Initialize(nil); err != nil {
		t.Fatalf("Initialize with nil context failed: %v", err)
	}
}

// ============================================================================
// CREATE TASK TESTS
// ============================================================================

func TestTaskManager_CreateTask_HappyPath(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	// Initialize to subscribe
	tm.Initialize(ctx)

	// Subscribe to the specific topic that CreateTask publishes to
	// The mock client requires explicit subscriptions for each topic to publish
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	task := createTestTask("task-1")

	if err := tm.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Verify task was added
	retrieved, err := tm.GetTask("task-1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if retrieved.ID != task.ID {
		t.Errorf("expected task ID %s, got %s", task.ID, retrieved.ID)
	}

	// Verify published to Busboy
	if mockClient.GetPublishCount() == 0 {
		t.Error("expected task to be published to Busboy")
	}
}

func TestTaskManager_CreateTask_NilTask_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	err := tm.CreateTask(ctx, nil)
	if err != ErrInvalidTask {
		t.Errorf("expected ErrInvalidTask, got %v", err)
	}
}

func TestTaskManager_CreateTask_EmptyID_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	task := createTestTask("task-1")
	task.ID = ""

	err := tm.CreateTask(ctx, task)
	if err != ErrEmptyTaskID {
		t.Errorf("expected ErrEmptyTaskID, got %v", err)
	}
}

func TestTaskManager_CreateTask_InvalidTask_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	testCases := []struct {
		name     string
		modifier func(*Task)
	}{
		{"empty title", func(t *Task) { t.Title = "" }},
		{"empty status", func(t *Task) { t.Status = "" }},
		{"invalid status", func(t *Task) { t.Status = "invalid" }},
		{"negative progress", func(t *Task) { t.Progress = -1 }},
		{"progress over 100", func(t *Task) { t.Progress = 101 }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := createTestTask("test-task")
			tc.modifier(task)

			err := tm.CreateTask(ctx, task)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestTaskManager_CreateTask_DuplicateID_ReturnsError(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)
	// Subscribe to the specific topic that CreateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	task := createTestTask("duplicate")

	// Create first time - should succeed
	if err := tm.CreateTask(ctx, task); err != nil {
		t.Fatalf("first CreateTask failed: %v", err)
	}

	// Create again - should fail
	err := tm.CreateTask(ctx, task)
	if err != ErrTaskAlreadyExists {
		t.Errorf("expected ErrTaskAlreadyExists, got %v", err)
	}
}

func TestTaskManager_CreateTask_PublishFails_RollsBack(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)

	// Inject publish error
	publishErr := busboyClient.ErrSubscriptionPending
	mockClient.SetError("publish", publishErr)

	task := createTestTask("rollback-test")

	err := tm.CreateTask(ctx, task)
	if err == nil {
		t.Fatal("expected error when publish fails")
	}

	// Verify task was NOT added (rollback successful)
	_, err = tm.GetTask("rollback-test")
	if err != ErrTaskNotFound {
		t.Error("expected task to be rolled back")
	}
}

// ============================================================================
// UPDATE TASK TESTS
// ============================================================================

func TestTaskManager_UpdateTask_HappyPath(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)
	// Subscribe to the specific topics for CreateTask and UpdateTask
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	// Create task first
	task := createTestTask("update-test")
	if err := tm.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Subscribe to the specific topic that UpdateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksUpdated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksUpdated)

	// Update task
	task.Title = "Updated Title"
	task.Progress = 50

	initialPublishCount := mockClient.GetPublishCount()

	if err := tm.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	// Verify update
	retrieved, _ := tm.GetTask("update-test")
	if retrieved.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %s", retrieved.Title)
	}

	if retrieved.Progress != 50 {
		t.Errorf("expected progress 50, got %d", retrieved.Progress)
	}

	// Verify published
	if mockClient.GetPublishCount() <= initialPublishCount {
		t.Error("expected update to be published")
	}
}

func TestTaskManager_UpdateTask_NotFound_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	task := createTestTask("nonexistent")

	err := tm.UpdateTask(ctx, task)
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestTaskManager_UpdateTask_PublishFails_RollsBack(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)
	// Subscribe to the specific topic that CreateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	// Create task
	task := createTestTask("rollback-update")
	originalTitle := task.Title
	if err := tm.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Subscribe to the specific topic that UpdateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksUpdated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksUpdated)

	// Inject error for update publish
	mockClient.SetError("publish", busboyClient.ErrRateLimited)

	// Try to update
	task.Title = "Should Rollback"
	err := tm.UpdateTask(ctx, task)
	if err == nil {
		t.Fatal("expected error when publish fails")
	}

	// Verify rollback
	retrieved, _ := tm.GetTask("rollback-update")
	if retrieved.Title != originalTitle {
		t.Errorf("expected rollback to original title, got %s", retrieved.Title)
	}
}

// ============================================================================
// DELETE TASK TESTS
// ============================================================================

func TestTaskManager_DeleteTask_HappyPath(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)
	// Subscribe to the specific topic that CreateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	// Create task
	task := createTestTask("delete-test")
	if err := tm.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Subscribe to the specific topic that DeleteTask publishes to
	mockClient.Subscribe(ctx, TopicTasksDeleted, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksDeleted)

	// Delete task
	if err := tm.DeleteTask(ctx, "delete-test"); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	// Verify deleted
	_, err := tm.GetTask("delete-test")
	if err != ErrTaskNotFound {
		t.Error("expected task to be deleted")
	}
}

func TestTaskManager_DeleteTask_EmptyID_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	err := tm.DeleteTask(ctx, "")
	if err != ErrEmptyTaskID {
		t.Errorf("expected ErrEmptyTaskID, got %v", err)
	}
}

func TestTaskManager_DeleteTask_NotFound_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	err := tm.DeleteTask(ctx, "nonexistent")
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestTaskManager_DeleteTask_PublishFails_RollsBack(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)
	// Subscribe to the specific topic that CreateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	// Create task
	task := createTestTask("rollback-delete")
	if err := tm.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Subscribe to the specific topic that DeleteTask publishes to
	mockClient.Subscribe(ctx, TopicTasksDeleted, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksDeleted)

	// Inject error
	mockClient.SetError("publish", busboyClient.ErrRateLimited)

	// Try to delete
	err := tm.DeleteTask(ctx, "rollback-delete")
	if err == nil {
		t.Fatal("expected error when publish fails")
	}

	// Verify rollback (task still exists)
	_, err = tm.GetTask("rollback-delete")
	if err != nil {
		t.Error("expected task to be rolled back (still exists)")
	}
}

// ============================================================================
// GET TASK TESTS
// ============================================================================

func TestTaskManager_GetTask_Exists(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)

	// Get one of the initial tasks
	tasks := tm.GetAllTasks()
	if len(tasks) == 0 {
		t.Fatal("no initial tasks loaded")
	}

	firstTask := tasks[0]

	retrieved, err := tm.GetTask(firstTask.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if retrieved.ID != firstTask.ID {
		t.Errorf("expected ID %s, got %s", firstTask.ID, retrieved.ID)
	}
}

func TestTaskManager_GetTask_NotFound(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	_, err := tm.GetTask("nonexistent")
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestTaskManager_GetTask_EmptyID_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	_, err := tm.GetTask("")
	if err != ErrEmptyTaskID {
		t.Errorf("expected ErrEmptyTaskID, got %v", err)
	}
}

// ============================================================================
// MESSAGE HANDLING TESTS
// ============================================================================

func TestTaskManager_HandleMessage_TaskCreated(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	task := createTestTask("msg-create")
	payload, _ := json.Marshal(task)

	msg := &busboyClient.Message{
		MessageID: "test-msg-1",
		Topic:     TopicTasksCreated,
		SenderID:  "test-sender",
		CreatedAt: time.Now(),
		Seq:       1,
		Payload:   string(payload),
	}

	if err := tm.handleMessage(msg); err != nil {
		t.Fatalf("handleMessage failed: %v", err)
	}

	// Verify task was added
	_, err := tm.GetTask("msg-create")
	if err != nil {
		t.Error("expected task to be added from message")
	}
}

func TestTaskManager_HandleMessage_TaskUpdated(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	// Add task first
	task := createTestTask("msg-update")
	tm.addTask(task)

	// Update via message
	task.Title = "Updated via Message"
	payload, _ := json.Marshal(task)

	msg := &busboyClient.Message{
		MessageID: "test-msg-2",
		Topic:     TopicTasksUpdated,
		SenderID:  "test-sender",
		CreatedAt: time.Now(),
		Seq:       2,
		Payload:   string(payload),
	}

	if err := tm.handleMessage(msg); err != nil {
		t.Fatalf("handleMessage failed: %v", err)
	}

	// Verify update
	retrieved, _ := tm.GetTask("msg-update")
	if retrieved.Title != "Updated via Message" {
		t.Error("expected task to be updated from message")
	}
}

func TestTaskManager_HandleMessage_NilMessage_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	err := tm.handleMessage(nil)
	if err == nil {
		t.Error("expected error for nil message")
	}
}

func TestTaskManager_HandleMessage_InvalidJSON_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	msg := &busboyClient.Message{
		MessageID: "test-bad",
		Topic:     TopicTasksCreated,
		Payload:   "invalid json{",
	}

	err := tm.handleMessage(msg)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestTaskManager_HandleMessage_InvalidTask_ReturnsError(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	invalidTask := createTestTask("invalid")
	invalidTask.Title = "" // Invalid

	payload, _ := json.Marshal(invalidTask)

	msg := &busboyClient.Message{
		MessageID: "test-invalid",
		Topic:     TopicTasksCreated,
		Payload:   string(payload),
	}

	err := tm.handleMessage(msg)
	if err == nil {
		t.Error("expected validation error for invalid task")
	}
}

// ============================================================================
// VALIDATION TESTS
// ============================================================================

func TestTaskManager_ValidateTask_HappyPath(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	task := createTestTask("valid")

	if err := tm.validateTask(task); err != nil {
		t.Errorf("validation failed for valid task: %v", err)
	}
}

func TestTaskManager_ValidateTask_NilTask(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	err := tm.validateTask(nil)
	if err == nil {
		t.Error("expected error for nil task")
	}
}

func TestTaskManager_ValidateTask_EmptyFields(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	testCases := []struct {
		name     string
		modifier func(*Task)
	}{
		{"empty ID", func(t *Task) { t.ID = "" }},
		{"empty title", func(t *Task) { t.Title = "" }},
		{"empty status", func(t *Task) { t.Status = "" }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := createTestTask("test")
			tc.modifier(task)

			err := tm.validateTask(task)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestTaskManager_ValidateTask_InvalidStatus(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	task := createTestTask("test")
	task.Status = "invalid-status"

	err := tm.validateTask(task)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestTaskManager_ValidateTask_InvalidProgress(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	testCases := []struct {
		name     string
		progress int
	}{
		{"negative", -1},
		{"over 100", 101},
		{"way over", 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := createTestTask("test")
			task.Progress = tc.progress

			err := tm.validateTask(task)
			if err == nil {
				t.Error("expected error for invalid progress")
			}
		})
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestTaskManager_ConcurrentReads(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)

	const numReaders = 100
	done := make(chan bool, numReaders)

	for i := 0; i < numReaders; i++ {
		go func() {
			_ = tm.GetAllTasks()
			done <- true
		}()
	}

	for i := 0; i < numReaders; i++ {
		<-done
	}
}

func TestTaskManager_ConcurrentWrites(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)
	mockClient.ApproveSubscription(TopicTasks)

	const numWriters = 50
	done := make(chan bool, numWriters)

	for i := 0; i < numWriters; i++ {
		go func(n int) {
			task := createTestTask(time.Now().Format("20060102150405") + string(rune('a'+n%26)))
			_ = tm.CreateTask(ctx, task)
			done <- true
		}(i)
	}

	for i := 0; i < numWriters; i++ {
		<-done
	}
}

func TestTaskManager_ConcurrentReadWrite(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	ctx := context.Background()

	tm.Initialize(ctx)
	mockClient.ApproveSubscription(TopicTasks)

	const numOps = 100
	done := make(chan bool, numOps*2)

	// Concurrent readers
	for i := 0; i < numOps; i++ {
		go func() {
			_ = tm.GetAllTasks()
			done <- true
		}()
	}

	// Concurrent writers
	for i := 0; i < numOps; i++ {
		go func(n int) {
			task := createTestTask(time.Now().Format("20060102150405") + string(rune('a'+n%26)))
			_ = tm.CreateTask(ctx, task)
			done <- true
		}(i)
	}

	for i := 0; i < numOps*2; i++ {
		<-done
	}
}
