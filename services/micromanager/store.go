// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package micromanager

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/statefile"
)

// Store manages task persistence (in-memory with mutex for concurrency)
type Store struct {
	tasks map[string]*Task
	mu    sync.RWMutex

	// state persists the tasks; nil = memory only, and the API refuses
	// writes (EnablePersistence). Until 2026-10-23 there was no file at all
	// and a restart lost every task.
	state *statefile.File
}

// ErrPersist wraps a failed save; the change it was for is undone.
var ErrPersist = errors.New("persist micromanager tasks")

const stateFileName = "micromanager-tasks.json"

// EnablePersistence loads dir's task file (if any) and saves every later
// change to it. A damaged file is an error, never an empty board.
func (s *Store) EnablePersistence(dir string) error {
	f := statefile.In(dir, stateFileName)
	if f == nil {
		return statefile.ErrNoDir
	}
	tasks := map[string]*Task{}
	if _, err := f.Load(&tasks); err != nil {
		return err
	}
	for id, t := range tasks {
		if t == nil || t.ID != id {
			return fmt.Errorf("%s: entry %q does not carry its own id", f.Path, id)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range tasks {
		s.tasks[id] = t
	}
	s.state = f
	return nil
}

// Persistent reports whether writes reach disk.
func (s *Store) Persistent() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state != nil
}

// saveLocked writes every task; the caller holds s.mu.
func (s *Store) saveLocked() error {
	if s.state == nil {
		return nil
	}
	if err := s.state.Save(s.tasks); err != nil {
		return fmt.Errorf("%w: %v", ErrPersist, err)
	}
	return nil
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
	if err := s.saveLocked(); err != nil {
		delete(s.tasks, task.ID)
		return err
	}
	return nil
}

// GetCopy retrieves a copy of a task by ID: callers change the copy and hand
// it to Update, so a half-applied change never reaches the store.
func (s *Store) GetCopy(id string) (*Task, error) {
	t, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	c := *t
	c.Tags = append([]string(nil), t.Tags...)
	return &c, nil
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

	prev, exists := s.tasks[task.ID]
	if !exists {
		return ErrTaskNotFound
	}

	task.UpdatedAt = time.Now()
	s.tasks[task.ID] = task
	if err := s.saveLocked(); err != nil {
		s.tasks[task.ID] = prev
		return err
	}
	return nil
}

// Delete removes a task from the store
func (s *Store) Delete(id string) error {
	if id == "" {
		return ErrEmptyID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	prev, exists := s.tasks[id]
	if !exists {
		return ErrTaskNotFound
	}

	delete(s.tasks, id)
	if err := s.saveLocked(); err != nil {
		s.tasks[id] = prev
		return err
	}
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
