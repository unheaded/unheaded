// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package micromanager

import (
	"sync"
	"time"
)

// Store manages task persistence (in-memory with mutex for concurrency)
type Store struct {
	tasks map[string]*Task
	mu    sync.RWMutex
}

// NewStore creates a new task store
func NewStore() *Store {
	return &Store{
		tasks: make(map[string]*Task),
	}
}

// Create adds a new task to the store
// Returns error if task is invalid or ID already exists
func (s *Store) Create(task *Task) error {
	if task == nil {
		return ErrNilTask
	}
	if err := task.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[task.ID]; exists {
		return TaskIDAlreadyExists(task.ID)
	}

	s.tasks[task.ID] = task
	return nil
}

// Get retrieves a task by ID
func (s *Store) Get(id string) (*Task, error) {
	if id == "" {
		return nil, ErrEmptyID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	task, exists := s.tasks[id]
	if !exists {
		return nil, ErrTaskNotFound
	}

	return task, nil
}

// Update modifies an existing task
func (s *Store) Update(task *Task) error {
	if task == nil {
		return ErrNilTask
	}
	if err := task.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[task.ID]; !exists {
		return ErrTaskNotFound
	}

	task.UpdatedAt = time.Now()
	s.tasks[task.ID] = task
	return nil
}

// Delete removes a task from the store
func (s *Store) Delete(id string) error {
	if id == "" {
		return ErrEmptyID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[id]; !exists {
		return ErrTaskNotFound
	}

	delete(s.tasks, id)
	return nil
}

// ListAll returns all tasks
func (s *Store) ListAll() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// ListByStatus returns all tasks with a given status
func (s *Store) ListByStatus(status TaskStatus) ([]*Task, error) {
	if !isValidStatus(status) {
		return nil, ErrInvalidStatus
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.Status == status {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

// ListByOwner returns all tasks owned by a specific owner
func (s *Store) ListByOwner(owner string) ([]*Task, error) {
	if owner == "" {
		return nil, ErrEmptyOwner
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.Owner == owner {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

// Count returns the total number of tasks
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.tasks)
}

// TaskIDAlreadyExists returns error with ID detail
func TaskIDAlreadyExists(id string) error {
	return &storeError{
		msg:    "task id already exists: " + id,
		taskID: id,
	}
}

type storeError struct {
	msg    string
	taskID string
}

func (e *storeError) Error() string {
	return e.msg
}
